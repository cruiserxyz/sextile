package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// the specification — Enter on a timeline row is part of
// the one rule that decides what every panel shows, and Details stays quiet
// while you are inside it.

var enteredSecondNatal = time.Date(1962, 2, 12, 1, 2, 0, 0, time.UTC)

// enteredRows is the October week with one transit row given to chart ②: the
// Sun-opposite-Venus row, whose exact aspect the canned cross list carries.
func enteredRows(t *testing.T) []lookup.TimelineRow {
	t.Helper()
	rows := append([]lookup.TimelineRow(nil), weekRows(t)...)
	for i := range rows {
		if rows[i].Transit && strings.Contains(rows[i].Line, "Opp natal (Ari) Venus") {
			rows[i].Numeral = 2
			return rows
		}
	}
	t.Fatal("fixture: no Sun opposite Venus transit row")
	return nil
}

func rowIndex(t *testing.T, rows []lookup.TimelineRow, what string, ok func(lookup.TimelineRow) bool) int {
	t.Helper()
	for i, r := range rows {
		if ok(r) {
			return i
		}
	}
	t.Fatalf("fixture: no %s row", what)
	return -1
}

func chartRowOf(t *testing.T, rows []lookup.TimelineRow, numeral int) int {
	return rowIndex(t, rows, "chart row", func(r lookup.TimelineRow) bool { return r.Numeral == numeral })
}

// firstChartRow is the index of the first row that belongs to a chart.
func firstChartRow(t *testing.T, rows []lookup.TimelineRow) int {
	t.Helper()
	return rowIndex(t, rows, "chart", func(r lookup.TimelineRow) bool { return r.Numeral != 0 })
}

func skyAspectRow(t *testing.T, rows []lookup.TimelineRow) int {
	return rowIndex(t, rows, "sky aspect", func(r lookup.TimelineRow) bool {
		return r.Numeral == 0 && strings.Contains(r.Line, "Tri (Leo) Mars")
	})
}

func skyIngressRow(t *testing.T, rows []lookup.TimelineRow) int {
	return rowIndex(t, rows, "ingress", func(r lookup.TimelineRow) bool {
		return r.Numeral == 0 && strings.Contains(r.Line, "--> Sagittarius")
	})
}

// journeyRow is the row the C51 journey presses: a chart row, so the cross
// compute the journey asserts still runs (rule 2 gives a sky row no cross).
func journeyRow(t *testing.T) int { return firstChartRow(t, weekRows(t)) }

func downKeys(n int) []tea.KeyMsg {
	keys := make([]tea.KeyMsg, n)
	for i := range keys {
		keys[i] = runes("j")
	}
	return keys
}

// enteredFixture is a 132x40 workspace of anchored charts ① ② [...] and the
// floating slot, the timeline loaded and Details attached on it, the cursor on
// ①. Chart ② holds one transit row. The cross compute answers a canned exact
// aspect (Sun opposite Venus, orb 0) only for chart ②'s natal and a decoy for
// any other, so a pair that is not ②'s cannot show the row's aspect.
func enteredFixture(t *testing.T, anchors int, floatingSelected bool, cfg Config) (Model, *[]textCall) {
	t.Helper()
	return enteredFixtureNamed(t, anchors, floatingSelected, cfg)
}

// enteredFixtureNamed names the first anchored charts before the timeline
// loads: a name is part of the timeline's key, so it cannot change afterwards.
func enteredFixtureNamed(t *testing.T, anchors int, floatingSelected bool, cfg Config, names ...string) (Model, *[]textCall) {
	t.Helper()
	return enteredFixtureRows(t, enteredRows(t), anchors, floatingSelected, cfg, names...)
}

// enteredFixtureRows is enteredFixtureNamed over a caller's timeline rows.
func enteredFixtureRows(t *testing.T, rows []lookup.TimelineRow, anchors int, floatingSelected bool, cfg Config, names ...string) (Model, *[]textCall) {
	t.Helper()
	return enteredFixtureAt(t, rows, anchors, floatingSelected, false, cfg, names...)
}

// enteredFixtureUnselectedNow is the one Timeline state an unselected floating
// slot has: the cursor on the floating slot itself, the anchored charts
// selected without it, and the reduced list, which is the sky alone (the rows
// of the fixture without a chart's numeral). From an anchored chart of that
// group there is no Timeline tab at all.
func enteredFixtureUnselectedNow(t *testing.T, anchors int, cfg Config) (Model, *[]textCall) {
	t.Helper()
	var sky []lookup.TimelineRow
	for _, r := range enteredRows(t) {
		if r.Numeral == 0 {
			sky = append(sky, r)
		}
	}
	return enteredFixtureAt(t, sky, anchors, false, true, cfg)
}

// enteredFixtureAt is enteredFixtureRows with the cursor either on ① (the
// default) or on the floating slot.
func enteredFixtureAt(t *testing.T, rows []lookup.TimelineRow, anchors int, floatingSelected, cursorOnFloating bool, cfg Config, names ...string) (Model, *[]textCall) {
	t.Helper()
	text := stubTextCompute(t)
	overlayCrossAspects = func(_ context.Context, _ lookup.Lens, natal store.AnchorChart, when time.Time, _ location.PlaceInfo) (lookup.OverlayAspects, error) {
		*text = append(*text, textCall{kind: "aspects", when: when})
		if natal.When.Equal(enteredSecondNatal) {
			return lookup.OverlayAspects{Aspects: []lookup.OverlayAspect{{Transit: aspect.Sun, Natal: aspect.Venus, Kind: aspect.Opposition, Orb: 0}}}, nil
		}
		return lookup.OverlayAspects{Aspects: []lookup.OverlayAspect{{Transit: aspect.Mars, Natal: aspect.Moon, Kind: aspect.Square, Orb: 3}}}, nil
	}
	stubTimeline(t, func(lookup.TimelineRequest) (lookup.TimelineResult, error) {
		return lookup.TimelineResult{Rows: rows, Commands: []string{"astrolog -dm"}}, nil
	})
	m := timelineModel(t, cfg)
	for i := 1; i < anchors; i++ {
		when := enteredSecondNatal
		if i > 1 {
			when = enteredSecondNatal.AddDate(i, 0, 0)
		}
		if err := addAnchor(m.slots, when, testPlaceAmsterdam()); err != nil {
			t.Fatalf("addAnchor: %v", err)
		}
	}
	all := m.slots.All()
	for i, name := range names {
		all[i].Chart.Name = name
	}
	m.slots.SetSlotSelected(len(all)-1, false) // room under the cap of six first
	for i := range all[:len(all)-1] {
		m.slots.SetSlotSelected(i, true)
	}
	m.slots.SetSlotSelected(len(all)-1, floatingSelected)
	m.cursorIdx = 0
	if cursorOnFloating {
		m.cursorIdx = len(all) - 1
	}
	if anchors > 3 {
		m.width, m.height = 240, 12+3*anchors // room for six columns and the longer chart list
	}
	m = m.noteArrival()
	m.width = max(m.width, dualColumnMinWidth(m.shownCount()))
	m, cmd := syncTimelineVisited(m)
	m = pump(t, m, cmd)
	if !m.timeline.complete || len(m.timeline.rows) != len(rows) {
		t.Fatalf("fixture timeline did not load: complete=%v rows=%d", m.timeline.complete, len(m.timeline.rows))
	}
	m, _ = tpress(t, m, runes("0"))
	m, _ = tpress(t, m, keyEnter)
	if !m.details.attached || m.activeTabID(PanelDetails) != TabDetailsTimeline {
		t.Fatalf("fixture: attached=%v tab=%s", m.details.attached, m.activeTabID(PanelDetails))
	}
	return m, text
}

// enter presses Enter on row i of the timeline and delivers every result.
func enter(t *testing.T, m Model, i int) Model {
	t.Helper()
	m.details.cursor = i
	m, cmd := tpress(t, m, keyEnter)
	return pump(t, m, cmd)
}

func activeAspectsTitle(m Model) string {
	for _, tab := range m.panelTabs(PanelAspects) {
		if tab.ID == m.activeTabID(PanelAspects) {
			return tab.Title
		}
	}
	return ""
}

// Rules 1 and 3: Enter on chart ②'s transit row makes `3 in 2` the Aspects
// tab, the row's aspect is in that list within 0°01', and a `]` pressed after
// Enter wins over the row's tab.
func TestEnterOnAChartRowPicksItsPair(t *testing.T) {
	// Only with now selected: an unselected now stays unselected on Enter
	// (TestTimelineEnterDoesNotSelectNow), so there is no pair tab to pick.
	for _, floatingSelected := range []bool{true} {
		m, _ := enteredFixture(t, 2, floatingSelected, Config{Accessible: true})
		row := m.timeline.rows[chartRowOf(t, m.timeline.rows, 2)]
		m = enter(t, m, chartRowOf(t, m.timeline.rows, 2))
		// The cursor goes to the floating slot, read in the row's chart ②
		// (a moment is never a base, operator 2026-10-24; this replaces C2's
		// walk to the row's chart); the press holds.
		if !m.focusedIsFloating() || !m.slots.Floating().Time.Equal(landedAt(row.Instant)) {
			t.Fatalf("floating selected=%v: Enter left the cursor at %d (floating: %v), want now's line and the moment on the row", floatingSelected, m.cursorIdx, m.focusedIsFloating())
		}
		if m.deriveScene().entered == nil {
			t.Fatalf("floating selected=%v: the pressed row does not hold right after Enter's own cursor move (C4)", floatingSelected)
		}
		if got, want := activeAspectsTitle(m), "3 in 2"; got != want {
			t.Errorf("floating selected=%v: the Aspects tab is %q, want %q", floatingSelected, got, want)
		}
		found := false
		for _, a := range m.chart.crossAspects {
			if a.Transit == aspect.Sun && a.Natal == aspect.Venus && a.Kind == aspect.Opposition && a.Orb <= 1.0/60 {
				found = true
			}
		}
		if !found {
			t.Errorf("floating selected=%v: the row's aspect (Sun opposite Venus, within 0°01') is not in the list: %+v", floatingSelected, m.chart.crossAspects)
		}
		// C2: the row's cell is bold, on a screen with colours off.
		{
			bm, _ := markFixture(t, Config{Color: ColorNever})
			bm.slots.SetSlotSelected(len(bm.slots.All())-1, floatingSelected)
			bm = enter(t, bm, timelineRowWith(t, bm.timeline.rows, "Opp natal (Ari) Venus"))
			if !bm.focusedIsFloating() {
				t.Errorf("floating selected=%v: the bold fixture's cursor is %d, want now's line", floatingSelected, bm.cursorIdx)
			}
			_, aspects := leftPanels(t, bm.View())
			if got := boldSpanTexts(aspects); len(got) != 1 || !strings.HasPrefix(got[0], bodyGlyph(aspect.Venus)+" "+aspectGlyph(aspect.Opposition)+" "+bodyGlyph(aspect.Sun)+" ") {
				t.Errorf("floating selected=%v: bold cells = %q, want the natal-Venus-opposite-Sun cell", floatingSelected, got)
			}
		}
		// Rule 3: a `]` after Enter is a deliberate choice and wins over what Enter stored.
		m, _ = tpress(t, m, keyEsc)
		m, _ = tpress(t, m, runes("3"))
		m, cmd := tpress(t, m, runes("]"))
		m = pump(t, m, cmd)
		if got := activeAspectsTitle(m); got == "3 in 2" || m.deriveScene().entered == nil {
			t.Errorf("floating selected=%v: after `]` the tab is %q (entered holds: %v), want another tab with the row still entered", floatingSelected, got, m.deriveScene().entered != nil)
		}
	}
}

// Rule 2: Enter on a sky row makes `Aspects` the active tab and runs no cross
// compute; an ingress row does the same and has no aspect to find.
func TestEnterOnASkyRowPicksAspects(t *testing.T) {
	for _, pick := range []func(*testing.T, []lookup.TimelineRow) int{skyAspectRow, skyIngressRow} {
		m, text := enteredFixture(t, 2, true, Config{Accessible: true})
		// Start from a pair tab (① read in ②) so the sky row has something to
		// change: arrival alone leaves the chart's own tab (S4a A1).
		m.base = m.slots.All()[1]
		if m.activeTabID(PanelAspects) == TabAspects {
			t.Fatal("fixture: the base did not choose a pair tab")
		}
		i := pick(t, m.timeline.rows)
		before := len(*text)
		m = enter(t, m, i)
		if !m.focusedIsFloating() || m.cursorIdx != len(m.slots.All())-1 {
			t.Errorf("row %q: the cursor is at %d, want the floating slot's line (C3)", m.timeline.rows[i].Line, m.cursorIdx)
		}
		if got := m.activeTabID(PanelAspects); got != TabAspects {
			t.Errorf("row %q: the Aspects tab is %s, want %s", m.timeline.rows[i].Line, got, TabAspects)
		}
		if !strings.Contains(m.View(), "[Aspects]") {
			t.Errorf("row %q: the strip does not show [Aspects] active", m.timeline.rows[i].Line)
		}
		// Enter steps the Chart panel's own Up/Down action on the way to now,
		// each applied for its state only: no arrival in between computes anything,
		// and the one compute is the Aspects tab's own, with no cross call.
		after := (*text)[before:]
		if len(after) == 0 {
			t.Errorf("row %q: Enter computed nothing", m.timeline.rows[i].Line)
		}
		for _, c := range after {
			if c.kind == "aspects" {
				t.Errorf("row %q: a cross compute ran for the Aspects tab: %v", m.timeline.rows[i].Line, after)
			}
		}
		if len(m.chart.crossAspects) != 0 {
			t.Errorf("row %q: the committed frame carries %d cross aspects on the Aspects tab", m.timeline.rows[i].Line, len(m.chart.crossAspects))
		}
	}
}

// Six charts: the floating slot cannot join the cap, the selected group is
// without it, and a group without the floating slot has no Timeline tab at
// all (timeline-tab-rules T1 rule 1), so there is no row to press Enter on.
// `0` and `]` skip the absent tab and the cursor and selection stay.
func TestSixSelectedHaveNoTimelineTab(t *testing.T) {
	stubTimeline(t, nil)
	m := timelineModel(t, Config{Accessible: true})
	for i := 1; i < 6; i++ {
		if err := addAnchor(m.slots, enteredSecondNatal.AddDate(i, 0, 0), testPlaceAmsterdam()); err != nil {
			t.Fatalf("addAnchor: %v", err)
		}
	}
	all := m.slots.All()
	m.slots.SetSlotSelected(len(all)-1, false)
	for i := range all[:len(all)-1] {
		m.slots.SetSlotSelected(i, true)
	}
	if m.slots.SetSlotSelected(len(all)-1, true) || m.slots.SelectedCount() != store.MaxSelectedSlots {
		t.Fatalf("fixture: the floating slot joined six selected charts (%d selected)", m.slots.SelectedCount())
	}
	m.width, m.height = 240, 30
	m.cursorIdx = 0
	m = m.noteArrival()
	for _, tab := range m.panelTabs(PanelDetails) {
		if tab.ID == TabDetailsTimeline {
			t.Fatalf("a Timeline tab with six selected charts and the cursor on chart ①: %+v", m.panelTabs(PanelDetails))
		}
	}
	for _, k := range []tea.KeyMsg{runes("0"), runes("]"), keyEnter} {
		var c tea.Cmd
		m, c = tpress(t, m, k)
		m = pump(t, m, c)
		if m.activeTabID(PanelDetails) != TabDetailsReport || m.cursorIdx != 0 {
			t.Fatalf("after %q: tab %s, cursor %d, want the report and chart ①", k.String(), m.activeTabID(PanelDetails), m.cursorIdx)
		}
	}
	if m.slots.SelectedCount() != store.MaxSelectedSlots || m.slots.SlotSelected(len(m.slots.All())-1) {
		t.Error("a key changed the selection")
	}
}

// reportSpawns counts every Details report spawn, whichever of the three
// report computes it goes through.
func reportSpawns(t *testing.T) func() int {
	t.Helper()
	n := 0
	oldRaw, oldCross, oldSyn := computeRawReport, computeCrossBodyReport, computeSynastryBodyReport
	t.Cleanup(func() { computeRawReport, computeCrossBodyReport, computeSynastryBodyReport = oldRaw, oldCross, oldSyn })
	computeRawReport = func(context.Context, time.Time, location.PlaceInfo, lookup.Lens, lookup.RawReportRequest) (string, string, error) {
		n++
		return "raw", "", nil
	}
	computeCrossBodyReport = func(context.Context, lookup.Lens, store.AnchorChart, time.Time, location.PlaceInfo, aspect.Body) (string, string, error) {
		n++
		return "cross", "", nil
	}
	computeSynastryBodyReport = func(context.Context, lookup.Lens, store.AnchorChart, time.Time, location.PlaceInfo, store.ReportLabel, aspect.Body) (string, string, error) {
		n++
		return "syn", "", nil
	}
	return func() int { return n }
}

func stripOfDetails(m Model) string { return m.panelTabs(PanelDetails)[0].Title }

// Rule 5 as amended by the specification: no report
// starts while the report tab is hidden, attached or not; the pending mark and
// the report's title, body stay as they were across Enter, three step keys and
// `n` on the timeline tab. (Before the amendment the rule was "while attached";
// the tests that pinned a report waiting on the report tab itself while
// attached are TestReportRefreshesWhileAttached.)
func TestDetailsIsQuietWhileTheReportTabIsHidden(t *testing.T) {
	spawns := reportSpawns(t)
	// Control: the same moves with the report tab visible DO spawn, so the
	// counter is live.
	{
		m, _ := enteredFixture(t, 2, true, Config{Accessible: true})
		m, _ = tpress(t, m, keyEsc)
		m, _ = tpress(t, m, runes("0")) // timeline -> report
		if m.activeTabID(PanelDetails) != TabDetailsReport {
			t.Fatal("fixture: the report tab is not active")
		}
		m.cursorIdx = len(m.slots.All()) - 1
		m, cmd := tpress(t, m, runes("d"))
		pump(t, m, cmd)
		if spawns() == 0 {
			t.Fatal("control: a step key with the report tab visible started no report, so the counter proves nothing")
		}
	}
	// A text frame already awaited when Details attaches onto the timeline:
	// its commit, arriving while the report tab is hidden, starts no report.
	{
		m, _ := enteredFixture(t, 2, true, Config{Accessible: true})
		m, _ = tpress(t, m, keyEsc)
		m, _ = tpress(t, m, runes("0")) // report visible again, as before Enter
		m.cursorIdx = len(m.slots.All()) - 1
		m, cmd := tpress(t, m, runes("d"))
		if !m.details.waitForText {
			t.Fatal("fixture: the step key did not leave a text frame awaited")
		}
		base := spawns()
		m, _ = tpress(t, m, keyEnter)
		if !m.details.attached || m.activeTabID(PanelDetails) != TabDetailsTimeline {
			t.Fatal("fixture: Enter did not attach Details onto the timeline")
		}
		pump(t, m, cmd)
		if got := spawns() - base; got != 0 {
			t.Fatalf("a text frame awaited before attaching, committed with the report hidden: %d report spawns, want 0", got)
		}
	}
	// A text frame awaited before attaching that FAILS while the report is
	// hidden leaves the pending mark as it was.
	{
		m, _ := enteredFixture(t, 2, true, Config{Accessible: true})
		m, _ = tpress(t, m, keyEsc)
		m, _ = tpress(t, m, runes("0"))
		m, cmd := m.afterFocusChange()
		m = pump(t, m, cmd)
		m.cursorIdx = len(m.slots.All()) - 1
		m, cmd = tpress(t, m, runes("d"))
		if !m.details.pending || !m.details.waitForText {
			t.Fatal("fixture: the step key left no pending mark")
		}
		m, _ = tpress(t, m, keyEnter)
		firstCallTransits = func(context.Context, *lookup.Probers, time.Time, location.PlaceInfo, lookup.Lens) (lookup.Result, error) {
			return lookup.Result{}, errors.New("astrolog failed")
		}
		m = pump(t, m, cmd)
		if !m.details.pending {
			t.Fatal("a text frame that failed with the report hidden cleared the pending mark")
		}
	}
	m, _ := enteredFixture(t, 2, true, Config{Accessible: true})
	base := spawns()
	title, body := stripOfDetails(m), m.details.body
	i := chartRowOf(t, m.timeline.rows, 2)
	m = enter(t, m, i)
	for _, k := range []string{"d", "h", "m", "n"} {
		var cmd tea.Cmd
		m, cmd = tpress(t, m, runes(k))
		m = pump(t, m, cmd)
		if got := spawns() - base; got != 0 {
			t.Fatalf("after %q on the timeline tab: %d report spawns, want 0", k, got)
		}
		if m.details.pending || m.details.waitForText {
			t.Fatalf("after %q on the timeline tab: the report is marked pending", k)
		}
	}
	if got := stripOfDetails(m); got != title || m.details.body != body {
		t.Errorf("the report changed while hidden: title %q -> %q", title, got)
	}
	if !m.details.attached {
		t.Error("Details detached")
	}
}

// Acceptance 1 of the specification (amends rule 5):
// attached to Bodies with the report tab on screen, a report that was awaited
// before attaching, a timer armed before attaching and a report running when
// Details attached all refresh the report; none is held back for the detach.
func TestReportRefreshesWhileAttached(t *testing.T) {
	spawns := reportSpawns(t)
	attachedOnBodies := func(t *testing.T) Model {
		m, _ := enteredFixture(t, 2, true, Config{Accessible: true})
		m, _ = tpress(t, m, keyEsc)
		m, cmd := m.afterFocusChange()
		m = pump(t, m, cmd)
		return m
	}
	// A report timer armed by a focus change just before Details attaches
	// fires while attached, and the report starts.
	{
		m := attachedOnBodies(t)
		m, cmd := tpress(t, m, runes("2"))
		if !m.details.timerArmed {
			t.Fatal("fixture: the focus change armed no report timer")
		}
		base := spawns()
		m, _ = tpress(t, m, keyEnter)
		if !m.details.attached || m.activeTabID(PanelDetails) != TabDetailsReport {
			t.Fatal("fixture: Enter did not attach Details onto the report tab")
		}
		pump(t, m, cmd)
		if got := spawns() - base; got != 1 {
			t.Fatalf("a report timer armed before attaching, firing while attached on the report tab: %d report spawns, want 1", got)
		}
	}
	// A report already running when Details attaches lands inside it.
	{
		m := attachedOnBodies(t)
		m, _ = tpress(t, m, runes("2"))
		m.details.body = "the frame before attaching"
		m, cmd := m.startDetailsNow()
		m, _ = tpress(t, m, keyEnter)
		if !m.details.attached {
			t.Fatal("fixture: Enter did not attach Details")
		}
		m = pump(t, m, cmd)
		if m.details.body == "the frame before attaching" || m.details.stale {
			t.Fatalf("a report started before attaching did not land while attached: body %q, stale %v", m.details.body, m.details.stale)
		}
	}
}

// Rule 6: the report catches up on the report tab becoming active (once), and
// on detach only if something changed while attached.
func TestReportCatchesUpOnTabSwitchAndDetach(t *testing.T) {
	spawns := reportSpawns(t)
	m, _ := enteredFixture(t, 2, true, Config{Accessible: true})
	base := spawns()
	m = enter(t, m, chartRowOf(t, m.timeline.rows, 2))
	if spawns() != base {
		t.Fatal("fixture: Enter spawned a report")
	}
	m, cmd := tpress(t, m, runes("0"))
	m = pump(t, m, cmd)
	if got := spawns() - base; got != 1 || m.activeTabID(PanelDetails) != TabDetailsReport {
		t.Fatalf("`0` to the report tab: %d spawns on %s, want 1 on the report", got, m.activeTabID(PanelDetails))
	}
	m, cmd = tpress(t, m, runes("0"))
	m = pump(t, m, cmd)
	m, cmd = tpress(t, m, runes("0"))
	m = pump(t, m, cmd)
	if got := spawns() - base; got != 1 {
		t.Errorf("tab switches with nothing changed: %d spawns, want still 1", got)
	}
	// Detach with nothing changed: none.
	m, cmd = tpress(t, m, keyEsc)
	m = pump(t, m, cmd)
	if got := spawns() - base; got != 1 {
		t.Errorf("Esc with nothing changed: %d spawns, want still 1", got)
	}

	// A change while attached on the timeline, then Esc: the tab Enter found
	// was the timeline, so it stays active and the report is not asked for
	// (rule 2 of the specification); choosing the report
	// tab then asks once.
	m, _ = enteredFixture(t, 2, true, Config{Accessible: true})
	base = spawns()
	m = enter(t, m, chartRowOf(t, m.timeline.rows, 2))
	m, cmd = tpress(t, m, runes("d"))
	m = pump(t, m, cmd)
	if spawns() != base {
		t.Fatal("fixture: a step spawned a report with the report tab hidden")
	}
	m, cmd = tpress(t, m, keyEsc)
	m = pump(t, m, cmd)
	if got := spawns() - base; got != 0 || m.details.attached || !m.details.stale {
		t.Errorf("Esc after a change onto the timeline tab: %d spawns (attached=%v stale=%v), want 0, detached and remembered", got, m.details.attached, m.details.stale)
	}
	m, cmd = tpress(t, m, runes("0"))
	m = pump(t, m, cmd)
	if got := spawns() - base; got != 1 || m.activeTabID(PanelDetails) != TabDetailsReport {
		t.Errorf("choosing the report tab after a change: %d spawns on %s, want 1 on the report", got, m.activeTabID(PanelDetails))
	}

	// Esc straight after attaching: none.
	m, _ = enteredFixture(t, 2, true, Config{Accessible: true})
	base = spawns()
	m, cmd = tpress(t, m, keyEsc)
	pump(t, m, cmd)
	if got := spawns() - base; got != 0 {
		t.Errorf("Esc with no change: %d spawns, want 0", got)
	}
}

// Rule 7: 1, 2, 3 from attached Details detach and focus that panel; in the
// open search line they stay text; the bar says so.
func TestDigitsDetachAndFocus(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want PanelID
	}{{"1", PanelChart}, {"2", PanelBodies}, {"3", PanelAspects}} {
		m, _ := enteredFixture(t, 2, true, Config{Accessible: true})
		if !strings.Contains(keyBarLine(200, m.keyBarEntries(), m.cfg), "Focus: 1/2/3") {
			t.Fatalf("the attached key bar does not offer Focus: 1/2/3: %q", keyBarLine(200, m.keyBarEntries(), m.cfg))
		}
		m, cmd := tpress(t, m, runes(tc.key))
		m = pump(t, m, cmd)
		if m.details.attached || m.focused != tc.want {
			t.Errorf("%s: attached=%v focused=%s, want detached on %s", tc.key, m.details.attached, m.focused, tc.want)
		}
	}
	m, _ := enteredFixture(t, 2, true, Config{Accessible: true})
	m, _ = tpress(t, m, runes("/"))
	if strings.Contains(keyBarLine(200, m.keyBarEntries(), m.cfg), "Focus: 1/2/3") {
		t.Errorf("the search line's key bar offers Focus: 1/2/3, where digits are text: %q", keyBarLine(200, m.keyBarEntries(), m.cfg))
	}
	m, _ = tpress(t, m, runes("2"))
	if !m.details.attached || m.focused != PanelChart || m.timeline.query != "2" {
		t.Errorf("a digit in the search line: attached=%v focused=%s query=%q, want text", m.details.attached, m.focused, m.timeline.query)
	}
}

// Rule 8: the entered row is a field of the scene while it holds, and each of
// the three events ends it.
func TestEnteredRowIsInTheScene(t *testing.T) {
	fresh := func() Model {
		m, _ := enteredFixture(t, 2, true, Config{Accessible: true})
		return enter(t, m, chartRowOf(t, m.timeline.rows, 2))
	}
	m := fresh()
	e := m.deriveScene().entered
	all := m.slots.All()
	if e == nil || e.numeral != 2 || e.chart != all[1] || !e.instant.Equal(m.timeline.rows[chartRowOf(t, m.timeline.rows, 2)].Instant) {
		t.Fatalf("after Enter on ②'s row the scene's entered row is %+v", e)
	}
	// A sky row is entered without a chart.
	m, _ = enteredFixture(t, 2, true, Config{Accessible: true})
	m = enter(t, m, skyAspectRow(t, m.timeline.rows))
	if e := m.deriveScene().entered; e == nil || e.chart != nil || e.numeral != 0 {
		t.Errorf("a sky row's entered row is %+v, want one without a chart", e)
	}

	// A time step ends it: with the cursor on ② the step key is a no-op (floating
	// only), so the moment moves by the path a step takes (C2 of
	// the specification).
	m = fresh()
	m, cmd := m.setFloatingTarget(m.targetFloatingTime.Add(24 * time.Hour))
	m = pump(t, m, cmd)
	if m.deriveScene().entered != nil {
		t.Error("a time step did not end the entered row")
	}
	// A cursor arrival ends it.
	m = fresh()
	m, _ = tpress(t, m, keyEsc)
	m, cmd = tpress(t, m, keyUp)
	m = pump(t, m, cmd)
	if m.deriveScene().entered != nil {
		t.Error("a cursor arrival did not end the entered row")
	}
	// A selection change ends it.
	m = fresh()
	m, _ = tpress(t, m, keyEsc)
	m, cmd = tpress(t, m, keySpace)
	m = pump(t, m, cmd)
	if m.deriveScene().entered != nil {
		t.Error("a selection change did not end the entered row")
	}
	// Its chart leaving the set ends it, whichever way the set changes.
	m = fresh()
	m.slots.SetSlotSelected(1, false)
	if m.deriveScene().entered != nil {
		t.Error("the entered row holds with its chart out of the set")
	}
}

// ---- frames (the specification) --------------

func enteredFrame(t *testing.T, floatingSelected bool) (string, int) {
	t.Helper()
	m, _ := enteredFixtureNamed(t, 2, floatingSelected, Config{Color: ColorNever}, "Aster", "Bo")
	m = enter(t, m, chartRowOf(t, m.timeline.rows, 2))
	view := m.View()
	assertFrameGeometry(t, view, m.width, 40)
	return view, m.width
}

func TestEnterRowPicksItsPairFrame(t *testing.T) {
	view, w := enteredFrame(t, true)
	checkGolden(t, fmt.Sprintf("enter-row-picks-its-pair-%dx40-never", w), view)
}

// Gate 5 for Enter (the specification through rules 1 and 4): the
// key-press frame keeps the old Bodies and Aspects, the result swaps them in
// one View, and no View between shows the new pair's title over the old
// pair's rows.
func TestEnterSwapsAspectsTogether(t *testing.T) {
	// Only with now selected: from a group without it there is no Timeline tab.
	for _, floatingSelected := range []bool{true} {
		m, _ := enteredFixture(t, 2, floatingSelected, Config{})
		// Settle one text frame first, so there is a committed scene to hold.
		m, cmd := m.afterFocusChange()
		m = pump(t, m, cmd)
		if !m.shown.stamped || m.chart.epoch == 0 {
			t.Fatalf("floating selected=%v: fixture: no committed frame to hold", floatingSelected)
		}
		_, before := leftColumn(t, m.View())
		m.details.cursor = chartRowOf(t, m.timeline.rows, 2)
		m1, cmd := tpress(t, m, keyEnter)
		if _, mid := leftColumn(t, m1.View()); mid != before {
			t.Fatalf("floating selected=%v: Bodies and Aspects changed in the key-press frame:\n--- before\n%s\n--- key-press frame\n%s", floatingSelected, before, mid)
		}
		m2, views := pumpViews(t, m1, cmd)
		_, after := leftColumn(t, m2.View())
		if after == before {
			t.Fatalf("floating selected=%v: the entered row never reached Bodies and Aspects:\n%s", floatingSelected, after)
		}
		for i, v := range views {
			if _, got := leftColumn(t, v); got != before && got != after {
				t.Fatalf("floating selected=%v: view %d of %d mixes the two scenes:\n--- before\n%s\n--- after\n%s\n--- got\n%s", floatingSelected, i+1, len(views), before, after, got)
			}
		}
	}
}

// Enter lands the moment on the LAST second of the row's listed minute
// (the specification rules 1, 3, 4): an event happens inside
// its minute, so the screen shows the state just after it. The seconds are
// asserted, not the minute — the minute alone passes with the old code.
func TestEnterLandsAtTheEndOfTheRowsMinute(t *testing.T) {
	cfg := Config{Color: ColorNever}
	kinds := []struct{ name, frag string }{
		{"aspect", "Tri (Leo) Mars"},
		{"ingress", "--> Sagittarius"},
		{"station", "Pluto <Aqu> S/D"},
		{"transit", "Opp natal (Ari) Venus"},
		{"return", "Lunar Return"},
	}
	for _, k := range kinds {
		t.Run(k.name, func(t *testing.T) {
			m, _ := enteredFixture(t, 2, true, cfg)
			i := timelineRowWith(t, m.timeline.rows, k.frag)
			row := m.timeline.rows[i]
			m = enter(t, m, i)
			want := row.Instant.Add(59 * time.Second)
			if !m.targetFloatingTime.Equal(want) {
				t.Errorf("%s row: target = %s, want %s (the listed minute's last second)", k.name, m.targetFloatingTime.Format(time.RFC3339), want.Format(time.RFC3339))
			}
			if got := m.floatingTime(); !got.Equal(want) {
				t.Errorf("%s row: committed floating time = %s, want %s", k.name, got.Format(time.RFC3339), want.Format(time.RFC3339))
			}
			// Rule 4: the row is still the nearest one and the cursor sits on it.
			if n := m.nearestRow(m.timelineShown()); n != i {
				t.Errorf("%s row: nearest row = %d, want %d", k.name, n, i)
			}
			if m.details.cursor != i {
				t.Errorf("%s row: cursor = %d, want %d", k.name, m.details.cursor, i)
			}
		})
	}

	// Rule 3: a step key moves from the landed instant, the seconds carrying.
	m, _ := enteredFixture(t, 2, true, cfg)
	i := skyAspectRow(t, m.timeline.rows)
	row := m.timeline.rows[i]
	m = enter(t, m, i)
	m.cursorIdx = len(m.slots.All()) - 1
	m, cmd := tpress(t, m, runes("d"))
	m = pump(t, m, cmd)
	want := row.Instant.Add(59*time.Second + 24*time.Hour)
	if !m.targetFloatingTime.Equal(want) {
		t.Errorf("d after Enter: target = %s, want %s", m.targetFloatingTime.Format(time.RFC3339), want.Format(time.RFC3339))
	}
	if s := m.targetFloatingTime.Second(); s != 59 {
		t.Errorf("d after Enter: seconds = %d, want 59 carried", s)
	}

	// The nearest row with the entered mark gone (the cursor moved) is still the
	// row whose minute the moment is in, not the next one.
	m, _ = enteredFixture(t, 2, true, cfg)
	i = skyAspectRow(t, m.timeline.rows)
	m = enter(t, m, i)
	m.details.pressedSet = false
	if n := m.nearestRow(m.timelineShown()); n != i {
		t.Errorf("after the entered mark ended: nearest row = %d, want %d", n, i)
	}

	// Rows listed in local mean time sit on a UTC second that is not :00 (12:00
	// LMT Amsterdam is 11:40:28 UTC): the landed moment still finds its row, and
	// a moment with no seconds finds the row at or after it, as before.
	base := time.Date(1850, 3, 1, 11, 40, 28, 0, time.UTC)
	lmt := []lookup.TimelineRow{{Instant: base}, {Instant: base.Add(time.Minute)}, {Instant: base.Add(2 * time.Minute)}}
	if n := nearestTimelineRow(lmt, landedAt(lmt[1].Instant)); n != 1 {
		t.Errorf("LMT row landed on: nearest = %d, want 1", n)
	}
	if n := nearestTimelineRow(lmt, time.Date(1850, 3, 1, 11, 41, 0, 0, time.UTC)); n != 1 {
		t.Errorf("LMT, a moment between rows: nearest = %d, want 1 (the row at or after it)", n)
	}
}

// Operator 2026-10-23: Enter moves the floating moment to the row's instant but
// never selects the floating slot, nor changes any other slot's selection. An
// unselected floating slot has the Timeline tab only with the cursor on itself
// (the sky-only list); a group without it, with the cursor on a member, has no
// tab, so the sky row is the one that can be pressed.
func TestTimelineEnterDoesNotSelectNow(t *testing.T) {
	m, _ := enteredFixtureUnselectedNow(t, 2, Config{Color: ColorAlways})
	nowIdx := len(m.slots.All()) - 1
	selectedBefore := m.slots.SelectedCount()
	var maskBefore []bool
	for i := range m.slots.All() {
		maskBefore = append(maskBefore, m.slots.SlotSelected(i))
	}
	if m.slots.SlotSelected(nowIdx) || !m.focusedIsFloating() {
		t.Fatal("fixture: now is selected or the cursor is not on it")
	}
	i := skyAspectRow(t, m.timeline.rows)
	row := m.timeline.rows[i]
	m = enter(t, m, i)
	if m.slots.SlotSelected(nowIdx) {
		t.Errorf("Enter selected the floating slot")
	}
	if m.slots.SelectedCount() != selectedBefore {
		t.Errorf("the selected count is %d after Enter, want %d", m.slots.SelectedCount(), selectedBefore)
	}
	for j, was := range maskBefore {
		if m.slots.SlotSelected(j) != was {
			t.Errorf("slot %d selection changed from %v", j, was)
		}
	}
	if m.details.cursor != i {
		t.Errorf("the row cursor is %d after Enter, want the pressed row %d", m.details.cursor, i)
	}
	if !m.targetFloatingTime.Equal(landedFor(row)) {
		t.Errorf("the floating target is %v, want the row's instant %v", m.targetFloatingTime, landedFor(row))
	}
	if got := m.slots.All()[nowIdx].Time; !got.Equal(landedFor(row)) {
		t.Errorf("now's time is %v, want the row's instant %v", got, landedFor(row))
	}
}
