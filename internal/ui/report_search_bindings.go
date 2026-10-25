package ui

import "github.com/charmbracelet/bubbles/key"

// reportSearchBindings is the bar while a Details report's `/` line is open.
func reportSearchBindings() []BindingEntry {
	return []BindingEntry{
		{ID: BindingConfirm, Binding: key.NewBinding(key.WithKeys("enter")), Action: "Apply", BarKey: "enter", Bar: true},
		{ID: BindingClose, Binding: key.NewBinding(key.WithKeys("esc")), Action: "Back", BarKey: "esc", Bar: true},
		// The line passes the Details scroll through; the report has no cursor
		// for the plain paging keys, and a letter is text here.
		{ID: BindingScrollUp, Binding: key.NewBinding(key.WithKeys("ctrl+pgup")), BarKey: "⌃⇞"},
		{ID: BindingScrollDown, Binding: key.NewBinding(key.WithKeys("ctrl+pgdown")), BarKey: "⌃⇟"},
	}
}
