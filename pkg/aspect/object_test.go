package aspect_test

import (
	"os"
	"testing"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The fixture pkg/aspect/testdata/ho-all.txt is a copy of
// internal/astrolog/testdata/ho-all.txt (`astrolog -R1 -HO`, 134 objects).
func fixtureRegistry(t *testing.T) aspect.Registry {
	t.Helper()
	b, err := os.ReadFile("testdata/ho-all.txt")
	if err != nil {
		t.Fatal(err)
	}
	reg, err := astrolog.ParseObjectRegistry(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if len(reg) != 134 {
		t.Fatalf("fixture has %d objects, want 134", len(reg))
	}
	return reg
}

func TestByCutResolvesAColumnCut(t *testing.T) {
	reg := fixtureRegistry(t)
	num := func(name string) int {
		o, ok := reg.ByName(name)
		if !ok {
			t.Fatalf("%q not in fixture", name)
		}
		return o.Number
	}
	cases := []struct {
		s     string
		width int
		want  int // -1: no match
	}{
		{"Titan", 7, 58},
		{"Titania", 7, 66},
		{"Zeta Retic", 10, num("Zeta Retic.")},
		{"Rigil K", 7, num("Rigil Kent.")},
		{"Alna", 4, -1},
		{"Sun", 7, 1},
		{"Nonesuch", 10, -1},
		{"Sun", 0, 1},
	}
	for _, c := range cases {
		o, ok := reg.ByCut(c.s, c.width)
		if c.want < 0 {
			if ok {
				t.Errorf("ByCut(%q,%d) = %v, want no match", c.s, c.width, o)
			}
			continue
		}
		if !ok || o.Number != c.want {
			t.Errorf("ByCut(%q,%d) = %v,%v want %d", c.s, c.width, o, ok, c.want)
		}
	}
}

func TestByCutEveryNameResolvesToItself(t *testing.T) {
	reg := fixtureRegistry(t)
	for _, w := range []int{7, 10} {
		for _, r := range reg {
			s := r.Name
			if len(s) > w {
				s = s[:w]
			}
			o, ok := reg.ByCut(s, w)
			if !ok || o.Number != r.Number {
				t.Errorf("width %d: %q (cut %q) -> %v,%v want %d", w, r.Name, s, o, ok, r.Number)
			}
		}
	}
}

func TestByNumberAndByName(t *testing.T) {
	reg := fixtureRegistry(t)
	if o, ok := reg.ByNumber(25); !ok || o.Name != "Nadir" {
		t.Errorf("ByNumber(25) = %v,%v", o, ok)
	}
	if _, ok := reg.ByNumber(999); ok {
		t.Error("ByNumber(999) matched")
	}
	if o, ok := reg.ByName("North Node"); !ok || o.Number != 16 {
		t.Errorf("ByName(North Node) = %v,%v", o, ok)
	}
	if _, ok := reg.ByName("Titan "); ok {
		t.Error("ByName is not exact")
	}
}

// The old body enum's wire names are the only thing LegacyName remembers: the
// EIGHTEEN names session files and template ids may still carry, each to the
// registry name Astrolog gives the object. The list is written out here, not
// derived from the table, so a name dropped from the table fails by name.
func TestLegacyNamesAreTheEighteen(t *testing.T) {
	reg := fixtureRegistry(t)
	want := []struct{ snake, name string }{
		{"sun", "Sun"}, {"moon", "Moon"}, {"mercury", "Mercury"}, {"venus", "Venus"},
		{"mars", "Mars"}, {"jupiter", "Jupiter"}, {"saturn", "Saturn"}, {"uranus", "Uranus"},
		{"neptune", "Neptune"}, {"pluto", "Pluto"}, {"north_node", "North Node"},
		{"south_node", "South Node"}, {"chiron", "Chiron"}, {"lilith", "Lilith"},
		{"ascendant", "Ascendant"}, {"midheaven", "Midheaven"}, {"descendant", "Descendant"},
		{"imum_coeli", "Nadir"},
	}
	if len(want) != 18 {
		t.Fatalf("the list holds %d names, want 18", len(want))
	}
	for _, w := range want {
		name, ok := aspect.LegacyName(w.snake)
		if !ok || name != w.name {
			t.Errorf("LegacyName(%q) = %q, %v; want %q", w.snake, name, ok, w.name)
			continue
		}
		if _, ok := reg.ByName(name); !ok {
			t.Errorf("%q (from %q) is not in the registry", name, w.snake)
		}
	}
	for _, other := range []string{"titan", "unknown", "Sun", "eris", ""} {
		if name, ok := aspect.LegacyName(other); ok {
			t.Errorf("LegacyName(%q) resolved to %q", other, name)
		}
	}
}

func TestCutIsTheInverseOfByCut(t *testing.T) {
	reg := fixtureRegistry(t)
	for _, ro := range reg {
		for _, w := range []int{7, 10} {
			got, ok := reg.ByCut(ro.Object.Cut(w), w)
			if !ok || got != ro.Object {
				t.Errorf("ByCut(%q, %d) = %v, %v; want %v", ro.Object.Cut(w), w, got, ok, ro.Object)
			}
		}
	}
	if got := (aspect.Object{Number: 58, Name: "Titan"}).Cut(4); got != "Tita" {
		t.Errorf("Titan Cut(4) = %q, want Tita", got)
	}
	if got := (aspect.Object{Number: 22, Name: "Ascendant"}).Cut(4); got != "Asce" {
		t.Errorf("Ascendant Cut(4) = %q, want Asce", got)
	}
	if got := (aspect.Object{Number: 1, Name: "Sun"}).Cut(4); got != "Sun" {
		t.Errorf("Sun Cut(4) = %q, want Sun", got)
	}
	if got := (aspect.Object{Number: 99, Name: "Ab  cdef"}).Cut(4); got != "Ab" {
		t.Errorf("a cut ending in spaces = %q, want trailing spaces trimmed", got)
	}
}

func TestCuspHouse(t *testing.T) {
	reg := fixtureRegistry(t)
	for _, c := range []struct {
		n     int
		house int
		name  string
	}{{22, 1, "Ascendant"}, {23, 2, "2nd Cusp"}, {25, 4, "Nadir"}, {28, 7, "Descendant"}, {31, 10, "Midheaven"}, {33, 12, "12th Cusp"}} {
		o, ok := reg.ByNumber(c.n)
		if !ok || o.Name != c.name {
			t.Fatalf("object %d = %v, want %q", c.n, o, c.name)
		}
		if h, ok := aspect.CuspHouse(o); !ok || h != c.house {
			t.Errorf("CuspHouse(%s) = %d, %v; want %d", c.name, h, ok, c.house)
		}
	}
	for _, n := range []int{21, 34} {
		o, _ := reg.ByNumber(n)
		if h, ok := aspect.CuspHouse(o); ok {
			t.Errorf("CuspHouse(%d %s) = %d, true; want false", n, o.Name, h)
		}
	}
	if _, ok := aspect.CuspHouse(aspect.Object{}); ok {
		t.Error("CuspHouse(zero Object) is true, want false")
	}
}

// A name two entries share resolves to nothing: never a guess.
func TestByNameRefusesADuplicateName(t *testing.T) {
	reg := aspect.Registry{
		{Object: aspect.Object{Number: 58, Name: "Titan"}},
		{Object: aspect.Object{Number: 66, Name: "Titan"}},
		{Object: aspect.Object{Number: 1, Name: "Sun"}},
	}
	if o, ok := reg.ByName("Titan"); ok {
		t.Errorf("ByName(Titan) = %v, true; want false for a shared name", o)
	}
	if o, ok := reg.ByName("Sun"); !ok || o.Number != 1 {
		t.Errorf("ByName(Sun) = %v, %v", o, ok)
	}
}

func TestIsStar(t *testing.T) {
	r := aspect.Registry{
		{Object: aspect.Object{Number: 12, Name: "Ceres"}},
		{Object: aspect.Object{Number: 84, Name: "Sirius"}, Star: true},
	}
	for n, want := range map[int]bool{12: false, 84: true, 999: false} {
		if got := r.IsStar(n); got != want {
			t.Errorf("IsStar(%d) = %v, want %v", n, got, want)
		}
	}
}

// TestAspectSidesOrderByNumber: a pair's sides are ordered by Astrolog's own
// object number, whichever side was printed first. The enum put the nodes
// before Chiron and the Midheaven before the Nadir and the Descendant; the
// numbers say Chiron (11) before the nodes (16, 17) and the Nadir (25) and
// Descendant (28) before the Midheaven (31). A side the registry could not
// resolve (the zero Object, whose number 0 is Earth's) is no number to order
// by: the pair keeps its printed order.
func TestAspectSidesOrderByNumber(t *testing.T) {
	reg := fixtureRegistry(t)
	obj := func(name string) aspect.Object {
		o, ok := reg.ByName(name)
		if !ok {
			t.Fatalf("%q not in fixture", name)
		}
		return o
	}
	chiron, node, nadir, desc, mc, sun := obj("Chiron"), obj("North Node"), obj("Nadir"), obj("Descendant"), obj("Midheaven"), obj("Sun")
	for _, tc := range []struct {
		name         string
		a, b         aspect.Object
		first, other aspect.Object
	}{
		{"Chiron against the North Node, printed node first", node, chiron, chiron, node},
		{"Chiron against the North Node, printed Chiron first", chiron, node, chiron, node},
		{"the Midheaven against the Nadir, printed MC first", mc, nadir, nadir, mc},
		{"the Nadir against the Midheaven, printed Nadir first", nadir, mc, nadir, mc},
		{"the Midheaven against the Descendant", mc, desc, desc, mc},
		{"the Sun against Chiron, printed Chiron first", chiron, sun, sun, chiron},
		{"unresolved first keeps the printed order", aspect.Object{}, sun, aspect.Object{}, sun},
		{"unresolved second keeps the printed order", mc, aspect.Object{}, mc, aspect.Object{}},
		{"two unresolved sides keep the printed order", aspect.Object{}, aspect.Object{}, aspect.Object{}, aspect.Object{}},
		{"the same object twice keeps the printed order", sun, sun, sun, sun},
	} {
		got := aspect.NewObjectAspect(tc.a, tc.b, kindOf("Opp"), 1)
		if got.AObject != tc.first || got.BObject != tc.other {
			t.Errorf("%s: sides %+v and %+v, want %+v and %+v", tc.name, got.AObject, got.BObject, tc.first, tc.other)
		}
	}
}
