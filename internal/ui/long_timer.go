package ui

import (
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// longTimersSleeping counts the long timers (the leader key's timeout, the copy
// flash, the Details hold) that are sleeping right now. Tests read it to tell a long timer, which
// they abandon, from real work, which they wait for however busy the host is
// (internal/ui pump). It costs one atomic add per timer.
var longTimersSleeping atomic.Int32

// longTimer is tea.Tick for a timer that runs half a second or more, or that
// tests decide the end of (the Details hold): the same message after d, and
// counted in longTimersSleeping while it sleeps.
func longTimer(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		longTimersSleeping.Add(1)
		defer longTimersSleeping.Add(-1)
		time.Sleep(d)
		return fn(time.Now())
	}
}
