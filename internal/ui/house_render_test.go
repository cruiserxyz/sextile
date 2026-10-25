package ui

import (
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// formatPositionRow must include the quadrant house indicator when
// Position.House is valid. This is what makes a city change visibly different
// in the bodies panel: same body, same geocentric longitude, different house.
//
// Context: 2026-04-29 user reported "Dubai → Berlin moon position stays the
// same". Geocentric longitudes correctly don't change with location; the
// per-body house assignment does. astrolog computes it; the quadrant frame
// (Slice B1) shows which house — and which zodiac quadrant — the body occupies.
func TestFormatPositionRow_IncludesHouseQuadrant(t *testing.T) {
	p := aspect.Position{
		Body:      aspect.Moon,
		Longitude: 194.05, // 14Lib03 — same in Dubai and Berlin (geocentric)
		House:     aspect.House(8),
	}
	row := formatPositionRow(p, false)
	if !strings.Contains(row, houseQuadrant(8)) {
		t.Errorf("row %q missing the house-8 quadrant frame %q", row, houseQuadrant(8))
	}
	if !strings.Contains(row, "Lib") {
		t.Errorf("row %q missing 'Lib' sign", row)
	}
}

// City change: same Moon longitude, different house assignment must produce a
// visibly different rendered row — the user-facing guarantee that switching
// cities updates the bodies view.
func TestFormatPositionRow_DiffersByHouseAcrossCities(t *testing.T) {
	dubai := aspect.Position{Body: aspect.Moon, Longitude: 194.05, House: aspect.House(1)}
	berlin := aspect.Position{Body: aspect.Moon, Longitude: 194.05, House: aspect.House(2)}
	if formatPositionRow(dubai, false) == formatPositionRow(berlin, false) {
		t.Errorf("rows must differ — same body/longitude, different house")
	}
}

// House.Valid() returns false for House(0); rows without house data render with
// no quadrant frame so synthetic positions that omit houses stay clean.
func TestFormatPositionRow_NoHouseSegmentWhenZero(t *testing.T) {
	p := aspect.Position{Body: aspect.Sun, Longitude: 35.5, House: aspect.House(0)}
	row := formatPositionRow(p, false)
	if strings.Contains(row, "|") {
		t.Errorf("row %q should not contain a quadrant frame when House is zero", row)
	}
}

// TestHouseQuadrant_AllTwelve: every house maps to its 18-cell frame containing
// the house number, and out-of-range houses render empty.
func TestHouseQuadrant_AllTwelve(t *testing.T) {
	for h := aspect.House(1); h <= 12; h++ {
		got := houseQuadrant(h)
		if len([]rune(got)) != 18 {
			t.Errorf("houseQuadrant(%d) width = %d, want 18 (%q)", h, len([]rune(got)), got)
		}
		if !strings.Contains(got, itoa(int(h))) {
			t.Errorf("houseQuadrant(%d) = %q, missing the house number", h, got)
		}
	}
	if got := houseQuadrant(0); got != "" {
		t.Errorf("houseQuadrant(0) = %q, want empty", got)
	}
	if got := houseQuadrant(13); got != "" {
		t.Errorf("houseQuadrant(13) = %q, want empty", got)
	}
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
