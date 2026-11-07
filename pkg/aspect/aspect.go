package aspect

// AspectKind enumerates the angular relationships astrolog reports between
// pairs of bodies.
//
// Astrolog source: this enum tracks astrolog's default aspect set — the five
// classical majors (conjunction, sextile, square, trine, opposition) plus the
// common Ptolemaic-adjacent minors (semisextile, semisquare, quintile,
// sesquisquare, biquintile, quincunx). Astrolog's aspect list is runtime-
// configurable; the subset here is the one whose meaning the bake pipeline has
// baked. The zero value (AspectUnknown) is reserved.
type AspectKind uint8

const (
	AspectUnknown AspectKind = iota
	Conjunction
	Semisextile
	Semisquare
	Sextile
	Quintile
	Square
	Trine
	Sesquisquare
	Biquintile
	Quincunx
	Opposition
)

// Angle returns the canonical angular separation of the aspect kind in
// degrees. Returns -1 for AspectUnknown.
func (k AspectKind) Angle() float64 {
	switch k {
	case Conjunction:
		return 0
	case Semisextile:
		return 30
	case Semisquare:
		return 45
	case Sextile:
		return 60
	case Quintile:
		return 72
	case Square:
		return 90
	case Trine:
		return 120
	case Sesquisquare:
		return 135
	case Biquintile:
		return 144
	case Quincunx:
		return 150
	case Opposition:
		return 180
	}
	return -1
}

// String returns a stable lowercase name for the aspect kind.
func (k AspectKind) String() string {
	switch k {
	case Conjunction:
		return "conjunction"
	case Semisextile:
		return "semisextile"
	case Semisquare:
		return "semisquare"
	case Sextile:
		return "sextile"
	case Quintile:
		return "quintile"
	case Square:
		return "square"
	case Trine:
		return "trine"
	case Sesquisquare:
		return "sesquisquare"
	case Biquintile:
		return "biquintile"
	case Quincunx:
		return "quincunx"
	case Opposition:
		return "opposition"
	default:
		return "unknown"
	}
}

// Aspect pairs two objects under a named angular relationship, with the
// observed orb (angular deviation from the exact angle).
//
// Ordering invariant: AObject.Number <= BObject.Number, Astrolog's own object
// numbers. NewObjectAspect enforces this so (Venus, Mars) and (Mars, Venus)
// collapse to a single canonical pair and consumers can dedupe without a
// normalisation step; a side the registry could not resolve (the zero Object)
// has no number, so a pair with one keeps the order it was printed in.
// Applying/separating is a property of the pair (the orb closing or opening),
// so it does not depend on side order — it is Astrolog's own verdict, read from
// the -aa list.
type Aspect struct {
	Kind AspectKind
	Orb  Orb

	// AObject and BObject are Astrolog's objects for the two sides, resolved
	// through the registry; the zero Object for a side the registry could not
	// resolve.
	AObject Object
	BObject Object

	// Power is astrolog's aspect strength (the `-a` "power:" column), used to
	// rank the aspect waterfall. Zero when unknown (e.g. cross-aspects computed
	// locally before a proxy is assigned). Higher = stronger.
	Power float64

	// Applying reports whether the aspect is tightening (orb decreasing toward
	// exact). It is Astrolog's own verdict — the `a`/`s` marker of the -aa
	// aspect list, computed from the bodies' velocities at the chart moment
	// — never derived in Sextile. Zero value (false) = separating, or
	// unknown for a list parsed without -aa (plain +/- orbs).
	Applying bool
}

// NewObjectAspect returns an Aspect for two sides given as Astrolog objects,
// ordered by object number: the lower number is the first side. Two resolved
// sides with the same number, and any pair with a zero (unresolved) side, keep
// the order given:
// the zero Object's number 0 is Earth's, so it says nothing about which side
// comes first, and the printed order is the one thing known.
func NewObjectAspect(a, b Object, kind AspectKind, orb Orb) Aspect {
	if a != (Object{}) && b != (Object{}) && b.Number < a.Number {
		a, b = b, a
	}
	return Aspect{AObject: a, BObject: b, Kind: kind, Orb: orb}
}
