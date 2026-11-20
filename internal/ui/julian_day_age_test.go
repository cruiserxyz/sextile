package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/lookup"
)

// TestAJulianDayChartShowsItsDateAndAgeInTheList: a file that states its
// moment as `-qj` lists with that day's date and an age, not a blank.
func TestAJulianDayChartShowsItsDateAndAgeInTheList(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "astrolog", "testdata", "moments", "MomentQJ.as"))
	if err != nil {
		t.Fatal(err)
	}
	titleLibrary(t, nil, map[string]string{"MomentQJ.as": string(raw)})
	lib, err := lookup.LoadChartLibrary()
	if err != nil || len(lib.Charts) != 1 {
		t.Fatalf("library: %+v %v", lib.Charts, err)
	}
	c := lib.Charts[0]
	if got := chartDate(c); got != "Jun  1, 1990" {
		t.Errorf("chartDate = %q, want %q", got, "Jun  1, 1990")
	}
	age, ok := chartAge(c, time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC))
	if !ok || age != 36 {
		t.Errorf("chartAge = %d, %v; want 36, true", age, ok)
	}
}
