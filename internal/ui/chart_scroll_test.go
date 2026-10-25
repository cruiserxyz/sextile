package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/store"
)

// All chart values in this file are constructed synthetic test inputs.

// The chart picker scrolls with its cursor
// (bug report of 2026-10-01).
//
// Before this, the list drew the first rows that fit and the cursor walked on
// past them: the marker vanished, and Enter anchored a chart the operator had
// never seen (on 30 charts at 80x24, twenty presses of ↓ anchored Client 21).
// The property is the one the operator relies on: the row Enter will pick is
// the row on screen with the marker.
func TestChartPickerScrollsWithCursor(t *testing.T) {
	for _, tc := range []struct {
		name  string
		drops []store.ChartDrop
	}{
		{"charts only", nil},
		// Unreadable files reserve rows below the charts, so the window the
		// keys scroll must be the chart block's, not the whole list's.
		{"with unreadable files", []store.ChartDrop{{File: "a.yaml", Reason: "x"}, {File: "b.yaml", Reason: "y"}}},
	} {
		for _, size := range []struct{ w, h int }{{80, 24}, {132, 40}} {
			t.Run(fmt.Sprintf("%s/%dx%d", tc.name, size.w, size.h), func(t *testing.T) {
				m := readyModelWithSlots(t, berlinP)
				m.width, m.height = size.w, size.h
				var charts []store.NamedChart
				for i := 1; i <= 30; i++ {
					charts = append(charts, testNamedChart(fmt.Sprintf("Client %02d", i), berlinP))
				}
				m.picker = chartPickerState{loaded: true, prior: startupReady, charts: charts, drops: tc.drops}
				m.picker.refilter()
				m.startup = startupChartOpen

				press := func(k tea.KeyType, n int) {
					for ; n > 0; n-- {
						u, _ := updateChartModalKey(m, tea.KeyMsg{Type: k})
						m = u.(Model)
					}
				}
				check := func(step string) {
					t.Helper()
					want := m.picker.matches[m.picker.cursor].Name
					out := ansi.Strip(renderChartModal(m.picker, Config{Accessible: true}, m.width, m.height))
					var marked []string
					for _, line := range strings.Split(out, "\n") {
						if i := strings.Index(line, "> "); i >= 0 {
							marked = append(marked, strings.TrimSpace(line[i+len("> "):]))
						}
					}
					if len(marked) != 1 || !strings.HasPrefix(marked[0], want+" ") {
						t.Fatalf("%s: Enter would pick %q, but the marked rows on screen are %q", step, want, marked)
					}
				}

				check("at rest")
				press(tea.KeyDown, 20)
				check("after 20 × ↓")
				press(tea.KeyDown, 40)
				check("at the bottom")
				press(tea.KeyUp, 15)
				check("back up 15")
				press(tea.KeyUp, 40)
				check("back at the top")

				// Typing a filter that keeps the cursor's row in the results
				// re-anchors the window at the top; the cursor's row must
				// still be the marked one on screen (round-3 review).
				press(tea.KeyDown, 20)
				u, _ := updateChartModalKey(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("client")})
				m = u.(Model)
				check("after typing a filter every row matches")

				// Shrinking the terminal while the picker is open must not
				// leave the cursor's row outside the shorter window.
				press(tea.KeyDown, 3)
				m.width, m.height = 80, 16
				check("after the terminal shrank")
			})
		}
	}
}
