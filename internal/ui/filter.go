package ui

import (
	"strings"

	"github.com/cruiserxyz/sextile/internal/location"
)

// filterMode is how a listFilter's query matches a row.
type filterMode int

const (
	filterExact     filterMode = iota // plain substring, case-folded
	filterFuzzy                       // the typed letters in order, case-folded
	filterExactCase                   // plain substring, case-sensitive
)

// listFilter is the one strategy any list view applies for its `/` query
// (the specification). The query is what was typed after the `/` that
// opened the line, so the mode comes from its own leading slashes: none is an
// exact substring ignoring case, one is fuzzy, two are an exact substring that
// respects case. The view keeps its own query and passes the count of leading
// rows it declares fixed.
type listFilter struct {
	query string
}

// mode reads the leading slashes of the query.
func (f listFilter) mode() filterMode {
	switch {
	case strings.HasPrefix(f.query, "//"):
		return filterExactCase
	case strings.HasPrefix(f.query, "/"):
		return filterFuzzy
	}
	return filterExact
}

// needle is the query with its mode slashes stripped.
func (f listFilter) needle() string {
	switch f.mode() {
	case filterExactCase:
		return f.query[2:]
	case filterFuzzy:
		return f.query[1:]
	}
	return f.query
}

// shown is the kept-query line text: the `/` that opened the line, then the query.
func (f listFilter) shown() string { return "/" + f.query }

// matches says whether one row passes.
func (f listFilter) matches(row string) bool {
	q := f.needle()
	switch f.mode() {
	case filterExactCase:
		return strings.Contains(row, q)
	case filterFuzzy:
		return location.MatchTier(strings.ToLower(row), strings.ToLower(q)) > 0
	}
	return strings.Contains(strings.ToLower(row), strings.ToLower(q))
}

// apply returns the indices kept, in order: the first `fixed` rows always, the
// rest when they match. n counts the kept rows beyond the fixed ones and m all
// rows beyond them, which is the `N of M` the view prints.
func (f listFilter) apply(rows []string, fixed int) (kept []int, n, m int) {
	fixed = max(0, min(fixed, len(rows)))
	kept = make([]int, 0, len(rows))
	for i, r := range rows {
		if i < fixed {
			kept = append(kept, i)
			continue
		}
		m++
		if f.matches(r) {
			kept = append(kept, i)
			n++
		}
	}
	return kept, n, m
}
