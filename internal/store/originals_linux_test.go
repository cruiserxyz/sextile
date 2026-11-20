// All chart values in this file are constructed synthetic test inputs.
package store

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// O24
func TestPlaceNeverLeavesAPartialChart(t *testing.T) {
	dir := t.TempDir()
	prev := linkFile
	t.Cleanup(func() { linkFile = prev })
	linkFile = func(from, to string) error { return &os.LinkError{Op: "link", Old: from, New: to, Err: unix.EPERM} }
	prevR := renameat2
	t.Cleanup(func() { renameat2 = prevR })
	renameat2 = func(int, string, int, string, uint) error { return unix.EINVAL }
	path, err := OriginalsDoor(dir).Place("anna", []byte(doorRecord))
	if err != nil {
		t.Fatal(err)
	}
	if readString(t, path) != doorRecord {
		t.Error("the chart is not the record")
	}
	onlyEntries(t, dir, "anna.as")
	t.Run("a failure removes both files", func(t *testing.T) {
		prevB := beforeReservedRename
		t.Cleanup(func() { beforeReservedRename = prevB })
		beforeReservedRename = func() {
			os.Remove(filepath.Join(dir, "ben.as"))
			os.WriteFile(filepath.Join(dir, "ben.as"), []byte("someone else\n"), 0o600)
		}
		if _, err := OriginalsDoor(dir).Place("ben", []byte(doorRecord)); err == nil {
			t.Fatal("a replaced reservation was renamed over")
		}
		if readString(t, filepath.Join(dir, "ben.as")) != "someone else\n" {
			t.Error("the other file was replaced")
		}
		onlyEntries(t, dir, "anna.as", "ben.as")
	})
}
