package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/store"
)

// TestMinuteStepMovesTargetOneMinute is C1 of the specification:
// `m` moves targetFloatingTime exactly one minute later and `M` exactly one
// minute earlier, each raising the text surface's wantID by exactly one.
// The target, not slots.Floating().Time, is the observable: advanceTime has
// moved the target and left the slot alone since an earlier review.
func TestMinuteStepMovesTargetOneMinute(t *testing.T) {
	cases := []struct {
		name string
		key  rune
		want time.Time
	}{
		{"m steps one minute forward", 'm', testFrozenTimeUTC().Add(time.Minute)},
		{"M steps one minute back", 'M', testFrozenTimeUTC().Add(-time.Minute)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTimeUTC()))
			{
				updated, _ := m.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
				m = updated.(Model)
			}
			slots, err := store.NewSlots(testFrozenTimeUTC(), testPlaceBerlin())
			if err != nil {
				t.Fatalf("NewSlots: %v", err)
			}
			m.slots = slots
			m.place = testPlaceBerlin()
			m.targetFloatingTime = slots.Floating().Time
			beforeWant := m.refresh[surfaceText].wantID

			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{tc.key}})
			got := updated.(Model)

			if cmd == nil {
				t.Fatalf("%q: expected a recompute dispatch, got nil cmd", tc.key)
			}
			if !got.targetFloatingTime.Equal(tc.want) {
				t.Errorf("%q: targetFloatingTime = %v, want %v",
					tc.key, got.targetFloatingTime.UTC(), tc.want.UTC())
			}
			if afterWant := got.refresh[surfaceText].wantID; afterWant != beforeWant+1 {
				t.Errorf("%q: wantID = %d, want %d (exactly one per press)",
					tc.key, afterWant, beforeWant+1)
			}
		})
	}
}

// TestMinuteStepActsWhenFloatingNotFocused is C2 as reversed by
// the specification rule 1 (operator 2026-10-13):
// with the cursor on the anchored slot, `m` and `M` still step the floating
// target a minute, and the cursor stays.
func TestMinuteStepActsWhenFloatingNotFocused(t *testing.T) {
	natalWhen := time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC)

	for _, k := range []rune{'m', 'M'} {
		t.Run(string(k), func(t *testing.T) {
			m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTimeUTC()))
			{
				updated, _ := m.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
				m = updated.(Model)
			}
			slots, err := store.NewSlots(testFrozenTimeUTC(), testPlaceBerlin())
			if err != nil {
				t.Fatalf("NewSlots: %v", err)
			}
			if err := addAnchor(slots, natalWhen, testPlaceAmsterdam()); err != nil {
				t.Fatalf("addAnchor: %v", err)
			}
			m.slots = slots
			m.place = testPlaceBerlin()
			m.cursorIdx = 0 // the anchored slot; floating is always last
			m.targetFloatingTime = slots.Floating().Time
			if m.focusedIsFloating() {
				t.Fatal("precondition: cursor should sit on the anchored slot")
			}
			before := m.targetFloatingTime

			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{k}})
			got := updated.(Model)

			// time-keys-everywhere rule 1: the step acts from the anchored
			// cursor, which stays where it is.
			want := before.Add(time.Minute)
			if k == 'M' {
				want = before.Add(-time.Minute)
			}
			if !got.targetFloatingTime.Equal(want) {
				t.Errorf("%q moved targetFloatingTime to %v; want %v", k, got.targetFloatingTime.UTC(), want.UTC())
			}
			if got.cursorIdx != m.cursorIdx {
				t.Errorf("%q moved the cursor to %d", k, got.cursorIdx)
			}
		})
	}
}

// TestMinuteStepCrossesTheDSTGap is C3: Berlin's 2026 spring-forward skips
// 02:00–02:59 local on 29 March, so one minute after 01:59 CET is 03:00 CEST.
// A duration step adds to the instant, which is what makes that come out
// right; a calendar step would land in the hole.
func TestMinuteStepCrossesTheDSTGap(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skipf("Europe/Berlin unavailable in this environment: %v", err)
	}
	start := time.Date(2026, 3, 29, 1, 59, 0, 0, berlin)
	if name, off := start.Zone(); off != 1*60*60 {
		t.Fatalf("precondition: 01:59 should be CET (+1h), got %s (%ds)", name, off)
	}

	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(start))
	slots, err := store.NewSlots(start, testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	m.slots = slots
	m.place = testPlaceBerlin()
	m.targetFloatingTime = start

	got, _ := m.advanceTime(unitMinute, +1)
	landed := got.targetFloatingTime.In(berlin)

	want := time.Date(2026, 3, 29, 3, 0, 0, 0, berlin)
	if !landed.Equal(want) {
		t.Errorf("one minute after 01:59 CET = %v, want %v (03:00 CEST)", landed, want)
	}
	if h, min := landed.Hour(), landed.Minute(); h == 2 {
		t.Errorf("landed on %02d:%02d, a local time that does not exist on this date", h, min)
	}
	if name, off := landed.Zone(); off != 2*60*60 {
		t.Errorf("landed in %s (%ds); want CEST (+2h)", name, off)
	}
}

// TestMinuteStepDeltaIsDurationOnly is C3a: the minute step contributes no
// calendar component at all, which is the mechanism C3 depends on.
func TestMinuteStepDeltaIsDurationOnly(t *testing.T) {
	for _, direction := range []int{+1, -1} {
		years, months, days, dur := stepDelta(unitMinute, direction)
		if years != 0 || months != 0 || days != 0 {
			t.Errorf("stepDelta(unitMinute, %+d) calendar part = (%d, %d, %d); want all zero",
				direction, years, months, days)
		}
		if want := time.Duration(direction) * time.Minute; dur != want {
			t.Errorf("stepDelta(unitMinute, %+d) duration = %v, want %v", direction, dur, want)
		}
	}
}

// TestBindingTableDeclaresTheMinuteStep is C4: the minute step is one
// help-visible binding claiming exactly m and M, its label names both keys,
// and the rendered help modal actually carries that label — a row that is
// declared but never drawn would be a key the operator cannot discover.
func TestBindingTableDeclaresTheMinuteStep(t *testing.T) {
	keys := defaultKeys()

	var matches []BindingEntry
	for _, e := range keys.mainBindings() {
		got := e.Binding.Keys()
		if len(got) == 2 && got[0] == "m" && got[1] == "M" {
			matches = append(matches, e)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("bindings claiming exactly [m M] = %d, want 1", len(matches))
	}
	entry := matches[0]
	if !entry.Help {
		t.Error("the minute step is not help-visible, so the operator cannot discover it")
	}

	h := entry.Binding.Help()
	if !strings.Contains(h.Key, "m") || !strings.Contains(h.Key, "M") {
		t.Errorf("help label key %q names only one direction; want both m and M", h.Key)
	}
	if !strings.Contains(h.Desc, "minute") {
		t.Errorf("help label description %q does not say what the key does", h.Desc)
	}

	rendered := helpListText(keys, defaultLeaderKey, defaultLeaderMap())
	if !strings.Contains(rendered, h.Desc) {
		t.Errorf("rendered help modal does not contain %q", h.Desc)
	}
}
