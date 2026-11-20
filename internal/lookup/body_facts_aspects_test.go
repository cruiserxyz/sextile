package lookup

import (
	"context"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The cycle search and the Sun pair are called with the lens's aspect registry:
// the call each hands to its searcher holds it, as every body call does.
func TestCycleAndSunPairCallsCarryTheLensAspectRegistry(t *testing.T) {
	var got []aspect.AspectRegistry
	search := func(_ context.Context, call astrolog.BodyCall) (string, string, error) {
		got = append(got, call.Aspects)
		return "", "", nil
	}
	venus := aspect.Object{Number: 4, Name: "Venus"}
	_, _ = SearchCycle(context.Background(), venus, CycleSpan{StartYear: 2016, Years: 3}, Hellenistic, search)
	_, _ = SunPair(context.Background(), venus, time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), location.PlaceInfo{Lat: 51}, Hellenistic, search)
	if len(got) != 2 {
		t.Fatalf("the searcher saw %d calls, want 2", len(got))
	}
	for i, reg := range got {
		if !reg.Ready() {
			t.Errorf("call %d held no aspect registry", i)
		}
	}
}
