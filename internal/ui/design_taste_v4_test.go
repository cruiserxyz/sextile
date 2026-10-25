package ui

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// The taste lane of the specification (V1–V4) renders frames for
// the operator to look at with `cat` in their own terminal, before anything
// freezes. The operator approved them on 2026-10-02, so they are goldens now:
// every run compares against internal/ui/testdata, -update re-freezes, and
// -design writes them to a directory for another look instead.
//
//	go test ./internal/ui -run TestDesignTaste -design ../../tmp/design-frames
//
// This file uses only what the tree had before piece 2, so it can also be run
// against main to render V4's "before" frames (-design-label before).
var (
	designFrames = flag.String("design", "", "write the design-system taste frames into this directory")
	designLabel  = flag.String("design-label", "after", "suffix for V4's frames: before or after")
)

// skipWithoutAstrolog skips a frame that only the real Astrolog can colour.
// ./check refuses to run without astrolog, so a receipt never skips these.
func skipWithoutAstrolog(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog is not on PATH; the taste frames carry its real colours")
	}
}

// designDemoModel is the demo's invented client over today, with the pair
// selected, computed by the real Astrolog so every body row carries the
// colours it prints (demo_capture_test.go).
func designDemoModel(t *testing.T, cfg Config, w, h int) Model {
	t.Helper()
	chart := store.AnchorChart{When: demoBirth, Place: demoPlace}
	now := computeDemoMoment(t, chart, demoTransit)
	raw := location.RawPlace{
		Name: demoPlace.Name, Country: demoPlace.Country,
		Lat: demoPlace.Lat, Lon: demoPlace.Lon, TZ: demoPlace.TZ,
		LatPresent: true, LonPresent: true,
	}
	var tm tea.Model = NewModelWithClock(cfg, FrozenClock(demoTransit))
	step := func(msg tea.Msg) { tm, _ = tm.Update(msg) }
	step(tea.WindowSizeMsg{Width: w, Height: h})
	step(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{demoPlace})})
	step(ingest.StartupPerimeterReadMsg{
		LocationCandidate: location.Candidate{Source: "stored-location", Place: raw},
		Session:           v1(&store.SessionChart{ID: "constructed-demo", Name: "Constructed Demo", When: demoBirth, Place: demoPlace}, true, true, nil, nil),
		HasSession:        true,
	})
	frame := now.overlayFrame()
	m := tm.(Model)
	m.limitedViewShown = true // the first-frame notice is not what this fixture is about
	tm = m
	st := m.refresh[surfaceText]
	frame.RunID, frame.WantID = st.runningRunID, st.wantID
	frame.TransitTime = m.targetFloatingTime
	step(frame)
	return withSceneColumns(tm.(Model))
}

// writeDesignFrame compares a frame with its golden, or under -design writes
// it out for the operator to look at.
func writeDesignFrame(t *testing.T, name, frame string) {
	t.Helper()
	if *designFrames == "" {
		checkGolden(t, name, frame)
		return
	}
	if err := os.MkdirAll(*designFrames, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*designFrames, name+".txt"), []byte(frame+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// V4 — the freed top row: the main screen at both sizes.
func TestDesignTasteV4(t *testing.T) {
	skipWithoutAstrolog(t)
	forceColor(t)
	for _, size := range []struct {
		name string
		w, h int
	}{{"80x24", 80, 24}, {"144x40", 144, 40}} {
		m := designDemoModel(t, Config{Color: ColorAlways}, size.w, size.h)
		name := "design-no-status-" + size.name + "-color"
		if *designFrames != "" {
			name += "-" + *designLabel
		}
		writeDesignFrame(t, name, m.View())
	}
}
