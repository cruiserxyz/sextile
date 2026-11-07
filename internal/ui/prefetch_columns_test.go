package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// Stepping `d` while a chart is compared with Now (operator, 2026-11-06: "when
// hold-pressing "d" for a certain duration of time, bodies does not show
// anything (blank), while aspects DOES uptate quickly"): a day the idle
// prefetch cast ahead is drawn from memory, and Bodies shows its columns for it
// as Aspects shows its list. The prefetch casts on a copy of the chart list, so
// the frame it files names Now by that copy's slot; Bodies must still know the
// column for Now's.
func TestPrefetchedDayDrawsBodiesColumns(t *testing.T) {
	stubColumnLookups(t, nil)
	m := settledComparing(t, 3)
	if !m.focusedIsFloating() {
		t.Fatal("fixture: the cursor is not on Now")
	}
	m = idlePeriod(t, m)
	// The draw happens in the frame of the key; the command it returns is what
	// the commit owes Details and the rest, not the frame itself.
	m, _ = pressOnly(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if m.mem.drawn == 0 {
		t.Fatal("the day ahead was not drawn from what the prefetch cast: this test proves nothing")
	}
	if _, ok := m.bodyColumnsForScene(); !ok {
		t.Errorf("Bodies has no columns for the day the prefetch cast ahead (columns %d, Aspects shows that frame)", len(m.chart.columns))
	}
	plain := strings.Split(ansi.Strip(m.View()), "\n")
	if body := panelText(m, plain, PanelBodies); !strings.Contains(body, "Sun") {
		t.Errorf("Bodies is blank on the day the prefetch cast ahead:\n%s", body)
	}
}
