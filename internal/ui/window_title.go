package ui

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/store"
)

// The terminal window's title is "Sextile". With `window_title: session` in
// preferences.yaml it is "<session name> · Sextile", and the unnamed session
// shows "Sextile" alone. Nothing else about a session reaches the title.

const (
	windowTitleDefault = "Sextile"
	windowTitleLimit   = 60
)

// applyWindowTitle takes the window_title key: `session`, or the default when
// the key is absent. Any other word is refused with one Log note and the default
// holds.
func (m Model) applyWindowTitle(c store.WindowTitleConfig) Model {
	m.titleSession = false
	if !c.Present {
		return m
	}
	if strings.ToLower(strings.TrimSpace(c.Value)) == "session" {
		m.titleSession = true
		return m
	}
	return m.logNote("settings", "window_title %q not valid, the title stays Sextile", c.Value)
}

// windowTitle is the title the window should carry now. A session name goes
// through titleText, so a control byte in it cannot end the title early.
func (m Model) windowTitle() string {
	if m.titleSession && m.named.running() && m.named.name != "" {
		return titleText(m.named.name) + " \u00b7 " + windowTitleDefault
	}
	return windowTitleDefault
}

// titleText is s with every control character made a space, cut to a length a
// window title can hold.
func titleText(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	if r := []rune(s); len(r) > windowTitleLimit {
		s = string(r[:windowTitleLimit-1]) + "\u2026"
	}
	return s
}

// withTitle asks for the title again when it differs from the one last sent.
// Init sends the default, so a model that never changes it starts no command.
func (m Model) withTitle(cmd tea.Cmd) (Model, tea.Cmd) {
	sent := m.titleSent
	if sent == "" {
		sent = windowTitleDefault
	}
	want := m.windowTitle()
	if want == sent {
		return m, cmd
	}
	m.titleSent = want
	return m, tea.Batch(cmd, tea.SetWindowTitle(want))
}
