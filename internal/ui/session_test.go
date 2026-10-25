package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
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
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("SEXTILE_CHARTS_DIR", filepath.Join(root, "charts"))
	t.Setenv("SEXTILE_DEV_CHARTS_DIR", "")
	t.Setenv("HOME", root)
	return filepath.Join(root, "data", "sextile", "session")
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

// withSession attaches a session that carries the snapshot and nothing else.
func withSession(p ingest.StartupPerimeterReadMsg, s store.Session) ingest.StartupPerimeterReadMsg {
	p.Session = s
	p.HasSession = true
	return p
}

// startedModel drives a cold start to the point where the workspace exists:
// the atlas index lands, then the perimeter read.
func startedModel(t *testing.T, p ingest.StartupPerimeterReadMsg) Model {
	t.Helper()
	var m tea.Model = NewModelWithClock(Config{Accessible: true}, FrozenClock(sessionFrozen))
	m, _ = m.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	m, _ = m.Update(p)
	return m.(Model)
}

// sessionLines is every Log line the restore wrote, in order. Startup writes
// other lines (the active city, resolver warnings); C15 governs these.
func sessionLines(m Model) []string {
	var out []string
	for _, l := range m.logLines {
		if strings.HasPrefix(l.text, "session:") || (l.lane == laneError && strings.HasPrefix(l.text, "Couldn't restore the last session")) {
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
		for _, b := range []aspect.Body{aspect.Sun, aspect.Moon} {
			if _, ok := m.slots.ToggleBodySelection(0, b); !ok {
				t.Fatalf("could not mark %v on the anchored slot", b)
			}
		}

		_, cmd := press(t, m, "q")
		assertQuits(t, cmd)

		want := `version: 3
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
    bodies: sun,moon
floating:
  selected: false
`
		assertSessionText(t, want)
	})

	t.Run("unanchored", func(t *testing.T) {
		sessionSandbox(t)
		m := readyModelWithSlots(t, berlinP)
		if _, ok := m.slots.ToggleBodySelection(0, aspect.Mars); !ok {
			t.Fatal("could not mark mars on the floating slot")
		}

		_, cmd := press(t, m, "q")
		assertQuits(t, cmd)

		assertSessionText(t, "version: 3\nfloating:\n  selected: false\n  bodies: mars\n")
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
		m := NewModelWithClock(Config{Accessible: true}, FrozenClock(sessionFrozen))
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

		m := NewModelWithClock(Config{Accessible: true}, FrozenClock(sessionFrozen))
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
	t.Setenv("XDG_DATA_HOME", blocker)
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
		{"help", func(m Model) Model { m.helpOpen = true; return m }},
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
var keptNote = regexp.MustCompile(`^session: refused, kept session\.yaml\.refused-[0-9]{8}T[0-9]{6}Z(-[0-9]+)?$`)

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
		{"a later generation", "version: 4\nselection:\n  floating_bodies: mars\n"},
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
			if len(got) != 2 || !keptNote.MatchString(got[0]) || got[1] != "Couldn't restore the last session. Its file cannot be used." {
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

			if got, want := sessionLines(m), []string{"session: natal not restored"}; !reflect.DeepEqual(got, want) {
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
	m := startedModel(t, withSession(perimeter(), v1(&snap, true, false, []aspect.Body{aspect.Sun, aspect.Sun, aspect.Moon}, []aspect.Body{aspect.Mars})))

	if m.slots.ComparisonSelected() {
		t.Error("the pair came back selected; the session said it was not")
	}
	if got, want := m.slots.SelectedBodies(0), []aspect.Body{aspect.Sun, aspect.Moon}; !reflect.DeepEqual(got, want) {
		t.Errorf("anchored selection = %v, want %v", got, want)
	}
	if !m.slots.BodySelected(0, aspect.Sun) {
		t.Error("a name repeated in the file toggled its own selection back off")
	}
	if got, want := m.slots.SelectedBodies(1), []aspect.Body{aspect.Mars}; !reflect.DeepEqual(got, want) {
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
	m.slots.ToggleBodySelection(0, aspect.Saturn)
	if anchorRecord(m).Name != "" {
		t.Fatal("this fixture must stand in for an anchor with no record")
	}

	_, cmd := press(t, m, "q")
	assertQuits(t, cmd)
	assertSessionText(t, "version: 3\nfloating:\n  selected: false\n")
}

// C12: with nothing anchored the anchored slot's state has nowhere to go.
func TestRestoreFloatingSelectionsWithoutAnchor(t *testing.T) {
	m := startedModel(t, withSession(perimeter(), v1(nil, false, true, []aspect.Body{aspect.Sun}, []aspect.Body{aspect.Mars, aspect.Mars})))

	if firstAnchor(m.slots) != nil {
		t.Fatal("an unanchored session anchored something")
	}
	if m.slots.ComparisonSelected() {
		t.Error("a comparison was selected with nothing to compare")
	}
	if got, want := m.slots.SelectedBodies(0), []aspect.Body{aspect.Mars}; !reflect.DeepEqual(got, want) {
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
	p := withSession(ingest.StartupPerimeterReadMsg{}, v1(&snap, true, false, []aspect.Body{aspect.Moon}, []aspect.Body{aspect.Mars}))

	var m tea.Model = NewModelWithClock(Config{Accessible: true}, FrozenClock(sessionFrozen))
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
	if sel := got.slots.SelectedBodies(0); !reflect.DeepEqual(sel, []aspect.Body{aspect.Moon}) {
		t.Errorf("anchored selection = %v, want [moon]", sel)
	}
	if sel := got.slots.SelectedBodies(1); !reflect.DeepEqual(sel, []aspect.Body{aspect.Mars}) {
		t.Errorf("floating selection = %v, want [mars]", sel)
	}
	if want := []string(nil); !reflect.DeepEqual(sessionLines(got), want) {
		t.Errorf("log = %v, want %v", sessionLines(got), want)
	}
}

// C23: a body name this build does not know is skipped, the rest are applied,
// and the skip is named.
func TestRestoreSkipsUnknownBodies(t *testing.T) {
	snap := snapshotChart()
	m := startedModel(t, withSession(perimeter(), v1(&snap, true, true, []aspect.Body{aspect.Sun}, []aspect.Body{aspect.Mars}, "sedna", "eris")))

	if sel := m.slots.SelectedBodies(0); !reflect.DeepEqual(sel, []aspect.Body{aspect.Sun}) {
		t.Errorf("anchored selection = %v, want [sun]", sel)
	}
	if sel := m.slots.SelectedBodies(1); !reflect.DeepEqual(sel, []aspect.Body{aspect.Mars}) {
		t.Errorf("floating selection = %v, want [mars]", sel)
	}
	want := []string{
		"session: skipped unknown bodies: sedna, eris",
	}
	if got := sessionLines(m); !reflect.DeepEqual(got, want) {
		t.Errorf("log = %v, want %v", got, want)
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
			want: []string{"Couldn't restore the last session. Its file cannot be used."},
		},
		{
			name: "a snapshot that cannot stand on its own",
			p:    withSession(perimeter(), v1(&broken, false, false, nil, nil)),
			want: []string{"session: natal not restored"},
		},
		{
			name: "unknown names beside a restore",
			p:    withSession(perimeter(), v1(&usable, true, false, nil, nil, "sedna")),
			want: []string{
				"session: skipped unknown bodies: sedna",
			},
		},
		{
			name: "an ordinary unanchored quit",
			p:    withSession(perimeter(), v1(nil, false, false, nil, []aspect.Body{aspect.Mars})),
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
	m.slots.ToggleBodySelection(0, aspect.Saturn)
	m.slots.ToggleBodySelection(1, aspect.Mars)
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
	if sel := relaunched.slots.SelectedBodies(0); !reflect.DeepEqual(sel, []aspect.Body{aspect.Saturn}) {
		t.Errorf("anchored selection = %v, want [saturn]", sel)
	}
	if sel := relaunched.slots.SelectedBodies(1); !reflect.DeepEqual(sel, []aspect.Body{aspect.Mars}) {
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
	path := seedSession(t, `version: 1
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
`)
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
