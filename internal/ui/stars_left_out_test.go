package ui

import (
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
)

const starsLeftNote = "no fixed stars in the influence chart, Astrolog 8.00 miscounts them"

func countStarsNote(m Model) int {
	n := 0
	for _, l := range m.consoleLines() {
		if strings.Contains(l.text, starsLeftNote) {
			n++
		}
	}
	return n
}

// The Chart report leaves fixed stars out (Astrolog NEXT.md BUG 7) and the large
// log says so once per set of stars, not on every refresh and not on a quiet one.
func TestStarsLeftOutSaidOnce(t *testing.T) {
	m := detailsTestModel(t)
	m.focused = PanelChart
	m.lens.Registry = lookup.Hellenistic.Registry
	m.lens.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 2, 89, 96}}

	// The real build carries the stars the chart report leaves out.
	msg := runDetailsCmd(t, m)
	p, _ := msg.payload.(detailsPayload)
	if !p.influence || len(p.starsLeft) != 2 {
		t.Fatalf("payload influence=%v starsLeft=%v, want the two ticked stars", p.influence, p.starsLeft)
	}
	if msg.err != nil {
		t.Fatalf("the chart report with stars ticked failed: %v", msg.err)
	}

	m, _ = m.commitRefresh(surfaceDetails, msg)
	if got := countStarsNote(m); got != 1 {
		t.Fatalf("first commit wrote %d notes, want 1", got)
	}
	for _, l := range m.logPanelLines() {
		if strings.Contains(l, starsLeftNote) {
			t.Errorf("the Log panel holds %q", l)
		}
	}
	m, _ = m.commitRefresh(surfaceDetails, msg)
	if got := countStarsNote(m); got != 1 {
		t.Errorf("a repeat of the same set wrote a second note (%d)", got)
	}

	// A quiet minute refresh says nothing.
	quiet := m
	quiet.starsLeftSaid = ""
	quiet.quiet[surfaceDetails] = msg.wantID
	before := countStarsNote(quiet)
	quiet, _ = quiet.commitRefresh(surfaceDetails, msg)
	if got := countStarsNote(quiet); got != before {
		t.Errorf("a quiet refresh wrote a note")
	}

	// The set changes: said again.
	p.starsLeft = []int{89}
	msg.payload = p
	m, _ = m.commitRefresh(surfaceDetails, msg)
	if got := countStarsNote(m); got != 2 {
		t.Errorf("a changed set wrote %d notes in all, want 2", got)
	}

	// No star left out: nothing, and the same set later is said again.
	p.starsLeft = nil
	msg.payload = p
	m, _ = m.commitRefresh(surfaceDetails, msg)
	p.starsLeft = []int{89}
	msg.payload = p
	m, _ = m.commitRefresh(surfaceDetails, msg)
	if got := countStarsNote(m); got != 3 {
		t.Errorf("after a star-free report the set was not said again (%d)", got)
	}

	// Not the chart report: a body report never says it.
	body := detailsTestModel(t)
	body.focused = PanelBodies
	if bp, _ := runDetailsCmd(t, body).payload.(detailsPayload); bp.influence {
		t.Error("a body report is marked as the influence chart")
	}
}
