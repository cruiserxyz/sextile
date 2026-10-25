package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// TestAspectWaterfall_PowerSortColumnMajor: aspects are sorted by power
// descending and laid out column-major (highest power fills column 0 top-down).
func TestAspectWaterfall_PowerSortColumnMajor(t *testing.T) {
	mk := func(b aspect.Body, k aspect.AspectKind, pow float64) aspect.Aspect {
		a := aspect.NewAspect(aspect.Sun, b, k, aspect.Orb(1))
		a.Power = pow
		return a
	}
	aspects := []aspect.Aspect{
		mk(aspect.Jupiter, aspect.Sextile, 1),  // ☉ ⚹ ♃  (lowest)
		mk(aspect.Moon, aspect.Conjunction, 4), // ☉ ☌ ☽  (highest)
		mk(aspect.Venus, aspect.Trine, 2),      // ☉ △ ♀
		mk(aspect.Mars, aspect.Square, 3),      // ☉ □ ♂
	}
	// innerWidth 31 → 2 columns; innerHeight 2 → 2 rows. Column-major fill:
	// col0 = [pow4, pow3], col1 = [pow2, pow1].
	out := renderAspectWaterfall(aspects, 31, 2)
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "☉ ☌ ☽") {
		t.Errorf("line0 must start with the highest-power aspect, got %q", lines[0])
	}
	if !strings.Contains(lines[0], "△") {
		t.Errorf("line0 col1 should hold the pow-2 trine, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "□") || !strings.Contains(lines[1], "⚹") {
		t.Errorf("line1 should hold pow-3 square (col0) and pow-1 sextile (col1), got %q", lines[1])
	}
}

// TestFormatAspectEntry_Width: every entry is exactly aspectColWidth display
// cells; orb >= 10° drops the space before "(" yet still fits.
func TestFormatAspectEntry_Width(t *testing.T) {
	small := aspect.NewAspect(aspect.Sun, aspect.Saturn, aspect.Square, aspect.Orb(1.55))
	if w := ansi.StringWidth(formatAspectEntry(small)); w != aspectColWidth {
		t.Errorf("entry width = %d, want %d", w, aspectColWidth)
	}
	big := aspect.NewAspect(aspect.Sun, aspect.Saturn, aspect.Square, aspect.Orb(12.5))
	eb := formatAspectEntry(big)
	if w := ansi.StringWidth(eb); w != aspectColWidth {
		t.Errorf("wide-orb entry width = %d, want %d (%q)", w, aspectColWidth, eb)
	}
	if strings.Contains(eb, " (") {
		t.Errorf("orb>=10 entry should drop the space before '(', got %q", eb)
	}
}

// TestFormatAspectEntry_ApplyingSuffix: the in-paren A/S suffix reflects the
// Applying flag (D2).
func TestFormatAspectEntry_ApplyingSuffix(t *testing.T) {
	applying := aspect.NewAspect(aspect.Sun, aspect.Moon, aspect.Square, aspect.Orb(1))
	applying.Applying = true
	if e := formatAspectEntry(applying); !strings.Contains(e, "'A)") {
		t.Errorf("applying aspect should show \"'A)\", got %q", e)
	}
	if e := formatAspectEntry(aspect.NewAspect(aspect.Sun, aspect.Moon, aspect.Square, aspect.Orb(1))); !strings.Contains(e, "'S)") {
		t.Errorf("separating aspect should show \"'S)\", got %q", e)
	}
}

// TestAspectWaterfall_WidthDegradationAndOverflow: narrow widths reduce the
// column count, and a grid that can't hold every aspect drops the lowest-power.
func TestAspectWaterfall_WidthDegradationAndOverflow(t *testing.T) {
	var aspects []aspect.Aspect
	for i := 0; i < 20; i++ {
		a := aspect.NewAspect(aspect.Sun, aspect.Moon, aspect.Conjunction, aspect.Orb(1))
		a.Power = float64(20 - i)
		aspects = append(aspects, a)
	}
	// Narrow: innerWidth 15 → 1 column, innerHeight 3 → only 3 rows shown.
	narrow := strings.Split(renderAspectWaterfall(aspects, 15, 3), "\n")
	if len(narrow) != 3 {
		t.Fatalf("narrow waterfall: %d lines, want 3", len(narrow))
	}
	// Wide: innerWidth 64 → 4 columns × 4 rows = 16 cells; 20 aspects → 4 drop.
	wide := renderAspectWaterfall(aspects, 64, 4)
	if n := strings.Count(wide, "☌"); n != 16 {
		t.Errorf("wide grid should show 16 of 20 aspects (lowest 4 dropped), got %d", n)
	}
}

func TestAspectWaterfallUsesAstrologSpansWithoutChangingWidth(t *testing.T) {
	a := aspect.NewAspect(aspect.Sun, aspect.Moon, aspect.Square, aspect.Orb(1.5))
	styles := astrologstyle.Aspects{
		{First: aspect.Sun, Second: aspect.Moon, Kind: aspect.Square}: {
			First: "\x1b[1;31m", Kind: "\x1b[1;34m", Second: "\x1b[1;32m",
		},
	}
	style := styles[astrologstyle.AspectIdentity{First: aspect.Sun, Second: aspect.Moon, Kind: aspect.Square}]
	plain := formatAspectEntryStyled(a, style, Config{Color: ColorNever})
	colored := formatAspectEntryStyled(a, style, Config{Color: ColorAlways})

	if strings.Contains(plain, "\x1b[") {
		t.Fatalf("no-color waterfall emitted ANSI: %q", plain)
	}
	for _, want := range []string{"\x1b[91m☉\x1b[0m", "\x1b[94m□\x1b[0m", "\x1b[92m☽\x1b[0m"} {
		if !strings.Contains(colored, want) {
			t.Errorf("colored waterfall %q missing exact normalized Astrolog span %q", colored, want)
		}
	}
	if astrologstyle.StripSGR(colored) != plain {
		t.Fatalf("color changed aspect text/alignment: colored=%q plain=%q", colored, plain)
	}
	if ansi.StringWidth(colored) != aspectColWidth {
		t.Fatalf("colored width = %d, want %d", ansi.StringWidth(colored), aspectColWidth)
	}
}

func TestCrossAspectWaterfallMapsPrimarySecondaryStylesToDisplayRoles(t *testing.T) {
	a := lookup.OverlayAspect{Transit: aspect.Mars, Natal: aspect.Moon, Kind: aspect.Sextile, Orb: 1}
	styles := astrologstyle.Aspects{
		// Astrolog -r0 order is primary/natal Moon, secondary/transit Mars.
		{First: aspect.Moon, Second: aspect.Mars, Kind: aspect.Sextile}: {
			First: "\x1b[1;34m", Kind: "\x1b[1;36m", Second: "\x1b[1;31m",
		},
	}
	got := renderCrossAspectWaterfallStyled([]lookup.OverlayAspect{a}, styles, aspectColWidth, 1, Config{Color: ColorAlways})
	for _, want := range []string{"\x1b[91m♂\x1b[0m", "\x1b[96m⚹\x1b[0m", "\x1b[94m☽\x1b[0m"} {
		if !strings.Contains(got, want) {
			t.Errorf("cross waterfall %q missing role-mapped span %q", got, want)
		}
	}
	// the cell keeps Astrolog's order — natal (primary) left, transit
	// (secondary) right — so the natal Moon span precedes the transit Mars span.
	if strings.Index(got, "\x1b[94m☽") > strings.Index(got, "\x1b[91m♂") {
		t.Errorf("cross cell prints transit before natal: %q", got)
	}
	if plain := renderCrossAspectWaterfall([]lookup.OverlayAspect{a}, aspectColWidth, 1); !strings.HasPrefix(plain, "☽ ⚹ ♂") {
		t.Errorf("plain cross cell = %q, want natal-left %q", plain, "☽ ⚹ ♂")
	}
}

func TestAspectsSuppressColorInAccessibleAndNoColorModes(t *testing.T) {
	a := aspect.NewAspect(aspect.Sun, aspect.Moon, aspect.Trine, 1)
	state := chartViewState{
		aspects: []aspect.Aspect{a},
		aspectStyles: astrologstyle.Aspects{
			{First: aspect.Sun, Second: aspect.Moon, Kind: aspect.Trine}: {
				First: "\x1b[1;31m", Kind: "\x1b[1;32m", Second: "\x1b[1;34m",
			},
		},
	}
	for name, cfg := range map[string]Config{
		"accessible": {Accessible: true},
		"no-color":   {Color: ColorNever},
	} {
		if got := renderAspectsBody(state, TabAspects, 64, 4, cfg); strings.Contains(got, "\x1b[") {
			t.Errorf("%s mode emitted aspect ANSI: %q", name, got)
		}
	}
	if got := renderAspectsBody(state, TabAspects, 64, 4, Config{}); !strings.Contains(got, "\x1b[") {
		t.Errorf("default mode suppressed Astrolog aspect color: %q", got)
	}
}

func TestApplyTextCommitCarriesSingleAndCrossAspectStyles(t *testing.T) {
	singleID := astrologstyle.AspectIdentity{First: aspect.Sun, Second: aspect.Moon, Kind: aspect.Trine}
	crossID := astrologstyle.AspectIdentity{First: aspect.Moon, Second: aspect.Mars, Kind: aspect.Sextile}
	m, _ := applyTextCommit(Model{}, FirstFrameReadyMsg{
		AspectStyles:      astrologstyle.Aspects{singleID: {Kind: "single"}},
		CrossAspectStyles: astrologstyle.Aspects{crossID: {Kind: "cross"}},
	})
	if got := m.chart.aspectStyles[singleID].Kind; got != "single" {
		t.Errorf("single aspect style commit = %q", got)
	}
	if got := m.chart.crossAspectStyles[crossID].Kind; got != "cross" {
		t.Errorf("cross aspect style commit = %q", got)
	}
}

// TestAspectCellDrawsAPlaceholderForGlyphlessBodies: a side the enum cannot name
// is drawn as the placeholder in both cells, never as a question mark.
func TestAspectCellDrawsAPlaceholderForGlyphlessBodies(t *testing.T) {
	single := aspect.NewLabelledAspect(aspect.BodyUnknown, "Ceres", aspect.Sun, "", aspect.Trine, aspect.Orb(1.5))
	cell := formatAspectEntry(single)
	if !strings.HasPrefix(cell, "· △ ☉") || strings.Contains(cell, "?") {
		t.Errorf("single cell = %q, want placeholder trine Sun and no question mark", cell)
	}
	both := formatAspectEntry(aspect.NewLabelledAspect(aspect.BodyUnknown, "Ceres", aspect.BodyUnknown, "Vertex", aspect.Square, aspect.Orb(2)))
	if !strings.HasPrefix(both, "· □ ·") || strings.Contains(both, "?") {
		t.Errorf("two-placeholder cell = %q, want '· □ ·'", both)
	}
	cross := formatCrossAspectEntry(lookup.OverlayAspect{
		Natal: aspect.BodyUnknown, NatalLabel: "Vertex", Transit: aspect.Moon,
		Kind: aspect.Opposition, Orb: aspect.Orb(1),
	})
	if !strings.HasPrefix(cross, "· ☍ ☽") || strings.Contains(cross, "?") {
		t.Errorf("cross cell = %q, want placeholder opposition Moon and no question mark", cross)
	}
}

// TestAspectsPanelShowsAngles: the Aspects panel body draws the angles as
// A/M/D/I, which it could not while the parser dropped every angle line.
func TestAspectsPanelShowsAngles(t *testing.T) {
	mk := func(b aspect.Body, pow float64) aspect.Aspect {
		a := aspect.NewAspect(aspect.Sun, b, aspect.Square, aspect.Orb(1))
		a.Power = pow
		return a
	}
	bag := aspectsBag{Settled: true, Rows: []aspect.Aspect{
		mk(aspect.Ascendant, 4), mk(aspect.Midheaven, 3), mk(aspect.Descendant, 2), mk(aspect.ImumCoeli, 1),
	}}
	out := renderAspectsTabRows(bag, 40, 4, Config{Color: ColorNever})
	for _, want := range []string{"☉ □ A", "☉ □ M", "☉ □ D", "☉ □ I"} {
		if !strings.Contains(out, want) {
			t.Errorf("Aspects panel lacks %q:\n%s", want, out)
		}
	}
}
