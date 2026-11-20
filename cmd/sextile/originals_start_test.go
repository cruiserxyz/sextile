package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/cruiserxyz/sextile/internal/store"
)

// O32: the launcher's word is recorded before the first frame; a SET word that
// cannot be recorded stops the launch; a CREATED folder that cannot be
// recorded stays at ask and starts.
func TestLauncherWordIsRecordedAtStart(t *testing.T) {
	sandbox := func(t *testing.T) string {
		root := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
		t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
		t.Setenv("SEXTILE_ORIGINALS_SET", "")
		t.Setenv("SEXTILE_ORIGINALS_CREATED", "")
		dir := filepath.Join(root, "charts")
		os.MkdirAll(dir, 0o700)
		return dir
	}
	for _, w := range []string{"protected", "ask", "add", "allow"} {
		t.Run("set "+w, func(t *testing.T) {
			dir := sandbox(t)
			t.Setenv("SEXTILE_ORIGINALS_SET", w)
			if err := recordLauncherWord(dir); err != nil {
				t.Fatal(err)
			}
			if r := store.ReadPosition(dir); r.Stored != store.Position(w) {
				t.Errorf("%+v, want %s", r, w)
			}
		})
	}
	t.Run("an unknown word stops the launch", func(t *testing.T) {
		dir := sandbox(t)
		t.Setenv("SEXTILE_ORIGINALS_SET", "open")
		if err := recordLauncherWord(dir); err == nil {
			t.Error("an unknown word was accepted")
		}
		if store.ReadPosition(dir).HasLine {
			t.Error("a line was written")
		}
	})
	t.Run("another window holds the folder", func(t *testing.T) {
		dir := sandbox(t)
		lock := store.BackupDir(dir) + ".writer.lock"
		os.MkdirAll(filepath.Dir(lock), 0o700)
		f, _ := os.OpenFile(lock, os.O_RDWR|os.O_CREATE, 0o600)
		defer f.Close()
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			t.Fatal(err)
		}
		t.Setenv("SEXTILE_ORIGINALS_SET", "protected")
		err := recordLauncherWord(dir)
		if want := "Another Sextile window is open on this charts folder.\nClose it, then start again."; err == nil || err.Error() != want {
			t.Errorf("err = %v, want %q", err, want)
		}
	})
	t.Run("set replaces every earlier line", func(t *testing.T) {
		dir := sandbox(t)
		id, _ := filepath.EvalSymlinks(dir)
		os.MkdirAll(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "sextile"), 0o700)
		os.WriteFile(store.PositionFile(), []byte(id+"=ask\n/x=allow\n"+id+"=add\n"), 0o600)
		t.Setenv("SEXTILE_ORIGINALS_SET", "protected")
		if err := recordLauncherWord(dir); err != nil {
			t.Fatal(err)
		}
		if r := store.ReadPosition(dir); r.Stored != store.PositionProtected || r.Fault != "" {
			t.Errorf("%+v, want one protected line", r)
		}
	})
	t.Run("created", func(t *testing.T) {
		dir := sandbox(t)
		t.Setenv("SEXTILE_ORIGINALS_CREATED", "1")
		if err := recordLauncherWord(dir); err != nil {
			t.Fatal(err)
		}
		if r := store.ReadPosition(dir); r.Stored != store.PositionAdd {
			t.Errorf("%+v, want add", r)
		}
	})
	t.Run("created, the line cannot be written", func(t *testing.T) {
		dir := sandbox(t)
		os.MkdirAll(os.Getenv("XDG_CONFIG_HOME"), 0o700)
		os.WriteFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "sextile"), []byte("a file where the folder goes"), 0o600)
		t.Setenv("SEXTILE_ORIGINALS_CREATED", "1")
		if err := recordLauncherWord(dir); err != nil {
			t.Errorf("a created folder that cannot be recorded stopped the launch: %v", err)
		}
		if r := store.ReadPosition(dir); r.Effective != store.PositionAsk {
			t.Errorf("%+v, want ask", r)
		}
	})
}
