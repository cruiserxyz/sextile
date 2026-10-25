package lookup

import (
	"context"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/payload"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// TestOverlayFrame_ExecBudget asserts the natal-set overlay frame's astrolog
// exec budget over the Probers seam, where a full ComputeChart call counts as
// 2 units (positions + -aa aspects spawns) and a positions-only call as 1.
// History: 10 units pre-an earlier change; 6 after an earlier change cut the applying probes to
// positions-only and reused the transit t+ε frame; 4 after an earlier change retired the
// probes entirely — applying/separating now arrives on the -aa list itself.
// The -rt cross-aspect spawn runs outside this seam (astrolog.ComputeCrossAspects)
// and is unchanged by an earlier change: it was one spawn before and is one spawn now.
//
// If this test fails after a future change, EITHER a spawn crept back in OR
// the budget genuinely needs updating after a measured perf-affecting feature
// lands. Update the constant deliberately with the new measured number.
func TestOverlayFrame_ExecBudget(t *testing.T) {
	const wantBudget = 4

	var fullCalls, positionsOnlyCalls int
	full := ChartComputer(func(ctx context.Context, spec recipe.ChartSpec) (payload.RawOutput, error) {
		fullCalls++
		return payload.RawOutput{
			Positions: []aspect.Position{{Body: aspect.Sun, Longitude: moment(t, spec).JD}},
			Aspects:   []aspect.Aspect{},
		}, nil
	})
	positionsOnly := ChartComputer(func(ctx context.Context, spec recipe.ChartSpec) (payload.RawOutput, error) {
		positionsOnlyCalls++
		return payload.RawOutput{
			Positions: []aspect.Position{{Body: aspect.Moon, Longitude: moment(t, spec).JD}},
		}, nil
	})
	probers := &Probers{Full: full, PositionsOnly: positionsOnly}

	natalPlace := location.PlaceInfo{
		Name: "Amsterdam", Country: "NL",
		Lat: 52.3676, Lon: 4.9041, TZ: "Europe/Amsterdam",
	}
	transitPlace := location.PlaceInfo{
		Name: "Berlin", Country: "DE",
		Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin",
	}
	natalChart := store.AnchorChart{
		When:  time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC),
		Place: natalPlace,
	}
	transitTime := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)

	// This is the call sequence computeOverlayCmd executes in ui/startup.go
	// through the Probers seam (the -rt cross-aspect spawn is outside it).
	ctx := context.Background()
	if _, err := FirstCallNatalPositions(ctx, probers, natalChart, Hellenistic); err != nil {
		t.Fatalf("FirstCallNatalPositions: %v", err)
	}
	if _, err := FirstCallTransits(ctx, probers, transitTime, transitPlace, Hellenistic); err != nil {
		t.Fatalf("FirstCallTransits: %v", err)
	}

	gotBudget := fullCalls*2 + positionsOnlyCalls*1
	if gotBudget > wantBudget {
		t.Errorf("overlay frame exec budget = %d (full=%d×2 + positionsOnly=%d×1); want ≤%d",
			gotBudget, fullCalls, positionsOnlyCalls, wantBudget)
	}
	// Pin the shape so a failure names the call site that changed.
	if fullCalls != 2 {
		t.Errorf("fullCalls = %d, want 2 (FirstCallNatalPositions + FirstCallTransits)", fullCalls)
	}
	if positionsOnlyCalls != 0 {
		t.Errorf("positionsOnlyCalls = %d, want 0", positionsOnlyCalls)
	}
}
