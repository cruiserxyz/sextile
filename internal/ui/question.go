package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// The labels of the question popups' buttons.
const (
	buttonYes    = "Yes"
	buttonAlways = "Always"
	buttonNo     = "No"
	buttonCancel = "Cancel"
)

// question is one question popup's table, the one place its keys come from:
// Enter takes the lit button, each button's own key takes it, Esc takes the
// button Esc names (the answer that changes nothing), the arrows and h l move
// the lit button, and every other key does nothing. Open is the button lit
// when the question opens: the answer that loses nothing.
type question struct {
	Buttons []dialog.Button
	Esc     int
	Open    int
}

// The questions: the Charts folder's two, New session and Delete session.
var (
	questionFirstWrite = question{Buttons: []dialog.Button{{Label: buttonYes, Key: "y"}, {Label: buttonAlways, Key: "a"}, {Label: buttonNo, Key: "n"}}, Esc: 2, Open: 2}
	questionChange     = question{Buttons: []dialog.Button{{Label: buttonYes, Key: "y"}, {Label: buttonNo, Key: "n"}}, Esc: 1, Open: 1}
	questionNewSession = question{Buttons: []dialog.Button{{Label: buttonYes, Key: "y"}, {Label: buttonNo, Key: "n"}, {Label: buttonCancel, Key: "esc"}}, Esc: 2, Open: 0}
	questionDelete     = question{Buttons: []dialog.Button{{Label: buttonYes, Key: "y"}, {Label: buttonNo, Key: "n"}}, Esc: 1, Open: 1}
)

// bindings is the question's key table, drawn from its buttons.
func (q question) bindings() []keys.BindingEntry {
	out := []keys.BindingEntry{
		{ID: keys.BindingChoosePrev, Binding: key.NewBinding(key.WithKeys("left", "h")), Title: "Choose left", BarTitle: "Choose", BarKey: "left/right", Bar: true},
		{ID: keys.BindingChooseNext, Binding: key.NewBinding(key.WithKeys("right", "l")), Title: "Choose right", BarKey: "right"},
		{ID: keys.BindingConfirm, Binding: key.NewBinding(key.WithKeys("enter")), Title: "Take", BarKey: "enter", Bar: true},
	}
	var esc keys.BindingEntry
	for i, b := range q.Buttons {
		ks := []string{b.Key}
		id := map[string]keys.BindingID{buttonYes: keys.BindingYes, buttonNo: keys.BindingNo, buttonAlways: keys.BindingToggle}[b.Label]
		if i == q.Esc {
			id = keys.BindingClose
			if b.Key != "esc" {
				ks = append(ks, "esc")
			}
			esc = keys.BindingEntry{ID: id, Binding: key.NewBinding(key.WithKeys(ks...)), Title: b.Label, BarKey: strings.Join(ks, "/"), Bar: true, Pinned: true}
			continue
		}
		out = append(out, keys.BindingEntry{ID: id, Binding: key.NewBinding(key.WithKeys(ks...)), Title: b.Label, BarKey: b.Key, Bar: true})
	}
	return append(out, esc)
}

// key is a key pressed while the question shows: the lit button after it, and
// the label of the button it took, "" when it took none.
func (q question) key(msg tea.KeyMsg, lit int) (int, string) {
	switch keys.MatchBinding(msg, q.bindings()) {
	case keys.BindingChoosePrev:
		return max(lit-1, 0), ""
	case keys.BindingChooseNext:
		return min(lit+1, len(q.Buttons)-1), ""
	case keys.BindingConfirm:
		return lit, q.Buttons[lit].Label
	case keys.BindingYes:
		return lit, buttonYes
	case keys.BindingNo:
		return lit, buttonNo
	case keys.BindingToggle:
		return lit, buttonAlways
	case keys.BindingClose:
		return lit, q.Buttons[q.Esc].Label
	}
	return lit, ""
}
