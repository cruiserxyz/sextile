package ui

import (
	"fmt"
	"testing"
	"time"
)

// The taste lane of specs/details-follows-its-tab.md (V1-V5), frozen as goldens
// on the operator's choice from previews (2026-11-05). The fixture is
// follow_fixture_test.go's: the frozen clock, Now and Synthetic 1, the Chart
// focused on Now, Details on its report; Now's frame and Chart report are
// committed before the captured message and every later result is held (its
// command is not run) until the frame is captured.

type paneMode struct {
	name string
	cfg  Config
}

var (
	paneColour     = paneMode{"color", Config{Color: ColorAlways}}
	paneAccessible = paneMode{"accessible", Config{Accessible: true}}
)

// paneFixture is the V rows' start, in the mode's colours.
func paneFixture(t *testing.T, mode paneMode, w, h int) (Model, *followStub) {
	t.Helper()
	if mode.cfg.UseColor() {
		forceColor(t)
	}
	return followFixture(t, mode.cfg, w, h)
}

func goldenName(base string, w, h int, mode paneMode) string {
	return fmt.Sprintf("pane-%s-%dx%d-%s", base, w, h, mode.name)
}

// V1. Details empty with `…`: `2` from the Chart, before the Sun's report.
func TestPaneFramesNewSubject(t *testing.T) {
	for _, tc := range []struct {
		w, h int
		mode paneMode
	}{{132, 40, paneColour}, {132, 40, paneAccessible}, {80, 24, paneColour}} {
		t.Run(goldenName("new-subject", tc.w, tc.h, tc.mode), func(t *testing.T) {
			m, _ := paneFixture(t, tc.mode, tc.w, tc.h)
			m, _ = pressOnly(t, m, runes("2"))
			checkGolden(t, goldenName("new-subject", tc.w, tc.h, tc.mode), m.View())
		})
	}
}

// V2. Details faint with the caption ending ` …`: the Sun's report committed,
// `d` pressed, the new moment's frame committed, its report not.
func TestPaneFramesSameSubject(t *testing.T) {
	for _, mode := range []paneMode{paneColour, paneAccessible} {
		t.Run(goldenName("same-subject", 132, 40, mode), func(t *testing.T) {
			m, _ := paneFixture(t, mode, 132, 40)
			m, cmd := pressOnly(t, m, runes("2"))
			m = pump(t, m, cmd)
			m, cmd = pressOnly(t, m, runes("d"))
			frame := ""
			pumpSeen(t, m, 400*time.Millisecond, func(m Model) {
				if frame == "" && m.textPaneState(PanelBodies) == paneCurrent && m.detailsReportState() == paneUpdating {
					frame = m.View()
				}
			}, cmd)
			if frame == "" {
				t.Fatal("no step had the new frame committed and the report held")
			}
			checkGolden(t, goldenName("same-subject", 132, 40, mode), frame)
		})
	}
}

// V3. Bodies and Aspects empty with `…` after the Chart cursor moved to another
// chart, before its frame. (Now is the last row of the list, so the key that
// moves to Synthetic 1 is ↑.)
func TestPaneFramesOtherChart(t *testing.T) {
	m, _ := paneFixture(t, paneColour, 132, 40)
	m, _ = pressOnly(t, m, keyUp)
	checkGolden(t, goldenName("other-chart", 132, 40, paneColour), m.View())
}

// V4. The failure line: `2`, and the Sun's report fails.
func TestPaneFramesFailed(t *testing.T) {
	m, stub := paneFixture(t, paneColour, 132, 40)
	stub.failBodies = true
	m, cmd := pressOnly(t, m, runes("2"))
	m = pump(t, m, cmd)
	if m.detailsReportState() != paneFailed {
		t.Fatalf("fixture: the Sun's report did not fail (%v)", m.detailsReportState())
	}
	checkGolden(t, goldenName("failed", 132, 40, paneColour), m.View())
}

// V5. The Chart report captioned by the chart's name: the floating chart's, and
// an anchored one's with the cursor there.
func TestPaneFramesChartCaption(t *testing.T) {
	m, _ := paneFixture(t, paneColour, 132, 40)
	checkGolden(t, "pane-chart-caption-now-132x40-color", m.View())
	m, cmd := pressOnly(t, m, keyUp)
	m = pump(t, m, cmd)
	if m.detailsReportState() != paneCurrent || m.textPaneState(PanelBodies) != paneCurrent {
		t.Fatalf("fixture: Synthetic 1's frame and report are not drawn (%v, %v)", m.detailsReportState(), m.textPaneState(PanelBodies))
	}
	checkGolden(t, "pane-chart-caption-anchored-132x40-color", m.View())
}
