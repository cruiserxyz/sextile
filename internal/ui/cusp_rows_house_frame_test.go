package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// the specification: a ticked plain cusp reads like an angle.

// framePrefix is a house frame without its trailing padding.
func framePrefix(h int) string {
	return strings.TrimRight(houseQuadrant(aspect.House(h)), " ")
}

// Acceptance 1 and 4: `<n> <ordinal>`, the position, then the frame with n in
// its quadrant; with colours off no escape, and colours on change no text.
func TestCuspRowReadsLikeAnAngle(t *testing.T) {
	cusps := fullChartCusps()
	state := chartViewState{positions: withTicked(fullChartPositions(), cusps, 2, 3), cusps: cusps}
	render := func(cfg Config) []string {
		body, rule, _ := renderBodiesBody(state, nil, 0, 0, 80, 40, false, cfg)
		return underRule(t, body, rule)
	}
	rows := render(Config{Color: ColorNever})
	// Under the rule: Asc, 2, 3, IC, Dsc, MC.
	for i, h := range map[int]int{1: 2, 2: 3} {
		row := rows[i]
		if !strings.HasPrefix(row, fmtCuspHead(h)) {
			t.Errorf("cusp %d row = %q, want it to start with %q", h, row, fmtCuspHead(h))
		}
		at, ok := strings.CutPrefix(row[strings.Index(row, cuspText(cusps[h-1])):], cuspText(cusps[h-1]))
		if !ok || !strings.HasSuffix(strings.TrimRight(row, " "), framePrefix(h)) {
			t.Errorf("cusp %d row %q does not end in its house frame %q (after position: %q)", h, row, framePrefix(h), at)
		}
		if strings.Contains(row, "\x1b") {
			t.Errorf("cusp %d row carries an escape with colours off: %q", h, row)
		}
	}
	forceColor(t)
	colored := render(Config{Color: ColorAlways})
	for i := range rows {
		if got := ansi.Strip(colored[i]); got != rows[i] {
			t.Errorf("row %d text changes with colours on: %q vs %q", i+1, got, rows[i])
		}
	}
}

func fmtCuspHead(h int) string {
	return map[int]string{2: "2 2nd", 3: "3 3rd", 11: "11 11th", 12: "12 12th"}[h]
}

// Acceptance 2: a two-digit cusp widens the glyph column, and every other row
// right-aligns its one-cell glyph in it; with none, rows are as before.
func TestTwoDigitCuspWidensTheGlyphColumn(t *testing.T) {
	cusps := fullChartCusps()
	cfg := Config{Color: ColorNever}
	col := func(row, name string) int { return strings.Index(row, name) }
	body := func(houses ...int) string {
		state := chartViewState{positions: withTicked(fullChartPositions(), cusps, houses...), cusps: cusps}
		b, _, _ := renderBodiesBody(state, nil, 0, 0, 80, 40, false, cfg)
		return b
	}
	wide := strings.Split(body(2, 11), "\n")
	plain := strings.Split(body(2), "\n")
	nameCol := func(rows []string) map[int]bool {
		seen := map[int]bool{}
		for _, r := range rows {
			for _, n := range []string{"Sun ", "Asc ", "2nd", "11th", "MC "} {
				if c := col(r, n); c >= 0 {
					seen[ansi.StringWidth(r[:c])] = true
				}
			}
		}
		return seen
	}
	if got := nameCol(wide); len(got) != 1 {
		t.Errorf("names are in %d columns with cusp 11 shown, want one: %v\n%s", len(got), got, strings.Join(wide, "\n"))
	}
	var sunWide, sunPlain string
	for _, r := range wide {
		if strings.Contains(r, "Sun ") {
			sunWide = r
		}
	}
	for _, r := range plain {
		if strings.Contains(r, "Sun ") {
			sunPlain = r
		}
	}
	if strings.TrimRight(ansi.Strip(sunWide), " ") != " "+strings.TrimRight(ansi.Strip(sunPlain), " ") {
		t.Errorf("Sun row with cusp 11 = %q, want one space more than %q", sunWide, sunPlain)
	}
	if strings.Contains(strings.Join(wide, "\n"), "11 11th") == false {
		t.Errorf("cusp 11 row missing:\n%s", strings.Join(wide, "\n"))
	}
	for _, r := range plain {
		if strings.HasPrefix(ansi.Strip(r), " ") && strings.Contains(r, "Sun ") {
			t.Errorf("without a two-digit cusp the Sun row moved: %q", r)
		}
	}
}

// Acceptance 3: the comparing columns carry the frame, each column its own
// chart's cusp.
func TestCuspRowInComparingColumns(t *testing.T) {
	cusps := fullChartCusps()
	other := fullChartCusps()
	for i := range other {
		other[i] = float64(int(other[i]+97) % 360)
	}
	state := chartViewState{positions: withTicked(fullChartPositions(), cusps, 2), cusps: cusps}
	cols := []bodyColumn{
		{positions: state.positions, cusps: cusps},
		{positions: state.positions, cusps: other},
	}
	view := bodiesView{shown: 2, comparing: true, ok: true, cols: cols, focus: 0}
	body, rule, _ := renderBodiesFor(view, state, nil, 0, 0, 100, 40, false, Config{Color: ColorNever})
	row := underRule(t, body, rule)[1]
	if !strings.HasPrefix(row, "2 2nd") {
		t.Errorf("comparing cusp row = %q, want it to start with %q", row, "2 2nd")
	}
	a, b := strings.Index(row, cuspText(cusps[1])), strings.Index(row, cuspText(other[1]))
	if a < 0 || b < 0 || a >= b {
		t.Errorf("row %q should read chart 1's %q then chart 2's %q", row, cuspText(cusps[1]), cuspText(other[1]))
	}
	if !strings.HasSuffix(strings.TrimRight(row, " "), framePrefix(2)) {
		t.Errorf("comparing cusp row %q does not end in house 2's frame %q", row, framePrefix(2))
	}
	if strings.Contains(row, "\x1b") {
		t.Errorf("escape with colours off: %q", row)
	}
}
