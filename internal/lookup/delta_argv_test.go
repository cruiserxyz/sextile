package lookup

import (
	"reflect"
	"testing"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
)

// A delta policy in Astrolog's object numbers compiles to exactly the argv the
// typed-body policy compiled to before the numbers moved (the delta-policy slice
// of docs/05-workbench/release-0.1.3/objects.md): every `-R` flip, the node
// mode, and the wheel's tail are byte-identical. The literals were taken from
// the compiler's output for the same four lenses on the tree before the change,
// and the first one is the argv astrolog's TestCompile_PositionsArgv pins.
func TestDeltaPolicyOnNumbersEmitsTheSameArgv(t *testing.T) {
	place := location.PlaceInfo{Name: "Munich", Lat: 48.1333, Lon: 11.5667, TZ: "Europe/Berlin"}
	// The first 24 tokens: the moment, the place, the houses, the zodiac, and
	// the fixed restrictions every compile starts with.
	head := []string{"-qj", "2446232.0000000005", "-zl", "11:34E", "48:08N", "-zi", "", "", "-c", "14", "_s", "_Ys", "_sr0", ":sz", "-qag", "10", "15", "1582", "12:00am", "0", "0:00E", "0:00N", "=Yt", "_Yd"}
	tail := func(rest ...string) []string { return append(append([]string{}, head...), rest...) }
	// withRemove is a lens with the objects removed from it.
	withRemove := func(l Lens, nums ...int) Lens {
		l.Objects.Remove = recipe.NewObjectSet(nums...)
		return l
	}
	// plainLens is the default lens: Whole Sign, tropical, the typed baseline.
	plainLens := func(add, remove recipe.ObjectSet) Lens {
		return Lens{Name: "plain", Houses: recipe.WholeSign, Zodiac: recipe.Tropical,
			Objects: recipe.ObjectPolicy{Baseline: recipe.AstrologDefault, Add: add, Remove: remove}}
	}
	cases := []struct {
		name      string
		lens      Lens
		positions []string
		wheel     []string
	}{
		{
			name:      "the Hellenistic lens",
			lens:      Hellenistic,
			positions: tail("-Yn", "-R", "11", "-R", "17", "-R", "18", "-R", "25", "-R", "28", "=v0", "-Yw", "0"),
			wheel:     tail("-Yn", "-R", "11", "-R", "17", "-R", "18", "-R", "25", "-R", "28", "-XV", "-Xw", "1200", "-Xi", "-XA", "-Xx", "-Xx0", "-Xr", "-Xu", "-Xs", "400", "-XS", "400", "-Xt", "-XL"),
		},
		{
			name:      "the Hellenistic lens with the Moon removed",
			lens:      withRemove(Hellenistic, 2),
			positions: tail("-Yn", "-R", "2", "-R", "11", "-R", "17", "-R", "18", "-R", "25", "-R", "28", "=v0", "-Yw", "0"),
			wheel:     tail("-Yn", "-R", "2", "-R", "11", "-R", "17", "-R", "18", "-R", "25", "-R", "28", "-XV", "-Xw", "1200", "-Xi", "-XA", "-Xx", "-Xx0", "-Xr", "-Xu", "-Xs", "400", "-XS", "400", "-Xt", "-XL"),
		},
		{
			name:      "the default lens with the Moon removed",
			lens:      plainLens(nil, recipe.NewObjectSet(2)),
			positions: tail("-R", "2", "=v0", "-Yw", "0"),
			wheel:     tail("-R", "2", "-XV", "-Xw", "1200", "-Xi", "-XA", "-Xx", "-Xx0", "-Xr", "-Xu", "-Xs", "400", "-XS", "400", "-Xt", "-XL"),
		},
		{
			name:      "the default lens with Chiron added and Pluto and the Moon removed",
			lens:      plainLens(recipe.NewObjectSet(11), recipe.NewObjectSet(2, 10)),
			positions: tail("-R", "2", "-R", "10", "-R", "11", "=v0", "-Yw", "0"),
			wheel:     tail("-R", "2", "-R", "10", "-R", "11", "-XV", "-Xw", "1200", "-Xi", "-XA", "-Xx", "-Xx0", "-Xr", "-Xu", "-Xs", "400", "-XS", "400", "-Xt", "-XL"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := tc.lens.specAt(2446232.0, place)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range []struct {
				label string
				proj  recipe.Projection
				want  []string
			}{
				{"positions", recipe.Positions{}, tc.positions},
				{"wheel", recipe.WheelImage{Render: recipe.DefaultWheelRender()}, tc.wheel},
			} {
				plan, err := astrolog.Compile(spec, p.proj)
				if err != nil {
					t.Fatal(err)
				}
				if got := plan.Invocations[0].Argv; !reflect.DeepEqual(got, p.want) {
					t.Errorf("%s argv\n got: %q\nwant: %q", p.label, got, p.want)
				}
			}
		})
	}
}
