package astrolog

import (
	"slices"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// The timeline's objects follow the ticks (F13) and the marks narrow the natal
// side (F14). measuredSets is a binary whose Main minus Transit is {2, 22, 31},
// as 8.00's is.

// bandTail is the chooser band of one call, as the string the band tests read.
func bandTail(t *testing.T, src TimelineSource, req TimelineRequest) string {
	t.Helper()
	_, tail := callTokens(t, src, req)
	return strings.Join(tail, " ")
}

// bandSides splits a band tail into its -R0 and -RT0 lists ("" when absent).
func bandSides(tail string) (natal, transiting []int) {
	var cur *[]int
	for _, f := range strings.Fields(tail) {
		switch f {
		case "-R0":
			natal, cur = []int{}, &natal
		case "-RT0":
			transiting, cur = []int{}, &transiting
		default:
			if cur == nil || strings.HasPrefix(f, "-") {
				cur = nil
				continue
			}
			n := 0
			for _, r := range f {
				n = n*10 + int(r-'0')
			}
			*cur = append(*cur, n)
		}
	}
	return natal, transiting
}

func explicit(nums ...int) recipe.ObjectPolicy {
	return recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: nums}
}

func withMarks(o recipe.ObjectPolicy, marks ...int) recipe.ObjectPolicy {
	o.Only = recipe.NewObjectSet(marks...)
	return o
}

// natalReq is bandReq with the sky policy and the natal's own policy told apart.
func natalReq(t *testing.T, sky, natal recipe.ObjectPolicy) TimelineRequest {
	req := bandReq(t, sky, recipe.AspectPolicy{})
	req.Natal.Objects = natal
	return req
}

func TestTimelineUntickHidesBothSides(t *testing.T) {
	o := without(defaultLens(), objMoon)
	for _, src := range []TimelineSource{TimelineTransit, TimelineReturn} {
		natal, transiting := bandSides(bandTail(t, src, bandReq(t, o, recipe.AspectPolicy{})))
		if slices.Contains(natal, 2) || slices.Contains(transiting, 2) {
			t.Errorf("%s: Moon unticked but listed: natal %v transiting %v", src, natal, transiting)
		}
	}
	if natal, _ := bandSides(bandTail(t, TimelineSky, bandReq(t, o, recipe.AspectPolicy{}))); slices.Contains(natal, 2) {
		t.Errorf("sky: Moon unticked but listed: %v", natal)
	}
}

func TestTimelineTickListsBothSides(t *testing.T) {
	for _, n := range []int{12, 50} {
		o := explicit(append(slices.Clone(measuredMain), n)...)
		natal, transiting := bandSides(bandTail(t, TimelineTransit, bandReq(t, o, recipe.AspectPolicy{})))
		if !slices.Contains(natal, n) || !slices.Contains(transiting, n) {
			t.Errorf("object %d ticked: natal %v transiting %v, want it on both sides", n, natal, transiting)
		}
		if sky, _ := bandSides(bandTail(t, TimelineSky, bandReq(t, o, recipe.AspectPolicy{}))); !slices.Contains(sky, n) {
			t.Errorf("object %d ticked: the sky search lists %v", n, sky)
		}
	}
}

func TestTimelineTickOfADeltaBodyListsBothSides(t *testing.T) {
	natal, transiting := bandSides(bandTail(t, TimelineTransit, bandReq(t, hellenistic(), recipe.AspectPolicy{})))
	if !slices.Contains(natal, 11) || !slices.Contains(transiting, 11) {
		t.Errorf("Chiron added by the lens: natal %v transiting %v, want it on both sides", natal, transiting)
	}
}

func TestTimelineTransitSideKeepsTheBinarysExclusions(t *testing.T) {
	// Moon ticked (it is in Main): never on the -t transiting side.
	_, transiting := bandSides(bandTail(t, TimelineTransit, bandReq(t, explicit(append(slices.Clone(measuredMain), 12)...), recipe.AspectPolicy{})))
	if slices.Contains(transiting, 2) {
		t.Errorf("the Moon is on the -t transiting side: %v", transiting)
	}
	// Ascendant and the 11th cusp ticked: on the natal side, never transiting.
	natal, transiting := bandSides(bandTail(t, TimelineTransit, bandReq(t, explicit(1, 22, 32), recipe.AspectPolicy{})))
	if !slices.Contains(natal, 22) || !slices.Contains(natal, 32) {
		t.Errorf("natal side %v lacks the ticked angle and cusp", natal)
	}
	for _, n := range []int{22, 32} {
		if slices.Contains(transiting, n) {
			t.Errorf("cusp point %d is on the -t transiting side: %v", n, transiting)
		}
	}
}

func TestTimelineReturnSideMirrorsTheNatalSide(t *testing.T) {
	natal, transiting := bandSides(bandTail(t, TimelineReturn, bandReq(t, explicit(1, 2, 22, 32), recipe.AspectPolicy{})))
	if !slices.Contains(natal, 2) || !slices.Contains(transiting, 2) {
		t.Errorf("Moon ticked: natal %v transiting %v, want it on both sides of -tr", natal, transiting)
	}
	if !slices.Contains(natal, 32) || slices.Contains(transiting, 32) || slices.Contains(transiting, 22) {
		t.Errorf("cusps: natal %v transiting %v, want them on the natal side only", natal, transiting)
	}
}

func TestTimelineEmptySetIsBare(t *testing.T) {
	for _, src := range []TimelineSource{TimelineTransit, TimelineReturn} {
		if got := bandTail(t, src, bandReq(t, explicit(), recipe.AspectPolicy{})); got != "-R0 -RT0" {
			t.Errorf("%s: an empty set emitted %q, want the bare \"-R0 -RT0\"", src, got)
		}
	}
	if got := bandTail(t, TimelineSky, bandReq(t, explicit(), recipe.AspectPolicy{})); got != "-R0" {
		t.Errorf("sky: an empty set emitted %q, want the bare \"-R0\"", got)
	}
}

func TestTimelineMarksNarrowTheNatalSide(t *testing.T) {
	o := defaultLens()
	marked := bandTail(t, TimelineTransit, natalReq(t, o, withMarks(o, 1)))
	plain := bandTail(t, TimelineTransit, natalReq(t, o, o))
	natal, transiting := bandSides(marked)
	_, plainTransiting := bandSides(plain)
	if !slices.Equal(natal, []int{1}) {
		t.Errorf("marks {1}: natal side %v, want [1] (band %q)", natal, marked)
	}
	if !slices.Equal(transiting, plainTransiting) {
		t.Errorf("marks touched the transiting side: %v, without marks %v", transiting, plainTransiting)
	}
	// A cusp-point mark narrows the -t natal side only.
	natal, _ = bandSides(bandTail(t, TimelineTransit, natalReq(t, o, withMarks(explicit(1, 2, 32), 32))))
	if !slices.Equal(natal, []int{32}) {
		t.Errorf("mark on a cusp: natal side %v, want [32]", natal)
	}
}

func TestTimelineDrainedMarksFallBack(t *testing.T) {
	av := Availability{Absent: map[int]string{12: "seas_18.se1"}, Default: measuredMain}
	withAvailability(t, av)
	o := explicit(append(slices.Clone(measuredMain), 11, 12)...)
	got := bandTail(t, TimelineTransit, natalReq(t, o, withMarks(o, 12)))
	want := bandTail(t, TimelineTransit, natalReq(t, o, o))
	if got != want {
		t.Errorf("a mark on an absent object alone: band %q, want the unnarrowed %q", got, want)
	}
	if natal, _ := bandSides(got); !slices.Contains(natal, 11) || slices.Contains(natal, 12) {
		t.Errorf("drained marks: natal side %v, want the unnarrowed list without the absent 12 (%q)", natal, got)
	}
}

// Marks only on an object the chart does not draw (it has its file, the
// chart's set just does not list it) leave the natal side un-narrowed, as marks
// that absence alone empties do: a bare -R0 would be a search of no body.
func TestTimelineMarksOutsideTheDrawnSetLeaveTheNatalSideAlone(t *testing.T) {
	withAvailability(t, Availability{Default: measuredMain})
	o := explicit(measuredMain...)
	got := bandTail(t, TimelineTransit, natalReq(t, o, withMarks(o, 11)))
	want := bandTail(t, TimelineTransit, natalReq(t, o, o))
	if got != want {
		t.Errorf("a mark on an undrawn object alone: band %q, want the unnarrowed %q", got, want)
	}
	if natal, _ := bandSides(got); len(natal) == 0 && strings.Contains(got, "-R0") {
		t.Errorf("a mark on an undrawn object emptied the natal side: %q", got)
	}
}

func TestTimelineSkyMarks(t *testing.T) {
	// Step 0 (tmp/orders/out/timeline-objects-build/step0.md): -RO is last-wins and
	// drops other bodies' ingresses, so the sky marks are the exact -R0 list.
	o := defaultLens()
	if got := bandTail(t, TimelineSky, bandReq(t, withMarks(o, 1, 2), recipe.AspectPolicy{})); got != "-Yn -R0 1 2" {
		t.Errorf("sky marks {1,2}: band %q, want \"-Yn -R0 1 2\"", got)
	}
	if got := bandTail(t, TimelineSky, bandReq(t, withMarks(o, 1), recipe.AspectPolicy{})); strings.Contains(got, "-RO") {
		t.Errorf("sky mark carries -RO: %q", got)
	}
	// The marks of the natal never reach the sky search.
	if got := bandTail(t, TimelineSky, natalReq(t, o, withMarks(o, 1))); got != "-Yn" {
		t.Errorf("a natal's marks changed the sky band: %q", got)
	}
}

// The now slot's marks narrow the transiting side, as Astrolog's -RT0 list;
// the chart's marks narrow the natal side (operator, 2026-11-04).
func TestTimelineNowMarksNarrowTheTransitingSide(t *testing.T) {
	o := defaultLens()
	plainNatal, _ := bandSides(bandTail(t, TimelineTransit, natalReq(t, o, o)))
	for _, src := range []TimelineSource{TimelineTransit, TimelineReturn} {
		natal, transiting := bandSides(bandTail(t, src, natalReq(t, withMarks(o, 5), o)))
		if !slices.Equal(transiting, []int{5}) {
			t.Errorf("%s, now marks {5}: transiting side %v, want [5]", src, transiting)
		}
		if src == TimelineTransit && !slices.Equal(natal, plainNatal) {
			t.Errorf("now marks touched the natal side: %v, without marks %v", natal, plainNatal)
		}
	}
	natal, transiting := bandSides(bandTail(t, TimelineTransit, natalReq(t, withMarks(o, 5), withMarks(o, 1))))
	if !slices.Equal(natal, []int{1}) || !slices.Equal(transiting, []int{5}) {
		t.Errorf("chart marks {1}, now marks {5}: natal %v transiting %v, want [1] and [5]", natal, transiting)
	}
}

// Marks never add: a now mark on the transiting Moon, which the binary keeps
// off its transit side, does not put the Moon on that side. The mark leaves
// nothing on the side, so it narrows nothing either: the side is what it is
// without the mark, never a bare -RT0 (the old expectation here, before marks
// that leave nothing were dropped on the transiting side as on the natal one).
func TestTimelineNowMarkNeverAddsToTheTransitingSide(t *testing.T) {
	o := defaultLens()
	plain := bandTail(t, TimelineTransit, natalReq(t, o, o))
	marked := bandTail(t, TimelineTransit, natalReq(t, withMarks(o, 2), o))
	if _, transiting := bandSides(marked); slices.Contains(transiting, 2) {
		t.Errorf("now marks {2}: the transiting side %v carries the Moon the binary keeps off it", transiting)
	}
	if marked != plain {
		t.Errorf("now marks {2}: band %q, want the unnarrowed %q (a mark that leaves nothing narrows nothing)", marked, plain)
	}
}

// Now marks on an object neither transiting side lists (it is not in the chart's
// set) narrow neither: the same sides as without the marks.
func TestTimelineNowMarksOutsideTheSidesLeaveThemAlone(t *testing.T) {
	withAvailability(t, Availability{Default: measuredMain})
	o := explicit(measuredMain...)
	for _, src := range []TimelineSource{TimelineTransit, TimelineReturn} {
		got := bandTail(t, src, natalReq(t, withMarks(o, 11), o))
		want := bandTail(t, src, natalReq(t, o, o))
		if got != want {
			t.Errorf("%s, now marks {11} outside the chart's set: band %q, want the unnarrowed %q", src, got, want)
		}
		if _, transiting := bandSides(got); transiting != nil && len(transiting) == 0 {
			t.Errorf("%s: the marks emptied the transiting side: %q", src, got)
		}
	}
}

// Now marks that absence alone empties narrow nothing.
func TestTimelineDrainedNowMarksFallBack(t *testing.T) {
	withAvailability(t, Availability{Absent: map[int]string{12: "seas_18.se1"}})
	o := explicit(append(slices.Clone(measuredMain), 11, 12)...)
	got := bandTail(t, TimelineTransit, natalReq(t, withMarks(o, 12), o))
	want := bandTail(t, TimelineTransit, natalReq(t, o, o))
	if got != want {
		t.Errorf("a now mark on an absent object alone: band %q, want the unnarrowed %q", got, want)
	}
}
