package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/store"
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

// listRowSpec is what a modal knows about one row.
type listRowSpec struct {
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
	// (renderRowState), and with colours off the cursor marker in the row's
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

	// listRowLeftFloor is how far the left cell shortens before the right
	// cells start to give way.
	listRowLeftFloor = 8

	// listRowRightFloor is how far a right cell shortens before it is
	// dropped instead.
	listRowRightFloor = 4
)

// listRow renders one row exactly width cells wide. The cursor column exists
// only with colours off; with colours on the band alone marks the cursor row.
func listRow(r listRowSpec, cfg Config, width int) string {
	mark := ""
	if !cfg.useColor() {
		mark = cursorColumn(r.Cursor, cfg)
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
	line = fitLine(line, width, cfg)

	line = renderRowState(line, width, r.Cursor, cfg)
	if r.Dim && cfg.useColor() {
		// Applied to the finished line: the layout above counts cells, and a
		// style inside the left cell would be cut like text. Faint outlives
		// the cursor band's own start, so it holds on the cursor row too.
		line = sgrFaint + line + sgrReset
	}
	return line
}

// layoutListRow places the left cell and the right block in room cells. When
// they do not fit, the left cell shortens to its floor, then the right cells
// shorten first to last to theirs, then right cells are dropped first to last
// and the whole thing is laid out again with the room that freed.
func layoutListRow(left, tail string, right []string, pinned bool, room int, cfg Config) string {
	for {
		cells := append([]string(nil), right...)
		l := left
		tw := ansi.StringWidth(tail)
		over := func() int { return ansi.StringWidth(l) + tw + rightBlockWidth(cells) - room }

		if over() > 0 {
			floor := listRowLeftFloor
			if pinned || len(cells) == 0 {
				floor = 1
			}
			if lw := ansi.StringWidth(l); lw > floor {
				l = shortenRight(l, max(floor, lw-over()), cfg)
			}
		}
		if !pinned {
			for i := range cells {
				if o := over(); o > 0 {
					if cw := ansi.StringWidth(cells[i]); cw > listRowRightFloor {
						cells[i] = shortenLeft(cells[i], max(listRowRightFloor, cw-o), cfg)
					}
				}
			}
			if over() > 0 && len(right) > 0 {
				right = right[1:]
				continue
			}
		}

		if tail != "" && cfg.useColor() {
			l += sgrFaint + tail + sgrReset
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

// listNote pads a line that is not a pickable row — a placeholder, a count of
// hidden rows — to the list's width. It is never a cursor row.
func listNote(s string, width int, cfg Config) string {
	return fitLine(s, width, cfg)
}

// listRule is a dashed line across the list's width, dividing a pinned block
// from the list under it. It is not a row: the cursor never lands on it and no
// count includes it. `┈` with colours on, `-` with them off or accessible.
func listRule(cfg Config, width int) string {
	glyph := "┈"
	if cfg.useASCII() || !cfg.useColor() {
		glyph = "-"
	}
	return fitLine(strings.Repeat(glyph, width), width, cfg)
}

// listBlock stacks a list's lines and fills it to height with blank lines, so
// the box below never floats up. Every line it is given is already width cells
// wide; it adds lines, never cells.
func listBlock(lines []string, width, height int) string {
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, listNote("", width, Config{}))
	}
	return strings.Join(lines, "\n")
}

// listMessage fills the whole list with one message — a load error, a loading
// line, a hint — wrapped to the width rather than cut, because a message the
// operator cannot finish reading is a message they did not get.
func listMessage(s string, width, height int) string {
	return lipgloss.NewStyle().Width(width).Height(height).Render(s)
}

// applyDisplayConfig takes the display: block into the active configuration.
// The reader reports; the decision and any Log line are made here, as for the
// keys: block. Silence is the contract for the good cases.
//
// A colour accepts the two forms lipgloss renders the same on every terminal
// it supports: a 256-colour palette number, or #rrggbb. Anything else would be
// handed to the terminal as a colour it might draw as anything, so it is
// refused with one Log note and the default stands. The retired row_stripe
// gets one note per run, however often the block is applied.
func (m Model) applyDisplayConfig(d store.DisplayConfig) Model {
	for _, c := range []struct {
		key, val string
		into     *string
	}{
		{"display.focus_color", d.FocusColour, &m.cfg.FocusColour},
		{"display.cursor_row_color", d.CursorRowColour, &m.cfg.CursorRowColour},
	} {
		if c.val == "" {
			continue
		}
		if !validDisplayColour(c.val) {
			m = m.logNote("preferences", "%s %q ignored: expected a 256-colour number (0-255) or #rrggbb; the default stands", c.key, c.val)
			continue
		}
		*c.into = c.val
	}
	if d.RetiredStripePresent && !m.rowStripeNoted {
		m.rowStripeNoted = true
		m = m.logNote("preferences", "display.row_stripe is no longer used: rows are not striped, and the cursor row's colour is display.cursor_row_color")
	}
	return m
}

func validDisplayColour(s string) bool {
	if n, err := strconv.Atoi(s); err == nil {
		return n >= 0 && n <= 255
	}
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}
