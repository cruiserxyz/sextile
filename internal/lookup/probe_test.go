package lookup

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/payload"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// ProbeBody selects exactly the one object whatever the lens selects, keeps the
// node mode, and turns the result into a verdict (the specification
// C1, C7; build notes N3, N10).
func TestProbeBodyVerdicts(t *testing.T) {
	place := location.PlaceInfo{Name: "Testford", Lat: 52, Lon: 13, TZ: "Europe/Berlin"}
	lens := Hellenistic
	lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 3, 16}, TrueNodeMode: true}
	run := func(raw payload.RawOutput) (BodyProbe, recipe.ChartSpec, error) {
		var got recipe.ChartSpec
		probers := &Probers{PositionsOnly: func(_ context.Context, spec recipe.ChartSpec) (payload.RawOutput, error) {
			got = spec
			return raw, nil
		}}
		p, err := ProbeBody(context.Background(), probers, time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC), place, lens, 43)
		return p, got, err
	}
	t.Run("positions: drawable, one object, node mode kept", func(t *testing.T) {
		p, spec, err := run(payload.RawOutput{Positions: []aspect.Position{{Label: "Hygi"}}})
		if err != nil || !p.Drawable {
			t.Fatalf("verdict %+v err %v, want drawable", p, err)
		}
		if got := spec.Objects.Selected(); !reflect.DeepEqual(got, []int{43}) {
			t.Errorf("selection = %v, want only [43]", got)
		}
		if !spec.Objects.TrueNodeMode {
			t.Error("the node mode was dropped")
		}
	})
	t.Run("no position and a named file: not drawable", func(t *testing.T) {
		p, _, err := run(payload.RawOutput{MissingEphemeris: []string{"a.se1"}})
		if err != nil || p.Drawable || !reflect.DeepEqual(p.MissingFiles, []string{"a.se1"}) {
			t.Fatalf("verdict %+v err %v, want undrawable naming a.se1", p, err)
		}
	})
	t.Run("no position and no file: an error, not a verdict", func(t *testing.T) {
		if p, _, err := run(payload.RawOutput{Unavailable: []string{"Hygiea"}}); err == nil {
			t.Fatalf("verdict %+v with no error, want an error", p)
		}
	})
	t.Run("a zone that does not load is refused before any call", func(t *testing.T) {
		called := false
		probers := &Probers{PositionsOnly: func(context.Context, recipe.ChartSpec) (payload.RawOutput, error) {
			called = true
			return payload.RawOutput{}, nil
		}}
		bad := location.PlaceInfo{Name: "Nowhere", Lat: 1, Lon: 1, TZ: "Invalid/Zone"}
		if _, err := ProbeBody(context.Background(), probers, time.Now(), bad, lens, 43); err == nil || called {
			t.Fatalf("err = %v, called = %v; want an error and no call", err, called)
		}
	})
}
