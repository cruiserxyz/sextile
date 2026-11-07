package aspect

import "math"

// Position is a body's ecliptic location at a moment in time.
//
// Astrolog source: astrolog emits longitude, latitude, and speed per body in
// its standard chart output. Longitude is measured from 0° Aries, wrapping at
// 360°; latitude is geocentric ecliptic, in [-90, 90]. Speed is signed
// (negative = retrograde); its MAGNITUDE depends on the invocation — under the
// `-v0` path Sextile uses it is a ratio to the body's average daily motion
// (1.0 = average), not raw degrees per day. Sign semantics hold either way.
type Position struct {
	Longitude float64 // degrees in [0, 360)
	Latitude  float64 // degrees in [-90, 90]
	Speed     float64 // signed; magnitude is the -v0 ratio (see type doc)
	House     House   // 1..12; 0 if unset (parser dropped it or no house data)

	// Object is Astrolog's own identity for the row: its number and the name
	// the registry of the binary in use gives it. Every parsed row carries one,
	// whatever the object (the Sun, Ceres, Vertex, Titan).
	Object Object
}

// Retrograde reports whether the body is moving retrograde at this position.
// Derived from Speed so retrograde state is never stored separately and
// cannot drift out of sync with the underlying motion. It reads the speed's
// SIGN BIT, not `< 0`: Astrolog prints R through the station minute at a speed
// of -0.000, and the parser keeps that as negative zero, so only the sign bit
// agrees with Astrolog's own R column (the specification C11).
//
// derivation: retrograde from the speed's sign
func (p Position) Retrograde() bool {
	return math.Signbit(p.Speed)
}

// Orb is the absolute angular deviation between the actual separation of two
// bodies and the exact angle of an aspect, in degrees. Always non-negative.
// Narrower orbs indicate tighter aspects.
type Orb float64
