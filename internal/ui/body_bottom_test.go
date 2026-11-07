package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// bottomLines splits a rendered popup into its inner lines (the border and the
// padding rows around them removed): the last entry is the bottom line.
func bottomLines(frame string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimRight(frame, "\n"), "\n") {
		l = strings.TrimRight(l, " ")
		if strings.HasPrefix(l, "+-") || strings.HasPrefix(l, "+>") || strings.HasPrefix(l, "╭") || strings.HasPrefix(l, "╰") || strings.HasPrefix(l, "+--") {
			continue
		}
		out = append(out, strings.TrimSpace(strings.Trim(l, "|│ ")))
	}
	// the bottom padding row is blank and last
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// F12: the count and legend are the box's last inner line, a blank buffer line
// above it, the list reaches down to the buffer, and no object name follows the
// legend.
func TestBodyModalBottomLine(t *testing.T) {
	stubProbe(t, undrawable(nil))
	m, cmd := openProbePopup(t, probeModel(t))
	m = pump(t, m, cmd)
	frame := popupText(m)
	lines := bottomLines(frame)
	n := len(lines)
	if n < 4 {
		t.Fatalf("too few lines:\n%s", frame)
	}
	last := lines[n-1]
	if !strings.Contains(last, " selected ") || !strings.HasSuffix(last, " none") {
		t.Errorf("the last inner line = %q, want the count and the legend and nothing after it:\n%s", last, frame)
	}
	if lines[n-2] != "" {
		t.Errorf("the line above the count = %q, want the blank buffer line:\n%s", lines[n-2], frame)
	}
	if !strings.HasPrefix(lines[n-3], "[") {
		t.Errorf("the line above the buffer = %q, want the list to reach it:\n%s", lines[n-3], frame)
	}
	for _, o := range probeRegistry() {
		if strings.Contains(last, o.Name) {
			t.Errorf("the bottom line names %q: %q", o.Name, last)
		}
	}
}

// F12: while the cursor is on an object Astrolog cannot draw, its message takes
// the bottom line and the count is nowhere in the box; one key away the count is
// back.
func TestBodyModalStatusReplacesTheBottomLine(t *testing.T) {
	stubProbe(t, undrawable([]string{"a.se1"}, 5))
	m, cmd := openProbePopup(t, probeModel(t))
	m = pump(t, m, cmd)
	m, cmd = probeKey(m, probeTyped("mars"))
	m = pump(t, m, cmd)
	frame := popupText(m)
	lines := bottomLines(frame)
	if last := lines[len(lines)-1]; last != "Astrolog: SwissEph file 'a.se1' not found in PATH '/x/'" {
		t.Errorf("the last inner line = %q, want the message:\n%s", last, frame)
	}
	if strings.Contains(frame, " selected ") {
		t.Errorf("the count is shown beside the message:\n%s", frame)
	}
	for range len("mars") {
		m, _ = probeKey(m, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	m, _ = probeKey(m, tea.KeyMsg{Type: tea.KeyDown})
	frame = popupText(m)
	if strings.Contains(frame, "can't be drawn") || !strings.Contains(frame, " selected ") {
		t.Errorf("the legend did not come back:\n%s", frame)
	}
}
