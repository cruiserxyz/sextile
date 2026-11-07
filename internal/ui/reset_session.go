package ui

import (
	"errors"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// resetSession is `<leader> n`: the window starts a new, unnamed session. It
// is opening a named session in the other direction: the open charts, the
// selection and the saved house and zodiac choices go, the floating slot stays
// at its moment and place, and the window runs the general session again.
//
// The named session that was open is neither written nor trashed: its lock is
// let go and its file stays as last saved. Where another window took the
// general session in the meantime, this one empties the screen and keeps
// nothing from then on, as a second window does.
func resetSession(m Model) (tea.Model, tea.Cmd) {
	switch m.blocked(keys.BindingNewSession) {
	case "":
	case whySessionEmpty:
		return m.logNote("session", "new, unnamed"), nil
	default:
		return m, nil
	}
	// A reset has no way back, so the first press only says what it would
	// close and arms a second one (operator, 2026-11-07). The arm lasts as long
	// as that notice (the leader press hides it, its timer still ends the arm in
	// onYankFlash): any other notice or key (dispatchKey) ends it.
	if !(m.resetArmed != 0 && m.yank.gen == m.resetArmed) {
		n := m.slots.AnchoredCount()
		m, cmd := m.showNotice(keys.LeaderSpelling(m.leaderKey, nil) + " n again closes " + chartCount(n))
		m.resetArmed = m.yank.gen
		return m, cmd
	}
	m.resetArmed = 0
	named := m.named
	floating := m.slots.Floating()
	slots, err := store.NewSlots(floating.Time, floating.Location)
	if err != nil {
		return m.logNote("slots", "%v", err), nil
	}

	// Leaving a named session always tries the unnamed one again: the launch's
	// verdict (a second window) is stale by now, either way.
	owns, lockNote := true, ""
	if named.running() {
		m.releaseRunningLock()
		_, err := store.AcquireInstanceLock()
		var held *store.LockHeldError
		switch {
		case err == nil:
			m.cfg.SecondWindow = false
		case errors.As(err, &held):
			m.cfg.SecondWindow, owns = true, false
		default:
			m.cfg.SecondWindow, owns, lockNote = true, false, firstLine(err.Error())
		}
	}
	m.named = namedRun{}
	m.sessionKept = false
	// sessionForeign stays as the launch set it: the unnamed session this run
	// returns to is still another charts folder's, and nothing is written to it.
	if owns && named.running() && !m.sessionForeign {
		// A --session launch never read the general session, so its mark
		// could not judge it. Returning to it, this run meets the file once,
		// as a launch does (R6 of specs/session-other-folder-kept.md): one
		// that names another charts folder marks the run, which then writes
		// nothing to it, and a refused one is kept aside before anything
		// here can write it. A window that does not own the general session
		// keeps nothing and so reads nothing: the read can move a refused
		// file aside, and that is the owner's to do (review 2026-11-07, finding 1).
		s, ok, keep, err := store.LoadSessionKeepingRefused()
		m = m.reportSessionKeep(keep)
		if ok && err == nil && keep == (store.SessionKeep{}) && s.Folder != "" && s.Folder != store.FolderID(store.ChartsDir()) {
			m.sessionForeign = true
			m = m.logNote("session", "this run is saved only by name (%s)", keys.LeaderSpelling(m.leaderKey, []string{"o"}))
		}
	}
	m.sessionGen++ // a named session still being read is dropped when it arrives

	// A hard clear: Bodies and Aspects must not go on painting the former
	// chart while the recompute runs.
	m.chart = chartViewState{}
	m.shown = shownFrame{}
	m = m.suspendDetails()
	m.bodyCursorIdx = 0
	m = m.resetBodyTop()
	m.slots = slots
	m.cursorIdx = 0
	m.targetFloatingTime = floating.Time // a pending time step must not make the moment jump
	m.pendingRechecks = nil
	m.frames.records = nil
	m.frames.volatile, m.frames.volatileZ = nil, nil
	// The new session follows the defaults: no drawn set, kept or waiting.
	m.lens.Objects = m.defaultObjects()
	m.drawnKept, m.drawnRaw, m.fallbackSaid = nil, nil, ""
	m = m.forgetTabMemory()
	switch {
	case lockNote != "":
		m = m.logNote("session", "new, this window keeps nothing: %s", lockNote)
	case !owns:
		m = m.logNote("session", "new: another window owns the unnamed session, this one keeps nothing")
	case named.running():
		m = m.logNote("session", "new and unnamed, %s stays as it was saved", logIdent(named.name, m.cfg))
	default:
		m = m.logNote("session", "new, unnamed")
	}

	m.startup = startupComputing
	var recompute, wheel, tick tea.Cmd
	m, recompute = m.bumpIdentity(surfaceText)
	m, wheel = m.refreshWheelIfActive()
	m, tick = m.armNowTick()
	return m, tea.Batch(recompute, wheel, tick)
}

// sessionIsEmpty is a workspace with nothing for `, n` to put away: no chart,
// no selected floating slot or body, no house or zodiac choice.
func (m Model) sessionIsEmpty() bool {
	f := m.slots.AnchoredCount()
	return f == 0 && !m.sessionHoldsMarks(f) && m.lens.Objects.Baseline != recipe.Explicit &&
		len(m.drawnKept) == 0 && m.drawnRaw == nil &&
		len(m.frames.records) == 0 && len(m.frames.volatile) == 0 && len(m.frames.volatileZ) == 0
}

func chartCount(n int) string {
	if n == 1 {
		return "1 chart"
	}
	return strconv.Itoa(n) + " charts"
}
