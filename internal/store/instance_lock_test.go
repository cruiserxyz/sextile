package store

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// the specification C1, C3, C6 and their build notes. The lock is taken
// here by the same function the shipped binary calls; the process-level proof
// that main calls it first is internal/harness.

// lockSandbox points the session at a fresh directory and returns the lock path.
func lockSandbox(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	return filepath.Join(SessionDir(), instanceLockFile)
}

// releaseLock gives a lock back, which the shipped binary never does (the OS
// does, at exit). Tests need it to take the lock again.
func releaseLock(t *testing.T, l *InstanceLock) {
	t.Helper()
	if l == nil || l.file == nil {
		return
	}
	l.file.Close()
	if heldLock == l {
		heldLock = nil
	}
}

func mustAcquire(t *testing.T) *InstanceLock {
	t.Helper()
	l, err := AcquireInstanceLock()
	if err != nil {
		t.Fatalf("AcquireInstanceLock: %v", err)
	}
	t.Cleanup(func() { releaseLock(t, l) })
	return l
}

// holdRaw takes the flock on path from a descriptor the test owns, leaving the
// file's content as given: the holder this process cannot be, because the real
// one always writes a good pid.
func holdRaw(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte(content), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
}

func wantLockError(t *testing.T, err error, kind LockKind) *LockError {
	t.Helper()
	var le *LockError
	if !errors.As(err, &le) {
		t.Fatalf("err = %v (%T), want a *LockError", err, err)
	}
	if le.Kind != kind {
		t.Fatalf("kind = %d, want %d (%v)", le.Kind, kind, err)
	}
	return le
}

func TestInstanceLockIsExclusive(t *testing.T) {
	t.Run("a second take is refused and names the holder", func(t *testing.T) {
		lockSandbox(t)
		first := mustAcquire(t)
		_, err := AcquireInstanceLock()
		var held *LockHeldError
		if !errors.As(err, &held) {
			t.Fatalf("second take: err = %v, want *LockHeldError", err)
		}
		if held.PID != os.Getpid() {
			t.Errorf("holder pid = %d, want %d", held.PID, os.Getpid())
		}
		runtime.KeepAlive(first)
	})

	t.Run("the lock is free again once the holder lets go", func(t *testing.T) {
		lockSandbox(t)
		releaseLock(t, mustAcquire(t))
		mustAcquire(t)
	})

	t.Run("the file is created 0600, in a session folder that did not exist", func(t *testing.T) {
		path := lockSandbox(t)
		mustAcquire(t)
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v, want 0600", st.Mode().Perm())
		}
	})

	chart := func(t *testing.T, path string) (string, []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		dat := filepath.Join(t.TempDir(), "client.as")
		body := []byte("; a client's chart\n-i client\n")
		if err := os.WriteFile(dat, body, 0o644); err != nil {
			t.Fatal(err)
		}
		return dat, body
	}
	stat := func(t *testing.T, p string) (time.Time, os.FileMode) {
		t.Helper()
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		return st.ModTime(), st.Mode()
	}

	t.Run("the lock path a symlink to a chart file", func(t *testing.T) {
		path := lockSandbox(t)
		dat, body := chart(t, path)
		mt, mode := stat(t, dat)
		if err := os.Symlink(dat, path); err != nil {
			t.Fatal(err)
		}
		_, err := AcquireInstanceLock()
		wantLockError(t, err, LockUnsafe)
		got, _ := os.ReadFile(dat)
		mt2, mode2 := stat(t, dat)
		if !bytes.Equal(got, body) || !mt2.Equal(mt) || mode2 != mode {
			t.Errorf("the chart file behind the symlink changed: %q, %v, %v", got, mt2, mode2)
		}
	})

	t.Run("the lock path a dangling symlink is not created through", func(t *testing.T) {
		path := lockSandbox(t)
		chart(t, path)
		target := filepath.Join(t.TempDir(), "not-there")
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		_, err := AcquireInstanceLock()
		wantLockError(t, err, LockUnsafe)
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Errorf("the symlink's target was created: %v", err)
		}
	})

	t.Run("the lock path a HARD link to a chart file", func(t *testing.T) {
		path := lockSandbox(t)
		dat, body := chart(t, path)
		mt, mode := stat(t, dat)
		if err := os.Link(dat, path); err != nil {
			t.Fatal(err)
		}
		_, err := AcquireInstanceLock()
		wantLockError(t, err, LockUnsafe)
		got, _ := os.ReadFile(dat)
		mt2, mode2 := stat(t, dat)
		if !bytes.Equal(got, body) || !mt2.Equal(mt) || mode2 != mode {
			t.Errorf("the chart file behind the hard link changed: %q, %v, %v", got, mt2, mode2)
		}
	})

	t.Run("the lock path a directory", func(t *testing.T) {
		path := lockSandbox(t)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := AcquireInstanceLock()
		wantLockError(t, err, LockUnsafe)
	})

	t.Run("a pid write failure refuses and lets go of the lock", func(t *testing.T) {
		lockSandbox(t)
		instanceLockFault = "pidwrite"
		_, err := AcquireInstanceLock()
		instanceLockFault = ""
		wantLockError(t, err, LockRecord)
		// The lock was taken before the write failed; refusing must give it back,
		// or this process would block itself and the next launch.
		mustAcquire(t)
	})
}

// A FIFO at the lock path, with no writer, must refuse at once: a blocking
// open would hang the launch before any screen exists (pass 7).
func TestInstanceLockFIFORefusesPromptly(t *testing.T) {
	path := lockSandbox(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := AcquireInstanceLock()
		done <- err
	}()
	select {
	case err := <-done:
		wantLockError(t, err, LockUnsafe)
	case <-time.After(5 * time.Second):
		t.Fatal("a FIFO at the lock path hung the launch")
	}
}

func TestInstanceLockNamesItsHolder(t *testing.T) {
	for _, c := range []struct {
		name, content string
		want          int
	}{
		{"a pid and a newline", "123\n", 123},
		{"a pid, no newline", "123", 123},
		{"the largest pid Linux allows", "4194304\n", 4194304},
		{"empty", "", 0},
		{"only a newline", "\n", 0},
		{"two words", "12 34\n", 0},
		{"two lines", "12\n34\n", 0},
		{"a blank line after", "123\n\n", 0},
		{"letters", "abc\n", 0},
		{"digits then letters", "12abc\n", 0},
		{"negative", "-5\n", 0},
		{"signed", "+12\n", 0},
		{"zero", "0\n", 0},
		{"a leading zero", "007\n", 0},
		{"a leading space", " 12\n", 0},
		{"a carriage return", "12\r\n", 0},
		{"beyond any pid", "4194305\n", 0},
		{"beyond any integer", "99999999999999999999\n", 0},
		{"a long run of digits", string(bytes.Repeat([]byte("1"), 4096)) + "\n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := lockSandbox(t)
			holdRaw(t, path, c.content)
			_, err := AcquireInstanceLock()
			var held *LockHeldError
			if !errors.As(err, &held) {
				t.Fatalf("err = %v, want *LockHeldError", err)
			}
			if held.PID != c.want {
				t.Errorf("PID for %q = %d, want %d", c.content, held.PID, c.want)
			}
		})
	}

	t.Run("the real holder writes its own pid", func(t *testing.T) {
		path := lockSandbox(t)
		mustAcquire(t)
		got, _ := os.ReadFile(path)
		if want := strconv.Itoa(os.Getpid()) + "\n"; string(got) != want {
			t.Errorf("lock file = %q, want %q", got, want)
		}
	})
}

// A holder with a shorter pid than the file's old content must leave only its
// own pid, not the old pid's tail.
func TestInstanceLockPidIsTruncated(t *testing.T) {
	path := lockSandbox(t)
	stale := "99999999999999999999999999\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(stale), 0o600); err != nil {
		t.Fatal(err)
	}
	mustAcquire(t)
	got, _ := os.ReadFile(path)
	if want := fmt.Sprintf("%d\n", os.Getpid()); string(got) != want {
		t.Errorf("lock file = %q, want exactly %q", got, want)
	}
	_, err := AcquireInstanceLock()
	var held *LockHeldError
	if !errors.As(err, &held) || held.PID != os.Getpid() {
		t.Errorf("contender saw %v, want a holder with pid %d", err, os.Getpid())
	}
}

func TestInstanceLockUnsupportedFilesystemRuns(t *testing.T) {
	for name, errno := range map[string]error{
		"ENOTSUP": syscall.ENOTSUP, "ENOLCK": syscall.ENOLCK, "EINVAL": syscall.EINVAL,
	} {
		t.Run(name, func(t *testing.T) {
			path := lockSandbox(t)
			old := flock
			flock = func(int, int) error { return errno }
			defer func() { flock = old }()
			l, err := AcquireInstanceLock()
			if err != nil {
				t.Fatalf("err = %v, want the app to run", err)
			}
			if !l.Unsupported {
				t.Error("Unsupported = false, want true so the Log can say so")
			}
			if got, _ := os.ReadFile(path); len(got) != 0 {
				t.Errorf("a lock that was never taken wrote %q", got)
			}
		})
	}
	t.Run("the injected fault the harness uses", func(t *testing.T) {
		lockSandbox(t)
		instanceLockFault = "ENOTSUP"
		defer func() { instanceLockFault = "" }()
		l, err := AcquireInstanceLock()
		if err != nil || !l.Unsupported {
			t.Errorf("= %+v, %v; want Unsupported and no error", l, err)
		}
	})
}

// Anything outside ENOTSUP, ENOLCK and EINVAL refuses and writes no state.
func TestUnexpectedFlockErrorRefusesWithoutState(t *testing.T) {
	for name, errno := range map[string]error{"EBADF": syscall.EBADF, "EIO": syscall.EIO, "EINTR": syscall.EINTR} {
		t.Run(name, func(t *testing.T) {
			path := lockSandbox(t)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("OLD\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			old := flock
			flock = func(int, int) error { return errno }
			defer func() { flock = old }()
			_, err := AcquireInstanceLock()
			le := wantLockError(t, err, LockFlock)
			if !errors.Is(le, errno) {
				t.Errorf("the refusal lost its cause: %v", le)
			}
			if got, _ := os.ReadFile(path); string(got) != "OLD\n" {
				t.Errorf("lock file = %q, want it untouched", got)
			}
			entries, _ := os.ReadDir(filepath.Dir(path))
			if len(entries) != 1 {
				t.Errorf("the session folder holds %d entries, want only the lock file", len(entries))
			}
		})
	}
}

func TestInstanceLockCannotBeOpened(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	path := lockSandbox(t)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	_, err := AcquireInstanceLock()
	le := wantLockError(t, err, LockOpen)
	if !errors.Is(le, os.ErrPermission) {
		t.Errorf("the cause was lost: %v", le)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("a refused launch left %s behind", path)
	}
}
