package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// the specification C2, shipped ahead of the rest of that spec as a hotfix
// (operator, 2026-10-12): a ticked cusp's side shows its house number, not the
// placeholder, and the glyph cells grow to two only when an 11 or 12 is listed.

// fixtureObject is Astrolog's object number n from the registry the fixtures
// were captured from (internal/astrolog/testdata/ho-all.txt).
func fixtureObject(t *testing.T, n int) aspect.Object {
	t.Helper()
	o, err := fixtureObjectNumbered(n)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// fixtureObjectNumbered is fixtureObject for a helper that has no *testing.T.
func fixtureObjectNumbered(n int) (aspect.Object, error) {
	fixtureRegistryOnce.Do(func() {
		b, err := os.ReadFile("../astrolog/testdata/ho-all.txt")
		if err == nil {
			fixtureRegistryList, err = astrolog.ParseObjectRegistry(string(b))
		}
		fixtureRegistryErr = err
	})
	if fixtureRegistryErr != nil {
		return aspect.Object{}, fixtureRegistryErr
	}
	for _, o := range fixtureRegistryList {
		if o.Number == n {
			return o.Object, nil
		}
	}
	return aspect.Object{}, fmt.Errorf("object %d is not in the fixture registry", n)
}

// markedObjects is a slot's marks, in ascending object number.
func markedObjects(s *store.Slots, i int) []aspect.Object {
	return s.SelectedObjects(i)
}

var (
	fixtureRegistryOnce sync.Once
	fixtureRegistryList []aspect.RegistryObject
	fixtureRegistryErr  error
)

func cuspSide(t *testing.T, n int) aspect.Aspect {
	return aspect.Aspect{AObject: objMoon, BObject: fixtureObject(t, n), Kind: kindOf("Con"), Orb: 1.5, Power: 9}
}

func TestAspectCuspShowsItsHouseNumber(t *testing.T) {
	for _, tc := range []struct {
		obj  int
		want string
	}{
		{23, "☽ ☌ 2 "},
		{27, "☽ ☌ 6 "},
		{33, "☽ ☌ 12"},
		{32, "☽ ☌ 11"},
	} {
		got := renderAspectWaterfall([]aspect.Aspect{cuspSide(t, tc.obj)}, 40, 1)
		if !strings.Contains(got, tc.want) || strings.Contains(got, panel.PlaceholderGlyph) {
			t.Errorf("object %d: got %q, want it to contain %q and no %q", tc.obj, got, tc.want, panel.PlaceholderGlyph)
		}
	}
	// An object that is not a cusp keeps the placeholder.
	if got := renderAspectWaterfall([]aspect.Aspect{cuspSide(t, 20)}, 40, 1); !strings.Contains(got, panel.PlaceholderGlyph) {
		t.Errorf("Vertex: got %q, want the placeholder", got)
	}
	t.Run("comparing", func(t *testing.T) {
		a := lookup.OverlayAspect{NatalObject: fixtureObject(t, 32), TransitObject: objMars, Kind: kindOf("Con"), Orb: 1, Power: 5}
		if got := renderCrossAspectWaterfall([]lookup.OverlayAspect{a}, 40, 1); !strings.HasPrefix(got, " ♂ ☌ 11") {
			t.Errorf("got %q, want the transiting Mars first (in the cusp number's two cells), then the natal cusp's number", got)
		}
	})
}

func TestAspectCuspWidensTheGlyphCellsOnlyForTwoDigits(t *testing.T) {
	plain := aspect.Aspect{AObject: objSun, BObject: objMars, Kind: kindOf("Squ"), Orb: 2, Power: 5}
	// kindCol is the display column of the aspect symbol in a row.
	kindCol := func(row string) int { return ansi.StringWidth(row[:strings.IndexAny(row, "□☌")]) }

	t.Run("no two-digit cusp: today's cells", func(t *testing.T) {
		got := strings.Split(renderAspectWaterfall([]aspect.Aspect{plain, cuspSide(t, 23)}, panel.AspectColWidth, 2), "\n")
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
		got := strings.Split(renderAspectWaterfall([]aspect.Aspect{plain, cuspSide(t, 33)}, panel.AspectColWidth+2, 2), "\n")
		if len(got) != 2 {
			t.Fatalf("want two rows, got %q", got)
		}
		for _, row := range got {
			if w := ansi.StringWidth(row); w > panel.AspectColWidth+2 {
				t.Errorf("row %q is %d cells, want at most %d", row, w, panel.AspectColWidth+2)
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
