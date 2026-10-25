package ui

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
