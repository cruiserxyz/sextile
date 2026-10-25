package store

import (
	"os"
	"testing"
)

// preferences.yaml's replacement is synced before it is renamed over the
// file: the one file synced is the temp file the rename then moves.
func TestPreferencesTempIsSyncedBeforeTheRename(t *testing.T) {
	path := prefPath(t)
	seedPreferences(t, path, clockBlock)
	var synced, renamed []string
	prevSync, prevRename := syncTemp, renameFile
	syncTemp = func(f *os.File) error { synced = append(synced, f.Name()); return prevSync(f) }
	renameFile = func(from, to string) error {
		renamed = append(renamed, from)
		if len(synced) != 1 || synced[0] != from {
			t.Errorf("renamed %s after syncing %q, want that file synced once first", from, synced)
		}
		return prevRename(from, to)
	}
	t.Cleanup(func() { syncTemp, renameFile = prevSync, prevRename })

	if err := SetLocation(berlin); err != nil {
		t.Fatal(err)
	}
	if len(renamed) != 1 {
		t.Errorf("%d renames, want 1", len(renamed))
	}
}
