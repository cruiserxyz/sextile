package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cruiserxyz/sextile/internal/store"
)

// A dev .env can set XDG_DATA_HOME, and the lock file lives in SessionDir(), so
// the lock is taken after the environment is loaded: it lands in the folder
// every later SessionDir() read uses, not in the one the process started with.
// The stand-in for .env sets XDG_DATA_HOME the way a real one does; this
// proves the order of the two calls, not that a dev build's .env parser
// applies the variable (internal/env's tests do that).
func TestLockIsTakenInTheFolderTheEnvironmentChose(t *testing.T) {
	before := t.TempDir()
	after := t.TempDir()
	t.Setenv("XDG_DATA_HOME", before)
	prev := loadEnv
	loadEnv = func() { os.Setenv("XDG_DATA_HOME", after) }
	t.Cleanup(func() { loadEnv = prev })

	if _, err := acquireLockAfterEnv(); err != nil {
		t.Fatalf("acquireLockAfterEnv: %v", err)
	}

	want := filepath.Join(store.SessionDir(), "sextile.lock")
	if filepath.Dir(want) != filepath.Join(after, "sextile", "session") {
		t.Fatalf("premise: SessionDir() = %s, want it under %s", store.SessionDir(), after)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("no lock file in the session folder the environment chose: %v", err)
	}
	if _, err := os.Stat(filepath.Join(before, "sextile", "session", "sextile.lock")); err == nil {
		t.Errorf("a lock file was also made in the folder the process started with")
	}
}
