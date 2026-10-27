package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// ruleLineAt returns the index of the frame line carrying the "All" family row
// and the lines around it, in plain text.
func aspectFrameLines(frame string) []string { return strings.Split(plain(frame), "\n") }

func allRowIndex(t *testing.T, lines []string) int {
	t.Helper()
	for i, l := range lines {
		if strings.Contains(l, "] All") {
			return i
		}
	}
	t.Fatalf("no `All` family row in frame:\n%s", strings.Join(lines, "\n"))
	return -1
}

// ruleRun is the glyph run a line carries between the box edges, or "" when the
// line is not a rule made of that glyph alone.
func ruleRun(line, glyph string) string {
	s := strings.Trim(line, "|│ ")
	if s != "" && strings.Trim(s, glyph) == "" {
		return s
	}
	return ""
}

var aspectRuleModes = []struct {
	name  string
	cfg   Config
	glyph string
}{
	{"color", Config{Color: ColorAlways}, "┈"},
	{"accessible", Config{Accessible: true, Color: ColorNever}, "-"},
}

// the specification 1, 3, 5: one rule row between the last family
// row and the first aspect row, pinned with the families, and still there when
// the filter leaves no aspects.
func TestAspectPopupRuleSitsBetweenGroupsAndList(t *testing.T) {
	for _, mode := range aspectRuleModes {
		for _, size := range []struct{ w, h int }{{80, 24}, {132, 40}} {
			t.Run(fmt.Sprintf("%s-%dx%d", mode.name, size.w, size.h), func(t *testing.T) {
				st := openAspectModal(bigRegistry(40), recipe.AspectPolicy{})
				lines := aspectFrameLines(renderAspectModal(st, mode.cfg, size.w, size.h))
				all := allRowIndex(t, lines)
				// The search separator above the families is a '-' rule in
				// accessible mode; only the line right under `All` counts.
				run := ruleRun(lines[all+1], mode.glyph)
				// The rule spans the box's content, as wide as the search
				// separator above the families.
				if boxContent := ansi.StringWidth(lines[0]) - 6; len([]rune(run)) != boxContent {
					t.Fatalf("line under the `All` row is not a %q rule:\n%s", mode.glyph, strings.Join(lines, "\n"))
				}
				if !strings.Contains(lines[all+2], "Aspect01") {
					t.Errorf("first aspect row is not directly under the rule: %q", lines[all+2])
				}

				// Scrolling the list never moves the rule.
				listH := aspectListHeight(size.h)
				for i := 0; i < 25; i++ {
					st, _ = updateAspectModalKey(st, tea.KeyMsg{Type: tea.KeyDown}, listH)
				}
				moved := aspectFrameLines(renderAspectModal(st, mode.cfg, size.w, size.h))
				if allRowIndex(t, moved) != all || ruleRun(moved[all+1], mode.glyph) != run {
					t.Errorf("scrolling moved the rule:\n%s", strings.Join(moved, "\n"))
				}
				if strings.Contains(moved[all+2], "Aspect01") {
					t.Errorf("list did not scroll under the rule: %q", moved[all+2])
				}

				// A filter that leaves no aspects keeps the rule above the empty list.
				st.filterText = "zzz"
				st.refilter()
				st.cursor = clampCursor(st.cursor, st.totalRows())
				empty := aspectFrameLines(renderAspectModal(st, mode.cfg, size.w, size.h))
				e := allRowIndex(t, empty)
				if ruleRun(empty[e+1], mode.glyph) == "" || !strings.Contains(empty[e+2], "(no matching aspects)") {
					t.Errorf("empty list lost its rule:\n%s", strings.Join(empty, "\n"))
				}
			})
		}
	}
}

// the specification 2, 4: the rule is not a row (cursor and `n of N`
// are unchanged) and costs the list exactly one visible row, with the cursor and
// search line still on screen at 80x24.
func TestAspectPopupRuleIsNotARow(t *testing.T) {
	cfg := Config{Accessible: true, Color: ColorNever}
	for _, size := range []struct{ w, h int }{{80, 24}, {132, 40}} {
		t.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(t *testing.T) {
			st := openAspectModal(bigRegistry(40), recipe.AspectPolicy{})
			if got, want := st.totalRows(), familyRows()+40; got != want {
				t.Fatalf("totalRows = %d, want %d: the rule must not be a row", got, want)
			}
			if got, want := aspectListHeight(size.h), modalListHeight(size.h)-familyRows()-1; got != want {
				t.Fatalf("aspectListHeight = %d, want %d", got, want)
			}
			lines := aspectFrameLines(renderAspectModal(st, cfg, size.w, size.h))
			visible := 0
			for _, l := range lines {
				if strings.Contains(l, "orb  1.0") {
					visible++
				}
			}
			if want := aspectListHeight(size.h); visible != want {
				t.Errorf("%d aspect rows visible, want %d (list loses one row to the rule)", visible, want)
			}

			listH := aspectListHeight(size.h)
			total := st.totalRows()
			for step := 1; step < total; step++ {
				st, _ = updateAspectModalKey(st, tea.KeyMsg{Type: tea.KeyDown}, listH)
				if st.cursor != step {
					t.Fatalf("down %d put the cursor on %d: it skipped or stopped at the rule", step, st.cursor)
				}
				frame := renderAspectModal(st, cfg, size.w, size.h)
				if got := countCursors(frame); got != 1 {
					t.Fatalf("cursor %d: %d cursor markers on screen, want 1", st.cursor, got)
				}
				if !strings.Contains(plain(frame), "Search:") {
					t.Fatalf("search line missing at cursor %d", st.cursor)
				}
				want := fmt.Sprintf("%d of %d", st.cursor+1, total)
				if !strings.Contains(plain(frame), want) {
					t.Fatalf("cursor %d: frame lacks %q", st.cursor, want)
				}
			}
		})
	}
}
