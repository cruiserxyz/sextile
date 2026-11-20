package lookup

import (
	"reflect"
	"testing"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
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
			positions: tail("_Yn", "_Ynn", "-R", "11", "-R", "17", "-R", "18", "-R", "25", "-R", "28", "=v0", "-Yw", "0"),
			wheel:     tail("_Yn", "_Ynn", "-R", "11", "-R", "17", "-R", "18", "-R", "25", "-R", "28", "-XV", "_XJ", "=Xm", "=XQ", "=XQ0", "=Xv0", ":Xv", "1", ":YXG", "111221", ":Xs", "200", ":XS", "100", ":XI0", "25", "1", ":Xkv", "Aut", ":YXf", "#000000", ":YXg", "0", ":YXx", "0", "-Xw", "1200", "=Xi", "=XA", "=Xx", "=Xx0", "=Xr", "_Xu", "_Xt", "=XL", "-Xs", "400", "-XS", "400"),
		},
		{
			name:      "the Hellenistic lens with the Moon removed",
			lens:      withRemove(Hellenistic, 2),
			positions: tail("_Yn", "_Ynn", "-R", "2", "-R", "11", "-R", "17", "-R", "18", "-R", "25", "-R", "28", "=v0", "-Yw", "0"),
			wheel:     tail("_Yn", "_Ynn", "-R", "2", "-R", "11", "-R", "17", "-R", "18", "-R", "25", "-R", "28", "-XV", "_XJ", "=Xm", "=XQ", "=XQ0", "=Xv0", ":Xv", "1", ":YXG", "111221", ":Xs", "200", ":XS", "100", ":XI0", "25", "1", ":Xkv", "Aut", ":YXf", "#000000", ":YXg", "0", ":YXx", "0", "-Xw", "1200", "=Xi", "=XA", "=Xx", "=Xx0", "=Xr", "_Xu", "_Xt", "=XL", "-Xs", "400", "-XS", "400"),
		},
		{
			name:      "the default lens with the Moon removed",
			lens:      plainLens(nil, recipe.NewObjectSet(2)),
			positions: tail("_Yn", "_Ynn", "-R", "2", "=v0", "-Yw", "0"),
			wheel:     tail("_Yn", "_Ynn", "-R", "2", "-XV", "_XJ", "=Xm", "=XQ", "=XQ0", "=Xv0", ":Xv", "1", ":YXG", "111221", ":Xs", "200", ":XS", "100", ":XI0", "25", "1", ":Xkv", "Aut", ":YXf", "#000000", ":YXg", "0", ":YXx", "0", "-Xw", "1200", "=Xi", "=XA", "=Xx", "=Xx0", "=Xr", "_Xu", "_Xt", "=XL", "-Xs", "400", "-XS", "400"),
		},
		{
			name:      "the default lens with Chiron added and Pluto and the Moon removed",
			lens:      plainLens(recipe.NewObjectSet(11), recipe.NewObjectSet(2, 10)),
			positions: tail("_Yn", "_Ynn", "-R", "2", "-R", "10", "-R", "11", "=v0", "-Yw", "0"),
			wheel:     tail("_Yn", "_Ynn", "-R", "2", "-R", "10", "-R", "11", "-XV", "_XJ", "=Xm", "=XQ", "=XQ0", "=Xv0", ":Xv", "1", ":YXG", "111221", ":Xs", "200", ":XS", "100", ":XI0", "25", "1", ":Xkv", "Aut", ":YXf", "#000000", ":YXg", "0", ":YXx", "0", "-Xw", "1200", "=Xi", "=XA", "=Xx", "=Xx0", "=Xr", "_Xu", "_Xt", "=XL", "-Xs", "400", "-XS", "400"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := tc.lens.specAt(AtMoment(2446232.0, place, store.ReportLabel{}))
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
