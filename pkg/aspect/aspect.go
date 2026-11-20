package aspect

// AspectKind is an aspect's identity: the index, the name and the three
// letters of the registry row Astrolog printed for it (`astrolog -HA`). The
// package holds no list of aspects; the registry of the binary in use is the
// only one, and a kind is read from it. Two kinds are the same aspect when
// they are equal. The zero value means no aspect.
type AspectKind struct {
	// Index is Astrolog's own registry index, 1-based.
	Index int
	// Name is the registry's name for the aspect.
	Name string
	// Abbrev is the three-letter form Astrolog prints in its lists.
	Abbrev string
}

// Known reports whether the kind names an aspect (the zero value does not).
func (k AspectKind) Known() bool { return k.Index > 0 }

// String returns the registry's name for the aspect.
func (k AspectKind) String() string { return k.Name }

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
