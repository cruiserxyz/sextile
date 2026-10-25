package ui

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
)

// manyTimelineRows are n distinct rows an hour apart, so a list is long enough
// to page through. Each has its own line, so each has its own identity (markOf).
func manyTimelineRows(t *testing.T, n int) []lookup.TimelineRow {
	t.Helper()
	week := weekRows(t)
	rows := make([]lookup.TimelineRow, n)
	for i := range rows {
		rows[i] = week[i%len(week)]
		rows[i].Instant = week[0].Instant.Add(time.Duration(i) * time.Hour)
		rows[i].Line = fmt.Sprintf("row %03d  %s", i, rows[i].Line)
	}
	return rows
}

// pagedTimeline is a loaded timeline of n rows with Details attached on it, at
// a height that gives the list a window well short of n.
func pagedTimeline(t *testing.T, n int) Model {
	t.Helper()
	rows := manyTimelineRows(t, n)
	stubTimeline(t, func(lookup.TimelineRequest) (lookup.TimelineResult, error) {
		return lookup.TimelineResult{Rows: rows, Commands: []string{"astrolog -dm"}}, nil
	})
	m := timelineModel(t, Config{Accessible: true})
	m.width = max(m.width, dualColumnMinWidth(m.shownCount()))
	m.height = 30
	m, cmd := syncTimelineVisited(m)
	m = pump(t, m, cmd)
	if len(m.timeline.rows) != n {
		t.Fatalf("fixture: %d timeline rows, want %d", len(m.timeline.rows), n)
	}
	m, _ = tpress(t, m, runes("0"))
	m, _ = tpress(t, m, keyEnter)
	if !m.details.attached || m.activeTabID(PanelDetails) != TabDetailsTimeline {
		t.Fatalf("fixture: attached=%v tab=%s", m.details.attached, m.activeTabID(PanelDetails))
	}
	if v := m.timelineHeight(); v < 6 || 3*v > n {
		t.Fatalf("fixture: the window is %d rows over %d", v, n)
	}
	return m
}

// cursorNamesItsRow is the timeline's identity record: after a page key the row
// under the cursor is the one cursorAt names, so a reload finds it again.
func cursorNamesItsRow(t *testing.T, m Model, when string) {
	t.Helper()
	rows := m.timelineShown()
	if m.details.cursor < 0 || m.details.cursor >= len(rows) {
		t.Fatalf("%s: cursor %d outside %d rows", when, m.details.cursor, len(rows))
	}
	if !m.details.cursorSet || m.details.cursorAt != markOf(rows[m.details.cursor]) {
		t.Errorf("%s: cursorAt names %v, but the cursor %d is on %v", when, m.details.cursorAt, m.details.cursor, markOf(rows[m.details.cursor]))
	}
}

// A2: attached on the timeline, PgUp/PgDn move the cursor to the window's edge
// first and the window only on the next press; Home/End/g/G jump; the offset
// is exactly the rule's (no scroll margin), and cursorAt follows every press.
func TestTimelinePageKeysMoveTheCursorFirst(t *testing.T) {
	const n = 100
	m := pagedTimeline(t, n)
	v := m.timelineHeight()
	m.details.cursor, m.details.rowOffset = 2*v-1, v
	m = m.recordTimelineCursor()

	step := func(name, key string, wantCursor, wantOffset int) {
		t.Helper()
		next, _ := tpress(t, m, keyMsgOf(key))
		m = next
		if m.details.cursor != wantCursor || m.details.rowOffset != wantOffset {
			t.Fatalf("%s: cursor %d offset %d, want %d / %d (window %d over %d)", name, m.details.cursor, m.details.rowOffset, wantCursor, wantOffset, v, n)
		}
		cursorNamesItsRow(t, m, name)
	}
	step("PgUp from the window's last row goes to its first", "pgup", v, v)
	step("PgUp again pages the window up by a page less a row", "pgup", 1, 1)
	step("PgDn goes to the window's last row", "pgdown", v, 1)
	step("PgDn again pages the window down", "pgdown", 2*v-1, v)
	step("End", "end", n-1, n-v)
	step("PgUp at the end", "pgup", n-v, n-v)
	step("Home", "home", 0, 0)
	step("G", "G", n-1, n-v)
	step("g", "g", 0, 0)
	// The arrow keys after a page use the margin mover, as today.
	m, _ = tpress(t, m, keyDown)
	if m.details.cursor != 1 {
		t.Errorf("Down after a page: cursor %d, want 1", m.details.cursor)
	}
}

func keyMsgOf(k string) tea.KeyMsg { return keyMsg(k) }

// A2: detached (the cursor hidden), Ctrl+PgDn scrolls the window and can drag the
// cursor to its edge; the next attach re-places the cursor on the nearest row and
// centres it, as it does without any scrolling.
func TestDetachedTimelineReattachesOnTheNearestRow(t *testing.T) {
	base := loadedTimeline(t, Config{Accessible: true})
	base.focused = PanelChart
	base = base.withDetailsTab(PanelChart, TabDetailsTimeline)
	if base.details.attached {
		t.Fatal("fixture: attached")
	}
	scrolled := base
	for i := 0; i < 3; i++ {
		scrolled, _ = tpress(t, scrolled, tea.KeyMsg{Type: tea.KeyCtrlPgDown})
	}
	if scrolled.details.rowOffset == base.details.rowOffset && scrolled.details.cursor == base.details.cursor {
		t.Skip("the fixture's list fits its window, so Ctrl+PgDn has nothing to scroll")
	}
	a, _ := tpress(t, base, keyEnter)
	b, _ := tpress(t, scrolled, keyEnter)
	if !b.details.attached {
		t.Fatal("Enter did not attach")
	}
	rows := b.timelineShown()
	nearest := b.nearestRow(rows)
	if nearest < 0 {
		t.Fatal("fixture: no nearest row")
	}
	if b.details.cursor != nearest || b.details.rowOffset != a.details.rowOffset || b.details.cursor != a.details.cursor {
		t.Errorf("after scrolling and attaching: cursor %d offset %d; attaching without scrolling gives cursor %d offset %d; nearest row is %d",
			b.details.cursor, b.details.rowOffset, a.details.cursor, a.details.rowOffset, nearest)
	}
}

// A2: the `/` search line passes Ctrl+PgUp/PgDn through to the Details scroll,
// PgUp/PgDn page the cursor as the arrows move it, and g is text.
func TestTimelineSearchLinePassesCtrlPage(t *testing.T) {
	const n = 100
	m := pagedTimeline(t, n)
	m, _ = tpress(t, m, runes("/"))
	if !m.timeline.searching {
		t.Fatal("fixture: the search line did not open")
	}
	v := m.timelineHeight()
	m.details.cursor, m.details.rowOffset = v+2, v
	m = m.recordTimelineCursor()

	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyCtrlPgDown})
	if m.details.rowOffset != v+detailsScrollStep {
		t.Errorf("Ctrl+PgDn in the line: offset %d, want %d", m.details.rowOffset, v+detailsScrollStep)
	}
	if !m.timeline.searching {
		t.Error("Ctrl+PgDn closed the search line")
	}
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyCtrlPgUp})
	if m.details.rowOffset != v {
		t.Errorf("Ctrl+PgUp in the line: offset %d, want %d", m.details.rowOffset, v)
	}
	// PgDn pages the cursor: first to the window's last row.
	m, _ = tpress(t, m, keyPgDn)
	if m.details.cursor != m.details.rowOffset+v-1 {
		t.Errorf("PgDn in the line: cursor %d offset %d, want the window's last row %d", m.details.cursor, m.details.rowOffset, m.details.rowOffset+v-1)
	}
	cursorNamesItsRow(t, m, "PgDn in the line")
	before := m.details
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyHome})
	if m.details.cursor != before.cursor || m.details.rowOffset != before.rowOffset || m.timeline.query != "" {
		t.Errorf("Home in the line moved something: cursor %d offset %d query %q", m.details.cursor, m.details.rowOffset, m.timeline.query)
	}
	m, _ = tpress(t, m, runes("g"))
	if m.timeline.query != "g" {
		t.Errorf("g in the line: query %q, want it typed", m.timeline.query)
	}
}

// A2: a timeline that is waiting for data has no cursor to act on: the paging
// keys, new with the cursor paging, are held by the same guard as the arrows.
func TestWaitingTimelineIgnoresPageKeys(t *testing.T) {
	stubTimeline(t, monthRows(t))
	stubTextCompute(t)
	m, _ := enteredFixture(t, 2, true, Config{Accessible: true})
	m, _ = nextMonth(m)
	if !m.detailsTabWaiting(TabDetailsTimeline) || !m.details.attached {
		t.Fatalf("fixture: waiting=%v attached=%v", m.detailsTabWaiting(TabDetailsTimeline), m.details.attached)
	}
	pageKeysDoNothing := func(m Model, when string) {
		t.Helper()
		cursor, offset := m.details.cursor, m.details.rowOffset
		for _, k := range []string{"pgup", "pgdown", "home", "end", "g", "G", "ctrl+pgup", "ctrl+pgdown"} {
			next, cmd := tpress(t, m, keyMsg(k))
			if cmd != nil || next.details.cursor != cursor || next.details.rowOffset != offset || next.timeline.query != "" {
				t.Errorf("%s, %s: cursor %d->%d offset %d->%d query %q cmd %v", when, k, cursor, next.details.cursor, offset, next.details.rowOffset, next.timeline.query, cmd != nil)
			}
		}
	}
	pageKeysDoNothing(m, "a new month's list owed (no rows shown)")

	// A re-search for the SAME inputs in flight keeps its rows in the list while
	// the tab waits: the one state where the guard, not an empty list, is what
	// stops a page key from moving a cursor over rows about to be replaced.
	long := pagedTimeline(t, 100)
	long.details.cursor, long.details.rowOffset = 7, 3
	long.timeline.complete = false
	if !long.detailsRowKeysBlocked() || len(long.timelineShown()) != 100 {
		t.Fatalf("fixture: blocked=%v shown=%d, want a waiting tab that still lists its rows", long.detailsRowKeysBlocked(), len(long.timelineShown()))
	}
	pageKeysDoNothing(long, "a re-search in flight (rows still listed)")
}
