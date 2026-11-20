package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/store"
)

// helpListText is everything the popup says, one line each: what it
// lists, independent of which window of it is on screen.
func helpListText(kmap keys.KeyMap, leader string, leaderBindings []keys.LeaderBinding) string {
	// A bare key table has no workspace, so this is the popup of a fresh one.
	lines := keybindingLines(Model{keys: kmap, leaderKey: leader, leaderBindings: leaderBindings})
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.Text
	}
	return strings.Join(out, "\n")
}

// helpFixture is a one-chart workspace at the given size, in the plain style.
func helpFixture(t *testing.T, w, h int) Model {
	t.Helper()
	m := escBase(t)
	m.cfg = Config{Color: ColorNever}
	m.width, m.height = w, h
	return m
}

// helpFixtureOneChart is helpFixture with a single chart, so the layout's
// dual-column threshold is the one-chart one.
func helpFixtureOneChart(t *testing.T, w, h int) Model {
	t.Helper()
	m := helpFixture(t, w, h)
	slots, err := store.NewSlots(testFrozenTime(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	m.slots, m.cursorIdx = slots, 0
	return m
}

// helpBindingTexts are the binding rows of the list, in order.
func helpBindingTexts(lines []kbLine) []string {
	var out []string
	for _, l := range lines {
		if l.Binding {
			out = append(out, l.Text)
		}
	}
	return out
}

// The Limited view line is not part of Keybindings at any size (the one-time
// notice elsewhere says it); nor is the Esc sentence.
func TestKeybindingsHasNoLimitedViewOrEscLine(t *testing.T) {
	for _, m := range []Model{helpFixtureOneChart(t, 80, 24), helpFixture(t, 80, 24), helpFixture(t, 144, 40)} {
		for _, l := range keybindingLines(m) {
			if strings.Contains(l.Text, "Limited view") || strings.HasPrefix(l.Text, "Esc ") {
				t.Errorf("the Help list says %q", l.Text)
			}
		}
	}
}
