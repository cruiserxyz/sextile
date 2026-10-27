package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/cruiserxyz/sextile/internal/store"
)

// A real held lock: another open file description holds the flock, so the
// launch's own AcquireInstanceLock gets LockHeldError, and main runs it as a
// second window rather than refusing.
func TestHeldLockStartsASecondWindow(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_STATE_HOME", data)
	holdSessionLock(t, data)
	lock, err := store.AcquireInstanceLock()
	var held *store.LockHeldError
	if !errors.As(err, &held) {
		t.Fatalf("premise: AcquireInstanceLock = %v, want LockHeldError", err)
	}
	cfg, refused := lockOutcome(lock, err)
	if refused != nil {
		t.Fatalf("a held lock refused the launch: %v", refused)
	}
	if !cfg.SecondWindow || cfg.InstanceLockUnsupported {
		t.Errorf("config = %+v, want SecondWindow only", cfg)
	}
}

// Every other answer keeps its meaning: a lock taken is a first window, an
// unsupported filesystem runs and says so, and every error refuses as it is.
func TestOtherLockOutcomesAreUnchanged(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_STATE_HOME", data)
	lock, err := store.AcquireInstanceLock()
	if err != nil {
		t.Fatalf("premise: a free lock was refused: %v", err)
	}
	if cfg, refused := lockOutcome(lock, err); refused != nil || cfg.SecondWindow || cfg.InstanceLockUnsupported {
		t.Errorf("a taken lock: config %+v, refused %v; want a plain first window", cfg, refused)
	}
	if cfg, refused := lockOutcome(&store.InstanceLock{Unsupported: true}, nil); refused != nil || cfg.SecondWindow || !cfg.InstanceLockUnsupported {
		t.Errorf("an unsupported filesystem: config %+v, refused %v", cfg, refused)
	}

	// A real refusal: a FIFO where the lock file belongs is not safe to use.
	fifoData := t.TempDir()
	t.Setenv("XDG_STATE_HOME", fifoData)
	dir := filepath.Join(fifoData, "sextile", "session")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "sextile.lock"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, fifoErr := store.AcquireInstanceLock()
	path := "/x/sextile.lock"
	for name, e := range map[string]error{
		"fifo (real)":   fifoErr,
		"unsafe":        &store.LockError{Kind: store.LockUnsafe, Path: path},
		"cannot open":   &store.LockError{Kind: store.LockOpen, Path: path, Err: syscall.EACCES},
		"flock error":   &store.LockError{Kind: store.LockFlock, Path: path, Err: syscall.EIO},
		"pid not saved": &store.LockError{Kind: store.LockRecord, Path: path, Err: syscall.EIO},
		"unclassified":  errors.New("boom"),
	} {
		cfg, refused := lockOutcome(nil, e)
		if e == nil || refused != e {
			t.Errorf("%s: refused = %v, want the lock's own error %v", name, refused, e)
		}
		if cfg.SecondWindow {
			t.Errorf("%s: became a second window", name)
		}
	}
}
