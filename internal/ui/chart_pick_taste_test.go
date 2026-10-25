package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/store"
)

// The taste lane its specification,
// V1). The listing itself is NOT taste: C6 and chartRow require it to render
// exactly as it did before this slice, so a changed listing frame is a contract
// failure. Only the two PICK states are a choice — what the picker says while
// it is reading a chart's file, and what it says when that read failed.
//
// Frozen here: both states live in the modal's FOOTER, in the row that
// otherwise carries the key hints, and the modal is laid out to the key-hint
// width in every state so the box never resizes under the operator's eyes.
// `Esc: Cancel` survives into both, because it is what cancels a resolve in
// flight and what dismisses a failure. The REASON is deliberately not in the
// footer: it goes on the error lane, where the 6-row Log and the `,l` console
// both keep it, and the footer points there instead of truncating it.
//
// Regenerate with `go test ./internal/ui -update` after a deliberate restyle;
// TestChartPickTasteContactSheet writes the frames to docs/90-assets/t120-*.txt
// for the operator's pick.

// chartPickTasteModel is a ready pair workspace at w×h with the chart picker
// open over it, its listing loaded, and a pick in the given state.
func chartPickTasteModel(t *testing.T, cfg Config, w, h int, failed bool) Model {
	t.Helper()
	m := tabsPairModel(t, cfg)
	m.width, m.height = w, h
	m.limitedViewShown = true // the first-frame notice is not what this fixture is about
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{
		Positions:    fullChartPositions(),
		Aspects:      tasteOwnAspects(),
		CrossAspects: tasteCrossAspects(),
		Overlay:      true,
	})
	m = withSceneColumns(m)
	m.picker = chartPickerState{
		loaded: true,
		prior:  startupReady,
		charts: []store.NamedChart{
			testNamedChart("Constructed One", fixtureOneP),
			testNamedChart("Great Conjunction", berlinP),
			testNamedChart("Constructed Two", fixtureOneP),
		},
	}
	m.picker.refilter()
	m.startup = startupChartOpen

	m, _ = applyChartPicked(m, ChartPickedMsg{Chart: m.picker.matches[0]})
	if failed {
		m, _ = applyChartResolved(m, ChartResolvedMsg{
			Chart: m.picker.pending,
			Err:   errors.New("astrolog exited 1: cannot open chart file"),
			Gen:   m.pickGen,
		})
	}
	return m
}

// V1: the picker while a pick is resolving, and after one failed — at both
// sizes, in both colour policies.
func TestChartPickTaste(t *testing.T) {
	// tasteModes declares a `color` mode, but lipgloss downgrades to the
	// ASCII profile and strips every style unless the profile is forced --
	// so the four `-color` goldens were byte-identical in SGR to their
	// `-plain` twins and differed only in glyph mode. The sibling taste
	// tests (console, console-argv, scroll, tabs) all force it here.
	forceColor(t)
	for _, size := range tasteSizes {
		for _, mode := range tasteModes {
			for _, state := range []struct {
				name   string
				failed bool
			}{{"resolving", false}, {"failed", true}} {
				if state.failed && size.name == "80x24" {
					// Byte-identical to "resolving" at this size.
					continue
				}
				name := fmt.Sprintf("chart-pick-%s-%s-%s", state.name, size.name, mode.name)
				t.Run(name, func(t *testing.T) {
					m := chartPickTasteModel(t, mode.cfg, size.w, size.h, state.failed)
					writeTasteGolden(t, name, m.View())
				})
			}
		}
	}
}

// TestChartPickTasteContactSheet writes the frames side by side for the
// operator's approval pass. It is a rendering, never an assertion.
func TestChartPickTasteContactSheet(t *testing.T) {
	if !*updateGolden {
		t.Skip("contact sheet is written with -update")
	}
	var b strings.Builder
	b.WriteString("requirement V1 — the chart picker while a pick resolves, and after one failed.\n")
	b.WriteString("The listing is not taste (C6/C17 pin it); the footer row is.\n\n")
	for _, size := range tasteSizes {
		for _, state := range []struct {
			name   string
			failed bool
		}{{"resolving", false}, {"failed", true}} {
			m := chartPickTasteModel(t, Config{Accessible: true}, size.w, size.h, state.failed)
			fmt.Fprintf(&b, "== %s, %s (plain) ==\n%s\n\n", state.name, size.name, m.View())
		}
	}
	path := filepath.Join("..", "..", "docs", "90-assets", "t120-v1-chart-pick-states.txt")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write contact sheet: %v", err)
	}
}

var tasteSizes = []struct {
	name string
	w, h int
}{
	{"132x40", 132, 40},
	{"80x24", 80, 24},
}

var tasteModes = []struct {
	name string
	cfg  Config
}{
	{"color", Config{Color: ColorAlways}},
	{"plain", Config{Accessible: true, Color: ColorNever}},
}
