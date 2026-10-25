package store

import (
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
)

// watchSyncDir records every folder syncDir is asked to flush, in order.
func watchSyncDir(t *testing.T) *[]string {
	t.Helper()
	var synced []string
	prev := syncDir
	syncDir = func(dir string) { synced = append(synced, dir); prev(dir) }
	t.Cleanup(func() { syncDir = prev })
	return &synced
}

// preferences.yaml's folder is flushed after the rename, not before: at the
// moment of the flush the temp file is gone and the file holds the new text.
func TestPreferencesFolderIsSyncedAfterTheRename(t *testing.T) {
	path := prefPath(t)
	seedPreferences(t, path, clockBlock)
	var atSync string
	var tempsAtSync []string
	prev := syncDir
	syncDir = func(dir string) {
		if dir == filepath.Dir(path) {
			b, _ := os.ReadFile(path)
			atSync = string(b)
			tempsAtSync, _ = filepath.Glob(filepath.Join(dir, "*.tmp"))
		}
		prev(dir)
	}
	t.Cleanup(func() { syncDir = prev })

	if err := SetLocation(berlin); err != nil {
		t.Fatal(err)
	}
	if atSync == "" || atSync != readPreferences(t, path) {
		t.Errorf("the folder was not flushed after preferences.yaml held its new text; saw %q", atSync)
	}
	if len(tempsAtSync) != 0 {
		t.Errorf("temp files still beside preferences.yaml when the folder was flushed: %q", tempsAtSync)
	}
}

// A first preferences write into a folder that does not exist flushes the
// parent of each folder it created, so the new entries survive a power cut.
func TestPreferencesFirstWriteSyncsTheCreatedFoldersParent(t *testing.T) {
	path := prefPath(t) // <tmp>/sextile/preferences.yaml: sextile/ is missing
	synced := watchSyncDir(t)

	if err := SetLocation(berlin); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Dir(filepath.Dir(path)); !slices.Contains(*synced, want) {
		t.Errorf("flushed %q, want %s (the parent of the folder the save created) among them", *synced, want)
	}
}

// session.yaml's folder is flushed after the rename: the temp file is gone and
// the file holds the new session when the flush happens.
func TestSessionFolderIsSyncedAfterTheRename(t *testing.T) {
	dir := sessionSandbox(t)
	path := filepath.Join(dir, "session.yaml")
	var atSync []byte
	var tempsAtSync []string
	prev := syncDir
	syncDir = func(d string) {
		if d == dir {
			atSync, _ = os.ReadFile(path)
			tempsAtSync, _ = filepath.Glob(filepath.Join(d, "*.tmp"))
		}
		prev(d)
	}
	t.Cleanup(func() { syncDir = prev })

	if err := SaveSession(Session{Slots: oneSlot(fixtureChart(), true)}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(atSync) == 0 || string(atSync) != string(want) {
		t.Errorf("the folder was not flushed after session.yaml held its new text; saw %q", atSync)
	}
	if len(tempsAtSync) != 0 {
		t.Errorf("temp files still beside session.yaml when the folder was flushed: %q", tempsAtSync)
	}
}

// A first session save flushes the parent of every folder it created: the
// session folder's parent and that parent's parent when both were missing.
func TestSessionFirstSaveSyncsEveryCreatedFoldersParent(t *testing.T) {
	dir := sessionSandbox(t) // <tmp>/sextile/session: neither level exists
	synced := watchSyncDir(t)

	if err := SaveSession(Session{Slots: oneSlot(fixtureChart(), false)}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{filepath.Dir(dir), filepath.Dir(filepath.Dir(dir))} {
		if !slices.Contains(*synced, want) {
			t.Errorf("flushed %q, want %s among them", *synced, want)
		}
	}
}

// Writing into a folder that already exists still flushes that folder after
// the rename; flushing a parent as well is allowed, not required.
func TestExistingFolderIsSyncedAfterTheSave(t *testing.T) {
	dir := sessionSandbox(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "session.yaml")
	var seen bool
	prev := syncDir
	syncDir = func(d string) {
		if _, err := os.Stat(path); d == dir && err == nil {
			seen = true
		}
		prev(d)
	}
	t.Cleanup(func() { syncDir = prev })

	if err := SaveSession(Session{Slots: oneSlot(fixtureChart(), false)}); err != nil {
		t.Fatal(err)
	}
	if !seen {
		t.Errorf("%s was not flushed after session.yaml was in it", dir)
	}
}

// The launch takes the instance lock first, and the lock creates the session
// folder; the first save then finds it there. Every folder the lock made must
// still have its entry flushed, and the session folder after the rename.
func TestLockedFirstLaunchSyncsTheFoldersTheLockCreated(t *testing.T) {
	dir := sessionSandbox(t) // <tmp>/sextile/session: neither level exists
	synced := watchSyncDir(t)
	mustAcquire(t)
	path := filepath.Join(dir, "session.yaml")
	var afterRename bool
	prev := syncDir
	syncDir = func(d string) {
		if _, err := os.Stat(path); d == dir && err == nil {
			afterRename = true
		}
		prev(d)
	}
	t.Cleanup(func() { syncDir = prev })

	if err := SaveSession(Session{Slots: oneSlot(fixtureChart(), false)}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{filepath.Dir(dir), filepath.Dir(filepath.Dir(dir))} {
		if !slices.Contains(*synced, want) {
			t.Errorf("flushed %q, want %s (the parent of a folder the lock created) among them", *synced, want)
		}
	}
	if !afterRename {
		t.Errorf("%s was not flushed after session.yaml was in it", dir)
	}
}

// Only "does not exist" marks a folder as one to create. A stat that fails for
// another reason (a denied or failing disk) says nothing about the folder, so
// it is treated as there, and no parent is flushed on its account.
func TestMkdirAllSyncedCountsOnlyMissingFoldersAsCreated(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kept")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	prevStat := statPath
	statPath = func(p string) (os.FileInfo, error) {
		if p == dir {
			return nil, &os.PathError{Op: "stat", Path: p, Err: syscall.EIO}
		}
		return prevStat(p)
	}
	t.Cleanup(func() { statPath = prevStat })
	synced := watchSyncDir(t)

	if err := mkdirAllSynced(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if len(*synced) != 0 {
		t.Errorf("flushed %q for a folder that was already there", *synced)
	}
}

// A first chart saved into a charts folder that does not exist yet flushes the
// parent of the folder it created, as preferences and session saves do.
func TestFirstChartSaveSyncsTheCreatedFoldersParent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "charts")
	path := filepath.Join(dir, "client"+ChartFileExt)
	var savedAtSync bool
	prev := syncDir
	syncDir = func(d string) {
		if _, err := os.Stat(path); d == dir && err == nil {
			savedAtSync = true
		}
		prev(d)
	}
	t.Cleanup(func() { syncDir = prev })
	synced := watchSyncDir(t)
	if _, err := PlaceChartFile(dir, []byte(astrologRecord), "client"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{filepath.Dir(dir), filepath.Dir(filepath.Dir(dir))} {
		if !slices.Contains(*synced, want) {
			t.Errorf("flushed %q, want %s (the parent of a folder the save created) among them", *synced, want)
		}
	}
	if !savedAtSync {
		t.Errorf("%s was not flushed after %s was in it", dir, path)
	}
}
