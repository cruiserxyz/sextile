package ui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The main panels swap together (the specification).

// swapSpawns counts every Astrolog-bound lookup a scene change makes, and
// remembers the first chart each cross run was given.
type swapSpawns struct {
	mu       sync.Mutex
	columns  *columnCalls
	cross    int
	crossFst []store.AnchorChart
}

func (s *swapSpawns) total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, t := s.columns.counts()
	return n + t + s.cross
}

// stubSwapLookups stubs the column lookups and gives every pair its own cross
// row, so two pairs never read alike: the orb carries the other chart's year.
func stubSwapLookups(t *testing.T) *swapSpawns {
	t.Helper()
	s := &swapSpawns{columns: stubColumnLookups(t, nil)}
	row := func(other int) lookup.OverlayAspects {
		return lookup.OverlayAspects{
			Aspects: []lookup.OverlayAspect{{Natal: aspect.Sun, Transit: aspect.Moon, Kind: aspect.Square, Orb: aspect.Orb(float64(other%9) + 0.5), Power: 5}},
			Report:  "cross",
		}
	}
	overlayCrossAspects = func(_ context.Context, _ lookup.Lens, natal store.AnchorChart, when time.Time, _ location.PlaceInfo) (lookup.OverlayAspects, error) {
		s.mu.Lock()
		s.cross++
		s.crossFst = append(s.crossFst, natal)
		s.mu.Unlock()
		return row(when.Year()), nil
	}
	synastryCrossAspects = func(_ context.Context, _ lookup.Lens, natal store.AnchorChart, when time.Time, _ location.PlaceInfo, _ store.ReportLabel) (lookup.OverlayAspects, error) {
		s.mu.Lock()
		s.cross++
		s.crossFst = append(s.crossFst, natal)
		s.mu.Unlock()
		return row(when.Year()), nil
	}
	return s
}

// leftColumn splits a rendered frame into the Chart list (the rows above
// Bodies' top border) and the Bodies + Aspects panels, both cut to the left
// column and stripped of escapes.
func leftColumn(t *testing.T, view string) (chartList, panels string) {
	t.Helper()
	lines := strings.Split(ansi.Strip(view), "\n")
	width := 0
	for i, r := range []rune(lines[0]) {
		if r == '╮' {
			width = i + 1
			break
		}
	}
	if width == 0 {
		t.Fatalf("no Chart panel corner in the first line %q", lines[0])
	}
	cut := make([]string, len(lines))
	for i, ln := range lines {
		r := []rune(ln)
		if len(r) > width {
			r = r[:width]
		}
		cut[i] = strings.TrimRight(string(r), " ")
	}
	bodies := -1
	for i, ln := range cut {
		// The Bodies panel by its digit: its title is the view ("2 in 1") while
		// a pair is on screen.
		if strings.Contains(ln, "[2] ") && strings.HasPrefix(ln, "╭") {
			bodies = i
			break
		}
	}
	if bodies < 0 {
		t.Fatalf("no Bodies panel in\n%s", ansi.Strip(view))
	}
	return strings.Join(cut[:bodies], "\n"), strings.Join(cut[bodies:len(cut)-1], "\n")
}

// pumpViews is pump that also records the View after every message it applies.
func pumpViews(t *testing.T, m Model, cmd tea.Cmd) (Model, []string) {
	t.Helper()
	var views []string
	pending := []tea.Cmd{cmd}
	for round := 0; len(pending) > 0; round++ {
		if round > 40 {
			t.Fatal("pumpViews: the model never went quiet")
		}
		var chans []chan tea.Msg
		var done atomic.Int32
		for _, c := range pending {
			if c == nil {
				continue
			}
			ch := make(chan tea.Msg, 1)
			go func(c tea.Cmd) { ch <- c(); done.Add(1) }(c)
			chans = append(chans, ch)
		}
		pending = nil
		n := len(chans)
		deadline := roundDeadline(400*time.Millisecond, func() int { return n - int(done.Load()) })
	collect:
		for i, ch := range chans {
			select {
			case msg := <-ch:
				if batch, ok := msg.(tea.BatchMsg); ok {
					pending = append(pending, batch...)
					continue
				}
				if msg == nil {
					continue
				}
				tm, next := m.Update(msg)
				m = tm.(Model)
				views = append(views, m.View())
				pending = append(pending, next)
			case <-deadline:
				awaitAbandoned(t, chans[i:])
				break collect
			}
		}
	}
	return m, views
}

// Rules 1, 2 and 3 for every trigger: the screen goes from the old scene's
// Bodies and Aspects to the new one's in one View, and no View between shows a
// mixture; the Chart list reacts at the key press.
func TestSceneChangeSwapsMainPanelsTogether(t *testing.T) {
	for _, tc := range []struct {
		name      string
		listMoves bool
		start     func(Model) Model
		trigger   func(*testing.T, Model) (Model, tea.Cmd)
	}{
		{"cursor down", true, func(m Model) Model { return m },
			func(t *testing.T, m Model) (Model, tea.Cmd) { return tpress(t, m, keyDown) }},
		{"space deselects", true, func(m Model) Model { return m },
			func(t *testing.T, m Model) (Model, tea.Cmd) { return tpress(t, m, keySpace) }},
		{"reorder", true, func(m Model) Model { return m },
			func(t *testing.T, m Model) (Model, tea.Cmd) { return tpress(t, m, keyCtrlJ) }},
		{"pair cycle", false, func(m Model) Model { m.focused = PanelAspects; return m },
			func(t *testing.T, m Model) (Model, tea.Cmd) { return tpress(t, m, runes("]")) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubSwapLookups(t)
			m := tc.start(settledComparing(t, 1))
			m.height = 40
			list0, before := leftColumn(t, m.View())

			m1, cmd := tc.trigger(t, m)
			list1, mid := leftColumn(t, m1.View())
			if mid != before {
				t.Fatalf("in the key-press frame Bodies and Aspects changed before their frame landed:\n--- before\n%s\n--- key-press frame\n%s", before, mid)
			}
			if tc.listMoves && list1 == list0 {
				t.Errorf("the Chart list did not react in the key-press frame:\n%s", list1)
			}

			m2, views := pumpViews(t, m1, cmd)
			_, after := leftColumn(t, m2.View())
			if after == before {
				t.Fatalf("the scene change never reached Bodies and Aspects:\n%s", after)
			}
			for i, v := range views {
				if _, got := leftColumn(t, v); got != before && got != after {
					t.Fatalf("view %d of %d shows a mixture of the two scenes:\n--- before\n%s\n--- after\n%s\n--- got\n%s", i+1, len(views), before, after, got)
				}
			}
		})
	}
}

// Rule 3: the Chart list shows the new cursor in the frame of the key press,
// while Bodies and Aspects are still the old scene's.
func TestChartListReactsBeforeTheSwap(t *testing.T) {
	stubSwapLookups(t)
	m := settledComparing(t, 1)
	m.height = 40
	list0, panels0 := leftColumn(t, m.View())
	m1, cmd := tpress(t, m, keyDown)
	if m1.cursorIdx != 2 {
		t.Fatalf("cursor = %d, want 2", m1.cursorIdx)
	}
	list1, panels1 := leftColumn(t, m1.View())
	if list1 == list0 {
		t.Fatalf("the Chart list did not move in the key-press frame:\n%s", list1)
	}
	if panels1 != panels0 {
		t.Fatalf("Bodies and Aspects moved in the key-press frame:\n%s", panels1)
	}
	if cmd == nil {
		t.Fatal("the key press asked for no recompute")
	}
	m2 := pump(t, m1, cmd)
	if _, panels2 := leftColumn(t, m2.View()); panels2 == panels0 {
		t.Fatal("the recompute never reached Bodies and Aspects")
	}
}

// Rule 4: holding the old scene adds no spawn. The same scene change on a
// model that holds nothing makes the same number of Astrolog lookups.
func TestSwapAddsNoSpawn(t *testing.T) {
	for _, tc := range []struct {
		name    string
		start   func(Model) Model
		trigger func(*testing.T, Model) (Model, tea.Cmd)
	}{
		// ① is the base, so the move down to ③ lands on a pair that makes a
		// cross call (the own tab alone looks nothing up).
		{"cursor down", func(m Model) Model { m.base = m.slots.All()[0]; return m },
			func(t *testing.T, m Model) (Model, tea.Cmd) { return tpress(t, m, keyDown) }},
		// `[`: from the chart's own tab it wraps onto the LAST pair (② in ③),
		// which makes a cross call; `]` would reach the first pair just as
		// well.
		{"pair cycle", func(m Model) Model { m.focused = PanelAspects; return m },
			func(t *testing.T, m Model) (Model, tea.Cmd) { return tpress(t, m, runes("[")) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spawns := stubSwapLookups(t)
			run := func(hold bool) int {
				m := tc.start(settledComparing(t, 1))
				if !hold {
					m.shown = shownFrame{}
				}
				before := spawns.total()
				m1, cmd := tc.trigger(t, m)
				if m1.shown.held != hold {
					t.Fatalf("held = %v, want %v", m1.shown.held, hold)
				}
				pump(t, m1, cmd)
				return spawns.total() - before
			}
			held, unheld := run(true), run(false)
			if unheld == 0 {
				t.Fatal("the scene change made no lookup at all: the count proves nothing")
			}
			if held != unheld {
				t.Fatalf("holding the old scene made %d lookups, the same change without it %d", held, unheld)
			}
		})
	}
}

// Rule 6, as the try of 2026-10-24 left it: a tap's pending report keeps
// its previous body as drawn; from the second key of a run its body is faint
// (colours on) and its title ends " …" (colours off); the frame the new
// report commits after the key is let go, both go.
func TestDetailsDimsWhileAKeyIsHeld(t *testing.T) {
	stubSwapLookups(t)
	stubRawReports(t)
	for _, tc := range []struct {
		name string
		cfg  Config
	}{
		{"colours off", Config{Color: ColorNever}},
		{"colours on", Config{Color: ColorAlways}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.cfg.useColor() {
				forceColor(t)
			}
			m := settledComparing(t, 1)
			m.cfg = tc.cfg
			m.height = 40
			m, cmd := m.requestDetails()
			m = pump(t, m, cmd)
			if m.details.body != "single raw" || m.details.pending {
				t.Fatalf("fixture: details body %q, pending %v", m.details.body, m.details.pending)
			}
			if got := detailsWaitingSigns(m.View()); got != "" {
				t.Fatalf("a settled Details shows the waiting sign: %s", got)
			}

			m1, cmd := tpress(t, m, keyDown)
			if !m1.details.pending {
				t.Fatal("fixture: the scene change left Details settled")
			}
			if got := detailsWaitingSigns(m1.View()); got != "" {
				t.Fatalf("a tap's pending Details shows the waiting sign: %s", got)
			}
			m1, cmd2 := tpress(t, m1, keyDown)
			cmd = tea.Batch(cmd, cmd2)
			if got := detailsWaitingSigns(m1.View()); got == "" {
				t.Fatalf("a held key's pending Details shows no waiting sign:\n%s", m1.View())
			}

			m2 := pump(t, m1, cmd)
			if m2.details.pending {
				t.Fatal("fixture: Details never settled after the new frame")
			}
			if got := detailsWaitingSigns(m2.View()); got != "" {
				t.Fatalf("the frame the new report committed still shows the waiting sign: %s", got)
			}
		})
	}
}

// detailsWaitingSigns names the waiting sign the Details panel shows, "" for
// none: the title's trailing ellipsis colours off, a faint report line colours on.
func detailsWaitingSigns(view string) string {
	if strings.Contains(ansi.Strip(view), "Chart …") {
		return "title ends …"
	}
	for _, ln := range strings.Split(view, "\n") {
		if strings.Contains(ln, sgrFaint+"single raw") {
			return "faint report"
		}
	}
	return ""
}

// Rule 7. Astrolog lists the first chart of a pair on the left of every cell,
// and the pair's first numeral names the same chart: since
// the specification that is the pair's LEFT chart (the base), and the compute
// is handed it first
// (buildTextCmd: withCrossLeft). The floating slot is pinned last in the
// selected set (store.Slots.SelectedInOrder), so it can never be the tab's
// first numeral.
func TestCrossRowLeftBodyIsTheTabsFirstChart(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cursor int // the cursor: ② anchored, or ④ the floating slot
		title  string
		left   int    // the pair's left chart, an index into the set (rule 4 of aspects-pairs-follow-the-cursor)
		last   string // the strip's last tab: `2 in 3` on ②, `4 in 3` on now
	}{
		{"anchored cursor", 1, "2 in 1", 0, "2 in 3"},
		{"floating cursor", 3, "4 in 1", 0, "4 in 3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spawns := stubSwapLookups(t)
			m := settledComparing(t, tc.cursor)
			m.height = 40
			{
				// The tab starts on the chart alone (S4a A1): the pair is one
				// real key away. `]` reaches the first pair, read in ①.
				key := ']'
				m.focused = PanelAspects
				var cmd tea.Cmd
				m, cmd = tpress(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
				m = pump(t, m, cmd)
				m.focused = PanelChart
			}
			if got := titlesOf(m.panelTabs(PanelAspects)); got[len(got)-1] != tc.last {
				t.Fatalf("fixture: tabs %v", got)
			}
			sc := m.deriveScene()
			if active := m.activeTabID(PanelAspects); sc.pair == nil || active != sc.pairTabID(sc.pairIndex()) {
				t.Fatalf("fixture: active tab %q", active)
			}
			leftChart := anchorOf(sc.set[tc.left])
			if first := spawns.crossFst; len(first) == 0 || first[len(first)-1] != leftChart {
				t.Fatalf("the cross run was handed %v first, want the pair's left chart %v", first, leftChart)
			}
			_, panels := leftColumn(t, m.View())
			if !strings.Contains(panels, tc.title) {
				t.Fatalf("no %q tab in\n%s", tc.title, panels)
			}
			if !strings.Contains(panels, "☉ □ ☽") || strings.Contains(panels, "☽ □ ☉") {
				t.Fatalf("the cell does not read ①'s body first:\n%s", panels)
			}
		})
	}
}

// The floating slot is never numeral ①, whichever way the set was selected: the
// premise rule 7 rests on, checked over every selection of three anchored slots
// and the floating one.
func TestFloatingSlotIsNeverTheTabsFirstNumeral(t *testing.T) {
	m := selectedSet(t, 3, true, 0)
	for mask := 0; mask < 16; mask++ {
		for i := 0; i < 4; i++ {
			m.slots.SetSlotSelected(i, mask&(1<<i) != 0)
		}
		for cursor := 0; cursor < 4; cursor++ {
			m.cursorIdx = cursor
			sc := m.deriveScene()
			if sc.mode != sceneComparing {
				continue
			}
			if sc.baseline == nil || sc.baseline.Floating || sc.set[0] != sc.baseline {
				t.Fatalf("selection %04b: ① is not the anchored baseline", mask)
			}
			for _, pair := range sc.pairs {
				if pair.left.Floating {
					t.Fatalf("selection %04b: the floating slot is a pair's left side", mask)
				}
			}
		}
	}
}

// detailsWaitingFrame is a comparing pair with Details' report loaded, one
// scene change in by a held ↑ (a tap draws no waiting sign): Details is
// waiting for the new report.
func detailsWaitingFrame(t *testing.T, cfg Config, w, h int) string {
	t.Helper()
	stubSwapLookups(t)
	stubRawReports(t)
	m := selectedSet(t, 1, true, 1)
	m.cfg = cfg
	m.width, m.height = w, h
	m.focused = PanelChart
	m, cmd := m.afterComparisonChange()
	m = pump(t, m, cmd)
	m, cmd = m.requestDetails()
	m = pump(t, m, cmd)
	if m.details.body != "single raw" {
		t.Fatalf("fixture: details body %q", m.details.body)
	}
	// The run's first ↑ arrived just now; this one is its first repeat.
	m.detailsHold.key, m.detailsHold.keys, m.detailsHold.last = keyUp.String(), 1, m.clock.Now()
	m, _ = tpress(t, m, keyUp)
	if !m.details.pending || !m.detailsHold.on {
		t.Fatalf("fixture: Details is not waiting in a hold (pending=%v held=%v)", m.details.pending, m.detailsHold.on)
	}
	return m.View()
}

// The frames the spec freezes (the specification).
func TestSceneSwapFrames(t *testing.T) {
	// No 80x24 frame: Details is not in the layout below dualColumnMinWidth
	// (layoutGeometry), so at 80x24 there is no Details panel to wait.
	frame := detailsWaitingFrame(t, Config{Color: ColorNever}, 144, 40)
	if !strings.Contains(ansi.Strip(frame), "Chart …") {
		t.Fatalf("no waiting mark in\n%s", frame)
	}
	checkGolden(t, "scene-swap-details-waiting-144x40-never", frame)
	forceColor(t)
	checkGolden(t, "scene-swap-details-waiting-144x40-color", detailsWaitingFrame(t, Config{Color: ColorAlways}, 144, 40))

	stubSwapLookups(t)
	m := settledComparing(t, 1)
	m.cfg = Config{Color: ColorNever}
	m.width, m.height = 132, 40
	m, _ = tpress(t, m, keySpace)
	checkGolden(t, "scene-swap-midway-132x40-never", m.View())
}

// Rule 5: a failed frame for the current scene swaps in with its error, and a
// superseded frame never does. Results go through the real coordinator path
// (Update -> onRefreshResult), tagged with the ids the runs were started with.
func TestFailedOrSupersededFrameAndTheHold(t *testing.T) {
	ids := func(m Model) (run, want uint64) {
		return m.refresh[surfaceText].runningRunID, m.refresh[surfaceText].runningWantID
	}
	send := func(t *testing.T, m Model, msg FirstFrameReadyMsg) Model {
		t.Helper()
		tm, _ := m.Update(msg)
		return tm.(Model)
	}

	t.Run("failed swaps in", func(t *testing.T) {
		stubSwapLookups(t)
		m := settledComparing(t, 1)
		m.height = 40
		_, before := leftColumn(t, m.View())
		m1, cmd := tpress(t, m, keyDown)
		if !m1.shown.held {
			t.Fatal("fixture: the scene change held nothing")
		}
		run, want := ids(m1)
		_ = cmd // never pumped: the failure below is this run's result
		m2 := send(t, m1, FirstFrameReadyMsg{Err: errors.New("astrolog: exit status 1"), RunID: run, WantID: want})
		if m2.shown.held {
			t.Fatal("a failed frame for the current scene left the old scene held")
		}
		if _, after := leftColumn(t, m2.View()); after == before {
			t.Fatalf("Bodies and Aspects still show the old scene after the failed frame:\n%s", after)
		}
		if log := strings.Join(m2.logPanelRows(), "\n"); !strings.Contains(log, "Couldn't cast the chart") {
			t.Fatalf("no error line in the Log:\n%s", log)
		}
	})

	t.Run("superseded never swaps in", func(t *testing.T) {
		stubSwapLookups(t)
		m := settledComparing(t, 0)
		m.height = 40
		mB, _ := tpress(t, m, keyDown)
		runB, wantB := ids(mB)
		mC, _ := tpress(t, mB, keyDown)
		runC, wantC := ids(mC)
		if runB == runC || wantB == wantC || !mC.shown.held {
			t.Fatalf("fixture: B=(%d,%d) C=(%d,%d) held=%v", runB, wantB, runC, wantC, mC.shown.held)
		}
		_, held := leftColumn(t, mC.View())

		m2 := send(t, mC, FirstFrameReadyMsg{Positions: fullChartPositions(), RunID: runB, WantID: wantB})
		if !m2.shown.held {
			t.Fatal("B's superseded frame released the hold")
		}
		if _, got := leftColumn(t, m2.View()); got != held {
			t.Fatalf("B's superseded frame changed Bodies or Aspects:\n%s", got)
		}

		m3 := send(t, m2, FirstFrameReadyMsg{Positions: fullChartPositions(), RunID: runC, WantID: wantC})
		if m3.shown.held {
			t.Fatal("C's frame did not release the hold")
		}
	})
}

// A hard reset (here the session re-anchor, `m.chart = chartViewState{}`) after
// a scene change zeroes the chart and with it the hold: Bodies and Aspects must
// not go on painting a frame of a workspace the operator has left.
func TestHardResetDuringAHoldDropsIt(t *testing.T) {
	stubSwapLookups(t)
	m := settledComparing(t, 1)
	m.height = 40
	_, settled := leftColumn(t, m.View())
	m1, _ := tpress(t, m, keyDown)
	if !m1.holding() {
		t.Fatal("fixture: the scene change held nothing")
	}
	if _, got := leftColumn(t, m1.View()); got != settled {
		t.Fatal("fixture: the hold does not paint the settled scene")
	}

	cand := m1.slots.All()[0].Chart
	m2, _ := m1.reanchorEditedChart(0, cand)
	if m2.holding() {
		t.Fatal("the re-anchor left the old frame held")
	}
	if _, got := leftColumn(t, m2.View()); got == settled {
		t.Fatalf("Bodies and Aspects still paint the held frame after the reset:\n%s", got)
	}
}
