package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"testing"
)

// A1: the one paging rule, pinned as a table. Every case names the window it is
// in: total rows, visible rows, the first row shown (offset) and the cursor.
func TestPageCursorRule(t *testing.T) {
	type in struct{ cursor, offset, total, visible int }
	type out struct{ cursor, offset int }
	cases := []struct {
		name string
		op   menu.PageOp
		in   in
		want out
	}{
		// Page down: first to the window's last row, content unchanged.
		{"down mid-window goes to the last row", menu.PageDown, in{13, 10, 40, 10}, out{19, 10}},
		{"down from the first row goes to the last row", menu.PageDown, in{10, 10, 40, 10}, out{19, 10}},
		// Then a page: the window moves by visible-1, the cursor sits on the new last row.
		{"down at the last row pages the window", menu.PageDown, in{19, 10, 40, 10}, out{28, 19}},
		{"down clamps the window at the end", menu.PageDown, in{37, 28, 40, 10}, out{39, 30}},
		{"down at the very end stays", menu.PageDown, in{39, 30, 40, 10}, out{39, 30}},
		// Page up mirrors it.
		{"up mid-window goes to the first row", menu.PageUp, in{16, 10, 40, 10}, out{10, 10}},
		{"up from the last row goes to the first row", menu.PageUp, in{19, 10, 40, 10}, out{10, 10}},
		{"up at the first row pages the window", menu.PageUp, in{10, 10, 40, 10}, out{1, 1}},
		{"up clamps the window at the top", menu.PageUp, in{4, 4, 40, 10}, out{0, 0}},
		{"up at the very top stays", menu.PageUp, in{0, 0, 40, 10}, out{0, 0}},
		// Home and End.
		{"home", menu.PageHome, in{25, 20, 40, 10}, out{0, 0}},
		{"end", menu.PageEnd, in{3, 0, 40, 10}, out{39, 30}},
		// A list that fits: only the cursor moves.
		{"fits: down goes to the last row", menu.PageDown, in{1, 0, 5, 10}, out{4, 0}},
		{"fits: down at the last row stays", menu.PageDown, in{4, 0, 5, 10}, out{4, 0}},
		{"fits: up goes to the first row", menu.PageUp, in{3, 0, 5, 10}, out{0, 0}},
		{"fits: end", menu.PageEnd, in{0, 0, 5, 10}, out{4, 0}},
		{"fits: home", menu.PageHome, in{3, 0, 5, 10}, out{0, 0}},
		{"exactly full: down goes to the last row", menu.PageDown, in{0, 0, 10, 10}, out{9, 0}},
		// Nothing to page.
		{"empty list down", menu.PageDown, in{0, 0, 0, 10}, out{0, 0}},
		{"empty list end", menu.PageEnd, in{7, 3, 0, 10}, out{0, 0}},
		{"negative total", menu.PageUp, in{2, 2, -4, 10}, out{0, 0}},
		// A window with no height reads as one row, and a one-row window still moves.
		{"zero height: down moves one row", menu.PageDown, in{2, 2, 10, 0}, out{3, 3}},
		{"zero height: up moves one row", menu.PageUp, in{2, 2, 10, 0}, out{1, 1}},
		{"zero height: end", menu.PageEnd, in{2, 2, 10, 0}, out{9, 9}},
		{"one row: down from the last row stops", menu.PageDown, in{9, 9, 10, 1}, out{9, 9}},
		// An impossible input is normalised before the op, never leaked.
		{"cursor 100 offset -1, home", menu.PageHome, in{100, -1, 20, 5}, out{0, 0}},
		{"cursor 100 offset -1, down", menu.PageDown, in{100, -1, 20, 5}, out{19, 15}},
		{"cursor 100 offset -1, up", menu.PageUp, in{100, -1, 20, 5}, out{15, 15}},
		{"negative cursor, down", menu.PageDown, in{-9, 3, 20, 5}, out{4, 0}},
		{"offset beyond the end, down", menu.PageDown, in{2, 99, 20, 5}, out{6, 2}},
	}
	for _, c := range cases {
		cur, off := menu.PageCursor(c.in.cursor, c.in.offset, c.in.total, c.in.visible, c.op)
		if cur != c.want.cursor || off != c.want.offset {
			t.Errorf("%s: pageCursor%+v = (%d, %d), want (%d, %d)", c.name, c.in, cur, off, c.want.cursor, c.want.offset)
		}
	}
}

// A1: the post-condition of the rule, for every window the helper can be
// handed, valid or not: the cursor is inside the window, and the window is
// inside the list.
func TestPageCursorKeepsTheCursorInSight(t *testing.T) {
	ops := []menu.PageOp{menu.PageUp, menu.PageDown, menu.PageHome, menu.PageEnd}
	for total := 0; total <= 40; total++ {
		for visible := 0; visible <= 12; visible++ {
			for cursor := -2; cursor <= total+2; cursor++ {
				for offset := -2; offset <= total+2; offset++ {
					for _, op := range ops {
						cur, off := menu.PageCursor(cursor, offset, total, visible, op)
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
