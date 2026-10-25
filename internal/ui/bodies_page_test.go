package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// bodiesPageModel is a ready Bodies panel with the full test chart: 14 planets,
// the rule and 12 cusp rows, in a panel `inner` rows tall. The model has the
// frame committed (stampShown) so Bodies draws from the live chart.
func bodiesPageModel(t *testing.T, inner int) Model {
	t.Helper()
	m := detailsTestModel(t)
	m.height = inner + 12 // the panel's body is the terminal's height less 12 at this width
	m.chart.positions = withTicked(fullChartPositions(), fullChartCusps(), allPlainCusps...)
	m.chart.cusps = fullChartCusps()
	m.focused = PanelBodies
	m.bodyCursorIdx = 0
	m = m.resetBodyTop()
	if got := m.geometry()[PanelBodies].h - 2; got != inner {
		t.Fatalf("fixture: the Bodies panel is %d rows tall, want %d", got, inner)
	}
	if n := bodyRowCount(m.chart.positions, m.chart.cusps); n != fullChartPlanets+fullChartCuspRows {
		t.Fatalf("fixture: %d body rows, want %d", n, fullChartPlanets+fullChartCuspRows)
	}
	return m
}

// bodiesPageRows is the Bodies body as drawn now, one string per row.
func bodiesPageRows(m Model) []string {
	body, _, _ := renderBodiesFor(m.bodiesView(), m.paintedChart(), m.slots, m.cursorIdx, m.bodyCursorIdx,
		m.geometry()[PanelBodies].w-2, m.geometry()[PanelBodies].h-2, true, Config{Accessible: true, Color: ColorNever})
	return strings.Split(ansi.Strip(body), "\n")
}

// sansCursorMark drops the row prefix (the cursor mark and the selection
// column), so two frames can be compared for everything else.
func sansCursorMark(rows []string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		if len([]rune(r)) > 3 {
			r = string([]rune(r)[3:])
		}
		out[i] = strings.TrimRight(r, " ")
	}
	return out
}

// storedTopIsDrawn says the Model's bodyTop is the window the render draws from:
// nothing is left for the render to correct, which is what re-clamping the
// stored window on resize, slot change and a changed list is for.
func storedTopIsDrawn(t *testing.T, m Model, when string) {
	t.Helper()
	p, u, h := m.bodiesRows()
	if got := bodyWindowStart(p, u, m.bodyCursorIdx, m.bodyTop, h); got != m.bodyTop {
		t.Errorf("%s: bodyTop %d is stale, the render draws from %d (cursor %d, %d+%d rows, %d tall)", when, m.bodyTop, got, m.bodyCursorIdx, p, u, h)
	}
}

func markedRow(m Model) int {
	for i, r := range bodiesPageRows(m) {
		if strings.HasPrefix(r, ">") {
			return i
		}
	}
	return -1
}

// A4: PgUp from the bottom of a scrolled Bodies list moves the cursor to the
// window's first row and leaves the window; the next PgUp pages the window.
func TestBodiesPageKeysKeepTheWindow(t *testing.T) {
	m := bodiesPageModel(t, 16)
	last := fullChartPlanets + fullChartCuspRows - 1
	for i := 0; i < last; i++ {
		m = pageKey(t, m, "j")
	}
	if m.bodyCursorIdx != last || m.bodyTop == 0 {
		t.Fatalf("fixture: j to the bottom left cursor %d top %d", m.bodyCursorIdx, m.bodyTop)
	}
	bottom := bodiesPageRows(m)
	top := m.bodyTop

	m = pageKey(t, m, "pgup")
	if m.bodyTop != top {
		t.Errorf("PgUp from the bottom moved the window: bodyTop %d -> %d", top, m.bodyTop)
	}
	if got := markedRow(m); got != 0 {
		t.Errorf("PgUp from the bottom: the cursor mark is on drawn row %d, want the window's first row", got)
	}
	if m.bodyCursorIdx != top {
		t.Errorf("PgUp from the bottom: cursor %d, want %d (the window's first row)", m.bodyCursorIdx, top)
	}
	if a, b := strings.Join(sansCursorMark(bottom), "\n"), strings.Join(sansCursorMark(bodiesPageRows(m)), "\n"); a != b {
		t.Errorf("the frame changed beyond the cursor mark:\nbefore:\n%s\nafter:\n%s", a, b)
	}

	// The second press scrolls: the window moves up by a page of the window less
	// a row (15 visible cursor rows here), clamped at the top of the list.
	m = pageKey(t, m, "pgup")
	if m.bodyCursorIdx != 0 || m.bodyTop != 0 {
		t.Errorf("second PgUp: cursor %d bodyTop %d, want the top of the list", m.bodyCursorIdx, m.bodyTop)
	}
	// And down again: first to the window's last row, then a page.
	m = pageKey(t, m, "pgdown")
	if want := bodyPageRowsForTest(m); m.bodyCursorIdx != want-1 || m.bodyTop != 0 {
		t.Errorf("PgDn from the top: cursor %d bodyTop %d, want cursor %d with the window unmoved", m.bodyCursorIdx, m.bodyTop, want-1)
	}
	m = pageKey(t, m, "pgdown")
	if m.bodyTop == 0 || markedRow(m) < 0 {
		t.Errorf("second PgDn: bodyTop %d, cursor row drawn at %d; want the window paged with the cursor in it", m.bodyTop, markedRow(m))
	}

	for _, tc := range []struct {
		key  string
		want int
	}{{"end", last}, {"home", 0}, {"G", last}, {"g", 0}} {
		m = pageKey(t, m, tc.key)
		if m.bodyCursorIdx != tc.want || markedRow(m) < 0 {
			t.Errorf("%s: cursor %d (drawn at %d), want %d in sight", tc.key, m.bodyCursorIdx, markedRow(m), tc.want)
		}
	}
}

// bodyPageRowsForTest is the cursor rows a page can rely on, as the render sees
// the list: the panel's rows less the rule.
func bodyPageRowsForTest(m Model) int {
	p, u, h := m.bodiesRows()
	if bodyRuled(p, u, h) {
		return h - 1
	}
	return h
}

// A4: the cursor stays in sight through a resize to a shorter panel, and when a
// recompute drops rows under it.
func TestBodiesWindowFollowsResizeAndRecompute(t *testing.T) {
	for _, size := range []int{24, 16, 12, 8, 5, 3} {
		m := bodiesPageModel(t, 16)
		for i := 0; i < 25; i++ {
			m = pageKey(t, m, "j") // the cursor on the last row, the window scrolled to it
		}
		m.height = size + 12
		m2, _ := m.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m = m2.(Model)
		if got := markedRow(m); got < 0 {
			t.Errorf("resized to %d rows: the cursor (row %d) is not drawn; top %d", size, m.bodyCursorIdx, m.bodyTop)
		}
		storedTopIsDrawn(t, m, fmt.Sprintf("resized to %d rows", size))
	}
	// A recompute that drops rows: the cursor was on the last row of 26.
	m := bodiesPageModel(t, 16)
	m = pageKey(t, m, "end")
	committed, _ := applyTextCommit(m, FirstFrameReadyMsg{Positions: fullChartPositions()[:fullChartPlanets]})
	if got := markedRow(committed); got < 0 {
		t.Errorf("rows dropped under the cursor: row %d of %d, drawn at %d, bodyTop %d",
			committed.bodyCursorIdx, bodyRowCount(committed.chart.positions, committed.chart.cusps), got, committed.bodyTop)
	}
	storedTopIsDrawn(t, committed, "rows dropped under the cursor")
	// Another slot has its own list: its window is derived from its own cursor.
	listed := bodiesPageModel(t, 16)
	listed.slots = listWorkspace(t, 2)
	listed.cursorIdx = 2 // the floating slot
	listed.width = dualColumnMinWidth(2)
	listed = listed.resetBodyTop()
	listed = pageKey(t, listed, "end")
	if listed.bodyTop == 0 {
		t.Fatal("fixture: End did not scroll the list")
	}
	listed.focused = PanelChart
	listed = pageKey(t, listed, "up")
	storedTopIsDrawn(t, listed, "another slot focused")
}

// drawnCursorRows are the frame rows that carry a cursor row: every drawn row
// but the rule's blank one. first and last index into the rendered frame.
func drawnCursorRows(m Model) (first, last int) {
	first, last = -1, -1
	for i, r := range bodiesPageRows(m) {
		if strings.TrimSpace(r) == "" {
			continue // the rule's row
		}
		if first < 0 {
			first = i
		}
		last = i
	}
	return first, last
}

// A4, amended 1b: Bodies pages by the window it draws. At every panel height
// from 3 to 9 rows, on a slot whose list has the cusp rule, the first PgUp from
// the bottom lands on the first DRAWN cursor row and the first PgDn from the top
// on the last; every press after pages the window and lands on the new window's
// edge, until the list ends. Read off the rendered frame, not off the arithmetic.
func TestBodiesPageUpLandsOnTheFirstDrawnRow(t *testing.T) {
	for inner := 3; inner <= 9; inner++ {
		last := fullChartPlanets + fullChartCuspRows - 1

		m := bodiesPageModel(t, inner)
		m = pageKey(t, m, "end")
		for step := 0; m.bodyCursorIdx > 0; step++ {
			before := m.bodyCursorIdx
			m = pageKey(t, m, "pgup")
			first, _ := drawnCursorRows(m)
			if got := markedRow(m); got != first {
				t.Fatalf("inner %d, PgUp %d (cursor %d -> %d): the mark is on drawn row %d, the first cursor row drawn is %d\n%s",
					inner, step+1, before, m.bodyCursorIdx, got, first, strings.Join(bodiesPageRows(m), "\n"))
			}
			if m.bodyCursorIdx >= before || step > 3*last {
				t.Fatalf("inner %d: PgUp did not move the cursor up (%d -> %d)", inner, before, m.bodyCursorIdx)
			}
		}

		m = bodiesPageModel(t, inner)
		m = pageKey(t, m, "home")
		for step := 0; m.bodyCursorIdx < last; step++ {
			before := m.bodyCursorIdx
			m = pageKey(t, m, "pgdown")
			_, lastRow := drawnCursorRows(m)
			if got := markedRow(m); got != lastRow {
				t.Fatalf("inner %d, PgDn %d (cursor %d -> %d): the mark is on drawn row %d, the last cursor row drawn is %d\n%s",
					inner, step+1, before, m.bodyCursorIdx, got, lastRow, strings.Join(bodiesPageRows(m), "\n"))
			}
			if m.bodyCursorIdx <= before || step > 3*last {
				t.Fatalf("inner %d: PgDn did not move the cursor down (%d -> %d)", inner, before, m.bodyCursorIdx)
			}
		}
	}
}
