package ui

import (
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// longTimersSleeping counts the long timers (the leader key's timeout, the copy
// flash, the Details hold) that are sleeping right now. Tests read it to tell a long timer, which
// they abandon, from real work, which they wait for however busy the host is
// (internal/ui pump). It costs one atomic add per timer.
var longTimersSleeping atomic.Int32

// longTimerStop is closed to end every long timer sleeping at that moment,
// and replaced by a fresh channel. Only the test pump closes it, when it
// abandons a round (abandonLongTimers); the app never does, so there an
// unabandoned timer fires after d as always.
var (
	longTimerStopMu sync.Mutex
	longTimerStop   = make(chan struct{})
)

func currentLongTimerStop() <-chan struct{} {
	longTimerStopMu.Lock()
	defer longTimerStopMu.Unlock()
	return longTimerStop
}

// abandonLongTimers ends the long timers sleeping now: each returns a nil
// message at once instead of sleeping out its duration.
func abandonLongTimers() {
	longTimerStopMu.Lock()
	defer longTimerStopMu.Unlock()
	close(longTimerStop)
	longTimerStop = make(chan struct{})
}

// longTimer is tea.Tick for a timer that runs half a second or more, or that
// tests decide the end of (the Details hold): the same message after d, and
// counted in longTimersSleeping while it sleeps. If it is abandoned
// (abandonLongTimers) it returns nil at once.
func longTimer(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		longTimersSleeping.Add(1)
		defer longTimersSleeping.Add(-1)
		stop := currentLongTimerStop()
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-t.C:
			return fn(time.Now())
		case <-stop:
			return nil
		}
	}
}
