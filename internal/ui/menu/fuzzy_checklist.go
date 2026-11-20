package menu

import (
	tea "github.com/charmbracelet/bubbletea"
	"slices"

	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// fuzzy_checklist.go is the filtered-list chassis: the search buffer and its
// editor, the cursor and its clamping, and the pinned blocks a checklist modal
// draws above and below its rows.
//
// It is an EXTRACTION from the atlas picker, not a new design, and it
// names no domain concept on purpose — the two checklist modals landing next
// bind the same type, and a chassis that knew what it was listing could not be
// bound twice. The box chrome around it is modal_box.go; the two are separate
// because the chart picker shares the box but not the list.
//
// Deliberately NOT here yet: selection, group rows and their tri-state, and the
// tag bar. Those arrive with their first real caller rather than being designed
// against an imagined one. What IS here ahead of a caller is the pinned-block
// renderer, because the contract row that authorised this extraction requires
// the chassis to expose a block above and below the rows.

// FuzzyList is the state a filtered picker keeps: what was typed, which row the
// cursor is on, the rows the filter produced, and whether the filter had to cap
// them. The caller owns the filtering itself — it knows what it is filtering
// and how — and hands the result back through SetMatches.
type FuzzyList[T any] struct {
	FilterText string
	// Cursor is the row the cursor is on. With a Lead row it can also be
	// LeadCursor, the row above row 0.
	Cursor int
	// Offset is the first visible row. It is REMEMBERED rather than derived
	// from the cursor, which is what lets the window hold still while the
	// cursor moves through its middle — see ScrollMargin.
	Offset    int
	Matches   []T
	Truncated bool

	// Lead is the text of the one action row this list can carry above its
	// matches ("" for none). It stays first whatever the search line holds, the
	// cursor can rest on it (Cursor is LeadCursor), and Enter on it runs its
	// action; the owner draws it (LeadRow) and acts on it.
	Lead string
}

// LeadCursor is the cursor's place while it rests on the lead row.
const LeadCursor = -1

// HasLead says whether the list carries a lead row.
func (l *FuzzyList[T]) HasLead() bool { return l.Lead != "" }

// LeadRows is how many rows the lead row takes: one, or none.
func (l *FuzzyList[T]) LeadRows() int {
	if l.HasLead() {
		return 1
	}
	return 0
}

// OnLead says whether the cursor rests on the lead row.
func (l *FuzzyList[T]) OnLead() bool { return l.HasLead() && l.Cursor == LeadCursor }

// HomeCursor is the first place the cursor can take: the lead row, else row 0.
func (l *FuzzyList[T]) HomeCursor() int {
	if l.HasLead() {
		return LeadCursor
	}
	return 0
}

// LeadRow draws the lead row: its text, with the cursor treatment while the
// cursor rests on it. It is the one drawing the lead row uses.
func LeadRow(text string, onCursor bool, cfg term.Style, width int) string {
	return Row(RowSpec{Left: text, Cursor: onCursor}, cfg, width)
}

// SetMatches installs a filter result and clamps the cursor into it. Sending
// the cursor to the top rather than to the last row is the atlas picker's
// long-standing behaviour and is preserved verbatim.
func (l *FuzzyList[T]) SetMatches(matches []T, truncated bool) {
	l.Matches = matches
	l.Truncated = truncated
	if l.Cursor >= len(l.Matches) {
		l.Cursor = 0
	}
	// A new result set invalidates the old window: re-anchor at the top the
	// way every filter-as-you-type list does, rather than leaving the
	// operator scrolled into the middle of results they have not seen.
	l.Offset = 0
}

// Clear empties the result without touching the typed text — the state a
// picker is in before its source data has arrived.
func (l *FuzzyList[T]) Clear() {
	l.Matches = nil
	l.Cursor = 0
	l.Offset = 0
	l.Truncated = false
}

// ResetFilter clears the typed text and the cursor, for a re-open.
func (l *FuzzyList[T]) ResetFilter() {
	l.FilterText = ""
	l.Cursor = 0
	l.Offset = 0
}

// MoveUp and moveDown walk the cursor without wrapping.
func (l *FuzzyList[T]) MoveUp() {
	if l.Cursor > l.HomeCursor() {
		l.Cursor--
	}
}

func (l *FuzzyList[T]) MoveDown() {
	if l.Cursor+1 < len(l.Matches) {
		l.Cursor++
	}
}

// Page applies a paging key to the cursor and the window by the one rule
// (PageCursor, cursor_page.go). visible is the number of list rows the owner's
// render shows: the SMALLEST count it can show when that depends on the window
// itself, so the cursor is never outside what is drawn.
func (l *FuzzyList[T]) Page(op PageOp, visible int) {
	if l.Cursor < 0 {
		// A page key leaves the lead row for the list.
		l.Cursor = 0
	}
	if len(l.Matches) == 0 {
		return
	}
	l.Cursor, l.Offset = PageCursor(l.Cursor, l.Offset, len(l.Matches), visible, op)
}

// PageBelow is page for a list whose cursor walks pinned rows first (the aspect
// families, the body groups): the cursor is the combined one, the list index is
// the cursor minus pinned, and the pinned rows are an always-visible header.
// Page down from a pinned row lands on the last visible list row; page up from
// the list's first row (nothing above it to scroll to) lands on pinned row 0;
// Home is pinned row 0, End the last list row. None of them stops on a lead
// row: the cursor reaches it with the up key and leaves it with any page key.
func (l *FuzzyList[T]) PageBelow(pinned int, op PageOp, visible int) {
	n := len(l.Matches)
	if n == 0 {
		if op == PageUp || op == PageHome {
			l.Cursor = 0
		} else if pinned > 0 {
			l.Cursor = pinned - 1
		}
		return
	}
	switch op {
	case PageHome:
		l.Cursor, l.Offset = 0, 0
		return
	case PageEnd:
		cursor, offset := PageCursor(n-1, l.Offset, n, visible, PageEnd)
		l.Cursor, l.Offset = pinned+cursor, offset
		return
	}
	row := l.Cursor - pinned
	if row < 0 {
		// On a pinned row. The window stands; the list row Page down lands on
		// is the last one it shows.
		if op == PageUp {
			l.Cursor = 0
			return
		}
		visible = max(visible, 1)
		l.Offset = min(max(l.Offset, 0), max(n-visible, 0))
		l.Cursor = pinned + min(l.Offset+visible-1, n-1)
		return
	}
	if op == PageUp && row == 0 {
		l.Cursor = 0
		return
	}
	row, l.Offset = PageCursor(row, l.Offset, n, visible, op)
	l.Cursor = pinned + row
}

// backspace removes the last rune of the filter text, reporting whether
// anything changed so the caller can skip a needless re-filter.
func (l *FuzzyList[T]) backspace() bool {
	if len(l.FilterText) == 0 {
		return false
	}
	r := []rune(l.FilterText)
	l.FilterText = string(r[:len(r)-1])
	return true
}

// AppendRunes adds typed runes to the filter text, reporting whether anything
// changed.
func (l *FuzzyList[T]) AppendRunes(runes []rune) bool {
	if len(runes) == 0 {
		return false
	}
	l.FilterText += string(runes)
	return true
}

// TypeKey is the search line's text entry: printable characters and Space
// type, Backspace deletes. A comma is never text in a filter: the leader key
// opens the command line before a key reaches here, and a comma that is not the
// leader is dropped. It reports whether the filter text changed, so the
// caller re-filters only then. Every other key is its owner's table's
// (the specification); a checklist's Space is
// matched as Toggle before it could reach this.
func (l *FuzzyList[T]) TypeKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyBackspace, tea.KeyCtrlH: // ^H is Backspace on some terminals (the specification)
		return l.backspace()
	case tea.KeySpace:
		return l.AppendRunes([]rune{' '})
	case tea.KeyRunes:
		return l.AppendRunes(slices.DeleteFunc(slices.Clone(msg.Runes), func(r rune) bool { return r == ',' }))
	}
	return false
}

// ScrollMargin is how many rows stay visible on EACH side of the cursor
// before the window scrolls. Three above and three below: the cursor moves
// freely through the middle of the window and the list only moves when the
// cursor would come within the margin of an edge.
//
// The symmetry is the point. An earlier version computed the offset as a pure
// function of the cursor, which gives a bottom margin only: walking down, the
// cursor crosses the window and then the list scrolls under a pinned cursor —
// but walking back up, the list scrolled on EVERY step because the offset
// tracked the cursor in lockstep. Going down felt like moving a cursor and
// going up felt like dragging the list, which is not one interaction. Keeping
// the offset means both directions behave the same way.
const ScrollMargin = 3

// ScrollTo moves the window the minimum amount needed to keep cursor inside it
// with the margin. It is called when the cursor MOVES, not when the list is
// drawn: scrolling is a consequence of navigating, and computing it at render
// time is what forced the offset to be a function of the cursor in the first
// place.
func (l *FuzzyList[T]) ScrollTo(cursor, height int) {
	if height <= 0 || len(l.Matches) == 0 {
		l.Offset = 0
		return
	}
	l.Offset = ScrollToShow(l.Offset, cursor, len(l.Matches), height)
}

// clampOffset keeps the window inside the list.
func (l *FuzzyList[T]) clampOffset(height int) {
	maxOffset := len(l.Matches) - height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if l.Offset > maxOffset {
		l.Offset = maxOffset
	}
	if l.Offset < 0 {
		l.Offset = 0
	}
}

// WindowAt returns the rows to draw and the index of the first one, using the
// offset ScrollTo has already settled. It only clamps; it never stores a
// scroll, so drawing the same state twice cannot move the list.
//
// The drawn window always holds cursor's row when cursor is in the list. The
// stored offset is settled by navigation, but two things move the window or
// its height without a key: a filter re-anchors the offset at the top while
// keeping a cursor that still fits, and a terminal that shrinks leaves an
// offset laid out for a taller window. Either way Enter acts on the cursor's
// row, so that row is the one on screen. A negative cursor means the cursor is
// not in this list (the aspect chooser's family block), and the stored window
// stands.
func (l *FuzzyList[T]) WindowAt(cursor, height int) ([]T, int) {
	if height <= 0 || len(l.Matches) == 0 {
		return nil, 0
	}
	if len(l.Matches) <= height {
		return l.Matches, 0
	}
	l.clampOffset(height)
	off := l.Offset
	if cursor >= 0 && cursor < len(l.Matches) {
		if cursor < off {
			off = cursor
		}
		if cursor >= off+height {
			off = cursor - height + 1
		}
	}
	return l.Matches[off : off+height], off
}

// PinnedBody lays out a checklist's content area: rows that stay put above the
// list, the list itself, and rows that stay put below it. Both blocks may be
// empty, which is how a plain picker uses this. The list block absorbs the
// remaining height and is filled so the box below never floats up.
//
// Every line it is given is already the list's width — rows from Row,
// notes from Note — so it stacks lines and never touches one.
type PinnedBody struct {
	Above []string
	Rows  []string
	Below []string
}

func RenderPinnedBody(b PinnedBody, width, height int) string {
	lines := make([]string, 0, height)
	lines = append(lines, b.Above...)
	lines = append(lines, b.Rows...)

	// The pinned tail keeps its rows even when the list would fill the box.
	if room := height - len(b.Below); len(lines) > room && room >= 0 {
		lines = lines[:room]
	}
	for len(lines) < height-len(b.Below) {
		lines = append(lines, Note("", width, blankStyle{}))
	}
	lines = append(lines, b.Below...)
	return Block(lines, width, height)
}
