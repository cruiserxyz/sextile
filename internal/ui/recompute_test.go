package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// TestSelectComputeMode_TransitOnlyWhenNoAnchor pins the routing contract:
// the workspace with no anchored chart selects computeModeTransit. A
// future accidental collapse where overlay always loses to transit would
// fail here — earlier nil-check-only tests could not catch that, the reviewer
// flagged it in review.
func TestSelectComputeMode_TransitOnlyWhenNoAnchor(t *testing.T) {
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(frozenInstantForGolden()))
	m.place = testPlaceBerlin()
	if !anchorHandle(m).IsZero() {
		t.Fatalf("precondition: lastChart should be zero, got %+v", anchorHandle(m))
	}

	if got := selectComputeMode(m); got != computeModeTransit {
		t.Errorf("selectComputeMode without anchor = %d, want computeModeTransit (%d)", got, computeModeTransit)
	}
	if cmd := buildTextCmd(m, context.Background(), 1, 1); cmd == nil {
		t.Error("buildTextCmd returned nil cmd in transit mode (freeze-safety: text must always be runnable)")
	}
}

// TestBuildTextCmd_NeverNil pins the freeze-safety contract from the reviewer
// S4 design-check: the text surface must ALWAYS be runnable — a nil Cmd
// with no live debounce tick would strand an unmet want forever. Even the
// degenerate pre-resolution model (nil slots, zero place) must produce a
// compute (wall-clock transit fallback).
func TestBuildTextCmd_NeverNil(t *testing.T) {
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(frozenInstantForGolden()))
	// nil slots, zero place — the most degenerate workspace possible.
	if cmd := buildTextCmd(m, context.Background(), 1, 1); cmd == nil {
		t.Fatal("buildTextCmd returned nil for the degenerate workspace — unmet wants could strand")
	}
}

// TestSelectComputeMode_OverlayWhenAnchorPresent pins the other half of
// the routing contract: an anchored slot with a non-zero moment and place
// flips the decision to computeModeOverlay. Together with the previous
// test, this prevents accidental routing regressions.
func TestSelectComputeMode_OverlayWhenAnchorPresent(t *testing.T) {
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(frozenInstantForGolden()))
	m.place = testPlaceBerlin()
	natal := store.AnchorChart{
		When:  time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC),
		Place: testPlaceAmsterdam(),
	}
	slots, err := store.NewSlots(frozenInstantForGolden(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	if err := addAnchor(slots, natal.When, natal.Place); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}
	m.slots = slots

	if got := selectComputeMode(m); got != computeModeOverlay {
		t.Errorf("selectComputeMode with anchor = %d, want computeModeOverlay (%d)", got, computeModeOverlay)
	}
	if cmd := buildTextCmd(m, context.Background(), 1, 1); cmd == nil {
		t.Error("buildTextCmd returned nil cmd in overlay mode")
	}

	slots.ToggleSlotSelection(1)
	if got := selectComputeMode(m); got != computeModeNatalSingle {
		t.Errorf("selectComputeMode after deselecting pair = %d, want focused natal single (%d)", got, computeModeNatalSingle)
	}
}

// TestChartSlotsBody_HeaderUsesSlotTimeNotLiveNow asserts the
// floating-time honesty fix: the rendered line for the floating slot
// reflects slot.Time (the compute time captured at construction), not
// the live clock passed in at render. Without this fix the chart panel
// would advertise a current wall-clock time next to bodies/aspects
// that were computed for an earlier moment — a trust-breaking lie
// during consults.
func TestChartSlotsBody_HeaderUsesSlotTimeNotLiveNow(t *testing.T) {
	// Slot built at T_construct = 2026-04-25 12:00 UTC (CEST 14:00).
	slotTime := frozenInstantForGolden()
	slots, err := store.NewSlots(slotTime, testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}

	// Render at a "live" now that is 6 hours later than the slot's
	// captured time. Pre-fix, the header would have displayed the live
	// time. Post-fix, it must display slotTime.
	livelyNow := slotTime.Add(6 * time.Hour)
	body := chartSlotsBody(slots, 0, true, livelyNow, testPlaceBerlin(), lookup.Hellenistic, 80, Config{})

	// CEST is UTC+2 in April so 12:00 UTC = 14:00 CEST. The renderer
	// formats the slot time in its own timezone (Europe/Berlin).
	if !strings.Contains(body, "14:00 CEST") {
		t.Errorf("expected slot time 14:00 CEST in header, got:\n%s", body)
	}
	// 20:00 CEST is what would appear if the live `livelyNow` (slotTime+6h)
	// had been used instead.
	if strings.Contains(body, "20:00 CEST") {
		t.Errorf("header advertised live clock time (20:00 CEST) instead of slot.Time; got:\n%s", body)
	}
}

// TestChartSlotsBody_HeaderStableAcrossRenders asserts the slot-time
// honesty fix from a different angle: rendering the same workspace
// twice with two different `now` arguments produces identical bytes
// (the slot lines do not vary with wall-clock advancement). Catches
// any future regression that re-introduces the live-now branch.
func TestChartSlotsBody_HeaderStableAcrossRenders(t *testing.T) {
	slots, err := store.NewSlots(frozenInstantForGolden(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	first := chartSlotsBody(slots, 0, true, frozenInstantForGolden(), testPlaceBerlin(), lookup.Hellenistic, 80, Config{})
	later := chartSlotsBody(slots, 0, true, frozenInstantForGolden().Add(45*time.Minute), testPlaceBerlin(), lookup.Hellenistic, 80, Config{})
	if first != later {
		t.Errorf("chartSlotsBody output differs across renders despite same workspace\nfirst:\n%s\nlater:\n%s", first, later)
	}
}

// _ keeps location import alive for the location.PlaceInfo references
// implicit through testPlaceBerlin/testPlaceAmsterdam helpers.
var _ = location.PlaceInfo{}
