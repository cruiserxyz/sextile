package ui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/internal/ui/term"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// specs/details-follows-its-tab.md, slice D1: every pane shows only its own
// subject's content. The fixture is follow_fixture_test.go's.

// The defect that started it (strategy page, "The problem, measured"): `2` from
// the Chart with the Chart report committed switched the caption to the Sun
// while the body below stayed the Chart's influence table until the Sun's
// report landed.
func TestChartReportNeverUnderTheSunsCaption(t *testing.T) {
	m, _ := followFixture(t, Config{Accessible: true}, 132, 40)
	if got := detailsPanelText(m); !strings.Contains(got, influenceHead) {
		t.Fatalf("fixture: the Chart report is not drawn:\n%s", got)
	}
	m, _ = pressOnly(t, m, runes("3")) // the Sun's report is not back
	if got := stripOfDetails(m); got != "Sun" {
		t.Fatalf("caption after 2 = %q, want Sun", got)
	}
	if got := detailsPanelText(m); strings.Contains(got, influenceHead) {
		t.Fatalf("the Sun's caption stands over the Chart's influence table:\n%s", got)
	}
}

// C1. A committed result is stored with its key: the report body, the Aspects
// copy, the Timeline and the template; Bodies and Aspects keep the key of the
// frame they draw.
func TestContentCarriesItsKey(t *testing.T) {
	t.Run("report", func(t *testing.T) {
		m, _ := followFixture(t, Config{Accessible: true}, 132, 40)
		want, ok := m.detailsKeyFor()
		if !ok || !m.details.keyed || m.details.key != (contentKey{kind: reportRaw, raw: want}) {
			t.Fatalf("the Chart report carries key %+v (keyed %v), want the key it was asked for %+v", m.details.key, m.details.keyed, want)
		}
		m, cmd := pressOnly(t, m, runes("3"))
		m = pump(t, m, cmd)
		want, _ = m.detailsKeyFor()
		if m.details.key.raw != want || want.object != objSun.Number {
			t.Errorf("the Sun's report carries key %+v, want %+v", m.details.key.raw, want)
		}
	})

	t.Run("bodies and aspects", func(t *testing.T) {
		m, _ := followFixture(t, Config{Accessible: true}, 132, 40)
		for _, p := range []PanelID{PanelBodies, PanelAspects} {
			shown, ok := m.shownPaneKey(p)
			want, _ := m.wantedPaneKey(p)
			if !ok || shown != want {
				t.Errorf("%v draws pane key %+v (ok %v), want the frame's %+v", p, shown, ok, want)
			}
		}
		b, _ := m.shownPaneKey(PanelBodies)
		a, _ := m.shownPaneKey(PanelAspects)
		if b == a || b.text != a.text {
			t.Errorf("the two panes' keys: %+v and %+v, want one text frame under two pane ids", b, a)
		}
	})

	t.Run("aspects copy", func(t *testing.T) {
		m, _ := followFixture(t, Config{Accessible: true}, 132, 40)
		m, cmd := pressOnly(t, m, runes("4"))
		m = pump(t, m, cmd)
		shown, _ := m.shownPaneKey(PanelAspects)
		if m.details.key.kind != reportFrame || m.details.key.frame != shown {
			t.Fatalf("the Aspects copy carries key %+v, want the frame it was copied from %+v", m.details.key, shown)
		}
		if !strings.Contains(m.details.body, "SUBJECT aspects floating") {
			t.Errorf("the Aspects copy is %q", m.details.body)
		}
	})

	t.Run("timeline", func(t *testing.T) {
		freshModels(t)
		stubTimeline(t, nil)
		m := timelineModel(t, Config{Accessible: true}).withDetailsTab(PanelChart, TabDetailsTimeline)
		m, cmd := m.syncTimeline()
		m = pump(t, m, cmd)
		want, ok := m.timelineKeyFor()
		if !ok || !m.timeline.keyed || m.timeline.key != want || !m.timeline.complete {
			t.Fatalf("the timeline carries key %+v (keyed %v complete %v), want %+v", m.timeline.key, m.timeline.keyed, m.timeline.complete, want)
		}
	})

	t.Run("template", func(t *testing.T) {
		freshModels(t)
		stubBodyFacts(t, nil)
		m := activeOnTemplate(templateTestModel(t))
		m = runTemplate(t, m)
		want, ok := m.templateKeyFor()
		if !ok || !m.tpl.keyed || m.tpl.key != want || len(m.tpl.lines) == 0 {
			t.Fatalf("the template carries key %+v (keyed %v, %d lines), want %+v", m.tpl.key, m.tpl.keyed, len(m.tpl.lines), want)
		}
	})
}

// C2. The caption comes from the wanted key in the frame of the key, whatever
// the report on screen is.
func TestCaptionIsTheWantedSubject(t *testing.T) {
	m, _ := followFixture(t, Config{Accessible: true}, 132, 40)
	caption := func(m Model) string { return stripOfDetails(m) }
	if got := caption(m); got != "Now" {
		t.Errorf("Chart on the floating chart: %q, want Now", got)
	}
	tabs := m.panelTabs(PanelDetails)
	if len(tabs) != 3 || tabs[1].Title != tabTitleTimeline || tabs[2].Title != tabTitleMoon {
		t.Errorf("the Timeline keeps its own tab beside the subject: %+v", tabs)
	}
	up, _ := pressOnly(t, m, keyUp) // the new chart's frame and report are held
	if got := caption(up); got != "Synthetic 1" {
		t.Errorf("Chart on an anchored chart: %q, want its name", got)
	}
	bodies, _ := pressOnly(t, m, runes("3"))
	if got := caption(bodies); got != "Sun" {
		t.Errorf("Bodies on the Sun: %q, want Sun", got)
	}
	down, _ := pressOnly(t, bodies, keyDown)
	if got := caption(down); got != "Moon" {
		t.Errorf("Bodies a row down: %q, want Moon", got)
	}
	asp, _ := pressOnly(t, m, runes("4"))
	if got := caption(asp); got != "Aspects" {
		t.Errorf("Aspects: %q, want Aspects", got)
	}
	// Bodies' title and the Aspects strip come from the live scene, not from the
	// frame on screen.
	if got := up.panelTabs(PanelBodies)[0].Title; got != tabTitleBodies {
		t.Errorf("Bodies title = %q", got)
	}
	pair := tabsOnPair(t, Config{Accessible: true})
	if got := pair.panelTabs(PanelBodies)[0].Title; got == tabTitleBodies {
		t.Errorf("Bodies title %q on a pair tab: it follows the live scene", got)
	}
	if got := pair.tabStrips()[PanelAspects].activeID(); got != TabAspectsCross {
		t.Errorf("Aspects strip's active tab %q before any frame lands, want the live scene's %q", got, TabAspectsCross)
	}
	pair.focused = PanelBodies
	pair.chart.positions = []aspect.Position{{Object: objVenus}}
	if got := pair.detailsWanted().caption; !strings.HasSuffix(got, " in 1") {
		t.Errorf("Bodies on a pair tab: caption %q, want the body and the pair", got)
	}
}

// C3-C5, and C7's half that is the rule itself: the pane draws by the keys and
// by nothing else.
func TestViewRule(t *testing.T) {
	t.Run("current", func(t *testing.T) {
		forceColor(t)
		m, _ := followFixture(t, Config{Color: ColorAlways}, 132, 40)
		if m.detailsReportState() != paneCurrent || m.textPaneState(PanelBodies) != paneCurrent || m.textPaneState(PanelAspects) != paneCurrent {
			t.Fatalf("fixture states: Details %v Bodies %v Aspects %v", m.detailsReportState(), m.textPaneState(PanelBodies), m.textPaneState(PanelAspects))
		}
		if isDimmed(m) || strings.HasSuffix(stripTitle(m), detailsPendingMark) {
			t.Errorf("a current report is faint or marked: %q", stripTitle(m))
		}
		if view := m.View(); !strings.Contains(ansi.Strip(view), influenceHead) || strings.Contains(view, term.SGRFaint+"☉") {
			t.Errorf("a current pane does not draw its content bright:\n%s", ansi.Strip(view))
		}
	})

	t.Run("same_subject", func(t *testing.T) {
		for _, cfg := range []Config{{Color: ColorAlways}, {Accessible: true}} {
			name := fmt.Sprintf("%v", cfg.UseColor())
			t.Run("details/colour="+name, func(t *testing.T) {
				if cfg.UseColor() {
					forceColor(t)
				}
				m, _ := followFixture(t, cfg, 132, 40)
				m, cmd := pressOnly(t, m, runes("3"))
				m = pump(t, m, cmd) // the Sun's report is committed
				// `d` on Bodies: the Sun of the floating chart, one day on. Its
				// frame is committed, its report is held.
				var seen bool
				m, cmd = pressOnly(t, m, runes("d"))
				pumpSeen(t, m, 400_000_000, func(m Model) {
					if seen || m.textPaneState(PanelBodies) != paneCurrent || m.detailsReportState() != paneUpdating {
						return
					}
					seen = true
					if got := stripTitle(m); !strings.HasSuffix(got, detailsPendingMark) || !strings.HasPrefix(got, "Sun") {
						t.Errorf("the caption of an updating report is %q, want Sun …", got)
					}
					text := detailsPanelText(m)
					if !strings.Contains(text, "SUBJECT body Sun floating") {
						t.Errorf("the same subject's report is not drawn:\n%s", text)
					}
					if cfg.UseColor() && !isDimmed(m) {
						t.Errorf("the previous report is not faint")
					}
					if !cfg.UseColor() && isDimmed(m) {
						t.Errorf("colours off drew a faint attribute")
					}
				}, cmd)
				if !seen {
					t.Fatal("no step drew the report as the same subject at another moment")
				}
			})
			t.Run("panes/colour="+name, func(t *testing.T) {
				if cfg.UseColor() {
					forceColor(t)
				}
				// Bodies and Aspects at the same kind of step: `d` registers the
				// new moment; until the frame lands the old rows stay as they are,
				// never faint, and the frame replaces them in one go (operator,
				// 2026-11-06: "lets excempt the main panels from content to be dimmed
				// while content is generated ... flicker free has prority").
				m, _ := followFixture(t, cfg, 132, 40)
				before := map[PanelID]string{PanelBodies: rawPanel(m, PanelBodies), PanelAspects: rawPanel(m, PanelAspects)}
				m, _ = pressOnly(t, m, runes("d"))
				for _, p := range []PanelID{PanelBodies, PanelAspects} {
					if got := m.textPaneState(p); got != paneCurrent {
						t.Errorf("%v after d, before its frame: %v, want the rows as they are", p, got)
					}
					if got := rawPanel(m, p); got != before[p] {
						t.Errorf("%v changed before its frame landed:\nbefore %q\nafter  %q", p, before[p], got)
					}
				}
			})
		}
	})

	t.Run("other_subject", func(t *testing.T) {
		forceColor(t)
		m, _ := followFixture(t, Config{Color: ColorAlways}, 132, 40)
		// Details: the Chart's report under a new panel's caption.
		on2, _ := pressOnly(t, m, runes("3"))
		text := ansi.Strip(detailsPanelText(on2))
		// Amended 2026-11-06 (no `…` on a blank Details): the pane is blank.
		if !detailsBodyBlank(text) || strings.Contains(text, influenceHead) || strings.Contains(text, "SUBJECT") {
			t.Errorf("another subject's Details is not blank:\n%s", text)
		}
		if strings.HasSuffix(stripTitle(on2), detailsPendingMark) {
			t.Errorf("a new subject's caption ends in ` …`: %q", stripTitle(on2))
		}
		// Bodies and Aspects: the cursor on another chart.
		up, _ := pressOnly(t, m, keyUp)
		if up.textPaneState(PanelBodies) != paneEmpty || up.textPaneState(PanelAspects) != paneEmpty {
			t.Fatalf("after ↑ to another chart: Bodies %v Aspects %v, want empty", up.textPaneState(PanelBodies), up.textPaneState(PanelAspects))
		}
		view := ansi.Strip(up.View())
		for _, content := range []string{"Tau", "Mercury", "(no aspects within orb)"} {
			if strings.Contains(view, content) {
				t.Errorf("Bodies or Aspects still draw the other chart's %q:\n%s", content, view)
			}
		}
		if strings.Count(view, emptyPaneLine) < 2 {
			t.Errorf("Bodies and Aspects do not each read `…` (Details is blank since 2026-11-06):\n%s", view)
		}
		// Nothing is shown at all: also an empty pane.
		blank := m
		blank.details = detailsViewState{tabs: m.details.tabs}
		if blank.detailsReportState() != paneEmpty {
			t.Errorf("nothing shown: %v, want empty", blank.detailsReportState())
		}
	})
}

// rawPanel is panel p's inner rows on the composed screen, escapes included.
func rawPanel(m Model, p PanelID) string {
	r, ok := m.geometry()[p]
	if !ok {
		return ""
	}
	rows := strings.Split(m.View(), "\n")
	var out []string
	for y := r.y + 1; y < r.y+r.h-1 && y < len(rows); y++ {
		out = append(out, ansi.Cut(rows[y], r.x+1, r.x+r.w-1))
	}
	return strings.Join(out, "\n")
}

// bodiesPanelText is the rendered Bodies panel's rows, escapes included.
func bodiesPanelText(m Model) string {
	lines := strings.Split(m.View(), "\n")
	var out []string
	for _, l := range lines {
		if strings.Contains(ansi.Strip(l), "☉ Sun") || strings.Contains(ansi.Strip(l), "☽ Moon") {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// C6. A failed Details report reads one faint line naming its subject, and no
// older content; the Log keeps its error line; the line is the wanted key's,
// and goes when the wanted key changes.
func TestFailedReportLine(t *testing.T) {
	forceColor(t)
	m, stub := followFixture(t, Config{Color: ColorAlways}, 132, 40)
	stub.failBodies = true
	m, cmd := pressOnly(t, m, runes("3"))
	m = pump(t, m, cmd)
	if m.detailsReportState() != paneFailed {
		t.Fatalf("after the Sun's report failed: %v, want failed", m.detailsReportState())
	}
	text := detailsPanelText(m)
	plain := ansi.Strip(text)
	if !strings.Contains(plain, "Couldn't read the Sun's report.") || !strings.Contains(text, term.SGRFaint+"Couldn't read the Sun's report.") {
		t.Errorf("the failure line is missing or not faint:\n%s", plain)
	}
	if strings.Contains(plain, influenceHead) || strings.Contains(plain, "SUBJECT") {
		t.Errorf("older content under the failure line:\n%s", plain)
	}
	logged := false
	for _, l := range m.logLines {
		logged = logged || strings.HasPrefix(l.text, "details: report not read")
	}
	if !logged {
		t.Errorf("the Log lost its error line: %v", m.logLines)
	}
	if m.detailsBodyShown() != "" {
		t.Errorf("an action could still read the old report: %q", m.detailsBodyShown())
	}

	// The wanted key moves to the Moon: the Sun's failure is not the Moon's.
	m, cmd = pressOnly(t, m, keyDown)
	if m.detailsReportState() == paneFailed {
		t.Errorf("the Sun's failure stands under the Moon's caption")
	}
	m = pump(t, m, cmd)
	if got := ansi.Strip(detailsPanelText(m)); !strings.Contains(got, "Couldn't read the Moon's report.") {
		t.Errorf("the Moon's own failure is not drawn:\n%s", got)
	}
	// A recovered Astrolog: the Sun again, and the line is gone.
	stub.failBodies = false
	m, cmd = pressOnly(t, m, keyUp)
	m = pump(t, m, cmd)
	if m.detailsReportState() != paneCurrent || !strings.Contains(detailsPanelText(m), "SUBJECT body Sun floating") {
		t.Errorf("after recovery: %v\n%s", m.detailsReportState(), ansi.Strip(detailsPanelText(m)))
	}
}

// C18. A Bodies cursor on a row that is no object owes no report: the panel's
// own title for caption, one `…`, no wanted key, nothing scheduled.
func TestCuspRowHasNoReport(t *testing.T) {
	m, stub := followFixture(t, Config{Accessible: true}, 132, 40)
	m, cmd := pressOnly(t, m, runes("3"))
	m = pump(t, m, cmd)
	spawned := stub.reports
	m.bodyCursorIdx = 99 // past the rows: ObjectAtCursor gives no object
	if _, ok := panel.ObjectAtCursor(m.chart.positions, m.chart.cusps, m.bodyCursorIdx); ok {
		t.Fatal("fixture: the cursor row is an object")
	}
	if w := m.detailsWanted(); w.has || w.caption != tabTitleBodies {
		t.Errorf("wanted = %+v, want the panel's title and no key", w)
	}
	if _, ok := m.detailsKeyFor(); ok {
		t.Error("a report key exists for a row that is no object")
	}
	if got := detailsPanelText(m); !detailsBodyBlank(got) || strings.Contains(got, "SUBJECT") {
		t.Errorf("the pane under a non-object row is not blank:\n%s", got)
	}
	m, cmd = m.requestDetails()
	m = pump(t, m, cmd)
	m, cmd = m.deferDetails()
	m = pump(t, m, cmd)
	if stub.reports != spawned {
		t.Errorf("%d report spawns for a row that is no object", stub.reports-spawned)
	}
	if m.refresh[surfaceDetails].cancel != nil {
		t.Error("a report run is in flight for a row that is no object")
	}
}

// subjectMarker is the marker a report or the Aspects copy must carry for the
// subject the Model's caption names, and the cell the Bodies Sun row must show.
func expectedSubject(m Model) (marker, sunSign string) {
	who, sunSign := "floating", "Tau"
	if !m.focusedSlot().Floating {
		who, sunSign = "natal", "Can"
	}
	switch m.focused {
	case PanelBodies:
		ch := m.shownChart()
		if o, ok := panel.ObjectAtCursor(ch.positions, ch.cusps, m.bodyCursorIdx); ok {
			return "SUBJECT body " + panel.BodyName(o) + " " + who, sunSign
		}
		return "", sunSign
	case PanelAspects:
		return "SUBJECT aspects " + who, sunSign
	}
	return "SUBJECT chart - " + who, sunSign
}

var subjectLine = regexp.MustCompile(`SUBJECT [a-z]+ \S+(?: \S+)?`)

// assertOneSubject holds one frame to C7: whatever of Details, Bodies and
// Aspects is drawn is drawn for the subject the captions name.
func assertOneSubject(t *testing.T, m Model, where string) {
	t.Helper()
	marker, sunSign := expectedSubject(m)
	plain := ansi.Strip(m.View())
	// Details: any marker drawn is the caption's. The match is the whole
	// marker line, because the Chart report's marker has the placeholder `-`.
	lines := strings.Split(plain, "\n")
	for _, l := range lines {
		if i := strings.Index(l, "SUBJECT "); i >= 0 {
			got := strings.TrimRight(strings.TrimRight(l[i:], "│ |"), " ")
			if got != marker {
				t.Fatalf("%s: Details draws %q under the caption for %q\n%s", where, got, marker, plain)
			}
		}
	}
	if m.focused == PanelChart && strings.Contains(plain, influenceHead) && stripOfDetails(m) != chartCaption(m.focusedSlot()) {
		t.Fatalf("%s: the influence table under the caption %q", where, stripOfDetails(m))
	}
	// Bodies: the Sun row belongs to the chart under the cursor.
	for _, l := range lines {
		if !strings.Contains(l, "☉ Sun") {
			continue
		}
		other := "Can"
		if sunSign == "Can" {
			other = "Tau"
		}
		if strings.Contains(l, other+" ") && !strings.Contains(l, sunSign+" ") {
			t.Fatalf("%s: Bodies draws the other chart's Sun under this cursor:\n%s", where, plain)
		}
	}
	// A pane that draws nothing of a subject says so with one `…`.
	if m.textPaneState(PanelBodies) == paneEmpty && strings.Contains(plain, "☉ Sun") {
		t.Fatalf("%s: Bodies is empty by the rule and draws rows:\n%s", where, plain)
	}
}

// C7, C10. No entry point, message order or timer draws a caption and a content
// of different subjects. Two corpora: a fixed one of key sequences, each
// delivered three ways (every result held, every result delivered at once,
// results arriving one key late), with every frame held to one subject; and the
// seeded corpus of C10 (every_frame_test.go).
func TestEveryFrameShowsOneSubject(t *testing.T) {
	keyFor := func(s string) tea.KeyMsg {
		switch s {
		case "up":
			return keyUp
		case "down":
			return keyDown
		case "tab":
			return keyTab
		case "right":
			return keyRight
		case "left":
			return keyLeft
		}
		return runes(s)
	}
	corpus := [][]string{
		{"3", "down", "d", "2", "up", "3"},
		{"up", "3", "4", "2", "down", "3"},
		{"d", "3", "d", "up", "4", "2"},
		{"3", "up", "tab", "down", "2", "D"},
	}
	// The seeded corpus over the virtual scheduler (every_frame_test.go).
	t.Run("seeded", seededEveryFrame)
	for _, mode := range []string{"held", "settled", "late"} {
		for i, seq := range corpus {
			t.Run(fmt.Sprintf("%s/%d", mode, i), func(t *testing.T) {
				m, _ := followFixture(t, Config{Accessible: true}, 132, 40)
				assertOneSubject(t, m, "fixture")
				var late []tea.Cmd
				for n, k := range seq {
					where := fmt.Sprintf("%s %v, key %d %q", mode, seq, n+1, k)
					var cmd tea.Cmd
					m, cmd = pressOnly(t, m, keyFor(k))
					assertOneSubject(t, m, where)
					see := func(m Model) { assertOneSubject(t, m, where+" (a result)") }
					switch mode {
					case "settled":
						m = pumpSeen(t, m, 400_000_000, see, cmd)
					case "late":
						// Last key's results arrive now, after this key's frame.
						m = pumpSeen(t, m, 400_000_000, see, late...)
						late = []tea.Cmd{cmd}
					}
				}
				if mode == "late" {
					m = pumpSeen(t, m, 400_000_000, func(m Model) { assertOneSubject(t, m, "late, last results") }, late...)
				}
			})
		}
	}
}
