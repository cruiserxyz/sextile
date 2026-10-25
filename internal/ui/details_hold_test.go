package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Try feedback, 2026-10-23: a held navigation key made Details flicker
// between faint and bright, because each new report committed between two
// repeats and the next repeat dimmed it again. Details stays faint until the
// key is let go.

// A repeat whose report has already committed still leaves Details faint
// while the hold lasts; only the newest quiet timer ends it.
func TestHeldKeyKeepsDetailsFaintAfterTheCommit(t *testing.T) {
	m, clock := heldFixture(t)
	m = pressAt(t, m, clock, 0, runes("d"))
	m = pressAt(t, m, clock, 40*time.Millisecond, runes("d"))
	if m.details.pending {
		t.Fatal("fixture: the report did not commit")
	}
	if !m.detailsHold.on || !isDimmed(m) {
		t.Fatalf("a committed report inside the hold: held=%v dimmed=%v, want faint until the key is let go", m.detailsHold.on, isDimmed(m))
	}
	m = m.releaseDetailsHold(detailsHoldMsg{id: m.detailsHold.id})
	if isDimmed(m) {
		t.Fatalf("the newest quiet timer did not end the hold:\n%s", detailsPanelText(m))
	}
}

// With colours off the hold shows as the title's ` …`.
func TestHeldKeyMarksTheTitleWithoutColour(t *testing.T) {
	reportSpawns(t)
	m, _ := enteredFixture(t, 2, true, Config{Accessible: true})
	m, cmd := tpress(t, m, runes("0"))
	m = pump(t, m, cmd)
	clock := &fakeClock{t: tickStart()}
	m.clock = clock
	m = pressAt(t, m, clock, 0, runes("d"))
	m = pressAt(t, m, clock, 40*time.Millisecond, runes("d"))
	if m.details.pending {
		t.Fatal("fixture: the report did not commit")
	}
	if got := stripTitle(m); got[len(got)-len(" …"):] != " …" {
		t.Fatalf("held, colours off: title %q, want it to end in ` …`", got)
	}
	m = m.releaseDetailsHold(detailsHoldMsg{id: m.detailsHold.id})
	if got := stripTitle(m); len(got) >= len(" …") && got[len(got)-len(" …"):] == " …" {
		t.Fatalf("released: title %q still ends in ` …`", got)
	}
}

// The hold changes only the look: once the timeline's list has committed,
// its row keys work during the hold.
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
	if !m.detailsHold.on {
		t.Fatal("a held month step on the timeline tab did not start the hold")
	}
	if m.detailsRowKeysBlocked() {
		t.Error("the hold blocked the committed timeline's row keys")
	}
}

// A key that leaves nothing to await starts no hold.
func TestKeyWithNothingToAwaitStartsNoHold(t *testing.T) {
	reportSpawns(t)
	m, _ := enteredFixture(t, 2, true, Config{Accessible: true})
	m, cmd := tpress(t, m, runes("0"))
	m = pump(t, m, cmd)
	m = m.releaseDetailsHold(detailsHoldMsg{id: m.detailsHold.id})
	m, _ = tpress(t, m, runes("?"))
	if m.detailsHold.on {
		t.Error("opening help started the Details hold")
	}
}

// Try feedback, 2026-10-24: a single tap greyed Details for the whole quiet
// window, which read as lag. A tap never greys; the second key of a run
// starts the hold, and the run's measured repeat gap ends it.

// heldFixture is the settled report tab with colours on and a fake clock the
// test moves between keys.
func heldFixture(t *testing.T) (Model, *fakeClock) {
	t.Helper()
	forceColor(t)
	reportSpawns(t)
	m, _ := enteredFixture(t, 2, true, Config{Color: ColorAlways})
	m, cmd := tpress(t, m, runes("0")) // timeline -> report
	m = pump(t, m, cmd)
	clock := &fakeClock{t: tickStart()}
	m.clock = clock
	if m.activeTabID(PanelDetails) != TabDetailsReport || isDimmed(m) {
		t.Fatalf("fixture: tab=%s dimmed=%v, want the settled report", m.activeTabID(PanelDetails), isDimmed(m))
	}
	return m, clock
}

// pressAt delivers k with the clock d after its last reading.
func pressAt(t *testing.T, m Model, clock *fakeClock, d time.Duration, k tea.KeyMsg) Model {
	t.Helper()
	clock.advanceTo(clock.t.Add(d))
	m, cmd := tpress(t, m, k)
	return pumpHeld(t, m, cmd)
}

func TestTapNeverGreysDetails(t *testing.T) {
	m, clock := heldFixture(t)
	before := m.details.body
	m, cmd := tpress(t, m, runes("d"))
	// The scene swap shows the previous frame until the chart text commits;
	// the report is still owed after that, and must not draw faint either.
	if !m.details.pending {
		t.Fatal("fixture: a day step left no report owed")
	}
	if isDimmed(m) || m.detailsTabDimmed(TabDetailsReport) || m.detailsHold.on {
		t.Fatalf("a tap, before its report commits: dimmed=%v/%v held=%v, want the previous report as drawn", isDimmed(m), m.detailsTabDimmed(TabDetailsReport), m.detailsHold.on)
	}
	m = pumpHeld(t, m, cmd)
	if m.details.pending || m.details.body == before {
		t.Fatalf("a tap: pending=%v, want its report painted at once", m.details.pending)
	}
	if isDimmed(m) || m.detailsHold.on {
		t.Fatalf("a tap, after its report committed: dimmed=%v held=%v", isDimmed(m), m.detailsHold.on)
	}
	// The same key again after the run window is another tap.
	m = pressAt(t, m, clock, holdStartWindow+time.Millisecond, runes("d"))
	if isDimmed(m) || m.detailsHold.on {
		t.Fatalf("a second tap after %v: dimmed=%v held=%v", holdStartWindow, isDimmed(m), m.detailsHold.on)
	}
}

func TestHoldGreysFromTheFirstRepeat(t *testing.T) {
	m, clock := heldFixture(t)
	m = pressAt(t, m, clock, 0, runes("d"))
	m = pressAt(t, m, clock, 500*time.Millisecond, runes("d"))
	if !m.detailsHold.on || !isDimmed(m) {
		t.Fatalf("the first repeat, 500 ms on: held=%v dimmed=%v, want faint", m.detailsHold.on, isDimmed(m))
	}
	first := m.detailsHold.id
	m = pressAt(t, m, clock, 33*time.Millisecond, runes("d"))
	m = m.releaseDetailsHold(detailsHoldMsg{id: first})
	if !isDimmed(m) {
		t.Fatal("an older quiet timer ended the hold while the key still repeated")
	}
}

func TestHoldEndsAfterTheMeasuredGap(t *testing.T) {
	m, clock := heldFixture(t)
	m = pressAt(t, m, clock, 0, runes("d"))
	m = pressAt(t, m, clock, 500*time.Millisecond, runes("d"))
	if got := m.detailsHold.releaseQuiet(); got != 150*time.Millisecond {
		t.Fatalf("before a repeat gap is measured: quiet=%v, want 150ms", got)
	}
	for range 6 {
		m = pressAt(t, m, clock, 33*time.Millisecond, runes("d"))
	}
	if got := m.detailsHold.releaseQuiet(); got < 90*time.Millisecond || got > 110*time.Millisecond {
		t.Fatalf("repeats every 33 ms: quiet=%v, want about 100ms", got)
	}
	m = m.releaseDetailsHold(detailsHoldMsg{id: m.detailsHold.id})
	if isDimmed(m) {
		t.Fatalf("the quiet timer did not end the hold:\n%s", detailsPanelText(m))
	}
}

func TestOtherKeyEndsTheRun(t *testing.T) {
	m, clock := heldFixture(t)
	m = pressAt(t, m, clock, 0, runes("d"))
	m = pressAt(t, m, clock, 500*time.Millisecond, runes("d"))
	if !isDimmed(m) {
		t.Fatal("fixture: the run did not start the hold")
	}
	m = pressAt(t, m, clock, 33*time.Millisecond, runes("h"))
	if m.detailsHold.on || isDimmed(m) {
		t.Fatalf("another key inside a run: held=%v dimmed=%v, want a tap", m.detailsHold.on, isDimmed(m))
	}
	m = pressAt(t, m, clock, 33*time.Millisecond, runes("h"))
	if !m.detailsHold.on || !isDimmed(m) {
		t.Fatalf("that key again: held=%v dimmed=%v, want its own run", m.detailsHold.on, isDimmed(m))
	}
}
