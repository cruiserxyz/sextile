package ui

import (
	"fmt"
	"testing"
)

// H5 (frames): one component golden per new form of the house column, the
// Bodies body at one width with the chart's own house and the house in a
// compared chart's frame side by side, and the House display popup at both
// sizes. The default form's column is not pinned here: it is every existing
// Bodies golden, and none of them moves.
//
// Regenerate ONLY on a deliberate restyle: `go test ./internal/ui -update`.

func TestHouseDisplayGoldens(t *testing.T) {
	forceColor(t)

	for _, info := range houseForms {
		if info.form == houseAngularity {
			continue
		}
		name := fmt.Sprintf("house-display-column-%s-132x40-color", info.value)
		t.Run(name, func(t *testing.T) {
			state := chartViewState{
				positions: fullChartPositions(), cusps: fullChartCusps(),
				natalCusps: wholeSignLeoRising, natalPositions: shiftedChart(40),
				houseForm: info.form,
			}
			body, _, _ := renderBodiesBodyAt(state, nil, 0, 0, 64, 18, false, Config{Color: ColorAlways}, 0)
			checkGolden(t, name, body)
		})
	}

	popups := []struct {
		name           string
		cursor, chosen int
		cfg            Config
		w, h           int
	}{
		{"rest-132x40-color", 0, 0, Config{Color: ColorAlways}, 132, 40},
		{"moved-132x40-color", 3, 1, Config{Color: ColorAlways}, 132, 40},
		{"rest-80x24-color", 0, 0, Config{Color: ColorAlways}, 80, 24},
		{"moved-80x24-color", 3, 1, Config{Color: ColorAlways}, 80, 24},
		{"moved-80x24-accessible", 2, 1, Config{Accessible: true}, 80, 24},
	}
	for _, p := range popups {
		name := "house-display-popup-" + p.name
		t.Run(name, func(t *testing.T) {
			list := newRadioList(len(houseForms), p.chosen)
			list.cursor = p.cursor
			checkGolden(t, name, renderHouseDisplay(houseDisplayState{list: list}, p.cfg, p.w, p.h))
		})
	}
}
