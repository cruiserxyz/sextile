package ui

import (
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/lookup"
)

// A search that left a body out because Astrolog has no data for the date lands
// like any other: no line in the Log panel, no error mark, and the large log
// holds which body was left out and Astrolog's own line.
func TestATimelineWithABodyLeftOutSaysNothingOnScreen(t *testing.T) {
	const said = "Chiron's ephemeris is restricted to JD 1967601.5 - JD 3419437.5"
	m := detailsTimelineModel(t, Config{Accessible: true})
	panel := len(m.logPanelLines())
	p := timelinePayload{key: m.list(listTimeline).key, leftOut: lookup.LeftOut{Bodies: []string{"Chiron", "Chiron"}, Lines: []string{said, said}}}
	m, _ = m.commitTimelineFor(listTimeline, refreshResultMsg{surface: listTimeline.surface(), payload: p})
	if got := m.logPanelLines()[panel:]; len(got) != 0 {
		t.Errorf("the Log panel took %q, want nothing", got)
	}
	if m.errorUnseen {
		t.Error("an unseen-error mark was set")
	}
	var traced []string
	for _, s := range consoleLineTexts(m) {
		if strings.Contains(s, "Chiron") {
			traced = append(traced, s)
		}
	}
	want := []string{"bodies: the timeline search left out Chiron: no ephemeris data for this date", "bodies: Astrolog said " + said}
	if strings.Join(traced, "\n") != strings.Join(want, "\n") {
		t.Errorf("the console holds %q, want %q", traced, want)
	}
}
