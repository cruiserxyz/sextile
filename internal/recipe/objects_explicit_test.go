package recipe

import (
	"reflect"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// objects_explicit_test.go covers the second VisibilityProfile (body-selector
// spec § The decision this slice makes). `Explicit` carries the operator's
// chosen set as Astrolog's own object numbers rather than as deltas over a
// baseline, because the operator picked a set and not a difference — and
// because `BodySet` names seventeen bodies where the registry carries 134.

// TestObjectPolicy_ExplicitIsAValidProfile: the profile must pass validation on
// its own, or nothing downstream can be built.
func TestObjectPolicy_ExplicitIsAValidProfile(t *testing.T) {
	if _, err := NewChartSpec(
		MomentAtPlace{JD: 2446232.0, Lon: 0, Lat: 0},
		WholeSign, Tropical,
		ObjectPolicy{Baseline: Explicit, Selection: []int{1, 2, 3}},
	); err != nil {
		t.Errorf("an Explicit policy with a selection must validate: %v", err)
	}
}

// TestObjectPolicy_ExplicitRejectsDeltas: a delta over an explicit set has no
// meaning. Accepting one would silently pick a winner between two descriptions
// of the visible set.
func TestObjectPolicy_ExplicitRejectsDeltas(t *testing.T) {
	moment := MomentAtPlace{JD: 1, Lon: 0, Lat: 0}
	if _, err := NewChartSpec(moment, WholeSign, Tropical,
		ObjectPolicy{Baseline: Explicit, Selection: []int{1}, Add: NewBodySet(aspect.Chiron)},
	); err == nil {
		t.Error("Add alongside Explicit must be rejected")
	}
	if _, err := NewChartSpec(moment, WholeSign, Tropical,
		ObjectPolicy{Baseline: Explicit, Selection: []int{1}, Remove: NewBodySet(aspect.Sun)},
	); err == nil {
		t.Error("Remove alongside Explicit must be rejected")
	}
}

// TestObjectPolicy_SelectionRequiresExplicit mirrors AspectPolicy's symmetric
// guard: a selection under a delta baseline would never be emitted, so it reads
// as a chosen set that silently does nothing.
func TestObjectPolicy_SelectionRequiresExplicit(t *testing.T) {
	if _, err := NewChartSpec(
		MomentAtPlace{JD: 1, Lon: 0, Lat: 0}, WholeSign, Tropical,
		ObjectPolicy{Baseline: AstrologDefault, Selection: []int{1, 2}},
	); err == nil {
		t.Error("a Selection under AstrologDefault must be rejected — it would never be emitted")
	}
}

// TestObjectPolicy_ExplicitAcceptsObjectZero: Astrolog's object registry is
// 0-BASED — `astrolog -R1 -HO` opens with `0 Earth`. AspectPolicy's registry is
// 1-based and rejects 0, so mirroring its bound here would make Earth
// unselectable while every other row in the lane still passed.
func TestObjectPolicy_ExplicitAcceptsObjectZero(t *testing.T) {
	if _, err := NewChartSpec(
		MomentAtPlace{JD: 1, Lon: 0, Lat: 0}, WholeSign, Tropical,
		ObjectPolicy{Baseline: Explicit, Selection: []int{0}},
	); err != nil {
		t.Errorf("object 0 (Earth) is a real registry row and must validate: %v", err)
	}
	if _, err := NewChartSpec(
		MomentAtPlace{JD: 1, Lon: 0, Lat: 0}, WholeSign, Tropical,
		ObjectPolicy{Baseline: Explicit, Selection: []int{-1}},
	); err == nil {
		t.Error("a negative object number is not a registry row and must be rejected")
	}
}

// TestObjectPolicy_ExplicitAllowsTheEmptySet: "the operator chose none" is a
// real state that differs from "the operator has not chosen" (C8), and the two
// are told apart by the baseline, never by the length of the selection.
func TestObjectPolicy_ExplicitAllowsTheEmptySet(t *testing.T) {
	if _, err := NewChartSpec(
		MomentAtPlace{JD: 1, Lon: 0, Lat: 0}, WholeSign, Tropical,
		ObjectPolicy{Baseline: Explicit},
	); err != nil {
		t.Errorf("Explicit with an empty selection must validate: %v", err)
	}
}

// TestObjectPolicy_SelectedIsAscendingAndDeduplicated: the compiler emits this
// order and nothing else, so a caller may build the selection in any order
// without changing the argv — and therefore without changing the chart.
func TestObjectPolicy_SelectedIsAscendingAndDeduplicated(t *testing.T) {
	got := (ObjectPolicy{Baseline: Explicit, Selection: []int{31, 1, 22, 1, 0, 10}}).Selected()
	want := []int{0, 1, 10, 22, 31}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Selected() = %v, want %v", got, want)
	}
	// The baseline decides, never the field's contents. A policy carrying a
	// leftover selection under a delta baseline must report nothing — the
	// struct is reachable unvalidated (validate() refuses this combination,
	// but Selected is a plain method on a plain struct), and a caller that
	// trusted the slice would emit a set the chart is not drawing.
	if n := (ObjectPolicy{Baseline: AstrologDefault, Selection: []int{1, 2}}).Selected(); n != nil {
		t.Errorf("Selected() on a delta policy carrying a leftover selection = %v, want nil", n)
	}
}

// TestObjectPolicy_ZeroValueIsUnchanged: AstrologDefault must stay the zero
// value, so every caller predating this profile compiles and behaves
// identically. This is the whole reason the new profile could be appended.
func TestObjectPolicy_ZeroValueIsUnchanged(t *testing.T) {
	if (ObjectPolicy{}).Baseline != AstrologDefault {
		t.Fatal("the zero ObjectPolicy must still mean AstrologDefault")
	}
	zero := ObjectPolicy{}
	if !reflect.DeepEqual(zero.Visible(), profileBodies(AstrologDefault)) {
		t.Error("the zero policy's visible set must still be the AstrologDefault profile")
	}
}

// TestObjectPolicy_VisibleIsNotMeaningfulUnderExplicit pins a GAP, not a
// behaviour. `Visible()` answers in `aspect.Body`, which cannot name a registry
// object, and `recipe` holds no number→Body mapping — that lives in the
// compiler. So it reports the empty set here.
//
// Its one production consumer is the wheel-image highlight
// (`highlightMembers`), which drops undrawn bodies; an empty answer therefore
// degrades an Explicit chart's highlight to the plain wheel SILENTLY. Step 2
// (C1a, the explicit set meeting the highlight composite) is where that is
// settled. This test exists so the gap fails loudly if someone reaches for
// `Visible()` under Explicit before then.
func TestObjectPolicy_VisibleIsNotMeaningfulUnderExplicit(t *testing.T) {
	if got := (ObjectPolicy{Baseline: Explicit, Selection: []int{1, 2}}).Visible(); len(got) != 0 {
		t.Errorf("Visible() under Explicit = %v; recipe cannot resolve registry numbers to bodies, so it must report the empty set until C1a settles the highlight", got)
	}
}
