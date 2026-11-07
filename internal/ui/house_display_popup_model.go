package ui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/internal/ui/popup"

	tea "github.com/charmbracelet/bubbletea"
)

// house_display_popup_model.go is the Model's half of the House display popup
// (`, a`; the popup itself is internal/ui/popup/house_display.go): one row per
// form with a radio mark, and under a dotted line the focused form's description
// and an example row. Moving the cursor only changes what the lower area says;
// Space chooses the focused form and puts it live on the Bodies panel at once,
// with the popup still open. Enter and Esc close and keep what is chosen, as
// the Aspects and Bodies choosers do.
//
// The choice is the session's, for every slot. It is written by `, c d` (with
// the default city) and read at start (house_display.go); the popup itself
// writes nothing.

// popupTitleDisplay is the popup's name in its frame.
const popupTitleDisplay = popup.TitleDisplay

// houseDisplayState is the popup's list, in internal/ui/popup.
type houseDisplayState = popup.HouseDisplay

// displayForms is houseForms as the popup lists them.
func displayForms() []popup.HouseFormRow {
	rows := make([]popup.HouseFormRow, len(houseForms))
	for i, info := range houseForms {
		rows[i] = popup.HouseFormRow{Form: info.form, Title: info.title, Description: info.description}
	}
	return rows
}

// newHouseDisplay is the popup with its cursor on the form in force.
func newHouseDisplay(chosen panel.HouseForm) houseDisplayState {
	return popup.NewHouseDisplay(displayForms(), houseComparingNote, chosen)
}

// openHouseDisplay is the `, a` handler: it opens the popup on the main screen
// with the cursor on the form in force.
func openHouseDisplay(m Model) (tea.Model, tea.Cmd) {
	if m.blocked(keys.BindingHouseDisplay) != "" {
		return m, nil
	}
	m.houseDisplayPopup = newHouseDisplay(m.houseDisplay)
	m.startup = startupDisplayOpen
	return m, nil
}

// displayBindings is the House display popup's table. Nothing in it types, so
// j and k move as they do on the main screen, g and G are Home and End, and q
// leaves as Esc does (C11). Space chooses the focused form and leaves the popup
// open; it is on the bar only while the focus is on a form that is not yet the
// chosen one. Enter and Esc close it keeping the choice.
func displayBindings() []keys.BindingEntry {
	return keys.Titled([]keys.BindingEntry{
		{ID: keys.BindingListUp, Binding: key.NewBinding(key.WithKeys("up", "k")), Title: "Move row", HelpKey: "↑/↓", BarKey: "↑↓", Bar: true},
		{ID: keys.BindingListDown, Folded: true, Binding: key.NewBinding(key.WithKeys("down", "j")), BarKey: "↓"},
		{ID: keys.BindingToggle, Binding: key.NewBinding(key.WithKeys(" ")), Title: "Choose", BarKey: "space", Bar: true, When: func(c keys.Context) bool { return c.Display.CanChoose }},
		{ID: keys.BindingClose, Binding: key.NewBinding(key.WithKeys("enter", "esc", "q")), Title: "Close", BarKey: "enter/esc/q", Bar: true, Pinned: true},
		{ID: keys.BindingPageUp, Title: "Page list", HelpKey: "⇞/⇟", Binding: key.NewBinding(key.WithKeys("pgup")), BarKey: "⇞"},
		{ID: keys.BindingPageDown, Folded: true, Binding: key.NewBinding(key.WithKeys("pgdown")), BarKey: "⇟"},
		{ID: keys.BindingPageHome, Title: "First row", Binding: key.NewBinding(key.WithKeys("home", "g")), BarKey: "home/g"},
		{ID: keys.BindingPageEnd, Title: "Last row", Binding: key.NewBinding(key.WithKeys("end", "G")), BarKey: "end/G"},
	})
}

// updateHouseDisplayKey handles one keystroke while the popup has the keyboard:
// the key resolves through the bindings table to the popup's Action.
func updateHouseDisplayKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	st := m.houseDisplayPopup
	a := popup.Type
	switch keys.MatchBinding(msg, displayBindings()) {
	case keys.BindingClose:
		a = popup.Dismiss
	case keys.BindingListUp:
		a = popup.Up
	case keys.BindingListDown:
		a = popup.Down
	case keys.BindingToggle:
		a = popup.Toggle
	case keys.BindingPageUp:
		a = popup.PageUp
	case keys.BindingPageDown:
		a = popup.PageDown
	case keys.BindingPageHome:
		a = popup.PageHome
	case keys.BindingPageEnd:
		a = popup.PageEnd
	}
	out := st.Apply(a)
	if out.Closed {
		m.startup = startupReady
		return m, nil
	}
	if out.Chose {
		m.houseDisplay = out.Form
	}
	m.houseDisplayPopup = st
	return m, nil
}

// displayCanChoose is where Space acts: the focus is on a form that is not
// the chosen one.
func (m Model) displayCanChoose() bool {
	return m.houseDisplayPopup.CanChoose()
}

// renderHouseDisplay draws the popup.
func renderHouseDisplay(st houseDisplayState, cfg Config, termW, termH int) string {
	return st.Render(cfg, termW, termH, popupChrome(cfg))
}
