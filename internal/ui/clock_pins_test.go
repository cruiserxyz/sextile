package ui

// the specification C10, C12, C13, N6, N18, N24: the clock preference is
// read once at start and held; the first frames use it, and Details/Aspects
// adds no spawn.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/testenv"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

type clockCase struct {
	hours, dates string
}

var clockCases = []clockCase{
	{"24", "month-first"}, {"24", "day-first"}, {"12", "month-first"}, {"12", "day-first"},
}

func (c clockCase) String() string { return c.hours + "h/" + c.dates }

// writeClockPrefs points the preferences at a fresh file holding c and returns
// its path.
func writeClockPrefs(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "sextile", "preferences.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lookup.HoldClock(lookup.ClockPref{}) })
	return path
}

func clockPrefsBody(c clockCase) string {
	return "clock:\n  hours: " + c.hours + "\n  dates: " + c.dates + "\n"
}

// coldStart reads the startup perimeter from the file on disk and applies it,
// the way a launch does.
func coldStart(t *testing.T) Model {
	t.Helper()
	msg, ok := ingest.ReadStartupPerimeter()().(ingest.StartupPerimeterReadMsg)
	if !ok {
		t.Fatal("the startup read did not answer with its message")
	}
	m, _ := applyStartupPerimeter(NewModel(Config{}), msg)
	return m
}

// N18 / C10: one Log line per refused field, naming the field and the winning
// value, and one line however the value is spelled.
func TestClockRejectionIsLoggedOnce(t *testing.T) {
	cases := []struct {
		name, body string
		want       []string // substrings, one line each
	}{
		{"duplicates: the winner", "clock:\n  hours: 7\n  hours: 13\n  dates: iso\n  dates: us\n", []string{`clock.hours "13"`, `clock.dates "us"`}},
		{"valid then invalid", "clock:\n  hours: 12\n  hours: soon\n", []string{`clock.hours "soon"`}},
		{"invalid then valid: silent", "clock:\n  hours: soon\n  hours: 12\n", nil},
		{"an escaped newline", "clock:\n  dates: \"a\\nb\"\n", []string{`clock.dates "a\nb"`}},
		{"a control byte", "clock:\n  hours: \"x\\x01y\"\n", []string{`clock.hours "x\x01y"`}},
		{"nothing refused", clockPrefsBody(clockCase{"12", "day-first"}), nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			writeClockPrefs(t, c.body)
			m := coldStart(t)
			var got []string
			for _, l := range m.logLines {
				if strings.Contains(l.text, "clock.") {
					got = append(got, l.text)
				}
			}
			if len(got) != len(c.want) {
				t.Fatalf("clock lines = %q, want %d", got, len(c.want))
			}
			for i, line := range got {
				if !strings.Contains(line, c.want[i]) {
					t.Errorf("line %d = %q, want it to name %s", i, line, c.want[i])
				}
				for _, r := range line {
					if r < 0x20 || r == 0x7f {
						t.Errorf("line %d holds the control character %q: %q", i, r, line)
					}
				}
			}
		})
	}
}

var (
	reHeader24  = regexp.MustCompile(`chart for [A-Z][a-z]{2} [A-Z][a-z]{2} [ 0-9]{2}, \d{4} [ 0-9]{1,2}:\d{2} \(`)
	reHeader12  = regexp.MustCompile(`chart for [A-Z][a-z]{2} [A-Z][a-z]{2} [ 0-9]{2}, \d{4} [ 0-9]{1,2}:\d{2}(am|pm) \(`)
	reHeader24D = regexp.MustCompile(`chart for [A-Z][a-z]{2} [ 0-9]{2} [A-Z][a-z]{2} \d{4} [ 0-9]{1,2}:\d{2} \(`)
	reHeader12D = regexp.MustCompile(`chart for [A-Z][a-z]{2} [ 0-9]{2} [A-Z][a-z]{2} \d{4} [ 0-9]{1,2}:\d{2}(am|pm) \(`)
)

func headerFormOK(c clockCase, report string) bool {
	re := map[clockCase]*regexp.Regexp{
		{"24", "month-first"}: reHeader24, {"12", "month-first"}: reHeader12,
		{"24", "day-first"}: reHeader24D, {"12", "day-first"}: reHeader12D,
	}[c]
	first, _, _ := strings.Cut(report, "\n")
	return re.MatchString(first)
}

func pinWords(c clockCase) (hours, dates string) {
	hours, dates = "'=Yt'", "_Yd"
	if c.hours == "12" {
		hours = "_Yt"
	}
	if c.dates == "day-first" {
		dates = "'=Yd'"
	}
	return
}

func commandsPinned(t *testing.T, label string, c clockCase, commands []string) {
	t.Helper()
	if len(commands) == 0 {
		t.Errorf("%s: no command carried", label)
	}
	h, d := pinWords(c)
	for _, cmd := range commands {
		words := strings.Fields(cmd)
		if !containsWord(words, h) || !containsWord(words, d) {
			t.Errorf("%s [%s]: command lacks %s %s: %s", label, c, h, d, cmd)
		}
		for _, w := range words {
			if w == "-Yt" || w == "-Yd" {
				t.Errorf("%s [%s]: a bare %s in %s", label, c, w, cmd)
			}
		}
	}
}

func containsWord(words []string, w string) bool {
	for _, x := range words {
		if x == w {
			return true
		}
	}
	return false
}

func needLiveAstrolog(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
}

// C12, N6, N24.
func TestClockPreferenceReachesFirstFrame(t *testing.T) {
	needLiveAstrolog(t)
	old := computeTimeline
	computeTimeline = lookup.Timeline
	t.Cleanup(func() { computeTimeline = old })

	for _, c := range clockCases {
		t.Run(c.String(), func(t *testing.T) {
			path := writeClockPrefs(t, clockPrefsBody(c))
			coldStart(t)

			text := func(t *testing.T, label string, m Model) {
				t.Helper()
				msg, ok := buildTextCmd(m, context.Background(), 1, 1)().(FirstFrameReadyMsg)
				if !ok || msg.Err != nil {
					t.Fatalf("%s: text compute failed: %#v", label, msg.Err)
				}
				commandsPinned(t, label+" chart call", c, msg.Commands)
				if !headerFormOK(c, msg.AspectsReport) {
					first, _, _ := strings.Cut(msg.AspectsReport, "\n")
					t.Errorf("%s [%s]: the report header is not in the preference's form: %q", label, c, first)
				}
			}
			timeline := func(t *testing.T, label string, m Model) {
				t.Helper()
				m, _ = syncTimelineVisited(m)
				cmd := buildTimelineCmd(m, context.Background(), 1, 1)
				if cmd == nil {
					t.Fatalf("%s: no timeline search", label)
				}
				msg, ok := cmd().(refreshResultMsg)
				if !ok || msg.err != nil {
					t.Fatalf("%s: timeline search: %#v", label, msg.err)
				}
				p, _ := msg.payload.(timelinePayload)
				commandsPinned(t, label+" timeline", c, p.commands)
				m, _ = m.commitTimeline(msg)
				if len(m.timeline.rows) == 0 {
					t.Fatalf("%s: the search showed no rows", label)
				}
				for _, row := range m.timeline.rows {
					twelve := strings.HasSuffix(row.Time, "am") || strings.HasSuffix(row.Time, "pm")
					dayFirst := strings.Contains(row.Date, "-")
					if twelve != (c.hours == "12") || dayFirst != (c.dates == "day-first") {
						t.Errorf("%s [%s]: row %q is not in the preference's form", label, c, row.Line)
						break
					}
				}
			}

			single := clockSingleModel(t)
			text(t, "first frame", single)
			timeline(t, "first search", detailsTimelineModel(t, Config{Accessible: true}))

			// After a time step.
			stepped, _ := press(t, single, "h")
			text(t, "after a step", stepped)
			tl, _ := press(t, detailsTimelineModel(t, Config{Accessible: true}), "h")
			timeline(t, "after a step", tl)

			// Editing the file while Sextile runs changes none of it (N24).
			other := clockCase{"12", "day-first"}
			if c == other {
				other = clockCase{"24", "month-first"}
			}
			if err := os.WriteFile(path, []byte(clockPrefsBody(other)), 0o644); err != nil {
				t.Fatal(err)
			}
			text(t, "after an edit", stepped)
			timeline(t, "after an edit", detailsTimelineModel(t, Config{Accessible: true}))
		})
	}
}

// clockSingleModel is a ready single-chart workspace (the floating slot alone)
// with Aspects focused and room for Details.
func clockSingleModel(t *testing.T) Model {
	t.Helper()
	m := readyModelWithSlots(t, berlinP)
	m.place = testPlaceBerlin()
	m.width, m.height = 132, 40
	m.focused = PanelAspects
	return m
}

// C13: a first frame, a time-stepped frame, and then opening Details/Aspects
// for a single chart start exactly the processes the frames start.
func TestAspectsDetailsAddsNoSpawn(t *testing.T) {
	spawns := fixtureAstrolog(t, nil)
	m := clockSingleModel(t)

	frame := func(m Model) FirstFrameReadyMsg {
		t.Helper()
		msg, ok := buildTextCmd(m, context.Background(), 1, 1)().(FirstFrameReadyMsg)
		if !ok || msg.Err != nil {
			t.Fatalf("text compute: %#v", msg.Err)
		}
		return msg
	}
	first := frame(m)
	if n := len(spawns()); n != 2 {
		t.Fatalf("the first frame started %d processes, want 2 (positions, then aspects)", n)
	}
	m, _ = applyTextCommit(m, first)
	stepped, _ := press(t, m, "h")
	second := frame(stepped)
	if n := len(spawns()); n != 4 {
		t.Fatalf("the stepped frame brought the total to %d, want 4", n)
	}
	m, _ = applyTextCommit(stepped, second)

	m = m.applyAspectsDetails()
	if n := len(spawns()); n != 4 {
		t.Errorf("opening Details/Aspects started %d more processes", n-4)
	}
	if m.details.body == "" || m.details.body != second.AspectsReport {
		t.Errorf("Details/Aspects is not the committed chart call's own text:\n%q", m.details.body)
	}
}

// pairClockModel is a ready pair workspace whose two charts sit in different
// IANA zones on different local dates: the floating slot in Tokyo, the anchored
// chart in New York, Aspects focused with the transit pair tab active.
func pairClockModel(t *testing.T) Model {
	t.Helper()
	m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTime()))
	tokyo := testPlaceTokyo()
	slots, err := store.NewSlots(time.Date(2026, 4, 25, 20, 30, 0, 0, time.UTC), tokyo)
	if err != nil {
		t.Fatal(err)
	}
	newYork := location.PlaceInfo{Name: "New York", Country: "US", Lat: 40.7128, Lon: -74.006, TZ: "America/New_York"}
	if err := addAnchor(slots, time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC), newYork); err != nil {
		t.Fatal(err)
	}
	m.slots = slots
	m.cursorIdx = 1
	m.place = tokyo
	m.targetFloatingTime = slots.Floating().Time
	m.startup = startupReady
	m.focused = PanelAspects
	m.width, m.height = 132, 40
	// Arrival at now lands on `Aspects` (the specification
	// C1); the pair tab is one real `]` away, and the recompute that press
	// starts is not pumped: the callers run the pair compute themselves.
	m, _ = tpress(t, m.noteArrival(), tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if m.activeTabID(PanelAspects) != TabAspectsCross {
		t.Fatalf("pairClockModel: active tab %q, want the pair tab", m.activeTabID(PanelAspects))
	}
	return m
}

func afterLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) < n {
		return ""
	}
	return strings.Join(lines[n:], "\n")
}

// C6: each chart's header in the rendered pair Details is its own clock and
// zone, and the pair's cross aspects are the UT run's, only the header
// differing.
func TestPairReportsReadEachChartsClock(t *testing.T) {
	needLiveAstrolog(t)
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
			if msg.CrossAspectsReport == "" || len(msg.CrossAspects) == 0 {
				t.Fatal("the pair compute returned no cross report")
			}
			st := m.refresh[surfaceText]
			msg.RunID, msg.WantID = st.runningRunID, st.wantID
			m, _ = applyTextCommit(m, msg)
			m = m.applyAspectsDetails()

			// Each header, in its own slot's clock and zone.
			natalWant := map[clockCase]string{
				{"24", "month-first"}: "Fri Jun 15, 1990 10:30 (DT Zone 5W)",
				{"24", "day-first"}:   "Fri 15 Jun 1990 10:30 (DT Zone 5W)",
				{"12", "month-first"}: "Fri Jun 15, 1990 10:30am (DT Zone 5W)",
				{"12", "day-first"}:   "Fri 15 Jun 1990 10:30am (DT Zone 5W)",
			}[c]
			transitWant := map[clockCase]string{
				{"24", "month-first"}: "Sun Apr 26, 2026  5:30 (ST Zone 9E)",
				{"24", "day-first"}:   "Sun 26 Apr 2026  5:30 (ST Zone 9E)",
				{"12", "month-first"}: "Sun Apr 26, 2026  5:30am (ST Zone 9E)",
				{"12", "day-first"}:   "Sun 26 Apr 2026  5:30am (ST Zone 9E)",
			}[c]
			lines := strings.Split(m.chart.crossAspectsReport, "\n")
			if len(lines) < 2 || !strings.Contains(lines[0], natalWant) || !strings.Contains(lines[1], transitWant) {
				t.Errorf("pair headers = %q / %q, want %q / %q", lines[0], lines[1], natalWant, transitWant)
			}
			view := m.View()
			for _, want := range []string{natalWant, transitWant} {
				if !strings.Contains(view, want) {
					t.Errorf("the rendered Details panel lacks %q", want)
				}
			}

			// The same pair compiled independently in UT: identical cross
			// aspects, identical report below the two headers.
			lens := m.viewLens()
			anchored := m.slots.All()[0]
			floating := m.slots.Floating()
			utReport, utAspects, err := testenv.UTTransitCross(context.Background(),
				anchored.Time, testenv.Place{Lon: anchored.Location.Lon, Lat: anchored.Location.Lat},
				floating.Time, testenv.Place{Lon: floating.Location.Lon, Lat: floating.Location.Lat},
				lens.Houses, lens.Zodiac, lens.Objects, lens.Aspects)
			if err != nil {
				t.Fatal(err)
			}
			if a, b := afterLines(m.chart.crossAspectsReport, 2), afterLines(utReport, 2); a != b {
				t.Errorf("the pair report below its headers differs from the UT run:\n%s\n---\n%s", a, b)
			}
			if len(msg.CrossAspects) != len(utAspects) {
				t.Fatalf("cross aspects = %d, UT run = %d", len(msg.CrossAspects), len(utAspects))
			}
			for i, a := range msg.CrossAspects {
				u := utAspects[i]
				if a.Transit != u.Secondary || a.Natal != u.Primary || a.Kind != u.Kind || a.Orb != u.Orb || a.Power != u.Power || a.Applying != u.Applying {
					t.Errorf("cross aspect %d differs from the UT run: %+v vs %+v", i, a, u)
				}
			}
		})
	}
}

var (
	reDegreeOrb   = regexp.MustCompile(`orb: [as]\d+°\d{2}'`)
	reColonOrb    = regexp.MustCompile(`orb: [as]?\d+:\d{2}'`)
	reStagedChart = regexp.MustCompile(`'?(/[^ ']*/chart([01])\.as)'?`)
)

// plainReport is what a report must equal: Astrolog's own output with only the
// SGR sequences stripped and the CP437 degree byte (0xF8) read as U+00B0.
func plainReport(raw string) string {
	return strings.ReplaceAll(astrologstyle.StripSGR(raw), "\xf8", "°")
}

// C8: every raw report Details shows is asked with the degree character
// (-k0) and reaches the screen unchanged otherwise, so each orb reads a0°04',
// never 0:04'. Each report is compared whole with live Astrolog output.
func TestEveryReportPrintsDegreeOrbs(t *testing.T) {
	needLiveAstrolog(t)
	writeClockPrefs(t, clockPrefsBody(clockCase{"24", "month-first"}))
	coldStart(t)

	lens := lookup.Hellenistic
	place := testPlaceBerlin()
	newYork := location.PlaceInfo{Name: "New York", Country: "US", Lat: 40.7128, Lon: -74.006, TZ: "America/New_York"}
	when := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	natal := store.AnchorChart{When: time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC), Place: newYork}
	other := time.Date(1985, 3, 21, 14, 30, 0, 0, time.UTC)

	type report struct {
		name    string
		text    string
		command string
		staged  []stagedChart // chart0, chart1 of a relationship
		orbs    bool
	}
	var reports []report
	for _, k := range []struct {
		name string
		req  lookup.RawReportRequest
		orbs bool
	}{
		{"chart", lookup.RawReportRequest{Kind: lookup.RawReportChart}, false},
		{"aspects", lookup.RawReportRequest{Kind: lookup.RawReportAspects}, true},
		{"body", lookup.RawReportRequest{Kind: lookup.RawReportBody, Body: aspect.Sun}, true},
	} {
		text, command, err := lookup.ComputeRawReport(context.Background(), when, place, lens, k.req)
		if err != nil {
			t.Fatalf("%s: %v", k.name, err)
		}
		reports = append(reports, report{name: k.name, text: text, command: command, orbs: k.orbs})
	}
	local := func(w time.Time, p location.PlaceInfo) stagedChart { return stagedChart{w, p} }
	text, command, err := lookup.CrossBodyRawReport(context.Background(), lens, natal, when, place, aspect.Sun)
	if err != nil {
		t.Fatalf("transit: %v", err)
	}
	reports = append(reports, report{"transit cross body", text, command, []stagedChart{local(natal.When, newYork), local(when, place)}, true})
	text, command, err = lookup.SynastryCrossBodyRawReport(context.Background(), lens, natal, other, place, store.ReportLabel{}, aspect.Sun)
	if err != nil {
		t.Fatalf("synastry: %v", err)
	}
	reports = append(reports, report{"synastry cross body", text, command, []stagedChart{local(natal.When, newYork), local(other, place)}, true})

	applying, separating := 0, 0
	for _, r := range reports {
		t.Run(r.name, func(t *testing.T) {
			if !strings.Contains(" "+r.command+" ", " -k0 ") {
				t.Errorf("the report was not asked with -k0: %s", r.command)
			}
			if strings.ContainsRune(r.text, 0xF8) || strings.Contains(r.text, "\x1b[") {
				t.Errorf("the report still holds a raw degree byte or an SGR sequence")
			}
			if reColonOrb.MatchString(r.text) {
				t.Errorf("an orb still reads with a colon: %q", reColonOrb.FindString(r.text))
			}
			if r.orbs && !reDegreeOrb.MatchString(r.text) {
				t.Errorf("no orb reads with the degree character")
			}
			applying += strings.Count(r.text, "orb: a")
			separating += strings.Count(r.text, "orb: s")

			// Stage the relationship's chart files where the recorded command
			// names them (they are gone by now), then run it as pasted.
			for _, m := range reStagedChart.FindAllStringSubmatch(r.command, -1) {
				idx := int(m[2][0] - '0')
				if idx >= len(r.staged) {
					t.Fatalf("command names chart%d.as but only %d are staged", idx, len(r.staged))
				}
				content, err := testenv.LocalChartFile(r.staged[idx].when, r.staged[idx].place.Lon, r.staged[idx].place.Lat, r.staged[idx].place.TZ, lens.Houses, lens.Zodiac)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Dir(m[1]), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(m[1], []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
				staged := m[1]
				t.Cleanup(func() { os.Remove(staged) })
			}
			pasted, err := exec.Command("sh", "-c", r.command).Output()
			if err != nil {
				t.Fatalf("running the recorded command: %v", err)
			}
			if got := plainReport(string(pasted)); got != r.text {
				t.Errorf("the shown report differs from live Astrolog output\n--- shown\n%.600s\n--- live\n%.600s", r.text, got)
			}
		})
	}
	if applying == 0 || separating == 0 {
		t.Errorf("the reports held %d applying and %d separating orbs; the control needs both", applying, separating)
	}
}

// stagedChart is one chart of a relationship, as its staged file will say it.
type stagedChart struct {
	when  time.Time
	place location.PlaceInfo
}
