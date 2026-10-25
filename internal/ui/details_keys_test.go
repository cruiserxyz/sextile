package ui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// D36 (operator, 2026-10-13: "details should simulate keystrokes to the chart
// panel instead of having an own set of hotkeys"): no Details-owned file moves
// the Chart cursor or changes the selection itself; it steps the Chart panel's
// own action (walkCursorTo). E2 (operator, 2026-10-24: "UI actions in regards
// to panels MUST be user-induced ONLY") retired the half that focused the
// panel for the press: the walks never touch the focus.
func TestDetailsStepsThePanelsActionsWithoutFocus(t *testing.T) {
	forbidden := map[string]bool{
		"focusSlot": true, "cycleSlotCursor": true, "cycleSlot": true, "cycleFocusedRow": true,
		"SetSlotSelected": true, "ToggleSlotSelection": true, "toggleFocusedSelection": true,
		// The Bodies panel's own keys too (the specification).
		"cycleBodyCursor": true,
	}
	for _, file := range []string{"timeline.go", "details.go"} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.AssignStmt:
				for _, l := range x.Lhs {
					sel, ok := l.(*ast.SelectorExpr)
					if ok && sel.Sel.Name == "cursorIdx" {
						t.Errorf("%s:%d sets the Chart cursor itself; press the Chart panel's key (D36)", file, fset.Position(sel.Pos()).Line)
					}
					if ok && sel.Sel.Name == "bodyCursorIdx" {
						t.Errorf("%s:%d sets the Bodies cursor itself; press the Bodies panel's key (D36)", file, fset.Position(sel.Pos()).Line)
					}
				}
			case *ast.CallExpr:
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok && forbidden[sel.Sel.Name] {
					t.Errorf("%s:%d calls %s itself; press the Chart panel's key (D36)", file, fset.Position(sel.Pos()).Line, sel.Sel.Name)
				}
			}
			return true
		})
	}
	// The walks take the panels' own actions with the focus left alone: no
	// assignment to the focus in them or in stateOnly.
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "model.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	walks := map[string]bool{"walkCursorTo": true, "walkBodyCursorTo": true, "stateOnly": true}
	seen := 0
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || !walks[fn.Name.Name] {
			continue
		}
		seen++
		ast.Inspect(fn, func(n ast.Node) bool {
			if as, ok := n.(*ast.AssignStmt); ok {
				for _, l := range as.Lhs {
					if sel, ok := l.(*ast.SelectorExpr); ok && sel.Sel.Name == "focused" {
						t.Errorf("model.go:%d: %s moves the focus; a walk is never a focus change (E2)", fset.Position(sel.Pos()).Line, fn.Name.Name)
					}
				}
			}
			return true
		})
	}
	if seen != len(walks) {
		t.Errorf("model.go declares %d of the walks %v; the check reads nothing it cannot find", seen, walks)
	}
}

// Enter on a timeline row moves the Chart cursor to now's line, read in ②
// (the floating slot's selection stays as it was), and moves no focus: the
// ring panel Details was attached from keeps it and the Bodies row stays where
// it was (operator, 2026-10-24: "UI actions in regards to panels MUST be
// user-induced ONLY").
func TestEnterMovesTheChartCursorWithoutMovingFocus(t *testing.T) {
	// With now selected: a group without it has no Timeline tab to attach.
	m, _ := enteredFixture(t, 2, true, Config{Accessible: true})
	// The timeline is the Chart's tab (the specification acceptance 7), so
	// Details can only be attached on it from the Chart.
	bodyRow := m.bodyCursorIdx
	i := chartRowOf(t, m.timeline.rows, 2)
	m = enter(t, m, i)
	if !m.focusedIsFloating() || m.activePairSlot() == nil || m.activePairSlot().left != m.slots.All()[1] {
		t.Errorf("Enter left the Chart cursor at %d, want now's line read in ②", m.cursorIdx)
	}
	all := m.slots.All()
	if got := m.slots.Numeral(all[len(all)-1]); got != 3 {
		t.Errorf("the floating slot's numeral is %d after Enter, want 3 (selected before, as before)", got)
	}
	if m.focused != PanelChart || m.bodyCursorIdx != bodyRow {
		t.Errorf("after Enter the focus is %v and the Bodies row %d, want Chart and row %d as before", m.focused, m.bodyCursorIdx, bodyRow)
	}
}

// E1 (operator, 2026-10-24: "the UI simulating details-exit, refocus,
// reenter-details is NOT permitted"): Details stays attached and drawn
// focused through the whole Enter, in the state the key leaves and in every
// state a delivered message leaves after it; the ring focus never moves. Only
// the Chart cursor and the Aspects tab change, underneath.
func TestEnterOnATimelineRowKeepsDetailsFocused(t *testing.T) {
	m, _ := enteredFixture(t, 2, true, Config{Color: ColorAlways})
	focus := m.focused
	held := func(m Model, when string) {
		t.Helper()
		if !m.details.attached || !m.detailsFrame().state.attached || m.focused != focus {
			t.Errorf("%s: Details attached %v, drawn focused %v, ring focus %v; want attached, drawn focused, %v",
				when, m.details.attached, m.detailsFrame().state.attached, m.focused, focus)
		}
	}
	// Now is cast in its own houses, so the walk to it changes the list's lens
	// and the timeline waits for the new list: the state in which Details was
	// drawn detached and the Chart panel focused.
	all := m.slots.All()
	floating := all[len(all)-1]
	m.frames.volatile = map[string]recipe.HouseSystem{volatileKey(floating): recipe.Placidus}
	m.details.cursor = chartRowOf(t, m.timeline.rows, 2)
	m, cmd := tpress(t, m, keyEnter)
	if !m.timelineWaiting() {
		t.Fatal("after the key the timeline is not waiting; the fixture no longer reaches the state under test")
	}
	held(m, "after the key")
	n := 0
	m = pumpSeen(t, m, 400*time.Millisecond, func(m Model) { n++; held(m, fmt.Sprintf("after message %d", n)) }, cmd)
	m = m.releaseDetailsHold(detailsHoldMsg{id: m.detailsHold.id})
	held(m, "settled")
	if !m.focusedIsFloating() {
		t.Errorf("settled: the Chart cursor is at %d, want now", m.cursorIdx)
	}
	wantTab(t, m, "③ in ②", "settled")
}
