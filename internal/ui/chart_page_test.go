package ui

import (
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// chartPageModel is a ready Chart panel with 12 slots (eleven charts and the
// floating one) at 144x40, where the panel shows 7 rows, focused on the first.
func chartPageModel(t *testing.T) Model {
	t.Helper()
	m := detailsTestModel(t)
	m.slots = listWorkspace(t, 11)
	m.width = dualColumnMinWidth(2) // two charts are selected, so Details needs the wider layout
	m.focused = PanelChart
	m.cursorIdx, m.chartTop = 0, 0
	m.chart.positions = []aspect.Position{{Body: aspect.Sun}, {Body: aspect.Moon}}
	if n := len(m.slots.All()); n != 12 {
		t.Fatalf("fixture: %d slots, want 12", n)
	}
	if h := m.geometry()[PanelChart].h - 2; h != 7 {
		t.Fatalf("fixture: the Chart panel shows %d rows, want 7", h)
	}
	return m
}

func pageKey(t *testing.T, m Model, k string) Model {
	t.Helper()
	next, _ := press(t, m, k)
	return next
}

// A3: PgUp/PgDn/Home/End/g/G move the Chart panel's cursor by the one rule:
// first to the edge of the window, then by a page of the window minus a row.
func TestChartPanelPagesTheCursor(t *testing.T) {
	m := chartPageModel(t)
	step := func(name, key string, wantCursor, wantTop int) {
		t.Helper()
		m = pageKey(t, m, key)
		if m.cursorIdx != wantCursor || m.chartTop != wantTop {
			t.Fatalf("%s: cursor %d chartTop %d, want %d / %d", name, m.cursorIdx, m.chartTop, wantCursor, wantTop)
		}
		if m.focusedSlot() != m.slots.All()[m.cursorIdx] {
			t.Errorf("%s: the focused slot did not follow the cursor", name)
		}
		// chartWindow finds nothing to correct: the stored window is the drawn one.
		if start, _ := chartWindow(len(m.slots.All()), m.cursorIdx, 7, m.chartTop); start != m.chartTop {
			t.Errorf("%s: chartWindow corrected chartTop %d to %d", name, m.chartTop, start)
		}
	}
	step("PgDn from the top goes to the window's last row", "pgdown", 6, 0)
	if m.chart.positions != nil {
		t.Error("PgDn did not refresh Bodies for the newly focused slot")
	}
	step("PgDn again pages the window", "pgdown", 11, 5)
	step("PgUp goes to the window's first row", "pgup", 5, 5)
	step("PgUp again pages the window back", "pgup", 0, 0)
	step("End", "end", 11, 5)
	step("Home", "home", 0, 0)
	step("G", "G", 11, 5)
	step("g", "g", 0, 0)
}

// A3: with ONE slot the Chart panel owns PgDn and it does nothing; Details does
// not scroll. With NO slot there is no list, and PgDn scrolls Details as before.
func TestChartPagingFallsThroughOnlyWithoutAList(t *testing.T) {
	one := scrollModel(t, 0)
	one.focused = PanelChart
	if n := len(one.slots.All()); n != 1 {
		t.Fatalf("fixture: %d slots, want 1", n)
	}
	for _, k := range []string{"pgdown", "pgup", "end", "home"} {
		got := pageKey(t, one, k)
		if got.cursorIdx != 0 || got.details.scrollOffset != 0 {
			t.Errorf("one slot, %s: cursor %d, Details scrolled to %d; the press must be a no-op", k, got.cursorIdx, got.details.scrollOffset)
		}
	}
	none := scrollModel(t, 0)
	none.focused = PanelChart
	none.slots = nil
	got := pageKey(t, none, "pgdown")
	if got.details.scrollOffset != detailsScrollStep {
		t.Errorf("no slot: PgDn left Details at %d, want it scrolled by %d", got.details.scrollOffset, detailsScrollStep)
	}
	// Aspects owns no cursor: PgDn scrolls Details exactly as before.
	aspects := scrollModel(t, 0)
	aspects.focused = PanelAspects
	if got := pageKey(t, aspects, "pgdown"); got.details.scrollOffset != detailsScrollStep {
		t.Errorf("Aspects focus: PgDn left Details at %d, want %d", got.details.scrollOffset, detailsScrollStep)
	}
	// Ctrl+PgDn scrolls Details from the Chart panel, whatever the list.
	m := chartPageModel(t)
	m.details.tabs = map[PanelID]TabID{PanelChart: TabDetailsReport}
	m.details.title, m.details.body = "[0] Chart", influenceReport(t)
	m = pageKey(t, m, "ctrl+pgdown")
	if m.details.scrollOffset != detailsScrollStep || m.cursorIdx != 0 {
		t.Errorf("Ctrl+PgDn: Details at %d, cursor %d; want it to scroll Details and leave the cursor", m.details.scrollOffset, m.cursorIdx)
	}
}

// Decision 2: the configured leader is consumed before any table, so a leader of
// g keeps the leader and loses the g jump; Home and End always work, and G (not
// the leader) still jumps.
func TestLeaderOfGKeepsTheLeader(t *testing.T) {
	m := chartPageModel(t)
	m.leaderKey = "g"
	m.cursorIdx, m.chartTop = 5, 0
	m = pageKey(t, m, "g")
	if !m.leader.pending || m.cursorIdx != 5 {
		t.Errorf("g with a leader of g: pending=%v cursor %d, want the leader armed and the cursor where it was", m.leader.pending, m.cursorIdx)
	}
	m = pageKey(t, m, "esc")
	if m.leader.pending {
		t.Fatal("fixture: Esc did not cancel the leader")
	}
	m = pageKey(t, m, "home")
	if m.cursorIdx != 0 {
		t.Errorf("Home with a leader of g: cursor %d, want 0", m.cursorIdx)
	}
	m = pageKey(t, m, "G")
	if m.cursorIdx != 11 {
		t.Errorf("G with a leader of g: cursor %d, want the last row", m.cursorIdx)
	}
}
