package astrolog

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// Live (F13/F14): the real binary, the report's natal (JD 2451545.5 at 0E 52N,
// tmp/orders/out/timeline-objects/). Names stay short: Astrolog's ephemeris
// path limit is 255 characters.

func liveObjectsNatal(o recipe.ObjectPolicy) *TimelineNatal {
	return &TimelineNatal{When: time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC), Lon: 0, Lat: 52, Objects: o}
}

// liveObjectRows runs one -t search of a month under the object policy and
// returns the presented lines.
func liveObjectRows(t *testing.T, month time.Month, o recipe.ObjectPolicy) []string {
	t.Helper()
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH")
	}
	withClock(t, Clock{Hours: Hours24, Dates: MonthFirst})
	sets, err := ReadDefaultSets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	req := TimelineRequest{Year: 2026, Month: month, Zone: time.UTC, Natal: liveObjectsNatal(o), Objects: o, Defaults: sets, AspectRegistry: mustTestAspects()}
	rows, _, err := ComputeTimeline(context.Background(), req, []TimelineSource{TimelineTransit}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, r := range rows {
		lines = append(lines, r.Line)
		if r.Transit && r.Body == "Moon" {
			t.Errorf("a transiting-Moon row is listed, the binary keeps it off the transit side: %q", r.Line)
		}
	}
	return lines
}

func anyContains(lines []string, parts ...string) bool {
	for _, l := range lines {
		ok := true
		for _, p := range parts {
			ok = ok && strings.Contains(l, p)
		}
		if ok {
			return true
		}
	}
	return false
}

var liveDefault = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 16, 22, 31}

func TestTimelineListsATickedCusp(t *testing.T) {
	o := explicit(append(slices.Clone(liveDefault), 12, 32)...)
	lines := liveObjectRows(t, time.October, o)
	if !anyContains(lines, "trans", "Mars", "Con", "11th Cusp") {
		t.Errorf("October with 32 ticked: no `trans Mars Con natal 11th Cusp` row among %d rows", len(lines))
	}
}

func TestTimelineListsTransitingCeres(t *testing.T) {
	o := explicit(append(slices.Clone(liveDefault), 12)...)
	lines := liveObjectRows(t, time.January, o)
	if !anyContains(lines, "trans", "Ceres", "natal") {
		t.Errorf("January with Ceres ticked: no `trans Ceres` row among %d rows", len(lines))
	}
}

func TestTimelineMarkNarrowsLive(t *testing.T) {
	o := withMarks(explicit(liveDefault...), 1)
	lines := liveObjectRows(t, time.October, o)
	if len(lines) == 0 {
		t.Fatal("the Sun marked: no rows at all")
	}
	for _, l := range lines {
		// A -t row reads `trans <body> (sign) <asp> natal (sign) <object>`: the
		// natal object is what follows "natal (xxx) ".
		i := strings.Index(l, " natal ")
		if i < 0 {
			t.Errorf("a -t row without a natal side: %q", l)
			continue
		}
		rest := strings.SplitN(l[i+len(" natal "):], ") ", 2)
		if len(rest) != 2 || !strings.HasPrefix(strings.TrimSpace(rest[1]), "Sun") {
			t.Errorf("a row names a natal object other than the Sun: %q", l)
		}
	}
}
