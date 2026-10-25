package ui

import (
	"strings"
	"testing"
)

// the specification: Enter applies what was typed; Esc closes
// the line and puts back the filter, and the list position, it opened with.

// Timeline: from no filter, and from an applied one; the row cursor comes back.
func TestEscGoesBackToTheFilterTheLineOpenedWith(t *testing.T) {
	m := onTimeline(t, Config{Accessible: true})
	all := len(m.timelineShown())
	m, _ = tpress(t, m, keyDown)
	m, _ = tpress(t, m, keyDown)
	at := m.details.cursor

	m, _ = tpress(t, m, runes("/"))
	m = typeQuery(t, m, "venus")
	m, _ = tpress(t, m, keyUp)
	m, _ = tpress(t, m, keyEsc)
	if m.timeline.searching || m.timeline.query != "" || len(m.timelineShown()) != all {
		t.Fatalf("Esc from no filter: searching=%v query=%q shown=%d, want closed, empty, %d rows",
			m.timeline.searching, m.timeline.query, len(m.timelineShown()), all)
	}
	if m.details.cursor != at {
		t.Errorf("Esc left the cursor on row %d, want %d where the line opened", m.details.cursor, at)
	}
	if !m.details.attached {
		t.Error("Esc in the line detached Details")
	}

	m, _ = tpress(t, m, runes("/"))
	m = typeQuery(t, m, "venus")
	m, _ = tpress(t, m, keyEnter)
	n := len(m.timelineShown())
	m, _ = tpress(t, m, runes("/"))
	m = typeQuery(t, m, " sq")
	m, _ = tpress(t, m, keyEsc)
	if m.timeline.query != "venus" || len(m.timelineShown()) != n {
		t.Errorf("Esc from an applied filter: query=%q shown=%d, want venus with %d rows", m.timeline.query, len(m.timelineShown()), n)
	}
	if !m.details.attached {
		t.Error("Esc in the line detached Details")
	}
	// The closed line's Esc is still the panel's: it detaches.
	m, _ = tpress(t, m, keyEsc)
	if m.details.attached || m.timeline.query != "venus" {
		t.Errorf("Esc with the line closed: attached=%v query=%q, want detached, venus kept", m.details.attached, m.timeline.query)
	}
}

// Details report: the query and the scroll come back.
func TestEscGoesBackInTheDetailsReport(t *testing.T) {
	m := attachedReport(t, false, "[0] Aspects", ownFilterBody(80))
	m.details.scrollOffset = 5
	m = m.clampDetailsOffset()
	scrolled := m.details.scrollOffset
	if scrolled == 0 {
		t.Fatal("fixture: the report does not scroll")
	}
	m, _ = tpress(t, m, runes("/"))
	if !m.detailsReportSearchOpen() {
		t.Fatal("fixture: `/` did not open the report's line")
	}
	m = typeQuery(t, m, "squ")
	m, _ = tpress(t, m, keyEsc)
	if m.details.searching || m.details.query != "" {
		t.Fatalf("Esc: searching=%v query=%q, want closed and empty", m.details.searching, m.details.query)
	}
	if m.details.scrollOffset != scrolled {
		t.Errorf("Esc: scroll %d, want %d where the line opened", m.details.scrollOffset, scrolled)
	}
	if !m.details.attached {
		t.Error("Esc in the line detached Details")
	}

	m, _ = tpress(t, m, runes("/"))
	m = typeQuery(t, m, "squ")
	m, _ = tpress(t, m, keyEnter)
	m, _ = tpress(t, m, runes("/"))
	m = typeQuery(t, m, "zzz")
	m, _ = tpress(t, m, keyEsc)
	if m.details.query != "squ" {
		t.Errorf("Esc from an applied filter: query %q, want squ", m.details.query)
	}
}

// The bars say which key does which.
func TestSearchLineBarsNameEnterAndEsc(t *testing.T) {
	for name, entries := range map[string][]BindingEntry{
		"timeline": timelineSearchBindings(),
		"report":   reportSearchBindings(),
	} {
		var got []string
		for _, e := range entries {
			if e.Bar {
				got = append(got, e.BarKey+" "+e.Action)
			}
		}
		bar := strings.Join(got, " · ")
		if !strings.Contains(bar, "enter Apply") || !strings.Contains(bar, "esc Back") {
			t.Errorf("%s bar %q, want `enter Apply` and `esc Back`", name, bar)
		}
	}
}
