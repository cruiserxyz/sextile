package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
)

// p/P on every tab step Now through the Moon's phases
// (specs/moon-observatory.md, slice P).

// phaseTarget is where p (dir +1) or P (dir -1) puts Now: the first phase
// landing after the target moment, or the last one landing before it, from
// Now's own void window in memory. ok is false when that window is not in
// memory or holds no phase that way.
func (m Model) phaseTarget(dir int) (time.Time, bool) {
	if m.slots == nil {
		return time.Time{}, false
	}
	floating := m.slots.Floating()
	_, key, ok := m.voidRequestForSlot(&floating)
	if !ok {
		return time.Time{}, false
	}
	res, ok := m.mem.voids.peek(key)
	if !ok {
		return time.Time{}, false
	}
	var best time.Time
	found := false
	for _, p := range res.Phases {
		at := phaseLanding(p)
		switch {
		case dir > 0 && at.After(m.targetFloatingTime):
			if !found || at.Before(best) {
				best, found = at, true
			}
		case dir < 0 && at.Before(m.targetFloatingTime):
			if !found || at.After(best) {
				best, found = at, true
			}
		}
	}
	return best, found
}

// stepPhase moves Now to the next (dir +1) or previous (dir -1) phase through
// the time-step path. With no phase to go to, Now stays and nothing is logged.
func (m Model) stepPhase(dir int) (Model, tea.Cmd) {
	at, ok := m.phaseTarget(dir)
	if !ok {
		return m, nil
	}
	m.details.pressedSet, m.details.followNow = false, true
	return m.aimNow(at).setFloatingTarget(at)
}

// aimNow makes instant the floating slot's target and detaches it from the
// clock. The slot's Time follows when the compute for the target commits.
func (m Model) aimNow(instant time.Time) Model {
	m.targetFloatingTime = instant
	m.floatingDetached = true
	return m
}

// phaseLanding is where Enter would put the moment on the phase's row.
func phaseLanding(p lookup.MoonPhase) time.Time {
	return landedFor(lookup.TimelineRow{Instant: p.Row.Instant, Kind: p.Row.Kind, Station: p.Row.Station})
}
