package ui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// The Details loading label (specs/details-loading-overlay.md, v3): while a
// tab waits its bottom edge carries no counter; a second in, `loading` at the
// edge's left, its dots stepping every half second, in neon blue
// (term.LoadingColour) as text, no background; a blank tab shows no `…`.

var loadingRe = regexp.MustCompile(`loading\.{0,3}`)

// realLoadingTimer puts the app's repaint timer back for one test: the test
// binary switches it off for every other test (main_test.go).
func realLoadingTimer(t *testing.T) {
	t.Helper()
	prev := loadingTimer
	loadingTimer = longTimer
	t.Cleanup(func() { loadingTimer = prev })
}

// loadingFixture is a Timeline tab that has just started waiting on a fake
// clock: the previous list is on screen and the next month's is owed.
func loadingFixture(t *testing.T, cfg Config) (Model, *fakeClock, tea.Cmd) {
	t.Helper()
	realLoadingTimer(t)
	forceColor(t)
	stubTimeline(t, monthRows(t))
	stubTextCompute(t)
	m := loadedTimeline(t, cfg)
	m = m.withDetailsTab(PanelChart, TabDetailsTimeline)
	clock := &fakeClock{t: tickStart()}
	m.clock = clock
	m, cmd := nextMonth(m)
	if !m.detailsTabDimmed(TabDetailsTimeline) {
		t.Fatal("fixture: the timeline is not waiting")
	}
	// Any Update starts the clock of the wait.
	tm, tcmd := m.Update(loadingTickMsg{})
	m = tm.(Model)
	if tcmd == nil {
		t.Fatal("fixture: a wait starting armed no timer")
	}
	return m, clock, cmd
}

// loadingAt moves the clock d past the wait's start and delivers the tick of
// that wait.
func loadingAt(m Model, clock *fakeClock, d time.Duration) Model {
	clock.advanceTo(m.loading.since.Add(d))
	tm, _ := m.Update(loadingTickMsg{gen: m.loading.gen})
	return tm.(Model)
}

// bottomEdge is the Details panel's last line as drawn: raw keeps its SGR.
func bottomEdge(m Model, raw bool) string {
	lines := strings.Split(detailsPanelText(m), "\n")
	edge := lines[len(lines)-1]
	if raw {
		return edge
	}
	return ansi.Strip(edge)
}

// loadingText is the label as the bottom edge reads it, "" for none.
func loadingText(m Model) string {
	return loadingRe.FindString(bottomEdge(m, false))
}

var counterRe = regexp.MustCompile(`\d+ of \d+`)

func TestLoadingOverlayAfterOneSecond(t *testing.T) {
	m, clock, _ := loadingFixture(t, Config{Color: ColorAlways})
	early := loadingAt(m, clock, 900*time.Millisecond)
	if got := loadingText(early); got != "" {
		t.Errorf("at 0.9 s the edge shows %q, want none", got)
	}
	if edge := bottomEdge(early, false); counterRe.MatchString(edge) {
		t.Errorf("at 0.9 s, waiting, the edge still carries a counter: %q", edge)
	}
	m = loadingAt(m, clock, time.Second)
	if got := loadingText(m); got != "loading" {
		t.Errorf("at 1.0 s the edge reads %q, want loading", got)
	}
	edge := bottomEdge(m, false)
	if counterRe.MatchString(edge) {
		t.Errorf("at 1.0 s the counter is still in the edge beside the label: %q", edge)
	}
	if !strings.HasPrefix(edge, "╰─loading") || !strings.HasSuffix(edge, "─╯") || strings.Contains(edge, " ") {
		t.Errorf("the label is not at the edge's left in a whole edge: %q", edge)
	}
	if n := strings.Count(ansi.Strip(detailsPanelText(m)), "loading"); n != 1 {
		t.Errorf("the panel shows loading %d times, want once, in the edge (no overlay in the body)", n)
	}
}

func TestLoadingDotsStepEveryHalfSecond(t *testing.T) {
	m, clock, _ := loadingFixture(t, Config{Color: ColorAlways})
	want := []string{"loading", "loading.", "loading..", "loading...", "loading"}
	for i, w := range want {
		d := time.Second + time.Duration(i)*500*time.Millisecond
		if got := loadingText(loadingAt(m, clock, d)); got != w {
			t.Errorf("at %v the overlay reads %q, want %q", d, got, w)
		}
	}
}

func TestLoadingOverlayGoesWithTheWait(t *testing.T) {
	m, clock, cmd := loadingFixture(t, Config{Color: ColorAlways})
	m = loadingAt(m, clock, 2*time.Second)
	if loadingText(m) == "" {
		t.Fatal("fixture: no overlay at 2 s")
	}
	stale := m.loading.gen
	m = pump(t, m, cmd)
	if m.detailsTabDimmed(TabDetailsTimeline) {
		t.Fatal("fixture: the content did not arrive")
	}
	if got := loadingText(m); got != "" {
		t.Errorf("after the content arrived the overlay shows %q", got)
	}
	tm, tcmd := m.Update(loadingTickMsg{gen: stale})
	m = tm.(Model)
	if tcmd != nil || loadingText(m) != "" || m.loading.on {
		t.Errorf("a stale tick changed things: cmd=%v text=%q on=%v", tcmd != nil, loadingText(m), m.loading.on)
	}
}

func TestNoLoadingTimerWhenNothingWaits(t *testing.T) {
	realLoadingTimer(t)
	forceColor(t)
	stubTimeline(t, monthRows(t))
	stubTextCompute(t)
	m := loadedTimeline(t, Config{Color: ColorAlways})
	m = m.withDetailsTab(PanelChart, TabDetailsTimeline)
	if m.detailsTabDimmed(TabDetailsTimeline) {
		t.Fatal("fixture: the timeline waits")
	}
	for _, msg := range []tea.Msg{loadingTickMsg{}, loadingTickMsg{gen: 7}} {
		tm, cmd := m.Update(msg)
		if cmd != nil {
			t.Errorf("an Update with nothing waiting returned a timer for %T", msg)
		}
		m = tm.(Model)
	}
}

func TestLoadingOverlayWithoutColours(t *testing.T) {
	m, clock, _ := loadingFixture(t, Config{Color: ColorNever})
	m = loadingAt(m, clock, 2500*time.Millisecond)
	if got := loadingText(m); got != "loading..." {
		t.Errorf("colours off: the edge reads %q, want loading...: %q", got, bottomEdge(m, false))
	}
	if edge := bottomEdge(m, true); strings.Contains(edge, "\x1b[") {
		t.Errorf("colours off: the edge carries an escape: %q", edge)
	}
}

// The operator, 2026-11-06: "as foreground text color, no bg color".
func TestLoadingLabelIsTextColourOnly(t *testing.T) {
	m, clock, _ := loadingFixture(t, Config{Color: ColorAlways})
	m = loadingAt(m, clock, 2*time.Second)
	edge := bottomEdge(m, true)
	i := strings.Index(edge, "loading")
	if i < 0 {
		t.Fatalf("no label in the edge: %q", edge)
	}
	lead := edge[:i]
	j := strings.LastIndex(lead, "\x1b[")
	if j < 0 || !sgrHasForeground(strings.TrimSuffix(lead[j+2:], "m")) {
		t.Errorf("the label has no text colour of its own: %q", edge)
	}
	for _, m := range edgeSGRRe.FindAllStringSubmatch(edge, -1) {
		if sgrHasBackground(m[1]) {
			t.Errorf("the edge draws a background (%q): %q", m[0], edge)
		}
	}
}

var edgeSGRRe = regexp.MustCompile("\x1b\\[([0-9;]*)m")

// sgrHasForeground: a 16-colour foreground (30-37, 90-97) or an extended one (38;...).
func sgrHasForeground(params string) bool {
	for _, p := range strings.Split(params, ";") {
		switch {
		case p == "38", len(p) == 2 && (p[0] == '3' || p[0] == '9') && p[1] >= '0' && p[1] <= '7':
			return true
		}
	}
	return false
}

// sgrHasBackground: a 16-colour background (40-47, 100-107), an extended one (48;...) or reverse video (7).
func sgrHasBackground(params string) bool {
	for _, p := range strings.Split(params, ";") {
		switch {
		case p == "48", p == "7", len(p) == 2 && p[0] == '4' && p[1] >= '0' && p[1] <= '7',
			len(p) == 3 && p[:2] == "10" && p[2] >= '0' && p[2] <= '7':
			return true
		}
	}
	return false
}

func TestLoadingOverlayOnlyInDetails(t *testing.T) {
	m, clock, _ := loadingFixture(t, Config{Color: ColorAlways})
	m = loadingAt(m, clock, 2*time.Second)
	if n := strings.Count(ansi.Strip(m.View()), "loading"); n != 1 {
		t.Errorf("the whole screen shows loading %d times, want once (Details only)", n)
	}
	other := m.withDetailsTab(PanelChart, TabDetailsReport)
	if loadingText(other) != "" && !other.detailsTabDimmed(TabDetailsReport) {
		t.Errorf("a settled report tab shows the overlay")
	}
}

// A tab with nothing of its subject yet draws no `…` and no counter (operator,
// 2026-11-06: "remove the '...' on a blank screen ... no '0 of 0' text or
// anything else, none if loading"); a second into the wait the label shows.
func TestBlankDetailsShowsLoadingOnly(t *testing.T) {
	realLoadingTimer(t)
	forceColor(t)
	freshModels(t)
	stubTimeline(t, monthRows(t))
	stubTextCompute(t)
	m := timelineModel(t, Config{Color: ColorAlways})
	m.width = max(m.width, dualColumnMinWidth(m.shownCount()))
	m = m.withDetailsTab(PanelChart, TabDetailsTimeline)
	clock := &fakeClock{t: tickStart()}
	m.clock = clock
	m, _ = syncTimelineVisited(m)
	if m.timelinePaneState() != paneEmpty || !m.detailsTabWaiting(TabDetailsTimeline) {
		t.Fatalf("fixture: state %v waiting %v, want an empty tab waiting", m.timelinePaneState(), m.detailsTabWaiting(TabDetailsTimeline))
	}
	tm, _ := m.Update(loadingTickMsg{})
	m = tm.(Model)
	plain := ansi.Strip(detailsPanelText(m))
	lines := strings.Split(plain, "\n")
	for _, l := range lines[1 : len(lines)-1] {
		if strings.Contains(l, emptyPaneLine) {
			t.Errorf("the blank tab still shows %q: %q", emptyPaneLine, l)
		}
	}
	if edge := bottomEdge(m, false); counterRe.MatchString(edge) || loadingText(m) != "" {
		t.Errorf("before a second the blank tab's edge carries %q, want a plain edge", edge)
	}
	m = loadingAt(m, clock, time.Second)
	if got := loadingText(m); got != "loading" {
		t.Errorf("a second into the first load the edge reads %q, want loading", got)
	}
}
