package ui

import (
	"fmt"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// the specification V1: the finder's house rows, frozen from
// the specification F1 (without its Zodiac group, which
// the specification adds), F2 and F3. Chart names are made up.
//
// Regenerate ONLY on a deliberate restyle: `go test ./internal/ui -update`.

func houseFinderFrame(name, filter string, current recipe.HouseSystem) houseFinderState {
	st := houseFinderState{name: name, current: current, slotIdx: 0}
	st.filterText = filter
	st.refilter()
	for i, h := range st.matches {
		if h == current {
			st.cursor = i
		}
	}
	return st
}

func TestHouseFinderGolden(t *testing.T) {
	forceColor(t)
	frames := []struct {
		name  string
		state houseFinderState
	}{
		{"f1", houseFinderFrame("Constructed One", "", recipe.Placidus)},
		{"f2", houseFinderFrame("Constructed One", "pl", recipe.Placidus)},
		{"f3", houseFinderFrame("Maria", "", recipe.Koch)},
	}
	modes := []struct {
		name string
		cfg  Config
	}{{"color", Config{Color: ColorAlways}}, {"accessible", Config{Accessible: true}}}
	for _, fr := range frames {
		for _, size := range listRowsSizes {
			for _, mode := range modes {
				name := fmt.Sprintf("house_finder_%s_%s_%s", fr.name, size.name, mode.name)
				t.Run(name, func(t *testing.T) {
					checkGolden(t, name, renderHouseFinder(fr.state, mode.cfg, size.w, size.h))
				})
			}
		}
	}
}
