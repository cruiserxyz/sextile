package ui

import (
	"fmt"
	"strings"
	"testing"
)

// the specification § Taste lane. These goldens were frozen
// MECHANICALLY — rendered and written as they came out, with no taste
// judgement made — so that each frame is pinned against regression. The
// contact sheet at the design notes shows them to
// the operator; a different treatment after looking is a new item that
// re-renders and re-freezes.

var tasteDetailsWidths = []int{52, 66, 80, 100}

// V1 — the Details tab strip, carrying the [0] prefix, at four Details widths,
// with each tab active, in colour and with colour off. There is no one-tab
// frame: C11 declares two tabs for every subject.
func TestDetailsTabStripTaste(t *testing.T) {
	forceColor(t)
	for _, mode := range []struct {
		name string
		cfg  Config
	}{{"color", Config{Color: ColorAlways}}, {"never", Config{Color: ColorNever}}} {
		for _, tab := range []TabID{TabDetailsReport, TabDetailsTimeline} {
			var sheet strings.Builder
			for _, w := range tasteDetailsWidths {
				st := detailsViewState{title: "[0] Chart", tabs: map[PanelID]TabID{PanelChart: tab}}
				box := renderDetailsPanel(w, 3, (detailsFrameOf(st)).state.attached, mode.cfg, detailsFrameOf(st))
				fmt.Fprintf(&sheet, "== details width %d ==\n%s\n", w, strings.SplitN(box, "\n", 2)[0])
			}
			checkGolden(t, fmt.Sprintf("details-tabstrip-%s-%s", tab, mode.name), sheet.String())
		}
	}
}

// V2 — one October 2026 week, all three sources interleaved, at four Details
// widths. Rows are truncated at the edge, never wrapped: one row is one
// Astrolog sentence, and Enter and the cursor both address rows.
func TestDetailsTimelineWeekTaste(t *testing.T) {
	rows := weekRows(t)
	var sheet strings.Builder
	for _, w := range tasteDetailsWidths {
		st := detailsViewState{title: "[0] Chart", tabs: map[PanelID]TabID{PanelChart: TabDetailsTimeline}}
		frame := detailsFrameOf(st)
		frame.rows = rows
		fmt.Fprintf(&sheet, "== details width %d ==\n%s\n", w, renderDetailsPanel(w, len(rows)+2, (frame).state.attached, Config{Color: ColorNever}, frame))
	}
	checkGolden(t, "details-timeline-week", sheet.String())
}

// V3 — the row cursor on an attached timeline at 132x40, against today's
// active-row treatment (the invert band), in colour and with colour off.
func TestDetailsRowCursorTaste(t *testing.T) {
	forceColor(t)
	for _, mode := range []struct {
		name string
		cfg  Config
	}{{"color", Config{Color: ColorAlways}}, {"never", Config{Color: ColorNever}}} {
		m := onTimeline(t, mode.cfg)
		for i := 0; i < 5; i++ {
			m, _ = tpress(t, m, runes("j"))
		}
		view := m.View()
		assertFrameGeometry(t, view, m.width, 40)
		checkGolden(t, "details-rowcursor-"+mode.name, view)
	}
}
