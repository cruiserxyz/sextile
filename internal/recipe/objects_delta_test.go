package recipe

import (
	"reflect"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// objects_delta_test.go pins the delta policy in Astrolog's object numbers:
// what Toggles answers and the structural refusals (with the number in the
// message). The policy holds no list of the host's default set, so what a
// delta leaves drawn is not asked here; it is the compiler's one rule
// (internal/astrolog drawnObjectNumbers) and is pinned there, with the argv.

// TestDeltaTogglesInNumbers: Toggles is the objects Add and Remove name together,
// ascending, whatever order the sets were built in.
func TestDeltaTogglesInNumbers(t *testing.T) {
	p := ObjectPolicy{
		Baseline: AstrologDefault,
		Add:      NewObjectSet(28, 11, 19), // Descendant, Chiron, Fortune
		Remove:   NewObjectSet(10, 2),      // Pluto, Moon
	}
	if got, want := p.Toggles(), []int{2, 10, 11, 19, 28}; !reflect.DeepEqual(got, want) {
		t.Errorf("Toggles = %v, want %v", got, want)
	}
	if got := (ObjectPolicy{}).Toggles(); len(got) != 0 {
		t.Errorf("an untouched policy flips nothing, got %v", got)
	}
}

// TestDeltaRefusalsAreStructural: a number below zero and an object in both Add
// and Remove are refused, and the message says which object. Whether an Add is
// already in the host's default set, or a Remove is not, is not asked: the
// policy cannot know the host's set, so both validate (and each draws what it
// says, astrolog.deltaSet).
func TestDeltaRefusalsAreStructural(t *testing.T) {
	cases := []struct {
		name string
		p    ObjectPolicy
		want string
	}{
		{"add below zero", ObjectPolicy{Add: NewObjectSet(-3)}, "Add: -3 is not an object number"},
		{"remove below zero", ObjectPolicy{Remove: NewObjectSet(-3)}, "Remove: -3 is not an object number"},
		{"in both", ObjectPolicy{Add: NewObjectSet(11), Remove: NewObjectSet(11)}, "object 11 is in both Add and Remove"},
	}
	for _, tc := range cases {
		err := tc.p.validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: validate error = %v, want it to contain %q", tc.name, err, tc.want)
		}
	}
	// Neither of these was a policy before the host's default set was the only
	// baseline: object 1 (the Sun) in Add, object 19 (Fortune) in Remove.
	for _, p := range []ObjectPolicy{
		{Add: NewObjectSet(1)},
		{Remove: NewObjectSet(19)},
		{Add: NewObjectSet(0)}, // Earth (object 0) is a number like any other
	} {
		if err := p.validate(); err != nil {
			t.Errorf("%+v must validate structurally: %v", p, err)
		}
	}
}

// TestNewHighlightFocusIsAFlag: object 0 is Earth, so the focus is a flag and a
// number, never a sentinel number. The zero Object focuses nothing.
func TestNewHighlightFocusIsAFlag(t *testing.T) {
	earth := NewHighlight(nil, aspect.Object{Number: 0, Name: "Earth"})
	if !earth.Active() || !earth.HasFocus || earth.FocusedObject != 0 {
		t.Errorf("a focused Earth is an active highlight on object 0: %+v", earth)
	}
	fortune := NewHighlight([]int{1}, aspect.Object{Number: 19, Name: "Fortune"})
	if !fortune.HasFocus || fortune.FocusedObject != 19 || !fortune.Selected.Has(1) {
		t.Errorf("a focused Fortune keeps its number beside the marks: %+v", fortune)
	}
	if none := NewHighlight(nil, aspect.Object{}); none.Active() || none.HasFocus {
		t.Errorf("the zero Object focuses nothing: %+v", none)
	}
	if marksOnly := NewHighlight([]int{1}, aspect.Object{}); !marksOnly.Active() || marksOnly.HasFocus {
		t.Errorf("marks alone are active without a focus: %+v", marksOnly)
	}
}
