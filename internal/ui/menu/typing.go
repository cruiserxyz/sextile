package menu

import "sort"

// RankByTier orders items best tier first, keeping the list's own order within
// a tier. tiers[i] is items[i]'s location.MatchTier.
func RankByTier[T any](items []T, tiers []int) []T {
	idx := make([]int, len(items))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return tiers[idx[a]] > tiers[idx[b]] })
	out := make([]T, len(items))
	for i, k := range idx {
		out[i] = items[k]
	}
	return out
}

// CursorAfterTyping is where a popup's combined cursor goes when its filter
// text changes: on the first matching item, below the pinned group rows, so
// Space ticks what was typed for (the specification).
// With nothing matching it stays where it was, clamped.
func CursorAfterTyping(cur, pinned, matches int) int {
	if matches > 0 {
		return pinned
	}
	return ClampCursor(cur, pinned+matches)
}

// ClampCursor keeps the combined cursor inside the rows that exist. The family
// block never shrinks, so a filter that empties the list parks the cursor on
// the last family row rather than off the end.
func ClampCursor(cur, total int) int {
	if total < 1 {
		return 0
	}
	if cur >= total {
		return total - 1
	}
	if cur < 0 {
		return 0
	}
	return cur
}
