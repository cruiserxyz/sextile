package ui

import (
	"errors"
	"fmt"
	"testing"

	"github.com/cruiserxyz/sextile/internal/store"
)

// The step 2, gate 1 — the characterization net for rebinding the chart
// picker onto the shared modal box (docs/99-archive/the specification).
//
// renderChartModal and renderAtlasModal are character-identical in their box
// math and differ only in the border colour ("12" blue against "10" green).
// chart_modal.go:134 says that duplication stands only "so the tested atlas
// render is untouched; a future simplify pass can converge them" — step 1
// tested atlas, so the box converges here.
//
// The picker already has 8 taste goldens (chart-pick-{resolving,failed}-…),
// but they cover only the two FOOTER states and render the whole frame. These
// cover the branches of renderChartResultList and chartRows that nothing else
// does, isolated to the modal so a box change fails here and nowhere else:
// at rest (an empty filter BROWSES, where atlas shows a hint), filtered, no
// matches, loading, load error, drops present, and an empty library.
//
// Frozen against the PRE-EXTRACTION implementation. After the rebind they must
// be byte-identical; if one cannot be, the extraction is wrong and the box
// changes — never the golden.

func chartBoxCharts() []store.NamedChart {
	return []store.NamedChart{
		testNamedChart("Constructed One", fixtureOneP),
		testNamedChart("Great Conjunction", berlinP),
		testNamedChart("Constructed Two", fixtureOneP),
	}
}

func chartBoxDrops() []store.ChartDrop {
	return []store.ChartDrop{
		{File: "broken.yaml", Reason: "no birth time"},
		{File: "truncated.yaml", Reason: "unreadable YAML"},
	}
}

// chartBoxState builds a loaded picker through the REAL refilter(), so these
// characterize the shipped pipeline rather than a hand-assembled match list.
func chartBoxState(filter string, charts []store.NamedChart, drops []store.ChartDrop) chartPickerState {
	st := chartPickerState{
		loaded: true,
		prior:  startupReady,
		charts: charts,
		drops:  drops,
	}
	st.filterText = filter
	st.refilter()
	return st
}

func TestChartModalBoxGolden(t *testing.T) {
	// Without this lipgloss strips every style off a TTY, and the border
	// colour — the one thing that differs between this box and the atlas
	// box — would not be in the goldens at all.
	forceColor(t)

	all := chartBoxCharts()

	cases := []struct {
		name  string
		state chartPickerState
	}{
		// An empty filter browses the whole library. This is the branch that
		// most distinguishes the picker from the atlas modal, which renders a
		// "(type to filter)" hint instead, and it is why the two lists are NOT
		// converged by this step.
		{"rest", chartBoxState("", all, nil)},
		{"filtered", chartBoxState("two", all, nil)},
		{"nomatches", chartBoxState("zzz", all, nil)},
		{"loading", chartPickerState{loaded: false, prior: startupReady}},
		{"loaderr", chartPickerState{loaded: true, prior: startupReady, loadErr: errors.New("charts dir unreadable")}},
		// Drops reserve rows from the chart block under the rule stated in
		// renderChartResultList: an astrologer whose files failed to parse
		// must never be told they have no charts.
		{"drops", chartBoxState("", all, chartBoxDrops())},
		// The empty library with no drops is the only state that says
		// "(no saved charts yet)"; with drops present that line is suppressed.
		{"empty", chartBoxState("", nil, nil)},
	}

	modes := []struct {
		name string
		cfg  Config
	}{
		{"color", Config{Color: ColorAlways}},
		{"plain", Config{Accessible: true, Color: ColorNever}},
	}

	for _, tc := range cases {
		for _, md := range modes {
			name := fmt.Sprintf("chart-box-%s-80x24-%s", tc.name, md.name)
			t.Run(name, func(t *testing.T) {
				checkGolden(t, name, renderChartModal(tc.state, md.cfg, 80, 24))
			})
		}
	}
}
