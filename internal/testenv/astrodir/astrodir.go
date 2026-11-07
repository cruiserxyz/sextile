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
	"slices"
	"strings"
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

// Reduced makes a copy of the data directory without the named files (a body
// whose ephemeris file is gone), appends lines to its astrolog.as, and points
// Astrolog at it for the rest of the test the way the launcher does: ASTROLOG
// and the Swiss Ephemeris's SE_EPHE_PATH name it, ASTR800 and ASTR (which would
// outrank them) are unset, and the working directory is an empty temp dir (a
// cwd astrolog.as would shadow it). It returns the copy.
//
// Only astrolog.as is copied, because lines are appended to it; every other
// file is a symbolic link to the original. A share holding the planetary
// moons' files is about 230 MB, and a byte copy per call would cost that much
// at every one of the helper's callers.
func Reduced(t testing.TB, without []string, lines ...string) string {
	t.Helper()
	src := Need(t)
	dst := t.TempDir()
	ents, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.IsDir() || slices.Contains(without, e.Name()) {
			continue
		}
		if e.Name() != "astrolog.as" {
			if err := os.Symlink(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				t.Fatal(err)
			}
			continue
		}
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if len(lines) > 0 {
		as := filepath.Join(dst, "astrolog.as")
		b, err := os.ReadFile(as)
		if err != nil {
			t.Fatal(err)
		}
		b = append(b, []byte("\n"+strings.Join(lines, "\n")+"\n")...)
		if err := os.WriteFile(as, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("ASTROLOG", dst)
	t.Setenv("SE_EPHE_PATH", dst)
	t.Setenv("ASTR800", os.Getenv("ASTR800"))
	t.Setenv("ASTR", os.Getenv("ASTR"))
	os.Unsetenv("ASTR800")
	os.Unsetenv("ASTR")
	t.Chdir(t.TempDir())
	return dst
}

// ReducedWithoutStars is Reduced without the fixed-star catalogue
// (sefstars.txt): Astrolog prints no row and no stderr line for a star.
func ReducedWithoutStars(t testing.TB, lines ...string) string {
	t.Helper()
	return Reduced(t, []string{"sefstars.txt"}, lines...)
}

// ReducedWithoutUranians is Reduced without the orbital elements of the
// fictitious bodies (seorbel.txt): Astrolog refuses Vulcan with a stderr line
// that names no file.
func ReducedWithoutUranians(t testing.TB, lines ...string) string {
	t.Helper()
	return Reduced(t, []string{"seorbel.txt"}, lines...)
}
