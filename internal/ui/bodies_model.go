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
}

func (v bodiesView) count() int { return max(v.shown, 1) }

func (m Model) bodiesView() bodiesView {
	sc := m.deriveScene()
	v := bodiesView{shown: len(sc.shown), comparing: sc.mode == sceneComparing, focus: sc.focus, colStart: m.bodiesColStart, top: m.bodyTop,
		bodies: m.textPaneState(PanelBodies), aspects: m.textPaneState(PanelAspects)}
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

// geometry is the layout of the terminal for the scene on screen.
func (m Model) geometry() map[PanelID]panelRect {
	return layoutGeometry(m.width, m.height, chartContentLines(m.slots), m.shownCount())
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
