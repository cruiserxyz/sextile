package ui

import (
	"os"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Objects slice 1: Space marks any object the Bodies panel shows, and the
// session file keeps the marks under Astrolog's own names.

// rowOfObject is the Bodies cursor index of the row standing for object n.
func rowOfObject(t *testing.T, m Model, n int) int {
	t.Helper()
	for i := 0; i < panel.BodyRowCount(m.chart.positions, m.chart.cusps); i++ {
		if o, ok := panel.ObjectAtCursor(m.chart.positions, m.chart.cusps, i); ok && o.Number == n {
			return i
		}
	}
	t.Fatalf("no Bodies row for object %d", n)
	return -1
}

// Space on a row with no glyph (Fortune) marks it, the mark column shows
// it, and Space again unmarks it. The Sun marks exactly as before.
func TestSpaceMarksAnUntypedRow(t *testing.T) {
	stubSelectionSeams(t)
	m := selectionModel(t, true)
	m.chart.positions = append(m.chart.positions, aspect.Position{Object: fixtureObject(t, 19), Longitude: 100})
	m.bodyCursorIdx = rowOfObject(t, m, 19)
	if !m.mainSelects() {
		t.Fatal("Space is not offered on the Fortune row")
	}
	m, _ = pressSpace(t, m)
	if !m.slots.ObjectSelected(m.cursorIdx, 19) {
		t.Fatal("Space did not mark Fortune")
	}
	if hl := m.wheelHighlight(); !slices.Equal(hl.Selected, []int{19}) {
		t.Errorf("the wheel highlight's selected tier = %v, want [19]", hl.Selected)
	}
	if !m.showLens(m.focusedSlot()).Objects.Only.Has(19) {
		t.Error("the chart's own lens does not narrow to Fortune")
	}
	if got := m.slots.SelectedObjects(m.cursorIdx); len(got) != 1 || got[0].Name != "Fortune" {
		t.Errorf("marks = %v, want Fortune by Astrolog's name", got)
	}
	rows := strings.Split(bodiesPanel(m), "\n")
	marked := 0
	for _, r := range rows {
		if strings.Contains(r, "Fort") && strings.ContainsAny(r, "●*") {
			marked++
		}
	}
	if marked != 1 {
		t.Errorf("%d Fortune rows show the mark, want 1:\n%s", marked, bodiesPanel(m))
	}
	m, _ = pressSpace(t, m)
	if m.slots.ObjectSelected(m.cursorIdx, 19) || m.slots.HasMarks(m.cursorIdx) {
		t.Error("a second Space did not unmark Fortune")
	}
}

// A ticked cusp point (object 23, 2nd Cusp) is an object like any other.
func TestSpaceMarksATickedCusp(t *testing.T) {
	stubSelectionSeams(t)
	m := selectionModel(t, true)
	cusps := fullChartCusps()
	m.chart.cusps = cusps
	m.chart.positions = withTicked(append(fullChartPositions(), m.chart.positions[:0]...), cusps, 2)
	m.bodyCursorIdx = rowOfObject(t, m, 23)
	if !m.mainSelects() {
		t.Fatal("Space is not offered on the ticked 2nd Cusp row")
	}
	m, _ = pressSpace(t, m)
	if got := m.slots.SelectedObjects(m.cursorIdx); len(got) != 1 || got[0].Number != 23 || got[0].Name != "2nd Cusp" {
		t.Errorf("marks = %v, want 2nd Cusp (23)", got)
	}
}

// A row with no object (nothing Astrolog printed it for) and an index off the
// list mark nothing; and a frame cusp nobody ticked has no row at all.
func TestSpaceRefusesAPlainCusp(t *testing.T) {
	stubSelectionSeams(t)
	m := selectionModel(t, true)
	cusps := fullChartCusps()
	m.chart.cusps = cusps
	plain := fullChartPositions()
	if got, want := panel.BodyRowCount(plain, cusps), fullChartPlanets+fullChartAngles; got != want {
		t.Fatalf("an unticked frame cusp has a row: %d rows, want %d", got, want)
	}
	m.chart.positions = append(plain, aspect.Position{Longitude: 5}) // no body, no object
	m.bodyCursorIdx = fullChartPlanets                               // the extra row follows the planets
	if m.mainSelects() {
		t.Error("Space is offered on a row with no object")
	}
	before := m.slots.HasMarks(m.cursorIdx)
	m, _ = pressSpace(t, m)
	if m.slots.HasMarks(m.cursorIdx) != before || len(m.slots.SelectedObjects(m.cursorIdx)) != 0 {
		t.Error("Space on a row with no object changed the marks")
	}
	if _, ok := panel.ObjectAtCursor(m.chart.positions, cusps, 999); ok {
		t.Error("an index off the list resolved to an object")
	}
	if _, ok := panel.ObjectAtCursor(m.chart.positions, cusps, -1); ok {
		t.Error("a negative index resolved to an object")
	}
}

// A 0.1.2 session file on disk (wire names, bare comma) resolves through the
// alias table, and the next quit writes the registry names.
func TestLegacyMarkNamesResolve(t *testing.T) {
	sessionSandbox(t)
	seedPreferences(t)
	snap := snapshotChart()
	err := store.SaveSession(store.Session{
		Folder: store.FolderID(store.ChartsDir()),
		Slots:  []store.SessionSlot{{Chart: snap, Usable: true, Bodies: []string{"sun", "north_node", "imum_coeli"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(store.SessionPath())
	legacy := strings.Replace(string(b), "version: 4", "version: 2", 1)
	if !strings.Contains(legacy, "    bodies: sun,north_node,imum_coeli\n") {
		t.Fatalf("fixture: not a 0.1.2 line:\n%s", legacy)
	}
	if err := os.WriteFile(store.SessionPath(), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	m := startedModel(t, realPerimeter(t))
	for _, n := range []int{1, 16, 25} {
		if !m.slots.ObjectSelected(0, n) {
			t.Errorf("object %d is not marked after the restore: %v", n, m.slots.SelectedObjects(0))
		}
	}
	if len(m.slots.KeptMarks(0)) != 0 {
		t.Errorf("a wire name was kept as unknown: %v", m.slots.KeptMarks(0))
	}
	quitModel(t, m)
	if got := fileLine(t, "    bodies:"); got != "    bodies: Sun,North Node,Nadir" {
		t.Errorf("the rewritten line is %q, want %q", got, "    bodies: Sun,North Node,Nadir")
	}
}

// marksStart is a cold start whose session names marks and drawn, with the
// six-object list and the ephemeris answer in, as a normal start has them.
func marksStart(t *testing.T, av lookup.EphemerisAvailability, sess store.Session) Model {
	t.Helper()
	var tm tea.Model = newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen))
	mm := tm.(Model)
	mm.objectRegistry = drawnRegistry()
	mm.lens.Registry = mm.objectRegistry
	mm.ephemeris = av
	tm = mm
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(withSession(perimeter(), sess))
	return tm.(Model)
}

// A mark on an object the drawn set does not draw is kept by number and
// written back; the Log has no line about it.
func TestMarkOnAnUndrawnObjectIsKept(t *testing.T) {
	sessionSandbox(t)
	sess := store.Session{Version: 3, Drawn: []string{"Sun", "Moon"}, FloatingBodies: []string{"Fortune"}}
	m := marksStart(t, lookup.EphemerisAvailability{}, sess)
	idx := len(m.slots.All()) - 1
	if !m.slots.ObjectSelected(idx, 19) {
		t.Fatalf("the Fortune mark is not on the floating slot: %v", m.slots.SelectedObjects(idx))
	}
	if n := countLines(m, "is not an object of this Astrolog"); n != 0 {
		t.Errorf("the Log says %d unknown-object lines for a known object:\n%s", n, strings.Join(logTexts(m), "\n"))
	}
	if got := m.markNames(idx); !slices.Equal(got, []string{"Fortune"}) {
		t.Errorf("the session would write %v, want Fortune", got)
	}
}

// A name in both drawn: and a bodies: line is said once.
func TestKeptMarkSaidOnceWithDrawn(t *testing.T) {
	sessionSandbox(t)
	sess := store.Session{Version: 3, Drawn: []string{"Sun", "Nibiru"}, FloatingBodies: []string{"Nibiru", "Sedna"}}
	m := marksStart(t, lookup.EphemerisAvailability{}, sess)
	for _, name := range []string{"Nibiru", "Sedna"} {
		line := name + " is not an object of this Astrolog, kept in the session"
		if n := countLines(m, line); n != 1 {
			t.Errorf("the Log carries %q %d times, want once:\n%s", line, n, strings.Join(logTexts(m), "\n"))
		}
	}
	idx := len(m.slots.All()) - 1
	if got := m.markNames(idx); !slices.Equal(got, []string{"Nibiru", "Sedna"}) {
		t.Errorf("the session would write %v, want both kept names", got)
	}
}

// `, n` drops the slots, and the kept names with them.
func TestResetClearsKeptMarks(t *testing.T) {
	sessionSandbox(t)
	m := marksStart(t, lookup.EphemerisAvailability{}, store.Session{Version: 3, FloatingBodies: []string{"Sun", "Sedna"}})
	idx := len(m.slots.All()) - 1
	if len(m.slots.KeptMarks(idx)) != 1 || !m.slots.ObjectSelected(idx, 1) {
		t.Fatalf("fixture: marks %v kept %v", m.slots.SelectedObjects(idx), m.slots.KeptMarks(idx))
	}
	m.startup = startupReady
	m, cmd := pressReset(t, m)
	m = pump(t, m, cmd)
	idx = len(m.slots.All()) - 1
	if m.slots.HasMarks(idx) || len(m.slots.KeptMarks(idx)) != 0 {
		t.Errorf("after `, n` the slot still holds marks %v kept %v", m.slots.SelectedObjects(idx), m.slots.KeptMarks(idx))
	}
	if got := m.sessionSnapshot(); len(got.FloatingBodies) != 0 {
		t.Errorf("the reset session would write %v", got.FloatingBodies)
	}
}

// A second window writes no session, marks and kept names included.
func TestSecondWindowWritesNoMarks(t *testing.T) {
	sessionSandbox(t)
	holder := listModel(t, 2, 0)
	if err := store.SaveSession(holder.sessionSnapshot()); err != nil {
		t.Fatal(err)
	}
	general := fileText(t, store.SessionPath())
	p := readPerimeterCmd(true)().(ingest.StartupPerimeterReadMsg)
	p.LocationCandidate = location.Candidate{Source: "stored-location", Place: berlinRaw}
	var tm tea.Model = newTestModel(t, Config{Accessible: true, SecondWindow: true}, FrozenClock(sessionFrozen))
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(p)
	m := tm.(Model)
	m.startup = startupReady
	idx := len(m.slots.All()) - 1
	m.slots.ToggleObjectSelection(idx, fixtureObject(t, 19))
	m.slots.KeepMark(idx, "Sedna")
	quitWindow(t, m)
	if got := fileText(t, store.SessionPath()); got != general {
		t.Errorf("a second window wrote the general session:\n%s", got)
	}
	if entries, _ := os.ReadDir(store.SessionsDir()); len(entries) != 0 {
		t.Errorf("a second window wrote a named session: %v", entries)
	}
}

// The marks come back after a quit and a start, under Astrolog's own names:
// the Sun, Fortune and a ticked 2nd Cusp. (The name stays short: the test's
// temp folders are in Astrolog's ephemeris path, which it truncates past 255
// characters.)
func TestMarkSurvivesARestart(t *testing.T) {
	sessionSandbox(t)
	m, _, _ := ticksStart(t)
	cm := tickObject(t, chooserOf(t, m), 23) // draw the 2nd Cusp point
	cm = pressInChooser(t, cm, tea.KeyMsg{Type: tea.KeyEnter})
	idx := len(cm.slots.All()) - 1
	for _, n := range []int{23, 1, 19} { // marked in no particular order
		o, ok := aspect.Registry(cm.objectRegistry).ByNumber(n)
		if !ok {
			t.Fatalf("object %d is not in the registry", n)
		}
		cm.slots.ToggleObjectSelection(idx, o)
	}
	if _, cmd := cm.quit(); cmd == nil {
		t.Fatal("q did not quit")
	}
	if got := fileLine(t, "  bodies:"); got != "  bodies: Sun,Fortune,2nd Cusp" {
		t.Fatalf("the file's floating line is %q, want %q", got, "  bodies: Sun,Fortune,2nd Cusp")
	}

	m2 := drawnRestart(t, cm, cm.ephemeris, reloaded(t))
	idx2 := len(m2.slots.All()) - 1
	for _, n := range []int{1, 19, 23} {
		if !m2.slots.ObjectSelected(idx2, n) {
			t.Errorf("after the restart object %d is not marked: %v", n, m2.slots.SelectedObjects(idx2))
		}
	}
	if len(m2.slots.KeptMarks(idx2)) != 0 {
		t.Errorf("a known name was kept as unknown: %v", m2.slots.KeptMarks(idx2))
	}
}

// A mark on Chiron where seas_18.se1 is missing: the row is not shown, the file
// keeps the name, and the only Log line is the existing one for the file.
func TestMarkedAbsentBodyIsKept(t *testing.T) {
	sessionSandbox(t)
	m, _, _ := ticksStart(t)
	sess := store.Session{Version: 3, Folder: store.FolderID(store.ChartsDir()), FloatingBodies: []string{"Chiron", "Sun"}}
	m2 := drawnRestart(t, m, m.ephemeris, sess)
	idx := len(m2.slots.All()) - 1
	if !m2.slots.ObjectSelected(idx, 11) || !m2.slots.ObjectSelected(idx, 1) {
		t.Fatalf("the marks are %v, want the Sun and Chiron", m2.slots.SelectedObjects(idx))
	}
	for _, p := range m2.chart.positions {
		if p.Object.Number == 11 {
			t.Error("the Bodies panel shows Chiron without its ephemeris file")
		}
	}
	if n := countLines(m2, "is not an object of this Astrolog"); n != 0 {
		t.Errorf("the Log says %d unknown-object lines for Chiron", n)
	}
	if n := countLines(m2, "can't be drawn here"); n != 1 {
		t.Errorf("the Log says which bodies lack a file %d times, want once:\n%s", n, strings.Join(logTexts(m2), "\n"))
	}
	m2.quit()
	if got := fileLine(t, "  bodies:"); got != "  bodies: Sun,Chiron" {
		t.Errorf("the file after the quit holds %q, want the Sun and Chiron", got)
	}
}

// TestMarkedFortuneNarrowsTheAspects of the spec, under a shorter name (the
// 34-character one pushes the test folder past Astrolog's 255-character
// ephemeris path). With Fortune and the Sun marked, the chart's own text
// surface asks Astrolog for those two alone: the show list carries 19, and the
// Aspects list holds exactly the unmarked list's rows between marked objects.
func TestFortuneMarkNarrows(t *testing.T) {
	sessionSandbox(t)
	m, read, _ := ticksStart(t)
	idx := len(m.slots.All()) - 1
	m, cmd := m.noteWant(surfaceText, true)
	m = pumpPatient(t, m, cmd)
	all := slices.Clone(m.chart.aspects)
	if len(all) == 0 {
		t.Fatal("fixture: the unmarked chart has no aspects")
	}
	marked := map[int]bool{}
	var want []aspect.Aspect
	// Mark Fortune and every object it makes an aspect with in the full list.
	for _, a := range all {
		if a.AObject.Number == 19 || a.BObject.Number == 19 {
			marked[a.AObject.Number], marked[a.BObject.Number] = true, true
		}
	}
	if !marked[19] || len(marked) < 2 {
		t.Fatalf("fixture: Fortune makes no aspect in the unmarked list (%d rows)", len(all))
	}
	for _, a := range all {
		if marked[a.AObject.Number] && marked[a.BObject.Number] {
			want = append(want, a)
		}
	}
	for n := range marked {
		o, ok := aspect.Registry(m.objectRegistry).ByNumber(n)
		if !ok {
			t.Fatalf("object %d is not in the registry", n)
		}
		m.slots.ToggleObjectSelection(idx, o)
	}

	mark := len(read())
	m, cmd = m.noteWant(surfaceText, true)
	m = pumpPatient(t, m, cmd)
	argv := textArgv(read()[mark:])
	if argv == nil {
		t.Fatalf("no text-surface cast; log:\n%s", strings.Join(logTexts(m), "\n"))
	}
	i := slices.Index(argv, "-R0")
	if i < 0 || !slices.Contains(argv[i+1:], "19") {
		t.Errorf("the show list does not carry Fortune: %v", band(argv))
	}
	if len(m.chart.aspects) >= len(all) {
		t.Errorf("the Aspects list did not narrow: %d rows, was %d", len(m.chart.aspects), len(all))
	}
	pair := func(a aspect.Aspect) [3]int { return [3]int{a.AObject.Number, a.BObject.Number, int(a.Kind)} }
	var got, wantPairs [][3]int
	for _, a := range m.chart.aspects {
		got = append(got, pair(a))
	}
	for _, a := range want {
		wantPairs = append(wantPairs, pair(a))
	}
	less := func(a, b [3]int) int {
		for k := range a {
			if a[k] != b[k] {
				return a[k] - b[k]
			}
		}
		return 0
	}
	slices.SortFunc(got, less)
	slices.SortFunc(wantPairs, less)
	if !slices.Equal(got, wantPairs) {
		t.Errorf("the narrowed Aspects list is %v, want the unmarked rows between marked objects %v", got, wantPairs)
	}
}

// A registry that lists one name twice cannot say which object a file means:
// the name is kept verbatim and said, never guessed.
func TestDuplicateRegistryNameIsKept(t *testing.T) {
	sessionSandbox(t)
	var tm tea.Model = newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen))
	mm := tm.(Model)
	mm.objectRegistry = append(drawnRegistry(),
		aspect.RegistryObject{Object: aspect.Object{Number: 58, Name: "Titan"}},
		aspect.RegistryObject{Object: aspect.Object{Number: 66, Name: "Titan"}})
	mm.lens.Registry = mm.objectRegistry
	tm = mm
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(withSession(perimeter(), store.Session{Version: 3, FloatingBodies: []string{"Titan", "Sun"}}))
	m := tm.(Model)
	idx := len(m.slots.All()) - 1
	if got := m.slots.KeptMarks(idx); !slices.Equal(got, []string{"Titan"}) {
		t.Errorf("kept names = %v, want [Titan]", got)
	}
	if m.slots.ObjectSelected(idx, 58) || m.slots.ObjectSelected(idx, 66) || !m.slots.ObjectSelected(idx, 1) {
		t.Errorf("marks = %v, want the Sun alone", m.slots.SelectedObjects(idx))
	}
	line := "Titan is not an object of this Astrolog, kept in the session"
	if n := countLines(m, line); n != 1 {
		t.Errorf("the Log carries %q %d times, want once", line, n)
	}
}

// A resolved mark whose registry name holds a comma is not written (the reader
// could not read it back as one name); the other marks are.
func TestMarkNameWithACommaIsNotWritten(t *testing.T) {
	sessionSandbox(t)
	m := marksStart(t, lookup.EphemerisAvailability{}, store.Session{Version: 3})
	m.objectRegistry = append(m.objectRegistry, aspect.RegistryObject{Object: aspect.Object{Number: 60, Name: "Foo, Bar"}})
	idx := len(m.slots.All()) - 1
	for _, n := range []int{1, 60, 19} {
		o, _ := aspect.Registry(m.objectRegistry).ByNumber(n)
		m.slots.ToggleObjectSelection(idx, o)
	}
	if got := m.markNames(idx); !slices.Equal(got, []string{"Sun", "Fortune"}) {
		t.Errorf("markNames = %v, want [Sun Fortune] without the comma name", got)
	}
}

// A slot holding only a kept name reserves no marker column (no row can show a
// mark), and still reaches the file.
func TestKeptOnlySlotReservesNoMarkerColumn(t *testing.T) {
	sessionSandbox(t)
	snap := snapshotChart()
	with := startedModel(t, withSession(perimeter(), v1(&snap, true, false, []string{"sedna"}, nil)))
	without := startedModel(t, withSession(perimeter(), v1(&snap, true, false, nil, nil)))
	for _, m := range []*Model{&with, &without} {
		m.chart.positions = fullChartPositions()
		m.focused = PanelBodies
	}
	if a, b := bodiesPanel(with), bodiesPanel(without); a != b {
		t.Errorf("a kept name changed the Bodies panel:\n%s\nwant\n%s", a, b)
	}
	if !with.sessionHoldsMarks(0) {
		t.Error("a slot with only a kept name is not held for the file")
	}
	quitModel(t, with)
	if got := fileLine(t, "    bodies:"); got != "    bodies: sedna" {
		t.Errorf("the quit wrote %q, want %q", got, "    bodies: sedna")
	}
}

// Earth is object 0: only the zero Object (empty name) is no object, so Space
// marks it like any row.
func TestEarthMarksLikeAnyObject(t *testing.T) {
	stubSelectionSeams(t)
	m := selectionModel(t, true)
	m.chart.positions = append(m.chart.positions, aspect.Position{Object: aspect.Object{Number: 0, Name: "Earth"}, Longitude: 200})
	m.bodyCursorIdx = rowOfObject(t, m, 0)
	if !m.mainSelects() {
		t.Fatal("Space is not offered on the Earth row")
	}
	m, _ = pressSpace(t, m)
	if !m.slots.ObjectSelected(m.cursorIdx, 0) {
		t.Fatal("Space did not mark Earth")
	}
	if !m.showLens(m.focusedSlot()).Objects.Only.Has(0) {
		t.Error("the chart's own lens does not narrow to Earth")
	}
	if hl := m.wheelHighlight(); !slices.Equal(hl.Selected, []int{0}) {
		t.Errorf("the wheel highlight's selected tier = %v, want [0]", hl.Selected)
	}
	marked := 0
	for _, r := range strings.Split(bodiesPanel(m), "\n") {
		if strings.Contains(r, "Eart") && strings.ContainsAny(r, "●*") {
			marked++
		}
	}
	if marked != 1 {
		t.Errorf("%d Earth rows show the mark, want 1:\n%s", marked, bodiesPanel(m))
	}
}
