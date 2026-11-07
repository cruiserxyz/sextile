package ui

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// body-details-base: on a pair tab the Details report is asked for the
// cursor chart (②) against the base (①); lookup keeps only ②'s side of the rows,
// so the ui asks with ① as chart 1 and ② as the other end, and shows the text as
// it comes back.
func pairVenusModel(t *testing.T) Model {
	t.Helper()
	m := tabsOnPair(t, Config{Accessible: true})
	m.width, m.height = 144, 40
	m.focused = PanelBodies
	m.chart.positions = []aspect.Position{{Object: objSun}, {Object: objVenus}}
	m.bodyCursorIdx = 1
	return m
}

func TestPairTabBodyDetailsAsksForTheCursorChartAgainstTheBase(t *testing.T) {
	m := pairVenusModel(t)
	req, ok := m.detailsRequestFor()
	if !ok || !req.cross {
		t.Fatalf("ok=%v cross=%v, want a cross request", ok, req.cross)
	}
	if req.report.Object != objVenus {
		t.Errorf("body = %v, want Venus", req.report.Object)
	}
	if req.natalChart != anchorHandle(m) {
		t.Errorf("chart 1 = %+v, want the base %+v", req.natalChart, anchorHandle(m))
	}
	if !req.when.Equal(m.slots.Floating().Time) {
		t.Errorf("other end = %v, want the cursor chart's committed time %v", req.when, m.slots.Floating().Time)
	}
	if req.title != "[0] Venus 2 in 1" {
		t.Errorf("title = %q, want the pair named like the Aspects tab", req.title)
	}
	var cross *[]crossBodyCall
	m = withNewStubs(t, m, func() { cross = stubCrossBodyReports(t) })
	msg := runDetailsCmd(t, m)
	if len(*cross) != 1 || (*cross)[0].body != objVenus {
		t.Fatalf("cross calls = %+v", *cross)
	}
	if got := msg.payload.(detailsPayload).body; got != "cross raw" {
		t.Errorf("the ui changed the report text: %q", got)
	}
}

func TestSwitchingPairTabKeepsTheBodiesCursorAndRows(t *testing.T) {
	m := pairVenusModel(t)
	positions := append([]aspect.Position(nil), m.chart.positions...)
	before, _ := m.detailsRequestFor()
	m.focused = PanelAspects
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	m.focused = PanelBodies
	if m.bodyCursorIdx != 1 {
		t.Errorf("Bodies cursor moved to %d", m.bodyCursorIdx)
	}
	if len(m.chart.positions) != len(positions) || m.chart.positions[1] != positions[1] {
		t.Errorf("Bodies rows changed: %+v", m.chart.positions)
	}
	after, ok := m.detailsRequestFor()
	if !ok || after.report.Object != objVenus {
		t.Fatalf("ok=%v body=%v", ok, after.report.Object)
	}
	if after.cross && after.title == before.title {
		t.Errorf("the Details request did not follow the tab: %q", after.title)
	}
}

func TestOwnTabBodyDetailsIsTheChartsOwnReport(t *testing.T) {
	m := pairVenusModel(t)
	m.focused = PanelAspects
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	m.focused = PanelBodies
	if got := m.activeTabID(PanelAspects); got != TabAspects {
		t.Fatalf("active tab %q, want the own Aspects tab", got)
	}
	req, ok := m.detailsRequestFor()
	if !ok || req.cross || req.title != "[0] Venus" {
		t.Fatalf("ok=%v cross=%v title=%q, want the single-chart report", ok, req.cross, req.title)
	}
	var single *[]rawReportCall
	var cross *[]crossBodyCall
	m = withNewStubs(t, m, func() { single, cross = stubRawReports(t), stubCrossBodyReports(t) })
	msg := runDetailsCmd(t, m)
	if len(*single) != 1 || len(*cross) != 0 || msg.payload.(detailsPayload).body != "single raw" {
		t.Errorf("single=%d cross=%d payload=%+v", len(*single), len(*cross), msg.payload)
	}
}

// The report committed for a pair tab's body was filtered by lookup.KeepCursorSide
// (the cursor chart's rows only), so the large log says a rule was applied after
// Astrolog: one trace-lane line right after the argv line, with the body, the
// cursor chart's numeral and the counts. No line on the own tab.
func TestPairTabBodyReportCommitsWithTheFilterNoteInTheConsole(t *testing.T) {
	fixture, err := os.ReadFile("../lookup/testdata/f3-venus-cross.txt")
	if err != nil {
		t.Fatal(err)
	}
	stubSpawns(t, func(s *rowSpawns) {
		s.reports.cross = func(context.Context, lookup.Lens, store.AnchorChart, time.Time, location.PlaceInfo, aspect.Object) (string, string, error) {
			return string(fixture), "astrolog -fixture", nil
		}
	})

	commit := func(m Model) Model {
		msg := runDetailsCmd(t, m)
		m, _ = m.commitRefresh(surfaceDetails, msg)
		return m
	}
	want := "details: kept the rows where Venus is 2's: 3 of 11 rows, totals dropped"

	m := pairVenusModel(t)
	m.cfg.Color = ColorNever
	m = commit(m)
	lines := consoleLineTexts(m)
	at := -1
	for i, l := range lines {
		if l == "astrolog -fixture" {
			at = i
		}
	}
	if at < 0 || at+1 >= len(lines) || lines[at+1] != want {
		t.Errorf("console = %q, want the argv line then %q", lines, want)
	}
	body := m.details.body
	if strings.Contains(body, "Uranus") || !strings.Contains(body, "Venus") || strings.Contains(body, "Sum power") {
		t.Errorf("Details shows the unfiltered report:\n%s", body)
	}

	// With colours on the numeral is the circled one, as the tab title spells it.
	c := pairVenusModel(t)
	c.cfg.Color = ColorAlways
	c.cfg.Accessible = false
	c = commit(c)
	if got := strings.Join(consoleLineTexts(c), "\n"); !strings.Contains(got, "Venus is ②'s: 3 of 11 rows") {
		t.Errorf("coloured Log lacks the circled numeral line:\n%s", got)
	}

	// The own tab: a single-chart report, no note.
	o := pairVenusModel(t)
	o.focused = PanelAspects
	o, _ = tpress(t, o, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	o.focused = PanelBodies
	o = withNewStubs(t, o, func() { stubRawReports(t) })
	o = commit(o)
	for _, l := range consoleLineTexts(o) {
		if strings.HasPrefix(l, "details:") {
			t.Errorf("own tab wrote a details note: %q", l)
		}
	}
}
