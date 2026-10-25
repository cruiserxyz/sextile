package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/store"
)

// startedFromPrefs is a fresh Model that has read the profile's preferences the
// way the program does at start: the startup perimeter, through Update.
func startedFromPrefs(t *testing.T) Model {
	t.Helper()
	msg, ok := ingest.ReadStartupPerimeter()().(ingest.StartupPerimeterReadMsg)
	if !ok {
		t.Fatal("fixture: the perimeter read returned another message")
	}
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTime()))
	m.width, m.height = 132, 40
	tm, _ := m.Update(msg)
	return tm.(Model)
}

func noteCount(m Model, contains string) int {
	n := 0
	for _, l := range m.logLines {
		if l.lane == laneNote && strings.Contains(l.text, contains) {
			n++
		}
	}
	return n
}

// H4: `, c d` keeps the house display with the city. With a non-default form
// live it writes the `house_display:` line and the city, every other line of
// preferences.yaml (comments, a block store has no parser for) survives byte
// for byte, a restart reads the form back, and an unknown word falls back to
// angularity with one Log note.
func TestSaveDefaultsKeepsTheHouseDisplay(t *testing.T) {
	// The fixture: a comment, a version, a clock block, a block no parser here
	// knows, an indented comment, the old house display line with a trailing
	// comment of its own, and a last block with NO final newline.
	head := "# my preferences\n" +
		"version: 1\n" +
		"clock:\n  mode: held\n" +
		"\n" +
		"unknown_block:\n  deep:\n    - a\n    - b\n  # indented, belongs to nobody\n"
	tail := "theme:\n  accent: teal" // unterminated
	before := head + "house_display: angularity  # the old default\n" + tail
	locationBlock := "location:\n  name: Amsterdam\n  country: NL\n  lat: 52.37\n  lon: 4.89\n  tz: Europe/Amsterdam\n"
	// The floating chart is in Whole Sign and tropical here, and `, c d` keeps
	// the frame beside the city and the form.
	frameBlock := "frame:\n  houses: Whole\n  zodiac: tropical\n"
	if strings.HasSuffix(before, "\n") {
		t.Fatal("fixture: the file ends in a newline; it must not")
	}

	t.Run("a non-default form is written with the city and nothing else moves", func(t *testing.T) {
		m, _ := guessedStart(t)
		path := store.PreferencesPath()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
			t.Fatal(err)
		}
		m.houseDisplay = houseMarked

		m, cmd := press(t, m, ",", "c", "d")
		if cmd == nil {
			t.Fatal("`,c d` scheduled no save")
		}
		m = deliver(m, cmd)

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		// Every original byte survives but the one replaced line; what is new is
		// the separating newline the unterminated last line needed, the city's
		// block, the frame's, and nothing else.
		rest, ok := strings.CutPrefix(string(got), head+"house_display: marked\n")
		if !ok {
			t.Fatalf("the bytes before the replaced line moved:\n%s", got)
		}
		rest, ok = strings.CutPrefix(rest, tail)
		if !ok {
			t.Fatalf("the unterminated last block did not survive byte for byte:\n%s", got)
		}
		if rest != "\n"+locationBlock+frameBlock {
			t.Errorf("after the last original byte the file holds %q, want a separating newline, the city block and the frame block %q", rest, "\n"+locationBlock+frameBlock)
		}
		if h := store.HouseDisplay(); h.Value != "marked" || !h.Present {
			t.Errorf("stored house display = %+v, want marked", h)
		}
		if l, ok := store.Location(); !ok || l.Name != "Amsterdam" {
			t.Errorf("stored location = %+v (%v), want Amsterdam", l, ok)
		}
		if n := noteCount(m, "default house display saved: Number + angularity"); n != 1 {
			t.Errorf("the Log names the kept house display %d times, want once: %v", n, m.logLines)
		}
		if want := "default city saved: Amsterdam, NL"; lastLine(m).text != want {
			t.Errorf("last Log line = %q, want %q", lastLine(m).text, want)
		}
		if !strings.Contains(ansi.Strip(m.View()), "Defaults saved") {
			t.Errorf("the footer does not show the save notice:\n%s", ansi.Strip(m.View()))
		}

		// A restart reads it back.
		restarted := startedFromPrefs(t)
		if restarted.houseDisplay != houseMarked {
			t.Errorf("after a restart the form is %v, want marked", restarted.houseDisplay)
		}
		if n := noteCount(restarted, "house_display"); n != 0 {
			t.Errorf("a good value logged %d notes, want silence", n)
		}
	})

	t.Run("an unterminated file with no house line gets a newline and the new lines", func(t *testing.T) {
		m, _ := guessedStart(t)
		path := store.PreferencesPath()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		original := "# prefs\nversion: 1\ntheme:\n  accent: teal"
		if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
			t.Fatal(err)
		}
		m.houseDisplay = houseNumber
		m, cmd := press(t, m, ",", "c", "d")
		m = deliver(m, cmd)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rest, ok := strings.CutPrefix(string(got), original)
		if !ok {
			t.Fatalf("an original byte moved:\n%s", got)
		}
		if want := "\n" + locationBlock + "house_display: number\n" + frameBlock; rest != want {
			t.Errorf("after the last original byte the file holds %q, want %q", rest, want)
		}
	})

	t.Run("a file from a newer generation sets no form at start", func(t *testing.T) {
		sessionSandbox(t)
		path := store.PreferencesPath()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("version: 99\nhouse_display: marked\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		m := startedFromPrefs(t)
		if m.houseDisplay != houseAngularity {
			t.Errorf("a version 99 file set the form to %v", m.houseDisplay)
		}
	})

	t.Run("each form comes back", func(t *testing.T) {
		for _, info := range houseForms {
			m, _ := guessedStart(t)
			m.houseDisplay = info.form
			m, cmd := press(t, m, ",", "c", "d")
			m = deliver(m, cmd)
			if got := startedFromPrefs(t).houseDisplay; got != info.form {
				t.Errorf("%s: after a restart the form is %v", info.value, got)
			}
		}
	})

	t.Run("the default is written too, over an older choice", func(t *testing.T) {
		m, _ := guessedStart(t)
		path := store.PreferencesPath()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("house_display: quadrants\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		m, cmd := press(t, m, ",", "c", "d")
		m = deliver(m, cmd)
		if h := store.HouseDisplay(); h.Value != "angularity" {
			t.Errorf("stored house display = %+v, want angularity written over quadrants", h)
		}
	})

	t.Run("an unknown word falls back to angularity with one Log note", func(t *testing.T) {
		sessionSandbox(t)
		path := store.PreferencesPath()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("house_display: spiral\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		m := startedFromPrefs(t)
		if m.houseDisplay != houseAngularity {
			t.Errorf("an unknown word left the form at %v, want angularity", m.houseDisplay)
		}
		if n := noteCount(m, `house_display "spiral" ignored`); n != 1 {
			t.Errorf("the Log refuses the word %d times, want once: %v", n, m.logLines)
		}
		for _, w := range []string{"angularity", "quadrants", "number", "marked"} {
			if noteCount(m, w) == 0 {
				t.Errorf("the refusal does not name %q", w)
			}
		}
	})

	t.Run("a key that is absent says nothing", func(t *testing.T) {
		sessionSandbox(t)
		m := startedFromPrefs(t)
		if m.houseDisplay != houseAngularity || noteCount(m, "house_display") != 0 {
			t.Errorf("no key: form %v, %d notes", m.houseDisplay, noteCount(m, "house_display"))
		}
	})

	t.Run("a house display that could not be written is said, the city still is", func(t *testing.T) {
		m, _ := guessedStart(t)
		m, _ = applyDefaultCitySaved(m, DefaultCitySavedMsg{Place: amsterdamNL, House: houseNumber, HouseErr: errors.New("disk full\nsecond line")})
		if want := "default house display not saved: disk full"; lastLine(m).text != want || lastLine(m).lane != laneError {
			t.Errorf("last Log line = %q (lane %d), want the error line %q", lastLine(m).text, lastLine(m).lane, want)
		}
		if noteCount(m, "default city saved: Amsterdam, NL") != 1 {
			t.Errorf("the saved city is not in the Log: %v", m.logLines)
		}
		if strings.Contains(ansi.Strip(m.View()), "Defaults saved") {
			t.Error("the footer claims the defaults were saved")
		}
	})
}
