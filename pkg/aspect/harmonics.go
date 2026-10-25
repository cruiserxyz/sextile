package aspect

import "math"

// harmonics.go holds the two angle facts the aspect chooser needs, here rather
// than in the UI because an angle is an astrological fact and `internal/ui`
// must not carry one as a literal.

// angleTolerance is how close a registry angle must be to a kind's canonical
// angle to be that kind. `astrolog -HA` prints angles rounded to two decimals —
// the harmonic aspects read 51.43, 102.86, 130.91 — so an exact float compare
// would be a coin toss. Every one of the eleven named kinds is an exact decimal
// and matches well inside this.
const angleTolerance = 0.01

// KindForAngle maps a separation in degrees to the AspectKind Sextile names for
// it. The second return is false when no kind matches within tolerance, which
// is the ordinary case for most of Astrolog's registry: it carries 24 aspects
// and this enum names 11. A non-match maps to NO kind — never to the nearest
// one, which would silently relabel a septile as a sextile.
func KindForAngle(deg float64) (AspectKind, bool) {
	for _, k := range AllKinds() {
		if math.Abs(k.Angle()-deg) <= angleTolerance {
			return k, true
		}
	}
	return AspectUnknown, false
}

// AllKinds lists every real AspectKind, ascending by angle. AspectUnknown is
// not a kind and is absent.
func AllKinds() []AspectKind {
	return []AspectKind{
		Conjunction, Semisextile, Semisquare, Sextile, Quintile, Square,
		Trine, Sesquisquare, Biquintile, Quincunx, Opposition,
	}
}

// CircleDivision is the harmonic family an angle belongs to: the SMALLEST n
// such that the angle is a whole multiple of 360/n. It is not 360/angle — the
// sesquiquadrate at 135° is three eighths of the circle, so its family is 8,
// and 360/135 is not a whole number at all. Getting that backwards drops every
// aspect that is not a unit fraction out of its own group.
//
//	  0° → 1    180° → 2    120° → 3     90° → 4     72° → 5
//	 60° → 6   51.43° → 7    45° → 8     40° → 9     36° → 10
//	32.73° → 11  30° → 12   135° → 8    144° → 5    150° → 12
//
// Groups are defined over this and never over names, because a user's binary
// may rename an aspect (`-YAD` does exactly that) but cannot move it.
//
// The second return is false for an angle that belongs to no family within
// maxHarmonic — which, for a registry Astrolog itself tops out at the 11th
// harmonic, should not happen, and is reported rather than guessed.
func CircleDivision(deg float64) (int, bool) {
	if deg < 0 || deg > 360 {
		return 0, false
	}
	if deg == 0 {
		return 1, true // the conjunction: one whole circle
	}
	for n := 1; n <= maxHarmonic; n++ {
		turns := deg * float64(n) / 360
		if math.Abs(turns-math.Round(turns)) <= harmonicTolerance {
			return n, true
		}
	}
	return 0, false
}

// maxHarmonic bounds the family search. Astrolog's own registry stops at the
// 11th harmonic; the headroom is so a richer binary still resolves.
const maxHarmonic = 32

// harmonicTolerance absorbs the two-decimal rounding in `-HA`: a septile prints
// as 51.43 where the exact value is 51.428571…, so the seventh-harmonic test
// lands at 0.99997 turns rather than exactly 1.
const harmonicTolerance = 0.002

// Family is one of the five groups the aspect chooser pins above its list.
//
// A family is a PREDICATE over the harmonic, never a list of names or indices,
// so its membership is whatever the live registry happens to contain. Legacy
// showed "All (31)" against its binary; this one has 24. That difference is the
// argument for the predicate — a constant would have been wrong for one of
// them.
//
// The families overlap by design: Ptolemaic is a subset of Major, Hard Minor is
// the rest of Major, and Harmonic is disjoint from both. Toggling Ptolemaic off
// while Major is on leaves Major partial, which is what a tri-state marker is
// for.
type Family int

const (
	// FamilyPtolemaic is the classical five: conjunction, opposition,
	// trine, square, sextile.
	FamilyPtolemaic Family = iota
	// FamilyMajor adds the eighth and twelfth harmonics to the Ptolemaic
	// five — the aspects traditional practice treats as hard.
	FamilyMajor
	// FamilyHardMinor is Major minus Ptolemaic: the semisquare,
	// sesquiquadrate, semisextile and inconjunct.
	FamilyHardMinor
	// FamilyHarmonic is the fifth, seventh, ninth, tenth and eleventh
	// harmonics — disjoint from Major.
	FamilyHarmonic
	// FamilyAll is every aspect the registry carries, including any whose
	// harmonic matches none of the above. The list is the authority; the
	// families are shortcuts into it.
	FamilyAll
)

// Families returns the five in display order.
func Families() []Family {
	return []Family{FamilyPtolemaic, FamilyMajor, FamilyHardMinor, FamilyHarmonic, FamilyAll}
}

// Name is the row label.
func (f Family) Name() string {
	switch f {
	case FamilyPtolemaic:
		return "Ptolemaic"
	case FamilyMajor:
		return "Major"
	case FamilyHardMinor:
		return "Hard Minor"
	case FamilyHarmonic:
		return "Harmonic"
	case FamilyAll:
		return "All"
	}
	return ""
}

// Contains reports whether an aspect at this separation belongs to the family.
func (f Family) Contains(deg float64) bool {
	if f == FamilyAll {
		return true
	}
	n, ok := CircleDivision(deg)
	if !ok {
		// An angle with no harmonic family belongs to no group but still
		// appears under All, which is why FamilyAll short-circuits above.
		return false
	}
	switch f {
	case FamilyPtolemaic:
		return n == 1 || n == 2 || n == 3 || n == 4 || n == 6
	case FamilyMajor:
		return n == 1 || n == 2 || n == 3 || n == 4 || n == 6 || n == 8 || n == 12
	case FamilyHardMinor:
		return n == 8 || n == 12
	case FamilyHarmonic:
		return n == 5 || n == 7 || n == 9 || n == 10 || n == 11
	}
	return false
}
