package ui

// the specification: a library chart's name and place text reach
// Astrolog's own header in every report Details shows, and nowhere else.
// Every chart here is a constructed synthetic.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Two clients born at the same place with the same moment but different names
// would be the cache-aliasing case; these two differ in moment and place, and
// their FILE place labels differ from the places the chart list shows.
const (
	asterChart = "@AI780  ; Astrolog chart info.\n-qb Jun 15 1990 4:30pm DT 1E 4:53E 52:22N\n-zi \"Synthetic Aster\" \"Aster Home Label\"\n"
	birchChart = "@AI780  ; Astrolog chart info.\n-qb Jan 20 1985 9:00am ST 1E 11:34E 48:08N\n-zi \"Synthetic Birch\" \"Birch Home Label\"\n"
	// twinChart has Aster's moment and place under another name.
	twinChart = "@AI780  ; Astrolog chart info.\n-qb Jun 15 1990 4:30pm DT 1E 4:53E 52:22N\n-zi \"Synthetic Twin\" \"Twin Home Label\"\n"
	// untitledChart states no title at all; the list shows its file stem.
	untitledChart = "@AI780  ; Astrolog chart info.\n-qb Sep 23 1995 6:00pm DT 1E 11:34E 48:08N\n"
)

const (
	asterList = "Amsterdam, NL" // what the chart list shows for asterChart's coordinates
	birchList = "Munich, DE"
)

// namesLive is a ready 168x40 workspace (Details beside a four-chart left column) with colours off over the library
// {files}, the way a launch with that library looks.
func namesLive(t *testing.T, files map[string]string) Model {
	t.Helper()
	needLiveAstrolog(t)
	titleLibrary(t, nil, files)
	m := readyModelWithSlots(t, berlinP)
	m.cfg = Config{Color: ColorNever}
	m.width, m.height = dualColumnMinWidth(4), 40
	return m
}

// pickLive picks file through the real pick, resolve and anchor, then lets the
// frame it dispatches settle (real Astrolog throughout).
func pickLive(t *testing.T, m Model, file string) Model {
	t.Helper()
	u, load := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = u.(Model)
	if load == nil {
		t.Fatal("Ctrl+O dispatched no load")
	}
	m, _ = runCmd(t, m, load)
	m, cmd := pickRow(t, m, file)
	if cmd == nil && m.startup == startupTitleOpen {
		// An untitled chart asks for a title first; Enter with it left empty
		// is the pick as it was (the specification C3.6).
		u, cmd = updateTitleFormKey(m, tea.KeyMsg{Type: tea.KeyEnter})
		m = u.(Model)
	}
	if cmd == nil {
		t.Fatalf("picking %s dispatched no resolve (startup %s)", file, m.startup)
	}
	m, next := runCmd(t, m, cmd)
	if m.picker.resolveErr != nil || m.picker.refusal != "" {
		t.Fatalf("the pick of %s did not anchor: %v %q", file, m.picker.resolveErr, m.picker.refusal)
	}
	return pump(t, m, next)
}

// slotOf is the anchored slot holding file.
func slotOf(t *testing.T, m Model, file string) *store.Slot {
	t.Helper()
	for i := 0; i < m.slots.AnchoredCount(); i++ {
		if s := m.slots.All()[i]; s.Chart.Source.File == file {
			return s
		}
	}
	t.Fatalf("no slot holds %s", file)
	return nil
}

// namesSettle recomputes the text frame and, where the focus asks for one, the
// Details report, and lets both land.
func namesSettle(t *testing.T, m Model) Model {
	t.Helper()
	m, cmd := m.bumpIdentity(surfaceText)
	m = pump(t, m, cmd)
	m, cmd = m.requestDetails()
	return pump(t, m, cmd)
}

// detailsRows are the rows Details paints, panel border and padding removed.
func detailsRows(t *testing.T, view string) []string {
	t.Helper()
	var rows []string
	inDetails := false
	left := 0
	for _, ln := range strings.Split(ansi.Strip(view), "\n") {
		r := []rune(ln)
		if i := strings.Index(ln, detailsPrefix); i >= 0 && strings.Contains(ln, "╭") {
			inDetails = true
			// the Details panel's left border column, counted in cells
			left = len([]rune(ln[:strings.LastIndex(ln[:i], "╭")]))
			continue
		}
		if !inDetails {
			continue
		}
		if strings.Contains(ln, "╰") && len(r) > left && r[left] == '╰' {
			break
		}
		if len(r) <= left+1 || r[left] != '│' {
			continue
		}
		body := string(r[left+1:])
		if j := strings.LastIndex(body, "│"); j >= 0 {
			body = body[:j]
		}
		rows = append(rows, strings.TrimRight(strings.TrimSpace(body), " "))
	}
	if len(rows) == 0 {
		t.Fatalf("no Details panel in\n%s", ansi.Strip(view))
	}
	return rows
}

// ---- C5 -----------------------------------------------------------------

func TestDetailsReportsNameTheirCharts(t *testing.T) {
	const oneLine = `^Astrolog 8\.00 chart for [A-Z][a-z]{2} [A-Z][a-z]{2} [ 0-9]{2}, `
	reOneLine := regexp.MustCompile(oneLine)

	// momentLine is the second line a named header prints: the chart's own
	// moment and, after the zone, the chart-list place.
	wantMoment := func(t *testing.T, line, place string) {
		t.Helper()
		if !strings.Contains(line, ") "+place+" ") {
			t.Errorf("the moment line %q does not show the chart-list place %q", line, place)
		}
		if strings.Contains(line, "Home Label") {
			t.Errorf("the moment line %q shows the file's own place label", line)
		}
	}
	// requireBegins: the first rendered row at scroll offset zero shows the
	// header up to the panel edge, and the report text holds the whole header.
	requireBegins := func(t *testing.T, m Model, name string) {
		t.Helper()
		want := "Astrolog 8.00 chart for " + name
		if !strings.HasPrefix(m.details.body, want+"\n") {
			t.Fatalf("the report does not begin %q:\n%.300s", want, m.details.body)
		}
		if m.details.scrollOffset != 0 {
			t.Fatalf("scroll offset %d", m.details.scrollOffset)
		}
		rows := detailsRows(t, m.View())
		first := rows[0]
		if !strings.HasPrefix(want, first) && !strings.HasPrefix(first, want) {
			t.Errorf("Details paints %q as its first row, want %q (up to the panel edge)", first, want)
		}
		if len(first) < 20 {
			t.Errorf("the first row %q is too short to be the header", first)
		}
	}

	newSingle := func(t *testing.T) Model {
		m := namesLive(t, map[string]string{"Aster.as": asterChart, "Birch.as": birchChart})
		m = pickLive(t, m, "Aster.as")
		// The first load selects the floating slot beside the new chart; a
		// single chart is the cursor's own, alone.
		m.slots.SetSlotSelected(m.slots.AnchoredCount(), false)
		m.cursorIdx = 0
		return m
	}

	t.Run("Chart", func(t *testing.T) {
		m := newSingle(t)
		m.focused = PanelChart
		m = namesSettle(t, m)
		if m.comparing() {
			t.Fatal("fixture: comparing")
		}
		requireBegins(t, m, "Synthetic Aster")
		wantMoment(t, strings.Split(m.details.body, "\n")[1], asterList)
	})

	t.Run("every body in the Bodies panel", func(t *testing.T) {
		m := newSingle(t)
		m.focused = PanelBodies
		m = namesSettle(t, m)
		n := 0
		for i := 0; ; i++ {
			if _, ok := bodyAtCursor(m.chart.positions, m.chart.cusps, i); !ok {
				break
			}
			m.bodyCursorIdx = i
			m, cmd := m.requestDetails()
			m = pump(t, m, cmd)
			requireBegins(t, m, "Synthetic Aster")
			n++
		}
		if n < 10 {
			t.Fatalf("only %d bodies were selectable", n)
		}
	})

	t.Run("Aspects", func(t *testing.T) {
		m := newSingle(t)
		m.focused = PanelAspects
		m = namesSettle(t, m)
		requireBegins(t, m, "Synthetic Aster")
		// Through the live chart compute, not a stubbed raw-report seam.
		if !strings.HasPrefix(m.chart.aspectsReport, "Astrolog 8.00 chart for Synthetic Aster\n") {
			t.Errorf("the chart compute's aspect text does not carry the name:\n%.200s", m.chart.aspectsReport)
		}
	})

	pairCases := []struct {
		name     string
		synastry bool
	}{{"transit pair", false}, {"synastry pair", true}}
	for _, pc := range pairCases {
		newPair := func(t *testing.T) Model {
			m := namesLive(t, map[string]string{"Aster.as": asterChart, "Birch.as": birchChart})
			m = pickLive(t, m, "Aster.as")
			if pc.synastry {
				m = pickLive(t, m, "Birch.as")
				m.slots.SetSlotSelected(m.slots.AnchoredCount(), false) // not the floating slot
				for i := 0; i < m.slots.AnchoredCount(); i++ {
					m.slots.SetSlotSelected(i, true)
				}
				// The base is the pair's left side, so ② in ① is the tab
				// of the cursor on ② read in ① (the cursor on ① reads ① in ②).
				for i := 0; i < m.slots.AnchoredCount(); i++ {
					if m.slots.All()[i] != m.slots.Baseline() {
						m.cursorIdx = i
					}
				}
			} else {
				m.cursorIdx = m.slots.AnchoredCount() // the floating slot: the pair ② in ① is its tab
			}
			m.base = nil
			m = m.noteArrival()
			if pc.synastry {
				// The tab starts on the chart alone: ② read in ①.
				m.base = m.slots.Baseline()
			} else {
				// On now the arrival tab is `Aspects`; the pair is one real `]`
				// away. namesSettle runs the compute that press starts.
				m.focused = PanelAspects
				m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
			}
			return m
		}
		// pairHeaders checks the whole header block: chart 1's name and moment
		// line, chart 2's header and moment line.
		pairHeaders := func(t *testing.T, text string) {
			t.Helper()
			lines := strings.Split(text, "\n")
			if len(lines) < 4 {
				t.Fatalf("no four header lines in\n%.300s", text)
			}
			if lines[0] != "Astrolog 8.00 chart for Synthetic Aster" {
				t.Errorf("chart 1 header %q", lines[0])
			}
			wantMoment(t, lines[1], asterList)
			if pc.synastry {
				if lines[2] != "Chart #2 comparison for Synthetic Birch" {
					t.Errorf("chart 2 header %q", lines[2])
				}
				wantMoment(t, lines[3], birchList)
				return
			}
			// The floating slot is unnamed: its header is its moment, one line.
			if !strings.HasPrefix(lines[2], "Chart #2 comparison for ") || strings.Contains(lines[2], "Synthetic") {
				t.Errorf("the floating slot's header %q is not the one-line form", lines[2])
			}
		}
		t.Run(pc.name+" Aspects", func(t *testing.T) {
			m := newPair(t)
			m.focused = PanelAspects
			m = namesSettle(t, m)
			if !m.comparing() {
				t.Fatal("fixture: not comparing")
			}
			requireBegins(t, m, "Synthetic Aster")
			pairHeaders(t, m.details.body)
			if m.details.body != m.chart.crossAspectsReport {
				t.Error("Details is not the committed frame's own pair report")
			}
		})
		t.Run(pc.name+" cross body", func(t *testing.T) {
			m := newPair(t)
			m.focused = PanelBodies
			m = namesSettle(t, m)
			m.bodyCursorIdx = 0
			m, cmd := m.requestDetails()
			m = pump(t, m, cmd)
			if !m.comparing() {
				t.Fatal("fixture: not comparing")
			}
			if !strings.Contains(m.details.title, " in ") {
				t.Fatalf("fixture: Details is not a pair report: %q", m.details.title)
			}
			requireBegins(t, m, "Synthetic Aster")
			pairHeaders(t, m.details.body)
		})
	}

	t.Run("an unnamed floating slot keeps the one-line header", func(t *testing.T) {
		m := namesLive(t, map[string]string{"Aster.as": asterChart})
		m.focused = PanelAspects
		m.cursorIdx = m.slots.AnchoredCount() // the floating slot
		m = namesSettle(t, m)
		first, _, _ := strings.Cut(m.details.body, "\n")
		if !reOneLine.MatchString(first) {
			t.Errorf("header %q", first)
		}
	})

	t.Run("two library charts with one moment and place each show their own name", func(t *testing.T) {
		m := namesLive(t, map[string]string{"Aster.as": asterChart, "Twin.as": twinChart})
		m = pickLive(t, m, "Aster.as")
		m = pickLive(t, m, "Twin.as")
		m.slots.SetSlotSelected(m.slots.AnchoredCount(), false)
		for i := 0; i < m.slots.AnchoredCount(); i++ {
			m.slots.SetSlotSelected(i, false)
		}
		for want, file := range map[string]string{"Synthetic Aster": "Aster.as", "Synthetic Twin": "Twin.as"} {
			idx := -1
			for i := 0; i < m.slots.AnchoredCount(); i++ {
				if m.slots.All()[i].Chart.Source.File == file {
					idx = i
				}
			}
			m.cursorIdx = idx
			m.focused = PanelAspects
			m = namesSettle(t, m)
			requireBegins(t, m, want)
		}
		// Selected together as a pair, each header names its own slot.
		for i := 0; i < m.slots.AnchoredCount(); i++ {
			m.slots.SetSlotSelected(i, true)
		}
		// The cursor on the baseline, read in the pair's second chart: the
		// base is the left side, ① in ②.
		baselineSlot := m.slots.Baseline()
		var second *store.Slot
		for i := 0; i < m.slots.AnchoredCount(); i++ {
			if m.slots.All()[i] != baselineSlot {
				second = m.slots.All()[i]
			}
		}
		m.cursorIdx = indexOfSlot(m, baselineSlot)
		m.base = nil
		m = m.noteArrival()
		m.base = second
		m.focused = PanelAspects
		m = namesSettle(t, m)
		if !m.comparing() {
			t.Fatal("fixture: not comparing")
		}
		// The base is the pair's left side, the cursor's chart (the
		// baseline) the right.
		lines := strings.Split(m.details.body, "\n")
		left := second.Chart.Label.Name
		other := m.focusedSlot().Chart.Label.Name
		if left == other {
			t.Fatal("fixture: the cursor is on the baseline")
		}
		if lines[0] != "Astrolog 8.00 chart for "+left || lines[2] != "Chart #2 comparison for "+other {
			t.Errorf("a pair of same-moment charts reads %q / %q, want %q then %q", lines[0], lines[2], left, other)
		}
	})
}

// ---- helpers for the cases below ----------------------------------------

// namesArgvLines are the executed commands the Log holds, oldest first.
func namesArgvLines(m Model) []string {
	var out []string
	for _, l := range m.logLines {
		if l.lane == laneArgv {
			out = append(out, l.text)
		}
	}
	return out
}

// labelSpy wraps every lookup seam the ui calls with a report and records the
// label each call was handed, then calls through to the real function.
type labelSpy struct {
	mu    sync.Mutex
	calls []string // "<seam> <name>|<place>"
}

func (s *labelSpy) note(seam string, l store.ReportLabel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, fmt.Sprintf("%s %s|%s", seam, l.Name, l.Place))
}

func (s *labelSpy) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

func spyOnLabels(t *testing.T) *labelSpy {
	t.Helper()
	spy := &labelSpy{}
	oN, oT, oO, oS := firstCallNatalPositions, firstCallTransits, overlayCrossAspects, synastryCrossAspects
	oR, oC, oY := computeRawReport, computeCrossBodyReport, computeSynastryBodyReport
	t.Cleanup(func() {
		firstCallNatalPositions, firstCallTransits, overlayCrossAspects, synastryCrossAspects = oN, oT, oO, oS
		computeRawReport, computeCrossBodyReport, computeSynastryBodyReport = oR, oC, oY
	})
	firstCallNatalPositions = func(ctx context.Context, p *lookup.Probers, c store.AnchorChart, l lookup.Lens) (lookup.Result, error) {
		spy.note("natal", c.Label)
		return oN(ctx, p, c, l)
	}
	firstCallTransits = func(ctx context.Context, p *lookup.Probers, w time.Time, pl location.PlaceInfo, l lookup.Lens) (lookup.Result, error) {
		spy.note("transit", store.ReportLabel{})
		return oT(ctx, p, w, pl, l)
	}
	overlayCrossAspects = func(ctx context.Context, l lookup.Lens, c store.AnchorChart, w time.Time, pl location.PlaceInfo) (lookup.OverlayAspects, error) {
		spy.note("overlay", c.Label)
		return oO(ctx, l, c, w, pl)
	}
	synastryCrossAspects = func(ctx context.Context, l lookup.Lens, c store.AnchorChart, w time.Time, pl location.PlaceInfo, ol store.ReportLabel) (lookup.OverlayAspects, error) {
		spy.note("synastry", c.Label)
		spy.note("synastry-other", ol)
		return oS(ctx, l, c, w, pl, ol)
	}
	computeRawReport = func(ctx context.Context, w time.Time, pl location.PlaceInfo, l lookup.Lens, r lookup.RawReportRequest) (string, string, error) {
		spy.note("report", r.Label)
		return oR(ctx, w, pl, l, r)
	}
	computeCrossBodyReport = func(ctx context.Context, l lookup.Lens, c store.AnchorChart, w time.Time, pl location.PlaceInfo, b aspect.Body) (string, string, error) {
		spy.note("cross-body", c.Label)
		return oC(ctx, l, c, w, pl, b)
	}
	computeSynastryBodyReport = func(ctx context.Context, l lookup.Lens, c store.AnchorChart, w time.Time, pl location.PlaceInfo, ol store.ReportLabel, b aspect.Body) (string, string, error) {
		spy.note("synastry-body", c.Label)
		spy.note("synastry-body-other", ol)
		return oY(ctx, l, c, w, pl, ol, b)
	}
	return spy
}

// ---- C0 / C6: an untitled chart sends no name ----------------------------

var reEmptyZI = regexp.MustCompile(` -zi '' ''( |$)`)

func requireUnnamedCalls(t *testing.T, what string, m Model, spy *labelSpy, forbidden ...string) {
	t.Helper()
	for _, c := range spy.snapshot() {
		_, label, _ := strings.Cut(c, " ")
		if label != "|" {
			t.Errorf("%s: a call carried the label %q (%s)", what, label, c)
		}
	}
	checked := 0
	for _, line := range namesArgvLines(m) {
		// The chart and report calls; the timeline's searches are not reports.
		if !strings.Contains(line, " -v0") && !strings.Contains(line, " -aa") && !strings.Contains(line, "=j0") && !strings.Contains(line, " -RO") {
			continue
		}
		// A relationship call (-rt / -r0) takes two staged .as files instead of
		// a -zi pair; the arrival on ① with now selected makes one since
		// the specification rule 3, and its labels are
		// checked through the spy above.
		if strings.Contains(line, " -rt ") || strings.Contains(line, " -r0 ") {
			continue
		}
		checked++
		if !reEmptyZI.MatchString(line) {
			t.Errorf("%s: a chart call does not send `-zi '' ''`: %s", what, line)
		}
		for _, f := range forbidden {
			if strings.Contains(line, f) {
				t.Errorf("%s: a command carries %q: %s", what, f, line)
			}
		}
	}
	if checked == 0 {
		t.Errorf("%s: no chart command was recorded", what)
	}
}

func TestUntitledChartSendsNoNameAfterPickAndRestore(t *testing.T) {
	needLiveAstrolog(t)
	reOneLine := regexp.MustCompile(`^Astrolog 8\.00 chart for [A-Z][a-z]{2} [A-Z][a-z]{2} [ 0-9]{2}, `)
	files := map[string]string{"Untitled.as": untitledChart, "Aster.as": asterChart}

	t.Run("after a pick", func(t *testing.T) {
		m := namesLive(t, files)
		spy := spyOnLabels(t)
		m = pickLive(t, m, "Untitled.as")
		slot := slotOf(t, m, "Untitled.as")
		if slot.Chart.Name != "Untitled" {
			t.Fatalf("fixture: the list shows %q, not the file stem", slot.Chart.Name)
		}
		if !slot.Chart.Label.IsZero() {
			t.Errorf("an untitled chart carries the label %+v", slot.Chart.Label)
		}
		m.slots.SetSlotSelected(m.slots.AnchoredCount(), false)
		m.cursorIdx = 0
		for _, panel := range []PanelID{PanelChart, PanelAspects} {
			m.focused = panel
			m = namesSettle(t, m)
			first, _, _ := strings.Cut(m.details.body, "\n")
			if !reOneLine.MatchString(first) {
				t.Errorf("%v: the header %q is not the one-line form", panel, first)
			}
		}
		requireUnnamedCalls(t, "pick", m, spy, "Untitled")
	})

	// restore builds a relaunch whose session holds rec and whose library
	// holds files, and lets the first frame and the re-check both land.
	restore := func(t *testing.T, files map[string]string, rec store.SessionChart) (Model, *labelSpy) {
		t.Helper()
		sessionSandbox(t)
		titleLibrary(t, nil, files)
		if err := store.SaveSession(store.Session{FloatingSelected: false, Slots: []store.SessionSlot{{Chart: rec, Usable: true, Selected: true}}}); err != nil {
			t.Fatal(err)
		}
		spy := spyOnLabels(t)
		var tm tea.Model = NewModelWithClock(Config{Color: ColorNever}, FrozenClock(sessionFrozen))
		tm, _ = tm.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
		tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, rec.Place})})
		p := withLocation(readPerimeterCmd(false)().(ingest.StartupPerimeterReadMsg))
		tm, cmd := tm.Update(p)
		m := pumpPatient(t, tm.(Model), cmd)
		return m, spy
	}
	munich := location.PlaceInfo{Name: "Munich", Country: "DE", Lat: 48.1333, Lon: 11.5667, TZ: "Europe/Berlin"}
	when := time.Date(1995, 9, 23, 16, 0, 0, 0, time.UTC)

	t.Run("after a restore of an untitled chart", func(t *testing.T) {
		m, spy := restore(t, files, store.SessionChart{ID: "untitled", Name: "Untitled", When: when, Place: munich,
			Source: store.ChartSource{Kind: store.ChartKindFile, File: "Untitled.as"}})
		slot := slotOf(t, m, "Untitled.as")
		if !slot.Chart.Label.IsZero() {
			t.Errorf("the restored untitled chart carries the label %+v", slot.Chart.Label)
		}
		m.focused = PanelChart
		m = namesSettle(t, m)
		first, _, _ := strings.Cut(m.details.body, "\n")
		if !reOneLine.MatchString(first) {
			t.Errorf("the header %q is not the one-line form", first)
		}
		requireUnnamedCalls(t, "restore", m, spy, "Untitled")
	})

	for _, tc := range []struct {
		name  string
		files map[string]string
	}{
		{"whose chart file is gone", map[string]string{"Aster.as": asterChart}},
		{"whose chart file is not a chart", map[string]string{"Gone.as": "not an astrolog chart file\n", "Aster.as": asterChart}},
	} {
		t.Run("after a restore "+tc.name, func(t *testing.T) {
			m, spy := restore(t, tc.files, store.SessionChart{ID: "gone", Name: "Snapshot Client", When: when, Place: munich,
				Source: store.ChartSource{Kind: store.ChartKindFile, File: "Gone.as"}})
			slot := slotOf(t, m, "Gone.as")
			if slot.Chart.Name != "Snapshot Client" || !slot.Chart.Label.IsZero() {
				t.Errorf("the snapshot slot is %q with label %+v, want the display name and no label", slot.Chart.Name, slot.Chart.Label)
			}
			m.focused = PanelChart
			m = namesSettle(t, m)
			requireUnnamedCalls(t, "restore, file "+tc.name, m, spy, "Snapshot Client")
		})
	}
}

// N23: from cold start through the re-check, no report call carries the
// snapshot's name; the label arrives only from the file, once it has been read.
func TestMissingRestoredSourceNeverEmitsSnapshotName(t *testing.T) {
	needLiveAstrolog(t)
	sessionSandbox(t)
	titleLibrary(t, nil, map[string]string{"Aster.as": asterChart})
	when := time.Date(1990, 6, 15, 2, 30, 0, 0, time.UTC)
	place := location.PlaceInfo{Name: "Amsterdam", Country: "NL", Lat: 52.3667, Lon: 4.8833, TZ: "Europe/Amsterdam"}
	rec := store.SessionChart{ID: "gone", Name: "Snapshot Client", When: when, Place: place,
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "Gone.as"}}
	if err := store.SaveSession(store.Session{Slots: []store.SessionSlot{{Chart: rec, Usable: true, Selected: true}}}); err != nil {
		t.Fatal(err)
	}
	spy := spyOnLabels(t)
	var tm tea.Model = NewModelWithClock(Config{Color: ColorNever}, FrozenClock(sessionFrozen))
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, place})})
	p := withLocation(readPerimeterCmd(false)().(ingest.StartupPerimeterReadMsg))
	tm, cmd := tm.Update(p)
	m := tm.(Model)
	// The first compute alone, before the re-check has run.
	if first, ok := buildTextCmd(m, context.Background(), 1, 1)().(FirstFrameReadyMsg); !ok || first.Err != nil {
		t.Fatalf("the first compute failed: %#v", first.Err)
	}
	early := spy.snapshot()
	if len(early) == 0 {
		t.Fatal("fixture: the first compute made no report call")
	}
	for _, c := range early {
		if strings.Contains(c, "Snapshot Client") {
			t.Errorf("the first compute carries the snapshot name: %s", c)
		}
	}
	m = pumpPatient(t, m, cmd)
	m.focused = PanelChart
	m = namesSettle(t, m)
	for _, c := range spy.snapshot() {
		if strings.Contains(c, "Snapshot Client") || strings.Contains(c, "Gone") {
			t.Errorf("a call through the re-check carries the snapshot name: %s", c)
		}
	}
	requireUnnamedCalls(t, "cold start through the re-check", m, spy, "Snapshot Client")
}

// ---- N25 / N31: one read of the file at pick -------------------------------

func TestPickReadsNameAndMomentTogether(t *testing.T) {
	pickAfterReplacing := func(t *testing.T, replacement string) (Model, tea.Cmd) {
		t.Helper()
		m := namesLive(t, map[string]string{"Client.as": asterChart})
		u, load := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
		m = u.(Model)
		m, _ = runCmd(t, m, load)
		row := m.picker.matches[0]
		if row.Name != "Synthetic Aster" || row.Place.Display() != asterList {
			t.Fatalf("fixture: the list shows %q at %q", row.Name, row.Place.Display())
		}
		// Another client's chart replaces the file between the listing and the pick.
		if err := os.WriteFile(filepath.Join(store.ChartsDir(), "Client.as"), []byte(replacement), 0o600); err != nil {
			t.Fatal(err)
		}
		m, cmd := pickRow(t, m, "Client.as")
		return runCmd(t, m, cmd)
	}
	t.Run("a titled file replaced by another titled one", func(t *testing.T) {
		m, next := pickAfterReplacing(t, birchChart)
		slot := slotOf(t, m, "Client.as")
		if got, want := slot.Chart.Label, (store.ReportLabel{Name: "Synthetic Birch", Place: birchList}); got != want {
			t.Errorf("label %+v, want the replacement's %+v", got, want)
		}
		if want := time.Date(1985, 1, 20, 8, 0, 0, 0, time.UTC); !slot.Time.Equal(want) {
			t.Errorf("moment %v, want the replacement's %v", slot.Time, want)
		}
		if slot.Location.Display() != birchList || slot.Chart.Place != slot.Location {
			t.Errorf("place %q (chart %q), want the replacement's %q", slot.Location.Display(), slot.Chart.Place.Display(), birchList)
		}
		if slot.Chart.Name != "Synthetic Birch" {
			t.Errorf("the slot is named %q, want the replacement's title", slot.Chart.Name)
		}
		m = pump(t, m, next)
		m.slots.SetSlotSelected(m.slots.AnchoredCount(), false)
		m.cursorIdx = 0
		m.focused = PanelAspects
		m = namesSettle(t, m)
		if !strings.HasPrefix(m.details.body, "Astrolog 8.00 chart for Synthetic Birch\n") || !strings.Contains(strings.Split(m.details.body, "\n")[1], birchList) {
			t.Errorf("the report is not the replacement's:\n%.200s", m.details.body)
		}
	})
	t.Run("a titled file that became untitled", func(t *testing.T) {
		m, _ := pickAfterReplacing(t, untitledChart)
		slot := slotOf(t, m, "Client.as")
		// Displayed under its file stem, sending no name; the old client's name
		// appears nowhere on the slot.
		if slot.Chart.Name != "Client" {
			t.Errorf("the slot is named %q, want the file stem %q", slot.Chart.Name, "Client")
		}
		if !slot.Chart.Label.IsZero() {
			t.Errorf("an untitled read sends the label %+v", slot.Chart.Label)
		}
		if want := time.Date(1995, 9, 23, 16, 0, 0, 0, time.UTC); !slot.Time.Equal(want) {
			t.Errorf("moment %v, want the replacement's %v", slot.Time, want)
		}
		if slot.Location.Display() != birchList || slot.Chart.Place != slot.Location {
			t.Errorf("place %q, want the replacement's %q", slot.Location.Display(), birchList)
		}
	})
	t.Run("a file whose place cannot be named", func(t *testing.T) {
		beyond := strings.Replace(asterChart, "4:53E 52:22N", "150:00W 80:00S", 1)
		m, _ := pickAfterReplacing(t, beyond)
		if m.slots.AnchoredCount() != 0 {
			t.Fatalf("a chart with no known place was anchored: %q", slotOf(t, m, "Client.as").Chart.Name)
		}
		if m.picker.refusal == "" {
			t.Error("the pick was not refused")
		}
	})
}

// ---- N21: equal-moment charts stay two charts -------------------------------

func TestPairNamesPreserveDistinctSlotsAtSameMoment(t *testing.T) {
	m := namesLive(t, map[string]string{"Aster.as": asterChart, "Twin.as": twinChart})
	m = pickLive(t, m, "Aster.as")
	m = pickLive(t, m, "Twin.as")
	for i := 0; i <= m.slots.AnchoredCount(); i++ {
		m.slots.SetSlotSelected(i, true)
	}
	m.base = nil
	m.cursorIdx = 0
	m = m.noteArrival()
	m.focused = PanelChart
	m = namesSettle(t, m)
	cols, ok := m.bodyColumnsForScene()
	if !ok || len(cols) != 3 {
		t.Fatalf("fixture: %d columns (ok %v)", len(cols), ok)
	}
	aster, twin := slotOf(t, m, "Aster.as"), slotOf(t, m, "Twin.as")
	if !aster.Time.Equal(twin.Time) || aster.Location != twin.Location {
		t.Fatalf("fixture: the two charts differ in moment or place")
	}
	if anchoredKey(anchorOf(aster)).equal(anchoredKey(anchorOf(twin))) {
		t.Fatal("two library charts with one moment and place share one chart key")
	}
	for want, slot := range map[string]*store.Slot{"Synthetic Aster": aster, "Synthetic Twin": twin} {
		res, hit := m.chart.results.get(anchoredKey(anchorOf(slot)), m.viewLens())
		if !hit {
			t.Fatalf("no cached lookup for %s", want)
		}
		if !strings.HasPrefix(res.AspectsReport, "Astrolog 8.00 chart for "+want+"\n") {
			t.Errorf("the cached report for %s is %.80q", want, res.AspectsReport)
		}
	}
}

// ---- N7: a pair's header follows the tab ------------------------------------

func TestPairHeaderFollowsTabOrder(t *testing.T) {
	m := namesLive(t, map[string]string{"Aster.as": asterChart, "Birch.as": birchChart, "Twin.as": twinChart})
	m = pickLive(t, m, "Aster.as")
	m = pickLive(t, m, "Birch.as")
	m = pickLive(t, m, "Twin.as")
	for i := 0; i <= m.slots.AnchoredCount(); i++ {
		m.slots.SetSlotSelected(i, true)
	}
	baseline := m.slots.Baseline()
	if baseline == nil || baseline.Chart.Label.Name == "" {
		t.Fatal("fixture: no named baseline")
	}
	// Details needs room beside the grown left column of a four-chart set.
	m.width = max(m.width, dualColumnMinWidth(len(m.slots.SelectedInOrder())))
	// Both scenes: the cursor's chart is read in every other anchored member,
	// with now in the set or not (a moment is never a base). The report's
	// first chart is the pair's LEFT chart, its second the right (rule 6).
	for _, nowSelected := range []bool{true, false} {
		sm := m
		if !nowSelected {
			sm.slots.SetSlotSelected(sm.slots.AnchoredCount(), false)
		}
		sm.base = nil
		sm.cursorIdx = indexOfSlot(sm, baseline)
		sm = sm.noteArrival()
		if len(sm.pairSlots()) != 2 {
			t.Fatalf("fixture (now selected %v): %d pair tabs, want 2", nowSelected, len(sm.pairSlots()))
		}
		pairs := sm.pairSlots()
		if nowSelected {
			// The floating slot's row reads now in each of the three charts.
			fm := sm
			fm.cursorIdx = fm.slots.AnchoredCount()
			if len(fm.pairSlots()) != 3 {
				t.Fatalf("fixture: now's row has %d pair tabs, want 3", len(fm.pairSlots()))
			}
			pairs = append(pairs, fm.pairSlots()...)
		}
		for _, pair := range pairs {
			for _, panel := range []PanelID{PanelAspects, PanelBodies} {
				mm := showPair(sm, pair)
				mm.focused = panel
				mm.bodyCursorIdx = 0
				mm = namesSettle(t, mm)
				sc := mm.deriveScene()
				tab := mm.pairTabTitle(sc.numeral(pair.left), sc.numeral(pair.right))
				first := "Astrolog 8.00 chart for " + pair.left.Chart.Label.Name
				lines := strings.Split(mm.details.body, "\n")
				if len(lines) < 4 || lines[0] != first {
					t.Fatalf("tab %s on %v (now selected %v): the first chart is %q, want the left chart %q", tab, panel, nowSelected, lines[0], first)
				}
				wantSecond := "Chart #2 comparison for "
				if !pair.right.Floating {
					if want := wantSecond + pair.right.Chart.Label.Name; lines[2] != want {
						t.Errorf("tab %s on %v: the second chart is %q, want %q", tab, panel, lines[2], want)
					}
				} else if !strings.HasPrefix(lines[2], wantSecond) || strings.Contains(lines[2], "Synthetic") {
					t.Errorf("tab %s on %v: the floating tab's second header is %q", tab, panel, lines[2])
				}
			}
		}
	}
}

func indexOfSlot(m Model, want *store.Slot) int {
	for i, s := range m.slots.All() {
		if s == want {
			return i
		}
	}
	return -1
}

// ---- N18: rapid refocus ----------------------------------------------------

func TestRapidRefocusKeepsCurrentReportName(t *testing.T) {
	m := namesLive(t, map[string]string{"Aster.as": asterChart, "Birch.as": birchChart})
	m = pickLive(t, m, "Aster.as")
	m = pickLive(t, m, "Birch.as")
	for i := 0; i <= m.slots.AnchoredCount(); i++ {
		m.slots.SetSlotSelected(i, false)
	}
	m.focused = PanelChart
	m = namesSettle(t, m)
	idx := func(file string) int { return indexOfSlot(m, slotOf(t, m, file)) }

	for _, order := range []string{"stale second", "stale first"} {
		t.Run(order, func(t *testing.T) {
			mm := m
			mm.cursorIdx = idx("Aster.as")
			mm, cmdA := mm.startDetailsNow()
			msgA := cmdA()
			mm.cursorIdx = idx("Birch.as")
			mm, cmdB := mm.startDetailsNow()
			msgB := cmdB()
			deliver := []tea.Msg{msgB, msgA}
			if order == "stale first" {
				deliver = []tea.Msg{msgA, msgB}
			}
			for _, msg := range deliver {
				u, _ := mm.Update(msg)
				mm = u.(Model)
			}
			if !strings.HasPrefix(mm.details.body, "Astrolog 8.00 chart for Synthetic Birch\n") {
				t.Errorf("Details shows %.60q, want only the latest focused chart's name", mm.details.body)
			}
		})
	}
}

// ---- N3 / N13: a rename refreshes the report ---------------------------------

func TestRenamedChartRefreshesItsReport(t *testing.T) {
	m := namesLive(t, map[string]string{"Aster.as": asterChart})
	m = pickLive(t, m, "Aster.as")
	m.slots.SetSlotSelected(m.slots.AnchoredCount(), false)
	m.cursorIdx = 0
	m.focused = PanelAspects
	m = namesSettle(t, m)
	if !strings.HasPrefix(m.details.body, "Astrolog 8.00 chart for Synthetic Aster\n") {
		t.Fatalf("fixture: %.60q", m.details.body)
	}
	// Details is scrolled down, away from the header.
	m = m.scrollDetails(4)
	if m.details.scrollOffset == 0 {
		t.Fatal("fixture: Details did not scroll")
	}

	u, load := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m = u.(Model)
	m, _ = runCmd(t, m, load)
	m = cursorTo(t, m, "Aster.as")
	m, _ = ctrlR(t, m)
	m = eraseField(t, m)
	m, _ = escSend(t, m, escRunes("Renamed Aster"))
	m, cmd := escSend(t, m, escKey(tea.KeyEnter))
	m, cmd = runCmd(t, m, cmd)
	if m.title.refusal != "" || cmd == nil {
		t.Fatalf("the rename did not finish: %q", m.title.refusal)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("a rename of an open chart returned no reload-and-recompute batch")
	}
	m = pump(t, m, batch...)
	m, _ = escSend(t, m, escKey(tea.KeyEsc)) // back from the picker
	m = pump(t, m)
	if !strings.HasPrefix(m.details.body, "Astrolog 8.00 chart for Renamed Aster\n") {
		t.Fatalf("Details still shows the old name:\n%.200s", m.details.body)
	}
	if m.details.scrollOffset != 0 {
		t.Errorf("Details is still scrolled to %d; the new header is out of sight", m.details.scrollOffset)
	}
	if first := detailsRows(t, m.View())[0]; !strings.HasPrefix(first, "Astrolog 8.00 chart for Renamed Aster") {
		t.Errorf("Details paints %q as its first row", first)
	}
}

// ---- N14 / N19: the copied command is the executed argv -----------------------

// shellWords splits a copied command the way a shell would, without running it.
func shellWords(t *testing.T, line string) []string {
	t.Helper()
	out, err := exec.Command("sh", "-c", `eval "set -- $1"; for a in "$@"; do printf '%s\0' "$a"; done`, "sh", line).Output()
	if err != nil {
		t.Fatalf("splitting %q: %v", line, err)
	}
	return strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
}

func TestNamedCommandCopyRoundTripsArgv(t *testing.T) {
	needLiveAstrolog(t)
	// Every name is paired with what Astrolog is sent: controls become spaces.
	cases := []struct{ name, label, sent string }{
		{"quotes, semicolon and substitution", `O'Brien; $(echo PWNED) \ \x ` + "`id`", `O'Brien; $(echo PWNED) \ \x ` + "`id`"},
		{"leading dash", "-v", "-v"},
		{"non-ASCII", "Zoë 日本", "Zoë 日本"},
	}
	for r := rune(1); r <= 0x1f; r++ {
		cases = append(cases, struct{ name, label, sent string }{fmt.Sprintf("control U+%04X", r), "A" + string(r) + "B", "A B"})
	}
	cases = append(cases, struct{ name, label, sent string }{"DEL", "A\x7fB", "A B"})

	base := namesLive(t, map[string]string{"Client.as": asterChart})
	base = pickLive(t, base, "Client.as")
	base.slots.SetSlotSelected(base.slots.AnchoredCount(), false)
	base.cursorIdx = 0
	base.focused = PanelChart
	copyOf := func(t *testing.T, label string) string {
		t.Helper()
		// Whatever the chart file could hold, the label is what the ui hands on:
		// Astrolog's own reader refuses some control bytes in a title, so the
		// label is set on the slot rather than written into a file.
		slotOf(t, base, "Client.as").Chart.Label = store.ReportLabel{Name: label, Place: asterList}
		m := namesSettle(t, base)
		var captured string
		old := yankLine
		yankLine = func(text string) error { captured = text; return nil }
		t.Cleanup(func() { yankLine = old })
		// Open the console on the newest chart report and copy it with Enter.
		u, _ := m.openConsole()
		m = u.(Model)
		found := false
		for i := len(m.console.snapshot) - 1; i >= 0; i-- {
			l := m.console.snapshot[i]
			if l.lane == laneArgv && strings.Contains(l.text, " -qj ") && strings.Contains(l.text, "=j0") {
				m.console.cursor, found = i, true
				break
			}
		}
		if !found {
			t.Fatalf("no chart report command in the Log: %q", namesArgvLines(m))
		}
		m.updateConsoleKey(tea.KeyMsg{Type: tea.KeyEnter})
		if captured == "" {
			t.Fatal("nothing was copied")
		}
		return captured
	}
	control := shellWords(t, copyOf(t, "Plain Name"))
	zi := slices.Index(control, "-zi")
	if zi < 0 || control[zi+1] != "Plain Name" || control[zi+2] != asterList {
		t.Fatalf("fixture: the control's -zi operands are not the label: %q", control)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			words := shellWords(t, copyOf(t, c.label))
			if len(words) != len(control) {
				t.Fatalf("the copied command parses to %d words, the control's %d: %q", len(words), len(control), words)
			}
			for i := range words {
				if i == zi+1 {
					if words[i] != c.sent {
						t.Errorf("the name parses back as %q, want %q", words[i], c.sent)
					}
					continue
				}
				if words[i] != control[i] {
					t.Errorf("word %d is %q, the control's is %q", i, words[i], control[i])
				}
			}
			for _, w := range words {
				if w == "PWNED" || strings.HasPrefix(w, "uid=") {
					t.Errorf("a substitution inside the name was run: %q", w)
				}
			}
		})
	}
}

// ---- N20: a failed report keeps Astrolog's stdout out of the Log -----------

func TestFailedNamedReportKeepsAstrologOutputOutOfTheLog(t *testing.T) {
	m := detailsTestModel(t)
	m.slots.All()[0].Chart.Label = store.ReportLabel{Name: "Synthetic Aster", Place: asterList}
	old := computeRawReport
	t.Cleanup(func() { computeRawReport = old })
	computeRawReport = func(context.Context, time.Time, location.PlaceInfo, lookup.Lens, lookup.RawReportRequest) (string, string, error) {
		return "", "astrolog -zi 'Synthetic Aster' 'Amsterdam, NL'", errors.New("astrolog: ComputeRawReport: exit status 1 (output: Astrolog 8.00 chart for Synthetic Aster)")
	}
	m.focused = PanelChart
	m, cmd := m.requestDetails()
	m = pump(t, m, cmd)
	for _, l := range m.logLines {
		if l.lane == laneArgv {
			continue // the command itself may carry the name: the operator chose that
		}
		if strings.Contains(l.text, "Synthetic Aster") || strings.Contains(l.text, "output:") {
			t.Errorf("the Log carries Astrolog's stdout: %q", l.text)
		}
	}
}

// ---- C6: an unnamed slot keeps today's header --------------------------------

func TestUnnamedDetailsKeepOneLineHeader(t *testing.T) {
	needLiveAstrolog(t)
	oneLine := regexp.MustCompile(`^Astrolog 8\.00 chart for [A-Z][a-z]{2} [A-Z][a-z]{2} [ 0-9]{2}, `)
	newModel := func(t *testing.T) Model {
		t.Helper()
		m := readyModelWithSlots(t, berlinP)
		m.cfg = Config{Color: ColorNever}
		m.width, m.height = 132, 40
		return m
	}
	check := func(t *testing.T, m Model, what string) {
		t.Helper()
		for _, panel := range []PanelID{PanelChart, PanelAspects, PanelBodies} {
			m.focused = panel
			m.bodyCursorIdx = 0
			mm := namesSettle(t, m)
			if panel == PanelBodies && (mm.details.title == "" || mm.details.title == detailsPrefix+tabTitleChart || mm.details.title == detailsPrefix+tabTitleAspects) {
				t.Fatalf("%s: no body report: %q", what, mm.details.title)
			}
			lines := strings.Split(mm.details.body, "\n")
			if len(lines) < 3 || !oneLine.MatchString(lines[0]) {
				t.Errorf("%s on %v: the header is %q, want today's one-line form", what, panel, lines[0])
			}
			// One line: nothing of the place is printed beneath it.
			if strings.Contains(strings.Join(lines[:2], "\n"), "Berlin") || strings.Contains(strings.Join(lines[:2], "\n"), "Tokyo") {
				t.Errorf("%s on %v: the header prints a place:\n%s", what, panel, strings.Join(lines[:2], "\n"))
			}
			if rows := detailsRows(t, mm.View()); !oneLine.MatchString(rows[0]) {
				t.Errorf("%s on %v: Details paints %q as its first row", what, panel, rows[0])
			}
		}
	}
	t.Run("the floating slot", func(t *testing.T) {
		check(t, newModel(t), "floating")
	})
	t.Run("an atlas-picked floating slot", func(t *testing.T) {
		m := newModel(t)
		tokyo := location.PlaceInfo{Name: "Tokyo", Country: "JP", Lat: 35.6762, Lon: 139.6503, TZ: "Asia/Tokyo"}
		m, cmd := applyAtlasPicked(m, AtlasPickedMsg{Place: tokyo})
		_ = cmd // persisting the city is not this test's
		m.startup = startupReady
		check(t, m, "atlas pick")
	})
	t.Run("an unnamed anchored slot", func(t *testing.T) {
		m := newModel(t)
		if err := addAnchor(m.slots, time.Date(1990, 6, 15, 14, 30, 0, 0, time.UTC), berlinP); err != nil {
			t.Fatal(err)
		}
		m.slots.SetSlotSelected(m.slots.AnchoredCount(), false)
		m.cursorIdx = 0
		if !m.slots.All()[0].Chart.Label.IsZero() {
			t.Fatal("fixture: the slot carries a label")
		}
		check(t, m, "unnamed anchored")
	})
}

// ---- C0: the re-check is where a restored chart gets its name -------------------

// A restored titled chart sends no name on the first frame and takes its title
// and place from the file once the re-check has read it; the reports then
// recompute under that name.
func TestRestoredChartNamesItselfAfterTheRecheck(t *testing.T) {
	needLiveAstrolog(t)
	sessionSandbox(t)
	titleLibrary(t, nil, map[string]string{"Aster.as": asterChart})
	// The snapshot a quit would have written for this file: the listing row's
	// casting place and the file's own moment (the re-check compares both).
	lib, err := lookup.LoadChartLibrary()
	if err != nil || len(lib.Charts) != 1 {
		t.Fatalf("fixture library: %+v %v", lib.Charts, err)
	}
	row := lib.Charts[0]
	real, err := lookup.ResolvePickedChart(context.Background(), row)
	if err != nil {
		t.Fatal(err)
	}
	place := row.Place
	rec := store.SessionChart{ID: row.ID, Name: row.Name, When: real.When, Place: row.Place, Source: row.Source}
	if err := store.SaveSession(store.Session{Slots: []store.SessionSlot{{Chart: rec, Usable: true, Selected: true}}}); err != nil {
		t.Fatal(err)
	}
	var tm tea.Model = NewModelWithClock(Config{Color: ColorNever}, FrozenClock(sessionFrozen))
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, place})})
	p := withLocation(readPerimeterCmd(false)().(ingest.StartupPerimeterReadMsg))
	tm, cmd := tm.Update(p)
	m := tm.(Model)
	if got := slotOf(t, m, "Aster.as").Chart.Label; !got.IsZero() {
		t.Fatalf("the restored slot carried the label %+v before its file was read", got)
	}
	// The re-check is a live Astrolog read; a loaded host can keep it past
	// pump's 400 ms (bug report of 2026-10-10).
	m = pumpPatient(t, m, cmd)
	want := store.ReportLabel{Name: "Synthetic Aster", Place: asterList}
	if got := slotOf(t, m, "Aster.as").Chart.Label; got != want {
		t.Fatalf("after the re-check the label is %+v, want %+v", got, want)
	}
	m.slots.SetSlotSelected(m.slots.AnchoredCount(), false)
	m.cursorIdx = 0
	m.focused = PanelAspects
	m = namesSettle(t, m)
	if !strings.HasPrefix(m.details.body, "Astrolog 8.00 chart for Synthetic Aster\n") {
		t.Errorf("the report is not named after the re-check:\n%.200s", m.details.body)
	}
}

// The re-check judges the file by what the re-read found: the row it compares
// with the snapshot takes the read's title and place, and the label is the
// read's own. A read that could not place the chart changes nothing and sends
// no name.
func TestRecheckLabelRequiresTheRowsFile(t *testing.T) {
	munich := location.PlaceInfo{Name: "Munich", Country: "DE", Lat: 48.1333, Lon: 11.5667, TZ: "Europe/Berlin"}
	berlin := location.PlaceInfo{Name: "Mitte", Country: "DE", Lat: 52.5167, Lon: 13.4, TZ: "Europe/Berlin"}
	src := store.ChartSource{Kind: store.ChartKindFile, File: "stem.as"}
	titled := store.NamedChart{Name: "Synthetic Aster", Place: munich, Source: src}
	untitled := store.NamedChart{Name: "stem", Untitled: true, Place: munich, Source: src}
	read := func(name string, pl location.PlaceInfo) lookup.PickedChart {
		label := store.ReportLabel{}
		if name != "" {
			label = store.ReportLabel{Name: name, Place: pl.Display()}
		}
		return lookup.PickedChart{Label: label, Place: pl, HasPlace: true}
	}
	for _, tc := range []struct {
		name      string
		row       store.NamedChart
		got       lookup.PickedChart
		wantName  string
		wantPlace location.PlaceInfo
		wantUntit bool
		want      store.ReportLabel
	}{
		{"same file", titled, read("Synthetic Aster", munich), "Synthetic Aster", munich, false, store.ReportLabel{Name: "Synthetic Aster", Place: "Munich, DE"}},
		{"untitled row, untitled read", untitled, read("", munich), "stem", munich, true, store.ReportLabel{}},
		{"the file now carries another title", titled, read("Synthetic Birch", munich), "Synthetic Birch", munich, false, store.ReportLabel{Name: "Synthetic Birch", Place: "Munich, DE"}},
		{"the file now carries another place", titled, read("Synthetic Aster", berlin), "Synthetic Aster", berlin, false, store.ReportLabel{Name: "Synthetic Aster", Place: "Mitte, DE"}},
		{"an untitled row, a titled read", untitled, read("Synthetic Birch", munich), "Synthetic Birch", munich, false, store.ReportLabel{Name: "Synthetic Birch", Place: "Munich, DE"}},
		{"a titled row, an untitled read", titled, read("", munich), "stem", munich, true, store.ReportLabel{}},
		{"a read that could not place the chart", titled, lookup.PickedChart{}, "Synthetic Aster", munich, false, store.ReportLabel{}},
	} {
		row := rowOfRead(tc.row, tc.got)
		if row.Name != tc.wantName || row.Place != tc.wantPlace || row.Untitled != tc.wantUntit {
			t.Errorf("%s: row %q at %q (untitled %v), want %q at %q (%v)", tc.name, row.Name, row.Place.Display(), row.Untitled, tc.wantName, tc.wantPlace.Display(), tc.wantUntit)
		}
		if got := labelOfRead(tc.got); got != tc.want {
			t.Errorf("%s: label %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// C0: the re-check takes the name, the place and the moment from ONE read. A
// file replaced between the listing and the resolve is either anchored whole
// under the replacement's name, place and moment, or refused; the old client is
// never put at the replacement's birth moment.
func TestRestoreRecheckReadsNameAndMomentTogether(t *testing.T) {
	needLiveAstrolog(t)
	// restore anchors Aster.as from a session, replaces the file after the
	// re-check's listing, and runs the re-check synchronously (a timing-bounded
	// pump could abandon it under load and let the test pass without it).
	restore := func(t *testing.T, replacement string) (Model, SessionChartRecheckedMsg, store.SessionChart) {
		t.Helper()
		sessionSandbox(t)
		titleLibrary(t, nil, map[string]string{"Aster.as": asterChart})
		lib, err := lookup.LoadChartLibrary()
		if err != nil || len(lib.Charts) != 1 {
			t.Fatalf("fixture library: %+v %v", lib.Charts, err)
		}
		row := lib.Charts[0]
		real, err := lookup.ResolvePickedChart(context.Background(), row)
		if err != nil {
			t.Fatal(err)
		}
		rec := store.SessionChart{ID: row.ID, Name: row.Name, When: real.When, Place: row.Place, Source: row.Source}
		if err := store.SaveSession(store.Session{Slots: []store.SessionSlot{{Chart: rec, Usable: true, Selected: true}}}); err != nil {
			t.Fatal(err)
		}
		var tm tea.Model = NewModelWithClock(Config{Color: ColorNever}, FrozenClock(sessionFrozen))
		tm, _ = tm.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
		tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, row.Place})})
		p := withLocation(readPerimeterCmd(false)().(ingest.StartupPerimeterReadMsg))
		tm, _ = tm.Update(p)
		m := tm.(Model)
		old := afterRecheckListing
		t.Cleanup(func() { afterRecheckListing = old })
		afterRecheckListing = func() {
			if err := os.WriteFile(filepath.Join(store.ChartsDir(), "Aster.as"), []byte(replacement), 0o600); err != nil {
				t.Error(err)
			}
		}
		msg, ok := recheckSessionChartCmd(rec, slotOf(t, m, "Aster.as"))().(SessionChartRecheckedMsg)
		if !ok {
			t.Fatal("the re-check did not answer with its message")
		}
		m, _ = applySessionChartRecheck(m, msg)
		return m, msg, rec
	}
	requireOriginal := func(t *testing.T, m Model, rec store.SessionChart) {
		t.Helper()
		slot := slotOf(t, m, "Aster.as")
		// The substitution is REFUSED (review round 3): the slot keeps the
		// snapshot's own moment and place and sends no name; it is never
		// re-anchored at the replacement, and never a mix of the two.
		if !(slot.Time.Equal(rec.When) && slot.Chart.Name == rec.Name && slot.Location == rec.Place) {
			t.Fatalf("the slot is not the snapshot: %q at %v in %q (snapshot %q at %v in %q)", slot.Chart.Name, slot.Time, slot.Location.Display(), rec.Name, rec.When, rec.Place.Display())
		}
		if !slot.Chart.Label.IsZero() {
			t.Errorf("a refused source still sends the name %+v", slot.Chart.Label)
		}
	}
	sessionSaid := func(m Model, want string) bool {
		for _, l := range sessionLines(m) {
			if strings.Contains(l, want) {
				return true
			}
		}
		return false
	}

	t.Run("another client's chart", func(t *testing.T) {
		m, msg, rec := restore(t, birchChart)
		// The re-check found and read the file, and it no longer matches the
		// snapshot: the substitution is refused and reported.
		if !msg.Found || msg.Err != nil {
			t.Fatalf("the re-check did not find and read the file: found %v, err %v", msg.Found, msg.Err)
		}
		if got := msg.Row.Name; got != "Synthetic Birch" {
			t.Fatalf("the re-check judged %q, not the replacement", got)
		}
		if !sessionSaid(m, "Aster.as now holds a different chart") {
			t.Errorf("the refusal is not reported: %q", sessionLines(m))
		}
		requireOriginal(t, m, rec)
	})

	t.Run("the same moment at coordinates beyond the atlas", func(t *testing.T) {
		// Aster's own moment (4:30pm DT 1E), a place no atlas city is near, and
		// a house line the snapshot never saw.
		beyond := strings.Replace(asterChart, "4:53E 52:22N", "150:00W 80:00S", 1) + "-c 3\n"
		sessionSandbox(t)
		titleLibrary(t, nil, map[string]string{"Aster.as": asterChart})
		m, msg, rec := restore(t, beyond)
		if !msg.Found || msg.Err != nil || !msg.Unplaced {
			t.Fatalf("the re-read should have found the file but not placed it: found %v, err %v, unplaced %v", msg.Found, msg.Err, msg.Unplaced)
		}
		if !sessionSaid(m, "that chart file can't be cast") {
			t.Errorf("the refusal is not the can't-be-cast report: %q", sessionLines(m))
		}
		requireOriginal(t, m, rec)
		if got := m.ownHouses(slotOf(t, m, "Aster.as")).hs; got == recipe.Campanus {
			t.Errorf("the replacement's house system was taken: %v", got)
		}
	})
}
