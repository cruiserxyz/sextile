package ui

import (
	"strings"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
)

// house_display.go is how the Bodies house column is drawn. There are four
// forms, chosen in the `, a` popup (house_display_popup.go) and kept with the
// default city by `, c d`; every one of them is drawn by panel.HouseColumn, the one
// seam, so no row renderer draws a form by hand.
//
// All four fill the same 18 cells as the frame always did, so the Bodies panel
// never shifts when the form changes. A row that carries the chart's own house
// AND the house in the compared chart (the overlay) draws the own house faint
// and the compared house bold in every form.

// houseFormInfo is what the popup and the preferences file know about a form.
type houseFormInfo struct {
	form panel.HouseForm
	// value is the word preferences.yaml carries.
	value string
	title string
	// description is one or two sentences; every one is followed, on its own
	// line, by houseComparingNote.
	description string
}

// houseComparingNote ends every form's description, on its own line.
const houseComparingNote = "Comparing: the chart's own house faint, the house in the compared chart bold."

// houseForms lists the forms in the order the popup shows them.
var houseForms = []houseFormInfo{
	{panel.HouseAngularity, "angularity", "Angularity at a glance",
		"Houses grouped around the four angles: Ascendant, IC, Descendant, Midheaven. In each group the middle cell is angular, the right succedent, the left cadent."},
	{panel.HouseQuadrants, "quadrants", "True quadrants",
		"Houses in the four quadrants, 1-3, 4-6, 7-9, 10-12, counted from the Ascendant."},
	{panel.HouseNumber, "number", "Plain number",
		"The house number alone."},
	{panel.HouseMarked, "marked", "Number + angularity",
		"The house number with A for angular, s for succedent, c for cadent."},
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
	return m.logNote("preferences", "house_display %q ignored: expected %s. Angularity stands", c.Value, houseFormWords())
}

// houseFormWords lists the words a preferences file may carry, for the Log note
// that refuses any other.
func houseFormWords() string {
	words := make([]string, len(houseForms))
	for i, f := range houseForms {
		words[i] = f.value
	}
	return strings.Join(words[:len(words)-1], ", ") + " or " + words[len(words)-1]
}
