package ui

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// refresh-descriptor-image: the Image surface's refresh policy is one row
// value, and the coordinator reaches it only by asking
// (specs/refresh-descriptor-image.md).

// imageRunLog dumps the Log of one wheel run. "ok" is a landed multi-ring
// render with two argv lines, "fail" a refused one, "stale" a stale success
// of a held time step (it publishes and chains a follow-up). The literals
// below were captured at play f17968c39725, before the row existed.
func imageRunLog(t *testing.T, mode string) []string {
	t.Helper()
	freshModels(t)
	stubShowWheel(t)
	m := modelWithSlot(t)
	m.previewActive = true
	m, _ = m.requestImageNow()
	st := imageState(m)
	msg := refreshResultMsg{
		surface: surfaceImage, runID: st.runningRunID, wantID: st.wantID,
		payload: imagePayload{bytes: []byte("<svg/>"), commands: []string{"/bin/astrolog ring one", "/bin/astrolog ring two"}},
	}
	switch mode {
	case "fail":
		msg.err = errors.New("astrolog failed")
	case "stale":
		m, _ = m.requestImage() // want 2 while run 1 is in flight
		msg.wantID = 1
	}
	m, _ = m.onRefreshResult(msg)
	if mode == "stale" {
		f := imageState(m)
		m, _ = m.onRefreshResult(refreshResultMsg{
			surface: surfaceImage, runID: f.runningRunID, wantID: f.wantID,
			payload: imagePayload{bytes: []byte("<svg/>"), commands: []string{"/bin/astrolog follow-up"}},
		})
	}
	var out []string
	for _, l := range m.logLines {
		out = append(out, l.text)
	}
	return out
}

func TestImageRunLogTranscript(t *testing.T) {
	cases := []struct {
		mode string
		want []string
	}{
		{"ok", []string{"/bin/astrolog ring one", "/bin/astrolog ring two"}},
		{"fail", []string{"/bin/astrolog ring one", "/bin/astrolog ring two", "wheel: wheel not drawn, Astrolog could not render it"}},
		{"stale", []string{"/bin/astrolog ring one", "/bin/astrolog ring two", "/bin/astrolog follow-up"}},
	}
	for _, c := range cases {
		got := imageRunLog(t, c.mode)
		if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("%s: Log %q, want %q", c.mode, got, c.want)
		}
	}
}

// Acceptance 1. refresh.go holds no Image branch: the coordinator asks the
// row. rowFor alone names the surface.
func TestSurfaceImageIsOnlyReachedThroughItsRow(t *testing.T) {
	src, err := os.ReadFile("refresh.go")
	if err != nil {
		t.Fatal(err)
	}
	if got := surfaceBranches(t, "refresh.go", string(src), "surfaceImage", "rowFor"); len(got) > 0 {
		t.Errorf("refresh.go branches on surfaceImage outside the row (%d): %s", len(got), strings.Join(got, ", "))
	}
}

// Wrong-name control: one planted Image branch of each shape is named, rowFor
// is left alone, and a Text branch is not reported.
func TestSurfaceBranchScannerNamesAPlantedImageBranch(t *testing.T) {
	const fixture = `package ui
func rowFor(s surface) bool { return s == surfaceImage }
func planted(s surface) int {
	switch s {
	case surfaceText:
		return 1
	case surfaceImage:
		return 2
	}
	return 0
}
func gate(s surface) bool { return (s == surfaceImage || s == surfaceText) && s != surfaceTemplate }
`
	got := surfaceBranches(t, "fixture.go", fixture, "surfaceImage", "rowFor")
	if len(got) != 2 || got[0] != "planted:7" || got[1] != "gate:12" {
		t.Fatalf("scanner reported %v, want [planted:7 gate:12]", got)
	}
}

// Acceptance 2. The table is complete: every surface has a row, and refresh.go
// names none of the five outside rowFor.
func TestRefreshTableIsComplete(t *testing.T) {
	src, err := os.ReadFile("refresh.go")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"surfaceImage", "surfaceText", "surfaceDetails", "surfaceTimeline", "surfaceTemplate"}
	for i, name := range names {
		if got := surfaceBranches(t, "refresh.go", string(src), name, "rowFor"); len(got) > 0 {
			t.Errorf("refresh.go branches on %s outside the row: %s", name, strings.Join(got, ", "))
		}
		_ = i
	}
	for s := surface(0); s < numSurfaces; s++ {
		r, ok := defaultRowOf(s)
		if !ok {
			t.Errorf("surface %d has no row", s)
			continue
		}
		if r.build == nil || r.accept == nil || r.commit == nil {
			t.Errorf("surface %d: row lacks build, accept or commit", s)
		}
	}
	if _, ok := defaultRowOf(numSurfaces); ok {
		t.Error("a surface past numSurfaces has a row")
	}
}

// The row's columns as the spec settles them.
func TestImageRowColumns(t *testing.T) {
	r, ok := defaultRowOf(surfaceImage)
	if !ok {
		t.Fatal("Image has no row")
	}
	if r.debounce != 150*time.Millisecond || debounceOf(surfaceImage) != 150*time.Millisecond {
		t.Errorf("row debounce %v, debounceFor %v, want 150ms", r.debounce, debounceOf(surfaceImage))
	}
	if r.trigger != triggerEventLeading || r.supersede != supersedeLiveSerial {
		t.Errorf("trigger %v supersede %v", r.trigger, r.supersede)
	}
	if r.inputs != nil || r.suspend != nil {
		t.Error("Image has inputs or a suspend; the spec settles both as nil")
	}
	if r.rest.armed != nil || r.rest.hidden != nil || r.rest.park != nil || r.rest.disarm != nil {
		t.Error("Image has a rest timer")
	}
	if r.build == nil || r.accept == nil || r.commit == nil || r.started == nil {
		t.Error("Image row lacks build, accept, commit or started")
	}
}

// The row's started column stamps the run key, and only a run that builds a
// command stamps it.
func TestImageStartedStampsTheRunKey(t *testing.T) {
	stubShowWheel(t)
	m := modelWithSlot(t)
	m.previewActive = true
	m, _ = m.requestImageNow()
	want, ok := m.wheelStillKey()
	if !ok || want == "" || m.wheelRunKey != want {
		t.Fatalf("wheelRunKey %q, want %q", m.wheelRunKey, want)
	}
	// Nothing to render (preview off): no command, so no stamp.
	n := modelWithSlot(t)
	n.wheelRunKey = "kept"
	n, _ = n.requestImageNow()
	if n.wheelRunKey != "kept" {
		t.Errorf("a run with no command restamped the key: %q", n.wheelRunKey)
	}
}

// Acceptance 6. A stale error restarts silently.
func TestImageStaleErrorRestartsSilently(t *testing.T) {
	stubShowWheel(t)
	m := modelWithSlot(t)
	m.previewActive = true
	m, _ = m.requestImageNow()
	run := imageState(m).runningRunID
	m, _ = m.requestImage() // want 2
	lines := len(m.logLines)
	m, cmd := m.onRefreshResult(refreshResultMsg{surface: surfaceImage, runID: run, wantID: 1, err: errors.New("boom")})
	if cmd == nil || !m.previewActive || len(m.logLines) != lines {
		t.Errorf("stale error: follow-up nil=%v previewActive=%v lines %d->%d", cmd == nil, m.previewActive, lines, len(m.logLines))
	}
	if st := imageState(m); st.runningRunID != run+1 || st.runningWantID != 2 || st.lastCommitWantID != 0 {
		t.Errorf("stale error: run %d want %d lastCommit %d", st.runningRunID, st.runningWantID, st.lastCommitWantID)
	}
}

// Acceptance 6. A stale success of another plan is dropped and restarts.
func TestImageStaleSuccessOfAnotherPlanIsDroppedAndRestarts(t *testing.T) {
	stubShowWheel(t)
	m := modelWithSlot(t)
	m.previewActive = true
	m, _ = m.requestImageNow()
	run := imageState(m).runningRunID
	m, _ = m.requestImage()
	m.wheelRunKey = "another plan"
	lines := len(m.logLines)
	m, cmd := m.onRefreshResult(refreshResultMsg{
		surface: surfaceImage, runID: run, wantID: 1,
		payload: imagePayload{bytes: []byte("<svg/>"), commands: []string{"/bin/astrolog dropped"}},
	})
	if cmd == nil || m.wheelPublishing || len(m.logLines) != lines {
		t.Errorf("other plan: follow-up nil=%v publishing=%v lines %d->%d", cmd == nil, m.wheelPublishing, lines, len(m.logLines))
	}
	if st := imageState(m); st.runningRunID != run+1 || st.lastCommitWantID != 0 {
		t.Errorf("other plan: run %d lastCommit %d", st.runningRunID, st.lastCommitWantID)
	}
}

// Acceptance 7. A polite tick does not cancel an older render in flight and
// re-arms; the live serial supersede is not the hard one.
func TestImageTickDoesNotCancelAnOlderRender(t *testing.T) {
	stubShowWheel(t)
	m := modelWithSlot(t)
	m.previewActive = true
	m, _ = m.requestImageNow()
	m, _ = m.requestImage()
	st := imageState(m)
	m, cmd := m.onRefreshTick(refreshTickMsg{surface: surfaceImage, timerID: st.timerID})
	if cmd == nil || imageState(m).cancel == nil || imageState(m).runningRunID != st.runningRunID {
		t.Error("the tick cancelled or restarted the older render")
	}
}

// Acceptance 4. Quit still ends the preview: the quit path reaches
// preview.Cleanup, observed through the package seam.
func TestQuitCallsPreviewCleanup(t *testing.T) {
	calls := 0
	prev := cleanupPreview
	t.Cleanup(func() { cleanupPreview = prev })
	cleanupPreview = func() error { calls++; return nil }
	m := newTestModel(t, Config{}, FrozenClock(testFrozenTime())) // no slots: quit writes no session
	if _, cmd := m.quit(); cmd == nil {
		t.Fatal("quit returned no command")
	}
	if calls != 1 {
		t.Errorf("quit called preview.Cleanup %d times, want 1", calls)
	}
}
