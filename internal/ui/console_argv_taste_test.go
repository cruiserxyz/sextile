package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/lookup"
)

// The taste lane its specification.
// Every frame renders through the real Model.View() over a real committed
// chart, and the commands in the console are REAL: forty spawns recorded by
// the real compute path over a real chart sweep, frozen once as a fixture so
// the goldens stay deterministic without a binary.
//
//	SEXTILE_TASTE_SHEET=1 go test ./internal/ui -run TestConsoleArgvSweepFixture
//
// re-records the fixture from the live binary (twenty consecutive days at
// Berlin, two spawns each — positions and aspects); the binary path in each
// line is the recording machine's, which is the point: the line is what ran.
// `go test ./internal/ui -update` re-freezes the goldens after a deliberate
// restyle; TestConsoleArgvTasteContactSheet writes the V2 alternatives to
// docs/90-assets/t111-*.txt for the pick.

const consoleArgvSweepFixture = "console-argv-sweep.commands"

// TestConsoleArgvSweepFixture records the V1 fixture from the real compute
// path. It is the render step of the approval loop, never a verifier: it
// skips unless asked for, and needs the live binary.
func TestConsoleArgvSweepFixture(t *testing.T) {
	if os.Getenv("SEXTILE_TASTE_SHEET") == "" {
		t.Skip("set SEXTILE_TASTE_SHEET=1 to re-record testdata/" + consoleArgvSweepFixture + " from the live binary")
	}
	var lines []string
	start := testFrozenTime()
	for day := 0; day < 20; day++ {
		res, err := lookup.FirstCallTransits(context.Background(), nil, start.AddDate(0, 0, day), testPlaceBerlin(), lookup.Hellenistic)
		if err != nil {
			t.Fatalf("day %d: %v", day, err)
		}
		lines = append(lines, res.Commands...)
	}
	if len(lines) != 40 {
		t.Fatalf("recorded %d commands, want 40 (two per day)", len(lines))
	}
	path := filepath.Join("testdata", consoleArgvSweepFixture)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	t.Logf("wrote %s", path)
}

// sweepCommands reads the frozen V1 fixture: forty real commands.
func sweepCommands(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", consoleArgvSweepFixture))
	if err != nil {
		t.Fatalf("read fixture: %v (SEXTILE_TASTE_SHEET=1 go test ./internal/ui -run TestConsoleArgvSweepFixture records it)", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != 40 {
		t.Fatalf("fixture holds %d commands, want 40", len(lines))
	}
	return lines
}

// consoleArgvModel is a ready pair workspace at w×h with a committed chart
// and the forty real sweep commands behind it, committed two at a time the
// way a sweep commits them, with the notes a real session writes between.
func consoleArgvModel(t *testing.T, cfg Config, w, h int) Model {
	t.Helper()
	m := tabsPairModel(t, cfg)
	m.width, m.height = w, h
	m = m.testNote("active city: Berlin, DE (source=stored-location)")
	m = m.testNote("anchor chart: 1990-06-15 14:30 Amsterdam, NL")
	commands := sweepCommands(t)
	for i := 0; i+1 < len(commands); i += 2 {
		m, _ = applyTextCommit(m, FirstFrameReadyMsg{
			Positions:    fullChartPositions(),
			Aspects:      tasteOwnAspects(),
			CrossAspects: tasteCrossAspects(),
			Overlay:      true,
			Commands:     commands[i : i+2],
		})
		m = withSceneColumns(m)
	}
	m.bodyCursorIdx = 2
	return m
}

// V1 — the console full of real commands, both sizes, cursor at the newest
// and mid-scroll. Truncation at the border is the point of the frame.
func TestConsoleArgvTaste(t *testing.T) {
	forceColor(t)
	for _, cfg := range tasteConfigs() {
		for _, size := range []struct {
			name string
			w, h int
		}{{"144x40", 144, 40}, {"80x24", 80, 24}} {
			for _, where := range []string{"newest", "mid"} {
				t.Run(size.name+"-"+where+"-"+cfg.name, func(t *testing.T) {
					m := consoleArgvModel(t, cfg.cfg, size.w, size.h)
					m, _ = press(t, m, ",", "l")
					if where == "mid" {
						for i := 0; i < 20; i++ {
							m, _ = press(t, m, "up")
						}
					}
					writeTasteGolden(t, "console-argv-"+size.name+"-"+where+"-"+cfg.name, m.View())
				})
			}
		}
	}
}

// consoleArgvErrorModel is V2's buffer: one error line between two real
// commands — a chart compute that failed after its spawn, then the next
// frame that succeeded — opened as a console at 144x40.
func consoleArgvErrorModel(t *testing.T, cfg Config) Model {
	t.Helper()
	m := tabsPairModel(t, cfg)
	m.width, m.height = 144, 40
	commands := sweepCommands(t)
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{
		Err:      errors.New("astrolog exited 1: unknown switch -Yq"),
		Commands: commands[:1],
	})
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{
		Positions: fullChartPositions(), Aspects: tasteOwnAspects(), CrossAspects: tasteCrossAspects(), Overlay: true,
		Commands: commands[2:3],
	})
	m = withSceneColumns(m)
	m.bodyCursorIdx = 2
	m, _ = press(t, m, ",", "l")
	return m
}

// V2 — how an error reads among commands: red, or behind `! ` without
// colour (the specification A1, A2).
func TestConsoleArgvErrorTaste(t *testing.T) {
	forceColor(t)
	for _, cfg := range tasteConfigs() {
		t.Run(cfg.name, func(t *testing.T) {
			m := consoleArgvErrorModel(t, cfg.cfg)
			if got := len(m.console.snapshot); got != 3 {
				t.Fatalf("snapshot holds %d lines, want argv, error, argv", got)
			}
			writeTasteGolden(t, "console-argv-error-"+cfg.name, m.View())
		})
	}
}

// V3 — the 6-row Log with the commands gone: the default corner panel after
// a session that has computed twenty times.
func TestLogPanelNoArgvTaste(t *testing.T) {
	forceColor(t)
	for _, cfg := range tasteConfigs() {
		t.Run(cfg.name, func(t *testing.T) {
			m := consoleArgvModel(t, cfg.cfg, 144, 40)
			if strings.Contains(m.View(), "-qj") || strings.Contains(m.View(), "-qb") {
				t.Fatal("a command reached the closed frame")
			}
			writeTasteGolden(t, "log-panel-no-argv-"+cfg.name, m.View())
		})
	}
}
