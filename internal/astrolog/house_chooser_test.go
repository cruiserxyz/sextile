package astrolog

// the specification C2, C3, C4 (argv half), C7 (live half). Live tests use
// the installed Astrolog; they skip when it is not on PATH.

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// chartWithoutC is Equinox-2026-03's record minus its `-c` line.
const chartWithoutC = "@AI770  ; Astrolog chart info.\n-qb Mar 20 2026 3:46pm ST 1E 11:34E 48:08N\n-zi \"\" \"Munich, Germany\"\n_s\n"

func writeChart(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "c.as")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestHouseSystemTableMatchesAstrolog(t *testing.T) {
	needAstrolog(t)
	p := writeChart(t, chartWithoutC)
	for n := 0; n < recipe.HouseSystemCount; n++ {
		out, _, err := spawn(context.Background(), testCall([]string{"-c", strconv.Itoa(n), "-i", p, "-R0", "-w"}))
		if err != nil {
			t.Fatalf("-c %d: %v", n, err)
		}
		got := wheelHousesHeader(out)
		want := recipe.HouseSystem(n).StoredName()
		if got != want {
			t.Errorf("-c %d printed %q, table says %q", n, got, want)
		}
		if back, ok := recipe.ParseHouseSystem(got); !ok || int(back) != n {
			t.Errorf("-c %d: ParseHouseSystem(%q) = %d,%v", n, got, back, ok)
		}
	}
	out, _, err := spawn(context.Background(), testCall([]string{"-c", "22", "-i", p, "-R0", "-w"}))
	if err != nil {
		t.Fatal(err)
	}
	if got := wheelHousesHeader(out); got != "Null" {
		t.Errorf("-c 22 printed %q, want Null", got)
	}
}

func TestChartFileHouseSystemIsAstrologs(t *testing.T) {
	needAstrolog(t)
	cases := []struct {
		name, path string
		want       recipe.HouseSystem
		has        bool
	}{
		{"Equinox-2026-03 names Placidus", fixture("Equinox-2026-03.as"), recipe.Placidus, true},
		{"no -c line names none", writeChart(t, chartWithoutC), 0, false},
		{"-c Koch", writeChart(t, strings.Replace(chartWithoutC, "_s\n", "-c Koch\n_s\n", 1)), recipe.Koch, true},
	}
	for _, c := range cases {
		cf, err := ResolveChartFile(context.Background(), c.path)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if cf.HasHouses != c.has || (c.has && cf.Houses != c.want) {
			t.Errorf("%s: Houses=%v HasHouses=%v, want %v %v", c.name, cf.Houses, cf.HasHouses, c.want, c.has)
		}
	}
}

func TestChartFileWithTwoHouseSwitchesUsesAstrologsHeader(t *testing.T) {
	needAstrolog(t)
	body := strings.Replace(chartWithoutC, "_s\n", "-c Koch\n-c Equal\n_s\n", 1)
	cf, err := ResolveChartFile(context.Background(), writeChart(t, body))
	if err != nil {
		t.Fatal(err)
	}
	// Astrolog applies the later switch; Go does not parse the -c lines.
	out, _, err := spawn(context.Background(), testCall([]string{"-c", "22", "-i", writeChart(t, body), "-R0", "-w"}))
	if err != nil {
		t.Fatal(err)
	}
	want, ok := recipe.ParseHouseSystem(wheelHousesHeader(out))
	if !ok || !cf.HasHouses || cf.Houses != want {
		t.Errorf("Houses=%v has=%v, Astrolog's header says %v (%v)", cf.Houses, cf.HasHouses, want, ok)
	}
}

func TestHouseSystemReachesArgvAndStagedFiles(t *testing.T) {
	m := recipe.MomentAtPlace{JD: 2450000.5, Lon: 10, Lat: 52}
	for n := 0; n < recipe.HouseSystemCount; n++ {
		hs := recipe.HouseSystem(n)
		code := strconv.Itoa(n)
		for _, src := range []recipe.Source{m, recipe.Comparison{Primary: m, Secondary: m}} {
			spec := fixtureSpec(t, src, hs, recipe.Tropical)
			var proj recipe.Projection = recipe.Positions{}
			if _, ok := src.(recipe.Comparison); ok {
				proj = recipe.Aspects{}
			}
			plan, err := Compile(spec, proj)
			if err != nil {
				t.Fatalf("%d: %v", n, err)
			}
			inv := plan.Invocations[0]
			if !hasPair(inv.Argv, "-c", code) {
				t.Errorf("%T hs=%d: argv %v lacks -c %s", src, n, inv.Argv, code)
			}
			for _, tmp := range inv.TempInputs {
				if !strings.Contains(tmp.Content, "\n-c "+code+"\n") {
					t.Errorf("hs=%d staged file lacks -c %s:\n%s", n, code, tmp.Content)
				}
			}
		}
		spec := fixtureSpec(t, recipe.MultiWheel{Rings: []recipe.Source{m, m, m}}, hs, recipe.Tropical)
		plan, err := Compile(spec, recipe.WheelImage{Render: recipe.DefaultWheelRender()})
		if err != nil {
			t.Fatal(err)
		}
		for _, tmp := range plan.Invocations[0].TempInputs {
			if !strings.Contains(tmp.Content, "\n-c "+code+"\n") {
				t.Errorf("multi-wheel hs=%d staged file lacks -c %s", n, code)
			}
		}
	}
}

func hasPair(argv []string, a, b string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == a && argv[i+1] == b {
			return true
		}
	}
	return false
}

func TestPolarSubstitutionIsReadFromAstrolog(t *testing.T) {
	needAstrolog(t)
	cases := []struct {
		lat      float64
		asked    recipe.HouseSystem
		wantUsed recipe.HouseSystem
	}{
		{69.6, recipe.Placidus, recipe.Porphyry},
		{69.6, recipe.Koch, recipe.Porphyry},
		{69.6, recipe.WholeSign, recipe.WholeSign},
		{80, recipe.Placidus, recipe.Porphyry},
		{80, recipe.Koch, recipe.Porphyry},
		{80, recipe.WholeSign, recipe.WholeSign},
		{52, recipe.Placidus, recipe.Placidus},
	}
	for _, c := range cases {
		spec := fixtureSpec(t, recipe.MomentAtPlace{JD: 2451545.25, Lon: 18.95, Lat: c.lat}, c.asked, recipe.Tropical)
		out, err := ComputeChartPositionsOnly(context.Background(), spec)
		if err != nil {
			t.Fatalf("%v@%v: %v", c.asked, c.lat, err)
		}
		if out.HousesUsed != c.wantUsed.StoredName() {
			t.Errorf("%v@%v: HousesUsed=%q, want %q", c.asked, c.lat, out.HousesUsed, c.wantUsed.StoredName())
		}
		// The Ascendant (cusp 1) of a substituted system is the substitute's.
		subst := fixtureSpec(t, spec.Source, c.wantUsed, recipe.Tropical)
		ref, err := ComputeChartPositionsOnly(context.Background(), subst)
		if err != nil {
			t.Fatal(err)
		}
		if out.Cusps[0] != ref.Cusps[0] {
			t.Errorf("%v@%v: Ascendant %v, want the %v Ascendant %v", c.asked, c.lat, out.Cusps[0], c.wantUsed, ref.Cusps[0])
		}
	}
}
