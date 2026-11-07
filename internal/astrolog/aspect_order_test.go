package astrolog

import (
	"testing"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// TestParsedPairsFollowNumberOrder: a parsed aspect orders its sides by
// Astrolog's object number whichever way the line printed them, and the colored
// spans of each side go with it. The pairs the old body order ordered
// otherwise: Chiron (11) against the nodes (16, 17), the Midheaven (31) against
// the Nadir (25), and an object with no glyph (Fortune, 19) against a presented
// one, which the old order put first.
func TestParsedPairsFollowNumberOrder(t *testing.T) {
	reg := testRegistry(t)
	const lines = `  1:  Chiron (Ari) Tri (Vir) North Node  - orb: a1:00' - power:  4.50
  2: North N (Vir) Tri (Ari) Chiron      - orb: a1:00' - power:  4.40
  3: Midheav (Cap) Opp (Can) Nadir       - orb: s0:30' - power:  9.00
  4: Fortune (Leo) Con (Leo) Sun         - orb: a0:10' - power:  8.00
  5:     Sun (Leo) Sex (Gem) Moon        - orb: a0:10' - power:  7.00
`
	got := parseAspects(lines, reg)
	if len(got) != 5 {
		t.Fatalf("parsed %d aspects, want 5", len(got))
	}
	for i, tc := range []struct {
		first, second string
	}{{"Chiron", "North Node"}, {"Chiron", "North Node"}, {"Nadir", "Midheaven"}, {"Sun", "Fortune"}, {"Sun", "Moon"}} {
		a, b := got[i].AObject, got[i].BObject
		if a.Name != tc.first || b.Name != tc.second {
			t.Errorf("line %d: sides %s, %s; want %s, %s", i+1, a.Name, b.Name, tc.first, tc.second)
		}
		if a.Number >= b.Number {
			t.Errorf("line %d: sides %d, %d are not in number order", i+1, a.Number, b.Number)
		}
	}

	chiron, _ := reg.ByName("Chiron")
	node, _ := reg.ByName("North Node")
	rows := []astrologstyle.AspectRow{
		{FirstName: "North N", KindName: "Tri", SecondName: "Chiron", Style: astrologstyle.AspectStyle{First: "node", Kind: "tri", Second: "chiron"}},
		{FirstName: "Chiron", KindName: "Sex", SecondName: "North Node", Style: astrologstyle.AspectStyle{First: "chiron", Kind: "sex", Second: "node"}},
	}
	styles := mapAspectStyles(rows, reg)
	for _, kind := range []aspect.AspectKind{aspect.Trine, aspect.Sextile} {
		style := styles[astrologstyle.AspectIdentity{First: chiron, Second: node, Kind: kind}]
		if style.First != "chiron" || style.Second != "node" {
			t.Errorf("%v: spans %+v, want Chiron's first and the node's second", kind, style)
		}
	}
}
