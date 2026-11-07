package menu

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/cruiserxyz/sextile/internal/ui/term"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// list_row.go is the one renderer every list modal draws its pickable rows
// through (the specification). The city picker, the chart picker, the
// aspect chooser and the body selector each say WHAT a row holds; how a row is
// laid out, shortened and marked as the cursor row is decided here and
// nowhere else.
//
// A row is the cursor column, a left cell, and right cells pinned to the right
// edge. The line it returns is exactly the width it was given, cursor band and
// all, and the caller hands it to the box untouched: anything that trimmed or
// padded it afterwards would cut the band short or pull the right edge off
// true, on some row at some width. TestListModalsShareOneRowRenderer (internal/ui) holds that by following the code.

// RowSpec is what a modal knows about one row.
type RowSpec struct {
	// Left is the row's identity: a name, or a checkbox and a name.
	Left string

	// LeftTail is plain text that follows Left, kept whole while Left
	// shortens, and drawn faint when colours are on. Its cells count in the
	// layout like any other.
	LeftTail string

	// Right are the columns pinned to the right edge, in display order.
	// Empty cells are dropped before anything is laid out.
	Right []string

	// Cursor makes this the cursor row: the shared cursor-row treatment
	// (RenderRowState), and with colours off the cursor marker in the row's
	// first column.
	Cursor bool

	// Pinned keeps every right cell whole. The left cell gives way first and
	// without a floor; a caller that needs a right cell to go when room is
	// short decides that before it builds the row. The aspect chooser's row
	// is pinned (the specification).
	Pinned bool

	// Dim draws the whole row faint when colours are on. With colours off the
	// row's own text has to say what the dimming would (the body popup's `!`).
	Dim bool
}

const (
	// listRowGap is the space between the left cell and the right block,
	// at least, and between right cells, exactly.
	listRowGap = 3

	// ListRowLeftFloor is how far the left cell shortens before the right
	// cells start to give way.
	ListRowLeftFloor = 8

	// listRowRightFloor is how far a right cell shortens before it is
	// dropped instead.
	listRowRightFloor = 4
)

// Row renders one row exactly width cells wide. The cursor column exists
// only with colours off; with colours on the band alone marks the cursor row.
func Row(r RowSpec, cfg term.Style, width int) string {
	mark := ""
	if !cfg.UseColor() {
		mark = CursorColumn(r.Cursor, cfg)
	}
	room := width - ansi.StringWidth(mark)

	var right []string
	for _, c := range r.Right {
		if c != "" {
			right = append(right, c)
		}
	}

	line := mark + layoutListRow(r.Left, r.LeftTail, right, r.Pinned, room, cfg)
	// The layout is exact; this only guards a width below the cursor
	// column, which the modal box never produces (its content is >= 54).
	line = term.FitLine(line, width, cfg)

	line = RenderRowState(line, width, r.Cursor, cfg)
	if r.Dim && cfg.UseColor() {
		// Applied to the finished line: the layout above counts cells, and a
		// style inside the left cell would be cut like text. Faint outlives
		// the cursor band's own start, so it holds on the cursor row too.
		line = term.SGRFaint + line + term.SGRReset
	}
	return line
}

// layoutListRow places the left cell and the right block in room cells. When
// they do not fit, the left cell shortens to its floor, then the right cells
// shorten first to last to theirs, then right cells are dropped first to last
// and the whole thing is laid out again with the room that freed.
func layoutListRow(left, tail string, right []string, pinned bool, room int, cfg term.Style) string {
	for {
		cells := append([]string(nil), right...)
		l := left
		tw := ansi.StringWidth(tail)
		over := func() int { return ansi.StringWidth(l) + tw + rightBlockWidth(cells) - room }

		if over() > 0 {
			floor := ListRowLeftFloor
			if pinned || len(cells) == 0 {
				floor = 1
			}
			if lw := ansi.StringWidth(l); lw > floor {
				l = term.ShortenRight(l, max(floor, lw-over()), cfg)
			}
		}
		if !pinned {
			for i := range cells {
				if o := over(); o > 0 {
					if cw := ansi.StringWidth(cells[i]); cw > listRowRightFloor {
						cells[i] = term.ShortenLeft(cells[i], max(listRowRightFloor, cw-o), cfg)
					}
				}
			}
			if over() > 0 && len(right) > 0 {
				right = right[1:]
				continue
			}
		}

		if tail != "" && cfg.UseColor() {
			l += term.SGRFaint + tail + term.SGRReset
		} else {
			l += tail
		}
		if len(cells) == 0 {
			return l
		}
		block := strings.Join(cells, strings.Repeat(" ", listRowGap))
		pad := room - ansi.StringWidth(l) - ansi.StringWidth(block)
		if pad < listRowGap {
			pad = listRowGap
		}
		return l + strings.Repeat(" ", pad) + block
	}
}

// RowWidth is the cells a row needs to be drawn whole: the cursor column
// colours off draw, the left cell and its tail, and the right block with the
// gaps. A box fitted to its widest row's RowWidth cuts nothing and leaves
// the widest row's gap at listRowGap, no wider.
func RowWidth(r RowSpec, cfg term.Style) int {
	w := ansi.StringWidth(r.Left) + ansi.StringWidth(r.LeftTail)
	if !cfg.UseColor() {
		w += ansi.StringWidth(CursorColumn(false, cfg))
	}
	var right []string
	for _, c := range r.Right {
		if c != "" {
			right = append(right, c)
		}
	}
	return w + rightBlockWidth(right)
}

// WidestRow is the widest of several rows, as the box must hold them.
func WidestRow(cfg term.Style, rows ...RowSpec) int {
	w := 0
	for _, r := range rows {
		w = max(w, RowWidth(r, cfg))
	}
	return w
}

// FitMessage is the width and the rows of a message the list shows in place of
// its rows -- a hint, a load error. It is measured wrapped at prose, the width a
// sentence in a popup wraps at; floor is the width the box has anyway (the search line's), and the message
// wraps at that, as Message wraps it at the width the box ends up with.
func FitMessage(s string, floor, prose int) (width, rows int) {
	for _, l := range strings.Split(ansi.Wrap(s, prose, ""), "\n") {
		width = max(width, ansi.StringWidth(l))
	}
	return width, len(strings.Split(ansi.Wrap(s, max(width, floor), ""), "\n"))
}

// rightBlockWidth is the right cells, their gaps, and the gap before them.
func rightBlockWidth(cells []string) int {
	if len(cells) == 0 {
		return 0
	}
	w := listRowGap * len(cells)
	for _, c := range cells {
		w += ansi.StringWidth(c)
	}
	return w
}

// Note pads a line that is not a pickable row — a placeholder, a count of
// hidden rows — to the list's width. It is never a cursor row.
func Note(s string, width int, cfg term.Style) string {
	return term.FitLine(s, width, cfg)
}

// Heading is a Note drawn bold, colours on or off, the way the Bodies lead
// row is: the padding that fills the width stays outside the bold span, so
// the line's right trim still finds its spaces. It is a title, never a row.
func Heading(s string, width int, cfg term.Style) string {
	line := term.FitLine(s, width, cfg)
	text := strings.TrimRight(line, " ")
	if text == "" {
		return line
	}
	return term.SGRBold + ReapplyAfterSGR(text, term.SGRBold) + term.SGRReset + line[len(text):]
}

// Rule is a dashed line across the list's width, dividing a pinned block
// from the list under it. It is not a row: the cursor never lands on it and no
// count includes it. `┈` with colours on, `-` with them off or accessible.
func Rule(cfg term.Style, width int) string {
	glyph := "┈"
	if cfg.UseASCII() || !cfg.UseColor() {
		glyph = "-"
	}
	return term.FitLine(strings.Repeat(glyph, width), width, cfg)
}

// Block stacks a list's lines and fills it to height with blank lines, so
// the box below never floats up. Every line it is given is already width cells
// wide; it adds lines, never cells.
func Block(lines []string, width, height int) string {
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, Note("", width, blankStyle{}))
	}
	return strings.Join(lines, "\n")
}

// Message fills the whole list with one message — a load error, a loading
// line, a hint — wrapped to the width rather than cut, because a message the
// operator cannot finish reading is a message they did not get.
func Message(s string, width, height int) string {
	lines := strings.Split(ansi.Wrap(s, width, ""), "\n")
	for i := range lines {
		lines[i] = Note(lines[i], width, blankStyle{})
	}
	return Block(lines, width, height)
}

// blankStyle is the launch configuration with nothing chosen: colours on, unicode
// glyphs, the default focus and cursor-row colours. It is what a bare ui.Config{}
// answered, for the empty rows that carry no mark.
type blankStyle struct{}

func (blankStyle) UseColor() bool                  { return true }
func (blankStyle) UseASCII() bool                  { return false }
func (blankStyle) FocusColour() lipgloss.Color     { return term.DefaultFocusColour }
func (blankStyle) CursorRowColour() lipgloss.Color { return term.DefaultCursorRowColour }
