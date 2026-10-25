package astrolog

// The chart fixtures are public sky events, not people: each file is named for
// an event a reader can check with Astrolog itself, and says in a comment the
// instant Astrolog gives for it. See testdata/README.md.

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// fixtureEvents is the allowlist of the chart fixtures: file, the one -zi name
// the file may carry ("" for none) and the ;sextile-id: line it keeps ("" for
// none). InvalidFile.as, the negative case, is the only file outside it.
var fixtureEvents = []struct{ file, name, id string }{
	{"Eclipse-2026-08-12.as", "", "68f95245"},
	{"Equinox-2026-03.as", "", "decae797"},
	{"Equinox-2026-09.as", "", ""},
	{"FullMoon-2026-02.as", "", ""},
	{"GreatConjunction-2020-12-21.as", "Great Conjunction", "2b7258ba"},
	{"Solstice-2025-12.as", "", ""},
}

// F1-1, F1-1b: only the named events are fixtures, none carries a person's
// name, one keeps an empty -zi (the name fallback in lookup reads the stem),
// and the ids stay with their files.
func TestFixtureChartsAreEvents(t *testing.T) {
	allowed := map[string]bool{"InvalidFile.as": true}
	for _, ev := range fixtureEvents {
		allowed[ev.file] = true
	}
	ents, err := os.ReadDir(filepath.Join("testdata", "charts"))
	if err != nil {
		t.Fatal(err)
	}
	present := map[string]bool{}
	for _, e := range ents {
		present[e.Name()] = true
		if !allowed[e.Name()] {
			t.Errorf("%s is a chart fixture outside the event allowlist", e.Name())
		}
	}
	emptyLabels, ids := 0, 0
	reID := regexp.MustCompile(`(?m)^;sextile-id: (\S+)$`)
	for _, ev := range fixtureEvents {
		if !present[ev.file] {
			t.Errorf("%s is on the allowlist but not in testdata/charts", ev.file)
			continue
		}
		b, err := os.ReadFile(fixture(ev.file))
		if err != nil {
			t.Fatal(err)
		}
		sw := scanChartSwitches(string(b))
		name, place := sw.labels()
		if name != ev.name {
			t.Errorf("%s: -zi name = %q, want %q", ev.file, name, ev.name)
		}
		if name == "" && place == "" {
			emptyLabels++
		}
		id := ""
		if m := reID.FindStringSubmatch(string(b)); m != nil {
			id = m[1]
			ids++
		}
		if id != ev.id {
			t.Errorf("%s: ;sextile-id = %q, want %q", ev.file, id, ev.id)
		}
	}
	if emptyLabels == 0 {
		t.Error("no fixture keeps an empty -zi name and place; the stem fallback in lookup has nothing to read")
	}
	if ids != 3 {
		t.Errorf("%d fixtures carry a ;sextile-id: line, want 3", ids)
	}
}

// F1-2: each event's one fact is recomputed by a live Astrolog and held
// tightly (the Sun moves 0.04 degrees an hour, the Moon 0.5), and the UTC
// instant the file's comment gives is the instant the file resolves to.
func TestFixtureEventsMatchAstrolog(t *testing.T) {
	needAstrolog(t)
	// 0 at the new moon, 180 at the full moon.
	elongation := func(p map[aspect.Body]float64) float64 {
		return math.Mod(p[aspect.Moon]-p[aspect.Sun]+360, 360)
	}
	apart := func(d, target float64) float64 { return math.Abs(math.Remainder(d-target, 360)) }
	tcs := []struct {
		file string
		fact string
		off  func(p map[aspect.Body]float64) float64 // degrees away from the event
		tol  float64
	}{
		{"Equinox-2026-03.as", "Sun at 0 Aries", func(p map[aspect.Body]float64) float64 { return apart(p[aspect.Sun], 0) }, 0.02},
		{"Equinox-2026-09.as", "Sun at 0 Libra", func(p map[aspect.Body]float64) float64 { return apart(p[aspect.Sun], 180) }, 0.02},
		{"Solstice-2025-12.as", "Sun at 0 Capricorn", func(p map[aspect.Body]float64) float64 { return apart(p[aspect.Sun], 270) }, 0.02},
		{"FullMoon-2026-02.as", "Moon opposite the Sun", func(p map[aspect.Body]float64) float64 { return apart(elongation(p), 180) }, 0.25},
		{"Eclipse-2026-08-12.as", "Moon at the Sun", func(p map[aspect.Body]float64) float64 { return apart(elongation(p), 0) }, 0.25},
		{"GreatConjunction-2020-12-21.as", "Jupiter at Saturn", func(p map[aspect.Body]float64) float64 { return apart(p[aspect.Jupiter], p[aspect.Saturn]) }, 0.15},
	}
	if len(tcs) != len(fixtureEvents) {
		t.Fatalf("%d events checked, %d on the allowlist", len(tcs), len(fixtureEvents))
	}
	reUTC := regexp.MustCompile(`(?m)^; .* ([0-9]{2}:[0-9]{2}) UTC per Astrolog 8\.00\.$`)
	for _, tc := range tcs {
		t.Run(tc.file, func(t *testing.T) {
			path := fixture(tc.file)
			out, err := exec.Command("astrolog", "-i", path).Output()
			if err != nil {
				t.Fatalf("astrolog -i %s: %v", path, err)
			}
			p := map[aspect.Body]float64{}
			for _, pos := range parsePositions(string(out)) {
				p[pos.Body] = pos.Longitude
			}
			if off := tc.off(p); off > tc.tol {
				t.Errorf("%s: %s is %.3f degrees off, want within %.2f", tc.file, tc.fact, off, tc.tol)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			m := reUTC.FindStringSubmatch(string(b))
			if m == nil {
				t.Fatalf("%s carries no `; ... HH:MM UTC per Astrolog 8.00.` comment", tc.file)
			}
			cf, err := ResolveChartFile(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if got := cf.WhenUTC.Format("15:04"); got != m[1] {
				t.Errorf("%s: the comment says %s UTC, the file resolves to %s UTC", tc.file, m[1], got)
			}
		})
	}
}
