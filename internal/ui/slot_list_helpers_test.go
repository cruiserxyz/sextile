package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// addAnchor adds an anchored slot holding no named chart: the old
// Slots.WithAnchor for a test that needs a moment and a place and nothing
// else. With no anchored slot selected it selects the new slot and the
// floating one, as WithAnchor did. The store no longer does this on its own
// (opening a chart selects nothing), so the helper states it for the tests
// that were written for a ready comparison.
func addAnchor(s *store.Slots, when time.Time, place location.PlaceInfo) error {
	first := s.Baseline() == nil
	if err := s.AddAnchored(store.SessionChart{}, when, place); err != nil {
		return err
	}
	if first {
		s.SetSlotSelected(s.AnchoredCount()-1, true)
		s.SetSlotSelected(s.AnchoredCount(), true)
	}
	return nil
}

// firstAnchor is the first anchored slot, or nil: what Slots.Anchored() was
// while the workspace held at most one.
func firstAnchor(s *store.Slots) *store.Slot {
	if s == nil || s.AnchoredCount() == 0 {
		return nil
	}
	return s.All()[0]
}

// v1 is the session a version-1 file reads as: at most one chart — chart nil
// for none — whose selection is the pair's (comparison), the bodies marked in
// it and in the floating slot, and any body names the reader could not parse.
// It exists so the restore tests written for one-chart sessions state their
// input in the terms they always did (the specification C15).
func v1(chart *store.SessionChart, usable, comparison bool, anchored, floating []aspect.Body, unknown ...string) store.Session {
	s := store.Session{Version: 1, Folder: testFolderID, FloatingBodies: floating, UnknownBodies: unknown}
	if chart != nil {
		s.Slots = []store.SessionSlot{{Chart: *chart, Usable: usable, Selected: comparison, Bodies: anchored}}
		s.FloatingSelected = usable && comparison
	}
	return s
}

// anchorHandle is the first anchored slot's moment and place, or the zero
// handle: what anchorHandle(Model) held while the workspace had one anchor.
func anchorHandle(m Model) store.AnchorChart { return anchorOf(firstAnchor(m.slots)) }

// anchorRecord is the first anchored slot's chart record with the slot's own
// moment and place: what anchorRecord(Model) held while the workspace had one
// anchor. Zero when nothing is anchored.
func anchorRecord(m Model) store.SessionChart {
	a := firstAnchor(m.slots)
	if a == nil {
		return store.SessionChart{}
	}
	rec := a.Chart
	rec.When, rec.Place = a.Time, a.Location
	return rec
}

// setAnchor moves the first anchored slot to handle's moment and place: the
// test-side equivalent of assigning Model.chart.lastChart while the workspace
// had one anchor. It does nothing with no anchored slot.
func setAnchor(m Model, handle store.AnchorChart) {
	if a := firstAnchor(m.slots); a != nil {
		a.Time, a.Location = handle.When, handle.Place
	}
}

// listWorkspace is a workspace of n anchored charts with distinct synthetic
// names, moments and places, above the floating slot at the frozen test moment.
// With n >= 1 the first anchored slot and the floating slot are selected
// explicitly here, because adding a chart selects nothing.
func listWorkspace(t *testing.T, n int) *store.Slots {
	t.Helper()
	slots, err := store.NewSlots(testFrozenTime(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	for i := 1; i <= n; i++ {
		chart := store.SessionChart{
			ID:     fmt.Sprintf("synthetic-%d", i),
			Name:   fmt.Sprintf("Synthetic %d", i),
			Source: store.ChartSource{Kind: store.ChartKindFile, File: fmt.Sprintf("synthetic-%d.as", i)},
		}
		when := time.Date(1960+i, time.Month(i), 10+i, 6, i, 0, 0, time.UTC)
		place := location.PlaceInfo{Name: fmt.Sprintf("Testford %d", i), Country: "TS", Lat: 40 + float64(i), Lon: -70 - float64(i), TZ: "America/New_York"}
		if err := slots.AddAnchored(chart, when, place); err != nil {
			t.Fatalf("AddAnchored(%d): %v", i, err)
		}
	}
	if n >= 1 { // adding selects nothing: select the first anchored slot and the floating slot
		slots.ToggleSlotSelection(0)
		slots.ToggleSlotSelection(n)
	}
	return slots
}

// focusedRecord is the chart record of the anchored slot under the cursor — the
// one a pick adds and focuses (the specification C3) — with the slot's own
// moment and place. Zero when the cursor is on the floating slot.
func focusedRecord(m Model) store.SessionChart {
	if !m.anchoredFocused() {
		return store.SessionChart{}
	}
	a := m.focusedSlot()
	rec := a.Chart
	rec.When, rec.Place = a.Time, a.Location
	return rec
}

// floatingIndex is the floating slot's position in All() order — always last,
// with or without an anchor.
func floatingIndex(slots *store.Slots) int {
	return len(slots.All()) - 1
}

// cloneSlots is a workspace equal to s that shares nothing with it, for a test
// that presses keys which change the list (del, ⌃k, ⌃j) against states built
// from one base: Model copies share their *Slots.
func cloneSlots(t *testing.T, s *store.Slots) *store.Slots {
	t.Helper()
	if s == nil {
		return nil
	}
	f := s.Floating()
	c, err := store.NewSlots(f.Time, f.Location)
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range s.All()[:s.AnchoredCount()] {
		if err := c.AddAnchored(slot.Chart, slot.Time, slot.Location); err != nil {
			t.Fatal(err)
		}
	}
	c.CopySelectionsFrom(s)
	return c
}
