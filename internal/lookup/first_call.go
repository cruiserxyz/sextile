package lookup

import (
	"context"
	"fmt"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/payload"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// ChartComputer is the chart-compute function shape FirstCallTransits calls. It
// takes the typed recipe.ChartSpec lookup builds from (moment, place, lens) —
// not loose primitives — so the astrolog seam speaks chart semantics end to end.
// Production code passes nil to use astrolog.ComputeChart; tests inject a stub
// so the package can be exercised without spawning the astrolog binary.
type ChartComputer func(ctx context.Context, spec recipe.ChartSpec) (payload.RawOutput, error)

// Probers bundles the full and positions-only chart-compute functions
// the lookup layer dispatches through. The split dates from the
// finite-difference applying/separating probes, which an earlier change
// retired in favour of Astrolog's own -aa phase markers; since then no
// lookup path spawns a positions-only compute, and PositionsOnly is kept
// only as the measurement seam the exec-budget test reads. Per
// WR-6 / an earlier change the goal is to bring natal-set first-frame astrolog
// execs from ~10 to ≤6 — load-bearing for the held-key time-step
// sweep follow-up.
//
// Nil receiver, or nil fields within, fall back to the bundled
// astrolog wrappers. Tests inject separate spy stubs into Full and
// PositionsOnly so a weighted measurement (full=2 exec units,
// positions-only=1 unit) can be asserted.
type Probers struct {
	Full          ChartComputer
	PositionsOnly ChartComputer
}

// full returns the full chart-compute function: Probers.Full when set,
// otherwise astrolog.ComputeChart.
func (p *Probers) full() ChartComputer {
	if p != nil && p.Full != nil {
		return p.Full
	}
	return astrolog.ComputeChart
}

// positionsOnly returns the positions-only chart-compute function:
// Probers.PositionsOnly when set, otherwise astrolog.ComputeChartPositionsOnly.
func (p *Probers) positionsOnly() ChartComputer {
	if p != nil && p.PositionsOnly != nil {
		return p.PositionsOnly
	}
	return astrolog.ComputeChartPositionsOnly
}

// probersFromComputer wraps a single ChartComputer as Probers where
// both Full and PositionsOnly point at the same stub. Preserves
// backward compatibility for callers that haven't migrated to
// Probers yet. Returns nil if computer is nil (so defaults to astrolog
// bindings).
func probersFromComputer(computer ChartComputer) *Probers {
	if computer == nil {
		return nil
	}
	return &Probers{Full: computer, PositionsOnly: computer}
}

// FirstCallTransits is the natal-UNSET + location-known first-frame query.
// It computes the here-now chart at `now` from `place` under `lens`, and
// returns positions + aspects in a Result.
//
// The cp2 contract pins the lens as a Go constant (Hellenistic). Once
// lenses.json lands in cp3+ the lens registry crosses through internal/
// astrolog instead.
//
// Errors are wrapped with %w. Callers may errors.Is the underlying chart-
// compute error to detect astrolog-side failures (ErrUnsafeArgv, exec
// errors, ErrEphemerisDisabled).
func FirstCallTransits(ctx context.Context, probers *Probers, now time.Time, place location.PlaceInfo, lens Lens) (Result, error) {
	if !place.HasTimezone() {
		return Result{}, fmt.Errorf("lookup: FirstCallTransits: place %q has no IANA timezone", place.Display())
	}
	jd := astrolog.SlotJD(now)
	spec, err := lens.chartSpecAt(jd, place, store.ReportLabel{})
	if err != nil {
		return Result{}, fmt.Errorf("lookup: FirstCallTransits: %w", err)
	}
	raw, err := probers.full()(ctx, spec)
	if err != nil {
		// A spawn that ran and then failed still happened: its command rides
		// beside the wrapped error instead of vanishing with a zero value
		//.
		return Result{Commands: raw.Commands}, fmt.Errorf("lookup: FirstCallTransits: %w", err)
	}
	// Applying/separating arrives on each aspect from Astrolog's -aa list;
	// the t+ε positions-only probe is gone, so a chart frame is the
	// one full compute.
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
