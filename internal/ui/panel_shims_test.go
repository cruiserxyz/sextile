package ui

import (
	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The Bodies and Aspects renderers live in internal/ui/panel and read one
// resolved bag. These helpers keep the tests' older call shapes: each builds
// the bag through resolveBodiesBag (the same seam compose uses) or fills in the
// plain-colour Config the renderer's styled form wants.

func renderBodiesFor(v bodiesView, state chartViewState, slots *store.Slots, slotIdx, cursorIdx, innerWidth, innerHeight int, panelFocused bool, cfg Config) (string, int, pane.Viewport) {
	return panel.RenderBodies(resolveBodiesBag(v, state, slots, slotIdx, cursorIdx, innerWidth, innerHeight, panelFocused, cfg))
}

func renderBodiesBody(state chartViewState, slots *store.Slots, slotIdx, cursorIdx, innerWidth, innerHeight int, panelFocused bool, cfg Config) (string, int, pane.Viewport) {
	return renderBodiesBodyAt(state, slots, slotIdx, cursorIdx, innerWidth, innerHeight, panelFocused, cfg, 0)
}

func renderBodiesBodyAt(state chartViewState, slots *store.Slots, slotIdx, cursorIdx, innerWidth, innerHeight int, panelFocused bool, cfg Config, top int) (string, int, pane.Viewport) {
	return renderBodiesFor(bodiesView{top: top}, state, slots, slotIdx, cursorIdx, innerWidth, innerHeight, panelFocused, cfg)
}

func renderSingleBodies(positions []aspect.Position, cusps aspect.Cusps, styles astrologstyle.Bodies, slots *store.Slots, slotIdx, cursorIdx, innerWidth, innerHeight int, panelFocused bool, cfg Config, ov panel.HouseOverlay) (string, int, pane.Viewport) {
	state := chartViewState{positions: positions, cusps: cusps, bodyStyles: styles, natalCusps: ov.Cusps, natalPositions: ov.Positions, framedPositions: ov.Framed, stations: ov.Stations, forced: ov.Forced, houseForm: ov.Form}
	return renderBodiesBody(state, slots, slotIdx, cursorIdx, innerWidth, innerHeight, panelFocused, cfg)
}

var plainColour = Config{Color: ColorNever}

func renderAspectWaterfall(aspects []aspect.Aspect, innerWidth, innerHeight int) string {
	return panel.RenderAspectWaterfallStyled(aspects, nil, innerWidth, innerHeight, plainColour)
}

func renderCrossAspectWaterfall(aspects []lookup.OverlayAspect, innerWidth, innerHeight int) string {
	return panel.RenderCrossAspectWaterfallStyled(aspects, nil, innerWidth, innerHeight, plainColour)
}

func formatAspectEntry(a aspect.Aspect) string {
	return panel.FormatAspectEntryStyled(a, astrologstyle.AspectStyle{}, plainColour)
}

func formatCrossAspectEntry(a lookup.OverlayAspect) string {
	return panel.FormatCrossAspectEntryStyled(a, astrologstyle.AspectStyle{}, plainColour)
}

// colourOf is the launch style a shim's colour flag stands for.
func colourOf(useColor bool) Config {
	if useColor {
		return Config{Color: ColorAlways}
	}
	return plainColour
}

func formatPositionRow(p aspect.Position, useColor bool) string {
	return panel.FormatPositionRowStyled(p, astrologstyle.BodyRow{}, colourOf(useColor), panel.HouseOverlay{})
}

// positionRows is the flat planets-then-cusps ordering.
func positionRows(ps []aspect.Position, useColor bool) []string {
	planets, under := panel.BodyRows(ps, nil, colourOf(useColor), panel.HouseOverlay{})
	rows := make([]string, 0, len(planets)+len(under))
	for _, row := range append(planets, under...) {
		rows = append(rows, row.Text)
	}
	return rows
}
