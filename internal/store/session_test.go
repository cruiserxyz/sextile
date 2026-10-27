package store

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// All chart values in this file are constructed synthetic test inputs.

// sessionSandbox points SessionDir() at a temp directory. Every test here
// writes a real file, so the isolation is not optional: without it the suite
// would overwrite the operator's own session on any host that has one.
func sessionSandbox(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	t.Setenv("HOME", filepath.Join(dir, "home"))
	return filepath.Join(dir, "sextile", "session")
}

// oneSlot is the session shape every test below used to build by hand: one
// anchored chart, with the selection and bodies it carried.
func oneSlot(c SessionChart, selected bool, bodies ...aspect.Body) []SessionSlot {
	return []SessionSlot{{Chart: c, Usable: true, Selected: selected, Bodies: bodies}}
}

func fixtureChart() SessionChart {
	return SessionChart{
		ID:   "fixture-one",
		Name: "Constructed One",
		When: time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC),
		Place: location.PlaceInfo{
			Name: "Test Bay", Country: "TS", Lat: 12.34, Lon: 56.78, TZ: "Etc/UTC",
		},
	}
}

// The session and its lock are state, kept in $XDG_STATE_HOME/sextile/session
// (fallback ~/.local/state/sextile/session) — never under XDG_DATA_HOME, where
// 0.1.1's launcher pointed into the charts folder.
func TestSessionDirIsXDGStateHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/data")
	t.Setenv("XDG_STATE_HOME", "/xdg-state")
	if got, want := SessionPath(), "/xdg-state/sextile/session/session.yaml"; got != want {
		t.Errorf("SessionPath() = %q, want %q", got, want)
	}
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/u")
	if got, want := SessionDir(), "/home/u/.local/state/sextile/session"; got != want {
		t.Errorf("SessionDir() without XDG_STATE_HOME = %q, want %q", got, want)
	}
}

// A fresh session folder is private: 0700, whether the lock or the first save
// makes it. One left 0755 by an earlier build is narrowed.
func TestFreshSessionDirIsPrivate(t *testing.T) {
	for _, maker := range []string{"lock", "save", "existing 0755"} {
		t.Run(maker, func(t *testing.T) {
			dir := sessionSandbox(t)
			switch maker {
			case "lock":
				mustAcquire(t)
			case "save":
				if err := SaveSession(Session{Slots: oneSlot(fixtureChart(), false)}); err != nil {
					t.Fatal(err)
				}
			case "existing 0755":
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				mustAcquire(t)
			}
			for _, d := range []string{dir, filepath.Dir(dir)} {
				fi, err := os.Stat(d)
				if err != nil {
					t.Fatal(err)
				}
				if d == dir || maker != "existing 0755" {
					if got := fi.Mode().Perm(); got != 0o700 {
						t.Errorf("%s has mode %v, want 0700", d, got)
					}
				}
			}
		})
	}
}

// The file's exact text, so the shape an operator opens in an editor is under
// test and not merely its round-trip.
func TestSessionWrittenText(t *testing.T) {
	dir := sessionSandbox(t)
	c := fixtureChart()
	err := SaveSession(Session{
		Slots:            oneSlot(c, true, aspect.Sun, aspect.Moon),
		FloatingSelected: true,
		FloatingBodies:   []aspect.Body{aspect.Mars},
	})
	if err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "session.yaml"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	want := `version: 3
charts:
  - id: fixture-one
    name: Constructed One
    when: "2001-02-03T04:05:00Z"
    place: Test Bay
    country: TS
    lat: 12.34
    lon: 56.78
    tz: Etc/UTC
    selected: true
    bodies: sun,moon
floating:
  selected: true
  bodies: mars
`
	if string(got) != want {
		t.Errorf("session file =\n%s\nwant\n%s", got, want)
	}
}

// C3: tmpfile + rename. A write that fails leaves the previous session intact
// and leaves no half-written file behind for the next launch to trip over.
func TestSessionSaveAtomic(t *testing.T) {
	t.Run("rename failure keeps the previous session and leaves no tmp", func(t *testing.T) {
		dir := sessionSandbox(t)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// A non-empty directory standing where the file goes: CreateTemp
		// succeeds, the rename onto it cannot.
		blocked := filepath.Join(dir, "session.yaml")
		if err := os.MkdirAll(filepath.Join(blocked, "occupied"), 0o755); err != nil {
			t.Fatal(err)
		}
		c := fixtureChart()
		if err := SaveSession(Session{Slots: oneSlot(c, false)}); err == nil {
			t.Fatal("SaveSession onto a directory reported success")
		}
		if _, err := os.Stat(filepath.Join(blocked, "occupied")); err != nil {
			t.Errorf("the blocking directory did not survive the failed write: %v", err)
		}
		assertNoTmpFiles(t, dir)
	})

	t.Run("an unwritable directory keeps the previous bytes", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("running as root: mode bits do not deny the write")
		}
		dir := sessionSandbox(t)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "session.yaml")
		const previous = "version: 1\nselection:\n  floating_bodies: mars\n"
		if err := os.WriteFile(path, []byte(previous), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

		c := fixtureChart()
		if err := SaveSession(Session{Slots: oneSlot(c, false)}); err == nil {
			t.Fatal("SaveSession into a read-only directory reported success")
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if string(got) != previous {
			t.Errorf("previous session was disturbed:\n%s", got)
		}
	})

	t.Run("a successful write replaces the bytes and leaves no tmp", func(t *testing.T) {
		dir := sessionSandbox(t)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "session.yaml")
		if err := os.WriteFile(path, []byte("version: 1\nselection:\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := SaveSession(Session{FloatingBodies: []aspect.Body{aspect.Mars}}); err != nil {
			t.Fatalf("SaveSession: %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if want := "version: 3\nfloating:\n  selected: false\n  bodies: mars\n"; string(got) != want {
			t.Errorf("session file = %q, want %q", got, want)
		}
		assertNoTmpFiles(t, dir)
	})

	t.Run("a missing session directory is created", func(t *testing.T) {
		dir := sessionSandbox(t)
		if err := SaveSession(Session{}); err != nil {
			t.Fatalf("SaveSession: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, "session.yaml")); err != nil {
			t.Errorf("session file not written into a fresh directory: %v", err)
		}
	})
}

func assertNoTmpFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("%s left behind in the session directory", e.Name())
		}
	}
}

// C3.1: every value SaveSession accepts comes back equal, with the
// read-side diagnostics (Usable, UnknownBodies) rebuilt rather than
// carried, and moments compared in UTC at the second precision the file has.
func TestSessionRoundTrip(t *testing.T) {
	awkward := SessionChart{
		ID:   "awkward-id",
		Name: `A: name with "quotes" and a # hash`,
		When: time.Date(2001, 12, 31, 23, 59, 59, 0, time.FixedZone("odd", 5400)),
		Place: location.PlaceInfo{
			Name: "  padded  ", Country: "", Lat: -33.8688, Lon: 151.2093, TZ: "Australia/Sydney",
		},
	}
	plain := fixtureChart()
	empty := SessionChart{ID: "n-id", Name: "n", When: time.Unix(0, 0).UTC(), Place: location.PlaceInfo{Name: "p", TZ: "UTC"}}

	cases := []struct {
		name string
		in   Session
	}{
		{"anchored with an id", Session{
			Slots:            oneSlot(plain, true, aspect.Sun, aspect.Moon),
			FloatingSelected: true,
			FloatingBodies:   []aspect.Body{aspect.Mars},
		}},
		{"anchored with an empty id and awkward strings", Session{
			Slots: oneSlot(awkward, false, aspect.ImumCoeli),
		}},
		{"anchored with no selections at all", Session{Slots: oneSlot(empty, false)}},
		{"unanchored with a floating selection", Session{
			FloatingBodies: []aspect.Body{aspect.Pluto, aspect.Chiron},
		}},
		{"unanchored with nothing selected", Session{}},
		{"a floating selection with nothing anchored", Session{
			FloatingSelected: true,
			FloatingBodies:   []aspect.Body{aspect.Mars},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessionSandbox(t)
			if err := SaveSession(tc.in); err != nil {
				t.Fatalf("SaveSession: %v", err)
			}
			got, ok, err := LoadSession()
			if err != nil || !ok {
				t.Fatalf("LoadSession() = _, %v, %v; want a session", ok, err)
			}
			assertSessionEqual(t, got, tc.in)
			if len(got.UnknownBodies) != 0 {
				t.Errorf("a written session read back with unknown bodies %v", got.UnknownBodies)
			}
			for i, slot := range got.Slots {
				if !slot.Usable {
					t.Errorf("slot %d read back unusable; a written chart is always usable", i)
				}
			}
		})
	}
}

// The floating slot's selection and marks are the workspace's own and do not
// depend on any chart being named: a session with no anchored slot still has
// marks worth keeping, so the writer keeps them rather than normalizing them
// away with a chart that is not there.
func TestSessionSelectionSurvivesWithoutAChartBlock(t *testing.T) {
	sessionSandbox(t)
	err := SaveSession(Session{
		FloatingSelected: true,
		FloatingBodies:   []aspect.Body{aspect.Mars},
	})
	if err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	got, ok, err := LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession() = _, %v, %v", ok, err)
	}
	if len(got.Slots) != 0 {
		t.Errorf("Slots = %+v, want none: none was written", got.Slots)
	}
	if !got.FloatingSelected {
		t.Error("the floating selection did not survive a quit with no chart")
	}
	if !reflect.DeepEqual(got.FloatingBodies, []aspect.Body{aspect.Mars}) {
		t.Errorf("floating bodies = %v, want [mars]", got.FloatingBodies)
	}
}

// A value carrying a newline must not become two physical lines. These readers
// are line-based, so an unquoted one would truncate a client's chart label at
// the first line — silently, in the file that is supposed to remember it.
func TestSessionRoundTripsControlCharactersInStrings(t *testing.T) {
	sessionSandbox(t)
	c := fixtureChart()
	c.Name = "Alice\nBob"
	c.Place.Name = "Test Bay\tSouth"
	if err := SaveSession(Session{Slots: oneSlot(c, false)}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	got, ok, err := LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession() = _, %v, %v", ok, err)
	}
	if len(got.Slots) != 1 {
		t.Fatal("the chart did not come back")
	}
	if got.Slots[0].Chart.Name != c.Name {
		t.Errorf("Name = %q, want %q", got.Slots[0].Chart.Name, c.Name)
	}
	if got.Slots[0].Chart.Place.Name != c.Place.Name {
		t.Errorf("Place.Name = %q, want %q", got.Slots[0].Chart.Place.Name, c.Place.Name)
	}
}

func assertSessionEqual(t *testing.T, got, want Session) {
	t.Helper()
	if len(got.Slots) != len(want.Slots) {
		t.Fatalf("%d slots came back, want %d", len(got.Slots), len(want.Slots))
	}
	for i := range want.Slots {
		w, g := want.Slots[i], got.Slots[i]
		if !g.Chart.When.Equal(w.Chart.When.UTC().Truncate(time.Second)) {
			t.Errorf("slot %d When = %v, want %v", i, g.Chart.When, w.Chart.When)
		}
		wc, gc := w.Chart, g.Chart
		wc.When, gc.When = time.Time{}, time.Time{}
		if !reflect.DeepEqual(gc, wc) {
			t.Errorf("slot %d Chart =\n  %+v\nwant\n  %+v", i, gc, wc)
		}
		if g.Selected != w.Selected {
			t.Errorf("slot %d Selected = %v, want %v", i, g.Selected, w.Selected)
		}
		if !sameBodies(g.Bodies, w.Bodies) {
			t.Errorf("slot %d Bodies = %v, want %v", i, g.Bodies, w.Bodies)
		}
	}
	if got.FloatingSelected != want.FloatingSelected {
		t.Errorf("FloatingSelected = %v, want %v", got.FloatingSelected, want.FloatingSelected)
	}
	if !sameBodies(got.FloatingBodies, want.FloatingBodies) {
		t.Errorf("FloatingBodies = %v, want %v", got.FloatingBodies, want.FloatingBodies)
	}
}

func sameBodies(a, b []aspect.Body) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// C20: no session file is the ordinary first run, not a failure.
func TestSessionLoadMissingFile(t *testing.T) {
	sessionSandbox(t)
	got, ok, err := LoadSession()
	if ok || err != nil {
		t.Fatalf("LoadSession() = _, %v, %v; want (zero, false, nil)", ok, err)
	}
	if !reflect.DeepEqual(got, Session{}) {
		t.Errorf("LoadSession() returned %+v, want the zero session", got)
	}
}

// C21: a file this build must not act on is refused whole, so startup can
// treat it exactly as an absent one.
//
// The version line is the WHOLE of what can be refused for being unparsable:
// below it C26 requires unknown keys and unknown blocks to be read past, so
// text this build cannot make sense of is ignored rather than rejected. Both
// halves of that boundary are pinned here, because a fixture that happens to
// omit `version:` would otherwise look like it tested malformed YAML while
// only testing the missing-version path.
func TestSessionLoadMalformed(t *testing.T) {
	assertRefused(t, "not: yaml: at all\n\t- [ unterminated\n")
	assertRefused(t, "version:\nselection:\n")
	assertRefused(t, "version: 1.5\nselection:\n")
}

func TestSessionLoadReadsPastTextItCannotParse(t *testing.T) {
	dir := sessionSandbox(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	text := "version: 1\nselection:\n  floating_bodies: mars\nbroken: [unterminated\n"
	if err := os.WriteFile(filepath.Join(dir, "session.yaml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession() = _, %v, %v; C26 requires reading past an unknown block", ok, err)
	}
	if !sameBodies(got.FloatingBodies, []aspect.Body{aspect.Mars}) {
		t.Errorf("FloatingBodies = %v, want [mars]", got.FloatingBodies)
	}
}

func TestSessionLoadMissingVersion(t *testing.T) {
	assertRefused(t, "chart:\n  name: Constructed One\n  tz: Europe/Berlin\n")
}

func TestSessionLoadUnknownVersion(t *testing.T) {
	assertRefused(t, "version: 4\nselection:\n  floating_bodies: mars\n")
	assertRefused(t, "version: nonsense\nselection:\n")
}

func assertRefused(t *testing.T, text string) {
	t.Helper()
	dir := sessionSandbox(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session.yaml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadSession()
	if ok {
		t.Errorf("LoadSession() accepted %q", text)
	}
	if err == nil {
		t.Errorf("LoadSession() refused %q without saying why", text)
	}
	if !reflect.DeepEqual(got, Session{}) {
		t.Errorf("a refused file yielded %+v, want the zero session", got)
	}
}

// C26: unknown keys and unknown blocks are read past, and the next write does
// not carry them — the file is machine-managed, and nobody else authors it.
func TestSessionUnknownKeysIgnored(t *testing.T) {
	dir := sessionSandbox(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "session.yaml")
	text := `# a comment nobody keeps
version: 1
chart:
  id: fixture-one
  name: Constructed One
  when: "2001-02-03T04:05:00Z"
  place: Test Bay
  country: DE
  lat: 48.4
  lon: 10
  tz: Europe/Berlin
  lens: hellenistic
selection:
  comparison: true
  floating_bodies: mars
  focused_panel: bodies
whatever:
  who: knows
`
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession() = _, %v, %v; want the session", ok, err)
	}
	if len(got.Slots) != 1 || got.Slots[0].Chart.Name != "Constructed One" || !got.Slots[0].Usable {
		t.Fatalf("the known keys did not survive the unknown ones: %+v", got.Slots)
	}
	if !got.Slots[0].Selected || !got.FloatingSelected || !sameBodies(got.FloatingBodies, []aspect.Body{aspect.Mars}) {
		t.Errorf("selection = %v / %v / %v, want the pair selected and [mars]", got.Slots[0].Selected, got.FloatingSelected, got.FloatingBodies)
	}

	if err := SaveSession(got); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"lens:", "focused_panel:", "whatever:", "a comment nobody keeps"} {
		if strings.Contains(string(after), gone) {
			t.Errorf("%q survived the rewrite; the session file preserves nothing it did not author", gone)
		}
	}
}

// C22's parse half: which chart: blocks can stand on their own. The decision
// itself is startup's, but the facts it decides on are read here.
func TestSessionChartUsable(t *testing.T) {
	const complete = `version: 1
chart:
  id: fixture-one
  name: Constructed One
  when: "2001-02-03T04:05:00Z"
  place: Test Bay
  country: DE
  lat: 48.4
  lon: 10
  tz: Europe/Berlin
`
	cases := []struct {
		name   string
		text   string
		usable bool
	}{
		{"complete", complete, true},
		{"no name", strings.Replace(complete, "  name: Constructed One\n", "", 1), false},
		{"no when", strings.Replace(complete, "  when: \"2001-02-03T04:05:00Z\"\n", "", 1), false},
		{"no place", strings.Replace(complete, "  place: Test Bay\n", "", 1), false},
		{"no lat", strings.Replace(complete, "  lat: 48.4\n", "", 1), false},
		{"no lon", strings.Replace(complete, "  lon: 10\n", "", 1), false},
		{"no tz", strings.Replace(complete, "  tz: Europe/Berlin\n", "", 1), false},
		{"when is not a moment", strings.Replace(complete, `"2001-02-03T04:05:00Z"`, "yesterday", 1), false},
		{"lat is not a number", strings.Replace(complete, "lat: 48.4", "lat: north", 1), false},
		{"lon is not finite", strings.Replace(complete, "lon: 10", "lon: NaN", 1), false},
		{"tz is not a zone the host has", strings.Replace(complete, "Europe/Berlin", "Mars/Olympus_Mons", 1), false},
		{"no country is still usable", strings.Replace(complete, "  country: DE\n", "", 1), true},
		{"no id is still usable", strings.Replace(complete, "  id: fixture-one\n", "", 1), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := sessionSandbox(t)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "session.yaml"), []byte(tc.text), 0o644); err != nil {
				t.Fatal(err)
			}
			got, ok, err := LoadSession()
			if err != nil || !ok {
				t.Fatalf("LoadSession() = _, %v, %v", ok, err)
			}
			if len(got.Slots) != 1 {
				t.Fatal("a chart: block was present but did not come back")
			}
			if got.Slots[0].Usable != tc.usable {
				t.Errorf("Usable = %v, want %v (chart %+v)", got.Slots[0].Usable, tc.usable, got.Slots[0].Chart)
			}
		})
	}
}

// SessionChart.Usable is the single bar a chart must clear to anchor, whether
// it arrived from the session file or from the live library. A row the library
// happily lists but that cannot anchor must fail it.
func TestSessionChartUsableIsTheOneBar(t *testing.T) {
	full := SessionChart{
		Name: "Constructed One", When: time.Date(2001, 3, 14, 11, 30, 0, 0, time.UTC),
		Place: location.PlaceInfo{Name: "Test Bay", Country: "DE", Lat: 48.4, Lon: 10, TZ: "Europe/Berlin"},
	}
	if !full.Usable() {
		t.Fatalf("a complete record was refused: %+v", full)
	}
	if reason := full.UnusableReason(); reason != "" {
		t.Fatalf("a complete record was given a reason to refuse it: %q", reason)
	}
	cases := []struct {
		name   string
		break_ func(SessionChart) SessionChart
		reason string
	}{
		{"no label", func(c SessionChart) SessionChart { c.Name = ""; return c }, "no name"},
		{"no moment", func(c SessionChart) SessionChart { c.When = time.Time{}; return c }, "no birth time"},
		{"no place name", func(c SessionChart) SessionChart { c.Place.Name = ""; return c }, "no birth place"},
		{"a latitude that is not a number", func(c SessionChart) SessionChart {
			c.Place.Lat = math.NaN()
			return c
		}, "no numeric coordinates"},
		{"an infinite longitude", func(c SessionChart) SessionChart {
			c.Place.Lon = math.Inf(1)
			return c
		}, "no numeric coordinates"},
		{"no timezone", func(c SessionChart) SessionChart { c.Place.TZ = ""; return c }, "an unknown timezone"},
		{"a timezone the host does not have", func(c SessionChart) SessionChart {
			c.Place.TZ = "Mars/Olympus_Mons"
			return c
		}, "an unknown timezone"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			broken := tc.break_(full)
			if broken.Usable() {
				t.Errorf("Usable() accepted %+v", broken)
			}
			// The refusal has to NAME what it refused over. A pick that says
			// only "no" leaves the astrologer opening files at random, and a
			// reason that names the wrong key sends them to edit a line that
			// was never the problem.
			if got := broken.UnusableReason(); got != tc.reason {
				t.Errorf("UnusableReason() = %q, want %q (record %+v)", got, tc.reason, broken)
			}
		})
	}
}

// The names no body answers to are named, once each, anchored before floating
// however the file ordered its keys.
func TestSessionUnknownBodyNames(t *testing.T) {
	dir := sessionSandbox(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	text := "version: 1\nchart:\n  id: fixture-one\n  name: Constructed One\n  when: \"2001-02-03T04:05:00Z\"\n  place: Test Bay\n  country: DE\n  lat: 48.4\n  lon: 10\n  tz: Europe/Berlin\nselection:\n  floating_bodies: eris,sun,eris\n  anchored_bodies: sedna,moon,eris\n"
	if err := os.WriteFile(filepath.Join(dir, "session.yaml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession() = _, %v, %v", ok, err)
	}
	if want := []string{"sedna", "eris"}; !reflect.DeepEqual(got.UnknownBodies, want) {
		t.Errorf("UnknownBodies = %v, want %v", got.UnknownBodies, want)
	}
	if len(got.Slots) != 1 || !sameBodies(got.Slots[0].Bodies, []aspect.Body{aspect.Moon}) {
		t.Errorf("anchored bodies = %+v, want [moon]", got.Slots)
	}
	if !sameBodies(got.FloatingBodies, []aspect.Body{aspect.Sun}) {
		t.Errorf("FloatingBodies = %v, want [sun]", got.FloatingBodies)
	}
}

// Verifiers for an earlier change: the session file records WHERE a chart was read from,
// not only what it is called, because those two facts fail separately.

// The locator survives a round trip for both kinds, so a relaunch can reopen
// the file rather than only recognise a slug. Positive control: drop the
// `kind:`/`file:` pair from formatSession and both subtests fail by name.
func TestSessionRemembersWhereAChartCameFrom(t *testing.T) {
	// A chart file's locator round-trips. A record naming no file this build reads
	// writes none, so it reads back with no locator at all.
	for _, tc := range []struct {
		kind ChartKind
		want ChartSource
	}{
		{ChartKindFile, ChartSource{Kind: ChartKindFile, File: "fixture-one.as"}},
		{ChartKindNone, ChartSource{}},
	} {
		kind := tc.kind
		t.Run("kind="+kind.String(), func(t *testing.T) {
			sessionSandbox(t)
			c := fixtureChart()
			c.Source = ChartSource{Kind: kind, File: "fixture-one.as"}
			if err := SaveSession(Session{Slots: oneSlot(c, false)}); err != nil {
				t.Fatal(err)
			}
			got, ok, err := LoadSession()
			if err != nil || !ok {
				t.Fatalf("LoadSession() = %v, %v", ok, err)
			}
			if len(got.Slots) != 1 {
				t.Fatal("the chart block did not come back")
			}
			if got.Slots[0].Chart.Source != tc.want {
				t.Errorf("source = %+v, want %+v", got.Slots[0].Chart.Source, tc.want)
			}
		})
	}
}

// A session written before an earlier change carries neither key. It must read back as
// what it actually meant — no locator, no chart file — rather than as a
// `.as` the relaunch would then try to reopen by an empty filename.
func TestASessionWithNoLocatorReadsAsNoLocator(t *testing.T) {
	dir := sessionSandbox(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	older := `version: 1
chart:
  id: fixture-one
  name: Constructed One
  when: "2001-02-03T04:05:00Z"
  place: Test Bay
  country: DE
  lat: 48.4
  lon: 10
  tz: Europe/Berlin
selection:
  comparison: true
`
	if err := os.WriteFile(filepath.Join(dir, "session.yaml"), []byte(older), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession() = %v, %v", ok, err)
	}
	if len(got.Slots) != 1 {
		t.Fatal("the chart block did not come back")
	}
	if want := (ChartSource{}); got.Slots[0].Chart.Source != want {
		t.Errorf("source = %+v, want the zero source %+v", got.Slots[0].Chart.Source, want)
	}
	if !got.Slots[0].Usable {
		t.Error("an older session's chart must still anchor; the locator is additive")
	}
}

// The wire names are their own constants so a renumbered enum cannot silently
// turn one kind of file into another across a relaunch. An unknown name reads
// as ChartKindNone, which is what String() writes for anything that is not a
// .as, so the two agree in both directions. "yaml" is one of those unknown
// names now: a record written when YAML charts existed names no file this
// build reads.
func TestChartKindWireNamesRoundTrip(t *testing.T) {
	for _, kind := range []ChartKind{ChartKindNone, ChartKindFile} {
		if got := ParseChartKind(kind.String()); got != kind {
			t.Errorf("ParseChartKind(%q) = %v, want %v", kind.String(), got, kind)
		}
	}
	for _, unknown := range []string{"", "yaml", "toml", "DAT", "1"} {
		if got := ParseChartKind(unknown); got != ChartKindNone {
			t.Errorf("ParseChartKind(%q) = %v, want ChartKindNone", unknown, got)
		}
	}
}

// A session record written while YAML charts existed carries `kind: yaml`. It
// must read back as ChartKindNone — a chart with no file this build reads —
// and still anchor from the snapshot.
func TestASessionRecordedAsYAMLReadsAsKindNone(t *testing.T) {
	dir := sessionSandbox(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	older := `version: 1
chart:
  id: fixture-one
  name: Constructed One
  when: "2001-02-03T04:05:00Z"
  place: Test Bay
  country: DE
  lat: 48.4
  lon: 10
  tz: Europe/Berlin
  kind: yaml
  file: fixture-one.yaml
`
	if err := os.WriteFile(filepath.Join(dir, "session.yaml"), []byte(older), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession() = %v, %v", ok, err)
	}
	if len(got.Slots) != 1 {
		t.Fatal("the chart block did not come back")
	}
	if got.Slots[0].Chart.Source.Kind != ChartKindNone {
		t.Errorf("kind = %v, want ChartKindNone for a record written with kind: yaml", got.Slots[0].Chart.Source.Kind)
	}
	if !got.Slots[0].Usable {
		t.Error("the snapshot must still anchor; only the file locator is gone")
	}
	// Written back, it carries no locator: neither the old `.yaml` path nor
	// an empty `kind:` beside it.
	out := formatSession(got)
	if strings.Contains(out, "kind:") || strings.Contains(out, "fixture-one.yaml") {
		t.Errorf("the rewritten session still names a locator nothing reads:\n%s", out)
	}
	if !strings.Contains(out, "id: fixture-one") || !strings.Contains(out, "name: Constructed One") {
		t.Errorf("the rewritten session lost its snapshot:\n%s", out)
	}
}

// listSession is three synthetic anchored slots with distinct identities,
// moments, places, selections and marks, plus a marked floating slot.
func listSession() Session {
	// The second chart is dated 44 BC, the third in the year 12345 and carries a
	// 70 KiB name: the reader takes any year the chart reader does and any line
	// the writer can produce in practice.
	mk := func(n int, kind ChartKind, file string) SessionChart {
		return SessionChart{
			ID:   fmt.Sprintf("synthetic-%d", n),
			Name: fmt.Sprintf("Synthetic %d", n),
			When: time.Date(1970+n, time.Month(n), 10+n, 6, n, 0, 0, time.UTC),
			Place: location.PlaceInfo{
				Name: fmt.Sprintf("Testford %d", n), Country: "TS",
				Lat: 40.5 + float64(n), Lon: -70.25 - float64(n), TZ: "America/New_York",
			},
			Source: ChartSource{Kind: kind, File: file},
		}
	}
	bc := mk(2, ChartKindNone, "")
	bc.When = time.Date(-43, 3, 15, 12, 0, 0, 0, time.UTC)
	far := mk(3, ChartKindFile, "synthetic-3.as")
	far.When = time.Date(12345, 6, 7, 8, 9, 10, 0, time.UTC)
	far.Name = strings.Repeat("L", 70*1024)
	first := mk(1, ChartKindFile, "synthetic-1.as")
	first.When = time.Date(100000, 1, 2, 3, 4, 5, 0, time.UTC) // six digits: the writer has no year bound
	return Session{
		Slots: []SessionSlot{
			{Chart: first, Usable: true, Selected: true, Bodies: []aspect.Body{aspect.Sun}},
			{Chart: bc, Usable: true, Bodies: []aspect.Body{aspect.Moon, aspect.Mars}},
			{Chart: far, Usable: true, Selected: true},
		},
		FloatingSelected: true,
		FloatingBodies:   []aspect.Body{aspect.Venus},
	}
}

// C14: every anchored slot's identity, snapshot, order, selection and marks,
// and the floating slot's selection and marks, come back.
func TestSessionRoundTripsTheList(t *testing.T) {
	sessionSandbox(t)
	in := listSession()
	if err := SaveSession(in); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	got, ok, err := LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession() = _, %v, %v", ok, err)
	}
	assertSessionEqual(t, got, in)
	for i, slot := range got.Slots {
		w := in.Slots[i].Chart
		if slot.Chart.ID != w.ID || slot.Chart.Source != w.Source || slot.Chart.Name != w.Name ||
			!slot.Chart.When.Equal(w.When) || slot.Chart.Place != w.Place {
			t.Errorf("slot %d came back as %+v, want %+v", i, slot.Chart, w)
		}
	}
	if got.DroppedSelections != 0 || len(got.UnknownBodies) != 0 {
		t.Errorf("a written session read back with dropped selections %d / unknown bodies %v", got.DroppedSelections, got.UnknownBodies)
	}
}

// C14 (as amended by the specification C11): the file declares version 3
// and carries one `charts:` sequence.
func TestSessionVersionIsThree(t *testing.T) {
	dir := sessionSandbox(t)
	if err := SaveSession(listSession()); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "session.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.HasPrefix(text, "version: 3\n") {
		t.Errorf("the file does not open with `version: 3`:\n%s", text)
	}
	if n := strings.Count(text, "\ncharts:\n"); n != 1 {
		t.Errorf("%d `charts:` blocks, want exactly one:\n%s", n, text)
	}
	if n := strings.Count(text, "\n  - id: "); n != 3 {
		t.Errorf("%d chart entries, want 3:\n%s", n, text)
	}
}

// C15: a version-1 file, written verbatim by the version-1 writer, restores as
// it always did: one anchored slot, the pair's selection, both slots' marks.
func TestSessionReadsVersionOne(t *testing.T) {
	dir := sessionSandbox(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join("testdata", "session-v1.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(fixture), "version: 1\n") {
		t.Fatalf("the fixture is not a version-1 file:\n%s", fixture)
	}
	if err := os.WriteFile(filepath.Join(dir, "session.yaml"), fixture, 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession() = _, %v, %v", ok, err)
	}
	if got.Version != 1 || len(got.Slots) != 1 {
		t.Fatalf("Version = %d with %d slots, want version 1 and one slot", got.Version, len(got.Slots))
	}
	slot := got.Slots[0]
	c := slot.Chart
	if !slot.Usable || c.ID != "synthetic-aster" || c.Name != "Synthetic Aster" ||
		c.Source != (ChartSource{Kind: ChartKindFile, File: "synthetic-aster.as"}) ||
		!c.When.Equal(time.Date(1985, 7, 14, 9, 30, 0, 0, time.UTC)) ||
		c.Place.Name != "Testford" || c.Place.Lat != 41.25 || c.Place.Lon != -73.5 || c.Place.TZ != "America/New_York" {
		t.Errorf("the anchored slot came back as usable=%v %+v", slot.Usable, c)
	}
	if !slot.Selected || !got.FloatingSelected {
		t.Errorf("selection = anchored %v / floating %v, want both (comparison: true)", slot.Selected, got.FloatingSelected)
	}
	if !sameBodies(slot.Bodies, []aspect.Body{aspect.Sun, aspect.Moon}) || !sameBodies(got.FloatingBodies, []aspect.Body{aspect.Mars}) {
		t.Errorf("marks = %v / %v, want [sun moon] / [mars]", slot.Bodies, got.FloatingBodies)
	}
}

// C16: only the integers 1 and 2 are read; anything else restores no slot and
// comes back as an error the caller can put on the Log.
func TestSessionRejectsUnknownVersion(t *testing.T) {
	body := "charts:\n  - id: x\n    name: n\n    when: \"2001-02-03T04:05:00Z\"\n    place: p\n    lat: 1\n    lon: 2\n    tz: UTC\n"
	for _, v := range []string{"version: 0", "version: -1", "version: 4", "version: 99", "version: two", "version: 2.5", "version:", "# no version line"} {
		assertRefused(t, v+"\n"+body)
	}
	// And the two it does read are not refused by the same rule.
	for _, v := range []string{"version: 1", "version: 2", "version: 3"} {
		dir := sessionSandbox(t)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "session.yaml"), []byte(v+"\n"+body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, ok, err := LoadSession(); !ok || err != nil {
			t.Errorf("%q was refused: ok=%v err=%v", v, ok, err)
		}
	}
}

func loadText(t *testing.T, text string) Session {
	t.Helper()
	dir := sessionSandbox(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session.yaml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession() = _, %v, %v", ok, err)
	}
	return got
}

// strictChart is a usable synthetic chart; the tests below write it with the
// production writer, so every line of it is in the form the writer emits.
func strictChart(n int) SessionChart {
	return SessionChart{
		ID:   fmt.Sprintf("client-%d", n),
		Name: fmt.Sprintf("Client %d", n),
		When: time.Date(1960+n, time.Month(n), 10+n, 6, n, 0, 0, time.UTC),
		Place: location.PlaceInfo{
			Name: fmt.Sprintf("Place %d", n), Country: "TS", Lat: 10 + float64(n), Lon: 20 + float64(n), TZ: "Europe/Berlin",
		},
	}
}

// chartsText writes charts through the production writer, in list order.
func chartsText(slots ...SessionSlot) string {
	return formatSession(Session{Slots: slots})
}

func usableAndDropped(s Session) (usable, dropped int) {
	for _, sl := range s.Slots {
		if sl.Usable {
			usable++
		} else {
			dropped++
		}
	}
	return
}

// C17: an entry that is malformed, misses a required field, names no chart by
// id or file, or carries no loadable timezone is marked unusable alone; the
// rest stay, in order.
func TestSessionMarksUnresolvableEntriesUnusable(t *testing.T) {
	entry := func(n int) SessionSlot { return SessionSlot{Chart: strictChart(n), Selected: true} }
	mid := chartsText(entry(0))
	mid = mid[strings.Index(mid, "  - id:"):strings.Index(mid, "floating:")]
	whenLit := yamlEscape(strictChart(0).When.UTC().Format(time.RFC3339))
	broken := map[string]string{
		"no when":     strings.Replace(mid, "    when: "+whenLit+"\n", "", 1),
		"bad when":    strings.Replace(mid, whenLit, "yesterday", 1),
		"no place":    strings.Replace(mid, "    place: Place 0\n", "", 1),
		"no lat":      strings.Replace(mid, "    lat: 10\n", "", 1),
		"no lon":      strings.Replace(mid, "    lon: 20\n", "", 1),
		"no tz":       strings.Replace(mid, "    tz: Europe/Berlin\n", "", 1),
		"unknown tz":  strings.Replace(mid, "Europe/Berlin", "Mars/Olympus_Mons", 1),
		"no id/file":  strings.Replace(mid, "id: client-0", `id: ""`, 1),
		"lat is text": strings.Replace(mid, "lat: 10", "lat: north", 1),
	}
	for name, bad := range broken {
		t.Run(name, func(t *testing.T) {
			if bad == mid {
				t.Fatal("fixture: the variant changed nothing")
			}
			good1, good2 := chartsText(entry(1)), chartsText(entry(2))
			body := func(s string) string { return s[strings.Index(s, "  - id:"):strings.Index(s, "floating:")] }
			text := "version: 2\ncharts:\n" + body(good1) + bad + body(good2) + "floating:\n  selected: false\n"
			got := loadText(t, text)
			if len(got.Slots) != 3 {
				t.Fatalf("%d entries read, want 3 (the bad one kept, marked)", len(got.Slots))
			}
			usable := []bool{got.Slots[0].Usable, got.Slots[1].Usable, got.Slots[2].Usable}
			if !reflect.DeepEqual(usable, []bool{true, false, true}) {
				t.Errorf("usable = %v, want only the middle entry refused", usable)
			}
			if got.Slots[0].Chart.ID != "client-1" || got.Slots[2].Chart.ID != "client-2" {
				t.Errorf("the good entries came back out of order or damaged: %+v", got.Slots)
			}
		})
	}
}

// C17a: more than six selections keeps the first six in list order, floating
// last, and counts what it turned off.
func TestRestoreCapsSelectionAtSix(t *testing.T) {
	var slots []SessionSlot
	for i := 1; i <= 7; i++ {
		slots = append(slots, SessionSlot{Chart: strictChart(i), Selected: true})
	}
	got := loadText(t, formatSession(Session{Slots: slots, FloatingSelected: true}))
	sel := make([]bool, len(got.Slots))
	for i, s := range got.Slots {
		sel[i] = s.Selected
	}
	if want := []bool{true, true, true, true, true, true, false}; !reflect.DeepEqual(sel, want) {
		t.Errorf("selected = %v, want the first six only", sel)
	}
	if got.FloatingSelected {
		t.Error("the floating slot stayed selected as a seventh")
	}
	if got.DroppedSelections != 2 {
		t.Errorf("DroppedSelections = %d, want 2 (the seventh anchored slot and the floating slot)", got.DroppedSelections)
	}
}

// The strict rule (operator, 2026-10-04, "option 1, strict"): session.yaml is
// machine-managed, so the reader accepts only the exact layout Sextile writes
// and drops anything else, by name. An entry is one `  - id: …` marker and
// `    key: value` field lines in the writer's order and forms; any other line
// inside `charts:` makes the entry it falls in unusable, and lines are skipped
// to the next valid marker, the whole span counting as ONE dropped entry.
func TestSessionEntryBoundaryNeverMixesCharts(t *testing.T) {
	a := strictChart(1)
	b := strictChart(2)
	body := func(c SessionChart) string {
		s := chartsText(SessionSlot{Chart: c, Selected: true})
		return s[strings.Index(s, "  - id:"):strings.Index(s, "floating:")]
	}
	bFields := strings.SplitN(body(b), "\n", 2)[1]
	cases := map[string]string{
		"a bare dash line":            "  -\n",
		"a flow-style entry marker":   "  - {}\n",
		"a line that is not a key":    "    garbage without a colon\n",
		"a tab-indented entry marker": "\t-\n",
		"a column-zero dash":          "- id: client-2\n",
	}
	for name, stray := range cases {
		t.Run(name, func(t *testing.T) {
			got := loadText(t, "version: 2\ncharts:\n"+body(a)+stray+bFields+"floating:\n  selected: false\n")
			usable, dropped := usableAndDropped(got)
			if usable != 0 || dropped != 1 {
				t.Errorf("%d usable, %d dropped, want one dropped span and nothing restored: %+v", usable, dropped, got.Slots)
			}
		})
	}
	t.Run("a repeated key drops the entry", func(t *testing.T) {
		got := loadText(t, "version: 2\ncharts:\n"+body(a)+"    when: \"1999-09-09T09:09:00Z\"\nfloating:\n  selected: false\n")
		if usable, dropped := usableAndDropped(got); usable != 0 || dropped != 1 {
			t.Errorf("%d usable, %d dropped, want the entry dropped", usable, dropped)
		}
	})
}

// A column-zero block sequence is valid YAML and not what the writer writes:
// strict reading drops it, and says so by counting it.
func TestSessionDropsColumnZeroEntries(t *testing.T) {
	text := "version: 2\ncharts:\n"
	for _, n := range []int{1, 2} {
		s := chartsText(SessionSlot{Chart: strictChart(n), Selected: true})
		s = s[strings.Index(s, "  - id:"):strings.Index(s, "floating:")]
		lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
		lines[0] = strings.TrimPrefix(lines[0], "  ")
		for i := 1; i < len(lines); i++ {
			lines[i] = strings.TrimPrefix(lines[i], "  ")
		}
		text += strings.Join(lines, "\n") + "\n"
	}
	got := loadText(t, text+"floating:\n  selected: false\n")
	usable, dropped := usableAndDropped(got)
	if usable != 0 || dropped == 0 {
		t.Errorf("%d usable, %d dropped: both column-zero entries must be dropped and counted", usable, dropped)
	}
}

// A bare `-` is a dropped entry, counted, and the valid entry after it restores.
func TestSessionCountsABareDashAsDropped(t *testing.T) {
	s := chartsText(SessionSlot{Chart: strictChart(1)})
	entry := s[strings.Index(s, "  - id:"):strings.Index(s, "floating:")]
	got := loadText(t, "version: 2\ncharts:\n  -\n"+entry+"floating:\n  selected: false\n")
	if usable, dropped := usableAndDropped(got); usable != 1 || dropped != 1 {
		t.Errorf("%d usable, %d dropped, want 1 and 1", usable, dropped)
	}
}

// Fields are read only as the writer lays them out. A nested block under an
// unknown key, a deeper line, or an unknown key never contributes a field, and
// the entry it falls in is dropped.
func TestSessionNestedBlockNeverBecomesFields(t *testing.T) {
	s := chartsText(SessionSlot{Chart: strictChart(1)})
	entry := s[strings.Index(s, "  - id:"):strings.Index(s, "floating:")]
	bob := "      name: Bob\n      when: \"1999-09-09T09:09:00Z\"\n      place: Place B\n      lat: 30\n      lon: 40\n      tz: Asia/Tokyo\n"
	idLine := strings.SplitN(entry, "\n", 2)[0] + "\n"
	cases := map[string]string{
		"an unknown key with nested children": idLine + "    extra:\n" + bob,
		"a known key at a deeper indent":      entry + "      selected: true\n",
		"a required field only when deeper":   strings.Replace(entry, "    when:", "      when:", 1),
		"an unknown key at the field indent":  entry + "    lens: hellenistic\n",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			got := loadText(t, "version: 2\ncharts:\n"+text+"floating:\n  selected: false\n")
			if usable, dropped := usableAndDropped(got); usable != 0 || dropped != 1 {
				t.Errorf("%d usable, %d dropped, want the one entry dropped: %+v", usable, dropped, got.Slots)
			}
		})
	}
	got := loadText(t, "version: 2\ncharts:\n"+entry+"floating:\n  selected: false\n")
	if usable, _ := usableAndDropped(got); usable != 1 || got.Slots[0].Chart.Name != "Client 1" {
		t.Errorf("the ordinary entry: %+v", got.Slots)
	}
}

// The point of the strict rule: however a written session is damaged — one line
// deleted, doubled, re-indented, tabbed, swapped, or another chart's fields
// nested in — a restored chart is EXACTLY one of the charts that was written,
// never a blend of two, and never twice.
func TestSessionMutationsNeverMixCharts(t *testing.T) {
	c1, c2, c3 := strictChart(1), strictChart(2), strictChart(3)
	c1.Source = ChartSource{Kind: ChartKindFile, File: "client-1.as"}
	c3.Place.Country = ""
	c3.Name = `Client "3": a # name`
	// Years the chart reader accepts: BC, and more than four digits.
	c1.When = time.Date(100000, 1, 2, 3, 4, 5, 0, time.UTC)
	c2.When = time.Date(-43, 3, 15, 12, 0, 0, 0, time.UTC)
	c3.When = time.Date(12345, 6, 7, 8, 9, 10, 0, time.UTC)
	originals := []SessionSlot{
		{Chart: c1, Selected: true, Bodies: []aspect.Body{aspect.Sun, aspect.Moon}},
		{Chart: c2},
		{Chart: c3, Selected: true, Bodies: []aspect.Body{aspect.Mars}},
	}
	text := formatSession(Session{Slots: originals, FloatingSelected: true, FloatingBodies: []aspect.Body{aspect.Venus}})
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	join := func(ls []string) string { return strings.Join(ls, "\n") + "\n" }

	same := func(a, b SessionSlot) bool {
		return a.Chart.ID == b.Chart.ID && a.Chart.Source == b.Chart.Source && a.Chart.Name == b.Chart.Name &&
			a.Chart.When.Equal(b.Chart.When) && a.Chart.Place == b.Chart.Place && a.Selected == b.Selected && sameBodies(a.Bodies, b.Bodies)
	}
	nested := func(c SessionChart) []string {
		return []string{
			"    extra:",
			"      name: " + yamlEscape(c.Name),
			"      when: " + yamlEscape(c.When.UTC().Format(time.RFC3339)),
			"      place: " + yamlEscape(c.Place.Name),
			"      lat: " + strconv.FormatFloat(c.Place.Lat, 'f', -1, 64),
			"      lon: " + strconv.FormatFloat(c.Place.Lon, 'f', -1, 64),
			"      tz: " + yamlEscape(c.Place.TZ),
		}
	}
	insert := func(ls []string, after int, extra ...string) []string {
		out := append([]string(nil), ls[:after+1]...)
		out = append(out, extra...)
		return append(out, ls[after+1:]...)
	}

	// Which entry each written line belongs to (-1: not an entry line).
	entryOf := make([]int, len(lines))
	isMarker := make([]bool, len(lines))
	chartsLine := -1
	k := -1
	for i, l := range lines {
		entryOf[i] = -1
		switch {
		case l == "charts:":
			chartsLine = i
		case strings.HasPrefix(l, "  - id: "):
			k++
			isMarker[i] = true
			entryOf[i] = k
		case strings.HasPrefix(l, "    ") && k >= 0:
			entryOf[i] = k
		default:
			if k >= 0 && !strings.HasPrefix(l, "  ") {
				k = -2 // past the charts block
			}
		}
	}
	// affectedBy is the set of entries a mutation of the given lines can spoil —
	// the entries the lines belong to, and the entry before a marker, which the
	// marker's fields fall into when the marker goes — or ok=false when it
	// touches something that is not an entry line (its blast radius is the
	// file's, and only the no-mixing property is asserted).
	const strayBeforeFirst = -100
	affectedBy := func(touched ...int) (set map[int]bool, ok bool) {
		set = map[int]bool{}
		for _, i := range touched {
			if i == strayBeforeFirst {
				continue // an insertion after `charts:`: a stray span, no entry
			}
			if entryOf[i] < 0 {
				return nil, false
			}
			set[entryOf[i]] = true
			if isMarker[i] && entryOf[i] > 0 {
				set[entryOf[i]-1] = true
			}
		}
		return set, true
	}

	insTouch := func(i int) int {
		if i == chartsLine {
			return strayBeforeFirst
		}
		return i
	}
	runs := 0
	check := func(label string, mutated []string, touched ...int) {
		t.Helper()
		runs++
		dir := sessionSandbox(t)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "session.yaml"), []byte(join(mutated)), 0o644); err != nil {
			t.Fatal(err)
		}
		got, ok, err := LoadSession()
		affected, confined := affectedBy(touched...)
		if err != nil || !ok {
			if confined {
				t.Errorf("%s: the file was refused whole, but the damage was confined to entries %v\n%s", label, affected, join(mutated))
			}
			return // refused whole: nothing restored, nothing mixed
		}
		seen := map[int]bool{}
		order := []int{}
		usable := 0
		for _, sl := range got.Slots {
			if !sl.Usable {
				continue
			}
			usable++
			match := -1
			for i, o := range originals {
				if same(sl, o) {
					match = i
				}
			}
			if match < 0 {
				t.Errorf("%s: a restored chart is none of the written ones: %+v\n%s", label, sl, join(mutated))
				continue
			}
			if seen[match] {
				t.Errorf("%s: original %d was restored twice\n%s", label, match, join(mutated))
			}
			seen[match] = true
			order = append(order, match)
		}
		if len(got.Slots) > len(originals)+1 {
			t.Errorf("%s: %d entries (usable %d), more than the %d written plus one stray span\n%s", label, len(got.Slots), usable, len(originals), join(mutated))
		}
		// Survival: damage confined to some entries leaves every OTHER entry
		// restored, usable, in its original relative order. A reader that
		// drops everything passes the checks above and fails this one.
		if confined {
			want := []int{}
			for j := range originals {
				if !affected[j] {
					want = append(want, j)
					if !seen[j] {
						t.Errorf("%s: entry %d was untouched and did not restore\n%s", label, j, join(mutated))
					}
				}
			}
			var survivors []int
			for _, j := range order {
				if !affected[j] {
					survivors = append(survivors, j)
				}
			}
			if !reflect.DeepEqual(survivors, want) {
				t.Errorf("%s: untouched entries restored as %v, want %v in order\n%s", label, survivors, want, join(mutated))
			}
		}
	}

	for i := range lines {
		cp := func() []string { return append([]string(nil), lines...) }
		// delete line i
		d := cp()
		check(fmt.Sprintf("delete %d", i), append(d[:i], d[i+1:]...), i)
		// duplicate line i
		check(fmt.Sprintf("duplicate %d", i), insert(cp(), i, lines[i]), insTouch(i))
		// re-indent by -2, -1, +1, +2
		for _, delta := range []int{-2, -1, 1, 2} {
			m := cp()
			trim := strings.TrimLeft(lines[i], " ")
			lead := len(lines[i]) - len(trim) + delta
			if lead < 0 {
				continue
			}
			m[i] = strings.Repeat(" ", lead) + trim
			check(fmt.Sprintf("indent %+d on %d", delta, i), m, i)
		}
		// leading spaces become a tab
		if strings.HasPrefix(lines[i], " ") {
			m := cp()
			m[i] = "\t" + strings.TrimLeft(lines[i], " ")
			check(fmt.Sprintf("tab on %d", i), m, i)
		}
		// swap with the next line
		if i+1 < len(lines) {
			m := cp()
			m[i], m[i+1] = m[i+1], m[i]
			check(fmt.Sprintf("swap %d,%d", i, i+1), m, i, i+1)
		}
		// an unknown key at four spaces
		check(fmt.Sprintf("unknown key after %d", i), insert(cp(), i, "    lens: hellenistic"), insTouch(i))
		// a blank line, a comment, and a trailing space
		check(fmt.Sprintf("blank after %d", i), insert(cp(), i, ""), insTouch(i))
		check(fmt.Sprintf("comment after %d", i), insert(cp(), i, "# a comment"), insTouch(i))
		m := cp()
		m[i] += " "
		check(fmt.Sprintf("trailing space on %d", i), m, i)
		// another chart's fields nested under an unknown key
		for _, other := range []SessionChart{c1, c2, c3} {
			check(fmt.Sprintf("nested %s after %d", other.ID, i), insert(cp(), i, nested(other)...), insTouch(i))
		}
		// a marker in column-zero form
		if strings.HasPrefix(lines[i], "  - ") {
			m := cp()
			m[i] = strings.TrimPrefix(lines[i], "  ")
			check(fmt.Sprintf("column zero on %d", i), m, i)
		}
	}
	t.Logf("%d mutations over %d lines", runs, len(lines))
	if runs < 500 {
		t.Fatalf("only %d mutations ran", runs)
	}
}

// A line longer than the scanner's limit must refuse the file, not truncate it:
// reading on past it would silently lose every chart after it, and the next quit
// would write the shorter list over the file.
func TestSessionTooLongLineIsRefusedNotTruncated(t *testing.T) {
	long := strictChart(2)
	long.Name = strings.Repeat("N", sessionMaxLine+1024) // past the reader's 1 MiB line limit
	cases := map[string]string{
		"version 2": chartsText(SessionSlot{Chart: strictChart(1)}, SessionSlot{Chart: long}, SessionSlot{Chart: strictChart(3)}),
		"version 1": "version: 1\nchart:\n  id: x\n  name: " + long.Name + "\n  when: \"2001-02-03T04:05:00Z\"\n  place: P\n  lat: 1\n  lon: 2\n  tz: UTC\nselection:\n  comparison: false\n",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			dir := sessionSandbox(t)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "session.yaml"), []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
			got, ok, err := LoadSession()
			if !errors.Is(err, ErrSessionLineTooLong) || ok || len(got.Slots) != 0 {
				t.Errorf("LoadSession() = %d slots, ok %v, err %v; want a refusal naming the long line", len(got.Slots), ok, err)
			}
		})
	}
}

// The limit is inclusive: a line of exactly sessionMaxLine bytes of content
// reads, one byte more is refused.
func TestSessionLineLimitIsInclusive(t *testing.T) {
	line := func(n int) string { return "    name: " + strings.Repeat("N", n-len("    name: ")) }
	for _, tc := range []struct {
		name    string
		size    int
		refused bool
	}{{"exactly at the limit", sessionMaxLine, false}, {"one byte over", sessionMaxLine + 1, true}} {
		t.Run(tc.name, func(t *testing.T) {
			base := chartsText(SessionSlot{Chart: strictChart(1)})
			text := strings.Replace(base, "    name: Client 1", line(tc.size), 1)
			if len(line(tc.size)) != tc.size {
				t.Fatalf("fixture: line is %d bytes", len(line(tc.size)))
			}
			dir := sessionSandbox(t)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "session.yaml"), []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
			got, ok, err := LoadSession()
			if tc.refused {
				if !errors.Is(err, ErrSessionLineTooLong) {
					t.Errorf("a line of %d bytes: err %v, want a refusal", tc.size, err)
				}
				return
			}
			if err != nil || !ok || len(got.Slots) != 1 || !got.Slots[0].Usable {
				t.Errorf("a line of exactly the limit: ok %v err %v slots %+v, want it read", ok, err, got.Slots)
			}
		})
	}
}
