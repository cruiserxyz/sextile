package ui

import tea "github.com/charmbracelet/bubbletea"

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

// fuzzyList is the state a filtered picker keeps: what was typed, which row the
// cursor is on, the rows the filter produced, and whether the filter had to cap
// them. The caller owns the filtering itself — it knows what it is filtering
// and how — and hands the result back through setMatches.
type fuzzyList[T any] struct {
	filterText string
	cursor     int
	// offset is the first visible row. It is REMEMBERED rather than derived
	// from the cursor, which is what lets the window hold still while the
	// cursor moves through its middle — see listScrollMargin.
	offset    int
	matches   []T
	truncated bool
}

// setMatches installs a filter result and clamps the cursor into it. Sending
// the cursor to the top rather than to the last row is the atlas picker's
// long-standing behaviour and is preserved verbatim.
func (l *fuzzyList[T]) setMatches(matches []T, truncated bool) {
	l.matches = matches
	l.truncated = truncated
	if l.cursor >= len(l.matches) {
		l.cursor = 0
	}
	// A new result set invalidates the old window: re-anchor at the top the
	// way every filter-as-you-type list does, rather than leaving the
	// operator scrolled into the middle of results they have not seen.
	l.offset = 0
}

// clear empties the result without touching the typed text — the state a
// picker is in before its source data has arrived.
func (l *fuzzyList[T]) clear() {
	l.matches = nil
	l.cursor = 0
	l.offset = 0
	l.truncated = false
}

// resetFilter clears the typed text and the cursor, for a re-open.
func (l *fuzzyList[T]) resetFilter() {
	l.filterText = ""
	l.cursor = 0
	l.offset = 0
}

// moveUp and moveDown walk the cursor without wrapping.
func (l *fuzzyList[T]) moveUp() {
	if l.cursor > 0 {
		l.cursor--
	}
}

func (l *fuzzyList[T]) moveDown() {
	if l.cursor+1 < len(l.matches) {
		l.cursor++
	}
}

// page applies a paging key to the cursor and the window by the one rule
// (pageCursor, cursor_page.go). visible is the number of list rows the owner's
// render shows: the SMALLEST count it can show when that depends on the window
// itself, so the cursor is never outside what is drawn.
func (l *fuzzyList[T]) page(op pageOp, visible int) {
	if len(l.matches) == 0 {
		return
	}
	l.cursor, l.offset = pageCursor(l.cursor, l.offset, len(l.matches), visible, op)
}

// pageBelow is page for a list whose cursor walks pinned rows first (the aspect
// families, the body groups): the cursor is the combined one, the list index is
// the cursor minus pinned, and the pinned rows are an always-visible header.
// Page down from a pinned row lands on the last visible list row; page up from
// the list's first row (nothing above it to scroll to) lands on pinned row 0;
// Home is pinned row 0, End the last list row.
func (l *fuzzyList[T]) pageBelow(pinned int, op pageOp, visible int) {
	n := len(l.matches)
	if n == 0 {
		if op == pageUp || op == pageHome {
			l.cursor = 0
		} else if pinned > 0 {
			l.cursor = pinned - 1
		}
		return
	}
	switch op {
	case pageHome:
		l.cursor, l.offset = 0, 0
		return
	case pageEnd:
		cursor, offset := pageCursor(n-1, l.offset, n, visible, pageEnd)
		l.cursor, l.offset = pinned+cursor, offset
		return
	}
	row := l.cursor - pinned
	if row < 0 {
		// On a pinned row. The window stands; the list row Page down lands on
		// is the last one it shows.
		if op == pageUp {
			l.cursor = 0
			return
		}
		visible = max(visible, 1)
		l.offset = min(max(l.offset, 0), max(n-visible, 0))
		l.cursor = pinned + min(l.offset+visible-1, n-1)
		return
	}
	if op == pageUp && row == 0 {
		l.cursor = 0
		return
	}
	row, l.offset = pageCursor(row, l.offset, n, visible, op)
	l.cursor = pinned + row
}

// backspace removes the last rune of the filter text, reporting whether
// anything changed so the caller can skip a needless re-filter.
func (l *fuzzyList[T]) backspace() bool {
	if len(l.filterText) == 0 {
		return false
	}
	r := []rune(l.filterText)
	l.filterText = string(r[:len(r)-1])
	return true
}

// appendRunes adds typed runes to the filter text, reporting whether anything
// changed.
func (l *fuzzyList[T]) appendRunes(runes []rune) bool {
	if len(runes) == 0 {
		return false
	}
	l.filterText += string(runes)
	return true
}

// typeKey is the search line's text entry: printable characters and Space
// type, Backspace deletes. It reports whether the filter text changed, so the
// caller re-filters only then. Every other key is its owner's table's
// (the specification); a checklist's Space is
// matched as Toggle before it could reach this.
func (l *fuzzyList[T]) typeKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyBackspace, tea.KeyCtrlH: // ^H is Backspace on some terminals (the specification)
		return l.backspace()
	case tea.KeySpace:
		return l.appendRunes([]rune{' '})
	case tea.KeyRunes:
		return l.appendRunes(msg.Runes)
	}
	return false
}

// listScrollMargin is how many rows stay visible on EACH side of the cursor
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
const listScrollMargin = 3

// scrollTo moves the window the minimum amount needed to keep cursor inside it
// with the margin. It is called when the cursor MOVES, not when the list is
// drawn: scrolling is a consequence of navigating, and computing it at render
// time is what forced the offset to be a function of the cursor in the first
// place.
func (l *fuzzyList[T]) scrollTo(cursor, height int) {
	if height <= 0 || len(l.matches) == 0 {
		l.offset = 0
		return
	}
	l.offset = scrollToShow(l.offset, cursor, len(l.matches), height)
}

// clampOffset keeps the window inside the list.
func (l *fuzzyList[T]) clampOffset(height int) {
	maxOffset := len(l.matches) - height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if l.offset > maxOffset {
		l.offset = maxOffset
	}
	if l.offset < 0 {
		l.offset = 0
	}
}

// windowAt returns the rows to draw and the index of the first one, using the
// offset scrollTo has already settled. It only clamps; it never stores a
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
func (l *fuzzyList[T]) windowAt(cursor, height int) ([]T, int) {
	if height <= 0 || len(l.matches) == 0 {
		return nil, 0
	}
	if len(l.matches) <= height {
		return l.matches, 0
	}
	l.clampOffset(height)
	off := l.offset
	if cursor >= 0 && cursor < len(l.matches) {
		if cursor < off {
			off = cursor
		}
		if cursor >= off+height {
			off = cursor - height + 1
		}
	}
	return l.matches[off : off+height], off
}

// pinnedBody lays out a checklist's content area: rows that stay put above the
// list, the list itself, and rows that stay put below it. Both blocks may be
// empty, which is how a plain picker uses this. The list block absorbs the
// remaining height and is filled so the box below never floats up.
//
// Every line it is given is already the list's width — rows from listRow,
// notes from listNote — so it stacks lines and never touches one.
type pinnedBody struct {
	Above []string
	Rows  []string
	Below []string
}

func renderPinnedBody(b pinnedBody, width, height int) string {
	lines := make([]string, 0, height)
	lines = append(lines, b.Above...)
	lines = append(lines, b.Rows...)

	// The pinned tail keeps its rows even when the list would fill the box.
	if room := height - len(b.Below); len(lines) > room && room >= 0 {
		lines = lines[:room]
	}
	for len(lines) < height-len(b.Below) {
		lines = append(lines, listNote("", width, Config{}))
	}
	lines = append(lines, b.Below...)
	return listBlock(lines, width, height)
}
