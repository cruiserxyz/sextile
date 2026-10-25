package ui

import (
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// influenceReport is the frozen an earlier change chart report: 52 real Astrolog lines
// against a 30-row panel, so the clipping these tests exercise is the one the
// operator actually hits.
func influenceReport(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/influence-chart.report")
	if err != nil {
		t.Fatalf("read influence fixture: %v", err)
	}
	return string(raw)
}

// scrollModel is a ready dual-column workspace showing the influence report,
// scrolled to `offset`.
func scrollModel(t *testing.T, offset int) Model {
	t.Helper()
	m := detailsTestModel(t)
	// 39 rows give Details the 30-line body the spec's worked examples are
	// written against, now that no row goes to a status bar.
	m.height = 39
	m.details = detailsViewState{title: "[0] Chart", body: influenceReport(t), scrollOffset: offset}
	return m
}

func pressScroll(t *testing.T, m Model, k tea.KeyType) (Model, tea.Cmd) {
	t.Helper()
	var tm tea.Model = m
	tm, cmd := tm.Update(tea.KeyMsg{Type: k})
	return tm.(Model), cmd
}

func wheel(t *testing.T, m Model, button tea.MouseButton, x, y int) (Model, tea.Cmd) {
	t.Helper()
	var tm tea.Model = m
	tm, cmd := tm.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: button, X: x, Y: y})
	return tm.(Model), cmd
}

// detailsBodyLines returns the report lines the panel actually rendered,
// borders and padding stripped.
func detailsBodyLines(t *testing.T, m Model) []string {
	t.Helper()
	r, ok := m.geometry()[PanelDetails]
	if !ok {
		t.Fatal("no Details panel in the layout")
	}
	box := renderDetailsPanel(r.w, r.h, (detailsFrameOf(m.details)).state.attached, Config{Accessible: true}, detailsFrameOf(m.details))
	rows := strings.Split(box, "\n")
	out := make([]string, 0, len(rows))
	// Strip the two border cells positionally: the right one is the thumb
	// glyph on a thumb row, not `|`.
	for _, row := range rows[1 : len(rows)-1] {
		cells := []rune(row)
		out = append(out, strings.TrimRight(string(cells[1:len(cells)-1]), " "))
	}
	return out
}

// C9 — the panel renders from its scroll offset down, at most height-2 lines.
func TestDetailsRendersFromOffset(t *testing.T) {
	report := influenceReport(t)
	lines := detailsReportLines(report)
	if len(lines) != 52 {
		t.Fatalf("fixture is %d lines, the spec's worked examples assume 52", len(lines))
	}
	for _, offset := range []int{0, 11, 22} {
		m := scrollModel(t, offset)
		got := detailsBodyLines(t, m)
		if len(got) != 30 {
			t.Fatalf("offset %d: panel rendered %d rows, want 30", offset, len(got))
		}
		for i, line := range got {
			want := strings.TrimRight(lines[offset+i], " ")
			if !strings.HasPrefix(want, uncut(got[i])) && line != want {
				t.Errorf("offset %d row %d = %q, want report line %d %q", offset, i, line, offset+i, want)
			}
		}
	}
	// The tail is reachable: the last page ends on the report's last line.
	m := scrollModel(t, 22)
	got := detailsBodyLines(t, m)
	if last := got[len(got)-1]; !strings.HasPrefix(strings.TrimRight(lines[51], " "), last) {
		t.Errorf("last rendered row = %q, want the report's final line %q", last, lines[51])
	}
	if !strings.Contains(strings.Join(got, "\n"), "12th:") {
		t.Error("the house block an earlier change clipped away is still unreachable")
	}
}

// C10 / C11 — Ctrl+PgDn advances two lines and Ctrl+PgUp retreats two, each
// clamped to its end of the report. (They were PgDn and PgUp until the plain
// keys went to the focused list's cursor.)
func TestDetailsScrollsOnCtrlPage(t *testing.T) {
	m := scrollModel(t, 0)
	for want := detailsScrollStep; want <= 3*detailsScrollStep; want += detailsScrollStep {
		var cmd tea.Cmd
		m, cmd = pressScroll(t, m, tea.KeyCtrlPgDown)
		if cmd != nil {
			t.Error("Ctrl+PgDn returned a Cmd")
		}
		if m.details.scrollOffset != want {
			t.Fatalf("offset = %d, want %d", m.details.scrollOffset, want)
		}
	}
	// Clamped at Total-Visible (52-30) however long the key is held.
	for i := 0; i < 40; i++ {
		m, _ = pressScroll(t, m, tea.KeyCtrlPgDown)
	}
	if m.details.scrollOffset != 22 {
		t.Errorf("offset after holding Ctrl+PgDn = %d, want the floor 22", m.details.scrollOffset)
	}
	m, cmd := pressScroll(t, m, tea.KeyCtrlPgUp)
	if cmd != nil {
		t.Error("Ctrl+PgUp returned a Cmd")
	}
	if m.details.scrollOffset != 20 {
		t.Errorf("offset = %d, want 20", m.details.scrollOffset)
	}
	for i := 0; i < 40; i++ {
		m, _ = pressScroll(t, m, tea.KeyCtrlPgUp)
	}
	if m.details.scrollOffset != 0 {
		t.Errorf("offset after holding Ctrl+PgUp = %d, want 0", m.details.scrollOffset)
	}
}

// C12 / C13 — a wheel notch inside the Details rectangle scrolls it by the
// same step, clamped the same way.
func TestWheelDownScrollsDetails(t *testing.T) {
	m := scrollModel(t, 0)
	m, cmd := wheel(t, m, tea.MouseButtonWheelDown, 100, 10)
	if cmd != nil {
		t.Error("wheel returned a Cmd")
	}
	if m.details.scrollOffset != detailsScrollStep {
		t.Errorf("offset = %d, want %d", m.details.scrollOffset, detailsScrollStep)
	}
	for i := 0; i < 40; i++ {
		m, _ = wheel(t, m, tea.MouseButtonWheelDown, 100, 10)
	}
	if m.details.scrollOffset != 22 {
		t.Errorf("offset after spinning the wheel = %d, want the floor 22", m.details.scrollOffset)
	}
}

func TestWheelUpScrollsDetails(t *testing.T) {
	m := scrollModel(t, 10)
	m, _ = wheel(t, m, tea.MouseButtonWheelUp, 100, 10)
	if m.details.scrollOffset != 10-detailsScrollStep {
		t.Errorf("offset = %d, want %d", m.details.scrollOffset, 10-detailsScrollStep)
	}
	for i := 0; i < 40; i++ {
		m, _ = wheel(t, m, tea.MouseButtonWheelUp, 100, 10)
	}
	if m.details.scrollOffset != 0 {
		t.Errorf("offset after spinning the wheel up = %d, want 0", m.details.scrollOffset)
	}
}

// C14 — pointing is not choosing: a wheel event never moves focus, over
// Details or over any focusable panel.
func TestWheelNeverChangesFocus(t *testing.T) {
	for _, pos := range []struct {
		name string
		x, y int
	}{{"details", 100, 10}, {"chart", 10, 1}, {"bodies", 10, 10}, {"aspects", 10, 34}} {
		for _, button := range []tea.MouseButton{tea.MouseButtonWheelUp, tea.MouseButtonWheelDown} {
			m := scrollModel(t, 4)
			m.focused = PanelChart
			m, _ = wheel(t, m, button, pos.x, pos.y)
			if m.focused != PanelChart {
				t.Errorf("%s wheel moved focus to %v", pos.name, m.focused)
			}
		}
	}
}

// C15 — a wheel event outside the Details rectangle changes nothing at all.
func TestWheelElsewhereIsInert(t *testing.T) {
	for _, pos := range []struct {
		name string
		x, y int
	}{{"chart", 10, 1}, {"bodies", 10, 10}, {"aspects", 10, 34}, {"log", 100, 34}, {"keybar", 10, 39}} {
		for _, button := range []tea.MouseButton{tea.MouseButtonWheelUp, tea.MouseButtonWheelDown} {
			base := scrollModel(t, 4)
			m, cmd := wheel(t, base, button, pos.x, pos.y)
			if cmd != nil {
				t.Errorf("%s wheel returned a Cmd", pos.name)
			}
			if !reflect.DeepEqual(m, base) {
				t.Errorf("%s wheel changed the model (offset %d -> %d)", pos.name, base.details.scrollOffset, m.details.scrollOffset)
			}
		}
	}
}

// C16 — a completed report under the same title is the same context
// refreshed: the operator keeps their place, re-clamped to the new text.
func TestOffsetSurvivesSameContextRefresh(t *testing.T) {
	m := scrollModel(t, 18)
	report := influenceReport(t)
	m, _ = m.commitRefresh(surfaceDetails, refreshResultMsg{
		surface: surfaceDetails,
		payload: detailsPayload{title: "[0] Chart", body: report},
	})
	if m.details.scrollOffset != 18 {
		t.Errorf("offset = %d, want the reader's place 18", m.details.scrollOffset)
	}
	// A shorter report under the same title re-clamps instead of hanging off
	// the end: 34 lines in a 30-row panel can only be scrolled by 4.
	short := strings.Join(detailsReportLines(report)[:34], "\n")
	m, _ = m.commitRefresh(surfaceDetails, refreshResultMsg{
		surface: surfaceDetails,
		payload: detailsPayload{title: "[0] Chart", body: short},
	})
	if m.details.scrollOffset != 4 {
		t.Errorf("offset after a shorter report = %d, want the re-clamped 4", m.details.scrollOffset)
	}
}

// C17 — a different title is a different report, read from the top.
func TestOffsetResetsOnNewContext(t *testing.T) {
	m := scrollModel(t, 18)
	m, _ = m.commitRefresh(surfaceDetails, refreshResultMsg{
		surface: surfaceDetails,
		payload: detailsPayload{title: "[0] Moon", body: influenceReport(t)},
	})
	if m.details.scrollOffset != 0 {
		t.Errorf("offset = %d, want 0 for a new report", m.details.scrollOffset)
	}
}

// C18 — a resize re-clamps the rendered offset with no keypress.
func TestOffsetReclampsOnResize(t *testing.T) {
	m := scrollModel(t, 22)
	lines := detailsReportLines(m.details.body)

	// Grow the panel past the report: the window can only start at the top.
	tall := renderDetailsPanel(66, len(lines)+8, (detailsFrameOf(m.details)).state.attached, Config{Accessible: true}, detailsFrameOf(m.details))
	first := strings.Trim(strings.Split(tall, "\n")[1], "|")
	if !strings.HasPrefix(lines[0], uncut(strings.TrimRight(first, " "))) {
		t.Errorf("tall panel starts at %q, want the report's first line %q", first, lines[0])
	}
	if m.details.scrollOffset != 22 {
		t.Error("render must not mutate the stored offset")
	}
	// Shrink it: the offset is still legal, so the window keeps its place.
	short := renderDetailsPanel(66, 12, (detailsFrameOf(m.details)).state.attached, Config{Accessible: true}, detailsFrameOf(m.details))
	first = strings.Trim(strings.Split(short, "\n")[1], "|")
	if !strings.HasPrefix(lines[22], strings.TrimRight(first, " ")) {
		t.Errorf("short panel starts at %q, want report line 22 %q", first, lines[22])
	}
}

// A resize that makes the report fit re-clamps the STORED offset too, so the
// next scroll key steps from where the panel is actually drawn instead of
// jumping. Without this the short-report failure mode ("both keys leave the
// model unchanged") would only hold for an offset that was already 0.
// (the review, medium.)
func TestResizeClampsStoredOffset(t *testing.T) {
	m := scrollModel(t, 22)
	var tm tea.Model = m
	// Tall enough for all 52 lines: nothing is hidden any more.
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: m.width, Height: 70})
	m = tm.(Model)
	if m.details.scrollOffset != 0 {
		t.Fatalf("offset after growing past the report = %d, want 0", m.details.scrollOffset)
	}
	base := m
	for _, k := range []tea.KeyType{tea.KeyCtrlPgDown, tea.KeyCtrlPgUp} {
		next, cmd := pressScroll(t, m, k)
		if cmd != nil {
			t.Errorf("%v returned a Cmd on a report that fits", k)
		}
		if !reflect.DeepEqual(next, base) {
			t.Errorf("%v changed the model on a report that fits", k)
		}
	}
	// Shrinking back keeps the offset legal for the new geometry.
	tm = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: m.width, Height: 39})
	m = tm.(Model)
	if got := m.details.scrollOffset; got < 0 || got > 22 {
		t.Errorf("offset after shrinking = %d, want a legal offset in [0,22]", got)
	}
	// And the drawn window agrees with the stored offset in both directions.
	lines := detailsReportLines(m.details.body)
	first := detailsBodyLines(t, m)[0]
	if !strings.HasPrefix(strings.TrimRight(lines[m.details.scrollOffset], " "), uncut(first)) {
		t.Errorf("drawn first row %q does not match stored offset %d (%q)", first, m.details.scrollOffset, lines[m.details.scrollOffset])
	}
}

// C19 — scrolling is presentation only: no Astrolog process starts and no
// refresh surface is touched.
func TestScrollKeysStartNoCompute(t *testing.T) {
	m := scrollModel(t, 0)
	before := m.refresh
	title, body := m.details.title, m.details.body
	for _, k := range []tea.KeyType{tea.KeyCtrlPgDown, tea.KeyCtrlPgDown, tea.KeyCtrlPgUp} {
		var cmd tea.Cmd
		m, cmd = pressScroll(t, m, k)
		if cmd != nil {
			t.Errorf("%v returned a Cmd", k)
		}
	}
	if !reflect.DeepEqual(m.refresh, before) {
		t.Error("a scroll key touched a refresh surface")
	}
	if m.details.title != title || m.details.body != body {
		t.Error("a scroll key changed the retained Details frame")
	}
	if m.details.pending || m.details.waitForText || m.details.timerArmed {
		t.Error("a scroll key marked Details pending")
	}
}

// C20 — both bindings are registered in main scope, reach the help modal, and
// the footer carries their single entry wherever Details is on screen.
func TestScrollBindingsPresent(t *testing.T) {
	keys := defaultKeys()
	found := map[BindingID]BindingEntry{}
	for _, e := range keys.mainBindings() {
		found[e.ID] = e
	}
	for _, id := range []BindingID{BindingScrollUp, BindingScrollDown} {
		e, ok := found[id]
		if !ok {
			t.Fatalf("binding %v missing from keyMap.mainBindings()", id)
		}
		if !e.Help {
			t.Errorf("binding %v is not in the help modal", id)
		}
	}
	if got := keys.ScrollUp.Keys(); len(got) != 1 || got[0] != "ctrl+pgup" {
		t.Errorf("ScrollUp keys = %v, want [ctrl+pgup]", got)
	}
	if got := keys.ScrollDown.Keys(); len(got) != 1 || got[0] != "ctrl+pgdown" {
		t.Errorf("ScrollDown keys = %v, want [ctrl+pgdown]", got)
	}
	// Both bindings carry the condition under which they act: a key that cannot
	// scroll must not match (it used to be only ScrollUp).
	for _, id := range []BindingID{BindingScrollUp, BindingScrollDown} {
		if found[id].When == nil {
			t.Errorf("binding %v has no When: it would match with nothing to scroll", id)
		}
	}
	help := helpListText(keys, defaultLeaderKey, defaultLeaderMap())
	for _, want := range []string{keys.ScrollUp.Help().Desc, keys.ScrollDown.Help().Desc} {
		if !strings.Contains(help, want) {
			t.Errorf("help modal missing %q", want)
		}
	}
	// The bar names the pair wherever Details is on screen, and a layout
	// without the panel does not advertise a key that does nothing.
	const scrollPair = "Scroll details: ⌃⇞⌃⇟"
	m := scrollModel(t, 0)
	if !strings.Contains(m.View(), scrollPair) {
		t.Errorf("the 132x40 frame's key bar is missing %q", scrollPair)
	}
	m.width, m.height = 80, 24
	if got := m.View(); strings.Contains(got, scrollPair) {
		t.Errorf("the 80x24 key bar advertises scrolling with no Details panel")
	}
}

// Failure mode — an empty report has nothing to scroll and no indicator.
func TestScrollEmptyReport(t *testing.T) {
	m := detailsTestModel(t)
	m.details = detailsViewState{title: "[0] Details"}
	box := renderDetailsPanel(66, 32, (detailsFrameOf(m.details)).state.attached, Config{Accessible: true}, detailsFrameOf(m.details))
	if strings.Contains(box, scrollThumbASCII) {
		t.Errorf("empty report drew an indicator:\n%s", box)
	}
	for _, k := range []tea.KeyType{tea.KeyCtrlPgDown, tea.KeyCtrlPgUp} {
		next, cmd := pressScroll(t, m, k)
		if cmd != nil || next.details.scrollOffset != 0 {
			t.Errorf("%v on an empty report: offset = %d, cmd = %v", k, next.details.scrollOffset, cmd)
		}
	}
}

// Failure mode — a report shorter than the panel pins the offset at 0 and
// both keys leave the model alone.
func TestScrollShortReport(t *testing.T) {
	m := detailsTestModel(t)
	m.details = detailsViewState{title: "[0] Chart", body: strings.Join(detailsReportLines(influenceReport(t))[:23], "\n")}
	base := m
	for _, k := range []tea.KeyType{tea.KeyCtrlPgDown, tea.KeyCtrlPgUp} {
		next, cmd := pressScroll(t, m, k)
		if cmd != nil {
			t.Errorf("%v returned a Cmd", k)
		}
		if !reflect.DeepEqual(next, base) {
			t.Errorf("%v changed the model on a report that fits (offset %d)", k, next.details.scrollOffset)
		}
	}
	box := renderDetailsPanel(66, 32, (detailsFrameOf(m.details)).state.attached, Config{Accessible: true}, detailsFrameOf(m.details))
	if strings.Contains(box, scrollThumbASCII) {
		t.Errorf("a report that fits drew an indicator:\n%s", box)
	}
}

// Failure mode — a failed run leaves the retained frame AND its offset alone;
// the failure reaches only the Log.
func TestScrollOffsetSurvivesErrorFrame(t *testing.T) {
	m := scrollModel(t, 16)
	before := renderDetailsPanel(66, 32, (detailsFrameOf(m.details)).state.attached, Config{Accessible: true}, detailsFrameOf(m.details))
	logLines := len(m.logLines)
	m, _ = m.commitRefresh(surfaceDetails, refreshResultMsg{
		surface: surfaceDetails,
		err:     errors.New("astrolog exploded"),
	})
	if m.details.scrollOffset != 16 {
		t.Errorf("offset = %d, want the untouched 16", m.details.scrollOffset)
	}
	if got := renderDetailsPanel(66, 32, (detailsFrameOf(m.details)).state.attached, Config{Accessible: true}, detailsFrameOf(m.details)); got != before {
		t.Error("an error frame repainted the Details panel")
	}
	if len(m.logLines) != logLines+1 {
		t.Errorf("the failure did not reach the Log: %v", m.logLines)
	}
}

// Failure mode — below the dual-column minimum there is no Details panel, so the
// keys do nothing and return no Cmd.
func TestScrollKeysInertWithoutDetailsPanel(t *testing.T) {
	m := scrollModel(t, 0)
	m.width = dualColumnMinWidth(1) - 1
	base := m
	for _, k := range []tea.KeyType{tea.KeyCtrlPgDown, tea.KeyCtrlPgUp} {
		next, cmd := pressScroll(t, m, k)
		if cmd != nil {
			t.Errorf("%v returned a Cmd with no Details panel", k)
		}
		if !reflect.DeepEqual(next, base) {
			t.Errorf("%v changed the model with no Details panel", k)
		}
	}
}

// Failure mode — an open modal owns the keyboard and the mouse.
func TestScrollKeysIgnoredInModal(t *testing.T) {
	for _, tc := range []struct {
		name  string
		apply func(Model) Model
	}{
		{"help", func(m Model) Model { m.helpOpen = true; return m }},
		{"atlas", func(m Model) Model { m.startup = startupAtlasOpen; return m }},
		{"charts", func(m Model) Model { m.startup = startupChartOpen; return m }},
	} {
		base := tc.apply(scrollModel(t, 6))
		for _, k := range []tea.KeyType{tea.KeyPgDown, tea.KeyPgUp, tea.KeyCtrlPgDown, tea.KeyCtrlPgUp} {
			next, _ := pressScroll(t, base, k)
			if next.details.scrollOffset != 6 {
				t.Errorf("%s modal: %v scrolled to %d", tc.name, k, next.details.scrollOffset)
			}
		}
		for _, button := range []tea.MouseButton{tea.MouseButtonWheelUp, tea.MouseButtonWheelDown} {
			next, cmd := wheel(t, base, button, 100, 10)
			if cmd != nil || !reflect.DeepEqual(next, base) {
				t.Errorf("%s modal: wheel changed the model", tc.name)
			}
		}
	}
}

// Failure mode — a panel with no room renders the empty string, exactly as it
// did before the indicator existed.
func TestScrollDegenerateGeometry(t *testing.T) {
	state := detailsViewState{title: "[0] Chart", body: influenceReport(t), scrollOffset: 9}
	for _, tc := range []struct{ w, h int }{{0, 0}, {3, 20}, {66, 2}, {66, 0}, {-4, -4}, {4, 3}} {
		got := renderDetailsPanel(tc.w, tc.h, (detailsFrameOf(state)).state.attached, Config{Accessible: true}, detailsFrameOf(state))
		if tc.w >= 4 && tc.h >= 3 {
			if got == "" {
				t.Errorf("%dx%d rendered nothing", tc.w, tc.h)
			}
			continue
		}
		if got != "" {
			t.Errorf("%dx%d rendered %q, want the empty string", tc.w, tc.h, strconv.Quote(got))
		}
	}
}

// uncut is a drawn row without the mark its cut left (text.go), so it can be
// compared as a prefix of the report line it was cut from.
func uncut(s string) string {
	return strings.TrimSuffix(strings.TrimSuffix(s, "..."), "…")
}
