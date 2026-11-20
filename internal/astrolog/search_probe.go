package astrolog

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// searchLackingMemory keeps what searchLacking found, by the files Astrolog
// named and the objects the search drew: the sky is searched a day at a time,
// and every day asks the same question.
var (
	searchLackingMu     sync.Mutex
	searchLackingMemory = map[string][]int{}
)

// searchLacking is, for a call that Astrolog warned about with a line
// that names no object (a missing ephemeris file, the natural Lilith's range),
// the objects of the call that lack data. A chart usually prints a no-data row for
// each such object, which says who it is; a search prints none, and an asteroid's
// line names no object of the registry either, so the call
// is run again over halves of its objects until each warning is pinned to one.
// nil for any other call, or when the question cannot be answered.
func searchLacking(ctx context.Context, c BodyCall, warnings []string) []int {
	if len(c.Objects) == 0 {
		return nil
	}
	var unnamed []string
	for _, w := range warnings {
		if namesNoObject(w) {
			unnamed = append(unnamed, w)
		}
	}
	if len(unnamed) == 0 {
		return nil
	}
	key := binary + "\n" + strings.Join(unnamed, "\n") + "\n" + joinInts(c.Objects)
	searchLackingMu.Lock()
	known, ok := searchLackingMemory[key]
	searchLackingMu.Unlock()
	if ok {
		return known
	}
	var lacking func(set []int) []int
	lacking = func(set []int) []int {
		if len(set) == 0 || ctx.Err() != nil {
			return nil
		}
		var heard []string
		probe := c.without(slices.DeleteFunc(slices.Clone(c.Objects), func(n int) bool { return slices.Contains(set, n) })).probingCall()
		probe.heard, probe.searching, probe.leftOut, probe.leftOutLines = &heard, false, nil, nil
		if _, _, err := spawn(ctx, probe); err != nil {
			return nil
		}
		warned := false
		for _, l := range heard {
			warned = warned || namesNoObject(l)
		}
		switch {
		case !warned:
			return nil
		case len(set) == 1:
			return set
		}
		mid := len(set) / 2
		return append(lacking(set[:mid]), lacking(set[mid:])...)
	}
	found := lacking(c.Objects)
	if ctx.Err() == nil {
		searchLackingMu.Lock()
		searchLackingMemory[key] = found
		searchLackingMu.Unlock()
	}
	return found
}

func joinInts(nums []int) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, " ")
}
