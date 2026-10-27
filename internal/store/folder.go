package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// folder.go names a charts folder, and makes the private folders Sextile keeps
// its own state in.
//
// A charts folder is known by its FolderID: the full sha256, in lowercase hex,
// of the folder's canonical physical path. It keys the folder's backups
// (BackupDir) and is written into the session, so a session saved in one
// charts folder is never restored over another folder's same-named charts.

// folderHash turns a folder's canonical path into its ID. A test swaps it to
// force two folders onto one ID; production never reassigns it.
var folderHash = func(identity string) string {
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])
}

// folderIdentity is the canonical physical path a charts folder is known by.
// Inside the launcher's Docker container (SEXTILE_LAUNCH "docker ..." and the
// folder exactly /charts) it is the launcher's own `pwd -P` of the folder on
// the host, which the app cannot see. Otherwise it
// is the absolute path with every symlink resolved, so one folder reached by
// two names is one folder; a folder that does not exist yet keeps its cleaned
// absolute path.
func folderIdentity(chartsDir string) (string, error) {
	if host, ok := chartsHostPath(chartsDir); ok {
		return host, nil
	}
	abs, err := filepath.Abs(chartsDir)
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	return filepath.Clean(abs), nil
}

// FolderID is the charts folder's identity: the full lowercase hex sha256 of
// its canonical physical path. Empty when the path cannot be made absolute;
// an empty ID matches nothing.
func FolderID(chartsDir string) string {
	id, err := folderIdentity(chartsDir)
	if err != nil {
		return ""
	}
	return folderHash(id)
}

// BackupDir is where chartsDir's backups are kept:
// $XDG_DATA_HOME/sextile/backup/<FolderID>/. Two charts folders never share
// one, and the charts folder itself holds nothing but charts.
func BackupDir(chartsDir string) string {
	return filepath.Join(BackupBase(), FolderID(chartsDir))
}

// ChartBackupDisplay is the backup of file as the operator is told it: the
// absolute path, or, when the launcher ran this copy in Docker and said where
// the backups are on the host, the host's path to the same file.
func ChartBackupDisplay(chartsDir, file string) string {
	return filepath.Join(ChartBackupFolderDisplay(chartsDir), filepath.Base(file))
}

// ChartBackupFolderDisplay is BackupDir(chartsDir) as the operator is told it.
func ChartBackupFolderDisplay(chartsDir string) string {
	if host, ok := backupHostPath(chartsDir); ok {
		return filepath.Join(host, FolderID(chartsDir))
	}
	return BackupDir(chartsDir)
}

// errLinkedFolder is a folder on the way to one of Sextile's own files that is
// a symlink or not a folder at all. Nothing is written through it.
var errLinkedFolder = errors.New("is a link or not a folder, so Sextile will not write through it")

// privateDir makes sure base/parts... exists as real folders and returns it.
// base itself is the operator's (an XDG base may well be a link) and is only
// created when absent; every folder below it must be a real folder, never a
// symlink, or the call refuses before anything is made inside it. Folders it
// creates are 0700 and their entries are flushed. A leaf that already exists
// with group or other access loses that access when narrowLeaf is set; the
// owner's own bits are left as they are.
func privateDir(base string, narrowLeaf bool, parts ...string) (string, error) {
	if err := mkdirAllSynced(base, 0o700); err != nil {
		return "", err
	}
	dir := base
	for i, part := range parts {
		next := filepath.Join(dir, part)
		fi, err := os.Lstat(next)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(next, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				return "", err
			}
			syncDir(dir)
			fi, err = os.Lstat(next)
		}
		if err != nil {
			return "", err
		}
		if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
			return "", fmt.Errorf("%s %w", next, errLinkedFolder)
		}
		if i == len(parts)-1 && narrowLeaf && fi.Mode().Perm()&0o077 != 0 {
			if err := os.Chmod(next, fi.Mode().Perm()&^0o077); err != nil {
				return "", err
			}
		}
		dir = next
	}
	return dir, nil
}

// checkPrivateDir is privateDir without writing: every folder of parts below
// base that exists must be a real folder. It reports whether the leaf exists.
func checkPrivateDir(base string, parts ...string) (exists bool, err error) {
	dir := base
	for _, part := range parts {
		dir = filepath.Join(dir, part)
		fi, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
			return false, fmt.Errorf("%s %w", dir, errLinkedFolder)
		}
	}
	return true, nil
}

// ensureSessionDir makes SessionDir() as a private folder (0700), refusing a
// link anywhere below the XDG state base.
func ensureSessionDir() (string, error) {
	return privateDir(stateHome(), true, "sextile", sessionDirName)
}

// publishExclusive gives data the name dir/name only if nothing has that name
// yet. The bytes go to a temp file in dir first, are synced, and are then
// LINKED to the name, which fails when the name exists: an intervening file is
// never replaced, as a rename would replace it. published is false, with no
// error, when the name was already taken. The file is mode 0600.
func publishExclusive(dir, name string, data []byte) (published bool, err error) {
	tmp, err := os.CreateTemp(dir, "."+name+".*"+chartTempSuffix)
	if err != nil {
		return false, err
	}
	defer os.Remove(tmp.Name())
	_, werr := tmp.Write(data)
	if werr == nil {
		werr = tmp.Chmod(0o600)
	}
	if werr == nil {
		werr = tmp.Sync()
	}
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return false, werr
	}
	if err := os.Link(tmp.Name(), filepath.Join(dir, name)); err != nil {
		if errors.Is(err, os.ErrExist) {
			return false, nil
		}
		return false, err
	}
	syncDir(dir)
	return true, nil
}

// readRegular reads path only if it is a regular file, never through a link.
func readRegular(path string) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return nil, fmt.Errorf("%s is a link, not a file", path)
		}
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	return io.ReadAll(f)
}

// inContainer reports whether this process runs inside a Docker container:
// Docker creates /.dockerenv, and an ordinary user cannot create a file in /.
// Only inside one is /charts the launcher's mount, so only there does the
// launcher's host path name the charts folder. A variable so tests can drive
// both answers; production never reassigns it.
var inContainer = func() bool {
	_, err := os.Lstat("/.dockerenv")
	return err == nil
}

// OverrideContainerProbe makes the container check answer f until the
// returned function restores it. It exists for tests, which must exercise the
// Docker and the native answer on any host.
func OverrideContainerProbe(f func() bool) (restore func()) {
	prev := inContainer
	inContainer = f
	return func() { inContainer = prev }
}
