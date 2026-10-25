package ui

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/lookup"
)

// the specification C12, N3, N4, N10: a chart call Astrolog warned
// on is refused, and the operator reads the first warning in the Log and every
// warning line in the console. Every failure below is a REAL one: a PATH-local
// Astrolog stand-in writes the warning to stderr, and the refusal travels the
// real lookup seam into the real commit.

const (
	warnOne = "SwissEph file 'se00010s.se1' not found"
	warnTwo = "a second, distinct warning from the host file"
)

// astrologWarning installs an Astrolog stand-in that writes warning (verbatim,
// bytes included) to stderr and then serves body, a POSIX sh fragment.
func astrologWarning(t *testing.T, warning, body string) {
	t.Helper()
	dir := t.TempDir()
	warnPath := filepath.Join(dir, "warning.txt")
	if err := os.WriteFile(warnPath, []byte(warning), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n/bin/cat " + shQuote(warnPath) + " >&2\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "astrolog"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// serveChart replays the repository's constructed chart fixtures.
func serveChart(t *testing.T) string {
	t.Helper()
	fix, err := filepath.Abs(filepath.Join("..", "astrolog", "testdata", "provenance"))
	if err != nil {
		t.Fatal(err)
	}
	return "case \" $* \" in\n  *\" -aa \"*) exec /bin/cat " + shQuote(filepath.Join(fix, "aspects-k.txt")) +
		" ;;\n  *) exec /bin/cat " + shQuote(filepath.Join(fix, "positions-k.txt")) + " ;;\nesac"
}

func consoleTexts(m Model) string {
	var parts []string
	for _, l := range m.consoleLines() {
		parts = append(parts, l.text)
	}
	return strings.Join(parts, "\n")
}

// C12: one subtest per surface — compute, wheel, Details. Each puts the first
// warning in its Log error line and both in the console.
func TestRefusedChartShowsWarningInLog(t *testing.T) {
	warning := warnOne + "\n" + warnTwo + "\n"
	check := func(t *testing.T, m Model, subject string) {
		t.Helper()
		var line string
		for _, l := range errorLines(m) {
			if strings.HasPrefix(l, subject) {
				line = l
			}
		}
		if want := subject + " Astrolog warned: " + warnOne + "."; line != want {
			t.Errorf("Log error line = %q, want %q (all error lines: %q)", line, want, errorLines(m))
		}
		if strings.Contains(strings.Join(m.logPanelLines(), "\n"), warnTwo) {
			t.Errorf("the second warning line belongs to the console, not the 6-row Log")
		}
		console := consoleTexts(m)
		for _, w := range []string{warnOne, warnTwo} {
			if !strings.Contains(console, w) {
				t.Errorf("the console lost the warning line %q:\n%s", w, console)
			}
		}
	}
	t.Run("compute", func(t *testing.T) {
		astrologWarning(t, warning, serveChart(t))
		m := textModel(t)
		m, cmd := m.advanceTime(unitDay, +1)
		m = pump(t, m, cmd)
		check(t, m, "Couldn't cast the chart.")
	})
	t.Run("wheel", func(t *testing.T) {
		astrologWarning(t, warning, serveChart(t))
		m, cmd := modelWithSlot(t).dispatchRenderWheel()
		m = pump(t, m, cmd)
		check(t, m, "Couldn't draw the wheel.")
	})
	t.Run("details", func(t *testing.T) {
		astrologWarning(t, warning, serveChart(t))
		m := detailsTestModel(t)
		msg := runDetailsCmd(t, m)
		m, _ = m.commitRefresh(surfaceDetails, msg)
		check(t, m, "Couldn't read the full report.")
	})
}

// N10: a warning carrying terminal control bytes reaches the Log line and the
// console's stored (and so copied) text with them removed.
func TestWarningTextRendersWithoutControlBytes(t *testing.T) {
	astrologWarning(t, "a\x1b[31mred\x07bell\x1b[0m\nsecond\x1b]0;title\x07line\n", serveChart(t))
	m := textModel(t)
	m, cmd := m.advanceTime(unitDay, +1)
	m = pump(t, m, cmd)
	copied, _, restore := stubYankRecording(t, nil)
	defer restore()
	tm, _ := m.openConsole()
	m = tm.(Model)
	sawLine, sawDetail := false, false
	for i, l := range m.console.snapshot {
		for _, r := range l.text {
			if r < 0x20 || r == 0x7f {
				t.Errorf("snapshot line %d holds control byte %q: %q", i, r, l.text)
				break
			}
		}
		m.console.cursor = i
		_, _ = m.yankConsoleLine()
		if *copied != l.text {
			t.Errorf("copying line %d gave %q, want the stored text %q", i, *copied, l.text)
		}
		sawLine = sawLine || strings.Contains(l.text, "Astrolog warned: aredbell")
		sawDetail = sawDetail || strings.Contains(l.text, "secondline")
	}
	if !sawLine || !sawDetail {
		t.Errorf("the cleaned warning is missing from the console (line %v, detail %v):\n%s", sawLine, sawDetail, consoleTexts(m))
	}
	for _, row := range m.logPanelRows() {
		if strings.Contains(row, "\x07") || strings.Contains(row, "]0;") {
			t.Errorf("the Log row holds a control sequence: %q", row)
		}
	}
}

// N4: a warning in one timeline call drops its rows and names the warning,
// while the other calls' rows stay. The sky search is served; the transit and
// return searches print valid rows AND a warning.
func TestTimelineWarningDropsItsRows(t *testing.T) {
	defaultClock(t)
	dir := t.TempDir()
	for name, content := range map[string]string{"sky": weekSky, "transit": weekTransit, "return": weekReturn} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	script := "#!/bin/sh\ncase \" $* \" in\n" +
		"  *\" -dm \"*) exec /bin/cat " + shQuote(filepath.Join(dir, "sky")) + " ;;\n" +
		"  *\" -tr \"*) echo " + shQuote(warnOne) + " >&2; exec /bin/cat " + shQuote(filepath.Join(dir, "return")) + " ;;\n" +
		"  *\" -t \"*) echo " + shQuote(warnOne) + " >&2; exec /bin/cat " + shQuote(filepath.Join(dir, "transit")) + " ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "astrolog"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	stubTimeline(t, func(req lookup.TimelineRequest) (lookup.TimelineResult, error) {
		return lookup.TimelineWith(context.Background(), req, nil, stubDefaults)
	})
	m := timelineModel(t, Config{Accessible: true})
	m.width = max(m.width, dualColumnMinWidth(m.shownCount()))
	m, cmd := syncTimelineVisited(m)
	m = pump(t, m, cmd)
	if !m.timeline.complete || len(m.timeline.rows) == 0 {
		t.Fatalf("the sky search's rows were not shown: complete=%v rows=%d", m.timeline.complete, len(m.timeline.rows))
	}
	for _, r := range m.timeline.rows {
		if !strings.Contains(r.Command, " -dm") {
			t.Errorf("a row from a warned search is displayed: %q (command %q)", r.Line, r.Command)
		}
	}
	// An October with a clock change runs each search once per offset
	// segment, so a warned search names itself once per segment: what matters
	// is that BOTH warned searches are named, each with the warning.
	named := map[string]bool{}
	for _, l := range errorLines(m) {
		if !strings.Contains(l, "Astrolog warned: "+warnOne) {
			continue
		}
		for _, sw := range []string{"The -tr search", "The -t search"} {
			if strings.Contains(l, sw) {
				named[sw] = true
				break
			}
		}
	}
	if len(named) != 2 {
		t.Errorf("warned searches named with their warning = %v, want the -t and the -tr search: %q", named, errorLines(m))
	}
}

// N14, UI level: a refused call's stdout is chart data. A fake Astrolog prints
// a recognisable position line beside the warning; the line must appear in no
// Log line, no console line, not in the Details state, and not in the frame,
// while the warning does.
func TestRefusedChartLeaksNoPositionsToTheUI(t *testing.T) {
	// The stdout is a FULL, parseable -v position table (the repository's
	// constructed fixture) with the Sun's longitude changed to a value no other
	// fixture carries, so a leak into chart parsing would show as a Sun there.
	const leakText, leakLon = "17Vir41", 150 + 17 + 41.0/60
	fix, err := filepath.Abs(filepath.Join("..", "astrolog", "testdata", "provenance", "positions-k.txt"))
	if err != nil {
		t.Fatal(err)
	}
	body := "/bin/sed 's/24Gem19/" + leakText + "/' " + shQuote(fix)
	surfaces := map[string]func(t *testing.T) Model{
		"compute": func(t *testing.T) Model {
			m := textModel(t)
			m, cmd := m.advanceTime(unitDay, +1)
			return pump(t, m, cmd)
		},
		"wheel": func(t *testing.T) Model {
			m, cmd := modelWithSlot(t).dispatchRenderWheel()
			return pump(t, m, cmd)
		},
		"details": func(t *testing.T) Model {
			m := detailsTestModel(t)
			m, _ = m.commitRefresh(surfaceDetails, runDetailsCmd(t, m))
			return m
		},
	}
	for name, drive := range surfaces {
		t.Run(name, func(t *testing.T) {
			astrologWarning(t, warnOne+"\n", body)
			m := drive(t)
			var all []string
			for _, l := range m.logLines {
				all = append(all, l.text)
			}
			for _, l := range m.consoleLines() {
				all = append(all, l.text)
			}
			all = append(all, fmt.Sprintf("%+v", m.details), m.View())
			joined := strings.Join(all, "\n")
			if strings.Contains(joined, leakText) {
				t.Errorf("a refused call's chart output reached the UI text")
			}
			for _, p := range m.chart.positions {
				if math.Abs(p.Longitude-leakLon) < 0.01 {
					t.Errorf("a refused call's position reached the chart state: %v", p)
				}
			}
			// (The Details fixture is seeded with two positions of its own.)
			if name != "details" && len(m.chart.positions) != 0 {
				t.Errorf("the chart state holds %d positions after a refused call, want none", len(m.chart.positions))
			}
			if !strings.Contains(joined, "Astrolog warned: "+warnOne) {
				t.Errorf("the warning is not shown:\n%s", strings.Join(errorLines(m), "\n"))
			}
		})
	}
}
