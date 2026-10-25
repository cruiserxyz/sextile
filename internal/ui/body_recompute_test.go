package ui

import (
	"reflect"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// body_recompute_test.go asserts against what the COMPUTE receives, never
// against a Model field. That distinction is the trap an earlier change paid for: the three
// text-surface computes took the package constant `lookup.Hellenistic`, so a
// confirmed policy could sit correctly in the model and change nothing on
// screen. recordLensSeams substitutes the lookup seams and records the lens
// each compute is actually handed.

// C3 (the confirm half) — the confirmed set reaches the next compute.
func TestBodyConfirmReachesTheCompute(t *testing.T) {
	seen := recordLensSeams(t)

	m := readyModelWithObjects(testObjectRegistry())
	m.lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 3}}

	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	// Row 0 of the list is Earth (0) and opens unticked; tick it, so the
	// confirmed set is strictly different from the entry set. The cursor starts
	// on the group rows, so walk past them first.
	for i := 0; i < groupRows(); i++ {
		tm, _ = tm.(Model).Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	tm, _ = tm.(Model).Update(tea.KeyMsg{Type: tea.KeySpace})
	tm, cmd := tm.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})

	got := tm.(Model)
	if got.startup == startupBodyOpen {
		t.Fatal("Enter left the selector open")
	}
	if cmd == nil {
		t.Fatal("confirming scheduled no recompute — the chart would keep showing the old set")
	}
	drainCmd(cmd)

	if len(*seen) == 0 {
		t.Fatal("no compute ran; the confirmed set never reached Astrolog")
	}
	for i, lens := range *seen {
		if lens.Objects.Baseline != recipe.Explicit {
			t.Errorf("compute %d got baseline %v, want Explicit", i, lens.Objects.Baseline)
		}
		if want := []int{0, 1, 2, 3}; !reflect.DeepEqual(lens.Objects.Selected(), want) {
			t.Errorf("compute %d got selection %v, want %v", i, lens.Objects.Selected(), want)
		}
	}
}

// C3 (the cancel half) — Esc restores the policy in force at open, and
// RECOMPUTES it. Restoring the model alone would leave the previewed set on
// screen, which makes the cancel a lie.
func TestBodyCancelRestoresAndRecomputes(t *testing.T) {
	oldChooserRule(t)
	seen := recordLensSeams(t)

	entry := recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 3}}
	m := readyModelWithObjects(testObjectRegistry())
	m.lens.Objects = entry

	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	tm, _ = tm.(Model).Update(tea.KeyMsg{Type: tea.KeySpace}) // preview a different set
	tm, cmd := tm.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})

	got := tm.(Model)
	if got.startup == startupBodyOpen {
		t.Fatal("Esc left the selector open")
	}
	if !reflect.DeepEqual(got.lens.Objects.Selected(), entry.Selected()) {
		t.Errorf("policy after cancel = %v, want the entry policy %v", got.lens.Objects.Selected(), entry.Selected())
	}
	if cmd == nil {
		t.Fatal("cancelling scheduled no recompute — the previewed set would stay on screen")
	}
	drainCmd(cmd)

	if len(*seen) == 0 {
		t.Fatal("cancel ran no compute")
	}
	for i, lens := range *seen {
		if !reflect.DeepEqual(lens.Objects.Selected(), entry.Selected()) {
			t.Errorf("compute %d after cancel got %v, want the entry set %v", i, lens.Objects.Selected(), entry.Selected())
		}
	}
}

// A cancel that changed nothing must not recompute at all — opening the
// selector and pressing Esc is a no-op, not a free Astrolog spawn.
func TestBodyCancelWithoutEditsRecomputesNothing(t *testing.T) {
	m := readyModelWithObjects(testObjectRegistry())
	m.lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2}}

	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	tm, cmd := tm.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Error("cancelling an unedited selector scheduled work")
	}
	if tm.(Model).startup == startupBodyOpen {
		t.Error("Esc left the selector open")
	}
}

// C7 — opening, filtering, moving and toggling start no Astrolog process. As
// with the aspect chooser's C9 this is asserted on the pure state machine,
// where the assertion is that no work is even SCHEDULED; the debounced preview
// that a toggle DOES request lives one layer up and has its own test below.
func TestBodyTogglesStartNoCompute(t *testing.T) {
	st := openBodyTest(recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)
	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyDown}, {Type: tea.KeyDown}, {Type: tea.KeySpace},
		{Type: tea.KeyRunes, Runes: []rune("sun")}, {Type: tea.KeySpace},
		{Type: tea.KeyBackspace}, {Type: tea.KeyUp},
	} {
		var out bodyModalOutcome
		st, out = updateBodyModalKey(st, msg, 10)
		if out.Confirmed || out.Cancelled {
			t.Fatalf("%v ended the modal", msg.Type)
		}
	}
	if _, out := updateBodyModalKey(st, tea.KeyMsg{Type: tea.KeyEnter}, 10); !out.Confirmed {
		t.Error("Enter did not confirm")
	}
}

// A toggle previews: the panels behind the modal repaint so the operator judges
// the set against the chart rather than against their memory of it. Plain
// typing does NOT — the trigger is a changed selection, not a difference from
// the policy in force, which is the distinction that keeps a filter keystroke
// from spawning Astrolog.
func TestBodyPreviewFiresOnToggleNotOnTyping(t *testing.T) {
	// the specification (Amends, body-selector.md C7): a move
	// or a keystroke in the filter returns a Cmd that, pumped, makes no call
	// before the cursor rests and exactly one probe call after it — and never
	// a chart recompute.
	seen := recordLensSeams(t)
	rec := stubProbe(t, nil)

	m := readyModelWithSlots(t, testPlaceBerlin())
	m.width, m.height = 132, 40
	m.objectRegistry = testObjectRegistry()
	m.lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1}}

	tm, open := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	// The open armed the rest for the first row; let it settle.
	pump(t, tm.(Model), open)
	// The cursor starts on the group rows, where no body is under it: walk
	// onto the first list row, and let that rest settle too.
	for i := 0; i < groupRows(); i++ {
		var step tea.Cmd
		tm, step = tm.(Model).Update(tea.KeyMsg{Type: tea.KeyDown})
		if i == groupRows()-1 {
			tm = pump(t, tm.(Model), step)
		}
	}
	base := rec.count()

	for _, step := range []struct {
		what string
		key  tea.KeyMsg
	}{
		{"typing into the filter", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")}},
		{"moving the cursor", tea.KeyMsg{Type: tea.KeyDown}},
	} {
		// A tea.Tick delivers once, so the same key is pressed twice from the
		// same state: the first timer is abandoned at 50 ms (it has not fired,
		// so no call yet), the second is waited out.
		pre := tm.(Model)
		_, early := pre.Update(step.key)
		if early == nil {
			t.Fatalf("%s scheduled nothing; the body under the cursor changed and must be probed once it rests", step.what)
		}
		pumpWithin(t, pre, 50*time.Millisecond, early)
		if n := rec.count() - base; n != 0 {
			t.Fatalf("%s: %d probe call(s) before the cursor rested", step.what, n)
		}
		var cmd tea.Cmd
		tm, cmd = pre.Update(step.key)
		tm = pump(t, tm.(Model), cmd)
		if n := rec.count() - base; n != 1 {
			t.Fatalf("%s: %d probe call(s) after the rest, want exactly 1", step.what, n)
		}
		base = rec.count()
		if len(*seen) != 0 {
			t.Fatalf("%s started a chart recompute: %v", step.what, *seen)
		}
	}
	_, toggled := tm.(Model).Update(tea.KeyMsg{Type: tea.KeySpace})
	if toggled == nil {
		t.Error("toggling scheduled nothing; the preview would never repaint")
	}
}

// The preview must carry the set being built, not the set in force — and it
// must carry it to the COMPUTE.
//
// The first version of this test asserted `m.lens.Objects` and stopped there.
// That is the shape the aspect chooser's C3 already paid for once: a model
// field holding the right value proves nothing about what the chart was drawn
// from, and this test would have passed against a scheduled compute that
// ignored the edited lens entirely. It now settles the debounce and reads the
// lens each lookup seam is handed.
func TestBodyPreviewCarriesTheEditedSet(t *testing.T) {
	seen := recordLensSeams(t)

	m := readyModelWithObjects(testObjectRegistry())
	m.lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1}}

	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	for i := 0; i < groupRows(); i++ { // past the group rows
		tm, _ = tm.(Model).Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	tm, cmd := tm.(Model).Update(tea.KeyMsg{Type: tea.KeySpace}) // tick Earth (list row 0)

	got := tm.(Model)
	want := []int{0, 1}
	if !reflect.DeepEqual(got.lens.Objects.Selected(), want) {
		t.Errorf("previewed selection = %v, want %v", got.lens.Objects.Selected(), want)
	}
	if cmd == nil {
		t.Fatal("the toggle scheduled no preview")
	}
	// A preview ARMS the trailing debounce; the compute starts when the tick
	// fires. Settling it is what proves the debounce is the path — draining
	// the Cmd alone would only prove a timer was set.
	settle(t, got, cmd)
	if len(*seen) == 0 {
		t.Fatal("the preview started no compute once the debounce settled")
	}
	for i, lens := range *seen {
		if !reflect.DeepEqual(lens.Objects.Selected(), want) {
			t.Errorf("preview compute %d was handed %v, want the edited %v", i, lens.Objects.Selected(), want)
		}
	}
}

// C8 — an empty selection computes with no bodies rather than falling back to
// the lens default. Asserted at the seam, because "falls back" is precisely the
// failure a model-field assertion cannot see.
func TestEmptyBodySelectionComputesNothing(t *testing.T) {
	seen := recordLensSeams(t)

	m := readyModelWithObjects(testObjectRegistry())
	m.lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1}}

	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	// Untick the only ticked object: the Sun is list row 1, past the group rows.
	for i := 0; i < groupRows()+1; i++ {
		tm, _ = tm.(Model).Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	tm, _ = tm.(Model).Update(tea.KeyMsg{Type: tea.KeySpace})
	_, cmd := tm.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	drainCmd(cmd)

	if len(*seen) == 0 {
		t.Fatal("confirming an empty set ran no compute")
	}
	for i, lens := range *seen {
		if lens.Objects.Baseline != recipe.Explicit {
			t.Errorf("compute %d fell back to baseline %v; the operator chose none, which is not the same as not having chosen", i, lens.Objects.Baseline)
		}
		if n := len(lens.Objects.Selected()); n != 0 {
			t.Errorf("compute %d got %d objects, want none", i, n)
		}
	}
}

// --- objects this Astrolog cannot compute ---------------------------------

// The Log says once which selected objects were dropped, or the operator sees a
// body they ticked simply not appear — the silent-drop failure this whole
// treatment exists to avoid.
func TestUnavailableObjectsAreReportedOnce(t *testing.T) {
	m := readyModelWithObjects(testObjectRegistry())

	m = m.reportUnavailable([]string{"Eris", "Io"})
	if !logMentions(m, "no ephemeris for Eris, Io") {
		t.Fatal("the dropped objects must be named on the Log")
	}
	before := len(m.logLines)

	// ONCE means once per change. The text surface recomputes on every
	// time-step; the same names arriving again must not push the Log out of
	// its six rows.
	m = m.reportUnavailable([]string{"Eris", "Io"})
	if len(m.logLines) != before {
		t.Error("the same set was reported twice; a held-key sweep would flood the Log")
	}
	// Order is Astrolog's, and a reordering is not a change.
	m = m.reportUnavailable([]string{"Io", "Eris"})
	if len(m.logLines) != before {
		t.Error("a reordered set was reported as a change")
	}
}

// Recovery says nothing: the bodies are drawn again, which is what the
// operator sees (the specification). A later loss is
// still reported, because the set was reset.
func TestUnavailableRecoveryIsQuiet(t *testing.T) {
	m := readyModelWithObjects(testObjectRegistry())
	m = m.reportUnavailable([]string{"Eris"})
	before := len(m.logLines)
	m = m.reportUnavailable(nil)
	if len(m.logLines) != before {
		t.Errorf("recovery wrote %v", m.logPanelLines()[before:])
	}
	m = m.reportUnavailable([]string{"Eris"})
	if !logMentions(m, "no ephemeris for Eris") || len(m.logLines) != before+1 {
		t.Error("losing the body again was not reported")
	}
}

// A chart that never had an unavailable object says nothing at all.
func TestNoUnavailableObjectsSaysNothing(t *testing.T) {
	m := readyModelWithObjects(testObjectRegistry())
	before := len(m.logLines)
	m = m.reportUnavailable(nil)
	if len(m.logLines) != before {
		t.Error("a healthy chart logged a line about unavailable bodies")
	}
}

// Overlay mode computes two charts under one lens, so an object with no
// ephemeris is missing from BOTH — and the overlay message used to carry
// neither, which made the body disappear silently in exactly the mode where a
// consulting astrologer is reading a client's transits. Found by the
// independent review, 2026-09-17.
func TestUnavailableNamesAreUnionedNotDropped(t *testing.T) {
	got := unionNames([]string{"Eris", "Io"}, []string{"Io", "Hygiea"})
	want := []string{"Eris", "Io", "Hygiea"}
	if len(got) != len(want) {
		t.Fatalf("unionNames = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unionNames = %v, want %v (first-encounter order, deduplicated)", got, want)
		}
	}
	if n := unionNames(nil, nil); n != nil {
		t.Errorf("unionNames(nil, nil) = %v, want nil", n)
	}
}
