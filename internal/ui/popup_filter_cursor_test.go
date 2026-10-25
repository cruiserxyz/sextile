package ui

import (
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
	press := func(st aspectModalState, k tea.KeyMsg) aspectModalState {
		st, _ = updateAspectModalKey(st, k, testListHeight)
		return st
	}
	st := openTest(recipe.AspectPolicy{})
	if st.cursor >= familyRows() {
		t.Fatalf("fixture: the cursor opens at %d, want it in the group rows", st.cursor)
	}
	st = typeIntoPopup(st, "inco", press)
	if len(st.matches) == 0 || st.matches[0].Name != "Inconjunct" {
		t.Fatalf("fixture: matches %v, want Inconjunct first", st.matches)
	}
	if st.cursor != familyRows() {
		t.Fatalf("after typing the cursor is on row %d, want %d, the first match", st.cursor, familyRows())
	}
	was := st.selected[6]
	st = press(st, tea.KeyMsg{Type: tea.KeySpace})
	if st.selected[6] == was {
		t.Error("Space did not tick Inconjunct, the item typed for")
	}
	// Nothing matching: the cursor parks on the last family row, as before.
	st = typeIntoPopup(st, "zzz", press)
	if len(st.matches) != 0 || st.cursor != familyRows()-1 {
		t.Errorf("no match: matches %d, cursor %d of %d rows", len(st.matches), st.cursor, st.totalRows())
	}
	// Deleting the filter back to empty: the list's first row.
	for range "incozzz" {
		st = press(st, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	if st.filterText != "" || st.cursor != familyRows() {
		t.Errorf("emptied filter %q: cursor %d, want %d, the list's first row", st.filterText, st.cursor, familyRows())
	}
}

func TestBodiesPopupFilterLandsOnTheFirstMatch(t *testing.T) {
	press := func(st bodyModalState, k tea.KeyMsg) bodyModalState {
		st, _ = updateBodyModalKey(st, k, testListHeight)
		return st
	}
	st := openBodyTest(recipe.ObjectPolicy{}, nil)
	st = typeIntoPopup(st, "cere", press)
	if len(st.matches) == 0 || st.matches[0].Name != "Ceres" {
		t.Fatalf("fixture: matches %v, want Ceres first", st.matches)
	}
	if st.cursor != groupRows() {
		t.Fatalf("after typing the cursor is on row %d, want %d, the first match", st.cursor, groupRows())
	}
	was := st.selected[12]
	st = press(st, tea.KeyMsg{Type: tea.KeySpace})
	if st.selected[12] == was {
		t.Error("Space did not tick Ceres, the item typed for")
	}
}
