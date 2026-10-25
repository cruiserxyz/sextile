package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/templates"
)

// Geometry constants mined from legacy_sextile UI-surface composition.
const (
	// MinPanelWidth is the narrowest terminal any layout draws on (the
	// tooSmall gate); it is no longer the left column's width.
	MinPanelWidth = 66
	// leftBaseWidth is the left column with one chart shown; it grows by
	// leftColumnStep for every further chart of the scene (the specification
	// C5), and Details needs detailsMinWidth beside it or it goes first.
	leftBaseWidth   = 52
	leftColumnStep  = 12
	detailsMinWidth = 80
	AspectsHeight   = 6
	LogHeight       = 6
	KeyBarHeight    = 1
	// MinTerminalHeight is the floor a single-slot layout needs:
	// chart(3) + bodies(5 min) + aspects(6) + keybar(1) = 15. No row is
	// reserved for a status bar (the specification C15).
	// Multi-slot workspaces raise this; layoutGeometry refuses anything
	// that would overflow, and tooSmallView reports the slot-aware
	// minimum so users see an accurate "required" line.
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
// slot in the workspace; layoutGeometry holds it to chartPanelLines. Returns 1 when the workspace is not yet built (cold start,
// pre-resolution) so the panel still has a body to render the placeholder
// line. Drives the panel's responsive height (chartH = lines + 2) and shrinks
// Bodies to suit.
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

// layoutGeometry computes each visible panel's rectangle for the given
// terminal size. Returns nil when the terminal is below the minimum.
// chartLines is the Chart panel's content-line count, which sets its height
// (chartH = chartLines + 2 for the borders). Pass chartContentLines().
// shown is how many charts the scene shows (Bodies columns); it sets the left
// column's width.
func layoutGeometry(width, height, chartLines, shown int) map[PanelID]panelRect {
	if width < MinPanelWidth || height < MinTerminalHeight {
		return nil
	}
	if chartLines < 1 {
		chartLines = 1
	}
	chartLines = chartPanelLines(height, chartLines)
	mainHeight := height - KeyBarHeight

	leftWidth := width
	dual := width >= dualColumnMinWidth(shown)
	if dual {
		leftWidth = leftColumnWidth(shown)
	}
	rightWidth := width - leftWidth

	chartH := chartLines + 2 // content lines + top/bottom border
	aspectsH := AspectsHeight
	// Refuse layouts where chart + minimum bodies + aspects cannot fit.
	// MinTerminalHeight gates the *base* layout (1-slot chart); multi-slot
	// workspaces (chartLines > 1) raise the effective minimum, so check
	// per layout rather than relying on the constant alone. Falling back
	// to tooSmallView is the documented behavior under the minimum.
	const minBodiesH = 5
	if mainHeight < chartH+minBodiesH+aspectsH {
		return nil
	}
	bodiesH := mainHeight - chartH - aspectsH
	if bodiesH < minBodiesH {
		bodiesH = minBodiesH
	}

	rects := map[PanelID]panelRect{
		PanelChart:   {x: 0, y: 0, w: leftWidth, h: chartH},
		PanelBodies:  {x: 0, y: chartH, w: leftWidth, h: bodiesH},
		PanelAspects: {x: 0, y: chartH + bodiesH, w: leftWidth, h: aspectsH},
	}

	if dual {
		logH := LogHeight
		detailsH := mainHeight - logH
		rects[PanelDetails] = panelRect{x: leftWidth, y: 0, w: rightWidth, h: detailsH}
		rects[PanelLog] = panelRect{x: leftWidth, y: detailsH, w: rightWidth, h: logH}
	}

	return rects
}

// hitTestPanel returns the name of the panel whose rectangle contains (x, y),
// or "" if the click is on chrome.
func hitTestPanel(width, height, chartLines, shown, x, y int) PanelID {
	for name, r := range layoutGeometry(width, height, chartLines, shown) {
		if x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h {
			return name
		}
	}
	return PanelNone
}

// logLines is the 6-row Log's view of the buffer (error and note lines —
// Model.logPanelLines); the console never reads it and renders its own
// snapshot instead, at both of its call sites below.
//
// Exactly one frame is drawn focused: the one that owns the keyboard
// (the specification C1). focused is the ring panel that holds
// focus, and popup reports a popup over the whole screen. The open console
// owns the keyboard over the ring, and attached Details does too; a popup owns
// it over all of them.
func compose(width, height int, panels []Panel, focused PanelID, popup bool, bar []BindingEntry, cfg Config, logLines []string, chartState chartViewState, details detailsFrame, slots *store.Slots, cursorIdx, bodyCursorIdx, chartTop int, now time.Time, place location.PlaceInfo, lens lookup.Lens, rowTag func(*store.Slot) string, floatTitle string, strips map[PanelID]tabStrip, console consoleState, indicator string, bv bodiesView) string {
	chartLines := chartContentLines(slots)
	rects := layoutGeometry(width, height, chartLines, bv.count())
	if rects == nil {
		return tooSmallView(width, height, chartLines, cfg)
	}

	detailsR, detailsShown := rects[PanelDetails]
	consoleFocused := console.open && !popup
	if popup || console.open || (details.state.attached && detailsShown) {
		focused = PanelNone
	}

	// A narrow console takes the whole main area, so the left column is not
	// built at all — the three panels have nowhere to go.
	if console.open && width < dualColumnMinWidth(bv.count()) {
		r, _ := consoleRect(width, height, chartLines, bv.count())
		keybar := renderKeyBar(width, bar, cfg, indicator)
		return lipgloss.JoinVertical(lipgloss.Left,
			renderConsole(console.snapshot, console, strips[PanelLog], r.w, r.h, consoleFocused, cfg), keybar)
	}

	chartR := rects[PanelChart]
	bodiesR := rects[PanelBodies]
	aspectsR := rects[PanelAspects]

	// Title is title-only; each slot's location lives in its slot line,
	// truncated to fit. Pre-resolution falls back to the single-slot
	// rendering driven by clock+place.
	slotBody := chartSlotsBodyLayout(slots, cursorIdx, focused == PanelChart, now, place, lens, chartR.w-2, cfg, rowTag, floatTitle, bv.count() == 1)
	slotCount := 0
	if slots != nil {
		slotCount = len(slots.All())
	}
	// More slots than rows: draw the window that holds the cursor.
	winStart, winRows := chartWindow(slotCount, cursorIdx, chartR.h-2, chartTop)
	if rows := strings.Split(slotBody, "\n"); slotCount > chartR.h-2 && len(rows) == slotCount {
		slotBody = strings.Join(rows[winStart:winStart+winRows], "\n")
	}
	chart := renderTabbedBox(strips[PanelChart], chartR.w, chartR.h, focused == PanelChart, cfg, slotBody)
	// The Chart panel is as tall as its content (chartH = chartLines + 2) up to
	// chartPanelLines, so while every slot shows this viewport never overflows and
	// the shared renderer draws nothing; once the list windows, it reports the
	// window.
	chart = withScrollIndicator(chart, viewport{Total: max(slotCount, chartLines), Visible: chartR.h - 2, Offset: winStart}, cfg, focused == PanelChart)
	chart = withPositionCounter(chart, listPosition{Cursor: cursorIdx, Total: slotCount}, cfg, focused == PanelChart)
	_ = findPanel(panels, PanelChart) // panel reserved for richer content; slot lines sourced from compose
	// A held Bodies panel is the previous scene's: its selection marks and row
	// cursor belong to the slot and row that frame was cast with.
	bodiesSlot, bodiesCursor := cursorIdx, bodyCursorIdx
	if bv.held {
		bodiesSlot, bodiesCursor = bv.slot, bv.cursor
	}
	bodiesBody, bodiesRule, bodiesScroll := renderBodiesFor(bv, chartState, slots, bodiesSlot, bodiesCursor, bodiesR.w-2, bodiesR.h-2, focused == PanelBodies, cfg)
	bodies := renderTabbedBox(strips[PanelBodies], bodiesR.w, bodiesR.h, focused == PanelBodies, cfg, bodiesBody)
	// Rule first, thumb second: the thumb's track stops above the rule row, so
	// the two post-passes can never write the same cell.
	bodies = withPanelRule(bodies, bodiesRule, cfg, focused == PanelBodies)
	bodies = withScrollIndicator(bodies, bodiesScroll, cfg, focused == PanelBodies)
	// The Bodies cursor walks planets and the cusp rows alike (cycleBodyCursor).
	bodies = withPositionCounter(bodies, listPosition{Cursor: bodiesCursor, Total: bodyRowCount(chartState.positions, chartState.cusps)}, cfg, focused == PanelBodies)
	aspectsStrip := strips[PanelAspects]
	aspectsBody := renderAspectsBody(chartState, aspectsStrip.activeID(), aspectsR.w-2, aspectsR.h-2, cfg)
	aspects := renderTabbedBox(aspectsStrip, aspectsR.w, aspectsR.h, focused == PanelAspects, cfg, aspectsBody)

	leftCol := lipgloss.JoinVertical(lipgloss.Left, chart, bodies, aspects)

	main := leftCol
	// An open console takes the whole right column: the Log grows into the
	// space Details was using, and Details is not rendered. The left column
	// is composed exactly as it was, so the panels the operator was reading
	// do not move under the console.
	if console.open {
		r, _ := consoleRect(width, height, chartLines, bv.count())
		rightCol := renderConsole(console.snapshot, console, strips[PanelLog], r.w, r.h, consoleFocused, cfg)
		main = lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol)
		detailsShown = false
	} else if detailsShown {
		logR := rects[PanelLog]
		detailsBox := renderDetailsPanel(detailsR.w, detailsR.h, details.state.attached && !popup, cfg, details)
		logBody := strings.Join(logLines, "\n")
		logView := renderTabbedBox(strips[PanelLog], logR.w, logR.h, focused == PanelLog, cfg, logBody)
		// The side Log shows its newest lines (padBody), so what it hides is
		// above them: the thumb sits at the bottom of the track. It has no
		// scroll key; the console (,l) is where older lines are read.
		if logR.h > 2 {
			total := len(strings.Split(logBody, "\n"))
			logView = withScrollIndicator(logView, viewport{Total: total, Visible: logR.h - 2, Offset: max(total-(logR.h-2), 0)}, cfg, focused == PanelLog)
		}
		rightCol := lipgloss.JoinVertical(lipgloss.Left, detailsBox, logView)
		main = lipgloss.JoinHorizontal(lipgloss.Top, leftCol, rightCol)
	}

	keybar := renderKeyBar(width, bar, cfg, indicator)

	return lipgloss.JoinVertical(lipgloss.Left, main, keybar)
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
	// nearest is the index in rows of the nearest event; hasNearest says
	// there is one (the zero value is "none", so a hand-built frame has no
	// bold row).
	nearest    int
	hasNearest bool
	// search is the timeline's last row, when it has one.
	search timelineSearchView
	// template is a body template tab's lines, the last complete frame,
	// laid out at draw time for the panel's width.
	template []templates.Line
	// report is the report tab's lines under its `/` query; nil draws the
	// report as printed (a hand-built frame).
	report *detailsViewLines
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
	search := timelineSearchView{show: m.timelineSearchLine(), open: m.timeline.searching && m.details.attached, query: m.timeline.query, total: len(m.timelineRows())}
	frame := detailsFrame{state: m.details, strip: m.tabStrips()[PanelDetails], rows: rows, marks: m.timeline.marks, nearest: n, hasNearest: n >= 0, search: search, template: m.tpl.lines}
	if frame.strip.activeID() == TabDetailsReport {
		v := m.detailsView()
		frame.report = &v
		frame.search = timelineSearchView{show: v.line, open: m.detailsReportSearchOpen(), query: m.details.query, total: v.total}
	}
	frame.waiting = m.detailsTabDimmed(frame.strip.activeID())
	if m.detailsTabWaiting(frame.strip.activeID()) && frame.strip.activeID() == TabDetailsTimeline {
		// The previous list, with no cursor and no nearest row: nothing on it
		// can be acted on (rule 4, rule 7).
		frame.rows, frame.nearest, frame.hasNearest = m.timelineDisplayRows(), -1, false
		frame.noCursor = true
	}
	return frame
}

// dimBody makes every line of a Details body faint with colours on; with
// colours off the tab's title carries the ` …` mark instead.
func dimBody(body string, cfg Config) string {
	if body == "" || !cfg.useColor() {
		return body
	}
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		lines[i] = sgrFaint + reapplyAfterSGR(ln, sgrFaint) + sgrReset
	}
	return strings.Join(lines, "\n")
}

// renderDetailsPanel draws Details: the `[0]` tab strip in the title border
// (`report` titled with the report's own context, and `timeline`), and the
// active tab's body. The report tab shows Astrolog's report from the panel's
// scroll offset down — the generic panel renderer autoscrolls to the final
// lines for Log, and clipping here keeps Details anchored where the operator
// scrolled to instead; the offset is re-clamped on every render, so a resize
// that shortens the report's tail corrects itself with no keypress (C18).
// The timeline tab shows its rows (renderTimelineBody). The border draws
// focused while Details owns the keyboard: attached, with no popup over it.
func renderDetailsPanel(width, height int, focused bool, cfg Config, frame detailsFrame) string {
	state := frame.state
	innerW, innerH := width-2, height-2
	var body string
	var scroll viewport
	if frame.strip.activeID() == TabDetailsTimeline {
		rowState := state
		rowState.attached = rowState.attached && !frame.noCursor
		body, scroll = renderTimelineBody(frame.rows, frame.marks, frame.nearestRow(), rowState, frame.search, innerW, innerH, cfg)
		if frame.waiting && len(frame.rows) > 0 {
			body = dimBody(body, cfg)
		}
	} else if innerH > 0 {
		lines := detailsReportLines(state.body)
		listH := innerH
		searchRow := ""
		if frame.report != nil && frame.report.line && !isTemplateTab(frame.strip.activeID()) {
			lines = frame.report.lines
			listH = innerH - 1
			searchRow = cutRight(frame.search.searchLine(frame.report.shown, innerW), innerW, cfg)
		}
		if isTemplateTab(frame.strip.activeID()) {
			lines = templateBodyLines(frame.template, innerW, cfg)
		}
		scroll = viewport{
			Total:   len(lines),
			Visible: listH,
			Offset:  clampOffset(state.scrollOffset, len(lines), listH),
		}
		end := scroll.Offset + listH
		if end > len(lines) {
			end = len(lines)
		}
		shownLines := lines[scroll.Offset:end]
		body = strings.Join(shownLines, "\n")
		if frame.waiting && (!isTemplateTab(frame.strip.activeID()) || len(frame.template) > 0) {
			// What is on screen is the previous content: faint until the new
			// one commits (the specification rule 6, widened to every
			// Details tab by the specification rule 5).
			body = dimBody(body, cfg)
		}
		if searchRow != "" {
			// The query line is the panel's last row, as on the timeline.
			var pad []string
			if len(shownLines) > 0 {
				pad = strings.Split(body, "\n")
			}
			for len(pad) < listH {
				pad = append(pad, "")
			}
			body = strings.Join(append(pad, searchRow), "\n")
		}
	}
	box := withScrollIndicator(renderTabbedBox(frame.strip, width, height, focused, cfg, body), scroll, cfg, focused)
	if frame.strip.activeID() == TabDetailsTimeline {
		box = withPositionCounter(box, listPosition{Cursor: state.cursor, Total: len(frame.rows)}, cfg, focused)
	}
	return box
}

// tooSmallView is the friendly fallback for terminals below the minimum.
// chartLines is the slot-count-driven chart-panel height (chartContentLines)
// so the reported "required" minimum reflects the *actual* multi-slot floor
// rather than the static one-slot constant.
func tooSmallView(width, height, chartLines int, cfg Config) string {
	sep := "×"
	if cfg.useASCII() {
		sep = "x"
	}
	requiredHeight := MinTerminalHeight
	chartLines = chartPanelLines(height, chartLines)
	if chartLines > 1 {
		// chart(chartLines+2) + bodies(5 min) + aspects(6) + keybar(1)
		requiredHeight = chartLines + 2 + 5 + AspectsHeight + KeyBarHeight
	}
	msg := fmt.Sprintf(
		"Terminal too small\n\ncurrent:   %d%s%d\nrequired:  %d%s%d (minimum)\n\nresize and we'll pick up.",
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
	if cfg.useASCII() {
		border = asciiBorder
	}
	style := lipgloss.NewStyle().Border(border)
	if cfg.useColor() {
		style = style.BorderForeground(tooSmallFrameColour)
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

// templateBodyLines lays a body template's lines out in the panel's three
// columns (the specification R1) and dims the aside, the
// body's only second intensity (R8).
func templateBodyLines(lines []templates.Line, width int, cfg Config) []string {
	laid := templates.Layout(lines, width)
	out := make([]string, 0, len(laid))
	for _, l := range laid {
		if l.Aside != "" && cfg.useColor() {
			out = append(out, l.Main+templateAsideStyle.Render(l.Aside))
			continue
		}
		out = append(out, l.Main+l.Aside)
	}
	return out
}
