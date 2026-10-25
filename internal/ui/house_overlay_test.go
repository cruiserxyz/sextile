package ui

import (
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// wholeSignLeoRising is the real Whole Sign frame Astrolog computes for
// 1985-03-21 13:30 UT at 13:24E 52:31N (Ascendant 21Leo50, so house 1 is Leo).
var wholeSignLeoRising = aspect.Cusps{120, 150, 180, 210, 240, 270, 300, 330, 0, 30, 60, 90}

func plain(s string) string { return astrologstyle.StripSGR(s) }

// Both numbers occupy one frame, each at the cell its own house owns — so
// the pair costs no extra width and position still says which house is which.
func TestComposeHouseQuadrant_BothNumbersInOneFrame(t *testing.T) {
	got := plain(composeHouseQuadrant(1, 7, "", false))
	want := "  1 |   | 7 |     "
	if got != want {
		t.Errorf("composeHouseQuadrant(1, 7) = %q, want %q", got, want)
	}
	if len([]rune(got)) != len([]rune(houseQuadrantStrings[0])) {
		t.Errorf("frame width changed: %d vs %d", len([]rune(got)), len([]rune(houseQuadrantStrings[0])))
	}
}

// Every house must land on exactly the cells its own single-house frame uses,
// so a pair never shifts a number out of its quadrant.
func TestComposeHouseQuadrant_MatchesSingleFrameCells(t *testing.T) {
	for h := aspect.House(1); h <= 12; h++ {
		if got, want := plain(composeHouseQuadrant(0, h, "", false)), houseQuadrant(h); got != want {
			t.Errorf("house %d alone = %q, want %q", h, got, want)
		}
	}
}

// Weight carries the distinction: the natal number faint, the live one bold.
// Colour is decoration and may be absent; these two must not be.
func TestComposeHouseQuadrant_DimNatalBoldLive(t *testing.T) {
	got := composeHouseQuadrant(1, 7, "", true)
	faintAt, boldAt := strings.Index(got, sgrFaint), strings.Index(got, sgrBold)
	if faintAt < 0 {
		t.Errorf("styled frame %q has no faint span for the natal house", got)
	}
	if boldAt < 0 {
		t.Errorf("styled frame %q has no bold span for the live house", got)
	}
	if faintAt >= 0 && boldAt >= 0 && faintAt > boldAt {
		t.Errorf("frame %q: natal house must be styled before the live one (house 1 precedes house 7)", got)
	}
}

// The load-bearing test. A transiting body's house must name one of the
// CLIENT's houses, measured against the anchored chart's cusps — not the
// house Astrolog assigned it in a chart cast for now at the operator's place.
//
// Fixture is real: on 2026-08-26 10:00 UT the Sun sits at 3Vir12. In a chart
// cast for that moment in Berlin it is in the 11th; in the 1985 chart's frame
// it is transiting the 2nd.
func TestHouseOverlay_UsesAnchoredFrameNotOwnChart(t *testing.T) {
	ov := houseOverlay{
		cusps: wholeSignLeoRising,
		positions: []aspect.Position{
			{Body: aspect.Sun, Longitude: 0 + 52.0/60, House: 9}, // natal Sun 0Ari52
		},
	}
	movingSun := aspect.Position{
		Body:      aspect.Sun,
		Longitude: 150 + 3 + 12.0/60, // 3Vir12
		House:     11,                // its house in a chart cast for now, in Berlin
	}
	natal, live := ov.houses(movingSun)
	if live != 2 {
		t.Errorf("live house = %d, want 2 (3Vir12 in a Leo-rising whole-sign frame)", live)
	}
	if live == movingSun.House {
		t.Errorf("live house still reads the body's own chart (%d)", movingSun.House)
	}
	if natal != 9 {
		t.Errorf("natal house = %d, want 9", natal)
	}
	// In the frame's zodiac the Sun stands elsewhere (24° back, sidereal): the
	// cusps are the frame's, so the house is read from that position, never
	// from the row's own tropical one.
	ov.framed = []aspect.Position{{Body: aspect.Sun, Longitude: movingSun.Longitude - 24}}
	if _, live := ov.houses(movingSun); live != 1 {
		t.Errorf("framed live house = %d, want 1 (9Leo12 in a Leo-rising whole-sign frame)", live)
	}
}

// A body the anchored chart does not carry shows its live house alone. Pairing
// is by identity, so it must never borrow the neighbouring row's natal house.
func TestHouseOverlay_BodyAbsentFromAnchoredChart(t *testing.T) {
	ov := houseOverlay{
		cusps:     wholeSignLeoRising,
		positions: []aspect.Position{{Body: aspect.Sun, Longitude: 0.867, House: 9}},
	}
	natal, live := ov.houses(aspect.Position{Body: aspect.Chiron, Longitude: 153, House: 11})
	if natal.Valid() {
		t.Errorf("natal house = %d, want none — Chiron is not in the anchored chart", natal)
	}
	if live != 2 {
		t.Errorf("live house = %d, want 2", live)
	}
}

// No frame must degrade to the body's own house, never to an invented
// placement: a wrong-but-well-formatted house reads as authoritative.
func TestHouseOverlay_NoFrameFallsBack(t *testing.T) {
	var ov houseOverlay
	if ov.active() {
		t.Fatal("zero houseOverlay reported active")
	}
	natal, live := ov.houses(aspect.Position{Body: aspect.Mars, Longitude: 194.05, House: 8})
	if natal.Valid() {
		t.Errorf("natal house = %d, want none", natal)
	}
	if live != 8 {
		t.Errorf("live house = %d, want the body's own 8", live)
	}
}

// Single-chart rows keep exactly the rendering they had: one number, no bold.
func TestFormatPositionRow_SingleChartFrameUnchanged(t *testing.T) {
	p := aspect.Position{Body: aspect.Moon, Longitude: 194.05, House: 8}
	row := formatPositionRow(p, false)
	if !strings.Contains(row, houseQuadrant(8)) {
		t.Errorf("row %q lost the single-house frame", row)
	}
	if strings.Contains(row, sgrBold) || strings.Contains(row, sgrFaint) {
		t.Errorf("row %q gained overlay weighting on a single chart", row)
	}
}
