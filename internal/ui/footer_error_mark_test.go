package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// errorMarkModel is a ready model at w×h with no log lines.
func errorMarkModel(t *testing.T, w, h int) Model {
	t.Helper()
	m := detailsTestModel(t)
	m.cfg = Config{Accessible: true}
	m.width, m.height = w, h
	m.logLines = nil
	m.errorUnseen = false
	return m
}

const errorMarkWant = "! error - ,l"

// the specification 1, 2, 6.
func TestErrorMarkWhenLogIsHidden(t *testing.T) {
	m := errorMarkModel(t, 80, 24)
	if _, ok := m.geometry()[PanelLog]; ok {
		t.Fatal("80x24 shows a Log panel; the premise of the test is gone")
	}
	// 6: note and argv lines never raise it.
	m = m.logNote("save", "saved")
	m = m.logCommands([]string{"astrolog -qa"})
	if got := m.footerIndicator(); got != "" {
		t.Errorf("a note or argv line raised %q", got)
	}
	// 1: an error does.
	m = m.logError("Couldn't cast the chart. %s", "Astrolog could not be started")
	if got := m.footerIndicator(); got != errorMarkWant {
		t.Fatalf("footer = %q, want %q", got, errorMarkWant)
	}
	if !strings.Contains(ansi.Strip(m.View()), errorMarkWant) {
		t.Errorf("the key bar does not end with the mark:\n%s", m.View())
	}
	// 2: it stays through further errors and notes...
	m = m.logError("Couldn't copy that line to the clipboard.").logNote("x", "y")
	if got := m.footerIndicator(); got != errorMarkWant {
		t.Errorf("after more lines footer = %q", got)
	}
	// ...goes when the console opens...
	m, _ = press(t, m, ",", "l")
	if !m.console.open {
		t.Fatal("the console did not open")
	}
	m, _ = press(t, m, "esc")
	if got := m.footerIndicator(); got != "" {
		t.Errorf("after the console visit footer = %q", got)
	}
	// ...and a new error brings it back.
	m = m.logError("Couldn't cast the chart. %s", "again")
	if got := m.footerIndicator(); got != errorMarkWant {
		t.Errorf("after a new error footer = %q", got)
	}
}

// the specification 3.
func TestErrorMarkFollowsTheLayout(t *testing.T) {
	m := errorMarkModel(t, 80, 24)
	m = m.logError("Couldn't cast the chart. %s", "x")
	if m.footerIndicator() == "" {
		t.Fatal("no mark at 80x24")
	}
	m.width, m.height = 132, 40
	if _, ok := m.geometry()[PanelLog]; !ok {
		t.Fatal("132x40 shows no Log panel")
	}
	if got := m.footerIndicator(); got != "" {
		t.Errorf("mark with the Log on screen: %q", got)
	}
	m.width, m.height = 80, 24
	if got := m.footerIndicator(); got != errorMarkWant {
		t.Errorf("after resizing back footer = %q", got)
	}
}

// the specification 4 (colours) and 5.
func TestErrorMarkYieldsToIndicators(t *testing.T) {
	_, restore := stubYank(t, nil)
	defer restore()
	m := errorMarkModel(t, 80, 24)
	m = m.logError("Couldn't cast the chart. %s", "x")
	m.yank = yankState{active: true, chars: 5}
	if got := m.footerIndicator(); strings.Contains(got, "error") || got == "" {
		t.Errorf("copy flash should show, got %q", got)
	}
	m.yank = yankState{}
	m.leader.pending = true
	if got := m.footerIndicator(); strings.Contains(got, "error") || got == "" {
		t.Errorf("pending leader should show, got %q", got)
	}
	m.leader.pending = false
	if got := m.footerIndicator(); got != errorMarkWant {
		t.Errorf("mark did not return, got %q", got)
	}
}

// the specification 4: colours on paint the `!` in the error colour.
func TestErrorMarkColour(t *testing.T) {
	forceColor(t)
	on := errorMarkText(",", Config{})
	if !strings.Contains(on, "\x1b[") || ansi.Strip(on) != "! error · ,l" {
		t.Errorf("coloured mark = %q", on)
	}
	if got := errorMarkText(",", Config{Color: ColorNever}); got != "! error · ,l" {
		t.Errorf("colours-off mark = %q", got)
	}
}

// Frames: the specification.
func TestErrorMarkFrames(t *testing.T) {
	forceColor(t)
	for _, c := range []struct {
		name string
		cfg  Config
		w, h int
	}{
		{"footer-error-mark-80x24-never", Config{Color: ColorNever}, 80, 24},
		{"footer-error-mark-80x24-color", Config{}, 80, 24},
		{"footer-error-mark-132x40-never", Config{Color: ColorNever}, 132, 40},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := consoleTasteModel(t, c.cfg, c.w, c.h, 0)
			m.logLines = nil
			m = m.logError("Couldn't cast the chart. %s", "Astrolog could not be started")
			writeTasteGolden(t, c.name, m.View())
		})
	}
}
