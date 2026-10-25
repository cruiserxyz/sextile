package lookup

import (
	"context"
	"fmt"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// OverlayAspect is a transit→natal cross-aspect ready for display. Unlike
// aspect.Aspect it preserves ROLES — Transit is the moving chart's body, Natal
// the fixed chart's — because aspect.NewAspect canonicalizes A<=B and would
// collapse the pair. Power is astrolog's real -r0 strength; Applying is
// Astrolog's own -rt -aa verdict: the natal chart's velocities are
// zeroed, so the phase follows the transiting body's motion against the
// fixed natal position. Nothing is derived in Sextile.
type OverlayAspect struct {
	Transit  aspect.Body
	Natal    aspect.Body
	Kind     aspect.AspectKind
	Orb      aspect.Orb
	Power    float64
	Applying bool

	// TransitLabel and NatalLabel are the names Astrolog printed for a side the
	// Body enum cannot name, and are empty for every body it can. Nothing reads
	// a label when its Body is set; a labelled side is BodyUnknown, renders as a
	// placeholder, and is not addressable as a typed body.
	TransitLabel string
	NatalLabel   string
}

// OverlayAspects keeps Astrolog's role-preserving style map beside the display
// records. AspectStyles is keyed primary/natal first and secondary/transit
// second, exactly as the `-r0` output emitted it — the order the Aspects cell
// prints too.
type OverlayAspects struct {
	Aspects      []OverlayAspect
	AspectStyles astrologstyle.Aspects
	// Report is the -rt -aa list as Astrolog printed it (minus color): the
	// text the Aspects were parsed from, shown verbatim in Details.
	Report string
	// Commands is the comparison path's carrier: the ephemeris
	// probe's command followed by the relationship run's, in spawn order,
	// carried from payload unchanged — beside the error too when a spawn
	// ran and then failed (C14).
	Commands []string
}

// OverlayCrossAspects computes transit→natal cross-aspects via astrolog's -rt
// -aa relationship run: real power, real orb, role-preserving, and the
// applying/separating verdict from Astrolog itself with the natal chart held
// still. One spawn; no position snapshots, no t+ε probe. The natal
// chart's header carries natalChart.Label; the transiting chart is the floating
// slot and is unnamed.
func OverlayCrossAspects(
	ctx context.Context,
	lens Lens,
	natalChart store.AnchorChart,
	transitWhen time.Time,
	transitPlace location.PlaceInfo,
) (OverlayAspects, error) {
	// Both staged charts carry their own local clock and zone, so the report
	// Details shows reads each chart's own time (the specification C6); the
	// cross-aspect values are the UT run's, to the byte.
	spec, err := lens.localComparisonSpecAt(
		astrolog.SlotJD(natalChart.When), natalChart.Place, natalChart.Label,
		astrolog.SlotJD(transitWhen), transitPlace, store.ReportLabel{},
	)
	if err != nil {
		return OverlayAspects{}, fmt.Errorf("lookup: OverlayCrossAspects: %w", err)
	}
	cross, err := astrolog.ComputeCrossAspects(ctx, spec)
	if err != nil {
		// The probe and relationship spawns that ran before the failure
		// still happened; their commands ride beside the error.
		return OverlayAspects{Commands: cross.Commands}, fmt.Errorf("lookup: OverlayCrossAspects: %w", err)
	}
	return OverlayAspects{
		Aspects:      overlayAspectsFromCross(cross.Aspects),
		AspectStyles: cross.AspectStyles,
		Report:       cross.Report,
		Commands:     cross.Commands,
	}, nil
}

// SynastryCrossAspects computes the cross-aspects between two ANCHORED charts
// via astrolog's -r0 -aa relationship run: chart 1 (natalChart, the baseline)
// is Primary, chart 2 (otherWhen at otherPlace) is Secondary. It is
// OverlayCrossAspects with Relation Synastry instead of TransitToNatal; the
// result type is shared, and a row's Transit field is chart 2's body, Natal
// chart 1's. Applying is Astrolog's synthetic both-natal-motions verdict, which
// a pair does not display. Each chart's header names it: chart 1 by
// natalChart.Label, chart 2 by otherLabel (the specification C1).
func SynastryCrossAspects(
	ctx context.Context,
	lens Lens,
	natalChart store.AnchorChart,
	otherWhen time.Time,
	otherPlace location.PlaceInfo,
	otherLabel store.ReportLabel,
) (OverlayAspects, error) {
	spec, err := lens.localSynastrySpecAt(
		astrolog.SlotJD(natalChart.When), natalChart.Place, natalChart.Label,
		astrolog.SlotJD(otherWhen), otherPlace, otherLabel,
	)
	if err != nil {
		return OverlayAspects{}, fmt.Errorf("lookup: SynastryCrossAspects: %w", err)
	}
	cross, err := astrolog.ComputeCrossAspects(ctx, spec)
	if err != nil {
		return OverlayAspects{Commands: cross.Commands}, fmt.Errorf("lookup: SynastryCrossAspects: %w", err)
	}
	return OverlayAspects{
		Aspects:      overlayAspectsFromCross(cross.Aspects),
		AspectStyles: cross.AspectStyles,
		Report:       cross.Report,
		Commands:     cross.Commands,
	}, nil
}

// overlayAspectsFromCross maps astrolog's generic []CrossAspect (Primary=chart1=
// natal, Secondary=chart2=transit) to display []OverlayAspect with the roles
// laid out and Astrolog's phase verdict carried through unchanged. Pure.
func overlayAspectsFromCross(cross []aspect.CrossAspect) []OverlayAspect {
	out := make([]OverlayAspect, 0, len(cross))
	for _, c := range cross {
		out = append(out, OverlayAspect{
			Transit:  c.Secondary,
			Natal:    c.Primary,
			Kind:     c.Kind,
			Orb:      c.Orb,
			Power:    c.Power,
			Applying: c.Applying,

			TransitLabel: c.SecondaryLabel,
			NatalLabel:   c.PrimaryLabel,
		})
	}
	return out
}
