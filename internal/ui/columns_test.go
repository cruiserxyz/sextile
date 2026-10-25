package ui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// columnCalls records the position lookups the Bodies columns ride on.
type columnCalls struct {
	mu      sync.Mutex
	natal   []store.AnchorChart
	transit []time.Time
}

func (c *columnCalls) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.natal, c.transit = nil, nil
}

func (c *columnCalls) counts() (natal, transit int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.natal), len(c.transit)
}

// natalLongitude and transitLongitude make every chart's Sun distinct, so a
// column that shows another chart's position cannot pass.
func natalLongitude(a store.AnchorChart) float64 { return float64(a.When.Year() % 360) }
func transitLongitude(when time.Time) float64    { return float64(when.Year()%360) + 0.5 }

// stubColumnLookups answers every text compute at once with a distinct Sun per
// chart and counts the position lookups. failNatal, when set, fails the natal
// lookup of the charts it names.
func stubColumnLookups(t *testing.T, failNatal func(store.AnchorChart) bool) *columnCalls {
	t.Helper()
	calls := &columnCalls{}
	oldN, oldT, oldO, oldS := firstCallNatalPositions, firstCallTransits, overlayCrossAspects, synastryCrossAspects
	t.Cleanup(func() {
		firstCallNatalPositions, firstCallTransits, overlayCrossAspects, synastryCrossAspects = oldN, oldT, oldO, oldS
	})
	firstCallNatalPositions = func(_ context.Context, _ *lookup.Probers, c store.AnchorChart, _ lookup.Lens) (lookup.Result, error) {
		calls.mu.Lock()
		calls.natal = append(calls.natal, c)
		calls.mu.Unlock()
		if failNatal != nil && failNatal(c) {
			return lookup.Result{Commands: []string{"astrolog failing natal"}}, errors.New("astrolog: exit status 1")
		}
		return lookup.Result{
			Positions: []aspect.Position{{Body: aspect.Sun, Longitude: natalLongitude(c)}},
			Commands:  []string{"astrolog natal"},
		}, nil
	}
	firstCallTransits = func(_ context.Context, _ *lookup.Probers, when time.Time, _ location.PlaceInfo, _ lookup.Lens) (lookup.Result, error) {
		calls.mu.Lock()
		calls.transit = append(calls.transit, when)
		calls.mu.Unlock()
		return lookup.Result{
			Positions: []aspect.Position{{Body: aspect.Sun, Longitude: transitLongitude(when)}},
			Commands:  []string{"astrolog transit"},
		}, nil
	}
	overlayCrossAspects = func(context.Context, lookup.Lens, store.AnchorChart, time.Time, location.PlaceInfo) (lookup.OverlayAspects, error) {
		return lookup.OverlayAspects{Report: "transit cross"}, nil
	}
	synastryCrossAspects = func(context.Context, lookup.Lens, store.AnchorChart, time.Time, location.PlaceInfo, store.ReportLabel) (lookup.OverlayAspects, error) {
		return lookup.OverlayAspects{Report: "synastry cross"}, nil
	}
	return calls
}

// settledComparing is ① ② ③ + now selected, the cursor on cursor, its first
// comparing frame committed through the real coordinator path.
func settledComparing(t *testing.T, cursor int) Model {
	t.Helper()
	m := selectedSet(t, 3, true, cursor)
	m.cfg = Config{Color: ColorNever}
	m.focused = PanelChart
	m = m.noteArrival()
	m, cmd := m.afterComparisonChange()
	m = pump(t, m, cmd)
	if _, ok := m.bodyColumnsForScene(); !ok {
		t.Fatalf("fixture: no comparing frame committed (settled=%v, columns=%d)", m.chart.settled, len(m.chart.columns))
	}
	return m
}

// C2: each column is the position that chart's own row shows when it is
// focused alone, from the same Astrolog call.
func TestBodiesColumnEqualsThatChartAlone(t *testing.T) {
	stubColumnLookups(t, nil)
	m := settledComparing(t, 1)
	cols, _ := m.bodyColumnsForScene()
	shown := m.deriveScene().shown
	if len(cols) != len(shown) || len(shown) != 4 {
		t.Fatalf("columns = %d, shown = %d, want 4 each", len(cols), len(shown))
	}
	seen := map[float64]bool{}
	for i, slot := range shown {
		var alone FirstFrameReadyMsg
		if slot.Floating {
			alone = computeChartCmd(context.Background(), 1, 1, nil, m.targetFloatingTime, m.place, m.lens)().(FirstFrameReadyMsg)
		} else {
			alone = computeNatalSingleCmd(context.Background(), 1, 1, nil, anchorOf(slot), m.lens)().(FirstFrameReadyMsg)
		}
		if len(alone.Positions) == 0 || len(cols[i].positions) != len(alone.Positions) || cols[i].positions[0] != alone.Positions[0] {
			t.Errorf("column %d (%s): %v, want the alone frame's %v", i+1, cols[i].label, cols[i].positions, alone.Positions)
		}
		seen[alone.Positions[0].Longitude] = true
	}
	if len(seen) != 4 {
		t.Fatalf("fixture: the four charts' positions are not distinct: %v", seen)
	}
}

// C6: a time step recomputes only the floating chart, a cursor move inside the
// set recomputes no chart.
func TestColumnsRecomputeOnlyWhatMoved(t *testing.T) {
	calls := stubColumnLookups(t, nil)
	m := settledComparing(t, 1)
	if n, tr := calls.counts(); n != 3 || tr != 1 {
		t.Fatalf("fixture: the first comparing frame made %d natal and %d transit lookups, want 3 and 1", n, tr)
	}

	// (b) the cursor moves from ② to ③, inside the set: no lookup at all, yet
	// the frame follows the cursor (Bodies now carries ③'s own positions).
	calls.reset()
	m, cmd := tpress(t, m, keyDown)
	m = pump(t, m, cmd)
	want3 := natalLongitude(anchorOf(m.slots.All()[2]))
	if m.cursorIdx != 2 || len(m.chart.positions) == 0 || m.chart.positions[0].Longitude != want3 {
		t.Fatalf("cursor ② to ③: cursor %d, positions %v, want ③'s %v committed", m.cursorIdx, m.chart.positions, want3)
	}
	if n, tr := calls.counts(); n != 0 || tr != 0 {
		t.Errorf("cursor ② to ③: %d natal and %d transit lookups, want none", n, tr)
	}

	// The cursor onto the floating slot: its chart is already cast.
	calls.reset()
	m, cmd = tpress(t, m, keyDown)
	m = pump(t, m, cmd)
	if !m.focusedIsFloating() {
		t.Fatal("fixture: the cursor is not on the floating slot")
	}
	if n, tr := calls.counts(); n != 0 || tr != 0 {
		t.Errorf("cursor onto now: %d natal and %d transit lookups, want none", n, tr)
	}

	// (a) a time step: exactly the floating chart's transit.
	calls.reset()
	before := m.slots.Floating().Time
	m, cmd = tpress(t, m, runes("y"))
	m = pump(t, m, cmd)
	if !m.slots.Floating().Time.After(before) {
		t.Fatalf("fixture: the time step did not commit (floating time %v, was %v)", m.slots.Floating().Time, before)
	}
	n, tr := calls.counts()
	if n != 0 || tr != 1 {
		t.Errorf("time step: %d natal and %d transit lookups, want 0 and 1", n, tr)
	}
	cols, ok := m.bodyColumnsForScene()
	if !ok || len(cols) != 4 || cols[3].positions[0].Longitude != transitLongitude(m.slots.Floating().Time) {
		t.Errorf("after the step the floating column is %v (ok=%v), want the new moment's", cols, ok)
	}
	// The cache holds only the charts on screen.
	if got := len(m.chart.results); got != 4 {
		t.Errorf("cache holds %d charts, want the 4 shown", got)
	}
}

// C8: one failed chart leaves only its own column blank, and the Log names it.
func TestOneFailedChartLeavesItsColumnBlank(t *testing.T) {
	var third store.AnchorChart
	stubColumnLookups(t, func(c store.AnchorChart) bool { return c.When.Equal(third.When) })
	m := selectedSet(t, 3, true, 1)
	third = anchorOf(m.slots.All()[2])
	m.cfg = Config{Color: ColorNever}
	m.focused = PanelChart
	m = m.noteArrival()
	m, cmd := m.afterComparisonChange()
	m = pump(t, m, cmd)

	if !m.chart.settled {
		t.Fatal("a non-focused chart's failure failed the whole frame")
	}
	cols, ok := m.bodyColumnsForScene()
	if !ok || len(cols) != 4 {
		t.Fatalf("columns = %d (ok=%v), want 4", len(cols), ok)
	}
	for i, c := range cols {
		if i == 2 {
			if c.err == nil || len(c.positions) != 0 {
				t.Errorf("failed column ③: err %v positions %v, want an error and no positions", c.err, c.positions)
			}
			continue
		}
		if c.err != nil || len(c.positions) == 0 {
			t.Errorf("column %d: err %v positions %v, want its positions", i+1, c.err, c.positions)
		}
	}
	// Render half (C8): ③'s twelve cells are blank on every row, ① ② ④ show.
	body, _, _ := renderBodiesFor(m.bodiesView(), m.chart, m.slots, m.cursorIdx, 0, 100, 30, false, m.cfg)
	if !strings.Contains(body, "Sun") {
		t.Fatalf("the Bodies panel draws no Sun row:\n%s", body)
	}
	for _, line := range strings.Split(body, "\n") {
		if !strings.Contains(line, "Sun") {
			continue
		}
		if cell := ansi.Strip(ansi.Cut(line, 11+2*bodiesColumnWidth, 11+3*bodiesColumnWidth)); strings.Trim(cell, " []") != "" {
			t.Errorf("failed column ③ is not blank on the Sun row: %q in %q", cell, ansi.Strip(line))
		}
		for _, want := range []float64{natalLongitude(anchorOf(m.slots.All()[0])), natalLongitude(anchorOf(m.slots.All()[1])), transitLongitude(m.slots.Floating().Time)} {
			signIdx, deg, min := splitLongitude(want)
			if cell := fmt.Sprintf("%s %2d°%02d'", signAbbrevString(signIdx), deg, min); !strings.Contains(line, cell) {
				t.Errorf("Sun row lacks %q of a chart that did cast: %q", cell, line)
			}
		}
	}
	var lines []string
	for _, l := range m.logLines {
		if l.lane == laneError {
			lines = append(lines, l.text)
		}
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "Synthetic 3") {
		t.Errorf("error lines = %q, want exactly one naming Synthetic 3", lines)
	}
}

// C7 (data half): columns are only handed out for the scene they were
// computed for, until the next frame lands.
func TestBodiesNeverShowsAnotherScenesColumns(t *testing.T) {
	stubColumnLookups(t, nil)
	m := settledComparing(t, 1)
	if cols, ok := m.bodyColumnsForScene(); !ok || len(cols) != 4 {
		t.Fatalf("settled: columns = %d (ok=%v), want 4", len(cols), ok)
	}
	if msg := textComputeFollowsScene(m, m.deriveScene()); msg != "" {
		t.Fatalf("the scene projection on a settled comparing frame: %s", msg)
	}

	// Deselect a member that is not under the cursor.
	m.slots.SetSlotSelected(2, false)
	if cols, ok := m.bodyColumnsForScene(); ok {
		t.Fatalf("after deselecting ③, before its frame: still %d columns of the old set", len(cols))
	}
	// Render half (C7): the panel waits — no rows at all, not the alone rows
	// and not the old set's columns.
	if body, _, _ := renderBodiesFor(m.bodiesView(), m.chart, m.slots, m.cursorIdx, 0, 100, 30, false, m.cfg); body != "" {
		t.Fatalf("after deselecting ③, before its frame, Bodies draws %q, want its waiting state (nothing)", body)
	}
	m, cmd := m.afterComparisonChange()
	m = pump(t, m, cmd)
	if cols, ok := m.bodyColumnsForScene(); !ok || len(cols) != len(m.deriveScene().shown) || len(cols) != 3 {
		t.Fatalf("after the new frame: columns = %d (ok=%v), want 3", len(cols), ok)
	}

	// Reorder: ⌃j moves ② below the next member, the numerals swap. The panel
	// keeps the scene it last swapped to (the specification rule 8
	// reverses "paints nothing"): the old order's columns, never a mix.
	m = settledComparing(t, 1)
	settledBody, _, _ := renderBodiesFor(m.bodiesView(), m.shownChart(), m.slots, m.cursorIdx, 0, 100, 30, false, m.cfg)
	m, cmd = tpress(t, m, keyCtrlJ)
	if cols, ok := m.bodyColumnsForScene(); !ok || len(cols) != 4 {
		t.Fatalf("after ⌃j, before its frame: columns = %d (ok=%v), want the old order's 4", len(cols), ok)
	}
	if body, _, _ := renderBodiesFor(m.bodiesView(), m.shownChart(), m.slots, m.bodiesView().slot, 0, 100, 30, false, m.cfg); body != settledBody {
		t.Fatalf("after ⌃j, before its frame, Bodies draws\n%q\nwant the settled scene's\n%q", body, settledBody)
	}
	m = pump(t, m, cmd)
	if cols, ok := m.bodyColumnsForScene(); !ok || len(cols) != 4 {
		t.Fatalf("after ⌃j and its frame: columns = %d (ok=%v), want 4", len(cols), ok)
	}
}

// withSceneColumns settles a hand-built comparing fixture the way a landed
// frame would: one column per shown chart. The focused column carries the
// positions the fixture's frame committed; the others carry them shifted by a
// distinct amount each. Alone, it changes nothing. (Fixtures that commit
// a FirstFrameReadyMsg by hand carry no columns; a comparing Bodies panel
// without them is waiting by C7.)
func withSceneColumns(m Model) Model {
	sc := m.deriveScene()
	if sc.mode != sceneComparing || m.slots == nil {
		return m
	}
	shown := m.shownColumns(sc, m.slots.Floating().Time)
	cols := make([]bodyColumn, 0, len(shown))
	for i, id := range shown {
		ps := m.chart.positions
		cusps := m.chart.cusps
		if i != sc.focus {
			// Another chart's column: the same bodies moved round the zodiac by
			// an amount of its own, so no two columns read alike.
			ps = append([]aspect.Position(nil), ps...)
			for j := range ps {
				ps[j].Longitude = math.Mod(ps[j].Longitude+float64(i-sc.focus)*37.5+360, 360)
			}
			// The frame moves with the bodies, as a real chart's would.
			for j := range cusps {
				if cusps[j] != 0 {
					cusps[j] = math.Mod(cusps[j]+float64(i-sc.focus)*37.5+360, 360)
				}
			}
		}
		cols = append(cols, bodyColumn{key: id.key, label: id.label, positions: ps, cusps: cusps})
	}
	m.chart.columns, m.chart.columnsFor = cols, shown
	return m
}
