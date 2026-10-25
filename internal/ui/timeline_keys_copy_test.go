package ui

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// the specification. The acceptance rule numbers below are that
// spec's.

func captureYank(t *testing.T) *[]string {
	t.Helper()
	var copied []string
	old := yankLine
	yankLine = func(s string) error { copied = append(copied, s); return nil }
	t.Cleanup(func() { yankLine = old })
	return &copied
}

// displayed is the rows as the panel shows and `,y` copies them (C11: a
// comparing row opens with its numeral), under the Accessible config every
// caller's model uses.
func displayed(rows ...lookup.TimelineRow) string {
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = displayTimelineRow(r, Config{Accessible: true})
	}
	return strings.Join(lines, "\n")
}

func lastNote(m Model) string {
	if len(m.logLines) == 0 {
		return ""
	}
	return m.logLines[len(m.logLines)-1].text
}

// Rule 1: every step key acts as on the main screen while Details is attached,
// on the timeline tab and on the Report tab alike.
func TestTimelineTabForwardsEveryStepKey(t *testing.T) {
	for _, k := range []string{"y", "Y", "w", "W", "d", "D", "h", "H", "m", "M"} {
		stubTextCompute(t)
		m := onTimeline(t, Config{Accessible: true})
		start := m.targetFloatingTime
		main := m
		main, _ = tpress(t, main, keyEsc)
		main, _ = tpress(t, main, runes(k))
		if main.targetFloatingTime.Equal(start) {
			t.Fatalf("fixture: %s does not step on the main screen", k)
		}
		m, _ = tpress(t, m, runes(k))
		if !m.targetFloatingTime.Equal(main.targetFloatingTime) {
			t.Errorf("attached %s: target %v, want what the main screen does, %v", k, m.targetFloatingTime, main.targetFloatingTime)
		}
		if !m.details.attached {
			t.Errorf("attached %s detached Details", k)
		}

		// Report tab: the same step (time-keys-everywhere rule 3).
		r := onTimeline(t, Config{Accessible: true})
		r, _ = tpress(t, r, runes("]"))
		r, _ = tpress(t, r, runes(k))
		if !r.targetFloatingTime.Equal(main.targetFloatingTime) {
			t.Errorf("%s on the report tab: target %v, want %v", k, r.targetFloatingTime, main.targetFloatingTime)
		}
	}
}

// Time keys step now from everywhere (the specification
// rule 1): with the anchored slot focused, every step key and n/N move the
// floating target, and the cursor, the selection and the active tab stay.
func TestStepKeysMoveNowFromTheAnchoredSlot(t *testing.T) {
	stubTextCompute(t)
	m := loadedTimeline(t, Config{Accessible: true})
	for _, k := range []tea.KeyMsg{keyUp, runes("0"), keyEnter} {
		var cmd tea.Cmd
		m, cmd = tpress(t, m, k)
		m = pump(t, m, cmd)
	}
	if m.focusedIsFloating() || !m.details.attached {
		t.Fatal("fixture: the anchored slot must be focused with Details attached")
	}
	selection := func(m Model) []*store.Slot { return m.slots.SelectedInOrder() }
	cursor, tab, sel := m.cursorIdx, m.activeTabID(PanelDetails), selection(m)
	for _, k := range []string{"y", "Y", "o", "w", "d", "D", "h", "m"} {
		before := m.targetFloatingTime
		var cmd tea.Cmd
		m, cmd = tpress(t, m, runes(k))
		if m.targetFloatingTime.Equal(before) {
			t.Errorf("%s did not step the floating slot from the anchored slot", k)
		}
		m = pump(t, m, cmd)
		if !m.slots.Floating().Time.Equal(m.targetFloatingTime) {
			t.Errorf("%s: the slot's Time %v did not follow the target %v", k, m.slots.Floating().Time, m.targetFloatingTime)
		}
		if m.cursorIdx != cursor || m.activeTabID(PanelDetails) != tab || !reflect.DeepEqual(selection(m), sel) {
			t.Errorf("%s moved the cursor, the selection or the tab", k)
		}
	}
	stepped := m.targetFloatingTime
	m, _ = tpress(t, m, runes("n"))
	if m.targetFloatingTime.Equal(stepped) || m.cursorIdx != cursor {
		t.Errorf("n from the anchored slot: target %v (was %v), cursor %d (was %d)", m.targetFloatingTime, stepped, m.cursorIdx, cursor)
	}
	m, _ = tpress(t, m, runes("N"))
	if !m.targetFloatingTime.Equal(stepped) || m.cursorIdx != cursor {
		t.Errorf("N from the anchored slot: target %v, want %v back", m.targetFloatingTime, stepped)
	}
}

// Rule 2 and 3: `,` arms the leader inside Details with the ordinary pending
// state, Esc cancels, and `,y` copies the whole list when nothing is marked.
func TestLeaderYCopiesEveryRowAsDisplayed(t *testing.T) {
	copied := captureYank(t)
	m := onTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, runes(","))
	if !m.leader.pending {
		t.Fatal(", did not arm the leader inside Details")
	}
	m, _ = tpress(t, m, runes("y"))
	if m.leader.pending {
		t.Error("the leader is still pending after a completed sequence")
	}
	if want := displayed(m.timeline.rows...); len(*copied) != 1 || (*copied)[0] != want {
		t.Fatalf(",y copied %q, want the aligned rows %q", *copied, want)
	}
	if got := lastNote(m); !strings.Contains(got, "timeline: copied 34 rows") {
		t.Errorf("Log note = %q, want it to say 34 rows were copied", got)
	}
}

func TestLeaderInDetailsCancelsWithEscAndSwallowsWrongKeys(t *testing.T) {
	copied := captureYank(t)
	stubTextCompute(t)
	m := onTimeline(t, Config{Accessible: true})
	target := m.targetFloatingTime
	m, _ = tpress(t, m, runes(","))
	m, _ = tpress(t, m, keyEsc)
	if m.leader.pending || !m.details.attached {
		t.Fatalf("Esc: pending=%v attached=%v, want the leader cancelled and Details still attached", m.leader.pending, m.details.attached)
	}
	m, _ = tpress(t, m, runes(","))
	m, _ = tpress(t, m, runes("x"))
	m, _ = tpress(t, m, runes("y")) // a plain year step now, not a copy
	if len(*copied) != 0 {
		t.Errorf(", x then y copied %q", *copied)
	}
	if want := target.AddDate(1, 0, 0); !m.targetFloatingTime.Equal(want) {
		t.Errorf("y after a cancelled leader: target %v, want the year step %v", m.targetFloatingTime, want)
	}
}

func TestLeaderTimesOutInsideDetails(t *testing.T) {
	m := onTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, runes(","))
	gen := m.leader.gen
	tm, _ := m.Update(leaderTimeoutMsg{gen: gen})
	if tm.(Model).leader.pending {
		t.Fatal("the pending leader outlived its timeout inside Details")
	}
}

// `,y` is Details'. On the main screen it stays a mistyped leader sequence.
func TestLeaderYOnTheMainScreenDoesNothing(t *testing.T) {
	copied := captureYank(t)
	stubTextCompute(t)
	m := loadedTimeline(t, Config{Accessible: true})
	target := m.targetFloatingTime
	m, _ = tpress(t, m, runes(","))
	m, _ = tpress(t, m, runes("y"))
	if len(*copied) != 0 || !m.targetFloatingTime.Equal(target) || m.leader.pending {
		t.Fatalf("main ,y: copied %d, target moved %v, pending %v; want nothing", len(*copied), !m.targetFloatingTime.Equal(target), m.leader.pending)
	}
}

// Rule 4: yy copies nothing, and a single y is the year step.
func TestYYNoLongerCopies(t *testing.T) {
	copied := captureYank(t)
	stubTextCompute(t)
	m := onTimeline(t, Config{Accessible: true})
	target := m.targetFloatingTime
	m, _ = tpress(t, m, runes("y"))
	m, _ = tpress(t, m, runes("y"))
	if len(*copied) != 0 {
		t.Fatalf("yy copied %q", *copied)
	}
	if want := target.AddDate(2, 0, 0); !m.targetFloatingTime.Equal(want) {
		t.Errorf("yy: target %v, want two year steps, %v", m.targetFloatingTime, want)
	}
	r := onTimeline(t, Config{Accessible: true})
	r, _ = tpress(t, r, runes("]"))
	r, _ = tpress(t, r, runes("y"))
	r, _ = tpress(t, r, runes("y"))
	// The report tab steps too (time-keys-everywhere rule 3): still no copy.
	if want := target.AddDate(2, 0, 0); len(*copied) != 0 || !r.targetFloatingTime.Equal(want) {
		t.Errorf("report-tab yy: copied %d, target %v, want two year steps, %v", len(*copied), r.targetFloatingTime, want)
	}
}

// A burst of ",y" is the same keys typed slowly; a pasted ",y" is text.
func TestBurstOfLeaderYCopiesAndAPasteDoesNot(t *testing.T) {
	copied := captureYank(t)
	m := onTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(",y"), Paste: true})
	if len(*copied) != 0 || m.leader.pending {
		t.Fatalf("a pasted ,y acted as keys: copied %d pending %v", len(*copied), m.leader.pending)
	}
	m, _ = tpress(t, m, runes(",y"))
	if len(*copied) != 1 {
		t.Fatalf(",y as one read copied %d times, want once", len(*copied))
	}
}

// Rule 5 and 3: Space marks the cursor row; only marked rows are copied, in
// time order as displayed, whatever order they were marked in.
func TestSpaceMarksRowsAndLeaderYCopiesOnlyThose(t *testing.T) {
	copied := captureYank(t)
	m := onTimeline(t, Config{Accessible: true})
	rows := m.timeline.rows
	m, _ = tpress(t, m, runes("j"))
	m, _ = tpress(t, m, runes("j"))
	m, _ = tpress(t, m, runes(" "))
	m, _ = tpress(t, m, runes("k"))
	m, _ = tpress(t, m, runes(" "))
	m, _ = tpress(t, m, runes("j"))
	m, _ = tpress(t, m, runes("j"))
	m, _ = tpress(t, m, runes(" "))
	m, _ = tpress(t, m, runes(" ")) // toggled off again
	m, _ = tpress(t, m, runes(","))
	m, _ = tpress(t, m, runes("y"))
	want := displayed(rows[1], rows[2])
	if len(*copied) != 1 || (*copied)[0] != want {
		t.Fatalf("copied %q, want only the two marked rows in time order, %q", *copied, want)
	}
	if got := lastNote(m); !strings.Contains(got, "copied 2 rows") {
		t.Errorf("Log note = %q, want 2 rows", got)
	}
}

func TestMarkedRowsAreDrawnWithTheSelectionMarkerAndClearOnANewMonth(t *testing.T) {
	m := onTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, runes(" "))
	shown := detailsBodyRows(renderDetails(m))
	if !strings.Contains(shown[0], selectionMarker(true, m.cfg)) {
		t.Errorf("the marked row reads %q, want it to start with the chart slots' marker", shown[0])
	}
	if strings.Contains(shown[1], selectionMarker(true, m.cfg)) {
		t.Errorf("an unmarked row carries the marker: %q", shown[1])
	}
	m.targetFloatingTime = m.targetFloatingTime.AddDate(0, 1, 0)
	m.slots.SetFloatingTime(m.targetFloatingTime)
	m, cmd := syncTimelineVisited(m)
	m = pump(t, m, cmd)
	copied := captureYank(t)
	m, _ = tpress(t, m, runes(","))
	m, _ = tpress(t, m, runes("y"))
	if len(*copied) != 1 || (*copied)[0] != displayed(m.timeline.rows...) {
		t.Errorf("after a new month the mark survived: copied %q", *copied)
	}
}

// Rule 3: the Report tab copies the whole report.
func TestLeaderYOnTheReportTabCopiesTheReport(t *testing.T) {
	copied := captureYank(t)
	m := onTimeline(t, Config{Accessible: true})
	m.details.title, m.details.body = "[0] Chart", influenceReport(t)
	m, _ = tpress(t, m, runes("]"))
	m, _ = tpress(t, m, runes(","))
	m, _ = tpress(t, m, runes("y"))
	if len(*copied) != 1 || (*copied)[0] != influenceReport(t) {
		t.Fatalf("report ,y copied %d items", len(*copied))
	}
}

// U3 5: `,y` in Details confirms with Copied.
func TestLeaderYReadsCopied(t *testing.T) {
	captureYank(t)
	m := onTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, runes(","))
	m, _ = tpress(t, m, runes("y"))
	if got := m.footerIndicator(); got != "Copied" {
		t.Errorf("footer = %q, want Copied", got)
	}
}

// U3 6: `,y` with nothing to copy says so.
func TestLeaderYWithNothingToCopySaysSo(t *testing.T) {
	captureYank(t)
	m := onTimeline(t, Config{Accessible: true})
	m.details.title, m.details.body = "", ""
	m, _ = tpress(t, m, runes("]"))
	m, _ = tpress(t, m, runes(","))
	m, _ = tpress(t, m, runes("y"))
	if got := m.footerIndicator(); got != "Nothing to copy" {
		t.Errorf("footer = %q, want the notice", got)
	}
}

// U3 7: a clipboard that fails shows the notice and leaves the Log's error
// lines as they were.
func TestCopyFailureShowsANotice(t *testing.T) {
	old := yankLine
	yankLine = func(string) error { return errors.New("no clipboard") }
	t.Cleanup(func() { yankLine = old })

	m := onTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, runes(","))
	m, _ = tpress(t, m, runes("y"))
	if got := m.footerIndicator(); got != "Copy failed: the clipboard is unavailable." {
		t.Errorf("footer = %q, want the failure notice", got)
	}
	last := m.logLines[len(m.logLines)-1]
	if last.lane != laneError || last.text != "Couldn't copy the Details panel to the clipboard." {
		t.Errorf("last Log line = %+v, want the unchanged copy error", last)
	}
}
