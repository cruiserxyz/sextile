package ui

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// consoleModel is a ready dual-column workspace carrying `lines` argv lines —
// the lane the console shows — rendered in accessible mode so a test
// can read the frame as text.
func consoleModel(t *testing.T, lines int) Model {
	t.Helper()
	m := detailsTestModel(t)
	m.cfg = Config{Accessible: true}
	for i := 0; i < lines; i++ {
		m = m.logCommands([]string{fmt.Sprintf("command %02d", i)})
	}
	return m
}

// snapshotTexts is the console's snapshot as plain strings.
func snapshotTexts(m Model) []string {
	out := make([]string, len(m.console.snapshot))
	for i, l := range m.console.snapshot {
		out[i] = l.text
	}
	return out
}

// logTexts is the whole buffer as plain strings, lanes dropped.
func logTexts(m Model) []string {
	out := make([]string, len(m.logLines))
	for i, l := range m.logLines {
		out[i] = l.text
	}
	return out
}

func openedConsole(t *testing.T, lines int) Model {
	t.Helper()
	m, _ := press(t, consoleModel(t, lines), ",", "l")
	if !m.console.open {
		t.Fatal("the console did not open")
	}
	return m
}

// frameRows splits a rendered frame into rows.
func frameRows(view string) []string { return strings.Split(view, "\n") }

// columnSpan returns the [left, right) column span of every row, ANSI-aware.
//
// ansi.Cut carries the style state sitting exactly AT the right-hand cut
// column, which belongs to the next column rather than to this span, so a
// purely trailing run of zero-width escapes is dropped. Anything that styles a
// visible cell inside the span survives, which is what the comparison is for.
func columnSpan(view string, left, right int) []string {
	rows := frameRows(view)
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = trailingSGR.ReplaceAllString(ansi.Cut(r, left, right), "")
	}
	return out
}

var trailingSGR = regexp.MustCompile(`(?:\x1b\[[0-9;]*m)+$`)

// consoleBodyLines returns the log lines the console actually rendered, with
// borders, padding and any styling stripped.
func consoleBodyLines(t *testing.T, m Model) []string {
	t.Helper()
	r, ok := m.consoleRect()
	if !ok {
		t.Fatal("no console rectangle in this layout")
	}
	box := renderConsole(m.console.snapshot, m.console, m.tabStrips()[PanelLog], r.w, r.h, true, m.cfg)
	rows := frameRows(box)
	if len(rows) < 3 {
		t.Fatalf("console rendered %d rows", len(rows))
	}
	out := make([]string, 0, len(rows)-2)
	for _, row := range rows[1 : len(rows)-1] {
		inner := ansi.Strip(ansi.Cut(row, 1, r.w-1))
		out = append(out, strings.TrimRight(inner, " "))
	}
	return out
}

// C26: `,l` opens the console, remembers the focused panel, and hands
// subsequent keys to the console's own handler.
func TestLeaderLOpensConsole(t *testing.T) {
	m := consoleModel(t, 20)
	m.focused = PanelBodies
	m, _ = press(t, m, ",", "l")
	if !m.console.open {
		t.Fatal("the console is not open")
	}
	if m.console.priorFocus != PanelBodies {
		t.Errorf("priorFocus = %q, want bodies", m.console.priorFocus)
	}
	cursor := m.console.cursor
	m, _ = press(t, m, "up")
	if m.console.cursor != cursor-1 {
		t.Error("keys after the open did not reach the console handler")
	}
}

// C27: the console is a mode, not a panel — nothing about panel focus moves.
func TestConsoleDoesNotChangePanelFocus(t *testing.T) {
	m := consoleModel(t, 20)
	m.focused = PanelAspects
	wantPanels := focusablePanels()

	m, _ = press(t, m, ",", "l")
	if m.focused != PanelAspects {
		t.Errorf("m.focused = %q, want it untouched", m.focused)
	}
	if !reflect.DeepEqual(wantPanels, focusablePanels()) {
		t.Errorf("focusablePanels = %v, want %v", focusablePanels(), wantPanels)
	}
	if m.nextFocus() != PanelChart || m.prevFocus() != PanelBodies {
		t.Errorf("the tab cycle moved: next=%q prev=%q", m.nextFocus(), m.prevFocus())
	}
	// And once it closes, Tab still cycles exactly as it did.
	m, _ = press(t, m, "esc")
	m, _ = press(t, m, "tab")
	if m.focused != PanelChart {
		t.Errorf("after closing, tab put focus on %q, want chart", m.focused)
	}
}

// C28, under the specification: the console's own frame says
// the keyboard is there. There is no status bar; the mark is on the Log title
// and on no other.
func TestConsoleOwnsTheFocusMark(t *testing.T) {
	m := openedConsole(t, 20)
	view := ansi.Strip(m.View())
	if strings.Count(view, "+->") != 1 || !strings.Contains(view, "+->Log") {
		t.Errorf("want exactly one focus mark, on the Log title:\n%s", view)
	}
}

// C29: dual-column, the console takes the whole right column and Details is
// not rendered.
func TestConsoleTakesRightColumn(t *testing.T) {
	m := openedConsole(t, 40)
	r, ok := m.consoleRect()
	if !ok {
		t.Fatal("no console rectangle")
	}
	if r.w != m.width-leftBaseWidth || r.h != m.height-KeyBarHeight {
		t.Errorf("console rect = %+v, want the whole right column", r)
	}
	view := ansi.Strip(m.View())
	if strings.Contains(view, detailsPrefix) {
		t.Error("Details is still rendered under the console")
	}
	right := columnSpan(m.View(), leftBaseWidth, m.width)
	// Rows 0..mainHeight-1 are the console: one box, top border to bottom.
	if !strings.Contains(ansi.Strip(right[0]), "Log") {
		t.Errorf("the right column does not start with the Log box: %q", right[0])
	}
	if got := strings.TrimRight(ansi.Strip(right[m.height-KeyBarHeight-1]), " "); !strings.HasPrefix(got, "+") {
		t.Errorf("the console does not reach the bottom of the main area: %q", got)
	}
}

// C30: narrow, the console takes the whole main area.
func TestConsoleTakesMainAreaWhenNarrow(t *testing.T) {
	m := consoleModel(t, 40)
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = press(t, tm.(Model), ",", "l")

	r, ok := m.consoleRect()
	if !ok {
		t.Fatal("no console rectangle")
	}
	if r.w != 80 || r.h != 24-KeyBarHeight {
		t.Errorf("console rect = %+v, want the whole main area", r)
	}
	view := ansi.Strip(m.View())
	for _, gone := range []string{"[1] Chart", "[2] Bodies", "[3] Aspects"} {
		if strings.Contains(view, gone) {
			t.Errorf("%s is still rendered beside a full-width console", gone)
		}
	}
	if !strings.Contains(view, "Log") {
		t.Error("the console is not rendered")
	}
}

// C31: the panels the operator was reading do not move under the console.
// Opening it changes exactly one thing about them: the console owns the
// keyboard, so the panel that had focus draws unfocused
// (the specification C1). The left column with the console open
// is therefore the closed frame with no panel focused, row for row.
func TestConsoleLeavesLeftColumnAlone(t *testing.T) {
	for _, cfg := range []Config{{Accessible: true}, {Color: ColorAlways}, {Color: ColorNever}} {
		m := consoleModel(t, 40)
		m.cfg = cfg
		unfocused := m
		unfocused.focused = PanelNone
		closed := columnSpan(unfocused.View(), 0, leftBaseWidth)
		opened, _ := press(t, m, ",", "l")
		got := columnSpan(opened.View(), 0, leftBaseWidth)
		if len(closed) != len(got) {
			t.Fatalf("%+v: frame height changed: %d -> %d", cfg, len(closed), len(got))
		}
		// Row len-1 is the key bar, which may change.
		for i := 0; i < len(closed)-1; i++ {
			if closed[i] != got[i] {
				t.Errorf("%+v: left column row %d changed:\n closed: %q\n opened: %q", cfg, i, closed[i], got[i])
			}
		}
	}
}

// requirement C32, rewritten over the snapshot: the view is frozen. A
// line arriving moves neither the cursor's line nor the rendered text — and
// since the console renders the snapshot it took when it opened, the
// indicator's total holds too. Both regimes: a growing buffer, and a full
// buffer where every append also evicts.
func TestConsoleViewIsFrozen(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines int
	}{
		{"growing buffer", 40},
		{"full buffer, every append trims", MaxLogLines},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := openedConsole(t, tc.lines)
			m, _ = press(t, m, "up", "up", "up", "up", "up")

			innerH := m.consoleInnerHeight()
			wantCursorLine := m.console.snapshot[m.console.cursor].text
			wantStart, wantEnd := consoleWindow(len(m.console.snapshot), m.console.offset, innerH)
			wantRows := consoleBodyLines(t, m)
			wantTotal := consoleViewport(len(m.console.snapshot), m.console, innerH).Total

			m = m.logCommands([]string{"a command that arrives while you are reading"})

			if got := m.console.snapshot[m.console.cursor].text; got != wantCursorLine {
				t.Errorf("the cursor is on a different line:\n want %q\n  got %q", wantCursorLine, got)
			}
			if got := consoleBodyLines(t, m); !reflect.DeepEqual(wantRows, got) {
				t.Errorf("rendered rows changed:\n want %q\n  got %q", wantRows, got)
			}
			gotStart, gotEnd := consoleWindow(len(m.console.snapshot), m.console.offset, innerH)
			if gotStart != wantStart || gotEnd != wantEnd {
				t.Errorf("window [%d,%d) -> [%d,%d) moved under a snapshot", wantStart, wantEnd, gotStart, gotEnd)
			}
			if got := consoleViewport(len(m.console.snapshot), m.console, innerH).Total; got != wantTotal {
				t.Errorf("the indicator reads %d, want the snapshot's %d", got, wantTotal)
			}
		})
	}
}

// C33: the console opens on the newest line.
func TestConsoleCursorStartsAtNewest(t *testing.T) {
	m := openedConsole(t, 40)
	if want := len(m.console.snapshot) - 1; m.console.cursor != want {
		t.Errorf("cursor = %d, want %d (the newest line)", m.console.cursor, want)
	}
}

// C34: up and k move towards the oldest, clamped there.
func TestConsoleCursorMovesUp(t *testing.T) {
	for _, k := range []string{"up", "k"} {
		m := openedConsole(t, 40)
		start := m.console.cursor
		m, _ = press(t, m, k)
		if m.console.cursor != start-1 {
			t.Errorf("%s moved the cursor to %d, want %d", k, m.console.cursor, start-1)
		}
		for i := 0; i < 100; i++ {
			m, _ = press(t, m, k)
		}
		if m.console.cursor != 0 {
			t.Errorf("%s did not clamp at the oldest line: %d", k, m.console.cursor)
		}
	}
}

// C35: down and j move towards the newest, clamped there.
func TestConsoleCursorMovesDown(t *testing.T) {
	for _, k := range []string{"down", "j"} {
		m := openedConsole(t, 40)
		m, _ = press(t, m, "up", "up", "up")
		start := m.console.cursor
		m, _ = press(t, m, k)
		if m.console.cursor != start+1 {
			t.Errorf("%s moved the cursor to %d, want %d", k, m.console.cursor, start+1)
		}
		for i := 0; i < 100; i++ {
			m, _ = press(t, m, k)
		}
		if want := len(m.console.snapshot) - 1; m.console.cursor != want {
			t.Errorf("%s did not clamp at the newest line: %d, want %d", k, m.console.cursor, want)
		}
	}
}

// C36: wherever the cursor goes, the window follows it.
func TestConsoleWindowFollowsCursor(t *testing.T) {
	m := openedConsole(t, 120)
	innerH := m.consoleInnerHeight()
	if innerH < 2 {
		t.Fatalf("inner height %d is too small to test windowing", innerH)
	}
	check := func(where string) {
		t.Helper()
		start, end := consoleWindow(len(m.console.snapshot), m.console.offset, innerH)
		if m.console.cursor < start || m.console.cursor >= end {
			t.Errorf("%s: cursor %d outside the rendered window [%d,%d)", where, m.console.cursor, start, end)
		}
	}
	check("at open")
	for i := 0; i < 119; i++ {
		m, _ = press(t, m, "up")
		check("walking up")
	}
	for i := 0; i < 119; i++ {
		m, _ = press(t, m, "down")
		check("walking down")
	}
}

// C36, across a resize: a console with no inner height cannot scroll its
// window onto a moving cursor, so widening is where that gets put right.
func TestConsoleWindowFollowsCursorAcrossResize(t *testing.T) {
	m := openedConsole(t, 120)
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	m = tm.(Model)
	if m.consoleInnerHeight() != 0 {
		t.Fatalf("inner height = %d, want a console with no room at all", m.consoleInnerHeight())
	}
	for i := 0; i < 60; i++ {
		m, _ = press(t, m, "up")
	}
	tm = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
	m = tm.(Model)

	start, end := consoleWindow(len(m.console.snapshot), m.console.offset, m.consoleInnerHeight())
	if m.console.cursor < start || m.console.cursor >= end {
		t.Errorf("cursor %d outside the rendered window [%d,%d) after widening", m.console.cursor, start, end)
	}
}

// C37: the console carries the shared scroll indicator, measuring the whole
// buffer against the window it rendered.
func TestConsoleCarriesScrollIndicator(t *testing.T) {
	m := openedConsole(t, 120)
	innerH := m.consoleInnerHeight()
	v := consoleViewport(len(m.logLines), m.console, innerH)
	start, _ := consoleWindow(len(m.console.snapshot), m.console.offset, innerH)
	if v.Total != len(m.logLines) || v.Visible != innerH || v.Offset != start {
		t.Errorf("viewport = %+v, want Total=%d Visible=%d Offset=%d", v, len(m.logLines), innerH, start)
	}
	if !v.overflows() {
		t.Fatal("a 120-line buffer in this panel must overflow")
	}
	r, _ := m.consoleRect()
	box := renderConsole(m.console.snapshot, m.console, m.tabStrips()[PanelLog], r.w, r.h, true, m.cfg)
	// The thumb replaces the right border glyph on its rows.
	if !strings.Contains(ansi.Strip(box), scrollThumbASCII) {
		t.Error("no thumb on the console's right border")
	}
}

// C38: Esc closes the console and gives focus back.
func TestConsoleEscRestoresFocus(t *testing.T) {
	m := consoleModel(t, 20)
	m.focused = PanelAspects
	m, _ = press(t, m, ",", "l")
	m, _ = press(t, m, "esc")
	if m.console.open {
		t.Error("esc did not close the console")
	}
	if m.focused != PanelAspects {
		t.Errorf("focus came back as %q, want aspects", m.focused)
	}
}

// C39: the console owns the keyboard. Quit is the one main-mode binding that
// still acts.
func TestConsoleOwnsKeyboard(t *testing.T) {
	m := openedConsole(t, 20)
	wantTime := m.targetFloatingTime
	wantFocus := m.focused
	beforeRefresh := m.refresh

	// The paging keys are the console's own now: they move its cursor (A6 of the
	// page-cursor specification) instead of falling through to Details.
	if paged, _ := press(t, m, "pgup"); paged.console.cursor >= m.console.cursor {
		t.Errorf("pgup in the console left the cursor at %d (was %d)", paged.console.cursor, m.console.cursor)
	}
	if paged, _ := press(t, m, "home"); paged.console.cursor != 0 {
		t.Errorf("home in the console left the cursor at %d", paged.console.cursor)
	}

	for _, k := range []string{"?", "tab", "1", "2", "3", " ", "y", "Y", "d", "n", "N", "t", "pgup", "pgdown", "ctrl+l", "ctrl+o", "ctrl+x"} {
		m, _ = press(t, m, k)
	}
	if m.helpOpen {
		t.Error("? opened the help modal from inside the console")
	}
	if m.startup != startupReady {
		t.Errorf("a modal opened from inside the console: startup = %q", m.startup)
	}
	if m.focused != wantFocus {
		t.Errorf("focus moved to %q", m.focused)
	}
	if !m.targetFloatingTime.Equal(wantTime) {
		t.Error("a time-step key acted from inside the console")
	}
	if !reflect.DeepEqual(beforeRefresh, m.refresh) {
		t.Error("a swallowed key reached a refresh surface")
	}
	if !m.console.open {
		t.Error("a swallowed key closed the console")
	}

	// q closes the console as Esc does and never quits from here (C11), and
	// Ctrl+C quits from here as from everywhere (C10).
	if after, cmd := press(t, m, "q"); cmd != nil || after.console.open {
		t.Fatal("q did not just close the console")
	}
	_, cmd := press(t, m, "ctrl+c")
	if cmd == nil {
		t.Fatal("ctrl+c did not quit from inside the console")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c returned %T, want a quit", cmd())
	}
}

// C40: opening and closing the console starts nothing.
func TestConsoleStartsNoCompute(t *testing.T) {
	m := consoleModel(t, 20)
	before := m.refresh

	m, cmd := press(t, m, ",", "l")
	if !reflect.DeepEqual(before, m.refresh) {
		t.Error("opening touched a refresh surface")
	}
	if cmd != nil {
		t.Error("opening returned a command")
	}
	m, cmd = press(t, m, "esc")
	if !reflect.DeepEqual(before, m.refresh) {
		t.Error("closing touched a refresh surface")
	}
	if cmd != nil {
		t.Error("closing returned a command")
	}
}

// C41: a compute that finishes while the console is open still commits, and
// the left column shows it.
func TestConsoleDoesNotBlockCommits(t *testing.T) {
	m := consoleModel(t, 20)
	m, _ = press(t, m, "d") // starts a text run for the next day
	st := m.refresh[surfaceText]
	if st.runningRunID == 0 {
		t.Fatal("no text run is in flight to complete")
	}
	m, _ = press(t, m, ",", "l")
	if !m.console.open {
		t.Fatal("the console did not open")
	}

	var tm tea.Model = m
	tm, _ = tm.Update(FirstFrameReadyMsg{
		RunID:       st.runningRunID,
		WantID:      st.wantID,
		Positions:   []aspect.Position{{Body: aspect.Sun}, {Body: aspect.Moon}},
		TransitTime: m.targetFloatingTime,
	})
	m = tm.(Model)
	if len(m.chart.positions) != 2 {
		t.Fatalf("the frame did not commit: %d positions", len(m.chart.positions))
	}
	left := strings.Join(columnSpan(m.View(), 0, leftBaseWidth), "\n")
	if !strings.Contains(ansi.Strip(left), "Sun") {
		t.Error("the left column does not render the committed frame")
	}
}

// C42: console state lives on the Model and is zeroed on close.
func TestConsoleStateIsEphemeral(t *testing.T) {
	m := openedConsole(t, 40)
	m, _ = press(t, m, "up", "up")
	if reflect.DeepEqual(m.console, consoleState{}) {
		t.Fatal("the open console has no state to lose")
	}
	m, _ = press(t, m, "esc")
	if !reflect.DeepEqual(m.console, consoleState{}) {
		t.Errorf("closing left %+v behind", m.console)
	}
}

// C52: narrowing under the console keeps it, as the full main area.
func TestConsoleSurvivesNarrowing(t *testing.T) {
	m := openedConsole(t, 40)
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = tm.(Model)
	if !m.console.open {
		t.Fatal("narrowing closed the console")
	}
	r, ok := m.consoleRect()
	if !ok || r.w != 80 {
		t.Errorf("console rect = %+v ok=%v, want the whole 80-wide main area", r, ok)
	}
	if strings.Contains(ansi.Strip(m.View()), "[1] Chart") {
		t.Error("the left column is still rendered beside a full-width console")
	}
}

// C53: below the minimum the frame is the too-small notice, and the console
// state survives so widening returns to it.
func TestConsoleSurvivesTooSmall(t *testing.T) {
	m := openedConsole(t, 40)
	cursor := m.console.cursor

	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	m = tm.(Model)
	if !strings.Contains(ansi.Strip(m.View()), "Terminal too small") {
		t.Error("the too-small notice did not render")
	}
	if !m.console.open || m.console.cursor != cursor {
		t.Errorf("console state did not survive: %+v", m.console)
	}
	if _, ok := m.consoleRect(); ok {
		t.Error("a console rectangle was reported below the minimum")
	}

	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
	m = tm.(Model)
	if !m.console.open || m.console.cursor != cursor {
		t.Errorf("widening did not return to the console: %+v", m.console)
	}
	if start, end := consoleWindow(len(m.console.snapshot), m.console.offset, m.consoleInnerHeight()); m.console.cursor < start || m.console.cursor >= end {
		t.Errorf("widening left the cursor %d outside the window [%d,%d)", m.console.cursor, start, end)
	}
	if strings.Contains(ansi.Strip(m.View()), detailsPrefix) {
		t.Error("Details came back under the open console")
	}
}

// C54: an empty buffer opens a console with nothing in it, and Enter has
// nothing to copy.
func TestConsoleEmptyBuffer(t *testing.T) {
	m := consoleModel(t, 0)
	if len(m.logLines) != 0 {
		t.Fatalf("this workspace already logged %v", m.logLines)
	}
	m, _ = press(t, m, ",", "l")
	if !m.console.open {
		t.Fatal("the console did not open over an empty buffer")
	}
	innerH := m.consoleInnerHeight()
	if v := consoleViewport(0, m.console, innerH); v.overflows() {
		t.Error("an empty buffer drew an indicator")
	}
	// It says so, once, and nothing else (the specification A6).
	for i, row := range consoleBodyLines(t, m) {
		if want := map[bool]string{true: consoleEmptyLine, false: ""}[i == 0]; row != want {
			t.Errorf("empty console row %d = %q, want %q", i, row, want)
		}
	}

	calls, restore := stubYank(t, nil)
	defer restore()
	m, cmd := press(t, m, "enter")
	if *calls != 0 {
		t.Error("enter over an empty buffer reached the clipboard")
	}
	if cmd == nil || m.footerIndicator() != noticeNothingCopy {
		t.Errorf("enter over an empty buffer showed %q, want the nothing-to-copy notice", m.footerIndicator())
	}
}

// requirement C55, rewritten over the snapshot: a buffer trimmed under
// an open console cannot reach the cursor, because the console is looking at
// the list it took when it opened. The trim shows up on the next open, where
// the cursor lands on the newest line of the trimmed list.
func TestConsoleCursorSurvivesTrim(t *testing.T) {
	m := consoleModel(t, MaxLogLines)
	m, _ = press(t, m, ",", "l")
	if m.console.cursor != MaxLogLines-1 {
		t.Fatalf("cursor = %d, want the newest of a full buffer", m.console.cursor)
	}
	m, _ = press(t, m, "up", "up", "up")
	wantLine := m.console.snapshot[m.console.cursor].text
	wantSnapshot := snapshotTexts(m)
	for i := 0; i < 10; i++ {
		m = m.logCommands([]string{fmt.Sprintf("overflow command %d", i)})
	}
	if len(m.logLines) != MaxLogLines {
		t.Fatalf("buffer = %d lines, want it trimmed to %d", len(m.logLines), MaxLogLines)
	}
	if got := m.console.snapshot[m.console.cursor].text; got != wantLine {
		t.Errorf("the cursor slid onto another line:\n want %q\n  got %q", wantLine, got)
	}
	if got := snapshotTexts(m); !reflect.DeepEqual(wantSnapshot, got) {
		t.Error("the trim reached into the open console's snapshot")
	}
	// Close and reopen: the fresh snapshot is the trimmed buffer, newest last.
	m, _ = press(t, m, "esc", ",", "l")
	if got := len(m.console.snapshot); got != MaxLogLines {
		t.Fatalf("reopened over %d lines, want the trimmed %d", got, MaxLogLines)
	}
	if got := m.console.snapshot[m.console.cursor].text; got != "overflow command 9" {
		t.Errorf("reopened cursor on %q, want the newest line", got)
	}
	if got := m.console.snapshot[0].text; got != "command 10" {
		t.Errorf("oldest surviving line = %q, want the ten oldest evicted", got)
	}
}

// C58: a rectangle with no room renders nothing, as the other panels do.
func TestConsoleDegenerateGeometry(t *testing.T) {
	m := openedConsole(t, 20)
	strip := m.tabStrips()[PanelLog]
	for _, tc := range []struct{ w, h int }{{0, 20}, {20, 0}, {2, 20}, {20, 2}, {-5, 10}, {10, -5}} {
		if got := renderConsole(m.console.snapshot, m.console, strip, tc.w, tc.h, true, m.cfg); got != "" {
			t.Errorf("renderConsole(%d,%d) = %q, want empty", tc.w, tc.h, got)
		}
	}
}

// U3 3: a notice clears on its own tick after noticeDuration, and an older
// notice's tick does not clear a newer one.
func TestNoticeClearsOnItsOwnTick(t *testing.T) {
	if noticeDuration != 3*time.Second {
		t.Errorf("noticeDuration = %v, want 3s", noticeDuration)
	}
	m := consoleModel(t, 3)
	m, cmd := m.showNotice(noticeNothingCopy)
	if cmd == nil {
		t.Fatal("no tick was armed")
	}
	if got := m.footerIndicator(); got != noticeNothingCopy {
		t.Fatalf("footer = %q, want the notice", got)
	}
	older := m.yank.gen
	m, _ = m.showNotice(noticeCopyFailed)

	tm, _ := m.Update(yankFlashMsg{gen: older})
	if got := tm.(Model).footerIndicator(); got != noticeCopyFailed {
		t.Errorf("an older tick changed the footer to %q", got)
	}
	tm, _ = tm.(Model).Update(yankFlashMsg{gen: m.yank.gen})
	if got := tm.(Model).footerIndicator(); got != "" {
		t.Errorf("its own tick left the footer at %q", got)
	}
}

// U3 5: the copy confirmation reads Copied, after Enter in the console.
func TestConsoleCopyReadsCopied(t *testing.T) {
	_, restore := stubYank(t, nil)
	defer restore()
	m := openedConsole(t, 5)
	m, _ = press(t, m, "enter")
	if got := m.footerIndicator(); got != "Copied" {
		t.Errorf("footer = %q, want Copied", got)
	}
}

// U3 6: Enter in an empty console has nothing to copy and says so.
func TestEmptyConsoleEnterSaysNothingToCopy(t *testing.T) {
	_, restore := stubYank(t, nil)
	defer restore()
	m := openedConsole(t, 0)
	m.console.snapshot = nil
	m, _ = press(t, m, "enter")
	if got := m.footerIndicator(); got != "Nothing to copy" {
		t.Errorf("footer = %q, want the notice", got)
	}
}
