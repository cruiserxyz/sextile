package lookup

import (
	"context"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Elongation wraps at Aries: a body just past 0° with the Sun just before
// it is a little east, not 359° west; the side is read on the unrounded
// value, and exactly 0 is at conjunction.
func TestElongationWrapsAtAries(t *testing.T) {
	for _, tc := range []struct {
		sun, body, want float64
		side            string
	}{
		{359, 1, 2, "east"},
		{1, 359, -2, "west"},
		{201.16, 216.04, 14.88, "east"},
		{10, 190, 180, "east"},
		{190, 10, 180, "east"}, // (−180, 180]: the opposition is +180
		{30, 30, 0, "at-conjunction"},
		{30, 30 - 1.0/3600, -1.0 / 3600, "west"},
	} {
		got := Elongation(tc.sun, tc.body)
		if math.Abs(got-tc.want) > 1e-9 || ElongationSide(got) != tc.side {
			t.Errorf("Elongation(%v, %v) = %v %s, want %v %s", tc.sun, tc.body, got, ElongationSide(got), tc.want, tc.side)
		}
	}
	if degMin(16.99972) != "16°59′" || degMin(17) != "17°00′" || degMin(14.87) != "14°52′" {
		t.Errorf("degMin: %s %s %s", degMin(16.99972), degMin(17), degMin(14.87))
	}
}

func cycleFixture(t *testing.T) []astrolog.CycleEvent {
	t.Helper()
	b, err := os.ReadFile("../astrolog/testdata/cycle-sun-venus-2016-13y.txt")
	if err != nil {
		t.Fatal(err)
	}
	ev, err := astrolog.ParseCycleEvents(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

// The last five inner conjunctions at or before the moment, newest first;
// an outer one (Venus in round brackets) never counts.
func TestLastFiveInferior(t *testing.T) {
	events := cycleFixture(t)
	when := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC) // after the 03:44 UT inner conjunction
	got := LastInferior(events, aspect.Venus, when, 5)
	var dates []string
	for _, e := range got {
		dates = append(dates, e.Instant.Format("2006-01-02")+" "+e.OtherAt.Sign)
	}
	want := "2026-10-24 Sco 2025-03-23 Ari 2023-08-13 Leo 2022-01-09 Cap 2020-06-03 Gem"
	if strings.Join(dates, " ") != want {
		t.Errorf("got %v, want %s", dates, want)
	}
	before := LastInferior(events, aspect.Venus, time.Date(2026, 10, 24, 3, 0, 0, 0, time.UTC), 5)
	if before[0].Instant.Year() != 2025 {
		t.Errorf("an hour before the conjunction, the newest is %v", before[0].Instant)
	}
	for _, e := range LastInferior(events, aspect.Venus, when, 99) {
		if e.OtherAt.Bracket != '[' {
			t.Errorf("an outer conjunction counted: %s", e.Line)
		}
	}
}

// The facts for the research catalogue's sky, 2026-10-14 12:00 UT: Venus
// 6Sco02′08″ R, Sun 21Lib09′41″ — 14°52′ east, the list in the slot's zone.
func TestNowFactsForTheResearchSky(t *testing.T) {
	when := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	pair := SunPairResult{
		Sun:  astrolog.PrecisePosition{Body: aspect.Sun, Longitude: 180 + 21 + 9.0/60 + 41.0/3600},
		Body: astrolog.PrecisePosition{Body: aspect.Venus, Longitude: 210 + 6 + 2.0/60 + 8.0/3600},
	}
	cycle := CycleResult{Span: CycleSpanFor(when, time.UTC), Events: cycleFixture(t)}
	facts, err := NowFacts(pair, cycle, when, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if f := facts["elongation.lon"]; f.Text != "14°52′" || math.Abs(f.Num-(14+52.0/60+27.0/3600)) > 1e-9 {
		t.Errorf("elongation.lon %+v", f)
	}
	if f := facts["elongation.lon.abs"]; math.Abs(f.Num-(14+52.0/60+27.0/3600)) > 1e-9 {
		t.Errorf("the measure a rule judges is not the unrounded value: %+v", f)
	}
	if facts["elongation.side"].Term != "east" {
		t.Errorf("side %+v", facts["elongation.side"])
	}
	items := facts["synodic.conjunction.inferior.last5"].Items
	if len(items) != 5 || items[0] != "Mar 23, 2025   2°39′ Ari" {
		t.Errorf("items %q", items)
	}
	// A search that holds fewer inner conjunctions than the list needs is a
	// failure, never a short list.
	var few []astrolog.CycleEvent
	for _, e := range cycle.Events {
		if e.Instant.Year() >= 2022 {
			few = append(few, e)
		}
	}
	if _, err := NowFacts(pair, CycleResult{Span: cycle.Span, Events: few}, when, time.UTC); err == nil {
		t.Error("a search with three inner conjunctions rendered a short list")
	}
	// A moment outside the span's year is refused, never computed on the
	// wrong events.
	if _, err := NowFacts(pair, CycleResult{Span: CycleSpan{StartYear: 2000, Years: 13}}, when, time.UTC); err == nil {
		t.Error("a cycle from another span was used")
	}
}

// The searches run through the searcher and keep their commands.
func TestBodyFactSearchesCarryTheirCommands(t *testing.T) {
	b, _ := os.ReadFile("../astrolog/testdata/cycle-sun-venus-2016-13y.txt")
	search := func(_ context.Context, call astrolog.BodyCall) (string, string, error) {
		return string(b), "astrolog -dY …", nil
	}
	res, err := SearchCycle(context.Background(), aspect.Venus, CycleSpan{StartYear: 2016, Years: 13}, Hellenistic, search)
	if err != nil || len(res.Commands) != 1 || len(res.Events) == 0 {
		t.Errorf("%+v, %v", res.Commands, err)
	}
	bad := func(_ context.Context, call astrolog.BodyCall) (string, string, error) {
		return "garbage\n", "astrolog -dY …", nil
	}
	if res, err := SearchCycle(context.Background(), aspect.Venus, CycleSpan{StartYear: 2016, Years: 13}, Hellenistic, bad); err == nil || len(res.Commands) != 1 {
		t.Errorf("an unreadable result: %v, commands %v", err, res.Commands)
	}
}

// The span and the list's dates are the floating slot's zone's, not UTC's:
// at Kiritimati (UTC+14) 31 December 2025 12:00 UT is already 2026, and an
// inner conjunction late on a UT day is dated the next local day.
func TestCycleSpanAndListDatesAreTheSlotsZone(t *testing.T) {
	kiri, err := time.LoadLocation("Pacific/Kiritimati")
	if err != nil {
		t.Fatal(err)
	}
	when := time.Date(2025, 12, 31, 12, 0, 0, 0, time.UTC)
	if got := CycleSpanFor(when, kiri); got != (CycleSpan{StartYear: 2016, Years: 13}) {
		t.Errorf("span at Kiritimati %+v, want 2016+13 (local year 2026)", got)
	}
	if got := CycleSpanFor(when, time.UTC); got.StartYear != 2015 {
		t.Errorf("span at UTC %+v, want 2015", got)
	}
	pair := SunPairResult{Sun: astrolog.PrecisePosition{Body: aspect.Sun}, Body: astrolog.PrecisePosition{Body: aspect.Venus, Longitude: 10}}
	facts, err := NowFacts(pair, CycleResult{Span: CycleSpanFor(when, kiri), Events: cycleFixture(t)}, when, kiri)
	if err != nil {
		t.Fatal(err)
	}
	// 2025-03-23 01:07 UT is 15:07 the same day at UTC+14; 2023-08-13
	// 11:16 UT is 01:16 on the 14th there.
	items := facts["synodic.conjunction.inferior.last5"].Items
	if len(items) < 2 || items[0] != "Mar 23, 2025   2°39′ Ari" || !strings.HasPrefix(items[1], "Aug 14, 2023 ") {
		t.Errorf("items at Kiritimati %q", items)
	}
}

// R5 of the specification: the list's dates follow the
// operator's clock order, like every date Sextile spells itself.
func TestTabDatesFollowTheClock(t *testing.T) {
	held := HeldClock()
	t.Cleanup(func() { HoldClock(held) })
	when := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	pair := SunPairResult{
		Sun:  astrolog.PrecisePosition{Body: aspect.Sun, Longitude: 180 + 21 + 9.0/60 + 41.0/3600},
		Body: astrolog.PrecisePosition{Body: aspect.Venus, Longitude: 210 + 6 + 2.0/60 + 8.0/3600},
	}
	cycle := CycleResult{Span: CycleSpanFor(when, time.UTC), Events: cycleFixture(t)}
	for _, tc := range []struct {
		pref ClockPref
		want string
	}{{ClockPref{}, "Mar 23, 2025   2°39′ Ari"}, {ClockPref{DayFirst: true}, "23 Mar 2025   2°39′ Ari"}, {ClockPref{Hours12: true}, "Mar 23, 2025   2°39′ Ari"}} {
		HoldClock(tc.pref)
		facts, err := NowFacts(pair, cycle, when, time.UTC)
		if err != nil {
			t.Fatal(err)
		}
		if got := facts["synodic.conjunction.inferior.last5"].Items[0]; got != tc.want {
			t.Errorf("%+v: %q, want %q", tc.pref, got, tc.want)
		}
	}
}

// R6: the degree is padded so the ° sign sits in one column down the list.
func TestDegreesAlignOnTheSign(t *testing.T) {
	when := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	pair := SunPairResult{
		Sun:  astrolog.PrecisePosition{Body: aspect.Sun, Longitude: 201},
		Body: astrolog.PrecisePosition{Body: aspect.Venus, Longitude: 216},
	}
	cycle := CycleResult{Span: CycleSpanFor(when, time.UTC), Events: cycleFixture(t)}
	facts, err := NowFacts(pair, cycle, when, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	items := facts["synodic.conjunction.inferior.last5"].Items
	col := strings.Index(items[0], "°")
	for _, it := range items {
		if strings.Index(it, "°") != col {
			t.Errorf("° at %d in %q, at %d in %q", strings.Index(it, "°"), it, col, items[0])
		}
	}
	if !strings.Contains(items[0], "   2°39′") {
		t.Errorf("a one-digit degree is not padded: %q", items[0])
	}
}
