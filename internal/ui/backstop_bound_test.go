package ui

import (
	"fmt"
	"strings"
	"testing"
)

// The date backstop's line has the bound the other accounts have: few bodies
// keep the names, many read a count, and no line outgrows the Log.
func TestBackstopLineIsBounded(t *testing.T) {
	date := strings.TrimSpace(civilDate(1500, 3, 15))
	m := errLaneModel(t)
	last := func(m Model) string { return m.logLines[len(m.logLines)-1].text }

	few := m.reportUnavailable([]string{"Eris", "Io"}, tUnavailable)
	if got, want := last(few), "Eris, Io can't be drawn: no ephemeris data for "+date; got != want {
		t.Errorf("two bodies: %q, want %q", got, want)
	}

	var names []string
	for i := 1; i <= 12; i++ {
		names = append(names, fmt.Sprintf("Asteroid %d", i))
	}
	many := m.reportUnavailable(names, tUnavailable)
	if got, want := last(many), "12 bodies can't be drawn: no ephemeris data for "+date; got != want {
		t.Errorf("twelve bodies: %q, want %q", got, want)
	}

	// Three long names that do not fit the Log read a count too.
	long := []string{strings.Repeat("A", 30), strings.Repeat("B", 30), strings.Repeat("C", 30)}
	if got, want := last(m.reportUnavailable(long, tUnavailable)), "3 bodies can't be drawn: no ephemeris data for "+date; got != want {
		t.Errorf("three long names: %q, want %q", got, want)
	}

	for _, line := range []string{last(few), last(many)} {
		if frame := errLaneModel(t).testError("%s", line).View(); !strings.Contains(frame, line) {
			t.Errorf("the Log truncated %q", line)
		}
	}
}
