package ui

import (
	"slices"
	"testing"

	"github.com/cruiserxyz/sextile/internal/store"
)

// wheelNamesModel is a workspace with two named charts and Now, the wheel
// open, stubs filed before the Model exists.
func wheelNamesModel(t *testing.T) Model {
	t.Helper()
	stubShowWheel(t)
	stubRenderWheelImage(t)
	stubRenderPairWheelImage(t)
	m := modelWithSlot(t)
	m.previewActive = true
	for _, name := range []string{"Kepler", "Goethe"} {
		if err := m.slots.AddAnchored(store.SessionChart{Name: name}, testFrozenTime().AddDate(-300, 0, 0), testPlaceBerlin()); err != nil {
			t.Fatalf("AddAnchored: %v", err)
		}
	}
	m.slots.SetSlotSelected(0, true)
	m.slots.SetSlotSelected(1, true)
	return m
}

// asked runs the render the wheel would be asked for now, and hands back its
// result as the coordinator receives it.
func askedWheel(t *testing.T, m Model) (Model, refreshResultMsg) {
	t.Helper()
	m, cmd := m.requestImageNow()
	if cmd == nil {
		t.Fatal("no render was started")
	}
	msg, ok := cmd().(refreshResultMsg)
	if !ok || msg.err != nil {
		t.Fatalf("the render did not yield a result: %+v", msg)
	}
	return m, msg
}

func wheelLines(m Model) []string {
	var out []string
	for _, l := range m.logLines {
		if len(l.text) >= 6 && l.text[:6] == "wheel:" {
			out = append(out, l.text)
		}
	}
	return out
}

// With the wheel open on a pair tab, the Log names the chart read and the
// base, each with its number.
func TestWheelLineNamesTheChartsItShows(t *testing.T) {
	m := wheelNamesModel(t)
	// Kepler (1) read in Goethe (2).
	m.cursorIdx = 0
	m.base = m.slots.All()[1]
	m, msg := askedWheel(t, m)
	m, cmd := m.onRefreshResult(msg)
	if cmd == nil {
		t.Fatal("the picture was not published")
	}
	m, _ = m.onWheelPublished(cmd().(wheelPublishedMsg))
	if got, want := wheelLines(m), []string{"wheel: ① Kepler in ② Goethe"}; !slices.Equal(got, want) {
		t.Errorf("pair tab: Log lines %q, want %q", got, want)
	}

}

// On a chart's own tab the line names that chart.
func TestWheelLineNamesTheChartOnItsOwnTab(t *testing.T) {
	own := wheelNamesModel(t)
	own.cursorIdx = 0
	own.base = nil
	own, msg := askedWheel(t, own)
	own, cmd := own.onRefreshResult(msg)
	if cmd == nil {
		t.Fatal("the picture was not published")
	}
	own, _ = own.onWheelPublished(cmd().(wheelPublishedMsg))
	if got, want := wheelLines(own), []string{"wheel: ① Kepler"}; !slices.Equal(got, want) {
		t.Errorf("own tab: Log lines %q, want %q", got, want)
	}
}

// A picture asked for Kepler and Goethe and published after the cursor moved to
// another pair is logged as Kepler and Goethe, and a second frame of the same
// two charts adds no second line.
func TestWheelLineNamesThePairItWasAskedFor(t *testing.T) {
	m := wheelNamesModel(t)
	m.cursorIdx = 0
	m.base = m.slots.All()[1]
	m, msg := askedWheel(t, m)
	m, cmd := m.onRefreshResult(msg)
	if cmd == nil {
		t.Fatal("the picture was not published")
	}
	// The cursor moves to another pair while the publish is in flight.
	m.cursorIdx = 1
	m.base = m.slots.All()[0]
	m, _ = m.onWheelPublished(cmd().(wheelPublishedMsg))
	if got, want := wheelLines(m), []string{"wheel: ① Kepler in ② Goethe"}; !slices.Equal(got, want) {
		t.Errorf("Log lines %q, want %q", got, want)
	}
	m.wheelPublishPending, m.wheelPublishShows = []byte("<svg/>"), "① Kepler in ② Goethe"
	m.wheelPublishKey, _ = m.wheelStillKey()
	m, cmd = m.kickWheelPublish()
	if cmd == nil {
		t.Fatal("the second frame was not published")
	}
	m, _ = m.onWheelPublished(cmd().(wheelPublishedMsg))
	if got := wheelLines(m); len(got) != 1 {
		t.Errorf("a second frame of the same charts logged %q, want the one line", got)
	}
}
