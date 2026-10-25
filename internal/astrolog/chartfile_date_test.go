package astrolog

import (
	"os"
	"path/filepath"
	"testing"
)

// All chart values in this file are constructed synthetic test inputs.

// the specification C10: the chart picker shows a .as chart's birth
// date, read from the same -qb line its coordinates and year come from. It is
// a label off the file's own text — no astrolog process, no conversion.

func writeHeaderFixture(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "chart.as")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// noAstrolog empties PATH, so a spawn would fail and the read with it.
func noAstrolog(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

func TestReadChartFileHeaderDate(t *testing.T) {
	cases := []struct {
		name             string
		text             string
		year, month, day int
	}{
		{"month as a name", "@AI780\n-qb Mar 20 1995 12:00pm DT 1E   8:48E 53:04N\n", 1995, 3, 20},
		{"month as a number", "@AI780\n-qb 6 15 1990  4:30pm DT 1E   8:43E 52:51N\n", 1990, 6, 15},
		{"month spelled out", "@AI780\n-qb September 23 1995 6:00pm DT 1E 11:34E 48:08N\n", 1995, 9, 23},
		{"zero-padded day", "@AI780\n-qb Apr 02 2026 10:30pm DT 1E 11:34E 48:08N\n", 2026, 4, 2},
		// Leap days in astrolog's calendar: Gregorian from 1582, Julian
		// before it, where 1500 and -100 are leap years.
		{"a Gregorian leap day", "@AI780\n-qb Feb 29 2000 9:00am ST 0W 8:40E 50:06N\n", 2000, 2, 29},
		{"a Julian leap day", "@AI780\n-qb Feb 29 1500 9:00am ST 0W 8:40E 50:06N\n", 1500, 2, 29},
		{"a Julian leap day BC", "@AI780\n-qb Feb 29 -100 9:00am ST 0W 8:40E 50:06N\n", -100, 2, 29},
		// The date comes from the line the coordinates came from, not the
		// first -qb-looking line in the file.
		{"the coordinates' line", "@AI780\n-qb Jan 1 2000 9:00am\n-qb Feb 3 2001 9:00am ST 0W 8:40E 50:06N\n", 2001, 2, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			noAstrolog(t)
			h, err := ReadChartFileHeader(writeHeaderFixture(t, tc.text))
			if err != nil {
				t.Fatalf("ReadChartFileHeader: %v", err)
			}
			if h.Year != tc.year || h.Month != tc.month || h.Day != tc.day {
				t.Errorf("date = %d-%d-%d, want %d-%d-%d", h.Year, h.Month, h.Day, tc.year, tc.month, tc.day)
			}
		})
	}
}

func TestReadChartFileHeaderBadDate(t *testing.T) {
	cases := []struct{ name, qb string }{
		{"Feb 30", "-qb Feb 30 1995 12:00pm DT 1E 8:48E 53:04N"},
		{"Feb 29 in a common year", "-qb Feb 29 1995 12:00pm DT 1E 8:48E 53:04N"},
		{"month 13", "-qb 13 1 1995 12:00pm DT 1E 8:48E 53:04N"},
		{"month 0", "-qb 0 1 1995 12:00pm DT 1E 8:48E 53:04N"},
		{"a non-numeric day", "-qb Mar xx 1995 12:00pm DT 1E 8:48E 53:04N"},
		{"an unknown month name", "-qb Smarch 1 1995 12:00pm DT 1E 8:48E 53:04N"},
		// 1900 is a leap year to Julius and not to Gregory; astrolog reads
		// it as Gregorian, so Feb 29 1900 is not a date.
		{"Feb 29 1900", "-qb Feb 29 1900 12:00pm ST 0W 8:48E 53:04N"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			noAstrolog(t)
			h, err := ReadChartFileHeader(writeHeaderFixture(t, "@AI780\n"+tc.qb+"\n"))
			if err != nil {
				t.Fatalf("the chart must still list; ReadChartFileHeader: %v", err)
			}
			if h.Month != 0 || h.Day != 0 {
				t.Errorf("month, day = %d, %d; want 0, 0 — %q is not a date", h.Month, h.Day, tc.qb)
			}
			if (h.Year != 1995 && h.Year != 1900) || h.Lat == 0 {
				t.Errorf("the rest of the header was lost: %+v", h)
			}
		})
	}
}
