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

// applying/separating is Astrolog's verdict, read off the -aa list by
// the astrolog parser and carried through lookup untouched. These tests pin
// that lookup neither derives nor overrides it, and no longer spawns a t+ε
// positions-only probe to do so.

func phaseStub(t *testing.T, calls *int) ChartComputer {
	t.Helper()
	return func(ctx context.Context, spec recipe.ChartSpec) (payload.RawOutput, error) {
		*calls++
		applying := aspect.NewAspect(aspect.Sun, aspect.Moon, aspect.Conjunction, aspect.Orb(2))
		applying.Applying = true
		separating := aspect.NewAspect(aspect.Sun, aspect.Mars, aspect.Trine, aspect.Orb(1))
		return payload.RawOutput{
			Positions: []aspect.Position{{Body: aspect.Sun, Longitude: 0}, {Body: aspect.Moon, Longitude: 2}, {Body: aspect.Mars, Longitude: 121}},
			Aspects:   []aspect.Aspect{applying, separating},
		}, nil
	}
}

func assertPhasePassThrough(t *testing.T, res Result) {
	t.Helper()
	if len(res.Aspects) != 2 {
		t.Fatalf("want 2 aspects, got %d", len(res.Aspects))
	}
	if !res.Aspects[0].Applying {
		t.Error("Astrolog's applying verdict was lost on the way through lookup")
	}
	if res.Aspects[1].Applying {
		t.Error("Astrolog's separating verdict was overridden on the way through lookup")
	}
}

func TestFirstCallTransits_CarriesAstrologPhaseWithoutProbe(t *testing.T) {
	var full, probe int
	probers := &Probers{Full: phaseStub(t, &full), PositionsOnly: phaseStub(t, &probe)}
	place := location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"}
	res, err := FirstCallTransits(context.Background(), probers, time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC), place, Hellenistic)
	if err != nil {
		t.Fatalf("FirstCallTransits: %v", err)
	}
	assertPhasePassThrough(t, res)
	if full != 1 || probe != 0 {
		t.Errorf("computer calls full=%d probe=%d, want 1/0 (no t+ε probe since an earlier change)", full, probe)
	}
}

func TestFirstCallNatalPositions_CarriesAstrologPhaseWithoutProbe(t *testing.T) {
	var full, probe int
	probers := &Probers{Full: phaseStub(t, &full), PositionsOnly: phaseStub(t, &probe)}
	chart := store.AnchorChart{
		When:  time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC),
		Place: location.PlaceInfo{Name: "Amsterdam", Country: "NL", Lat: 52.3676, Lon: 4.9041, TZ: "Europe/Amsterdam"},
	}
	res, err := FirstCallNatalPositions(context.Background(), probers, chart, Hellenistic)
	if err != nil {
		t.Fatalf("FirstCallNatalPositions: %v", err)
	}
	assertPhasePassThrough(t, res)
	if full != 1 || probe != 0 {
		t.Errorf("computer calls full=%d probe=%d, want 1/0 (no t+ε probe since an earlier change)", full, probe)
	}
}

// TestOverlayAspectsFromCross: roles are laid out natal/transit from
// primary/secondary and Astrolog's phase verdict rides along unchanged.
func TestOverlayAspectsFromCross(t *testing.T) {
	cross := []aspect.CrossAspect{
		{Primary: aspect.Sun, Secondary: aspect.Saturn, Kind: aspect.Square, Orb: 0.5, Power: 9, Applying: true},
		{Primary: aspect.Moon, Secondary: aspect.Jupiter, Kind: aspect.Trine, Orb: 1.4, Power: 6},
	}
	out := overlayAspectsFromCross(cross)
	if len(out) != 2 {
		t.Fatalf("want 2, got %d", len(out))
	}
	if out[0].Natal != aspect.Sun || out[0].Transit != aspect.Saturn || !out[0].Applying || out[0].Power != 9 {
		t.Errorf("first = %+v, want natal Sun / transit Saturn, applying, power 9", out[0])
	}
	if out[1].Natal != aspect.Moon || out[1].Transit != aspect.Jupiter || out[1].Applying {
		t.Errorf("second = %+v, want natal Moon / transit Jupiter, separating", out[1])
	}
}

// TestComparisonSpecIsTransitToNatal pins the relation the overlay and the
// raw comparison report compile under: -rt, so Astrolog zeroes the natal
// velocities before judging phase.
func TestComparisonSpecIsTransitToNatal(t *testing.T) {
	spec, err := Hellenistic.comparisonSpecAt(2446232, location.PlaceInfo{Lon: 4.9, Lat: 52.4, TZ: "Europe/Amsterdam"}, 2461000, location.PlaceInfo{Lon: 13.4, Lat: 52.5, TZ: "Europe/Berlin"})
	if err != nil {
		t.Fatal(err)
	}
	c, ok := spec.Source.(recipe.Comparison)
	if !ok || c.Relation != recipe.TransitToNatal {
		t.Errorf("comparison relation = %+v, want TransitToNatal", spec.Source)
	}
}
