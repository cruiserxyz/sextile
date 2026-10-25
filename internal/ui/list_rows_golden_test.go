package ui

import (
	"fmt"
	"testing"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// the specification V1: the striped, right-aligned rows, frozen.
//
// The atlas, chart-box and body-modal goldens already cover their modals and
// are regenerated with this change. These add the two frames nothing covered:
// the chart picker over a library big enough to show the stripe and the
// date column lining up, and the aspect chooser, which had no golden at all.
//
// Regenerate ONLY on a deliberate restyle: `go test ./internal/ui -update`.

// listRowsCharts is six synthetic charts: long and short names, places of
// different widths, and one with no date, so the right edge has something to
// prove.
func listRowsCharts() []store.NamedChart {
	mk := func(name string, p location.PlaceInfo, y, m, d int) store.NamedChart {
		c := testNamedChart(name, p)
		c.Year, c.Month, c.Day = y, m, d
		return c
	}
	bremen := location.PlaceInfo{Name: "Bremen", Country: "DE", TZ: "Europe/Berlin"}
	frankfurt := location.PlaceInfo{Name: "Frankfurt am Main", Country: "DE", TZ: "Europe/Berlin"}
	return []store.NamedChart{
		mk("Constructed One", bremen, 1995, 3, 20),
		mk("Great Conjunction", frankfurt, 2020, 12, 21),
		mk("Fixture Two", berlinP, 1990, 6, 15),
		mk("A Considerably Longer Constructed Subject Name", fixtureOneP, 1987, 11, 2),
		mk("Short", bremen, 2026, 4, 2),
		mk("Undated Subject", berlinP, 1980, 0, 0),
	}
}

func listRowsChartState() chartPickerState {
	st := chartPickerState{loaded: true, prior: startupReady, charts: listRowsCharts()}
	st.refilter()
	st.cursor = 2
	return st
}

func listRowsAspectState(filter string) aspectModalState {
	st := openTest(recipe.AspectPolicy{Explicit: true, Indices: []int{1, 3, 5}})
	st.filterText = filter
	st.refilter()
	st.cursor = familyRows() + 1
	return st
}

func listRowsBodyState() bodyModalState {
	st := openBodyTest(recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 3}}, nil)
	st.cursor = groupRows() + 1 // the Sun, one row into the list past the group rows
	return st
}

var listRowsSizes = []struct {
	name string
	w, h int
}{{"132x40", 132, 40}, {"80x24", 80, 24}}

var listRowsModes = []struct {
	name string
	cfg  Config
}{{"color", Config{Color: ColorAlways}}, {"accessible", Config{Accessible: true}}}

func TestListRowsGolden(t *testing.T) {
	forceColor(t)
	for _, size := range listRowsSizes {
		for _, mode := range listRowsModes {
			name := fmt.Sprintf("list-rows-chart-%s-%s", size.name, mode.name)
			t.Run(name, func(t *testing.T) {
				checkGolden(t, name, renderChartModal(listRowsChartState(), mode.cfg, size.w, size.h))
			})
			for _, st := range []struct{ name, filter string }{{"rest", ""}, {"filtered", "se"}} {
				name := fmt.Sprintf("aspect-modal-%s-%s-%s", st.name, size.name, mode.name)
				t.Run(name, func(t *testing.T) {
					checkGolden(t, name, renderAspectModal(listRowsAspectState(st.filter), mode.cfg, size.w, size.h))
				})
			}
		}
	}
}
