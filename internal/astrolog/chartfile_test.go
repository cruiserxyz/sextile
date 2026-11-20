package astrolog

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// All chart values in this file are constructed synthetic test inputs.

func fixture(name string) string {
	return filepath.Join("testdata", "charts", name)
}

// ResolveChartFile invokes the real astrolog binary (-i <f> -w) to read its
// authoritative Julian Day; these are integration tests like run_test.go.
func TestResolveChartFile_Equinox(t *testing.T) {
	cf, err := ResolveChartFile(context.Background(), fixture("Equinox-2026-03.as"))
	if err != nil {
		t.Fatalf("ResolveChartFile: %v", err)
	}
	// Mar 20 2026 3:46pm ST 1E → UT 14:46.
	want := time.Date(2026, 3, 20, 14, 46, 0, 0, time.UTC)
	if !cf.WhenUTC.Equal(want) {
		t.Errorf("WhenUTC = %v, want %v", cf.WhenUTC, want)
	}
	assertCoord(t, "Lon", cf.Lon, 11.5667)
	assertCoord(t, "Lat", cf.Lat, 48.1333)
	if cf.Name != "" {
		t.Errorf("Name = %q, want empty", cf.Name)
	}
	if cf.PlaceLabel != "Munich, Germany" {
		t.Errorf("PlaceLabel = %q, want %q", cf.PlaceLabel, "Munich, Germany")
	}
}

// A named chart (-zi "Great Conjunction" "Frankfurt, Germany") with ST 0W: UT == local.
func TestResolveChartFile_NamedAndStandardTime(t *testing.T) {
	cf, err := ResolveChartFile(context.Background(), fixture("GreatConjunction-2020-12-21.as"))
	if err != nil {
		t.Fatalf("ResolveChartFile: %v", err)
	}
	want := time.Date(2020, 12, 21, 18, 19, 0, 0, time.UTC)
	if !cf.WhenUTC.Equal(want) {
		t.Errorf("WhenUTC = %v, want %v", cf.WhenUTC, want)
	}
	if cf.Name != "Great Conjunction" {
		t.Errorf("Name = %q, want %q", cf.Name, "Great Conjunction")
	}
	if cf.PlaceLabel != "Frankfurt, Germany" {
		t.Errorf("PlaceLabel = %q, want %q", cf.PlaceLabel, "Frankfurt, Germany")
	}
}

// Empty -zi name AND place must resolve (name/place filled in a higher layer).
func TestResolveChartFile_EmptyLabels(t *testing.T) {
	cf, err := ResolveChartFile(context.Background(), fixture("FullMoon-2026-02.as"))
	if err != nil {
		t.Fatalf("ResolveChartFile: %v", err)
	}
	want := time.Date(2026, 2, 1, 22, 9, 0, 0, time.UTC)
	if !cf.WhenUTC.Equal(want) {
		t.Errorf("WhenUTC = %v, want %v", cf.WhenUTC, want)
	}
	if cf.Name != "" || cf.PlaceLabel != "" {
		t.Errorf("labels = (%q, %q), want both empty", cf.Name, cf.PlaceLabel)
	}
	assertCoord(t, "Lon", cf.Lon, 9.0167)
	assertCoord(t, "Lat", cf.Lat, 53.0000)
}

// A file that opens with @AI and holds no chart must be rejected, not
// silently mis-resolved.
func TestResolveChartFile_RejectsNonChart(t *testing.T) {
	if _, err := ResolveChartFile(context.Background(), fixture("InvalidFile.as")); err == nil {
		t.Error("ResolveChartFile on a non-chart file returned nil error")
	}
}

// A chart is a file that OPENS with @AI. Text that merely holds the header
// somewhere, around a perfectly good -qb line, is not one.
func TestChartFileHeaderMustOpenTheFile(t *testing.T) {
	body := "a note first\n@AI780\n-qb Mar 20 1995 12:00pm DT 1E 8:48E 53:04N\n"
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadChartFileHeader(path); err == nil {
		t.Error("ReadChartFileHeader accepted a file that does not open with @AI")
	}
	if _, err := ResolveChartFile(context.Background(), path); err == nil {
		t.Error("ResolveChartFile accepted a file that does not open with @AI")
	}
}

func TestResolveChartFile_MissingFile(t *testing.T) {
	if _, err := ResolveChartFile(context.Background(), fixture("does-not-exist.as")); err == nil {
		t.Error("ResolveChartFile on a missing file returned nil error")
	}
}

// Astrolog owns the Julian/Gregorian boundary. These are the Phase A probe
// rows turned into an executable guard so Go never grows a competing calendar.
func TestJulianDayLabelsFollowAstrologs1582Boundary(t *testing.T) {
	cases := []struct {
		jd, label string
	}{
		{"2299160.5", "Fri Oct  5, 1582 12:00am"},
		{"2299161.5", "Sat Oct  6, 1582 12:00am"},
		{"2299170.5", "Mon Oct 25, 1582 12:00am"},
		{"2299200.5", "Wed Nov 24, 1582 12:00am"},
		{"2460000.5", "Sat Feb 25, 2023 12:00am"},
	}
	for _, tc := range cases {
		t.Run(tc.jd, func(t *testing.T) {
			out, _, err := invoke(context.Background(), []string{"-qj", tc.jd, "-zl", "0:00E", "0:00N", "-v"})
			if err != nil {
				t.Fatalf("Astrolog -qj %s: %v", tc.jd, err)
			}
			header := strings.SplitN(out, "\n", 2)[0]
			if !strings.Contains(header, tc.label) {
				t.Errorf("header = %q, want it to contain %q", header, tc.label)
			}
		})
	}
}

// parseCoordinate reads every spelling of a coordinate that Astrolog reads in
// a chart file, and nothing else.
func TestParseCoordinate(t *testing.T) {
	cases := []struct {
		in       string
		pos, neg byte
		want     float64
		ok       bool
	}{
		{"11:34E", 'E', 'W', 11 + 34.0/60, true},
		{"48:08N", 'N', 'S', 48 + 8.0/60, true},
		{"0W", 'E', 'W', 0, true},
		{"5:30W", 'E', 'W', -5.5, true},
		{"10:00S", 'N', 'S', -10, true},
		{"52:31'30N", 'N', 'S', 52 + 31.0/60 + 30.0/3600, true},
		{"4.88E", 'E', 'W', 4.88, true},
		{"4:53e", 'E', 'W', 4 + 53.0/60, true},
		{"4E53", 'E', 'W', 4 + 53.0/60, true},
		{"52n22:30", 'N', 'S', 52 + 22.0/60 + 30.0/3600, true},
		{"157:28W", 'E', 'W', -(157 + 28.0/60), true},
		{"4:53", 'E', 'W', 0, false},
		{"E53", 'E', 'W', 0, false},
		{"4:75E", 'E', 'W', 0, false},
		{"52:22N", 'E', 'W', 0, false},
	}
	for _, c := range cases {
		got, ok := parseCoordinate(c.in, c.pos, c.neg)
		if ok != c.ok || math.Abs(got-c.want) > 1e-9 {
			t.Errorf("parseCoordinate(%q) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func assertCoord(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.001 {
		t.Errorf("%s = %.4f, want %.4f", label, got, want)
	}
}
