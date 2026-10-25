package store

import "testing"

// the specification C9 and C17: the two display colours are read
// from preferences, an absent or empty entry means the default, and a
// row_stripe entry is reported as present with or without a value, so ui can
// say once that it is retired.
func TestReadDisplayBlock(t *testing.T) {
	cases := []struct {
		name, text    string
		focus, row    string
		stripePresent bool
	}{
		{"a palette number", "display:\n  focus_color: 10\n", "10", "", false},
		{"a quoted hex colour", "display:\n  cursor_row_color: \"#1f6f6f\"\n", "", "#1f6f6f", false},
		{"an inline comment", "display:\n  focus_color: 10  # green\n", "10", "", false},
		{"a quoted hex colour and a comment", "display:\n  cursor_row_color: \"#1f6f6f\" # teal\n", "", "#1f6f6f", false},
		{"both", "display:\n  focus_color: 2\n  cursor_row_color: 4\n", "2", "4", false},
		{"only a comment", "display:\n  focus_color: # none yet\n", "", "", false},
		{"an empty entry", "display:\n  focus_color:\n", "", "", false},
		{"a stripe with a value", "display:\n  row_stripe: 58\n", "", "", true},
		{"a stripe with no value", "display:\n  row_stripe:\n", "", "", true},
		{"no display block", "keys:\n  leader: \",\"\n", "", "", false},
		{"only the first block", "display:\n  focus_color: 10\nkeys:\n  leader: x\ndisplay:\n  focus_color: 100\n", "10", "", false},
		{"another block's entry", "keys:\n  focus_color: 10\n  row_stripe: 58\n", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := prefPath(t)
			seedPreferences(t, path, tc.text)
			got := Display()
			if got.FocusColour != tc.focus || got.CursorRowColour != tc.row || got.RetiredStripePresent != tc.stripePresent {
				t.Errorf("Display() = %+v, want focus %q, cursor row %q, stripe present=%v", got, tc.focus, tc.row, tc.stripePresent)
			}
		})
	}
	t.Run("no file", func(t *testing.T) {
		prefPath(t)
		if got := Display(); got != (DisplayConfig{}) {
			t.Errorf("Display() with no file = %+v, want absent", got)
		}
	})
}

// the specification C17: a row_stripe entry is reported as
// present whether or not it carries a value, so an operator with an empty
// `row_stripe:` still hears that it is retired.
func TestRowStripeRetiredIsReportedWithOrWithoutAValue(t *testing.T) {
	for _, text := range []string{"display:\n  row_stripe: 58\n", "display:\n  row_stripe:\n", "display:\n  row_stripe: # was 58\n"} {
		path := prefPath(t)
		seedPreferences(t, path, text)
		if got := Display(); !got.RetiredStripePresent {
			t.Errorf("Display() for %q = %+v, want row_stripe present", text, got)
		}
	}
}
