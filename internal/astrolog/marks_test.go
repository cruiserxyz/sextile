package astrolog

import (
	"reflect"
	"slices"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// mustHighlightMembers is highlightMembers that fails the test on an error.
func mustHighlightMembers(t *testing.T, o recipe.ObjectPolicy, h recipe.HighlightSpec) []int {
	t.Helper()
	got, err := highlightMembers(o, h)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// A mark on an object with no glyph narrows the chart's own argv by its
// number, under both baselines: the show list carries 19 for Fortune.
func TestCompiledArgvCarriesAnUntypedMark(t *testing.T) {
	withAvailability(t, Availability{Default: []int{1, 2, 3, 19, 22}})

	t.Run("delta baseline", func(t *testing.T) {
		p := recipe.ObjectPolicy{Baseline: recipe.AstrologDefault, Only: recipe.NewObjectSet(19)}
		want := []string{"-R0", "19"}
		if got := objectBandOf(t, p); !reflect.DeepEqual(got, want) {
			t.Errorf("object band = %v, want %v (the marked Fortune alone)", got, want)
		}
		p.Only = recipe.NewObjectSet(19, 1, 3)
		want = []string{"-R0", "1", "3", "19"}
		if got := objectBandOf(t, p); !reflect.DeepEqual(got, want) {
			t.Errorf("object band = %v, want %v", got, want)
		}
	})

	t.Run("delta baseline, a mark the host does not draw is not drawn", func(t *testing.T) {
		p := recipe.ObjectPolicy{Baseline: recipe.AstrologDefault, Only: recipe.NewObjectSet(19, 60)}
		want := []string{"-R0", "19"}
		if got := objectBandOf(t, p); !reflect.DeepEqual(got, want) {
			t.Errorf("object band = %v, want %v", got, want)
		}
	})

	t.Run("explicit baseline", func(t *testing.T) {
		p := recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 19, 23}, Only: recipe.NewObjectSet(19, 23)}
		want := []string{"-R0", "19", "23"}
		if got := objectBandOf(t, p); !reflect.DeepEqual(got, want) {
			t.Errorf("object band = %v, want %v", got, want)
		}
	})
}

// The wheel highlight's selected tier carries the number too: Fortune is a
// member of the foreground show list.
func TestHighlightCarriesAnUntypedMark(t *testing.T) {
	spec, err := recipe.NewChartSpec(
		recipe.MomentAtPlace{JD: 2446232.0, Lon: 11.5667, Lat: 48.1333},
		recipe.WholeSign, recipe.Tropical,
		recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 19}},
	)
	if err != nil {
		t.Fatal(err)
	}
	render := recipe.DefaultWheelRender()
	render.Highlight = recipe.NewHighlight([]int{19}, aspect.Object{})
	plan, err := Compile(spec, recipe.WheelImage{Render: render})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Invocations) != 3 {
		t.Fatalf("got %d invocations, want the three-layer composite", len(plan.Invocations))
	}
	fg := plan.Invocations[1].Argv
	if n := len(fg); n < 2 || fg[n-2] != "-R0" || fg[n-1] != "19" {
		t.Errorf("foreground argv tail = %v, want -R0 19", fg)
	}
	// A mark the chart does not draw is left out, never emitted malformed.
	render.Highlight = recipe.NewHighlight([]int{19, 60}, objSun)
	members := mustHighlightMembers(t, spec.Objects, render.Highlight)
	if !slices.Equal(members, []int{1, 19}) {
		t.Errorf("highlightMembers = %v, want [1 19]", members)
	}
}

// Marks never add (operator, interview 2026-11-04: "Marks never add, they are
// kept"): a mark narrows what the policy draws and never draws an object the
// policy leaves off. Under both baselines, in the chart's own argv and in the
// wheel's highlight list, on each of two host default sets (the narrowing reads
// the host's own default set, there is no other baseline).
func TestMarkNeverDrawsAnUndrawnObject(t *testing.T) {
	for _, host := range []struct {
		name string
		set  []int
	}{
		{"host default of the thirteen", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 16, 22, 31}},
		{"host default with Fortune", []int{1, 2, 3, 4, 5, 10, 16, 19, 22, 31}},
	} {
		t.Run(host.name, func(t *testing.T) {
			withAvailability(t, Availability{Default: host.set})

			t.Run("delta baseline: the Moon is removed, the mark on the Moon adds nothing", func(t *testing.T) {
				p := recipe.ObjectPolicy{Baseline: recipe.AstrologDefault, Remove: recipe.NewObjectSet(2), Only: recipe.NewObjectSet(1, 2)}
				if band, want := objectBandOf(t, p), []string{"-R0", "1"}; !reflect.DeepEqual(band, want) {
					t.Errorf("chart band = %v, want %v (the Moon is not drawn, so its mark is kept and not drawn)", band, want)
				}
				p.Only = recipe.NewObjectSet(2)
				if band, want := objectBandOf(t, p), []string{"-R", "2"}; !reflect.DeepEqual(band, want) {
					t.Errorf("chart band with only the Moon marked = %v, want the baseline's %v (a mark that leaves nothing narrows nothing, never a bare -R0)", band, want)
				}
				// The wheel: the highlight's selected and cursor tiers are cut by the same drawn set.
				plan := mustCompile(t, onlySpec(t, p), recipe.WheelImage{Render: highlightRenderObjects([]int{2}, objMoon)})
				if len(plan.Invocations) != 1 {
					t.Errorf("a highlight on the removed Moon compiles to the plain wheel, got %d invocations", len(plan.Invocations))
				}
				removed := recipe.ObjectPolicy{Baseline: recipe.AstrologDefault, Remove: recipe.NewObjectSet(2)}
				if got := mustHighlightMembers(t, removed, recipe.NewHighlight([]int{2, 1}, objMoon)); !slices.Equal(got, []int{1}) {
					t.Errorf("highlight members = %v, want [1] (Moon removed by the lens)", got)
				}
			})

			t.Run("delta baseline: an object the lens does not add is not drawn by its mark", func(t *testing.T) {
				p := recipe.ObjectPolicy{Baseline: recipe.AstrologDefault, Only: recipe.NewObjectSet(1, 11, 60)}
				if band, want := objectBandOf(t, p), []string{"-R0", "1"}; !reflect.DeepEqual(band, want) {
					t.Errorf("chart band = %v, want %v (Chiron and Iapetus are not in the lens)", band, want)
				}
				if got := mustHighlightMembers(t, recipe.ObjectPolicy{Baseline: recipe.AstrologDefault}, recipe.NewHighlight([]int{11, 60, 1}, objChiron)); !slices.Equal(got, []int{1}) {
					t.Errorf("highlight members = %v, want [1]", got)
				}
			})

			t.Run("explicit baseline: Mars is not selected, the mark on Mars adds nothing", func(t *testing.T) {
				p := recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 19}, Only: recipe.NewObjectSet(5, 19)}
				if band, want := objectBandOf(t, p), []string{"-R0", "19"}; !reflect.DeepEqual(band, want) {
					t.Errorf("chart band = %v, want %v", band, want)
				}
				p.Only = recipe.NewObjectSet(5)
				if band, want := objectBandOf(t, p), []string{"-R0", "1", "2", "19"}; !reflect.DeepEqual(band, want) {
					t.Errorf("chart band with only Mars marked = %v, want the selection's %v (a mark that leaves nothing narrows nothing, never a bare -R0)", band, want)
				}
				plan := mustCompile(t, onlySpec(t, p), recipe.WheelImage{Render: highlightRenderObjects([]int{5}, objMars)})
				if len(plan.Invocations) != 1 {
					t.Errorf("a highlight on the unselected Mars compiles to the plain wheel, got %d invocations", len(plan.Invocations))
				}
				sel := recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 19}}
				if got := mustHighlightMembers(t, sel, recipe.NewHighlight([]int{5, 19}, objMars)); !slices.Equal(got, []int{19}) {
					t.Errorf("highlight members = %v, want [19]", got)
				}
			})
		})
	}
}

// The wheel's cursor tier is an object number: Fortune under
// the Bodies cursor is a member of the foreground show list, drawn when the
// chart draws it and absent when it does not.
func TestHighlightFocusedObjectCarriesItsNumber(t *testing.T) {
	fortune := aspect.Object{Number: 19, Name: "Fortune"}
	drawn := recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 19}}
	spec, err := recipe.NewChartSpec(
		recipe.MomentAtPlace{JD: 2446232.0, Lon: 11.5667, Lat: 48.1333},
		recipe.WholeSign, recipe.Tropical, drawn,
	)
	if err != nil {
		t.Fatal(err)
	}
	plan := mustCompile(t, spec, recipe.WheelImage{Render: highlightRenderObjects(nil, fortune)})
	if len(plan.Invocations) != 3 {
		t.Fatalf("a focused Fortune is a highlight member: %d invocations, want the three-layer composite", len(plan.Invocations))
	}
	// The chart's own band (-R0 1 2 19) comes first; the highlight's show list is the tail.
	fg := plan.Invocations[1].Argv
	if n := len(fg); n < 2 || fg[n-2] != "-R0" || fg[n-1] != "19" {
		t.Errorf("foreground argv tail = %v, want -R0 19", fg)
	}
	// The same focus on a chart that does not draw Fortune lights nothing.
	if got := mustHighlightMembers(t, recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2}}, recipe.NewHighlight(nil, fortune)); len(got) != 0 {
		t.Errorf("highlight members = %v, want none (Fortune is not drawn)", got)
	}
}
