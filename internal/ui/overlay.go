package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// overlay places fg on top of bg at position (x, y), measured in visible cells.
// The bg lines outside the overlay rectangle show through unchanged.
// ANSI-aware: styling state is closed at each overlay boundary so bg colours
// downstream of the cut are not clobbered.
func overlay(bg, fg string, x, y int) string {
	if fg == "" {
		return bg
	}

	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	fgWidth := 0
	for _, line := range fgLines {
		if w := ansi.StringWidth(line); w > fgWidth {
			fgWidth = w
		}
	}

	out := make([]string, len(bgLines))
	copy(out, bgLines)

	for i, fgLine := range fgLines {
		row := y + i
		if row < 0 || row >= len(bgLines) {
			continue
		}
		bgLine := bgLines[row]

		left := blankGlyphAtEnd(ansi.Truncate(bgLine, x, ""))
		right := ansi.TruncateLeft(bgLine, x+fgWidth, "")

		// Pad fg line out to fgWidth so short lines don't let bg bleed through.
		if w := ansi.StringWidth(fgLine); w < fgWidth {
			fgLine += strings.Repeat(" ", fgWidth-w)
		}

		// Reset SGR at the boundary so bg cells downstream recover their colour.
		// Skip the reset when neither fg nor the underlying bg line carries
		// ANSI state — the marker just clutters accessible-mode goldens.
		sep := sgrReset
		if !strings.ContainsRune(fgLine, escapeByte) && !strings.ContainsRune(bgLine, escapeByte) {
			sep = ""
		}
		out[row] = left + fgLine + sep + right
	}

	return strings.Join(out, "\n")
}

// placeCentered overlays fg at the centre of a bg of given width/height.
func placeCentered(bg, fg string, bgWidth, bgHeight int) string {
	fgWidth := 0
	fgHeight := 0
	for _, line := range strings.Split(fg, "\n") {
		if w := ansi.StringWidth(line); w > fgWidth {
			fgWidth = w
		}
		fgHeight++
	}

	x := (bgWidth - fgWidth) / 2
	y := (bgHeight - fgHeight) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	return overlay(bg, fg, x, y)
}
