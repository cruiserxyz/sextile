package recipe

import (
	"reflect"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// objects_only_test.go covers ObjectPolicy.Only, the narrowing that makes a
// selected body show only itself. The argv it compiles to is pinned in
// internal/astrolog (TestOnlyIntersectsTheBodyList); here is the policy's own
// meaning.

// TestObjectPolicy_OnlyNarrowsVisible: what the chart draws is the resolved set
// intersected with Only, never a union with it.
func TestObjectPolicy_OnlyNarrowsVisible(t *testing.T) {
	p := ObjectPolicy{
		Baseline: AstrologDefault,
		Add:      NewBodySet(aspect.Chiron),
		Remove:   NewBodySet(aspect.Moon),
		Only:     NewBodySet(aspect.Sun, aspect.Moon, aspect.Chiron, aspect.Lilith),
	}
	// Moon was removed and Lilith was never added: neither comes back.
	want := []aspect.Body{aspect.Sun, aspect.Chiron}
	if got := p.Visible().Members(); !reflect.DeepEqual(got, want) {
		t.Errorf("Visible with Only = %v, want %v", got, want)
	}

	p.Only = nil
	if got := p.Visible(); len(got) != 13 { // the 13 default bodies, minus Moon, plus Chiron
		t.Errorf("Visible without Only has %d bodies, want 13: %v", len(got), got.Members())
	}
}

// TestObjectPolicy_OnlyMustNameKnownBodies: a body that has no Astrolog number
// would silently narrow to nothing, so the policy refuses it before a compile.
func TestObjectPolicy_OnlyMustNameKnownBodies(t *testing.T) {
	for _, base := range []VisibilityProfile{AstrologDefault, Explicit} {
		_, err := NewChartSpec(
			MomentAtPlace{JD: 2446232.0, Lon: 0, Lat: 0},
			WholeSign, Tropical,
			ObjectPolicy{Baseline: base, Only: BodySet{aspect.BodyUnknown: true}},
		)
		if err == nil {
			t.Errorf("baseline %d: an unknown body in Only validated", int(base))
		}
	}
	if _, err := NewChartSpec(
		MomentAtPlace{JD: 2446232.0, Lon: 0, Lat: 0},
		WholeSign, Tropical,
		ObjectPolicy{Baseline: Explicit, Selection: []int{1}, Only: NewBodySet(aspect.Sun)},
	); err != nil {
		t.Errorf("Only is valid beside a selection: %v", err)
	}
}

// TestObjectPolicy_OnlyObjectDropsOnly: the probe's single-object policy keeps
// nothing of the policy it came from, Only included, so no narrowing can reach
// the chooser's ephemeris probe.
func TestObjectPolicy_OnlyObjectDropsOnly(t *testing.T) {
	p := ObjectPolicy{Baseline: AstrologDefault, TrueNodeMode: true, Only: NewBodySet(aspect.Sun)}
	got := p.OnlyObject(11)
	if len(got.Only) != 0 || got.Baseline != Explicit || !reflect.DeepEqual(got.Selected(), []int{11}) || !got.TrueNodeMode {
		t.Errorf("OnlyObject = %+v, want Explicit [11] with the node mode and no Only", got)
	}
}
