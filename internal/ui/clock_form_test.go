package ui

// the specification: every moment Sextile prints itself (the chart rows and
// the chart picker) reads the characters Astrolog prints under the operator's
// chosen form, and the rendered screen agrees with itself.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

func (c clockCase) pref() lookup.ClockPref {
	return lookup.ClockPref{Hours12: c.hours == "12", DayFirst: c.dates == "day-first"}
}

// holdForm holds c for the test, the way a cold start does, without a file.
func holdForm(t *testing.T, c clockCase) {
	t.Helper()
	lookup.HoldClock(c.pref())
	t.Cleanup(func() { lookup.HoldClock(lookup.ClockPref{}) })
}

// refMoment spells t the way Astrolog prints a chart-header moment: weekday,
// date, clock, the day and the hour space-padded to two cells. It is written
// from the four measured forms in the specification, not from the code
// under test.
func refMoment(t time.Time, c clockCase) string {
	hour, suffix := t.Hour(), ""
	if c.hours == "12" {
		suffix = "am"
		if hour >= 12 {
			suffix = "pm"
		}
		hour %= 12
		if hour == 0 {
			hour = 12
		}
	}
	clock := fmt.Sprintf("%2d:%02d%s", hour, t.Minute(), suffix)
	wd, mon := t.Weekday().String()[:3], t.Month().String()[:3]
	if c.dates == "day-first" {
		return fmt.Sprintf("%s %2d %s %d %s", wd, t.Day(), mon, t.Year(), clock)
	}
	return fmt.Sprintf("%s %s %2d, %d %s", wd, mon, t.Day(), t.Year(), clock)
}

// momentRE matches one chart-row or report-header moment in any of the four
// forms: weekday, date, clock.
const momentRE = `[A-Z][a-z]{2} (?:[A-Z][a-z]{2} [ 0-9]\d, \d+|[ 0-9]\d [A-Z][a-z]{2} \d+) [ 0-9]\d:\d{2}(?:am|pm)?`

// Acceptance 1: both chart-row paths print the chosen form exactly as Astrolog
// does, under all four forms; days 5 and 15, hours 0, 9, 12 and 13.
func TestSlotMomentMatchesAstrologsClock(t *testing.T) {
	berlin := testPlaceBerlin()
	loc, err := time.LoadLocation(berlin.TZ)
	if err != nil {
		t.Fatal(err)
	}

	// The spec's own examples, literally: Mon 5 Oct 2026 at 9:05 CEST.
	literal := map[clockCase]string{
		{"24", "month-first"}: "Mon Oct  5, 2026  9:05",
		{"12", "month-first"}: "Mon Oct  5, 2026  9:05am",
		{"24", "day-first"}:   "Mon  5 Oct 2026  9:05",
		{"12", "day-first"}:   "Mon  5 Oct 2026  9:05am",
	}
	// Midnight and noon in the 12-hour form.
	midnightNoon := map[string]string{
		"midnight": "Mon Oct  5, 2026 12:05am",
		"noon":     "Mon Oct  5, 2026 12:05pm",
	}

	for _, c := range clockCases {
		t.Run(c.String(), func(t *testing.T) {
			holdForm(t, c)
			rows := func(at time.Time) map[string]string {
				return map[string]string{
					"floating": tagSlotLine(at.UTC(), berlin, "WhTr", 100, Config{Accessible: true}),
					"named":    tagNamedSlotLine("Ann", 3, 4, at.UTC(), berlin, "WhTr", 110, Config{Accessible: true}),
					"moment":   slotMoment(at.UTC(), berlin) + " CEST",
				}
			}
			for _, day := range []int{5, 15} {
				for _, hour := range []int{0, 9, 12, 13} {
					at := time.Date(2026, 10, day, hour, 5, 0, 0, loc)
					want := refMoment(at, c) + " CEST"
					for name, got := range rows(at) {
						if !strings.Contains(got, want) {
							t.Errorf("%s row, day %d hour %d: %q lacks %q", name, day, hour, got, want)
						}
					}
				}
			}
			at := time.Date(2026, 10, 5, 9, 5, 0, 0, loc)
			for name, got := range rows(at) {
				if !strings.Contains(got, literal[c]+" CEST") {
					t.Errorf("%s row: %q lacks the spec's literal %q", name, got, literal[c])
				}
			}
			if c.hours == "12" && c.dates == "month-first" {
				for name, want := range midnightNoon {
					hour := map[string]int{"midnight": 0, "noon": 12}[name]
					got := tagSlotLine(time.Date(2026, 10, 5, hour, 5, 0, 0, loc).UTC(), berlin, "", 100, Config{Accessible: true})
					if !strings.Contains(got, want+" CEST") {
						t.Errorf("%s: %q lacks %q", name, got, want)
					}
				}
			}
		})
	}
}

// Acceptance 2: the picker's date is `Oct  5, 2026` / ` 5 Oct 2026`, no weekday
// or clock, spelled field by field so a Julian date keeps what the file says;
// in a narrow row the place is shortened before the date.
func TestPickerDateKeepsTheFilesCivilDate(t *testing.T) {
	mk := func(y, mo, d int) store.NamedChart {
		c := testNamedChart("Constructed One", fixtureOneP)
		c.Year, c.Month, c.Day = y, mo, d
		return c
	}
	cases := []struct {
		name       string
		c          store.NamedChart
		month, day string
	}{
		{"single digit day", mk(2026, 10, 5), "Oct  5, 2026", " 5 Oct 2026"},
		{"two digit day", mk(1995, 3, 20), "Mar 20, 1995", "20 Mar 1995"},
		{"a Julian leap day", mk(1500, 2, 29), "Feb 29, 1500", "29 Feb 1500"},
		{"no date", mk(1995, 0, 0), "", ""},
	}
	for _, form := range []clockCase{{"24", "month-first"}, {"12", "day-first"}} {
		for _, tc := range cases {
			t.Run(form.String()+"/"+tc.name, func(t *testing.T) {
				holdForm(t, form)
				want := tc.month
				if form.dates == "day-first" {
					want = tc.day
				}
				if got := chartDate(tc.c); got != want {
					t.Errorf("chartDate = %q, want %q", got, want)
				}
			})
		}
	}

	t.Run("a narrow row shortens the place before the date", func(t *testing.T) {
		holdForm(t, clockCase{"24", "month-first"})
		c := mk(2026, 10, 5)
		c.Place.Name, c.Place.Country = "Llanfairpwllgwyngyllgogerychwyrndrobwll", "GB"
		got := listRow(chartRow(c, false, time.Time{}), plainCfg, 54)
		if !strings.HasSuffix(got, "Oct  5, 2026") {
			t.Errorf("the date was cut before the place: %q", got)
		}
		if strings.Contains(got, "Llanfairpwllgwyngyllgogerychwyrndrobwll, GB") {
			t.Errorf("the place was not shortened in a narrow row: %q", got)
		}
	})
}

// Acceptance 8: no golden under internal/ui/testdata holds an Astrolog clock
// in am/pm form or a day-first date, except goldens named for a 12-hour or
// day-first case.
func TestNoAmPmClockInGoldens(t *testing.T) {
	var (
		reAmPm      = regexp.MustCompile(`\b\d{1,2}:\d{2}(?:am|pm)\b`)
		reDayFirstA = regexp.MustCompile(`\b[ 0-9]\d-\d{1,2}-\d{4}\b`)
		reDayFirstS = regexp.MustCompile(`\b[ 0-9]\d (?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec) \d{4}\b`)
		allowed     = regexp.MustCompile(`12h|12-hour|day-first|dayfirst`)
	)
	files, err := filepath.Glob(filepath.Join("testdata", "*.golden"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no goldens found: %v", err)
	}
	for _, f := range files {
		if allowed.MatchString(filepath.Base(f)) {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		text := ansi.Strip(string(raw))
		for _, re := range []*regexp.Regexp{reAmPm, reDayFirstA, reDayFirstS} {
			if hit := re.FindString(text); hit != "" {
				t.Errorf("%s holds %q: re-record it under the default clock (24-hour, month first)", filepath.Base(f), hit)
				break
			}
		}
	}
}
