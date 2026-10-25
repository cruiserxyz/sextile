package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

func rowSelectionModel(t *testing.T) Model {
	t.Helper()
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTime()))
	slots, err := store.NewSlots(testFrozenTime(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	anchorWhen := time.Date(1990, 1, 1, 14, 0, 0, 0, time.UTC)
	if err := addAnchor(slots, anchorWhen, testPlaceAmsterdam()); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}
	m.slots = slots
	m.cursorIdx = 1
	m.place = testPlaceBerlin()
	m.chart.positions = []aspect.Position{
		{Body: aspect.Midheaven, Longitude: 100},
		{Body: aspect.Sun, Longitude: 35},
		{Body: aspect.Moon, Longitude: 136},
		{Body: aspect.Mars, Longitude: 210},
		{Body: aspect.Jupiter, Longitude: 240},
		{Body: aspect.Saturn, Longitude: 270},
		{Body: aspect.Uranus, Longitude: 300},
		{Body: aspect.Neptune, Longitude: 330},
		{Body: aspect.Pluto, Longitude: 15},
		{Body: aspect.Ascendant, Longitude: 10},
		{Body: aspect.Descendant, Longitude: 190},
		{Body: aspect.ImumCoeli, Longitude: 280},
	}
	return m
}

func TestBodiesOwnsArrowCursorAndSpaceSelection(t *testing.T) {
	m := rowSelectionModel(t)
	m.focused = PanelBodies

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	if m.bodyCursorIdx != 1 {
		t.Fatalf("body cursor after Down = %d, want 1", m.bodyCursorIdx)
	}
	if m.cursorIdx != 1 {
		t.Fatalf("slot cursor moved while Bodies owned focus: %d", m.cursorIdx)
	}
	body, ok := bodyAtCursor(m.chart.positions, m.chart.cusps, m.bodyCursorIdx)
	if !ok || body != aspect.Moon {
		t.Fatalf("body cursor resolved to (%v, %v), want Moon", body, ok)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	if !m.slots.BodySelected(1, aspect.Moon) {
		t.Error("Space did not persist floating-slot Moon selection in store")
	}
	if m.slots.BodySelected(0, aspect.Moon) {
		t.Error("floating-slot body selection leaked into natal slot")
	}
}

func TestChartRowMoveClearsStaleBodiesUntilFocusedSlotCommits(t *testing.T) {
	m := rowSelectionModel(t)
	m.focused = PanelChart
	m.bodyCursorIdx = 5

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if cmd == nil {
		t.Error("Chart row move did not request focused-slot recompute")
	}
	if m.cursorIdx != 0 {
		t.Fatalf("slot cursor after Up = %d, want anchored index 0", m.cursorIdx)
	}
	if len(m.chart.positions) != 0 {
		t.Error("previous floating-slot bodies remained selectable under anchored focus")
	}
	if m.bodyCursorIdx != 0 {
		t.Errorf("body cursor after slot change = %d, want reset 0", m.bodyCursorIdx)
	}
}

// Changed by selected-charts-compare C1-C3: what the screens compare is read
// from the cursor AND the selected set (comparing), not from the baseline-and-
// floating pair alone. Each step below also asserts comparing(): the cursor on
// the floating slot compares only while that slot is selected with the baseline.
func TestChartSpaceAndTDeriveComparisonFromSlotSelection(t *testing.T) {
	m := rowSelectionModel(t)
	m.focused = PanelChart
	beforePolicy := m.lens.Aspects
	beforeChooser := ticked(m.aspectModal)
	if !m.slots.ComparisonSelected() || !m.comparing() {
		t.Fatal("new anchor should start with selected comparison pair, the cursor on a member")
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	if cmd == nil {
		t.Error("crossing out of comparison did not request a recompute")
	}
	if m.slots.ComparisonSelected() || m.comparing() || m.slots.SlotSelected(1) {
		t.Error("Space did not deselect focused floating slot and disable comparison")
	}
	if !sameSelection(m.lens.Aspects, beforePolicy) || !sameInts(ticked(m.aspectModal), beforeChooser) {
		t.Error("main-scope Space reached the closed aspect chooser or changed its policy")
	}

	// `t` is retired (the specification C4): Space on the floating slot
	// selects the pair again.
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	if cmd == nil || !m.slots.ComparisonSelected() || !m.comparing() || !m.slots.SlotSelected(0) || !m.slots.SlotSelected(1) {
		t.Error("t did not select the anchored+floating pair through store")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	m.cursorIdx = 0
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	if m.slots.ComparisonSelected() || m.comparing() || m.slots.SlotSelected(0) || m.slots.SlotSelected(1) {
		t.Error("Space on each member did not deselect the comparison pair")
	}
}

func TestBodiesCursorScrollsWithTheAnglesNotPinned(t *testing.T) {
	m := rowSelectionModel(t)
	m.bodyCursorIdx = 7 // Pluto in planets-then-angles order
	m.slots.ToggleBodySelection(1, aspect.Pluto)

	// With colours off the cursor row is reverse video and opens with the
	// cursor column, then the selection column.
	focused, _, _ := renderBodiesBody(m.chart, m.slots, 1, m.bodyCursorIdx, 48, 6, true, Config{Accessible: true})
	if !strings.Contains(focused, cursorRowOff+rowCursorASCII+rowSelectionASCII+" ♇ Pluto") {
		t.Fatalf("focused selected Pluto row lacks the cursor row, cursor marker and selection marker:\n%s", focused)
	}
	// Nothing is pinned (the specification sentence 5, reversing
	// the pinned angle block): with the cursor on Pluto the window holds
	// planets only, and the angles come into view as the cursor walks down.
	for _, angle := range []string{"Asc", "MC", "Dsc", "IC"} {
		if strings.Contains(focused, angle) {
			t.Errorf("angle %s is pinned into a window scrolled to the planets:\n%s", angle, focused)
		}
	}
	last, _, _ := renderBodiesBody(m.chart, m.slots, 1, 11, 48, 6, true, Config{Accessible: true})
	for _, angle := range []string{"MC"} {
		if !strings.Contains(last, angle) {
			t.Errorf("the cursor on the last row does not bring %s into the window:\n%s", angle, last)
		}
	}
	for i, line := range strings.Split(focused, "\n") {
		if got := ansi.StringWidth(line); got > 48 {
			t.Errorf("line %d width = %d, exceeds 48: %q", i, got, line)
		}
	}

	unfocused, _, _ := renderBodiesBody(m.chart, m.slots, 1, m.bodyCursorIdx, 48, 6, false, Config{Accessible: true})
	if strings.Contains(unfocused, rowCursorASCII) {
		t.Fatalf("Bodies retained cursor after losing panel focus:\n%s", unfocused)
	}
	// The cursor row (Pluto) stays bold without the keyboard, and nothing else
	// does: no reverse, no other bold row (the specification).
	if !strings.Contains(unfocused, sgrBold+rowSelectionASCII+" ♇ Pluto") || strings.Count(unfocused, sgrBold) != 1 || strings.Contains(unfocused, sgrReverse) {
		t.Fatalf("Bodies lost the persistent marker, or its cursor row is not the one bold row, after losing panel focus:\n%s", unfocused)
	}
	for _, line := range strings.Split(unfocused, "\n") {
		plain := astrologstyle.StripSGR(line)
		if strings.Contains(plain, "Sun") && !strings.HasPrefix(plain, rowMarkerBlank+"☉ Sun") {
			t.Fatalf("unselected row did not reserve the active selection column: %q", plain)
		}
	}
	m.slots.ToggleBodySelection(1, aspect.Pluto)
	cleared, _, _ := renderBodiesBody(m.chart, m.slots, 1, m.bodyCursorIdx, 48, 6, false, Config{Accessible: true})
	for _, line := range strings.Split(cleared, "\n") {
		plain := astrologstyle.StripSGR(line)
		if strings.Contains(plain, "Pluto") && !strings.HasPrefix(plain, "♇ Pluto") {
			t.Fatalf("empty selection retained marker indentation: %q", plain)
		}
	}
}

// the specification C6: the cursor row keeps every cell's own
// foreground. The band sets only bold and a background, and is written again
// after the reset that ends an Astrolog span, so the text after it is still
// on the band.
func TestCursorRowKeepsAstrologColour(t *testing.T) {
	forceColor(t)
	cfg := Config{Color: ColorAlways}
	band := cursorBandStart(cfg)
	got := renderRowState("\x1b[91mMars\x1b[0m", 8, true, cfg)
	want := band + "\x1b[91m" + band + "Mars\x1b[0m" + band + "    " + sgrReset
	if got != want {
		t.Errorf("cursor row = %q, want %q", got, want)
	}
}

// C11a: selection is a marker, never a treatment of the row.
func TestSelectionAddsNothingToTheRow(t *testing.T) {
	row := "Mars \x1b[91mred\x1b[0m end"
	for _, cfg := range []Config{{Color: ColorAlways}, {Color: ColorNever}, {Accessible: true}} {
		if got := renderRowState(row, 40, false, cfg); got != row {
			t.Errorf("%+v: a row off the cursor = %q, want it untouched", cfg, got)
		}
	}
}
