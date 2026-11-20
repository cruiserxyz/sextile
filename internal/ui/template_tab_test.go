package ui

import (
	"context"
	"fmt"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/rules"
	"github.com/cruiserxyz/sextile/internal/templates"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// templateTestModel is a ready workspace with Bodies focused and the cursor
// on Venus, the body the shipped template lists (rows: Sun, Moon, Venus).
func templateTestModel(t *testing.T) Model {
	t.Helper()
	m := detailsTestModel(t)
	m.chart.positions = []aspect.Position{{Object: objSun}, {Object: objMoon}, {Object: objVenus}}
	m.focused = PanelBodies
	m.bodyCursorIdx = 2
	return m
}

// bodyFactsCall is one call the stubbed facts producer received.
type bodyFactsCall struct {
	req          lookup.BodyFactsRequest
	reusedCycle  bool
	cycleStarted int
}

// stubBodyFacts replaces the facts producer with canned facts for the
// research catalogue's sky, recording each request.
func stubBodyFacts(t *testing.T, fail error) *[]bodyFactsCall {
	t.Helper()
	calls := &[]bodyFactsCall{}
	stubSpawns(t, func(s *rowSpawns) {
		s.bodyFacts = func(_ context.Context, req lookup.BodyFactsRequest) (lookup.BodyFactsResult, error) {
			zone, _ := time.LoadLocation(req.Place.TZ)
			res := lookup.BodyFactsResult{Commands: []string{"astrolog -qj … -b0 -v -Z -R0 1 4 -RA0 1"}}
			c := bodyFactsCall{req: req, reusedCycle: req.Cycle != nil}
			if req.Cycle != nil {
				res.Cycle = *req.Cycle
			} else {
				res.Cycle = lookup.CycleResult{Span: lookup.CycleSpanFor(req.When, zone), Commands: []string{"astrolog -z 0 -qy … -dY 13 -b0"}}
				res.Commands = append(append([]string(nil), res.Cycle.Commands...), res.Commands...)
				c.cycleStarted = 1
			}
			*calls = append(*calls, c)
			if fail != nil {
				return res, fail
			}
			res.Now = researchSkyFacts()
			return res, nil
		}
	})
	return calls
}

// researchSkyFacts are lookup's facts for 2026-10-14 12:00 UT in Berlin,
// printed from the live Astrolog 8.00 binary (lookup's
// TestCycleAgainstHorizons2026 pins the same values at UT).
func researchSkyFacts() map[string]lookup.BodyFact {
	v, c, i, l := lookup.FactValue, lookup.FactCategory, lookup.FactInstant, lookup.FactList
	return map[string]lookup.BodyFact{
		"brightness.lit":                     {Kind: v, Text: "4%", Num: 4.03},
		"brightness.magnitude":               {Kind: v, Text: "−4.3", Num: -4.32},
		"elongation.lon":                     {Kind: v, Text: "14°52'", Num: 14.874166666666667},
		"elongation.lon.abs":                 {Kind: v, Text: "14°52'", Num: 14.874166666666667},
		"elongation.side":                    {Kind: c, Term: "east"},
		"elongation.trend":                   {Kind: c, Term: "approaching"},
		"motion.direction":                   {Kind: c, Term: "retrograde"},
		"sign.same-as-sun":                   {Kind: c, Term: "no"},
		"station.last":                       {Kind: i, Text: "Oct 3, 2026 9:16 8°29' Sco", Term: "station-retrograde"},
		"station.next":                       {Kind: i, Text: "Nov 14, 2026 1:27 22°51' Lib", Term: "station-direct"},
		"synodic.conjunction.inferior.drift": {Kind: v, Text: "−2°18'", Num: -2.3},
		"synodic.conjunction.inferior.last":  {Kind: i, Text: "Mar 23, 2025 2:08 2°39' Ari"},
		"synodic.conjunction.inferior.last5": {Kind: l, Items: []string{"Mar 23, 2025   2°39' Ari", "Aug 13, 2023  20°28' Leo", "Jan  9, 2022  18°43' Cap", "Jun  3, 2020  13°35' Gem", "Oct 26, 2018   3°06' Sco"}},
		"synodic.conjunction.inferior.next":  {Kind: i, Text: "Oct 24, 2026 5:44 0°45' Sco"},
		"synodic.conjunction.inferior.sixth": {Kind: v, Text: "Mar 25, 2017   4°57' Ari", Num: 4.95},
		"synodic.conjunction.superior.last":  {Kind: i, Text: "Jan 6, 2026 17:36 16°22' Cap"},
		"synodic.conjunction.superior.next":  {Kind: i, Text: "Aug 12, 2027 2:21 19°06' Leo"},
		"synodic.phase":                      {Kind: c, Term: "evening-star"},
		"synodic.phase.alt":                  {Kind: c, Term: "occidental"},
	}
}

// conjunctionFacts are the same for 2026-10-24 03:44 UT (05:44 in Berlin),
// the inner conjunction's minute: elongation 0 to the second, so no trend.
func conjunctionFacts() map[string]lookup.BodyFact {
	f := researchSkyFacts()
	v, c := lookup.FactValue, lookup.FactCategory
	f["brightness.lit"] = lookup.BodyFact{Kind: v, Text: "1%", Num: 0.61}
	f["brightness.magnitude"] = lookup.BodyFact{Kind: v, Text: "−4.2", Num: -4.19}
	f["elongation.lon"] = lookup.BodyFact{Kind: v, Text: "0°00'", Num: 0}
	f["elongation.lon.abs"] = lookup.BodyFact{Kind: v, Text: "0°00'", Num: 0}
	f["elongation.side"] = lookup.BodyFact{Kind: c, Term: "at-conjunction"}
	f["elongation.trend"] = lookup.BodyFact{Kind: c, Absent: true}
	f["sign.same-as-sun"] = lookup.BodyFact{Kind: c, Term: "yes"}
	f["synodic.phase"] = lookup.BodyFact{Kind: c, Term: "at-conjunction"}
	f["synodic.phase.alt"] = lookup.BodyFact{Kind: c, Term: "at-conjunction"}
	return f
}

// runTemplate syncs the tab and runs its command to a committed result.
func runTemplate(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = m.syncTemplate()
	st := m.refresh[surfaceTemplate]
	cmd := buildTemplateCmd(m, context.Background(), st.runningRunID+1, st.wantID)
	if cmd == nil {
		t.Fatal("buildTemplateCmd returned nil")
	}
	msg := cmd().(refreshResultMsg)
	m, _ = m.commitTemplate(msg)
	return m
}

func activeOnTemplate(m Model) Model {
	tabs := m.panelTabs(PanelDetails)
	return m.withDetailsTab(PanelBodies, tabs[len(tabs)-1].ID)
}

// P1: the tab is declared on Venus's row in Bodies, titled by the template,
// and nowhere else.
func TestTemplateTabOnlyOnItsBodies(t *testing.T) {
	m := templateTestModel(t)
	tabs := m.panelTabs(PanelDetails)
	if len(tabs) != 2 || tabs[1].ID != "tpl:body-inferior-planet" || tabs[1].Title != "Details" {
		t.Fatalf("on Venus: %+v", tabs)
	}
	m.bodyCursorIdx = 1
	if tabs := m.panelTabs(PanelDetails); len(tabs) != 1 {
		t.Errorf("on the Moon: %+v", tabs)
	}
	m.bodyCursorIdx = 2
	m.focused = PanelChart
	for _, tab := range m.panelTabs(PanelDetails) {
		if isTemplateTab(tab.ID) {
			t.Errorf("Chart focused: %+v", tab)
		}
	}
}

// P2: arriving on the row keeps the active tab; a body no template lists
// shows the report without forgetting the stored choice, so Venus gets its
// tab back; `0` cycles [report, Details] from anywhere, `[` and `]` while
// Details is attached.
func TestTemplateTabChoiceAndKeys(t *testing.T) {
	m := templateTestModel(t)
	if m.activeTabID(PanelDetails) != TabDetailsReport {
		t.Fatalf("arriving: %q", m.activeTabID(PanelDetails))
	}
	m, _ = press(t, m, "0")
	if !isTemplateTab(m.activeTabID(PanelDetails)) {
		t.Fatalf("after 0: %q", m.activeTabID(PanelDetails))
	}
	m.bodyCursorIdx = 1
	if m.activeTabID(PanelDetails) != TabDetailsReport || !isTemplateTab(m.tabChoice(PanelDetails)) {
		t.Errorf("on the Moon: active %q, stored %q", m.activeTabID(PanelDetails), m.tabChoice(PanelDetails))
	}
	m.bodyCursorIdx = 2
	if !isTemplateTab(m.activeTabID(PanelDetails)) {
		t.Errorf("back on Venus: %q", m.activeTabID(PanelDetails))
	}
	m, _ = press(t, m, "0")
	if m.activeTabID(PanelDetails) != TabDetailsReport {
		t.Errorf("after 0 again: %q", m.activeTabID(PanelDetails))
	}
	m.details.attached = true
	m, _ = press(t, m, "]")
	if !isTemplateTab(m.activeTabID(PanelDetails)) {
		t.Errorf("attached, after ]: %q", m.activeTabID(PanelDetails))
	}
	m, _ = press(t, m, "[")
	if m.activeTabID(PanelDetails) != TabDetailsReport {
		t.Errorf("attached, after [: %q", m.activeTabID(PanelDetails))
	}
}

// P10: the Log carries every argv behind the tab's facts and each verdict's
// rule row id; the body shows the rendered lines.
func TestTemplateTabLogsArgvAndRuleRows(t *testing.T) {
	stubBodyFacts(t, nil)
	m := activeOnTemplate(templateTestModel(t))
	m = runTemplate(t, m)
	var log []string
	for _, l := range m.logLines {
		log = append(log, l.text)
	}
	all := strings.Join(log, "\n")
	for _, want := range []string{"-dY 13 -b0", "-b0 -v -Z -R0 1 4 -RA0 1", "solar.beams/17", "solar.beams/12", "solar.combust/8.5"} {
		if !strings.Contains(all, want) {
			t.Errorf("the Log lacks %q:\n%s", want, all)
		}
	}
	if len(m.tpl.lines) == 0 || m.tpl.lines[0].Label != "elongation" || m.tpl.lines[0].Value != "14°52' east" {
		t.Errorf("lines %+v", m.tpl.lines)
	}
	if body := renderDetailsPanel(80, 12, false, m.cfg, m.detailsFrame()); !strings.Contains(body, "under the beams    yes") {
		t.Errorf("the panel does not show the verdict:\n%s", body)
	}
	// After the Log has evicted the cycle search's argv, a frame built on the
	// kept search writes it again.
	m = m.logCommands(func() []string {
		var many []string
		for i := 0; i < MaxLogLines; i++ {
			many = append(many, fmt.Sprintf("argv %03d", i))
		}
		return many
	}())
	if m.logHasArgv("astrolog -z 0 -qy … -dY 13 -b0") {
		t.Fatal("the fixture did not evict the cycle argv")
	}
	f := m.slots.Floating()
	m.slots.SetFloatingTime(f.Time.Add(time.Hour))
	m = runTemplate(t, m)
	if !m.logHasArgv("astrolog -z 0 -qy … -dY 13 -b0") {
		t.Error("a frame on the kept cycle search shows facts whose argv the Log no longer carries")
	}
}

// P5: a failed run keeps the last complete frame and says why in the Log;
// another body's lines are never left under this one's tab.
func TestTemplateFailureKeepsTheFrame(t *testing.T) {
	stubBodyFacts(t, nil)
	m := runTemplate(t, activeOnTemplate(templateTestModel(t)))
	before := fmt.Sprint(m.tpl.lines)
	if len(m.tpl.lines) == 0 {
		t.Fatal("no first frame")
	}
	m = withNewStubs(t, m, func() { stubBodyFacts(t, os.ErrDeadlineExceeded) })
	f := m.slots.Floating()
	m.slots.SetFloatingTime(f.Time.Add(time.Hour))
	m = runTemplate(t, m)
	if fmt.Sprint(m.tpl.lines) != before {
		t.Errorf("a failed run replaced the frame: %+v", m.tpl.lines)
	}
	if last := m.logLines[len(m.logLines)-1]; last.lane != laneError || !strings.HasPrefix(last.text, "details: the body-inferior-planet tab did not fill") {
		t.Errorf("last Log line %+v", last)
	}
	// A different template key (another body) drops the lines at once.
	m.tpl.key.object = objMercury.Number
	m, _ = m.syncTemplate()
	if len(m.tpl.lines) != 0 {
		t.Errorf("another body's lines stayed: %+v", m.tpl.lines)
	}
}

// P11: the cycle search runs once per span; a time step inside the span
// reuses it, one into another span searches again.
func TestCycleWindowFetchedOncePerSpan(t *testing.T) {
	calls := stubBodyFacts(t, nil)
	m := runTemplate(t, activeOnTemplate(templateTestModel(t)))
	f := m.slots.Floating()
	m.slots.SetFloatingTime(f.Time.Add(24 * time.Hour))
	m = runTemplate(t, m)
	m.slots.SetFloatingTime(f.Time.AddDate(1, 0, 0))
	m = runTemplate(t, m)
	if len(*calls) != 3 || (*calls)[0].reusedCycle || !(*calls)[1].reusedCycle || (*calls)[2].reusedCycle {
		t.Errorf("calls %+v: want a search, a reuse inside the span, a search for the next span", *calls)
	}
}

// V1 (the specification) — the @now section at rest, for the research
// catalogue's sky (2026-10-14 12:00 UT) and the inner conjunction's minute
// (2026-10-24 05:44 in Berlin), at the Details panel's size in an 80x24 and a
// two-column layout, colours on and off, in the reading contract's three columns
// (the specification R1-R10).
func TestVenusNowTaste(t *testing.T) {
	forceColor(t)
	m := activeOnTemplate(templateTestModel(t))
	tpl, _, _ := templates.ForObject(shippedTemplates, objVenus)
	var sheet strings.Builder
	for _, frame := range []struct {
		name  string
		facts map[string]lookup.BodyFact
	}{{"2026-10-14 12:00 UT", researchSkyFacts()}, {"2026-10-24 05:44 Berlin", conjunctionFacts()}} {
		r, err := templates.Render(tpl, "venus", map[string]templates.Section{"now": factSection(frame.facts)}, shippedRules)
		if err != nil {
			t.Fatal(err)
		}
		m.tpl.lines, m.tpl.complete = r.Lines, true
		// A hand-built frame is a settled one: its key is the current inputs'
		// and its run ended, so the tab is not waiting and does not dim.
		if _, _, key, ok := m.templateInputs(); ok {
			m.tpl.key, m.tpl.keyed, m.tpl.done, m.tpl.hasDone = key, true, key, true
			m.tpl.content, m.tpl.hasContent = key, true
		}
		for _, mode := range []struct {
			name string
			cfg  Config
		}{{"color", Config{Color: ColorAlways}}, {"never", Config{Color: ColorNever}}} {
			for _, size := range [][2]int{{80, 24}, {dualColumnMinWidth(len(m.chart.positions)), 40}} {
				rects := rectsAt(size[0], size[1], chartContentLines(m.slots), len(m.chart.positions))
				d, ok := rects[PanelDetails]
				if !ok {
					fmt.Fprintf(&sheet, "== %s, %s %dx%d: no Details panel in this layout ==\n", frame.name, mode.name, size[0], size[1])
					continue
				}
				fmt.Fprintf(&sheet, "== %s, %s %dx%d: Details %dx%d ==\n%s\n", frame.name, mode.name, size[0], size[1], d.w, d.h, renderDetailsPanel(d.w, d.h, false, mode.cfg, m.detailsFrame()))
			}
		}
	}
	checkGolden(t, "details-venus-now", sheet.String())
}

// C9: the tab follows the Bodies list: with Venus unticked there is no Venus
// row and no tab; with the Sun unticked the tab and its request are as
// before, because the facts' calls name the Sun and the body themselves.
func TestTemplateTabFollowsTheChooser(t *testing.T) {
	m := activeOnTemplate(templateTestModel(t))
	_, before, _, ok := m.templateInputs()
	if !ok {
		t.Fatal("no request on Venus")
	}
	noSun := m
	noSun.chart.positions = []aspect.Position{{Object: objMoon}, {Object: objVenus}}
	noSun.bodyCursorIdx = 1
	noSun.lens.Objects.Remove = recipe.NewObjectSet(objSun.Number)
	if !isTemplateTab(noSun.activeTabID(PanelDetails)) {
		t.Errorf("Sun unticked: the tab is gone (%q)", noSun.activeTabID(PanelDetails))
	}
	_, after, _, ok := noSun.templateInputs()
	if !ok || after.Object != before.Object || !after.When.Equal(before.When) || after.Place != before.Place {
		t.Errorf("Sun unticked: request %+v, before %+v", after, before)
	}
	noVenus := m
	noVenus.chart.positions = []aspect.Position{{Object: objSun}, {Object: objMoon}}
	noVenus.bodyCursorIdx = 1
	for _, tab := range noVenus.panelTabs(PanelDetails) {
		if isTemplateTab(tab.ID) {
			t.Errorf("Venus unticked: %+v", tab)
		}
	}
}

// Failure mode: a search that succeeded but lacks an event a line needs is a
// failure (lookup refuses it), so the frame stays and the Log says why.
func TestMissingEventKeepsTheFrame(t *testing.T) {
	stubBodyFacts(t, nil)
	m := runTemplate(t, activeOnTemplate(templateTestModel(t)))
	before := fmt.Sprint(m.tpl.lines)
	m = withNewStubs(t, m, func() { stubBodyFacts(t, &lookup.MissingEventError{Event: "a station before and after the moment"}) })
	f := m.slots.Floating()
	m.slots.SetFloatingTime(f.Time.Add(time.Hour))
	m = runTemplate(t, m)
	if fmt.Sprint(m.tpl.lines) != before {
		t.Errorf("a missing event replaced the frame: %+v", m.tpl.lines)
	}
	if last := m.logLines[len(m.logLines)-1]; last.lane != laneError || last.text != "details: the body-inferior-planet tab did not fill, Astrolog's list of events has no a station before and after the moment" {
		t.Errorf("last Log line %+v", last)
	}
}

// T14: an expression slot that ran writes one Log line naming the template,
// the file line, its ordinal on the line and what it chose, through the
// production path (a loaded format 2 template, the facts seam, buildTemplateCmd
// and commitTemplate); never the fact's value.
func TestTemplateTabLogsSlotBranch(t *testing.T) {
	stubBodyFacts(t, nil)
	// The second slot of line 10 reaches an absent fact: the line is omitted,
	// but its first slot ran and is still logged.
	stubSpawns(t, func(s *rowSpawns) {
		stubbed := s.bodyFacts
		if stubbed == nil {
			return
		}
		s.bodyFacts = func(ctx context.Context, req lookup.BodyFactsRequest) (lookup.BodyFactsResult, error) {
			res, err := stubbed(ctx, req)
			res.Now["elongation.lon.abs"] = lookup.BodyFact{Kind: lookup.FactValue, Absent: true}
			return res, err
		}
	})
	allow, err := os.ReadFile("../templates/allowlist.txt")
	if err != nil {
		t.Fatal(err)
	}
	src := "---\nformat: 2\norigin: shipped\nid: log-fixture\ntab: Details\nsubject: {kind: body, class: inferior-planet, bodies: [venus]}\n---\n## @now\n" +
		"{term:elongation} {elongation.side == term(\"east\") ? term(\"evening-star\") : term(\"morning-star\")}\n{term:elongation} {elongation.side == term(\"east\") ? term(\"evening-star\") : term(\"morning-star\")} {has(elongation.lon) ? elongation.lon.abs : term(\"west\")}\n{term:elongation} {elongation.lon}\n"
	ts, err := templates.LoadFS(fstest.MapFS{"log-fixture.md": {Data: []byte(src)}, "allowlist.txt": {Data: allow}}, factRegistry{}, shippedRules)
	if err != nil {
		t.Fatal(err)
	}
	old := shippedTemplates
	t.Cleanup(func() { shippedTemplates = old })
	shippedTemplates = ts
	m := runTemplate(t, activeOnTemplate(templateTestModel(t)))
	var slotLines []string
	var all []string
	for _, l := range m.logLines {
		all = append(all, l.text)
		if strings.HasPrefix(l.text, "template: log-fixture:") {
			slotLines = append(slotLines, l.text)
		}
	}
	if len(slotLines) != 2 || slotLines[0] != "template: log-fixture:9/1 → evening-star" || slotLines[1] != "template: log-fixture:10/1 → evening-star" {
		t.Errorf("slot lines %q", slotLines)
	}
	if strings.Contains(strings.Join(all, "\n"), "14°52'") {
		t.Errorf("the Log carries a fact's value:\n%s", strings.Join(all, "\n"))
	}
	if len(m.tpl.lines) != 2 || m.tpl.lines[0].Value != "evening star" || m.tpl.lines[1].Value != "14°52'" {
		t.Errorf("lines %+v", m.tpl.lines)
	}
}

// 3c of the page-cursor specification: the Details scroll measures the ACTIVE
// tab. A template tab longer than the report scrolls to its own last line, and
// the scroll key is offered while the tab overflows although the report fits.
func TestTemplateTabScrollsItsOwnLines(t *testing.T) {
	m := activeOnTemplate(templateTestModel(t))
	m.details.title, m.details.body = "[0] Venus", "one short report line"
	lines := make([]templates.Line, 0, 90)
	for i := 0; i < 90; i++ {
		lines = append(lines, templates.Line{Label: fmt.Sprintf("row %02d", i), Value: "x"})
	}
	m.tpl.lines, m.tpl.complete = lines, true
	// A hand-built frame is a settled one (as in TestVenusNowTaste): its key is
	// the current inputs', so a key press does not drop it as another body's.
	if _, _, key, ok := m.templateInputs(); ok {
		m.tpl.key, m.tpl.keyed, m.tpl.done, m.tpl.hasDone = key, true, key, true
		m.tpl.content, m.tpl.hasContent = key, true
	}

	total, height := m.detailsScrollSpan()
	if total <= height {
		t.Fatalf("fixture: the template tab (%d lines) must overflow its panel (%d rows)", total, height)
	}
	if want := len(panel.TemplateBodyLines(lines, m.geometry()[PanelDetails].w-2, m.cfg)); total != want {
		t.Fatalf("scroll span = %d lines, want the template's own %d", total, want)
	}
	if len(m.detailsView().Lines) > height {
		t.Fatalf("fixture: the report (%d lines) must fit, or the test cannot tell the tabs apart", len(m.detailsView().Lines))
	}
	if !m.detailsScrolls() {
		t.Error("detailsScrolls is false on an overflowing template tab")
	}
	for i := 0; i < total; i++ {
		m, _ = pressScroll(t, m, tea.KeyCtrlPgDown)
	}
	if want := total - height; m.details.scrollOffset != want {
		t.Errorf("Ctrl+PgDn held: offset %d, want the template's last page %d", m.details.scrollOffset, want)
	}
	for i := 0; i < total; i++ {
		m, _ = pressScroll(t, m, tea.KeyCtrlPgUp)
	}
	if m.details.scrollOffset != 0 {
		t.Errorf("Ctrl+PgUp held: offset %d, want 0", m.details.scrollOffset)
	}
}

// Any row with an Object finds the template that lists it: Ceres, which has
// no glyph, opens a template listing `ceres` while Venus keeps the shipped
// one; the facts are asked for object 12, the key carries the number and the
// subject id, and Render gets that subject (two rule rows are written for one
// body each, so only the row for `ceres` may show).
func TestTemplateTabForAnUntypedObject(t *testing.T) {
	calls := stubBodyFacts(t, nil)
	allow, err := os.ReadFile("../templates/allowlist.txt")
	if err != nil {
		t.Fatal(err)
	}
	src := "---\nformat: 2\norigin: shipped\nid: ceres-fixture\ntab: Details\nsubject: {kind: body, class: dwarf-planet, bodies: [ceres]}\n---\n## @now\n" +
		"{term:elongation} {elongation.lon} {elongation.side}\n{rule:solar.beams}\n"
	ts, err := templates.LoadFS(fstest.MapFS{"ceres-fixture.md": {Data: []byte(src)}, "allowlist.txt": {Data: allow}}, factRegistry{}, shippedRules)
	if err != nil {
		t.Fatal(err)
	}
	// The two beams rows, one body each: Render's subject is the only thing
	// that tells which of them applies.
	oldTemplates, oldRules := shippedTemplates, shippedRules
	t.Cleanup(func() { shippedTemplates, shippedRules = oldTemplates, oldRules })
	set := rules.Set{Terms: oldRules.Terms, Rules: map[string]rules.Rule{}}
	for id, r := range oldRules.Rules {
		set.Rules[id] = r
	}
	beams := set.Rules["solar.beams"]
	beams.Rows = append([]rules.Row(nil), beams.Rows...)
	seen := 0
	for i := range beams.Rows {
		switch beams.Rows[i].ID {
		case "solar.beams/17":
			beams.Rows[i].Bodies, seen = []string{"ceres"}, seen+1
		case "solar.beams/12":
			beams.Rows[i].Bodies, seen = []string{"venus"}, seen+1
		}
	}
	if seen != 2 {
		t.Fatalf("fixture: %d of the two beams rows found", seen)
	}
	set.Rules["solar.beams"] = beams
	shippedRules = set
	shippedTemplates = append(append([]templates.Template(nil), oldTemplates...), ts...)

	m := templateTestModel(t)
	ceres := fixtureObject(t, 12)
	m.chart.positions = append(m.chart.positions, aspect.Position{Object: ceres})
	m.bodyCursorIdx = 3
	if tabs := m.panelTabs(PanelDetails); len(tabs) != 2 || tabs[1].ID != "tpl:ceres-fixture" {
		t.Fatalf("on Ceres: %+v", tabs)
	}
	m.bodyCursorIdx = 2
	if tabs := m.panelTabs(PanelDetails); len(tabs) != 2 || tabs[1].ID != "tpl:body-inferior-planet" {
		t.Fatalf("on Venus: %+v", tabs)
	}

	m.bodyCursorIdx = 3
	m = runTemplate(t, activeOnTemplate(m))
	key, ok := m.templateKeyFor()
	if !ok || key.template != "ceres-fixture" || key.object != 12 || key.listsBy != "ceres" {
		t.Errorf("key %+v (%v), want the ceres-fixture template, object 12, subject ceres", key, ok)
	}
	if len(*calls) == 0 {
		t.Fatal("no facts were asked for")
	}
	for _, c := range *calls {
		if c.req.Object != ceres {
			t.Errorf("facts asked for %+v, want Ceres %+v", c.req.Object, ceres)
		}
	}
	var all []string
	for _, l := range m.logLines {
		all = append(all, l.text)
	}
	log := strings.Join(all, "\n")
	if !strings.Contains(log, "ceres-fixture verdicts from rule rows solar.beams/17") || strings.Contains(log, "solar.beams/12") {
		t.Errorf("the rule rows are not the ones written for ceres:\n%s", log)
	}
	if len(m.tpl.lines) == 0 || m.tpl.lines[0].Label != "elongation" {
		t.Errorf("lines %+v", m.tpl.lines)
	}
}
