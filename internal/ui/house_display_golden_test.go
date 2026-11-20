package ui

import (
	"fmt"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"testing"
)

// H5 (frames): one component golden per new form of the house column, the
// Bodies body at one width with the chart's own house and the house in a
// compared chart's frame side by side. The default form's column is not pinned
// here: it is every existing Bodies golden, and none of them moves.
//
// Regenerate ONLY on a deliberate restyle: `go test ./internal/ui -update`.

func TestHouseDisplayGoldens(t *testing.T) {
	forceColor(t)

	for _, info := range houseForms {
		if info.form == panel.HouseAngularity {
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
}
