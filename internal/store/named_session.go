package store

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// named_session.go keeps the operator's NAMED sessions: a session saved under
// a name, so that a workspace can be put away and opened again later, beside
// the one general session every window starts from.
//
// A named session is the general session's own file -- the same version, the
// same `folder:`, the same charts, floating and frames blocks, written by the
// same formatter -- with one more top-level line, `name:`, the name the
// operator gave it. It lives in SessionsDir() as <file>.yaml, the file name
// made from the name by the chart-file naming rule (Slugify). The folder is
// private (0700), every file in it is 0600, and a link anywhere below the XDG
// data base is refused rather than followed.
//
// A new name is PUBLISHED exclusively (temp, fsync, link): a file that has the
// name already is never replaced. Only the window that runs a session rewrites
// it, and only while it holds that session's lock, sessions/<stem>.lock -- an
// flock like the instance lock. A trashed session moves to sessions/.trash
// under a UTC stamp, byte for byte, and is never deleted.

const (
	namedSessionExt = ".yaml"
	namedLockExt    = ".lock"
	sessionTrashDir = ".trash"
)

var (
	// ErrSessionNameEmpty is a name with no letters or digits to make a file from.
	ErrSessionNameEmpty = errors.New("that name has no letters or digits")
	// ErrSessionNameTaken is a name whose file already exists.
	ErrSessionNameTaken = errors.New("that name is taken")
	// ErrSessionOpenElsewhere is a session another Sextile window runs.
	ErrSessionOpenElsewhere = errors.New("it is open in another Sextile window")
	// ErrSessionRunning is a session this window runs, which cannot be trashed.
	ErrSessionRunning = errors.New("this window runs it. Switch to another session first")
	// ErrSessionOtherFolder is a session saved in another charts folder.
	ErrSessionOtherFolder = errors.New("it belongs to another charts folder")
	// ErrSessionNotFound is a name with no saved session.
	ErrSessionNotFound = errors.New("there is no saved session of that name")
	// ErrSessionLockUnsupported is a sessions folder on a filesystem that
	// cannot lock files: two windows could write one session there, so named
	// sessions are not kept on it.
	ErrSessionLockUnsupported = errors.New("this folder cannot lock files, so named sessions cannot be kept here safely")
	// ErrSessionChangedOutside is a running session whose file was changed
	// since this window last read or wrote it: it is not written over.
	ErrSessionChangedOutside = errors.New("its file changed outside Sextile and was left as it is")
	// ErrSessionUnknownContent is a session holding a block or key this build
	// does not know; saving it would drop that, so it is left as it is.
	ErrSessionUnknownContent = errors.New("it holds something this version of Sextile does not know, so it is left as it is")
	// ErrSessionUnreadable is a session file this build will not run: a later
	// generation, or entries it cannot read. It is left exactly as it is.
	ErrSessionUnreadable = errors.New("its file cannot be used")
)

// NamedSessionFile is the file a session of this name is kept in: the name by
// the chart-file naming rule, plus .yaml; "" when the name has nothing to
// make a file name from.
func NamedSessionFile(name string) string {
	slug := slugify(name)
	if slug == "" {
		return ""
	}
	return slug + namedSessionExt
}

// NamedSession is one saved session as the Sessions list shows it.
type NamedSession struct {
	Name     string    // the name the operator gave it (`name:`)
	File     string    // its file's base name in SessionsDir()
	Folder   string    // the FolderID it was saved in
	Modified time.Time // when it was last written
	// OpenElsewhere is set when another window holds its lock: it cannot be
	// opened, renamed or trashed from here.
	OpenElsewhere bool
}

// SessionLock is a held named-session lock. A window holds at most one, for
// the session it runs; Release drops it.
type SessionLock struct {
	file string
	f    *os.File
	// sum is the sha256 of the bytes this window last read from or wrote to
	// the file. A save finds them there, or writes nothing.
	sum [sha256.Size]byte
}

// File is the base name of the session file this lock is for.
func (l *SessionLock) File() string {
	if l == nil {
		return ""
	}
	return l.file
}

// Release drops the lock. It is safe on nil and twice.
func (l *SessionLock) Release() {
	if l == nil || l.f == nil {
		return
	}
	l.f.Close()
	l.f = nil
}

func (l *SessionLock) holds(file string) bool {
	return l != nil && l.f != nil && l.file == file
}

// ensureSessionsDir makes SessionsDir() as a private folder (0700), refusing a
// link anywhere below the XDG data base.
func ensureSessionsDir() (string, error) {
	return privateDir(dataHome(), true, "sextile", sessionsDirName)
}

// lockFileFor is the lock file of the session kept in file.
func lockFileFor(file string) string {
	return strings.TrimSuffix(file, namedSessionExt) + namedLockExt
}

// lockNamed takes the lock of the session kept in dir/file. Another holder is
// ErrSessionOpenElsewhere. A filesystem that cannot lock is
// ErrSessionLockUnsupported: unlike the general session, which runs unlocked
// there and says so, a named session is never handed to a window that cannot
// prove it is the only writer.
func lockNamed(dir, file string) (*SessionLock, error) {
	path := filepath.Join(dir, lockFileFor(file))
	f, unsupported, created, err := takeLockCreated(path)
	if unsupported && created {
		// Nothing can be held there, so the empty lock file this call made
		// would only claim a session nobody runs.
		_ = os.Remove(path)
	}
	var held *LockHeldError
	switch {
	case errors.As(err, &held):
		return nil, ErrSessionOpenElsewhere
	case err != nil:
		return nil, err
	case unsupported:
		return nil, ErrSessionLockUnsupported
	}
	return &SessionLock{file: file, f: f}, nil
}

// formatNamedSession is the general session's text with `name:` straight
// after its version line.
func formatNamedSession(name string, s Session) string {
	return withSessionName(formatSession(s), name)
}

// withSessionName sets the top-level `name:` line of a session's text: the
// first one is replaced, or one is put straight after the version line. Every
// other byte is carried as it is.
func withSessionName(text, name string) string {
	line := "name: " + yamlEscape(name) + "\n"
	lines := splitAfterNewlines(text)
	for i, l := range lines {
		if strings.HasPrefix(l, "name:") {
			lines[i] = line
			return strings.Join(lines, "")
		}
	}
	if len(lines) > 0 && strings.HasPrefix(lines[0], "version:") {
		first := lines[0]
		if !strings.HasSuffix(first, "\n") {
			first += "\n"
		}
		return first + line + strings.Join(lines[1:], "")
	}
	return line + text
}

// sessionName is the value of the first column-zero `name:` line.
func sessionName(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "name:"); ok {
			return unquoteScalar(strings.TrimSpace(v))
		}
	}
	return ""
}

// ListNamedSessions is every named session saved in the charts folder
// folderID, newest first. The session this window runs (running, its file, or
// "") is listed without asking its lock, which this window holds; every other
// one is marked OpenElsewhere when another window holds its lock. An absent
// folder lists nothing; a linked one is an error.
func ListNamedSessions(folderID, running string) ([]NamedSession, error) {
	if exists, err := checkPrivateDir(dataHome(), "sextile", sessionsDirName); err != nil || !exists {
		return nil, err
	}
	dir := SessionsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []NamedSession
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, namedSessionExt) {
			continue
		}
		path := filepath.Join(dir, name)
		fi, err := os.Lstat(path)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		b, err := readRegular(path)
		if err != nil {
			continue
		}
		text := string(b)
		if sessionFolder(text) != folderID || !validFolderID(folderID) {
			continue
		}
		ns := NamedSession{Name: sessionName(text), File: name, Folder: folderID, Modified: fi.ModTime()}
		if ns.Name == "" {
			ns.Name = strings.TrimSuffix(name, namedSessionExt)
		}
		if name != running {
			if l, err := lockNamed(dir, name); errors.Is(err, ErrSessionOpenElsewhere) {
				ns.OpenElsewhere = true
			} else if err == nil {
				l.Release()
			}
		}
		out = append(out, ns)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Modified.Equal(out[j].Modified) {
			return out[i].Modified.After(out[j].Modified)
		}
		return out[i].File < out[j].File
	})
	return out, nil
}

// SaveNamedSessionAs saves s as a new named session and takes its lock. The
// name's file must not exist: a taken name is ErrSessionNameTaken and nothing
// is written. The file is published exclusively and is 0600.
func SaveNamedSessionAs(name string, s Session) (*SessionLock, error) {
	file := NamedSessionFile(name)
	if file == "" {
		return nil, ErrSessionNameEmpty
	}
	dir, err := ensureSessionsDir()
	if err != nil {
		return nil, err
	}
	// A taken name is refused before anything, its lock file included, is
	// made; the exclusive publish below still decides a race.
	if _, err := os.Lstat(filepath.Join(dir, file)); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return nil, err
		}
		return nil, ErrSessionNameTaken
	}
	lock, err := lockNamed(dir, file)
	if errors.Is(err, ErrSessionOpenElsewhere) {
		return nil, ErrSessionNameTaken
	}
	if err != nil {
		return nil, err
	}
	text := formatNamedSession(name, s)
	published, err := publishExclusive(dir, file, []byte(text))
	if err != nil || !published {
		lock.Release()
		if err != nil {
			return nil, err
		}
		return nil, ErrSessionNameTaken
	}
	lock.sum = sha256.Sum256([]byte(text))
	return lock, nil
}

// SaveNamedSession writes s over the named session this window runs, the way
// SaveSession writes the general one. It refuses without that session's lock.
// The file must still hold exactly what this window last read or wrote: a file
// changed outside Sextile since is ErrSessionChangedOutside, and nothing is
// written over it.
func SaveNamedSession(lock *SessionLock, name string, s Session) error {
	if lock == nil || lock.file == "" || lock.f == nil {
		return errors.New("store: SaveNamedSession: no session is held")
	}
	dir, err := ensureSessionsDir()
	if err != nil {
		return fmt.Errorf("store: SaveNamedSession: mkdir: %w", err)
	}
	now, err := readRegular(filepath.Join(dir, lock.file))
	if err != nil || sha256.Sum256(now) != lock.sum {
		return ErrSessionChangedOutside
	}
	text := formatNamedSession(name, s)
	if err := replaceSessionFile("SaveNamedSession", dir, lock.file, text); err != nil {
		return err
	}
	lock.sum = sha256.Sum256([]byte(text))
	return nil
}

// NamedSessionPath is the full path of a named session's file.
func NamedSessionPath(file string) string {
	return filepath.Join(SessionsDir(), file)
}

// NamedSessionDisplay is a path in SessionsDir() as the operator is told it:
// absolute, or, when the launcher ran this copy in Docker, the host's path.
func NamedSessionDisplay(path string) string {
	rel, err := filepath.Rel(SessionsDir(), path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	if host, ok := sessionsHostPath(ChartsDir()); ok {
		return filepath.Join(host, rel)
	}
	return path
}

// LoadNamedSession reads a named session: what it holds, and its name. A file
// this build would not restore whole is ErrSessionUnreadable and is left as
// it is, so running it can never write a shorter session over it.
func LoadNamedSession(file string) (Session, string, error) {
	s, name, _, err := loadNamedSession(file)
	return s, name, err
}

// loadNamedSession is LoadNamedSession, also returning the bytes it judged.
func loadNamedSession(file string) (Session, string, []byte, error) {
	if exists, err := checkPrivateDir(dataHome(), "sextile", sessionsDirName); err != nil {
		return Session{}, "", nil, err
	} else if !exists {
		return Session{}, "", nil, ErrSessionNotFound
	}
	path := NamedSessionPath(file)
	b, err := readRegular(path)
	if errors.Is(err, os.ErrNotExist) {
		return Session{}, "", nil, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, "", nil, err
	}
	s, ok, err := parseSessionFile(path, b)
	if err != nil || !ok || s.DroppedFrames > 0 {
		return Session{}, "", nil, ErrSessionUnreadable
	}
	if formatNamedSession(sessionName(string(b)), s) != string(b) {
		return Session{}, "", nil, ErrSessionUnknownContent
	}
	for _, sl := range s.Slots {
		if !sl.Usable {
			return Session{}, "", nil, ErrSessionUnreadable
		}
	}
	name := sessionName(string(b))
	if name == "" {
		name = strings.TrimSuffix(file, namedSessionExt)
	}
	return s, name, b, nil
}

// A named session is opened only when it is exactly what the writer would
// write for what was read: the allow-list is the writer itself. Any other line
// -- a comment, an unknown block or key, at any indent -- would be lost at the
// next save, so the session is refused and left as it is.

// OpenNamedSession takes the lock of the session kept in file and reads it,
// for the charts folder folderID. A session another window runs is
// ErrSessionOpenElsewhere; one saved in another folder ErrSessionOtherFolder.
// On any error no lock is held.
func OpenNamedSession(file, folderID string) (Session, string, *SessionLock, error) {
	dir, err := ensureSessionsDir()
	if err != nil {
		return Session{}, "", nil, err
	}
	if _, err := os.Lstat(filepath.Join(dir, file)); errors.Is(err, os.ErrNotExist) {
		return Session{}, "", nil, ErrSessionNotFound
	}
	lock, err := lockNamed(dir, file)
	if err != nil {
		return Session{}, "", nil, err
	}
	s, name, b, err := loadNamedSession(file)
	if err == nil && (s.Folder == "" || s.Folder != folderID) {
		err = ErrSessionOtherFolder
	}
	if err != nil {
		lock.Release()
		return Session{}, "", nil, err
	}
	lock.sum = sha256.Sum256(b)
	return s, name, lock, nil
}

// OpenNamedSessionByName is OpenNamedSession for a session named on the
// command line: its file is found from the name by the naming rule.
func OpenNamedSessionByName(name, folderID string) (Session, string, *SessionLock, error) {
	file := NamedSessionFile(name)
	if file == "" {
		return Session{}, "", nil, ErrSessionNotFound
	}
	if exists, err := checkPrivateDir(dataHome(), "sextile", sessionsDirName); err != nil {
		return Session{}, "", nil, err
	} else if !exists {
		return Session{}, "", nil, ErrSessionNotFound
	}
	return OpenNamedSession(file, folderID)
}

// RenameNamedSession gives the session kept in oldFile the name newName: its
// `name:` line is rewritten and, when the name makes another file name, it
// moves there exclusively -- the new file is published by link, which a taken
// name refuses, and only then is the old one unlinked. The file keeps its
// last-updated time. running is the lock this window holds (nil for none):
// renaming the session this window runs moves its lock with it, and the new
// lock is returned; renaming one another window runs is ErrSessionOpenElsewhere.
func RenameNamedSession(oldFile, newName string, running *SessionLock) (string, *SessionLock, error) {
	newFile := NamedSessionFile(newName)
	if newFile == "" {
		return "", nil, ErrSessionNameEmpty
	}
	dir, err := ensureSessionsDir()
	if err != nil {
		return "", nil, err
	}
	mine := running.holds(oldFile)
	if !mine {
		old, err := lockNamed(dir, oldFile)
		if err != nil {
			return "", nil, err
		}
		defer old.Release()
	}
	// A session is renamed only if it could be opened: one this build cannot
	// read whole is left exactly as it is, its name included.
	if _, _, b, err := loadNamedSession(oldFile); err != nil {
		return "", nil, err
	} else if mine && sha256.Sum256(b) != running.sum {
		return "", nil, ErrSessionChangedOutside
	}
	oldPath := filepath.Join(dir, oldFile)
	fi, err := os.Lstat(oldPath)
	if err != nil {
		return "", nil, err
	}
	b, err := readRegular(oldPath)
	if err != nil {
		return "", nil, err
	}
	text := withSessionName(string(b), newName)
	if newFile == oldFile {
		if err := replaceSessionFile("RenameNamedSession", dir, oldFile, text); err != nil {
			return "", nil, err
		}
		_ = os.Chtimes(oldPath, time.Now(), fi.ModTime())
		if mine {
			running.sum = sha256.Sum256([]byte(text))
			return oldFile, running, nil
		}
		return oldFile, nil, nil
	}
	lock, err := lockNamed(dir, newFile)
	if errors.Is(err, ErrSessionOpenElsewhere) {
		return "", nil, ErrSessionNameTaken
	}
	if err != nil {
		return "", nil, err
	}
	published, err := publishExclusive(dir, newFile, []byte(text))
	if err != nil || !published {
		lock.Release()
		if err != nil {
			return "", nil, err
		}
		return "", nil, ErrSessionNameTaken
	}
	_ = os.Chtimes(filepath.Join(dir, newFile), time.Now(), fi.ModTime())
	if err := os.Remove(oldPath); err != nil {
		lock.Release()
		return "", nil, err
	}
	syncDir(dir)
	if mine {
		running.Release()
		lock.sum = sha256.Sum256([]byte(text))
		return newFile, lock, nil
	}
	lock.Release()
	return newFile, nil, nil
}

// trashStamp is the UTC form in a trashed session's name.
const trashStamp = "20060102T150405Z"

// TrashNamedSession moves the session kept in file to sessions/.trash as
// <stem>-<UTC stamp>.yaml, byte for byte, and returns its new path. The move
// is a link to the new name, which never replaces anything, then an unlink of
// the old one. The session this window runs (running) is ErrSessionRunning;
// one another window runs is ErrSessionOpenElsewhere.
func TrashNamedSession(file string, running *SessionLock, now time.Time) (string, error) {
	if running != nil && running.file == file {
		return "", ErrSessionRunning
	}
	dir, err := ensureSessionsDir()
	if err != nil {
		return "", err
	}
	lock, err := lockNamed(dir, file)
	if err != nil {
		return "", err
	}
	defer lock.Release()
	src := filepath.Join(dir, file)
	fi, err := os.Lstat(src)
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", src)
	}
	trash, err := privateDir(dataHome(), true, "sextile", sessionsDirName, sessionTrashDir)
	if err != nil {
		return "", err
	}
	dst := filepath.Join(trash, strings.TrimSuffix(file, namedSessionExt)+"-"+now.UTC().Format(trashStamp)+namedSessionExt)
	if err := os.Link(src, dst); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("%s already exists. Try again in a second", dst)
		}
		return "", err
	}
	syncDir(trash)
	if err := os.Remove(src); err != nil && !errors.Is(err, syscall.ENOENT) {
		return "", err
	}
	syncDir(dir)
	return dst, nil
}
