package aspect

import "strings"

// Object is Astrolog's identity for anything it computes: the number and
// the name the binary in use gives it (`astrolog -R1 -HO`). Two Objects are
// the same object when their Numbers are equal; the Name is what files and
// the screen carry.
type Object struct {
	Number int
	Name   string
}

// Registry is the binary's object list as ParseObjectRegistry read it,
// in Astrolog's order. It is the only source of an object's identity.
type Registry []RegistryObject

// ByNumber returns the object with Astrolog number n.
func (r Registry) ByNumber(n int) (Object, bool) {
	for _, o := range r {
		if o.Number == n {
			return o.Object, true
		}
	}
	return Object{}, false
}

// IsStar reports whether the registry's object n is a fixed star (the row
// shape the parser derived, RegistryObject.Star). An unknown number is not.
func (r Registry) IsStar(n int) bool {
	for _, o := range r {
		if o.Number == n {
			return o.Star
		}
	}
	return false
}

// ByName returns the object whose registry name is exactly name. A name two
// entries share resolves to nothing: never a guess (as ByCut).
func (r Registry) ByName(name string) (Object, bool) {
	var found Object
	n := 0
	for _, o := range r {
		if o.Name == name {
			found = o.Object
			n++
		}
	}
	return found, n == 1
}

// ByCut resolves a name as Astrolog prints it in a column of width
// characters, already trimmed of padding. When len(s) < width the name was
// not cut, so only an exact match counts. When len(s) == width the name may
// have been cut, so a registry name equal to s or longer and starting with s
// matches. Exactly one match returns it; zero or several return false, never
// a guess. A width of 0 means exact.
func (r Registry) ByCut(s string, width int) (Object, bool) {
	if width == 0 || len(s) < width {
		return r.ByName(s)
	}
	var found Object
	n := 0
	for _, o := range r {
		if strings.HasPrefix(o.Name, s) {
			found = o.Object
			n++
		}
	}
	if n != 1 {
		return Object{}, false
	}
	return found, true
}

// Cut returns the name as Astrolog prints it in a column of width cells: the
// first width bytes (the registry's names are ASCII) with trailing spaces
// trimmed. It is the inverse of Registry.ByCut; Cut(4) is the label of a
// position row.
func (o Object) Cut(width int) string {
	n := o.Name
	if width >= 0 && len(n) > width {
		n = n[:width]
	}
	return strings.TrimRight(n, " ")
}

// CuspHouse reports the house a house cusp point stands for. Astrolog numbers
// its twelve cusp points 22 (Ascendant, house 1) to 33 (12th Cusp, house 12),
// one per house in order, so house = Number - 21 in that range; any other
// object, and the zero Object, is false.
func CuspHouse(o Object) (house int, ok bool) {
	if o.Number < 22 || o.Number > 33 {
		return 0, false
	}
	return o.Number - 21, true
}

// legacyNames is the only memory of the old body enum's wire names, kept for
// session files and template ids written before objects were Astrolog's own:
// eighteen snake_case names against the registry names of the binary in use
// (never numbers, so a renumbered binary resolves through its own registry).
// Nothing writes these names any more; they are read.
var legacyNames = []struct{ snake, name string }{
	{"sun", "Sun"},
	{"moon", "Moon"},
	{"mercury", "Mercury"},
	{"venus", "Venus"},
	{"mars", "Mars"},
	{"jupiter", "Jupiter"},
	{"saturn", "Saturn"},
	{"uranus", "Uranus"},
	{"neptune", "Neptune"},
	{"pluto", "Pluto"},
	{"north_node", "North Node"},
	{"south_node", "South Node"},
	{"chiron", "Chiron"},
	{"lilith", "Lilith"},
	{"ascendant", "Ascendant"},
	{"midheaven", "Midheaven"},
	{"descendant", "Descendant"},
	{"imum_coeli", "Nadir"},
}

// LegacyName maps an old wire name of the retired body enum ("sun",
// "north_node", "imum_coeli") to the registry name it has in Astrolog's own
// list ("Sun", "North Node", "Nadir").
func LegacyName(snake string) (string, bool) {
	for _, l := range legacyNames {
		if l.snake == snake {
			return l.name, true
		}
	}
	return "", false
}
