package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// footerIndicator resolves the footer's right-hand slot for one frame. A
// pending leader outranks a copy confirmation: the leader is a live state the
// operator is in the middle of, the confirmation is a receipt for something
// already finished. They can overlap — copy in the console, Esc out, arm a
// leader, all inside yankFlashDuration — and arming also drops the receipt, so
// this ordering is the second of two guards rather than the only one.
func (m Model) footerIndicator() string {
	switch {
	case m.leader.pending:
		return keys.LeaderPendingText(m.leaderKey, m.leader.seq, m.cfg)
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
