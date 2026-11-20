package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	keyShiftUp   = tea.KeyMsg{Type: tea.KeyShiftUp}
	keyShiftDown = tea.KeyMsg{Type: tea.KeyShiftDown}
	keyCtrlUpK   = tea.KeyMsg{Type: tea.KeyCtrlK}
	keyCtrlDownJ = tea.KeyMsg{Type: tea.KeyCtrlJ}
)

// K and Shift+Up move the chart under the cursor up, J and Shift+Down down; at
// the top and the bottom nothing moves; Ctrl+K and Ctrl+J do nothing.
func TestCapitalKAndJMoveAChart(t *testing.T) {
	for _, up := range []tea.KeyMsg{keyMoveUp, keyShiftUp} {
		m := listModel(t, 3, 2)
		m = send(t, m, up)
		if want := []string{"Synthetic 1", "Synthetic 3", "Synthetic 2", "now"}; !reflect.DeepEqual(order(m), want) {
			t.Errorf("%v on the third: %v, want %v", up, order(m), want)
		}
	}
	for _, down := range []tea.KeyMsg{keyMoveDown, keyShiftDown} {
		m := listModel(t, 3, 0)
		m = send(t, m, down)
		if want := []string{"Synthetic 2", "Synthetic 1", "Synthetic 3", "now"}; !reflect.DeepEqual(order(m), want) {
			t.Errorf("%v on the first: %v, want %v", down, order(m), want)
		}
	}
	for _, c := range []struct {
		name string
		key  tea.KeyMsg
		at   int
	}{{"K at the top", keyMoveUp, 0}, {"Shift+Up at the top", keyShiftUp, 0}, {"J at the bottom", keyMoveDown, 2}, {"Shift+Down at the bottom", keyShiftDown, 2}} {
		m := listModel(t, 3, c.at)
		before := order(m)
		m = send(t, m, c.key)
		if !reflect.DeepEqual(order(m), before) || m.cursorIdx != c.at {
			t.Errorf("%s moved something: %v cursor %d", c.name, order(m), m.cursorIdx)
		}
	}
	for _, k := range []tea.KeyMsg{keyCtrlUpK, keyCtrlDownJ} {
		m := listModel(t, 3, 1)
		before := order(m)
		m = send(t, m, k)
		if !reflect.DeepEqual(order(m), before) || m.cursorIdx != 1 {
			t.Errorf("%v moved a chart: %v cursor %d", k, order(m), m.cursorIdx)
		}
	}
}

// N does nothing after n and after a step, and neither Help nor the Actions
// list names it.
func TestNDoesNothingAndIsNotListed(t *testing.T) {
	m := readyModelWithSlots(t, berlinP)
	m, _ = tpress(t, m, runes("d"))
	stepped := m.targetFloatingTime
	m, _ = tpress(t, m, runes("N"))
	if !m.targetFloatingTime.Equal(stepped) {
		t.Errorf("N after a step moved the moment: %v, want %v", m.targetFloatingTime, stepped)
	}
	m, _ = tpress(t, m, runes("n"))
	now := m.targetFloatingTime
	m, _ = tpress(t, m, runes("N"))
	if !m.targetFloatingTime.Equal(now) {
		t.Errorf("N after n moved the moment: %v, want %v", m.targetFloatingTime, now)
	}
	for _, l := range keybindingLines(m) {
		if strings.Contains(l.Text, "previous time") {
			t.Errorf("Help still names N: %q", l.Text)
		}
	}
	for _, e := range mainBindings(m.keys) {
		if e.Palette && strings.Contains(e.Title, "previous time") {
			t.Errorf("Keybindings still names N: %q", e.Title)
		}
	}
}
