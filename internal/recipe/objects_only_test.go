package recipe

import (
	"reflect"
	"testing"
)

// objects_only_test.go covers ObjectPolicy.Only, the narrowing that makes a
// selected body show only itself. The argv it compiles to is pinned in
// internal/astrolog (TestOnlyIntersectsTheBodyList); here is the policy's own
// meaning.

// TestOnlySetHoldsAnyObject: Only is object numbers, so an object with no glyph
// (Fortune 19, a fixed star, a cusp point) is a member like the Sun, in
// ascending order; a negative number is no object and the policy refuses it.
func TestOnlySetHoldsAnyObject(t *testing.T) {
	set := NewObjectSet(60, 19, 1, 23, 19)
	if got, want := set.Members(), []int{1, 19, 23, 60}; !reflect.DeepEqual(got, want) {
		t.Errorf("Members = %v, want %v", got, want)
	}
	if !set.Has(19) || set.Has(2) {
		t.Error("Has is wrong for Fortune or the Moon")
	}
	for _, base := range []VisibilityProfile{AstrologDefault, Explicit} {
		if _, err := NewChartSpec(MomentAtPlace{JD: 2446232.0, Lon: 0, Lat: 0}, WholeSign, Tropical,
			ObjectPolicy{Baseline: base, Selection: selectionFor(base), Only: set}); err != nil {
			t.Errorf("baseline %d: Only with Fortune and a star was refused: %v", int(base), err)
		}
		if _, err := NewChartSpec(MomentAtPlace{JD: 2446232.0, Lon: 0, Lat: 0}, WholeSign, Tropical,
			ObjectPolicy{Baseline: base, Selection: selectionFor(base), Only: NewObjectSet(-1)}); err == nil {
			t.Errorf("baseline %d: a negative object number in Only validated", int(base))
		}
	}
}

func selectionFor(base VisibilityProfile) []int {
	if base == Explicit {
		return []int{1, 19}
	}
	return nil
}

// TestObjectPolicy_OnlyObjectDropsOnly: the probe's single-object policy keeps
// nothing of the policy it came from, Only included, so no narrowing can reach
// the chooser's ephemeris probe.
func TestObjectPolicy_OnlyObjectDropsOnly(t *testing.T) {
	p := ObjectPolicy{Baseline: AstrologDefault, TrueNodeMode: true, Only: NewObjectSet(1)}
	got := p.OnlyObject(11)
	if len(got.Only) != 0 || got.Baseline != Explicit || !reflect.DeepEqual(got.Selected(), []int{11}) || !got.TrueNodeMode {
		t.Errorf("OnlyObject = %+v, want Explicit [11] with the node mode and no Only", got)
	}
}
