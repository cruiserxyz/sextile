package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/popup"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// the specification: typing in the Aspects or Bodies
// popup's filter puts the cursor on the first matching item below the groups,
// so Space ticks what was typed for without moving the cursor first.

func typeIntoPopup[S any](st S, s string, press func(S, tea.KeyMsg) S) S {
	for _, r := range s {
		st = press(st, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return st
}

func TestAspectsPopupFilterLandsOnTheFirstMatch(t *testing.T) {
	press := func(st popup.Aspects, k tea.KeyMsg) popup.Aspects {
		st, _ = updateAspectModalKey(st, k, testListHeight)
		return st
	}
	st := openTest(recipe.AspectPolicy{})
	if st.Cursor >= popup.FamilyRows() {
		t.Fatalf("fixture: the cursor opens at %d, want it in the group rows", st.Cursor)
	}
	st = typeIntoPopup(st, "inco", press)
	if len(st.Matches) == 0 || st.Matches[0].Kind.Name != "Inconjunct" {
		t.Fatalf("fixture: matches %v, want Inconjunct first", st.Matches)
	}
	if st.Cursor != popup.FamilyRows() {
		t.Fatalf("after typing the cursor is on row %d, want %d, the first match", st.Cursor, popup.FamilyRows())
	}
	was := st.Selected[6]
	st = press(st, tea.KeyMsg{Type: tea.KeySpace})
	if st.Selected[6] == was {
		t.Error("Space did not tick Inconjunct, the item typed for")
	}
	// Nothing matching: the cursor parks on the last family row, as before.
	st = typeIntoPopup(st, "zzz", press)
	if len(st.Matches) != 0 || st.Cursor != popup.FamilyRows()-1 {
		t.Errorf("no match: matches %d, cursor %d of %d rows", len(st.Matches), st.Cursor, st.TotalRows())
	}
	// Deleting the filter back to empty: the list's first row.
	for range "incozzz" {
		st = press(st, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	if st.FilterText != "" || st.Cursor != popup.FamilyRows() {
		t.Errorf("emptied filter %q: cursor %d, want %d, the list's first row", st.FilterText, st.Cursor, popup.FamilyRows())
	}
}

func TestBodiesPopupFilterLandsOnTheFirstMatch(t *testing.T) {
	press := func(st popup.Body, k tea.KeyMsg) popup.Body {
		st, _ = updateBodyModalKey(st, k, testListHeight)
		return st
	}
	st := openBodyTest(recipe.ObjectPolicy{}, nil)
	st = typeIntoPopup(st, "cere", press)
	if len(st.Matches) == 0 || st.Matches[0].Name != "Ceres" {
		t.Fatalf("fixture: matches %v, want Ceres first", st.Matches)
	}
	if st.Cursor != popup.GroupRows() {
		t.Fatalf("after typing the cursor is on row %d, want %d, the first match", st.Cursor, popup.GroupRows())
	}
	was := st.Selected[12]
	st = press(st, tea.KeyMsg{Type: tea.KeySpace})
	if st.Selected[12] == was {
		t.Error("Space did not tick Ceres, the item typed for")
	}
}
