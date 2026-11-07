package menu

import (
	"github.com/cruiserxyz/sextile/internal/ui/term"
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
// marker (RowCursorBlurred). The timeline's nearest event is drawn the same
// way. Selection never adds bold. Once
// a panel contains any selection, every row reserves one marker column so
// toggling a row never changes table geometry, and a selected row's marker is
// in the focus colour.
const (
	RowCursorUnicode    = "▶ "
	RowCursorASCII      = "> "
	RowSelectionUnicode = "●"
	RowSelectionASCII   = "*"
	RowMarkerBlank      = "  "
)

// RowPrefix is the row's marker columns. The cursor column exists only with
// colours off, and only in the list that owns the keyboard: with colours on
// the band alone marks the cursor row.
func RowPrefix(panelFocused, cursorRow, selected, selectionColumn bool, cfg term.Style) string {
	prefix := ""
	if !cfg.UseColor() && panelFocused {
		prefix += CursorColumn(cursorRow, cfg)
	}
	if selectionColumn {
		prefix += SelectionMarker(selected, cfg)
	}
	return prefix
}

// ChartRowPrefix is RowPrefix for the Chart panel, where a selected row's
// marker is its numeral ①–⑥ (`1`–`6` in ASCII mode) in place of the selection
// ●, spelled by the caller as mark; an unselected row's (empty mark) cell stays blank (selected-charts-compare C14).
// Bodies, the timeline and the console keep RowPrefix and its ●.
func ChartRowPrefix(panelFocused, cursorRow bool, mark string, selectionColumn bool, cfg term.Style) string {
	prefix := ""
	if !cfg.UseColor() && panelFocused {
		prefix += CursorColumn(cursorRow, cfg)
	}
	if selectionColumn {
		prefix += numeralMarker(mark, cfg)
	}
	return prefix
}

// numeralMarker is a selected Chart row's numeral marker, in the focus colour
// while colours are on like the ● it replaces, or the blank that keeps the
// column for an unselected row (an empty mark). The caller spells the numeral
// for the glyph mode; the marker only colours it.
func numeralMarker(mark string, cfg term.Style) string {
	if mark == "" {
		return RowMarkerBlank
	}
	if cfg.UseColor() {
		mark = lipgloss.NewStyle().Foreground(cfg.FocusColour()).Render(mark)
	}
	return mark + " "
}

// CursorColumn is the cursor marker, or the blank that keeps the column.
func CursorColumn(cursorRow bool, cfg term.Style) string {
	if !cursorRow {
		return RowMarkerBlank
	}
	if cfg.UseASCII() {
		return RowCursorASCII
	}
	return RowCursorUnicode
}

// SelectionMarker is a selected row's marker, in the focus colour while
// colours are on, or the blank that keeps the column.
func SelectionMarker(selected bool, cfg term.Style) string {
	if !selected {
		return RowMarkerBlank
	}
	mark := RowSelectionUnicode
	if cfg.UseASCII() {
		mark = RowSelectionASCII
	}
	if cfg.UseColor() {
		mark = lipgloss.NewStyle().Foreground(cfg.FocusColour()).Render(mark)
	}
	return mark + " "
}

// RowState is how one row is drawn: plain, the cursor row of a panel that
// does not own the keyboard (bold only), or the cursor row of the panel that
// does (bold plus band, or reverse and marker with colours off).
type RowState int

const (
	RowPlain RowState = iota
	RowCursorBlurred
	rowCursorFocused
)

// CursorRowState is the state of a panel's cursor row, or of any other row.
func CursorRowState(panelFocused, cursorRow bool) RowState {
	switch {
	case !cursorRow:
		return RowPlain
	case panelFocused:
		return rowCursorFocused
	}
	return RowCursorBlurred
}

// RenderRow draws a row across exactly width cells in the given state.
func RenderRow(row string, width int, state RowState, cfg term.Style) string {
	switch state {
	case rowCursorFocused:
		return RenderRowState(row, width, true, cfg)
	case RowCursorBlurred:
		if width <= 0 {
			return row
		}
		line := term.FitLine(row, width, cfg)
		return term.SGRBold + ReapplyAfterSGR(line, term.SGRBold) + term.SGRReset
	}
	return row
}

// RenderRowState draws the keyboard-owning cursor row across exactly width cells. The band
// sets only bold and a background (or, with colours off, bold and reverse), so
// Astrolog's foregrounds stay as they are; it is re-applied after every SGR
// sequence inside the row, so a reset that ends an Astrolog span, or a
// background Astrolog sets itself, never breaks it. Any other row is returned
// untouched: selection is the marker, never a treatment of the row.
func RenderRowState(row string, width int, active bool, cfg term.Style) string {
	if !active || width <= 0 {
		return row
	}
	start := term.CursorRowOff
	if cfg.UseColor() {
		start = term.CursorBandStart(cfg)
	}
	line := term.FitLine(row, width, cfg)
	return start + ReapplyAfterSGR(line, start) + term.SGRReset
}

// ReapplyAfterSGR writes start again after every SGR sequence in s.
func ReapplyAfterSGR(s, start string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == term.EscapeByte && i+1 < len(s) && s[i+1] == '[' {
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
