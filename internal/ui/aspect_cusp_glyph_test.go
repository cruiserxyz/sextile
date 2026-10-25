package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// the specification C2, shipped ahead of the rest of that spec as a hotfix
// (operator, 2026-10-12): a ticked cusp's side shows its house number, not the
// placeholder, and the glyph cells grow to two only when an 11 or 12 is listed.

func cuspSide(label string) aspect.Aspect {
	return aspect.Aspect{A: aspect.Moon, B: aspect.BodyUnknown, BLabel: label, Kind: aspect.Conjunction, Orb: 1.5, Power: 9}
}

func TestAspectCuspShowsItsHouseNumber(t *testing.T) {
	for _, tc := range []struct{ label, want string }{
		{"2nd Cusp", "☽ ☌ 2 "},
		{"6th Cusp", "☽ ☌ 6 "},
		{"12th Cusp", "☽ ☌ 12"},
		{"11th Cu", "☽ ☌ 11"}, // the left column truncates to seven characters
	} {
		got := renderAspectWaterfall([]aspect.Aspect{cuspSide(tc.label)}, 40, 1)
		if !strings.Contains(got, tc.want) || strings.Contains(got, placeholderGlyph) {
			t.Errorf("%q: got %q, want it to contain %q and no %q", tc.label, got, tc.want, placeholderGlyph)
		}
	}
	// A label that is not a cusp keeps the placeholder.
	if got := renderAspectWaterfall([]aspect.Aspect{cuspSide("Vertex")}, 40, 1); !strings.Contains(got, placeholderGlyph) {
		t.Errorf("Vertex: got %q, want the placeholder", got)
	}
	t.Run("comparing", func(t *testing.T) {
		a := lookup.OverlayAspect{Natal: aspect.BodyUnknown, NatalLabel: "11th Cusp", Transit: aspect.Mars, Kind: aspect.Conjunction, Orb: 1, Power: 5}
		if got := renderCrossAspectWaterfall([]lookup.OverlayAspect{a}, 40, 1); !strings.HasPrefix(got, "11 ☌ ♂") {
			t.Errorf("got %q, want the natal cusp's number first", got)
		}
	})
}

func TestAspectCuspWidensTheGlyphCellsOnlyForTwoDigits(t *testing.T) {
	plain := aspect.Aspect{A: aspect.Sun, B: aspect.Mars, Kind: aspect.Square, Orb: 2, Power: 5}
	// kindCol is the display column of the aspect symbol in a row.
	kindCol := func(row string) int { return ansi.StringWidth(row[:strings.IndexAny(row, "□☌")]) }

	t.Run("no two-digit cusp: today's cells", func(t *testing.T) {
		got := strings.Split(renderAspectWaterfall([]aspect.Aspect{plain, cuspSide("2nd Cusp")}, aspectColWidth, 2), "\n")
		if len(got) != 2 || kindCol(got[0]) != 2 || kindCol(got[1]) != 2 {
			t.Errorf("one-cell rows changed: %q", got)
		}
		for _, row := range got {
			if strings.HasPrefix(row, " ") {
				t.Errorf("a one-cell glyph was padded: %q", row)
			}
		}
	})
	t.Run("a 12 widens every cell to two", func(t *testing.T) {
		got := strings.Split(renderAspectWaterfall([]aspect.Aspect{plain, cuspSide("12th Cusp")}, aspectColWidth+2, 2), "\n")
		if len(got) != 2 {
			t.Fatalf("want two rows, got %q", got)
		}
		for _, row := range got {
			if w := ansi.StringWidth(row); w > aspectColWidth+2 {
				t.Errorf("row %q is %d cells, want at most %d", row, w, aspectColWidth+2)
			}
		}
		// The left glyph is right-aligned and the right one left-aligned in
		// two cells, so the aspect symbols stay in one column.
		if !strings.Contains(strings.Join(got, "\n"), " ☽ ☌ 12") || !strings.Contains(strings.Join(got, "\n"), " ☉ □ ♂ ") {
			t.Errorf("cells not two wide: %q", got)
		}
		if kindCol(got[0]) != kindCol(got[1]) || kindCol(got[0]) != 3 {
			t.Errorf("aspect symbols not in one column: %q", got)
		}
	})
}
