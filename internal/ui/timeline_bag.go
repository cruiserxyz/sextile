package ui

import "github.com/cruiserxyz/sextile/internal/ui/panel"

// resolveTimelineBag lifts the Details frame's timeline side into the one value
// panel.RenderTimeline reads: each row's mark resolved here, where timelineMark
// lives, and the cursor dropped while the tab waits (Details stays attached and
// its border focused: a wait is not a detach). focused is the border's own fact.
func resolveTimelineBag(frame detailsFrame, focused bool, innerW, innerH int, cfg Config) panel.TimelineBag {
	marked := make([]bool, len(frame.rows))
	for i, r := range frame.rows {
		marked[i] = frame.marks[markOf(r)]
	}
	return panel.TimelineBag{
		Rows:      frame.rows,
		Marked:    marked,
		AnyMarked: len(frame.marks) > 0,
		Nearest:   frame.nearestRow(),
		// No cursor row is drawn while the filter line is open: it only types.
		Attached:  frame.state.attached && !frame.noCursor && !frame.search.Open,
		Focused:   focused,
		Cursor:    frame.state.cursor,
		RowOffset: frame.state.rowOffset,
		Search:    frame.search,
		Dim:       frame.waiting,
		InnerW:    innerW,
		InnerH:    innerH,
		Style:     cfg,
	}
}
