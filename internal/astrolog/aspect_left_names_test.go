package astrolog

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// the specification C1: Astrolog prints the FIRST name of an aspect row cut
// to seven characters (charts2.cpp:750, "%7.7s") and the second cut to ten
// (:890, "%.10s"). A cut that leaves more than one character after a space --
// "East Po", "6th Cus", "11th Cu", "Kaus Au", "Zeta Re" -- once made the whole
// row vanish from the Aspects panel without a word.

// sides is an aspect's two sides by name: the registry name of each side's
// Object.
func sides(a aspect.Aspect) [2]string {
	return [2]string{a.AObject.Name, a.BObject.Name}
}

func hasSides(a aspect.Aspect, x, y string) bool {
	// A side's registry name may be longer than the text Astrolog printed in
	// its seven-cell column, so the printed text is a prefix of it.
	same := func(side, printed string) bool { return side == printed || strings.HasPrefix(side, printed) }
	s := sides(a)
	return (same(s[0], x) && same(s[1], y)) || (same(s[0], y) && same(s[1], x))
}

// isPrinted reports whether one parsed side is the object Astrolog printed as
// `printed` (a name, or its seven-cell cut).
func isPrinted(o aspect.Object, printed string) bool {
	return o.Name != "" && strings.HasPrefix(o.Name, printed)
}

// pairIs: the aspect's two sides are exactly the two printed names, in either
// order (NewObjectAspect may swap them).
func pairIs(a aspect.Aspect, x, y string) bool {
	return (isPrinted(a.AObject, x) && isPrinted(a.BObject, y)) ||
		(isPrinted(a.AObject, y) && isPrinted(a.BObject, x))
}

func TestParseAspects_KeepsEveryCutOffFirstName(t *testing.T) {
	for _, tc := range []struct {
		line string
		x, y string
		kind aspect.AspectKind
	}{
		{"  14: 5th Cus [Tau] Con [Tau] 6th Cusp   - orb: a0:12' - power:  4.10", "5th Cus", "6th Cusp", aspect.Conjunction},
		{"  18: East Po (Ari) Squ (Cap) Midheaven  - orb: s0:14' - power:  6.65", "East Po", objMidheaven.Name, aspect.Square},
		{"  11: 11th Cu (Leo) Con [Leo] Zeus       - orb: s0:39' - power:  3.00", "11th Cu", "Zeus", aspect.Conjunction},
		{"   7: Kaus Au (Cap) Tri (Tau) Zeta Retic - orb: a1:02' - power:  1.20", "Kaus Au", "Zeta Retic", aspect.Trine},
		{"   9: Zeta Re (Ari) Sex (Gem) Kaus Austr - orb: s0:40' - power:  1.10", "Zeta Re", "Kaus Austr", aspect.Sextile},
		// Controls: the second-name side, and a two-word typed name, as before.
		{"  89: Neptune [Ari] Con (Ari) East Point - orb: a0:58' - power:  3.08", objNeptune.Name, "East Point", aspect.Conjunction},
		{"   3:     Sun (Ari) Tri (Leo) North Node - orb: a2:00' - power:  5.00", objSun.Name, objNorthNode.Name, aspect.Trine},
		{"   4: North N (Leo) Opp (Aqu) Moon       - orb: s1:00' - power:  5.00", objNorthNode.Name, objMoon.Name, aspect.Opposition},
	} {
		got := parseAspects(tc.line, testRegistry(t))
		if len(got) != 1 || !hasSides(got[0], tc.x, tc.y) || got[0].Kind != tc.kind {
			var s []string
			for _, a := range got {
				s = append(s, fmt.Sprintf("%v %v", sides(a), a.Kind))
			}
			t.Errorf("%q\n  parsed %v, want %s / %s %v", tc.line, s, tc.x, tc.y, tc.kind)
		}
	}
	// What stays out: a first name wider than Astrolog's seven-cell field is no
	// Astrolog row, and a row whose sign is in <angle brackets> is a body with
	// no speed at all -- in practice one Astrolog could not compute (its
	// ephemeris file is missing: it sits at 0Ari00, +0.000), so its aspects
	// would be drawn at a position that is not the body's.
	for _, line := range []string{
		"   1: Sunshine (Ari) Con (Ari) Moon       - orb: a0:20' - power:  9.51",
		"  23: Midheav (Cap) Squ <Ari> Hygiea     - orb: s1:19' - power:  5.84",
		"  24:  Hygiea <Ari> Con <Ari> Eris       - orb: s0:00' - power: 27.50",
	} {
		if got := parseAspects(line, testRegistry(t)); len(got) != 0 {
			t.Errorf("%q parsed as %v, want no aspect", line, sides(got[0]))
		}
	}
}

// Every object the host's Astrolog knows, printed the way Astrolog prints it on
// either side of a single-chart and of a comparing row, comes back as that
// object. The rows are built from Astrolog's own print formats, not from the
// parser's pattern.
func TestEveryAstrologObjectParsesOnEitherSide(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH")
	}
	reg, err := ObjectRegistry(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(reg) < 100 {
		t.Fatalf("registry holds %d objects, want Astrolog 8's full table", len(reg))
	}
	partner := "Sun"
	for _, o := range reg {
		first := fmt.Sprintf("%7.7s", o.Name)
		second := o.Name
		if len(second) > 10 {
			second = second[:10]
		}
		pad := strings.Repeat(" ", 10-len(second))
		f := strings.TrimSpace(first)
		for _, tc := range []struct{ line, x string }{
			{fmt.Sprintf("  1: %s (Ari) Con (Ari) %-10s - orb: a0:20' - power:  9.51", first, partner), f},
			{fmt.Sprintf("  1: %7s (Ari) Con (Ari) %s%s - orb: a0:20' - power:  9.51", partner, second, pad), second},
		} {
			got := parseAspects(tc.line, testRegistry(t))
			if len(got) != 1 || !pairIs(got[0], tc.x, partner) || got[0].Kind != aspect.Conjunction {
				t.Errorf("object %d %q: %q not read as %q / %q", o.Number, o.Name, tc.line, tc.x, partner)
			}
		}
		for _, tc := range []struct {
			line          string
			first, second string
		}{
			{fmt.Sprintf("  1: %s (Ari) Con with (Ari) %-10s - orb: a0:20' - power:  9.51", first, partner), f, partner},
			{fmt.Sprintf("  1: %7s (Ari) Con with (Ari) %s%s - orb: a0:20' - power:  9.51", partner, second, pad), partner, second},
		} {
			got := parseCrossAspects(tc.line, testRegistry(t))
			if len(got) != 1 || !isPrinted(got[0].PrimaryObject, tc.first) ||
				!isPrinted(got[0].SecondaryObject, tc.second) {
				t.Errorf("object %d %q: comparing row %q not read as %q with %q", o.Number, o.Name, tc.line, tc.first, tc.second)
			}
		}
	}
}

// A real chart with every object switched on: each aspect row Astrolog prints
// with a moving body on both sides is kept, with the first name Astrolog
// printed in its fixed seven-cell column. The oracle reads that column by
// position, not through the parser's pattern.
func TestEveryPrintedAspectRowIsKept(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH")
	}
	// -R1 switches every object on, including ones whose ephemeris file this
	// share lacks: the call collects those lines, this test reads aspect rows.
	var files []string
	out, _, err := spawn(context.Background(), testCall([]string{
		"-qa", "3", "21", "1985", "14:30", "0", "0:10W", "51:30N", "-R1", "-aa",
	}).reportingMissing(&files).probingCall())
	if err != nil {
		t.Fatal(err)
	}
	rows, cutOff := 0, 0
	for _, line := range strings.Split(out, "\n") {
		// The second name may abut the dash ("Descendant- orb:").
		if !strings.Contains(line, "- orb: ") {
			continue
		}
		at := strings.Index(line, ": ")
		if at < 0 || len(line) < at+2+7 {
			t.Fatalf("aspect row without its name column: %q", line)
		}
		first := strings.TrimSpace(line[at+2 : at+2+7])
		// The second name follows the second sign's closing bracket and runs
		// to the dash before "orb:".
		rest, closes := line[at+2+7:], 0
		second := ""
		for i, r := range rest {
			if r == ')' || r == ']' || r == '>' {
				if closes++; closes == 2 {
					second = strings.TrimSpace(rest[i+1 : strings.Index(rest, "- orb: ")])
					break
				}
			}
		}
		got := parseAspects(line, testRegistry(t))
		if strings.ContainsAny(line, "<>") {
			if len(got) != 0 {
				t.Errorf("a body with no speed was kept: %q", line)
			}
			continue
		}
		rows++
		if strings.Contains(first, " ") && first[len(first)-2] != ' ' {
			cutOff++
		}
		if second == "" || len(got) != 1 || !pairIs(got[0], first, second) {
			t.Errorf("row dropped or misread (%q with %q): %q", first, second, line)
		}
	}
	if rows < 300 || cutOff == 0 {
		t.Errorf("%d rows, %d with a cut-off two-word first name: the chart no longer exercises the case", rows, cutOff)
	}
}
