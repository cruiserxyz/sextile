package astrolog

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// Before its zone's standard time, Now is on the local mean time of its own
// longitude, as Astrolog has it, and Astrolog is told so: its header prints the
// clock Sextile's row shows, labelled LMT. Hamburg lies 9°59' east, so its LMT
// is 39 min 56 s ahead of UT, where Go's Europe/Berlin LMT is Berlin's 53:28.
func TestNowBeforeStandardTimeIsItsOwnLMT(t *testing.T) {
	needAstrolog(t)
	const lon, lat = 9.9833, 53.55
	instant := time.Date(1852, 6, 1, 0, 0, 0, 0, time.UTC)
	zone, err := NowZone(instant, "Europe/Berlin", lon)
	if err != nil {
		t.Fatal(err)
	}
	if name, off := instant.In(zone).Zone(); name != "LMT" || off != 599*4 {
		t.Fatalf("zone %s %+ds, want LMT +2396s (9°59' over 15)", name, off)
	}
	if later, _ := NowZone(time.Date(1900, 6, 1, 0, 0, 0, 0, time.UTC), "Europe/Berlin", lon); later.String() != "Europe/Berlin" {
		t.Errorf("after standard time the zone is %s, want Europe/Berlin", later)
	}
	spec, err := recipe.NewChartSpec(recipe.MomentAtPlace{JD: SlotJD(instant), Lon: lon, Lat: lat, TZ: "Europe/Berlin"},
		recipe.Placidus, recipe.Tropical, recipe.ObjectPolicy{Baseline: recipe.AstrologDefault})
	if err != nil {
		t.Fatal(err)
	}
	argv, _, _, err := emitPrefix(spec, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(argv, " "), "-zZ LMT 0") {
		t.Errorf("argv %q does not send Astrolog's LMT", argv)
	}
	out, _, err := invoke(context.Background(), append(argv, "-v"))
	if err != nil {
		t.Fatal(err)
	}
	want := SlotLabel(instant, zone)
	if head := strings.SplitN(out, "\n", 2)[0]; !strings.Contains(head, " 0:39 (ST Zone LMT)") || want.Hour != 0 || want.Minute != 39 {
		t.Errorf("Astrolog's header %q, the row's clock %02d:%02d", head, want.Hour, want.Minute)
	}
}

// Stepping by months or years moves the date Astrolog names (R13): before the
// reform a month from Julian 25 January 1572 is Julian 25 February, where Go's
// proleptic calendar lands two days short. A day step across 4 October 1582
// lands on 15 October, as Astrolog's searches step.
func TestStepFollowsAstrologsCalendar(t *testing.T) {
	cal := AstrologCalendar{}
	from, err := cal.Parse(Fields{Year: 1572, Month: 1, Day: 25, Hour: 12}, 0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := cal.Step(from.Instant, time.UTC, 0, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := cal.Parse(Fields{Year: 1572, Month: 2, Day: 25, Hour: 12}, 0); !got.Equal(want.Instant) {
		t.Errorf("a month from Julian 25 Jan 1572 is %v, want Julian 25 Feb (%v)", got, want.Instant)
	}
	oct4, _ := cal.Parse(Fields{Year: 1582, Month: 10, Day: 4, Hour: 9, Minute: 30}, 0)
	next, err := cal.Step(oct4.Instant, time.UTC, 0, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := cal.Parse(Fields{Year: 1582, Month: 10, Day: 15, Hour: 9, Minute: 30}, 0); !next.Equal(want.Instant) {
		t.Errorf("a day from 4 Oct 1582 is %v, want 15 Oct (%v)", next, want.Instant)
	}
	berlin, _ := time.LoadLocation("Europe/Berlin")
	summer := time.Date(2026, 4, 25, 14, 0, 7, 0, berlin)
	back, err := cal.Step(summer, berlin, 0, -1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 3, 25, 14, 0, 7, 0, berlin); !back.Equal(want) {
		t.Errorf("a month back from 25 Apr 14:00:07 CEST is %v, want the same clock on 25 Mar (%v)", back, want)
	}
}
