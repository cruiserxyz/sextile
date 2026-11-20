package ui

import (
	"fmt"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/x/ansi"
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"github.com/cruiserxyz/sextile/internal/ui/term"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// chart_modal.go is the saved-chart library picker — the visible
// consumer of the earlier change chart-library substrate. It mirrors atlas_modal_model.go: a
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
//   - matches derives from FilterText via refilter(); never set directly.
//   - cursor stays in [0, len(matches)); clamped on every refilter.
//   - the window scrolls with the cursor (menu.FuzzyList.scrollTo), over the rows
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
	menu.FuzzyList[store.NamedChart]

	charts     []store.NamedChart // full library; nil until loaded
	drops      []store.ChartDrop  // files the library could not read
	seen       map[string]string  // the listing's fingerprint of each chart file
	dir        string             // the charts folder the listing read
	pending    store.NamedChart   // the row a resolve is running for (or failed on); the Log names it
	loaded     bool               // ChartsLoadedMsg arrived (distinguishes loading from empty)
	loadErr    error              // non-nil → library read failed
	resolving  bool               // a pick's resolve is in flight
	resolveErr error              // non-nil → the last pick's resolve failed; anchor unchanged
	refusal    string             // non-empty → the last pick read fine but could not anchor
	prior      startupState       // view to restore on Esc (captured at open)
	now        time.Time          // today, for the age beside each name; zero shows no age
	land       string             // the file the cursor lands on when the next load arrives (a renamed chart); "" for none
	// refused is why each listed chart file will not open, by file name: its
	// row carries a red ! and, under the cursor, this reason.
	refused map[string]string

	// fit is the picker's size, decided once: when the first load of this
	// opening arrives, for everything the library holds then. A chart that
	// appears later does not resize the box; the list scrolls, and the next
	// opening sizes for it. Zero until then (the loading line has no size to
	// keep).
	fit dialog.Fit

	// ticked is the chart files ticked with Space, in the order they were
	// ticked. They live as long as the list is open.
	ticked []string
	// queue and late are what an Enter on ticked charts still has to open: the
	// charts that open at once, in tick order, then the ones that need a title
	// first, which are asked for after the others.
	queue, late []store.NamedChart
}

// maxChartResults caps a filtered result set. A curated chart library is small,
// so this is a generous ceiling, matching the atlas modal's posture.
const maxChartResults = 200

// refilter recomputes matches from FilterText. An empty filter browses the
// whole (sorted) library — unlike the 40k-row atlas, a chart library is small
// enough to show in full. A non-empty filter runs the three-tier FilterCharts.
func (p *chartPickerState) refilter() {
	if !p.loaded {
		p.Clear()
		return
	}
	if p.FilterText == "" {
		p.SetMatches(p.charts, false)
		return
	}
	found, truncated := store.FilterCharts(p.charts, p.FilterText, maxChartResults)
	p.SetMatches(p.tickedFirst(found), truncated)
}

// tickedFirst puts the ticked charts at the top of a search's matches, in the
// order they were ticked, whether they match or not.
func (p *chartPickerState) tickedFirst(found []store.NamedChart) []store.NamedChart {
	top := p.tickedCharts()
	if len(top) == 0 {
		return found
	}
	out := top
	for _, c := range found {
		if !p.isTicked(c) {
			out = append(out, c)
		}
	}
	return out
}

func (p *chartPickerState) isTicked(c store.NamedChart) bool {
	return slices.Contains(p.ticked, c.Source.File)
}

// tickedCharts is the ticked charts the library still lists and can open, in
// tick order.
func (p *chartPickerState) tickedCharts() []store.NamedChart {
	var out []store.NamedChart
	for _, f := range p.ticked {
		for _, c := range p.charts {
			if c.Source.File != f {
				continue
			}
			if _, refused := p.refused[f]; !refused {
				out = append(out, c)
			}
			break
		}
	}
	return out
}

// toggleTick ticks the chart under the cursor, or unticks it. A chart that
// will not open cannot be ticked. While a search is typed the ticked charts
// move to the top: ticking leaves the cursor on the first row below them, and
// unticking on the chart where it went.
func (p *chartPickerState) toggleTick(height int) {
	if p.loadErr != nil || p.Cursor < 0 || p.Cursor >= len(p.Matches) {
		return
	}
	c := p.Matches[p.Cursor]
	f := c.Source.File
	if _, refused := p.refused[f]; refused {
		return
	}
	on := p.isTicked(c)
	if on {
		p.ticked = slices.DeleteFunc(p.ticked, func(t string) bool { return t == f })
	} else {
		p.ticked = append(p.ticked, f)
	}
	if p.FilterText == "" {
		return
	}
	p.refilter()
	p.Cursor = min(len(p.ticked), max(len(p.Matches)-1, 0))
	if on {
		for i, m := range p.Matches {
			if m.Source.File == f {
				p.Cursor = i
			}
		}
	}
	p.ScrollTo(p.Cursor, height)
}

// openChartPicker is Ctrl+O: the picker sets the NATAL ANCHOR for a
// transit-over-natal overlay, so it needs a resolved transit workspace first
// (blocked says when it has none). Before location resolution Ctrl+O is a no-op
// -- the user resolves their location via the atlas (Ctrl+L) first. Without
// this gate, picking a chart pre-resolution would dispatch an overlay compute
// against an empty place (the reviewer a review).
func (m Model) openChartPicker() (Model, tea.Cmd) {
	if m.blocked(keys.BindingOpenCharts) != "" {
		return m, nil
	}
	// Open the picker: capture the current view to restore on Esc, reset the
	// buffer, bump the picker generation (so a stale load from a prior open is
	// dropped), and lazily load the library -- the modal shows a loading line
	// until ChartsLoadedMsg lands.
	m.pickerGen++
	m.picker = chartPickerState{prior: m.startup}
	m.startup = startupChartOpen
	return m, loadChartsCmd(m.pickerGen)
}

// openChartBindings is the Open chart popup's table. Ctrl+R renames the
// highlighted chart; it is this table's alone, so it does nothing anywhere
// else (the specification C4.5).
func openChartBindings() []keys.BindingEntry {
	return keys.TitledLocal(append([]keys.BindingEntry{
		listUp().ShownWhen(func(c keys.Context) bool { return c.Popup.Rows > 1 }), listDown(),
		{ID: keys.BindingToggle, Binding: key.NewBinding(key.WithKeys(" ")), Title: "Tick", BarKey: "space", Bar: true, When: func(c keys.Context) bool { return c.Popup.Rows > 0 }},
		openTicked(),
		{ID: keys.BindingRename, Binding: key.NewBinding(key.WithKeys("ctrl+r")), Title: "Rename", BarKey: "ctrl+r", Bar: true, When: func(c keys.Context) bool { return c.Picker.Renames }},
		closeOn("Cancel"),
	}, pageEntries()...))
}

// openTicked is Enter in the chart list: it opens the chart under the cursor,
// or, with charts ticked, those and only those, and the bar says how many.
func openTicked() keys.BindingEntry {
	e := confirm("Open").ShownWhen(func(c keys.Context) bool { return c.Popup.Rows > 0 })
	e.TitleFn = func(c keys.Context) string {
		if c.Picker.Ticked > 0 {
			return fmt.Sprintf("Open %d", c.Picker.Ticked)
		}
		return "Open"
	}
	return e
}

// updateChartModalKey handles one keystroke for the chart picker. The caller in
// model.go has already gated on m.startup == startupChartOpen.
func updateChartModalKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keys.MatchBinding(msg, openChartBindings()) {
	case keys.BindingToggle:
		m.picker.toggleTick(chartBlockHeight(m.picker, dialog.ListHeight(m.height)))
		return m, nil
	case keys.BindingConfirm:
		if m.picker.loadErr != nil || len(m.picker.Matches) == 0 {
			return m, nil
		}
		if len(m.picker.tickedCharts()) > 0 {
			return openTickedCharts(m)
		}
		picked := m.picker.Matches[m.picker.Cursor]
		if _, refused := m.picker.refused[picked.Source.File]; refused {
			return m, nil // the reason is shown under the list already
		}
		var cmd tea.Cmd
		m, cmd = applyChartPicked(m, ChartPickedMsg{Chart: picked})
		return m, cmd
	case keys.BindingRename:
		if m.picker.loadErr != nil || len(m.picker.Matches) == 0 {
			return m, nil
		}
		return openRenameForm(m, m.picker.Matches[m.picker.Cursor]), nil
	case keys.BindingListUp:
		m.picker.MoveUp()
		m.picker.ScrollTo(m.picker.Cursor, chartBlockHeight(m.picker, dialog.ListHeight(m.height)))
		return m, nil
	case keys.BindingListDown:
		m.picker.MoveDown()
		m.picker.ScrollTo(m.picker.Cursor, chartBlockHeight(m.picker, dialog.ListHeight(m.height)))
		return m, nil
	case keys.BindingPageUp:
		m.picker.Page(menu.PageUp, chartBlockHeight(m.picker, dialog.ListHeight(m.height)))
		return m, nil
	case keys.BindingPageDown:
		m.picker.Page(menu.PageDown, chartBlockHeight(m.picker, dialog.ListHeight(m.height)))
		return m, nil
	case keys.BindingPageHome:
		m.picker.Page(menu.PageHome, chartBlockHeight(m.picker, dialog.ListHeight(m.height)))
		return m, nil
	case keys.BindingPageEnd:
		m.picker.Page(menu.PageEnd, chartBlockHeight(m.picker, dialog.ListHeight(m.height)))
		return m, nil
	case keys.BindingClose:
		// Esc closes the picker without changing the anchor, restoring the
		// view it opened from.
		//
		// Bumping the pick generation cancels any resolve still in flight:
		// without it, a pick the operator just backed out of would land a
		// moment later and silently change the anchor they kept.
		m.pickGen++
		m.picker.queue, m.picker.late = nil, nil
		m.startup = m.picker.prior
		return m, nil
	}
	if m.picker.TypeKey(msg) {
		m.picker.refilter()
		m.picker.Cursor = 0
		if m.picker.FilterText != "" {
			// The first match below the ticked rows, so Space ticks what was
			// typed for.
			if top := len(m.picker.tickedCharts()); top < len(m.picker.Matches) {
				m.picker.Cursor = top
			}
		}
		m.picker.Offset = 0
		m.picker.ScrollTo(m.picker.Cursor, chartBlockHeight(m.picker, dialog.ListHeight(m.height)))
	}
	return m, nil
}

// renderChartModal renders the centered picker: search line + result list +
// state line. The chrome is the shared dialog.Box — the box math here was
// character-identical to the atlas modal's and is no longer duplicated.
func renderChartModal(state chartPickerState, cfg Config, termW, termH int) string {
	// The cursor walks the readable charts only; an unreadable file is listed
	// but cannot be picked, so it is not a position.
	pos := listPosition{Cursor: state.Cursor}
	if state.loaded && state.loadErr == nil {
		pos.Total = len(state.Matches)
	}
	return dialog.Render(dialog.Box{
		Title:    dialog.TitleOpenChart,
		Fit:      state.fitOrNow(cfg),
		Position: popupCounter(&pos, cfg),
		Scroll: func(height int) (pane.Viewport, int) {
			block := chartBlockHeight(state, height)
			_, offset := state.WindowAt(state.Cursor, block)
			return pane.Viewport{Total: pos.Total, Visible: block, Offset: offset}, 0
		},
		FilterText: state.FilterText,
	}, cfg, termW, termH, popupChrome(cfg), func(width, height int) string {
		return renderChartResultList(state, cfg, width, height)
	})
}

// fitOrNow is the size the picker was given when it opened, or, before the
// first load has given it one, the size of what it shows now.
func (s chartPickerState) fitOrNow(cfg Config) dialog.Fit {
	f := s.fit
	if f.Width == 0 {
		f = chartFit(s, cfg)
	}
	if len(s.ticked) > 0 && cfg.UseColor() {
		// With colours on a tick takes two cells before the name. They are
		// added once a chart is ticked, so a tick never shortens a name.
		f.Width += 2
	}
	return f
}

// chartFit is the picker's size for everything the library holds: wide enough
// for the widest chart and for the unreadable files' lines up to a sentence;
// as tall as every chart and the unreadable files' block. The block keeps no
// more rows than the charts have, so a long list of unreadable files does not
// stretch the box (chartBlockHeight reserves them by the same rule).
func chartFit(state chartPickerState, cfg Config) dialog.Fit {
	switch {
	case state.loadErr != nil:
		w, rows := menu.FitMessage(fmt.Sprintf("(charts unavailable: %v)", state.loadErr), dialog.SearchLineWidth("Search: "), dialog.ProseWidth)
		return dialog.Fit{Width: w, Rows: rows}
	case !state.loaded:
		return dialog.Fit{Width: 1, Rows: 1} // "(loading charts…)"
	}
	fit := dialog.Fit{Width: 1}
	for _, c := range state.charts {
		_, refused := state.refused[c.Source.File]
		fit.Width = max(fit.Width, menu.RowWidth(chartRow(c, false, refused, state.now), cfg))
	}
	drops := chartDropLines(state.drops)
	for _, l := range drops {
		fit.Width = max(fit.Width, min(ansi.StringWidth(l), dialog.ProseWidth))
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
	if len(state.refused) > 0 {
		fit.Rows += refusalRows
		// Wide enough that a reason fits its rows, as a sentence goes.
		for _, why := range state.refused {
			fit.Width = max(fit.Width, min(ansi.StringWidth(why)/(refusalRows-1)+8, dialog.ProseWidth))
		}
	}
	return fit
}

// refusalRows is what the reason a chart does not open takes under the list
// while the cursor is on it: a rule and up to three lines.
const refusalRows = 4

// cursorRefusal is why the chart under the cursor does not open, or "".
func cursorRefusal(state chartPickerState) string {
	if state.Cursor < 0 || state.Cursor >= len(state.Matches) {
		return ""
	}
	return state.refused[state.Matches[state.Cursor].Source.File]
}

// refusalLines is the reason the chart under the cursor does not open, as the
// list shows it under its rows (the operator, 2026-11-15): a rule, then the
// reason after the red !, wrapped to the width.
func refusalLines(why string, cfg Config, width int) []string {
	out := []string{menu.Rule(cfg, width)}
	for i, l := range dialog.MessageLines(why, width-2) {
		if i == refusalRows-1 {
			break
		}
		lead := "  "
		if i == 0 {
			lead = term.RenderErrorMark("!", cfg) + " "
		}
		out = append(out, menu.Note(lead+l, width, cfg))
	}
	return out
}

// renderChartResultList lays the list out in two blocks: the pickable charts,
// then the files the library could not read. The unreadable block is never
// squeezed out by a long chart list — it keeps a reserved tail (at most half
// the rows when charts share the list), overflowing into a single counted line.
// The point is the hard rule below: an astrologer whose files failed to parse
// must never be told they have no charts.
func renderChartResultList(state chartPickerState, cfg Config, width, height int) string {
	if state.loadErr != nil {
		return menu.Message(fmt.Sprintf("(charts unavailable: %v)", state.loadErr), width, height)
	}
	if !state.loaded {
		return menu.Message("(loading charts…)", width, height)
	}

	block := chartBlockHeight(state, height)
	lines := make([]string, 0, height)
	lines = append(lines, chartRows(state, cfg, width, block)...)
	// The unreadable files keep the foot of the list, whatever the filter left.
	for len(lines) < block {
		lines = append(lines, menu.Note("", width, cfg))
	}
	if why := cursorRefusal(state); why != "" {
		lines = append(lines, refusalLines(why, cfg, width)...)
		for len(lines) < block+refusalRows {
			lines = append(lines, menu.Note("", width, cfg))
		}
	}
	lines = append(lines, chartDropBlock(fitDropLines(chartDropLines(state.drops), height-len(lines)), cfg, width)...)
	return menu.Block(lines, width, height)
}

// chartBlockHeight is how many of the list's rows the pickable charts get: the
// rest is reserved for the unreadable files, at most half the list when charts
// share it, and never all of it. Drawing and navigating both ask this, so the
// window the cursor scrolls is the window that is painted.
func chartBlockHeight(state chartPickerState, height int) int {
	if cursorRefusal(state) != "" && height > refusalRows {
		height -= refusalRows
	}
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
		return []string{menu.Note("(no saved charts yet)", width, cfg)}
	}
	if len(state.Matches) == 0 {
		return []string{menu.Note("(no matches)", width, cfg)}
	}

	// The window scrolls to follow the cursor; without it the cursor walks
	// past the bottom of the list and Enter picks a chart nobody can see.
	visible, offset := state.WindowAt(state.Cursor, height)
	lines := make([]string, 0, height)
	for i, c := range visible {
		_, refused := state.refused[c.Source.File]
		lines = append(lines, menu.Row(withTick(chartRow(c, offset+i == state.Cursor, refused, state.now), state.isTicked(c)), cfg, width))
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
		row = menu.Note(row, width, cfg)
		if cfg.UseColor() {
			row = term.ChartDropStyle.Render(row)
		}
		out = append(out, row)
	}
	return out
}

// chartRow is one library entry: the chart's name on the left, and its place
// and birth date pinned to the right edge so the dates line up down the list
// (the specification C7). A missing place or date is simply absent.
func chartRow(c store.NamedChart, cursor, refused bool, now time.Time) menu.RowSpec {
	tail := ""
	if age, ok := chartAge(c, now); ok {
		tail = fmt.Sprintf(" (%d)", age)
	}
	return menu.RowSpec{
		Left:     c.Name,
		LeftTail: tail,
		Right:    []string{c.Place.Display(), chartDate(c)},
		Cursor:   cursor,
		Alert:    refused,
	}
}

// chartAge is the completed years from the chart's stated civil date to
// now's date. The stated date is read as Astrolog reads it, Julian before 15
// October 1582, and compared on the same calendar as now: a Julian birthday
// falls ten or more days later in Go's. A chart with no usable date, a zero
// now, or a date after now has no age.
func chartAge(c store.NamedChart, now time.Time) (int, bool) {
	if now.IsZero() || chartDate(c) == "" {
		return 0, false
	}
	by, bm, bd, ok := lookup.GregorianDay(c.Year, c.Month, c.Day)
	if !ok {
		return 0, false
	}
	y, m, d := now.Date()
	age := y - by
	if m < bm || (m == bm && d < bd) {
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

// openTickedCharts is Enter with charts ticked: those open, in the order they
// were ticked, and only those, wherever the cursor is. A chart that is open
// already is left out without a word. The charts that ask for a title wait
// until the others are open (nextTicked). When every ticked chart is open
// already, the last one ticked is picked as a single Enter would.
func openTickedCharts(m Model) (tea.Model, tea.Cmd) {
	ticked := m.picker.tickedCharts()
	if m.slots == nil {
		return applyChartPicked(m, ChartPickedMsg{Chart: ticked[len(ticked)-1]})
	}
	m.picker.queue, m.picker.late = nil, nil
	for _, c := range ticked {
		switch {
		case m.chartIsOpen(c):
		case needsTitle(c):
			m.picker.late = append(m.picker.late, c)
		default:
			m.picker.queue = append(m.picker.queue, c)
		}
	}
	if len(m.picker.queue)+len(m.picker.late) == 0 {
		return applyChartPicked(m, ChartPickedMsg{Chart: ticked[len(ticked)-1]})
	}
	m, cmd, _ := nextTicked(m)
	return m, cmd
}

// chartIsOpen is whether a slot holds the chart already.
func (m Model) chartIsOpen(c store.NamedChart) bool {
	return m.slots != nil && m.slots.FindAnchored(store.SessionChart{Source: c.Source}) >= 0
}

// nextTicked starts the next ticked chart still to open: the next one that
// opens as it is, then, once none is left, the next that asks for a title
// first. It reports whether it started one.
func nextTicked(m Model) (Model, tea.Cmd, bool) {
	for len(m.picker.queue) > 0 {
		c := m.picker.queue[0]
		m.picker.queue = m.picker.queue[1:]
		if m.chartIsOpen(c) {
			continue
		}
		m, cmd := resolvePick(m, c)
		return m, cmd, true
	}
	for len(m.picker.late) > 0 {
		c := m.picker.late[0]
		m.picker.late = m.picker.late[1:]
		if m.chartIsOpen(c) {
			continue
		}
		m = openTitleForm(m, c)
		m.title.batch = true
		return m, nil, true
	}
	return m, nil, false
}

// withTick is a row with its tick: a ✓ in the cursor column, or before the name
// with colours on.
func withTick(r menu.RowSpec, ticked bool) menu.RowSpec {
	r.Ticked = ticked
	return r
}
