package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
)

// TestChartRowFormat pins a row of an atlas place: date, clock with seconds
// and the offset, the place, and the condensed house and zodiac tag ("WhTr")
// at the far right, fitted to exactly the given width.
func TestChartRowFormat(t *testing.T) {
	when := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	place := location.PlaceInfo{Name: "Bremen", Country: "DE", TZ: "Europe/Berlin"}

	const width = 64
	got := rowText(when, place, "", "WhTr", width, Config{})

	if w := ansi.StringWidth(got); w != width {
		t.Fatalf("row display width = %d, want %d (%q)", w, width, got)
	}
	if !strings.HasPrefix(got, "Sat Apr 25, 2026 14:00:00 +2 DT Bremen, DE") {
		t.Errorf("the row should read the Berlin clock, its offset and the place, got %q", got)
	}
	if !strings.HasSuffix(got, "WhTr") {
		t.Errorf("the row should end with the condensed tag WhTr, got %q", got)
	}
}

// TestChartRowNoTimezone: a place with no zone (a chart file's, before its
// file is read) shows the instant in UT, whose offset is +0.
func TestChartRowNoTimezone(t *testing.T) {
	when := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	if got := rowText(when, location.PlaceInfo{Name: "Nowhere"}, "", "WhTr", 70, Config{}); !strings.HasPrefix(got, "Sat Apr 25, 2026 12:00:00 +0 ") {
		t.Errorf("a place with no zone shows its instant in UT, got %q", got)
	}
}

// TestChartRowCutsTheWords: confirmed place words are cut with the mark, the
// tag stays pinned right, and the width is exact.
func TestChartRowCutsTheWords(t *testing.T) {
	when := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	place := location.PlaceInfo{Name: "Bremen", Country: "DE", TZ: "Europe/Berlin"}
	left := "Sat Apr 25, 2026 14:00:00 +2 DT"
	width := ansi.StringWidth(left) + 1 + 14 + 1 + 4
	got := rowText(when, place, "", "WhTr", width, Config{})
	if w := ansi.StringWidth(got); w != width {
		t.Fatalf("width = %d, want %d (%q)", w, width, got)
	}
	if !strings.Contains(got, "Bremen, DE") || !strings.HasSuffix(got, "WhTr") {
		t.Errorf("a place that fits stays whole and the tag pinned right, got %q", got)
	}
	long := location.PlaceInfo{Name: "Bremerhaven-Lehe", Country: "DE", TZ: "Europe/Berlin"}
	if got := rowText(when, long, "", "WhTr", width, Config{}); !strings.Contains(got, "Bremerhaven-L…") {
		t.Errorf("long words should be cut to %q, got %q", "Bremerhaven-L…", got)
	}
	if got := rowText(when, long, "", "WhTr", width, Config{Accessible: true}); !strings.Contains(got, "Bremerhaven...") {
		t.Errorf("under --accessible the words should be cut to %q, got %q", "Bremerhaven...", got)
	}
}

// TestChartRowWideCharPlace guards display-width correctness: a CJK place
// (each glyph 2 cells) still gives a row of exactly width display cells.
func TestChartRowWideCharPlace(t *testing.T) {
	when := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	const width = 60
	got := rowText(when, location.PlaceInfo{Name: "東京", Country: "JP", TZ: "Asia/Tokyo"}, "", "WhTr", width, Config{})
	if w := ansi.StringWidth(got); w != width {
		t.Fatalf("display width = %d, want %d (%q)", w, width, got)
	}
	if !strings.HasSuffix(got, "WhTr") {
		t.Errorf("tag should stay pinned right, got %q", got)
	}
}

// TestLensAbbrev pins the condensed codes used in the slot tag.
func TestLensAbbrev(t *testing.T) {
	if got := lookup.Hellenistic.HouseAbbrev() + lookup.Hellenistic.ZodiacAbbrev(); got != "WhTr" {
		t.Errorf("Hellenistic tag = %q, want %q", got, "WhTr")
	}
}

func TestChartWindowEdgeTravelDown(t *testing.T) {
	for cursor, want := range []int{0, 0, 0, 0, 0} {
		if got, _ := chartWindow(8, cursor, 5, 0); got != want {
			t.Errorf("cursor %d from top 0: start %d, want %d", cursor, got, want)
		}
	}
	if got, _ := chartWindow(8, 5, 5, 0); got != 1 {
		t.Errorf("cursor 5 from top 0: start %d, want 1", got)
	}
	if got, _ := chartWindow(8, 7, 5, 1); got != 3 {
		t.Errorf("cursor 7 from top 1: start %d, want 3", got)
	}
}

func TestChartWindowEdgeTravelUp(t *testing.T) {
	for cursor := 7; cursor >= 3; cursor-- {
		if got, _ := chartWindow(8, cursor, 5, 3); got != 3 {
			t.Errorf("cursor %d from top 3: start %d, want 3", cursor, got)
		}
	}
	if got, _ := chartWindow(8, 2, 5, 3); got != 2 {
		t.Errorf("cursor 2 from top 3: start %d, want 2", got)
	}
	if got, _ := chartWindow(8, 0, 5, 2); got != 0 {
		t.Errorf("cursor 0 from top 2: start %d, want 0", got)
	}
}

func TestChartWindowAlwaysHoldsCursor(t *testing.T) {
	for total := 1; total <= 9; total++ {
		for rows := 1; rows <= 7; rows++ {
			for top := -1; top <= 9; top++ {
				for cursor := 0; cursor < total; cursor++ {
					start, visible := chartWindow(total, cursor, rows, top)
					if cursor < start || cursor >= start+visible || start+visible > total || start < 0 {
						t.Errorf("total %d rows %d top %d cursor %d: window [%d,+%d) misses the cursor or the list", total, rows, top, cursor, start, visible)
					}
					if total <= rows && (start != 0 || visible != total) {
						t.Errorf("total %d rows %d top %d: a list that fits gave start %d visible %d", total, rows, top, start, visible)
					}
				}
			}
		}
	}
}
