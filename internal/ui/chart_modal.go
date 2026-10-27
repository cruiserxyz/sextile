package ui

import (
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/store"
)

// chart_modal.go is the saved-chart library picker — the visible
// consumer of the earlier change chart-library substrate. It mirrors atlas_modal.go: a
// filter buffer + cursor over store.FilterCharts results. Enter sets the chosen
// chart as the natal anchor (applyChartPicked); Esc restores the view it opened
// from. The library is loaded lazily on open (loadChartsCmd → ChartsLoadedMsg →
// applyChartsLoaded), unlike the atlas which loads once at Init.
//
// State machine (driven from model.go Update):
//
//	main ──Ctrl+O──▶ startupChartOpen ──Enter(match)──▶ startupComputing ──text commit──▶ ready
//	                      │ ▲ ChartsLoadedMsg (populate)
//	                      └─Esc──▶ prior state
//
// Invariants (mirror atlasModalState):
//   - matches derives from filterText via refilter(); never set directly.
//   - cursor stays in [0, len(matches)); clamped on every refilter.
//   - the window scrolls with the cursor (fuzzyList.scrollTo), over the rows
//     the chart block actually gets — chartBlockHeight — so the row Enter
//     picks is always the marked row on screen.
//   - charts is nil and loaded=false until ChartsLoadedMsg arrives; the modal
//     renders a loading line until then.
//   - loadErr non-nil → modal renders an error line and Enter does nothing.
//   - drops are the library files that could not be read. They render as named
//     rows below the charts and are never navigable: the cursor and Enter see
//     matches only, so nothing here can be picked.
type chartPickerState struct {
	// The search buffer, cursor, window and matches are the shared chassis,
	// as for the place picker. Before it was embedded here the picker kept
	// its own cursor and no window, and the cursor walked off the bottom of
	// the list into rows nobody could see.
	fuzzyList[store.NamedChart]

	charts     []store.NamedChart // full library; nil until loaded
	drops      []store.ChartDrop  // files the library could not read
	seen       map[string]string  // the listing's fingerprint of each chart file
	pending    store.NamedChart   // the row a resolve is running for (or failed on); the Log names it
	loaded     bool               // ChartsLoadedMsg arrived (distinguishes loading from empty)
	loadErr    error              // non-nil → library read failed
	resolving  bool               // a pick's resolve is in flight
	resolveErr error              // non-nil → the last pick's resolve failed; anchor unchanged
	refusal    string             // non-empty → the last pick read fine but could not anchor
	prior      startupState       // view to restore on Esc (captured at open)
	now        time.Time          // today, for the age beside each name; zero shows no age
	land       string             // the file the cursor lands on when the next load arrives (a renamed chart); "" for none

	// fit is the picker's size, decided once: when the first load of this
	// opening arrives, for everything the library holds then. A chart that
	// appears later does not resize the box; the list scrolls, and the next
	// opening sizes for it. Zero until then (the loading line has no size to
	// keep).
	fit modalFit
}

// maxChartResults caps a filtered result set. A curated chart library is small,
// so this is a generous ceiling, matching the atlas modal's posture.
const maxChartResults = 200

// refilter recomputes matches from filterText. An empty filter browses the
// whole (sorted) library — unlike the 40k-row atlas, a chart library is small
// enough to show in full. A non-empty filter runs the three-tier FilterCharts.
func (p *chartPickerState) refilter() {
	if !p.loaded {
		p.clear()
		return
	}
	if p.filterText == "" {
		p.setMatches(p.charts, false)
		return
	}
	p.setMatches(store.FilterCharts(p.charts, p.filterText, maxChartResults))
}

// updateChartModalKey handles one keystroke for the chart picker. The caller in
// model.go has already gated on m.startup == startupChartOpen.
func updateChartModalKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch matchBinding(msg, openChartBindings()) {
	case BindingConfirm:
		if m.picker.loadErr != nil || len(m.picker.matches) == 0 {
			return m, nil
		}
		picked := m.picker.matches[m.picker.cursor]
		var cmd tea.Cmd
		m, cmd = applyChartPicked(m, ChartPickedMsg{Chart: picked})
		return m, cmd
	case BindingRename:
		if m.picker.loadErr != nil || len(m.picker.matches) == 0 {
			return m, nil
		}
		return openRenameForm(m, m.picker.matches[m.picker.cursor]), nil
	case BindingListUp:
		m.picker.moveUp()
		m.picker.scrollTo(m.picker.cursor, chartBlockHeight(m.picker, modalListHeight(m.height)))
		return m, nil
	case BindingListDown:
		m.picker.moveDown()
		m.picker.scrollTo(m.picker.cursor, chartBlockHeight(m.picker, modalListHeight(m.height)))
		return m, nil
	case BindingPageUp:
		m.picker.page(pageUp, chartBlockHeight(m.picker, modalListHeight(m.height)))
		return m, nil
	case BindingPageDown:
		m.picker.page(pageDown, chartBlockHeight(m.picker, modalListHeight(m.height)))
		return m, nil
	case BindingPageHome:
		m.picker.page(pageHome, chartBlockHeight(m.picker, modalListHeight(m.height)))
		return m, nil
	case BindingPageEnd:
		m.picker.page(pageEnd, chartBlockHeight(m.picker, modalListHeight(m.height)))
		return m, nil
	case BindingClose:
		// Esc closes the picker without changing the anchor, restoring the
		// view it opened from.
		//
		// Bumping the pick generation cancels any resolve still in flight:
		// without it, a pick the operator just backed out of would land a
		// moment later and silently change the anchor they kept.
		m.pickGen++
		m.startup = m.picker.prior
		return m, nil
	}
	if m.picker.typeKey(msg) {
		m.picker.refilter()
		m.picker.cursor = 0
		m.picker.offset = 0
	}
	return m, nil
}

// renderChartModal renders the centered picker: search line + result list +
// state line. The chrome is the shared modalBox — the box math here was
// character-identical to the atlas modal's and is no longer duplicated.
func renderChartModal(state chartPickerState, cfg Config, termW, termH int) string {
	// The cursor walks the readable charts only; an unreadable file is listed
	// but cannot be picked, so it is not a position.
	pos := listPosition{Cursor: state.cursor}
	if state.loaded && state.loadErr == nil {
		pos.Total = len(state.matches)
	}
	return renderModalBox(modalBox{
		Title:    popupTitleOpenChart,
		Fit:      state.fitOrNow(cfg),
		Position: &pos,
		Scroll: func(height int) (viewport, int) {
			block := chartBlockHeight(state, height)
			_, offset := state.windowAt(state.cursor, block)
			return viewport{Total: pos.Total, Visible: block, Offset: offset}, 0
		},
		FilterText: state.filterText,
	}, cfg, termW, termH, func(width, height int) string {
		return renderChartResultList(state, cfg, width, height)
	})
}

// fitOrNow is the size the picker was given when it opened, or, before the
// first load has given it one, the size of what it shows now.
func (s chartPickerState) fitOrNow(cfg Config) modalFit {
	if s.fit.Width > 0 {
		return s.fit
	}
	return chartFit(s, cfg)
}

// chartFit is the picker's size for everything the library holds: wide enough
// for the widest chart and for the unreadable files' lines up to a sentence;
// as tall as every chart and the unreadable files' block. The block keeps no
// more rows than the charts have, so a long list of unreadable files does not
// stretch the box (chartBlockHeight reserves them by the same rule).
func chartFit(state chartPickerState, cfg Config) modalFit {
	switch {
	case state.loadErr != nil:
		w, rows := fitMessage(fmt.Sprintf("(charts unavailable: %v)", state.loadErr), searchLineWidth("Search: "))
		return modalFit{Width: w, Rows: rows}
	case !state.loaded:
		return modalFit{Width: 1, Rows: 1} // "(loading charts…)"
	}
	fit := modalFit{Width: 1}
	for _, c := range state.charts {
		fit.Width = max(fit.Width, listRowWidth(chartRow(c, false, state.now), cfg))
	}
	drops := chartDropLines(state.drops)
	for _, l := range drops {
		fit.Width = max(fit.Width, min(ansi.StringWidth(l), modalProseWidth))
	}
	block := 1 // the charts, or the one line that says there are none
	switch {
	case len(state.charts) == 0 && len(drops) > 0:
		block = 0
	case len(state.charts) > 0:
		block = len(state.charts)
	}
	fit.Rows = block + len(drops)
	if len(state.charts) > 0 {
		fit.Rows = block + min(len(drops), block)
	}
	return fit
}

// renderChartResultList lays the list out in two blocks: the pickable charts,
// then the files the library could not read. The unreadable block is never
// squeezed out by a long chart list — it keeps a reserved tail (at most half
// the rows when charts share the list), overflowing into a single counted line.
// The point is the hard rule below: an astrologer whose files failed to parse
// must never be told they have no charts.
func renderChartResultList(state chartPickerState, cfg Config, width, height int) string {
	if state.loadErr != nil {
		return listMessage(fmt.Sprintf("(charts unavailable: %v)", state.loadErr), width, height)
	}
	if !state.loaded {
		return listMessage("(loading charts…)", width, height)
	}

	block := chartBlockHeight(state, height)
	lines := make([]string, 0, height)
	lines = append(lines, chartRows(state, cfg, width, block)...)
	// The unreadable files keep the foot of the list, whatever the filter left.
	for len(lines) < block {
		lines = append(lines, listNote("", width, cfg))
	}
	lines = append(lines, chartDropBlock(fitDropLines(chartDropLines(state.drops), height-block), cfg, width)...)
	return listBlock(lines, width, height)
}

// chartBlockHeight is how many of the list's rows the pickable charts get: the
// rest is reserved for the unreadable files, at most half the list when charts
// share it, and never all of it. Drawing and navigating both ask this, so the
// window the cursor scrolls is the window that is painted.
func chartBlockHeight(state chartPickerState, height int) int {
	reserved := len(state.drops)
	if reserved > 0 && len(state.charts) > 0 {
		if half := (height + 1) / 2; reserved > half {
			reserved = half
		}
		if reserved > height-1 { // the chart block always keeps a row to speak in
			reserved = height - 1
		}
	}
	if reserved > height {
		reserved = height
	}
	if reserved < 0 {
		reserved = 0
	}
	return height - reserved
}

// chartRows renders the pickable block: the filtered matches, or the one line
// that explains why there are none. The empty library says "(no saved charts
// yet)" only when the library really is empty — with unreadable files present
// the drop rows below are the whole and only truthful story, so this block
// stays silent and yields its rows to them.
func chartRows(state chartPickerState, cfg Config, width, height int) []string {
	if height < 1 {
		return nil
	}
	if len(state.charts) == 0 {
		if len(state.drops) > 0 {
			return nil
		}
		return []string{listNote("(no saved charts yet)", width, cfg)}
	}
	if len(state.matches) == 0 {
		return []string{listNote("(no matches)", width, cfg)}
	}

	// The window scrolls to follow the cursor; without it the cursor walks
	// past the bottom of the list and Enter picks a chart nobody can see.
	visible, offset := state.windowAt(state.cursor, height)
	lines := make([]string, 0, height)
	for i, c := range visible {
		lines = append(lines, listRow(chartRow(c, offset+i == state.cursor, state.now), cfg, width))
	}
	return lines
}

// chartDropLines is one row per unreadable file, in the same shape as a chart
// row so the two read as one list — but marked with "!" instead of the cursor's
// "▶", which is also how the eye tells a pickable row from an inert one.
func chartDropLines(drops []store.ChartDrop) []string {
	lines := make([]string, 0, len(drops))
	for _, d := range drops {
		lines = append(lines, fmt.Sprintf("! %s  —  %s", d.File, d.Reason))
	}
	return lines
}

// fitDropLines trims the unreadable block to its reserved rows, spending the
// last row on the count it had to hide. A budget of one row therefore still
// reports every unreadable file, just without naming them.
func fitDropLines(lines []string, budget int) []string {
	if budget < 1 || len(lines) == 0 {
		return nil
	}
	if len(lines) <= budget {
		return lines
	}
	out := make([]string, 0, budget)
	out = append(out, lines[:budget-1]...)
	return append(out, fmt.Sprintf("! …%d more could not be read", len(lines)-(budget-1)))
}

// chartDropBlock fits the unreadable rows to the list's width and greys them.
// They are not pickable, so they are never striped and never go through the
// row renderer: their shape is their own.
func chartDropBlock(lines []string, cfg Config, width int) []string {
	out := make([]string, 0, len(lines))
	for _, row := range lines {
		row = listNote(row, width, cfg)
		if cfg.useColor() {
			row = chartDropStyle.Render(row)
		}
		out = append(out, row)
	}
	return out
}

// chartRow is one library entry: the chart's name on the left, and its place
// and birth date pinned to the right edge so the dates line up down the list
// (the specification C7). A missing place or date is simply absent.
func chartRow(c store.NamedChart, cursor bool, now time.Time) listRowSpec {
	tail := ""
	if age, ok := chartAge(c, now); ok {
		tail = fmt.Sprintf(" (%d)", age)
	}
	return listRowSpec{
		Left:     c.Name,
		LeftTail: tail,
		Right:    []string{c.Place.Display(), chartDate(c)},
		Cursor:   cursor,
	}
}

// chartAge is the completed years from the chart's stated civil date to
// now's date, compared as (year, month, day) triples rather than through
// time.Date for the reason chartDate gives. A chart with no usable date, a
// zero now, or a date after now has no age.
func chartAge(c store.NamedChart, now time.Time) (int, bool) {
	if now.IsZero() || chartDate(c) == "" {
		return 0, false
	}
	y, m, d := now.Date()
	age := y - c.Year
	if int(m) < c.Month || (int(m) == c.Month && d < c.Day) {
		age--
	}
	if age < 0 {
		return 0, false
	}
	return age, true
}

// chartDate is the row's date label, as the chart file states it, in the
// held date order: `Jan  2, 2006` or ` 2 Jan 2006` (the specification). A file that states no usable date shows none, rather than a
// year standing in for one.
//
// It is spelled out field by field, not through time.Date: Go's calendar is
// proleptic Gregorian and would roll a Julian `29 Feb 1500` the file states
// into 1 March — a date the file never said.
func chartDate(c store.NamedChart) string {
	if c.Month < 1 || c.Month > 12 || c.Day < 1 {
		return ""
	}
	return civilDate(c.Year, c.Month, c.Day)
}
