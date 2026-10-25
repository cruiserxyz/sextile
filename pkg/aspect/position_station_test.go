package aspect

import (
	"math"
	"testing"
)

// TestRetrogradeFollowsAstrologsSign: Astrolog prints R in the station minute
// at a speed of exactly -0.000, and the parser keeps that as negative zero. The
// sign bit, not `< 0`, is what says "Astrolog printed R"
// (the specification C11).
func TestRetrogradeFollowsAstrologsSign(t *testing.T) {
	negZero := math.Copysign(0, -1)
	cases := []struct {
		name  string
		speed float64
		want  bool
	}{
		{"-0.000 is retrograde", negZero, true},
		{"+0.000 is direct", 0, false},
		{"-0.320 is retrograde", -0.32, true},
		{"+0.420 is direct", 0.42, false},
	}
	for _, c := range cases {
		if got := (Position{Body: Mercury, Speed: c.speed}).Retrograde(); got != c.want {
			t.Errorf("%s: Retrograde() = %v, want %v", c.name, got, c.want)
		}
	}
}
