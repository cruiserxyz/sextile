package lookup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/store"
)

// All chart values in this file are constructed synthetic test inputs.

// useLibraryDir isolates ChartsDir at a temp dir via the top-precedence live
// tier. Dev-tier isolation is not enough: an existing operator library at the
// XDG default beats dev, so it would capture test writes.
func useLibraryDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SEXTILE_CHARTS_DIR", dir)
	t.Setenv("SEXTILE_DEV_CHARTS_DIR", "")
	return dir
}

const munichChart = `@AI780  ; test
-qb Sep 23 1995 6:00pm DT 1E 11:34E 48:08N
-zi "Test Person" "Munich, Germany"
`

func writeChartFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A .as lists from its own text, and its moment comes from Astrolog at the
// pick.
func TestLoadChartLibrary_TranslatesChartFile(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "munich.as", munichChart)

	lib, err := LoadChartLibrary()
	charts := lib.Charts
	if err != nil {
		t.Fatalf("LoadChartLibrary: %v", err)
	}
	if len(charts) != 1 {
		t.Fatalf("want 1 chart, got %d: %v", len(charts), charts)
	}
	c := charts[0]
	if c.Name != "Test Person" {
		t.Errorf("Name = %q, want Test Person", c.Name)
	}
	// The ROW carries a display year, read from the file's own -qb line — no
	// moment, and no astrolog spawn to get it.
	if c.Year != 1995 {
		t.Errorf("Year = %d, want 1995 (the -qb year field)", c.Year)
	}
	if c.Source != (store.ChartSource{Kind: store.ChartKindFile, File: "munich.as"}) {
		t.Errorf("Source = %+v, want the .as it came from", c.Source)
	}
	// The MOMENT comes from a resolve, which is where astrolog runs.
	when, err := ResolvePicked(context.Background(), c)
	if err != nil {
		t.Fatalf("ResolvePicked: %v", err)
	}
	if want := time.Date(1995, 9, 23, 16, 0, 0, 0, time.UTC); !when.Equal(want) {
		t.Errorf("resolved moment = %v, want %v (astrolog UT)", when, want)
	}
	// The place is the file's own, with no zone: the file's record casts it.
	if c.Place.Display() != "Munich, Germany" || c.Place.TZ != "" {
		t.Errorf("Place = %+v, want the file's \"Munich, Germany\" and no zone", c.Place)
	}
	if c.Place.Lon < 11.5 || c.Place.Lon > 11.6 {
		t.Errorf("Lon = %v, want the .as's precise ~11.567", c.Place.Lon)
	}
	if !c.IsValid() {
		t.Error("translated chart is not valid")
	}
}

// An empty -zi name falls back to the filename stem.
func TestLoadChartLibrary_EmptyNameUsesStem(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "Anon.as", "@AI780\n-qb Sep 23 1995 6:00pm DT 1E 11:34E 48:08N\n-zi \"\" \"\"\n")

	lib, err := LoadChartLibrary()
	charts := lib.Charts
	if err != nil {
		t.Fatal(err)
	}
	if len(charts) != 1 || charts[0].Name != "Anon" {
		t.Fatalf("want 1 chart named Anon (stem), got %v", charts)
	}
	if charts[0].ID != "anon" {
		t.Errorf("ID = %q, want slug 'anon'", charts[0].ID)
	}
}

// C3.20: a .as's id is the slug of its file stem alone. A ";sextile-id:" line
// an older Sextile appended changes neither the id nor whether the file lists.
func TestChartFileIDIsTheFileStem(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "Constructed-Old.as", munichChart+";sextile-id: something-else\n")
	writeChartFile(t, dir, "Mixed Case.as", munichChart+";sextile-id: !!!\n")

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, c := range lib.Charts {
		ids = append(ids, c.ID)
	}
	if len(ids) != 2 || ids[0] != "constructed-old" || ids[1] != "mixed-case" {
		t.Errorf("ids = %q, want [constructed-old mixed-case] — the slug of each stem", ids)
	}
	if len(lib.Drops) != 0 {
		t.Errorf("a legacy line cost a file its row: %+v", lib.Drops)
	}
}

// A non-chart file is skipped, not surfaced.
func TestLoadChartLibrary_SkipsInvalidChartFile(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "junk.as", "this is not an astrolog chart\n")
	writeChartFile(t, dir, "good.as", munichChart)

	lib, err := LoadChartLibrary()
	charts := lib.Charts
	if err != nil {
		t.Fatal(err)
	}
	if len(charts) != 1 || charts[0].Name != "Test Person" {
		t.Errorf("want only the valid chart, got %v", charts)
	}
}

func TestLoadChartLibrary_MissingDirIsEmpty(t *testing.T) {
	t.Setenv("SEXTILE_CHARTS_DIR", filepath.Join(t.TempDir(), "nope"))
	t.Setenv("SEXTILE_DEV_CHARTS_DIR", "")
	lib, err := LoadChartLibrary()
	charts := lib.Charts
	if err != nil {
		t.Fatalf("missing dir should not error: %v", err)
	}
	if len(charts) != 0 {
		t.Errorf("missing dir → 0 charts, got %d", len(charts))
	}
}

// A .as whose text Sextile cannot read still lists: whether it is a chart is
// Astrolog's to say, and the pick says why it is not.
func TestLoadChartLibrary_ListsAChartFileAndThePickSaysWhy(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "junk.as", "@AI780\nthis is not an astrolog chart\n")
	writeChartFile(t, dir, "good.as", munichChart)

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Charts) != 2 || len(lib.Drops) != 0 {
		t.Fatalf("want both files listed, got %v (drops %+v)", chartNames(lib.Charts), lib.Drops)
	}
	for _, c := range lib.Charts {
		if c.Source.File != "junk.as" {
			continue
		}
		if c.Month != 0 || c.Place.Name != "" {
			t.Errorf("junk.as lists a date or place it does not state: %+v", c)
		}
		// Astrolog runs a bare word in a chart file as a switch, so the guard
		// refuses the file and names the word.
		if _, err := ResolvePickedChart(context.Background(), c); err == nil || !strings.Contains(err.Error(), `"this"`) {
			t.Errorf("the pick of junk.as says %v, want a refusal naming \"this\"", err)
		}
	}
}

// A missing directory is not a failure to report: nothing was there to fail.
func TestLoadChartLibrary_MissingDirDropsNothing(t *testing.T) {
	t.Setenv("SEXTILE_CHARTS_DIR", filepath.Join(t.TempDir(), "nope"))
	t.Setenv("SEXTILE_DEV_CHARTS_DIR", "")
	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Charts) != 0 || len(lib.Drops) != 0 {
		t.Errorf("a missing library → nothing and no accusations, got %d/%d", len(lib.Charts), len(lib.Drops))
	}
}

// A .yaml in the charts folder is no longer a chart: it is named as a drop
// with its own reason, through the same list as the .as ones, so the operator
// sees one accounting of everything unreadable.
func TestLoadChartLibrary_CarriesYAMLDropsThrough(t *testing.T) {
	dir := useLibraryDir(t)
	if err := os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte("nothing: useful\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeChartFile(t, dir, "---.as", munichChart)

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Drops) != 2 {
		t.Fatalf("want the YAML and the .as both named, got %+v", lib.Drops)
	}
	if lib.Drops[0].File != "---.as" || lib.Drops[1].File != "broken.yaml" {
		t.Errorf("both kinds should be in one filename-sorted list, got %+v", lib.Drops)
	}
	if lib.Drops[1].Reason != "Sextile no longer reads charts in the old .yaml form" {
		t.Errorf("the .yaml reason = %q, want it to say Sextile no longer reads charts in the old .yaml form", lib.Drops[0].Reason)
	}
}
