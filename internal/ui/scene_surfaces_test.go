package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// the specification: every compute reads the scene; `t` is gone; nothing
// else may read the selection.

// C2: the text recompute key covers the whole shown set, in order, so a change
// to a slot other than the cursor's recomputes.
func TestTextKeyCoversTheShownSet(t *testing.T) {
	// The key itself: the cursor stays on ①, and ③ joins the set.
	m := selectedSet(t, 3, false, 0)
	m.slots.SetSlotSelected(2, false)
	m = m.noteArrival()
	before := m.comparisonSig()
	m.slots.SetSlotSelected(2, true)
	if m.comparisonSig() == before {
		t.Fatal("a non-cursor slot joined the set and the text key did not change")
	}

	// The order: ⌃j swaps ② and ③ under the cursor on ②. Baseline ① and the
	// pair ② are unchanged; only the shown order differs, and the text frame
	// must be computed again. Driven through the real key and the returned
	// command tree.
	calls := stubPairCompute(t)
	m = selectedSet(t, 3, false, 1)
	m.focused = PanelChart
	m.base = m.slots.All()[0] // ② read in ①
	if p := m.activePairSlot(); p == nil || p.right != m.focusedSlot() {
		t.Fatal("fixture: the cursor's tab is not the active pair")
	}
	sigBefore := m.comparisonSig()
	m, cmd := tpress(t, m, keyCtrlJ)
	if m.comparisonSig() == sigBefore {
		t.Fatal("the shown order changed and the text key did not")
	}
	m = pump(t, m, cmd)
	_, _, cross, synastry := calls.snapshot()
	if len(cross)+len(synastry) == 0 {
		t.Error("reordering the shown set recomputed nothing: the Aspects pair was not asked for")
	}
}

// selectionReadBanned are the store reads only scene.go may make.
var selectionReadBanned = map[string]bool{
	"SelectedInOrder": true, "Numeral": true, "Baseline": true,
	"SelectedCount": true, "SlotSelected": true, "ComparisonSelected": true,
}

// selectionReads lists the positions in src that call a banned store read or
// touch the stored pair choice.
func selectionReads(fset *token.FileSet, f *ast.File) []token.Position {
	var out []token.Position
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok && selectionReadBanned[sel.Sel.Name] {
				out = append(out, fset.Position(sel.Sel.Pos()))
			}
		case *ast.SelectorExpr:
			if x.Sel.Name == "base" {
				out = append(out, fset.Position(x.Sel.Pos()))
			}
		}
		return true
	})
	return out
}

// C3: after the scene, nothing but scene.go reads the selection. session.go is
// exempt by name: it persists the selection on quit; it does not display it.
func TestSelectionIsReadOnlyThroughTheScene(t *testing.T) {
	exempt := map[string]string{
		"scene.go":   "the scene is where the selection is read",
		"pairs.go":   "pairs.go holds the tab memory's accessors: the base is read there, for the scene (activePair) and nowhere else",
		"session.go": "session.go persists the selection; it does not display it",
	}
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no source files: %v", err)
	}
	checked := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || exempt[name] != "" {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, pos := range selectionReads(fset, f) {
			t.Errorf("%s:%d reads the selection outside the scene; derive it from deriveScene", pos.Filename, pos.Line)
		}
	}
	if checked < 20 {
		t.Fatalf("scanned only %d files; the guard is not looking", checked)
	}

	// Positive control: the scanner fires on each banned form.
	const bad = `package ui
func f(m Model) {
	_ = m.slots.SelectedInOrder()
	_ = m.slots.Numeral(nil)
	_ = m.slots.Baseline()
	_ = m.slots.SelectedCount()
	_ = m.slots.SlotSelected(0)
	_ = m.slots.ComparisonSelected()
	_ = m.base
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "bad.go", bad, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(selectionReads(fset, f)); got != 7 {
		t.Errorf("the guard found %d of 7 planted reads", got)
	}
}

// C4: `t` does nothing, appears nowhere, and its handler is gone.
func TestNoComparisonToggleKey(t *testing.T) {
	// Pressed where it used to flip the comparison: no command, no change.
	m := selectedSet(t, 2, false, 0)
	m.focused = PanelChart
	setBefore := len(m.slots.SelectedInOrder())
	pairBefore := m.comparisonSig()
	got, cmd := tpress(t, m, runes("t"))
	if cmd != nil {
		t.Error("`t` returned a command")
	}
	if len(got.slots.SelectedInOrder()) != setBefore || got.comparisonSig() != pairBefore {
		t.Error("`t` changed the selection")
	}
	// A fresh workspace, with its default selection: `t` used to flip it.
	fresh := listModel(t, 2, 0)
	fresh.focused = PanelChart
	freshBefore := len(fresh.slots.SelectedInOrder())
	if got, _ := tpress(t, fresh, runes("t")); len(got.slots.SelectedInOrder()) != freshBefore {
		t.Error("`t` changed the selection of a fresh workspace")
	}

	// Not in the bar, the Keys popup or help: no binding spells it.
	for _, e := range defaultKeys().mainBindings() {
		for _, k := range e.Binding.Keys() {
			if k == "t" {
				t.Errorf("binding %v still answers `t`", e.ID)
			}
		}
		if e.BarKey == "t" {
			t.Errorf("binding %v still shows `t` in the bar", e.ID)
		}
	}
	for _, l := range m.helpLines() {
		if strings.Contains(l.text, "Comparison pair") {
			t.Errorf("help still lists %q", l.text)
		}
	}

	// The handler and the second rule are gone from the source.
	for _, name := range []string{"model.go", "keys.go"} {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"comparisonActive", "toggleComparisonSelection", "BindingToggleOverlay"} {
			if strings.Contains(string(src), banned) {
				t.Errorf("%s still mentions %s", name, banned)
			}
		}
	}
}

// setPairSelection selects or deselects ① and the floating slot the way the
// retired `t` did after changing the store (the same follow-up the Space key
// runs), for tests whose focus is not the Chart panel, where Space is inert.
func setPairSelection(m Model, on bool) (Model, tea.Cmd) {
	m.slots.SetComparisonSelected(on)
	m = m.pruneTabMemory().noteArrival()
	if m.focused == PanelAspects {
		m = m.applyAspectsDetails()
	}
	return m.afterComparisonChange()
}
