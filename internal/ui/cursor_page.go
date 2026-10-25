package ui

// cursor_page.go is the one paging rule for every cursor in the program
// (the page-cursor specification, decision 1). The Page and Home/End keys move
// the cursor first, to the edge of the window it is in, and only then move the
// window, by a page of the window minus one row. Every cursor owner (the Chart
// and Bodies panels, the attached timeline, the Log console and the filtered
// popups) calls pageCursor with its own total and the number of cursor rows its
// render shows, then stores both results. No surface pages by hand, which is
// what TestEveryCursorOwnerPagesCursorFirst holds them to.

// pageOp is one paging key.
type pageOp int

const (
	pageUp pageOp = iota
	pageDown
	pageHome
	pageEnd
)

// pageCursor applies op to a cursor that sits in a window of visible rows over
// total rows whose first row is offset, and returns the new cursor and offset.
//
// The inputs are normalised first, so a stale or impossible pair never leaks
// out: visible is at least 1, an empty list is (0, 0), the offset is held in
// [0, max(total-visible, 0)], the cursor in [0, total-1], and the offset moves
// the least amount that puts the cursor inside the window.
//
// Page down: a cursor above the window's last row goes to that row and the
// window stays (the first press does not change content); a cursor on the last
// row moves the window by visible-1, so the row that was at the edge stays in
// view as one row of context, and the cursor goes to the new last row. Page up
// mirrors it. A one-row window moves by one, so it never sticks.
//
// Paging sets the offset exactly: the margin the single-step movers keep
// (scrollToShow) is not applied. The post-condition holds for every op and
// every input: offset <= cursor <= offset+visible-1 and
// 0 <= offset <= max(total-visible, 0).
func pageCursor(cursor, offset, total, visible int, op pageOp) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	visible = max(visible, 1)
	maxOffset := max(total-visible, 0)
	offset = min(max(offset, 0), maxOffset)
	cursor = min(max(cursor, 0), total-1)
	if cursor < offset {
		offset = cursor
	}
	if cursor > offset+visible-1 {
		offset = cursor - visible + 1
	}
	step := max(visible-1, 1)

	switch op {
	case pageDown:
		if last := min(offset+visible-1, total-1); cursor < last {
			cursor = last
		} else {
			offset = min(offset+step, maxOffset)
			cursor = min(offset+visible-1, total-1)
		}
	case pageUp:
		if cursor > offset {
			cursor = offset
		} else {
			offset = max(offset-step, 0)
			cursor = offset
		}
	case pageHome:
		cursor, offset = 0, 0
	case pageEnd:
		cursor, offset = total-1, maxOffset
	}
	return cursor, offset
}
