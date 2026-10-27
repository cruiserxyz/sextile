package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// migrate.go copies, once, what Sextile 0.1.1 kept inside a charts folder to
// the places it is kept now. Before 0.1.2 the launcher put the session, the
// preferences and the retitle backups under <charts>/.sextile; a hand-run
// Sextile kept its session under $XDG_DATA_HOME/sextile/session. Each file is
// COPIED, only where its new place holds nothing, and published so that a
// file put there meanwhile is never replaced. No source is ever modified or
// removed: the old folder is left exactly as it was.

// MigrationResult is what MigrateLegacyState did, for one Log line.
type MigrationResult struct {
	Copied  []string // what was copied, in the operator's words
	Already []string // what was not copied because its new place already held a file
	Refused []string // what was not copied because a source or a destination was unsafe
}

// Empty reports that there was nothing legacy to copy.
func (r MigrationResult) Empty() bool {
	return len(r.Copied) == 0 && len(r.Already) == 0 && len(r.Refused) == 0
}

// Line is the one Log line for r. It is "" unless something was copied or
// refused: the sources are never removed, so every later launch finds them
// already there, and saying so each time would bury the Log.
func (r MigrationResult) Line() string {
	if len(r.Copied) == 0 && len(r.Refused) == 0 {
		return ""
	}
	var parts []string
	if len(r.Copied) > 0 {
		parts = append(parts, "copied "+strings.Join(r.Copied, ", ")+" from the old places")
	}
	if len(r.Already) > 0 {
		parts = append(parts, "not copied, already there: "+strings.Join(r.Already, ", "))
	}
	if len(r.Refused) > 0 {
		parts = append(parts, "not copied, a link or not a plain file or folder: "+strings.Join(r.Refused, ", "))
	}
	return strings.Join(parts, ", ") + ". The old files were left as they are"
}

// afterAbsenceCheck runs after a destination was found empty and before the
// copy is published there: a test pauses two migrations here to prove the
// publish, not the check, decides who wins. Production never reassigns it.
var afterAbsenceCheck = func(target string) {}

// legacyStateDir is the folder 0.1.1's launcher kept state in, inside the
// charts folder.
const legacyStateDir = ".sextile"

// MigrateLegacyState copies chartsDir's 0.1.1 state, and a hand-run
// pre-0.1.2 session, to the current places. It is called once per launch, by
// the window holding the instance lock, after the charts folder is final. It
// keeps no record of its own and is idempotent by its exclusive publish: a
// later run finds every destination taken and copies nothing, except a legacy
// file added since, which it copies.
func MigrateLegacyState(chartsDir string) MigrationResult {
	var r MigrationResult
	id := FolderID(chartsDir)
	if !validFolderID(id) {
		return r
	}
	legacy := filepath.Join(chartsDir, legacyStateDir)

	// The session: the charts folder's first, then the hand-run one. Each is
	// stamped with the folder it belongs to; the second finds the first's
	// copy there and is reported as already there.
	sessionSources := []struct{ label, path string }{
		{"session", filepath.Join(legacy, "data", "sextile", sessionDirName, sessionFile)},
		{"session (hand-run)", legacySessionPath()},
	}
	for i, src := range sessionSources {
		if src.path == SessionPath() || (i > 0 && src.path == sessionSources[0].path) {
			continue // the same file reached twice: nothing more to move
		}
		b, ok := r.readSource(chartsDir, src.label, src.path)
		if !ok {
			continue
		}
		r.copyInto(src.label, func() (string, error) { return ensureSessionDir() }, sessionFile, stampFolder(b, id))
	}

	if b, ok := r.readSource(chartsDir, "preferences", filepath.Join(legacy, "config", "sextile", preferencesFile)); ok {
		r.copyInto("preferences", func() (string, error) { return privateDir(configHome(), false, "sextile") }, preferencesFile, b)
	}

	r.migrateBackups(chartsDir, filepath.Join(legacy, chartBackupSub))
	return r
}

// readSource reads a legacy file: every folder from chartsDir down to it must
// be a real folder and the file a regular one, never followed through a link.
// An absent source is no line at all; an unsafe one is reported as refused.
func (r *MigrationResult) readSource(chartsDir, label, path string) ([]byte, bool) {
	if rel, err := filepath.Rel(chartsDir, filepath.Dir(path)); err == nil && !strings.HasPrefix(rel, "..") {
		if _, err := checkPrivateDir(chartsDir, strings.Split(rel, string(filepath.Separator))...); err != nil {
			r.Refused = append(r.Refused, label)
			return nil, false
		}
	}
	fi, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return nil, false
	}
	if err != nil || !fi.Mode().IsRegular() {
		r.Refused = append(r.Refused, label)
		return nil, false
	}
	b, err := readRegular(path)
	if err != nil {
		r.Refused = append(r.Refused, label)
		return nil, false
	}
	return b, true
}

// copyInto publishes data as name in the folder dest makes, only if nothing
// has that name. A taken name is reported as already there; an unsafe
// destination as refused.
func (r *MigrationResult) copyInto(label string, dest func() (string, error), name string, data []byte) {
	dir, err := dest()
	if err != nil {
		r.Refused = append(r.Refused, label)
		return
	}
	target := filepath.Join(dir, name)
	if fi, err := os.Lstat(target); err == nil {
		if fi.Mode().IsRegular() {
			r.Already = append(r.Already, label)
		} else {
			r.Refused = append(r.Refused, label)
		}
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		r.Refused = append(r.Refused, label)
		return
	}
	afterAbsenceCheck(target)
	published, err := publishExclusive(dir, name, data)
	switch {
	case err != nil:
		r.Refused = append(r.Refused, label)
	case !published:
		r.Already = append(r.Already, label)
	default:
		r.Copied = append(r.Copied, label)
	}
}

// migrateBackups copies every regular file in the legacy backup folder into
// BackupDir(chartsDir), marking that folder as chartsDir's first. Dot files
// (a crashed temp) are not backups and are passed over.
func (r *MigrationResult) migrateBackups(chartsDir, legacy string) {
	if _, err := checkPrivateDir(chartsDir, legacyStateDir, chartBackupSub); err != nil {
		r.Refused = append(r.Refused, "backups")
		return
	}
	entries, err := os.ReadDir(legacy)
	if err != nil {
		return // absent: nothing to copy
	}
	var names []string
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return
	}
	sort.Strings(names)
	var copied, already, refused int
	dest := func() (string, error) { return backupFolder(chartsDir) }
	for _, name := range names {
		var one MigrationResult
		if b, ok := one.readSource(chartsDir, name, filepath.Join(legacy, name)); ok {
			one.copyInto(name, dest, name, b)
		}
		copied += len(one.Copied)
		already += len(one.Already)
		refused += len(one.Refused)
	}
	if copied > 0 {
		r.Copied = append(r.Copied, countBackups(copied))
	}
	if already > 0 {
		r.Already = append(r.Already, countBackups(already))
	}
	if refused > 0 {
		r.Refused = append(r.Refused, countBackups(refused))
	}
}

func countBackups(n int) string {
	if n == 1 {
		return "1 backup"
	}
	return fmt.Sprintf("%d backups", n)
}

// stampFolder writes `folder: <id>` into a legacy session, straight after its
// version line (or first, when it has none), unless it names a folder already.
// The rest of the bytes are carried as they are.
func stampFolder(b []byte, id string) []byte {
	text := string(b)
	if sessionFolder(text) != "" || !validFolderID(id) {
		return b
	}
	line := "folder: " + id + "\n"
	if strings.HasPrefix(text, "version:") {
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			return []byte(text[:i+1] + line + text[i+1:])
		}
		return []byte(text + "\n" + line)
	}
	return []byte(line + text)
}
