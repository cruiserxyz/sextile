package ui

// the specification: a chart before the Gregorian reform shows the date
// Astrolog gives it. C7 TestOldDatesShowAstrologsDate, C9
// TestEnterOnAnOldRowLandsOnItsMoment, C10 TestStepStopsAtJulianDayZero, N8
// TestCursorAcrossAJulianMonthMovesTheTimeline.

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// oldCase is a moment with the date Astrolog prints for it, measured on the
// binary (`astrolog -qj <jd> -zZ <zone> 0 -v`, 2026-10-11, Astrolog 8.00) and
// written here as literals. unix is the instant; the zone is the fixed zone of
// tz. Astrolog names a day before 15 Oct 1582 in the Julian calendar, and
// inside the 1582 window it is asked for no zone, so the row shows UT.
type oldCase struct {
	name         string
	unix         int64
	tz, zoneText string
	wd, mon      string
	day, year    int
	hour, minute int
	julian, ut   bool
}

var oldCases = []oldCase{
	{"1571", -12559271400, "Etc/GMT-1", "+01", "Thu", "Dec", 27, 1571, 2, 30, true, false},
	{"5 Oct 1582 (Julian)", -12219274800, "Etc/GMT-1", "+01", "Fri", "Oct", 5, 1582, 6, 0, true, false},
	{"the first Gregorian header", -12218359680, "Etc/GMT-1", "+01", "Mon", "Oct", 25, 1582, 20, 12, false, false},
	{"the window at +1", -12218430600, "Etc/GMT-1", "UT", "Sun", "Oct", 14, 1582, 23, 30, true, true},
	{"the window at +5", -12218437440, "Etc/GMT-5", "UT", "Sun", "Oct", 14, 1582, 21, 36, true, true},
	{"year 0", -62153010000, "Etc/GMT-1", "+01", "Tue", "Jun", 15, 0, 12, 0, true, false},
	{"2026", 1791223200, "Etc/GMT-1", "+01", "Mon", "Oct", 5, 2026, 19, 0, false, false},
}

func (c oldCase) place() location.PlaceInfo {
	return location.PlaceInfo{Name: "Oldtown", Country: "NL", Lat: 52.37, Lon: 4.9, TZ: c.tz}
}

func (c oldCase) at() time.Time { return time.Unix(c.unix, 0).UTC() }

// stamp spells the case the way the spec says a row reads under a clock form:
// the date, ` Jul` after it on the Julian branch, then the clock.
func (c oldCase) stamp(form clockCase) string {
	hour, suffix := c.hour, ""
	if form.hours == "12" {
		suffix = "am"
		if hour >= 12 {
			suffix = "pm"
		}
		hour %= 12
		if hour == 0 {
			hour = 12
		}
	}
	mark := ""
	if c.julian {
		mark = " Jul"
	}
	clock := fmt.Sprintf("%2d:%02d%s", hour, c.minute, suffix)
	if form.dates == "day-first" {
		return fmt.Sprintf("%s %2d %s %d%s %s", c.wd, c.day, c.mon, c.year, mark, clock)
	}
	return fmt.Sprintf("%s %s %2d, %d%s %s", c.wd, c.mon, c.day, c.year, mark, clock)
}

func (c oldCase) noMark(form clockCase) string {
	return strings.Replace(c.stamp(form), " Jul", "", 1)
}

const oldMomentRE = `[A-Z][a-z]{2} (?:[A-Z][a-z]{2} [ 0-9]\d, -?\d+|[ 0-9]\d [A-Z][a-z]{2} -?\d+)(?: Jul)? [ 0-9]\d:\d{2}(?:am|pm)?`

var reOldChartFor = regexp.MustCompile(`(?:chart|comparison) for (` + strings.ReplaceAll(oldMomentRE, "(?: Jul)?", "") + `) \(`)

func TestOldDatesShowAstrologsDate(t *testing.T) {
	needLiveAstrolog(t)
	cfg := Config{Accessible: true}
	for _, form := range clockCases {
		for _, c := range oldCases {
			t.Run(form.String()+"/"+c.name, func(t *testing.T) {
				holdForm(t, form)
				at, place := c.at(), c.place()
				want := c.stamp(form)

				// Every row path: the moment, the zone, and the whole row.
				if got := slotMoment(at, place); got != want {
					t.Errorf("slotMoment = %q, want %q", got, want)
				}
				if got := slotZone(at, place); got != c.zoneText {
					t.Errorf("slotZone = %q, want %q", got, c.zoneText)
				}
				for name, row := range map[string]string{
					"floating": tagSlotLine(at, place, "WhTr", 110, cfg),
					"named":    tagNamedSlotLine("Ann", 3, 4, at, place, "WhTr", 110, cfg),
					"one":      oneChartLine("Ann", 3, 4, at, place, "WhTr", 110, cfg),
				} {
					if !strings.Contains(row, want+" "+c.zoneText) {
						t.Errorf("%s row %q lacks %q", name, row, want+" "+c.zoneText)
					}
				}

				// The anchored old chart and the floating slot side by side on the
				// rendered screen: both rows read the same date.
				m := formModel(t, formCase{place, at})
				if err := addAnchor(m.slots, at, place); err != nil {
					t.Fatal(err)
				}
				view := plainView(m)
				if n := strings.Count(view, want+" "+c.zoneText); n < 2 {
					t.Errorf("the screen shows %d rows reading %q, want the anchored and the floating row:\n%s", n, want+" "+c.zoneText, view)
				}

				// The Details header is Astrolog's own: the row says what it says
				// (the 5-14 Oct 1582 rows and the window row included). Astrolog
				// refuses a chart for year 0 outright: Chiron's ephemeris starts at
				// JD 1967601.5 and the report warns instead of printing.
				if c.year < 100 {
					return
				}
				report, _, err := lookup.ComputeRawReport(context.Background(), at, place, m.viewLens(), lookup.RawReportRequest{Kind: lookup.RawReportChart})
				if err != nil {
					t.Fatalf("report: %v", err)
				}
				mm := m
				mm.focused = PanelChart
				mm = mm.adoptDetailsReport("[0] Chart", report)
				h := reOldChartFor.FindStringSubmatch(plainView(mm))
				if h == nil {
					t.Fatalf("no Details header in the screen")
				}
				if h[1] != c.noMark(form) {
					t.Errorf("Details header reads %q, the row %q: they must agree", h[1], c.noMark(form))
				}
			})
		}
	}

	// A 40-character name keeps the whole marked date visible at 80x24.
	t.Run("long-name-80x24", func(t *testing.T) {
		holdForm(t, clockCase{"24", "month-first"})
		c := oldCases[0]
		m := formModel(t, formCase{c.place(), c.at()})
		m.width, m.height = 80, 24
		if err := addAnchor(m.slots, c.at(), c.place()); err != nil {
			t.Fatal(err)
		}
		m.slots.All()[0].Chart.Name = "Alexandria Catherine of Aragon-Habsburg"
		if got := len([]rune(m.slots.All()[0].Chart.Name)); got != 39 && got != 40 {
			t.Fatalf("fixture name has %d characters", got)
		}
		view := plainView(m)
		if want := c.stamp(clockCase{"24", "month-first"}); !strings.Contains(view, want) {
			t.Errorf("the 80x24 screen lost the marked date %q:\n%s", want, view)
		}
	})

	// A timeline row keeps Astrolog's printed date and carries ` Jul` after it
	// when Parse read it on the Julian branch.
	t.Run("timeline-rows", func(t *testing.T) {
		defaultClock(t)
		rows := oldTimelineRows(t, time.Date(1500, 2, 20, 12, 0, 0, 0, time.UTC))
		out := renderedTimelineRows(rows)
		var feb29, others int
		for _, line := range out {
			if strings.Contains(line, "2/29/1500") {
				feb29++
				if !strings.Contains(line, "2/29/1500 Jul") {
					t.Errorf("the 29 February 1500 row lacks the Julian mark: %q", line)
				}
			}
			others++
		}
		if feb29 == 0 || others == 0 {
			t.Fatalf("no 29 February row in %q", out)
		}
		oct := renderedTimelineRows(oldTimelineRows(t, time.Date(1582, 10, 25, 12, 0, 0, 0, time.UTC)))
		var julian, gregorian int
		for _, line := range oct {
			switch {
			case strings.Contains(line, "10/ 4/1582"):
				julian++
				if !strings.Contains(line, "10/ 4/1582 Jul") {
					t.Errorf("a row before the reform lacks the mark: %q", line)
				}
			case strings.Contains(line, "10/15/1582"), strings.Contains(line, "10/16/1582"), strings.Contains(line, "10/31/1582"):
				gregorian++
				if strings.Contains(line, "Jul") {
					t.Errorf("a row from the 15th on carries the mark: %q", line)
				}
			}
		}
		if julian == 0 || gregorian == 0 {
			t.Fatalf("the October 1582 rows cover %d Julian and %d Gregorian rows: %q", julian, gregorian, oct)
		}
	})
}

// ---- old timelines ---------------------------------------------------------

// julianUnix is the Unix second of an hour on a Julian-calendar date, by
// Meeus's formula (AA 2e ch. 7, with B = 0), written out here and not taken
// from the code under test.
func julianUnix(year, month, day int, hour float64) int64 {
	y, m := float64(year), float64(month)
	if month <= 2 {
		y, m = y-1, m+12
	}
	jd := math.Floor(365.25*(y+4716)) + math.Floor(30.6001*(m+1)) + float64(day) - 1524.5 + hour/24
	return int64(math.Round((jd - 2440587.5) * 86400))
}

// julianDays is the length of a month in Astrolog's calendar before 1583.
func julianDays(year, month int) int {
	switch month {
	case 2:
		if year%4 == 0 {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

// oldSearch answers the -t and -tr searches with transit rows on the first, the
// 4th, the 15th and the last day of the month asked for, in Astrolog's own
// printed form (default clock), and the sky with nothing. The month comes from
// the argv, as it does from Astrolog.
func oldSearch(_ context.Context, call lookup.BodyCall) (string, string, error) {
	argv := call.Argv()
	cmd := "astrolog " + strings.Join(argv, " ")
	for i, a := range argv {
		if (a == "-t" || a == "-tr") && i+2 < len(argv) {
			var month, year int
			fmt.Sscan(argv[i+1], &month)
			fmt.Sscan(argv[i+2], &year)
			if a == "-tr" {
				return "", cmd, nil
			}
			var b strings.Builder
			for _, d := range []int{1, 4, 15, julianDays(year, month)} {
				if year == 1582 && month == 10 && d == 4 {
					fmt.Fprintf(&b, "%2d/%2d/%4d  6:46 trans    Moon (Ari) Opp natal (Lib) Mercury\n", month, 4, year)
					continue
				}
				if year == 1582 && month == 10 && d > 4 && d < 15 {
					continue
				}
				fmt.Fprintf(&b, "%2d/%2d/%4d  6:46 trans    Moon (Ari) Opp natal (Lib) Mercury\n", month, d, year)
			}
			return b.String(), cmd, nil
		}
	}
	return "", cmd, nil
}

// oldPlace is the floating slot's place in the old-timeline fixtures: a fixed
// zone, UTC+1, so no tzdata release can move a historical local mean time under
// the frames or the instants asserted.
func oldPlace() location.PlaceInfo {
	return location.PlaceInfo{Name: "Oldtown", Country: "NL", Lat: 52.37, Lon: 4.9, TZ: "Etc/GMT-1"}
}

// oldTimelineRows runs lookup's own merge over oldSearch for the month the
// instant lies in, with the anchored natal selected.
func oldTimelineRows(t *testing.T, when time.Time) []lookup.TimelineRow {
	t.Helper()
	res, err := lookup.TimelineWith(context.Background(),
		lookup.TimelineRequest{When: when, Place: oldPlace(), Natals: []lookup.TimelineNatal{{Numeral: 1, Chart: tlNatal()}}}, oldSearch, stubDefaults)
	if err != nil {
		t.Fatalf("old timeline: %v", err)
	}
	if len(res.Rows) == 0 {
		t.Fatal("old timeline: no rows")
	}
	return res.Rows
}

// renderedTimelineRows is the rows as the panel body draws them, ANSI-stripped.
func renderedTimelineRows(rows []lookup.TimelineRow) []string {
	body, _ := renderTimelineBody(rows, nil, -1, detailsViewState{}, timelineSearchView{}, 120, len(rows)+2, Config{Accessible: true})
	return strings.Split(ansi.Strip(body), "\n")
}

// oldTimelineModel is timelineModel with the floating slot at when and every
// search answered by oldSearch through lookup's own merge. It returns the model
// and the requests the searches were asked.
func oldTimelineModel(t *testing.T, when time.Time) (Model, *[]lookup.TimelineRequest) {
	t.Helper()
	defaultClock(t)
	calls := &[]lookup.TimelineRequest{}
	old, oldStubbed := computeTimeline, timelineStubbed
	computeTimeline = func(ctx context.Context, req lookup.TimelineRequest) (lookup.TimelineResult, error) {
		*calls = append(*calls, req)
		return lookup.TimelineWith(ctx, req, oldSearch, stubDefaults)
	}
	timelineStubbed = true
	t.Cleanup(func() { computeTimeline, timelineStubbed = old, oldStubbed })

	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTime()))
	slots, err := store.NewSlots(when, oldPlace())
	if err != nil {
		t.Fatal(err)
	}
	if err := addAnchor(slots, tlNatalWhen, testPlaceAmsterdam()); err != nil {
		t.Fatal(err)
	}
	m.slots = slots
	m.cursorIdx = len(slots.All()) - 1
	m.place = oldPlace()
	m.targetFloatingTime = slots.Floating().Time
	m.startup = startupReady
	m.focused = PanelChart
	m.width, m.height = 144, 40
	m = m.noteArrival()
	m, cmd := syncTimelineShown(m)
	m = pump(t, m, cmd)
	if !m.timeline.complete {
		t.Fatalf("the old timeline did not load: %+v", m.timeline.key)
	}
	return m, calls
}

// ---- C9 ----------------------------------------------------------------------

func TestEnterOnAnOldRowLandsOnItsMoment(t *testing.T) {
	stubTextCompute(t)
	// Julian 10 Feb 1500, noon UT.
	m, _ := oldTimelineModel(t, time.Unix(julianUnix(1500, 2, 10, 12), 0).UTC())
	m, _ = tpress(t, m, runes("0"))
	m, _ = tpress(t, m, keyEnter)
	if !m.details.attached || m.activeTabID(PanelDetails) != TabDetailsTimeline {
		t.Fatalf("fixture: attached=%v tab=%s", m.details.attached, m.activeTabID(PanelDetails))
	}
	if len(m.timeline.rows) == 0 {
		t.Fatalf("the old month's timeline has no rows (complete=%v, log %+v)", m.timeline.complete, m.logLines)
	}
	m = homedTimeline(t, m)
	last := len(m.timeline.rows) - 1
	for i := 0; i < last; i++ {
		m, _ = tpress(t, m, runes("j"))
	}
	row := m.timeline.rows[last]
	if !strings.Contains(row.Line, "2/29/1500") {
		t.Fatalf("fixture: the last row is %q, want 29 February 1500", row.Line)
	}
	// The row's instant is Julian 29 Feb 1500 6:46 at UTC+1.
	wantAt := julianUnix(1500, 2, 29, 0) + 6*3600 + 46*60 - 3600
	if got := row.Instant.Unix(); got != wantAt {
		t.Fatalf("the row's instant is Unix %d, want %d: the parser has not read the Julian date", got, wantAt)
	}

	text := stubTextCompute(t)
	m, cmd := tpress(t, m, keyEnter)
	m = pump(t, m, cmd)
	if got := m.slots.Floating().Time; !got.Equal(landedAt(row.Instant)) {
		t.Errorf("committed floating Time = %v, want the row's instant %v", got, landedAt(row.Instant))
	}
	asked := false
	for _, c := range *text {
		if c.kind == "bodies" && c.when.Equal(landedAt(row.Instant)) {
			asked = true
		}
	}
	if !asked {
		t.Errorf("no chart call for %v after Enter: %+v", landedAt(row.Instant), *text)
	}
	if got := m.nearestRow(m.timelineShown()); got != last {
		t.Errorf("the nearest-row bold is on row %d, want the row Enter landed on (%d)", got, last)
	}
}

// ---- N8 ----------------------------------------------------------------------

func TestCursorAcrossAJulianMonthMovesTheTimeline(t *testing.T) {
	stubTextCompute(t)
	// Julian 29 Feb 1500 noon UT: Go's calendar calls it 10 March.
	m, calls := oldTimelineModel(t, time.Unix(julianUnix(1500, 2, 29, 12), 0).UTC())
	if got := m.timeline.key; got.year != 1500 || got.month != time.February {
		t.Fatalf("the timeline is keyed %d-%02d, want 1500-02 (Julian 29 February)", got.year, int(got.month))
	}
	first := len(*calls)
	m, cmd := tpress(t, m, runes("d")) // one day on: Julian 1 March
	m = pump(t, m, cmd)
	if got := m.timeline.key; got.year != 1500 || got.month != time.March {
		t.Fatalf("after a day the timeline is keyed %d-%02d, want 1500-03: the old month is still shown", got.year, int(got.month))
	}
	if len(*calls) != first+1 {
		t.Errorf("the step across the month boundary asked %d searches, want 1", len(*calls)-first)
	}
	if rows := m.timeline.rows; len(rows) == 0 || !strings.HasPrefix(strings.TrimSpace(rows[0].Date), "3/") {
		t.Errorf("the rows after the step are not March's: %+v", rows)
	}
}

// ---- C10 ---------------------------------------------------------------------

// astrologCalls puts an `astrolog` on PATH that counts its starts, then runs the
// real one; the returned func is the count so far.
func astrologCalls(t *testing.T) func() int {
	t.Helper()
	real, err := exec.LookPath("astrolog")
	if err != nil {
		t.Skip("astrolog not on PATH")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "starts")
	script := "#!/bin/sh\necho x >> '" + log + "'\nexec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "astrolog"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() int {
		b, _ := os.ReadFile(log)
		return strings.Count(string(b), "x")
	}
}

// jdZero is Julian Day 0 as a Unix second.
const jdZero = -210866760000

func TestStepStopsAtJulianDayZero(t *testing.T) {
	defaultClock(t)
	started := astrologCalls(t)
	startWith := func(sec int64) Model {
		m := timelineModel(t, Config{Accessible: true})
		slots, err := store.NewSlots(time.Unix(sec, 0).UTC(), testPlaceAmsterdam())
		if err != nil {
			t.Fatal(err)
		}
		m.slots = slots
		m.cursorIdx = 0
		m.targetFloatingTime = slots.Floating().Time
		return m
	}

	// A step across: the floating time stays, one plain line says why, and no
	// Astrolog call starts.
	for _, key := range []string{"M", "H", "D", "W", "O", "Y"} {
		m := startWith(jdZero + 30)
		before, lines := m.targetFloatingTime, len(m.logLines)
		m, cmd := tpress(t, m, runes(key))
		m = pump(t, m, cmd)
		if !m.targetFloatingTime.Equal(before) || !m.slots.Floating().Time.Equal(before) {
			t.Errorf("%s: the floating time moved from %v to target %v, committed %v", key, before, m.targetFloatingTime, m.slots.Floating().Time)
		}
		if got := len(m.logLines) - lines; got != 1 {
			t.Errorf("%s: %d lines logged, want one", key, got)
		}
	}
	if n := started(); n != 0 {
		t.Errorf("%d Astrolog calls started for refused steps, want none", n)
	}

	// The control: a step that stays at or after JD 0 moves as ever.
	m := startWith(jdZero + 3600)
	m, cmd := tpress(t, m, runes("M"))
	if want := time.Unix(jdZero+3600-60, 0).UTC(); !m.targetFloatingTime.Equal(want) {
		t.Errorf("a step that stays after JD 0 went to %v, want %v", m.targetFloatingTime, want)
	}
	_ = cmd

	// A floating moment restored from the session before JD 0 starts no call:
	// the text, the wheel and the timeline all stay quiet.
	before := started()
	m = startWith(jdZero - 86400)
	m.focused = PanelChart
	m.width, m.height = 144, 40
	m, textCmd := m.noteWant(surfaceText, true)
	m = pump(t, m, textCmd)
	m.previewActive = true
	m, imgCmd := m.requestImage()
	m = pump(t, m, imgCmd)
	old := computeTimeline
	computeTimeline = lookup.Timeline
	t.Cleanup(func() { computeTimeline = old })
	m, tlCmd := syncTimelineVisited(m)
	_ = pump(t, m, tlCmd)
	if n := started() - before; n != 0 {
		t.Errorf("%d Astrolog calls started for a moment before Julian Day 0, want none", n)
	}
}

// ---- frames ------------------------------------------------------------------

// The frames of the specification under the default form: a 1571 chart row at
// 80x24; the same chart's Details report beside its row, and the February 1500
// timeline with its 29th, at 132x40.
func TestOldDatesFrames(t *testing.T) {
	needLiveAstrolog(t)
	holdForm(t, clockCase{"24", "month-first"})
	cfg := Config{Color: ColorNever}
	c := oldCases[0]
	place := c.place()
	floating := time.Date(2026, 10, 5, 19, 0, 0, 0, time.UTC)

	build := func(w, h int) Model {
		m := formModel(t, formCase{place, floating})
		m.cfg = cfg
		if err := addAnchor(m.slots, c.at(), place); err != nil {
			t.Fatal(err)
		}
		m.slots.All()[0].Chart.Name = "Synthetic Aster"
		m.cursorIdx = 0
		m.focused = PanelChart
		m.width, m.height = w, h
		return m
	}
	writeDesignFrame(t, "old-dates-80x24", build(80, 24).View())

	m := build(144, 40)
	report, _, err := lookup.ComputeRawReport(context.Background(), c.at(), place, m.viewLens(), lookup.RawReportRequest{Kind: lookup.RawReportChart})
	if err != nil {
		t.Fatal(err)
	}
	m = m.adoptDetailsReport("[0] Chart", report)
	writeDesignFrame(t, "old-dates-144x40-details", m.View())

	tl, _ := oldTimelineModel(t, time.Unix(julianUnix(1500, 2, 10, 12), 0).UTC())
	tl.cfg = cfg
	tl, _ = tpress(t, tl, runes("0"))
	tl, _ = tpress(t, tl, keyEnter)
	writeDesignFrame(t, "old-dates-144x40-timeline", tl.View())
}
