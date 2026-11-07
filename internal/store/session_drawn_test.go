package store

import (
	"errors"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The `drawn:` line (specs/session-drawn-set.md): written only when the session
// holds a drawn set, before `charts:`, read back name for name.
func TestSessionDrawnRoundTrip(t *testing.T) {
	sessionSandbox(t)
	want := []string{"Sun", "Moon", "Fortune", "East Point"}
	s := Session{Drawn: want, Slots: oneSlot(fixtureChart(), false)}
	if err := SaveSession(s); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(SessionPath())
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	const line = "drawn: Sun,Moon,Fortune,East Point\n"
	i := strings.Index(text, line)
	if i < 0 {
		t.Fatalf("the file lacks %q:\n%s", line, text)
	}
	if c := strings.Index(text, "charts:"); c < i {
		t.Errorf("drawn: comes after charts: in\n%s", text)
	}
	got, ok, err := LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession: ok %v err %v", ok, err)
	}
	if !slices.Equal(got.Drawn, want) {
		t.Errorf("Drawn = %q, want %q", got.Drawn, want)
	}

	// No drawn set: no line, and nil comes back.
	if err := SaveSession(Session{Slots: oneSlot(fixtureChart(), false)}); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(SessionPath())
	if strings.Contains(string(b), "drawn:") {
		t.Errorf("a session without a drawn set wrote a drawn: line:\n%s", b)
	}
	if got, _, _ := LoadSession(); got.Drawn != nil {
		t.Errorf("Drawn = %q, want nil", got.Drawn)
	}

	// An empty, non-nil set is the bare line, exactly.
	if err := SaveSession(Session{Drawn: []string{}}); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(SessionPath())
	if !strings.Contains(string(b), "\ndrawn:\n") {
		t.Errorf("an empty drawn set is not the bare line drawn:\n%s", b)
	}
	if got, _, _ := LoadSession(); got.Drawn == nil || len(got.Drawn) != 0 {
		t.Errorf("an empty drawn set came back as %#v", got.Drawn)
	}
}

// Literal files: the separator may be ", " or a bare ",", names keep their
// spaces, and a bare line is an empty list.
func TestSessionDrawnReadsLiteralForms(t *testing.T) {
	sessionSandbox(t)
	for _, c := range []struct {
		line string
		want []string
	}{
		{"drawn: Sun, North Node, East Point", []string{"Sun", "North Node", "East Point"}},
		{"drawn: Sun,North Node,East Point", []string{"Sun", "North Node", "East Point"}},
		{"drawn:", []string{}},
	} {
		text := "version: 3\n" + c.line + "\nfloating:\n  selected: false\n"
		got, ok, err := parseSessionFile("x", []byte(text))
		if err != nil || !ok || !slices.Equal(got.Drawn, c.want) || got.Drawn == nil {
			t.Errorf("%q read as %#v (ok %v err %v), want %#v", c.line, got.Drawn, ok, err, c.want)
		}
	}
}

// A named version-3 file with the bare line opens through the strict reader.
func TestNamedSessionBareDrawnOpens(t *testing.T) {
	sessionSandbox(t)
	l, err := SaveNamedSessionAs("bare", Session{Drawn: []string{}, Slots: oneSlot(fixtureChart(), false)})
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	if b, _ := os.ReadFile(NamedSessionPath(l.File())); !strings.Contains(string(b), "\ndrawn:\n") {
		t.Fatalf("fixture: the named file has no bare line:\n%s", b)
	}
	s, _, err := LoadNamedSession(l.File())
	if err != nil {
		t.Fatalf("the named session did not open: %v", err)
	}
	if s.Drawn == nil || len(s.Drawn) != 0 {
		t.Errorf("Drawn = %#v, want empty and not nil", s.Drawn)
	}
}

// A general session written before the bare comma (`drawn: Sun, Moon`) reads
// back the same set, and the next save writes the bare comma (spec
// session-drawn-bare-comma, rule 2).
func TestDrawnLineReadsTheOldJoin(t *testing.T) {
	sessionSandbox(t)
	got, ok, err := parseSessionFile("x", []byte("version: 3\ndrawn: Sun, North Node, East Point\nfloating:\n  selected: false\n"))
	if err != nil || !ok {
		t.Fatalf("parse: ok %v err %v", ok, err)
	}
	if want := []string{"Sun", "North Node", "East Point"}; !slices.Equal(got.Drawn, want) {
		t.Fatalf("Drawn = %q, want %q", got.Drawn, want)
	}
	if err := SaveSession(got); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(SessionPath())
	if !strings.Contains(string(b), "\ndrawn: Sun,North Node,East Point\n") {
		t.Errorf("the next save did not write the bare comma:\n%s", b)
	}
}

// A NAMED session is byte-strict: one saved with the old ", " join is refused
// as unknown content (spec rule 3, accepted by the operator: re-save it once).
func TestNamedSessionOldDrawnJoinIsRefused(t *testing.T) {
	sessionSandbox(t)
	l, err := SaveNamedSessionAs("old", Session{Drawn: []string{"Sun", "Moon"}, Slots: oneSlot(fixtureChart(), false)})
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	path := NamedSessionPath(l.File())
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "\ndrawn: Sun,Moon\n") {
		t.Fatalf("fixture: the named file lacks the bare-comma line:\n%s", b)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(b), "\ndrawn: Sun,Moon\n", "\ndrawn: Sun, Moon\n", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadNamedSession(l.File()); !errors.Is(err, ErrSessionUnknownContent) {
		t.Errorf("an old-join named session opened (err %v), want ErrSessionUnknownContent", err)
	}
}

// The joined value goes through yamlEscape whole: a name that needs quoting
// quotes the line, and the set reads back unchanged (independent review, round 1, finding 2).
func TestDrawnLineEscapesTheJoinedValue(t *testing.T) {
	sessionSandbox(t)
	want := []string{"Sun", "Body: One"}
	if err := SaveSession(Session{Drawn: want, Slots: oneSlot(fixtureChart(), false)}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(SessionPath())
	if line := "\ndrawn: " + strconv.Quote("Sun,Body: One") + "\n"; !strings.Contains(string(b), line) {
		t.Errorf("the file lacks %q:\n%s", line, b)
	}
	got, ok, err := LoadSession()
	if err != nil || !ok || !slices.Equal(got.Drawn, want) {
		t.Errorf("read back %q (ok %v err %v), want %q", got.Drawn, ok, err, want)
	}
}

// Opening an old-join named session the way the operator does refuses it,
// hands out no lock, and leaves the file byte for byte (independent review, round 1, finding 4).
func TestOpenNamedSessionOldDrawnJoinLeavesTheFile(t *testing.T) {
	sessionSandbox(t)
	l, err := SaveNamedSessionAs("old2", Session{Drawn: []string{"Sun", "Moon"}, Slots: oneSlot(fixtureChart(), false)})
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	path := NamedSessionPath(l.File())
	b, _ := os.ReadFile(path)
	old := []byte(strings.Replace(string(b), "\ndrawn: Sun,Moon\n", "\ndrawn: Sun, Moon\n", 1))
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, lock, err := OpenNamedSession(l.File(), FolderID(ChartsDir()))
	if !errors.Is(err, ErrSessionUnknownContent) || lock != nil {
		t.Errorf("open = lock %v, err %v; want no lock and ErrSessionUnknownContent", lock, err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(old) {
		t.Errorf("the refused named file changed:\n%s", after)
	}
}
