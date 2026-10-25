package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
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

// the specification — the Details timeline tab, attachment and
// the tab keys. Every row fixture below is Astrolog 8.00's own output for
// 14–20 October 2026 at `-z -2` (Amsterdam is CEST until the 25th), taken
// 2026-09-30 on the spec's synthetic natal and turned into rows by lookup's
// real merge, so instants and commands are exactly what production produces.

const (
	weekTransit = "Astrolog 8.00 chart for Tue May 15, 1979  9:30 (ST Zone 0W)   4:54E 52:22N\n\nTransits during Oct 2026 (ST Zone 2E)\n" +
		"10/14/2026 23:26 trans     Sun (Lib) Opp natal (Ari) Midheaven\n" +
		"10/17/2026 21:16 trans North N [Aqu] Sex natal (Ari) Venus\n" +
		"10/18/2026  3:13 trans Mercury (Sco) Con natal [Sco] Uranus\n" +
		"10/18/2026  6:40 trans    Mars (Leo) Con natal (Leo) Ascendant\n" +
		"10/20/2026  7:00 trans     Sun (Lib) Opp natal (Ari) Venus\n"
	weekSky = "Wed 10/14/2026  2:59    Moon (Sco) --> Sagittarius\n" +
		"Wed 10/14/2026  7:55    Moon (Sag) Tri [Ari] Neptune\n" +
		"Wed 10/14/2026  9:01    Moon (Sag) Sex [Aqu] Pluto\n" +
		"Wed 10/14/2026 13:18    Moon (Sag) Opp [Gem] Uranus\n" +
		"Wed 10/14/2026 21:51    Moon (Sag) Tri (Leo) Mars\n" +
		"Wed 10/14/2026 23:44    Moon (Sag) Tri [Ari] Saturn\n" +
		"Thu 10/15/2026 10:21     Sun (Lib) Sex (Leo) Jupiter\n" +
		"Thu 10/15/2026 22:57    Moon (Sag) Tri (Leo) Jupiter\n" +
		"Thu 10/15/2026 23:55     Sun (Lib) Sex (Sag) Moon (v/c 15:01)\n" +
		"Fri 10/16/2026  4:40   Pluto <Aqu> S/D\n" +
		"Fri 10/16/2026  8:41    Moon (Sag) Sex [Aqu] North Node\n" +
		"Fri 10/16/2026 10:32    Mars (Leo) Tri [Ari] Saturn\n" +
		"Fri 10/16/2026 14:57    Moon (Sag) --> Capricorn\n" +
		"Fri 10/16/2026 19:53    Moon (Cap) Squ [Ari] Neptune\n" +
		"Sat 10/17/2026  0:51    Moon (Cap) Sex [Sco] Venus\n" +
		"Sat 10/17/2026 11:50    Moon (Cap) Squ [Ari] Saturn\n" +
		"Sun 10/18/2026  5:07    Moon (Cap) Sex (Sco) Mercury\n" +
		"Sun 10/18/2026 18:12     Sun (Lib) Squ (Cap) Moon (Half Moon) (v/c 9:27)\n" +
		"Mon 10/19/2026  3:39    Moon (Cap) --> Aquarius\n" +
		"Mon 10/19/2026  8:25    Moon (Aqu) Sex [Ari] Neptune\n" +
		"Mon 10/19/2026  9:49    Moon (Aqu) Con (Aqu) Pluto\n" +
		"Mon 10/19/2026 10:52    Moon (Aqu) Squ [Sco] Venus\n" +
		"Mon 10/19/2026 13:51    Moon (Aqu) Tri [Gem] Uranus\n" +
		"Mon 10/19/2026 23:51    Moon (Aqu) Sex [Ari] Saturn\n" +
		"Tue 10/20/2026  4:05     Sun (Lib) Tri [Aqu] North Node\n" +
		"Tue 10/20/2026  4:24    Moon (Aqu) Opp (Leo) Mars (Total Occultation)\n" +
		"Tue 10/20/2026  8:56   Venus [Sco] Squ (Aqu) Pluto\n" +
		"Tue 10/20/2026 19:42    Moon (Aqu) Squ (Sco) Mercury\n"
	weekReturn = "Astrolog 8.00 chart for Tue May 15, 1979  9:30 (ST Zone 0W)   4:54E 52:22N\n\nTransits during Oct 2026 (ST Zone 2E)\n" +
		"10/17/2026  4:05 trans    Moon (Cap) Con natal (Cap) Moon (Lunar Return)\n"
)

var (
	tlNatalWhen    = time.Date(1979, 5, 15, 9, 30, 0, 0, time.UTC)
	tlFloatingTime = time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC) // 12:00 CEST
)

func tlNatal() store.AnchorChart {
	return store.AnchorChart{When: tlNatalWhen, Place: testPlaceAmsterdam()}
}

// stubDefaults answers lookup's default-set read with the sets astrolog 8.00
// listed on 2026-10-03, so a scripted searcher needs no binary.
func stubDefaults(context.Context) (lookup.DefaultSets, error) {
	return lookup.DefaultSets{
		Main:    []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 16, 22, 31},
		Transit: []int{1, 3, 4, 5, 6, 7, 8, 9, 10, 16},
	}, nil
}

// weekSearch answers the CEST segment with the week and the CET segment with
// nothing, the way a real October at `-z -1` would for rows it keeps.
func weekSearch(_ context.Context, call lookup.BodyCall) (string, string, error) {
	argv := call.Argv()
	cmd := "astrolog " + strings.Join(argv, " ")
	if argv[1] != "-2" {
		return "", cmd, nil
	}
	switch {
	case slices.Contains(argv, "-tr"):
		return weekReturn, cmd, nil
	case slices.Contains(argv, "-t"):
		return weekTransit, cmd, nil
	}
	return weekSky, cmd, nil
}

// weekRows is the merged fixture list, produced by lookup's own merge for a
// comparing timeline with one natal: its transit and return rows carry numeral 1
// (P4), its sky rows none.
func weekRows(t *testing.T) []lookup.TimelineRow {
	defaultClock(t)
	t.Helper()
	res, err := lookup.TimelineWith(context.Background(),
		lookup.TimelineRequest{When: tlFloatingTime, Place: testPlaceAmsterdam(), Natals: []lookup.TimelineNatal{{Numeral: 1, Chart: tlNatal()}}}, weekSearch, stubDefaults)
	if err != nil {
		t.Fatalf("fixture merge: %v", err)
	}
	if len(res.Rows) != 34 {
		t.Fatalf("fixture = %d rows, want 5 transit + 28 sky + 1 return", len(res.Rows))
	}
	return res.Rows
}

type timelineCall struct {
	req lookup.TimelineRequest
}

// stubTimeline replaces the spawn seam. answer decides each call's result;
// nil answers every call with the October week.
func stubTimeline(t *testing.T, answer func(lookup.TimelineRequest) (lookup.TimelineResult, error)) *[]timelineCall {
	defaultClock(t)
	t.Helper()
	calls := &[]timelineCall{}
	week := weekRows(t)
	if answer == nil {
		answer = func(lookup.TimelineRequest) (lookup.TimelineResult, error) {
			return lookup.TimelineResult{Rows: week, Commands: []string{"astrolog -z -2 -qm 10 2026 -dm"}}, nil
		}
	}
	old := computeTimeline
	computeTimeline = func(_ context.Context, req lookup.TimelineRequest) (lookup.TimelineResult, error) {
		*calls = append(*calls, timelineCall{req: req})
		return answer(req)
	}
	timelineStubbed = true
	t.Cleanup(func() { computeTimeline, timelineStubbed = old, false })
	return calls
}

// timelineStubbed is whether the running test already replaced the spawn
// seam; loadedTimeline installs the week only when it has not.
var timelineStubbed bool

type textCall struct {
	kind string // "bodies" or "aspects": which panel's compute this was
	when time.Time
}

// stamp turns a moment into a number a stub can carry in its result, so the
// COMMITTED Bodies and Aspects state can be traced back to the moment each
// was computed for — not merely to the fact that some compute ran.
func stamp(when time.Time) float64 { return float64(when.Truncate(time.Minute).Unix()%100000) / 1000 }

// assertEnteredChartRowRecomputed proves, after Enter on a CHART row, that
// Bodies and Aspects each committed a result, independently: one can be dropped
// without the other. Enter moves the cursor to that row's chart (C1 of
// the specification) and stores now as the visitor, so the view is "now in
// k" (D40): Bodies commits now's positions stamped with the row's moment, read
// in that chart's houses (its natal compute, the stub's Moon at 7), the
// floating slot's moment is the row's, and Aspects still commits the cross
// compute stamped with that moment.
func assertEnteredChartRowRecomputed(t *testing.T, m Model, when time.Time, what string) {
	t.Helper()
	if len(m.chart.positions) == 0 || m.chart.positions[0].Body != aspect.Sun || m.chart.positions[0].Longitude != stamp(when) {
		t.Errorf("%s: Bodies did not commit now's bodies at the row's moment: %+v", what, m.chart.positions)
	}
	if len(m.chart.natalPositions) == 0 || m.chart.natalPositions[0].Body != aspect.Moon || m.chart.natalPositions[0].Longitude != 7 {
		t.Errorf("%s: Bodies is not read in the row's chart (its natal compute): %+v", what, m.chart.natalPositions)
	}
	if got := m.slots.Floating().Time; !got.Equal(when) {
		t.Errorf("%s: the floating moment is %v, want the row's %v", what, got, when)
	}
	if len(m.chart.crossAspects) == 0 || float64(m.chart.crossAspects[0].Orb) != stamp(when) {
		t.Errorf("%s: Aspects did not commit a result for %v: %+v", what, when, m.chart.crossAspects)
	}
}

// stubAnyWheel stubs both wheel renderers — single chart and the comparison
// bi-wheel, which is what the fixture's selected pair draws — and returns the
// moment each render was asked for, in order.
func stubAnyWheel(t *testing.T) *[]time.Time {
	t.Helper()
	calls := &[]time.Time{}
	single, overlay := renderWheelImage, renderOverlayWheelImage
	t.Cleanup(func() { renderWheelImage, renderOverlayWheelImage = single, overlay })
	renderWheelImage = func(_ context.Context, when time.Time, _ location.PlaceInfo, _ lookup.Lens, _ lookup.WheelHighlight) ([]byte, error) {
		*calls = append(*calls, when)
		return []byte("BM-wheel"), nil
	}
	renderOverlayWheelImage = func(_ context.Context, _ store.AnchorChart, when time.Time, _ location.PlaceInfo, _ lookup.Lens) ([]byte, error) {
		*calls = append(*calls, when)
		return []byte("BM-overlay"), nil
	}
	return calls
}

// stubTextCompute answers every chart compute at once, recording the moment
// it was asked for, so a jump's recompute can be observed without Astrolog.
func stubTextCompute(t *testing.T) *[]textCall {
	t.Helper()
	calls := &[]textCall{}
	natal, transits, overlay := firstCallNatalPositions, firstCallTransits, overlayCrossAspects
	t.Cleanup(func() { firstCallNatalPositions, firstCallTransits, overlayCrossAspects = natal, transits, overlay })
	firstCallTransits = func(_ context.Context, _ *lookup.Probers, when time.Time, _ location.PlaceInfo, _ lookup.Lens) (lookup.Result, error) {
		*calls = append(*calls, textCall{kind: "bodies", when: when})
		return lookup.Result{Positions: []aspect.Position{{Body: aspect.Sun, Longitude: stamp(when)}}}, nil
	}
	firstCallNatalPositions = func(context.Context, *lookup.Probers, store.AnchorChart, lookup.Lens) (lookup.Result, error) {
		*calls = append(*calls, textCall{kind: "natal"})
		return lookup.Result{Positions: []aspect.Position{{Body: aspect.Moon, Longitude: 7}}}, nil
	}
	overlayCrossAspects = func(_ context.Context, _ lookup.Lens, _ store.AnchorChart, when time.Time, _ location.PlaceInfo) (lookup.OverlayAspects, error) {
		*calls = append(*calls, textCall{kind: "aspects", when: when})
		return lookup.OverlayAspects{Aspects: []lookup.OverlayAspect{{Transit: aspect.Sun, Natal: aspect.Moon, Kind: aspect.Conjunction, Orb: aspect.Orb(stamp(when))}}}, nil
	}
	return calls
}

// timelineModel is a ready 132x40 workspace in Amsterdam: the natal chart
// anchored, the floating slot at 12:00 CEST on 17 October 2026 and focused,
// Chart the focused panel, nothing searched yet.
func timelineModel(t *testing.T, cfg Config) Model {
	t.Helper()
	m := NewModelWithClock(cfg, FrozenClock(testFrozenTime()))
	slots, err := store.NewSlots(tlFloatingTime, testPlaceAmsterdam())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	if err := addAnchor(slots, tlNatalWhen, testPlaceAmsterdam()); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}
	m.slots = slots
	m.cursorIdx = len(slots.All()) - 1
	m.place = testPlaceAmsterdam()
	m.targetFloatingTime = slots.Floating().Time
	m.startup = startupReady
	m.focused = PanelChart
	m.width, m.height = 144, 40
	// The cursor sits on ② (the floating slot); its pair tab is pre-selected
	// on arrival (selected-charts-compare C1a).
	return m.noteArrival()
}

// syncTimelineVisited asks for the timeline's search the way an operator who
// opened the timeline tab and went back to the tab they were on would have:
// the search is owed only while the timeline tab is active
// (the specification rule 3), so a fixture that wants the
// list loaded under another visible tab visits the timeline for the sync and
// restores the choice afterwards.
// detailsTimelineModel is timelineModel with room for Details beside the left
// column, for the tests that read the timeline or a report off it.
func detailsTimelineModel(t *testing.T, cfg Config) Model {
	t.Helper()
	m := timelineModel(t, cfg)
	m.width = max(m.width, dualColumnMinWidth(m.shownCount()))
	return m
}

func syncTimelineVisited(m Model) (Model, tea.Cmd) {
	before, had := m.details.tabs[m.focused]
	m = m.withDetailsTab(m.focused, TabDetailsTimeline)
	m, cmd := m.syncTimeline()
	if had {
		return m.withDetailsTab(m.focused, before), cmd
	}
	tabs := make(map[PanelID]TabID, len(m.details.tabs))
	for k, v := range m.details.tabs {
		if k != m.focused {
			tabs[k] = v
		}
	}
	m.details.tabs = tabs
	return m, cmd
}

// syncTimelineShown is syncTimelineVisited for a fixture whose operator stays
// on the timeline tab: the tests after it change search inputs and expect the
// visible timeline to follow (rule 4 of the specification).
func syncTimelineShown(m Model) (Model, tea.Cmd) {
	m = m.withDetailsTab(m.focused, TabDetailsTimeline)
	return m.syncTimeline()
}

// loadedTimeline is timelineModel with its first search run and committed
// through the real coordinator path.
func loadedTimeline(t *testing.T, cfg Config) Model {
	t.Helper()
	if !timelineStubbed {
		stubTimeline(t, nil)
	}
	m := timelineModel(t, cfg)
	// Details, which holds the timeline, needs room beside the left column.
	m.width = max(m.width, dualColumnMinWidth(m.shownCount()))
	m, cmd := syncTimelineVisited(m)
	m = pump(t, m, cmd)
	if !m.timeline.complete || len(m.timeline.rows) != 34 {
		t.Fatalf("fixture timeline did not load: complete=%v rows=%d", m.timeline.complete, len(m.timeline.rows))
	}
	return m
}

// pump is a small tea runtime: it runs every command, delivers each message
// through Update and keeps going with what Update returns, until nothing is
// left. A command still running after the round's deadline is a timer — a
// debounce or a copy flash — and is abandoned rather than waited out; every
// compute in these tests is a stub and answers at once.
//
// Abandoned is not forgotten: an abandoned command is waited out when the test
// ends (awaitAbandoned), so it cannot finish during a LATER test and spawn
// through that test's fake Astrolog (bug report of 2026-10-10).
func pump(t *testing.T, m Model, cmds ...tea.Cmd) Model {
	t.Helper()
	return pumpWithin(t, m, 400*time.Millisecond, cmds...)
}

// pumpPatient is pump for a test whose commands do real work (a live Astrolog
// call, a spawn through the fixture) that a loaded host can keep past pump's
// 400 ms: every command is waited for, up to patientWait per round.
func pumpPatient(t *testing.T, m Model, cmds ...tea.Cmd) Model {
	t.Helper()
	return pumpWithin(t, m, patientWait, cmds...)
}

const patientWait = 20 * time.Second

func pumpWithin(t *testing.T, m Model, wait time.Duration, cmds ...tea.Cmd) Model {
	t.Helper()
	m = pumpHeldWithin(t, m, wait, cmds...)
	// The Details hold is a long timer, abandoned like the others; a pumped
	// model is a settled one, so its quiet window has passed: the hold ends.
	return m.releaseDetailsHold(detailsHoldMsg{id: m.detailsHold.id})
}

// pumpHeld is pump with the Details hold left on, as it is while a
// navigation key still repeats (details_hold_test.go).
func pumpHeld(t *testing.T, m Model, cmds ...tea.Cmd) Model {
	t.Helper()
	return pumpHeldWithin(t, m, 400*time.Millisecond, cmds...)
}

func pumpHeldWithin(t *testing.T, m Model, wait time.Duration, cmds ...tea.Cmd) Model {
	t.Helper()
	return pumpSeen(t, m, wait, nil, cmds...)
}

// pumpSeen is pumpHeldWithin that hands see every model a delivered message
// leaves, so a test can hold each intermediate state to a rule.
func pumpSeen(t *testing.T, m Model, wait time.Duration, see func(Model), cmds ...tea.Cmd) Model {
	t.Helper()
	pending := cmds
	for round := 0; len(pending) > 0; round++ {
		if round > 40 {
			t.Fatal("pump: the model never went quiet")
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
		deadline := roundDeadline(wait, func() int { return n - int(done.Load()) })
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
				if _, quiet := msg.(detailsHoldMsg); quiet {
					// The hold's quiet timer is a long timer even when it
					// is short enough to beat the deadline: the test
					// decides when the key is let go.
					continue
				}
				tm, next := m.Update(msg)
				m = tm.(Model)
				if see != nil {
					see(m)
				}
				pending = append(pending, next)
			case <-deadline:
				awaitAbandoned(t, chans[i:])
				break collect
			}
		}
	}
	return m
}

// awaitAbandoned waits, when the test ends, for commands a pump stopped
// waiting for. Their messages are dropped, exactly as before; only their
// goroutines are not allowed to outlive the test. t.Cleanup runs these before
// the test's earlier cleanups (its PATH, its temp dirs), so a late spawn still
// lands in this test's own fixture. A command that never returns is given up
// after abandonedWait, with a log line naming the wait.
func awaitAbandoned(t *testing.T, chans []chan tea.Msg) {
	t.Cleanup(func() {
		limit := time.After(abandonedWait)
		for _, ch := range chans {
			select {
			case <-ch:
			case <-limit:
				t.Logf("pump: an abandoned command was still running %v after the test", abandonedWait)
				return
			}
		}
	})
}

const abandonedWait = 30 * time.Second

func tpress(t *testing.T, m Model, k tea.KeyMsg) (Model, tea.Cmd) {
	t.Helper()
	tm, cmd := m.Update(k)
	return tm.(Model), cmd
}

var (
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyEsc   = tea.KeyMsg{Type: tea.KeyEsc}
	keyUp    = tea.KeyMsg{Type: tea.KeyUp}
	keyDown  = tea.KeyMsg{Type: tea.KeyDown}
	keyPgDn  = tea.KeyMsg{Type: tea.KeyPgDown}
	keyPgUp  = tea.KeyMsg{Type: tea.KeyPgUp}
	keyTab   = tea.KeyMsg{Type: tea.KeyTab}
	keyRight = tea.KeyMsg{Type: tea.KeyRight}
	keyLeft  = tea.KeyMsg{Type: tea.KeyLeft}
)

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// onTimeline is a loaded model with the timeline tab chosen and Details
// attached from Chart.
func onTimeline(t *testing.T, cfg Config) Model {
	t.Helper()
	m := loadedTimeline(t, cfg)
	m, _ = tpress(t, m, runes("0"))
	m, _ = tpress(t, m, keyEnter)
	if !m.details.attached || m.activeTabID(PanelDetails) != TabDetailsTimeline {
		t.Fatalf("fixture: attached=%v tab=%s", m.details.attached, m.activeTabID(PanelDetails))
	}
	return homedTimeline(t, m)
}

// homeKeys walks the cursor to the first row with real keys. Attaching puts the
// cursor on the nearest event (the specification rule 4); most
// of these tests were written when it started on row 0, and count from there.
func homeKeys() []tea.KeyMsg {
	keys := make([]tea.KeyMsg, 40)
	for i := range keys {
		keys[i] = runes("k")
	}
	return keys
}

func homedTimeline(t *testing.T, m Model) Model {
	t.Helper()
	for _, k := range homeKeys() {
		m, _ = tpress(t, m, k)
	}
	if m.details.cursor != 0 {
		t.Fatalf("fixture: cursor %d after homing", m.details.cursor)
	}
	return m
}

// detailsBodyRows is the Details panel's inner rows, ANSI-stripped, border
// columns removed and trailing padding trimmed — what the operator reads.
func detailsBodyRows(box string) []string {
	lines := strings.Split(ansi.Strip(box), "\n")
	var out []string
	for _, l := range lines[1 : len(lines)-1] {
		r := []rune(l)
		out = append(out, strings.TrimRight(string(r[1:len(r)-1]), " "))
	}
	return out
}

func renderDetails(m Model) string {
	r := m.geometry()[PanelDetails]
	return renderDetailsPanel(r.w, r.h, (m.detailsFrame()).state.attached, m.cfg, m.detailsFrame())
}

// ---- Recomputation -------------------------------------------------------

// C43 (amended by the specification: "first declares"
// became "first makes active"): the first time the timeline tab is the active
// one for a set of inputs, the searches run for exactly those inputs —
// reached through a text commit under the report tab (nothing runs), then
// through choosing the tab, the way a real session first reaches it.
func TestTimelineRunsOnItsFirstDeclaration(t *testing.T) {
	calls := stubTimeline(t, nil)
	m := timelineModel(t, Config{Accessible: true})
	m, cmd := applyTextCommit(m, FirstFrameReadyMsg{TransitTime: tlFloatingTime})
	m = pump(t, m, cmd)
	if len(*calls) != 0 {
		t.Fatalf("searches ran %d times with the report tab active, want none", len(*calls))
	}
	m, cmd = tpress(t, m, runes("0"))
	m = pump(t, m, cmd)
	if len(*calls) != 1 {
		t.Fatalf("searches ran %d times on the first declaration, want once", len(*calls))
	}
	got := (*calls)[0].req
	// Changed by selected-charts-compare P4/C10: a comparing timeline asks for
	// its anchored slots as Natals (numeral 1 here), not as the one Natal.
	if !got.When.Equal(tlFloatingTime) || got.Place != testPlaceAmsterdam() || len(got.Natals) != 1 || got.Natals[0] != (lookup.TimelineNatal{Numeral: 1, Chart: tlNatal()}) {
		t.Errorf("searched %+v, want the floating slot's time and place and the anchored chart", got)
	}
	if len(m.timeline.rows) != 34 {
		t.Errorf("rows = %d after the first search, want the 34 it returned", len(m.timeline.rows))
	}
}

// C10: each search input, changed alone, re-runs the searches.
func TestTimelineRecomputesWhenASearchInputChanges(t *testing.T) {
	calls := stubTimeline(t, nil)
	commit := func(m Model, when time.Time) Model {
		m, cmd := applyTextCommit(m, FirstFrameReadyMsg{TransitTime: when})
		return pump(t, m, cmd)
	}
	berlin := testPlaceBerlin()
	for _, tc := range []struct {
		name   string
		change func(*testing.T, Model) Model
	}{
		{"the month", func(t *testing.T, m Model) Model { return commit(m, tlFloatingTime.AddDate(0, 1, 0)) }},
		{"the year", func(t *testing.T, m Model) Model { return commit(m, tlFloatingTime.AddDate(1, 0, 0)) }},
		{"the floating zone", func(t *testing.T, m Model) Model {
			slots, _ := store.NewSlots(tlFloatingTime, berlin)
			_ = addAnchor(slots, tlNatalWhen, testPlaceAmsterdam())
			m.slots = slots
			return commit(m, tlFloatingTime)
		}},
		{"whether a slot is anchored", func(t *testing.T, m Model) Model {
			slots, _ := store.NewSlots(tlFloatingTime, testPlaceAmsterdam())
			m.slots, m.cursorIdx = slots, 0
			return commit(m, tlFloatingTime)
		}},
		{"which chart is anchored", func(t *testing.T, m Model) Model {
			setAnchor(m, store.AnchorChart{When: time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC), Place: berlin})
			return commit(m, tlFloatingTime)
		}},
		{"the pair is deselected", func(t *testing.T, m Model) Model {
			m.slots.SetComparisonSelected(false)
			return commit(m, tlFloatingTime)
		}},
		{"another library chart with the same moment and place", func(t *testing.T, m Model) Model {
			m, _ = applyChartResolved(m, ChartResolvedMsg{
				Chart: store.NamedChart{ID: "twin", Name: "Constructed Twin", Place: testPlaceAmsterdam()},
				When:  tlNatalWhen, Gen: m.pickGen})
			if anchorHandle(m) != tlNatal() || m.focusedChart() != tlNatal() {
				t.Fatal("fixture: the twin chart does not share the moment and place")
			}
			// A pick adds the twin below the first chart; it is the baseline —
			// the chart the comparison and the timeline use — once the
			// astrologer selects it and deselects the first (the specification M1).
			m.slots.ToggleSlotSelection(1)
			m.slots.ToggleSlotSelection(0)
			return commit(m, tlFloatingTime)
		}},
		{"the anchored chart's own place", func(t *testing.T, m Model) Model {
			moved := tlNatal()
			moved.Place.Lon += 1
			setAnchor(m, moved)
			return commit(m, tlFloatingTime)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := loadedTimeline(t, Config{Accessible: true})
			// The searches follow the inputs while the timeline tab is on screen.
			m = m.withDetailsTab(PanelChart, TabDetailsTimeline)
			before := len(*calls)
			m = tc.change(t, m)
			if got := len(*calls) - before; got != 1 {
				t.Errorf("changing %s ran the searches %d times, want once", tc.name, got)
			}
		})
	}
}

// C25: moving the floating slot inside the same month — by day, hour or
// minute, through the real step keys — never re-runs the searches.
func TestTimelineDoesNotRecomputeWithinTheSameInputs(t *testing.T) {
	calls := stubTimeline(t, nil)
	stubTextCompute(t)
	m := loadedTimeline(t, Config{Accessible: true})
	before := len(*calls)
	for _, k := range []string{"d", "D", "h", "H", "m", "M", "w"} {
		var cmd tea.Cmd
		m, cmd = tpress(t, m, runes(k))
		m = pump(t, m, cmd)
	}
	if got := m.slots.Floating().Time; got.Month() != time.October || got.Equal(tlFloatingTime) {
		t.Fatalf("fixture: the steps did not move the slot within October: %v", got)
	}
	if len(*calls) != before {
		t.Errorf("stepping within October re-ran the searches %d times", len(*calls)-before)
	}
}

// ---- The Details panel ---------------------------------------------------

// C11: report and timeline, for every subject, in every focus state,
// attached or not, pair selected or not. These models have the cursor on the
// floating slot, which always has the Timeline tab (alone or in a group);
// which other cursors have it is TestTimelineTabFollowsTheGroup.
func TestDetailsAlwaysDeclaresTheTimelineTab(t *testing.T) {
	for _, paired := range []bool{false, true} {
		for _, focus := range focusablePanels() {
			for _, attached := range []bool{false, true} {
				for _, title := range []string{"", "[0] Chart", "[0] Moon 2 in 1"} {
					m := timelineModel(t, Config{Accessible: true})
					m.slots.SetComparisonSelected(paired)
					m.focused, m.details.attached, m.details.title = focus, attached, title
					// Only the Chart panel offers the timeline (the specification acceptance 1).
					want := []TabID{TabDetailsReport}
					if focus == PanelChart {
						want = append(want, TabDetailsTimeline)
					}
					if got := tabIDs(m.panelTabs(PanelDetails)); !reflect.DeepEqual(got, want) {
						t.Errorf("paired=%v focus=%s attached=%v title=%q: tabs %v", paired, focus, attached, title, got)
					}
				}
			}
		}
	}
	// With no workspace at all there is no cursor to offer a Timeline for.
	empty := Model{focused: PanelChart}
	if got := tabIDs(empty.panelTabs(PanelDetails)); !reflect.DeepEqual(got, []TabID{TabDetailsReport}) {
		t.Errorf("empty model: tabs %v", got)
	}
}

// C12: the report tab's body is today's, byte for byte, below the strip. The
// baseline is the four goldens frozen BEFORE this slice (the scroll taste
// test now pins the tabbed frames under their own -tabbed names, so these
// files cannot have been regenerated into agreeing). The strip lives in the
// border, so it consumes no body height: every body row is compared.
func TestDetailsReportTabIsUnchangedBelowTheStrip(t *testing.T) {
	forceColor(t)
	for _, tc := range []struct {
		golden string
		cfg    Config
		offset int
	}{
		{"scroll-details-top", Config{Color: ColorAlways}, 0},
		{"scroll-details-mid", Config{Color: ColorAlways}, 11},
		{"scroll-details-bottom", Config{Color: ColorAlways}, 22},
		{"scroll-details-mid-accessible", Config{Accessible: true}, 11},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", tc.golden+".golden"))
			if err != nil {
				t.Fatal(err)
			}
			// 39 rows give Details exactly the height it had in the baseline,
			// which predates the specification and so spent row 0
			// on a status bar: each baseline row sits one lower.
			// The baseline Details is 66 cells wide at x=66 on a 132-wide
			// terminal. Details now needs 80 cells, so the same report sits at
			// x=64 in a 144-wide frame (two shown) and is wider than the
			// baseline: what is compared is the cells both frames draw, the
			// 62 inside the baseline's border less the last, where the
			// baseline's cut mark stood.
			m := scrollTasteModel(t, tc.cfg, 144, 39, tc.offset, PanelChart)
			if m.activeTabID(PanelDetails) != TabDetailsReport {
				t.Fatal("fixture: the report tab is not active")
			}
			if strings.Contains(string(raw), "[0]") || !strings.Contains(string(raw), "[4] Details") {
				t.Fatalf("%s is not the pre-slice frame: the baseline has been regenerated", tc.golden)
			}
			r := layoutGeometry(144, 39, chartContentLines(m.slots), m.shownCount())[PanelDetails]
			const baselineX = 66 // where the frozen frame drew Details
			const shared = 61    // cells after the border that both frames draw intact
			want := strings.Split(ansi.Strip(string(raw)), "\n")
			got := strings.Split(ansi.Strip(m.View()), "\n")
			col := func(line string) string { return ansi.Cut(line, r.x, r.x+r.w) }
			for y := r.y + 1; y < r.y+r.h; y++ {
				g := ansi.Cut(got[y], r.x+1, r.x+1+shared)
				w := ansi.Cut(want[y+1], baselineX+1, baselineX+1+shared)
				if g != w {
					t.Errorf("row %d differs below the strip:\n got %q\nwant %q", y, g, w)
				}
			}
			if top := col(got[r.y]); !strings.Contains(top, "[0] ") || !strings.Contains(top, "Timeline") {
				t.Errorf("the border is not the tab strip: %q", top)
			}
		})
	}
}

// rowsFromSearch runs lookup's own merge over canned Astrolog output, so a
// fixture row carries the columns the parser cut and not a hand-built struct.
// sky answers the -dm search, transit the -t search; both at one zone.
func rowsFromSearch(t *testing.T, when time.Time, sky, transit string) []lookup.TimelineRow {
	amPmMonthFirst(t)
	t.Helper()
	search := func(_ context.Context, call lookup.BodyCall) (string, string, error) {
		argv := call.Argv()
		cmd := "astrolog " + strings.Join(argv, " ")
		switch {
		case slices.Contains(argv, "-tr"):
			return "", cmd, nil
		case slices.Contains(argv, "-t"):
			return transit, cmd, nil
		}
		return sky, cmd, nil
	}
	req := lookup.TimelineRequest{When: when, Place: testPlaceAmsterdam()}
	if transit != "" {
		req.Natal = tlNatal()
	}
	res, err := lookup.TimelineWith(context.Background(), req, search, stubDefaults)
	if err != nil {
		t.Fatalf("fixture merge: %v", err)
	}
	return res.Rows
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Acceptance 5: the operator's hand-aligned sample, byte for byte modulo
// trailing spaces. Only its 125 sky rows are committed; its transit rows name
// somebody's natal positions.
func TestTimelineRowsAlignLikeTheOperatorsSample(t *testing.T) {
	in := readFixture(t, "timeline-align-in.txt")
	want := strings.Split(strings.TrimRight(readFixture(t, "timeline-align-want.txt"), "\n"), "\n")
	rows := rowsFromSearch(t, tlFloatingTime, in, "")
	if len(rows) != 125 || len(want) != 125 {
		t.Fatalf("rows = %d, want lines = %d, both 125", len(rows), len(want))
	}
	for i, r := range rows {
		if got := strings.TrimRight(alignTimelineRow(r), " "); got != strings.TrimRight(want[i], " ") {
			t.Errorf("row %d:\n got %q\nwant %q", i, got, want[i])
		}
	}
}

// Acceptances 2 and 3 on the C54 transit grammars, aligned by hand to the
// column rule, and a body wider than its column, which shifts the row right
// rather than being cut.
func TestTimelineTransitRowsAndWideBodiesAlign(t *testing.T) {
	transit := " 5/ 6/2026  9:13am trans    Moon (Cap) Con natal (Cap) Moon (Lunar Return)\n" +
		" 5/14/2026  7:20pm trans     Sun (Tau) Con natal (Tau) Sun (Solar Return)\n"
	sky := "Tue  8/18/2026  9:34pm Part of Fortune [Pis] <-- Aquarius\n"
	got := map[string]string{}
	// Each line is searched in its own month: the merge keeps only rows that
	// fall inside the month it was asked for.
	for _, r := range rowsFromSearch(t, time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC), "", "10/ 2/2026  5:08am trans Jupiter (Leo) Tri natal [Sag] Neptune\n") {
		got[r.Line] = alignTimelineRow(r)
	}
	for _, r := range rowsFromSearch(t, time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC), "", transit) {
		got[r.Line] = alignTimelineRow(r)
	}
	for _, r := range rowsFromSearch(t, time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC), sky, "") {
		got[r.Line] = alignTimelineRow(r)
	}
	for line, want := range map[string]string{
		"10/ 2/2026  5:08am trans Jupiter (Leo) Tri natal [Sag] Neptune":             "    10/ 2/2026  5:08am trans Jupiter (Leo) Tri natal [Sag] Neptune",
		" 5/ 6/2026  9:13am trans    Moon (Cap) Con natal (Cap) Moon (Lunar Return)": "     5/ 6/2026  9:13am trans    Moon (Cap) Con natal (Cap) Moon (Lunar Return)",
		" 5/14/2026  7:20pm trans     Sun (Tau) Con natal (Tau) Sun (Solar Return)":  "     5/14/2026  7:20pm trans     Sun (Tau) Con natal (Tau) Sun (Solar Return)",
		"Tue  8/18/2026  9:34pm Part of Fortune [Pis] <-- Aquarius":                  "Tue  8/18/2026  9:34pm       Part of Fortune [Pis] <-- Aquarius",
	} {
		if got[line] != want {
			t.Errorf("row %q:\n got %q\nwant %q", line, got[line], want)
		}
	}
}

// C13 (amended 2026-10-03): a row's visible text holds Astrolog's words, in
// Astrolog's order; only the spaces between them are Sextile's. The clip and
// the cursor styling still apply on top — asserted for every grammar.
func TestTimelineRowsKeepAstrologsWords(t *testing.T) {
	// The week has an S/D station but no S/R; Venus stations retrograde on
	// 3 October (measured at -z -2), so that row joins the rendered set.
	rows := append(weekRows(t), rowsFromSearch(t, tlFloatingTime, "Sat 10/ 3/2026  9:15am   Venus <Sco> S/R\n", "")...)
	kinds := map[string]bool{}
	for _, width := range []int{120, 66, 52} {
		for _, attached := range []bool{false, true} {
			st := detailsViewState{tabs: map[PanelID]TabID{PanelChart: TabDetailsTimeline}, attached: attached, cursor: 5}
			// The cursor is on the floating slot, which has the Timeline tab.
			m := timelineModel(t, Config{Color: ColorNever})
			m.details = st
			box := renderDetailsPanel(width, len(rows)+2, (detailsFrame{state: st, strip: m.tabStrips()[PanelDetails], rows: rows}).state.attached, Config{Color: ColorNever}, detailsFrame{state: st, strip: m.tabStrips()[PanelDetails], rows: rows})
			got := detailsBodyRows(box)
			for i, r := range rows {
				// With colours off the attached list opens every row on the
				// cursor column, which is cursor styling too.
				prefix := ""
				if attached {
					prefix = cursorColumn(i == st.cursor, Config{Color: ColorNever})
				}
				want := strings.TrimRight(cutRight(prefix+displayTimelineRow(r, Config{Color: ColorNever}), width-2, Config{Color: ColorNever}), " ")
				if got[i] != want {
					t.Errorf("width %d attached=%v row %d:\n got %q\nwant %q", width, attached, i, got[i], want)
				}
				// A comparing row opens with its numeral (C11); the rest is
				// Astrolog's words.
				words := strings.Fields(strings.TrimPrefix(got[i], prefix))
				if r.Numeral > 0 && len(words) > 0 {
					words = words[1:]
				}
				if width == 120 && !slices.Equal(words, strings.Fields(r.Line)) {
					t.Errorf("row %d words changed:\n got %q\nwant %q", i, got[i], r.Line)
				}
				for _, k := range []string{" trans ", "(Lunar Return)", "-->", "S/D", "S/R", "(Half Moon)", "(v/c ", "(Leo)", "[Ari]", "<Aqu>"} {
					if strings.Contains(r.Line, k) {
						kinds[k] = true
					}
				}
			}
		}
	}
	for _, k := range []string{" trans ", "(Lunar Return)", "-->", "S/D", "S/R", "(Half Moon)", "(v/c ", "(Leo)", "[Ari]", "<Aqu>"} {
		if !kinds[k] {
			t.Errorf("no rendered row carries %q", k)
		}
	}
}

// C14: a failed search for the SAME inputs leaves the visible frame — title
// and body — byte for byte.
func TestTimelineFailureRetainsTheVisibleFrame(t *testing.T) {
	fail := false
	week := weekRows(t)
	stubTimeline(t, func(lookup.TimelineRequest) (lookup.TimelineResult, error) {
		if fail {
			return lookup.TimelineResult{Commands: []string{"astrolog -z -2 -qm 10 2026 -dm"}}, errors.New("astrolog exec: exit status 1")
		}
		return lookup.TimelineResult{Rows: week}, nil
	})
	m := loadedTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, runes("0"))
	before := renderDetails(m)
	fail = true
	m, cmd := m.bumpIdentity(surfaceTimeline) // the same inputs, searched again
	m = pump(t, m, cmd)
	if got := renderDetails(m); got != before {
		t.Fatalf("a failed search changed the frame:\nBEFORE\n%s\nAFTER\n%s", before, got)
	}
}

// C44: once any input has changed, no row of the previous inputs is shown —
// not while the new search runs, and not after it fails.
func TestNoRowsSurviveASearchInputChange(t *testing.T) {
	fail := false
	week := weekRows(t)
	stubTimeline(t, func(lookup.TimelineRequest) (lookup.TimelineResult, error) {
		if fail {
			return lookup.TimelineResult{}, errors.New("astrolog exec: exit status 1")
		}
		return lookup.TimelineResult{Rows: week}, nil
	})
	m := loadedTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, runes("0"))
	fail = true
	// Another client is anchored: the old client's transits must go at once.
	setAnchor(m, store.AnchorChart{When: time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC), Place: testPlaceBerlin()})
	m, cmd := applyTextCommit(m, FirstFrameReadyMsg{TransitTime: tlFloatingTime})
	pending := strings.Join(detailsBodyRows(renderDetails(m)), "\n")
	m = pump(t, m, cmd)
	failed := strings.Join(detailsBodyRows(renderDetails(m)), "\n")
	for _, r := range week {
		if strings.Contains(pending, r.Line) || strings.Contains(failed, r.Line) {
			t.Fatalf("a row of the previous inputs is still on screen: %q", r.Line)
		}
	}
}

// C44 through the paths that really change an input: a new client resolved
// by the chart picker, and a new city from the atlas. Both change the anchor
// or the zone at once, long before their recompute commits, and neither may
// leave the previous client's rows on screen for that interval.
func TestNoRowsSurviveAnAnchorOrCityPick(t *testing.T) {
	week := weekRows(t)
	for name, change := range map[string]func(Model) Model{
		// A pick adds a chart below the first one, which stays the baseline; the
		// baseline — and so the timeline's inputs — changes when the astrologer
		// selects the new chart and deselects the first (the specification M1).
		"chart picker": func(m Model) Model {
			m, _ = applyChartResolved(m, ChartResolvedMsg{
				Chart: store.NamedChart{ID: "another", Name: "Another Constructed", Place: testPlaceBerlin()},
				When:  time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC), Gen: m.pickGen})
			m.slots.ToggleSlotSelection(1)
			m.slots.ToggleSlotSelection(0)
			return m
		},
		"a twin chart": func(m Model) Model {
			m, _ = applyChartResolved(m, ChartResolvedMsg{
				Chart: store.NamedChart{ID: "twin", Name: "Constructed Twin", Place: testPlaceAmsterdam()},
				When:  tlNatalWhen, Gen: m.pickGen})
			m.slots.ToggleSlotSelection(1)
			m.slots.ToggleSlotSelection(0)
			return m
		},
		"atlas": func(m Model) Model {
			m, _ = applyAtlasPicked(m, AtlasPickedMsg{Place: testPlaceBerlin()})
			return m
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := loadedTimeline(t, Config{Accessible: true})
			m, _ = tpress(t, m, runes("0"))
			m = change(m)
			if _, key, ok := m.timelineInputs(); !ok || key == m.timeline.key {
				t.Fatalf("fixture: the %s did not change the search inputs", name)
			}
			shown := strings.Join(detailsBodyRows(renderDetails(m)), "\n")
			for _, r := range week {
				if strings.Contains(shown, r.Line) {
					t.Fatalf("after the %s, the previous inputs' row is on screen: %q", name, r.Line)
				}
			}
			if copied := m.copiedRows(); len(copied) != 0 {
				t.Errorf("after the %s, the content would copy the previous inputs' rows", name)
			}
		})
	}
}

// C41: the failure is on the error lane and names what was refused — the
// line itself when the parser refused one.
func TestTimelineFailureReportsOnTheErrorLane(t *testing.T) {
	defaultClock(t)
	refused := "Astrolog: Value 13 passed to parameter #1 of switch -t out of range."
	_, parseErr := lookup.TimelineWith(context.Background(),
		lookup.TimelineRequest{When: tlFloatingTime, Place: testPlaceAmsterdam()},
		func(context.Context, lookup.BodyCall) (string, string, error) {
			return refused + "\n", "astrolog -dm", nil
		}, stubDefaults)
	if parseErr == nil {
		t.Fatal("fixture: the refusal did not fail")
	}
	stubTimeline(t, func(lookup.TimelineRequest) (lookup.TimelineResult, error) {
		return lookup.TimelineResult{Commands: []string{"astrolog -dm"}}, parseErr
	})
	m := timelineModel(t, Config{Accessible: true})
	m, cmd := syncTimelineVisited(m)
	m = pump(t, m, cmd)
	var lane []string
	for _, l := range m.logLines {
		if l.lane == laneError {
			lane = append(lane, l.text)
		}
	}
	if len(lane) != 1 || !strings.HasPrefix(lane[0], "Couldn't read the timeline.") || !strings.Contains(lane[0], refused) {
		t.Fatalf("error lane = %q, want one timeline failure naming %q", lane, refused)
	}
}

// § Failure modes: a result for inputs the operator has since left is
// discarded — the late search for the previous client never replaces the
// current list, whichever gate catches it.
func TestStaleTimelineResultIsDiscarded(t *testing.T) {
	week := weekRows(t)
	november := []lookup.TimelineRow{{Instant: time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC), Line: "Tue 11/ 3/2026 10:00am    Moon (Leo) Tri (Sag) Sun", Command: "astrolog -dm"}}
	stubTimeline(t, func(req lookup.TimelineRequest) (lookup.TimelineResult, error) {
		if req.When.Month() == time.November {
			return lookup.TimelineResult{Rows: november}, nil
		}
		return lookup.TimelineResult{Rows: week}, nil
	})
	m := timelineModel(t, Config{Accessible: true})
	m, octoberCmd := syncTimelineShown(m) // October's search starts and is not yet delivered
	m, novemberCmd := applyTextCommit(m, FirstFrameReadyMsg{TransitTime: tlFloatingTime.AddDate(0, 0, 20)})
	m = pump(t, m, octoberCmd) // October's result arrives late
	if len(m.timeline.rows) != 0 {
		t.Fatalf("the late October result painted %d rows over November's inputs", len(m.timeline.rows))
	}
	m = pump(t, m, novemberCmd)
	if len(m.timeline.rows) != 1 || m.timeline.rows[0].Line != november[0].Line {
		t.Fatalf("rows = %+v, want November's", m.timeline.rows)
	}

	// The key guard on its own: a result whose run and want are current but
	// whose inputs are not is still dropped.
	stale := refreshResultMsg{surface: surfaceTimeline, runID: m.refresh[surfaceTimeline].runningRunID, wantID: m.refresh[surfaceTimeline].wantID,
		payload: timelinePayload{key: timelineKey{year: 2026, month: time.October, zone: "Europe/Amsterdam"}, rows: week}}
	m, _ = m.commitTimeline(stale)
	if len(m.timeline.rows) != 1 {
		t.Fatalf("a result for other inputs replaced the list: %d rows", len(m.timeline.rows))
	}
}

// ---- Attachment, subject and the tab keys --------------------------------

// C31: the subject follows the ring and the slot cursor; attaching and
// detaching change neither.
func TestDetailsSubjectFollowsTheRingNotAttachment(t *testing.T) {
	m := loadedTimeline(t, Config{Accessible: true})
	want := detailsSubject{panel: PanelChart, slot: m.cursorIdx}
	if m.detailsSubject() != want {
		t.Fatalf("subject = %+v, want %+v", m.detailsSubject(), want)
	}
	m, _ = tpress(t, m, keyTab)
	if got := m.detailsSubject().panel; got != PanelBodies {
		t.Errorf("after tab the subject is %s, want Bodies", got)
	}
	m, _ = tpress(t, m, runes("3"))
	if got := m.detailsSubject().panel; got != PanelAspects {
		t.Errorf("after 3 the subject is %s, want Aspects", got)
	}
	m, _ = tpress(t, m, runes("1"))
	m, _ = tpress(t, m, keyUp)
	if got := m.detailsSubject().slot; got != 0 {
		t.Errorf("after ↑ on Chart the subject slot is %d, want 0", got)
	}
	subject := m.detailsSubject()
	m, _ = tpress(t, m, keyEnter)
	if m.detailsSubject() != subject {
		t.Errorf("attaching moved the subject to %+v", m.detailsSubject())
	}
	// `2` is not here: from attached Details a digit detaches and focuses its
	// panel (the specification rule 7; TestDigitsDetachAndFocus).
	for _, k := range []tea.KeyMsg{keyTab, keyRight, keyDown, runes("j")} {
		m, _ = tpress(t, m, k)
		if m.detailsSubject() != subject {
			t.Errorf("%v while attached moved the subject to %+v", k, m.detailsSubject())
		}
	}
	m, _ = tpress(t, m, keyEsc)
	if m.detailsSubject() != subject {
		t.Errorf("detaching moved the subject to %+v", m.detailsSubject())
	}
}

// C32: attaching never collapses the list or moves off the chosen tab.
func TestAttachingDoesNotCollapseTheTimelineTab(t *testing.T) {
	// Chart focus only: the timeline tab is the Chart's (acceptance 7).
	for _, focus := range []string{"1"} {
		m := loadedTimeline(t, Config{Accessible: true})
		m, _ = tpress(t, m, runes(focus))
		m, _ = tpress(t, m, runes("0"))
		before := m.panelTabs(PanelDetails)
		m, _ = tpress(t, m, keyEnter)
		if !m.details.attached {
			t.Fatalf("focus %s: Enter did not attach", focus)
		}
		if got := m.panelTabs(PanelDetails); !reflect.DeepEqual(tabIDs(got), tabIDs(before)) {
			t.Errorf("focus %s: attaching changed the tabs to %v", focus, tabIDs(got))
		}
		if m.activeTabID(PanelDetails) != TabDetailsTimeline {
			t.Errorf("focus %s: attaching left the timeline tab", focus)
		}
	}
}

// C33: `0` cycles Details' tabs from any ring panel, wrapping, and moves
// neither focus nor attachment.
func TestZeroCyclesDetailsTabsFromAnywhere(t *testing.T) {
	for _, focus := range focusablePanels() {
		m := loadedTimeline(t, Config{Accessible: true})
		m.focused = focus
		wants := []TabID{TabDetailsTimeline, TabDetailsReport, TabDetailsTimeline}
		if focus != PanelChart {
			// One tab outside the Chart: `0` does nothing there
			// (the specification acceptance 2).
			wants = []TabID{TabDetailsReport, TabDetailsReport, TabDetailsReport}
		}
		for _, want := range wants {
			var cmd tea.Cmd
			m, cmd = tpress(t, m, runes("0"))
			if got := m.activeTabID(PanelDetails); got != want {
				t.Errorf("%s: 0 made %s active, want %s", focus, got, want)
			}
			if m.focused != focus || m.details.attached || cmd != nil {
				t.Errorf("%s: 0 moved focus to %s, attached=%v, cmd=%v", focus, m.focused, m.details.attached, cmd != nil)
			}
		}
	}
}

// C34: `]` and `[` cycle the FOCUSED VIEW — Details while attached, the ring
// panel otherwise, and from the Chart panel the Aspects tabs (amendment (a) of
// the view rule, 2026-10-24).
func TestBracketKeysCycleTheFocusedViewsTabs(t *testing.T) {
	m := loadedTimeline(t, Config{Accessible: true})
	if m.focusedView() != PanelAspects {
		t.Fatalf("unattached focused view = %s", m.focusedView())
	}
	m, _ = tpress(t, m, keyEnter)
	if m.focusedView() != PanelDetails {
		t.Fatalf("attached focused view = %s", m.focusedView())
	}
	for _, step := range []struct {
		r    string
		want TabID
	}{{"]", TabDetailsReport}, {"]", TabDetailsTimeline}, {"[", TabDetailsReport}, {"[", TabDetailsTimeline}} {
		// Enter from the Chart already landed on the timeline (acceptance 3).
		m, _ = tpress(t, m, runes(step.r))
		if got := m.activeTabID(PanelDetails); got != step.want {
			t.Errorf("%s made %s active, want %s", step.r, got, step.want)
		}
	}
	m, _ = tpress(t, m, keyEsc)
	before := m.activeTabID(PanelAspects)
	m, _ = tpress(t, m, runes("]"))
	if tabs := m.panelTabs(PanelAspects); len(tabs) > 1 && m.activeTabID(PanelAspects) == before {
		t.Errorf("] on unattached Chart left the Aspects tab on %s", before)
	}
	if m.focused != PanelChart || m.details.attached {
		t.Errorf("] on unattached Chart moved focus to %s, attached=%v", m.focused, m.details.attached)
	}
}

// C36: a tab key reaching a one-tab view returns the model unchanged with no
// command.
func TestTabKeysNoopOnSingleTab(t *testing.T) {
	for _, focus := range focusablePanels() {
		base := loadedTimeline(t, Config{Accessible: true})
		base.focused = focus
		// Amended by selected-charts-compare C1: while comparing, Aspects has
		// two tabs and its brackets cycle (TestBracketsCycleAspectPairs).
		if len(base.panelTabs(focus)) > 1 {
			if focus != PanelAspects {
				t.Errorf("%s declares %d tabs, want 1", focus, len(base.panelTabs(focus)))
			}
			continue
		}
		for _, r := range []string{"[", "]"} {
			if focus == PanelChart {
				// From the Chart panel the keys step the Aspects tabs
				// (TestBracketKeysFromChartChangeTheAspectsTab).
				break
			}
			m, cmd := tpress(t, base, runes(r))
			if cmd != nil || !reflect.DeepEqual(m, base) {
				t.Errorf("%s: %q changed the model or returned a Cmd", focus, r)
			}
		}
		if m, cmd := base.cycleTab(focus, +1); cmd != nil || !reflect.DeepEqual(m, base) {
			t.Errorf("%s: cycleTab changed a one-tab view", focus)
		}
	}
}

// C37: a stored choice that is not declared right now resolves to the first
// tab without being overwritten, so it is honoured again once redeclared.
func TestActiveTabClampsToFirstAndTheChoiceSurvives(t *testing.T) {
	a, b := Tab{ID: "a"}, Tab{ID: "b"}
	if got := activeTabIndex([]Tab{a, b}, "b"); got != 1 {
		t.Errorf("declared choice resolved to %d", got)
	}
	if got := activeTabIndex([]Tab{a}, "b"); got != 0 {
		t.Errorf("undeclared choice resolved to %d, want the first tab", got)
	}
	if got := activeTabIndex([]Tab{a, b}, "b"); got != 1 {
		t.Errorf("redeclared choice resolved to %d, want it back", got)
	}
	m := loadedTimeline(t, Config{Accessible: true})
	m.details.tabs = map[PanelID]TabID{m.focused: "gone"}
	if got := m.activeTabID(PanelDetails); got != TabDetailsReport {
		t.Errorf("unknown Details choice resolved to %s, want report", got)
	}
	if m.details.tabs[m.focused] != "gone" {
		t.Errorf("resolving overwrote the stored choice with %q", m.details.tabs[m.focused])
	}
}

// C38: switching tabs is presentation only.
func TestTabSwitchStartsNoCompute(t *testing.T) {
	m := loadedTimeline(t, Config{Accessible: true})
	before := m.refresh
	for _, k := range []tea.KeyMsg{runes("0"), runes("0"), keyEnter, runes("]"), runes("["), runes("0")} {
		var cmd tea.Cmd
		m, cmd = tpress(t, m, k)
		if cmd != nil {
			t.Errorf("%v returned a Cmd", k)
		}
	}
	for _, s := range []surface{surfaceText, surfaceImage, surfaceDetails, surfaceTimeline} {
		if m.refresh[s].wantID != before[s].wantID || m.refresh[s].cancel != nil {
			t.Errorf("surface %d: want %d -> %d, running=%v", s, before[s].wantID, m.refresh[s].wantID, m.refresh[s].cancel != nil)
		}
	}
}

// C39: Details renders as [0], and the Log keeps no digit.
func TestDetailsRendersAsPanelZeroAndLogHasNoDigit(t *testing.T) {
	m := loadedTimeline(t, Config{Accessible: true})
	view := ansi.Strip(m.View())
	rects := layoutGeometry(m.width, m.height, chartContentLines(m.slots), m.shownCount())
	lines := strings.Split(view, "\n")
	detailsTop := ansi.Cut(lines[rects[PanelDetails].y], rects[PanelDetails].x, rects[PanelDetails].x+rects[PanelDetails].w)
	logTop := ansi.Cut(lines[rects[PanelLog].y], rects[PanelLog].x, rects[PanelLog].x+rects[PanelLog].w)
	if !strings.HasPrefix(detailsTop, "+-[0] ") {
		t.Errorf("Details border = %q, want it to open with [0]", detailsTop)
	}
	if strings.Contains(view, "[4]") {
		t.Error("[4] is still on screen")
	}
	if !strings.HasPrefix(logTop, "+-Log") {
		t.Errorf("Log border = %q, want no digit", logTop)
	}
	if got := m.tabStrips()[PanelDetails].prefix; got != "[0] " {
		t.Errorf("Details strip prefix = %q", got)
	}
}

// C40: `0` never focuses Details, and setFocusedPanel still refuses it.
func TestZeroDoesNotFocusDetails(t *testing.T) {
	for _, focus := range focusablePanels() {
		m := loadedTimeline(t, Config{Accessible: true})
		m.focused = focus
		for i := 0; i < 3; i++ {
			m, _ = tpress(t, m, runes("0"))
			if m.focused != focus {
				t.Errorf("0 moved focus from %s to %s", focus, m.focused)
			}
		}
		if got, cmd := m.setFocusedPanel(PanelDetails); got.focused != focus || cmd != nil {
			t.Errorf("setFocusedPanel(Details) focused %s", got.focused)
		}
	}
}

// ---- Focus and keys ------------------------------------------------------

// C15: Enter from a ring panel attaches Details and is never a focus change.
func TestEnterAttachesDetails(t *testing.T) {
	for _, focus := range focusablePanels() {
		m := loadedTimeline(t, Config{Accessible: true})
		m.focused = focus
		m, cmd := tpress(t, m, keyEnter)
		if !m.details.attached {
			t.Errorf("%s: Enter did not attach", focus)
		}
		if m.focused != focus {
			t.Errorf("%s: Enter moved focus to %s", focus, m.focused)
		}
		if cmd != nil {
			t.Errorf("%s: attaching returned a Cmd", focus)
		}
	}
	// A modal owns Enter: attachment never happens underneath it.
	m := loadedTimeline(t, Config{Accessible: true})
	m.helpOpen = true
	if m, _ = tpress(t, m, keyEnter); m.details.attached {
		t.Error("Enter attached Details under the help modal")
	}
}

// C16: Esc detaches and the focused panel is the subject's, unchanged.
func TestEscDetachesDetails(t *testing.T) {
	for _, digit := range []string{"1", "2", "3"} {
		// q and Left leave exactly as Esc does (time-keys-everywhere rule 4).
		for _, leave := range []tea.KeyMsg{keyEsc, runes("q"), keyLeft} {
			m := loadedTimeline(t, Config{Accessible: true})
			m, _ = tpress(t, m, runes(digit))
			subject := m.focused
			m, _ = tpress(t, m, keyEnter)
			m, cmd := tpress(t, m, leave)
			if m.details.attached {
				t.Errorf("%s: %v left Details attached", digit, leave)
			}
			if m.focused != subject {
				t.Errorf("%s: focus after %v = %s, want %s", digit, leave, m.focused, subject)
			}
			if quits(cmd) {
				t.Errorf("%s: %v quit the app from Details", digit, leave)
			}
		}
	}
}

// C17: attached on the timeline, ↑ ↓ j k move the row cursor and never the
// focused slot.
func TestDetailsAttachedArrowsAndJKMoveTheRowCursor(t *testing.T) {
	m := onTimeline(t, Config{Accessible: true})
	slot := m.cursorIdx
	for _, step := range []struct {
		k    tea.KeyMsg
		want int
	}{{keyDown, 1}, {runes("j"), 2}, {runes("j"), 3}, {keyUp, 2}, {runes("k"), 1}} {
		m, _ = tpress(t, m, step.k)
		if m.details.cursor != step.want {
			t.Errorf("%v: cursor %d, want %d", step.k, m.details.cursor, step.want)
		}
		if m.cursorIdx != slot {
			t.Errorf("%v moved the focused slot", step.k)
		}
	}
}

// C26: attached on the report, ↑ ↓ j k scroll the report one line.
func TestReportTabArrowsAndJKScrollByALine(t *testing.T) {
	m := loadedTimeline(t, Config{Accessible: true})
	m.details.title, m.details.body = "[0] Chart", influenceReport(t)
	m, _ = tpress(t, m, keyEnter)
	// Enter from the Chart lands on the timeline; `]` goes to the report.
	m, _ = tpress(t, m, runes("]"))
	slot := m.cursorIdx
	for _, step := range []struct {
		k    tea.KeyMsg
		want int
	}{{keyDown, 1}, {runes("j"), 2}, {keyUp, 1}, {runes("k"), 0}} {
		m, _ = tpress(t, m, step.k)
		if m.details.scrollOffset != step.want {
			t.Errorf("%v: offset %d, want %d", step.k, m.details.scrollOffset, step.want)
		}
		if m.cursorIdx != slot {
			t.Errorf("%v moved the focused slot", step.k)
		}
	}
}

// C18: unattached, j/k are exactly ↓/↑ in every ring panel's own routing —
// Chart the slot, Bodies its own row cursor, Aspects nothing.
func TestUnattachedArrowsAndJKKeepPerPanelRouting(t *testing.T) {
	stubTextCompute(t)
	stubRawReports(t)
	for _, focus := range focusablePanels() {
		base := loadedTimeline(t, Config{Accessible: true})
		base.focused = focus
		base.chart.positions = fullChartPositions()
		for i, pair := range [][2]tea.KeyMsg{{keyDown, runes("j")}, {keyUp, runes("k")}} {
			// Lists stop at their ends (the specification
			// C12), so each direction starts where it has room to move.
			base := base
			if i == 0 {
				base.cursorIdx, base.bodyCursorIdx = 0, 0
			} else {
				base.cursorIdx, base.bodyCursorIdx = len(base.slots.All())-1, 1
			}
			arrow, _ := tpress(t, base, pair[0])
			letter, _ := tpress(t, base, pair[1])
			if arrow.cursorIdx != letter.cursorIdx || arrow.bodyCursorIdx != letter.bodyCursorIdx {
				t.Errorf("%s: %v and %v disagree: slot %d/%d body %d/%d", focus, pair[0], pair[1],
					arrow.cursorIdx, letter.cursorIdx, arrow.bodyCursorIdx, letter.bodyCursorIdx)
			}
			switch focus {
			case PanelChart:
				if letter.cursorIdx == base.cursorIdx {
					t.Errorf("Chart: %v did not cycle the slot", pair[1])
				}
			case PanelBodies:
				if letter.cursorIdx != base.cursorIdx || letter.bodyCursorIdx == base.bodyCursorIdx {
					t.Errorf("Bodies: %v moved the slot or not the body cursor", pair[1])
				}
			case PanelAspects:
				if letter.cursorIdx != base.cursorIdx || letter.bodyCursorIdx != base.bodyCursorIdx {
					t.Errorf("Aspects: %v moved something", pair[1])
				}
			}
			if letter.details.cursor != base.details.cursor {
				t.Errorf("%s: %v moved the timeline cursor while unattached", focus, pair[1])
			}
		}
	}
}

// C19: Ctrl+PgUp/PgDn scroll the ACTIVE Details tab from every state, attached
// or not (the plain keys page the focused list's cursor instead).
func TestPageKeysScrollTheActiveDetailsTabFromAnyFocus(t *testing.T) {
	ctrlDn := tea.KeyMsg{Type: tea.KeyCtrlPgDown}
	ctrlUp := tea.KeyMsg{Type: tea.KeyCtrlPgUp}
	for _, focus := range focusablePanels() {
		for _, attached := range []bool{false, true} {
			for _, tab := range []TabID{TabDetailsReport, TabDetailsTimeline} {
				if tab == TabDetailsTimeline && focus != PanelChart {
					continue // the timeline tab is the Chart's alone (acceptance 7)
				}
				m := loadedTimeline(t, Config{Accessible: true})
				m.focused, m.details.attached = focus, attached
				m.details.tabs = map[PanelID]TabID{focus: tab}
				m.details.title, m.details.body = "[0] Chart", influenceReport(t)
				m, _ = tpress(t, m, ctrlDn)
				report, rows := m.details.scrollOffset, m.details.rowOffset
				if tab == TabDetailsReport && (report != detailsScrollStep || rows != 0) {
					t.Errorf("%s attached=%v report: ctrl+pgdn moved report %d rows %d", focus, attached, report, rows)
				}
				if tab == TabDetailsTimeline && (rows != detailsScrollStep || report != 0) {
					t.Errorf("%s attached=%v timeline: ctrl+pgdn moved report %d rows %d", focus, attached, report, rows)
				}
				m, _ = tpress(t, m, ctrlUp)
				if m.details.scrollOffset != 0 || m.details.rowOffset != 0 {
					t.Errorf("%s attached=%v %s: ctrl+pgup did not scroll back", focus, attached, tab)
				}
			}
		}
	}
}

// With the Chart panel focused, plain PgUp moves the CHART cursor, exactly as
// the Up arrow does, and leaves the Details report where it was.
func TestPlainPageKeyWithChartFocusMovesTheChartCursor(t *testing.T) {
	m := loadedTimeline(t, Config{Accessible: true})
	m.focused = PanelChart
	m.details.tabs = map[PanelID]TabID{PanelChart: TabDetailsReport}
	m.details.title, m.details.body = "[0] Chart", influenceReport(t)
	if m.cursorIdx != 1 {
		t.Fatalf("fixture: cursor %d, want the floating slot (1)", m.cursorIdx)
	}
	arrow, _ := tpress(t, m, keyUp)
	paged, _ := tpress(t, m, keyPgUp)
	if paged.cursorIdx != 0 || paged.cursorIdx != arrow.cursorIdx {
		t.Errorf("PgUp moved the Chart cursor to %d, the Up arrow to %d, want both 0", paged.cursorIdx, arrow.cursorIdx)
	}
	if paged.details.scrollOffset != 0 || paged.details.rowOffset != m.details.rowOffset {
		t.Errorf("PgUp scrolled Details: report %d rows %d", paged.details.scrollOffset, paged.details.rowOffset)
	}
}

// C20: Enter on a row moves the floating slot to the row's instant — the
// COMMITTED Time, after the compute's result message — and Details stays
// attached, on the timeline, with the cursor where it was.
func TestEnterOnATimelineRowMovesTheFloatingSlotAndStays(t *testing.T) {
	stubTextCompute(t)
	m := onTimeline(t, Config{Accessible: true})
	for i := 0; i < 16; i++ {
		m, _ = tpress(t, m, runes("j"))
	}
	row := m.timeline.rows[16]
	m, cmd := tpress(t, m, keyEnter)
	if !m.targetFloatingTime.Equal(landedAt(row.Instant)) {
		t.Fatalf("target = %v, want the row's instant %v", m.targetFloatingTime, landedAt(row.Instant))
	}
	m = pump(t, m, cmd)
	if got := m.slots.Floating().Time; !got.Equal(landedAt(row.Instant)) {
		t.Fatalf("committed Time = %v, want %v (%q)", got, landedAt(row.Instant), row.Line)
	}
	if !m.details.attached || m.activeTabID(PanelDetails) != TabDetailsTimeline || m.details.cursor != 16 {
		t.Errorf("after the jump: attached=%v tab=%s cursor=%d, want attached on the timeline at row 16",
			m.details.attached, m.activeTabID(PanelDetails), m.details.cursor)
	}
}

// C20 from the ANCHORED slot (review round 4): picking a client focuses it,
// and the jump must still move the floating slot — and, since
// the specification rule 8, the cursor with it, so the screen
// shows the chart that moved (Bodies and Aspects follow the new moment).
func TestEnterOnATimelineRowFromTheAnchoredSlot(t *testing.T) {
	// Only with the pair selected: an unselected anchored chart, or a group
	// without the floating slot, has no Timeline tab to press Enter on.
	for _, paired := range []bool{true} {
		stubTextCompute(t)
		m := loadedTimeline(t, Config{Accessible: true})
		m.slots.SetComparisonSelected(paired)
		for _, k := range append(append([]tea.KeyMsg{keyUp, runes("0"), keyEnter}, homeKeys()...), runes("j"), runes("j")) {
			var cmd tea.Cmd
			m, cmd = tpress(t, m, k)
			m = pump(t, m, cmd)
		}
		if m.focusedIsFloating() {
			t.Fatal("fixture: ↑ did not focus the anchored slot")
		}
		row := m.timeline.rows[2]
		if paired && m.activePairSlot() != nil {
			// S4a A1: an arrival never chooses a pair, whatever is selected.
			t.Fatalf("fixture: the arrival chose a pair: %v", m.activePairSlot())
		}
		m, cmd := tpress(t, m, keyEnter)
		m = pump(t, m, cmd)
		if got := m.slots.Floating().Time; !got.Equal(landedAt(row.Instant)) {
			t.Fatalf("paired=%v: committed floating Time %v, want %v", paired, got, landedAt(row.Instant))
		}
		// Rule 8 of the specification reverses the old "focus
		// stays": the cursor goes to the floating slot, whichever slot the
		// operator picked the row from (operator, 2026-10-09: "if another slot
		// than floating is selected, the focused chart slot needs to change to
		// the floating slot"). The list is left as it was.
		if !m.focusedIsFloating() || !m.details.attached || m.details.cursor != 2 {
			t.Errorf("paired=%v: the jump left the cursor off the floating slot or left the list", paired)
		}
		if len(m.chart.positions) == 0 || m.chart.positions[0].Longitude != stamp(row.Instant) {
			t.Errorf("paired=%v: Bodies is not the floating chart at the row's instant: %+v", paired, m.chart.positions)
		}
		if paired {
			// Rule 2 of the specification reverses the old
			// cross list here: row 2 is a sky row, so the Aspects panel shows
			// the floating chart's own list (operator, 2026-10-12: "when hitting
			// enter in timeline, all main planes must reflect what the
			// 'entered' timeline row shows"). A chart row's cross list is
			// asserted by TestEnterOnAChartRowPicksItsPair.
			if got := m.activeTabID(PanelAspects); got != TabAspects {
				t.Errorf("paired: a sky row left the Aspects tab on %s, want %s", got, TabAspects)
			}
		}
	}
}

// Operator hot-fix, 2026-09-30: with only one slot selected the timeline
// shows no transit or return rows — the natal searches do not run and the
// rows already on screen for the pair disappear at once; selecting the pair
// again, through the real Space key, brings them back.
func TestOneSelectedSlotShowsNoTransitRows(t *testing.T) {
	calls := stubTimeline(t, func(req lookup.TimelineRequest) (lookup.TimelineResult, error) {
		rows := weekRows(t)
		if req.Natal.IsZero() && len(req.Natals) == 0 {
			var sky []lookup.TimelineRow
			for _, r := range rows {
				if !strings.Contains(r.Line, " trans ") {
					sky = append(sky, r)
				}
			}
			return lookup.TimelineResult{Rows: sky}, nil
		}
		return lookup.TimelineResult{Rows: rows}, nil
	})
	stubTextCompute(t)
	stubRawReports(t)
	m := loadedTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, runes("0"))
	transitShown := func(m Model) bool {
		return strings.Contains(strings.Join(detailsBodyRows(renderDetailsPanel(66, 40, (m.detailsFrame()).state.attached, m.cfg, m.detailsFrame())), "\n"), " trans ")
	}
	if !transitShown(m) {
		t.Fatal("fixture: with the pair selected the transit rows are not on screen")
	}
	// Space on the floating slot deselects it: one slot selected.
	m, cmd := tpress(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if m.slots.ComparisonSelected() {
		t.Fatal("fixture: Space did not leave a single slot selected")
	}
	// A change of the compared charts keeps nothing (C44, kept by
	// the specification rule 4): the rows go at once.
	if transitShown(m) {
		t.Fatal("transit rows are still on screen with one slot selected")
	}
	m = pump(t, m, cmd)
	if transitShown(m) {
		t.Fatal("transit rows are still on screen with one slot selected after the re-search")
	}
	last := (*calls)[len(*calls)-1].req
	if !last.Natal.IsZero() || len(last.Natals) != 0 {
		t.Fatalf("with one slot selected the natal searches were requested: %+v %+v", last.Natal, last.Natals)
	}
	if transitShown(m) || len(m.timelineRows()) == 0 {
		t.Fatalf("after the re-search: transit shown=%v rows=%d, want the sky rows alone", transitShown(m), len(m.timelineRows()))
	}
	m, cmd = tpress(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m = pump(t, m, cmd)
	if !m.slots.ComparisonSelected() || !transitShown(m) {
		t.Fatal("selecting the pair again did not bring the transit rows back")
	}
}

// C21: the jump recomputes Bodies and Aspects through the time-step path,
// and refreshes the wheel when — and only when — its preview is open.
func TestTimelineJumpRecomputesThroughTheTimeStepPath(t *testing.T) {
	for _, preview := range []bool{false, true} {
		text := stubTextCompute(t)
		wheel := stubAnyWheel(t)
		stubShowWheel(t)
		m := onTimeline(t, Config{Accessible: true})
		m.previewActive = preview
		// A chart row (numeral 1): Enter on a sky row shows the floating
		// chart's own aspects and runs no cross compute (rule 2 of
		// the specification).
		first := firstChartRow(t, m.timeline.rows)
		for range first {
			m, _ = tpress(t, m, runes("j"))
		}
		row := m.timeline.rows[first]
		textBefore := len(*text)
		m, cmd := tpress(t, m, keyEnter)
		m = pump(t, m, cmd)
		if !m.slots.ComparisonSelected() {
			t.Fatal("fixture: the pair is not selected, so Aspects is not the cross compute")
		}
		kinds := map[string]bool{}
		for _, c := range (*text)[textBefore:] {
			if c.when.Equal(landedAt(row.Instant)) {
				kinds[c.kind] = true
			}
		}
		if !kinds["bodies"] || !kinds["aspects"] {
			t.Errorf("preview=%v: computes for %v = %v, want both the Bodies and the Aspects compute", preview, landedAt(row.Instant), kinds)
		}
		assertEnteredChartRowRecomputed(t, m, landedAt(row.Instant), "jump")
		if preview && (len(*wheel) == 0 || !(*wheel)[len(*wheel)-1].Equal(landedAt(row.Instant))) {
			t.Errorf("preview open: the wheel was not refreshed for the jump: %v", *wheel)
		}
		if !preview && len(*wheel) != 0 {
			t.Errorf("preview closed: the jump rendered the wheel %d times", len(*wheel))
		}
	}
}

// C48 and D18: attached Details owns the keyboard. Ring keys and keys it does
// not name are swallowed. Detached, `y`/`Y` step the year as they ship.
//
// The step keys are deliberately NOT in the swallowed list: C57 and C58, as
// amended by the specification, forward every step key on the
// timeline tab, because the list is cut to the floating slot's month and these
// are the keys that move it; on the report tab they are inert (C59). Their
// assertions are TestAttachedTimelineForwardsTheMonthStep and
// TestTimelineTabForwardsEveryStepKey.
func TestAttachedDetailsOwnsTheKeyboard(t *testing.T) {
	stubTextCompute(t)
	m := onTimeline(t, Config{Accessible: true})
	target, focus := m.targetFloatingTime, m.focused
	// The digits are not swallowed any more: rule 7 of
	// the specification makes 1/2/3 detach and focus
	// (TestDigitsDetachAndFocus).
	for _, k := range []tea.KeyMsg{runes("x"), runes("z"), keyTab, keyRight, runes("t")} {
		m, _ = tpress(t, m, k)
	}
	if !m.targetFloatingTime.Equal(target) || !m.slots.Floating().Time.Equal(target) {
		t.Fatalf("keys reached the time steps while attached: target %v -> %v", target, m.targetFloatingTime)
	}
	if m.focused != focus || !m.details.attached {
		t.Fatalf("keys reached the ring while attached: focus %s attached=%v", m.focused, m.details.attached)
	}
	m, _ = tpress(t, m, keyEsc)
	m, _ = tpress(t, m, runes("y"))
	if want := target.AddDate(1, 0, 0); !m.targetFloatingTime.Equal(want) {
		t.Errorf("detached y: target %v, want a year on, %v", m.targetFloatingTime, want)
	}
}

// C50: while attached, `0`, `[` and `]` are forwarded to the tab cycle.
func TestAttachedDetailsForwardsTheTabKeys(t *testing.T) {
	m := loadedTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, keyEnter)
	for _, step := range []struct {
		k    string
		want TabID
	}{{"0", TabDetailsReport}, {"0", TabDetailsTimeline}, {"]", TabDetailsReport}, {"[", TabDetailsTimeline}} {
		m, _ = tpress(t, m, runes(step.k))
		if got := m.activeTabID(PanelDetails); got != step.want || !m.details.attached {
			t.Errorf("%s while attached: %s attached=%v, want %s", step.k, got, m.details.attached, step.want)
		}
	}
}

// C51: the journey, through real key dispatch only, on both routes of D3/D17
// — Chart-focused with `0` before Enter, and Aspects-focused with Enter
// before `]` — delivering every result message, with the wheel preview open
// and closed.
func TestTimelineJourneyThroughRealKeyDispatch(t *testing.T) {
	for _, route := range []struct {
		name string
		keys []tea.KeyMsg
	}{
		{"chart: 0 then enter", append(append(append([]tea.KeyMsg{runes("1"), runes("0"), keyEnter}, homeKeys()...), downKeys(journeyRow(t))...), keyEnter)},
		{"chart: enter lands on the timeline", append(append(append([]tea.KeyMsg{runes("1"), keyEnter}, homeKeys()...), downKeys(journeyRow(t))...), keyEnter)},
	} {
		for _, preview := range []bool{false, true} {
			t.Run(route.name, func(t *testing.T) {
				text := stubTextCompute(t)
				wheel := stubAnyWheel(t)
				stubShowWheel(t)
				m := loadedTimeline(t, Config{Accessible: true})
				m.previewActive = preview
				textBefore := len(*text)
				for _, k := range route.keys {
					var cmd tea.Cmd
					m, cmd = tpress(t, m, k)
					m = pump(t, m, cmd)
				}
				row := m.timeline.rows[journeyRow(t)]
				if got := m.slots.Floating().Time; !got.Equal(landedAt(row.Instant)) {
					t.Fatalf("preview=%v: committed Time %v, want %v (%q)", preview, got, landedAt(row.Instant), row.Line)
				}
				kinds := map[string]bool{}
				for _, c := range (*text)[textBefore:] {
					if c.when.Equal(landedAt(row.Instant)) {
						kinds[c.kind] = true
					}
				}
				if !kinds["bodies"] || !kinds["aspects"] {
					t.Errorf("preview=%v: computes for the jump = %v, want Bodies and Aspects", preview, kinds)
				}
				assertEnteredChartRowRecomputed(t, m, landedAt(row.Instant), "journey")
				if got := len(*wheel) > 0; got != preview {
					t.Errorf("preview=%v: wheel refreshes = %d", preview, len(*wheel))
				}
				if !m.details.attached || m.activeTabID(PanelDetails) != TabDetailsTimeline {
					t.Errorf("preview=%v: the jump left the timeline", preview)
				}
			})
		}
	}
}

// C52: Details disappearing under an attachment detaches it, with focus on
// the subject's panel and the keyboard back with the ring.
func TestResizeWhileAttachedLeavesNoOrphanedAttachment(t *testing.T) {
	m := loadedTimeline(t, Config{Accessible: true})
	// The timeline is the Chart's tab, so the subject is the Chart.
	m, _ = tpress(t, m, runes("1"))
	m, _ = tpress(t, m, runes("0"))
	m, _ = tpress(t, m, keyEnter)
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = tm.(Model)
	if m.details.attached {
		t.Fatal("Details is hidden and still attached")
	}
	if m.focused != PanelChart {
		t.Fatalf("focus after the hide = %s, want the subject's Chart", m.focused)
	}
	m, _ = tpress(t, m, runes("3"))
	if m.focused != PanelAspects {
		t.Fatal("the ring did not get the keyboard back")
	}
	tm, _ = m.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
	m = tm.(Model)
	if m.details.attached || m.activeTabID(PanelDetails) != TabDetailsReport {
		t.Errorf("after the return: attached=%v tab=%s, want detached on Aspects' own single tab", m.details.attached, m.activeTabID(PanelDetails))
	}
	m, _ = tpress(t, m, runes("1"))
	if m.activeTabID(PanelDetails) != TabDetailsTimeline {
		t.Errorf("back on the Chart the tab is %s, want the timeline chosen before Enter", m.activeTabID(PanelDetails))
	}
}

// C29: the cursor row is always inside the rendered window — after cursor
// moves past its edge and immediately after a page key.
func TestTimelineCursorStaysWithinTheViewport(t *testing.T) {
	m := onTimeline(t, Config{Color: ColorNever})
	visible := m.detailsInnerHeight()
	check := func(what string) {
		t.Helper()
		if c, off := m.details.cursor, m.details.rowOffset; c < off || c >= off+visible {
			t.Fatalf("%s: cursor %d outside [%d, %d)", what, c, off, off+visible)
		}
		raw := strings.Split(renderDetails(m), "\n")
		if drawn := raw[1+m.details.cursor-m.details.rowOffset]; !strings.HasPrefix(drawn, "│"+cursorRowOff) {
			t.Fatalf("%s: the cursor row is not drawn with the cursor treatment: %q", what, drawn)
		}
	}
	for i := 0; i < len(m.timeline.rows); i++ {
		m, _ = tpress(t, m, runes("j"))
		check("walking down")
	}
	for i := 0; i < 5; i++ {
		m, _ = tpress(t, m, keyPgUp)
		check("after pgup")
	}
	for i := 0; i < 5; i++ {
		m, _ = tpress(t, m, keyPgDn)
		check("after pgdn")
	}
}

// C30: the cursor stops at both ends and nothing else moves.
func TestTimelineCursorStopsAtTheEnds(t *testing.T) {
	m := onTimeline(t, Config{Accessible: true})
	slot, target := m.cursorIdx, m.targetFloatingTime
	for _, k := range []tea.KeyMsg{keyUp, runes("k")} {
		m, _ = tpress(t, m, k)
		if m.details.cursor != 0 {
			t.Errorf("%v on the first row moved the cursor to %d", k, m.details.cursor)
		}
	}
	last := len(m.timeline.rows) - 1
	m.details.cursor = last
	m = m.clampTimelineCursor()
	for _, k := range []tea.KeyMsg{keyDown, runes("j")} {
		m, _ = tpress(t, m, k)
		if m.details.cursor != last {
			t.Errorf("%v on the last row moved the cursor to %d", k, m.details.cursor)
		}
	}
	if m.cursorIdx != slot || !m.targetFloatingTime.Equal(target) {
		t.Error("a key at an end moved a slot")
	}
}

// C23: the global additions are keyMap fields carrying exactly these keys,
// named — parity (C42) passes even if a key was never added.
func TestNewGlobalBindingsAreRegisteredByName(t *testing.T) {
	k := defaultKeys()
	for _, tc := range []struct {
		name string
		got  []string
		want []string
	}{
		{"Attach", k.Attach.Keys(), []string{"enter"}},
		{"TabPrev", k.TabPrev.Keys(), []string{"["}},
		{"TabNext", k.TabNext.Keys(), []string{"]"}},
		{"FocusChart", k.FocusChart.Keys(), []string{"1"}},
		{"FocusBodies", k.FocusBodies.Keys(), []string{"2"}},
		{"FocusAspects", k.FocusAspects.Keys(), []string{"3"}},
		{"DetailsTab", k.DetailsTab.Keys(), []string{"0"}},
		{"SlotCycleUp", k.SlotCycleUp.Keys(), []string{"up", "k"}},
		{"SlotCycleDown", k.SlotCycleDown.Keys(), []string{"down", "j"}},
	} {
		if !slices.Equal(tc.got, tc.want) {
			t.Errorf("%s keys = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
	ids := map[BindingID]bool{}
	for _, e := range k.mainBindings() {
		ids[e.ID] = true
	}
	for _, id := range []BindingID{BindingAttach, BindingTabPrev, BindingTabNext} {
		if !ids[id] {
			t.Errorf("binding %d is not in Entries()", id)
		}
	}
}

// C23a: attached Details' own `esc` and `y` add no global binding and leave
// the shipped ones alone — the routing changed, the registry did not.
func TestAttachedDetailsAddsNoNewGlobalBindings(t *testing.T) {
	k := defaultKeys()
	claims := map[string][]BindingID{}
	for _, e := range k.mainBindings() {
		for _, k := range e.Binding.Keys() {
			claims[k] = append(claims[k], e.ID)
		}
	}
	if got := claims["esc"]; !slices.Equal(got, []BindingID{BindingCancelLeader}) {
		t.Errorf("esc is claimed by %v, want exactly CancelLeader", got)
	}
	for _, y := range []string{"y", "Y"} {
		if got := claims[y]; !slices.Equal(got, []BindingID{BindingTimeStepYear}) {
			t.Errorf("%s is claimed by %v, want exactly TimeStepYear", y, got)
		}
	}
	for k := range claims {
		if k == "yy" || k == "y y" {
			t.Errorf("a multi-key binding %q entered the registry", k)
		}
	}
}

// § Failure modes: Enter on the report tab while attached moves nothing.
func TestEnterOnTheReportTabMovesNothing(t *testing.T) {
	m := loadedTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, keyEnter)
	m, _ = tpress(t, m, runes("]")) // Enter from the Chart lands on the timeline
	before := m
	m, cmd := tpress(t, m, keyEnter)
	if cmd != nil || !reflect.DeepEqual(m, before) {
		t.Fatal("Enter on the report tab changed the model or started work")
	}
}

// § Failure modes: Enter with no row under the cursor — an empty month or a
// refused search — leaves Time, focus and the tab alone.
func TestEnterOnAnEmptyTimelineMovesNothing(t *testing.T) {
	for name, answer := range map[string]func(lookup.TimelineRequest) (lookup.TimelineResult, error){
		"empty month": func(lookup.TimelineRequest) (lookup.TimelineResult, error) { return lookup.TimelineResult{}, nil },
		"refused search": func(lookup.TimelineRequest) (lookup.TimelineResult, error) {
			return lookup.TimelineResult{}, errors.New("refused")
		},
	} {
		stubTimeline(t, answer)
		m := timelineModel(t, Config{Accessible: true})
		m, cmd := syncTimelineVisited(m)
		m = pump(t, m, cmd)
		m, _ = tpress(t, m, runes("0"))
		m, _ = tpress(t, m, keyEnter)
		target, focus := m.targetFloatingTime, m.focused
		m, cmd = tpress(t, m, keyEnter)
		if cmd != nil || !m.targetFloatingTime.Equal(target) || !m.slots.Floating().Time.Equal(target) ||
			m.focused != focus || m.activeTabID(PanelDetails) != TabDetailsTimeline || !m.details.attached {
			t.Errorf("%s: Enter moved something", name)
		}
	}
}

// C57: `o`/`O` are forwarded while the timeline is attached, so the month the
// list spans moves from inside the list — the same motion `Esc` then `o`
// performs, which is what the operator affirmed on 2026-09-30, reversing
// pass-7 P2. The other step keys follow (TestTimelineTabForwardsEveryStepKey).
func TestAttachedTimelineForwardsTheMonthStep(t *testing.T) {
	step := func(t *testing.T, k tea.KeyMsg, direction int) {
		t.Helper()
		stubTextCompute(t)
		m := onTimeline(t, Config{Accessible: true})
		if !m.focusedIsFloating() {
			t.Fatal("fixture: the floating slot must be focused for the ordinary step path")
		}
		start, focus := m.targetFloatingTime, m.focused
		want := advanceTimeForPlace(start, m.slots.Floating().Location, 0, direction, 0, 0)

		m, _ = tpress(t, m, k)
		if !m.targetFloatingTime.Equal(want) {
			t.Fatalf("attached %s: target %v, want %v", k, m.targetFloatingTime, want)
		}
		if m.focused != focus || !m.details.attached || m.activeTabID(PanelDetails) != TabDetailsTimeline {
			t.Fatalf("attached %s moved the mode: focus=%s attached=%v tab=%s",
				k, m.focused, m.details.attached, m.activeTabID(PanelDetails))
		}
	}
	t.Run("o steps the month on", func(t *testing.T) { step(t, runes("o"), +1) })
	t.Run("O steps the month back", func(t *testing.T) { step(t, runes("O"), -1) })

	// C58, operator 2026-10-01: `n` (now) and `N` (toggle back) move the
	// floating slot's clock, so on the timeline they move the list — the same
	// reason the month keys are forwarded.
	t.Run("n jumps the list to now", func(t *testing.T) {
		stubTextCompute(t)
		m := onTimeline(t, Config{Accessible: true})
		m, _ = tpress(t, m, runes("o")) // leave "now" so the jump is observable
		moved := m.targetFloatingTime
		m, _ = tpress(t, m, runes("n"))
		if m.targetFloatingTime.Equal(moved) {
			t.Fatalf("attached n did not reach the now jump: still %v", m.targetFloatingTime)
		}
		if !m.targetFloatingTime.Equal(m.clock.Now()) {
			t.Errorf("attached n: target %v, want now %v", m.targetFloatingTime, m.clock.Now())
		}
		if !m.details.attached || m.activeTabID(PanelDetails) != TabDetailsTimeline {
			t.Errorf("attached n moved the mode: attached=%v tab=%s", m.details.attached, m.activeTabID(PanelDetails))
		}
	})
	t.Run("N toggles back to where n left", func(t *testing.T) {
		stubTextCompute(t)
		m := onTimeline(t, Config{Accessible: true})
		m, _ = tpress(t, m, runes("o"))
		before := m.targetFloatingTime
		m, _ = tpress(t, m, runes("n"))
		// Without this the subtest would pass while BOTH keys were swallowed.
		if m.targetFloatingTime.Equal(before) {
			t.Fatalf("n did not move the clock, so N proves nothing: %v", m.targetFloatingTime)
		}
		m, _ = tpress(t, m, runes("N"))
		if !m.targetFloatingTime.Equal(before) {
			t.Errorf("attached N: target %v, want back at %v", m.targetFloatingTime, before)
		}
	})

	// C59 reversed (the specification rule 3,
	// operator 2026-10-13): the report tab forwards the same keys, with the
	// same effect as on the timeline tab.
	t.Run("all of them act on the report tab", func(t *testing.T) {
		for _, k := range []tea.KeyMsg{runes("o"), runes("O"), runes("n"), runes("N")} {
			stubTextCompute(t)
			tl := onTimeline(t, Config{Accessible: true})
			m := tl
			m, _ = tpress(t, m, runes("]")) // to the report tab, still attached
			if m.activeTabID(PanelDetails) != TabDetailsReport {
				t.Fatalf("fixture: tab is %s, want the report tab", m.activeTabID(PanelDetails))
			}
			start := m.targetFloatingTime
			m, _ = tpress(t, m, k)
			tl, _ = tpress(t, tl, k)
			if !m.targetFloatingTime.Equal(tl.targetFloatingTime) {
				t.Errorf("%s on the report tab: %v, want what the timeline tab does, %v", k, m.targetFloatingTime, tl.targetFloatingTime)
			}
			if k.String() != "N" && m.targetFloatingTime.Equal(start) {
				t.Errorf("%s did not reach the clock on the report tab", k)
			}
			if !m.details.attached {
				t.Errorf("%s detached Details from the report tab", k)
			}
		}
	})

	// C48 is unchanged for the year keys: `y` is the copy key, so neither it
	// nor `Y` may reach a time step while attached.
	t.Run("the year keys stay swallowed", func(t *testing.T) {
		stubTextCompute(t)
		m := onTimeline(t, Config{Accessible: true})
		start := m.targetFloatingTime
		for _, k := range []tea.KeyMsg{runes("y"), runes("Y")} {
			m, _ = tpress(t, m, k)
		}
		if !m.targetFloatingTime.Equal(start) {
			t.Fatalf("a year key reached the time step while attached: %v, want %v", m.targetFloatingTime, start)
		}
	})
}

// the specification acceptance 1, 2 and 6: the
// timeline tab is the Chart's alone, and no timeline search is started for a
// tab no panel offers.
func TestDetailsTimelineTabIsTheChartsOnly(t *testing.T) {
	for _, focus := range focusablePanels() {
		m := timelineModel(t, Config{Accessible: true})
		m.focused = focus
		want := []TabID{TabDetailsReport}
		if focus == PanelChart {
			want = append(want, TabDetailsTimeline)
		}
		if got := tabIDs(m.panelTabs(PanelDetails)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s focused: Details tabs %v, want %v", focus, got, want)
		}
	}

	// The choice is remembered per panel: the Chart's survives a visit elsewhere.
	m := loadedTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, runes("0"))
	m, _ = tpress(t, m, runes("2"))
	if got := m.activeTabID(PanelDetails); got != TabDetailsReport {
		t.Errorf("Bodies focused: Details shows %s, want its report", got)
	}
	m, _ = tpress(t, m, runes("1"))
	if got := m.activeTabID(PanelDetails); got != TabDetailsTimeline {
		t.Errorf("back on the Chart: Details shows %s, want the Chart's last choice, the timeline", got)
	}

	// Acceptance 6: with Bodies focused and Details on its report, a model
	// whose timeline was never asked for starts no search, and one whose month
	// changes starts none either; the Chart's first visit pays them.
	calls := stubTimeline(t, nil)
	stubTextCompute(t)
	m = timelineModel(t, Config{Accessible: true})
	m.focused = PanelBodies
	m, cmd := syncTimelineVisited(m)
	m = pump(t, m, cmd)
	if len(*calls) != 0 || m.timeline.keyed {
		t.Fatalf("Bodies focused: syncTimeline asked for %d searches (keyed=%v), want none", len(*calls), m.timeline.keyed)
	}
	m, cmd = tpress(t, m, runes("o"))
	m = pump(t, m, cmd)
	if len(*calls) != 0 {
		t.Errorf("a month step with Bodies focused ran %d timeline searches, want none", len(*calls))
	}
	// The Chart's tab choice is the timeline (rule 3 of
	// the specification: the search is owed to the active
	// tab), so the Chart's first visit pays the searches.
	m = m.withDetailsTab(PanelChart, TabDetailsTimeline)
	m, cmd = tpress(t, m, runes("1"))
	m = pump(t, m, cmd)
	if len(*calls) == 0 || !m.timeline.complete {
		t.Errorf("coming to the Chart ran %d searches (complete=%v), want the timeline loaded", len(*calls), m.timeline.complete)
	}
}

// the specification acceptance 3: Enter from the
// Chart switches the default tab to the timeline and keeps any other choice.
func TestEnterFromChartOpensTheTimeline(t *testing.T) {
	m := loadedTimeline(t, Config{Accessible: true})
	if got := m.activeTabID(PanelDetails); got != TabDetailsReport {
		t.Fatalf("fixture: Chart starts on %s, want the report", got)
	}
	m, _ = tpress(t, m, keyEnter)
	if !m.details.attached || m.activeTabID(PanelDetails) != TabDetailsTimeline {
		t.Errorf("Enter on the report: attached=%v tab=%s, want attached on the timeline", m.details.attached, m.activeTabID(PanelDetails))
	}
	if !m.details.preEnterSet || m.details.preEnter != "" {
		t.Errorf("pre-Enter tab = %q (set %v), want the report remembered", m.details.preEnter, m.details.preEnterSet)
	}

	m = loadedTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, runes("0"))
	m, _ = tpress(t, m, keyEnter)
	if !m.details.attached || m.activeTabID(PanelDetails) != TabDetailsTimeline || m.details.preEnter != TabDetailsTimeline {
		t.Errorf("Enter on a chosen timeline: attached=%v tab=%s remembered=%q, want the timeline kept and remembered",
			m.details.attached, m.activeTabID(PanelDetails), m.details.preEnter)
	}

	// From Bodies or Aspects Enter attaches on their report, as today.
	for _, focus := range []string{"2", "3"} {
		m = loadedTimeline(t, Config{Accessible: true})
		m, _ = tpress(t, m, runes(focus))
		m, _ = tpress(t, m, keyEnter)
		if !m.details.attached || m.activeTabID(PanelDetails) != TabDetailsReport || m.details.preEnterSet {
			t.Errorf("focus %s: Enter gave attached=%v tab=%s remembered=%v, want attached on the report", focus, m.details.attached, m.activeTabID(PanelDetails), m.details.preEnterSet)
		}
	}
}

// the specification acceptance 4 and 5: every way out
// of attached Details puts back the tab that was active before Enter,
// whatever was chosen in between.
func TestLeavingDetailsPutsBackTheTab(t *testing.T) {
	exits := []struct {
		name  string
		key   tea.KeyMsg
		focus PanelID
	}{
		{"esc", keyEsc, PanelChart}, {"q", runes("q"), PanelChart}, {"left", tea.KeyMsg{Type: tea.KeyLeft}, PanelChart},
		{"digit 2", runes("2"), PanelBodies}, {"digit 1", runes("1"), PanelChart},
	}
	for _, exit := range exits {
		for _, preselected := range []bool{false, true} {
			name := exit.name + "/report"
			want := TabDetailsReport
			if preselected {
				name, want = exit.name+"/timeline", TabDetailsTimeline
			}
			t.Run(name, func(t *testing.T) {
				m := loadedTimeline(t, Config{Accessible: true})
				if preselected {
					m, _ = tpress(t, m, runes("0"))
				}
				m, _ = tpress(t, m, keyEnter)
				if m.activeTabID(PanelDetails) != TabDetailsTimeline {
					t.Fatalf("Enter did not land on the timeline: %s", m.activeTabID(PanelDetails))
				}
				// Acceptance 5: a tab chosen while attached is replaced on leaving.
				if !preselected {
					m, _ = tpress(t, m, runes("0"))
					if m.activeTabID(PanelDetails) != TabDetailsReport {
						t.Fatalf("fixture: 0 while attached did not reach the report")
					}
					m, _ = tpress(t, m, runes("0"))
				}
				m, cmd := tpress(t, m, exit.key)
				m = pump(t, m, cmd)
				if m.details.attached {
					t.Fatalf("%s did not leave attached Details", exit.name)
				}
				if m.focused != exit.focus {
					t.Fatalf("%s left focus on %s, want %s", exit.name, m.focused, exit.focus)
				}
				got := m.details.tabs[PanelChart]
				if got == "" {
					got = TabDetailsReport // no stored choice is the first tab
				}
				if got != want {
					t.Errorf("after %s the Chart's Details tab is %q, want %q", exit.name, got, want)
				}
				m, _ = tpress(t, m, runes("1"))
				if got := m.activeTabID(PanelDetails); got != want {
					t.Errorf("after %s the Chart shows %s, want %s", exit.name, got, want)
				}
			})
		}
	}
}
