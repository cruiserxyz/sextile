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
// whole arc-minutes a saved record carries. The atlas's nearest entry to that
// point is the district "Mitte" (13.4049E 52.52N), not Berlin.
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
	if err != nil {
		t.Fatalf("ResolvePickedChartIn: %v", err)
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
}

// T2: the same coordinates with no place label open under the coordinates,
// not under the atlas's nearest entry.
func TestChartWithoutPlaceLabelOpensAtItsCoordinates(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "nolabel.as", berlinCoordsChart(`-zi "No Label" ""`))
	if got := openedPlace(t, dir, "nolabel.as"); got.Display() != "13°25'E 52°31'N" {
		t.Errorf("a file with no place label opens under %q, want its coordinates", got.Display())
	}
}

// T3: a label naming a city far from the coordinates is the file's own words,
// and the chart opens under them.
func TestPlaceLabelIsTheFilesOwn(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "hamburg.as", berlinCoordsChart(`-zi "Far Label" "Hamburg, DE"`))
	if got := openedPlace(t, dir, "hamburg.as"); got.Display() != "Hamburg, DE" {
		t.Errorf("the chart opens under %q, want the file's \"Hamburg, DE\"", got.Display())
	}
}
