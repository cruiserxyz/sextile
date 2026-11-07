package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Details follows its keys alone (specs/details-follows-its-tab.md C8). The
// hold of 2026-10-23/24, which turned every Details tab faint from the second
// repeat of a held key until it was let go, is gone: what a pane draws is the
// view rule (subject.go), whatever the keys before it did. These tests keep
// what is left of the old ones: the run of equal keys is still tracked
// (followKeyRun) and a tap or a held key paints by the rule.

// heldFixture is the settled report tab on the floating chart (the follow
// fixture) with colours on and a fake clock the test moves between keys: a day
// step changes the Chart report's key, so every key of a run owes a report.
func heldFixture(t *testing.T) (Model, *fakeClock) {
	t.Helper()
	forceColor(t)
	m, _ := followFixture(t, Config{Color: ColorAlways}, 132, 40)
	clock := &fakeClock{t: tickStart()}
	m.clock = clock
	if m.activeTabID(PanelDetails) != TabDetailsReport || isDimmed(m) || m.details.body == "" {
		t.Fatalf("fixture: tab=%s dimmed=%v, want the settled report", m.activeTabID(PanelDetails), isDimmed(m))
	}
	return m, clock
}

// pressAt delivers k with the clock d after its last reading and runs what it
// started.
func pressAt(t *testing.T, m Model, clock *fakeClock, d time.Duration, k tea.KeyMsg) Model {
	t.Helper()
	clock.advanceTo(clock.t.Add(d))
	m, cmd := tpress(t, m, k)
	return pump(t, m, cmd)
}

// A held key's repeats dim nothing by themselves: once the report of a repeat
// has committed, Details is bright again while the key still repeats.
func TestNoHoldDimming(t *testing.T) {
	m, clock := heldFixture(t)
	m = pressAt(t, m, clock, 0, runes("d"))
	m = pressAt(t, m, clock, 500*time.Millisecond, runes("d"))
	for range 4 {
		m = pressAt(t, m, clock, 33*time.Millisecond, runes("d"))
	}
	if m.keyRun.keys < 3 {
		t.Fatalf("fixture: the run has %d keys, want a held key", m.keyRun.keys)
	}
	if isDimmed(m) || m.detailsTabDimmed(TabDetailsReport) || strings.HasSuffix(stripTitle(m), detailsPendingMark) {
		t.Fatalf("a held key, its report committed: dimmed=%v title %q, want it bright", isDimmed(m), stripTitle(m))
	}
	if m.textPaneState(PanelBodies) != paneCurrent || m.textPaneState(PanelAspects) != paneCurrent {
		t.Fatalf("a held key, its frame committed: Bodies %v Aspects %v, want both current", m.textPaneState(PanelBodies), m.textPaneState(PanelAspects))
	}
}

// A tap draws by the view rule at every step: the previous report bright
// until the new moment's frame commits, faint with ` …` from then until its
// report lands (the same subject at another version), bright after.
func TestTapNeverGreysDetails(t *testing.T) {
	m, clock := heldFixture(t)
	clock.advanceTo(clock.t.Add(time.Second))
	m, cmd := tpress(t, m, runes("d"))
	if isDimmed(m) {
		t.Fatalf("a tap, before the new moment's frame commits: the report is already faint")
	}
	var states []paneState
	m = pumpSeen(t, m, 400*time.Millisecond, func(m Model) { states = append(states, m.detailsReportState()) }, cmd)
	if m.details.pending {
		t.Fatalf("a tap: pending=%v, want its report painted", m.details.pending)
	}
	if isDimmed(m) || m.detailsReportState() != paneCurrent {
		t.Fatalf("a tap, settled: state %v dimmed=%v, want current", m.detailsReportState(), isDimmed(m))
	}
	sawFaint := false
	for _, s := range states {
		if s == paneUpdating {
			sawFaint = true
		}
		if s == paneEmpty || s == paneFailed {
			t.Fatalf("a tap on the same subject drew %v on the way: %v", s, states)
		}
	}
	if !sawFaint {
		t.Fatalf("no step drew the report faint between the frame and the report: %v", states)
	}
	// The same key again after the run window is another tap.
	m = pressAt(t, m, clock, holdStartWindow+time.Millisecond, runes("d"))
	if isDimmed(m) {
		t.Fatalf("a second tap after %v: dimmed", holdStartWindow)
	}
	if m.keyRun.keys != 1 {
		t.Fatalf("a second tap after %v: the run has %d keys, want a new run of 1", holdStartWindow, m.keyRun.keys)
	}
}

// A repeat inside the window joins the run; the first repeat is the second
// key of the run.
func TestHoldGreysFromTheFirstRepeat(t *testing.T) {
	m, clock := heldFixture(t)
	m = pressAt(t, m, clock, 0, runes("d"))
	if m.keyRun.keys != 1 {
		t.Fatalf("the first key: the run has %d keys, want 1", m.keyRun.keys)
	}
	m = pressAt(t, m, clock, 500*time.Millisecond, runes("d"))
	if m.keyRun.keys != 2 || m.keyRun.key != "d" {
		t.Fatalf("the first repeat, 500 ms on: run %q x%d, want d x2", m.keyRun.key, m.keyRun.keys)
	}
	if isDimmed(m) {
		t.Fatal("the first repeat left Details faint after its report committed")
	}
}

// The run goes on while repeats arrive inside the window and ends at a gap
// longer than it.
func TestHoldEndsAfterTheMeasuredGap(t *testing.T) {
	m, clock := heldFixture(t)
	m = pressAt(t, m, clock, 0, runes("d"))
	m = pressAt(t, m, clock, 500*time.Millisecond, runes("d"))
	for range 6 {
		m = pressAt(t, m, clock, 33*time.Millisecond, runes("d"))
	}
	if m.keyRun.keys != 8 {
		t.Fatalf("repeats every 33 ms: the run has %d keys, want 8", m.keyRun.keys)
	}
	m = pressAt(t, m, clock, holdStartWindow+time.Millisecond, runes("d"))
	if m.keyRun.keys != 1 {
		t.Fatalf("after a gap of %v: the run has %d keys, want a new run of 1", holdStartWindow, m.keyRun.keys)
	}
}

// Another key ends the run and starts its own.
func TestOtherKeyEndsTheRun(t *testing.T) {
	m, clock := heldFixture(t)
	m = pressAt(t, m, clock, 0, runes("d"))
	m = pressAt(t, m, clock, 500*time.Millisecond, runes("d"))
	if m.keyRun.keys != 2 {
		t.Fatalf("fixture: the run has %d keys, want 2", m.keyRun.keys)
	}
	m = pressAt(t, m, clock, 33*time.Millisecond, runes("h"))
	if m.keyRun.key != "h" || m.keyRun.keys != 1 {
		t.Fatalf("another key inside a run: run %q x%d, want h x1", m.keyRun.key, m.keyRun.keys)
	}
	m = pressAt(t, m, clock, 33*time.Millisecond, runes("h"))
	if m.keyRun.key != "h" || m.keyRun.keys != 2 {
		t.Fatalf("that key again: run %q x%d, want h x2", m.keyRun.key, m.keyRun.keys)
	}
}

// A held key's report, once committed, leaves Details bright: nothing keeps it
// faint until the key is let go.
func TestHeldKeyKeepsDetailsFaintAfterTheCommit(t *testing.T) {
	m, clock := heldFixture(t)
	m = pressAt(t, m, clock, 0, runes("d"))
	m = pressAt(t, m, clock, 40*time.Millisecond, runes("d"))
	if m.details.pending {
		t.Fatal("fixture: the report did not commit")
	}
	if isDimmed(m) {
		t.Fatalf("a committed report inside a run is faint:\n%s", detailsPanelText(m))
	}
}

// With colours off the ` …` shows the same state the faint shows with colour,
// and only while the content is another version of its subject.
func TestHeldKeyMarksTheTitleWithoutColour(t *testing.T) {
	m, _ := followFixture(t, Config{Accessible: true}, 132, 40)
	clock := &fakeClock{t: tickStart()}
	m.clock = clock
	m = pressAt(t, m, clock, 0, runes("d"))
	m = pressAt(t, m, clock, 40*time.Millisecond, runes("d"))
	if m.details.pending {
		t.Fatal("fixture: the report did not commit")
	}
	if got := stripTitle(m); strings.HasSuffix(got, detailsPendingMark) {
		t.Fatalf("settled inside a run, colours off: title %q still ends in ` …`", got)
	}
	// The next repeat's frame has committed and its report has not.
	clock.advanceTo(clock.t.Add(33 * time.Millisecond))
	m, cmd := tpress(t, m, runes("d"))
	var marked bool
	m = pumpSeen(t, m, 400*time.Millisecond, func(m Model) {
		marked = marked || strings.HasSuffix(stripTitle(m), detailsPendingMark)
	}, cmd)
	if !marked {
		t.Fatal("no step between the frame and the report ended the title in ` …`")
	}
}

// The look is the view rule's only: once the timeline's list has committed,
// its row keys work during a run.
func TestHoldDoesNotBlockRowKeys(t *testing.T) {
	stubTimeline(t, monthRows(t))
	stubTextCompute(t)
	m, _ := enteredFixture(t, 2, true, Config{Accessible: true})
	clock := &fakeClock{t: tickStart()}
	m.clock = clock
	m = pressAt(t, m, clock, 0, runes("o")) // a month on, on the timeline tab
	m = pressAt(t, m, clock, 40*time.Millisecond, runes("o"))
	if m.detailsTabWaiting(TabDetailsTimeline) {
		t.Fatal("fixture: the timeline did not commit")
	}
	if m.keyRun.keys < 2 {
		t.Fatal("a held month step on the timeline tab did not make a run")
	}
	if m.detailsRowKeysBlocked() {
		t.Error("a run blocked the committed timeline's row keys")
	}
}

// A key that changes nothing Details describes starts no run.
func TestKeyWithNothingToAwaitStartsNoHold(t *testing.T) {
	reportSpawns(t)
	m, _ := enteredFixture(t, 2, true, Config{Accessible: true})
	m, cmd := tpress(t, m, runes("0"))
	m = pump(t, m, cmd)
	m, _ = tpress(t, m, runes("?"))
	if m.keyRun.keys != 0 {
		t.Error("opening help started a run")
	}
}
