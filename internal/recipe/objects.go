package recipe

import (
	"fmt"
	"sort"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// VisibilityProfile names how an ObjectPolicy says which bodies are visible.
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
	// AstrologDefault is the set astrolog shows with no restrictions applied:
	// the classical ten, the (mean or true) North Node, the Ascendant and the
	// Midheaven. Deltas (Add/Remove) are taken against it.
	//
	// It is deliberately the ZERO value: every caller written before Explicit
	// existed keeps compiling, and keeps producing byte-identical argv.
	AstrologDefault VisibilityProfile = iota

	// Explicit is the operator's own set, carried in ObjectPolicy.Selection as
	// Astrolog's own object numbers and compiled as `-R0 <ascending list>`.
	//
	// The numbers, rather than a BodySet, because BodySet is a set of
	// aspect.Body — seventeen values — and the object registry
	// (`astrolog -R1 -HO`) carries 134, fifty of them fixed stars. A selector
	// bounded by the enum would decide on the operator's behalf, silently,
	// that the other 117 do not exist.
	Explicit
)

func (p VisibilityProfile) valid() bool {
	return p == AstrologDefault || p == Explicit
}

// profileBodies resolves a VisibilityProfile to its set of visible bodies. This
// is semantic (which bodies), not syntactic (their astrolog numbers) — the
// numbers live in the compiler.
//
// Explicit has no answer here and falls to the empty set on purpose: its
// selection is registry numbers, and this package holds no number→Body
// mapping. See Visible, which carries the consequence.
func profileBodies(p VisibilityProfile) BodySet {
	switch p {
	case AstrologDefault:
		return NewBodySet(
			aspect.Sun, aspect.Moon, aspect.Mercury, aspect.Venus, aspect.Mars,
			aspect.Jupiter, aspect.Saturn, aspect.Uranus, aspect.Neptune, aspect.Pluto,
			aspect.NorthNode, aspect.Ascendant, aspect.Midheaven,
		)
	default:
		return NewBodySet()
	}
}

// BodySet is a deterministic set of bodies. Iterate via Members (sorted) so any
// emission derived from it is stable — never range a map directly.
type BodySet map[aspect.Body]bool

// NewBodySet builds a BodySet from the given bodies (duplicates collapse).
func NewBodySet(bodies ...aspect.Body) BodySet {
	s := make(BodySet, len(bodies))
	for _, b := range bodies {
		s[b] = true
	}
	return s
}

// Has reports membership.
func (s BodySet) Has(b aspect.Body) bool { return s[b] }

// Members returns the set's bodies in ascending aspect.Body order — a stable
// iteration order independent of map layout.
func (s BodySet) Members() []aspect.Body {
	out := make([]aspect.Body, 0, len(s))
	for b := range s {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ObjectPolicy describes which bodies the chart should show, as deltas against
// a named baseline profile. Callers express semantics — "also show Chiron",
// "hide Pluto" — never astrolog `-R` toggles; the compiler alone turns the
// deltas into the canonical (sorted, idempotent) restriction switches.
//
//   - Add: bodies to make visible beyond the baseline (must not already be in it)
//   - Remove: baseline bodies to hide (must be in the baseline)
//   - Selection: under Explicit only, the chosen objects as Astrolog's own
//     registry numbers
//   - TrueNodeMode: use astrolog's true node + Black Moon Lilith (-Yn) instead
//     of the mean variants
//   - Only: a narrowing applied AFTER the rest is resolved (see the field)
//
// Add/Remove and Selection are mutually exclusive, and which pair is live is
// decided by Baseline alone — never by whether a field happens to be populated.
type ObjectPolicy struct {
	Baseline     VisibilityProfile
	Add          BodySet
	Remove       BodySet
	TrueNodeMode bool

	// Selection is the operator's chosen objects under Explicit, as Astrolog's
	// own object numbers from `astrolog -R1 -HO`. It is order-free: Selected
	// sorts and deduplicates, so the argv — and therefore the chart — does not
	// depend on the order a modal happened to collect ticks in.
	//
	// These are NOT aspect.Body values. The registry is 0-BASED and runs to
	// 134; the enum is 1-based and names seventeen. Reading one as the other
	// silently draws different bodies.
	//
	// An empty Selection under Explicit means the operator chose NO bodies,
	// which is a different chart from "the operator has not chosen" — the
	// latter is Baseline AstrologDefault. The two are never told apart by
	// length.
	Selection []int

	// Only narrows the resolved visible set to these bodies: what the chart
	// draws is (baseline + Add - Remove, or Selection) intersected with Only,
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
	Only BodySet
}

// knownBody reports whether b is a real, supported body — not BodyUnknown and
// not an out-of-range value (aspect.Body is a uint8, so aspect.Body(99) is
// constructible). Every known body has an astrolog object number in the
// compiler, so rejecting unknown bodies here is what makes the contract "a
// validated policy can always be compiled" actually hold.
func knownBody(b aspect.Body) bool {
	return b != aspect.BodyUnknown && b.String() != "unknown"
}

// validate enforces the policy's invariants: known baseline, only real bodies,
// Add disjoint from the baseline, Remove a subset of the baseline, and
// Add/Remove disjoint. A policy that passes can always be compiled.
func (o ObjectPolicy) validate() error {
	if !o.Baseline.valid() {
		return fmt.Errorf("recipe: ObjectPolicy: unknown VisibilityProfile %d", int(o.Baseline))
	}
	for _, b := range o.Only.Members() {
		if !knownBody(b) {
			return fmt.Errorf("recipe: ObjectPolicy.Only: %v is not a known body", b)
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
	base := profileBodies(o.Baseline)
	for _, b := range o.Add.Members() {
		if !knownBody(b) {
			return fmt.Errorf("recipe: ObjectPolicy.Add: %v is not a known body", b)
		}
		if base.Has(b) {
			return fmt.Errorf("recipe: ObjectPolicy.Add: %s is already in the baseline (use Remove to hide, not Add to show)", b)
		}
		if o.Remove.Has(b) {
			return fmt.Errorf("recipe: ObjectPolicy: %s is in both Add and Remove", b)
		}
	}
	for _, b := range o.Remove.Members() {
		if !knownBody(b) {
			return fmt.Errorf("recipe: ObjectPolicy.Remove: %v is not a known body", b)
		}
		if !base.Has(b) {
			return fmt.Errorf("recipe: ObjectPolicy.Remove: %s is not in the baseline (nothing to hide)", b)
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
// cusps of houses 2 to 12 and the Midheaven among them (see the angle mapping
// bodyObjectNum in internal/astrolog/compiler.go: Ascendant 22, Imum Coeli 25,
// Descendant 28, Midheaven 31). Astrolog computes them for every chart. They
// live here, not in internal/astrolog, because internal/ui may not import
// astrolog and needs the fact (the body popup does not probe them).
const (
	HouseWheelObjectFirst = 22
	HouseWheelObjectLast  = 33
)

// ObjectGroup is one of the body popup's group rows: a name and the Astrolog
// object-number ranges (inclusive) it holds.
type ObjectGroup struct {
	Name   string
	Ranges [][2]int
}

// ObjectGroups is the body popup's grouping (the specification), in
// row order. Astrolog numbers its objects this way (8.00; it prints no groups,
// `-HO` was checked), so the ranges are Astrolog's own numbering and not
// Sextile's. The last group, Other, holds 52-133 and also takes any object
// number no other group covers (see ObjectGroupOf).
var ObjectGroups = []ObjectGroup{
	{"Planets", [][2]int{{0, 10}}},
	{"Points & angles", [][2]int{{16, 33}}},
	{"Asteroids", [][2]int{{11, 15}, {43, 51}}},
	{"Uranians", [][2]int{{34, 42}}},
	{"Other", [][2]int{{52, 133}}},
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

// OnlyObject is this policy narrowed to the single object n, by registry
// number: Explicit, with the same node mode. Nothing else of the policy
// survives — not the baseline, not the deltas, not a selection, not Only — so
// no other body can answer for n. The body popup's ephemeris probe is its one
// user. (It was named Only until the Only field took the name.)
func (o ObjectPolicy) OnlyObject(n int) ObjectPolicy {
	return ObjectPolicy{Baseline: Explicit, Selection: []int{n}, TrueNodeMode: o.TrueNodeMode}
}

// Visible resolves the policy to the full set of bodies the chart draws:
// the baseline profile plus Add minus Remove, intersected with Only when Only
// has members. Emissions that must cover
// "every drawn body" (the earlier change highlight tiers) derive from this set, so
// they can never recolor a body the wheel does not show.
//
// It is NOT meaningful under Explicit and reports the empty set there: it
// answers in aspect.Body, the selection is registry numbers, and the
// number→Body mapping lives in the compiler rather than here. The consequence
// is real and is not settled in this slice — highlightMembers drops bodies the
// chart does not draw, so an Explicit chart's highlight degrades to the plain
// wheel. That is C1a's row, where the explicit set meets the highlight
// composite; TestObjectPolicy_VisibleIsNotMeaningfulUnderExplicit is the
// marker that keeps it from being forgotten.
func (o ObjectPolicy) Visible() BodySet {
	out := make(BodySet)
	for b := range profileBodies(o.Baseline) {
		out[b] = true
	}
	for b := range o.Add {
		out[b] = true
	}
	for b := range o.Remove {
		delete(out, b)
	}
	if len(o.Only) > 0 {
		for b := range out {
			if !o.Only.Has(b) {
				delete(out, b)
			}
		}
	}
	return out
}

// Toggles returns the bodies that differ from the baseline — every body whose
// visibility the compiler must flip. It is exactly Add ∪ Remove (both compile to
// astrolog `-R` toggles), in a stable order. The compiler maps these to object
// numbers and emits them sorted; this method only needs to be deterministic.
func (o ObjectPolicy) Toggles() []aspect.Body {
	seen := make(BodySet, len(o.Add)+len(o.Remove))
	for b := range o.Add {
		seen[b] = true
	}
	for b := range o.Remove {
		seen[b] = true
	}
	return seen.Members()
}
