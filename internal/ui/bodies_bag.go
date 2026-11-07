package ui

import (
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// resolveBodiesBag lifts what the Bodies panel is drawn from — the committed
// frame, the scene's view of it, the slot's selection and the size it gets —
// into the one value panel.RenderBodies reads. It is the only place the Model's
// side of the panel meets the renderer, so the renderer never sees the Model,
// the frame type or store. slotIdx and cursorIdx are the slot and Bodies row
// the frame was cast with (a held frame's own, not the live ones).
func resolveBodiesBag(v bodiesView, state chartViewState, slots *store.Slots, slotIdx, cursorIdx, innerWidth, innerHeight int, panelFocused bool, cfg Config) panel.BodiesBag {
	bag := panel.BodiesBag{
		Positions:      state.positions,
		Cusps:          state.cusps,
		Stations:       state.stations,
		Forced:         state.forced,
		Lit:            state.lit,
		LitSGR:         state.mark.Colour,
		BodyStyles:     state.bodyStyles,
		NatalPositions: state.natalPositions,
		NatalCusps:     state.natalCusps,
		Framed:         state.framedPositions,
		HouseForm:      state.houseForm,
		Columns:        v.cols,
		ColumnsOK:      v.ok,
		Comparing:      v.comparing,
		Focus:          v.focus,
		ColStart:       v.colStart,
		Top:            v.top,
		Cursor:         cursorIdx,
		HasSelections:  markedRowShown(slots, slotIdx, state.positions, state.cusps),
		InnerW:         innerWidth,
		InnerH:         innerHeight,
		Focused:        panelFocused,
		Style:          cfg,
	}
	if slots != nil {
		for _, o := range slots.SelectedObjects(slotIdx) {
			if bag.Selected == nil {
				bag.Selected = map[int]bool{}
			}
			bag.Selected[o.Number] = true
		}
	}
	return bag
}

// markedRowShown reports whether the slot has a mark that has a row in the
// Bodies table drawn from positions and cusps: the one condition under which the
// table reserves its mark column. A mark on an object the chart does not draw
// (resolved, kept in the session, but with no row) reserves nothing, so the
// table never shows a column with nothing in it.
func markedRowShown(slots *store.Slots, slotIdx int, positions []aspect.Position, cusps aspect.Cusps) bool {
	if slots == nil || !slots.HasMarks(slotIdx) {
		return false
	}
	marked := map[int]bool{}
	for _, o := range slots.SelectedObjects(slotIdx) {
		marked[o.Number] = true
	}
	return panel.AnyRowMarked(positions, cusps, marked)
}
