package aspect

// registry.go carries the shape of one row of the user's Astrolog aspect
// registry. The PARSE lives in internal/astrolog, which is the only package
// allowed to run the binary; the TYPE lives here because `internal/ui` renders
// these rows and may not import an active package (the isolation invariant).
// Keeping the vocabulary passive is what lets both sides speak it.

// RegistryAspect is one row of `astrolog -HA`.
type RegistryAspect struct {
	// Index is Astrolog's own registry index, 1-based and ordered by
	// importance. It is what the `-RA0` band emits, and it is NOT an
	// aspect.AspectKind.
	Index int
	Name  string
	// Abbrev is the three-letter form the aspect waterfall prints, which is
	// the bridge between a row here and a row there.
	Abbrev string
	// Angle is the separation in degrees, as printed — rounded to two
	// decimals, so the harmonic aspects read 51.43, 102.86, 130.91.
	Angle float64
	// Orb is the EFFECTIVE orb. Zero means the aspect cannot be reported at
	// all, whatever the restriction band says.
	Orb float64
	// InForce reports whether the binary COUNTS this aspect in the state
	// `-HA` was run in, read from that output's trailing "Unrestricted
	// aspects" section. It is what the host's own astrolog.as has decided,
	// and it is the set the chooser must open on when the operator has not
	// overridden anything.
	//
	// It is orthogonal to Orb in both directions. An aspect can be
	// unrestricted with an effective orb of zero — `-Ao 5 0 -HA` prints the
	// sextile in the unrestricted section as `covers 60 to 60 - span 0` —
	// and it can carry a wide orb while being restricted out, which is the
	// ordinary case: a default astrolog.as counts five of twenty-four and
	// every one of the other nineteen still prints a positive orb.
	//
	// Reading the orb instead of this field is how the chooser came to claim
	// the host counted all twenty-four: the first toggle then
	// confirmed that claim as an explicit policy and silently widened the
	// chart the operator was looking at.
	InForce bool
}

// CountsAnything reports whether selecting this aspect could actually produce a
// row. An orb of zero is how a host astrolog.as disables an aspect, and it
// survives every restriction switch, so a chooser that does not surface it
// shows a ticked row that yields nothing.
func (a RegistryAspect) CountsAnything() bool { return a.Orb > 0 }
