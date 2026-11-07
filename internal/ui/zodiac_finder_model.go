package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/popup"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// zodiac_finder_model.go is the Model's half of the zodiac finder (⌃z, or the
// leader sequence `, z`), the house finder's sibling: Tropical and Astrolog's
// ten named ayanamsas in the shared picker chassis (the popup is
// internal/ui/popup/finder.go, one type for both), and Enter casts the focused
// chart in the one under the cursor (the specification C8). The operator asked
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

// openZodiacFinder is the ⌃z / `, z` handler: it opens the finder for the
// focused chart, from the main screen only and once the first frame is in.
func openZodiacFinder(m Model) (tea.Model, tea.Cmd) {
	if m.blocked(keys.BindingZodiac) != "" {
		return m, nil
	}
	slot := m.focusedSlot()
	st := newZodiacFinder(houseFinderChartName(slot.Chart.Name, slot.Floating), m.ownZodiac(slot), m.slotIndexOf(slot))
	st.Open(dialog.ListHeight(m.height))
	m.zodiacFinder = st
	m.startup = startupZodiacOpen
	return m, nil
}

// zodiacBindings is the zodiac finder's table: the house finder's, since it is
// the same kind of popup. Ctrl+H is not in it either: inside the search line it
// is a backspace.
func zodiacBindings() []keys.BindingEntry { return housesBindings() }

// updateZodiacFinderKey handles one keystroke while the finder has the keyboard:
// the key resolves through the bindings table to the popup's Action.
func updateZodiacFinderKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.zodiacFinder
	a := popup.Type
	switch keys.MatchBinding(msg, zodiacBindings()) {
	case keys.BindingClose:
		a = popup.Dismiss
	case keys.BindingConfirm:
		a = popup.Confirm
	case keys.BindingListUp:
		a = popup.Up
	case keys.BindingListDown:
		a = popup.Down
	case keys.BindingPageUp:
		a = popup.PageUp
	case keys.BindingPageDown:
		a = popup.PageDown
	case keys.BindingPageHome:
		a = popup.PageHome
	case keys.BindingPageEnd:
		a = popup.PageEnd
	}
	out := f.Apply(a, msg, dialog.ListHeight(m.height))
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
