package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// the specification Part A: two forms for a Log line, errors that look
// like errors, settled empty states, a session failure reported after quit,
// and late work still logged. Each requirement is read off the composed
// screen where the operator would see it.

// messagesModel is a ready 144x40 pair workspace with the Log drawn, holding
// two notes and two errors written through the production writers.
func messagesModel(t *testing.T, cfg Config) Model {
	t.Helper()
	m := tabsPairModel(t, cfg)
	m.width, m.height = 144, 40
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{Positions: fullChartPositions()})
	m = withSceneColumns(m)
	m.logLines = nil
	m = m.logNote("save", "%q saved to %s", "Constructed", "charts/constructed.as")
	m = m.logError("Couldn't cast the chart. %s", "Astrolog could not be started")
	m = m.logNote("session", "natal not restored")
	m = m.logError("Couldn't copy that line to the clipboard.")
	return m
}

const (
	messagesErrOne = "Couldn't cast the chart. Astrolog could not be started."
	messagesErrTwo = "Couldn't copy that line to the clipboard."
	messagesNote   = "save: \"Constructed\" saved to charts/constructed.as"
)

// cellsOf is the cells of the first occurrence of sub on screen row y, or nil.
func cellsOf(s designScreen, y int, sub string) []designCell {
	var cells []designCell
	for _, c := range s[y] {
		if c.r != 0 {
			cells = append(cells, c)
		}
	}
	runes := []rune(sub)
	for x := 0; x+len(runes) <= len(cells); x++ {
		match := true
		for i, r := range runes {
			if cells[x+i].r != r {
				match = false
				break
			}
		}
		if match {
			return cells[x : x+len(runes)]
		}
	}
	return nil
}

// rowWith is the first screen row inside r holding sub, or -1.
func rowWith(s designScreen, r panelRect, sub string) int {
	for y := r.y; y < r.y+r.h && y < len(s); y++ {
		if cellsOf(s, y, sub) != nil {
			return y
		}
	}
	return -1
}

// A1, A2: an error line is red across the whole line with colours on, in the
// Log panel and the console, on the console's cursor row and off it; it opens
// with `! ` with colours off; a note is neither; Enter copies the line as
// stored.
func TestErrorLinesStandOut(t *testing.T) {
	forceColor(t)
	red := styleFG(t, errorColour)

	t.Run("colour", func(t *testing.T) {
		m := messagesModel(t, Config{Color: ColorAlways})
		logR := m.geometry()[PanelLog]
		check := func(where string, s designScreen, r panelRect) {
			t.Helper()
			for _, e := range []string{messagesErrOne, messagesErrTwo} {
				y := rowWith(s, r, e)
				if y < 0 {
					t.Fatalf("%s: %q is not on screen", where, e)
				}
				for _, c := range cellsOf(s, y, e) {
					if c.r != ' ' && c.fg != red {
						t.Errorf("%s: %q: cell %q is fg %q, want the error colour %q", where, e, c.r, c.fg, red)
						break
					}
				}
			}
			y := rowWith(s, r, messagesNote)
			if y < 0 {
				t.Fatalf("%s: the note is not on screen", where)
			}
			for _, c := range cellsOf(s, y, messagesNote) {
				if c.fg == red {
					t.Errorf("%s: the note is drawn in the error colour", where)
					break
				}
			}
		}
		check("Log panel", parseDesignScreen(m.View()), logR)

		// The console opens with its cursor on the newest line, an error;
		// the other error is off the cursor row.
		m, _ = press(t, m, ",", "l")
		r, ok := m.consoleRect()
		if !ok {
			t.Fatal("no console")
		}
		s := parseDesignScreen(m.View())
		check("console", s, r)
		cursorY := rowWith(s, r, messagesErrTwo)
		if c := cellsOf(s, cursorY, messagesErrTwo); c == nil || c[0].bg == "" {
			t.Errorf("the newest error is not on the cursor band")
		}
	})

	t.Run("no colour", func(t *testing.T) {
		m := messagesModel(t, Config{Accessible: true, Color: ColorNever})
		rows := logPanelRows(t, m)
		want := map[string]bool{"! " + messagesErrOne: false, "! " + messagesErrTwo: false}
		for _, r := range rows {
			if _, ok := want[strings.TrimSpace(r)]; ok {
				want[strings.TrimSpace(r)] = true
			}
			if strings.HasPrefix(strings.TrimSpace(r), "! save:") || strings.HasPrefix(strings.TrimSpace(r), "! session:") {
				t.Errorf("a note carries the error mark: %q", r)
			}
		}
		for line, seen := range want {
			if !seen {
				t.Errorf("the Log panel does not show %q; rows %q", line, rows)
			}
		}

		last, _, restore := stubYankRecording(t, nil)
		defer restore()
		m, _ = press(t, m, ",", "l")
		body := strings.Join(consoleBodyLines(t, m), "\n")
		if !strings.Contains(body, "! "+messagesErrTwo) || strings.Contains(body, "! "+messagesNote) {
			t.Errorf("console rows:\n%s", body)
		}
		m, _ = press(t, m, "enter")
		if *last != messagesErrTwo {
			t.Errorf("Enter copied %q, want the stored line %q", *last, messagesErrTwo)
		}
	})
}

// newestLogRow is the last non-empty row of the closed Log panel.
func newestLogRow(t *testing.T, m Model) string {
	t.Helper()
	rows := logPanelRows(t, m)
	for i := len(rows) - 1; i >= 0; i-- {
		if r := strings.TrimSpace(rows[i]); r != "" {
			return r
		}
	}
	t.Fatal("the Log panel is empty")
	return ""
}

// A3: a note is `scope: text`, with no closing stop and no line break,
// whatever the substituted text holds.
func TestLogNoteForm(t *testing.T) {
	for _, c := range []struct {
		name, arg, want string
	}{
		{"plain", "kept", "chart: titled kept"},
		{"closing stop", "kept.", "chart: titled kept"},
		{"line break", "kept\nand more", "chart: titled kept and more"},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := messagesModel(t, Config{Accessible: true, Color: ColorNever})
			m = m.logNote("chart", "titled %s", c.arg)
			if got := newestLogRow(t, m); got != c.want {
				t.Errorf("Log shows %q, want %q", got, c.want)
			}
		})
	}
}

// A4: an error ends with exactly one full stop and holds no line break.
func TestLogErrorForm(t *testing.T) {
	for _, c := range []struct {
		name, arg string
	}{
		{"no stop", "It stopped before it finished"},
		{"one stop", "It stopped before it finished."},
		{"two stops", "It stopped before it finished.."},
		{"line break", "It stopped\nbefore it finished"},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := messagesModel(t, Config{Accessible: true, Color: ColorNever})
			m = m.logError("Couldn't cast the chart. %s", c.arg)
			want := "! Couldn't cast the chart. It stopped before it finished."
			if got := newestLogRow(t, m); got != want {
				t.Errorf("Log shows %q, want %q", got, want)
			}
		})
	}
}

// aspectsPanelText is the Aspects panel's inner text on the composed screen.
func aspectsPanelText(t *testing.T, m Model) string {
	t.Helper()
	r, ok := m.geometry()[PanelAspects]
	if !ok {
		t.Fatal("no Aspects panel")
	}
	rows := frameRows(m.View())
	var out []string
	for y := r.y + 1; y < r.y+r.h-1; y++ {
		out = append(out, strings.TrimSpace(ansi.Strip(ansi.Cut(rows[y], r.x+1, r.x+r.w-1))))
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// A6: a settled empty list says why; before the first frame and after a
// failed one the Aspects tabs stay blank.
func TestSettledEmptyStatesSayWhy(t *testing.T) {
	cfg := Config{Accessible: true, Color: ColorNever}
	base := func(t *testing.T) Model {
		m := tabsPairModel(t, cfg)
		m.width, m.height = 144, 40
		return m
	}
	t.Run("own tab", func(t *testing.T) {
		// One slot, so the own tab is the only one.
		m := readyModelWithSlots(t, berlinP)
		m.cfg = cfg
		m.width, m.height = 144, 40
		m, _ = applyTextCommit(m, FirstFrameReadyMsg{Positions: fullChartPositions()})
		m = withSceneColumns(m)
		if got := aspectsPanelText(t, m); got != ownEmptyLine {
			t.Errorf("Aspects shows %q, want %q", got, ownEmptyLine)
		}
	})
	t.Run("cross tab", func(t *testing.T) {
		// The pair tab is reached with a real `]`: arrival at now is `Aspects`
		// (the specification C1).
		onPair, _ := tpress(t, base(t), tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
		m, _ := applyTextCommit(onPair, FirstFrameReadyMsg{Positions: fullChartPositions(), Aspects: tasteOwnAspects(), Overlay: true})
		m = withSceneColumns(m)
		if m.activeTabID(PanelAspects) != TabAspectsCross {
			t.Fatalf("the cross tab is not active")
		}
		if got := aspectsPanelText(t, m); got != crossEmptyLine {
			t.Errorf("the cross tab shows %q, want %q", got, crossEmptyLine)
		}
	})
	t.Run("console", func(t *testing.T) {
		m := base(t)
		m.logLines = nil
		m, _ = press(t, m, ",", "l")
		if got := strings.TrimSpace(strings.Join(consoleBodyLines(t, m), "\n")); got != consoleEmptyLine {
			t.Errorf("the empty console shows %q, want %q", got, consoleEmptyLine)
		}
	})
	t.Run("before the first frame", func(t *testing.T) {
		if got := aspectsPanelText(t, base(t)); got != "" {
			t.Errorf("Aspects shows %q before any frame", got)
		}
	})
	for _, overlay := range []bool{false, true} {
		t.Run(fmt.Sprintf("failed frame, overlay %v", overlay), func(t *testing.T) {
			m, _ := applyTextCommit(base(t), FirstFrameReadyMsg{Overlay: overlay, Err: errors.New("boom")})
			if got := aspectsPanelText(t, m); got != "" {
				t.Errorf("Aspects shows %q after a failed frame", got)
			}
		})
	}
}

// A7: a session that cannot be written at quit, by q or Ctrl+C, is carried
// out of the model with its file and cause.
func TestSessionWriteFailureIsReported(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		t.Run(k, func(t *testing.T) {
			// The session directory is a FILE, so the write cannot make it.
			blocker := filepath.Join(t.TempDir(), "not-a-dir")
			if err := os.WriteFile(blocker, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("XDG_STATE_HOME", blocker)
			m := escBase(t)
			m.focused = PanelChart
			tm, cmd := m.Update(keyMsgFor(t, k))
			if cmd == nil {
				t.Fatal("the key did not quit")
			}
			err := tm.(Model).SessionWriteError()
			if err == nil {
				t.Fatalf("the failed session write was not kept (session path %s)", store.SessionPath())
			}
			if !strings.HasPrefix(err.Error(), "couldn't save the session to "+store.SessionPath()+": ") {
				t.Errorf("error = %q", err)
			}
			if strings.Contains(err.Error(), "store:") || strings.Contains(err.Error(), "SaveSession") {
				t.Errorf("the line carries the write path's plumbing: %q", err)
			}
		})
	}
	t.Run("one line", func(t *testing.T) {
		e := &SessionWriteError{Path: "/a\nb/session.yaml", Err: errors.New("no\nroom")}
		if strings.ContainsAny(e.Error(), "\r\n") {
			t.Errorf("the stderr line breaks: %q", e.Error())
		}
	})
	t.Run("written", func(t *testing.T) {
		t.Setenv("XDG_STATE_HOME", t.TempDir())
		m := escBase(t)
		tm, _ := m.Update(keyMsgFor(t, "q"))
		if err := tm.(Model).SessionWriteError(); err != nil {
			t.Errorf("a written session reported %v", err)
		}
	})
}

// A8: a save or title write Esc walked away from still leaves its Log line
// when it lands, once, and changes nothing else on screen; a superseded
// write leaves nothing.
func TestLateWorkStillLogs(t *testing.T) {
	saving := func(t *testing.T) Model {
		m := escBase(t)
		m.focused = PanelChart
		m.cursorIdx = len(m.slots.All()) - 1
		m, _ = escSend(t, m, escKey(tea.KeyCtrlS), escRunes("Constructed"))
		var cmd tea.Cmd
		m, cmd = escSend(t, m, escKey(tea.KeyEnter))
		if cmd == nil || !m.save.saving {
			t.Fatal("the save did not start")
		}
		return m
	}
	land := func(t *testing.T, m Model, msg tea.Msg, want string) {
		t.Helper()
		before := snapshotForEsc(m)
		logged := len(m.logPanelLines())
		m, _ = escSend(t, m, msg)
		requireSameScreen(t, "the late result", before, snapshotForEsc(m))
		got := m.logPanelLines()[logged:]
		if len(got) != 1 || got[0] != want {
			t.Errorf("the late result logged %q, want exactly %q", got, want)
		}
		// Once: the same result again is a stale one.
		if m2, _ := escSend(t, m, msg); len(m2.logPanelLines()) != len(m.logPanelLines()) {
			t.Error("a second delivery logged again")
		}
	}

	t.Run("save succeeded", func(t *testing.T) {
		m := saving(t)
		gen := m.saveGen
		m, _ = escSend(t, m, escKey(tea.KeyEsc))
		land(t, m, SaveChartDoneMsg{Gen: gen, Name: "Constructed", Saved: lookup.SavedChart{Name: "Constructed", Path: "/charts/constructed.as", AtlasPlaced: true}},
			`save: "Constructed" saved to /charts/constructed.as`)
	})
	t.Run("save failed", func(t *testing.T) {
		m := saving(t)
		gen := m.saveGen
		m, _ = escSend(t, m, escKey(tea.KeyEsc))
		land(t, m, SaveChartDoneMsg{Gen: gen, Name: "Constructed", Err: &lookup.ErrSlugTaken{Slug: "constructed"}},
			`Couldn't save "Constructed". That name is taken.`)
	})
	t.Run("two abandoned saves", func(t *testing.T) {
		m := saving(t)
		first := m.saveGen
		m, _ = escSend(t, m, escKey(tea.KeyEsc), escKey(tea.KeyCtrlS), escRunes("Second"))
		m, _ = escSend(t, m, escKey(tea.KeyEnter))
		second := m.saveGen
		m, _ = escSend(t, m, escKey(tea.KeyEsc))
		land(t, m, SaveChartDoneMsg{Gen: second, Name: "Second", Saved: lookup.SavedChart{Name: "Second", Path: "/c/second.as", AtlasPlaced: true}},
			`save: "Second" saved to /c/second.as`)
		land(t, m, SaveChartDoneMsg{Gen: first, Name: "Constructed", Saved: lookup.SavedChart{Name: "Constructed", Path: "/c/constructed.as", AtlasPlaced: true}},
			`save: "Constructed" saved to /c/constructed.as`)
	})

	titling := func(t *testing.T) Model {
		m := escBase(t)
		m, _ = escSend(t, m, escKey(tea.KeyCtrlO))
		m, _ = escSend(t, m, ChartsLoadedMsg{Charts: listRowsCharts(), Gen: m.pickerGen}, ChartPickedMsg{Chart: keysUntitled()})
		m, _ = escSend(t, m, escRunes("Constructed"), escKey(tea.KeyTab), escRunes("Berlin"))
		m, _ = escSend(t, m, escKey(tea.KeyEnter)) // the second step
		var cmd tea.Cmd
		m, cmd = escSend(t, m, escKey(tea.KeyEnter))
		if cmd == nil || !m.title.saving {
			t.Fatalf("the title write did not start (startup %s)", m.startup)
		}
		return m
	}
	t.Run("title written", func(t *testing.T) {
		m := titling(t)
		gen := m.titleGen
		m, _ = escSend(t, m, escKey(tea.KeyEsc))
		land(t, m, TitleWrittenMsg{Gen: gen, Chart: keysUntitled(), Result: lookup.Retitled{File: "keys-untitled.as", Name: "Constructed", Backup: "/data/op/.local/share/sextile/backup/0f/keys-untitled.as"}},
			`chart: titled keys-untitled.as as "Constructed" (original kept as /data/op/.local/share/sextile/backup/0f/keys-untitled.as)`)
	})
	t.Run("title failed", func(t *testing.T) {
		m := titling(t)
		gen := m.titleGen
		m, _ = escSend(t, m, escKey(tea.KeyEsc))
		land(t, m, TitleWrittenMsg{Gen: gen, Chart: keysUntitled(), Err: &fs.PathError{Op: "rename", Path: "x", Err: fs.ErrPermission}},
			"Couldn't title that chart. rename x: permission denied.")
	})
	t.Run("superseded title write", func(t *testing.T) {
		m := titling(t)
		gen := m.titleGen
		// Not Esc: a newer write supersedes this one's generation.
		m.titleGen++
		logged := len(m.logPanelLines())
		m, _ = escSend(t, m, TitleWrittenMsg{Gen: gen, Chart: keysUntitled(), Result: lookup.Retitled{File: "keys-untitled.as", Name: "Constructed"}})
		if got := m.logPanelLines()[logged:]; len(got) != 0 {
			t.Errorf("a superseded write logged %q", got)
		}
	})
}

// reportFailed commits a failed full report, which keeps the last report.
func reportFailed(t *testing.T) Model {
	t.Helper()
	m := detailsTestModel(t)
	m.width, m.height = 80, 24
	m.logLines = nil
	m.errorUnseen = false
	m, _ = m.commitRefresh(surfaceDetails, refreshResultMsg{surface: surfaceDetails, err: errors.New("boom"), payload: detailsPayload{}})
	return m
}

// U3 8: after a report failure the mark says the previous view is still valid.
func TestReportFailureMarkSaysViewUnchanged(t *testing.T) {
	m := reportFailed(t)
	got := m.footerIndicator()
	if want := errorMarkTextView(",", m.cfg, true); got != want {
		t.Fatalf("footer = %q, want %q", got, want)
	}
	if !isErrorMark(got) || !strings.Contains(ansi.Strip(got), "view unchanged") {
		t.Errorf("mark %q is not the kept-view error mark", got)
	}
	if a := ansi.Strip(errorMarkTextView(",", Config{Accessible: true}, true)); a != "! error - view unchanged - ,l" {
		t.Errorf("ASCII mark = %q", a)
	}
}

// U3 9: an error from any other site keeps the plain mark, and opening the
// console clears the mark and the kept-view flag.
func TestOtherErrorsKeepThePlainMark(t *testing.T) {
	m := detailsTestModel(t)
	m.width, m.height = 80, 24
	m.logLines = nil
	m = m.logError("Couldn't cast the chart. %s", "boom")
	if got := ansi.Strip(m.footerIndicator()); got != "! error · ,l" {
		t.Errorf("mark = %q, want the plain mark", got)
	}
	// A later error from another site retires the kept-view claim.
	k := reportFailed(t).logError("Couldn't cast the chart. %s", "boom")
	if got := ansi.Strip(k.footerIndicator()); got != "! error · ,l" {
		t.Errorf("mark after another error = %q, want the plain mark", got)
	}
	r := reportFailed(t)
	n, _ := r.openConsole()
	r = n.(Model)
	if r.errorUnseen || r.errorKeptView || r.footerIndicator() != "" {
		t.Errorf("opening the console left unseen=%v kept=%v footer=%q", r.errorUnseen, r.errorKeptView, r.footerIndicator())
	}
}
