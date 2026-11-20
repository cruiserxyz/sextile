package ui

import (
	"strings"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
)

// house_display.go is how the Bodies house column is drawn. There are four
// forms, chosen in the houses chooser's Shown as part (house_display_popup_model.go) and kept with
// the default city by `, c d`, or alone by that part's first row; every one of them is drawn by panel.HouseColumn, the one
// seam, so no row renderer draws a form by hand.
//
// All four fill the same 18 cells as the frame always did, so the Bodies panel
// never shifts when the form changes. A row that carries the chart's own house
// AND the house in the compared chart (the overlay) draws the own house faint
// and the compared house bold in every form.

// houseFormInfo is what the Shown as part and the preferences file know about a form.
type houseFormInfo struct {
	form panel.HouseForm
	// value is the word preferences.yaml carries.
	value string
	title string
}

// houseForms lists the forms in the order the Shown as part shows them. The
// reference page (docs/REFERENCE.md, House display) says each in words.
var houseForms = []houseFormInfo{
	{panel.HouseAngularity, "angularity", "Angularity at a glance"},
	{panel.HouseQuadrants, "quadrants", "True quadrants"},
	{panel.HouseNumber, "number", "Plain number"},
	{panel.HouseMarked, "marked", "Number + angularity"},
}

// houseFormRow is the form's row in houseForms.
func houseFormRow(f panel.HouseForm) houseFormInfo {
	for _, i := range houseForms {
		if i.form == f {
			return i
		}
	}
	return houseForms[0]
}

// parseHouseForm reads the word preferences.yaml carries.
func parseHouseForm(s string) (panel.HouseForm, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, i := range houseForms {
		if i.value == s {
			return i.form, true
		}
	}
	return panel.HouseAngularity, false
}

// applyHouseDisplay takes the house_display key into the Model: a known word
// is the form the session starts in, an absent key leaves the default, and any
// other word is refused with one Log note naming the words that are allowed,
// angularity standing. The reader reports; the decision and the Log line are
// made here, as for wheel_format. Silence is the contract for the good cases.
func (m Model) applyHouseDisplay(c store.HouseDisplayConfig) Model {
	if !c.Present {
		return m
	}
	if form, ok := parseHouseForm(c.Value); ok {
		m.houseDisplay = form
		return m
	}
	m.houseDisplay = panel.HouseAngularity
	return m.logNote("settings", "house_display %q not valid, ignored", c.Value)
}
