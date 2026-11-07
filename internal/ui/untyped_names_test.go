package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Operator, interview 2026-11-04 (decisions-while-away): a Bodies row for an
// object Sextile has no short name for prints the object's full name as
// Astrolog's registry gives it, cut to the nine-cell name column with the
// repository's mark (C11, TestTruncationIsMarked). A name that fits prints
// whole; an object with a glyph keeps Sextile's own name. Both row layouts, colours on
// and off, and the accessible mode whose mark is ASCII.
func TestUntypedRowPrintsItsFullName(t *testing.T) {
	obj := func(n int) aspect.Object {
		o, err := fixtureObjectNumbered(n)
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	for _, mode := range []struct {
		name  string
		cfg   Config
		ascii bool
	}{
		{"colours on", Config{Color: ColorAlways}, false},
		{"colours off", Config{Color: ColorNever}, false},
		{"accessible", Config{Accessible: true}, true},
	} {
		for _, c := range []struct {
			what string
			p    aspect.Position
			// the nine-cell name column, exactly as it must print; a cut name
			// ends in the mark ("…", or "..." in accessible mode), so the
			// accessible cell keeps two cells fewer of the name.
			cell, asciiCell string
		}{
			{"Fortune", aspect.Position{Object: obj(19), Longitude: 306.25}, "Fortune  ", "Fortune  "},
			{"Ceres", aspect.Position{Object: obj(12), Longitude: 113.93}, "Ceres    ", "Ceres    "},
			{"11th Cusp fits exactly", aspect.Position{Object: obj(32), Longitude: 124.17}, "11th Cusp", "11th Cusp"},
			{"East Point is cut and marked", aspect.Position{Object: obj(21), Longitude: 301.25}, "East Poi…", "East P..."},
			{"Rigil Kent. is cut and marked", aspect.Position{Object: obj(86), Longitude: 239.12}, "Rigil Ke…", "Rigil ..."},
			{"a typed Sun keeps Sextile's name", aspect.Position{Object: objSun, Longitude: 84.2167}, "Sun      ", "Sun      "},
		} {
			cell := c.cell
			if mode.ascii {
				cell = c.asciiCell
			}
			if w := ansi.StringWidth(cell); w != 9 {
				t.Fatalf("%s: fixture cell %q is %d cells, want the nine-cell name column", c.what, cell, w)
			}
			glyph := panel.PositionGlyph(c.p)
			want := glyph + " " + cell + " " + cuspText(c.p.Longitude)

			row := ansi.Strip(panel.FormatPositionRowStyled(c.p, astrologstyle.BodyRow{}, mode.cfg, panel.HouseOverlay{}))
			if !strings.HasPrefix(row, want) {
				t.Errorf("%s, %s, one chart: row %q does not start with %q", mode.name, c.what, row, want)
			}

			cols := []panel.BodyColumn{{Positions: []aspect.Position{c.p}}, {Positions: []aspect.Position{c.p}}}
			comparing := ansi.Strip(panel.FormatColumnRow(c.p, astrologStyleZero, cols, 0, panel.WholeWindow(len(cols)), mode.cfg, panel.HouseOverlay{}))
			if !strings.HasPrefix(comparing, glyph+" "+cell) {
				t.Errorf("%s, %s, comparing: row %q does not start with %q", mode.name, c.what, comparing, glyph+" "+cell)
			}
			// Nothing else in the row moved: the position cell follows the
			// nine-cell name column in both layouts, at the same offset.
			at := strings.Index(comparing, cuspText(c.p.Longitude))
			if at < 0 || ansi.StringWidth(comparing[:at]) != ansi.StringWidth(glyph+" "+cell)+1 {
				t.Errorf("%s, %s, comparing: position cell not one cell after the name column in %q", mode.name, c.what, comparing)
			}
		}
	}
}

// The frame the operator chose (interview 2026-11-04): the Bodies panel at
// 80x24 with the objects Sextile has no short name for, in the two styles that
// differ here (the mark of a cut name is "…", or "..." in accessible mode).
// East Point is the cut name; the eleventh cusp the operator ticked is a plain
// cusp row under the rule and keeps its own "11th" label.
func TestBodiesUntypedNamesFrames(t *testing.T) {
	forceColor(t)
	obj := func(n int) aspect.Object {
		o, err := fixtureObjectNumbered(n)
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	cusps := fullChartCusps()
	positions := withTicked([]aspect.Position{
		{Object: objSun, Longitude: 84.2167, Speed: 0.9690, House: 8},
		{Object: objMoon, Longitude: 346.7500, Speed: 1.0160, House: 5},
		{Object: obj(19), Longitude: 306.25, Speed: 0, House: 3},
		{Object: obj(12), Longitude: 113.9333, Speed: 0.2100, House: 9},
		{Object: obj(21), Longitude: 301.25, Speed: 0, House: 3},
		{Object: objAscendant, Longitude: 210.8667, Speed: 0.6890, House: 1},
		{Object: objNadir, Longitude: 312.0667, Speed: 0.9940, House: 4},
		{Object: objDescendant, Longitude: 30.8667, Speed: 0.6890, House: 7},
		{Object: objMidheaven, Longitude: 132.0667, Speed: 0.9940, House: 10},
	}, cusps, 11)
	for _, mode := range []struct {
		name string
		cfg  Config
	}{{"color", Config{Color: ColorAlways}}, {"accessible", Config{Accessible: true}}} {
		m := frameModel(t, mode.cfg, 80, 24, 1, 1)
		m.slots.ToggleSlotSelection(1) // alone: the comparing pair is deselected
		if m.comparing() {
			t.Fatal("fixture: still comparing")
		}
		m.chart.columns, m.chart.columnsFor = nil, nil
		m.chart.positions = positions
		m.chart.cusps = cusps
		m.focused = PanelBodies
		m = m.restampFrame() // the frame is the rows, not the view rule
		writeDesignFrame(t, "bodies-untyped-names-80x24-"+mode.name, m.View())
	}
}
