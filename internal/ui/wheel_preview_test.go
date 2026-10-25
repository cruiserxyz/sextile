package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/preview"
	"github.com/cruiserxyz/sextile/internal/store"
)

func modelWithSlot(t *testing.T) Model {
	t.Helper()
	m := NewModelWithClock(Config{}, FrozenClock(testFrozenTime()))
	slots, err := store.NewSlots(testFrozenTime(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	m.slots = slots
	m.cursorIdx = 0
	return m
}

// stubShowWheel swaps the wheel publisher for a stub that records the bytes it
// was handed, restoring the real one on cleanup. Lets the commit path run
// without starting the HTTP server or launching a browser.
func stubShowWheel(t *testing.T) *int {
	t.Helper()
	calls := 0
	prev := showWheel
	showWheel = func([]byte) (string, error) {
		calls++
		return "/tmp/stub.png", nil
	}
	t.Cleanup(func() { showWheel = prev })
	return &calls
}

func stubRenderWheelImage(t *testing.T) *[]time.Time {
	t.Helper()
	calls := []time.Time{}
	prev := renderWheelImage
	renderWheelImage = func(_ context.Context, when time.Time, _ location.PlaceInfo, _ lookup.Lens, _ lookup.WheelHighlight) ([]byte, error) {
		calls = append(calls, when)
		return []byte("BM-wheel"), nil
	}
	t.Cleanup(func() { renderWheelImage = prev })
	return &calls
}

func stubRenderOverlayWheelImage(t *testing.T) *[]time.Time {
	t.Helper()
	calls := []time.Time{}
	prev := renderOverlayWheelImage
	renderOverlayWheelImage = func(_ context.Context, _ store.AnchorChart, transitWhen time.Time, _ location.PlaceInfo, _ lookup.Lens) ([]byte, error) {
		calls = append(calls, transitWhen)
		return []byte("BM-overlay"), nil
	}
	t.Cleanup(func() { renderOverlayWheelImage = prev })
	return &calls
}

func imageState(m Model) refreshState { return m.refresh[surfaceImage] }

// TestDispatchRenderWheel: Ctrl+X with a focused slot activates the preview and
// starts an image run immediately; before any slot exists it is a no-op.
func TestDispatchRenderWheel(t *testing.T) {
	m := NewModelWithClock(Config{}, FrozenClock(testFrozenTime()))
	got, cmd := m.dispatchRenderWheel()
	if got.previewActive || cmd != nil {
		t.Error("dispatchRenderWheel before slots must be a no-op")
	}

	got, cmd = modelWithSlot(t).dispatchRenderWheel()
	if !got.previewActive {
		t.Error("dispatchRenderWheel must set previewActive")
	}
	if imageState(got).cancel == nil {
		t.Error("dispatchRenderWheel must start an image run (cancel set)")
	}
	if imageState(got).runningRunID != 1 {
		t.Errorf("runningRunID = %d, want 1 (one run started)", imageState(got).runningRunID)
	}
	if cmd == nil {
		t.Error("dispatchRenderWheel must return a render Cmd")
	}
}

// TestCtrlX_Immediate_ThenTrailingDoesNotDoubleStart: Ctrl+X starts a run now;
// a subsequent auto-refresh while it runs bumps the want and arms a debounce
// but must NOT start a second concurrent run.
func TestCtrlX_Immediate_ThenTrailingDoesNotDoubleStart(t *testing.T) {
	m := modelWithSlot(t)
	m.previewActive = true

	m, cmd := m.requestImageNow()
	if imageState(m).cancel == nil || cmd == nil {
		t.Fatal("requestImageNow must start a run immediately")
	}
	if imageState(m).runningRunID != 1 || imageState(m).wantID != 1 {
		t.Fatalf("after Ctrl+X: runningRunID=%d wantID=%d, want 1/1", imageState(m).runningRunID, imageState(m).wantID)
	}

	m, cmd = m.requestImage() // auto-refresh while the run is in flight
	if imageState(m).runningRunID != 1 {
		t.Errorf("a trailing request while running must NOT start a second run (runningRunID=%d, want 1)", imageState(m).runningRunID)
	}
	if imageState(m).wantID != 2 {
		t.Errorf("a trailing request must bump wantID to 2, got %d", imageState(m).wantID)
	}
	if cmd == nil {
		t.Error("an auto-refresh request must arm a debounce tick (non-nil Cmd)")
	}
}

// TestImageResult_LatestCommitsAndPublishes: a result for the in-flight run,
// wantID still current, commits + starts an OFF-THREAD publish (it must not call
// showWheel synchronously in Update). Driving the publish Cmd then publishes.
func TestImageResult_LatestCommitsAndPublishes(t *testing.T) {
	calls := stubShowWheel(t)
	m := modelWithSlot(t)
	m.previewActive = true
	m, _ = m.requestImageNow()
	st := imageState(m)

	m, cmd := m.onRefreshResult(refreshResultMsg{
		surface: surfaceImage, runID: st.runningRunID, wantID: st.wantID,
		payload: imagePayload{bytes: []byte("<svg/>")},
	})
	if imageState(m).cancel != nil {
		t.Error("after commit the render surface must be idle (cancel nil)")
	}
	if imageState(m).lastCommitWantID != st.wantID {
		t.Errorf("lastCommitWantID = %d, want %d", imageState(m).lastCommitWantID, st.wantID)
	}
	if !m.wheelPublishing || cmd == nil {
		t.Fatal("commit must start an off-thread publish")
	}
	if *calls != 0 {
		t.Errorf("commit must not publish synchronously in Update, got %d", *calls)
	}
	msg, ok := cmd().(wheelPublishedMsg)
	if !ok {
		t.Fatal("publish Cmd must yield a wheelPublishedMsg")
	}
	if *calls != 1 {
		t.Errorf("driving the publish Cmd must call showWheel once, got %d", *calls)
	}
	m, _ = m.onWheelPublished(msg)
	if m.wheelPublishing {
		t.Error("publish completion must clear wheelPublishing")
	}
}

// TestCtrlX_RepeatCoalesces: repeated Ctrl+X while a render is in flight must NOT
// cancel/restart it (the reviewer an earlier change — key-repeat would otherwise starve renders).
func TestCtrlX_RepeatCoalesces(t *testing.T) {
	m := modelWithSlot(t)
	m.previewActive = true
	m, _ = m.requestImageNow()
	r1 := imageState(m).runningRunID
	if imageState(m).cancel == nil {
		t.Fatal("first Ctrl+X must start a render")
	}
	m, cmd := m.requestImageNow()
	if cmd != nil {
		t.Error("repeat Ctrl+X while rendering must be a no-op")
	}
	if imageState(m).runningRunID != r1 {
		t.Errorf("repeat Ctrl+X must not restart the render (runID %d -> %d)", r1, imageState(m).runningRunID)
	}
}

// TestWheelPublish_SerializesLatestWins: a publish arriving while one is in
// flight queues (no concurrent preview.Show publication) and
// runs after — the newest bytes always win.
func TestWheelPublish_SerializesLatestWins(t *testing.T) {
	calls := stubShowWheel(t)
	m := modelWithSlot(t)
	m.previewActive = true

	// Pending bytes carry the plan they were rendered for (S4b B4).
	m.wheelPublishKey, _ = m.wheelStillKey()
	m.wheelPublishPending = []byte("first")
	m, cmd := m.kickWheelPublish()
	if !m.wheelPublishing || cmd == nil {
		t.Fatal("first publish must start")
	}
	// Second commit while publishing: queues, no concurrent publish.
	m.wheelPublishPending = []byte("second")
	m, c2 := m.kickWheelPublish()
	if c2 != nil {
		t.Error("a publish in flight must not start a concurrent one")
	}
	// First publish finishes -> chains the queued (latest) bytes.
	m, cmd3 := m.onWheelPublished(cmd().(wheelPublishedMsg))
	if cmd3 == nil || !m.wheelPublishing {
		t.Error("the queued publish must run after the first completes")
	}
	// Second publish finishes -> idle, nothing pending.
	m, _ = m.onWheelPublished(cmd3().(wheelPublishedMsg))
	if m.wheelPublishing || m.wheelPublishPending != nil {
		t.Error("after draining, publishing must be idle")
	}
	if *calls != 2 {
		t.Errorf("both publishes should have run, got %d", *calls)
	}
}

// TestImageResult_StaleRunDropped: a result whose runID is not the running run
// (a newer run already superseded it) is ignored entirely — no publish.
func TestImageResult_StaleRunDropped(t *testing.T) {
	calls := stubShowWheel(t)
	m := modelWithSlot(t)
	m.previewActive = true
	m, _ = m.requestImageNow() // runningRunID = 1

	m, cmd := m.onRefreshResult(refreshResultMsg{
		surface: surfaceImage, runID: 99, wantID: imageState(m).wantID,
		payload: imagePayload{bytes: []byte("<svg/>")},
	})
	if *calls != 0 {
		t.Error("a stale-runID result must not publish")
	}
	if cmd != nil || imageState(m).cancel == nil {
		t.Error("a stale-runID result must not touch the live run")
	}
}

// TestImageResult_CanceledDroppedSilently: a canceled run still delivers a
// result; with matching runID it clears the in-flight handle but neither
// publishes nor starts a follow-up.
func TestImageResult_CanceledDroppedSilently(t *testing.T) {
	calls := stubShowWheel(t)
	m := modelWithSlot(t)
	m.previewActive = true
	m, _ = m.requestImageNow()
	st := imageState(m)

	m, cmd := m.onRefreshResult(refreshResultMsg{
		surface: surfaceImage, runID: st.runningRunID, wantID: st.wantID,
		err: context.Canceled,
	})
	if *calls != 0 {
		t.Error("a canceled result must not publish")
	}
	if cmd != nil {
		t.Error("a canceled result must not start a follow-up")
	}
	if imageState(m).cancel != nil {
		t.Error("a canceled result must clear the in-flight handle")
	}
}

// TestImageResult_StaleWantStartsFollowup: the in-flight run completes but a
// newer want arrived meanwhile — the stale frame is not committed; a follow-up
// run starts for the latest want.
func TestImageResult_StaleWantStartsFollowup(t *testing.T) {
	calls := stubShowWheel(t)
	m := modelWithSlot(t)
	m.previewActive = true
	m, _ = m.requestImageNow() // runID 1, want 1
	runID := imageState(m).runningRunID
	m, _ = m.requestImage() // want -> 2, still running run 1

	m, cmd := m.onRefreshResult(refreshResultMsg{
		surface: surfaceImage, runID: runID, wantID: 1,
		payload: imagePayload{bytes: []byte("<svg/>")},
	})
	if !m.wheelPublishing {
		t.Error("a stale-want image success must publish as a live intermediate frame")
	}
	if cmd == nil {
		t.Error("a stale-want image success must publish and start a follow-up run")
	}
	if imageState(m).runningRunID != 2 || imageState(m).runningWantID != 2 {
		t.Errorf("follow-up must run the latest want: runningRunID=%d runningWantID=%d, want 2/2",
			imageState(m).runningRunID, imageState(m).runningWantID)
	}
	if *calls != 0 {
		t.Error("publish command should be returned, not run synchronously")
	}
}

// TestRefreshTick_StaleTimerIgnored: a debounce tick whose timerID was
// superseded by a newer input must do nothing — the reviewer-flagged riskiest path
// (Ctrl+X must not be cancelled/restarted by a leftover tick).
func TestRefreshTick_StaleTimerIgnored(t *testing.T) {
	m := modelWithSlot(t)
	m.previewActive = true
	m, _ = m.requestImage() // starts run 1 and arms timerID = 1
	m, _ = m.requestImageNow()
	freshRun := imageState(m).runningRunID // Ctrl+X started a run under timerID = 2

	m, cmd := m.onRefreshTick(refreshTickMsg{surface: surfaceImage, timerID: 1})
	if cmd != nil {
		t.Error("a stale-timer tick must not start/cancel anything")
	}
	if imageState(m).runningRunID != freshRun {
		t.Errorf("a stale-timer tick must not disturb the fresh run (runningRunID %d -> %d)", freshRun, imageState(m).runningRunID)
	}
}

func TestImageRefreshTick_DoesNotCancelOlderInFlightRender(t *testing.T) {
	m := modelWithSlot(t)
	m.previewActive = true
	m, _ = m.requestImageNow() // run 1, want 1
	m, _ = m.requestImage()    // want 2 while run 1 is still in flight

	m, cmd := m.onRefreshTick(refreshTickMsg{surface: surfaceImage, timerID: imageState(m).timerID})
	if cmd == nil {
		t.Fatal("tick should re-arm while waiting for the active render to finish")
	}
	if imageState(m).runningRunID != 1 || imageState(m).runningWantID != 1 {
		t.Fatalf("tick cancelled/restarted the render: runningRunID=%d runningWantID=%d, want run 1/want 1",
			imageState(m).runningRunID, imageState(m).runningWantID)
	}
	if imageState(m).cancel == nil {
		t.Fatal("older render should remain in flight")
	}
}

// TestRequestImage_StartsLeadingWhenIdle: auto-refresh starts the image render
// immediately when no render is in flight, so held time-step keys visibly
// advance the open wheel preview.
func TestRequestImage_StartsLeadingWhenIdle(t *testing.T) {
	m := modelWithSlot(t)
	m.previewActive = true
	m, cmd := m.requestImage()
	if imageState(m).cancel == nil || cmd == nil {
		t.Fatal("image request must start immediately while idle")
	}
	if imageState(m).runningRunID != 1 {
		t.Errorf("runningRunID = %d, want 1", imageState(m).runningRunID)
	}
}

func TestAdvanceTime_RequestsImageImmediatelyWhenPreviewActive(t *testing.T) {
	m := modelWithSlot(t)
	m.previewActive = true
	m.targetFloatingTime = m.slots.Floating().Time

	m, cmd := m.advanceTime(unitDay, +1)
	if cmd == nil {
		t.Fatal("advanceTime with preview active must return batched text+image work")
	}
	if imageState(m).wantID != 1 {
		t.Fatalf("image wantID = %d, want 1 from the keypress itself", imageState(m).wantID)
	}
	if imageState(m).cancel == nil {
		t.Fatal("image render must start immediately while idle")
	}
}

func TestImageRender_UsesTargetFloatingTime(t *testing.T) {
	calls := stubRenderWheelImage(t)
	m := modelWithSlot(t)
	m.previewActive = true
	committed := m.slots.Floating().Time
	target := committed.Add(48 * time.Hour)
	m.targetFloatingTime = target

	cmd := m.buildSurfaceCmd(surfaceImage, context.Background(), 1, 1)
	if cmd == nil {
		t.Fatal("buildSurfaceCmd returned nil")
	}
	_ = cmd()
	if len(*calls) != 1 {
		t.Fatalf("render calls = %d, want 1", len(*calls))
	}
	if !(*calls)[0].Equal(target) {
		t.Errorf("image render time = %v, want target %v (committed slot was %v)", (*calls)[0], target, committed)
	}
}

func TestImageRender_OverlayUsesTargetFloatingTime(t *testing.T) {
	calls := stubRenderOverlayWheelImage(t)
	m := modelWithSlot(t)
	m.previewActive = true
	if err := addAnchor(m.slots, testFrozenTime().AddDate(-30, 0, 0), testPlaceBerlin()); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}
	committed := m.slots.Floating().Time
	target := committed.Add(72 * time.Hour)
	m.targetFloatingTime = target
	m.cursorIdx = 1           // the floating slot
	m.base = m.slots.All()[0] // now read in ①: the wheel draws that tab's bi-wheel

	cmd := m.buildSurfaceCmd(surfaceImage, context.Background(), 1, 1)
	if cmd == nil {
		t.Fatal("buildSurfaceCmd returned nil")
	}
	_ = cmd()
	if len(*calls) != 1 {
		t.Fatalf("overlay render calls = %d, want 1", len(*calls))
	}
	if !(*calls)[0].Equal(target) {
		t.Errorf("overlay transit render time = %v, want target %v (committed slot was %v)", (*calls)[0], target, committed)
	}
}

func TestImageRender_WithAnchorButUnselectedPairUsesSingleWheel(t *testing.T) {
	singleCalls := stubRenderWheelImage(t)
	overlayCalls := stubRenderOverlayWheelImage(t)
	m := modelWithSlot(t)
	m.previewActive = true
	anchorWhen := testFrozenTime().AddDate(-30, 0, 0)
	if err := addAnchor(m.slots, anchorWhen, testPlaceBerlin()); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}
	m.cursorIdx = 1
	m.slots.ToggleSlotSelection(1)

	cmd := m.buildSurfaceCmd(surfaceImage, context.Background(), 1, 1)
	if cmd == nil {
		t.Fatal("buildSurfaceCmd returned nil")
	}
	_ = cmd()
	if len(*singleCalls) != 1 || len(*overlayCalls) != 0 {
		t.Errorf("render calls after pair deselection: single=%d overlay=%d, want 1/0", len(*singleCalls), len(*overlayCalls))
	}
}

// C7 (body selector) and C9 (aspect chooser) both exclude the WHEEL from the
// live preview: it is an external browser preview, and a burst
// of toggles would spend one render each on it.
//
// The exclusion is easy to state and was not implemented. A toggle does not
// touch the wheel directly — it schedules a TEXT recompute, and the ordinary
// text-commit path auto-refreshes the wheel — so with the viewer open
// (Ctrl+X), one Space inside the selector spent a render. Asserted here at the
// shared choke point, in both chooser states, with the closed state as the
// control that this is a suppression and not a dead function.
func TestWheelIsExcludedFromModalPreview(t *testing.T) {
	for _, open := range []startupState{startupAspectOpen, startupBodyOpen} {
		t.Run(string(open), func(t *testing.T) {
			m := modelWithSlot(t)
			m.previewActive = true
			m.startup = open
			got, cmd := m.refreshWheelIfActive()
			if cmd != nil || imageState(got).wantID != 0 {
				t.Errorf("a preview commit spent a wheel render while %s; C7/C9 exclude the wheel by name", open)
			}
		})
	}

	// ...and the wheel is not simply switched off: once the chooser closes,
	// the next commit catches it up.
	m := modelWithSlot(t)
	m.previewActive = true
	m.startup = startupReady
	got, cmd := m.refreshWheelIfActive()
	if cmd == nil || imageState(got).wantID == 0 {
		t.Error("with no chooser open the wheel must still refresh; the suppression would otherwise be permanent")
	}
}

// TestRefreshWheelIfActive: no-op unless the preview is active; when active it
// registers a desired refresh and starts immediately while idle.
func TestRefreshWheelIfActive(t *testing.T) {
	m := modelWithSlot(t)
	if got, cmd := m.refreshWheelIfActive(); cmd != nil || imageState(got).wantID != 0 {
		t.Error("refreshWheelIfActive must be a no-op when the preview is inactive")
	}
	m.previewActive = true
	got, cmd := m.refreshWheelIfActive()
	if cmd == nil || imageState(got).wantID != 1 {
		t.Error("refreshWheelIfActive must register a refresh when the preview is active")
	}
	if imageState(got).cancel == nil {
		t.Error("refreshWheelIfActive must start a render immediately while idle")
	}
}

// TestImageCommit_ErrorClearsPreview: a render error (not a cancellation) on the
// latest run clears previewActive so a broken render/viewer stops auto-refreshing.
func TestImageCommit_ErrorClearsPreview(t *testing.T) {
	m := modelWithSlot(t)
	m.previewActive = true
	m, _ = m.requestImageNow()
	st := imageState(m)

	got, _ := m.onRefreshResult(refreshResultMsg{
		surface: surfaceImage, runID: st.runningRunID, wantID: st.wantID,
		err: errors.New("render failed"),
	})
	if got.previewActive {
		t.Error("a wheel render error must clear previewActive")
	}
	if imageState(got).cancel != nil {
		t.Error("a render error must clear the in-flight handle")
	}
}

// TestPublishError_DropsInFlightRender: a publish failure clears previewActive;
// a render that was ALREADY in flight must not revive the preview by committing
// + queuing another publish when it completes (the reviewer an earlier change high). The render-
// start gate alone can't catch this — the render was started while active.
func TestPublishError_DropsInFlightRender(t *testing.T) {
	_ = stubShowWheel(t)
	m := modelWithSlot(t)
	m.previewActive = true

	// R1 commits -> publish P1 in flight.
	m, _ = m.requestImageNow()
	st1 := imageState(m)
	m, pub := m.onRefreshResult(refreshResultMsg{surface: surfaceImage, runID: st1.runningRunID, wantID: st1.wantID, payload: imagePayload{bytes: []byte("a")}})
	if !m.wheelPublishing || pub == nil {
		t.Fatal("commit should start a publish")
	}
	// R2 starts (auto-refresh) while P1 is publishing; previewActive still true.
	m, _ = m.requestImage()
	m, _ = m.onRefreshTick(refreshTickMsg{surface: surfaceImage, timerID: imageState(m).timerID})
	r2 := imageState(m)
	if r2.cancel == nil {
		t.Fatal("R2 should be in flight")
	}
	// P1 FAILS -> previewActive cleared.
	m, _ = m.onWheelPublished(wheelPublishedMsg{err: errors.New("publish boom")})
	if m.previewActive {
		t.Fatal("publish failure must clear previewActive")
	}
	// R2 now succeeds: must be DROPPED, not published.
	m, cmd := m.onRefreshResult(refreshResultMsg{surface: surfaceImage, runID: r2.runningRunID, wantID: r2.wantID, payload: imagePayload{bytes: []byte("b")}})
	if cmd != nil || m.wheelPublishing {
		t.Error("a render completing after publish failure must not revive the preview")
	}
}

func TestBrowserLaunchFailureStillLogsThePreviewURL(t *testing.T) {
	m := modelWithSlot(t)
	m.previewActive = true
	m.wheelPublishing = true

	m, _ = m.onWheelPublished(wheelPublishedMsg{
		path: "http://localhost:5307/",
		err:  errors.New("cmd.exe failed"),
	})
	if m.previewActive || m.wheelPublishing {
		t.Fatal("publish failure did not disable the active publication")
	}
	wantURL, wantError := false, false
	for _, line := range m.logLines {
		if line.lane == laneNote && line.text == "wheel: open it by hand at http://localhost:5307/" {
			wantURL = true
		}
		if line.lane == laneError && line.text == "Couldn't show the wheel. The image could not be opened." {
			wantError = true
		}
	}
	if !wantURL || !wantError {
		t.Fatalf("browser failure log = %v; want the usable URL and the failure", m.logLines)
	}
}

// TestImageSurface_GatedOnPreviewActive: a tick that WOULD otherwise start a
// render must not render while previewActive is false. The image surface gates
// the render-start on previewActive, so a queued tick (or a stale follow-up)
// can never revive a preview disabled by a render error (the reviewer an earlier change).
func TestImageSurface_GatedOnPreviewActive(t *testing.T) {
	m := modelWithSlot(t)
	m.previewActive = true
	m, _ = m.requestImage() // want 1, tick armed, run 1 in flight
	m.previewActive = false // preview disabled (as a render error would)
	imageState(m).cancel()
	m.refresh[surfaceImage].cancel = nil

	// The live tick fires after the old render has ended: ensureRunningLatest
	// would start (want 1 > commit 0), but the gate must suppress the render.
	m, cmd := m.onRefreshTick(refreshTickMsg{surface: surfaceImage, timerID: imageState(m).timerID})
	if cmd != nil || imageState(m).cancel != nil {
		t.Error("image surface must not render while previewActive is false")
	}
}

// TestImageError_PendingTickDoesNotRevive: the end-to-end shape the reviewer flagged —
// a render error clears previewActive while a debounce tick is still armed; the
// tick firing afterwards must not restart rendering.
func TestImageError_PendingTickDoesNotRevive(t *testing.T) {
	calls := stubShowWheel(t)
	m := modelWithSlot(t)
	m.previewActive = true
	m, _ = m.requestImageNow() // R1 in flight for want 1
	r1 := imageState(m).runningRunID
	m, _ = m.requestImage() // want 2, arm tick T2; R1 still running

	// R1 completes stale (want 1 != 2) -> follow-up R2 for want 2; preview still on.
	m, _ = m.onRefreshResult(refreshResultMsg{surface: surfaceImage, runID: r1, wantID: 1, payload: imagePayload{bytes: []byte("x")}})
	r2 := imageState(m).runningRunID
	if r2 == r1 {
		t.Fatal("a stale-want success must start a follow-up run")
	}
	// R2 fails -> commit error -> previewActive cleared.
	m, _ = m.onRefreshResult(refreshResultMsg{surface: surfaceImage, runID: r2, wantID: 2, err: errors.New("boom")})
	if m.previewActive {
		t.Fatal("render error must clear previewActive")
	}
	// The still-armed tick T2 fires: it must not revive rendering.
	m, cmd := m.onRefreshTick(refreshTickMsg{surface: surfaceImage, timerID: imageState(m).timerID})
	if cmd != nil || imageState(m).cancel != nil {
		t.Error("a pending tick after a render error must not revive rendering")
	}
	if *calls != 0 {
		t.Errorf("no image should have been published in this flow, got %d", *calls)
	}
}

// TestClosedViewerStopsFollowingAndCtrlXReopens is the ui half of the
// operator's bug of 2026-10-05 ("when the wheel terminal instance is closed,
// it cannot be reopened"): Ctrl+X asks the preview to open a viewer, a follow
// refresh never asks, and a publish answered with ErrViewerClosed stops the
// follow quietly — no error line — so the next Ctrl+X opens it again.
func TestClosedViewerStopsFollowingAndCtrlXReopens(t *testing.T) {
	asks := 0
	prevAsk := requestViewerOpen
	requestViewerOpen = func() { asks++ }
	t.Cleanup(func() { requestViewerOpen = prevAsk })
	prevShow := showWheel
	showWheel = func([]byte) (string, error) { return "", preview.ErrViewerClosed }
	t.Cleanup(func() { showWheel = prevShow })

	m, _ := modelWithSlot(t).dispatchRenderWheel()
	if asks != 1 {
		t.Fatalf("Ctrl+X asked to open %d times, want 1", asks)
	}
	m, _ = m.requestImage() // a follow refresh
	if asks != 1 {
		t.Fatalf("a follow refresh asked to open (asks = %d): only Ctrl+X may", asks)
	}

	errorsBefore := len(m.logLines)
	m.wheelPublishing = true
	m.wheelPublishPending = []byte("queued")
	m, cmd := m.onWheelPublished(wheelPublishedMsg{err: preview.ErrViewerClosed, openSeq: m.wheelOpenSeq})
	if m.previewActive {
		t.Error("a closed viewer must stop the follow (previewActive still true)")
	}
	if m.wheelPublishPending != nil || cmd != nil {
		t.Error("a closed viewer must drop the queued publish and start nothing")
	}
	if n := len(m.logLines); n != errorsBefore {
		t.Errorf("closing the viewer wrote %d Log line(s); it is a choice, not a failure", n-errorsBefore)
	}

	m, _ = m.dispatchRenderWheel()
	if asks != 2 || !m.previewActive {
		t.Errorf("Ctrl+X after a close: asks = %d previewActive = %v, want 2 and true", asks, m.previewActive)
	}
}

// TestStaleViewerClosedCannotCancelANewerCtrlX (the reviewer, hotfix round 1): a
// "viewer closed" answer from a publish started before the newest Ctrl+X
// leaves that press's reopen alone.
func TestStaleViewerClosedCannotCancelANewerCtrlX(t *testing.T) {
	prevAsk := requestViewerOpen
	requestViewerOpen = func() {}
	t.Cleanup(func() { requestViewerOpen = prevAsk })

	m, _ := modelWithSlot(t).dispatchRenderWheel()
	staleSeq := m.wheelOpenSeq
	m.wheelPublishing = true
	m, _ = m.dispatchRenderWheel()           // the operator presses Ctrl+X again
	m.wheelPublishKey, _ = m.wheelStillKey() // pending bytes carry their plan (S4b B4)
	m.wheelPublishPending = []byte("reopen")

	m, cmd := m.onWheelPublished(wheelPublishedMsg{err: preview.ErrViewerClosed, openSeq: staleSeq})
	if !m.previewActive {
		t.Fatal("a stale closed answer cancelled the newer Ctrl+X (previewActive false)")
	}
	if cmd == nil || !m.wheelPublishing {
		t.Error("the newer Ctrl+X's queued publish must start")
	}
}
