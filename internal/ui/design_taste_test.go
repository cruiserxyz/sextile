package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// designStep feeds one message to a model.
func designStep(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

// V1 — the attached timeline at 144x40, colours on. (The cursor-row band and
// selection frames were retired 2026-10-03: the screen tour pins them.)
func TestDesignTasteV1(t *testing.T) {
	skipWithoutAstrolog(t)
	forceColor(t)
	cfg := Config{Color: ColorAlways}
	writeDesignFrame(t, "design-rows-timeline-144x40-color", onTimeline(t, cfg).View())
}

// V3 — colours off: the Chart owner at 80x24, and attached Details at 144x40,
// each in --color=never and --accessible.
func TestDesignTasteV3(t *testing.T) {
	skipWithoutAstrolog(t)
	key := func(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }
	for _, mode := range []struct {
		name string
		cfg  Config
	}{{"color-never", Config{Color: ColorNever}}, {"accessible", Config{Accessible: true}}} {
		ready := func(w, h int) Model {
			m := designDemoModel(t, mode.cfg, w, h)
			m = designStep(m, ObjectRegistryLoadedMsg{Registry: testObjectRegistry()})
			return designStep(m, AspectRegistryLoadedMsg{Registry: testRegistry()})
		}
		writeDesignFrame(t, "design-nocolour-chart-80x24-"+mode.name, designStep(ready(80, 24), runes("1")).View())
		// Enter from the Chart lands on the timeline; `]` shows the report, the frame this sheet pins.
		writeDesignFrame(t, "design-nocolour-details-attached-144x40-"+mode.name, designStep(designStep(ready(144, 40), key(tea.KeyEnter)), runes("]")).View())
	}
}
