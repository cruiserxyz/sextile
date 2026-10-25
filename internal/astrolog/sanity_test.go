package astrolog

import (
	"errors"
	"testing"
)

// The real fixture contains one body at 0°Ari55' — within the 0Ari-range
// but not matching the bounded 0Ari00 forbid pattern. One false positive
// would already fail the test; this is the deliberately-close true case.
func TestCheckPositionSanityAcceptsGoodOutput(t *testing.T) {
	good := `Sun :  0Ari55   - 0:00' (X) [ 8th house] [-] +0.993
Moon:  2Ari04   - 3:45' (-) [ 8th house] [f] +11.93
Merc: 17Ari59   + 2:57' (-) [ 9th house] [d] +0.421
`
	if err := checkPositionSanity(good); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// The canonical symptom: Swiss Ephemeris disabled — every body collapses
// to 0Ari00. Two matches are enough to trip the predicate (a single real
// body at 0° Aries 0' is rare but permitted).
func TestCheckPositionSanityRejectsEphemerisDisabled(t *testing.T) {
	bad := `Sun :  0Ari00   - 0:00' (X) [ 8th house] [-] +0.993
Moon:  0Ari00   - 3:45' (-) [ 8th house] [f] +11.93
Merc:  0Ari00   + 2:57' (-) [ 9th house] [d] +0.421
Venu:  0Ari00 R + 7:55' (d) [ 9th house] [-] -0.320
`
	err := checkPositionSanity(bad)
	if !errors.Is(err, ErrEphemerisDisabled) {
		t.Errorf("expected ErrEphemerisDisabled, got %v", err)
	}
}

// A single body at 0Ari00 is legitimate (a conjunction with the vernal
// point). The predicate must not false-positive on it.
func TestCheckPositionSanityAcceptsSingleZeroAries(t *testing.T) {
	borderline := `Sun :  0Ari00   - 0:00' (X) [ 8th house] [-] +0.993
Moon:  2Ari04   - 3:45' (-) [ 8th house] [f] +11.93
`
	if err := checkPositionSanity(borderline); err != nil {
		t.Errorf("expected nil for single 0Ari00, got %v", err)
	}
}
