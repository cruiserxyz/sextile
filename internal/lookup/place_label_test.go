// All chart values in this file are constructed synthetic test inputs.
package lookup

import (
	"context"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// berlinCoordsChart is Berlin, DE (atlas 13.4105E 52.5244N) rounded to the
// whole arc-minutes a saved record carries. Nearest takes that point for the
// district "Mitte" (13.4049E 52.52N), not for Berlin.
func berlinCoordsChart(zi string) string {
	return "@AI800  ; test\n-qb Mar 14 1987  4:30pm ST 2E  13:25E 52:31N\n" + zi + "\n"
}

// openedPlace returns the place the chart named by file opens under, and the
// place its row in the listing carries; both must agree.
func openedPlace(t *testing.T, dir, file string) location.PlaceInfo {
	t.Helper()
	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatalf("LoadChartLibrary: %v", err)
	}
	var row *store.NamedChart
	for i := range lib.Charts {
		if lib.Charts[i].Source.File == file {
			row = &lib.Charts[i]
		}
	}
	if row == nil {
		t.Fatalf("the library does not list %s (drops: %+v)", file, lib.Drops)
	}
	picked, err := ResolvePickedChartIn(context.Background(), dir, *row)
	if err != nil || !picked.HasPlace {
		t.Fatalf("ResolvePickedChartIn: %v (HasPlace %v)", err, picked.HasPlace)
	}
	if picked.Place.Display() != row.Place.Display() {
		t.Errorf("opening says %q, the listing says %q", picked.Place.Display(), row.Place.Display())
	}
	return picked.Place
}

// T1: Ctrl+S in Berlin, DE, then reopen: the row said "Mitte, DE".
func TestSavedBerlinReopensAsBerlin(t *testing.T) {
	dir := useLibraryDir(t)
	berlin := location.PlaceInfo{Name: "Berlin", Country: "DE", Lon: 13.4105, Lat: 52.5244, TZ: "Europe/Berlin"}
	saved, err := SaveFloatingChart(context.Background(), dir, "Berlin Person",
		time.Date(1987, 3, 14, 16, 30, 0, 0, time.FixedZone("CET", 3600)), berlin)
	if err != nil {
		t.Fatalf("SaveFloatingChart: %v", err)
	}
	got := openedPlace(t, dir, saved.File)
	if got.Display() != "Berlin, DE" {
		t.Errorf("the saved chart reopens under %q, want \"Berlin, DE\"", got.Display())
	}
	if got.TZ != "Europe/Berlin" {
		t.Errorf("TZ = %q, want Europe/Berlin", got.TZ)
	}
}

// T2: the same coordinates with no place label open as Nearest says.
func TestChartWithoutPlaceLabelOpensAsNearest(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "nolabel.as", berlinCoordsChart(`-zi "No Label" ""`))
	if got := openedPlace(t, dir, "nolabel.as"); got.Display() != "Mitte, DE" {
		t.Errorf("a file with no place label opens under %q, want Nearest's \"Mitte, DE\"", got.Display())
	}
}

// T3: a label naming a city more than an arc-minute away is ignored.
func TestPlaceLabelFarAwayIsIgnored(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "hamburg.as", berlinCoordsChart(`-zi "Far Label" "Hamburg, DE"`))
	if got := openedPlace(t, dir, "hamburg.as"); got.Display() != "Mitte, DE" {
		t.Errorf("a label naming a distant city opens under %q, want Nearest's \"Mitte, DE\"", got.Display())
	}
}
