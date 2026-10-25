package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestPanelDigitAgainStepsItsTabs pins one design rule for EVERY panel digit
// (operator, 2026-10-04: continuous "3" in Aspects cycles the tabs): a digit
// focuses its panel, and pressed again on the panel it already focuses it
// steps that panel's tabs, wrapping, as `]` does — a one-tab panel stays as it
// is. The test walks panelDigits and focusablePanels rather than naming
// panels, so a panel that gains tabs, or a new focusable panel, is held to the
// rule without anyone writing a line for it.
func TestPanelDigitAgainStepsItsTabs(t *testing.T) {
	// Every focusable ring panel has a digit: a panel without one could not
	// take part in the rule.
	for _, p := range focusablePanels() {
		found := false
		for _, q := range panelDigits {
			found = found || q == p
		}
		if !found {
			t.Errorf("focusable panel %v has no digit in panelDigits", p)
		}
	}

	stepped := 0
	for id, p := range panelDigits {
		stubPairCompute(t)
		// Comparing ①②③ + now, so Aspects declares three pair tabs.
		m := selectedSet(t, 3, true, 0)
		key := ""
		for _, e := range m.keys.mainBindings() {
			if ks := e.Binding.Keys(); e.ID == id && len(ks) > 0 {
				key = ks[0]
			}
		}
		if key == "" {
			t.Fatalf("panel %v: its binding has no key", p)
		}
		m.cfg = Config{Color: ColorNever}
		m, cmd := m.afterComparisonChange()
		m = pump(t, m, cmd)
		press := func() {
			var c tea.Cmd
			m, c = tpress(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
			m = pump(t, m, c)
		}
		active := func() int { return activeTabIndex(m.panelTabs(p), m.tabChoice(p)) }

		if m.focused == p {
			// Start elsewhere, so the first press is the focusing one.
			m.focused = focusablePanels()[0]
			if m.focused == p {
				m.focused = focusablePanels()[1]
			}
		}
		press()
		if m.focused != p {
			t.Fatalf("%q focused %v, want %v", key, m.focused, p)
		}
		n := len(m.panelTabs(p))
		for i := 1; i <= n+1; i++ {
			before := active()
			press()
			if m.focused != p {
				t.Fatalf("%q again moved focus to %v", key, m.focused)
			}
			want := before
			if n > 1 {
				want = (before + 1) % n
			}
			if got := active(); got != want {
				t.Fatalf("%q again (%d of %d tabs): active tab %d, want %d", key, i, n, got, want)
			}
		}
		if n > 1 {
			stepped++
		}
	}
	// The fixture must exercise the rule somewhere, or it proves nothing.
	if stepped == 0 {
		t.Fatal("no panel declared more than one tab in the fixture; the rule went untested")
	}
}
