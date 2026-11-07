package menu

// radio_list.go is the one-choice list: a cursor that walks the rows, and
// exactly one selected row that moves only when it is asked to. The two are
// independent on purpose. A checklist (fuzzy_checklist.go, the Aspects and
// Bodies choosers) holds a set of ticks and is not bent into this shape; a
// radio list holds a single index, so "two rows are selected" cannot be
// represented at all.
//
// It names no domain concept, so the next popup that offers one choice out of
// a few binds the same type.

// RadioList is the state of a one-choice list over n rows.
type RadioList struct {
	N int
	// Cursor is the row the operator is looking at.
	Cursor int
	// Selected is the one chosen row, -1 only for a list with no rows.
	Selected int
	// Offset is the first visible row, kept for the paging rule.
	Offset int
}

// NewRadioList is a list of n rows with row selected chosen and the cursor on
// it. selected is clamped into the list.
func NewRadioList(n, selected int) RadioList {
	r := RadioList{N: max(n, 0), Selected: -1}
	if r.N == 0 {
		return r
	}
	r.Selected = min(max(selected, 0), r.N-1)
	r.Cursor = r.Selected
	return r
}

// IsSelected reports whether row i is the chosen one.
func (r RadioList) IsSelected(i int) bool { return i == r.Selected && r.N > 0 }

// MoveUp and moveDown walk the cursor without wrapping. They never touch the
// selection.
func (r *RadioList) MoveUp() {
	if r.Cursor > 0 {
		r.Cursor--
	}
}

func (r *RadioList) MoveDown() {
	if r.Cursor+1 < r.N {
		r.Cursor++
	}
}

// SelectAt makes row i the one chosen row, clamped into the list; the row that
// was chosen before is no longer. The cursor stays where it is.
func (r *RadioList) SelectAt(i int) {
	if r.N == 0 {
		return
	}
	r.Selected = min(max(i, 0), r.N-1)
}

// SelectCursor chooses the row the cursor is on.
func (r *RadioList) SelectCursor() { r.SelectAt(r.Cursor) }

// Page applies a paging key by the one rule (PageCursor, cursor_page.go) over a
// window of visible rows. The selection stays.
func (r *RadioList) Page(op PageOp, visible int) {
	if r.N == 0 {
		return
	}
	r.Cursor, r.Offset = PageCursor(r.Cursor, r.Offset, r.N, visible, op)
}
