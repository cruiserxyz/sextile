package store

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

func marksSlots(t *testing.T) *Slots {
	t.Helper()
	place := location.PlaceInfo{Name: "Berlin", Country: "DE", TZ: "Europe/Berlin"}
	s, err := NewSlots(time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC), place)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddAnchored(SessionChart{}, time.Date(1990, 1, 1, 14, 0, 0, 0, time.UTC), place); err != nil {
		t.Fatal(err)
	}
	return s
}

// A mark is on the object's number: any object marks, the list comes back in
// number order whatever the order of marking, and the zero object is refused.
func TestMarksKeyOnTheNumber(t *testing.T) {
	s := marksSlots(t)
	star := aspect.Object{Number: 60, Name: "Algol"}
	for _, o := range []aspect.Object{star, objFortune, objSun} {
		if sel, ok := s.ToggleObjectSelection(0, o); !ok || !sel {
			t.Fatalf("marking %v = (%v, %v)", o, sel, ok)
		}
	}
	if got, want := s.SelectedObjects(0), []aspect.Object{objSun, objFortune, star}; !slices.Equal(got, want) {
		t.Errorf("SelectedObjects = %v, want %v (ascending number)", got, want)
	}
	if !s.ObjectSelected(0, 19) || s.ObjectSelected(1, 19) {
		t.Error("the Fortune mark is not on slot 0 alone")
	}
	if sel, ok := s.ToggleObjectSelection(0, objFortune); !ok || sel || s.ObjectSelected(0, 19) {
		t.Error("a second toggle did not unmark Fortune")
	}
	earth := aspect.Object{Number: 0, Name: "Earth"}
	if sel, ok := s.ToggleObjectSelection(0, earth); !ok || !sel || !s.ObjectSelected(0, 0) {
		t.Error("Earth (number 0, named) was refused; only the zero Object is")
	}
	if got := s.SelectedObjects(0); len(got) == 0 || got[0] != earth {
		t.Errorf("Earth is not listed first: %v", got)
	}
	if _, ok := s.ToggleObjectSelection(0, aspect.Object{}); ok {
		t.Error("the zero object was accepted")
	}
}

// Kept names belong to their slot: they copy with it, count as a selection,
// collapse a repeat, and stay out of the marked objects.
func TestKeptMarksFollowTheSlot(t *testing.T) {
	s := marksSlots(t)
	s.KeepMark(0, "sedna")
	s.KeepMark(0, "sedna")
	s.KeepMark(0, "eris")
	s.KeepMark(99, "ignored")
	if got := s.KeptMarks(0); !slices.Equal(got, []string{"sedna", "eris"}) {
		t.Fatalf("KeptMarks = %v, want [sedna eris] once each, in order", got)
	}
	if s.HasMarks(0) {
		t.Error("kept names reserve the marker column")
	}
	if len(s.SelectedObjects(0)) != 0 {
		t.Error("a kept name came back as a marked object")
	}
	next := marksSlots(t)
	next.CopySelectionsFrom(s)
	if got := next.KeptMarks(0); !slices.Equal(got, []string{"sedna", "eris"}) {
		t.Errorf("the rebuild lost the kept names: %v", got)
	}
	next.KeepMark(0, "vesta")
	if len(s.KeptMarks(0)) != 2 {
		t.Error("the copy shares its kept names with the original")
	}
}

// The file holds the names as given, joined by ", ", for an entry and for the
// floating slot alike.
func TestSessionBodiesAreNames(t *testing.T) {
	dir := sessionSandbox(t)
	in := Session{
		Slots:            oneSlot(fixtureChart(), false, "Sun", "Fortune", "2nd Cusp", "sedna"),
		FloatingSelected: false,
		FloatingBodies:   []string{"North Node", "Nadir"},
	}
	if err := SaveSession(in); err != nil {
		t.Fatal(err)
	}
	got := readSessionText(t, dir)
	for _, line := range []string{"    bodies: Sun,Fortune,2nd Cusp,sedna\n", "  bodies: North Node,Nadir\n"} {
		if !strings.Contains(got, line) {
			t.Errorf("session file lacks %q:\n%s", line, got)
		}
	}
	back, ok, err := LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession = %v, %v", ok, err)
	}
	if !slices.Equal(back.Slots[0].Bodies, in.Slots[0].Bodies) || !slices.Equal(back.FloatingBodies, in.FloatingBodies) {
		t.Errorf("names came back as %v / %v", back.Slots[0].Bodies, back.FloatingBodies)
	}
}

// The splitter, and the floating line it reads, tolerate a space after the
// comma: a hand-written ", " line and the writer's bare comma give the same
// names, and an empty entry is no name. (A chart ENTRY is stricter, see
// TestAnchoredBodiesKeepTheWritersForm.)
func TestSplitNamesAndFloatingLineTolerateASpace(t *testing.T) {
	sessionSandbox(t)
	for _, c := range []struct {
		val  string
		want []string
	}{
		{"sun,north_node", []string{"sun", "north_node"}},
		{"Sun, North Node", []string{"Sun", "North Node"}},
		{" Sun ,, 2nd Cusp ,", []string{"Sun", "2nd Cusp"}},
		{"", nil},
	} {
		if got := splitNames(c.val); !slices.Equal(got, c.want) {
			t.Errorf("splitNames(%q) = %v, want %v", c.val, got, c.want)
		}
	}
	text := "version: 3\nfloating:\n  selected: false\n  bodies: sun,north_node\n"
	got, ok, err := parseSessionFile("x", []byte(text))
	if err != nil || !ok || !slices.Equal(got.FloatingBodies, []string{"sun", "north_node"}) {
		t.Errorf("a 0.1.2 floating line read as %v (ok %v err %v)", got.FloatingBodies, ok, err)
	}
}

// A named session is read strictly: a bodies value must be what the writer
// emits (the bare comma), and ", " is unknown content.
func TestNamedSessionBodiesStrictForm(t *testing.T) {
	for _, c := range []struct {
		val string
		ok  bool
	}{
		{"Sun,Fortune", true},
		{"sun,moon", true},
		{"Sun", true},
		{`""`, true},
		{"Sun, Fortune", false},
		{"Sun,  Fortune", false},
		{"Sun ,Fortune", false},
		{"Sun,,Fortune", false},
		{"Sun,Fortune,", false},
	} {
		if _, ok := canonicalValue("bodies", c.val); ok != c.ok {
			t.Errorf("canonicalValue(bodies, %q) ok = %v, want %v", c.val, ok, c.ok)
		}
	}
	sessionSandbox(t)
	l, err := SaveNamedSessionAs("marks", Session{Slots: oneSlot(fixtureChart(), false, "Sun", "Fortune", "sedna")})
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	s, _, err := LoadNamedSession(l.File())
	if err != nil {
		t.Fatalf("the writer's own named file was refused: %v", err)
	}
	if !slices.Equal(s.Slots[0].Bodies, []string{"Sun", "Fortune", "sedna"}) {
		t.Errorf("names = %v", s.Slots[0].Bodies)
	}
}

// A named session as 0.1.2 wrote it, byte for byte (wire names, the bare comma,
// the name: line straight after version: 3, the generation 0.1.2 declared),
// still opens, and a save of what was read writes this build's generation, 4,
// so that a 0.1.2 reading it back refuses it whole and keeps it aside instead
// of rewriting it without the names it does not know.
func TestNamedSessionLegacyBodiesOpens(t *testing.T) {
	sessionSandbox(t)
	l, err := SaveNamedSessionAs("legacy", Session{Slots: oneSlot(fixtureChart(), false)})
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	const legacy = `version: 3
name: legacy
charts:
  - id: fixture-one
    name: Constructed One
    when: "2001-02-03T04:05:00Z"
    place: Test Bay
    country: TS
    lat: 12.34
    lon: 56.78
    tz: Etc/UTC
    selected: false
    bodies: sun,north_node
floating:
  selected: false
`
	if err := os.WriteFile(NamedSessionPath(l.File()), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _, err := LoadNamedSession(l.File())
	if err != nil {
		t.Fatalf("a 0.1.2 named session did not open: %v", err)
	}
	if !slices.Equal(s.Slots[0].Bodies, []string{"sun", "north_node"}) {
		t.Errorf("names = %v, want the wire names verbatim", s.Slots[0].Bodies)
	}
	l2, err := SaveNamedSessionAs("legacy again", s)
	if err != nil {
		t.Fatal(err)
	}
	l2.Release()
	b, _ := os.ReadFile(NamedSessionPath(l2.File()))
	if !strings.HasPrefix(string(b), "version: 4\n") {
		t.Errorf("a save of what was read does not write version 4:\n%s", b)
	}
}

// A chart ENTRY's bodies value must be the writer's form (the bare join), as
// every other key of an entry, or the entry is unusable.
func TestAnchoredBodiesKeepTheWritersForm(t *testing.T) {
	sessionSandbox(t)
	for _, c := range []struct {
		val    string
		usable bool
	}{{"Sun,Fortune", true}, {"Sun, Fortune", false}} {
		if err := SaveSession(Session{Slots: oneSlot(fixtureChart(), false, "Sun", "Fortune")}); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(SessionPath())
		text := strings.Replace(string(b), "bodies: Sun,Fortune", "bodies: "+c.val, 1)
		got, ok, err := parseSessionFile("x", []byte(text))
		if err != nil || !ok {
			t.Fatalf("%q: parse = %v, %v", c.val, ok, err)
		}
		usable := len(got.Slots) == 1 && got.Slots[0].Usable
		if usable != c.usable {
			t.Errorf("bodies: %s leaves the entry usable = %v, want %v (%+v)", c.val, usable, c.usable, got.Slots)
		}
	}
}

// Names that need quoting round-trip unchanged on an anchored entry and on the
// floating line.
func TestSessionBodiesEscapeRoundTrip(t *testing.T) {
	all := []string{`say "hi"`, "#hash", "a: b", "ctl\x01char", "Sun"}
	lists := [][]string{all, {`"quoted"`}, {"#hash"}, {"a: b"}, {"ctl\x01char"}, {`say "hi"`}}
	for _, names := range lists {
		sessionSandbox(t)
		in := Session{Slots: oneSlot(fixtureChart(), false, names...), FloatingBodies: names}
		if err := SaveSession(in); err != nil {
			t.Fatal(err)
		}
		got, ok, err := LoadSession()
		if err != nil || !ok {
			t.Fatalf("%q: LoadSession = %v, %v", names, ok, err)
		}
		if !slices.Equal(got.Slots[0].Bodies, names) {
			t.Errorf("anchored names = %q, want %q", got.Slots[0].Bodies, names)
		}
		if !slices.Equal(got.FloatingBodies, names) {
			t.Errorf("floating names = %q, want %q", got.FloatingBodies, names)
		}
	}
}

func readSessionText(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "session.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
