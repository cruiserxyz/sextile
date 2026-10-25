package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Row interaction keeps focus and selection independent
// (the specification), amended by the specification.
// A panel's cursor row is drawn bold in every panel that has a row cursor. The
// panel that owns the keyboard also marks it: with colours on the cursor-row
// background across its full width, every cell keeping its own colour; with
// colours off the cursor marker and reverse video. A panel that does not own
// the keyboard keeps the bold and nothing else: no band, no reverse, no
// marker (rowCursorBlurred). The timeline's nearest event is drawn the same
// way. Selection never adds bold. Once
// a panel contains any selection, every row reserves one marker column so
// toggling a row never changes table geometry, and a selected row's marker is
// in the focus colour.
const (
	rowCursorUnicode    = "▶ "
	rowCursorASCII      = "> "
	rowSelectionUnicode = "●"
	rowSelectionASCII   = "*"
	rowMarkerBlank      = "  "
)

// rowPrefix is the row's marker columns. The cursor column exists only with
// colours off, and only in the list that owns the keyboard: with colours on
// the band alone marks the cursor row.
func rowPrefix(panelFocused, cursorRow, selected, selectionColumn bool, cfg Config) string {
	prefix := ""
	if !cfg.useColor() && panelFocused {
		prefix += cursorColumn(cursorRow, cfg)
	}
	if selectionColumn {
		prefix += selectionMarker(selected, cfg)
	}
	return prefix
}

// chartRowPrefix is rowPrefix for the Chart panel, where a selected row's
// marker is its numeral ①–⑥ (`1`–`6` in ASCII mode) in place of the selection
// ●; an unselected row's marker cell stays blank (selected-charts-compare C14).
// Bodies, the timeline and the console keep rowPrefix and its ●.
func chartRowPrefix(panelFocused, cursorRow bool, numeral int, selectionColumn bool, cfg Config) string {
	prefix := ""
	if !cfg.useColor() && panelFocused {
		prefix += cursorColumn(cursorRow, cfg)
	}
	if selectionColumn {
		prefix += numeralMarker(numeral, cfg)
	}
	return prefix
}

// numeralMarker is a selected Chart row's numeral marker, in the focus colour
// while colours are on like the ● it replaces, or the blank that keeps the
// column for an unselected row (numeral 0).
func numeralMarker(numeral int, cfg Config) string {
	if numeral < 1 {
		return rowMarkerBlank
	}
	mark := numeralForGlyphs(numeral, cfg)
	if cfg.useColor() {
		mark = lipgloss.NewStyle().Foreground(focusColourFor(cfg)).Render(mark)
	}
	return mark + " "
}

// cursorColumn is the cursor marker, or the blank that keeps the column.
func cursorColumn(cursorRow bool, cfg Config) string {
	if !cursorRow {
		return rowMarkerBlank
	}
	if cfg.useASCII() {
		return rowCursorASCII
	}
	return rowCursorUnicode
}

// selectionMarker is a selected row's marker, in the focus colour while
// colours are on, or the blank that keeps the column.
func selectionMarker(selected bool, cfg Config) string {
	if !selected {
		return rowMarkerBlank
	}
	mark := rowSelectionUnicode
	if cfg.useASCII() {
		mark = rowSelectionASCII
	}
	if cfg.useColor() {
		mark = lipgloss.NewStyle().Foreground(focusColourFor(cfg)).Render(mark)
	}
	return mark + " "
}

// rowState is how one row is drawn: plain, the cursor row of a panel that
// does not own the keyboard (bold only), or the cursor row of the panel that
// does (bold plus band, or reverse and marker with colours off).
type rowState int

const (
	rowPlain rowState = iota
	rowCursorBlurred
	rowCursorFocused
)

// cursorRowState is the state of a panel's cursor row, or of any other row.
func cursorRowState(panelFocused, cursorRow bool) rowState {
	switch {
	case !cursorRow:
		return rowPlain
	case panelFocused:
		return rowCursorFocused
	}
	return rowCursorBlurred
}

// renderRow draws a row across exactly width cells in the given state.
func renderRow(row string, width int, state rowState, cfg Config) string {
	switch state {
	case rowCursorFocused:
		return renderRowState(row, width, true, cfg)
	case rowCursorBlurred:
		if width <= 0 {
			return row
		}
		line := fitLine(row, width, cfg)
		return sgrBold + reapplyAfterSGR(line, sgrBold) + sgrReset
	}
	return row
}

// renderRowState draws the keyboard-owning cursor row across exactly width cells. The band
// sets only bold and a background (or, with colours off, bold and reverse), so
// Astrolog's foregrounds stay as they are; it is re-applied after every SGR
// sequence inside the row, so a reset that ends an Astrolog span, or a
// background Astrolog sets itself, never breaks it. Any other row is returned
// untouched: selection is the marker, never a treatment of the row.
func renderRowState(row string, width int, active bool, cfg Config) string {
	if !active || width <= 0 {
		return row
	}
	start := cursorRowOff
	if cfg.useColor() {
		start = cursorBandStart(cfg)
	}
	line := fitLine(row, width, cfg)
	return start + reapplyAfterSGR(line, start) + sgrReset
}

// reapplyAfterSGR writes start again after every SGR sequence in s.
func reapplyAfterSGR(s, start string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == escapeByte && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j < len(s) && s[j] == 'm' {
				b.WriteString(s[i : j+1])
				b.WriteString(start)
				i = j + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
