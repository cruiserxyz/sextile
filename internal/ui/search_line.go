package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ui/menu"
)

// searchBefore is what a `/` line found when it opened: the query it was
// prefilled with and where the list stood. Enter keeps what was typed; Esc puts
// this back (the specification). One line is open at a time,
// so one copy serves the timeline and the Details reports.
type searchBefore struct {
	query string
	// The timeline's row cursor and window, and the report's scroll.
	cursor, rowOffset int
	cursorAt          timelineMark
	cursorSet         bool
	scrollOffset      int
}

// rememberSearch records the state a line opens on.
func (m Model) rememberSearch(query string) Model {
	m.searchBefore = searchBefore{
		query:        query,
		cursor:       m.details.cursor,
		rowOffset:    m.details.rowOffset,
		cursorAt:     m.details.cursorAt,
		cursorSet:    m.details.cursorSet,
		scrollOffset: m.details.scrollOffset,
	}
	return m
}

// restoreTimelineCursor puts the timeline's row cursor and window back.
func (m Model) restoreTimelineCursor() Model {
	b := m.searchBefore
	m.details.cursor, m.details.rowOffset = b.cursor, b.rowOffset
	m.details.cursorAt, m.details.cursorSet = b.cursorAt, b.cursorSet
	return m
}

// filterAct is what a key did in an open Details filter line.
type filterAct int

const (
	filterIgnored filterAct = iota // a key the line does not read: arrows, paging
	filterEdited                   // the query changed
	filterNoop                     // read, and changed nothing
	filterClose                    // Enter, or Backspace on an empty query
	filterCancel                   // Esc
)

// filterLineKey is the one key reader of the three Details filter lines (the
// Timeline's, the Moon's and the report's). The open line takes typing,
// Backspace, Enter and Esc and nothing else: no arrow, no paging key and no
// pane scroll. It returns the query after the key.
func filterLineKey(msg tea.KeyMsg, query string) (string, filterAct) {
	switch msg.Type {
	case tea.KeyEnter:
		return query, filterClose
	case tea.KeyEsc:
		return query, filterCancel
	case tea.KeyBackspace, tea.KeyCtrlH:
		if query == "" {
			return query, filterClose
		}
	case tea.KeyUp, tea.KeyDown, tea.KeyPgUp, tea.KeyPgDown, tea.KeyCtrlPgUp, tea.KeyCtrlPgDown:
		return query, filterIgnored
	}
	l := menu.FuzzyList[struct{}]{FilterText: query}
	if l.TypeKey(msg) {
		return l.FilterText, filterEdited
	}
	return query, filterNoop
}

// clearDetailsFilters ends every Details filter: the Timeline's, the Moon's
// and the report's. A move of the floating chart in time or place does it, so
// the cursor keeps following the moment where it can be seen.
func (m Model) clearDetailsFilters() Model {
	any := m.details.query != "" || m.details.searching
	for _, k := range allLists {
		l := m.list(k)
		any = any || l.query != "" || l.searching
	}
	if !any {
		return m
	}
	for _, k := range allLists {
		l := m.list(k)
		l.query, l.searching = "", false
	}
	m.details.query, m.details.searching = "", false
	return m.placeTimelineCursor().clampTimelineCursor().clampDetailsOffset()
}
