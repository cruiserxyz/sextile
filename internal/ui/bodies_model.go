package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/panel"
)

// bodiesView is what the Bodies panel and the layout read from the scene:
// how many charts it shows, and — while comparing — the columns computed for
// exactly that shown set.
type bodiesView struct {
	// shown is len(scene.shown); 0 when there is no scene yet.
	shown int
	// comparing is the scene's mode; ok says cols were computed for this
	// scene's shown set (bodyColumnsForScene).
	comparing bool
	ok        bool
	cols      []panel.BodyColumn
	// focus is the cursor's column index.
	focus int
	// colStart is the first column the window held last (Model.bodiesColStart).
	colStart int
	// held says the view is the previous scene's, kept until its frame lands
	// (shown.go): slot and cursor are then the slot index and Bodies row that
	// frame was cast with, not the live ones.
	held         bool
	slot, cursor int
	// top is the display row the list was last drawn from (Model.bodyTop).
	top int
	// bodies and aspects are where the two text panes stand against the key
	// each asks for (the view rule, subject.go): the zero value draws the
	// content bright, as a hand-built view does.
	bodies, aspects paneState
	// failed is the line a pane in paneFailed reads: the cast of the chart
	// under the cursor was refused.
	failed string
	// aspectsRefusal is the sentence the Aspects panel shows, on every tab, in
	// place of its lists: an aspect is renamed to letters no list can be read
	// with (aspect.AspectRegistry.Refusal). Empty when every list is read.
	aspectsRefusal string
}

func (v bodiesView) count() int { return max(v.shown, 1) }

func (m Model) bodiesView() bodiesView {
	sc := m.deriveScene()
	v := bodiesView{shown: len(sc.shown), comparing: sc.mode == sceneComparing, focus: sc.focus, colStart: m.bodiesColStart, top: m.bodyTop,
		bodies: m.textPaneState(PanelBodies), aspects: m.textPaneState(PanelAspects),
		aspectsRefusal: m.lens.AspectRegistry.Refusal()}
	if v.bodies == paneFailed || v.aspects == paneFailed {
		v.failed = failedCastLine(chartCaption(m.focusedSlot()))
	}
	if m.holding() {
		v.comparing, v.focus = m.shown.comparing, m.shown.focus
		v.shown = m.shown.count
		v.held, v.slot, v.cursor = true, m.shown.slot, m.shown.bodyCursor
	}
	if v.comparing {
		v.cols, v.ok = m.bodyColumnsForScene()
	}
	return v
}

// shownCount is how many charts the scene shows now (at least one).
func (m Model) shownCount() int {
	if m.holding() {
		// The panels keep their width with their content until the frame lands.
		return max(m.shown.count, 1)
	}
	return max(len(m.deriveScene().shown), 1)
}

// layoutInput is the four things that decide where a panel is drawn: the
// window, the panel in focus (the last left panel that had the keys while the
// Log, a popup or the console holds them), whether Details is open, and the
// charts the scene shows.
func (m Model) layoutInput() layoutInput {
	focus := m.focused
	if !isFocusable(focus) {
		focus = PanelChart
	}
	return layoutInput{width: m.width, height: m.height, focus: focus, detailsOpen: m.details.attached, charts: chartContentLines(m.slots), shown: m.shownCount()}
}

// layout is the screen's layout for the scene on screen; nil below the minimum.
func (m Model) layout() *screenLayout { return layoutFor(m.layoutInput()) }

// geometry is each panel's rectangle on screen, nil below the minimum.
func (m Model) geometry() map[PanelID]panelRect {
	if l := m.layout(); l != nil {
		return l.rects
	}
	return nil
}

// panelAt is the panel whose rectangle holds the cell (x, y), PanelNone on
// chrome.
func (m Model) panelAt(x, y int) PanelID {
	for id, r := range m.geometry() {
		if x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h {
			return id
		}
	}
	return PanelNone
}

// settleBodiesColStart records the comparing window for the focus's current
// place at the current width, so the next focus move travels from it.
func (m Model) settleBodiesColStart() Model {
	if m.slots == nil {
		return m
	}
	sc := m.deriveScene()
	if sc.mode != sceneComparing {
		return m
	}
	room := panel.ColumnRoom(m.geometry()[PanelBodies].w-2, m.focused == PanelBodies, markedRowShown(m.slots, m.cursorIdx, m.chart.positions, m.chart.cusps), m.cfg)
	m.bodiesColStart, _, _, _ = panel.ColumnWindow(len(sc.shown), sc.focus, room, m.bodiesColStart)
	return m
}
