package ui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// reportSearchBindings is the bar while a Details report's `/` line is open.
func reportSearchBindings() []keys.BindingEntry {
	return keys.Titled([]keys.BindingEntry{
		{ID: keys.BindingConfirm, Binding: key.NewBinding(key.WithKeys("enter")), Title: "Apply", BarKey: "enter", Bar: true},
		{ID: keys.BindingClose, Binding: key.NewBinding(key.WithKeys("esc")), Title: "Back", BarKey: "esc", Bar: true},
		// The line passes the Details scroll through; the report has no cursor
		// for the plain paging keys, and a letter is text here.
		{ID: keys.BindingScrollUp, Binding: key.NewBinding(key.WithKeys("ctrl+pgup")), BarKey: "⌃⇞"},
		{ID: keys.BindingScrollDown, Binding: key.NewBinding(key.WithKeys("ctrl+pgdown")), BarKey: "⌃⇟"},
	})
}
