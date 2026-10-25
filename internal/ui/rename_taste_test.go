// All chart values reachable from this file are constructed synthetic test inputs.
package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// TestRenameTasteFrames is the specification V2. "titled": the rename
// form Ctrl+R opens on a titled chart, at both sizes, with colour and
// without. The "rich" frame went with the rich-chart refusal
// (the specification Part C). Under -design they are written out for a
// look; otherwise each is compared with its frozen golden.
func TestRenameTasteFrames(t *testing.T) {
	skipWithoutAstrolog(t)
	forceColor(t)
	titled := store.NamedChart{ID: "constructed-titled", Name: "Constructed Titled", PlaceLabel: "Munich, Germany",
		Place:  location.PlaceInfo{Name: "Munich", Country: "DE", Lat: 48.13, Lon: 11.57, TZ: "Europe/Berlin"},
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "Constructed-Titled.as"}}
	modes := []struct {
		name string
		cfg  Config
	}{{"color", Config{Color: ColorAlways}}, {"accessible", Config{Accessible: true, Color: ColorNever}}}

	for _, size := range [][2]int{{132, 40}, {80, 24}} {
		for _, mode := range modes {
			name := "rename-form-titled-" + sizeName(size) + "-" + mode.name
			t.Run(name, func(t *testing.T) {
				// Opened the way the operator opens it: Ctrl+R on the row.
				m := escBase(t)
				m.picker = chartPickerState{loaded: true, prior: startupReady, charts: []store.NamedChart{titled}}
				m.picker.refilter()
				m.startup = startupChartOpen
				m = designStep(m, tea.KeyMsg{Type: tea.KeyCtrlR})
				if m.startup != startupTitleOpen || !m.title.rename {
					t.Fatalf("Ctrl+R did not open the rename form (startup %s)", m.startup)
				}
				writeDesignFrame(t, name, renderTitleForm(m.title, mode.cfg, size[0], size[1]))
			})
		}
	}
}

func sizeName(size [2]int) string {
	if size[0] == 132 {
		return "132x40"
	}
	return "80x24"
}
