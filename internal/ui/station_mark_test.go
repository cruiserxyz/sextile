package ui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// the specification — a slow, direct body's speed lane reads S/R or
// S/D instead of an empty bar, computed live from Astrolog for the focused
// chart.

type stationMarksFunc = func(context.Context, *lookup.Probers, []aspect.Position, time.Time, location.PlaceInfo, lookup.Lens) (map[aspect.Body]lookup.Station, []string, error)

// installStationMarks replaces the station seam for one test; the package
// default (main_test.go) makes no mark and no spawn.
func installStationMarks(t *testing.T, fn stationMarksFunc) {
	t.Helper()
	old := stationMarks
	t.Cleanup(func() { stationMarks = old })
	stationMarks = fn
}

// mercurySlow is the position the floating chart's stub returns while Mercury
// is slow and direct; the natal stub's Mercury is fast.
var mercurySlow = aspect.Position{Body: aspect.Mercury, Longitude: 65.8667, Speed: 0.02, House: 8}

func stationLookups(t *testing.T) {
	t.Helper()
	stubColumnLookups(t, nil)
	firstCallTransits = func(context.Context, *lookup.Probers, time.Time, location.PlaceInfo, lookup.Lens) (lookup.Result, error) {
		return lookup.Result{Positions: []aspect.Position{{Body: aspect.Sun, Longitude: 84.2167, Speed: 0.97, House: 8}, mercurySlow}, Commands: []string{"astrolog transit"}}, nil
	}
	firstCallNatalPositions = func(context.Context, *lookup.Probers, store.AnchorChart, lookup.Lens) (lookup.Result, error) {
		return lookup.Result{Positions: []aspect.Position{{Body: aspect.Sun, Longitude: 10, Speed: 0.97}, {Body: aspect.Mercury, Longitude: 20, Speed: 0.5}}, Commands: []string{"astrolog natal"}}, nil
	}
}

// markMercury is a probe stub: Mercury at +0.020 gets S/R, anything else
// nothing; it counts its runs and carries its own command.
func markMercury(runs *int) stationMarksFunc {
	return func(_ context.Context, _ *lookup.Probers, ps []aspect.Position, _ time.Time, _ location.PlaceInfo, _ lookup.Lens) (map[aspect.Body]lookup.Station, []string, error) {
		for _, p := range ps {
			if p.Body == aspect.Mercury && p.Speed == mercurySlow.Speed {
				*runs++
				return map[aspect.Body]lookup.Station{aspect.Mercury: lookup.StationRetro}, []string{"astrolog station probe"}, nil
			}
		}
		return nil, nil, nil
	}
}

// N3 / C4: the marks land in the same commit as the positions.
func TestStationMarkLandsWithItsPositions(t *testing.T) {
	stationLookups(t)
	runs := 0
	installStationMarks(t, markMercury(&runs))

	// One message carries both.
	msg := computeChartCmd(context.Background(), 1, 1, nil, testFrozenTime(), testPlaceBerlin(), lookup.Hellenistic)().(FirstFrameReadyMsg)
	if len(msg.Positions) == 0 || msg.Stations[aspect.Mercury] != lookup.StationRetro {
		t.Fatalf("the compute's message carries positions %v and marks %v; want both", msg.Positions, msg.Stations)
	}

	// Through the coordinator: no frame shows the Mercury row without its mark.
	m := settledComparing(t, 3) // the floating slot
	if m.chart.stations[aspect.Mercury] != lookup.StationRetro {
		t.Fatalf("the committed frame lacks the mark: %v", m.chart.stations)
	}
	m, cmd := tpress(t, m, runes("d"))
	m, views := pumpViews(t, m, cmd)
	if len(views) == 0 {
		t.Fatal("no view recorded")
	}
	sawRow := false
	for i, v := range views {
		plain := ansi.Strip(v)
		if strings.Contains(plain, "Mercury") {
			sawRow = true
			if !strings.Contains(plain, "S/R") {
				t.Errorf("view %d of %d shows Mercury without its mark:\n%s", i+1, len(views), plain)
			}
		}
	}
	if !sawRow {
		t.Fatal("fixture: no view showed the Mercury row")
	}

	// A focus move to a chart without a mark never shows the previous chart's
	// once its frame lands.
	mercuryRow := func(view string) string {
		bodies, _ := leftPanels(t, view)
		for _, l := range bodies {
			if plain := strings.TrimRight(ansi.Strip(l), " │"); strings.Contains(plain, "Mercury") {
				return plain
			}
		}
		return ""
	}
	markedRow := mercuryRow(m.View())
	if !strings.Contains(markedRow, "S/R") {
		t.Fatalf("fixture: the marked chart's Mercury row is %q", markedRow)
	}
	m, cmd = tpress(t, m, keyUp)
	if len(m.chart.stations) != 0 {
		t.Errorf("the focus move left the previous chart's marks %v on the live frame", m.chart.stations)
	}
	// Until the new frame lands the held frame (holdShown) paints the previous
	// chart's rows with the previous chart's marks, as one scene: the exact row.
	if held := mercuryRow(m.View()); held != markedRow {
		t.Errorf("the pending frame's Mercury row is %q, want the previous chart's %q", held, markedRow)
	}
	m, _ = pumpViews(t, m, cmd)
	if len(m.chart.stations) != 0 {
		t.Errorf("after moving to the natal chart the frame still carries marks %v", m.chart.stations)
	}
	if after := mercuryRow(m.View()); after == markedRow {
		t.Errorf("the new frame still paints the previous chart's row %q", after)
	}
	if strings.Contains(ansi.Strip(m.View()), "S/") {
		t.Errorf("the newly focused chart shows a mark:\n%s", ansi.Strip(m.View()))
	}
}

// C5: a failing probe costs the marks only; one note per failed run.
func TestStationProbeFailureKeepsThePositions(t *testing.T) {
	stationLookups(t)
	installStationMarks(t, func(context.Context, *lookup.Probers, []aspect.Position, time.Time, location.PlaceInfo, lookup.Lens) (map[aspect.Body]lookup.Station, []string, error) {
		return nil, []string{"astrolog station probe"}, errors.New("boom")
	})
	notes := func(m Model) int {
		n := 0
		for _, l := range m.logLines {
			if l.lane == laneNote && strings.Contains(l.text, "station marks unavailable") {
				n++
			}
		}
		return n
	}
	m := settledComparing(t, 3)
	if len(m.chart.positions) == 0 || len(m.chart.stations) != 0 {
		t.Fatalf("positions %d marks %v; want the positions without marks", len(m.chart.positions), m.chart.stations)
	}
	if n := notes(m); n != 1 {
		t.Fatalf("%d notes after the first failed run, want 1", n)
	}
	m, cmd := tpress(t, m, runes("d"))
	m = pump(t, m, cmd)
	if n := notes(m); n != 2 {
		t.Errorf("%d notes after a second failed run, want 2", n)
	}
	if len(m.chart.positions) == 0 {
		t.Error("the second failed run lost the positions")
	}
}

// C6: the probe's argv is in the Log's argv lane, after the frame's own.
func TestStationProbeArgvIsLogged(t *testing.T) {
	stationLookups(t)
	runs := 0
	installStationMarks(t, markMercury(&runs))
	m := settledComparing(t, 3)
	probe, transit := -1, -1
	n := 0
	for i, l := range m.logLines {
		if l.lane != laneArgv {
			continue
		}
		switch l.text {
		case "astrolog station probe":
			probe = i
			n++
		case "astrolog transit":
			transit = i
		}
	}
	if n != 1 {
		t.Fatalf("the probe's command is in the argv lane %d times, want once", n)
	}
	if transit < 0 || probe < transit {
		t.Errorf("the probe's line (%d) must follow the frame's own transit command (%d)", probe, transit)
	}
}

func stationRows() []aspect.Position {
	return []aspect.Position{
		{Body: aspect.Sun, Longitude: 84.2167, Speed: 0.97, House: 8},
		{Body: aspect.Mercury, Longitude: 65.8667, Speed: 0.02, House: 8},
		{Body: aspect.Moon, Longitude: 346.75, Speed: 1.016, House: 5},
		{Body: aspect.Venus, Longitude: 48.9, Speed: 0.0, House: 7},
		{Body: aspect.Mars, Longitude: 11.1, Speed: 1.371, House: 6},
		{Body: aspect.Ascendant, Longitude: 210.8667, Speed: 0.689, House: 1},
	}
}

var stationMarksTwo = map[aspect.Body]lookup.Station{aspect.Mercury: lookup.StationRetro, aspect.Venus: lookup.StationDirect}

// C7: S/R and S/D take the speed lane of every marked row, in both layouts,
// whatever row the cursor is on.
func TestBodiesPrintsTheStationMark(t *testing.T) {
	cfg := Config{Color: ColorNever}
	rows := func(text string) map[string]string {
		out := map[string]string{}
		for _, l := range strings.Split(text, "\n") {
			for _, name := range []string{"Sun", "Mercury", "Moon", "Venus", "Mars"} {
				if strings.Contains(l, " "+name) {
					out[name] = ansi.Strip(l)
				}
			}
		}
		return out
	}

	// Single chart: the cell after the minute is blank, then the three cells.
	state := chartViewState{positions: stationRows(), stations: stationMarksTwo}
	body, _, _ := renderBodiesBody(state, nil, 0, 2, 80, 20, false, cfg) // the cursor is on neither marked row
	got := rows(body)
	for name, want := range map[string]string{"Mercury": "S/R", "Venus": "S/D"} {
		if !strings.Contains(got[name], "'  "+want) {
			t.Errorf("single: %s row %q lacks %q in its speed lane", name, got[name], want)
		}
	}
	for _, name := range []string{"Sun", "Moon", "Mars"} {
		if strings.Contains(got[name], "S/") {
			t.Errorf("single: unmarked %s row carries a mark: %q", name, got[name])
		}
	}
	if strings.Count(ansi.Strip(body), "S/") != 2 {
		t.Errorf("single: %d marks in the body, want exactly 2:\n%s", strings.Count(ansi.Strip(body), "S/"), body)
	}

	// An angle never carries one, even if a mark names it.
	angled := chartViewState{positions: stationRows(), stations: map[aspect.Body]lookup.Station{aspect.Ascendant: lookup.StationRetro}}
	if b, _, _ := renderBodiesBody(angled, nil, 0, 0, 80, 20, false, cfg); strings.Contains(b, "S/") {
		t.Errorf("an angle row carries a mark:\n%s", b)
	}

	// Comparing: two charts' columns, the speed lane is the focused chart's.
	cols := []bodyColumn{{positions: stationRows()}, {positions: stationRows()}}
	cstate := chartViewState{positions: stationRows(), stations: stationMarksTwo}
	view := bodiesView{comparing: true, ok: true, cols: cols, focus: 1}
	cbody, _, _ := renderBodiesFor(view, cstate, nil, 0, 2, 100, 20, false, cfg)
	cgot := rows(cbody)
	for name, want := range map[string]string{"Mercury": "S/R", "Venus": "S/D"} {
		line := cgot[name]
		if !strings.Contains(line, want) {
			t.Errorf("columns: %s row %q lacks %q", name, line, want)
		}
		// Only the speed lane carries it: cut the columns' cells away.
		cells := 0
		for _, c := range cols {
			cells += strings.Count(ansi.Strip(columnCell(c, aspect.Mercury, 0, 0, "", cfg)), "S/")
		}
		if cells != 0 {
			t.Errorf("columns: a chart column cell carries %q", want)
		}
		if i := strings.Index(line, want); i < 11+len(cols)*bodiesColumnWidth {
			t.Errorf("columns: %s mark at %d is inside the columns (%d..%d)", name, i, 11, 11+len(cols)*bodiesColumnWidth)
		}
	}
	if strings.Count(ansi.Strip(cbody), "S/") != 2 {
		t.Errorf("columns: %d marks, want exactly 2:\n%s", strings.Count(ansi.Strip(cbody), "S/"), cbody)
	}
	for _, name := range []string{"Sun", "Moon", "Mars"} {
		if strings.Contains(cgot[name], "S/") {
			t.Errorf("columns: unmarked %s row carries a mark: %q", name, cgot[name])
		}
	}
}

// C8: the mark takes the retrograde gradient at stop round((1 - speed/0.05) * 6).
func TestStationMarkColour(t *testing.T) {
	forceColor(t)
	paint := func(stop int) string {
		return lipgloss.NewStyle().Foreground(speedGradientColour(stop)).Render("S/R")
	}
	for _, c := range []struct {
		speed float64
		stop  int
	}{{0.000, 124}, {0.025, 203}, {0.049, 255}} {
		if got, want := stationMarkCells(lookup.StationRetro, c.speed, true), paint(c.stop); got != want {
			t.Errorf("speed %.3f: %q, want stop %d: %q", c.speed, got, c.stop, want)
		}
	}
	if !strings.Contains(stationMarkCells(lookup.StationDirect, 0, true), "124") {
		t.Errorf("S/D at zero is not the deepest red: %q", stationMarkCells(lookup.StationDirect, 0, true))
	}
	if got := stationMarkCells(lookup.StationDirect, 0.02, false); got != "S/D" || strings.Contains(got, "\x1b") {
		t.Errorf("colours off: %q, want plain S/D", got)
	}
	// Every row in a colour frame uses it too.
	row := formatPositionRowStyled(aspect.Position{Body: aspect.Venus, Longitude: 48.9, Speed: 0}, astrologstyle.BodyRow{}, true, houseOverlay{stations: stationMarksTwo})
	if !strings.Contains(row, stationMarkCells(lookup.StationDirect, 0, true)) {
		t.Errorf("a Venus row at 0.000 is not S/D at the deepest red: %q", row)
	}
}

// C12 / C11 at the bar: a row at -0.000 shows ℞, +0.000 does not.
func TestSpeedBarFollowsAstrologsSign(t *testing.T) {
	negZero := math.Copysign(0, -1)
	if got := velocityBar(negZero, false, false); !strings.HasPrefix(got, "℞") {
		t.Errorf("velocityBar(-0.000) = %q, want it to open with ℞", got)
	}
	if got := velocityBar(0, false, false); strings.HasPrefix(got, "℞") {
		t.Errorf("velocityBar(+0.000) = %q, want no ℞", got)
	}
	row := func(speed float64) string {
		return formatPositionRow(aspect.Position{Body: aspect.Saturn, Longitude: 294.0167, Speed: speed, House: 3}, false)
	}
	if !strings.Contains(row(negZero), "℞") {
		t.Errorf("a Saturn row at -0.000 has no ℞: %q", row(negZero))
	}
	if strings.Contains(row(0), "℞") {
		t.Errorf("a Saturn row at +0.000 has a ℞: %q", row(0))
	}
}

// V1: the marked speed lane, one slow-direct body before an S/R and one after
// an S/D, colours on and off.
func TestBodiesStationMarkFrames(t *testing.T) {
	forceColor(t)
	for _, mode := range []struct {
		name string
		cfg  Config
	}{{"color", Config{Color: ColorAlways}}, {"never", Config{Color: ColorNever}}} {
		m := frameModel(t, mode.cfg, 132, 40, 1, 1)
		m.slots.ToggleSlotSelection(1)
		m.chart.columns, m.chart.columnsFor = nil, nil
		m.chart.positions = stationFramePositions()
		m.chart.stations = map[aspect.Body]lookup.Station{aspect.Mercury: lookup.StationRetro, aspect.Venus: lookup.StationDirect}
		m.focused = PanelBodies
		writeDesignFrame(t, fmt.Sprintf("bodies-station-mark-132x40-%s", mode.name), m.View())
	}
}

func stationFramePositions() []aspect.Position {
	return []aspect.Position{
		{Body: aspect.Sun, Longitude: 84.2167, Speed: 0.9690, House: 8},
		{Body: aspect.Moon, Longitude: 346.7500, Speed: 1.0160, House: 5},
		{Body: aspect.Mercury, Longitude: 65.8667, Speed: 0.0310, House: 8},
		{Body: aspect.Venus, Longitude: 48.9000, Speed: 0.0000, House: 7},
		{Body: aspect.Mars, Longitude: 11.1000, Speed: 1.3710, House: 6},
		{Body: aspect.Jupiter, Longitude: 105.9000, Speed: 0.0420, House: 9},
		{Body: aspect.Saturn, Longitude: 294.0167, Speed: -0.4550, House: 3},
		{Body: aspect.Ascendant, Longitude: 210.8667, Speed: 0.6890, House: 1},
	}
}

// C9 (amended 2026-10-14): Enter on an S/D row lands at the end of its minute
// (+0.000, the lane says S/D at the deepest red); on an S/R row at its start
// (still direct, C14), where the lane says S/R and no ℞. Each case enters its
// own real timeline row.
func TestEnterOnAStationRowShowsItsMark(t *testing.T) {
	for _, tc := range []struct {
		name  string
		frag  string // the timeline row entered
		body  aspect.Body
		label string
		speed float64
		mark  string // text expected in the body's row
		not   string
		rule  lookup.Station // what the 5% rule says at the landed instant
	}{
		{"S/D row reads +0.000", "Pluto <Aqu> S/D", aspect.Pluto, "Pluto", 0, "S/D", "℞", lookup.StationDirect},
		{"S/R row reads +0.002, still direct", "Venus <Sco> S/R", aspect.Venus, "Venus", 0.002, "S/R", "℞", lookup.StationRetro},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forceColor(t)
			rows := append([]lookup.TimelineRow(nil), enteredRows(t)...)
			rows = append(rows, rowsFromSearch(t, tlFloatingTime, "Sat 10/17/2026  3:15am   Venus <Sco> S/R\n", "")...)
			sort.SliceStable(rows, func(a, b int) bool { return rows[a].Instant.Before(rows[b].Instant) })
			m, _ := enteredFixtureRows(t, rows, 2, true, Config{Color: ColorAlways})
			body := aspect.Position{Body: tc.body, Longitude: 301, Speed: tc.speed, House: 3}
			firstCallTransits = func(context.Context, *lookup.Probers, time.Time, location.PlaceInfo, lookup.Lens) (lookup.Result, error) {
				return lookup.Result{Positions: []aspect.Position{body}}, nil
			}
			firstCallNatalPositions = func(context.Context, *lookup.Probers, store.AnchorChart, lookup.Lens) (lookup.Result, error) {
				return lookup.Result{Positions: []aspect.Position{body}}, nil
			}
			// The rule itself is proven in internal/lookup; this stub answers what
			// the 5% rule says for the case (tc.rule).
			installStationMarks(t, func(_ context.Context, _ *lookup.Probers, ps []aspect.Position, _ time.Time, _ location.PlaceInfo, _ lookup.Lens) (map[aspect.Body]lookup.Station, []string, error) {
				marks := map[aspect.Body]lookup.Station{}
				for _, p := range ps {
					if !math.Signbit(p.Speed) && p.Speed < 0.05 {
						marks[p.Body] = tc.rule
					}
				}
				return marks, nil, nil
			})
			i := timelineRowWith(t, m.timeline.rows, tc.frag)
			row := m.timeline.rows[i]
			m = enter(t, m, i)
			want := row.Instant
			if tc.mark == "S/D" {
				want = want.Add(59 * time.Second)
			}
			if !m.targetFloatingTime.Equal(want) {
				t.Errorf("landed at %s, want %s", m.targetFloatingTime.Format(time.RFC3339), want.Format(time.RFC3339))
			}
			bodies, _ := leftPanels(t, m.View())
			var line string
			for _, l := range bodies {
				if strings.Contains(ansi.Strip(l), tc.label) {
					line = l
				}
			}
			if line == "" {
				t.Fatalf("no %s row in Bodies:\n%s", tc.label, strings.Join(bodies, "\n"))
			}
			plain := ansi.Strip(line)
			if !strings.Contains(plain, tc.mark) || strings.Contains(plain, tc.not) {
				t.Errorf("%s row %q: want %q and no %q", tc.label, plain, tc.mark, tc.not)
			}
			if tc.mark == "S/D" && !strings.Contains(line, "124") {
				t.Errorf("S/D at +0.000 is not the deepest red: %q", line)
			}
		})
	}
}

// C4: the natal-only compute carries its marks and the probe's command in the
// same message as the positions, probed at the chart's own moment and place.
func TestNatalSingleComputeCarriesItsStationMarks(t *testing.T) {
	stationLookups(t)
	firstCallNatalPositions = func(context.Context, *lookup.Probers, store.AnchorChart, lookup.Lens) (lookup.Result, error) {
		return lookup.Result{Positions: []aspect.Position{mercurySlow}, Commands: []string{"astrolog natal"}}, nil
	}
	var at time.Time
	var where location.PlaceInfo
	installStationMarks(t, func(_ context.Context, _ *lookup.Probers, ps []aspect.Position, when time.Time, place location.PlaceInfo, _ lookup.Lens) (map[aspect.Body]lookup.Station, []string, error) {
		at, where = when, place
		return map[aspect.Body]lookup.Station{aspect.Mercury: lookup.StationDirect}, []string{"astrolog station probe"}, nil
	})
	chart := commandsTestChart()
	msg := computeNatalSingleCmd(context.Background(), 1, 1, nil, chart, lookup.Hellenistic)().(FirstFrameReadyMsg)
	if len(msg.Positions) != 1 || msg.Stations[aspect.Mercury] != lookup.StationDirect {
		t.Fatalf("natal message: positions %v marks %v; want both", msg.Positions, msg.Stations)
	}
	if !at.Equal(chart.When) || where != chart.Place {
		t.Errorf("probed at %s in %s, want the chart's %s in %s", at, where.Name, chart.When, chart.Place.Name)
	}
	if want := []string{"astrolog natal", "astrolog station probe"}; !slices.Equal(msg.Commands, want) {
		t.Errorf("Commands = %q, want %q", msg.Commands, want)
	}
}

// C4: a failed transit or cross run still delivers the focused chart's marks
// (and the probe's command) beside the error.
func TestStationMarkSurvivesACrossAspectError(t *testing.T) {
	boom := errors.New("boom")
	chart := commandsTestChart()
	run := func(t *testing.T, focusedNatal bool, transitErr, crossErr error) FirstFrameReadyMsg {
		t.Helper()
		stationLookups(t)
		firstCallNatalPositions = func(context.Context, *lookup.Probers, store.AnchorChart, lookup.Lens) (lookup.Result, error) {
			return lookup.Result{Positions: []aspect.Position{mercurySlow}, Commands: []string{"astrolog natal"}}, nil
		}
		firstCallTransits = func(context.Context, *lookup.Probers, time.Time, location.PlaceInfo, lookup.Lens) (lookup.Result, error) {
			return lookup.Result{Positions: []aspect.Position{mercurySlow}, Commands: []string{"astrolog transit"}}, transitErr
		}
		overlayCrossAspects = func(context.Context, lookup.Lens, store.AnchorChart, time.Time, location.PlaceInfo) (lookup.OverlayAspects, error) {
			return lookup.OverlayAspects{Commands: []string{"astrolog cross"}}, crossErr
		}
		runs := 0
		installStationMarks(t, markMercury(&runs))
		return computeOverlayCmd(context.Background(), 1, 1, nil, chart, store.AnchorChart{}, testFrozenTime(), testPlaceBerlin(), focusedNatal, lookup.Hellenistic)().(FirstFrameReadyMsg)
	}
	t.Run("cross fails", func(t *testing.T) {
		msg := run(t, false, nil, boom)
		if !errors.Is(msg.Err, boom) || msg.Stations[aspect.Mercury] != lookup.StationRetro {
			t.Errorf("Err=%v marks=%v; want the error and the mark", msg.Err, msg.Stations)
		}
		if n := len(msg.Commands); n == 0 || msg.Commands[n-1] != "astrolog station probe" {
			t.Errorf("Commands = %q, want the probe's command last", msg.Commands)
		}
	})
	t.Run("transit fails with the natal chart focused", func(t *testing.T) {
		msg := run(t, true, boom, nil)
		if !errors.Is(msg.Err, boom) || msg.Stations[aspect.Mercury] != lookup.StationRetro {
			t.Errorf("Err=%v marks=%v; want the error and the mark", msg.Err, msg.Stations)
		}
		if n := len(msg.Commands); n == 0 || msg.Commands[n-1] != "astrolog station probe" {
			t.Errorf("Commands = %q, want the probe's command last", msg.Commands)
		}
	})
}

// stationRowsFixture is the entered fixture's week plus a Venus S/R row, a
// North Node row Astrolog words S/R (lookup relabels it S/D, C17) and a South
// Node row it words S/D (relabelled S/R).
func stationRowsFixture(t *testing.T) []lookup.TimelineRow {
	t.Helper()
	rows := append([]lookup.TimelineRow(nil), enteredRows(t)...)
	rows = append(rows, rowsFromSearch(t, tlFloatingTime,
		"Sat 10/17/2026  3:15am   Venus <Sco> S/R\n"+
			"Sat 10/17/2026  4:15am North N <Pis> S/R\n"+
			"Sat 10/17/2026  5:15am South N <Vir> S/D\n"+
			"Sat 10/17/2026  6:15am  Lilith <Leo> S/R\n", "")...)
	sort.SliceStable(rows, func(a, b int) bool { return rows[a].Instant.Before(rows[b].Instant) })
	return rows
}

// C14: Enter lands a station row on the side where the body is direct: an S/R
// on the minute's first second, an S/D (and every other kind) on its last.
func TestEnterOnAStationRowLandsOnTheDirectSide(t *testing.T) {
	for _, tc := range []struct {
		frag   string
		offset time.Duration
	}{
		{"Venus <Sco> S/R", 0},
		{"Pluto <Aqu> S/D", 59 * time.Second},
		{"North N <Pis> S/D", 59 * time.Second}, // Astrolog's S/R, relabelled
		{"South N <Vir> S/R", 0},                // Astrolog's S/D, relabelled
		{"Tri (Leo) Mars", 59 * time.Second},
		{"--> Sagittarius", 59 * time.Second},
	} {
		t.Run(tc.frag, func(t *testing.T) {
			m, _ := enteredFixtureRows(t, stationRowsFixture(t), 2, true, Config{Color: ColorNever})
			i := timelineRowWith(t, m.timeline.rows, tc.frag)
			row := m.timeline.rows[i]
			m = enter(t, m, i)
			want := row.Instant.Add(tc.offset)
			if !m.targetFloatingTime.Equal(want) {
				t.Errorf("target = %s, want %s", m.targetFloatingTime.Format(time.RFC3339), want.Format(time.RFC3339))
			}
			if got := m.floatingTime(); !got.Equal(want) {
				t.Errorf("committed floating time = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
			}
			if n := m.nearestRow(m.timelineShown()); n != i {
				t.Errorf("nearest row = %d, want the entered %d", n, i)
			}
			if m.deriveScene().entered == nil {
				t.Error("the entered row does not hold at its landed instant")
			}
		})
	}
}

// bodiesLine is the Bodies row of the left column naming label, escapes kept.
func bodiesLine(t *testing.T, view, label string) string {
	t.Helper()
	bodies, _ := leftPanels(t, view)
	for _, l := range bodies {
		if strings.Contains(ansi.Strip(l), label) {
			return l
		}
	}
	t.Fatalf("no %s row in Bodies:\n%s", label, strings.Join(bodies, "\n"))
	return ""
}

func stationBodiesLookups() {
	row := func(b aspect.Body, speed float64) aspect.Position {
		return aspect.Position{Body: b, Longitude: 100 + float64(b), Speed: speed, House: 3}
	}
	all := []aspect.Position{row(aspect.Sun, 0.97), row(aspect.Venus, 0.02), row(aspect.Pluto, 0.0), row(aspect.NorthNode, 3.0), row(aspect.SouthNode, 3.0), row(aspect.Lilith, 3.0)}
	firstCallTransits = func(context.Context, *lookup.Probers, time.Time, location.PlaceInfo, lookup.Lens) (lookup.Result, error) {
		return lookup.Result{Positions: all}, nil
	}
	firstCallNatalPositions = func(context.Context, *lookup.Probers, store.AnchorChart, lookup.Lens) (lookup.Result, error) {
		return lookup.Result{Positions: all}, nil
	}
}

// C15: while an entered station row holds, its body shows the row's word at
// the deepest red whatever the 5% rule says; when the row ends the rule alone
// decides.
func TestEnteredStationRowShowsItsWord(t *testing.T) {
	forceColor(t)
	cases := []struct {
		frag, label, word, not string
	}{
		{"Venus <Sco> S/R", "Venus", "S/R", "℞"},
		{"North N <Pis> S/D", "N.Node", "S/D", "S/R"},
		{"South N <Vir> S/R", "S.Node", "S/R", "S/D"},
		{"Lilith <Leo> S/R", "Lilith", "S/R", "℞"}, // excluded from the 5% rule
	}
	for _, tc := range cases {
		t.Run(tc.frag, func(t *testing.T) {
			m, _ := enteredFixtureRows(t, stationRowsFixture(t), 2, true, Config{Color: ColorAlways})
			stationBodiesLookups() // the rule (default seam) marks nothing here
			m = enter(t, m, timelineRowWith(t, m.timeline.rows, tc.frag))
			line := bodiesLine(t, m.View(), tc.label)
			plain := ansi.Strip(line)
			if !strings.Contains(plain, tc.word) || strings.Contains(plain, tc.not) {
				t.Errorf("%s row %q: want %q and not %q", tc.label, plain, tc.word, tc.not)
			}
			if !strings.Contains(line, stationMarkCells(map[string]lookup.Station{"S/R": lookup.StationRetro, "S/D": lookup.StationDirect}[tc.word], 0, true)) {
				t.Errorf("%s row is not at the deepest red: %q", tc.label, line)
			}
			// A time step ends the entered row: the rule alone decides again.
			m.cursorIdx = len(m.slots.All()) - 1
			m, cmd := tpress(t, m, runes("d"))
			m = pump(t, m, cmd)
			if after := ansi.Strip(bodiesLine(t, m.View(), tc.label)); strings.Contains(after, "S/") && tc.label != "Venus" {
				t.Errorf("after the row ended, %s still reads %q", tc.label, after)
			}
			if after := ansi.Strip(bodiesLine(t, m.View(), tc.label)); tc.label == "Venus" && strings.Contains(after, "S/") {
				t.Errorf("after the row ended, the rule (no mark) must decide, got %q", after)
			}
		})
	}
}

// C16: for a planet at its listed station the entered word and the real rule
// (lookup.StationMarksBetween over stubbed readings a day apart) say the same.
func TestEnteredStationWordAgreesWithTheRule(t *testing.T) {
	for _, tc := range []struct {
		frag, label string
		at, later   float64 // the reading at the landed instant, and a day on
		second      int     // the second of the minute the body is direct on
		word        string
	}{
		{"Venus <Sco> S/R", "Venus", 0.001, -0.05, 0, "S/R"}, // landed :00, still direct, falling
		{"Pluto <Aqu> S/D", "Pluto", 0.0, 0.119, 59, "S/D"},  // landed :59, direct, rising
	} {
		t.Run(tc.frag, func(t *testing.T) {
			m, _ := enteredFixtureRows(t, stationRowsFixture(t), 2, true, Config{Color: ColorNever})
			id := map[string]aspect.Body{"Venus": aspect.Venus, "Pluto": aspect.Pluto}[tc.label]
			// Astrolog's reading: direct on the station's own side of the minute,
			// retrograde (-0.000) on the other.
			at := func(when time.Time) aspect.Position {
				speed := math.Copysign(0, -1)
				if when.Second() == tc.second {
					speed = tc.at
				}
				return aspect.Position{Body: id, Longitude: 100, Speed: speed, House: 3}
			}
			var last aspect.Position
			firstCallTransits = func(_ context.Context, _ *lookup.Probers, when time.Time, _ location.PlaceInfo, _ lookup.Lens) (lookup.Result, error) {
				last = at(when)
				return lookup.Result{Positions: []aspect.Position{last}}, nil
			}
			body := at(time.Time{}.Add(time.Duration(tc.second) * time.Second))
			firstCallNatalPositions = func(context.Context, *lookup.Probers, store.AnchorChart, lookup.Lens) (lookup.Result, error) {
				return lookup.Result{Positions: []aspect.Position{body}}, nil
			}
			dayLater := []aspect.Position{{Body: body.Body, Speed: tc.later}}
			installStationMarks(t, func(_ context.Context, _ *lookup.Probers, ps []aspect.Position, _ time.Time, _ location.PlaceInfo, _ lookup.Lens) (map[aspect.Body]lookup.Station, []string, error) {
				return lookup.StationMarksBetween(ps, dayLater), nil, nil
			})
			i := timelineRowWith(t, m.timeline.rows, tc.frag)
			rowWord := m.timeline.rows[i].Station
			m = enter(t, m, i)
			if rowWord != tc.word {
				t.Fatalf("fixture: the row says %q, want %q", rowWord, tc.word)
			}
			if rule := lookup.StationMarksBetween([]aspect.Position{last}, dayLater)[id].String(); rule != rowWord {
				t.Errorf("the rule says %q for %s at the landed instant (speed %v), the row %q", rule, tc.label, last.Speed, rowWord)
			}
			plain := ansi.Strip(bodiesLine(t, m.View(), tc.label))
			other := map[string]string{"S/R": "S/D", "S/D": "S/R"}[rowWord]
			if !strings.Contains(plain, rowWord) || strings.Contains(plain, other) || strings.Contains(plain, "℞") {
				t.Errorf("%s row %q: want %q, not %q or ℞", tc.label, plain, rowWord, other)
			}
		})
	}
}
