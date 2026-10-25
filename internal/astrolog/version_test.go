package astrolog

import "testing"

func TestVersionParse(t *testing.T) {
	for _, c := range []struct {
		name, in, want string
		ok             bool
	}{
		{"banner", "|   ** Astrolog version 8.00 **   |", "8.00", true},
		{"end of output", "Astrolog version 7.99", "7.99", true},
		{"third component is not 8.00", "|   ** Astrolog version 8.00.1 **   |", "", false},
		{"one decimal is not N.NN", "Astrolog version 8.0 **", "", false},
		{"three decimals is not N.NN", "Astrolog version 8.000 **", "", false},
		{"no line", "hello", "", false},
	} {
		got, ok := parseVersion(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: parseVersion(%q) = %q, %v; want %q, %v", c.name, c.in, got, ok, c.want, c.ok)
		}
	}
}
