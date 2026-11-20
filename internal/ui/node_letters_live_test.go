package ui

// The letters of Bodies, the Details title and the comparison's cast, against
// the real binary: the form each names is the one the call that cast the row
// carried, read off its argv.

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// liveComparison is two natal charts and Now, all selected, the cursor on the
// second and the first its base, cast live. Each chart's own form is its own
// setting for the run; the call's form is the base's.
func liveComparison(t *testing.T, baseForm, otherForm recipe.NodeForm) (Model, func() [][]string, int) {
	t.Helper()
	read := argvShim(t)
	m, _ := liveScreens(t)
	if err := addAnchor(m.slots, time.Date(1971, 8, 20, 4, 40, 0, 0, time.UTC), testPlaceAmsterdam()); err != nil {
		t.Fatal(err)
	}
	m = m.withNodeSetting(0, baseForm).withNodeSetting(1, otherForm)
	for i := 0; i < 3; i++ {
		m.slots.SetSlotSelected(i, true)
	}
	m.cursorIdx = 1
	m.base = slotAt(m, 0)
	m.height = 60
	m.width = max(m.width, dualColumnMinWidth(m.shownCount()))
	m.focused = PanelBodies
	mark := len(read())
	m, cmd := m.noteWant(surfaceText, true)
	m = pumpPatient(t, m, cmd)
	return m, read, mark
}

// withNodeSetting is the chart's own form for the run, a chart with no file.
func (m Model) withNodeSetting(i int, f recipe.NodeForm) Model {
	vol := make(map[string]recipe.NodeForm, len(m.frames.volatileN)+1)
	for k, v := range m.frames.volatileN {
		vol[k] = v
	}
	vol[volatileKey(slotAt(m, i))] = f
	m.frames.volatileN = vol
	return m
}

var nodeLetterRow = regexp.MustCompile(`(N\.Node|S\.Node|Lilith) ([mtn])\s`)

// nodeLetters is the letter of each of the three rows on screen.
func nodeLetters(view string) map[string]string {
	out := map[string]string{}
	for _, l := range strings.Split(ansi.Strip(view), "\n") {
		if m := nodeLetterRow.FindStringSubmatch(l); m != nil {
			out[m[1]] = m[2]
		}
	}
	return out
}

// castingArgvs are the spawns that cast bodies (they name objects with -R): the
// date probe, which asks Astrolog how it reads a date and draws no body, is
// left out.
func castingArgvs(all [][]string) [][]string {
	var out [][]string
	for _, argv := range all {
		if slices.Contains(argv, "-R") {
			out = append(out, argv)
		}
	}
	return out
}

// lastNodeSwitches is the form the last Yn and Ynn switches of an argv cast.
func lastNodeSwitches(argv []string) (yn, ynn string) {
	for _, a := range argv {
		switch a {
		case "=Yn", "_Yn":
			yn = a
		case "=Ynn", "_Ynn":
			ynn = a
		}
	}
	return yn, ynn
}

// N14: the letter after N.Node, S.Node and Lilith comes from the two switches
// of the call that cast the row, never from the chart's setting.
func TestBodiesNamesTheNodeFormFromTheCall(t *testing.T) {
	for _, tc := range []struct {
		name         string
		base, other  recipe.NodeForm
		node, lilith string
		yn, ynn      string
	}{
		{"base mean, the chart under the cursor true", recipe.NodesMean, recipe.NodesTrue, "m", "m", "_Yn", "_Ynn"},
		// Control: only the call's switches change (the base's), the chart's own
		// setting stays true, and the letters follow the call.
		{"base true and natural, the chart under the cursor still true", recipe.NodesTrueNatural, recipe.NodesTrue, "t", "n", "=Yn", "=Ynn"},
		{"base mean and natural", recipe.NodesMeanNatural, recipe.NodesTrue, "m", "n", "_Yn", "=Ynn"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, read, mark := liveComparison(t, tc.base, tc.other)
			if got, _ := m.ownNodes(slotAt(m, 1)); got != tc.other {
				t.Fatalf("setup: the chart under the cursor is set to %v", got)
			}
			spawns := castingArgvs(read()[mark:])
			if len(spawns) == 0 {
				t.Fatal("the cast spawned nothing")
			}
			for _, argv := range spawns {
				if yn, ynn := lastNodeSwitches(argv); yn != tc.yn || ynn != tc.ynn {
					t.Errorf("a spawn carried %s %s, want the base's %s %s: %v", yn, ynn, tc.yn, tc.ynn, argv)
				}
			}
			got := nodeLetters(m.View())
			want := map[string]string{"N.Node": tc.node, "S.Node": tc.node, "Lilith": tc.lilith}
			for name, letter := range want {
				if got[name] != letter {
					t.Errorf("%s carries %q, want %q (letters on screen: %v)\n%s", name, got[name], letter, got, ansi.Strip(m.View()))
				}
			}
		})
	}
	t.Run("alone, the chart's own form", func(t *testing.T) {
		m, read, mark := liveComparison(t, recipe.NodesMean, recipe.NodesTrueNatural)
		m.slots.SetSlotSelected(0, false)
		m.slots.SetSlotSelected(2, false)
		m.base = nil
		mark = len(read())
		m, cmd := m.noteWant(surfaceText, true)
		m = pumpPatient(t, m, cmd)
		for _, argv := range castingArgvs(read()[mark:]) {
			if yn, ynn := lastNodeSwitches(argv); yn != "=Yn" || ynn != "=Ynn" {
				t.Errorf("a spawn carried %s %s, want =Yn =Ynn: %v", yn, ynn, argv)
			}
		}
		if got := nodeLetters(m.View()); got["N.Node"] != "t" || got["Lilith"] != "n" {
			t.Errorf("letters = %v, want t and n\n%s", got, ansi.Strip(m.View()))
		}
	})
}
