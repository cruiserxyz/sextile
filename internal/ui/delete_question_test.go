package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/store"
)

// trySessionList is the Sessions list with one saved session, try3, under the cursor.
func trySessionList(t *testing.T) Model {
	t.Helper()
	m := sessionsModel(t, 1)
	otherSession(t, "try3", "Client Zed")
	m = openSessions(t, m)
	for i, r := range m.sessions.Rows {
		if r.File == "try3.yaml" {
			m.sessions.Cursor = i + 2
			return m
		}
	}
	t.Fatalf("try3 is not listed: %+v", m.sessions.Rows)
	return m
}

func trashed(t *testing.T) []string {
	t.Helper()
	got, _ := filepath.Glob(filepath.Join(store.SessionsDir(), ".trash", "try3-*.yaml"))
	return got
}

// (a) Deleting a session asks first, from Ctrl+D and from `, d`.
func TestDeletingASessionAsksFirst(t *testing.T) {
	for _, via := range []string{"ctrl+d", ", d"} {
		t.Run(via, func(t *testing.T) {
			ask := func() Model {
				m := trySessionList(t)
				if via == "ctrl+d" {
					m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
				} else {
					m, _ = press(t, m, ",", "d")
				}
				return m
			}
			m := ask()
			v := ansi.Strip(m.View())
			for _, want := range []string{"Delete session", "Move try3 to the trash?", "Sextile cannot bring it back.", "[ No (n) ]", "Yes (y)"} {
				if !strings.Contains(v, want) {
					t.Fatalf("the question lacks %q:\n%s", want, v)
				}
			}
			if len(trashed(t)) != 0 {
				t.Fatal("the session was trashed before the answer")
			}
			m, _ = press(t, m, "enter") // No is lit
			if !slices.ContainsFunc(m.sessions.Rows, func(r store.NamedSession) bool { return r.File == "try3.yaml" }) {
				t.Errorf("No took try3 out of the list")
			}
			if _, err := os.Stat(store.NamedSessionPath("try3.yaml")); err != nil || len(trashed(t)) != 0 || m.sessions.Asking {
				t.Errorf("Enter on No: file %v, trash %v, asking %v", err, trashed(t), m.sessions.Asking)
			}
			m = ask()
			m, _ = press(t, m, "y")
			if len(trashed(t)) != 1 {
				t.Errorf("Yes did not move try3 to the trash: %v", trashed(t))
			}
		})
	}
}

// (b) The four questions behave alike, from one table each.
func TestQuestionsShareOneBehaviour(t *testing.T) {
	labels := func(q question) []string {
		var out []string
		for _, b := range q.Buttons {
			out = append(out, b.Label)
		}
		return out
	}
	for _, c := range []struct {
		name      string
		q         question
		esc, open string
	}{
		{"first write", questionFirstWrite, buttonNo, buttonNo},
		{"change", questionChange, buttonNo, buttonNo},
		{"new session", questionNewSession, buttonCancel, buttonYes},
		{"delete session", questionDelete, buttonNo, buttonNo},
	} {
		t.Run(c.name, func(t *testing.T) {
			q := c.q
			names := labels(q)
			if got := names[q.Open]; got != c.open {
				t.Errorf("lit on open is %s, want %s", got, c.open)
			}
			for lit := range q.Buttons {
				if got, took := q.key(tea.KeyMsg{Type: tea.KeyEnter}, lit); got != lit || took != names[lit] {
					t.Errorf("Enter on %d took %q", lit, took)
				}
			}
			for _, b := range q.Buttons {
				k := keyMsg(b.Key)
				if b.Key != "esc" {
					k = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(b.Key)}
				}
				if _, took := q.key(k, 0); took != b.Label {
					t.Errorf("%q took %q, want %q", b.Key, took, b.Label)
				}
			}
			if _, took := q.key(keyMsg("esc"), 0); took != c.esc {
				t.Errorf("Esc took %q, want %q", took, c.esc)
			}
			last := len(q.Buttons) - 1
			for _, move := range []struct {
				msg        tea.KeyMsg
				from, want int
			}{
				{tea.KeyMsg{Type: tea.KeyRight}, 0, 1}, {tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")}, 0, 1},
				{tea.KeyMsg{Type: tea.KeyLeft}, last, last - 1}, {tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")}, last, last - 1},
			} {
				if got, took := q.key(move.msg, move.from); got != move.want || took != "" {
					t.Errorf("%v from %d: lit %d, took %q", move.msg, move.from, got, took)
				}
			}
			for _, other := range []string{"x", "q", "c", "z", "1", ","} {
				if got, took := q.key(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(other)}, 0); got != 0 || took != "" {
					t.Errorf("%q did something: lit %d, took %q", other, got, took)
				}
			}
		})
	}
}

// (c) New session's Cancel shows esc, and c does nothing.
func TestNewSessionCancelIsEscNotC(t *testing.T) {
	m := askingModel(t)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Cancel (esc)") || strings.Contains(v, "Cancel (c)") {
		t.Errorf("the Cancel button reads wrongly:\n%s", v)
	}
	m, _ = press(t, m, "c")
	if !m.sessions.Asking || m.slots.AnchoredCount() != 2 {
		t.Errorf("c answered the question: asking %v, %d charts", m.sessions.Asking, m.slots.AnchoredCount())
	}
}

// (d) Del on a chart still asks nothing.
func TestDelOnAChartAsksNothing(t *testing.T) {
	m := listModel(t, 3, 1)
	before := len(m.slots.All())
	m = send(t, m, keyDelete)
	if m.sessions.Asking || m.startup == startupSessionsOpen || len(m.slots.All()) != before-1 {
		t.Errorf("Del: asking %v, startup %q, %d slots of %d", m.sessions.Asking, m.startup, len(m.slots.All()), before)
	}
}
