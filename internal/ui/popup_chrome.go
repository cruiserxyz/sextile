package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
)

// popupChrome is what the popups' box borrows from the panels: the top edge
// that carries the popup's title, the ASCII border and the faint rendering. The
// box itself lives in dialog and knows none of them.
func popupChrome(cfg Config) dialog.Chrome {
	return dialog.Chrome{
		TopEdge: func(width int, title string, frame lipgloss.Color, leftCorner, rightCorner, dash string) string {
			return renderTopBorder(width, tabStrip{tabs: []Tab{{Title: title}}}, frame, cfg.FocusColour(), true, cfg, leftCorner, rightCorner, dash)
		},
		ASCIIBorder: asciiBorder,
		Dim:         func(text string) string { return panel.RenderDim(text, true) },
	}
}

// popupCounter is a list's `N of M` for the box: the widest text it can read
// (the box is never narrower than that) and the call that writes it into the
// bottom edge.
//
// It takes the position by pointer because a caller may still change it
// (the City list puts a cut name in Text) after the box is described and
// before the box is drawn.
func popupCounter(p *listPosition, cfg Config) *dialog.Counter {
	return &dialog.Counter{
		Widest: func() string {
			widest := listPosition{Cursor: max(p.Total, 1), Total: max(p.Total, 1)}
			return widest.text()
		},
		Draw: func(box string) string { return withPositionCounter(box, *p, cfg, true) },
	}
}
