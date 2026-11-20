// All chart values in this file are constructed synthetic test inputs.
package lookup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

func amsterdamPlace() location.PlaceInfo {
	return location.PlaceInfo{
		Name: "Amsterdam", Country: "NL",
		Lat: 52.3667, Lon: 4.9, TZ: "Europe/Amsterdam",
	}
}

func amsterdamMoment() time.Time {
	return time.Date(1987, 3, 14, 16, 30, 0, 0, time.FixedZone("CEST", 2*3600))
}

// C6: a slug the library already owns is refused, nothing is written, the file
// found is left byte-identical, and the refusal names it.
//
// Two shapes a chart can own a slug in, each its own subtest, because they
// are found by different routes and a fix for one does not imply the other.
func TestSaveRefusesASlugTheLibraryOwns(t *testing.T) {
	const slug = "test-person"
	for _, tc := range []struct {
		name     string
		file     string
		contents string
	}{
		{"a dat chart of that name", slug + ".as", "@AI800  ; someone else's chart\n"},
		// A .as's id is the slug of its stem (C3.20), so a stem spelled
		// differently that slugifies to the same id owns it too.
		{"a dat whose stem slugifies to that slug", "Test Person.as", "@AI800  ; someone else's chart\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("SEXTILE_CHARTS_DIR", dir)
			path := filepath.Join(dir, tc.file)
			if err := os.WriteFile(path, []byte(tc.contents), 0o600); err != nil {
				t.Fatalf("seed: %v", err)
			}

			_, err := SaveFloatingChart(context.Background(), dir, "Test Person", amsterdamMoment(), amsterdamPlace())
			var taken *ErrSlugTaken
			if !errors.As(err, &taken) {
				t.Fatalf("err = %v, want ErrSlugTaken", err)
			}
			if taken.File != tc.file {
				t.Errorf("refusal names %q, want the file it found, %q", taken.File, tc.file)
			}

			// Nothing written: the occupying file is untouched and it is still
			// the only thing in the directory.
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read back: %v", err)
			}
			if string(got) != tc.contents {
				t.Errorf("the existing chart was modified.\n got: %q\nwant: %q", got, tc.contents)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("read dir: %v", err)
			}
			if len(entries) != 1 {
				var names []string
				for _, e := range entries {
					names = append(names, e.Name())
				}
				t.Errorf("the refused save left the directory holding %q, want only %q", names, tc.file)
			}
		})
	}
}

// A .yaml is no longer a chart, so a stray one of the same stem owns no slug:
// the save goes through and the .yaml is left exactly as it was.
func TestSaveIgnoresAStrayYAMLOfTheSameName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SEXTILE_CHARTS_DIR", dir)
	const contents = "name: Someone Else\n"
	path := filepath.Join(dir, "test-person.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := SaveFloatingChart(context.Background(), dir, "Test Person", amsterdamMoment(), amsterdamPlace()); err != nil {
		t.Fatalf("a stray .yaml must not block the save, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "test-person.as")); err != nil {
		t.Errorf("the save wrote no test-person.as: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != contents {
		t.Errorf("the stray .yaml was modified.\n got: %q\nwant: %q", got, contents)
	}
}

// C8: after a successful save the library lists exactly one row for that slug,
// under the name astrolog recorded, and anchoring it resolves to the saved
// moment.
//
// This is the row that proves the save closed the loop rather than merely
// writing a file: the chart is in the library the picker reads, and picking it
// gives back the instant it was saved from.
func TestSavedChartListsAndAnchors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SEXTILE_CHARTS_DIR", dir)
	when := amsterdamMoment()

	saved, err := SaveFloatingChart(context.Background(), dir, "Test Person", when, amsterdamPlace())
	if err != nil {
		t.Fatalf("SaveFloatingChart: %v", err)
	}
	if saved.ID != "test-person" {
		t.Errorf("saved.ID = %q, want %q", saved.ID, "test-person")
	}

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatalf("LoadChartLibrary: %v", err)
	}
	var rows int
	var row = struct {
		name string
		file string
	}{}
	for _, c := range lib.Charts {
		if c.ID == saved.ID {
			rows++
			row.name, row.file = c.Name, c.Source.File
		}
	}
	if rows != 1 {
		t.Fatalf("the library lists %d rows for %q, want exactly 1 (drops: %+v)", rows, saved.ID, lib.Drops)
	}
	if row.name != saved.Name {
		t.Errorf("the row is named %q, want the name astrolog recorded, %q", row.name, saved.Name)
	}

	// ...and anchoring it gives back the moment it was saved from.
	var picked = -1
	for i, c := range lib.Charts {
		if c.ID == saved.ID {
			picked = i
		}
	}
	got, err := ResolvePicked(context.Background(), lib.Charts[picked])
	if err != nil {
		t.Fatalf("ResolvePicked: %v", err)
	}
	if !got.UTC().Truncate(time.Minute).Equal(when.UTC().Truncate(time.Minute)) {
		t.Errorf("anchoring the saved chart resolves to %s, want the saved moment %s",
			got.UTC().Format(time.RFC3339), when.UTC().Format(time.RFC3339))
	}
}

// A chart saved far from any atlas city lists like any other: the list reads
// the file, never the atlas.
func TestSavedChartFarFromAnyCityLists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SEXTILE_CHARTS_DIR", dir)

	// The South Pacific, far from any atlas city.
	middleOfNowhere := location.PlaceInfo{
		Name: "Nowhere", Country: "", Lat: -30.0, Lon: -140.0, TZ: "Etc/GMT+9",
	}
	saved, err := SaveFloatingChart(context.Background(), dir, "Test Person",
		time.Date(1987, 3, 14, 16, 30, 0, 0, time.FixedZone("X", -9*3600)), middleOfNowhere)
	if err != nil {
		t.Fatalf("SaveFloatingChart: %v", err)
	}
	// The file IS there...
	if _, err := os.Stat(filepath.Join(dir, saved.File)); err != nil {
		t.Errorf("the chart was reported saved but is not on disk: %v", err)
	}
	// A chart in a city, saved into the same directory, so the list below is
	// known to work.
	placed, err := SaveFloatingChart(context.Background(), dir, "Placeable Control", amsterdamMoment(), amsterdamPlace())
	if err != nil {
		t.Fatalf("SaveFloatingChart (the placeable control): %v", err)
	}

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatalf("LoadChartLibrary: %v", err)
	}
	var sawPlaced, sawFar bool
	for _, c := range lib.Charts {
		switch c.ID {
		case placed.ID:
			sawPlaced = true
		case saved.ID:
			sawFar = true
		}
	}
	if !sawPlaced {
		t.Fatalf("the library lists neither chart, so it is not working; this test can prove nothing (drops: %+v)", lib.Drops)
	}
	if !sawFar {
		t.Fatalf("the library does not list %q (drops: %+v)", saved.ID, lib.Drops)
	}
}

// An empty slug is refused before anything is written.
func TestSaveRefusesAnEmptySlug(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SEXTILE_CHARTS_DIR", dir)

	_, err := SaveFloatingChart(context.Background(), dir, "!!! ???", amsterdamMoment(), amsterdamPlace())
	if !errors.Is(err, ErrEmptySlug) {
		t.Fatalf("err = %v, want ErrEmptySlug", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("a refused save wrote %d file(s)", len(entries))
	}
}

// The charts directory is resolved ONCE per save, by the caller, and both the
// collision scan and the write use that one value rather than each asking
// store.ChartsDir() again.
//
// store.ChartsDir()'s answer changes the moment the XDG default directory comes
// into existence, so a save that asked it twice could check one directory, show
// the operator a second and write into a third.
//
// The divergence itself needs SEXTILE_DEV_CHARTS_DIR, which a test outside
// internal/store's own test file does not set (a dev-tier library is shadowed
// by a real one, which is how a test once reached an operator's real charts).
// So the property is pinned the other way
// round, on the top tier: ChartsDir() is made to answer somewhere ELSE, and
// the save must still write where it was told.
func TestSaveWritesWhereItWasToldNotWhereChartsDirPoints(t *testing.T) {
	told := t.TempDir()
	elsewhere := t.TempDir()
	t.Setenv("SEXTILE_CHARTS_DIR", elsewhere)

	if got := store.ChartsDir(); got != elsewhere {
		t.Fatalf("precondition: ChartsDir() = %q, want %q", got, elsewhere)
	}
	if told == elsewhere {
		t.Fatal("precondition: the two directories are the same, so this proves nothing")
	}

	saved, err := SaveFloatingChart(context.Background(), told, "Test Person", amsterdamMoment(), amsterdamPlace())
	if err != nil {
		t.Fatalf("SaveFloatingChart: %v", err)
	}
	if _, err := os.Stat(filepath.Join(told, saved.File)); err != nil {
		t.Errorf("the chart did not land in the directory the save was given: %v", err)
	}
	if _, err := os.Stat(filepath.Join(elsewhere, saved.File)); err == nil {
		t.Errorf("the chart landed in %s, which is where ChartsDir() points and not where the save was told", elsewhere)
	}
	// The collision scan used the same directory: a chart already in `told`
	// must be found, while one in `elsewhere` must not shadow it.
	if _, err := SaveFloatingChart(context.Background(), told, "Test Person", amsterdamMoment(), amsterdamPlace()); err == nil {
		t.Error("the second save was not refused, so the collision scan read a different directory than the write")
	}
}

// A moment that arrives in UT (a search's target, such as the next phase) is
// saved in the place's own clock, to the second it was shown at (bug
// 20261113T1918Z): the file and the floating row name the same second.
func TestSaveWritesThePlacesClockToTheSecond(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SEXTILE_CHARTS_DIR", dir)
	phase := time.Date(2026, 11, 17, 11, 47, 31, 996_000_000, time.UTC)
	saved, err := SaveFloatingChart(context.Background(), dir, "Next phase", phase, amsterdamPlace())
	if err != nil {
		t.Fatalf("SaveFloatingChart: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, saved.File))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "-qb Nov 17 2026 12:47:31pm ST 1:00E") {
		t.Errorf("the file does not state the Amsterdam clock to the second:\n%s", raw)
	}
}
