// All chart values in this file are constructed synthetic test inputs.
package store

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	retitleBefore = "@AI780  ; Astrolog chart info.\n-qb Mar 20 1995 12:00pm DT 1E   8:48E 53:04N\n-zi \"\" \"Bremen, Germany\"\n"
	retitleFirst  = "@AI800  ; Astrolog chart info.\n-qb Mar 20 1995 12:00pm DT 1E   8:48E 53:04N\n-zi \"Constructed One\" \"Bremen, Germany\"\n"
	retitleSecond = "@AI800  ; Astrolog chart info.\n-qb Mar 20 1995 12:00pm DT 1E   8:48E 53:04N\n-zi \"Constructed Two\" \"Bremen, Germany\"\n"
)

func seedChart(t *testing.T, dir, file, body string) string {
	t.Helper()
	path := filepath.Join(dir, file)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// tempHolding is a chart temp file holding body, as Astrolog would leave it.
func tempHolding(t *testing.T, dir, body string) string {
	t.Helper()
	tmp, err := CreateChartTemp(dir, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return tmp
}

// backupOf is where the backup of the chart at path is kept.
func backupOf(path string) string {
	return filepath.Join(filepath.Dir(path), ChartBackupRel(filepath.Base(path)))
}

// backupFolderEntries are the names inside dir's backup folder.
func backupFolderEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, ChartBackupFolderRel()))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	return got
}

func readString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func onlyEntries(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("directory holds %q, want %q", got, want)
	}
}

// C3.9: the first original is kept as .sextile/backup/<file>, with an
// exclusive create at mode 0600, and a later write never replaces it. A short
// write leaves no backup and the chart unchanged.
func TestChartBackupKeepsTheFirst(t *testing.T) {
	t.Run("two retitles", func(t *testing.T) {
		dir := t.TempDir()
		path := seedChart(t, dir, "Equinox-2026-09.as", retitleBefore)

		backedUp, err := ReplaceChartWithBackup(dir, "Equinox-2026-09.as", tempHolding(t, dir, retitleFirst), []byte(retitleBefore))
		if err != nil || !backedUp {
			t.Fatalf("first retitle: backedUp=%v err=%v", backedUp, err)
		}
		backedUp, err = ReplaceChartWithBackup(dir, "Equinox-2026-09.as", tempHolding(t, dir, retitleSecond), []byte(retitleFirst))
		if err != nil || backedUp {
			t.Fatalf("second retitle: backedUp=%v err=%v; want no new backup", backedUp, err)
		}
		if got := readString(t, backupOf(path)); got != retitleBefore {
			t.Errorf("the backup holds %q, want the bytes from before the FIRST write", got)
		}
		if got := readString(t, path); got != retitleSecond {
			t.Errorf("chart holds %q, want the second record", got)
		}
		fi, err := os.Stat(backupOf(path))
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("backup mode = %v (%v), want 0600", fi.Mode().Perm(), err)
		}
		onlyEntries(t, dir, ".sextile", "Equinox-2026-09.as")
		if got := backupFolderEntries(t, dir); strings.Join(got, ",") != "Equinox-2026-09.as" {
			t.Errorf("the backup folder holds %q, want only Equinox-2026-09.as", got)
		}
	})

	t.Run("short write", func(t *testing.T) {
		dir := t.TempDir()
		path := seedChart(t, dir, "Equinox-2026-09.as", retitleBefore)
		prev := writeBackup
		writeBackup = func(w io.Writer, b []byte) (int, error) { return w.Write(b[:len(b)/2]) }
		t.Cleanup(func() { writeBackup = prev })

		tmp := tempHolding(t, dir, retitleFirst)
		if _, err := ReplaceChartWithBackup(dir, "Equinox-2026-09.as", tmp, []byte(retitleBefore)); !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("err = %v, want a short-write refusal", err)
		}
		if _, err := os.Stat(backupOf(path)); !os.IsNotExist(err) {
			t.Errorf("a partial backup was left behind (stat err %v)", err)
		}
		if got := readString(t, path); got != retitleBefore {
			t.Errorf("the chart changed after a refused backup: %q", got)
		}
	})

	// An existing backup that is not a regular file cannot hold the original,
	// so the write is refused rather than going ahead with no backup.
	for _, kind := range []string{"directory", "symlink"} {
		t.Run("the backup is a "+kind, func(t *testing.T) {
			dir := t.TempDir()
			path := seedChart(t, dir, "Equinox-2026-09.as", retitleBefore)
			if err := os.MkdirAll(filepath.Dir(backupOf(path)), 0o700); err != nil {
				t.Fatal(err)
			}
			if kind == "directory" {
				if err := os.Mkdir(backupOf(path), 0o700); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Symlink(filepath.Join(dir, "elsewhere"), backupOf(path)); err != nil {
				t.Fatal(err)
			}
			if _, err := ReplaceChartWithBackup(dir, "Equinox-2026-09.as", tempHolding(t, dir, retitleFirst), []byte(retitleBefore)); err == nil {
				t.Fatal("a write went ahead with no file holding the original")
			}
			if got := readString(t, path); got != retitleBefore {
				t.Errorf("the chart changed: %q", got)
			}
		})
	}

	// The backup is never visible half-written: a short write leaves no file
	// of that name, and no staged temp behind either.
	t.Run("no staged backup left", func(t *testing.T) {
		dir := t.TempDir()
		seedChart(t, dir, "Equinox-2026-09.as", retitleBefore)
		prev := writeBackup
		t.Cleanup(func() { writeBackup = prev })
		writeBackup = func(w io.Writer, b []byte) (int, error) { return w.Write(b[:1]) }
		tmp := tempHolding(t, dir, retitleFirst)
		ReplaceChartWithBackup(dir, "Equinox-2026-09.as", tmp, []byte(retitleBefore))
		os.Remove(tmp)
		onlyEntries(t, dir, ".sextile", "Equinox-2026-09.as")
		if got := backupFolderEntries(t, dir); len(got) != 0 {
			t.Errorf("a short write left %q in the backup folder", got)
		}
	})

	t.Run("changed underneath", func(t *testing.T) {
		dir := t.TempDir()
		path := seedChart(t, dir, "Equinox-2026-09.as", retitleSecond)
		tmp := tempHolding(t, dir, retitleFirst)
		if _, err := ReplaceChartWithBackup(dir, "Equinox-2026-09.as", tmp, []byte(retitleBefore)); !errors.Is(err, ErrChartChanged) {
			t.Fatalf("err = %v, want ErrChartChanged", err)
		}
		if got := readString(t, path); got != retitleSecond {
			t.Errorf("a changed chart was replaced: %q", got)
		}
		if _, err := os.Stat(backupOf(path)); !os.IsNotExist(err) {
			t.Errorf("a refused write made a backup (stat err %v)", err)
		}
	})
}

// C3.10: the replacement is a same-directory rename, so a fault between the
// write and the rename leaves the original bytes; the result has mode 0600.
func TestChartReplaceIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := seedChart(t, dir, "Equinox-2026-09.as", retitleBefore)

	prev := beforeReplaceRename
	t.Cleanup(func() { beforeReplaceRename = prev })
	beforeReplaceRename = func() error { return errors.New("injected crash before rename") }
	tmp := tempHolding(t, dir, retitleFirst)
	if filepath.Dir(tmp) != dir {
		t.Fatalf("temp file %s is not in the chart's own directory", tmp)
	}
	if _, err := ReplaceChartWithBackup(dir, "Equinox-2026-09.as", tmp, []byte(retitleBefore)); err == nil {
		t.Fatal("the injected fault did not surface")
	}
	if got := readString(t, path); got != retitleBefore {
		t.Errorf("after a fault before the rename the chart holds %q, want the original", got)
	}
	beforeReplaceRename = prev

	// A temp file a looser umask or another writer left wider than 0600: the
	// replacement must still end at 0600.
	if err := os.Chmod(tmp, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReplaceChartWithBackup(dir, "Equinox-2026-09.as", tmp, []byte(retitleBefore)); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, path); got != retitleFirst {
		t.Errorf("chart holds %q, want the new record", got)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("chart mode = %v (%v), want 0600", fi.Mode().Perm(), err)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("the temp name survived the rename (stat err %v)", err)
	}
}

// RemoveChartTemp removes only a temp file this package made, never a chart.
func TestRemoveChartTempRefusesAChart(t *testing.T) {
	dir := t.TempDir()
	path := seedChart(t, dir, "Equinox-2026-09.as", retitleBefore)
	if err := RemoveChartTemp(path); err == nil {
		t.Error("RemoveChartTemp removed a chart")
	}
	if got := readString(t, path); got != retitleBefore {
		t.Errorf("the chart changed: %q", got)
	}
}

// An edit saved while the backup was being staged is caught by the check just
// before the rename, and is not renamed over.
func TestChartReplaceChecksLateEdits(t *testing.T) {
	dir := t.TempDir()
	path := seedChart(t, dir, "Equinox-2026-09.as", retitleBefore)
	prev := beforeReplaceRename
	t.Cleanup(func() { beforeReplaceRename = prev })
	beforeReplaceRename = func() error { return os.WriteFile(path, []byte(retitleSecond), 0o600) }
	tmp := tempHolding(t, dir, retitleFirst)
	if _, err := ReplaceChartWithBackup(dir, "Equinox-2026-09.as", tmp, []byte(retitleBefore)); !errors.Is(err, ErrChartChanged) {
		t.Fatalf("err = %v, want ErrChartChanged", err)
	}
	if got := readString(t, path); got != retitleSecond {
		t.Errorf("the late edit was overwritten: %q", got)
	}
	// The refused attempt takes back the backup it made, so the next write
	// keeps the version it actually overwrites. (The temp is the caller's to
	// remove, as lookup does on every path.)
	os.Remove(tmp)
	onlyEntries(t, dir, ".sextile", "Equinox-2026-09.as")
	if got := backupFolderEntries(t, dir); len(got) != 0 {
		t.Errorf("the refused attempt left %q in the backup folder", got)
	}
	beforeReplaceRename = prev
	if _, err := ReplaceChartWithBackup(dir, "Equinox-2026-09.as", tempHolding(t, dir, retitleFirst), []byte(retitleSecond)); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, backupOf(path)); got != retitleSecond {
		t.Errorf("backup = %q, want the version the successful write overwrote", got)
	}
}

// A chart that is a symlink is refused, so the link is never replaced by a
// regular file.
func TestChartReplaceRefusesASymlink(t *testing.T) {
	dir := t.TempDir()
	target := seedChart(t, dir, "target.as.real", retitleBefore)
	link := filepath.Join(dir, "Equinox-2026-09.as")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReplaceChartWithBackup(dir, "Equinox-2026-09.as", tempHolding(t, dir, retitleFirst), []byte(retitleBefore)); err == nil {
		t.Fatal("a symlinked chart was replaced")
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the symlink is gone or replaced (%v)", err)
	}
	if got := readString(t, target); got != retitleBefore {
		t.Errorf("the link's target changed: %q", got)
	}
}

// ChartBackupState reads and never writes: absent, a file, and a directory
// that ReplaceChartWithBackup would refuse.
func TestChartBackupState(t *testing.T) {
	dir := t.TempDir()
	if held, err := ChartBackupState(dir, "Equinox-2026-09.as"); held || err != nil {
		t.Errorf("absent: held %v, err %v", held, err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ChartBackupFolderRel()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ChartBackupRel("Equinox-2026-09.as")), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if held, err := ChartBackupState(dir, "Equinox-2026-09.as"); !held || err != nil {
		t.Errorf("a file: held %v, err %v", held, err)
	}
	if err := os.Mkdir(filepath.Join(dir, ChartBackupRel("G.as")), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ChartBackupState(dir, "G.as"); err == nil {
		t.Error("a directory was accepted as a backup")
	}
	if got := backupFolderEntries(t, dir); len(got) != 2 {
		t.Errorf("the backup folder holds %q after three reads, want the 2 it started with", got)
	}
}

// A `.sextile` or `.sextile/backup` that is a link must never be trusted,
// even when the link's target already holds a regular file under the chart's
// name: the retitle refuses, and neither the chart nor the file the link
// leads to changes.
func TestBackupParentLinksAreRefused(t *testing.T) {
	for _, linked := range []string{"state", "backup"} {
		t.Run(linked+" is a link", func(t *testing.T) {
			dir := t.TempDir()
			path := seedChart(t, dir, "Equinox-2026-09.as", retitleBefore)
			outside := t.TempDir()
			external := filepath.Join(outside, "Equinox-2026-09.as")
			if err := os.WriteFile(external, []byte("somebody else's file\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			switch linked {
			case "state":
				// <dir>/.sextile -> outside, which holds backup/Equinox-2026-09.as
				if err := os.MkdirAll(filepath.Join(outside, "backup"), 0o700); err != nil {
					t.Fatal(err)
				}
				external = filepath.Join(outside, "backup", "Equinox-2026-09.as")
				if err := os.WriteFile(external, []byte("somebody else's file\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(dir, ".sextile")); err != nil {
					t.Fatal(err)
				}
			case "backup":
				// <dir>/.sextile/backup -> outside, which holds Equinox-2026-09.as
				if err := os.Mkdir(filepath.Join(dir, ".sextile"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(dir, ".sextile", "backup")); err != nil {
					t.Fatal(err)
				}
			}
			tmp := tempHolding(t, dir, retitleFirst)
			if _, err := ReplaceChartWithBackup(dir, "Equinox-2026-09.as", tmp, []byte(retitleBefore)); err == nil {
				t.Fatal("a retitle went ahead through a linked backup folder")
			}
			if got := readString(t, path); got != retitleBefore {
				t.Errorf("the chart changed: %q", got)
			}
			if got := readString(t, external); got != "somebody else's file\n" {
				t.Errorf("the file the link leads to changed: %q", got)
			}
			if _, err := ChartBackupState(dir, "Equinox-2026-09.as"); err == nil {
				t.Error("ChartBackupState trusted a backup found through a link")
			}
		})
	}
}
