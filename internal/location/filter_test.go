package location

import (
	"strings"
	"testing"
)

func TestFilter_EmptyQuery(t *testing.T) {
	matches, truncated := Filter(NewIndex([]PlaceInfo{{Name: "Berlin", Country: "DE"}}), "", 5)
	if matches != nil {
		t.Fatalf("Filter() matches = %#v, want nil", matches)
	}
	if truncated {
		t.Fatal("Filter() truncated = true, want false")
	}
}

func TestFilter_Exact(t *testing.T) {
	index := NewIndex([]PlaceInfo{
		{Name: "Berlin", Country: "DE"},
		{Name: "Bern", Country: "CH"},
	})

	matches, truncated := Filter(index, "berlin de", 0)
	if truncated {
		t.Fatal("Filter() truncated = true, want false")
	}
	if len(matches) != 1 || matches[0].Name != "Berlin" || matches[0].Country != "DE" {
		t.Fatalf("Filter() matches = %#v, want Berlin DE only", matches)
	}
}

func TestFilter_SubstringBeatsExactlessness(t *testing.T) {
	index := NewIndex([]PlaceInfo{
		{Name: "Berlin", Country: "DE"},
		{Name: "Berlinerburg", Country: "DE"},
		{Name: "Athens", Country: "GR"},
	})

	matches, _ := Filter(index, "berl", 0)
	if len(matches) != 2 {
		t.Fatalf("len(Filter()) = %d, want 2", len(matches))
	}
	if matches[0].Name != "Berlin" || matches[1].Name != "Berlinerburg" {
		t.Fatalf("Filter() order = %#v, want Berlin then Berlinerburg", matches)
	}
}

func TestFilter_FuzzySequential(t *testing.T) {
	index := NewIndex([]PlaceInfo{{Name: "Berlin", Country: "DE"}})

	matches, _ := Filter(index, "brl", 0)
	if len(matches) != 1 || matches[0].Name != "Berlin" {
		t.Fatalf("Filter() matches = %#v, want Berlin", matches)
	}
}

func TestFilter_TierOrdering(t *testing.T) {
	index := NewIndex([]PlaceInfo{
		{Name: "Ber", Country: "US"},
		{Name: "X Ber", Country: "US"},
		{Name: "BxxExxR", Country: "US"},
	})

	matches, _ := Filter(index, "ber us", 0)
	if len(matches) != 3 {
		t.Fatalf("len(Filter()) = %d, want 3", len(matches))
	}
	if matches[0].Name != "Ber" || matches[1].Name != "X Ber" || matches[2].Name != "BxxExxR" {
		t.Fatalf("Filter() order = %#v, want exact, substring, fuzzy", matches)
	}
}

func TestFilter_StableInTier(t *testing.T) {
	entries := []PlaceInfo{
		{Name: "Alpha", Country: "US"},
		{Name: "Bravo", Country: "US"},
		{Name: "Charlie", Country: "US"},
		{Name: "Delta", Country: "US"},
		{Name: "Echo", Country: "US"},
		{Name: "X Berl", Country: "US"},
		{Name: "Foxtrot", Country: "US"},
		{Name: "Golf", Country: "US"},
		{Name: "Hotel", Country: "US"},
		{Name: "India", Country: "US"},
		{Name: "Juliet", Country: "US"},
		{Name: "Kilo", Country: "US"},
		{Name: "Y Berl", Country: "US"},
	}

	matches, _ := Filter(NewIndex(entries), "berl", 0)
	if len(matches) != 2 {
		t.Fatalf("len(Filter()) = %d, want 2", len(matches))
	}
	if matches[0].Name != "X Berl" || matches[1].Name != "Y Berl" {
		t.Fatalf("Filter() order = %#v, want source order preserved", matches)
	}
}

func TestFilter_Truncation(t *testing.T) {
	entries := make([]PlaceInfo, 10)
	for i := range entries {
		entries[i] = PlaceInfo{Name: "Berlin", Country: "DE"}
	}

	matches, truncated := Filter(NewIndex(entries), "ber", 3)
	if len(matches) != 3 {
		t.Fatalf("len(Filter()) = %d, want 3", len(matches))
	}
	if !truncated {
		t.Fatal("Filter() truncated = false, want true")
	}
}

func TestFilter_NonASCIIPassThrough(t *testing.T) {
	index := NewIndex([]PlaceInfo{{Name: "münchen", Country: "DE"}})

	matches, _ := Filter(index, "München", 0)
	if len(matches) != 1 || matches[0].Name != "münchen" {
		t.Fatalf("Filter() matches = %#v, want münchen", matches)
	}
}

func TestFilter_NoCapMeansAll(t *testing.T) {
	index := NewIndex([]PlaceInfo{
		{Name: "Berlin", Country: "DE"},
		{Name: "Berne", Country: "CH"},
		{Name: "Bergen", Country: "NO"},
	})

	matches, truncated := Filter(index, "be", 0)
	if len(matches) != 3 {
		t.Fatalf("len(Filter()) = %d, want 3", len(matches))
	}
	if truncated {
		t.Fatal("Filter() truncated = true, want false")
	}
}

func TestMatchTierRanksExactPrefixSubstringSubsequence(t *testing.T) {
	for _, tc := range []struct {
		key, query string
		want       int
	}{
		{"paris france", "paris france", 4},
		{"paris france", "paris", 3},
		{"paris france", "france", 2},
		{"paris france", "prfr", 1},
		{"paris france", "xyz", 0},
	} {
		if got := MatchTier(tc.key, tc.query); got != tc.want {
			t.Errorf("MatchTier(%q, %q) = %d, want %d", tc.key, tc.query, got, tc.want)
		}
	}
}

func TestFilter_ExactNameRanksFirst(t *testing.T) {
	index := NewIndex([]PlaceInfo{
		{Name: "Parisot", Country: "FR"},
		{Name: "Paris Hill", Country: "US"},
		{Name: "Paris", Country: "FR"},
		{Name: "Le Paris", Country: "FR"},
	})
	matches, _ := Filter(index, "paris", 0)
	got := make([]string, len(matches))
	for i, p := range matches {
		got[i] = p.Name
	}
	want := []string{"Paris", "Parisot", "Paris Hill", "Le Paris"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("Filter(paris) = %v, want exact name, then starts-with, then contains: %v", got, want)
	}
}

// The city chooser's example query is true: the key is `name country`, so a
// code after the name (no comma) puts Paris, FR ahead of Paris, TX.
func TestFilter_ExampleQueryRanksFirst(t *testing.T) {
	index := NewIndex([]PlaceInfo{
		{Name: "Paris", Country: "TX"},
		{Name: "Paris", Country: "FR"},
	})
	matches, _ := Filter(index, "paris fr", 0)
	if len(matches) == 0 || matches[0].Name != "Paris" || matches[0].Country != "FR" {
		t.Fatalf("Filter(paris fr) = %#v, want Paris FR first", matches)
	}
	if got, _ := Filter(index, "paris, fr", 0); len(got) != 0 {
		t.Errorf("a comma should not match: %#v", got)
	}
}
