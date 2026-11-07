package ui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The launcher says how it started Sextile in SEXTILE_LAUNCH; the large log's
// first line names it, so the console always says what ran. The Log panel does
// not: how Sextile was started is its own working.
func TestLaunchModeIsTheLogsFirstLine(t *testing.T) {
	cases := []struct {
		launch string
		want   string // the first Log line; empty: the Log stays empty
	}{
		{"no-docker v0.1.0", "launcher: without Docker, build v0.1.0"},
		{"no-docker v0.1.0-3-gabc1234-dirty", "launcher: without Docker, build v0.1.0-3-gabc1234-dirty"},
		{"docker localhost/sextile:v0.1.0", "launcher: in Docker, image localhost/sextile:v0.1.0"},
		{"", ""},
		{"docker", ""},
		{"docker ", ""},
		{"no-docker", ""},
		{"no-docker  ", ""},
		{"native v0.1.0", ""},
		{"Docker localhost/sextile:v0.1.0", ""},
		{"no-docker v0.1.0\nsecond", ""},
		{"docker img\x1b[31m", ""},
		{"docker img\u202egpj.exe", ""},
		{"no-docker v0.1.0\u200b", ""},
		{"no-docker v0.1.0 extra", ""},
		{"no-docker vé", ""},
	}
	for _, c := range cases {
		m := newTestModel(t, Config{Launch: c.launch, InstanceLockUnsupported: true}, RealClock{})
		var first string
		if len(m.logLines) > 0 {
			first = m.logLines[0].text
		}
		if c.want == "" {
			if strings.HasPrefix(first, "launcher:") {
				t.Errorf("Launch %q wrote %q, want no launcher line", c.launch, first)
			}
			continue
		}
		if first != c.want {
			t.Errorf("Launch %q: first Log line = %q, want %q", c.launch, first, c.want)
		}
		if m.logLines[0].lane != laneTrace {
			t.Errorf("Launch %q: the line is on lane %v, want the trace lane", c.launch, m.logLines[0].lane)
		}
		// It comes before every other startup line, and the Log panel shows neither.
		if len(m.logLines) != 2 || !strings.Contains(m.logLines[1].text, lockUnsupportedN) {
			t.Errorf("Launch %q: the lock note must follow it: %+v", c.launch, m.logLines)
		}
		var tm tea.Model = m
		tm, _ = tm.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
		if strings.Contains(csText(tm.(Model).View()), "launcher:") {
			t.Errorf("Launch %q: the Log panel shows the launcher line at 132x40", c.launch)
		}
	}
	if n := len(newTestModel(t, Config{}, RealClock{}).logLines); n != 0 {
		t.Errorf("a plain run (Launch unset) wrote %d Log lines, want none", n)
	}
}

// The one-time copy of 0.1.1's state files is Sextile's housekeeping: the
// large log says what it did, the Log panel says nothing (operator,
// 2026-11-05: "a user has no relation to this whatsoever").
func TestStateCopyIsOnlyInTheLargeLog(t *testing.T) {
	const line = "copied session from the old places, not copied, already there: session (hand-run). The old files were left as they are"
	m := newTestModel(t, Config{Migration: line}, RealClock{})
	var trace, other []string
	for _, l := range m.logLines {
		if l.lane == laneTrace {
			trace = append(trace, l.text)
		} else {
			other = append(other, l.text)
		}
	}
	if len(trace) != 1 || trace[0] != "state: "+line {
		t.Errorf("trace lane = %q, want the one state line", trace)
	}
	if len(other) != 0 {
		t.Errorf("the state copy reached the Log panel's lanes: %q", other)
	}
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
	if v := csText(tm.(Model).View()); strings.Contains(v, "old places") {
		t.Errorf("the Log panel shows the state copy at 132x40:\n%s", v)
	}
	if !slices.Contains(consoleLineTexts(tm.(Model)), "state: "+line) {
		t.Errorf("the large log lacks the state line: %q", consoleLineTexts(tm.(Model)))
	}
}
