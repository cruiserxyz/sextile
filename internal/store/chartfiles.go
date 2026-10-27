package store

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// chartfiles.go holds every function that creates, replaces, renames or
// removes a file in the chart library, apart from PlaceChartFile beside it
// (the specification C3.18; internal/invariants.TestChartFileWritersAreNamed
// holds the list closed).
//
// None of them composes a byte of a chart. Astrolog writes every record whole,
// into a temp file one of these created; these only swap that file in and
// keep the original once under BackupDir(<charts>)/<file>, outside the
// charts folder. They parse
// nothing: whether a record is right is decided in internal/lookup, before any
// of them is called.

// ErrChartChanged means the chart file's bytes are no longer the ones the
// retitle started from: something rewrote it while Astrolog was running.
// Nothing is written, because the record in hand was made from the old file.
var ErrChartChanged = errors.New("store: the chart file changed while it was being retitled")

// chartTempSuffix marks a temp file this package created. It starts with a dot,
// so a temp file that a crash leaves behind never lists as a chart.
const chartTempSuffix = ".sextile-tmp"

// CreateChartTemp creates an empty file, mode 0600, in dir, for Astrolog to
// write a record into, and returns its path. It sits in the SAME directory
// as the chart it will replace, so the replacing rename is atomic.
func CreateChartTemp(dir, stem string) (string, error) {
	f, err := os.CreateTemp(dir, "."+stem+".*"+chartTempSuffix)
	if err != nil {
		return "", fmt.Errorf("store: CreateChartTemp: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("store: CreateChartTemp: %w", err)
	}
	return f.Name(), nil
}

// RemoveChartTemp removes a temp file CreateChartTemp made. It refuses any
// other path, so a caller's slip cannot turn it into a way to delete a chart.
func RemoveChartTemp(path string) error {
	if !strings.HasSuffix(path, chartTempSuffix) {
		return fmt.Errorf("store: RemoveChartTemp: %q is not a chart temp file", path)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("store: RemoveChartTemp: %w", err)
	}
	return nil
}

// Fault-injection seams for the tests of C3.9 and C3.10. Production never
// reassigns them.
var (
	// writeBackup writes the original bytes into the new backup file.
	writeBackup = func(w io.Writer, b []byte) (int, error) { return w.Write(b) }
	// beforeReplaceRename runs between the backup and the replacing rename,
	// where a crash would fall.
	beforeReplaceRename = func() error { return nil }
)

// A chart is known by its first bytes, so a backup (which holds the chart's
// bytes) must never sit where the library lists. It lives outside the charts
// folder altogether, in BackupDir(dir): one folder per charts folder, one
// backup per chart, under the chart's own file name.
const chartBackupSub = "backup"

// backupParts are the folders below the XDG data base that lead to dir's
// backup folder. Every one of them must be a real folder, never a link.
func backupParts(dir string) []string {
	return []string{"sextile", chartBackupSub, FolderID(dir)}
}

// ChartBackupState reports, without writing, whether dir's backup of file
// already exists: absent is false, a regular file is true, and anything else
// is an error, because ReplaceChartWithBackup would refuse it.
func ChartBackupState(dir, file string) (held bool, err error) {
	folder, exists, err := checkBackupFolder(dir)
	if err != nil {
		return false, fmt.Errorf("store: ChartBackupState: %w", err)
	}
	if !exists {
		return false, nil
	}
	bak := filepath.Join(folder, filepath.Base(file))
	fi, err := os.Lstat(bak)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("store: ChartBackupState: %w", err)
	case !fi.Mode().IsRegular():
		return false, fmt.Errorf("store: ChartBackupState: %s is not a file", bak)
	}
	return true, nil
}

// checkBackupFolder refuses a backup folder (or a folder on the way to it)
// that exists and is not a real folder. It runs BEFORE any backup file is
// looked at or trusted: a link could lead anywhere, and a backup found
// through it would be someone else's file. Absent is fine; backupFolder makes
// it when a backup is first kept.
func checkBackupFolder(dir string) (folder string, exists bool, err error) {
	if !validFolderID(FolderID(dir)) {
		return "", false, errNoFolderID
	}
	folder = BackupDir(dir)
	exists, err = checkPrivateDir(dataHome(), backupParts(dir)...)
	if err != nil || !exists {
		return folder, false, err
	}
	return folder, true, nil
}

// errNoFolderID is a charts folder whose path cannot be made absolute, so it
// has no backup folder of its own.
var errNoFolderID = errors.New("the charts folder's path cannot be resolved")

// backupFolder makes sure dir's backup folder exists as a real, private
// folder, and returns it. A link anywhere below the XDG data base is refused:
// it could point a client's chart anywhere.
func backupFolder(dir string) (string, error) {
	if !validFolderID(FolderID(dir)) {
		return "", errNoFolderID
	}
	return privateDir(dataHome(), true, backupParts(dir)...)
}

// ReplaceChartWithBackup swaps tmp in as dir/file, keeping the first original
// in BackupDir(dir)/file.
//
// original is the content the retitle started from. If dir/file no longer
// holds exactly those bytes, nothing happens and ErrChartChanged comes back.
//
// When the backup does not exist, original is written there first (C3.9):
// into a temp name in the backup folder, synced to disk, and only then given
// the backup's name by an exclusive hard link, so a process that dies
// mid-write never leaves a partial backup that a later write would mistake
// for the first original. The
// directory is synced after the link and after the rename; what survives a
// power cut beyond that is the filesystem's guarantee, not this function's. A failed or short write leaves
// no backup and refuses, with the chart as it was. When a backup already
// exists as a regular file it is left alone: the first original is the one
// worth keeping. A backup that is anything else -- a directory, a symlink --
// refuses the write, because nothing then holds the original. backedUp
// reports whether this call made the backup.
//
// The new record is synced before it is renamed over the chart, and the
// rename is of a same-directory file, so a crash leaves either the old chart
// or the whole new one (C3.10). The new chart has mode 0600 whatever the old
// one had.
func ReplaceChartWithBackup(dir, file, tmp string, original []byte) (backedUp bool, err error) {
	path := filepath.Join(dir, filepath.Base(file))
	// A chart that is a symlink is refused: renaming over it would replace
	// the link with a regular file and cut the chart off from its target.
	if fi, err := os.Lstat(path); err != nil {
		return false, fmt.Errorf("store: ReplaceChartWithBackup: %w", err)
	} else if !fi.Mode().IsRegular() {
		return false, fmt.Errorf("store: ReplaceChartWithBackup: %s is not a regular file", filepath.Base(file))
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("store: ReplaceChartWithBackup: %w", err)
	}
	if !bytes.Equal(current, original) {
		return false, fmt.Errorf("%w: %s", ErrChartChanged, filepath.Base(file))
	}

	backups, _, err := checkBackupFolder(dir)
	if err != nil {
		return false, fmt.Errorf("store: ReplaceChartWithBackup: cannot keep the original: %w", err)
	}
	bak := filepath.Join(backups, filepath.Base(file))
	// No `if x, err := …` here: the else branch below installs a deferred
	// undo that must read the function's own err, and an if-scoped err would
	// shadow it.
	existing, statErr := os.Lstat(bak)
	if statErr == nil {
		if !existing.Mode().IsRegular() {
			return false, fmt.Errorf("store: ReplaceChartWithBackup: %s is not a file, so it cannot hold the original", bak)
		}
	} else {
		folder, ferr := backupFolder(dir)
		if ferr != nil {
			return false, fmt.Errorf("store: ReplaceChartWithBackup: cannot keep the original: %w", ferr)
		}
		// serr, not err, for the same reason.
		staged, serr := os.CreateTemp(folder, "."+filepath.Base(file)+".*"+chartTempSuffix)
		if serr != nil {
			return false, fmt.Errorf("store: ReplaceChartWithBackup: cannot keep the original: %w", serr)
		}
		n, werr := writeBackup(staged, original)
		if werr == nil && n != len(original) {
			werr = io.ErrShortWrite
		}
		if werr == nil {
			werr = staged.Sync()
		}
		if cerr := staged.Close(); werr == nil {
			werr = cerr
		}
		var made os.FileInfo
		if werr == nil {
			made, werr = os.Lstat(staged.Name())
		}
		if werr == nil {
			werr = os.Link(staged.Name(), bak)
			if errors.Is(werr, os.ErrExist) {
				werr = fmt.Errorf("%s appeared while the original was being kept", bak)
			}
		}
		os.Remove(staged.Name())
		syncDir(folder)
		if werr != nil {
			return false, fmt.Errorf("store: ReplaceChartWithBackup: cannot keep the original: %w", werr)
		}
		backedUp = true
		// If the swap does not happen after all, the backup this call made is
		// taken back: the chart was not rewritten, so it holds no "first
		// original" yet, and leaving it would let a later write keep this
		// version while the one actually overwritten went unkept. Only the
		// file this call linked is removed, never one put there meanwhile.
		defer func() {
			if err == nil {
				return
			}
			if now, lerr := os.Lstat(bak); lerr == nil && os.SameFile(now, made) {
				os.Remove(bak)
				syncDir(folder)
				backedUp = false
			}
		}()
	}

	if err := os.Chmod(tmp, 0o600); err != nil {
		return backedUp, fmt.Errorf("store: ReplaceChartWithBackup: %w", err)
	}
	if err := syncFile(tmp); err != nil {
		return backedUp, fmt.Errorf("store: ReplaceChartWithBackup: %w", err)
	}
	if err := beforeReplaceRename(); err != nil {
		return backedUp, fmt.Errorf("store: ReplaceChartWithBackup: %w", err)
	}
	// Checked again as late as it can be: the backup was staged and synced
	// since the first check, and an edit saved meanwhile must not be renamed
	// over. (Between this read and the rename there is no portable way to
	// make the replace conditional; the window is microseconds.)
	if now, err := os.ReadFile(path); err != nil || !bytes.Equal(now, original) {
		return backedUp, fmt.Errorf("%w: %s", ErrChartChanged, filepath.Base(file))
	}
	if err := os.Rename(tmp, path); err != nil {
		return backedUp, fmt.Errorf("store: ReplaceChartWithBackup: %w", err)
	}
	syncDir(dir)
	return backedUp, nil
}

// syncDir flushes a directory's entries, so a name just linked or renamed in
// it is on disk. Best effort: not every filesystem supports it. A variable so
// a test can watch when it runs.
var syncDir = func(dir string) {
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
}

// statPath is os.Stat, a variable so a test can make a stat fail.
var statPath = os.Stat

// mkdirAllSynced is os.MkdirAll that also flushes the entry of every directory
// it had to create, in the folder that holds it, so a first save into a folder
// that did not exist survives a power cut as a whole path and not as a file in
// a directory the disk never heard of.
func mkdirAllSynced(dir string, perm os.FileMode) error {
	var created []string
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := statPath(d); !errors.Is(err, fs.ErrNotExist) {
			break // there, or unreadable: MkdirAll reports what is really wrong
		}
		created = append(created, d)
		if filepath.Dir(d) == d {
			break
		}
	}
	if err := os.MkdirAll(dir, perm); err != nil {
		return err
	}
	for _, d := range created {
		syncDir(filepath.Dir(d))
	}
	return nil
}

// syncFile flushes a file Astrolog wrote to disk before it is published
// under a chart's name.
func syncFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	serr := f.Sync()
	if cerr := f.Close(); serr == nil {
		serr = cerr
	}
	return serr
}
