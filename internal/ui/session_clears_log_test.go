package ui

import (
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/store"
)

// A new session, and opening another one, start with an empty Log panel and no
// error mark in the footer. The console keeps every line, and the line that
// announces the session is the panel's only one.
func TestANewSessionStartsWithAnEmptyLog(t *testing.T) {
	const old = "an old thing was not read"
	check := func(t *testing.T, m Model, announce string) {
		t.Helper()
		panel := m.logPanelLines()
		if len(panel) != 1 || !strings.HasPrefix(panel[0], announce) {
			t.Errorf("the Log panel holds %q, want only the line starting %q", panel, announce)
		}
		if m.errorUnseen {
			t.Error("the footer still marks an unseen error")
		}
		if !strings.Contains(strings.Join(consoleLineTexts(m), "\n"), old) {
			t.Errorf("the console lost %q: %q", old, consoleLineTexts(m))
		}
	}
	t.Run("new session", func(t *testing.T) {
		m := namedResetModel(t)
		m = m.logNote("charts", "a note from before").logError("chart", "%s", old)
		if !m.errorUnseen {
			t.Fatal("fixture: no unseen error")
		}
		m = resetKey(t, m)
		check(t, m, "Session  new session")
	})
	t.Run("opening another session", func(t *testing.T) {
		m := listModel(t, 1, 0)
		m = m.logNote("charts", "a note from before").logError("chart", "%s", old)
		p := withSession(perimeter(), store.Session{Version: 3})
		m, _ = applyNamedSessionLoaded(m, NamedSessionLoadedMsg{Gen: m.sessionGen, Name: "plain", Perimeter: p})
		check(t, m, "Session  opened")
	})
}
