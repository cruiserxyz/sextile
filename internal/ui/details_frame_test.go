package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// while Aspects is focused, Details is the committed frame's own
// Astrolog report — the text the waterfall was parsed from — with no Details
// spawn, timer, or intermediate frame.

func frameWithReports() FirstFrameReadyMsg {
	f := tabsOverlayFrame()
	f.AspectsReport = "Astrolog 8.00 chart\n  1: Sun (Gem) Squ (Pis) Moon - orb: a0:30' - power: 22.00"
	f.CrossAspectsReport = "Chart #2 comparison\n  1: Sun (Gem) Squ with (Ari) Saturn - orb: a0:30' - power:  9.00"
	return f
}

// A settled commit while Aspects is focused fills Details from the frame at
// once — a comparison pair is selected, so its -rt -aa report.
func TestAspectsDetailsFollowsCommit(t *testing.T) {
	m := tabsPairModel(t, Config{Accessible: true})
	m.width, m.height = 144, 40
	var tm tea.Model = m
	tm, cmd := tm.Update(FirstFrameReadyMsg{})
	m = tm.(Model)
	_ = cmd
	frame := frameWithReports()
	st := m.refresh[surfaceText]
	frame.RunID, frame.WantID = st.runningRunID, st.wantID
	m, cmd = applyTextCommit(m, frame)
	if cmd != nil {
		t.Error("commit with Aspects focused produced a Cmd; Details must come from the frame")
	}
	// The cursor sits on now, so the panel shows `Aspects` and Details is the
	// frame's own report (the specification C1).
	if m.details.title != "[0] Aspects" || m.details.body != frame.AspectsReport {
		t.Errorf("Details after commit = %q / %q, want the frame's own -aa report", m.details.title, m.details.body)
	}
	if m.details.pending || m.details.waitForText || m.details.timerArmed || m.refresh[surfaceDetails].cancel != nil {
		t.Errorf("Details left pending/armed after a frame-sourced fill: %+v", m.details)
	}
	// A later settled commit refreshes it even though Details is not empty.
	frame2 := frame
	frame2.AspectsReport = "newer -aa report"
	m, _ = applyTextCommit(m, frame2)
	if m.details.body != "newer -aa report" {
		t.Errorf("Details did not follow the next commit: %q", m.details.body)
	}
}

// Focusing Aspects from another panel reads the frame synchronously and
// fences the previous context's in-flight report.
func TestFocusAspectsReadsFrameAndFencesOldReport(t *testing.T) {
	m := tabsPairModel(t, Config{Accessible: true})
	m.width, m.height = 144, 40
	m.focused = PanelChart
	m, _ = applyTextCommit(m, frameWithReports())
	m.details = detailsViewState{title: "[0] Chart", body: "chart raw", pending: true}
	m.refresh[surfaceDetails].runningRunID = 3
	m, cmd := pressKey(t, m, '3')
	if cmd != nil {
		t.Error("focusing Aspects returned a Cmd; expected a synchronous frame read")
	}
	if m.details.title != "[0] Aspects" || m.details.body != frameWithReports().AspectsReport {
		t.Errorf("Details after focusing Aspects = %q / %q, want the frame's own -aa report", m.details.title, m.details.body)
	}
	if m.refresh[surfaceDetails].runningRunID == 3 || m.refresh[surfaceDetails].commitFloor != m.refresh[surfaceDetails].wantID {
		t.Error("the previous context's in-flight report was not fenced")
	}
}

// An error frame carries no report; the last complete Details frame stays.
func TestAspectsDetailsKeepsLastFrameOnEmptyReport(t *testing.T) {
	m := tabsPairModel(t, Config{Accessible: true})
	m.width, m.height = 144, 40
	m, _ = applyTextCommit(m, frameWithReports())
	want := m.details
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{Overlay: true})
	if m.details.title != want.title || m.details.body != want.body {
		t.Errorf("Details changed on a report-less frame: %q / %q", m.details.title, m.details.body)
	}
}

// Below the dual-column width there is no Details panel and nothing happens.
func TestAspectsDetailsNoopWhenHidden(t *testing.T) {
	m := tabsPairModel(t, Config{Accessible: true})
	m.width, m.height = 80, 24
	m.limitedViewShown = true // the one-time notice is not what this test asks about
	m, cmd := applyTextCommit(m, frameWithReports())
	if cmd != nil || m.details.body != "" {
		t.Errorf("hidden Details was filled or produced a Cmd: %+v", m.details)
	}
}

// Deselecting the pair while the cross tab is active clamps the tab to
// Aspects at once; Details must switch to the frame's own report
// in the same keypress, not linger on the cross report until the single
// frame settles (the reviewer an earlier change HIGH).
func TestPairDeselectSwitchesDetailsAtOnce(t *testing.T) {
	m := tabsOnPair(t, Config{Accessible: true})
	m.width, m.height = 144, 40
	// The `]` that reached the pair tab bumped the text identity, so the frame
	// carries the settled run ids like a real result would.
	frame := frameWithReports()
	st := m.refresh[surfaceText]
	frame.RunID, frame.WantID = st.runningRunID, st.wantID
	m, _ = applyTextCommit(m, frame)
	if m.details.title != "[0] 2 in 1" {
		t.Fatalf("precondition: cross report shown, got %q", m.details.title)
	}
	m, _ = setPairSelection(m, false)
	if m.slots.ComparisonSelected() {
		t.Fatal("precondition: the pair must be deselected")
	}
	if m.details.title != "[0] Aspects" || m.details.body != frameWithReports().AspectsReport {
		t.Errorf("Details after deselect = %q / %q, want the frame's own -aa report at once", m.details.title, m.details.body)
	}
	if m.details.pending || m.details.waitForText {
		t.Errorf("Details left waiting after a frame-sourced fill: %+v", m.details)
	}
	// Reselecting changes what the Aspects panel shows, so Details follows to
	// it in the same keypress; the overlay commit then refreshes both.
	m, _ = setPairSelection(m, true)
	// With the cursor on now, reselecting lands on `Aspects`, not the pair
	// (the specification C1), so Details follows to the
	// frame's own report, which is what the panel shows.
	if m.details.title != "[0] Aspects" || m.details.body != frameWithReports().AspectsReport {
		t.Errorf("Details after reselect = %q / %q, want the frame's own report (the panel shows Aspects)", m.details.title, m.details.body)
	}
}
