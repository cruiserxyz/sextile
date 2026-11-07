package ui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// logModel is a ready dual-column workspace, accessible, with n note lines in
// the Log panel (the lane the focused Log lists).
func logModel(t *testing.T, n int) Model {
	t.Helper()
	m := detailsTestModel(t)
	m.cfg = Config{Accessible: true}
	for i := 0; i < n; i++ {
		m = m.logNote("test", "note %02d", i)
	}
	return m
}

// logTitleRow is the frame row that carries the Log box's title, markers kept.
func logTitleRow(t *testing.T, m Model) string {
	t.Helper()
	for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(row, "Log") && strings.Contains(row, "+-") {
			return row
		}
	}
	t.Fatal("no Log title row in the frame")
	return ""
}

func TestLeaderLFocusesLog(t *testing.T) {
	m := logModel(t, 8)
	m.focused = PanelBodies
	strip, body := m.tabStrips()[PanelDetails], m.details.body
	refresh := m.refresh

	m, cmd := press(t, m, ",", "l")
	if m.console.open {
		t.Fatal(",l opened the console, want the Log focused")
	}
	if !m.logFocus.active || m.currentOwner() != ownerLog {
		t.Fatalf("the Log is not focused: active %v, owner %v", m.logFocus.active, m.currentOwner())
	}
	if m.logFocus.cursor != 7 {
		t.Errorf("cursor = %d, want the newest line (7)", m.logFocus.cursor)
	}
	if m.focused != PanelBodies {
		t.Errorf("m.focused = %q, want bodies untouched", m.focused)
	}
	if cmd != nil || !reflect.DeepEqual(refresh, m.refresh) {
		t.Error("entering the Log focus started or deferred a refresh")
	}
	if !reflect.DeepEqual(strip, m.tabStrips()[PanelDetails]) || body != m.details.body {
		t.Error("Details' title or text changed when the Log took the focus")
	}
	if row := logTitleRow(t, m); !strings.Contains(row, focusMarkASCII+"Log") {
		t.Errorf("the Log box is not drawn focused: %q", row)
	}
	for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(row, "Bodies") && strings.Contains(row, focusMarkASCII) {
			t.Errorf("the prior panel still draws focused: %q", row)
		}
	}

	// Leaving puts everything back with no refresh either.
	before := m.refresh
	m, cmd = press(t, m, "esc")
	if m.logFocus.active || m.currentOwner() != ownerMain || m.focused != PanelBodies {
		t.Fatalf("esc: active %v, owner %v, focused %q", m.logFocus.active, m.currentOwner(), m.focused)
	}
	if cmd != nil || !reflect.DeepEqual(before, m.refresh) {
		t.Error("leaving the Log focus started or deferred a refresh")
	}
}

func TestLogFocusKeys(t *testing.T) {
	copied := captureYank(t)
	m := logModel(t, 12)
	m, _ = press(t, m, ",", "l")
	m, _ = press(t, m, "up", "k", "k")
	if m.logFocus.cursor != 8 {
		t.Fatalf("cursor after three ups = %d, want 8", m.logFocus.cursor)
	}
	m, _ = press(t, m, "home")
	if m.logFocus.cursor != 0 || m.logFocus.offset != 0 {
		t.Errorf("home: cursor %d offset %d, want 0 0", m.logFocus.cursor, m.logFocus.offset)
	}
	m, _ = press(t, m, "G")
	if m.logFocus.cursor != 11 {
		t.Errorf("G: cursor %d, want 11", m.logFocus.cursor)
	}
	m, _ = press(t, m, "pgup")
	if m.logFocus.cursor >= 11 {
		t.Errorf("pgup did not move the cursor: %d", m.logFocus.cursor)
	}
	m, _ = press(t, m, "g", "j")
	if m.logFocus.cursor != 1 {
		t.Errorf("g then j: cursor %d, want 1", m.logFocus.cursor)
	}
	// The window follows the cursor over all lines, not only the visible four.
	m, _ = press(t, m, "end")
	if rows := strings.Join(strings.Split(ansi.Strip(m.View()), "\n"), "\n"); !strings.Contains(rows, "test: note 11") {
		t.Error("the newest line is not on screen after End")
	}

	// Enter copies the highlighted line in full, writes no Log line.
	m, _ = press(t, m, "up")
	logLen := len(m.logLines)
	m, _ = press(t, m, "enter")
	if !reflect.DeepEqual(*copied, []string{"test: note 10"}) {
		t.Errorf("enter copied %q, want the highlighted line", *copied)
	}
	if len(m.logLines) != logLen {
		t.Error("copying a line wrote a Log line")
	}
	if !m.logFocus.active {
		t.Error("enter left the Log focus")
	}

	// l opens the console; its Esc closes onto the focused Log with the cursor
	// where it was, and only the Log's own Esc returns to the prior panel.
	m.focused = PanelAspects
	cursor := m.logFocus.cursor
	m, _ = press(t, m, "l")
	if !m.console.open || m.logFocus.active {
		t.Fatalf("l: console %v, log focus %v", m.console.open, m.logFocus.active)
	}
	m, _ = press(t, m, "esc")
	if m.console.open || !m.logFocus.active || m.currentOwner() != ownerLog || m.logFocus.cursor != cursor {
		t.Errorf("console esc: console %v, log focus %v, owner %v, cursor %d (want %d)", m.console.open, m.logFocus.active, m.currentOwner(), m.logFocus.cursor, cursor)
	}
	m, _ = press(t, m, "esc")
	if m.logFocus.active || m.focused != PanelAspects || m.currentOwner() != ownerMain {
		t.Errorf("second esc: log focus %v, focused %q, owner %v", m.logFocus.active, m.focused, m.currentOwner())
	}

	// ,l pressed while the Log is focused opens the console as well, and q
	// closes it onto the Log as Esc does.
	m, _ = press(t, m, ",", "l")
	m, _ = press(t, m, ",", "l")
	if !m.console.open || m.logFocus.active {
		t.Errorf(",l while focused: console %v, log focus %v", m.console.open, m.logFocus.active)
	}
	m, _ = press(t, m, "q")
	if m.console.open || !m.logFocus.active {
		t.Error("q did not close the console onto the Log")
	}

	// q leaves the focus too.
	m, _ = press(t, m, "q")
	if m.logFocus.active || m.focused != PanelAspects {
		t.Error("q did not leave the Log focus")
	}
}

// l closes the console it opened, onto the focused Log, as Esc does: the key
// that opens the large log also closes it (operator, 2026-11-05).
func TestConsoleLToggles(t *testing.T) {
	m := logModel(t, 6)
	m.focused = PanelBodies
	m, _ = press(t, m, ",", "l", "l")
	if !m.console.open {
		t.Fatal(",l l did not open the console")
	}
	m, _ = press(t, m, "l")
	if m.console.open || !m.logFocus.active || m.currentOwner() != ownerLog {
		t.Errorf("l in the console: console %v, log focus %v, owner %v", m.console.open, m.logFocus.active, m.currentOwner())
	}
	m, _ = press(t, m, "l")
	if !m.console.open {
		t.Error("l on the focused Log did not open the console again")
	}
}

// A panel's digit in the console leaves the console and the Log's focus
// together for that panel. Every other key stays swallowed.
func TestConsoleDigitLeavesForThePanel(t *testing.T) {
	m := logModel(t, 6)
	m.focused = PanelChart
	m, _ = press(t, m, ",", "l", "l")
	if !m.console.open {
		t.Fatal(",l l did not open the console")
	}
	m, _ = press(t, m, "2")
	if m.console.open || m.logFocus.active || m.focused != PanelBodies || m.currentOwner() != ownerMain {
		t.Errorf("2: console %v, log focus %v, focused %q, owner %v", m.console.open, m.logFocus.active, m.focused, m.currentOwner())
	}

	m, _ = press(t, m, ",", "l", "l")
	target := m.targetFloatingTime
	m, _ = press(t, m, "d")
	if !m.console.open || !m.targetFloatingTime.Equal(target) {
		t.Errorf("d: console %v, day stepped %v, want it swallowed", m.console.open, !m.targetFloatingTime.Equal(target))
	}

	// A console opened with no Log panel on screen leaves the same way.
	m, _ = press(t, m, "esc", "esc")
	m.width, m.height = 80, 24
	m, _ = press(t, m, ",", "l")
	if !m.console.open {
		t.Fatal("80x24 ,l did not open the console")
	}
	m, _ = press(t, m, "3")
	if m.console.open || m.focused != PanelAspects {
		t.Errorf("3 at 80x24: console %v, focused %q", m.console.open, m.focused)
	}
}

// A console opened from the focused Log closes onto the panel when a resize
// took the Log panel away meanwhile.
func TestConsoleClosesWithoutTheLogPanel(t *testing.T) {
	m := logModel(t, 6)
	m.focused = PanelBodies
	m, _ = press(t, m, ",", "l", "l")
	m.width, m.height = 80, 24
	m, _ = press(t, m, "esc")
	if m.console.open || m.logFocus.active || m.focused != PanelBodies || m.currentOwner() != ownerMain {
		t.Errorf("esc: console %v, log focus %v, focused %q, owner %v", m.console.open, m.logFocus.active, m.focused, m.currentOwner())
	}
}

func TestLogFocusOtherKeyLeavesAndRuns(t *testing.T) {
	m := logModel(t, 6)
	m, _ = press(t, m, ",", "l", "2")
	if m.logFocus.active || m.focused != PanelBodies {
		t.Errorf("2: log focus %v, focused %q, want it gone and Bodies focused", m.logFocus.active, m.focused)
	}
	m, _ = press(t, m, ",", "l")
	target := m.targetFloatingTime
	m, _ = press(t, m, "d")
	if m.logFocus.active {
		t.Error("d left the Log focus on")
	}
	if m.targetFloatingTime.Equal(target) {
		t.Error("d did not step a day after leaving the Log focus")
	}
	m, _ = press(t, m, ",", "l", "tab")
	if m.logFocus.active || m.focused == PanelBodies {
		t.Errorf("tab: log focus %v, focused %q", m.logFocus.active, m.focused)
	}
}

func TestLeaderLOpensConsoleWithoutLogPanel(t *testing.T) {
	m := logModel(t, 4)
	m.width, m.height = 80, 24
	m, _ = press(t, m, ",", "l")
	if !m.console.open || m.logFocus.active {
		t.Errorf("80x24 ,l: console %v, log focus %v, want the console", m.console.open, m.logFocus.active)
	}
}

// A line arriving while the Log is focused keeps the cursor on its own line.
func TestLogFocusKeepsCursorOnArrival(t *testing.T) {
	captureYank(t)
	m := logModel(t, 9)
	m, _ = press(t, m, ",", "l", "up", "up")
	if m.logFocus.cursor != 6 {
		t.Fatalf("cursor = %d, want 6", m.logFocus.cursor)
	}
	m = m.logNote("test", "late")
	if m.logFocus.cursor != 6 {
		t.Errorf("cursor moved to %d on an arrival", m.logFocus.cursor)
	}
	m, _ = press(t, m, "enter")
}

// copyModel is a Chart, Bodies and Aspects with more to copy than their panels
// have room for, at a width where the Chart row has to drop its place.
func copyModel(t *testing.T, two bool) Model {
	t.Helper()
	m := logModel(t, 12)
	if two {
		m.slots = twoSlots(t, time.Date(2001, 7, 3, 4, 5, 0, 0, time.UTC), nameColLongPlace)
		firstAnchor(m.slots).Chart.Name = "Constructed One"
	}
	m.width, m.height = 100, 30
	m.chart.positions = []aspect.Position{
		{Object: objSun, Longitude: 16.7}, {Object: objMoon, Longitude: 249.25},
		{Object: objAscendant, Longitude: 210.8667, House: 1}, {Object: objMidheaven, Longitude: 124.57, House: 10},
	}
	for i := 0; i < 24; i++ {
		m.chart.aspects = append(m.chart.aspects, aspect.Aspect{AObject: objSun, BObject: objMoon, Kind: aspect.AspectKind(i % 4), Orb: aspect.Orb(float64(i) / 10), Applying: i%2 == 0, Power: float64(100 - i)})
	}
	return m
}

// TestCopyEveryPanel: `,y` copies the whole content of whatever has the
// keyboard, every row uncut and uncoloured, and says so on the note lane. The
// expected text is built from the rows' own formatters, never from the panel's
// layout, so a row the panel had no room for still has to be there.
func TestCopyEveryPanel(t *testing.T) {
	plain := Config{Accessible: true, Color: ColorNever}
	trimmed := func(rows ...string) string {
		for i, r := range rows {
			rows[i] = strings.TrimRight(r, " ")
		}
		return strings.Join(rows, "\n")
	}
	type tc struct {
		name  string
		focus PanelID
		keys  []string // everything but the final `y`
		want  func(Model) (string, int)
		note  string
	}
	cases := []tc{
		{"log focus", PanelChart, []string{",", "l", ","}, func(m Model) (string, int) {
			return strings.Join(m.logPanelLines(), "\n"), len(m.logPanelLines())
		}, "log: copied %d lines"},
		{"console", PanelChart, []string{",", "l", "l", ","}, func(m Model) (string, int) {
			var out []string
			for _, l := range m.console.snapshot {
				out = append(out, l.text)
			}
			return strings.Join(out, "\n"), len(out)
		}, "log: copied %d lines"},
		{"chart", PanelChart, []string{","}, func(m Model) (string, int) {
			var rows []string
			for i, s := range m.slots.All() {
				name := "~ Now"
				if !s.Floating {
					name = s.Chart.Name
				}
				// Both charts are selected, so each row opens with its numeral.
				rows = append(rows, fmt.Sprintf("%d %s %s %s %s WhTr", i+1, name, slotMoment(s.Time, s.Location), slotZone(s.Time, s.Location), s.Location.Display()))
			}
			return strings.Join(rows, "\n"), len(rows)
		}, "chart: copied %d rows"},
		{"bodies", PanelBodies, []string{","}, func(m Model) (string, int) {
			planets, under := panel.BodyRows(m.chart.positions, m.chart.bodyStyles, plain, panel.HouseOverlay{})
			var rows []string
			for _, r := range append(planets, under...) {
				rows = append(rows, strings.TrimRight(r.Text, " "))
			}
			return trimmed(rows...), len(rows)
		}, "bodies: copied %d rows"},
		{"aspects", PanelAspects, []string{","}, func(m Model) (string, int) {
			var rows []string
			for _, a := range m.chart.aspects {
				rows = append(rows, panel.FormatAspectEntryStyled(a, astrologstyle.AspectStyle{}, plain))
			}
			return trimmed(rows...), len(rows)
		}, "aspects: copied %d aspects"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			copied := captureYank(t)
			m := copyModel(t, c.name == "chart")
			m.focused = c.focus
			m, _ = press(t, m, c.keys...)
			want, n := c.want(m)
			if n < 2 {
				t.Fatalf("the fixture gives %d rows to copy, want several", n)
			}
			m, _ = press(t, m, "y")
			if len(*copied) != 1 {
				t.Fatalf("copied %d times, want once: %q", len(*copied), *copied)
			}
			got := (*copied)[0]
			if c.name == "chart" {
				// The Chart row pins its tag right: the words are the oracle, the
				// padding between them is the layout's.
				if !sameWords(got, want) {
					t.Errorf("copied\n%s\nwant the words of\n%s", got, want)
				}
			} else if got != want {
				t.Errorf("copied\n%s\nwant\n%s", got, want)
			}
			if strings.Contains(got, "\x1b") || strings.Contains(got, "…") {
				t.Errorf("an escape sequence or a cut mark reached the clipboard: %q", got)
			}
			for _, line := range strings.Split(got, "\n") {
				if line != strings.TrimRight(line, " ") {
					t.Errorf("trailing space kept in %q", line)
				}
			}
			note := fmt.Sprintf(c.note, n)
			if lines := logTexts(m); !strings.Contains(strings.Join(lines, "\n"), note) {
				t.Errorf("no Log note %q in %q", note, lines)
			}
		})
	}
}

// Nothing to copy says so and writes nothing; a failed copy names the panel.
func TestCopyNothingAndFailure(t *testing.T) {
	copied := captureYank(t)
	m := detailsTestModel(t)
	m.cfg = Config{Accessible: true}
	m.focused = PanelAspects
	m, _ = press(t, m, ",", "y")
	if len(*copied) != 0 || m.yank.notice != noticeNothingCopy {
		t.Errorf("empty Aspects: copied %q, notice %q", *copied, m.yank.notice)
	}
	yankLine = func(string) error { return fmt.Errorf("no clipboard") }
	m = copyModel(t, false)
	m.focused = PanelBodies
	m, _ = press(t, m, ",", "y")
	if got := strings.Join(logTexts(m), "\n"); !strings.Contains(got, "Couldn't copy the Bodies panel to the clipboard.") {
		t.Errorf("the failure line does not name the panel: %q", got)
	}
}

// Every default chord resolves inside attached Details. The Details map used
// to be the default map plus `y`; a duplicate there emptied its tree and killed
// every chord inside Details (leader.go).
func TestDetailsKeepsItsChords(t *testing.T) {
	copied := captureYank(t)
	m := onTimeline(t, Config{Accessible: true})
	m = m.logNote("test", "a line")
	if m.currentOwner() != ownerDetails {
		t.Fatalf("owner = %v, want attached Details", m.currentOwner())
	}
	after, _ := press(t, m, ",", "y")
	if len(*copied) != 1 {
		t.Errorf("attached Details ,y copied %d times, want once", len(*copied))
	}
	after, _ = press(t, m, ",", "a")
	if after.startup != startupDisplayOpen {
		t.Errorf("attached Details ,a: startup %v, want the House display open", after.startup)
	}
	after, _ = press(t, m, ",", "l")
	if !after.logFocus.active {
		t.Error("attached Details ,l did not focus the Log")
	}
	after, _ = press(t, after, "esc")
	if after.currentOwner() != ownerDetails {
		t.Errorf("leaving the Log focus returned to %v, want attached Details", after.currentOwner())
	}
}

// Comparing: a Bodies row copies with one column per shown chart, as shown.
func TestCopyBodiesComparing(t *testing.T) {
	copied := captureYank(t)
	m := columnsModel(t, Config{Color: ColorNever}, 1)
	m.focused = PanelBodies
	cols, ok := m.bodyColumnsForScene()
	if !ok || len(cols) != 4 {
		t.Fatalf("fixture: columns = %d (ok=%v), want 4", len(cols), ok)
	}
	m, _ = press(t, m, ",", "y")
	if len(*copied) != 1 {
		t.Fatalf("copied %d times, want once", len(*copied))
	}
	rows := strings.Split((*copied)[0], "\n")
	if want := panel.BodyRowCount(m.chart.positions, m.chart.cusps); len(rows) != want {
		t.Errorf("copied %d rows, want every one of the %d", len(rows), want)
	}
	for _, b := range []aspect.Object{objSun, objSaturn, objMidheaven} {
		var row string
		for _, r := range rows {
			if strings.HasPrefix(r, panel.PositionGlyph(positionOf(m.chart.positions, b))+" "+panel.BodyName(b)) {
				row = r
			}
		}
		if row == "" {
			t.Errorf("no row for %s in\n%s", panel.BodyName(b), strings.Join(rows, "\n"))
			continue
		}
		for i, c := range cols {
			sep := " "
			switch i {
			case 1:
				sep = "["
			case 2:
				sep = "]"
			}
			if want := expectedColumn(sep, positionOf(c.Positions, b)); !strings.Contains(row, want) {
				t.Errorf("%s: chart %d column %q missing from %q", panel.BodyName(b), i+1, want, row)
			}
		}
	}
	if !strings.Contains(strings.Join(logTexts(m), "\n"), fmt.Sprintf("bodies: copied %d rows", len(rows))) {
		t.Errorf("no Log note for %d rows", len(rows))
	}
}

// Copy in the Actions palette is greyed, with its reason, while the panel under
// the cursor has nothing to copy; it runs otherwise.
func TestCopyBlockedWhenNothing(t *testing.T) {
	m := detailsTestModel(t)
	m.cfg = Config{Accessible: true}
	m.focused = PanelAspects
	if got := m.blocked(keys.BindingCopy); got != whyNothingToCopy {
		t.Errorf("empty Aspects: blocked = %q, want %q", got, whyNothingToCopy)
	}
	m = copyModel(t, false)
	m.focused = PanelAspects
	if got := m.blocked(keys.BindingCopy); got != "" {
		t.Errorf("Aspects with aspects: blocked = %q, want it free", got)
	}
}

// logFocusFrameModel is the one-chart workspace at 132x40 with a Log holding
// more lines than its four rows, an error among them.
func logFocusFrameModel(t *testing.T, cfg Config) Model {
	t.Helper()
	m := detailsTestModel(t)
	m.cfg = cfg
	m.chart.positions = fullChartPositions()
	m.chart.aspects = tasteOwnAspects()
	m = m.logNote("session", "general session, 2 charts")
	m = m.logNote("location", "Berlin, DE")
	m = m.logError("Couldn't cast the chart. Press ,l l for what Astrolog ran.")
	m = m.logNote("preferences", "keys.leader %q ignored: one key only", ",,")
	m = m.logNote("chart", "copied 6 rows")
	m = m.logNote("aspects", "copied 14 aspects")
	return m
}

// The focused Log at 132x40: the cursor row, the window that holds it, the
// thumb and the count, and the prior panel drawn unfocused.
func TestLogFocusFrame(t *testing.T) {
	forceColor(t)
	for _, cfg := range tasteConfigs() {
		t.Run(cfg.name, func(t *testing.T) {
			m := logFocusFrameModel(t, cfg.cfg)
			m, _ = press(t, m, ",", "l", "up", "up")
			if !m.logFocus.active {
				t.Fatal("the Log is not focused")
			}
			writeTasteGolden(t, "log-focus-132x40-"+cfg.name, m.View())
		})
	}
}

// A layout that loses the Log panel while it is focused ends the focus: the
// next key runs on the screen, not on a panel nobody can see.
func TestLogFocusEndsWhenThePanelGoes(t *testing.T) {
	m := logModel(t, 5)
	m, _ = press(t, m, ",", "l")
	m.width, m.height = 80, 24
	target := m.targetFloatingTime
	m, _ = press(t, m, "d")
	if m.logFocus.active || m.targetFloatingTime.Equal(target) {
		t.Errorf("d with no Log panel: focus %v, stepped %v", m.logFocus.active, !m.targetFloatingTime.Equal(target))
	}
}

// TestCopyAspectsPair: on a pair tab the Aspects panel draws the cross
// waterfall, and `,y` copies every cross aspect of it, one per line, in the
// panel's own cell text and order, though the panel has room for only a few.
func TestCopyAspectsPair(t *testing.T) {
	copied := captureYank(t)
	plain := Config{Accessible: true, Color: ColorNever}
	m := tabsOnPair(t, Config{Accessible: true})
	m.width, m.height = 132, 40
	for i := 0; i < 24; i++ {
		m.chart.crossAspects = append(m.chart.crossAspects, lookup.OverlayAspect{
			TransitObject: objMoon, NatalObject: objSun, Kind: aspect.AspectKind(i % 4),
			Orb: aspect.Orb(float64(i) / 10), Applying: i%2 == 0, Power: float64(100 - i),
		})
	}
	var rows []string
	for _, a := range m.chart.crossAspects {
		rows = append(rows, strings.TrimRight(panel.FormatCrossAspectEntryStyled(a, astrologstyle.AspectStyle{}, plain), " "))
	}
	m, _ = press(t, m, ",", "y")
	want := strings.Join(rows, "\n")
	if len(*copied) != 1 || (*copied)[0] != want {
		t.Fatalf("copied %q\nwant\n%s", *copied, want)
	}
	if strings.Contains(want, "\x1b") {
		t.Error("oracle carries an escape")
	}
	if note := "aspects: copied 24 aspects"; !strings.Contains(strings.Join(logTexts(m), "\n"), note) {
		t.Errorf("no Log note %q in %q", note, logTexts(m))
	}
}
