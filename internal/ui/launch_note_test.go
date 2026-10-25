package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The launcher says how it started Sextile in SEXTILE_LAUNCH; the Log's first
// line names it, so a frame always says what ran.
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
		m := NewModel(Config{Launch: c.launch, InstanceLockUnsupported: true})
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
		if m.logLines[0].lane != laneNote {
			t.Errorf("Launch %q: the line is on lane %v, want the note lane", c.launch, m.logLines[0].lane)
		}
		// It comes before every other startup note, and shows in the 6-row Log.
		if len(m.logLines) != 2 || !strings.Contains(m.logLines[1].text, lockUnsupportedN) {
			t.Errorf("Launch %q: the lock note must follow it: %q", c.launch, m.logLines)
		}
		var tm tea.Model = m
		tm, _ = tm.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
		if len(c.want) < 50 && !strings.Contains(csText(tm.(Model).View()), c.want) {
			t.Errorf("Launch %q: the Log does not show %q at 132x40", c.launch, c.want)
		}
	}
	if n := len(NewModel(Config{}).logLines); n != 0 {
		t.Errorf("a plain run (Launch unset) wrote %d Log lines, want none", n)
	}
}
