package astrolog

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"
)

// The write half of the moment oracle: a chart saved at an instant is a file
// Astrolog reads back to that instant's second, wherever its zone puts the
// clock: before standard time, in a zone with seconds, half an hour off, at
// the date line, inside the 1582 window and before the reform. Where the zone
// is whole minutes, the file's own clock is the city's clock to the second.
func TestSavedChartReadsBackToTheSecond(t *testing.T) {
	needAstrolog(t)
	zone := func(name string) *time.Location {
		loc, err := time.LoadLocation(name)
		if err != nil {
			t.Fatal(err)
		}
		return loc
	}
	for _, tc := range []struct {
		name     string
		when     time.Time
		lon, lat float64
		clock    bool // the file's clock is the city's in its zone to the whole minute
	}{
		{"Berlin before standard time", time.Date(1850, 6, 1, 12, 34, 56, 0, zone("Europe/Berlin")), 13.41, 52.52, false},
		{"Amsterdam time with seconds", time.Date(1930, 1, 6, 12, 34, 56, 0, zone("Europe/Amsterdam")), 4.89, 52.37, true},
		{"Amsterdam winter", time.Date(2026, 11, 13, 20, 8, 40, 0, zone("Europe/Amsterdam")), 4.89, 52.37, true},
		{"a fraction of a second", time.Date(2026, 11, 17, 12, 47, 31, 996_000_000, time.UTC), 4.89, 52.37, true},
		{"India", time.Date(1987, 7, 14, 16, 30, 5, 0, zone("Asia/Kolkata")), 88.36, 22.57, true},
		{"New York summer", time.Date(1987, 7, 14, 16, 30, 5, 0, zone("America/New_York")), -74.0, 40.71, true},
		{"Kiritimati", time.Date(2000, 7, 4, 12, 0, 1, 0, zone("Pacific/Kiritimati")), -157.47, 1.87, true},
		{"inside the 1582 window", time.Date(1582, 10, 14, 23, 30, 0, 0, time.UTC), 13.41, 52.52, false},
		{"before the reform", time.Date(1500, 3, 1, 9, 15, 30, 0, time.UTC), 13.41, 52.52, false}, // Go names the day in its own calendar, not the Julian one
	} {
		t.Run(tc.name, func(t *testing.T) {
			record, err := ChartRecord(context.Background(), ChartToSave{Name: "Saved", Place: "Here", When: tc.when, Lon: tc.lon, Lat: tc.lat})
			if err != nil {
				t.Fatalf("ChartRecord: %v", err)
			}
			// The file holds the chart and nothing of Sextile's own settings:
			// a node line in it could not be told from the client's own wish.
			for _, line := range strings.Split(string(record), "\n") {
				if f := strings.Fields(line); len(f) > 0 && strings.Contains(f[0], "Yn") {
					t.Errorf("the save wrote the node line %q", line)
				}
			}
			cf, err := ResolveChartBytes(context.Background(), record, "saved.as")
			if err != nil {
				t.Fatalf("Astrolog does not read the saved file back: %v\n%s", err, record)
			}
			if want := tc.when.Truncate(time.Second).UTC(); !cf.WhenUTC.Equal(want) {
				t.Errorf("the saved file reads back as %v, saved from %v\n%s", cf.WhenUTC, want, record)
			}
			_, east := tc.when.Zone()
			local := tc.when.Truncate(time.Second).UTC().Add(time.Duration(math.Round(float64(east)/60)) * time.Minute)
			if c := cf.Clock; tc.clock && (c.Hour != local.Hour() || c.Minute != local.Minute() || c.Second != local.Second() || c.Day != local.Day()) {
				t.Errorf("the saved file's clock is %d %02d:%02d:%02d, the city's %v\n%s", c.Day, c.Hour, c.Minute, c.Second, local, record)
			}
			if !cf.StatedSeconds {
				t.Errorf("the saved file does not state its second:\n%s", record)
			}
		})
	}
}
