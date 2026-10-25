package astrolog

import (
	"math"
	"testing"
	"time"
)

func TestGregorianToJD(t *testing.T) {
	cases := []struct {
		name string
		in   time.Time
		want float64
	}{
		// Known canonical fixtures from Meeus 2e ch. 7 §Examples.
		{"J2000.0 epoch", time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC), 2451545.0},
		{"1999-01-01 00:00 UTC", time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC), 2451179.5},
		{"1987-01-27 00:00 UTC", time.Date(1987, 1, 27, 0, 0, 0, 0, time.UTC), 2446822.5},
		{"1957-10-04 19:28:34 UTC (Sputnik 1 launch)", time.Date(1957, 10, 4, 19, 28, 34, 0, time.UTC), 2436116.31150},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := GregorianToJD(c.in)
			if math.Abs(got-c.want) > 1e-4 {
				t.Errorf("GregorianToJD(%s) = %.6f, want %.6f", c.in, got, c.want)
			}
		})
	}
}

// TestGregorianToJD_NormalisesToUTC asserts that a time in a non-UTC zone
// produces the same JD as its UTC equivalent.
func TestGregorianToJD_NormalisesToUTC(t *testing.T) {
	utc := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	cest := time.FixedZone("CEST", 2*3600)
	local := time.Date(2026, 4, 25, 12, 0, 0, 0, cest) // same instant, civil 12:00 CEST
	if math.Abs(GregorianToJD(utc)-GregorianToJD(local)) > 1e-9 {
		t.Errorf("expected identical JD across timezones for the same instant")
	}
}
