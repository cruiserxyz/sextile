package astrodir

import (
	"os"
	"path/filepath"
	"testing"
)

// The data directory follows the install: $ASTROLOG first, then wherever the
// atlas resolves, then nothing.
func TestFindFollowsTheInstall(t *testing.T) {
	moved := t.TempDir()
	if err := os.WriteFile(filepath.Join(moved, "atlas.as"), []byte("@AC800\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	t.Setenv("ASTROLOG", "")
	t.Setenv("SEXTILE_ATLAS_PATH", filepath.Join(moved, "atlas.as"))
	if got := find(); got != moved {
		t.Errorf("find() = %q, want the atlas's directory %q", got, moved)
	}

	t.Setenv("ASTROLOG", "/elsewhere")
	if got := find(); got != "/elsewhere" {
		t.Errorf("find() = %q, want $ASTROLOG", got)
	}

	t.Setenv("ASTROLOG", "")
	t.Setenv("SEXTILE_ATLAS_PATH", filepath.Join(moved, "missing.as"))
	if got := find(); got != "" {
		t.Errorf("find() = %q, want none for an unreadable SEXTILE_ATLAS_PATH", got)
	}
}
