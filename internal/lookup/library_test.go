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

// A .as is translated through astrolog (authoritative UT) and the atlas
// (real country + IANA tz + canonical city), exercising the full an earlier change seam.
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
	// Country + tz borrowed from the nearest atlas city (Munich); precise
	// coordinates kept from the .as.
	if c.Place.Country != "DE" {
		t.Errorf("Country = %q, want DE (from atlas)", c.Place.Country)
	}
	if c.Place.TZ != "Europe/Berlin" {
		t.Errorf("TZ = %q, want Europe/Berlin (from atlas)", c.Place.TZ)
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

// a .as astrolog cannot read is named rather than silently missing.
func TestLoadChartLibrary_NamesUnreadableChartFile(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "junk.as", "@AI780\nthis is not an astrolog chart\n")
	writeChartFile(t, dir, "good.as", munichChart)

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Charts) != 1 || lib.Charts[0].Name != "Test Person" {
		t.Errorf("want only the valid chart, got %v", lib.Charts)
	}
	if len(lib.Drops) != 1 {
		t.Fatalf("want the unreadable .as named, got %d: %+v", len(lib.Drops), lib.Drops)
	}
	if lib.Drops[0].File != "junk.as" {
		t.Errorf("drop names %q, want junk.as", lib.Drops[0].File)
	}
	if lib.Drops[0].Reason == "" {
		t.Error("a named drop must carry a reason")
	}
	if lib.AtlasErr != nil {
		t.Errorf("the atlas was available; AtlasErr should be nil, got %v", lib.AtlasErr)
	}
}

// Without the atlas there is no timezone to give a .as, so every one of them
// drops. the contract is that each is still named, and that the shared
// cause comes back once — the picker puts it on the error lane.
func TestLoadChartLibrary_NoAtlasNamesEveryChartFileAndReportsTheCauseOnce(t *testing.T) {
	dir := useLibraryDir(t)
	t.Setenv("SEXTILE_ATLAS_PATH", filepath.Join(t.TempDir(), "no-atlas-here.as"))
	writeChartFile(t, dir, "one.as", munichChart)
	writeChartFile(t, dir, "two.as", munichChart)

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatalf("a missing atlas must not fail the whole read: %v", err)
	}
	if lib.AtlasErr == nil {
		t.Fatal("a missing atlas must come back as AtlasErr, not as silence")
	}
	if len(lib.Charts) != 0 {
		t.Errorf("a .as cannot list without the atlas, got %v", lib.Charts)
	}
	if len(lib.Drops) != 2 {
		t.Fatalf("want both .as files named, got %d: %+v", len(lib.Drops), lib.Drops)
	}
	for _, d := range lib.Drops {
		if !strings.Contains(d.Reason, "place list") {
			t.Errorf("%s reason = %q, want it to blame the missing place list", d.File, d.Reason)
		}
	}
	if lib.Drops[0].File != "one.as" || lib.Drops[1].File != "two.as" {
		t.Errorf("drops should be sorted by filename, got %+v", lib.Drops)
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
	writeChartFile(t, dir, "junk.as", "@AI780\nnot a chart\n")

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Drops) != 2 {
		t.Fatalf("want the YAML and the .as both named, got %+v", lib.Drops)
	}
	if lib.Drops[0].File != "broken.yaml" || lib.Drops[1].File != "junk.as" {
		t.Errorf("both kinds should be in one filename-sorted list, got %+v", lib.Drops)
	}
	if lib.Drops[0].Reason != "YAML charts are no longer read" {
		t.Errorf("the .yaml reason = %q, want it to say YAML charts are no longer read", lib.Drops[0].Reason)
	}
}
