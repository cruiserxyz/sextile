package ui

import (
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/lookup"
)

// T2: whichever chart the cursor is on, the bold row is the floating chart's
// moment. A step key moves the floating moment and the bold row follows it;
// Enter on a row of the chart under the cursor moves the floating time and the
// cursor goes to now, read in that chart.
func TestTimelineBoldRowIsTheFloatingMoment(t *testing.T) {
	forceColor(t)
	stubTextCompute(t)
	rows := enteredRows(t)
	stubTimeline(t, func(lookup.TimelineRequest) (lookup.TimelineResult, error) {
		return lookup.TimelineResult{Rows: rows, Commands: []string{"astrolog -dm"}}, nil
	})
	m := timelineModel(t, Config{Accessible: true})
	// ① is the 1979 natal; ② is born in 1985, before the listed month.
	if err := addAnchor(m.slots, time.Date(1985, 3, 2, 8, 0, 0, 0, time.UTC), testPlaceAmsterdam()); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}
	for i := range m.slots.All() {
		m.slots.SetSlotSelected(i, true)
	}
	m.cursorIdx = 1 // the 1985 chart
	m.width = max(m.width, dualColumnMinWidth(m.shownCount()))
	m = m.noteArrival()
	m, zero := tpress(t, m, runes("0"))
	m, cmd := syncTimelineVisited(m)
	m = pump(t, m, zero, cmd)
	if !m.timeline.complete || m.focusedIsFloating() || m.activeTabID(PanelDetails) != TabDetailsTimeline {
		t.Fatalf("fixture: complete=%v floating=%v tab=%s", m.timeline.complete, m.focusedIsFloating(), m.activeTabID(PanelDetails))
	}
	boldIsNearest := func(what string) {
		t.Helper()
		want := nearestTimelineRow(m.timelineRows(), m.floatingTime())
		bold, _ := boldRows(m)
		switch {
		case want < 0 && len(bold) != 0:
			t.Errorf("%s: bold rows %v, want none", what, bold)
		case want >= 0 && (len(bold) != 1 || bold[0] != want-m.details.rowOffset):
			t.Errorf("%s: bold rows %v, want the floating moment's row %d (offset %d)", what, bold, want, m.details.rowOffset)
		}
	}
	// The cursor on the 1985 chart: its own moment is outside the list, and the
	// bold row is still the floating moment's.
	if want := nearestTimelineRow(m.timelineRows(), m.floatingTime()); want < 0 {
		t.Fatal("fixture: the floating moment has no row")
	}
	boldIsNearest("cursor on the 1985 chart")

	// A step key on the floating chart moves it, and the bold row follows.
	m, c1 := tpress(t, m, keyDown)
	m = pump(t, m, c1)
	if !m.focusedIsFloating() {
		t.Fatal("fixture: the cursor is not on the floating slot")
	}
	was := m.floatingTime()
	m, c2 := tpress(t, m, runes("d"))
	m = pump(t, m, c2)
	if !m.floatingTime().After(was) {
		t.Fatalf("`d` left the floating moment at %v", m.floatingTime())
	}
	boldIsNearest("after `d` on the floating chart")

	// Back to the 1985 chart: Enter opens the list, Enter on the row of its own
	// transit moves the floating time and the cursor goes to now, read in it.
	m, c3 := tpress(t, m, keyUp)
	m = pump(t, m, c3)
	m, _ = tpress(t, m, keyEnter)
	if !m.details.attached || m.activeTabID(PanelDetails) != TabDetailsTimeline {
		t.Fatalf("fixture: attached=%v tab=%s", m.details.attached, m.activeTabID(PanelDetails))
	}
	i := chartRowOf(t, m.timelineRows(), 2)
	row := m.timelineRows()[i]
	selected := m.slots.SelectedCount()
	m = enter(t, m, i)
	if got := m.floatingTime(); !got.Equal(landedAt(row.Instant)) {
		t.Errorf("Enter moved the floating moment to %v, want the row's %v", got, landedAt(row.Instant))
	}
	if !m.focusedIsFloating() || m.activePairSlot() == nil || m.activePairSlot().left != m.slots.All()[1] {
		t.Errorf("Enter left the cursor at %d (floating: %v), want it on now read in the 1985 chart", m.cursorIdx, m.focusedIsFloating())
	}
	if m.slots.SelectedCount() != selected {
		t.Errorf("Enter changed the selection: %d selected, was %d", m.slots.SelectedCount(), selected)
	}
	m, _ = tpress(t, m, keyEsc)
	boldIsNearest("after Enter, detached")
	if bold, _ := boldRows(m); len(bold) != 1 || bold[0] != i-m.details.rowOffset {
		t.Errorf("bold rows %v after Enter, want the pressed row %d (offset %d)", bold, i, m.details.rowOffset)
	}
}
