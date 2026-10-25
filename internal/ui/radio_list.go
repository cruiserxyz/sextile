package ui

// radio_list.go is the one-choice list: a cursor that walks the rows, and
// exactly one selected row that moves only when it is asked to. The two are
// independent on purpose. A checklist (fuzzy_checklist.go, the Aspects and
// Bodies choosers) holds a set of ticks and is not bent into this shape; a
// radio list holds a single index, so "two rows are selected" cannot be
// represented at all.
//
// It names no domain concept, so the next popup that offers one choice out of
// a few binds the same type.

// radioList is the state of a one-choice list over n rows.
type radioList struct {
	n int
	// cursor is the row the operator is looking at.
	cursor int
	// selected is the one chosen row, -1 only for a list with no rows.
	selected int
	// offset is the first visible row, kept for the paging rule.
	offset int
}

// newRadioList is a list of n rows with row selected chosen and the cursor on
// it. selected is clamped into the list.
func newRadioList(n, selected int) radioList {
	r := radioList{n: max(n, 0), selected: -1}
	if r.n == 0 {
		return r
	}
	r.selected = min(max(selected, 0), r.n-1)
	r.cursor = r.selected
	return r
}

// isSelected reports whether row i is the chosen one.
func (r radioList) isSelected(i int) bool { return i == r.selected && r.n > 0 }

// moveUp and moveDown walk the cursor without wrapping. They never touch the
// selection.
func (r *radioList) moveUp() {
	if r.cursor > 0 {
		r.cursor--
	}
}

func (r *radioList) moveDown() {
	if r.cursor+1 < r.n {
		r.cursor++
	}
}

// selectAt makes row i the one chosen row, clamped into the list; the row that
// was chosen before is no longer. The cursor stays where it is.
func (r *radioList) selectAt(i int) {
	if r.n == 0 {
		return
	}
	r.selected = min(max(i, 0), r.n-1)
}

// selectCursor chooses the row the cursor is on.
func (r *radioList) selectCursor() { r.selectAt(r.cursor) }

// page applies a paging key by the one rule (pageCursor, cursor_page.go) over a
// window of visible rows. The selection stays.
func (r *radioList) page(op pageOp, visible int) {
	if r.n == 0 {
		return
	}
	r.cursor, r.offset = pageCursor(r.cursor, r.offset, r.n, visible, op)
}
