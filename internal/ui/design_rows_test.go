package ui

// the specification: the cursor row, the selection marker, the
// two display colours and the freed status-bar row, each checked on the
// rendered Model.View() of the place the requirement names. A frame is read
// back cell by cell (designScreen), so a requirement about what the operator
// sees is checked against what the terminal would show.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// designCell is one terminal cell and the SGR state it was drawn in.
type designCell struct {
	r       rune
	fg, bg  string
	bold    bool
	reverse bool
}

type designScreen [][]designCell

// parseDesignScreen replays a View()'s SGR sequences into per-cell state.
func parseDesignScreen(s string) designScreen {
	var out designScreen
	var st designCell
	for _, line := range strings.Split(s, "\n") {
		var row []designCell
		for i := 0; i < len(line); {
			if line[i] == 0x1b && i+1 < len(line) && line[i+1] == '[' {
				j := i + 2
				for j < len(line) && (line[j] < 0x40 || line[j] > 0x7e) {
					j++
				}
				if j < len(line) && line[j] == 'm' {
					applyDesignSGR(&st, line[i+2:j])
				}
				i = j + 1
				continue
			}
			r := []rune(line[i:])[0]
			c := st
			c.r = r
			row = append(row, c)
			if ansi.StringWidth(string(r)) == 2 {
				f := st
				f.r = 0
				row = append(row, f)
			}
			i += len(string(r))
		}
		out = append(out, row)
	}
	return out
}

func applyDesignSGR(st *designCell, params string) {
	if params == "" {
		params = "0"
	}
	ps := strings.Split(params, ";")
	for i := 0; i < len(ps); i++ {
		n, _ := strconv.Atoi(ps[i])
		switch {
		case n == 0:
			*st = designCell{}
		case n == 1:
			st.bold = true
		case n == 22:
			st.bold = false
		case n == 7:
			st.reverse = true
		case n == 27:
			st.reverse = false
		case n == 39:
			st.fg = ""
		case n == 49:
			st.bg = ""
		case (n >= 30 && n <= 37) || (n >= 90 && n <= 97):
			st.fg = ps[i]
		case (n >= 40 && n <= 47) || (n >= 100 && n <= 107):
			st.bg = ps[i]
		case n == 38 || n == 48:
			k := 0
			if i+1 < len(ps) && ps[i+1] == "5" {
				k = 2
			} else if i+1 < len(ps) && ps[i+1] == "2" {
				k = 4
			}
			k = min(k, len(ps)-1-i)
			v := strings.Join(ps[i:i+k+1], ";")
			if n == 38 {
				st.fg = v
			} else {
				st.bg = v
			}
			i += k
		}
	}
}

func (s designScreen) text(y int) string {
	var b strings.Builder
	for _, c := range s[y] {
		if c.r != 0 {
			b.WriteRune(c.r)
		}
	}
	return b.String()
}

// rowsWhere lists the rows holding any cell that satisfies pred.
func (s designScreen) rowsWhere(pred func(designCell) bool) []int {
	var rows []int
	for y, row := range s {
		for _, c := range row {
			if pred(c) {
				rows = append(rows, y)
				break
			}
		}
	}
	return rows
}

// styleFG and styleBG are the SGR a colour renders as under the forced
// profile, as lipgloss writes it.
func styleFG(t *testing.T, c lipgloss.Color) string {
	t.Helper()
	s := parseDesignScreen(lipgloss.NewStyle().Foreground(c).Render("x"))
	if s[0][0].fg == "" {
		t.Fatalf("colour %q renders no foreground: is the profile forced?", c)
	}
	return s[0][0].fg
}

func styleBG(t *testing.T, c lipgloss.Color) string {
	t.Helper()
	s := parseDesignScreen(lipgloss.NewStyle().Background(c).Render("x"))
	if s[0][0].bg == "" {
		t.Fatalf("colour %q renders no background: is the profile forced?", c)
	}
	return s[0][0].bg
}

// assertFullWidthRun holds row y's run of pred cells to the cursor-row
// contract: contiguous, every cell bold, and filling the frame's inner width —
// exactly gap blank cells between it and each border: a popup's padding
// (popupRowGap), or none in a panel. It returns the run's first cell.
func assertFullWidthRun(t *testing.T, s designScreen, y int, pred func(designCell) bool, gap int, what string) designCell {
	t.Helper()
	row := s[y]
	a, b := -1, -1
	for x, c := range row {
		if pred(c) {
			if a < 0 {
				a = x
			}
			b = x
		}
	}
	for x := a; x <= b; x++ {
		if !pred(row[x]) {
			t.Fatalf("%s: the cursor row breaks at cell %d: %q", what, x, s.text(y))
		}
		if !row[x].bold {
			t.Fatalf("%s: cell %d of the cursor row is not bold: %q", what, x, s.text(y))
		}
	}
	blank := func(from, step int) int {
		n := 0
		for x := from; x >= 0 && x < len(row); x += step {
			if row[x].r != ' ' {
				return n
			}
			n++
		}
		return -1
	}
	left, right := blank(a-1, -1), blank(b+1, 1)
	// Past the gap is the frame itself: a border, or the scroll thumb on it.
	// Anything else means part of the row was left off the band.
	edge := func(x int) bool {
		return x >= 0 && x < len(row) && strings.ContainsRune("│|"+scrollThumbGlyph+scrollThumbASCII, row[x].r)
	}
	if left != gap || right != gap || !edge(a-1-gap) || !edge(b+1+gap) {
		t.Fatalf("%s: the cursor row does not fill its frame (gap %d left, %d right): %q", what, left, right, s.text(y))
	}
	return row[a]
}

// popupRowGap is modalBox's horizontal padding: a popup's list rows sit that
// far inside its border.
const popupRowGap = 2

// rowGap is the blank cells the list's cursor row leaves at each border.
func (l cursorList) rowGap() int {
	if l.popup {
		return popupRowGap
	}
	return 0
}

// cursorList is one list that shows a cursor row while it owns the keyboard.
// cursorText is text only the row under the model's own cursor holds, read
// from the list's state, so a band drawn on the wrong row is caught.
type cursorList struct {
	name       string
	model      func(t *testing.T, cfg Config) Model
	popup      bool
	cursorText func(t *testing.T, m Model) string
}

// prefixOf is the first n runes of s, trimmed: enough to recognise a row.
func prefixOf(s string, n int) string {
	r := []rune(strings.TrimSpace(ansi.Strip(s)))
	return strings.TrimSpace(string(r[:min(n, len(r))]))
}

// assertOnCursor holds the styled row to the model's cursor.
func assertOnCursor(t *testing.T, s designScreen, y int, l cursorList, m Model) {
	t.Helper()
	if want := l.cursorText(t, m); want == "" || !strings.Contains(s.text(y), want) {
		t.Fatalf("%s: the cursor row is not the model's cursor (want %q): %q", l.name, want, s.text(y))
	}
}

// designBase is a 132x40 ready workspace: the comparison pair, a full chart,
// thirty Log lines.
func designBase(t *testing.T, cfg Config) Model {
	t.Helper()
	return consoleTasteModel(t, cfg, 132, 40, 30)
}

func cursorLists() []cursorList {
	return []cursorList{
		{name: "chart slots", model: func(t *testing.T, cfg Config) Model {
			m := designBase(t, cfg)
			m.focused = PanelChart
			return m
		}, cursorText: func(t *testing.T, m Model) string {
			all := m.slots.All()
			if all[0].Time.Year() == all[1].Time.Year() {
				t.Fatal("fixture: the two slots share a year, so the year cannot tell their rows apart")
			}
			return fmt.Sprint(all[m.cursorIdx].Time.Year())
		}},
		{name: "bodies", model: func(t *testing.T, cfg Config) Model {
			m := designBase(t, cfg)
			m.focused = PanelBodies
			return m
		}, cursorText: func(t *testing.T, m Model) string {
			planets, _ := bodyRows(m.chart.positions, nil, false, houseOverlay{})
			return prefixOf(planets[m.bodyCursorIdx].text, 10)
		}},
		{name: "log console", model: func(t *testing.T, cfg Config) Model {
			m, _ := press(t, designBase(t, cfg), ",", "l")
			return m
		}, cursorText: func(t *testing.T, m Model) string {
			// The row as drawn: an argv line shows short (argv_display.go).
			return prefixOf(argvDisplay(m.console.snapshot[m.console.cursor].text, 0, m.cfg), 24)
		}},
		{name: "attached timeline", model: func(t *testing.T, cfg Config) Model {
			return onTimeline(t, cfg)
		}, cursorText: func(t *testing.T, m Model) string {
			// The row as drawn: the time column is 7 cells wide in every clock form.
			return prefixOf(alignTimelineRow(m.timelineRows()[m.details.cursor]), 24)
		}},
		{name: "city popup", popup: true, model: func(t *testing.T, cfg Config) Model {
			m := designBase(t, cfg)
			m.atlas = liftState("ber", 1)
			m.startup = startupAtlasOpen
			return m
		}, cursorText: func(t *testing.T, m Model) string {
			return m.atlas.matches[m.atlas.cursor].Name
		}},
		{name: "open chart popup", popup: true, model: func(t *testing.T, cfg Config) Model {
			m := designBase(t, cfg)
			m.picker = listRowsChartState()
			m.startup = startupChartOpen
			return m
		}, cursorText: func(t *testing.T, m Model) string {
			return m.picker.matches[m.picker.cursor].Name
		}},
		{name: "bodies popup", popup: true, model: func(t *testing.T, cfg Config) Model {
			m := designBase(t, cfg)
			m.bodyModal = listRowsBodyState()
			m.startup = startupBodyOpen
			return m
		}, cursorText: func(t *testing.T, m Model) string {
			return m.bodyModal.matches[m.bodyModal.cursor-groupRows()].Name
		}},
		{name: "aspects popup", popup: true, model: func(t *testing.T, cfg Config) Model {
			m := designBase(t, cfg)
			m.aspectModal = listRowsAspectState("")
			m.startup = startupAspectOpen
			return m
		}, cursorText: func(t *testing.T, m Model) string {
			if c := m.aspectModal.cursor; c < familyRows() {
				return aspect.Families()[c].Name()
			}
			return m.aspectModal.matches[m.aspectModal.cursor-familyRows()].Name
		}},
	}
}

// C5, C7 and the narrow-terminal failure mode: with colours on, the cursor row
// of each list carries the cursor-row background on every cell of its inner
// width, bold, and is the only row that does; no list popup draws ▶. C6: each
// cell keeps the foreground it has off the cursor, an Astrolog background
// included in the span loses to the band.
func TestCursorRowBand(t *testing.T) {
	forceColor(t)
	cfg := Config{Color: ColorAlways}
	band := styleBG(t, cursorRowBackground)
	onBand := func(c designCell) bool { return c.bg == band }
	for _, l := range cursorLists() {
		t.Run(l.name, func(t *testing.T) {
			m := l.model(t, cfg)
			view := m.View()
			s := parseDesignScreen(view)
			rows := s.rowsWhere(onBand)
			if len(rows) != 1 {
				t.Fatalf("C5: %d rows carry the cursor-row background, want exactly 1:\n%s", len(rows), ansi.Strip(view))
			}
			assertFullWidthRun(t, s, rows[0], onBand, l.rowGap(), "C5 "+l.name)
			assertOnCursor(t, s, rows[0], l, m)
			if l.popup && strings.Contains(ansi.Strip(view), "▶") {
				t.Errorf("C7: the %s draws ▶ with colours on:\n%s", l.name, ansi.Strip(view))
			}
		})
	}

	t.Run("80 columns", func(t *testing.T) {
		m := designBase(t, cfg)
		m.width, m.height = 80, 24
		m.focused = PanelBodies
		s := parseDesignScreen(m.View())
		rows := s.rowsWhere(onBand)
		if len(rows) != 1 {
			t.Fatalf("%d band rows at 80 columns", len(rows))
		}
		assertFullWidthRun(t, s, rows[0], onBand, 0, "80 columns")
	})

	t.Run("astrolog colours kept", func(t *testing.T) {
		styled := func(cursorOnSun bool) designScreen {
			m := designBase(t, cfg)
			m.focused = PanelBodies
			// An Astrolog span with its own background, the reset that ends
			// it, and the text after the reset.
			m.chart.bodyStyles = astrologstyle.Bodies{aspect.Sun: {
				Body:     "\x1b[91m",
				Position: "\x1b[41;33m",
				House:    "\x1b[36m",
			}}
			m.bodyCursorIdx = 1
			if cursorOnSun {
				m.bodyCursorIdx = 0
			}
			return parseDesignScreen(m.View())
		}
		on, off := styled(true), styled(false)
		sunRow := func(s designScreen) int {
			for y := range s {
				if strings.Contains(s.text(y), "☉ Sun") {
					return y
				}
			}
			t.Fatal("no Sun row on screen")
			return -1
		}
		y := sunRow(on)
		if y != sunRow(off) {
			t.Fatal("the Sun row moved between the two frames")
		}
		r := layoutGeometry(132, 40, 2, 1)[PanelBodies]
		spans := 0
		for x := r.x + 1; x < r.x+r.w-1; x++ {
			c, o := on[y][x], off[y][x]
			if c.fg != o.fg {
				t.Errorf("C6: cell %d (%q) is %q on the cursor row and %q off it", x, c.r, c.fg, o.fg)
			}
			if o.fg != "" {
				spans++
			}
			if c.bg != band {
				t.Errorf("C6: cell %d (%q) has background %q on the cursor row, want the band %q", x, c.r, c.bg, band)
			}
		}
		if spans == 0 {
			t.Fatal("fixture: the Sun row carries no Astrolog colour to keep")
		}
	})
}

// C12: with colours off the cursor row of each list opens on the cursor
// marker and is reverse video across its inner width, and it is the only
// reverse row.
func TestCursorRowColoursOff(t *testing.T) {
	isReverse := func(c designCell) bool { return c.reverse }
	for _, mode := range []struct {
		name string
		cfg  Config
		mark rune
	}{{"color-never", Config{Color: ColorNever}, '▶'}, {"accessible", Config{Accessible: true}, '>'}} {
		for _, l := range cursorLists() {
			t.Run(mode.name+"/"+l.name, func(t *testing.T) {
				m := l.model(t, mode.cfg)
				view := m.View()
				s := parseDesignScreen(view)
				rows := s.rowsWhere(isReverse)
				if len(rows) != 1 {
					t.Fatalf("%d reverse rows, want exactly 1:\n%s", len(rows), ansi.Strip(view))
				}
				first := assertFullWidthRun(t, s, rows[0], isReverse, l.rowGap(), "C12 "+l.name)
				assertOnCursor(t, s, rows[0], l, m)
				if first.r != mode.mark {
					t.Errorf("the cursor row opens on %q, want %q: %q", first.r, mode.mark, s.text(rows[0]))
				}
			})
		}
	}
}

// C16: Sextile adds no background to any row but the cursor row. Every list
// of C5 plus Aspects, the Details report and the Log, colours on, at every
// row index; the fixtures carry no Astrolog background.
func TestNoRowIsStriped(t *testing.T) {
	forceColor(t)
	cfg := Config{Color: ColorAlways}
	band := styleBG(t, cursorRowBackground)
	lists := cursorLists()
	lists = append(lists,
		cursorList{name: "aspects", model: func(t *testing.T, cfg Config) Model {
			m := designBase(t, cfg)
			m.focused = PanelAspects
			return m
		}},
		cursorList{name: "details report and log", model: func(t *testing.T, cfg Config) Model {
			return scrollTasteModel(t, cfg, 132, 40, 0, PanelChart)
		}},
	)
	for _, l := range lists {
		t.Run(l.name, func(t *testing.T) {
			s := parseDesignScreen(l.model(t, cfg).View())
			bandRows := s.rowsWhere(func(c designCell) bool { return c.bg == band })
			for y, row := range s {
				for x, c := range row {
					if c.bg == "" || (c.bg == band && len(bandRows) == 1 && bandRows[0] == y) {
						continue
					}
					t.Fatalf("row %d cell %d (%q) carries background %q: %q", y, x, c.r, c.bg, s.text(y))
				}
			}
		})
	}
}

// C10, C11, C11a: selection is a marker column and nothing else. Once a panel
// has a selection every row reserves the column; a selected row's marker is
// in the focus colour (colours on), ● (ColorNever) or * (--accessible);
// selection makes no cell bold; in Chart only the cursor row is bold.
//
// Changed by selected-charts-compare C14: on a CHART row the marker is the
// row's numeral — ① with colours on or off, `1` in ASCII mode — in place of the
// ●/*; Bodies keeps ●/*. The column, the focus colour and the no-bold rules are
// pinned exactly as before, for whichever glyph the panel uses.
func TestSelectionMarker(t *testing.T) {
	forceColor(t)
	focusFG := styleFG(t, focusColour)
	modes := []struct {
		name string
		cfg  Config
		mark rune
	}{
		{"colours-on", Config{Color: ColorAlways}, '●'},
		{"color-never", Config{Color: ColorNever}, '●'},
		{"accessible", Config{Accessible: true}, '*'},
	}
	type panelCase struct {
		panel PanelID
		setup func(m Model, selected bool) Model
		// rowText finds the row whose selection is toggled.
		rowText string
		// plainShift is how far the unselected render sits left of the
		// selected one: 2 when the panel then has no selection, so no marker
		// column; 0 when another row keeps the column.
		plainShift int
		// numeral is true where the selected row's marker is its numeral
		// (the Chart panel) rather than the mode's selection mark.
		numeral bool
	}
	panels := []panelCase{
		{PanelChart, func(m Model, selected bool) Model {
			// The pair is selected in the fixture; the cursor sits on the
			// floating slot, so the anchored row is selected and off-cursor.
			m.focused = PanelChart
			m.cursorIdx = 1
			if !selected {
				m.slots.SetComparisonSelected(false)
				m.slots.ToggleSlotSelection(1)
			}
			return m
		}, "1990", 0, true},
		{PanelBodies, func(m Model, selected bool) Model {
			m.focused = PanelBodies
			m.bodyCursorIdx = 0
			if selected {
				m.slots.ToggleBodySelection(m.cursorIdx, aspect.Mars)
			}
			return m
		}, "♂ Mars", 2, false},
	}
	for _, mode := range modes {
		for _, pc := range panels {
			t.Run(fmt.Sprintf("%s/%s", mode.name, pc.panel), func(t *testing.T) {
				// The glyph this panel's selected row carries in this mode.
				marks := map[rune]bool{mode.mark: true}
				want := mode.mark
				if pc.numeral {
					want = '①'
					if mode.cfg.useASCII() {
						want = '1'
					}
					// Every selected Chart row carries its own numeral.
					marks = map[rune]bool{}
					for _, n := range []string{"①", "②", "③", "④", "⑤", "⑥", "1", "2", "3", "4", "5", "6"} {
						if mode.cfg.useASCII() == (len(n) == 1) {
							marks[[]rune(n)[0]] = true
						}
					}
				}
				render := func(selected bool) designScreen {
					return parseDesignScreen(pc.setup(designBase(t, mode.cfg), selected).View())
				}
				sel, plain := render(true), render(false)
				r := layoutGeometry(132, 40, 2, 1)[pc.panel]
				find := func(s designScreen) int {
					for y := r.y + 1; y < r.y+r.h-1; y++ {
						if strings.Contains(s.text(y), pc.rowText) {
							return y
						}
					}
					t.Fatalf("fixture: no %q row in %s", pc.rowText, pc.panel)
					return -1
				}
				cursorCol := 0
				if !mode.cfg.useColor() {
					cursorCol = 2
				}
				markX := r.x + 1 + cursorCol

				// C11: every row reserves the marker column.
				for y := r.y + 1; y < r.y+r.h-1; y++ {
					inner := strings.TrimRight(ansi.Cut(sel.text(y), r.x+1, r.x+r.w-1), " ")
					if inner == "" || strings.ContainsRune(inner, '┈') || strings.ContainsRune(inner, '-') && strings.Trim(inner, "-") == "" {
						continue
					}
					c := sel[y][markX]
					next := sel[y][markX+1]
					if (!marks[c.r] && c.r != ' ') || next.r != ' ' {
						t.Errorf("C11: row %d does not reserve the marker column: %q", y, inner)
					}
				}

				y := find(sel)
				if c := sel[y][markX]; c.r != want {
					t.Fatalf("C11: the selected row's marker is %q, want %q: %q", c.r, want, sel.text(y))
				} else if mode.cfg.useColor() && c.fg != focusFG {
					t.Errorf("C11: the marker is in %q, want the focus colour %q", c.fg, focusFG)
				}

				// C11a: selection adds no bold anywhere in the row.
				bolds := func(s designScreen, y int) int {
					n := 0
					for x := r.x + 1; x < r.x+r.w-1; x++ {
						if s[y][x].bold {
							n++
						}
					}
					return n
				}
				// Cell by cell, aligned for the marker column: the marker
				// itself is not bold, and every other cell is bold exactly
				// where it is without the selection.
				yp := find(plain)
				for x := r.x + 1; x < r.x+r.w-1; x++ {
					c := sel[y][x]
					switch {
					case x == markX || x == markX+1:
						if c.bold {
							t.Errorf("C11a: the marker cell %d (%q) is bold: %q", x, c.r, sel.text(y))
						}
					case x < markX:
						if c.bold != plain[yp][x].bold {
							t.Errorf("C11a: cell %d (%q) bold=%v selected, %v not: %q", x, c.r, c.bold, plain[yp][x].bold, sel.text(y))
						}
					default:
						if p := plain[yp][x-pc.plainShift]; c.bold != p.bold {
							t.Errorf("C11a: cell %d (%q) bold=%v selected, %v not: %q", x, c.r, c.bold, p.bold, sel.text(y))
						}
					}
				}

				// C10: in Chart only the cursor row is bold.
				if pc.panel == PanelChart && mode.cfg.useColor() {
					for yy := r.y + 1; yy < r.y+r.h-1; yy++ {
						onCursor := false
						for x := r.x + 1; x < r.x+r.w-1; x++ {
							onCursor = onCursor || sel[yy][x].bg != ""
						}
						if !onCursor && bolds(sel, yy) > 0 {
							t.Errorf("C10: a Chart row off the cursor is bold: %q", sel.text(yy))
						}
					}
				}
			})
		}
	}
}

// C8: the defaults are terminal colour 10 for focus and 4 for the cursor row.
func TestThemeDefaults(t *testing.T) {
	if focusColour != "10" || focusColourFor(Config{}) != "10" {
		t.Errorf("focus colour = %q / %q, want 10", focusColour, focusColourFor(Config{}))
	}
	if cursorRowBackground != "4" || cursorRowColourFor(Config{}) != "4" {
		t.Errorf("cursor-row colour = %q / %q, want 4", cursorRowBackground, cursorRowColourFor(Config{}))
	}
}

// C9: display.focus_color and display.cursor_row_color, written to
// preferences.yaml in either form, reach the screen through the real startup
// read.
func TestDisplayColoursReachTheScreen(t *testing.T) {
	forceColor(t)
	for _, tc := range []struct{ focus, row string }{
		{"13", `"#1f6f6f"`},
		{`"#ff8800"`, "30"},
	} {
		t.Run(tc.focus+"/"+tc.row, func(t *testing.T) {
			path := store.PreferencesPath()
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			text := "display:\n  focus_color: " + tc.focus + "\n  cursor_row_color: " + tc.row + "\n"
			if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Remove(path) })

			perimeter := ingest.ReadStartupPerimeter()().(ingest.StartupPerimeterReadMsg)
			perimeter.LocationCandidate = location.Candidate{Source: "stored-location", Place: location.RawPlace{
				Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin",
				LatPresent: true, LonPresent: true,
			}}
			perimeter.HasSession, perimeter.Session = false, store.Session{}
			var m tea.Model = NewModelWithClock(Config{Color: ColorAlways}, FrozenClock(testFrozenTime()))
			for _, msg := range []tea.Msg{
				tea.WindowSizeMsg{Width: 132, Height: 40},
				AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"}})},
				perimeter,
				FirstFrameReadyMsg{Positions: fullChartPositions(), RunID: 1, WantID: 1},
			} {
				m, _ = m.Update(msg)
			}
			s := parseDesignScreen(m.View())
			unquote := func(v string) lipgloss.Color { return lipgloss.Color(strings.Trim(v, `"`)) }
			if got, want := s[0][0].fg, styleFG(t, unquote(tc.focus)); got != want {
				t.Errorf("the focused frame is drawn in %q, want display.focus_color %s as %q:\n%s", got, tc.focus, want, ansi.Strip(m.View()))
			}
			band := styleBG(t, unquote(tc.row))
			if rows := s.rowsWhere(func(c designCell) bool { return c.bg == band }); len(rows) != 1 {
				t.Errorf("%d rows carry display.cursor_row_color %s as %q, want the one cursor row:\n%s", len(rows), tc.row, band, ansi.Strip(m.View()))
			}
		})
	}
}

// C15, C15a: no row goes to a status bar. The top panel starts on row 0, the
// panels and the console use every row but the key bar, the rendered frame is
// exactly the terminal's height with a panel border on row 0, and the minimum
// height is 15.
func TestNoStatusBar(t *testing.T) {
	if MinTerminalHeight != 15 {
		t.Errorf("MinTerminalHeight = %d, want 15", MinTerminalHeight)
	}
	for _, size := range [][2]int{{80, 24}, {132, 40}, {80, MinTerminalHeight}} {
		w, h := size[0], size[1]
		t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
			rects := layoutGeometry(w, h, 1, 1)
			if rects == nil {
				t.Fatalf("no layout at %dx%d", w, h)
			}
			if r := rects[PanelChart]; r.y != 0 {
				t.Errorf("the top panel starts on row %d, want 0", r.y)
			}
			left := rects[PanelChart].h + rects[PanelBodies].h + rects[PanelAspects].h
			if left != h-KeyBarHeight {
				t.Errorf("the left column uses %d rows, want %d", left, h-KeyBarHeight)
			}
			if d, ok := rects[PanelDetails]; ok {
				if d.y != 0 || d.h+rects[PanelLog].h != h-KeyBarHeight {
					t.Errorf("the right column is %+v + %+v, want rows 0..%d", d, rects[PanelLog], h-KeyBarHeight-1)
				}
			}
			if c, ok := consoleRect(w, h, 1, 1); !ok || c.y != 0 || c.h != h-KeyBarHeight {
				t.Errorf("console rect = %+v, want rows 0..%d", c, h-KeyBarHeight-1)
			}
			m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTime()))
			m.width, m.height = w, h
			view := ansi.Strip(m.View())
			lines := strings.Split(view, "\n")
			if len(lines) != h {
				t.Errorf("View() is %d rows, want %d", len(lines), h)
			}
			if !strings.HasPrefix(lines[0], "+-") {
				t.Errorf("row 0 is not a panel border: %q", lines[0])
			}
		})
	}
	t.Run("one row under the minimum", func(t *testing.T) {
		m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTime()))
		m.width, m.height = 80, MinTerminalHeight-1
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "Terminal too small") || !strings.Contains(view, "66x15") {
			t.Errorf("at %d rows want the too-small screen naming 66x15:\n%s", MinTerminalHeight-1, view)
		}
	})
}
