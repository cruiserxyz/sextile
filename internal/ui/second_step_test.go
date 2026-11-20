package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/store"
)

// TestSecondStepMovesTheMomentOneSecond: s steps the floating slot's Time
// forward one second, S back one.
func TestSecondStepMovesTheMomentOneSecond(t *testing.T) {
	for _, tc := range []struct {
		k    rune
		want time.Duration
	}{{'s', time.Second}, {'S', -time.Second}} {
		t.Run(string(tc.k), func(t *testing.T) {
			m := newTestModel(t, Config{Accessible: true}, FrozenClock(testFrozenTimeUTC()))
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
			m = updated.(Model)
			slots, err := store.NewSlots(testFrozenTimeUTC(), testPlaceBerlin())
			if err != nil {
				t.Fatalf("NewSlots: %v", err)
			}
			m.slots = slots
			m.place = testPlaceBerlin()
			m.targetFloatingTime = slots.Floating().Time
			before := m.targetFloatingTime

			updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{tc.k}})
			got := updated.(Model)
			if want := before.Add(tc.want); !got.targetFloatingTime.Equal(want) {
				t.Errorf("%q moved the moment to %v; want %v", tc.k, got.targetFloatingTime, want)
			}
		})
	}
}
