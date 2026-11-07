// Package pane draws the chrome of a scrolling region: the viewport arithmetic,
// the scroll thumb on a box's right edge, a horizontal rule across it, and the
// border colour a focused or unfocused pane takes. It knows nothing of charts
// or the model; the terminal is described by term.Style.
package pane

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// ThumbGlyph is the V1 taste pick — a heavy vertical bar, so the thumb
// reads as the side border thickening rather than as content, and stays
// distinguishable from the light `│` border with no colour at all.
var ThumbGlyph = "┃"

// RuleGlyph is the V2 taste pick for the Bodies rule: a dotted run, so
// the pinned angle block is separated without a second solid border line.
var RuleGlyph = "┈"

// ASCII substitutes for both, used whenever the frame itself is ASCII.
const (
	ThumbASCII   = "#"
	RuleASCII    = "-"
	RuleTeeASCII = "+"
)

// Unicode tees joining the rule to the two side borders.
const (
	panelRuleLeftTee  = "├"
	panelRuleRightTee = "┤"
)

// Viewport is one panel's scroll arithmetic: how many content lines exist,
// how many the panel body can show, and where the shown window starts. Pure
// data — it renders nothing and knows nothing about the Model.
type Viewport struct {
	Total   int
	Visible int
	Offset  int
}

// Overflows reports whether content is hidden AND the track has room to say
// so. A one-row body could only ever draw a full-length thumb, which reads as
// "nothing is hidden" — so it draws nothing instead.
func (v Viewport) Overflows() bool {
	return v.Total > v.Visible && v.Visible > 1
}

// Thumb returns the indicator's first row (counted from the first body row)
// and its length in rows. The Thumb's share of the track is the content's
// visible fraction, so the LESS there is to scroll the LONGER the Thumb, down
// to a single row for a report many screens long. One track row always stays
// empty, so a Thumb can never fill the track it is measuring. Returns (0, 0)
// when nothing overflows.
func (v Viewport) Thumb() (top, size int) {
	if !v.Overflows() {
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
	top = int(math.Round(float64(ClampOffset(v.Offset, v.Total, v.Visible)) * float64(v.Visible-size) / float64(span)))
	if top < 0 {
		top = 0
	}
	if limit := v.Visible - size; top > limit {
		top = limit
	}
	return top, size
}

// ClampOffset holds a scroll offset inside [0, Total-Visible]. Content that
// fits pins the offset at 0.
func ClampOffset(offset, total, visible int) int {
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

// BorderColour is the colour a panel paints its border in. The scroll
// indicator and the panel rule reuse it verbatim, so both brighten with focus
// exactly as the border does and can never introduce a colour of their own.
func BorderColour(focused bool, cfg term.Style) lipgloss.Color {
	if focused {
		return cfg.FocusColour()
	}
	return term.FrameColour
}

// WithScrollIndicator overlays the thumb onto an already-rendered panel box:
// it replaces the right border glyph on the thumb's rows and touches nothing
// else. A viewport that does not overflow returns the box byte for byte, so a
// panel with nothing hidden renders exactly as it did before this existed.
func WithScrollIndicator(box string, v Viewport, cfg term.Style, focused bool) string {
	return WithScrollIndicatorAt(box, v, cfg, focused, 1)
}

// WithScrollIndicatorAt is withScrollIndicator for a track that starts on
// frame row firstRow: a popup's list sits below its padding and search line.
func WithScrollIndicatorAt(box string, v Viewport, cfg term.Style, focused bool, firstRow int) string {
	if box == "" || !v.Overflows() {
		return box
	}
	top, size := v.Thumb()
	if size <= 0 {
		return box
	}
	width := lipgloss.Width(box)
	if width < 1 {
		return box
	}
	glyph := ThumbGlyph
	if cfg.UseASCII() {
		glyph = ThumbASCII
	}
	if cfg.UseColor() {
		glyph = lipgloss.NewStyle().Foreground(BorderColour(focused, cfg)).Render(glyph)
	}
	rows := make([]string, size)
	for i := range rows {
		rows[i] = glyph
	}
	// x = width-1 is the right border column; y = firstRow+top skips the
	// title row and anything above the track.
	return term.Composite(box, strings.Join(rows, "\n"), width-1, firstRow+top)
}

// WithPanelRule overlays a horizontal rule joining both side borders on body
// row `row`, so the rows above it (which scroll) are visibly separate from the
// pinned block below. A negative row means the panel has no pinned block and
// the box is returned untouched.
func WithPanelRule(box string, row int, cfg term.Style, focused bool) string {
	if box == "" || row < 0 {
		return box
	}
	width := lipgloss.Width(box)
	if width < 2 {
		return box
	}
	dash, left, right := RuleGlyph, panelRuleLeftTee, panelRuleRightTee
	if cfg.UseASCII() {
		dash, left, right = RuleASCII, RuleTeeASCII, RuleTeeASCII
	}
	rule := left + strings.Repeat(dash, width-2) + right
	if cfg.UseColor() {
		rule = lipgloss.NewStyle().Foreground(BorderColour(focused, cfg)).Render(rule)
	}
	return term.Composite(box, rule, 0, 1+row)
}
