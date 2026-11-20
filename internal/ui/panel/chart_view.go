// Package panel holds the renderers of the Bodies and Aspects panels: pure
// functions from a resolved bag to the panel body. The Model resolves the bag
// (ui/bodies_bag.go, ui/aspects_tabs.go); nothing here sees the Model, the
// frame type or store.
package panel

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"github.com/cruiserxyz/sextile/internal/ui/term"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// BodyRuled says whether the list draws its rule: one display row of its own,
// worth having only where there are rows on both sides of it and the panel can
// hold more than the rule and one row beside it.
func BodyRuled(planets, under, height int) bool {
	return planets > 0 && under > 0 && height >= 3
}

// BodyWindowStart is the one place the Bodies window is decided: the display
// row (the rule counts as one) the panel draws from. top is where the window
// stood; it stays put while the cursor row is inside it and slides only as far
// as the cursor needs, and never leaves the list. The render and the Model's
// stored bodyTop both ask it, so the window the keys settle is the one painted.
func BodyWindowStart(planets, under, cursor, top, height int) int {
	n := planets + under
	if n == 0 {
		return 0
	}
	height = max(height, 1)
	cursor = min(max(cursor, 0), n-1)
	display, cursorDisplay := n, cursor
	if BodyRuled(planets, under, height) {
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

// BodyDisplayRow is the display row (the rule counts as one) a cursor row is
// drawn on.
func BodyDisplayRow(planets, under, height, row int) int {
	if BodyRuled(planets, under, height) && row >= planets {
		return row + 1
	}
	return row
}

// BodyDrawnRows are the first and last cursor rows the window drawn from display
// row start shows: the rule is not a cursor row, so a window that begins or ends
// on it shows one row less at that edge.
func BodyDrawnRows(planets, under, height, start int) (first, last int) {
	n := planets + under
	ruled := BodyRuled(planets, under, height)
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

// layoutBodyRows lays out the formatted rows: the scroll
// window, the rule and the row prefixes. planets and under carry indices that
// run on from one into the other; the rule sits between them. top is the window
// the panel stood at (bodyWindowStart).
func layoutBodyRows(planets, under []BodyRenderRow, b BodiesBag) (string, int, pane.Viewport) {
	cursorIdx, innerWidth, innerHeight, panelFocused, cfg, top := b.Cursor, b.InnerW, b.InnerH, b.Focused, b.Style, b.Top
	all := append(append([]BodyRenderRow{}, planets...), under...)
	allCount := len(all)
	if allCount == 0 {
		return "", -1, pane.Viewport{}
	}
	if cursorIdx < 0 {
		cursorIdx = 0
	}
	if cursorIdx >= allCount {
		cursorIdx = allCount - 1
	}
	ruled := BodyRuled(len(planets), len(under), innerHeight)
	display := len(all)
	if ruled {
		display++
	}
	start := BodyWindowStart(len(planets), len(under), cursorIdx, top, innerHeight)
	end := min(start+innerHeight, display)

	selectionColumn := b.HasSelections
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
			first = row.Index
		}
		shown++
		selected := row.Object != (aspect.Object{}) && b.Selected[row.Object.Number]
		cursorRow := row.Index == cursorIdx
		prefix := menu.RowPrefix(panelFocused, cursorRow, selected, selectionColumn, cfg)
		contentWidth := innerWidth - ansi.StringWidth(prefix)
		if contentWidth < 0 {
			contentWidth = 0
		}
		line := prefix + term.CutRight(row.Text, contentWidth, cfg)
		state := menu.CursorRowState(panelFocused, cursorRow)
		rows = append(rows, menu.RenderRow(line, innerWidth, state, cfg))
	}
	return strings.Join(rows, "\n"), ruleRow, pane.Viewport{Total: allCount, Visible: shown, Offset: first}
}

// HouseOverlay is the anchored chart's house frame, carried into row
// rendering so a moving body can be located in the CLIENT's houses.
//
// The zero value means "no overlay": rows render exactly as a single chart
// does. It is populated only when the floating slot is focused, because that
// is the only case where the rows on screen are moving bodies and the second
// number means anything — with the anchored slot focused, a body's own house
// already IS its natal house.
type HouseOverlay struct {
	Cusps     aspect.Cusps
	Positions []aspect.Position
	// Framed are the moving bodies as the frame's zodiac places them, when
	// the row's own zodiac is another: a body's live house is read from its
	// Framed longitude, since the cusps are in the frame's zodiac. Nil places
	// the row's own longitude.
	Framed []aspect.Position
	// Stations are the focused chart's station marks. They ride in this
	// per-row context because every row renderer already takes it; they have
	// nothing to do with houses, and active() ignores them.
	Stations map[int]lookup.Station
	Forced   map[int]lookup.Station
	// Lit, with LitSGR, is the object whose row draws its name and position in
	// that colour (the enter-colour experiment): the entered Timeline aspect's
	// led object. An empty LitSGR lights nothing.
	Lit    aspect.Object
	LitSGR astrologstyle.SGR
	// Form is how the house column is drawn (house_display.go). Like the
	// stations it rides here because every row renderer already takes this
	// context; the zero value is the default Form, so a bare HouseOverlay{}
	// draws today's frame.
	Form HouseForm
	// MoonLane is what the Moon's three speed cells show instead of a bar,
	// by the Moon's object number: MoonLaneVoid, MoonLaneNone, MoonLanePending
	// or MoonLaneFailed. Only the Moon's number is ever set; it rides here for
	// the reason Stations does.
	MoonLane map[int]string
	// Nodes is the form of the Nodes and Lilith the call that cast the rows
	// carried: the North Node, the South Node and Lilith name it by a letter.
	Nodes recipe.NodeForm
}

func (o HouseOverlay) Active() bool { return o.Cusps.Valid() }

// Houses resolves the pair for one moving body: the house it occupies in the
// anchored chart natally, and the house it occupies in that same frame now.
//
// The live house is computed against the anchored chart's cusps rather than
// taken from p.House. p.House is the body's house in a chart cast for the
// current moment at the operator's active place; it rotates with the
// ascendant and shifts when a different city is picked, so it says nothing
// about the client. Legacy shipped that, watched Houses jump during time
// navigation, and fixed it by placing moving positions into the fixed cusps.
func (o HouseOverlay) Houses(p aspect.Position) (natal, live aspect.House) {
	if !o.Active() {
		return 0, p.House
	}
	lon := p.Longitude
	for _, q := range o.Framed {
		if samePosition(q, p) {
			lon = q.Longitude
			break
		}
	}
	live = o.Cusps.HouseFor(lon)
	if !live.Valid() {
		// No usable placement: fall back to the body's own house rather than
		// inventing one. A wrong-but-well-formatted house reads as authoritative.
		return 0, p.House
	}
	return natalHouseOf(o.Positions, p), live
}

// natalHouseOf returns the body's house in the anchored chart, or 0 when that
// chart does not carry the body. Matched by body identity, never by row
// order: the two sets need not hold the same bodies, and pairing by position
// would put one body's house beside another's.
func natalHouseOf(natal []aspect.Position, p aspect.Position) aspect.House {
	for _, n := range natal {
		if samePosition(n, p) {
			return n.House
		}
	}
	return 0
}

type BodyRenderRow struct {
	Index int
	// Object is the object the row stands for, the one Space marks: the
	// position's for a planet row (and for an angle), the ticked position's
	// for a cusp row the operator ticked. The zero Object marks a plain frame
	// cusp, which is no object.
	Object aspect.Object
	Text   string
}

// BodyRows is bodyRowsFor with no house frame: the planet rows and, under the
// rule, only the angles (in house order).
func BodyRows(ps []aspect.Position, styles astrologstyle.Bodies, cfg term.Style, ov HouseOverlay) (planets, under []BodyRenderRow) {
	return bodyRowsFor(ps, aspect.Cusps{}, styles, cfg, ov)
}

// bodyRowsFor formats the planet rows (astrolog emit order) and the rows under
// the rule: the twelve cusps in house order, the angles among them as the
// bodies they are. A chart without a usable frame lists its angles alone.
func bodyRowsFor(ps []aspect.Position, cusps aspect.Cusps, styles astrologstyle.Bodies, cfg term.Style, ov HouseOverlay) (planets, under []BodyRenderRow) {
	return bodyRowsWith(ps, cusps, func(p aspect.Position) string {
		return FormatPositionRowStyled(p, styles[p.Object], cfg, ov)
	}, func(h int, lon float64, o aspect.Object) string {
		return formatCuspRow(h, lon, cuspStyle(styles, o), cfg.UseColor(), ov.Form)
	})
}

// cuspStyle is the Astrolog colour of a cusp row: its own object's (the cusp
// point the operator ticked), or, for a plain frame cusp that is no object (the
// zero Object), the Ascendant's, the cusp point of house 1.
func cuspStyle(styles astrologstyle.Bodies, o aspect.Object) astrologstyle.BodyRow {
	if o != (aspect.Object{}) {
		return styles[o]
	}
	for k, row := range styles {
		if h, ok := aspect.CuspHouse(k); ok && h == 1 {
			return row
		}
	}
	return astrologstyle.BodyRow{}
}

// cuspEntry is one row under the rule: an angle (the position the chart
// carries for it) or a plain cusp (its house number and longitude, and the
// Object of the cusp point the operator ticked).
type cuspEntry struct {
	house  int // 1..12
	angle  *aspect.Position
	lon    float64
	object aspect.Object // the ticked cusp point; the zero Object for a plain frame cusp
}

// AngleHouse is the house an angle opens: Asc 1, IC 4, Dsc 7, MC 10, and zero
// for every other object. An angle is one of Astrolog's cusp points
// (aspect.CuspHouse) whose house is 1, 4, 7 or 10; the other eight cusp points
// are not angles.
func AngleHouse(o aspect.Object) int {
	if h, ok := aspect.CuspHouse(o); ok && isAngleHouse(h) {
		return h
	}
	return 0
}

// cuspObjectHouse is the house of a ticked plain-cusp object row: Astrolog's
// objects 23-33 are the house cusps (aspect.CuspHouse), and the angle houses
// (1, 4, 7, 10) are angles, never plain cusp rows, so they are not matched
// here. 0 for any other row.
func cuspObjectHouse(p aspect.Position) int {
	if h, ok := aspect.CuspHouse(p.Object); ok && !isAngleHouse(h) {
		return h
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

// isAngleHouse reports whether h is an angle's house.
func isAngleHouse(h int) bool { return h == 1 || h == 4 || h == 7 || h == 10 }

// tickedCusps is the plain cusps whose object row the positions hold: the
// ones the operator ticked in the Bodies popup (the specification
// rule 2), each as the Object of the ticked cusp point. Index is the house
// number; the zero Object means the cusp is not ticked.
func tickedCusps(ps []aspect.Position) [13]aspect.Object {
	var out [13]aspect.Object
	for _, p := range ps {
		if h := cuspObjectHouse(p); h > 0 {
			out[h] = p.Object
		}
	}
	return out
}

// cuspEntries lists what sits under the rule, in house order: the four angles,
// and a plain cusp only for a house in ticked (and only with a usable frame).
func cuspEntries(angles []aspect.Position, cusps aspect.Cusps, ticked [13]aspect.Object) []cuspEntry {
	byHouse := map[int]*aspect.Position{}
	for i := range angles {
		byHouse[AngleHouse(angles[i].Object)] = &angles[i]
	}
	var out []cuspEntry
	for h := 1; h <= 12; h++ {
		if a := byHouse[h]; a != nil {
			out = append(out, cuspEntry{house: h, angle: a})
		} else if cusps.Valid() && ticked[h] != (aspect.Object{}) {
			out = append(out, cuspEntry{house: h, lon: cusps[h-1], object: ticked[h]})
		}
	}
	return out
}

// bodyRowsWith is bodyRowsFor with the row text supplied by format and
// formatCusp, so the comparing rows (columns) share the ordering and indexing
// of the alone rows.
func bodyRowsWith(ps []aspect.Position, cusps aspect.Cusps, format func(aspect.Position) string, formatCusp func(house int, lon float64, o aspect.Object) string) (planets, under []BodyRenderRow) {
	pl, an := PartitionAngles(ps)
	ticked := tickedCusps(ps)
	for _, p := range pl {
		planets = append(planets, BodyRenderRow{Object: p.Object, Text: format(p)})
	}
	entries := cuspEntries(an, cusps, ticked)
	for _, e := range entries {
		if e.angle != nil {
			under = append(under, BodyRenderRow{Object: e.angle.Object, Text: format(*e.angle)})
		} else {
			under = append(under, BodyRenderRow{Object: e.object, Text: formatCusp(e.house, e.lon, e.object)})
		}
	}
	widenGlyphColumn(planets, under, entries)
	for i := range planets {
		planets[i].Index = i
	}
	for i := range under {
		under[i].Index = len(planets) + i
	}
	return planets, under
}

// BodyRowCount is how many rows the Bodies cursor walks: the planets and
// everything under the rule.
func BodyRowCount(ps []aspect.Position, cusps aspect.Cusps) int {
	planets, under := BodyRowSplit(ps, cusps)
	return planets + under
}

// BodyRowSplit is how many rows sit above the rule (the planets) and below it
// (the angles and cusps): the two counts the window arithmetic needs.
func BodyRowSplit(ps []aspect.Position, cusps aspect.Cusps) (planets, under int) {
	pl, an := PartitionAngles(ps)
	return len(pl), len(cuspEntries(an, cusps, tickedCusps(ps)))
}

// AnyRowMarked reports whether any row of the Bodies table drawn from ps and
// cusps (the planets and everything under the rule, wherever the window sits)
// stands for an object in marked. The mark column is reserved only while it is
// true: a mark whose object has no row in this table (the chart does not draw
// it) shows nowhere, and a column reserved for it would be an unexplained gap.
func AnyRowMarked(ps []aspect.Position, cusps aspect.Cusps, marked map[int]bool) bool {
	if len(marked) == 0 {
		return false
	}
	planets, under := bodyRowIdentities(ps, cusps)
	for _, r := range append(planets, under...) {
		if r.Object != (aspect.Object{}) && marked[r.Object.Number] {
			return true
		}
	}
	return false
}

// ObjectAtCursor resolves a panel-local cursor into the object Space marks: the
// row's Object, typed or not (a planet, Fortune, a fixed star, a ticked cusp
// point). A plain frame cusp is no object and gives false, as does an index out
// of range. Marking, the Timeline lead, the Details report, the Template tab
// and the wheel's cursor tier all read the cursor through it.
func ObjectAtCursor(ps []aspect.Position, cusps aspect.Cusps, cursorIdx int) (aspect.Object, bool) {
	planets, under := bodyRowIdentities(ps, cusps)
	all := append(planets, under...)
	if cursorIdx < 0 || cursorIdx >= len(all) {
		return aspect.Object{}, false
	}
	o := all[cursorIdx].Object
	return o, o != (aspect.Object{})
}

// PositionGlyph is the glyph column for one row: the glyph of the row's object
// by its registry name, and a neutral marker when Sextile presents none. A bare
// "?" reads as an error; the bullet reads as "an object, unglyphed", which is
// what it is.
func PositionGlyph(p aspect.Position) string {
	return BodyGlyph(p.Object)
}

// PartitionAngles splits positions into planets/points vs the four chart
// angles. A ticked plain-cusp object row is neither: it decides whether its
// cusp row shows under the rule (tickedCusps), and is not drawn above it.
func PartitionAngles(ps []aspect.Position) (planets, angles []aspect.Position) {
	for _, p := range ps {
		if cuspObjectHouse(p) > 0 {
			continue
		}
		if IsAngle(p.Object) {
			angles = append(angles, p)
		} else {
			planets = append(planets, p)
		}
	}
	return planets, angles
}

// IsAngle reports whether o is one of the four chart angles: Astrolog's cusp
// point of house 1, 4, 7 or 10 (Ascendant, Nadir, Descendant, Midheaven). The
// other eight cusp points are not angles.
func IsAngle(o aspect.Object) bool { return AngleHouse(o) > 0 }

// bodyNameWidth is the name column of a Bodies row, in cells.
const bodyNameWidth = 9

// bodyNameCell is a row's name cell: Sextile's own name for one of the eighteen
// objects it presents, the object's full name from Astrolog's registry for any
// other, cut to the column with the repository's mark (term.CutRight, the rule
// TestTruncationIsMarked pins) and padded to it. A name that fits prints whole.
//
// The North Node, the South Node and Lilith carry one letter after the name:
// `m` mean, `t` true, `n` natural, the form of the call that cast the row,
// faint, plain with colours off.
func bodyNameCell(p aspect.Position, cfg term.Style, ov HouseOverlay) string {
	letter := nodeLetter(p.Object, ov.Nodes)
	room := bodyNameWidth
	if letter != 0 {
		room -= 2
	}
	name := term.CutRight(BodyName(p.Object), room, cfg)
	if letter != 0 {
		mark := string(letter)
		if cfg.UseColor() {
			mark = term.SGRFaint + mark + term.SGRNormalIntensity
		}
		name += " " + mark
		if w := ansi.StringWidth(name); w < bodyNameWidth {
			name += strings.Repeat(" ", bodyNameWidth-w)
		}
		return name
	}
	if w := ansi.StringWidth(name); w < bodyNameWidth {
		name += strings.Repeat(" ", bodyNameWidth-w)
	}
	return name
}

// nodeLetter is the letter of the form f for o: the Node's for the North and
// South Node, Lilith's for Lilith, 0 for every other object. An object is what
// Astrolog's registry names it.
func nodeLetter(o aspect.Object, f recipe.NodeForm) byte {
	node, lilith := f.Letters()
	switch o.Name {
	case "North Node", "South Node":
		return node
	case "Lilith":
		return lilith
	}
	return 0
}

// bodyRowIdentities is the Bodies rows as the panel draws them, each with its
// object and index and no text. The order is PartitionAngles' and cuspEntries',
// never the text's, so a caller that reads only which object a row is (the
// cursor lookups, run many times per frame on every key) formats nothing: with
// every group ticked, formatting each row's text made a held cursor key trail
// behind its release (operator's walk, 2026-11-06).
func bodyRowIdentities(ps []aspect.Position, cusps aspect.Cusps) (planets, under []BodyRenderRow) {
	return bodyRowsWith(ps, cusps, func(aspect.Position) string { return "" },
		func(int, float64, aspect.Object) string { return "" })
}

func FormatPositionRowStyled(p aspect.Position, styles astrologstyle.BodyRow, cfg term.Style, ov HouseOverlay) string {
	useColor := cfg.UseColor()
	signIdx, deg, min := SplitLongitude(p.Longitude)
	bodyPart := PositionGlyph(p) + " " + bodyNameCell(p, cfg, ov)
	positionPart := fmt.Sprintf(" %s %2d°%02d'",
		SignAbbrevString(signIdx),
		deg, min,
	)
	lead := bodyPart + positionPart
	// The speed field is the ℞/space cell plus a 3-cell bar. The cell goes
	// right after the minute mark; the bar follows after one space, so the
	// house column stays where it was.
	vel := speedField(p, ov, IsAngle(p.Object), useColor)
	rx, bar := splitFirstCell(vel)
	rx += " "
	// Overlay rows carry two house numbers and their own weighting. Without
	// an overlay the row renders exactly as it always has — one number, in
	// Astrolog's own house colour, no bold — so single-chart output is
	// untouched by this path.
	var tail string
	if ov.Active() {
		natalHouse, liveHouse := ov.Houses(p)
		tail = HouseColumn(ov.Form, natalHouse, liveHouse, true, styles.House, useColor)
	} else {
		tail = HouseColumn(ov.Form, p.House, 0, false, styles.House, useColor)
	}
	if useColor {
		if ov.LitSGR != "" && p.Object == ov.Lit {
			// The led row: the glyph keeps its colour, the name cell and the
			// position part (sign, degrees) take the aspect's. The speed lane
			// and the house column are as they were.
			glyph := PositionGlyph(p) + " "
			return RenderSGR(styles.Body, glyph) + RenderSGR(ov.LitSGR, bodyNameCell(p, cfg, ov)+positionPart) + rx + bar + tail
		}
		return RenderSGR(styles.Body, bodyPart) + RenderSGR(styles.Position, positionPart) + rx + bar + tail
	}
	return lead + rx + bar + tail
}

// formatCuspRow is a plain cusp's row (house 2, 3, 5, ...): it reads like an
// angle (the specification) — the house number as its glyph, its
// ordinal as its name, the position cell, then the house frame with its own
// number in its quadrant. No speed bar: the cells a bar takes stay blank so
// the frame lines up with the bodies'. The longitude is Astrolog's, already
// parsed; this only formats it.
func formatCuspRow(house int, lon float64, styles astrologstyle.BodyRow, useColor bool, form HouseForm) string {
	signIdx, deg, min := SplitLongitude(lon)
	label := cuspLabel(house)
	position := fmt.Sprintf(" %s %2d°%02d'", SignAbbrevString(signIdx), deg, min)
	tail := cuspFrameTail(house, form, styles.House, useColor)
	if useColor {
		return RenderSGR(styles.Body, label) + RenderSGR(styles.Position, position) + "     " + tail
	}
	return label + position + "     " + tail
}

// cuspLabel is a plain cusp's glyph and name cells: "2 2nd", "11 11th ".
func cuspLabel(house int) string {
	return fmt.Sprintf("%d %-9s", house, ordinalLabel(house))
}

// cuspFrameTail is the space and the house column a plain cusp's row ends in:
// the column of the cusp's own house, drawn in the form the operator chose.
func cuspFrameTail(house int, form HouseForm, style astrologstyle.SGR, useColor bool) string {
	return HouseColumn(form, aspect.House(house), 0, false, style, useColor)
}

// widenGlyphColumn makes the glyph column two cells wide when a two-digit
// cusp row (11, 12) is among the rows under the rule: every other row's
// one-cell glyph is right-aligned in it (the specification
// acceptance 2). The space goes in front of any escape, so the row's text and
// styling are otherwise untouched.
func widenGlyphColumn(planets, under []BodyRenderRow, entries []cuspEntry) {
	wide := false
	for _, e := range entries {
		if e.angle == nil && e.house >= 10 {
			wide = true
		}
	}
	if !wide {
		return
	}
	pad := func(rows []BodyRenderRow) {
		for i := range rows {
			rows[i].Text = " " + rows[i].Text
		}
	}
	pad(planets)
	k := 0
	for i := range under {
		if entries[k].angle == nil && entries[k].house >= 10 {
			k++
			continue
		}
		under[i].Text = " " + under[i].Text
		k++
	}
}

// splitFirstCell separates a velocity field's ℞/space cell, which never
// carries an escape sequence, from the bar after it.
func splitFirstCell(vel string) (first, rest string) {
	_, n := utf8.DecodeRuneInString(vel)
	return vel[:n], vel[n:]
}

// HouseQuadrantStrings render a body's occupied house as its number positioned
// within a fixed 18-cell frame whose `|` separators split the twelve houses into
// the four zodiac quadrants [12·1·2] [3·4·5] [6·7·8] [9·10·11]. Ported from
// legacy gui/views/bodies_houses.go; indexed by house-1. This is the
// "angularity at a glance" form (house_display.go); the others are drawn by the
// same seam, HouseColumn.
var HouseQuadrantStrings = [12]string{
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
var angularityFrame = newHouseFrame(HouseQuadrantStrings)

// HouseQuadrant returns the angularity frame for house h (1..12); "" for an
// out-of-range house.
func HouseQuadrant(h aspect.House) string { return angularityFrame.Of(h) }

// Of returns the frame for house h (1..12); "" for an out-Of-range house.
func (f houseFrame) Of(h aspect.House) string {
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

// ComposeHouseQuadrant renders the angularity frame carrying up to two house
// numbers; see houseFrame.compose.
func ComposeHouseQuadrant(natal, live aspect.House, liveStyle astrologstyle.SGR, useColor bool) string {
	return angularityFrame.Compose(natal, live, liveStyle, useColor)
}

// Compose renders one frame carrying up to two house numbers.
//
// Position already encodes which house a number is, so both fit in the same
// 18 cells with no extra width: the natal house sits at its own column and the
// transiting house at its own. Weight tells them apart — the natal number is
// dim, the live one bold — which is the distinction the operator asked to be
// load-bearing, with colour left as decoration on top.
//
// When both land on the same house the number is shown once, bold: a repeated
// digit in the same cell is impossible anyway, and boldness is the useful half.
func (f houseFrame) Compose(natal, live aspect.House, liveStyle astrologstyle.SGR, useColor bool) string {
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
			b.WriteString(RenderDim(run, useColor))
		case 2:
			b.WriteString(renderLive(run, liveStyle, useColor))
		default:
			b.WriteString(run)
		}
		i = j
	}
	return b.String()
}

// RenderDim de-emphasises the natal house so it does not compete with the
// live number beside it. Faint only, no colour: the natal position is
// context, not the thing being read.
func RenderDim(text string, useColor bool) string {
	if !useColor || strings.TrimSpace(text) == "" {
		return text
	}
	return term.SGRFaint + text + term.SGRReset
}

// renderLive emphasises the house a moving body currently occupies in the
// client's chart. Bold carries the distinction on its own; Astrolog's own
// house colour rides along when there is one, as decoration.
func renderLive(text string, style astrologstyle.SGR, useColor bool) string {
	if !useColor || strings.TrimSpace(text) == "" {
		return text
	}
	return term.SGRBold + string(NormalWeightSGR(style)) + text + term.SGRReset
}

func RenderSGR(style astrologstyle.SGR, text string) string {
	if style == "" || text == "" {
		return text
	}
	style = NormalWeightSGR(style)
	if style == "" {
		return text
	}
	return string(style) + text + term.SGRReset
}

// NormalWeightSGR removes ANSI's bold attribute while preserving Astrolog's
// color intent. For the classic 30–37 foreground range, where Astrolog uses
// bold as the bright-color bit, it translates to the equivalent 90–97 bright
// foreground code. This keeps Astrolog's source styling separate from the
// whole-row bold treatment applied later when a body is selected.
func NormalWeightSGR(style astrologstyle.SGR) astrologstyle.SGR {
	raw := string(style)
	if !strings.HasPrefix(raw, term.SGRIntro) || !strings.HasSuffix(raw, "m") {
		return style
	}
	params := strings.Split(strings.TrimSuffix(strings.TrimPrefix(raw, term.SGRIntro), "m"), ";")
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
	return astrologstyle.SGR(term.SGRIntro + strings.Join(kept, ";") + "m")
}

// SplitLongitude takes a 0..360 longitude and returns (signIndex 0..11,
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
func SplitLongitude(lon float64) (int, int, int) {
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

// presentation is Sextile's own spelling of one object: the short name a row
// prints and the glyph in its glyph column.
type presentation struct{ name, glyph string }

// presented is Sextile's presentation for the eighteen objects it has glyphs
// for, keyed by the REGISTRY NAME the binary in use gives them (the names
// aspect.LegacyName maps to), never by number: a binary that numbers its
// objects differently keeps every glyph. Any other object prints its registry
// name and PlaceholderGlyph.
var presented = map[string]presentation{
	"Sun":        {"Sun", "☉"},
	"Moon":       {"Moon", "☽"},
	"Mercury":    {"Mercury", "☿"},
	"Venus":      {"Venus", "♀"},
	"Mars":       {"Mars", "♂"},
	"Jupiter":    {"Jupiter", "♃"},
	"Saturn":     {"Saturn", "♄"},
	"Uranus":     {"Uranus", "♅"},
	"Neptune":    {"Neptune", "♆"},
	"Pluto":      {"Pluto", "♇"},
	"North Node": {"N.Node", "☊"},
	"South Node": {"S.Node", "☋"},
	"Chiron":     {"Chiron", "⚷"},
	"Lilith":     {"Lilith", "⚸"},
	"Ascendant":  {"Asc", "A"},
	"Midheaven":  {"MC", "M"},
	"Descendant": {"Dsc", "D"},
	"Nadir":      {"IC", "I"},
}

// BodyName is the name a row prints for an object: Sextile's short name for
// one of the eighteen it presents (N.Node, Asc, MC), the registry's own name
// for any other (Fortune, Ceres, 11th Cusp), "?" when the object has no name.
// The caller cuts it to its column.
func BodyName(o aspect.Object) string {
	if p, ok := presented[o.Name]; ok {
		return p.name
	}
	if n := strings.TrimRight(o.Name, " "); n != "" {
		return n
	}
	return "?"
}

// PlaceholderGlyph stands in for an object that has no glyph: one Sextile does
// not present (Fortune, a fixed star, a cusp point). Every caller of
// BodyGlyph is a position or aspect cell, so an unpresented object is always
// drawn as this, never as a question mark.
const PlaceholderGlyph = "·"

// BodyGlyph is the glyph of an object by its registry name, PlaceholderGlyph
// for any object Sextile does not present.
func BodyGlyph(o aspect.Object) string {
	if p, ok := presented[o.Name]; ok {
		return p.glyph
	}
	return PlaceholderGlyph
}

// aspectGlyphs is the glyph of the aspects that have one, keyed by Astrolog's
// three letters. It is the only table of glyphs; any other aspect is drawn as
// its three letters.
var aspectGlyphs = map[string]string{
	"Con": "☌",
	"Opp": "☍",
	"Squ": "□",
	"Tri": "△",
	"Sex": "⚹",
	"Inc": "⚻",
	"SSx": "⚺",
	"SSq": "∠",
	"Ses": "⚼",
	"Qui": "Q",
	"BQn": "bQ",
}

// HasGlyph reports whether the aspect has a glyph of its own in the table.
func HasGlyph(k aspect.AspectKind) bool {
	_, ok := aspectGlyphs[k.Abbrev]
	return ok
}

// AspectGlyph is the mark of an aspect: its glyph where it has one, else
// Astrolog's three letters; "?" for no aspect.
func AspectGlyph(k aspect.AspectKind) string {
	if !k.Known() {
		return "?"
	}
	if g, ok := aspectGlyphs[k.Abbrev]; ok {
		return g
	}
	return k.Abbrev
}

func SignAbbrevString(idx int) string {
	signs := []string{"Ari", "Tau", "Gem", "Can", "Leo", "Vir", "Lib", "Sco", "Sag", "Cap", "Aqu", "Pis"}
	if idx < 0 || idx >= len(signs) {
		return "???"
	}
	return signs[idx]
}
