// All chart values reachable from this file are constructed synthetic test inputs.
package lookup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cruiserxyz/sextile/internal/store"
)

// Verifiers for the specification item 3 that live in lookup: what a
// library row says about its file's title (C3.1) and richness (C3.2).

// copyFixture copies one of internal/astrolog's chart fixtures into dir. The
// fixtures are only ever COPIED: nothing here writes inside testdata/.
func copyFixture(t *testing.T, dir, name string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "astrolog", "testdata", "charts", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func rowByFile(t *testing.T, lib Library, file string) store.NamedChart {
	t.Helper()
	for _, c := range lib.Charts {
		if c.Source.File == file {
			return c
		}
	}
	t.Fatalf("no row for %s in %+v (drops %+v)", file, lib.Charts, lib.Drops)
	return store.NamedChart{}
}

// C3.1: a row says whether its file carries a title and a place label.
func TestLibraryMarksUntitled(t *testing.T) {
	dir := useLibraryDir(t)
	for _, f := range []string{"Equinox-2026-09.as", "FullMoon-2026-02.as", "GreatConjunction-2020-12-21.as"} {
		copyFixture(t, dir, f)
	}
	writeChartFile(t, dir, "NoZI.as", "@AI780  ; Astrolog chart info.\n-qb Sep 23 1995 6:00pm DT 1E 11:34E 48:08N\n")
	// A YAML chart is no longer read: it lists as a named drop, never a row.
	writeChartFile(t, dir, "constructed.yaml", "chart:\n  name: Constructed\n")

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		file       string
		untitled   bool
		placeLabel string
		name       string
	}{
		{"Equinox-2026-09.as", true, "Bremen, Germany", "Equinox-2026-09"},
		{"FullMoon-2026-02.as", true, "", "FullMoon-2026-02"},
		{"GreatConjunction-2020-12-21.as", false, "Frankfurt, Germany", "Great Conjunction"},
		{"NoZI.as", true, "", "NoZI"},
	} {
		c := rowByFile(t, lib, tc.file)
		if c.Untitled != tc.untitled || c.PlaceLabel != tc.placeLabel || c.Name != tc.name {
			t.Errorf("%s: Untitled=%v PlaceLabel=%q Name=%q; want %v %q %q",
				tc.file, c.Untitled, c.PlaceLabel, c.Name, tc.untitled, tc.placeLabel, tc.name)
		}
	}
	for _, c := range lib.Charts {
		if c.Source.File == "constructed.yaml" {
			t.Error("a YAML chart listed as a row")
		}
	}
	if len(lib.Drops) != 1 || lib.Drops[0].File != "constructed.yaml" || lib.Drops[0].Reason != "YAML charts are no longer read" {
		t.Errorf("drops = %+v, want constructed.yaml named as no longer read", lib.Drops)
	}
}
