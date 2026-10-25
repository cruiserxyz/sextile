package ui

import (
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// fullChartPositions is a real Astrolog chart: the 14 planets/points and the
// 4 angles Sextile's Hellenistic lens emits (captured from the same
// 1990-06-15 Berlin chart as testdata/influence-chart.report).
func fullChartPositions() []aspect.Position {
	return []aspect.Position{
		{Body: aspect.Sun, Longitude: 84.2167, Speed: 0.9690, House: 8},
		{Body: aspect.Moon, Longitude: 346.7500, Speed: 1.0160, House: 5},
		{Body: aspect.Mercury, Longitude: 65.8667, Speed: 1.7450, House: 8},
		{Body: aspect.Venus, Longitude: 48.9000, Speed: 1.1930, House: 7},
		{Body: aspect.Mars, Longitude: 11.1000, Speed: 1.3710, House: 6},
		{Body: aspect.Jupiter, Longitude: 105.9000, Speed: 2.6080, House: 9},
		{Body: aspect.Saturn, Longitude: 294.0167, Speed: -1.7550, House: 3},
		{Body: aspect.Uranus, Longitude: 278.1500, Speed: -3.3200, House: 3},
		{Body: aspect.Neptune, Longitude: 283.7000, Speed: -4.1870, House: 3},
		{Body: aspect.Pluto, Longitude: 225.3833, Speed: -5.0650, House: 1},
		{Body: aspect.Chiron, Longitude: 106.0667, Speed: 5.2110, House: 9},
		{Body: aspect.NorthNode, Longitude: 308.1167, Speed: -0.2230, House: 4},
		{Body: aspect.SouthNode, Longitude: 128.1167, Speed: -0.2230, House: 10},
		{Body: aspect.Lilith, Longitude: 259.1167, Speed: 8.8830, House: 2},
		{Body: aspect.Ascendant, Longitude: 210.8667, Speed: 0.6890, House: 1},
		{Body: aspect.ImumCoeli, Longitude: 312.0667, Speed: 0.9940, House: 4},
		{Body: aspect.Descendant, Longitude: 30.8667, Speed: 0.6890, House: 7},
		{Body: aspect.Midheaven, Longitude: 132.0667, Speed: 0.9940, House: 10},
	}
}

const (
	fullChartPlanets = 14
	fullChartAngles  = 4
)

// fullChartCusps is a house frame to go with fullChartPositions: the four
// angle houses carry the chart's angles, the rest are made-up fixture values
// (Sextile never computes a cusp; real ones are Astrolog's).
func fullChartCusps() aspect.Cusps {
	return aspect.Cusps{210.8667, 240.5, 270.25, 312.0667, 340.75, 10.5, 30.8667, 60.5, 100.25, 132.0667, 160.75, 185.5}
}

const fullChartCuspRows = 12

// C21 (reversed by the specification sentences 3 and 5: the angle
// block is no longer pinned) — the rule sits on the row immediately above the
// first cusp row, wherever the window has scrolled to.
func TestBodiesRuleAboveCusps(t *testing.T) {
	state := chartViewState{positions: withTicked(fullChartPositions(), fullChartCusps(), allPlainCusps...), cusps: fullChartCusps()}
	for _, innerHeight := range []int{3, 6, 10, 14, 18, 19, 26, 40} {
		body, ruleRow, _ := renderBodiesBody(state, nil, 0, 0, 64, innerHeight, false, Config{Accessible: true})
		rows := strings.Split(body, "\n")
		if len(rows) > innerHeight {
			t.Errorf("innerHeight %d: rendered %d rows", innerHeight, len(rows))
		}
		if innerHeight <= fullChartPlanets {
			// Cursor on row 0: the window is the top of the planets.
			if ruleRow != -1 {
				t.Errorf("innerHeight %d: rule at %d with the window still above it", innerHeight, ruleRow)
			}
			continue
		}
		if ruleRow != fullChartPlanets {
			t.Fatalf("innerHeight %d: rule at %d, want %d planet rows above it", innerHeight, ruleRow, fullChartPlanets)
		}
		if rows[ruleRow] != "" {
			t.Errorf("innerHeight %d: rule row %d is not the blank the post-pass overlays: %q", innerHeight, ruleRow, rows[ruleRow])
		}
		if !strings.Contains(rows[ruleRow+1], bodyName(aspect.Ascendant)) {
			t.Errorf("innerHeight %d: row below the rule = %q, want the Asc row", innerHeight, rows[ruleRow+1])
		}
		for _, row := range rows[:ruleRow] {
			if strings.Contains(row, " "+bodyName(aspect.Ascendant)+" ") {
				t.Errorf("innerHeight %d: an angle row appears above the rule: %q", innerHeight, row)
			}
		}
	}
}

// C22 (reversed by sentences 3 and 5) — the rule costs exactly one display
// row of the window: with it in view the panel shows one body row fewer.
func TestRuleCostsOneRow(t *testing.T) {
	state := chartViewState{positions: withTicked(fullChartPositions(), fullChartCusps(), allPlainCusps...), cusps: fullChartCusps()}
	for _, innerHeight := range []int{16, 20, 26} {
		body, ruleRow, scroll := renderBodiesBody(state, nil, 0, fullChartPlanets, 64, innerHeight, false, Config{Accessible: true})
		rows := strings.Split(body, "\n")
		if ruleRow < 0 {
			t.Fatalf("innerHeight %d: cursor on the first cusp row but no rule in the window", innerHeight)
		}
		if len(rows) != min(innerHeight, fullChartPlanets+fullChartCuspRows+1) {
			t.Errorf("innerHeight %d: %d rendered rows", innerHeight, len(rows))
		}
		if scroll.Visible != len(rows)-1 {
			t.Errorf("innerHeight %d: %d body rows visible beside the rule, rendered %d rows", innerHeight, scroll.Visible, len(rows))
		}
	}
}

// C23 / C25 (reversed by sentences 3 and 5) — the thumb measures the visible
// window over EVERY row, planets and cusps alike; nothing is pinned.
func TestBodiesThumbMeasuresAllRows(t *testing.T) {
	state := chartViewState{positions: withTicked(fullChartPositions(), fullChartCusps(), allPlainCusps...), cusps: fullChartCusps()}
	total := fullChartPlanets + fullChartCuspRows
	// Cursor on the very last row: the window has scrolled to the end, rule
	// included in the display list when it is still in view.
	body, _, scroll := renderBodiesBody(state, nil, 0, total-1, 64, 14, false, Config{Accessible: true})
	if scroll.Total != total || scroll.Offset+scroll.Visible != total {
		t.Fatalf("viewport = %+v, want Total %d ending at the last row", scroll, total)
	}
	rows := strings.Split(body, "\n")
	if !strings.Contains(rows[len(rows)-1], "12") {
		t.Errorf("last row of the window = %q, want cusp 12", rows[len(rows)-1])
	}
	if !scroll.overflows() {
		t.Errorf("a 14-row window over %d rows does not report overflow: %+v", total, scroll)
	}
}

// C25 (reversed by sentences 3 and 5) — the viewport counts body rows and
// never the rule's blank row, at every height and cursor.
func TestRuleRowIsNotCounted(t *testing.T) {
	state := chartViewState{positions: withTicked(fullChartPositions(), fullChartCusps(), allPlainCusps...), cusps: fullChartCusps()}
	for _, innerHeight := range []int{3, 6, 8, 10, 14, 18, 19, 20, 26, 40} {
		for cursor := 0; cursor < fullChartPlanets+fullChartCuspRows; cursor++ {
			body, ruleRow, scroll := renderBodiesBody(state, nil, 0, cursor, 64, innerHeight, false, Config{Accessible: true})
			rows := strings.Split(body, "\n")
			want := len(rows)
			if ruleRow >= 0 {
				want--
			}
			if scroll.Visible != want || scroll.Total != fullChartPlanets+fullChartCuspRows {
				t.Fatalf("innerHeight %d cursor %d: viewport %+v for %d rows (rule %d)", innerHeight, cursor, scroll, len(rows), ruleRow)
			}
		}
	}
}

// C24 — no cusp rows, or a panel too short to hold the rule beside a row,
// means no rule and a thumb measured over the full row list.
func TestBodiesFallbackHasNoRule(t *testing.T) {
	planetsOnly := fullChartPositions()[:fullChartPlanets]
	body, ruleRow, scroll := renderBodiesBody(chartViewState{positions: planetsOnly}, nil, 0, 13, 64, 9, false, Config{Accessible: true})
	if ruleRow != -1 {
		t.Errorf("no angles: rule at %d, want none", ruleRow)
	}
	if strings.Count(body, "\n")+1 != 9 {
		t.Errorf("no angles: rendered %d rows, want 9", strings.Count(body, "\n")+1)
	}
	if want := (viewport{Total: 14, Visible: 9, Offset: 5}); scroll != want {
		t.Errorf("no angles: viewport = %+v, want %+v", scroll, want)
	}

	// Too short for the rule beside a row: it needs innerHeight >= 3. Without
	// a frame the rows under it are the four angles.
	state := chartViewState{positions: fullChartPositions()}
	for _, innerHeight := range []int{1, 2} {
		body, ruleRow, scroll = renderBodiesBody(state, nil, 0, 0, 64, innerHeight, false, Config{Accessible: true})
		if ruleRow != -1 {
			t.Errorf("innerHeight %d: rule at %d, want none", innerHeight, ruleRow)
		}
		rows := strings.Split(body, "\n")
		if len(rows) != innerHeight {
			t.Errorf("innerHeight %d: rendered %d rows", innerHeight, len(rows))
		}
		if scroll.Total != fullChartPlanets+fullChartAngles {
			t.Errorf("innerHeight %d: viewport counts %d rows, want the full %d", innerHeight, scroll.Total, fullChartPlanets+fullChartAngles)
		}
		if scroll.Visible != len(rows) {
			t.Errorf("innerHeight %d: viewport shows %d, rendered %d", innerHeight, scroll.Visible, len(rows))
		}
	}

	// Degenerate geometry still renders nothing and reports nothing.
	for _, tc := range []struct{ w, h int }{{0, 10}, {64, 0}, {-1, -1}} {
		body, ruleRow, scroll = renderBodiesBody(state, nil, 0, 0, tc.w, tc.h, false, Config{Accessible: true})
		if body != "" || ruleRow != -1 || scroll.overflows() {
			t.Errorf("%dx%d: body=%q rule=%d scroll=%+v", tc.w, tc.h, body, ruleRow, scroll)
		}
	}
}

// C26 — the Chart panel is as tall as its content for every workspace size up
// to its cap, so it reports "fits" and its frame is byte-identical to today's;
// past the cap it stays that tall and windows (the specification C21).
func TestChartViewportNeverOverflows(t *testing.T) {
	for slots := 1; slots <= 12; slots++ {
		rects := layoutGeometry(132, 60, slots, 1)
		if rects == nil {
			t.Fatalf("%d slots: no layout at 132x60", slots)
		}
		chartR := rects[PanelChart]
		v := viewport{Total: slots, Visible: chartR.h - 2}
		if slots > chartRowsTall {
			// The panel stopped growing (the specification C21): the list
			// windows, and the viewport says so instead of the box overflowing.
			if chartR.h-2 != chartRowsTall || !v.overflows() {
				t.Errorf("%d slots: chart rows %d, overflows %v, want the panel held at %d rows and reporting its window", slots, chartR.h-2, v.overflows(), chartRowsTall)
			}
			continue
		}
		if v.overflows() {
			t.Errorf("%d slots: chart viewport %+v overflows", slots, v)
		}
		body := strings.TrimSuffix(strings.Repeat("slot\n", slots), "\n")
		for _, cfg := range []Config{{Accessible: true}, {Color: ColorAlways}} {
			for _, focused := range []bool{false, true} {
				box := renderTitledBox("[1] Chart", chartR.w, chartR.h, focused, cfg, body)
				if got := withScrollIndicator(box, v, cfg, focused); got != box {
					t.Errorf("%d slots cfg=%+v focused=%v: the Chart frame changed", slots, cfg, focused)
				}
			}
		}
	}
}
