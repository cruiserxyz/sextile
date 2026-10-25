package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// replayBurst resolves a burst of typed runes one key at a time, through the
// whole Update path, exactly as if they had been typed slowly
// (the specification C14).
//
// The terminal reader hands over everything it read in one go, so `jjjj`
// typed faster than a read — key repeat, a macro — arrives as ONE KeyRunes
// whose String() is "jjjj". A table matches none of it, so without this the
// whole burst was dropped: on the main screen, in the console, in attached
// Details.
//
// Going back through Update rather than one owner's handler means a key that
// changes who owns the keyboard hands the rest of the burst to the new owner,
// as slow typing would: `?jj` opens help and then moves in it. A quit ends the
// burst: the session is written and nothing after it should act. Whether a
// key quits is asked of the owner that has the keyboard when it arrives, so a
// `q` typed into a popup's search line stays a `q`.
//
// A bracketed paste is not a burst: it is text, and replaying it as keys
// would let a pasted `yy` copy or a pasted `q` quit. isBurst leaves it one
// key, which a text line types and every other owner ignores.
func (m Model) replayBurst(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cur tea.Model = m
	for _, r := range msg.Runes {
		one := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: msg.Alt}
		if cur.(Model).quitsOn(one) {
			var quit tea.Cmd
			cur, quit = cur.Update(one)
			return cur, tea.Batch(append(cmds, quit)...)
		}
		var cmd tea.Cmd
		cur, cmd = cur.Update(one)
		cmds = append(cmds, cmd)
	}
	return cur, tea.Batch(cmds...)
}

// quitsOn reports whether msg would quit from where the keyboard is now: `q`
// on the main screen with no leader pending.
func (m Model) quitsOn(msg tea.KeyMsg) bool {
	if m.currentOwner() != ownerMain || m.leader.pending {
		return false
	}
	if m.leaderKey != "" && msg.String() == m.leaderKey {
		return false
	}
	return matchBinding(msg, m.available(m.keys.mainBindings())) == BindingQuit
}

// isBurst reports whether msg is several typed runes read at once. Only a
// typed key carries runes, and a named key at most one.
func isBurst(msg tea.KeyMsg) bool {
	return !msg.Paste && len(msg.Runes) > 1
}
