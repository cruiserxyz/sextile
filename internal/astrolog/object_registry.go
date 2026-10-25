package astrolog

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// object_registry.go reads the object set out of the user's own Astrolog
// binary, the counterpart of aspect_registry.go.
//
// Two things about the read are easy to get wrong and expensive to get wrong
// quietly.
//
// The COMMAND is `-R1 -HO`, never a bare `-HO`. `-HO` lists only unrestricted
// objects, so a read taken once this program's own `-R0` band is in force
// returns the current selection and nothing else — the operator could then
// never re-add a body, and every subsequent open would narrow further. `-R1`
// unrestricts everything first.
//
// The FORMAT is two row shapes under one header, and a parser written for the
// first drops or mangles the second while reporting success:
//
//	 11 Chiron      Sag      Gem       Pis   Vir
//	130 Andromeda   Star #47  27Ari28   +33:20'  3.44
//
// Fifty of the 134 rows are stars. Names also carry spaces ("North Node") and
// periods ("Zeta Retic."), and a star name can be followed by a SINGLE space
// ("Zeta Retic. Star #48"), so neither whitespace splitting nor a two-space
// separator can find the column edge. The layout is fixed-width and is parsed
// as such: number in columns 0–2, name in columns 4–15, the shape-bearing
// remainder from column 16.

const (
	objNumEnd   = 3  // columns 0..2 hold the right-aligned object number
	objNameCol  = 4  // the name column starts here
	objNameEnd  = 16 // ...and ends here, 12 columns wide
	objRestCol  = 16 // the dignity or star columns
	objHeaderNo = "Num."
)

// ParseObjectRegistry reads `astrolog -R1 -HO` output.
//
// It refuses rather than guesses on anything that would compile to the wrong
// object: a number that does not parse, a negative number, or two rows sharing
// a number. A registry that parses into an ambiguous table is worse than one
// that does not parse at all, because the ambiguous one reaches the argv.
func ParseObjectRegistry(out string) ([]aspect.RegistryObject, error) {
	var reg []aspect.RegistryObject
	seen := map[int]bool{}
	// The producer may COLOUR this output: `PrintObjects` emits an SGR change
	// before every object number when the host's `=k` setting is on, and this
	// parse is COLUMN-BASED, so an escape sequence at the start of the line
	// shifts every field and the registry is refused. Strip first, slice
	// after. The aspect registry carried the same defect.
	for _, line := range strings.Split(astrologstyle.StripSGR(out), "\n") {
		if len(line) < objNameCol+1 {
			continue
		}
		numField := strings.TrimSpace(line[:objNumEnd])
		if numField == "" || strings.HasPrefix(line, objHeaderNo) {
			continue
		}
		// The line must open with a number and nothing else. The prose
		// astrolog folds into this stream — the title, the header, blank
		// filler — carries no digit in these three columns at all, so it is
		// skipped rather than refused.
		//
		// A field that is PART digits is a different thing entirely: `-1`,
		// `12x` and `1.2` are corrupted rows, not prose, and skipping one
		// opens the selector missing an object the binary has, with nothing
		// anywhere saying so. That is C10's failure mode wearing a success,
		// and it is the same defect the aspect registry carried.
		if !allDigits(numField) {
			if strings.ContainsFunc(numField, func(r rune) bool { return r >= '0' && r <= '9' }) {
				return nil, fmt.Errorf("astrolog: -R1 -HO: unparseable object number %q", numField)
			}
			continue
		}
		// Digits only, so a negative is impossible here; what `err` still
		// catches is a field too long to fit an int.
		num, err := strconv.Atoi(numField)
		if err != nil {
			return nil, fmt.Errorf("astrolog: -R1 -HO: object number %q: %w", numField, err)
		}
		// Two rows sharing a number parse cleanly and then compile to the
		// wrong object, which is the failure this whole function exists to
		// refuse.
		if seen[num] {
			return nil, fmt.Errorf("astrolog: -R1 -HO: object number %d appears twice", num)
		}
		name := strings.TrimSpace(line[objNameCol:min(objNameEnd, len(line))])
		if name == "" {
			return nil, fmt.Errorf("astrolog: -R1 -HO: object %d has no name", num)
		}
		seen[num] = true
		rest := ""
		if len(line) > objRestCol {
			rest = line[objRestCol:]
		}
		reg = append(reg, aspect.RegistryObject{
			Number: num,
			Name:   name,
			// The star rows are the ones whose remainder opens with the
			// catalogue marker. Derived from the SHAPE, not from a number
			// range, so a differently ordered registry still classifies.
			Star: strings.HasPrefix(strings.TrimSpace(rest), "Star #"),
		})
	}
	if len(reg) == 0 {
		return nil, fmt.Errorf("astrolog: -R1 -HO: no object rows found")
	}
	return reg, nil
}

// allDigits reports whether s is a non-empty run of ASCII digits. It is what
// separates a registry row from the header and from the warnings astrolog
// folds into the same stream.
func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ObjectRegistry runs `astrolog -R1 -HO` and parses it. The caller reads this
// ONCE, at startup, before any `-R0` band this program emits — see the
// command note above, and C5a.
func ObjectRegistry(ctx context.Context) ([]aspect.RegistryObject, error) {
	out, _, err := spawn(ctx, newExempt("object registry: -R1 -HO lists the object table, no chart", "-R1", "-HO"))
	if err != nil {
		return nil, err
	}
	return ParseObjectRegistry(out)
}
