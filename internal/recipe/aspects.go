package recipe

import (
	"fmt"
	"sort"
)

// AspectPolicy says which aspects a chart should count.
//
// The distinction that matters is between "the operator has not chosen" and
// "the operator chose none", because they compile to different argv and to
// different charts. The zero value is the former, which is why adding this
// field to ChartSpec changed no existing caller: an unset policy emits no
// aspect switch at all and whatever the host's astrolog.as sets still stands.
//
//   - Explicit false — no override. Nothing is emitted. This is the startup
//     state and the state a cancelled modal returns to.
//   - Explicit true, Indices empty — the operator chose no aspects. A bare
//     `-RA0` is emitted and the chart counts none.
//   - Explicit true, Indices set — exactly those aspects.
//
// Indices are Astrolog's own registry indices, as read from `astrolog -HA` at
// startup. They are NOT aspect.AspectKind values: Astrolog's registry runs to
// 24 and orders by importance, where the enum names 11 and orders by angle.
// Carrying the registry's own numbers is what lets the operator select the
// thirteen aspects the enum cannot name.
type AspectPolicy struct {
	Explicit bool
	Indices  []int
}

// Selected returns the chosen indices, ascending and deduplicated. It is the
// only ordering the compiler emits, so a caller may build Indices in any order
// without changing the argv — and therefore without changing the chart.
func (a AspectPolicy) Selected() []int {
	if len(a.Indices) == 0 {
		return nil
	}
	seen := make(map[int]bool, len(a.Indices))
	out := make([]int, 0, len(a.Indices))
	for _, n := range a.Indices {
		if seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

// validate reports whether the policy is well-formed. Indices are meaningless
// without Explicit, so carrying them there is a caller bug rather than a
// harmless leftover: it reads as a chosen set that will never be emitted.
func (a AspectPolicy) validate() error {
	if !a.Explicit && len(a.Indices) > 0 {
		return fmt.Errorf("recipe: AspectPolicy: %d indices with Explicit=false (they would never be emitted)", len(a.Indices))
	}
	for _, n := range a.Indices {
		if n < 1 {
			return fmt.Errorf("recipe: AspectPolicy: aspect index %d is not a registry index (they are 1-based)", n)
		}
	}
	return nil
}
