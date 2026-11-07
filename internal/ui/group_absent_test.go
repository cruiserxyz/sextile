package ui

import (
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// A group holding a member that cannot be ticked (no ephemeris file) still
// toggles: Space on its row ticks what can be ticked, and the next Space
// unticks the group. Before, the untickable member kept the group short of
// "all", so every later Space tried to tick again and the row never reacted
// (operator, 2026-11-04: "space on Others ... still does not react").
func TestGroupWithAbsentMemberUnticks(t *testing.T) {
	reg := []aspect.RegistryObject{
		{Object: aspect.Object{Number: 58, Name: "Titan"}},
		{Object: aspect.Object{Number: 84, Name: "Sirius"}},
		{Object: aspect.Object{Number: 85, Name: "Canopus"}},
	}
	st := popup.Open(reg, recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{}}, nil)
	st.Absent = map[int]string{58: "sepm9606.se1"}
	other := recipe.ObjectGroupOf(84)
	if recipe.ObjectGroupOf(58) != other {
		t.Fatal("Titan and Sirius are not in one group")
	}
	st.ToggleGroup(other)
	if !st.Selected[84] || !st.Selected[85] || st.Selected[58] {
		t.Fatalf("first Space: ticked %v, want Sirius and Canopus, not Titan", st.Selected)
	}
	st.ToggleGroup(other)
	if st.Selected[84] || st.Selected[85] || st.Selected[58] {
		t.Fatalf("second Space: ticked %v, want none (the group unticks)", st.Selected)
	}
	// A member ticked before its file went missing counts and unticks with the group.
	st.Selected[58] = true
	st.ToggleGroup(other) // ticks Sirius and Canopus: now every member that can be ticked is
	st.ToggleGroup(other)
	if st.Selected[84] || st.Selected[85] || st.Selected[58] {
		t.Fatalf("with Titan ticked from before: %v after two Spaces, want none", st.Selected)
	}
}
