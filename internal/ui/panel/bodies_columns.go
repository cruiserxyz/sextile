package panel

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"github.com/cruiserxyz/sextile/internal/ui/term"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// BodiesColumnWidth is one chart's Bodies column: a separating cell, the
// chart's own ℞ cell, and "Sgn DD°MM'" (the specification C1).
const BodiesColumnWidth = 12

// BodiesRowFixedWidth is what a comparing row spends outside its columns: the
// name (11), the trailing cell (1), the speed bar (3) and the house frame (19).
const BodiesRowFixedWidth = 11 + 1 + 3 + 19

// colWindow is the run of whole columns a comparing row draws, and how many
// columns lie hidden on each side of it.
type colWindow struct{ lo, hi, hiddenL, hiddenR int }

func WholeWindow(n int) colWindow { return colWindow{0, n, 0, 0} }

func digitCount(n int) int { return len(fmt.Sprint(n)) }

// HintCost is the cells a hidden-column hint takes: the left one is the arrow
// and the count, the right one the trailing cell, the count and the arrow.
func HintCost(hidden int, right bool) int {
	switch {
	case hidden <= 0:
		return 0
	case right:
		return 2 + digitCount(hidden)
	}
	return 1 + digitCount(hidden)
}

// ColumnWindow picks which whole columns of n fit in avail cells (the room
// left once the row's fixed cells and prefix are taken). The window starts
// where it was (start) and stays put while the focused column is inside it;
// once the focus passes an edge it moves just far enough to show it. A window
// that ends at the last column then takes back hidden columns on its left
// while they fit. The columns and the hints for what stays hidden always fit;
// when everything fits there is no hint. A focused column alone wider than
// avail is kept; the caller clips it.
func ColumnWindow(n, focus, avail, start int) (lo, hi, hiddenL, hiddenR int) {
	if n*BodiesColumnWidth <= avail {
		return 0, n, 0, 0
	}
	cost := func(lo, hi int) int {
		return (hi-lo)*BodiesColumnWidth + HintCost(lo, false) + HintCost(n-hi, true)
	}
	reach := func(lo int) int {
		hi := lo + 1
		for hi < n && cost(lo, hi+1) <= avail {
			hi++
		}
		return hi
	}
	lo = min(max(start, 0), focus)
	hi = reach(lo)
	for focus >= hi {
		lo++
		hi = reach(lo)
	}
	if hi == n {
		for lo > 0 && cost(lo-1, hi) <= avail {
			lo--
		}
	}
	return lo, hi, lo, n - hi
}

// leftHint is a hidden-column hint's arrow and count.
func (w colWindow) leftHint(cfg term.Style) string {
	if w.hiddenL == 0 {
		return ""
	}
	arrow := "‹"
	if cfg.UseASCII() {
		arrow = "<"
	}
	return hintDress(arrow+fmt.Sprint(w.hiddenL), cfg)
}

// rightHint is the trailing cell, with the hint in it: the frame's closing
// bracket stays when colours are off and the focus is the last column shown.
func (w colWindow) rightHint(trailing string, cfg term.Style) string {
	if w.hiddenR == 0 {
		return trailing
	}
	arrow := "›"
	if cfg.UseASCII() {
		arrow = ">"
	}
	return trailing + hintDress(fmt.Sprint(w.hiddenR)+arrow, cfg)
}

func hintDress(s string, cfg term.Style) string {
	if cfg.UseColor() {
		return term.SGRFaint + s + term.SGRReset
	}
	return s
}

// ColumnRoom is the cells a comparing row has for its columns and hints once
// the row prefix and the fixed cells are taken.
func ColumnRoom(innerWidth int, panelFocused, hasSelections bool, cfg term.Style) int {
	prefixW := ansi.StringWidth(menu.RowPrefix(panelFocused, false, false, hasSelections, cfg))
	return innerWidth - prefixW - BodiesRowFixedWidth
}

// BodiesBag is the resolved input of the Bodies panel: the chart under the
// cursor's facts (its positions, houses and station marks, and on a pair tab the
// base's houses, the only other-chart facts self-check 14 allows) and the
// presentation (cursor, selection marks, column window, focus, size). The Model
// resolves it; RenderBodies turns it into the panel body.
type BodiesBag struct {
	Positions []aspect.Position
	Cusps     aspect.Cusps
	Stations  map[int]lookup.Station
	Forced    map[int]lookup.Station
	// Lit and LitSGR light one row's name and position (HouseOverlay).
	Lit        aspect.Object
	LitSGR     astrologstyle.SGR
	BodyStyles astrologstyle.Bodies
	// NatalPositions, NatalCusps and Framed are the base's houses on a pair tab.
	NatalPositions []aspect.Position
	NatalCusps     aspect.Cusps
	Framed         []aspect.Position
	HouseForm      HouseForm
	// Columns are the comparing frame's columns; ColumnsOK says they were
	// computed for the scene on screen.
	Columns   []BodyColumn
	ColumnsOK bool
	Comparing bool
	// Focus is the cursor's column; ColStart the first column the window held
	// last; Top the display row the list was last drawn from.
	Focus, ColStart, Top int
	// Cursor is the Bodies row the cursor stands on (a held frame's own).
	Cursor int
	// Selected is the slot's selected bodies; HasSelections reserves the
	// marker column.
	Selected      map[int]bool
	HasSelections bool
	// InnerW and InnerH are the body's size; Focused is whether the panel has
	// the focus.
	InnerW, InnerH int
	Focused        bool
	Style          term.Style
}

func (b BodiesBag) overlay() HouseOverlay {
	return HouseOverlay{Cusps: b.NatalCusps, Positions: b.NatalPositions, Framed: b.Framed, Stations: b.Stations, Forced: b.Forced, Lit: b.Lit, LitSGR: b.LitSGR, Form: b.HouseForm}
}

// RenderBodies draws the Bodies body: today's rows alone, one column per shown
// chart while comparing, and nothing while the comparing frame is still on its
// way (never another scene's columns). It returns the body, the inner-row index
// the rule belongs on (-1 for none) and the viewport the scroll indicator
// measures.
func RenderBodies(b BodiesBag) (string, int, pane.Viewport) {
	if len(b.Positions) == 0 || b.InnerW <= 0 || b.InnerH <= 0 {
		return "", -1, pane.Viewport{}
	}
	if !b.Comparing {
		planets, under := bodyRowsFor(b.Positions, b.Cusps, b.BodyStyles, b.Style, b.overlay())
		return layoutBodyRows(planets, under, b)
	}
	if !b.ColumnsOK {
		return "", -1, pane.Viewport{}
	}
	ov := b.overlay()
	cfg := b.Style
	var win colWindow
	win.lo, win.hi, win.hiddenL, win.hiddenR = ColumnWindow(len(b.Columns), b.Focus, ColumnRoom(b.InnerW, b.Focused, b.HasSelections, cfg), b.ColStart)
	planets, angles := bodyRowsWith(b.Positions, b.Cusps, func(p aspect.Position) string {
		return FormatColumnRow(p, b.BodyStyles[p.Object], b.Columns, b.Focus, win, cfg, ov)
	}, func(house int, _ float64, o aspect.Object) string {
		return formatCuspColumnRow(house, cuspStyle(b.BodyStyles, o), b.Columns, b.Focus, win, cfg, ov.Form)
	})
	allCount := len(planets) + len(angles)
	if b.Cursor < 0 {
		b.Cursor = 0
	}
	if b.Cursor >= allCount {
		b.Cursor = allCount - 1
	}
	return layoutBodyRows(planets, angles, b)
}

// FormatColumnRow is one comparing row: the body, one column per shown chart,
// then the cursor chart's speed bar and house frame as the alone row has them.
// Columns are matched to the row's body by identity, never by index.
func FormatColumnRow(p aspect.Position, styles astrologstyle.BodyRow, cols []BodyColumn, focus int, win colWindow, cfg term.Style, ov HouseOverlay) string {
	useColor := cfg.UseColor()
	bodyPart := PositionGlyph(p) + " " + bodyNameCell(p, cfg)
	// The led row of the enter-colour experiment: the name cell and the focused
	// chart's position take the aspect's colour, the glyph and the rest as ever.
	lit := useColor && ov.LitSGR != "" && p.Object == ov.Lit
	position := styles.Position
	var b strings.Builder
	switch {
	case lit:
		b.WriteString(RenderSGR(styles.Body, PositionGlyph(p)+" ") + RenderSGR(ov.LitSGR, bodyNameCell(p, cfg)))
		position = ov.LitSGR
	case useColor:
		b.WriteString(RenderSGR(styles.Body, bodyPart))
	default:
		b.WriteString(bodyPart)
	}
	b.WriteString(win.leftHint(cfg))
	for i := win.lo; i < win.hi; i++ {
		b.WriteString(ColumnCell(cols[i], p, i, focus, position, cfg))
	}
	// The trailing cell is reserved even with colours on, so the closing
	// bracket never moves anything.
	if !useColor && focus == win.hi-1 {
		b.WriteString(win.rightHint("]", cfg))
	} else {
		b.WriteString(win.rightHint(" ", cfg))
	}
	// The ℞ now sits in each column; the speed bar is its three cells.
	if IsAngle(p.Object) {
		b.WriteString("   ")
	} else {
		b.WriteString(DropFirstCell(speedField(p, ov, false, useColor)))
	}
	if ov.Active() {
		natalHouse, liveHouse := ov.Houses(p)
		b.WriteString(HouseColumn(ov.Form, natalHouse, liveHouse, true, styles.House, useColor))
	} else {
		b.WriteString(HouseColumn(ov.Form, p.House, 0, false, styles.House, useColor))
	}
	return b.String()
}

// formatCuspColumnRow is a plain cusp's comparing row: it reads like an angle
// (the specification) — the house number and ordinal, each shown
// chart's own cusp for that house in its column, then the focused chart's
// house frame with the cusp's own number in its quadrant. No speed bar. The
// row's own longitude is the focused chart's and is not drawn here: every
// column shows its chart's cusp.
func formatCuspColumnRow(house int, styles astrologstyle.BodyRow, cols []BodyColumn, focus int, win colWindow, cfg term.Style, form HouseForm) string {
	useColor := cfg.UseColor()
	label := cuspLabel(house)
	var b strings.Builder
	if useColor {
		b.WriteString(RenderSGR(styles.Body, label))
	} else {
		b.WriteString(label)
	}
	b.WriteString(win.leftHint(cfg))
	for i := win.lo; i < win.hi; i++ {
		c := cols[i]
		text := strings.Repeat(" ", BodiesColumnWidth-1)
		if c.Err == nil && c.Cusps.Valid() {
			signIdx, deg, min := SplitLongitude(c.Cusps[house-1])
			text = fmt.Sprintf("%s %2d°%02d' ", SignAbbrevString(signIdx), deg, min)
		}
		b.WriteString(framedCell(text, i, focus, styles.Position, cfg))
	}
	// The trailing cell and the bar's three cells stay blank, as an angle's.
	if !useColor && focus == win.hi-1 {
		b.WriteString(win.rightHint("]", cfg))
	} else {
		b.WriteString(win.rightHint(" ", cfg))
	}
	b.WriteString("   ")
	b.WriteString(cuspFrameTail(house, form, styles.House, useColor))
	return b.String()
}

// DropFirstCell removes a velocity field's ℞/space cell, which never carries
// an escape sequence.
func DropFirstCell(vel string) string {
	_, n := utf8.DecodeRuneInString(vel)
	return vel[n:]
}

// samePosition reports whether p and q are the same object of two charts:
// Astrolog's own number is the identity.
func samePosition(p, q aspect.Position) bool {
	return p.Object.Number == q.Object.Number
}

// ColumnCell is one chart's 12 cells for position p's body. Colours on: the focused
// column in Astrolog's own position colour, exactly as the alone row draws its
// position (operator, 2026-10-05: "reenable astrolog colors for the active
// row"), the rest faint. Colours off: `[` is the focused column's own
// separating cell and `]` the next column's.
func ColumnCell(c BodyColumn, p aspect.Position, idx, focus int, position astrologstyle.SGR, cfg term.Style) string {
	body := strings.Repeat(" ", BodiesColumnWidth-1)
	if c.Err == nil {
		for _, q := range c.Positions {
			if !samePosition(q, p) {
				continue
			}
			signIdx, deg, min := SplitLongitude(q.Longitude)
			// The ℞ (or a space) is the cell right after the minute mark.
			retro := " "
			if q.Retrograde() {
				retro = "℞"
			}
			body = fmt.Sprintf("%s %2d°%02d'%s", SignAbbrevString(signIdx), deg, min, retro)
			break
		}
	}
	return framedCell(body, idx, focus, position, cfg)
}

// framedCell puts one column's 11 cells of text behind its separating cell and
// dresses it for the focus (see columnCell).
func framedCell(body string, idx, focus int, position astrologstyle.SGR, cfg term.Style) string {
	sep := " "
	if !cfg.UseColor() {
		switch idx {
		case focus:
			sep = "["
		case focus + 1:
			sep = "]"
		}
	}
	if !cfg.UseColor() {
		return sep + body
	}
	if idx == focus {
		return sep + RenderSGR(position, body)
	}
	return term.SGRFaint + sep + body + term.SGRReset
}
