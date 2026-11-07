package lookup

import (
	"context"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
)

// numbersAfterR are the object numbers the call's -R and -R0 switches carry.
func numbersAfterR(command string) []int {
	var out []int
	f := strings.Fields(command)
	for i, tok := range f {
		if tok != "-R" && tok != "-R0" {
			continue
		}
		for _, next := range f[i+1:] {
			n, err := strconv.Atoi(next)
			if err != nil {
				break
			}
			out = append(out, n)
		}
	}
	return out
}

func influenceCommand(t *testing.T, request RawReportRequest, objects recipe.ObjectPolicy) string {
	t.Helper()
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH")
	}
	place := location.PlaceInfo{Name: "London", Country: "GB", Lat: 51.5074, Lon: -0.1278, TZ: "Europe/London"}
	l := Hellenistic
	l.Objects = objects
	// The command is returned whether or not the run succeeded.
	_, command, _ := ComputeRawReport(context.Background(), time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC), place, l, request)
	if command == "" {
		t.Fatal("no command was recorded")
	}
	return command
}

// The influence chart (-j) reads past its arrays for a fixed star (Astrolog
// NEXT.md BUG 7), so the chart report never carries one, whatever form the
// policy takes. The other reports keep them.
func TestInfluenceChartHasNoStars(t *testing.T) {
	const capella, acrux = 89, 96
	for _, n := range []int{capella, acrux} {
		if !Hellenistic.Registry.IsStar(n) {
			t.Fatalf("the test registry does not flag object %d as a star", n)
		}
	}
	forms := map[string]recipe.ObjectPolicy{
		"explicit":      {Baseline: recipe.Explicit, Selection: []int{0, 1, 2, capella, acrux}},
		"explicit-only": {Baseline: recipe.Explicit, Selection: []int{capella, acrux}},
		"delta-add":     {Baseline: recipe.AstrologDefault, Add: recipe.NewObjectSet(11, capella, acrux), TrueNodeMode: true},
		"only":          {Baseline: recipe.AstrologDefault, Only: recipe.NewObjectSet(1, capella), TrueNodeMode: true},
	}
	for name, policy := range forms {
		got := numbersAfterR(influenceCommand(t, RawReportRequest{Kind: RawReportChart}, policy))
		for _, n := range got {
			if Hellenistic.Registry.IsStar(n) {
				t.Errorf("%s: the chart report's call carries star %d: %v", name, n, got)
			}
		}
		if name == "explicit" && !slices.Contains(got, 1) {
			t.Errorf("explicit: the planets must stay: %v", got)
		}
		if name == "explicit-only" && len(got) == 0 {
			t.Errorf("explicit-only: a pick of stars alone falls back to the default set, not to nothing")
		}
	}
	// The other reports keep the stars.
	for _, request := range []RawReportRequest{{Kind: RawReportAspects}, {Kind: RawReportBody, Object: objMoon}} {
		got := numbersAfterR(influenceCommand(t, request, forms["explicit"]))
		if !slices.Contains(got, capella) || !slices.Contains(got, acrux) {
			t.Errorf("report kind %d dropped the stars: %v", request.Kind, got)
		}
	}
}

// The real Astrolog, every fixed star ticked beside the ten planets: the chart
// report comes back. Without the narrowing, stars 89, 91 and 93 end an
// Astrolog 8.00 run on a signal on this machine's builds.
func TestChartReportWithEveryStar(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH")
	}
	pick := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for _, o := range Hellenistic.Registry {
		if o.Star {
			pick = append(pick, o.Number)
		}
	}
	l := Hellenistic
	l.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: pick, TrueNodeMode: true}
	place := location.PlaceInfo{Name: "London", Country: "GB", Lat: 51.5074, Lon: -0.1278, TZ: "Europe/London"}
	raw, _, err := ComputeRawReport(context.Background(), time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC), place, l, RawReportRequest{Kind: RawReportChart})
	if err != nil {
		t.Fatalf("the chart report with every star ticked: %v", err)
	}
	if !strings.Contains(raw, "Sign:") {
		t.Errorf("the report has no sign table:\n%s", raw)
	}
	if got := l.InfluenceLeavesOut(); len(got) != len(pick)-10 {
		t.Errorf("InfluenceLeavesOut = %d stars, want %d", len(got), len(pick)-10)
	}
}
