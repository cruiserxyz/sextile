package astrolog

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// A body Astrolog cannot place at the chart's moment (Chiron's ephemeris
// starts in AD 675; the asteroids' file for 600 BC is not in the data folder)
// is left out of the cast and named, and the rest of the chart is cast:
// whether the lens lists its bodies or toggles Astrolog's default set.
// Without it Astrolog prints the body at 0 Aries and finds aspects to it
// (NEXT.md BUG 11), and Sextile refused the whole chart.
func TestABodyAstrologCannotPlaceIsLeftOut(t *testing.T) {
	needAstrolog(t)
	raw, err := os.ReadFile("testdata/moments/BC-0050.as")
	if err != nil {
		t.Fatal(err)
	}
	cf, err := ResolveChartBytes(context.Background(), raw, "BC-0050.as")
	if err != nil {
		t.Fatal(err)
	}
	reg, err := ObjectRegistry(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for name, pol := range map[string]recipe.ObjectPolicy{
		"a listed set":        {Baseline: recipe.Explicit, Selection: []int{1, 2, 11, 12}},
		"the default toggled": {Baseline: recipe.AstrologDefault, Add: recipe.NewObjectSet(11, 12)},
	} {
		t.Run(name, func(t *testing.T) {
			spec, err := recipe.NewChartSpec(recipe.FileChart{Record: cf.Record, JD: cf.JD, Lon: cf.Lon, Lat: cf.Lat}, recipe.Placidus, recipe.Tropical, pol)
			if err != nil {
				t.Fatal(err)
			}
			spec.Registry = reg
			spec.AspectRegistry = testAspects(t)
			out, err := ComputeChart(context.Background(), spec)
			if err != nil {
				t.Fatalf("the chart was not cast: %v", err)
			}
			if !slices.Contains(out.Unavailable, "Chiron") || !slices.Contains(out.Unavailable, "Ceres") {
				t.Errorf("left out %v, want Chiron and Ceres named", out.Unavailable)
			}
			for _, p := range out.Positions {
				if p.Object.Number == 11 || p.Object.Number == 12 {
					t.Errorf("%s was cast at %v", p.Object.Name, p.Longitude)
				}
			}
			if len(out.Positions) == 0 {
				t.Error("no body was cast")
			}
			for _, a := range out.Aspects {
				if n := []int{a.AObject.Number, a.BObject.Number}; slices.Contains(n, 11) || slices.Contains(n, 12) {
					t.Errorf("an aspect to a body left out: %+v", a)
				}
			}
		})
	}
}
