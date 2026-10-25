package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

// the specification, phase B: the selected set's numerals, the
// Aspects pair tabs, the pair compute, Details and the Bodies house frame.
// Every chart here is a constructed synthetic (listWorkspace).

// selectedSet is a ready workspace of `anchored` synthetic charts with EVERY
// anchored chart selected and the floating slot selected or not, the cursor on
// list index cursor. The selected set is therefore anchored+1 slots long with
// nowSelected, anchored without.
func selectedSet(t *testing.T, anchored int, nowSelected bool, cursor int) Model {
	t.Helper()
	m := listModel(t, anchored, cursor)
	for i := 0; i < anchored; i++ {
		m.slots.SetSlotSelected(i, true)
	}
	m.slots.SetSlotSelected(anchored, nowSelected)
	m.base = nil
	// Details needs room beside the grown left column (the specification
	// C5: the left column is 52 + 12 per extra chart shown), so the fixture
	// is as wide as its set needs.
	m.width = max(m.width, dualColumnMinWidth(len(m.slots.SelectedInOrder())))
	// The cursor's own tab is pre-selected on arrival (C1a).
	return m.noteArrival()
}

func titlesOf(tabs []Tab) []string {
	out := make([]string, len(tabs))
	for i, tab := range tabs {
		out[i] = tab.Title
	}
	return out
}

// nowCase is one selection with the floating slot in it: which slots of a
// list of `anchors` charts + now are selected (list indices; now is index
// `anchors`, always selected) and which list index holds the cursor.
type nowCase struct {
	anchors  int
	selected []int
	cursor   int
}

// wantNowStrip spells the rule out from the selection alone, never from the
// code under test: numerals are positions in the selected set; the cursor's
// chart is the right side of one pair per other ANCHORED member in numeral
// order, each a base, and the floating slot (the last member) never is one; a
// pair's tab reads `<right> in <left>` and the tab starts on `Aspects`, the
// chart alone, wherever the cursor stands (S4a A1, A13).
func wantNowStrip(tc nowCase, color bool) (titles []string, ids []TabID, active int, strip string) {
	numeral := map[int]int{}
	for pos, idx := range tc.selected {
		numeral[idx] = pos + 1
	}
	size := len(tc.selected)
	cn := numeral[tc.cursor]
	glyph := func(n int) string {
		if color {
			// Literal glyphs, never the production table (review round 1, P2:
			// an expectation read from circledNumerals passes with a wrong glyph).
			return []string{"①", "②", "③", "④", "⑤", "⑥"}[n-1]
		}
		return strconv.Itoa(n)
	}
	titles = []string{"Aspects"}
	ids = []TabID{TabAspects}
	for k := 1; k < size; k++ { // size is now, never a base
		if k == cn {
			continue
		}
		l, r := k, cn
		titles = append(titles, glyph(r)+" in "+glyph(l))
		if len(titles) == 2 {
			ids = append(ids, TabAspectsCross)
		} else {
			ids = append(ids, TabID(fmt.Sprintf("aspects-pair-%d-%d", l, r)))
		}
	}
	active = 0
	if len(titles) == 1 {
		// A one-tab strip is the plain title (an anchored chart beside now alone).
		return titles, ids, active, "[3] Aspects"
	}
	parts := append([]string(nil), titles...)
	parts[active] = "[" + parts[active] + "]"
	return titles, ids, active, "[3] " + strings.Join(parts, " - ")
}

func nowCaseModel(t *testing.T, tc nowCase) Model {
	t.Helper()
	m := listModel(t, tc.anchors, tc.cursor)
	for i := 0; i <= tc.anchors; i++ {
		m.slots.SetSlotSelected(i, false)
	}
	for _, i := range tc.selected {
		m.slots.SetSlotSelected(i, true)
	}
	m.base = nil
	m.width = max(m.width, dualColumnMinWidth(len(tc.selected)))
	return m.noteArrival()
}

// the specification C1-C3 (correcting rules 1 and 3 of
// the specification), inverted by the operator on 2026-10-24: the cursor's
// chart is the one you read, the right side of one pair per other anchored
// member in numeral order, and the floating slot is never a base: `Aspects`,
// then `c in k` for every other anchored member k; on now itself `N in k` for
// every anchored member. The tab starts on `Aspects` (S4a A1).
func TestAspectsPairsNowIsAlwaysRight(t *testing.T) {
	var cases []nowCase
	for a := 1; a <= 5; a++ {
		dense := make([]int, a+1)
		for i := range dense {
			dense[i] = i
		}
		for cursor := 0; cursor <= a; cursor++ {
			cases = append(cases, nowCase{anchors: a, selected: dense, cursor: cursor})
		}
	}
	// Sparse: five anchored charts, the first, third and fifth selected + now.
	for _, cursor := range []int{0, 2, 4, 5} {
		cases = append(cases, nowCase{anchors: 5, selected: []int{0, 2, 4, 5}, cursor: cursor})
	}
	for _, tc := range cases {
		name := fmt.Sprintf("%d anchored, selected %v, cursor %d", tc.anchors, tc.selected, tc.cursor)
		t.Run(name, func(t *testing.T) {
			for _, color := range []bool{false, true} {
				m := nowCaseModel(t, tc)
				m.cfg = Config{Color: ColorNever}
				if color {
					m.cfg = Config{Color: ColorAlways}
				}
				titles, ids, active, strip := wantNowStrip(tc, color)
				tabs := m.panelTabs(PanelAspects)
				if got := titlesOf(tabs); !reflect.DeepEqual(got, titles) {
					t.Errorf("color %v: titles = %q, want %q", color, got, titles)
				}
				if got := tabIDs(tabs); !reflect.DeepEqual(got, ids) {
					t.Errorf("color %v: ids = %v, want %v", color, got, ids)
				}
				if got := m.activeTabID(PanelAspects); got != ids[active] {
					t.Errorf("color %v: active %q, want %q", color, got, ids[active])
				}
				if color {
					continue
				}
				// The pairs by slot identity: the left side is the other
				// anchored slot, the base, the right side the cursor's slot.
				all := m.slots.All()
				sc := m.deriveScene()
				var want []scenePair
				for _, idx := range tc.selected {
					if idx == tc.cursor || idx == tc.anchors {
						continue
					}
					want = append(want, scenePair{left: all[idx], right: all[tc.cursor]})
				}
				if !reflect.DeepEqual(sc.pairs, want) {
					t.Errorf("pairs by slot identity differ from the rule")
				}
				if len(tc.selected) > 4 {
					continue
				}
				for _, size := range [][2]int{{132, 40}, {80, 24}} {
					m.width, m.height = size[0], size[1]
					m.focused = PanelAspects
					if line := ansi.Strip(aspectsTopBorder(t, m)); !strings.Contains(line, strip) {
						t.Errorf("%dx%d: the strip is %q, want it to contain %q", size[0], size[1], line, strip)
					}
				}
			}
		})
	}
}

// Rules 2 and 3 without the floating slot, as inverted on 2026-10-24: the
// cursor's chart is the right side of every pair, read in each other chart as
// the base, for two charts as for three, and the tab starts on the chart alone
// (S4a A1, A13). "One rule for any count" (operator, 2026-10-12).
func TestAspectsPairsCursorIsRight(t *testing.T) {
	for _, tc := range []struct {
		name    string
		anchors int
		cursor  int
		titles  []string
		ids     []TabID
	}{
		{"two charts, cursor on ①", 2, 0, []string{"Aspects", "1 in 2"}, []TabID{TabAspects, TabAspectsCross}},
		{"two charts, cursor on ②", 2, 1, []string{"Aspects", "2 in 1"}, []TabID{TabAspects, TabAspectsCross}},
		{"three charts, cursor on ①", 3, 0, []string{"Aspects", "1 in 2", "1 in 3"}, []TabID{TabAspects, TabAspectsCross, "aspects-pair-3-1"}},
		{"three charts, cursor on ②", 3, 1, []string{"Aspects", "2 in 1", "2 in 3"}, []TabID{TabAspects, TabAspectsCross, "aspects-pair-3-2"}},
		{"three charts, cursor on ③", 3, 2, []string{"Aspects", "3 in 1", "3 in 2"}, []TabID{TabAspects, TabAspectsCross, "aspects-pair-2-3"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := selectedSet(t, tc.anchors, false, tc.cursor)
			m.cfg = Config{Color: ColorNever}
			tabs := m.panelTabs(PanelAspects)
			if got := titlesOf(tabs); !reflect.DeepEqual(got, tc.titles) {
				t.Errorf("titles = %q, want %q", got, tc.titles)
			}
			if got := tabIDs(tabs); !reflect.DeepEqual(got, tc.ids) {
				t.Errorf("ids = %v, want %v", got, tc.ids)
			}
			if got := m.activeTabID(PanelAspects); got != TabAspects {
				t.Errorf("active %q on arrival, want the chart's own tab", got)
			}
			// `]` cycles every declared tab, `Aspects` included, and wraps.
			m.focused = PanelAspects
			for i := 0; i < len(tc.ids); i++ {
				m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
				want := tc.ids[(i+1)%len(tc.ids)]
				if got := m.activeTabID(PanelAspects); got != want {
					t.Errorf("after %d ]: active %q, want %q", i+1, got, want)
				}
			}
		})
	}
}

// pairCalls records what the stubbed Astrolog producers were asked.
type pairCalls struct {
	mu       sync.Mutex
	natal    []store.AnchorChart
	transit  int
	cross    []store.AnchorChart // overlayCrossAspects: the natal side
	synastry [][2]store.AnchorChart
}

func (c *pairCalls) snapshot() (natal []store.AnchorChart, transit int, cross []store.AnchorChart, synastry [][2]store.AnchorChart) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append(natal, c.natal...), c.transit, append(cross, c.cross...), append(synastry, c.synastry...)
}

func (c *pairCalls) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.natal, c.transit, c.cross, c.synastry = nil, 0, nil, nil
}

// stubPairCompute answers every text compute at once. A pair's report names
// its two sides, so a committed frame says which pair it was computed for.
func stubPairCompute(t *testing.T) *pairCalls {
	t.Helper()
	calls := &pairCalls{}
	oldN, oldT, oldO, oldS := firstCallNatalPositions, firstCallTransits, overlayCrossAspects, synastryCrossAspects
	t.Cleanup(func() {
		firstCallNatalPositions, firstCallTransits, overlayCrossAspects, synastryCrossAspects = oldN, oldT, oldO, oldS
	})
	firstCallNatalPositions = func(_ context.Context, _ *lookup.Probers, c store.AnchorChart, _ lookup.Lens) (lookup.Result, error) {
		calls.mu.Lock()
		calls.natal = append(calls.natal, c)
		calls.mu.Unlock()
		return lookup.Result{Positions: []aspect.Position{{Body: aspect.Sun, Longitude: float64(c.When.Year() % 360)}}}, nil
	}
	firstCallTransits = func(context.Context, *lookup.Probers, time.Time, location.PlaceInfo, lookup.Lens) (lookup.Result, error) {
		calls.mu.Lock()
		calls.transit++
		calls.mu.Unlock()
		return lookup.Result{Positions: []aspect.Position{{Body: aspect.Sun, Longitude: 1}}}, nil
	}
	overlayCrossAspects = func(_ context.Context, _ lookup.Lens, c store.AnchorChart, when time.Time, _ location.PlaceInfo) (lookup.OverlayAspects, error) {
		calls.mu.Lock()
		calls.cross = append(calls.cross, c)
		calls.mu.Unlock()
		return lookup.OverlayAspects{Report: fmt.Sprintf("transit %d x now", c.When.Year())}, nil
	}
	synastryCrossAspects = func(_ context.Context, _ lookup.Lens, c store.AnchorChart, when time.Time, place location.PlaceInfo, _ store.ReportLabel) (lookup.OverlayAspects, error) {
		calls.mu.Lock()
		calls.synastry = append(calls.synastry, [2]store.AnchorChart{c, {When: when, Place: place}})
		calls.mu.Unlock()
		return lookup.OverlayAspects{Report: fmt.Sprintf("synastry %d x %d", c.When.Year(), when.Year())}, nil
	}
	return calls
}

// C1a (2026-10-04: "the tab that corresponds with the chart cursor position
// should be pre-selected and rendered thus. it guarantees that the chart is
// always rendering to the ui what happens under the cursor."). The design rule,
// walked over EVERY row: A B C selected, D E not, now selected (① ② ③ ④), the
// cursor arriving by the real Up/Down keys in several orders, once after `]`
// cycled the tab elsewhere. After each arrival the active tab, the rendered
// panel title and the panel's content all describe the row under the cursor,
// and an arrival on ① made no cross call.
func TestAspectsTabIsTheCursorsChart(t *testing.T) {
	calls := stubPairCompute(t)
	// ① must show ITS OWN aspects: the stubbed natal compute returns a report
	// that names the chart it was computed for.
	oldN := firstCallNatalPositions
	firstCallNatalPositions = func(ctx context.Context, p *lookup.Probers, c store.AnchorChart, l lookup.Lens) (lookup.Result, error) {
		res, err := oldN(ctx, p, c, l)
		res.Aspects = tabsOwnAspects()
		res.AspectsReport = fmt.Sprintf("own %d", c.When.Year())
		return res, err
	}
	t.Cleanup(func() { firstCallNatalPositions = oldN })
	// On now the `Aspects` tab is the floating chart's own list, from the
	// transit compute (the specification C1).
	oldT := firstCallTransits
	firstCallTransits = func(ctx context.Context, p *lookup.Probers, when time.Time, place location.PlaceInfo, l lookup.Lens) (lookup.Result, error) {
		res, err := oldT(ctx, p, when, place, l)
		res.Aspects = tabsOwnAspects()
		res.AspectsReport = "own now"
		return res, err
	}
	t.Cleanup(func() { firstCallTransits = oldT })

	build := func(t *testing.T) Model {
		m := selectedSet(t, 5, true, 0)
		m.slots.SetSlotSelected(3, false) // D
		m.slots.SetSlotSelected(4, false) // E
		m.cfg = Config{Color: ColorNever}
		m.focused = PanelChart
		m = m.noteArrival()
		m, cmd := m.afterComparisonChange()
		return pump(t, m, cmd)
	}
	// check asserts the row under the cursor: wantTitle is the active pair's tab
	// ("" for the cursor's own chart), the rendered title marks it, and the
	// panel's content describes it.
	check := func(t *testing.T, m Model, why, wantTitle string) {
		t.Helper()
		cursor := m.focusedSlot()
		numeral := m.slots.Numeral(cursor)
		onNow := cursor.Floating
		line := aspectsTopBorder(t, m)
		if wantTitle == "" {
			if got := m.activeTabID(PanelAspects); got != TabAspects {
				t.Errorf("%s: cursor %d (numeral %d): active tab %q, want the chart's own", why, m.cursorIdx, numeral, got)
			}
			if !strings.Contains(line, "[Aspects]") && !strings.Contains(line, "[3] Aspects") {
				t.Errorf("%s: cursor %d: title %q does not mark the own tab active", why, m.cursorIdx, line)
			}
			if m.chart.crossAspectsReport != "" || len(m.chart.crossAspects) != 0 {
				t.Errorf("%s: cursor %d: the own-chart tab carries a cross list %q", why, m.cursorIdx, m.chart.crossAspectsReport)
			}
			if len(m.chart.aspects) == 0 {
				t.Errorf("%s: cursor %d: the own-chart tab has no aspects", why, m.cursorIdx)
			}
		} else {
			if !strings.Contains(line, "["+wantTitle+"]") {
				t.Errorf("%s: cursor %d: title %q does not mark %q active", why, m.cursorIdx, line, wantTitle)
			}
			if m.chart.crossAspectsReport == "" {
				t.Errorf("%s: cursor %d: the pair tab %q has no cross list", why, m.cursorIdx, wantTitle)
			}
		}
		if onNow && wantTitle == "" && m.chart.aspectsReport != "own now" {
			t.Errorf("%s: on now the panel reports %q, want the floating chart's own %q", why, m.chart.aspectsReport, "own now")
		}
		if numeral == 1 && wantTitle == "" {
			if want := fmt.Sprintf("own %d", cursor.Chart.When.Year()); m.chart.aspectsReport != want {
				t.Errorf("%s: on ① the panel reports %q, want ①'s own %q", why, m.chart.aspectsReport, want)
			}
		}
	}
	// step presses Up/Down once, pumping every command, and checks the row the
	// cursor lands on against want.
	step := func(t *testing.T, m Model, key tea.KeyMsg, why, want string) Model {
		t.Helper()
		calls.reset()
		var cmd tea.Cmd
		m, cmd = tpress(t, m, key)
		m = pump(t, m, cmd)
		if m.slots.Numeral(m.focusedSlot()) == 0 {
			if _, _, cross, syn := calls.snapshot(); len(cross)+len(syn) != 0 {
				t.Errorf("%s: arriving on an unselected row made %d cross calls, want none", why, len(cross)+len(syn))
			}
		}
		check(t, m, why, want)
		return m
	}

	// S4a A1: the tab starts on the chart alone, and arriving on any row, in any
	// order, leaves it there: no visitor was chosen.
	m := build(t)
	check(t, m, "start", "")
	for _, order := range [][]int{{1, 2, 3, 4, 5}, {5, 4, 3, 2, 1, 0}, {0, 3, 1, 5, 2, 4, 0, 2, 5, 1, 3, 0}} {
		for _, idx := range order {
			for m.cursorIdx != idx {
				key := keyDown
				if idx < m.cursorIdx {
					key = keyUp
				}
				m = step(t, m, key, "arrival", "")
			}
		}
	}

	// After `]` chose a base: from ①, two `]` reach "1 in 3" (① read in ③),
	// and the base is remembered as the cursor walks, landing on it swapping
	// it for the chart you came from, the unselected rows showing their own
	// chart alone.
	m = build(t)
	m.focused = PanelAspects
	for i := 0; i < 2; i++ {
		var cmd tea.Cmd
		m, cmd = tpress(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
		m = pump(t, m, cmd)
	}
	check(t, m, "chosen", "1 in 3")
	m.focused = PanelChart
	for _, s := range []struct {
		key  tea.KeyMsg
		want string
	}{
		{keyDown, "2 in 3"}, // ③ stays the base
		{keyDown, "3 in 2"}, // landing on the base swaps to the chart you came from
		{keyDown, ""},       // D is not selected
		{keyDown, ""},       // E is not selected
		{keyDown, "4 in 2"}, // now read in ②, the base carried
		{keyUp, ""},
		{keyUp, ""},
		{keyUp, "3 in 2"},
		{keyUp, "2 in 3"}, // landing on the base swaps again
		{keyUp, "1 in 3"},
	} {
		m = step(t, m, s.key, "after choosing", s.want)
	}
}

// C1b (amended 2026-10-04): `]` / `[` cycle ALL the tabs, `Aspects` included,
// wrap, never move the slot cursor, and recompute what they land on; landing on
// `Aspects` makes no cross call. Amended by the view rule (2026-10-24): the
// cursor on ① starts on its own chart, the pairs are `c in k` for every other
// anchored member k, a -r0 call with the base k as chart 1; on now each pair
// `now in k` is an -rt call whose natal is the base k.
func TestBracketsCycleAspectPairs(t *testing.T) {
	type step struct {
		key         rune
		want        string
		wantReport  string
		wantCrossFn string
	}
	for _, tc := range []struct {
		name   string
		now    bool
		cursor int
		start  string
		steps  []step
	}{
		// With ① ② ③ ④now selected the cursor on ① starts on `Aspects`; `]` walks
		// `1 in 2` and `1 in 3` (synastries, -r0) and wraps: now is never a base.
		{"now selected", true, 0, "Aspects", []step{
			{']', "1 in 2", "synastry 1962 x 1961", "synastry"},
			{']', "1 in 3", "synastry 1963 x 1961", "synastry"},
			{']', "Aspects", "", "none"}, // wraps onto the cursor's own chart
			{'[', "1 in 3", "synastry 1963 x 1961", "synastry"},
			{'[', "1 in 2", "synastry 1962 x 1961", "synastry"},
		}},
		{"now not selected", false, 0, "Aspects", []step{
			{']', "1 in 2", "synastry 1962 x 1961", "synastry"},
			{']', "1 in 3", "synastry 1963 x 1961", "synastry"},
			{']', "Aspects", "", "none"},
			{'[', "1 in 3", "synastry 1963 x 1961", "synastry"},
			{'[', "1 in 2", "synastry 1962 x 1961", "synastry"},
		}},
		// The cursor on now: each anchored chart is a base in turn, -rt.
		{"on now", true, 3, "Aspects", []step{
			{']', "4 in 1", "transit 1961 x now", "transit"},
			{']', "4 in 2", "transit 1962 x now", "transit"},
			{']', "4 in 3", "transit 1963 x now", "transit"},
			{']', "Aspects", "", "none"},
			{'[', "4 in 3", "transit 1963 x now", "transit"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := stubPairCompute(t)
			m := selectedSet(t, 3, tc.now, tc.cursor)
			m.cfg = Config{Color: ColorNever}
			m.focused = PanelAspects
			m, cmd0 := m.afterComparisonChange()
			m = pump(t, m, cmd0)
			press := func(r rune) (Model, tea.Cmd) { return tpress(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}) }
			active := func() string {
				return m.panelTabs(PanelAspects)[activeTabIndex(m.panelTabs(PanelAspects), m.tabChoice(PanelAspects))].Title
			}
			if got := active(); got != tc.start {
				t.Fatalf("on the baseline: %q, want %q", got, tc.start)
			}
			for _, step := range tc.steps {
				calls.reset()
				var cmd tea.Cmd
				m, cmd = press(step.key)
				if cmd == nil {
					t.Fatalf("%q: a pair switch started no compute", step.key)
				}
				if m.cursorIdx != tc.cursor {
					t.Fatalf("%q moved the slot cursor to %d", step.key, m.cursorIdx)
				}
				if got := active(); got != step.want {
					t.Fatalf("%q: active %q, want %q", step.key, got, step.want)
				}
				m = pump(t, m, cmd)
				if m.chart.crossAspectsReport != step.wantReport {
					t.Errorf("%q: the committed frame is %q, want the pair's %q", step.key, m.chart.crossAspectsReport, step.wantReport)
				}
				_, _, cross, syn := calls.snapshot()
				if (step.wantCrossFn == "transit") != (len(cross) == 1) || (step.wantCrossFn == "synastry") != (len(syn) == 1) || (step.wantCrossFn == "none" && len(cross)+len(syn) != 0) {
					t.Errorf("%q: transit cross calls %d, synastry calls %d, want %s only", step.key, len(cross), len(syn), step.wantCrossFn)
				}
			}
		})
	}
	press := func(m Model, r rune) (Model, tea.Cmd) {
		return tpress(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// A single tab is left alone: a cursor outside the set has nothing to cycle.
	m := selectedSet(t, 3, false, 3)
	m.focused = PanelAspects
	if _, cmd := press(m, ']'); cmd != nil {
		t.Error("`]` with the cursor outside the selected set started a compute")
	}
}

// chartFileCapturingAstrolog installs a PATH-local Astrolog stand-in that records
// every spawn's argv AND the contents of every staged .as file it was handed,
// so a test can say which chart sat in which argument. A wheel render's `-Xo`
// sink is written (three bytes), so an image call succeeds.
type spawnRecord struct {
	argv   []string // the call's own argv, after -YQ 0 and the hook clears
	prefix []string // what stood before it: -YQ 0 and the hook clears
	dats   []string // contents, in argument order
}

func chartFileCapturingAstrolog(t *testing.T) func() []spawnRecord {
	t.Helper()
	dir := t.TempDir()
	positions, err := os.ReadFile(filepath.Join("..", "astrolog", "testdata", "provenance", "positions-k.txt"))
	if err != nil {
		t.Fatalf("read positions fixture: %v", err)
	}
	aspects, err := os.ReadFile(filepath.Join("..", "astrolog", "testdata", "provenance", "aspects-k.txt"))
	if err != nil {
		t.Fatalf("read aspects fixture: %v", err)
	}
	positionsPath, aspectsPath, logPath := filepath.Join(dir, "positions.txt"), filepath.Join(dir, "aspects.txt"), filepath.Join(dir, "argv.log")
	if err := os.WriteFile(positionsPath, positions, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(aspectsPath, aspects, 0o600); err != nil {
		t.Fatal(err)
	}
	// One record is built in a private file and appended with a single write:
	// concurrent spawns (parallel timeline and wheel runs) would otherwise
	// interleave their lines, and a record could close on another spawn's SPAWN
	// line with an empty argv.
	script := "#!/bin/sh\n" +
		"rec=" + strconv.Quote(logPath) + ".$$\n" +
		"{ printf 'SPAWN\\n'; for a in \"$@\"; do printf 'ARG %s\\n' \"$a\"; done\n" +
		"  for a in \"$@\"; do case \"$a\" in *.as) printf 'DAT\\n'; /bin/cat \"$a\"; printf '\\nENDDAT\\n';; esac; done\n" +
		"  printf 'END\\n'; } > \"$rec\"\n" +
		"/bin/cat \"$rec\" >> " + strconv.Quote(logPath) + "; /bin/rm -f \"$rec\"\n" +
		"prev=; for a in \"$@\"; do if [ \"$prev\" = -Xo ]; then printf IMG > \"$a\"; fi; prev=$a; done\n" +
		"case \" $* \" in\n" +
		"  *\" -aa \"*) exec /bin/cat " + strconv.Quote(aspectsPath) + " ;;\n" +
		"  *) exec /bin/cat " + strconv.Quote(positionsPath) + " ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "astrolog"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return func() []spawnRecord {
		data, err := os.ReadFile(logPath)
		if err != nil {
			return nil
		}
		var out []spawnRecord
		var cur spawnRecord
		inChartFile := false
		var dat []string
		for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
			switch {
			case line == "SPAWN":
				cur = spawnRecord{}
			case line == "END":
				// C1: every spawn starts with -YQ 0 and the hook clears; the
				// tests are about the call's own argv.
				if own, prefix, ok := stripSpawnPrefix(cur.argv); !ok {
					t.Errorf("spawn argv %q does not start with -YQ 0 and the hook clears (C1)", cur.argv[:min(len(cur.argv), 6)])
				} else {
					cur.argv, cur.prefix = own, prefix
				}
				out = append(out, cur)
			case line == "DAT":
				inChartFile, dat = true, nil
			case line == "ENDDAT":
				inChartFile = false
				cur.dats = append(cur.dats, strings.Join(dat, "\n"))
			case inChartFile:
				dat = append(dat, line)
			case strings.HasPrefix(line, "ARG "):
				cur.argv = append(cur.argv, strings.TrimPrefix(line, "ARG "))
			}
		}
		return out
	}
}

// normaliseStage replaces the per-run staging directory in an argv with
// <stage>, so a recorded argv can be pinned literally.
func normaliseStage(argv []string) []string {
	out := make([]string, len(argv))
	for i, a := range argv {
		if strings.Contains(a, "sextile-r0-") {
			a = "<stage>/" + filepath.Base(a)
		}
		out[i] = a
	}
	return out
}

func relationSpawn(t *testing.T, spawns []spawnRecord, relation string) spawnRecord {
	t.Helper()
	var found []spawnRecord
	for _, s := range spawns {
		if len(s.argv) > 0 && s.argv[0] == relation {
			found = append(found, s)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d spawns start with %s, want exactly one: %v", len(found), relation, spawns)
	}
	return found[0]
}

// C1c: an active pair of two anchored slots is computed with -r0, baseline as
// chart 1 — through the real lookup, compiler and process seam.
func TestAnchoredPairIsSynastry(t *testing.T) {
	spawns := chartFileCapturingAstrolog(t)
	// A and B selected, now not; the cursor on B read in A, so ② in ① is the
	// pair, the baseline its base.
	m := selectedSet(t, 2, false, 1)
	m.base = m.slots.All()[0] // ② read in ①: the tab starts on the chart alone
	m, cmd := m.afterComparisonChange()
	m = pump(t, m, cmd)
	got := spawns()
	syn := relationSpawn(t, got, "-r0")
	if len(syn.argv) < 4 || filepath.Base(syn.argv[1]) != "chart0.as" || filepath.Base(syn.argv[2]) != "chart1.as" {
		t.Fatalf("-r0 argv = %v, want -r0 <chart 1 file> <chart 2 file> …", syn.argv)
	}
	if !hasArg(syn.argv, "-aa") {
		t.Errorf("-r0 argv carries no -aa: %v", syn.argv)
	}
	for _, s := range got {
		if len(s.argv) > 0 && s.argv[0] == "-rt" {
			t.Errorf("an anchored pair also ran a transit comparison: %v", s.argv)
		}
	}
	// Chart 1 is the baseline (the 1st chart, 1961), chart 2 the other (1962).
	if len(syn.dats) != 2 || chartFileYear(syn.dats[0]) != "1961" || chartFileYear(syn.dats[1]) != "1962" {
		t.Errorf("staged charts = %q, want the baseline (1961) then the other (1962)", syn.dats)
	}
	// The pair differs from today's transit comparison in the relation switch
	// and nothing else: same band, same colour, same list mode.
	today := stageNormalisedRtArgv(t, spawns)
	if want := append([]string{"-r0"}, today[1:]...); !reflect.DeepEqual(normaliseStage(syn.argv), want) {
		t.Errorf("-r0 argv = %v, want today's transit argv with -rt replaced: %v", normaliseStage(syn.argv), want)
	}
}

func hasArg(argv []string, tok string) bool {
	for _, a := range argv {
		if a == tok {
			return true
		}
	}
	return false
}

// stageNormalisedRtArgv computes today's baseline x now comparison in the
// same fake (a second model, same PATH) and returns its normalised -rt argv.
func stageNormalisedRtArgv(t *testing.T, spawns func() []spawnRecord) []string {
	t.Helper()
	m := listModel(t, 2, 2) // A and now selected, the cursor on now
	// C1a (2026-10-04): on now the tab is `Aspects`; now read in A is chosen
	// here as `]` would, which is the comparison this helper records.
	m.base = m.slots.All()[0]
	m, cmd := m.afterComparisonChange()
	pump(t, m, cmd)
	var rt []string
	for _, s := range spawns() {
		if len(s.argv) > 0 && s.argv[0] == "-rt" {
			rt = normaliseStage(s.argv)
		}
	}
	if rt == nil {
		t.Fatal("no -rt spawn for the baseline x now comparison")
	}
	return rt
}

// C2: an active pair of the baseline and the floating slot is computed with
// -rt, byte-identical in argv to today's comparison. The sequence below was
// recorded from the tree BEFORE this item changed any compute (the fork point,
// the baseline A selected with now, the cursor on A) and is pinned literally.
func TestAspectsPairBaselineWithNowIsTodaysTransit(t *testing.T) {
	spawns := fixtureAstrolog(t, nil)
	m := listModel(t, 3, 3) // A and now selected; B and C are not; the cursor on now
	// Amended by C1a (2026-10-04): the tab starts on `Aspects`; now read in A
	// is one `3` away, so it is chosen here as `]` would (2026-10-24: a moment
	// is never a base, so the pair lives on now's row).
	if m.activePairSlot() != nil {
		t.Fatalf("on now the active pair is %v, want none (the Aspects tab)", m.activePairSlot())
	}
	m.base = m.slots.All()[0]
	pair := m.activePairSlot()
	if pair == nil || !pair.right.Floating {
		t.Fatalf("the active pair is %v, want the floating slot", pair)
	}
	if _, ok := buildTextCmd(m, context.Background(), 1, 1)().(FirstFrameReadyMsg); !ok {
		t.Fatal("the compute did not return a frame")
	}

	// The frame band, the reform-date pin (the specification C6), the clock pins
	// (the specification C1), then the chooser band.
	pins := []string{"-qag", "10", "15", "1582", "12:00am", "0", "0:00E", "0:00N", "=Yt", "_Yd"} // the reform-date pin, then the clock's
	band := []string{"-c", "14", "_s", "_Ys", "_sr0", ":sz", "-qag", "10", "15", "1582", "12:00am", "0", "0:00E", "0:00N", "=Yt", "_Yd", "-Yn", "-R", "11", "-R", "17", "-R", "18", "-R", "25", "-R", "28"}
	cat := func(parts ...[]string) []string {
		var out []string
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	want := [][]string{
		// The two single-chart computes carry the slot's own zone (-zZ, C4) and
		// the one instant every call for the slot sends (the second, one float up).
		// Every single-chart call sends the chart's label, empty for these
		// unnamed slots (the specification C1, C6).
		cat([]string{"-qj", "2437310.750694445", "-zl", "71:00W", "41:00N", "-zZ", "5", "0", "-zi", "", ""}, band, []string{"=v0", "-Yw", "0", "-k"}),
		cat([]string{"-qj", "2437310.750694445", "-zl", "71:00W", "41:00N", "-zZ", "5", "0", "-zi", "", ""}, band, []string{"=v0", "-Yw", "0", "-aa", "-k"}),
		cat([]string{"-qj", "2461156.0000000005", "-zl", "13:25E", "52:31N", "-zZ", "-1", "1", "-zi", "", ""}, band, []string{"=v0", "-Yw", "0", "-k"}),
		cat([]string{"-qj", "2461156.0000000005", "-zl", "13:25E", "52:31N", "-zZ", "-1", "1", "-zi", "", ""}, band, []string{"=v0", "-Yw", "0", "-aa", "-k"}),
		// The ephemeris probe never carries -zZ, and stays in UT.
		cat([]string{"-qj", "2437310.750694445", "-zl", "71:00W", "41:00N", "-zi", "", "", "-c", "14", "_s", "_Ys", "_sr0", ":sz"}, pins, []string{"=v0", "-Yw", "0", "-k"}),
		cat([]string{"-rt", "<stage>/chart0.as", "<stage>/chart1.as"}, band, []string{"-aa", "-k"}),
	}
	var got [][]string
	for _, s := range spawns() {
		got = append(got, normaliseStage(s))
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the comparison's spawns changed.\n got: %q\nwant: %q", got, want)
	}
}

// C3: while single, the Aspects panel shows the cursor's own chart, with no
// pair tabs: on an unselected slot, and with one slot selected.
func TestAspectsAloneWhenSingle(t *testing.T) {
	// The cursor on an unselected chart while A, B and now are selected.
	m := selectedSet(t, 3, true, 2)
	m.slots.SetSlotSelected(2, false)
	m.cfg = Config{Color: ColorNever}
	if m.comparing() {
		t.Fatal("fixture: the cursor on an unselected slot must not compare")
	}
	if got := tabIDs(m.panelTabs(PanelAspects)); !reflect.DeepEqual(got, []TabID{TabAspects}) {
		t.Errorf("unselected cursor: tabs = %v, want [aspects]", got)
	}
	if mode := selectComputeMode(m); mode != computeModeNatalSingle {
		t.Errorf("unselected cursor: compute mode %d, want that chart alone", mode)
	}
	m.width, m.height = 132, 40
	if view := ansi.Strip(m.View()); strings.Contains(view, " vs ") || !strings.Contains(view, "[3] Aspects") {
		t.Errorf("unselected cursor: the Aspects panel is not single:\n%s", view)
	}

	// Only one slot selected, the cursor on it.
	m = listModel(t, 2, 0)
	m.slots.SetSlotSelected(m.slots.AnchoredCount(), false) // now off: A alone
	if m.comparing() || m.slots.SelectedCount() != 1 {
		t.Fatal("fixture: one selected slot must not compare")
	}
	if got := tabIDs(m.panelTabs(PanelAspects)); !reflect.DeepEqual(got, []TabID{TabAspects}) {
		t.Errorf("one slot selected: tabs = %v, want [aspects]", got)
	}

	// The floating cursor with the floating slot the only member.
	m = listModel(t, 2, 2)
	m.slots.SetSlotSelected(0, false)
	if m.comparing() {
		t.Error("the floating slot alone compared")
	}
}

// stubSynastryBodyReports records calls to the anchored pair's body report.
func stubSynastryBodyReports(t *testing.T) *[]crossBodyCall {
	t.Helper()
	calls := []crossBodyCall{}
	old := computeSynastryBodyReport
	computeSynastryBodyReport = func(_ context.Context, _ lookup.Lens, natal store.AnchorChart, when time.Time, place location.PlaceInfo, _ store.ReportLabel, body aspect.Body) (string, string, error) {
		calls = append(calls, crossBodyCall{natal: natal, when: when, place: place, body: body})
		return "synastry raw", "", nil
	}
	t.Cleanup(func() { computeSynastryBodyReport = old })
	return &calls
}

// C4: the Details report for the body under the Bodies cursor follows the
// Aspects panel's active pair, or the cursor's chart alone while single.
// Amended by the specification rule 6: the report is the
// pair's LEFT chart (the base) against the RIGHT chart (the cursor's), titled
// with both numerals.
func TestDetailsFollowsThePair(t *testing.T) {
	single := stubRawReports(t)
	cross := stubCrossBodyReports(t)
	synastry := stubSynastryBodyReports(t)
	base := func(cursor int, now bool) Model {
		m := selectedSet(t, 3, now, cursor)
		m.cfg = Config{Color: ColorNever}
		// ① ② ③ and now are shown: the left column is 88 wide (C5), and
		// Details needs 80 beside it.
		m.width, m.height = dualColumnMinWidth(4), 40
		m.focused = PanelBodies
		m.chart.positions = []aspect.Position{{Body: aspect.Moon}}
		m.bodyCursorIdx = 0
		return m
	}
	counts := func() [3]int { return [3]int{len(*single), len(*cross), len(*synastry)} }
	run := func(m Model) Model {
		t.Helper()
		*single, *cross, *synastry = nil, nil, nil
		m, cmd := m.startDetailsNow()
		return pump(t, m, cmd)
	}
	slotsOf := func(m Model) []*store.Slot { return m.slots.All() }

	// Without now, the cursor on the baseline read in ③, pair ① in ③ (two
	// anchored): synastry with ③, the base, as the natal side.
	m := base(0, false)
	m.base = slotsOf(m)[2]
	m = run(m)
	if counts() != [3]int{0, 0, 1} {
		t.Fatalf("anchored pair: spawns single/cross/synastry = %v, want one synastry", counts())
	}
	call := (*synastry)[0]
	if call.natal != anchorOf(slotsOf(m)[2]) || !call.when.Equal(slotsOf(m)[0].Time) || call.body != aspect.Moon {
		t.Errorf("synastry call = %+v, want ③ x chart ①'s moment, body Moon", call)
	}
	if m.details.title != "[0] Moon 1 in 3" || m.details.body != "synastry raw" {
		t.Errorf("Details = %q / %q, want the pair's title and report", m.details.title, m.details.body)
	}

	// Without now, the cursor on B read in ①: ② in ①, and the report is ①
	// against ②'s moment (the sides flipped).
	m = base(1, false)
	m = send(t, m, keyUp)
	m = send(t, m, keyDown)
	m.base = slotsOf(m)[0]
	m.focused = PanelBodies
	m.chart.positions = []aspect.Position{{Body: aspect.Moon}}
	m = run(m)
	if m.details.title != "[0] Moon 2 in 1" || counts() != [3]int{0, 0, 1} {
		t.Errorf("cursor on B: Details %q, spawns %v; want 2 in 1 by synastry", m.details.title, counts())
	}
	if call := (*synastry)[0]; call.natal != anchorOf(slotsOf(m)[0]) || !call.when.Equal(slotsOf(m)[1].Time) {
		t.Errorf("cursor on B: synastry call = %+v, want ①'s chart against ②'s moment", call)
	}

	// With now selected, the cursor on now read in B: `④ in ②`, ② as the
	// natal of the transit cross report at now.
	m = base(3, true)
	m.base = slotsOf(m)[1]
	m = run(m)
	if counts() != [3]int{0, 1, 0} {
		t.Fatalf("pair with now: spawns = %v, want one transit cross", counts())
	}
	if m.details.title != "[0] Moon 4 in 2" || (*cross)[0].natal != anchorOf(slotsOf(m)[1]) || !(*cross)[0].when.Equal(slotsOf(m)[3].Time) {
		t.Errorf("pair with now: title %q, call %+v; want ② as the natal, now as the moment", m.details.title, (*cross)[0])
	}

	// The cursor on the floating slot itself: the `Aspects` tab, the single
	// report of now's chart (C6 of the specification).
	m = base(3, true)
	m = run(m)
	if m.details.title != "[0] Moon" || counts() != [3]int{1, 0, 0} {
		t.Errorf("cursor on now: title %q, spawns %v, call %+v; want the single report", m.details.title, counts(), *cross)
	}

	// Single (the cursor on an unselected chart): that chart alone.
	m = base(0, true)
	m.slots.SetSlotSelected(1, false)
	m.cursorIdx = 1
	m = run(m)
	if counts() != [3]int{1, 0, 0} || m.details.title != "[0] Moon" {
		t.Errorf("single: Details %q, spawns %v; want one single-chart report titled plain", m.details.title, counts())
	}

	// The Aspects panel's Details takes the active tab's title. From ① the
	// base ③ makes `1 in 3`, a synastry, because every anchored chart's pairs
	// are (k, cursor).
	m = base(0, true)
	m.focused = PanelAspects
	m.base = slotsOf(m)[2]
	m.chart.crossAspectsReport = "pair report"
	if title, body, ok := m.aspectsDetailsFromFrame(); !ok || title != "[0] 1 in 3" || body != "pair report" {
		t.Errorf("Aspects Details = %q / %q / %v, want the active pair's title", title, body, ok)
	}
}

// C14: a selected slot's marker in the Chart panel is its numeral, in list
// order; an unselected slot's cell stays blank; ⌃k swaps numerals with their
// charts. ①–⑥ in Unicode mode, `1`–`6` in ASCII mode.
func TestSelectedRowsShowTheirNumeral(t *testing.T) {
	markers := func(m Model, cfg Config) []string {
		t.Helper()
		body := ansi.Strip(chartSlotsBody(m.slots, m.cursorIdx, false, testFrozenTime(), testPlaceBerlin(), lookup.Hellenistic, 100, cfg))
		var out []string
		for _, line := range strings.Split(body, "\n") {
			out = append(out, string([]rune(line)[:1]))
		}
		return out
	}
	// Four charts and now: select the 3rd, then the 1st, then now.
	m := listModel(t, 4, 0)
	for i := 0; i < 5; i++ {
		m.slots.SetSlotSelected(i, false)
	}
	for _, i := range []int{2, 0, 4} {
		m.slots.SetSlotSelected(i, true)
	}
	for _, tc := range []struct {
		name string
		cfg  Config
		want []string
	}{
		{"unicode", Config{Color: ColorNever}, []string{"①", " ", "②", " ", "③"}},
		{"ascii", Config{Accessible: true, Color: ColorNever}, []string{"1", " ", "2", " ", "3"}},
	} {
		if got := markers(m, tc.cfg); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: markers %q, want %q", tc.name, got, tc.want)
		}
	}

	// ⌃k the 3rd above the 2nd: it now precedes the 2nd (still after the 1st
	// in list order): the numerals follow list order, not selection order.
	m.cursorIdx = 2
	m.focused = PanelChart
	m = send(t, m, keyCtrlK) // list: 1st, 3rd, 2nd, 4th, now
	m = send(t, m, keyCtrlK) // list: 3rd, 1st, 2nd, 4th, now
	all := m.slots.All()
	if all[0].Chart.Name != "Synthetic 3" || all[1].Chart.Name != "Synthetic 1" {
		t.Fatalf("fixture: list order is %v", order(m))
	}
	if got, want := markers(m, Config{Accessible: true, Color: ColorNever}), []string{"1", "2", " ", " ", "3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after ⌃k: markers %q, want %q (the 3rd and the 1st swapped)", got, want)
	}
	// And the selection mark ● never appears on a Chart row.
	if body := chartSlotsBody(m.slots, 0, true, testFrozenTime(), testPlaceBerlin(), lookup.Hellenistic, 100, Config{Color: ColorNever}); strings.Contains(body, rowSelectionUnicode) || strings.Contains(body, rowSelectionASCII) {
		t.Errorf("a Chart row still carries the selection mark: %q", body)
	}
}

// V1: the Chart panel with ①–④ on rows, now selected, the cursor on ②.
func TestSlotNumeralsFrames(t *testing.T) {
	forceColor(t)
	for _, size := range []struct {
		name string
		w, h int
	}{{"80x24", 80, 24}, {"132x40", 132, 40}} {
		var frames []string
		for _, cfg := range []Config{{Color: ColorAlways}, {Color: ColorNever}} {
			m := selectedSet(t, 3, true, 1) // ① ② ③ and now ④: the cursor on ②
			for i := 0; i < 3; i++ {
				m.slots.All()[i].Chart.Name = []string{"Synthetic Aster", "Synthetic Birch", "Synthetic Cedar"}[i]
			}
			m.cfg = cfg
			m = withFrame(m)
			m.width, m.height = size.w, size.h
			frames = append(frames, m.View())
		}
		writeDesignFrame(t, "slot-numerals-"+size.name, strings.Join(frames, "\n\n"))
	}
}

// V1 (S4a): the Aspects tab strip with "X in Y": ① ② and now selected, the cursor on Ben ②
// with ① visiting ("① in ②" active), over a synastry pair's cells.
func TestAspectsPairTabsFrames(t *testing.T) {
	forceColor(t)
	for _, size := range []struct {
		name string
		w, h int
	}{{"80x24", 80, 24}, {"132x40", 132, 40}} {
		for _, mode := range []struct {
			name string
			cfg  Config
		}{{"color", Config{Color: ColorAlways}}, {"never", Config{Color: ColorNever}}} {
			m := selectedSet(t, 2, true, 1) // ① ② anchored and now ③ selected; the cursor on ②: the tabs are ① in ②, ③ in ②
			for i := 0; i < 2; i++ {
				m.slots.All()[i].Chart.Name = []string{"Synthetic Aster", "Synthetic Birch"}[i]
			}
			m.cfg = mode.cfg
			m.base = m.slots.All()[0] // `]` from the chart's own tab
			m, _ = applyTextCommit(m, FirstFrameReadyMsg{
				Overlay:       true,
				Positions:     fullChartPositions(),
				CrossSynastry: true,
				CrossAspects: []lookup.OverlayAspect{
					{Natal: aspect.Sun, Transit: aspect.Moon, Kind: aspect.Square, Orb: 0.5, Power: 9},
					{Natal: aspect.Venus, Transit: aspect.Jupiter, Kind: aspect.Trine, Orb: 1.4, Power: 6},
					{Natal: aspect.Mars, Transit: aspect.Saturn, Kind: aspect.Opposition, Orb: 10.2, Power: 3},
				},
			})
			m = withSceneColumns(m)
			m.width, m.height = size.w, size.h
			m.focused = PanelAspects
			if got := m.activeTabID(PanelAspects); got != TabAspectsCross {
				t.Fatalf("fixture: active tab %q, want the first pair tab (① in ②)", got)
			}
			writeDesignFrame(t, fmt.Sprintf("aspects-pair-tabs-%s-%s", size.name, mode.name), m.View())
		}
	}
}
