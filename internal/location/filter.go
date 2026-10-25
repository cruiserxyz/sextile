package location

import (
	"regexp"
	"sort"
	"strings"
)

var filterRow = regexp.MustCompile(`^[\x00-\x7f]*$`)

// Indexed is a PlaceInfo paired with a precomputed lowercased search key.
type Indexed struct {
	Place     PlaceInfo
	SearchKey string
}

// NewIndex precomputes search keys for all entries.
func NewIndex(entries []PlaceInfo) []Indexed {
	index := make([]Indexed, len(entries))
	for i := range entries {
		index[i] = Indexed{
			Place:     entries[i],
			SearchKey: lowerASCII(entries[i].Name) + " " + lowerASCII(entries[i].Country),
		}
	}
	return index
}

// Filter applies the four-tier match against query and returns matching entries.
func Filter(index []Indexed, query string, cap int) ([]PlaceInfo, bool) {
	if query == "" {
		return nil, false
	}

	query = lowerASCII(query)

	type match struct {
		place PlaceInfo
		index int
		tier  int
	}

	matches := make([]match, 0)
	for i := range index {
		tier := MatchTier(index[i].SearchKey, query)
		if tier == 0 {
			continue
		}
		// The key also holds the country, so a query that is the whole place
		// name only reaches the starts-with tier there; the name itself decides
		// whether it is an exact match.
		if tier == tierPrefix && lowerASCII(index[i].Place.Name) == query {
			tier = tierExact
		}
		matches = append(matches, match{
			place: index[i].Place,
			index: i,
			tier:  tier,
		})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].tier != matches[j].tier {
			return matches[i].tier > matches[j].tier
		}
		return matches[i].index < matches[j].index
	})

	truncated := false
	if cap > 0 && len(matches) > cap {
		matches = matches[:cap]
		truncated = true
	}

	out := make([]PlaceInfo, len(matches))
	for i := range matches {
		out[i] = matches[i].place
	}
	return out, truncated
}

// The tiers MatchTier returns, best first.
const (
	tierExact    = 4
	tierPrefix   = 3
	tierContains = 2
	tierLoose    = 1
)

// MatchTier is the one four-tier test every picker and fuzzy line uses: 4 when
// key equals query, 3 when key starts with it, 2 when key contains it, 1 when
// query is a subsequence of key, 0 when it does not match. Both arguments must
// already be lowercased by the caller; an empty query is not a match request
// and the callers do not ask.
func MatchTier(key, query string) int {
	switch {
	case query == key:
		return tierExact
	case strings.HasPrefix(key, query):
		return tierPrefix
	case strings.Contains(key, query):
		return tierContains
	case fuzzyMatch(key, query):
		return tierLoose
	}
	return 0
}

func lowerASCII(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func fuzzyMatch(haystack, needle string) bool {
	if needle == "" {
		return true
	}

	j := 0
	for i := 0; i < len(haystack) && j < len(needle); i++ {
		if haystack[i] == needle[j] {
			j++
		}
	}
	return j == len(needle)
}
