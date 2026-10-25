package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// chartViewState carries the parsed astrolog output the bodies + aspects
// panels render. Empty when the startup state is anything but ready.
//
// The panels always show the FOCUSED slot's single chart (positions +
// intra-aspects); overlay mode (S5b) re-skins them: the `overlay` marker adds
// the transit→natal cross waterfall. positions carries the focused slot's
// bodies in BOTH modes; Bodies never adds an inline overlay heading.
type chartViewState struct {
	positions []aspect.Position
	// stations are the focused chart's station marks (D38), committed with
	// positions in one frame; a frame held on screen keeps its own.
	stations map[aspect.Body]lookup.Station
	// forced is the entered station row's word for its body, for THIS paint
	// only (enteredStation); it outranks stations and is drawn deepest red.
	forced map[aspect.Body]lookup.Station
	// cusps is the FOCUSED chart's own house frame, parsed from the lookup
	// that produced positions; Bodies lists its twelve cusps under the rule.
	// The zero value means "no frame": only the angles are listed.
	cusps        aspect.Cusps
	aspects      []aspect.Aspect
	bodyStyles   astrologstyle.Bodies
	aspectStyles astrologstyle.Aspects
	// crossAspects is the transit→natal overlay aspect list (astrolog -r0).
	// Populated only on the overlay branch; rendered by the Aspects panel's
	// Transit→Natal tab. aspects is populated on that branch too (the
	// focused slot's own aspects), so the Aspects tab stays live.
	crossAspects      []lookup.OverlayAspect
	crossAspectStyles astrologstyle.Aspects
	// aspectsReport / crossAspectsReport are the two lists exactly as Astrolog
	// printed them (the -aa and -rt -aa stdout the rows above were parsed
	// from). Details shows the one behind the active Aspects tab.
	aspectsReport      string
	crossAspectsReport string
	// crossSynastry marks crossAspects as the pair of two anchored charts
	// (Astrolog -r0): its applying/separating verdict is a synthetic one and
	// is never displayed.
	crossSynastry bool
	// overlay marks an overlay-mode frame. Explicit (not derived from a
	// populated slice) so an overlay with zero cross-aspects is still an
	// overlay frame. Since an earlier change it is the frame marker only — the first
	// overlay commit after a non-overlay frame activates the cross tab — and
	// no longer a render switch: the Aspects body follows the active tab.
	overlay bool
	// settled is true when the frame on screen is a compute that committed
	// with no error. Only a settled frame's empty list says so; before the
	// first frame and after a failed one the Aspects tabs stay blank
	// (the specification A6).
	settled bool
	// natalPositions and natalCusps describe the ANCHORED chart, populated
	// on the overlay branch. Together they let a moving body's row show
	// where that body sat natally and which of the client's houses it is
	// transiting now. natalCusps is the frame; natalPositions supplies the
	// natal house per body, matched by identity.
	natalPositions []aspect.Position
	natalCusps     aspect.Cusps
	// framedPositions are the moving bodies in the frame's zodiac, when
	// theirs is another (FirstFrameReadyMsg.FramedPositions).
	framedPositions []aspect.Position
	// columns are the comparing frame's Bodies columns, one per chart of the
	// shown set it was computed for (columnsFor); results is the cache of
	// position lookups that lets the next frame recompute only what moved.
	columns    []bodyColumn
	columnsFor columnsFor
	results    resultCache
	// epoch is non-zero once a text commit has filled the chart; a hard reset
	// zeroes it with everything else (shown.go: holding).
	epoch uint64
	// mark is the entered row's marks for THIS paint only: Model.paintedChart
	// derives it from the scene each time it draws, and a committed frame never
	// carries one.
	mark enteredMark
	// houseForm is the house display the operator chose, for THIS paint only,
	// like mark: Model.paintedChart fills it from the Model, so a committed
	// frame never carries one and a hard reset cannot lose the choice.
	houseForm houseForm
}

// renderBodiesBody returns the contents (no border) of the [2] Bodies
// panel for the current chart, sized to fit innerWidth × innerHeight.
//
// Format mined from legacy: one body per line, "<glyph> <name> <sign>
// <deg>°<min>' <retrograde>". The `℞` retrograde marker is rendered from
// Position.Retrograde() in the cell right after the minute mark; a slow direct
// body's S/R or S/D mark (D38) takes the speed lane in place of the bar.
//
// Under a horizontal rule the twelve house cusps follow in house order
// (the specification); nothing is pinned, so the whole list
// scrolls with its cursor. Bodies always renders only the currently focused
// slot's positions.
//
// Returns the body, the inner-row index the rule belongs on (-1 for none) and
// the viewport the scroll indicator measures.
func renderBodiesBody(state chartViewState, slots *store.Slots, slotIdx, cursorIdx, innerWidth, innerHeight int, panelFocused bool, cfg Config) (string, int, viewport) {
	return renderBodiesBodyAt(state, slots, slotIdx, cursorIdx, innerWidth, innerHeight, panelFocused, cfg, 0)
}

// renderBodiesBodyAt is renderBodiesBody with the window the panel stood at: top
// is the display row the list was last drawn from (Model.bodyTop). The window
// stays where it was while the cursor is inside it and slides only as far as
// the cursor needs; a top of 0 is a list drawn for the first time.
func renderBodiesBodyAt(state chartViewState, slots *store.Slots, slotIdx, cursorIdx, innerWidth, innerHeight int, panelFocused bool, cfg Config, top int) (string, int, viewport) {
	return renderSingleBodiesMarked(state.positions, state.cusps, state.bodyStyles, slots, slotIdx, cursorIdx, innerWidth, innerHeight, panelFocused, cfg,
		houseOverlay{cusps: state.natalCusps, positions: state.natalPositions, framed: state.framedPositions, stations: state.stations, forced: state.forced, form: state.houseForm}, state.mark, top)
}

// renderSingleBodies lays out one slot's positions: planets, the rule, then
// the cusp rows. The whole list scrolls to keep its cursor visible.
func renderSingleBodies(positions []aspect.Position, cusps aspect.Cusps, styles astrologstyle.Bodies, slots *store.Slots, slotIdx, cursorIdx, innerWidth, innerHeight int, panelFocused bool, cfg Config, ov houseOverlay) (string, int, viewport) {
	return renderSingleBodiesMarked(positions, cusps, styles, slots, slotIdx, cursorIdx, innerWidth, innerHeight, panelFocused, cfg, ov, enteredMark{}, 0)
}

// renderSingleBodiesMarked is renderSingleBodies with the entered row's moving
// body drawn bold.
func renderSingleBodiesMarked(positions []aspect.Position, cusps aspect.Cusps, styles astrologstyle.Bodies, slots *store.Slots, slotIdx, cursorIdx, innerWidth, innerHeight int, panelFocused bool, cfg Config, ov houseOverlay, mark enteredMark, top int) (string, int, viewport) {
	if len(positions) == 0 || innerWidth <= 0 || innerHeight <= 0 {
		return "", -1, viewport{}
	}
	planets, under := bodyRowsFor(positions, cusps, styles, cfg.useColor(), ov)
	return layoutBodyRows(planets, under, slots, slotIdx, cursorIdx, innerWidth, innerHeight, panelFocused, cfg, mark, top)
}

// bodyRuled says whether the list draws its rule: one display row of its own,
// worth having only where there are rows on both sides of it and the panel can
// hold more than the rule and one row beside it.
func bodyRuled(planets, under, height int) bool {
	return planets > 0 && under > 0 && height >= 3
}

// bodyWindowStart is the one place the Bodies window is decided: the display
// row (the rule counts as one) the panel draws from. top is where the window
// stood; it stays put while the cursor row is inside it and slides only as far
// as the cursor needs, and never leaves the list. The render and the Model's
// stored bodyTop both ask it, so the window the keys settle is the one painted.
func bodyWindowStart(planets, under, cursor, top, height int) int {
	n := planets + under
	if n == 0 {
		return 0
	}
	height = max(height, 1)
	cursor = min(max(cursor, 0), n-1)
	display, cursorDisplay := n, cursor
	if bodyRuled(planets, under, height) {
		display++
		if cursor >= planets {
			cursorDisplay++
		}
	}
	start := min(max(top, 0), max(display-height, 0))
	if cursorDisplay < start {
		start = cursorDisplay
	}
	if cursorDisplay >= start+height {
		start = cursorDisplay - height + 1
	}
	return start
}

// bodyDisplayRow is the display row (the rule counts as one) a cursor row is
// drawn on.
func bodyDisplayRow(planets, under, height, row int) int {
	if bodyRuled(planets, under, height) && row >= planets {
		return row + 1
	}
	return row
}

// bodyDrawnRows are the first and last cursor rows the window drawn from display
// row start shows: the rule is not a cursor row, so a window that begins or ends
// on it shows one row less at that edge.
func bodyDrawnRows(planets, under, height, start int) (first, last int) {
	n := planets + under
	ruled := bodyRuled(planets, under, height)
	display := n
	if ruled {
		display++
	}
	lo, hi := start, min(start+max(height, 1), display)-1
	if ruled && lo == planets {
		lo++
	}
	if ruled && hi == planets {
		hi--
	}
	toRow := func(d int) int {
		if ruled && d > planets {
			return d - 1
		}
		return d
	}
	return toRow(lo), toRow(hi)
}

// layoutBodyRows is renderSingleBodies from formatted rows on: the scroll
// window, the rule and the row prefixes. planets and under carry indices that
// run on from one into the other; the rule sits between them. top is the window
// the panel stood at (bodyWindowStart).
func layoutBodyRows(planets, under []bodyRenderRow, slots *store.Slots, slotIdx, cursorIdx, innerWidth, innerHeight int, panelFocused bool, cfg Config, mark enteredMark, top int) (string, int, viewport) {
	all := append(append([]bodyRenderRow{}, planets...), under...)
	allCount := len(all)
	if allCount == 0 {
		return "", -1, viewport{}
	}
	if cursorIdx < 0 {
		cursorIdx = 0
	}
	if cursorIdx >= allCount {
		cursorIdx = allCount - 1
	}
	ruled := bodyRuled(len(planets), len(under), innerHeight)
	display := len(all)
	if ruled {
		display++
	}
	start := bodyWindowStart(len(planets), len(under), cursorIdx, top, innerHeight)
	end := min(start+innerHeight, display)

	selectionColumn := slots != nil && slots.HasBodySelections(slotIdx)
	rows := make([]string, 0, end-start)
	ruleRow := -1
	first, shown := 0, 0
	for d := start; d < end; d++ {
		if ruled && d == len(planets) {
			ruleRow = len(rows)
			// A blank row the rule post-pass overlays.
			rows = append(rows, "")
			continue
		}
		i := d
		if ruled && d > len(planets) {
			i = d - 1
		}
		row := all[i]
		if shown == 0 {
			first = row.index
		}
		shown++
		selected := slots != nil && slots.BodySelected(slotIdx, row.body)
		cursorRow := row.index == cursorIdx
		prefix := rowPrefix(panelFocused, cursorRow, selected, selectionColumn, cfg)
		contentWidth := innerWidth - ansi.StringWidth(prefix)
		if contentWidth < 0 {
			contentWidth = 0
		}
		line := prefix + cutRight(row.text, contentWidth, cfg)
		state := cursorRowState(panelFocused, cursorRow)
		rows = append(rows, renderRow(line, innerWidth, state, cfg))
	}
	return strings.Join(rows, "\n"), ruleRow, viewport{Total: allCount, Visible: shown, Offset: first}
}

// houseOverlay is the anchored chart's house frame, carried into row
// rendering so a moving body can be located in the CLIENT's houses.
//
// The zero value means "no overlay": rows render exactly as a single chart
// does. It is populated only when the floating slot is focused, because that
// is the only case where the rows on screen are moving bodies and the second
// number means anything — with the anchored slot focused, a body's own house
// already IS its natal house.
type houseOverlay struct {
	cusps     aspect.Cusps
	positions []aspect.Position
	// framed are the moving bodies as the frame's zodiac places them, when
	// the row's own zodiac is another: a body's live house is read from its
	// framed longitude, since the cusps are in the frame's zodiac. Nil places
	// the row's own longitude.
	framed []aspect.Position
	// stations are the focused chart's station marks. They ride in this
	// per-row context because every row renderer already takes it; they have
	// nothing to do with houses, and active() ignores them.
	stations map[aspect.Body]lookup.Station
	forced   map[aspect.Body]lookup.Station
	// form is how the house column is drawn (house_display.go). Like the
	// stations it rides here because every row renderer already takes this
	// context; the zero value is the default form, so a bare houseOverlay{}
	// draws today's frame.
	form houseForm
}

func (o houseOverlay) active() bool { return o.cusps.Valid() }

// houses resolves the pair for one moving body: the house it occupies in the
// anchored chart natally, and the house it occupies in that same frame now.
//
// The live house is computed against the anchored chart's cusps rather than
// taken from p.House. p.House is the body's house in a chart cast for the
// current moment at the operator's active place; it rotates with the
// ascendant and shifts when a different city is picked, so it says nothing
// about the client. Legacy shipped that, watched houses jump during time
// navigation, and fixed it by placing moving positions into the fixed cusps.
func (o houseOverlay) houses(p aspect.Position) (natal, live aspect.House) {
	if !o.active() {
		return 0, p.House
	}
	lon := p.Longitude
	for _, q := range o.framed {
		if q.Body == p.Body {
			lon = q.Longitude
			break
		}
	}
	live = o.cusps.HouseFor(lon)
	if !live.Valid() {
		// No usable placement: fall back to the body's own house rather than
		// inventing one. A wrong-but-well-formatted house reads as authoritative.
		return 0, p.House
	}
	return natalHouseOf(o.positions, p.Body), live
}

// natalHouseOf returns the body's house in the anchored chart, or 0 when that
// chart does not carry the body. Matched by body identity, never by row
// order: the two sets need not hold the same bodies, and pairing by position
// would put one body's house beside another's.
func natalHouseOf(natal []aspect.Position, b aspect.Body) aspect.House {
	for _, n := range natal {
		if n.Body == b {
			return n.House
		}
	}
	return 0
}

type bodyRenderRow struct {
	index int
	body  aspect.Body
	text  string
}

// bodyRows is bodyRowsFor with no house frame: the planet rows and, under the
// rule, only the angles (in house order).
func bodyRows(ps []aspect.Position, styles astrologstyle.Bodies, useColor bool, ov houseOverlay) (planets, under []bodyRenderRow) {
	return bodyRowsFor(ps, aspect.Cusps{}, styles, useColor, ov)
}

// bodyRowsFor formats the planet rows (astrolog emit order) and the rows under
// the rule: the twelve cusps in house order, the angles among them as the
// bodies they are. A chart without a usable frame lists its angles alone.
func bodyRowsFor(ps []aspect.Position, cusps aspect.Cusps, styles astrologstyle.Bodies, useColor bool, ov houseOverlay) (planets, under []bodyRenderRow) {
	return bodyRowsWith(ps, cusps, func(p aspect.Position) string {
		return formatPositionRowStyled(p, styles[p.Body], useColor, ov)
	}, func(h int, lon float64) string {
		return formatCuspRow(h, lon, styles[aspect.Ascendant], useColor, ov.form)
	})
}

// cuspEntry is one row under the rule: an angle (the position the chart
// carries for it) or a plain cusp (its house number and longitude).
type cuspEntry struct {
	house int // 1..12
	angle *aspect.Position
	lon   float64
}

// angleHouse is the house an angle opens: Asc 1, IC 4, Dsc 7, MC 10.
func angleHouse(b aspect.Body) int {
	switch b {
	case aspect.Ascendant:
		return 1
	case aspect.ImumCoeli:
		return 4
	case aspect.Descendant:
		return 7
	case aspect.Midheaven:
		return 10
	default:
		return 0
	}
}

// cuspObjectHouse is the house of a ticked plain-cusp object row: Astrolog
// prints objects 23-33 with the label of their house ("2nd", "12th") and the
// enum has no body for them, so the row arrives as BodyUnknown with that
// label. 0 for any other row. The angle houses (1, 4, 7, 10) are typed bodies,
// never labelled cusp rows, so they are not matched here.
func cuspObjectHouse(p aspect.Position) int {
	if p.Body != aspect.BodyUnknown {
		return 0
	}
	for h := 2; h <= 12; h++ {
		if isAngleHouse(h) {
			continue
		}
		if p.Label == ordinalLabel(h) {
			return h
		}
	}
	return 0
}

func ordinalLabel(h int) string {
	switch h {
	case 2:
		return "2nd"
	case 3:
		return "3rd"
	}
	return strconv.Itoa(h) + "th"
}

// isAngleHouse reports whether h is an angle's house, which is a typed body.
func isAngleHouse(h int) bool { return h == 1 || h == 4 || h == 7 || h == 10 }

// tickedCusps marks the plain cusps whose object row the positions hold: the
// ones the operator ticked in the Bodies popup (the specification
// rule 2). Index is the house number.
func tickedCusps(ps []aspect.Position) [13]bool {
	var out [13]bool
	for _, p := range ps {
		if h := cuspObjectHouse(p); h > 0 {
			out[h] = true
		}
	}
	return out
}

// cuspEntries lists what sits under the rule, in house order: the four angles,
// and a plain cusp only for a house in ticked (and only with a usable frame).
func cuspEntries(angles []aspect.Position, cusps aspect.Cusps, ticked [13]bool) []cuspEntry {
	byHouse := map[int]*aspect.Position{}
	for i := range angles {
		byHouse[angleHouse(angles[i].Body)] = &angles[i]
	}
	var out []cuspEntry
	for h := 1; h <= 12; h++ {
		if a := byHouse[h]; a != nil {
			out = append(out, cuspEntry{house: h, angle: a})
		} else if cusps.Valid() && ticked[h] {
			out = append(out, cuspEntry{house: h, lon: cusps[h-1]})
		}
	}
	return out
}

// bodyRowsWith is bodyRowsFor with the row text supplied by format and
// formatCusp, so the comparing rows (columns) share the ordering and indexing
// of the alone rows.
func bodyRowsWith(ps []aspect.Position, cusps aspect.Cusps, format func(aspect.Position) string, formatCusp func(house int, lon float64) string) (planets, under []bodyRenderRow) {
	pl, an := partitionAngles(ps)
	ticked := tickedCusps(ps)
	for _, p := range pl {
		planets = append(planets, bodyRenderRow{body: p.Body, text: format(p)})
	}
	entries := cuspEntries(an, cusps, ticked)
	for _, e := range entries {
		if e.angle != nil {
			under = append(under, bodyRenderRow{body: e.angle.Body, text: format(*e.angle)})
		} else {
			under = append(under, bodyRenderRow{body: aspect.BodyUnknown, text: formatCusp(e.house, e.lon)})
		}
	}
	widenGlyphColumn(planets, under, entries)
	for i := range planets {
		planets[i].index = i
	}
	for i := range under {
		under[i].index = len(planets) + i
	}
	return planets, under
}

// bodyRowCount is how many rows the Bodies cursor walks: the planets and
// everything under the rule.
func bodyRowCount(ps []aspect.Position, cusps aspect.Cusps) int {
	planets, under := bodyRowSplit(ps, cusps)
	return planets + under
}

// bodyRowSplit is how many rows sit above the rule (the planets) and below it
// (the angles and cusps): the two counts the window arithmetic needs.
func bodyRowSplit(ps []aspect.Position, cusps aspect.Cusps) (planets, under int) {
	pl, an := partitionAngles(ps)
	return len(pl), len(cuspEntries(an, cusps, tickedCusps(ps)))
}

// positionRows is the flat planets-then-cusps ordering used by tests and
// callers that do not need row interaction metadata.
func positionRows(ps []aspect.Position, useColor bool) []string {
	planets, under := bodyRows(ps, nil, useColor, houseOverlay{})
	rows := make([]string, 0, len(planets)+len(under))
	for _, row := range append(planets, under...) {
		rows = append(rows, row.text)
	}
	return rows
}

// bodyAtCursor resolves a panel-local cursor into the stable body identity
// used by store selection. A plain cusp row is no body and resolves to false.
func bodyAtCursor(ps []aspect.Position, cusps aspect.Cusps, cursorIdx int) (aspect.Body, bool) {
	planets, under := bodyRowsFor(ps, cusps, nil, false, houseOverlay{})
	all := append(planets, under...)
	if cursorIdx < 0 || cursorIdx >= len(all) {
		return aspect.BodyUnknown, false
	}
	// A row the enum cannot name has no typed identity to hand out. It is
	// drawn, and it is not addressable: a per-body report is compiled from an
	// aspect.Body, and BodyUnknown has no object number. Reporting false here
	// is what makes the caller fall back instead of asking for a report that
	// cannot be built.
	if all[cursorIdx].body == aspect.BodyUnknown {
		return aspect.BodyUnknown, false
	}
	return all[cursorIdx].body, true
}

// positionGlyph is the glyph column for one row: the body's glyph when the enum
// names it, and a neutral marker when it does not. A bare "?" reads as an
// error; the bullet reads as "a body, unglyphed", which is what it is.
func positionGlyph(p aspect.Position) string {
	return bodyGlyph(p.Body)
}

// partitionAngles splits positions into planets/points vs the four chart
// angles. A ticked plain-cusp object row is neither: it decides whether its
// cusp row shows under the rule (tickedCusps), and is not drawn above it.
func partitionAngles(ps []aspect.Position) (planets, angles []aspect.Position) {
	for _, p := range ps {
		if cuspObjectHouse(p) > 0 {
			continue
		}
		if isAngle(p.Body) {
			angles = append(angles, p)
		} else {
			planets = append(planets, p)
		}
	}
	return planets, angles
}

func isAngle(b aspect.Body) bool {
	switch b {
	case aspect.Ascendant, aspect.Midheaven, aspect.Descendant, aspect.ImumCoeli:
		return true
	default:
		return false
	}
}

// renderAspectsBody returns the contents (no border) of the [3] Aspects panel
// for the active tab: the focused slot's own power-sorted waterfall on
// TabAspects, the active pair's cross waterfall on a pair tab (TabAspectsCross
// for the first pair, aspects-pair-N for the rest; see aspects_tabs.go).
func renderAspectsBody(state chartViewState, active TabID, innerWidth, innerHeight int, cfg Config) string {
	render, ok := aspectsRenderers[active]
	if !ok {
		render = aspectsRenderers[TabAspects]
		if isAspectsPairTab(active) {
			render = renderAspectsTabCross
		}
	}
	bag := resolveAspectsBag(state)
	if bag.Mark.tab != active {
		bag.Mark = enteredMark{} // the marks stay on the row's own tab (rule 5)
	}
	return render(bag, innerWidth, innerHeight, cfg)
}

func formatPositionRow(p aspect.Position, useColor bool) string {
	return formatPositionRowStyled(p, astrologstyle.BodyRow{}, useColor, houseOverlay{})
}

func formatPositionRowStyled(p aspect.Position, styles astrologstyle.BodyRow, useColor bool, ov houseOverlay) string {
	signIdx, deg, min := splitLongitude(p.Longitude)
	bodyPart := fmt.Sprintf("%s %-9s", positionGlyph(p), p.DisplayName(bodyName(p.Body)))
	positionPart := fmt.Sprintf(" %s %2d°%02d'",
		signAbbrevString(signIdx),
		deg, min,
	)
	lead := bodyPart + positionPart
	// The speed field is the ℞/space cell plus a 3-cell bar. The cell goes
	// right after the minute mark; the bar follows after one space, so the
	// house column stays where it was.
	vel := speedField(p, ov, isAngle(p.Body), useColor)
	rx, bar := splitFirstCell(vel)
	rx += " "
	// Overlay rows carry two house numbers and their own weighting. Without
	// an overlay the row renders exactly as it always has — one number, in
	// Astrolog's own house colour, no bold — so single-chart output is
	// untouched by this path.
	var tail string
	if ov.active() {
		natalHouse, liveHouse := ov.houses(p)
		tail = houseColumn(ov.form, natalHouse, liveHouse, true, styles.House, useColor)
	} else {
		tail = houseColumn(ov.form, p.House, 0, false, styles.House, useColor)
	}
	if useColor {
		return renderSGR(styles.Body, bodyPart) + renderSGR(styles.Position, positionPart) + rx + bar + tail
	}
	return lead + rx + bar + tail
}

// formatCuspRow is a plain cusp's row (house 2, 3, 5, ...): it reads like an
// angle (the specification) — the house number as its glyph, its
// ordinal as its name, the position cell, then the house frame with its own
// number in its quadrant. No speed bar: the cells a bar takes stay blank so
// the frame lines up with the bodies'. The longitude is Astrolog's, already
// parsed; this only formats it.
func formatCuspRow(house int, lon float64, styles astrologstyle.BodyRow, useColor bool, form houseForm) string {
	signIdx, deg, min := splitLongitude(lon)
	label := cuspLabel(house)
	position := fmt.Sprintf(" %s %2d°%02d'", signAbbrevString(signIdx), deg, min)
	tail := cuspFrameTail(house, form, styles.House, useColor)
	if useColor {
		return renderSGR(styles.Body, label) + renderSGR(styles.Position, position) + "     " + tail
	}
	return label + position + "     " + tail
}

// cuspLabel is a plain cusp's glyph and name cells: "2 2nd", "11 11th ".
func cuspLabel(house int) string {
	return fmt.Sprintf("%d %-9s", house, ordinalLabel(house))
}

// cuspFrameTail is the space and the house column a plain cusp's row ends in:
// the column of the cusp's own house, drawn in the form the operator chose.
func cuspFrameTail(house int, form houseForm, style astrologstyle.SGR, useColor bool) string {
	return houseColumn(form, aspect.House(house), 0, false, style, useColor)
}

// widenGlyphColumn makes the glyph column two cells wide when a two-digit
// cusp row (11, 12) is among the rows under the rule: every other row's
// one-cell glyph is right-aligned in it (the specification
// acceptance 2). The space goes in front of any escape, so the row's text and
// styling are otherwise untouched.
func widenGlyphColumn(planets, under []bodyRenderRow, entries []cuspEntry) {
	wide := false
	for _, e := range entries {
		if e.angle == nil && e.house >= 10 {
			wide = true
		}
	}
	if !wide {
		return
	}
	pad := func(rows []bodyRenderRow) {
		for i := range rows {
			rows[i].text = " " + rows[i].text
		}
	}
	pad(planets)
	k := 0
	for i := range under {
		if entries[k].angle == nil && entries[k].house >= 10 {
			k++
			continue
		}
		under[i].text = " " + under[i].text
		k++
	}
}

// splitFirstCell separates a velocity field's ℞/space cell, which never
// carries an escape sequence, from the bar after it.
func splitFirstCell(vel string) (first, rest string) {
	_, n := utf8.DecodeRuneInString(vel)
	return vel[:n], vel[n:]
}

// houseQuadrantStrings render a body's occupied house as its number positioned
// within a fixed 18-cell frame whose `|` separators split the twelve houses into
// the four zodiac quadrants [12·1·2] [3·4·5] [6·7·8] [9·10·11]. Ported from
// legacy gui/views/bodies_houses.go; indexed by house-1. This is the
// "angularity at a glance" form (house_display.go); the others are drawn by the
// same seam, houseColumn.
var houseQuadrantStrings = [12]string{
	"  1 |   |   |     ",
	"   2|   |   |     ",
	"    |3  |   |     ",
	"    | 4 |   |     ",
	"    |  5|   |     ",
	"    |   |6  |     ",
	"    |   | 7 |     ",
	"    |   |  8|     ",
	"    |   |   |9    ",
	"    |   |   | 10  ",
	"    |   |   |   11",
	"12  |   |   |     ",
}

// houseFrame is an 18-cell frame of `|` separators in which each house owns
// the cells its number sits in. Everything about it is derived from one table
// of twelve strings, so the strings stay the single source of truth for a
// frame's layout and no second copy can drift from them.
type houseFrame struct {
	// cells is the frame with house h's number in it, indexed by h-1.
	cells [12]string
	// spans records, per house, the columns its number occupies.
	spans [12][2]int
	// blank is the frame with no house marked: the separators alone.
	blank []rune
}

func newHouseFrame(cells [12]string) houseFrame {
	f := houseFrame{cells: cells}
	for i, s := range cells {
		start, end := -1, -1
		for j, r := range s {
			if r == ' ' || r == '|' {
				continue
			}
			if start < 0 {
				start = j
			}
			end = j
		}
		f.spans[i] = [2]int{start, end}
	}
	f.blank = []rune(cells[0])
	for i := range f.blank {
		if f.blank[i] != '|' {
			f.blank[i] = ' '
		}
	}
	return f
}

// angularityFrame is houseQuadrantStrings as a frame.
var angularityFrame = newHouseFrame(houseQuadrantStrings)

// houseQuadrant returns the angularity frame for house h (1..12); "" for an
// out-of-range house.
func houseQuadrant(h aspect.House) string { return angularityFrame.of(h) }

// of returns the frame for house h (1..12); "" for an out-of-range house.
func (f houseFrame) of(h aspect.House) string {
	if !h.Valid() {
		return ""
	}
	return f.cells[h-1]
}

// houseCellSpans and houseFrameBlank are the angularity frame's spans and
// blank frame.
var (
	houseCellSpans  = angularityFrame.spans
	houseFrameBlank = angularityFrame.blank
)

// composeHouseQuadrant renders the angularity frame carrying up to two house
// numbers; see houseFrame.compose.
func composeHouseQuadrant(natal, live aspect.House, liveStyle astrologstyle.SGR, useColor bool) string {
	return angularityFrame.compose(natal, live, liveStyle, useColor)
}

// compose renders one frame carrying up to two house numbers.
//
// Position already encodes which house a number is, so both fit in the same
// 18 cells with no extra width: the natal house sits at its own column and the
// transiting house at its own. Weight tells them apart — the natal number is
// dim, the live one bold — which is the distinction the operator asked to be
// load-bearing, with colour left as decoration on top.
//
// When both land on the same house the number is shown once, bold: a repeated
// digit in the same cell is impossible anyway, and boldness is the useful half.
func (f houseFrame) compose(natal, live aspect.House, liveStyle astrologstyle.SGR, useColor bool) string {
	if !natal.Valid() && !live.Valid() {
		return ""
	}
	cells := append([]rune(nil), f.blank...)
	// owner[i] is 0 for frame, 1 for the natal number, 2 for the live one.
	owner := make([]byte, len(cells))
	write := func(h aspect.House, who byte) {
		if !h.Valid() {
			return
		}
		src := []rune(f.cells[h-1])
		span := f.spans[h-1]
		for i := span[0]; i <= span[1]; i++ {
			cells[i] = src[i]
			owner[i] = who
		}
	}
	// Live written second so it wins the cell if both name the same house.
	write(natal, 1)
	write(live, 2)

	var b strings.Builder
	for i := 0; i < len(cells); {
		j := i
		for j < len(cells) && owner[j] == owner[i] {
			j++
		}
		run := string(cells[i:j])
		switch owner[i] {
		case 1:
			b.WriteString(renderDim(run, useColor))
		case 2:
			b.WriteString(renderLive(run, liveStyle, useColor))
		default:
			b.WriteString(run)
		}
		i = j
	}
	return b.String()
}

// renderDim de-emphasises the natal house so it does not compete with the
// live number beside it. Faint only, no colour: the natal position is
// context, not the thing being read.
func renderDim(text string, useColor bool) string {
	if !useColor || strings.TrimSpace(text) == "" {
		return text
	}
	return sgrFaint + text + sgrReset
}

// renderLive emphasises the house a moving body currently occupies in the
// client's chart. Bold carries the distinction on its own; Astrolog's own
// house colour rides along when there is one, as decoration.
func renderLive(text string, style astrologstyle.SGR, useColor bool) string {
	if !useColor || strings.TrimSpace(text) == "" {
		return text
	}
	return sgrBold + string(normalWeightSGR(style)) + text + sgrReset
}

func renderSGR(style astrologstyle.SGR, text string) string {
	if style == "" || text == "" {
		return text
	}
	style = normalWeightSGR(style)
	if style == "" {
		return text
	}
	return string(style) + text + sgrReset
}

// normalWeightSGR removes ANSI's bold attribute while preserving Astrolog's
// color intent. For the classic 30–37 foreground range, where Astrolog uses
// bold as the bright-color bit, it translates to the equivalent 90–97 bright
// foreground code. This keeps Astrolog's source styling separate from the
// whole-row bold treatment applied later when a body is selected.
func normalWeightSGR(style astrologstyle.SGR) astrologstyle.SGR {
	raw := string(style)
	if !strings.HasPrefix(raw, sgrIntro) || !strings.HasSuffix(raw, "m") {
		return style
	}
	params := strings.Split(strings.TrimSuffix(strings.TrimPrefix(raw, sgrIntro), "m"), ";")
	bold := false
	kept := params[:0]
	for _, param := range params {
		if param == "1" {
			bold = true
			continue
		}
		kept = append(kept, param)
	}
	if bold {
		for i, param := range kept {
			if len(param) == 2 && param[0] == '3' && param[1] >= '0' && param[1] <= '7' {
				kept[i] = string([]byte{'9', param[1]})
				break
			}
		}
	}
	if len(kept) == 0 {
		return ""
	}
	return astrologstyle.SGR(sgrIntro + strings.Join(kept, ";") + "m")
}

// splitLongitude takes a 0..360 longitude and returns (signIndex 0..11,
// integer degree 0..29, integer arcminute 0..59).
//
// The arcminute is ROUNDED, not floored, and that is a correctness rule rather
// than a preference. A longitude reaches this function as degrees plus
// arcminutes over sixty — 3Vir12 arrives as 150 + 3 + 12/60 — and no such sum
// is exact in binary. 153.2 stored as a float64 is a hair BELOW 153.2, so
// subtracting the sign leaves 3.1999999999999886, and flooring the remainder
// yields 11: one arcminute less than the number Astrolog printed and the
// operator is reading off the screen beside it. Every body row in every frame
// was a coin flip on that.
//
// Rounding cannot overflow for a value that came from Astrolog — the remainder
// is within a fraction of an arcminute of a whole one — but a longitude computed
// rather than parsed can land at 29.9999, so the carry is handled: 60' becomes
// the next degree, 30° the next sign, and the twelfth sign wraps to the first.
//
// Out-of-range input WRAPS rather than clamping, because a longitude is an
// angle on a circle: 360° is 0° Aries and −1° is 29° Pisces. The clamp this
// replaced pinned anything at or past a full circle to 359.999, which the
// rounding above would then carry to Aries 0°00' anyway — so every value from
// 360 upward, 400 included, silently became the same one. Wrapping says what
// the number means instead of picking an arbitrary edge.
func splitLongitude(lon float64) (int, int, int) {
	lon = math.Mod(lon, 360)
	if lon < 0 {
		lon += 360
	}
	signIdx := int(lon / 30)
	within := lon - float64(signIdx*30)
	deg := int(within)
	min := int(math.Round((within - float64(deg)) * 60))
	if min == 60 {
		min = 0
		deg++
	}
	if deg == 30 {
		deg = 0
		signIdx = (signIdx + 1) % 12
	}
	return signIdx, deg, min
}

func bodyName(b aspect.Body) string {
	switch b {
	case aspect.Sun:
		return "Sun"
	case aspect.Moon:
		return "Moon"
	case aspect.Mercury:
		return "Mercury"
	case aspect.Venus:
		return "Venus"
	case aspect.Mars:
		return "Mars"
	case aspect.Jupiter:
		return "Jupiter"
	case aspect.Saturn:
		return "Saturn"
	case aspect.Uranus:
		return "Uranus"
	case aspect.Neptune:
		return "Neptune"
	case aspect.Pluto:
		return "Pluto"
	case aspect.NorthNode:
		return "N.Node"
	case aspect.SouthNode:
		return "S.Node"
	case aspect.Chiron:
		return "Chiron"
	case aspect.Lilith:
		return "Lilith"
	case aspect.Ascendant:
		return "Asc"
	case aspect.Midheaven:
		return "MC"
	case aspect.Descendant:
		return "Dsc"
	case aspect.ImumCoeli:
		return "IC"
	default:
		return "?"
	}
}

// placeholderGlyph stands in for a body that has no glyph: one Astrolog
// printed that aspect.Body cannot name. Every caller of bodyGlyph is a
// position or aspect cell, so an unnamed body is always drawn as this, never as
// a question mark.
const placeholderGlyph = "·"

func bodyGlyph(b aspect.Body) string {
	switch b {
	case aspect.Sun:
		return "☉"
	case aspect.Moon:
		return "☽"
	case aspect.Mercury:
		return "☿"
	case aspect.Venus:
		return "♀"
	case aspect.Mars:
		return "♂"
	case aspect.Jupiter:
		return "♃"
	case aspect.Saturn:
		return "♄"
	case aspect.Uranus:
		return "♅"
	case aspect.Neptune:
		return "♆"
	case aspect.Pluto:
		return "♇"
	case aspect.NorthNode:
		return "☊"
	case aspect.SouthNode:
		return "☋"
	case aspect.Chiron:
		return "⚷"
	case aspect.Lilith:
		return "⚸"
	case aspect.Ascendant:
		return "A"
	case aspect.Midheaven:
		return "M"
	case aspect.Descendant:
		return "D"
	case aspect.ImumCoeli:
		return "I"
	default:
		return placeholderGlyph
	}
}

func aspectGlyph(k aspect.AspectKind) string {
	switch k {
	case aspect.Conjunction:
		return "☌"
	case aspect.Opposition:
		return "☍"
	case aspect.Square:
		return "□"
	case aspect.Trine:
		return "△"
	case aspect.Sextile:
		return "⚹"
	case aspect.Quincunx:
		return "⚻"
	case aspect.Semisextile:
		return "⚺"
	case aspect.Semisquare:
		return "∠"
	case aspect.Sesquisquare:
		return "⚼"
	case aspect.Quintile:
		return "Q"
	case aspect.Biquintile:
		return "bQ"
	default:
		return "?"
	}
}

func signAbbrevString(idx int) string {
	signs := []string{"Ari", "Tau", "Gem", "Can", "Leo", "Vir", "Lib", "Sco", "Sag", "Cap", "Aqu", "Pis"}
	if idx < 0 || idx >= len(signs) {
		return "???"
	}
	return signs[idx]
}
