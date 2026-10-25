package astrolog

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// ObjectPolicy.Only is how "selected bodies show only themselves" reaches
// Astrolog: the one encoder (emitObjects) intersects the resolved body list with
// it and emits the exact `-R0` show-list. These tests read the argv the
// compiler hands Astrolog, so a policy that carried Only in the model but never
// reached the switches fails here.

// onlySpec is hellenisticSpec with Only (and optionally a Remove) on its policy.
func onlySpec(t *testing.T, policy recipe.ObjectPolicy) recipe.ChartSpec {
	t.Helper()
	spec, err := recipe.NewChartSpec(
		recipe.MomentAtPlace{JD: 2446232.0, Lon: 11.5667, Lat: 48.1333},
		recipe.WholeSign, recipe.Tropical, policy,
	)
	if err != nil {
		t.Fatalf("spec: %v", err)
	}
	return spec
}

func hellenisticPolicy() recipe.ObjectPolicy {
	return recipe.ObjectPolicy{
		Baseline:     recipe.AstrologDefault,
		Add:          recipe.NewBodySet(aspect.Chiron, aspect.SouthNode, aspect.Lilith, aspect.ImumCoeli, aspect.Descendant),
		TrueNodeMode: true,
	}
}

// bandOf is the object band of a positions argv: from `-Yn` or `-R…` to the
// `=v0` that follows it.
func objectBandOf(t *testing.T, policy recipe.ObjectPolicy) []string {
	t.Helper()
	got := argv(mustCompile(t, onlySpec(t, policy), recipe.Positions{}))
	end := slices.Index(got, "=v0")
	start := slices.Index(got, "-Yn")
	if start < 0 {
		start = slices.IndexFunc(got, func(s string) bool { return strings.HasPrefix(s, "-R") })
	}
	if start < 0 || end < start {
		t.Fatalf("no object band in %v", got)
	}
	return got[start:end]
}

// B1: a baseline of many bodies narrowed to {Sun, Moon} compiles to the exact
// show-list of those two, in Astrolog's numbers (Sun 1, Moon 2).
func TestOnlyIntersectsTheBodyList(t *testing.T) {
	t.Run("delta baseline narrows to the two", func(t *testing.T) {
		p := hellenisticPolicy()
		p.Only = recipe.NewBodySet(aspect.Sun, aspect.Moon)
		want := []string{"-Yn", "-R0", "1", "2"}
		if got := objectBandOf(t, p); !reflect.DeepEqual(got, want) {
			t.Errorf("object band = %v, want %v", got, want)
		}
	})

	t.Run("the numbers are Astrolog's, in ascending order", func(t *testing.T) {
		p := hellenisticPolicy()
		// Chiron 11, Midheaven 31, Venus 4, Pluto 10, set in no order.
		p.Only = recipe.NewBodySet(aspect.Midheaven, aspect.Chiron, aspect.Pluto, aspect.Venus)
		want := []string{"-Yn", "-R0", "4", "10", "11", "31"}
		if got := objectBandOf(t, p); !reflect.DeepEqual(got, want) {
			t.Errorf("object band = %v, want %v", got, want)
		}
	})

	t.Run("an explicit selection is narrowed in its own numbers", func(t *testing.T) {
		p := recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 4, 10}}
		p.Only = recipe.NewBodySet(aspect.Moon, aspect.Venus, aspect.Mercury) // Mercury is not in the selection
		want := []string{"-R0", "2", "4"}
		if got := objectBandOf(t, p); !reflect.DeepEqual(got, want) {
			t.Errorf("object band = %v, want %v", got, want)
		}
	})

	t.Run("Only empty compiles byte-identical to today's argv", func(t *testing.T) {
		// The argv TestCompile_PositionsArgv pins for this policy, spelled out
		// again here so this test is its own witness.
		want := []string{
			"-qj", "2446232.0000000005", "-zl", "11:34E", "48:08N", "-zi", "", "", "-c", "14", "_s", "_Ys", "_sr0", ":sz", "-qag", "10", "15", "1582", "12:00am", "0", "0:00E", "0:00N", "=Yt", "_Yd",
			"-Yn", "-R", "11", "-R", "17", "-R", "18", "-R", "25", "-R", "28", "=v0", "-Yw", "0",
		}
		for name, only := range map[string]recipe.BodySet{"nil": nil, "empty": recipe.NewBodySet()} {
			p := hellenisticPolicy()
			p.Only = only
			if got := argv(mustCompile(t, onlySpec(t, p), recipe.Positions{})); !reflect.DeepEqual(got, want) {
				t.Errorf("Only %s: argv\n got: %v\nwant: %v", name, got, want)
			}
		}
	})

	t.Run("a body the policy removed stays removed", func(t *testing.T) {
		p := hellenisticPolicy()
		p.Remove = recipe.NewBodySet(aspect.Moon)
		p.Only = recipe.NewBodySet(aspect.Sun, aspect.Moon)
		want := []string{"-Yn", "-R0", "1"}
		if got := objectBandOf(t, p); !reflect.DeepEqual(got, want) {
			t.Errorf("object band = %v, want %v (Moon was removed, Only must not bring it back)", got, want)
		}
	})

	t.Run("an Only that leaves nothing is the bare show-list, not a refusal", func(t *testing.T) {
		p := hellenisticPolicy()
		p.Remove = recipe.NewBodySet(aspect.Moon)
		p.Only = recipe.NewBodySet(aspect.Moon)
		want := []string{"-Yn", "-R0"}
		if got := objectBandOf(t, p); !reflect.DeepEqual(got, want) {
			t.Errorf("object band = %v, want %v", got, want)
		}
	})

	t.Run("the band is the compiler's one encoder", func(t *testing.T) {
		// Every projection that carries the policy takes the same band: the
		// aspect list, the raw report and the wheel read it from emitObjects.
		p := hellenisticPolicy()
		p.Only = recipe.NewBodySet(aspect.Sun, aspect.Moon)
		spec := onlySpec(t, p)
		for _, proj := range []recipe.Projection{recipe.Positions{}, recipe.Aspects{}, recipe.WheelImage{Render: recipe.DefaultWheelRender()}} {
			a := strings.Join(argv(mustCompile(t, spec, proj)), " ")
			if !strings.Contains(a, "-Yn -R0 1 2") {
				t.Errorf("%s argv = %q, want it to carry the narrowed band", proj.Label(), a)
			}
		}
	})

	t.Run("the timeline keeps the full lens", func(t *testing.T) {
		// Only is the chart surfaces' narrowing; the Details timeline follows
		// the lens the operator committed and must not read it.
		full := hellenisticPolicy()
		narrow := hellenisticPolicy()
		narrow.Only = recipe.NewBodySet(aspect.Sun, aspect.Moon)
		a, err := timelineBand(TimelineTransit, full, recipe.AspectPolicy{}, measuredSets)
		if err != nil {
			t.Fatal(err)
		}
		b, err := timelineBand(TimelineTransit, narrow, recipe.AspectPolicy{}, measuredSets)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(a, b) {
			t.Errorf("the timeline band moved with Only:\n full:   %v\n narrow: %v", a, b)
		}
	})
}
