package ui

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/cruiserxyz/sextile/internal/ui/keys"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/store"
)

// The leader key puts every action one keystroke away, the way vim does:
// press it, the key bar shows a pending prefix, and the next key either
// completes a sequence or cancels it.
//
// The resolver below is a prefix tree from the first commit even though the
// map that ships is one key deep, so handing the operator arbitrary `g s`
// sequences later needs no change to the input model.

// defaultLeaderKey is the leader when preferences.yaml says nothing.
const defaultLeaderKey = ","

// defaultLeaderMap is the map that ships: one binding per command of the
// keys package's table, one letter deep. No user-writable map exists yet.
// Attached Details uses it too: `y` copies the whole content of whatever has
// the keyboard, so it is a main-map chord and no longer Details' alone.
func defaultLeaderMap() []keys.LeaderBinding {
	var out []keys.LeaderBinding
	for _, c := range keys.Commands() {
		seq := []string{c.Letter}
		if c.Then != "" {
			seq = append(seq, c.Then)
		}
		out = append(out, keys.LeaderBinding{Seq: seq, ID: c.ID})
	}
	return out
}

// mustDefaultLeaderMap builds the shipped map. An ambiguous compiled-in map
// is a build-time mistake, not a runtime condition, so this falls back to an
// empty tree rather than taking the process down — TestDefaultLeaderMapBuilds
// is what catches it.
func mustDefaultLeaderMap() *keys.LeaderNode {
	root, err := keys.NewLeaderMap(defaultLeaderMap())
	if err != nil {
		return &keys.LeaderNode{}
	}
	return root
}

// leaderState is the pending prefix: whether one is armed and what has been
// typed since. A pending leader waits until a key or Esc.
type leaderState struct {
	pending bool
	seq     []string
}

func isStrandingKey(k string) bool {
	for _, s := range strandingKeys() {
		if k == s {
			return true
		}
	}
	return false
}

// applyKeyConfig turns what the reader found into the active configuration.
// The reader parses and reports; every default, every refusal and every Log
// line is decided here.
//
// Silence is the contract for the good cases: a valid leader, or none, passes
// without a word. Only a refusal or a shadowed binding is worth the operator's
// attention. The file's keys.timeoutlen is still read and kept (the
// preferences file must not lose a line it holds); nothing uses it.
func (m Model) applyKeyConfig(cfg store.KeysConfig) Model {
	for _, r := range cfg.Rejections {
		m = m.logNote("settings", "%s %q not valid, ignored", r.Field, r.Value)
	}
	if !cfg.LeaderPresent {
		return m
	}
	switch {
	case isStrandingKey(cfg.Leader):
		return m.logNote("settings", "%q cannot start a command, it stays %q",
			cfg.Leader, m.leaderKey)
	case utf8.RuneCountInString(cfg.Leader) != 1:
		return m.logNote("settings", "%q is not one key, the command key stays %q",
			cfg.Leader, m.leaderKey)
	}
	if shadowed := keys.ShadowedBinding(mainBindings(m.keys), cfg.Leader); shadowed != "" {
		m = m.logNote("settings", "%q now starts a command, %s is shadowed", cfg.Leader, shadowed)
	}
	m.leaderKey = cfg.Leader
	return m
}

// armLeader enters the pending state with nothing typed yet. The keystroke is consumed: a leader press is never also a main-mode
// key, which is what makes a configured leader able to shadow one.
func (m Model) armLeader() (Model, tea.Cmd) {
	// The operator has moved on from whatever was copied; the footer slot is
	// the pending leader's now. The flash tick still in flight finds the
	// confirmation already cleared and does nothing.
	m.yank.active = false
	m.leader.pending = true
	m.leader.seq = nil
	return m, nil
}

// clearLeader drops the pending prefix.
func (m Model) clearLeader() Model {
	m.leader.pending = false
	m.leader.seq = nil
	return m
}

// limitedLeader is whether the command line over owner o takes only some of the
// chords: the lists with a filter, the open search lines of Details, and the Log
// console. The main screen, attached Details and the focused Log take every one.
func (m Model) limitedLeader(o owner) bool {
	switch o {
	case ownerCity, ownerOpenChart, ownerBodies, ownerAspects, ownerHouses, ownerZodiac, ownerNodes, ownerKeybindings, ownerConsole, ownerSessions:
		return true
	case ownerDetails:
		return m.detailsSearchLineOpen()
	}
	return false
}

// commandsOver is the one place that says what the command line offers over an
// owner that takes only some of the chords: exactly the entries its own table
// marks as chords, each one that can run now. The menu lists them with their
// own words, and they are the only ones that run. Where that is nothing, the
// leader key does nothing over the owner: no command line, and no text in the
// filter. Copy is offered over Details' search lines and the console, where it
// copies their own content, and over no list: it would copy the panel under it.
func (m Model) commandsOver(o owner) []keys.BindingID {
	if !m.limitedLeader(o) {
		return nil
	}
	var offered []keys.BindingID
	for _, e := range m.ownerBindings(o) {
		if !e.Chord || !m.leaderRuns(e.ID) || m.blocked(e.ID) != "" {
			continue
		}
		offered = append(offered, e.ID)
	}
	return offered
}

// twinsOver are the comma forms of the Ctrl keys owner o binds, for the facts c
// of its entries: each entry with a Ctrl+letter key that is available now has
// the leader then that letter run it, and the menu names it with the entry's own
// word. Five letters mean one act everywhere (o open, s save, y copy, r rename,
// d delete), and no table binds one of them to another.
func (m Model) twinsOver(o owner, c keys.Context) []keys.Twin {
	if !m.limitedLeader(o) {
		return nil
	}
	var out []keys.Twin
	for _, e := range m.ownerBindings(o) {
		if e.When != nil && !e.When(c) {
			continue
		}
		for _, k := range e.Binding.Keys() {
			if letter, ok := keys.CtrlLetter(k); ok {
				out = append(out, keys.Twin{Letter: letter, Key: k, Word: strings.ToLower(e.TitleFor(c))})
			}
		}
	}
	return out
}

// leaderRuns is whether a chord of the leader map runs id.
func (m Model) leaderRuns(id keys.BindingID) bool {
	return slices.ContainsFunc(m.leaderBindings, func(b keys.LeaderBinding) bool { return b.ID == id })
}

// detailsSearchLineOpen is whether attached Details has a search line taking
// the keys: the timeline's or the report's.
func (m Model) detailsSearchLineOpen() bool {
	return (m.list(m.shownList()).searching && m.timelineSearchLine()) || m.detailsReportSearchOpen()
}

// leaderOverPopup is the key of a command line that stands over an owner that takes only some of the chords:
// Esc closes it, an offered chord runs, and any other chord is swallowed and
// changes nothing. The filter text stays as it was.
func (m Model) leaderOverPopup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if keys.MatchBinding(msg, m.available(ownerMain, mainBindings(m.keys))) == keys.BindingCancelLeader {
		return m.clearLeader(), nil
	}
	if len(m.leader.seq) == 0 {
		for _, tw := range m.keysFacts(m.currentOwner()).Leader.Twins {
			if tw.Letter == msg.String() {
				// The comma form of a Ctrl key runs the entry the key runs.
				return m.clearLeader().Update(pressedKey(tw.Key))
			}
		}
	}
	offered := m.commandsOver(m.currentOwner())
	seq := append(append([]string{}, m.leader.seq...), msg.String())
	outcome, id := m.leaderMap.Resolve(seq)
	if outcome == keys.ResolvePrefix {
		// The first key of a longer chord, none of which is offered here: it is
		// taken with the rest, so no key of it reaches the filter.
		m.leader.seq = seq
		return m, nil
	}
	m = m.clearLeader()
	if outcome != keys.ResolveAction || !slices.Contains(offered, id) {
		return m, nil
	}
	tm, cmd, _ := m.runAction(id)
	return tm, cmd
}

// updateLeaderKey handles the key after a pending leader. Every path here
// consumes the key: a sequence that resolves to nothing must not fall through
// to main mode, or a mistyped `,y` would step the chart a year.
func (m Model) updateLeaderKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if keys.MatchBinding(msg, m.available(ownerMain, mainBindings(m.keys))) == keys.BindingCancelLeader {
		return m.clearLeader(), nil
	}
	seq := append(append([]string{}, m.leader.seq...), msg.String())
	switch outcome, id := m.leaderMap.Resolve(seq); outcome {
	case keys.ResolveAction:
		m = m.clearLeader()
		// The Log's focus stays for the two chords that act on it; any other
		// chord leaves it and runs from the screen it came from.
		if m.logFocus.active && id != keys.BindingCopy && id != keys.BindingLogConsole {
			m.logFocus = logFocusState{}
		}
		// Every action a leader can reach is a BindingID, so the map never
		// holds behaviour of its own: the action runs where the keys' do.
		tm, cmd, _ := m.runAction(id)
		return tm, cmd
	case keys.ResolvePrefix:
		m.leader.seq = seq
		return m, nil
	default:
		return m.clearLeader(), nil
	}
}
