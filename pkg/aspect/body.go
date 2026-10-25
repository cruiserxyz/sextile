package aspect

// Body enumerates the celestial bodies astrolog can report on.
//
// Astrolog source: astrolog numbers objects internally in the order documented
// under "Objects" in the astrolog 7.x manual. The constants below cover the
// classical ten (Sun–Pluto), the lunar nodes, Chiron, Black Moon Lilith, and the
// four angles (Ascendant, Midheaven, Descendant, Imum Coeli). Additional bodies
// land as coverage grows.
//
// The zero value (BodyUnknown) is not a valid body; a Body read from
// untrusted input should be compared against BodyUnknown before use.
type Body uint8

const (
	BodyUnknown Body = iota
	Sun
	Moon
	Mercury
	Venus
	Mars
	Jupiter
	Saturn
	Uranus
	Neptune
	Pluto
	NorthNode
	SouthNode
	Chiron
	Lilith
	Ascendant
	Midheaven
	Descendant
	ImumCoeli
)

// String returns a stable lowercase snake_case name for the body.
//
// Since an earlier change these names ARE a wire format: the session file names the
// operator's selected bodies with them, and ParseBody reads them back. Baked
// tables and Astrolog-facing output still speak the enum's numeric value, so
// this is the one place a body crosses a file boundary as text. Renaming a
// body here silently drops that body from every session file already on disk.
func (b Body) String() string {
	switch b {
	case Sun:
		return "sun"
	case Moon:
		return "moon"
	case Mercury:
		return "mercury"
	case Venus:
		return "venus"
	case Mars:
		return "mars"
	case Jupiter:
		return "jupiter"
	case Saturn:
		return "saturn"
	case Uranus:
		return "uranus"
	case Neptune:
		return "neptune"
	case Pluto:
		return "pluto"
	case NorthNode:
		return "north_node"
	case SouthNode:
		return "south_node"
	case Chiron:
		return "chiron"
	case Lilith:
		return "lilith"
	case Ascendant:
		return "ascendant"
	case Midheaven:
		return "midheaven"
	case Descendant:
		return "descendant"
	case ImumCoeli:
		return "imum_coeli"
	default:
		return "unknown"
	}
}

// ParseBody returns the Body whose String() equals name, so that every body
// other than BodyUnknown round-trips through the session file. Matching is
// exact: a caller that read the name out of a file trims it first.
//
// The scan runs the enum rather than a parallel table on purpose. A second
// list of names would be a thing to forget when a body is added, and the
// failure would be silent — a body that writes fine and never reads back.
func ParseBody(name string) (Body, bool) {
	if name == "" {
		return BodyUnknown, false
	}
	for b := Sun; b.String() != BodyUnknown.String(); b++ {
		if b.String() == name {
			return b, true
		}
	}
	return BodyUnknown, false
}
