package astrolog

// The README's example chart is the one moment every public example starts
// from: Hamburg, 24 October 2026, 12:00 CEST. The file is the record Ctrl+S
// wrote; this pins what Astrolog reads back from it, so an edit to the file
// that moves the moment, the place or the title fails here by name.

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"
)

const exampleChart = "Demo-Hamburg-2026-10-24.as"

func TestExampleChartIsHamburgAtNoon(t *testing.T) {
	needAstrolog(t)
	path := filepath.Join("..", "..", "examples", exampleChart)
	cf, err := ResolveChartFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	// 12:00 in Hamburg on this date is summer time, UTC+2.
	if want := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC); !cf.WhenUTC.Equal(want) {
		t.Errorf("instant = %v, want %v", cf.WhenUTC, want)
	}
	// Astrolog records the atlas entry (9.993E 53.5507N) to the minute.
	if math.Abs(cf.Lon-10.0) > 1e-9 || math.Abs(cf.Lat-(53+33.0/60)) > 1e-9 {
		t.Errorf("coordinates = %v %v, want 10:00E 53:33N", cf.Lon, cf.Lat)
	}
	if cf.Name != "Hamburg noon" || cf.PlaceLabel != "Hamburg, DE" {
		t.Errorf("name/place = %q/%q, want \"Hamburg noon\"/\"Hamburg, DE\"", cf.Name, cf.PlaceLabel)
	}
	// The file names no house system, so the reader's own default applies.
	if cf.HasHouses {
		t.Errorf("the example names a house system (%d); docs/REFERENCE.md says it does not", int(cf.Houses))
	}
}
