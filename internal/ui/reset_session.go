package ui

import (
	"errors"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// startNewSession is the act of the Sessions list's New row: the window starts a
// new, unnamed session. It is opening a named session in the other direction:
// the open charts, the selection and the saved house and zodiac choices go, the
// floating slot stays at its moment and place, and the window runs the general
// session again.
//
// The named session that was open is written first (saveBeforeLeaving; the
// floating chart's moment is not in the file) and then its lock is let go; it
// is never trashed. A write that fails, or is refused because the file changed
// outside Sextile, leaves everything as it was: the window keeps the session
// and its lock and no new one starts. A session that is not written on leaving
// (savedWhenLeft) was asked about before this point. Where another window took the
// general session in the meantime, this one empties the screen and keeps
// nothing from then on, as a second window does. The caller has asked blocked
// and, for an unnamed session holding charts, the question.
func startNewSession(m Model) (tea.Model, tea.Cmd) { return startNewSessionAfter(m, false) }

// startNewSessionAfter is startNewSession; saved says the running session was
// written a moment ago (the question's Save as), so leaving does not write it again.
func startNewSessionAfter(m Model, saved bool) (tea.Model, tea.Cmd) {
	named := m.named
	if named.running() && !saved && m.savedWhenLeft() {
		var ok bool
		if m, ok = m.saveBeforeLeaving(); !ok {
			return m, nil
		}
	}
	floating := m.slots.Floating()
	slots, err := store.NewSlots(floating.Time, floating.Location)
	if err != nil {
		return m.logTrace("session", "%v", err).logNote("session", "a new session did not start"), nil
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
	m = m.forgetLastSession().clearLogPanel()
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
			m = m.logNote("session", "these charts are not kept unless the session gets a name")
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
	m.frames.volatile, m.frames.volatileZ, m.frames.volatileN = nil, nil, nil
	// The new session follows the defaults: no drawn set, kept or waiting.
	m.lens.Objects = m.defaultObjects()
	m.drawnKept, m.drawnRaw, m.fallbackSaid = nil, nil, ""
	m = m.forgetTabMemory()
	m.recentActions = nil
	switch {
	case lockNote != "":
		m = m.logTrace("session", "the session lock: %s", lockNote).logNote("session", "new session, this window keeps nothing after quit")
	case !owns:
		m = m.logNote("session", "new session, another window holds the unnamed session, this one keeps nothing")
	case named.running():
		m = m.logNote("session", "new session with no name, %s stays as saved", logIdent(named.name, m.cfg))
	default:
		m = m.logNote("session", "new session with no name")
	}

	m.startup = startupComputing
	var recompute, wheel, tick tea.Cmd
	m, recompute = m.bumpIdentity(surfaceText)
	m, wheel = m.refreshWheelIfActive()
	m, tick = m.armNowTick()
	return m, tea.Batch(recompute, wheel, tick)
}

// sessionIsEmpty is a workspace with nothing for a new session to put away: no chart,
// no selected floating slot or body, no house or zodiac choice, no recent command.
func (m Model) sessionIsEmpty() bool {
	f := m.slots.AnchoredCount()
	return f == 0 && !m.sessionHoldsMarks(f) && m.lens.Objects.Baseline != recipe.Explicit &&
		len(m.recentActions) == 0 &&
		len(m.drawnKept) == 0 && m.drawnRaw == nil &&
		len(m.frames.records) == 0 && len(m.frames.volatile) == 0 && len(m.frames.volatileZ) == 0 && len(m.frames.volatileN) == 0
}

func chartCount(n int) string {
	if n == 1 {
		return "1 chart"
	}
	return strconv.Itoa(n) + " charts"
}
