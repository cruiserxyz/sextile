package ui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// ctrlMsg is the key Bubble Tea reports for Ctrl and a lower-case letter.
func ctrlMsg(letter string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyType(letter[0] - 'a' + 1)}
}

// mapShape is what a command leaves on the screen's state, enough to tell one
// command's run from another's.
func mapShape(m Model) string {
	return fmt.Sprintf("%v|%v|%v|%v|%v|%v|%d", m.startup, m.keybindingsOpen, m.keybindingsOpen, m.logFocus.active, m.console.open,
		m.leader.pending, len(m.logLines))
}

func sendKeys(t *testing.T, m Model, msgs ...tea.Msg) Model {
	t.Helper()
	var tm tea.Model = m
	for _, msg := range msgs {
		tm, _ = tm.Update(msg)
	}
	return tm.(Model)
}

func commaMsgs(c keys.Command) []tea.Msg {
	out := []tea.Msg{keyMsg(","), keyMsg(c.Letter)}
	if c.Then != "" {
		out = append(out, keyMsg(c.Then))
	}
	return out
}

// Every row of the table: `,` then its letters runs its command, on the main
// screen, exactly as the command run by its id does.
func TestEveryCommandLetterRunsItsCommand(t *testing.T) {
	for _, c := range keys.Commands() {
		t.Run(c.Letter+c.Then, func(t *testing.T) {
			sessionSandbox(t)
			base := leaderModel(t)
			got := sendKeys(t, base, commaMsgs(c)...)
			want, _, ok := base.runAction(c.ID)
			if !ok {
				t.Fatalf("%v is not a command", c.ID)
			}
			if mapShape(got) != mapShape(want.(Model)) {
				t.Errorf("`, %s%s` left %s, the command by its id left %s", c.Letter, c.Then, mapShape(got), mapShape(want.(Model)))
			}
			if got.leader.pending {
				t.Error("the menu is still pending")
			}
		})
	}
}

// Every command with a Ctrl key runs the same command by it; O, L and c d have none.
func TestEveryDerivedCtrlKeyRunsTheSameCommand(t *testing.T) {
	derived := 0
	for _, c := range keys.Commands() {
		k, ok := keys.CommandCtrl(c)
		if !ok {
			continue
		}
		derived++
		t.Run(k, func(t *testing.T) {
			sessionSandbox(t)
			base := leaderModel(t)
			got := sendKeys(t, base, ctrlMsg(c.Letter))
			want := sendKeys(t, base, commaMsgs(c)...)
			if mapShape(got) != mapShape(want) {
				t.Errorf("%s left %s, `, %s` left %s", k, mapShape(got), c.Letter, mapShape(want))
			}
			if mapShape(got) == mapShape(base) && mapShape(want) != mapShape(base) {
				t.Errorf("%s did nothing", k)
			}
		})
	}
	if derived != 10 {
		t.Errorf("%d commands derive a Ctrl key, want 10", derived)
	}
}

// Ctrl+H has no meaning on the main screen any more.
func TestCtrlHDoesNothingOnTheMainScreen(t *testing.T) {
	base := leaderModel(t)
	got := sendKeys(t, base, tea.KeyMsg{Type: tea.KeyCtrlH})
	if mapShape(got) != mapShape(base) {
		t.Errorf("Ctrl+H left %s, want %s", mapShape(got), mapShape(base))
	}
}

// `, c d` keeps the defaults; `, c` waits for d, and any other key cancels.
func TestCommaCDKeepsAndCommaCThenAnythingCancels(t *testing.T) {
	sessionSandbox(t)
	m := leaderModel(t)
	m.place, m.placeFromZone = amsterdamNL, true
	before := profileSnapshot(t)
	lines := len(m.logLines)

	m, _ = press(t, m, ",", "c")
	if !m.leader.pending {
		t.Fatal("`, c` is not waiting for a second key")
	}
	m, cmd := press(t, m, "x")
	if m.leader.pending || cmd != nil || len(m.logLines) != lines || !m.placeFromZone {
		t.Errorf("`, c x`: pending=%v cmd=%v log %d -> %d, guessed=%v", m.leader.pending, cmd != nil, lines, len(m.logLines), m.placeFromZone)
	}
	sameProfile(t, before, profileSnapshot(t), "`, c x`")

	m, cmd = press(t, m, ",", "c", "d")
	if m.leader.pending || cmd == nil {
		t.Errorf("`, c d`: pending=%v cmd=%v, want the keep to start", m.leader.pending, cmd != nil)
	}
}

// A pending comma is still pending after any message that is not a key.
func TestPendingCommaOutlastsEveryNonKeyMessage(t *testing.T) {
	m := sendKeys(t, leaderModel(t), keyMsg(","))
	m = sendKeys(t, m, tea.WindowSizeMsg{Width: 100, Height: 30}, nowTickMsg{}, yankFlashMsg{})
	if !m.leader.pending {
		t.Error("the comma stopped waiting without a key")
	}
	m = sendKeys(t, m, keyMsg("esc"))
	if m.leader.pending {
		t.Error("Esc did not end the menu")
	}
}

// Ctrl+Y copies where `, y` copies: the main panels and the focused Log.
func TestCtrlYCopiesWhereCommaYCopies(t *testing.T) {
	t.Run("main", func(t *testing.T) {
		forEachList(t, func(t *testing.T, lk timelineList) {
			copied := captureYank(t)
			stubTextCompute(t)
			m := loadedList(t, lk, Config{Accessible: true})
			m.focused = PanelChart
			target := m.targetFloatingTime
			m = sendKeys(t, m, ctrlMsg("y"))
			if len(*copied) != 1 || !m.targetFloatingTime.Equal(target) {
				t.Fatalf("Ctrl+Y: copied %d, target moved %v; want one copy and no step", len(*copied), !m.targetFloatingTime.Equal(target))
			}
			viaComma := loadedList(t, lk, Config{Accessible: true})
			viaComma.focused = PanelChart
			c2 := captureYank(t)
			viaComma = sendKeys(t, viaComma, keyMsg(","), keyMsg("y"))
			if len(*c2) != 1 || (*c2)[0] != (*copied)[0] {
				t.Errorf("`, y` copied %q, Ctrl+Y copied %q", *c2, *copied)
			}
		})
	})
	t.Run("log", func(t *testing.T) {
		copied := captureYank(t)
		m := logModel(t, 3)
		m, _ = press(t, m, ",", "L")
		if !m.logFocus.active {
			t.Fatal("the Log did not take the focus")
		}
		m = sendKeys(t, m, ctrlMsg("y"))
		if len(*copied) != 1 || !m.logFocus.active {
			t.Errorf("Ctrl+Y in the Log: copied %d, Log focused %v; want one copy and the focus kept", len(*copied), m.logFocus.active)
		}
	})
	t.Run("console", func(t *testing.T) {
		copied := captureYank(t)
		m := consoleModel(t, 5)
		m, _ = press(t, m, ",", "L", "l")
		if !m.console.open {
			t.Fatal("the console did not open")
		}
		m = sendKeys(t, m, ctrlMsg("y"))
		if len(*copied) != 1 || !m.console.open {
			t.Errorf("Ctrl+Y in the console: copied %d, open %v", len(*copied), m.console.open)
		}
	})
}

// Ctrl+W does nothing: the folder row has no key.
func TestCtrlWDoesNothing(t *testing.T) {
	m, dir := keyWorld(t, store.PositionProtected)
	before := mapShape(m)
	m = sendKeys(t, m, ctrlMsg("w"), ctrlMsg("w"))
	if stored(dir) != store.PositionProtected || mapShape(m) != before {
		t.Errorf("Ctrl+W twice: stored %q, shape %q (was %q); want nothing changed", stored(dir), mapShape(m), before)
	}
}

// No derived Ctrl key opens a command while a popup has the keyboard.
func TestCtrlOInsideTheCityChooserDoesNotOpenTheChartPicker(t *testing.T) {
	sessionSandbox(t)
	m := sendKeys(t, leaderModel(t), ctrlMsg("l"))
	if m.startup != startupAtlasOpen {
		t.Fatalf("premise: Ctrl+L left startup %q, want the city chooser", m.startup)
	}
	m = sendKeys(t, m, ctrlMsg("o"))
	if m.startup == startupChartOpen {
		t.Error("Ctrl+O inside the city chooser opened the chart picker")
	}
}
