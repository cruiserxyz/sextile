package astrolog

// the specification C3, N3, N8, N15, N21: the timeline parser reads the four
// row forms of the held clock preference, and only that form.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// rowIn writes one -dm row for instant t (already in the search's zone) in the
// form clock c prints, the way Astrolog spaces it.
func rowIn(c Clock, t time.Time, tail string) string {
	var date string
	if c.Dates == DayFirst {
		date = fmt.Sprintf("%2d-%d-%d", t.Day(), int(t.Month()), t.Year())
	} else {
		date = fmt.Sprintf("%2d/%2d/%d", int(t.Month()), t.Day(), t.Year())
	}
	var clock string
	if c.Hours == Hours12 {
		h, ap := t.Hour()%12, "am"
		if h == 0 {
			h = 12
		}
		if t.Hour() >= 12 {
			ap = "pm"
		}
		clock = fmt.Sprintf("%2d:%02d%s", h, t.Minute(), ap)
	} else {
		clock = fmt.Sprintf("%2d:%02d", t.Hour(), t.Minute())
	}
	return t.Weekday().String()[:3] + " " + date + " " + clock + "    " + tail
}

const skyTail = "Moon (Gem) --> Cancer"

// N3 / N15: the widened grammar keeps the clock's bounds.
func TestTimelineClockBounds(t *testing.T) {
	ok := func(c Clock, line string, h, m int) {
		t.Helper()
		rows, err := ParseTimeline(line+"\n", time.UTC)
		if err != nil || len(rows) != 1 {
			t.Errorf("[%s] %q refused: %v", clockName(c), line, err)
			return
		}
		if rows[0].Instant.Hour() != h || rows[0].Instant.Minute() != m {
			t.Errorf("[%s] %q read as %v, want %d:%02d", clockName(c), line, rows[0].Instant, h, m)
		}
	}
	bad := func(c Clock, line string) {
		t.Helper()
		_, err := ParseTimeline(line+"\n", time.UTC)
		var le *TimelineLineError
		if !errors.As(err, &le) {
			t.Errorf("[%s] %q: err = %v, want a timeline-line error", clockName(c), line, err)
		}
	}
	for _, c := range allClocks {
		withClock(t, c)
		d := "10/ 1/2026"
		if c.Dates == DayFirst {
			d = " 1-10-2026"
		}
		wd := "Thu "
		if c.Hours == Hours24 {
			ok(c, wd+d+"  0:40    "+skyTail, 0, 40)
			ok(c, wd+d+" 23:59    "+skyTail, 23, 59)
			ok(c, wd+d+" 12:00    "+skyTail, 12, 0)
			bad(c, wd+d+" 24:00    "+skyTail)
			bad(c, wd+d+" 12:60    "+skyTail)
			bad(c, wd+d+"  9:05am    "+skyTail) // the other hour form
		} else {
			ok(c, wd+d+" 12:40am    "+skyTail, 0, 40)
			ok(c, wd+d+" 12:05pm    "+skyTail, 12, 5)
			ok(c, wd+d+" 11:59pm    "+skyTail, 23, 59)
			bad(c, wd+d+"  0:05am    "+skyTail)
			bad(c, wd+d+" 13:05pm    "+skyTail)
			bad(c, wd+d+" 12:60pm    "+skyTail)
			bad(c, wd+d+" 23:59    "+skyTail) // the other hour form
		}
	}
}

// stubSearch answers every search with the whole month's events at the call's
// own fixed offset, as Astrolog does, in the held clock's form.
func stubSearch(events []time.Time) TimelineSearcher {
	offsets := map[string]int{}
	for _, o := range []int{3600, 7200} {
		offsets[formatSearchZone(o)] = o
	}
	return func(ctx context.Context, call BodyCall) (string, string, error) {
		argv := call.Argv()
		off, ok := offsets[argv[slices.Index(argv, "-z")+1]]
		if !ok {
			return "", "", fmt.Errorf("stub: unknown zone in %q", argv)
		}
		z := time.FixedZone("", off)
		var b strings.Builder
		for _, e := range events {
			b.WriteString(rowIn(CurrentClock(), e.In(z), skyTail) + "\n")
		}
		return b.String(), "stub " + strings.Join(argv, " "), nil
	}
}

func amsterdamRequest(t *testing.T, year int, month time.Month) TimelineRequest {
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	return TimelineRequest{Year: year, Month: month, Zone: loc}
}

func instantsOf(rows []TimelineRow) []time.Time {
	var out []time.Time
	for _, r := range rows {
		out = append(out, r.Instant.UTC())
	}
	slices.SortFunc(out, func(a, b time.Time) int { return a.Compare(b) })
	return out
}

func equalInstants(a, b []time.Time) bool {
	return slices.EqualFunc(a, b, func(x, y time.Time) bool { return x.Equal(y) })
}

// N8: the autumn repeated hour keeps both instants under every form.
func TestTimelineRepeatedHourInEveryForm(t *testing.T) {
	events := []time.Time{
		time.Date(2026, 10, 24, 22, 10, 0, 0, time.UTC),
		time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC), // 02:30 CEST
		time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC), // 02:30 CET
		time.Date(2026, 10, 26, 9, 45, 0, 0, time.UTC),
	}
	for _, c := range allClocks {
		t.Run(clockName(c), func(t *testing.T) {
			withClock(t, c)
			rows, _, err := ComputeTimeline(context.Background(), amsterdamRequest(t, 2026, time.October), []TimelineSource{TimelineSky}, stubSearch(events))
			if err != nil {
				t.Fatal(err)
			}
			if got := instantsOf(rows); !equalInstants(got, events) {
				t.Errorf("instants = %v, want %v", got, events)
			}
		})
	}
}

// N21: a search across the spring clock jump loses or misdates no row.
func TestTimelineSpringTransitionInEveryForm(t *testing.T) {
	events := []time.Time{
		time.Date(2026, 3, 29, 0, 59, 0, 0, time.UTC), // 01:59 CET
		time.Date(2026, 3, 29, 1, 0, 0, 0, time.UTC),  // 03:00 CEST
		time.Date(2026, 3, 29, 1, 5, 0, 0, time.UTC),
		time.Date(2026, 3, 30, 23, 30, 0, 0, time.UTC),
	}
	for _, c := range allClocks {
		t.Run(clockName(c), func(t *testing.T) {
			withClock(t, c)
			rows, _, err := ComputeTimeline(context.Background(), amsterdamRequest(t, 2026, time.March), []TimelineSource{TimelineSky}, stubSearch(events))
			if err != nil {
				t.Fatal(err)
			}
			if got := instantsOf(rows); !equalInstants(got, events) {
				t.Errorf("instants = %v, want %v", got, events)
			}
		})
	}
}

// C3 (live): the same searches resolve the same instants under every
// combination, each parses rows, and a row in any other form is refused.
func TestTimelineRowsKeepTheirInstantsInEveryForm(t *testing.T) {
	needAstrolog(t)
	natal := &TimelineNatal{When: time.Date(1990, 5, 1, 12, 0, 0, 0, time.UTC), Lon: 4.9, Lat: 52.37}
	req := amsterdamRequest(t, 2026, time.October)
	req.Natal = natal
	srcs := []TimelineSource{TimelineSky, TimelineTransit, TimelineReturn}

	type run struct {
		instants map[TimelineSource][]time.Time
		lines    []string
	}
	runs := map[Clock]run{}
	for _, c := range allClocks {
		withClock(t, c)
		r := run{instants: map[TimelineSource][]time.Time{}}
		for _, src := range srcs {
			rows, _, err := ComputeTimeline(context.Background(), req, []TimelineSource{src}, nil)
			if err != nil {
				t.Fatalf("[%s] %s: %v", clockName(c), src, err)
			}
			if src != TimelineReturn && len(rows) == 0 {
				t.Fatalf("[%s] %s parsed no rows", clockName(c), src)
			}
			r.instants[src] = instantsOf(rows)
			for _, row := range rows {
				r.lines = append(r.lines, row.Line)
			}
		}
		runs[c] = r
	}
	base := runs[allClocks[0]]
	for _, c := range allClocks[1:] {
		for _, src := range srcs {
			if !equalInstants(runs[c].instants[src], base.instants[src]) {
				t.Errorf("[%s] %s instants differ from [%s]", clockName(c), src, clockName(allClocks[0]))
			}
		}
	}
	// Each row's own text is in the form of its preference (the date and time
	// columns are what the Timeline shows), and every other form's row is refused.
	for _, c := range allClocks {
		for _, other := range allClocks {
			if other == c {
				continue
			}
			withClock(t, c)
			line := runs[other].lines[0]
			_, err := ParseTimeline(line+"\n", req.Zone)
			var le *TimelineLineError
			if !errors.As(err, &le) {
				t.Errorf("[%s] parsed %q from [%s]: err = %v, want a timeline-line error", clockName(c), line, clockName(other), err)
			}
		}
	}
}
