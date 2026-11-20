package ui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// reportSearchBindings is the bar while a Details report's `/` line is open.
func reportSearchBindings() []keys.BindingEntry {
	return keys.TitledLocal([]keys.BindingEntry{
		{ID: keys.BindingConfirm, Binding: key.NewBinding(key.WithKeys("enter")), Title: "Apply", BarKey: "enter", Bar: true},
		{ID: keys.BindingClose, Binding: key.NewBinding(key.WithKeys("esc")), Title: "Back", BarKey: "esc", Bar: true},
		// A letter is text here, and no scroll or paging key is read.
		{ID: keys.BindingCopy, Binding: key.NewBinding(), Title: "Copy", BarKeyFn: chordKey("y"), BarKey: ", y", Chord: true, HelpWhen: func(keys.Context) bool { return false }},
	})
}
