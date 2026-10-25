package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// TestAdvanceTime_NoOpWhenNoSlots asserts that pressing a time-step key
// before slots are constructed (cold-start, pre-resolution) is a silent
// no-op. The handler must not panic on nil m.slots and must not dispatch
// a recompute that would have no input.
func TestAdvanceTime_NoOpWhenNoSlots(t *testing.T) {
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTime()))

	_, cmd := m.advanceTime(unitDay, +1)
	if cmd != nil {
		t.Errorf("advanceTime(nil slots) returned non-nil cmd; expected silent no-op")
	}
}

// TestAdvanceTime_UnitsAndDirections asserts that each timeStepUnit
// applied with direction +1 / -1 mutates the floating slot's Time by
// the documented amount. Anchored slot stays untouched.
func TestAdvanceTime_UnitsAndDirections(t *testing.T) {
	natalWhen := time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC)
	cases := []struct {
		name      string
		unit      timeStepUnit
		direction int
		want      time.Time
	}{
		// Start time = 2026-04-25 12:00 UTC = 14:00 CEST (Berlin DST on).
		// Calendar steps preserve the user's local clock time (14:00) per
		// the DST-aware AdvanceFloating contract; week/day fall within
		// CEST so the UTC offset is unchanged. "month -1" crosses into
		// March 25, which is BEFORE Berlin's 2026 DST start (Mar 29) —
		// i.e. CET (UTC+1) — so the 14:00 local clock corresponds to
		// 13:00 UTC there. This asymmetry is correct.
		{"year +1", unitYear, +1, time.Date(2027, 4, 25, 12, 0, 0, 0, time.UTC)},
		{"year -1", unitYear, -1, time.Date(2025, 4, 25, 12, 0, 0, 0, time.UTC)},
		{"month +1", unitMonth, +1, time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)},
		{"month -1 (crosses DST start)", unitMonth, -1, time.Date(2026, 3, 25, 13, 0, 0, 0, time.UTC)},
		{"week +1", unitWeek, +1, time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)},
		{"week -1", unitWeek, -1, time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)},
		{"day +1", unitDay, +1, time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)},
		{"day -1", unitDay, -1, time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)},
		{"hour +1", unitHour, +1, time.Date(2026, 4, 25, 13, 0, 0, 0, time.UTC)},
		{"hour -1", unitHour, -1, time.Date(2026, 4, 25, 11, 0, 0, 0, time.UTC)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTimeUTC()))
			slots, err := store.NewSlots(testFrozenTimeUTC(), testPlaceBerlin())
			if err != nil {
				t.Fatalf("NewSlots: %v", err)
			}
			if err := addAnchor(slots, natalWhen, testPlaceAmsterdam()); err != nil {
				t.Fatalf("addAnchor: %v", err)
			}
			m.slots = slots
			m.cursorIdx = 1 // floating slot; time-step keys are floating-only
			m.place = testPlaceBerlin()

			m.targetFloatingTime = m.slots.Floating().Time
			got, cmd := m.advanceTime(tc.unit, tc.direction)
			if cmd == nil {
				t.Fatal("advanceTime returned nil cmd; expected a recompute dispatch")
			}

			// a review finding advanceTime mutates m.targetFloatingTime,
			// NOT slot.Time. The slot's Time is the "what's currently
			// displayed" truth, advanced only by the text commit
			// when a matching compute lands. Assert the target.
			if !got.targetFloatingTime.Equal(tc.want) {
				t.Errorf("targetFloatingTime = %v, want %v", got.targetFloatingTime.UTC(), tc.want.UTC())
			}
			// slot.Time must NOT have moved yet (no commit happened).
			if !got.slots.Floating().Time.Equal(testFrozenTimeUTC()) {
				t.Errorf("slot.Time advanced prematurely: %v (want unchanged %v until the text commit lands)",
					got.slots.Floating().Time.UTC(), testFrozenTimeUTC())
			}
			// Anchor must NOT have moved regardless of unit/direction.
			if !firstAnchor(got.slots).Time.Equal(natalWhen) {
				t.Errorf("anchored.Time mutated to %v, want %v",
					firstAnchor(got.slots).Time.UTC(), natalWhen)
			}
		})
	}
}

// TestAdvanceTime_LeadingThenCoalesce pins the post-its specification
// contract ("first + latest, fast"): the burst's FIRST press starts a
// compute immediately (leading edge); further presses while it runs do
// NOT start more computes — they bump the desired want and re-arm the
// trailing debounce. The leading run's result COMMITS even though newer
// presses arrived (instant first frame), with NO chained follow-up —
// the pending debounce tick computes the settled final target.
func TestAdvanceTime_LeadingThenCoalesce(t *testing.T) {
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTimeUTC()))
	slots, err := store.NewSlots(testFrozenTimeUTC(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	m.slots = slots
	m.place = testPlaceBerlin()
	m.targetFloatingTime = slots.Floating().Time

	// Press 1: leading edge — run 1 starts NOW.
	m, cmd := m.advanceTime(unitDay, +1)
	if cmd == nil {
		t.Fatal("press 1: expected leading-edge dispatch, got nil cmd")
	}
	leadTime := m.targetFloatingTime
	st := m.refresh[surfaceText]
	if st.runningRunID != 1 || st.cancel == nil {
		t.Fatalf("press 1 must start text run 1 (runningRunID=%d, cancel set=%v)", st.runningRunID, st.cancel != nil)
	}

	// Presses 2-3 while run 1 is in flight: want grows, NO new run starts.
	m, _ = m.advanceTime(unitDay, +1)
	m, _ = m.advanceTime(unitDay, +1)
	st = m.refresh[surfaceText]
	if st.runningRunID != 1 {
		t.Errorf("mid-burst presses must not start runs (runningRunID=%d, want 1)", st.runningRunID)
	}
	if st.wantID != 3 {
		t.Errorf("wantID = %d, want 3 (one per press)", st.wantID)
	}
	finalTarget := m.targetFloatingTime
	if !finalTarget.Equal(testFrozenTimeUTC().AddDate(0, 0, 3)) {
		t.Fatalf("target after 3 presses = %v, want +3d", finalTarget.UTC())
	}

	// Run 1 completes mid-burst: the LEADING frame commits (slot.Time
	// advances — instant feedback) and NO follow-up is chained (the
	// pending tick owns the trailing run).
	var tm tea.Model = m
	tm, cmd = tm.Update(FirstFrameReadyMsg{RunID: 1, WantID: 1, TransitTime: leadTime})
	m = tm.(Model)
	if !m.slots.Floating().Time.Equal(leadTime) {
		t.Errorf("leading frame did not commit: slot.Time = %v, want %v", m.slots.Floating().Time.UTC(), leadTime.UTC())
	}
	if cmd != nil {
		t.Error("leading commit must not chain a follow-up (the pending tick handles the trailing run)")
	}

	// The live trailing tick fires: run 2 starts for the settled want.
	m, cmd = m.onRefreshTick(refreshTickMsg{surface: surfaceText, timerID: m.refresh[surfaceText].timerID})
	st = m.refresh[surfaceText]
	if st.runningRunID != 2 || cmd == nil {
		t.Fatalf("trailing tick must start run 2 for the settled want (runningRunID=%d)", st.runningRunID)
	}

	// Final result commits the settled frame.
	tm = m
	tm, _ = tm.Update(FirstFrameReadyMsg{RunID: 2, WantID: 3, TransitTime: finalTarget})
	m = tm.(Model)
	if !m.slots.Floating().Time.Equal(finalTarget) {
		t.Errorf("final frame did not commit: slot.Time = %v, want %v", m.slots.Floating().Time.UTC(), finalTarget.UTC())
	}
}

// TestStepDirectionFromRune asserts the direction-from-rune contract:
// lowercase letters map to forward (+1), uppercase to backward (-1).
// Defends against accidental case-sign-flip regressions.
func TestStepDirectionFromRune(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.KeyMsg
		want int
	}{
		{"y lowercase forward", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}}, +1},
		{"Y uppercase backward", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Y'}}, -1},
		{"d lowercase forward", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}, +1},
		{"D uppercase backward", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}}, -1},
		{"empty runes defaults forward", tea.KeyMsg{Type: tea.KeyRunes}, +1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stepDirectionFromRune(tc.msg); got != tc.want {
				t.Errorf("stepDirectionFromRune(%v) = %d, want %d", tc.msg, got, tc.want)
			}
		})
	}
}

// TestTransitTimeFromWorkspace_FollowsSteppedSlot is the routing-layer
// half of the review fix. The bug the reviewer caught: prior
// versions of advanceTime mutated slot.Time but recompute passed
// clock.Now() to lookup, so bodies/aspects rendered for the wall-clock
// moment instead of the stepped target. The fix introduced
// transitTimeFromWorkspace as the single seam that resolves "the time
// the chart compute targets" — this test pins its contract.
//
// internal/ui cannot import internal/astrolog (architectural isolation
// invariant), so end-to-end JD-level assertions live in
// internal/lookup. Here we assert the routing decision: the time
// passed to compute follows slots.Floating().Time, not m.clock.Now().
func TestTransitTimeFromWorkspace_FollowsSteppedSlot(t *testing.T) {
	clockNow := testFrozenTimeUTC()
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(clockNow))
	slots, err := store.NewSlots(clockNow, testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	m.slots = slots
	m.place = testPlaceBerlin()
	m.targetFloatingTime = slots.Floating().Time

	// At cold-start, transit time should equal clock.Now() because
	// targetFloatingTime was just initialized to it.
	if got := transitTimeFromWorkspace(m); !got.Equal(clockNow) {
		t.Errorf("pre-step: transitTime = %v, want %v", got.UTC(), clockNow)
	}

	// Step +1 month. Now the routing must resolve to clockNow + 1 month,
	// NOT clockNow (which would be the bug the reviewer flagged).
	m, _ = m.advanceTime(unitMonth, +1)
	wantStepped := clockNow.AddDate(0, 1, 0)
	got := transitTimeFromWorkspace(m)
	if !got.Equal(wantStepped) {
		t.Errorf("post-step: transitTime = %v, want %v (stepped)", got.UTC(), wantStepped.UTC())
	}
	if got.Equal(clockNow) {
		t.Error("post-step: transitTime still equal to clock.Now() — recompute would lie to user")
	}
}

// TestAdvanceTime_SlowSteppingCommitsEveryStep pins the deliberate-
// stepping feel: when each press's compute FINISHES before the next
// press arrives, every step paints instantly (a fresh leading edge per
// press) — no debounce lag, no skipped frames. The coordinator only
// coalesces when presses outpace compute.
func TestAdvanceTime_SlowSteppingCommitsEveryStep(t *testing.T) {
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTimeUTC()))
	slots, err := store.NewSlots(testFrozenTimeUTC(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	m.slots = slots
	m.place = testPlaceBerlin()
	m.targetFloatingTime = slots.Floating().Time

	positions := []aspect.Position{{Body: aspect.Sun, Longitude: 100}}
	for i := uint64(1); i <= 2; i++ {
		// Press: leading edge starts run i (the surface is idle again).
		var cmd tea.Cmd
		m, cmd = m.advanceTime(unitDay, +1)
		if cmd == nil {
			t.Fatalf("press %d: expected leading-edge dispatch", i)
		}
		st := m.refresh[surfaceText]
		if st.runningRunID != i {
			t.Fatalf("press %d: runningRunID = %d, want %d (leading start per settled press)", i, st.runningRunID, i)
		}
		target := m.targetFloatingTime

		// Its result lands before the next press: commits, goes idle.
		var tm tea.Model = m
		tm, _ = tm.Update(FirstFrameReadyMsg{Positions: positions, RunID: st.runningRunID, WantID: st.wantID, TransitTime: target})
		m = tm.(Model)
		if !m.slots.Floating().Time.Equal(target) {
			t.Errorf("press %d: slot.Time = %v, want %v (instant per-step commit)", i, m.slots.Floating().Time.UTC(), target.UTC())
		}
		if m.refresh[surfaceText].cancel != nil {
			t.Errorf("press %d: surface must be idle after its commit", i)
		}
	}
}

// TestTextResult_SupersededRunDropped: a result whose RunID is not the
// running run was canceled/replaced (identity bump, tick restart) — it
// must be ignored entirely, payload and error alike. Single-flight means
// the old out-of-order-Seq failure mode reduces to this one gate: there
// is never more than one live run to race.
func TestTextResult_SupersededRunDropped(t *testing.T) {
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTimeUTC()))
	slots, _ := store.NewSlots(testFrozenTimeUTC(), testPlaceBerlin())
	m.slots = slots
	m.place = testPlaceBerlin()

	// Run 1 starts (leading), then an identity bump supersedes it with run 2.
	m, _ = m.advanceTime(unitDay, +1)
	m, _ = m.bumpIdentity(surfaceText)
	if st := m.refresh[surfaceText]; st.runningRunID != 2 {
		t.Fatalf("precondition: bumpIdentity should have started run 2, got %d", st.runningRunID)
	}

	// Run 1's late result — superseded RunID — must not paint.
	var tm tea.Model = m
	tm, cmd := tm.Update(FirstFrameReadyMsg{
		Positions: []aspect.Position{{Body: aspect.Sun, Longitude: 999}},
		RunID:     1, WantID: 1,
	})
	m = tm.(Model)
	if len(m.chart.positions) != 0 {
		t.Errorf("superseded run's payload committed: %v", m.chart.positions)
	}
	if cmd != nil {
		t.Error("superseded run's result must not produce a follow-up")
	}
	if m.refresh[surfaceText].cancel == nil {
		t.Error("the live run (2) must remain in flight, undisturbed")
	}
}

// TestApplyAtlasPicked_RePick_PreservesSteppedFloatingTime asserts the
// a review finding: re-picking a city after the operator
// has time-stepped to a target moment must KEEP the floating slot at
// that stepped time (not reset to wall-clock now). The new city is
// computed AT the stepped target.
func TestApplyAtlasPicked_RePick_PreservesSteppedFloatingTime(t *testing.T) {
	clockNow := testFrozenTimeUTC()
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(clockNow))
	{
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
		m = updated.(Model)
	}
	slots, err := store.NewSlots(clockNow, testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	m.slots = slots
	m.place = testPlaceBerlin()
	m.startup = startupReady
	m.targetFloatingTime = clockNow

	// Step +6 months forward then commit so slot.Time advances. The press
	// is a leading edge (run 1, want 1) — the commit carries its tokens.
	m, _ = m.advanceTime(unitMonth, +6)
	steppedTarget := m.targetFloatingTime
	{
		st := m.refresh[surfaceText]
		var tm tea.Model = m
		tm, _ = tm.Update(FirstFrameReadyMsg{RunID: st.runningRunID, WantID: st.wantID, TransitTime: steppedTarget})
		m = tm.(Model)
	}
	if got := m.slots.Floating().Time; !got.Equal(steppedTarget) {
		t.Fatalf("precondition: commit should have advanced slot.Time to %v; got %v", steppedTarget.UTC(), got.UTC())
	}

	// Now re-pick a different city.
	tokyo := testPlaceTokyo()
	got, _ := applyAtlasPicked(m, AtlasPickedMsg{Place: tokyo})

	// The floating slot must STILL carry the stepped time, not reset.
	gotTime := got.slots.Floating().Time
	if !gotTime.Equal(steppedTarget) {
		t.Errorf("re-pick discarded stepped time: floating.Time = %v, want %v (stepped target preserved)",
			gotTime.UTC(), steppedTarget.UTC())
	}
}

// TestApplyAtlasPicked_RePick_PreservesUncommittedTarget is the
// a review finding: if the user steps to T+5 then
// re-picks a city BEFORE any FirstFrameReadyMsg commits (slot.Time
// still at T+0, targetFloatingTime at T+5), the new city must be
// computed for T+5, not silently snapped back to T+0. Without
// preserving m.targetFloatingTime across refreshSlotsForPlace,
// the held-key target gets discarded by the re-pick.
func TestApplyAtlasPicked_RePick_PreservesUncommittedTarget(t *testing.T) {
	clockNow := testFrozenTimeUTC()
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(clockNow))
	{
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
		m = updated.(Model)
	}
	slots, err := store.NewSlots(clockNow, testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	m.slots = slots
	m.place = testPlaceBerlin()
	m.startup = startupReady
	m.targetFloatingTime = clockNow

	// Step +5 days. NO commit fires — slot.Time stays at clockNow but
	// targetFloatingTime races to clockNow+5d.
	for i := 0; i < 5; i++ {
		m, _ = m.advanceTime(unitDay, +1)
	}
	steppedTarget := m.targetFloatingTime
	if !steppedTarget.Equal(clockNow.AddDate(0, 0, 5)) {
		t.Fatalf("precondition: target should be +5d; got %v", steppedTarget.UTC())
	}
	if !m.slots.Floating().Time.Equal(clockNow) {
		t.Fatalf("precondition: slot.Time should still be clockNow (no commits); got %v", m.slots.Floating().Time.UTC())
	}

	// Now re-pick a different city mid-burst, before any commit.
	tokyo := testPlaceTokyo()
	got, _ := applyAtlasPicked(m, AtlasPickedMsg{Place: tokyo})

	// The new workspace must carry the STEPPED target, not snap back
	// to the uncommitted slot.Time.
	if !got.targetFloatingTime.Equal(steppedTarget) {
		t.Errorf("re-pick discarded uncommitted target: got %v, want %v",
			got.targetFloatingTime.UTC(), steppedTarget.UTC())
	}
	if !got.slots.Floating().Time.Equal(steppedTarget) {
		t.Errorf("re-pick new slot.Time = %v, want %v (new city at stepped target, not unstepped clockNow)",
			got.slots.Floating().Time.UTC(), steppedTarget.UTC())
	}
}

// TestTextResult_IdentityFloorDropsQueuedResult is the reviewer an earlier change
// round-4 regression net, restated for the coordinator: a result from a
// SUPERSEDED WORKSPACE IDENTITY (prior city / prior anchor) must never
// paint, even if it was already posted to the message queue when the
// identity changed. Two fences enforce it — the runID gate (bumpIdentity
// starts a new run, invalidating older runIDs) and the commitFloor (the
// explicit want-axis fence, exercised here with a hand-built result that
// passes the runID gate).
func TestTextResult_IdentityFloorDropsQueuedResult(t *testing.T) {
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTimeUTC()))
	slots, err := store.NewSlots(testFrozenTimeUTC(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	m.slots = slots
	m.place = testPlaceBerlin()
	m.startup = startupReady

	// Two identity changes (e.g. two rapid atlas re-picks).
	m, _ = m.bumpIdentity(surfaceText)
	m.place = testPlaceTokyo()
	m, _ = m.bumpIdentity(surfaceText)
	st := m.refresh[surfaceText]
	if st.commitFloor != 2 || st.runningRunID != 2 {
		t.Fatalf("precondition: floor=%d runID=%d, want 2/2", st.commitFloor, st.runningRunID)
	}

	// (a) The first identity's real result: superseded RunID → dropped.
	var tm tea.Model = m
	tm, _ = tm.Update(FirstFrameReadyMsg{RunID: 1, WantID: 1,
		Positions: []aspect.Position{{Body: aspect.Sun, Longitude: 999}}})
	m = tm.(Model)
	if len(m.chart.positions) != 0 {
		t.Errorf("(a) superseded-run result painted: %v", m.chart.positions)
	}

	// (b) The floor fence itself: a result that would PASS the runID gate
	// but whose want predates the identity floor must still drop.
	tm = m
	tm, _ = tm.Update(FirstFrameReadyMsg{RunID: m.refresh[surfaceText].runningRunID, WantID: 1,
		Positions: []aspect.Position{{Body: aspect.Sun, Longitude: 999}}})
	m = tm.(Model)
	if len(m.chart.positions) != 0 {
		t.Errorf("(b) below-floor result painted: %v", m.chart.positions)
	}
}

// testPlaceTokyo is a third fixture for the re-pick-after-step test
// (distinct from Berlin / Amsterdam used elsewhere).
func testPlaceTokyo() location.PlaceInfo {
	return location.PlaceInfo{
		Name:    "Tokyo",
		Country: "JP",
		Lat:     35.6762,
		Lon:     139.6503,
		TZ:      "Asia/Tokyo",
	}
}

// TestTransitTimeFromWorkspace_NilSlotsFallsBackToClock asserts the
// defensive cold-start path: before slots are constructed,
// transitTimeFromWorkspace returns m.clock.Now() so a recompute that
// somehow fires pre-resolution still produces a plausible chart
// rather than a zero time.
func TestTransitTimeFromWorkspace_NilSlotsFallsBackToClock(t *testing.T) {
	clockNow := testFrozenTimeUTC()
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(clockNow))
	// m.slots stays nil.

	got := transitTimeFromWorkspace(m)
	if !got.Equal(clockNow) {
		t.Errorf("nil slots: transitTime = %v, want clock.Now() = %v", got.UTC(), clockNow)
	}
}

// TestTimeStepKeyBinding_DispatchesAdvanceTime asserts that the y key
// binding (lowercase = year forward) reaches advanceTime through the
// Update handler. Smoke test for the binding wiring; the deeper unit
// coverage is above.
func TestTimeStepKeyBinding_DispatchesAdvanceTime(t *testing.T) {
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTimeUTC()))
	{
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
		m = updated.(Model)
	}
	slots, err := store.NewSlots(testFrozenTimeUTC(), testPlaceBerlin())
	if err != nil {
		t.Fatalf("NewSlots: %v", err)
	}
	m.slots = slots
	m.place = testPlaceBerlin()
	m.targetFloatingTime = slots.Floating().Time

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	mm := updated.(Model)
	if cmd == nil {
		t.Fatal("y keypress: expected recompute cmd, got nil")
	}
	want := testFrozenTimeUTC().AddDate(1, 0, 0)
	if !mm.targetFloatingTime.Equal(want) {
		t.Errorf("y keypress: targetFloatingTime = %v, want %v", mm.targetFloatingTime.UTC(), want)
	}
}

// testFrozenTimeUTC returns the canonical fixture instant in UTC for
// time-step assertions where timezone-specific formatting is not the
// point being tested. testFrozenTime (in slot_cursor_test.go) returns
// CEST and is used by the rendering tests.
func testFrozenTimeUTC() time.Time {
	return time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
}
