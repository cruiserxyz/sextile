package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The taste lane its specification.
// Every frame renders through the real Model.View() over a real committed
// chart and a real log buffer, so what is frozen is what the operator sees.
//
// Defaults frozen here: the pending leader as `, …` at the right of the FOOTER
// (operator's call, 2026-08-25 — not the top status bar, which keeps its
// resting slot untouched), the copy confirmation as `yanked`, and the console cursor in whatever
// active-row treatment the run is configured for — the console reuses
// cfg.RowFocus rather than growing a treatment of its own, so the operator's
// existing invert/indicator setting governs it.
// `go test ./internal/ui -update` re-freezes after a deliberate restyle;
// TestConsoleTasteContactSheet writes the alternatives to
// docs/90-assets/t105-*.txt for the pick.
//
// Re-frozen by an earlier change (2026-08-26): the seed became lane-aware and the console
// goldens seed sixty lines instead of forty, so the console — now showing
// only argv+error lines — still overflows at both sizes. The treatments
// frozen here did not change; the lines under them did.

// consoleTasteModel is a ready pair workspace at w×h with a committed chart
// and `lines` log lines behind it, seeded lane-aware: the
// commands are the forty real sweep spawns on the argv lane, with the notes
// and the one error a real session writes among them. The console shows EVERY
// lane and the 6-row Log shows error+note, so every frame here renders
// each view over the lines it would really hold.
//
// Until an earlier change the console filtered the note lane out, on the reasoning that the
// two surfaces between them gave every lane a home. That is true at 144x40 and
// false at 80x24, where there is no Log panel — so these twelve goldens were
// re-frozen when the console became the complete record.
func consoleTasteModel(t *testing.T, cfg Config, w, h, lines int) Model {
	t.Helper()
	m := tabsPairModel(t, cfg)
	m.width, m.height = w, h
	m.limitedViewShown = true // the first-frame notice is not what this fixture is about
	m, _ = applyTextCommit(m, FirstFrameReadyMsg{
		Positions:    fullChartPositions(),
		Aspects:      tasteOwnAspects(),
		CrossAspects: tasteCrossAspects(),
		Overlay:      true,
	})
	m = withSceneColumns(m)
	m.bodyCursorIdx = 2
	for _, l := range consoleTasteLines(t, lines) {
		m = m.appendLog(l.lane, l.text)
	}
	return m
}

// consoleTasteLines is a buffer of the lines Sextile actually writes, in the
// shape a session writes them: two startup notes, then real commands with a
// wheel-publish note every ninth line and a Details failure every
// eighteenth. Each line is exactly what the operator would copy.
func consoleTasteLines(t *testing.T, n int) []logLine {
	t.Helper()
	commands := sweepCommands(t)
	out := make([]logLine, 0, n)
	for i := 0; i < n; i++ {
		switch {
		case i == 0:
			out = append(out, logLine{laneNote, "active city: Berlin, DE (source=stored-location)"})
		case i == 1:
			out = append(out, logLine{laneNote, "anchor chart: 1990-06-15 14:30 Amsterdam, NL"})
		case i%18 == 17:
			out = append(out, logLine{laneError, "details: astrolog exited 1: unknown switch -Yq"})
		case i%9 == 8:
			out = append(out, logLine{laneNote, "wheel: /tmp/sextile-chart.png"})
		default:
			out = append(out, logLine{laneArgv, commands[i%len(commands)]})
		}
	}
	return out
}

func writeTasteGolden(t *testing.T, name, got string) {
	t.Helper()
	assertFrameGeometry(t, got, tasteFrameWidth(got), strings.Count(got, "\n")+1)
	checkGolden(t, name, got)
}

// tasteFrameWidth is the frame's own first-row width; assertFrameGeometry then
// holds every other row to it, which is the tear this guards against.
func tasteFrameWidth(view string) int {
	rows := strings.Split(view, "\n")
	if len(rows) == 0 {
		return 0
	}
	return ansi.StringWidth(rows[0])
}

// V1 — the pending-leader indicator beside the at-rest slot.
func TestLeaderPendingTaste(t *testing.T) {
	forceColor(t)
	for _, cfg := range tasteConfigs() {
		t.Run(cfg.name, func(t *testing.T) {
			m := consoleTasteModel(t, cfg.cfg, 144, 40, 12)
			if got := m.footerIndicator(); got != "" {
				t.Fatalf("the at-rest footer carries %q", got)
			}
			pending, _ := press(t, m, ",")
			writeTasteGolden(t, "leader-pending-"+cfg.name, pending.View())
		})
	}
}

// V4 — the copy confirmation in the status bar's right slot.
func TestYankFlashTaste(t *testing.T) {
	forceColor(t)
	_, restore := stubYank(t, nil)
	defer restore()
	for _, cfg := range tasteConfigs() {
		t.Run(cfg.name, func(t *testing.T) {
			m := consoleTasteModel(t, cfg.cfg, 144, 40, 60)
			m, _ = press(t, m, ",", "l")
			m, _ = press(t, m, "enter")
			if !m.yank.active {
				t.Fatal("the copy did not flash")
			}
			writeTasteGolden(t, "console-yank-flash-"+cfg.name, m.View())
		})
	}
}

func tasteConfigs() []struct {
	name string
	cfg  Config
} {
	return []struct {
		name string
		cfg  Config
	}{
		{"color", Config{}},
		{"accessible", Config{Accessible: true}},
	}
}

// TestConsoleTasteContactSheet writes every alternative to docs/90-assets/
// for the operator's pick. It never fails on content; it is the render step of
// the approval loop.
func TestConsoleTasteContactSheet(t *testing.T) {
	if os.Getenv("SEXTILE_TASTE_SHEET") == "" {
		t.Skip("set SEXTILE_TASTE_SHEET=1 to re-render docs/90-assets/t105-*.txt")
	}
	forceColor(t)
	dir := os.Getenv("SEXTILE_TASTE_SHEET_DIR")
	if dir == "" {
		dir = filepath.Join("..", "..", "docs", "90-assets")
	}

	// V1: the three pending treatments, each beside the at-rest slot.
	var v1 strings.Builder
	for _, cfg := range tasteConfigs() {
		m := consoleTasteModel(t, cfg.cfg, 144, 40, 12)
		fmt.Fprintf(&v1, "== %s / at rest ==\n%s\n\n", cfg.name, footerRowOf(m.View()))
		for _, style := range []struct {
			name string
			mode leaderPendingStyleMode
		}{{"bare", leaderPendingBare}, {"ellipsis", leaderPendingEllipsis}, {"bracketed", leaderPendingBracketed}} {
			prev := leaderPendingStyle
			leaderPendingStyle = style.mode
			pending, _ := press(t, m, ",")
			fmt.Fprintf(&v1, "== %s / pending %s ==\n%s\n\n", cfg.name, style.name, footerRowOf(pending.View()))
			leaderPendingStyle = prev
		}
	}
	writeSheet(t, dir, "t105-v1-leader-pending.txt", v1.String())

	// V2/V3: the console at both sizes. The cursor row is the shared one
	// (the specification), so there is no row treatment to vary.
	var v2 strings.Builder
	for _, cfg := range tasteConfigs() {
		for _, size := range []struct{ w, h int }{{144, 40}, {80, 24}} {
			m := consoleTasteModel(t, cfg.cfg, size.w, size.h, 60)
			m, _ = press(t, m, ",", "l")
			fmt.Fprintf(&v2, "== %s / %dx%d ==\n%s\n\n", cfg.name, size.w, size.h, m.View())
		}
	}
	writeSheet(t, dir, "t105-v2-console.txt", v2.String())

	// V4: the three confirmation treatments.
	_, restore := stubYank(t, nil)
	defer restore()
	var v4 strings.Builder
	for _, cfg := range tasteConfigs() {
		for _, style := range []struct {
			name string
			mode yankFlashStyleMode
		}{{"plain", yankFlashPlain}, {"counted", yankFlashCounted}, {"arrow", yankFlashArrow}} {
			prev := yankFlashStyle
			yankFlashStyle = style.mode
			m := consoleTasteModel(t, cfg.cfg, 144, 40, 60)
			m, _ = press(t, m, ",", "l")
			m, _ = press(t, m, "enter")
			fmt.Fprintf(&v4, "== %s / %s ==\n%s\n\n", cfg.name, style.name, footerRowOf(m.View()))
			yankFlashStyle = prev
		}
	}
	writeSheet(t, dir, "t105-v4-yank-flash.txt", v4.String())
}

// footerRowOf returns the frame's last row: the key bar, which is where the
// pending leader and the copy confirmation now live.
func footerRowOf(view string) string {
	rows := strings.Split(view, "\n")
	return rows[len(rows)-1]
}

func writeSheet(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	t.Logf("wrote %s", path)
}
