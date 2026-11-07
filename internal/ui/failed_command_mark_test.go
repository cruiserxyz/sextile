package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The large log marks the Astrolog command whose call failed with a `!`: red
// with colours on, the plain `! ` with colours off. The stored line stays the
// plain command, so Enter copies it without the mark.

// argvLines returns the argv lines of the buffer.
func failedArgvLines(m Model) []logLine {
	var out []logLine
	for _, l := range m.logLines {
		if l.lane == laneArgv {
			out = append(out, l)
		}
	}
	return out
}

// openLargeLog opens the console on m with colours off.
func openLargeLog(t *testing.T, m Model) Model {
	t.Helper()
	m.cfg = Config{Accessible: true}
	m, _ = press(t, m, ",", "l", "l")
	if !m.console.open {
		t.Fatal("the console did not open")
	}
	return m
}

func TestFailedCommandMarked(t *testing.T) {
	// A Details report Astrolog refuses (it wrote to stderr): the real report
	// path, so the command is the one that ran.
	astrologWarning(t, warnOne+"\n", serveChart(t))
	m := detailsTestModel(t)
	msg := runDetailsCmd(t, m)
	p, _ := msg.payload.(detailsPayload)
	if msg.err == nil || len(p.commands) != 1 {
		t.Fatalf("want a failed report with its command, got err=%v commands=%v", msg.err, p.commands)
	}
	m, _ = m.commitRefresh(surfaceDetails, msg)
	lines := failedArgvLines(m)
	if len(lines) != 1 || !lines[0].failed || lines[0].text != p.commands[0] {
		t.Fatalf("argv lines = %+v, want the report's command stored plain and failed", lines)
	}

	m = openLargeLog(t, m)
	row := ""
	for _, r := range consoleBodyLines(t, m) {
		if strings.HasPrefix(strings.TrimSpace(r), "! astrolog") {
			row = r
		}
	}
	if row == "" {
		t.Fatalf("no console row begins `! astrolog`:\n%s", strings.Join(consoleBodyLines(t, m), "\n"))
	}

	// Enter copies the plain command.
	copied, _, restore := stubYankRecording(t, nil)
	defer restore()
	for i, l := range m.console.snapshot {
		if l.lane == laneArgv {
			m.console.cursor = i
		}
	}
	_, _ = m.yankConsoleLine()
	if *copied != p.commands[0] {
		t.Errorf("Enter copied %q, want the plain command %q", *copied, p.commands[0])
	}

	// A report that succeeds is not marked.
	astrologWarning(t, "", serveChart(t))
	ok := detailsTestModel(t)
	okMsg := runDetailsCmd(t, ok)
	if okMsg.err != nil {
		t.Fatalf("the plain report failed: %v", okMsg.err)
	}
	ok, _ = ok.commitRefresh(surfaceDetails, okMsg)
	got := failedArgvLines(ok)
	if len(got) != 1 || got[0].failed {
		t.Errorf("a successful report's line = %+v, want one unmarked line", got)
	}
}

// With colours on the mark is red and the command is drawn as an unmarked
// line is, fitted to what the mark leaves.
func TestFailedCommandMarkColour(t *testing.T) {
	forceColor(t)
	cmd := "'/usr/bin/astrolog' -qb 2024 1 1 12:00 ST 1 0 -R0 'x'"
	cfg := Config{}
	plain := renderLaneLine(logLine{lane: laneArgv, text: cmd}, 60, cfg)
	marked := renderLaneLine(logLine{lane: laneArgv, text: cmd, failed: true}, 60, cfg)
	if plain != ansi.Strip(plain) {
		t.Fatalf("an unmarked line carries styling: %q", plain)
	}
	if marked == ansi.Strip(marked) || !strings.HasPrefix(ansi.Strip(marked), "! ") {
		t.Errorf("marked line = %q, want a styled `!` and a space first", marked)
	}
	if !strings.HasSuffix(marked, " "+argvDisplay(cmd, 58, cfg)) {
		t.Errorf("marked line = %q, want the command fitted to 58 cells after the mark", marked)
	}
	if w := ansi.StringWidth(marked); w > 60 {
		t.Errorf("marked line is %d cells, want at most 60", w)
	}
	now := renderLaneLine(logLine{lane: laneNow, text: cmd, failed: true}, 60, cfg)
	if !strings.HasPrefix(ansi.Strip(now), "! now: astrolog") {
		t.Errorf("marked now line = %q, want `! now: astrolog` first", ansi.Strip(now))
	}
}

func TestFailedFrameMarksItsCommand(t *testing.T) {
	cmds := []string{"cmd natal", "cmd transit", "cmd stations"}
	frame := func(f FirstFrameReadyMsg) []logLine {
		m := tabsPairModel(t, Config{Accessible: true})
		m, _ = applyTextCommit(m, f)
		return failedArgvLines(m)
	}
	marks := func(ls []logLine) []bool {
		var out []bool
		for _, l := range ls {
			out = append(out, l.failed)
		}
		return out
	}
	same := func(a, b []bool) bool {
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
	t.Run("an error frame marks its last command", func(t *testing.T) {
		got := marks(frame(FirstFrameReadyMsg{Err: errors.New("astrolog exited 1"), Commands: cmds[:2]}))
		if want := []bool{false, true}; !same(got, want) {
			t.Errorf("marks = %v, want %v", got, want)
		}
	})
	t.Run("a frame that names its failed call marks that one", func(t *testing.T) {
		// The station probe ran after the failing transit lookup.
		got := marks(frame(FirstFrameReadyMsg{Err: errors.New("astrolog exited 1"), Commands: cmds, FailedAt: []int{1}}))
		if want := []bool{false, true, false}; !same(got, want) {
			t.Errorf("marks = %v, want %v", got, want)
		}
	})
	t.Run("a failing lookup that spawned nothing marks nothing", func(t *testing.T) {
		got := marks(frame(FirstFrameReadyMsg{Err: errors.New("astrolog could not start"), Commands: cmds[:1], FailedAt: []int{}}))
		if want := []bool{false}; !same(got, want) {
			t.Errorf("marks = %v, want %v", got, want)
		}
	})
	t.Run("a frame that succeeded marks nothing", func(t *testing.T) {
		got := marks(frame(FirstFrameReadyMsg{Commands: cmds}))
		if want := []bool{false, false, false}; !same(got, want) {
			t.Errorf("marks = %v, want %v", got, want)
		}
	})
	t.Run("a failed column marks its command and the frame still paints", func(t *testing.T) {
		got := marks(frame(FirstFrameReadyMsg{Commands: cmds, FailedAt: []int{1}}))
		if want := []bool{false, true, false}; !same(got, want) {
			t.Errorf("marks = %v, want %v", got, want)
		}
	})
	t.Run("the failed call is the one frame helpers name", func(t *testing.T) {
		if got := failedAfter([]string{"a", "b"}, []string{"x"}, []string{"y"}); len(got) != 1 || got[0] != 3 {
			t.Errorf("failedAfter = %v, want [3]", got)
		}
		if got := failedAfter(nil, []string{"x"}); got == nil || len(got) != 0 {
			t.Errorf("failedAfter with no spawn = %#v, want an empty, non-nil list", got)
		}
		if got := columnFailed(4, []int{0, 2}); len(got) != 2 || got[0] != 4 || got[1] != 6 {
			t.Errorf("columnFailed = %v, want [4 6]", got)
		}
	})
}

// The other sites that write argv lines at a commit gate mark the last command
// of a result that ended in an error and no other.
func TestFailedCommandMarkedOnEverySite(t *testing.T) {
	boom := errors.New("astrolog exited 1")
	cmds := []string{"one", "two"}
	sites := []struct {
		name string
		run  func(m Model, err error) Model
	}{
		{"timeline", func(m Model, err error) Model {
			m, _ = m.commitTimeline(refreshResultMsg{surface: surfaceTimeline, err: err, payload: timelinePayload{key: m.timeline.key, commands: cmds}})
			return m
		}},
		{"template", func(m Model, err error) Model {
			m, _ = m.commitTemplate(refreshResultMsg{surface: surfaceTemplate, err: err, payload: templatePayload{key: m.tpl.key, commands: cmds}})
			return m
		}},
		{"wheel", func(m Model, err error) Model {
			m, _ = commitImageResult(m, refreshResultMsg{surface: surfaceImage, err: err, payload: imagePayload{commands: cmds}})
			return m
		}},
	}
	for _, s := range sites {
		t.Run(s.name, func(t *testing.T) {
			failed := failedArgvLines(s.run(detailsTestModel(t), boom))
			if len(failed) != 2 || failed[0].failed || !failed[1].failed {
				t.Errorf("failed result: %+v, want only the last command marked", failed)
			}
			ok := failedArgvLines(s.run(detailsTestModel(t), nil))
			if len(ok) != 2 || ok[0].failed || ok[1].failed {
				t.Errorf("successful result: %+v, want no mark", ok)
			}
		})
	}
}

// A quiet frame (the minute tick's) that failed marks its now line.
func TestFailedTickMarksItsNowLine(t *testing.T) {
	m := tabsPairModel(t, Config{Accessible: true})
	m = m.setNowLine([]string{"a", "b"}, 1)
	var now []logLine
	for _, l := range m.logLines {
		if l.lane == laneNow {
			now = append(now, l)
		}
	}
	if len(now) != 1 || !now[0].failed || now[0].text != "b" {
		t.Errorf("now lines = %+v, want one marked line holding the plain command", now)
	}
	m = m.setNowLine([]string{"a", "b"})
	for _, l := range m.logLines {
		if l.lane == laneNow && l.failed {
			t.Errorf("a later successful tick left the mark on %+v", l)
		}
	}
}
