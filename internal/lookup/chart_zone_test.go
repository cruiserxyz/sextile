package lookup

// the specification N1, C4: the single-chart compute receives the slot's
// IANA zone; an unloadable zone refuses before any spawn.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/payload"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

func TestChartCallRefusesAnUnloadableZone(t *testing.T) {
	when := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	bad := location.PlaceInfo{Name: "Nowhere", Country: "XX", Lat: 10, Lon: 10, TZ: "Mars/Olympus_Mons"}
	good := location.PlaceInfo{Name: "Amsterdam", Country: "NL", Lat: 52.37, Lon: 4.9, TZ: "Europe/Amsterdam"}

	var got []recipe.ChartSpec
	stub := ChartComputer(func(ctx context.Context, spec recipe.ChartSpec) (payload.RawOutput, error) {
		got = append(got, spec)
		return payload.RawOutput{}, nil
	})
	probers := &Probers{Full: stub, PositionsOnly: stub}

	calls := map[string]func(place location.PlaceInfo) error{
		"FirstCallTransits": func(place location.PlaceInfo) error {
			_, err := FirstCallTransits(context.Background(), probers, when, place, Hellenistic)
			return err
		},
		"FirstCallNatalPositions": func(place location.PlaceInfo) error {
			_, err := FirstCallNatalPositions(context.Background(), probers, store.AnchorChart{When: when, Place: place}, Hellenistic)
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			got = nil
			err := call(bad)
			if err == nil || !strings.Contains(err.Error(), "Mars/Olympus_Mons") {
				t.Fatalf("err = %v, want one naming the zone", err)
			}
			if len(got) != 0 {
				t.Fatalf("the compute ran %d times for an unloadable zone", len(got))
			}
			if err := call(good); err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 {
				t.Fatalf("computes = %d, want 1", len(got))
			}
			src, ok := got[0].Source.(recipe.MomentAtPlace)
			if !ok || src.TZ != "Europe/Amsterdam" {
				t.Errorf("source = %#v, want a MomentAtPlace carrying the slot's zone", got[0].Source)
			}
		})
	}
}
