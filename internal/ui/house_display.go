package ui

import (
	"strconv"
	"strings"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// house_display.go is how the Bodies house column is drawn. There are four
// forms, chosen in the `, a` popup (house_display_popup.go) and kept with the
// default city by `, c d`; every one of them is drawn by houseColumn, the one
// seam, so no row renderer draws a form by hand.
//
// All four fill the same 18 cells as the frame always did, so the Bodies panel
// never shifts when the form changes. A row that carries the chart's own house
// AND the house in the compared chart (the overlay) draws the own house faint
// and the compared house bold in every form.

// houseForm is one way of drawing the house column. The zero value is the
// default, so a renderer built without a choice draws today's frame.
type houseForm int

const (
	// houseAngularity is the frame `12 1 2 | 3 4 5 | 6 7 8 | 9 10 11`: each
	// bracket is one angle with its neighbours.
	houseAngularity houseForm = iota
	// houseQuadrants is the frame `1 2 3 | 4 5 6 | 7 8 9 | 10 11 12`.
	houseQuadrants
	// houseNumber is the house number alone.
	houseNumber
	// houseMarked is the number and one letter: A angular, s succedent, c cadent.
	houseMarked
)

// houseColumnWidth is the cells every form fills.
const houseColumnWidth = 18

// houseFormInfo is what the popup and the preferences file know about a form.
type houseFormInfo struct {
	form houseForm
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
	{houseAngularity, "angularity", "Angularity at a glance",
		"Houses grouped around the four angles: Ascendant, IC, Descendant, Midheaven. In each group the middle cell is angular, the right succedent, the left cadent."},
	{houseQuadrants, "quadrants", "True quadrants",
		"Houses in the four quadrants, 1-3, 4-6, 7-9, 10-12, counted from the Ascendant."},
	{houseNumber, "number", "Plain number",
		"The house number alone."},
	{houseMarked, "marked", "Number + angularity",
		"The house number with A for angular, s for succedent, c for cadent."},
}

// info is the form's row in houseForms.
func (f houseForm) info() houseFormInfo {
	for _, i := range houseForms {
		if i.form == f {
			return i
		}
	}
	return houseForms[0]
}

// parseHouseForm reads the word preferences.yaml carries.
func parseHouseForm(s string) (houseForm, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, i := range houseForms {
		if i.value == s {
			return i.form, true
		}
	}
	return houseAngularity, false
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
	m.houseDisplay = houseAngularity
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

// quadrantsFrame is the true-quadrants frame: houses 1 to 12 in order, three to
// a quadrant, one cell per single-digit house and two per double-digit one, so
// every house owns cells no other house does. 3+1+3+1+3+1+6 = 18 cells, the
// same as the angularity frame.
var quadrantsFrame = func() houseFrame {
	starts := [12]int{0, 1, 2, 4, 5, 6, 8, 9, 10, 12, 14, 16}
	var cells [12]string
	for i := range cells {
		row := []rune("   |   |   |      ")
		for j, r := range []rune(strconv.Itoa(i + 1)) {
			row[starts[i]+j] = r
		}
		cells[i] = string(row)
	}
	return newHouseFrame(cells)
}()

// houseAngularLetter is the letter of the compact marked form: A for the
// angular houses (1 4 7 10), s for the succedent (2 5 8 11), c for the cadent
// (3 6 9 12).
func houseAngularLetter(h aspect.House) string {
	switch (h - 1) % 3 {
	case 0:
		return "A"
	case 1:
		return "s"
	}
	return "c"
}

// compactHouseText is a compact form's text for one house, "" for a house out
// of range.
func compactHouseText(form houseForm, h aspect.House) string {
	if !h.Valid() {
		return ""
	}
	if form == houseMarked {
		return strconv.Itoa(int(h)) + houseAngularLetter(h)
	}
	return strconv.Itoa(int(h))
}

// houseColumn is the one seam that draws the house column of a Bodies row, in
// the form the operator chose: the space that separates it from the row, then
// exactly houseColumnWidth cells, or "" when there is no house to show.
//
// comparing says the row carries two houses: own, the chart's own house, drawn
// faint, and compared, the house in the compared chart, drawn bold. Frame forms
// place each number at its own cell, the same house in both sharing one, bold;
// compact forms put own first, then compared, always, also when they are equal. With colours off weight
// is lost and position (or order) carries the difference.
//
// Not comparing, the row has one house, own, and the whole column wears style
// (Astrolog's own house colour), with no weight.
func houseColumn(form houseForm, own, compared aspect.House, comparing bool, style astrologstyle.SGR, useColor bool) string {
	if comparing {
		cell := ""
		switch form {
		case houseQuadrants:
			cell = quadrantsFrame.compose(own, compared, style, useColor)
		case houseNumber, houseMarked:
			cell = composeCompactHouses(form, own, compared, style, useColor)
		default:
			cell = angularityFrame.compose(own, compared, style, useColor)
		}
		if cell == "" {
			return ""
		}
		return " " + cell
	}
	cell := ""
	switch form {
	case houseQuadrants:
		cell = quadrantsFrame.of(own)
	case houseNumber, houseMarked:
		if text := compactHouseText(form, own); text != "" {
			cell = text + strings.Repeat(" ", houseColumnWidth-len(text))
		}
	default:
		cell = angularityFrame.of(own)
	}
	if cell == "" {
		return ""
	}
	tail := " " + cell
	if useColor {
		return renderSGR(style, tail)
	}
	return tail
}

// composeCompactHouses is the compact forms' two-house column: own faint, then
// compared bold, padded to the column's width. The gap is two cells for the
// plain number and one for the marked form, whose letter takes the other.
func composeCompactHouses(form houseForm, own, compared aspect.House, style astrologstyle.SGR, useColor bool) string {
	a, b := compactHouseText(form, own), compactHouseText(form, compared)
	if a == "" && b == "" {
		return ""
	}
	gap := "  "
	if form == houseMarked {
		gap = " "
	}
	var out, plain string
	switch {
	case a == "":
		out, plain = renderLive(b, style, useColor), b
	case b == "":
		out, plain = renderDim(a, useColor), a
	default:
		out, plain = renderDim(a, useColor)+gap+renderLive(b, style, useColor), a+gap+b
	}
	return out + strings.Repeat(" ", max(houseColumnWidth-len(plain), 0))
}
