// All chart values in this file are constructed synthetic test inputs.
package astrolog

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A TMPDIR inside the charts folder puts nothing there: a chart read, a save's
// record and a rename's record all use Sextile's private temp folder.
func TestTempFilesIgnoreTMPDIR(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog is not on PATH")
	}
	charts := t.TempDir()
	src := filepath.Join(charts, "Anna.as")
	if err := os.WriteFile(src, []byte("@AI780  ; Astrolog chart info.\n-qb Sep 23 2026 2:06 DT 1E   8:48E 53:04N\n-zi \"Anna\" \"Bremen\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A stand-in astrolog lists the charts folder each time it runs, so a
	// temp folder that exists only during a call is seen.
	real, _ := exec.LookPath("astrolog")
	shim, log := t.TempDir(), filepath.Join(t.TempDir(), "listing")
	script := "#!/bin/sh\nls -A " + strconv.Quote(charts) + " >> " + strconv.Quote(log) + "\nexec " + strconv.Quote(real) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(shim, "astrolog"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shim+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMPDIR", charts)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	before := folderSnapshot(t, charts)
	if _, err := ResolveChartFile(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	if _, err := ChartRecord(context.Background(), ChartToSave{Name: "Ben", Place: "Bremen", When: time.Date(1990, 1, 2, 3, 4, 0, 0, time.UTC), Lon: 8.8, Lat: 53.07}); err != nil {
		t.Fatal(err)
	}
	if _, err := RetitleRecord(context.Background(), src, "Anna Two", "Bremen"); err != nil {
		t.Fatal(err)
	}
	listing, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range strings.Fields(string(listing)) {
		if name != "Anna.as" {
			t.Errorf("while Astrolog ran, the charts folder held %s", name)
		}
	}
	if after := folderSnapshot(t, charts); after != before {
		t.Errorf("the charts folder changed:\n--- before\n%s\n--- after\n%s", before, after)
	}
	base, err := PrivateTempBase()
	if err != nil || !strings.HasPrefix(base, os.Getenv("XDG_CACHE_HOME")) {
		t.Errorf("private temp base %q (%v), want it under XDG_CACHE_HOME", base, err)
	}
}

func folderSnapshot(t *testing.T, dir string) string {
	t.Helper()
	var rows []string
	filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		row := fmt.Sprintf("%s %v %d", rel, fi.Mode().Type(), fi.Size())
		if fi.Mode().IsRegular() {
			b, _ := os.ReadFile(p)
			row += fmt.Sprintf(" %x", sha256.Sum256(b))
		}
		rows = append(rows, row)
		return nil
	})
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}

// The two folders below the cache base are real folders, never followed
// through a link, and the last one is private.
func TestPrivateTempBaseIsPrivate(t *testing.T) {
	t.Run("a link", func(t *testing.T) {
		cache, elsewhere := t.TempDir(), t.TempDir()
		t.Setenv("XDG_CACHE_HOME", cache)
		os.Mkdir(filepath.Join(cache, "sextile"), 0o700)
		if err := os.Symlink(elsewhere, filepath.Join(cache, "sextile", "tmp")); err != nil {
			t.Fatal(err)
		}
		if _, err := PrivateTempBase(); err == nil {
			t.Error("a linked temp folder was used")
		}
	})
	t.Run("an open mode", func(t *testing.T) {
		cache := t.TempDir()
		t.Setenv("XDG_CACHE_HOME", cache)
		os.MkdirAll(filepath.Join(cache, "sextile", "tmp"), 0o755)
		os.Chmod(filepath.Join(cache, "sextile", "tmp"), 0o755)
		base, err := PrivateTempBase()
		if err != nil {
			t.Fatal(err)
		}
		if fi, _ := os.Stat(base); fi.Mode().Perm() != 0o700 {
			t.Errorf("mode %v, want 0700", fi.Mode().Perm())
		}
	})
}

// A cache folder that cannot be made fails a chart read with a
// *PrivateTempError naming that folder: no copy of the chart is made anywhere
// else. HOME=/ with no XDG_CACHE_HOME is what a Docker --user with no home
// entry gets.
func TestNoUsableCacheFailsTheReadNamingTheFolder(t *testing.T) {
	ro := filepath.Join(t.TempDir(), "home")
	if err := os.Mkdir(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", ro)
	if err := os.Mkdir(filepath.Join(ro, "probe"), 0o700); err == nil {
		t.Skip("this user can write a mode-0555 folder")
	}
	_, err := ResolveChartFile(context.Background(), fixture("Equinox-2026-03.as"))
	var pt *PrivateTempError
	if !errors.As(err, &pt) {
		t.Fatalf("err = %v, want a *PrivateTempError", err)
	}
	if pt.Dir != filepath.Join(ro, ".cache") {
		t.Errorf("Dir = %q, want %q", pt.Dir, filepath.Join(ro, ".cache"))
	}
	t.Setenv("HOME", "relative")
	if _, err := PrivateTempBase(); !errors.As(err, &pt) || pt.Dir != "" || !errors.Is(err, ErrHomeNotAbsolute) {
		t.Errorf("a relative HOME: err %v, Dir %q, want a *PrivateTempError with no folder", err, pt.Dir)
	}
}

// A cache base in a folder anyone may write in is used only when it is a real
// folder of this user's own: a link there (into a charts folder, say) is
// refused before anything is made through it.
func TestCacheBaseInASharedFolderMustBeOwnFolder(t *testing.T) {
	shared := filepath.Join(t.TempDir(), "tmp")
	if err := os.Mkdir(shared, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0o1777); err != nil {
		t.Fatal(err)
	}
	charts := t.TempDir()
	base := filepath.Join(shared, "sextile-cache")
	if err := os.Symlink(charts, base); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", base)
	_, err := PrivateTempBase()
	var pt *PrivateTempError
	if !errors.As(err, &pt) {
		t.Fatalf("a linked base in a shared folder: err %v, want a *PrivateTempError", err)
	}
	if entries, _ := os.ReadDir(charts); len(entries) != 0 {
		t.Errorf("something was made through the link: %v", entries)
	}
	own := filepath.Join(shared, "own-cache")
	if err := os.Mkdir(own, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(own, 0o777); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", own)
	if _, err := PrivateTempBase(); err != nil {
		t.Errorf("a base this user made in the shared folder: %v", err)
	}
	if fi, _ := os.Stat(own); fi.Mode().Perm()&0o077 != 0 {
		t.Errorf("a base others may write in was used as it is: mode %v", fi.Mode().Perm())
	}
}
