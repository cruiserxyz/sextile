package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// the specification: every ℞ sits in the cell right after the
// minute mark, and the rule is followed by all twelve cusps in house order.

// cuspText is "Sgn DD°MM'" for a longitude, written from the spec's format.
func cuspText(lon float64) string {
	signIdx, deg, min := splitLongitude(lon)
	return fmt.Sprintf("%s %2d°%02d'", signAbbrevString(signIdx), deg, min)
}

// Sentences 1 and 2: the cell right after the minute is ℞ or a space, in both
// layouts; the bar follows and keeps its three cells.
func TestRetrogradeSitsAfterTheMinute(t *testing.T) {
	retro := aspect.Position{Body: aspect.Saturn, Longitude: 130.2733, Speed: -1.5, House: 3}
	direct := aspect.Position{Body: aspect.Mars, Longitude: 5.9667, Speed: 1.2, House: 6}
	angle := aspect.Position{Body: aspect.Ascendant, Longitude: 210.8667, Speed: 0.7, House: 1}

	for _, tc := range []struct {
		name string
		p    aspect.Position
		rx   string
	}{{"retrograde", retro, "℞"}, {"direct", direct, " "}, {"angle", angle, " "}} {
		row := formatPositionRow(tc.p, false)
		at := strings.Index(row, cuspText(tc.p.Longitude))
		if at < 0 {
			t.Fatalf("%s: no position cell in %q", tc.name, row)
		}
		after := row[at+len(cuspText(tc.p.Longitude)):]
		if !strings.HasPrefix(after, tc.rx+" ") {
			t.Errorf("%s: the cell after the minute should be %q and then a space, row reads %q", tc.name, tc.rx, row)
		}
		if strings.Count(row, "℞") != strings.Count(tc.rx, "℞") {
			t.Errorf("%s: %d ℞ glyphs in %q, want %d", tc.name, strings.Count(row, "℞"), row, strings.Count(tc.rx, "℞"))
		}
		// bar: 3 cells after the space, unchanged length.
		if !isAngle(tc.p.Body) {
			bar := dropFirstCell(velocityBar(tc.p.Speed, false, false))
			if !strings.Contains(row, tc.rx+" "+bar) {
				t.Errorf("%s: the 3-cell bar %q does not follow the ℞ cell in %q", tc.name, bar, row)
			}
		}
	}

	// Comparing: the same cell inside the column, before the next separator.
	cols := []bodyColumn{
		{positions: []aspect.Position{retro}},
		{positions: []aspect.Position{{Body: aspect.Saturn, Longitude: 200.6833, Speed: 0.4}}},
	}
	cfg := Config{Color: ColorNever}
	row := formatColumnRow(retro, astrologStyleZero, cols, 0, wholeWindow(len(cols)), cfg, houseOverlay{})
	if !strings.Contains(row, "["+cuspText(retro.Longitude)+"℞]") {
		t.Errorf("retrograde column should read [%s℞], row %q", cuspText(retro.Longitude), row)
	}
	if !strings.Contains(row, cuspText(200.6833)+" ") || strings.Contains(strings.SplitN(row, "]", 2)[1], "℞") {
		t.Errorf("direct column should end in a space, no ℞ beyond the first column: %q", row)
	}
	if w := ansi.StringWidth(columnCell(cols[0], aspect.Saturn, 0, 0, "", cfg)); w != bodiesColumnWidth {
		t.Errorf("column is %d cells wide, want %d (the ℞ cell moved, it was not added)", w, bodiesColumnWidth)
	}
}

// houseLabels is what the rows under the rule start with, in house order.
// A plain cusp reads like an angle (the specification acceptance
// 1); with cusp 11 or 12 shown the glyph column is two cells, so a one-cell
// glyph has a leading space, which the prefix checks trim.
var houseLabels = []string{"A Asc", "2 2nd", "3 3rd", "I IC", "5 5th", "6 6th", "D Dsc", "8 8th", "9 9th", "M MC", "11 11th", "12 12th"}

// underRule returns the rendered rows below the rule.
func underRule(t *testing.T, body string, ruleRow int) []string {
	t.Helper()
	rows := strings.Split(body, "\n")
	if ruleRow < 0 || ruleRow >= len(rows) {
		t.Fatalf("no rule in view (rule row %d of %d rows):\n%s", ruleRow, len(rows), body)
	}
	return rows[ruleRow+1:]
}

// plainCuspLabels is what Astrolog prints for the eight plain-cusp objects
// (23-33 minus the angle houses), keyed by house.
var plainCuspLabels = map[int]string{2: "2nd", 3: "3rd", 5: "5th", 6: "6th", 8: "8th", 9: "9th", 11: "11th", 12: "12th"}

// withTicked appends the cusp object rows Astrolog prints for the given houses
// (their longitudes are the frame's), as the parser delivers them: no typed
// body, the printed label.
func withTicked(ps []aspect.Position, cusps aspect.Cusps, houses ...int) []aspect.Position {
	out := append([]aspect.Position{}, ps...)
	for _, h := range houses {
		out = append(out, aspect.Position{Label: plainCuspLabels[h], Longitude: cusps[h-1], House: aspect.House(h)})
	}
	return out
}

var allPlainCusps = []int{2, 3, 5, 6, 8, 9, 11, 12}

// Sentences 3, 4 and 6: twelve rows in house order, each with its cusp's sign
// and degree; the angles keep their house mark and bar, the other eight show
// the house frame of their own house and no speed bar
// (the specification); each comparing column shows its own chart's cusps.
func TestBodiesListsTwelveCuspsInHouseOrder(t *testing.T) {
	cusps := fullChartCusps()
	// the specification rule 1: the eight plain cusps show only
	// when ticked, so tick them all; the twelve-row order still holds.
	state := chartViewState{positions: withTicked(fullChartPositions(), cusps, allPlainCusps...), cusps: cusps}
	cfg := Config{Accessible: true}
	body, ruleRow, _ := renderBodiesBody(state, nil, 0, 0, 80, 40, false, cfg)
	under := underRule(t, body, ruleRow)
	if len(under) != 12 {
		t.Fatalf("%d rows under the rule, want 12:\n%s", len(under), body)
	}
	for i, row := range under {
		if !strings.HasPrefix(strings.TrimLeft(row, " "), houseLabels[i]) {
			t.Errorf("row %d under the rule = %q, want it to start with %q", i+1, row, houseLabels[i])
		}
		wantCusp := cuspText(cusps[i])
		if !strings.Contains(row, wantCusp) {
			t.Errorf("house %d row %q lacks its cusp %q", i+1, row, wantCusp)
		}
		isAngleRow := i%3 == 0
		hasBar := strings.ContainsAny(row, "▏▎▍▌▋▊▉█")
		hasHouse := strings.Contains(row, "|")
		if !hasHouse {
			t.Errorf("row %q lost its house frame", row)
		}
		// Reversed by the specification acceptance 1 (was: a plain
		// cusp row shows the position cell only): the frame carries the cusp's
		// own house number; still no speed bar on a cusp.
		if !isAngleRow && (hasBar || !strings.HasSuffix(strings.TrimRight(row, " "), framePrefix(i+1))) {
			t.Errorf("plain cusp row %q should end in house %d's frame and have no speed bar", row, i+1)
		}
	}

	// Comparing: each column shows its own chart's cusp for the row's house.
	other := fullChartCusps()
	for i := range other {
		other[i] = float64(int(other[i]+97) % 360)
	}
	// Real angle positions ARE their cusps (Astrolog prints both), so the
	// second chart's angles move with its frame.
	otherPositions := fullChartPositions()
	for i, p := range otherPositions {
		if h := angleHouse(p.Body); h > 0 {
			otherPositions[i].Longitude = other[h-1]
		}
	}
	cols := []bodyColumn{
		{positions: withTicked(fullChartPositions(), cusps, allPlainCusps...), cusps: cusps},
		{positions: withTicked(otherPositions, other, allPlainCusps...), cusps: other},
	}
	view := bodiesView{shown: 2, comparing: true, ok: true, cols: cols, focus: 0}
	cbody, crule, _ := renderBodiesFor(view, state, nil, 0, 0, 100, 40, false, cfg)
	cunder := underRule(t, cbody, crule)
	if len(cunder) != 12 {
		t.Fatalf("comparing: %d rows under the rule, want 12", len(cunder))
	}
	for i, row := range cunder {
		if !strings.HasPrefix(strings.TrimLeft(row, " "), houseLabels[i]) {
			t.Errorf("comparing row %d = %q, want it to start with %q", i+1, row, houseLabels[i])
		}
		a, b := strings.Index(row, cuspText(cusps[i])), strings.Index(row, cuspText(other[i]))
		if a < 0 || b < 0 || a >= b {
			t.Errorf("comparing house %d row %q should read chart 1's cusp %q then chart 2's %q", i+1, row, cuspText(cusps[i]), cuspText(other[i]))
		}
	}
}

// The focused chart's own cusps, and each shown chart's, travel from the
// lookup that produced its positions into the panel's state (no new call).
func TestFocusedChartCuspsReachTheBodiesState(t *testing.T) {
	stubColumnLookups(t, nil)
	cuspsFor := func(shift float64) aspect.Cusps {
		c := fullChartCusps()
		for i := range c {
			c[i] = float64(int(c[i]+shift) % 360)
		}
		return c
	}
	firstCallNatalPositions = func(_ context.Context, _ *lookup.Probers, c store.AnchorChart, _ lookup.Lens) (lookup.Result, error) {
		return lookup.Result{Positions: shiftedChart(shiftOf(c.When)), Cusps: cuspsFor(shiftOf(c.When))}, nil
	}
	firstCallTransits = func(_ context.Context, _ *lookup.Probers, when time.Time, _ location.PlaceInfo, _ lookup.Lens) (lookup.Result, error) {
		return lookup.Result{Positions: shiftedChart(shiftOf(when) + 11), Cusps: cuspsFor(shiftOf(when) + 11)}, nil
	}
	m := settledComparing(t, 1)
	cols, _ := m.bodyColumnsForScene()
	shown := m.deriveScene().shown
	for i, slot := range shown {
		shift := shiftOf(slot.Time)
		if slot.Floating {
			shift += 11
		} else {
			shift = shiftOf(anchorOf(slot).When)
		}
		if want := cuspsFor(shift); cols[i].cusps != want {
			t.Errorf("column %d carries cusps %v, want its own chart's %v", i+1, cols[i].cusps, want)
		}
	}
	if !m.chart.cusps.Valid() {
		t.Fatalf("the focused chart's cusps did not reach the panel state: %v", m.chart.cusps)
	}
	if m.chart.cusps != cols[1].cusps {
		t.Errorf("focused (cursor on ②) cusps = %v, want ②'s %v", m.chart.cusps, cols[1].cusps)
	}
	// Single frames too: the natal and transit single computes carry them.
	nat := computeNatalSingleCmd(context.Background(), 1, 1, nil, anchorOf(shown[0]), m.lens)().(FirstFrameReadyMsg)
	if !nat.Cusps.Valid() {
		t.Errorf("natal single frame carries no cusps")
	}
	tr := computeChartCmd(context.Background(), 1, 1, nil, m.targetFloatingTime, m.place, m.lens)().(FirstFrameReadyMsg)
	if !tr.Cusps.Valid() {
		t.Errorf("transit single frame carries no cusps")
	}
}

// Sentences 5 and 7: the cusp rows scroll with the list, the counter counts
// them, and the existing cursor walks all of them and keeps its memory.
func TestCuspRowsScrollWithTheList(t *testing.T) {
	m := rowSelectionModel(t)
	m.chart.cusps = fullChartCusps()
	m.chart.positions = withTicked(m.chart.positions, m.chart.cusps, allPlainCusps...)
	m.focused = PanelBodies
	total := bodyRowCount(m.chart.positions, m.chart.cusps)
	if total != 8+12 {
		t.Fatalf("fixture: %d rows, want 8 planets + 12 cusps", total)
	}
	m.bodyCursorIdx = 0
	for i := 0; i < total+3; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
	}
	if m.bodyCursorIdx != total-1 {
		t.Fatalf("the cursor ends on row %d after walking down, want the last cusp row %d", m.bodyCursorIdx, total-1)
	}
	// A window too short to hold the list: the last row is in it, scrolled.
	body, _, scroll := renderBodiesBody(m.chart, m.slots, 1, m.bodyCursorIdx, 60, 8, true, Config{Accessible: true})
	rows := strings.Split(body, "\n")
	if !strings.Contains(rows[len(rows)-1], "12") || !strings.Contains(rows[len(rows)-1], cuspText(m.chart.cusps[11])) {
		t.Errorf("window scrolled to the last row should end on cusp 12, got %q", rows[len(rows)-1])
	}
	if scroll.Total != total || !scroll.overflows() {
		t.Errorf("viewport = %+v, want Total %d and overflow", scroll, total)
	}
	if pos := (listPosition{Cursor: m.bodyCursorIdx, Total: bodyRowCount(m.chart.positions, m.chart.cusps)}).text(); pos != fmt.Sprintf("%d of %d", total, total) {
		t.Errorf("counter reads %q", pos)
	}
	// Per-chart cursor memory: leave the slot and come back.
	m.bodyCursorIdx = 15
	m = m.rememberBodyRow()
	if got := m.bodyRows[m.focusedSlot()]; got != 15 {
		t.Errorf("remembered row = %d, want 15 (a cusp row)", got)
	}
	// A plain cusp row is no body: Space and Details have nothing to select.
	if _, ok := bodyAtCursor(m.chart.positions, m.chart.cusps, 9); ok {
		t.Errorf("cursor on cusp row 2 resolved to a body")
	}
	if b, ok := bodyAtCursor(m.chart.positions, m.chart.cusps, 8); !ok || b != aspect.Ascendant {
		t.Errorf("first row under the rule = (%v, %v), want the Ascendant", b, ok)
	}
}

// A new frame with fewer rows pulls a cursor left on a cusp row back in range.
func TestCuspCursorClampsWhenTheFrameShrinks(t *testing.T) {
	m := rowSelectionModel(t)
	m.chart.cusps = fullChartCusps()
	m.bodyCursorIdx = 19
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{Positions: m.chart.positions[:4]})
	if m.bodyCursorIdx != bodyRowCount(m.chart.positions, m.chart.cusps)-1 {
		t.Errorf("cursor = %d after a smaller frame, want the last row %d", m.bodyCursorIdx, bodyRowCount(m.chart.positions, m.chart.cusps)-1)
	}
}

// The frames the spec freezes: one column with a retrograde body and the
// twelve cusps (132x40), the comparing layout (132x40), and one column
// scrolled to the cusps where the list no longer fits (80x24).
func TestBodiesRxAndCuspsFrames(t *testing.T) {
	forceColor(t)
	one := func(w, h int) Model {
		m := frameModel(t, Config{Color: ColorNever}, w, h, 1, 1)
		m.slots.ToggleSlotSelection(1) // alone: the comparing pair is deselected
		if m.comparing() {
			t.Fatal("fixture: still comparing")
		}
		m.chart.columns, m.chart.columnsFor = nil, nil
		m.chart.cusps = fullChartCusps()
		m.focused = PanelBodies
		return m
	}
	m := one(132, 40)
	writeDesignFrame(t, "bodies-rx-cusps-one-132x40-never", m.View())

	m = one(80, 24)
	m.bodyCursorIdx = bodyRowCount(m.chart.positions, m.chart.cusps) - 1
	writeDesignFrame(t, "bodies-rx-cusps-one-80x24-never", m.View())

	stubColumnLookups(t, nil)
	cuspsFor := func(shift float64) aspect.Cusps {
		c := fullChartCusps()
		for i := range c {
			c[i] = float64(int(c[i]+shift) % 360)
		}
		return c
	}
	firstCallNatalPositions = func(_ context.Context, _ *lookup.Probers, c store.AnchorChart, _ lookup.Lens) (lookup.Result, error) {
		return lookup.Result{Positions: shiftedChart(shiftOf(c.When)), Cusps: cuspsFor(shiftOf(c.When))}, nil
	}
	firstCallTransits = func(_ context.Context, _ *lookup.Probers, when time.Time, _ location.PlaceInfo, _ lookup.Lens) (lookup.Result, error) {
		return lookup.Result{Positions: shiftedChart(shiftOf(when) + 11), Cusps: cuspsFor(shiftOf(when) + 11)}, nil
	}
	pair := settledComparing(t, 1)
	pair.cfg = Config{Color: ColorNever}
	pair.width, pair.height = 132, 40
	pair.focused = PanelBodies
	writeDesignFrame(t, "bodies-rx-cusps-pair-132x40-never", pair.View())
}

// the specification rules 1 and 6: with nothing ticked, under the
// rule are the four angles and no plain cusp; the counter, the cursor walk
// and the cursor lookup count exactly those.
func TestPlainCuspsOffByDefault(t *testing.T) {
	cusps := fullChartCusps()
	ps := fullChartPositions()
	state := chartViewState{positions: ps, cusps: cusps}
	body, ruleRow, scroll := renderBodiesBody(state, nil, 0, 0, 80, 40, false, Config{Accessible: true})
	under := underRule(t, body, ruleRow)
	want := []string{"A Asc", "I IC", "D Dsc", "M MC"}
	if len(under) != len(want) {
		t.Fatalf("%d rows under the rule, want the 4 angles:\n%s", len(under), body)
	}
	for i, row := range under {
		if !strings.HasPrefix(row, want[i]) {
			t.Errorf("row %d under the rule = %q, want it to start with %q", i+1, row, want[i])
		}
	}
	total := fullChartPlanets + fullChartAngles
	if got := bodyRowCount(ps, cusps); got != total || scroll.Total != total {
		t.Errorf("bodyRowCount = %d, viewport total = %d, want %d", got, scroll.Total, total)
	}
	if b, ok := bodyAtCursor(ps, cusps, total-1); !ok || b != aspect.Midheaven {
		t.Errorf("last row = (%v, %v), want the Midheaven", b, ok)
	}
	if _, ok := bodyAtCursor(ps, cusps, total); ok {
		t.Errorf("a cursor past the last row resolved to a body: a hidden cusp row is reachable")
	}
}

// Rules 2, 3 and 4: a ticked cusp's object row puts that house's plain cusp
// row under the rule in house order, once, with the value from the parsed
// cusps; its own object row is not also above the rule; unticking removes it.
func TestTickedCuspShowsUnderTheRuleOnce(t *testing.T) {
	cusps := fullChartCusps()
	cfg := Config{Accessible: true}
	ticked := withTicked(fullChartPositions(), cusps, 2, 11)
	state := chartViewState{positions: ticked, cusps: cusps}
	body, ruleRow, _ := renderBodiesBody(state, nil, 0, 0, 80, 40, false, cfg)
	rows := strings.Split(body, "\n")
	under := underRule(t, body, ruleRow)
	want := []string{"A Asc", "2 2nd", "I IC", "D Dsc", "M MC", "11 11th"}
	if len(under) != len(want) {
		t.Fatalf("%d rows under the rule, want %d:\n%s", len(under), len(want), body)
	}
	for i, row := range under {
		if !strings.HasPrefix(strings.TrimLeft(row, " "), want[i]) {
			t.Errorf("row %d under the rule = %q, want it to start with %q", i+1, row, want[i])
		}
	}
	for _, h := range []int{2, 11} {
		if n := strings.Count(body, cuspText(cusps[h-1])); n != 1 {
			t.Errorf("cusp %d's position %q appears %d times, want once (one row per cusp)", h, cuspText(cusps[h-1]), n)
		}
	}
	for _, row := range rows[:ruleRow] {
		if strings.Contains(row, "2nd") || strings.Contains(row, "11th") {
			t.Errorf("a ticked cusp's object row is drawn above the rule: %q", row)
		}
	}
	if got, want := bodyRowCount(ticked, cusps), fullChartPlanets+6; got != want {
		t.Errorf("bodyRowCount = %d, want %d", got, want)
	}
	// The cursor lands on the cusp row and it is no body.
	if _, ok := bodyAtCursor(ticked, cusps, fullChartPlanets+1); ok {
		t.Errorf("the plain cusp row resolved to a body")
	}

	// Unticking 2nd: the popup's next frame no longer holds its object row.
	unticked := withTicked(fullChartPositions(), cusps, 11)
	body, ruleRow, _ = renderBodiesBody(chartViewState{positions: unticked, cusps: cusps}, nil, 0, 0, 80, 40, false, cfg)
	if under := underRule(t, body, ruleRow); len(under) != 5 || strings.Contains(body, cuspText(cusps[1])) {
		t.Errorf("after unticking 2nd, %d rows under the rule (want 5) and its cusp still drawn:\n%s", len(under), body)
	}
}

// Rule 5: comparing shows a ticked cusp's row, each column with its own
// chart's cusp, and counts it.
func TestTickedCuspInComparing(t *testing.T) {
	cusps := fullChartCusps()
	other := fullChartCusps()
	for i := range other {
		other[i] = float64(int(other[i]+97) % 360)
	}
	otherPositions := fullChartPositions()
	for i, p := range otherPositions {
		if h := angleHouse(p.Body); h > 0 {
			otherPositions[i].Longitude = other[h-1]
		}
	}
	state := chartViewState{positions: withTicked(fullChartPositions(), cusps, 2, 11), cusps: cusps}
	cols := []bodyColumn{
		{positions: state.positions, cusps: cusps},
		{positions: withTicked(otherPositions, other, 2, 11), cusps: other},
	}
	view := bodiesView{shown: 2, comparing: true, ok: true, cols: cols, focus: 0}
	body, rule, _ := renderBodiesFor(view, state, nil, 0, 0, 100, 40, false, Config{Accessible: true})
	under := underRule(t, body, rule)
	want := []string{"A Asc", "2 2nd", "I IC", "D Dsc", "M MC", "11 11th"}
	if len(under) != len(want) {
		t.Fatalf("comparing: %d rows under the rule, want %d:\n%s", len(under), len(want), body)
	}
	house := map[string]int{"2 2nd": 2, "11 11th": 11}
	for i, row := range under {
		if !strings.HasPrefix(strings.TrimLeft(row, " "), want[i]) {
			t.Errorf("comparing row %d = %q, want it to start with %q", i+1, row, want[i])
		}
		if h, ok := house[want[i]]; ok {
			a, b := strings.Index(row, cuspText(cusps[h-1])), strings.Index(row, cuspText(other[h-1]))
			if a < 0 || b < 0 || a >= b {
				t.Errorf("comparing cusp %d row %q should read chart 1's %q then chart 2's %q", h, row, cuspText(cusps[h-1]), cuspText(other[h-1]))
			}
		}
	}
	// The cusp rows now carry their ordinal under the rule (acceptance 1 of
	// the specification); the object row stays out above it.
	for _, row := range strings.Split(body, "\n")[:rule] {
		if strings.Contains(row, "2nd") || strings.Contains(row, "11th") {
			t.Errorf("a ticked cusp's object row is drawn as a body row in comparing: %q", row)
		}
	}
	if got, want := bodyRowCount(state.positions, cusps), fullChartPlanets+6; got != want {
		t.Errorf("bodyRowCount = %d, want %d", got, want)
	}
}

// The frame the spec freezes: one column, 132x40, 2nd and 11th ticked.
func TestCuspsTickedFrame(t *testing.T) {
	forceColor(t)
	m := frameModel(t, Config{Color: ColorNever}, 132, 40, 1, 1)
	m.slots.ToggleSlotSelection(1)
	m.chart.columns, m.chart.columnsFor = nil, nil
	m.chart.cusps = fullChartCusps()
	m.chart.positions = withTicked(m.chart.positions, m.chart.cusps, 2, 11)
	m.focused = PanelBodies
	writeDesignFrame(t, "cusps-ticked-one-132x40-never", m.View())
}

// The comparing frame the spec freezes: 132x40, the 2nd ticked, each column
// with its own chart's cusp and the focused chart's frame.
func TestCuspsTickedPairFrame(t *testing.T) {
	forceColor(t)
	stubColumnLookups(t, nil)
	cuspsFor := func(shift float64) aspect.Cusps {
		c := fullChartCusps()
		for i := range c {
			c[i] = float64(int(c[i]+shift) % 360)
		}
		return c
	}
	firstCallNatalPositions = func(_ context.Context, _ *lookup.Probers, c store.AnchorChart, _ lookup.Lens) (lookup.Result, error) {
		cu := cuspsFor(shiftOf(c.When))
		return lookup.Result{Positions: withTicked(shiftedChart(shiftOf(c.When)), cu, 2), Cusps: cu}, nil
	}
	firstCallTransits = func(_ context.Context, _ *lookup.Probers, when time.Time, _ location.PlaceInfo, _ lookup.Lens) (lookup.Result, error) {
		cu := cuspsFor(shiftOf(when) + 11)
		return lookup.Result{Positions: withTicked(shiftedChart(shiftOf(when)+11), cu, 2), Cusps: cu}, nil
	}
	pair := settledComparing(t, 1)
	pair.cfg = Config{Color: ColorNever}
	pair.width, pair.height = 132, 40
	pair.focused = PanelBodies
	writeDesignFrame(t, "cusps-ticked-pair-132x40-never", pair.View())
}
