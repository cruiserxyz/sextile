package ui

import (
	"testing"

	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Enter on a Timeline row whose body sits below the Bodies window scrolls the
// list so the led row is in the middle of the panel, not at its edge (operator,
// 2026-11-06: "in bodies we just need to make sure if the list is long, it
// scrolls there centered"). A row already in sight does not move the list.
func TestTimelineLeadCentresAFarRow(t *testing.T) {
	var extra []aspect.Position
	for n := 34; n < 80; n++ {
		extra = append(extra, aspect.Position{Object: fixtureObject(t, n), Longitude: float64(n)})
	}
	m := sameOrderFixtureWith(t, Config{Color: ColorNever}, extra...)
	planets, under, height := m.bodiesRows()
	if planets+under < height+height/2 {
		t.Fatalf("fixture: %d rows in a window of %d, want a long list", planets+under, height)
	}
	far := rowOfObject(t, m, 60)
	if first, last := panel.BodyDrawnRows(planets, under, height, m.bodyTop); far >= first && far <= last {
		t.Fatalf("fixture: row %d is already drawn (%d-%d)", far, first, last)
	}
	i := shownRowWith(t, m, "Moon (Sco) --> Sagittarius")
	m.timeline.rows[i].Object = fixtureObject(t, 60)
	m = enter(t, m, i)
	if m.bodyCursorIdx != far {
		t.Fatalf("the Bodies cursor is on %d, want the led row %d", m.bodyCursorIdx, far)
	}
	first, last := panel.BodyDrawnRows(planets, under, height, m.bodyTop)
	if above, below := far-first, last-far; above-below > 1 || below-above > 1 {
		t.Errorf("the led row is drawn %d rows from the top and %d from the bottom (rows %d-%d), want it in the middle", above, below, first, last)
	}

	// A row already in sight leaves the window where it is.
	top := m.bodyTop
	near := rowOfObject(t, m, 58)
	m.timeline.rows[i].Object = fixtureObject(t, 58)
	m = enter(t, m, i)
	if m.bodyCursorIdx != near || m.bodyTop != top {
		t.Errorf("Enter on a row in sight: cursor %d (want %d), window from %d (was %d)", m.bodyCursorIdx, near, m.bodyTop, top)
	}
}
