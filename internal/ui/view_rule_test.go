package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The view rule (operator 2026-10-24, rework of transfer specs/sextile-release/
// academic/view-x-in-y.md): the chart under the cursor is always the one you
// read, and the Aspects tab picks the base it is read against. "X in Y" is X,
// the cursor's chart, in Y, the base, whose houses, house system and zodiac
// frame the comparison. Bold is the cursor everywhere, faint the base. A moment
// is never a base. Fixture as frames_test.go: Anna ①, Ben ②, [Carl ③,] now
// last.

// viewFrame is Anna ①, Ben ②, [Carl ③,] now selected, the cursor on cursor and
// base the chosen base (-1: the own tab), its frame committed through the real
// coordinator path, colours off, Chart focused. bodiesStubs casts every chart
// at its own longitudes and cusps.
func viewFrame(t *testing.T, anchored, cursor, base int) Model {
	t.Helper()
	m := selectedSet(t, anchored, true, cursor)
	m.cfg = Config{Color: ColorNever}
	m.width, m.height = max(m.width, 132), 40
	m.focused = PanelChart
	if base >= 0 {
		m.base = slotAt(m, base)
	}
	m, cmd := m.afterComparisonChange()
	return pump(t, m, cmd)
}

// stubPositions is what bodiesStubs casts for slot s: its natal chart, or the
// sky at the compute's moment.
func stubPositions(m Model, s *store.Slot) []aspect.Position {
	if s.Floating {
		return shiftedChart(shiftOf(transitTimeFromWorkspace(m)) + 11)
	}
	return shiftedChart(shiftOf(s.Time))
}

// stubCusps is the house frame bodiesStubs casts for an anchored slot.
func stubCusps(s *store.Slot) aspect.Cusps {
	c := fullChartCusps()
	for i := range c {
		c[i] = float64(int(c[i]+shiftOf(s.Time)) % 360)
	}
	return c
}

// bodiesTitleOf is the Bodies panel's title as its strip prints it.
func bodiesTitleOf(m Model) string { return m.tabStrips()[PanelBodies].tabs[0].Title }

// bodyLine is the Bodies panel's row of the named body as the panel draws it,
// unfocused.
func bodyLine(t *testing.T, m Model, name string) string {
	t.Helper()
	v := m.bodiesView()
	body, _, _ := renderBodiesFor(v, m.paintedChart(), m.slots, m.cursorIdx, m.bodyCursorIdx, 120, 30, false, m.cfg)
	for _, line := range strings.Split(ansi.Strip(body), "\n") {
		if strings.Contains(line, name) {
			return line
		}
	}
	t.Fatalf("no %s row in the Bodies panel:\n%s", name, ansi.Strip(body))
	return ""
}

// sunCell is a Sun position as a Bodies column prints it.
func sunCell(ps []aspect.Position) string {
	for _, p := range ps {
		if p.Body == aspect.Sun {
			signIdx, deg, min := splitLongitude(p.Longitude)
			return fmt.Sprintf("%s %2d°%02d'", signAbbrevString(signIdx), deg, min)
		}
	}
	return ""
}

// Amendment (a): with the Chart panel focused, `]` and `[` step the Aspects
// tab exactly as they do with Aspects focused, and focus stays on the rows.
// Bodies keeps its own meaning (one tab: nothing), Details attached its own.
func TestBracketKeysFromChartChangeTheAspectsTab(t *testing.T) {
	m := framesFixture(t, 3, 0)
	want := bracket(t, m, "]") // the same press with Aspects focused
	m = send(t, m, runes("]"))
	if m.focused != PanelChart || m.cursorIdx != 0 {
		t.Fatalf("] from Chart: focus %s cursor %d, want Chart and Anna", m.focused, m.cursorIdx)
	}
	if m.base != want.base || m.base != slotAt(m, 1) {
		t.Fatalf("] from Chart: base %v, want Ben (as from Aspects: %v)", m.base, want.base)
	}
	wantTab(t, m, "① in ②", "] from Chart")
	m = send(t, m, runes("["))
	wantTab(t, m, "Aspects", "[ from Chart")
	if m.focused != PanelChart || m.base != nil {
		t.Fatalf("[ from Chart: focus %s base %v, want Chart and no base", m.focused, m.base)
	}

	// Bodies has one tab: the keys leave the base alone there.
	m.focused = PanelBodies
	m = send(t, m, runes("]"))
	if m.base != nil || m.focused != PanelBodies {
		t.Errorf("] from Bodies: base %v focus %s, want no change", m.base, m.focused)
	}
}

// V1: whatever the tab, Bodies lists the chart under the cursor, its column the
// bracketed one, under the active tab's title; on the own tab and on a row
// outside the set it carries one house per row, titled "Bodies".
func TestCursorIsTheChartYouRead(t *testing.T) {
	bodiesStubs(t)
	for cursor := 0; cursor < 4; cursor++ {
		for base := -1; base < 3; base++ {
			if base == cursor {
				continue
			}
			m := viewFrame(t, 3, cursor, base)
			cur := slotAt(m, cursor)
			where := fmt.Sprintf("cursor %d base %d (%s)", cursor, base, activeAspectsTitle(m))
			if got, want := m.chart.positions, stubPositions(m, cur); len(got) == 0 || got[0] != want[0] {
				t.Errorf("%s: Bodies lists %v, want the cursor's %v", where, got[:min(len(got), 1)], want[0])
			}
			if line, want := bodyLine(t, m, "Sun"), "["+sunCell(stubPositions(m, cur)); !strings.Contains(line, want) {
				t.Errorf("%s: the bracketed column of %q is not the cursor's %q", where, line, want)
			}
			if got, want := bodiesTitleOf(m), activeAspectsTitle(m); (base < 0 && got != "Bodies") || (base >= 0 && got != want) {
				t.Errorf("%s: the Bodies title is %q", where, got)
			}
			if m.chart.natalCusps.Valid() != (base >= 0) {
				t.Errorf("%s: a base's house frame rides along %v, want %v", where, m.chart.natalCusps.Valid(), base >= 0)
			}
		}
	}
	// Unselected rows beside a comparing set: the anchored one, and now.
	for _, row := range []int{1, 2} {
		m := selectedSet(t, 2, true, row)
		m.slots.SetSlotSelected(row, false)
		m.cfg = Config{Color: ColorNever}
		m.focused = PanelChart
		m, cmd := m.afterComparisonChange()
		m = pump(t, m, cmd)
		if got, want := m.chart.positions, stubPositions(m, slotAt(m, row)); m.comparing() || len(got) == 0 || got[0] != want[0] {
			t.Errorf("unselected row %d: comparing %v, Bodies lists %v, want the row's own", row, m.comparing(), got[:min(len(got), 1)])
		}
		if bodiesTitleOf(m) != "Bodies" || m.chart.natalCusps.Valid() {
			t.Errorf("unselected row %d: title %q, house frame %v", row, bodiesTitleOf(m), m.chart.natalCusps.Valid())
		}
	}
}

// V2: the pair tab's left chart is the base: it frames the comparison (its
// cusps, its house system), is chart 1 of the cross call and the wheel's outer
// ring, and the title ends in its numeral. The own tab has no base.
func TestTabPicksTheBase(t *testing.T) {
	bodiesStubs(t)
	m := viewFrame(t, 3, 0, 1) // Anna read in Ben
	wantTab(t, m, "1 in 2", "Anna, base Ben")
	sc := m.deriveScene()
	ben := slotAt(m, 1)
	if sc.pair == nil || sc.pair.left != ben || sc.pair.right != slotAt(m, 0) || sc.frame != ben {
		t.Fatalf("pair %+v frame %p, want (Ben, Anna) framed by Ben", sc.pair, sc.frame)
	}
	if m.chart.natalCusps != stubCusps(ben) {
		t.Errorf("the house frame is %v, want Ben's %v", m.chart.natalCusps, stubCusps(ben))
	}
	frame, focus, opts := m.overlayInputs(sc, transitTimeFromWorkspace(m))
	var p overlayPlan
	for _, o := range opts {
		o(&p)
	}
	if frame != anchorOf(ben) || p.crossLeft != anchorOf(ben) || p.synastry != anchorOf(slotAt(m, 0)) || focus != anchorOf(slotAt(m, 0)) {
		t.Errorf("the compute is framed by %+v, cross %+v x %+v, Bodies %+v; want Ben, Ben x Anna, Anna", frame, p.crossLeft, p.synastry, focus)
	}
	if plan, ok := m.wheelPlan(); !ok || plan.mode != wheelPair || plan.inner != anchorOf(ben) {
		t.Errorf("the wheel is %+v, want the bi-wheel with Ben passed first", plan)
	}
	ml := withOwnHouses(m, recipe.Placidus, recipe.Koch, recipe.Campanus)
	if got := ml.comparisonLens().Houses; got != recipe.Koch {
		t.Errorf("the comparison is read in %v, want Ben's Koch", got)
	}
	// The own tab: Anna framed by herself, no base.
	m = viewFrame(t, 3, 0, -1)
	if sc := m.deriveScene(); sc.pair != nil || sc.frame != slotAt(m, 0) || m.chart.natalCusps.Valid() {
		t.Errorf("own tab: pair %+v frame %p house frame %v, want none, Anna, none", sc.pair, sc.frame, m.chart.natalCusps.Valid())
	}
}

// V3: on a pair tab a Bodies row's bold house is the cursor chart's body in the
// base's houses and its faint house the base's own house of that body, both
// ways round.
func TestBoldIsTheCursorFaintIsTheBase(t *testing.T) {
	const venusAnna, venusBen = 20.0, 153.2                                          // Anna's Venus 20 Ari, Ben's 3 Vir
	annaCusps := aspect.Cusps{120, 150, 180, 210, 240, 270, 300, 330, 0, 30, 60, 90} // Leo rising, whole sign
	benCusps := aspect.Cusps{0, 30, 60, 90, 120, 150, 180, 210, 240, 270, 300, 330}  // Aries rising
	stubColumnLookups(t, nil)
	firstCallNatalPositions = func(_ context.Context, _ *lookup.Probers, c store.AnchorChart, _ lookup.Lens) (lookup.Result, error) {
		if c.When.Year() == 1961 { // Anna
			return lookup.Result{Positions: []aspect.Position{{Body: aspect.Venus, Longitude: venusAnna, House: 9}}, Cusps: annaCusps}, nil
		}
		return lookup.Result{Positions: []aspect.Position{{Body: aspect.Venus, Longitude: venusBen, House: 6}}, Cusps: benCusps}, nil
	}
	firstCallTransits = func(context.Context, *lookup.Probers, time.Time, location.PlaceInfo, lookup.Lens) (lookup.Result, error) {
		return lookup.Result{Positions: []aspect.Position{{Body: aspect.Venus, Longitude: 5, House: 4}}, Cusps: benCusps}, nil
	}
	for _, tc := range []struct {
		name         string
		cursor, base int
		bold, faint  aspect.House
	}{
		{"① in ②: Anna's Venus in Ben's houses, Ben's own Venus house", 0, 1, 1, 6},
		{"② in ①: Ben's Venus in Anna's houses, Anna's own Venus house", 1, 0, 2, 9},
		{"③ in ①: now's Venus in Anna's houses, Anna's own Venus house", 2, 0, 9, 9},
	} {
		m := viewFrame(t, 2, tc.cursor, tc.base)
		ov := houseOverlay{cusps: m.chart.natalCusps, positions: m.chart.natalPositions, framed: m.chart.framedPositions}
		faint, bold := ov.houses(m.chart.positions[0])
		if bold != tc.bold || faint != tc.faint {
			t.Errorf("%s: bold %d faint %d, want bold %d faint %d", tc.name, bold, faint, tc.bold, tc.faint)
		}
		if want := ansi.Strip(houseColumn(m.houseDisplay, tc.faint, tc.bold, true, astrologstyle.SGR(""), false)); !strings.Contains(bodyLine(t, m, "Venus"), want) {
			t.Errorf("%s: the Venus row %q does not draw %q", tc.name, bodyLine(t, m, "Venus"), want)
		}
	}
}

// V3, degrees: in a pair view every degree Bodies shows is in the base's
// zodiac, as the wheel draws it; on its own tab each chart keeps its own. Anna
// is tropical, Ben Lahiri.
func TestPairViewInTheBasesZodiac(t *testing.T) {
	const venus, ayanamsa = 153.2, 24.0 // 3 Vir tropical, 9 Leo sidereal
	stubColumnLookups(t, nil)
	firstCallNatalPositions = func(_ context.Context, _ *lookup.Probers, _ store.AnchorChart, l lookup.Lens) (lookup.Result, error) {
		lon := venus
		if l.Zodiac != recipe.Tropical {
			lon -= ayanamsa
		}
		return lookup.Result{Positions: []aspect.Position{{Body: aspect.Venus, Longitude: lon}}}, nil
	}
	for _, tc := range []struct {
		cursor, base int
		title, want  string
	}{
		{0, 1, "1 in 2", "[Leo  9°12'"},   // Anna read in Ben: Lahiri
		{0, -1, "Aspects", "[Vir  3°12'"}, // Anna alone: tropical
		{1, 0, "2 in 1", "[Vir  3°12'"},   // Ben read in Anna: tropical
		{1, -1, "Aspects", "[Leo  9°12'"}, // Ben alone: Lahiri
	} {
		m := withOwnZodiacs(framesFixture(t, 2, tc.cursor), recipe.Tropical, recipe.Lahiri)
		m.cfg = Config{Color: ColorNever}
		if tc.base >= 0 {
			m.base = slotAt(m, tc.base)
		}
		m, cmd := m.afterComparisonChange()
		m = pump(t, m, cmd)
		wantTab(t, m, tc.title, tc.title)
		if line := bodyLine(t, m, "Venus"); !strings.Contains(line, tc.want) {
			t.Errorf("%s on %d: the Venus row %q does not bracket %q", tc.title, tc.cursor, line, tc.want)
		}
	}
}

// V4: a moment is never a base. An anchored row offers one pair per other
// anchored chart and none with now; now's row offers every anchored chart; a
// stored floating base shows no pair.
func TestNoFloatingBaseOnNatalRows(t *testing.T) {
	for cursor, want := range [][]string{
		{"Aspects", "① in ②", "① in ③"},
		{"Aspects", "② in ①", "② in ③"},
		{"Aspects", "③ in ①", "③ in ②"},
		{"Aspects", "④ in ①", "④ in ②", "④ in ③"},
	} {
		m := framesFixture(t, 3, cursor)
		if got := titlesOf(m.panelTabs(PanelAspects)); !slices.Equal(got, want) {
			t.Errorf("cursor %d: tabs %q, want %q", cursor, got, want)
		}
		for _, p := range m.deriveScene().pairs {
			if p.left.Floating {
				t.Errorf("cursor %d: a pair has the floating slot as its base", cursor)
			}
		}
		m.base = slotAt(m, 3)
		if m.activePairSlot() != nil {
			t.Errorf("cursor %d: a stored floating base shows a pair", cursor)
		}
	}
	m := framesFixture(t, 1, 0) // Anna and now only
	if got := titlesOf(m.panelTabs(PanelAspects)); !slices.Equal(got, []string{"Aspects"}) {
		t.Errorf("Anna with now only: tabs %q, want Aspects alone", got)
	}
}

// V5: moving the cursor keeps the base; landing on it makes the chart you came
// from the base, unless that is a moment: then the own tab shows and the base
// is kept.
func TestLandingOnTheBaseSwaps(t *testing.T) {
	m := framesFixture(t, 3, 0)
	m = bracket(t, m, "]") // Anna read in Ben
	for _, step := range []struct {
		key   tea.KeyMsg
		title string
		base  int
	}{
		{keyDown, "② in ①", 0}, // onto Ben, the base: Anna, whom you came from
		{keyDown, "③ in ①", 0}, // Carl: Anna stays
		{keyUp, "② in ①", 0},
		{keyUp, "① in ②", 1}, // onto Anna, the base: Ben
	} {
		m = walk(t, m, step.key, 1)
		wantTab(t, m, step.title, "walking")
		if m.base != slotAt(m, step.base) {
			t.Errorf("%s: base %v, want slot %d", step.title, m.base, step.base)
		}
	}
	// From now's row onto the base: a moment cannot be one, so the own tab.
	m = framesFixture(t, 3, 3)
	m.at = slotAt(m, 3)
	m.base = slotAt(m, 2)
	wantTab(t, m, "④ in ③", "on now, base Carl")
	m = walk(t, m, keyUp, 1)
	wantTab(t, m, "Aspects", "from now onto Carl")
	if m.base != slotAt(m, 2) {
		t.Errorf("from now onto Carl: base %v, want Carl kept", m.base)
	}
	m = walk(t, m, keyUp, 1)
	wantTab(t, m, "② in ③", "on to Ben: Carl is the base again")
}

// Row 6: the cursor on now with a base reads now's bodies in the base, as
// before the rule: the same frame, byte for byte, but for the Bodies title,
// which names the view.
func TestNowRowIsUnchanged(t *testing.T) {
	forceColor(t)
	bodiesStubs(t)
	a := bodiesFrame(t, 2) // the cursor on now: "③ in ①"
	if !a.chart.natalCusps.Valid() {
		t.Fatal("now read in Anna: no house frame")
	}
	if got := bodiesTitleOf(a); got != "3 in 1" {
		t.Errorf("the Bodies title is %q, want 3 in 1", got)
	}
	writeDesignFrame(t, "bodies-now-overlay-132x40-never", a.View())
}

// V6: Space in Bodies marks the cursor chart's body, on a pair tab too; the
// base's own selection is untouched.
func TestSpaceMarksTheCursorChartsBody(t *testing.T) {
	bodiesStubs(t)
	m := viewFrame(t, 2, 0, 1) // Anna read in Ben: "1 in 2"
	m.focused = PanelBodies
	m.bodyCursorIdx = 0
	m, cmd := tpress(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m = pump(t, m, cmd)
	body := m.chart.positions[0].Body
	if !m.slots.BodySelected(0, body) || m.slots.HasBodySelections(1) {
		t.Fatalf("Space on %s in 1 in 2: Anna's marked %v, Ben's any %v; want Anna's alone", bodyName(body), m.slots.BodySelected(0, body), m.slots.HasBodySelections(1))
	}
}

// § 2, 3+ selected: the base carries over every row but its own, and Bodies
// lists the cursor's chart under the tab's title on every row.
func TestThreeSelectedKeepTheBase(t *testing.T) {
	m := framesFixture(t, 3, 0)
	m = bracket(t, m, "]")
	m = bracket(t, m, "]") // Anna read in Carl
	wantTab(t, m, "① in ③", "chosen")
	for _, step := range []struct {
		key   tea.KeyMsg
		title string
	}{
		{keyDown, "② in ③"}, // Ben: Carl stays
		{keyDown, "③ in ②"}, // onto Carl, the base: Ben
		{keyDown, "④ in ②"}, // now: Ben stays
		{keyUp, "③ in ②"},
		{keyUp, "② in ③"}, // onto Ben, the base: Carl
		{keyUp, "① in ③"},
	} {
		m = walk(t, m, step.key, 1)
		wantTab(t, m, step.title, "walking")
		if got := bodiesTitleOf(m); got != step.title {
			t.Errorf("%s: the Bodies title is %q", step.title, got)
		}
		if sc := m.deriveScene(); sc.shown[sc.focus] != sc.cursor {
			t.Errorf("%s: the Bodies column is not the cursor's", step.title)
		}
	}
}

// § 3: `]` changes the base and nothing else; `0` changes neither the base
// nor the tab.
func TestTabChangeMovesOnlyTheBase(t *testing.T) {
	for cursor := 0; cursor < 4; cursor++ {
		m := framesFixture(t, 3, cursor)
		selection := slices.Clone(m.slots.SelectedInOrder())
		floating := m.slots.Floating().Time
		for press := 0; press < len(m.panelTabs(PanelAspects)); press++ {
			before := m.base
			m, _ = tpress(t, m, runes("]"))
			if m.base == before {
				t.Errorf("cursor %d press %d: ] left the base %v", cursor, press, before)
			}
			if m.cursorIdx != cursor || m.focused != PanelChart || !slices.Equal(m.slots.SelectedInOrder(), selection) ||
				!m.slots.Floating().Time.Equal(floating) {
				t.Errorf("cursor %d press %d: ] moved the cursor, focus, selection or floating time", cursor, press)
			}
			b, title := m.base, activeAspectsTitle(m)
			m, _ = tpress(t, m, runes("0"))
			if m.base != b || activeAspectsTitle(m) != title {
				t.Errorf("cursor %d press %d: 0 changed the view from %q to %q", cursor, press, title, activeAspectsTitle(m))
			}
		}
	}
}
