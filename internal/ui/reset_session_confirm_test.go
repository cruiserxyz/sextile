package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// All chart values in this file are constructed synthetic test inputs.

// Rule pinned (operator, 2026-11-07, release survey): `, n` with something to
// lose does not reset at once. The first press shows a footer notice and arms
// a second press; only a second `, n` resets. Any other key disarms.

// pressReset is `, n` as a reset: twice when the session holds something to
// lose, once when it is empty (then one press is the whole command).
func pressReset(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	if m.blocked(keys.BindingNewSession) == "" {
		return press(t, m, ",", "n", ",", "n")
	}
	return press(t, m, ",", "n")
}

func armedModel(t *testing.T) Model {
	t.Helper()
	freshModels(t)
	stubHouseLookups(t)
	m := sessionsModel(t, 2)
	t.Cleanup(store.ReleaseInstanceLock)
	return m
}

// Rule: the first `, n` closes nothing, says so in the footer, logs nothing.
func TestResetFirstPressOnlyWarns(t *testing.T) {
	m := armedModel(t)
	before := len(logTexts(m))
	m, _ = press(t, m, ",", "n")
	if got := m.slots.AnchoredCount(); got != 2 {
		t.Fatalf("first `, n` left %d charts, want 2", got)
	}
	if got, want := m.footerIndicator(), ", n again closes 2 charts"; got != want {
		t.Errorf("notice %q, want %q", got, want)
	}
	if got := len(logTexts(m)); got != before {
		t.Errorf("Log gained %v on the first press", logTexts(m)[before:])
	}
}

// Rule: a second `, n` while armed resets as before.
func TestResetSecondPressResets(t *testing.T) {
	m := armedModel(t)
	m, _ = press(t, m, ",", "n", ",", "n")
	if got := m.slots.AnchoredCount(); got != 0 {
		t.Errorf("after two `, n` %d charts remain", got)
	}
	if logHas(m, "session: new, unnamed") != 1 {
		t.Errorf("no reset Log line: %v", logTexts(m))
	}
}

// Rule: any other key disarms; the next `, n` is a first press again.
func TestResetOtherKeyDisarms(t *testing.T) {
	m := armedModel(t)
	m, _ = press(t, m, ",", "n", "j", ",", "n")
	if got := m.slots.AnchoredCount(); got != 2 {
		t.Errorf("`, n` j `, n` closed charts: %d remain, want 2", got)
	}
}

// Rule: the Keys popup and the Actions list say the second press after the
// keys, `,n (twice)`, and the title is the action alone (operator, 2026-11-07:
// "the 'twice' should sit after the , n").
func TestResetKeySpellingSaysTwice(t *testing.T) {
	m := armedModel(t)
	var row string
	for _, l := range keys.HelpLines(m.keysContextFor(ownerHelp)) {
		if l.Binding && strings.HasPrefix(l.Text, "New session ") {
			row = l.Text
		}
	}
	if !strings.HasSuffix(row, "  ,n (twice)") || strings.Contains(row, "session (twice)") {
		t.Errorf("Keys popup row %q, want `New session` then `,n (twice)`", row)
	}
}

// Rule: with nothing to lose `, n` is unchanged: one press, the Log line, no notice.
func TestResetEmptySessionUnchanged(t *testing.T) {
	freshModels(t)
	stubHouseLookups(t)
	sessionSandbox(t)
	m := listModel(t, 0, 0)
	if !m.sessionIsEmpty() {
		t.Skip("fixture is not empty")
	}
	m, _ = press(t, m, ",", "n")
	if logHas(m, "session: new, unnamed") != 1 {
		t.Errorf("Log lacks the reset line: %v", logTexts(m))
	}
	if m.yank.active {
		t.Errorf("notice shown on an empty session: %q", m.footerIndicator())
	}
}
