package ui

import (
	"fmt"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Verifiers for an earlier change: the arcminute a row shows is the arcminute Astrolog
// printed, not one less.

// The reported regression, exactly. Astrolog prints `3Vir12`; internal/astrolog
// parses that into 150 + 3 + 12/60, which float64 stores a hair below 153.2; the
// old floor turned the remainder 11.999999999999318 into 11.
func TestSplitLongitudeReproducesAstrologsArcminute(t *testing.T) {
	const virgo = 5 // sign index: Aries 0 … Virgo 5
	lon := float64(virgo*30) + 3 + 12.0/60.0

	sign, deg, min := splitLongitude(lon)
	if sign != virgo || deg != 3 || min != 12 {
		t.Errorf("splitLongitude(%.17g) = (%d, %d, %d), want Virgo 3°12' as Astrolog printed it", lon, sign, deg, min)
	}
}

// The property behind that case: every arcminute Astrolog can print survives
// the trip through a float64 longitude and back. 21,600 of them, which is the
// whole space — a body row cannot show a value this does not cover.
func TestEveryArcminuteRoundTrips(t *testing.T) {
	for sign := 0; sign < 12; sign++ {
		for deg := 0; deg < 30; deg++ {
			for min := 0; min < 60; min++ {
				// The assembly internal/astrolog.parsePositions performs.
				lon := float64(sign*30) + float64(deg) + float64(min)/60.0
				gotSign, gotDeg, gotMin := splitLongitude(lon)
				if gotSign != sign || gotDeg != deg || gotMin != min {
					t.Fatalf("%d°%02d' of sign %d came back as %d°%02d' of sign %d",
						deg, min, sign, gotDeg, gotMin, gotSign)
				}
			}
		}
	}
}

// A longitude that is computed rather than parsed can land just under a whole
// degree, and rounding then has to carry rather than print a sixtieth minute or
// a thirtieth degree.
func TestSplitLongitudeCarries(t *testing.T) {
	cases := []struct {
		name           string
		lon            float64
		sign, deg, min int
	}{
		{"a hair under the next degree", 3.99999, 0, 4, 0},
		{"a hair under the next sign", 29.99999, 1, 0, 0},
		{"a hair under a full circle wraps to Aries", 359.99999, 0, 0, 0},
		{"exactly half an arcminute rounds up", 0.5 / 60.0, 0, 0, 1},
		{"the start of the circle", 0, 0, 0, 0},
		{"a full circle is the start of the circle", 360, 0, 0, 0},
		{"below zero wraps backwards round the circle", -1, 11, 29, 0},
		{"past a full circle wraps rather than pinning to the edge", 400, 1, 10, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sign, deg, min := splitLongitude(tc.lon)
			if sign != tc.sign || deg != tc.deg || min != tc.min {
				t.Errorf("splitLongitude(%.17g) = (%d, %d, %d), want (%d, %d, %d)",
					tc.lon, sign, deg, min, tc.sign, tc.deg, tc.min)
			}
			if min < 0 || min > 59 {
				t.Errorf("arcminute %d is not a minute", min)
			}
			if deg < 0 || deg > 29 {
				t.Errorf("degree %d is not a degree of a sign", deg)
			}
			if sign < 0 || sign > 11 {
				t.Errorf("sign index %d is not a sign", sign)
			}
		})
	}
}

// And the row an astrologer reads carries it: the fix is only worth anything
// where it reaches the screen.
func TestBodyRowShowsTheRoundedArcminute(t *testing.T) {
	const virgo = 5
	positions := []aspect.Position{
		{Body: aspect.Sun, Longitude: float64(virgo*30) + 3 + 12.0/60.0},
	}
	planets, _ := bodyRows(positions, nil, false, houseOverlay{})
	if len(planets) != 1 {
		t.Fatalf("bodyRows returned %d rows, want one", len(planets))
	}
	if want := fmt.Sprintf("%s %2d°%02d'", "Vir", 3, 12); !contains(planets[0].text, want) {
		t.Errorf("the Sun row reads %q, want it to carry %q", planets[0].text, want)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		}()
}
