package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// bodiesColumnWidth is one chart's Bodies column: a separating cell, the
// chart's own ℞ cell, and "Sgn DD°MM'" (the specification C1).
const bodiesColumnWidth = 12

// bodiesRowFixedWidth is what a comparing row spends outside its columns: the
// name (11), the trailing cell (1), the speed bar (3) and the house frame (19).
const bodiesRowFixedWidth = 11 + 1 + 3 + 19

// colWindow is the run of whole columns a comparing row draws, and how many
// columns lie hidden on each side of it.
type colWindow struct{ lo, hi, hiddenL, hiddenR int }

func wholeWindow(n int) colWindow { return colWindow{0, n, 0, 0} }

func digitCount(n int) int { return len(fmt.Sprint(n)) }

// hintCost is the cells a hidden-column hint takes: the left one is the arrow
// and the count, the right one the trailing cell, the count and the arrow.
func hintCost(hidden int, right bool) int {
	switch {
	case hidden <= 0:
		return 0
	case right:
		return 2 + digitCount(hidden)
	}
	return 1 + digitCount(hidden)
}

// columnWindow picks which whole columns of n fit in avail cells (the room
// left once the row's fixed cells and prefix are taken). The window starts
// where it was (start) and stays put while the focused column is inside it;
// once the focus passes an edge it moves just far enough to show it. A window
// that ends at the last column then takes back hidden columns on its left
// while they fit. The columns and the hints for what stays hidden always fit;
// when everything fits there is no hint. A focused column alone wider than
// avail is kept; the caller clips it.
func columnWindow(n, focus, avail, start int) (lo, hi, hiddenL, hiddenR int) {
	if n*bodiesColumnWidth <= avail {
		return 0, n, 0, 0
	}
	cost := func(lo, hi int) int {
		return (hi-lo)*bodiesColumnWidth + hintCost(lo, false) + hintCost(n-hi, true)
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
func (w colWindow) leftHint(cfg Config) string {
	if w.hiddenL == 0 {
		return ""
	}
	arrow := "‹"
	if cfg.useASCII() {
		arrow = "<"
	}
	return hintDress(arrow+fmt.Sprint(w.hiddenL), cfg)
}

// rightHint is the trailing cell, with the hint in it: the frame's closing
// bracket stays when colours are off and the focus is the last column shown.
func (w colWindow) rightHint(trailing string, cfg Config) string {
	if w.hiddenR == 0 {
		return trailing
	}
	arrow := "›"
	if cfg.useASCII() {
		arrow = ">"
	}
	return trailing + hintDress(fmt.Sprint(w.hiddenR)+arrow, cfg)
}

func hintDress(s string, cfg Config) string {
	if cfg.useColor() {
		return sgrFaint + s + sgrReset
	}
	return s
}

// bodiesView is what the Bodies panel and the layout read from the scene:
// how many charts it shows, and — while comparing — the columns computed for
// exactly that shown set.
type bodiesView struct {
	// shown is len(scene.shown); 0 when there is no scene yet.
	shown int
	// comparing is the scene's mode; ok says cols were computed for this
	// scene's shown set (bodyColumnsForScene).
	comparing bool
	ok        bool
	cols      []bodyColumn
	// focus is the cursor's column index.
	focus int
	// colStart is the first column the window held last (Model.bodiesColStart).
	colStart int
	// held says the view is the previous scene's, kept until its frame lands
	// (shown.go): slot and cursor are then the slot index and Bodies row that
	// frame was cast with, not the live ones.
	held         bool
	slot, cursor int
	// top is the display row the list was last drawn from (Model.bodyTop).
	top int
}

func (v bodiesView) count() int { return max(v.shown, 1) }

func (m Model) bodiesView() bodiesView {
	sc := m.deriveScene()
	v := bodiesView{shown: len(sc.shown), comparing: sc.mode == sceneComparing, focus: sc.focus, colStart: m.bodiesColStart, top: m.bodyTop}
	if m.holding() {
		v.comparing, v.focus = m.shown.comparing, m.shown.focus
		v.shown = m.shown.count
		v.held, v.slot, v.cursor = true, m.shown.slot, m.shown.bodyCursor
	}
	if v.comparing {
		v.cols, v.ok = m.bodyColumnsForScene()
	}
	return v
}

// shownCount is how many charts the scene shows now (at least one).
func (m Model) shownCount() int {
	if m.holding() {
		// The panels keep their width with their content until the frame lands.
		return max(m.shown.count, 1)
	}
	return max(len(m.deriveScene().shown), 1)
}

// geometry is the layout of the terminal for the scene on screen.
func (m Model) geometry() map[PanelID]panelRect {
	return layoutGeometry(m.width, m.height, chartContentLines(m.slots), m.shownCount())
}

// bodiesColumnRoom is the cells a comparing row has for its columns and hints
// once the row prefix and the fixed cells are taken.
func bodiesColumnRoom(innerWidth int, panelFocused bool, slots *store.Slots, slotIdx int, cfg Config) int {
	prefixW := ansi.StringWidth(rowPrefix(panelFocused, false, false, slots != nil && slots.HasBodySelections(slotIdx), cfg))
	return innerWidth - prefixW - bodiesRowFixedWidth
}

// settleBodiesColStart records the comparing window for the focus's current
// place at the current width, so the next focus move travels from it.
func (m Model) settleBodiesColStart() Model {
	if m.slots == nil {
		return m
	}
	sc := m.deriveScene()
	if sc.mode != sceneComparing {
		return m
	}
	room := bodiesColumnRoom(m.geometry()[PanelBodies].w-2, m.focused == PanelBodies, m.slots, m.cursorIdx, m.cfg)
	m.bodiesColStart, _, _, _ = columnWindow(len(sc.shown), sc.focus, room, m.bodiesColStart)
	return m
}

// renderBodiesFor draws the Bodies body for the scene: today's rows alone, one
// column per shown chart while comparing, and nothing while the comparing
// frame is still on its way (never another scene's columns).
func renderBodiesFor(v bodiesView, state chartViewState, slots *store.Slots, slotIdx, cursorIdx, innerWidth, innerHeight int, panelFocused bool, cfg Config) (string, int, viewport) {
	if !v.comparing {
		return renderBodiesBodyAt(state, slots, slotIdx, cursorIdx, innerWidth, innerHeight, panelFocused, cfg, v.top)
	}
	if !v.ok || len(state.positions) == 0 || innerWidth <= 0 || innerHeight <= 0 {
		return "", -1, viewport{}
	}
	ov := houseOverlay{cusps: state.natalCusps, positions: state.natalPositions, framed: state.framedPositions, stations: state.stations, forced: state.forced, form: state.houseForm}
	var win colWindow
	win.lo, win.hi, win.hiddenL, win.hiddenR = columnWindow(len(v.cols), v.focus, bodiesColumnRoom(innerWidth, panelFocused, slots, slotIdx, cfg), v.colStart)
	planets, angles := bodyRowsWith(state.positions, state.cusps, func(p aspect.Position) string {
		return formatColumnRow(p, state.bodyStyles[p.Body], v.cols, v.focus, win, cfg, ov)
	}, func(house int, _ float64) string {
		return formatCuspColumnRow(house, state.bodyStyles[aspect.Ascendant], v.cols, v.focus, win, cfg, ov.form)
	})
	allCount := len(planets) + len(angles)
	if cursorIdx < 0 {
		cursorIdx = 0
	}
	if cursorIdx >= allCount {
		cursorIdx = allCount - 1
	}
	return layoutBodyRows(planets, angles, slots, slotIdx, cursorIdx, innerWidth, innerHeight, panelFocused, cfg, state.mark, v.top)
}

// formatColumnRow is one comparing row: the body, one column per shown chart,
// then the cursor chart's speed bar and house frame as the alone row has them.
// Columns are matched to the row's body by identity, never by index.
func formatColumnRow(p aspect.Position, styles astrologstyle.BodyRow, cols []bodyColumn, focus int, win colWindow, cfg Config, ov houseOverlay) string {
	useColor := cfg.useColor()
	bodyPart := fmt.Sprintf("%s %-9s", positionGlyph(p), p.DisplayName(bodyName(p.Body)))
	var b strings.Builder
	if useColor {
		b.WriteString(renderSGR(styles.Body, bodyPart))
	} else {
		b.WriteString(bodyPart)
	}
	b.WriteString(win.leftHint(cfg))
	for i := win.lo; i < win.hi; i++ {
		b.WriteString(columnCell(cols[i], p.Body, i, focus, styles.Position, cfg))
	}
	// The trailing cell is reserved even with colours on, so the closing
	// bracket never moves anything.
	if !useColor && focus == win.hi-1 {
		b.WriteString(win.rightHint("]", cfg))
	} else {
		b.WriteString(win.rightHint(" ", cfg))
	}
	// The ℞ now sits in each column; the speed bar is its three cells.
	if isAngle(p.Body) {
		b.WriteString("   ")
	} else {
		b.WriteString(dropFirstCell(speedField(p, ov, false, useColor)))
	}
	if ov.active() {
		natalHouse, liveHouse := ov.houses(p)
		b.WriteString(houseColumn(ov.form, natalHouse, liveHouse, true, styles.House, useColor))
	} else {
		b.WriteString(houseColumn(ov.form, p.House, 0, false, styles.House, useColor))
	}
	return b.String()
}

// formatCuspColumnRow is a plain cusp's comparing row: it reads like an angle
// (the specification) — the house number and ordinal, each shown
// chart's own cusp for that house in its column, then the focused chart's
// house frame with the cusp's own number in its quadrant. No speed bar. The
// row's own longitude is the focused chart's and is not drawn here: every
// column shows its chart's cusp.
func formatCuspColumnRow(house int, styles astrologstyle.BodyRow, cols []bodyColumn, focus int, win colWindow, cfg Config, form houseForm) string {
	useColor := cfg.useColor()
	label := cuspLabel(house)
	var b strings.Builder
	if useColor {
		b.WriteString(renderSGR(styles.Body, label))
	} else {
		b.WriteString(label)
	}
	b.WriteString(win.leftHint(cfg))
	for i := win.lo; i < win.hi; i++ {
		c := cols[i]
		text := strings.Repeat(" ", bodiesColumnWidth-1)
		if c.err == nil && c.cusps.Valid() {
			signIdx, deg, min := splitLongitude(c.cusps[house-1])
			text = fmt.Sprintf("%s %2d°%02d' ", signAbbrevString(signIdx), deg, min)
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

// dropFirstCell removes a velocity field's ℞/space cell, which never carries
// an escape sequence.
func dropFirstCell(vel string) string {
	_, n := utf8.DecodeRuneInString(vel)
	return vel[n:]
}

// columnCell is one chart's 12 cells for body b. Colours on: the focused
// column in Astrolog's own position colour, exactly as the alone row draws its
// position (operator, 2026-10-05: "reenable astrolog colors for the active
// row"), the rest faint. Colours off: `[` is the focused column's own
// separating cell and `]` the next column's.
func columnCell(c bodyColumn, b aspect.Body, idx, focus int, position astrologstyle.SGR, cfg Config) string {
	body := strings.Repeat(" ", bodiesColumnWidth-1)
	if c.err == nil {
		for _, q := range c.positions {
			if q.Body != b {
				continue
			}
			signIdx, deg, min := splitLongitude(q.Longitude)
			// The ℞ (or a space) is the cell right after the minute mark.
			retro := " "
			if q.Retrograde() {
				retro = "℞"
			}
			body = fmt.Sprintf("%s %2d°%02d'%s", signAbbrevString(signIdx), deg, min, retro)
			break
		}
	}
	return framedCell(body, idx, focus, position, cfg)
}

// framedCell puts one column's 11 cells of text behind its separating cell and
// dresses it for the focus (see columnCell).
func framedCell(body string, idx, focus int, position astrologstyle.SGR, cfg Config) string {
	sep := " "
	if !cfg.useColor() {
		switch idx {
		case focus:
			sep = "["
		case focus + 1:
			sep = "]"
		}
	}
	if !cfg.useColor() {
		return sep + body
	}
	if idx == focus {
		return sep + renderSGR(position, body)
	}
	return sgrFaint + sep + body + sgrReset
}
