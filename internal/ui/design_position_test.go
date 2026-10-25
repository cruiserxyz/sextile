package ui

// the specification: where you are in every list, the bar
// on whatever is taller than its frame, help as an ordinary popup list, and a
// mark on every cut. Every check reads the rendered View() of a model reached
// through keys, apart from the two the plan scopes to a renderer (C10, and the
// empty-title refusal, which no screen can show).

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// frameRect is one frame on screen, in cells.
type frameRect struct{ x, y, w, h int }

// screenRunes is a rendered view as rows of cells; the second cell of a wide
// character is 0, so a cell index is a screen column.
func screenRunes(view string) [][]rune {
	s := parseDesignScreen(view)
	out := make([][]rune, len(s))
	for y, row := range s {
		for _, c := range row {
			out[y] = append(out[y], c.r)
		}
	}
	return out
}

// findTitledFrame finds the one frame whose top edge opens with title — after
// the corner, one dash and, with colours off, the focus mark.
func findTitledFrame(t *testing.T, view, title string) frameRect {
	t.Helper()
	rows := screenRunes(view)
	var found []frameRect
	for y, row := range rows {
		for x := 0; x+1 < len(row); x++ {
			if (row[x] != '╭' || row[x+1] != '─') && (row[x] != '+' || row[x+1] != '-') {
				continue
			}
			rest := string(row[x+2:])
			rest = strings.TrimPrefix(strings.TrimPrefix(rest, "▸"), ">")
			if !strings.HasPrefix(rest, title) {
				continue
			}
			corner := '╮'
			if row[x] == '+' {
				corner = '+'
			}
			w := 0
			for i := x + 1; i < len(row); i++ {
				if row[i] == corner {
					w = i - x + 1
					break
				}
			}
			bottom := '╰'
			if row[x] == '+' {
				bottom = '+'
			}
			h := 0
			for j := y + 1; j < len(rows); j++ {
				if x < len(rows[j]) && rows[j][x] == bottom {
					h = j - y + 1
					break
				}
			}
			if w > 0 && h > 0 {
				found = append(found, frameRect{x, y, w, h})
			}
		}
	}
	if len(found) != 1 {
		t.Fatalf("want one frame titled %q on screen, found %d:\n%s", title, len(found), ansi.Strip(view))
	}
	return found[0]
}

// edgeText is a frame row's text across the frame's width.
func edgeText(view string, r frameRect, y int) string {
	rows := screenRunes(view)
	var b strings.Builder
	for x := r.x; x < r.x+r.w && x < len(rows[y]); x++ {
		if c := rows[y][x]; c != 0 {
			b.WriteRune(c)
		}
	}
	return b.String()
}

var counterPattern = regexp.MustCompile(`(\d+) of (\d+)`)

// readCounter reads `N of M` from the frame's bottom edge, and fails unless it
// sits right of the edge's midpoint.
func readCounter(t *testing.T, view string, r frameRect) (n, m int, ok bool) {
	t.Helper()
	edge := edgeText(view, r, r.y+r.h-1)
	loc := counterPattern.FindStringSubmatchIndex(edge)
	if loc == nil {
		return 0, 0, false
	}
	if col := ansi.StringWidth(edge[:loc[0]]); col < r.w/2 {
		t.Errorf("the counter starts at column %d of a %d-wide edge, left of its midpoint: %q", col, r.w, edge)
	}
	n, _ = strconv.Atoi(edge[loc[2]:loc[3]])
	m, _ = strconv.Atoi(edge[loc[4]:loc[5]])
	return n, m, true
}

// positionOwner is one list with a cursor, how to reach it, and the frame
// title that finds it.
type positionOwner struct {
	name  string
	wide  bool // exists at 144 columns only
	model func(t *testing.T, cfg Config, w, h int) Model
	title string
	// pos is the model's own cursor and the number of positions it walks:
	// what the counter must say (the plan's table).
	pos func(m Model) (cursor, total int)
}

func designAt(t *testing.T, cfg Config, w, h int) Model {
	t.Helper()
	return consoleTasteModel(t, cfg, w, h, 30)
}

func positionOwners() []positionOwner {
	return []positionOwner{
		{name: "chart slots", title: "[1] Chart", model: func(t *testing.T, cfg Config, w, h int) Model {
			m, _ := press(t, designAt(t, cfg, w, h), "1")
			m.cursorIdx = 0
			return m
		}, pos: func(m Model) (int, int) { return m.cursorIdx, len(m.slots.All()) }},
		{name: "bodies", title: "[2] Bodies", model: func(t *testing.T, cfg Config, w, h int) Model {
			m, _ := press(t, designAt(t, cfg, w, h), "2")
			m.bodyCursorIdx = 0
			return m
		}, pos: func(m Model) (int, int) { return m.bodyCursorIdx, len(m.chart.positions) }},
		{name: "log console", title: "Log", model: func(t *testing.T, cfg Config, w, h int) Model {
			m, _ := press(t, designAt(t, cfg, w, h), ",", "l")
			m.console.cursor = 0
			m.console.offset = 0
			return m
		}, pos: func(m Model) (int, int) { return m.console.cursor, len(m.console.snapshot) }},
		{name: "attached timeline", wide: true, title: "[0] ", model: func(t *testing.T, cfg Config, w, h int) Model {
			m := onTimeline(t, cfg)
			m.width, m.height = w, h
			m.details.cursor, m.details.rowOffset = 0, 0
			return m
		}, pos: func(m Model) (int, int) { return m.details.cursor, len(m.timelineRows()) }},
		{name: "city popup", title: popupTitleCity, model: func(t *testing.T, cfg Config, w, h int) Model {
			m := designAt(t, cfg, w, h)
			m.atlas = liftState("b", 0)
			m.startup = startupAtlasOpen
			return m
		}, pos: func(m Model) (int, int) { return m.atlas.cursor, len(m.atlas.matches) }},
		{name: "open chart popup", title: popupTitleOpenChart, model: func(t *testing.T, cfg Config, w, h int) Model {
			m := designAt(t, cfg, w, h)
			m.picker = listRowsChartState()
			m.picker.drops = []store.ChartDrop{{File: "broken.as", Reason: "not a chart"}}
			m.picker.cursor = 0
			m.startup = startupChartOpen
			return m
		}, pos: func(m Model) (int, int) { return m.picker.cursor, len(m.picker.matches) }},
		{name: "bodies popup", title: popupTitleBodies, model: func(t *testing.T, cfg Config, w, h int) Model {
			m := designAt(t, cfg, w, h)
			m.bodyModal = listRowsBodyState()
			m.bodyModal.cursor = 0
			m.startup = startupBodyOpen
			return m
		}, pos: func(m Model) (int, int) { return m.bodyModal.cursor, m.bodyModal.totalRows() }},
		{name: "aspects popup", title: popupTitleAspects, model: func(t *testing.T, cfg Config, w, h int) Model {
			m := designAt(t, cfg, w, h)
			m.aspectModal = listRowsAspectState("")
			m.aspectModal.cursor = 0
			m.startup = startupAspectOpen
			return m
		}, pos: func(m Model) (int, int) {
			return m.aspectModal.cursor, familyRows() + len(m.aspectModal.matches)
		}},
	}
}

var positionSizes = [][2]int{{80, 24}, {144, 40}}

// C6: every list with a cursor shows `N of M` in its frame's bottom edge, at
// the first row, after one ↓, and at the last row, N and M being the cursor's
// 1-based position and the positions the cursor walks.
func TestEveryListShowsPosition(t *testing.T) {
	cfg := Config{Color: ColorNever}
	for _, o := range positionOwners() {
		for _, size := range positionSizes {
			w, h := size[0], size[1]
			if o.wide && w < 144 {
				continue
			}
			t.Run(fmt.Sprintf("%s/%dx%d", o.name, w, h), func(t *testing.T) {
				m := o.model(t, cfg, w, h)
				check := func(when string, m Model) (int, int) {
					t.Helper()
					view := m.View()
					n, total, ok := readCounter(t, view, findTitledFrame(t, view, o.title))
					if !ok {
						t.Fatalf("%s: no `N of M` in the bottom edge of %s:\n%s", when, o.name, ansi.Strip(view))
					}
					cur, want := o.pos(m)
					if total != want || n != cur+1 {
						t.Fatalf("%s: counter reads %d of %d, want %d of %d", when, n, total, cur+1, want)
					}
					return n, total
				}
				if n, _ := check("first row", m); n != 1 {
					t.Fatalf("first row: counter reads %d, want 1", n)
				}
				m, _ = press(t, m, "down")
				n, total := check("after one ↓", m)
				if total > 1 && n != 2 {
					t.Fatalf("after one ↓ the counter reads %d, want 2", n)
				}
				// Some lists wrap (Bodies), so walk exactly to the last row.
				for i := 2; i < total; i++ {
					m, _ = press(t, m, "down")
				}
				if n, total := check("last row", m); n != total {
					t.Fatalf("after walking to the end the counter reads %d of %d", n, total)
				}
			})
		}
	}
}

// C7: a list with no position the cursor can take reads `0 of 0`.
func TestEmptyListsReadZeroOfZero(t *testing.T) {
	cfg := Config{Color: ColorNever}
	cases := []struct {
		name, title string
		model       func(t *testing.T) Model
	}{
		{"city popup before typing", popupTitleCity, func(t *testing.T) Model {
			m := designAt(t, cfg, 80, 24)
			m.atlas = liftState("", 0)
			m.startup = startupAtlasOpen
			return m
		}},
		{"city popup with no match", popupTitleCity, func(t *testing.T) Model {
			m := designAt(t, cfg, 80, 24)
			m.atlas = liftState("zzzzqqq", 0)
			m.startup = startupAtlasOpen
			return m
		}},
		{"open chart popup, empty library", popupTitleOpenChart, func(t *testing.T) Model {
			m := designAt(t, cfg, 80, 24)
			m.picker = chartPickerState{loaded: true, prior: startupReady}
			m.picker.refilter()
			m.startup = startupChartOpen
			return m
		}},
		{"open chart popup, loading", popupTitleOpenChart, func(t *testing.T) Model {
			m := designAt(t, cfg, 80, 24)
			m.picker = chartPickerState{prior: startupReady}
			m.startup = startupChartOpen
			return m
		}},
		{"console, empty", "Log", func(t *testing.T) Model {
			m, _ := press(t, consoleTasteModel(t, cfg, 80, 24, 0), ",", "l")
			return m
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view := tc.model(t).View()
			n, total, ok := readCounter(t, view, findTitledFrame(t, view, tc.title))
			if !ok || n != 0 || total != 0 {
				t.Errorf("want `0 of 0`, read %d of %d (found=%v):\n%s", n, total, ok, ansi.Strip(view))
			}
		})
	}
}

// thumbRows counts the right-edge cells of a frame that carry the bar.
func thumbRows(view string, r frameRect, cfg Config) int {
	glyph := []rune(scrollThumbGlyph)[0]
	if cfg.useASCII() {
		glyph = []rune(scrollThumbASCII)[0]
	}
	rows := screenRunes(view)
	n := 0
	for y := r.y + 1; y < r.y+r.h-1; y++ {
		if x := r.x + r.w - 1; x < len(rows[y]) && rows[y][x] == glyph {
			n++
		}
	}
	return n
}

// manyCharts is a library longer than any popup list.
func manyCharts(n int) []store.NamedChart {
	out := make([]store.NamedChart, n)
	for i := range out {
		out[i] = store.NamedChart{
			ID: fmt.Sprintf("c%02d", i), Name: fmt.Sprintf("Constructed %02d", i), Year: 1950 + i,
			Place:  location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5, Lon: 13.4, TZ: "Europe/Berlin"},
			Source: store.ChartSource{Kind: store.ChartKindFile, File: fmt.Sprintf("c%02d.as", i)},
		}
	}
	return out
}

// manyAspects is more aspects than the Aspects panel's grid can hold.
func manyAspects() []aspect.Aspect {
	bodies := []aspect.Body{aspect.Sun, aspect.Moon, aspect.Mercury, aspect.Venus, aspect.Mars, aspect.Jupiter, aspect.Saturn, aspect.Uranus, aspect.Neptune, aspect.Pluto}
	kinds := []aspect.AspectKind{aspect.Conjunction, aspect.Opposition, aspect.Square, aspect.Trine, aspect.Sextile}
	var out []aspect.Aspect
	for i, a := range bodies {
		for _, b := range bodies[i+1:] {
			out = append(out, aspect.NewAspect(a, b, kinds[len(out)%len(kinds)], 1))
		}
	}
	return out
}

// manyCrossAspects is manyAspects as transit-to-natal aspects.
func manyCrossAspects() []lookup.OverlayAspect {
	var out []lookup.OverlayAspect
	for _, a := range manyAspects() {
		out = append(out, lookup.OverlayAspect{Transit: a.A, Natal: a.B, Kind: a.Kind, Orb: a.Orb, Power: a.Power})
	}
	return out
}

// C9: whatever is taller than its body shows the bar on its frame's right
// edge, and nothing that fits does. The Aspects panel cannot scroll, so it
// never shows one (operator, 2026-10-02).
func TestEveryOverflowShowsTheBar(t *testing.T) {
	cfg := Config{Color: ColorNever}
	cases := []struct {
		name, title string
		model       func(t *testing.T) Model
		want        bool
	}{
		{"Bodies panel at 80x24", "[2] Bodies", func(t *testing.T) Model { return designAt(t, cfg, 80, 24) }, true},
		{"Details report", "[0] ", func(t *testing.T) Model { return scrollModel(t, 0) }, true},
		{"side Log, 30 lines", "Log", func(t *testing.T) Model { return designAt(t, cfg, 144, 40) }, true},
		{"side Log, 2 lines", "Log", func(t *testing.T) Model { return consoleTasteModel(t, cfg, 144, 40, 2) }, false},
		{"console, 30 lines at 80x24", "Log", func(t *testing.T) Model {
			m, _ := press(t, designAt(t, cfg, 80, 24), ",", "l")
			return m
		}, true},
		{"city popup, capped matches", popupTitleCity, func(t *testing.T) Model {
			m := designAt(t, cfg, 80, 24)
			m.atlas = liftCappedState()
			m.startup = startupAtlasOpen
			return m
		}, true},
		{"open chart popup, 40 charts", popupTitleOpenChart, func(t *testing.T) Model {
			m := designAt(t, cfg, 80, 24)
			m.picker = chartPickerState{loaded: true, prior: startupReady, charts: manyCharts(40)}
			m.picker.refilter()
			m.startup = startupChartOpen
			return m
		}, true},
		{"open chart popup, 3 charts", popupTitleOpenChart, func(t *testing.T) Model {
			m := designAt(t, cfg, 80, 24)
			m.picker = listRowsChartState()
			m.startup = startupChartOpen
			return m
		}, false},
		// 144x40, not 80x24: at 80x24 the five group rows, their rule and the
		// Enabled bar leave the list ONE row (the specification 7),
		// and a one-row track has no thumb to draw (viewport.overflows).
		{"bodies popup, full registry", popupTitleBodies, func(t *testing.T) Model {
			m := designAt(t, cfg, 144, 40)
			m.bodyModal = openBodyModal(bigObjectRegistry(60), recipe.ObjectPolicy{Baseline: recipe.Explicit}, nil)
			m.startup = startupBodyOpen
			return m
		}, 60 > bodyListHeight(40)},
		{"aspects popup, full registry", popupTitleAspects, func(t *testing.T) Model {
			m := designAt(t, cfg, 80, 24)
			m.aspectModal = listRowsAspectState("")
			m.startup = startupAspectOpen
			return m
		}, len(listRowsAspectState("").matches) > modalListHeight(24)-familyRows()},
		{"help popup at 80x24", popupTitleKeys, func(t *testing.T) Model {
			m, _ := press(t, designAt(t, cfg, 80, 24), "?")
			return m
		}, true},
		{"Aspects panel, more than fit", "[3] ", func(t *testing.T) Model {
			m := designAt(t, cfg, 80, 24)
			m.chart.aspects = manyAspects()
			m.chart.crossAspects = manyCrossAspects()
			return m
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view := tc.model(t).View()
			got := thumbRows(view, findTitledFrame(t, view, tc.title), cfg) > 0
			if got != tc.want {
				t.Errorf("bar shown = %v, want %v:\n%s", got, tc.want, ansi.Strip(view))
			}
		})
	}

	// The side Log shows its newest lines, so what it hides is above them:
	// its thumb sits at the bottom of the track, on the last body row.
	t.Run("side Log thumb at the bottom", func(t *testing.T) {
		view := designAt(t, cfg, 144, 40).View()
		r := findTitledFrame(t, view, "Log")
		rows := screenRunes(view)
		glyph := []rune(scrollThumbGlyph)[0]
		if last := rows[r.y+r.h-2][r.x+r.w-1]; last != glyph {
			t.Errorf("the last body row's edge is %q, want the thumb: the hidden lines are above\n%s", last, ansi.Strip(view))
		}
		if first := rows[r.y+1][r.x+r.w-1]; first == glyph {
			t.Errorf("the thumb reaches the first body row, as if nothing were hidden above")
		}
	})
}

// C8: with colours on, the counter is in the focus colour and bold while its
// owner has the keyboard, and in the frame colour, not bold, otherwise.
func TestPositionCounterStyle(t *testing.T) {
	forceColor(t)
	cfg := Config{Color: ColorAlways}
	focus, frame := styleFG(t, focusColour), styleFG(t, frameColour)
	cases := []struct {
		name, title string
		model       func(t *testing.T) Model
		keyboard    bool
	}{
		{"bodies focused", "[2] Bodies", func(t *testing.T) Model {
			m, _ := press(t, designAt(t, cfg, 144, 40), "2")
			return m
		}, true},
		{"bodies unfocused", "[2] Bodies", func(t *testing.T) Model {
			m, _ := press(t, designAt(t, cfg, 144, 40), "1")
			return m
		}, false},
		{"console open", "Log", func(t *testing.T) Model {
			m, _ := press(t, designAt(t, cfg, 144, 40), ",", "l")
			return m
		}, true},
		{"timeline attached", "[0] ", func(t *testing.T) Model { return onTimeline(t, cfg) }, true},
		{"timeline unattached", "[0] ", func(t *testing.T) Model {
			m, _ := press(t, onTimeline(t, cfg), "esc")
			if m.details.attached {
				t.Fatal("fixture: Esc did not detach Details")
			}
			return m
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view := tc.model(t).View()
			r := findTitledFrame(t, view, tc.title)
			s := parseDesignScreen(view)
			edge := edgeText(view, r, r.y+r.h-1)
			loc := counterPattern.FindStringIndex(edge)
			if loc == nil {
				t.Fatalf("no counter in %q", edge)
			}
			start := r.x + ansi.StringWidth(edge[:loc[0]])
			for x := start; x < start+ansi.StringWidth(edge[loc[0]:loc[1]]); x++ {
				c := s[r.y+r.h-1][x]
				if tc.keyboard && (c.fg != focus || !c.bold) {
					t.Fatalf("counter cell %q is fg=%q bold=%v, want the focus colour %q, bold", c.r, c.fg, c.bold, focus)
				}
				if !tc.keyboard && (c.fg != frame || c.bold) {
					t.Fatalf("counter cell %q is fg=%q bold=%v, want the frame colour %q, not bold", c.r, c.fg, c.bold, frame)
				}
			}
		})
	}

	// C10: an edge too narrow for the whole counter keeps its plain edge. No
	// supported size reaches this (popups are 60 wide, panels 66), so it is
	// checked on the edge renderer, as the plan scopes it.
	t.Run("too narrow", func(t *testing.T) {
		box := renderTitledBox("T", 12, 3, true, cfg, "")
		got := withPositionCounter(box, listPosition{Cursor: 1233, Total: 5678}, cfg, true)
		if got != box {
			t.Errorf("a 12-cell edge was changed by a 12-cell counter:\n%q\n%q", box, got)
		}
		if strings.ContainsAny(ansi.Strip(got), "0123456789") {
			t.Errorf("part of the counter was drawn: %q", ansi.Strip(got))
		}
		if _, ok := bottomEdgeWithCounter(12, listPosition{Cursor: 1233, Total: 5678}, cfg, true); ok {
			t.Error("bottomEdgeWithCounter reports a 12-cell counter fits a 12-cell edge")
		}
	})
}

// cursorRowText is the text of the frame row carrying the colours-off cursor
// marker.
func cursorRowText(t *testing.T, view string, r frameRect) string {
	t.Helper()
	for y := r.y + 1; y < r.y+r.h-1; y++ {
		if row := edgeText(view, r, y); strings.Contains(row, rowCursorUnicode) {
			return row
		}
	}
	t.Fatalf("no cursor row in the frame:\n%s", ansi.Strip(view))
	return ""
}

// Help fits whole on screen, has no cursor row and no `N of M` counter, and
// scrolls: ↓ moves the window one line (the first visible line changes), ↑
// returns, and it reopens at the top.
func TestHelpPopupFits(t *testing.T) {
	cfg := Config{Color: ColorNever}
	for _, size := range positionSizes {
		w, h := size[0], size[1]
		t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
			m, _ := press(t, designAt(t, cfg, w, h), "?")
			view := m.View()
			r := findTitledFrame(t, view, popupTitleKeys)
			rows := screenRunes(view)
			if r.y < 0 || r.x < 0 || r.y+r.h > h || r.x+r.w > w || rows[r.y+r.h-1][r.x+r.w-1] != '╯' {
				t.Fatalf("help's frame %+v is not whole on a %dx%d screen:\n%s", r, w, h, ansi.Strip(view))
			}
			for y := r.y + 1; y < r.y+r.h-1; y++ {
				if strings.Contains(edgeText(view, r, y), rowCursorUnicode) {
					t.Fatalf("help draws a cursor row:\n%s", ansi.Strip(view))
				}
			}
			if _, _, ok := readCounter(t, view, r); ok {
				t.Fatalf("help shows an `N of M` counter:\n%s", ansi.Strip(view))
			}
			first := func(m Model) string {
				return strings.TrimSpace(edgeText(m.View(), r, r.y+2))
			}
			top := first(m)
			m, _ = press(t, m, "down")
			if m.help.offset != 1 || first(m) == top {
				t.Fatalf("↓ did not scroll one line (offset %d)", m.help.offset)
			}
			m, _ = press(t, m, "up")
			if m.help.offset != 0 || first(m) != top {
				t.Fatalf("↑ did not return to the top (offset %d)", m.help.offset)
			}
			m, _ = press(t, m, "down")
			m, _ = press(t, m, "?")
			m, _ = press(t, m, "?")
			if m.help.offset != 0 {
				t.Fatalf("help reopened at offset %d, want 0", m.help.offset)
			}
		})
	}
}

// C11: wherever text is shortened to fit, the mark stands where text was
// removed. Each case puts text too long for its place on screen and names
// the part that must sit beside the mark.
func TestTruncationIsMarked(t *testing.T) {
	long := "Constructedverylongname Withanevenlongersurname Andmore"
	for _, mode := range []struct {
		name string
		cfg  Config
	}{{"unicode", Config{Color: ColorNever}}, {"accessible", Config{Accessible: true}}} {
		mark := cutMark(mode.cfg)
		cases := []struct {
			name string
			view func(t *testing.T) string
			// right: the kept text is followed by the mark; left: the mark
			// is followed by the kept text.
			before, after string
		}{
			{"Chart panel place column", func(t *testing.T) string {
				m := designAt(t, mode.cfg, 80, 24)
				place := testPlaceBerlin()
				place.Name = "Llanfairpwllgwyngyllgogerychwyrndrobwllllantysiliogogogoch"
				slots, err := store.NewSlots(testFrozenTime(), place)
				if err != nil {
					t.Fatal(err)
				}
				// The cut stays in the comparing layout; the one-chart layout drops
				// what does not fit without a mark (the specification
				// rule 3), so this case needs two selected charts.
				if err := addAnchor(slots, testFrozenTime().AddDate(-30, 0, 0), place); err != nil {
					t.Fatal(err)
				}
				m.slots, m.cursorIdx = slots, 0
				return m.View()
			}, "Llanfair", ""},
			{"Bodies popup tag bar", func(t *testing.T) string {
				m := designAt(t, mode.cfg, 80, 24)
				var reg []aspect.RegistryObject
				var sel []int
				for i := 0; i < 40; i++ {
					reg = append(reg, aspect.RegistryObject{Number: i, Name: "Object"})
					sel = append(sel, i)
				}
				m.bodyModal = openBodyModal(reg, recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: sel}, nil)
				m.startup = startupBodyOpen
				return m.View()
			}, "Object", ""},
			{"Open chart list row", func(t *testing.T) string {
				m := designAt(t, mode.cfg, 80, 24)
				charts := manyCharts(2)
				charts[0].Name = long
				m.picker = chartPickerState{loaded: true, prior: startupReady, charts: charts}
				m.picker.refilter()
				m.startup = startupChartOpen
				return m.View()
			}, "Constructed", ""},
			{"Details report line", func(t *testing.T) string {
				m := scrollModel(t, 0)
				m.cfg = mode.cfg
				// Details needs 80 cells beside the 52-cell left base, so
				// 132 is its narrowest width; the fixture's longest line is
				// 76 cells, so a header wider than the panel stands in.
				m.width = 132
				m.details.body = "Astrolog 8.00 chart for " + long + " " + long
				return m.View()
			}, "Astrolog", ""},
			{"Side Log line", func(t *testing.T) string {
				m := consoleTasteModel(t, mode.cfg, 144, 40, 2)
				m = m.appendLog(laneNote, "note: "+long+" "+long)
				return m.View()
			}, "Constructed", ""},
			{"Timeline row", func(t *testing.T) string {
				m := onTimeline(t, mode.cfg)
				m.width, m.height = 144, 40
				return m.View()
			}, "Moon (", ""},
			// No popup title is long enough to be cut at a supported size, so
			// the box is drawn with one; the frame is what reaches the screen.
			{"Popup title", func(t *testing.T) string {
				return renderModalBox(modalBox{Title: long + long}, mode.cfg, 80, 24, func(w, h int) string { return "" })
			}, "Constructed", ""},
			// Bodies rows fit at every supported width; the panel body is
			// drawn narrower than any of them to reach the cut.
			{"Bodies row", func(t *testing.T) string {
				m := designAt(t, mode.cfg, 80, 24)
				body, _, _ := renderBodiesBody(m.chart, m.slots, m.cursorIdx, m.bodyCursorIdx, 14, 10, true, mode.cfg)
				return body
			}, "Sun", ""},
			{"Save chart typed value", func(t *testing.T) string {
				tm, _ := designAt(t, mode.cfg, 80, 24).Update(tea.KeyMsg{Type: tea.KeyCtrlS})
				m, _ := press(t, tm.(Model), long+long)
				if m.startup != startupSaveOpen {
					t.Fatal("fixture: Ctrl+S did not open the save popup")
				}
				return m.View()
			}, "", "Andmore"},
		}
		for _, tc := range cases {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				plain := ansi.Strip(tc.view(t))
				found := false
				for _, line := range strings.Split(plain, "\n") {
					i := strings.Index(line, mark)
					for i >= 0 {
						left, right := line[:i], line[i+len(mark):]
						if (tc.before == "" || strings.Contains(left, tc.before)) && (tc.after == "" || strings.Contains(right, tc.after)) {
							found = true
						}
						j := strings.Index(right, mark)
						if j < 0 {
							break
						}
						i += len(mark) + j
					}
				}
				if !found {
					t.Errorf("no cut marked %q near %q/%q on screen:\n%s", mark, tc.before, tc.after, plain)
				}
			})
		}
	}
}

// C13: the open-chart popup says `(no matches)` if, and only if, the library
// has a readable chart, the filter is not empty and no readable chart
// matches. Unreadable files stay listed under every filter.
func TestChartPickNoMatches(t *testing.T) {
	cfg := Config{Color: ColorNever}
	drop := store.ChartDrop{File: "broken-anna.as", Reason: "not a chart"}
	cases := []struct {
		name    string
		charts  []store.NamedChart
		filter  string
		note    bool
		dropRow bool
	}{
		{"filter matching nothing, unreadable file present", manyCharts(3), "zzzz", true, true},
		{"filter matching a readable chart", manyCharts(3), "01", false, true},
		{"empty library with one unreadable file", nil, "", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := designAt(t, cfg, 80, 24)
			m.picker = chartPickerState{loaded: true, prior: startupReady, charts: tc.charts, drops: []store.ChartDrop{drop}}
			m.picker.filterText = tc.filter
			m.picker.refilter()
			m.startup = startupChartOpen
			plain := ansi.Strip(m.View())
			if got := strings.Contains(plain, "(no matches)"); got != tc.note {
				t.Errorf("(no matches) shown = %v, want %v:\n%s", got, tc.note, plain)
			}
			if got := strings.Contains(plain, drop.File); got != tc.dropRow {
				t.Errorf("unreadable row shown = %v, want %v:\n%s", got, tc.dropRow, plain)
			}
		})
	}
}

// topTitles reads every frame's title text from a rendered view: what sits
// between a top-left corner's dash and the next run of dashes.
func topTitles(view string) []string {
	var out []string
	for _, row := range screenRunes(view) {
		line := string(row)
		for _, corner := range []string{"╭─", "+-"} {
			for i := strings.Index(line, corner); i >= 0; {
				rest := line[i+len(corner):]
				end := strings.Index(rest, "──")
				if corner == "+-" {
					end = strings.Index(rest, "--")
				}
				if end < 0 {
					end = len(rest)
				}
				if t := strings.TrimRight(rest[:end], "─-╮+"); t != "" {
					out = append(out, t)
				}
				j := strings.Index(rest, corner)
				if j < 0 {
					break
				}
				i += len(corner) + j
			}
		}
	}
	return out
}

// sentenceCaseViolation says what is wrong with one title, or "".
var tabPrefix = regexp.MustCompile(`^\[\d\] `)

// isNumeralWord reports whether w is a selected-set numeral, circled or plain.
func isNumeralWord(w string) bool {
	for _, n := range circledNumerals {
		if w == n {
			return true
		}
	}
	return len(w) == 1 && w[0] >= '1' && w[0] <= '6'
}

func sentenceCaseViolation(title string) string {
	title = strings.TrimPrefix(strings.TrimPrefix(title, "▸"), ">")
	title = tabPrefix.ReplaceAllString(title, "")
	for _, tab := range strings.Split(title, " - ") {
		tab = strings.Trim(tab, "[]")
		if tab == "" {
			continue
		}
		// After a colon comes the subject: a body, Chart, or a chart's name,
		// which is a proper name and keeps its capitals.
		head, subject, _ := strings.Cut(tab, ":")
		// The subject opens with its name's capital; what follows may be in
		// sentence case (`Details: Transit aspects`).
		if w := strings.Fields(subject); len(w) > 0 {
			if r := []rune(w[0]); !unicode.IsUpper(r[0]) && !unicode.IsDigit(r[0]) {
				return fmt.Sprintf("%q names its subject %q in lower case; a subject is a proper name", tab, w[0])
			}
		}
		words := strings.Fields(head)
		for i, w := range words {
			r := []rune(w)
			// A pair tab (`① vs ②`, `1 vs 2`) opens with a numeral, which has
			// no case (selected-charts-compare C1); the words after it keep
			// the rule.
			if i == 0 && !unicode.IsUpper(r[0]) && !isNumeralWord(w) {
				return fmt.Sprintf("%q does not start with a capital", tab)
			}
			for k, c := range r {
				if (i > 0 || k > 0) && unicode.IsUpper(c) {
					return fmt.Sprintf("%q has a capital inside it", tab)
				}
			}
		}
	}
	return ""
}

// C14: every panel title, tab name and popup title on screen is in sentence
// case.
func TestTitlesAreSentenceCase(t *testing.T) {
	cfg := Config{Color: ColorNever}
	views := map[string]func(t *testing.T) string{
		"workspace": func(t *testing.T) string { return designAt(t, cfg, 144, 40).View() },
		"timeline":  func(t *testing.T) string { return onTimeline(t, cfg).View() },
		"console": func(t *testing.T) string {
			m, _ := press(t, designAt(t, cfg, 144, 40), ",", "l")
			return m.View()
		},
	}
	for _, o := range positionOwners() {
		if strings.HasSuffix(o.name, "popup") {
			o := o
			views[o.name] = func(t *testing.T) string { return o.model(t, cfg, 144, 40).View() }
		}
	}
	views["help popup"] = func(t *testing.T) string {
		m, _ := press(t, designAt(t, cfg, 144, 40), "?")
		return m.View()
	}
	views["chart title popup"] = func(t *testing.T) string {
		return designPopup(t, cfg, 144, 40, "chart-title").View()
	}
	views["save chart popup"] = func(t *testing.T) string {
		m, _ := designAt(t, cfg, 144, 40).Update(tea.KeyMsg{Type: tea.KeyCtrlS})
		return m.View()
	}
	seen := map[string]bool{}
	for name, view := range views {
		for _, title := range topTitles(view(t)) {
			seen[title] = true
			if v := sentenceCaseViolation(title); v != "" {
				t.Errorf("%s: %s", name, v)
			}
		}
	}
	for _, want := range []string{"Timeline", popupTitleKeys, popupTitleSaveChart, popupTitleChartTitle} {
		found := false
		for title := range seen {
			if strings.Contains(title, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("no title on screen contains %q, so the check did not see it; titles seen: %v", want, seen)
		}
	}
	if v := sentenceCaseViolation("[0] Details: Moon - timeline"); v == "" {
		t.Error("the checker passes a lower-case tab name")
	}
	if v := sentenceCaseViolation("[0] Details: moon"); v == "" {
		t.Error("the checker passes a lower-case subject")
	}
	if v := sentenceCaseViolation("[3] ① vs ②"); v != "" {
		t.Errorf("the checker refuses a numeral pair title: %s", v)
	}
	if v := sentenceCaseViolation("[3] 1 Vs 2"); v == "" {
		t.Error("the checker passes a capital inside a pair title")
	}
	if v := sentenceCaseViolation("[0] Details: Transit aspects - Timeline"); v != "" {
		t.Errorf("the checker refuses a valid multi-word subject: %s", v)
	}
}

// C2: a popup with no name is refused. No screen can show it, so it is
// checked on the box itself.
func TestModalBoxRefusesAnEmptyTitle(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("renderModalBox drew a popup with an empty title")
		}
	}()
	renderModalBox(modalBox{}, Config{}, 80, 24, func(w, h int) string { return "" })
}
