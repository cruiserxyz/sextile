package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// One reconciler (specs/details-follows-its-tab.md C9, C11).
//
// Every pane that shows a computed result has a wanted key, a pure function of
// the Model, and a shown key stored with its content. After EVERY Update the
// reconciler compares the two for each rendered pane, and when a pane is owed
// a result nothing has started, it starts one through the surface's own row
// (want, run, commit floor, accept, latest-wins stay as they are). A pane is
// owed nothing while its shown key is its wanted key, while its wanted key is
// the one whose accepted run failed, while a run for that key is in flight or
// scheduled, or while a result it is built from is still on its way. So no
// entry point can leave a pane waiting for ever by forgetting to ask, and a
// failure is not retried in a loop.
//
// The entry points keep their own requests where they start a run sooner than
// the reconciler would (a text commit asks for its report at once; a key that
// moves the Bodies cursor does not). That is a speed, not the guarantee.
//
// Timeline and template have always been reconciled this way (syncTimeline and
// syncTemplate are idempotent per key); they are called from here so that the
// four surfaces are one place.
func (m Model) reconcile() (Model, tea.Cmd) {
	var cmds []tea.Cmd
	var c tea.Cmd
	m, c = m.reconcileText()
	cmds = append(cmds, c)
	m, c = m.reconcileDetailsTabs()
	cmds = append(cmds, c)
	var live []tea.Cmd
	for _, c := range cmds {
		if c != nil {
			live = append(live, c)
		}
	}
	switch len(live) {
	case 0:
		return m, nil
	case 1:
		return m, live[0]
	}
	return m, tea.Batch(live...)
}

// reconcileDetailsTabs is the Details panel's turn: the report tab, then the
// template and Timeline tabs through their idempotent observers.
func (m Model) reconcileDetailsTabs() (Model, tea.Cmd) {
	var cmds []tea.Cmd
	var c tea.Cmd
	m, c = m.reconcileDetails()
	cmds = append(cmds, c)
	m, c = m.syncTemplate()
	cmds = append(cmds, c)
	m, c = m.syncTimeline()
	cmds = append(cmds, c)
	return m, tea.Batch(cmds...)
}

// startupPreviews says a chooser that previews on the panels is open: its
// toggles recompute the text frame behind it, so the panels and Details go on
// following their keys while it is.
func (m Model) startupPreviews() bool {
	return m.startup == startupAspectOpen || m.startup == startupBodyOpen
}

// detailsLive says Details may ask for a result: the workspace is ready, or a
// previewing chooser is open over a ready workspace. It gates asking only: what
// Details is about follows the cursor, the focus and the tab, popup or not.
func (m Model) detailsLive() bool {
	return m.startup == startupReady || m.startupPreviews()
}

// textOutstanding says a text frame is on its way: a want is registered whose
// result has not committed (a run in flight, or the debounce tick that starts
// it). The commit that lands it asks for what waits on it.
func (m Model) textOutstanding() bool {
	return m.refresh[surfaceText].wantID != m.refresh[surfaceText].lastCommitWantID
}

// reconcileText is the text frame's turn (Bodies and Aspects share one). Before
// the first frame has committed the startup and pick flows own the request.
func (m Model) reconcileText() (Model, tea.Cmd) {
	if m.slots == nil || !m.shown.stamped || m.cantStart.any() || !m.detailsLive() {
		return m, nil
	}
	want, ok := m.textKeyFor()
	if !ok {
		return m, nil
	}
	if m.textRefusedNow() {
		return m, nil // refused: not retried
	}
	if shown, ok := m.shownPaneKey(PanelBodies); ok && shown.text == want {
		return m, nil
	}
	// Asked for this key and on its way (a run in flight, or the tick that
	// starts it): the commit decides. Asked for another key, or not at all, an
	// entry point forgot: the want is registered now, and an older run that is
	// still going finishes first as the burst's leading frame.
	if m.textAskedOK && m.textAsked == want && (m.textOutstanding() || m.refresh[surfaceText].cancel != nil) {
		return m, nil
	}
	return m.scheduleText()
}

// scheduleText is the one place the reconciler asks for a text frame: the text
// row's own want, leading when idle, polite when a run is out. noteWant draws a
// frame remembered for the key instead of running it.
func (m Model) scheduleText() (Model, tea.Cmd) {
	return m.noteWant(surfaceText, true)
}

// reconcileDetails is the Details report tab's turn. The Timeline and template
// tabs are reconciled by their own observers; a tab that is not the active one
// is owed nothing.
func (m Model) reconcileDetails() (Model, tea.Cmd) {
	if m.slots == nil || !m.detailsVisible() || m.reportTabHidden() || m.cantStart.any() || !m.detailsLive() {
		return m, nil
	}
	w := m.detailsWanted()
	if !w.has {
		return m, nil
	}
	if w.key.kind == reportFrame {
		return m.reconcileAspectsCopy(w), nil
	}
	// A failure belongs to the key it was for: a wanted key that has moved on
	// leaves it behind.
	if m.details.failed.set && m.details.failed.key != w.key.raw {
		m.details.failed.set = false
	}
	switch {
	case m.details.keyed && m.details.key == w.key:
		return m, nil // shown
	case m.details.failed.set:
		return m, nil // failed: not retried
	case m.textOutstanding() || !m.chart.settled || m.textRefusedNow():
		// The report is cast over the frame beside it: it waits for the frame,
		// and the frame's commit asks for it. A frame that was refused owes no
		// report.
		return m, nil
	case m.reportInMemory():
		// Computed before for this very key: drawn now, over whatever is in
		// flight or scheduled for another (C13).
		if m.unresolvedFor(m.focusedSlot()) != nil {
			return m, nil
		}
		nm, _ := m.adoptReportFromMemory()
		return nm, nil
	case m.refresh[surfaceDetails].cancel != nil && m.details.runKey == w.key.raw:
		return m, nil // in flight
	case m.details.timerArmed:
		return m, nil // scheduled
	case m.unresolvedFor(m.focusedSlot()) != nil:
		return m, nil // casts nothing, as the text, the wheel and Details refuse
	}
	return m.scheduleDetails()
}

// scheduleDetails is the one place the reconciler asks for a report run: the
// Details row's own debounce and fences. A memory hit never gets here
// (reconcileDetails draws it first). The first key of a run of equal keys
// starts the run at once, a repeat inside the run rests for the debounce (C14).
func (m Model) scheduleDetails() (Model, tea.Cmd) {
	return m.deferDetailsWith(m.keyFirst)
}

// reconcileAspectsCopy fills Details with the Aspects frame's own report once
// that frame is the one asked for: a copy, no Astrolog process.
func (m Model) reconcileAspectsCopy(w detailsWant) Model {
	if shown, ok := m.shownPaneKey(PanelAspects); !ok || shown != w.key.frame {
		return m // the frame is still on its way
	}
	if m.details.keyed && m.details.key == w.key {
		return m
	}
	if _, _, key, ok := m.aspectsDetailsFromFrame(); !ok || key != w.key {
		return m
	}
	return m.applyAspectsDetails()
}
