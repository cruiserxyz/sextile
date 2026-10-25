package lookup

// Verifiers for the listing contract: the chart library lists with ZERO
// astrolog spawns, writes nothing, and resolves only what is picked.

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/store"
)

// All chart values in this file are constructed synthetic test inputs.

// astrologShim points PATH at a fake `astrolog` that records every invocation,
// and returns the count so far plus a way to clear it.
//
// internal/astrolog spawns the BARE NAME "astrolog", so exec resolves it
// through PATH and this intercepts every package's spawns — with no test-only
// seam, export or build tag in production code. That is exactly what
// invariants.TestAstrologSpawnChokePoint (C22) pins: a direct os/exec, an
// absolute path or a hidden seam would slip past this shim, and every
// spawn-freedom verifier here would report zero while spawning freely.
//
// Deliberately duplicated in internal/ui: an unexported test helper cannot
// cross a package boundary, and exporting one would be the seam C22 forbids.
func astrologShim(t *testing.T) (count func() int, clear func()) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "invocations")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + strconv.Quote(log) + "\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "astrolog"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return func() int {
			b, err := os.ReadFile(log)
			if err != nil {
				return 0
			}
			return len(strings.Split(strings.TrimSuffix(string(b), "\n"), "\n"))
		}, func() {
			_ = os.Remove(log)
		}
}

// readOnlyChartDir makes dir and everything in it unwritable, so any attempt to
// create or rewrite a chart file fails instead of passing unnoticed. The
// permissions are restored before t.TempDir's own cleanup runs (cleanups are
// LIFO), or the directory could not be removed.
func readOnlyChartDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if err := os.Chmod(filepath.Join(dir, e.Name()), 0o400); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700)
		for _, e := range entries {
			_ = os.Chmod(filepath.Join(dir, e.Name()), 0o600)
		}
	})
}

// seedMixedLibrary writes two .as charts, both of which list.
func seedMixedLibrary(t *testing.T) string {
	t.Helper()
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "munich.as", munichChart)
	writeChartFile(t, dir, "saved.as", "@AI780\n-qb Jan 5 1980 9:00am ST 1E 8:48E 53:05N\n-zi \"Saved Client\" \"Bremen, Germany\"\n")
	return dir
}

// C1: opening the picker used to cost one astrolog spawn per chart in the
// library, before a single row was drawn. It now costs none.
func TestLoadChartLibrary_SpawnsNothing(t *testing.T) {
	dir := seedMixedLibrary(t)
	count, clear := astrologShim(t)

	// Positive control: a path that DOES spawn, under this very shim. Without
	// it a zero below would prove "the shim never intercepted" just as readily
	// as "nothing spawned".
	_, _ = ResolvePicked(context.Background(), store.NamedChart{
		Name:   "control",
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "munich.as"},
	})
	if got := count(); got != 1 {
		t.Fatalf("positive control recorded %d spawns, want 1 — the shim is not intercepting", got)
	}
	clear()

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if got := count(); got != 0 {
		t.Errorf("listing the library spawned astrolog %d time(s); it must spawn none", got)
	}
	if len(lib.Charts) != 2 {
		t.Fatalf("the listing must still produce both rows, got %v (drops %+v)", chartNames(lib.Charts), lib.Drops)
	}
	_ = dir
}

// C13: listing a library is a read. It never opens a file under ChartsDir()
// for writing — the operator's client files are not Sextile's scratch space.
func TestLoadChartLibrary_NeverWritesChartDir(t *testing.T) {
	dir := seedMixedLibrary(t)
	readOnlyChartDir(t, dir)

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatalf("listing a read-only library must still work: %v", err)
	}
	if len(lib.Charts) != 2 {
		t.Errorf("want both rows from a read-only library, got %v (drops %+v)", chartNames(lib.Charts), lib.Drops)
	}
}

// C19: neither does resolving one.
func TestResolvePicked_NeverWritesChartDir(t *testing.T) {
	dir := seedMixedLibrary(t)
	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	readOnlyChartDir(t, dir)

	for _, c := range lib.Charts {
		when, err := ResolvePicked(context.Background(), c)
		if err != nil {
			t.Errorf("%s: resolving from a read-only library failed: %v", c.Name, err)
			continue
		}
		if when.IsZero() {
			t.Errorf("%s: resolve returned a zero moment", c.Name)
		}
	}
}

// C14: a row's source identity is a chart kind plus a locator under
// ChartsDir() — enough to reopen the exact file, and nothing more.
func TestNamedChart_CarriesSourceHandle(t *testing.T) {
	seedMixedLibrary(t)
	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]store.ChartSource{
		"Test Person":  {Kind: store.ChartKindFile, File: "munich.as"},
		"Saved Client": {Kind: store.ChartKindFile, File: "saved.as"},
	}
	for _, c := range lib.Charts {
		w, ok := want[c.Name]
		if !ok {
			t.Errorf("unexpected chart %q", c.Name)
			continue
		}
		if c.Source != w {
			t.Errorf("%s: Source = %+v, want %+v", c.Name, c.Source, w)
		}
		if filepath.Base(c.Source.File) != c.Source.File {
			t.Errorf("%s: locator %q is not a plain name under the chart folder", c.Name, c.Source.File)
		}
		// The handle alone must reopen the file: a resolve is given nothing
		// else about the chart.
		if _, err := ResolvePicked(context.Background(), store.NamedChart{Name: c.Name, Source: c.Source}); err != nil {
			t.Errorf("%s: the source handle alone did not reopen the file: %v", c.Name, err)
		}
	}
}

// C3: a .as row's display year is the year FIELD of that file's own -qb line,
// read from the text. It is a label; the moment it belongs to is never derived
// here.
func TestTranslateChartFile_DisplayYearFromQB(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "munich.as", munichChart)
	writeChartFile(t, dir, "numeric.as", "@AI780\n-qb 1 1 2000 9:00am ST 0W 11:34E 48:08N\n-zi \"Numeric Month\" \"\"\n")

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"Test Person": 1995, "Numeric Month": 2000}
	if len(lib.Charts) != len(want) {
		t.Fatalf("want %d rows, got %v (drops %+v)", len(want), chartNames(lib.Charts), lib.Drops)
	}
	for _, c := range lib.Charts {
		if c.Year != want[c.Name] {
			t.Errorf("%s: Year = %d, want %d (the -qb year field)", c.Name, c.Year, want[c.Name])
		}
	}
}

// The failure mode behind C3: a -qb line with no parseable year drops the
// chart, exactly as an unparseable chart is dropped.
func TestTranslateChartFile_UnparseableYearDrops(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "noyear.as", "@AI780\n-qb Jun 15 xxxx 2:30pm DT 1E 11:34E 48:08N\n-zi \"No Year\" \"\"\n")
	writeChartFile(t, dir, "good.as", munichChart)

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Charts) != 1 || lib.Charts[0].Name != "Test Person" {
		t.Errorf("want only the readable chart, got %v", chartNames(lib.Charts))
	}
	if len(lib.Drops) != 1 || lib.Drops[0].File != "noyear.as" {
		t.Errorf("the yearless file must be named as a drop, got %+v", lib.Drops)
	}
}

// A file that opens with @AI and is no chart is refused by the header read exactly as
// ResolveChartFile refused it, so the listing drops what it always dropped.
func TestTranslateChartFile_NonChartFileDrops(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "junk.as", "@AI780\nthis is not an astrolog chart\n")
	writeChartFile(t, dir, "good.as", munichChart)

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Charts) != 1 || lib.Charts[0].Name != "Test Person" {
		t.Errorf("want only the valid chart, got %v", chartNames(lib.Charts))
	}
	if len(lib.Drops) != 1 || lib.Drops[0].File != "junk.as" {
		t.Errorf("the non-chart file must be named as a drop, got %+v", lib.Drops)
	}
}

// A .yaml in the charts folder is a named drop, not a chart, and naming it
// costs no astrolog spawn.
func TestLoadChartLibrary_YAMLIsANamedDropAndSpawnsNothing(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "saved.yaml", "chart:\n  name: Saved Client\n")
	writeChartFile(t, dir, "control.as", munichChart)
	count, clear := astrologShim(t)

	// Positive control: this shim does intercept a spawn.
	_, _ = ResolvePicked(context.Background(), store.NamedChart{
		Name:   "control",
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "control.as"},
	})
	if got := count(); got != 1 {
		t.Fatalf("positive control recorded %d spawns, want 1 — the shim is not intercepting", got)
	}
	clear()
	if err := os.Remove(filepath.Join(dir, "control.as")); err != nil {
		t.Fatal(err)
	}

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Charts) != 0 {
		t.Errorf("a .yaml must not list as a chart, got %v", chartNames(lib.Charts))
	}
	if len(lib.Drops) != 1 || lib.Drops[0].File != "saved.yaml" || lib.Drops[0].Reason != "YAML charts are no longer read" {
		t.Errorf("want saved.yaml named with the no-longer-read reason, got %+v", lib.Drops)
	}
	if got := count(); got != 0 {
		t.Errorf("naming a .yaml spawned astrolog %d time(s); it must spawn none", got)
	}
}

// Without the atlas a .as has no timezone to be given, so it drops — named,
// per an earlier change, whose contract governs how the drop is reported.
func TestNoAtlas_ChartFileChartsDrop(t *testing.T) {
	dir := useLibraryDir(t)
	t.Setenv("SEXTILE_ATLAS_PATH", filepath.Join(t.TempDir(), "no-atlas.as"))
	writeChartFile(t, dir, "munich.as", munichChart)

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Charts) != 0 {
		t.Errorf("a .as cannot list without the atlas, got %v", chartNames(lib.Charts))
	}
	if lib.AtlasErr == nil {
		t.Error("the shared cause must come back for the error lane")
	}
	if len(lib.Drops) != 1 || lib.Drops[0].File != "munich.as" {
		t.Errorf("the dropped .as must still be named, got %+v", lib.Drops)
	}
}

// And it still spawns nothing on the way there: a broken atlas must not turn
// the listing back into a subprocess storm.
func TestNoAtlas_ListingStillSpawnFree(t *testing.T) {
	dir := useLibraryDir(t)
	t.Setenv("SEXTILE_ATLAS_PATH", filepath.Join(t.TempDir(), "no-atlas.as"))
	writeChartFile(t, dir, "munich.as", munichChart)
	writeChartFile(t, dir, "second.as", munichChart)
	count, clear := astrologShim(t)

	_, _ = ResolvePicked(context.Background(), store.NamedChart{
		Name:   "control",
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "munich.as"},
	})
	if got := count(); got != 1 {
		t.Fatalf("positive control recorded %d spawns, want 1 — the shim is not intercepting", got)
	}
	clear()

	if _, err := LoadChartLibrary(); err != nil {
		t.Fatal(err)
	}
	if got := count(); got != 0 {
		t.Errorf("listing without an atlas spawned astrolog %d time(s), want 0", got)
	}
}

func chartNames(cs []store.NamedChart) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = cs[i].Name
		_ = c
	}
	return out
}

// The year and the coordinates must come from the SAME -qb line. Read from the
// whole file, two patterns can take their fields from different lines and build
// a row describing a chart that does not exist: this file's second line carries
// the year 1990, its first line carries the coordinates near Munich, and
// pairing them would list "1990 in Munich" for a chart that is neither.
//
// Refusing the file is a correct outcome — the coordinates come from the line
// astrolog itself would read, and that line has no year, so there is nothing to
// put on a row. What must never happen is the mixed row.
func TestTranslateChartFile_YearAndCoordsComeFromOneLine(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "twoqb.as", "@AI780\n"+
		"-qb Jun 15 2:30pm DT 1E 11:34E 48:08N\n"+
		"-qb Jun 15 1990 4:30pm DT 1E 8:43E 52:51N\n"+
		"-zi \"Two QB\" \"\"\n")

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range lib.Charts {
		if c.Year == 1990 && c.Place.Lon > 11 {
			t.Errorf("row pairs the second line's year with the first line's coordinates: %+v", c)
		}
	}
	if len(lib.Charts) == 0 {
		if len(lib.Drops) != 1 || lib.Drops[0].File != "twoqb.as" {
			t.Errorf("a refused file must still be named: %+v", lib.Drops)
		}
	}
}

// A .as whose -qb line is well formed enough to read but which astrolog cannot
// turn into an instant now LISTS and fails at pick time, instead of vanishing
// from the listing. That is the deliberate boundary: not asking astrolog is the
// point, and a named failure beats a silent absence.
func TestTranslateChartFile_ListsWhatOnlyAstrologCanRefuse(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "oddzone.as", "@AI780\n-qb Jun 31 1985 nonsense XX 11:34E 48:08N\n-zi \"Odd Zone\" \"\"\n")

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Charts) != 1 || lib.Charts[0].Name != "Odd Zone" {
		t.Fatalf("want the row listed for a pick-time verdict, got %v (drops %+v)", chartNames(lib.Charts), lib.Drops)
	}
	if lib.Charts[0].Year != 1985 {
		t.Errorf("Year = %d, want 1985 from its own -qb line", lib.Charts[0].Year)
	}
}
