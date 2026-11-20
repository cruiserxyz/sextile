package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/popup"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// zodiac_finder_model.go is the Model's half of the zodiac finder (⌃z, or the
// leader sequence `, z`), the house finder's sibling: Tropical and Astrolog's
// ten named ayanamsas in the shared picker chassis (the popup is
// internal/ui/popup/finder.go, one type for both). Enter casts the focused
// chart in the zodiac under the cursor (the specification C8). The operator asked
// for it to be a popup of its own, not a second group in the house finder
// (2026-10-08).
//
// Like the house finder it adds no write path: Enter hands the choice to
// chooseZodiac, which records, saves the session once and recasts. Moving the
// cursor, typing and Esc touch nothing outside this state.

// zodiacFinderState is the finder's state. The list machinery is the chassis's.
type zodiacFinderState = popup.Finder[recipe.Zodiac]

// newZodiacFinder is a closed zodiac finder for the chart called name, whose
// own zodiac is current; refilter it before use.
func newZodiacFinder(name string, current recipe.Zodiac, slotIdx int) zodiacFinderState {
	return zodiacFinderState{Kind: popup.ZodiacKind(), Name: name, Current: current, SlotIdx: slotIdx}
}

// zodiacFinderFor is the finder for slot: a chart whose file sets an offset no
// named ayanamsa has, and whose zodiac the operator has not chosen, gets that
// offset as its first row, marked as the current one.
func (m Model) zodiacFinderFor(slot *store.Slot) zodiacFinderState {
	st := newZodiacFinder(houseFinderChartName(slot.Chart.Name, slot.Floating), m.ownZodiac(slot), m.slotIndexOf(slot))
	c := slot.Chart.Clock
	if slot.Floating || !c.ZodiacSet || m.zodiacChosen(slot) {
		return st
	}
	if _, named := lookup.ZodiacOfOffset(c.Sidereal, c.Offset); named {
		return st
	}
	tag := lookup.ZodiacTag(c.Sidereal, c.Offset)
	st.Kind = popup.ZodiacKindWithFile("Sidereal "+tag+", from its file", tag)
	st.Current = popup.FileZodiac
	return st
}

// openZodiacFinder is the ⌃z / `, z` handler: it opens the finder for the
// focused chart, from the main screen only and once the first frame is in.
func openZodiacFinder(m Model) (tea.Model, tea.Cmd) {
	if m.blocked(keys.BindingZodiac) != "" {
		return m, nil
	}
	slot := m.focusedSlot()
	st := m.zodiacFinderFor(slot)
	st.Open(dialog.ListHeight(m.height))
	m.zodiacFinder = st
	m.startup = startupZodiacOpen
	return m, nil
}

// zodiacBindings is the zodiac finder's table: the house finder's, since it is
// the same kind of popup. Ctrl+H is not in it either: inside the search line it
// is a backspace.
func zodiacBindings() []keys.BindingEntry { return finderBindings() }

// updateZodiacFinderKey handles one keystroke while the finder has the keyboard:
// the key resolves through the bindings table to the popup's Action.
func updateZodiacFinderKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.zodiacFinder
	id := keys.MatchBinding(msg, zodiacBindings())
	out := f.Apply(finderAction(id), msg, dialog.ListHeight(m.height))
	switch {
	case out.Dismissed:
		m.startup = startupReady
		return m, nil
	case out.Confirmed:
		m.startup = startupReady
		return m.chooseZodiac(f.SlotIdx, out.Chosen)
	}
	m.zodiacFinder = f
	return m, nil
}

// renderZodiacFinder draws the finder.
func renderZodiacFinder(f zodiacFinderState, cfg Config, termW, termH int) string {
	return f.Render(cfg, termW, termH, popupChrome(cfg), func(cursor, total int) *dialog.Counter {
		pos := listPosition{Cursor: cursor, Total: total}
		return popupCounter(&pos, cfg)
	})
}
