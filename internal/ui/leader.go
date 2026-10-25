package ui

import (
	"fmt"
	"sort"
	"strings"
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

// leaderOutcome is what a typed sequence came to. Exactly one of the three
// comes back from resolve, so no caller has to reason about a half-match.
type leaderOutcome int

const (
	// resolveNone: nothing in the map starts this way.
	resolveNone leaderOutcome = iota
	// resolvePrefix: a longer sequence continues from here.
	resolvePrefix
	// resolveAction: this sequence is a complete binding.
	resolveAction
)

// leaderNode is one level of the prefix tree. The type can represent a map in
// which a sequence is both a binding and the start of a longer one; the
// constructor is what refuses to build such a map.
type leaderNode struct {
	action    BindingID
	hasAction bool
	next      map[string]*leaderNode
}

// leaderBinding is one sequence and the action it runs. The help text is
// deliberately absent: it comes from the key.Binding the id names, so
// defaultKeys stays the single place a binding's description is written.
type leaderBinding struct {
	Seq []string
	ID  BindingID
}

// defaultLeaderMap is the map that ships: exactly one sequence, `l`, which
// turns the Log into a console. No user-writable map exists yet.
func defaultLeaderMap() []leaderBinding {
	return []leaderBinding{
		{Seq: []string{"l"}, ID: BindingLogConsole},
		{Seq: []string{"s"}, ID: BindingSaveChart},
		{Seq: []string{"h"}, ID: BindingHouses},
		{Seq: []string{"z"}, ID: BindingZodiac},
		{Seq: []string{"a"}, ID: BindingHouseDisplay},
		{Seq: []string{"c", "d"}, ID: BindingSaveCity},
	}
}

// detailsLeaderMap is the sequences available while Details is attached: the
// main screen's, plus `y`, which copies. The main screen's map does not have
// `y`, so `,y` there stays a mistyped sequence.
func detailsLeaderMap() []leaderBinding {
	return append(defaultLeaderMap(), leaderBinding{Seq: []string{"y"}, ID: BindingCopy})
}

var detailsLeaderTree = func() *leaderNode {
	root, err := newLeaderMap(detailsLeaderMap())
	if err != nil {
		return &leaderNode{}
	}
	return root
}()

// mustDefaultLeaderMap builds the shipped map. An ambiguous compiled-in map
// is a build-time mistake, not a runtime condition, so this falls back to an
// empty tree rather than taking the process down — TestDefaultLeaderMapBuilds
// is what catches it.
func mustDefaultLeaderMap() *leaderNode {
	root, err := newLeaderMap(defaultLeaderMap())
	if err != nil {
		return &leaderNode{}
	}
	return root
}

// newLeaderMap builds the prefix tree, refusing any map in which one sequence
// is both a complete binding and a proper prefix of another. vim resolves
// that case by waiting out timeoutlen and then firing the shorter binding;
// deciding that here would be inventing policy for a map nobody can write
// yet, so the constructor refuses and the rule lands with user-written maps.
func newLeaderMap(bindings []leaderBinding) (*leaderNode, error) {
	root := &leaderNode{}
	for _, b := range bindings {
		if len(b.Seq) == 0 {
			return nil, fmt.Errorf("leader map: a binding has no keys to type")
		}
		node := root
		for i, k := range b.Seq {
			if node.hasAction {
				return nil, fmt.Errorf("leader map: %q is a binding on its own and the start of %q",
					showSeq(b.Seq[:i]), showSeq(b.Seq))
			}
			if node.next == nil {
				node.next = make(map[string]*leaderNode)
			}
			child, ok := node.next[k]
			if !ok {
				child = &leaderNode{}
				node.next[k] = child
			}
			node = child
		}
		if node.hasAction {
			return nil, fmt.Errorf("leader map: %q is bound twice", showSeq(b.Seq))
		}
		if longer := firstSequenceUnder(node, b.Seq); longer != nil {
			return nil, fmt.Errorf("leader map: %q is a binding on its own and the start of %q",
				showSeq(b.Seq), showSeq(longer))
		}
		node.action = b.ID
		node.hasAction = true
	}
	return root, nil
}

// firstSequenceUnder returns one complete sequence continuing from node, so an
// ambiguity error can name the binding it collides with. Keys are walked in
// sorted order so the error text is the same on every run.
func firstSequenceUnder(node *leaderNode, prefix []string) []string {
	if len(node.next) == 0 {
		return nil
	}
	keys := make([]string, 0, len(node.next))
	for k := range node.next {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		child := node.next[k]
		seq := append(append([]string{}, prefix...), k)
		if child.hasAction {
			return seq
		}
		if deeper := firstSequenceUnder(child, seq); deeper != nil {
			return deeper
		}
	}
	return nil
}

// resolve walks the typed sequence. A nil tree resolves nothing rather than
// panicking, so a Model built without a map is inert instead of fatal.
func (root *leaderNode) resolve(seq []string) (leaderOutcome, BindingID) {
	node := root
	for _, k := range seq {
		if node == nil {
			return resolveNone, 0
		}
		child, ok := node.next[k]
		if !ok {
			return resolveNone, 0
		}
		node = child
	}
	if node == nil {
		return resolveNone, 0
	}
	if node.hasAction {
		return resolveAction, node.action
	}
	if len(node.next) > 0 {
		return resolvePrefix, 0
	}
	return resolveNone, 0
}

// showSeq renders a sequence the way the operator typed it.
func showSeq(seq []string) string { return strings.Join(seq, " ") }

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

// shadowedBinding reports the main-mode binding a leader would take over, as
// "key description", or "" when the leader collides with nothing.
func shadowedBinding(keys keyMap, leader string) string {
	for _, e := range keys.mainBindings() {
		for _, k := range e.Binding.Keys() {
			if k != leader {
				continue
			}
			h := e.Binding.Help()
			return h.Key + " " + h.Desc
		}
	}
	return ""
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
		return m.logNote("preferences", "keys.leader %q ignored: %s are the only ways out; leader stays %q",
			cfg.Leader, showSeq(strandingKeys()), m.leaderKey)
	case utf8.RuneCountInString(cfg.Leader) != 1:
		return m.logNote("preferences", "keys.leader %q ignored: a leader is one key; leader stays %q",
			cfg.Leader, m.leaderKey)
	}
	if shadowed := shadowedBinding(m.keys, cfg.Leader); shadowed != "" {
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
	if matchBinding(msg, m.available(m.keys.mainBindings())) == BindingCancelLeader {
		return m.clearLeader(), nil
	}
	seq := append(append([]string{}, m.leader.seq...), msg.String())
	tree := m.leaderMap
	if m.currentOwner() == ownerDetails {
		tree = detailsLeaderTree
	}
	switch outcome, id := tree.resolve(seq); outcome {
	case resolveAction:
		return m.clearLeader().runLeaderAction(id)
	case resolvePrefix:
		m.leader.gen++
		m.leader.seq = seq
		return m, armLeaderTimeout(m.leader.gen, m.leaderTimeout)
	default:
		return m.clearLeader(), nil
	}
}

// runLeaderAction dispatches a completed sequence. Every action a leader can
// reach is a BindingID, so the map never holds behaviour of its own.
func (m Model) runLeaderAction(id BindingID) (tea.Model, tea.Cmd) {
	switch id {
	case BindingLogConsole:
		return m.openConsole()
	case BindingSaveChart:
		return m.openSavePrompt()
	case BindingHouses:
		return openHouseFinder(m)
	case BindingZodiac:
		return openZodiacFinder(m)
	case BindingCopy:
		return m.yankDetails()
	case BindingSaveCity:
		return m.saveDefaultCity()
	case BindingHouseDisplay:
		return openHouseDisplay(m)
	}
	return m, nil
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
