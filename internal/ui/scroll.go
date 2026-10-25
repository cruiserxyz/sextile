package ui

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// detailsScrollStep is how far one PgUp/PgDn press or one wheel notch moves
// the Details report: two lines, mirroring lazygit's `scrollHeight`.
const detailsScrollStep = 2

// scrollThumbGlyph is the V1 taste pick — a heavy vertical bar, so the thumb
// reads as the side border thickening rather than as content, and stays
// distinguishable from the light `│` border with no colour at all.
var scrollThumbGlyph = "┃"

// panelRuleGlyph is the V2 taste pick for the Bodies rule: a dotted run, so
// the pinned angle block is separated without a second solid border line.
var panelRuleGlyph = "┈"

// ASCII substitutes for both, used whenever the frame itself is ASCII.
const (
	scrollThumbASCII  = "#"
	panelRuleASCII    = "-"
	panelRuleTeeASCII = "+"
)

// Unicode tees joining the rule to the two side borders.
const (
	panelRuleLeftTee  = "├"
	panelRuleRightTee = "┤"
)

// viewport is one panel's scroll arithmetic: how many content lines exist,
// how many the panel body can show, and where the shown window starts. Pure
// data — it renders nothing and knows nothing about the Model.
type viewport struct {
	Total   int
	Visible int
	Offset  int
}

// overflows reports whether content is hidden AND the track has room to say
// so. A one-row body could only ever draw a full-length thumb, which reads as
// "nothing is hidden" — so it draws nothing instead.
func (v viewport) overflows() bool {
	return v.Total > v.Visible && v.Visible > 1
}

// thumb returns the indicator's first row (counted from the first body row)
// and its length in rows. The thumb's share of the track is the content's
// visible fraction, so the LESS there is to scroll the LONGER the thumb, down
// to a single row for a report many screens long. One track row always stays
// empty, so a thumb can never fill the track it is measuring. Returns (0, 0)
// when nothing overflows.
func (v viewport) thumb() (top, size int) {
	if !v.overflows() {
		return 0, 0
	}
	size = int(math.Round(float64(v.Visible) * float64(v.Visible) / float64(v.Total)))
	if size < 1 {
		size = 1
	}
	if limit := v.Visible - 1; size > limit {
		size = limit
	}
	span := v.Total - v.Visible // > 0 by overflows(), so this cannot divide by zero
	top = int(math.Round(float64(clampOffset(v.Offset, v.Total, v.Visible)) * float64(v.Visible-size) / float64(span)))
	if top < 0 {
		top = 0
	}
	if limit := v.Visible - size; top > limit {
		top = limit
	}
	return top, size
}

// clampOffset holds a scroll offset inside [0, Total-Visible]. Content that
// fits pins the offset at 0.
func clampOffset(offset, total, visible int) int {
	if offset < 0 {
		return 0
	}
	limit := total - visible
	if limit < 0 {
		limit = 0
	}
	if offset > limit {
		return limit
	}
	return offset
}

// panelBorderColor is the colour a panel paints its border in. The scroll
// indicator and the panel rule reuse it verbatim, so both brighten with focus
// exactly as the border does and can never introduce a colour of their own.
func panelBorderColor(focused bool, cfg Config) lipgloss.Color {
	if focused {
		return focusColourFor(cfg)
	}
	return frameColour
}

// withScrollIndicator overlays the thumb onto an already-rendered panel box:
// it replaces the right border glyph on the thumb's rows and touches nothing
// else. A viewport that does not overflow returns the box byte for byte, so a
// panel with nothing hidden renders exactly as it did before this existed.
func withScrollIndicator(box string, v viewport, cfg Config, focused bool) string {
	return withScrollIndicatorAt(box, v, cfg, focused, 1)
}

// withScrollIndicatorAt is withScrollIndicator for a track that starts on
// frame row firstRow: a popup's list sits below its padding and search line.
func withScrollIndicatorAt(box string, v viewport, cfg Config, focused bool, firstRow int) string {
	if box == "" || !v.overflows() {
		return box
	}
	top, size := v.thumb()
	if size <= 0 {
		return box
	}
	width := lipgloss.Width(box)
	if width < 1 {
		return box
	}
	glyph := scrollThumbGlyph
	if cfg.useASCII() {
		glyph = scrollThumbASCII
	}
	if cfg.useColor() {
		glyph = lipgloss.NewStyle().Foreground(panelBorderColor(focused, cfg)).Render(glyph)
	}
	rows := make([]string, size)
	for i := range rows {
		rows[i] = glyph
	}
	// x = width-1 is the right border column; y = firstRow+top skips the
	// title row and anything above the track.
	return overlay(box, strings.Join(rows, "\n"), width-1, firstRow+top)
}

// withPanelRule overlays a horizontal rule joining both side borders on body
// row `row`, so the rows above it (which scroll) are visibly separate from the
// pinned block below. A negative row means the panel has no pinned block and
// the box is returned untouched.
func withPanelRule(box string, row int, cfg Config, focused bool) string {
	if box == "" || row < 0 {
		return box
	}
	width := lipgloss.Width(box)
	if width < 2 {
		return box
	}
	dash, left, right := panelRuleGlyph, panelRuleLeftTee, panelRuleRightTee
	if cfg.useASCII() {
		dash, left, right = panelRuleASCII, panelRuleTeeASCII, panelRuleTeeASCII
	}
	rule := left + strings.Repeat(dash, width-2) + right
	if cfg.useColor() {
		rule = lipgloss.NewStyle().Foreground(panelBorderColor(focused, cfg)).Render(rule)
	}
	return overlay(box, rule, 0, 1+row)
}
