package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/store"
)

// The selected set, its numerals and the comparison it drives
// (the specification). The store owns the set and the numerals
// (Slots.SelectedInOrder, Slots.Numeral); this file derives what the UI does
// with them. What is stored is the tab's memory: the base, the chart the
// cursor's chart is read against (operator 2026-10-24), and the row the cursor
// arrived on last.

// circledNumerals are ①–⑥, indexed by numeral-1: the Unicode numeral glyphs
// the operator asked for (P6).
var circledNumerals = [store.MaxSelectedSlots]string{"①", "②", "③", "④", "⑤", "⑥"}

// numeralForColour is a numeral spelled for the current COLOUR mode: ①–⑥ with
// colours on, `1`–`6` with colours off (C1's tab titles). Out of range spells
// the plain digits, so a stray numeral can never index out of the table.
func numeralForColour(n int, cfg Config) string {
	if n < 1 || n > len(circledNumerals) || !cfg.useColor() {
		return fmt.Sprintf("%d", n)
	}
	return circledNumerals[n-1]
}

// numeralForGlyphs is a numeral spelled for the current GLYPH mode: ①–⑥ in
// Unicode mode, `1`–`6` in ASCII mode — the existing useASCII switch that picks
// ● or `*` (C14's Chart row marker).
func numeralForGlyphs(n int, cfg Config) string {
	if n < 1 || n > len(circledNumerals) || cfg.useASCII() {
		return fmt.Sprintf("%d", n)
	}
	return circledNumerals[n-1]
}

// pairTabTitle is the Aspects tab for a pair, `<right> in <left>`: `③ in ①`,
// or `3 in 1` with colours off (S4 D3). The right chart is the one under the
// cursor, the one you read; the left one is the base and supplies the houses
// and the zodiac.
func pairTabTitle(left, right int, cfg Config) string {
	return numeralForColour(right, cfg) + " in " + numeralForColour(left, cfg)
}

func (m Model) pairTabTitle(left, right int) string { return pairTabTitle(left, right, m.cfg) }

// aspectsPairTabID is the identity of the left-vs-right tab, derived from both
// numerals. The first declared pair tab is named TabAspectsCross instead
// (see pairTabID), so readers of the old cross tab still find it.
func aspectsPairTabID(left, right int) TabID {
	return TabID(fmt.Sprintf("aspects-pair-%d-%d", left, right))
}

// pairTabID is the id of the i-th declared pair tab of the scene.
func (sc scene) pairTabID(i int) TabID {
	if i == 0 {
		return TabAspectsCross
	}
	p := sc.pairs[i]
	return aspectsPairTabID(sc.numeral(p.left), sc.numeral(p.right))
}

// pairTitle is the tab title of the i-th declared pair.
func (sc scene) pairTitle(i int, cfg Config) string {
	p := sc.pairs[i]
	return pairTabTitle(sc.numeral(p.left), sc.numeral(p.right), cfg)
}

// pairIndex is the index of the active pair in pairs, -1 for the `Aspects` tab.
func (sc scene) pairIndex() int {
	for i := range sc.pairs {
		if sc.pair == &sc.pairs[i] {
			return i
		}
	}
	return -1
}

// isAspectsPairTab reports whether id names one of the comparing pair tabs.
func isAspectsPairTab(id TabID) bool {
	return id == TabAspectsCross || strings.HasPrefix(string(id), "aspects-pair-")
}

// comparing reports whether the cursor is on a slot of the selected set AND the
// set holds at least two slots, with the baseline anchored somewhere real.
// Otherwise the screens are "single": the cursor's own chart, everywhere (P2).
func (m Model) comparing() bool {
	return m.deriveScene().mode == sceneComparing
}

// pairSlots are the declared Aspects pairs in tab order: one tab each. Nil
// unless comparing.
func (m Model) pairSlots() []scenePair {
	return m.deriveScene().pairs
}

// activePairSlot is the active pair. Nil is the `Aspects` tab (the cursor's own
// chart): nothing stored, or not comparing (C1a, operator 2026-10-04).
func (m Model) activePairSlot() *scenePair {
	return m.deriveScene().pair
}

// activePair is the pair the Aspects panel shows for a comparing scene, read off
// the stored base (operator 2026-10-24: "bold is always the cursor, EVERYWHERE,
// faint is always the base, aligning with aspects tab"):
//
//	base nil                   the cursor's own tab, no pair
//	base != cursor             (base, cursor): the cursor's chart read in the base
//	base == cursor             the own tab (an arrival swaps first, noteArrival)
//
// A base that is no longer in the set, or a moment (never a base), shows no pair.
func (m Model) activePair(sc scene) *scenePair {
	b := m.base
	if b == nil || b == sc.cursor || b.Floating || sc.numeral(b) == 0 {
		return nil
	}
	want := scenePair{left: b, right: sc.cursor}
	for i := range sc.pairs {
		if sc.pairs[i] == want {
			return &sc.pairs[i]
		}
	}
	return nil
}

// withBase stores the base (nil is the cursor's own tab). It is written by a
// deliberate act (`[` / `]`, Enter on a timeline row, a deselect or a removal)
// and by one arrival: landing on the base swaps it (noteArrival).
func (m Model) withBase(s *store.Slot) Model {
	m.base = s
	return m
}

// noteArrival is what a cursor arrival does to the tab's memory: landing ON the
// base makes the chart you came from the base, so "① in ②" on Anna becomes
// "② in ①" on Ben. A moment cannot be a base, so arriving from the floating
// slot (or from a chart outside the set) keeps the base and the row shows its
// own tab. Any other arrival keeps the base. at records the row arrived on.
func (m Model) noteArrival() Model {
	if m.slots == nil {
		return m
	}
	c, from := m.focusedSlot(), m.at
	m.at = c
	if c == nil || from == nil || from == c || m.base != c {
		return m
	}
	if from.Floating || numeralIn(selectionOf(m.slots), from) == 0 {
		return m
	}
	return m.withBase(from)
}

// withTabMemoryOf puts back the tab memory another Model held: a walk that
// passes rows on its way restores it, so only its destination is an arrival.
func (m Model) withTabMemoryOf(o Model) Model {
	m.at, m.base = o.at, o.base
	return m
}

// pruneTabMemory drops a base that left the selected set: it was deselected or
// removed, and the tab is the own tab again.
func (m Model) pruneTabMemory() Model {
	var set []*store.Slot
	if m.slots != nil {
		set = selectionOf(m.slots)
	}
	if m.base != nil && numeralIn(set, m.base) == 0 {
		m.base = nil
	}
	return m
}

// remapTabMemory carries the base and the row arrived on over to rebuilt
// slots: each old slot is replaced by its counterpart, and one with none is
// dropped (a base with none becomes the chart's own tab).
func (m Model) remapTabMemory(counterpart map[*store.Slot]*store.Slot) Model {
	m.base = counterpart[m.base]
	m.at = counterpart[m.at]
	return m
}

// forgetTabMemory starts the tab on the chart alone: a restored session, or a
// rebuilt workspace whose slots are new objects.
func (m Model) forgetTabMemory() Model {
	m.base, m.at = nil, nil
	return m
}

// comparisonSig is everything about the selection that decides WHAT the text
// frame computes besides time: whether the cursor's screens compare, against
// which frame, and which pair. Two equal signatures need no recompute.
type comparisonSig struct {
	comparing bool
	cursor    *store.Slot
	frame     *store.Slot
	pair      scenePair
	// shown is the shown set in numeral order: a change of any member's
	// selection, not only the cursor's, changes what the text frame computes
	// (the specification C2).
	shown [store.MaxSelectedSlots]*store.Slot
}

func (m Model) comparisonSig() comparisonSig {
	sc := m.deriveScene()
	if sc.mode != sceneComparing {
		return comparisonSig{}
	}
	sig := comparisonSig{comparing: true, cursor: sc.cursor, frame: sc.frame}
	if sc.pair != nil {
		sig.pair = *sc.pair
	}
	copy(sig.shown[:], sc.shown)
	return sig
}

// afterPairChange recomputes after the active pair moved without the slot
// cursor moving (`[` / `]`). The previous pair's cross list must not sit under
// the new pair's tab while the compute is in flight, so it is blanked first;
// Details waits for the new frame like any other change of what is computed.
func (m Model) afterPairChange() (Model, tea.Cmd) {
	m = m.holdShown()
	m.chart.crossAspects = nil
	m.chart.crossAspectStyles = nil
	m.chart.crossAspectsReport = ""
	m.chart.crossSynastry = false
	m.chart.settled = false
	m = m.waitDetailsForText()
	return m.bumpIdentity(surfaceText)
}
