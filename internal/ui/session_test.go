package ui

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// All chart values in this file are constructed synthetic test inputs.

// Verifiers for an earlier change: the natal anchor and its selections come back on
// relaunch. The write end is one file on the way out; the read end is what
// startup anchors from.

var (
	sessionFrozen  = time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	fixtureOneWhen = time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC)

	berlinRaw = location.RawPlace{
		Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin",
		LatPresent: true, LonPresent: true,
	}
)

// sessionSandbox points every file the session touches at a temp directory.
// The quit path WRITES, so this is not optional: without it the suite would
// overwrite the operator's own session on any host that has one.
func sessionSandbox(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("SEXTILE_CHARTS_DIR", filepath.Join(root, "charts"))
	t.Setenv("SEXTILE_DEV_CHARTS_DIR", "")
	t.Setenv("HOME", root)
	return filepath.Join(root, "state", "sextile", "session")
}

func seedSession(t *testing.T, text string) string {
	t.Helper()
	dir := store.SessionDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := store.SessionPath()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// perimeter is a cold-start read with a location that resolves on its own, so
// startup builds a workspace without asking for a city.
func perimeter() ingest.StartupPerimeterReadMsg {
	return ingest.StartupPerimeterReadMsg{
		LocationCandidate: location.Candidate{Source: "stored-location", Place: berlinRaw},
	}
}

func snapshotChart() store.SessionChart {
	return store.SessionChart{ID: "fixture-one", Name: "Constructed One", When: fixtureOneWhen, Place: fixtureOneP}
}

// withSession attaches a session that carries the snapshot and nothing else,
// saved in the charts folder this launch uses.
func withSession(p ingest.StartupPerimeterReadMsg, s store.Session) ingest.StartupPerimeterReadMsg {
	s.Folder = testFolderID
	p.Session = s
	p.HasSession = true
	p.FolderID = testFolderID
	return p
}

// here stamps s as saved in the charts folder this test launches in, as a
// quit there would have.
func here(s store.Session) store.Session {
	s.Folder = store.FolderID(store.ChartsDir())
	return s
}

// hereText is a seeded session file's text, stamped the same way: the folder
// line goes straight after the version line.
func hereText(text string) string {
	line := "folder: " + store.FolderID(store.ChartsDir()) + "\n"
	if i := strings.IndexByte(text, '\n'); i >= 0 && strings.HasPrefix(text, "version:") {
		return text[:i+1] + line + text[i+1:]
	}
	return line + text
}

// testFolderID is the charts folder a hand-built perimeter read says it is in.
var testFolderID = strings.Repeat("5e", 32)

// startedModel drives a cold start to the point where the workspace exists:
// the atlas index lands, then the perimeter read.
func startedModel(t *testing.T, p ingest.StartupPerimeterReadMsg) Model {
	t.Helper()
	var m tea.Model = newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen))
	// A saved drawn set keeps waiting for the list, as the tests of that wait
	// need; every other start has it in.
	if p.HasSession && p.Session.Drawn != nil {
		m = withoutObjectList(m.(Model))
	}
	m, _ = m.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	m, _ = m.Update(p)
	return m.(Model)
}

// withoutObjectList is a Model whose start has not heard of the object list
// yet: the tests of the wait for it build their Model so.
func withoutObjectList(m Model) Model {
	m.objectRegistry = nil
	m.lens.Registry = nil
	return m
}

// withObjectList is a Model that has Astrolog's object list in before its
// start, as a normal start has it (objects slice 0b: no chart is cast before
// it). A test that hand-builds a start and does not mean to test the wait
// takes it from here.
func withObjectList(m Model) Model {
	reg := startObjectList()
	m.objectRegistry = reg
	m.lens.Registry = reg
	return withAspectList(m)
}

// withAspectList is a Model that has Astrolog's aspect list in, as a normal
// start has it before any chart is cast.
func withAspectList(m Model) Model {
	m.lens.AspectRegistry = testAspects()
	return m
}

// withoutAspectList is a Model whose start has not heard of the aspect list
// yet: the tests of the wait for it build their Model so.
func withoutAspectList(m Model) Model {
	m.lens.AspectRegistry = aspect.AspectRegistry{}
	return m
}

var (
	startListOnce sync.Once
	startList     []aspect.RegistryObject
)

// startObjectList is the binary's own object list, read once for the test
// binary; the six-object list the drawn-set tests use stands in when there is
// no Astrolog to ask. Calls name their rows through it, so a list that lacked
// Mercury would show Mercury as a plain registry name.
func startObjectList() []aspect.RegistryObject {
	startListOnce.Do(func() {
		if reg, err := lookup.ObjectRegistry(context.Background()); err == nil {
			startList = reg
			return
		}
		startList = drawnRegistry()
	})
	return startList
}

// sessionLines is every Log line the restore wrote, in order. Startup writes
// other lines (the active city, resolver warnings); C15 governs these.
func sessionLines(m Model) []string {
	var out []string
	for _, l := range m.logLines {
		if strings.HasPrefix(l.text, "session:") {
			out = append(out, l.text)
		}
	}
	return out
}

func anchoredSlot(t *testing.T, m Model) *store.Slot {
	t.Helper()
	if m.slots == nil {
		t.Fatal("no workspace was built")
	}
	a := firstAnchor(m.slots)
	if a == nil {
		t.Fatal("nothing is anchored")
	}
	return a
}

// anchorFixture builds a ready workspace with a chart anchored the way a pick
// anchors one: resolve first, then anchor. It is the state a quit writes down.
func anchorFixture(t *testing.T) Model {
	t.Helper()
	m := readyModelWithSlots(t, berlinP)
	chart := store.NamedChart{
		ID: "fixture-one", Name: "Constructed One", Year: 2001, Place: fixtureOneP, PlaceLabel: "Test Bay",
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "fixture-one.as"},
	}
	picked, cmd := applyChartPicked(m, ChartPickedMsg{Chart: chart})
	if cmd == nil {
		t.Fatal("a pick must dispatch a resolve")
	}
	got, _ := resolved(t, picked, fixtureOneWhen)
	return got
}

// C4: the pick's identity survives the resolve, because the slot it anchors
// knows only when and where.
func TestChartPickKeepsAnchorRecord(t *testing.T) {
	m := anchorFixture(t)
	want := store.SessionChart{ID: "fixture-one", Name: "Constructed One", When: fixtureOneWhen, Place: fixtureOneP}
	if !sameRecord(anchorRecord(m), want) {
		t.Errorf("anchorChart = %+v, want %+v", anchorRecord(m), want)
	}
}

func sameRecord(got, want store.SessionChart) bool {
	return got.ID == want.ID && got.Name == want.Name &&
		got.When.Equal(want.When) && got.Place == want.Place
}

// C1: Quit writes the session, then leaves.
func TestQuitWritesSession(t *testing.T) {
	t.Run("anchored", func(t *testing.T) {
		sessionSandbox(t)
		m := anchorFixture(t)
		if !m.slots.SetComparisonSelected(false) {
			t.Fatal("the pair could not be deselected")
		}
		for _, b := range []aspect.Object{objSun, objMoon} {
			if _, ok := m.slots.ToggleObjectSelection(0, b); !ok {
				t.Fatalf("could not mark %v on the anchored slot", b)
			}
		}

		_, cmd := press(t, m, "q")
		assertQuits(t, cmd)

		want := `version: 4
charts:
  - id: fixture-one
    kind: file
    file: fixture-one.as
    name: Constructed One
    when: "2001-02-03T04:05:00Z"
    place: Test Bay
    country: TS
    lat: 12.34
    lon: 56.78
    tz: Etc/UTC
    selected: false
    bodies: Sun,Moon
floating:
  selected: false
`
		assertSessionText(t, want)
	})

	t.Run("unanchored", func(t *testing.T) {
		sessionSandbox(t)
		m := readyModelWithSlots(t, berlinP)
		if _, ok := m.slots.ToggleObjectSelection(0, objMars); !ok {
			t.Fatal("could not mark mars on the floating slot")
		}

		_, cmd := press(t, m, "q")
		assertQuits(t, cmd)

		assertSessionText(t, "version: 4\nfloating:\n  selected: false\n  bodies: Mars\n")
	})
}

func assertQuits(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("q did not quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("q returned %T, want a quit", cmd())
	}
}

func assertSessionText(t *testing.T, want string) {
	t.Helper()
	want = hereText(want) // a quit records the folder it was saved in
	got, err := os.ReadFile(store.SessionPath())
	if err != nil {
		t.Fatalf("read the session back: %v", err)
	}
	if string(got) != want {
		t.Errorf("session file =\n%s\nwant\n%s", got, want)
	}
}

// C2: a quit before the workspace exists has nothing to remember, and must not
// erase what the last real session remembered.
func TestQuitBeforeWorkspaceLeavesSessionUntouched(t *testing.T) {
	t.Run("no session file is created", func(t *testing.T) {
		sessionSandbox(t)
		m := newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen))
		if m.startup != startupBootstrapping {
			t.Fatalf("startup = %q, want %q", m.startup, startupBootstrapping)
		}
		_, cmd := press(t, m, "q")
		assertQuits(t, cmd)
		if _, err := os.Stat(store.SessionPath()); !os.IsNotExist(err) {
			t.Errorf("a session file appeared before there was a workspace: %v", err)
		}
	})

	t.Run("an existing session file is left alone", func(t *testing.T) {
		sessionSandbox(t)
		const previous = "version: 1\nselection:\n  floating_bodies: pluto\n"
		path := seedSession(t, previous)

		m := newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen))
		_, cmd := press(t, m, "q")
		assertQuits(t, cmd)

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != previous {
			t.Errorf("the previous session was rewritten:\n%s", got)
		}
	})
}

// C24: the write is best-effort. Nothing about quitting depends on it, because
// there is no frame left to report a failure in.
func TestQuitProceedsWhenSessionWriteFails(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_STATE_HOME", blocker)
	t.Setenv("HOME", root)

	m := anchorFixture(t)
	_, cmd := press(t, m, "q")
	assertQuits(t, cmd)
}

// C25: a modal owns the keyboard, so the key never reaches the quit at all.
func TestQuitInModalDoesNotWriteSession(t *testing.T) {
	cases := []struct {
		name string
		open func(Model) Model
	}{
		{"help", func(m Model) Model { m = openedKeybindings(m); return m }},
		{"atlas", func(m Model) Model { m.startup = startupAtlasOpen; return m }},
		{"charts", func(m Model) Model {
			m.startup = startupChartOpen
			m.picker = chartPickerState{prior: startupReady}
			return m
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessionSandbox(t)
			m := tc.open(anchorFixture(t))
			press(t, m, "q")
			if _, err := os.Stat(store.SessionPath()); !os.IsNotExist(err) {
				t.Errorf("the %s modal let a quit write the session: %v", tc.name, err)
			}
		})
	}
}

// Ctrl+C quits from the console through the same quit, so it writes.
func TestQuitFromConsoleWritesSession(t *testing.T) {
	sessionSandbox(t)
	m := readyModelWithSlots(t, berlinP)
	opened, _ := m.openConsole()
	_, cmd := press(t, opened.(Model), "ctrl+c")
	assertQuits(t, cmd)
	if _, err := os.Stat(store.SessionPath()); err != nil {
		t.Errorf("quitting from the console wrote no session: %v", err)
	}
}

// C7: a deleted, renamed or never-saved library file still restores — that is
// what the snapshot is for.
func TestRestoreFallsBackToSnapshot(t *testing.T) {
	snap := snapshotChart()
	m := startedModel(t, withSession(perimeter(), v1(&snap, true, false, nil, nil)))

	a := anchoredSlot(t, m)
	if !a.Time.Equal(snap.When) || a.Location != snap.Place {
		t.Errorf("anchored slot = %v / %+v, want the snapshot's %v / %+v", a.Time, a.Location, snap.When, snap.Place)
	}
	if !sameRecord(anchorRecord(m), snap) {
		t.Errorf("anchorChart = %+v, want the snapshot %+v", anchorRecord(m), snap)
	}
	if got, want := sessionLines(m), []string(nil); !reflect.DeepEqual(got, want) {
		t.Errorf("log = %v, want %v", got, want)
	}
}

// C9: a session that yields no anchor leaves the workspace floating-only.
// Nothing else anchors at startup.
func TestRestoreUnusableSessionLeavesFloatingOnly(t *testing.T) {
	broken := store.SessionChart{ID: "fixture-one", Name: "", When: fixtureOneWhen, Place: fixtureOneP}
	cases := []struct {
		name string
		p    ingest.StartupPerimeterReadMsg
	}{
		{"no chart: block in the session", withSession(perimeter(), store.Session{})},
		{"an unusable snapshot", withSession(perimeter(), v1(&broken, false, false, nil, nil))},
		{"an unreadable session file", func() ingest.StartupPerimeterReadMsg {
			p := perimeter()
			p.SessionErr = os.ErrInvalid
			return p
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := startedModel(t, tc.p)
			if m.slots == nil {
				t.Fatal("no workspace was built")
			}
			if a := firstAnchor(m.slots); a != nil {
				t.Errorf("anchored slot = %+v, want none", a)
			}
			if anchorRecord(m) != (store.SessionChart{}) {
				t.Errorf("anchorChart = %+v, want no record", anchorRecord(m))
			}
		})
	}
}

// C20: an absent session is the ordinary first run and says nothing at all.
func TestRestoreNoSessionSilent(t *testing.T) {
	m := startedModel(t, perimeter())
	if got := sessionLines(m); len(got) != 0 {
		t.Errorf("a first run wrote %v", got)
	}
	if a := firstAnchor(m.slots); a != nil {
		t.Errorf("anchored slot = %+v, want none on a first run", a)
	}
}

// keptNote is the specification C3's note-lane line.
var keptNote = regexp.MustCompile(`^session: unreadable, kept session\.yaml\.refused-[0-9]{8}T[0-9]{6}Z(-[0-9]+)?$`)

// C21: a session file this build must not act on is named once, ignored, kept
// beside the new session (the specification C1; it was "overwritten by
// the next quit" until 2026-10-08), and replaced by the next quit.
func TestRestoreMalformedSessionIgnored(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"unreadable", ""}, // mode 000, seeded below
		{"not parseable", "not: yaml: at all\n\t- [ unterminated\n"},
		{"no version", "chart:\n  name: Constructed One\n  tz: Europe/Berlin\n"},
		{"a later generation", "version: 5\nselection:\n  floating_bodies: mars\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "unreadable" && os.Geteuid() == 0 {
				t.Skip("running as root: mode bits do not deny the read")
			}
			sessionSandbox(t)
			seedPreferences(t)
			text := tc.text
			if tc.name == "unreadable" {
				text = "version: 1\nselection:\n"
			}
			path := seedSession(t, text)
			if tc.name == "unreadable" {
				if err := os.Chmod(path, 0o000); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
			}

			m := startedModel(t, realPerimeter(t))
			// the specification C3: one note naming the copy the launch
			// kept, written when the perimeter arrives (C4), so before the
			// restore's own refusal line.
			got := sessionLines(m)
			if len(got) != 2 || !keptNote.MatchString(got[0]) || got[1] != "session: the last session did not open, its file cannot be read" {
				t.Fatalf("log = %v, want the kept-copy note and then the refusal", got)
			}
			// C16: exactly one line, and it is an error.
			if got := errorLines(m); len(got) != 1 {
				t.Fatalf("error lines = %q, want exactly one", got)
			}
			if a := firstAnchor(m.slots); a != nil {
				t.Errorf("anchored slot = %+v, want none", a)
			}

			if tc.name == "unreadable" {
				// the specification C1: the launch moved the file aside,
				// so the unreadable one is now the kept copy.
				kept, _ := filepath.Glob(path + ".refused-*")
				if len(kept) != 1 {
					t.Fatalf("kept copies = %v, want one", kept)
				}
				if err := os.Chmod(kept[0], 0o644); err != nil {
					t.Fatal(err)
				}
			}
			ready := anchorFixture(t)
			if _, cmd := press(t, ready, "q"); cmd == nil {
				t.Fatal("q did not quit")
			}
			if _, ok, err := store.LoadSession(); !ok || err != nil {
				t.Errorf("the next quit did not replace the refused file: ok = %v, err = %v", ok, err)
			}
		})
	}
}

// seedPreferences writes the location: block the real perimeter read needs, so
// the C21 cases exercise the file on disk rather than a hand-built message.
func seedPreferences(t *testing.T) {
	t.Helper()
	path := store.PreferencesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	text := `location:
  name: Berlin
  country: DE
  lat: 52.5244
  lon: 13.4105
  tz: Europe/Berlin
`
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// realPerimeter runs the actual cold-start read against the sandboxed files.
func realPerimeter(t *testing.T) ingest.StartupPerimeterReadMsg {
	t.Helper()
	msg, ok := ingest.ReadStartupPerimeter()().(ingest.StartupPerimeterReadMsg)
	if !ok {
		t.Fatal("the perimeter read emitted something else")
	}
	return msg
}

// C22: a snapshot that cannot stand on its own does not anchor, and says so.
func TestRestoreSnapshotUnusable(t *testing.T) {
	full := snapshotChart()
	cases := []struct {
		name   string
		break_ func(store.SessionChart) store.SessionChart
	}{
		{"no name", func(c store.SessionChart) store.SessionChart { c.Name = ""; return c }},
		{"no moment", func(c store.SessionChart) store.SessionChart { c.When = time.Time{}; return c }},
		{"no place", func(c store.SessionChart) store.SessionChart { c.Place.Name = ""; return c }},
		{"no timezone", func(c store.SessionChart) store.SessionChart { c.Place.TZ = ""; return c }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := tc.break_(full)
			p := withSession(perimeter(), v1(&broken, false, false, nil, nil))
			m := startedModel(t, p)

			if got, want := sessionLines(m), []string{"session: the chart of the last session did not open"}; !reflect.DeepEqual(got, want) {
				t.Errorf("log = %v, want %v", got, want)
			}
			if a := firstAnchor(m.slots); a != nil {
				t.Errorf("anchored slot = %+v, want none", a)
			}
		})
	}
}

// C10: a relaunch always shows current transits. The floating slot is created
// at now whatever the session said.
func TestRestoreFloatingIsNow(t *testing.T) {
	snap := snapshotChart()
	m := startedModel(t, withSession(perimeter(), v1(&snap, true, false, nil, nil)))
	if got := m.slots.Floating().Time; !got.Equal(sessionFrozen) {
		t.Errorf("floating Time = %v, want now (%v)", got, sessionFrozen)
	}
}

// C11: the marks come back where they were, and a name the file carries twice
// still means selected.
func TestRestoreSelections(t *testing.T) {
	snap := snapshotChart()
	m := startedModel(t, withSession(perimeter(), v1(&snap, true, false, []string{"Sun", "Sun", "Moon"}, []string{"Mars"})))

	if m.slots.ComparisonSelected() {
		t.Error("the pair came back selected; the session said it was not")
	}
	if got, want := markedObjects(m.slots, 0), []aspect.Object{objSun, objMoon}; !reflect.DeepEqual(got, want) {
		t.Errorf("anchored selection = %v, want %v", got, want)
	}
	if !m.slots.ObjectSelected(0, objSun.Number) {
		t.Error("a name repeated in the file toggled its own selection back off")
	}
	if got, want := markedObjects(m.slots, 1), []aspect.Object{objMars}; !reflect.DeepEqual(got, want) {
		t.Errorf("floating selection = %v, want %v", got, want)
	}
}

// An anchor with no record writes no chart entry; with the entry go its marks,
// which belong to it, and the floating slot's selection is all that is left to
// write. No path anchors without naming the chart; this pins the guard in
// sessionSnapshot that keeps a nameless chart record out of the file.
func TestNamelessAnchorWritesSelectionsOnly(t *testing.T) {
	sessionSandbox(t)
	m := readyModelWithSlots(t, berlinP)
	if err := addAnchor(m.slots, fixtureOneWhen, fixtureOneP); err != nil {
		t.Fatal(err)
	}
	m.slots.SetComparisonSelected(false)
	m.slots.ToggleObjectSelection(0, objSaturn)
	if anchorRecord(m).Name != "" {
		t.Fatal("this fixture must stand in for an anchor with no record")
	}

	_, cmd := press(t, m, "q")
	assertQuits(t, cmd)
	assertSessionText(t, "version: 4\nfloating:\n  selected: false\n")
}

// C12: with nothing anchored the anchored slot's state has nowhere to go.
func TestRestoreFloatingSelectionsWithoutAnchor(t *testing.T) {
	m := startedModel(t, withSession(perimeter(), v1(nil, false, true, []string{"Sun"}, []string{"Mars", "Mars"})))

	if firstAnchor(m.slots) != nil {
		t.Fatal("an unanchored session anchored something")
	}
	if m.slots.ComparisonSelected() {
		t.Error("a comparison was selected with nothing to compare")
	}
	if got, want := markedObjects(m.slots, 0), []aspect.Object{objMars}; !reflect.DeepEqual(got, want) {
		t.Errorf("floating selection = %v, want %v", got, want)
	}
	if got := sessionLines(m); len(got) != 0 {
		t.Errorf("an unanchored restore wrote %v; there was nothing to report", got)
	}
}

// C13: a cold start that had to ask for the city restores after the pick, with
// the same outcome as one that did not.
func TestRestoreThroughAtlasPick(t *testing.T) {
	snap := snapshotChart()
	// No location candidate and no preseed: the resolver fails and the modal
	// opens with the perimeter stashed.
	p := withSession(ingest.StartupPerimeterReadMsg{}, v1(&snap, true, false, []string{"Moon"}, []string{"Mars"}))

	var m tea.Model = newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen))
	m, _ = m.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	m, _ = m.Update(p)
	if got := m.(Model).startup; got != startupAtlasOpen {
		t.Fatalf("startup = %q, want the atlas modal open", got)
	}
	if got := sessionLines(m.(Model)); len(got) != 0 {
		t.Errorf("the session was restored before there was a workspace: %v", got)
	}

	m, _ = m.Update(AtlasPickedMsg{Place: berlinP})
	got := m.(Model)

	a := anchoredSlot(t, got)
	if !a.Time.Equal(snap.When) || a.Location != snap.Place {
		t.Errorf("anchored slot = %v / %+v, want the session's", a.Time, a.Location)
	}
	if !got.slots.Floating().Time.Equal(sessionFrozen) {
		t.Errorf("floating Time = %v, want now", got.slots.Floating().Time)
	}
	if got.slots.ComparisonSelected() {
		t.Error("the pair came back selected; the session said it was not")
	}
	if sel := markedObjects(got.slots, 0); !reflect.DeepEqual(sel, []aspect.Object{objMoon}) {
		t.Errorf("anchored selection = %v, want [moon]", sel)
	}
	if sel := markedObjects(got.slots, 1); !reflect.DeepEqual(sel, []aspect.Object{objMars}) {
		t.Errorf("floating selection = %v, want [mars]", sel)
	}
	if want := []string(nil); !reflect.DeepEqual(sessionLines(got), want) {
		t.Errorf("log = %v, want %v", sessionLines(got), want)
	}
}

// C23 (rewritten, objects slice 1): a name this Astrolog does not list is no
// longer skipped. It is kept on its slot verbatim, said once with the drawn
// set's line, and the names that do resolve are applied.
func TestRestoreKeepsUnknownBodies(t *testing.T) {
	snap := snapshotChart()
	m := startedModel(t, withSession(perimeter(), v1(&snap, true, true, []string{"sedna", "Sun", "eris"}, []string{"Mars", "eris"})))

	if sel := markedObjects(m.slots, 0); !reflect.DeepEqual(sel, []aspect.Object{objSun}) {
		t.Errorf("anchored selection = %v, want [sun]", sel)
	}
	if sel := markedObjects(m.slots, 1); !reflect.DeepEqual(sel, []aspect.Object{objMars}) {
		t.Errorf("floating selection = %v, want [mars]", sel)
	}
	if got := m.slots.KeptMarks(0); !reflect.DeepEqual(got, []string{"sedna", "eris"}) {
		t.Errorf("anchored kept names = %v, want [sedna eris]", got)
	}
	if got := m.slots.KeptMarks(1); !reflect.DeepEqual(got, []string{"eris"}) {
		t.Errorf("floating kept names = %v, want [eris]", got)
	}
	for _, name := range []string{"sedna", "eris"} {
		line := name + " is not known to this Astrolog, the session keeps its name"
		if n := countLines(m, line); n != 1 {
			t.Errorf("the Log carries %q %d times, want once:\n%s", line, n, strings.Join(logTexts(m), "\n"))
		}
	}
	if got := sessionLines(m); len(got) != 0 {
		t.Errorf("session lines = %v, want none (the skipped-bodies line is gone)", got)
	}
	// The next quit writes them back: the resolved names, then the kept ones.
	snapshot := m.sessionSnapshot()
	if got := snapshot.Slots[0].Bodies; !reflect.DeepEqual(got, []string{"Sun", "sedna", "eris"}) {
		t.Errorf("the anchored line would be %v, want [Sun sedna eris]", got)
	}
	if got := snapshot.FloatingBodies; !reflect.DeepEqual(got, []string{"Mars", "eris"}) {
		t.Errorf("the floating line would be %v, want [Mars eris]", got)
	}
}

// C15: one line per outcome, in one order, and no other session: line. A
// restore that worked is not an outcome worth a line since design piece 5
// (the specification): the header shows what came back.
func TestRestoreHintLines(t *testing.T) {
	usable := snapshotChart()
	broken := store.SessionChart{ID: "fixture-one", When: fixtureOneWhen, Place: fixtureOneP}

	cases := []struct {
		name string
		p    ingest.StartupPerimeterReadMsg
		want []string
	}{
		{
			name: "the snapshot alone",
			p:    withSession(perimeter(), v1(&usable, true, false, nil, nil)),
			want: []string(nil),
		},
		{
			name: "a file that must not be acted on",
			p: func() ingest.StartupPerimeterReadMsg {
				p := perimeter()
				p.SessionErr = os.ErrInvalid
				return p
			}(),
			want: []string{"session: the last session did not open, its file cannot be read"},
		},
		{
			name: "a snapshot that cannot stand on its own",
			p:    withSession(perimeter(), v1(&broken, false, false, nil, nil)),
			want: []string{"session: the chart of the last session did not open"},
		},
		{
			// The name is kept and said with the drawn set's line, which is no
			// session: line (TestRestoreKeepsUnknownBodies).
			name: "unknown names beside a restore",
			p:    withSession(perimeter(), v1(&usable, true, false, []string{"sedna"}, nil)),
			want: []string(nil),
		},
		{
			name: "an ordinary unanchored quit",
			p:    withSession(perimeter(), v1(nil, false, false, nil, []string{"Mars"})),
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sessionLines(startedModel(t, tc.p)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("log = %v, want %v", got, tc.want)
			}
		})
	}
}

// The round trip end to end: what one launch wrote, the next launch reads.
func TestSessionSurvivesAQuitAndARelaunch(t *testing.T) {
	sessionSandbox(t)
	// The relaunch reads the real files, and with no stored location startup
	// falls back to the host's timezone city — Berlin passed, anything else
	// opened the city picker and built no workspace.
	seedPreferences(t)
	m := anchorFixture(t)
	m.slots.SetComparisonSelected(false)
	m.slots.ToggleObjectSelection(0, objSaturn)
	m.slots.ToggleObjectSelection(1, objMars)
	if _, cmd := press(t, m, "q"); cmd == nil {
		t.Fatal("q did not quit")
	}

	relaunched := startedModel(t, realPerimeter(t))
	a := anchoredSlot(t, relaunched)
	if !a.Time.Equal(fixtureOneWhen) || a.Location != fixtureOneP {
		t.Errorf("anchored slot = %v / %+v, want the chart that was anchored", a.Time, a.Location)
	}
	if relaunched.slots.ComparisonSelected() {
		t.Error("the pair came back selected")
	}
	if sel := markedObjects(relaunched.slots, 0); !reflect.DeepEqual(sel, []aspect.Object{objSaturn}) {
		t.Errorf("anchored selection = %v, want [saturn]", sel)
	}
	if sel := markedObjects(relaunched.slots, 1); !reflect.DeepEqual(sel, []aspect.Object{objMars}) {
		t.Errorf("floating selection = %v, want [mars]", sel)
	}
	if want := []string(nil); !reflect.DeepEqual(sessionLines(relaunched), want) {
		t.Errorf("log = %v, want %v", sessionLines(relaunched), want)
	}
}

// A session written while YAML charts existed (`kind: yaml`, retired
// 2026-10-02) still brings the client's chart back: read from disk, through
// the real perimeter, anchored from its snapshot, owing no re-read of a file
// nothing reads -- and the next quit writes it back with no locator.
func TestAYAMLEraSessionRestoresFromItsSnapshot(t *testing.T) {
	sessionSandbox(t)
	seedPreferences(t)
	path := seedSession(t, hereText(`version: 1
chart:
  id: fixture-one
  kind: yaml
  file: fixture-one.yaml
  name: Constructed One
  when: "2001-02-03T04:05:00Z"
  place: Test Bay
  country: TS
  lat: 12.34
  lon: 56.78
  tz: Etc/UTC
`))
	p := realPerimeter(t)
	m := startedModel(t, p)
	if got := anchoredSlot(t, m).Time; !got.Equal(fixtureOneWhen) {
		t.Fatalf("anchored at %v, want the snapshot's %v", got, fixtureOneWhen)
	}
	if anchorRecord(m).Name != "Constructed One" || anchorRecord(m).Source.Kind != store.ChartKindNone {
		t.Errorf("anchorChart = %+v, want the snapshot with no readable source", anchorRecord(m))
	}
	// A restore that worked says nothing (the specification § Log
	// scope); the anchor above is what came back.
	if got := sessionLines(m); len(got) != 0 {
		t.Errorf("log = %v, want nothing", got)
	}
	if m.sessionRecheckCmd(p, true) != nil {
		t.Error("a YAML-era record owes a re-read of a file nothing reads")
	}
	if _, cmd := press(t, m, "q"); cmd == nil {
		t.Fatal("q did not quit")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if out := string(b); strings.Contains(out, "kind:") || strings.Contains(out, ".yaml\n") || !strings.Contains(out, "name: Constructed One") {
		t.Errorf("the session written at quit:\n%s\nwant the snapshot kept and no locator", out)
	}
}

// Restoring a session restores exactly the saved selection, now that opening a
// chart selects nothing: anchoring the saved charts leaves everything unselected
// and applySessionSelections sets only what the file marked.
func TestRestoreSetsExactlyTheSavedSelection(t *testing.T) {
	recA := store.SessionChart{ID: "a", Name: "A", Source: store.ChartSource{Kind: store.ChartKindFile, File: "a.as"}, When: fixtureOneWhen, Place: fixtureOneP}
	recB := store.SessionChart{ID: "b", Name: "B", Source: store.ChartSource{Kind: store.ChartKindFile, File: "b.as"}, When: fixtureOneWhen, Place: fixtureOneP}
	for _, tc := range []struct {
		name           string
		a, b, floating bool
	}{
		{"nothing selected", false, false, false},
		{"second chart only", false, true, false},
		{"first chart and floating", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := readyModelWithSlots(t, berlinP)
			for _, rec := range []store.SessionChart{recA, recB} {
				var ok bool
				if m, ok = m.anchorFromSession(rec); !ok {
					t.Fatal("anchorFromSession refused a usable chart")
				}
			}
			if m.slots.SlotSelected(0) || m.slots.SlotSelected(1) || m.slots.SlotSelected(2) {
				t.Fatal("precondition: anchoring from the session selected something")
			}
			m = m.applySessionSelections(ingest.StartupPerimeterReadMsg{HasSession: true, Session: store.Session{
				Version: 2, FloatingSelected: tc.floating,
				Slots: []store.SessionSlot{{Chart: recA, Usable: true, Selected: tc.a}, {Chart: recB, Usable: true, Selected: tc.b}},
			}})
			got := []bool{m.slots.SlotSelected(0), m.slots.SlotSelected(1), m.slots.SlotSelected(2)}
			want := []bool{tc.a, tc.b, tc.floating}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("selected (a, b, floating) = %v, want %v", got, want)
			}
		})
	}
}
