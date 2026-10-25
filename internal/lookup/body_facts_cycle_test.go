package lookup

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// C1: the phase and its sense from the unrounded elongation.
func TestSynodicPhase(t *testing.T) {
	for e, want := range map[float64][2]string{
		14.87: {"evening-star", "occidental"}, -0.0001: {"morning-star", "oriental"}, 0: {"at-conjunction", "at-conjunction"},
	} {
		if p, s := SynodicPhase(e); p != want[0] || s != want[1] {
			t.Errorf("SynodicPhase(%v) = %s %s, want %v", e, p, s, want)
		}
	}
}

// C2: approaching while the distance shrinks, leaving while it grows, none
// at zero elongation or zero rate.
func TestElongationTrend(t *testing.T) {
	for _, tc := range []struct {
		e, rate float64
		want    string
		ok      bool
	}{
		{14.87, -1.42, "approaching", true}, // east, Venus retrograde: closing
		{14.87, 0.3, "leaving", true},
		{-10, 0.3, "approaching", true}, // west, gaining on the Sun
		{-10, -0.3, "leaving", true},
		{0, 1, "", false},
		{5, 0, "", false},
	} {
		if got, ok := ElongationTrend(tc.e, tc.rate); got != tc.want || ok != tc.ok {
			t.Errorf("ElongationTrend(%v, %v) = %q %v, want %q %v", tc.e, tc.rate, got, ok, tc.want, tc.ok)
		}
	}
}

func researchFacts(t *testing.T, when time.Time, zone *time.Location) map[string]BodyFact {
	t.Helper()
	pair := SunPairResult{
		Sun:  astrolog.PrecisePosition{Body: aspect.Sun, Longitude: 180 + 21 + 9.0/60 + 41.0/3600, Speed: 0.9907618, Sign: "Lib", Magnitude: -26.87, LitPercent: 100},
		Body: astrolog.PrecisePosition{Body: aspect.Venus, Longitude: 210 + 6 + 2.0/60 + 8.0/3600, Speed: -0.4292777, Sign: "Sco", Retrograde: true, Magnitude: -4.32, LitPercent: 4.03},
	}
	facts, err := NowFacts(pair, CycleResult{Span: CycleSpanFor(when, zone), Events: cycleFixture(t)}, when, zone)
	if err != nil {
		t.Fatal(err)
	}
	return facts
}

// C4: the last station at or before the moment, the next strictly after,
// each with its kind, rounded to the minute in the slot's zone.
func TestLastAndNextStation(t *testing.T) {
	f := researchFacts(t, time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), time.UTC)
	if g := f["station.last"]; g.Term != "station-retrograde" || g.Text != "Oct 3, 2026 7:16 8°29′ Sco" {
		t.Errorf("station.last %+v", g)
	}
	if g := f["station.next"]; g.Term != "station-direct" || g.Text != "Nov 14, 2026 0:27 22°51′ Lib" {
		t.Errorf("station.next %+v", g)
	}
	if f["motion.direction"].Term != "retrograde" {
		t.Errorf("motion %+v", f["motion.direction"])
	}
	// A blank R column is direct motion.
	pair := SunPairResult{Sun: astrolog.PrecisePosition{Body: aspect.Sun, Sign: "Lib"}, Body: astrolog.PrecisePosition{Body: aspect.Venus, Longitude: 216, Sign: "Sco", Speed: 1.2}}
	when := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	direct, err := NowFacts(pair, CycleResult{Span: CycleSpanFor(when, time.UTC), Events: cycleFixture(t)}, when, time.UTC)
	if err != nil || direct["motion.direction"].Term != "direct" {
		t.Errorf("a blank R column: %+v, %v", direct["motion.direction"], err)
	}
	// In Berlin the same instants read two hours later (CEST).
	berlin, _ := time.LoadLocation("Europe/Berlin")
	if g := researchFacts(t, time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), berlin)["station.last"]; g.Text != "Oct 3, 2026 9:16 8°29′ Sco" {
		t.Errorf("station.last in Berlin %+v", g)
	}
	// At the station's own minute it is the last one, not the next.
	at := researchFacts(t, time.Date(2026, 10, 3, 7, 16, 0, 0, time.UTC).Add(time.Minute), time.UTC)
	if !strings.HasPrefix(at["station.last"].Text, "Oct 3, 2026") {
		t.Errorf("just after the station: %+v", at["station.last"])
	}
}

// C5: the four conjunctions, last at or before, next strictly after, in UT.
func TestLastAndNextConjunction(t *testing.T) {
	f := researchFacts(t, time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), time.UTC)
	for key, want := range map[string]string{
		"synodic.conjunction.inferior.last": "Mar 23, 2025 1:08 2°39′ Ari",
		"synodic.conjunction.inferior.next": "Oct 24, 2026 3:44 0°45′ Sco",
		"synodic.conjunction.superior.last": "Jan 6, 2026 16:36 16°22′ Cap",
		"synodic.conjunction.superior.next": "Aug 12, 2027 0:21 19°06′ Leo",
	} {
		if g := f[key]; g.Text != want || g.Kind != FactInstant || g.Term != "" {
			t.Errorf("%s = %+v, want %q", key, g, want)
		}
	}
	// At the conjunction's own second it is the last one, not the next.
	at := researchFacts(t, time.Date(2026, 10, 24, 3, 44, 6, 0, time.UTC), time.UTC)
	if g := at["synodic.conjunction.inferior.last"].Text; g != "Oct 24, 2026 3:44 0°45′ Sco" {
		t.Errorf("at the conjunction, last = %q", g)
	}
	if g := at["synodic.conjunction.inferior.next"].Text; !strings.HasPrefix(g, "Jun 1, 2028 ") {
		t.Errorf("at the conjunction, next = %q", g)
	}
	// A list without a next outer conjunction is a failure, not an absence.
	var cut []astrolog.CycleEvent
	for _, e := range cycleFixture(t) {
		if e.Instant.Year() < 2027 {
			cut = append(cut, e)
		}
	}
	when := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	if _, err := NowFacts(SunPairResult{Sun: astrolog.PrecisePosition{Body: aspect.Sun}, Body: astrolog.PrecisePosition{Body: aspect.Venus, Longitude: 10}},
		CycleResult{Span: CycleSpanFor(when, time.UTC), Events: cut}, when, time.UTC); err == nil {
		t.Error("a cycle list without a next outer conjunction gave facts")
	}
}

// C6: magnitude to one decimal with a true minus, lit as a whole percent.
func TestBrightness(t *testing.T) {
	f := researchFacts(t, time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), time.UTC)
	if g := f["brightness.magnitude"]; g.Text != "−4.3" || g.Num != -4.32 {
		t.Errorf("magnitude %+v", g)
	}
	if g := f["brightness.lit"]; g.Text != "4%" {
		t.Errorf("lit %+v", g)
	}
	for m, want := range map[float64]string{-0.04: "0.0", 0.04: "0.0", -0.05: "−0.1", 1.26: "1.3", -26.87: "−26.9"} {
		if got := magnitudeText(m); got != want {
			t.Errorf("magnitudeText(%v) = %q, want %q", m, got, want)
		}
	}
}

// C7: the sixth-most-recent inner conjunction and the drift to the newest.
func TestEightYearDrift(t *testing.T) {
	f := researchFacts(t, time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), time.UTC)
	if g := f["synodic.conjunction.inferior.sixth"]; g.Text != "Mar 25, 2017   4°57′ Ari" {
		t.Errorf("sixth %+v", g)
	}
	if g := f["synodic.conjunction.inferior.drift"]; g.Text != "−2°18′" || math.Abs(g.Num-(-(2+18.0/60))) > 1e-9 {
		t.Errorf("drift %+v", g)
	}
	if d := EightYearDrift(astrolog.CyclePoint{Deg: 1, Sign: "Ari"}, astrolog.CyclePoint{Deg: 29, Sign: "Pis"}); math.Abs(d-2) > 1e-9 {
		t.Errorf("across Aries: %v, want +2", d)
	}
	// Fewer than six inner conjunctions before the moment is a failure.
	var late []astrolog.CycleEvent
	for _, e := range cycleFixture(t) {
		if e.Instant.Year() >= 2018 {
			late = append(late, e)
		}
	}
	when := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	_, err := NowFacts(SunPairResult{Sun: astrolog.PrecisePosition{Body: aspect.Sun}, Body: astrolog.PrecisePosition{Body: aspect.Venus, Longitude: 10}},
		CycleResult{Span: CycleSpanFor(when, time.UTC), Events: late}, when, time.UTC)
	var missing *MissingEventError
	if !errors.As(err, &missing) {
		t.Errorf("five inner conjunctions gave %v, want a MissingEventError", err)
	}
	if f["sign.same-as-sun"].Term != "no" || SameSignAsSun("Sco", "Sco") != "yes" {
		t.Errorf("same sign %+v", f["sign.same-as-sun"])
	}
}

// C8: the live Astrolog 8.00 binary at 2026-10-14 12:00 UT gives the values
// the research catalogue checked against NAOJ and JPL Horizons. With no
// astrolog on PATH this FAILS: the spawn returns an error.
func TestCycleAgainstHorizons2026(t *testing.T) {
	when := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	res, err := bodyNowFactsWith(context.Background(), BodyFactsRequest{Body: aspect.Venus, When: when,
		Place: location.PlaceInfo{Lat: 51, Lon: 0, TZ: "UTC"}, Lens: Hellenistic}, astrolog.RunTimelineSearch)
	if err != nil {
		t.Fatal(err)
	}
	f := res.Now
	for key, want := range map[string]string{
		"synodic.phase": "evening-star", "elongation.side": "east", "elongation.trend": "approaching",
		"station.last": "station-retrograde", "station.next": "station-direct",
	} {
		if f[key].Term != want {
			t.Errorf("%s = %+v, want %s", key, f[key], want)
		}
	}
	for key, want := range map[string]string{
		"elongation.lon":                     "14°52′",
		"station.last":                       "Oct 3, 2026 7:16 8°29′ Sco",
		"station.next":                       "Nov 14, 2026 0:27 22°51′ Lib",
		"synodic.conjunction.inferior.next":  "Oct 24, 2026 3:44 0°45′ Sco",
		"synodic.conjunction.superior.last":  "Jan 6, 2026 16:36 16°22′ Cap",
		"synodic.conjunction.superior.next":  "Aug 12, 2027 0:21 19°06′ Leo",
		"brightness.magnitude":               "−4.3",
		"brightness.lit":                     "4%",
		"synodic.conjunction.inferior.sixth": "Mar 25, 2017   4°57′ Ari",
		"synodic.conjunction.inferior.drift": "−2°18′",
	} {
		if f[key].Text != want {
			t.Errorf("%s = %q, want %q", key, f[key].Text, want)
		}
	}
	if g := f["synodic.conjunction.inferior.last"].Text; !strings.HasPrefix(g, "Mar 23, 2025 ") || !strings.HasSuffix(g, " 2°39′ Ari") {
		t.Errorf("inferior last %q", g)
	}
	var years []string
	for _, it := range f["synodic.conjunction.inferior.last5"].Items {
		// "Mar 23, 2025   2°39′ Ari": the year is the third field, the sign the last.
		fs := strings.Fields(it)
		years = append(years, fs[2]+" "+fs[len(fs)-1])
	}
	if strings.Join(years, ", ") != "2025 Ari, 2023 Leo, 2022 Cap, 2020 Gem, 2018 Sco" {
		t.Errorf("last five %v", years)
	}
}

// The instants read in the operator's held clock form, like every other
// moment on screen (the specification): month or day first, 24 or 12 hours.
func TestInstantsFollowTheHeldClock(t *testing.T) {
	defer HoldClock(HeldClock())
	when := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		pref ClockPref
		want string
	}{
		{ClockPref{}, "Oct 3, 2026 7:16 8°29′ Sco"},
		{ClockPref{DayFirst: true}, "3 Oct 2026 7:16 8°29′ Sco"},
		{ClockPref{Hours12: true}, "Oct 3, 2026 7:16am 8°29′ Sco"},
		{ClockPref{Hours12: true, DayFirst: true}, "3 Oct 2026 7:16am 8°29′ Sco"},
	} {
		HoldClock(tc.pref)
		if got := researchFacts(t, when, time.UTC)["station.last"].Text; got != tc.want {
			t.Errorf("%+v: %q, want %q", tc.pref, got, tc.want)
		}
	}
	HoldClock(ClockPref{Hours12: true})
	if got := researchFacts(t, when, time.UTC)["station.next"].Text; got != "Nov 14, 2026 12:27am 22°51′ Lib" {
		t.Errorf("12-hour midnight: %q", got)
	}
}
