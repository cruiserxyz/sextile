package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/store"
)

// All chart values in this file are constructed synthetic test inputs.

func lastRecorded(t *testing.T) string {
	t.Helper()
	f, err := store.LastSession(store.ChartsDir())
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// The window records the named session it switches to at the switch, not at
// quit: Save as and opening from the list each record it, and returning to
// the unnamed session removes the record.
func TestSwitchRecordsTheLastSession(t *testing.T) {
	freshModels(t)
	stubHouseLookups(t)
	m := sessionsModel(t, 1)
	t.Cleanup(store.ReleaseInstanceLock)
	other := otherSession(t, "Other work", "Client Zed")
	if got := lastRecorded(t); got != "" {
		t.Fatalf("premise: a record before any switch: %q", got)
	}

	m = saveAs(t, m, "Mine")
	if got := lastRecorded(t); got != "mine.yaml" {
		t.Errorf("after Save as the record is %q, want mine.yaml", got)
	}

	m = openSessions(t, m)
	for i, r := range m.sessions.Rows {
		if r.File == other {
			m.sessions.Cursor = i + 2
		}
	}
	m, cmd := press(t, m, "enter")
	if got := lastRecorded(t); got != other {
		t.Errorf("after opening %s the record is %q", other, got)
	}
	m = pump(t, m, cmd)

	m = resetKey(t, m)
	if m.named.running() {
		t.Fatal("premise: the reset left the named session running")
	}
	if got := lastRecorded(t); got != "" {
		t.Errorf("after the return to the unnamed session the record is %q", got)
	}
}

// A rename in the list moves the record with the session; a trash removes it.
func TestRenameAndTrashKeepTheRecordTrue(t *testing.T) {
	m := sessionsModel(t, 1)
	t.Cleanup(store.ReleaseInstanceLock)
	otherSession(t, "Old", "Client Zed")
	if err := store.RecordLastSession(store.ChartsDir(), "old.yaml"); err != nil {
		t.Fatal(err)
	}
	m = openSessions(t, m)
	row := func(file string) {
		t.Helper()
		for i, r := range m.sessions.Rows {
			if r.File == file {
				m.sessions.Cursor = i + 2
				return
			}
		}
		t.Fatalf("%s is not listed: %+v", file, m.sessions.Rows)
	}
	row("old.yaml")
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	m = clearField(t, m, 10)
	m = typeText(t, m, "Newer")
	m, _ = press(t, m, "enter")
	if got := lastRecorded(t); got != "newer.yaml" {
		t.Errorf("after the rename the record is %q, want newer.yaml", got)
	}
	row("newer.yaml")
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
	m, _ = press(t, m, "y")
	if got := lastRecorded(t); got != "" {
		t.Errorf("after the trash the record is %q", got)
	}
	_ = m
}

// A start that runs a named session records it, so a --session start is
// reopened the next time too.
func TestStartRunningANamedSessionRecordsIt(t *testing.T) {
	sessionSandbox(t)
	otherSession(t, "Flagged", "Client Zed")
	_, name, lock, err := store.OpenNamedSession("flagged.yaml", store.FolderID(store.ChartsDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(lock.Release)
	NewModel(Config{NamedSession: lock, NamedSessionName: name})
	if got := lastRecorded(t); got != "flagged.yaml" {
		t.Errorf("the record after a start running Flagged is %q", got)
	}
}
