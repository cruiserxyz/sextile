package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// house_finder_model.go is the Model's half of the house finder (⌃h, or the
// leader sequence `, h`; the popup itself, shared with the zodiac finder, is
// internal/ui/popup/finder.go): Astrolog's 22 house systems in the shared
// picker chassis, and Enter casts the focused chart in the one under the cursor
// (the specification).
//
// It adds no write path. Enter hands the choice to chooseHouseSystem, the one
// door a house choice comes through (the specification), which resolves,
// records, saves the session once and recasts. Moving the cursor, typing and
// Esc touch nothing outside this state, and there is no live preview: the
// finder is a chooser, not a try-on.

// houseFinderState is the finder's state. The list machinery is the chassis's.
type houseFinderState = popup.Finder[recipe.HouseSystem]

// newHouseFinder is a closed house finder for the chart called name, whose own
// system is current; refilter it before use.
func newHouseFinder(name string, current recipe.HouseSystem, slotIdx int) houseFinderState {
	return houseFinderState{Kind: popup.HouseKind(), Name: name, Current: current, SlotIdx: slotIdx}
}

// houseFinderChartName is how the title names a chart.
func houseFinderChartName(name string, floating bool) string {
	if floating || strings.TrimSpace(name) == "" {
		return "Now"
	}
	return name
}

// openHouseFinder is the ⌃h / `, h` handler: it opens the finder for the
// focused chart, from the main screen only and once the first frame is in.
// Enter changes the focused chart's own system and no other chart's, in a
// group or not (operator, 2026-10-24: "only the focused chart should have
// it"); the title names that chart.
func openHouseFinder(m Model) (tea.Model, tea.Cmd) {
	if m.blocked(keys.BindingHouses) != "" {
		return m, nil
	}
	slot := m.focusedSlot()
	st := newHouseFinder(houseFinderChartName(slot.Chart.Name, slot.Floating), m.ownHouses(slot).hs, m.slotIndexOf(slot))
	st.Open(dialog.ListHeight(m.height))
	m.houseFinder = st
	m.startup = startupHouseOpen
	return m, nil
}

// housesBindings is the house finder's table. Ctrl+H is not in it: inside the
// search line it is a backspace.
func housesBindings() []keys.BindingEntry {
	return keys.Titled(append([]keys.BindingEntry{listUp().ShownWhen(func(c keys.Context) bool { return c.Popup.Rows > 1 }), listDown(), confirm("Choose").ShownWhen(func(c keys.Context) bool { return c.Popup.Rows > 0 }), closeOn("Cancel")}, pageEntries()...))
}

// updateHouseFinderKey handles one keystroke while the finder has the keyboard:
// the key resolves through the bindings table to the popup's Action.
func updateHouseFinderKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.houseFinder
	a := popup.Type
	switch keys.MatchBinding(msg, housesBindings()) {
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
		return m.chooseHouseSystem(f.SlotIdx, out.Chosen)
	}
	m.houseFinder = f
	return m, nil
}

// renderHouseFinder draws the finder.
func renderHouseFinder(f houseFinderState, cfg Config, termW, termH int) string {
	return f.Render(cfg, termW, termH, popupChrome(cfg), func(cursor, total int) *dialog.Counter {
		pos := listPosition{Cursor: cursor, Total: total}
		return popupCounter(&pos, cfg)
	})
}
