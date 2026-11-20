package ui

import (
	"fmt"
	"github.com/cruiserxyz/sextile/internal/ui/term"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The wheel notice: the Log names where the wheel is served, once per session
// (the specification W1 part 1, A1 and A4).

const wheelURLStub = "http://localhost:5307/"

// stubShowWheelURL swaps the publisher for one that answers like the HTTP
// preview does: the server's address, with its closing slash.
func stubShowWheelURL(t *testing.T, url string) *int {
	t.Helper()
	calls := 0
	stubSpawns(t, func(s *rowSpawns) {
		s.wheel.show = func([]byte) (string, error) {
			calls++
			return url, nil
		}
	})
	return &calls
}

// publishOnce drives one off-thread publish the way the coordinator does:
// bytes queued, the publish Cmd run, its answer handed back to Update.
func publishOnce(t *testing.T, m Model) Model {
	t.Helper()
	m.previewActive = true
	m.wheelPublishKey, _ = m.wheelStillKey()
	m.wheelPublishPending = []byte("<svg/>")
	m, cmd := m.kickWheelPublish()
	if cmd == nil {
		t.Fatal("no publish started")
	}
	msg, ok := cmd().(wheelPublishedMsg)
	if !ok {
		t.Fatal("the publish Cmd did not yield a wheelPublishedMsg")
	}
	m, _ = m.onWheelPublished(msg)
	return m
}

func wheelOpenLines(m Model) []string {
	var out []string
	for _, l := range m.logLines {
		if l.lane == laneWheelOpen {
			out = append(out, l.text)
		}
	}
	return out
}

// A1: the first successful publish says where the wheel is, once; a refresh
// says nothing more; a desktop viewer's file path is no address.
func TestWheelNoticeOncePerSession(t *testing.T) {
	stubShowWheelURL(t, wheelURLStub)
	m := publishOnce(t, modelWithSlot(t))
	want := "wheel: served at http://localhost:5307"
	if got := wheelOpenLines(m); len(got) != 1 || got[0] != want {
		t.Fatalf("after the first publish the notice lines = %q, want exactly [%q]", got, want)
	}
	for i := 0; i < 3; i++ {
		m = publishOnce(t, m)
	}
	if got := wheelOpenLines(m); len(got) != 1 {
		t.Errorf("after refreshes the notice lines = %q, want still one", got)
	}

	// Not an address: nothing to name.
	freshModels(t)
	stubShowWheel(t)
	if got := wheelOpenLines(publishOnce(t, modelWithSlot(t))); len(got) != 0 {
		t.Errorf("a file path produced the notice %q", got)
	}
}

// A4: the notice in the Log panel at 132x40 and in the console at 80x24, with
// colours and without; its colour is its own, neither the focus green nor the
// error red.
func wheelNoticeModel(t *testing.T, cfg Config, w, h int, closed bool) Model {
	t.Helper()
	m := modelWithSlot(t)
	m.cfg = cfg
	m.startup = startupReady
	m.focused = PanelBodies
	m.width, m.height = w, h
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{Positions: fullChartPositions()})
	m.logLines = nil
	m = m.logNote("session", "the chart of the last session did not open")
	m = m.logWheelOpen(wheelURLStub)
	if closed {
		m = m.logWheelClosed()
	}
	return m
}

// logPanelRegion is the Log panel's rectangle of the composed frame, escapes
// kept.
func logPanelRegion(t *testing.T, m Model) string {
	t.Helper()
	r, ok := m.geometry()[PanelLog]
	if !ok {
		t.Fatal("no Log panel in this layout")
	}
	rows := frameRows(m.View())
	var out []string
	for y := r.y; y < r.y+r.h; y++ {
		out = append(out, ansi.Cut(rows[y], r.x, r.x+r.w))
	}
	return strings.Join(out, "\n") + "\n"
}

func TestWheelNoticeFrames(t *testing.T) {
	forceColor(t)
	modes := []struct {
		label string
		cfg   Config
	}{{"accessible", Config{Accessible: true, Color: ColorNever}}, {"color", Config{Color: ColorAlways}}}
	for _, md := range modes {
		t.Run("log-132x40-"+md.label, func(t *testing.T) {
			m := wheelNoticeModel(t, md.cfg, 132, 40, false)
			checkGolden(t, fmt.Sprintf("wheel-notice-open-log-132x40-%s", md.label), logPanelRegion(t, m))
		})
		t.Run("console-80x24-"+md.label, func(t *testing.T) {
			m := wheelNoticeModel(t, md.cfg, 80, 24, false)
			m, _ = press(t, m, ",", "L", "l")
			r, ok := m.consoleRect()
			if !ok {
				t.Fatal("no console")
			}
			box := renderConsole(m.console.snapshot, m.console, m.tabStrips()[PanelLog], r.w, r.h, true, m.cfg)
			checkGolden(t, fmt.Sprintf("wheel-notice-open-console-80x24-%s", md.label), box+"\n")
		})
	}

	t.Run("the group is cyan, the words are plain", func(t *testing.T) {
		m := wheelNoticeModel(t, Config{Color: ColorAlways}, 132, 40, false)
		s := parseDesignScreen(m.View())
		text := "Wheel    served at http://localhost:5307"
		y := rowWith(s, m.geometry()[PanelLog], text)
		if y < 0 {
			t.Fatal("the notice is not on screen")
		}
		cyan, green, red := styleFG(t, term.LogFactColour), styleFG(t, term.DefaultFocusColour), styleFG(t, term.ErrorColour)
		if cyan == green || cyan == red {
			t.Fatalf("the group colour %q collides with the focus green %q or the error red %q", cyan, green, red)
		}
		cells := cellsOf(s, y, text)
		for i, c := range cells {
			switch {
			case i < len("Wheel") && c.fg != cyan:
				t.Errorf("group cell %q is fg %q, want cyan %q", c.r, c.fg, cyan)
			case i >= logGroupCells+1 && c.r != ' ' && c.fg == cyan:
				t.Errorf("message cell %q is cyan", c.r)
			}
		}
	})
}

// A4 for the closing line: after the notice, red with colours on, plain words
// without (no error mark: the operator asked for it), in the Log panel at
// 132x40 and the console at 80x24.
func TestWheelClosedFrames(t *testing.T) {
	forceColor(t)
	modes := []struct {
		label string
		cfg   Config
	}{{"accessible", Config{Accessible: true, Color: ColorNever}}, {"color", Config{Color: ColorAlways}}}
	for _, md := range modes {
		t.Run("log-132x40-"+md.label, func(t *testing.T) {
			m := wheelNoticeModel(t, md.cfg, 132, 40, true)
			checkGolden(t, fmt.Sprintf("wheel-notice-closed-log-132x40-%s", md.label), logPanelRegion(t, m))
		})
		t.Run("console-80x24-"+md.label, func(t *testing.T) {
			m := wheelNoticeModel(t, md.cfg, 80, 24, true)
			m, _ = press(t, m, ",", "L", "l")
			r, ok := m.consoleRect()
			if !ok {
				t.Fatal("no console")
			}
			box := renderConsole(m.console.snapshot, m.console, m.tabStrips()[PanelLog], r.w, r.h, true, m.cfg)
			checkGolden(t, fmt.Sprintf("wheel-notice-closed-console-80x24-%s", md.label), box+"\n")
		})
	}

	t.Run("a closing line is a fact, not an error", func(t *testing.T) {
		m := wheelNoticeModel(t, Config{Color: ColorAlways}, 132, 40, true)
		s := parseDesignScreen(m.View())
		text := "Wheel    server closed"
		y := rowWith(s, m.geometry()[PanelLog], text)
		if y < 0 {
			t.Fatal("the closing line is not on screen")
		}
		cyan := styleFG(t, term.LogFactColour)
		for i, c := range cellsOf(s, y, text) {
			if i < len("Wheel") && c.fg != cyan {
				t.Errorf("group cell %q is fg %q, want cyan %q", c.r, c.fg, cyan)
			}
		}
		for _, row := range m.logPanelRows() {
			if strings.Contains(ansi.Strip(row), "! ") && strings.Contains(row, "Wheel") {
				t.Errorf("the closing line carries the error mark: %q", row)
			}
		}
	})
}
