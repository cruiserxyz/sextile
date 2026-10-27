package ui

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The verifiers C1-C23 its specification: every log
// line has a lane, commands are appended only at the two commit gates, the
// 6-row Log and the console each render their own lane set, only argv lines
// are capped, and the console renders the snapshot it took when it opened.

// lanesOf is the buffer's lanes in order.
func lanesOf(m Model) []logLane {
	out := make([]logLane, len(m.logLines))
	for i, l := range m.logLines {
		out[i] = l.lane
	}
	return out
}

// countLane is how many buffer lines sit on lane.
func countLane(m Model, lane logLane) int {
	n := 0
	for _, l := range m.logLines {
		if l.lane == lane {
			n++
		}
	}
	return n
}

// argvTexts is the argv lines' text, in order.
func argvTexts(m Model) []string {
	var out []string
	for _, l := range m.logLines {
		if l.lane == laneArgv {
			out = append(out, l.text)
		}
	}
	return out
}

// uiSources parses every non-test Go file of this package.
func uiSources(t *testing.T) (*token.FileSet, []*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", p, err)
		}
		files = append(files, f)
	}
	return fset, files
}

// callSites lists every call of a Model method named `method` in the
// package's non-test sources: the enclosing function, the file, and the first
// argument when it is a string literal.
type callSite struct {
	enclosing, file, literal string
	// format is the second argument when it is a string literal: a
	// logNote's format, its first being the scope.
	format string
}

// drawnAs is lines as the Log draws them with colours off: the mixed
// fixture's error lines behind `! `.
func drawnAs(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		if strings.HasPrefix(l, "error") {
			l = errorLinePrefix + l
		}
		// An argv line is drawn in its short form (argv_display.go).
		if strings.HasPrefix(l, "argv") {
			l = argvDisplay(l, 0, Config{})
		}
		out[i] = l
	}
	return out
}

func callSites(t *testing.T, method string) []callSite {
	t.Helper()
	fset, files := uiSources(t)
	var out []callSite
	for _, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != method {
					return true
				}
				site := callSite{enclosing: fn.Name.Name, file: filepath.Base(fset.Position(call.Pos()).Filename)}
				if len(call.Args) > 0 {
					if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						site.literal, _ = strconv.Unquote(lit.Value)
					}
				}
				if len(call.Args) > 1 {
					if lit, ok := call.Args[1].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						site.format, _ = strconv.Unquote(lit.Value)
					}
				}
				out = append(out, site)
				return true
			})
		}
	}
	return out
}

// C1: every log line carries exactly one lane, assigned by the writer verb
// and never inferred from the text. Statically, the buffer is appended in one
// place and that place is reached only through the three verbs; behaviourally,
// a note that reads like an error stays a note and a command that reads like
// a note stays argv.
func TestEveryLogLineHasALane(t *testing.T) {
	appends := 0
	fset, files := uiSources(t)
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for i, lhs := range as.Lhs {
				sel, ok := lhs.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "logLines" || i >= len(as.Rhs) {
					continue
				}
				if call, ok := as.Rhs[i].(*ast.CallExpr); ok {
					if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "append" {
						appends++
						if pos := fset.Position(call.Pos()); filepath.Base(pos.Filename) != "model.go" {
							t.Errorf("the buffer is appended outside model.go at %s", pos)
						}
					}
				}
			}
			return true
		})
	}
	if appends != 1 {
		t.Errorf("found %d appends to logLines, want exactly one (appendLog)", appends)
	}
	for _, site := range callSites(t, "appendLog") {
		switch site.enclosing {
		case "logNote", "logError", "logErrorLine", "logCommands", "logWarnings", "logWheelOpen", "logWheelClosed", "setNowLine":
		default:
			t.Errorf("appendLog is called from %s (%s), which is not a writer verb", site.enclosing, site.file)
		}
	}

	m := NewModelWithClock(Config{}, FrozenClock(testFrozenTime()))
	m = m.testNote("wheel: %v", errors.New("this text looks like an error but was written as a note"))
	m = m.testError("copy failed: %v", errors.New("no clipboard"))
	m = m.logCommands([]string{"active city: this text looks like a note but was written as a command"})
	if got, want := lanesOf(m), []logLane{laneNote, laneError, laneArgv}; !reflect.DeepEqual(got, want) {
		t.Errorf("lanes = %v, want %v: the lane is the verb's, never the text's", got, want)
	}
	for _, l := range m.logLines {
		if l.lane != laneNote && l.lane != laneError && l.lane != laneArgv {
			t.Errorf("line %q carries an unknown lane %d", l.text, l.lane)
		}
	}
}

// C2: a committed text-surface result appends one argv line per carried
// command, in carried order, at the commit gate — driven through the
// coordinator, so the gate is the real one.
func TestCommittedFrameLogsItsCommands(t *testing.T) {
	m := textModel(t)
	m, _ = m.advanceTime(unitDay, +1)
	st := m.refresh[surfaceText]
	before := len(m.logLines)
	commands := []string{"astrolog positions spawn", "astrolog aspects spawn"}
	var tm tea.Model = m
	tm, _ = tm.Update(FirstFrameReadyMsg{
		RunID: st.runningRunID, WantID: st.wantID,
		Positions: []aspect.Position{{Body: aspect.Sun}}, TransitTime: m.targetFloatingTime,
		Commands: commands,
	})
	m = tm.(Model)
	if len(m.chart.positions) != 1 {
		t.Fatal("the frame did not commit")
	}
	if got := m.logLines[before:]; len(got) != 2 || got[0] != (logLine{laneArgv, commands[0]}) || got[1] != (logLine{laneArgv, commands[1]}) {
		t.Errorf("appended %+v, want the two commands on the argv lane in carried order", got)
	}
}

// C3: a result the coordinator drops appends no argv line, on the text
// surface and on Details, at every gate that applies to each.
func TestDroppedResultLogsNoCommand(t *testing.T) {
	commands := []string{"a spawn that really ran"}
	boom := errors.New("boom")

	textCases := []struct {
		name string
		msg  func(m Model) (Model, FirstFrameReadyMsg)
	}{
		{"superseded runID", func(m Model) (Model, FirstFrameReadyMsg) {
			st := m.refresh[surfaceText]
			return m, FirstFrameReadyMsg{RunID: st.runningRunID + 1, WantID: st.wantID, Commands: commands}
		}},
		{"below the commit floor", func(m Model) (Model, FirstFrameReadyMsg) {
			st := m.refresh[surfaceText]
			m.refresh[surfaceText].commitFloor = st.wantID + 1
			return m, FirstFrameReadyMsg{RunID: st.runningRunID, WantID: st.wantID, Commands: commands}
		}},
		{"context canceled", func(m Model) (Model, FirstFrameReadyMsg) {
			st := m.refresh[surfaceText]
			return m, FirstFrameReadyMsg{RunID: st.runningRunID, WantID: st.wantID, Err: context.Canceled, Commands: commands}
		}},
		{"stale text error", func(m Model) (Model, FirstFrameReadyMsg) {
			m, _ = m.advanceTime(unitDay, +1) // a newer want, run 1 still in flight
			st := m.refresh[surfaceText]
			if st.runningWantID == st.wantID {
				t.Fatal("no newer want is pending, so this cannot be a stale error")
			}
			return m, FirstFrameReadyMsg{RunID: st.runningRunID, WantID: st.runningWantID, Err: boom, Commands: commands}
		}},
	}
	for _, tc := range textCases {
		t.Run("text/"+tc.name, func(t *testing.T) {
			m := textModel(t)
			m, _ = m.advanceTime(unitDay, +1)
			m, msg := tc.msg(m)
			before := len(m.logLines)
			var tm tea.Model = m
			tm, _ = tm.Update(msg)
			m = tm.(Model)
			if len(m.logLines) != before {
				t.Errorf("a dropped text result wrote %+v", m.logLines[before:])
			}
		})
	}

	detailsCases := []struct {
		name string
		msg  func(m Model) (Model, refreshResultMsg)
	}{
		{"superseded runID", func(m Model) (Model, refreshResultMsg) {
			st := m.refresh[surfaceDetails]
			return m, refreshResultMsg{surface: surfaceDetails, runID: st.runningRunID + 1, wantID: st.wantID, payload: detailsPayload{commands: commands}}
		}},
		{"below the commit floor", func(m Model) (Model, refreshResultMsg) {
			st := m.refresh[surfaceDetails]
			m.refresh[surfaceDetails].commitFloor = st.wantID + 1
			return m, refreshResultMsg{surface: surfaceDetails, runID: st.runningRunID, wantID: st.wantID, payload: detailsPayload{commands: commands}}
		}},
		{"context canceled", func(m Model) (Model, refreshResultMsg) {
			st := m.refresh[surfaceDetails]
			return m, refreshResultMsg{surface: surfaceDetails, runID: st.runningRunID, wantID: st.wantID, payload: detailsPayload{commands: commands}, err: context.Canceled}
		}},
		{"strict latest", func(m Model) (Model, refreshResultMsg) {
			st := m.refresh[surfaceDetails]
			m.refresh[surfaceDetails].wantID++ // a newer want than the run's
			return m, refreshResultMsg{surface: surfaceDetails, runID: st.runningRunID, wantID: st.wantID, payload: detailsPayload{commands: commands}}
		}},
	}
	for _, tc := range detailsCases {
		t.Run("details/"+tc.name, func(t *testing.T) {
			m := detailsTestModel(t)
			m, cmd := m.startDetailsNow()
			if cmd == nil {
				t.Fatal("no Details run started")
			}
			m, msg := tc.msg(m)
			before := len(m.logLines)
			m, _ = m.onRefreshResult(msg)
			if len(m.logLines) != before {
				t.Errorf("a dropped Details result wrote %+v", m.logLines[before:])
			}
		})
	}
}

// C4: an error frame still logs its commands — beside its error line.
func TestErrorFrameStillLogsItsCommands(t *testing.T) {
	m := textModel(t)
	before := len(m.logLines)
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{Err: errors.New("astrolog exited 1"), Commands: []string{"the spawn that failed"}})
	got := m.logLines[before:]
	if len(got) != 2 {
		t.Fatalf("appended %+v, want the command and the error", got)
	}
	if got[0] != (logLine{laneArgv, "the spawn that failed"}) {
		t.Errorf("first line = %+v, want the failed command on the argv lane", got[0])
	}
	// The text is the sentence an earlier change writes, not the wrapped cause: the cause
	// is the argv line above it, which is what an operator diagnosing this
	// actually reads.
	if got[1].lane != laneError || !strings.HasPrefix(got[1].text, "Couldn't cast the chart.") {
		t.Errorf("second line = %+v, want the failure on the error lane", got[1])
	}
}

// C5: a spawned Details result accepted at commitRefresh(surfaceDetails)
// logs exactly one argv line, on the success and the accepted-error branch.
func TestDetailsCommitLogsItsCommand(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{{"success", nil}, {"accepted error", errors.New("astrolog exited 1")}} {
		t.Run(tc.name, func(t *testing.T) {
			m := detailsTestModel(t)
			before := len(m.logLines)
			m, _ = m.commitRefresh(surfaceDetails, refreshResultMsg{
				surface: surfaceDetails,
				runID:   m.refresh[surfaceDetails].runningRunID,
				wantID:  m.refresh[surfaceDetails].wantID,
				payload: detailsPayload{title: "[0] Chart", body: "report\n", commands: []string{"the report spawn"}},
				err:     tc.err,
			})
			got := m.logLines[before:]
			if len(got) == 0 || got[0] != (logLine{laneArgv, "the report spawn"}) {
				t.Fatalf("appended %+v, want the report's command first", got)
			}
			if tc.err == nil && len(got) != 1 {
				t.Errorf("a successful report appended %+v, want the command alone", got)
			}
			if tc.err != nil && (len(got) != 2 || got[1].lane != laneError || !strings.HasPrefix(got[1].text, "Couldn't read the full report.")) {
				t.Errorf("a failed report appended %+v, want the command beside its failure line", got)
			}
		})
	}
}

// C6: Details filled from the frame in hand performs no spawn and logs none.
func TestAspectsDetailsLogsNoCommand(t *testing.T) {
	m := detailsTestModel(t)
	m.focused = PanelAspects
	m.chart.aspectsReport = "the frame's own -aa report\n"
	before := len(m.logLines)
	m = m.applyAspectsDetails()
	if m.details.body != m.chart.aspectsReport {
		t.Fatal("Details was not filled from the frame")
	}
	if len(m.logLines) != before {
		t.Errorf("filling Details from the frame wrote %+v", m.logLines[before:])
	}
}

// mixedLogModel is a ready, accessible 132x40 pair workspace whose buffer
// interleaves the three lanes.
func mixedLogModel(t *testing.T) Model {
	t.Helper()
	m := tabsPairModel(t, Config{Accessible: true})
	m.width, m.height = 144, 40
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{Positions: fullChartPositions()})
	m = m.testNote("note one")
	m = m.logCommands([]string{"argv one"})
	m = m.testError("error one")
	m = m.testNote("note two")
	m = m.logCommands([]string{"argv two", "argv three"})
	m = m.testError("error two")
	m = m.testNote("note three")
	return m
}

// logPanelRows is what the closed 6-row Log actually rendered, chrome stripped.
func logPanelRows(t *testing.T, m Model) []string {
	t.Helper()
	logR, ok := m.geometry()[PanelLog]
	if !ok {
		t.Fatal("no Log panel in this layout")
	}
	rows := frameRows(m.View())
	var out []string
	for y := logR.y + 1; y < logR.y+logR.h-1; y++ {
		out = append(out, strings.TrimRight(ansi.Strip(ansi.Cut(rows[y], logR.x+1, logR.x+logR.w-1)), " "))
	}
	return out
}

// C7: the 6-row Log renders error and note lines in arrival order and no
// argv line.
func TestSixRowLogHidesArgv(t *testing.T) {
	m := mixedLogModel(t)
	want := []string{"note one", "error one", "note two", "error two", "note three"}
	if got := m.logPanelLines(); !reflect.DeepEqual(got, want) {
		t.Errorf("logPanelLines = %q, want %q", got, want)
	}
	rows := logPanelRows(t, m)
	joined := strings.Join(rows, "\n")
	for _, argv := range []string{"argv one", "argv two", "argv three"} {
		if strings.Contains(joined, argv) {
			t.Errorf("the 6-row Log rendered %q", argv)
		}
	}
	var shown []string
	for _, r := range rows {
		if r != "" {
			shown = append(shown, r)
		}
	}
	// padBody's tail-clipping autoscroll is untouched: the panel shows the
	// newest lines that fit its four inner rows, in arrival order. With
	// colours off an error is drawn behind `! ` (the specification A2).
	if tail := drawnAs(want[len(want)-len(rows):]); !reflect.DeepEqual(shown, tail) {
		t.Errorf("the 6-row Log rendered %q, want %q", shown, tail)
	}
}

// C8: the console's snapshot holds the argv and error lines present at open,
// in arrival order, together with every note; while open it renders that
// snapshot. an earlier change added the notes: they had no surface at 80x24 at all.
func TestConsoleShowsEveryLane(t *testing.T) {
	m, _ := press(t, mixedLogModel(t), ",", "l")
	want := []string{
		"note one", "argv one", "error one", "note two",
		"argv two", "argv three", "error two", "note three",
	}
	if got := snapshotTexts(m); !reflect.DeepEqual(got, want) {
		t.Errorf("snapshot = %q, want %q", got, want)
	}
	var shown []string
	for _, r := range consoleBodyLines(t, m) {
		// With colours off every row opens on the cursor column.
		if r = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(r), ">")); r != "" {
			shown = append(shown, r)
		}
	}
	if !reflect.DeepEqual(shown, drawnAs(want)) {
		t.Errorf("the console rendered %q, want %q", shown, drawnAs(want))
	}
	// The console is the COMPLETE record since an earlier change: a note that has scrolled
	// out of the 6-row Log is readable here, and at 80x24 — where there is no
	// Log panel at all — this is the only surface it ever had.
	if !strings.Contains(ansi.Strip(m.View()), "note one") {
		t.Error("a note did not reach the console frame")
	}
}

// C9: the MaxLogLines cap applies to argv lines alone.
func TestOnlyArgvLinesAreCapped(t *testing.T) {
	m := NewModelWithClock(Config{}, FrozenClock(testFrozenTime()))
	m = m.testNote("note before")
	m = m.testError("error before")
	for i := 0; i < MaxLogLines+50; i++ {
		m = m.logCommands([]string{fmt.Sprintf("argv %03d", i)})
	}
	m = m.testNote("note after")
	m = m.testError("error after")
	if got := countLane(m, laneArgv); got != MaxLogLines {
		t.Errorf("%d argv lines survive, want %d", got, MaxLogLines)
	}
	if got := argvTexts(m); got[0] != "argv 050" || got[len(got)-1] != fmt.Sprintf("argv %03d", MaxLogLines+49) {
		t.Errorf("argv lines run %q..%q, want the fifty oldest dropped", got[0], got[len(got)-1])
	}
	if countLane(m, laneNote) != 2 || countLane(m, laneError) != 2 {
		t.Errorf("notes=%d errors=%d, want both untouched at 2", countLane(m, laneNote), countLane(m, laneError))
	}
	if got := m.logPanelLines(); !reflect.DeepEqual(got, []string{"note before", "error before", "note after", "error after"}) {
		t.Errorf("the Log's view after the trim = %q", got)
	}
}

// C10: error and note lines survive for the life of the process, however
// many argv lines arrive.
func TestErrorsAndNotesAreNeverEvicted(t *testing.T) {
	m := NewModelWithClock(Config{}, FrozenClock(testFrozenTime()))
	for i := 0; i < 3; i++ {
		m = m.testNote("note %d", i)
		m = m.testError("error %d", i)
	}
	for i := 0; i < 5*MaxLogLines; i++ {
		m = m.logCommands([]string{"argv"})
	}
	want := []string{"note 0", "error 0", "note 1", "error 1", "note 2", "error 2"}
	if got := m.logPanelLines(); !reflect.DeepEqual(got, want) {
		t.Errorf("after %d commands the Log's view is %q, want %q", 5*MaxLogLines, got, want)
	}
	if got := len(m.logLines); got != MaxLogLines+6 {
		t.Errorf("buffer holds %d lines, want %d argv + 6", got, MaxLogLines+6)
	}
}

// C11: the error lane is written by a closed set of failure sites, told apart
// by call site and static prefix — never by text. The publish-success
// `wheel: ` line stays a note. The set is pinned by NAME, never by a count:
// an earlier change pinned five, an earlier change added the chart library's missing atlas, an earlier change
// added a failed chart resolve (its C15) and an earlier change added a resolve that
// SUCCEEDED into a chart that cannot anchor. Each row below is a site somebody
// chose; a new one is a deliberate line in this table.
func TestErrorLaneMembership(t *testing.T) {
	// Since an earlier change every one of these is a SENTENCE, not a dump: the literal
	// carries the subject and a %s takes the translated cause. The two "%s"
	// sites are the whole-line constants — the startup probe and the missing
	// place list — which have one cause each and nothing to translate.
	want := map[string]int{
		"Couldn't draw the wheel. %s":                                            1, // render failure, refresh.go
		"Couldn't show the wheel. %s":                                            1, // publish failure, refresh.go
		"Couldn't read the full report. %s":                                      1,
		"Couldn't copy that line to the clipboard.":                              1,
		"Couldn't cast the chart. %s":                                            1,
		"Couldn't cast %s. %s":                                                   1,
		"Couldn't open that chart. %s":                                           1, // requirement C15: a picked chart's resolve failed
		"Couldn't anchor that chart. It has %s.":                                 1, // an earlier change: it read fine and still cannot be cast
		"Couldn't restore the last session. Its file cannot be used.":            1, // the specification C16
		"Couldn't keep the refused session file: %s":                             1, // the specification C4: the launch could not move the refused file aside
		"Couldn't restore %d of the open charts.":                                1, // the specification C17: restore dropped entries that could not stand alone
		"Couldn't keep %d of the selections. At most six slots can be selected.": 1, // the specification C17a
		"Couldn't title that chart. %s":                                          2, // the specification § Failure modes: a retitle write failed, open or late (design-messages A8)
		// The two whole-line errors with one cause each (design-messages
		// § Message inventory): the startup probe and the missing atlas.
		"Couldn't find the Astrolog program. Install it and restart.": 1,
		"Couldn't find atlas.as. Put it beside Astrolog and restart.": 1,
		// A save that failed (design-messages § Late work): open, and late
		// after Esc, where a refusal of the name has no form left to sit in.
		"Couldn't save %q. %s":                                  2,
		"Couldn't save %q. That name is taken.":                 1,
		"Couldn't save %q. That name has no letters or digits.": 1,
		// the specification: a timeline search failed or was
		// refused (C41), and the whole-panel `yy` copy failed (C22).
		// the specification C8, C9: the save a house (or zodiac) choice makes failed, and
		// the put-back of the session after a failed rename failed.
		"Couldn't save the %s choice. It will be tried again at quit.":           1, // house or zodiac: chooseFrame, the one door of both (the specification C7)
		"Couldn't restore the session after the failed rename.":                  1,
		"Couldn't rename that chart. Its house system could not be saved first.": 1,
		// A second window cannot keep a renamed chart's house system.
		"Couldn't rename that chart. Its house system can only be kept by the first Sextile window.": 1,
		"Couldn't read the timeline. %s": 1,
		// A body template tab's facts (the specification P5).
		"Couldn't compute the %s tab. %s":                   1,
		"Couldn't copy the Details panel to the clipboard.": 1,
		// Save as and opening a named session save the session the window ran
		// first; a failure of that write is said (sessions_popup.go).
		"Couldn't save the session this window ran. Its file could not be written.": 2,
	}
	got := map[string]int{}
	for _, site := range callSites(t, "logError") {
		if site.literal == "" {
			t.Errorf("logError in %s (%s) has no literal prefix", site.enclosing, site.file)
			continue
		}
		got[site.literal]++
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("error-lane writers = %v, want exactly %v", got, want)
	}
	// The wheel's address is a note when the browser did not open (design-messages
	// § Log scope). A wheel that did open names it once on its own lane (W1).
	addressIsNote := false
	for _, site := range callSites(t, "logNote") {
		if site.literal == "wheel" && site.format == "open it by hand at %s" && site.enclosing == "onWheelPublished" {
			addressIsNote = true
		}
	}
	if !addressIsNote {
		t.Error("the wheel's address is not written as a note in onWheelPublished")
	}
	// And the two wheel sites are distinct call sites in distinct functions.
	// They now differ in text as well, which is a consequence of saying what
	// failed rather than naming the surface — a render that never happened and
	// an image that could not be shown are different things to the operator —
	// but the lane still follows the call site, never the text.
	wheel := map[string]string{}
	for _, site := range callSites(t, "logError") {
		if strings.HasPrefix(site.literal, "Couldn't draw the wheel") ||
			strings.HasPrefix(site.literal, "Couldn't show the wheel") {
			wheel[site.enclosing] = site.literal
		}
	}
	if len(wheel) != 2 {
		t.Errorf("the two wheel sites sit in %v, want two distinct functions", wheel)
	}
	checkErrorLineMembership(t)
}

// consoleFrame is everything the console shows that a snapshot could move:
// cursor, rendered rows, and the indicator's totals.
type consoleFrame struct {
	cursor  int
	rows    []string
	view    viewport
	snapped []string
}

func captureConsole(t *testing.T, m Model) consoleFrame {
	t.Helper()
	return consoleFrame{
		cursor:  m.console.cursor,
		rows:    consoleBodyLines(t, m),
		view:    consoleViewport(len(m.console.snapshot), m.console, m.consoleInnerHeight()),
		snapped: snapshotTexts(m),
	}
}

// C12: the console renders the snapshot it took at open; arrivals in any
// lane and evictions change nothing; reopening takes a fresh snapshot.
func TestConsoleSnapshotIsInert(t *testing.T) {
	m := openedConsole(t, MaxLogLines)
	m, _ = press(t, m, "up", "up", "up")
	before := captureConsole(t, m)

	m = m.testNote("a note arriving")
	m = m.testError("an error arriving")
	for i := 0; i < 20; i++ {
		m = m.logCommands([]string{fmt.Sprintf("an evicting command %d", i)})
	}
	if got := captureConsole(t, m); !reflect.DeepEqual(before, got) {
		t.Errorf("the open console changed:\n before %+v\n  after %+v", before, got)
	}

	m, _ = press(t, m, "esc", ",", "l")
	fresh := snapshotTexts(m)
	if len(fresh) != MaxLogLines+2 {
		t.Fatalf("reopened over %d lines, want %d argv + 1 note + 1 error", len(fresh), MaxLogLines+2)
	}
	if fresh[len(fresh)-1] != "an evicting command 19" || fresh[0] != "command 20" {
		t.Errorf("the fresh snapshot runs %q..%q", fresh[0], fresh[len(fresh)-1])
	}
	if got := m.console.cursor; got != len(fresh)-1 {
		t.Errorf("reopened cursor = %d, want the newest %d", got, len(fresh)-1)
	}
}

// C13: cursor, window, indicator and Enter address the snapshot, never the
// live buffer — built so the two index spaces disagree.
func TestConsoleCursorAddressesSnapshot(t *testing.T) {
	last, _, restore := stubYankRecording(t, nil)
	defer restore()
	m := detailsTestModel(t)
	m.cfg = Config{Accessible: true}
	m = m.testNote("note 0")
	m = m.logCommands([]string{"argv A"})
	m = m.testNote("note 1")
	m = m.testError("error B")
	m = m.testNote("note 2")
	m = m.logCommands([]string{"argv C"})
	m = m.testNote("note 3")
	m, _ = press(t, m, ",", "l")
	want := []string{"note 0", "argv A", "note 1", "error B", "note 2", "argv C", "note 3"}
	if got := snapshotTexts(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot = %q, want %q", got, want)
	}
	// The two index spaces are made to disagree by TIME rather than by lane
	// since an earlier change, which admits every lane: lines arriving after the open
	// extend the buffer and never the snapshot, so a cursor that had drifted
	// onto the buffer would address the wrong line from here on.
	for i := 0; i < 4; i++ {
		m = m.testNote("arrived after the open %d", i)
	}
	if len(m.logLines) == len(m.console.snapshot) {
		t.Fatal("the fixture no longer makes the two index spaces disagree")
	}
	if m.console.cursor != 6 {
		t.Errorf("cursor = %d, want 6, the newest snapshot index (the buffer now runs to %d)",
			m.console.cursor, len(m.logLines)-1)
	}
	m, _ = press(t, m, "up", "up", "up")
	if m.console.cursor != 3 || m.console.snapshot[m.console.cursor].text != "error B" {
		t.Errorf("after up: cursor %d on %q, want 3 on error B", m.console.cursor, m.console.snapshot[m.console.cursor].text)
	}
	m, _ = press(t, m, "enter")
	if *last != "error B" {
		t.Errorf("Enter copied %q, want the snapshot line under the cursor", *last)
	}
	// A buffer append while open changes no index the console uses.
	m = m.logCommands([]string{"argv D"})
	if v := consoleViewport(len(m.console.snapshot), m.console, m.consoleInnerHeight()); v.Total != 7 {
		t.Errorf("indicator total = %d, want the snapshot's 7", v.Total)
	}
	if start, end := consoleWindow(len(m.console.snapshot), m.console.offset, m.consoleInnerHeight()); start != 0 || end != 7 {
		t.Errorf("window [%d,%d), want [0,7) over the snapshot", start, end)
	}
}

// C14: open on the newest snapshot line; ↑/k and ↓/j step within the
// snapshot, clamped at its ends. Since an earlier change the snapshot admits every lane,
// so the counts here are the whole buffer at open rather than the argv subset.
func TestConsoleOpenAndMoveOverSnapshot(t *testing.T) {
	build := func() Model {
		m := detailsTestModel(t)
		m.cfg = Config{Accessible: true}
		for i := 0; i < 10; i++ {
			m = m.testNote("note %d", i)
			m = m.logCommands([]string{fmt.Sprintf("argv %d", i)})
		}
		return m
	}
	m, _ := press(t, build(), ",", "l")
	if len(m.console.snapshot) != 20 || len(m.logLines) != 20 {
		t.Fatalf("snapshot %d over buffer %d, want 20 over 20", len(m.console.snapshot), len(m.logLines))
	}
	if m.console.cursor != 19 {
		t.Errorf("opened on %d, want 19 (the newest snapshot line)", m.console.cursor)
	}
	for _, k := range []string{"up", "k"} {
		m, _ = press(t, build(), ",", "l")
		m, _ = press(t, m, k)
		if m.console.cursor != 18 {
			t.Errorf("%s moved to %d, want 18", k, m.console.cursor)
		}
		for i := 0; i < 30; i++ {
			m, _ = press(t, m, k)
		}
		if m.console.cursor != 0 {
			t.Errorf("%s did not clamp at 0: %d", k, m.console.cursor)
		}
	}
	for _, k := range []string{"down", "j"} {
		m, _ = press(t, build(), ",", "l")
		m, _ = press(t, m, "up", "up", "up")
		m, _ = press(t, m, k)
		if m.console.cursor != 17 {
			t.Errorf("%s moved to %d, want 17", k, m.console.cursor)
		}
		for i := 0; i < 30; i++ {
			m, _ = press(t, m, k)
		}
		if m.console.cursor != 19 {
			t.Errorf("%s did not clamp at 19: %d", k, m.console.cursor)
		}
	}
}

// C15: the rendered window always contains the cursor, over the snapshot.
func TestConsoleWindowFollowsCursorFiltered(t *testing.T) {
	m := detailsTestModel(t)
	m.cfg = Config{Accessible: true}
	for i := 0; i < 120; i++ {
		m = m.testNote("note %d", i)
		m = m.logCommands([]string{fmt.Sprintf("argv %d", i)})
	}
	m, _ = press(t, m, ",", "l")
	innerH := m.consoleInnerHeight()
	check := func(where string) {
		t.Helper()
		start, end := consoleWindow(len(m.console.snapshot), m.console.offset, innerH)
		if m.console.cursor < start || m.console.cursor >= end {
			t.Errorf("%s: cursor %d outside the window [%d,%d)", where, m.console.cursor, start, end)
		}
		rows := consoleBodyLines(t, m)
		want := m.console.snapshot[m.console.cursor].text
		if m.console.snapshot[m.console.cursor].lane == laneArgv {
			want = argvDisplay(want, 0, Config{})
		}
		if !strings.Contains(strings.Join(rows, "\n"), want) {
			t.Errorf("%s: the cursor line %q is not rendered", where, want)
		}
	}
	check("at open")
	for i := 0; i < 119; i++ {
		m, _ = press(t, m, "up")
		check("walking up")
	}
	for i := 0; i < 119; i++ {
		m, _ = press(t, m, "down")
		check("walking down")
	}
}

// C16: Enter on an argv line hands the whole stored command to the clipboard,
// including the part truncated at the border, with no chrome — over a buffer
// that interleaves notes among the argv and error lines.
func TestYankCopiesWholeCommand(t *testing.T) {
	last, _, restore := stubYankRecording(t, nil)
	defer restore()
	long := "/usr/local/bin/astrolog -qj 2461156.000000 -zl 13:25E 52:31N -c 14 -Yn -R 11 -R 17 -R 18 -R 25 -R 28 -v0 -aa -k " + strings.Repeat("-R 30 ", 20)
	m := detailsTestModel(t)
	m.cfg = Config{Accessible: true}
	m = m.testNote("note before")
	m = m.logCommands([]string{"short command"})
	m = m.testNote("note between")
	m = m.testError("error between")
	m = m.logCommands([]string{long})
	m = m.testNote("note after")
	m, _ = press(t, m, ",", "l")
	// The newest snapshot line is `note after` since an earlier change admitted the note
	// lane, so step up onto the command. That the step is needed at all is the
	// console carrying notes, which is the point of that ticket.
	m, _ = press(t, m, "up")
	m, _ = press(t, m, "enter")
	if *last != long {
		t.Errorf("copied %d chars, want the stored %d", len(*last), len(long))
	}
	for _, chrome := range []string{"|", "+", "\x1b", "\n"} {
		if strings.Contains(*last, chrome) {
			t.Errorf("the copied text carries chrome %q", chrome)
		}
	}
	rows := consoleBodyLines(t, m)
	if r, ok := m.consoleRect(); !ok || len(long) <= r.w-2 {
		t.Fatal("the command is not wider than the console, so untruncated copy proves nothing")
	}
	if got := rows[len(rows)-1]; len(got) >= len(long) {
		t.Error("the rendered row was not truncated")
	}
}

// C17: Enter on an error line copies the whole stored error text, unmarked.
func TestYankCopiesErrorLine(t *testing.T) {
	last, _, restore := stubYankRecording(t, nil)
	defer restore()
	for _, cfg := range []Config{{Accessible: true}, {Accessible: true, Color: ColorNever}, {Color: ColorAlways}} {
		m := detailsTestModel(t)
		m.cfg = cfg
		m = m.logCommands([]string{"argv before"})
		m = m.testError("details: astrolog exited 1: %s", strings.Repeat("a long explanation ", 8))
		want := m.logLines[len(m.logLines)-1].text
		m, _ = press(t, m, ",", "l")
		m, _ = press(t, m, "enter")
		if *last != want {
			t.Errorf("%+v: copied %q, want the stored error %q", cfg, *last, want)
		}
		for _, chrome := range []string{"|", "+", "\x1b", errorLinePrefix} {
			if strings.Contains(*last, chrome) {
				t.Errorf("%+v: the copied error carries %q", cfg, chrome)
			}
		}
	}
}

// C18: an empty snapshot opens with no cursor row and no indicator, Enter
// does nothing, and it stays that way — whatever arrives — until reopened.
//
// Emptiness used to be produced by filling the buffer with notes and letting
// the console filter them all out. an earlier change admits every lane, so that no longer
// empties anything — and the contract this pins was never about notes, it was
// about a console opened over nothing at all. The fixture is now a buffer with
// nothing in it, which is the only way the case still arises (a launch that has
// said nothing yet).
func TestConsoleEmptyFilteredList(t *testing.T) {
	calls, restore := stubYank(t, nil)
	defer restore()
	m := detailsTestModel(t)
	m.cfg = Config{Accessible: true}
	if len(m.logLines) != 0 {
		t.Fatalf("the fixture starts with %d lines, want an empty buffer", len(m.logLines))
	}
	m, _ = press(t, m, ",", "l")
	if !m.console.open || len(m.console.snapshot) != 0 {
		t.Fatalf("open=%v snapshot=%d, want an open console over an empty snapshot", m.console.open, len(m.console.snapshot))
	}
	assertEmpty := func(where string) {
		t.Helper()
		if v := consoleViewport(len(m.console.snapshot), m.console, m.consoleInnerHeight()); v.overflows() {
			t.Errorf("%s: an empty snapshot drew an indicator", where)
		}
		for i, row := range consoleBodyLines(t, m) {
			if want := map[bool]string{true: consoleEmptyLine, false: ""}[i == 0]; row != want {
				t.Errorf("%s: empty console row %d = %q, want %q", where, i, row, want)
			}
		}
		m, _ = press(t, m, "enter")
		if *calls != 0 || m.footerIndicator() != noticeNothingCopy {
			t.Errorf("%s: enter over an empty snapshot reached the clipboard or showed %q", where, m.footerIndicator())
		}
	}
	assertEmpty("at open")
	m = m.logCommands([]string{"a command arriving while open"})
	m = m.testError("an error arriving while open")
	assertEmpty("after arrivals")

	m, _ = press(t, m, "esc", ",", "l")
	if got := snapshotTexts(m); !reflect.DeepEqual(got, []string{"a command arriving while open", "an error arriving while open"}) {
		t.Errorf("reopened snapshot = %q", got)
	}
}

// C21: a committed result with no commands logs nothing and commits normally.
func TestCommitWithNoCommandsLogsNothing(t *testing.T) {
	m := textModel(t)
	before := len(m.logLines)
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{Positions: []aspect.Position{{Body: aspect.Sun}}, TransitTime: m.targetFloatingTime})
	if len(m.logLines) != before {
		t.Errorf("a frame with no commands wrote %+v", m.logLines[before:])
	}
	if m.startup != startupReady || len(m.chart.positions) != 1 {
		t.Error("the frame did not commit normally")
	}
}

// C22: a burst of commits while the console is open appends every command in
// commit order — none coalesced, none reordered — evicting past MaxLogLines,
// and the console shows exactly its snapshot throughout.
func TestBurstLeavesConsoleSnapshotInert(t *testing.T) {
	m := openedConsole(t, 30)
	m, _ = press(t, m, "up", "up")
	before := captureConsole(t, m)
	frames := MaxLogLines/2 + 10 // two commands each: exceeds the cap
	var want []string
	for i := 0; i < frames; i++ {
		cmds := []string{fmt.Sprintf("frame %03d positions", i), fmt.Sprintf("frame %03d aspects", i)}
		want = append(want, cmds...)
		m, _ = applyTextCommit(m, FirstFrameReadyMsg{Positions: fullChartPositions(), Commands: cmds})
		if i%50 == 0 {
			if got := captureConsole(t, m); !reflect.DeepEqual(before, got) {
				t.Fatalf("after frame %d the open console changed", i)
			}
		}
	}
	if got := captureConsole(t, m); !reflect.DeepEqual(before, got) {
		t.Error("the open console changed under the burst")
	}
	got := argvTexts(m)
	if len(got) != MaxLogLines {
		t.Fatalf("%d argv lines survive, want %d", len(got), MaxLogLines)
	}
	if !reflect.DeepEqual(got, want[len(want)-MaxLogLines:]) {
		t.Errorf("surviving commands are not the newest %d in commit order", MaxLogLines)
	}
}

// C23: an error line enters the buffer once and reaches both views from that
// one entry.
func TestErrorAppearsInBothViewsOnce(t *testing.T) {
	m := detailsTestModel(t)
	m.cfg = Config{Accessible: true}
	before := len(m.logLines)
	m, _ = m.commitRefresh(surfaceDetails, refreshResultMsg{surface: surfaceDetails, err: errors.New("astrolog exploded")})
	if got := m.logLines[before:]; len(got) != 1 || got[0].lane != laneError {
		t.Fatalf("the failure wrote %+v, want one error line", got)
	}
	count := func(lines []string) int {
		n := 0
		for _, l := range lines {
			if strings.Contains(l, "Couldn't read the full report.") {
				n++
			}
		}
		return n
	}
	if n := count(m.logPanelLines()); n != 1 {
		t.Errorf("the 6-row Log holds the error %d times, want once", n)
	}
	m, _ = press(t, m, ",", "l")
	if n := count(snapshotTexts(m)); n != 1 {
		t.Errorf("the console snapshot holds the error %d times, want once", n)
	}
	if n := count(logTexts(m)); n != 1 {
		t.Errorf("the buffer holds the error %d times, want once", n)
	}
}

// The taste fixture must not be stale: every command in it is one the real
// compiler shape produces, and the file has the forty lines V1 renders.
func TestSweepFixtureIsWellFormed(t *testing.T) {
	for i, c := range sweepCommands(t) {
		if !strings.Contains(c, "astrolog") || !strings.Contains(c, " -qj ") {
			t.Errorf("line %d does not look like a recorded Astrolog command: %q", i, c)
		}
	}
	if _, err := os.Stat(filepath.Join("testdata", consoleArgvSweepFixture)); err != nil {
		t.Fatal(err)
	}
}

// at 80x24 there is no Log panel, so before this ticket a note line had
// nowhere to be read at all — not truncated, absent. Driving the real binary at
// that size found zero occurrences of `session:`, `active city:` or `location:`
// anywhere in a whole capture.
//
// The test says the whole story in one place: the ordinary 80x24 frame draws no
// Log panel and shows the note nowhere, and the `,l` console then shows it. The
// first half is what makes the second half necessary, so they are pinned
// together — a later layout change that gave 80x24 a Log panel should make
// somebody re-read this, not silently pass.
//
// Positive control: restore the `l.lane != laneNote` filter in consoleLines and
// this fails by name.
func TestANoteIsReachableAtTheSmallerWidth(t *testing.T) {
	const note = "session: restored Rosa Lang from snapshot"
	m := detailsTestModel(t)
	m.cfg = Config{Accessible: true}
	sized, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = sized.(Model)
	m = m.testNote("%s", note)
	m = m.logCommands([]string{"/usr/local/bin/astrolog -qj 2461156.000000"})

	ordinary := ansi.Strip(m.View())
	if strings.Contains(ordinary, "-Log-") {
		t.Fatal("80x24 now draws a Log panel; this test's premise has changed")
	}
	if strings.Contains(ordinary, note) {
		t.Fatalf("the note is already on the ordinary frame:\n%s", ordinary)
	}

	opened, _ := press(t, m, ",", "l")
	if !opened.console.open {
		t.Fatal("the console did not open")
	}
	if got := ansi.Strip(opened.View()); !strings.Contains(got, note) {
		t.Errorf("the console does not reach the note at 80x24; it renders as:\n%s", got)
	}
}

// logErrorLine is the error lane for a sentence the caller words in full. It
// has exactly the call sites below, by function name, and it adds nothing
// to what it is given: no "Couldn't" form, no full stop.
func checkErrorLineMembership(t *testing.T) {
	t.Helper()
	got := map[string]int{}
	for _, site := range callSites(t, "logErrorLine") {
		got[site.enclosing]++
	}
	want := map[string]int{"saveDefaultCity": 2, "applyDefaultCitySaved": 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("logErrorLine is called from %v, want exactly %v", got, want)
	}
	m := NewModelWithClock(Config{}, FrozenClock(testFrozenTime())).logErrorLine("default city not saved: x")
	if l := lastLine(m); l.lane != laneError || l.text != "default city not saved: x" {
		t.Errorf("line = %q on lane %d, want the text as given on the error lane", l.text, l.lane)
	}
}
