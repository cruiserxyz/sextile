package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The `/` line's modes and the pickers' one ranking rule.

// timelineAfter opens the timeline's line and types q, which includes any mode
// slashes after the opening one.
func timelineAfter(t *testing.T, q string) Model {
	t.Helper()
	m := onTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, runes("/"))
	return typeQuery(t, m, q)
}

// `/` is an exact substring ignoring case, `//` is fuzzy, `///` is an exact
// substring respecting case.
func TestSlashModesOnTheTimeline(t *testing.T) {
	all := len(onTimeline(t, Config{Accessible: true}).timeline.rows)

	exact := shownLines(timelineAfter(t, "sun"))
	if len(exact) == 0 || len(exact) >= all {
		t.Fatalf("fixture: `/sun` keeps %d of %d rows, want a proper subset", len(exact), all)
	}
	for _, l := range exact {
		if !strings.Contains(strings.ToLower(l), "sun") {
			t.Errorf("`/sun` kept %q, which holds no `sun`", l)
		}
	}

	fuzzy := shownLines(timelineAfter(t, "/sun"))
	if len(fuzzy) <= len(exact) {
		t.Errorf("`//sun` keeps %d rows, want more than the %d of the exact search", len(fuzzy), len(exact))
	}

	cased := shownLines(timelineAfter(t, "//Sun"))
	if len(cased) == 0 {
		t.Fatal("fixture: no row holds `Sun` with a capital")
	}
	for _, l := range cased {
		if !strings.Contains(l, "Sun") {
			t.Errorf("`///Sun` kept %q, which holds no `Sun`", l)
		}
	}
	if lower := shownLines(timelineAfter(t, "//sun")); len(lower) != 0 {
		t.Errorf("`///sun` keeps %d rows, want none: the case counts there", len(lower))
	}
}

// The open line shows the mode's slashes and never `Search:`.
func TestOpenLineShowsTheModeSlashes(t *testing.T) {
	for q, want := range map[string]string{"": "/█", "sun": "/sun█", "/sun": "//sun█", "//Sun": "///Sun█"} {
		m := timelineAfter(t, q)
		rows := detailsBodyRows(renderDetails(m))
		last := rows[len(rows)-1]
		if !strings.HasPrefix(last, want) || strings.Contains(last, "Search:") {
			t.Errorf("query %q: last row = %q, want it to start with %q and not say Search:", q, last, want)
		}
	}
}

// filterCells is a cross list of n cells in falling power, most with the Sun on
// the transit side and each fourth with Mars there.
func filterCells(n int) []lookup.OverlayAspect {
	natal := []aspect.Body{aspect.Moon, aspect.Venus, aspect.Jupiter, aspect.Mercury, aspect.Neptune, aspect.Pluto}
	var list []lookup.OverlayAspect
	for i := 0; i < n; i++ {
		transit := aspect.Sun
		if i%4 == 3 {
			transit = aspect.Mars
		}
		list = append(list, lookup.OverlayAspect{Transit: transit, Natal: natal[i%len(natal)], Kind: aspect.Square, Orb: aspect.Orb(i % 9), Power: float64(1000 - i)})
	}
	return list
}

// aspectsFocused is markFixture on `2 vs 3` with the Aspects panel focused and
// the given cross list for every chart.
func aspectsFocused(t *testing.T, list []lookup.OverlayAspect) Model {
	t.Helper()
	m, _ := markFixture(t, Config{Color: ColorNever})
	overlayCrossAspects = func(_ context.Context, _ lookup.Lens, _ store.AnchorChart, _ time.Time, _ location.PlaceInfo) (lookup.OverlayAspects, error) {
		return lookup.OverlayAspects{Aspects: list}, nil
	}
	m = enter(t, m, timelineRowWith(t, m.timeline.rows, "Opp natal (Ari) Venus"))
	m, _ = press(t, m, "3")
	if m.focused != PanelAspects || m.details.attached {
		t.Fatalf("fixture: focused=%v attached=%v, want the Aspects panel focused", m.focused, m.details.attached)
	}
	m.width, m.height = 132, 40
	return m
}

// barLine is the key bar, the last line of the frame.
func barLine(m Model) string {
	lines := strings.Split(strings.TrimRight(ansi.Strip(m.View()), "\n"), "\n")
	return lines[len(lines)-1]
}

// `/` with the Aspects panel focused opens nothing and changes nothing.
func TestSlashOnTheAspectsPanelDoesNothing(t *testing.T) {
	m := aspectsFocused(t, filterCells(40))
	before := m.View()
	got, cmd := press(t, m, "/")
	if cmd != nil {
		t.Errorf("`/` returned a command")
	}
	if got.currentOwner() != ownerMain {
		t.Errorf("owner after `/` = %v, want the main screen's", got.currentOwner())
	}
	if after := got.View(); after != before {
		t.Errorf("the frame changed after `/` on the Aspects panel:\nbefore\n%s\nafter\n%s", ansi.Strip(before), ansi.Strip(after))
	}
	if strings.Contains(barLine(m), "Filter: /") {
		t.Errorf("the bar still offers `/` on the Aspects panel: %q", barLine(m))
	}
}

func bodyNames(st bodyModalState) []string {
	var out []string
	for _, o := range st.matches {
		out = append(out, o.Name)
	}
	return out
}

// Typing `ma` in the bodies popup ranks Mars above the loose matches and puts
// the cursor on it, below the group rows.
func TestBodiesPopupRanksAndLandsOnTheBestMatch(t *testing.T) {
	reg := []aspect.RegistryObject{
		{Number: 20, Name: "Dharma"},
		{Number: 21, Name: "Moon Satellite"},
		{Number: 4, Name: "Mars"},
		{Number: 22, Name: "Lunar Mass"},
	}
	st := openBodyModal(reg, recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)
	st.cursor = groupRows() + 3
	for _, r := range "ma" {
		st, _ = updateBodyModalKey(st, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}, 10)
	}
	got := bodyNames(st)
	if len(got) == 0 || got[0] != "Mars" {
		t.Fatalf("`ma` ranks %v, want Mars first", got)
	}
	if st.cursor != groupRows() {
		t.Errorf("cursor = %d, want %d: the first match below the group rows", st.cursor, groupRows())
	}
	if want := []string{"Mars", "Dharma", "Lunar Mass", "Moon Satellite"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("`ma` ranks %v, want %v (starts-with, contains, then loose in list order)", got, want)
	}
}

// Typing `pla` in the house finder puts the cursor on Placidus.
func TestHouseFinderCursorLandsOnTheBestMatch(t *testing.T) {
	m := finderModel(t, 1, 0)
	m = openFinder(t, m)
	// Park the cursor away from the top so a kept index would show.
	for i := 0; i < 3; i++ {
		m, _ = tpress(t, m, keyDown)
	}
	m = typeInFinder(t, m, "pla")
	f := m.houseFinder
	if f.cursor != 0 || len(f.matches) == 0 || f.matches[f.cursor] != recipe.Placidus {
		t.Errorf("after `pla` the cursor is %d on %v, want 0 on Placidus", f.cursor, finderNames(m))
	}
	if shown := visibleCursorRow(t, m); shown != "Placidus" {
		t.Errorf("the screen marks %q, want Placidus", shown)
	}
}

// The zodiac finder ranks by the same rule and lands on the best match too.
func TestZodiacFinderCursorLandsOnTheBestMatch(t *testing.T) {
	m := finderModel(t, 1, 0)
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyCtrlZ})
	if m.startup != startupZodiacOpen {
		t.Fatalf("Ctrl+Z did not open the zodiac finder: %q", m.startup)
	}
	for i := 0; i < 3; i++ {
		m, _ = tpress(t, m, keyDown)
	}
	for _, r := range "ra" {
		m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	f := m.zodiacFinder
	if f.cursor != 0 || len(f.matches) == 0 || f.matches[0].Name() != "Raman" {
		t.Errorf("after `ra` the cursor is %d over %d matches, want 0 on Raman", f.cursor, len(f.matches))
	}
}

// An exact city name ranks that city first, and the cursor follows it.
func TestCityPickerRanksTheExactNameFirst(t *testing.T) {
	m := NewModel(Config{Accessible: true})
	m.startup = startupAtlasOpen
	m.atlas.index = location.NewIndex([]location.PlaceInfo{
		{Name: "Parisot", Country: "FR"},
		{Name: "Paris Hill", Country: "US"},
		{Name: "Paris", Country: "FR"},
	})
	type_ := func(s string) {
		for _, r := range s {
			tm, _ := updateAtlasModalKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = tm.(Model)
		}
	}
	type_("par")
	tm, _ := updateAtlasModalKey(m, keyDown)
	m = tm.(Model)
	type_("is")
	if got := m.atlas.matches; len(got) == 0 || got[0].Name != "Paris" {
		t.Fatalf("`paris` ranks %v, want the exact name first", got)
	}
	if m.atlas.cursor != 0 {
		t.Errorf("cursor = %d, want 0 on the best match", m.atlas.cursor)
	}
}

// The chart picker ranks the exact name first and the cursor follows the best
// match after each keystroke.
func TestChartPickerCursorLandsOnTheBestMatch(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	m.startup = startupChartOpen
	m.picker = chartPickerState{prior: startupReady, loaded: true}
	m.picker.charts = []store.NamedChart{
		testNamedChart("Old Carl Reed", fixtureOneP),
		testNamedChart("Carlton Reed", fixtureOneP),
		testNamedChart("Carl", berlinP),
	}
	m.picker.refilter()
	type_ := func(s string) {
		for _, r := range s {
			tm, _ := updateChartModalKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = tm.(Model)
		}
	}
	type_("car")
	tm, _ := updateChartModalKey(m, keyDown)
	m = tm.(Model)
	type_("l")
	if got := pickerNames(m.picker.matches); len(got) != 3 || got[0] != "Carl" || got[1] != "Carlton Reed" {
		t.Fatalf("`carl` ranks %v, want Carl, Carlton Reed, Old Carl Reed", got)
	}
	if m.picker.cursor != 0 {
		t.Errorf("cursor = %d, want 0 on the best match", m.picker.cursor)
	}
}
