package popup

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// form.go is what the Save chart prompt (⌃s) and the chart title form share: a
// typed field set, the refusal the last submit drew, and whether a write is in
// flight. The write commands, the done messages, the Log lines and the checks of
// what was typed stay in ui (save_modal.go, title_form.go); the popup holds the
// typed text and draws it, and ui reads the text and acts.

// Two actions only a form has: Tab and Shift+Tab.
const (
	// NextField moves the cursor to the next field.
	NextField Action = Trash + 1 + iota
	// PrevField moves it to the previous one.
	PrevField
)

// Form is the typed field set and its state.
type Form struct {
	Fields dialog.Form
	// Refusal is the last submit's refusal; the form stays open and says it.
	Refusal string
	// Saving is a write in flight: nothing is typed, and the name on screen is the
	// name being written.
	Saving bool
	// Confirming is a question the form asks before it writes: the title form's
	// change question and the save prompt's first-write question. Nothing is
	// written until it is answered.
	Confirming bool
}

// Apply runs one action: typing or a move between fields. While a write is in
// flight it does nothing.
func (f *Form) Apply(a Action, msg tea.KeyMsg) {
	if f.Saving {
		return
	}
	switch a {
	case NextField:
		f.Fields = f.Fields.NextField()
	case PrevField:
		f.Fields = f.Fields.PrevField()
	case Type:
		f.Fields, _ = f.Fields.UpdateKey(msg)
	}
}

// Render draws the form with the lines ui computed for it: the box, the fields,
// the refusal under them.
func (f Form) Render(v dialog.FormView, cfg term.Style, termW, termH int, chrome dialog.Chrome) string {
	v.Refusal = f.Refusal
	return dialog.RenderForm(f.Fields, v, cfg, termW, termH, chrome)
}
