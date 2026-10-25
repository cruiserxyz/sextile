package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// the specification: every popup opens from attached Details, and
// closing it lands back in Details, attached, on the same tab and row.
func TestPopupsOpenFromDetailsAndCloseBackIntoIt(t *testing.T) {
	cases := []struct {
		name string
		key  tea.KeyMsg
		want owner
	}{
		{"ctrl+l", tea.KeyMsg{Type: tea.KeyCtrlL}, ownerCity},
		{"ctrl+o", tea.KeyMsg{Type: tea.KeyCtrlO}, ownerOpenChart},
		{"ctrl+s", tea.KeyMsg{Type: tea.KeyCtrlS}, ownerSaveChart},
		{"ctrl+b", tea.KeyMsg{Type: tea.KeyCtrlB}, ownerBodies},
		{"ctrl+h", tea.KeyMsg{Type: tea.KeyCtrlH}, ownerHouses},
		{"ctrl+z", tea.KeyMsg{Type: tea.KeyCtrlZ}, ownerZodiac},
		{"ctrl+a", tea.KeyMsg{Type: tea.KeyCtrlA}, ownerAspects},
		{"?", runes("?"), ownerHelp},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := onTimeline(t, Config{Accessible: true})
			m.aspectRegistry, m.objectRegistry = testRegistry(), testObjectRegistry()
			m, _ = tpress(t, m, keyDown)
			m, _ = tpress(t, m, keyDown)
			tab, row := m.activeTabID(PanelDetails), m.details.cursor
			if m.currentOwner() != ownerDetails {
				t.Fatalf("fixture: owner %v, want attached Details", m.currentOwner())
			}

			m, _ = tpress(t, m, c.key)
			if got := m.currentOwner(); got != c.want {
				t.Fatalf("%s in Details: owner %v, want the popup %v", c.name, got, c.want)
			}

			m, _ = tpress(t, m, keyEsc)
			if got := m.currentOwner(); got != ownerDetails || !m.details.attached {
				t.Fatalf("Esc in the popup: owner %v attached=%v, want back in Details", got, m.details.attached)
			}
			if m.activeTabID(PanelDetails) != tab || m.details.cursor != row {
				t.Errorf("back in Details on tab %q row %d, want tab %q row %d",
					m.activeTabID(PanelDetails), m.details.cursor, tab, row)
			}
		})
	}
}
