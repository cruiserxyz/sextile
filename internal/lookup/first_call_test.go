package lookup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/payload"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

func TestFirstCallTransits_RejectsPlaceWithoutTimezone(t *testing.T) {
	place := location.PlaceInfo{Name: "Atlantis", Country: "??", Lat: 0, Lon: 0, TZ: ""}
	called := false
	stub := ChartComputer(func(ctx context.Context, spec recipe.ChartSpec) (payload.RawOutput, error) {
		called = true
		return payload.RawOutput{}, nil
	})
	_, err := FirstCallTransits(context.Background(), &Probers{Full: stub, PositionsOnly: stub}, time.Now(), place, Hellenistic)
	if err == nil {
		t.Fatal("expected error for place without IANA timezone")
	}
	if called {
		t.Error("ChartComputer must not be invoked when place has no timezone")
	}
}

func TestFirstCallTransits_PassesLensAndCoords(t *testing.T) {
	place := location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"}
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)

	// an earlier change split the lookup into Full vs PositionsOnly probers. This
	// test only cares about the MAIN chart compute (the Full path);
	// using separate stubs prevents the probe's positions-only call
	// from overwriting the captured spec (a review finding).
	var gotJD, gotLon, gotLat float64
	var gotHouses recipe.HouseSystem
	var gotZodiac recipe.Zodiac
	var gotTrueNode bool
	var gotAdd recipe.BodySet
	aspectStyles := astrologstyle.Aspects{
		{First: aspect.Sun, Second: aspect.Moon, Kind: aspect.Square}: {
			First: "sun", Kind: "square", Second: "moon",
		},
	}
	full := ChartComputer(func(ctx context.Context, spec recipe.ChartSpec) (payload.RawOutput, error) {
		m := moment(t, spec)
		gotJD, gotLon, gotLat = m.JD, m.Lon, m.Lat
		gotHouses, gotZodiac = spec.Houses, spec.Zodiac
		gotTrueNode, gotAdd = spec.Objects.TrueNodeMode, spec.Objects.Add
		return payload.RawOutput{
			Positions:    []aspect.Position{{Body: aspect.Sun, Longitude: 35.5}},
			Aspects:      []aspect.Aspect{},
			AspectStyles: aspectStyles,
		}, nil
	})
	probe := ChartComputer(func(ctx context.Context, spec recipe.ChartSpec) (payload.RawOutput, error) {
		return payload.RawOutput{}, nil
	})

	result, err := FirstCallTransits(context.Background(), &Probers{Full: full, PositionsOnly: probe}, now, place, Hellenistic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotLon != 13.4105 || gotLat != 52.5244 {
		t.Errorf("coords mismatch: lon=%v lat=%v", gotLon, gotLat)
	}
	if gotHouses != recipe.WholeSign {
		t.Errorf("Hellenistic houses should be Whole Sign, got %v", gotHouses)
	}
	if gotZodiac != recipe.Tropical {
		t.Error("Hellenistic must be tropical, not sidereal")
	}
	// B2: the body config (true node/Lilith + the v1 body set) must thread
	// through the lens to the computer as typed values.
	if !gotTrueNode {
		t.Error("Hellenistic true-node mode should pass through as true (-Yn)")
	}
	wantAdd := recipe.NewBodySet(aspect.Chiron, aspect.SouthNode, aspect.Lilith, aspect.ImumCoeli, aspect.Descendant)
	if !sameBodySet(gotAdd, wantAdd) {
		t.Errorf("Add body set = %v, want %v", gotAdd.Members(), wantAdd.Members())
	}
	if gotJD < 2461000 || gotJD > 2462000 {
		t.Errorf("JD looks wrong for 2026-04-25: %v", gotJD)
	}
	if len(result.Positions) != 1 || result.Positions[0].Body != aspect.Sun {
		t.Errorf("Result.Positions did not pass through: %+v", result.Positions)
	}
	if got := result.AspectStyles[astrologstyle.AspectIdentity{First: aspect.Sun, Second: aspect.Moon, Kind: aspect.Square}]; got.Kind != "square" {
		t.Errorf("Result.AspectStyles did not pass through: %#v", result.AspectStyles)
	}
}

// TestFirstCallTransits_DifferentCitiesHitComputerWithDifferentCoords proves
// that switching cities at the same `now` propagates the new (lat, lon) to
// the chart computer. The user's "Dubai → Berlin moon stays the same"
// observation can have multiple causes; this test rules out the worst one
// (compute didn't fire / wrong coords). Astrolog returns geocentric
// longitudes by default, so the moon's longitude legitimately won't change
// substantially for the same UTC instant — but the (lat, lon) MUST flow
// through to the computer so houses (and topocentric corrections, when
// enabled) can do their work.
func TestFirstCallTransits_DifferentCitiesHitComputerWithDifferentCoords(t *testing.T) {
	type call struct {
		jd       float64
		lon, lat float64
	}
	var calls []call
	stub := ChartComputer(func(ctx context.Context, spec recipe.ChartSpec) (payload.RawOutput, error) {
		m := moment(t, spec)
		calls = append(calls, call{jd: m.JD, lon: m.Lon, lat: m.Lat})
		// Synthetic moon position derived from lat — proves the model would
		// see different positions if astrolog actually responded to lat/lon.
		return payload.RawOutput{
			Positions: []aspect.Position{{Body: aspect.Moon, Longitude: 100.0 + m.Lat*0.01}},
		}, nil
	})

	dubai := location.PlaceInfo{Name: "Dubai", Country: "AE", Lat: 25.276, Lon: 55.296, TZ: "Asia/Dubai"}
	berlin := location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"}
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)

	dubaiResult, err := FirstCallTransits(context.Background(), &Probers{Full: stub, PositionsOnly: stub}, now, dubai, Hellenistic)
	if err != nil {
		t.Fatalf("Dubai compute: %v", err)
	}
	berlinResult, err := FirstCallTransits(context.Background(), &Probers{Full: stub, PositionsOnly: stub}, now, berlin, Hellenistic)
	if err != nil {
		t.Fatalf("Berlin compute: %v", err)
	}

	// Each FirstCallTransits makes ONE computer call: the chart at `now`.
	// (The applying/separating probe at now+ε that once doubled this was
	// retired by an earlier change — the phase now rides on the -aa list itself.)
	if len(calls) != 2 {
		t.Fatalf("expected 2 computer calls (one chart per city), got %d", len(calls))
	}
	dubaiCall, berlinCall := calls[0], calls[1]
	if dubaiCall.lat != dubai.Lat || dubaiCall.lon != dubai.Lon {
		t.Errorf("Dubai chart call: got (lat=%v, lon=%v), want (%v, %v)",
			dubaiCall.lat, dubaiCall.lon, dubai.Lat, dubai.Lon)
	}
	if berlinCall.lat != berlin.Lat || berlinCall.lon != berlin.Lon {
		t.Errorf("Berlin chart call: got (lat=%v, lon=%v), want (%v, %v)",
			berlinCall.lat, berlinCall.lon, berlin.Lat, berlin.Lon)
	}
	if dubaiCall.lat == berlinCall.lat && dubaiCall.lon == berlinCall.lon {
		t.Error("Dubai and Berlin reached the computer with identical coords")
	}
	// Same `now` → same chart JD across cities.
	if dubaiCall.jd != berlinCall.jd {
		t.Errorf("chart JD differs across cities at same now: %v vs %v", dubaiCall.jd, berlinCall.jd)
	}
	// And: the Result that came back IS different (because the stub keys on lat).
	if dubaiResult.Positions[0].Longitude == berlinResult.Positions[0].Longitude {
		t.Error("Result.Positions identical across cities — wiring did not propagate lat/lon")
	}
}

func TestFirstCallTransits_WrapsComputerError(t *testing.T) {
	place := location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"}
	sentinel := errors.New("astrolog crashed")
	stub := ChartComputer(func(ctx context.Context, spec recipe.ChartSpec) (payload.RawOutput, error) {
		return payload.RawOutput{}, sentinel
	})
	_, err := FirstCallTransits(context.Background(), &Probers{Full: stub, PositionsOnly: stub}, time.Now(), place, Hellenistic)
	if !errors.Is(err, sentinel) {
		t.Errorf("expected wrapped sentinel, got %v", err)
	}
}

// moment extracts the MomentAtPlace source from a spec for assertions; it fails
// the test if the source is some other (future) variant.
func moment(t *testing.T, spec recipe.ChartSpec) recipe.MomentAtPlace {
	t.Helper()
	m, ok := spec.Source.(recipe.MomentAtPlace)
	if !ok {
		t.Fatalf("spec.Source is %T, want recipe.MomentAtPlace", spec.Source)
	}
	return m
}

// sameBodySet reports whether two body sets have identical members.
func sameBodySet(a, b recipe.BodySet) bool {
	am, bm := a.Members(), b.Members()
	if len(am) != len(bm) {
		return false
	}
	for i := range am {
		if am[i] != bm[i] {
			return false
		}
	}
	return true
}
