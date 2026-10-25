package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/lookup"
)

// the specification.

// detailsLines is the Details panel's inner rows with their escapes kept.
func detailsLines(m Model) []string {
	lines := strings.Split(renderDetails(m), "\n")
	return lines[1 : len(lines)-1]
}

// boldRows is the indexes of the inner Details rows drawn bold, and
// reverseRows those drawn in reverse video.
func boldRows(m Model) (bold, reverse []int) {
	for i, l := range detailsLines(m) {
		if strings.Contains(l, sgrBold) {
			bold = append(bold, i)
		}
		if strings.Contains(l, sgrReverse) {
			reverse = append(reverse, i)
		}
	}
	return bold, reverse
}

func nearestOf(t *testing.T, m Model) int {
	t.Helper()
	n := nearestTimelineRow(m.timelineRows(), m.floatingTime())
	if n < 0 {
		t.Fatal("fixture: no nearest event")
	}
	return n
}

// Rule 2: the first row at or after the floating time; never one before it;
// none when the month is past.
func TestNearestEventIsTheFirstRowAtOrAfterTheFloatingTime(t *testing.T) {
	at := func(h int) lookup.TimelineRow {
		return lookup.TimelineRow{Instant: time.Date(2026, 10, 17, h, 0, 0, 0, time.UTC)}
	}
	rows := []lookup.TimelineRow{at(8), at(10), at(10), at(12)}
	for _, c := range []struct {
		hour, want int
	}{{7, 0}, {9, 1}, {10, 1}, {11, 3}, {12, 3}, {13, -1}} {
		if got := nearestTimelineRow(rows, time.Date(2026, 10, 17, c.hour, 0, 0, 0, time.UTC)); got != c.want {
			t.Errorf("floating at %d:00: nearest = %d, want %d", c.hour, got, c.want)
		}
	}
	if got := nearestTimelineRow(nil, time.Now()); got != -1 {
		t.Errorf("no rows: nearest = %d, want -1", got)
	}
}

// Rules 2 and 3: with Details not attached, the nearest event is the one bold
// row, it sits in the middle row of the window, and nothing is reverse video
// or banded.
func TestDetachedTimelineCentresAndBoldsTheNearestEvent(t *testing.T) {
	forceColor(t)
	for _, cfg := range []Config{{Color: ColorAlways}, {Color: ColorNever}} {
		m := loadedTimeline(t, cfg)
		m, _ = tpress(t, m, runes("0"))
		if m.details.attached || m.activeTabID(PanelDetails) != TabDetailsTimeline {
			t.Fatalf("fixture: attached=%v tab=%s", m.details.attached, m.activeTabID(PanelDetails))
		}
		n := nearestOf(t, m)
		bold, reverse := boldRows(m)
		visible := m.detailsInnerHeight()
		if want := n - m.details.rowOffset; len(bold) != 1 || bold[0] != want {
			t.Errorf("%v: bold rows %v, want only the nearest event's row %d", cfg.Color, bold, want)
		}
		marker := false
		for _, l := range detailsLines(m) {
			plain := strings.TrimPrefix(strings.TrimPrefix(ansi.Strip(l), "│"), " ")
			marker = marker || strings.HasPrefix(plain, rowCursorUnicode) || strings.HasPrefix(plain, rowCursorASCII)
		}
		if len(reverse) != 0 || marker {
			t.Errorf("%v: an unattached timeline drew reverse video or a cursor marker", cfg.Color)
		}
		if got := n - m.details.rowOffset; got != visible/2 && m.details.rowOffset != 0 && m.details.rowOffset != len(m.timeline.rows)-visible {
			t.Errorf("%v: nearest event is on window row %d of %d, want the middle", cfg.Color, got, visible)
		}
	}
}

// Rule 3, the follow: a committed change of the floating time moves the
// window with it.
func TestDetachedTimelineFollowsTheFloatingTime(t *testing.T) {
	m := loadedTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, runes("0"))
	before := nearestOf(t, m)
	later := tlFloatingTime.Add(30 * time.Hour)
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{TransitTime: later})
	m.slots.SetFloatingTime(later)
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{TransitTime: later})
	after := nearestOf(t, m)
	if after <= before {
		t.Fatalf("fixture: nearest %d -> %d, want it to move on", before, after)
	}
	bold, _ := boldRows(m)
	if want := after - m.details.rowOffset; len(bold) != 1 || bold[0] != want {
		t.Fatalf("bold rows %v after the floating time moved, want the new nearest event at %d", bold, want)
	}
}

// Rule 4: entering Details puts the cursor on the nearest event.
func TestEnteringDetailsPutsTheCursorOnTheNearestEvent(t *testing.T) {
	m := loadedTimeline(t, Config{Accessible: true})
	m, _ = tpress(t, m, runes("0"))
	m, _ = tpress(t, m, keyEnter)
	if !m.details.attached {
		t.Fatal("fixture: Details did not attach")
	}
	if n := nearestOf(t, m); m.details.cursor != n {
		t.Fatalf("cursor %d after Enter, want the nearest event %d", m.details.cursor, n)
	}
}

// Rule 5: while attached, a step inside the month keeps the cursor on its
// event, and the nearest event moves on, so two rows are bold.
func TestAttachedTimeStepKeepsTheCursorOnItsEvent(t *testing.T) {
	stubTextCompute(t)
	m := onTimeline(t, Config{Accessible: true})
	for i := 0; i < 4; i++ {
		m, _ = tpress(t, m, runes("j"))
	}
	row := m.timeline.rows[m.details.cursor]
	m, cmd := tpress(t, m, runes("d"))
	m = pump(t, m, cmd)
	if got := m.timeline.rows[m.details.cursor]; markOf(got) != markOf(row) {
		t.Fatalf("after a day step the cursor is on %q, want it still on %q", got.Line, row.Line)
	}
	if n := nearestOf(t, m); n == m.details.cursor {
		t.Fatalf("fixture: nearest event %d did not move off the cursor", n)
	}
	if bold, _ := boldRows(m); len(bold) != 2 {
		t.Fatalf("bold rows %v, want the cursor row and the nearest event", bold)
	}
}

// Rule 5, the other half: a month step loads a list that does not hold the
// cursor's event, so the cursor goes to the nearest event, centred.
func TestAttachedMonthStepMovesTheCursorToTheNearestEvent(t *testing.T) {
	stubTextCompute(t)
	week := weekRows(t)
	stubTimeline(t, func(req lookup.TimelineRequest) (lookup.TimelineResult, error) {
		rows := append([]lookup.TimelineRow(nil), week...)
		if req.When.Month() != time.October {
			for i := range rows {
				rows[i].Instant = rows[i].Instant.AddDate(0, 1, 0)
			}
		}
		return lookup.TimelineResult{Rows: rows, Commands: []string{"astrolog -dm"}}, nil
	})
	m := onTimeline(t, Config{Accessible: true})
	for i := 0; i < 4; i++ {
		m, _ = tpress(t, m, runes("j"))
	}
	m, cmd := tpress(t, m, runes("o"))
	m = pump(t, m, cmd)
	if m.slots.Floating().Time.Month() != time.November || !m.timeline.complete {
		t.Fatalf("fixture: floating %v, complete=%v", m.slots.Floating().Time, m.timeline.complete)
	}
	n := nearestOf(t, m)
	if m.details.cursor != n {
		t.Fatalf("cursor %d after a month step, want the nearest event %d", m.details.cursor, n)
	}
	if want := centreOffset(n, len(m.timeline.rows), m.detailsInnerHeight()); m.details.rowOffset != want {
		t.Fatalf("offset %d, want %d (nearest event centred)", m.details.rowOffset, want)
	}
}

// Rule 1: Details attached, the cursor row carries the band and the nearest
// event is bold only; with colours off, the cursor row has the marker and
// reverse video and the nearest event neither.
func TestAttachedTimelineDrawsCursorAndNearestApart(t *testing.T) {
	forceColor(t)
	for _, cfg := range []Config{{Color: ColorAlways}, {Color: ColorNever}} {
		m := onTimeline(t, cfg)
		for i := 0; i < 3; i++ {
			m, _ = tpress(t, m, runes("j"))
		}
		n := nearestOf(t, m)
		if n == m.details.cursor {
			t.Fatal("fixture: cursor and nearest event on one row")
		}
		lines := detailsLines(m)
		cur, near := lines[m.details.cursor-m.details.rowOffset], lines[n-m.details.rowOffset]
		if cfg.Color == ColorNever {
			if !strings.Contains(cur, cursorRowOff) || strings.Contains(near, sgrReverse) || !strings.Contains(near, sgrBold) {
				t.Errorf("colours off: cursor %q nearest %q", cur, near)
			}
			if !strings.Contains(ansi.Strip(cur), rowCursorUnicode) && !strings.Contains(ansi.Strip(cur), rowCursorASCII) {
				t.Errorf("colours off: cursor row lost its marker: %q", ansi.Strip(cur))
			}
		} else if !strings.Contains(cur, cursorBandStart(cfg)) || strings.Contains(near, cursorBandStart(cfg)) || !strings.Contains(near, sgrBold) {
			t.Errorf("colours on: cursor %q nearest %q", cur, near)
		}
	}
}

// Rule 1 on the other panels: Chart's slot cursor and Bodies' body cursor stay
// bold when another panel has the keyboard, with no band.
func TestUnfocusedPanelsKeepTheirCursorRowBold(t *testing.T) {
	forceColor(t)
	cfg := Config{Color: ColorAlways}
	m := loadedTimeline(t, cfg)
	m, _ = tpress(t, m, runes("0")) // Details shown; Chart still focused
	m, _ = tpress(t, m, keyEnter)   // Details attached: no ring panel has the keyboard
	slots := chartSlotsBody(m.slots, m.cursorIdx, false, testFrozenTime(), m.place, m.lens, 60, cfg)
	if lines := strings.Split(slots, "\n"); !strings.HasPrefix(lines[m.cursorIdx], sgrBold) || strings.Contains(slots, cursorBandStart(cfg)) {
		t.Fatalf("an unfocused Chart must bold its cursor row and draw no band: %q", slots)
	}
}

// Rule 6: the console, help and timeline share the popups' margin helper.
func TestScrollToShowKeepsTheMarginEverywhere(t *testing.T) {
	// Walking down a 40-line list through a 12-line window keeps three lines of
	// context below the cursor until the end.
	off := 0
	for line := 0; line < 40; line++ {
		off = scrollToShow(off, line, 40, 12)
		if line-off > 12-1-listScrollMargin && off+12 < 40 {
			t.Fatalf("line %d at window row %d: less than %d rows of context below", line, line-off, listScrollMargin)
		}
	}
	// Walking back up keeps three above.
	for line := 39; line >= 0; line-- {
		off = scrollToShow(off, line, 40, 12)
		if line-off < listScrollMargin && off > 0 {
			t.Fatalf("line %d at window row %d: less than %d rows of context above", line, line-off, listScrollMargin)
		}
	}
	// A window too short for both margins centres.
	if got := scrollToShow(0, 20, 40, 4); got != 18 {
		t.Fatalf("short window offset = %d, want 18 (centred)", got)
	}
}

// The three new frames at 132x40 (the specification).
func TestFocusAndNearestFrames(t *testing.T) {
	forceColor(t)
	for _, mode := range []struct {
		name string
		cfg  Config
	}{{"color", Config{Color: ColorAlways}}, {"never", Config{Color: ColorNever}}} {
		u := loadedTimeline(t, mode.cfg)
		u, _ = tpress(t, u, runes("0"))
		uview := u.View()
		assertFrameGeometry(t, uview, u.width, 40)
		checkGolden(t, fmt.Sprintf("details-nearest-unfocused-%dx40-", u.width)+mode.name, uview)
		m := onTimeline(t, mode.cfg)
		for i := 0; i < 3; i++ {
			m, _ = tpress(t, m, runes("j"))
		}
		view := m.View()
		assertFrameGeometry(t, view, m.width, 40)
		checkGolden(t, fmt.Sprintf("details-nearest-attached-%dx40-", m.width)+mode.name, view)
	}
}
