package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// the specification. Every chart value here is a constructed
// synthetic input (specs/frames/residue.md).

// oneChartRows renders the Chart body in the one-chart layout, unstyled.
func oneChartRows(slots *store.Slots, name string, width int, oneChart bool) []string {
	firstAnchor(slots).Chart.Name = name
	body := chartSlotsBodyLayout(slots, 1, false, testFrozenTime(), testPlaceBerlin(), lookup.Hellenistic, width, Config{Accessible: true}, nil, "", oneChart)
	return strings.Split(ansi.Strip(body), "\n")
}

// Acceptance 1-3: in the one-chart layout the moment is never cut, the name
// column is as wide as the longest name up to min(20, what remains), and the
// zone and place follow only while they fit whole.
func TestOneChartRowKeepsItsTime(t *testing.T) {
	defaultClock(t)
	prefixW := ansi.StringWidth(rowPrefix(false, false, true, true, Config{Accessible: true}))
	birth := time.Date(2001, 7, 3, 4, 5, 0, 0, time.UTC)
	slots := twoSlots(t, birth, nameColLongPlace)
	const tagW = len("WhTr")
	for _, width := range []int{50, 64, 78} {
		for _, cells := range []int{15, 17, 20, 24} {
			name := strings.Repeat("N", cells)
			rows := oneChartRows(slots, name, width, true)
			if len(rows) != 2 {
				t.Fatalf("w%d n%d: %d rows, want 2", width, cells, len(rows))
			}
			momentW := ansi.StringWidth(clockMoment(zonedIn(slots.All()[0].Time, nameColLongPlace)))
			capW := min(nameColumnMax, width-prefixW-2-momentW-1-tagW)
			wantN := min(cells, capW)
			for i, row := range rows {
				s := slots.All()[i]
				moment := clockMoment(zonedIn(s.Time, s.Location))
				if w := ansi.StringWidth(row); w != width {
					t.Errorf("w%d n%d row %d: width %d: %q", width, cells, i, w, row)
				}
				if got := cellOf(row, moment); got != prefixW+wantN+2 {
					t.Errorf("w%d n%d row %d: moment %q at cell %d, want %d (name column %d): %q", width, cells, i, moment, got, prefixW+wantN+2, wantN, row)
				}
				if !strings.HasSuffix(row, "WhTr") {
					t.Errorf("w%d n%d row %d: tag not pinned right: %q", width, cells, i, row)
				}
				// Past the name column nothing is cut: no mark anywhere.
				at := strings.Index(row, moment)
				if at < 0 {
					continue // already reported: the moment was cut
				}
				rest := row[at:]
				if strings.Contains(rest, "…") || strings.Contains(rest, "...") {
					t.Errorf("w%d n%d row %d: a cut mark after the name: %q", width, cells, i, row)
				}
				// Zone and place: whole or absent, in that order.
				tail := strings.Join(strings.Fields(strings.TrimSuffix(rest[len(moment):], "WhTr")), " ")
				zone := slotZone(s.Time, s.Location)
				okTails := []string{"", zone, zone + " " + strings.Join(strings.Fields(s.Location.Display()), " ")}
				if tail != okTails[0] && tail != okTails[1] && tail != okTails[2] {
					t.Errorf("w%d n%d row %d: zone/place tail %q is none of %q", width, cells, i, tail, okTails)
				}
			}
		}
	}
	// The widths the spec names: at 50 with a prefix of 4 the name column is 17.
	if got := min(nameColumnMax, 50-4-2-22-1-tagW); got != 17 {
		t.Fatalf("spec arithmetic: %d, want 17", got)
	}
	// 80x24 shows the zone and the place whole when they fit.
	rows := oneChartRows(slots, "Constructed One", 78, true)
	if !strings.Contains(rows[1], "CEST Berlin, DE") {
		t.Errorf("a wide row dropped its zone or place: %q", rows[1])
	}
}

// Acceptance 4: the comparing layout is unchanged — zone kept, only the place
// cut with the existing mark.
func TestComparingRowUnchanged(t *testing.T) {
	defaultClock(t)
	prefixW := ansi.StringWidth(rowPrefix(false, false, true, true, Config{Accessible: true}))
	slots := twoSlots(t, time.Date(2001, 7, 3, 4, 5, 0, 0, time.UTC), nameColLongPlace)
	rows := oneChartRows(slots, "Fixture-Constructed-Longname", 64, false) // N = 20
	for _, row := range rows {
		if w := ansi.StringWidth(row); w != 64 {
			t.Errorf("row width %d, want 64: %q", w, row)
		}
	}
	if got := cellOf(rows[0], "Tue Jul  3, 2001  6:05 CEST "); got != prefixW+nameColumnMax+2 {
		t.Fatalf("moment at cell %d in %q, want the 20-cell name column kept", got, rows[0])
	}
	if !strings.Contains(rows[0], "CEST ") || !strings.Contains(strings.TrimSuffix(rows[0], "WhTr"), "... ") {
		t.Errorf("comparing row lost its zone or the place's cut mark: %q", rows[0])
	}
	// The view passes the layout it is in: two selected charts keep the cut.
	m := timelineModel(t, Config{Color: ColorNever})
	firstAnchor(m.slots).Chart.Name = "Constructed One"
	if m.bodiesView().count() < 2 {
		t.Fatal("fixture: the view is not comparing")
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Constructed One  Tue May 15, 1979 11:30 CEST A") {
		t.Fatalf("the comparing view's chart row lost its zone or its cut place:\n%s", view)
	}
}

// Acceptance 1 through the whole screen: one selected chart, 132x40.
func TestOneChartViewKeepsTheMoment(t *testing.T) {
	defaultClock(t)
	m := timelineModel(t, Config{Color: ColorNever})
	m.slots.SetComparisonSelected(false)
	firstAnchor(m.slots).Chart.Name = strings.Repeat("N", 24)
	m.cursorIdx = len(m.slots.All()) - 1
	if m.bodiesView().count() != 1 {
		t.Fatalf("fixture: %d Bodies columns, want 1", m.bodiesView().count())
	}
	view := ansi.Strip(m.View())
	moment := clockMoment(zonedIn(m.slots.All()[0].Time, testPlaceAmsterdam()))
	if !strings.Contains(view, moment) {
		t.Errorf("the one-chart view cut the anchored moment %q:\n%s", moment, view)
	}
}

// ---- the timeline follows the cursor -------------------------------------

// inMonth is a chart born inside the listed week: 04:00 UTC on 18 October
// 2026, so the first row at or after it is Mars's conjunction at 06:40 CEST.
var inMonth = time.Date(2026, 10, 18, 4, 0, 0, 0, time.UTC)

// nearestFixture is the loaded timeline with a third chart, born in the listed
// month, added after the 1979 natal; the Chart panel keeps the keyboard.
func nearestFixture(t *testing.T) (Model, *[]timelineCall) {
	t.Helper()
	calls := stubTimeline(t, nil)
	m := loadedTimeline(t, Config{Accessible: true})
	if err := addAnchor(m.slots, inMonth, testPlaceAmsterdam()); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}
	// All three are selected, so the scene is comparing wherever the cursor
	// stands and the timeline's inputs never change under it.
	m.slots.SetSlotSelected(1, true)
	m.cursorIdx = len(m.slots.All()) - 1
	m.width = max(m.width, dualColumnMinWidth(m.shownCount()))
	// Choosing the timeline tab with `0` is what owes its search for the new
	// inputs (rule 3 of the specification); its command is
	// delivered too.
	m, zeroCmd := tpress(t, m, runes("0"))
	m, cmd := syncTimelineVisited(m)
	m = pump(t, m, zeroCmd, cmd)
	if !m.timeline.complete || len(m.timelineRows()) != 34 {
		t.Fatalf("fixture: timeline complete=%v rows=%d", m.timeline.complete, len(m.timelineRows()))
	}
	return m, calls
}

func boldLineText(t *testing.T, m Model) []string {
	t.Helper()
	lines := detailsLines(m)
	bold, _ := boldRows(m)
	var out []string
	for _, i := range bold {
		out = append(out, ansi.Strip(lines[i]))
	}
	return out
}

// Acceptance 5-7, as timeline-tab-rules T1 rule 4 restates them: the bold row
// is the FLOATING slot's moment whichever chart the cursor is on, so moving the
// cursor moves nothing in the list, starts no new search and leaves the window
// where it was.
func TestNearestEventStaysOnTheFloatingMoment(t *testing.T) {
	forceColor(t)
	m, calls := nearestFixture(t)
	if len(m.slots.All()) != 3 {
		t.Fatalf("fixture: %d slots", len(m.slots.All()))
	}
	floatNearest := nearestOf(t, m)
	floatBold, _ := boldRows(m)
	if len(floatBold) != 1 || floatBold[0] != floatNearest-m.details.rowOffset {
		t.Fatalf("floating cursor: bold %v, want only the floating nearest %d", floatBold, floatNearest-m.details.rowOffset)
	}
	floatLine := boldLineText(t, m)
	offset := m.details.rowOffset
	searches := len(*calls)

	// ↑ onto the chart born in the month, then ↑ onto the 1979 natal: the bold
	// row is the floating moment's row in the frame of the key and after it
	// settles, and the window does not move.
	for _, chart := range []string{"the chart born in the month", "the 1979 natal"} {
		var cmd tea.Cmd
		m, cmd = tpress(t, m, keyUp)
		if got := boldLineText(t, m); strings.Join(got, "|") != strings.Join(floatLine, "|") {
			t.Fatalf("cursor on %s: bold rows %q, want the floating moment's row %q", chart, got, floatLine)
		}
		m = pump(t, m, cmd)
		if got := boldLineText(t, m); strings.Join(got, "|") != strings.Join(floatLine, "|") {
			t.Fatalf("cursor on %s, settled: bold rows %q, want the floating moment's row %q", chart, got, floatLine)
		}
		if m.details.rowOffset != offset {
			t.Errorf("cursor on %s: window moved %d -> %d", chart, offset, m.details.rowOffset)
		}
	}

	// Back down to the floating slot: exactly as before.
	m, cmd := tpress(t, m, keyDown)
	m, cmd2 := tpress(t, m, keyDown)
	m = pump(t, m, cmd, cmd2)
	if !m.focusedIsFloating() {
		t.Fatal("fixture: the cursor did not return to the floating slot")
	}
	if bold, _ := boldRows(m); len(bold) != 1 || bold[0] != nearestOf(t, m)-m.details.rowOffset {
		t.Errorf("floating slot again: bold rows %v, want its nearest event", bold)
	}
	if len(*calls) != searches {
		t.Errorf("moving the cursor started %d new timeline searches, want none", len(*calls)-searches)
	}
}

// Acceptance 10, restated: a time step with the cursor on an anchored chart
// moves the FLOATING moment (D37), and the bold row follows it: the first row
// at or after the new moment, none when the list ends before it. A list that
// ends before the floating moment has no bold row either.
func TestStepMovesTheFloatingBoldRowFromAnAnchoredCursor(t *testing.T) {
	stubTextCompute(t)
	m, _ := nearestFixture(t)
	want := boldLineText(t, m)
	m, cmd := tpress(t, m, keyUp)
	m = pump(t, m, cmd)
	before := boldLineText(t, m)
	if len(before) != 1 || strings.Join(before, "|") != strings.Join(want, "|") {
		t.Fatalf("cursor on the in-month chart: bold rows %q, want the floating moment's row %q", before, want)
	}
	if m.focusedIsFloating() {
		t.Fatal("fixture: the cursor is on the floating slot")
	}
	moved := false
	for _, k := range []string{"d", "h", "o"} {
		was := m.floatingTime()
		m, cmd = tpress(t, m, runes(k))
		m = pump(t, m, cmd)
		if m.floatingTime().Equal(was) {
			t.Fatalf("step %q from the anchored cursor did not move the floating moment", k)
		}
		if m.focusedIsFloating() {
			t.Fatalf("step %q moved the cursor to the floating slot", k)
		}
		n := nearestTimelineRow(m.timelineRows(), m.floatingTime())
		bold, _ := boldRows(m)
		switch {
		case n < 0 && len(bold) != 0:
			t.Errorf("step %q: bold rows %v, want none (the list ends before the floating moment)", k, bold)
		case n >= 0 && (len(bold) != 1 || bold[0] != n-m.details.rowOffset):
			t.Errorf("step %q: bold rows %v, want the first row at or after the floating moment (%d, offset %d)", k, bold, n, m.details.rowOffset)
		}
		moved = moved || n >= 0
	}
	if !moved {
		t.Error("fixture: no step left a row at or after the floating moment, so the bold row was never followed")
	}
	// A list whose rows all come before the floating moment is outside its
	// range: no row is bold, and the old row is not kept.
	earlier := make([]lookup.TimelineRow, len(m.timeline.rows))
	copy(earlier, m.timeline.rows)
	for i := range earlier {
		earlier[i].Instant = earlier[i].Instant.AddDate(0, -1, 0)
	}
	m.timeline.rows = earlier
	m = m.clampTimelineCursor()
	if bold, _ := boldRows(m); len(bold) != 0 {
		t.Errorf("a list before the floating moment: bold rows %v, want none", bold)
	}
}

// ---- Enter ----------------------------------------------------------------

// Acceptance 8: Enter on a row puts the floating slot at the row's instant and
// the cursor on the floating slot, with ONE recompute; the main panels still
// hold the old scene until it lands.
func TestEnterMovesCursorToFloating(t *testing.T) {
	// Only with the pair selected: an unselected anchored chart has no
	// Timeline tab to press Enter on.
	for _, paired := range []bool{true} {
		text := stubTextCompute(t)
		m := loadedTimeline(t, Config{Accessible: true})
		m.slots.SetComparisonSelected(paired)
		for _, k := range append(append([]tea.KeyMsg{keyUp, runes("0"), keyEnter}, homeKeys()...), runes("j"), runes("j")) {
			var c tea.Cmd
			m, c = tpress(t, m, k)
			m = pump(t, m, c)
		}
		if m.focusedIsFloating() {
			t.Fatal("fixture: ↑ did not focus the anchored slot")
		}
		row := m.timeline.rows[2]
		runs, spawned := m.refresh[surfaceText].runningRunID, len(*text)
		m, cmd := tpress(t, m, keyEnter)
		if !m.focusedIsFloating() {
			t.Errorf("paired=%v: the cursor did not move to the floating slot", paired)
		}
		if !m.holding() {
			t.Errorf("paired=%v: the panels did not keep the old scene while the new one computes", paired)
		}
		m = pump(t, m, cmd)
		if m.holding() {
			t.Errorf("paired=%v: the panels never switched to the new scene", paired)
		}
		if got := m.slots.Floating().Time; !got.Equal(landedAt(row.Instant)) {
			t.Errorf("paired=%v: committed floating Time %v, want %v", paired, got, landedAt(row.Instant))
		}
		if !m.details.attached || m.activeTabID(PanelDetails) != TabDetailsTimeline || m.details.cursor != 2 {
			t.Errorf("paired=%v: the jump left the list: attached=%v cursor=%d", paired, m.details.attached, m.details.cursor)
		}
		if got := m.refresh[surfaceText].runningRunID - runs; got != 1 {
			t.Errorf("paired=%v: %d text recomputes, want exactly 1", paired, got)
		}
		bodies := 0
		for _, c := range (*text)[spawned:] {
			if c.kind == "bodies" {
				bodies++
				if !c.when.Equal(landedAt(row.Instant)) {
					t.Errorf("paired=%v: computed for %v, want %v", paired, c.when, landedAt(row.Instant))
				}
			}
		}
		if bodies != 1 {
			t.Errorf("paired=%v: %d Bodies computes, want 1", paired, bodies)
		}
		if len(m.chart.positions) == 0 || m.chart.positions[0].Longitude != stamp(row.Instant) {
			t.Errorf("paired=%v: Bodies is not the floating chart at the row's instant: %+v", paired, m.chart.positions)
		}
	}
}

// Acceptance 9: the row pressed is the bold row while the floating moment
// equals its instant, even with an earlier row at the same instant; the next
// step returns to the nearest event.
func TestPressedRowStaysBold(t *testing.T) {
	stubTextCompute(t)
	week := weekRows(t)
	const k = 14
	rows := append([]lookup.TimelineRow(nil), week...)
	rows[k+1] = rows[k] // one transit listed once per natal: same instant
	rows[k+1].Line = "TWIN " + rows[k].Line
	stubTimeline(t, func(lookup.TimelineRequest) (lookup.TimelineResult, error) {
		return lookup.TimelineResult{Rows: rows, Commands: []string{"astrolog -dm"}}, nil
	})
	m := onTimeline(t, Config{Accessible: true})
	for m.details.cursor < k+1 {
		m, _ = tpress(t, m, runes("j"))
	}
	if !strings.HasPrefix(m.timeline.rows[k+1].Line, "TWIN ") || !m.timeline.rows[k].Instant.Equal(m.timeline.rows[k+1].Instant) {
		t.Fatalf("fixture: rows %d and %d are not same-instant twins", k, k+1)
	}
	m, cmd := tpress(t, m, keyEnter)
	m = pump(t, m, cmd)
	if !m.focusedIsFloating() || !m.slots.Floating().Time.Equal(landedAt(rows[k+1].Instant)) {
		t.Fatalf("fixture: after Enter floating=%v time=%v", m.focusedIsFloating(), m.slots.Floating().Time)
	}
	m, _ = tpress(t, m, keyEsc) // detach: only the bold row is left to read
	bold, _ := boldRows(m)
	if want := k + 1 - m.details.rowOffset; len(bold) != 1 || bold[0] != want {
		t.Fatalf("after Enter on the second twin: bold rows %v, want only %d (the pressed row)", bold, want)
	}
	// The next step goes back to the first row at or after the new moment.
	m, cmd = tpress(t, m, runes("d"))
	m = pump(t, m, cmd)
	n := nearestOf(t, m)
	bold, _ = boldRows(m)
	if want := n - m.details.rowOffset; len(bold) != 1 || bold[0] != want {
		t.Errorf("after a step: bold rows %v, want the nearest event %d", bold, want)
	}
}

// ---- frames (the specification) ---------------------

// rowsFrameModel is a 132x40 or 80x24 one-chart screen: a 15-cell and a
// 24-cell name, Chart focused, colours off.
func rowsFrameModel(t *testing.T, w, h int, name string) Model {
	t.Helper()
	m := timelineModel(t, Config{Color: ColorNever})
	m.slots.SetComparisonSelected(false)
	firstAnchor(m.slots).Chart.Name = name
	m.width, m.height = w, h
	return m
}

func TestChartRowsFrames(t *testing.T) {
	defaultClock(t)
	for _, c := range []struct {
		name string
		w, h int
	}{{"chart-rows-one-132x40-never", 132, 40}, {"chart-rows-one-80x24-never", 80, 24}} {
		var views []string
		for _, name := range []string{"Constructed One", "Constructedverylongname Withanevenlongersurname"} {
			m := rowsFrameModel(t, c.w, c.h, name)
			if m.bodiesView().count() != 1 {
				t.Fatalf("fixture: %d Bodies columns", m.bodiesView().count())
			}
			v := m.View()
			assertFrameGeometry(t, v, c.w, c.h)
			views = append(views, v)
		}
		checkGolden(t, c.name, strings.Join(views, "\n"))
	}
}

// nearest-follows-cursor: the cursor on a chart born inside the listed month
// (its row bold), then on a natal outside it (no bold).
func TestNearestFollowsCursorFrames(t *testing.T) {
	forceColor(t)
	m, _ := nearestFixture(t)
	m.cfg = Config{Color: ColorNever}
	m, cmd := tpress(t, m, keyUp)
	m = pump(t, m, cmd)
	inside := m.View()
	m, cmd = tpress(t, m, keyUp)
	m = pump(t, m, cmd)
	outside := m.View()
	assertFrameGeometry(t, inside, 156, 40)
	assertFrameGeometry(t, outside, 156, 40)
	checkGolden(t, "nearest-follows-cursor-156x40-never", inside+"\n"+outside)
}

// enter-pressed-row-bold: after Enter on the second of two same-instant rows
// the cursor is on the floating slot and that second row is the bold one.
func TestEnterPressedRowFrame(t *testing.T) {
	stubTextCompute(t)
	week := weekRows(t)
	rows := append([]lookup.TimelineRow(nil), week...)
	rows[15] = rows[14]
	rows[15].Line = "TWIN " + rows[14].Line
	stubTimeline(t, func(lookup.TimelineRequest) (lookup.TimelineResult, error) {
		return lookup.TimelineResult{Rows: rows, Commands: []string{"astrolog -dm"}}, nil
	})
	m := onTimeline(t, Config{Color: ColorNever})
	for m.details.cursor < 15 {
		m, _ = tpress(t, m, runes("j"))
	}
	m, cmd := tpress(t, m, keyEnter)
	m = pump(t, m, cmd)
	view := m.View()
	assertFrameGeometry(t, view, 144, 40)
	checkGolden(t, "enter-pressed-row-bold-144x40-never", view)
}

var _ = location.PlaceInfo{}
