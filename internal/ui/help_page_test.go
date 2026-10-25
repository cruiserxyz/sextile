package ui

import "testing"

// A7: the Keys popup has no cursor, so Home/End (and g/G, it has nothing to
// type) go to the top and bottom of the text; PgUp/PgDn keep paging by a page.
func TestHelpHomeEnd(t *testing.T) {
	m, _ := press(t, helpFixture(t, 80, 24), "?")
	if !m.helpOpen {
		t.Fatal("fixture: Help did not open")
	}
	h := helpListHeight(24)
	last := len(m.helpLines()) - h
	if last <= 0 {
		t.Fatalf("fixture: the text (%d lines) fits one page of %d", len(m.helpLines()), h)
	}
	for _, tc := range []struct {
		key  string
		want int
	}{{"end", last}, {"home", 0}, {"G", last}, {"g", 0}, {"pgdown", h}, {"end", last}, {"pgup", last - h}} {
		m, _ = press(t, m, tc.key)
		if m.help.offset != tc.want {
			t.Errorf("%s: offset %d, want %d", tc.key, m.help.offset, tc.want)
		}
		if !m.helpOpen {
			t.Fatalf("%s closed the popup", tc.key)
		}
	}
}
