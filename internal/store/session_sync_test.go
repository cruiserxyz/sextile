package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// session.yaml's replacement is synced while the file still holds the previous
// session, so before the rename, and the file synced is the one renamed into
// place: a temp file beside session.yaml that is gone after the save, whose
// bytes session.yaml then holds.
func TestSessionTempIsSyncedBeforeTheRename(t *testing.T) {
	dir := sessionSandbox(t)
	if err := SaveSession(Session{Slots: oneSlot(fixtureChart(), false)}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "session.yaml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var synced []string
	var syncedBytes []byte
	prev := syncTemp
	syncTemp = func(f *os.File) error {
		synced = append(synced, f.Name())
		syncedBytes, _ = os.ReadFile(f.Name())
		if now, err := os.ReadFile(path); err != nil || string(now) != string(before) {
			t.Errorf("session.yaml changed before its temp file was synced (%v)", err)
		}
		return prev(f)
	}
	t.Cleanup(func() { syncTemp = prev })

	if err := SaveSession(Session{Slots: oneSlot(fixtureChart(), true)}); err != nil {
		t.Fatal(err)
	}
	if len(synced) != 1 {
		t.Fatalf("%d syncs, want 1: %q", len(synced), synced)
	}
	if filepath.Dir(synced[0]) != dir || synced[0] == path {
		t.Errorf("synced %s, want a temp file beside %s", synced[0], path)
	}
	if _, err := os.Lstat(synced[0]); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the synced file %s is still there after the save (%v): it was not the one renamed", synced[0], err)
	}
	now, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(now) == string(before) || string(now) != string(syncedBytes) {
		t.Errorf("session.yaml does not hold the synced file's bytes:\n got: %q\nsynced: %q", now, syncedBytes)
	}
}
