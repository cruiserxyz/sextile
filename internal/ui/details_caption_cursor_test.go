package ui

import (
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// settledOnMercury is a Bodies cursor on Mercury (row 0) with Mercury's report
// committed; the next rows are Venus and Mars.
func settledOnMercury(t *testing.T) Model {
	t.Helper()
	stubRawReports(t)
	m := detailsTestModel(t)
	m.cfg = Config{Color: ColorNever}
	m.focused = PanelBodies
	m.chart.positions = []aspect.Position{{Object: objMercury}, {Object: objVenus}, {Object: objMars}}
	m.chart.settled = true // the frame beside the report is a committed one
	m.bodyCursorIdx = 0
	m, cmd := m.requestDetails()
	m = pump(t, m, cmd)
	if m.details.title != "[0] Mercury" || m.details.body != "single raw" || m.details.pending {
		t.Fatalf("fixture: title %q body %q pending %v", m.details.title, m.details.body, m.details.pending)
	}
	return m
}

// Acceptance 1. One key from Mercury to Venus, the report not yet back: the tab
// reads Venus and the pane draws nothing of Mercury's report (C2, C5). The
// committed report stays in the model for the next key to come back to.
func TestDetailsCaptionFollowsBodyCursorBeforeReportCommit(t *testing.T) {
	m := settledOnMercury(t)
	m, _ = tpress(t, m, keyDown) // the returned command is NOT run
	if got := stripOfDetails(m); got != "Venus" {
		t.Errorf("caption after one key = %q, want Venus", got)
	}
	if got := detailsPanelText(m); strings.Contains(got, "single raw") || !detailsBodyBlank(got) {
		t.Errorf("the pane under the Venus caption: want it blank (no `…` since 2026-11-06), none of Mercury's report:\n%s", got)
	}
	if m.details.title != "[0] Mercury" || m.details.body != "single raw" {
		t.Errorf("committed report = %q / %q, want Mercury's kept", m.details.title, m.details.body)
	}
	if !m.details.pending {
		t.Error("the report for Venus is not pending")
	}
}

// Acceptance 2.
func TestDetailsCaptionStaysVenusWhenReportLands(t *testing.T) {
	m := settledOnMercury(t)
	m, cmd := tpress(t, m, keyDown)
	m = pump(t, m, cmd)
	if m.details.title != "[0] Venus" || m.details.pending {
		t.Fatalf("report did not land: title %q pending %v", m.details.title, m.details.pending)
	}
	if got := stripOfDetails(m); got != "Venus" {
		t.Errorf("caption after the report = %q, want Venus", got)
	}
}

// Acceptance 3. Before the report: the caption names the current body whether
// the key was a tap or a repeat, and the pane is the same `…` either way; a
// held key dims nothing (C8).
func TestDetailsCaptionIsCurrentWhileHeldKeyDimsText(t *testing.T) {
	m := settledOnMercury(t)
	m, _ = tpress(t, m, keyDown)
	m, _ = tpress(t, m, keyDown) // the repeat
	if got := stripOfDetails(m); got != "Mars" {
		t.Errorf("held caption = %q, want Mars", got)
	}
	if m.detailsTabDimmed(TabDetailsReport) || strings.HasSuffix(stripTitle(m), detailsPendingMark) {
		t.Error("a held key marked the report as updating; another subject is not an update")
	}
	if got := detailsPanelText(m); !detailsBodyBlank(got) || strings.Contains(got, "single raw") {
		t.Errorf("held, before the report: want a blank pane and no Mercury report:\n%s", got)
	}
}

// Acceptance 4. No body under the cursor: the focused panel's title, as today.
func TestDetailsCaptionWithoutBodyNamesFocusedPanel(t *testing.T) {
	m := detailsTestModel(t)
	m.focused = PanelBodies
	m.chart.positions = nil
	if got := stripOfDetails(m); got != "Bodies" {
		t.Errorf("no body cursor: caption %q, want Bodies", got)
	}
}

// Acceptance 5. The search and the scroll place belong to the committed report:
// the key alone does not clear them, the landing report (a new subject) does.
func TestDetailsCaptionKeyLeavesSearchAndScrollToTheReport(t *testing.T) {
	m := settledOnMercury(t)
	m.details.query, m.details.scrollOffset = "sun", 3
	m, cmd := tpress(t, m, keyDown)
	if m.details.query != "sun" || m.details.scrollOffset != 3 {
		t.Errorf("a key changed query %q / scroll %d before the report", m.details.query, m.details.scrollOffset)
	}
	m = pump(t, m, cmd)
	if m.details.query != "" || m.details.scrollOffset != 0 {
		t.Errorf("a new subject kept query %q / scroll %d", m.details.query, m.details.scrollOffset)
	}
}

// Acceptance 6. The other tabs are as before, and the wait mark ` …` is the
// same subject at another version only: a new subject has none (C4, C5).
func TestDetailsCaptionLeavesWaitMarkAndOtherTabs(t *testing.T) {
	m := settledOnMercury(t)
	m, _ = tpress(t, m, keyDown)
	m, _ = tpress(t, m, keyDown) // the repeat
	if strings.Contains(m.View(), "Mars …") {
		t.Error("a new subject carries the wait mark")
	}
	m.focused = PanelChart
	tabs := m.panelTabs(PanelDetails)
	if tabs[0].Title != "Now" || len(tabs) < 1 {
		t.Errorf("Chart focus tabs = %+v", tabs)
	}
}

// Acceptance 7. Frame-owned chrome stays with the frame: the Aspects strip
// and the Bodies title do not move at the key.
func TestDetailsCaptionLeavesAspectsStripAndBodiesTitle(t *testing.T) {
	m := settledOnMercury(t)
	aspects, bodies := m.panelTabs(PanelAspects), m.panelTabs(PanelBodies)
	m, _ = tpress(t, m, keyDown)
	if got := m.panelTabs(PanelAspects); len(got) != len(aspects) || got[0] != aspects[0] {
		t.Errorf("Aspects strip moved: %+v -> %+v", aspects, got)
	}
	if got := m.panelTabs(PanelBodies); got[0] != bodies[0] {
		t.Errorf("Bodies title moved: %+v -> %+v", bodies, got)
	}
}
