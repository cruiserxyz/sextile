package lookup

import (
	"context"
	"errors"
	"math"
	"os/exec"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/payload"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// sp is a position of a registry object as a parsed row carries it: with the
// registry's Object, which is what the marks key on.
func sp(t *testing.T, o aspect.Object, speed float64) aspect.Position {
	t.Helper()
	if got, ok := Hellenistic.Registry.ByNumber(o.Number); !ok || got != o {
		t.Fatalf("registry has no %v", o)
	}
	return aspect.Position{Object: o, Speed: speed}
}

func stationPlace() location.PlaceInfo {
	return location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"}
}

// stationProbe is a PositionsOnly stub that records each call's JD and answers
// with the given positions.
type stationProbe struct {
	calls []recipe.ChartSpec
	jds   []float64
	pos   []aspect.Position
	err   error
}

func (s *stationProbe) probers() *Probers {
	stub := ChartComputer(func(ctx context.Context, spec recipe.ChartSpec) (payload.RawOutput, error) {
		s.calls = append(s.calls, spec)
		s.jds = append(s.jds, spec.Source.(recipe.MomentAtPlace).JD)
		return payload.RawOutput{Positions: s.pos, Commands: []string{"astrolog probe"}}, s.err
	})
	return &Probers{PositionsOnly: stub}
}

// N1 / C1: the boundary and the zero-call case.
func TestStationCandidatesAreTheSlowDirectBodies(t *testing.T) {
	cases := []struct {
		name string
		p    aspect.Position
		want bool
	}{
		{"+0.049 is a candidate", sp(t, objMercury, 0.049), true},
		{"+0.000 is a candidate", sp(t, objSaturn, 0), true},
		{"+0.050 is not", sp(t, objMercury, 0.050), false},
		{"-0.010 is not (retrograde)", sp(t, objMercury, -0.010), false},
		{"ascendant at +0.000 is not", sp(t, objAscendant, 0), false},
		{"midheaven at +0.000 is not", sp(t, objMidheaven, 0), false},
		{"north node at +0.000 is not", sp(t, objNorthNode, 0), false},
		{"south node at +0.000 is not", sp(t, objSouthNode, 0), false},
		{"lilith at +0.000 is not", sp(t, objLilith, 0), false},
		{"Ceres (12) at +0.000 is a candidate", aspect.Position{Object: registryObject(t, 12), Speed: 0}, true},
		{"Ceres (12) slow and direct is a candidate", aspect.Position{Object: registryObject(t, 12), Speed: 0.03}, true},
		{"a position with no Object is not", aspect.Position{Speed: 0}, false},
		{"Sirius (84, a star) at +0.000 is not", aspect.Position{Object: registryObject(t, 84), Speed: 0}, false},
		{"Fortune (19) at +0.000 is not", aspect.Position{Object: registryObject(t, 19), Speed: 0}, false},
		{"Vertex (20) at +0.000 is not", aspect.Position{Object: registryObject(t, 20), Speed: 0}, false},
		{"East Point (21) at +0.000 is not", aspect.Position{Object: registryObject(t, 21), Speed: 0}, false},
		{"11th Cusp (32) at +0.000 is not", aspect.Position{Object: registryObject(t, 32), Speed: 0}, false},
	}
	for _, c := range cases {
		if got := stationCandidate(c.p, Hellenistic.Registry); got != c.want {
			t.Errorf("%s: candidate = %v, want %v", c.name, got, c.want)
		}
	}

	// A negative zero is Astrolog's R: not a candidate either.
	if stationCandidate(sp(t, objMercury, math.Copysign(0, -1)), Hellenistic.Registry) {
		t.Error("-0.000 is a candidate; Astrolog prints R there")
	}

	// No candidate, no Astrolog call.
	probe := &stationProbe{}
	got, cmds, err := StationMarks(context.Background(), probe.probers(), []aspect.Position{
		sp(t, objSun, 1.0),
		sp(t, objMercury, 0.050),
		sp(t, objAscendant, 0),
		sp(t, objNorthNode, 0),
		{Object: registryObject(t, 84), Speed: 0},
	}, time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), stationPlace(), Hellenistic)
	if err != nil || len(got) != 0 || len(cmds) != 0 {
		t.Fatalf("no candidate: marks=%v cmds=%v err=%v, want none", got, cmds, err)
	}
	if len(probe.calls) != 0 {
		t.Errorf("no candidate made %d prober calls, want 0", len(probe.calls))
	}
}

// C2: ONE call, JD exactly +1.0, same place and lens, across a DST change.
func TestStationProbeIsOneCallADayLater(t *testing.T) {
	place := stationPlace()
	berlin, err := time.LoadLocation(place.TZ)
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	// 2026-03-28 12:00 Berlin; the next civil day crosses the DST change
	// (2026-03-29 02:00), so a civil-clock "+24h" would not be one UT day.
	at := time.Date(2026, 3, 28, 12, 0, 0, 0, berlin)
	probe := &stationProbe{pos: []aspect.Position{sp(t, objMercury, 0.02)}}
	positions := []aspect.Position{sp(t, objMercury, 0.03), sp(t, objVenus, 0.04)}
	_, cmds, err := StationMarks(context.Background(), probe.probers(), positions, at, place, Hellenistic)
	if err != nil {
		t.Fatal(err)
	}
	if len(probe.calls) != 1 {
		t.Fatalf("two candidates made %d probe calls, want exactly 1", len(probe.calls))
	}
	if len(cmds) != 1 || cmds[0] != "astrolog probe" {
		t.Errorf("probe commands = %q, want the probe's own", cmds)
	}
	wantSpec, err := Hellenistic.chartSpecAt(AtMoment(2461137.0, place, store.ReportLabel{}))
	if err != nil {
		t.Fatal(err)
	}
	got := moment(t, probe.calls[0])
	want := moment(t, wantSpec)
	if got.Lon != want.Lon || got.Lat != want.Lat || got.TZ != want.TZ {
		t.Errorf("probe place = %+v, want %+v", got, want)
	}
	if probe.calls[0].Houses != wantSpec.Houses || probe.calls[0].Zodiac != wantSpec.Zodiac {
		t.Errorf("probe lens differs: houses %v/%v zodiac %v/%v", probe.calls[0].Houses, wantSpec.Houses, probe.calls[0].Zodiac, wantSpec.Zodiac)
	}
	// JD of 2026-03-28 10:00 UT is 2461137.9166...; +1.0 exactly.
	base := astrolog.SlotJD(at)
	if probe.jds[0] != base+1.0 {
		t.Errorf("probe JD = %v, want %v + 1.0 = %v", probe.jds[0], base, base+1.0)
	}
}

// N2 / C3: the direction comes from the probe; equal and missing show nothing.
func TestStationDirectionComesFromTheProbe(t *testing.T) {
	at := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		from  float64
		later []aspect.Position
		want  Station
	}{
		{"+0.030 to +0.000 is S/R", 0.030, []aspect.Position{sp(t, objMercury, 0.000)}, StationRetro},
		{"+0.000 to +0.119 is S/D", 0.000, []aspect.Position{sp(t, objMercury, 0.119)}, StationDirect},
		{"+0.030 to -0.010 is S/R", 0.030, []aspect.Position{sp(t, objMercury, -0.010)}, StationRetro},
		{"equal is none", 0.030, []aspect.Position{sp(t, objMercury, 0.030)}, StationNone},
		{"missing is none", 0.030, []aspect.Position{sp(t, objVenus, 0.5)}, StationNone},
	}
	for _, c := range cases {
		probe := &stationProbe{pos: c.later}
		marks, _, err := StationMarks(context.Background(), probe.probers(),
			[]aspect.Position{sp(t, objMercury, c.from)}, at, stationPlace(), Hellenistic)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := marks[registryObject(t, 3).Number]; got != c.want {
			t.Errorf("%s: mark = %q, want %q", c.name, got, c.want)
		}
	}
	if StationRetro.String() != "S/R" || StationDirect.String() != "S/D" || StationNone.String() != "" {
		t.Errorf("Station text = %q %q %q", StationRetro, StationDirect, StationNone)
	}
}

// The marks key on the object's number, not on a glyph: a Ceres mark (12)
// comes back under 12.
func TestStationMarksKeyOnTheNumber(t *testing.T) {
	ceres := registryObject(t, 12)
	probe := &stationProbe{pos: []aspect.Position{{Object: ceres, Speed: 0.0}}}
	marks, _, err := StationMarks(context.Background(), probe.probers(),
		[]aspect.Position{{Object: ceres, Speed: 0.03}}, time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), stationPlace(), Hellenistic)
	if err != nil {
		t.Fatal(err)
	}
	if len(marks) != 1 || marks[12] != StationRetro {
		t.Errorf("marks = %v, want {12: S/R}", marks)
	}
}

// A failing probe returns the error and the command that failed, no marks.
func TestStationMarksProbeFailure(t *testing.T) {
	probe := &stationProbe{err: errors.New("boom")}
	marks, cmds, err := StationMarks(context.Background(), probe.probers(),
		[]aspect.Position{sp(t, objMercury, 0.03)}, time.Now(), stationPlace(), Hellenistic)
	if err == nil || len(marks) != 0 {
		t.Fatalf("marks=%v err=%v, want an error and no marks", marks, err)
	}
	if len(cmds) != 1 {
		t.Errorf("commands = %q, want the failed probe's command", cmds)
	}
}

// C16 with REAL readings: at the landed second C14 gives, the real rule over
// Astrolog's own positions agrees with the word Astrolog lists for the station
// (S/R rows land on the minute's :00, S/D rows on :59). Times are UT, read off
// `astrolog -qm <m> 2026 -dm -z 0`.
func TestStationRuleAgreesWithAstrologAtListedStations(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live station test")
	}
	place := location.PlaceInfo{Name: "Amsterdam", Country: "NL", Lat: 52.3667, Lon: 4.9, TZ: "Europe/Amsterdam"}
	for _, tc := range []struct {
		name string
		body aspect.Object
		at   time.Time
		want Station
	}{
		{"Mercury S/R", objMercury, time.Date(2026, 10, 24, 7, 12, 0, 0, time.UTC), StationRetro},
		{"Venus S/R", objVenus, time.Date(2026, 10, 3, 7, 15, 0, 0, time.UTC), StationRetro},
		{"Pluto S/D", objPluto, time.Date(2026, 10, 16, 2, 40, 59, 0, time.UTC), StationDirect},
		{"Saturn S/D", objSaturn, time.Date(2026, 12, 10, 23, 31, 59, 0, time.UTC), StationDirect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := FirstCallTransits(context.Background(), nil, tc.at, place, Hellenistic)
			if err != nil {
				t.Fatal(err)
			}
			marks, _, err := StationMarks(context.Background(), nil, res.Positions, tc.at, place, Hellenistic)
			if err != nil {
				t.Fatal(err)
			}
			if got := marks[sp(t, tc.body, 0).Object.Number]; got != tc.want {
				var speed float64
				for _, p := range res.Positions {
					if p.Object == tc.body {
						speed = p.Speed
					}
				}
				t.Errorf("%s at %s: rule says %q (speed %v), Astrolog lists %q", tc.name, tc.at.Format(time.RFC3339), got, speed, tc.want)
			}
		})
	}
}

// Rule pinned (council Custodian, 2026-11-07): the planetary moons, Astrolog's
// objects 52 to 78, carry no station mark. A moon's geocentric speed crosses
// zero several times a day, so the one-day probe would alias. The dwarf (51)
// and the centres of body (79 and up) keep theirs.
func TestStationShapeExcludesPlanetaryMoons(t *testing.T) {
	for _, c := range []struct {
		n    int
		name string
		want bool
	}{
		{51, "Dwarf", true},
		{52, "FirstMoon", false},
		{54, "Io", false},
		{78, "LastMoon", false},
		{79, "CentreOfBody", true},
	} {
		p := aspect.Position{Object: aspect.Object{Number: c.n, Name: c.name}, Speed: 0.01}
		if got := stationShape(p); got != c.want {
			t.Errorf("object %d (%s) at +0.010: candidate = %v, want %v", c.n, c.name, got, c.want)
		}
	}
}
