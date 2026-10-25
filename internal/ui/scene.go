package ui

import (
	"time"

	"github.com/cruiserxyz/sextile/internal/store"
)

// sceneMode says whether the screens show the cursor's chart alone or the
// selected set side by side.
type sceneMode int

const (
	sceneAlone     sceneMode = iota // the cursor's own chart, everywhere
	sceneComparing                  // the selected set, everywhere
)

// scene is the ONE answer to "what do the panels show?", derived from the
// cursor and the selected set (the specification). Operator, 2026-10-05:
// "as soon as the focused one receives selection, pairs show".
//
// It is derived on every read from the store and holds slot pointers only for
// the instant of that read; nothing caches one across a change.
type scene struct {
	mode     sceneMode
	cursor   *store.Slot   // the slot under the cursor; nil before a workspace exists
	cursorN  int           // its numeral in the selected set, 0 when unselected
	set      []*store.Slot // the selected set in numeral order
	shown    []*store.Slot // alone: [cursor]; comparing: set
	baseline *store.Slot   // first selected anchored slot (①), nil with none
	pairs    []scenePair   // comparing: the declared Aspects pairs, left then right; else nil
	pair     *scenePair    // the active pair (an element of pairs); nil is the cursor's own tab
	frame    *store.Slot   // the slot whose cusps the screen reads: the active pair's left chart (the base), else the cursor, while comparing; nil alone
	focus    int           // the cursor's index in shown
	// entered is the timeline row Enter last pressed, while it holds (rule 8 of
	// the specification); nil otherwise.
	entered *enteredRow
}

// enteredRow is the pressed row as the scene knows it: the chart whose row it
// is (nil for a sky row) and its instant.
type enteredRow struct {
	chart   *store.Slot
	numeral int
	instant time.Time
}

// scenePair is one Aspects pair: the left chart and the right chart of its
// cross rows (the specification rule 9).
type scenePair struct{ left, right *store.Slot }

// pairsOf is the declared pairs of a comparing set: the cursor's chart is the
// right side of one pair per other ANCHORED member, in numeral order, each
// pair (k, cursor), `<cursor> in k`: the chart under the cursor read against
// the base k (operator 2026-10-24: "the cursor on charts denominates which
// column in bodies is highlighted ... anna dictates house base"). A moment is
// never a base, so the floating slot is never a pair's left side: on an
// anchored row it declares no pair, and on the floating slot's own row each
// anchored member is a base, as before (2026-10-13: "floating must always
// stand to the right side").
func pairsOf(set []*store.Slot, cursor *store.Slot) []scenePair {
	var pairs []scenePair
	for _, s := range set {
		if s == cursor || s.Floating {
			continue
		}
		pairs = append(pairs, scenePair{left: s, right: cursor})
	}
	return pairs
}

// deriveScene is the only place the rule lives. Comparing when the cursor's
// slot is selected, at least two slots are selected and ① has a moment.
func (m Model) deriveScene() scene {
	var sc scene
	if m.slots == nil {
		return sc
	}
	sc.cursor = m.focusedSlot()
	sc.set = selectionOf(m.slots)
	sc.baseline = m.slots.Baseline()
	sc.cursorN = sc.numeral(sc.cursor)
	sc.entered = m.enteredRowOf(sc)
	if len(sc.set) >= 2 && sc.cursorN != 0 && !anchorOf(sc.baseline).IsZero() {
		sc.mode = sceneComparing
		sc.shown = sc.set
		sc.pairs = pairsOf(sc.set, sc.cursor)
		sc.focus = sc.cursorN - 1
		// The base frames: the active pair's left chart, whose houses, house
		// system and zodiac the comparison is read in; on the own tab the
		// cursor's chart frames itself. The chart under the cursor is always
		// the one Bodies lists (operator 2026-10-24).
		sc.pair = m.activePair(sc)
		sc.frame = sc.cursor
		if sc.pair != nil {
			sc.frame = sc.pair.left
		}
		return sc
	}
	if sc.cursor != nil {
		sc.shown = []*store.Slot{sc.cursor}
	}
	return sc
}

// enteredRowOf is rule 8: the pressed row, while the floating moment is its
// instant and its chart, if it has one, is still in the set. The moment is the
// target the press set, so the scene is right from the one compute the press
// starts, before the slot's Time commits.
func (m Model) enteredRowOf(sc scene) *enteredRow {
	if !m.details.pressedSet || !m.targetFloatingTime.Equal(m.details.pressedAt.landed()) {
		return nil
	}
	e := &enteredRow{numeral: m.details.pressedAt.numeral, instant: m.details.pressedAt.instant}
	if e.numeral == 0 {
		return e
	}
	if m.details.pressedChart == nil || sc.numeral(m.details.pressedChart) == 0 {
		return nil
	}
	e.chart = m.details.pressedChart
	e.numeral = sc.numeral(e.chart)
	return e
}

// selectionOf is the selected slots in numeral order. It is the one door to
// the store's selection for code that holds a *store.Slots but no Model (the
// Chart panel's row renderer).
func selectionOf(slots *store.Slots) []*store.Slot { return slots.SelectedInOrder() }

// numeralIn is a slot's 1-based numeral in set, 0 when it is not a member.
func numeralIn(set []*store.Slot, s *store.Slot) int {
	for i, member := range set {
		if member == s {
			return i + 1
		}
	}
	return 0
}

// numeral is a slot's numeral in the selected set, 0 when unselected.
func (sc scene) numeral(s *store.Slot) int { return numeralIn(sc.set, s) }
