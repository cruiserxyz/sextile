package aspect

import "testing"

// dm builds a longitude from sign-base degrees plus degree/arcminute, the
// only resolution Astrolog reports.
func dm(base float64, deg, min int) float64 {
	return base + float64(deg) + float64(min)/60
}

const (
	ari, tau, gem, can = 0.0, 30.0, 60.0, 90.0
	leo, vir, lib, sco = 120.0, 150.0, 180.0, 210.0
	sag, cap, aqu, pis = 240.0, 270.0, 300.0, 330.0
)

// placidusBerlin1985 is the real Placidus frame Astrolog computes for
// 1985-03-21 13:30 UT at 13:24E 52:31N. Quadrant cusps, so nothing lands on
// a sign boundary — the point being that the lookup must not assume whole sign.
var placidusBerlin1985 = Cusps{
	dm(leo, 21, 50), dm(vir, 10, 11), dm(lib, 4, 30), dm(sco, 7, 15),
	dm(sag, 17, 6), dm(cap, 23, 31), dm(aqu, 21, 50), dm(pis, 10, 11),
	dm(ari, 4, 30), dm(tau, 7, 15), dm(gem, 17, 6), dm(can, 23, 31),
}

// HouseFor must reproduce Astrolog's own house assignments for the same
// chart. Golden values are Astrolog's `[ Nth house]` output for every body it
// reported, so this fails if the lookup ever disagrees with the ephemeris
// rather than merely with itself.
//
// Asce and Midh sit exactly on cusps 1 and 10, which pins the boundary rule:
// a cusp degree belongs to the house it opens.
func TestCuspsHouseFor_MatchesAstrologPlacidus(t *testing.T) {
	cases := []struct {
		name string
		lon  float64
		want House
	}{
		{"Sun 0Ari52", dm(ari, 0, 52), 8},
		{"Moon 1Ari34", dm(ari, 1, 34), 8},
		{"Mercury 17Ari58", dm(ari, 17, 58), 9},
		{"Venus 21Ari02", dm(ari, 21, 2), 9},
		{"Mars 4Tau36", dm(tau, 4, 36), 9},
		{"Jupiter 9Aqu04", dm(aqu, 9, 4), 6},
		{"Saturn 27Sco57", dm(sco, 27, 57), 4},
		{"Uranus 17Sag59", dm(sag, 17, 59), 5},
		{"Neptune 3Cap33", dm(cap, 3, 33), 5},
		{"Pluto 4Sco13", dm(sco, 4, 13), 3},
		{"NorthNode 20Tau56", dm(tau, 20, 56), 10},
		{"Ascendant 21Leo50 (exactly cusp 1)", dm(leo, 21, 50), 1},
		{"Midheaven 7Tau15 (exactly cusp 10)", dm(tau, 7, 15), 10},
	}
	for _, c := range cases {
		if got := placidusBerlin1985.HouseFor(c.lon); got != c.want {
			t.Errorf("%s: HouseFor(%.4f) = %d, Astrolog says %d", c.name, c.lon, got, c.want)
		}
	}
}

// The frame that wraps through 0° Aries is ordinary, not an edge case: house
// 8 here runs 10Pis11 → 4Ari30, crossing the origin.
func TestCuspsHouseFor_WrapsThroughAries(t *testing.T) {
	for _, c := range []struct {
		lon  float64
		want House
	}{
		{dm(pis, 29, 59), 8},
		{dm(ari, 0, 0), 8},
		{dm(ari, 4, 29), 8},
		{dm(ari, 4, 30), 9},
		{dm(ari, 4, 31), 9},
	} {
		if got := placidusBerlin1985.HouseFor(c.lon); got != c.want {
			t.Errorf("HouseFor(%.4f) = %d, want %d", c.lon, got, c.want)
		}
	}
}

// Whole Sign is the lens Sextile ships. Cusps land on sign boundaries and the
// same lookup handles them with no special case.
func TestCuspsHouseFor_WholeSign(t *testing.T) {
	ws := Cusps{leo, vir, lib, sco, sag, cap, aqu, pis, ari, tau, gem, can}
	for _, c := range []struct {
		lon  float64
		want House
	}{
		{dm(leo, 0, 0), 1},
		{dm(leo, 29, 59), 1},
		{dm(vir, 0, 0), 2},
		{dm(ari, 0, 0), 9},
		{dm(can, 29, 59), 12},
	} {
		if got := ws.HouseFor(c.lon); got != c.want {
			t.Errorf("HouseFor(%.4f) = %d, want %d", c.lon, got, c.want)
		}
	}
}

// An absent frame must yield an invalid house, never a plausible one.
func TestCuspsHouseFor_ZeroFrameRefuses(t *testing.T) {
	var none Cusps
	if none.Valid() {
		t.Fatal("zero Cusps reported Valid")
	}
	if got := none.HouseFor(dm(leo, 15, 0)); got.Valid() {
		t.Errorf("HouseFor on zero frame = %d, want invalid", got)
	}
}
