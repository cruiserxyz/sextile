package astrolog

// the specification C9, N13, N17: the recorded Astrolog text holds the
// default clock (24-hour, month first), and re-recording a chart file kept
// every value the file stood for.

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var (
	reAmPmClock  = regexp.MustCompile(`[0-9]{1,2}:[0-9]{2}(:[0-9]{2})?(am|pm)`)
	reDayFirst   = regexp.MustCompile(`(^|[^0-9])[0-9]{1,2}-[0-9]{1,2}-[0-9]{4}`)
	reHeaderLine = regexp.MustCompile(`(chart|comparison) for `)
	reHeaderForm = regexp.MustCompile(`(chart|comparison) for [A-Z][a-z]{2} [A-Z][a-z]{2} [ 0-9]{2}, -?[0-9]+ [ 0-9]{1,2}:[0-9]{2} \(`)
)

// A fixture that exists to test a 12-hour INPUT form says so in its name
// (12h or ampm) and is exempt; none exists today.
func exemptTwelveHour(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "12h") || strings.Contains(n, "ampm")
}

func TestNoAmPmClockInFixtures(t *testing.T) {
	checked := 0
	err := filepath.WalkDir("testdata", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Base(path) == "README.md" || exemptTwelveHour(filepath.Base(path)) {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		checked++
		for i, line := range strings.Split(string(b), "\n") {
			if reAmPmClock.MatchString(line) {
				t.Errorf("%s:%d holds an am/pm clock: %q", path, i+1, line)
			}
			if reDayFirst.MatchString(line) {
				t.Errorf("%s:%d holds a day-first date: %q", path, i+1, line)
			}
			if reHeaderLine.MatchString(line) && !reHeaderForm.MatchString(line) {
				t.Errorf("%s:%d header is not month-first with a 24-hour clock: %q", path, i+1, line)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 15 {
		t.Fatalf("only %d fixture files checked", checked)
	}
}

// chartRecord is what each chart file stood for BEFORE it was re-recorded in
// the 24-hour form, read from the 12-hour originals on 2026-10-10.
var chartRecords = []struct {
	file             string
	when             time.Time
	jd               float64
	lon, lat         float64
	name, place      string
	houses           int
	hasHouses        bool
	year, month, day int
}{
	{"Eclipse-2026-08-12.as", time.Date(2026, 8, 12, 17, 37, 0, 0, time.UTC), 2.46126523403e+06, 8.716666666666667, 52.85, "", "Bassum, Germany", 0, false, 2026, 8, 12},
	{"Equinox-2026-03.as", time.Date(2026, 3, 20, 14, 46, 0, 0, time.UTC), 2.46112011528e+06, 11.566666666666666, 48.13333333333333, "", "Munich, Germany", 0, true, 2026, 3, 20},
	{"Equinox-2026-09.as", time.Date(2026, 9, 23, 0, 6, 0, 0, time.UTC), 2.46130650417e+06, 8.8, 53.06666666666667, "", "Bremen, Germany", 0, false, 2026, 9, 23},
	{"FullMoon-2026-02.as", time.Date(2026, 2, 1, 22, 9, 0, 0, time.UTC), 2.46107342292e+06, 9.016666666666667, 53, "", "", 14, true, 2026, 2, 1},
	{"GreatConjunction-2020-12-21.as", time.Date(2020, 12, 21, 18, 19, 0, 0, time.UTC), 2.45920526319e+06, 8.666666666666666, 50.1, "Great Conjunction", "Frankfurt, Germany", 2, true, 2020, 12, 21},
	{"Solstice-2025-12.as", time.Date(2025, 12, 21, 15, 3, 0, 0, time.UTC), 2.46103112708e+06, 11.566666666666666, 48.13333333333333, "", "Munich, Germany", 0, true, 2025, 12, 21},
}

func TestRewrittenChartFixturesKeepTheirInstants(t *testing.T) {
	needAstrolog(t)
	for _, c := range chartRecords {
		t.Run(c.file, func(t *testing.T) {
			path := filepath.Join("testdata", "charts", c.file)
			cf, err := ResolveChartFile(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if !cf.WhenUTC.Equal(c.when) || math.Abs(cf.JD-c.jd) > 1e-5 {
				t.Errorf("instant = %v (JD %v), want %v (JD %v)", cf.WhenUTC, cf.JD, c.when, c.jd)
			}
			if cf.Name != c.name || cf.PlaceLabel != c.place {
				t.Errorf("name/place = %q/%q, want %q/%q", cf.Name, cf.PlaceLabel, c.name, c.place)
			}
			if math.Abs(cf.Lon-c.lon) > 1e-9 || math.Abs(cf.Lat-c.lat) > 1e-9 {
				t.Errorf("coordinates = %v %v, want %v %v", cf.Lon, cf.Lat, c.lon, c.lat)
			}
			if int(cf.Houses) != c.houses || cf.HasHouses != c.hasHouses {
				t.Errorf("houses = %d/%v, want %d/%v", int(cf.Houses), cf.HasHouses, c.houses, c.hasHouses)
			}
			h, err := ReadChartFileHeader(path)
			if err != nil {
				t.Fatal(err)
			}
			if h.Year != c.year || h.Month != c.month || h.Day != c.day || h.Name != c.name || h.PlaceLabel != c.place {
				t.Errorf("header = %+v, want %d-%d-%d %q %q", h, c.year, c.month, c.day, c.name, c.place)
			}
		})
	}
}
