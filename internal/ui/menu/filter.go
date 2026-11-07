package menu

import (
	"strings"

	"github.com/cruiserxyz/sextile/internal/location"
)

// FilterMode is how a Filter's query matches a row.
type FilterMode int

const (
	FilterExact     FilterMode = iota // plain substring, case-folded
	FilterFuzzy                       // the typed letters in order, case-folded
	FilterExactCase                   // plain substring, case-sensitive
)

// Filter is the one strategy any list view applies for its `/` query
// (the specification). The query is what was typed after the `/` that
// opened the line, so the mode comes from its own leading slashes: none is an
// exact substring ignoring case, one is fuzzy, two are an exact substring that
// respects case. The view keeps its own query and passes the count of leading
// rows it declares fixed.
type Filter struct {
	Query string
}

// Mode reads the leading slashes of the query.
func (f Filter) Mode() FilterMode {
	switch {
	case strings.HasPrefix(f.Query, "//"):
		return FilterExactCase
	case strings.HasPrefix(f.Query, "/"):
		return FilterFuzzy
	}
	return FilterExact
}

// needle is the query with its mode slashes stripped.
func (f Filter) needle() string {
	switch f.Mode() {
	case FilterExactCase:
		return f.Query[2:]
	case FilterFuzzy:
		return f.Query[1:]
	}
	return f.Query
}

// Shown is the kept-query line text: the `/` that opened the line, then the query.
func (f Filter) Shown() string { return "/" + f.Query }

// Matches says whether one row passes.
func (f Filter) Matches(row string) bool {
	q := f.needle()
	switch f.Mode() {
	case FilterExactCase:
		return strings.Contains(row, q)
	case FilterFuzzy:
		return location.MatchTier(strings.ToLower(row), strings.ToLower(q)) > 0
	}
	return strings.Contains(strings.ToLower(row), strings.ToLower(q))
}

// Apply returns the indices kept, in order: the first `fixed` rows always, the
// rest when they match. n counts the kept rows beyond the fixed ones and m all
// rows beyond them, which is the `N of M` the view prints.
func (f Filter) Apply(rows []string, fixed int) (kept []int, n, m int) {
	fixed = max(0, min(fixed, len(rows)))
	kept = make([]int, 0, len(rows))
	for i, r := range rows {
		if i < fixed {
			kept = append(kept, i)
			continue
		}
		m++
		if f.Matches(r) {
			kept = append(kept, i)
			n++
		}
	}
	return kept, n, m
}
