package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"time"
	"unicode/utf8"

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
const (
	// defaultLeaderKey is the leader when preferences.yaml says nothing.
	defaultLeaderKey = ","
	// defaultLeaderTimeout is how long a pending leader waits for the next
	// key before giving up in silence.
	defaultLeaderTimeout = 1000 * time.Millisecond
)

// defaultLeaderMap is the map that ships. No user-writable map exists yet.
// Attached Details uses it too: `y` copies the whole content of whatever has
// the keyboard, so it is a main-map chord and no longer Details' alone.
func defaultLeaderMap() []keys.LeaderBinding {
	return []keys.LeaderBinding{
		{Seq: []string{"l"}, ID: keys.BindingLogConsole},
		{Seq: []string{"s"}, ID: keys.BindingSaveChart},
		{Seq: []string{"h"}, ID: keys.BindingHouses},
		{Seq: []string{"z"}, ID: keys.BindingZodiac},
		{Seq: []string{"a"}, ID: keys.BindingHouseDisplay},
		{Seq: []string{"o"}, ID: keys.BindingSessions},
		{Seq: []string{"n"}, ID: keys.BindingNewSession, Twice: true},
		{Seq: []string{"c", "d"}, ID: keys.BindingSaveCity},
		{Seq: []string{"y"}, ID: keys.BindingCopy},
	}
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

// leaderState is the pending prefix: whether one is armed, what has been
// typed since, and which generation armed it. The generation is what lets a
// timeout tick tell "the leader I armed" from "a leader armed since".
type leaderState struct {
	pending bool
	seq     []string
	gen     uint64
}

// leaderTimeoutMsg fires when a pending leader has waited out its window. It
// carries the generation that armed it, exactly as the refresh coordinator's
// ticks carry a timer id, so a tick from an older pending state is dropped
// instead of cancelling a newer one.
type leaderTimeoutMsg struct{ gen uint64 }

func armLeaderTimeout(gen uint64, d time.Duration) tea.Cmd {
	return longTimer(d, func(time.Time) tea.Msg { return leaderTimeoutMsg{gen: gen} })
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
// Silence is the contract for the good cases: a valid leader, a valid
// timeoutlen, and either one simply absent all pass without a word. Only a
// refusal or a shadowed binding is worth the operator's attention.
func (m Model) applyKeyConfig(cfg store.KeysConfig) Model {
	for _, r := range cfg.Rejections {
		m = m.logNote("preferences", "%s %q ignored: %s", r.Field, r.Value, r.Reason)
	}
	if cfg.TimeoutlenPresent {
		m.leaderTimeout = time.Duration(cfg.TimeoutlenMS) * time.Millisecond
	}
	if !cfg.LeaderPresent {
		return m
	}
	switch {
	case isStrandingKey(cfg.Leader):
		return m.logNote("preferences", "keys.leader %q ignored: %s are the only ways out — leader stays %q",
			cfg.Leader, keys.ShowSeq(strandingKeys()), m.leaderKey)
	case utf8.RuneCountInString(cfg.Leader) != 1:
		return m.logNote("preferences", "keys.leader %q ignored: a leader is one key — leader stays %q",
			cfg.Leader, m.leaderKey)
	}
	if shadowed := keys.ShadowedBinding(mainBindings(m.keys), cfg.Leader); shadowed != "" {
		m = m.logNote("preferences", "leader %q now shadows %s", cfg.Leader, shadowed)
	}
	m.leaderKey = cfg.Leader
	return m
}

// armLeader enters the pending state with nothing typed yet and starts the
// window. The keystroke is consumed: a leader press is never also a main-mode
// key, which is what makes a configured leader able to shadow one.
func (m Model) armLeader() (Model, tea.Cmd) {
	// The operator has moved on from whatever was copied; the footer slot is
	// the pending leader's now. The flash tick still in flight finds the
	// confirmation already cleared and does nothing.
	m.yank.active = false
	m.leader.gen++
	m.leader.pending = true
	m.leader.seq = nil
	return m, armLeaderTimeout(m.leader.gen, m.leaderTimeout)
}

// clearLeader drops the pending prefix. The generation is left where it is,
// so the tick already in flight for it arrives stale and is dropped.
func (m Model) clearLeader() Model {
	m.leader.pending = false
	m.leader.seq = nil
	return m
}

// updateLeaderKey handles the key after a pending leader. Every path here
// consumes the key: a sequence that resolves to nothing must not fall through
// to main mode, or a mistyped `,y` would step the chart a year.
func (m Model) updateLeaderKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.updateLeaderKeyAmong(msg, nil)
}

// updateLeaderKeyAmong is updateLeaderKey for an owner that takes only some of
// the leader's chords: allowed, when set, names the ids that may run, and a
// sequence that completes any other is swallowed like a mistyped one.
func (m Model) updateLeaderKeyAmong(msg tea.KeyMsg, allowed func(keys.BindingID) bool) (tea.Model, tea.Cmd) {
	if keys.MatchBinding(msg, m.available(ownerMain, mainBindings(m.keys))) == keys.BindingCancelLeader {
		return m.clearLeader(), nil
	}
	seq := append(append([]string{}, m.leader.seq...), msg.String())
	switch outcome, id := m.leaderMap.Resolve(seq); outcome {
	case keys.ResolveAction:
		m = m.clearLeader()
		if allowed != nil && !allowed(id) {
			return m, nil
		}
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
		m.leader.gen++
		m.leader.seq = seq
		return m, armLeaderTimeout(m.leader.gen, m.leaderTimeout)
	default:
		return m.clearLeader(), nil
	}
}

// onLeaderTimeout retires a pending leader that waited too long: no action, no
// Log line, nothing to see. A tick carrying any generation but the current one
// belongs to a pending state that is already gone.
func (m Model) onLeaderTimeout(msg leaderTimeoutMsg) (tea.Model, tea.Cmd) {
	if !m.leader.pending || msg.gen != m.leader.gen {
		return m, nil
	}
	return m.clearLeader(), nil
}
