// All chart values in this file are constructed synthetic test inputs.
package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// backupFolderEntries are the names inside dir's backup folder.
func backupFolderEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(BackupDir(dir))
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

// stamp is a fixed moment for the dated copies these tests make.
var stamp = time.Date(2026, 11, 13, 9, 18, 0, 0, time.UTC)

// C3.10: the replacement is a same-directory rename, so a fault between the
// write and the rename leaves the original bytes; the result has mode 0600.
func TestChartReplaceIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := seedChart(t, dir, "Equinox-2026-09.as", retitleBefore)

	prev := beforeReplaceRename
	t.Cleanup(func() { beforeReplaceRename = prev })
	beforeReplaceRename = func() error { return errors.New("injected crash before rename") }
	if _, err := OriginalsDoor(dir).Replace("Equinox-2026-09.as", []byte(retitleBefore), []byte(retitleFirst), stamp); err == nil {
		t.Fatal("the injected fault did not surface")
	}
	if got := readString(t, path); got != retitleBefore {
		t.Errorf("after a fault before the rename the chart holds %q, want the original", got)
	}
	onlyEntries(t, dir, "Equinox-2026-09.as")
	beforeReplaceRename = prev

	if _, err := OriginalsDoor(dir).Replace("Equinox-2026-09.as", []byte(retitleBefore), []byte(retitleFirst), stamp.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, path); got != retitleFirst {
		t.Errorf("chart holds %q, want the new record", got)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("chart mode = %v (%v), want 0600", fi.Mode().Perm(), err)
	}
	onlyEntries(t, dir, "Equinox-2026-09.as")
}

// An edit saved while the dated copy was being kept is caught by the check
// just before the rename, and is not renamed over.
func TestChartReplaceChecksLateEdits(t *testing.T) {
	dir := t.TempDir()
	path := seedChart(t, dir, "Equinox-2026-09.as", retitleBefore)
	prev := beforeReplaceRename
	t.Cleanup(func() { beforeReplaceRename = prev })
	beforeReplaceRename = func() error { return os.WriteFile(path, []byte(retitleSecond), 0o600) }
	if _, err := OriginalsDoor(dir).Replace("Equinox-2026-09.as", []byte(retitleBefore), []byte(retitleFirst), stamp); !errors.Is(err, ErrChartChanged) {
		t.Fatalf("err = %v, want ErrChartChanged", err)
	}
	if got := readString(t, path); got != retitleSecond {
		t.Errorf("the late edit was overwritten: %q", got)
	}
	onlyEntries(t, dir, "Equinox-2026-09.as")
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
	if _, err := OriginalsDoor(dir).Replace("Equinox-2026-09.as", []byte(retitleBefore), []byte(retitleFirst), stamp); err == nil {
		t.Fatal("a symlinked chart was replaced")
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the symlink is gone or replaced (%v)", err)
	}
	if got := readString(t, target); got != retitleBefore {
		t.Errorf("the link's target changed: %q", got)
	}
}

// A folder on the way to the backup that is a link must never be trusted,
// even when the link's target already holds a regular file under the chart's
// name: the retitle refuses, and neither the chart nor the file the link
// leads to changes. Each level below the XDG data base is tried:
// sextile, sextile/backup and sextile/backup/<FolderID>.
func TestBackupParentLinksAreRefused(t *testing.T) {
	for _, linked := range []string{"sextile", "backup", "folder"} {
		t.Run(linked+" is a link", func(t *testing.T) {
			data := t.TempDir()
			t.Setenv("XDG_DATA_HOME", data)
			dir := t.TempDir()
			path := seedChart(t, dir, "Equinox-2026-09.as", retitleBefore)
			outside := t.TempDir()
			id := FolderID(dir)
			// Below the link, the outside folder mirrors the layout, so the
			// chart's name resolves to a real file through it.
			var link, inside string
			switch linked {
			case "sextile":
				link, inside = filepath.Join(data, "sextile"), filepath.Join(outside, "backup", id)
			case "backup":
				if err := os.Mkdir(filepath.Join(data, "sextile"), 0o700); err != nil {
					t.Fatal(err)
				}
				link, inside = filepath.Join(data, "sextile", "backup"), filepath.Join(outside, id)
			case "folder":
				if err := os.MkdirAll(filepath.Join(data, "sextile", "backup"), 0o700); err != nil {
					t.Fatal(err)
				}
				link, inside = filepath.Join(data, "sextile", "backup", id), outside
			}
			if err := os.MkdirAll(inside, 0o700); err != nil {
				t.Fatal(err)
			}
			external := filepath.Join(inside, "Equinox-2026-09.as")
			if err := os.WriteFile(external, []byte("somebody else's file\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Fatal(err)
			}
			before := treeOf(t, outside)
			if _, err := OriginalsDoor(dir).Replace("Equinox-2026-09.as", []byte(retitleBefore), []byte(retitleFirst), stamp); err == nil {
				t.Fatal("a retitle went ahead through a linked backup folder")
			}
			if got := readString(t, path); got != retitleBefore {
				t.Errorf("the chart changed: %q", got)
			}
			if got := treeOf(t, outside); got != before {
				t.Errorf("the folder the link leads to changed:\nbefore %s\nafter  %s", before, got)
			}
		})
	}
}

// treeOf lists every path under root with its mode and content, so a test can
// see that nothing under it was created, changed or removed.
func treeOf(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		b.WriteString(rel + " " + fi.Mode().String())
		if fi.Mode().IsRegular() {
			c, _ := os.ReadFile(p)
			b.WriteString(" " + string(c))
		}
		b.WriteString("; ")
		return nil
	})
	return b.String()
}
