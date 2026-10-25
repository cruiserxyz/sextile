package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// TestChartSlotsBody_NilSlots_FallsBackToSingleLine asserts that the
// pre-resolution case (m.slots == nil) renders one line built from
// clock+place. Returning empty here would leave the [1] Chart panel
// dark on cold-start before location resolves.
func TestChartSlotsBody_NilSlots_FallsBackToSingleLine(t *testing.T) {
	now := testFrozenTime()
	place := testPlaceBerlin()
	cfg := Config{Color: ColorNever}
	got := chartSlotsBody(nil, 0, true, now, place, lookup.Hellenistic, 80, cfg)

	if got == "" {
		t.Fatal("nil slots: expected single fallback line, got empty body")
	}
	if strings.Count(got, "\n") != 0 {
		t.Errorf("nil slots: expected exactly one line, got %d (\\n count)", strings.Count(got, "\n"))
	}
	if !strings.HasPrefix(got, cursorRowOff+rowPrefix(true, true, false, false, cfg)) {
		t.Errorf("nil-slots fallback should carry the focused cursor prefix; got %q", got[:6])
	}
}

// TestChartSlotsBody_TwoSlots_CursorMarksFocused asserts that with
// anchored+floating in slot order, the focused slot (cursorIdx)
// carries ▶ and the other carries the blank prefix. The body is two
// lines tall. Changed by selected-charts-compare C14: a selected Chart row's
// marker is its numeral (the anchored row ①, the floating row ②), not the ●.
func TestChartSlotsBody_TwoSlots_CursorMarksFocused(t *testing.T) {
	cfg := Config{Color: ColorNever}
	slots, err := store.NewSlots(testFrozenTime(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	if err := addAnchor(slots, time.Date(1990, 1, 1, 14, 0, 0, 0, time.UTC), testPlaceAmsterdam()); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}

	// Cursor on floating (index 1).
	bodyFloating := chartSlotsBody(slots, 1, true, testFrozenTime(), testPlaceBerlin(), lookup.Hellenistic, 80, cfg)
	linesF := strings.Split(bodyFloating, "\n")
	if len(linesF) != 2 {
		t.Fatalf("expected 2 lines for anchored+floating workspace, got %d", len(linesF))
	}
	if !strings.HasPrefix(linesF[0], chartRowPrefix(true, false, 1, true, cfg)) {
		t.Errorf("anchored line should carry blank prefix when cursor is on floating; got %q", linesF[0][:4])
	}
	if !strings.HasPrefix(linesF[1], cursorRowOff+chartRowPrefix(true, true, 2, true, cfg)) {
		t.Errorf("floating line should carry ▶ prefix when cursor is on it; got %q", linesF[1][:8])
	}

	// Cursor on anchored (index 0): focus indicator must flip.
	bodyAnchored := chartSlotsBody(slots, 0, true, testFrozenTime(), testPlaceBerlin(), lookup.Hellenistic, 80, cfg)
	linesA := strings.Split(bodyAnchored, "\n")
	if len(linesA) != 2 {
		t.Fatalf("expected 2 lines after cursor move, got %d", len(linesA))
	}
	if !strings.HasPrefix(linesA[0], cursorRowOff+chartRowPrefix(true, true, 1, true, cfg)) {
		t.Errorf("anchored line should carry ▶ when cursor moves to it; got %q", linesA[0][:8])
	}
	if !strings.HasPrefix(linesA[1], chartRowPrefix(true, false, 2, true, cfg)) {
		t.Errorf("floating line should carry blank prefix when cursor leaves it; got %q", linesA[1][:4])
	}
}

func TestChartSlotsBody_DefaultFocusIsFlushFullWidthBand(t *testing.T) {
	slots, err := store.NewSlots(testFrozenTime(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}

	const width = 64
	body := chartSlotsBody(slots, 0, true, testFrozenTime(), testPlaceBerlin(), lookup.Hellenistic, width, Config{})
	if !strings.HasPrefix(body, cursorBandStart(Config{})+"Sat") {
		t.Fatalf("default active row did not start flush inside focus band: %q", body)
	}
	if strings.Contains(body, rowCursorUnicode) || strings.Contains(body, rowCursorASCII) {
		t.Fatalf("default active row retained an indicator glyph: %q", body)
	}
	if got := ansi.StringWidth(body); got != width {
		t.Errorf("focused row width = %d, want %d", got, width)
	}
	if !strings.Contains(body, "WhTr"+sgrReset) {
		t.Fatalf("right-pinned lens tag moved out of the focus band: %q", body)
	}
}

// Changed by selected-charts-compare C14: the selected row's marker is its
// numeral (`1` in ASCII mode), not the ●/*.
func TestChartSlotsBody_SelectionColumnAppearsForWholePanel(t *testing.T) {
	slots, err := store.NewSlots(testFrozenTime(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	if err := addAnchor(slots, time.Date(1990, 1, 1, 14, 0, 0, 0, time.UTC), testPlaceAmsterdam()); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}
	slots.ToggleSlotSelection(1) // leave only the anchored slot selected

	body := chartSlotsBody(slots, 1, false, testFrozenTime(), testPlaceBerlin(), lookup.Hellenistic, 64, Config{Accessible: true})
	lines := strings.Split(body, "\n")
	if got := astrologstyle.StripSGR(lines[0]); !strings.HasPrefix(got, "1 Mon") {
		t.Fatalf("selected row lacks marker column: %q", got)
	}
	if got := astrologstyle.StripSGR(lines[1]); !strings.HasPrefix(got, rowMarkerBlank+"Sat") {
		t.Fatalf("unselected row did not share marker column: %q", got)
	}

	slots.ToggleSlotSelection(0) // no selections: remove the column for all rows
	body = chartSlotsBody(slots, 1, false, testFrozenTime(), testPlaceBerlin(), lookup.Hellenistic, 64, Config{Accessible: true})
	lines = strings.Split(body, "\n")
	if !strings.HasPrefix(astrologstyle.StripSGR(lines[0]), "Mon") || !strings.HasPrefix(astrologstyle.StripSGR(lines[1]), "Sat") {
		t.Fatalf("empty selection retained marker indentation: %q", lines)
	}
}

// TestChartSlotsBody_PreservesDisplayWidth asserts that each rendered
// slot line is exactly `width` DISPLAY CELLS wide. The cursor prefix
// (▶ is multi-byte) must be measured by cell count, not byte length —
// otherwise the right-pinned tag shifts inward.
func TestChartSlotsBody_PreservesDisplayWidth(t *testing.T) {
	slots, err := store.NewSlots(testFrozenTime(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	if err := addAnchor(slots, time.Date(1990, 1, 1, 14, 0, 0, 0, time.UTC), testPlaceAmsterdam()); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}
	const width = 64
	body := chartSlotsBody(slots, 1, true, testFrozenTime(), testPlaceBerlin(), lookup.Hellenistic, width, Config{})
	for i, line := range strings.Split(body, "\n") {
		if w := ansi.StringWidth(line); w != width {
			t.Errorf("line %d display width = %d, want %d (line: %q)", i, w, width, line)
		}
	}
}

// Changed by selected-charts-compare C14: the persistent marker is each
// selected row's numeral (`1`, `2` in ASCII mode), not the ●/*.
func TestChartSlotsBody_HidesCursorWithoutPanelFocusButKeepsBoldSelection(t *testing.T) {
	slots, err := store.NewSlots(testFrozenTime(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	if err := addAnchor(slots, time.Date(1990, 1, 1, 14, 0, 0, 0, time.UTC), testPlaceAmsterdam()); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}

	const width = 64
	body := chartSlotsBody(slots, 1, false, testFrozenTime(), testPlaceBerlin(), lookup.Hellenistic, width, Config{Accessible: true})
	for i, line := range strings.Split(body, "\n") {
		plain := astrologstyle.StripSGR(line)
		if strings.HasPrefix(plain, rowCursorUnicode) || strings.HasPrefix(plain, rowCursorASCII) {
			t.Errorf("line %d retained a cursor while Chart was unfocused: %q", i, line)
		}
		if !strings.HasPrefix(plain, fmt.Sprintf("%d ", i+1)) {
			t.Errorf("line %d lost its persistent marker: %q", i, line)
		}
		// the specification rule 1: the cursor row (slot 1)
		// stays bold without the keyboard, and only it; no reverse video.
		if bold := strings.HasPrefix(line, sgrBold); bold != (i == 1) {
			t.Errorf("line %d bold = %v while Chart was unfocused: %q", i, bold, line)
		}
		if strings.Contains(line, sgrReverse) {
			t.Errorf("line %d is in reverse video while Chart was unfocused: %q", i, line)
		}
		if got := ansi.StringWidth(line); got != width {
			t.Errorf("line %d width = %d, want %d", i, got, width)
		}
	}
}

// TestChartContentLines_GrowsWithSlots asserts that the chart-panel
// content-line count matches the workspace cardinality and falls back
// to 1 before the workspace is built. This is the height contract the
// layout depends on (chartH = chartLines + 2).
func TestChartContentLines_GrowsWithSlots(t *testing.T) {
	if got := chartContentLines(nil); got != 1 {
		t.Errorf("nil slots: chartContentLines = %d, want 1", got)
	}

	one, err := store.NewSlots(testFrozenTime(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	if got := chartContentLines(one); got != 1 {
		t.Errorf("floating only: chartContentLines = %d, want 1", got)
	}

	if err := addAnchor(one, time.Date(1990, 1, 1, 14, 0, 0, 0, time.UTC), testPlaceAmsterdam()); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}
	if got := chartContentLines(one); got != 2 {
		t.Errorf("anchored+floating: chartContentLines = %d, want 2", got)
	}
}
