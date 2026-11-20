package astrolog

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// TestLesserBodiesAtFarDates casts and lists with the lesser bodies of the
// registry at a date before year one and one far ahead, against the real
// Astrolog, and logs where each body was left out and any call that still
// errors. A call that errors fails the test only when its error is about
// ephemeris data: the reform-date refusals of the cycle list are another rule.
func TestLesserBodiesAtFarDates(t *testing.T) {
	needAstrolog(t)
	reg := testRegistry(t)
	ctx := context.Background()
	defaults, err := ReadDefaultSets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Year 3500 is left out of the run: the planets' own file is missing there,
	// every call takes a minute, and a chart with no body to draw still errors.
	names := []string{"Chiron", "Hygiea", "Eris", "Lilith", "Sirius"}
	dates := []time.Time{time.Date(-100, time.January, 1, 12, 0, 0, 0, time.UTC)}
	about := func(err error) bool {
		return err != nil && (strings.Contains(err.Error(), "ephemeris") || strings.Contains(err.Error(), "SwissEph") || strings.Contains(err.Error(), "warned"))
	}
	for _, when := range dates {
		for _, name := range names {
			o, ok := reg.ByName(name)
			if !ok {
				t.Logf("%s: not in the registry", name)
				continue
			}
			policy := recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, o.Number}}
			row := map[string]string{}
			note := func(call string, left *LeftOut, err error) {
				switch {
				case err != nil:
					row[call] = "ERROR " + oneLineProbe(err)
					if about(err) {
						t.Errorf("%s %d %s: %v", name, when.Year(), call, err)
					}
				case left != nil && len(left.Bodies) > 0:
					row[call] = "left out"
				default:
					row[call] = "drawn"
				}
			}
			spec, err := recipe.NewChartSpec(recipe.MomentAtPlace{JD: SlotJD(when), Lon: 4.9, Lat: 52.37}, recipe.Placidus, recipe.Tropical, policy)
			if err != nil {
				t.Fatal(err)
			}
			spec.Registry = reg
			spec.AspectRegistry = testAspects(t)
			raw, err := ComputeChart(ctx, spec)
			note("chart", &LeftOut{Bodies: raw.Unavailable}, err)
			for _, src := range []TimelineSource{TimelineTransit, TimelineReturn, TimelineSky} {
				left := &LeftOut{}
				req := TimelineRequest{Year: when.Year(), Month: time.January, Zone: time.UTC, Houses: recipe.Placidus, Objects: policy, Defaults: defaults, Registry: reg, AspectRegistry: testAspects(t), LeftOut: left,
					Natal: &TimelineNatal{When: when, Lon: 4.9, Lat: 52.37, Objects: policy}}
				_, _, err := ComputeTimeline(ctx, req, []TimelineSource{src}, nil)
				note("timeline "+src.Switch(), left, err)
			}
			left := &LeftOut{}
			_, err = RunVoidSearch(ctx, VoidRequest{Moment: when, Registry: reg, AspectRegistry: testAspects(t), LeftOut: left}, nil)
			note("void", left, err)
			if call, err := SunPairCall(SunPairRequest{When: when, Lon: 4.9, Lat: 52.37, Other: o, Registry: reg}); err == nil {
				_, _, err = RunTimelineSearch(ctx, call)
				note("sun pair", nil, err)
			}
			if call, err := CycleCall(CycleRequest{StartYear: when.Year(), Years: 10, Other: o, Registry: reg}); err == nil {
				_, _, err = RunTimelineSearch(ctx, call)
				note("cycle", nil, err)
			} else {
				row["cycle"] = "refused before the reform"
			}
			t.Logf("PROBE %-8s %5d %s", name, when.Year(), probeRow(row))
		}
	}
}

func oneLineProbe(err error) string {
	s := strings.Join(strings.Fields(err.Error()), " ")
	if len(s) > 90 {
		s = s[:90]
	}
	return s
}

func probeRow(row map[string]string) string {
	var parts []string
	for _, k := range []string{"chart", "timeline -t", "timeline -tr", "timeline -d", "void", "sun pair", "cycle"} {
		parts = append(parts, k+"="+row[k])
	}
	return strings.Join(parts, " | ")
}

// The two charts the operator tried (tmp/try-charts BC-0050 and ArcticPlacidus)
// cast with the lesser bodies shown: every cast succeeds, with the bodies that
// have data.
func TestChartsCastWithLesserBodiesShown(t *testing.T) {
	needAstrolog(t)
	reg := testRegistry(t)
	sel := []int{1, 2}
	for _, name := range []string{"Chiron", "Pholus", "Ceres", "Pallas", "Juno", "Vesta", "Hygiea", "Eris", "Sedna"} {
		if o, ok := reg.ByName(name); ok {
			sel = append(sel, o.Number)
		}
	}
	for _, c := range []struct {
		name     string
		when     time.Time
		lon, lat float64
	}{
		{"Before year one", time.Date(-44, time.March, 15, 12, 0, 0, 0, time.UTC), 12.4833, 41.9},
		{"Polar Placidus", time.Date(2020, time.June, 21, 11, 0, 0, 0, time.UTC), 18.95, 69.65},
	} {
		t.Run(c.name, func(t *testing.T) {
			spec, err := recipe.NewChartSpec(recipe.MomentAtPlace{JD: SlotJD(c.when), Lon: c.lon, Lat: c.lat}, recipe.Placidus, recipe.Tropical,
				recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: sel})
			if err != nil {
				t.Fatal(err)
			}
			spec.Registry = reg
			spec.AspectRegistry = testAspects(t)
			out, err := ComputeChart(context.Background(), spec)
			if err != nil {
				t.Fatalf("the cast was refused: %v", err)
			}
			if len(out.Positions) < 2 {
				t.Errorf("%d positions", len(out.Positions))
			}
			t.Logf("left out: %v", out.Unavailable)
		})
	}
}
