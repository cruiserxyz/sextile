package ui

import "testing"

// selectedCount is how many rows of r report themselves chosen.
func selectedCount(r radioList) int {
	n := 0
	for i := -2; i < r.n+2; i++ {
		if r.isSelected(i) {
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
	r := newRadioList(4, 0)
	if r.selected != 0 || r.cursor != 0 || selectedCount(r) != 1 {
		t.Fatalf("a new list: selected %d, cursor %d, %d chosen; want row 0 chosen with the cursor on it", r.selected, r.cursor, selectedCount(r))
	}

	// Cursor moves leave the selection where it was.
	r.moveDown()
	r.moveDown()
	if r.cursor != 2 || r.selected != 0 || !r.isSelected(0) || r.isSelected(2) {
		t.Fatalf("after two moves down: cursor %d, selected %d; the cursor moved the selection or stayed put", r.cursor, r.selected)
	}

	// Choosing moves the one mark: the old row is no longer chosen.
	r.selectCursor()
	if r.selected != 2 || r.isSelected(0) || !r.isSelected(2) || selectedCount(r) != 1 {
		t.Fatalf("after choosing row 2: selected %d, row 0 chosen %v, row 2 chosen %v, %d chosen; want only row 2", r.selected, r.isSelected(0), r.isSelected(2), selectedCount(r))
	}

	// And the cursor is free again afterwards.
	r.moveUp()
	r.moveUp()
	r.moveUp()
	if r.cursor != 0 || r.selected != 2 {
		t.Fatalf("after moving up past the top: cursor %d, selected %d; want 0 and 2 (no wrap, selection kept)", r.cursor, r.selected)
	}
	for i := 0; i < 9; i++ {
		r.moveDown()
	}
	if r.cursor != 3 || r.selected != 2 {
		t.Fatalf("after moving down past the end: cursor %d, selected %d; want 3 and 2", r.cursor, r.selected)
	}

	// Out of range is clamped onto the list, and still exactly one is chosen.
	r.selectAt(99)
	if r.selected != 3 || selectedCount(r) != 1 {
		t.Fatalf("selectAt(99): selected %d, %d chosen; want the last row alone", r.selected, selectedCount(r))
	}
	r.selectAt(-7)
	if r.selected != 0 || selectedCount(r) != 1 {
		t.Fatalf("selectAt(-7): selected %d, %d chosen; want the first row alone", r.selected, selectedCount(r))
	}
	if over := newRadioList(4, 9); over.selected != 3 || over.cursor != 3 || selectedCount(over) != 1 {
		t.Fatalf("newRadioList(4, 9): selected %d, cursor %d; want both clamped to 3", over.selected, over.cursor)
	}
	if under := newRadioList(4, -1); under.selected != 0 || selectedCount(under) != 1 {
		t.Fatalf("newRadioList(4, -1): selected %d; want 0", under.selected)
	}

	// Paging moves the cursor by the shared rule and never the selection.
	r = newRadioList(4, 1)
	r.page(pageEnd, 4)
	if r.cursor != 3 || r.selected != 1 {
		t.Fatalf("End: cursor %d, selected %d; want 3 and 1", r.cursor, r.selected)
	}
	r.page(pageHome, 4)
	if r.cursor != 0 || r.selected != 1 {
		t.Fatalf("Home: cursor %d, selected %d; want 0 and 1", r.cursor, r.selected)
	}

	// An empty list has nothing to choose and nothing to break.
	var empty radioList
	empty.selectAt(3)
	empty.moveDown()
	empty.page(pageEnd, 4)
	if selectedCount(empty) != 0 {
		t.Fatalf("an empty list reports %d chosen rows", selectedCount(empty))
	}
}
