package store

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// chartfiles.go holds the backup folder's helpers and the seams the door's
// tests use. Writing into a charts folder is the door's alone (originals.go).

// ErrChartChanged means the chart file's bytes are no longer the ones the
// retitle started from: something rewrote it while Astrolog was running.
// Nothing is written, because the record in hand was made from the old file.
var ErrChartChanged = errors.New("store: the chart file changed while it was being retitled")

// chartTempSuffix marks a temp file this package created. It starts with a dot,
// so a temp file that a crash leaves behind never lists as a chart.
const chartTempSuffix = ".sextile-tmp"

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
// folder altogether, in BackupDir(dir): one folder per charts folder, holding
// a dated copy for every change (BackupName).
const chartBackupSub = "backup"

// backupParts are the folders below the XDG data base that lead to dir's
// backup folder. Every one of them must be a real folder, never a link.
func backupParts(dir string) []string {
	return []string{"sextile", chartBackupSub, FolderID(dir)}
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
