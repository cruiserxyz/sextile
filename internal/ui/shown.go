package ui

// shownFrame is what Bodies and Aspects keep painting while a new scene's
// frame is still on its way (the specification): the last committed
// results of the previous scene, tab strip included, so the screen never mixes
// two selections. It is ephemeral presentation state, never stored.
//
// Every text commit STAMPS the scene it drew (the Aspects tab strip, whether it
// compared, the cursor's column, the slot and body row it was cast for). A
// scene change HOLDS the committed results as they stand (holdShown) and the
// next text commit replaces the hold by stamping its own scene: that one
// commit carries positions, aspects, cross-aspects and columns, so the swap
// waits for nothing else. A hard reset zeroes the live chart, and with it the chart epoch holding() reads.
type shownFrame struct {
	// stamped says a text commit has described the scene on screen. A hold needs
	// it: with nothing committed there is nothing to keep painting.
	stamped bool
	// held says Bodies and Aspects draw chart below instead of the live results.
	held  bool
	chart chartViewState

	aspects    tabStrip   // the Aspects strip as the committed scene had it
	comparing  bool       // the committed scene compared
	focus      int        // the cursor's column in it
	count      int        // how many charts it showed (the panels' width)
	slot       int        // the slot index the committed frame was cast for
	bodyCursor int        // the Bodies row the committed frame was cast with
	columns    columnsFor // the shown set the committed scene named
}

// holding says Bodies and Aspects draw the held frame. A zeroed chart epoch
// means a hard reset (`m.chart = chartViewState{}`) happened after the hold, so
// there is nothing left to keep painting; the next text commit stamps anew.
func (m Model) holding() bool { return m.shown.held && m.chart.epoch != 0 }

// holdShown keeps the committed results painting until the next text commit.
// A second scene change before that commit keeps the first hold: the screen
// still shows the scene it last swapped to.
func (m Model) holdShown() Model {
	if m.holding() || !m.shown.stamped || m.chart.epoch == 0 {
		return m
	}
	m.shown.held = true
	m.shown.chart = m.chart
	return m
}

// stampShown records the scene the text commit just drew and releases any hold.
// It runs after the commit has set the floating slot's time, so the columns'
// identity is the one bodyColumnsForScene judges.
func (m Model) stampShown() Model {
	sc := m.deriveScene()
	tabs := m.panelTabs(PanelAspects)
	s := shownFrame{
		stamped:    true,
		aspects:    tabStrip{prefix: aspectsStripPrefix, tabs: tabs, active: activeTabIndex(tabs, m.tabChoice(PanelAspects))},
		comparing:  sc.mode == sceneComparing,
		focus:      sc.focus,
		count:      len(sc.shown),
		slot:       m.cursorIdx,
		bodyCursor: m.bodyCursorIdx,
	}
	if s.comparing && m.slots != nil {
		s.columns = m.shownColumns(sc, m.slots.Floating().Time)
	}
	m.shown = s
	return m
}

// shownChart is the committed results Bodies and Aspects paint: the held ones
// between a scene change and its frame, the live ones otherwise.
func (m Model) shownChart() chartViewState {
	if m.holding() {
		return m.shown.chart
	}
	return m.chart
}

// clearChartKeepingHold empties the live chart for a pick (an atlas place, a
// library chart) while Bodies and Aspects go on painting the previous frame
// (the specification). It holds first, then clears everything
// except the chart epoch, which holding() reads: the hold ends at the picked
// chart's text commit, which stamps anew. With nothing stamped yet (the first
// chart of a session) there is nothing to hold and the clear is a full reset.
func (m Model) clearChartKeepingHold() Model {
	m = m.holdShown()
	epoch := m.chart.epoch
	m.chart = chartViewState{}
	if m.shown.held {
		m.chart.epoch = epoch
	}
	return m
}

// paintedChart is shownChart with the entered row's marks for this paint, and
// the house display the operator chose. The marks come from the scene and are
// empty while a held frame paints, so they never land on a frame the entered
// row did not produce; the house display is the Model's, so a frame that is
// held or replaced still draws it.
func (m Model) paintedChart() chartViewState {
	state := m.shownChart()
	state.mark = m.enteredMark()
	state.forced = m.enteredStation()
	state.houseForm = m.houseDisplay
	return state
}
