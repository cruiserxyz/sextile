package astrolog

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// orderChart is one staged chart of the ring-order test: its moment, the argv
// that makes Astrolog print its own longitudes, and the Sun's zodiac window the
// fixture must land in (a sanity check on the fixture, not on Sextile).
type orderChart struct {
	name     string
	t        time.Time
	argv     []string
	lo, hi   float64
	sunLong  float64
	ascendLo float64
}

var (
	orderDot       = regexp.MustCompile(`<circle r="1" cx="([-0-9.]+)" cy="([-0-9.]+)"/>`)
	orderCenterCir = regexp.MustCompile(`<circle[^>]*>`)
	orderAttr      = func(tag, name string) (float64, bool) {
		m := regexp.MustCompile(`\b` + name + `="([-0-9.]+)"`).FindStringSubmatch(tag)
		if m == nil {
			return 0, false
		}
		v, err := strconv.ParseFloat(m[1], 64)
		return v, err == nil
	}
)

// TestMultiWheelFirstCastChartIsTheInnerRing pins what the specification
// rule 8 measured on Astrolog 8.00: the first-cast chart is the inner ring,
// for the multi-wheel (-r3) and the bi-wheel (-r0) alike. Three charts with
// distinct Suns, Sun only, rendered to SVG: the r="1" circles are the glyph and
// position dots; the dot at a chart's Sun angle (Astrolog's own -v longitude
// against the start of chart 1's Ascendant sign, house 1's whole-sign cusp, at 180 degrees, counter-clockwise) that lies
// farthest from the centre is that chart's ring, and the rings must grow with
// the chart's place in the call. Skipped without astrolog, which ./check
// refuses anyway.
func TestMultiWheelFirstCastChartIsTheInnerRing(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary ring-order test")
	}
	ctx := context.Background()
	charts := []orderChart{
		{name: "A", t: time.Date(1961, 1, 11, 12, 0, 0, 0, time.UTC), argv: []string{"-qa", "1", "11", "1961", "12:00", "0", "0:00W", "51:30N"}, lo: 270, hi: 300},
		{name: "B", t: time.Date(1962, 7, 11, 12, 0, 0, 0, time.UTC), argv: []string{"-qa", "7", "11", "1962", "12:00", "0", "0:00W", "51:30N"}, lo: 90, hi: 120},
		{name: "C", t: time.Date(1963, 10, 11, 12, 0, 0, 0, time.UTC), argv: []string{"-qa", "10", "11", "1963", "12:00", "0", "0:00W", "51:30N"}, lo: 180, hi: 210},
	}
	var asc float64
	for i := range charts {
		out, err := Run(ctx, testCall(charts[i].argv))
		if err != nil {
			t.Fatalf("chart %s: Run: %v", charts[i].name, err)
		}
		charts[i].sunLong = findPosition(t, out.Positions, aspect.Sun).Longitude
		if charts[i].sunLong < charts[i].lo || charts[i].sunLong >= charts[i].hi {
			t.Fatalf("fixture: chart %s Sun at %.3f, want [%v,%v)", charts[i].name, charts[i].sunLong, charts[i].lo, charts[i].hi)
		}
		if i == 0 {
			asc = findPosition(t, out.Positions, aspect.Ascendant).Longitude
			// Measured 2026-10-13: with whole-sign houses the wheel puts the
			// START of the Ascendant's sign, house 1's cusp, at 180 degrees
			// (Ascendant 14Tau43 drew 14.7 degrees past it), so the reference
			// is the sign start, not the degree.
			asc = 30 * math.Floor(asc/30)
		}
	}

	policy := recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1}}
	src := func(c orderChart) recipe.Source {
		return recipe.MomentAtPlace{JD: GregorianToJD(c.t), Lon: 0, Lat: 51.5}
	}
	tri, err := recipe.NewChartSpec(recipe.MultiWheel{Rings: []recipe.Source{src(charts[0]), src(charts[1]), src(charts[2])}}, recipe.WholeSign, recipe.Tropical, policy)
	if err != nil {
		t.Fatal(err)
	}
	bi, err := recipe.NewChartSpec(recipe.Comparison{Primary: src(charts[0]), Secondary: src(charts[1]), Relation: recipe.Synastry}, recipe.WholeSign, recipe.Tropical, policy)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name  string
		spec  recipe.ChartSpec
		rings int
		count int
		flag  string
	}{
		{"tri-wheel", tri, 3, 5, "-r3"},
		{"bi-wheel", bi, 2, 3, "-r0"},
	} {
		plan, err := Compile(tc.spec, recipe.WheelImage{Render: recipe.DefaultWheelRender()})
		if err != nil {
			t.Fatalf("%s: Compile: %v", tc.name, err)
		}
		if len(plan.Invocations) != 1 {
			t.Fatalf("%s: %d invocations, want 1", tc.name, len(plan.Invocations))
		}
		av := plan.Invocations[0].Argv
		if !hasToken(av, tc.flag) || !hasAdjacent(av, "-R0", "1") || !hasToken(av, "-XV") {
			t.Fatalf("%s: argv %v does not carry %s -R0 1 -XV", tc.name, av, tc.flag)
		}
		data, err := RenderWheelImage(ctx, tc.spec, recipe.DefaultWheelRender())
		if err != nil {
			t.Fatalf("%s: RenderWheelImage: %v", tc.name, err)
		}
		svg := string(data)
		cx, cy, ok := orderCentre(svg)
		if !ok {
			t.Fatalf("%s: no separator circle with a centre in the SVG", tc.name)
		}
		type dot struct{ ang, dist float64 }
		var dots []dot
		for _, m := range orderDot.FindAllStringSubmatch(svg, -1) {
			x, _ := strconv.ParseFloat(m[1], 64)
			y, _ := strconv.ParseFloat(m[2], 64)
			a := math.Atan2(cy-y, x-cx) * 180 / math.Pi
			a = math.Mod(a+360, 360)
			dots = append(dots, dot{a, math.Hypot(x-cx, y-cy)})
		}
		if len(dots) != tc.count {
			t.Fatalf("%s: %d r=\"1\" dots, want exactly %d (centre %.0f,%.0f)", tc.name, len(dots), tc.count, cx, cy)
		}
		want := make([]float64, tc.rings)
		for k := range want {
			want[k] = math.Mod(180+charts[k].sunLong-asc+720, 360)
		}
		far := make([]float64, tc.rings)
		near := make([]int, tc.rings)
		diag := fmt.Sprintf("%s: centre %.0f,%.0f, expected angles %.1f, dots", tc.name, cx, cy, want)
		for _, d := range dots {
			diag += fmt.Sprintf(" (%.1f deg, %.0f)", d.ang, d.dist)
		}
		for _, d := range dots {
			hits := 0
			for k := range want {
				if angDist(d.ang, want[k]) <= 1 {
					hits++
					near[k]++
					far[k] = math.Max(far[k], d.dist)
				}
			}
			if hits != 1 {
				t.Errorf("%s: a dot at %.1f deg is within 1 degree of %d chart angles, want exactly 1", diag, d.ang, hits)
			}
		}
		for k := range want {
			if near[k] == 0 {
				t.Fatalf("%s: no dot within 1 degree of chart %s's angle %.1f", diag, charts[k].name, want[k])
			}
		}
		for k := 1; k < tc.rings; k++ {
			if !(far[k-1] < far[k]) {
				t.Errorf("%s: chart %s's farthest dot %.0f is not outside chart %s's %.0f; the first-cast chart is not the inner ring", diag, charts[k].name, far[k], charts[k-1].name, far[k-1])
			}
		}
		t.Logf("%s: dots %d, expected angles %.1f, farthest per chart %.0f", tc.name, len(dots), want, far)
	}
}

func hasToken(av []string, tok string) bool {
	for _, a := range av {
		if a == tok {
			return true
		}
	}
	return false
}

func hasAdjacent(av []string, a, b string) bool {
	for i := 0; i+1 < len(av); i++ {
		if av[i] == a && av[i+1] == b {
			return true
		}
	}
	return false
}

// orderCentre is the cx, cy of the first circle inside the ring-separator group.
func orderCentre(svg string) (cx, cy float64, ok bool) {
	g := ringSeparatorGroup.FindStringSubmatch(svg)
	if g == nil {
		return 0, 0, false
	}
	tag := orderCenterCir.FindString(g[1])
	if tag == "" {
		return 0, 0, false
	}
	x, okx := orderAttr(tag, "cx")
	y, oky := orderAttr(tag, "cy")
	return x, y, okx && oky
}

// angDist is the shorter way round between two angles in degrees.
func angDist(a, b float64) float64 {
	d := math.Mod(math.Abs(a-b), 360)
	return math.Min(d, 360-d)
}
