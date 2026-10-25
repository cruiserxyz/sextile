package astrolog

import (
	"os"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// requirement C4 — the aspect set is READ from the user's binary, not compiled in.
// The fixture is a real `astrolog -HA` capture (Astrolog 8.00).

func loadHA(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(b)
}

func TestParseAspectRegistry(t *testing.T) {
	reg, err := ParseAspectRegistry(loadHA(t, "ha-default.txt"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(reg) != 24 {
		t.Fatalf("parsed %d aspects, want 24 — the registry runs well past the eleven the enum names", len(reg))
	}

	if got := reg[0]; got.Index != 1 || got.Name != "Conjunct" || got.Abbrev != "Con" || got.Angle != 0 || got.Orb != 7 {
		t.Errorf("row 1 = %+v", got)
	}
	// The septile is the row that matters: `-HA` prints its angle rounded to
	// 51.43 where the exact value is 51.428571…, so a parser or a matcher
	// demanding exactness would lose it.
	var septile bool
	for _, r := range reg {
		if r.Index == 13 {
			septile = true
			if r.Angle != 51.43 {
				t.Errorf("septile angle = %v, want the printed 51.43", r.Angle)
			}
		}
	}
	if !septile {
		t.Error("index 13 (Septile) missing from the parsed registry")
	}

	// The trailing "Unrestricted aspects" block is a gap analysis of the
	// CURRENT restriction state, not part of the registry. Sweeping it in
	// would invent aspects that do not exist.
	for _, r := range reg {
		if r.Index > 24 {
			t.Errorf("parsed a row past the registry: %+v", r)
		}
	}
}

// The orb is the EFFECTIVE orb, which is what makes C14 possible at all.
func TestParseAspectRegistryReadsEffectiveOrb(t *testing.T) {
	zeroed := strings.Replace(loadHA(t, "ha-default.txt"),
		" 5 Sextile        (Sex)  60    +/- 6   degrees",
		" 5 Sextile        (Sex)  60    +/- 0   degrees", 1)
	reg, err := ParseAspectRegistry(zeroed)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, r := range reg {
		if r.Index != 5 {
			continue
		}
		if r.Orb != 0 {
			t.Fatalf("sextile orb = %v, want 0", r.Orb)
		}
		if r.CountsAnything() {
			t.Error("a zero-orb aspect reports that it counts; it cannot produce a row however it is restricted")
		}
		return
	}
	t.Error("sextile missing")
}

// C3a — `-HA` says which aspects the host COUNTS, and the parse reads it.
//
// The fixture is a real capture from a default binary: twenty-four aspects in
// the registry, five of them unrestricted. That gap is the whole finding. A
// chooser seeded from the orb column opens claiming all twenty-four, and the
// nineteen extra ride into the chart on the first confirm.
func TestParseAspectRegistryReadsTheInForceSet(t *testing.T) {
	reg, err := ParseAspectRegistry(loadHA(t, "ha-default.txt"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Con(1) Opp(2) Squ(3) Tri(4) Sex(5) — the section names them by
	// abbreviation and in zodiac order, so this also proves the mapping back
	// to registry indices rather than to the order they were printed in.
	want := map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true}
	for _, r := range reg {
		if r.InForce != want[r.Index] {
			t.Errorf("%s (%d): InForce = %v, want %v", r.Name, r.Index, r.InForce, want[r.Index])
		}
		if !r.InForce && r.Orb <= 0 {
			t.Errorf("%s (%d): the fixture's restricted rows are supposed to carry positive orbs, or this proves nothing", r.Name, r.Index)
		}
	}

	// Restriction and orb are orthogonal in BOTH directions, and this is the
	// direction that is easy to miss: `-Ao 5 0 -HA` leaves the sextile in the
	// unrestricted section (`covers 60 to 60 - span 0`) while its orb is
	// zero. It is in force and still cannot report anything.
	zeroed := strings.Replace(loadHA(t, "ha-default.txt"),
		" 5 Sextile        (Sex)  60    +/- 6   degrees",
		" 5 Sextile        (Sex)  60    +/- 0   degrees", 1)
	reg2, err := ParseAspectRegistry(zeroed)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, r := range reg2 {
		if r.Index != 5 {
			continue
		}
		if !r.InForce {
			t.Error("a zero-orb aspect lost its InForce; orb and restriction are separate switches")
		}
		if r.CountsAnything() {
			t.Error("a zero-orb aspect reports that it counts")
		}
	}

	// An empty section is an ANSWER, not a failure: `-RA0 -HA` prints the
	// heading with no aspect rows under it, and it means the host counts
	// none. Refusing here would make a legitimate host state unreachable.
	none := loadHA(t, "ha-default.txt")
	none = none[:strings.Index(none, unrestrictedHeading)] + unrestrictedHeading + " in order around zodiac:\n\nTotal aspects (0.00, 0.00%), Total gaps (180.00, 100.00%)\n"
	reg3, err := ParseAspectRegistry(none)
	if err != nil {
		t.Fatalf("an all-restricted host failed to parse: %v", err)
	}
	for _, r := range reg3 {
		if r.InForce {
			t.Errorf("%s is in force with an empty unrestricted section", r.Name)
		}
	}
}

// The rename and colour shapes this parser is known to read, each captured from
// the binary rather than imagined.
//
// NOT "every shape `-YAD` can produce" — an earlier version of this comment
// claimed that and the cross-family reviewer disproved it in one line, because
// the fields accept a NEWLINE and no fixture table can enumerate free text. The
// guarantee lives in the two tests below this one instead: an ambiguous row is
// refused, and a forged row is refused. This table is the set that must READ,
// and it is a floor rather than a closure.
//
// Three review rounds found six defects in this parser, and the first four
// fixes were each a regex that read the case in front of it and broke on the
// next. The design that stopped that is in readHARow: enumerate every reading
// of a row and insist on exactly one. This table is the closed set of shapes it
// has to survive, and the one below is the set it has to REFUSE.
//
// Regenerate with:
//
//	astrolog -YAD 5 "Semi Square" "S X" '*' -HA                        > testdata/ha-renamed.txt
//	astrolog -YAD 5 Sextile "S:X" '*' -HA                              > testdata/ha-colon.txt
//	astrolog -YAD 5 Sextile 'X)Y' '*' -HA                              > testdata/ha-paren.txt
//	astrolog -YAD 5 "Semi Square With A Longer Name" LONGABBR '*' -HA   > testdata/ha-overflow.txt
//	astrolog -k -HA                                                    > testdata/ha-ansi.txt
func TestParseAspectRegistrySurvivesEveryRenameShape(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		why     string
		name    string
		abbrev  string
	}{
		{"ha-renamed.txt", "a space in BOTH the name and the abbreviation", "Semi Square", "S X"},
		{"ha-colon.txt", "a COLON in the abbreviation: the section's separator is not the first colon on the line", "Sextile", "S:X"},
		{"ha-paren.txt", "a PARENTHESIS in the abbreviation, which an [^)]+ capture cannot read", "Sextile", "X)Y"},
		{"ha-overflow.txt", "the name fills its field, so the binary leaves NO space before the abbreviation", "Semi Square With A Longer Name", "LONGABBR"},
		{"ha-ansi.txt", "the host turned COLOUR on, so every row and heading is preceded by an SGR escape", "Sextile", "Sex"},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			reg, err := ParseAspectRegistry(loadHA(t, tc.fixture))
			if err != nil {
				t.Fatalf("%s: %v", tc.why, err)
			}
			if len(reg) != 24 {
				t.Fatalf("parsed %d aspects, want 24", len(reg))
			}
			var five *aspect.RegistryAspect
			for i := range reg {
				if reg[i].Index == 5 {
					five = &reg[i]
				}
			}
			if five == nil {
				t.Fatal("index 5 missing")
			}
			if five.Name != tc.name {
				t.Errorf("name = %q, want %q", five.Name, tc.name)
			}
			if five.Abbrev != tc.abbrev {
				t.Errorf("abbrev = %q, want %q", five.Abbrev, tc.abbrev)
			}
			// The angle and the orb are what a decoy can steal, and they are
			// what C5's grouping and C14's selectability are computed from.
			if five.Angle != 60 {
				t.Errorf("angle = %v, want 60 — a fabricated angle moves the aspect into the wrong harmonic family (C5)", five.Angle)
			}
			if five.Orb != 6 {
				t.Errorf("orb = %v, want 6 — a fabricated orb decides whether the row can be selected at all (C14)", five.Orb)
			}
			if !five.InForce {
				t.Error("came back restricted though the unrestricted section lists it")
			}
			for _, r := range reg {
				if want := r.Index <= 5; r.InForce != want {
					t.Errorf("%s (%d): InForce = %v, want %v", r.Name, r.Index, r.InForce, want)
				}
			}
		})
	}
}

// A row whose own free text contains something shaped like its data is
// genuinely ambiguous, and `-YAD` restricts none of the three free fields. The
// parser must refuse rather than pick a reading.
//
// Both of these were found by review rather than by the suite, and they point
// in OPPOSITE directions — which is the whole argument for enumerating instead
// of pattern-matching. A non-greedy name capture read the glyph decoy correctly
// and fabricated 42/3 from the name decoy; making it greedy swapped which one
// broke. Neither reading is more correct than the other, because the producer
// really did emit both.
//
//	astrolog -YAD 5 "Sextile (Sex) 42 +/- 3 degrees" Sex '*' -HA   > testdata/ha-decoy.txt
//	astrolog -YAD 5 Sextile Sex '(Sex) 42 +/- 3 degrees' -HA       > testdata/ha-glyphdecoy.txt
//
// A renamed field can FORGE A ROW, and no amount of reading one line correctly
// catches that — the forgery is well-formed. `-YAD` restricts no character in a
// name, an abbreviation or a glyph, and that includes the NEWLINE:
//
//	astrolog -YAD 5 Sextile Sex $'"'"'(Sex) 42 +/- 3 degrees\n25 Bogus (Bog) 42 +/- 3 degrees'"'"' -HA
//
// makes the binary print `25 Bogus (Bog) 42 +/- 3 degrees` as its own physical
// line between rows 5 and 6. It has exactly one reading, so candidate
// enumeration accepts it and the chooser gains an aspect the binary does not
// have — which then reaches `-RA0` and the chart.
//
// The fence is the STREAM's grammar rather than the row's: Astrolog walks its
// own aspect array, so the registry is numbered 1..N contiguously. A forged row
// is caught by where it sits, not by how it reads.
//
// Found by the independent cross-family reviewer, on the re-read of the fix for
// the previous shape of this same defect — the fourth and last in a chain where
// each repair read the case in front of it. This one is a different kind of
// rule, which is why it ends the chain.
func TestParseAspectRegistryRefusesAForgedRow(t *testing.T) {
	_, err := ParseAspectRegistry(loadHA(t, "ha-injected.txt"))
	if err == nil {
		t.Fatal("a row forged through a newline in the glyph was accepted; the chooser would offer an aspect the binary does not have, and -RA0 would carry it into the chart")
	}
	if !strings.Contains(err.Error(), "contiguously") {
		t.Errorf("refused with %q, want the sequence violation named", err)
	}

	// The same forgery on the other side of the heading, and a forged heading.
	full := loadHA(t, "ha-default.txt")
	for name, in := range map[string]string{
		"a registry row after the heading": full + "\n25 Bogus (Bog) 42 +/- 3 degrees\n",
		"a forged second heading":          full + "\n" + unrestrictedHeading + " in order around zodiac:\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseAspectRegistry(in); err == nil {
				t.Error("parsed without error")
			}
		})
	}
}

func TestParseAspectRegistryRefusesAnAmbiguousRow(t *testing.T) {
	for _, tc := range []struct{ fixture, where string }{
		{"ha-decoy.txt", "the NAME carries a complete second reading"},
		{"ha-glyphdecoy.txt", "the GLYPH carries a complete second reading, and it is printed LAST"},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			reg, err := ParseAspectRegistry(loadHA(t, tc.fixture))
			if err == nil {
				var five aspect.RegistryAspect
				for _, r := range reg {
					if r.Index == 5 {
						five = r
					}
				}
				t.Fatalf("%s, and the parser picked one: angle=%v orb=%v. A fabricated angle regroups the aspect (C5) and a fabricated orb decides whether it can be chosen (C14)", tc.where, five.Angle, five.Orb)
			}
			if !strings.Contains(err.Error(), "can be read") {
				t.Errorf("refused with %q, want the ambiguity to be named", err)
			}
		})
	}
}

// A registry that parses into an ambiguous table is worse than one that does
// not parse at all, because it compiles.
func TestParseAspectRegistryRefusesAmbiguity(t *testing.T) {
	full := loadHA(t, "ha-default.txt")
	cases := map[string]string{
		"empty output":    "",
		"no aspect rows":  "Astrolog aspects:\nNo. Name Abbrev. Angle Orb\n\n",
		"duplicate index": " 1 Conjunct (Con) 0 +/- 7 degrees x\n 1 Opposite (Opp) 180 +/- 7 degrees y\n",
		// Without the section there is no way to know what the host
		// counts, and a chooser that guesses puts its guess in the chart.
		// C10's refusal is the right outcome.
		"no unrestricted section": full[:strings.Index(full, unrestrictedHeading)],
		// The section names an aspect the registry does not carry, so the
		// two halves cannot be reconciled.
		"section names an unknown aspect": strings.Replace(full, "Sex: Sextile", "Zzz: Nonesuch", 1),
		// The section half of the look-alike rule. A row that plainly
		// belongs to this section and does not parse must refuse, exactly
		// as an unparseable registry row does — skipping it reports an
		// aspect the host counts as one it does not.
		"unparseable section row": strings.Replace(full, "Sex: Sextile", " Sex Sextile", 1),
		// Two rows sharing an abbreviation make the section ambiguous
		// even though the registry itself parses.
		"duplicate abbreviation": strings.Replace(full, "(Inc)", "(Con)", 1),
		// A row that is plainly an aspect and does not parse must refuse,
		// not be skipped as incidental output. Skipped, the chooser opens
		// missing an aspect the binary has and says nothing.
		"unparseable orb":   strings.Replace(full, "(Inc) 150    +/- 3", "(Inc) 150    +/- ?", 1),
		"unparseable angle": strings.Replace(full, "(Qui)  72    +/- 2", "(Qui)  7x    +/- 2", 1),
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseAspectRegistry(in); err == nil {
				t.Error("parsed without error")
			}
		})
	}
}
