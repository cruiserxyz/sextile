package ui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// hotkey_coverage_test.go holds every hotkey to the same three promises, over
// the WHOLE keymap rather than a hand-picked sample:
//
//  1. every binding declares at least one key, and a help label if it is
//     help-visible;
//  2. no two main-scope bindings claim the same key;
//  3. every main-scope key actually DOES something when pressed — it either
//     changes the model or returns a command.
//
// Promise 3 is the one that matters. A binding can be declared, appear in the
// help modal, pass the help-parity invariant, and still be wired to nothing,
// because nothing else in the suite presses every key. That is exactly how a
// hotkey dies quietly: the table says it exists and the switch never names it.
//
// When this test fails for a NEW binding, the fix is to wire the key — never to
// add it to the exemption list below without a reason written next to it.

// keyMsgFor turns a binding's first declared key into the KeyMsg bubbletea
// would deliver. It covers the forms this keymap actually uses.
func keyMsgFor(t *testing.T, k string) tea.KeyMsg {
	t.Helper()
	switch k {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case " ", "space":
		return tea.KeyMsg{Type: tea.KeySpace}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	case "ctrl+pgup":
		return tea.KeyMsg{Type: tea.KeyCtrlPgUp}
	case "ctrl+pgdown":
		return tea.KeyMsg{Type: tea.KeyCtrlPgDown}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "delete":
		return tea.KeyMsg{Type: tea.KeyDelete}
	}
	if strings.HasPrefix(k, "ctrl+") {
		// bubbletea encodes ctrl+<letter> as the ASCII control code, so the
		// KeyTypes live at 0..31 — NOT in the negative range the arrow and
		// function keys use. Found by search rather than a table so it
		// cannot drift, and the range is asserted below.
		for kt := tea.KeyType(0); kt <= 31; kt++ {
			if (tea.KeyMsg{Type: kt}).String() == k {
				return tea.KeyMsg{Type: kt}
			}
		}
	}
	if len([]rune(k)) == 1 {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
	t.Fatalf("hotkey coverage: no KeyMsg form known for %q — teach keyMsgFor about it", k)
	return tea.KeyMsg{}
}

func TestEveryBindingIsDeclaredWell(t *testing.T) {
	for _, e := range defaultKeys().mainBindings() {
		name := fmt.Sprintf("binding-%d", int(e.ID))
		if len(e.Binding.Keys()) == 0 {
			t.Errorf("%s declares no keys", name)
			continue
		}
		for _, k := range e.Binding.Keys() {
			if strings.TrimSpace(k) == "" && k != " " {
				t.Errorf("%s declares an empty key", name)
			}
		}
		if e.Help && e.Binding.Help().Desc == "" {
			t.Errorf("%s is help-visible with no description", name)
		}
	}
}

func TestNoTwoBindingsClaimTheSameKey(t *testing.T) {
	owner := map[string]BindingID{}
	for _, e := range defaultKeys().mainBindings() {
		for _, k := range e.Binding.Keys() {
			if prev, taken := owner[k]; taken && prev != e.ID {
				t.Errorf("key %q is claimed by both binding %d and binding %d", k, int(prev), int(e.ID))
				continue
			}
			owner[k] = e.ID
		}
	}
	// The leader key must not also be a main-scope binding, or arming the
	// leader and firing that binding are the same keystroke.
	if id, taken := owner[defaultLeaderKey]; taken {
		t.Errorf("the leader key %q is also main-scope binding %d", defaultLeaderKey, int(id))
	}
}

// inertByDesign lists keys that legitimately do nothing from EVERY candidate
// state below. Each needs a reason written next to it; anything else failing
// this test is a dead key, and the fix is to wire it.
var inertByDesign = map[string]string{
	"esc": `on the main screen esc cancels a pending leader and nothing else ` +
		`(BindingCancelLeader), and none of the candidate states has one pending`,
	"l": `the log console is a LEADER binding (",l"), so bare "l" in the main ` +
		`table is correctly inert: the leader resolver dispatches it`,
}

// hotkeyStates are the starting points a key is tried against. A key passes if
// it has an effect in ANY of them, because several bindings are legitimately
// no-ops in a particular state — jumping to the panel already focused, paging
// Details that are not scrollable, moving a cursor already at the end. Testing
// one state only would force those into the exemption list and hide a genuinely
// dead key behind a plausible excuse.
func hotkeyStates(t *testing.T) []Model {
	t.Helper()

	base := tabsPairModel(t, Config{Accessible: true, Color: ColorNever})
	base.width, base.height = 132, 40
	base, _ = applyTextCommit(base, FirstFrameReadyMsg{
		Positions:    fullChartPositions(),
		Aspects:      tasteOwnAspects(),
		CrossAspects: tasteCrossAspects(),
		Overlay:      true,
	})
	base.startup = startupReady
	// BOTH registries. Seeding only the aspect one left ⌃b permanently on
	// its not-yet-read branch, where its only effect was the duplicate
	// registry read it used to start — so this test passed for ⌃b on the
	// strength of a defect, and went red the moment that was fixed.
	base.aspectRegistry = testRegistry()
	base.objectRegistry = testObjectRegistry()

	onChart := base
	onChart.focused = PanelChart

	onAspects := base
	onAspects.focused = PanelAspects

	onBodies := base
	onBodies.focused = PanelBodies

	// Details open and scrollable, so the paging keys have somewhere to go.
	scrolled := scrollModel(t, 2)
	scrolled.aspectRegistry = testRegistry()
	scrolled.objectRegistry = testObjectRegistry()

	// A workspace that is a list: three anchored charts above the floating
	// slot, the cursor on the middle one, so the list keys (⌃k, ⌃j, del) have
	// somewhere to act (the specification C7, C8).
	onList := onChart
	onList.slots = listWorkspace(t, 3)
	onList.cursorIdx = 1
	onList.bodyCursorIdx = 0
	onList.bodyRows = nil

	// A previous time for `N` to jump back to, and a pending leader for `b`
	// (it is a leader binding): both keys used to "work" here only by starting
	// the Details timeline search from Bodies or Aspects focus, which
	// the specification retires.
	withBack := onChart
	withBack.targetFloatingTime = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	withBack.previousFloatingTime = withBack.targetFloatingTime.AddDate(0, 0, -1)
	leaderArmed := onChart
	leaderArmed.leader.pending = true

	return []Model{onChart, onAspects, onBodies, scrolled, onList, withBack, leaderArmed}
}

// TestEveryMainScopeKeyDoesSomething presses every main-scope key against each
// candidate state and requires an effect somewhere: a changed frame or a
// returned command.
func TestEveryMainScopeKeyDoesSomething(t *testing.T) {
	for _, e := range defaultKeys().mainBindings() {
		for _, k := range e.Binding.Keys() {
			k := k
			id := int(e.ID)
			t.Run(fmt.Sprintf("%d-%s", id, k), func(t *testing.T) {
				for _, before := range hotkeyStates(t) {
					tm, cmd := before.Update(keyMsgFor(t, k))
					after := tm.(Model)
					if cmd != nil || !modelsLookEqual(before, after) {
						return
					}
				}
				if why, ok := inertByDesign[k]; ok {
					t.Logf("inert by design: %s", why)
					return
				}
				t.Errorf("key %q (binding %d) changed nothing and started nothing in any state — it is declared but not wired", k, id)
			})
		}
	}
}

// modelsLookEqual compares what a keystroke could visibly or materially move.
// The rendered frame is the primary comparison — it catches a moved cursor, a
// selected slot, a scrolled panel and an opened modal without this test having
// to know which field each of those lives in. The refresh coordinator and the
// aspect policy are compared too, because a key may schedule work without the
// frame changing yet.
func modelsLookEqual(a, b Model) bool {
	return a.View() == b.View() &&
		a.startup == b.startup &&
		reflect.DeepEqual(a.refresh, b.refresh) &&
		reflect.DeepEqual(a.lens.Aspects, b.lens.Aspects)
}
