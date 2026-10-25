package ui

import (
	"os/exec"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// roundDeadline is when a pump round stops waiting for its commands: after
// wait, once every command of the round still running is a sleeping long timer
// (longTimer: the leader key's timeout, the copy flash), or after patientWait.
// unfinished reports how many of the round's commands have not returned.
//
// A command still running at the bare deadline used to be taken for a timer and
// abandoned. On a busy host (another session's build, a receipt beside it) real
// work — a live Astrolog call, or a re-check running in the test itself —
// outlives 400 ms, was abandoned, and the test saw the old screen: three
// receipts failed that way on 2026-10-11 (TestDetailsReportsNameTheirCharts,
// TestRenamedChartRefreshesItsReport, TestRestoredChartFileReadFailureCastsNothing)
// while each passed alone. Work is now waited for; only the long timers pump
// means to cut off are abandoned at the bare deadline.
// pumpWait is pump's own deadline.
const pumpWait = 400 * time.Millisecond

func roundDeadline(wait time.Duration, unfinished func() int) <-chan time.Time {
	ch := make(chan time.Time, 1)
	go func() {
		start := time.Now()
		time.Sleep(wait)
		// A wait shorter than pump's own is a caller asking for a hard cut
		// ("nothing runs before the 150 ms rest"): honoured as before.
		for wait >= pumpWait && time.Since(start) < patientWait && unfinished() > int(longTimersSleeping.Load()) {
			time.Sleep(25 * time.Millisecond)
		}
		ch <- time.Now()
	}()
	return ch
}

// A command doing real work in a child process is waited for past pump's 400 ms,
// as on a loaded host; its message reaches the model.
func TestPumpAwaitsRunningChildWork(t *testing.T) {
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep binary")
	}
	m := probeModel(t)
	work := func() tea.Msg {
		_ = exec.Command(sleep, "1").Run() // a child for a second, like a slow Astrolog call
		return tea.WindowSizeMsg{Width: 101, Height: 33}
	}
	m = pump(t, m, work)
	if m.width != 101 || m.height != 33 {
		t.Fatalf("the child's work was abandoned: size %dx%d, want 101x33", m.width, m.height)
	}
}

// A long timer (longTimer) is still abandoned at pump's deadline: its
// message does not reach the model, and pump returns well before it fires.
func TestPumpStillAbandonsATimer(t *testing.T) {
	m := probeModel(t)
	start := time.Now()
	m = pump(t, m, longTimer(1500*time.Millisecond, func(time.Time) tea.Msg {
		return tea.WindowSizeMsg{Width: 99, Height: 22}
	}))
	if m.width == 99 {
		t.Fatal("pump waited out a 1.5 s timer it should abandon")
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("pump took %v over a bare timer, want about 400 ms", d)
	}
}

// Work done in the test process itself (no child, no timer) is waited for too:
// on a loaded host an in-process re-check can outlive 400 ms.
func TestPumpAwaitsSlowWorkInProcess(t *testing.T) {
	m := probeModel(t)
	work := func() tea.Msg {
		time.Sleep(time.Second) // stands in for a starved re-check
		return tea.WindowSizeMsg{Width: 77, Height: 31}
	}
	m = pump(t, m, work)
	if m.width != 77 || m.height != 31 {
		t.Fatalf("in-process work was abandoned: size %dx%d, want 77x31", m.width, m.height)
	}
}
