package ui

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/lookup"
)

// refresh-descriptor-timeline: the Timeline surface's refresh policy is one
// row value, and the coordinator reaches it only by asking
// (specs/refresh-descriptor-timeline.md).

// timelineRunLog dumps the Log of one Timeline search: "ok" is a search that
// lands, "fail" one that is refused, "left" a result that arrives after the
// operator moved to another month.
func timelineRunLog(t *testing.T, mode string) []string {
	t.Helper()
	freshModels(t)
	week := weekRows(t)
	stubTimeline(t, func(req lookup.TimelineRequest) (lookup.TimelineResult, error) {
		cmds := []string{"astrolog -timeline " + req.When.Month().String()}
		if mode == "fail" {
			return lookup.TimelineResult{Commands: cmds}, errors.New("astrolog failed")
		}
		return lookup.TimelineResult{Rows: week, Commands: cmds}, nil
	})
	m := timelineModel(t, Config{Color: ColorNever})
	m, cmd := syncTimelineShown(m)
	m = pump(t, m, cmd)
	if mode == "left" {
		// A result whose run and want are current but whose inputs the
		// operator has left: it paints nothing, yet its argv line is written.
		m.logLines = nil
		left := refreshResultMsg{surface: surfaceTimeline, runID: m.refresh[surfaceTimeline].runningRunID, wantID: m.refresh[surfaceTimeline].wantID,
			payload: timelinePayload{key: timelineKey{year: 2020, month: time.May, zone: "Europe/Amsterdam"}, rows: week, commands: []string{"astrolog -timeline May 2020"}}}
		m, _ = m.onRefreshResult(left)
	}
	var out []string
	for _, l := range m.logLines {
		out = append(out, l.text)
	}
	return out
}

// Acceptance 3. The Log of a search that lands, one that is refused, and a
// result for a left key (which paints nothing and still writes its argv line).
// The literals were captured at play 1fda7ccae381, before the row existed.
func TestTimelineRunLogTranscript(t *testing.T) {
	cases := []struct {
		mode string
		want []string
	}{
		{"ok", []string{"astrolog -timeline October"}},
		{"fail", []string{"astrolog -timeline October", "timeline: the timeline not read, Astrolog returned nothing usable"}},
		{"left", []string{"astrolog -timeline May 2020"}},
	}
	for _, c := range cases {
		got := timelineRunLog(t, c.mode)
		if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("%s: Log %q, want %q", c.mode, got, c.want)
		}
	}
}

// Acceptance 1. refresh.go holds no Timeline branch: the coordinator asks the
// row. rowFor alone names the surface.
func TestSurfaceTimelineIsOnlyReachedThroughItsRow(t *testing.T) {
	src, err := os.ReadFile("refresh.go")
	if err != nil {
		t.Fatal(err)
	}
	if got := surfaceBranches(t, "refresh.go", string(src), "surfaceTimeline", "rowFor"); len(got) > 0 {
		t.Errorf("refresh.go branches on surfaceTimeline outside the row (%d): %s", len(got), strings.Join(got, ", "))
	}
}

// Wrong-name control: the scanner over a fixture with one planted Timeline
// branch names it, leaves rowFor alone, and does not report a Details branch.
func TestSurfaceBranchScannerNamesAPlantedTimelineBranch(t *testing.T) {
	const fixture = `package ui
func rowFor(s surface) bool { return s == surfaceTimeline }
func planted(s surface) int {
	switch s {
	case surfaceDetails:
		return 1
	case surfaceTimeline:
		return 2
	}
	return 0
}
func gate(s surface) bool { return s == surfaceTimeline || s == surfaceTemplate }
`
	got := surfaceBranches(t, "fixture.go", fixture, "surfaceTimeline", "rowFor")
	if len(got) != 2 || got[0] != "planted:7" || got[1] != "gate:12" {
		t.Fatalf("scanner reported %v, want [planted:7 gate:12]", got)
	}
}

// Acceptance 4. The row reads the old constant, has no rest timer, and the
// coordinator reads the row.
func TestTimelineRowKeepsTheDebounce(t *testing.T) {
	r, ok := defaultRowOf(surfaceTimeline)
	if !ok {
		t.Fatal("Timeline has no row")
	}
	if r.debounce != 50*time.Millisecond || debounceOf(surfaceTimeline) != 50*time.Millisecond {
		t.Errorf("row debounce %v, debounceFor %v, want 50ms", r.debounce, debounceOf(surfaceTimeline))
	}
	if r.trigger != triggerIdentity || r.supersede != supersedeHard {
		t.Errorf("trigger %v supersede %v", r.trigger, r.supersede)
	}
	if r.rest.armed != nil || r.rest.hidden != nil || r.rest.park != nil || r.rest.disarm != nil {
		t.Error("Timeline has a rest timer")
	}
}

// Acceptance 4. A Timeline tick takes the ensureRunningLatest path: with a
// want nothing has run for, it starts the run; it never reaches a rest timer.
func TestTimelineTickTakesTheEnsureRunningLatestPath(t *testing.T) {
	stubTimeline(t, nil)
	m := timelineModel(t, Config{Accessible: true})
	m, _ = syncTimelineShown(m) // keyed, run started
	s := m.refresh[surfaceTimeline]
	m.refresh[surfaceTimeline].cancel = nil // idle, the want not yet committed
	m.refresh[surfaceTimeline].lastCommitWantID = s.wantID - 1
	before := m.refresh[surfaceTimeline].runningRunID
	m, cmd := m.onRefreshTick(refreshTickMsg{surface: surfaceTimeline, timerID: s.timerID})
	if cmd == nil || m.refresh[surfaceTimeline].runningRunID != before+1 {
		t.Errorf("tick did not start the run: cmd nil=%v run %d, was %d", cmd == nil, m.refresh[surfaceTimeline].runningRunID, before)
	}
	// A tick another input re-armed is ignored.
	m2, cmd2 := m.onRefreshTick(refreshTickMsg{surface: surfaceTimeline, timerID: s.timerID - 1})
	if cmd2 != nil || m2.refresh[surfaceTimeline].runningRunID != m.refresh[surfaceTimeline].runningRunID {
		t.Error("a stale tick started a run")
	}
}

// The row's Inputs column is the key the commit gate compares, and its accept
// is the exact want: a result for an older want is dropped, the current one
// is taken, and the key is NOT part of accept (commit logs argv first).
func TestTimelineRowInputsAndAccept(t *testing.T) {
	stubTimeline(t, nil)
	m := timelineModel(t, Config{Accessible: true})
	m, _ = syncTimelineShown(m)
	r, _ := defaultRowOf(surfaceTimeline)
	k1, ok1 := r.inputs(m)
	_, k2, ok2 := m.timelineInputs()
	if !ok1 || !ok2 || k1 != refreshKey(k2) || k2 != m.timeline.key {
		t.Fatalf("row inputs %v/%v, timelineInputs %v/%v", k1, ok1, k2, ok2)
	}
	if k3, ok3 := m.timelineKeyFor(); !ok3 || k3 != k2 {
		t.Errorf("timelineKeyFor %v/%v, want %v", k3, ok3, k2)
	}
	cur := refreshResultMsg{surface: surfaceTimeline, wantID: m.refresh[surfaceTimeline].wantID}
	if !r.accept(m, cur) {
		t.Error("accept refused the current want")
	}
	old := cur
	old.wantID--
	if r.accept(m, old) {
		t.Error("accept took a result for an older want")
	}
	// Not in accept: a left key passes the gate and is dropped by commit.
	left := cur
	left.payload = timelinePayload{key: timelineKey{year: 1999}}
	if !r.accept(m, left) {
		t.Error("accept looked at the key; commit must, after the argv lines")
	}
	m.slots = nil
	if _, ok := r.inputs(m); ok {
		t.Error("inputs answered with no workspace")
	}
}

// Acceptance 5. Template keeps its own want gate in the coordinator: a result
// for an older want drops and lands nothing.
func TestTemplateWantGateDropsAStaleResult(t *testing.T) {
	stubBodyFacts(t, nil)
	m := activeOnTemplate(templateTestModel(t))
	m, _ = m.syncTemplate()
	st := m.refresh[surfaceTemplate]
	cmd := buildTemplateCmd(m, context.Background(), st.runningRunID, st.wantID)
	if cmd == nil {
		t.Fatal("buildTemplateCmd returned nil")
	}
	msg := cmd().(refreshResultMsg)
	m.refresh[surfaceTemplate].wantID++ // a newer want arrived while it ran
	m, _ = m.onRefreshResult(msg)
	if m.tpl.hasDone {
		t.Error("a result for an older want landed")
	}
	m.refresh[surfaceTemplate].wantID--
	m, _ = m.onRefreshResult(msg)
	if !m.tpl.hasDone {
		t.Error("the current result did not land")
	}
}
