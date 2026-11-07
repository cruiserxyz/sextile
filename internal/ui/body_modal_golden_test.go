package ui

import (
	"fmt"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Taste-lane goldens for the body selector (the specification § Taste
// lane): V1 the list at rest, V2 the tag bar, V4 the row, V5 filtering and the
// empty result.
//
// V3 is vacant, as the lane says — it was true-node mode, which went to
// § Deferred. The id is left rather than renumbered.

// tasteObjectRegistry is shaped like the real read at the three name lengths
// V4 asks for: short ("Sun"), long ("North Node"), and an abbreviated star
// ("Zeta Retic."). Numbering is 0-based and non-contiguous, as the registry's
// is once the stars begin.
func tasteObjectRegistry() []aspect.RegistryObject {
	return []aspect.RegistryObject{
		{Object: aspect.Object{Number: 0, Name: "Earth"}},
		{Object: aspect.Object{Number: 1, Name: "Sun"}},
		{Object: aspect.Object{Number: 2, Name: "Moon"}},
		{Object: aspect.Object{Number: 3, Name: "Mercury"}},
		{Object: aspect.Object{Number: 11, Name: "Chiron"}},
		{Object: aspect.Object{Number: 12, Name: "Ceres"}},
		{Object: aspect.Object{Number: 16, Name: "North Node"}},
		{Object: aspect.Object{Number: 84, Name: "Sirius"}, Star: true},
		{Object: aspect.Object{Number: 131, Name: "Zeta Retic."}, Star: true},
	}
}

func tasteBodyState(filter string, selection []int) popup.Body {
	st := popup.Open(tasteObjectRegistry(), recipe.ObjectPolicy{
		Baseline: recipe.Explicit, Selection: selection,
	}, nil)
	if filter != "" {
		st.AppendRunes([]rune(filter))
		st.Refilter()
	}
	return st
}

func TestBodyModalGolden(t *testing.T) {
	// Without this lipgloss strips every style off a TTY and the "-color"
	// goldens carry no colour at all — blind to exactly the border and style
	// regressions they exist to catch.
	forceColor(t)

	cases := []struct {
		name  string
		state popup.Body
		sizes [][2]int
	}{
		// V1 — the list at rest. The visible difference from the aspect
		// chooser is the absence of a group block above the rows.
		{"rest", tasteBodyState("", []int{1, 2, 3}), [][2]int{{80, 24}, {132, 40}}},
		// V2 — a selection long enough to truncate at both widths.
		{"tagbar", tasteBodyState("", []int{0, 1, 2, 3, 11, 12, 16, 84, 131}), [][2]int{{80, 24}, {132, 40}}},
		// V4 — the row at three name lengths, with ticked and unticked marks
		// and the cursor on the first row.
		{"row", tasteBodyState("", []int{1, 16, 131}), [][2]int{{80, 24}}},
		// V5 — a filtered subset, and the empty result.
		{"filtered", tasteBodyState("no", []int{16}), [][2]int{{80, 24}}},
		{"empty", tasteBodyState("zzzz", nil), [][2]int{{80, 24}}},
	}

	modes := []struct {
		name string
		cfg  Config
	}{
		{"color", Config{Color: ColorAlways}},
		{"accessible", Config{Accessible: true, Color: ColorNever}},
	}

	for _, tc := range cases {
		for _, size := range tc.sizes {
			for _, md := range modes {
				name := fmt.Sprintf("body-modal-%s-%dx%d-%s", tc.name, size[0], size[1], md.name)
				t.Run(name, func(t *testing.T) {
					checkGolden(t, name, renderBodyModal(tc.state, md.cfg, size[0], size[1]))
				})
			}
		}
	}
}
