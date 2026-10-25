package store

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Two Sextile windows share the chart folder and preferences.yaml (the
// session is the lock holder's alone, and the UI gates it). These pin the two
// writes both windows may make.

// Two windows saving a new chart under the same name: the exclusive create
// lets exactly one of them have it, and the loser leaves the winner's bytes.
func TestTwoWindowsSavingOneNameKeepTheFirst(t *testing.T) {
	dir := t.TempDir()
	const first = "@AI800  ; the first window's chart\n"
	const second = "@AI800  ; the second window's chart\n"
	path, err := PlaceChartFile(dir, []byte(first), "client")
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	if _, err := PlaceChartFile(dir, []byte(second), "client"); !errors.Is(err, ErrChartExists) {
		t.Fatalf("second save: err = %v, want ErrChartExists", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != first {
		t.Errorf("the second save changed the first window's chart:\n got: %q\nwant: %q", got, first)
	}
}

// The `, c d` writes (the default city, then the house display) replace
// preferences.yaml by renaming a temp file from its own folder over it: the
// file is never written in place, so the other window reads the old file or
// the new one, never half of either.
func TestPreferencesWritesReplaceByRename(t *testing.T) {
	path := prefPath(t)
	seedPreferences(t, path, clockBlock)
	inode := func() uint64 {
		t.Helper()
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		return fi.Sys().(*syscall.Stat_t).Ino
	}
	for name, write := range map[string]func() error{
		"default city":  func() error { return SetLocation(berlin) },
		"house display": func() error { return SetHouseDisplay("number") },
	} {
		t.Run(name, func(t *testing.T) {
			before, was := readPreferences(t, path), inode()
			renamed := 0
			prev := renameFile
			renameFile = func(from, to string) error {
				renamed++
				if filepath.Dir(from) != filepath.Dir(path) || to != path {
					t.Errorf("renamed %s to %s, want a temp file beside %s", from, to, path)
				}
				if got := readPreferences(t, path); got != before {
					t.Errorf("the file was written in place before the rename:\n%s", got)
				}
				return prev(from, to)
			}
			t.Cleanup(func() { renameFile = prev })
			if err := write(); err != nil {
				t.Fatal(err)
			}
			if renamed != 1 {
				t.Errorf("%d renames, want 1", renamed)
			}
			if inode() == was {
				t.Error("preferences.yaml kept its inode: it was rewritten in place, not replaced")
			}
		})
	}
}
