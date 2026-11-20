package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// All chart values in this file are constructed synthetic test inputs.

// The Sessions list's second row starts a new, unnamed session. An unnamed
// session holding a chart is asked about first, in the button popup.

// armedModel is an unnamed window with two charts open and the unnamed
// session's lock held.
func armedModel(t *testing.T) Model {
	t.Helper()
	freshModels(t)
	stubHouseLookups(t)
	m := sessionsModel(t, 2)
	t.Cleanup(store.ReleaseInstanceLock)
	return m
}

// onNewRow is `, O` with the cursor on the New row.
func onNewRow(t *testing.T, m Model) Model {
	t.Helper()
	m = openSessions(t, m)
	m, _ = press(t, m, "down")
	if !m.sessions.OnNewRow() {
		t.Fatalf("the cursor is on row %d, want the New row", m.sessions.Cursor)
	}
	return m
}

// pressNewSession is Enter on the New row of the Sessions list. An unnamed
// session with charts is then asked: n answers No, so the new session starts.
func pressNewSession(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	m = onNewRow(t, m)
	m, cmd := press(t, m, "enter")
	if m.sessions.Asking {
		m, cmd = press(t, m, "n")
	}
	return m, cmd
}

// pressReset is pressNewSession, kept under the name the reset tests use.
func pressReset(t *testing.T, m Model) (Model, tea.Cmd) { return pressNewSession(t, m) }

func askingModel(t *testing.T) Model {
	t.Helper()
	m, _ := press(t, onNewRow(t, armedModel(t)), "enter")
	if !m.sessions.Asking {
		t.Fatal("premise: the question is not showing")
	}
	return m
}

func TestSessionsListShowsTheTwoActionRowsBeforeTheSessions(t *testing.T) {
	freshModels(t)
	sessionSandbox(t)
	otherSession(t, "Alpha", "A")
	m := openSessions(t, listModel(t, 0, 0))
	t.Cleanup(store.ReleaseInstanceLock)
	rows := sessionsRows(m)
	if len(rows) != 3 {
		t.Fatalf("%d rows, want Save, New and one session", len(rows))
	}
	if !strings.HasPrefix(rows[0].Left, "+ Save this session as") || rows[1].Left != "+ New, unnamed session" || rows[2].Left != "Alpha" {
		t.Errorf("rows = %q, %q, %q", rows[0].Left, rows[1].Left, rows[2].Left)
	}
	v := ansi.Strip(m.View())
	if strings.Index(v, "+ Save this session") > strings.Index(v, "+ New, unnamed session") || strings.Index(v, "+ New, unnamed session") > strings.Index(v, "Alpha") {
		t.Errorf("the rows are not drawn in that order:\n%s", v)
	}
}

func TestNewRowFromANamedSessionStartsAtOnceWithNoQuestion(t *testing.T) {
	m := namedResetModel(t)
	m = onNewRow(t, m)
	m, _ = press(t, m, "enter")
	if m.sessions.Asking {
		t.Error("a named session was asked about")
	}
	if m.named.running() || m.slots.AnchoredCount() != 0 {
		t.Errorf("named %v, %d charts: want a new, unnamed session", m.named.running(), m.slots.AnchoredCount())
	}
	if m.startup == startupSessionsOpen {
		t.Error("the list is still open")
	}
	if got := lastRecorded(t); got != "" {
		t.Errorf("the last-session record names %q, want none", got)
	}
}

func TestNewRowFromAnUnnamedSessionWithChartsAsksWithYesLit(t *testing.T) {
	m := askingModel(t)
	v := ansi.Strip(m.View())
	for _, want := range []string{"Save this session first?", "[ Yes (y) ]", "No (n)", "Cancel (esc)"} {
		if !strings.Contains(v, want) {
			t.Errorf("the question lacks %q:\n%s", want, v)
		}
	}
	if body := newSessionChoice(m).Body; len(body) != 2 || body[1] != "Its 2 charts are open under no name. No forgets which were open. Their files stay." {
		t.Errorf("question body = %q", body)
	}
	if m.slots.AnchoredCount() != 2 {
		t.Errorf("%d charts after only asking", m.slots.AnchoredCount())
	}
}

func TestNewSessionQuestionNoStartsTheNewSessionAndClosesTheCharts(t *testing.T) {
	m := askingModel(t)
	m, _ = press(t, m, "n")
	if m.slots.AnchoredCount() != 0 || m.named.running() || m.sessions.Asking || m.startup == startupSessionsOpen {
		t.Errorf("charts %d, named %v, asking %v, startup %q", m.slots.AnchoredCount(), m.named.running(), m.sessions.Asking, m.startup)
	}
	if logHas(m, "session: new session with no name") != 1 {
		t.Errorf("no Log line: %v", logTexts(m))
	}
}

func TestNewSessionQuestionCancelAndEscReturnToTheList(t *testing.T) {
	for _, k := range []string{"esc"} {
		m := askingModel(t)
		m, _ = press(t, m, k)
		if m.sessions.Asking || m.startup != startupSessionsOpen || m.slots.AnchoredCount() != 2 {
			t.Errorf("%s: asking %v, startup %q, %d charts", k, m.sessions.Asking, m.startup, m.slots.AnchoredCount())
		}
	}
}

func TestNewSessionQuestionArrowsMoveTheLitButtonAndEnterTakesIt(t *testing.T) {
	m := askingModel(t)
	m, _ = escSend(t, m, escKey(tea.KeyRight))
	if m.sessions.Lit != 1 {
		t.Fatalf("lit %d after →, want No", m.sessions.Lit)
	}
	m, _ = press(t, m, "enter")
	if m.slots.AnchoredCount() != 0 {
		t.Errorf("Enter on No left %d charts", m.slots.AnchoredCount())
	}
	m = askingModel(t)
	m, _ = escSend(t, m, escKey(tea.KeyRight), escKey(tea.KeyRight), escKey(tea.KeyRight), escKey(tea.KeyEnter))
	if m.sessions.Asking || m.startup != startupSessionsOpen || m.slots.AnchoredCount() != 2 {
		t.Errorf("Enter on Cancel did not return to the list")
	}
}

func TestNewSessionQuestionYesSavesUnderANameThenStartsTheNewSession(t *testing.T) {
	m := askingModel(t)
	m, _ = press(t, m, "y")
	if m.startup != startupSessionNameOpen {
		t.Fatalf("Yes opened %q, want the name field", m.startup)
	}
	m = clearField(t, m, 80)
	m = typeText(t, m, "Mine")
	m, _ = press(t, m, "enter")
	if m.startup == startupSessionNameOpen || m.startup == startupSessionsOpen {
		t.Fatalf("the save did not finish (startup %q, refusal %q)", m.startup, m.sessions.Refusal)
	}
	if m.slots.AnchoredCount() != 0 || m.named.running() {
		t.Errorf("after the save: %d charts, named %v; want a new, unnamed session", m.slots.AnchoredCount(), m.named.running())
	}
	if got := fileText(t, store.NamedSessionPath("mine.yaml")); !strings.Contains(got, "Synthetic") {
		t.Errorf("the saved session holds no chart:\n%s", got)
	}
	if got := lastRecorded(t); got != "" {
		t.Errorf("the last-session record names %q", got)
	}
}

func TestNewSessionQuestionATakenNameKeepsTheFieldAndStartsNothing(t *testing.T) {
	m := askingModel(t)
	otherSession(t, "Taken", "T")
	m, _ = press(t, m, "y")
	m = clearField(t, m, 80)
	m = typeText(t, m, "Taken")
	m, _ = press(t, m, "enter")
	if m.startup != startupSessionNameOpen || m.sessions.Refusal == "" {
		t.Errorf("startup %q, refusal %q: want the field with its refusal", m.startup, m.sessions.Refusal)
	}
	if m.slots.AnchoredCount() != 2 || m.named.running() {
		t.Errorf("%d charts, named %v: nothing should have started", m.slots.AnchoredCount(), m.named.running())
	}
}

func TestEscInTheNameFieldAfterYesStartsNothing(t *testing.T) {
	m := askingModel(t)
	m, _ = press(t, m, "y", "esc")
	if m.startup != startupSessionsOpen || m.sessions.Asking || m.slots.AnchoredCount() != 2 {
		t.Errorf("startup %q, asking %v, %d charts", m.startup, m.sessions.Asking, m.slots.AnchoredCount())
	}
	// A later plain Save as must not start a new session.
	m, _ = press(t, m, "up", "enter")
	m = clearField(t, m, 80)
	m = typeText(t, m, "Plain")
	m, _ = press(t, m, "enter")
	if m.slots.AnchoredCount() != 2 || !m.named.running() {
		t.Errorf("a plain Save as left %d charts, named %v", m.slots.AnchoredCount(), m.named.running())
	}
}

func TestNewRowFromAnUnnamedSessionWithNoChartAsksNothing(t *testing.T) {
	freshModels(t)
	stubHouseLookups(t)
	m := sessionsModel(t, 0)
	t.Cleanup(store.ReleaseInstanceLock)
	m = onNewRow(t, m)
	m, _ = press(t, m, "enter")
	if m.sessions.Asking || m.startup == startupSessionsOpen {
		t.Errorf("asking %v, startup %q", m.sessions.Asking, m.startup)
	}
}

func TestNoKeyAndNoActionsRowStartsANewSession(t *testing.T) {
	m := armedModel(t)
	for _, r := range openedKeybindings(m).keybindings.All {
		if r.ID == keys.BindingNewSession {
			t.Error("Keybindings has a New session row")
		}
	}
	m, _ = press(t, m, ",", "n")
	if m.slots.AnchoredCount() != 2 || m.sessions.Asking || m.startup != startupReady {
		t.Errorf("a comma and n changed something: %d charts, asking %v, startup %q", m.slots.AnchoredCount(), m.sessions.Asking, m.startup)
	}
	for _, c := range keys.Commands() {
		if c.ID == keys.BindingNewSession {
			t.Errorf("the key table gives New session the letter %q", c.Letter)
		}
	}
}
