package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// `, c d` keeps the floating chart's house system and zodiac as the default
// frame, whichever slot the cursor is on: the write lands in preferences.yaml
// beside the city, the Log says what was kept, and a fresh start opens charts
// that name no system in it, the row tag showing it.
func TestSaveDefaultsKeepsTheFloatingFrame(t *testing.T) {
	sessionSandbox(t)
	path := store.PreferencesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# mine\nframe:\n  base: cursor\n  houses: Koch\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := framesFixture(t, 2, 0)
	m.place = amsterdamNL
	floating := m.slots.All()[len(m.slots.All())-1]
	// The floating chart is cast in Placidus and sidereal Lahiri; the chart
	// under the cursor, a different slot, in Koch: the floating one is kept.
	m.frames = m.frames.with(store.FrameRecord{
		Floating: true, Houses: recipe.Placidus, HasHouses: true, From: store.FrameFromChosen,
		Zodiac: recipe.Lahiri.RecordName(), HasZodiac: true,
	})
	m, _ = m.chooseHouseSystem(0, recipe.Koch)
	if got := m.rowTag(floating); got != "PlLa" {
		t.Fatalf("fixture: the floating tag is %q, want PlLa", got)
	}
	if got := m.rowTag(slotAt(m, 0)); got != "KoTr" {
		t.Fatalf("fixture: the cursor chart's tag is %q, want KoTr", got)
	}

	m, cmd := press(t, m, ",", "c", "d")
	if cmd == nil {
		t.Fatal("`,c d` scheduled no save")
	}
	m = deliver(m, cmd)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Only the two keys moved: the retired `base:` and the comment stand.
	want := "version: 1\n# mine\nframe:\n  base: cursor\n  houses: Placidus\n  zodiac: sidereal Lahiri\n" +
		"location:\n  name: Amsterdam\n  country: NL\n  lat: 52.37\n  lon: 4.89\n  tz: Europe/Amsterdam\n" +
		"house_display: angularity\n"
	if string(got) != want {
		t.Errorf("preferences.yaml:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	if n := noteCount(m, "default frame saved: Placidus, Lahiri"); n != 1 {
		t.Errorf("the Log names the kept frame %d times, want once: %v", n, m.logLines)
	}
	if want := "default city saved: Amsterdam, NL"; lastLine(m).text != want {
		t.Errorf("last Log line = %q, want %q", lastLine(m).text, want)
	}
	if !strings.Contains(ansi.Strip(m.View()), "Defaults saved") {
		t.Errorf("the footer does not show the save notice:\n%s", ansi.Strip(m.View()))
	}

	// A fresh start from that profile: charts that name no system take it.
	fresh := framesFixture(t, 2, 0).applyFrameConfig(realPerimeter(t).Frame)
	for i := range fresh.slots.All() {
		if tag := fresh.rowTag(slotAt(fresh, i)); tag != "PlLa" {
			t.Errorf("after a restart row %d is tagged %q, want PlLa", i, tag)
		}
	}
	if n := noteCount(fresh, "preferences:"); n != 0 {
		t.Errorf("a good frame logged %d notes at start, want silence: %v", n, fresh.logLines)
	}
}

// A frame that could not be written is one error line, and the city, which was
// kept, is still said. The failure is a real one: the preferences file holds
// `frame: <value>`, which is no block to write keys into, so the store refuses
// and leaves that line alone while the city and the house display are saved.
func TestSaveDefaultsFrameFailureIsOneErrorLine(t *testing.T) {
	sessionSandbox(t)
	path := store.PreferencesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("frame: retired-default\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := framesFixture(t, 2, 0)
	m.place = amsterdamNL
	m, cmd := press(t, m, ",", "c", "d")
	if cmd == nil {
		t.Fatal("`,c d` scheduled no save")
	}
	m = deliver(m, cmd)

	want := "default frame not saved: store: preferences.yaml has 'frame: retired-default', not a block; the frame was not saved"
	if lastLine(m).text != want || lastLine(m).lane != laneError {
		t.Errorf("last Log line = %q (lane %d), want the error line %q", lastLine(m).text, lastLine(m).lane, want)
	}
	errors := 0
	for _, l := range m.logLines {
		if l.lane == laneError {
			errors++
		}
	}
	if errors != 1 {
		t.Errorf("the Log holds %d error lines, want one: %v", errors, m.logLines)
	}
	if noteCount(m, "default city saved: Amsterdam, NL") != 1 {
		t.Errorf("the saved city is not in the Log: %v", m.logLines)
	}
	if noteCount(m, "default frame saved") != 0 {
		t.Errorf("the Log claims the frame was kept: %v", m.logLines)
	}
	if strings.Contains(ansi.Strip(m.View()), "Defaults saved") {
		t.Error("the footer claims the defaults were saved")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), "version: 1\nframe: retired-default\n") || !strings.Contains(string(got), "location:\n  name: Amsterdam\n") {
		t.Errorf("the city was not kept beside the untouched frame line:\n%s", got)
	}
}

// Every house system the store will write, with tropical and every sidereal
// zodiac, comes back through the store and the UI's own resolution as the
// system and zodiac that were written, names with spaces and punctuation
// ("Equal (MC)", "Carter P.Equat.", "J.N. Bhasin") included.
func TestSavedFrameResolvesToWhatWasWritten(t *testing.T) {
	sessionSandbox(t)
	base := framesFixture(t, 1, 0)
	for h := 0; h < recipe.HouseSystemCount; h++ {
		hs := recipe.HouseSystem(h)
		for z := 0; z < recipe.ZodiacCount; z++ {
			zod := recipe.Zodiac(z)
			if err := store.SetFrame(hs.StoredName(), zod.RecordName()); err != nil {
				t.Fatalf("SetFrame(%q, %q): %v", hs.StoredName(), zod.RecordName(), err)
			}
			m := base.applyFrameConfig(store.Frame())
			if got := m.ownHouses(nil).hs; got != hs {
				t.Fatalf("wrote houses %q, the UI resolved %q", hs.StoredName(), got.StoredName())
			}
			if got := m.ownZodiac(nil); got != zod {
				t.Fatalf("wrote zodiac %q, the UI resolved %q", zod.RecordName(), got.RecordName())
			}
			if n := noteCount(m, "preferences:"); n != 0 {
				t.Fatalf("%q, %q: the UI complained about what the store wrote: %v", hs.StoredName(), zod.RecordName(), m.logLines)
			}
		}
	}
}
