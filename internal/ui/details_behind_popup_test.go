package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// A popup that changes nothing behind it leaves Details as it was (Sextile
// FINDINGS, 2026-11-05: with the Chart report showing, `Ctrl+O` drew one faint
// `…` under the caption `[0] Now`, and Esc brought the report back). What
// Details is about follows the cursor, the focus and the tab alone; an open
// popup only stops new work from starting.
func TestDetailsKeepsItsReportBehindAPopup(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyMsg
	}{
		{"the chart picker", tea.KeyMsg{Type: tea.KeyCtrlO}},
		{"the zodiac chooser", tea.KeyMsg{Type: tea.KeyCtrlZ}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := followFixture(t, Config{Accessible: true}, 132, 40)
			before := m.details.body
			u, _ := m.Update(tc.key)
			m = u.(Model)
			if !m.modalOpen() {
				t.Fatalf("fixture: %s did not open", tc.name)
			}
			if got := m.detailsReportState(); got != paneCurrent {
				t.Errorf("Details behind %s: state %v, want the report as it was (current)", tc.name, got)
			}
			if got := m.detailsBodyShown(); got != before || !strings.Contains(got, "SUBJECT chart - floating") {
				t.Errorf("Details behind %s draws %q, want the Chart report %q", tc.name, got, before)
			}
		})
	}
}
