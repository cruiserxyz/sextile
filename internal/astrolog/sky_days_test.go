package astrolog

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

// The sky is read a day at a time: Astrolog's month search drops the late
// events of a day once a month averages more than 75 events a day (Astrolog_8.0
// NEXT.md BUG 8; operator, 2026-11-06: the planets' moons ticked, the sky list
// refused). Here the void search holds the Moon row with its mark and a day
// search has a late event of another body: both are shown, the Moon row from
// the void search, which alone prints the mark across midnight.
func TestSkyReadsEachDayAndTakesTheVoidSearchsMarks(t *testing.T) {
	withClock(t, Clock{Hours: Hours12, Dates: MonthFirst})
	const (
		bare   = "Sat 10/ 3/2026  1:10am    Moon (Leo) Sex [Lib] Venus"
		marked = bare + " (v/c 8:17)"
		late   = "Sat 10/ 3/2026 11:50pm      Io (Leo) Con (Leo) Europa"
	)
	search := func(_ context.Context, call BodyCall) (string, string, error) {
		argv := call.Argv()
		command := "astrolog " + strings.Join(argv, " ")
		switch {
		case slices.Contains(argv, "-dY"):
			return marked + "\nSun 10/ 4/2026  9:27am    Moon (Leo) --> Virgo\n", command, nil
		case slices.Contains(argv, "-d") && argv[slices.Index(argv, "-qd")+2] == "3":
			return bare + "\n" + late + "\n", command, nil
		}
		return "", command, nil
	}
	rows, _, err := ComputeTimeline(context.Background(),
		TimelineRequest{Year: 2026, Month: time.October, Zone: time.UTC, Registry: testRegistry(t), AspectRegistry: testAspects(t)}, []TimelineSource{TimelineSky}, search)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Line != marked || rows[1].Line != late {
		t.Fatalf("rows =\n%s want the marked Moon row and the late Io row", dumpRows(rows))
	}
	if !strings.Contains(rows[0].Command, " -dY ") || !strings.Contains(rows[1].Command, " -qd 10 3 2026 -d ") {
		t.Errorf("commands %q and %q, want the void search's and the 3rd's day search's", rows[0].Command, rows[1].Command)
	}
}

// The days are Astrolog's: October 1582 steps from the 4th to the 15th, as its
// searches do (AddDay), and each day search names its date in that calendar.
func TestSkyDaysFollowAstrologsCalendar(t *testing.T) {
	segs := TimelineSegments(1582, time.October, time.UTC)
	if len(segs) != 1 {
		t.Fatalf("segments = %v", segs)
	}
	var got []int
	for _, day := range skyDays(segs[0]) {
		y, m, d := AstrologCalendar{}.DateOf(day.Start, day.Offset)
		if y != 1582 || m != time.October {
			t.Fatalf("a day of October 1582 is %d-%02d-%02d", y, m, d)
		}
		got = append(got, d)
	}
	want := []int{1, 2, 3, 4}
	for d := 15; d <= 31; d++ {
		want = append(want, d)
	}
	if !slices.Equal(got, want) {
		t.Errorf("days = %v, want %v", got, want)
	}
}
