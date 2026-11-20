package astrolog

import "testing"

func TestFormatLon(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{13.4105, "13:25E"},    // Berlin
		{-122.4194, "122:25W"}, // San Francisco
		{0, "0:00E"},
		{151.2, "151:12E"}, // Sydney
		{-0.1278, "0:08W"}, // London (just west of Greenwich)
	}
	for _, c := range cases {
		if got := formatLon(c.in); got != c.want {
			t.Errorf("formatLon(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatLat(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{52.5244, "52:31N"},  // Berlin
		{-33.8688, "33:52S"}, // Sydney
		{0, "0:00N"},
		{37.7749, "37:46N"}, // San Francisco
	}
	for _, c := range cases {
		if got := formatLat(c.in); got != c.want {
			t.Errorf("formatLat(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSplitDegMin_RoundingCarry(t *testing.T) {
	// 0.999... degrees rounds to 60 arcminutes, must carry to 1°00'.
	deg, min := splitDegMin(0.99999)
	if deg != 1 || min != 0 {
		t.Errorf("splitDegMin(0.99999) = (%d, %d), want (1, 0)", deg, min)
	}
}
