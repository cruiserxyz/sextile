// Package menu is the list machinery every picker and checklist draws with: the
// filtered list, the radio list, the row and its state marks, the page cursor,
// and the scroll-to-show rule. It knows nothing of charts or the model; the
// terminal is described by term.Style.
package menu

import "github.com/cruiserxyz/sextile/internal/ui/pane"

// cursor_page.go is the one paging rule for every cursor in the program
// (the page-cursor specification, decision 1). The Page and Home/End keys move
// the cursor first, to the edge of the window it is in, and only then move the
// window, by a page of the window minus one row. Every cursor owner (the Chart
// and Bodies panels, the attached timeline, the Log console and the filtered
// popups) calls PageCursor with its own total and the number of cursor rows its
// render shows, then stores both results. No surface pages by hand, which is
// what TestEveryCursorOwnerPagesCursorFirst holds them to.

// PageOp is one paging key.
type PageOp int

const (
	PageUp PageOp = iota
	PageDown
	PageHome
	PageEnd
)

// PageCursor applies op to a cursor that sits in a window of visible rows over
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
// (ScrollToShow) is not applied. The post-condition holds for every op and
// every input: offset <= cursor <= offset+visible-1 and
// 0 <= offset <= max(total-visible, 0).
func PageCursor(cursor, offset, total, visible int, op PageOp) (int, int) {
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
	case PageDown:
		if last := min(offset+visible-1, total-1); cursor < last {
			cursor = last
		} else {
			offset = min(offset+step, maxOffset)
			cursor = min(offset+visible-1, total-1)
		}
	case PageUp:
		if cursor > offset {
			cursor = offset
		} else {
			offset = max(offset-step, 0)
			cursor = offset
		}
	case PageHome:
		cursor, offset = 0, 0
	case PageEnd:
		cursor, offset = total-1, maxOffset
	}
	return cursor, offset
}

// ScrollToShow nudges the window the least it can to keep line inside it with
// ScrollMargin rows of context above and below (specs/
// focus-and-nearest-event.md rule 6): a cursor moving inside the margins does
// not scroll the window, one that would come within the margin of an edge
// scrolls it. A window too short to hold both margins plus a row centres on
// the line instead, because it would otherwise scroll on every step. The
// popups' fuzzy lists, the Log console, Keybindings and the timeline all use this.
func ScrollToShow(offset, line, total, visible int) int {
	if visible <= 0 {
		return offset
	}
	if total <= visible {
		return 0
	}
	if visible < 2*ScrollMargin+1 {
		return pane.ClampOffset(line-visible/2, total, visible)
	}
	if line-offset < ScrollMargin {
		offset = line - ScrollMargin
	}
	if line-offset > visible-1-ScrollMargin {
		offset = line - (visible - 1 - ScrollMargin)
	}
	return pane.ClampOffset(offset, total, visible)
}
