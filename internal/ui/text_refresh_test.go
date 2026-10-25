package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// textModel returns a ready single-slot workspace for text-surface tests.
func textModel(t *testing.T) Model {
	t.Helper()
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTimeUTC()))
	slots, err := store.NewSlots(testFrozenTimeUTC(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	m.slots = slots
	m.place = testPlaceBerlin()
	m.targetFloatingTime = slots.Floating().Time
	return m
}

// TestTextTick_PoliteWhileOlderRunInFlight pins the polite-tick
// recommendation: when the debounce tick fires while a run for an OLDER
// want is still in flight, the text surface must NOT cancel it (that run
// is the burst's leading frame — killing it when compute latency ≈ the
// debounce would erase the "first frame instant" half of the contract).
// The tick re-arms instead, checking back after another interval.
func TestTextTick_PoliteWhileOlderRunInFlight(t *testing.T) {
	m := textModel(t)
	m, _ = m.advanceTime(unitDay, +1) // leading run 1 (want 1)
	m, _ = m.advanceTime(unitDay, +1) // want 2, run 1 still in flight

	m, cmd := m.onRefreshTick(refreshTickMsg{surface: surfaceText, timerID: m.refresh[surfaceText].timerID})
	st := m.refresh[surfaceText]
	if st.runningRunID != 1 {
		t.Errorf("polite tick must not cancel/replace the leading run (runningRunID=%d, want 1)", st.runningRunID)
	}
	if st.cancel == nil {
		t.Error("the leading run must remain in flight after a polite tick")
	}
	if cmd == nil {
		t.Error("the polite tick must re-arm itself (non-nil Cmd) so the trailing run still happens")
	}
}

// TestTextTick_StaleTimerIgnored: a tick superseded by newer input does
// nothing at all — not even a polite re-arm (the newer input armed its own).
func TestTextTick_StaleTimerIgnored(t *testing.T) {
	m := textModel(t)
	m, _ = m.advanceTime(unitDay, +1) // want 1, timer 1
	m, _ = m.advanceTime(unitDay, +1) // want 2, timer 2

	m, cmd := m.onRefreshTick(refreshTickMsg{surface: surfaceText, timerID: 1})
	if cmd != nil {
		t.Error("a stale-timer tick must be a complete no-op")
	}
	if m.refresh[surfaceText].runningRunID != 1 {
		t.Error("a stale-timer tick must not disturb the in-flight run")
	}
}

// TestTextResult_CanceledDroppedSilently: a canceled run's result (it still
// delivers a message) must change nothing — no panel wipe, no error log, no
// follow-up. Cancellation is expected coordination noise, not a failure.
func TestTextResult_CanceledDroppedSilently(t *testing.T) {
	m := textModel(t)
	m.chart.positions = []aspect.Position{{Body: aspect.Sun, Longitude: 100}}
	m, _ = m.advanceTime(unitDay, +1)
	st := m.refresh[surfaceText]

	var tm tea.Model = m
	tm, cmd := tm.Update(FirstFrameReadyMsg{
		RunID: st.runningRunID, WantID: st.wantID,
		Err: context.Canceled,
	})
	m = tm.(Model)
	if cmd != nil {
		t.Error("a canceled result must not produce a follow-up")
	}
	if len(m.chart.positions) != 1 || m.chart.positions[0].Longitude != 100 {
		t.Errorf("a canceled result must not touch the committed panels: %v", m.chart.positions)
	}
	for _, line := range m.logLines {
		if strings.Contains(line.text, "chart compute error") {
			t.Errorf("a canceled result must not be logged as a failure: %q", line)
		}
	}
	if m.refresh[surfaceText].cancel != nil {
		t.Error("a canceled result must clear the in-flight handle")
	}
}

// TestTextResult_ErrorPolicy: a CURRENT-want error commits and logs (the
// operator should see a real failure, and the cold-start modal must not
// stick); a STALE-want error (mid-burst, a newer want already pending) is
// DROPPED WHOLE — neither logged nor committed, so a transient mid-burst
// failure can never blank a good chart (the review) — and the
// pending trailing tick still recovers the settled frame.
func TestTextResult_ErrorPolicy(t *testing.T) {
	// Current-want error: logged + committed (ready transition holds).
	m := textModel(t)
	m, _ = m.advanceTime(unitDay, +1)
	st := m.refresh[surfaceText]
	var tm tea.Model = m
	tm, _ = tm.Update(FirstFrameReadyMsg{RunID: st.runningRunID, WantID: st.wantID, Err: errors.New("boom")})
	m = tm.(Model)
	found := false
	for _, line := range m.logLines {
		if strings.HasPrefix(line.text, "Couldn't cast the chart.") {
			found = true
		}
	}
	if !found {
		t.Error("a current-want compute error must be logged")
	}
	if m.startup != startupReady {
		t.Error("an error commit still moves to ready (empty panels beat a stuck modal)")
	}

	// Stale-want error against a POPULATED chart: visible state unchanged.
	m2 := textModel(t)
	good := []aspect.Position{{Body: aspect.Sun, Longitude: 123}}
	m2.chart.positions = good
	committedTime := m2.slots.Floating().Time
	m2, _ = m2.advanceTime(unitDay, +1) // run 1, want 1
	m2, _ = m2.advanceTime(unitDay, +1) // want 2, run 1 in flight
	tm = m2
	tm, cmd := tm.Update(FirstFrameReadyMsg{RunID: 1, WantID: 1, Err: errors.New("mid-burst boom")})
	m2 = tm.(Model)
	if cmd != nil {
		t.Error("a stale-want error must not produce a follow-up directly (the pending tick recovers)")
	}
	if len(m2.chart.positions) != 1 || m2.chart.positions[0].Longitude != 123 {
		t.Errorf("a stale-want error blanked the good chart: %v", m2.chart.positions)
	}
	if !m2.slots.Floating().Time.Equal(committedTime) {
		t.Error("a stale-want error must not move the committed slot time")
	}
	for _, line := range m2.logLines {
		if strings.Contains(line.text, "mid-burst boom") {
			t.Errorf("a stale-want error must not be logged: %q", line)
		}
	}
	// Recovery: the pending tick (timer 2) fires → trailing run 2 starts.
	m2, cmd = m2.onRefreshTick(refreshTickMsg{surface: surfaceText, timerID: m2.refresh[surfaceText].timerID})
	if m2.refresh[surfaceText].runningRunID != 2 || cmd == nil {
		t.Errorf("the pending tick must start the trailing run after a dropped stale error (runningRunID=%d)",
			m2.refresh[surfaceText].runningRunID)
	}
}

// TestTextCommit_TriggersWheelRefreshWhenPreviewActive pins the cross-
// surface wiring: a text commit on the focused floating slot registers an
// image-surface want when the preview is open, and starts immediately while
// idle so the open viewer follows held-key chart changes. With the preview
// closed, nothing happens.
func TestTextCommit_TriggersWheelRefreshWhenPreviewActive(t *testing.T) {
	m := textModel(t) // single slot → cursor 0 is the floating slot
	m.previewActive = true
	m.targetFloatingTime = m.slots.Floating().Time.AddDate(0, 0, 1)
	m, _ = m.noteWant(surfaceText, true)
	st := m.refresh[surfaceText]

	var tm tea.Model = m
	tm, cmd := tm.Update(FirstFrameReadyMsg{RunID: st.runningRunID, WantID: st.wantID, TransitTime: m.targetFloatingTime})
	m = tm.(Model)
	if m.refresh[surfaceImage].wantID != 1 {
		t.Errorf("text commit with preview active must register an image want (wantID=%d, want 1)", m.refresh[surfaceImage].wantID)
	}
	if cmd == nil {
		t.Error("the image-surface batch must include work (non-nil Cmd)")
	}
	if m.refresh[surfaceImage].cancel == nil {
		t.Error("image auto-refresh must start immediately while idle")
	}
}

// TestColdStartThroughCoordinator drives the REAL cold-start path the
// invariants golden uses (perimeter + atlas → bumpIdentity → text run 1)
// and asserts the coordinator tokens line up with the documented contract:
// after a single cold-start dispatch, RunID = WantID = commitFloor = 1.
func TestColdStartThroughCoordinator(t *testing.T) {
	m := textModel(t)
	m.startup = startupComputing
	m, _ = m.bumpIdentity(surfaceText)
	st := m.refresh[surfaceText]
	if st.runningRunID != 1 || st.wantID != 1 || st.commitFloor != 1 {
		t.Fatalf("cold start tokens = run %d / want %d / floor %d, want 1/1/1 (the invariants golden hand-builds this)",
			st.runningRunID, st.wantID, st.commitFloor)
	}

	var tm tea.Model = m
	tm, _ = tm.Update(FirstFrameReadyMsg{RunID: 1, WantID: 1, TransitTime: m.targetFloatingTime})
	m = tm.(Model)
	if m.startup != startupReady {
		t.Error("the cold-start commit must move the UI to ready")
	}
	// The cold-start tick fires after the commit: want == lastCommit → no-op.
	m, cmd := m.onRefreshTick(refreshTickMsg{surface: surfaceText, timerID: m.refresh[surfaceText].timerID})
	if cmd != nil || m.refresh[surfaceText].cancel != nil {
		t.Error("the cold-start tick after a clean commit must be a no-op")
	}
}
