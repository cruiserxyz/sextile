package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// instance_lock.go keeps a second Sextile from running beside the first
// (the specification). One exclusive flock on SessionDir()/sextile.lock,
// taken at launch and held until the process exits: the operating system drops
// it when the holder dies, however it dies, so there is no stale lock and no
// takeover to design.
//
// The lock file is written by exactly one function, AcquireInstanceLock, and
// only ever holds the holder's process id. It is not a chart file and never
// touches the chart folder, even when XDG_DATA_HOME sits under it.

const (
	instanceLockFile = "sextile.lock"

	// maxLockPID is the largest process id Linux hands out (pid_max, 2^22). A
	// larger number in the lock file is not a pid and is not shown as one.
	maxLockPID = 4194304
)

// instanceLockFault lets the process-level tests break the lock on purpose. It
// is set only with -ldflags "-X sextile/internal/store.instanceLockFault=...",
// by a test building its own binary; no environment variable in the shipped
// binary controls the lock. "ENOTSUP" makes flock answer as an unsupported
// filesystem would; "pidwrite" makes writing the holder pid fail.
var instanceLockFault string

// flock is syscall.Flock; tests swap it to answer with an errno the host's
// filesystem will not.
var flock = syscall.Flock

// LockKind says why the lock could not be had, so the screen can say what to do.
type LockKind int

const (
	// LockUnsafe: the lock path is a symlink, a directory, a FIFO or any file
	// that is not a regular file with one name. Nothing was written to it.
	LockUnsafe LockKind = iota + 1
	// LockOpen: the lock file could not be created or opened.
	LockOpen
	// LockFlock: flock failed for a reason other than "someone holds it" or "this
	// filesystem cannot lock".
	LockFlock
	// LockRecord: the lock was taken but the holder pid could not be written.
	LockRecord
)

// LockHeldError is the answer when another process holds the lock. PID is the
// holder's process id as the lock file records it, or 0 when the file does not
// hold exactly one valid pid.
type LockHeldError struct{ PID int }

func (e *LockHeldError) Error() string {
	if e.PID > 0 {
		return fmt.Sprintf("sextile is already running (process %d)", e.PID)
	}
	return "sextile is already running"
}

// LockError is every other reason the launch must not go on.
type LockError struct {
	Kind LockKind
	Path string
	Err  error
}

func (e *LockError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("lock file %s is not safe to use", e.Path)
	}
	return fmt.Sprintf("lock file %s: %v", e.Path, e.Err)
}

func (e *LockError) Unwrap() error { return e.Err }

// InstanceLock is the lock this process holds. Unsupported means the
// filesystem cannot flock, so nothing is held and a second copy cannot be
// detected; the app runs and says so once.
type InstanceLock struct {
	Unsupported bool
	file        *os.File
}

// heldLock keeps the descriptor reachable for the life of the process: an
// *os.File that is collected is closed, and closing it would drop the lock.
var heldLock *InstanceLock

// AcquireInstanceLock takes the lock, or says why not. The order is the
// safety: the path is opened without following a symlink and without blocking
// (a FIFO would otherwise hang the launch), the opened descriptor is checked
// to be a regular file with one name (a hard link to a chart file would
// otherwise be written), and only then is the lock taken and the pid written.
func AcquireInstanceLock() (*InstanceLock, error) {
	dir := SessionDir()
	path := filepath.Join(dir, instanceLockFile)
	if err := mkdirAllSynced(dir, 0o755); err != nil {
		return nil, &LockError{Kind: LockOpen, Path: path, Err: err}
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
	if err != nil {
		kind := LockOpen
		if errors.Is(err, syscall.ELOOP) || errors.Is(err, syscall.EISDIR) || errors.Is(err, syscall.ENXIO) {
			kind = LockUnsafe
		}
		return nil, &LockError{Kind: kind, Path: path, Err: unwrapPathError(err)}
	}
	taken := false
	defer func() {
		if !taken {
			f.Close() // closing drops any lock this call took
		}
	}()

	st, err := f.Stat()
	if err != nil {
		return nil, &LockError{Kind: LockOpen, Path: path, Err: unwrapPathError(err)}
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); !ok || !st.Mode().IsRegular() || sys.Nlink != 1 {
		return nil, &LockError{Kind: LockUnsafe, Path: path}
	}

	switch err := lockFile(f); {
	case err == nil:
	case errors.Is(err, syscall.EWOULDBLOCK): // EAGAIN: another process holds it
		return nil, &LockHeldError{PID: readHolderPID(f)}
	case errors.Is(err, syscall.ENOTSUP), errors.Is(err, syscall.ENOLCK), errors.Is(err, syscall.EINVAL):
		// The filesystem cannot lock (some network and container mounts). Refusing
		// would make Sextile unusable there; run, and say so.
		return &InstanceLock{Unsupported: true}, nil
	default:
		return nil, &LockError{Kind: LockFlock, Path: path, Err: err}
	}

	if err := writeHolderPID(f); err != nil {
		return nil, &LockError{Kind: LockRecord, Path: path, Err: err}
	}
	taken = true
	l := &InstanceLock{file: f}
	heldLock = l
	return l, nil
}

func lockFile(f *os.File) error {
	if instanceLockFault == "ENOTSUP" {
		return syscall.ENOTSUP
	}
	return flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// writeHolderPID replaces the file's content with this process's pid and a
// newline. Truncating first matters: a shorter pid over a longer old one would
// otherwise leave the old one's tail behind.
func writeHolderPID(f *os.File) error {
	if instanceLockFault == "pidwrite" {
		return syscall.EIO
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	_, err := f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	return err
}

// readHolderPID reads the holder's pid from the same descriptor that failed to
// lock. Anything but exactly one valid decimal process id, optionally followed
// by one newline, is 0.
func readHolderPID(f *os.File) int {
	buf := make([]byte, 32)
	n, _ := f.ReadAt(buf, 0)
	return parseLockPID(buf[:n])
}

func parseLockPID(b []byte) int {
	if n := len(b); n > 0 && b[n-1] == '\n' {
		b = b[:n-1]
	}
	if len(b) == 0 || len(b) > 7 || b[0] == '0' {
		return 0
	}
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0
		}
	}
	pid, err := strconv.Atoi(string(b))
	if err != nil || pid < 1 || pid > maxLockPID {
		return 0
	}
	return pid
}

// unwrapPathError drops the "open /path:" prefix: the error already carries
// the path separately.
func unwrapPathError(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
