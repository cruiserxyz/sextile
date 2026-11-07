package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/popup"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Specification chooser-absent-mark: an undrawable body's row carries a red `!`
// that the cursor band does not paint over.

var sgrSeq = regexp.MustCompile("\x1b\\[([0-9;]*)m")

// fgBefore is the foreground the terminal would be drawing with at byte offset
// at of s: "" for the default, otherwise the last foreground parameter set.
func fgBefore(s string, at int) string {
	fg := ""
	for _, m := range sgrSeq.FindAllStringSubmatchIndex(s[:at], -1) {
		params := strings.Split(s[m[2]:m[3]], ";")
		for i := 0; i < len(params); i++ {
			p := params[i]
			switch {
			case p == "" || p == "0" || p == "39":
				fg = ""
			case p == "38" && i+1 < len(params):
				// 38;5;n or 38;2;r;g;b
				n := 2
				if params[i+1] == "2" {
					n = 4
				}
				end := min(i+1+n, len(params))
				fg = strings.Join(params[i:end], ";")
				i = end - 1
			case len(p) == 2 && (p[0] == '3' && p[1] >= '0' && p[1] <= '7' || p[0] == '9' && p[1] >= '0' && p[1] <= '7'):
				fg = p
			}
		}
	}
	return fg
}

func absentRowFixture() (popup.Body, int) {
	reg := []aspect.RegistryObject{
		{Object: aspect.Object{Number: 1, Name: "Sun"}}, {Object: aspect.Object{Number: 12, Name: "Ceres"}}, {Object: aspect.Object{Number: 13, Name: "Pallas"}},
	}
	st := popup.Open(reg, recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1}}, nil)
	st.SetAbsent(map[int]string{12: "seas_18.se1"}, map[int]string{12: "SwissEph file 'seas_18.se1' not found in PATH '/x/'"})
	return st, popup.GroupRows() + 2 // the Ceres row, listed last
}

// rowLine is the rendered chooser line holding the tick box and the name.
func rowLine(t *testing.T, frame, name string) string {
	t.Helper()
	for _, l := range strings.Split(frame, "\n") {
		if strings.Contains(ansi.Strip(l), "] ! "+name) || strings.Contains(ansi.Strip(l), "] "+name) {
			return l
		}
	}
	t.Fatalf("no %s row in:\n%s", name, ansi.Strip(frame))
	return ""
}

func TestChooserAbsentRowCarriesARedMark(t *testing.T) {
	forceColor(t)
	cfg := Config{Color: ColorAlways}
	st, ceres := absentRowFixture()
	for _, c := range []struct {
		name   string
		cursor int
	}{{"away from the cursor", popup.GroupRows()}, {"under the cursor", ceres}} {
		t.Run(c.name, func(t *testing.T) {
			s := st
			s.Cursor = c.cursor
			line := rowLine(t, renderBodyModal(s, cfg, 132, 40), "Ceres")
			i := strings.Index(line, "!")
			if i < 0 {
				t.Fatalf("no ! on the row: %q", line)
			}
			if got := fgBefore(line, i); got != "31" {
				t.Errorf("the ! is drawn in foreground %q, want the error red 31: %q", got, line)
			}
		})
	}
	// The mark belongs to the ! alone: the name after it is not red.
	s := st
	s.Cursor = ceres
	line := rowLine(t, renderBodyModal(s, cfg, 132, 40), "Ceres")
	if fgBefore(line, strings.Index(line, "Ceres")) == "31" {
		t.Errorf("the name is red as well: %q", line)
	}
}

func TestChooserAbsentMarkIsPlainWithColoursOff(t *testing.T) {
	st, ceres := absentRowFixture()
	st.Cursor = ceres
	frame := renderBodyModal(st, plainCfg, 132, 40)
	if line := rowLine(t, frame, "Ceres"); strings.Contains(line, "\x1b[3") {
		t.Errorf("colours off, yet the row carries a colour: %q", line)
	}
	if !strings.Contains(frame, "[ ] ! Ceres") {
		t.Errorf("the plain ! is missing from the row:\n%s", frame)
	}
	if !strings.Contains(frame, "Astrolog: SwissEph file 'seas_18.se1' not found in PATH") {
		t.Errorf("the notice beside the cursor row changed:\n%s", frame)
	}
}

func TestChooserDrawableRowHasNoMark(t *testing.T) {
	forceColor(t)
	st, _ := absentRowFixture()
	st.Cursor = popup.GroupRows()
	line := rowLine(t, renderBodyModal(st, Config{Color: ColorAlways}, 132, 40), "Pallas")
	if strings.Contains(ansi.Strip(line), "!") {
		t.Errorf("a drawable row carries a !: %q", line)
	}
	if strings.Contains(line, "\x1b[31m") {
		t.Errorf("a drawable row carries red: %q", line)
	}
}

func TestChooserAbsentMarkKeepsRowWidthAndTicking(t *testing.T) {
	forceColor(t)
	cfg := Config{Color: ColorAlways}
	st, ceres := absentRowFixture()
	st.Cursor = ceres
	for _, c := range []Config{cfg, plainCfg} {
		line := rowLine(t, renderBodyModal(st, c, 132, 40), "Ceres")
		other := rowLine(t, renderBodyModal(st, c, 132, 40), "Pallas")
		if ansi.StringWidth(line) != ansi.StringWidth(other) {
			t.Errorf("row widths differ: %d vs %d", ansi.StringWidth(line), ansi.StringWidth(other))
		}
	}
	st.ToggleAt(ceres)
	if st.Selected[12] {
		t.Error("an absent row was ticked")
	}
	st.Selected[12] = true
	st.ToggleAt(ceres)
	if st.Selected[12] {
		t.Error("a ticked absent row could not be unticked")
	}
}
