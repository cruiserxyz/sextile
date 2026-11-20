package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

var tickSpace = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
var tickEnter = tea.KeyMsg{Type: tea.KeyEnter}

// tickPicker is a ready workspace with the chart list open over the named
// charts. A name ending in "?" has no place label: it asks for a title.
func tickPicker(t *testing.T, names ...string) Model {
	t.Helper()
	m := readyModelWithSlots(t, berlinP)
	m.startup = startupChartOpen
	var charts []store.NamedChart
	for _, n := range names {
		c := testNamedChart(strings.TrimSuffix(n, "?"), fixtureOneP)
		if strings.HasSuffix(n, "?") {
			c.PlaceLabel = ""
		}
		charts = append(charts, c)
	}
	m.picker = chartPickerState{loaded: true, prior: startupReady, charts: charts}
	m.picker.refilter()
	return m
}

func tickKey(t *testing.T, m Model, k tea.KeyMsg) (Model, tea.Cmd) {
	t.Helper()
	u, cmd := updateChartModalKey(m, k)
	return u.(Model), cmd
}

// tickAt moves the cursor to the named chart and presses Space.
func tickRow(t *testing.T, m Model, name string) Model {
	t.Helper()
	for i, c := range m.picker.Matches {
		if c.Name == name {
			m.picker.Cursor = i
			m, _ = tickKey(t, m, tickSpace)
			return m
		}
	}
	t.Fatalf("no row %q in %v", name, pickerNames(m.picker.Matches))
	return m
}

func typeFilter(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m, _ = tickKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

// openTicks delivers the resolve the batch is waiting on, until it asks for
// nothing more. It stops at a title form.
func openTicks(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for i := 0; i < 20 && m.picker.resolving; i++ {
		m, cmd = resolvedByLoop(t, m, time.Date(1990, 1, 1+i, 12, 0, 0, 0, time.UTC))
	}
	return m
}

// resolvedByLoop delivers the resolve the picker waits on through Update, as
// the program does, so the batch goes on to its next chart.
func resolvedByLoop(t *testing.T, m Model, when time.Time) (Model, tea.Cmd) {
	t.Helper()
	u, cmd := m.Update(ChartResolvedMsg{Chart: m.picker.pending, When: when, Gen: m.pickGen})
	return u.(Model), cmd
}

func openNames(m Model) []string {
	var out []string
	for _, s := range m.slots.All()[:m.slots.AnchoredCount()] {
		out = append(out, s.Chart.Name)
	}
	return out
}

func lastLog(m Model) string {
	if len(m.logLines) == 0 {
		return ""
	}
	return m.logLines[len(m.logLines)-1].text
}

// Item 1: Space ticks the row under the cursor and unticks it; the search takes no space.
func TestTicksSpaceTicksAndUnticks(t *testing.T) {
	m := tickPicker(t, "Albert Einstein", "Johannes Kepler")
	m.picker.Cursor = 1
	m, _ = tickKey(t, m, tickSpace)
	if got := m.picker.ticked; len(got) != 1 || got[0] != "Johannes Kepler.as" {
		t.Fatalf("ticked = %v after Space", got)
	}
	if view := ansi.Strip(renderChartModal(m.picker, plainCfg, 80, 24)); !strings.Contains(view, ">✓Johannes Kepler") {
		t.Errorf("the ticked row under the cursor shows no tick:\n%s", view)
	}
	m, _ = tickKey(t, m, tickSpace)
	if len(m.picker.ticked) != 0 {
		t.Errorf("a second Space left ticks %v", m.picker.ticked)
	}
	if m.picker.FilterText != "" {
		t.Errorf("Space typed into the search: %q", m.picker.FilterText)
	}
}

func TestTicksSearchFindsTwoWordNamesWithoutTheSpace(t *testing.T) {
	// The longer name comes first: only a name that matches whole, spaces
	// aside, puts Johannes Kepler above it.
	m := tickPicker(t, "Johannes Keplerian", "Johannes Kepler")
	m = typeFilter(t, m, "johanneskepler")
	if got := strings.Join(pickerNames(m.picker.Matches), ","); got != "Johannes Kepler,Johannes Keplerian" {
		t.Errorf("typed search found %s", got)
	}
	m = tickPicker(t, "Albert Einstein", "Johannes Kepler")
	m = typeFilter(t, m, "kepler")
	if len(m.picker.Matches) != 1 || m.picker.Matches[0].Name != "Johannes Kepler" {
		t.Errorf("search kepler found %v", pickerNames(m.picker.Matches))
	}
}

// Item 2: with nothing ticked Enter opens the chart under the cursor.
func TestTicksEnterWithoutTicksOpensTheCursorChart(t *testing.T) {
	m := tickPicker(t, "Albert Einstein", "Johannes Kepler")
	m.picker.Cursor = 1
	m, cmd := tickKey(t, m, tickEnter)
	if cmd == nil || m.picker.pending.Name != "Johannes Kepler" {
		t.Fatalf("Enter did not read the chart under the cursor (pending %q)", m.picker.pending.Name)
	}
}

// Items 3 and 10: the ticked charts open, only they, in tick order, cursor on the last, nothing selected.
func TestTicksEnterOpensTheTickedChartsOnly(t *testing.T) {
	m := tickPicker(t, "Albert Einstein", "Goethe", "Johannes Kepler", "Marie Curie")
	m = tickRow(t, m, "Marie Curie")
	m = tickRow(t, m, "Albert Einstein")
	m = tickRow(t, m, "Johannes Kepler")
	m.picker.Cursor = 1 // Goethe, which is not ticked
	m, cmd := tickKey(t, m, tickEnter)
	m = openTicks(t, m, cmd)
	if got := strings.Join(openNames(m), ","); got != "Marie Curie,Albert Einstein,Johannes Kepler" {
		t.Fatalf("opened %s", got)
	}
	if m.cursorIdx != 2 {
		t.Errorf("cursor on slot %d, want the last (2)", m.cursorIdx)
	}
	if n := m.slots.SelectedCount(); n != 0 {
		t.Errorf("a batch selected %d charts", n)
	}
	if m.startup != startupComputing {
		t.Errorf("startup = %s after the last chart opened", m.startup)
	}
}

// Item 4: while a search is typed the ticked rows stay on top, in tick order, matching or not.
func TestTicksStayOnTopWhileSearching(t *testing.T) {
	m := tickPicker(t, "Albert Einstein", "Goethe", "Johannes Kepler", "Kepler Two", "Marie Curie")
	m = tickRow(t, m, "Marie Curie")
	m = tickRow(t, m, "Albert Einstein")
	m = typeFilter(t, m, "kepler")
	if got := strings.Join(pickerNames(m.picker.Matches), ","); got != "Marie Curie,Albert Einstein,Kepler Two,Johannes Kepler" {
		t.Fatalf("rows while searching: %s", got)
	}
	m = tickRow(t, m, "Johannes Kepler")
	if got := strings.Join(pickerNames(m.picker.Matches), ","); got != "Marie Curie,Albert Einstein,Johannes Kepler,Kepler Two" {
		t.Fatalf("rows after a third tick: %s", got)
	}
	if m.picker.Matches[m.picker.Cursor].Name != "Kepler Two" {
		t.Errorf("cursor on %q, want the first row below the ticks", m.picker.Matches[m.picker.Cursor].Name)
	}
	m, cmd := tickKey(t, m, tickEnter)
	m = openTicks(t, m, cmd)
	if got := strings.Join(openNames(m), ","); got != "Marie Curie,Albert Einstein,Johannes Kepler" {
		t.Errorf("opened %s", got)
	}
}

// Typing leaves the cursor on the first match below the ticked rows, so Space ticks what was typed for.
func TestTicksTypingLeavesTheCursorBelowTheTicks(t *testing.T) {
	m := tickPicker(t, "Albert Einstein", "Johannes Kepler")
	m = tickRow(t, m, "Albert Einstein")
	m = typeFilter(t, m, "kepl")
	if got := m.picker.Matches[m.picker.Cursor].Name; got != "Johannes Kepler" {
		t.Errorf("cursor on %q after typing, want Johannes Kepler", got)
	}
}

// Item 5: a refused chart cannot be ticked.
func TestTicksARefusedChartCannotBeTicked(t *testing.T) {
	m := tickPicker(t, "Albert Einstein", "Helio")
	m.picker.refused = map[string]string{"Helio.as": "a heliocentric chart"}
	m.picker.Cursor = 1
	m, _ = tickKey(t, m, tickSpace)
	if len(m.picker.ticked) != 0 {
		t.Errorf("a refused chart was ticked: %v", m.picker.ticked)
	}
}

// Item 6: a chart that needs a title asks after the others; Esc skips it and the Log says so.
func TestTicksATitleIsAskedAfterTheOthersAndEscOpensUnderTheFileName(t *testing.T) {
	m := tickPicker(t, "Albert Einstein", "Equinox?", "Goethe", "Solstice?")
	for _, n := range []string{"Equinox", "Albert Einstein", "Solstice", "Goethe"} {
		m = tickRow(t, m, n)
	}
	m, cmd := tickKey(t, m, tickEnter)
	m = openTicks(t, m, cmd)
	if m.startup != startupTitleOpen || m.title.chart.Name != "Equinox" {
		t.Fatalf("startup = %s, asking for %q; want the form for Equinox", m.startup, m.title.chart.Name)
	}
	if got := strings.Join(openNames(m), ","); got != "Albert Einstein,Goethe" {
		t.Fatalf("opened before the forms: %s", got)
	}
	// The bar says what Esc does here: it opens the chart under its file name.
	// Outside a batch it goes back to the list.
	escTitle := func(m Model) string {
		for _, e := range chartTitleBindings() {
			if e.ID == keys.BindingClose {
				return e.TitleFor(m.keysContextFor(ownerChartTitle))
			}
		}
		return ""
	}
	if got := escTitle(m); got != "Use the file name" {
		t.Errorf("in a batch the title form's Esc reads %q, want Use the file name", got)
	}
	single := m
	single.title.batch = false
	if got := escTitle(single); got != "Back" {
		t.Errorf("outside a batch the title form's Esc reads %q, want Back", got)
	}
	// Esc opens Equinox untitled, as an empty title does, and the batch goes on.
	before := len(m.logLines)
	u, cmd := updateTitleFormKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	m = u.(Model)
	if cmd == nil || !m.picker.resolving {
		t.Fatal("Esc in a batch form did not open the chart")
	}
	m, _ = resolvedByLoop(t, m, time.Date(1990, 1, 1, 12, 0, 0, 0, time.UTC))
	if m.startup != startupTitleOpen || m.title.chart.Name != "Solstice" {
		t.Fatalf("after Esc: startup = %s, asking for %q; want the form for Solstice", m.startup, m.title.chart.Name)
	}
	for _, l := range m.logLines[before:] {
		if strings.Contains(l.text, "not opened") {
			t.Errorf("Esc wrote %q", l.text)
		}
	}
}

// Item 6 again: Enter with an empty title in a batch form opens the chart untitled, then goes on.
func TestTicksAnEmptyTitleOpensTheChartAndGoesOn(t *testing.T) {
	m := tickPicker(t, "Equinox?", "Solstice?")
	m = tickRow(t, m, "Equinox")
	m = tickRow(t, m, "Solstice")
	m, _ = tickKey(t, m, tickEnter)
	if m.startup != startupTitleOpen || m.title.chart.Name != "Equinox" {
		t.Fatalf("startup = %s, asking for %q", m.startup, m.title.chart.Name)
	}
	u, cmd := updateTitleFormKey(m, tickEnter)
	m = u.(Model)
	if cmd == nil || !m.picker.resolving {
		t.Fatal("an empty title did not open the chart")
	}
	m, _ = resolvedByLoop(t, m, time.Date(1990, 1, 1, 12, 0, 0, 0, time.UTC))
	if m.startup != startupTitleOpen || m.title.chart.Name != "Solstice" {
		t.Fatalf("startup = %s, asking for %q; want the next form", m.startup, m.title.chart.Name)
	}
}

// Item 7: a chart that is open already is not opened twice, and the batch says nothing.
func TestTicksAnOpenChartIsNotOpenedTwice(t *testing.T) {
	m := tickPicker(t, "Albert Einstein", "Goethe")
	m.picker.Cursor = 0
	m, cmd := tickKey(t, m, tickEnter)
	m = openTicks(t, m, cmd)
	m.startup, m.picker.prior = startupChartOpen, startupReady
	m = tickRow(t, m, "Albert Einstein")
	m = tickRow(t, m, "Goethe")
	before := len(m.logLines)
	m, cmd = tickKey(t, m, tickEnter)
	if m.picker.pending.Name != "Goethe" {
		t.Errorf("the batch began with %q, which is open already", m.picker.pending.Name)
	}
	m = openTicks(t, m, cmd)
	if got := strings.Join(openNames(m), ","); got != "Albert Einstein,Goethe" {
		t.Errorf("opened %s", got)
	}
	if len(m.logLines) != before {
		t.Errorf("the batch said something: %q", lastLog(m))
	}
}

// Item 8: Esc opens nothing, and the ticks are gone when the list opens again.
func TestTicksEscOpensNothingAndTheTicksGo(t *testing.T) {
	m := tickPicker(t, "Albert Einstein", "Goethe")
	m = tickRow(t, m, "Goethe")
	m, cmd := tickKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil || m.startup != startupReady || m.slots.AnchoredCount() != 0 {
		t.Fatalf("Esc: startup %s, %d charts open", m.startup, m.slots.AnchoredCount())
	}
	u, load := m.openChartPicker()
	if load == nil || len(u.picker.ticked) != 0 {
		t.Errorf("ticks survived: %v", u.picker.ticked)
	}
}

// Item 9: the bar says Tick, and Open N with the number ticked.
func TestTicksTheBarSaysTickAndOpenN(t *testing.T) {
	var tick, open keys.BindingEntry
	for _, e := range openChartBindings() {
		switch e.ID {
		case keys.BindingToggle:
			tick = e
		case keys.BindingConfirm:
			open = e
		}
	}
	if tick.Binding.Keys()[0] != " " || tick.TitleFor(keys.Context{}) != "Tick" || !tick.Bar {
		t.Errorf("tick entry = %+v", tick)
	}
	for n, want := range map[int]string{0: "Open", 1: "Open 1", 3: "Open 3"} {
		c := keys.Context{}
		c.Picker.Ticked = n
		if got := open.TitleFor(c); got != want {
			t.Errorf("%d ticked: Enter is %q, want %q", n, got, want)
		}
	}
	m := tickPicker(t, "Albert Einstein", "Goethe")
	m = tickRow(t, m, "Goethe")
	if c := m.keysContext(); c.Picker.Ticked != 1 {
		t.Errorf("the key context counts %d ticked", c.Picker.Ticked)
	}
}
