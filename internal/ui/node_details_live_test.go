package ui

// The Details title of the three objects names the form of the call that cast
// the report, against the real binary.

import (
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// TestDetailsTitleNamesTheNodeForm is N15: `North Node (true)`, `South Node
// (mean)`, `Lilith (natural)`, the words coming from the pair on the call that
// produced the report, so a comparison report names the base's form.
func TestDetailsTitleNamesTheNodeForm(t *testing.T) {
	titleOf := func(t *testing.T, m Model, name string) string {
		t.Helper()
		m.focused = PanelBodies
		for i := 0; i < 60; i++ {
			if o, ok := bodyUnderCursor(m); ok && o.Name == name {
				title, _, ok := m.detailsBodyTitle()
				if !ok {
					t.Fatal("no Details title for the row")
				}
				return title
			}
			m.bodyCursorIdx = i
		}
		t.Fatalf("no Bodies row for %s:\n%s", name, ansi.Strip(m.View()))
		return ""
	}
	t.Run("a single chart, in the chart's own form", func(t *testing.T) {
		for _, tc := range []struct {
			form recipe.NodeForm
			want map[string]string
		}{
			{recipe.NodesTrueNatural, map[string]string{"North Node": "[0] North Node (true)", "South Node": "[0] South Node (true)", "Lilith": "[0] Lilith (natural)"}},
			{recipe.NodesMean, map[string]string{"North Node": "[0] North Node (mean)", "South Node": "[0] South Node (mean)", "Lilith": "[0] Lilith (mean)"}},
		} {
			m, _, _ := liveComparison(t, tc.form, tc.form)
			m.slots.SetSlotSelected(0, false)
			m.slots.SetSlotSelected(2, false)
			m.base = nil
			m, cmd := m.noteWant(surfaceText, true)
			m = pumpPatient(t, m, cmd)
			for name, want := range tc.want {
				if got := titleOf(t, m, name); got != want {
					t.Errorf("form %v: the title of %s is %q, want %q", tc.form, name, got, want)
				}
			}
		}
	})
	t.Run("a comparison report names the base's form", func(t *testing.T) {
		m, _, _ := liveComparison(t, recipe.NodesMeanNatural, recipe.NodesTrue)
		if sc := m.deriveScene(); sc.pair == nil {
			t.Fatal("setup: no pair on screen")
		}
		for name, want := range map[string]string{"North Node": "[0] North Node (mean)", "Lilith": "[0] Lilith (natural)"} {
			got := titleOf(t, m, name)
			if got[:len(want)] != want {
				t.Errorf("the title of %s is %q, want it to start with %q (the base's form, not the chart's own true)", name, got, want)
			}
		}
	})
}

// bodyUnderCursor is the object of the Bodies row the cursor stands on.
func bodyUnderCursor(m Model) (aspect.Object, bool) {
	ch := m.shownChart()
	return panel.ObjectAtCursor(ch.positions, ch.cusps, m.bodyCursorIdx)
}

// The words of the title are those of the call that cast the report: a request
// that asked for the true form, answered by a call that cast the mean one,
// names the mean one.
func TestDetailsTitleFollowsTheCall(t *testing.T) {
	node := aspect.Object{Name: "North Node"}
	lilith := aspect.Object{Name: "Lilith"}
	for _, tc := range []struct {
		o              aspect.Object
		title, command string
		want           string
	}{
		// Commands as the console shows them: formatCommand quotes every `=` word.
		{node, "[0] North Node (true)", "astrolog -qj 1 _Yn _Ynn -R0 16", "[0] North Node (mean)"},
		{lilith, "[0] Lilith (mean) [2] in [3]", "astrolog '=Yn' _Ynn -r0 a b _Yn '=Ynn'", "[0] Lilith (natural) [2] in [3]"},
		{node, "[0] North Node (mean)", "astrolog _Yn _Ynn -R0 16 '=Yn' _Ynn", "[0] North Node (true)"},
		{node, "[0] North Node (mean)", "astrolog _Yn _Ynn", "[0] North Node (mean)"},
		{aspect.Object{Name: "Sun"}, "[0] Sun", "astrolog '=Yn' '=Ynn'", "[0] Sun"},
	} {
		if got := nodeTitleFromCall(tc.title, tc.o, lookup.CommandNodeForm(tc.command)); got != tc.want {
			t.Errorf("%q after %q = %q, want %q", tc.title, tc.command, got, tc.want)
		}
	}
}
