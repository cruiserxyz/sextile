package ui

import (
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// F14: the Space marks of a slot narrow that slot's searches, and the timeline
// key carries them so a mark change asks again.
func TestTimelineKeyCarriesTheMarks(t *testing.T) {
	m := octoberSet(t, 0)
	req0, key0, ok := m.timelineInputs()
	if !ok || len(req0.Natals) != 2 {
		t.Fatalf("fixture: ok %v with %d natals, want a comparing timeline of two", ok, len(req0.Natals))
	}
	for i, n := range req0.Natals {
		if len(n.Objects.Only) != 0 {
			t.Fatalf("fixture: natal %d already carries marks %v", i+1, n.Objects.Only.Members())
		}
	}

	// A mark on ① changes the key and narrows ①'s policy only.
	m.slots.ToggleObjectSelection(0, objSun)
	req1, key1, _ := m.timelineInputs()
	if key1 == key0 {
		t.Error("marking the Sun on ① did not change the timeline key")
	}
	if got := req1.Natals[0].Objects.Only.Members(); len(got) != 1 || got[0] != 1 {
		t.Errorf("① carries marks %v, want [1]", got)
	}
	if len(req1.Natals[1].Objects.Only) != 0 || len(req1.Lens.Objects.Only) != 0 {
		t.Errorf("② carries %v and the sky %v, want no marks on either", req1.Natals[1].Objects.Only.Members(), req1.Lens.Objects.Only.Members())
	}

	// A mark on ② is another key, and carries ②'s own.
	m.slots.ToggleObjectSelection(1, objMoon)
	req2, key2, _ := m.timelineInputs()
	if key2 == key1 {
		t.Error("marking the Moon on ② did not change the timeline key")
	}
	if got := req2.Natals[1].Objects.Only.Members(); len(got) != 1 || got[0] != 2 {
		t.Errorf("② carries marks %v, want [2]", got)
	}

	// A mark on the floating slot narrows the sky search and changes the key.
	m.slots.ToggleObjectSelection(len(m.slots.All())-1, objMars)
	req3, key3, _ := m.timelineInputs()
	if key3 == key2 {
		t.Error("marking Mars on the floating slot did not change the timeline key")
	}
	if got := req3.Lens.Objects.Only.Members(); len(got) != 1 || got[0] != 5 {
		t.Errorf("the sky search carries marks %v, want [5]", got)
	}

	// The same marks again: the same key (nothing re-asks).
	if _, again, _ := m.timelineInputs(); again != key3 {
		t.Error("an unchanged mark set changed the key")
	}

	// A tick-only change moves the object part of the key and no mark part.
	m.lens.Objects.Remove = recipe.NewObjectSet(objMoon.Number)
	_, key4, _ := m.timelineInputs()
	if key4 == key3 || key4.objects == key3.objects && key4.natals == key3.natals {
		t.Errorf("a tick change left the key as it was: %+v", key4)
	}
	if key4.natals != key3.natals || key4.skyMarks != key3.skyMarks {
		t.Error("a tick-only change moved the marks part of the key")
	}
}
