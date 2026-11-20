package recipe

import (
	"fmt"
	"slices"
	"sort"
)

// VisibilityProfile names how an ObjectPolicy says which objects are visible.
// Naming the baseline (rather than assuming astrolog's hidden default) is what
// keeps the restriction algebra deterministic and testable.
//
// Two profiles, and the difference between them is what is being described:
// AstrologDefault describes a DIFFERENCE from a baseline, Explicit describes a
// SET. They are not interchangeable spellings of one idea — a delta's argv
// changes when the host's astrolog.as default changes, for a chart nobody
// touched, which is precisely what an operator who picked a set did not ask
// for.
type VisibilityProfile int

const (
	// AstrologDefault is the set the host's own Astrolog shows with no
	// restrictions applied (`astrolog -HO`, read at startup into
	// astrolog.Availability.Default). Deltas (Add/Remove) are taken against it,
	// and it is not known here: this package holds no list of its members, so
	// the answer to "what is drawn" is the compiler's alone
	// (astrolog.drawnObjectNumbers).
	//
	// It is deliberately the ZERO value: every caller written before Explicit
	// existed keeps compiling, and keeps producing byte-identical argv.
	AstrologDefault VisibilityProfile = iota

	// Explicit is the operator's own set, carried in ObjectPolicy.Selection as
	// Astrolog's own object numbers and compiled as `-R0 <ascending list>`.
	//
	// The numbers, rather than a list of Sextile's own, because the
	// object registry (`astrolog -R1 -HO`) carries 134, fifty of them fixed
	// stars. A selector bounded by a Sextile-side list would decide on the
	// operator's behalf, silently, that the others do not exist.
	Explicit
)

func (p VisibilityProfile) valid() bool {
	return p == AstrologDefault || p == Explicit
}

// ObjectSet is a deterministic set of Astrolog object numbers. Iterate via
// Members (ascending).
type ObjectSet map[int]bool

// NewObjectSet builds an ObjectSet from the given numbers (duplicates collapse).
func NewObjectSet(nums ...int) ObjectSet {
	s := make(ObjectSet, len(nums))
	for _, n := range nums {
		s[n] = true
	}
	return s
}

// Has reports membership.
func (s ObjectSet) Has(n int) bool { return s[n] }

// Members returns the numbers in ascending order.
func (s ObjectSet) Members() []int {
	out := make([]int, 0, len(s))
	for n, in := range s {
		if in {
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out
}

// ObjectPolicy describes which objects the chart should show, as deltas against
// a named baseline profile. Callers express semantics — "also show Chiron",
// "hide Pluto" — never astrolog `-R` toggles; the compiler alone turns the
// deltas into the canonical (sorted, idempotent) restriction switches. Every
// set in it holds Astrolog's own object numbers, never a Sextile-side identity.
//
//   - Add: objects to show, whether or not the host's default set lists them
//   - Remove: objects to hide, whether or not it does (neither ever does the
//     opposite: the compiler sends the `-R <n>` switches that take the host's
//     own default set to the result, and none for a name the host already
//     has the right way)
//   - Selection: under Explicit only, the chosen objects as Astrolog's own
//     registry numbers
//   - Nodes: the form of the Nodes and Lilith the chart is cast in (NodeForm);
//     the zero value is Astrolog's own, the mean Node and the mean Lilith
//   - Only: a narrowing applied AFTER the rest is resolved (see the field)
//
// Add/Remove and Selection are mutually exclusive, and which pair is live is
// decided by Baseline alone — never by whether a field happens to be populated.
type ObjectPolicy struct {
	Baseline VisibilityProfile
	Add      ObjectSet
	Remove   ObjectSet
	Nodes    NodeForm

	// Selection is the operator's chosen objects under Explicit, as Astrolog's
	// own object numbers from `astrolog -R1 -HO`. It is order-free: Selected
	// sorts and deduplicates, so the argv — and therefore the chart — does not
	// depend on the order a modal happened to collect ticks in.
	//
	// These are Astrolog's numbers. The registry is 0-BASED and runs to 134.
	//
	// An empty Selection under Explicit means the operator chose NO bodies,
	// which is a different chart from "the operator has not chosen" — the
	// latter is Baseline AstrologDefault. The two are never told apart by
	// length.
	Selection []int

	// Only narrows the resolved visible set to these objects, by Astrolog's
	// object number (any object: Fortune, a fixed star, a cusp point): what the
	// chart draws is (the host default with the Add and Remove flips, or Selection) intersected with Only,
	// and compiled as the exact `-R0` show-list of that intersection. It is how
	// "the selected bodies show only themselves" reaches Astrolog, through the
	// one encoder and no second spelling of the band.
	//
	// Empty (nil or no members) means no narrowing and compiles byte-identical
	// to a policy that never had the field. It is an intersection, never a
	// union: a body the lens or the body chooser already removed stays removed
	// even when it is in Only, so Only can leave nothing at all. It is not part
	// of the chooser's identity — it is set per surface (the selection of the
	// slot on screen), not by the operator's lens — and it is valid under both
	// profiles, which is why it is not covered by the Add/Remove or Selection
	// rules above.
	Only ObjectSet
}

// validate enforces the policy's invariants, all of them structural: known
// baseline, only object numbers (0 or more: object 0 is Earth), Add and Remove
// disjoint, and no Add or Remove under Explicit. Whether an Add is already in
// the host's default set, or a Remove is not, cannot be said here (the default
// set is the host's, read at startup), so it is not checked: such a name draws
// what it says all the same (astrolog.deltaSet). A policy that passes can
// always be compiled: a number needs no table to be lowered.
func (o ObjectPolicy) validate() error {
	if !o.Baseline.valid() {
		return fmt.Errorf("recipe: ObjectPolicy: unknown VisibilityProfile %d", int(o.Baseline))
	}
	for _, n := range o.Only.Members() {
		if n < 0 {
			return fmt.Errorf("recipe: ObjectPolicy.Only: %d is not an object number", n)
		}
	}
	if o.Baseline == Explicit {
		// A delta over an explicit set has no meaning: the policy would carry
		// two descriptions of the visible set and the compiler would have to
		// pick a winner. Refusing here is what keeps that choice from being
		// made silently, one emission at a time.
		if len(o.Add) > 0 {
			return fmt.Errorf("recipe: ObjectPolicy: Add is meaningless under Explicit (the selection IS the visible set)")
		}
		if len(o.Remove) > 0 {
			return fmt.Errorf("recipe: ObjectPolicy: Remove is meaningless under Explicit (the selection IS the visible set)")
		}
		for _, n := range o.Selection {
			// Lower bound 0, not 1: Astrolog's object registry opens with
			// `0 Earth`. The aspect registry is 1-based, so borrowing its
			// bound here would drop Earth and nothing else would notice.
			if n < 0 {
				return fmt.Errorf("recipe: ObjectPolicy: object number %d is not a registry number (they are 0-based)", n)
			}
		}
		return nil
	}
	if len(o.Selection) > 0 {
		// Mirrors AspectPolicy: a selection that will never be emitted reads as
		// a chosen set that silently does nothing, which is a caller bug rather
		// than a harmless leftover.
		return fmt.Errorf("recipe: ObjectPolicy: %d selected objects under a delta baseline (they would never be emitted)", len(o.Selection))
	}
	for _, n := range o.Add.Members() {
		if n < 0 {
			return fmt.Errorf("recipe: ObjectPolicy.Add: %d is not an object number", n)
		}
		if o.Remove.Has(n) {
			return fmt.Errorf("recipe: ObjectPolicy: object %d is in both Add and Remove", n)
		}
	}
	for _, n := range o.Remove.Members() {
		if n < 0 {
			return fmt.Errorf("recipe: ObjectPolicy.Remove: %d is not an object number", n)
		}
	}
	return nil
}

// Selected returns the chosen object numbers, ascending and deduplicated —
// the only ordering the compiler emits. Nil unless the baseline is Explicit,
// so a caller cannot mistake a leftover slice for a live selection.
//
// Mirrors AspectPolicy.Selected, and differs from it in exactly one way: no
// lower bound is applied, because object 0 is Earth.
func (o ObjectPolicy) Selected() []int {
	if o.Baseline != Explicit || len(o.Selection) == 0 {
		return nil
	}
	seen := make(map[int]bool, len(o.Selection))
	out := make([]int, 0, len(o.Selection))
	for _, n := range o.Selection {
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// Astrolog numbers the house-wheel objects 22 through 33: the Ascendant, the
// cusps of houses 2 to 12 and the Midheaven among them (Ascendant 22, Imum
// Coeli 25, Descendant 28, Midheaven 31). Astrolog computes them for every
// chart. They live here, not in internal/astrolog, because internal/ui may not
// import astrolog and needs the fact (the body popup does not probe them).
const (
	HouseWheelObjectFirst = 22
	HouseWheelObjectLast  = 33
)

// ObjectGroup is one of the body popup's group rows: a name and the Astrolog
// object-number ranges (inclusive) it holds. Manual are members the group lists
// and a filter on its name finds, but never ticks: its Space, its mark and its
// two numbers leave them out, so they are ticked by their own row alone.
type ObjectGroup struct {
	Name   string
	Ranges [][2]int
	Manual []int
}

// ObjectGroups is the body popup's grouping (the specification), in
// row order. Astrolog numbers its objects this way (8.00; it prints no groups,
// `-HO` was checked), so the ranges are Astrolog's own numbering and not
// Sextile's. Angles are the four that name a house wheel's axes (the Ascendant
// 22, the Nadir 25, the Descendant 28, the Midheaven 31) and House cusps the
// eight other cusps; Earth (0) is a planet the astrologer ticks on purpose
// (operator, 2026-11-04). The last group, Other, holds 52-133 and also takes any
// object number no other group covers (see ObjectGroupOf).
var ObjectGroups = []ObjectGroup{
	{Name: "Planets", Ranges: [][2]int{{0, 10}}, Manual: []int{0}},
	{Name: "Angles", Ranges: [][2]int{{22, 22}, {25, 25}, {28, 28}, {31, 31}}},
	{Name: "Points", Ranges: [][2]int{{16, 21}}},
	{Name: "House cusps", Ranges: [][2]int{{23, 24}, {26, 27}, {29, 30}, {32, 33}}},
	{Name: "Asteroids", Ranges: [][2]int{{11, 15}, {43, 51}}},
	{Name: "Uranians", Ranges: [][2]int{{34, 42}}},
	{Name: "Other", Ranges: [][2]int{{52, 133}}},
}

// ObjectGroupOf is the index into ObjectGroups an object number belongs to.
// A number the table does not cover goes to Other, the last group.
func ObjectGroupOf(n int) int {
	for i, g := range ObjectGroups {
		for _, r := range g.Ranges {
			if n >= r[0] && n <= r[1] {
				return i
			}
		}
	}
	return len(ObjectGroups) - 1
}

// ObjectManual is true for an object its group lists but leaves out of its
// toggle, its mark and its count (Earth, in Planets).
func ObjectManual(n int) bool {
	return slices.Contains(ObjectGroups[ObjectGroupOf(n)].Manual, n)
}

// OnlyObject is this policy narrowed to the single object n, by registry
// number: Explicit, with the same node mode. Nothing else of the policy
// survives — not the baseline, not the deltas, not a selection, not Only — so
// no other body can answer for n. The body popup's ephemeris probe is its one
// user. (It was named Only until the Only field took the name.)
func (o ObjectPolicy) OnlyObject(n int) ObjectPolicy {
	return ObjectPolicy{Baseline: Explicit, Selection: []int{n}, Nodes: o.Nodes}
}

// Toggles returns the objects the policy names, ascending: Add ∪ Remove, the
// candidates for astrolog `-R` switches, in the one order the compiler emits.
// Which of them need a switch, and what the policy leaves drawn, depend on the
// host's default set and are the compiler's to say (astrolog.deltaFlips,
// astrolog.drawnObjectNumbers).
func (o ObjectPolicy) Toggles() []int {
	seen := make(ObjectSet, len(o.Add)+len(o.Remove))
	for _, n := range o.Add.Members() {
		seen[n] = true
	}
	for _, n := range o.Remove.Members() {
		seen[n] = true
	}
	return seen.Members()
}
