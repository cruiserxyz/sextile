package ui

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// refresh-descriptor-template: the Template surface's refresh policy is one
// row value, and the coordinator reaches it only by asking
// (specs/refresh-descriptor-template.md).

// templateRunLog dumps the Log of one Template run: "ok" is a run that lands,
// "fail" one that is refused, "left" a result that arrives after the inputs
// moved on (it paints nothing, yet writes its argv line).
func templateRunLog(t *testing.T, mode string) []string {
	t.Helper()
	freshModels(t)
	var fail error
	if mode == "fail" {
		fail = errors.New("astrolog failed")
	}
	stubBodyFacts(t, fail)
	m := activeOnTemplate(templateTestModel(t))
	m, cmd := m.syncTemplate()
	m = pump(t, m, cmd)
	if mode == "left" {
		m.logLines = nil
		key := m.tpl.key
		key.when++
		left := refreshResultMsg{surface: surfaceTemplate, runID: m.refresh[surfaceTemplate].runningRunID, wantID: m.refresh[surfaceTemplate].wantID,
			payload: templatePayload{key: key, commands: []string{"astrolog -left"}}}
		m, _ = m.onRefreshResult(left)
	}
	var out []string
	for _, l := range m.logLines {
		out = append(out, l.text)
	}
	return out
}

// Acceptance 3. The Log of a run that lands, one that is refused, and a result
// for a left key (which paints nothing and still writes its argv line).
// The literals were captured at play 89834f2f2b54, before the row existed.
func TestTemplateRunLogTranscript(t *testing.T) {
	cases := []struct {
		mode string
		want []string
	}{
		{"ok", []string{"astrolog -z 0 -qy … -dY 13 -b0", "astrolog -qj … -b0 -v -Z -R0 1 4 -RA0 1",
			"template: body-inferior-planet verdicts from rule rows solar.beams/17, solar.beams/12, solar.combust/8.5"}},
		{"fail", []string{"astrolog -z 0 -qy … -dY 13 -b0", "astrolog -qj … -b0 -v -Z -R0 1 4 -RA0 1",
			"details: the body-inferior-planet tab did not fill, Astrolog returned nothing usable"}},
		{"left", []string{"astrolog -left"}},
	}
	for _, c := range cases {
		got := templateRunLog(t, c.mode)
		if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("%s: Log %q, want %q", c.mode, got, c.want)
		}
	}
}

// Acceptance 1. refresh.go holds no Template branch: the coordinator asks the
// row. rowFor alone names the surface.
func TestSurfaceTemplateIsOnlyReachedThroughItsRow(t *testing.T) {
	src, err := os.ReadFile("refresh.go")
	if err != nil {
		t.Fatal(err)
	}
	if got := surfaceBranches(t, "refresh.go", string(src), "surfaceTemplate", "rowFor"); len(got) > 0 {
		t.Errorf("refresh.go branches on surfaceTemplate outside the row (%d): %s", len(got), strings.Join(got, ", "))
	}
}

// Wrong-name control: the scanner over a fixture with one planted Template
// branch of each shape names them, leaves rowFor alone, and does not report a
// Timeline branch.
func TestSurfaceBranchScannerNamesAPlantedTemplateBranch(t *testing.T) {
	const fixture = `package ui
func rowFor(s surface) bool { return s == surfaceTemplate }
func planted(s surface) int {
	switch s {
	case surfaceTimeline:
		return 1
	case surfaceTemplate:
		return 2
	}
	return 0
}
func gate(s surface) bool { return s == surfaceTemplate || s == surfaceTimeline }
`
	got := surfaceBranches(t, "fixture.go", fixture, "surfaceTemplate", "rowFor")
	if len(got) != 2 || got[0] != "planted:7" || got[1] != "gate:12" {
		t.Fatalf("scanner reported %v, want [planted:7 gate:12]", got)
	}
}

// Acceptance 4. The row reads the old constant, has no rest timer, and the
// coordinator reads the row.
func TestTemplateRowKeepsTheDebounce(t *testing.T) {
	r, ok := defaultRowOf(surfaceTemplate)
	if !ok {
		t.Fatal("Template has no row")
	}
	if r.debounce != 50*time.Millisecond || debounceOf(surfaceTemplate) != 50*time.Millisecond {
		t.Errorf("row debounce %v, debounceFor %v, want 50ms", r.debounce, debounceOf(surfaceTemplate))
	}
	if r.trigger != triggerIdentity || r.supersede != supersedeHard {
		t.Errorf("trigger %v supersede %v", r.trigger, r.supersede)
	}
	if r.rest.armed != nil || r.rest.hidden != nil || r.rest.park != nil || r.rest.disarm != nil {
		t.Error("Template has a rest timer")
	}
}

// Acceptance 4. A Template tick takes the ensureRunningLatest path.
func TestTemplateTickTakesTheEnsureRunningLatestPath(t *testing.T) {
	stubBodyFacts(t, nil)
	m := activeOnTemplate(templateTestModel(t))
	m, _ = m.syncTemplate() // keyed, run started
	s := m.refresh[surfaceTemplate]
	m.refresh[surfaceTemplate].cancel = nil // idle, the want not yet committed
	m.refresh[surfaceTemplate].lastCommitWantID = s.wantID - 1
	before := m.refresh[surfaceTemplate].runningRunID
	m, cmd := m.onRefreshTick(refreshTickMsg{surface: surfaceTemplate, timerID: s.timerID})
	if cmd == nil || m.refresh[surfaceTemplate].runningRunID != before+1 {
		t.Errorf("tick did not start the run: cmd nil=%v run %d, was %d", cmd == nil, m.refresh[surfaceTemplate].runningRunID, before)
	}
	m2, cmd2 := m.onRefreshTick(refreshTickMsg{surface: surfaceTemplate, timerID: s.timerID - 1})
	if cmd2 != nil || m2.refresh[surfaceTemplate].runningRunID != m.refresh[surfaceTemplate].runningRunID {
		t.Error("a stale tick started a run")
	}
}

// The row's Inputs column is the key the commit compares, and its accept is
// the exact want; the key is not part of accept (commit logs argv first).
func TestTemplateRowInputsAndAccept(t *testing.T) {
	stubBodyFacts(t, nil)
	m := activeOnTemplate(templateTestModel(t))
	m, _ = m.syncTemplate()
	r, _ := defaultRowOf(surfaceTemplate)
	k1, ok1 := r.inputs(m)
	_, _, k2, ok2 := m.templateInputs()
	if !ok1 || !ok2 || k1 != refreshKey(k2) || k2 != m.tpl.key {
		t.Fatalf("row inputs %v/%v, templateInputs %v/%v", k1, ok1, k2, ok2)
	}
	if k3, ok3 := m.templateKeyFor(); !ok3 || k3 != k2 {
		t.Errorf("templateKeyFor %v/%v, want %v", k3, ok3, k2)
	}
	cur := refreshResultMsg{surface: surfaceTemplate, wantID: m.refresh[surfaceTemplate].wantID}
	if !r.accept(m, cur) {
		t.Error("accept refused the current want")
	}
	old := cur
	old.wantID--
	if r.accept(m, old) {
		t.Error("accept took a result for an older want")
	}
	left := cur
	left.payload = templatePayload{key: templateKey{template: "elsewhere"}}
	if !r.accept(m, left) {
		t.Error("accept looked at the key; commit must, after the argv lines")
	}
	m.slots = nil
	if _, ok := r.inputs(m); ok {
		t.Error("inputs answered with no workspace")
	}
}

// Acceptance 5. A Template run in flight when Details leaves the layout: the
// row's suspend fences the surface and forgets the key, the run's late result
// is dropped, and m.tpl holds no partial content.
func TestTemplateSuspendMidRunDropsTheLateResult(t *testing.T) {
	stubBodyFacts(t, nil)
	m := activeOnTemplate(templateTestModel(t))
	m, cmd := m.syncTemplate()
	if cmd == nil || m.tpl.complete {
		t.Fatal("no run in flight")
	}
	st := m.refresh[surfaceTemplate]
	build := buildTemplateCmd(m, context.Background(), st.runningRunID, st.wantID)
	if build == nil {
		t.Fatal("buildTemplateCmd returned nil")
	}
	late := build().(refreshResultMsg)
	m = m.suspendDetails()
	if m.tpl.keyed || m.tpl.complete || len(m.tpl.lines) != 0 || m.tpl.hasDone {
		t.Errorf("m.tpl holds partial content after suspend: %+v", m.tpl)
	}
	logs := len(m.logLines)
	m, _ = m.onRefreshResult(late)
	if m.tpl.hasDone || m.tpl.complete || len(m.tpl.lines) != 0 || len(m.logLines) != logs {
		t.Errorf("the late result landed: %+v, %d log lines, was %d", m.tpl, len(m.logLines), logs)
	}
}

// Acceptance 5. A complete frame cancelled with its lines still the previous
// moment's forgets its key (the lines stay), through the row's suspend.
func TestTemplateSuspendKeepsCompleteLinesAndForgetsAStaleKey(t *testing.T) {
	stubBodyFacts(t, nil)
	m := runTemplate(t, activeOnTemplate(templateTestModel(t)))
	m.tpl.complete = true
	r, _ := defaultRowOf(surfaceTemplate)
	same := r.suspend(m)
	if !same.tpl.keyed || !same.tpl.complete {
		t.Errorf("a frame done for its own key was forgotten: %+v", same.tpl)
	}
	m.tpl.done.when++
	stale := r.suspend(m)
	if stale.tpl.keyed || !stale.tpl.complete {
		t.Errorf("a stale frame kept its key or lost its lines: keyed=%v complete=%v", stale.tpl.keyed, stale.tpl.complete)
	}
}
