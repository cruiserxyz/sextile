package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// the specification: `/` on a Details report that is one list of
// Astrolog rows filters the records and leaves the header and footer as
// printed.

var filterAspectNames = []string{"Con", "Squ", "Tri", "Sex", "Opp"}

// ownFilterBody is a canned own-chart `-aa` report of n rows: 2 header lines,
// the rows (every second one a Squ), then the 5 footer lines.
func ownFilterBody(n int) string {
	var b strings.Builder
	b.WriteString("Astrolog 8.00 chart for Tue Aug 25, 2026 14:00 (DT Zone 1E)  13°25E 52°31N\n\n")
	for i := 1; i <= n; i++ {
		name := "Con"
		if i%2 == 0 {
			name = "Squ"
		} else if i%3 == 0 {
			name = "Tri"
		}
		fmt.Fprintf(&b, "%3d:    Moon (Aqu) %s [Tau] Chiron     - orb: s0°50' - power: 11.09\n", i, name)
	}
	b.WriteString("\nSum power: 41.16 - Average power: 4.57\nCon:  3   Opp:  0   Squ:  2   Tri:  3   Sex:  1\nSun:  1   Moo: 10\nNep:  1   Plu:  1\n")
	return b.String()
}

// crossFilterBody is a canned comparison report: 3 header lines.
func crossFilterBody(n int) string {
	var b strings.Builder
	b.WriteString("Astrolog 8.00 chart for Tue May 15, 1990 14:00 (DT Zone 1E)  13°25E 52°31N\nChart #2 comparison for Tue Aug 25, 2026 14:00 (DT Zone 1E)  13°25E 52°31N\n\n")
	for i := 1; i <= n; i++ {
		name := "Con"
		if i%2 == 0 {
			name = "Squ"
		}
		fmt.Fprintf(&b, "%3d:    Moon (Cap) %s with [Tau] Chiron    - orb: a3°29' - power:  6.98\n", i, name)
	}
	b.WriteString("\nSum power: 19.91 - Average power: 9.96\nCon:  1   Opp:  0\n")
	return b.String()
}

// attachedReport is the Aspects-focused fixture with Details attached on the
// report tab showing title/body.
func attachedReport(t *testing.T, onPair bool, title, body string) Model {
	t.Helper()
	m := tabsPairModel(t, Config{Accessible: true})
	m.width, m.height = 144, 40
	if onPair {
		m, _ = tpress(t, m, runes("]"))
		if !isAspectsPairTab(m.activeTabID(PanelAspects)) {
			t.Fatalf("fixture: active Aspects tab %q, want a pair tab", m.activeTabID(PanelAspects))
		}
	}
	m.details.title, m.details.body, m.details.attached = title, body, true
	m.details.pending = false // the pair tab's recompute is not under test
	if m.activeTabID(PanelDetails) != TabDetailsReport {
		t.Fatalf("fixture: Details tab %q, want the report", m.activeTabID(PanelDetails))
	}
	return m
}

func hasSearchBinding(m Model) bool {
	for _, e := range m.available(m.keys.detailsBindings()) {
		if e.ID == BindingSearch {
			return true
		}
	}
	return false
}

func viewText(m Model) string { return strings.Join(m.detailsView().lines, "\n") }

// Acceptance 2: `/squ` keeps the 2 header lines, every Squ row and the 5
// footer lines, and the query line says N of M with M the row count.
func TestSlashKeepsHeaderAndFooterAroundTheMatchingRows(t *testing.T) {
	body := ownFilterBody(8)
	m := attachedReport(t, false, "[0] Aspects", body)
	if !hasSearchBinding(m) {
		t.Fatal("the key bar does not offer / on a report with a list")
	}
	m, _ = tpress(t, m, runes("/"))
	m = typeQuery(t, m, "squ") // exact, so Squ and nothing fuzzier
	src := detailsReportLines(body)
	var want []string
	want = append(want, src[:2]...)
	for _, l := range src[2:10] {
		if strings.Contains(l, "Squ") {
			want = append(want, l)
		}
	}
	want = append(want, src[10:]...)
	if got := m.detailsView().lines; strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("lines =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(want) != 2+4+5 {
		t.Fatalf("fixture: %d lines kept, want 2 header + 4 Squ rows + 5 footer", len(want))
	}
	rows := detailsBodyRows(renderDetails(m))
	if last := rows[len(rows)-1]; !strings.HasPrefix(last, "/squ█") || !strings.HasSuffix(last, "4 of 8") {
		t.Errorf("open query line = %q, want `/squ█` ... `4 of 8`", last)
	}
	m, _ = tpress(t, m, keyEnter)
	rows = detailsBodyRows(renderDetails(m))
	if last := rows[len(rows)-1]; last != "/squ  4 of 8" {
		t.Errorf("kept query line = %q, want `/squ  4 of 8`", last)
	}
	if m.details.attached != true {
		t.Error("Enter in the line detached Details")
	}
}

// Acceptance 3: a comparison list (3 header lines) behaves the same, and a
// single-chart body report under Bodies reads the own-chart rows.
func TestSlashFiltersComparisonAndBodyReports(t *testing.T) {
	body := crossFilterBody(6)
	m := attachedReport(t, true, "[0] Aspects ②", body)
	m, _ = tpress(t, m, runes("/"))
	m = typeQuery(t, m, "squ")
	src := detailsReportLines(body)
	got := m.detailsView().lines
	if strings.Join(got[:3], "\n") != strings.Join(src[:3], "\n") || strings.Join(got[len(got)-3:], "\n") != strings.Join(src[len(src)-3:], "\n") {
		t.Errorf("comparison header/footer moved:\n%s", strings.Join(got, "\n"))
	}
	if len(got) != 3+3+3 {
		t.Errorf("comparison: %d lines kept, want 3 header + 3 Squ rows + 3 footer:\n%s", len(got), strings.Join(got, "\n"))
	}

	b := tabsPairModel(t, Config{Accessible: true})
	b.width, b.height = 144, 40
	b.focused = PanelBodies
	b.slots.SetComparisonSelected(false)
	b.details.title, b.details.body, b.details.attached = "[0] Moon", ownFilterBody(4), true
	b, _ = tpress(t, b, runes("/"))
	b = typeQuery(t, b, "/tri")
	if got := b.detailsView().lines; len(got) != 2+1+5 {
		t.Errorf("single body report: %d lines kept, want 2 + 1 Tri row + 5:\n%s", len(got), strings.Join(got, "\n"))
	}
}

// Acceptance 4: a record is kept whole when any of its lines matches.
func TestMultiLineRecordIsKeptWholeWhenAnyLineMatches(t *testing.T) {
	body := "Astrolog 8.00 chart for X\n\n" +
		"  1:    Moon (Aqu) Con [Tau] Chiron     - orb: s0°50' - power: 11.09\n" +
		"        Midpoint Moon makes Con with Chiron\n" +
		"        second note line\n" +
		"  2:    Moon (Aqu) Squ [Tau] Chiron     - orb: s0°50' - power: 11.09\n" +
		"\nSum power: 22.18\n"
	m := attachedReport(t, false, "[0] Aspects", body)
	m, _ = tpress(t, m, runes("/"))
	m = typeQuery(t, m, "/makes")
	got := m.detailsView().lines
	want := []string{"Astrolog 8.00 chart for X", "",
		"  1:    Moon (Aqu) Con [Tau] Chiron     - orb: s0°50' - power: 11.09",
		"        Midpoint Moon makes Con with Chiron", "        second note line",
		"", "Sum power: 22.18"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("lines =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	m = typeQuery(t, m, "") // line still open
	m, _ = tpress(t, m, keyEnter)
	if rows := detailsBodyRows(renderDetails(m)); !strings.HasSuffix(rows[len(rows)-1], "1 of 2") {
		t.Errorf("count = %q, want `1 of 2` (records, not lines)", rows[len(rows)-1])
	}
}

// Acceptance 5: no list means no / on the bar and the key does nothing.
func TestNoListOffersNoSlash(t *testing.T) {
	row := "  1:    Moon (Aqu) Con [Tau] Chiron     - orb: s0°50' - power: 11.09\n"
	row2 := "  2:    Moon (Aqu) Squ [Tau] Chiron     - orb: s0°50' - power: 11.09\n"
	for name, body := range map[string]string{
		"foreign line between": "head\n\n" + row + "Interlude\n" + row2,
		"no record":            "Astrolog 8.00 chart for X\n\nPlanet: Sun\n",
		"empty":                "",
	} {
		m := attachedReport(t, false, "[0] Aspects", body)
		if hasSearchBinding(m) {
			t.Errorf("%s: the key bar offers /", name)
		}
		m2, cmd := tpress(t, m, runes("/"))
		if cmd != nil || m2.details.searching || m2.detailsReportSearchLine() {
			t.Errorf("%s: / opened a line (searching=%v)", name, m2.details.searching)
		}
	}
}

// Acceptance 6: the Chart report offers no /, even when its text happens to
// hold numbered rows, and so does a template tab.
func TestChartReportOffersNoSlash(t *testing.T) {
	m := attachedReport(t, false, "[0] Aspects", ownFilterBody(4))
	m.focused = PanelChart
	m.details.title = "[0] Chart"
	if m.activeTabID(PanelDetails) != TabDetailsReport {
		t.Fatalf("fixture: tab %q", m.activeTabID(PanelDetails))
	}
	if hasSearchBinding(m) {
		t.Error("the Chart report offers /")
	}
	m, _ = tpress(t, m, runes("/"))
	if m.details.searching {
		t.Error("/ opened a line on the Chart report")
	}
	m.details.tabs = map[PanelID]TabID{PanelChart: TabDetailsTimeline}
	if g := m.detailsGrammar(); g != 0 {
		t.Errorf("a non-report tab declares grammar %v", g)
	}
}

// Acceptance 7: the query survives the same report refreshing (a new frame for
// the same title) and is cleared when the title changes; it is Details' own.
func TestQueryLivesAndDiesWithTheReportTitle(t *testing.T) {
	m := tabsPairModel(t, Config{Accessible: true})
	m.width, m.height = 144, 40
	var cmd tea.Cmd
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{})
	frame := frameWithReports()
	frame.AspectsReport = ownFilterBody(6)
	frame.CrossAspectsReport = crossFilterBody(6)
	st := m.refresh[surfaceText]
	frame.RunID, frame.WantID = st.runningRunID, st.wantID
	m, cmd = applyTextCommit(m, frame)
	m = pump(t, m, cmd)
	if m.details.title != "[0] Aspects" {
		t.Fatalf("fixture: title %q", m.details.title)
	}
	m.details.attached = true
	m, _ = tpress(t, m, runes("/"))
	m = typeQuery(t, m, "squ")
	m, _ = tpress(t, m, keyEnter)
	m, _ = tpress(t, m, keyEsc) // detach, query stays
	if m.details.query != "squ" {
		t.Fatalf("query after detaching = %q", m.details.query)
	}
	// The same report refreshed with a new text keeps the query.
	frame2 := frame
	frame2.AspectsReport = ownFilterBody(8)
	m, cmd = applyTextCommit(m, frame2)
	m = pump(t, m, cmd)
	if m.details.query != "squ" || len(m.detailsView().lines) != 2+4+5 {
		t.Errorf("after a refresh of the same title: query %q, %d lines, want it kept and 4 Squ rows", m.details.query, len(m.detailsView().lines))
	}
	if m.timeline.query != "" {
		t.Errorf("Details' query leaked into the timeline: %q", m.timeline.query)
	}
	// A different title is a new subject.
	m, cmd = tpress(t, m, runes("]"))
	m = pump(t, m, cmd)
	if m.details.title == "[0] Aspects" {
		t.Fatalf("fixture: the title did not change (%q)", m.details.title)
	}
	if m.details.query != "" || m.details.searching {
		t.Errorf("a new title kept query %q searching=%v", m.details.query, m.details.searching)
	}
}

// Acceptance 8: while the report tab waits, / and the open line's keys do
// nothing.
func TestWaitingReportIgnoresTheQueryKeys(t *testing.T) {
	m := attachedReport(t, false, "[0] Aspects", ownFilterBody(6))
	m.details.pending = true
	if !m.detailsTabWaiting(TabDetailsReport) {
		t.Fatal("fixture: the report is not waiting")
	}
	m, _ = tpress(t, m, runes("/"))
	if m.details.searching {
		t.Error("/ opened the line on a waiting report")
	}
	// A line that was open when the wait began swallows the keys.
	m.details.pending = false
	m, _ = tpress(t, m, runes("/"))
	m = typeQuery(t, m, "sq")
	m.details.pending = true
	m, _ = tpress(t, m, runes("u"))
	m, _ = tpress(t, m, keyEnter)
	if m.details.query != "sq" || !m.details.searching {
		t.Errorf("a waiting report took query keys: query %q searching=%v", m.details.query, m.details.searching)
	}
	m.details.pending = false
	m = typeQuery(t, m, "u")
	if m.details.query != "squ" {
		t.Errorf("settled, the line did not type again: %q", m.details.query)
	}
}

// Acceptance 10: an empty query draws the report exactly as printed, and a
// filtered report re-clamps the scroll to its shorter text.
func TestEmptyQueryIsByteIdenticalAndScrollReclamps(t *testing.T) {
	body := ownFilterBody(40)
	m := attachedReport(t, false, "[0] Aspects", body)
	opened, _ := tpress(t, m, runes("/")) // open, nothing typed
	plain := m
	plain.details.attached = false
	// With no line and no query the frame is the printed report.
	fr := m.detailsFrame()
	fr2 := fr
	fr2.report = nil
	r := m.geometry()[PanelDetails]
	if renderDetailsPanel(r.w, r.h, true, m.cfg, fr) != renderDetailsPanel(r.w, r.h, true, m.cfg, fr2) {
		t.Error("an empty query changed the report's drawing")
	}
	if got := opened.detailsView().lines; strings.Join(got, "\n") != strings.Join(detailsReportLines(body), "\n") {
		t.Error("an open line with an empty query changed the lines")
	}
	// Scroll to the end, then filter down to a handful of rows.
	for i := 0; i < 20; i++ {
		m, _ = tpress(t, m, keyPgDn)
	}
	if m.details.scrollOffset == 0 {
		t.Fatal("fixture: the report did not scroll")
	}
	m, _ = tpress(t, m, runes("/"))
	m = typeQuery(t, m, "/tri")
	lines := len(m.detailsView().lines)
	if limit := max(0, lines-m.detailsReportHeight()); m.details.scrollOffset > limit {
		t.Errorf("offset %d beyond what %d filtered lines can scroll (%d)", m.details.scrollOffset, lines, limit)
	}
	box := strings.Join(detailsBodyRows(renderDetails(m)), "\n")
	if !strings.Contains(box, "Sum power") || !strings.Contains(box, "Astrolog 8.00") && m.details.scrollOffset == 0 {
		t.Errorf("filtered view lost header or footer:\n%s", box)
	}
}
