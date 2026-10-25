package lookup

// the specification C8: the month the timeline asks Astrolog for is the one
// Astrolog's reader names for the floating slot's moment, and no moment before
// Julian Day 0 starts a search.

import (
	"context"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
)

// julianNoon is the Unix second of an hour on a Julian-calendar date, by
// Meeus's formula, written out here.
func julianSecond(year, month, day int, hour float64) int64 {
	y, m := float64(year), float64(month)
	if month <= 2 {
		y, m = y-1, m+12
	}
	jd := math.Floor(365.25*(y+4716)) + math.Floor(30.6001*(m+1)) + float64(day) - 1524.5 + hour/24
	return int64(math.Round((jd - 2440587.5) * 86400))
}

func TestTimelineAsksForAstrologsMonth(t *testing.T) {
	utc := location.PlaceInfo{Name: "Greenwich", Lat: 51.5, Lon: 0, TZ: "UTC"}
	// Gregorian 1 November 1582 is the Unix second below; Julian 1 Oct 1582 and
	// Julian 1 Feb 1500 by Meeus.
	nov1 := time.Date(1582, 11, 1, 0, 0, 0, 0, time.UTC).Unix() // Gregorian
	cases := []struct {
		name        string
		unix        int64
		month, year int
	}{
		{"an hour after Julian 1 Feb 1500", julianSecond(1500, 2, 1, 1), 2, 1500},
		{"an hour before Julian 1 Feb 1500", julianSecond(1500, 2, 1, 0) - 3600, 1, 1500},
		{"Julian 29 Feb 1500 noon", julianSecond(1500, 2, 29, 12), 2, 1500},
		{"Julian 1 Mar 1500 an hour in", julianSecond(1500, 3, 1, 1), 3, 1500},
		{"an hour after Julian 1 Oct 1582", julianSecond(1582, 10, 1, 1), 10, 1582},
		{"an hour before Julian 1 Oct 1582", julianSecond(1582, 10, 1, 0) - 3600, 9, 1582},
		{"Julian 4 Oct 1582 late", julianSecond(1582, 10, 4, 23), 10, 1582},
		{"an hour before Gregorian 1 Nov 1582", nov1 - 3600, 10, 1582},
		{"an hour after Gregorian 1 Nov 1582", nov1 + 3600, 11, 1582},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			search := &cannedSearch{byMode: map[string]string{}}
			_, err := TimelineWith(context.Background(), TimelineRequest{When: time.Unix(c.unix, 0).UTC(), Place: utc}, search.run, stubDefaults)
			if err != nil {
				t.Fatal(err)
			}
			if len(search.argvs) == 0 {
				t.Fatal("no search was asked")
			}
			argv := search.argvs[0]
			var month, year string
			for i, a := range argv {
				if a == "-qm" && i+2 < len(argv) {
					month, year = argv[i+1], argv[i+2]
				}
			}
			if month != strconv.Itoa(c.month) || year != strconv.Itoa(c.year) {
				t.Errorf("asked for %s/%s, want %d/%d (argv %q)", month, year, c.month, c.year, argv)
			}
		})
	}
}

func TestTimelineBeforeJulianDayZeroStartsNoSearch(t *testing.T) {
	utc := location.PlaceInfo{Name: "Greenwich", Lat: 51.5, Lon: 0, TZ: "UTC"}
	defaults, searches := 0, 0
	read := func(ctx context.Context) (DefaultSets, error) { defaults++; return stubDefaults(ctx) }
	search := func(context.Context, BodyCall) (string, string, error) { searches++; return "", "", nil }
	_, err := TimelineWith(context.Background(), TimelineRequest{When: time.Unix(-210866760000-86400, 0).UTC(), Place: utc}, search, read)
	if err == nil {
		t.Error("a moment before Julian Day 0 was searched")
	}
	if defaults != 0 || searches != 0 {
		t.Errorf("%d default reads and %d searches started for a moment before Julian Day 0, want none", defaults, searches)
	}
}
