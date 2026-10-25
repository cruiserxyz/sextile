package lookup

import (
	"context"
	"testing"
	"time"
)

// All chart values in this file are constructed synthetic test inputs.

// the other half. The rule the ticket settles is that a listing row states
// the year at the BIRTH PLACE — the year on the client's birth certificate —
// and this holds it for the `.as` side, where the row's year comes from the
// file's own `-qb` field and cannot come from anywhere else without spawning
// astrolog per file (which is exactly what an earlier change removed).
//
// NEXT.md's note that opened this ticket expected the row and the anchored
// chart to disagree by one for a birth near New Year. They do not, and the
// test below is what establishes it: the anchored chart is DISPLAYED in the
// birth place's own timezone (ui.chartSlotLine), not in UT, so it names the
// same local year the `-qb` line does. Only the UT moment falls in the
// adjacent year, and nothing displays that.

// newYearChart is a birth half an hour into 1990 in Auckland (+13 in January).
// Its UT moment is 1989-12-31 11:30 — the straddle this test needs, asserted
// below so the case cannot go stale into a vacuous pass.
const newYearChart = `@AI780  ; test
-qb Jan 1 1990 0:30am DT 12E 174:46E 36:51S
-zi "New Year Client" "Auckland, New Zealand"
`

func TestChartFileRowYearIsTheYearAtTheBirthPlace(t *testing.T) {
	dir := useLibraryDir(t)
	writeChartFile(t, dir, "newyear.as", newYearChart)

	lib, err := LoadChartLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Charts) != 1 {
		t.Fatalf("want one row, got %v (drops %+v, atlas %v)", chartNames(lib.Charts), lib.Drops, lib.AtlasErr)
	}
	row := lib.Charts[0]
	if row.Year != 1990 {
		t.Errorf("Year = %d, want 1990 — the year this client was born in Auckland", row.Year)
	}

	when, err := ResolvePicked(context.Background(), row)
	if err != nil {
		t.Fatalf("ResolvePicked: %v", err)
	}
	// The straddle, stated: without it the agreement below proves nothing.
	if got := when.UTC().Year(); got != 1989 {
		t.Fatalf("the fixture's UT moment is %v (year %d); this test needs a birth whose UT year differs from its local one", when.UTC(), got)
	}

	loc, err := time.LoadLocation(row.Place.TZ)
	if err != nil {
		t.Fatalf("LoadLocation(%q): %v", row.Place.TZ, err)
	}
	// What the anchored slot line shows: the same moment, at the birth place.
	if got := when.In(loc).Year(); got != row.Year {
		t.Errorf("the row says %d and the anchored chart shows %d; they are the same chart", row.Year, got)
	}
}
