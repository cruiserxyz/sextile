package ui

import "testing"

// A1: the one paging rule, pinned as a table. Every case names the window it is
// in: total rows, visible rows, the first row shown (offset) and the cursor.
func TestPageCursorRule(t *testing.T) {
	type in struct{ cursor, offset, total, visible int }
	type out struct{ cursor, offset int }
	cases := []struct {
		name string
		op   pageOp
		in   in
		want out
	}{
		// Page down: first to the window's last row, content unchanged.
		{"down mid-window goes to the last row", pageDown, in{13, 10, 40, 10}, out{19, 10}},
		{"down from the first row goes to the last row", pageDown, in{10, 10, 40, 10}, out{19, 10}},
		// Then a page: the window moves by visible-1, the cursor sits on the new last row.
		{"down at the last row pages the window", pageDown, in{19, 10, 40, 10}, out{28, 19}},
		{"down clamps the window at the end", pageDown, in{37, 28, 40, 10}, out{39, 30}},
		{"down at the very end stays", pageDown, in{39, 30, 40, 10}, out{39, 30}},
		// Page up mirrors it.
		{"up mid-window goes to the first row", pageUp, in{16, 10, 40, 10}, out{10, 10}},
		{"up from the last row goes to the first row", pageUp, in{19, 10, 40, 10}, out{10, 10}},
		{"up at the first row pages the window", pageUp, in{10, 10, 40, 10}, out{1, 1}},
		{"up clamps the window at the top", pageUp, in{4, 4, 40, 10}, out{0, 0}},
		{"up at the very top stays", pageUp, in{0, 0, 40, 10}, out{0, 0}},
		// Home and End.
		{"home", pageHome, in{25, 20, 40, 10}, out{0, 0}},
		{"end", pageEnd, in{3, 0, 40, 10}, out{39, 30}},
		// A list that fits: only the cursor moves.
		{"fits: down goes to the last row", pageDown, in{1, 0, 5, 10}, out{4, 0}},
		{"fits: down at the last row stays", pageDown, in{4, 0, 5, 10}, out{4, 0}},
		{"fits: up goes to the first row", pageUp, in{3, 0, 5, 10}, out{0, 0}},
		{"fits: end", pageEnd, in{0, 0, 5, 10}, out{4, 0}},
		{"fits: home", pageHome, in{3, 0, 5, 10}, out{0, 0}},
		{"exactly full: down goes to the last row", pageDown, in{0, 0, 10, 10}, out{9, 0}},
		// Nothing to page.
		{"empty list down", pageDown, in{0, 0, 0, 10}, out{0, 0}},
		{"empty list end", pageEnd, in{7, 3, 0, 10}, out{0, 0}},
		{"negative total", pageUp, in{2, 2, -4, 10}, out{0, 0}},
		// A window with no height reads as one row, and a one-row window still moves.
		{"zero height: down moves one row", pageDown, in{2, 2, 10, 0}, out{3, 3}},
		{"zero height: up moves one row", pageUp, in{2, 2, 10, 0}, out{1, 1}},
		{"zero height: end", pageEnd, in{2, 2, 10, 0}, out{9, 9}},
		{"one row: down from the last row stops", pageDown, in{9, 9, 10, 1}, out{9, 9}},
		// An impossible input is normalised before the op, never leaked.
		{"cursor 100 offset -1, home", pageHome, in{100, -1, 20, 5}, out{0, 0}},
		{"cursor 100 offset -1, down", pageDown, in{100, -1, 20, 5}, out{19, 15}},
		{"cursor 100 offset -1, up", pageUp, in{100, -1, 20, 5}, out{15, 15}},
		{"negative cursor, down", pageDown, in{-9, 3, 20, 5}, out{4, 0}},
		{"offset beyond the end, down", pageDown, in{2, 99, 20, 5}, out{6, 2}},
	}
	for _, c := range cases {
		cur, off := pageCursor(c.in.cursor, c.in.offset, c.in.total, c.in.visible, c.op)
		if cur != c.want.cursor || off != c.want.offset {
			t.Errorf("%s: pageCursor%+v = (%d, %d), want (%d, %d)", c.name, c.in, cur, off, c.want.cursor, c.want.offset)
		}
	}
}

// A1: the post-condition of the rule, for every window the helper can be
// handed, valid or not: the cursor is inside the window, and the window is
// inside the list.
func TestPageCursorKeepsTheCursorInSight(t *testing.T) {
	ops := []pageOp{pageUp, pageDown, pageHome, pageEnd}
	for total := 0; total <= 40; total++ {
		for visible := 0; visible <= 12; visible++ {
			for cursor := -2; cursor <= total+2; cursor++ {
				for offset := -2; offset <= total+2; offset++ {
					for _, op := range ops {
						cur, off := pageCursor(cursor, offset, total, visible, op)
						if total <= 0 {
							if cur != 0 || off != 0 {
								t.Fatalf("pageCursor(%d,%d,%d,%d,%v) = (%d,%d), want (0,0)", cursor, offset, total, visible, op, cur, off)
							}
							continue
						}
						v := max(visible, 1)
						if off < 0 || off > max(total-v, 0) || cur < off || cur > off+v-1 || cur > total-1 {
							t.Fatalf("pageCursor(cursor=%d, offset=%d, total=%d, visible=%d, op=%v) = (%d, %d): cursor outside its window",
								cursor, offset, total, visible, op, cur, off)
						}
					}
				}
			}
		}
	}
}
