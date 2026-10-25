package payload

import (
	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// RawOutput is astrolog's structured stdout after the astrolog has parsed it
// into typed records. The astrolog produces RawOutput; payload carries it
// unchanged to the lookup; the lookup matches rules against it.
//
// Fields:
//
//   - Positions holds every body astrolog reported on.
//   - Aspects holds every body-pair aspect astrolog reported, after NewAspect
//     canonicalisation.
//   - Raw holds the unparsed astrolog stdout for diagnostics. It is never
//     matched against, parsed again, or interpreted downstream.
//
// RawOutput carries no opinion. The astrolog does not tag results as interesting
// or not; selection and arrangement are the lookup's job further upstream.
type RawOutput struct {
	Positions []aspect.Position
	// Unavailable names the objects Astrolog was asked for and could not
	// compute — it has no ephemeris file for them, and returns the no-data
	// sentinel instead of a position. Their rows are dropped from Positions,
	// so this is the only remaining trace of them and the perimeter is
	// expected to tell the operator once.
	Unavailable []string
	BodyStyles  astrologstyle.Bodies
	Aspects     []aspect.Aspect
	// AspectStyles carries Astrolog's exact `-a -k` body/aspect/body spans,
	// keyed independently from the domain records so presentation metadata does
	// not leak into pkg/aspect.
	AspectStyles astrologstyle.Aspects
	Raw          string
	// AspectsReport is the aspect list exactly as Astrolog printed it for
	// this chart (the -aa invocation's stdout minus color, degree sign
	// normalized), so a panel can show the operator the report the parsed
	// Aspects came from without a second spawn.
	AspectsReport string
	// Commands holds one ready-to-paste command line per Astrolog spawn that
	// produced this value, in spawn order (positions first, then aspects).
	// Composed only in internal/astrolog; carried everywhere else, never
	// parsed or re-assembled.
	Commands []string
	// Cusps is this chart's house frame — its twelve cusp longitudes. Zero
	// when Astrolog reported fewer than twelve. The frame travels with the
	// chart so a consumer can place ANOTHER chart's longitudes into these
	// houses; without it a house number can only ever describe the chart
	// that produced it.
	Cusps aspect.Cusps
	// HousesUsed is the house system Astrolog's own header says it used,
	// "" when it printed none (the specification C7). It differs from the
	// one asked for near the poles.
	HousesUsed string
	// MissingEphemeris names the Swiss Ephemeris files Astrolog reported absent
	// on stderr while computing this chart (the expected "SwissEph file '…'
	// not found" lines, which are filtered out of the refusal rule). In spawn
	// order, positions invocation first, each name once; empty when none were
	// reported (the specification C2).
	MissingEphemeris []string
}

// CrossAspectOutput is the role-preserving comparison aspect payload. Styles
// use the same First/Second ordering as Aspects: primary chart first, secondary
// chart second.
type CrossAspectOutput struct {
	Aspects      []aspect.CrossAspect
	AspectStyles astrologstyle.Aspects
	// Report is the relationship aspect list as Astrolog printed it (minus
	// color, degree sign normalized) — the text the Aspects were parsed from.
	Report string
	// Commands is the comparison path's carrier: the ephemeris
	// probe's command followed by the relationship run's, in spawn order.
	// Same rule as RawOutput.Commands — carried, never composed here.
	Commands []string
}
