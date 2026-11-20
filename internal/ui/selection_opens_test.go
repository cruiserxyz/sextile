package ui

import "testing"

// Selecting a second chart opens their comparison: with no base stored, the
// chart selected before becomes the base, so the Aspects tab and the wheel
// read the cursor's chart in it. Now is never a base.
func TestSelectingASecondChartOpensTheComparison(t *testing.T) {
	fresh := func(t *testing.T, anchored int) Model {
		t.Helper()
		m := listModel(t, anchored, 0)
		for i := range m.slots.All() {
			m.slots.SetSlotSelected(i, false)
		}
		m.base, m.lastSelected = nil, nil
		m.focused = PanelChart
		return m
	}
	space := func(t *testing.T, m Model, row int) Model {
		t.Helper()
		m.cursorIdx = row
		m = m.noteArrival()
		m, _ = m.toggleFocusedSelection()
		return m
	}
	t.Run("a chart, then Now", func(t *testing.T) {
		m := fresh(t, 1)
		m = space(t, m, 0)
		m = space(t, m, 1)
		p := m.activePairSlot()
		if p == nil || p.left != m.slots.All()[0] || p.right != m.slots.All()[1] {
			t.Fatalf("after selecting the chart and Now the tab is %+v, want Now read in the chart", p)
		}
		if plan, ok := m.wheelPlan(); !ok || plan.mode != wheelPair {
			t.Errorf("the wheel would draw mode %v, want the bi-wheel", plan.mode)
		}
	})
	t.Run("two charts", func(t *testing.T) {
		m := fresh(t, 2)
		m = space(t, m, 1)
		m = space(t, m, 0)
		if p := m.activePairSlot(); p == nil || p.left != m.slots.All()[1] || p.right != m.slots.All()[0] {
			t.Fatalf("the tab is %+v, want the first chart read in the one selected before", p)
		}
	})
	t.Run("Now, then a chart: Now is never a base", func(t *testing.T) {
		m := fresh(t, 1)
		m = space(t, m, 1)
		m = space(t, m, 0)
		if p := m.activePairSlot(); p != nil {
			t.Errorf("Now became a base: %+v", p)
		}
	})
	t.Run("a stored base stays", func(t *testing.T) {
		m := fresh(t, 3)
		m = space(t, m, 0)
		m = space(t, m, 1)
		m = space(t, m, 2)
		if p := m.activePairSlot(); p == nil || p.left != m.slots.All()[0] {
			t.Errorf("the third selection moved the stored base: %+v", p)
		}
	})
}
