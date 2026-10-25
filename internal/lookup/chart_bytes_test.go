package lookup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// D1. A chart is known by its first bytes and never by its name. Files named
// .as, .legacy and .txt all list when they open with Astrolog's `@AI`; a
// settings file (`@AD`), a text with no header, a note that merely mentions
// the header, a dotfile (even a hidden legacy yaml) and a sub-folder never do, and
// none of them is reported. A file that opens with `@AI` and cannot be read as a chart is the
// one thing that is named, with its reason.
func TestChartsAreKnownByTheirFirstBytes(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "a.as", munichChart)
	writeChartFile(t, dir, "b.legacy", munichChart)
	writeChartFile(t, dir, "c.txt", munichChart)
	writeChartFile(t, dir, "astrolog.as", "@AD780  ; Astrolog defaults.\n-YQ 8\n")
	writeChartFile(t, dir, "colours.as", "@AC780  ; Astrolog colours.\n")
	writeChartFile(t, dir, "zones.as", "@AZ780  ; Astrolog zones.\n")
	writeChartFile(t, dir, "notes.txt", "Birth data for later.\n")
	writeChartFile(t, dir, "mentions.txt", "see the @AI780 header in "+munichChart)
	writeChartFile(t, dir, ".hidden.as", munichChart)
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeChartFile(t, filepath.Join(dir, "sub"), "d.as", munichChart)
	if err := os.MkdirAll(filepath.Join(dir, ".sextile", "backup"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeChartFile(t, filepath.Join(dir, ".sextile", "backup"), "e.as", munichChart)
	// A hidden legacy file is not reported; a visible one still is (below).
	writeChartFile(t, dir, ".hidden.yaml", "chart:\n  name: Hidden\n")
	writeChartFile(t, dir, "visible.yaml", "chart:\n  name: Visible\n")
	// Opens with the header and is no chart: the one named failure.
	writeChartFile(t, dir, "broken.as", "@AI780\nno birth line here\n")

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatalf("LoadChartLibrary: %v", err)
	}
	var files []string
	for _, c := range lib.Charts {
		files = append(files, c.Source.File)
	}
	if got, want := strings.Join(files, ","), "a.as,b.legacy,c.txt"; got != want {
		t.Errorf("listed charts = %q, want %q", got, want)
	}
	if len(lib.Drops) != 2 || lib.Drops[0].File != "broken.as" || lib.Drops[0].Reason == "" ||
		lib.Drops[1].File != "visible.yaml" {
		t.Errorf("drops = %+v, want exactly broken.as (with a reason) and the visible legacy yaml, and no hidden file", lib.Drops)
	}
	if len(lib.Seen) != 3 {
		t.Errorf("fingerprinted %v, want only the three charts", lib.Seen)
	}
}

// D3. Two charts with one file stem (X.as and X.legacy, both valid by their
// bytes) are one chart id. The first in directory order is the chart; the
// second is a drop row that says another file already has the name. Sorting
// is by name, so X.as comes first. The save collision check refuses the slug
// whichever of the two it meets.
func TestSameStemSecondFileIsADrop(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "X.as", munichChart)
	writeChartFile(t, dir, "X.legacy", munichChart)

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Charts) != 1 || lib.Charts[0].Source.File != "X.as" {
		t.Fatalf("charts = %v, want exactly X.as", lib.Charts)
	}
	if len(lib.Drops) != 1 || lib.Drops[0].File != "X.legacy" ||
		!strings.Contains(lib.Drops[0].Reason, "another file already has this name") {
		t.Fatalf("drops = %+v, want one row for X.legacy saying another file already has this name", lib.Drops)
	}
	if owner, err := chartOwning(dir, "x"); err != nil || owner != "X.as" {
		t.Errorf("chartOwning(x) = %q, %v; want the slug refused, naming X.as", owner, err)
	}
	// A chart astrolog cannot read still owns its name.
	writeChartFile(t, dir, "Z.legacy", "@AI780\nno birth line\n")
	if owner, err := chartOwning(dir, "z"); err != nil || owner != "Z.legacy" {
		t.Errorf("chartOwning(z) = %q, %v; an unreadable chart still owns its slug", owner, err)
	}
	// A header-less file of the same stem owns nothing.
	writeChartFile(t, dir, "Y.txt", "notes\n")
	if owner, err := chartOwning(dir, "y"); err != nil || owner != "" {
		t.Errorf("chartOwning(y) = %q, %v; a file with no chart header owns no slug", owner, err)
	}
}
