package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/internal/ui/term"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// aspect_letters_cell_test.go: an aspect without a glyph is drawn by its three
// letters set directly between the two sides (`☿Sep♄ (0°20'S)`), with the
// experiment on by a centered dot with the usual spaces (`☿ · ♄ (0°20'S)`);
// either way the cell is as wide as any other, so no column depends on which
// aspects are ticked or shown.

var lettersCellModes = []struct {
	name string
	cfg  Config
}{
	{"colour", Config{Color: ColorAlways}},
	{"plain", Config{Color: ColorNever}},
	{"accessible", Config{Accessible: true}},
}

func septileAt(a, b aspect.Object) aspect.Aspect {
	return aspect.NewObjectAspect(a, b, kindOf("Sep"), aspect.Orb(float64(0)+float64(20)/60.0))
}

// The column widths never depend on what is ticked or shown: a glyph-less cell,
// in either form, measures as a classical cell does, and a list that holds
// Septile needs the same rows and the same column as one that does not.
func TestAspectColumnsDoNotDependOnWhichAspectsAreShown(t *testing.T) {
	sunSat := aspect.NewObjectAspect(objSun, objSaturn, kindOf("Squ"), aspect.Orb(1.55))
	for _, mode := range lettersCellModes {
		for _, dot := range []bool{false, true} {
			sep := septileAt(objMercuryFixture, objSaturn)
			bag := panel.AspectsBag{Rows: []aspect.Aspect{sep}, Dot: dot}
			plain := panel.AspectsBag{Rows: []aspect.Aspect{sunSat}}
			if a, b := ansi.StringWidth(panel.RenderAspectsTabRows(bag, 40, 1, mode.cfg)), ansi.StringWidth(panel.RenderAspectsTabRows(plain, 40, 1, mode.cfg)); a != b {
				t.Errorf("%s dot=%v: a Septile cell is %d wide, a Square cell %d", mode.name, dot, a, b)
			}
			cell := panel.FormatAspectEntryStyled(sep, astrologstyle.AspectStyle{}, mode.cfg)
			if dot {
				cell = panel.FormatAspectEntryDot(sep, astrologstyle.AspectStyle{}, mode.cfg)
			}
			if w := ansi.StringWidth(cell); w != panel.AspectColWidth {
				t.Errorf("%s dot=%v: the cell is %d cells, want %d", mode.name, dot, w, panel.AspectColWidth)
			}
		}
	}
	// One column of two cells: ticking Septile adds a cell, never width.
	wide := func(rows ...aspect.Aspect) int {
		out := ansi.Strip(panel.RenderAspectsTabRows(panel.AspectsBag{Rows: rows}, 31, 2, Config{Color: ColorNever}))
		w := 0
		for _, l := range strings.Split(out, "\n") {
			w = max(w, ansi.StringWidth(strings.TrimRight(l, " ")))
		}
		return w
	}
	if a, b := wide(sunSat), wide(sunSat, septileAt(objMercuryFixture, objSaturn)); a != b {
		t.Errorf("the panel's text is %d wide without Septile and %d with it", a, b)
	}
}

// The default cell: the first glyph, the letters, the second glyph, no spaces,
// then the orb as every cell draws it; the letters carry Astrolog's colour of
// the aspect. A Sun-first cell is drawn the same.
func TestAspectWithoutAGlyphIsDrawnByItsLetters(t *testing.T) {
	style := astrologstyle.AspectStyle{First: "\x1b[1;31m", Kind: "\x1b[1;36m", Second: "\x1b[1;32m"}
	for _, tc := range []struct {
		a, b aspect.Object
		want string
	}{
		{objMercuryFixture, objSaturn, "☿Sep♄ (0°20'S)"},
		{objSun, objSaturn, "☉Sep♄ (0°20'S)"},
	} {
		sep := septileAt(tc.a, tc.b)
		if got := strings.TrimSpace(formatAspectEntry(sep)); got != tc.want {
			t.Errorf("single list: %q, want %q", got, tc.want)
		}
		coloured := panel.FormatAspectEntryStyled(sep, style, Config{Color: ColorAlways})
		if !strings.Contains(coloured, "\x1b[96mSep\x1b[0m") {
			t.Errorf("the letters are not in Astrolog's colour: %q", coloured)
		}
		if astrologstyle.StripSGR(coloured) != panel.FormatAspectEntryStyled(sep, style, Config{Color: ColorNever}) {
			t.Errorf("colour changed the cell's text or width")
		}
		if w := ansi.StringWidth(coloured); w != panel.AspectColWidth {
			t.Errorf("the cell is %d wide, want %d", w, panel.AspectColWidth)
		}
	}
	cross := lookup.OverlayAspect{TransitObject: objSun, NatalObject: objMercuryFixture, Kind: kindOf("Sep"), Orb: aspect.Orb(20.0 / 60)}
	if got := strings.TrimSpace(formatCrossAspectEntry(cross)); !strings.HasPrefix(got, "☉Sep☿ (0°") {
		t.Errorf("pair tab: %q, want the sides and the letters with no space", got)
	}
}

// The experiment: the same cell with a centered dot in the glyph's place and
// the usual spaces, in the aspect's colour, as wide as any cell.
func TestAspectDotExperimentDrawsTheDot(t *testing.T) {
	style := astrologstyle.AspectStyle{First: "\x1b[1;31m", Kind: "\x1b[1;36m", Second: "\x1b[1;32m"}
	sep := septileAt(objMercuryFixture, objSaturn)
	styles := astrologstyle.Aspects{{First: objMercuryFixture, Second: objSaturn, Kind: kindOf("Sep")}: style}
	bag := panel.AspectsBag{Rows: []aspect.Aspect{sep}, RowStyles: styles, Dot: true}
	coloured := panel.RenderAspectsTabRows(bag, 40, 1, Config{Color: ColorAlways})
	if got := strings.TrimSpace(ansi.Strip(coloured)); got != "☿ · ♄ (0°20'S)" {
		t.Errorf("with the experiment on: %q, want %q", got, "☿ · ♄ (0°20'S)")
	}
	if !strings.Contains(coloured, "\x1b[96m·\x1b[0m") {
		t.Errorf("the dot is not in Astrolog's colour: %q", coloured)
	}
	bag.Dot = false
	if got := strings.TrimSpace(ansi.Strip(panel.RenderAspectsTabRows(bag, 40, 1, Config{Color: ColorAlways}))); got != "☿Sep♄ (0°20'S)" {
		t.Errorf("with the experiment off: %q", got)
	}
	// A classical aspect keeps its glyph either way.
	sq := aspect.NewObjectAspect(objSun, objSaturn, kindOf("Squ"), aspect.Orb(1.55))
	if got := ansi.Strip(panel.RenderAspectsTabRows(panel.AspectsBag{Rows: []aspect.Aspect{sq}, Dot: true}, 40, 1, Config{Color: ColorNever})); !strings.Contains(got, "□") {
		t.Errorf("the dot replaced a glyph: %q", got)
	}
}

// The Model asks the experiment where the cell is painted.
func TestAspectDotExperimentReachesThePaint(t *testing.T) {
	m := newTestModel(t, Config{Accessible: true}, FrozenClock(sessionFrozen))
	if m.paintedChart().aspectDot {
		t.Fatal("the dot is on by default")
	}
	m.experimentsOn = map[string]bool{expAspectDot: true}
	if !m.paintedChart().aspectDot {
		t.Error("the experiment is on and the paint does not carry it")
	}
}

// The glyph-room rule has one exemption, the form of this cell, and no other
// glyph followed directly by letters passes: not a word, not a circled number,
// not four letters, not lowercase, not a glyph at the end of a line.
func TestGlyphRoomExemptsOnlyTheAspectLettersCell(t *testing.T) {
	for _, tc := range []struct {
		name  string
		frame string
		bad   bool
	}{
		{"the cell, Sun first", "☉Sep♄ (0°20'S)", false},
		{"the cell, Mars first", "♂Un1♀ (1°07'A)", false},
		{"the cell, a cusp number second", "☿Sep11 (1°07'A)", false},
		{"the cell as the renderer draws it", panel.FormatAspectEntryStyled(septileAt(objSun, objSaturn), astrologstyle.AspectStyle{First: "\x1b[91m", Kind: "\x1b[94m", Second: "\x1b[92m"}, Config{Color: ColorAlways}), false},
		{"a word after the glyph", "☉next", true},
		{"a word after a planet glyph, longer", "♂Sepx♄", true},
		{"lowercase letters", "☉sep♄", true},
		{"a circled number and the letters", "②Sep♄", true},
		{"letters at the end of the line", "☉Sep", true},
		{"letters and a space", "☉Sep (1°)", true},
	} {
		got := wideGlyphFaults(tc.frame)
		if (len(got) > 0) != tc.bad {
			t.Errorf("%s: %q: faults %v, want bad=%v", tc.name, tc.frame, got, tc.bad)
		}
	}
	if term.IsAspectLettersCell('②', []rune("Sep♄")) {
		t.Error("a circled number is exempt")
	}
}
