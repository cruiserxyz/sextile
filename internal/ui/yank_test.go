package ui

import (
	"bytes"
	"encoding/base64"
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// stubYank replaces the clipboard seam with one that records what it was
// handed and returns err. Returns the call count, the last text, and the
// restore func — no test in this package may emit an OSC 52 escape into the
// test runner's terminal.
func stubYank(t *testing.T, err error) (*int, func()) {
	t.Helper()
	calls := 0
	prev := yankLine
	yankLine = func(string) error {
		calls++
		return err
	}
	return &calls, func() { yankLine = prev }
}

// stubYankRecording is stubYank plus the text of the last call.
func stubYankRecording(t *testing.T, err error) (*string, *int, func()) {
	t.Helper()
	calls := 0
	last := ""
	prev := yankLine
	yankLine = func(text string) error {
		calls++
		last = text
		return err
	}
	return &last, &calls, func() { yankLine = prev }
}

// C43: Enter hands the line under the cursor to the clipboard seam.
func TestConsoleEnterYanks(t *testing.T) {
	last, calls, restore := stubYankRecording(t, nil)
	defer restore()

	m := openedConsole(t, 40)
	m, _ = press(t, m, "up", "up")
	want := m.console.snapshot[m.console.cursor].text

	m, _ = press(t, m, "enter")
	if *calls != 1 {
		t.Fatalf("the seam was called %d times, want once", *calls)
	}
	if *last != want {
		t.Errorf("copied %q, want the line under the cursor %q", *last, want)
	}
}

// C43, the seam itself: what the shipped path emits really is an OSC 52
// clipboard sequence carrying the text.
func TestYankSeamEmitsOSC52(t *testing.T) {
	var buf bytes.Buffer
	if err := copyToClipboard(&buf, "hello console"); err != nil {
		t.Fatalf("copyToClipboard: %v", err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "\x1b]52;c;") {
		t.Errorf("emitted %q, want an OSC 52 clipboard sequence", got)
	}
	if want := base64.StdEncoding.EncodeToString([]byte("hello console")); !strings.Contains(got, want) {
		t.Errorf("emitted %q, want it to carry %q", got, want)
	}
}

// C44: the copied text is the stored line in full — never the rendered row.
func TestYankCopiesUntruncatedLine(t *testing.T) {
	last, _, restore := stubYankRecording(t, nil)
	defer restore()

	long := "a log line far wider than any panel could ever show: " + strings.Repeat("wide ", 40)
	m := consoleModel(t, 5)
	m = m.logCommands([]string{long})
	m, _ = press(t, m, ",", "l")
	m, _ = press(t, m, "enter")

	if *last != long {
		t.Errorf("copied %d chars, want the stored %d", len(*last), len(long))
	}
	for _, chrome := range []string{"|", "+", "\x1b"} {
		if strings.Contains(*last, chrome) {
			t.Errorf("the copied text carries chrome %q: %q", chrome, *last)
		}
	}
	// And the rendered row really is shorter, so this test is not vacuous.
	rows := consoleBodyLines(t, m)
	if len(rows) == 0 || len(rows[len(rows)-1]) >= len(long) {
		t.Error("the rendered row was not truncated, so untruncated copy proves nothing")
	}
}

// C45: the seam is one package-level variable a test can replace.
func TestYankGoesThroughSeam(t *testing.T) {
	calls, restore := stubYank(t, nil)
	defer restore()
	m := openedConsole(t, 10)
	m, _ = press(t, m, "enter")
	if *calls != 1 {
		t.Fatalf("the seam was called %d times, want once", *calls)
	}

	// Nothing in the package builds an OSC 52 sequence itself: the seam is
	// the only route, so replacing it silences the whole package.
	m, _ = press(t, m, "up")
	m, _ = press(t, m, "enter")
	if *calls != 2 {
		t.Errorf("the seam was called %d times after a second copy", *calls)
	}
}

// C46: a copy flashes a confirmation, and only its own timer clears it.
func TestYankFlashesAndClears(t *testing.T) {
	_, restore := stubYank(t, nil)
	defer restore()

	m := openedConsole(t, 20)
	m, cmd := press(t, m, "enter")
	if !m.yank.active {
		t.Fatal("a copy did not flash")
	}
	if cmd == nil {
		t.Fatal("no flash timer was armed")
	}
	if m.footerIndicator() == "" {
		t.Error("the footer shows no confirmation")
	}
	if yankFlashDuration.Milliseconds() != 1500 {
		t.Errorf("yankFlashDuration = %v, want the contract default of 1500ms", yankFlashDuration)
	}

	var tm tea.Model = m
	tm, _ = tm.Update(yankFlashMsg{gen: m.yank.gen})
	m = tm.(Model)
	if m.yank.active {
		t.Error("the flash tick did not clear the confirmation")
	}
	if got := m.footerIndicator(); got != "" {
		t.Errorf("the footer still reads %q", got)
	}
}

// The footer has one slot and two claimants. A copy, an Esc out of the
// console, and a leader armed inside yankFlashDuration must show the leader:
// it is the state the operator is in the middle of, not a receipt.
func TestPendingLeaderOutranksYankFlash(t *testing.T) {
	_, restore := stubYank(t, nil)
	defer restore()

	m := openedConsole(t, 20)
	m, _ = press(t, m, "enter")
	if !m.yank.active {
		t.Fatal("the copy did not flash")
	}
	m, _ = press(t, m, "esc")
	m, _ = press(t, m, ",")
	if !m.leader.pending {
		t.Fatal("the leader did not arm")
	}
	got := m.footerIndicator()
	if strings.Contains(got, "yank") {
		t.Errorf("the footer reads %q while a leader is pending", got)
	}
	if !strings.Contains(got, ",") {
		t.Errorf("the footer reads %q, want the pending leader", got)
	}

	// The flash tick still in flight must not resurrect anything.
	var tm tea.Model = m
	tm, _ = tm.Update(yankFlashMsg{gen: m.yank.gen})
	if got := tm.(Model).footerIndicator(); !strings.Contains(got, ",") {
		t.Errorf("a late flash tick changed the footer to %q", got)
	}
}

// C47: a tick from a flash that is already gone changes nothing.
func TestYankStaleFlashTickDropped(t *testing.T) {
	_, restore := stubYank(t, nil)
	defer restore()

	m := openedConsole(t, 20)
	m, _ = press(t, m, "enter")
	stale := m.yank.gen
	m, _ = press(t, m, "up")
	m, _ = press(t, m, "enter") // a newer flash, newer generation

	before := m
	var tm tea.Model = m
	tm, cmd := tm.Update(yankFlashMsg{gen: stale})
	if cmd != nil {
		t.Error("a stale flash tick returned a command")
	}
	if !reflect.DeepEqual(before, tm.(Model)) {
		t.Error("a stale flash tick changed the Model")
	}
	if !tm.(Model).yank.active {
		t.Error("a stale tick cleared the current flash")
	}
}

// C48: copying starts no chart compute.
func TestYankStartsNoCompute(t *testing.T) {
	_, restore := stubYank(t, nil)
	defer restore()

	m := openedConsole(t, 20)
	before := m.refresh
	m, _ = press(t, m, "enter")
	if !reflect.DeepEqual(before, m.refresh) {
		t.Error("a copy touched a refresh surface")
	}
}

// C56: a clipboard that refuses is one quiet Log line — no flash, no moved
// cursor, no changed rows.
func TestYankFailureIsQuiet(t *testing.T) {
	_, restore := stubYank(t, errors.New("no clipboard on this terminal"))
	defer restore()

	m := openedConsole(t, 40)
	m, _ = press(t, m, "up", "up")
	innerH := m.consoleInnerHeight()
	wantCursor := m.console.cursor
	wantRows := consoleBodyLines(t, m)
	wantStart, wantEnd := consoleWindow(len(m.console.snapshot), m.console.offset, innerH)
	beforeLog := len(m.logLines)

	m, _ = press(t, m, "enter")
	if m.yank.active && m.yank.notice == "" {
		t.Error("a failed copy flashed success")
	}
	if m.console.cursor != wantCursor {
		t.Errorf("the cursor moved: %d -> %d", wantCursor, m.console.cursor)
	}
	if got := consoleBodyLines(t, m); !reflect.DeepEqual(wantRows, got) {
		t.Errorf("the rendered rows changed:\n want %q\n  got %q", wantRows, got)
	}
	gotStart, gotEnd := consoleWindow(len(m.console.snapshot), m.console.offset, innerH)
	if gotStart != wantStart || gotEnd != wantEnd {
		t.Errorf("the window moved: [%d,%d) -> [%d,%d)", wantStart, wantEnd, gotStart, gotEnd)
	}
	if len(m.logLines) != beforeLog+1 {
		t.Fatalf("the failure wrote %d lines, want exactly one", len(m.logLines)-beforeLog)
	}
	// Since an earlier change the line is the sentence, not the backend's words: a copy
	// has no cause taxonomy worth switching on, and the doctrine's fallback is
	// a calm line rather than a passed-through error.
	if last := m.logLines[len(m.logLines)-1]; !strings.HasPrefix(last.text, "Couldn't copy that line") || last.lane != laneError {
		t.Errorf("the failure line does not say what happened on the error lane: %+v", last)
	}
	if got := m.footerIndicator(); got != noticeCopyFailed {
		t.Errorf("the footer shows %q after a failed copy, want the failure notice", got)
	}
}
