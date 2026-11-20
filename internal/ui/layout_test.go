package ui

import (
	"fmt"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// TestLayoutGeometry_ChartHeightIsContentDriven verifies the Chart panel is as
// tall as its content (chartH = chartLines + 2) and that Bodies absorbs the
// difference responsively as the slot list grows.
func TestLayoutGeometry_ChartHeightIsContentDriven(t *testing.T) {
	const w, h = 132, 40

	one := rectsAt(w, h, 1, 1)
	three := rectsAt(w, h, 3, 1)
	if one == nil || three == nil {
		t.Fatal("the layout is nil at 132x40")
	}

	if got := one[PanelChart].h; got != 3 {
		t.Errorf("chart height for 1 line = %d, want 3", got)
	}
	if got := three[PanelChart].h; got != 5 {
		t.Errorf("chart height for 3 lines = %d, want 5", got)
	}

	// Two extra chart lines must come out of Bodies, not Aspects, and the
	// columns must still tile the full main height.
	if three[PanelBodies].h != one[PanelBodies].h-2 {
		t.Errorf("bodies should shrink by 2 as chart grows by 2: one=%d three=%d",
			one[PanelBodies].h, three[PanelBodies].h)
	}
	if one[PanelAspects].h != three[PanelAspects].h {
		t.Errorf("aspects height should be stable: one=%d three=%d",
			one[PanelAspects].h, three[PanelAspects].h)
	}
	sum := three[PanelStatus].h + three[PanelChart].h + three[PanelBodies].h + three[PanelAspects].h
	if want := h - KeyBarHeight; sum != want {
		t.Errorf("left column heights sum = %d, want main height %d", sum, want)
	}
}

// TestLayoutGeometry_DegradesWhenChartCannotFit asserts the layout is nil
// (the too-small screen) only below 66x15, however many charts are open:
// a taller Chart folds the other panels instead of refusing.
func TestLayoutGeometry_DegradesWhenChartCannotFit(t *testing.T) {
	if got := rectsAt(132, MinTerminalHeight-1, 2, 1); got != nil {
		t.Errorf("rectsAt(132x%d, 2 charts) = non-nil; the layout is nil below 66x%d", MinTerminalHeight-1, MinTerminalHeight)
	}
	if got := rectsAt(65, 40, 2, 1); got != nil {
		t.Errorf("rectsAt(65x40, 2 charts) = non-nil; the layout is nil below 66 columns")
	}
	if got := rectsAt(132, MinTerminalHeight, 2, 1); got == nil {
		t.Errorf("rectsAt(132x%d, 2 charts) = nil; the layout exists from 66x%d", MinTerminalHeight, MinTerminalHeight)
	}
	if got := rectsAt(66, MinTerminalHeight, 2, 1); got == nil {
		t.Errorf("rectsAt(66x%d, 2 charts) = nil; the layout exists from 66x%d", MinTerminalHeight, MinTerminalHeight)
	}
}

// readyFrame is a first ready frame committed at the model's size.
func readyFrame(m Model) (Model, tea.Cmd) {
	return applyTextCommit(m, FirstFrameReadyMsg{
		Positions: fullChartPositions(), Aspects: tasteOwnAspects(), CrossAspects: tasteCrossAspects(), Overlay: true,
	})
}

// The Limited view notice is gone: no first ready frame says what size would
// show Details, at any size, because Enter opens Details at every size.
func TestNoLimitedViewNoticeAtAnySize(t *testing.T) {
	for _, size := range [][2]int{{66, 15}, {80, 24}, {132, 30}, {144, 40}} {
		m := helpFixtureOneChart(t, size[0], size[1])
		m.startup = startupComputing
		m, _ = readyFrame(m)
		if got := m.footerIndicator(); got != "" {
			t.Errorf("%dx%d: the first ready frame sets the footer notice %q", size[0], size[1], got)
		}
		if strings.Contains(m.View(), "Limited view") {
			t.Errorf("%dx%d: the screen says Limited view", size[0], size[1])
		}
	}
}

// chartPanelRows is the Chart panel's slot rows as drawn, one string each.
func chartPanelRows(t *testing.T, m Model) []string {
	t.Helper()
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	r0 := m.geometry()[PanelChart]
	var rows []string
	for _, l := range lines[r0.y+1 : r0.y+r0.h-1] {
		r := []rune(l)
		rows = append(rows, strings.TrimSpace(string(r[:min(len(r), 66)])))
	}
	return rows
}

// rowSlot reads which list index a drawn row is: "Synthetic n" is index n-1,
// anything else is the floating slot, index last.
func rowSlot(row string, last int) int {
	var n int
	s := strings.TrimLeft(strings.TrimLeft(strings.TrimLeft(row, "|│"), ">*▶ 123456①②③④⑤⑥"), " ")
	if _, err := fmt.Sscanf(s, "Synthetic %d", &n); err != nil {
		return last
	}
	return n - 1
}

func TestChartPanelShowsWindowFromChartTop(t *testing.T) {
	m := listModel(t, 7, 0)
	m.width, m.height = 220, 39 // Details beside the panels, under 40 rows: the Chart panel keeps its five-row cap
	for i := 0; i < 7; i++ {
		m = send(t, m, keyDown)
	}
	for i := 0; i < 2; i++ { // cursor 5, window still 3..7
		m = send(t, m, keyUp)
	}
	if m.chartTop != 3 {
		t.Fatalf("chartTop %d, want 3", m.chartTop)
	}
	rows := chartPanelRows(t, m)
	if len(rows) != 5 {
		t.Fatalf("%d rows, want 5: %q", len(rows), rows)
	}
	for i, r := range rows {
		if got := rowSlot(r, 7); got != m.chartTop+i {
			t.Errorf("row %d shows slot %d, want %d (%q)", i, got, m.chartTop+i, r)
		}
	}
	frame := strings.Split(ansi.Strip(m.View()), "\n")
	var thumb []int
	cr := m.geometry()[PanelChart]
	for y := 0; y < cr.h; y++ {
		r := []rune(frame[cr.y+y])
		if l := strings.TrimRight(string(r[:min(len(r), cr.x+cr.w)]), " "); strings.HasSuffix(l, pane.ThumbGlyph) || strings.HasSuffix(l, pane.ThumbASCII) {
			thumb = append(thumb, y)
		}
	}
	top, size := pane.Viewport{Total: 8, Visible: 5, Offset: m.chartTop}.Thumb()
	if len(thumb) != size || len(thumb) == 0 || thumb[0] != 1+top {
		t.Errorf("scroll thumb rows %v, want %d rows from row %d (offset %d)", thumb, size, 1+top, m.chartTop)
	}
}

func TestChartTopSurvivesResize(t *testing.T) {
	m := listModel(t, 7, 0)
	m.width, m.height = 220, 39 // Details beside the panels, under 40 rows: the Chart panel keeps its five-row cap
	for i := 0; i < 7; i++ {
		m = send(t, m, keyDown)
	}
	for _, sz := range [][2]int{{132, 40}, {80, 24}} {
		tm, _ := m.Update(tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
		m = tm.(Model)
		found := false
		for _, r := range chartPanelRows(t, m) {
			if rowSlot(r, 7) == m.cursorIdx {
				found = true
			}
		}
		if !found {
			t.Errorf("at %dx%d the cursor row %d is not shown: %q", sz[0], sz[1], m.cursorIdx, chartPanelRows(t, m))
		}
	}
}

// Details needs 67 columns beside the left column: two charts at 132x40 are one
// column, one chart keeps Details at exactly 67.
func TestTwoChartsAt132x40FallToOneColumn(t *testing.T) {
	two := rectsAt(132, 40, 1, 2)
	if _, ok := two[PanelDetails]; ok {
		t.Errorf("two charts at 132x40 still have a Details rect: %+v", two[PanelDetails])
	}
	if w := two[PanelBodies].w; w != 132 {
		t.Errorf("two charts at 132x40: Bodies is %d wide, want 132", w)
	}
	one := rectsAt(132, 40, 1, 1)
	d, ok := one[PanelDetails]
	if !ok || d.w != 67 {
		t.Errorf("one chart at 132x40: Details = %+v (present %v), want 67 wide", d, ok)
	}
}
