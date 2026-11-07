package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"testing"
)

// selectedCount is how many rows of r report themselves chosen.
func selectedCount(r menu.RadioList) int {
	n := 0
	for i := -2; i < r.N+2; i++ {
		if r.IsSelected(i) {
			n++
		}
	}
	return n
}

// H1: a radio list holds exactly one chosen row. Choosing a row un-chooses the
// one before it, the cursor walks without choosing, and an index outside the
// list is clamped onto it rather than leaving nothing (or something invisible)
// chosen.
func TestRadioListSelectsExactlyOne(t *testing.T) {
	r := menu.NewRadioList(4, 0)
	if r.Selected != 0 || r.Cursor != 0 || selectedCount(r) != 1 {
		t.Fatalf("a new list: selected %d, cursor %d, %d chosen; want row 0 chosen with the cursor on it", r.Selected, r.Cursor, selectedCount(r))
	}

	// Cursor moves leave the selection where it was.
	r.MoveDown()
	r.MoveDown()
	if r.Cursor != 2 || r.Selected != 0 || !r.IsSelected(0) || r.IsSelected(2) {
		t.Fatalf("after two moves down: cursor %d, selected %d; the cursor moved the selection or stayed put", r.Cursor, r.Selected)
	}

	// Choosing moves the one mark: the old row is no longer chosen.
	r.SelectCursor()
	if r.Selected != 2 || r.IsSelected(0) || !r.IsSelected(2) || selectedCount(r) != 1 {
		t.Fatalf("after choosing row 2: selected %d, row 0 chosen %v, row 2 chosen %v, %d chosen; want only row 2", r.Selected, r.IsSelected(0), r.IsSelected(2), selectedCount(r))
	}

	// And the cursor is free again afterwards.
	r.MoveUp()
	r.MoveUp()
	r.MoveUp()
	if r.Cursor != 0 || r.Selected != 2 {
		t.Fatalf("after moving up past the top: cursor %d, selected %d; want 0 and 2 (no wrap, selection kept)", r.Cursor, r.Selected)
	}
	for i := 0; i < 9; i++ {
		r.MoveDown()
	}
	if r.Cursor != 3 || r.Selected != 2 {
		t.Fatalf("after moving down past the end: cursor %d, selected %d; want 3 and 2", r.Cursor, r.Selected)
	}

	// Out of range is clamped onto the list, and still exactly one is chosen.
	r.SelectAt(99)
	if r.Selected != 3 || selectedCount(r) != 1 {
		t.Fatalf("selectAt(99): selected %d, %d chosen; want the last row alone", r.Selected, selectedCount(r))
	}
	r.SelectAt(-7)
	if r.Selected != 0 || selectedCount(r) != 1 {
		t.Fatalf("selectAt(-7): selected %d, %d chosen; want the first row alone", r.Selected, selectedCount(r))
	}
	if over := menu.NewRadioList(4, 9); over.Selected != 3 || over.Cursor != 3 || selectedCount(over) != 1 {
		t.Fatalf("newRadioList(4, 9): selected %d, cursor %d; want both clamped to 3", over.Selected, over.Cursor)
	}
	if under := menu.NewRadioList(4, -1); under.Selected != 0 || selectedCount(under) != 1 {
		t.Fatalf("newRadioList(4, -1): selected %d; want 0", under.Selected)
	}

	// Paging moves the cursor by the shared rule and never the selection.
	r = menu.NewRadioList(4, 1)
	r.Page(menu.PageEnd, 4)
	if r.Cursor != 3 || r.Selected != 1 {
		t.Fatalf("End: cursor %d, selected %d; want 3 and 1", r.Cursor, r.Selected)
	}
	r.Page(menu.PageHome, 4)
	if r.Cursor != 0 || r.Selected != 1 {
		t.Fatalf("Home: cursor %d, selected %d; want 0 and 1", r.Cursor, r.Selected)
	}

	// An empty list has nothing to choose and nothing to break.
	var empty menu.RadioList
	empty.SelectAt(3)
	empty.MoveDown()
	empty.Page(menu.PageEnd, 4)
	if selectedCount(empty) != 0 {
		t.Fatalf("an empty list reports %d chosen rows", selectedCount(empty))
	}
}
