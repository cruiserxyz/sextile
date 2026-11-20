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

// onlyObjects is the Only set of the objects, by Astrolog's numbers.
func onlyObjects(objects ...aspect.Object) recipe.ObjectSet {
	return objectSet(objects...)
}

func hellenisticPolicy() recipe.ObjectPolicy {
	return recipe.ObjectPolicy{
		Baseline: recipe.AstrologDefault,
		Add:      recipe.NewObjectSet(11, 17, 18, 25, 28), // Chiron, South Node, Lilith, Nadir, Descendant
		Nodes:    recipe.NodesTrue,
	}
}

// objectBandOf is the object band of a positions argv: from `=Yn` or `_Yn` to the
// `=v0` that follows it.
func objectBandOf(t *testing.T, policy recipe.ObjectPolicy) []string {
	t.Helper()
	got := argv(mustCompile(t, onlySpec(t, policy), recipe.Positions{}))
	end := slices.Index(got, "=v0")
	start := slices.IndexFunc(got, func(s string) bool { return s == "=Yn" || s == "_Yn" })
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
		p.Only = onlyObjects(objSun, objMoon)
		want := []string{"=Yn", "_Ynn", "-R0", "1", "2"}
		if got := objectBandOf(t, p); !reflect.DeepEqual(got, want) {
			t.Errorf("object band = %v, want %v", got, want)
		}
	})

	t.Run("the numbers are Astrolog's, in ascending order", func(t *testing.T) {
		p := hellenisticPolicy()
		// Chiron 11, Midheaven 31, Venus 4, Pluto 10, set in no order.
		p.Only = onlyObjects(objMidheaven, objChiron, objPluto, objVenus)
		want := []string{"=Yn", "_Ynn", "-R0", "4", "10", "11", "31"}
		if got := objectBandOf(t, p); !reflect.DeepEqual(got, want) {
			t.Errorf("object band = %v, want %v", got, want)
		}
	})

	t.Run("an explicit selection is narrowed in its own numbers", func(t *testing.T) {
		p := recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 4, 10}}
		p.Only = onlyObjects(objMoon, objVenus, objMercury) // Mercury is not in the selection
		want := []string{"_Yn", "_Ynn", "-R0", "2", "4"}
		if got := objectBandOf(t, p); !reflect.DeepEqual(got, want) {
			t.Errorf("object band = %v, want %v", got, want)
		}
	})

	t.Run("Only empty compiles byte-identical to today's argv", func(t *testing.T) {
		// The argv TestCompile_PositionsArgv pins for this policy, spelled out
		// again here so this test is its own witness.
		want := []string{
			"-qj", "2446232.0000000005", "-zl", "11:34E", "48:08N", "-zi", "", "", "-c", "14", "_s", "_Ys", "_sr0", ":sz", "-qag", "10", "15", "1582", "12:00am", "0", "0:00E", "0:00N", "=Yt", "_Yd",
			"=Yn", "_Ynn", "-R", "11", "-R", "17", "-R", "18", "-R", "25", "-R", "28", "=v0", "-Yw", "0",
		}
		for name, only := range map[string]recipe.ObjectSet{"nil": nil, "empty": recipe.NewObjectSet()} {
			p := hellenisticPolicy()
			p.Only = only
			if got := argv(mustCompile(t, onlySpec(t, p), recipe.Positions{})); !reflect.DeepEqual(got, want) {
				t.Errorf("Only %s: argv\n got: %v\nwant: %v", name, got, want)
			}
		}
	})

	t.Run("a body the policy removed stays removed", func(t *testing.T) {
		p := hellenisticPolicy()
		p.Remove = recipe.NewObjectSet(2) // the Moon
		p.Only = onlyObjects(objSun, objMoon)
		want := []string{"=Yn", "_Ynn", "-R0", "1"}
		if got := objectBandOf(t, p); !reflect.DeepEqual(got, want) {
			t.Errorf("object band = %v, want %v (Moon was removed, Only must not bring it back)", got, want)
		}
	})

	t.Run("an Only that leaves nothing is dropped, not a bare show-list and not a refusal", func(t *testing.T) {
		p := hellenisticPolicy()
		p.Remove = recipe.NewObjectSet(2) // the Moon
		unmarked := objectBandOf(t, p)
		p.Only = onlyObjects(objMoon)
		got := objectBandOf(t, p)
		if !reflect.DeepEqual(got, unmarked) || slices.Contains(got, "-R0") {
			t.Errorf("object band = %v, want the policy's own %v and no bare -R0", got, unmarked)
		}
	})

	t.Run("the band is the compiler's one encoder", func(t *testing.T) {
		// Every projection that carries the policy takes the same band: the
		// aspect list, the raw report and the wheel read it from emitObjects.
		p := hellenisticPolicy()
		p.Only = onlyObjects(objSun, objMoon)
		spec := onlySpec(t, p)
		for _, proj := range []recipe.Projection{recipe.Positions{}, recipe.Aspects{}, recipe.WheelImage{Render: recipe.DefaultWheelRender()}} {
			a := strings.Join(argv(mustCompile(t, spec, proj)), " ")
			if !strings.Contains(a, "=Yn _Ynn -R0 1 2") {
				t.Errorf("%s argv = %q, want it to carry the narrowed band", proj.Label(), a)
			}
		}
	})

	t.Run("the timeline's natal side follows Only, its transiting side does not", func(t *testing.T) {
		// F14: the marks narrow the slot's own natal side (an exact -R0 list);
		// the transiting side is the ticks', as without the marks.
		full := hellenisticPolicy()
		narrow := hellenisticPolicy()
		narrow.Only = onlyObjects(objSun, objMoon)
		a, err := timelineBand(TimelineTransit, full, nil, recipe.AspectPolicy{}, measuredSets, 0)
		if err != nil {
			t.Fatal(err)
		}
		b, err := timelineBand(TimelineTransit, narrow, nil, recipe.AspectPolicy{}, measuredSets, 0)
		if err != nil {
			t.Fatal(err)
		}
		fa, ta := bandSides(strings.Join(a.tokens, " "))
		fb, tb := bandSides(strings.Join(b.tokens, " "))
		if !slices.Equal(fb, []int{1, 2}) {
			t.Errorf("the narrowed natal side = %v, want [1 2]", fb)
		}
		if len(fa) <= 2 || !slices.Equal(ta, tb) {
			t.Errorf("the transiting side moved with Only: %v vs %v (unnarrowed natal %v)", ta, tb, fa)
		}
	})
}
