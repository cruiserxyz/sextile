package astrolog

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// aspect_registry.go reads the aspect set out of the user's own Astrolog
// binary. It is deliberately a PARSE and not a table: `astrolog -HA` prints
// number, name, abbreviation, angle and orb per aspect, and a user's binary may
// carry a different set, different names, or different orbs than any constant
// compiled in here would claim.
//
// The orb printed is the EFFECTIVE orb, not a compiled-in default: running
// `-Ao 5 0 -HA` prints `5 Sextile (Sex) 60 +/- 0`. That matters, because an
// aspect whose effective orb is zero is one the chart cannot report no matter
// how the chooser is set — restriction and orb are orthogonal switches.

// rowHead matches the one field of a registry row that is unambiguously
// machine-generated and unambiguously positioned: the index, at the start.
var rowHead = regexp.MustCompile(`^\s*(\d+)\s+`)

// rowTail matches the machine-generated remainder that follows the
// abbreviation's closing parenthesis: `) <angle> +/- <orb> degrees`.
var rowTail = regexp.MustCompile(`^\)\s+([0-9.]+)\s+\+/-\s+([0-9.]+)\s+degrees`)

// haCandidate is one possible reading of a registry row.
type haCandidate struct {
	Name   string
	Abbrev string
	Angle  float64
	Orb    float64
}

// readHARow enumerates every way a registry row could be read, and insists on
// exactly one.
//
// THREE of this row's fields are free text the user controls — `-YAD` sets the
// name, the abbreviation and the trailing glyph, and restricts none of them
// beyond a length floor. All three sit on the same line as the angle and the
// orb, so a row like
//
//	5 Sextile        (Sex)  60    +/- 6   degrees  (Sex) 42 +/- 3 degrees
//
// is genuinely ambiguous: the glyph contains a complete second reading. Every
// regex tried here picked a side and was wrong for some real capture — a
// non-greedy name stopped at a decoy inside the NAME and reported 42 degrees
// with a 3 degree orb; a greedy one fixed that and broke on the same decoy
// inside the GLYPH, which is printed last.
//
// There is no pattern that resolves this, because the ambiguity is in the
// producer's format and not in the reading of it. So this does not resolve it:
// it enumerates the readings and refuses when there is more than one. A
// selection built on a fabricated angle silently regroups the aspect (C5) and
// misreports whether it can be chosen at all (C14); a refusal is C10, which the
// operator sees and can act on.
//
// Enumerating also removes the need to guess where the abbreviation ends, so an
// abbreviation containing a parenthesis — `(X)Y)`, legal and real — reads
// correctly as `X)Y` rather than being refused by an `[^)]+` capture.
func readHARow(line string) ([]haCandidate, bool) {
	h := rowHead.FindStringSubmatchIndex(line)
	if h == nil {
		return nil, false
	}
	body := line[h[1]:]
	var out []haCandidate
	for i, r := range body {
		if r != '(' {
			continue
		}
		for j := i + 1; j < len(body); j++ {
			if body[j] != ')' {
				continue
			}
			m := rowTail.FindStringSubmatch(body[j:])
			if m == nil {
				continue
			}
			angle, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				continue
			}
			orb, err := strconv.ParseFloat(m[2], 64)
			if err != nil {
				continue
			}
			out = append(out, haCandidate{
				Name:   strings.TrimSpace(body[:i]),
				Abbrev: body[i+1 : j],
				Angle:  angle,
				Orb:    orb,
			})
		}
	}
	return out, true
}

// looksLikeRow catches a line that is TRYING to be an aspect row and failing.
// Skipping one silently is C10's failure mode wearing a success: the chooser
// opens missing an aspect the binary actually has, the operator cannot select
// it, and nothing anywhere says why. A registry read either produces the
// binary's set or refuses.
//
// The header (`No. Name  Abbrev. ...`) does not lead with a digit, and the
// unrestricted section is past the break, so the only lines this can reach are
// registry rows.
var looksLikeRow = regexp.MustCompile(`^\s*\d+\s`)

// unrestrictedHeading opens the second half of `-HA`: a walk around the zodiac
// naming every aspect the binary currently COUNTS. It is not part of the
// registry — it lists no index, angle or orb — but it is the only place the
// output says what the host's astrolog.as has restricted, so it is parsed for
// exactly that and nothing else.
const unrestrictedHeading = "Unrestricted aspects"

// gapRow is one of the two things this section legitimately contains: the
// stretches of the zodiac no unrestricted aspect covers. They carry no
// abbreviation and are indented past the column one would start in.
var gapRow = regexp.MustCompile(`^\s+No aspect\s+covers\s`)

// sectionAbbrev resolves one section row to a registry index, or reports that
// the line is not an aspect row at all.
//
// It is a LOOKUP AGAINST A CLOSED SET, not a pattern over free text, and that
// is the whole design. The section prints `<abbrev>: <name>   covers …`, and
// `-YAD` lets a user make the abbreviation any string of three or more
// characters — including one containing a colon. Two successive regexes tried
// to describe that shape and each was wrong for a case the binary really
// emits: `^(\S+):` could not read `S X:`, and `^([^:\s][^:]*):` could not read
// `S:X:` and then REFUSED THE WHOLE REGISTRY over it, taking the chooser down
// for the session on a legitimate host.
//
// The registry half has already been parsed by the time this runs, so every
// abbreviation the binary uses is known. Matching against that set cannot be
// hijacked by whatever a name or an abbreviation happens to contain: longest
// first, so a longer abbreviation is never shadowed by a shorter one that
// prefixes it.
func sectionAbbrev(line string, byAbbrev map[string]int, order []string) (int, bool) {
	for _, ab := range order {
		if strings.HasPrefix(line, ab+":") {
			return byAbbrev[ab], true
		}
	}
	return 0, false
}

// ParseAspectRegistry reads `astrolog -HA` output: the registry itself, and
// then the trailing unrestricted section for each row's InForce.
func ParseAspectRegistry(out string) ([]aspect.RegistryAspect, error) {
	// The producer may COLOUR this output. `PrintAspects` emits an SGR change
	// before every registry row and before the section's rows whenever the
	// host's config turns ANSI on, and every pattern here anchors at byte
	// zero — so an ordinary colour-enabled astrolog.as reported "no aspect
	// rows found" and took the chooser down. Strip first, parse after.
	lines := strings.Split(astrologstyle.StripSGR(out), "\n")

	// The heading is a grammar landmark, and `-YAD` can put a newline into a
	// name, an abbreviation or a glyph — so a field can forge one. Astrolog
	// prints it once; more than one means a field has, and stopping at the
	// first would silently truncate the registry to whatever preceded the
	// forgery.
	headings := 0
	for _, line := range lines {
		if strings.HasPrefix(line, unrestrictedHeading) {
			headings++
		}
	}
	if headings > 1 {
		return nil, fmt.Errorf("astrolog: -HA: %q appears %d times; a renamed field has forged the section heading", unrestrictedHeading, headings)
	}

	var reg []aspect.RegistryAspect
	rest := len(lines)
	for i, line := range lines {
		if strings.HasPrefix(line, unrestrictedHeading) {
			rest = i + 1
			break
		}
		cands, isRow := readHARow(line)
		if !isRow || len(cands) == 0 {
			if looksLikeRow.MatchString(line) {
				return nil, fmt.Errorf("astrolog: -HA: unparseable aspect row %q", strings.TrimSpace(line))
			}
			continue
		}
		// More than one reading means the row's own free-text fields contain
		// something indistinguishable from its data. Guessing produces a
		// fabricated angle and orb, which silently regroups the aspect (C5)
		// and misreports whether it can be selected (C14). Refusing is C10.
		if len(cands) > 1 {
			return nil, fmt.Errorf("astrolog: -HA: row %q can be read %d ways; its name, abbreviation or glyph contains something shaped like its own angle and orb", strings.TrimSpace(line), len(cands))
		}
		c := cands[0]
		idxField := rowHead.FindStringSubmatch(line)[1]
		idx, err := strconv.Atoi(idxField)
		if err != nil {
			return nil, fmt.Errorf("astrolog: -HA: index %q: %w", idxField, err)
		}
		// THE STREAM GRAMMAR, and the fence that candidate-counting cannot
		// provide. Astrolog walks its own aspect array in order, so the
		// registry is numbered 1..N contiguously and ascending.
		//
		// `-YAD` copies a name, an abbreviation and a glyph verbatim, and it
		// restricts no character — INCLUDING THE NEWLINE. So a glyph of
		//
		//	(Sex) 42 +/- 3 degrees\n25 Bogus (Bog) 42 +/- 3 degrees
		//
		// makes the binary emit `25 Bogus (Bog) 42 +/- 3 degrees` as its own
		// physical line between rows 5 and 6 (testdata/ha-injected.txt, a real
		// capture). That line has exactly ONE reading, so enumerating
		// candidates accepts it and the chooser gains an aspect the binary
		// does not have. A forged row can only be caught where it sits in the
		// SEQUENCE, not by how it reads on its own.
		//
		// Contiguity subsumes the two checks that used to be here: an index
		// below 1 and an index seen twice are both non-successors.
		if idx != len(reg)+1 {
			return nil, fmt.Errorf("astrolog: -HA: row %d arrived where %d was expected; this registry is numbered contiguously from 1, so that line is not one the producer emitted here", idx, len(reg)+1)
		}
		reg = append(reg, aspect.RegistryAspect{
			Index:  idx,
			Name:   c.Name,
			Abbrev: c.Abbrev,
			Angle:  c.Angle,
			Orb:    c.Orb,
		})
	}
	if len(reg) == 0 {
		return nil, fmt.Errorf("astrolog: -HA: no aspect rows found")
	}
	if err := readInForce(reg, lines, rest); err != nil {
		return nil, err
	}
	return reg, nil
}

// readInForce stamps InForce onto the registry from the unrestricted section.
//
// It fails closed. A registry Sextile can list but whose restriction state it
// cannot read is worse than no registry at all: the chooser would open showing
// a set the host does not have, and confirming it would put that invented set
// into the chart. C10's refusal is the right outcome for every case here.
func readInForce(reg []aspect.RegistryAspect, lines []string, rest int) error {
	if rest >= len(lines) {
		return fmt.Errorf("astrolog: -HA: no %q section, so which aspects the host counts cannot be read", unrestrictedHeading)
	}

	// The section names aspects by abbreviation and by nothing else, so a
	// binary whose registry repeats one is a binary this cannot resolve.
	byAbbrev := make(map[string]int, len(reg))
	order := make([]string, 0, len(reg))
	for _, r := range reg {
		if _, dup := byAbbrev[r.Abbrev]; dup {
			return fmt.Errorf("astrolog: -HA: abbreviation %q appears twice, so the unrestricted section is ambiguous", r.Abbrev)
		}
		byAbbrev[r.Abbrev] = r.Index
		order = append(order, r.Abbrev)
	}
	// Longest first: an abbreviation that prefixes another must never claim
	// the other's row.
	sort.Slice(order, func(i, j int) bool { return len(order[i]) > len(order[j]) })

	// An EMPTY section is a legitimate answer — `-RA0 -HA` prints the
	// heading, no aspect rows, and `Total aspects (0.00, 0.00%)`. It means
	// the host counts nothing, which is a set, not a parse failure.
	in := map[int]bool{}
	for _, line := range lines[rest:] {
		// Nothing shaped like a registry row belongs past the heading. One
		// there is the same forgery the contiguity fence catches before it,
		// arriving on the other side of the landmark.
		if looksLikeRow.MatchString(line) {
			return fmt.Errorf("astrolog: -HA: a registry row appears after the section heading: %q", strings.TrimSpace(line))
		}
		idx, ok := sectionAbbrev(line, byAbbrev, order)
		if !ok {
			// Symmetry with looksLikeRow above, and the reason this
			// function has two guards rather than one: a line that says
			// `covers` is either an aspect's row or one of the gaps
			// between them. Anything else saying `covers` is a shape this
			// parse does not understand, and CONTINUING past it reports an
			// aspect the host counts as one it does not — silently, with
			// no error anywhere. Refusing is C10's outcome.
			if strings.Contains(line, " covers ") && !gapRow.MatchString(line) {
				return fmt.Errorf("astrolog: -HA: unrestricted-section row names no aspect this registry carries: %q", strings.TrimSpace(line))
			}
			continue
		}
		in[idx] = true
	}
	for i := range reg {
		reg[i].InForce = in[reg[i].Index]
	}
	return nil
}

// AspectRegistry runs `astrolog -HA` and parses it. The argv is BARE on
// purpose: no restriction switch of Sextile's own goes in it, so the InForce
// set that comes back is the host astrolog.as's own — which is exactly what
// "no override in force" means to the chooser, and the set it opens on.
//
// The caller reads this once at startup. Unlike the object registry, the
// registry ROWS do not narrow under the restrictions this program emits, and
// the InForce half is only consulted while no override is in force, so one
// read serves the session.
func AspectRegistry(ctx context.Context) ([]aspect.RegistryAspect, error) {
	out, _, err := spawn(ctx, newExempt("aspect registry: -HA lists the aspect table, no chart", "-HA"))
	if err != nil {
		return nil, err
	}
	return ParseAspectRegistry(out)
}
