package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Frames for the specification: an unprobed row, a marked row
// under the cursor with the missing-file line below the Enabled bar, a marked
// row away from the cursor, a long file name and a scrolled list, at 80x24 with
// colours on and in the accessible form. The states are built directly, so the
// frames pin what is drawn for a verdict and not how the verdict was reached.
func TestBodyModalEphemerisGolden(t *testing.T) {
	forceColor(t)

	reg := make([]aspect.RegistryObject, 0, 40)
	names := []string{"Earth", "Sun", "Moon", "Mercury", "Venus", "Mars", "Jupiter", "Saturn", "Uranus", "Neptune", "Pluto", "Chiron", "Ceres", "Pallas", "Juno", "Vesta"}
	for i, n := range names {
		reg = append(reg, aspect.RegistryObject{Number: i, Name: n})
	}
	for i := len(names); i < 40; i++ {
		reg = append(reg, aspect.RegistryObject{Number: 40 + i, Name: fmt.Sprintf("Asteroid %d", i)})
	}
	// cursor is a LIST row (0 = Earth); the group rows sit above it.
	build := func(cursor int, marks map[int][]string) bodyModalState {
		st := openBodyModal(reg, recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2}}, nil)
		for obj, files := range marks {
			st.recordProbe(obj, len(files) == 0, files)
		}
		st.cursor = groupRows() + cursor
		st.scrollTo(cursor, bodyListHeight(24))
		return st
	}
	long := []string{"se_a_very_long_ephemeris_file_name_for_one_asteroid.se1", "second.se1"}
	frames := []struct {
		label string
		state bodyModalState
		w, h  int
	}{
		{"unprobed row under the cursor", build(2, nil), 80, 24},
		{"marked row under the cursor (no room for the missing-file line at 80x24)", build(12, map[int][]string{12: {"se00010s.se1"}}), 80, 24},
		{"marked row away from the cursor", build(1, map[int][]string{12: {"se00010s.se1"}}), 80, 24},
		{"a long file name (no room for the line at 80x24)", build(12, map[int][]string{12: long}), 80, 24},
		{"scrolled list, marked row under the cursor", build(30, map[int][]string{70: {"se00010s.se1"}, 12: {"se00010s.se1"}}), 80, 24},
		// At 80x24 the group block leaves the list one row, which the cursor
		// row takes, so the missing-file line gives way there
		// (the specification 7); it shows once the box is taller.
		{"marked row under the cursor, with the missing-file line (132x40)", build(12, map[int][]string{12: {"se00010s.se1"}}), 132, 40},
		{"a long file name (132x40)", build(12, map[int][]string{12: long}), 132, 40},
	}
	for _, md := range []struct {
		name string
		cfg  Config
	}{
		{"color", Config{Color: ColorAlways}},
		{"accessible", Config{Accessible: true, Color: ColorNever}},
	} {
		name := "body-modal-ephemeris-80x24-" + md.name
		t.Run(name, func(t *testing.T) {
			var b strings.Builder
			for _, f := range frames {
				fmt.Fprintf(&b, "== %s ==\n%s\n", f.label, renderBodyModal(f.state, md.cfg, f.w, f.h))
			}
			checkGolden(t, name, b.String())
		})
	}
}
