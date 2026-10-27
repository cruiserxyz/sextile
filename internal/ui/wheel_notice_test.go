package ui

import (
	"fmt"
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
	prev := showWheel
	showWheel = func([]byte) (string, error) {
		calls++
		return url, nil
	}
	t.Cleanup(func() { showWheel = prev })
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
	want := "Wheel at http://localhost:5307 (Ctrl+X again closes it)"
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
	m = m.logNote("session", "natal not restored")
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
			m, _ = press(t, m, ",", "l")
			r, ok := m.consoleRect()
			if !ok {
				t.Fatal("no console")
			}
			box := renderConsole(m.console.snapshot, m.console, m.tabStrips()[PanelLog], r.w, r.h, true, m.cfg)
			checkGolden(t, fmt.Sprintf("wheel-notice-open-console-80x24-%s", md.label), box+"\n")
		})
	}

	t.Run("colour is its own", func(t *testing.T) {
		m := wheelNoticeModel(t, Config{Color: ColorAlways}, 132, 40, false)
		s := parseDesignScreen(m.View())
		text := "Wheel at http://localhost:5307 (Ctrl+X again closes it)"
		y := rowWith(s, m.geometry()[PanelLog], text)
		if y < 0 {
			t.Fatal("the notice is not on screen")
		}
		want, green, red := styleFG(t, wheelOpenColour), styleFG(t, focusColour), styleFG(t, errorColour)
		if want == green || want == red {
			t.Fatalf("the notice colour %q collides with the focus green %q or the error red %q", want, green, red)
		}
		for _, c := range cellsOf(s, y, text) {
			if c.r != ' ' && c.fg != want {
				t.Errorf("cell %q is fg %q, want the notice colour %q", c.r, c.fg, want)
				break
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
			m, _ = press(t, m, ",", "l")
			r, ok := m.consoleRect()
			if !ok {
				t.Fatal("no console")
			}
			box := renderConsole(m.console.snapshot, m.console, m.tabStrips()[PanelLog], r.w, r.h, true, m.cfg)
			checkGolden(t, fmt.Sprintf("wheel-notice-closed-console-80x24-%s", md.label), box+"\n")
		})
	}

	t.Run("colour is the error red, the words are plain", func(t *testing.T) {
		m := wheelNoticeModel(t, Config{Color: ColorAlways}, 132, 40, true)
		s := parseDesignScreen(m.View())
		text := "Wheel closed. Ctrl+X opens it again"
		y := rowWith(s, m.geometry()[PanelLog], text)
		if y < 0 {
			t.Fatal("the closing line is not on screen")
		}
		want := styleFG(t, wheelClosedColour)
		for _, c := range cellsOf(s, y, text) {
			if c.r != ' ' && c.fg != want {
				t.Errorf("cell %q is fg %q, want the red %q", c.r, c.fg, want)
				break
			}
		}
		for _, row := range m.logPanelRows() {
			if strings.Contains(ansi.Strip(row), "! Wheel") {
				t.Errorf("the closing line carries the error mark: %q", row)
			}
		}
	})
}
