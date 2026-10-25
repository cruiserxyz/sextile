package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
)

// The Details report tab's `/` filter (the specification). A report
// is a list only by declaring a row grammar, which astrolog owns (reached
// through lookup); this file only filters the line spans it is handed. The
// header above the list and the footer below it always stay as printed.

// detailsGrammar is the row grammar the report on screen declares: the
// Aspects panel's own list is own-chart rows, its pair tabs are comparison
// rows; a Bodies report is a single-chart list, or a comparison one while the
// panel compares; the Chart report and the other tabs declare nothing.
func (m Model) detailsGrammar() lookup.ReportGrammar {
	if m.activeTabID(PanelDetails) != TabDetailsReport {
		return lookup.GrammarNone
	}
	switch m.focused {
	case PanelAspects:
		if isAspectsPairTab(m.activeTabID(PanelAspects)) {
			return lookup.GrammarCrossAspects
		}
		return lookup.GrammarAspects
	case PanelBodies:
		if m.deriveScene().pair != nil {
			return lookup.GrammarCrossAspects
		}
		return lookup.GrammarAspects
	}
	return lookup.GrammarNone
}

// detailsReportFilterable is whether `/` has a list to act on: the report tab
// is active and its text holds exactly one list under its grammar.
func (m Model) detailsReportFilterable() bool {
	_, ok := lookup.ReportListSpans(m.details.body, m.detailsGrammar())
	return ok
}

// detailsFilterable is where Details' `/` acts: the timeline, or a report with
// a list.
func (m Model) detailsFilterable() bool {
	return m.detailsOnTimeline() || m.detailsReportFilterable()
}

// detailsReportFilterBlocked is the report tab waiting for new content: `/`
// and the query line's keys do nothing, as the timeline's row keys do not
// (the specification; acceptance 8).
func (m Model) detailsReportFilterBlocked() bool {
	return m.activeTabID(PanelDetails) == TabDetailsReport && m.detailsTabWaiting(TabDetailsReport)
}

// detailsReportSearchLine says whether the report's last row is the query
// line: open for typing (only while Details has the keyboard) or showing a
// kept query, and only where the text has a list.
func (m Model) detailsReportSearchLine() bool {
	return ((m.details.searching && m.details.attached) || m.details.query != "") && m.detailsReportFilterable()
}

// detailsReportSearchOpen is the line open and owning the keyboard.
func (m Model) detailsReportSearchOpen() bool {
	return m.details.searching && m.details.attached && m.detailsReportSearchLine()
}

// detailsViewLines is what the report tab draws: the lines the panel scrolls,
// and, when the query line is there, how many records it keeps of how many.
type detailsViewLines struct {
	lines        []string
	line         bool // the query line takes the panel's last row
	shown, total int
}

// detailsView is the report's lines under the query. With no query it is the
// report as printed, byte for byte.
func (m Model) detailsView() detailsViewLines {
	lines := detailsReportLines(m.details.body)
	spans, ok := lookup.ReportListSpans(m.details.body, m.detailsGrammar())
	if !ok {
		return detailsViewLines{lines: lines}
	}
	v := detailsViewLines{lines: lines, line: m.detailsReportSearchLine(), shown: len(spans.Records), total: len(spans.Records)}
	if m.details.query == "" {
		return v
	}
	recs := make([]string, len(spans.Records))
	for i, r := range spans.Records {
		recs[i] = strings.Join(lines[r.Start:r.End], " ")
	}
	kept, n, total := listFilter{query: m.details.query}.apply(recs, 0)
	out := make([]string, 0, len(lines))
	out = append(out, lines[spans.Header.Start:spans.Header.End]...)
	if n == 0 && total > 0 {
		out = append(out, fmt.Sprintf("No rows match %q.", m.details.query))
	}
	for _, i := range kept {
		out = append(out, lines[spans.Records[i].Start:spans.Records[i].End]...)
	}
	out = append(out, lines[spans.Footer.Start:spans.Footer.End]...)
	v.lines, v.shown, v.total = out, n, total
	return v
}

// detailsReportHeight is how many report lines the panel can show: its inner
// height, less the query line when it is there.
func (m Model) detailsReportHeight() int {
	h := m.detailsInnerHeight()
	if h > 0 && m.detailsView().line {
		h--
	}
	return h
}

// openDetailsReportSearch is `/` on a report with a list: the line opens,
// prefilled with the kept query.
func (m Model) openDetailsReportSearch() Model {
	m.details.searching = true
	return m.rememberSearch(m.details.query).clampDetailsOffset()
}

// updateDetailsReportSearchKey is the line's own key state: letters, Space and
// Backspace edit the query; Enter closes it and the query stays, Esc closes it
// and the query and scroll it opened with come back. Nothing else is read.
func (m Model) updateDetailsReportSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.detailsReportFilterBlocked() {
		return m, nil
	}
	switch matchBinding(msg, reportSearchBindings()) {
	case BindingScrollUp:
		return m.scrollDetails(-detailsScrollStep), nil
	case BindingScrollDown:
		return m.scrollDetails(detailsScrollStep), nil
	}
	switch msg.Type {
	case tea.KeyEnter:
		m.details.searching = false
		return m.clampDetailsOffset(), nil
	case tea.KeyEsc:
		m.details.searching = false
		m.details.query = m.searchBefore.query
		m.details.scrollOffset = m.searchBefore.scrollOffset
		return m.clampDetailsOffset(), nil
	}
	l := fuzzyList[struct{}]{filterText: m.details.query}
	if l.typeKey(msg) {
		m.details.query = l.filterText
		m = m.clampDetailsOffset()
	}
	return m, nil
}
