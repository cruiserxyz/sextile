package ui

import (
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/store"

	"github.com/charmbracelet/bubbles/key"
)

// helpListText is everything the help list says, one line each: what it
// lists, independent of which window of it is on screen.
func helpListText(keys keyMap, leader string, leaderBindings []leaderBinding) string {
	// A bare key table has no workspace, so this is the popup of a fresh one.
	lines := Model{keys: keys, leaderKey: leader, leaderBindings: leaderBindings}.helpLines()
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.text
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
func helpBindingTexts(lines []helpLine) []string {
	var out []string
	for _, l := range lines {
		if l.binding {
			out = append(out, l.text)
		}
	}
	return out
}

// The Keys popup's binding rows follow the order that answers "how do I move,
// open, inspect, go back, quit" first (help order).
func TestHelpListFollowsTheStartHereOrder(t *testing.T) {
	m := helpFixture(t, 144, 40)
	k := m.keys
	order := []key.Binding{
		k.NextView, k.SlotCycleUp, k.SlotCycleDown, k.Attach, k.OpenCharts, k.OpenAtlas, k.ForceQuit,
		k.PrevView, k.FocusChart, k.FocusBodies, k.FocusAspects, k.Select, k.SlotMoveUp, k.SlotMoveDown, k.SlotDelete, k.PageUp, k.PageDown, k.CursorHome, k.CursorEnd,
		k.TimeStepYear, k.TimeStepMonth, k.TimeStepWeek, k.TimeStepDay, k.TimeStepHour,
		k.TimeStepMinute, k.JumpNow, k.ToggleBack,
		k.DetailsTab, k.TabPrev, k.TabNext, k.ScrollUp, k.ScrollDown, k.Bodies, k.Aspects, k.Houses, k.Zodiac,
		k.SaveChart, k.RenderWheel,
	}
	rows := helpBindingTexts(m.helpLines())
	if len(rows) < len(order) {
		t.Fatalf("the list has %d binding rows, want at least %d", len(rows), len(order))
	}
	for i, b := range order {
		if want := b.Help().Desc + " "; !strings.HasPrefix(rows[i], want) {
			t.Errorf("binding row %d is %q, want it to start %q", i+1, rows[i], want)
		}
	}
}

// The list opens with a heading, not a binding row,
// and ends with the zodiac note, unchanged (heading and trailing note).
func TestHelpListHeadingNoteAndTrailingZodiacNote(t *testing.T) {
	m := helpFixture(t, 144, 40)
	lines := m.helpLines()
	if lines[0].text != "Start here" || lines[0].binding {
		t.Errorf("line 0 = %+v, want the non-binding heading `Start here`", lines[0])
	}
	if !lines[1].binding {
		t.Errorf("line 1 = %+v, want the first binding row right under the heading", lines[1])
	}
	tail := lines[len(lines)-len(helpZodiacNote):]
	want := []string{
		"Zodiac: Astrolog's ayanamsas are fixed offsets from",
		"Fagan-Bradley. Each chart is precessed to its own",
		"date, so a sidereal comparison's pair differs from the",
		"tropical pair by the precession between the two dates.",
	}
	for i, l := range tail {
		if l.text != want[i] || l.binding {
			t.Errorf("zodiac line %d = %+v, want %q as a note", i, l, want[i])
		}
	}
	lastBinding := 0
	for i, l := range lines {
		if l.binding {
			lastBinding = i
		}
	}
	if lastBinding >= len(lines)-len(helpZodiacNote) {
		t.Errorf("a binding row (line %d) sits inside or after the zodiac note", lastBinding)
	}
}

// The Limited view line is not part of Help at any size (the one-time notice
// elsewhere says it); nor is the Esc sentence.
func TestHelpListHasNoLimitedViewOrEscLine(t *testing.T) {
	for _, m := range []Model{helpFixtureOneChart(t, 80, 24), helpFixture(t, 80, 24), helpFixture(t, 144, 40)} {
		for _, l := range m.helpLines() {
			if strings.Contains(l.text, "Limited view") || strings.HasPrefix(l.text, "Esc ") {
				t.Errorf("the Help list says %q", l.text)
			}
		}
	}
}
