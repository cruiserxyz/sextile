package ui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// TestGlyphsFollowTheRegistryName: Sextile's glyph and short name for the
// eighteen objects it presents are found by the registry NAME, never by
// number. The renumbered registry the parser tests carry puts the Sun at 7, the
// Moon at 3 and the North Node at 21; each still prints its own glyph and
// name, and an object Sextile does not present (here the Earth, renamed, and a
// fixed star) prints its registry name and the placeholder.
func TestGlyphsFollowTheRegistryName(t *testing.T) {
	b, err := os.ReadFile("../astrolog/testdata/ho-renumbered.txt")
	if err != nil {
		t.Fatal(err)
	}
	list, err := astrolog.ParseObjectRegistry(string(b))
	if err != nil {
		t.Fatal(err)
	}
	reg := aspect.Registry(list)
	for _, tc := range []struct {
		num         int
		glyph, name string
	}{
		{7, "☉", "Sun"}, {3, "☽", "Moon"}, {21, "☊", "N.Node"},
		{0, panel.PlaceholderGlyph, "Terra"}, {99, panel.PlaceholderGlyph, "Zeta Retic."},
	} {
		o, ok := reg.ByNumber(tc.num)
		if !ok {
			t.Fatalf("the renumbered registry has no object %d", tc.num)
		}
		if got := panel.BodyGlyph(o); got != tc.glyph {
			t.Errorf("object %d (%s): glyph %q, want %q", tc.num, o.Name, got, tc.glyph)
		}
		if got := panel.BodyName(o); got != tc.name {
			t.Errorf("object %d (%s): name %q, want %q", tc.num, o.Name, got, tc.name)
		}
		if tc.num == 99 || tc.num == 0 {
			continue
		}
		row := panel.FormatPositionRowStyled(aspect.Position{Object: o, Longitude: 10}, astrologstyle.BodyRow{}, plainColour, panel.HouseOverlay{})
		name := tc.name
		if tc.num == 21 {
			name = "N.Node m" // the North Node names the form of its call
		}
		if want := fmt.Sprintf("%s %-9s", tc.glyph, name); !strings.HasPrefix(row, want) {
			t.Errorf("object %d (%s): row %q, want it to start with %q", tc.num, o.Name, row, want)
		}
	}

	// The eighteen are exactly the names LegacyName maps to, at any number.
	presented := 0
	for _, snake := range []string{"sun", "moon", "mercury", "venus", "mars", "jupiter", "saturn", "uranus", "neptune", "pluto", "north_node", "south_node", "chiron", "lilith", "ascendant", "midheaven", "descendant", "imum_coeli"} {
		name, ok := aspect.LegacyName(snake)
		if !ok {
			t.Fatalf("no legacy name for %v", snake)
		}
		if panel.BodyGlyph(aspect.Object{Number: 4242, Name: name}) != panel.PlaceholderGlyph {
			presented++
		}
	}
	if presented != 18 {
		t.Errorf("%d of the 18 legacy names have a glyph, want all 18", presented)
	}
	if got := panel.BodyGlyph(aspect.Object{Number: 1, Name: "Ceres"}); got != panel.PlaceholderGlyph {
		t.Errorf("Ceres at the Sun's usual number has the glyph %q, want the placeholder", got)
	}
}

// TestIsAngleIsTheFourAngleCuspPoints: an angle is Astrolog's cusp point of
// house 1, 4, 7 or 10, and AngleHouse names that house; the other eight cusp
// points (a ticked 11th Cusp among them) are plain cusps, not angles, and
// every other object is neither.
func TestIsAngleIsTheFourAngleCuspPoints(t *testing.T) {
	houses := map[int]int{22: 1, 25: 4, 28: 7, 31: 10}
	for n := 0; n <= 40; n++ {
		o := aspect.Object{Number: n, Name: fmt.Sprintf("object %d", n)}
		want := houses[n]
		if got := panel.AngleHouse(o); got != want {
			t.Errorf("object %d: AngleHouse = %d, want %d", n, got, want)
		}
		if got := panel.IsAngle(o); got != (want > 0) {
			t.Errorf("object %d: IsAngle = %v, want %v", n, got, want > 0)
		}
	}
	if panel.IsAngle(aspect.Object{}) || panel.AngleHouse(aspect.Object{}) != 0 {
		t.Errorf("the zero Object is an angle")
	}
}

// TestTickedCuspTakesItsOwnColour: a ticked cusp point's row under the rule
// is drawn in the colour Astrolog gave that cusp point, alone and comparing,
// not in the Ascendant's.
func TestTickedCuspTakesItsOwnColour(t *testing.T) {
	cusps := fullChartCusps()
	ps := withTicked(fullChartPositions(), cusps, 11)
	cusp11 := fixtureObject(t, 32)
	asc := objAscendant
	own := astrologstyle.BodyRow{Body: "\x1b[1;32m", Position: "\x1b[1;32m", House: "\x1b[1;32m"}
	ascStyle := astrologstyle.BodyRow{Body: "\x1b[1;35m", Position: "\x1b[1;35m", House: "\x1b[1;35m"}
	styles := astrologstyle.Bodies{cusp11: own, asc: ascStyle}
	cfg := Config{Color: ColorAlways}
	label := fmt.Sprintf("%d %-9s", 11, "11th")

	row := func(body string) string {
		for _, line := range strings.Split(body, "\n") {
			if strings.Contains(line, "11th") {
				return line
			}
		}
		t.Fatalf("no 11th cusp row in:\n%s", body)
		return ""
	}
	check := func(name, line string) {
		t.Helper()
		if want := panel.RenderSGR(own.Body, label); !strings.Contains(line, want) {
			t.Errorf("%s: the 11th cusp row %q lacks its own colour %q", name, line, want)
		}
		if bad := panel.RenderSGR(ascStyle.Body, label); strings.Contains(line, bad) {
			t.Errorf("%s: the 11th cusp row %q is in the Ascendant's colour", name, line)
		}
	}

	alone, _, _ := renderSingleBodies(ps, cusps, styles, nil, 0, 0, 80, 40, false, cfg, panel.HouseOverlay{})
	check("alone", row(alone))

	state := chartViewState{positions: ps, cusps: cusps, bodyStyles: styles}
	cols := []panel.BodyColumn{{Positions: ps, Cusps: cusps}, {Positions: ps, Cusps: cusps}}
	view := bodiesView{shown: 2, comparing: true, ok: true, cols: cols, focus: 0}
	comparing, _, _ := renderBodiesFor(view, state, nil, 0, 0, 100, 40, false, cfg)
	check("comparing", row(comparing))
}

// TestWaterfallSkipsAnUnresolvedSide: an aspect cell with a side the registry
// could not resolve (the zero Object) takes no colour, though an Earth-Mars
// style exists: object number 0 alone is not an identity.
func TestWaterfallSkipsAnUnresolvedSide(t *testing.T) {
	earth := aspect.Object{Number: 0, Name: "Earth"}
	mars := objMars
	style := astrologstyle.AspectStyle{First: "\x1b[1;31m", Kind: "\x1b[1;32m", Second: "\x1b[1;34m"}
	styles := astrologstyle.Aspects{{First: earth, Second: mars, Kind: kindOf("Con")}: style}
	aspects := []aspect.Aspect{
		{AObject: earth, BObject: mars, Kind: kindOf("Con"), Orb: 1, Power: 9},
		{BObject: mars, Kind: kindOf("Con"), Orb: 1, Power: 8},
	}
	out := panel.RenderAspectWaterfallStyled(aspects, styles, panel.AspectColWidth, 2, Config{Color: ColorAlways})
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("%d lines, want the two cells one above the other:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "\x1b[") {
		t.Errorf("the Earth-Mars cell has no colour: %q", lines[0])
	}
	if strings.Contains(lines[1], "\x1b[") {
		t.Errorf("the unresolved-side cell took Earth's colours: %q", lines[1])
	}
}

// TestLiveRowsAreColoured (live): the rows of objects with no glyph
// carry Astrolog's own colours on screen. Fortune, Vertex and the East Point
// are drawn by the host default and each has the colour of its own row; a
// ticked 11th Cusp is drawn under the rule in the colour Astrolog gave that
// cusp point.
func TestLiveRowsAreColoured(t *testing.T) {
	m, _, _ := ticksStart(t)
	reg := aspect.Registry(m.objectRegistry)
	for _, n := range []int{19, 20, 21} {
		o, ok := reg.ByNumber(n)
		if !ok {
			t.Fatalf("object %d is not in the registry", n)
		}
		if row, ok := m.chart.bodyStyles[o]; !ok || row.Body == "" {
			t.Errorf("%s (object %d) has no Astrolog colour: %+v", o.Name, n, row)
		}
	}

	cm := tickObject(t, chooserOf(t, m), 32)
	cm = pressInChooser(t, cm, tea.KeyMsg{Type: tea.KeyEnter})
	cusp, _ := reg.ByNumber(32)
	own, ok := cm.chart.bodyStyles[cusp]
	if !ok || own.Body == "" {
		t.Fatalf("the ticked %s has no Astrolog colour: %+v", cusp.Name, own)
	}
	body, _, _ := renderBodiesBody(cm.chart, nil, 0, 0, 100, 60, false, Config{Color: ColorAlways})
	want := panel.RenderSGR(own.Body, fmt.Sprintf("%d %-9s", 11, "11th"))
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "11th") {
			if !strings.Contains(line, want) {
				t.Errorf("the 11th cusp row %q lacks its own colour %q", line, want)
			}
			return
		}
	}
	t.Errorf("no 11th cusp row after ticking it:\n%s", body)
}
