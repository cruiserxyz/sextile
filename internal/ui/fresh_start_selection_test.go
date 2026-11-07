package ui

import (
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/store"
)

// Specification: specs/fresh-start-selection.md. While only the floating slot
// is open there is nothing to compare, so nothing can be selected.

func TestAFreshStartHasNothingSelected(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	if m.slots.SelectedCount() != 0 || m.slots.SlotSelected(0) {
		t.Errorf("a fresh start has %d selected, want none", m.slots.SelectedCount())
	}
}

func TestSpaceSelectsNothingWhileOnlyTheFloatingSlotIsOpen(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	logged := len(m.logLines)
	tm, cmd := m.Update(keySpace)
	got := tm.(Model)
	if got.slots.SlotSelected(0) || got.slots.SelectedCount() != 0 {
		t.Error("Space selected the floating slot with no chart open")
	}
	if cmd != nil {
		t.Error("Space on the lone floating slot queued work (a refresh)")
	}
	if len(got.logLines) != logged {
		t.Errorf("Space on the lone floating slot wrote to the Log: %v", got.logLines[logged:])
	}
}

func TestKeyBarOffersSelectOnlyWithAChartOpen(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	if bar := barKeys(m); strings.Contains(bar, "Select") {
		t.Errorf("the bar offers Select with only the floating slot open: %q", bar)
	}
	m = listModel(t, 1, 0)
	if bar := barKeys(m); !strings.Contains(bar, "Select") {
		t.Errorf("the bar hides Select with a chart open: %q", bar)
	}
}

func TestSpaceSelectsAnySlotOnceAChartIsOpen(t *testing.T) {
	m := listModel(t, 1, 1) // cursor on the floating slot
	m.slots.SetSlotSelected(0, false)
	m.slots.SetSlotSelected(1, false)
	m.cursorIdx = 1
	if got := send(t, m, keySpace); !got.slots.SlotSelected(1) {
		t.Error("Space did not select the floating slot beside an open chart")
	}
	m.cursorIdx = 0
	if got := send(t, m, keySpace); !got.slots.SlotSelected(0) {
		t.Error("Space did not select the open chart")
	}
}

func TestClosingTheLastChartLeavesAnExistingSelection(t *testing.T) {
	m := listModel(t, 1, 1)
	m.slots.SetSlotSelected(1, true)
	if !m.slots.RemoveAnchored(0) {
		t.Fatal("remove refused")
	}
	if !m.slots.SlotSelected(m.slots.AnchoredCount()) {
		t.Error("closing the last chart cleared the floating selection")
	}
}

func TestARestoredLoneFloatingSelectionIsDropped(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	m = m.applySessionSelections(ingest.StartupPerimeterReadMsg{HasSession: true, Session: store.Session{
		Version: 2, FloatingSelected: true,
	}})
	if m.slots.SlotSelected(0) {
		t.Error("a session that saved a lone floating selection restored it")
	}
}
