package ui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// body_modal_test.go covers the body selector's own rows of
// the specification. The ones that assert against what the COMPUTE
// receives live in body_recompute_test.go, because asserting that a Model field
// changed is exactly the trap an earlier change paid for.

// testObjectRegistry is a small stand-in shaped like the real read: 0-based,
// carrying names the enum knows, names it does not, and a star row.
func testObjectRegistry() []aspect.RegistryObject {
	return []aspect.RegistryObject{
		{Number: 0, Name: "Earth"},
		{Number: 1, Name: "Sun"},
		{Number: 2, Name: "Moon"},
		{Number: 3, Name: "Mercury"},
		{Number: 4, Name: "Venus"},
		{Number: 10, Name: "Pluto"},
		{Number: 12, Name: "Ceres"},
		{Number: 16, Name: "North Node"},
		{Number: 84, Name: "Sirius", Star: true},
		{Number: 133, Name: "Great Attr.", Star: true},
	}
}

func openBodyTest(policy recipe.ObjectPolicy, lensDefault []int) bodyModalState {
	return openBodyModal(testObjectRegistry(), policy, lensDefault)
}

func readyModelWithObjects(reg []aspect.RegistryObject) Model {
	m := NewModel(Config{Accessible: true, Color: ColorNever})
	m.width, m.height = 132, 40
	m.startup = startupReady
	m.objectRegistry = reg
	return m
}

// --- opening and seeding ---------------------------------------------------

// ⌃b opens the selector, and it is seeded from the policy in force.
func TestBodyKeyOpensSelector(t *testing.T) {
	m := readyModelWithObjects(testObjectRegistry())
	m.lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2}}

	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	got := tm.(Model)
	if got.startup != startupBodyOpen {
		t.Fatalf("startup = %q, want %q", got.startup, startupBodyOpen)
	}
	if cmd != nil {
		t.Error("opening the selector returned a Cmd; opening must start no work")
	}
	if !got.modalOpen() {
		t.Error("modalOpen() is false while the selector is open")
	}
	if !got.bodyModal.selected[1] || !got.bodyModal.selected[2] {
		t.Error("the selector must open seeded from the policy in force")
	}
	if got.bodyModal.selected[10] {
		t.Error("an object outside the in-force selection must open unticked")
	}
}

// With no override in force the seed is the LENS's drawn set, not the whole
// registry. Seeding everything would turn the operator's first keystroke into a
// chart with every object the binary carries — see openBodyModal's comment.
func TestBodySelectorSeedsFromTheLensNotTheRegistry(t *testing.T) {
	st := openBodyTest(recipe.ObjectPolicy{Baseline: recipe.AstrologDefault}, []int{1, 2, 4})
	on := ticksOn(st.selected)
	if len(on) != 3 {
		t.Fatalf("seeded %d objects, want the lens's 3 — a registry-wide seed would silently widen the chart", len(on))
	}
	for _, n := range []int{1, 2, 4} {
		if !st.selected[n] {
			t.Errorf("object %d is drawn by the lens and must open ticked", n)
		}
	}
	if st.selected[133] {
		t.Error("a fixed star the lens does not draw must not open ticked")
	}
}

// C5 — an object the enum cannot name is listed, selectable, and carried into
// the policy by its registry number.
func TestUnnamedObjectsAreStillSelectable(t *testing.T) {
	st := openBodyTest(recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)

	var ceres, star int = -1, -1
	for i, o := range st.matches {
		switch o.Number {
		case 12:
			ceres = i
		case 133:
			star = i
		}
	}
	if ceres < 0 || star < 0 {
		t.Fatal("Ceres and the fixed star must both be listed; a selector bounded by aspect.Body would show neither")
	}
	st.toggleAt(groupRows() + ceres)
	st.toggleAt(groupRows() + star)

	got := st.policy().Selected()
	if !reflect.DeepEqual(got, []int{12, 133}) {
		t.Errorf("policy selection = %v, want [12 133] — by registry number, under the registry's own name", got)
	}
}

// The policy the modal produces is always Explicit, and it carries TrueNodeMode
// through from the entry policy. Dropping it would silently swap the mean node
// for the true one in a slice that is not about nodes.
func TestBodyPolicyKeepsTrueNodeMode(t *testing.T) {
	st := openBodyTest(recipe.ObjectPolicy{Baseline: recipe.AstrologDefault, TrueNodeMode: true}, []int{1, 16})
	p := st.policy()
	if p.Baseline != recipe.Explicit {
		t.Errorf("Baseline = %v, want Explicit", p.Baseline)
	}
	if !p.TrueNodeMode {
		t.Error("TrueNodeMode must survive the modal; it selects the true-node VARIANT of object 16, not which objects are drawn")
	}
}

// C8 — the empty selection is a real chart, not a fall back to a default.
func TestEmptyBodySelectionIsExplicit(t *testing.T) {
	st := openBodyTest(recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1}}, nil)
	st.toggleAt(0) // Earth is row 0 and is off; tick then untick to be sure
	st.selected = map[int]bool{}
	p := st.policy()
	if p.Baseline != recipe.Explicit {
		t.Fatal("an empty selection must stay Explicit — falling back to a delta baseline would silently redraw the lens default")
	}
	if len(p.Selected()) != 0 {
		t.Errorf("Selected() = %v, want empty", p.Selected())
	}
}

// --- filtering and the list ------------------------------------------------

func TestBodyFilterMatchesNameSubstring(t *testing.T) {
	st := openBodyTest(recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)
	st.appendRunes([]rune("nod"))
	st.refilter()
	if len(st.matches) != 1 || st.matches[0].Number != 16 {
		t.Fatalf("filter %q matched %v, want North Node alone", st.filterText, st.matches)
	}
}

// Filtering must never change the selection. A ticked object that the filter
// hides stays ticked, or the operator loses their set by typing.
func TestBodyFilterDoesNotDisturbSelection(t *testing.T) {
	st := openBodyTest(recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 133}}, nil)
	before := ticksOn(st.selected)

	st.appendRunes([]rune("sun"))
	st.refilter()
	if !sameTickSets(before, ticksOn(st.selected)) {
		t.Error("filtering changed the selection")
	}
	if !st.selected[133] {
		t.Error("an object hidden by the filter must stay ticked")
	}
	// ...and it is still in the confirmed policy, which is what actually
	// reaches the chart.
	if got := st.policy().Selected(); !reflect.DeepEqual(got, []int{1, 133}) {
		t.Errorf("policy selection under a filter = %v, want [1 133]", got)
	}
}

// A toggle acts on the row under the cursor in the FILTERED list, not on the
// registry index — the bug a flat list makes easy and a filter makes invisible.
func TestBodyToggleFollowsTheFilteredRow(t *testing.T) {
	st := openBodyTest(recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)
	st.appendRunes([]rune("ceres"))
	st.refilter()
	st.toggleAt(groupRows()) // the first list row, past the group rows
	if !st.selected[12] {
		t.Errorf("toggling the only filtered row must tick Ceres (12); selected = %v", ticksOn(st.selected))
	}
	if len(ticksOn(st.selected)) != 1 {
		t.Errorf("exactly one object should be ticked, got %v", ticksOn(st.selected))
	}
}

// An empty match set must not panic on toggle, and must render its own line.
func TestBodyEmptyFilterIsSafe(t *testing.T) {
	st := openBodyTest(recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)
	st.appendRunes([]rune("zzzz"))
	st.refilter()
	st.toggleAt(st.cursor) // must not panic
	rows := bodyListRows(st, Config{}, 40, 5)
	if len(rows) != 1 || !strings.Contains(rows[0], "no matching") {
		t.Errorf("rows = %v, want a single no-matches line", rows)
	}
}

// --- the tag bar -----------------------------------------------------------

// The tag bar is the reason the selection is visible at all: 134 rows do not
// fit, and a filter hides most of what is ticked. It opens with the count and
// the legend of the group marks, then the chosen names.
func TestBodyTagBarNamesTheSelection(t *testing.T) {
	st := openBodyTest(recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2}}, nil)
	bar := bodyTagBar(st, 80, Config{})
	want := "2 of 10 selected · ● all ◐ some ○ none "
	if !strings.HasPrefix(bar, want) {
		t.Errorf("tag bar = %q, want it to start %q", bar, want)
	}
	if !strings.Contains(bar, "Sun") || !strings.Contains(bar, "Moon") {
		t.Errorf("tag bar = %q, want it to name the ticked objects", bar)
	}
	if strings.Contains(bar, "Enabled") {
		t.Errorf("tag bar = %q, the Enabled: prefix is gone", bar)
	}
	ascii := bodyTagBar(st, 80, Config{Accessible: true})
	if !strings.HasPrefix(ascii, "2 of 10 selected - ● all ◐ some ○ none ") {
		t.Errorf("ASCII tag bar = %q, want - for the middle dot", ascii)
	}
}

// With nothing ticked the bar still counts, and has no names to show.
func TestBodyTagBarSaysWhenEmpty(t *testing.T) {
	st := openBodyTest(recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)
	bar := bodyTagBar(st, 80, Config{})
	if !strings.HasPrefix(bar, "0 of 10 selected · ● all ◐ some ○ none") {
		t.Errorf("tag bar = %q, want it to start `0 of 10 selected`", bar)
	}
	if strings.Contains(bar, "(none)") || strings.Contains(bar, "Enabled") {
		t.Errorf("tag bar = %q, the old `Enabled: (none)` text is gone", bar)
	}
}

// It elides rather than wrapping. A bar that grew a line would change the list
// height under a moving cursor, which is how the window and the cursor drift
// apart. The names follow the legend after one space, cut with the mark, and
// are omitted when the legend leaves no room.
func TestBodyTagBarNeverWraps(t *testing.T) {
	reg := make([]aspect.RegistryObject, 0, 40)
	sel := make([]int, 0, 40)
	for i := 0; i < 40; i++ {
		reg = append(reg, aspect.RegistryObject{Number: i, Name: "Object"})
		sel = append(sel, i)
	}
	st := openBodyModal(reg, recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: sel}, nil)
	for _, w := range []int{54, 60} { // the list width at 80x24 and 132x40
		bar := bodyTagBar(st, w, Config{Accessible: true})
		if strings.Contains(bar, "\n") {
			t.Errorf("width %d: the tag bar must be one line", w)
		}
		if got := ansi.StringWidth(bar); got > w {
			t.Errorf("width %d: tag bar is %d cells wide: %q", w, got, bar)
		}
		if !strings.HasSuffix(bar, "...") {
			t.Errorf("width %d: tag bar = %q, want ... when it runs out of room under --accessible", w, bar)
		}
		// With Unicode the mark is the one every cut uses (the specification C11).
		if bar := bodyTagBar(st, w, Config{}); !strings.HasSuffix(bar, "…") || ansi.StringWidth(bar) > w {
			t.Errorf("width %d: tag bar = %q, want … when it runs out of room", w, bar)
		}
	}
	// No room beside the legend: the names are omitted, never wrapped.
	legend := "40 of 40 selected · ● all ◐ some ○ none"
	bar := bodyTagBar(st, ansi.StringWidth(legend), Config{})
	if bar != legend {
		t.Errorf("tag bar = %q, want the legend alone %q", bar, legend)
	}
}

// Ticking a group row changes the count by the members that were unticked;
// unticking changes it back; the filter changes neither N nor M.
func TestBodyTagBarCountFollowsGroupToggles(t *testing.T) {
	st := openBodyModal(groupsRegistry(), recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1}}, nil)
	count := func(s bodyModalState) string {
		return strings.SplitN(bodyTagBar(s, 200, Config{}), " selected", 2)[0]
	}
	if got := count(st); got != "1 of 134" {
		t.Fatalf("count = %q, want 1 of 134", got)
	}
	st.toggleAt(0) // Planets: 11 members, Sun already ticked
	if got := count(st); got != "11 of 134" {
		t.Errorf("after ticking Planets count = %q, want 11 of 134 (10 newly ticked)", got)
	}
	st.toggleAt(0)
	if got := count(st); got != "0 of 134" {
		t.Errorf("after unticking Planets count = %q, want 0 of 134", got)
	}
	st.toggleAt(0)
	st.appendRunes([]rune("hyg"))
	st.refilter()
	if got := count(st); got != "11 of 134" {
		t.Errorf("a filter changed the count: %q", got)
	}
}

// The legend's marks are the glyphs the group rows draw, from one constant set.
func TestBodyLegendMarksAreTheGroupRowMarks(t *testing.T) {
	legend := bodyTagBar(openBodyTest(recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil), 80, Config{})
	want := groupMarkAll + " all " + groupMarkSome + " some " + groupMarkNone + " none"
	if !strings.Contains(legend, want) {
		t.Errorf("legend = %q, want %q", legend, want)
	}
	for _, c := range []struct {
		sel  []int
		mark string
	}{{nil, groupMarkNone}, {[]int{1}, groupMarkSome}} {
		st := openBodyModal(groupsRegistry(), recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: c.sel}, nil)
		row := bodyGroupRows(st, Config{}, 60)[0]
		if !strings.Contains(row, "["+c.mark+"]") {
			t.Errorf("Planets row = %q, want mark %q", row, c.mark)
		}
	}
	all := make([]int, 0, 11)
	for i := 0; i <= 10; i++ {
		all = append(all, i)
	}
	st := openBodyModal(groupsRegistry(), recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: all}, nil)
	if row := bodyGroupRows(st, Config{}, 60)[0]; !strings.Contains(row, "["+groupMarkAll+"]") {
		t.Errorf("Planets row = %q, want mark %q", row, groupMarkAll)
	}
}

// The Bodies popup says what it searches; every other popup keeps `Search: `.
func TestBodiesSearchLineSaysBodies(t *testing.T) {
	st := openBodyTest(recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)
	st.appendRunes([]rune("su"))
	view := ansi.Strip(renderBodyModal(st, Config{Accessible: true}, 132, 40))
	if !strings.Contains(view, "Search bodies: su█") {
		t.Errorf("the Bodies input line is not `Search bodies: su`:\n%s", view)
	}
	city := ansi.Strip(renderAtlasModal(liftState("ber", 0), Config{Accessible: true}, 132, 40))
	if !strings.Contains(city, "Search: ber█") || strings.Contains(city, "Search bodies") {
		t.Errorf("the City input line changed:\n%s", city)
	}
	plain := ansi.Strip(renderModalBox(modalBox{Title: popupTitleCity, FilterText: "x"}, Config{Accessible: true}, 132, 40,
		func(w, h int) string { return "" }))
	if !strings.Contains(plain, "Search: x█") {
		t.Errorf("the default prompt is not `Search: `:\n%s", plain)
	}
}

// --- C6: the chassis is bound, not reimplemented ---------------------------

// TestBodySelectorReusesChassis is behavioural, per C6: it drives search,
// cursor movement, filtering and selection through the modal and asserts the
// state IS the embedded chassis instance's. Construction alone would not pass —
// an unused chassis sitting beside a parallel list satisfies any AST check.
func TestBodySelectorReusesChassis(t *testing.T) {
	m := readyModelWithObjects(testObjectRegistry())
	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})

	// Type, move, and toggle through the real key path.
	for _, r := range "u" {
		tm, _ = tm.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	// Typing lands on the first match, below the group rows
	// (the specification), so one row into the list
	// is one press.
	for i := 0; i < 1; i++ {
		tm, _ = tm.(Model).Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	got := tm.(Model)

	// The typed text, the cursor and the match set are all read off the
	// EMBEDDED fuzzyList — if the modal kept its own, these would be stale.
	if got.bodyModal.fuzzyList.filterText != "u" {
		t.Errorf("chassis filterText = %q, want %q", got.bodyModal.fuzzyList.filterText, "u")
	}
	if got.bodyModal.fuzzyList.cursor != groupRows()+1 {
		t.Errorf("chassis cursor = %d, want %d", got.bodyModal.fuzzyList.cursor, groupRows()+1)
	}
	if len(got.bodyModal.fuzzyList.matches) == 0 {
		t.Fatal("chassis matches is empty after filtering")
	}
	// And the rendered rows come from that same instance.
	rows := bodyListRows(got.bodyModal, Config{}, 40, 6)
	if len(rows) != len(got.bodyModal.fuzzyList.matches) {
		t.Errorf("rendered %d rows from %d chassis matches; the render must read the chassis", len(rows), len(got.bodyModal.fuzzyList.matches))
	}
	if !strings.HasPrefix(rows[got.bodyModal.fuzzyList.cursor-groupRows()], cursorBandStart(Config{})) {
		t.Error("the cursor mark must sit on the chassis cursor's row")
	}
}

// --- C10: refusals ----------------------------------------------------------

// A failed registry read refuses to open; the frame keeps computing with
// whatever was in force. The read named its cause when it landed; the key
// says nothing (the specification B1).
func TestBodyModalRefusesWithoutRegistry(t *testing.T) {
	m := NewModel(Config{Accessible: true, Color: ColorNever})
	m.width, m.height = 132, 40
	m.startup = startupReady
	m.objectRegErr = errors.New("astrolog: -R1 -HO: no object rows found")

	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	got := tm.(Model)
	if got.startup == startupBodyOpen {
		t.Fatal("the selector opened without a registry; it would offer a set Sextile invented")
	}
	if cmd != nil {
		t.Error("a refusal must start no work")
	}
	if len(got.logLines) != len(m.logLines) {
		t.Errorf("the refused key wrote to the Log: %v", got.logPanelLines())
	}
}

// A read that has not LANDED is not a refusal: the press is taken and the
// selector opens when the registry arrives. A hotkey that depends on how long
// the operator waited is indistinguishable from a broken one.
func TestBodyModalTakesThePressBeforeTheRegistryLands(t *testing.T) {
	m := NewModel(Config{Accessible: true, Color: ColorNever})
	m.width, m.height = 132, 40
	m.startup = startupReady

	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	got := tm.(Model)
	if !got.bodyOpenPending {
		t.Fatal("the press must be recorded as pending")
	}
	// ...and starts no second read: Init already fired one, so a duplicate
	// spends a second Astrolog process for the same answer and races the
	// first — a duplicate that failed first would clear the pending flag and
	// leave the original's success with nothing to open (C5a).
	if cmd != nil {
		t.Error("an early press started a SECOND registry read; the startup one is already in flight")
	}
	if _, again := got.Update(tea.KeyMsg{Type: tea.KeyCtrlB}); again != nil {
		t.Error("a second early press started a read")
	}

	got, _ = applyObjectRegistryLoaded(got, ObjectRegistryLoadedMsg{Registry: testObjectRegistry()})
	if got.startup != startupBodyOpen {
		t.Errorf("startup = %q, want the selector to open on arrival", got.startup)
	}
	if got.bodyOpenPending {
		t.Error("the pending flag must clear")
	}
}

// The selector refuses before the first frame commits: `startup` names both the
// startup phase and the open modal, so one opened early is silently clobbered.
func TestBodyModalRefusesBeforeFirstFrame(t *testing.T) {
	m := readyModelWithObjects(testObjectRegistry())
	m.startup = startupComputing

	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	got := tm.(Model)
	if got.startup == startupBodyOpen {
		t.Fatal("the selector opened mid-startup and would be overwritten by the frame commit")
	}
	if len(got.logLines) != len(m.logLines) {
		t.Errorf("an early press wrote to the Log: %v", got.logPanelLines())
	}
}

// C11 — a toggle arriving while the modal is closed returns the model unchanged
// with a nil command.
func TestBodyTogglesIgnoredWhenClosed(t *testing.T) {
	m := readyModelWithObjects(testObjectRegistry())
	before := m.lens.Objects

	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	got := tm.(Model)
	if !sameObjectSelection(got.lens.Objects, before) {
		t.Error("Space outside the selector changed the object policy")
	}
}

func logMentions(m Model, want string) bool {
	for _, l := range m.logLines {
		if strings.Contains(l.text, want) {
			return true
		}
	}
	return false
}
