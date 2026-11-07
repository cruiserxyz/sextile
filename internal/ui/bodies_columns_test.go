package ui

import (
	"context"
	"fmt"
	"github.com/cruiserxyz/sextile/internal/ui/term"
	"math"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

var astrologStyleZero astrologstyle.BodyRow

// sunStyle is a body row as Astrolog colours one: a position colour the
// focused column must carry.
var sunStyle = astrologstyle.BodyRow{Body: "\x1b[1;33m", Position: "\x1b[0;33m", House: "\x1b[0;36m"}

// shiftedChart is fullChartPositions moved round the zodiac by shift degrees,
// so every chart of a fixture shows visibly different positions.
func shiftedChart(shift float64) []aspect.Position {
	ps := fullChartPositions()
	for i := range ps {
		ps[i].Longitude = math.Mod(ps[i].Longitude+shift, 360)
	}
	return ps
}

// shiftOf gives each chart of the fixture its own shift from its moment.
func shiftOf(when time.Time) float64 { return math.Mod(float64(when.Year()%360)*3.7, 360) }

// columnsModel is ① ② ③ + now selected, the cursor on cursor, its comparing
// frame landed through the real coordinator path (pump), with every chart's
// positions the full body list shifted differently.
func columnsModel(t *testing.T, cfg Config, cursor int) Model {
	t.Helper()
	freshModels(t)
	stubColumnLookups(t, nil)
	stubSpawns(t, func(s *rowSpawns) {
		s.cast.natal = func(_ context.Context, _ *lookup.Probers, c store.AnchorChart, _ lookup.Lens) (lookup.Result, error) {
			return lookup.Result{Positions: shiftedChart(shiftOf(c.When)), Commands: []string{"astrolog natal"}}, nil
		}
		s.cast.transit = func(_ context.Context, _ *lookup.Probers, when time.Time, _ location.PlaceInfo, _ lookup.Lens) (lookup.Result, error) {
			return lookup.Result{Positions: shiftedChart(shiftOf(when) + 11), Commands: []string{"astrolog transit"}}, nil
		}
	})
	m := settledComparing(t, cursor)
	m.cfg = cfg
	return m
}

// columnRowParts is what a row must read, written from the spec rather than
// from the renderer: for one body, each chart's 12 cells.
func expectedColumn(sep string, p aspect.Position) string {
	signIdx, deg, min := panel.SplitLongitude(p.Longitude)
	retro := " "
	if p.Retrograde() {
		retro = "℞"
	}
	// The ℞ (or a space) is the cell right after the minute mark.
	return fmt.Sprintf("%s%s %2d°%02d'%s", sep, panel.SignAbbrevString(signIdx), deg, min, retro)
}

func positionOf(ps []aspect.Position, b aspect.Object) aspect.Position {
	for _, p := range ps {
		if p.Object == b {
			return p
		}
	}
	return aspect.Position{}
}

// C1: a comparing row is the body, then one 12-cell column per shown chart in
// numeral order, then the cursor chart's speed bar and house frame; rows are
// matched by body identity, and no numeral stands in front of the row.
func TestBodiesRowHasOneColumnPerShownChart(t *testing.T) {
	m := columnsModel(t, Config{Color: ColorNever}, 1)
	cols, ok := m.bodyColumnsForScene()
	if !ok || len(cols) != 4 {
		t.Fatalf("fixture: columns = %d (ok=%v), want 4", len(cols), ok)
	}
	view := ansi.Strip(m.View())
	for _, b := range []aspect.Object{objSun, objSaturn, objMidheaven} {
		own := positionOf(m.chart.positions, b)
		// The cursor is ②: its column is bracketed, ③'s separating cell is the
		// closing bracket (colours off).
		want := fmt.Sprintf("%s %-9s", panel.PositionGlyph(own), panel.BodyName(b))
		for i, c := range cols {
			q := positionOf(c.Positions, b)
			if q.Object != b {
				t.Fatalf("fixture: chart %d lacks %v", i+1, b)
			}
			sep := " "
			switch i {
			case 1:
				sep = "["
			case 2:
				sep = "]"
			}
			want += expectedColumn(sep, q)
		}
		want += " " // the cell the closing bracket would take
		if !strings.Contains(view, want) {
			t.Errorf("no %v row reads %q:\n%s", b, want, view)
		}
	}

	// The speed bar is the cursor chart's, three cells (the ℞ sits in each
	// column), and an angle has none.
	sun := positionOf(m.chart.positions, objSun)
	row := panel.FormatColumnRow(sun, astrologStyleZero, cols, 1, panel.WholeWindow(len(cols)), m.cfg, panel.HouseOverlay{})
	bar := panel.DropFirstCell(panel.VelocityBar(sun.Speed, false, false))
	// body 11 + four columns of 12 + the closing cell + the bar + " " and the
	// 18-cell house frame: the ℞ cell is not repeated before the bar.
	if w := ansi.StringWidth(row); w != 11+4*panel.BodiesColumnWidth+1+3+1+18 {
		t.Errorf("Sun row is %d cells wide, want %d", w, 11+4*panel.BodiesColumnWidth+1+3+1+18)
	}
	if ansi.StringWidth(bar) != 3 || !strings.Contains(ansi.Strip(row), " "+bar) {
		t.Errorf("Sun row %q lacks the cursor chart's 3-cell bar %q", ansi.Strip(row), bar)
	}

	// Matched by body, never by position in the list: ③ lacks the Sun, so its
	// cells are blank and ④ still shows its own.
	partial := append([]panel.BodyColumn(nil), cols...)
	partial[2].Positions = []aspect.Position{positionOf(cols[2].Positions, objMoon)}
	got := ansi.Strip(panel.FormatColumnRow(sun, astrologStyleZero, partial, 1, panel.WholeWindow(len(partial)), m.cfg, panel.HouseOverlay{}))
	prefix := fmt.Sprintf("%s %-9s", panel.PositionGlyph(sun), panel.BodyName(objSun))
	wantPartial := prefix + expectedColumn(" ", positionOf(cols[0].Positions, objSun)) +
		expectedColumn("[", positionOf(cols[1].Positions, objSun)) +
		"]" + strings.Repeat(" ", panel.BodiesColumnWidth-1) +
		expectedColumn(" ", positionOf(cols[3].Positions, objSun)) + " "
	if !strings.HasPrefix(got, wantPartial) {
		t.Errorf("a chart without the Sun:\n got %q\nwant %q", got, wantPartial)
	}

	// Nothing stands in front of the rows: the first cell of the Bodies panel
	// row is the body's glyph, not a numeral.
	for _, line := range strings.Split(view, "\n") {
		if i := strings.Index(line, "Sun       "); i >= 0 && strings.Contains(line, "Sun       ") && strings.Contains(line, "°") {
			if before := strings.TrimLeft(line[:i], "│ "); strings.ContainsAny(before, "①②③④1234") {
				t.Errorf("a numeral stands in front of the Sun row: %q", line)
			}
		}
	}
}

// C3: with colours on the focused column is in Astrolog's position colour and
// every other column dimmed, with no brackets; with colours off the brackets take
// the separating cells either side of the focused column. In both modes no
// column starts anywhere else as focus moves.
func TestBodiesColumnsNeverMove(t *testing.T) {
	forceColor(t)
	m := columnsModel(t, Config{Color: ColorNever}, 1)
	cols, _ := m.bodyColumnsForScene()
	sun := positionOf(m.chart.positions, objSun)
	for _, mode := range []struct {
		name string
		cfg  Config
	}{{"colours on", Config{Color: ColorAlways}}, {"colours off", Config{Color: ColorNever}}} {
		var baseline []int
		for focus := range cols {
			raw := panel.FormatColumnRow(sun, sunStyle, cols, focus, panel.WholeWindow(len(cols)), mode.cfg, panel.HouseOverlay{})
			plain := ansi.Strip(raw)
			var starts []int
			for i, c := range cols {
				q := positionOf(c.Positions, objSun)
				signIdx, deg, min := panel.SplitLongitude(q.Longitude)
				cell := fmt.Sprintf("%s %2d°%02d'", panel.SignAbbrevString(signIdx), deg, min)
				at := strings.Index(plain, cell)
				if at < 0 {
					t.Fatalf("%s focus %d: column %d (%q) not in %q", mode.name, focus, i+1, cell, plain)
				}
				starts = append(starts, ansi.StringWidth(plain[:at]))
			}
			// The speed bar starts after the reserved closing cell.
			bar := panel.DropFirstCell(panel.VelocityBar(sun.Speed, false, false))
			barAt := strings.Index(plain, bar)
			if barAt < 0 {
				t.Fatalf("%s focus %d: no speed bar %q in %q", mode.name, focus, bar, plain)
			}
			starts = append(starts, ansi.StringWidth(plain[:barAt]))
			if focus == 0 {
				baseline = starts
				for i := range cols {
					if want := 11 + panel.BodiesColumnWidth*i + 1; starts[i] != want {
						t.Errorf("%s: column %d starts at %d, want %d (body 11 cells, 12 per column, one separator cell before the text)", mode.name, i+1, starts[i], want)
					}
				}
				if want := 11 + panel.BodiesColumnWidth*len(cols) + 1; starts[len(cols)] != want {
					t.Errorf("%s: the speed bar starts at %d, want %d (after the columns and the reserved closing cell)", mode.name, starts[len(cols)], want)
				}
			} else if fmt.Sprint(starts) != fmt.Sprint(baseline) {
				t.Errorf("%s: with focus on column %d the offsets are %v, with focus on column 1 %v", mode.name, focus+1, starts, baseline)
			}

			opens, closes := strings.Count(plain, "["), strings.Count(plain, "]")
			if mode.cfg.UseColor() {
				if opens+closes != 0 {
					t.Errorf("%s focus %d: brackets with colours on: %q", mode.name, focus+1, plain)
				}
				for i, c := range cols {
					style := columnStyle(t, raw, positionOf(c.Positions, objSun))
					faint := strings.Contains(style, term.SGRFaint)
					if faint == (i == focus) {
						t.Errorf("colours on focus %d: column %d faint=%v, want only the others faint", focus+1, i+1, faint)
					}
					want := strings.TrimSuffix(panel.RenderSGR(sunStyle.Position, "x"), "x"+term.SGRReset)
					if i == focus && !strings.Contains(style, want) {
						t.Errorf("colours on focus %d: the focused column opens with %q, want Astrolog's position colour %q", focus+1, style, want)
					}
				}
				continue
			}
			if opens != 1 || closes != 1 {
				t.Errorf("colours off focus %d: %d [ and %d ] in %q, want one each", focus+1, opens, closes, plain)
			}
			open := ansi.StringWidth(plain[:strings.Index(plain, "[")])
			closeAt := ansi.StringWidth(plain[:strings.Index(plain, "]")])
			if open != 11+panel.BodiesColumnWidth*focus || closeAt != open+panel.BodiesColumnWidth {
				t.Errorf("colours off focus %d: [ at %d and ] at %d, want %d and %d (the separating cells either side of the column)", focus+1, open, closeAt, 11+panel.BodiesColumnWidth*focus, 11+panel.BodiesColumnWidth*(focus+1))
			}
		}
	}
}

// columnStyle is the escape sequences that open column i of a coloured row:
// whatever stands between the previous reset and the column's own text.
func columnStyle(t *testing.T, raw string, p aspect.Position) string {
	t.Helper()
	signIdx, deg, min := panel.SplitLongitude(p.Longitude)
	cell := fmt.Sprintf("%s %2d°%02d'", panel.SignAbbrevString(signIdx), deg, min)
	at := strings.Index(raw, cell)
	if at < 0 {
		t.Fatalf("%q not in %q", cell, raw)
	}
	before := raw[:at]
	return before[strings.LastIndex(before, term.SGRReset)+1:]
}

// C4: alone, the row is exactly today's.
func TestBodiesAloneRowUnchanged(t *testing.T) {
	forceColor(t)
	ps := fullChartPositions()
	for _, overlay := range []bool{false, true} {
		ov := panel.HouseOverlay{}
		if overlay {
			ov = panel.HouseOverlay{Cusps: wholeSignLeoRising, Positions: shiftedChart(40)}
			if !ov.Active() {
				t.Fatal("fixture: the overlay is not active")
			}
		}
		for _, cfg := range []Config{{Color: ColorAlways}, {Color: ColorNever}} {
			state := chartViewState{positions: ps, natalCusps: ov.Cusps, natalPositions: ov.Positions}
			alone := bodiesView{shown: 1}
			got, _, _ := renderBodiesFor(alone, state, nil, 0, 0, 100, 40, false, cfg)
			want, _, _ := renderBodiesBody(state, nil, 0, 0, 100, 40, false, cfg)
			if got != want {
				t.Errorf("overlay=%v color=%v: the alone panel differs from renderBodiesBody", overlay, cfg.UseColor())
			}
			// And every row in it is formatPositionRowStyled's, byte for byte.
			planets, _ := panel.PartitionAngles(ps)
			for _, p := range planets[1:] { // the first row carries the cursor
				row := panel.FormatPositionRowStyled(p, state.bodyStyles[p.Object], cfg, ov)
				if !strings.Contains(got, row) {
					t.Errorf("overlay=%v color=%v: %v row is not today's %q", overlay, cfg.UseColor(), p.Object.Name, row)
				}
			}
		}
	}
}

// C9: a row wider than the panel clips at the panel edge and never wraps.
func TestBodiesColumnsClipNeverWrap(t *testing.T) {
	m := columnsModel(t, Config{Color: ColorNever}, 1)
	v := m.bodiesView()
	rows := len(m.chart.positions)
	for _, inner := range []int{12, 30, 47, 60, 100} {
		body, rule, _ := renderBodiesFor(v, m.chart, m.slots, m.cursorIdx, 0, inner, 40, false, m.cfg)
		lines := strings.Split(body, "\n")
		if rule < 0 || len(lines) != rows+1 {
			t.Errorf("inner width %d: %d lines (rule row %d), want the %d rows and the rule", inner, len(lines), rule, rows)
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w > inner {
				t.Errorf("inner width %d: line %d is %d cells wide: %q", inner, i, w, ansi.Strip(l))
			}
		}
	}
	// On the real screen: 80 columns cannot hold four columns, and every line
	// of the frame is still exactly the terminal's width.
	m.width, m.height = 80, 24
	screen := strings.Split(m.View(), "\n")
	if len(screen) != 24 {
		t.Errorf("the 80x24 frame has %d lines, want 24", len(screen))
	}
	for i, l := range screen {
		if w := ansi.StringWidth(l); w != 80 {
			t.Errorf("80x24 line %d is %d cells wide", i, w)
		}
	}
}

// C5: the left column is 52 + 12 per extra chart shown, and Details shows only
// when 80 more cells fit; otherwise the single column takes the terminal.
func TestLeftColumnGrowsByTwelve(t *testing.T) {
	for shown := 1; shown <= 6; shown++ {
		left := 52 + 12*(shown-1)
		for _, w := range []int{66, 80, 103, 104, 115, 116, 127, 128, 131, 132, 139, 140, 143, 144, 151, 152, 155, 156, 163, 164, 200} {
			rects := layoutGeometry(w, 40, 1, shown)
			if rects == nil {
				t.Fatalf("width %d: no layout", w)
			}
			details, hasDetails := rects[PanelDetails]
			if want := w >= left+80; hasDetails != want {
				t.Errorf("shown %d width %d: Details present = %v, want %v (left %d + 80)", shown, w, hasDetails, want, left)
			}
			wantLeft := w
			if hasDetails {
				wantLeft = left
				if details.x != left || details.w != w-left {
					t.Errorf("shown %d width %d: Details at x=%d w=%d, want x=%d w=%d", shown, w, details.x, details.w, left, w-left)
				}
				if rects[PanelLog].x != left || rects[PanelLog].w != w-left {
					t.Errorf("shown %d width %d: Log at x=%d w=%d, want x=%d w=%d", shown, w, rects[PanelLog].x, rects[PanelLog].w, left, w-left)
				}
				if c, _ := consoleRect(w, 40, 1, shown); c.x != left || c.w != w-left {
					t.Errorf("shown %d width %d: console at x=%d w=%d, want Details' x=%d w=%d", shown, w, c.x, c.w, left, w-left)
				}
				if got := hitTestPanel(w, 40, 1, shown, left, 0); got != PanelDetails {
					t.Errorf("shown %d width %d: a click on Details' first cell hits %q", shown, w, got)
				}
			}
			for _, id := range []PanelID{PanelChart, PanelBodies, PanelAspects} {
				if rects[id].w != wantLeft || rects[id].x != 0 {
					t.Errorf("shown %d width %d: %s at x=%d w=%d, want x=0 w=%d", shown, w, id, rects[id].x, rects[id].w, wantLeft)
				}
			}
		}
	}
}

// V1: Bodies with ① ② ③ and now selected, the cursor on ②, as whole screens.
// Frozen under the name the spec gives; a full screen because the columns' fit
// in the left column is the point.
func TestBodiesColumnsFrames(t *testing.T) {
	forceColor(t)
	for _, size := range []struct {
		name string
		w, h int
	}{{"80x24", 80, 24}, {"132x40", 132, 40}} {
		for _, mode := range []struct {
			name string
			cfg  Config
		}{{"color", Config{Color: ColorAlways}}, {"never", Config{Color: ColorNever}}} {
			m := columnsModel(t, mode.cfg, 1)
			m.width, m.height = size.w, size.h
			m.focused = PanelBodies
			writeDesignFrame(t, fmt.Sprintf("bodies-columns-%s-%s", size.name, mode.name), m.View())
		}
	}
}

// sixColumns is a comparing view of six charts, each with its own shift.
func sixColumns(focus int) (bodiesView, chartViewState) {
	state := chartViewState{positions: fullChartPositions(), cusps: fullChartCusps()}
	var cols []panel.BodyColumn
	for i := 0; i < 6; i++ {
		cols = append(cols, panel.BodyColumn{Positions: shiftedChart(float64(i) * 17), Cusps: fullChartCusps()})
	}
	return bodiesView{shown: 6, comparing: true, ok: true, cols: cols, focus: focus}, state
}

// When every column fits there is no window, no hint, and the row is the one
// the whole list draws.
func TestColumnWindowAllFitIsUnchanged(t *testing.T) {
	for n := 1; n <= 6; n++ {
		for focus := 0; focus < n; focus++ {
			avail := n * panel.BodiesColumnWidth
			lo, hi, hl, hr := panel.ColumnWindow(n, focus, avail, 0)
			if lo != 0 || hi != n || hl != 0 || hr != 0 {
				t.Errorf("n %d focus %d avail %d: window %d..%d hidden %d/%d, want all", n, focus, avail, lo, hi, hl, hr)
			}
		}
	}
	view, state := sixColumns(2)
	cfg := Config{Color: ColorNever}
	body, _, _ := renderBodiesFor(view, state, nil, 0, 0, 100+6*panel.BodiesColumnWidth, 40, false, cfg)
	sun := positionOf(state.positions, objSun)
	want := panel.FormatColumnRow(sun, astrologStyleZero, view.cols, 2, panel.WholeWindow(6), cfg, panel.HouseOverlay{})
	if !strings.Contains(body, strings.TrimRight(want, " ")) {
		t.Errorf("an all-fit Sun row differs from the whole-list row %q:\n%s", want, body)
	}
	if strings.ContainsAny(body, "‹›") {
		t.Errorf("an all-fit body carries a hint:\n%s", body)
	}
}

// The focused column is always inside the window.
func TestColumnWindowKeepsFocus(t *testing.T) {
	for n := 2; n <= 6; n++ {
		for focus := 0; focus < n; focus++ {
			for avail := 12; avail <= 80; avail++ {
				lo, hi, hl, hr := panel.ColumnWindow(n, focus, avail, 0)
				if lo > focus || focus >= hi {
					t.Errorf("n %d focus %d avail %d: window %d..%d lost the focus", n, focus, avail, lo, hi)
				}
				if hl != lo || hr != n-hi {
					t.Errorf("n %d focus %d avail %d: hidden %d/%d for window %d..%d", n, focus, avail, hl, hr, lo, hi)
				}
			}
		}
	}
}

// Columns and hints together fit, unless the focused column alone overflows.
func TestColumnWindowNeverHalf(t *testing.T) {
	for n := 2; n <= 6; n++ {
		for focus := 0; focus < n; focus++ {
			for avail := 12; avail <= 80; avail++ {
				lo, hi, hl, hr := panel.ColumnWindow(n, focus, avail, 0)
				used := (hi-lo)*panel.BodiesColumnWidth + panel.HintCost(hl, false) + panel.HintCost(hr, true)
				if used > avail && hi-lo > 1 {
					t.Errorf("n %d focus %d avail %d: window %d..%d uses %d cells", n, focus, avail, lo, hi, used)
				}
			}
		}
	}
}

// The hints count what is hidden, on every row, and only on a side that hides.
func TestBodiesHintsCountHiddenColumns(t *testing.T) {
	cfg := Config{Color: ColorNever}
	for _, focus := range []int{0, 2, 5} {
		view, state := sixColumns(focus)
		body, rule, _ := renderBodiesFor(view, state, nil, 0, 0, 78, 40, false, cfg)
		lines := strings.Split(body, "\n")
		_, hi, hl, hr := panel.ColumnWindow(6, focus, 78-panel.BodiesRowFixedWidth, 0)
		lo := hi - (6 - hl - hr)
		if hl+(hi-lo)+hr != 6 {
			t.Fatalf("focus %d: %d + %d + %d != 6", focus, hl, hi-lo, hr)
		}
		for i, l := range lines {
			if i == rule {
				continue
			}
			l = ansi.Strip(l)
			left := fmt.Sprintf("‹%d", hl)
			if (hl > 0) != strings.Contains(l, left) {
				t.Errorf("focus %d line %d: left hint %q present = %v, want %v: %q", focus, i, left, !(hl > 0), hl > 0, l)
			}
			right := fmt.Sprintf("%d›", hr)
			if (hr > 0) != strings.Contains(l, right) {
				t.Errorf("focus %d line %d: right hint %q present = %v, want %v: %q", focus, i, right, !(hr > 0), hr > 0, l)
			}
		}
		if focus == 0 && strings.Contains(body, "‹") {
			t.Errorf("focus 0 shows a left hint:\n%s", body)
		}
		if focus == 5 && strings.Contains(body, "›") {
			t.Errorf("focus 5 shows a right hint:\n%s", body)
		}
	}
	view, state := sixColumns(5)
	body, rule, _ := renderBodiesFor(view, state, nil, 0, 0, 78, 40, false, cfg)
	first := strings.Split(body, "\n")[0]
	if rule == 0 || !strings.Contains(ansi.Strip(first), "‹") {
		t.Errorf("focus 5: first row has no left hint: %q", first)
	}
}

// At 80x24 with six charts the focused chart's position is on screen for
// every focus, and no line is cut or padded off the terminal's width.
func TestBodiesFocusedColumnVisibleAt80x24(t *testing.T) {
	for focus := 0; focus < 6; focus++ {
		freshModels(t)
		stubColumnLookups(t, nil)
		stubSpawns(t, func(sp *rowSpawns) {
			sp.cast.natal = func(_ context.Context, _ *lookup.Probers, c store.AnchorChart, _ lookup.Lens) (lookup.Result, error) {
				return lookup.Result{Positions: shiftedChart(shiftOf(c.When)), Commands: []string{"astrolog natal"}}, nil
			}
			sp.cast.transit = func(_ context.Context, _ *lookup.Probers, when time.Time, _ location.PlaceInfo, _ lookup.Lens) (lookup.Result, error) {
				return lookup.Result{Positions: shiftedChart(shiftOf(when) + 11), Commands: []string{"astrolog transit"}}, nil
			}
		})
		m := selectedSet(t, 5, true, focus)
		m.cfg = Config{Color: ColorNever}
		m.focused = PanelChart
		m = m.noteArrival()
		m, cmd := m.afterComparisonChange()
		m = pump(t, m, cmd)
		cols, ok := m.bodyColumnsForScene()
		if !ok || len(cols) != 6 {
			t.Fatalf("focus %d: %d columns (ok %v), want 6", focus, len(cols), ok)
		}
		m.width, m.height = 80, 24
		screen := ansi.Strip(m.View())
		want := expectedColumn("", positionOf(cols[m.bodiesView().focus].Positions, objSun))
		if !strings.Contains(screen, strings.TrimRight(want, " ")) {
			t.Errorf("focus %d: the focused chart's Sun %q is not on the 80x24 screen:\n%s", focus, want, screen)
		}
		lines := strings.Split(m.View(), "\n")
		if len(lines) != 24 {
			t.Errorf("focus %d: %d lines, want 24", focus, len(lines))
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w != 80 {
				t.Errorf("focus %d: line %d is %d cells wide", focus, i, w)
			}
		}
	}
}

// avail41 holds three whole columns with both hints (36 + 2 + 3).
const avail41 = 3*panel.BodiesColumnWidth + 5

func TestColumnWindowEdgeTravelRight(t *testing.T) {
	for focus, want := range []int{0, 0, 0, 1, 2, 3} {
		if lo, _, _, _ := panel.ColumnWindow(6, focus, avail41, 0); lo != want {
			t.Errorf("focus %d from start 0: lo %d, want %d", focus, lo, want)
		}
	}
}

func TestColumnWindowEdgeTravelLeft(t *testing.T) {
	for _, c := range []struct{ focus, want int }{{5, 3}, {4, 3}, {3, 3}, {2, 2}, {0, 0}} {
		if lo, _, _, _ := panel.ColumnWindow(6, c.focus, avail41, 3); lo != c.want {
			t.Errorf("focus %d from start 3: lo %d, want %d", c.focus, lo, c.want)
		}
	}
	// A middle window (1..4, hints on both sides) is held while the focus is in
	// it; the right-end re-expansion cannot produce this.
	for _, c := range []struct{ focus, want int }{{3, 1}, {2, 1}, {1, 1}, {0, 0}} {
		if lo, _, _, _ := panel.ColumnWindow(6, c.focus, avail41, 1); lo != c.want {
			t.Errorf("focus %d from start 1: lo %d, want %d", c.focus, lo, c.want)
		}
	}
}

func TestColumnWindowAlwaysHoldsFocus(t *testing.T) {
	for n := 2; n <= 6; n++ {
		for focus := 0; focus < n; focus++ {
			for start := 0; start <= n; start++ {
				for avail := 12; avail <= 80; avail++ {
					lo, hi, hl, hr := panel.ColumnWindow(n, focus, avail, start)
					if lo > focus || focus >= hi || hl != lo || hr != n-hi {
						t.Fatalf("n %d focus %d start %d avail %d: window %d..%d hidden %d/%d", n, focus, start, avail, lo, hi, hl, hr)
					}
					used := (hi-lo)*panel.BodiesColumnWidth + panel.HintCost(hl, false) + panel.HintCost(hr, true)
					if used > avail && hi-lo > 1 {
						t.Fatalf("n %d focus %d start %d avail %d: window %d..%d uses %d cells", n, focus, start, avail, lo, hi, used)
					}
				}
			}
		}
	}
}

// sixChartModel is six selected charts with their comparing frame landed, at
// the given terminal size.
func sixChartModel(t *testing.T, w, h int) Model {
	t.Helper()
	stubColumnLookups(t, nil)
	m := selectedSet(t, 5, true, 0)
	m.cfg = Config{Color: ColorNever}
	m.focused = PanelChart
	m = m.noteArrival()
	m, cmd := m.afterComparisonChange()
	m = pump(t, m, cmd)
	return resizeTo(t, m, w, h)
}

func resizeTo(t *testing.T, m Model, w, h int) Model {
	t.Helper()
	tm, cmd := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return pump(t, tm.(Model), cmd)
}

// shownWindow is the Bodies panel's window as it draws: the first column and
// the hidden counts, read from the Sun row's hints.
func shownWindow(t *testing.T, m Model) (lo, hiddenR int) {
	t.Helper()
	inner := m.geometry()[PanelBodies].w - 2
	body, _, _ := renderBodiesFor(m.bodiesView(), m.chart, m.slots, m.cursorIdx, 0, inner, 40, false, m.cfg)
	for _, l := range strings.Split(ansi.Strip(body), "\n") {
		if !strings.Contains(l, "Sun") {
			continue
		}
		if i := strings.Index(l, "‹"); i >= 0 {
			fmt.Sscanf(l[i+len("‹"):], "%d", &lo)
		}
		if i := strings.Index(l, "›"); i >= 0 {
			j := i
			for j > 0 && l[j-1] >= '0' && l[j-1] <= '9' {
				j--
			}
			fmt.Sscanf(l[j:i], "%d", &hiddenR)
		}
		return lo, hiddenR
	}
	t.Fatalf("no Sun row:\n%s", body)
	return
}

func TestBodiesColumnsHoldUntilTheEdge(t *testing.T) {
	m := sixChartModel(t, 80, 24)
	step := func(k tea.KeyMsg) {
		var cmd tea.Cmd
		m, cmd = tpress(t, m, k)
		m = pump(t, m, cmd)
	}
	lo0, hr0 := shownWindow(t, m)
	if lo0 != 0 || hr0 == 0 {
		t.Fatalf("fixture: at chart 1 the window starts at %d with %d hidden right, want 0 and some", lo0, hr0)
	}
	shown := 6 - hr0
	for f := 1; f < shown; f++ {
		step(keyDown)
		if lo, _ := shownWindow(t, m); lo != 0 {
			t.Fatalf("focus %d (inside the window): first column %d, want 0", f, lo)
		}
	}
	step(keyDown)
	lo, hr := shownWindow(t, m)
	if lo != 1 || hr != hr0-1 {
		t.Fatalf("one step past the right edge: first column %d, %d hidden right, want 1 and %d", lo, hr, hr0-1)
	}
	for f := shown - 1; f >= 1; f-- {
		step(keyUp)
		if lo, _ := shownWindow(t, m); lo != 1 {
			t.Fatalf("walking back to focus %d: first column %d, want it held at 1", f, lo)
		}
	}
	step(keyUp)
	if lo, _ := shownWindow(t, m); lo != 0 {
		t.Fatalf("past the left edge: first column %d, want 0", lo)
	}
}

func TestBodiesColumnStartSurvivesResize(t *testing.T) {
	m := sixChartModel(t, 80, 24)
	for i := 0; i < 5; i++ {
		var cmd tea.Cmd
		m, cmd = tpress(t, m, keyDown)
		m = pump(t, m, cmd)
	}
	m = resizeTo(t, m, 132, 40)
	m = resizeTo(t, m, 80, 24)
	lo, hr := shownWindow(t, m)
	shown := 6 - hr - lo
	if focus := m.deriveScene().focus; focus < lo || focus >= lo+shown {
		t.Fatalf("after 132 and back to 80: focus %d outside shown columns %d..%d", focus, lo, lo+shown)
	}
}

// unnamedCusps are the points Astrolog prints that Sextile has no glyph for:
// a label each, at distinct longitudes (the parser's output for the three of
// them on arch, 2026-11-02).
func unnamedCusps() []aspect.Position {
	obj := func(n int) aspect.Object {
		o, err := fixtureObjectNumbered(n)
		if err != nil {
			panic(err)
		}
		return o
	}
	return []aspect.Position{
		{Object: obj(19), Longitude: 136.05},
		{Object: obj(20), Longitude: 200.5},
		{Object: obj(21), Longitude: 301.25},
	}
}

// Bug 20261102T1814Z: columnCell matched by the typed body alone, so every
// unnamed body answered with the first unnamed position's cell.
func TestColumnCellTellsUnnamedBodiesApart(t *testing.T) {
	cfg := Config{Color: ColorNever}
	col := panel.BodyColumn{Positions: unnamedCusps()}
	seen := map[string]string{}
	for _, p := range unnamedCusps() {
		cell := panel.ColumnCell(col, p, 0, 0, "", cfg)
		want := expectedColumn("[", p)
		if cell != want {
			t.Errorf("%s cell = %q, want %q", p.Object.Name, cell, want)
		}
		if other, dup := seen[cell]; dup {
			t.Errorf("%s and %s answer with the same cell %q", p.Object.Name, other, cell)
		}
		seen[cell] = p.Object.Name
	}
}

// The houses overlay looked the framed position up by the typed body alone as
// well.
func TestHouseOverlayTellsUnnamedBodiesApart(t *testing.T) {
	cusps := unnamedCusps()
	ov := panel.HouseOverlay{
		Cusps:     wholeSignLeoRising,
		Positions: []aspect.Position{{Object: cusps[0].Object, Longitude: 10, House: 9}, {Object: cusps[1].Object, Longitude: 40, House: 10}, {Object: cusps[2].Object, Longitude: 70, House: 11}},
		Framed:    cusps,
	}
	var lives, natals []aspect.House
	for _, p := range cusps {
		natal, live := ov.Houses(p)
		natals = append(natals, natal)
		lives = append(lives, live)
	}
	// 136.05 is Virgo (house 1 in Leo-rising whole sign is Leo: 120-150).
	wantLive := []aspect.House{wholeSignLeoRising.HouseFor(136.05), wholeSignLeoRising.HouseFor(200.5), wholeSignLeoRising.HouseFor(301.25)}
	for i := range cusps {
		if lives[i] != wantLive[i] {
			t.Errorf("%s live house = %d, want %d", cusps[i].Object.Name, lives[i], wantLive[i])
		}
	}
	if natals[0] != 9 || natals[1] != 10 || natals[2] != 11 {
		t.Errorf("natal houses = %v, want [9 10 11] (natalHouseOf has the same shape)", natals)
	}
}

// Panel level: two or more charts shown, the three unnamed points come from
// the parser with their own Objects, and each row reads its own degree in every column.
func TestBodiesColumnsShowUnnamedBodiesApart(t *testing.T) {
	stubColumnLookups(t, nil)
	withCusps := func(shift float64) []aspect.Position {
		ps := shiftedChart(shift)
		for _, u := range unnamedCusps() {
			u.Longitude = math.Mod(u.Longitude+shift, 360)
			ps = append(ps, u)
		}
		return ps
	}
	stubSpawns(t, func(s *rowSpawns) {
		s.cast.natal = func(_ context.Context, _ *lookup.Probers, c store.AnchorChart, _ lookup.Lens) (lookup.Result, error) {
			return lookup.Result{Positions: withCusps(shiftOf(c.When)), Commands: []string{"astrolog natal"}}, nil
		}
		s.cast.transit = func(_ context.Context, _ *lookup.Probers, when time.Time, _ location.PlaceInfo, _ lookup.Lens) (lookup.Result, error) {
			return lookup.Result{Positions: withCusps(shiftOf(when) + 11), Commands: []string{"astrolog transit"}}, nil
		}
	})
	m := settledComparing(t, 1)
	m.cfg = Config{Color: ColorNever}
	cols, _ := m.bodyColumnsForScene()
	view := ansi.Strip(m.View())
	for _, u := range unnamedCusps() {
		// The row names the object as the panel prints it: the full name, cut
		// to the nine-cell column and marked when it does not fit (East Poi…).
		printed := term.CutRight(u.Object.Name, 9, m.cfg)
		var line string
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, printed+" ") {
				line = l
				break
			}
		}
		if line == "" {
			t.Fatalf("no %s row in the panel:\n%s", printed, view)
		}
		for i, c := range cols {
			var q aspect.Position
			for _, p := range c.Positions {
				if p.Object == u.Object {
					q = p
				}
			}
			sep := " "
			switch i {
			case 1:
				sep = "["
			case 2:
				sep = "]"
			}
			if want := expectedColumn(sep, q); !strings.Contains(line, want) {
				t.Errorf("%s row, chart %d: want %q in %q", u.Object.Name, i+1, want, line)
			}
		}
	}
}
