package astrolog

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// the specification § Fixtures: zone and instant (the place does not
// matter to the zone fields).
var localMomentFixtures = []struct {
	name, tz string
	utc      time.Time
	marker   string
	zone     string // Astrolog hours-west form
}{
	{"F1", "Europe/Amsterdam", time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC), "DT", "-1"},
	{"F2", "Europe/Amsterdam", time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC), "ST", "-1"},
	{"F3", "America/New_York", time.Date(2026, 10, 3, 2, 30, 0, 0, time.UTC), "DT", "5"},
	{"F4", "Australia/Sydney", time.Date(2026, 1, 10, 20, 0, 0, 0, time.UTC), "DT", "-10"},
	{"F5", "Asia/Kolkata", time.Date(2026, 3, 1, 6, 0, 0, 0, time.UTC), "ST", "-5:30"},
	{"F6", "Asia/Kathmandu", time.Date(2026, 3, 1, 6, 0, 0, 0, time.UTC), "ST", "-5:45"},
	// Thirty-minute daylight shift: ST and the TOTAL offset (+11), never DT.
	{"F7", "Australia/Lord_Howe", time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC), "ST", "-11"},
	{"F8", "Pacific/Kiritimati", time.Date(2026, 3, 1, 6, 0, 0, 0, time.UTC), "ST", "-14"},
	// The repeated 2:30 local hour differs only in the marker.
	{"F9a", "Europe/Amsterdam", time.Date(2026, 10, 25, 0, 30, 0, 0, time.UTC), "DT", "-1"},
	{"F9b", "Europe/Amsterdam", time.Date(2026, 10, 25, 1, 30, 0, 0, time.UTC), "ST", "-1"},
	// Amsterdam Time: an east zone under one hour, with seconds.
	{"F10a", "Europe/Amsterdam", time.Date(1930, 1, 1, 11, 40, 28, 0, time.UTC), "ST", "0:19:32E"},
	{"F10b", "Europe/Amsterdam", time.Date(1930, 6, 1, 10, 40, 28, 0, time.UTC), "DT", "0:19:32E"},
}

// localMomentFields spells a slot's wall clock the way a `-qb` entry would: month,
// day, year, H:MM:SS, the ST|DT marker and the zone in Astrolog's hours-WEST
// form. NO production call builds these any more (the specification C1: every
// moment is a `-qj` Julian Day, and the local header comes from `-zZ`); the
// tests keep it to check localClock, the one derivation of the marker and the
// zone `-zZ` speaks, against the fixtures below.
func localMomentFields(m recipe.LocalMomentAtPlace) ([]string, error) {
	t, marker, offset, err := localClock(m.JD, m.TZ)
	if err != nil {
		return nil, err
	}
	return []string{
		strconv.Itoa(int(t.Month())), strconv.Itoa(t.Day()), strconv.Itoa(t.Year()),
		fmt.Sprintf("%d:%02d:%02d", t.Hour(), t.Minute(), t.Second()),
		marker, formatSearchZone(offset),
	}, nil
}

func localFieldsAt(t *testing.T, tz string, utc time.Time) []string {
	t.Helper()
	f, err := localMomentFields(recipe.LocalMomentAtPlace{JD: GregorianToJD(utc), Lon: 4.9, Lat: 52.3667, TZ: tz})
	if err != nil {
		t.Fatalf("%s: %v", tz, err)
	}
	if len(f) != 6 {
		t.Fatalf("%s: %d fields, want 6: %v", tz, len(f), f)
	}
	return f
}

// Rule 3: DT plus the standard offset only for a one-hour daylight shift;
// otherwise ST plus the total offset.
func TestLocalMomentZoneMarker(t *testing.T) {
	for _, c := range localMomentFixtures {
		t.Run(c.name, func(t *testing.T) {
			f := localFieldsAt(t, c.tz, c.utc)
			if f[4] != c.marker || f[5] != c.zone {
				t.Errorf("%s %s: marker %s zone %s, want %s %s", c.tz, c.utc, f[4], f[5], c.marker, c.zone)
			}
		})
	}
}

// emittedUT converts the emitted -qb fields back to UT exactly as Astrolog
// reads them: local clock plus zone (hours west), less an hour when DT.
func emittedUT(t *testing.T, f []string) time.Time {
	t.Helper()
	n := func(s string) int {
		v, err := strconv.Atoi(s)
		if err != nil {
			t.Fatalf("field %q: %v", s, err)
		}
		return v
	}
	hms := strings.Split(f[3], ":")
	local := time.Date(n(f[2]), time.Month(n(f[0])), n(f[1]), n(hms[0]), n(hms[1]), n(hms[2]), 0, time.UTC)
	// Astrolog's reading, not the writer's intent: an E or W suffix names the
	// side; otherwise the sign is the HOURS field's own, so "-0:19:32" is
	// WEST (measured 2026-10-03). A writer that emits "-0:MM" fails here.
	z := f[5]
	east := false
	switch {
	case strings.HasSuffix(z, "E"):
		east, z = true, strings.TrimSuffix(z, "E")
	case strings.HasSuffix(z, "W"):
		z = strings.TrimSuffix(z, "W")
	}
	zp := strings.Split(z, ":")
	h := n(zp[0])
	if h < 0 {
		east, h = true, -h
	}
	west := h * 3600
	if len(zp) > 1 {
		west += n(zp[1]) * 60
	}
	if len(zp) > 2 {
		west += n(zp[2])
	}
	if east {
		west = -west
	}
	if f[4] == "DT" {
		west -= 3600
	}
	return local.Add(time.Duration(west) * time.Second)
}

// Rule 4: the emitted clock, marker and zone name the slot's instant to the
// second; a fractional second is truncated (amended by the specification).
func TestLocalMomentRoundTrips(t *testing.T) {
	for _, c := range localMomentFixtures {
		t.Run(c.name, func(t *testing.T) {
			if got := emittedUT(t, localFieldsAt(t, c.tz, c.utc)); !got.Equal(c.utc) {
				t.Errorf("round trip = %s, want %s", got, c.utc)
			}
		})
	}
	t.Run("fractional seconds truncate to the displayed second (the specification)", func(t *testing.T) {
		base := time.Date(2026, 7, 1, 10, 0, 7, 0, time.UTC)
		for _, c := range []struct {
			frac time.Duration
			want time.Time
		}{
			{200 * time.Millisecond, base},
			{700 * time.Millisecond, base},
		} {
			got := emittedUT(t, localFieldsAt(t, "Europe/Amsterdam", base.Add(c.frac)))
			if !got.Equal(c.want) {
				t.Errorf("+%s: got %s, want %s", c.frac, got, c.want)
			}
		}
	})
}

// An unloadable zone is a compile error that names the zone, never UT.
func TestLocalMomentRefusesAnUnknownZone(t *testing.T) {
	_, err := localMomentFields(recipe.LocalMomentAtPlace{JD: 2461316.5, TZ: "Mars/Olympus_Mons"})
	if err == nil || !strings.Contains(err.Error(), "Mars/Olympus_Mons") {
		t.Fatalf("err = %v, want one naming the zone", err)
	}
}

// Astrolog reads a zone's sign from its hours field, so an east zone under
// one hour must not be written `-0:MM`: that parses as WEST (measured
// 2026-10-03, astrolog 8.00). Amsterdam Time, +0:19:32, is the live case.
func TestFormatSearchZoneKeepsSubHourEastZonesEast(t *testing.T) {
	for _, c := range []struct {
		offset int // seconds east
		want   string
	}{
		{19*60 + 32, "0:19:32E"},
		{20 * 60, "0:20E"},
		{-(19*60 + 32), "0:19:32"},
		{3600, "-1"},
		{5*3600 + 45*60, "-5:45"},
		{-5 * 3600, "5"},
		{0, "0"},
	} {
		if got := formatSearchZone(c.offset); got != c.want {
			t.Errorf("formatSearchZone(%d) = %q, want %q", c.offset, got, c.want)
		}
	}
}
