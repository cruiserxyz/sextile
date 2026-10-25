package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// The taste frames of the specification: what the C rows
// cannot decide (a popup's name, the counter's spacing, how help reads, how a
// cut looks) is decided by the operator on these. Under -design they are
// written out for a look; otherwise each is compared with its frozen golden.

var designSizesV = []struct {
	name string
	w, h int
}{{"144x40", 144, 40}, {"80x24", 80, 24}}

// designPopup opens one popup on the demo workspace the way the operator does.
func designPopup(t *testing.T, cfg Config, w, h int, name string) Model {
	t.Helper()
	m := designDemoModel(t, cfg, w, h)
	switch name {
	case "city":
		return designStep(m, tea.KeyMsg{Type: tea.KeyCtrlL})
	case "open-chart":
		m.picker = listRowsChartState()
		m.picker.cursor = len(m.picker.matches) / 2
		m.startup = startupChartOpen
		return m
	case "chart-title":
		return designStep(m, ChartPickedMsg{Chart: store.NamedChart{
			ID: "constructed-untitled", Name: "Constructed untitled", Untitled: true, Year: 1990,
			Place:  location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"},
			Source: store.ChartSource{Kind: store.ChartKindFile, File: "constructed-untitled.as"},
		}})
	case "save-chart":
		return designStep(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	// The demo workspace loads no Astrolog registries, so the two choosers
	// open on the fixture registries the list-row goldens use.
	case "bodies":
		m.bodyModal = listRowsBodyState()
		m.startup = startupBodyOpen
		return m
	case "aspects":
		m.aspectModal = listRowsAspectState("")
		m.startup = startupAspectOpen
		return m
	case "keys":
		m, _ = press(t, m, "?")
		return m
	}
	t.Fatalf("no popup %q", name)
	return m
}

// V1 — the seven popups with their names, at both sizes.
func TestDesignPopupsV1(t *testing.T) {
	skipWithoutAstrolog(t)
	forceColor(t)
	cfg := Config{Color: ColorAlways}
	for _, size := range designSizesV {
		for _, name := range []string{"city", "open-chart", "chart-title", "save-chart", "bodies", "aspects", "keys"} {
			writeDesignFrame(t, "design-popup-"+name+"-"+size.name+"-color", designPopup(t, cfg, size.w, size.h, name).View())
		}
	}
}

// V2 — the `N of M` counter: Bodies with the keyboard, and the console at
// 144x40. (Chart, console 80x24 and the open-chart popup are pinned by the
// tour frames, which are byte-identical.)
func TestDesignPopupsV2(t *testing.T) {
	skipWithoutAstrolog(t)
	forceColor(t)
	cfg := Config{Color: ColorAlways}
	for _, size := range designSizesV {
		base := designDemoModel(t, cfg, size.w, size.h)
		m := base
		m.focused = PanelBodies
		writeDesignFrame(t, "design-position-bodies-focused-"+size.name+"-color", m.View())
		if size.w == 144 {
			m, _ = press(t, base, ",", "l")
			writeDesignFrame(t, "design-position-console-"+size.name+"-color", m.View())
		}
	}
}

// V4 — cut lines: a long place name at 80x24.
func TestDesignPopupsV4(t *testing.T) {
	skipWithoutAstrolog(t)
	forceColor(t)
	cfg := Config{Color: ColorAlways}
	m := designDemoModel(t, cfg, 80, 24)
	place := demoPlace
	place.Name = "Llanfairpwllgwyngyllgogerychwyrndrobwllllantysiliogogogoch"
	slots, err := store.NewSlots(demoTransit, place)
	if err != nil {
		t.Fatal(err)
	}
	m.slots, m.cursorIdx = slots, 0
	writeDesignFrame(t, "design-cut-place-80x24-color", m.View())
}
