package ui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/store"
)

// windowTitlesSet runs cmd and returns the window titles it sets, in order.
func windowTitlesSet(cmd tea.Cmd) []string {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []string
		for _, c := range b {
			out = append(out, windowTitlesSet(c)...)
		}
		return out
	}
	if reflect.TypeOf(msg).Name() == "setWindowTitleMsg" {
		return []string{fmt.Sprint(msg)}
	}
	return nil
}

var sessionTitle = store.WindowTitleConfig{Value: "session", Present: true}

// The title is "Sextile" unless window_title asks for the session; then a named
// session reads "<name> · Sextile" and the unnamed one "Sextile" alone.
func TestWindowTitleText(t *testing.T) {
	m := namedResetModel(t)
	if got := m.windowTitle(); got != "Sextile" {
		t.Errorf("setting absent: %q, want Sextile", got)
	}
	m = m.applyWindowTitle(sessionTitle)
	if got, want := m.windowTitle(), resetNamed+" · Sextile"; got != want {
		t.Errorf("named: %q, want %q", got, want)
	}
	m.named.name = ""
	if got := m.windowTitle(); got != "Sextile" {
		t.Errorf("unnamed: %q, want Sextile", got)
	}
	m.named.name = "a\x1b]0;x\x07b"
	if got := m.windowTitle(); strings.ContainsAny(got, "\x1b\x07") {
		t.Errorf("a control byte reached the title: %q", got)
	}
}

// The title follows a switch: a new session sets "Sextile" once, and nothing is
// set while the title does not change.
func TestWindowTitleFollowsASwitch(t *testing.T) {
	m := namedResetModel(t).applyWindowTitle(sessionTitle)
	named := resetNamed + " \u00b7 Sextile"
	if got := windowTitlesSet(func() tea.Cmd { _, c := m.withTitle(nil); return c }()); len(got) != 1 || got[0] != named {
		t.Fatalf("a named session sets %q, want %q once", got, named)
	}
	m.titleSent = named
	m, _ = pressReset(t, m)
	if m.titleSent != "Sextile" {
		t.Errorf("after a new session the title sent is %q, want Sextile", m.titleSent)
	}
	if _, cmd := m.withTitle(nil); cmd != nil {
		t.Errorf("an unchanged title was set again: %q", windowTitlesSet(cmd))
	}
	m.named.name = "Other"
	if _, cmd := m.withTitle(nil); cmd != nil {
		t.Errorf("a session that is not running changed the title")
	}
}

// A value other than session gives one settings line and the default holds.
func TestAWrongWindowTitleValueIsRefusedOnce(t *testing.T) {
	m := namedResetModel(t)
	n := len(m.logPanelLines())
	m = m.applyWindowTitle(store.WindowTitleConfig{Value: "chart", Present: true})
	got := m.logPanelLines()[n:]
	if len(got) != 1 || got[0] != `Settings window_title "chart" not valid, the title stays Sextile` {
		t.Errorf("Log lines = %q", got)
	}
	if m.windowTitle() != "Sextile" {
		t.Errorf("title = %q, want the default", m.windowTitle())
	}
}
