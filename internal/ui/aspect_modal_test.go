package ui

import (
	"errors"
	"sort"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// an earlier change — the aspect chooser. Contract rows C5, C6, C7 and C9
// (the specification); C1 lives in internal/invariants.

// testRegistry is a fixture standing in for `astrolog -HA`. It is deliberately
// NOT the live binary's: a fixture is how the group predicates can be asserted
// against a known membership, and how a renamed or zero-orb aspect can be
// exercised at all.
//
// InForce mirrors the shape a default host really has, and the mirroring is the
// point: the Ptolemaic five are unrestricted and the other seven are NOT, while
// every one of the seven still carries a wide positive orb. A fixture with
// everything in force would let a chooser that reads the orb instead of the
// restriction pass every test in this file.
func testRegistry() []aspect.RegistryAspect {
	return []aspect.RegistryAspect{
		{Index: 1, Name: "Conjunct", Abbrev: "Con", Angle: 0, Orb: 7, InForce: true},
		{Index: 2, Name: "Opposite", Abbrev: "Opp", Angle: 180, Orb: 7, InForce: true},
		{Index: 3, Name: "Square", Abbrev: "Squ", Angle: 90, Orb: 7, InForce: true},
		{Index: 4, Name: "Trine", Abbrev: "Tri", Angle: 120, Orb: 7, InForce: true},
		{Index: 5, Name: "Sextile", Abbrev: "Sex", Angle: 60, Orb: 6, InForce: true},
		{Index: 6, Name: "Inconjunct", Abbrev: "Inc", Angle: 150, Orb: 3},
		{Index: 7, Name: "SemiSextile", Abbrev: "SSx", Angle: 30, Orb: 3},
		{Index: 8, Name: "SemiSquare", Abbrev: "SSq", Angle: 45, Orb: 3},
		{Index: 9, Name: "SesquiQuadrate", Abbrev: "Ses", Angle: 135, Orb: 3},
		{Index: 10, Name: "Quintile", Abbrev: "Qui", Angle: 72, Orb: 2},
		{Index: 11, Name: "BiQuintile", Abbrev: "BQn", Angle: 144, Orb: 2},
		{Index: 13, Name: "Septile", Abbrev: "Sep", Angle: 51.43, Orb: 1},
	}
}

// ticked is the sorted set of indices the modal reports as chosen.
func ticked(st aspectModalState) []int {
	var out []int
	for n, on := range st.selected {
		if on {
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func openTest(policy recipe.AspectPolicy) aspectModalState {
	return openAspectModal(testRegistry(), policy)
}

// C5 — families are defined by the angle's harmonic, not by names or indices.
func TestGroupsAreDefinedByAngle(t *testing.T) {
	st := openTest(recipe.AspectPolicy{})

	want := map[aspect.Family][]int{
		aspect.FamilyPtolemaic: {1, 2, 3, 4, 5},
		aspect.FamilyMajor:     {1, 2, 3, 4, 5, 6, 7, 8, 9},
		aspect.FamilyHardMinor: {6, 7, 8, 9},
		aspect.FamilyHarmonic:  {10, 11, 13},
		aspect.FamilyAll:       {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 13},
	}
	for f, exp := range want {
		got := st.familyMembers(f)
		if len(got) != len(exp) {
			t.Errorf("%s members = %v, want %v", f.Name(), got, exp)
			continue
		}
		for i := range got {
			if got[i] != exp[i] {
				t.Errorf("%s members = %v, want %v", f.Name(), got, exp)
				break
			}
		}
	}

	// Renaming every aspect must change no membership: the predicate reads
	// the angle, and a user's binary can rename aspects with -YAD.
	renamed := testRegistry()
	for i := range renamed {
		renamed[i].Name = "Renamed"
		renamed[i].Abbrev = "Rnm"
	}
	rst := openAspectModal(renamed, recipe.AspectPolicy{})
	for _, f := range aspect.Families() {
		a, b := st.familyMembers(f), rst.familyMembers(f)
		if len(a) != len(b) {
			t.Errorf("%s membership changed when every aspect was renamed", f.Name())
		}
	}
}

// C5 — a family whose predicate matches nothing still renders its row.
func TestGroupRowSurvivesEmptyMembership(t *testing.T) {
	onlyConjunction := []aspect.RegistryAspect{{Index: 1, Name: "Conjunct", Abbrev: "Con", Angle: 0, Orb: 7}}
	st := openAspectModal(onlyConjunction, recipe.AspectPolicy{})
	if got := len(st.familyMembers(aspect.FamilyHarmonic)); got != 0 {
		t.Fatalf("Harmonic has %d members in a conjunction-only registry", got)
	}
	rows := aspectFamilyRows(st, Config{}, 60)
	if len(rows) != familyRows() {
		t.Fatalf("family block rendered %d rows, want %d", len(rows), familyRows())
	}
	if !strings.Contains(rows[3], "Harmonic") || !strings.Contains(rows[3], "(0)") {
		t.Errorf("empty family row = %q, want it present with a zero count", rows[3])
	}
}

// C6 — a family row reads all / partial / none from live membership, and
// toggling it selects all when not all are selected and deselects when all are.
func TestGroupTriStateAndToggle(t *testing.T) {
	st := openTest(recipe.AspectPolicy{Explicit: true, Indices: []int{1, 2, 3, 4, 5}})

	if got := st.familyState(aspect.FamilyPtolemaic); got != familyAll {
		t.Errorf("Ptolemaic = %v, want all-selected", got)
	}
	if got := st.familyState(aspect.FamilyMajor); got != familyPartial {
		t.Errorf("Major = %v, want partial (it is a superset of Ptolemaic)", got)
	}
	if got := st.familyState(aspect.FamilyHarmonic); got != familyNone {
		t.Errorf("Harmonic = %v, want none-selected", got)
	}

	// Not all selected -> select all.
	st.toggleFamily(aspect.FamilyMajor)
	if got := st.familyState(aspect.FamilyMajor); got != familyAll {
		t.Errorf("after toggling a partial family it is %v, want all", got)
	}
	// All selected -> deselect all.
	st.toggleFamily(aspect.FamilyMajor)
	if got := st.familyState(aspect.FamilyMajor); got != familyNone {
		t.Errorf("after toggling a full family it is %v, want none", got)
	}
	// Overlap is not a defect: Ptolemaic went with Major because it is a
	// subset, and that is what the tri-state marker exists to show.
	if got := st.familyState(aspect.FamilyPtolemaic); got != familyNone {
		t.Errorf("Ptolemaic = %v after Major was cleared, want none", got)
	}
}

// C7 — an empty selection compiles to "no aspects", never to a fallback.
func TestEmptySelectionComputesNoAspects(t *testing.T) {
	st := openTest(recipe.AspectPolicy{Explicit: true, Indices: []int{1, 2}})
	// All is PARTIAL here (two of twelve), so the first toggle selects every
	// member and only the second clears them. That is C6's rule and not an
	// accident: a partial group fills before it empties.
	st.toggleFamily(aspect.FamilyAll)
	if got := st.familyState(aspect.FamilyAll); got != familyAll {
		t.Fatalf("toggling a partial All gave %v, want all-selected", got)
	}
	st.toggleFamily(aspect.FamilyAll)
	p := st.policy()
	if !p.Explicit {
		t.Fatal("an emptied selection is not Explicit; it would silently fall back to the host default")
	}
	if len(p.Selected()) != 0 {
		t.Errorf("selected = %v, want none", p.Selected())
	}
}

// C9 — opening, filtering, moving and toggling start no Astrolog process.
// Shaped after TestScrollKeysStartNoCompute: the assertion is that no work is
// even SCHEDULED, which is stronger than counting processes.
func TestModalTogglesStartNoCompute(t *testing.T) {
	st := openTest(recipe.AspectPolicy{})
	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyDown}, {Type: tea.KeyDown}, {Type: tea.KeySpace},
		{Type: tea.KeyRunes, Runes: []rune("squ")}, {Type: tea.KeySpace},
		{Type: tea.KeyBackspace}, {Type: tea.KeyUp},
	} {
		var out aspectModalOutcome
		st, out = updateAspectModalKey(st, msg, testListHeight)
		if out.Confirmed || out.Cancelled {
			t.Fatalf("%v ended the modal", msg.Type)
		}
	}
	// Only Enter hands a policy back.
	_, out := updateAspectModalKey(st, tea.KeyMsg{Type: tea.KeyEnter}, testListHeight)
	if !out.Confirmed {
		t.Error("Enter did not confirm")
	}
}

// C3 (the cancel half) — Esc restores the policy that was in force at open.
func TestConfirmAppliesCancelRestores(t *testing.T) {
	oldChooserRule(t)
	entry := recipe.AspectPolicy{Explicit: true, Indices: []int{1, 2, 3, 4, 5}}
	st := openTest(entry)

	st.toggleFamily(aspect.FamilyHarmonic)
	st.toggleAt(0)

	_, cancelled := updateAspectModalKey(st, tea.KeyMsg{Type: tea.KeyEsc}, testListHeight)
	if !cancelled.Cancelled {
		t.Fatal("Esc did not cancel")
	}
	if got, want := cancelled.Policy.Selected(), entry.Selected(); len(got) != len(want) {
		t.Fatalf("cancel restored %v, want the entry policy %v", got, want)
	}

	_, confirmed := updateAspectModalKey(st, tea.KeyMsg{Type: tea.KeyEnter}, testListHeight)
	if !confirmed.Confirmed {
		t.Fatal("Enter did not confirm")
	}
	if len(confirmed.Policy.Selected()) == len(entry.Selected()) {
		t.Error("confirm handed back the entry policy; the edits were lost")
	}
}

// C11 — an aspect the enum cannot name is listed, selectable, and reaches the
// policy under its registry index.
func TestUnknownAspectsAreStillSelectable(t *testing.T) {
	st := openTest(recipe.AspectPolicy{Explicit: true})
	if _, named := aspect.KindForAngle(51.43); named {
		t.Fatal("the fixture's septile is supposed to be unnameable by the enum")
	}
	found := false
	for i, r := range st.matches {
		if r.Index != 13 {
			continue
		}
		found = true
		st.toggleAt(familyRows() + i)
	}
	if !found {
		t.Fatal("the septile is not in the list at all")
	}
	for _, n := range st.policy().Selected() {
		if n == 13 {
			return
		}
	}
	t.Error("the septile was selectable but never reached the policy")
}

// C14 — an aspect whose effective orb is zero is marked differently, at every
// width, including the narrow one where the orb column itself is dropped.
func TestZeroOrbAspectsAreDistinguished(t *testing.T) {
	reg := testRegistry()
	reg[4].Orb = 0 // the host's astrolog.as disabled the sextile
	st := openAspectModal(reg, recipe.AspectPolicy{})

	if st.selected[5] {
		t.Error("a zero-orb aspect was seeded as selected; it cannot produce a row")
	}
	for _, width := range []int{54, 60} {
		row := listRow(aspectRow(st, reg[4], false, width), Config{}, width)
		if !strings.Contains(row, "[-]") {
			t.Errorf("width %d: zero-orb row = %q, want the unavailable marker", width, row)
		}
		live := listRow(aspectRow(st, reg[3], false, width), Config{}, width)
		if strings.Contains(live, "[-]") {
			t.Errorf("width %d: a counting aspect was marked unavailable: %q", width, live)
		}
	}

	// The second half of C14, and the half a marker cannot deliver: selecting
	// it must not REPORT SUCCESS. A `-` row that still ticks is worse than an
	// unmarked one — the chooser then names a set Astrolog will silently omit
	// from, and every other row of this contract passes while it does.

	// Individually, from the list.
	for i, r := range st.matches {
		if r.Index == 5 {
			st.toggleAt(familyRows() + i)
		}
	}
	if st.selected[5] {
		t.Error("Space on a zero-orb row ticked it")
	}

	// Through the family row that contains it. Ptolemaic is Con/Opp/Squ/Tri
	// plus this sextile, so "select all" is the path that used to sweep it in
	// without the operator ever putting a cursor on it.
	st.toggleFamily(aspect.FamilyPtolemaic) // full -> empty
	if got := st.familyState(aspect.FamilyPtolemaic); got != familyNone {
		t.Fatalf("Ptolemaic = %v after clearing it, want none", got)
	}
	st.toggleFamily(aspect.FamilyPtolemaic) // empty -> full
	if st.selected[5] {
		t.Error("selecting the family swept the zero-orb aspect in")
	}
	if got := st.familyState(aspect.FamilyPtolemaic); got != familyAll {
		t.Errorf("Ptolemaic = %v after select-all, want all — a family holding one member that cannot be ticked would fill forever and never empty", got)
	}

	// And it reaches no policy by any route, including an entry policy that
	// still names it from a session where its orb was not yet zero.
	for _, n := range st.policy().Selected() {
		if n == 5 {
			t.Error("a zero-orb aspect reached the policy; the chart will omit it and the chooser says it is on")
		}
	}
	stale := openAspectModal(reg, recipe.AspectPolicy{Explicit: true, Indices: []int{1, 5}})
	for _, n := range stale.policy().Selected() {
		if n == 5 {
			t.Error("a stale explicit policy carried the zero-orb aspect back into a confirmed set")
		}
	}
}

// C3a — the chooser opens on the set the HOST actually counts.
//
// The defect this pins is not cosmetic. Opening seeds the ticks; the first
// confirm turns whatever was seeded into an explicit policy; so a seed that
// overstates the host's set does not merely mislabel the screen, it widens the
// chart. Reading each row's orb rather than its restriction overstates it by
// nineteen aspects on a default binary.
func TestOpenSeedsTheHostsInForceSet(t *testing.T) {
	seen := recordLensSeams(t)

	m := readyModelWithRegistry(testRegistry())
	m.lens.Aspects = recipe.AspectPolicy{} // no override: the startup state

	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	got := tm.(Model)
	if got.startup != startupAspectOpen {
		t.Fatal("the chooser did not open")
	}
	want := []int{1, 2, 3, 4, 5}
	if g := ticked(got.aspectModal); !sameInts(g, want) {
		t.Errorf("opened with %v ticked, want the host's in-force set %v", g, want)
	}
	// Said the other way round, because this is the exact wrong reading: the
	// inconjunct has a 3-degree orb and is restricted out. An orb is not a
	// restriction.
	if got.aspectModal.selected[6] {
		t.Error("a restricted aspect was ticked because its orb is positive; the seed is reading the wrong fact")
	}

	// Confirming what was seeded, unedited, must leave the chart counting the
	// same aspects it was already counting.
	tm2, cmd := got.Update(tea.KeyMsg{Type: tea.KeyEnter})
	after := tm2.(Model)
	if !after.lens.Aspects.Explicit {
		t.Fatal("confirm left the policy non-explicit")
	}
	if g := after.lens.Aspects.Selected(); !sameInts(g, want) {
		t.Errorf("confirming an untouched chooser put %v in force, want the host's own %v", g, want)
	}
	drainCmd(cmd)
	if len(*seen) == 0 {
		t.Fatal("confirming started no compute")
	}
	for i, lens := range *seen {
		if g := lens.Aspects.Selected(); !sameInts(g, want) {
			t.Errorf("compute %d was handed %v, want %v", i, g, want)
		}
	}
}

// The family row's count and its glyph must describe the SAME set. They came
// from two different derivations — the count over filtered membership, the
// glyph over the whole registry — so a filter leaving one selected member
// visible drew `◐ (1)`: partly selected, of one, of which one is selected.
func TestFilteredFamilyRowAgreesWithItsGlyph(t *testing.T) {
	// Square (3) selected, SemiSquare (8) and SesquiQuadrate (9) not. All
	// three match "squ"; nothing else in the fixture does.
	st := openTest(recipe.AspectPolicy{Explicit: true, Indices: []int{3}})
	st.filterText = "squ"
	st.refilter()

	if got := len(st.matches); got != 3 {
		t.Fatalf("filter matched %d rows, want Square, SemiSquare and SesquiQuadrate", got)
	}

	// Ptolemaic's only visible member is Square, and Square is selected, so
	// the row is ALL of what it can currently speak for.
	if got := st.familyState(aspect.FamilyPtolemaic); got != familyAll {
		t.Errorf("Ptolemaic = %v with its one visible member selected, want all", got)
	}
	rows := aspectFamilyRows(st, Config{}, 60)
	if !strings.Contains(rows[0], "[●]") || !strings.Contains(rows[0], "(1)") {
		t.Errorf("Ptolemaic row = %q, want a full glyph and a count of 1 — both over the same set", rows[0])
	}

	// And the toggle acts on what the row speaks for. Clearing it must not
	// reach Conjunct, Opposite or Trine, which the operator cannot see.
	st.toggleFamily(aspect.FamilyPtolemaic)
	if st.selected[3] {
		t.Error("clearing the family left its one visible member selected")
	}

	st.filterText = ""
	st.refilter()
	st.selected = map[int]bool{}
	st.filterText = "squ"
	st.refilter()
	st.toggleFamily(aspect.FamilyPtolemaic)
	for _, n := range []int{1, 2, 4, 5} {
		if st.selected[n] {
			t.Errorf("filling a filtered family reached index %d, which is not on screen", n)
		}
	}
	if !st.selected[3] {
		t.Error("filling a filtered family did not reach its one visible member")
	}
}

// renderPinnedBody's first real caller: the family block must actually reach
// the rendered modal, above the list.
func TestAspectModalRendersFamilyBlockAboveList(t *testing.T) {
	st := openTest(recipe.AspectPolicy{Explicit: true, Indices: []int{1, 2, 3, 4, 5}})
	out := renderAspectModal(st, Config{Accessible: true, Color: ColorNever}, 80, 24)

	for _, f := range aspect.Families() {
		if !strings.Contains(out, f.Name()) {
			t.Errorf("rendered modal is missing the %q family row", f.Name())
		}
	}
	iPtol := strings.Index(out, "Ptolemaic")
	iList := strings.Index(out, "Conjunct")
	if iPtol < 0 || iList < 0 || iPtol > iList {
		t.Errorf("family block is not above the list (Ptolemaic at %d, first aspect at %d)", iPtol, iList)
	}
	// Assert on the abbreviation of a row that is actually visible. At 80x24
	// the box gives the list seven rows and the family block takes five of
	// them, so only the first two aspects render -- see the note in the
	// spec's taste lane about what the pinned block costs at this width.
	if !strings.Contains(out, "Con") {
		t.Error("rendered modal is missing an abbreviation, the bridge to the aspect waterfall")
	}
}

// --- wiring: the chooser reached from the running model -------------------

func readyModelWithRegistry(reg []aspect.RegistryAspect) Model {
	m := NewModel(Config{Accessible: true, Color: ColorNever})
	m.width, m.height = 132, 40
	m.startup = startupReady
	m.aspectRegistry = reg
	return m
}

// `a` opens the chooser, and the modal it opens is seeded from the policy in
// force.
func TestAspectKeyOpensChooser(t *testing.T) {
	m := readyModelWithRegistry(testRegistry())
	m.lens.Aspects = recipe.AspectPolicy{Explicit: true, Indices: []int{1, 2}}

	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	got := tm.(Model)
	if got.startup != startupAspectOpen {
		t.Fatalf("startup = %q, want %q", got.startup, startupAspectOpen)
	}
	if cmd != nil {
		t.Error("opening the chooser returned a Cmd; opening must start no work")
	}
	if !got.modalOpen() {
		t.Error("modalOpen() is false while the chooser is open")
	}
	if !got.aspectModal.selected[1] || !got.aspectModal.selected[2] || got.aspectModal.selected[3] {
		t.Errorf("modal seeded %v, want exactly the policy in force", got.aspectModal.selected)
	}
	if !strings.Contains(got.View(), "Ptolemaic") {
		t.Error("the open chooser is not on screen")
	}

	// C12: modals remain usable over the friendly too-small fallback. Refusing
	// only this chooser would make its hotkey behave unlike every other modal.
	small := readyModelWithRegistry(testRegistry())
	small.width, small.height = 70, 14
	if view := small.View(); !strings.Contains(view, "Terminal too small") {
		t.Fatalf("70x14 baseline is not tooSmallView:\n%s", view)
	}
	smallModel, smallCmd := small.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	opened := smallModel.(Model)
	if smallCmd != nil || opened.startup != startupAspectOpen {
		t.Fatalf("small-terminal open = startup %q, cmd %v; want the chooser open with no work", opened.startup, smallCmd != nil)
	}
	if view := opened.View(); !strings.Contains(view, "Search:") || !strings.Contains(view, "Ptolemaic") {
		t.Errorf("chooser did not render over tooSmallView:\n%s", view)
	}
}

// C10 — with no registry the chooser refuses to open, and the frame keeps
// computing with whatever was already in force. The failed read is logged
// once, when it lands; the key says nothing (the specification B1).
func TestModalRefusesWithoutRegistry(t *testing.T) {
	m := readyModelWithRegistry(nil)
	m.aspectRegErr = errors.New("astrolog: -HA: no aspect rows found")
	before := m.lens.Aspects

	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	got := tm.(Model)
	if got.startup == startupAspectOpen {
		t.Fatal("the chooser opened with no registry; it would offer a set the binary does not have")
	}
	if cmd != nil {
		t.Error("the refusal returned a Cmd")
	}
	if got.lens.Aspects.Explicit != before.Explicit {
		t.Error("the refusal changed the policy in force")
	}
	if len(got.logLines) != len(m.logLines) {
		t.Errorf("the refused key wrote to the Log: %v", got.logPanelLines())
	}

	// A registry that has not arrived YET is NOT a refusal. Treating it as
	// one made the hotkey depend on how long the operator had waited, which
	// is indistinguishable from a broken key. The press is TAKEN and the
	// modal opens when the startup read lands.
	//
	// And it starts no second read. Init fires one unconditionally, so a
	// nil registry with no error can only mean that one is in flight; a
	// duplicate would spend another Astrolog process for the same answer
	// and, if it failed first, would clear the pending flag and leave the
	// original's success with nothing to open.
	young := readyModelWithRegistry(nil)
	tm2, cmd2 := young.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	got2 := tm2.(Model)
	if cmd2 != nil {
		t.Error("an early press started a SECOND registry read; the startup one is already in flight")
	}
	if !got2.aspectOpenPending {
		t.Fatal("the press was not remembered, so the arriving registry will not open anything")
	}
	if _, cmd3 := got2.Update(tea.KeyMsg{Type: tea.KeyCtrlA}); cmd3 != nil {
		t.Error("a second early press started a read")
	}
	if line := strings.Join(got2.logPanelLines(), "\n"); strings.Contains(line, "unavailable") {
		t.Errorf("a not-yet-read registry reported as unavailable: %q", line)
	}

	// ...and when it lands, the chooser opens without a second press.
	tm3, _ := got2.Update(AspectRegistryLoadedMsg{Registry: testRegistry()})
	if tm3.(Model).startup != startupAspectOpen {
		t.Error("the registry landed on a pending request and the chooser did not open")
	}
}

// Confirm puts the chosen set in force; cancel leaves it exactly as it was.
func TestChooserConfirmAndCancelReachTheModel(t *testing.T) {
	oldChooserRule(t)
	entry := recipe.AspectPolicy{Explicit: true, Indices: []int{1, 2, 3, 4, 5}}

	// Cancel.
	m := readyModelWithRegistry(testRegistry())
	m.lens.Aspects = entry
	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	tm, _ = tm.(Model).Update(tea.KeyMsg{Type: tea.KeySpace}) // toggle Ptolemaic off
	tm, _ = tm.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := tm.(Model)
	if got.startup == startupAspectOpen {
		t.Error("Esc left the chooser open")
	}
	if len(got.lens.Aspects.Selected()) != len(entry.Selected()) {
		t.Errorf("cancel left %v in force, want the entry policy %v", got.lens.Aspects.Selected(), entry.Selected())
	}

	// Confirm.
	m2 := readyModelWithRegistry(testRegistry())
	m2.lens.Aspects = entry
	tm2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	tm2, _ = tm2.(Model).Update(tea.KeyMsg{Type: tea.KeySpace}) // Ptolemaic off
	tm2, _ = tm2.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	got2 := tm2.(Model)
	if got2.startup == startupAspectOpen {
		t.Error("Enter left the chooser open")
	}
	if len(got2.lens.Aspects.Selected()) == len(entry.Selected()) {
		t.Error("confirm did not put the edited set in force")
	}
	if !got2.lens.Aspects.Explicit {
		t.Error("confirm left the policy non-explicit; the chart would silently use the host default")
	}
}

// The registry read lands on the Model, and a failed read is kept so the
// refusal can name it.
func TestAspectRegistryLoadedReachesTheModel(t *testing.T) {
	m := NewModel(Config{Accessible: true})
	tm, _ := m.Update(AspectRegistryLoadedMsg{Registry: testRegistry()})
	if got := len(tm.(Model).aspectRegistry); got != len(testRegistry()) {
		t.Errorf("registry = %d rows, want %d", got, len(testRegistry()))
	}

	m2 := NewModel(Config{Accessible: true})
	tm2, _ := m2.Update(AspectRegistryLoadedMsg{Err: errors.New("boom")})
	if tm2.(Model).aspectRegErr == nil {
		t.Error("a failed read was discarded; the refusal could not name its cause")
	}
}

// Every typed character goes into the filter buffer while the chooser is open
// — including `q`. Losing the operator's half-made selection to the quit
// binding would be the worst possible way to learn this was unhandled.
func TestChooserSwallowsMainScopeKeys(t *testing.T) {
	m := readyModelWithRegistry(testRegistry())
	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	if tm.(Model).startup != startupAspectOpen {
		t.Fatal("chooser did not open")
	}
	tm, cmd := tm.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	got := tm.(Model)
	if cmd != nil {
		t.Error("`q` inside the chooser produced a Cmd — the quit binding fired")
	}
	if got.startup != startupAspectOpen {
		t.Errorf("`q` inside the chooser left it: startup = %q", got.startup)
	}
	if got.aspectModal.filterText != "q" {
		t.Errorf("filterText = %q, want %q — typed characters must filter, not act", got.aspectModal.filterText, "q")
	}
}

// The chooser cannot open before the first frame: `startup` names both the
// startup phase and the open modal, so a modal opened during bootstrap is
// overwritten the moment the frame commits. Pressing early must say so, not
// arm something that will be silently clobbered.
func TestChooserDoesNotOpenBeforeFirstFrame(t *testing.T) {
	m := NewModel(Config{Accessible: true, Color: ColorNever})
	m.width, m.height = 132, 40
	m.aspectRegistry = testRegistry()
	if m.startup == startupReady {
		t.Fatal("fixture is already ready; this test needs a bootstrapping model")
	}

	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	got := tm.(Model)
	if got.startup == startupAspectOpen {
		t.Error("the chooser opened during startup; the frame commit would overwrite it")
	}
	if got.aspectOpenPending {
		t.Error("an early press armed a deferred open that startup would clobber")
	}
	if cmd != nil {
		t.Error("an early press started work")
	}
	if len(got.logLines) != len(m.logLines) {
		t.Errorf("an early press wrote to the Log: %v", got.logPanelLines())
	}
}
