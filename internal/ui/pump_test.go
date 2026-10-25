package ui

import (
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// the bug report of 2026-10-10: pump abandoned slow commands and forgot them,
// so their goroutines finished during LATER tests and spawned through those
// tests' fake Astrolog; and real work slower than pump's 400 ms (a loaded
// host) was dropped. These pin both halves of the fix.

// An abandoned command has finished by the time its test's cleanups are done.
func TestPumpWaitsOutAbandonedCommands(t *testing.T) {
	var finished atomic.Bool
	// A long timer: since pump waits for any other slow work
	// (pump_load_test.go), a long timer is what it still abandons.
	slow := longTimer(600*time.Millisecond, func(time.Time) tea.Msg {
		finished.Store(true)
		return nil
	})
	t.Run("abandons", func(t *testing.T) {
		m := NewModelWithClock(Config{Color: ColorNever}, FrozenClock(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)))
		pump(t, m, slow)
		if finished.Load() {
			t.Fatal("fixture: the command finished inside pump's deadline; it must outlast it")
		}
	})
	if !finished.Load() {
		t.Error("the abandoned command was still running after its test ended: it can spawn during the next test")
	}
}

// pumpPatient applies a long timer slower than pump's deadline; pump drops it.
// (Slow work that is not a long timer, pump now waits for as well:
// pump_load_test.go.)
func TestPumpPatientAppliesSlowCommands(t *testing.T) {
	slow := longTimer(600*time.Millisecond, func(time.Time) tea.Msg {
		return tea.WindowSizeMsg{Width: 99, Height: 33}
	})
	fresh := func() Model {
		return NewModelWithClock(Config{Color: ColorNever}, FrozenClock(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)))
	}
	if m := pump(t, fresh(), slow); m.width == 99 {
		t.Fatal("fixture: pump applied a command slower than its deadline")
	}
	if m := pumpPatient(t, fresh(), slow); m.width != 99 {
		t.Errorf("pumpPatient dropped a command slower than 400 ms: width %d, want 99", m.width)
	}
}
