//go:build linux

// All chart values in this file are constructed synthetic test inputs.
package store

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

// renameat2Errnos are the answers that mean "no RENAME_NOREPLACE here"
// (ENOTSUP is EOPNOTSUPP on Linux).
var renameat2Errnos = []syscall.Errno{unix.EINVAL, unix.EPERM, unix.EOPNOTSUPP, unix.ENOSYS}

// stubRenameat2 makes renameat2 answer errno for the rest of the test.
func stubRenameat2(t *testing.T, errno syscall.Errno) {
	t.Helper()
	prev := renameat2
	renameat2 = func(int, string, int, string, uint) error { return errno }
	t.Cleanup(func() { renameat2 = prev })
}

// assertOnlyFile fails for any entry in dir other than the file at path.
func assertOnlyFile(t *testing.T, dir, path string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != filepath.Base(path) {
			t.Errorf("the saves left %s in the charts folder", e.Name())
		}
	}
}

// On a filesystem without RENAME_NOREPLACE the save still lands, by a hard
// link, and still refuses a taken name.
func TestPlaceChartFileFallsBackToALinkWithoutNoReplace(t *testing.T) {
	for _, errno := range renameat2Errnos {
		t.Run(unix.ErrnoName(errno), func(t *testing.T) {
			dir := t.TempDir()
			stubRenameat2(t, errno)
			linked := 0
			prev := linkFile
			linkFile = func(from, to string) error { linked++; return prev(from, to) }
			t.Cleanup(func() { linkFile = prev })

			path, err := PlaceChartFile(dir, []byte(astrologRecord), "client")
			if err != nil {
				t.Fatalf("first save: %v", err)
			}
			if linked != 1 {
				t.Errorf("%d links, want the save to land by one", linked)
			}
			if _, err := PlaceChartFile(dir, []byte("@AI800  ; another chart\n"), "client"); !errors.Is(err, ErrChartExists) {
				t.Fatalf("second save: err = %v, want ErrChartExists", err)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != astrologRecord {
				t.Errorf("the chart holds %q (%v), want the first record", got, err)
			}
			assertOnlyFile(t, dir, path)
		})
	}
}

// On a filesystem with neither RENAME_NOREPLACE nor hard links (exFAT, some
// FUSE mounts) the save creates the chart's name exclusively and writes into
// it: it still lands whole, and still refuses a taken name.
func TestPlaceChartFileWritesInPlaceWithoutAnyNoReplaceMove(t *testing.T) {
	for _, errno := range []syscall.Errno{
		unix.EPERM, unix.EOPNOTSUPP, unix.ENOSYS, unix.EXDEV, unix.EINVAL,
	} {
		t.Run(unix.ErrnoName(errno), func(t *testing.T) {
			dir := t.TempDir()
			stubRenameat2(t, unix.EINVAL)
			prev := linkFile
			linkFile = func(from, to string) error { return &os.LinkError{Op: "link", Old: from, New: to, Err: errno} }
			t.Cleanup(func() { linkFile = prev })

			path, err := PlaceChartFile(dir, []byte(astrologRecord), "client")
			if err != nil {
				t.Fatalf("first save: %v", err)
			}
			if _, err := PlaceChartFile(dir, []byte("@AI800  ; another chart\n"), "client"); !errors.Is(err, ErrChartExists) {
				t.Fatalf("second save: err = %v, want ErrChartExists", err)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != astrologRecord {
				t.Errorf("the chart holds %q (%v), want the first record", got, err)
			}
			assertOnlyFile(t, dir, path)
		})
	}
}

// TestTooManyLinksIsNotUnsupported keeps EMLINK, an answer of a filesystem
// that does hard links, from sending a save down the in-place path.
func TestTooManyLinksIsNotUnsupported(t *testing.T) {
	if linkUnsupported(unix.EMLINK) {
		t.Fatal("EMLINK counts as \"no hard links\"; the save would write in place")
	}
}
