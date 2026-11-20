package astrolog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// PrivateTempError is a private temp folder that could not be made: Dir is
// the cache folder it was to go in ("" when HOME gives no place for one), and
// Err the system's reason.
type PrivateTempError struct {
	Dir string
	Err error
}

func (e *PrivateTempError) Error() string {
	if e.Dir == "" {
		return "astrolog: no private temp folder: " + e.Err.Error()
	}
	return "astrolog: private temp folder in " + e.Dir + ": " + e.Err.Error()
}

func (e *PrivateTempError) Unwrap() error { return e.Err }

// PrivateTempBase is the folder every temp file and folder of Sextile lives
// in: $XDG_CACHE_HOME/sextile/tmp, else $HOME/.cache/sextile/tmp, mode 0700.
// It is never the system temp folder, so a TMPDIR that points into a charts
// folder cannot put a copy of a chart there. A folder that cannot be made is
// a *PrivateTempError.
func PrivateTempBase() (string, error) {
	base := os.Getenv("XDG_CACHE_HOME")
	if !filepath.IsAbs(base) {
		home := os.Getenv("HOME")
		if !filepath.IsAbs(home) {
			return "", &PrivateTempError{Err: ErrHomeNotAbsolute}
		}
		base = filepath.Join(home, ".cache")
	}
	fail := func(err error) (string, error) { return "", &PrivateTempError{Dir: base, Err: err} }
	if err := os.MkdirAll(base, 0o700); err != nil {
		return fail(err)
	}
	// A base in a folder anyone may write in (the image's /tmp/sextile-cache)
	// could have been made by someone else, or be a link into a charts
	// folder: there it must be a real folder of this user's own.
	if err := ownBaseInSharedFolder(base); err != nil {
		return fail(err)
	}
	// The cache base is the user's, and may be a link. The two folders below
	// it are Sextile's: real folders, never followed through a link, and the
	// last one private.
	dir := base
	for _, part := range []string{"sextile", "tmp"} {
		dir = filepath.Join(dir, part)
		if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
			return fail(err)
		}
		fi, err := os.Lstat(dir)
		if err != nil {
			return fail(err)
		}
		if !fi.IsDir() {
			return fail(fmt.Errorf("%s is a link or not a folder", dir))
		}
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fail(err)
	}
	return dir, nil
}

// ErrHomeNotAbsolute is a HOME that gives no place for a cache folder.
var ErrHomeNotAbsolute = errors.New("HOME is not an absolute path")

// ownBaseInSharedFolder refuses a cache base whose parent anyone may write in,
// unless the base is a real folder (not a link) owned by this user.
func ownBaseInSharedFolder(base string) error {
	parent, err := os.Stat(filepath.Dir(base))
	if err != nil || parent.Mode().Perm()&0o002 == 0 {
		return err
	}
	fi, err := os.Lstat(base)
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() || !ok || int(st.Uid) != os.Getuid() {
		return fmt.Errorf("%s is not a folder of this user's own", base)
	}
	// Its own, but others may write in it: they could swap what lies below.
	if fi.Mode().Perm()&0o022 != 0 {
		return os.Chmod(base, fi.Mode().Perm()&^0o077)
	}
	return nil
}

// privateTempDir is os.MkdirTemp in PrivateTempBase. A folder that cannot be
// made there is a *PrivateTempError too.
func privateTempDir(pattern string) (string, error) {
	base, err := PrivateTempBase()
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(base, pattern)
	if err != nil {
		return "", &PrivateTempError{Dir: base, Err: err}
	}
	return dir, nil
}
