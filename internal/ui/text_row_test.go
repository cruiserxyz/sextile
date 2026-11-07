package ui

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// refresh-descriptor-text: the Text surface's refresh policy is one row
// value, and the coordinator reaches it only by asking
// (specs/refresh-descriptor-text.md).

// textRunLog dumps the Log of one cast: "ok" lands, "fail" is refused, "tick"
// is the minute tick's frame (it writes the now line, no argv line).
func textRunLog(t *testing.T, mode string) []string {
	t.Helper()
	var m Model
	switch mode {
	case "tick":
		clock := &fakeClock{t: tickStart()}
		m, _ = liveModel(t, clock)
		clock.advanceTo(time.Date(2026, 4, 25, 12, 1, 0, 0, time.UTC))
		m, _ = deliverTick(m)
		m = commit(t, m, frameFor(m, clock.Now(), "/bin/astrolog setup", "/bin/astrolog tick-1"))
	default:
		m = textModel(t)
		m, _ = m.advanceTime(unitDay, +1)
		st := m.refresh[surfaceText]
		msg := FirstFrameReadyMsg{RunID: st.runningRunID, WantID: st.wantID, Commands: []string{"/bin/astrolog one", "/bin/astrolog two"}}
		if mode == "fail" {
			msg.Err = errors.New("astrolog failed")
		}
		m = commit(t, m, msg)
	}
	var out []string
	for _, l := range m.logLines {
		out = append(out, l.text)
	}
	return out
}

// Acceptance 3. The Log of a cast that lands, one that is refused and a
// minute-tick cast. The literals were captured at play eac41e625002, before
// the row existed.
func TestTextRunLogTranscript(t *testing.T) {
	cases := []struct {
		mode string
		want []string
	}{
		{"ok", []string{"/bin/astrolog one", "/bin/astrolog two"}},
		{"fail", []string{"/bin/astrolog one", "/bin/astrolog two", "Couldn't cast the chart. Press ,l l for what Astrolog ran."}},
		{"tick", []string{"/bin/astrolog tick-1"}},
	}
	for _, c := range cases {
		got := textRunLog(t, c.mode)
		if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("%s: Log %q, want %q", c.mode, got, c.want)
		}
	}
}

// Acceptance 1. refresh.go holds no Text branch: the coordinator asks the
// row. rowFor alone names the surface.
func TestSurfaceTextIsOnlyReachedThroughItsRow(t *testing.T) {
	src, err := os.ReadFile("refresh.go")
	if err != nil {
		t.Fatal(err)
	}
	if got := surfaceBranches(t, "refresh.go", string(src), "surfaceText", "rowFor"); len(got) > 0 {
		t.Errorf("refresh.go branches on surfaceText outside the row (%d): %s", len(got), strings.Join(got, ", "))
	}
}

// Wrong-name control: one planted Text branch of each shape is named, rowFor
// is left alone, and a Template branch is not reported.
func TestSurfaceBranchScannerNamesAPlantedTextBranch(t *testing.T) {
	const fixture = `package ui
func rowFor(s surface) bool { return s == surfaceText }
func planted(s surface) int {
	switch s {
	case surfaceTemplate:
		return 1
	case surfaceText:
		return 2
	}
	return 0
}
func gate(s surface) bool { return (s == surfaceText || s == surfaceImage) && s != surfaceTemplate }
`
	got := surfaceBranches(t, "fixture.go", fixture, "surfaceText", "rowFor")
	if len(got) != 2 || got[0] != "planted:7" || got[1] != "gate:12" {
		t.Fatalf("scanner reported %v, want [planted:7 gate:12]", got)
	}
}

// Acceptance 5. The row's columns as the spec settles them.
func TestTextRowColumns(t *testing.T) {
	r, ok := defaultRowOf(surfaceText)
	if !ok {
		t.Fatal("Text has no row")
	}
	if r.debounce != 50*time.Millisecond || debounceOf(surfaceText) != 50*time.Millisecond {
		t.Errorf("row debounce %v, debounceFor %v, want 50ms", r.debounce, debounceOf(surfaceText))
	}
	if r.trigger != triggerEventLeading || r.supersede != supersedePolite {
		t.Errorf("trigger %v supersede %v", r.trigger, r.supersede)
	}
	if r.inputs != nil || r.suspend != nil {
		t.Error("Text has inputs or a suspend; the spec settles both as nil")
	}
	if r.rest.armed != nil || r.rest.hidden != nil || r.rest.park != nil || r.rest.disarm != nil {
		t.Error("Text has a rest timer")
	}
	if r.build == nil || r.accept == nil || r.commit == nil {
		t.Error("Text row lacks build, accept or commit")
	}
	// A nil suspend is tolerated by the generic fence.
	m := textModel(t)
	m, _ = m.advanceTime(unitDay, +1)
	m = m.suspendSurface(surfaceText)
	if m.refresh[surfaceText].cancel != nil {
		t.Error("suspendSurface left a run in flight")
	}
}

// Acceptance 4. The row's accept is the stale-error drop: a stale success is
// accepted (the burst's leading frame), a stale error is not, a current error is.
func TestTextRowAcceptDropsOnlyAStaleError(t *testing.T) {
	r, _ := defaultRowOf(surfaceText)
	m := textModel(t)
	m, _ = m.advanceTime(unitDay, +1)
	m, _ = m.advanceTime(unitDay, +1)
	want := m.refresh[surfaceText].wantID
	boom := errors.New("boom")
	cases := []struct {
		name string
		msg  refreshResultMsg
		ok   bool
	}{
		{"stale success", refreshResultMsg{surface: surfaceText, wantID: want - 1}, true},
		{"stale error", refreshResultMsg{surface: surfaceText, wantID: want - 1, err: boom}, false},
		{"current error", refreshResultMsg{surface: surfaceText, wantID: want, err: boom}, true},
		{"current success", refreshResultMsg{surface: surfaceText, wantID: want}, true},
	}
	for _, c := range cases {
		if got := r.accept(m, c.msg); got != c.ok {
			t.Errorf("%s: accept %v, want %v", c.name, got, c.ok)
		}
	}
}

// Acceptance 4. A stale success still commits as the leading frame, and the
// drop leaves lastCommitWantID behind so the pending tick still runs.
func TestTextStaleSuccessCommitsAndStaleErrorLeavesTheFloor(t *testing.T) {
	m := textModel(t)
	m, _ = m.advanceTime(unitDay, +1)
	m, _ = m.advanceTime(unitDay, +1) // want 2, run 1 in flight
	st := m.refresh[surfaceText]
	stale := FirstFrameReadyMsg{RunID: st.runningRunID, WantID: st.wantID - 1, Commands: []string{"/bin/astrolog lead"}}
	m2 := commit(t, m, stale)
	if got := m2.refresh[surfaceText].lastCommitWantID; got != st.wantID-1 {
		t.Errorf("a stale success left lastCommitWantID %d, want %d", got, st.wantID-1)
	}
	if !m2.chart.settled {
		t.Error("a stale success did not commit its frame")
	}
	stale.Err = errors.New("boom")
	m3 := commit(t, m, stale)
	if m3.refresh[surfaceText].lastCommitWantID != st.lastCommitWantID || len(m3.logLines) != len(m.logLines) {
		t.Errorf("a stale error moved the floor or wrote a line: floor %d, %d lines", m3.refresh[surfaceText].lastCommitWantID, len(m3.logLines))
	}
}

// Acceptance 5. bumpIdentity still hard-cancels and fences; noteWant(…, true)
// still starts an idle run at once.
func TestTextTriggersKeepTheirShape(t *testing.T) {
	m := textModel(t)
	m, cmd := m.noteWant(surfaceText, true)
	if cmd == nil || m.refresh[surfaceText].cancel == nil || m.refresh[surfaceText].runningRunID != 1 {
		t.Fatalf("a leading want on an idle surface did not start a run: cmd nil=%v run %d", cmd == nil, m.refresh[surfaceText].runningRunID)
	}
	m, _ = m.bumpIdentity(surfaceText)
	st := m.refresh[surfaceText]
	if st.runningRunID != 2 || st.commitFloor != st.wantID || st.cancel == nil {
		t.Errorf("bumpIdentity: run %d floor %d want %d cancel nil=%v", st.runningRunID, st.commitFloor, st.wantID, st.cancel == nil)
	}
	var tm tea.Model = m
	tm, _ = tm.Update(FirstFrameReadyMsg{RunID: 1, WantID: 1, Commands: []string{"/bin/astrolog old"}})
	if len(tm.(Model).logLines) != 0 {
		t.Error("the fenced run's result was committed")
	}
}
