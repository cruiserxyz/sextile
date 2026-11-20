package ui

import (
	"fmt"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"github.com/cruiserxyz/sextile/internal/ui/term"
	"maps"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/templates"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
)

// Geometry constants mined from legacy_sextile UI-surface composition.
const (
	// MinPanelWidth is the narrowest terminal any layout draws on (the
	// tooSmallView gate); it is no longer the left column's width.
	MinPanelWidth = 66
	// leftBaseWidth is the left column with one chart shown; it grows by
	// leftColumnStep for every further chart of the scene (the specification
	// C5), and Details needs detailsMinWidth beside it or it goes first. At
	// 132 columns one chart leaves Details 65 cells inside its border: the
	// Chart panel's table keeps a place of coordinates whole, and Astrolog's
	// wider reports are cut at Details' edge.
	leftBaseWidth   = 65
	leftColumnStep  = 12
	detailsMinWidth = 67
	AspectsHeight   = 6
	LogHeight       = 6
	KeyBarHeight    = 1
	// MinTerminalHeight is the shortest window any layout draws on: four title
	// lines, a panel of three rows, and the key bar. No row is reserved for a
	// status bar (the specification C15). Below it tooSmallView is drawn.
	MinTerminalHeight = 15
	MaxLogLines       = 200
)

// The Chart panel grows with the list, and stops (operator, 2026-10-04: "the
// charts panel may increase to max 7 rows and adapt the bodies panel height
// accordingly. >7 introduces scrolling" and "max a lower resolution to 5/7"):
// at most chartRowsTall rows — six charts and now — on a terminal at least
// chartTallTerminal rows tall, chartRowsShort on a shorter one. Past that it
// windows around the cursor (chartWindow). Bodies takes whatever height is
// left; it scrolls and remembers its row per chart. A workspace of two charts
// and now fits whole at either size, as it always has.
const (
	chartTallTerminal = 40
	chartRowsTall     = 7
	chartRowsShort    = 5
)

// chartPanelLines is how many slot rows the Chart panel gets for a workspace of
// `lines` slots on a terminal `height` rows tall.
func chartPanelLines(height, lines int) int {
	limit := chartRowsShort
	if height >= chartTallTerminal {
		limit = chartRowsTall
	}
	return min(lines, limit)
}

// chartContentLines is the Chart panel's content height in lines — one per
// slot in the workspace; layoutFor holds it to chartPanelLines. Returns 1 when
// the workspace is not yet built (cold start, pre-resolution) so the panel
// still has a body to render the placeholder line.
func chartContentLines(slots *store.Slots) int {
	if slots == nil {
		return 1
	}
	if n := len(slots.All()); n > 0 {
		return n
	}
	return 1
}

// leftColumnWidth is the two-column layout's left width for `shown` charts
// (one when alone).
func leftColumnWidth(shown int) int {
	return leftBaseWidth + leftColumnStep*(max(shown, 1)-1)
}

// dualColumnMinWidth is the narrowest terminal that still shows Details beside
// the left column for `shown` charts.
func dualColumnMinWidth(shown int) int {
	return leftColumnWidth(shown) + detailsMinWidth
}

// panelRect is a single panel's rectangle in cell coordinates.
type panelRect struct {
	x, y, w, h int
}

// compose draws the screen from the layout: the left column's panels in
// order (a folded one as its title line), Details right below the panel it
// belongs to or on the right beside them, and the Log on the right when there
// is room for it.
//
// Exactly one frame is drawn focused: the one that owns the keyboard
// (the specification C1). focused is the ring panel that holds
// focus, and popup reports a popup over the whole screen. The open console
// owns the keyboard over the ring, and attached Details does too; a popup owns
// it over all of them. The Log's rows are logLines, the console never reads
// them and renders its own snapshot.
func compose(width, height int, lay *screenLayout, panels []Panel, focused PanelID, popup bool, bar keys.Context, cfg Config, logLines []string, chartState chartViewState, details detailsFrame, slots *store.Slots, cursorIdx, bodyCursorIdx, chartTop int, now time.Time, place location.PlaceInfo, lens lookup.Lens, rowTag func(*store.Slot) string, floatTitle string, witness placeWitness, strips map[PanelID]tabStrip, console consoleState, bv bodiesView, lf logFocusView, sessionLine, cursorChart string) string {
	if lay == nil {
		return tooSmallView(width, height, cfg)
	}
	chartLines := chartContentLines(slots)
	keybar := keys.RenderBar(bar, width)

	detailsR, detailsShown := lay.rects[PanelDetails]
	consoleFocused := console.open && !popup
	// The panel Details was entered from keeps a green mark while Details owns
	// the keyboard.
	origin := PanelNone
	if details.state.attached && detailsShown && !popup && !console.open && !lf.active {
		origin = focused
	}
	if popup || console.open || lf.active || (details.state.attached && detailsShown) {
		focused = PanelNone
	}
	if s, ok := strips[origin]; ok {
		strips = maps.Clone(strips)
		s.origin = true
		strips[origin] = s
	}

	// A console off to the side has no right column to take, so it takes the
	// whole main area and the left column is not built at all.
	if console.open && !lay.beside {
		r, _ := consoleRectOf(lay, height)
		return lipgloss.JoinVertical(lipgloss.Left,
			renderConsole(console.snapshot, console, strips[PanelLog], r.w, r.h, consoleFocused, cfg), keybar)
	}

	slotCount := 0
	if slots != nil {
		slotCount = len(slots.All())
	}
	detailsBelow := func(id PanelID) bool {
		d, ok := lay.rects[PanelDetails]
		return ok && !lay.beside && d.y == lay.rects[id].y+lay.rects[id].h
	}

	var left []string
	for _, id := range leftStack {
		r := lay.rects[id]
		if lay.folded[id] {
			left = append(left, foldedLine(id, r.w, strips[id], foldedFacts{sessionLine: sessionLine, chartCaption: cursorChart, cursor: cursorIdx, charts: slotCount, bodyCursor: bodyCursorIdx, bodyRows: panel.BodyRowCount(chartState.positions, chartState.cusps)}, cfg))
			continue
		}
		var box string
		switch id {
		case PanelStatus:
			box = renderTabbedBox(strips[PanelStatus], r.w, r.h, focused == PanelStatus, cfg, sessionLine)
		case PanelChart:
			// Title is title-only; each slot's location lives in its slot line,
			// truncated to fit. Pre-resolution falls back to the single-slot
			// rendering driven by clock+place.
			slotBody := chartSlotsBodyLayout(slots, cursorIdx, focused == PanelChart, now, place, lens, r.w-2, cfg, rowTag, floatTitle, witness)
			// More slots than rows: draw the window that holds the cursor, and
			// below an open Details the cursor's row with one row each side.
			top := chartTop
			if detailsBelow(PanelChart) {
				top = min(max(cursorIdx-1, 0), max(slotCount-(r.h-2), 0))
			}
			winStart, winRows := chartWindow(slotCount, cursorIdx, r.h-2, top)
			if rows := strings.Split(slotBody, "\n"); slotCount > r.h-2 && len(rows) == slotCount {
				slotBody = strings.Join(rows[winStart:winStart+winRows], "\n")
			}
			box = renderTabbedBox(strips[PanelChart], r.w, r.h, focused == PanelChart, cfg, slotBody)
			// The Chart panel is as tall as its content up to chartPanelLines,
			// so while every slot shows this viewport never overflows and the
			// shared renderer draws nothing; once the list windows, it reports
			// the window.
			box = pane.WithScrollIndicator(box, pane.Viewport{Total: max(slotCount, chartLines), Visible: r.h - 2, Offset: winStart}, cfg, focused == PanelChart)
			box = withPositionCounter(box, listPosition{Cursor: cursorIdx, Total: slotCount}, cfg, focused == PanelChart)
			_ = findPanel(panels, PanelChart) // panel reserved for richer content; slot lines sourced from compose
		case PanelBodies:
			// A held Bodies panel is the previous scene's: its selection marks and row
			// cursor belong to the slot and row that frame was cast with.
			bodiesSlot, bodiesCursor := cursorIdx, bodyCursorIdx
			if bv.held {
				bodiesSlot, bodiesCursor = bv.slot, bv.cursor
			}
			bodiesTotal := panel.BodyRowCount(chartState.positions, chartState.cusps)
			view := bv
			if detailsBelow(PanelBodies) {
				planets, under := panel.BodyRowSplit(chartState.positions, chartState.cusps)
				view.top = max(panel.BodyDisplayRow(planets, under, r.h-2, bodiesCursor)-1, 0)
			}
			bodiesBody, bodiesRule, bodiesScroll := panel.RenderBodies(resolveBodiesBag(view, chartState, slots, bodiesSlot, bodiesCursor, r.w-2, r.h-2, focused == PanelBodies, cfg))
			switch bv.bodies {
			case paneEmpty:
				// Nothing of this subject yet: one faint line, no rows, no rule, no count.
				bodiesBody, bodiesRule, bodiesScroll = panel.DimBody(emptyPaneLine, cfg), -1, pane.Viewport{}
			case paneFailed:
				bodiesBody, bodiesRule, bodiesScroll = panel.DimBody(bv.failed, cfg), -1, pane.Viewport{}
			case paneRefused:
				bodiesBody = panel.DimBody(ansi.Strip(bodiesBody), cfg)
			}
			box = renderTabbedBox(strips[PanelBodies], r.w, r.h, focused == PanelBodies, cfg, bodiesBody)
			// Rule first, thumb second: the thumb's track stops above the rule row, so
			// the two post-passes can never write the same cell.
			box = pane.WithPanelRule(box, bodiesRule, cfg, focused == PanelBodies)
			box = pane.WithScrollIndicator(box, bodiesScroll, cfg, focused == PanelBodies)
			// The Bodies cursor walks planets and the cusp rows alike (cycleBodyCursor).
			if bv.bodies != paneEmpty && bv.bodies != paneFailed {
				box = withPositionCounter(box, listPosition{Cursor: bodiesCursor, Total: bodiesTotal}, cfg, focused == PanelBodies)
			}
		case PanelAspects:
			aspectsStrip := strips[PanelAspects]
			aspectsBody := renderAspectsBody(chartState, aspectsStrip.activeID(), r.w-2, r.h-2, cfg)
			held := aspectsRowsHeld(chartState, aspectsStrip.activeID(), r.w-2)
			switch bv.aspects {
			case paneEmpty:
				aspectsBody, held = panel.DimBody(emptyPaneLine, cfg), 0
			case paneFailed:
				aspectsBody, held = panel.DimBody(bv.failed, cfg), 0
			case paneRefused:
				aspectsBody = panel.DimBody(ansi.Strip(aspectsBody), cfg)
			}
			if bv.aspectsRefusal != "" {
				// An aspect is renamed to letters no list can be read with: every
				// tab says so, and none shows rows with an aspect missing.
				aspectsBody, held = panel.DimBody(strings.Join(wrapHanging(bv.aspectsRefusal, max(r.w-2, 1), ""), "\n"), cfg), 0
			}
			box = renderTabbedBox(aspectsStrip, r.w, r.h, focused == PanelAspects, cfg, aspectsBody)
			box = pane.WithScrollIndicator(box, pane.Viewport{Total: held, Visible: r.h - 2}, cfg, focused == PanelAspects)
		}
		if id == origin {
			box = pane.WithOriginMark(box, cfg)
		}
		left = append(left, box)
		if d, ok := lay.rects[PanelDetails]; ok && detailsBelow(id) {
			left = append(left, renderDetailsPanel(d.w, d.h, details.state.attached && !popup && !lf.active, cfg, details))
		}
	}
	if b := lay.blank; b.h > 0 {
		left = append(left, strings.TrimSuffix(strings.Repeat(strings.Repeat(" ", b.w)+"\n", b.h), "\n"))
	}
	leftCol := lipgloss.JoinVertical(lipgloss.Left, left...)

	main := leftCol
	// An open console takes the whole right column: the Log grows into the
	// space Details was using, and Details is not rendered. The left column
	// is composed exactly as it was, so the panels the operator was reading
	// do not move under the console.
	if console.open {
		r, _ := consoleRectOf(lay, height)
		rightCol := renderConsole(console.snapshot, console, strips[PanelLog], r.w, r.h, consoleFocused, cfg)
		main = lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol)
	} else if lay.beside {
		logR := lay.rects[PanelLog]
		detailsBox := renderDetailsPanel(detailsR.w, detailsR.h, details.state.attached && !popup && !lf.active, cfg, details)
		logBody := strings.Join(logLines, "\n")
		logView := renderTabbedBox(strips[PanelLog], logR.w, logR.h, focused == PanelLog, cfg, logBody)
		// The side Log shows its newest lines (padBody), so what it hides is
		// above them: the thumb sits at the bottom of the track. It has no
		// scroll key; the console (,l) is where older lines are read.
		if logR.h > 2 {
			total := len(strings.Split(logBody, "\n"))
			logView = pane.WithScrollIndicator(logView, pane.Viewport{Total: total, Visible: logR.h - 2, Offset: max(total-(logR.h-2), 0)}, cfg, focused == PanelLog)
		}
		if lf.active {
			// The focused Log is the console's list over the panel's own lines:
			// a cursor row, the window that holds it, the thumb and the count.
			logView = renderConsole(lf.lines, consoleState{cursor: lf.cursor, offset: lf.offset, panel: true}, strips[PanelLog], logR.w, logR.h, true, cfg)
		}
		rightCol := lipgloss.JoinVertical(lipgloss.Left, detailsBox, logView)
		main = lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol)
	}

	return lipgloss.JoinVertical(lipgloss.Left, main, keybar)
}

// foldedFacts is what a folded panel's title line says beyond its title.
type foldedFacts struct {
	sessionLine  string // the Status panel's one line
	chartCaption string // the chart under the cursor
	cursor       int    // the Chart panel's cursor
	charts       int
	bodyCursor   int // the Bodies panel's cursor row
	bodyRows     int
}

// foldedLine is a folded panel: its title line, drawn faint, `╶─[3] Bodies──…──2 of 18─╴`.
// The Status panel's line stands at the right end, the Chart panel's title
// names the chart under the cursor, and a list panel ends in its position.
// With colour off the characters are the same, and none is faint.
func foldedLine(id PanelID, width int, strip tabStrip, f foldedFacts, cfg Config) string {
	left, right, dash := "╶─", "─╴", "─"
	if cfg.UseASCII() {
		left, right, dash = "--", "--", "-"
	}
	title := strip.prefix
	if len(strip.tabs) > 0 {
		title += strip.tabs[min(max(strip.active, 0), len(strip.tabs)-1)].Title
	}
	end := ""
	switch id {
	case PanelStatus:
		end = f.sessionLine
	case PanelChart:
		if f.chartCaption != "" && f.chartCaption != tabTitleChart {
			title += " · " + f.chartCaption
		}
		end = listPosition{Cursor: f.cursor, Total: f.charts}.text()
	case PanelBodies:
		if f.bodyRows > 0 {
			end = listPosition{Cursor: f.bodyCursor, Total: f.bodyRows}.text()
		}
	}
	// The title keeps its place; the end gives way first, and a title too long
	// for the line is cut.
	room := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	title = term.CutRight(title, max(room-1, 0), cfg)
	if endRoom := room - ansi.StringWidth(title) - 1; end != "" && endRoom < ansi.StringWidth(end) {
		end = term.CutRight(end, max(endRoom, 0), cfg)
	}
	fill := max(room-ansi.StringWidth(title)-ansi.StringWidth(end), 0)
	line := left + title + strings.Repeat(dash, fill) + end + right
	if cfg.UseColor() {
		line = lipgloss.NewStyle().Faint(true).Render(line)
	}
	return line
}

// detailsFrame is everything the Details panel renders from: its view state
// (report frame, active tab choice, attachment, row cursor), the resolved tab
// strip, and the timeline's rows. Model.detailsFrame assembles it once per
// frame, so the strip, the body and the cursor read the same active tab.
type detailsFrame struct {
	state detailsViewState
	strip tabStrip
	rows  []lookup.TimelineRow
	marks map[timelineMark]bool
	// waiting is detailsTabWaiting for the active tab: its body dims.
	waiting bool
	// loading is detailsTabWaiting for the active tab: its replacement or
	// first content is owed. No counter is drawn while it holds.
	loading bool
	// blank draws an empty body: the tab has nothing of its subject yet.
	blank bool
	// loadingPhase is the loading label's dot count plus one; the zero
	// value (a hand-built frame) is no overlay.
	loadingPhase int
	// nearest is the index in rows of the nearest event; hasNearest says
	// there is one (the zero value is "none", so a hand-built frame has no
	// bold row).
	nearest    int
	hasNearest bool
	// search is the timeline's last row, when it has one.
	search panel.TimelineSearch
	// template is a body template tab's lines, the last complete frame,
	// laid out at draw time for the panel's width.
	template []templates.Line
	// report is the report tab's lines under its `/` query; nil draws the
	// report as printed (a hand-built frame).
	report *panel.ViewLines
	// notice is the one faint line the report tab reads instead of a body: the
	// `…` of a pane that has nothing of its subject yet, or the failure line.
	notice string
	// noCursor draws the timeline rows without the row cursor while the tab
	// waits. Details stays attached and its border focused: a wait is not a
	// detach (E1, operator 2026-10-24).
	noCursor bool
}

func (f detailsFrame) nearestRow() int {
	if !f.hasNearest {
		return -1
	}
	return f.nearest
}

func (m Model) detailsFrame() detailsFrame {
	rows := m.timelineShown()
	n := m.nearestRow(rows)
	shown := m.list(m.shownList())
	search := panel.TimelineSearch{Show: m.timelineSearchLine(), Open: shown.searching && m.details.attached, Query: shown.query, Total: len(m.timelineRows())}
	frame := detailsFrame{state: m.details, strip: m.tabStrips()[PanelDetails], rows: rows, marks: shown.marks, nearest: n, hasNearest: n >= 0, search: search, template: m.tpl.lines}
	if m.focused == PanelStatus {
		frame.state.body = m.detailsBodyShown()
	}
	if frame.strip.activeID() == TabDetailsReport {
		v := m.detailsView()
		frame.report = &v
		frame.search = panel.TimelineSearch{Show: v.Line, Open: m.detailsReportSearchOpen(), Query: m.details.query, Total: v.Total}
	}
	frame.waiting = m.detailsTabDimmed(frame.strip.activeID())
	frame.loading = m.detailsTabWaiting(frame.strip.activeID())
	if frame.loading {
		frame.loadingPhase = m.loadingPhase(frame.strip.activeID())
	}
	// A tab with nothing of its subject yet draws an empty body, no `…`
	// (operator, 2026-11-06: "remove the '...' on a blank screen"): the
	// loading label in the bottom edge says a wait is going on.
	switch id := frame.strip.activeID(); {
	case id == TabDetailsReport:
		switch m.detailsReportState() {
		case paneEmpty:
			frame.blank = true
		case paneFailed:
			frame.notice = failedReportLine(m.detailsWanted().caption)
		case paneRefused:
			// Kept from before a refused cast: faint, and no ` …`, since
			// nothing is on its way.
			frame.waiting = true
		}
	case isListTab(id):
		frame.blank = m.timelinePaneState() == paneEmpty
	case isTemplateTab(id):
		frame.blank = m.templatePaneState() == paneEmpty
	}
	if m.detailsTabWaiting(frame.strip.activeID()) && isListTab(frame.strip.activeID()) {
		// The previous list, with no cursor and no nearest row: nothing on it
		// can be acted on (rule 4, rule 7).
		frame.rows, frame.nearest, frame.hasNearest = m.timelineDisplayRows(), -1, false
		frame.noCursor = true
	}
	return frame
}

// renderDetailsPanel draws Details: the `[0]` tab strip in the title border
// (`report` titled with the report's own context, and `timeline`), and the
// active tab's body. The report tab shows Astrolog's report from the panel's
// scroll offset down — the generic panel renderer autoscrolls to the final
// lines for Log, and clipping here keeps Details anchored where the operator
// scrolled to instead; the offset is re-clamped on every render, so a resize
// that shortens the report's tail corrects itself with no keypress (C18).
// The timeline tab shows its rows (panel.RenderTimeline). The border draws
// focused while Details owns the keyboard: attached, with no popup over it.
func renderDetailsPanel(width, height int, focused bool, cfg Config, frame detailsFrame) string {
	state := frame.state
	innerW, innerH := width-2, height-2
	var body string
	var scroll pane.Viewport
	timeline := isListTab(frame.strip.activeID()) && frame.notice == "" && !frame.blank
	switch {
	case frame.blank:
		body, scroll = "", pane.Viewport{}
	case timeline:
		body, scroll = panel.RenderTimeline(resolveTimelineBag(frame, focused, innerW, innerH, cfg))
	default:
		body, scroll = panel.RenderDetailsReport(resolveReportBag(frame, innerW, innerH, cfg))
	}
	box := pane.WithScrollIndicator(renderTabbedBox(frame.strip, width, height, focused, cfg, body), scroll, cfg, focused)
	// While a wait is on the edge carries no counter (operator: "no '0 of 0' text
	// or anything else, none if loading"); a second into it, `loading` at its left.
	switch {
	case frame.loading:
		box = withLoadingLabel(box, frame.loadingPhase, cfg, focused)
	case timeline:
		box = withPositionCounter(box, listPosition{Cursor: state.cursor, Total: len(frame.rows)}, cfg, focused)
	}
	return box
}

// tooSmallView is the friendly fallback for terminals below the minimum.
func tooSmallView(width, height int, cfg Config) string {
	sep := "×"
	if cfg.UseASCII() {
		sep = "x"
	}
	requiredHeight := MinTerminalHeight
	msg := fmt.Sprintf(
		"The window is too small\n\nnow:      %d%s%d\nneeded:   %d%s%d or more\n\nMake it larger, and Sextile goes on.",
		width, sep, height, MinPanelWidth, sep, requiredHeight,
	)

	box := noticeBoxStyle(cfg).Padding(1, 2).Render(msg)

	w, h := width, height
	if w <= 0 {
		w = 40
	}
	if h <= 0 {
		h = 10
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}

// noticeBoxStyle is the frame of a whole-screen notice: the too-small box and
// the can't-start box share it. Padding is the caller's.
func noticeBoxStyle(cfg Config) lipgloss.Style {
	border := lipgloss.RoundedBorder()
	if cfg.UseASCII() {
		border = asciiBorder
	}
	style := lipgloss.NewStyle().Border(border)
	if cfg.UseColor() {
		style = style.BorderForeground(term.TooSmallFrameColour)
	}
	return style
}

func findPanel(panels []Panel, name PanelID) Panel {
	for _, p := range panels {
		if p.Name() == name {
			return p
		}
	}
	return nil
}

// statusHeight is the Status panel open: its one line and two borders.
const statusHeight = 3

// detailsBelowMin is the least height Details gets below its panel. With less,
// the panel folds to its title line as well.
const detailsBelowMin = 5

// leftStack is the order of the panels down the left column, at every size.
var leftStack = []PanelID{PanelStatus, PanelChart, PanelBodies, PanelAspects}

// layoutInput is everything that decides where a panel is drawn.
type layoutInput struct {
	width, height int
	focus         PanelID // one of the left column's panels
	detailsOpen   bool    // Enter went into Details
	charts        int     // rows of the Chart panel: the charts and now
	shown         int     // charts shown side by side in Bodies
}

// screenLayout is each panel's rectangle, which left panels are folded to
// their title line, and whether Details stands beside the panels.
type screenLayout struct {
	rects  map[PanelID]panelRect
	folded map[PanelID]bool
	beside bool
	// blank is the empty block under the left column when the open panels need
	// fewer rows than the window has; zero height when there is none.
	blank panelRect
}

// restPair is the two panels open at rest in a window without Details beside
// them: the panel in focus and its partner, in screen order.
func restPair(focus PanelID) [2]PanelID {
	switch focus {
	case PanelStatus:
		return [2]PanelID{PanelStatus, PanelChart}
	case PanelChart:
		return [2]PanelID{PanelChart, PanelBodies}
	}
	return [2]PanelID{PanelBodies, PanelAspects}
}

// layoutFor lays out the screen. It is the one place that decides where a
// panel is drawn; nil means the window is below the minimum.
func layoutFor(in layoutInput) *screenLayout {
	if in.width < MinPanelWidth || in.height < MinTerminalHeight {
		return nil
	}
	charts := max(in.charts, 1)
	mainH := in.height - KeyBarHeight
	chartH := chartPanelLines(in.height, charts) + 2
	order := []PanelID{PanelStatus, PanelChart, PanelBodies, PanelAspects}
	l := &screenLayout{rects: map[PanelID]panelRect{}, folded: map[PanelID]bool{}}

	if in.width >= dualColumnMinWidth(in.shown) && mainH >= statusHeight+chartH+5+AspectsHeight {
		left := leftColumnWidth(in.shown)
		bodiesH := mainH - statusHeight - chartH - AspectsHeight
		y := 0
		for _, p := range []struct {
			id PanelID
			h  int
		}{{PanelStatus, statusHeight}, {PanelChart, chartH}, {PanelBodies, bodiesH}, {PanelAspects, AspectsHeight}} {
			l.rects[p.id] = panelRect{x: 0, y: y, w: left, h: p.h}
			y += p.h
		}
		l.rects[PanelDetails] = panelRect{x: left, y: 0, w: in.width - left, h: mainH - LogHeight}
		l.rects[PanelLog] = panelRect{x: left, y: mainH - LogHeight, w: in.width - left, h: LogHeight}
		l.beside = true
		return l
	}

	heights := map[PanelID]int{}
	var detailsH int
	switch {
	case in.detailsOpen:
		// The panel in focus keeps its cursor row and one row each side;
		// Details takes the rest right below it. Too short for both, and the
		// panel folds as well.
		for _, id := range order {
			heights[id] = 1
		}
		own := min(rowsHeld(in.focus, charts), 3) + 2
		if mainH-(len(order)-1)-own < detailsBelowMin {
			own = 1
		}
		heights[in.focus] = own
		detailsH = mainH - (len(order) - 1) - own
	default:
		// At rest two panels are open, the panel in focus and its partner; the
		// others are their title lines. The Chart panel grows with its charts
		// until the screen is full, and only then scrolls. Rows the pair does
		// not need stay empty at the bottom.
		for _, id := range order {
			heights[id] = 1
		}
		free := mainH - len(order) + 2 // the pair's own two title rows
		switch pair := restPair(in.focus); pair[0] {
		case PanelStatus:
			heights[PanelStatus] = statusHeight
			heights[PanelChart] = min(charts+2, free-statusHeight)
		case PanelChart:
			heights[PanelChart] = min(charts+2, free-3)
			heights[PanelBodies] = free - heights[PanelChart]
		default:
			heights[PanelAspects] = min(AspectsHeight, free-3)
			heights[PanelBodies] = free - heights[PanelAspects]
		}
	}

	y := 0
	for _, id := range order {
		h := heights[id]
		l.rects[id] = panelRect{x: 0, y: y, w: in.width, h: h}
		l.folded[id] = h == 1
		y += h
		if in.detailsOpen && id == in.focus {
			l.rects[PanelDetails] = panelRect{x: 0, y: y, w: in.width, h: detailsH}
			y += detailsH
		}
	}
	if y < mainH {
		l.blank = panelRect{x: 0, y: y, w: in.width, h: mainH - y}
	}
	return l
}

// rowsHeld is how many rows a left panel holds, for the cursor window.
func rowsHeld(id PanelID, charts int) int {
	switch id {
	case PanelStatus:
		return 1
	case PanelChart:
		return charts
	}
	return 3
}
