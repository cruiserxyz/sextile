//go:build linux

package store

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// renameat2 is the syscall renameNoReplace makes; a test swaps it to stage a
// filesystem without RENAME_NOREPLACE.
var renameat2 = unix.Renameat2

// renameNoReplace moves from onto to in one step, refusing an existing to
// (os.ErrExist). A kernel or filesystem without RENAME_NOREPLACE falls back to
// a hard link, and one without either to placeExclusive.
func renameNoReplace(from, to string) error {
	err := renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, unix.EINVAL), errors.Is(err, unix.ENOSYS), errors.Is(err, unix.EPERM),
		errors.Is(err, unix.EOPNOTSUPP): // ENOTSUP is EOPNOTSUPP on Linux
		return linkNoReplace(from, to)
	}
	return &os.LinkError{Op: "renameat2", Old: from, New: to, Err: err}
}
