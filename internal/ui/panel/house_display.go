package panel

import (
	"strconv"
	"strings"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// HouseForm is one way of drawing the house column. The zero value is the
// default, so a renderer built without a choice draws today's frame.
type HouseForm int

const (
	// HouseAngularity is the frame `12 1 2 | 3 4 5 | 6 7 8 | 9 10 11`: each
	// bracket is one angle with its neighbours.
	HouseAngularity HouseForm = iota
	// HouseQuadrants is the frame `1 2 3 | 4 5 6 | 7 8 9 | 10 11 12`.
	HouseQuadrants
	// HouseNumber is the house number alone.
	HouseNumber
	// HouseMarked is the number and one letter: A angular, s succedent, c cadent.
	HouseMarked
)

// HouseColumnWidth is the cells every form fills.
const HouseColumnWidth = 18

// QuadrantsFrame is the true-quadrants frame: houses 1 to 12 in order, three to
// a quadrant, one cell per single-digit house and two per double-digit one, so
// every house owns cells no other house does. 3+1+3+1+3+1+6 = 18 cells, the
// same as the angularity frame.
var QuadrantsFrame = func() houseFrame {
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
func compactHouseText(form HouseForm, h aspect.House) string {
	if !h.Valid() {
		return ""
	}
	if form == HouseMarked {
		return strconv.Itoa(int(h)) + houseAngularLetter(h)
	}
	return strconv.Itoa(int(h))
}

// HouseColumn is the one seam that draws the house column of a Bodies row, in
// the form the operator chose: the space that separates it from the row, then
// exactly HouseColumnWidth cells, or "" when there is no house to show.
//
// comparing says the row carries two houses: own, the chart's own house, drawn
// faint, and compared, the house in the compared chart, drawn bold. Frame forms
// place each number at its own cell, the same house in both sharing one, bold;
// compact forms put own first, then compared, always, also when they are equal. With colours off weight
// is lost and position (or order) carries the difference.
//
// Not comparing, the row has one house, own, and the whole column wears style
// (Astrolog's own house colour), with no weight.
func HouseColumn(form HouseForm, own, compared aspect.House, comparing bool, style astrologstyle.SGR, useColor bool) string {
	if comparing {
		cell := ""
		switch form {
		case HouseQuadrants:
			cell = QuadrantsFrame.Compose(own, compared, style, useColor)
		case HouseNumber, HouseMarked:
			cell = composeCompactHouses(form, own, compared, style, useColor)
		default:
			cell = angularityFrame.Compose(own, compared, style, useColor)
		}
		if cell == "" {
			return ""
		}
		return " " + cell
	}
	cell := ""
	switch form {
	case HouseQuadrants:
		cell = QuadrantsFrame.Of(own)
	case HouseNumber, HouseMarked:
		if text := compactHouseText(form, own); text != "" {
			cell = text + strings.Repeat(" ", HouseColumnWidth-len(text))
		}
	default:
		cell = angularityFrame.Of(own)
	}
	if cell == "" {
		return ""
	}
	tail := " " + cell
	if useColor {
		return RenderSGR(style, tail)
	}
	return tail
}

// composeCompactHouses is the compact forms' two-house column: own faint, then
// compared bold, padded to the column's width. The gap is two cells for the
// plain number and one for the marked form, whose letter takes the other.
func composeCompactHouses(form HouseForm, own, compared aspect.House, style astrologstyle.SGR, useColor bool) string {
	a, b := compactHouseText(form, own), compactHouseText(form, compared)
	if a == "" && b == "" {
		return ""
	}
	gap := "  "
	if form == HouseMarked {
		gap = " "
	}
	var out, plain string
	switch {
	case a == "":
		out, plain = renderLive(b, style, useColor), b
	case b == "":
		out, plain = RenderDim(a, useColor), a
	default:
		out, plain = RenderDim(a, useColor)+gap+renderLive(b, style, useColor), a+gap+b
	}
	return out + strings.Repeat(" ", max(HouseColumnWidth-len(plain), 0))
}
