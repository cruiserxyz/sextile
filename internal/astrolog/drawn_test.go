package astrolog

import (
	"errors"
	"slices"
	"sort"
	"strconv"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// drawn_test.go: slice 8c. There is ONE rule for what a chart draws on this
// host: the host's default set (Availability.Default) with the policy's flips,
// or the Explicit selection, then Only, then minus the objects without an
// ephemeris file (drawnObjectNumbers). Every consumer asks it, and none reads
// the policy's typed profile (there is none) or a list of its own.

// hostWithout31 is a host whose astrolog.as lists no Midheaven (31): the case
// the typed thirteen got wrong, because it put 31 back on the wheel whenever a
// mark named it.
func hostWithout31() Availability {
	return Availability{Default: []int{1, 2, 3, 19, 22}}
}

// A mark on an object the host does not draw draws nothing, and the six
// consumers of the rule say the same thing. Four are asserted here, one each: the
// Only show-list, ArgvObjectNumbers with the numbers the parsers expect (it is
// what carryRegistry hands every call), the wheel highlight and the drained
// accounting. The other two, the chooser's ticks and the ephemeris account, are
// lookup's and are asserted in its package under the same name.
func TestDrawnIsTheHostDefaultWithFlips(t *testing.T) {
	withAvailability(t, hostWithout31())
	marked := recipe.ObjectPolicy{Only: recipe.NewObjectSet(1, 31)}

	// 1. emitObjects' Only list: the Sun, not the Midheaven.
	if got := objectBandOf(t, marked); !slices.Equal(got, []string{"-R0", "1"}) {
		t.Errorf("the Only list = %v, want [-R0 1] (the host does not draw 31, so its mark draws nothing)", got)
	}
	// 2. ArgvObjectNumbers, the numbers the call is made with.
	if got, err := ArgvObjectNumbers(marked); err != nil || !slices.Equal(got, []int{1}) {
		t.Errorf("ArgvObjectNumbers = %v, %v; want [1]", got, err)
	}
	// 3. The numbers the parsers expect, carried on every call the compiler issues.
	plan := mustCompile(t, onlySpec(t, marked), recipe.Positions{})
	if got := plan.Invocations[0].call.Objects; !slices.Equal(got, []int{1}) {
		t.Errorf("the call carries objects %v, want [1]", got)
	}
	// 4. The wheel highlight: a mark on 31 is not a member.
	if got := mustHighlightMembers(t, marked, recipe.NewHighlight([]int{31, 1}, aspect.Object{})); !slices.Equal(got, []int{1}) {
		t.Errorf("highlight members = %v, want [1]", got)
	}
	// 5. The drained-selection accounting: 1 is drawn, so the narrowing leaves
	// something and stays the operator's. Alone, a mark on 31 leaves nothing
	// (the host never drew it, with its file or without): the narrowing is
	// dropped, and 31 is named so the Log can say the default set is shown.
	if got, err := DrainedObjects(marked); err != nil || got != nil {
		t.Errorf("DrainedObjects = %v, %v; want none (1 is drawn)", got, err)
	}
	if got, err := DrainedObjects(recipe.ObjectPolicy{Only: recipe.NewObjectSet(31)}); err != nil || !slices.Equal(got, []int{31}) {
		t.Errorf("DrainedObjects of a mark on 31 alone = %v, %v; want [31] (the host does not draw 31, the baseline is drawn)", got, err)
	}
	noFile := hostWithout31()
	noFile.Absent = map[int]string{31: "seas_18.se1"}
	noFile.Names = map[int]string{31: "Midheaven"}
	SetAvailability(noFile)
	if got, err := DrainedObjects(recipe.ObjectPolicy{Only: recipe.NewObjectSet(31)}); err != nil || !slices.Equal(got, []int{31}) {
		t.Errorf("DrainedObjects with 31 missing its file = %v, %v; want [31]", got, err)
	}
	SetAvailability(hostWithout31())

	// Alone, the mark leaves nothing: the narrowing is dropped and the host
	// default goes out unnarrowed (no switch at all), never a bare list and
	// never the Midheaven.
	alone := recipe.ObjectPolicy{Only: recipe.NewObjectSet(31)}
	if got, err := emitObjects(alone); err != nil || len(got) != 0 {
		t.Errorf("a mark on 31 alone = %v, %v; want the baseline's band, which is empty here (no bare -R0)", got, err)
	}
	if got, err := ArgvObjectNumbers(alone); err != nil || !slices.Equal(got, []int{1, 2, 3, 19, 22}) {
		t.Errorf("a mark on 31 alone draws %v, %v; want the host default [1 2 3 19 22]", got, err)
	}
	// And the host default narrowed by nothing is the host default, whatever the
	// thirteen once were.
	if got, err := ArgvObjectNumbers(recipe.ObjectPolicy{}); err != nil || !slices.Equal(got, []int{1, 2, 3, 19, 22}) {
		t.Errorf("an untouched policy draws %v, %v; want the host default [1 2 3 19 22]", got, err)
	}
	// The policy's Add and Remove are applied to the host's set, not to a list
	// of Sextile's own.
	flipped := recipe.ObjectPolicy{Add: recipe.NewObjectSet(11), Remove: recipe.NewObjectSet(2)}
	if got, err := ArgvObjectNumbers(flipped); err != nil || !slices.Equal(got, []int{1, 3, 11, 19, 22}) {
		t.Errorf("Add 11, Remove 2 draws %v, %v; want [1 3 11 19 22]", got, err)
	}
	// A Remove of an object the host does not list does not turn it on: not in
	// the answer, not under a mark, and not as a `-R` switch (which would flip
	// the host's own state the wrong way).
	removed := recipe.ObjectPolicy{Remove: recipe.NewObjectSet(31)}
	if got, err := ArgvObjectNumbers(removed); err != nil || !slices.Equal(got, []int{1, 2, 3, 19, 22}) {
		t.Errorf("Remove 31 draws %v, %v; want the host default [1 2 3 19 22]", got, err)
	}
	if got, err := emitObjects(removed); err != nil || slices.Contains(got, "-R") {
		t.Errorf("Remove 31 on a host without 31 emitted %v, %v; want no switch", got, err)
	}
	removed.Only = recipe.NewObjectSet(1, 31)
	if got := objectBandOf(t, removed); !slices.Equal(got, []string{"-R0", "1"}) {
		t.Errorf("Remove 31 with a mark on 31 = %v, want [-R0 1]", got)
	}
	// And an Add of one the host does list does not turn it off.
	added := recipe.ObjectPolicy{Add: recipe.NewObjectSet(19)}
	if got, err := emitObjects(added); err != nil || slices.Contains(got, "-R") {
		t.Errorf("Add 19 on a host that lists 19 emitted %v, %v; want no switch", got, err)
	}
	if got, err := ArgvObjectNumbers(added); err != nil || !slices.Equal(got, []int{1, 2, 3, 19, 22}) {
		t.Errorf("Add 19 draws %v, %v; want the host default [1 2 3 19 22]", got, err)
	}
}

// A drained narrowing is the other half of consumer 5: every object it names
// has no ephemeris file, so the chart draws the policy's own baseline.
func TestDrawnDrainedNarrowingFallsBackToTheBaseline(t *testing.T) {
	withAvailability(t, Availability{
		Default: []int{1, 2, 12, 22},
		Absent:  map[int]string{12: "seas_18.se1"},
		Names:   map[int]string{12: "Ceres"},
	})
	p := recipe.ObjectPolicy{Only: recipe.NewObjectSet(12)}
	if got, err := DrainedObjects(p); err != nil || !slices.Equal(got, []int{12}) {
		t.Errorf("DrainedObjects = %v, %v; want [12]", got, err)
	}
	if got, err := ArgvObjectNumbers(p); err != nil || !slices.Equal(got, []int{1, 2, 22}) {
		t.Errorf("ArgvObjectNumbers = %v, %v; want the baseline without the absent Ceres [1 2 22]", got, err)
	}
	if got := objectBandOf(t, p); !slices.Equal(got, []string{"-R", "12"}) {
		t.Errorf("object band = %v, want the baseline's [-R 12] (Ceres switched off), not a bare -R0", got)
	}
}

// The numbers the parsers expect are the numbers the argv draws: for a spread
// of policies on one host, apply the `-R n` flips the compiler emitted to the
// host's default set (as Astrolog does), cut the absent objects, and compare to
// ArgvObjectNumbers. This is what a resolver that read its own list, or an
// argv that flipped a name the host already has the right way, would fail.
func TestDrawnMatchesWhatTheArgvDraws(t *testing.T) {
	av := Availability{
		Default: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 12, 16, 22, 31},
		Absent:  map[int]string{12: "seas_18.se1", 13: "seas_18.se1"},
		Names:   map[int]string{12: "Ceres", 13: "Pallas"},
	}
	withAvailability(t, av)
	for name, p := range map[string]recipe.ObjectPolicy{
		"untouched (Ceres is a default and absent)": {},
		"hellenistic adds":                          {Add: recipe.NewObjectSet(11, 17, 18, 25, 28)},
		"a removal":                                 {Remove: recipe.NewObjectSet(2, 10)},
		"a removed angle":                           {Remove: recipe.NewObjectSet(31)},
		"an absent addition":                        {Add: recipe.NewObjectSet(13, 19)},
		"adds and removals":                         {Add: recipe.NewObjectSet(11, 19), Remove: recipe.NewObjectSet(5)},
		"true node":                                 {TrueNodeMode: true, Add: recipe.NewObjectSet(19)},
		"a remove of an unlisted object":            {Remove: recipe.NewObjectSet(19)},
		"an add of a listed object":                 {Add: recipe.NewObjectSet(10)},
	} {
		argv, err := emitObjects(p)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		on := map[int]bool{}
		for _, n := range av.Default {
			on[n] = true
		}
		for i := 0; i+1 < len(argv); i++ {
			if argv[i] == "-R" {
				n, err := strconv.Atoi(argv[i+1])
				if err != nil {
					t.Fatalf("%s: argv %v", name, argv)
				}
				on[n] = !on[n]
			}
		}
		var want []int
		for n, in := range on {
			if in && !av.IsAbsent(n) {
				want = append(want, n)
			}
		}
		sort.Ints(want)
		got, err := ArgvObjectNumbers(p)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("%s: ArgvObjectNumbers = %v, %v; the argv %v draws %v", name, got, err, argv, want)
		}
	}
}

// Before the host's default set is known there is no answer. Every consumer
// fails with the same error; none guesses, and none draws the thirteen.
func TestDrawnBeforeTheProbeIsAnError(t *testing.T) {
	withAvailability(t, Availability{})
	cases := map[string]recipe.ObjectPolicy{
		"delta":    {},
		"flips":    {Add: recipe.NewObjectSet(11)},
		"marks":    {Only: recipe.NewObjectSet(1)},
		"explicit": {Baseline: recipe.Explicit, Selection: []int{1, 2}},
	}
	for name, p := range cases {
		if _, err := drawnObjectNumbers(p); !errors.Is(err, errNoHostDefault) {
			t.Errorf("%s: drawnObjectNumbers error = %v, want errNoHostDefault", name, err)
		}
		if _, err := ArgvObjectNumbers(p); !errors.Is(err, errNoHostDefault) {
			t.Errorf("%s: ArgvObjectNumbers error = %v, want errNoHostDefault", name, err)
		}
		if _, err := WantedObjectNumbers(p, Availability{}); !errors.Is(err, errNoHostDefault) {
			t.Errorf("%s: WantedObjectNumbers error = %v, want errNoHostDefault", name, err)
		}
		if _, err := highlightMembers(p, recipe.NewHighlight([]int{1}, aspect.Object{})); !errors.Is(err, errNoHostDefault) {
			t.Errorf("%s: highlightMembers error = %v, want errNoHostDefault", name, err)
		}
		if _, err := Compile(onlySpec(t, p), recipe.Positions{}); !errors.Is(err, errNoHostDefault) {
			t.Errorf("%s: Compile error = %v, want errNoHostDefault (no call is made on a guess)", name, err)
		}
	}
	// A narrowing is where a guess would hide: its drained account and its
	// show list both need the answer.
	narrowed := recipe.ObjectPolicy{Only: recipe.NewObjectSet(1)}
	if _, err := DrainedObjects(narrowed); !errors.Is(err, errNoHostDefault) {
		t.Errorf("DrainedObjects error = %v, want errNoHostDefault", err)
	}
	if _, err := emitObjects(narrowed); !errors.Is(err, errNoHostDefault) {
		t.Errorf("emitObjects error = %v, want errNoHostDefault", err)
	}
}
