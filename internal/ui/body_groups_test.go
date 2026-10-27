package ui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// body_groups_test.go covers the specification: five group rows
// above the Bodies list that toggle their members, like the aspect families.

// groupsRegistry is every Astrolog object number 0..133, named "Objnnn" except
// the few the tests look for by name.
func groupsRegistry() []aspect.RegistryObject {
	named := map[int]string{0: "Earth", 1: "Sun", 2: "Moon", 10: "Pluto", 11: "Chiron", 12: "Ceres", 16: "North Node", 34: "Vulcan", 43: "Hygiea", 84: "Sirius"}
	reg := make([]aspect.RegistryObject, 0, 134)
	for i := 0; i <= 133; i++ {
		name, ok := named[i]
		if !ok {
			name = fmt.Sprintf("Obj%03d", i)
		}
		reg = append(reg, aspect.RegistryObject{Number: i, Name: name, Star: i >= 84})
	}
	return reg
}

// groupWant is the member count of each group over groupsRegistry, from the
// spec's table.
var groupWant = []struct {
	name string
	n    int
}{{"Planets", 11}, {"Points & angles", 18}, {"Asteroids", 14}, {"Uranians", 9}, {"Other", 82}}

func plainBodyCfg() Config { return Config{Accessible: true, Color: ColorNever} }

// onlyDashes reports a modal line that is just the dashed rule between the
// box's side borders.
func onlyDashes(line string) bool {
	t := strings.Trim(line, "| ")
	return len(t) > 5 && strings.Trim(t, "-") == ""
}

// Acceptance 1, 2, 7 and 8's visible half.
func TestBodyGroupRowsComeFirst(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {132, 40}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			cfg := plainBodyCfg()
			st := openBodyModal(groupsRegistry(), recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1}}, nil)
			lines := strings.Split(renderBodyModal(st, cfg, size[0], size[1]), "\n")

			search := -1
			for i, l := range lines {
				if strings.Contains(l, "Search bodies:") {
					search = i
					break
				}
			}
			if search < 0 {
				t.Fatal("no search line")
			}
			// The box draws its own separator under the search line; the
			// five group rows follow it, then the dashed rule, then the list.
			first := search + 2
			for i, g := range groupWant {
				l := lines[first+i]
				if !strings.Contains(l, g.name) || !strings.Contains(l, fmt.Sprintf("(%d)", g.n)) {
					t.Errorf("group row %d = %q, want %q with (%d)", i, l, g.name, g.n)
				}
			}
			if !strings.Contains(lines[first], "[◐ ] Planets") {
				t.Errorf("Planets holds the ticked Sun and others, so it reads [◐ ]: %q", lines[first])
			}
			if !strings.Contains(lines[first+1], "[○ ] Points & angles") {
				t.Errorf("an untouched group reads [○ ]: %q", lines[first+1])
			}
			if !onlyDashes(lines[first+5]) {
				t.Errorf("the line after the group rows = %q, want the dashed rule", lines[first+5])
			}
			if !strings.Contains(lines[first+6], "Earth") {
				t.Errorf("the list starts under the rule: %q", lines[first+6])
			}

			// Acceptance 2: the cursor walks the group rows first, then the
			// list; `n of N` counts both.
			for i := 0; i < len(groupWant); i++ {
				if !strings.Contains(lines[first+i], "> [") {
					t.Fatalf("cursor %d is not on group row %d:\n%s", i, i, strings.Join(lines, "\n"))
				}
				st, _ = updateBodyModalKey(st, tea.KeyMsg{Type: tea.KeyDown}, bodyListHeight(size[1]))
				lines = strings.Split(renderBodyModal(st, cfg, size[0], size[1]), "\n")
			}
			if !strings.Contains(lines[first+6], "> [ ] Earth") {
				t.Errorf("after five Downs the cursor is on the first list row: %q", lines[first+6])
			}
			if got := strings.Join(lines, "\n"); !strings.Contains(got, "6 of 139") {
				t.Errorf("position should count 5 group rows + 134 objects (6 of 139):\n%s", got)
			}
		})
	}
}

// Acceptance 7: at every cursor position, at 80x24, the five group rows, the
// rule, the cursor row and the Enabled bar are all on screen.
func TestBodyGroupBlockAndCursorStayVisibleAt80x24(t *testing.T) {
	cfg := plainBodyCfg()
	st := openBodyModal(groupsRegistry(), recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1}}, nil)
	h := bodyListHeight(24)
	for pos := 0; pos < st.totalRows(); pos++ {
		out := renderBodyModal(st, cfg, 80, 24)
		lines := strings.Split(out, "\n")
		for _, g := range groupWant {
			if !strings.Contains(out, g.name) {
				t.Fatalf("cursor %d: group row %q is not visible:\n%s", pos, g.name, out)
			}
		}
		rules := 0
		for _, l := range lines {
			if onlyDashes(l) {
				rules++
			}
		}
		if rules < 2 { // the box's own separator and the group rule
			t.Fatalf("cursor %d: the dashed rule is not visible:\n%s", pos, out)
		}
		if !strings.Contains(out, " selected ") {
			t.Fatalf("cursor %d: the tag bar is not visible:\n%s", pos, out)
		}
		marks := 0
		for _, l := range lines {
			if strings.Contains(l, "> [") {
				marks++
			}
		}
		if marks != 1 {
			t.Fatalf("cursor %d: %d cursor rows on screen, want 1:\n%s", pos, marks, out)
		}
		if pos >= groupRows() {
			obj, ok := st.objectUnderCursor()
			if !ok || !strings.Contains(out, "> [") || !strings.Contains(out, obj.Name) {
				t.Fatalf("cursor %d: the row under the cursor (%q) is not drawn:\n%s", pos, obj.Name, out)
			}
		}
		st, _ = updateBodyModalKey(st, tea.KeyMsg{Type: tea.KeyDown}, h)
	}
}

// Acceptance 3, 4, 5 and 6.
func TestBodyGroupsToggleTheirMembers(t *testing.T) {
	oldChooserRule(t)
	planets := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	entry := recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1}}

	open := func(t *testing.T) Model {
		m := readyModelWithObjects(groupsRegistry())
		m.lens.Objects = entry
		tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
		return tm.(Model)
	}

	t.Run("3 ticks all unless all ticked, and the preview follows", func(t *testing.T) {
		seen := recordLensSeams(t)
		m := open(t)
		tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
		got := tm.(Model)
		if on := ticksOn(got.bodyModal.selected); len(on) != len(planets) {
			t.Fatalf("a partial group toggled to %d ticked, want all %d: %v", len(on), len(planets), on)
		}
		if cmd == nil {
			t.Fatal("the group toggle scheduled no preview")
		}
		settle(t, got, cmd)
		if len(*seen) == 0 {
			t.Fatal("the group toggle's preview started no compute")
		}
		for i, lens := range *seen {
			if !reflect.DeepEqual(lens.Objects.Selected(), planets) {
				t.Errorf("preview compute %d was handed %v, want %v", i, lens.Objects.Selected(), planets)
			}
		}
		tm, _ = got.Update(tea.KeyMsg{Type: tea.KeySpace})
		if on := ticksOn(tm.(Model).bodyModal.selected); len(on) != 0 {
			t.Errorf("toggling an all-ticked group left %v ticked, want none", on)
		}
	})

	t.Run("4 count and toggle follow the filter", func(t *testing.T) {
		m := open(t)
		for _, r := range "hyg" {
			tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = tm.(Model)
		}
		rows := strings.Join(bodyGroupRows(m.bodyModal, plainBodyCfg(), 60), "\n")
		if !strings.Contains(rows, "Asteroids") || !strings.Contains(rows, "Asteroids       (1)") {
			t.Errorf("with hyg typed Asteroids should read (1):\n%s", rows)
		}
		for i := 0; i < 2; i++ { // Planets -> Points & angles -> Asteroids
			tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
			m = tm.(Model)
		}
		tm, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
		on := ticksOn(tm.(Model).bodyModal.selected)
		if _, ok := on[43]; !ok || len(on) != 2 { // Sun from the entry set, plus Hygiea
			t.Errorf("the filtered Asteroids toggle should tick Hygiea only (plus the entry Sun): %v", on)
		}
	})

	t.Run("5 a probe-marked body is ticked and unticked by its group", func(t *testing.T) {
		m := open(t)
		m.bodyModal.recordProbe(43, false, []string{"seas_00.se1"})
		if _, marked := m.bodyModal.undrawable(43); !marked {
			t.Fatal("setup: Hygiea is not marked")
		}
		st := m.bodyModal
		st.toggleAt(2) // Asteroids
		if !st.selected[43] {
			t.Error("a marked body was skipped by its group's tick")
		}
		rows := strings.Join(bodyListRows(st, plainBodyCfg(), 60, 200), "\n")
		if !strings.Contains(rows, "! Hygiea") {
			t.Errorf("the mark must survive the toggle:\n%s", rows)
		}
		st.toggleAt(2)
		if st.selected[43] {
			t.Error("a marked body was skipped by its group's untick")
		}
	})

	t.Run("6 Esc restores the opening selection, Enter applies", func(t *testing.T) {
		seen := recordLensSeams(t)
		m := open(t)
		tm, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
		tm, cmd := tm.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
		if got := tm.(Model).lens.Objects.Selected(); !reflect.DeepEqual(got, entry.Selected()) {
			t.Errorf("Esc left %v, want the opening %v", got, entry.Selected())
		}
		if cmd == nil {
			t.Fatal("Esc after a group toggle scheduled no recompute")
		}
		drainCmd(cmd)
		for i, lens := range *seen {
			if !reflect.DeepEqual(lens.Objects.Selected(), entry.Selected()) {
				t.Errorf("compute %d after Esc was handed %v", i, lens.Objects.Selected())
			}
		}

		m = open(t)
		tm, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
		tm, _ = tm.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
		if got := tm.(Model).lens.Objects.Selected(); !reflect.DeepEqual(got, planets) {
			t.Errorf("Enter applied %v, want %v", got, planets)
		}
	})
}

// The newcomer route "remove a group, cancel the preview": unticking a whole
// group and pressing Esc leaves the lens' object policy as it was.
func TestBodyGroupUntickThenEscRestoresEntry(t *testing.T) {
	oldChooserRule(t)
	m := readyModelWithObjects(groupsRegistry())
	planets := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	m.lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: planets}
	entry := m.lens.Objects
	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	tm, _ = tm.(Model).Update(tea.KeyMsg{Type: tea.KeySpace}) // cursor on Planets: unticks all
	if got := tm.(Model).bodyModal.policy().Selected(); len(got) != 0 {
		t.Fatalf("setup: Planets still ticked: %v", got)
	}
	tm, _ = tm.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := tm.(Model).lens.Objects; !reflect.DeepEqual(got, entry) {
		t.Errorf("Esc left %+v, want the entry policy %+v", got, entry)
	}
}

// An object number the table does not cover goes to Other.
func TestObjectGroupOfFallsBackToOther(t *testing.T) {
	last := len(recipe.ObjectGroups) - 1
	for _, n := range []int{134, 500, -1} {
		if g := recipe.ObjectGroupOf(n); g != last {
			t.Errorf("object %d went to group %d, want Other (%d)", n, g, last)
		}
	}
	for n := 0; n <= 133; n++ {
		covered := 0
		for _, g := range recipe.ObjectGroups {
			for _, r := range g.Ranges {
				if n >= r[0] && n <= r[1] {
					covered++
				}
			}
		}
		if covered != 1 {
			t.Errorf("object %d is in %d group ranges, want exactly 1", n, covered)
		}
	}
}

// Frames (the specification): at rest, and filtered to hyg.
func TestBodyGroupsGolden(t *testing.T) {
	forceColor(t)
	cases := []struct {
		name   string
		filter string
	}{{"rest", ""}, {"hyg", "hyg"}}
	for _, tc := range cases {
		for _, size := range [][2]int{{80, 24}, {132, 40}} {
			for _, md := range []struct {
				name string
				cfg  Config
			}{{"color", Config{Color: ColorAlways}}, {"accessible", Config{Accessible: true, Color: ColorNever}}} {
				name := fmt.Sprintf("body-modal-groups-%s-%dx%d-%s", tc.name, size[0], size[1], md.name)
				t.Run(name, func(t *testing.T) {
					st := openBodyModal(groupsRegistry(), recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 43}}, nil)
					if tc.filter != "" {
						st.appendRunes([]rune(tc.filter))
						st.refilter()
					}
					checkGolden(t, name, renderBodyModal(st, md.cfg, size[0], size[1]))
				})
			}
		}
	}
}
