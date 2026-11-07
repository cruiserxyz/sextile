package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"reflect"
	"strings"
	"testing"
)

// feed builds the filter from a SHOWN query line (`/sun`, `//sun`): the first
// slash is the one that opened the line, the rest is what was typed.
func feed(shownForm string) menu.Filter {
	return menu.Filter{Query: strings.TrimPrefix(shownForm, "/")}
}

func TestFilterKeepsTheDeclaredLeadingLines(t *testing.T) {
	rows := []string{"head one", "head two", "head three", "Sun trine Moon", "Mars trine Venus", "Sun sextile Mars"}
	for _, fixed := range []int{0, 1, 3} {
		kept, n, m := feed("/zzzz").Apply(rows, fixed)
		if len(kept) != fixed || n != 0 || m != len(rows)-fixed {
			t.Errorf("fixed=%d no match: kept %v n=%d m=%d", fixed, kept, n, m)
		}
		for i := 0; i < fixed; i++ {
			if i >= len(kept) || kept[i] != i {
				t.Errorf("fixed=%d: leading line %d dropped: %v", fixed, i, kept)
			}
		}
		kept, n, m = feed("/sun").Apply(rows, fixed)
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
		mode  menu.FilterMode
		want  bool
	}{
		{"/sn", "Sun", menu.FilterExact, false},
		{"/sun", "Sun", menu.FilterExact, true},
		{"//sn", "Sun", menu.FilterFuzzy, true},
		{"//sun", "Sun", menu.FilterFuzzy, true},
		{"///sun", "Sun", menu.FilterExactCase, false},
		{"///Sun", "Sun", menu.FilterExactCase, true},
	}
	for _, c := range cases {
		f := feed(c.shown)
		if f.Shown() != c.shown {
			t.Errorf("shown() = %q, want %q", f.Shown(), c.shown)
		}
		if f.Mode() != c.mode {
			t.Errorf("%s: mode %v, want %v", c.shown, f.Mode(), c.mode)
		}
		kept, _, _ := f.Apply([]string{c.row}, 0)
		if got := len(kept) == 1; got != c.want {
			t.Errorf("%s on %q: kept=%v, want %v", c.shown, c.row, got, c.want)
		}
	}
}
