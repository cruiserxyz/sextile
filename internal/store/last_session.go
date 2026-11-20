package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// last_session.go remembers, per charts folder, the named session a window
// last started running, so the next start without --session reopens it.
//
// The record is one small file per charts folder in the state folder,
// $XDG_STATE_HOME/sextile/last-session/<FolderID>, mode 0600 in a 0700
// folder. It holds one line: the session's file name in SessionsDir(). It
// never holds a chart, a name or a place, and nothing is written in the
// charts folder. The last window to record or forget wins: every change
// holds an flock on .lock in that folder, so a window acting on what it read
// earlier never undoes a newer window's record.

const lastSessionDirName = "last-session"

// lastSessionKey is the record's file name for chartsDir; "" when the folder
// has no usable identity, which records nothing.
func lastSessionKey(chartsDir string) string {
	id := FolderID(chartsDir)
	if !validFolderID(id) {
		return ""
	}
	return id
}

// validSessionFile is a file name a record may hold: a plain name in
// SessionsDir() ending in .yaml, never a path or a hidden file.
func validSessionFile(file string) bool {
	return file != "" && file == filepath.Base(file) && !strings.HasPrefix(file, ".") &&
		strings.HasSuffix(file, namedSessionExt) && !strings.ContainsAny(file, "/\\\n\r")
}

// lockRecords holds the record folder's lock until the returned func runs.
// A filesystem that cannot lock runs unlocked: the record is a convenience,
// and a lost race costs one start of the unnamed session.
func lockRecords() (dir string, unlock func(), err error) {
	dir, err = privateDir(stateHome(), true, "sextile", lastSessionDirName)
	if err != nil {
		return "", nil, err
	}
	f, _, err := openLockFile(filepath.Join(dir, namedLockExt))
	if err != nil {
		return "", nil, err
	}
	for syscall.Flock(int(f.Fd()), syscall.LOCK_EX) == syscall.EINTR {
	}
	return dir, func() { f.Close() }, nil
}

// RecordLastSession records file as the session chartsDir's next start reopens.
func RecordLastSession(chartsDir, file string) error {
	key := lastSessionKey(chartsDir)
	if key == "" || !validSessionFile(file) {
		return nil
	}
	dir, unlock, err := lockRecords()
	if err != nil {
		return err
	}
	defer unlock()
	return replaceSessionFile("RecordLastSession", dir, key, file+"\n")
}

// ForgetLastSession removes chartsDir's record: its next start runs the
// unnamed session. No record is no error.
func ForgetLastSession(chartsDir string) error {
	return forgetLastSession(chartsDir, "")
}

// forgetLastSession removes chartsDir's record; when only is set, only while
// the record still names that file.
func forgetLastSession(chartsDir, only string) error {
	key := lastSessionKey(chartsDir)
	if key == "" {
		return nil
	}
	if exists, err := checkPrivateDir(stateHome(), "sextile", lastSessionDirName); err != nil || !exists {
		return err
	}
	dir, unlock, err := lockRecords()
	if err != nil {
		return err
	}
	defer unlock()
	if only != "" {
		if cur, err := readRecord(dir, key); err != nil || cur != only {
			return err
		}
	}
	return removeRecord(dir, key)
}

// removeRecord removes the record key in dir; one that is not there is no error.
func removeRecord(dir, key string) error {
	if err := os.Remove(filepath.Join(dir, key)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	syncDir(dir)
	return nil
}

// readRecord is the file name the record key in dir holds, or "".
func readRecord(dir, key string) (string, error) {
	b, err := readRegular(filepath.Join(dir, key))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	file := strings.TrimSpace(string(b))
	if !validSessionFile(file) {
		return "", nil
	}
	return file, nil
}

// LastSession is the session file chartsDir's record names, or "" when there
// is no record. A record that is not a plain session file name is no record.
func LastSession(chartsDir string) (string, error) {
	key := lastSessionKey(chartsDir)
	if key == "" {
		return "", nil
	}
	if exists, err := checkPrivateDir(stateHome(), "sextile", lastSessionDirName); err != nil || !exists {
		return "", err
	}
	return readRecord(filepath.Join(stateHome(), "sextile", lastSessionDirName), key)
}

// ReopenLastSession opens the session chartsDir's record names, as
// OpenNamedSession opens it. file is "" when there is no record, and then
// nothing is opened and err is nil. On an error no lock is held and name is
// the session's own name where its file still says it: a session that is no
// longer saved is ErrSessionNotFound and its record is removed; any other
// refusal leaves the record as it is.
func ReopenLastSession(chartsDir string) (file, name string, lock *SessionLock, err error) {
	file, err = LastSession(chartsDir)
	if err != nil || file == "" {
		return "", "", nil, err
	}
	name = strings.TrimSuffix(file, namedSessionExt)
	exists, err := checkPrivateDir(dataHome(), "sextile", sessionsDirName)
	if err == nil && !exists {
		err = ErrSessionNotFound
	}
	if err == nil {
		_, name, lock, err = OpenNamedSession(file, FolderID(chartsDir))
	}
	if err == nil {
		return file, name, lock, nil
	}
	if errors.Is(err, ErrSessionNotFound) {
		if ferr := forgetLastSession(chartsDir, file); ferr != nil {
			err = &RecordNotClearedError{Err: ferr}
		}
	} else if b, rerr := readRegular(NamedSessionPath(file)); rerr == nil {
		if n := sessionName(string(b)); n != "" {
			name = n
		}
	}
	if name == "" {
		name = strings.TrimSuffix(file, namedSessionExt)
	}
	return file, name, nil, err
}

// RecordNotClearedError is a session that is no longer saved whose record
// could not be removed: the next start will look for it again.
type RecordNotClearedError struct{ Err error }

func (e *RecordNotClearedError) Error() string {
	return "there is no saved session of that name, and its record could not be removed: " + e.Err.Error()
}

// Is makes a RecordNotClearedError still a missing session.
func (e *RecordNotClearedError) Is(target error) bool { return target == ErrSessionNotFound }

// MoveLastSession keeps chartsDir's record true when the session kept in
// oldFile moves: to newFile on a rename, or away ("") on a trash. A record
// that names another session is left as it is.
func MoveLastSession(chartsDir, oldFile, newFile string) error {
	key := lastSessionKey(chartsDir)
	if key == "" || oldFile == "" {
		return nil
	}
	if exists, err := checkPrivateDir(stateHome(), "sextile", lastSessionDirName); err != nil || !exists {
		return err
	}
	dir, unlock, err := lockRecords()
	if err != nil {
		return err
	}
	defer unlock()
	if cur, err := readRecord(dir, key); err != nil || cur != oldFile {
		return err
	}
	if newFile == "" || !validSessionFile(newFile) {
		return removeRecord(dir, key)
	}
	return replaceSessionFile("MoveLastSession", dir, key, newFile+"\n")
}
