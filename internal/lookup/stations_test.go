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
		{"+0.049 is a candidate", aspect.Position{Body: aspect.Mercury, Speed: 0.049}, true},
		{"+0.000 is a candidate", aspect.Position{Body: aspect.Saturn, Speed: 0}, true},
		{"+0.050 is not", aspect.Position{Body: aspect.Mercury, Speed: 0.050}, false},
		{"-0.010 is not (retrograde)", aspect.Position{Body: aspect.Mercury, Speed: -0.010}, false},
		{"ascendant at +0.000 is not", aspect.Position{Body: aspect.Ascendant, Speed: 0}, false},
		{"midheaven at +0.000 is not", aspect.Position{Body: aspect.Midheaven, Speed: 0}, false},
		{"north node at +0.000 is not", aspect.Position{Body: aspect.NorthNode, Speed: 0}, false},
		{"south node at +0.000 is not", aspect.Position{Body: aspect.SouthNode, Speed: 0}, false},
		{"lilith at +0.000 is not", aspect.Position{Body: aspect.Lilith, Speed: 0}, false},
		{"unnamed asteroid at +0.000 is not", aspect.Position{Body: aspect.BodyUnknown, Label: "Cere", Speed: 0}, false},
	}
	for _, c := range cases {
		if got := stationCandidate(c.p); got != c.want {
			t.Errorf("%s: candidate = %v, want %v", c.name, got, c.want)
		}
	}

	// A negative zero is Astrolog's R: not a candidate either.
	if stationCandidate(aspect.Position{Body: aspect.Mercury, Speed: math.Copysign(0, -1)}) {
		t.Error("-0.000 is a candidate; Astrolog prints R there")
	}

	// No candidate, no Astrolog call.
	probe := &stationProbe{}
	got, cmds, err := StationMarks(context.Background(), probe.probers(), []aspect.Position{
		{Body: aspect.Sun, Speed: 1.0},
		{Body: aspect.Mercury, Speed: 0.050},
		{Body: aspect.Ascendant, Speed: 0},
		{Body: aspect.NorthNode, Speed: 0},
		{Body: aspect.BodyUnknown, Label: "Cere", Speed: 0},
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
	probe := &stationProbe{pos: []aspect.Position{{Body: aspect.Mercury, Speed: 0.02}}}
	positions := []aspect.Position{{Body: aspect.Mercury, Speed: 0.03}, {Body: aspect.Venus, Speed: 0.04}}
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
	wantSpec, err := Hellenistic.chartSpecAt(2461137.0, place, store.ReportLabel{})
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
		{"+0.030 to +0.000 is S/R", 0.030, []aspect.Position{{Body: aspect.Mercury, Speed: 0.000}}, StationRetro},
		{"+0.000 to +0.119 is S/D", 0.000, []aspect.Position{{Body: aspect.Mercury, Speed: 0.119}}, StationDirect},
		{"+0.030 to -0.010 is S/R", 0.030, []aspect.Position{{Body: aspect.Mercury, Speed: -0.010}}, StationRetro},
		{"equal is none", 0.030, []aspect.Position{{Body: aspect.Mercury, Speed: 0.030}}, StationNone},
		{"missing is none", 0.030, []aspect.Position{{Body: aspect.Venus, Speed: 0.5}}, StationNone},
	}
	for _, c := range cases {
		probe := &stationProbe{pos: c.later}
		marks, _, err := StationMarks(context.Background(), probe.probers(),
			[]aspect.Position{{Body: aspect.Mercury, Speed: c.from}}, at, stationPlace(), Hellenistic)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := marks[aspect.Mercury]; got != c.want {
			t.Errorf("%s: mark = %q, want %q", c.name, got, c.want)
		}
	}
	if StationRetro.String() != "S/R" || StationDirect.String() != "S/D" || StationNone.String() != "" {
		t.Errorf("Station text = %q %q %q", StationRetro, StationDirect, StationNone)
	}
}

// A failing probe returns the error and the command that failed, no marks.
func TestStationMarksProbeFailure(t *testing.T) {
	probe := &stationProbe{err: errors.New("boom")}
	marks, cmds, err := StationMarks(context.Background(), probe.probers(),
		[]aspect.Position{{Body: aspect.Mercury, Speed: 0.03}}, time.Now(), stationPlace(), Hellenistic)
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
		body aspect.Body
		at   time.Time
		want Station
	}{
		{"Mercury S/R", aspect.Mercury, time.Date(2026, 10, 24, 7, 12, 0, 0, time.UTC), StationRetro},
		{"Venus S/R", aspect.Venus, time.Date(2026, 10, 3, 7, 15, 0, 0, time.UTC), StationRetro},
		{"Pluto S/D", aspect.Pluto, time.Date(2026, 10, 16, 2, 40, 59, 0, time.UTC), StationDirect},
		{"Saturn S/D", aspect.Saturn, time.Date(2026, 12, 10, 23, 31, 59, 0, time.UTC), StationDirect},
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
			if got := marks[tc.body]; got != tc.want {
				var speed float64
				for _, p := range res.Positions {
					if p.Body == tc.body {
						speed = p.Speed
					}
				}
				t.Errorf("%s at %s: rule says %q (speed %v), Astrolog lists %q", tc.name, tc.at.Format(time.RFC3339), got, speed, tc.want)
			}
		})
	}
}
