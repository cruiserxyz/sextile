package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// the specification contract lane: the shared row renderer (C2–C6) and
// what each of the four list modals puts in its rows (C7–C9). Test names match
// the spec's verifier column.

var plainCfg = Config{Accessible: true, Color: ColorNever}

func TestListRowWidth(t *testing.T) {
	lefts := []string{"Constructed One", "東京都千代田区丸の内一丁目", "A"}
	rights := [][]string{nil, {"Bremen, DE"}, {"Frankfurt am Main, DE", "20 Mar 1995"}, {"漢漢漢漢漢"}, {"東京, JP", "漢漢漢漢漢漢"}}
	for _, width := range []int{19, 20, 21, 54, 58, 86} {
		for _, left := range lefts {
			for _, right := range rights {
				for _, cursor := range []bool{false, true} {
					got := listRow(listRowSpec{Left: left, Right: right, Cursor: cursor}, plainCfg, width)
					if w := ansi.StringWidth(ansi.Strip(got)); w != width {
						t.Errorf("width %d, left %q, right %q: line is %d cells: %q", width, left, right, w, got)
					}
				}
			}
		}
	}
}

// unicodePlainCfg is colours off with Unicode glyphs, so a cut is marked "…";
// under --accessible it is "..." (text.go).
var unicodePlainCfg = Config{Color: ColorNever}

func TestListRowRightFlow(t *testing.T) {
	const width = 60
	got := listRow(listRowSpec{Left: "Constructed One", Right: []string{"Bremen, DE", "", "20 Mar 1995"}}, unicodePlainCfg, width)
	if !strings.HasSuffix(got, "Bremen, DE   20 Mar 1995") {
		t.Errorf("right cells do not end at the last cell, three spaces apart, with the empty one omitted: %q", got)
	}
	if !strings.HasPrefix(got, "  Constructed One   ") {
		t.Errorf("the left cell is not followed by at least three spaces: %q", got)
	}
	if strings.Contains(got, "Bremen, DE      20") {
		t.Errorf("an empty right cell left its gap behind: %q", got)
	}

	// A left cell that takes all the room it can still keeps three spaces.
	left := strings.Repeat("x", 60)
	got = listRow(listRowSpec{Left: left, Right: []string{"Bremen, DE"}}, unicodePlainCfg, width)
	if !strings.HasSuffix(got, "…   Bremen, DE") {
		t.Errorf("want the shortened left cell, three spaces, then the right cell at the edge: %q", got)
	}
}

func TestListRowNoRightCells(t *testing.T) {
	for _, right := range [][]string{nil, {""}, {"", ""}} {
		got := ansi.Strip(listRow(listRowSpec{Left: "Berlin", Right: right, Cursor: true}, plainCfg, 20))
		if want := "> Berlin" + strings.Repeat(" ", 12); got != want {
			t.Errorf("right %q: got %q, want the cursor column, the left cell and padding %q", right, got, want)
		}
	}
}

// Wide characters: a cut that would split one leaves a space instead, so the
// left cell still holds its 8-cell floor and the line its exact width.
func TestListRowShorteningWide(t *testing.T) {
	got := listRow(listRowSpec{Left: "漢漢漢漢漢", Right: []string{"RIGHT"}}, unicodePlainCfg, 18)
	if ansi.StringWidth(got) != 18 {
		t.Errorf("line is %d cells, want 18: %q", ansi.StringWidth(got), got)
	}
	if left := strings.TrimSuffix(got, "   RIGHT"); ansi.StringWidth(left) != 2+listRowLeftFloor || !strings.HasSuffix(left, "…") {
		t.Errorf("left cell %q is %d cells, want the cursor column and the %d-cell floor", left, ansi.StringWidth(left)-2, listRowLeftFloor)
	}
	// The floor is room the left cell keeps, not just cells it draws: at 20,
	// the right cell gets 18 - 8 - 3 = 7 cells, not the one a 7-cell left
	// cell would hand over.
	got = listRow(listRowSpec{Left: "漢漢漢漢漢", Right: []string{"RIGHTRIGHT"}}, unicodePlainCfg, 20)
	if !strings.HasSuffix(got, "   …TRIGHT") {
		t.Errorf("at 20 = %q, want the right cell held to 7 cells by the left cell's floor", got)
	}
	got = listRow(listRowSpec{Left: "abcdefgh", Right: []string{"漢漢漢漢漢"}}, unicodePlainCfg, 19)
	if ansi.StringWidth(got) != 19 || !strings.HasPrefix(got, "  abcdefgh   …") || !strings.HasSuffix(got, "漢") {
		t.Errorf("wide right cell at 19 = %q (%d cells), want the left cell whole and the right cell shortened from its left to the edge", got, ansi.StringWidth(got))
	}
}

func TestListRowShortening(t *testing.T) {
	left := "Constructed Subject Number One"
	place, date := "Frankfurt am Main, DE", "20 Mar 1995"
	cases := []struct {
		width     int
		wantLeft  string // the left cell as drawn
		wantRight string // the right block as drawn
	}{
		// 2 + 30 + 3 + 21 + 3 + 11 = 70: everything fits.
		{70, left, place + "   " + date},
		// 40: the left cell goes to its 8-cell floor, then the place gives up
		// the 8 cells still missing, from its left end.
		{40, "Constru…", "… am Main, DE   " + date},
		// 30: the place goes to its 4-cell floor, then the date gives up the
		// last cell.
		{30, "Constru…", "… DE   … Mar 1995"},
		// 20: even at their floors both cells do not fit, so the place —
		// first — is dropped and the row is laid out again without it.
		{20, "Constru…", "Constru…   …r 1995"},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.width), func(t *testing.T) {
			got := listRow(listRowSpec{Left: left, Right: []string{place, date}}, unicodePlainCfg, tc.width)
			if w := ansi.StringWidth(got); w != tc.width {
				t.Errorf("line is %d cells, want %d: %q", w, tc.width, got)
			}
			if !strings.HasPrefix(got, "  "+tc.wantLeft) {
				t.Errorf("left cell: got %q, want it to start %q", got, "  "+tc.wantLeft)
			}
			if !strings.HasSuffix(got, tc.wantRight) {
				t.Errorf("right block: got %q, want it to end %q", got, tc.wantRight)
			}
		})
	}
}

// sgrCells walks a rendered line and reports, for every display cell, the
// background colour in force when it was drawn: "5;N" for a palette colour,
// "2;R;G;B" for truecolor, "" for none.
func sgrCells(t *testing.T, s string) []string {
	t.Helper()
	var cells []string
	bg := ""
	for i := 0; i < len(s); {
		if strings.HasPrefix(s[i:], "\x1b[") {
			end := strings.IndexByte(s[i:], 'm')
			if end < 0 {
				t.Fatalf("unterminated SGR in %q", s)
			}
			params := strings.Split(s[i+2:i+end], ";")
			for k := 0; k < len(params); k++ {
				switch p := params[k]; {
				case p == "" || p == "0" || p == "49":
					bg = ""
				case p == "48" || p == "38":
					n := 0
					if k+1 < len(params) && params[k+1] == "5" {
						n = 2
					} else if k+1 < len(params) && params[k+1] == "2" {
						n = 4
					}
					if p == "48" && k+n < len(params) {
						bg = strings.Join(params[k+1:k+1+n], ";")
					}
					k += n
				case len(p) == 2 && p[0] == '4' && p[1] >= '0' && p[1] <= '7',
					len(p) == 3 && strings.HasPrefix(p, "10"):
					bg = p
				}
			}
			i += end + 1
			continue
		}
		r := []rune(s[i:])[0]
		for n := ansi.StringWidth(string(r)); n > 0; n-- {
			cells = append(cells, bg)
		}
		i += len(string(r))
	}
	return cells
}

// backgroundSGR is the background a colour renders as under the current
// profile, taken from lipgloss itself so a hex colour's conversion is
// lipgloss's to make. A renderer that drew any OTHER colour still differs.
func backgroundSGR(t *testing.T, c string) string {
	t.Helper()
	cells := sgrCells(t, lipgloss.NewStyle().Background(lipgloss.Color(c)).Render("x"))
	if len(cells) != 1 || cells[0] == "" {
		t.Fatalf("lipgloss renders no background for %q", c)
	}
	return cells[0]
}

// the specification C5, C7, C12: a list row's cursor is the band
// with colours on, and the marker plus reverse video with colours off. A row
// off the cursor carries no SGR at all.
func TestListRowCursor(t *testing.T) {
	forceColor(t)
	const width = 30
	on := listRow(listRowSpec{Left: "Berlin", Right: []string{"DE"}, Cursor: true}, Config{Color: ColorAlways}, width)
	if strings.Contains(on, "▶") || !strings.HasPrefix(ansi.Strip(on), "Berlin") {
		t.Errorf("colours on: the cursor row carries a marker: %q", ansi.Strip(on))
	}
	want := backgroundSGR(t, "4")
	cells := sgrCells(t, on)
	if len(cells) != width {
		t.Fatalf("%d cells, want %d: %q", len(cells), width, on)
	}
	for c, bg := range cells {
		if bg != want {
			t.Fatalf("colours on, cell %d: background %q, want the cursor-row band %q: %q", c, bg, want, on)
		}
	}
	for _, tc := range []struct {
		cfg  Config
		mark string
	}{{Config{Color: ColorNever}, "▶ Berlin"}, {Config{Accessible: true}, "> Berlin"}} {
		off := listRow(listRowSpec{Left: "Berlin", Right: []string{"DE"}, Cursor: true}, tc.cfg, width)
		if !strings.HasPrefix(ansi.Strip(off), tc.mark) || !strings.HasPrefix(off, cursorRowOff) {
			t.Errorf("%+v: want %q in reverse video, got %q", tc.cfg, tc.mark, off)
		}
		if got := listRow(listRowSpec{Left: "Berlin"}, tc.cfg, width); !strings.HasPrefix(got, "  Berlin") {
			t.Errorf("%+v: a row off the cursor must keep the blank cursor column: %q", tc.cfg, got)
		}
	}
	for _, cfg := range []Config{{Color: ColorAlways}, {Color: ColorNever}, {Accessible: true}} {
		if got := listRow(listRowSpec{Left: "Berlin", Right: []string{"DE"}}, cfg, width); strings.Contains(got, "\x1b[") {
			t.Errorf("%+v: a row off the cursor carries SGR: %q", cfg, got)
		}
	}
}

func TestChartPickerRowColumns(t *testing.T) {
	c := testNamedChart("Constructed One", fixtureOneP)
	c.Year, c.Month, c.Day = 1995, 3, 20
	got := listRow(chartRow(c, false, time.Time{}), plainCfg, 60)
	if !strings.HasPrefix(got, "  Constructed One ") || !strings.HasSuffix(got, "Test Bay, TS   Mar 20, 1995") {
		t.Errorf("chart row = %q, want the name left and `Test Bay, TS   Mar 20, 1995` at the right edge", got)
	}

	t.Run("no date", func(t *testing.T) {
		c := c
		c.Month, c.Day = 0, 0
		got := listRow(chartRow(c, false, time.Time{}), plainCfg, 60)
		if !strings.HasSuffix(got, "Test Bay, TS") || strings.Contains(got, "1995") {
			t.Errorf("a chart with no date must end on its place, with no date and no year: %q", got)
		}
	})

	// The date must come from the chart FILE, through the library and the
	// picker the operator actually opens — not from a hand-built row.
	t.Run("a .as read through the library", func(t *testing.T) {
		dir := fixtureLibrary(t)
		// A Julian leap day, near Bremen: a date astrolog accepts and Go's
		// calendar would roll into March.
		julian := "@AI780\n-qb Feb 29 1500 12:00pm ST 0W 8:48E 53:04N\n-zi \"Julian Leap\" \"Bremen\"\n"
		if err := os.WriteFile(filepath.Join(dir, "julian-leap.as"), []byte(julian), 0o644); err != nil {
			t.Fatal(err)
		}
		// A date that is not one: the chart still lists, with no date (C10b).
		bad := "@AI780\n-qb Feb 30 1995 12:00pm ST 0W 8:48E 53:04N\n-zi \"Bad Date\" \"Bremen\"\n"
		if err := os.WriteFile(filepath.Join(dir, "bad-date.as"), []byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}
		lib, err := lookup.LoadChartLibrary()
		if err != nil {
			t.Fatal(err)
		}
		st := chartPickerState{loaded: true, charts: lib.Charts}
		st.refilter()
		out := ansi.Strip(renderChartModal(st, plainCfg, 132, 40))
		// Equinox-2026-09.as: -qb Sep 23 2026 ... near Bremen.
		rowOf := func(name string) string {
			for _, line := range strings.Split(out, "\n") {
				if strings.Contains(line, name+" ") {
					return line
				}
			}
			return ""
		}
		for name, want := range map[string]string{
			"Julian Leap": "Feb 29, 1500",
		} {
			if r := rowOf(name); !strings.Contains(r, want) {
				t.Errorf("%s's row = %q, want it to show %q", name, r, want)
			}
		}
		if r := rowOf("Bad Date"); r == "" || strings.Contains(r, "1995") || !strings.HasSuffix(strings.TrimRight(r, " |│"), "Bremen, DE") {
			t.Errorf("Bad Date's row = %q, want it listed, ending on its place with no date", r)
		}
		row := rowOf("Equinox-2026-09")
		if row == "" {
			t.Fatalf("Equinox-2026-09 is not in the picker:\n%s", out)
		}
		if !strings.Contains(row, "Bremen, DE   Sep 23, 2026") {
			t.Errorf("Equinox-2026-09's row = %q, want its place and the date its file states, `Sep 23, 2026`", row)
		}
	})
}

// atlasRowState is a loaded, filtered place picker showing one place.
func atlasRowState(p location.PlaceInfo) atlasModalState {
	st := atlasModalState{index: []location.Indexed{}}
	st.filterText = "ber"
	st.setMatches([]location.PlaceInfo{p}, false)
	return st
}

func TestAtlasRowColumns(t *testing.T) {
	out := ansi.Strip(renderResultList(atlasRowState(berlinP), plainCfg, 40, 3))
	if first := strings.Split(out, "\n")[0]; first != "> Berlin"+strings.Repeat(" ", 30)+"DE" {
		t.Errorf("atlas row = %q, want the city left and the country code at the right edge", first)
	}

	t.Run("no country", func(t *testing.T) {
		p := berlinP
		p.Country = ""
		out := ansi.Strip(renderResultList(atlasRowState(p), plainCfg, 40, 3))
		if first := strings.Split(out, "\n")[0]; first != "> Berlin"+strings.Repeat(" ", 32) {
			t.Errorf("a city with no country must be the city alone: %q", first)
		}
	})
}

func TestAspectRowColumns(t *testing.T) {
	st := openAspectModal(testRegistry(), recipe.AspectPolicy{})
	sq := testRegistry()[3]
	for _, width := range []int{54, 70, 86} {
		got := listRow(aspectRow(st, sq, false, width), plainCfg, width)
		want := fmt.Sprintf("%-3s   %6.2f°   orb %4.1f", sq.Abbrev, sq.Angle, sq.Orb)
		if !strings.HasSuffix(got, want) {
			t.Errorf("width %d: aspect row = %q, want it to end %q — abbreviation, angle, orb, in that order", width, got, want)
		}
		if !strings.HasPrefix(got, "  [✓] "+sq.Name+" ") {
			t.Errorf("width %d: aspect row = %q, want the tick box and the name on the left", width, got)
		}
	}

	// C4b: below the modal's minimum the orb drops first, and only the name
	// ever shortens. The marker, abbreviation and angle stay whole.
	long := sq
	long.Name = "Sesquiquadrate-ish long"
	for _, width := range []int{30, 44} {
		got := ansi.Strip(listRow(aspectRow(st, long, true, width), plainCfg, width))
		if strings.Contains(got, "orb") {
			t.Errorf("width %d: the orb must drop first: %q", width, got)
		}
		if !strings.HasPrefix(got, "> [") || !strings.HasSuffix(got, fmt.Sprintf("%-3s   %6.2f°", long.Abbrev, long.Angle)) {
			t.Errorf("width %d: the marker, abbreviation and angle must stay whole: %q", width, got)
		}
		if ansi.StringWidth(got) != width {
			t.Errorf("width %d: line is %d cells", width, ansi.StringWidth(got))
		}
	}
}

func TestBodyRowColumns(t *testing.T) {
	st := openBodyTest(recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1}}, nil)
	o := aspect.RegistryObject{Number: 1, Name: "Sun"}
	spec := bodyRow(st, o, true)
	if len(spec.Right) != 0 {
		t.Errorf("a body row carries no right cell, got %q", spec.Right)
	}
	got := ansi.Strip(listRow(spec, plainCfg, 30))
	if want := "> [✓] Sun" + strings.Repeat(" ", 21); got != want {
		t.Errorf("body row = %q, want %q", got, want)
	}
}

// the specification C9 and its failure mode: the two display
// colours come from preferences.yaml. A usable value is taken silently;
// anything else is refused in one Log note naming the key and the value, and
// the default stands.
func TestDisplayColoursPreference(t *testing.T) {
	cases := []struct {
		name         string
		in           store.DisplayConfig
		focus, row   string
		loggedNaming string
	}{
		{"absent", store.DisplayConfig{}, "", "", ""},
		{"a palette number", store.DisplayConfig{FocusColour: "2", CursorRowColour: "30"}, "2", "30", ""},
		{"a hex colour", store.DisplayConfig{CursorRowColour: "#1f6f6f"}, "", "#1f6f6f", ""},
		{"out of the palette", store.DisplayConfig{FocusColour: "256"}, "", "", "display.focus_color \"256\""},
		{"a colour name", store.DisplayConfig{FocusColour: "grean"}, "", "", "display.focus_color \"grean\""},
		{"short hex", store.DisplayConfig{CursorRowColour: "#fff"}, "", "", "display.cursor_row_color \"#fff\""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel(Config{})
			before := len(m.logPanelLines())
			m = m.applyDisplayConfig(tc.in)
			if m.cfg.FocusColour != tc.focus || m.cfg.CursorRowColour != tc.row {
				t.Errorf("colours = %q, %q; want %q, %q", m.cfg.FocusColour, m.cfg.CursorRowColour, tc.focus, tc.row)
			}
			lines := m.logPanelLines()[before:]
			if tc.loggedNaming == "" && len(lines) > 0 {
				t.Errorf("a usable value wrote to the Log: %q", lines)
			}
			if tc.loggedNaming != "" && (len(lines) != 1 || !strings.Contains(lines[0], tc.loggedNaming)) {
				t.Errorf("want one Log note naming %s, got %q", tc.loggedNaming, lines)
			}
		})
	}
}

// the specification C17: a row_stripe entry, with or without a
// value, gets exactly one Log note saying it is retired and naming the key
// that now colours the cursor row, and startup carries on. Applying the same
// block again writes no second note.
func TestRowStripeRetiredNotedOnce(t *testing.T) {
	m, _ := applyStartupPerimeter(NewModel(Config{}), ingest.StartupPerimeterReadMsg{
		Display: store.DisplayConfig{RetiredStripePresent: true},
	})
	before := len(m.logPanelLines())
	m = m.applyDisplayConfig(store.DisplayConfig{RetiredStripePresent: true})
	if after := len(m.logPanelLines()); after != before {
		t.Errorf("a second application wrote %d more notes", after-before)
	}
	var notes []string
	for _, l := range m.logPanelLines() {
		if strings.Contains(l, "row_stripe") {
			notes = append(notes, l)
		}
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "no longer used") || !strings.Contains(notes[0], "display.cursor_row_color") {
		t.Errorf("want one note that row_stripe is no longer used and naming display.cursor_row_color, got %q", notes)
	}
}
