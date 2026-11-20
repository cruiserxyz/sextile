package ui

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/testenv/astrodir"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// session_drawn_test.go: specs/session-drawn-set.md. The session file holds the
// chooser's drawn set as `drawn:`, written only once the chooser was used.

func drawnRegistry() []aspect.RegistryObject {
	return []aspect.RegistryObject{
		{Object: aspect.Object{Number: 1, Name: "Sun"}}, {Object: aspect.Object{Number: 2, Name: "Moon"}}, {Object: aspect.Object{Number: 11, Name: "Chiron"}},
		{Object: aspect.Object{Number: 19, Name: "Fortune"}}, {Object: aspect.Object{Number: 20, Name: "Vertex"}}, {Object: aspect.Object{Number: 21, Name: "East Point"}},
	}
}

func chironAbsent() lookup.EphemerisAvailability {
	return lookup.EphemerisAvailability{
		Absent: map[int]string{11: "seas_18.se1"},
		Names:  map[int]string{11: "Chiron"},
		// An answer from the probe always carries the host's default set (the
		// account of what cannot be drawn is resolved against it).
		Default: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 16, 22, 31},
	}
}

// drawnStart is a cold start whose session names drawn, with the object
// registry and the ephemeris answer already in, as a normal start has them.
func drawnStart(t *testing.T, av lookup.EphemerisAvailability, drawn []string) Model {
	t.Helper()
	var tm tea.Model = newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen))
	mm := tm.(Model)
	mm.objectRegistry = drawnRegistry()
	mm.ephemeris = av
	tm = mm
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(withSession(perimeter(), store.Session{Version: 3, Drawn: drawn}))
	return tm.(Model)
}

// drawnStartAfterProbe is drawnStart with the startup probe's answer arriving
// as it does in a started app, before the session loads.
func drawnStartAfterProbe(t *testing.T, av lookup.EphemerisAvailability, drawn []string) Model {
	t.Helper()
	var tm tea.Model = newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen))
	mm := tm.(Model)
	mm.objectRegistry = drawnRegistry()
	mm, _ = applyEphemerisProbed(mm, EphemerisProbedMsg{Availability: av})
	tm = mm
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(withSession(perimeter(), store.Session{Version: 3, Drawn: drawn}))
	return tm.(Model)
}

func explicitOf(nums ...int) recipe.ObjectPolicy {
	return recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: nums}
}

// drawnRestart starts again from a saved session on the host m was started on:
// the object list is in before the session loads, as a normal start has it.
func drawnRestart(t *testing.T, m Model, av lookup.EphemerisAvailability, sess store.Session) Model {
	t.Helper()
	p := perimeter()
	p.Session, p.HasSession, p.FolderID = sess, true, sess.Folder
	var tm tea.Model = newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen))
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 144, Height: 40})
	tm, _ = applyEphemerisProbed(tm.(Model), EphemerisProbedMsg{Availability: av})
	mm := tm.(Model)
	mm.objectRegistry = m.objectRegistry
	tm, _ = mm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, cmd := tm.Update(p)
	return pumpPatient(t, tm.(Model), cmd)
}

// The operator's finding: unticked Fortune, Vertex and East Point come back
// after a quit and a start. (The name stays short: the test's temp folders are
// in Astrolog's ephemeris path, which it truncates past 255 characters.)
func TestDrawnSetSurvivesARestart(t *testing.T) {
	sessionSandbox(t)
	m, _, _ := ticksStart(t)
	cm := chooserOf(t, m)
	for _, n := range []int{19, 20, 21} {
		cm = tickObject(t, cm, n)
	}
	cm = pressInChooser(t, cm, tea.KeyMsg{Type: tea.KeyEnter})
	want := cm.drawnNames()

	// Quit through the model; the file is what the quit wrote.
	if _, cmd := cm.quit(); cmd == nil {
		t.Fatal("q did not quit")
	}
	b, err := os.ReadFile(store.SessionPath())
	if err != nil {
		t.Fatal(err)
	}
	var drawn string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "drawn: ") {
			drawn = l
		}
	}
	if !strings.HasPrefix(drawn, "drawn: Sun,Moon,Mercury") || drawn != "drawn: "+strings.Join(want, ",") {
		t.Fatalf("the file holds %q, want the whole set %q as one bare-comma line:\n%s", drawn, want, b)
	}
	for _, name := range []string{"Fortune", "Vertex", "East Point"} {
		if strings.Contains(drawn, name) {
			t.Errorf("the file still draws %s: %s", name, drawn)
		}
	}
	if !strings.Contains(drawn, "North Node") {
		t.Errorf("the file lost a body that stayed ticked: %s", drawn)
	}

	// Start again from the file.
	sess, ok, err := store.LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession: ok %v err %v", ok, err)
	}
	m2 := drawnRestart(t, m, cm.ephemeris, sess)
	if got := m2.drawnNames(); !slices.Equal(got, want) {
		t.Errorf("after the restart the drawn set is %q, want %q", got, want)
	}

	left := chooserLeftPanels(ansi.Strip(m2.View()))
	for _, word := range []string{"· Fort", "· Vert", "· East"} {
		if strings.Contains(left["bodies"], word) {
			t.Errorf("after the restart the Bodies panel shows %q:\n%s", word, left["bodies"])
		}
	}
	if !strings.Contains(left["bodies"], "Sun") {
		t.Errorf("after the restart the Bodies panel lost the Sun:\n%s", left["bodies"])
	}
	c2 := chooserOf(t, m2)
	for _, n := range []int{19, 20, 21} {
		if c2.bodyModal.Selected[n] {
			t.Errorf("after the restart the chooser ticks object %d", n)
		}
	}
	if !c2.bodyModal.Selected[1] {
		t.Error("after the restart the chooser does not tick the Sun")
	}
}

// A session that never opened the chooser writes the bytes 0.1.2 wrote: the
// fixture is typed out here, not produced by the writer under test.
func TestDrawnSetIsWrittenOnlyWhenTheChooserWasTouched(t *testing.T) {
	sessionSandbox(t)
	seedPreferences(t)
	m := startedModel(t, realPerimeter(t))
	m.objectRegistry = drawnRegistry()
	quitModel(t, m)
	b, err := os.ReadFile(store.SessionPath())
	if err != nil {
		t.Fatal(err)
	}
	want := "version: 4\nfolder: " + store.FolderID(store.ChartsDir()) + "\nfloating:\n  selected: false\n"
	if string(b) != want {
		t.Errorf("the untouched session is not the file this build writes:\n%s\nwant\n%s", b, want)
	}
}

// The chooser, used and confirmed, puts the line in the file the quit writes.
func TestDrawnSetTouchedIsWritten(t *testing.T) {
	sessionSandbox(t)
	m, _, _ := ticksStart(t)
	cm := tickObject(t, chooserOf(t, m), 5)
	cm = pressInChooser(t, cm, tea.KeyMsg{Type: tea.KeyEnter})
	want := cm.drawnNames()
	if _, cmd := cm.quit(); cmd == nil {
		t.Fatal("q did not quit")
	}
	line := fileLine(t, "drawn:")
	if !strings.HasPrefix(line, "drawn: Sun,Moon,Mercury,Venus,Jupiter,Saturn") || line != "drawn: "+strings.Join(want, ",") {
		t.Errorf("the file holds %q, want the whole set %q as one bare-comma line", line, want)
	}
}

// A name this Astrolog does not list is kept: said once, not drawn, and still
// in the file after the quit.
func TestDrawnSetKeepsAnUnknownName(t *testing.T) {
	sessionSandbox(t)
	m, _, _ := ticksStart(t)
	sess := store.Session{Version: 3, Folder: store.FolderID(store.ChartsDir()),
		Drawn: []string{"Sun", "Moon", "Nibiru"}}
	m2 := drawnRestart(t, m, m.ephemeris, sess)

	const line = "Nibiru is not known to this Astrolog, the session keeps its name"
	if n := countLines(m2, line); n != 1 {
		t.Errorf("the Log carries %q %d times, want once:\n%s", line, n, strings.Join(logTexts(m2), "\n"))
	}
	left := chooserLeftPanels(ansi.Strip(m2.View()))
	if !strings.Contains(left["bodies"], "Sun") || strings.Contains(left["bodies"], "Mars") || strings.Contains(left["bodies"], "Nibiru") {
		t.Errorf("the Bodies panel is not the Sun and the Moon alone:\n%s", left["bodies"])
	}
	m2.quit()
	b, _ := os.ReadFile(store.SessionPath())
	if !strings.Contains(string(b), "\ndrawn: Sun,Moon,Nibiru\n") {
		t.Errorf("the file after the quit does not hold the name:\n%s", b)
	}

	// A set of unknown names alone shows the defaults and says so, once.
	sess.Drawn = []string{"Nibiru"}
	m3 := drawnRestart(t, m, m.ephemeris, sess)
	const subst = "the bodies of the last session can't be drawn here, the usual ones are shown"
	for _, l := range []string{line, subst} {
		if n := countLines(m3, l); n != 1 {
			t.Errorf("the Log carries %q %d times, want once:\n%s", l, n, strings.Join(logTexts(m3), "\n"))
		}
	}
	m3.quit()
	if got := fileLine(t, "drawn:"); got != "drawn: Nibiru" {
		t.Errorf("the file holds %q, want drawn: Nibiru", got)
	}
}

func TestDrawnSetThatDrainsFallsBack(t *testing.T) {
	m := drawnStart(t, chironAbsent(), []string{"Chiron"})
	if m.lens.Objects.Baseline == recipe.Explicit {
		t.Errorf("a drawn set with nothing drawable is in force: %v", m.lens.Objects.Selected())
	}
	const line = "Chiron selected but can't be drawn here, all are shown"
	if n := countLines(m, line); n != 1 {
		t.Errorf("the Log carries %q %d times, want once:\n%s", line, n, strings.Join(logTexts(m), "\n"))
	}

	// Mixed with a drawable body, the rest is drawn and the absent one named.
	// A started app has the startup probe's answer in before the session loads,
	// and the lens's default view (which draws Chiron) names it then: once, and
	// not again by the restore.
	m = drawnStartAfterProbe(t, chironAbsent(), []string{"Sun", "Chiron"})
	if got := m.lens.Objects.Selected(); !slices.Equal(got, []int{1}) {
		t.Errorf("the restored set is %v, want the Sun alone", got)
	}
	const partial = "Chiron can't be drawn here: no file seas_18.se1"
	if n := countLines(m, partial); n != 1 {
		t.Errorf("the Log carries %q %d times, want once:\n%s", partial, n, strings.Join(logTexts(m), "\n"))
	}

	// Names that all fail to resolve fall back too, and say which.
	m = drawnStart(t, lookup.EphemerisAvailability{}, []string{"Nibiru"})
	if m.lens.Objects.Baseline == recipe.Explicit {
		t.Error("a drawn set of unknown names is in force")
	}
	if got := m.drawnNames(); !slices.Equal(got, []string{"Nibiru"}) {
		t.Errorf("the file would keep %q, want Nibiru", got)
	}
}

func TestOldSessionWithoutDrawnRestoresTheDefaults(t *testing.T) {
	m := drawnStart(t, lookup.EphemerisAvailability{}, nil)
	if m.lens.Objects.Baseline != recipe.AstrologDefault || m.lens.Objects.Selected() != nil {
		t.Errorf("a session without drawn: put a set in force: %+v", m.lens.Objects)
	}
	for _, l := range logTexts(m) {
		if strings.Contains(l, "drawn") {
			t.Errorf("a session without drawn: said %q", l)
		}
	}
}

// The not-written cases stay not written: the refused file is a link to a
// target the quit must leave byte for byte, whatever the chooser did.
func TestRefusedSessionIsNotRewrittenWithDrawn(t *testing.T) {
	sessionSandbox(t)
	seedPreferences(t)
	target := t.TempDir() + "/elsewhere.yaml"
	if err := os.WriteFile(target, []byte(refusedSession), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.SessionDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, store.SessionPath()); err != nil {
		t.Fatal(err)
	}
	ino, mod := fileIdentity(t, target)
	m := startedModel(t, realPerimeter(t))
	m.objectRegistry = drawnRegistry()
	m.lens.Objects = explicitOf(1, 2)
	m = quitModel(t, m)
	if i, mt := fileIdentity(t, target); i != ino || !mt.Equal(mod) {
		t.Errorf("the refused session's target was rewritten (inode %d -> %d, modified %v -> %v)", ino, i, mod, mt)
	}
	if b, _ := os.ReadFile(target); string(b) != refusedSession {
		t.Errorf("the target changed: %q", b)
	}
	if m.SessionWriteError() == nil {
		t.Error("the refused session's quit reported no kept file")
	}
}

func TestNamedSessionCarriesDrawn(t *testing.T) {
	m := sessionsModel(t, 1)
	t.Cleanup(store.ReleaseInstanceLock)
	m.objectRegistry = drawnRegistry()
	m.lens.Objects = explicitOf(1, 2)
	m = saveAs(t, m, "drawn-set")
	if !m.named.running() {
		t.Fatalf("the save did not make the window run the session (refusal %q)", m.sessions.Refusal)
	}
	b, err := os.ReadFile(store.NamedSessionPath(m.named.file))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "\ndrawn: Sun,Moon\n") {
		t.Fatalf("the named session lacks the drawn line:\n%s", b)
	}

	// Opened in another window, the set is back.
	sess, _, err := store.LoadNamedSession(m.named.file)
	if err != nil {
		t.Fatal(err)
	}
	m2 := listModel(t, 1, 0)
	m2.objectRegistry = drawnRegistry()
	p := withSession(perimeter(), sess)
	p.Session.Folder = sess.Folder
	m2, _ = applyNamedSessionLoaded(m2, NamedSessionLoadedMsg{Gen: m2.sessionGen, Name: "drawn-set", Perimeter: p})
	if got := m2.lens.Objects.Selected(); !slices.Equal(got, []int{1, 2}) {
		t.Errorf("opening the named session restored %v, want Sun and Moon", got)
	}
}

// The first frame waits for the object list when the session names a drawn
// set, so no frame shows the defaults first.
func TestDrawnSetWaitsForTheObjectList(t *testing.T) {
	var tm tea.Model = withoutObjectList(newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen)))
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(withSession(perimeter(), store.Session{Version: 3, Drawn: []string{"Sun", "Moon"}}))
	if tm.(Model).slots != nil {
		t.Fatal("the workspace was built before the object list arrived")
	}
	tm, _ = tm.Update(ObjectRegistryLoadedMsg{Registry: drawnRegistry()})
	m := tm.(Model)
	if m.slots == nil {
		t.Fatal("the workspace was not built when the object list arrived")
	}
	if got := m.lens.Objects.Selected(); !slices.Equal(got, []int{1, 2}) {
		t.Errorf("the restored set is %v, want Sun and Moon", got)
	}
}

func TestNamedSessionWithoutDrawnRestoresDefaults(t *testing.T) {
	m := listModel(t, 1, 0)
	m.objectRegistry = drawnRegistry()
	m.lens.Objects = explicitOf(1, 2)
	m.drawnKept = []string{"Nibiru"}
	p := withSession(perimeter(), store.Session{Version: 3})
	m, _ = applyNamedSessionLoaded(m, NamedSessionLoadedMsg{Gen: m.sessionGen, Name: "plain", Perimeter: p})
	if m.lens.Objects.Baseline != recipe.AstrologDefault || !slices.Contains(visibleObjects(t, m.lens), 5) {
		t.Errorf("the defaults did not come back: %+v", m.lens.Objects)
	}
	if got := m.drawnNames(); got != nil {
		t.Errorf("the previous session's names stay: %q", got)
	}
	if c := popup.Open(m.objectRegistry, m.lens.Objects, visibleObjects(t, m.lens)); !c.Selected[5] {
		t.Error("the chooser does not tick Mars")
	}
}

// The first build waits for the object list through a city ask too, and the
// saved set resolves once the city is picked.
func TestDrawnSetSurvivesTheCityAsk(t *testing.T) {
	p := withSession(ingest.StartupPerimeterReadMsg{
		LocationCandidate: location.Candidate{Source: "stored-location", Place: location.RawPlace{Name: "Atlantis", Country: "??"}},
	}, store.Session{Version: 3, Drawn: []string{"Sun", "Moon", "Nibiru"}})
	var tm tea.Model = withoutObjectList(newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen)))
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(p)
	if tm.(Model).startup == startupAtlasOpen {
		t.Fatal("the city was asked before the object list arrived")
	}
	tm, _ = tm.Update(ObjectRegistryLoadedMsg{Registry: drawnRegistry()})
	m := tm.(Model)
	if m.startup != startupAtlasOpen {
		t.Fatalf("startup = %q, want the atlas modal open", m.startup)
	}
	m, _ = applyAtlasPicked(m, AtlasPickedMsg{Place: berlinP})
	if got := m.lens.Objects.Selected(); !slices.Equal(got, []int{1, 2}) {
		t.Errorf("after the city pick the set is %v, want Sun and Moon", got)
	}
	if got := m.drawnNames(); !slices.Equal(got, []string{"Sun", "Moon", "Nibiru"}) {
		t.Errorf("the file would hold %q", got)
	}
}

// A body with no ephemeris file is not drawn, said by the existing line, and
// still in the file after the quit.
func TestDrawnSetKeepsAnAbsentBody(t *testing.T) {
	sessionSandbox(t)
	m, _, _ := ticksStart(t)
	sess := store.Session{Version: 3, Folder: store.FolderID(store.ChartsDir()),
		Drawn: []string{"Sun", "Moon", "Chiron"}}
	m2 := drawnRestart(t, m, m.ephemeris, sess)
	if got := m2.lens.Objects.Selected(); !slices.Equal(got, []int{1, 2}) {
		t.Errorf("the restored set is %v, want Sun and Moon", got)
	}
	// The host's default set already names Chiron: said once, by that account.
	if n := countLines(m2, "can't be drawn here"); n != 1 {
		t.Errorf("the Log says which bodies lack a file %d times, want once:\n%s", n, strings.Join(logTexts(m2), "\n"))
	}
	if c := chooserOf(t, m2); c.bodyModal.Selected[11] {
		t.Error("the chooser ticks Chiron")
	}
	m2.quit()
	b, _ := os.ReadFile(store.SessionPath())
	if !strings.Contains(string(b), "\ndrawn: Sun,Moon,Chiron\n") {
		t.Errorf("the file after the quit lost Chiron:\n%s", b)
	}
}

// reloaded is the session the last quit wrote, as a launch would read it.
func reloaded(t *testing.T) store.Session {
	t.Helper()
	sess, ok, err := store.LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession: ok %v err %v", ok, err)
	}
	return sess
}

func fileLine(t *testing.T, prefix string) string {
	t.Helper()
	b, _ := os.ReadFile(store.SessionPath())
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	return ""
}

// Removing the last drawable tick falls back to the defaults, as the restore
// does, and the next launch shows the same.
func TestDrawnSetLastTickFallsBack(t *testing.T) {
	sessionSandbox(t)
	m, _, _ := ticksStart(t)
	sess := store.Session{Version: 3, Folder: store.FolderID(store.ChartsDir()), Drawn: []string{"Sun", "Chiron"}}
	m2 := drawnRestart(t, m, m.ephemeris, sess)
	if got := m2.lens.Objects.Selected(); !slices.Equal(got, []int{1}) {
		t.Fatalf("fixture: the restored set is %v, want the Sun", got)
	}
	c := tickObject(t, chooserOf(t, m2), 1)
	c = pressInChooser(t, c, tea.KeyMsg{Type: tea.KeyEnter})
	if c.lens.Objects.Baseline == recipe.Explicit {
		t.Errorf("an empty chooser result is in force: %+v", c.lens.Objects)
	}
	left := chooserLeftPanels(ansi.Strip(c.View()))
	if !strings.Contains(left["bodies"], "· Fort") || !strings.Contains(left["bodies"], "Mars") {
		t.Errorf("the Bodies panel does not show the defaults:\n%s", left["bodies"])
	}
	const line = "Chiron selected but can't be drawn here, all are shown"
	if n := countLines(c, line); n != 1 {
		t.Errorf("the Log carries %q %d times, want once:\n%s", line, n, strings.Join(logTexts(c), "\n"))
	}
	c.quit()
	if got := fileLine(t, "drawn:"); got != "drawn: Chiron" {
		t.Errorf("the file holds %q, want drawn: Chiron", got)
	}
	m3 := drawnRestart(t, m, m.ephemeris, reloaded(t))
	left = chooserLeftPanels(ansi.Strip(m3.View()))
	if m3.lens.Objects.Baseline == recipe.Explicit || !strings.Contains(left["bodies"], "Mars") {
		t.Errorf("the restart does not show the defaults:\n%s", left["bodies"])
	}
}

// A set of absent bodies alone is kept through a quit and a start, and the
// defaults are what the panel shows.
func TestDrawnSetKeepsAnAbsentOnlySet(t *testing.T) {
	sessionSandbox(t)
	m, _, _ := ticksStart(t)
	sess := store.Session{Version: 3, Folder: store.FolderID(store.ChartsDir()), Drawn: []string{"Chiron"}}
	for i := 0; i < 2; i++ {
		m2 := drawnRestart(t, m, m.ephemeris, sess)
		left := chooserLeftPanels(ansi.Strip(m2.View()))
		if m2.lens.Objects.Baseline == recipe.Explicit || !strings.Contains(left["bodies"], "Mars") {
			t.Fatalf("round %d: the defaults are not shown:\n%s", i, left["bodies"])
		}
		m2.quit()
		if got := fileLine(t, "drawn:"); got != "drawn: Chiron" {
			t.Fatalf("round %d: the file holds %q, want drawn: Chiron", i, got)
		}
		sess = reloaded(t)
	}
}

// An absent body the startup account already named is not named again.
func TestDrawnSetAbsentDefaultSaidOnce(t *testing.T) {
	sessionSandbox(t)
	astrodir.Reduced(t, []string{"seas_18.se1", "seas_12.se1"}, "-R 12")
	m, _ := liveScreens(t)
	reg, err := lookup.ObjectRegistry(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m.objectRegistry = reg
	sess := store.Session{Version: 3, Folder: store.FolderID(store.ChartsDir()), Drawn: []string{"Sun", "Ceres"}}
	m2 := drawnRestart(t, m, m.ephemeris, sess)
	n := 0
	for _, l := range logTexts(m2) {
		if strings.Contains(l, "Ceres") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("Ceres is named %d times in the Log, want once:\n%s", n, strings.Join(logTexts(m2), "\n"))
	}
}

// A list that fails after the gate is armed is a can't-start (objects slice
// 0b): the screen names the error, nothing is cast, and the names the session
// held are not written, because no session is.
func TestDrawnSetListErrorCannotStart(t *testing.T) {
	sessionSandbox(t)
	var tm tea.Model = withoutObjectList(newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen)))
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(withSession(perimeter(), store.Session{Version: 3, Drawn: []string{"Sun", "Nibiru"}}))
	if tm.(Model).slots != nil {
		t.Fatal("fixture: the gate was not armed")
	}
	tm, _ = tm.Update(ObjectRegistryLoadedMsg{Err: errors.New("unreadable")})
	m := tm.(Model)
	if want := "Astrolog's list of bodies could not be read: unreadable"; !strings.Contains(csText(m.View()), want) {
		t.Errorf("the can't-start screen lacks %q:\n%s", want, csText(m.View()))
	}
	if m.slots != nil || len(argvTexts(m)) != 0 {
		t.Error("a chart was cast without the object list")
	}
	m.quit()
	if _, err := os.Stat(store.SessionPath()); err == nil {
		t.Errorf("a session was written: %q", fileLine(t, "drawn:"))
	}
}

// A quit while the start still waits for the list leaves the file as it was.
func TestQuitWhileWaitingKeepsTheFile(t *testing.T) {
	sessionSandbox(t)
	seedPreferences(t)
	text := hereText("version: 3\ndrawn: Sun, Nibiru\nfloating:\n  selected: false\n")
	path := seedSession(t, text)
	ino, mod := fileIdentity(t, path)
	m := startedModel(t, realPerimeter(t))
	if m.slots != nil {
		t.Fatal("fixture: the start did not wait for the list")
	}
	m.quit()
	if i, mt := fileIdentity(t, path); i != ino || !mt.Equal(mod) {
		t.Errorf("the session was rewritten (inode %d -> %d, modified %v -> %v)", ino, i, mod, mt)
	}
	if b, _ := os.ReadFile(path); string(b) != text {
		t.Errorf("the session changed:\n%s", b)
	}
}

// Opening a named file written by 0.1.2 (no drawn line) while a saved set still
// waits for the list brings the defaults back, and nothing returns when the
// list lands.
func TestNamedFileWithoutDrawnClearsAStaleWait(t *testing.T) {
	m := sessionsModel(t, 2)
	file := otherSession(t, "Plain work", "Client Zed")
	if b, err := os.ReadFile(store.NamedSessionPath(file)); err != nil || strings.Contains(string(b), "drawn") {
		t.Fatalf("fixture: the named file is not a 0.1.2 file (%v):\n%s", err, b)
	}
	m.drawnRaw = []string{"Sun"}
	m = openSessions(t, m)
	m, _ = press(t, m, "down", "down")
	m, cmd := press(t, m, "enter")
	if cmd == nil {
		t.Fatal("Enter on a saved session started nothing")
	}
	tm, _ := m.Update(cmd())
	tm, _ = tm.Update(ObjectRegistryLoadedMsg{Registry: drawnRegistry()})
	m = tm.(Model)
	if m.named.file != file {
		t.Fatalf("the window runs %q, want %q", m.named.file, file)
	}
	if m.lens.Objects.Baseline == recipe.Explicit || m.drawnNames() != nil {
		t.Errorf("the stale set came back: %+v %q", m.lens.Objects, m.drawnNames())
	}
	m.quit()
	if b, _ := os.ReadFile(store.NamedSessionPath(file)); strings.Contains(string(b), "drawn") {
		t.Errorf("the named file gained a drawn line:\n%s", b)
	}
}

// A named session opened by name keeps a drawn set changed in the chooser
// through the quit.
func TestNamedSessionKeepsAChangedDrawnSet(t *testing.T) {
	m := sessionsModel(t, 2)
	m.objectRegistry = drawnRegistry()
	file := otherSession(t, "Other work", "Client Zed")
	m = openSessions(t, m)
	m, _ = press(t, m, "down", "down")
	m, cmd := press(t, m, "enter")
	tm, _ := m.Update(cmd())
	m = tm.(Model)
	m.startup = startupReady
	m = chooserOf(t, m)
	m.bodyModal.Cursor = popup.GroupRows() + slices.IndexFunc(m.bodyModal.Matches, func(o aspect.RegistryObject) bool { return o.Number == 1 })
	tm, _ = applyBodyModalKey(m, tea.KeyMsg{Type: tea.KeySpace})
	tm, _ = applyBodyModalKey(tm.(Model), tea.KeyMsg{Type: tea.KeyEnter})
	tm.(Model).quit()
	sess, _, err := store.LoadNamedSession(file)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(sess.Drawn, "Moon") || slices.Contains(sess.Drawn, "Sun") {
		t.Errorf("the named session holds drawn %q, want the Moon and not the Sun", sess.Drawn)
	}
}

// New unnamed session starts at the defaults.
func TestResetStartsAtTheDefaultDrawnSet(t *testing.T) {
	m := namedResetModel(t)
	m.objectRegistry = drawnRegistry()
	m.lens.Objects = explicitOf(1)
	m.drawnKept = []string{"Nibiru"}
	m = resetKey(t, m)
	if m.lens.Objects.Baseline == recipe.Explicit || m.drawnNames() != nil || !slices.Contains(visibleObjects(t, m.lens), 5) {
		t.Errorf("the new session kept the drawn set: %+v %q", m.lens.Objects, m.drawnNames())
	}
	quitWindow(t, m)
	if b, _ := os.ReadFile(store.SessionPath()); strings.Contains(string(b), "drawn:") {
		t.Errorf("the general session gained a drawn line:\n%s", b)
	}
}

// An unnamed session holding nothing but a chooser set is not empty to the New row.
func TestResetDropsADrawnSetOnAnEmptySession(t *testing.T) {
	sessionSandbox(t)
	m := listModel(t, 0, 0)
	m.objectRegistry = drawnRegistry()
	m.lens.Objects = explicitOf(1)
	m, _ = pressReset(t, m)
	if m.lens.Objects.Baseline == recipe.Explicit {
		t.Errorf("the New row left the set in force: %+v", m.lens.Objects)
	}
}

// drawnNamed saves a named session holding drawn, as another window would, and
// lets go of its lock.
func drawnNamed(t *testing.T, name string, drawn []string) string {
	t.Helper()
	s := store.Session{Folder: store.FolderID(store.ChartsDir()), Drawn: drawn,
		Slots: []store.SessionSlot{{
			Chart: store.SessionChart{ID: "zed", Name: "Client Zed", When: time.Date(1977, 7, 7, 7, 7, 0, 0, time.UTC),
				Place: location.PlaceInfo{Name: "Testford", Country: "TS", Lat: 41, Lon: -71, TZ: "America/New_York"}},
			Usable: true,
		}}}
	l, err := store.SaveNamedSessionAs(name, s)
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	return l.File()
}

// openNamed is `, o`, down, Enter and the read landing, on a model whose
// session list holds file.
func openNamed(t *testing.T, m Model) Model {
	t.Helper()
	m = openSessions(t, m)
	m, _ = press(t, m, "down", "down")
	m, cmd := press(t, m, "enter")
	if cmd == nil {
		t.Fatal("Enter on a saved session started nothing")
	}
	tm, _ := m.Update(cmd())
	return tm.(Model)
}

// A read still in flight when a named session with drawn: opens: the names wait
// as read, and resolve when the read lands.
func TestDrawnSetResolvesWhenTheListLands(t *testing.T) {
	m := withoutObjectList(sessionsModel(t, 2))
	drawnNamed(t, "Waiting", []string{"Sun", "Nibiru"})
	m = openNamed(t, m)
	if m.lens.Objects.Baseline == recipe.Explicit || m.drawnNames() == nil {
		t.Fatalf("fixture: the names did not wait (%+v %q)", m.lens.Objects, m.drawnNames())
	}
	tm, _ := m.Update(ObjectRegistryLoadedMsg{Registry: drawnRegistry()})
	m = tm.(Model)
	if got := m.lens.Objects.Selected(); !slices.Equal(got, []int{1}) {
		t.Errorf("after the list the set is %v, want the Sun", got)
	}
	if got := m.drawnNames(); !slices.Equal(got, []string{"Sun", "Nibiru"}) {
		t.Errorf("after the list the file would hold %q", got)
	}
}

// The same on the real binary: the recompute fires and asks for the Sun and
// the Moon.
func TestDrawnSetLateListRepaints(t *testing.T) {
	sessionSandbox(t)
	m, read, _ := ticksStart(t)
	reg := m.objectRegistry
	m.objectRegistry = nil // the read is still in flight
	drawnNamed(t, "Waiting", []string{"Sun", "Moon"})
	m = openNamed(t, m)
	if m.lens.Objects.Baseline == recipe.Explicit {
		t.Fatal("fixture: the set was in force before the list")
	}
	mark := len(read())
	tm, cmd := m.Update(ObjectRegistryLoadedMsg{Registry: reg})
	if cmd == nil {
		t.Fatal("the list landing started no recompute")
	}
	m3 := pumpPatient(t, tm.(Model), cmd)
	argv := textArgv(read()[mark:])
	if explicit, _ := objectLists(argv); !slices.Equal(explicit, []int{1, 2}) {
		t.Errorf("the recompute asked for %v, want the Sun and the Moon: %v", explicit, band(argv))
	}
	if got := m3.lens.Objects.Selected(); !slices.Equal(got, []int{1, 2}) {
		t.Errorf("the set is %v", got)
	}
}

// A named file holding the bare line, as earlier 0.1.3 builds wrote it for an
// empty choice, opens as no choice: the default set, said once, and the quit
// drops the line.
func TestBareDrawnNamedSessionOpensDefaults(t *testing.T) {
	m := sessionsModel(t, 2)
	m.objectRegistry = drawnRegistry()
	file := drawnNamed(t, "Bare", []string{})
	m = openNamed(t, m)
	if m.lens.Objects.Baseline == recipe.Explicit {
		t.Errorf("the bare line opened as a chosen set: %+v", m.lens.Objects)
	}
	if n := countLines(m, noBodiesChosen); n != 1 {
		t.Errorf("the Log carries %q %d times, want once:\n%s", noBodiesChosen, n, said(m))
	}
	if c := popup.Open(m.objectRegistry, m.lens.Objects, visibleObjects(t, m.lens)); !c.Selected[5] {
		t.Error("the chooser does not tick Mars")
	}
	m.quit()
	if b, _ := os.ReadFile(store.NamedSessionPath(file)); strings.Contains(string(b), "drawn") {
		t.Errorf("the bare line survived the quit:\n%s", b)
	}
}

// A drained notice belongs to the session that drained: absent-only, then a
// session without drawn:, then absent-only again says it again.
func TestNamedSessionSaysItsDrainAgain(t *testing.T) {
	m := listModel(t, 1, 0)
	m.objectRegistry = drawnRegistry()
	m.ephemeris = chironAbsent()
	load := func(m Model, drawn []string) Model {
		p := withSession(perimeter(), store.Session{Version: 3, Drawn: drawn})
		m, _ = applyNamedSessionLoaded(m, NamedSessionLoadedMsg{Gen: m.sessionGen, Name: "n", Perimeter: p})
		return m
	}
	const line = "Chiron selected but can't be drawn here, all are shown"
	m = load(m, []string{"Chiron"})
	m = load(m, nil)
	m = load(m, []string{"Chiron"})
	if n := countLines(m, line); n != 2 {
		t.Errorf("the Log carries %q %d times, want twice:\n%s", line, n, strings.Join(logTexts(m), "\n"))
	}
}

// Two missing files, still one line: the selected bodies named together.
func TestDrawnSetTwoMissingFiles(t *testing.T) {
	sessionSandbox(t)
	astrodir.Reduced(t, []string{"seas_18.se1", "se00010s.se1"}, "-R 11 12")
	m, _ := liveScreens(t)
	reg, err := lookup.ObjectRegistry(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m.objectRegistry = reg
	folder := store.FolderID(store.ChartsDir())
	both := drawnRestart(t, m, m.ephemeris, store.Session{Version: 3, Folder: folder, Drawn: []string{"Chiron", "Hygiea"}})
	var drained []string
	for _, l := range logTexts(both) {
		if strings.Contains(l, "can't be drawn here, all are shown") {
			drained = append(drained, l)
		}
	}
	// (Both names and the sentence are wider than the Log, so the general form.)
	want := []string{"bodies: the selected bodies can't be drawn here, all are shown"}
	if !slices.Equal(drained, want) {
		t.Errorf("drained lines = %q, want %q", drained, want)
	}
	mixed := drawnRestart(t, m, m.ephemeris, store.Session{Version: 3, Folder: folder, Drawn: []string{"Sun", "Chiron", "Hygiea"}})
	if got := mixed.lens.Objects.Selected(); !slices.Equal(got, []int{1}) {
		t.Errorf("the restored set is %v, want the Sun", got)
	}
	if n := countLines(mixed, "Hygiea can't be drawn here: no file se00010s.se1"); n != 1 {
		t.Errorf("Hygiea's line appears %d times, want once:\n%s", n, strings.Join(logTexts(mixed), "\n"))
	}
}

// A general session saved with the old ", " join restores the same drawn set
// through the file reader, and the quit writes it back with the bare comma
// (spec session-drawn-bare-comma, rule 2; independent review, round 1, finding 3).
func TestOldDrawnJoinRewrittenAtQuit(t *testing.T) {
	sessionSandbox(t)
	seedSession(t, hereText("version: 3\ndrawn: Sun, Moon\nfloating:\n  selected: false\n"))
	sess, ok, err := store.LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession: ok %v err %v", ok, err)
	}
	m := drawnStart(t, lookup.EphemerisAvailability{}, sess.Drawn)
	if got := m.drawnNames(); !slices.Equal(got, []string{"Sun", "Moon"}) {
		t.Fatalf("restored %q, want Sun and Moon", got)
	}
	m = quitModel(t, m)
	if got := fileLine(t, "drawn:"); got != "drawn: Sun,Moon" {
		t.Errorf("the quit wrote %q, want the bare-comma line", got)
	}
}

// visibleObjects is the lens's drawn numbers, failing the test when the host's
// default object set is not known (the TestMain installs it).
func visibleObjects(t *testing.T, l lookup.Lens) []int {
	t.Helper()
	got, err := l.VisibleObjectNumbers()
	if err != nil {
		t.Fatal(err)
	}
	return got
}
