package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// TestCycleSlotCursor_NoSlots_NoOp asserts that pressing ↑/↓ before slots
// are constructed (cold-start, pre-resolution) is a silent no-op. The
// handler must not panic on a nil m.slots.
func TestCycleSlotCursor_NoSlots_NoOp(t *testing.T) {
	m := NewModelWithClock(Config{}, FrozenClock(testFrozenTime()))

	if got := m.cycleSlotCursor(+1); got.cursorIdx != 0 {
		t.Errorf("nil slots: cycleSlotCursor(+1) moved cursor to %d, want 0", got.cursorIdx)
	}
	if got := m.cycleSlotCursor(-1); got.cursorIdx != 0 {
		t.Errorf("nil slots: cycleSlotCursor(-1) moved cursor to %d, want 0", got.cursorIdx)
	}
	if m.focusedSlot() != nil {
		t.Error("focusedSlot() should be nil before slots are constructed")
	}
}

// TestCycleSlotCursor_FloatingOnly_NoOp asserts that with a single-slot
// workspace (floating only, no anchor) ↑/↓ does nothing — there is no
// other slot to move to.
func TestCycleSlotCursor_FloatingOnly_NoOp(t *testing.T) {
	m := NewModelWithClock(Config{}, FrozenClock(testFrozenTime()))
	slots, err := store.NewSlots(testFrozenTime(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	m.slots = slots
	m.cursorIdx = 0

	if got := m.cycleSlotCursor(+1); got.cursorIdx != 0 {
		t.Errorf("single slot: cycleSlotCursor(+1) moved cursor to %d, want 0", got.cursorIdx)
	}
	if got := m.cycleSlotCursor(-1); got.cursorIdx != 0 {
		t.Errorf("single slot: cycleSlotCursor(-1) moved cursor to %d, want 0", got.cursorIdx)
	}

	focused := m.focusedSlot()
	if focused == nil || !focused.Floating {
		t.Error("focusedSlot() should be the floating slot")
	}
}

// TestCycleSlotCursor_TwoSlots_StopsAtBothEnds asserts that with anchored +
// floating in slot order [anchored, floating], the cursor stops at each end
// instead of wrapping (the specification C12): from floating
// (index 1) ↓ stays 1, ↑ -> 0, ↑ stays 0, ↓ -> 1.
func TestCycleSlotCursor_TwoSlots_StopsAtBothEnds(t *testing.T) {
	m := NewModelWithClock(Config{}, FrozenClock(testFrozenTime()))
	slots, err := store.NewSlots(testFrozenTime(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	if err := addAnchor(slots, time.Date(1990, 1, 1, 14, 0, 0, 0, time.UTC), testPlaceAmsterdam()); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}
	m.slots = slots
	m.cursorIdx = 1 // cold-start vocab: cursor on floating; floating is index 1 here

	all := slots.All()
	if len(all) != 2 {
		t.Fatalf("All() length = %d, want 2", len(all))
	}

	// Down from floating, the last row, stays.
	m = m.cycleSlotCursor(+1)
	if m.cursorIdx != 1 {
		t.Errorf("after ↓ from floating: cursorIdx = %d, want 1 (it stops)", m.cursorIdx)
	}

	// Up from floating goes to anchored.
	m = m.cycleSlotCursor(-1)
	if m.cursorIdx != 0 {
		t.Errorf("after ↑ from floating: cursorIdx = %d, want 0 (anchored)", m.cursorIdx)
	}
	if got := m.focusedSlot(); got == nil || got.Floating {
		t.Error("focusedSlot() should be the anchored slot")
	}

	// Up from anchored, the first row, stays.
	m = m.cycleSlotCursor(-1)
	if m.cursorIdx != 0 {
		t.Errorf("after ↑ from anchored: cursorIdx = %d, want 0 (it stops)", m.cursorIdx)
	}

	// Down returns to floating.
	m = m.cycleSlotCursor(+1)
	if m.cursorIdx != 1 {
		t.Errorf("after ↓ from anchored: cursorIdx = %d, want 1 (floating)", m.cursorIdx)
	}
}

// TestSlotCycleKeyBindings_DispatchToHandler asserts that the ↑ and ↓
// keys in main mode reach cycleSlotCursor (smoke test for the binding
// wiring, not just the helper). Uses a two-slot workspace so the cursor
// actually moves and we can observe it.
func TestSlotCycleKeyBindings_DispatchToHandler(t *testing.T) {
	m := NewModelWithClock(Config{}, FrozenClock(testFrozenTime()))
	{
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
		m = updated.(Model)
	}
	slots, err := store.NewSlots(testFrozenTime(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	if err := addAnchor(slots, time.Date(1990, 1, 1, 14, 0, 0, 0, time.UTC), testPlaceAmsterdam()); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}
	m.slots = slots
	m.cursorIdx = 1

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	mm := updated.(Model)
	if mm.cursorIdx != 0 {
		t.Errorf("after KeyUp: cursorIdx = %d, want 0", mm.cursorIdx)
	}

	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyDown})
	mm = updated.(Model)
	if mm.cursorIdx != 1 {
		t.Errorf("after KeyDown: cursorIdx = %d, want 1", mm.cursorIdx)
	}
}

func testFrozenTime() time.Time {
	return time.Date(2026, 4, 25, 14, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
}

func testPlaceBerlin() location.PlaceInfo {
	return location.PlaceInfo{
		Name:    "Berlin",
		Country: "DE",
		Lat:     52.5244,
		Lon:     13.4105,
		TZ:      "Europe/Berlin",
	}
}

func testPlaceAmsterdam() location.PlaceInfo {
	return location.PlaceInfo{
		Name:    "Amsterdam",
		Country: "NL",
		Lat:     52.3676,
		Lon:     4.9041,
		TZ:      "Europe/Amsterdam",
	}
}

// Eight charts at 80x24 (window of five): the list moves only when the cursor
// would leave it, in either direction.
func TestSlotCursorUpWalksBeforeScrolling(t *testing.T) {
	m := listModel(t, 7, 0)
	m.width, m.height = 80, 24
	for i := 0; i < 7; i++ {
		m = send(t, m, keyDown)
	}
	if m.cursorIdx != 7 || m.chartTop != 3 {
		t.Fatalf("after walking down: cursor %d top %d, want 7 and 3", m.cursorIdx, m.chartTop)
	}
	for i := 1; i <= 4; i++ {
		m = send(t, m, keyUp)
		if m.chartTop != 3 {
			t.Fatalf("up %d (cursor %d): chartTop %d, want 3 (the list must not move)", i, m.cursorIdx, m.chartTop)
		}
	}
	m = send(t, m, keyUp)
	if m.chartTop != 2 {
		t.Errorf("fifth up (cursor %d): chartTop %d, want 2", m.cursorIdx, m.chartTop)
	}
}

func TestSlotCursorDownWalksBeforeScrolling(t *testing.T) {
	m := listModel(t, 7, 0)
	m.width, m.height = 80, 24
	for i := 1; i <= 4; i++ {
		m = send(t, m, keyDown)
		if m.chartTop != 0 {
			t.Fatalf("down %d (cursor %d): chartTop %d, want 0 (the list must not move)", i, m.cursorIdx, m.chartTop)
		}
	}
	m = send(t, m, keyDown)
	if m.chartTop != 1 {
		t.Errorf("fifth down (cursor %d): chartTop %d, want 1", m.cursorIdx, m.chartTop)
	}
}
