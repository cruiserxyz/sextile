package lookup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/cruiserxyz/sextile/internal/store"
)

// Every chart file Astrolog reads is a row in the chart list, and opens: a chart
// cast at sea or far from any atlas city, coordinates spelled in decimals or in
// lower case, and a file that sets its moment with `-qa` or `-qj` instead of
// `-qb`. The list never asks the atlas, so these rows stand without it too.
func TestTheListHoldsEveryChartAstrologReads(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH")
	}
	src := filepath.Join("..", "astrolog", "testdata", "moments")
	files := []string{"Ocean.as", "FarEast-Kiritimati.as", "CoordsDecimal.as", "CoordsLowercase.as", "MomentQA.as", "MomentQJ.as", "LaterQJ.as"}
	for _, withAtlas := range []bool{true, false} {
		dir := useLibraryDir(t)
		if !withAtlas {
			t.Setenv("SEXTILE_ATLAS_PATH", filepath.Join(t.TempDir(), "no-atlas.as"))
		}
		for _, f := range files {
			raw, err := os.ReadFile(filepath.Join(src, f))
			if err != nil {
				t.Fatal(err)
			}
			writeChartFile(t, dir, f, string(raw))
		}
		lib, err := LoadChartLibrary()
		if err != nil {
			t.Fatalf("LoadChartLibrary: %v", err)
		}
		if len(lib.Drops) != 0 {
			t.Errorf("atlas %v: the list dropped %+v", withAtlas, lib.Drops)
		}
		rows := map[string]store.NamedChart{}
		for _, c := range lib.Charts {
			rows[c.Source.File] = c
		}
		for _, f := range files {
			row, ok := rows[f]
			if !ok {
				t.Errorf("atlas %v: %s is not in the list", withAtlas, f)
				continue
			}
			if row.Place.Display() != "" && row.Place.Display() != row.PlaceLabel {
				t.Errorf("atlas %v: %s lists at %q, the file says %q", withAtlas, f, row.Place.Display(), row.PlaceLabel)
			}
			picked, err := ResolvePickedChart(context.Background(), row)
			if err != nil {
				t.Errorf("atlas %v: %s does not open: %v", withAtlas, f, err)
				continue
			}
			if picked.Record == "" || picked.Place.Display() != row.PlaceLabel {
				t.Errorf("atlas %v: %s opens at %q with record %q", withAtlas, f, picked.Place.Display(), picked.Record)
			}
		}
	}
}

// The date a row shows is the date Astrolog casts: a file whose last moment
// switch is a `-qj` after a `-qb` shows the Julian Day's date, not the earlier
// `-qb` year.
func TestTheListShowsTheDateOfALaterJulianDay(t *testing.T) {
	dir := useLibraryDir(t)
	raw, err := os.ReadFile(filepath.Join("..", "astrolog", "testdata", "moments", "LaterQJ.as"))
	if err != nil {
		t.Fatal(err)
	}
	writeChartFile(t, dir, "LaterQJ.as", string(raw))
	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Charts) != 1 {
		t.Fatalf("rows %+v, drops %+v", lib.Charts, lib.Drops)
	}
	if c := lib.Charts[0]; c.Year != 2000 || c.Month != 1 || c.Day != 1 {
		t.Errorf("the row shows %d-%d-%d, want 2000-1-1 (JD 2451545.0), not the earlier -qb date", c.Year, c.Month, c.Day)
	}
}

// A file that states its moment as a Julian Day lists with that day's date.
func TestAJulianDayChartListsWithItsDate(t *testing.T) {
	dir := useLibraryDir(t)
	raw, err := os.ReadFile(filepath.Join("..", "astrolog", "testdata", "moments", "MomentQJ.as"))
	if err != nil {
		t.Fatal(err)
	}
	writeChartFile(t, dir, "MomentQJ.as", string(raw))
	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Charts) != 1 {
		t.Fatalf("rows %+v, drops %+v", lib.Charts, lib.Drops)
	}
	if c := lib.Charts[0]; c.Year != 1990 || c.Month != 6 || c.Day != 1 {
		t.Errorf("the row shows %d-%d-%d, want 1990-6-1 (JD 2448044.1458333 UT)", c.Year, c.Month, c.Day)
	}
}

// A file that names its chart inside `-qc` lists under that name and place,
// and opens there: Astrolog takes the switch's last two words as -zi's.
func TestAQCChartListsAndOpensUnderItsOwnNames(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH")
	}
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "MomentQC.as", "@AI800\n-qc Jun 1 1990 16:30 DT 1E 4:53E 52:22N \"Moment by qc\" \"Amsterdam, Netherlands\"\n")
	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Charts) != 1 {
		t.Fatalf("rows %+v, drops %+v", lib.Charts, lib.Drops)
	}
	row := lib.Charts[0]
	if row.Name != "Moment by qc" || row.PlaceLabel != "Amsterdam, Netherlands" {
		t.Errorf("the row lists %q at %q", row.Name, row.PlaceLabel)
	}
	picked, err := ResolvePickedChart(context.Background(), row)
	if err != nil {
		t.Fatalf("the chart does not open: %v", err)
	}
	if picked.Place.Display() != "Amsterdam, Netherlands" {
		t.Errorf("it opens at %q", picked.Place.Display())
	}
}
