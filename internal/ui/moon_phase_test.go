package ui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// Slice P of specs/moon-observatory.md: p/P jump to a Moon phase on every tab.

// moonPhaseFixture is the 2026 void fixture read as lookup reads it.
func moonPhaseFixture(t *testing.T) astrolog.VoidResult {
	t.Helper()
	prev := astrolog.CurrentClock()
	astrolog.SetClock(astrolog.Clock{Hours: astrolog.Hours12, Dates: astrolog.MonthFirst})
	b, err := os.ReadFile("../lookup/testdata/voids-2026.txt")
	if err != nil {
		t.Fatal(err)
	}
	res, err := astrolog.ReadVoids(string(b), 3600, 2026, fixtureRegistry(), testAspects())
	astrolog.SetClock(prev)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// phaseModel is the Moon tab attached, Now at 2026-11-03 05:00 Amsterdam, and
// (remember) Now's window from the fixture in void memory.
func phaseModel(t *testing.T, remember bool) Model {
	t.Helper()
	m := onList(t, listMoon, Config{Accessible: true})
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 3, 5, 0, 0, 0, loc)
	m.slots.SetFloatingTime(now)
	m.targetFloatingTime = now
	if remember {
		floating := m.slots.Floating()
		_, key, ok := m.voidRequestForSlot(&floating)
		if !ok {
			t.Fatal("fixture: no void request for Now")
		}
		m.mem.voids = m.mem.voids.put(key, moonPhaseFixture(t))
	}
	return m
}

func phaseAt(t *testing.T, y int, mo time.Month, d, h, min int) time.Time {
	t.Helper()
	return time.Date(y, mo, d, h, min, 59, 0, time.FixedZone("", 3600))
}

// P1
func TestPhaseStepForward(t *testing.T) {
	stubTextCompute(t)
	m := phaseModel(t, true)
	m, _ = tpress(t, m, runes("p"))
	if want := phaseAt(t, 2026, time.November, 9, 8, 2); !m.targetFloatingTime.Equal(want) {
		t.Errorf("p: Now's target %v, want the New Moon's landing %v", m.targetFloatingTime, want)
	}
	if !m.floatingDetached {
		t.Error("p: Now should follow the target, detached from the clock")
	}
}

// P2
func TestPhaseStepBack(t *testing.T) {
	stubTextCompute(t)
	m := phaseModel(t, true)
	m, _ = tpress(t, m, runes("p"))
	landing := m.targetFloatingTime
	var before time.Time
	for _, p := range moonPhaseFixture(t).Phases {
		if at := phaseLanding(p); at.Before(landing) && at.After(before) {
			before = at
		}
	}
	if before.IsZero() {
		t.Fatal("fixture: no phase before the New Moon")
	}
	m, _ = tpress(t, m, runes("P"))
	if !m.targetFloatingTime.Equal(before) {
		t.Errorf("P from a landing: target %v, want the phase before it, %v", m.targetFloatingTime, before)
	}
}

// phaseViews are the places p and P must work from: Details' Moon tab
// attached, the Chart panel with the Moon tab behind it, Bodies, and the
// Aspects focus.
func phaseViews(t *testing.T) map[string]Model {
	t.Helper()
	attached := phaseModel(t, true)
	chart := attached
	chart.details.attached = false
	chart.focused = PanelChart
	bodies := chart
	bodies.focused = PanelBodies
	aspects := chart
	aspects.focused = PanelAspects
	return map[string]Model{"Details Moon tab": attached, "Chart": chart, "Bodies": bodies, "Aspects": aspects}
}

// P1b: the jump is the same on every tab, and it is the jump function's result.
func TestPhaseKeysJumpOnEveryTab(t *testing.T) {
	stubTextCompute(t)
	for name, m := range phaseViews(t) {
		next, ok := m.phaseTarget(+1)
		if !ok {
			t.Fatalf("%s: fixture has no next phase", name)
		}
		got, _ := tpress(t, m, runes("p"))
		if !got.targetFloatingTime.Equal(next) {
			t.Errorf("%s: p put Now at %v, want the jump function's %v", name, got.targetFloatingTime, next)
		}
		prev, ok := m.phaseTarget(-1)
		if !ok {
			t.Fatalf("%s: fixture has no previous phase", name)
		}
		got, _ = tpress(t, m, runes("P"))
		if !got.targetFloatingTime.Equal(prev) {
			t.Errorf("%s: P put Now at %v, want the jump function's %v", name, got.targetFloatingTime, prev)
		}
	}
}

// P3
func TestPhaseKeyIsTitled(t *testing.T) {
	stubTextCompute(t)
	titles := func(m Model) (bar, help string) {
		bar = keys.BarLine(m.keysContext(), 300)
		for _, l := range keybindingLinesOf(m, ownerMain) {
			if strings.Contains(l.Text, "Moon phase") || strings.Contains(l.Text, "Step a month") {
				help += l.Text + "\n"
			}
		}
		return bar, help
	}
	views := phaseViews(t)
	timeline := onList(t, listTimeline, Config{Accessible: true})
	views["Details Timeline tab"] = timeline
	views["Details report"] = phaseModel(t, true).withDetailsTab(PanelChart, TabDetailsReport)
	for name, m := range views {
		bar, help := titles(m)
		if !strings.Contains(bar, "Moon phase: p/P") || !strings.Contains(help, "Next Moon phase") || !strings.Contains(help, "Previous Moon phase") || !strings.Contains(help, "Step a month forward") || !strings.Contains(help, "Step a month back") {
			t.Errorf("%s: bar %q, help %q, want Moon phase p/P, the two Moon phase rows and the two month rows", name, bar, help)
		}
	}
}

// P4: o is the month on the Moon tab too.
func TestMonthStepOnEveryTab(t *testing.T) {
	stubTextCompute(t)
	for name, m := range phaseViews(t) {
		start := m.targetFloatingTime
		want := advanceTimeForPlace(start, m.slots.Floating().Location, 0, 1, 0, 0)
		got, _ := tpress(t, m, runes("o"))
		if !got.targetFloatingTime.Equal(want) {
			t.Errorf("%s: o put Now at %v, want a month on, %v", name, got.targetFloatingTime, want)
		}
		got, _ = tpress(t, m, runes("m"))
		if d := got.targetFloatingTime.Sub(start); d != time.Minute {
			t.Errorf("%s: m moved Now by %v, want a minute", name, d)
		}
	}
}

// P5
func TestPhaseStepWithoutAvailablePhase(t *testing.T) {
	calls := stubTextCompute(t)
	for _, key := range []string{"p", "P"} {
		m := phaseModel(t, false)
		start, logged, spawned := m.targetFloatingTime, len(m.logLines), len(*calls)
		m, cmd := tpress(t, m, runes(key))
		keyLogged := len(m.logLines)
		m = pump(t, m, cmd)
		if !m.targetFloatingTime.Equal(start) || keyLogged != logged || len(*calls) != spawned {
			t.Errorf("%s with no window in memory: target %v (was %v), log %d lines (was %d), %d computes spawned", key, m.targetFloatingTime, start, keyLogged, logged, len(*calls)-spawned)
		}
	}
}

// TestMoonPhaseLabelSurvivesANarrowBar: a bar too narrow for every entry drops
// the others first and keeps the phase key (operator, 2026-11-12: "Never drop
// it").
func TestMoonPhaseLabelSurvivesANarrowBar(t *testing.T) {
	stubTextCompute(t)
	for name, m := range phaseViews(t) {
		// The phase key gives way last: at 80 columns it still stands beside the
		// pinned Commands, Keybindings and Quit.
		for _, w := range []int{80, 100, 132, 144} {
			if bar := keys.BarLine(m.keysContext(), w); !strings.Contains(bar, "Moon phase: p/P") {
				t.Errorf("%s at %d columns: bar %q has no Moon phase", name, w, bar)
			}
		}
	}
}
