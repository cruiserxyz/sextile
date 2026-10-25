package lookup

import (
	"context"
	"fmt"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/store"
)

// FirstCallNatalPositions is the natal-SET first-frame query. Computes the
// natal chart at chart.When for chart.Place under `lens` and returns the
// positions + intra-natal aspects.
//
// cp3 contract: lens still pinned to a Go constant (Hellenistic). The
// internal/astrolog → lookup_rules.json wiring lands once the bake pipeline hands
// off baked rules; until then this routes through the same ComputeChart
// invocation as transits, just at a different moment.
func FirstCallNatalPositions(ctx context.Context, probers *Probers, chart store.AnchorChart, lens Lens) (Result, error) {
	if chart.IsZero() {
		return Result{}, fmt.Errorf("lookup: FirstCallNatalPositions: chart is zero")
	}
	if !chart.Place.HasTimezone() {
		return Result{}, fmt.Errorf("lookup: FirstCallNatalPositions: place %q has no IANA timezone", chart.Place.Display())
	}
	jd := astrolog.SlotJD(chart.When)
	spec, err := lens.chartSpecAt(jd, chart.Place, chart.Label)
	if err != nil {
		return Result{}, fmt.Errorf("lookup: FirstCallNatalPositions: %w", err)
	}
	raw, err := probers.full()(ctx, spec)
	if err != nil {
		// A spawn that ran and then failed still happened: its command rides
		// beside the wrapped error instead of vanishing with a zero value
		//.
		return Result{Commands: raw.Commands}, fmt.Errorf("lookup: FirstCallNatalPositions: %w", err)
	}
	// Applying/separating arrives on each aspect from Astrolog's -aa list
	// (the natal chart's own velocities at birth); no t+ε probe.
	return Result{
		Positions:     raw.Positions,
		BodyStyles:    raw.BodyStyles,
		Aspects:       raw.Aspects,
		AspectStyles:  raw.AspectStyles,
		AspectsReport: raw.AspectsReport,
		Commands:      raw.Commands,
		Cusps:         raw.Cusps,
		HousesUsed:    raw.HousesUsed,
		Unavailable:   raw.Unavailable,
	}, nil
}
