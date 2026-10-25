package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// the specification: the tab on screen keeps up with the
// floating moment and the subject, hidden tabs do no work, and a waiting tab
// keeps its previous content dimmed (` …` after its title with colours off).

// detailsPanelText is the rendered Details panel, escapes and all.
func detailsPanelText(m Model) string { return renderDetails(m) }

// isDimmed says the rendered panel carries the faint attribute.
func isDimmed(m Model) bool { return strings.Contains(detailsPanelText(m), sgrFaint) }

// stripTitle is the active Details tab's title as the strip prints it.
func stripTitle(m Model) string {
	s := m.tabStrips()[PanelDetails]
	return s.tabs[s.active].Title
}

// monthRows answers the timeline stub with a different single row per month,
// so a list for one month cannot be mistaken for another's.
func monthRows(t *testing.T) func(lookup.TimelineRequest) (lookup.TimelineResult, error) {
	t.Helper()
	week := weekRows(t)
	november := []lookup.TimelineRow{{Instant: time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC), Line: "Tue 11/ 3/2026 10:00am    Moon (Leo) Tri (Sag) Sun", Command: "astrolog -dm"}}
	return func(req lookup.TimelineRequest) (lookup.TimelineResult, error) {
		if req.When.Month() == time.November {
			return lookup.TimelineResult{Rows: november}, nil
		}
		return lookup.TimelineResult{Rows: week}, nil
	}
}

// nextMonth commits the floating moment one month on, the way a settled text
// frame does, and returns the model with the timeline's search not yet
// delivered.
func nextMonth(m Model) (Model, tea.Cmd) {
	return applyTextCommit(m, FirstFrameReadyMsg{TransitTime: m.slots.Floating().Time.AddDate(0, 1, 0)})
}

// Acceptance 1: attached, with the report tab on screen, a time key refreshes
// the report exactly as it does detached: the previous text stays as drawn
// until the new report commits (a tap never greys, try of 2026-10-24).
func TestAttachedReportTabRefreshesLikeDetached(t *testing.T) {
	forceColor(t)
	spawns := reportSpawns(t)
	m, _ := enteredFixture(t, 2, true, Config{Color: ColorAlways})
	m, cmd := tpress(t, m, runes("0")) // timeline -> report, attached
	m = pump(t, m, cmd)
	if !m.details.attached || m.activeTabID(PanelDetails) != TabDetailsReport {
		t.Fatalf("fixture: attached=%v tab=%s, want attached on the report", m.details.attached, m.activeTabID(PanelDetails))
	}
	m.details.body = "the previous report"
	base := spawns()
	m, cmd = tpress(t, m, runes("d"))
	if !m.details.pending {
		t.Fatal("a time key attached on the report tab did not mark the report waiting")
	}
	if isDimmed(m) {
		t.Errorf("a tap dimmed the previous report while the new one is awaited:\n%s", detailsPanelText(m))
	}
	m = pump(t, m, cmd)
	if got := spawns() - base; got != 1 {
		t.Fatalf("a time key attached on the report tab: %d report spawns, want 1", got)
	}
	if m.details.pending || m.details.body == "the previous report" || isDimmed(m) {
		t.Errorf("after the commit: pending=%v body=%q dimmed=%v, want the new report, settled", m.details.pending, m.details.body, isDimmed(m))
	}
}

// Acceptance 2: with the timeline tab active, attached or not, a change of
// moment starts no report; choosing the report tab then requests one, once.
func TestHiddenReportAsksOnlyWhenChosen(t *testing.T) {
	for _, attached := range []bool{true, false} {
		spawns := reportSpawns(t)
		m, _ := enteredFixture(t, 2, true, Config{Accessible: true})
		if !attached {
			m, _ = tpress(t, m, keyEsc)
		}
		if m.details.attached != attached || m.activeTabID(PanelDetails) != TabDetailsTimeline {
			t.Fatalf("fixture: attached=%v tab=%s", m.details.attached, m.activeTabID(PanelDetails))
		}
		base := spawns()
		m.cursorIdx = len(m.slots.All()) - 1
		m, cmd := tpress(t, m, runes("d"))
		m = pump(t, m, cmd)
		if got := spawns() - base; got != 0 {
			t.Fatalf("attached=%v: a time key under the timeline tab started %d reports, want 0", attached, got)
		}
		m, cmd = tpress(t, m, runes("0"))
		m = pump(t, m, cmd)
		if got := spawns() - base; got != 1 || m.activeTabID(PanelDetails) != TabDetailsReport {
			t.Errorf("attached=%v: choosing the report tab: %d reports on %s, want 1 on the report", attached, got, m.activeTabID(PanelDetails))
		}
	}
}

// Acceptance 3: with the report or a template tab active the timeline runs no
// search; choosing the timeline tab runs it when its inputs differ from the
// last list's.
func TestTimelineSearchesOnlyWhileItsTabIsActive(t *testing.T) {
	calls := stubTimeline(t, monthRows(t))
	stubTextCompute(t)
	m := loadedTimeline(t, Config{Accessible: true})
	if m.activeTabID(PanelDetails) != TabDetailsReport {
		t.Fatalf("fixture: tab %s, want the report", m.activeTabID(PanelDetails))
	}
	before := len(*calls)
	m, cmd := tpress(t, m, runes("o")) // a month on, under the report tab
	m = pump(t, m, cmd)
	if got := len(*calls) - before; got != 0 {
		t.Fatalf("a month step under the report tab ran %d timeline searches, want 0", got)
	}
	m, cmd = tpress(t, m, runes("0"))
	m = pump(t, m, cmd)
	if got := len(*calls) - before; got != 1 || !m.timeline.complete || len(m.timeline.rows) != 1 {
		t.Fatalf("choosing the timeline tab ran %d searches (complete=%v rows=%d), want 1 and November's row", got, m.timeline.complete, len(m.timeline.rows))
	}
	// Choosing it again with unchanged inputs searches nothing.
	m, cmd = tpress(t, m, runes("0"))
	m, cmd2 := tpress(t, m, runes("0"))
	m = pump(t, m, cmd, cmd2)
	if got := len(*calls) - before; got != 1 {
		t.Errorf("tab switches with unchanged inputs: %d searches in all, want still 1", got)
	}
}

// Acceptance 4: while the timeline is active and its inputs change, the
// previous list stays on screen dimmed until the new list commits; marks and
// the query clear.
func TestTimelineKeepsThePreviousListDimmedUntilTheNewOneCommits(t *testing.T) {
	forceColor(t)
	stubTimeline(t, monthRows(t))
	stubTextCompute(t)
	m := loadedTimeline(t, Config{Color: ColorAlways})
	m = m.withDetailsTab(PanelChart, TabDetailsTimeline)
	if isDimmed(m) {
		t.Fatal("fixture: a settled list is dimmed")
	}
	m.timeline.query = "Moon"
	m.timeline.marks = map[timelineMark]bool{markOf(m.timeline.rows[0]): true}
	m, cmd := nextMonth(m)
	if len(m.timeline.rows) != 0 || len(m.timeline.marks) != 0 || m.timeline.query != "" {
		t.Fatalf("a new month kept rows=%d marks=%d query=%q, want a fresh list", len(m.timeline.rows), len(m.timeline.marks), m.timeline.query)
	}
	panel := detailsPanelText(m)
	if !strings.Contains(panel, "Moon") || !isDimmed(m) {
		t.Errorf("the previous list is not on screen dimmed while the new one is searched:\n%s", panel)
	}
	m = pump(t, m, cmd)
	panel = detailsPanelText(m)
	if isDimmed(m) || len(m.timeline.rows) != 1 || len(m.timeline.kept) != 0 {
		t.Errorf("after the commit the new list should replace the old, undimmed:\n%s", panel)
	}
}

// Acceptance 4, last sentence: a change of the compared charts keeps nothing.
func TestTimelineClearsWhenTheComparedChartsChange(t *testing.T) {
	stubTimeline(t, nil)
	stubTextCompute(t)
	m := loadedTimeline(t, Config{Color: ColorNever})
	m = m.withDetailsTab(PanelChart, TabDetailsTimeline)
	if len(m.timelineDisplayRows()) == 0 {
		t.Fatal("fixture: no list")
	}
	// Space on the floating slot deselects it: the compared set changes.
	m, cmd := tpress(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if got := len(m.timelineDisplayRows()); got != 0 {
		t.Fatalf("%d previous rows on screen while the new search for other compared charts is in flight, want none", got)
	}
	m, cmd2 := m.syncTimeline()
	if got := len(m.timelineDisplayRows()); got != 0 || len(m.timeline.kept) != 0 {
		t.Fatalf("after the re-key: %d rows shown, %d kept, want none", got, len(m.timeline.kept))
	}
	_ = pump(t, m, cmd, cmd2)
}

// Acceptance 5: a template tab given a new moment keeps its previous lines
// dimmed until the new lines commit; a different body or template clears them.
func TestTemplateTabKeepsItsLinesDimmedForANewMoment(t *testing.T) {
	forceColor(t)
	stubBodyFacts(t, nil)
	m := templateTestModel(t)
	m.cfg = Config{Color: ColorAlways}
	m = runTemplate(t, activeOnTemplate(m))
	if len(m.tpl.lines) == 0 || m.detailsTabWaiting(m.activeTabID(PanelDetails)) || isDimmed(m) {
		t.Fatalf("fixture: lines=%d waiting=%v dimmed=%v, want settled lines", len(m.tpl.lines), m.detailsTabWaiting(m.activeTabID(PanelDetails)), isDimmed(m))
	}
	kept := len(m.tpl.lines)
	m.slots.SetFloatingTime(m.slots.Floating().Time.Add(time.Hour))
	m, _ = m.syncTemplate()
	if len(m.tpl.lines) != kept || !m.detailsTabWaiting(m.activeTabID(PanelDetails)) || !isDimmed(m) {
		t.Fatalf("new moment: lines=%d (want %d) waiting=%v dimmed=%v, want the previous lines dimmed", len(m.tpl.lines), kept, m.detailsTabWaiting(m.activeTabID(PanelDetails)), isDimmed(m))
	}
	m = runTemplate(t, m)
	if m.detailsTabWaiting(m.activeTabID(PanelDetails)) || isDimmed(m) {
		t.Errorf("after the commit the tab still waits or dims")
	}
	// Another body: the lines go at once, and a first load shows no dimming.
	m.tpl.key.body = aspect.Mercury
	m, _ = m.syncTemplate()
	if len(m.tpl.lines) != 0 || isDimmed(m) {
		t.Errorf("another body: lines=%d dimmed=%v, want none and undimmed", len(m.tpl.lines), isDimmed(m))
	}
}

// Acceptance 6: with colours off a waiting active tab's title ends in ` …`,
// for all three tabs, and a settled or hidden one does not.
func TestWaitingActiveTabTitleEndsInAnEllipsis(t *testing.T) {
	// Report.
	m := detailsTestModel(t)
	m.cfg = Config{Accessible: true}
	if strings.HasSuffix(stripTitle(m), detailsPendingMark) {
		t.Fatalf("a settled report is marked: %q", stripTitle(m))
	}
	m.details.pending = true
	if !strings.HasSuffix(stripTitle(m), detailsPendingMark) {
		t.Errorf("a waiting report is not marked: %q", stripTitle(m))
	}
	// Timeline.
	stubTimeline(t, nil)
	tm := timelineModel(t, Config{Accessible: true}).withDetailsTab(PanelChart, TabDetailsTimeline)
	if !strings.HasSuffix(stripTitle(tm), detailsPendingMark) {
		t.Errorf("a timeline owed its search is not marked: %q", stripTitle(tm))
	}
	tm, cmd := tm.syncTimeline()
	tm = pump(t, tm, cmd)
	if strings.HasSuffix(stripTitle(tm), detailsPendingMark) {
		t.Errorf("a settled timeline is marked: %q", stripTitle(tm))
	}
	// A waiting report is not marked while another tab is the active one.
	tm.details.pending = true
	if strings.HasSuffix(stripTitle(tm), detailsPendingMark) {
		t.Errorf("a hidden waiting report marked the active timeline tab: %q", stripTitle(tm))
	}
	// Template.
	stubBodyFacts(t, nil)
	pm := templateTestModel(t)
	pm.cfg = Config{Accessible: true}
	pm = activeOnTemplate(pm)
	pm, _ = pm.syncTemplate()
	if !strings.HasSuffix(stripTitle(pm), detailsPendingMark) {
		t.Errorf("a template tab awaiting its first run is not marked: %q", stripTitle(pm))
	}
	pm = runTemplate(t, pm)
	if strings.HasSuffix(stripTitle(pm), detailsPendingMark) {
		t.Errorf("a settled template tab is marked: %q", stripTitle(pm))
	}
}

// Acceptance 7: while the active tab has a row cursor and is waiting, its row
// keys do nothing, and the other keys act as they do today.
func TestWaitingTimelineIgnoresItsRowKeys(t *testing.T) {
	stubTimeline(t, monthRows(t))
	stubTextCompute(t)
	m, _ := enteredFixture(t, 2, true, Config{Accessible: true})
	// A new month: the list is waiting, attached, the cursor on row 0.
	m, cmd := nextMonth(m)
	if !m.detailsTabWaiting(TabDetailsTimeline) || !m.details.attached {
		t.Fatalf("fixture: waiting=%v attached=%v", m.detailsTabWaiting(TabDetailsTimeline), m.details.attached)
	}
	floating := m.slots.Floating().Time
	cursor, offset := m.details.cursor, m.details.rowOffset
	marks := len(m.timeline.marks)
	for name, key := range map[string]tea.KeyMsg{
		"down": keyDown, "up": keyUp, "j": runes("j"), "k": runes("k"), "enter": keyEnter,
		"space": {Type: tea.KeySpace}, "slash": runes("/"), "pgdn": keyPgDn, "pgup": keyPgUp,
	} {
		var c tea.Cmd
		m, c = tpress(t, m, key)
		if c != nil {
			t.Errorf("%s while waiting returned a command", name)
		}
		if m.details.cursor != cursor || m.details.rowOffset != offset || len(m.timeline.marks) != marks || m.timeline.searching || m.timeline.query != "" || !m.slots.Floating().Time.Equal(floating) {
			t.Errorf("%s while waiting changed cursor %d->%d offset %d->%d marks %d->%d searching=%v query=%q floating %v->%v",
				name, cursor, m.details.cursor, offset, m.details.rowOffset, marks, len(m.timeline.marks), m.timeline.searching, m.timeline.query, floating, m.slots.Floating().Time)
		}
	}
	// A time key still acts (the floating moment moves).
	m, c := tpress(t, m, runes("d"))
	if m.slots.Floating().Time.Equal(floating) && m.targetFloatingTime.Equal(floating) {
		t.Error("a time key was ignored while waiting")
	}
	// Esc still detaches.
	m2, _ := tpress(t, m, keyEsc)
	if m2.details.attached {
		t.Error("Esc did not detach a waiting timeline")
	}
	// Settled again, the row keys work: Down moves the cursor.
	m = pump(t, m, cmd, c)
	if m.detailsTabWaiting(TabDetailsTimeline) {
		t.Fatal("fixture: still waiting after the commit")
	}
	m.details.cursor = 0
	m, _ = tpress(t, m, keyDown)
	if m.details.cursor == 0 && len(m.timelineShown()) > 1 {
		t.Error("Down did nothing once the list settled")
	}
}

// Acceptance 8: a timeline search that fails while the list is dimmed removes
// the dimmed list, logs as today, and the row keys work again.
func TestFailedSearchRemovesTheDimmedList(t *testing.T) {
	forceColor(t)
	week := weekRows(t)
	stubTimeline(t, func(req lookup.TimelineRequest) (lookup.TimelineResult, error) {
		if req.When.Month() == time.November {
			return lookup.TimelineResult{Commands: []string{"astrolog -dm"}}, errors.New("astrolog failed")
		}
		return lookup.TimelineResult{Rows: week}, nil
	})
	stubTextCompute(t)
	m := loadedTimeline(t, Config{Color: ColorAlways})
	m = m.withDetailsTab(PanelChart, TabDetailsTimeline)
	m, cmd := nextMonth(m)
	if !isDimmed(m) {
		t.Fatal("fixture: the previous list is not dimmed")
	}
	errs := 0
	for _, l := range m.logLines {
		if l.lane == laneError {
			errs++
		}
	}
	m = pump(t, m, cmd)
	got := 0
	for _, l := range m.logLines {
		if l.lane == laneError && strings.HasPrefix(l.text, "Couldn't read the timeline.") {
			got++
		}
	}
	if got != errs+1 {
		t.Errorf("the failed search logged %d timeline errors, want 1", got-errs)
	}
	if isDimmed(m) || len(m.timeline.kept) != 0 || len(m.timelineDisplayRows()) != 0 || m.detailsTabWaiting(TabDetailsTimeline) || m.detailsRowKeysBlocked() {
		t.Errorf("after the failure: dimmed=%v waiting=%v blocked=%v, want the dimmed list gone and the keys free:\n%s",
			isDimmed(m), m.detailsTabWaiting(TabDetailsTimeline), m.detailsRowKeysBlocked(), detailsPanelText(m))
	}
}

// Acceptance 9: a first load with nothing to keep shows no dimming.
func TestFirstLoadIsNotDimmed(t *testing.T) {
	forceColor(t)
	stubTimeline(t, nil)
	m := timelineModel(t, Config{Color: ColorAlways}).withDetailsTab(PanelChart, TabDetailsTimeline)
	m, cmd := m.syncTimeline()
	if !m.detailsTabWaiting(TabDetailsTimeline) {
		t.Fatal("fixture: the first search is not awaited")
	}
	if isDimmed(m) {
		t.Errorf("a first load with nothing to keep is dimmed:\n%s", detailsPanelText(m))
	}
	m = pump(t, m, cmd)
	if len(m.timeline.rows) == 0 || isDimmed(m) {
		t.Errorf("after the first load: rows=%d dimmed=%v", len(m.timeline.rows), isDimmed(m))
	}
}

// Acceptance 10: the entered row's marks read the timeline list only while it
// is for the current inputs.
func TestEnteredRowMarksReadOnlyACurrentList(t *testing.T) {
	stubTimeline(t, monthRows(t))
	stubTextCompute(t)
	m, _ := enteredFixture(t, 2, true, Config{Accessible: true})
	m = enter(t, m, chartRowOf(t, m.timeline.rows, 2))
	if _, ok := m.enteredTimelineRow(); !ok {
		t.Fatal("fixture: the pressed row is not found in its own list")
	}
	// Inputs change: the kept (dimmed) list must not answer.
	m.slots.SetFloatingTime(m.slots.Floating().Time.AddDate(0, 1, 0))
	m, _ = m.syncTimeline()
	if len(m.timeline.kept) == 0 {
		t.Fatal("fixture: no kept list to read")
	}
	if row, ok := m.enteredTimelineRow(); ok {
		t.Errorf("the entered row was read from a list for other inputs: %+v", row)
	}
}
