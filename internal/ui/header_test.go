package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
)

// TestChartSlotLine_Format pins the slot line: date/time/TZ on the left, the
// condensed house·zodiac tag ("WhTr") at the far right, the location between —
// all fitted to exactly the given width.
func TestChartSlotLine_Format(t *testing.T) {
	when := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	place := location.PlaceInfo{Name: "Bremen", Country: "DE", TZ: "Europe/Berlin"}

	const width = 64
	got := chartSlotLine(when, place, lookup.Hellenistic, width, Config{})

	if w := ansi.StringWidth(got); w != width {
		t.Fatalf("slot line display width = %d, want %d (%q)", w, width, got)
	}
	if !strings.HasPrefix(got, "Sat Apr 25, 2026 14:00 CEST") {
		t.Errorf("slot line should start with the Berlin-zoned moment, got %q", got)
	}
	if !strings.HasSuffix(got, "WhTr") {
		t.Errorf("slot line should end with the condensed tag WhTr, got %q", got)
	}
	if !strings.Contains(got, "Bremen, DE") {
		t.Errorf("slot line should carry the location, got %q", got)
	}
}

// TestChartSlotLine_NoTimezone returns empty so the panel body stays blank
// before a location is resolved.
func TestChartSlotLine_NoTimezone(t *testing.T) {
	when := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	if got := chartSlotLine(when, location.PlaceInfo{Name: "Nowhere"}, lookup.Hellenistic, 64, Config{}); got != "" {
		t.Errorf("no-timezone slot line should be empty, got %q", got)
	}
}

// TestLayoutSlotLine_TruncatesLocation verifies the location is ellipsis-
// truncated to fit, the tag stays pinned right, and the width is exact.
func TestLayoutSlotLine_TruncatesLocation(t *testing.T) {
	left := "Sat Apr 25, 2026 14:00 CEST"
	// width leaves only 7 cells for the location field → "Bremen…" ("Brem..." under --accessible)
	width := ansi.StringWidth(left) + 1 + 7 + 1 + 4
	got := layoutSlotLine(left, "Bremen, DE", "WhTr", width, Config{})

	if w := ansi.StringWidth(got); w != width {
		t.Fatalf("width = %d, want %d (%q)", w, width, got)
	}
	if !strings.Contains(got, "Bremen…") {
		t.Errorf("location should be cut to %q, got %q", "Bremen…", got)
	}
	if got := layoutSlotLine(left, "Bremen, DE", "WhTr", width, Config{Accessible: true}); !strings.Contains(got, "Brem...") {
		t.Errorf("under --accessible the location should be cut to %q, got %q", "Brem...", got)
	}
	if !strings.HasSuffix(got, "WhTr") {
		t.Errorf("tag should stay pinned right, got %q", got)
	}
}

// TestLayoutSlotLine_WideCharLocation guards display-width correctness: a CJK
// location (each glyph 2 cells) must still produce a line of exactly width
// DISPLAY cells, matching renderTitledBox's padBody (rune count would be wrong).
func TestLayoutSlotLine_WideCharLocation(t *testing.T) {
	left := "Sat Apr 25, 2026 14:00 JST"
	const width = 60
	got := layoutSlotLine(left, "東京, JP", "WhTr", width, Config{}) // 東京 = 4 display cells

	if w := ansi.StringWidth(got); w != width {
		t.Fatalf("display width = %d, want %d (%q)", w, width, got)
	}
	if !strings.HasSuffix(got, "WhTr") {
		t.Errorf("tag should stay pinned right, got %q", got)
	}
}

// TestLayoutSlotLine_NarrowFallback pins the policy: when width can't fit
// left + tag + gaps, the location is dropped, the tag stays pinned right, and
// the line is still exactly width cells.
func TestLayoutSlotLine_NarrowFallback(t *testing.T) {
	left := "Sat Apr 25, 2026 14:00 CEST"
	width := ansi.StringWidth(left) + 4 // only room for left + "WhTr", no location
	got := layoutSlotLine(left, "Bremen, DE", "WhTr", width, Config{})

	if w := ansi.StringWidth(got); w != width {
		t.Fatalf("display width = %d, want %d (%q)", w, width, got)
	}
	if !strings.HasSuffix(got, "WhTr") {
		t.Errorf("tag should stay pinned right even when location is dropped, got %q", got)
	}
	if strings.Contains(got, "Bremen") {
		t.Errorf("location should be dropped when there's no room, got %q", got)
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
