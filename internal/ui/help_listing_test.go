package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/store"
)

// helpListText is everything the help list says, one line each: what it
// lists, independent of which window of it is on screen.
func helpListText(kmap keys.KeyMap, leader string, leaderBindings []keys.LeaderBinding) string {
	// A bare key table has no workspace, so this is the popup of a fresh one.
	lines := Model{keys: kmap, leaderKey: leader, leaderBindings: leaderBindings}.helpLines()
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
func helpBindingTexts(lines []keys.HelpLine) []string {
	var out []string
	for _, l := range lines {
		if l.Binding {
			out = append(out, l.Text)
		}
	}
	return out
}

// The Keys popup's binding rows follow the order that answers "how do I move,
// open, inspect, go back, quit" first (help order).
func TestHelpListFollowsTheStartHereOrder(t *testing.T) {
	m := helpFixture(t, 144, 40)
	// One row per pair: the first key of each carries it, the others are folded.
	order := []keys.BindingID{
		keys.BindingNextView, keys.BindingSlotCycleUp, keys.BindingAttach, keys.BindingOpenCharts, keys.BindingOpenAtlas, keys.BindingActions, keys.BindingForceQuit,
		keys.BindingFocusChart, keys.BindingSelect, keys.BindingSlotMoveUp, keys.BindingSlotMoveDown, keys.BindingSlotDelete, keys.BindingPageUp, keys.BindingPageHome, keys.BindingPageEnd,
		keys.BindingTimeStepYear, keys.BindingTimeStepMonth, keys.BindingTimeStepWeek, keys.BindingTimeStepDay, keys.BindingTimeStepHour,
		keys.BindingTimeStepMinute, keys.BindingJumpNow, keys.BindingToggleBack,
		keys.BindingDetailsTab, keys.BindingTabPrev, keys.BindingScrollUp, keys.BindingBodies, keys.BindingAspects, keys.BindingHouses, keys.BindingZodiac,
		keys.BindingSaveChart, keys.BindingRenderWheel,
	}
	byID := map[keys.BindingID]keys.BindingEntry{}
	for _, e := range mainBindings(m.keys) {
		byID[e.ID] = e
	}
	rows := helpBindingTexts(m.helpLines())
	if len(rows) < len(order) {
		t.Fatalf("the list has %d binding rows, want at least %d", len(rows), len(order))
	}
	for i, id := range order {
		if want := m.titleOf(byID[id]) + " "; !strings.HasPrefix(rows[i], want) {
			t.Errorf("binding row %d is %q, want it to start %q", i+1, rows[i], want)
		}
	}
}

// The list opens with a heading, not a binding row,
// and ends with the zodiac note, unchanged (heading and trailing note).
func TestHelpListHeadingNoteAndTrailingZodiacNote(t *testing.T) {
	m := helpFixture(t, 144, 40)
	lines := m.helpLines()
	if lines[0].Text != "Start here" || lines[0].Binding {
		t.Errorf("line 0 = %+v, want the non-binding heading `Start here`", lines[0])
	}
	if !lines[1].Binding {
		t.Errorf("line 1 = %+v, want the first binding row right under the heading", lines[1])
	}
	tail := lines[len(lines)-len(keys.HelpZodiacNote):]
	want := []string{
		"Zodiac: Astrolog's ayanamsas are fixed offsets from",
		"Fagan-Bradley. Each chart is precessed to its own",
		"date, so a sidereal comparison's pair differs from the",
		"tropical pair by the precession between the two dates.",
	}
	for i, l := range tail {
		if l.Text != want[i] || l.Binding {
			t.Errorf("zodiac line %d = %+v, want %q as a note", i, l, want[i])
		}
	}
	lastBinding := 0
	for i, l := range lines {
		if l.Binding {
			lastBinding = i
		}
	}
	if lastBinding >= len(lines)-len(keys.HelpZodiacNote) {
		t.Errorf("a binding row (line %d) sits inside or after the zodiac note", lastBinding)
	}
}

// The Limited view line is not part of Help at any size (the one-time notice
// elsewhere says it); nor is the Esc sentence.
func TestHelpListHasNoLimitedViewOrEscLine(t *testing.T) {
	for _, m := range []Model{helpFixtureOneChart(t, 80, 24), helpFixture(t, 80, 24), helpFixture(t, 144, 40)} {
		for _, l := range m.helpLines() {
			if strings.Contains(l.Text, "Limited view") || strings.HasPrefix(l.Text, "Esc ") {
				t.Errorf("the Help list says %q", l.Text)
			}
		}
	}
}
