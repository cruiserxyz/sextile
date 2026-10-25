package ui

// the specification V1: the zodiac finder - empty search, and `lah` typed
// - and the Charts list tag with a Lahiri row and a tropical row, frozen from
// the specification F1 (its zodiac rows) and F4. Chart names are
// made up.
//
// Regenerate ONLY on a deliberate restyle: `go test ./internal/ui -update`.

import (
	"fmt"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// zodiacFinderFrame is the zodiac finder on a chart whose own zodiac is current,
// the cursor on it when the search is empty.
func zodiacFinderFrame(name, filter string, current recipe.Zodiac, termH int) zodiacFinderState {
	st := zodiacFinderState{name: name, current: current}
	st.filterText = filter
	st.refilter()
	for i, z := range st.matches {
		if z == current {
			st.cursor = i
		}
	}
	st.scrollTo(st.cursor, modalListHeight(termH))
	return st
}

func TestZodiacGolden(t *testing.T) {
	forceColor(t)
	modes := []struct {
		name string
		cfg  Config
	}{{"color", Config{Color: ColorAlways}}, {"accessible", Config{Accessible: true}}}
	for _, size := range listRowsSizes {
		for _, mode := range modes {
			frames := []struct {
				name  string
				state zodiacFinderState
			}{
				{"empty", zodiacFinderFrame("Constructed One", "", recipe.Tropical, size.h)},
				{"lah", zodiacFinderFrame("Constructed One", "lah", recipe.Lahiri, size.h)},
			}
			for _, fr := range frames {
				name := fmt.Sprintf("zodiac_finder_%s_%s_%s", fr.name, size.name, mode.name)
				t.Run(name, func(t *testing.T) {
					checkGolden(t, name, renderZodiacFinder(fr.state, mode.cfg, size.w, size.h))
				})
			}
		}
		name := fmt.Sprintf("zodiac_tag_%s", size.name)
		t.Run(name, func(t *testing.T) {
			m := f4Model(t, size.w, size.h)
			// Maria, unselected, is Koch and Lahiri; the group is Placidus and tropical.
			rec, _ := m.frames.find("maria.as", false)
			rec.Zodiac, rec.HasZodiac = "sidereal Lahiri", true
			m.frames = m.frames.with(rec)
			checkGolden(t, name, ansi.Strip(m.View()))
		})
	}
}
