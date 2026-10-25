package ui

// the specification acceptance 3-7: the rendered screen agrees with itself
// under each of the four forms.

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// ---- rendered screens -------------------------------------------------------

var formZones = []location.PlaceInfo{
	{Name: "Berlin", Country: "DE", Lat: 52.52, Lon: 13.41, TZ: "Europe/Berlin"},
	{Name: "New York", Country: "US", Lat: 40.71, Lon: -74.0, TZ: "America/New_York"},
	{Name: "Tokyo", Country: "JP", Lat: 35.68, Lon: 139.69, TZ: "Asia/Tokyo"},
	{Name: "Mumbai", Country: "IN", Lat: 19.07, Lon: 72.88, TZ: "Asia/Kolkata"},
	{Name: "Lord Howe", Country: "AU", Lat: -31.55, Lon: 159.08, TZ: "Australia/Lord_Howe"},
	{Name: "Auckland", Country: "NZ", Lat: -36.85, Lon: 174.76, TZ: "Pacific/Auckland"},
	{Name: "London", Country: "GB", Lat: 51.51, Lon: -0.13, TZ: "Europe/London"},
}

type formCase struct {
	place location.PlaceInfo
	at    time.Time
}

// formCases are named instants either side of a transition and of local
// midnight and noon, then seeded random ones over the same zones.
func formCases() []formCase {
	utc := func(y int, mo time.Month, d, h, mi int) time.Time { return time.Date(y, mo, d, h, mi, 0, 0, time.UTC) }
	z := func(name string) location.PlaceInfo {
		for _, p := range formZones {
			if p.Name == name {
				return p
			}
		}
		panic(name)
	}
	cases := []formCase{
		{z("Berlin"), utc(2026, 10, 5, 7, 5)},      // 9:05 CEST, a one-digit hour and day
		{z("Berlin"), utc(2026, 10, 15, 22, 30)},   // 0:30 the next local day
		{z("New York"), utc(2026, 3, 8, 6, 59)},    // just before the spring jump
		{z("New York"), utc(2026, 3, 8, 7, 1)},     // just after
		{z("Tokyo"), utc(2026, 6, 15, 3, 5)},       // 12:05 noon
		{z("Tokyo"), utc(2026, 6, 14, 15, 5)},      // 0:05 midnight
		{z("Mumbai"), utc(2026, 1, 5, 3, 20)},      // a half-hour zone
		{z("Lord Howe"), utc(2026, 10, 4, 15, 15)}, // a half-hour daylight shift
		{z("Auckland"), utc(2026, 9, 26, 13, 59)},  // just before local midnight and a transition
		{z("London"), utc(2026, 10, 25, 0, 30)},    // the repeated autumn hour
		{z("London"), utc(2026, 10, 25, 1, 30)},    // its second pass
	}
	rng := rand.New(rand.NewSource(20261010))
	for i := 0; i < 6; i++ {
		p := formZones[rng.Intn(len(formZones))]
		at := utc(2026, time.Month(1+rng.Intn(12)), 1+rng.Intn(28), rng.Intn(24), rng.Intn(60))
		cases = append(cases, formCase{p, at})
	}
	return cases
}

// formModel is a ready single-chart workspace: the floating slot alone at
// (at, place), room for Details.
func formModel(t *testing.T, fc formCase) Model {
	t.Helper()
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(fc.at))
	slots, err := store.NewSlots(fc.at, fc.place)
	if err != nil {
		t.Fatal(err)
	}
	m.slots = slots
	m.place = fc.place
	m.targetFloatingTime = slots.Floating().Time
	m.startup = startupReady
	m.width, m.height = 144, 40
	return m
}

func plainView(m Model) string { return ansi.Strip(m.View()) }

// chartRowMoment is the moment the rendered Chart row of the given place
// shows: the cells before the zone abbreviation and the place. The place is
// the row's own fit-or-drop field: the floating row's `~ Now` title takes the
// cells that let it show in the narrowest left column, so the row is found by
// its zone and either the place or the gap the place left.
func chartRowMoment(t *testing.T, view string, p location.PlaceInfo, zoneAbbr string) string {
	t.Helper()
	re := regexp.MustCompile(`(` + momentRE + `) ` + regexp.QuoteMeta(zoneAbbr) + `(?: +` + regexp.QuoteMeta(p.Name) + `| {3,}\S+\|)`)
	m := re.FindStringSubmatch(view)
	if m == nil {
		t.Fatalf("no chart row for %s (%s) in the rendered screen:\n%s", p.Name, zoneAbbr, view)
	}
	return m[1]
}

var reChartFor = regexp.MustCompile(`(?:chart|comparison) for (` + momentRE + `) \(`)

// Acceptance 3: as a property over zones and instants, under each of the four
// forms, the RENDERED Details header of every single-chart report (Chart, a
// body, Aspects) shows the same weekday, date and clock as the slot's row.
func TestEveryDetailsReportReadsTheSlotClock(t *testing.T) {
	needLiveAstrolog(t)
	ctx := context.Background()
	for _, c := range clockCases {
		t.Run(c.String(), func(t *testing.T) {
			writeClockPrefs(t, clockPrefsBody(c))
			coldStart(t)
			for _, fc := range formCases() {
				name := fmt.Sprintf("%s/%s", fc.place.Name, fc.at.Format("2006-01-02T15:04Z"))
				t.Run(name, func(t *testing.T) {
					m := formModel(t, fc)
					loc, _ := time.LoadLocation(fc.place.TZ)
					local := fc.at.In(loc)
					abbr := local.Format("MST")
					want := refMoment(local, c)
					lens := m.viewLens()

					check := func(label, view string) {
						t.Helper()
						if row := chartRowMoment(t, view, fc.place, abbr); row != want {
							t.Errorf("%s: the chart row reads %q, want %q", label, row, want)
						}
						h := reChartFor.FindStringSubmatch(view)
						if h == nil {
							t.Errorf("%s: no Details header in the rendered screen", label)
							return
						}
						if h[1] != want {
							t.Errorf("%s: Details header reads %q, the slot row %q", label, h[1], want)
						}
					}

					for _, k := range []struct {
						label string
						req   lookup.RawReportRequest
					}{
						{"Chart", lookup.RawReportRequest{Kind: lookup.RawReportChart}},
						{"body", lookup.RawReportRequest{Kind: lookup.RawReportBody, Body: aspect.Sun}},
					} {
						body, _, err := lookup.ComputeRawReport(ctx, fc.at, fc.place, lens, k.req)
						if err != nil {
							t.Fatalf("%s report: %v", k.label, err)
						}
						mm := m
						mm.focused = PanelChart
						mm = mm.adoptDetailsReport("[0] "+k.label, body)
						check(k.label, plainView(mm))
					}

					mm := m
					mm.focused = PanelAspects
					msg, ok := buildTextCmd(mm, ctx, 1, 1)().(FirstFrameReadyMsg)
					if !ok || msg.Err != nil {
						t.Fatalf("text compute: %#v", msg.Err)
					}
					st := mm.refresh[surfaceText]
					msg.RunID, msg.WantID = st.runningRunID, st.wantID
					mm, _ = applyTextCommit(mm, msg)
					mm = mm.applyAspectsDetails()
					check("Aspects", plainView(mm))
				})
			}
		})
	}
}

// Acceptance 4: a pair's rendered Details reports show each chart's own clock,
// in the chosen form, after the pair is selected, under each of the four forms.
func TestPairReportsShowTheChosenForm(t *testing.T) {
	needLiveAstrolog(t)
	// The pair of pairClockModel: the anchored chart in New York (EDT), the
	// floating slot in Tokyo (JST).
	natal := time.Date(1990, 6, 15, 10, 30, 0, 0, mustLoc(t, "America/New_York"))
	transit := time.Date(2026, 4, 26, 5, 30, 0, 0, mustLoc(t, "Asia/Tokyo"))
	for _, c := range clockCases {
		t.Run(c.String(), func(t *testing.T) {
			writeClockPrefs(t, clockPrefsBody(c))
			coldStart(t)
			m := pairClockModel(t)
			m.width = max(m.width, dualColumnMinWidth(m.shownCount()))
			msg, ok := buildTextCmd(m, context.Background(), 1, 1)().(FirstFrameReadyMsg)
			if !ok || msg.Err != nil {
				t.Fatalf("pair compute: %#v", msg.Err)
			}
			st := m.refresh[surfaceText]
			msg.RunID, msg.WantID = st.runningRunID, st.wantID
			m, _ = applyTextCommit(m, msg)
			m = m.applyAspectsDetails()
			view := plainView(m)

			headers := reChartFor.FindAllStringSubmatch(view, -1)
			if len(headers) != 2 {
				t.Fatalf("the rendered Details shows %d chart headers, want 2:\n%s", len(headers), view)
			}
			for i, want := range []string{refMoment(natal, c), refMoment(transit, c)} {
				if headers[i][1] != want {
					t.Errorf("header %d reads %q, want %q", i, headers[i][1], want)
				}
			}
			// And the rows of the Chart panel read the same two moments.
			if row := chartRowMoment(t, view, location.PlaceInfo{Name: "New York"}, "EDT"); row != refMoment(natal, c) {
				t.Errorf("anchored row reads %q, want %q", row, refMoment(natal, c))
			}
			if row := chartRowMoment(t, view, testPlaceTokyo(), "JST"); row != refMoment(transit, c) {
				t.Errorf("floating row reads %q, want %q", row, refMoment(transit, c))
			}
		})
	}
}

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

// Acceptance 5: the rendered Timeline rows of a -dm, a -t and a -tr search
// show the chosen form, under all four.
func TestTimelineDisplaysChosenClock(t *testing.T) {
	needLiveAstrolog(t)
	old := computeTimeline
	computeTimeline = lookup.Timeline
	t.Cleanup(func() { computeTimeline = old })

	for _, c := range clockCases {
		t.Run(c.String(), func(t *testing.T) {
			writeClockPrefs(t, clockPrefsBody(c))
			coldStart(t)
			m := detailsTimelineModel(t, Config{Accessible: true})
			m, _ = syncTimelineVisited(m)
			cmd := buildTimelineCmd(m, context.Background(), 1, 1)
			if cmd == nil {
				t.Fatal("no timeline search")
			}
			res, ok := cmd().(refreshResultMsg)
			if !ok || res.err != nil {
				t.Fatalf("timeline search: %#v", res.err)
			}
			m, _ = m.commitTimeline(res)
			if len(m.timeline.rows) == 0 {
				t.Fatal("the search showed no rows")
			}
			// Attach Details to the Timeline the way an operator does.
			m, _ = tpress(t, m, runes("0"))
			m, _ = tpress(t, m, keyEnter)
			if m.activeTabID(PanelDetails) != TabDetailsTimeline {
				t.Fatalf("the Details panel shows %s, not the Timeline", m.activeTabID(PanelDetails))
			}
			rendered := wholeTimeline(m)
			seen := map[string]int{}
			for _, row := range m.timeline.rows {
				for _, kind := range []string{" -dm", " -tr", " -t "} {
					if strings.Contains(row.Command+" ", kind) {
						seen[strings.TrimSpace(kind)]++
					}
				}
				twelve := strings.HasSuffix(row.Time, "am") || strings.HasSuffix(row.Time, "pm")
				dayFirst := strings.Contains(row.Date, "-")
				if twelve != (c.hours == "12") || dayFirst != (c.dates == "day-first") {
					t.Errorf("row %q is not in the preference's form", row.Line)
				}
				if countContaining(rendered, row.Date) == 0 || countContaining(rendered, strings.TrimSpace(row.Time)) == 0 {
					t.Errorf("the rendered Timeline does not show %q %q", row.Date, row.Time)
				}
			}
			for _, kind := range []string{"-dm", "-t", "-tr"} {
				if seen[kind] == 0 {
					t.Errorf("no rendered row came from a %s search (saw %v)", kind, seen)
				}
			}
		})
	}
}

var reWaterfallCell = regexp.MustCompile(`\(\d+°\d{2}'[AS]\)`)

// Acceptance 6: every orb in a rendered Details report reads a0°04' / s0°04',
// and the Aspects panel's waterfall cells keep their form (0°04'A), width
// unchanged.
func TestAspectsWaterfallCellsUnchanged(t *testing.T) {
	needLiveAstrolog(t)
	for _, c := range clockCases {
		t.Run(c.String(), func(t *testing.T) {
			writeClockPrefs(t, clockPrefsBody(c))
			coldStart(t)
			m := clockSingleModel(t)
			msg, ok := buildTextCmd(m, context.Background(), 1, 1)().(FirstFrameReadyMsg)
			if !ok || msg.Err != nil {
				t.Fatalf("text compute: %#v", msg.Err)
			}
			st := m.refresh[surfaceText]
			msg.RunID, msg.WantID = st.runningRunID, st.wantID
			m, _ = applyTextCommit(m, msg)
			m = m.applyAspectsDetails()
			view := plainView(m)

			if !reDegreeOrb.MatchString(view) {
				t.Errorf("no orb in the rendered Details reads a0°04' / s0°04'")
			}
			if reColonOrb.MatchString(view) {
				t.Errorf("an orb in the rendered Details still reads with a colon: %q", reColonOrb.FindString(view))
			}
			cells := reWaterfallCell.FindAllString(view, -1)
			if len(cells) == 0 {
				t.Fatalf("no waterfall cell of the form (0°04'A) on the rendered screen")
			}
			if strings.Contains(view, ":0") && regexp.MustCompile(`\(\d+:\d{2}'[AS]\)`).MatchString(view) {
				t.Errorf("a waterfall cell reads with a colon")
			}
			// The cell is "(" digits "°" two digits "'" mark ")": its width is
			// that text, with nothing the clock form can add.
			for _, cell := range cells {
				if ansi.StringWidth(cell) > len("(00°00'A)")+2 {
					t.Errorf("waterfall cell %q is wider than the form allows", cell)
				}
			}
		})
	}
}

// Acceptance 7: on a cold start from a real preferences.yaml the first
// rendered chart row uses the chosen form, and so does the row after a time
// step.
func TestFirstRowFollowsThePreference(t *testing.T) {
	for _, c := range clockCases {
		t.Run(c.String(), func(t *testing.T) {
			writeClockPrefs(t, clockPrefsBody(c))
			coldStart(t)
			m := clockSingleModel(t)
			berlin := mustLoc(t, m.place.TZ)

			row := func(label string, m Model) {
				t.Helper()
				at := m.slots.Floating().Time.In(berlin)
				want := refMoment(at, c)
				if got := chartRowMoment(t, plainView(m), m.place, at.Format("MST")); got != want {
					t.Errorf("%s: the first row reads %q, want %q", label, got, want)
				}
			}
			row("first frame", m)
			first := m.slots.Floating().Time
			// The step sets the target; the row moves when its frame commits.
			stepped, _ := m.advanceTime(unitHour, +1)
			st := stepped.refresh[surfaceText]
			var tm tea.Model = stepped
			tm, _ = tm.Update(FirstFrameReadyMsg{
				Positions: []aspect.Position{{Body: aspect.Sun, Longitude: 100}},
				RunID:     st.runningRunID, WantID: st.wantID, TransitTime: stepped.targetFloatingTime,
			})
			stepped = tm.(Model)
			if stepped.slots.Floating().Time.Equal(first) {
				t.Fatal("the time step moved nothing")
			}
			row("after a step", stepped)

			// Editing the file while Sextile runs changes nothing.
			other := clockCase{"12", "day-first"}
			if c == other {
				other = clockCase{"24", "month-first"}
			}
			if err := os.WriteFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "sextile", "preferences.yaml"), []byte(clockPrefsBody(other)), 0o644); err != nil {
				t.Fatal(err)
			}
			row("after an edit", stepped)
		})
	}
}

// The frames of the specification under the default form (24-hour, month
// first): the Chart panel with the floating row and two named rows, the Details
// Report tab beside the chart row it must agree with, and the Timeline tab.
func TestClockFormFrames(t *testing.T) {
	holdForm(t, clockCase{"24", "month-first"})
	cfg := Config{Color: ColorNever}
	for _, size := range []struct {
		name string
		w, h int
	}{{"80x24", 80, 24}, {"132x40", 132, 40}} {
		m := frameModel(t, cfg, size.w, size.h, 2, 1)
		writeDesignFrame(t, "clock-form-chart-"+size.name+"-never", m.View())
	}

	// The report is Astrolog's own, recorded for the 1990-06-15 16:30 CEST
	// Berlin chart (testdata/influence-chart.report); the chart row beside it
	// is that same moment.
	raw, err := os.ReadFile("testdata/influence-chart.report")
	if err != nil {
		t.Fatal(err)
	}
	berlin := location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"}
	m := formModel(t, formCase{berlin, time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC)})
	m.cfg = cfg
	m.focused = PanelChart
	m = m.adoptDetailsReport("[0] Chart", string(raw))
	writeDesignFrame(t, "clock-form-details-144x40-never", m.View())

	tl := onTimeline(t, cfg)
	writeDesignFrame(t, "clock-form-timeline-144x40-never", tl.View())
}
