package lookup

import "github.com/cruiserxyz/sextile/pkg/aspect"

// fasterOrder is the order of objects by mean daily motion, fastest first,
// keyed on Astrolog's object number (internal/astrolog/testdata/ho-all.txt:
// Moon 2, Mercury 3, Venus 4, Sun 1, Mars 5, Vesta 15, Juno 14, Ceres 12,
// Pallas 13, Jupiter 6, Saturn 7, Uranus 8, Neptune 9, Pluto 10, Chiron 11,
// Lilith 18, North Node 16, South Node 17). The four asteroids sit between
// Mars and Jupiter by their mean motions (Vesta 0.27, Juno 0.23, Ceres 0.21,
// Pallas 0.21 degrees a day). It is an arrangement rule, a class fact of the
// object, not a calculation: no speed is read from any position (the chart
// call's `=v0` makes Position.Speed a ratio to the body's own average, which
// cannot rank two bodies), and the positions stay Astrolog's. An object not in
// the table ranks after every object in it.
var fasterOrder = []int{2, 3, 4, 1, 5, 15, 14, 12, 13, 6, 7, 8, 9, 10, 11, 18, 16, 17}

func fasterRank(o aspect.Object) int {
	for i, n := range fasterOrder {
		if n == o.Number {
			return i
		}
	}
	return len(fasterOrder)
}

// FasterObject is the faster of two objects by the order above. A tie, or two
// objects the table does not hold, keeps first (Astrolog's first object).
func FasterObject(first, second aspect.Object) aspect.Object {
	if fasterRank(second) < fasterRank(first) {
		return second
	}
	return first
}

// LeadObject is the object a timeline row leads the Bodies cursor to: the
// moving object of an ingress or a station, the transiting object of a chart
// row (the natal object or point is never the target), and the faster object
// of a sky aspect. ok is false only for a row that is not resolved.
func LeadObject(r TimelineRow) (aspect.Object, bool) {
	if !r.Resolved {
		return aspect.Object{}, false
	}
	if r.Kind == TimelineKindAspect && !r.Transit {
		return FasterObject(r.Object, r.OtherObject), true
	}
	return r.Object, true
}
