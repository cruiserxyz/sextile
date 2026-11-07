package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// chooser_groups_test.go: operator, 2026-11-04 (interview). The body chooser has
// seven groups, each with a ticked/total count, Ctrl+D restores the lens's own
// default bodies, the rows with no ephemeris file come last, and a group's name
// typed into the search lists its members.

func seq(from, to int) []int {
	var out []int
	for n := from; n <= to; n++ {
		out = append(out, n)
	}
	return out
}

func numbersOf(rows []aspect.RegistryObject) []int {
	out := make([]int, 0, len(rows))
	for _, o := range rows {
		out = append(out, o.Number)
	}
	return out
}

// groupRow is the pinned row of the group with this name, as the popup draws it.
func groupRow(t *testing.T, st popup.Body, name string) string {
	t.Helper()
	for _, l := range popup.PinnedGroupRows(st, plainBodyCfg(), 60) {
		if strings.Contains(l, name) {
			return l
		}
	}
	t.Fatalf("no group row for %q", name)
	return ""
}

// listIndex is the row of object n in the chooser's list.
func listIndex(t *testing.T, st popup.Body, n int) int {
	t.Helper()
	i := slices.IndexFunc(st.Matches, func(o aspect.RegistryObject) bool { return o.Number == n })
	if i < 0 {
		t.Fatalf("object %d is not listed", n)
	}
	return i
}

func TestSevenGroups(t *testing.T) {
	want := []struct {
		name    string
		members []int
	}{
		{"Planets", seq(0, 10)},
		{"Angles", []int{22, 25, 28, 31}},
		{"Points", seq(16, 21)},
		{"House cusps", []int{23, 24, 26, 27, 29, 30, 32, 33}},
		{"Asteroids", append(seq(11, 15), seq(43, 51)...)},
		{"Uranians", seq(34, 42)},
		{"Other", seq(52, 133)},
	}
	if got := len(recipe.ObjectGroups); got != len(want) {
		t.Fatalf("%d groups, want %d", got, len(want))
	}
	for i, w := range want {
		if got := recipe.ObjectGroups[i].Name; got != w.name {
			t.Errorf("group %d is %q, want %q", i, got, w.name)
		}
		var members []int
		for n := 0; n <= 133; n++ {
			if recipe.ObjectGroupOf(n) == i {
				members = append(members, n)
			}
		}
		if !slices.Equal(members, w.members) {
			t.Errorf("%s holds %v, want %v", w.name, members, w.members)
		}
	}

	// The pinned rows follow that order.
	st := popup.Open(groupsRegistry(), recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)
	if popup.GroupRows() != len(want) {
		t.Fatalf("%d pinned group rows, want %d", popup.GroupRows(), len(want))
	}
	for i, l := range popup.PinnedGroupRows(st, plainBodyCfg(), 60) {
		if !strings.Contains(l, want[i].name) {
			t.Errorf("pinned row %d = %q, want %q", i, l, want[i].name)
		}
	}

	// Earth is in Planets' list, ticked only by its own row: the group's Space,
	// its mark and its two numbers leave it out.
	if _, ok := st.ObjectUnderCursor(); ok {
		t.Fatal("fixture: the cursor starts on a list row")
	}
	if i := listIndex(t, st, 0); st.Matches[i].Name != "Earth" {
		t.Fatalf("fixture: object 0 is %q", st.Matches[i].Name)
	}
	st.ToggleAt(0) // Planets
	if on := tickedNums(st); !slices.Equal(on, seq(1, 10)) {
		t.Errorf("Space on Planets ticked %v, want Sun to Pluto and not Earth", on)
	}
	if l := groupRow(t, st, "Planets"); !strings.Contains(l, "[● ]") || !strings.Contains(l, "(10/10)") {
		t.Errorf("Planets with ten ticked reads %q, want [● ] and (10/10)", l)
	}
	st.ToggleAt(popup.GroupRows() + listIndex(t, st, 0)) // Earth's own row
	if !st.Selected[0] {
		t.Error("Earth's own row did not tick it")
	}
	if l := groupRow(t, st, "Planets"); !strings.Contains(l, "[● ]") || !strings.Contains(l, "(10/10)") {
		t.Errorf("Earth ticked changed Planets to %q, want [● ] and (10/10) as before", l)
	}
	st.ToggleAt(0) // all ticked: unticks the ten
	if on := tickedNums(st); !slices.Equal(on, []int{0}) {
		t.Errorf("Space on an all-ticked Planets left %v, want Earth alone (hers is her own row)", on)
	}
	if l := groupRow(t, st, "Planets"); !strings.Contains(l, "[○ ]") || !strings.Contains(l, "(0/10)") {
		t.Errorf("Planets with only Earth ticked reads %q, want [○ ] and (0/10)", l)
	}
}

func TestGroupCounts(t *testing.T) {
	st := popup.Open(groupsRegistry(), recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)
	for _, n := range []int{11, 12, 43} {
		st.ToggleAt(popup.GroupRows() + listIndex(t, st, n))
	}
	if l := groupRow(t, st, "Asteroids"); !strings.Contains(l, "(3/14)") || !strings.Contains(l, "[◐ ]") {
		t.Errorf("Asteroids with three ticked reads %q, want [◐ ] and (3/14)", l)
	}
	if l := groupRow(t, st, "Planets"); !strings.Contains(l, "(0/10)") {
		t.Errorf("Planets reads %q, want (0/10): Earth is not one of its members", l)
	}
	if l := groupRow(t, st, "Angles"); !strings.Contains(l, "(0/4)") {
		t.Errorf("Angles reads %q, want (0/4)", l)
	}
	// The numbers follow the filter, as the mark and the toggle do.
	st.AppendRunes([]rune("hyg"))
	st.Refilter()
	if l := groupRow(t, st, "Asteroids"); !strings.Contains(l, "[● ]") || !strings.Contains(l, "(1/1)") {
		t.Errorf("with hyg typed Asteroids reads %q, want [● ] and (1/1): Hygiea is ticked", l)
	}
	if l := groupRow(t, st, "Other"); !strings.Contains(l, "(0/0)") {
		t.Errorf("with hyg typed Other reads %q, want (0/0)", l)
	}
}

// lensDrawn is the set the lens draws when the operator has chosen nothing, on
// this host: Astrolog's own default objects plus the lens's additions.
func lensDrawn(t *testing.T) []int {
	t.Helper()
	want, err := lookup.Hellenistic.VisibleObjectNumbers()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{11, 17, 18, 25, 28} { // the lens's five, not Astrolog's own default
		if !slices.Contains(want, n) {
			t.Fatalf("fixture: the lens's drawn set %v lacks its own addition %d", want, n)
		}
	}
	return want
}

// tickedNums is the chooser's ticked objects, ascending.
func tickedNums(st popup.Body) []int {
	var out []int
	for n, on := range st.Selected {
		if on {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}

func openedChooser(t *testing.T, entry recipe.ObjectPolicy) Model {
	t.Helper()
	m := readyModelWithObjects(t, groupsRegistry())
	m.lens.Objects = entry
	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	got := tm.(Model)
	if got.startup != startupBodyOpen {
		t.Fatalf("the chooser did not open (startup %q)", got.startup)
	}
	return got
}

func keyIn(t *testing.T, m Model, k tea.KeyMsg) Model {
	t.Helper()
	tm, _ := m.Update(k)
	return tm.(Model)
}

func TestCtrlDRestoresLensDefaults(t *testing.T) {
	oldChooserRule(t)
	want := lensDrawn(t)
	entry := recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 43, 60}, TrueNodeMode: true}
	m := openedChooser(t, entry)

	m.bodyModal.Cursor = popup.GroupRows() + listIndex(t, m.bodyModal, 1)
	m = keyIn(t, m, tea.KeyMsg{Type: tea.KeySpace}) // untick the Sun
	m.bodyModal.Cursor = popup.GroupRows() + listIndex(t, m.bodyModal, 10)
	m = keyIn(t, m, tea.KeyMsg{Type: tea.KeySpace}) // tick Pluto
	if got := tickedNums(m.bodyModal); slices.Equal(got, want) {
		t.Fatalf("fixture: the ticks %v already are the defaults", got)
	}

	m = keyIn(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
	if got := tickedNums(m.bodyModal); !slices.Equal(got, want) {
		t.Errorf("after Ctrl+D the ticks are %v, want the lens's drawn set %v", got, want)
	}
	// Live, like any tick: the chart behind is asked for the same set (as the
	// default policy, which TestEqualTicksAreDefaults holds).
	if got := visibleObjects(t, m.lens); !slices.Equal(got, want) {
		t.Errorf("after Ctrl+D the lens draws %v, want %v", got, want)
	}

	// Esc undoes it all under the closing rule that puts the entry back.
	m = keyIn(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if !reflect.DeepEqual(m.lens.Objects, entry) {
		t.Errorf("Esc left %+v, want the entry policy %+v", m.lens.Objects, entry)
	}
}

// Four ways to arrive at "the default bodies" give one set, the lens's.
func TestDefaultsOneSource(t *testing.T) {
	want := lensDrawn(t)
	explicit := recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2}, TrueNodeMode: true}
	check := func(t *testing.T, m Model) {
		t.Helper()
		if got := visibleObjects(t, m.lens); !slices.Equal(got, want) {
			t.Errorf("the lens draws %v, want its default set %v", got, want)
		}
	}

	t.Run("nothing ticked", func(t *testing.T) {
		m := openedChooser(t, recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1}, TrueNodeMode: true})
		m.bodyModal.Cursor = popup.GroupRows() + listIndex(t, m.bodyModal, 1)
		m = keyIn(t, m, tea.KeyMsg{Type: tea.KeySpace}) // untick the last body
		if left := tickedNums(m.bodyModal); len(left) != 0 {
			t.Fatalf("fixture: still ticked %v", left)
		}
		check(t, m)
		if n := countLines(m, "no bodies chosen, the default bodies are shown"); n != 1 {
			t.Errorf("the Log carries the empty-choice line %d times, want once:\n%s", n, said(m))
		}
	})
	t.Run("new session", func(t *testing.T) {
		m := namedResetModel(t)
		m.lens.Objects = explicit
		check(t, resetKey(t, m))
	})
	t.Run("named session", func(t *testing.T) {
		m := listModel(t, 1, 0)
		m.objectRegistry = drawnRegistry()
		m.lens.Objects = explicit
		p := withSession(perimeter(), store.Session{Version: 3})
		m, _ = applyNamedSessionLoaded(m, NamedSessionLoadedMsg{Gen: m.sessionGen, Name: "plain", Perimeter: p})
		check(t, m)
	})
	t.Run("ctrl+d", func(t *testing.T) {
		m := openedChooser(t, explicit)
		check(t, keyIn(t, m, tea.KeyMsg{Type: tea.KeyCtrlD}))
	})
	t.Run("one spelling", func(t *testing.T) {
		// The default policy is spelled once. A bare policy built anywhere else
		// is the Astrolog default without the lens, which is the divergence this
		// test exists to stop.
		files, err := filepath.Glob("*.go")
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for i, l := range strings.Split(string(b), "\n") {
				if strings.Contains(l, "recipe.ObjectPolicy{TrueNodeMode") {
					t.Errorf("%s:%d builds its own default policy: %s", f, i+1, strings.TrimSpace(l))
				}
			}
		}
	})
}

func TestUndrawableRowsLast(t *testing.T) {
	missing := map[int]string{11: "seas_18.se1", 12: "seas_18.se1", 60: "sefstars.txt"}
	st := popup.Open(groupsRegistry(), recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)
	st.SetAbsent(missing, nil)

	// Without a filter: every drawable object in registry order, then every
	// object with no file in registry order, across the groups.
	got := numbersOf(st.Matches)
	if len(got) != 134 {
		t.Fatalf("%d rows listed, want 134", len(got))
	}
	var drawable, gone []int
	for n := 0; n <= 133; n++ {
		if _, bad := missing[n]; bad {
			gone = append(gone, n)
		} else {
			drawable = append(drawable, n)
		}
	}
	if want := append(append([]int(nil), drawable...), gone...); !slices.Equal(got, want) {
		t.Errorf("the rows read %v, want the drawable ones then %v", got, gone)
	}
	last := got[len(got)-len(gone):]
	if !slices.Equal(last, gone) {
		t.Errorf("the last rows are %v, want every row with no file: %v", last, gone)
	}

	// With a filter: the drawable matches in their rank order, then the others
	// in theirs, whatever tier they matched at.
	reg := []aspect.RegistryObject{
		{Object: aspect.Object{Number: 11, Name: "Ab1"}},
		{Object: aspect.Object{Number: 12, Name: "Ab2"}},
		{Object: aspect.Object{Number: 13, Name: "Ab3"}},
		{Object: aspect.Object{Number: 14, Name: "Zab"}},
		{Object: aspect.Object{Number: 15, Name: "Zab2"}},
	}
	f := popup.Open(reg, recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)
	f.SetAbsent(map[int]string{11: "x", 14: "y"}, nil)
	f.AppendRunes([]rune("ab"))
	f.Refilter()
	if got, want := numbersOf(f.Matches), []int{12, 13, 15, 11, 14}; !slices.Equal(got, want) {
		t.Errorf("filtered, the rows read %v, want %v (every row with no file after every drawable match)", got, want)
	}

	// The chooser takes the startup answer when it opens.
	m := readyModelWithObjects(t, groupsRegistry())
	m.ephemeris.Absent = map[int]string{12: "seas_18.se1"}
	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	opened := numbersOf(tm.(Model).bodyModal.Matches)
	if opened[len(opened)-1] != 12 {
		t.Errorf("Ceres has no file and is listed at %d of %d, not last", slices.Index(opened, 12), len(opened))
	}
}

func TestFilterByGroupName(t *testing.T) {
	st := popup.Open(groupsRegistry(), recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)
	typeInto := func(st popup.Body, q string) popup.Body {
		st.AppendRunes([]rune(q))
		st.Refilter()
		return st
	}

	angles := typeInto(st, "angles")
	if got := numbersOf(angles.Matches); !slices.Equal(got, []int{22, 25, 28, 31}) {
		t.Errorf("angles lists %v, want the four angles", got)
	}
	if l := groupRow(t, angles, "Angles"); !strings.Contains(l, "(0/4)") {
		t.Errorf("with angles typed the group reads %q, want (0/4)", l)
	}

	planets := typeInto(st, "planets")
	if got := numbersOf(planets.Matches); !slices.Equal(got, seq(0, 10)) {
		t.Errorf("planets lists %v, want Earth to Pluto", got)
	}
	if l := groupRow(t, planets, "Planets"); !strings.Contains(l, "(0/10)") {
		t.Errorf("with planets typed the group reads %q, want (0/10)", l)
	}

	if sun := typeInto(st, "sun"); len(sun.Matches) == 0 || sun.Matches[0].Name != "Sun" {
		t.Errorf("sun lists %v, want Sun first", numbersOf(sun.Matches))
	}

	// A body's own name outranks its group's name, whatever the tiers say: here
	// "ast" is a prefix of the group Asteroids and only inside Pasta's name.
	reg := []aspect.RegistryObject{
		{Object: aspect.Object{Number: 1, Name: "Sun"}},
		{Object: aspect.Object{Number: 3, Name: "Pasta"}},
		{Object: aspect.Object{Number: 11, Name: "Chiron"}},
		{Object: aspect.Object{Number: 12, Name: "Ceres"}},
		{Object: aspect.Object{Number: 16, Name: "North Node"}},
	}
	own := popup.Open(reg, recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)
	own = typeInto(own, "ast")
	if got := numbersOf(own.Matches); !slices.Equal(got, []int{3, 11, 12}) {
		t.Errorf("ast lists %v, want Pasta by its own name first, then the asteroids by their group", got)
	}

	// A group's name finds its members by the exact or prefix match only: `an`
	// is a prefix of Angles and merely inside Planets and Uranians, so those two
	// list nothing by their group (Vulcan is listed by its own name).
	an := typeInto(st, "an")
	if got := numbersOf(an.Matches); !slices.Equal(got, []int{34, 22, 25, 28, 31}) {
		t.Errorf("an lists %v, want Vulcan by its own name, then the four angles by their group", got)
	}
	// `e` is a prefix of no group: every row it lists has an e in its own name.
	e := typeInto(st, "e")
	if len(e.Matches) == 0 {
		t.Fatal("e lists nothing, even by an own name")
	}
	for _, o := range e.Matches {
		if !strings.Contains(strings.ToLower(o.Name), "e") {
			t.Errorf("e lists %s (%d) by its group %q, not by its own name", o.Name, o.Number, recipe.ObjectGroups[recipe.ObjectGroupOf(o.Number)].Name)
			break
		}
	}
}

// Ctrl+D is in the chooser's own key bar and in Help, as Defaults.
func TestCtrlDIsListed(t *testing.T) {
	m := helpFixtureOneChart(t, 144, 40)
	var listed []string
	for _, l := range m.helpLines() {
		if l.Binding && strings.Contains(l.Text, "Defaults") {
			listed = append(listed, l.Text)
		}
	}
	if len(listed) != 1 || !strings.Contains(listed[0], "⌃d") && !strings.Contains(listed[0], "ctrl+d") {
		t.Errorf("Help lists %q for Defaults, want one row with its key", listed)
	}
}

// Confirming ticks equal to the lens's drawn set puts the default policy in
// force, not a copy of it: the run goes on following the defaults and a quit
// writes no drawn: line. Until the confirm the preview shows the same chart
// from an explicit set (that keeps Esc's restore of the entry frame honest).
func TestEqualTicksAreDefaults(t *testing.T) {
	check := func(t *testing.T, m Model, where string) {
		t.Helper()
		if !reflect.DeepEqual(m.lens.Objects, m.defaultObjects()) {
			t.Errorf("%s: the lens holds %+v, want the default policy %+v", where, m.lens.Objects, m.defaultObjects())
		}
	}

	t.Run("ctrl+d then confirm", func(t *testing.T) {
		sessionSandbox(t)
		m, _, _ := ticksStart(t)
		cm := chooserOf(t, m)
		cm = tickObject(t, cm, 19) // Fortune off: a chosen set
		if cm.lens.Objects.Baseline != recipe.Explicit {
			t.Fatalf("fixture: unticking one body left %+v", cm.lens.Objects)
		}
		cm = pressInChooser(t, cm, tea.KeyMsg{Type: tea.KeyCtrlD})
		if got, want := visibleObjects(t, cm.lens), cm.defaultDrawn(); !slices.Equal(got, want) {
			t.Errorf("after Ctrl+D the preview draws %v, want the default set %v", got, want)
		}
		cm = pressInChooser(t, cm, tea.KeyMsg{Type: tea.KeyEnter})
		check(t, cm, "after confirming")
		noDrawnLine(t, cm)
	})

	t.Run("ticking back by hand", func(t *testing.T) {
		sessionSandbox(t)
		m, _, _ := ticksStart(t)
		cm := chooserOf(t, m)
		cm = tickObject(t, cm, 19)
		cm = tickObject(t, cm, 19) // and on again
		cm = pressInChooser(t, cm, tea.KeyMsg{Type: tea.KeyEnter})
		check(t, cm, "after confirming")
		noDrawnLine(t, cm)
	})

	t.Run("a different set stays chosen", func(t *testing.T) {
		sessionSandbox(t)
		m, _, _ := ticksStart(t)
		cm := chooserOf(t, m)
		cm = tickObject(t, cm, 19)
		cm = pressInChooser(t, cm, tea.KeyMsg{Type: tea.KeyEnter})
		if cm.lens.Objects.Baseline != recipe.Explicit {
			t.Errorf("a set short of one default body is %+v, want a chosen set", cm.lens.Objects)
		}
	})
}
