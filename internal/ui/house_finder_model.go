package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// house_finder_model.go is the Model's half of the house finder (the
// leader sequence `, u`; the popup itself, shared with the zodiac finder, is
// internal/ui/popup/finder.go): Astrolog's 22 house systems in the shared
// picker chassis. Enter on a system casts the focused chart in it; Enter on a
// form of the Shown as part puts that form in force and leaves the chart's
// system as it is.
//
// It adds no write path of its own. Enter hands the system to chooseHouseSystem,
// the one door a house choice comes through (the specification), which
// resolves, records, saves the session once and recasts. It keeps nothing as a
// default: `, c d` does that. Moving the cursor, typing and Esc touch nothing
// outside this state: the finder is a chooser, not a try-on.

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

// openHouseFinder is the `, u` handler: it opens the finder for the
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
	st.Shown = popup.NewShownAs(displayForms(), m.houseDisplay)
	st.Open(dialog.ListHeight(m.height))
	m.houseFinder = st
	m.startup = startupHouseOpen
	return m, nil
}

// finderBindings is the list keys the house and zodiac finders share.
func finderBindings() []keys.BindingEntry {
	return keys.TitledLocal(append([]keys.BindingEntry{listUp().ShownWhen(func(c keys.Context) bool { return c.Popup.Rows > 1 }), listDown(), confirm("Choose").ShownWhen(func(c keys.Context) bool { return c.Popup.Rows > 0 }), closeOn("Cancel")}, pageEntries()...))
}

// housesBindings is the house finder's table. Ctrl+H is not in it: inside the
// search line it is a backspace.
func housesBindings() []keys.BindingEntry { return finderBindings() }

// finderAction is the popup Action a key of the finders' table stands for.
func finderAction(id keys.BindingID) popup.Action {
	switch id {
	case keys.BindingClose:
		return popup.Dismiss
	case keys.BindingConfirm:
		return popup.Confirm
	case keys.BindingListUp:
		return popup.Up
	case keys.BindingListDown:
		return popup.Down
	case keys.BindingPageUp:
		return popup.PageUp
	case keys.BindingPageDown:
		return popup.PageDown
	case keys.BindingPageHome:
		return popup.PageHome
	case keys.BindingPageEnd:
		return popup.PageEnd
	}
	return popup.Type
}

// updateHouseFinderKey handles one keystroke while the finder has the keyboard:
// the key resolves through the bindings table to the popup's Action.
func updateHouseFinderKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := m.houseFinder
	id := keys.MatchBinding(msg, housesBindings())
	out := f.Apply(finderAction(id), msg, dialog.ListHeight(m.height))
	switch {
	case out.Dismissed:
		m.startup = startupReady
		return m, nil
	case out.Confirmed:
		m.startup = startupReady
		return m.applyHouseChoice(f, out)
	}
	m.houseFinder = f
	return m, nil
}

// applyHouseChoice is Enter in the house finder. A system goes through the one
// door (the chart's own system included: Enter on it records a choice). A form
// of the Shown as part comes into force for every slot and the chart's system is
// left as it was.
func (m Model) applyHouseChoice(f houseFinderState, out popup.FinderOutcome[recipe.HouseSystem]) (tea.Model, tea.Cmd) {
	if out.OnShown {
		m.houseDisplay = out.Form
		return m, nil
	}
	return m.chooseHouseSystem(f.SlotIdx, out.Chosen)
}

// renderHouseFinder draws the finder.
func renderHouseFinder(f houseFinderState, cfg Config, termW, termH int) string {
	return f.Render(cfg, termW, termH, popupChrome(cfg), func(cursor, total int) *dialog.Counter {
		pos := listPosition{Cursor: cursor, Total: total}
		return popupCounter(&pos, cfg)
	})
}
