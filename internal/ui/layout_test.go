package ui

import (
	"fmt"
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

	one := layoutGeometry(w, h, 1, 1)
	three := layoutGeometry(w, h, 3, 1)
	if one == nil || three == nil {
		t.Fatal("layoutGeometry returned nil at 132x40")
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
	sum := three[PanelChart].h + three[PanelBodies].h + three[PanelAspects].h
	if want := h - KeyBarHeight; sum != want {
		t.Errorf("left column heights sum = %d, want main height %d", sum, want)
	}
}

// TestLayoutGeometry_DegradesWhenChartCannotFit asserts the layout
// refuses (returns nil → tooSmallView) when chart + minimum bodies +
// aspects cannot fit in the main area. Multi-slot workspaces raise the
// effective minimum above MinTerminalHeight; without this guard the UI
// silently overflows at small heights with anchored slots present.
func TestLayoutGeometry_DegradesWhenChartCannotFit(t *testing.T) {
	const w = 132

	// At the base minimum height (12) a single-slot layout still fits:
	// chart(3) + bodies(5 minimum) + aspects(6) = 14 > 10 mainHeight, so
	// actually this overflows too. Guard at width-only minimum.
	// Verify the guard fires on a multi-slot workspace at the base height.
	if got := layoutGeometry(w, MinTerminalHeight, 2, 1); got != nil {
		t.Errorf("layoutGeometry(w=%d, h=%d, chartLines=2) = non-nil; expected nil because chart+bodies+aspects exceed main area",
			w, MinTerminalHeight)
	}

	// A 2-slot layout starts fitting at height 17 (chart=4 + bodies=5
	// + aspects=6 + status=1 + keybar=1).
	if got := layoutGeometry(w, 17, 2, 1); got == nil {
		t.Errorf("layoutGeometry(w=%d, h=17, chartLines=2) = nil; expected fit at exactly the multi-slot minimum",
			w)
	}
}

// readyFrame is a first ready frame committed at the model's size.
func readyFrame(m Model) (Model, tea.Cmd) {
	return applyTextCommit(m, FirstFrameReadyMsg{
		Positions: fullChartPositions(), Aspects: tasteOwnAspects(), CrossAspects: tasteCrossAspects(), Overlay: true,
	})
}

// Limited view notice: the first ready frame with no Details rect sets the footer notice once
// per session; a resize does not set it again, and 144x40 never does with two charts.
func TestLimitedViewNoticeOncePerSession(t *testing.T) {
	const sentence = "Limited view: 132x30 shows Details and the Log"
	m := helpFixtureOneChart(t, 80, 24)
	m.limitedViewShown = false
	m.startup = startupComputing
	m, cmd := readyFrame(m)
	if got := m.footerIndicator(); got != sentence {
		t.Fatalf("first frame at 80x24: indicator %q, want %q", got, sentence)
	}
	if cmd == nil {
		t.Errorf("the notice armed no clear timer")
	}
	tm, _ := m.Update(yankFlashMsg{gen: m.yank.gen})
	m = tm.(Model)
	if got := m.footerIndicator(); got != "" {
		t.Fatalf("after the tick the indicator is %q, want it cleared", got)
	}
	m, _ = escSend(t, m, tea.WindowSizeMsg{Width: 90, Height: 24}, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = readyFrame(m)
	if got := m.footerIndicator(); got != "" {
		t.Errorf("a later frame and a resize set the notice again: %q", got)
	}

	wide := helpFixture(t, 144, 40)
	wide.limitedViewShown = false
	wide.startup = startupComputing
	wide, _ = readyFrame(wide)
	if got := wide.footerIndicator(); got != "" {
		t.Errorf("at 144x40 the indicator is %q", got)
	}
}

// Limited view notice: the width is the layout's own threshold for the charts shown.
func TestLimitedViewNoticeNamesTheComputedWidth(t *testing.T) {
	m := helpFixture(t, 80, 24)
	m.limitedViewShown = false
	m.startup = startupComputing
	m, _ = readyFrame(m)
	want := fmt.Sprintf("Limited view: %dx30 shows Details and the Log", dualColumnMinWidth(m.shownCount()))
	if want != "Limited view: 144x30 shows Details and the Log" {
		t.Errorf("two charts: sentence %q, want the 144-column one", want)
	}
	if got := m.footerIndicator(); got != want {
		t.Errorf("indicator %q, want %q", got, want)
	}
}

// chartPanelRows is the Chart panel's slot rows as drawn, one string each.
func chartPanelRows(t *testing.T, m Model) []string {
	t.Helper()
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	h := m.geometry()[PanelChart].h
	var rows []string
	for _, l := range lines[1 : h-1] {
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
	m.width, m.height = 80, 24
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
	for y := 0; y < m.geometry()[PanelChart].h; y++ {
		if l := strings.TrimRight(frame[y], " "); strings.HasSuffix(l, scrollThumbGlyph) || strings.HasSuffix(l, scrollThumbASCII) {
			thumb = append(thumb, y)
		}
	}
	top, size := viewport{Total: 8, Visible: 5, Offset: m.chartTop}.thumb()
	if len(thumb) != size || len(thumb) == 0 || thumb[0] != 1+top {
		t.Errorf("scroll thumb rows %v, want %d rows from row %d (offset %d)", thumb, size, 1+top, m.chartTop)
	}
}

func TestChartTopSurvivesResize(t *testing.T) {
	m := listModel(t, 7, 0)
	m.width, m.height = 80, 24
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

// Details needs 80 columns beside the left column: two charts at 132x40 are one
// column, one chart keeps Details at exactly 80.
func TestTwoChartsAt132x40FallToOneColumn(t *testing.T) {
	two := layoutGeometry(132, 40, 1, 2)
	if _, ok := two[PanelDetails]; ok {
		t.Errorf("two charts at 132x40 still have a Details rect: %+v", two[PanelDetails])
	}
	if w := two[PanelBodies].w; w != 132 {
		t.Errorf("two charts at 132x40: Bodies is %d wide, want 132", w)
	}
	one := layoutGeometry(132, 40, 1, 1)
	d, ok := one[PanelDetails]
	if !ok || d.w != 80 {
		t.Errorf("one chart at 132x40: Details = %+v (present %v), want 80 wide", d, ok)
	}
}
