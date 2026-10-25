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

// Aspect pairs two bodies under a named angular relationship, with the
// observed orb (angular deviation from the exact angle).
//
// Ordering invariant: A <= B in Body enum order. NewAspect enforces this so
// (Venus, Mars) and (Mars, Venus) collapse to a single canonical pair and
// consumers can dedupe without a normalisation step. Applying/separating is a
// property of the pair (the orb closing or opening), so it does not depend on
// A/B order — it is Astrolog's own verdict, read from the -aa list.
type Aspect struct {
	A    Body
	B    Body
	Kind AspectKind
	Orb  Orb

	// ALabel and BLabel are the names Astrolog printed for a side this enum
	// cannot name, and are empty for every body it can. The enum stays
	// authoritative: nothing reads a label when its Body is set. They are what
	// let an aspect to Ceres or Vertex be kept instead of dropped; such a side
	// is BodyUnknown, renders, and is not addressable as a typed body. Labels
	// travel with their body through NewLabelledAspect's A<=B reorder.
	ALabel string
	BLabel string

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

// NewAspect returns an Aspect with A and B reordered so A <= B in Body enum
// order. The only constructor in this package that enforces an invariant
// downstream code relies on.
func NewAspect(a, b Body, kind AspectKind, orb Orb) Aspect {
	if b < a {
		a, b = b, a
	}
	return Aspect{A: a, B: b, Kind: kind, Orb: orb}
}

// NewLabelledAspect is NewAspect for sides that may be untyped: each body comes
// with its label (empty for a typed body), and the A<=B reorder swaps the
// labels with the bodies so a label never lands on the wrong side. Two untyped
// sides (both BodyUnknown) keep their printed order.
func NewLabelledAspect(a Body, aLabel string, b Body, bLabel string, kind AspectKind, orb Orb) Aspect {
	if b < a {
		a, b = b, a
		aLabel, bLabel = bLabel, aLabel
	}
	out := Aspect{A: a, B: b, Kind: kind, Orb: orb}
	out.ALabel, out.BLabel = aLabel, bLabel
	return out
}
