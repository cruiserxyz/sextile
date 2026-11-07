package astrolog

import (
	"context"
	"os/exec"
	"testing"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// colourFixtureAsked is the call the colour fixture was recorded from:
// `astrolog -qj 2451545.5 -zl 0 52 -z 0 -R0 1 2 13 19 22 58 66 -k`, which
// printed one colored row per asked object in ascending number (Sun, Moon,
// Pallas, Fortune, Ascendant, Titan, Titania; the last two both read `Tita`).
var colourFixtureAsked = []int{66, 1, 22, 2, 58, 19, 13, 1}

func colourFixtureRows(t *testing.T) astrologstyle.Rows {
	t.Helper()
	rows, err := astrologstyle.ParseBodies(readFixture(t, "k-rows-by-order.txt"))
	if err != nil {
		t.Fatalf("ParseBodies: %v", err)
	}
	if len(rows) != 7 {
		t.Fatalf("the fixture holds %d colored rows, want 7", len(rows))
	}
	return rows
}

// TestBodyColoursByRowOrder: a colored row takes its identity from its place
// among the asked objects, as a position does, never from its four-cell label:
// Titan and Titania both print `Tita` and each gets its own row's colours, so
// does Fortune, which has no glyph.
func TestBodyColoursByRowOrder(t *testing.T) {
	rows := colourFixtureRows(t)
	got := mapBodyStyles(rows, colourFixtureAsked, testRegistry(t))
	for _, tc := range []struct {
		num  int
		name string
		row  int // the place of its row in the printed order
	}{
		{1, "Sun", 0}, {2, "Moon", 1}, {13, "Pallas", 2}, {19, "Fortune", 3}, {22, "Ascendant", 4}, {58, "Titan", 5}, {66, "Titania", 6},
	} {
		o, ok := testRegistry(t).ByNumber(tc.num)
		if !ok || o.Name != tc.name {
			t.Fatalf("registry object %d = %+v, want %s", tc.num, o, tc.name)
		}
		if row, ok := got[o]; !ok || row != rows[tc.row] {
			t.Errorf("%s (object %d): style %+v (present %v), want row %d's %+v", tc.name, tc.num, row, ok, tc.row, rows[tc.row])
		}
	}
	if rows[5] == rows[6] {
		t.Fatalf("the fixture is no test: Titan's and Titania's rows are the same style %+v", rows[5])
	}
	if len(got) != 7 {
		t.Errorf("%d objects coloured, want the 7 asked", len(got))
	}
}

// TestBodyColoursCountMismatchGivesNone: a row count that differs from the
// asked count means the rows cannot be told apart from the objects, so no
// object gets a colour; a guess would colour a body with another's row.
func TestBodyColoursCountMismatchGivesNone(t *testing.T) {
	rows := colourFixtureRows(t)
	reg := testRegistry(t)
	for name, asked := range map[string][]int{
		"one asked too few":  {1, 2, 13, 19, 22, 58},
		"one asked too many": {1, 2, 13, 19, 22, 58, 66, 67},
		"none asked":         nil,
	} {
		if got := mapBodyStyles(rows, asked, reg); len(got) != 0 {
			t.Errorf("%s: %d objects coloured, want none: %+v", name, len(got), got)
		}
	}
	if got := mapBodyStyles(rows, []int{1, 2, 13, 19, 22, 58, 66}, reg); len(got) != 7 {
		t.Errorf("a matching count coloured %d objects, want 7", len(got))
	}
}

// TestEarthIsAStyleKeyTheZeroObjectIsNot: Earth is Astrolog's object 0 and a
// resolved side with a name of its own, so it keys a style; the zero Object,
// which is what an unresolved side resolves to, keys nothing and never takes
// Earth's style.
func TestEarthIsAStyleKeyTheZeroObjectIsNot(t *testing.T) {
	reg := testRegistry(t)
	earth, ok := reg.ByNumber(0)
	if !ok || earth == (aspect.Object{}) {
		t.Fatalf("object 0 = %+v (%v), want Earth with its name", earth, ok)
	}
	mars, _ := reg.ByNumber(5)
	bodies := mapBodyStyles(astrologstyle.Rows{{Body: "earth"}, {Body: "mars"}}, []int{0, 5}, reg)
	if row, ok := bodies[earth]; !ok || row.Body != "earth" {
		t.Errorf("Earth's body style = %+v (present %v), want its own row", row, ok)
	}
	if _, ok := bodies[aspect.Object{}]; ok {
		t.Errorf("the zero Object is a body style key: %+v", bodies)
	}

	rows := []astrologstyle.AspectRow{
		{FirstName: "Earth", KindName: "Con", SecondName: "Mars", Style: astrologstyle.AspectStyle{First: "earth", Kind: "con", Second: "mars"}},
		{FirstName: "Zork", KindName: "Con", SecondName: "Mars", Style: astrologstyle.AspectStyle{First: "zork", Kind: "con", Second: "mars"}},
		{FirstName: "Mars", KindName: "Squ", SecondName: "Nowhere", Style: astrologstyle.AspectStyle{First: "mars", Kind: "squ", Second: "nowhere"}},
	}
	for name, got := range map[string]astrologstyle.Aspects{
		"single": mapAspectStyles(rows, reg),
		"cross":  mapCrossAspectStyles(rows, reg),
	} {
		if len(got) != 1 {
			t.Errorf("%s: %d aspect styles, want Earth-Mars alone: %+v", name, len(got), got)
		}
		if style := got[astrologstyle.AspectIdentity{First: earth, Second: mars, Kind: aspect.Conjunction}]; style.First != "earth" {
			t.Errorf("%s: Earth-Mars style = %+v, want its own row's", name, style)
		}
		for id := range got {
			if id.First == (aspect.Object{}) || id.Second == (aspect.Object{}) {
				t.Errorf("%s: a style key has an unresolved side: %+v", name, id)
			}
		}
	}
}

// TestRunColoursEveryObject: the whole path, a real call asking for Titan and
// Titania beside the others (a probing call: whether or not the share holds
// the two moons' ephemeris files, Astrolog prints their colored rows). Run zips
// the colored rows with the asked numbers,
// so each of the seven objects has a style and the two that share a label have
// their own.
func TestRunColoursEveryObject(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary integration test")
	}
	asked := []int{1, 2, 13, 19, 22, 58, 66}
	argv := []string{"-qj", "2451545.5", "-zl", "0", "52", "-z", "0", "-R0", "1", "2", "13", "19", "22", "58", "66"}
	out, err := RunPositionsProbing(context.Background(), testCallFor(argv, asked...))
	if err != nil {
		t.Fatalf("RunPositionsProbing: %v", err)
	}
	reg := testRegistry(t)
	for _, n := range asked {
		o, _ := reg.ByNumber(n)
		if _, ok := out.BodyStyles[o]; !ok {
			t.Errorf("%s (object %d) has no style: %d objects coloured", o.Name, n, len(out.BodyStyles))
		}
	}
	titan, _ := reg.ByNumber(58)
	titania, _ := reg.ByNumber(66)
	if out.BodyStyles[titan] == out.BodyStyles[titania] {
		t.Errorf("Titan and Titania share the style %+v", out.BodyStyles[titan])
	}
}
