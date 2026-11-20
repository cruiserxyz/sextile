package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The first Enter in a Details filter line only closes it, query kept; the
// panel keeps the keyboard and nothing jumps. The next Enter is the row's.
func TestFirstEnterInTheFilterLineOnlyClosesIt(t *testing.T) {
	for _, k := range []timelineList{listTimeline, listMoon} {
		t.Run(k.scope(), func(t *testing.T) {
			m := onList(t, k, Config{Accessible: true})
			when := m.slots.Floating().Time
			target := m.targetFloatingTime
			m, _ = tpress(t, m, runes("/"))
			m = typeQuery(t, m, "a")
			query := m.list(k).query
			m, _ = tpress(t, m, keyEnter)
			if m.list(k).searching || m.list(k).query != query || query == "" {
				t.Fatalf("after Enter: searching %v query %q, want closed with %q kept", m.list(k).searching, m.list(k).query, query)
			}
			if !m.details.attached {
				t.Error("the first Enter left Details")
			}
			if !m.slots.Floating().Time.Equal(when) || !m.targetFloatingTime.Equal(target) {
				t.Error("the first Enter jumped")
			}
			m, _ = tpress(t, m, keyEnter)
			if m.details.attached {
				t.Error("the second Enter did not jump and leave Details")
			}
		})
	}
	t.Run("report", func(t *testing.T) {
		m := attachedReport(t, false, "[0] Aspects", ownFilterBody(80))
		m, _ = tpress(t, m, runes("/"))
		m = typeQuery(t, m, "squ")
		m, _ = tpress(t, m, keyEnter)
		if m.details.searching || m.details.query != "squ" || !m.details.attached {
			t.Errorf("after Enter: searching %v query %q attached %v, want closed, squ, attached", m.details.searching, m.details.query, m.details.attached)
		}
	})
}

// Backspace on an empty query closes the line; on a query it deletes.
func TestBackspaceOnAnEmptyFilterLineClosesIt(t *testing.T) {
	back := tea.KeyMsg{Type: tea.KeyBackspace}
	for _, k := range []timelineList{listTimeline, listMoon} {
		t.Run(k.scope(), func(t *testing.T) {
			m := onList(t, k, Config{Accessible: true})
			m, _ = tpress(t, m, runes("/"))
			m, _ = tpress(t, m, back)
			if m.list(k).searching {
				t.Fatal("Backspace on an empty line left it open")
			}
			m, _ = tpress(t, m, runes("/"))
			m = typeQuery(t, m, "ab")
			m, _ = tpress(t, m, back)
			m, _ = tpress(t, m, back)
			if !m.list(k).searching || m.list(k).query != "" {
				t.Fatalf("after two Backspaces: searching %v query %q, want open and empty", m.list(k).searching, m.list(k).query)
			}
			m, _ = tpress(t, m, back)
			if m.list(k).searching {
				t.Error("the third Backspace did not close the line")
			}
		})
	}
	t.Run("report", func(t *testing.T) {
		m := attachedReport(t, false, "[0] Aspects", ownFilterBody(80))
		m, _ = tpress(t, m, runes("/"))
		m = typeQuery(t, m, "ab")
		m, _ = tpress(t, m, back)
		m, _ = tpress(t, m, back)
		if !m.details.searching {
			t.Fatal("the line closed before it was empty")
		}
		m, _ = tpress(t, m, back)
		if m.details.searching || !m.details.attached {
			t.Errorf("searching %v attached %v, want the line closed and Details attached", m.details.searching, m.details.attached)
		}
	})
}
