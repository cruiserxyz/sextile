package lookup

import (
	"context"
	"os/exec"
	"reflect"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/payload"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/testenv/astrodir"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// ProbeBody selects exactly the one object whatever the lens selects, keeps the
// node mode, and turns the result into a verdict (the specification
// C1, C7; build notes N3, N10).
func TestProbeBodyVerdicts(t *testing.T) {
	place := location.PlaceInfo{Name: "Testford", Lat: 52, Lon: 13, TZ: "Europe/Berlin"}
	lens := Hellenistic
	lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 3, 16}, Nodes: recipe.NodesTrue}
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
		p, spec, err := run(payload.RawOutput{Positions: []aspect.Position{{Object: registryObject(t, 43)}}})
		if err != nil || !p.Drawable {
			t.Fatalf("verdict %+v err %v, want drawable", p, err)
		}
		if got := spec.Objects.Selected(); !reflect.DeepEqual(got, []int{43}) {
			t.Errorf("selection = %v, want only [43]", got)
		}
		if spec.Objects.Nodes != recipe.NodesTrue {
			t.Error("the node mode was dropped")
		}
	})
	t.Run("no position and a named file: not drawable", func(t *testing.T) {
		p, _, err := run(payload.RawOutput{MissingEphemeris: []string{"a.se1"}})
		if err != nil || p.Drawable || !reflect.DeepEqual(p.MissingFiles, []string{"a.se1"}) {
			t.Fatalf("verdict %+v err %v, want undrawable naming a.se1", p, err)
		}
	})
	t.Run("no position and no file: not drawable, no file", func(t *testing.T) {
		// Absent means asked for and not returned at a real position, whatever
		// Astrolog says or does not say about it (a stderr line that names no
		// file, a silently missing row): never an error, never a false "drawable".
		p, _, err := run(payload.RawOutput{Unavailable: []string{"Hygiea"}})
		if err != nil || p.Drawable || len(p.MissingFiles) != 0 {
			t.Fatalf("verdict %+v err %v, want undrawable with no file", p, err)
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

// Through the real binary: a folder without the Uranian elements makes Astrolog
// write a generic stderr line for Vulcan; the chooser's probe reads it as an
// undrawable body with no file, not as a refusal and not as an error.
func TestProbeBodyReadsAGenericStderrLineAsUndrawable(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH")
	}
	astrodir.ReducedWithoutUranians(t)
	place := location.PlaceInfo{Name: "Testford", Lat: 52, Lon: 13, TZ: "Europe/Berlin"}
	p, err := ProbeBody(context.Background(), &Probers{}, time.Date(2001, 2, 3, 4, 5, 0, 0, time.UTC), place, Hellenistic, 34)
	if err != nil || p.Drawable || len(p.MissingFiles) != 0 {
		t.Fatalf("verdict %+v err %v, want undrawable with no file", p, err)
	}
}

// registryObject is the object Astrolog numbers n in the fixture list this
// package's lens carries.
func registryObject(t *testing.T, n int) aspect.Object {
	t.Helper()
	o, ok := Hellenistic.Registry.ByNumber(n)
	if !ok {
		t.Fatalf("object %d is not in the fixture registry", n)
	}
	return o
}
