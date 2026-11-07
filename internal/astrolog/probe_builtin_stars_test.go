package astrolog

import (
	"context"
	"testing"

	"github.com/cruiserxyz/sextile/internal/testenv/astrodir"
)

// Evidence for the star-equality test: without sefstars.txt Astrolog still
// returns a real row for Spica (99) and the Galactic Centre (132) when each is
// asked for alone (measured 2026-11-02 on Astrolog 8.00), so the probe rightly
// leaves them available.
func TestSpicaAndGalacticCentreKeepARowWithoutTheCatalogue(t *testing.T) {
	needAstrolog(t)
	astrodir.ReducedWithoutStars(t)
	for _, n := range []int{99, 132} {
		out, _, _, err := probeCast(context.Background(), []int{n})
		if err != nil {
			t.Fatal(err)
		}
		if got := mustPositions(t, out, n); len(got) != 1 {
			t.Errorf("object %d: %d rows, want the one real row", n, len(got))
		}
	}
}
