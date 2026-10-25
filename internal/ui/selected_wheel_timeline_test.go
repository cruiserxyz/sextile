package ui

import (
	"context"
	"errors"
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// the specification, phase C: the wheel and the timeline of the
// selected set. Every chart is a constructed synthetic (listWorkspace).

// wheelCalls records what the three wheel producers were asked.
type wheelCalls struct {
	mu      sync.Mutex
	single  []time.Time
	overlay []overlayCall
	rings   [][]lookup.WheelRing
}

type overlayCall struct {
	chart store.AnchorChart
	when  time.Time
	place location.PlaceInfo
}

func (c *wheelCalls) total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.single) + len(c.overlay) + len(c.rings)
}

func (c *wheelCalls) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.single, c.overlay, c.rings = nil, nil, nil
}

// stubWheels replaces the three wheel producers with recorders that answer at
// once, and the preview publisher with a counter of what reached the screen.
func stubWheels(t *testing.T) (*wheelCalls, *[][]byte) {
	t.Helper()
	calls := &wheelCalls{}
	published := &[][]byte{}
	oldS, oldO, oldM, oldShow := renderWheelImage, renderOverlayWheelImage, renderMultiWheelImage, showWheel
	t.Cleanup(func() {
		renderWheelImage, renderOverlayWheelImage, renderMultiWheelImage, showWheel = oldS, oldO, oldM, oldShow
	})
	renderWheelImage = func(_ context.Context, when time.Time, _ location.PlaceInfo, _ lookup.Lens, _ lookup.WheelHighlight) ([]byte, error) {
		calls.mu.Lock()
		calls.single = append(calls.single, when)
		calls.mu.Unlock()
		return []byte("single"), nil
	}
	renderOverlayWheelImage = func(_ context.Context, chart store.AnchorChart, when time.Time, place location.PlaceInfo, _ lookup.Lens) ([]byte, error) {
		calls.mu.Lock()
		calls.overlay = append(calls.overlay, overlayCall{chart, when, place})
		calls.mu.Unlock()
		return []byte("overlay"), nil
	}
	renderMultiWheelImage = func(_ context.Context, rings []lookup.WheelRing, _ lookup.Lens) ([]byte, []string, error) {
		calls.mu.Lock()
		calls.rings = append(calls.rings, append([]lookup.WheelRing(nil), rings...))
		calls.mu.Unlock()
		return []byte("rings"), []string{"astrolog rings"}, nil
	}
	showWheel = func(img []byte) (string, error) {
		calls.mu.Lock()
		*published = append(*published, img)
		calls.mu.Unlock()
		return "/tmp/stub.png", nil
	}
	return calls, published
}

func argvLines(m Model) []string {
	var out []string
	for _, l := range m.logLines {
		if l.lane == laneArgv {
			out = append(out, l.text)
		}
	}
	return out
}

// openWheel is Ctrl+X on a model: the preview opens and its first render runs.
func openWheel(t *testing.T, m Model) Model {
	t.Helper()
	m.previewActive = true
	m, cmd := m.dispatchRenderWheel()
	return pump(t, m, cmd)
}

// ringSpawn is the one spawn whose argv holds the switch (-r0, -r3 …).
func ringSpawn(t *testing.T, spawns []spawnRecord, relation string) spawnRecord {
	t.Helper()
	var found []spawnRecord
	for _, s := range spawns {
		if slices.Contains(s.argv, relation) {
			found = append(found, s)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d spawns carry %s, want exactly one: %v", len(found), relation, spawns)
	}
	return found[0]
}

// C6: a selected set of exactly two draws today's bi-wheel (-r0, chart 1 the
// baseline) — the second ring the floating slot at the operator's target when
// ② is floating, or ②'s own moment and place when it is anchored.
func TestWheelPairUsesTodaysBiWheel(t *testing.T) {
	spawns := chartFileCapturingAstrolog(t)
	stubShowWheel(t)

	// ① × now: byte-identical in argv to the call RenderOverlayWheelImage
	// makes for the same two charts, as it always has.
	m := selectedSet(t, 1, true, 1)
	m.base = m.slots.All()[0] // now read in ①: the tab "② in ①"
	m = openWheel(t, m)
	got := spawns()
	pair := ringSpawn(t, got, "-r0")
	if hasArg(pair.argv, "-r3") || len(pair.dats) != 2 || chartFileYear(pair.dats[0]) != "1961" {
		t.Fatalf("pair spawn = %v dats %q, want the baseline then now as a -r0 bi-wheel", pair.argv, pair.dats)
	}
	floating := m.slots.Floating()
	if _, err := lookup.RenderOverlayWheelImage(context.Background(), anchorOf(m.slots.Baseline()), m.floatingImageTime(), floating.Location, m.lens); err != nil {
		t.Fatalf("RenderOverlayWheelImage: %v", err)
	}
	both := spawns()
	if len(both) != 2 {
		t.Fatalf("%d spawns after the direct call, want 2", len(both))
	}
	if a, b := normaliseStage(both[0].argv), normaliseStage(both[1].argv); !reflect.DeepEqual(a, b) {
		t.Errorf("the set's bi-wheel argv differs from today's:\n set   %v\n today %v", a, b)
	}
	if chartFileYear(both[0].dats[1]) != strconv.Itoa(m.floatingImageTime().UTC().Year()) {
		t.Errorf("the second ring is %q, want the floating slot's moment", both[0].dats[1])
	}

	// ① × ② anchored: the second ring is ②'s own chart, not now.
	spawns = chartFileCapturingAstrolog(t)
	m = selectedSet(t, 2, false, 1)
	m.base = m.slots.All()[0] // ② read in ①
	m = openWheel(t, m)
	anchoredPair := ringSpawn(t, spawns(), "-r0")
	if len(anchoredPair.dats) != 2 || chartFileYear(anchoredPair.dats[0]) != "1961" || chartFileYear(anchoredPair.dats[1]) != "1962" {
		t.Errorf("anchored pair staged %q, want the baseline (1961) then ② (1962)", anchoredPair.dats)
	}

	// The base is chart 1: the cursor on ① read in ②, the tab "① in ②",
	// stages ②'s file first, then ①'s.
	spawns = chartFileCapturingAstrolog(t)
	m = selectedSet(t, 2, false, 0)
	m.base = m.slots.All()[1]
	m = openWheel(t, m)
	onPair := ringSpawn(t, spawns(), "-r0")
	if len(onPair.dats) != 2 || chartFileYear(onPair.dats[0]) != "1962" || chartFileYear(onPair.dats[1]) != "1961" {
		t.Errorf("① in ②: staged %q, want ② (1962) then ① (1961)", onPair.dats)
	}
}

// S4a A16 (operator 2026-10-22: "exactly the tab"): while comparing, the wheel
// draws the active tab. A pair is a bi-wheel (-r0), the base passed first and
// the cursor's chart second, however many charts are selected; the chart's
// own tab is the cursor's chart alone, with its highlight. The 3 to 6 ring
// wheel is never asked for.
func TestWheelDrawsTheTab(t *testing.T) {
	for _, anchors := range []int{2, 5} {
		t.Run(fmt.Sprintf("set of %d", anchors+1), func(t *testing.T) {
			spawns := chartFileCapturingAstrolog(t)
			stubShowWheel(t)
			m := selectedSet(t, anchors, true, 0) // the cursor on ①, own tab
			if plan, ok := m.wheelPlan(); !ok || plan.mode != wheelSingle {
				t.Fatalf("own tab: plan %+v, want the cursor's chart alone", plan)
			}
			m = openWheel(t, m)
			for _, s := range spawns() {
				for _, a := range s.argv {
					if len(a) == 3 && a[:2] == "-r" && a[2] >= '3' && a[2] <= '6' {
						t.Errorf("a ring wheel was asked for: %v", s.argv)
					}
				}
			}

			// The last anchored chart read in ①: a bi-wheel, ① passed first.
			spawns = chartFileCapturingAstrolog(t)
			m = selectedSet(t, anchors, true, anchors-1)
			m.base = m.slots.All()[0]
			m = openWheel(t, m)
			pair := ringSpawn(t, spawns(), "-r0")
			if len(pair.dats) != 2 || chartFileYear(pair.dats[0]) != "1961" || chartFileYear(pair.dats[1]) != strconv.Itoa(1961+anchors-1) {
				t.Errorf("pair staged %q, want ① (1961) then the cursor's chart (%d)", pair.dats, 1961+anchors-1)
			}
		})
	}

	// The single wheel keeps the cursor chart's highlight on its own tab.
	m := selectedSet(t, 2, true, 1)
	m.slots.ToggleBodySelection(1, aspect.Venus)
	plan, ok := m.wheelPlan()
	if !ok || plan.mode != wheelSingle || !slices.Contains(plan.highlight.Selected, aspect.Venus) {
		t.Errorf("own tab plan %+v, want the cursor's chart alone with Venus highlighted", plan)
	}
}

// C8: with the cursor on a chart outside the selected set, or with fewer than
// two slots selected, the wheel shows the cursor's chart alone.
func TestWheelAloneWhenSingle(t *testing.T) {
	for name, build := range map[string]func() Model{
		"cursor on an unselected chart": func() Model {
			m := selectedSet(t, 2, true, 0)
			m.slots.SetSlotSelected(1, false)
			m.cursorIdx = 1
			return m
		},
		"one slot selected": func() Model {
			m := selectedSet(t, 2, false, 0)
			m.slots.SetSlotSelected(1, false)
			return m
		},
	} {
		t.Run(name, func(t *testing.T) {
			calls, _ := stubWheels(t)
			m := build()
			if m.comparing() {
				t.Fatal("fixture: the model compares")
			}
			want := m.focusedSlot().Time
			openWheel(t, m)
			if len(calls.single) != 1 || !calls.single[0].Equal(want) || len(calls.overlay)+len(calls.rings) != 0 {
				t.Errorf("single %v overlay %d rings %d, want one single render at %v", calls.single, len(calls.overlay), len(calls.rings), want)
			}
		})
	}
}

// Reorder inside the selected set (review round 1, High): ⌃j swaps ② and ③, so
// the timeline's numerals change though the baseline and the pair did not. The
// timeline must follow the new order. (The wheel draws the active tab, which a
// reorder does not change: it asks for no new picture, TestReorderKeepsTheVisitor.)
func TestReorderWithinTheSetRenumbersTheTimeline(t *testing.T) {
	wheels, _ := stubWheels(t)
	timeline := stubTimeline(t, nil)
	stubPairCompute(t)
	m := selectedSet(t, 3, true, 1) // ① A, ② B, ③ C, ④ now; the cursor on ②
	m.cfg = Config{Color: ColorNever}
	m.slots.SetFloatingTime(tlFloatingTime)
	m.targetFloatingTime = tlFloatingTime
	m.focused = PanelChart
	m = openWheel(t, m)
	m = loadTimeline(t, m)
	// The reorder renumbers the timeline while its tab is on screen.
	m = m.withDetailsTab(PanelChart, TabDetailsTimeline)
	if len(wheels.single) != 1 || len(*timeline) != 1 {
		t.Fatalf("fixture: %d wheel renders and %d timeline searches before moving, want 1 and 1", len(wheels.single), len(*timeline))
	}
	oldNatals := (*timeline)[0].req.Natals
	if len(oldNatals) != 3 {
		t.Fatalf("fixture: %d natals, want 3", len(oldNatals))
	}
	wheels.reset()

	var cmd tea.Cmd
	m, cmd = tpress(t, m, keyCtrlJ)
	m = pump(t, m, cmd)

	if wheels.total() != 0 {
		t.Errorf("%d wheel renders after the reorder, want none: ② is still the chart drawn alone", wheels.total())
	}
	if len(*timeline) != 2 {
		t.Fatalf("%d timeline searches in all, want exactly one new one", len(*timeline))
	}
	got := (*timeline)[1].req.Natals
	if len(got) != 3 || got[0] != oldNatals[0] ||
		got[1].Numeral != 2 || got[1].Chart != oldNatals[2].Chart ||
		got[2].Numeral != 3 || got[2].Chart != oldNatals[1].Chart {
		t.Errorf("natals after the reorder = %+v, want ② and ③ swapped from %+v", got, oldNatals)
	}
}

// stagedFailure is the shape of the typed error astrolog raises for an
// unwritable ring file: lookup.StagedRing reads it through its one method.
type stagedFailure struct{ ring int }

func (e stagedFailure) Error() string   { return "astrolog: stage inputs: write failed" }
func (e stagedFailure) StagedRing() int { return e.ring }

// C16 (ui half): a ring whose chart file cannot be staged leaves the picture on
// screen as it was and writes one Log error line naming the slot. Since S4a the
// wheel is the active tab's bi-wheel, so the ring is one of the pair's two.
func TestBiWheelStagingFailureKeepsThePreviousImage(t *testing.T) {
	_, published := stubWheels(t)
	stubPairCompute(t)
	m := selectedSet(t, 2, true, 2) // ① ② ③, the cursor on now
	m.cfg = Config{Color: ColorNever}
	m.base = m.slots.All()[0] // now read in ①
	m = openWheel(t, m)
	if len(*published) != 1 || string((*published)[0]) != "overlay" {
		t.Fatalf("fixture: published %q, want the first bi-wheel picture", *published)
	}

	renderOverlayWheelImage = func(context.Context, store.AnchorChart, time.Time, location.PlaceInfo, lookup.Lens) ([]byte, error) {
		return nil, fmt.Errorf("astrolog: RenderImage: %w", stagedFailure{ring: 2})
	}
	m, cmd := m.setFloatingTarget(m.targetFloatingTime.Add(24 * time.Hour))
	m = pump(t, m, cmd)

	if len(*published) != 1 {
		t.Errorf("%d pictures published, want the previous one only", len(*published))
	}
	if m.wheelPublishPending != nil {
		t.Errorf("a failed render left %q waiting to publish", m.wheelPublishPending)
	}
	lines := errorLines(m)
	if len(lines) != 1 || !strings.Contains(lines[0], "3 now") || !strings.Contains(lines[0], "wheel") {
		t.Errorf("error lane = %q, want one line naming the wheel and slot 3 now", lines)
	}
}

// --- the timeline ---------------------------------------------------------

// mergeSearcher answers the timeline's searches with output that ties on
// purpose: at 10/14 23:26 both selected slots' -t, the sky and both slots'
// -tr all have a row, and slot ① has two -t lines at that instant. fail names
// searches ("-t1961", "-tr1962", "-dm") that fail instead.
// qjYear is the UT year of the moment a `-qj <jd>` names (the specification C1:
// the natal and the staged charts are Julian Days, not calendar fields). These
// fixtures are all after the Gregorian reform, where Go's calendar agrees.
func qjYear(jd string) string {
	v, _ := strconv.ParseFloat(jd, 64)
	return strconv.Itoa(time.Unix(int64(math.Round((v-2440587.5)*86400)), 0).UTC().Year())
}

// chartFileYear is qjYear of a staged chart file's `-qj` line.
func chartFileYear(dat string) string {
	for _, l := range strings.Split(dat, "\n") {
		if jd, ok := strings.CutPrefix(l, "-qj "); ok {
			return qjYear(jd)
		}
	}
	return ""
}

type mergeSearcher struct {
	mu    sync.Mutex
	calls []string // "-t1961", "-dm" … for every spawn
	cmds  []string
	fail  map[string]bool
}

func (s *mergeSearcher) search(_ context.Context, call lookup.BodyCall) (string, string, error) {
	argv := call.Argv()
	cmd := "astrolog " + strings.Join(argv, " ")
	mode, year := "-dm", ""
	switch {
	case slices.Contains(argv, "-tr"):
		mode = "-tr"
	case slices.Contains(argv, "-t"):
		mode = "-t"
	}
	if mode != "-dm" {
		year = qjYear(argv[slices.Index(argv, "-qj")+1])
	}
	s.mu.Lock()
	s.calls = append(s.calls, mode+year)
	s.cmds = append(s.cmds, cmd)
	failing := s.fail[mode+year]
	s.mu.Unlock()
	if argv[1] != "-2" {
		return "", cmd, nil // the CET segment of the month has nothing
	}
	if failing {
		return "", cmd, errors.New("astrolog exec: exit status 1")
	}
	header := "Astrolog 8.00 chart for Tue May 15, 1979  9:30 (ST Zone 0W)   4:54E 52:22N\n\nTransits during Oct 2026 (ST Zone 2E)\n"
	switch mode {
	case "-t":
		out := header + "10/14/2026 23:26 trans     Sun (Lib) Opp natal (Ari) Midheaven\n"
		if year == "1961" {
			out += "10/14/2026 23:26 trans    Mars (Leo) Con natal (Leo) Ascendant\n" +
				"10/16/2026  1:00 trans  Venus (Sco) Con natal (Sco) Sun\n"
		} else {
			out += "10/15/2026  1:00 trans Jupiter (Cap) Con natal (Cap) Moon\n"
		}
		return out, cmd, nil
	case "-tr":
		return header + "10/14/2026 23:26 trans    Moon (Cap) Con natal (Cap) Moon (Lunar Return)\n", cmd, nil
	}
	return "Wed 10/14/2026  2:59    Moon (Sco) --> Sagittarius\n" +
		"Wed 10/14/2026 23:26    Moon (Sag) Tri [Ari] Saturn\n" +
		"Thu 10/15/2026 10:21     Sun (Lib) Sex (Leo) Jupiter\n", cmd, nil
}

// stubTimelineSearch routes the timeline's spawn seam through lookup's own
// merge with a scripted searcher, so the arrangement under test is lookup's.
func stubTimelineSearch(t *testing.T, s *mergeSearcher) {
	defaultClock(t)
	t.Helper()
	old := computeTimeline
	computeTimeline = func(ctx context.Context, req lookup.TimelineRequest) (lookup.TimelineResult, error) {
		return lookup.TimelineWith(ctx, req, s.search, stubDefaults)
	}
	timelineStubbed = true
	t.Cleanup(func() { computeTimeline, timelineStubbed = old, false })
}

// octoberSet is a ready ① ② ③ set (two anchored charts and now), the floating
// slot in the fixture's October, with Details in the layout.
func octoberSet(t *testing.T, cursor int) Model {
	t.Helper()
	m := selectedSet(t, 2, true, cursor)
	// Details keeps the 66 cells these rows were written against: the left
	// column is 52 + 12 per extra chart shown (the specification C5).
	m.width = leftColumnWidth(len(m.slots.SelectedInOrder())) + 80
	m.cfg = Config{Color: ColorNever}
	m.slots.SetFloatingTime(tlFloatingTime)
	m.targetFloatingTime = tlFloatingTime
	m.focused = PanelChart
	return m
}

func loadTimeline(t *testing.T, m Model) Model {
	t.Helper()
	m, cmd := syncTimelineVisited(m)
	// The searches spawn through the fixture: real work, waited for.
	return pumpPatient(t, m, cmd)
}

type rowCheck struct {
	numeral int
	contain string
}

func assertRows(t *testing.T, rows []lookup.TimelineRow, want []rowCheck) {
	t.Helper()
	var got []rowCheck
	for _, r := range rows {
		got = append(got, rowCheck{r.Numeral, r.Line})
	}
	if len(rows) != len(want) {
		t.Fatalf("%d rows, want %d: %+v", len(rows), len(want), got)
	}
	for i, r := range rows {
		if r.Numeral != want[i].numeral || !strings.Contains(r.Line, want[i].contain) {
			t.Errorf("row %d = numeral %d %q, want numeral %d containing %q", i, r.Numeral, r.Line, want[i].numeral, want[i].contain)
		}
	}
}

// C10: the timeline merges every selected anchored slot's -t and -tr with the
// sky, in time order — ties in today's source order (transit, sky, return),
// then by numeral, then each search's own output order — and keeps two slots'
// identical lines at one instant.
func TestTimelineMergesEverySelectedChart(t *testing.T) {
	s := &mergeSearcher{}
	stubTimelineSearch(t, s)
	m := loadTimeline(t, octoberSet(t, 2))

	// Two -t and two -tr searches, one sky search; the CET segment of the month
	// repeats each, which is how a month with a clock change is searched.
	set := map[string]bool{}
	for _, c := range s.calls {
		set[c] = true
	}
	if want := map[string]bool{"-t1961": true, "-t1962": true, "-tr1961": true, "-tr1962": true, "-dm": true}; !reflect.DeepEqual(set, want) {
		t.Fatalf("searches run = %v, want %v", set, want)
	}

	assertRows(t, m.timelineRows(), []rowCheck{
		{0, "Sagittarius"}, // 10/14  2:59
		// 10/14 23:26: transit (① twice in output order, then ②), the sky, the returns.
		{1, "Midheaven"}, {1, "Ascendant"}, {2, "Midheaven"},
		{0, "Saturn"},
		{1, "Lunar Return"}, {2, "Lunar Return"},
		{2, "Moon"},        // 10/15  1:00, ②'s Jupiter
		{0, "Jupiter"},     // 10/15 10:21, the sky
		{1, "natal (Sco)"}, // 10/16  1:00, ①'s Venus
	})
	for i := 1; i < len(m.timelineRows()); i++ {
		if m.timelineRows()[i].Instant.Before(m.timelineRows()[i-1].Instant) {
			t.Errorf("row %d is earlier than the row before it", i)
		}
	}
}

// C11: while comparing a transit or return row carries its slot's numeral in
// its empty weekday column, where a weekday's second letter sits (` ① ` under
// `Wed`); everything else is the row exactly as today's timeline lays it out,
// so every row keeps its width and the columns line up; a sky row carries none.
// The numeral is the only change (operator, 2026-10-04).
func TestTimelineRowsCarryTheirNumeral(t *testing.T) {
	stubTimelineSearch(t, &mergeSearcher{})
	m := loadTimeline(t, octoberSet(t, 0))
	rows := m.timelineRows()
	if len(rows) == 0 {
		t.Fatal("no rows")
	}
	// Today's rendered text of each fixture row, frozen byte for byte from the
	// numeral-less layout on trunk: an independent reference, so a spacing
	// change in alignTimelineRow cannot move the expectation with it.
	frozen := []string{
		"Wed 10/14/2026    2:59          Moon (Sco) --> Sagittarius",
		"    10/14/2026   23:26 trans     Sun (Lib) Opp natal (Ari) Midheaven",
		"    10/14/2026   23:26 trans    Mars (Leo) Con natal (Leo) Ascendant",
		"    10/14/2026   23:26 trans     Sun (Lib) Opp natal (Ari) Midheaven",
		"Wed 10/14/2026   23:26          Moon (Sag) Tri [Ari] Saturn",
		"    10/14/2026   23:26 trans    Moon (Cap) Con natal (Cap) Moon (Lunar Return)",
		"    10/14/2026   23:26 trans    Moon (Cap) Con natal (Cap) Moon (Lunar Return)",
		"    10/15/2026    1:00 trans Jupiter (Cap) Con natal (Cap) Moon",
		"Thu 10/15/2026   10:21           Sun (Lib) Sex (Leo) Jupiter",
		"    10/16/2026    1:00 trans   Venus (Sco) Con natal (Sco) Sun",
	}
	if len(rows) != len(frozen) {
		t.Fatalf("fixture: %d rows, the frozen reference has %d", len(rows), len(frozen))
	}
	for _, cfg := range []Config{{Color: ColorNever}, {Color: ColorAlways}} {
		glyphs := map[int]string{1: "1", 2: "2"}
		if cfg.Color == ColorAlways {
			glyphs = map[int]string{1: "①", 2: "②"}
		}
		numbered, sky := 0, 0
		for i, r := range rows {
			shown := displayTimelineRow(r, cfg)
			if r.Numeral == 0 {
				if shown != frozen[i] {
					t.Errorf("sky row %d shown as %q, frozen today's text %q", i, shown, frozen[i])
				}
				sky++
				if shown != alignTimelineRow(r) {
					t.Errorf("sky row shown as %q, want today's %q", shown, alignTimelineRow(r))
				}
				continue
			}
			numbered++
			glyph := glyphs[r.Numeral]
			if !strings.HasPrefix(frozen[i], "    ") {
				t.Fatalf("fixture: numbered row %d has a weekday: %q", i, frozen[i])
			}
			want := " " + glyph + " " + frozen[i][3:]
			if shown != want {
				t.Errorf("row %d shown as %q, want the numeral in the weekday column: %q", i, shown, want)
			}
			// Same width as the numeral-less row: the columns line up with
			// every sky row by construction.
			if lipgloss.Width(shown) != lipgloss.Width(frozen[i]) {
				t.Errorf("row %d is %d cells, the row without its numeral %d", i, lipgloss.Width(shown), lipgloss.Width(frozen[i]))
			}
			// Under the numeral is Astrolog's own line, word for word.
			if !slices.Equal(strings.Fields(strings.Replace(shown, glyph, "", 1)), strings.Fields(r.Line)) {
				t.Errorf("row %q lost Astrolog's words from %q", shown, r.Line)
			}
		}
		if numbered == 0 || sky == 0 {
			t.Fatalf("fixture: %d numbered and %d sky rows", numbered, sky)
		}
	}

	// And it is what the panel draws.
	m, _ = tpress(t, m, runes("0"))
	panel := strings.Join(detailsBodyRows(renderDetails(m)), "\n")
	for _, want := range []string{" 1  10/14/2026   23:26 trans     Sun (Lib) Opp natal", " 2  10/14/2026   23:26 trans     Sun (Lib) Opp natal", "Wed 10/14/2026    2:59          Moon (Sco) --> Sagittarius"} {
		if !strings.Contains(panel, want) {
			t.Errorf("the Details panel does not show %q:\n%s", want, panel)
		}
	}
}

// C10 keeps two slots' identical lines at one instant, so a mark has to tell
// them apart (review round 1): Space on ①'s row marks ①'s row alone, and `,y`
// copies one row.
func TestMarkingOneOfTwoIdenticalRowsMarksOnlyIt(t *testing.T) {
	stubTimelineSearch(t, &mergeSearcher{})
	m := loadTimeline(t, octoberSet(t, 0))
	m, _ = tpress(t, m, runes("0"))
	m, _ = tpress(t, m, keyEnter)
	m = homedTimeline(t, m)
	rows := m.timelineShown()
	first := -1
	for i := 1; i < len(rows); i++ {
		if rows[i].Numeral != rows[i-1].Numeral && rows[i].Line == rows[i-1].Line && rows[i].Instant.Equal(rows[i-1].Instant) {
			first = i - 1
			break
		}
	}
	if first < 0 {
		t.Fatalf("fixture: no two slots' identical rows at one instant in %+v", rows)
	}
	for i := 0; i < first; i++ {
		m, _ = tpress(t, m, runes("j"))
	}
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if len(m.timeline.marks) != 1 {
		t.Fatalf("Space marked %d marks, want 1", len(m.timeline.marks))
	}
	// The real keys, through Update, to the clipboard seam: what `,y` puts on
	// the clipboard, not what a helper says it would.
	copied := captureYank(t)
	m, _ = tpress(t, m, runes(","))
	m, _ = tpress(t, m, runes("y"))
	if want := displayTimelineRow(rows[first], m.cfg); len(*copied) != 1 || (*copied)[0] != want {
		t.Errorf(",y copied %q, want only ①'s row %q", *copied, want)
	}
	marked := 0
	for _, r := range rows {
		if m.timeline.marks[markOf(r)] {
			marked++
		}
	}
	if marked != 1 {
		t.Errorf("%d rows read as marked, want exactly the one", marked)
	}
}

// C13: with the cursor outside the selected set, or fewer than two slots
// selected, the timeline is the floating slot's month of sky rows only. The
// Timeline tab exists in those cases only on the floating slot (an unselected
// anchored chart, or a group without the floating slot, has no tab), so the
// cursor is on the floating slot in each.
func TestTimelineSkyOnlyWhenSingle(t *testing.T) {
	for name, build := range map[string]func() Model{
		"cursor on the unselected floating slot": func() Model {
			m := octoberSet(t, 0)
			m.slots.SetSlotSelected(2, false)
			m.cursorIdx = 2
			return m
		},
		"only the floating slot selected": func() Model {
			m := octoberSet(t, 0)
			m.slots.SetSlotSelected(0, false)
			m.slots.SetSlotSelected(1, false)
			m.cursorIdx = 2
			return m
		},
		"nothing selected": func() Model {
			m := octoberSet(t, 0)
			for i := range 3 {
				m.slots.SetSlotSelected(i, false)
			}
			m.cursorIdx = 2
			return m
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := &mergeSearcher{}
			stubTimelineSearch(t, s)
			m := build()
			if m.comparing() {
				t.Fatal("fixture: the model compares")
			}
			m = loadTimeline(t, m)
			for _, c := range s.calls {
				if c != "-dm" {
					t.Errorf("a single timeline ran the %s search", c)
				}
			}
			if len(m.timelineRows()) == 0 {
				t.Fatal("no sky rows")
			}
			for _, r := range m.timelineRows() {
				if r.Numeral != 0 || r.Transit {
					t.Errorf("row %+v is not a sky row", r)
				}
				if displayTimelineRow(r, m.cfg) != alignTimelineRow(r) {
					t.Errorf("a sky row gained something: %q", displayTimelineRow(r, m.cfg))
				}
			}
		})
	}
}

// C17a: if the sky search fails, the transit and return rows are still shown
// and one Log error line names the sky search; every command is on the console.
func TestFailedSkySearchKeepsTheTransits(t *testing.T) {
	s := &mergeSearcher{fail: map[string]bool{"-dm": true}}
	stubTimelineSearch(t, s)
	m := loadTimeline(t, octoberSet(t, 1))
	rows := m.timelineRows()
	if len(rows) == 0 {
		t.Fatal("a failed sky search lost the transits")
	}
	for _, r := range rows {
		if r.Numeral == 0 {
			t.Errorf("a sky row %q survived a failed sky search", r.Line)
		}
	}
	lines := errorLines(m)
	if len(lines) != 1 || !strings.Contains(lines[0], "sky") || !strings.Contains(lines[0], "-dm") {
		t.Errorf("error lane = %q, want one line naming the sky search (-dm)", lines)
	}
	if got := argvLines(m); !slices.Equal(got, s.cmds) {
		t.Errorf("argv lane = %d lines, want the %d spawned commands", len(got), len(s.cmds))
	}
}

// C17: if one -t or -tr call fails, the rows of every successful call — the
// same slot's other call included — and the sky rows are still shown, and one
// Log error line names the slot and the failed search.
func TestOneFailedSearchKeepsTheOthers(t *testing.T) {
	s := &mergeSearcher{fail: map[string]bool{"-tr1962": true}}
	stubTimelineSearch(t, s)
	m := loadTimeline(t, octoberSet(t, 1))
	have := map[string]bool{}
	for _, r := range m.timelineRows() {
		switch {
		case r.Numeral == 0:
			have["sky"] = true
		case strings.Contains(r.Line, "Lunar Return"):
			have[fmt.Sprintf("return%d", r.Numeral)] = true
		default:
			have[fmt.Sprintf("transit%d", r.Numeral)] = true
		}
	}
	for _, want := range []string{"sky", "transit1", "transit2", "return1"} {
		if !have[want] {
			t.Errorf("rows %v lost %s to a failed -tr of slot 2", have, want)
		}
	}
	if have["return2"] {
		t.Error("the failed search's rows appeared")
	}
	lines := errorLines(m)
	if len(lines) != 1 || !strings.Contains(lines[0], "2 Synthetic 2") || !strings.Contains(lines[0], "-tr") {
		t.Errorf("error lane = %q, want one line naming slot 2 Synthetic 2 and -tr", lines)
	}
}

// bandProblem names what is wrong with an argv's object and aspect band, or "".
func bandProblem(argv []string) string {
	objects := slices.Index(argv, "-R0")
	aspects := slices.Index(argv, "-RA0")
	switch {
	case objects < 0:
		return "no -R0 object band"
	case aspects < 0:
		return "no -RA0 aspect band"
	case !slices.Equal(argv[objects:min(objects+7, len(argv))], []string{"-R0", "1", "3", "4", "5", "6", "7"}):
		return fmt.Sprintf("object band %v", argv[objects:])
	case !slices.Equal(argv[aspects:min(aspects+4, len(argv))], []string{"-RA0", "1", "2", "3"}):
		return fmt.Sprintf("aspect band %v", argv[aspects:])
	}
	return ""
}

// notSpawned returns the logged lines the fake never ran. A spawn the Log lacks
// is legitimate (a run cancelled by bumpIdentity or fenced by commitFloor spawns
// and never commits); a logged line that was never spawned is not.
func notSpawned(logged, spawned []string) []string {
	left := map[string]int{}
	for _, l := range spawned {
		left[l]++
	}
	var missing []string
	for _, l := range logged {
		if left[l] == 0 {
			missing = append(missing, l)
			continue
		}
		left[l]--
	}
	return missing
}

// C15: every Astrolog call this item adds — the multi-wheel, the synastry cross
// and its raw report, the N-natal searches — carries the operator's body and
// aspect choices, and each is on the Log console's command list.
func TestEveryNewCallCarriesTheBand(t *testing.T) {
	spawns := chartFileCapturingAstrolog(t)
	stubShowWheel(t)
	s := &mergeSearcher{}
	stubTimelineSearch(t, s)

	lens := lookup.Hellenistic
	lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 3, 4, 5, 6, 7}, TrueNodeMode: true}
	lens.Aspects = recipe.AspectPolicy{Explicit: true, Indices: []int{1, 2, 3}}

	// ① ② ③ and now, with the cursor on ②: the pair is anchored x anchored
	// (synastry), ① visiting ② (`1 in 2`): the tab starts on the chart alone, so
	// the visitor is chosen here as `]` would. The wheel draws that same tab as
	// a bi-wheel. Now is selected: a group without it has no Timeline tab.
	m := selectedSet(t, 3, true, 1)
	m.base = m.slots.All()[0]
	m.width = leftColumnWidth(len(m.slots.SelectedInOrder())) + 80
	m.cfg = Config{Color: ColorNever}
	m.slots.SetFloatingTime(tlFloatingTime)
	m.targetFloatingTime = tlFloatingTime
	m.focused = PanelChart
	m.lens = lens
	m = openWheel(t, m)
	// The timeline's searches are owed while its tab is the active one.
	m = m.withDetailsTab(PanelChart, TabDetailsTimeline)
	var cmd tea.Cmd
	m, cmd = m.afterComparisonChange()
	m = pumpPatient(t, m, cmd)
	m.focused = PanelBodies
	m.chart.positions = []aspect.Position{{Body: aspect.Moon}}
	m, cmd = m.startDetailsNow()
	m = pumpPatient(t, m, cmd)
	m = loadTimeline(t, m)

	band := bandProblem
	seen := map[string]int{}
	for _, sp := range spawns() {
		if !slices.Contains(sp.argv, "-r0") {
			continue
		}
		kind := "synastry cross"
		switch {
		case slices.Contains(sp.argv, "-Xo"):
			kind = "bi-wheel"
		case slices.Contains(sp.argv, "-RO"):
			kind = "synastry raw report"
		}
		seen[kind]++
		if why := band(sp.argv); why != "" {
			t.Errorf("%s call %v carries %s", kind, normaliseStage(sp.argv), why)
		}
	}
	for _, kind := range []string{"bi-wheel", "synastry cross", "synastry raw report"} {
		if seen[kind] == 0 {
			t.Errorf("no %s call was spawned (saw %v)", kind, seen)
		}
	}
	// The N-natal searches carry the choosers too: the aspect band and the node
	// mode are written once for every search, and the object band is hide-only.
	s.mu.Lock()
	cmds := slices.Clone(s.cmds)
	s.mu.Unlock()
	if len(cmds) == 0 {
		t.Fatal("no timeline search ran")
	}
	for _, cmd := range cmds {
		argv := strings.Fields(cmd)
		if !slices.Contains(argv, "-Yn") {
			t.Errorf("timeline search %q lacks the node mode", cmd)
		}
		// The object band as well as the aspect band, as every other producer's.
		if why := band(argv); why != "" {
			t.Errorf("timeline search %q carries %s", cmd, why)
		}
	}

	// Every one of them is on the console's command list, and nothing else is:
	// the Log holds exactly the spawns this scenario made, by kind and count.
	// The wheel opens once (a bi-wheel, which the Log does not list), the cross
	// pair runs once (-r0 and its raw report), the text frame's chart computes are five -qj (② frames and is the cursor, so
	// its positions and aspects are cast once, ① is listed, `1 in 2`, so its are
	// cast for Bodies, and the Details report of the Bodies row asks once), and the timeline is the -t and -tr searches of
	// three natals and the sky's -dm, over the two offset segments of an October
	// with a clock change.
	kindOf := func(line string) string {
		line += " "
		switch {
		case strings.Contains(line, " -RO "):
			return "synastry raw report"
		case strings.Contains(line, " -r0 "):
			return "synastry cross"
		case strings.Contains(line, " -tr "):
			return "timeline -tr"
		case strings.Contains(line, " -t "):
			return "timeline -t"
		case strings.Contains(line, " -dm "):
			return "timeline -dm"
		case strings.Contains(line, " -qj "):
			return "chart compute"
		}
		return "unexpected: " + line
	}
	logged := map[string]int{}
	var loggedTimeline, loggedSpawned []string
	for _, l := range argvLines(m) {
		kind := kindOf(l)
		logged[kind]++
		if strings.HasPrefix(kind, "timeline") {
			loggedTimeline = append(loggedTimeline, l)
		} else if _, argv, ok := strings.Cut(l, " "); ok {
			loggedSpawned = append(loggedSpawned, argv) // the line minus the binary path
		}
	}
	wantKinds := map[string]int{
		"synastry raw report": 1, "synastry cross": 1,
		"timeline -t": 6, "timeline -tr": 6, "timeline -dm": 2, "chart compute": 5,
	}
	if !reflect.DeepEqual(logged, wantKinds) {
		t.Errorf("the Log's command list holds %v, want exactly %v:\n%s", logged, wantKinds, strings.Join(argvLines(m), "\n"))
	}
	// Identity, not just count: the timeline lines are the spawned commands.
	// Cancellation timing decides how many extra spawns a run leaves behind, so
	// the direction is logged ⊆ spawned; the exact kind counts above pin that
	// every committed call is on the Log.
	s.mu.Lock()
	spawnedCmds := slices.Clone(s.cmds)
	s.mu.Unlock()
	if miss := notSpawned(loggedTimeline, spawnedCmds); len(miss) > 0 {
		t.Errorf("the Log holds timeline lines no search spawned:\nmissing %q\nspawned %q", miss, spawnedCmds)
	}
	// And the same for every wheel, pair and compute command: each argv the
	// counting fake actually spawned is in the Log exactly once, and the Log
	// holds no other command of those kinds. Matched by the whole argv, so a
	// stale or previous command logged in place of the current one differs.
	var spawnedArgv []string
	for _, sp := range spawns() {
		// The Log records what ran, -YQ 0 and the hook clears included (C2);
		// the recorder strips them and keeps them in prefix. In a recorded
		// command a clear's switch and its empty argument are shell-quoted, and so
		// is a word that begins `=` (zsh expands `=cmd`): the default clock's `=Yt`.
		var words []string
		for _, w := range sp.prefix {
			if w == "" || strings.HasPrefix(w, "-~") || strings.HasPrefix(w, "=") {
				w = "'" + w + "'"
			}
			words = append(words, w)
		}
		for _, w := range sp.argv {
			switch {
			case w == "":
				// An empty operand (the unnamed chart's `-zi '' ''`) is shell-quoted too.
				w = "''"
			case strings.HasPrefix(w, "="):
				w = "'" + w + "'"
			}
			words = append(words, w)
		}
		spawnedArgv = append(spawnedArgv, strings.Join(words, " "))
	}
	if miss := notSpawned(loggedSpawned, spawnedArgv); len(miss) > 0 {
		t.Errorf("the Log holds commands the fake Astrolog never ran:\nmissing %q\nspawned %q", miss, spawnedArgv)
	}
}

// V2 — the merged timeline at 132x40: ① and ② rows interleaved with the sky
// rows, every transit and return row opening with its numeral, with colour off
// (the digits). Synthetic names only (specs/frames/residue.md). Frozen as
// rendered; the existing comparing timeline goldens carry ① in their own tests.
func TestTimelineMergedTaste(t *testing.T) {
	stubTimelineSearch(t, &mergeSearcher{})
	stubPairCompute(t)
	m := octoberSet(t, 1)
	m.width = dualColumnMinWidth(3) // with three shown Details gets its 80 beside the grown left column (C5)
	m, cmd := m.afterComparisonChange()
	m = pump(t, m, cmd)
	m = loadTimeline(t, m)
	m, _ = tpress(t, m, runes("0"))
	view := m.View()
	assertFrameGeometry(t, view, m.width, 40)
	if !strings.Contains(view, " 2  10/14/2026   23:26 trans") {
		t.Fatalf("the frame holds no ② row:\n%s", view)
	}
	checkGolden(t, fmt.Sprintf("timeline-merged-%dx40", m.width), view)
}

// Positive controls for TestEveryNewCallCarriesTheBand's two directions.
func TestBandProblemCatchesADroppedIndex(t *testing.T) {
	ok := []string{"-R0", "1", "3", "4", "5", "6", "7", "-RA0", "1", "2", "3"}
	if why := bandProblem(ok); why != "" {
		t.Fatalf("the whole band is reported: %s", why)
	}
	if bandProblem([]string{"-R0", "1", "3", "4", "5", "6", "-RA0", "1", "2", "3", "9"}) == "" {
		t.Error("an object band with an index dropped passed")
	}
	if bandProblem([]string{"-R0", "1", "3", "4", "5", "6", "7", "-RA0", "1", "2"}) == "" {
		t.Error("an aspect band with an index dropped passed")
	}
}

func TestLoggedMustBeSpawnedButSpawnedNeedNotBeLogged(t *testing.T) {
	if miss := notSpawned([]string{"a", "b"}, []string{"a", "b", "b", "c"}); len(miss) != 0 {
		t.Errorf("a cancelled spawn (extra, never logged) failed the comparison: %q", miss)
	}
	if miss := notSpawned([]string{"a", "b", "b"}, []string{"a", "b"}); !slices.Equal(miss, []string{"b"}) {
		t.Errorf("a logged line spawned once but logged twice = %q, want [b]", miss)
	}
	if miss := notSpawned([]string{"a", "x"}, []string{"a"}); !slices.Equal(miss, []string{"x"}) {
		t.Errorf("a logged line nobody spawned = %q, want [x]", miss)
	}
}
