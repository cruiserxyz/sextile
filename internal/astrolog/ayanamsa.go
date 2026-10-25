package astrolog

import (
	"strconv"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// ayanamsaOffsets is the ONE table of Astrolog 8.00's ten named ayanamsas:
// each one's offset, in degrees, from Fagan-Bradley, the zero point `=s 0`
// selects (the specification C2). It is a copy of an Astrolog fact, and
// TestAyanamsaTableMatchesAstrolog is what keeps it Astrolog's: it casts one
// chart under `=s <name>` and under `=s <offset>` to the millisecond of arc
// and requires them equal.
//
// Six decimals are what Astrolog's table carries and what reproduces its
// named casts to the millisecond of arc over the forty dates the offsets were
// measured on; Tropical has no entry, it has no offset.
var ayanamsaOffsets = map[recipe.Zodiac]float64{
	recipe.FaganBradley:   0,
	recipe.Lahiri:         0.883208,
	recipe.Deluce:         -3.075453,
	recipe.Raman:          2.329509,
	recipe.UshaShasi:      4.682759,
	recipe.Krishnamurti:   0.98006,
	recipe.DjwhalKhul:     -3.619379,
	recipe.Yukteshwar:     2.261497,
	recipe.JNBhasin:       1.978163,
	recipe.GalacticCenter: -2.105736,
}

// AyanamsaOffset is the offset of a named ayanamsa from Fagan-Bradley, in
// degrees; ok is false for Tropical and for a value that is no zodiac.
func AyanamsaOffset(z recipe.Zodiac) (float64, bool) {
	off, ok := ayanamsaOffsets[z]
	return off, ok
}

// OffsetArg spells an offset the way `=s` is given it: the shortest decimal
// that reads back to the same number, so "0.883208" and "-3.075453", never an
// exponent and never a padded zero. It is the only spelling of an offset on an
// argv or in a staged chart file.
func OffsetArg(off float64) string { return strconv.FormatFloat(off, 'f', -1, 64) }

// zodiacPin is the zodiac switch of the frame band and of a staged chart file:
// `_s` for tropical, `=s <offset>` for a sidereal zodiac. It is numeric, never
// the ayanamsa's name (a name would leave the table, and a typo, to Astrolog),
// and never a bare `-s`, which toggles against whatever the host left. A value
// that is no zodiac has no pin.
func zodiacPin(z recipe.Zodiac) ([]string, bool) {
	if z == recipe.Tropical {
		return []string{"_s"}, true
	}
	off, ok := AyanamsaOffset(z)
	if !ok {
		return nil, false
	}
	return []string{"=s", OffsetArg(off)}, true
}
