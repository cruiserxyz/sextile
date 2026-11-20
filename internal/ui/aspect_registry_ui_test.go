package ui

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// aspect_registry_ui_test.go: an aspect is what Astrolog's list says (the
// specification), seen from the screen.

var objMercuryFixture = aspect.Object{Number: 3, Name: "Mercury"}

// Acceptance 1: Kepler's septile, as `astrolog -RA0 13 -aa` lists it (Mercury,
// Saturn, 0°20', separating), is a cell that reads `☿Sep♄ (0°20'S)` in full:
// the two glyphs and the three letters with no space, the width of any cell.
func TestSeptileCellShowsItsLettersAndOrb(t *testing.T) {
	orb := aspect.Orb(float64(0) + float64(20)/60.0)
	sep := aspect.NewObjectAspect(objMercuryFixture, objSaturn, kindOf("Sep"), orb)
	if got := strings.TrimSpace(formatAspectEntry(sep)); got != "☿Sep♄ (0°20'S)" {
		t.Errorf("single list cell = %q, want %q", got, "☿Sep♄ (0°20'S)")
	}
	cross := lookup.OverlayAspect{TransitObject: objSaturn, NatalObject: objMercuryFixture, Kind: kindOf("Sep"), Orb: orb}
	if got := strings.TrimSpace(formatCrossAspectEntry(cross)); got != "♄Sep☿ (0°20'S)" {
		t.Errorf("pair tab cell = %q, want %q", got, "♄Sep☿ (0°20'S)")
	}
	out := ansi.Strip(renderAspectWaterfall([]aspect.Aspect{sep}, 40, 1))
	if !strings.HasPrefix(out, "☿Sep♄ (0°20'S)") {
		t.Errorf("waterfall = %q, want it to start with the whole septile cell", out)
	}
}

// Acceptance 7: the cell of an aspect with a glyph keeps the width it had, so a
// list of the classical eleven draws as before; no cell is ever wider.
func TestClassicalCellsKeepTheirWidth(t *testing.T) {
	for _, ab := range []string{"Con", "Opp", "Squ", "Tri", "Sex", "Inc", "SSx", "SSq", "Ses", "Qui", "BQn"} {
		a := aspect.NewObjectAspect(objSun, objSaturn, kindOf(ab), aspect.Orb(1.55))
		if w := ansi.StringWidth(formatAspectEntry(a)); w != panel.AspectColWidth {
			t.Errorf("%s: cell is %d cells wide, want %d", ab, w, panel.AspectColWidth)
		}
	}
}

// Acceptance 3: Enter on a Timeline row of a septile marks the cell of that
// septile in Aspects, as it does for a square, and no other cell.
func TestEnterOnASeptileRowMarksItsCell(t *testing.T) {
	cfg := Config{Color: ColorNever}
	rows := enteredRows(t)
	i := -1
	for j, r := range rows {
		if r.Numeral == 0 && strings.Contains(r.Line, "Tri (Leo) Mars") {
			i = j
		}
	}
	if i < 0 {
		t.Fatal("fixture: no sky Moon trine Mars row")
	}
	// The same row, as Astrolog prints a septile in its place.
	rows[i].Line = strings.Replace(rows[i].Line, " Tri ", " Sep ", 1)
	rows[i].Aspect, rows[i].AspectID = "Sep", kindOf("Sep")
	m, _ := markFixtureRows(t, rows, cfg)
	m = withSpawns(m, func(s *rowSpawns) {
		inner := s.cast.transit
		s.cast.transit = func(ctx context.Context, p *lookup.Probers, when time.Time, place location.PlaceInfo, lens lookup.Lens) (lookup.Result, error) {
			res, err := inner(ctx, p, when, place, lens)
			res.Aspects = append(res.Aspects,
				aspect.Aspect{AObject: objMoon, BObject: objMars, Kind: kindOf("Sep"), Orb: 0, Power: 7},
				aspect.Aspect{AObject: objMoon, BObject: objMars, Kind: kindOf("Un1"), Orb: 0.1, Power: 6})
			return res, err
		}
	})
	m = enter(t, m, timelineRowWith(t, m.timeline.rows, "Moon (Sag) Sep (Leo) Mars"))
	_, aspects := leftPanels(t, m.View())
	got := boldSpanTexts(aspects)
	want := panel.BodyGlyph(objMoon) + "Sep" + panel.BodyGlyph(objMars) + " "
	if len(got) != 1 || !strings.HasPrefix(got[0], want) {
		t.Errorf("bold cells = %q, want exactly the Moon-septile-Mars cell\n%s", got, strings.Join(aspects, "\n"))
	}
}

// Acceptance 6a: with the aspect list not yet read, no chart is computed. The
// object list lands first and the start still waits; when the aspect list
// lands the start goes on, and the lens that casts the first chart carries it.
func TestStartWaitsForTheAspectList(t *testing.T) {
	var tm tea.Model = withoutAspectList(withoutObjectList(newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen))))
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(perimeter())
	tm, _ = tm.Update(ObjectRegistryLoadedMsg{Registry: drawnRegistry()})
	m := tm.(Model)
	if m.slots != nil {
		t.Fatal("the workspace was built before the aspect list arrived")
	}
	if got := argvTexts(m); len(got) != 0 {
		t.Fatalf("Astrolog was called before the aspect list arrived:\n%s", strings.Join(got, "\n"))
	}
	tm, _ = tm.Update(AspectRegistryLoadedMsg{Registry: testAspects().Rows()})
	m = tm.(Model)
	if m.slots == nil {
		t.Fatal("the workspace was not built when the aspect list arrived")
	}
	if !m.lens.AspectRegistry.Ready() || len(m.lens.AspectRegistry.Rows()) != 24 {
		t.Errorf("the lens carries %d aspects, want the list's 24", len(m.lens.AspectRegistry.Rows()))
	}
}

// The aspect list arriving before the object list is a start that waits for the
// objects, as it did, and goes on when they land.
func TestStartWaitsForTheObjectListWhenTheAspectListIsIn(t *testing.T) {
	var tm tea.Model = withoutAspectList(withoutObjectList(newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen))))
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(perimeter())
	tm, _ = tm.Update(AspectRegistryLoadedMsg{Registry: testAspects().Rows()})
	if tm.(Model).slots != nil {
		t.Fatal("the workspace was built before the object list arrived")
	}
	tm, _ = tm.Update(ObjectRegistryLoadedMsg{Registry: drawnRegistry()})
	if tm.(Model).slots == nil {
		t.Fatal("the workspace was not built once both lists were in")
	}
}

// An aspect list that cannot be read stops the start on the can't-start screen:
// without it no printed aspect row can be read.
func TestUnreadableAspectListCannotStart(t *testing.T) {
	sessionSandbox(t)
	var tm tea.Model = withoutAspectList(withoutObjectList(newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen))))
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(perimeter())
	tm, _ = tm.Update(ObjectRegistryLoadedMsg{Registry: drawnRegistry()})
	tm, _ = tm.Update(AspectRegistryLoadedMsg{Err: errTest("astrolog: no aspect rows found")})
	m := tm.(Model)
	want := "Sextile can't start: Astrolog's list of aspects could not be read: astrolog: no aspect rows found"
	if got := csText(m.View()); !strings.Contains(got, want) {
		t.Errorf("the screen lacks %q:\n%s", want, got)
	}
	if m.slots != nil {
		t.Error("a workspace was built without the aspect list")
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }

// Acceptance 6a and 5, through the whole lookup: with the aspect list in, an
// aspect the binary prints beyond the classical eleven keeps its row, so the
// first Aspects list is complete; a row whose letters the list does not hold,
// or a lens that has no aspect list yet, refuses the call and no partial list
// comes back.
func TestFirstAspectsListIsCompleteOrRefused(t *testing.T) {
	where := location.PlaceInfo{Name: "Constructed Test Place", Lat: 48.13, Lon: 11.57, TZ: "Etc/UTC"}
	when := time.Date(1985, 6, 15, 12, 0, 0, 0, time.UTC)
	first := func(lens lookup.Lens, transform func(string) string) (lookup.Result, error) {
		fixtureAstrolog(t, transform)
		return lookup.FirstCallTransits(context.Background(), nil, when, where, lens)
	}
	baseline, err := first(lookup.Hellenistic, nil)
	if err != nil || len(baseline.Aspects) == 0 {
		t.Fatalf("baseline: %d aspects, err %v", len(baseline.Aspects), err)
	}
	septile, err := first(lookup.Hellenistic, func(in string) string { return strings.ReplaceAll(in, "Con ", "Sep ") })
	if err != nil {
		t.Fatal(err)
	}
	kept := 0
	for _, a := range septile.Aspects {
		if a.Kind == kindOf("Sep") {
			kept++
		}
	}
	if len(septile.Aspects) != len(baseline.Aspects) || kept == 0 {
		t.Errorf("with the septile printed: %d aspects of %d, %d of them septiles; want every row kept", len(septile.Aspects), len(baseline.Aspects), kept)
	}
	if res, err := first(lookup.Hellenistic, func(in string) string { return strings.ReplaceAll(in, "Con ", "Xyz ") }); err == nil || !strings.Contains(err.Error(), `"Xyz"`) || len(res.Aspects) != 0 {
		t.Errorf("a row of unknown letters: %d aspects, err %v; want the call refused, naming Xyz", len(res.Aspects), err)
	}
	early := lookup.Hellenistic
	early.AspectRegistry = aspect.AspectRegistry{}
	if res, err := first(early, nil); err == nil || len(res.Aspects) != 0 {
		t.Errorf("a lens with no aspect list: %d aspects, err %v; want the call refused", len(res.Aspects), err)
	}
}

// Acceptance 2 and 4 at the panel's output: every aspect from 12 to 24 has its
// rows in the single list and on a pair tab, drawn by its letters and in the
// colour Astrolog gave its kind, the septile included. The colour is read at
// the rendered text, not at the parser.
func TestAspectsBeyondTheClassicalElevenRenderWithTheirColour(t *testing.T) {
	const kindStyle = "\x1b[1;36m"
	const kindSpan = "\x1b[96m" // the style, as the panel draws it
	var own []aspect.Aspect
	var cross []lookup.OverlayAspect
	styles := astrologstyle.Aspects{}
	crossStyles := astrologstyle.Aspects{}
	var letters []string
	for _, row := range testAspects().Rows() {
		if row.Kind.Index < 12 {
			continue
		}
		letters = append(letters, row.Kind.Abbrev)
		a := aspect.NewObjectAspect(objSun, objMoon, row.Kind, aspect.Orb(1.5))
		a.Power = float64(100 - row.Kind.Index)
		own = append(own, a)
		x := lookup.OverlayAspect{TransitObject: objMars, NatalObject: objMoon, Kind: row.Kind, Orb: 1, Power: float64(100 - row.Kind.Index)}
		cross = append(cross, x)
		styles[astrologstyle.AspectIdentity{First: objSun, Second: objMoon, Kind: row.Kind}] = astrologstyle.AspectStyle{First: "\x1b[1;31m", Kind: kindStyle, Second: "\x1b[1;32m"}
		crossStyles[astrologstyle.AspectIdentity{First: objMoon, Second: objMars, Kind: row.Kind}] = astrologstyle.AspectStyle{First: "\x1b[1;34m", Kind: kindStyle, Second: "\x1b[1;31m"}
	}
	if len(letters) != 13 {
		t.Fatalf("the fixture registry has %d aspects from 12 to 24, want 13", len(letters))
	}
	cfg := Config{Color: ColorAlways}
	for name, got := range map[string]string{
		"single list": panel.RenderAspectWaterfallStyled(own, styles, 240, 13, cfg),
		"pair tab":    panel.RenderCrossAspectWaterfallStyled(cross, crossStyles, 240, 13, cfg),
	} {
		plain := ansi.Strip(got)
		for _, l := range letters {
			if !strings.Contains(plain, l) {
				t.Errorf("%s: no row shows %s:\n%s", name, l, plain)
			}
			if !strings.Contains(got, kindSpan+l+"\x1b[0m") {
				t.Errorf("%s: %s is not drawn in Astrolog's colour %q", name, l, kindSpan)
			}
		}
	}
}

// misfitAspects is the aspect registry of a host whose Septile is renamed
// "S:X": read in full, with that aspect recorded as one no list can be read with.
func misfitAspects(t *testing.T) (aspect.AspectRegistry, []aspect.RegistryAspect) {
	t.Helper()
	b, err := os.ReadFile("../astrolog/testdata/ha-default.txt")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := astrolog.ParseAspectRegistry(strings.Replace(string(b), "(Sep)", "(S:X)", 1))
	if err != nil {
		t.Fatal(err)
	}
	reg := lookup.NewAspectRegistry(rows)
	if reg.Refusal() == "" {
		t.Fatal("the registry holds no unreadable aspect")
	}
	return reg, rows
}

const septileRefusal = `Can't read the aspect list. Septile is renamed "S:X" in your Astrolog settings.`

// D1: an aspect renamed to letters no list can be read with does not stop the
// start. Bodies work; every aspect list says why it has no rows.
func TestRenamedAspectStartsAndRefusesEveryAspectList(t *testing.T) {
	bad, rows := misfitAspects(t)

	// The start succeeds when the registry lands.
	var tm tea.Model = withoutAspectList(withoutObjectList(newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen))))
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(perimeter())
	tm, _ = tm.Update(ObjectRegistryLoadedMsg{Registry: drawnRegistry()})
	tm, _ = tm.Update(AspectRegistryLoadedMsg{Registry: rows})
	if started := tm.(Model); started.slots == nil || started.cantStart.any() {
		t.Fatalf("the start did not succeed with a renamed aspect: slots %v, can't start %v", started.slots != nil, started.cantStart.any())
	} else if started.lens.AspectRegistry.Refusal() != septileRefusal {
		t.Fatalf("the lens refusal = %q", started.lens.AspectRegistry.Refusal())
	}

	// Bodies show positions; the Aspects panel says why on its own tab and on
	// every pair tab.
	m, _ := markFixture(t, Config{Color: ColorNever})
	m = enter(t, m, timelineRowWith(t, m.timeline.rows, "Opp natal (Ari) Venus"))
	m.lens.AspectRegistry = bad
	view := m.View()
	bodies, aspects := leftPanels(t, view)
	if !strings.Contains(strings.Join(bodies, "\n"), panel.BodyGlyph(objSun)) {
		t.Errorf("Bodies shows no positions:\n%s", strings.Join(bodies, "\n"))
	}
	seen := map[string]bool{}
	for range m.panelTabs(PanelAspects) {
		title := activeAspectsTitle(m)
		seen[title] = true
		_, aspects = leftPanels(t, m.View())
		text := ansi.Strip(strings.Join(aspects, " "))
		for _, want := range []string{"Can't read the aspect list", "Septile", `"S:X"`} {
			if !strings.Contains(text, want) {
				t.Errorf("tab %q lacks %q:\n%s", title, want, strings.Join(aspects, "\n"))
			}
		}
		m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	}
	if len(seen) < 2 {
		t.Errorf("the check saw tabs %v, want the own list and a pair tab", seen)
	}
}

// D1: the Timeline refuses with the same sentence, from the lookup, before any
// search starts.
func TestRenamedAspectRefusesTheTimeline(t *testing.T) {
	bad, _ := misfitAspects(t)
	searches := 0
	stubTimeline(t, func(req lookup.TimelineRequest) (lookup.TimelineResult, error) {
		return lookup.TimelineWith(context.Background(), req, func(ctx context.Context, c lookup.BodyCall) (string, string, error) {
			searches++
			return weekSearch(ctx, c)
		}, func(ctx context.Context) (lookup.DefaultSets, error) {
			searches++ // the default-set read is a spawn too
			return stubDefaults(ctx)
		})
	})
	m := timelineModel(t, Config{Accessible: true})
	m.lens.AspectRegistry = bad
	m.width = max(m.width, dualColumnMinWidth(m.shownCount()))
	m, cmd := syncListVisited(m, listTimeline)
	m = pump(t, m, cmd)
	if searches != 0 {
		t.Errorf("%d searches started for a list that refuses", searches)
	}
	found := false
	for _, l := range logTexts(m) {
		if l == `aspects: the aspect list was not read, Septile is renamed "S:X" in your Astrolog settings` {
			found = true
		}
	}
	if !found {
		t.Errorf("the Log lacks %q: %v", septileRefusal, logTexts(m))
	}
}

// Acceptance 2 at the Timeline's display: rows of every aspect from 12 to 24
// pass through the lookup and the list, and each shows in the rendered Timeline.
func TestTimelineShowsARowOfEveryAspectBeyondTheClassicalEleven(t *testing.T) {
	var lines, letters []string
	for i, r := range testAspects().Rows() {
		if r.Kind.Index < 12 {
			continue
		}
		letters = append(letters, r.Kind.Abbrev)
		lines = append(lines, fmt.Sprintf("Thu 10/15/2026 %2d:00    Moon (Leo) %s [Ari] Neptune\n", 1+i%20, r.Kind.Abbrev))
	}
	sky := strings.Join(lines, "")
	stubTimeline(t, func(req lookup.TimelineRequest) (lookup.TimelineResult, error) {
		return lookup.TimelineWith(context.Background(), req, func(_ context.Context, c lookup.BodyCall) (string, string, error) {
			argv := c.Argv()
			cmd := "astrolog " + strings.Join(argv, " ")
			if argv[1] != "-2" || slices.Contains(argv, "-tr") || slices.Contains(argv, "-t") {
				return "", cmd, nil
			}
			if slices.Contains(argv, "-dY") {
				return "", cmd, nil
			}
			return sky, cmd, nil
		}, stubDefaults)
	})
	m := timelineModel(t, Config{Accessible: true})
	m.width, m.height = max(m.width, dualColumnMinWidth(m.shownCount())), 80
	m, cmd := syncListVisited(m, listTimeline)
	m = pump(t, m, cmd)
	m, cmd = openList(t, m, listTimeline)
	m = pump(t, m, cmd)
	if got := len(m.list(listTimeline).rows); got != len(letters) {
		t.Fatalf("the list holds %d rows, want %d (one per aspect from 12 to 24): %v", got, len(letters), logTexts(m))
	}
	text := strings.Join(detailsBodyRows(renderDetails(m)), "\n")
	for _, l := range letters {
		if !strings.Contains(text, " "+l+" ") {
			t.Errorf("the rendered Timeline shows no %s row:\n%s", l, text)
		}
	}
}
