package astrolog

import (
	"slices"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// A narrowed list that drains to nothing only because its bodies have no file
// never goes out as a bare `-R0`: that restricts every object and leaves on
// screen just the objects the host enables beyond them (the cusp points). A
// deliberate empty selection keeps its bare `-R0`.
func TestOnlyThatDrainsFallsBackToTheBaseline(t *testing.T) {
	av := ceresPallasJuno()
	av.Absent[11], av.Names[11] = "seas_18.se1", "Chiron"
	withAvailability(t, av)
	explicit := func(only ...aspect.Object) recipe.ObjectPolicy {
		return recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 5, 11}, Only: onlyObjects(only...)}
	}

	got, err := emitObjects(explicit(objChiron))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"_Yn", "_Ynn", "-R0", "1", "2", "5"}; !slices.Equal(got, want) {
		t.Errorf("Only = {absent}: %v, want the baseline's list %v", got, want)
	}
	got, err = emitObjects(explicit(objChiron, objMars))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"_Yn", "_Ynn", "-R0", "5"}; !slices.Equal(got, want) {
		t.Errorf("Only = {absent, present}: %v, want %v", got, want)
	}
	got, err = emitObjects(recipe.ObjectPolicy{Baseline: recipe.Explicit})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"_Yn", "_Ynn", "-R0"}; !slices.Equal(got, want) {
		t.Errorf("a deliberate empty selection: %v, want the bare %v", got, want)
	}
	// A delta baseline falls back to its flips, never to a bare list.
	got, err = emitObjects(recipe.ObjectPolicy{Add: recipe.NewObjectSet(11), Only: onlyObjects(objChiron)})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(got, "-R0") {
		t.Errorf("a delta Only drained by an absent body went out as %v", got)
	}
}

// A mark on an object the chart does not draw, whatever the reason (the host's
// astrolog.as does not list it, the policy removed it), leaves nothing to draw,
// and Astrolog would be asked for a bare `-R0`: a chart with no body. The
// narrowing is dropped and the policy's own baseline goes out instead (operator,
// 2026-11-04: the Now chart rendered empty after the host's astrolog.as was
// replaced by the stock one, which draws fewer objects). Only the deliberate
// empty selection (an Explicit policy with no selection and no narrowing) keeps
// its bare list.
func TestOnlyOutsideTheDrawnSetFallsBack(t *testing.T) {
	// A host whose default set lacks Fortune (19), which is present: it has its
	// file, the chart just does not draw it.
	withAvailability(t, Availability{Default: []int{1, 2, 3, 22}})
	fortune := recipe.NewObjectSet(19)

	// A delta policy: the baseline's flips go out, never a bare list.
	delta := recipe.ObjectPolicy{Remove: recipe.NewObjectSet(2), Nodes: recipe.NodesTrue}
	marked := delta
	marked.Only = fortune
	want, err := emitObjects(delta)
	if err != nil {
		t.Fatal(err)
	}
	got, err := emitObjects(marked)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) || slices.Contains(got, "-R0") {
		t.Errorf("delta, Only = {19} (not drawn): %v, want the baseline's %v and no bare -R0", got, want)
	}
	if want := []string{"=Yn", "_Ynn", "-R", "2"}; !slices.Equal(got, want) {
		t.Errorf("delta, Only = {19}: %v, want the flips %v", got, want)
	}
	if nums, err := ArgvObjectNumbers(marked); err != nil || !slices.Equal(nums, []int{1, 3, 22}) {
		t.Errorf("delta, ArgvObjectNumbers = %v, %v; want the baseline's [1 3 22]", nums, err)
	}
	if nums, err := DrainedObjects(marked); err != nil || !slices.Equal(nums, []int{19}) {
		t.Errorf("delta, DrainedObjects = %v, %v; want [19]", nums, err)
	}

	// An Explicit selection: its own list goes out.
	explicitMarked := recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 5}, Only: fortune}
	got, err = emitObjects(explicitMarked)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"_Yn", "_Ynn", "-R0", "1", "2", "5"}; !slices.Equal(got, want) {
		t.Errorf("explicit, Only = {19} (not in the selection): %v, want %v", got, want)
	}
	if nums, err := ArgvObjectNumbers(explicitMarked); err != nil || !slices.Equal(nums, []int{1, 2, 5}) {
		t.Errorf("explicit, ArgvObjectNumbers = %v, %v; want the selection [1 2 5]", nums, err)
	}
	if nums, err := DrainedObjects(explicitMarked); err != nil || !slices.Equal(nums, []int{19}) {
		t.Errorf("explicit, DrainedObjects = %v, %v; want [19]", nums, err)
	}

	// Marks that name more than one undrawn object are all reported.
	both := explicitMarked
	both.Only = recipe.NewObjectSet(19, 20)
	if nums, err := DrainedObjects(both); err != nil || !slices.Equal(nums, []int{19, 20}) {
		t.Errorf("Only = {19, 20}: DrainedObjects = %v, %v; want [19 20]", nums, err)
	}

	// One mark that does draw keeps the narrowing: marks narrow, they are not
	// dropped for being partly stale.
	partly := explicitMarked
	partly.Only = recipe.NewObjectSet(2, 19)
	if got, err := emitObjects(partly); err != nil || !slices.Equal(got, []string{"_Yn", "_Ynn", "-R0", "2"}) {
		t.Errorf("Only = {2, 19}: %v, %v; want [-R0 2]", got, err)
	}
	if nums, err := DrainedObjects(partly); err != nil || nums != nil {
		t.Errorf("Only = {2, 19}: DrainedObjects = %v, %v; want none (the Moon is drawn)", nums, err)
	}

	// The deliberate empty selection keeps its bare list with marks on it too:
	// there is no set to fall back to, so nothing is said either.
	none := recipe.ObjectPolicy{Baseline: recipe.Explicit, Only: recipe.NewObjectSet(1)}
	if got, err := emitObjects(none); err != nil || !slices.Equal(got, []string{"_Yn", "_Ynn", "-R0"}) {
		t.Errorf("no bodies chosen, Only = {1}: %v, %v; want the bare [-R0]", got, err)
	}
	if nums, err := DrainedObjects(none); err != nil || nums != nil {
		t.Errorf("no bodies chosen, Only = {1}: DrainedObjects = %v, %v; want none", nums, err)
	}
}
