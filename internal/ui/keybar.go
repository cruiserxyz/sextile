package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// footerIndicator resolves the footer's right-hand slot for one frame. While
// a leader waits the bar is the comma menu and shows no indicator: a notice or
// the error mark due meanwhile shows when the menu closes.
func (m Model) footerIndicator() string {
	switch {
	case m.leader.pending:
		return ""
	case m.yank.active && m.yank.notice != "":
		return m.yank.notice
	case m.yank.active:
		return keys.YankFlashText(m.yank.chars, m.cfg)
	case m.errorUnseen && !m.logOnScreen():
		return keys.ErrorMarkTextView(m.leaderKey, m.cfg, m.errorKeptView)
	}
	return ""
}

// logOnScreen reports whether this frame's layout has a Log panel.
func (m Model) logOnScreen() bool {
	_, ok := m.geometry()[PanelLog]
	return ok
}
