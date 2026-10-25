package ui

import (
	"strings"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// enteredMark is what the entered row marks on screen while it holds
// (the specification): the aspect cell of the Aspects tab that
// is the row's own, and the moving body's row in Bodies. It is derived from
// the scene on every paint (enteredMark below) and never stored, so it cannot
// outlive the row it came from; the zero value marks nothing.
//
// The names were resolved on the Astrolog side (lookup.TimelineRow.Resolved):
// the ui only compares the resolved values.
type enteredMark struct {
	// tab is the Aspects tab the cell mark belongs to: `Aspects` for a sky row,
	// the row's own pair tab for a chart row. Empty when there is no cell mark.
	tab TabID
	// chartRow says the cell is a cross cell (natal = left body of chart k,
	// transit = right body); otherwise it is a cell of the own list, bodies in
	// either order.
	chartRow     bool
	first, other aspect.Body // the row's first body (transiting) and second (natal)
	kind         aspect.AspectKind
}

// enteredMark is rules 3 and 6 of the spec, from the scene: nothing while
// Bodies and Aspects paint a held frame (the marks never ride on the previous
// scene's frame), nothing without an entered row whose names all resolved.
func (m Model) enteredMark() enteredMark {
	if m.holding() {
		return enteredMark{}
	}
	sc := m.deriveScene()
	e := sc.entered
	if e == nil {
		return enteredMark{}
	}
	row, ok := m.enteredTimelineRow()
	if !ok || !row.Resolved {
		return enteredMark{}
	}
	switch row.Kind {
	case lookup.TimelineKindAspect:
		mk := enteredMark{first: row.BodyID, other: row.OtherID, kind: row.AspectID}
		if e.chart == nil {
			mk.tab = TabAspects
			return mk
		}
		// A chart row's cell is on the pair tab that has the floating slot as
		// its right side and chart k as its left: the cross list then holds
		// chart k's natal bodies against the bodies at the row's instant.
		for i := range sc.pairs {
			if sc.pairs[i].left == e.chart && sc.pairs[i].right.Floating {
				mk.tab, mk.chartRow = sc.pairTabID(i), true
				return mk
			}
		}
	}
	return enteredMark{}
}

// enteredTimelineRow finds the pressed row among the timeline's rows by its
// identity, so the mark reads the row's fields afresh and not a copy taken at
// the press.
func (m Model) enteredTimelineRow() (lookup.TimelineRow, bool) {
	for _, r := range m.timelineRows() {
		if markOf(r) == m.details.pressedAt {
			return r, true
		}
	}
	return lookup.TimelineRow{}, false
}

// ownCell is whether an aspect of the focused chart's own list is the row's:
// the same two bodies in either order, and the same aspect.
func (k enteredMark) ownCell(a aspect.Aspect) bool {
	if k.tab == "" || k.chartRow || a.Kind != k.kind {
		return false
	}
	return (a.A == k.first && a.B == k.other) || (a.A == k.other && a.B == k.first)
}

// crossCell is whether a cross cell is the row's: the natal body of chart k on
// the left, the transiting body on the right.
func (k enteredMark) crossCell(a lookup.OverlayAspect) bool {
	return k.tab != "" && k.chartRow && a.Kind == k.kind && a.Natal == k.other && a.Transit == k.first
}

// boldCell makes a formatted waterfall cell bold: Astrolog's own colours stay
// as they are and bold is added across the cell's text, re-applied after each
// reset inside it. The padding that fills the column stays outside the span,
// so the line's right trim still finds its spaces.
func boldCell(entry string) string {
	text := strings.TrimRight(entry, " ")
	if text == "" {
		return entry
	}
	return sgrBold + reapplyAfterSGR(text, sgrBold) + sgrReset + entry[len(text):]
}

// enteredStation is C15 of the specification: while an entered
// STATION row holds, its body shows the row's own word (after lookup's node
// relabel) in the speed lane whatever the 5% rule says, so the timeline row and
// the Bodies row cannot disagree. Nothing while a held frame paints, and
// nothing once the entered row ends: the rule alone decides again.
func (m Model) enteredStation() map[aspect.Body]lookup.Station {
	if m.holding() || m.deriveScene().entered == nil {
		return nil
	}
	row, ok := m.enteredTimelineRow()
	if !ok || !row.Resolved || row.Kind != lookup.TimelineKindStation {
		return nil
	}
	switch row.Station {
	case "S/R":
		return map[aspect.Body]lookup.Station{row.BodyID: lookup.StationRetro}
	case "S/D":
		return map[aspect.Body]lookup.Station{row.BodyID: lookup.StationDirect}
	}
	return nil
}
