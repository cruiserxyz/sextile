package ui

import (
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// A burst — several runes the terminal read at once, from key repeat or a
// macro — is the same keys typed slowly. These tests send the burst as the
// ONE KeyRunes the terminal delivers, never as separate presses, because a
// separate press is the case that already worked.

func TestABurstMovesTheAttachedTimelineCursorKeyByKey(t *testing.T) {
	m := onTimeline(t, Config{Accessible: true})
	slot := m.cursorIdx
	m, _ = tpress(t, m, runes("jjj"))
	if m.details.cursor != 3 {
		t.Fatalf("jjj as one read: cursor %d, want 3", m.details.cursor)
	}
	m, _ = tpress(t, m, runes("kk"))
	if m.details.cursor != 1 {
		t.Fatalf("kk as one read: cursor %d, want 1", m.details.cursor)
	}
	if m.cursorIdx != slot || !m.details.attached {
		t.Fatalf("a burst moved the slot or detached: slot %d -> %d, attached=%v", slot, m.cursorIdx, m.details.attached)
	}
}

// A bracketed paste is text, not keys: replaying it would let a pasted `,y`
// copy and a pasted `jjj` move the cursor.
func TestAPasteIsNotReplayedAsKeys(t *testing.T) {
	var copied int
	old := yankLine
	yankLine = func(string) error { copied++; return nil }
	t.Cleanup(func() { yankLine = old })
	m := onTimeline(t, Config{Accessible: true})
	for _, s := range []string{"jjj", ",y"} {
		m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s), Paste: true})
	}
	if m.details.cursor != 0 || copied != 0 || !m.details.attached {
		t.Fatalf("a paste acted as keys: cursor %d, copied %d, attached %v", m.details.cursor, copied, m.details.attached)
	}
}

func TestABurstMovesTheConsoleCursorKeyByKey(t *testing.T) {
	m := openedConsole(t, 20)
	start := m.console.cursor
	m, _ = tpress(t, m, runes("kkk"))
	if m.console.cursor != start-3 {
		t.Fatalf("kkk as one read: cursor %d, want %d", m.console.cursor, start-3)
	}
	if !m.console.open {
		t.Fatal("a burst closed the console")
	}
}

// A quit inside a burst on the main screen quits, after the keys before it
// and without the keys after it: the session is already written when they
// would act. Inside the console `q` closes it as Esc does and is not a quit
// (C11), so the same burst there closes and quits nothing.
func TestABurstEndsAtQuit(t *testing.T) {
	m := consoleModel(t, 20)
	if err := addAnchor(m.slots, time.Date(1990, 1, 1, 14, 0, 0, 0, time.UTC), testPlaceAmsterdam()); err != nil {
		t.Fatal(err)
	}
	m.focused = PanelChart
	m.cursorIdx = 0
	got, cmd := tpress(t, m, runes("jqk"))
	if !quits(cmd) {
		t.Fatal("jqk as one read did not quit")
	}
	if got.cursorIdx != 1 {
		t.Fatalf("jqk as one read: slot cursor %d, want 1 (the j before q only)", got.cursorIdx)
	}

	c := openedConsole(t, 20)
	got, cmd = tpress(t, c, runes("kqk"))
	if quits(cmd) {
		t.Fatal("kqk in the console quit: q is the main screen's")
	}
	if got.console.open {
		t.Fatal("kqk in the console: q did not close it")
	}
}

// isBatch reports whether cmd is a tea.Batch or tea.Sequence of several
// commands (bubbletea builds both in compactCmds), which can be opened without
// running anything. Any other command may be a tick or a compute, and is never
// called here.
func isBatch(cmd tea.Cmd) bool {
	// The closure's name carries the caller's when the generic is inlined
	// (`….replayBurst.Batch.compactCmds[...].func1`), so match the segment.
	return strings.Contains(runtime.FuncForPC(reflect.ValueOf(cmd).Pointer()).Name(), ".compactCmds[")
}

// cmdLeaves is cmd's commands with every batch opened.
func cmdLeaves(cmd tea.Cmd) []tea.Cmd {
	if cmd == nil {
		return nil
	}
	if !isBatch(cmd) {
		return []tea.Cmd{cmd}
	}
	var out []tea.Cmd
	list := reflect.ValueOf(cmd())
	for i := 0; i < list.Len(); i++ {
		c, _ := list.Index(i).Interface().(tea.Cmd)
		out = append(out, cmdLeaves(c)...)
	}
	return out
}

// cmdCount is how many commands cmd carries.
func cmdCount(cmd tea.Cmd) int { return len(cmdLeaves(cmd)) }

// quits reports whether cmd, or any command batched in it, is tea.Quit.
func quits(cmd tea.Cmd) bool {
	for _, c := range cmdLeaves(cmd) {
		if reflect.ValueOf(c).Pointer() == reflect.ValueOf(tea.Quit).Pointer() {
			return true
		}
	}
	return false
}
