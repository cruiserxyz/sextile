package astrolog

import (
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// object_rows_test.go: objects slice 0b. Every parsed row carries Astrolog's
// own object, by the order of the rows (positions) or by the registry
// (every list that prints names).

// The probe cast: Sun, Pallas, Ascendant, Midheaven, Titan and Titania, asked
// as `-R0 1 13 22 31 58 66`. The last two print the same four characters
// (`Tita`) and, on this host, no position. The rows take their objects by
// order, before the no-data rows are dropped.
func TestPositionsTakeTheirObjectByOrder(t *testing.T) {
	out := readFixture(t, "probe-titan-titania.txt")
	asked := []int{66, 1, 31, 13, 58, 22, 1} // unsorted and repeated on purpose
	ps := mustPositions(t, out, asked...)
	var got []string
	for _, p := range ps {
		got = append(got, p.Object.Name)
	}
	if want := "Sun Pallas Ascendant Midheaven"; strings.Join(got, " ") != want {
		t.Errorf("objects %q, want %q (Titan and Titania print no position and are dropped)", got, want)
	}
	if ps[0].Object.Number != 1 || ps[3].Object.Number != 31 || ps[0].Object != objSun || ps[1].Object.Name != "Pallas" {
		t.Errorf("rows %+v", ps)
	}

	// The same output with the last two rows given positions: the two rows
	// that print one label are two objects.
	real := strings.Replace(out, "Tita:  0Ari00   + 0:00' (-) [12th house] [-] +0.000", "Tita: 11Gem00   + 1:00' (-) [12th house] [-] +0.100", 1)
	real = strings.Replace(real, "Tita:  0Ari00   + 0:00' (-) [12th house] [-] +0.000", "Tita: 22Leo00   + 2:00' (-) [12th house] [-] +0.200", 1)
	ps = mustPositions(t, real, asked...)
	if len(ps) != 6 || ps[4].Object != (aspect.Object{Number: 58, Name: "Titan"}) || ps[5].Object != (aspect.Object{Number: 66, Name: "Titania"}) {
		t.Fatalf("Titan and Titania are not two objects: %+v", ps)
	}
	if ps[4].Longitude == ps[5].Longitude {
		t.Errorf("the two rows read as one: %+v", ps[4:])
	}
}

// A row count that differs from the objects asked is the call's error, naming
// what was asked; no identity is guessed.
func TestPositionsCountMismatchIsAnError(t *testing.T) {
	out := readFixture(t, "probe-titan-titania.txt")
	_, err := parsePositions(out, []int{1, 13, 22, 31, 58}, testRegistry(t))
	if err == nil || err.Error() != "astrolog: positions: 6 rows printed for 5 objects asked (1 13 22 31 58)" {
		t.Errorf("err = %v", err)
	}
	_, err = parsePositions(out, []int{1, 13, 22, 31, 58, 66, 110}, testRegistry(t))
	if err == nil || !strings.Contains(err.Error(), "6 rows printed for 7 objects asked") {
		t.Errorf("err = %v", err)
	}
}

// The left name is read at seven cells, the right at ten, through the
// registry: Titan and Titania, whose first four characters agree, resolve to
// two objects, on both sides, in a list and in a comparison.
func TestAspectRowsResolveTitanAndTitania(t *testing.T) {
	reg := testRegistry(t)
	got := parseAspects("   7:   Titan (Ari) Con (Ari) Titania    - orb: s0:00' - power: 27.50\n", reg)
	if len(got) != 1 {
		t.Fatalf("parsed %d aspects", len(got))
	}
	if a := got[0]; a.AObject != (aspect.Object{Number: 58, Name: "Titan"}) || a.BObject != (aspect.Object{Number: 66, Name: "Titania"}) {
		t.Errorf("sides %+v %+v", a.AObject, a.BObject)
	}
	// Titan <Ari> is a body with no speed: not read, as before.
	if got := parseAspects("   7:   Titan <Ari> Con <Ari> Titania    - orb: s0:00' - power: 27.50\n", reg); len(got) != 0 {
		t.Errorf("a no-speed row was read: %+v", got)
	}
	cross := parseCrossAspects("  14: Titania (Ari) Con with (Ari) Titan      - orb: s0:00' - power: 17.00\n", reg)
	if len(cross) != 1 || cross[0].PrimaryObject.Name != "Titania" || cross[0].SecondaryObject.Name != "Titan" {
		t.Errorf("cross %+v", cross)
	}
	// An object the registry does not list leaves the side the zero Object.
	odd := parseAspects("   1:     Sun (Ari) Con (Ari) Nibiru      - orb: a0:20' - power:  9.51\n", reg)
	// A pair with an unresolved side keeps its printed order: the zero Object has
	// no number to order by.
	if len(odd) != 1 || odd[0].BObject != (aspect.Object{}) || odd[0].AObject.Name != "Sun" {
		t.Errorf("an unlisted name: %+v", odd)
	}
	// The objects follow their sides through the number-order reorder: the Sun
	// (1) comes before Ceres, printed first.
	swapped := parseAspects("   1:  Ceres (Ari) Con (Ari) Sun         - orb: a0:20' - power:  9.51\n", reg)
	if len(swapped) != 1 || swapped[0].AObject != objSun || swapped[0].BObject.Name != "Ceres" {
		t.Errorf("reorder: %+v", swapped)
	}
}

// A Sun-pair row takes its object by order from the two numbers asked, never
// from its four-character label: a row printed `Tita` is whichever of the two
// asked, here Titan.
func TestSunPairRowTakesItsNumberByOrder(t *testing.T) {
	sample := "Astrolog 8.00: Wed Oct 14, 2026 12:00:00 (ST Zone 0:00W)   0:00'00W 51:00'00N\n" +
		"Body  Location   Ret. Latitude Rul.      House  Rul. Velocity  Lat-Velocity\n\n" +
		"Sun : 21Lib09'41\"   - 0:00'00\" (f) [11th house] [d] +0.9907618 +0.0000085\n" +
		"Tita:  6Sco02'08\" R - 7:31'15\" (d) [12th house] [X] -0.4292777 +0.0379225\n\n\n\n" +
		"Body   Altitude    Azimuth    Azi. Vector    Magn. Full(Area,Time)  Diameter\n" +
		"Sun : +30:40'03\" 265:58'28\" (1.000s 0.070w) -26.87 100.00% 100.00%  0:32'04.086\"\n" +
		"Tita: +18:02'22\" 277:53'36\" (1.000s 0.139e)  -4.32   4.03%  12.87%  0:00'57.784\"\n"
	sun, other, err := ParseSunPair(sample, []int{58, 1}, testRegistry(t))
	if err != nil {
		t.Fatal(err)
	}
	if sun.Object.Name != "Sun" || other.Object != (aspect.Object{Number: 58, Name: "Titan"}) || other.Magnitude != -4.32 {
		t.Errorf("Sun %+v, other %+v", sun, other)
	}
	if _, _, err := ParseSunPair(sample, []int{1}, testRegistry(t)); err == nil {
		t.Error("one object asked parsed")
	}
}

// The cycle list reads its two names through the registry as well.
func TestCycleEventsCarryTheirObjects(t *testing.T) {
	evs, err := ParseCycleEvents("Thu  1/20/2000 18:23:02     Sun (29Cap59) Con (29Cap59) Venus\n", testRegistry(t))
	if err != nil || len(evs) != 1 {
		t.Fatalf("%v %v", evs, err)
	}
	if evs[0].Object.Name != "Sun" || evs[0].OtherObject.Name != "Venus" {
		t.Errorf("%+v", evs[0])
	}
}

// The four-character label a `-v` row prints is the registry name cut to four
// cells: every row of the Titan/Titania probe (two objects printing one label
// among them) reads back as the label Astrolog printed on that row. Sextile's
// own row prints the full name (the Object's), so the two objects that share
// the label "Tita" are told apart by it.
func TestRowLabelIsTheObjectCutToFour(t *testing.T) {
	out := readFixture(t, "probe-titan-titania.txt")
	// Give the two no-data rows real positions so they survive the parse.
	out = strings.Replace(out, "Tita:  0Ari00   + 0:00' (-) [12th house] [-] +0.000", "Tita: 11Gem00   + 1:00' (-) [12th house] [-] +0.100", 1)
	out = strings.Replace(out, "Tita:  0Ari00   + 0:00' (-) [12th house] [-] +0.000", "Tita: 22Leo00   + 2:00' (-) [12th house] [-] +0.200", 1)
	var printed []string
	for _, line := range strings.Split(out, "\n") {
		if m := bodyPositionPat.FindStringSubmatch(line); m != nil {
			printed = append(printed, m[1])
		}
	}
	ps := mustPositions(t, out, 1, 13, 22, 31, 58, 66)
	if len(ps) != 6 || len(printed) != 6 {
		t.Fatalf("%d positions for %d printed rows, want 6 and 6", len(ps), len(printed))
	}
	for i, p := range ps {
		if got := p.Object.Cut(4); got != printed[i] {
			t.Errorf("row %d: Object %+v cut to four = %q, Astrolog printed %q", i, p.Object, got, printed[i])
		}
	}
	reg := testRegistry(t)
	full := map[int]string{}
	for _, p := range ps {
		if len(p.Object.Name) <= 4 {
			continue
		}
		o, ok := reg.ByNumber(p.Object.Number)
		if !ok {
			t.Fatalf("object %d is not in the registry", p.Object.Number)
		}
		got := strings.TrimRight(p.Object.Name, " ")
		if got != strings.TrimRight(o.Name, " ") {
			t.Errorf("object %d Name = %q, want the registry's full name %q", p.Object.Number, got, o.Name)
		}
		full[p.Object.Number] = got
	}
	if full[58] != "Titan" || full[66] != "Titania" {
		t.Errorf("the rows that print one label read %q and %q, want Titan and Titania", full[58], full[66])
	}
}

// Rule pinned (v0.1.3 release edges, 2026-11-07): the positions parser names a
// row by its place among the asked numbers, and checks that the row's own
// printed label agrees. A `-Uz` star sort in the host's astrolog.as reorders
// the star rows, so by place Regulus's row would be read as Antares.
func TestPositionsRefuseRowsInAnotherOrder(t *testing.T) {
	out := readFixture(t, "positions-stars-uz-order.txt")
	_, err := parsePositions(out, []int{98, 99, 104}, testRegistry(t))
	if err == nil {
		t.Fatal("stars in -Uz order were read by place, Regulus as Antares")
	}
	for _, want := range []string{"Regulus", "Antares", "row 1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err %q lacks %q", err, want)
		}
	}
}

// The same three rows in number order are read as asked.
func TestPositionsInNumberOrderParse(t *testing.T) {
	out := readFixture(t, "positions-stars-number-order.txt")
	ps := mustPositions(t, out, 98, 99, 104)
	var got []string
	for _, p := range ps {
		got = append(got, p.Object.Name)
	}
	if want := "Antares Spica Regulus"; strings.Join(got, " ") != want {
		t.Errorf("objects %q, want %q", got, want)
	}
}
