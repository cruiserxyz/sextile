package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The experimental: block (specs/experiments-block.md in the control
// repository, version 2). Every expected file below is written out by hand:
// an expectation built by the code under test could not see it go wrong.

// seedAged writes body and gives it a fixed past modification time, so a write
// that replaces the file with the same bytes still shows.
func seedAged(t *testing.T, path, body string) time.Time {
	t.Helper()
	seedPreferences(t, path, body)
	old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	return old
}

// countRenames counts the atomic replaces from here to the end of the test.
func countRenames(t *testing.T) *int {
	t.Helper()
	n := new(int)
	prev := renameFile
	renameFile = func(from, to string) error {
		*n++
		return prev(from, to)
	}
	t.Cleanup(func() { renameFile = prev })
	return n
}

// requireUntouched fails unless path holds body with the modification time
// seedAged gave it, and no rename happened since renames was taken.
func requireUntouched(t *testing.T, path, body string, mtime time.Time, renames *int) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := readPreferences(t, path); got != body {
		t.Errorf("file changed:%s", diffText(got, body))
	}
	if !fi.ModTime().Equal(mtime) {
		t.Errorf("file rewritten: mtime %v, want %v", fi.ModTime(), mtime)
	}
	if *renames != 0 {
		t.Errorf("%d rename(s), want none", *renames)
	}
}

// lockDir makes dir read-only, so that a write which creates a temporary file
// or a folder inside it fails, and returns the undo (also run at cleanup). As
// root modes restrain nothing, so the lock is skipped and the other checks
// stand alone.
func lockDir(t *testing.T, dir string) func() {
	t.Helper()
	if os.Geteuid() == 0 {
		return func() {}
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	locked = append(locked, dir)
	undo := func() { _ = os.Chmod(dir, 0o755) }
	t.Cleanup(undo)
	return undo
}

// locked lists the folders lockDir locked, for unlockAll.
var locked []string

func unlockAll(t *testing.T) {
	t.Helper()
	for _, d := range locked {
		_ = os.Chmod(d, 0o755)
	}
	locked = nil
}

// requireReads fails unless Experimental() reads id as want.
func requireReads(t *testing.T, id string, want bool) {
	t.Helper()
	if got := Experimental().On[id]; got != want {
		t.Errorf("read back %s = %v, want %v", id, got, want)
	}
}

func readExperimentalText(t *testing.T, text string) ExperimentalConfig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "preferences.yaml")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return readExperimental(path)
}

func laterGeneration() string { return fmt.Sprintf("version: %d\n", SchemaVersion+1) }

// T1 (R2).
func TestExperimentalReadsTheFirstBlock(t *testing.T) {
	for _, v := range []string{"on", "true", "yes", "ON", "Yes", `"on"`} {
		if got := readExperimentalText(t, "experimental:\n  enter-colour: "+v+"\n"); !got.On["enter-colour"] {
			t.Errorf("value %q: read off, want on", v)
		}
	}
	for _, v := range []string{"off", "maybe", "", "false", "no", "1"} {
		got := readExperimentalText(t, "experimental:\n  enter-colour: "+v+"\n")
		if on, present := got.On["enter-colour"]; !present || on {
			t.Errorf("value %q: On = %v, want enter-colour present and off", v, got.On)
		}
	}
	// The whole map, false values included: an off key must be present, and a
	// key that is not a direct child must be absent (fmt prints maps sorted).
	for name, c := range map[string]struct{ text, want string }{
		"nested key is not a switch": {"experimental:\n  group:\n    enter-colour: on\n", "map[group:false]"},
		"first of two keys wins":     {"experimental:\n  enter-colour: off\n  enter-colour: on\n", "map[enter-colour:false]"},
		"second block ignored":       {"experimental:\n  a: on\nother: 1\nexperimental:\n  b: on\n", "map[a:true]"},
		"comment after the colon":    {"experimental:  # mine\n  a: on\n", "map[a:true]"},
		"inline comment":             {"experimental:\n  a: on   # trying\n", "map[a:true]"},
		"document marker ends it":    {"experimental:\n  a: on\n---\n  b: on\n", "map[a:true]"},
		"unknown id is returned":     {"experimental:\n  future-switch: yes\n", "map[future-switch:true]"},
		"crlf":                       {"experimental:\r\n  a: on\r\n  b: off\r\n", "map[a:true b:false]"},
	} {
		if got := fmt.Sprint(readExperimentalText(t, c.text).On); got != c.want {
			t.Errorf("%s: On = %s, want %s", name, got, c.want)
		}
	}
	empty := map[string]ExperimentalConfig{
		"scalar":           readExperimentalText(t, "experimental: on\n"),
		"malformed":        readExperimentalText(t, "experimental:\ngarbage\n  a: on\n"),
		"absent":           readExperimental(filepath.Join(t.TempDir(), "none.yaml")),
		"later generation": readExperimentalText(t, laterGeneration()+"experimental:\n  a: on\n"),
	}
	if os.Geteuid() != 0 {
		path := filepath.Join(t.TempDir(), "preferences.yaml")
		if err := os.WriteFile(path, []byte("experimental:\n  a: on\n"), 0o000); err != nil {
			t.Fatal(err)
		}
		empty["unreadable"] = readExperimental(path)
	}
	for name, got := range empty {
		if got.On == nil || len(got.On) != 0 {
			t.Errorf("%s: On = %#v, want an empty, non-nil map", name, got.On)
		}
	}
}

// T2 (R5).
func TestSetExperimentalCreatesTheFile(t *testing.T) {
	path := prefPath(t)
	if err := SetExperimental("enter-colour", true); err != nil {
		t.Fatal(err)
	}
	if got, want := readPreferences(t, path), "experimental:\n  enter-colour: on\n"; got != want {
		t.Errorf("new file:%s", diffText(got, want))
	}
	requireReads(t, "enter-colour", true)
}

// T3 (R4, R8).
func TestSetExperimentalChangesOnlyTheValue(t *testing.T) {
	path := prefPath(t)
	before := "version: 1\r\n" +
		"frame:\r\n" +
		"  houses: Koch\r\n" +
		"# my switches\r\n" +
		"experimental:   # hand-made\r\n" +
		"  # the colour one\r\n" +
		"  enter-colour :\toff   # trying it\r\n" +
		"  enter-colour: off\r\n" +
		"  some-old-thing: on\r\n" +
		"  group:\r\n" +
		"    enter-colour: off\r\n" +
		"\r\n" +
		"clock:\r\n" +
		"  hours: 24\r\n" +
		"experimental:\r\n" +
		"  enter-colour: off"
	on := strings.Replace(before, "  enter-colour :\toff   # trying it", "  enter-colour :\ton   # trying it", 1)
	seedPreferences(t, path, before)
	if err := SetExperimental("enter-colour", true); err != nil {
		t.Fatal(err)
	}
	if got := readPreferences(t, path); got != on {
		t.Fatalf("on:%s", diffText(got, on))
	}
	requireReads(t, "enter-colour", true)
	if err := SetExperimental("enter-colour", false); err != nil {
		t.Fatal(err)
	}
	if got := readPreferences(t, path); got != before {
		t.Errorf("back to off:%s", diffText(got, before))
	}
	requireReads(t, "enter-colour", false)

	for _, c := range []struct{ seed, want string }{
		{"experimental:\n  enter-colour: off", "experimental:\n  enter-colour: on"},
		{"experimental:\n  enter-colour:\n", "experimental:\n  enter-colour: on\n"},
		{"experimental:\n  enter-colour:  # c\n", "experimental:\n  enter-colour: on  # c\n"},
		{"experimental:\n  enter-colour: \"a\\\"b\" # keep this note\n", "experimental:\n  enter-colour: on # keep this note\n"},
		{"experimental:\n  enter-colour: 'it''s' # n\n", "experimental:\n  enter-colour: on # n\n"},
		{"experimental:\n  enter-colour: \"open # n\n", "experimental:\n  enter-colour: on # n\n"},
		{"experimental:\n  enter-colour:#note\n", "experimental:\n  enter-colour: on\n"},
		{"experimental:\n  enter-colour:off # n\n", "experimental:\n  enter-colour: on # n\n"},
	} {
		seedPreferences(t, path, c.seed)
		if err := SetExperimental("enter-colour", true); err != nil {
			t.Fatal(err)
		}
		if got := readPreferences(t, path); got != c.want {
			t.Errorf("seed %q:%s", c.seed, diffText(got, c.want))
		}
		requireReads(t, "enter-colour", true)
	}
}

// T4 (R5).
func TestSetExperimentalAppendsAtTheBlocksIndent(t *testing.T) {
	path := prefPath(t)
	for name, c := range map[string]struct{ seed, want string }{
		"four-space block": {
			"experimental:\n    some-old-thing: off\n\n\nclock:\n  hours: 24\n",
			"experimental:\n    some-old-thing: off\n    enter-colour: on\n\n\nclock:\n  hours: 24\n",
		},
		"column-zero comment before the next block": {
			"experimental:\n  a: off\n\n# frame settings\nframe:\n  houses: Koch\n",
			"experimental:\n  a: off\n  enter-colour: on\n\n# frame settings\nframe:\n  houses: Koch\n",
		},
		"comment-only block": {
			"experimental: # empty\nclock:\n  hours: 24\n",
			"experimental: # empty\n  enter-colour: on\nclock:\n  hours: 24\n",
		},
		"bare block at the end": {
			"experimental:\n",
			"experimental:\n  enter-colour: on\n",
		},
		"no block, no final newline": {
			"clock:\n  hours: 12",
			"clock:\n  hours: 12\nexperimental:\n  enter-colour: on\n",
		},
	} {
		seedPreferences(t, path, c.seed)
		if err := SetExperimental("enter-colour", true); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := readPreferences(t, path); got != c.want {
			t.Errorf("%s:%s", name, diffText(got, c.want))
		}
		requireReads(t, "enter-colour", true)
	}
}

// T5 (R7).
func TestSetExperimentalRefusesAScalar(t *testing.T) {
	path := prefPath(t)
	body := "experimental: on\nclock:\n  hours: 24\n"
	const want = "store: preferences.yaml has 'experimental: on', not a block. The switch was not saved"
	for _, on := range []bool{true, false} {
		mtime := seedAged(t, path, body)
		renames := countRenames(t)
		err := SetExperimental("enter-colour", on)
		if err == nil || err.Error() != want {
			t.Errorf("on=%v: err = %v, want %q", on, err, want)
		}
		requireUntouched(t, path, body, mtime, renames)
	}
}

// T6 (R3).
func TestSetExperimentalRefusesABadId(t *testing.T) {
	path := prefPath(t)
	for _, id := range []string{"", "Enter", "a b", "x:y", "#x", "-x", "x\ny", "9x", "enter_colour"} {
		for _, on := range []bool{true, false} {
			if err := SetExperimental(id, on); err == nil {
				t.Errorf("id %q accepted (on=%v)", id, on)
			}
		}
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a refused id made the file: %v", err)
	}
	body := "experimental:\n  a: off\n"
	mtime := seedAged(t, path, body)
	renames := countRenames(t)
	for _, on := range []bool{true, false} {
		if err := SetExperimental("Bad id", on); err == nil {
			t.Errorf("a bad id was accepted against an existing file (on=%v)", on)
		}
	}
	requireUntouched(t, path, body, mtime, renames)
	if err := SetExperimental("a1-b", true); err != nil {
		t.Fatalf("a1-b: %v", err)
	}
	requireReads(t, "a1-b", true)
}

// T7 (R5, R6).
func TestSetExperimentalOffWhenAbsentWritesNothing(t *testing.T) {
	path := prefPath(t)
	renames := countRenames(t)
	// The config folder read-only: a write that made a folder or a temporary
	// file on the way would fail here.
	lockDir(t, filepath.Dir(filepath.Dir(path)))
	if err := SetExperimental("enter-colour", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("off made the file: %v", err)
	}
	if *renames != 0 {
		t.Errorf("%d rename(s) for a missing file", *renames)
	}
	unlockAll(t)
	for _, body := range []string{
		"clock:\n  hours: 24\n",
		"experimental:\n  other: on\n",
	} {
		mtime := seedAged(t, path, body)
		*renames = 0
		unlock := lockDir(t, filepath.Dir(path))
		if err := SetExperimental("enter-colour", false); err != nil {
			t.Fatal(err)
		}
		unlock()
		requireUntouched(t, path, body, mtime, renames)
	}
}

// T8 (R6).
func TestSetExperimentalSameValueWritesNothing(t *testing.T) {
	path := prefPath(t)
	for _, c := range []struct {
		body string
		on   bool
	}{
		{"experimental:\n  enter-colour: on\n", true},
		{"experimental:\n  enter-colour: off  # no\n", false},
	} {
		mtime := seedAged(t, path, c.body)
		renames := countRenames(t)
		unlock := lockDir(t, filepath.Dir(path))
		if err := SetExperimental("enter-colour", c.on); err != nil {
			t.Fatal(err)
		}
		unlock()
		requireUntouched(t, path, c.body, mtime, renames)
	}
}

// T9 (R7).
func TestSetExperimentalLaterGeneration(t *testing.T) {
	path := prefPath(t)
	body := laterGeneration() + "experimental:\n  enter-colour: off\n"
	mtime := seedAged(t, path, body)
	renames := countRenames(t)
	for _, on := range []bool{true, false} {
		err := SetExperimental("enter-colour", on)
		if err == nil || !strings.Contains(err.Error(), "newer Sextile") {
			t.Errorf("on=%v: err = %v, want the newer generation refused", on, err)
		}
	}
	requireUntouched(t, path, body, mtime, renames)
}

// T10 (R2-R5).
func TestSetExperimentalRoundTrips(t *testing.T) {
	path := prefPath(t)
	for _, seed := range []string{
		"",
		"experimental:\n    some-old-thing: off\n\n\nclock:\n  hours: 24\n",
		"experimental:\r\n  enter-colour: off   # trying it\r\n",
		"clock:\n  hours: 12",
	} {
		if seed == "" {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
		} else {
			seedPreferences(t, path, seed)
		}
		for _, on := range []bool{true, false, true} {
			if err := SetExperimental("enter-colour", on); err != nil {
				t.Fatal(err)
			}
			if got := Experimental().On["enter-colour"]; got != on {
				t.Errorf("seed %q: wrote %v, read %v", seed, on, got)
			}
		}
	}
}

// T11 (R9).
func TestSetExperimentalKeepsAConcurrentEdit(t *testing.T) {
	path := prefPath(t)
	seedPreferences(t, path, "experimental:\n  enter-colour: off\n")
	prev := betweenReadAndRename
	once := false
	betweenReadAndRename = func() {
		if once {
			return
		}
		once = true
		cur := readPreferences(t, path)
		if err := os.WriteFile(path, []byte("clock:\n  hours: 12\n"+cur), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { betweenReadAndRename = prev })
	if err := SetExperimental("enter-colour", true); err != nil {
		t.Fatal(err)
	}
	if got, want := readPreferences(t, path), "clock:\n  hours: 12\nexperimental:\n  enter-colour: on\n"; got != want {
		t.Errorf("concurrent edit:%s", diffText(got, want))
	}
}

// T12 (R7).
func TestSetExperimentalRefusesAMalformedBlock(t *testing.T) {
	path := prefPath(t)
	for _, c := range []struct{ body, quoted string }{
		{"experimental:\ngarbage\n  a: on\n", "garbage"},
		{"experimental:\n- enter-colour: off\nclock:\n  hours: 24\n", "- enter-colour: off"},
	} {
		body := c.body
		mtime := seedAged(t, path, body)
		renames := countRenames(t)
		for _, on := range []bool{true, false} {
			err := SetExperimental("enter-colour", on)
			if err == nil || !strings.Contains(err.Error(), c.quoted) {
				t.Errorf("%q on=%v: err = %v, want the line quoted", body, on, err)
			}
		}
		requireUntouched(t, path, body, mtime, renames)
	}
}

// T13 (R6): the short circuit is the one door's, so every preferences writer
// leaves an unchanged file alone, in the folder's read-only state as well.
func TestUnchangedPreferencesWriteNothing(t *testing.T) {
	for name, write := range map[string]func() error{
		"location":      func() error { return SetLocation(berlin) },
		"frame":         func() error { return SetFrame("Placidus", "sidereal Lahiri") },
		"house display": func() error { return SetHouseDisplay("number") },
		"experimental":  func() error { return SetExperimental("enter-colour", true) },
	} {
		path := prefPath(t)
		if err := write(); err != nil {
			t.Fatalf("%s: first write: %v", name, err)
		}
		body := readPreferences(t, path)
		old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
		renames := countRenames(t)
		unlock := lockDir(t, filepath.Dir(path))
		if err := write(); err != nil {
			t.Errorf("%s: unchanged write: %v", name, err)
		}
		unlock()
		requireUntouched(t, path, body, old, renames)
	}
}
