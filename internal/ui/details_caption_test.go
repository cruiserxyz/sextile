package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The Details caption names what its report is about, never "Details" itself
// (the specification, acceptance 1-2).
func TestDetailsCaptionNamesTheSubject(t *testing.T) {
	stubRawReports(t)
	stubCrossBodyReports(t)

	// Chart focus and a body row, single chart.
	m := detailsTestModel(t)
	m.focused = PanelChart
	if got := runDetailsCmd(t, m).payload.(detailsPayload).title; got != "[0] Chart" {
		t.Errorf("Chart focus: caption %q, want %q", got, "[0] Chart")
	}
	m.focused = PanelBodies
	m.bodyCursorIdx = 1
	if got := runDetailsCmd(t, m).payload.(detailsPayload).title; got != "[0] Moon" {
		t.Errorf("Bodies on Moon: caption %q, want %q", got, "[0] Moon")
	}

	// A body while comparing on the pair ①–②.
	p := tabsOnPair(t, Config{Accessible: true})
	p.width, p.height = 144, 40
	p.focused = PanelBodies
	p.chart.positions = []aspect.Position{{Body: aspect.Venus}}
	p.bodyCursorIdx = 0
	if req, ok := p.detailsRequestFor(); !ok || req.title != "[0] Venus 2 in 1" {
		t.Errorf("Bodies on Venus comparing: caption %q (ok=%v), want %q", req.title, ok, "[0] Venus 2 in 1")
	}

	// Aspects, its own report and a pair's.
	a := tabsPairModel(t, Config{Accessible: true})
	a.width, a.height = 144, 40
	var tm tea.Model = a
	tm, _ = tm.Update(FirstFrameReadyMsg{})
	a = tm.(Model)
	frame := frameWithReports()
	st := a.refresh[surfaceText]
	frame.RunID, frame.WantID = st.runningRunID, st.wantID
	a, _ = applyTextCommit(a, frame)
	if a.details.title != "[0] Aspects" {
		t.Errorf("Aspects: caption %q, want %q", a.details.title, "[0] Aspects")
	}
	pp := tabsOnPair(t, Config{Accessible: true})
	pp.width, pp.height = 144, 40
	frame = frameWithReports()
	st = pp.refresh[surfaceText]
	frame.RunID, frame.WantID = st.runningRunID, st.wantID
	pp, _ = applyTextCommit(pp, frame)
	if pp.details.title != "[0] 2 in 1" {
		t.Errorf("Aspects pair: caption %q, want %q", pp.details.title, "[0] 2 in 1")
	}

	// Before any report: the focused ring panel's own title.
	for panel, want := range map[PanelID]string{PanelChart: "Chart", PanelBodies: "Bodies", PanelAspects: "Aspects"} {
		s := detailsTestModel(t)
		s.focused = panel
		s.details.title = ""
		if got := s.panelTabs(PanelDetails)[0].Title; got != want {
			t.Errorf("no report yet, %v focused: caption %q, want %q", panel, got, want)
		}
	}
	for _, p := range defaultPanels() {
		if p.Name() == PanelDetails && strings.Contains(p.Title(), "Details") {
			t.Errorf("static Details panel title %q still says Details", p.Title())
		}
	}
}
