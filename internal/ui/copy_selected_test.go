package ui

import (
	"strings"
	"testing"
)

// copyChartModel is a Chart panel of four slots (three charts and the floating
// one), none selected, with the clipboard captured.
func copyChartModel(t *testing.T) (Model, *[]string) {
	t.Helper()
	copied := captureYank(t)
	m := copyModel(t, false)
	m.slots = listWorkspace(t, 3)
	for i := range m.slots.All() {
		m.slots.SetSlotSelected(i, false)
	}
	m.focused = PanelChart
	return m, copied
}

func copiedLines(t *testing.T, copied *[]string) []string {
	t.Helper()
	if len(*copied) != 1 {
		t.Fatalf("copied %d times, want once: %q", len(*copied), *copied)
	}
	return strings.Split((*copied)[0], "\n")
}

// `,y` in the Chart panel copies the selected charts, in slot order.
func TestCopyChartsCopiesOnlyTheSelected(t *testing.T) {
	m, copied := copyChartModel(t)
	m.slots.SetSlotSelected(0, true)
	m.slots.SetSlotSelected(2, true)
	m, _ = press(t, m, ",", "y")
	got := copiedLines(t, copied)
	if len(got) != 2 || !strings.Contains(got[0], "Synthetic 1") || !strings.Contains(got[1], "Synthetic 3") {
		t.Errorf("copied %q, want the rows of Synthetic 1 and Synthetic 3", got)
	}
	if n := logHas(m, "chart: 2 rows copied"); n != 1 {
		t.Errorf("the Log says it %d times, want once: %v", n, logTexts(m))
	}
}

// With none selected every row goes, as before.
func TestCopyChartsCopiesEveryRowWhenNoneIsSelected(t *testing.T) {
	m, copied := copyChartModel(t)
	m, _ = press(t, m, ",", "y")
	if got := copiedLines(t, copied); len(got) != 4 {
		t.Errorf("copied %d rows, want all 4: %q", len(got), got)
	}
	if n := logHas(m, "chart: 4 rows copied"); n != 1 {
		t.Errorf("the Log lacks the count: %v", logTexts(m))
	}
}

// The floating slot, selected alone, is the only row.
func TestCopyChartsFloatingSlotAlone(t *testing.T) {
	m, copied := copyChartModel(t)
	m.slots.SetSlotSelected(m.slots.AnchoredCount(), true)
	m, _ = press(t, m, ",", "y")
	got := copiedLines(t, copied)
	if len(got) != 1 || strings.Contains(got[0], "Synthetic") {
		t.Errorf("copied %q, want only the floating row", got)
	}
	if n := logHas(m, "chart: 1 row copied"); n != 1 {
		t.Errorf("the Log lacks the count: %v", logTexts(m))
	}
}
