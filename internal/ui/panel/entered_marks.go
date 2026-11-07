package panel

import (
	"strings"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/term"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// EnteredMark is what the entered row marks on screen while it holds
// (the specification): the aspect cell of the Aspects tab that
// is the row's own, and the moving object's row in Bodies. It is derived from
// the scene on every paint (EnteredMark below) and never stored, so it cannot
// outlive the row it came from; the zero value marks nothing.
//
// The names were resolved on the Astrolog side (lookup.TimelineRow.Resolved):
// the ui only compares the resolved values.
type EnteredMark struct {
	// Tab is the Aspects Tab the cell mark belongs to: `Aspects` for a sky row,
	// the row's own pair Tab for a chart row. Empty when there is no cell mark.
	Tab string
	// ChartRow says the cell is a cross cell (natal = left object of chart k,
	// transit = right object); otherwise it is a cell of the own list, objects
	// in either order.
	ChartRow     bool
	First, Other aspect.Object // the row's first object (transiting) and second (natal)
	Kind         aspect.AspectKind
	// Colour is Astrolog's colour of the marked cell's aspect glyph, set only by
	// the enter-colour experiment (Coloured): the cell's orb and the led Bodies
	// row's name and degrees are drawn in it. Empty draws today's frame.
	Colour astrologstyle.SGR
}

// ownCell is whether an aspect of the focused chart's own list is the row's:
// the same two objects in either order, and the same aspect.
func (k EnteredMark) ownCell(a aspect.Aspect) bool {
	if k.Tab == "" || k.ChartRow || a.Kind != k.Kind {
		return false
	}
	return (a.AObject == k.First && a.BObject == k.Other) || (a.AObject == k.Other && a.BObject == k.First)
}

// crossCell is whether a cross cell is the row's: the natal object of chart k on
// the left, the transiting object on the right.
func (k EnteredMark) crossCell(a lookup.OverlayAspect) bool {
	return k.Tab != "" && k.ChartRow && a.Kind == k.Kind && a.NatalObject == k.Other && a.TransitObject == k.First
}

// BoldCell makes a formatted waterfall cell bold: Astrolog's own colours stay
// as they are and bold is added across the cell's text, re-applied after each
// reset inside it. The padding that fills the column stays outside the span,
// so the line's right trim still finds its spaces.
func BoldCell(entry string) string {
	text := strings.TrimRight(entry, " ")
	if text == "" {
		return entry
	}
	return term.SGRBold + menu.ReapplyAfterSGR(text, term.SGRBold) + term.SGRReset + entry[len(text):]
}

// Coloured is k with Colour set to the aspect glyph's colour of the cell the
// mark holds, read from the same lists and styles the waterfalls draw: the
// own list on a sky row's tab, the cross list on a chart row's. A mark whose
// cell is not in its list, or whose style is not at hand (no identity in the
// loaded styles), stays as it was.
func (k EnteredMark) Coloured(own []aspect.Aspect, ownStyles astrologstyle.Aspects, cross []lookup.OverlayAspect, crossStyles astrologstyle.Aspects) EnteredMark {
	if k.Tab == "" {
		return k
	}
	if k.ChartRow {
		for _, a := range cross {
			if k.crossCell(a) && a.NatalObject != (aspect.Object{}) && a.TransitObject != (aspect.Object{}) {
				k.Colour = crossStyles[astrologstyle.AspectIdentity{First: a.NatalObject, Second: a.TransitObject, Kind: a.Kind}].Kind
				return k
			}
		}
		return k
	}
	for _, a := range own {
		if k.ownCell(a) && a.AObject != (aspect.Object{}) && a.BObject != (aspect.Object{}) {
			k.Colour = ownStyles[astrologstyle.AspectIdentity{First: a.AObject, Second: a.BObject, Kind: a.Kind}].Kind
			return k
		}
	}
	return k
}
