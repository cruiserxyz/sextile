package ui

import (
	"reflect"
	"strings"
	"testing"
)

// feed builds the filter from a SHOWN query line (`/sun`, `//sun`): the first
// slash is the one that opened the line, the rest is what was typed.
func feed(shownForm string) listFilter {
	return listFilter{query: strings.TrimPrefix(shownForm, "/")}
}

func TestFilterKeepsTheDeclaredLeadingLines(t *testing.T) {
	rows := []string{"head one", "head two", "head three", "Sun trine Moon", "Mars trine Venus", "Sun sextile Mars"}
	for _, fixed := range []int{0, 1, 3} {
		kept, n, m := feed("/zzzz").apply(rows, fixed)
		if len(kept) != fixed || n != 0 || m != len(rows)-fixed {
			t.Errorf("fixed=%d no match: kept %v n=%d m=%d", fixed, kept, n, m)
		}
		for i := 0; i < fixed; i++ {
			if i >= len(kept) || kept[i] != i {
				t.Errorf("fixed=%d: leading line %d dropped: %v", fixed, i, kept)
			}
		}
		kept, n, m = feed("/sun").apply(rows, fixed)
		want := []int{}
		for i := 0; i < fixed; i++ {
			want = append(want, i)
		}
		for _, i := range []int{3, 5} {
			if i >= fixed {
				want = append(want, i)
			}
		}
		wantN := 2
		if fixed > 3 {
			wantN = 0
		}
		if !reflect.DeepEqual(kept, want) || n != wantN || m != len(rows)-fixed {
			t.Errorf("fixed=%d sun: kept %v n=%d m=%d, want %v n=%d m=%d", fixed, kept, n, m, want, wantN, len(rows)-fixed)
		}
	}
}

func TestFilterModesByPrefix(t *testing.T) {
	cases := []struct {
		shown string
		row   string
		mode  filterMode
		want  bool
	}{
		{"/sn", "Sun", filterExact, false},
		{"/sun", "Sun", filterExact, true},
		{"//sn", "Sun", filterFuzzy, true},
		{"//sun", "Sun", filterFuzzy, true},
		{"///sun", "Sun", filterExactCase, false},
		{"///Sun", "Sun", filterExactCase, true},
	}
	for _, c := range cases {
		f := feed(c.shown)
		if f.shown() != c.shown {
			t.Errorf("shown() = %q, want %q", f.shown(), c.shown)
		}
		if f.mode() != c.mode {
			t.Errorf("%s: mode %v, want %v", c.shown, f.mode(), c.mode)
		}
		kept, _, _ := f.apply([]string{c.row}, 0)
		if got := len(kept) == 1; got != c.want {
			t.Errorf("%s on %q: kept=%v, want %v", c.shown, c.row, got, c.want)
		}
	}
}
