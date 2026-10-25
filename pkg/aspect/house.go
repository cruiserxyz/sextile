package aspect

import "math"

// House is the astrological house number, 1..12.
//
// Astrolog source: astrolog computes houses per the house system currently
// configured (Placidus, Koch, Campanus, Whole-Sign, Equal, …). The House type
// is system-agnostic — it is only the slot; the choice of system is a user
// preference held by the store and threaded into the IR during bind.
//
// The zero value is an invalid house; callers should treat any House outside
// the 1..12 range as malformed.
type House uint8

// Valid reports whether h is within the canonical 1..12 range.
func (h House) Valid() bool {
	return h >= 1 && h <= 12
}

// Cusps holds the twelve house cusp longitudes of one chart, in degrees
// [0, 360), index 0 being house 1. It is a chart's house *frame* — the thing
// a longitude is measured against.
//
// A body's longitude alone says nothing about houses: the same degree falls
// in different houses depending on whose cusps it is placed into. Carrying
// the frame explicitly is what lets one chart's body be located in another
// chart's houses — the difference between "where is this body in a chart
// cast for now" and "which of the client's houses is it transiting".
//
// System-agnostic by construction: under Whole Sign the cusps land on sign
// boundaries, under Placidus they do not, and HouseFor does not care.
type Cusps [12]float64

// Valid reports whether c looks like a real frame: every value in [0, 360)
// and not all zero. The zero value means "no frame available", which callers
// must treat as "cannot place a longitude", never as house 1.
func (c Cusps) Valid() bool {
	allZero := true
	for _, v := range c {
		if v < 0 || v >= 360 {
			return false
		}
		if v != 0 {
			allZero = false
		}
	}
	return !allZero
}

// HouseFor returns the house a longitude occupies in this frame, or 0 when
// the frame is unusable. It never guesses: an invalid frame yields an invalid
// house, so a caller that forgets to check renders nothing rather than
// something plausible and wrong.
//
// A house runs from its own cusp up to (not including) the next, and may wrap
// through 0° Aries — a house spanning 350°→10° is ordinary, not an edge case.
// The cusp degree itself belongs to the house it opens.
//
// derivation: house of a transiting body in the client's frame
func (c Cusps) HouseFor(longitude float64) House {
	if !c.Valid() {
		return 0
	}
	lon := norm360(longitude)
	for i := 0; i < 12; i++ {
		start := norm360(c[i])
		span := norm360(norm360(c[(i+1)%12]) - start)
		if span == 0 {
			continue
		}
		if norm360(lon-start) < span {
			return House(i + 1)
		}
	}
	return 0
}

// norm360 folds a degree value into [0, 360).
func norm360(d float64) float64 {
	d = math.Mod(d, 360)
	if d < 0 {
		d += 360
	}
	return d
}
