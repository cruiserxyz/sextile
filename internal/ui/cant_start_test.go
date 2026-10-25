package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// the specification. Every test drives Model.Update with the
// real message and reads View(); errors are injected WRAPPED so only errors.Is
// can match them.

const (
	csBinary    = "the astrolog binary was not found."
	csEphemeris = "Astrolog's ephemeris is not being read: every body came back at 0° Aries."
	csAtlas     = "atlas.as, Astrolog's city list, could not be read."
)

func csSized(t *testing.T, cfg Config, w, h int) Model {
	t.Helper()
	m := readyModelWithSlots(t, berlinP)
	m.cfg = cfg
	tm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return tm.(Model)
}

func csSend(t *testing.T, m Model, msgs ...tea.Msg) Model {
	t.Helper()
	var tm tea.Model = m
	for _, msg := range msgs {
		tm, _ = tm.Update(msg)
	}
	return tm.(Model)
}

func csEphemerisErr() error {
	return fmt.Errorf("lookup: FirstCallTransits: %w", lookup.ErrEphemerisMissing)
}

func csBinaryMsg() tea.Msg { return AstrologProbedMsg{Err: notInstalled()} }
func csAtlasMsg() tea.Msg {
	return AtlasLoadedMsg{Err: fmt.Errorf("atlas: read: %w", os.ErrNotExist)}
}
func csEphemerisMsg() tea.Msg {
	return refreshResultMsg{surface: surfaceText, err: csEphemerisErr(), payload: textPayload{msg: FirstFrameReadyMsg{Err: csEphemerisErr()}}}
}

// csText is a frame's words with the box and the wrapping taken out.
func csText(view string) string {
	view = strings.Map(func(r rune) rune {
		if strings.ContainsRune("│╭╮╰╯─|+-", r) {
			return ' '
		}
		return r
	}, view)
	return strings.Join(strings.Fields(view), " ")
}

func TestCantStartBinary(t *testing.T) {
	m := csSized(t, Config{}, 132, 40)
	if strings.Contains(m.View(), "can't start") {
		t.Fatal("a fresh model already shows the can't-start screen")
	}
	got := csText(csSend(t, m, csBinaryMsg()).View())
	if want := "Sextile can't start: " + csBinary; !strings.Contains(got, want) {
		t.Errorf("view lacks %q:\n%s", want, got)
	}
	// A probe that found Astrolog says nothing.
	if strings.Contains(csSend(t, m, AstrologProbedMsg{}).View(), "can't start") {
		t.Error("a successful probe showed the screen")
	}
}

func TestCantStartAtlas(t *testing.T) {
	for name, err := range map[string]error{
		"missing":    fmt.Errorf("atlas: %w", os.ErrNotExist),
		"unreadable": fmt.Errorf("atlas: %w", &fs.PathError{Op: "open", Path: "atlas.as", Err: fs.ErrPermission}),
	} {
		t.Run(name, func(t *testing.T) {
			m := csSend(t, csSized(t, Config{}, 132, 40), AtlasLoadedMsg{Err: err})
			if want := "Sextile can't start: " + csAtlas; !strings.Contains(csText(m.View()), want) {
				t.Errorf("view lacks %q:\n%s", want, m.View())
			}
		})
	}
	ok := csSend(t, csSized(t, Config{}, 132, 40), AtlasLoadedMsg{Index: []location.Indexed{}})
	if strings.Contains(ok.View(), "can't start") {
		t.Error("a loaded atlas showed the screen")
	}
}

// C3: one subtest per message whose handler reports through failureReason.
// The list is the grep in the spec; TestCantStartEphemerisCoversEveryCaller
// holds the grep to it.
func TestCantStartEphemeris(t *testing.T) {
	wrapped := csEphemerisErr()
	cases := map[string]func(m Model) tea.Msg{
		"columns (text commit)": func(m Model) tea.Msg {
			return refreshResultMsg{surface: surfaceText, payload: textPayload{msg: FirstFrameReadyMsg{Columns: []bodyColumn{{label: "Berlin", err: wrapped}}}}}
		},
		"chart cast (text commit)": func(m Model) tea.Msg { return csEphemerisMsg() },
		"wheel render": func(m Model) tea.Msg {
			return refreshResultMsg{surface: surfaceImage, err: wrapped, payload: imagePayload{}}
		},
		"full report": func(m Model) tea.Msg {
			st := m.refresh[surfaceDetails]
			return refreshResultMsg{surface: surfaceDetails, runID: st.runningRunID, wantID: st.wantID, err: wrapped, payload: detailsPayload{}}
		},
		"timeline search": func(m Model) tea.Msg {
			st := m.refresh[surfaceTimeline]
			return refreshResultMsg{surface: surfaceTimeline, runID: st.runningRunID, wantID: st.wantID, err: wrapped, payload: timelinePayload{key: m.timeline.key}}
		},
		"timeline comparing search": func(m Model) tea.Msg {
			st := m.refresh[surfaceTimeline]
			return refreshResultMsg{surface: surfaceTimeline, runID: st.runningRunID, wantID: st.wantID, err: wrapped, payload: timelinePayload{key: m.timeline.key, failures: []lookup.TimelineFailure{{Search: "transits", Err: wrapped}}}}
		},
		"wheel publish":      func(m Model) tea.Msg { return wheelPublishedMsg{err: wrapped} },
		"chart pick resolve": func(m Model) tea.Msg { return ChartResolvedMsg{Gen: m.pickGen, Err: wrapped} },
		"save":               func(m Model) tea.Msg { return SaveChartDoneMsg{Gen: m.saveGen, Name: "x", Err: wrapped} },
		"title write": func(m Model) tea.Msg {
			return TitleWrittenMsg{Gen: m.titleGen, Chart: store.NamedChart{}, Err: wrapped}
		},
	}
	prep := map[string]func(m Model) Model{
		"title write": func(m Model) Model { m.startup = startupTitleOpen; return m },
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			m := csSized(t, Config{}, 132, 40)
			if p := prep[name]; p != nil {
				m = p(m)
			}
			m = csSend(t, m, mk(m))
			if want := "Sextile can't start: " + csEphemeris; !strings.Contains(csText(m.View()), want) {
				t.Errorf("view lacks %q:\n%s", want, m.View())
			}
		})
	}
	// The late results of a save and a title write the operator walked away
	// from report through the same reason.
	t.Run("save, abandoned", func(t *testing.T) {
		m := csSized(t, Config{}, 132, 40)
		m.abandonedSaves = []int{7}
		m = csSend(t, m, SaveChartDoneMsg{Gen: 7, Name: "x", Err: wrapped})
		if !strings.Contains(csText(m.View()), csEphemeris) {
			t.Errorf("no screen:\n%s", m.View())
		}
	})
	t.Run("title write, abandoned", func(t *testing.T) {
		m := csSized(t, Config{}, 132, 40)
		m.startup = startupTitleOpen
		m.abandonedTitles = []int{7}
		m = csSend(t, m, TitleWrittenMsg{Gen: 7, Err: wrapped})
		if !strings.Contains(csText(m.View()), csEphemeris) {
			t.Errorf("no screen:\n%s", m.View())
		}
	})
	// An error that is not the ephemeris one leaves the frame alone.
	t.Run("another failure", func(t *testing.T) {
		m := csSend(t, csSized(t, Config{}, 132, 40), refreshResultMsg{surface: surfaceText, err: errors.New("exit status 1"), payload: textPayload{msg: FirstFrameReadyMsg{Err: errors.New("exit status 1")}}})
		if strings.Contains(m.View(), "can't start") {
			t.Error("an ordinary compute failure showed the screen")
		}
	})
}

// Every call of failureReason in a handler is preceded, on the same or the
// previous lines, by noteEphemeris, so a new call site cannot slip past C3.
func TestCantStartEphemerisCoversEveryCaller(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "failures.go" {
			continue
		}
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(b), "\n")
		for i, l := range lines {
			if !strings.Contains(l, "failureReason(") && !strings.Contains(l, "timelineReason(") || strings.Contains(l, "func (m Model)") {
				continue
			}
			lo := i - 5
			if lo < 0 {
				lo = 0
			}
			window := strings.Join(lines[lo:i+1], "\n")
			// timelineReason is itself a reporter: its callers note first.
			if !strings.Contains(window, "noteEphemeris(") && !strings.Contains(window, "func (m Model) timelineReason(") {
				t.Errorf("%s:%d reports a failure without noteEphemeris: %s", name, i+1, strings.TrimSpace(l))
			}
		}
	}
}

func TestCantStartNotForOneBody(t *testing.T) {
	m := csSized(t, Config{}, 132, 40)
	before := len(m.logLines)
	m = csSend(t, m, refreshResultMsg{surface: surfaceText, payload: textPayload{msg: FirstFrameReadyMsg{Unavailable: []string{"Eris"}}}})
	if strings.Contains(m.View(), "can't start") {
		t.Error("one body without an ephemeris showed the screen")
	}
	var note bool
	for _, l := range m.logLines[before:] {
		if strings.Contains(l.text, "your Astrolog has no ephemeris for Eris — not drawn") {
			note = true
		}
	}
	if !note {
		t.Errorf("the Log note was not written: %v", m.logLines[before:])
	}
}

func TestCantStartListsEachOnceInOrder(t *testing.T) {
	signals := []struct {
		name string
		msg  func() tea.Msg
		text string
	}{{"binary", csBinaryMsg, csBinary}, {"ephemeris", csEphemerisMsg, csEphemeris}, {"atlas", csAtlasMsg, csAtlas}}
	perms := [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	for _, p := range perms {
		name := signals[p[0]].name + "," + signals[p[1]].name + "," + signals[p[2]].name
		t.Run(name, func(t *testing.T) {
			m := csSized(t, Config{}, 132, 40)
			for _, i := range p {
				m = csSend(t, m, signals[i].msg(), signals[i].msg()) // each signal repeated
			}
			got := csText(m.View())
			if strings.Count(got, "Sextile can't start:") != 1 {
				t.Errorf("header count != 1:\n%s", got)
			}
			last := -1
			for _, s := range signals {
				if n := strings.Count(got, s.text); n != 1 {
					t.Errorf("%q appears %d times:\n%s", s.text, n, got)
				}
				at := strings.Index(got, s.text)
				if at < last {
					t.Errorf("%q is out of order:\n%s", s.text, got)
				}
				last = at
				if !strings.Contains(got, "· "+s.text) {
					t.Errorf("%q is not a bullet:\n%s", s.text, got)
				}
			}
			// Every bullet sits two spaces under its lead line, and a wrapped
			// continuation lines up with the bullet's text (four spaces).
			csCheckIndent(t, m.View())
			for _, once := range []string{"To fix it, either", "run Sextile from its Docker image, which builds Astrolog for you, or", "install Astrolog 8.00 as README, Install and run says.", "Then start Sextile again."} {
				if n := strings.Count(got, once); n != 1 {
					t.Errorf("%q appears %d times:\n%s", once, n, got)
				}
			}
			for _, s := range signals {
				if strings.Contains(got, "can't start: "+s.text) {
					t.Errorf("several failures must be bullets under the header:\n%s", got)
				}
			}
		})
	}
	for _, s := range signals {
		t.Run("only "+s.name, func(t *testing.T) {
			got := csText(csSend(t, csSized(t, Config{}, 132, 40), s.msg(), s.msg()).View())
			if want := "Sextile can't start: " + s.text; strings.Count(got, want) != 1 {
				t.Errorf("want %q once:\n%s", want, got)
			}
			if strings.Contains(got, "· "+s.text) {
				t.Errorf("one failure must not be a bullet:\n%s", got)
			}
		})
	}
}

func TestCantStartOnlyQuits(t *testing.T) {
	m := csSend(t, csSized(t, Config{}, 80, 24), csBinaryMsg())
	if m.View() == "" {
		t.Fatal("no screen")
	}
	runCmd := func(c tea.Cmd) tea.Msg {
		if c == nil {
			return nil
		}
		return c()
	}
	for name, k := range map[string]tea.KeyMsg{"q": escRunes("q"), "ctrl+c": escKey(tea.KeyCtrlC)} {
		_, cmd := m.Update(k)
		if _, ok := runCmd(cmd).(tea.QuitMsg); !ok {
			t.Errorf("%s did not quit", name)
		}
	}
	frame := m.View()
	others := map[string]tea.KeyMsg{
		"letter": escRunes("a"), "ctrl+s": escKey(tea.KeyCtrlS), "ctrl+o": escKey(tea.KeyCtrlO),
		"ctrl+x": escKey(tea.KeyCtrlX), "up": escKey(tea.KeyUp), "down": escKey(tea.KeyDown),
		"leader": escRunes(m.leaderKey), "burst": escRunes("abcdefghij"), "esc": escKey(tea.KeyEsc), "enter": escKey(tea.KeyEnter),
	}
	for name, k := range others {
		tm, cmd := m.Update(k)
		if cmd != nil {
			t.Errorf("%s returned a command", name)
		}
		if got := tm.(Model).View(); got != frame {
			t.Errorf("%s changed the frame", name)
		}
	}
	// A burst of ten single keys, one after the other, still changes nothing.
	var tm tea.Model = m
	for _, k := range []tea.KeyMsg{escRunes("a"), escRunes("b"), escKey(tea.KeyCtrlS), escKey(tea.KeyUp), escRunes(m.leaderKey), escRunes("l"), escKey(tea.KeyCtrlO), escRunes("x"), escKey(tea.KeyTab), escKey(tea.KeyDown)} {
		var cmd tea.Cmd
		tm, cmd = tm.Update(k)
		if cmd != nil {
			t.Errorf("%v returned a command", k)
		}
	}
	if tm.(Model).View() != frame {
		t.Error("ten keys changed the frame")
	}
}

func TestTooSmallWinsOverCantStart(t *testing.T) {
	m := csSend(t, csSized(t, Config{}, 80, 24), csBinaryMsg())
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 50, Height: 10})
	small := tm.(Model)
	got := small.View()
	if !strings.Contains(got, "Terminal too small") || strings.Contains(got, "can't start") {
		t.Errorf("below the minimum the view must be the too-small screen:\n%s", got)
	}
	plain := csSized(t, Config{}, 50, 10)
	if got != plain.View() {
		t.Error("the too-small screen differs from the one a healthy model shows")
	}
	tm, _ = small.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if !strings.Contains(tm.(Model).View(), "Sextile can't start:") {
		t.Errorf("resizing back lost the screen:\n%s", tm.(Model).View())
	}
}

func TestCantStartFitsTheMinimum(t *testing.T) {
	for _, cfg := range []Config{{}, {Accessible: true, Color: ColorNever}} {
		m := csSized(t, cfg, 66, 15)
		m = csSend(t, m, csBinaryMsg(), csEphemerisMsg(), csAtlasMsg())
		view := m.View()
		lines := strings.Split(view, "\n")
		if len(lines) > 15 {
			t.Errorf("accessible=%v: %d lines, want <= 15:\n%s", cfg.Accessible, len(lines), view)
		}
		for _, l := range lines {
			if w := ansi.StringWidth(l); w > 66 {
				t.Errorf("accessible=%v: line of %d cells: %q", cfg.Accessible, w, l)
			}
		}
		if !cfg.Accessible {
			csCheckIndent(t, view)
		}
		got := csText(view)
		for _, want := range []string{csBinary, csEphemeris, csAtlas, "Then start Sextile again."} {
			if !strings.Contains(got, want) {
				t.Errorf("accessible=%v: the minimum frame lost %q:\n%s", cfg.Accessible, want, view)
			}
		}
	}
}

func TestCantStartKeepsLogLines(t *testing.T) {
	m := csSized(t, Config{}, 132, 40)
	m = csSend(t, m, csBinaryMsg(), AtlasLoadedMsg{Err: os.ErrNotExist}, ChartsLoadedMsg{Gen: m.pickerGen, AtlasErr: os.ErrNotExist})
	lines := errorLines(m)
	count := func(s string) (n int) {
		for _, l := range lines {
			if l == s {
				n++
			}
		}
		return
	}
	if count(probeLine) != 1 {
		t.Errorf("probe line written %d times: %v", count(probeLine), lines)
	}
	if count(atlasLine) != 1 {
		t.Errorf("picker atlas line written %d times: %v", count(atlasLine), lines)
	}
}

func TestCantStartFrames(t *testing.T) {
	both := func(m Model) Model { return csSend(t, m, csEphemerisMsg(), csAtlasMsg()) }
	only := func(m Model) Model { return csSend(t, m, csBinaryMsg()) }
	frames := []struct {
		name string
		cfg  Config
		w, h int
		set  func(Model) Model
	}{
		{"cant_start_binary_80x24", Config{}, 80, 24, only},
		{"cant_start_two_80x24", Config{}, 80, 24, both},
		{"cant_start_binary_80x24_accessible_never", Config{Accessible: true, Color: ColorNever}, 80, 24, only},
		{"cant_start_binary_66x15", Config{}, 66, 15, only},
		{"cant_start_two_66x15", Config{}, 66, 15, both},
		{"cant_start_binary_66x15_accessible_never", Config{Accessible: true, Color: ColorNever}, 66, 15, only},
	}
	for _, f := range frames {
		t.Run(f.name, func(t *testing.T) {
			checkGolden(t, f.name, f.set(csSized(t, f.cfg, f.w, f.h)).View())
		})
	}
}

// csCheckIndent reads the box's rows (border cut off) and holds the bullet
// structure: lead lines at one indent, each bullet two spaces further in, and
// each continuation under the bullet's text.
func csCheckIndent(t *testing.T, view string) {
	t.Helper()
	lead := -1
	var bullets int
	prevBullet := false
	for _, row := range strings.Split(view, "\n") {
		row = strings.TrimRight(row, " ")
		i, j := strings.Index(row, "│"), strings.LastIndex(row, "│")
		if i < 0 || j <= i {
			continue
		}
		body := strings.TrimRight(row[i+len("│"):j], " ")
		if strings.TrimSpace(body) == "" {
			prevBullet = false
			continue
		}
		indent := len(body) - len(strings.TrimLeft(body, " "))
		text := strings.TrimLeft(body, " ")
		switch {
		case strings.HasPrefix(text, "Sextile can't start:") || strings.HasPrefix(text, "To fix it") || strings.HasPrefix(text, "Then start"):
			if lead < 0 {
				lead = indent
			}
			if indent != lead {
				t.Errorf("lead line %q at indent %d, want %d", text, indent, lead)
			}
			prevBullet = false
		case strings.HasPrefix(text, "· "):
			bullets++
			if indent != lead+2 {
				t.Errorf("bullet %q at indent %d, want %d (lead %d + 2)", text, indent, lead+2, lead)
			}
			prevBullet = true
		default:
			if !prevBullet || indent != lead+4 {
				t.Errorf("continuation %q at indent %d, want %d under a bullet", text, indent, lead+4)
			}
		}
	}
	if bullets == 0 {
		t.Error("no bullets found")
	}
}
