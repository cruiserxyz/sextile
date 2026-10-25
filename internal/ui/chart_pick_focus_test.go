package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/store"
)

// successfulChartPickFocusModel drives the picker through a successful resolve
// without making the test depend on a chart-library filesystem read. The
// resolve message is the boundary where the anchor and slot focus are allowed
// to change.
func successfulChartPickFocusModel(t *testing.T) Model {
	t.Helper()
	m := readyModelWithSlots(t, berlinP)
	m.width, m.height = 120, 40
	m.startup = startupChartOpen
	m.picker = chartPickerState{loaded: true, prior: startupReady}
	chart := testNamedChart("Focused Client", fixtureOneP)
	picked, resolve := applyChartPicked(m, ChartPickedMsg{Chart: chart})
	if resolve == nil {
		t.Fatal("C1 precondition: pick dispatched no resolve")
	}
	got, recompute := applyChartResolved(picked, ChartResolvedMsg{
		Chart: chart,
		When:  time.Date(2001, 3, 14, 11, 30, 0, 0, time.UTC),
		Gen:   picked.pickGen,
	})
	if recompute == nil {
		t.Fatal("C1 precondition: resolved pick dispatched no recompute")
	}
	return got
}

// T1 C1-C3: a completed pick closes its picker, focuses the anchor directly,
// and makes subsequent slot-sensitive keys act on that anchor.
func TestChartPickFocusesAnchoredSlot(t *testing.T) {
	t.Run("C1 close", func(t *testing.T) {
		got := successfulChartPickFocusModel(t)
		if got.startup == startupChartOpen {
			t.Fatal("successful pick left the chart picker open")
		}
		if view := got.View(); strings.Contains(view, "Enter: Set anchor") {
			t.Fatalf("successful pick left a picker overlay in the next View:\n%s", view)
		}
	})

	t.Run("C2 focus", func(t *testing.T) {
		got := successfulChartPickFocusModel(t)
		if firstAnchor(got.slots) == nil {
			t.Fatal("precondition: successful pick created no anchored slot")
		}
		if got.cursorIdx != 0 {
			t.Errorf("focused slot index = %d, want 0 (the anchored slot)", got.cursorIdx)
		}
	})

	t.Run("C3 next command acts on it", func(t *testing.T) {
		got := successfulChartPickFocusModel(t)
		updated, _ := got.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
		afterSave := updated.(Model)
		if afterSave.startup == startupSaveOpen {
			t.Fatal("Ctrl+S opened the save prompt after loading an anchored slot")
		}
		// Saving acts on the floating slot only; on the anchored slot the
		// key does nothing and says nothing (the specification B1).
		if len(afterSave.logLines) != len(got.logLines) {
			t.Errorf("Ctrl+S on the anchored slot wrote %v", afterSave.logPanelLines()[len(got.logPanelLines()):])
		}

		if !got.slots.SlotSelected(0) || !got.slots.SlotSelected(1) {
			t.Fatal("precondition: a successful pick selects the anchored and floating slots")
		}
		got.focused = PanelChart
		updated, _ = got.Update(tea.KeyMsg{Type: tea.KeySpace})
		afterSpace := updated.(Model)
		if afterSpace.slots.SlotSelected(0) {
			t.Error("Space did not toggle selection on the focused anchored slot")
		}
		if !afterSpace.slots.SlotSelected(1) {
			t.Error("Space toggled the floating slot instead of leaving it selected")
		}
	})
}

// T1 C4-C6: only a successful resolve may move focus. Cancelled, stale, and
// failed resolves preserve both the pre-existing anchor and focused slot.
func TestChartPickFocusGuards(t *testing.T) {
	t.Run("C4 cancel", func(t *testing.T) {
		m := readyModelWithSlots(t, berlinP)
		m.startup = startupChartOpen
		m.picker = chartPickerState{loaded: true, prior: startupReady}
		picked, _ := applyChartPicked(m, ChartPickedMsg{Chart: testNamedChart("Cancelled", fixtureOneP)})
		staleGen := picked.pickGen

		updated, _ := updateChartModalKey(picked, tea.KeyMsg{Type: tea.KeyEsc})
		got := updated.(Model)
		if got.cursorIdx != 0 || firstAnchor(got.slots) != nil {
			t.Fatalf("Esc changed focus or anchor: cursor=%d anchor=%+v", got.cursorIdx, firstAnchor(got.slots))
		}
		got, cmd := applyChartResolved(got, ChartResolvedMsg{
			Chart: testNamedChart("Cancelled", fixtureOneP),
			When:  time.Date(2001, 3, 14, 11, 30, 0, 0, time.UTC),
			Gen:   staleGen,
		})
		if cmd != nil || got.cursorIdx != 0 || firstAnchor(got.slots) != nil {
			t.Fatalf("cancelled resolve changed focus or anchor: cursor=%d anchor=%+v", got.cursorIdx, firstAnchor(got.slots))
		}
	})

	t.Run("C5 stale completion", func(t *testing.T) {
		m := readyModelWithSlots(t, berlinP)
		kept := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
		if err := addAnchor(m.slots, kept, fixtureOneP); err != nil {
			t.Fatal(err)
		}
		m.cursorIdx = 1 // preserve an existing floating focus through the stale message.

		first := testNamedChart("First", fixtureOneP)
		second := testNamedChart("Second", berlinP)
		m, _ = applyChartPicked(m, ChartPickedMsg{Chart: first})
		staleGen := m.pickGen
		m, _ = applyChartPicked(m, ChartPickedMsg{Chart: second})
		got, cmd := applyChartResolved(m, ChartResolvedMsg{
			Chart: first,
			When:  time.Date(2001, 3, 14, 11, 30, 0, 0, time.UTC),
			Gen:   staleGen,
		})
		if cmd != nil {
			t.Error("a stale completion dispatched a recompute")
		}
		if got.cursorIdx != 1 {
			t.Errorf("stale completion changed focus to index %d, want preserved floating index 1", got.cursorIdx)
		}
		anchor := firstAnchor(got.slots)
		if anchor == nil || !anchor.Time.Equal(kept) || anchor.Location != fixtureOneP {
			t.Errorf("stale completion changed the existing anchor: %+v", anchor)
		}
	})

	t.Run("C6 failure", func(t *testing.T) {
		m := readyModelWithSlots(t, berlinP)
		kept := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
		if err := addAnchor(m.slots, kept, fixtureOneP); err != nil {
			t.Fatal(err)
		}
		m.cursorIdx = 1
		picked, _ := applyChartPicked(m, ChartPickedMsg{Chart: testNamedChart("Broken", berlinP)})
		got, cmd := applyChartResolved(picked, ChartResolvedMsg{
			Chart: picked.picker.pending,
			Err:   errors.New("unreadable chart"),
			Gen:   picked.pickGen,
		})
		if cmd != nil {
			t.Error("a failed completion dispatched a recompute")
		}
		if got.cursorIdx != 1 {
			t.Errorf("failed completion changed focus to index %d, want preserved floating index 1", got.cursorIdx)
		}
		anchor := firstAnchor(got.slots)
		if anchor == nil || !anchor.Time.Equal(kept) || anchor.Location != fixtureOneP {
			t.Errorf("failed completion changed the existing anchor: %+v", anchor)
		}
		if got.picker.resolveErr == nil {
			t.Error("failed completion did not retain its error for the picker")
		}
	})
}

// Opening a chart from another panel hands the keyboard to the Charts panel,
// on the slot it opened: the operator's next ↑/↓ or Space acts on the chart
// they just picked, not on whatever panel the picker was opened over. Both
// resolve paths are covered — a new anchor, and a chart already in the list.
func TestChartPickFocusesChartsPanel(t *testing.T) {
	pick := func(t *testing.T, m Model, chart store.NamedChart) Model {
		t.Helper()
		m.startup = startupChartOpen
		m.picker = chartPickerState{loaded: true, prior: startupReady}
		picked, resolve := applyChartPicked(m, ChartPickedMsg{Chart: chart})
		if resolve == nil {
			t.Fatal("precondition: pick dispatched no resolve")
		}
		got, _ := applyChartResolved(picked, ChartResolvedMsg{
			Chart: chart,
			When:  time.Date(2001, 3, 14, 11, 30, 0, 0, time.UTC),
			Gen:   picked.pickGen,
		})
		return got
	}
	chart := testNamedChart("Focused Client", fixtureOneP)

	for _, from := range []PanelID{PanelBodies, PanelAspects} {
		t.Run("new chart from "+string(from), func(t *testing.T) {
			m := readyModelWithSlots(t, berlinP)
			m.width, m.height = 120, 40
			m.focused = from
			got := pick(t, m, chart)
			if got.focused != PanelChart {
				t.Errorf("focused panel = %v after opening a chart, want %v", got.focused, PanelChart)
			}
			if got.cursorIdx != 0 {
				t.Errorf("focused slot = %d, want 0 (the opened chart)", got.cursorIdx)
			}
		})
	}

	t.Run("chart already open", func(t *testing.T) {
		m := readyModelWithSlots(t, berlinP)
		m.width, m.height = 120, 40
		m = pick(t, m, chart)
		m = m.focusSlot(len(m.slots.All()) - 1)
		m.focused = PanelAspects
		got := pick(t, m, chart)
		if got.focused != PanelChart {
			t.Errorf("focused panel = %v after re-opening an open chart, want %v", got.focused, PanelChart)
		}
		if got.cursorIdx != 0 {
			t.Errorf("focused slot = %d, want 0 (the re-opened chart)", got.cursorIdx)
		}
	})

	t.Run("chart already open under the cursor", func(t *testing.T) {
		m := readyModelWithSlots(t, berlinP)
		m.width, m.height = 120, 40
		m = pick(t, m, chart)
		m.focused = PanelBodies
		got := pick(t, m, chart)
		if got.focused != PanelChart {
			t.Errorf("focused panel = %v after re-opening the focused chart, want %v", got.focused, PanelChart)
		}
	})
}
