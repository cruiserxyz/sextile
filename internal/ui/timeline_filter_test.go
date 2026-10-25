package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// the specification. The week fixture is 34 rows; "venus" is a word in
// some of them and a subsequence of more.

func typeQuery(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		k := runes(string(r))
		if r == ' ' {
			k = tea.KeyMsg{Type: tea.KeySpace}
		}
		m, _ = tpress(t, m, k)
	}
	return m
}

func shownLines(m Model) []string {
	var out []string
	for _, r := range m.timelineShown() {
		out = append(out, alignTimelineRow(r))
	}
	return out
}

// Rules 1, 2: `/` opens the line, typing filters live (exact by default),
// the order stays chronological and the rows above shrink by one.
func TestSlashFiltersTheTimelineLiveWithTheSharedMatcher(t *testing.T) {
	m := onTimeline(t, Config{Accessible: true})
	full := m.timelineHeight()
	m, _ = tpress(t, m, runes("/"))
	if !m.timeline.searching || m.timelineHeight() != full-1 {
		t.Fatalf("searching=%v height %d, want open and %d", m.timeline.searching, m.timelineHeight(), full-1)
	}
	m = typeQuery(t, m, "venus")
	got := shownLines(m)
	var want []string
	for _, r := range m.timeline.rows {
		line := alignTimelineRow(r)
		if strings.Contains(strings.ToLower(line), "venus") {
			want = append(want, line)
		}
	}
	if len(want) == 0 || len(want) == len(m.timeline.rows) {
		t.Fatalf("fixture: %d of %d rows match, want a proper subset", len(want), len(m.timeline.rows))
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("shown rows differ from the matcher's:\n got %q\nwant %q", got, want)
	}
	shown := m.timelineShown()
	for i := 1; i < len(shown); i++ {
		if shown[i].Instant.Before(shown[i-1].Instant) {
			t.Errorf("row %d is earlier than row %d: the filter reordered the list", i, i-1)
		}
	}
	rows := detailsBodyRows(renderDetails(m))
	last := rows[len(rows)-1]
	if !strings.HasPrefix(last, "/venus█") || !strings.HasSuffix(last, fmt.Sprintf("%d of %d", len(want), len(m.timeline.rows))) {
		t.Errorf("last row = %q, want `/venus█` with the count right-aligned", last)
	}
}

// Rule 3: while the line is open, j k y d space and the comma are text.
func TestOpenSearchLineTakesEveryLetterAsText(t *testing.T) {
	m := onTimeline(t, Config{Accessible: true})
	floating := m.slots.Floating().Time
	m, _ = tpress(t, m, runes("/"))
	m = typeQuery(t, m, "jkyd, o")
	if m.timeline.query != "jkyd, o" {
		t.Fatalf("query = %q, want the keys typed verbatim", m.timeline.query)
	}
	if !m.slots.Floating().Time.Equal(floating) || len(m.timeline.marks) != 0 {
		t.Errorf("a typed letter acted: floating %v marks %d", m.slots.Floating().Time, len(m.timeline.marks))
	}
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	if m.timeline.query != "jkyd, " {
		t.Errorf("after Backspace the query = %q", m.timeline.query+"|")
	}
}

// Rules 3 and 3a: Enter closes the line and keeps the query; `/` reopens it
// prefilled; only deleting it empties it. Esc is TestEscGoesBackToTheFilterTheLineOpenedWith.
func TestEnterClosesTheLineAndKeepsTheQuery(t *testing.T) {
	for _, closer := range []tea.KeyMsg{keyEnter} {
		m := onTimeline(t, Config{Accessible: true})
		m, _ = tpress(t, m, runes("/"))
		m = typeQuery(t, m, "venus")
		n := len(m.timelineShown())
		// q is text and Left does nothing while the line is open
		// (time-keys-everywhere rule 5); Left must not detach either.
		m, _ = tpress(t, m, keyLeft)
		if !m.timeline.searching || m.timeline.query != "venus" || !m.details.attached {
			t.Fatalf("Left in the open line: searching=%v query=%q attached=%v", m.timeline.searching, m.timeline.query, m.details.attached)
		}
		m, cmd := tpress(t, m, runes("q"))
		if quits(cmd) || !m.timeline.searching || m.timeline.query != "venusq" || !m.details.attached {
			t.Fatalf("q in the open line: searching=%v query=%q attached=%v", m.timeline.searching, m.timeline.query, m.details.attached)
		}
		m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
		m, _ = tpress(t, m, closer)
		if m.timeline.searching || m.timeline.query != "venus" || len(m.timelineShown()) != n {
			t.Fatalf("%v: searching=%v query=%q shown=%d, want closed with the query kept", closer, m.timeline.searching, m.timeline.query, len(m.timelineShown()))
		}
		if !m.details.attached {
			t.Errorf("%v closed the line and detached Details", closer)
		}
		rows := detailsBodyRows(renderDetails(m))
		if want := fmt.Sprintf("/venus  %d of %d", n, len(m.timeline.rows)); rows[len(rows)-1] != want {
			t.Errorf("closed indicator = %q, want %q", rows[len(rows)-1], want)
		}
		m, _ = tpress(t, m, runes("/"))
		if !m.timeline.searching || m.timeline.query != "venus" {
			t.Errorf("`/` reopened with %q, want the line prefilled with venus", m.timeline.query)
		}
		for range "venus" {
			m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
		}
		if m.timeline.query != "" || len(m.timelineShown()) != len(m.timeline.rows) {
			t.Errorf("deleting the query left %q with %d rows", m.timeline.query, len(m.timelineShown()))
		}
	}
}

// Rule 3a: nothing but deleting it, or a new month's rows, empties the query.
func TestTheQueryEmptiesOnlyByDeletingOrANewMonth(t *testing.T) {
	m := onTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, runes("/"))
	m = typeQuery(t, m, "venus")
	m, _ = tpress(t, m, keyEnter)
	// A jump, a tab change and a detach within the month follow.
	m, _ = tpress(t, m, keyEnter) // jump to the cursor row
	m, _ = tpress(t, m, runes("]"))
	m, _ = tpress(t, m, runes("["))
	if m.timeline.query != "venus" {
		t.Fatalf("query = %q after Enter, tab changes: only deleting or a new month may clear it", m.timeline.query)
	}
	m, _ = tpress(t, m, keyEsc)
	if m.timeline.query != "venus" {
		t.Fatalf("query = %q after detaching Details", m.timeline.query)
	}
	next := m.slots.Floating().Time.AddDate(0, 1, 0)
	m, cmd := applyTextCommit(m, FirstFrameReadyMsg{TransitTime: next})
	m = pump(t, m, cmd)
	if m.timeline.query != "" || m.timeline.searching {
		t.Errorf("a new month kept query %q", m.timeline.query)
	}
}

// Rule 5: nothing matches.
func TestNoMatchShowsOneLine(t *testing.T) {
	m := onTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, runes("/"))
	m = typeQuery(t, m, "zzzq")
	rows := detailsBodyRows(renderDetails(m))
	if rows[0] != `No rows match "zzzq".` {
		t.Errorf("first row = %q, want the no-match line", rows[0])
	}
	if !strings.HasPrefix(rows[len(rows)-1], "/zzzq█") || !strings.HasSuffix(rows[len(rows)-1], fmt.Sprintf("0 of %d", len(m.timeline.rows))) {
		t.Errorf("last row = %q", rows[len(rows)-1])
	}
	for _, r := range rows[1 : len(rows)-1] {
		if r != "" {
			t.Errorf("stray row %q between the no-match line and the search line", r)
		}
	}
}

// Rule 6: nearest event, Space marks and `,y` act on the rows shown.
func TestNearestMarksAndCopyActOnTheRowsShown(t *testing.T) {
	m := onTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, runes("/"))
	m = typeQuery(t, m, "venus")
	m, _ = tpress(t, m, keyEsc)
	shown := m.timelineShown()
	if got := m.copiedRows(); len(got) != len(shown) {
		t.Errorf(",y copies %d rows, want the %d shown", len(got), len(shown))
	}
	m = homedTimeline(t, m)
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeySpace})
	if !m.timeline.marks[markOf(shown[0])] || len(m.timeline.marks) != 1 {
		t.Errorf("Space marked %v, want the first shown row", m.timeline.marks)
	}
	if got := m.copiedRows(); len(got) != 1 || got[0] != alignTimelineRow(shown[0]) {
		t.Errorf(",y with one mark = %q", got)
	}
	// Enter acts on the shown row under the cursor.
	m, _ = tpress(t, m, runes("j"))
	want := landedAt(shown[1].Instant)
	m, cmd := tpress(t, m, keyEnter)
	m = pump(t, m, cmd)
	if got := m.slots.Floating().Time; !got.Equal(want) {
		t.Errorf("Enter jumped to %v, want the second shown row %v", got, want)
	}
}

// Taste: the three frames at 132x40, in colour and with colour off.
func TestTimelineFilterFrames(t *testing.T) {
	forceColor(t)
	for _, mode := range []struct {
		name string
		cfg  Config
	}{{"color", Config{Color: ColorAlways}}, {"never", Config{Color: ColorNever}}} {
		m := onTimeline(t, mode.cfg)
		m, _ = tpress(t, m, runes("/"))
		m = typeQuery(t, m, "venus")
		open := m.View()
		assertFrameGeometry(t, open, m.width, 40)
		checkGolden(t, "timeline-filter-open-"+mode.name, open)
		m, _ = tpress(t, m, keyEnter)
		closed := m.View()
		assertFrameGeometry(t, closed, m.width, 40)
		checkGolden(t, "timeline-filter-closed-"+mode.name, closed)
		m, _ = tpress(t, m, runes("/"))
		m = typeQuery(t, m, "zzzq")
		none := m.View()
		assertFrameGeometry(t, none, m.width, 40)
		checkGolden(t, "timeline-filter-nomatch-"+mode.name, none)
		if !strings.Contains(ansi.Strip(none), `No rows match "venuszzzq".`) {
			t.Errorf("%s: no-match frame lacks its line", mode.name)
		}
	}
}
