// Package astrodir finds the directory Astrolog's data lives in, for tests.
//
// A test that copies the data directory (to append a hostile line to its
// astrolog.as) or reads its atlas cannot assume where an install put it:
// scripts/build-astrolog installs under PREFIX, which is /usr/local in the
// container image and ~/.local on a stock Ubuntu host or in CI. Package
// astrolog's own tests cannot import internal/testenv (it imports astrolog),
// which is why this is a package of its own, below both.
//
// It is imported only from _test.go files.
package astrodir

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cruiserxyz/sextile/internal/location"
)

// dir is resolved once, at package initialisation: a test binary that points
// HOME and the XDG bases at a sandbox (internal/testenv.Run) does so in
// TestMain, after this ran, and must not lose sight of the install.
var dir = find()

// find is $ASTROLOG when set (it is what Astrolog itself reads first), else
// the directory holding the atlas Sextile resolves: $SEXTILE_ATLAS_PATH, the
// user-local data directory, then /usr/local/share and /usr/share
// (location.AtlasPath).
func find() string {
	if d := os.Getenv("ASTROLOG"); d != "" {
		return d
	}
	if p, err := location.AtlasPath(); err == nil {
		return filepath.Dir(p)
	}
	return ""
}

// Dir is Astrolog's data directory as the process started, or "" if none was
// found.
func Dir() string { return dir }

// Need is Dir, skipping the test when there is none: a test that copies or
// reads the data cannot run without it.
func Need(t testing.TB) string {
	t.Helper()
	if dir == "" {
		t.Skip("Astrolog's data directory not found (set ASTROLOG or SEXTILE_ATLAS_PATH)")
	}
	return dir
}
