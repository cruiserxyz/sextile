package astrolog

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The parser reads every line of a real cycle search (Astrolog 8.00, Sun and
// Venus, 2016-2028, -z 0 -b0): conjunctions with the occultation suffix,
// retrograde ingresses, stations and the equinox and solstice suffixes.
func TestParseCycleEvents(t *testing.T) {
	b, err := os.ReadFile("testdata/cycle-sun-venus-2016-13y.txt")
	if err != nil {
		t.Fatal(err)
	}
	events, err := ParseCycleEvents(string(b))
	if err != nil {
		t.Fatal(err)
	}
	lines := 0
	for _, l := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(l) != "" {
			lines++
		}
	}
	if len(events) != lines {
		t.Errorf("%d events from %d lines", len(events), lines)
	}
	var cons, stations int
	for _, e := range events {
		switch e.Kind {
		case CycleConjunction:
			cons++
		case CycleStation:
			stations++
		}
	}
	if cons != 16 || stations == 0 {
		t.Errorf("%d conjunctions, %d stations", cons, stations)
	}
	// 2017-03-25 10:17:07 UT, Sun ( 4Ari57) Con [ 4Ari57] Venus: Venus in
	// brackets, retrograde.
	want := CycleEvent{Instant: time.Date(2017, 3, 25, 10, 17, 7, 0, time.UTC), Kind: CycleConjunction, Body: aspect.Sun, Other: aspect.Venus,
		At: CyclePoint{Deg: 4, Min: 57, Sign: "Ari", Bracket: '('}, OtherAt: CyclePoint{Deg: 4, Min: 57, Sign: "Ari", Bracket: '['}}
	found := false
	for _, e := range events {
		if e.Instant.Equal(want.Instant) {
			found = true
			e.Line = ""
			if e != want {
				t.Errorf("2017-03-25: %+v, want %+v", e, want)
			}
		}
	}
	if !found {
		t.Error("no 2017-03-25 conjunction")
	}
	// The occultation suffix is read, not refused.
	occ, err := ParseCycleEvents("Mon  6/ 6/2016 21:49:20     Sun (16Gem35) Con (16Gem35) Venus (Total Solar Occultation 98%)\n")
	if err != nil || occ[0].Other != aspect.Venus || occ[0].OtherAt.Bracket != '(' || occ[0].Instant.Hour() != 21 {
		t.Errorf("occultation line: %+v, %v", occ, err)
	}
	// The clock is pinned to 24 hours: midnight is 0:00.
	mid, err := ParseCycleEvents("Thu  1/ 1/2026  0:00:05   Venus (10Cap00) --> Capricorn\nThu  1/ 1/2026 12:00:05   Venus <10Cap00> S/D\n")
	if err != nil || mid[0].Instant.Hour() != 0 || mid[1].Instant.Hour() != 12 || mid[1].Station != "S/D" {
		t.Errorf("24-hour clock: %+v, %v", mid, err)
	}
}

// Any line the grammar does not know refuses the whole result.
func TestParseCycleEventsRefusesUnknownLines(t *testing.T) {
	good := "Sat  3/25/2017 10:17:07     Sun ( 4Ari57) Con [ 4Ari57] Venus\n"
	for name, bad := range map[string]string{
		"another aspect":      "Sat  3/25/2017 10:17:07     Sun ( 4Ari57) Opp [ 4Lib57] Venus\n",
		"no seconds":          "Sat  3/25/2017 10:17     Sun ( 4Ari57) Con [ 4Ari57] Venus\n",
		"mismatched bracket":  "Sat  3/25/2017 10:17:07     Sun ( 4Ari57) Con [ 4Ari57) Venus\n",
		"unknown body":        "Sat  3/25/2017 10:17:07     Sun ( 4Ari57) Con [ 4Ari57] Vulcan\n",
		"impossible date":     "Sat  2/30/2017 10:17:07     Sun ( 4Ari57) Con [ 4Ari57] Venus\n",
		"degree 30":           "Sat  3/25/2017 10:17:07     Sun (30Ari57) Con [ 4Ari57] Venus\n",
		"trailing words":      "Sat  3/25/2017 10:17:07     Sun ( 4Ari57) Con [ 4Ari57] Venus and more\n",
		"a header":            "Astrolog 8.00 chart\n",
		"a station and more":  "Sat  3/ 4/2017  9:08:52   Venus <13Ari08> S/R twice\n",
		"an ingress and more": "Sat  3/ 4/2017  9:08:52   Venus (13Ari08) --> Taurus later\n",
		"a 12-hour time":      "Sat  3/25/2017 10:17:07am     Sun ( 4Ari57) Con [ 4Ari57] Venus\n",
	} {
		if _, err := ParseCycleEvents(good + bad); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}

// The call: -z 0 first, the year only, -dY and -b0, then the frame, the
// reform pin, the clock pins and the band of the Sun, the body and the
// conjunction.
func TestCycleCallArgv(t *testing.T) {
	call, err := CycleCall(CycleRequest{StartYear: 2016, Years: 13, Other: aspect.Venus})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(call.argv, " ")
	if !strings.HasPrefix(got, "-z 0 -qy 2016 -dY 13 -b0 ") || !strings.HasSuffix(got, " =Yt _Yd -R0 1 4 -RA0 1") {
		t.Errorf("argv %q", got)
	}
	for _, bad := range []CycleRequest{{StartYear: 1500, Years: 13, Other: aspect.Venus}, {StartYear: 2016, Years: 0, Other: aspect.Venus}, {StartYear: 2016, Years: 13, Other: aspect.Sun}} {
		if _, err := CycleCall(bad); err == nil {
			t.Errorf("%+v: no error", bad)
		}
	}
}

// The live binary prints what the parser reads.
func TestCycleSearchAgainstAstrolog(t *testing.T) {
	call, err := CycleCall(CycleRequest{StartYear: 2016, Years: 13, Other: aspect.Venus})
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := RunTimelineSearch(t.Context(), call)
	if err != nil {
		t.Fatal(err)
	}
	events, err := ParseCycleEvents(out)
	if err != nil {
		t.Fatal(err)
	}
	cons := 0
	for _, e := range events {
		if e.Kind == CycleConjunction {
			cons++
		}
	}
	if cons != 16 {
		t.Errorf("%d conjunctions in 2016-2028, want 16", cons)
	}
}

// The pair is read to the second of arc, from the live binary and from a
// sample; a third body or an unknown line refuses it.
func TestSunPairToTheSecond(t *testing.T) {
	sample := "Astrolog 8.00: Wed Oct 14, 2026 12:00:00 (ST Zone 0:00W)   0:00'00W 51:00'00N\n" +
		"Body  Location   Ret. Latitude Rul.      House  Rul. Velocity  Lat-Velocity\n\n" +
		"Sun : 21Lib09'41\"   - 0:00'00\" (f) [11th house] [d] +0.9907618 +0.0000085\n" +
		"Venu:  6Sco02'08\" R - 7:31'15\" (d) [12th house] [X] -0.4292777 +0.0379225\n\n\n\n" +
		"Body   Altitude    Azimuth    Azi. Vector    Magn. Full(Area,Time)  Diameter\n" +
		"Sun : +30:40'03\" 265:58'28\" (1.000s 0.070w) -26.87 100.00% 100.00%  0:32'04.086\"\n" +
		"Venu: +18:02'22\" 277:53'36\" (1.000s 0.139e)  -4.32   4.03%  12.87%  0:00'57.784\"\n"
	sun, ven, err := ParseSunPair(sample, aspect.Venus)
	if err != nil {
		t.Fatal(err)
	}
	if want := 180 + 21 + 9.0/60 + 41.0/3600; sun.Longitude != want || sun.Speed != 0.9907618 {
		t.Errorf("Sun %+v, want longitude %v", sun, want)
	}
	if want := 210 + 6 + 2.0/60 + 8.0/3600; ven.Longitude != want || ven.Speed != -0.4292777 {
		t.Errorf("Venus %+v, want longitude %v", ven, want)
	}
	if !ven.Retrograde || sun.Retrograde || ven.Sign != "Sco" || sun.Sign != "Lib" || ven.Magnitude != -4.32 || ven.LitPercent != 4.03 || sun.Magnitude != -26.87 {
		t.Errorf("columns: Sun %+v, Venus %+v", sun, ven)
	}
	for name, bad := range map[string]string{
		"no Venus":       strings.Split(sample, "Venu:")[0],
		"no horizon row": strings.Split(sample, "Venu: +18")[0],
		"no R column":    strings.Replace(sample, "6Sco02'08\" R - 7", "6Sco02'08\"R- 7", 1),
		"a third body":   sample + "Mars: 11Leo00'00\"   + 1:00'00\" (-) [ 8th house] [-] +0.5000000 +0.0000000\n",
		"Sun twice":      sample + "Sun : 21Lib09'41\"   - 0:00'00\" (f) [11th house] [d] +0.9907618 +0.0000085\n",
		"minutes only":   strings.Replace(sample, "21Lib09'41\"", "21Lib09", 1),
		"stray line":     sample + "  House cusp 1: 21Sag50\n",
	} {
		if _, _, err := ParseSunPair(bad, aspect.Venus); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
	// Below the horizon the altitude prints a space after its sign.
	night := strings.Replace(sample, "Sun : +30:40'03\" 265:58'28\" (1.000s 0.070w)", "Sun : - 3:03'28\" 190:55'25\" (0.193s 1.000w)", 1)
	if _, _, err := ParseSunPair(night, aspect.Venus); err != nil {
		t.Errorf("the Sun below the horizon: %v", err)
	}
	// And the live binary at midnight UT, the Sun far below the horizon.
	midnight, err := SunPairCall(SunPairRequest{When: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC), Lat: 51, Other: aspect.Venus})
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := RunTimelineSearch(t.Context(), midnight)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ParseSunPair(out, aspect.Venus); err != nil {
		t.Errorf("live at midnight: %v\n%s", err, out)
	}
	call, err := SunPairCall(SunPairRequest{When: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), Lat: 51, Other: aspect.Venus})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(call.argv, " "); !strings.HasPrefix(got, "-qj 2461328.0000000005 -zl 0:00E 51:00N -b0 -v -Z ") || !strings.HasSuffix(got, " =Yt _Yd -R0 1 4 -RA0 1") {
		t.Errorf("argv %q", got)
	}
	out, _, err = RunTimelineSearch(t.Context(), call)
	if err != nil {
		t.Fatal(err)
	}
	live, liveVen, err := ParseSunPair(out, aspect.Venus)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if live != sun || liveVen != ven {
		t.Errorf("live %+v %+v, sample %+v %+v", live, liveVen, sun, ven)
	}
}
