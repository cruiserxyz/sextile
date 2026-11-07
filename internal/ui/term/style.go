// Package term holds the terminal-text primitives the ui package draws with:
// cutting and padding a string to a cell width, the room a wide glyph needs,
// the SGR codes and the palette. It knows nothing about charts, slots or the
// model; what it needs from the launch configuration is the Style interface.
package term

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Style is what the primitives need to know about the terminal they draw for.
// ui.Config satisfies it.
type Style interface {
	UseColor() bool
	UseASCII() bool
	// FocusColour and CursorRowColour are the two colours a launch may choose,
	// resolved against the defaults below.
	FocusColour() lipgloss.Color
	CursorRowColour() lipgloss.Color
}

// CursorBandStart opens the cursor row with colours on: bold over the
// cursor-row background, and no foreground, so every cell keeps its own.
// lipgloss writes the sequence, so it follows the terminal's colour profile.
func CursorBandStart(cfg Style) string {
	const probe = "X"
	r := lipgloss.NewStyle().Bold(true).Background(cfg.CursorRowColour()).Render(probe)
	if i := strings.Index(r, probe); i > 0 {
		return r[:i]
	}
	return SGRBold
}
