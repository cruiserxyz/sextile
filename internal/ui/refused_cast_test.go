package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A cast Astrolog refuses never blanks the panels. For another chart than the
// one on screen, Bodies and Aspects say the cast failed, under the cursor's
// chart name, and draw nothing of the chart they showed before. The frame is
// not asked for again until what is asked for changes.
func TestRefusedCastOfAnotherChartSaysSo(t *testing.T) {
	m, _ := pickModel(t)
	_, before := leftColumn(t, m.View())
	m1, _ := libraryPick(t, m, testNamedChart("Picked A", fixtureOneP), pickMoment)
	tm, _ := m1.Update(FirstFrameReadyMsg{Err: errors.New("astrolog: exit status 1"),
		RunID: m1.refresh[surfaceText].runningRunID, WantID: m1.refresh[surfaceText].runningWantID})
	m2 := tm.(Model)

	_, after := leftColumn(t, m2.View())
	if n := strings.Count(ansi.Strip(after), "Couldn't cast Picked A."); n != 2 {
		t.Errorf("Bodies and Aspects say the cast failed %d times, want 2:\n%s", n, after)
	}
	for _, row := range strings.Split(ansi.Strip(before), "\n") {
		if r := strings.Trim(row, "│ "); len(r) > 8 && !strings.ContainsAny(r, "─╭╰") && strings.Contains(ansi.Strip(after), r) {
			t.Errorf("the chart shown before is still drawn (%q):\n%s", r, after)
		}
	}
	if log := strings.Join(m2.logPanelRows(), "\n"); !strings.Contains(log, "Picked A not cast,") {
		t.Errorf("the Log does not name the chart:\n%s", log)
	}
	if m2.textOutstanding() {
		t.Error("the refused frame is asked for again")
	}
}

// A refused cast of the chart already on screen (a time step of the floating
// chart) keeps that chart's last frame, faint, and the floating chart keeps
// the moment the frame was cast at.
func TestRefusedCastOfTheSameChartKeepsItsFrame(t *testing.T) {
	commit := func(m Model, msg FirstFrameReadyMsg) Model {
		msg.RunID, msg.WantID = m.refresh[surfaceText].runningRunID, m.refresh[surfaceText].wantID
		tm, _ := m.Update(msg)
		return tm.(Model)
	}
	m := textModel(t)
	m.width, m.height = 132, 40
	m, _ = m.advanceTime(unitDay, +1)
	when := m.targetFloatingTime
	m = commit(m, FirstFrameReadyMsg{Positions: fullChartPositions(), TransitTime: when})
	if len(m.chart.positions) == 0 || m.textPaneState(PanelBodies) != paneCurrent {
		t.Fatal("fixture: no frame committed")
	}
	positions := m.chart.positions

	m, _ = m.advanceTime(unitDay, +1)
	m = commit(m, FirstFrameReadyMsg{Err: errors.New("astrolog: exit status 1"), TransitTime: m.targetFloatingTime})

	if got := m.textPaneState(PanelBodies); got != paneRefused {
		t.Errorf("Bodies state = %d, want paneRefused", got)
	}
	if len(m.chart.positions) != len(positions) || m.chart.positions[0] != positions[0] {
		t.Error("the frame on screen was replaced by the refused one")
	}
	if !m.slots.Floating().Time.Equal(when) {
		t.Errorf("the floating chart moved to %v with a refused frame, want %v", m.slots.Floating().Time, when)
	}
	// The Log names the failure once; Bodies and Aspects say nothing of it.
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Sun") || strings.Count(view, " not cast,") != 1 {
		t.Errorf("Bodies no longer draws the kept frame:\n%s", view)
	}
	if m.textOutstanding() {
		t.Error("the refused frame is asked for again")
	}
}
