package astrolog

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
)

// ErrEphemerisDisabled means astrolog's Swiss Ephemeris was not active when
// the chart was computed — the canonical symptom is every body rendering as
// "0Ari00". The invocation almost certainly contained a naked toggle flag
// (typically -b) that un-set the astrolog.as preset's =b bit. Positions in
// this state are meaningless and must not flow to the lookup.
//
// Provenance: bake-pipeline neuron astrolog-switch-toggle-semantics (removed in Phase 1)
// (canonical_symptom.forbid_pattern).
var ErrEphemerisDisabled = errors.New("astrolog: Swiss Ephemeris disabled — multiple bodies render as 0Ari00")

// reAspectLineShape loosely matches a numbered aspect line ("  N:  … - orb:") —
// the broad shape, used by checkCrossAspectSanity to distinguish "no aspect
// output at all" from "lines that look like aspects".
var reAspectLineShape = regexp.MustCompile(`(?m)^\s*\d+:.*-\s+orb:`)

// reChart2Header is the second chart's header line of a relationship run. It
// is matched at the START of a line: a chart's NAME prints inside the first
// line (`chart for <name>`), so the words anywhere in the text prove nothing
// (the specification C4, N15).
var reChart2Header = regexp.MustCompile(`(?m)^\s*Chart #2 comparison for `)

// checkCrossAspectSanity guards ComputeCrossAspects against silently returning
// an empty result from non-aspect output. astrolog prints usage/diagnostic text
// to stdout even at exit 0, and the -r0 -a path parses stdout directly (there is
// no positions block for checkPositionSanity to scan). Two failure shapes are
// caught:
//   - no chart-2 comparison header → the run produced no relationship chart at all
//     (usage error / bad argv);
//   - aspect-shaped lines exist but the STRICT parser pattern (crossAspectLinePat)
//     matches NONE of them → the line FORMAT drifted past the parser.
//
// Crucially, "aspect-shaped lines but zero PARSED" is NOT itself fatal:
// parseCrossAspects intentionally skips lines with unknown bodies / unsupported
// aspect kinds (exactly as the single-chart parser does). Those lines still match
// the strict pattern, so a run that is all-unsupported filters to empty WITHOUT
// erroring — only a true format change (loose match, no strict match) is drift.
func checkCrossAspectSanity(stdout string) error {
	plain := astrologstyle.StripSGR(stdout)
	if !reChart2Header.MatchString(plain) {
		return fmt.Errorf("astrolog: cross-aspect run produced no comparison header (usage/diagnostic stdout?)")
	}
	if reAspectLineShape.MatchString(plain) && !crossAspectLinePat.MatchString(plain) {
		return fmt.Errorf("astrolog: cross-aspect output has aspect-shaped lines none of which match the parser (format drift?)")
	}
	return nil
}

// checkPositionSanity returns ErrEphemerisDisabled when EVERY body row in the
// positions output sits at 0Ari00 — the collapse this guard was written for,
// where a naked toggle flag un-set the astrolog.as `=b` bit and the ephemeris
// stopped answering at all.
//
// It scans BODY ROWS, not the whole stream, and that distinction is the whole
// correctness of it. Astrolog prints the house-cusp table in a second column of
// the same lines, and a whole-sign chart ALWAYS has one cusp at `0Ari00`
// (whichever house begins in Aries):
//
//	Sun : 24Gem19 … -  House cusp  7:  0Ari00
//
// The previous implementation counted `0Ari00` anywhere in stdout and fired at
// two or more, so that cusp was one match at baseline and any single body the
// installation cannot compute was the second. Selecting one of the forty
// objects this Astrolog has no Swiss Ephemeris file for — Eris, Hygiea, the
// planetary moons — therefore blanked the Bodies and Aspects panels entirely.
//
// The other half of the change is "every" rather than "two or more". An object
// with no ephemeris file comes back at 0Ari00 with zero velocity while the rest
// of the chart is fine; that is a gap in the user's Astrolog data, not a
// collapsed ephemeris, and it must not condemn the whole frame. A genuine
// collapse takes every body down together, which is exactly what this now
// requires.
func checkPositionSanity(stdout string) error {
	plain := astrologstyle.StripSGR(stdout)
	collapsed := 0
	for _, line := range strings.Split(plain, "\n") {
		m := bodyRowLongitudePat.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		// m[1..3] are the degrees, sign and minutes of the BODY's own
		// position — the left column, never the cusp table beside it.
		if m[1] != "0" || m[2] != "Ari" || m[3] != "00" {
			continue
		}
		// A row at 0Ari00 is evidence of collapse ONLY if it is moving. An
		// object this installation has no ephemeris file for also sits at
		// 0Ari00, but with zero latitude and zero velocity, and that is a gap
		// in the user's Astrolog data rather than a dead ephemeris — it is
		// dropped and reported by name, not treated as a broken chart.
		//
		// Without this, selecting two such objects (Eris and Io, say) made
		// every body row a sentinel, and "all collapsed" condemned the frame:
		// the operator got blank panels for a selection that should have drawn
		// nothing and said why. The two states are genuinely distinguishable —
		// a real collapse leaves the bodies MOVING (the canonical fixture
		// carries +0.993, +11.93, +0.421, -0.320) — so this asks the question
		// that separates them instead of counting rows.
		if full := bodyPositionPat.FindStringSubmatch(line); full != nil && isNoDataRow(full) {
			continue
		}
		collapsed++
	}
	// Two or more moving bodies stacked on the vernal point is the symptom. One
	// is legitimate — a body really can be at 0° Aries 0'.
	if collapsed >= 2 {
		return ErrEphemerisDisabled
	}
	return nil
}

// bodyRowLongitudePat matches a body row's NAME and its own longitude, and
// nothing else about the row.
//
// It is deliberately looser than bodyPositionPat, which this guard used at
// first: that one also requires the latitude, house and velocity fields, so a
// truncated or unusual row parsed as "not a body row at all" and the guard
// silently measured nothing. A sanity check that runs on raw text, before
// parsing, must not inherit the parser's strictness — its whole job is to
// answer a question about output the parser may be unable to read.
//
// The `^` anchor is what keeps the house-cusp column out: astrolog prints
// `-  House cusp  7:  0Ari00` in the right-hand column of these same lines, and
// only the body's own position sits at the start of one.
var bodyRowLongitudePat = regexp.MustCompile(`^\w{1,4}\s*:\s+(\d+)([A-Z][a-z]{2})(\d+)`)
