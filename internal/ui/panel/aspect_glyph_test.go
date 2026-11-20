package panel

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// binaryAspects is the aspect list of the Astrolog in use (`astrolog -HA`
// through the package's own reader), or, with no binary on the PATH, of the
// capture the astrolog package's fixtures were taken from.
func binaryAspects(t *testing.T) aspect.AspectRegistry {
	t.Helper()
	rows, err := astrolog.AspectRegistry(context.Background())
	if err == nil {
		return aspect.NewAspectRegistry(rows)
	}
	// Only a missing binary falls back to the capture. A binary that is
	// installed and cannot be run or read is a failure of this test.
	if !errors.Is(err, astrolog.ErrNotInstalled) {
		t.Fatalf("astrolog -HA, an installed binary: %v", err)
	}
	b, err := os.ReadFile("../../astrolog/testdata/ha-default.txt")
	if err != nil {
		t.Fatal(err)
	}
	rows, err = astrolog.ParseAspectRegistry(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return aspect.NewAspectRegistry(rows)
}

// Acceptance 9: the glyph table names only aspects the registry of the binary
// in use holds, and every aspect it holds is drawn: a glyph where the table has
// one, else the three letters.
func TestGlyphTableNamesOnlyAspectsTheRegistryHolds(t *testing.T) {
	reg := binaryAspects(t)
	if len(aspectGlyphs) == 0 {
		t.Fatal("the glyph table is empty")
	}
	for letters := range aspectGlyphs {
		if _, ok := reg.ByAbbrev(letters); !ok {
			t.Errorf("the glyph table names %q, which the registry of this Astrolog does not hold", letters)
		}
	}
	for _, row := range reg.Rows() {
		got := AspectGlyph(row.Kind)
		want := row.Kind.Abbrev
		if g, ok := aspectGlyphs[row.Kind.Abbrev]; ok {
			want = g
		}
		if got != want || got == "" || got == "?" {
			t.Errorf("%s is drawn as %q, want %q", row.Kind.Abbrev, got, want)
		}
	}
	if got := AspectGlyph(aspect.AspectKind{}); got != "?" {
		t.Errorf("no aspect is drawn as %q, want ?", got)
	}
}

// The eleven classical marks, written out by hand and not read from the glyph
// table: a change of a glyph, a mark of the same width included, fails here.
func TestClassicalMarksAreTheOnesTheScreenHadBefore(t *testing.T) {
	reg := binaryAspects(t)
	for _, want := range []struct{ letters, mark string }{
		{"Con", "☌"}, {"Opp", "☍"}, {"Squ", "□"}, {"Tri", "△"}, {"Sex", "⚹"}, {"Inc", "⚻"},
		{"SSx", "⚺"}, {"SSq", "∠"}, {"Ses", "⚼"}, {"Qui", "Q"}, {"BQn", "bQ"},
	} {
		row, ok := reg.ByAbbrev(want.letters)
		if !ok {
			t.Errorf("the registry holds no %s", want.letters)
			continue
		}
		if got := AspectGlyph(row.Kind); got != want.mark {
			t.Errorf("%s is drawn as %q, want %q", want.letters, got, want.mark)
		}
	}
}
