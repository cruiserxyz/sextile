package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// All chart values in this file are constructed synthetic test inputs.

// specs/details-at-every-size.md: Details opens on every panel at every size,
// right below the panel in focus where it does not fit beside the panels, and
// the panels fold.

// everySizeModel is a ready workspace of `charts` anchored charts above the
// floating slot, one frame committed with its bodies and aspects, the cursor
// on the floating slot, with the Chart panel in focus at w x h.
func everySizeModel(t *testing.T, charts, w, h int) Model {
	t.Helper()
	sessionSandbox(t)
	m := listModel(t, charts, charts)
	m.cfg = Config{Color: ColorNever}
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{Positions: fullChartPositions(), Aspects: tasteOwnAspects()})
	m = withSceneColumns(m)
	m.startup = startupReady
	m.focused = PanelChart
	m.width, m.height = w, h
	return m.restampFrame()
}

var leftColumnPanels = []PanelID{PanelStatus, PanelChart, PanelBodies, PanelAspects}

// titleOf is the text a panel's title carries on screen.
var titleOf = map[PanelID]string{
	PanelStatus: "[1] ", PanelChart: "[2] ", PanelBodies: "[3] ", PanelAspects: "[4] ",
}

// 1: the screen is drawn where the layout puts every panel: the open panels'
// title rows, the folded panels' title lines, Details' title row, at every
// size, with each panel in focus, Details open and closed.
func TestScreenIsDrawnWhereTheLayoutPlacesIt(t *testing.T) {
	for _, size := range [][2]int{{66, 15}, {80, 24}, {80, 29}, {80, 30}, {100, 40}, {160, 40}} {
		for _, focus := range leftColumnPanels {
			for _, open := range []bool{false, true} {
				m := everySizeModel(t, 1, size[0], size[1])
				m.focused = focus
				if open {
					m, _ = press(t, m, "enter")
				}
				name := fmt.Sprintf("%dx%d, %s in focus, Details open %v", size[0], size[1], focus, open)
				lay, lines := m.layout(), screenLines(m)
				if len(lines) != size[1] {
					t.Fatalf("%s: %d rows drawn", name, len(lines))
				}
				for _, id := range leftColumnPanels {
					r := lay.rects[id]
					row := lines[r.y]
					switch {
					case lay.folded[id]:
						if !strings.HasPrefix(row, "╶─"+titleOf[id]) || strings.Contains(row, "┃") {
							t.Errorf("%s: folded %s is drawn as %q", name, id, row)
						}
					case !strings.Contains(row, titleOf[id]):
						t.Errorf("%s: %s opens on row %d, which reads %q", name, id, r.y, row)
					}
				}
				if d, ok := lay.rects[PanelDetails]; ok {
					if row := lines[d.y]; !strings.Contains(row[ansiCell(row, d.x):], "[0] ") {
						t.Errorf("%s: Details' title is not on its row: %q", name, row)
					}
				} else if strings.Contains(strings.Join(lines, "\n"), "[0] ") {
					t.Errorf("%s: a [0] panel is drawn with no Details rectangle", name)
				}
			}
		}
	}
}

// ansiCell is the byte offset of cell column x in a plain-text row.
func ansiCell(row string, x int) int {
	n := 0
	for i := range row {
		if n == x {
			return i
		}
		n++
	}
	return len(row)
}

// 2: where Details fits beside the panels the screen is as it was, Details and
// the Log on the right, with no key pressed.
func TestDetailsStandsBesideThePanelsWhereItFits(t *testing.T) {
	m := everySizeModel(t, 1, 160, 40)
	lay := m.layout()
	if !lay.beside || m.details.attached || !m.detailsVisible() {
		t.Fatalf("160x40: beside %v, attached %v, visible %v", lay.beside, m.details.attached, m.detailsVisible())
	}
	if d, l := lay.rects[PanelDetails], lay.rects[PanelLog]; d.x == 0 || l.x != d.x || l.y != d.y+d.h {
		t.Errorf("Details %+v and the Log %+v are not stacked on the right", d, l)
	}
	for _, id := range leftColumnPanels {
		if lay.folded[id] {
			t.Errorf("%s is folded beside Details", id)
		}
	}
}

// 3: Enter opens Details right below the panel, whichever panel it is, and Esc
// goes back to the panel with Details gone. At a size that fits beside, Enter
// attaches in place.
func TestEnterOpensDetailsBelowEveryPanelAndEscClosesIt(t *testing.T) {
	for _, focus := range leftColumnPanels {
		m := everySizeModel(t, 1, 80, 24)
		m.focused = focus
		if m.detailsVisible() {
			t.Fatalf("%s: Details is on screen before Enter", focus)
		}
		m, _ = press(t, m, "enter")
		lay := m.layout()
		d, own := lay.rects[PanelDetails], lay.rects[focus]
		if !m.details.attached || m.focused != focus || d.y != own.y+own.h || d.w != 80 || lay.folded[focus] {
			t.Errorf("%s: Enter gave attached %v, focus %q, Details %+v under %+v", focus, m.details.attached, m.focused, d, own)
		}
		if want := map[PanelID]int{PanelStatus: 3, PanelChart: 4, PanelBodies: 5, PanelAspects: 5}[focus]; own.h != want {
			t.Errorf("%s: the panel is %d rows tall with Details open, want %d (its cursor row and a neighbour each side)", focus, own.h, want)
		}
		m, _ = press(t, m, "esc")
		if m.details.attached || m.detailsVisible() || m.focused != focus {
			t.Errorf("%s: Esc left attached %v, visible %v, focus %q", focus, m.details.attached, m.detailsVisible(), m.focused)
		}
		if strings.Contains(strings.Join(screenLines(m), "\n"), "[0] ") {
			t.Errorf("%s: Details is still drawn after Esc", focus)
		}
	}
	// Beside the panels Enter attaches Details where it already stands.
	m := everySizeModel(t, 1, 160, 40)
	before := m.layout().rects[PanelDetails]
	m, _ = press(t, m, "enter")
	if !m.details.attached || m.layout().rects[PanelDetails] != before {
		t.Errorf("160x40: Enter attached %v and moved Details from %+v to %+v", m.details.attached, before, m.layout().rects[PanelDetails])
	}
}

// 3: `0` opens Details too, and pressed again switches its tab.
func TestZeroOpensDetailsAndAgainSwitchesItsTab(t *testing.T) {
	m := everySizeModel(t, 1, 80, 24)
	m.cursorIdx = len(m.slots.All()) - 1 // the floating slot: the Chart offers more than a report
	m, _ = press(t, m, "0")
	if !m.details.attached || !m.detailsVisible() {
		t.Fatalf("0 did not open Details: attached %v, visible %v", m.details.attached, m.detailsVisible())
	}
	first := m.activeTabID(PanelDetails)
	m, _ = press(t, m, "0")
	if !m.details.attached || m.activeTabID(PanelDetails) == first {
		t.Errorf("0 again: attached %v, tab %q after %q, want another tab", m.details.attached, m.activeTabID(PanelDetails), first)
	}
}

// 4: with Details open below it, the panel shows the cursor's row with one row
// above and one below, and every other panel is its title line.
func TestPanelShowsItsCursorRowAndNeighboursBelowDetails(t *testing.T) {
	m := everySizeModel(t, 5, 80, 24) // five charts and now: the Chart list holds six rows
	m.cursorIdx = 3
	m, _ = press(t, m, "enter")
	r := m.geometry()[PanelChart]
	rows := screenLines(m)[r.y+1 : r.y+r.h-1]
	if len(rows) != 3 {
		t.Fatalf("the Chart panel shows %d rows, want 3", len(rows))
	}
	for i, want := range []string{"Synthetic 3", "Synthetic 4", "Synthetic 5"} {
		if !strings.Contains(rows[i], want) {
			t.Errorf("Chart row %d is %q, want %s (the cursor on Synthetic 4, one row each side)", i, rows[i], want)
		}
	}
	for _, id := range []PanelID{PanelStatus, PanelBodies, PanelAspects} {
		if !m.layout().folded[id] {
			t.Errorf("%s is open beside Details below the Chart panel", id)
		}
	}

	// Bodies: the cursor row and its neighbours, its own scroll.
	m = everySizeModel(t, 1, 80, 24)
	m.focused = PanelBodies
	m.bodyCursorIdx = 5
	m = m.settleBodyTop()
	m, _ = press(t, m, "enter")
	br := m.geometry()[PanelBodies]
	got := strings.Join(screenLines(m)[br.y+1:br.y+br.h-1], "\n")
	for _, want := range []string{"Mars", "Jupiter", "Saturn"} {
		if !strings.Contains(got, want) {
			t.Errorf("the Bodies panel below Details lacks %s around the cursor (row 5, Jupiter):\n%s", want, got)
		}
	}
}

// 5: at any height, with Details closed and not beside the panels, the panel in
// focus and its partner are open (Session with Chart, Chart with Bodies, Bodies
// with Aspects); the others are title lines, and the number key of a folded
// panel moves the focus there and opens it.
func TestRestOpensTwoPanelsAndTheNumberKeyOpensAFoldedPanel(t *testing.T) {
	for _, h := range []int{24, 30, 40} {
		m := everySizeModel(t, 1, 80, h)
		m.focused = PanelAspects
		for _, id := range []PanelID{PanelStatus, PanelChart} {
			if !m.layout().folded[id] {
				t.Fatalf("%s is open at 80x%d with Aspects in focus", id, h)
			}
		}
		for _, id := range []PanelID{PanelBodies, PanelAspects} {
			if m.layout().folded[id] {
				t.Errorf("%s is folded at 80x%d with Aspects in focus", id, h)
			}
		}
	}
	m := everySizeModel(t, 1, 80, 24)
	m.focused = PanelAspects
	m, _ = press(t, m, "2")
	if m.focused != PanelChart || m.layout().folded[PanelChart] || m.details.attached {
		t.Errorf("2 on a folded Chart: focus %q, folded %v, attached %v", m.focused, m.layout().folded[PanelChart], m.details.attached)
	}
	if !m.layout().folded[PanelAspects] || m.layout().folded[PanelBodies] {
		t.Errorf("Chart in focus: Aspects folded %v, Bodies folded %v, want Aspects folded and Bodies open", m.layout().folded[PanelAspects], m.layout().folded[PanelBodies])
	}
	if got := screenLines(m)[m.geometry()[PanelChart].y]; !strings.Contains(got, "╭─") {
		t.Errorf("Chart is not drawn open after its number key: %q", got)
	}
}

// 6: a panel that shows fewer rows than it holds draws the scroll bar, by the
// mechanism Details uses; a panel that shows all its rows, and a folded one,
// draw none.
func TestEveryPanelThatShowsFewerRowsThanItHoldsDrawsAScrollBar(t *testing.T) {
	bar := func(m Model, id PanelID) bool {
		r := m.geometry()[id]
		if r.h < 3 {
			return false // a folded panel is its title line
		}
		for _, row := range screenLines(m)[r.y+1 : r.y+r.h-1] {
			if strings.HasSuffix(row, "┃") {
				return true
			}
		}
		return false
	}
	// Bodies, Chart and Aspects, each opened under Details with three rows.
	more := make([]aspect.Aspect, 0, 40)
	for i := 0; i < 40; i++ {
		more = append(more, tasteOwnAspects()...)
	}
	for _, id := range []PanelID{PanelBodies, PanelChart, PanelAspects} {
		m := everySizeModel(t, 5, 80, 24)
		m.chart.aspects = more
		m.focused = id
		m, _ = press(t, m, "enter")
		if !bar(m, id) {
			t.Errorf("%s shows three rows of more and draws no scroll bar:\n%s", id, strings.Join(screenLines(m), "\n"))
		}
		for _, other := range leftColumnPanels {
			if other != id && m.layout().folded[other] && strings.Contains(screenLines(m)[m.geometry()[other].y], "┃") {
				t.Errorf("folded %s draws a scroll bar", other)
			}
		}
	}
	// Control: a panel that shows everything it holds has none.
	m := everySizeModel(t, 1, 80, 40)
	m.focused = PanelStatus // Session and Chart open, each showing every row
	if bar(m, PanelChart) || bar(m, PanelStatus) {
		t.Errorf("a panel that shows every row draws a scroll bar:\n%s", strings.Join(screenLines(m), "\n"))
	}
}

// 7: Details follows the focus. A focus key moves it below the new panel, it
// shows that panel's subject, the keys stay in it and the bar names the panel
// Esc goes back to.
func TestDetailsFollowsTheFocusAndTheBarNamesItsPanel(t *testing.T) {
	m := everySizeModel(t, 1, 80, 24)
	m.focused = PanelBodies
	m, _ = press(t, m, "enter")
	barLine := func(m Model) string { l := screenLines(m); return l[len(l)-1] }
	if !strings.Contains(barLine(m), "Back to Bodies: <esc>") {
		t.Errorf("the bar on Bodies' Details is %q, want Back to Bodies: <esc>", barLine(m))
	}
	for _, step := range []struct {
		key   string
		panel PanelID
		back  string
	}{{"4", PanelAspects, "Back to Aspects: <esc>"}, {"2", PanelChart, "Back to Chart: <esc>"}, {"1", PanelStatus, "Back to Status: <esc>"}, {"3", PanelBodies, "Back to Bodies: <esc>"}} {
		m, _ = press(t, m, step.key)
		lay := m.layout()
		own, d := lay.rects[step.panel], lay.rects[PanelDetails]
		if !m.details.attached || m.focused != step.panel || d.y != own.y+own.h {
			t.Errorf("%s: attached %v, focus %q, Details %+v under %+v", step.key, m.details.attached, m.focused, d, own)
		}
		if !strings.Contains(barLine(m), step.back) {
			t.Errorf("%s: the bar is %q, want %s", step.key, barLine(m), step.back)
		}
	}
	// Tab and Shift+Tab move it too, Right does nothing, and Esc closes it where it stands.
	m, _ = press(t, m, "tab")
	if !m.details.attached || m.focused != PanelAspects {
		t.Errorf("Tab: attached %v, focus %q", m.details.attached, m.focused)
	}
	m = sendKeys(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if !m.details.attached || m.focused != PanelAspects {
		t.Errorf("Right: attached %v, focus %q", m.details.attached, m.focused)
	}
	m, _ = press(t, m, "shift+tab")
	if !m.details.attached || m.focused != PanelBodies {
		t.Errorf("Shift+Tab: attached %v, focus %q", m.details.attached, m.focused)
	}
	// The keys stay in Details: ↓ moves its rows and leaves the Bodies cursor.
	row := m.bodyCursorIdx
	m, _ = press(t, m, "down")
	if m.bodyCursorIdx != row || !m.details.attached {
		t.Errorf("down moved the Bodies cursor (%d to %d) or left Details (attached %v)", row, m.bodyCursorIdx, m.details.attached)
	}
	m, _ = press(t, m, "esc")
	if m.details.attached || m.focused != PanelBodies || m.detailsVisible() {
		t.Errorf("Esc: attached %v, focus %q, visible %v", m.details.attached, m.focused, m.detailsVisible())
	}
}

// 7 for the Status panel: moving Details to it shows its Credits and, on the
// Session tab, the session text; moving away shows a report subject again.
func TestDetailsOnTheStatusPanelShowsTheSessionText(t *testing.T) {
	m := everySizeModel(t, 1, 80, 24)
	m.focused = PanelBodies
	m, _ = press(t, m, "enter", "1")
	if !strings.Contains(strings.Join(screenLines(m), "\n"), "Walter D. Pullen") {
		t.Errorf("Details under the Status panel shows no credits:\n%s", strings.Join(screenLines(m), "\n"))
	}
	m, _ = press(t, m, "]")
	if !strings.Contains(strings.Join(screenLines(m), "\n"), "Sessions in ") {
		t.Errorf("the Session tab shows no session text:\n%s", strings.Join(screenLines(m), "\n"))
	}
	if keys.BindingFocusSession == keys.BindingFocusChart {
		t.Fatal("the Session panel has no key of its own")
	}
	m, _ = press(t, m, "3")
	if strings.Contains(strings.Join(screenLines(m), "\n"), "Sessions in ") {
		t.Error("the Session text stays after the focus left the Session panel")
	}
}

// 8: a resize needs no rule of its own. Widen the window while Details is open
// and it moves to the right side, narrow it and it moves below its panel.
func TestResizeMovesAnOpenDetailsBesideOrBelow(t *testing.T) {
	m := everySizeModel(t, 1, 80, 24)
	m.focused = PanelBodies
	m, _ = press(t, m, "enter")
	if d, b := m.geometry()[PanelDetails], m.geometry()[PanelBodies]; d.y != b.y+b.h {
		t.Fatalf("Details %+v is not below Bodies %+v at 80x24", d, b)
	}
	m = sendKeys(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	if d := m.geometry()[PanelDetails]; !m.details.attached || d.x == 0 || !m.layout().beside {
		t.Errorf("160x40: attached %v, Details %+v, beside %v", m.details.attached, d, m.layout().beside)
	}
	m = sendKeys(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if d, b := m.geometry()[PanelDetails], m.geometry()[PanelBodies]; !m.details.attached || d.x != 0 || d.y != b.y+b.h {
		t.Errorf("80x24 again: attached %v, Details %+v, Bodies %+v", m.details.attached, d, b)
	}
}

// 9: the first open on a small screen shows the "on its way" state, an empty
// body, until the report is there. It never shows the report of another row,
// and a report of an older moment is faint with its caption marked.
func TestFirstOpenOnASmallScreenWaitsForItsOwnReport(t *testing.T) {
	m, _ := followFixture(t, Config{Accessible: true}, 80, 24)
	m, _ = pressOnly(t, m, runes("3")) // Bodies, on the Sun
	m, cmd := pressOnly(t, m, keyEnter)
	if got := detailsPanelText(m); strings.Contains(got, "SUBJECT") || strings.Contains(got, influenceHead) {
		t.Fatalf("the first open shows a report before it arrived:\n%s", got)
	}
	m = pump(t, m, cmd)
	if got := detailsPanelText(m); !strings.Contains(got, "SUBJECT body Sun") {
		t.Fatalf("the Sun's report did not arrive under Bodies' Details:\n%s", got)
	}
	// Another row: close, move the Bodies cursor, open again. Nothing of the Sun's report may show.
	m, _ = pressOnly(t, m, keyEsc)
	m, _ = pressOnly(t, m, keyDown)
	m, cmd = pressOnly(t, m, keyEnter)
	if got := detailsPanelText(m); strings.Contains(got, "SUBJECT body Sun") {
		t.Fatalf("the Moon's first open shows the Sun's report:\n%s", got)
	}
	m = pump(t, m, cmd)
	if got := detailsPanelText(m); !strings.Contains(got, "SUBJECT body Moon") {
		t.Fatalf("the Moon's report did not arrive:\n%s", got)
	}
	// An older moment: close, step the clock a day, open again. The Moon's
	// report of the old moment may not show; the new one arrives in its place.
	old := m.details.body
	m, _ = pressOnly(t, m, keyEsc)
	m, cmd = pressOnly(t, m, runes("d"))
	m = pump(t, m, cmd)
	m, cmd = pressOnly(t, m, keyEnter)
	if got := detailsPanelText(m); strings.Contains(got, "SUBJECT body") {
		t.Fatalf("the first open after a step shows a report of the old moment:\n%s", got)
	}
	m = pump(t, m, cmd)
	if got := detailsPanelText(m); !strings.Contains(got, "SUBJECT body Moon") || m.details.body == old {
		t.Errorf("the Moon's report for the new moment did not arrive (same as the old: %v):\n%s", m.details.body == old, got)
	}
}
