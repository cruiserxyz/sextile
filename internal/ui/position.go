package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// position.go draws where you are in a list: `3 of 14` in the bottom-right of
// the list's frame, as lazygit does (the specification C6–C10).
//
// It is kept apart from viewport on purpose. viewport is scroll geometry —
// what is shown of what exists — and knows nothing about a cursor; a counter
// built from the window's offset would read "1 of 40" while the cursor sat on
// row 12.

// listPosition is a list's cursor and the number of positions the cursor can
// take: the count the up/down keys walk. A list with none reads `0 of 0`.
type listPosition struct {
	Cursor int
	Total  int

	// Text, when set, is drawn in the counter's place: the full name of the
	// row under the cursor when the row had to cut it (the City list). It is
	// the caller's to fit to the edge.
	Text string
}

// text is the counter as drawn.
func (p listPosition) text() string {
	if p.Text != "" {
		return p.Text
	}
	if p.Total <= 0 {
		return "0 of 0"
	}
	n := min(max(p.Cursor, 0), p.Total-1) + 1
	return fmt.Sprintf("%d of %d", n, p.Total)
}

// withPositionCounter writes the counter into the box's bottom edge, right
// aligned with one border dash before the corner, as a title sits one dash in
// from the top-left corner. keyboard is whether the list's owner has the
// keyboard: the counter is then in the focus colour and bold, and otherwise in
// the frame colour. A box whose edge cannot hold the whole counter keeps its
// edge as it was (C10): a shortened count would be a wrong count.
func withPositionCounter(box string, p listPosition, cfg Config, keyboard bool) string {
	if box == "" {
		return box
	}
	lines := strings.Split(box, "\n")
	edge, ok := bottomEdgeWithCounter(lipgloss.Width(lines[len(lines)-1]), p, cfg, keyboard)
	if !ok {
		return box
	}
	lines[len(lines)-1] = edge
	return strings.Join(lines, "\n")
}

// bottomEdgeWithCounter is a frame's bottom edge, width cells wide, carrying
// the counter. It reports false when the counter does not fit with one dash
// on each side of it.
func bottomEdgeWithCounter(width int, p listPosition, cfg Config, keyboard bool) (string, bool) {
	text := p.text()
	tw := ansi.StringWidth(text)
	if width < tw+4 {
		return "", false
	}
	left, right, dash := "╰", "╯", "─"
	if cfg.useASCII() {
		left, right, dash = "+", "+", "-"
	}
	border := lipgloss.NewStyle()
	counter := lipgloss.NewStyle()
	if cfg.useColor() {
		border = border.Foreground(panelBorderColor(keyboard, cfg))
		counter = counter.Foreground(panelBorderColor(false, cfg))
		if keyboard {
			counter = lipgloss.NewStyle().Foreground(focusColourFor(cfg)).Bold(true)
		}
	}
	return border.Render(left+strings.Repeat(dash, width-3-tw)) +
		counter.Render(text) +
		border.Render(dash+right), true
}
