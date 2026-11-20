package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/store"
)

// All chart values in this file are constructed synthetic test inputs.

// Every popup draws its whole key bar at 80 columns: no entry is cut (the bar
// shows no `…`) and the row is not wider than the terminal. The main screen's,
// Details' and the Log panel's bars are not popups: the main bar already drops its
// unpinned entries at this width, by design.
func TestEveryOwnersKeyBarFitsEightyColumns(t *testing.T) {
	const w, h = 80, 24
	cfg := Config{Accessible: true}
	seen := map[owner]bool{}
	check := func(name string, m Model) {
		t.Helper()
		row := lastRow(m.View())
		if strings.Contains(row, "…") || strings.Contains(row, "...") {
			t.Errorf("%s: the key bar is cut: %q", name, row)
		}
		if got := ansi.StringWidth(row); got > w {
			t.Errorf("%s: the key bar is %d cells wide, want at most %d: %q", name, got, w, row)
		}
		if strings.TrimSpace(row) == "" {
			t.Errorf("%s: the key bar is empty", name)
		}
	}
	extra := []keyOwnerCase{
		{"houses", ownerHouses, "", func(t *testing.T, m Model) Model {
			m, _ = escSend(t, m, escRunes(","), escRunes("u"))
			return m
		}},
		{"zodiac", ownerZodiac, "", func(t *testing.T, m Model) Model {
			m, _ = escSend(t, m, escKey(tea.KeyCtrlZ))
			return m
		}},
		{"experiments", ownerExperiments, "", func(t *testing.T, m Model) Model {
			tm, _ := m.openExperiments()
			return tm.(Model)
		}},
	}
	for _, c := range append(keyOwnerCases(), extra...) {
		if c.owner == ownerMain || c.owner == ownerLog || c.owner == ownerDetails {
			seen[c.owner] = true
			continue
		}
		m := reachOwner(t, c, cfg, w, h)
		seen[c.owner] = true
		check(c.name, m)
	}

	// The Sessions list on a saved session, where Rename and Delete show; the
	// name field; and the question asked before a new session starts.
	freshModels(t)
	stubHouseLookups(t)
	sessionSandbox(t)
	otherSession(t, "Alpha", "A")
	m := listModel(t, 2, 0)
	t.Cleanup(store.ReleaseInstanceLock)
	m.cfg = cfg
	m.width, m.height = w, h
	m = openSessions(t, m)
	m, _ = press(t, m, "down", "down")
	if _, ok := m.sessionRow(); !ok {
		t.Fatal("premise: the cursor is not on a saved session")
	}
	if got := m.currentOwner(); got != ownerSessions {
		t.Fatalf("owner %d, want the Sessions list", got)
	}
	seen[ownerSessions] = true
	check("sessions on a saved session", m)
	m, _ = press(t, m, "up")
	check("sessions on the New row", m)
	m, _ = press(t, m, "enter")
	if !m.sessions.Asking {
		t.Fatal("premise: the question is not showing")
	}
	check("the new-session question", m)
	m, _ = press(t, m, "y")
	if got := m.currentOwner(); got != ownerSessionName {
		t.Fatalf("owner %d, want the name field", got)
	}
	seen[ownerSessionName] = true
	check("the session name field", m)

	for o := ownerMain; o <= ownerExperiments; o++ {
		if !seen[o] {
			t.Errorf("owner %d has no bar-fit case: add its popup here", int(o))
		}
	}
}
