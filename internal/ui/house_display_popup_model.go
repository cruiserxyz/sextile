package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/popup"
)

// house_display_popup_model.go is the Model's half of the houses chooser's Shown
// as part (the popup is internal/ui/popup/house_display.go): one row per form
// under the systems, the form in force marked ●. The cursor reaches it from the
// last system and leaves it with Up from the first form; Enter on a form puts it
// in force and closes (applyHouseChoice). Esc leaves everything as it was.
//
// The choice is the session's, for every slot. It is read at start
// (house_display.go) and written by `, c d` (with the default city).

// displayForms is houseForms as the Shown as part lists them.
func displayForms() []popup.HouseFormRow {
	rows := make([]popup.HouseFormRow, len(houseForms))
	for i, info := range houseForms {
		rows[i] = popup.HouseFormRow{Form: info.form, Title: info.title}
	}
	return rows
}
