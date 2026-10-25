package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// the specification rules 4 and 5: which chart's body
// stands on which side of a cross row, and which chart is the left side of the
// Astrolog call.

// sidedBody is the body a chart contributes in the sided fake: ① the Sun, ② the
// Moon, ③ Venus; anything else (now) Mars. The fake puts the LEFT chart's body
// in OverlayAspect.Natal and the RIGHT chart's in .Transit, so a rendered row
// says which chart was handed to Astrolog first.
func sidedBody(year int) aspect.Body {
	switch year {
	case 1961:
		return aspect.Sun
	case 1962:
		return aspect.Moon
	case 1963:
		return aspect.Venus
	}
	return aspect.Mars
}

type sidedCalls struct {
	mu       sync.Mutex
	rt       []store.AnchorChart    // the natal side of each -rt call
	rtAt     []store.AnchorChart    // the transit moment and place of each -rt call
	synastry [][2]store.AnchorChart // chart 1, chart 2 of each -r0 call
}

func (c *sidedCalls) snapshot() ([]store.AnchorChart, [][2]store.AnchorChart) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]store.AnchorChart(nil), c.rt...), append([][2]store.AnchorChart(nil), c.synastry...)
}

func (c *sidedCalls) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rt, c.rtAt, c.synastry = nil, nil, nil
}

// transits is the transit side (moment and place) of each -rt call, in call
// order beside snapshot's natal side.
func (c *sidedCalls) transits() []store.AnchorChart {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]store.AnchorChart(nil), c.rtAt...)
}

// stubSidedCross answers the text compute with cross rows whose glyphs name the
// two charts of the call, on top of the swap tests' column lookups.
func stubSidedCross(t *testing.T) (*sidedCalls, *columnCalls) {
	t.Helper()
	cols := stubColumnLookups(t, nil)
	calls := &sidedCalls{}
	row := func(left, right aspect.Body) lookup.OverlayAspects {
		return lookup.OverlayAspects{
			Aspects: []lookup.OverlayAspect{{Natal: left, Transit: right, Kind: aspect.Square, Orb: 0.5, Power: 5}},
			Report:  "cross",
		}
	}
	overlayCrossAspects = func(_ context.Context, _ lookup.Lens, natal store.AnchorChart, at time.Time, atPlace location.PlaceInfo) (lookup.OverlayAspects, error) {
		calls.mu.Lock()
		calls.rt = append(calls.rt, natal)
		calls.rtAt = append(calls.rtAt, store.AnchorChart{When: at, Place: atPlace})
		calls.mu.Unlock()
		return row(sidedBody(natal.When.Year()), aspect.Mars), nil
	}
	synastryCrossAspects = func(_ context.Context, _ lookup.Lens, c store.AnchorChart, when time.Time, place location.PlaceInfo, _ store.ReportLabel) (lookup.OverlayAspects, error) {
		calls.mu.Lock()
		calls.synastry = append(calls.synastry, [2]store.AnchorChart{c, {When: when, Place: place}})
		calls.mu.Unlock()
		return row(sidedBody(c.When.Year()), sidedBody(when.Year())), nil
	}
	return calls, cols
}

// crossRows are the rendered Aspects-panel rows carrying an aspect glyph.
func crossRows(t *testing.T, m Model) string {
	t.Helper()
	m.width, m.height = 132, 40
	_, panels := leftColumn(t, m.View())
	var rows []string
	for _, ln := range strings.Split(ansi.Strip(panels), "\n") {
		if strings.Contains(ln, " □ ") {
			rows = append(rows, ln)
		}
	}
	return strings.Join(rows, "\n")
}

// Rule 4: the left glyph of a cross row is the left chart's body, the base's.
// Two charts flip their sides with the cursor: ① read in ② puts ②'s Moon left
// of ①'s Sun, and landing on ②, the base, swaps it to ② read in ① with ①'s Sun
// on the left. With now selected as ③ and the cursor on it, the `③ in ②` rows
// carry ②'s body left and now's right.
func TestTwoChartsFlipTheirSides(t *testing.T) {
	_, _ = stubSidedCross(t)
	settle := func(m Model) Model {
		m.cfg = Config{Color: ColorNever}
		m.focused = PanelChart
		m, cmd := m.afterComparisonChange()
		return pump(t, m, cmd)
	}

	m := selectedSet(t, 2, false, 0)
	m.base = m.slots.All()[1] // ① read in ②
	m = settle(m)
	if rows := crossRows(t, m); !strings.Contains(rows, "☽ □ ☉") || strings.Contains(rows, "☉ □ ☽") {
		t.Fatalf("cursor on ①: the cross rows are not ②'s Moon left of ①'s Sun:\n%s", rows)
	}
	m, cmd := tpress(t, m, keyDown)
	m = pump(t, m, cmd)
	if m.cursorIdx != 1 || m.activeTabID(PanelAspects) != TabAspectsCross {
		t.Fatalf("fixture: cursor %d, active %q", m.cursorIdx, m.activeTabID(PanelAspects))
	}
	if rows := crossRows(t, m); !strings.Contains(rows, "☉ □ ☽") || strings.Contains(rows, "☽ □ ☉") {
		t.Errorf("cursor on ②: the cross rows are not ①'s Sun left of ②'s Moon:\n%s", rows)
	}

	// Now selected as ③, the cursor on now, read in ②: `③ in ②`.
	m = selectedSet(t, 2, true, 2)
	m.base = m.slots.All()[1]
	m = settle(m)
	if got := titlesOf(m.panelTabs(PanelAspects)); strings.Join(got, "|") != "Aspects|3 in 1|3 in 2" {
		t.Fatalf("fixture: tabs %v", got)
	}
	if rows := crossRows(t, m); !strings.Contains(rows, "☽ □ ♂") || strings.Contains(rows, "♂ □") || strings.Contains(rows, "☉ □") {
		t.Errorf("③ in ②: the cross rows are not ②'s Moon left of now's Mars:\n%s", rows)
	}
}

// Rule 5: a pair of two anchored charts is -r0 with the LEFT chart, the base, as
// chart 1, a pair with the floating slot is -rt with the base as the natal, and
// the house frame is the base's. Once at the compute seam (the counting
// fake), once through the real compiler (the staged chart files).
func TestPairArgvFollowsTheSides(t *testing.T) {
	// Through the real compiler: chart0.as is the LEFT chart. "Is" means the
	// staged file carries the chart lines (instant, place, zone, label) the
	// compiler writes for that slot ALONE (singleChartLines), not just its year
	// (the specification C4a).
	lines := func(idx, anchors int) string { return singleChartLines(t, idx, anchors) }
	wantLines := map[string]string{
		"② of 2": lines(1, 2), "① of 2": lines(0, 2), "now of 2": lines(2, 2),
	}
	spawns := chartFileCapturingAstrolog(t)
	m := selectedSet(t, 2, false, 1)
	m.base = m.slots.All()[0] // ② read in ①
	m, cmd := m.afterComparisonChange()
	m = pump(t, m, cmd)
	got := relationSpawn(t, spawns(), "-r0")
	if len(got.dats) != 2 || chartFileYear(got.dats[0]) != "1961" || chartFileYear(got.dats[1]) != "1962" {
		t.Errorf("-r0 staged charts = %q, want ① (1961) as chart 1 then ② (1962)", got.dats)
	} else if !strings.Contains(got.dats[0], wantLines["① of 2"]) || !strings.Contains(got.dats[1], wantLines["② of 2"]) {
		t.Errorf("-r0 staged chart lines differ from the slots' own: %q", got.dats)
	}
	spawns = chartFileCapturingAstrolog(t)
	m = selectedSet(t, 2, true, 2)
	m.base = m.slots.All()[1] // now read in ②
	m, cmd = m.afterComparisonChange()
	m = pump(t, m, cmd)
	got = relationSpawn(t, spawns(), "-rt")
	if len(got.dats) != 2 || chartFileYear(got.dats[0]) != "1962" {
		t.Errorf("-rt staged charts = %q, want ② (1962) as the natal", got.dats)
	} else if !strings.Contains(got.dats[0], wantLines["② of 2"]) || !strings.Contains(got.dats[1], wantLines["now of 2"]) {
		t.Errorf("-rt (base ②) staged chart lines are not ② then the floating slot's own: %q", got.dats)
	}
	// ① ② now, the cursor on now read in ①: one -rt, ① natal, the floating slot second.
	spawns = chartFileCapturingAstrolog(t)
	m = selectedSet(t, 2, true, 2)
	m.base = m.slots.All()[0]
	m, cmd = m.afterComparisonChange()
	m = pump(t, m, cmd)
	got = relationSpawn(t, spawns(), "-rt")
	if len(got.dats) != 2 || !strings.Contains(got.dats[0], wantLines["① of 2"]) || !strings.Contains(got.dats[1], wantLines["now of 2"]) {
		t.Errorf("-rt (base ①) staged chart lines are not ① then the floating slot's own: %q", got.dats)
	}
	// ① ② now, the cursor on ②, `]` from its own tab to `2 in 1`: -r0 with ① as chart 1, ② as chart 2.
	m = selectedSet(t, 2, true, 1)
	m, cmd = m.afterComparisonChange()
	m = pump(t, m, cmd)
	spawns = chartFileCapturingAstrolog(t)
	m.focused = PanelAspects
	m, cmd = tpress(t, m, runes("]"))
	m = pump(t, m, cmd)
	got = relationSpawn(t, spawns(), "-r0")
	if len(got.dats) != 2 || !strings.Contains(got.dats[0], wantLines["① of 2"]) || !strings.Contains(got.dats[1], wantLines["② of 2"]) {
		t.Errorf("-r0 after `]` on ② staged chart lines are not ① then ②: %q", got.dats)
	}
	calls, _ := stubSidedCross(t)
	settle := func(m Model) Model {
		m.cfg = Config{Color: ColorNever}
		m.focused = PanelChart
		calls.reset()
		m, cmd := m.afterComparisonChange()
		return pump(t, m, cmd)
	}
	chart := func(m Model, i int) store.AnchorChart { return anchorOf(m.slots.All()[i]) }

	// No now, the cursor on ② read in ①: -r0 with ① as chart 1, ② as chart 2.
	m = selectedSet(t, 3, false, 1)
	m.base = m.slots.All()[0]
	m = settle(m)
	rt, syn := calls.snapshot()
	if len(rt) != 0 || len(syn) != 1 || syn[0][0] != chart(m, 0) || syn[0][1].When != chart(m, 1).When {
		t.Errorf("no now, cursor ②: -rt %v -r0 %v, want one -r0 with ① then ②", rt, syn)
	}
	if sc := m.deriveScene(); sc.frame != m.slots.All()[0] {
		t.Errorf("no now, cursor ②: the house frame is not the base")
	}

	// Now selected, the cursor on now read in ③: -rt with ③ as the natal.
	m = selectedSet(t, 3, true, 3)
	m.base = m.slots.All()[2]
	m = settle(m)
	rt, syn = calls.snapshot()
	if len(syn) != 0 || len(rt) != 1 || rt[0] != chart(m, 2) {
		t.Errorf("now read in ③: -rt %v -r0 %v, want one -rt with ③ as the natal", rt, syn)
	}

	// Now selected, the cursor on now: the `Aspects` tab, no cross call on
	// arrival (C1 of the specification).
	m = settle(selectedSet(t, 3, true, 3))
	rt, syn = calls.snapshot()
	if len(syn) != 0 || len(rt) != 0 {
		t.Errorf("cursor on now: -rt %v -r0 %v, want no cross call on arrival", rt, syn)
	}

	// `]` from there lands on ① x now: the natal follows the tab, not the cursor.
	m.focused = PanelAspects
	calls.reset()
	m, cmd = tpress(t, m, runes("]"))
	m = pump(t, m, cmd)
	rt, syn = calls.snapshot()
	if len(syn) != 0 || len(rt) != 1 || rt[0] != chart(m, 0) {
		t.Errorf("after ]: -rt %v -r0 %v, want one -rt with ① as the natal", rt, syn)
	}

	// (b) Five anchored + now, every cursor, the arrival and every `]` step
	// through the declared tabs: the recorded call's mode and BOTH operands are
	// the scene's active pair's left and right. A compute reading the baseline
	// instead of the pair's left on a large set fails here while the tabs stay
	// right (C4b).
	for cursor := 0; cursor <= 5; cursor++ {
		m := selectedSet(t, 5, true, cursor)
		m.cfg = Config{Color: ColorNever}
		m.focused = PanelChart
		verify := func(step string, m Model) {
			t.Helper()
			sc := m.deriveScene()
			rt, syn := calls.snapshot()
			at := calls.transits()
			if sc.pair == nil {
				if len(rt)+len(syn) != 0 {
					t.Errorf("cursor %d %s: the Aspects tab made a cross call (-rt %d -r0 %d)", cursor, step, len(rt), len(syn))
				}
				return
			}
			p := *sc.pair
			if p.right.Floating {
				if len(syn) != 0 || len(rt) != 1 || rt[0] != anchorOf(p.left) || at[0].When != p.right.Time || at[0].Place != p.right.Location {
					t.Errorf("cursor %d %s: want one -rt natal %v transit %v, got -rt %v at %v -r0 %v", cursor, step, anchorOf(p.left), p.right.Time, rt, at, syn)
				}
				return
			}
			if len(rt) != 0 || len(syn) != 1 || syn[0][0] != anchorOf(p.left) || syn[0][1].When != p.right.Time || syn[0][1].Place != p.right.Location {
				t.Errorf("cursor %d %s: want one -r0 chart 1 %v chart 2 %v, got -rt %v -r0 %v", cursor, step, anchorOf(p.left), p.right.Time, rt, syn)
			}
		}
		calls.reset()
		m, cmd = m.afterComparisonChange()
		m = pump(t, m, cmd)
		verify("arrival", m)
		m.focused = PanelAspects
		for i := range m.panelTabs(PanelAspects) {
			calls.reset()
			m, cmd = tpress(t, m, runes("]"))
			m = pump(t, m, cmd)
			verify(fmt.Sprintf("] #%d", i+1), m)
		}
	}
}

// singleChartLines is the chart lines (`-qj`, `-zl`, `-zZ`, `-zi`) the real
// compiler writes for list slot idx ALONE: the argv of the single-chart call
// with the cursor on it, spelled the way a staged file spells them. The call is
// told apart from the others a run makes by the slot's year, and every
// matching call must agree.
func singleChartLines(t *testing.T, idx, anchors int) string {
	t.Helper()
	spawns := chartFileCapturingAstrolog(t)
	m := listModel(t, anchors, idx)
	m.base = nil
	m, cmd := m.afterComparisonChange()
	pump(t, m, cmd)
	year := strconv.Itoa(m.slots.All()[idx].Time.UTC().Year())
	var want string
	for _, s := range spawns() {
		a := s.argv
		if len(a) < 12 || a[0] != "-qj" || a[2] != "-zl" || a[5] != "-zZ" || a[8] != "-zi" || qjYear(a[1]) != year {
			continue
		}
		got := fmt.Sprintf("-qj %s\n-zl %s %s\n-zZ %s %s\n-zi %q %q", a[1], a[3], a[4], a[6], a[7], a[9], a[10])
		if want != "" && got != want {
			t.Fatalf("slot %d: two single-chart calls disagree: %q vs %q", idx, want, got)
		}
		want = got
	}
	if want == "" {
		t.Fatalf("slot %d: no single-chart call found", idx)
	}
	return want
}

// Frames of the pair tabs: with ① ② now selected the cursor's chart is the
// right side of every pair and a base the left; now is never a base (`3 in 1 -
// [3 in 2]` on now, `[1 in 2]` on ①, `[Aspects] - 3 in 1 - 3 in 2` on now's own
// tab), and two charts flip (① ② with now not selected, the cursor on ②).
func TestAspectsPairsFrames(t *testing.T) {
	for _, tc := range []struct {
		name      string
		now       bool
		cursor    int // list index of the cursor
		synastry  bool
		aspects   []lookup.OverlayAspect
		wantTitle string
		own       []aspect.Aspect // the cursor's own list, for the `Aspects` tab
		base      int             // list index of the base, -1 for the chart's own tab
	}{
		// The cursor on now, read in ②: now on the right.
		{"aspects-pairs-now-right-156x40-never", true, 2, false, []lookup.OverlayAspect{
			{Natal: aspect.Midheaven, Transit: aspect.Venus, Kind: aspect.Trine, Orb: 0, Power: 9, Applying: true},
			{Natal: aspect.Mercury, Transit: aspect.Jupiter, Kind: aspect.Conjunction, Orb: 1.2003, Power: 6},
		}, "[3] Aspects - 3 in 1 - [3 in 2]", nil, 1},
		// The cursor on ①, read in ②: a synastry, ②'s body on the left.
		{"aspects-pairs-cursor-one-156x40-never", true, 0, true, []lookup.OverlayAspect{
			{Natal: aspect.Midheaven, Transit: aspect.Venus, Kind: aspect.Trine, Orb: 0, Power: 9},
			{Natal: aspect.Mercury, Transit: aspect.Jupiter, Kind: aspect.Conjunction, Orb: 1.2003, Power: 6},
		}, "[3] Aspects - [1 in 2]", nil, 1},
		// V1 of the specification: the cursor on now, the
		// `Aspects` tab, now's own list.
		{"aspects-pairs-cursor-now-156x40-never", true, 2, false, []lookup.OverlayAspect{
			{Natal: aspect.Midheaven, Transit: aspect.Venus, Kind: aspect.Trine, Orb: 0, Power: 9, Applying: true},
			{Natal: aspect.Mercury, Transit: aspect.Jupiter, Kind: aspect.Conjunction, Orb: 1.2003, Power: 6},
		}, "[3] [Aspects] - 3 in 1 - 3 in 2", []aspect.Aspect{
			{A: aspect.Sun, B: aspect.Moon, Kind: aspect.Square, Orb: 0.5, Power: 9},
			{A: aspect.Moon, B: aspect.Mars, Kind: aspect.Trine, Orb: 1.25, Power: 7},
		}, -1},
		{"aspects-pairs-two-flip-144x40-never", false, 1, true, []lookup.OverlayAspect{
			// The pair `1 in 2` showed as ☽ □ ☉ (②'s Moon, ①'s Sun); seen as
			// `2 in 1` the same aspects put ①'s body on the left (rule 4).
			{Natal: aspect.Sun, Transit: aspect.Moon, Kind: aspect.Square, Orb: 0.5, Power: 9},
			{Natal: aspect.Venus, Transit: aspect.Jupiter, Kind: aspect.Trine, Orb: 1.3837, Power: 6},
		}, "[3] Aspects - [2 in 1]", nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forceColor(t)
			m := selectedSet(t, 2, tc.now, tc.cursor) // ① ②, then now or not
			m.cfg = Config{Color: ColorNever}
			if tc.base >= 0 {
				m.base = m.slots.All()[tc.base]
			}
			m, _ = applyTextCommit(m, FirstFrameReadyMsg{
				Overlay:       true,
				Positions:     fullChartPositions(),
				Aspects:       tc.own,
				CrossSynastry: tc.synastry,
				CrossAspects:  tc.aspects,
			})
			m = withSceneColumns(m)
			m.width, m.height = dualColumnMinWidth(m.shownCount()), 40
			m.focused = PanelAspects
			if line := ansi.Strip(aspectsTopBorder(t, m)); !strings.Contains(line, tc.wantTitle) {
				t.Fatalf("the strip is %q, want %q", line, tc.wantTitle)
			}
			writeDesignFrame(t, tc.name, m.View())
		})
	}
}

// the specification C3, by the real keys. ① ② now selected, colours off, the
// sided counting fake: the strip the operator sees says which pair is active,
// and the one cross compute per step names the active pair's LEFT chart, the
// base, as chart 1 or the natal. A hand-set cursorIdx would miss
// afterFocusChange, where the arrival is noted. The tab starts on the chart
// alone, `]` chooses the base, and the base is remembered as the cursor moves.
func TestAspectsStripFollowsTheCursorKeys(t *testing.T) {
	calls, _ := stubSidedCross(t)
	m := selectedSet(t, 2, true, 0)
	m.cfg = Config{Color: ColorNever}
	m.focused = PanelChart
	m, cmd := m.afterComparisonChange()
	m = pump(t, m, cmd)
	all := m.slots.All()

	strip := func(m Model) string {
		m.focused = PanelAspects // for the render only
		return ansi.Strip(aspectsTopBorder(t, m))
	}
	// check asserts the settled frame: the strip, the active tab id, the rows,
	// and that exactly one cross compute ran, of the wanted mode.
	check := func(step string, m Model, wantStrip string, wantActive TabID, wantRows string, left, right *store.Slot, wantSynastry bool) {
		t.Helper()
		if line := strip(m); !strings.Contains(line, wantStrip) {
			t.Errorf("%s: the strip is %q, want %q", step, line, wantStrip)
		}
		if got := m.activeTabID(PanelAspects); got != wantActive {
			t.Errorf("%s: active tab %q, want %q", step, got, wantActive)
		}
		rt, syn := calls.snapshot()
		at := calls.transits()
		if left == nil {
			// The `Aspects` tab (C1 of the specification): the
			// cursor's own list, no cross rows, and no cross compute at all.
			if rows := crossRows(t, m); strings.Contains(rows, wantRows) {
				t.Errorf("%s: the `Aspects` tab shows cross rows %q:\n%s", step, wantRows, rows)
			}
			if len(rt) != 0 || len(syn) != 0 {
				t.Errorf("%s: -rt %v -r0 %v, want no cross compute for the `Aspects` tab", step, rt, syn)
			}
			return
		}
		if rows := crossRows(t, m); !strings.Contains(rows, wantRows) {
			t.Errorf("%s: the cross rows lack %q:\n%s", step, wantRows, rows)
		}
		if wantSynastry {
			if len(rt) != 0 || len(syn) != 1 || syn[0][0] != anchorOf(left) || syn[0][1].When != right.Time || syn[0][1].Place != right.Location {
				t.Errorf("%s: -rt %v -r0 %v, want one -r0 with the left chart as chart 1 and the right as chart 2", step, rt, syn)
			}
			return
		}
		if len(syn) != 0 || len(rt) != 1 || rt[0] != anchorOf(left) || at[0].When != right.Time || at[0].Place != right.Location {
			t.Errorf("%s: -rt %v at %v -r0 %v, want one -rt with the left chart as the natal and the right as the transit", step, rt, at, syn)
		}
	}
	step := func(m Model, k tea.KeyMsg) Model {
		t.Helper()
		calls.reset()
		m, cmd := tpress(t, m, k)
		return pump(t, m, cmd)
	}

	// Arrival on ①: its own tab, no cross compute; now is never a base.
	calls.reset()
	m, cmd = m.afterComparisonChange()
	m = pump(t, m, cmd)
	check("cursor ①", m, "[3] [Aspects] - 1 in 2", TabAspects, "☽ □ ☉", nil, nil, false)

	// `]` from the own tab reaches `1 in 2`: one -r0 with ② as chart 1.
	m = step(m, runes("3"))
	m = step(m, runes("]"))
	check("] on ①", m, "[3] Aspects - [1 in 2]", TabAspectsCross, "☽ □ ☉", all[1], all[0], true)
	m = step(m, runes("1"))

	// Down onto ②, the base: the chart you came from, ①, becomes the base.
	m = step(m, keyDown)
	check("Down to ②", m, "[3] Aspects - [2 in 1]", TabAspectsCross, "☉ □ ☽", all[0], all[1], true)

	// Down to now: ① stays the base, one -rt with ① as the natal.
	m = step(m, keyDown)
	check("Down to now", m, "[3] Aspects - [3 in 1] - 3 in 2", TabAspectsCross, "☉ □ ♂", all[0], all[2], false)

	// `]` on now picks `3 in 2`: the natal follows the tab.
	m = step(m, runes("3"))
	m = step(m, runes("]"))
	check("] on now", m, "[3] Aspects - 3 in 1 - [3 in 2]", aspectsPairTabID(2, 3), "☽ □ ♂", all[1], all[2], false)
	m = step(m, runes("]"))
	check("]] on now", m, "[3] [Aspects] - 3 in 1 - 3 in 2", TabAspects, "☉ □ ♂", nil, nil, false)
	m = step(m, runes("1"))

	// The own tab was chosen, so it follows the cursor up to ②.
	m = step(m, keyUp)
	check("Up to ②", m, "[3] [Aspects] - 2 in 1", TabAspects, "☉ □ ☽", nil, nil, false)

	// `]` on ② reaches `2 in 1`: one -r0, ① as chart 1 then ②.
	m = step(m, runes("3"))
	if m.focused != PanelAspects {
		t.Fatalf("`3` did not focus Aspects (focused %v)", m.focused)
	}
	m = step(m, runes("]"))
	check("] on ②", m, "[3] Aspects - [2 in 1]", TabAspectsCross, "☉ □ ☽", all[0], all[1], true)
	if m.cursorIdx != 1 {
		t.Errorf("`]` moved the slot cursor to %d", m.cursorIdx)
	}

	// Up onto ①, the base: ②, the chart you came from, becomes the base.
	m = step(m, runes("1"))
	m = step(m, keyUp)
	check("Up to ① the base", m, "[3] Aspects - [1 in 2]", TabAspectsCross, "☽ □ ☉", all[1], all[0], true)
}
