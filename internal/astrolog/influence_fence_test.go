package astrolog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/cruiserxyz/sextile/internal/testenv/astrodir"
)

// TestInfluenceChartBugSeven is the fence's tripwire. Astrolog 8.00's influence
// chart (-j) reads past its arrays for every fixed star (Astrolog NEXT.md
// BUG 7), so lookup.ComputeRawReport leaves the stars out of the chart report.
// This runs -j with the ten planets and each star alone and logs the stars that
// end the run on a signal. It does not fail when none does: a build whose
// stray read lands on harmless memory prints a table with power in a wrong
// sign, and no check that is true of every build tells that table from a right
// one. A crash-free run is therefore not proof the bug is fixed, and the fence
// stays until Astrolog's own change says so.
func TestInfluenceChartBugSeven(t *testing.T) {
	dir := astrodir.Need(t)
	if _, err := os.Stat(filepath.Join(dir, "sefstars.txt")); err != nil {
		t.Skip("the data folder has no sefstars.txt: no star can be asked for")
	}
	reg, err := loadTestRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var crashed []int
	asked := 0
	for _, o := range reg {
		if !o.Star {
			continue
		}
		asked++
		argv := []string{"-qj", "2461349.69", "-zl", "4:53E", "52:22N", "-R0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", strconv.Itoa(o.Number), "-j"}
		_, _, err := invoke(context.Background(), argv)
		if errors.Is(err, ErrAstrologCrashed) {
			crashed = append(crashed, o.Number)
		}
	}
	if asked == 0 {
		t.Fatal("the registry holds no star")
	}
	t.Logf("-j with the ten planets and one star each: %d of %d stars end the run on a signal: %v", len(crashed), asked, crashed)
}
