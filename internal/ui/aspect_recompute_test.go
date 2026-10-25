package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// requirement C3, the half that matters.
//
// The first version of this test asserted that confirming the chooser set
// m.lens.Aspects — and it PASSED while the chart never changed, because the
// text-surface computes passed lookup.Hellenistic, the package constant, and
// never looked at the model's lens at all. That is precisely the failure the
// spec warned about: "not merely that the value reached a compiler input, which
// a stale already-launched computation satisfies while the operator sees the
// old set".
//
// So this test binds to what the COMPUTE receives. Combined with
// TestAspectPolicyArgv in internal/invariants (policy → `-RA0` argv), the chain
// from keystroke to Astrolog is covered end to end: here the operator's choice
// reaches the compute, there it reaches the command line.

// recordLensSeams swaps the three lookup entry points for recorders and returns
// a pointer to the lens each was handed.
func recordLensSeams(t *testing.T) *[]lookup.Lens {
	t.Helper()
	seen := &[]lookup.Lens{}

	natal, transits, overlay := firstCallNatalPositions, firstCallTransits, overlayCrossAspects
	t.Cleanup(func() {
		firstCallNatalPositions, firstCallTransits, overlayCrossAspects = natal, transits, overlay
	})

	firstCallNatalPositions = func(_ context.Context, _ *lookup.Probers, _ store.AnchorChart, lens lookup.Lens) (lookup.Result, error) {
		*seen = append(*seen, lens)
		return lookup.Result{}, context.Canceled
	}
	firstCallTransits = func(_ context.Context, _ *lookup.Probers, _ time.Time, _ location.PlaceInfo, lens lookup.Lens) (lookup.Result, error) {
		*seen = append(*seen, lens)
		return lookup.Result{}, context.Canceled
	}
	overlayCrossAspects = func(_ context.Context, lens lookup.Lens, _ store.AnchorChart, _ time.Time, _ location.PlaceInfo) (lookup.OverlayAspects, error) {
		*seen = append(*seen, lens)
		return lookup.OverlayAspects{}, context.Canceled
	}
	return seen
}

func TestConfirmedAspectsReachTheCompute(t *testing.T) {
	seen := recordLensSeams(t)

	m := readyModelWithRegistry(testRegistry())
	m.lens.Aspects = recipe.AspectPolicy{Explicit: true, Indices: []int{1, 2, 3, 4, 5}}

	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	// Clear the Ptolemaic family, leaving a strictly smaller set.
	tm, _ = tm.(Model).Update(tea.KeyMsg{Type: tea.KeySpace})
	tm, cmd := tm.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})

	got := tm.(Model)
	if got.startup == startupAspectOpen {
		t.Fatal("Enter left the chooser open")
	}
	if cmd == nil {
		t.Fatal("confirming scheduled no recompute — the chart would keep showing the old set")
	}

	// Drain the command so the compute seam actually runs.
	drainCmd(cmd)

	if len(*seen) == 0 {
		t.Fatal("confirming started no compute at all")
	}
	want := got.lens.Aspects
	for i, lens := range *seen {
		if !lens.Aspects.Explicit {
			t.Errorf("compute %d received a non-explicit policy — it would use the host default, not the operator's choice", i)
			continue
		}
		if len(lens.Aspects.Selected()) != len(want.Selected()) {
			t.Errorf("compute %d received %v, want the confirmed %v", i, lens.Aspects.Selected(), want.Selected())
		}
	}
}

// The regression guard in its sharpest form: no compute may be built from the
// package-level default once the operator has chosen. That constant is what the
// three compute commands used to hardcode.
func TestComputeNeverUsesTheDefaultLensAfterAChoice(t *testing.T) {
	seen := recordLensSeams(t)

	m := readyModelWithRegistry(testRegistry())
	m.lens.Aspects = recipe.AspectPolicy{Explicit: true, Indices: []int{4}}

	_, cmd := m.bumpIdentity(surfaceText)
	if cmd == nil {
		t.Fatal("bumpIdentity scheduled nothing")
	}
	drainCmd(cmd)

	if len(*seen) == 0 {
		t.Fatal("no compute ran")
	}
	for i, lens := range *seen {
		if lens.Aspects.Explicit == lookup.Hellenistic.Aspects.Explicit && !lens.Aspects.Explicit {
			t.Errorf("compute %d was handed the package default lens; the operator's choice is not in it", i)
		}
	}
}

// drainCmd runs a tea.Cmd and any commands it batches, so the seams underneath
// actually execute. A Cmd that merely exists proves nothing.
func drainCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			drainCmd(c)
		}
	}
}

// --- live preview ---------------------------------------------------------

// A toggle repaints the panels behind the modal, so the set is judged against
// the chart rather than against memory.
func TestToggleLivePreviewsBehindTheModal(t *testing.T) {
	seen := recordLensSeams(t)

	m := readyModelWithRegistry(testRegistry())
	m.lens.Aspects = recipe.AspectPolicy{Explicit: true, Indices: []int{1, 2, 3, 4, 5}}
	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})

	tm, cmd := tm.(Model).Update(tea.KeyMsg{Type: tea.KeySpace}) // clear Ptolemaic
	if cmd == nil {
		t.Fatal("a toggle scheduled no preview")
	}
	if tm.(Model).startup != startupAspectOpen {
		t.Error("the preview closed the modal")
	}
	// A preview ARMS the 50ms trailing debounce; the compute starts when the
	// tick fires. Settling it here is what proves the debounce is the path —
	// draining the Cmd alone would only prove a timer was set.
	settle(t, tm.(Model), cmd)

	if len(*seen) == 0 {
		t.Fatal("the preview started no compute once the debounce settled")
	}
	for i, lens := range *seen {
		if len(lens.Aspects.Selected()) == 5 {
			t.Errorf("preview %d still carries the pre-toggle set", i)
		}
	}
}

// A burst of toggles must coalesce into ONE compute. That is the whole reason
// preview is affordable: legacy previewed per keystroke, and here holding Space
// would otherwise be one Astrolog process per repeat.
func TestTogglesCoalesceIntoOneCompute(t *testing.T) {
	seen := recordLensSeams(t)

	m := readyModelWithRegistry(testRegistry())
	m.lens.Aspects = recipe.AspectPolicy{Explicit: true, Indices: []int{1, 2, 3, 4, 5}}
	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})

	var last tea.Cmd
	for i := 0; i < 6; i++ {
		tm, last = tm.(Model).Update(tea.KeyMsg{Type: tea.KeySpace})
	}
	settle(t, tm.(Model), last)

	if n := len(*seen); n != 1 {
		t.Errorf("six toggles produced %d computes, want 1 — the debounce is not coalescing", n)
	}
}

// settle runs a debounce Cmd, feeds the resulting tick back into the model the
// way the bubbletea loop would, and drains whatever that starts.
func settle(t *testing.T, m Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			settle(t, m, c)
		}
		return
	}
	tm, next := m.Update(msg)
	_ = tm
	drainCmd(next)
}

// Typing and moving the cursor must NOT preview. Only a changed selection is a
// new chart; filtering is just looking.
func TestFilteringDoesNotPreview(t *testing.T) {
	m := readyModelWithRegistry(testRegistry())
	m.lens.Aspects = recipe.AspectPolicy{Explicit: true, Indices: []int{1, 2}}
	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})

	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("s")},
		{Type: tea.KeyRunes, Runes: []rune("q")},
		{Type: tea.KeyDown},
		{Type: tea.KeyUp},
		{Type: tea.KeyBackspace},
	} {
		var cmd tea.Cmd
		tm, cmd = tm.(Model).Update(msg)
		if cmd != nil {
			t.Errorf("%v scheduled a compute; only a changed selection is a new chart", msg.Type)
		}
	}
}

// The trap: with preview on, Esc must not merely restore the policy in the
// model — it has to RECOMPUTE, or the cancelled selection stays on screen and
// the cancel is a lie.
func TestCancelRecomputesAwayThePreview(t *testing.T) {
	oldChooserRule(t)
	seen := recordLensSeams(t)
	entry := recipe.AspectPolicy{Explicit: true, Indices: []int{1, 2, 3, 4, 5}}

	m := readyModelWithRegistry(testRegistry())
	m.lens.Aspects = entry
	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	tm, previewCmd := tm.(Model).Update(tea.KeyMsg{Type: tea.KeySpace})
	drainCmd(previewCmd)
	*seen = nil

	tm, cancelCmd := tm.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := tm.(Model)
	if got.startup == startupAspectOpen {
		t.Fatal("Esc left the chooser open")
	}
	if !sameSelection(got.lens.Aspects, entry) {
		t.Errorf("cancel left %v in force, want the entry set %v", got.lens.Aspects.Selected(), entry.Selected())
	}
	if cancelCmd == nil {
		t.Fatal("cancel scheduled no recompute — the previewed set would stay on screen")
	}
	drainCmd(cancelCmd)
	if len(*seen) == 0 {
		t.Fatal("cancel started no compute")
	}
	for i, lens := range *seen {
		if !sameSelection(lens.Aspects, entry) {
			t.Errorf("cancel recompute %d used %v, want the entry set %v", i, lens.Aspects.Selected(), entry.Selected())
		}
	}
}

// Cancelling without having touched anything must not spend a compute.
func TestCancelWithoutEditsRecomputesNothing(t *testing.T) {
	m := readyModelWithRegistry(testRegistry())
	m.lens.Aspects = recipe.AspectPolicy{Explicit: true, Indices: []int{1, 2}}
	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	_, cmd := tm.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Error("cancelling an untouched chooser scheduled a recompute")
	}
}

// The whole point of a preview: the modal STAYS OPEN while the surfaces behind
// it change. `startup` names both the startup phase and the open modal, so the
// frame commit used to promote to startupReady unconditionally and the
// preview's own result slammed the chooser shut the moment it landed.
func TestPreviewResultDoesNotCloseTheModal(t *testing.T) {
	m := readyModelWithRegistry(testRegistry())
	m.lens.Aspects = recipe.AspectPolicy{Explicit: true, Indices: []int{1, 2, 3, 4, 5}}

	tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	tm, _ = tm.(Model).Update(tea.KeyMsg{Type: tea.KeySpace})
	if tm.(Model).startup != startupAspectOpen {
		t.Fatal("the toggle itself closed the chooser")
	}

	// The preview's compute lands.
	tm, _ = tm.(Model).Update(FirstFrameReadyMsg{
		Positions:    fullChartPositions(),
		Aspects:      tasteOwnAspects(),
		CrossAspects: tasteCrossAspects(),
		Overlay:      true,
	})
	got := tm.(Model)

	if got.startup != startupAspectOpen {
		t.Errorf("startup = %q after the preview committed, want the chooser still open", got.startup)
	}
	if !got.modalOpen() {
		t.Error("modalOpen() went false when the preview landed")
	}
	if !strings.Contains(got.View(), "Ptolemaic") {
		t.Error("the chooser is no longer on screen after its own preview committed")
	}
	// ...and the frame underneath really did update.
	if len(got.chart.aspects) == 0 {
		t.Error("the committed frame did not reach the model")
	}
}

// A commit arriving in a NON-modal state must still promote to ready, or the
// ordinary startup path never finishes.
func TestCommitStillPromotesWhenNoModalIsOpen(t *testing.T) {
	m := NewModel(Config{Accessible: true, Color: ColorNever})
	m.width, m.height = 132, 40
	if m.startup == startupReady {
		t.Fatal("fixture is already ready")
	}
	tm, _ := m.Update(FirstFrameReadyMsg{Positions: fullChartPositions()})
	if got := tm.(Model).startup; got != startupReady {
		t.Errorf("startup = %q after a commit with no modal open, want %q", got, startupReady)
	}
}
