package keys

import (
	"fmt"
	"sort"
	"strings"
)

// LeaderOutcome is what a typed sequence came to. Exactly one of the three
// comes back from Resolve, so no caller has to reason about a half-match.
type LeaderOutcome int

const (
	// ResolveNone: nothing in the map starts this way.
	ResolveNone LeaderOutcome = iota
	// ResolvePrefix: a longer sequence continues from here.
	ResolvePrefix
	// ResolveAction: this sequence is a complete binding.
	ResolveAction
)

// LeaderNode is one level of the prefix tree. The type can represent a map in
// which a sequence is both a binding and the start of a longer one; the
// constructor is what refuses to build such a map.
type LeaderNode struct {
	action    BindingID
	hasAction bool
	next      map[string]*LeaderNode
}

// LeaderBinding is one sequence and the action it runs. The help text is
// deliberately absent: it comes from the key.Binding the id names, so
// defaultKeys stays the single place a binding's description is written.
type LeaderBinding struct {
	Seq []string
	ID  BindingID
	// Twice marks a sequence that acts on its second press within a moment
	// (`, n` with charts open): the key column says so after the keys, where
	// it belongs (operator, 2026-11-07), and the title stays the action.
	Twice bool
}

// Spelling is how Help and the Actions list print the binding's keys: the
// leader and the sequence, then `(twice)` for a binding that asks a second
// press.
func (b LeaderBinding) Spelling(leader string) string {
	s := LeaderSpelling(leader, b.Seq)
	if b.Twice {
		s += " (twice)"
	}
	return s
}

// NewLeaderMap builds the prefix tree, refusing any map in which one sequence
// is both a complete binding and a proper prefix of another. vim resolves
// that case by waiting out timeoutlen and then firing the shorter binding;
// deciding that here would be inventing policy for a map nobody can write
// yet, so the constructor refuses and the rule lands with user-written maps.
func NewLeaderMap(bindings []LeaderBinding) (*LeaderNode, error) {
	root := &LeaderNode{}
	for _, b := range bindings {
		if len(b.Seq) == 0 {
			return nil, fmt.Errorf("leader map: a binding has no keys to type")
		}
		node := root
		for i, k := range b.Seq {
			if node.hasAction {
				return nil, fmt.Errorf("leader map: %q is a binding on its own and the start of %q",
					ShowSeq(b.Seq[:i]), ShowSeq(b.Seq))
			}
			if node.next == nil {
				node.next = make(map[string]*LeaderNode)
			}
			child, ok := node.next[k]
			if !ok {
				child = &LeaderNode{}
				node.next[k] = child
			}
			node = child
		}
		if node.hasAction {
			return nil, fmt.Errorf("leader map: %q is bound twice", ShowSeq(b.Seq))
		}
		if longer := firstSequenceUnder(node, b.Seq); longer != nil {
			return nil, fmt.Errorf("leader map: %q is a binding on its own and the start of %q",
				ShowSeq(b.Seq), ShowSeq(longer))
		}
		node.action = b.ID
		node.hasAction = true
	}
	return root, nil
}

// firstSequenceUnder returns one complete sequence continuing from node, so an
// ambiguity error can name the binding it collides with. Keys are walked in
// sorted order so the error text is the same on every run.
func firstSequenceUnder(node *LeaderNode, prefix []string) []string {
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

// Resolve walks the typed sequence. A nil tree resolves nothing rather than
// panicking, so a Model built without a map is inert instead of fatal.
func (root *LeaderNode) Resolve(seq []string) (LeaderOutcome, BindingID) {
	node := root
	for _, k := range seq {
		if node == nil {
			return ResolveNone, 0
		}
		child, ok := node.next[k]
		if !ok {
			return ResolveNone, 0
		}
		node = child
	}
	if node == nil {
		return ResolveNone, 0
	}
	if node.hasAction {
		return ResolveAction, node.action
	}
	if len(node.next) > 0 {
		return ResolvePrefix, 0
	}
	return ResolveNone, 0
}

// ShowSeq renders a sequence the way the operator typed it.
func ShowSeq(seq []string) string { return strings.Join(seq, " ") }

// ShadowedBinding reports the main-mode binding a leader would take over, as
// "key description", or "" when the leader collides with nothing.
func ShadowedBinding(main []BindingEntry, leader string) string {
	for _, e := range main {
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
