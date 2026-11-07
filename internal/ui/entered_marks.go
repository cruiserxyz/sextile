package ui

import (
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// enteredMark is rules 3 and 6 of the spec, from the scene: nothing while
// Bodies and Aspects paint a held frame (the marks never ride on the previous
// scene's frame), nothing without an entered row whose names all resolved.
func (m Model) enteredMark() panel.EnteredMark {
	if m.holding() {
		return panel.EnteredMark{}
	}
	sc := m.deriveScene()
	e := sc.entered
	if e == nil {
		return panel.EnteredMark{}
	}
	row, ok := m.enteredTimelineRow()
	if !ok || !row.Resolved {
		return panel.EnteredMark{}
	}
	switch row.Kind {
	case lookup.TimelineKindAspect:
		mk := panel.EnteredMark{First: row.Object, Other: row.OtherObject, Kind: row.AspectID}
		if e.chart == nil {
			mk.Tab = string(TabAspects)
			return mk
		}
		// A chart row's cell is on the pair tab that has the floating slot as
		// its right side and chart k as its left: the cross list then holds
		// chart k's natal bodies against the bodies at the row's instant.
		for i := range sc.pairs {
			if sc.pairs[i].left == e.chart && sc.pairs[i].right.Floating {
				mk.Tab, mk.ChartRow = string(sc.pairTabID(i)), true
				return mk
			}
		}
	}
	return panel.EnteredMark{}
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

// enteredStation is C15 of the specification: while an entered
// STATION row holds, its body shows the row's own word (after lookup's node
// relabel) in the speed lane whatever the 5% rule says, so the timeline row and
// the Bodies row cannot disagree. Nothing while a held frame paints, and
// nothing once the entered row ends: the rule alone decides again.
func (m Model) enteredStation() map[int]lookup.Station {
	if m.holding() || m.deriveScene().entered == nil {
		return nil
	}
	row, ok := m.enteredTimelineRow()
	if !ok || !row.Resolved || row.Object == (aspect.Object{}) || row.Kind != lookup.TimelineKindStation {
		return nil
	}
	switch row.Station {
	case "S/R":
		return map[int]lookup.Station{row.Object.Number: lookup.StationRetro}
	case "S/D":
		return map[int]lookup.Station{row.Object.Number: lookup.StationDirect}
	}
	return nil
}

// enterColour is the enter-colour experiment (experiments.go): with the switch
// on and colours on, the entered aspect row's mark carries Astrolog's colour of
// its cell, and the Bodies row of the led object (lookup.LeadObject) is lit in
// it. The colour is read from the same lists and styles the Aspects waterfall
// draws, on every paint, so nothing is stored; a mark with no style at hand
// stays as it was.
func (m Model) enterColour(state chartViewState) chartViewState {
	if !m.experiment(expEnterColour) || !m.cfg.UseColor() || state.mark.Tab == "" {
		return state
	}
	state.mark = state.mark.Coloured(state.aspects, state.aspectStyles, state.crossAspects, state.crossAspectStyles)
	if state.mark.Colour == "" {
		return state
	}
	row, ok := m.enteredTimelineRow()
	if !ok {
		state.mark.Colour = ""
		return state
	}
	if o, ok := lookup.LeadObject(row); ok {
		state.lit = o
	} else {
		state.mark.Colour = ""
	}
	return state
}
