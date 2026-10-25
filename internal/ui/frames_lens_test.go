package ui

import (
	"context"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Each chart keeps its own lens (operator, 2026-10-24, the Saturday try steps
// 12-15). A chart's own rows are cast in its own house system and zodiac, in a
// group or not; a comparison is computed in the frame's, the chart you are on;
// the timeline asks in the lens of the chart under the cursor; a wheel render
// for a chart the cursor has left never replaces the picture on screen; the
// finders name, and change, the focused chart.

// withOwn sets one field of slot i's saved record, keeping the other.
func withOwn(m Model, i int, set func(*store.FrameRecord)) Model {
	s := slotAt(m, i)
	file := slotFile(s)
	rec, ok := m.frames.find(file, s.Floating)
	if !ok {
		rec = store.FrameRecord{File: file, When: s.Time, Floating: s.Floating, From: store.FrameFromChosen}
	}
	set(&rec)
	m.frames = m.frames.with(rec)
	return m
}

// withOwnHouses gives slot i (list index) the chosen house system hs[i], as a
// saved record in the session: a chart's own system.
func withOwnHouses(m Model, hs ...recipe.HouseSystem) Model {
	for i, h := range hs {
		m = withOwn(m, i, func(r *store.FrameRecord) { r.Houses, r.HasHouses = h, true })
	}
	return m
}

// withOwnZodiacs gives slot i (list index) the chosen zodiac zs[i].
func withOwnZodiacs(m Model, zs ...recipe.Zodiac) Model {
	for i, z := range zs {
		m = withOwn(m, i, func(r *store.FrameRecord) { r.Zodiac, r.HasZodiac = z.RecordName(), true })
	}
	return m
}

// Operator, 2026-10-24: "changing all group charts is not wanted! only the
// focused chart should have it ... the other slots need to keep 'their' house
// system all the time ... same for lahiri". A choice changes the focused chart
// alone; each chart's own rows are cast in its own; a comparison carries the
// frame's house system and zodiac.
func TestEachChartKeepsItsOwnSystem(t *testing.T) {
	sessionSandbox(t)
	spy := stubFrameLookups(t)
	m := framesFixture(t, 2, 1) // Anna ①, Ben ②, now ③, all selected; on Ben
	m = withOwnHouses(m, recipe.WholeSign, recipe.WholeSign)
	m = settleHouses(t, m)

	m = chooseInFinder(t, m, keyCtrlHMain, "placidus")
	if got := m.ownHouses(slotAt(m, 1)).hs; got != recipe.Placidus {
		t.Fatalf("Ctrl+H Placidus on Ben: Ben's own system is %v", got)
	}
	if got := m.ownHouses(slotAt(m, 0)).hs; got != recipe.WholeSign {
		t.Errorf("Ctrl+H on Ben changed Anna's own system to %v", got)
	}
	if got := m.rowTag(slotAt(m, 0)); got != "WhTr" {
		t.Errorf("Ctrl+H on Ben: Anna's row tag = %q, want her own WhTr", got)
	}
	if got := m.rowTag(slotAt(m, 1)); got != "PlTr" {
		t.Errorf("Ctrl+H on Ben: Ben's row tag = %q, want PlTr", got)
	}

	m = walk(t, m, keyUp, 1) // Anna
	m = chooseInFinder(t, m, keyCtrlZ, "lahiri")
	if got := m.ownZodiac(slotAt(m, 0)); got != recipe.Lahiri {
		t.Fatalf("Ctrl+Z Lahiri on Anna: Anna's own zodiac is %v", got)
	}
	if got := m.ownZodiac(slotAt(m, 1)); got != recipe.Tropical {
		t.Errorf("Ctrl+Z on Anna changed Ben's zodiac to %v", got)
	}
	cols, ok := m.bodyColumnsForScene()
	if !ok || len(cols) != 3 {
		t.Fatalf("no comparing columns: ok=%v n=%d", ok, len(cols))
	}
	sun := func(c bodyColumn) float64 {
		for _, p := range c.positions {
			if p.Body == aspect.Sun {
				return p.Longitude
			}
		}
		return -1
	}
	if got, want := sun(cols[0]), zodiacLongitude(slotAt(m, 0).Time.Year(), recipe.Lahiri); got != want {
		t.Errorf("Anna's Sun is at %v, want her own sidereal %v", got, want)
	}
	if got, want := sun(cols[1]), zodiacLongitude(slotAt(m, 1).Time.Year(), recipe.Tropical); got != want {
		t.Errorf("Ben's Sun is at %v, want his own tropical %v", got, want)
	}

	// The comparison call of a pair tab carries the base's house system and
	// zodiac: Ben's on "① in ②", Anna's on "② in ①".
	comparison := func(m Model, where string, hs recipe.HouseSystem, z recipe.Zodiac) {
		t.Helper()
		saw := false
		for _, c := range spy.snapshot() {
			if c.label != "synastry" {
				continue
			}
			saw = true
			if c.houses != hs || c.zodiac != z {
				t.Errorf("%s: the comparison carried %v / %v, want the frame's %v / %v", where, c.houses, c.zodiac, hs, z)
			}
		}
		if !saw {
			t.Errorf("%s: no comparison was asked for: %v", where, spy.snapshot())
		}
	}
	spy.reset()
	m = choosePair(m, scenePair{left: slotAt(m, 1), right: slotAt(m, 0)})
	m = settleHouses(t, m)
	comparison(m, "① in ②", recipe.Placidus, recipe.Tropical)
	spy.reset()
	m = showPair(m, scenePair{left: slotAt(m, 0), right: slotAt(m, 1)})
	m = settleHouses(t, m)
	comparison(m, "② in ①", recipe.WholeSign, recipe.Lahiri)
}

// Every chart is shown in its own house system on every row and tab; the
// comparison's is the frame's: the base the tab picks, else the chart under
// the cursor on its own tab.
func TestComparisonHousesFollowTheFrame(t *testing.T) {
	const (
		anna = recipe.Placidus
		ben  = recipe.Koch
		now  = recipe.Campanus
	)
	m := withOwnHouses(framesFixture(t, 2, 0), anna, ben, now)
	check := func(m Model, frame recipe.HouseSystem, where string) {
		t.Helper()
		for i, own := range []recipe.HouseSystem{anna, ben, now} {
			if got := m.lensFor(slotAt(m, i)).Houses; got != own {
				t.Errorf("%s: slot %d is shown in %v, want its own %v", where, i, got, own)
			}
		}
		if got := m.comparisonLens().Houses; got != frame {
			t.Errorf("%s: the comparison carries %v, want the frame's %v", where, got, frame)
		}
	}
	check(m, anna, "on Anna")
	m = walk(t, m, keyDown, 1)
	check(m, ben, "an arrow to Ben: his own tab, Ben frames")
	m = walk(t, m, keyDown, 1)
	check(m, now, "on now's own tab: no pair, the cursor's own")
	m = choosePair(m, scenePair{left: slotAt(m, 0), right: slotAt(m, 2)})
	wantTab(t, m, "③ in ①", "now read in Anna")
	check(m, anna, "now read in Anna: Anna frames")
	m = choosePair(m, scenePair{left: slotAt(m, 1), right: slotAt(m, 2)})
	wantTab(t, m, "③ in ②", "now read in Ben")
	check(m, ben, "now read in Ben: Ben frames")

	// Outside a comparison a chart is shown in its own system too.
	m = withOwnHouses(framesFixture(t, 3, 0), anna, ben, recipe.Equal, now)
	m.slots.SetSlotSelected(2, false)
	if got := m.lensFor(slotAt(m, 2)).Houses; got != recipe.Equal {
		t.Errorf("an unselected chart is shown in %v, want its own %v", got, recipe.Equal)
	}
}

// The zodiac follows the same rule: each chart's own on its rows, the frame's
// for the comparison, wherever the cursor stands.
func TestComparisonZodiacFollowsTheFrame(t *testing.T) {
	own := []recipe.Zodiac{recipe.Tropical, recipe.Lahiri, recipe.Raman}
	m := withOwnZodiacs(framesFixture(t, 2, 0), own...)
	check := func(m Model, frame recipe.Zodiac, where string) {
		t.Helper()
		for i, z := range own {
			if got := m.lensFor(slotAt(m, i)).Zodiac; got != z {
				t.Errorf("%s: slot %d is shown in %v, want its own %v", where, i, got, z)
			}
		}
		if got := m.comparisonLens().Zodiac; got != frame {
			t.Errorf("%s: the comparison carries %v, want the frame's %v", where, got, frame)
		}
	}
	check(m, recipe.Tropical, "on Anna")
	m = walk(t, m, keyDown, 1)
	check(m, recipe.Lahiri, "an arrow to Ben")
	m = walk(t, m, keyDown, 1)
	check(m, recipe.Raman, "now's own tab")
	m = choosePair(m, scenePair{left: slotAt(m, 1), right: slotAt(m, 2)})
	wantTab(t, m, "③ in ②", "now read in Ben")
	check(m, recipe.Lahiri, "now read in Ben: Ben frames")
	m = choosePair(m, scenePair{left: slotAt(m, 0), right: slotAt(m, 2)})
	check(m, recipe.Tropical, "now read in Anna: Anna frames")
	m = walk(t, m, keyUp, 1)
	m.base = slotAt(m, 0)
	check(m, recipe.Tropical, "Ben read in Anna on Ben's row: Anna frames")
}

// The timeline asks in the lens of the chart the cursor is on, the floating
// chart's own there (operator, 2026-10-24: "the chart you are on"). A move to
// a chart in another lens asks again; a move between charts in one lens asks
// nothing.
func TestTimelineFollowsTheCursorsLens(t *testing.T) {
	sessionSandbox(t)
	stubFrameLookups(t)
	searcher := &mergeSearcher{}
	stubTimelineSearch(t, searcher)
	m := octoberSet(t, 0) // ① ② now
	m = withOwnHouses(m, recipe.Placidus, recipe.Placidus, recipe.Koch)
	m = withOwnZodiacs(m, recipe.Lahiri, recipe.Lahiri, recipe.Tropical)
	searches := func() []string {
		searcher.mu.Lock()
		defer searcher.mu.Unlock()
		return append([]string(nil), searcher.cmds...)
	}
	moveTo := func(m Model, cursor int) Model {
		m = m.focusSlot(cursor)
		m, cmd := m.afterFocusChange()
		m = pump(t, m, cmd)
		return loadTimeline(t, m)
	}
	// asked checks every search since from: Lahiri's band and Placidus on ①
	// and ②, the tropical pin and Koch on now.
	asked := func(from int, sidereal bool, hs recipe.HouseSystem, where string) {
		t.Helper()
		after := searches()[from:]
		if len(after) == 0 {
			t.Fatalf("%s: nothing was asked", where)
		}
		for _, argv := range after {
			if band := strings.Contains(argv, " =s 0.883208 _Ys "); band != sidereal {
				t.Errorf("%s: Lahiri's band present = %v, want %v: %s", where, band, sidereal, argv)
			}
			fields := strings.Fields(argv)
			if !slices.Contains(fields, "-t") && !slices.Contains(fields, "-tr") {
				continue
			}
			if i := slices.Index(fields, "-c"); i < 0 || i+1 >= len(fields) || fields[i+1] != strconv.Itoa(int(hs)) {
				t.Errorf("%s: a natal's search is not asked in %v: %s", where, hs, argv)
			}
		}
	}

	m = loadTimeline(t, m)
	if req, _, _ := m.timelineInputs(); req.Lens.Houses != recipe.Placidus || req.Lens.Zodiac != recipe.Lahiri {
		t.Errorf("on ①: the request asks in %v / %v, want ①'s Placidus / Lahiri", req.Lens.Houses, req.Lens.Zodiac)
	}
	asked(0, true, recipe.Placidus, "on ①")
	n := len(searches())
	m = moveTo(m, 1)
	if got := len(searches()); got != n {
		t.Errorf("a move to ② in ①'s very lens asked %d searches again", got-n)
	}
	m = moveTo(m, 2)
	if req, _, _ := m.timelineInputs(); req.Lens.Houses != recipe.Koch || req.Lens.Zodiac != recipe.Tropical {
		t.Errorf("on now: the request asks in %v / %v, want now's own Koch / tropical", req.Lens.Houses, req.Lens.Zodiac)
	}
	asked(n, false, recipe.Koch, "on now")
	n = len(searches())
	m = moveTo(m, 0)
	asked(n, true, recipe.Placidus, "back on ①")
	// A pair tab moves the comparison's frame, not the cursor: nothing is asked.
	n = len(searches())
	m = choosePair(m, scenePair{left: slotAt(m, 0), right: slotAt(m, 1)})
	m, cmd := m.afterComparisonChange()
	m = pump(t, m, cmd)
	loadTimeline(t, m)
	if got := len(searches()); got != n {
		t.Errorf("a pair tab on ① asked %d searches again", got-n)
	}
}

// imageRun starts a wheel render for the model's current plan and returns the
// model with that run in flight (previewActive, as after Ctrl+X).
func imageRun(t *testing.T, m Model) Model {
	t.Helper()
	m.previewActive = true
	m, _ = m.requestImageNow()
	if imageState(m).cancel == nil {
		t.Fatal("no wheel run started")
	}
	return m
}

// finishRun hands the in-flight run's result (a picture) to the coordinator
// and returns the command it answers with.
func finishRun(m Model, run refreshState, picture string) (Model, tea.Cmd) {
	return m.onRefreshResult(refreshResultMsg{
		surface: surfaceImage, runID: run.runningRunID, wantID: run.runningWantID,
		payload: imagePayload{bytes: []byte(picture)},
	})
}

// driveCmds runs a command the way the runtime would, batches included, and
// returns every message it produced.
func driveCmds(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, driveCmds(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// recordShowWheel swaps the preview publisher for one that records the bytes
// it was handed, and stubs the render seams so a follow-up render runs too.
func recordShowWheel(t *testing.T) *[]string {
	t.Helper()
	stubRenderWheelImage(t)
	stubRenderOverlayWheelImage(t)
	var shown []string
	prev := showWheel
	showWheel = func(b []byte) (string, error) {
		shown = append(shown, string(b))
		return "/tmp/stub.png", nil
	}
	t.Cleanup(func() { showWheel = prev })
	return &shown
}

// B4: a wheel for a chart the cursor has left never replaces the picture on
// screen; the render for the chart the cursor is on still follows.
func TestCursorScanNeverPublishesStaleWheels(t *testing.T) {
	// followed says the answer started the render for the newest want.
	followed := func(m Model) bool {
		return imageState(m).cancel != nil && imageState(m).runningWantID == imageState(m).wantID
	}

	shown := recordShowWheel(t)
	m := imageRun(t, framesFixture(t, 3, 0))
	run := imageState(m)
	for i := 0; i < 2; i++ { // Anna -> Ben -> Carl while Anna's wheel renders
		m = walk(t, m, keyDown, 1)
	}
	if imageState(m).wantID == run.runningWantID {
		t.Fatal("the cursor scan asked for no new wheel")
	}
	m, cmd := finishRun(m, run, "ANNA")
	driveCmds(cmd)
	if len(*shown) != 0 {
		t.Errorf("Anna's wheel reached the viewer after the cursor reached Carl: %q", *shown)
	}
	if !followed(m) {
		t.Errorf("no render followed for the newest want: running want %d, newest %d", imageState(m).runningWantID, imageState(m).wantID)
	}

	// A tab chosen while a render runs is the same: the old tab's picture is stale.
	m = imageRun(t, framesFixture(t, 3, 0))
	run = imageState(m)
	m = bracket(t, m, "]")
	m, _ = m.refreshWheelIfActive() // the text commit of the new tab asks for its wheel
	if imageState(m).wantID == run.runningWantID {
		t.Fatal("the chosen tab asked for no new wheel")
	}
	m, cmd = finishRun(m, run, "OWN")
	driveCmds(cmd)
	if len(*shown) != 0 {
		t.Errorf("the own tab's wheel reached the viewer after a pair tab was chosen: %q", *shown)
	}

	// A house system or zodiac chosen while the old system's render runs: the
	// wheel in the old system is stale too, though cursor and tab are the same.
	for _, change := range []struct {
		name  string
		apply func(Model) Model
	}{
		{"house system", func(m Model) Model { return withOwnHouses(m, recipe.Koch) }},
		{"zodiac", func(m Model) Model { return withOwnZodiacs(m, recipe.Lahiri) }},
	} {
		m = imageRun(t, framesFixture(t, 3, 0))
		run = imageState(m)
		m = change.apply(m)
		m, _ = m.refreshWheelIfActive()
		if imageState(m).wantID == run.runningWantID {
			t.Fatalf("a %s change asked for no new wheel", change.name)
		}
		m, cmd = finishRun(m, run, "OLD")
		driveCmds(cmd)
		if len(*shown) != 0 {
			t.Errorf("the wheel in the old %s reached the viewer: %q", change.name, *shown)
		}
		if !followed(m) {
			t.Errorf("a %s change: no render followed for the newest want", change.name)
		}
	}
}

// B5: a held time step still shows its intermediate pictures: the stale wheel
// differs from the newest want in the floating time alone.
func TestHeldStepStillAnimates(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cursor int
		visit  bool
	}{{"now's own tab", 2, false}, {"now's wheel in a pair", 2, true}} {
		t.Run(tc.name, func(t *testing.T) {
			shown := recordShowWheel(t)
			m := framesFixture(t, 2, tc.cursor)
			if tc.visit {
				m.base = slotAt(m, 0)
			}
			m = imageRun(t, m)
			run := imageState(m)
			m.targetFloatingTime = m.targetFloatingTime.Add(time.Hour)
			m, _ = m.refreshWheelIfActive()
			if imageState(m).wantID == run.runningWantID {
				t.Fatal("the time step asked for no new wheel")
			}
			m, cmd := finishRun(m, run, "STEP")
			driveCmds(cmd)
			if len(*shown) != 1 || (*shown)[0] != "STEP" {
				t.Errorf("the viewer was shown %q, want the one intermediate frame [STEP]", *shown)
			}
			if imageState(m).cancel == nil || imageState(m).runningWantID != imageState(m).wantID {
				t.Error("no render followed for the newest time")
			}
		})
	}
}

// A finder names the focused chart and Enter edits its record alone, whatever
// tab is active (operator, 2026-10-24: "only the focused chart should have it").
func TestFinderTitlesNameTheFocusedChart(t *testing.T) {
	setup := func(t *testing.T) Model {
		sessionSandbox(t)
		stubHouseLookups(t)
		m := framesFixture(t, 2, 2) // on now, three charts: Anna ①, Ben ②, now ③
		m.cfg = Config{Accessible: true, Color: ColorNever}
		return m
	}
	houseTitle := func(t *testing.T, m Model) string {
		t.Helper()
		m, _ = tpress(t, m, keyCtrlHMain)
		if m.startup != startupHouseOpen {
			t.Fatalf("Ctrl+H opened %q", m.startup)
		}
		return topEdge(t, m)
	}
	zodiacTitle := func(t *testing.T, m Model) string {
		t.Helper()
		return zodiacTopEdge(t, openZodiacFinderOn(t, m))
	}
	named := func(e, want string) bool { return strings.Contains(e, want) && !strings.Contains(e, "comparison") }

	m := setup(t)
	if e := houseTitle(t, m); !named(e, "Houses · Now") {
		t.Errorf("now's own tab: houses title %q", e)
	}
	if e := zodiacTitle(t, m); !named(e, "Zodiac · Now") {
		t.Errorf("now's own tab: zodiac title %q", e)
	}
	// Now read in Ben: Ben frames the comparison, the finders still name now.
	m = setup(t)
	m = choosePair(m, scenePair{left: slotAt(m, 1), right: slotAt(m, 2)})
	wantTab(t, m, "3 in 2", "now read in Ben")
	if e := houseTitle(t, m); !named(e, "Houses · Now") {
		t.Errorf("now in Ben: houses title %q", e)
	}
	if e := zodiacTitle(t, m); !named(e, "Zodiac · Now") {
		t.Errorf("now in Ben: zodiac title %q", e)
	}
	m = setup(t)
	m = walk(t, m, keyUp, 1)
	if e := houseTitle(t, m); !named(e, "Houses · Synthetic 2") {
		t.Errorf("on Ben: houses title %q", e)
	}
	if e := zodiacTitle(t, m); !named(e, "Zodiac · Synthetic 2") {
		t.Errorf("on Ben: zodiac title %q", e)
	}

	// Enter on now, read in Ben, edits now's own record; Ben's is left alone.
	m = setup(t)
	m = choosePair(m, scenePair{left: slotAt(m, 1), right: slotAt(m, 2)})
	m, _ = tpress(t, m, keyCtrlHMain)
	for _, r := range "campanus" {
		m, _ = tpress(t, m, runes(string(r)))
	}
	m, _ = tpress(t, m, keyEnter)
	if rec, ok := m.frames.find("", true); !ok || !rec.HasHouses || rec.Houses != recipe.Campanus {
		t.Errorf("now's own record = %+v (found %v), want Campanus", rec, ok)
	}
	if rec, ok := m.frames.find(slotFile(slotAt(m, 1)), false); ok && rec.HasHouses {
		t.Errorf("the frame's record was edited: %+v", rec)
	}
}

// The floating slot never frames another chart: every pair tab, on any row,
// is framed by an anchored chart and its comparison never carries the
// moment's own system; on the moment's own tab every row keeps its own.
func TestNowNeverFramesOtherHouses(t *testing.T) {
	m := withOwnHouses(framesFixture(t, 2, 2), recipe.Koch, recipe.Equal, recipe.WholeSign)
	if sc := m.deriveScene(); sc.mode != sceneComparing || sc.pair != nil || sc.frame == nil || !sc.frame.Floating {
		t.Fatalf("setup: want comparing on now's own tab, got mode %v pair %v", sc.mode, sc.pair)
	}
	for cursor := 0; cursor < 3; cursor++ {
		m.cursorIdx = cursor
		for _, p := range m.deriveScene().pairs {
			pm := choosePair(m, p)
			sc := pm.deriveScene()
			if sc.frame == nil || sc.frame.Floating {
				t.Errorf("cursor %d, pair %s in %s: the moment frames it", cursor, p.right.Chart.Name, p.left.Chart.Name)
				continue
			}
			if got := pm.comparisonLens().Houses; got == recipe.WholeSign {
				t.Errorf("cursor %d, pair %s in %s: the comparison carries the moment's Whole Sign", cursor, p.right.Chart.Name, p.left.Chart.Name)
			}
		}
	}
	// What the operator reads on now's own tab: each row its own tag.
	m.cursorIdx = 2
	m = settleHouses(t, m)
	screen := ansi.Strip(m.View())
	for _, tag := range []string{"KoTr", "EqTr", "WhTr"} {
		if !strings.Contains(screen, tag) {
			t.Errorf("no row carries %s on now's own tab:\n%s", tag, screen)
		}
	}
}

// A chart whose file could not be read has no house system: with the cursor on
// it the timeline searches nothing in a guessed Whole Sign, as the main chart
// casts nothing. On any other chart the timeline asks in that chart's own.
func TestTimelineRefusesUnresolvedCursorHouses(t *testing.T) {
	for cursor := 0; cursor < 3; cursor++ {
		m := framesFixture(t, 2, cursor)
		if _, _, ok := m.timelineInputs(); !ok {
			t.Fatalf("cursor %d: the resolved fixture has no timeline inputs", cursor)
		}
		m.frames.failed = map[string]string{slotFile(slotAt(m, 0)): "astrolog exploded"}
		if _, _, ok := m.timelineInputs(); ok == (cursor == 0) {
			t.Errorf("cursor %d: with ① unresolved the timeline asked = %v, want %v", cursor, ok, cursor != 0)
		}
	}
}

// A template tab's facts are cast in the screen's lens: when the frame's chart
// file could not be read there is no lens, and nothing is cast.
func TestTemplateTabRefusesUnresolvedFrame(t *testing.T) {
	m := framesFixture(t, 2, 1) // Ben frames
	m.chart.positions = []aspect.Position{{Body: aspect.Sun}, {Body: aspect.Moon}, {Body: aspect.Venus}}
	m.focused = PanelBodies
	m.bodyCursorIdx = 2
	m = activeOnTemplate(m)
	build := func(m Model) tea.Cmd {
		m, _ = m.syncTemplate()
		return buildTemplateCmd(m, context.Background(), 1, m.refresh[surfaceTemplate].wantID)
	}
	if build(m) == nil {
		t.Fatal("the resolved fixture builds no template run")
	}
	m.frames.failed = map[string]string{slotFile(slotAt(m, 1)): "astrolog exploded"}
	if build(m) != nil {
		t.Error("a template run was built in a guessed system for an unresolved frame")
	}
}

// feedMsgs hands the messages of driven commands back to the model, publish
// answers included, until none is left to answer.
func feedMsgs(m Model, msgs []tea.Msg) Model {
	for _, msg := range msgs {
		if p, ok := msg.(wheelPublishedMsg); ok {
			var cmd tea.Cmd
			m, cmd = m.onWheelPublished(p)
			m = feedMsgs(m, driveCmds(cmd))
		}
	}
	return m
}

// B4: a picture queued behind a running publish is judged when it is about to
// start, not when it was rendered. A publishes; B completes and waits; the
// cursor moves to C; A finishes: B never reaches the viewer, C does.
func TestQueuedStaleWheelNeverPublishes(t *testing.T) {
	shown := recordShowWheel(t)
	m := imageRun(t, framesFixture(t, 3, 0))
	m, publishA := finishRun(m, imageState(m), "A")
	if !m.wheelPublishing {
		t.Fatal("A did not start publishing")
	}
	m = walk(t, m, keyDown, 1) // Ben: B renders
	runB := imageState(m)
	if runB.cancel == nil {
		t.Fatal("no render started for B")
	}
	m, cmd := finishRun(m, runB, "B")
	driveCmds(cmd)
	if m.wheelPublishPending == nil {
		t.Fatal("B did not queue behind A")
	}
	m = walk(t, m, keyDown, 1) // Carl: C renders
	runC := imageState(m)
	if runC.cancel == nil || runC.runningWantID == runB.runningWantID {
		t.Fatal("no render started for C")
	}
	m = feedMsgs(m, driveCmds(publishA)) // A completes; B must not start
	if want := []string{"A"}; !reflect.DeepEqual(*shown, want) {
		t.Fatalf("after A completed the viewer was shown %q, want %q (B is a chart the screen has left)", *shown, want)
	}
	m, cmd = finishRun(m, runC, "C")
	m = feedMsgs(m, driveCmds(cmd))
	if want := []string{"A", "C"}; !reflect.DeepEqual(*shown, want) {
		t.Errorf("the viewer was shown %q, want %q", *shown, want)
	}

	// Held stepping: B differs from C in the floating time alone, so it is an
	// intermediate frame of the animation and still publishes.
	*shown = nil
	m = imageRun(t, framesFixture(t, 2, 2))
	m, publishA = finishRun(m, imageState(m), "A")
	m.targetFloatingTime = m.targetFloatingTime.Add(time.Hour)
	m, _ = m.refreshWheelIfActive()
	runB = imageState(m)
	m, cmd = finishRun(m, runB, "B")
	driveCmds(cmd)
	if m.wheelPublishPending == nil {
		t.Fatal("stepping: B did not queue behind A")
	}
	m.targetFloatingTime = m.targetFloatingTime.Add(time.Hour)
	m, _ = m.refreshWheelIfActive()
	runC = imageState(m)
	m = feedMsgs(m, driveCmds(publishA))
	if want := []string{"A", "B"}; !reflect.DeepEqual(*shown, want) {
		t.Fatalf("stepping: after A completed the viewer was shown %q, want %q", *shown, want)
	}
	m, cmd = finishRun(m, runC, "C")
	m = feedMsgs(m, driveCmds(cmd))
	if want := []string{"A", "B", "C"}; !reflect.DeepEqual(*shown, want) {
		t.Errorf("stepping: the viewer was shown %q, want %q", *shown, want)
	}
}
