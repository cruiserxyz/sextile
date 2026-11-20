package store

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// originals.go is the door: the one place that writes, creates, renames, links
// or removes anything inside a charts folder. Reading is free elsewhere. Every
// write here takes bytes, never a path for someone else to write to, and
// refuses at the protected position before it touches the disk.
//
// A charts folder has one position, kept by folder identity in the position
// file (PositionFile): protected writes nothing, ask asks before every write,
// add saves new charts freely and asks before a change, allow asks nothing.
// The asking is the screen's; the door enforces protected, keeps a dated copy
// of every file it replaces, and appends a journal line before every write.

// ChartFileExt is the ending of the file name Sextile gives a chart it writes:
// `.as`, Astrolog's own convention for a file in its syntax. It is the ending
// of what is WRITTEN, never a test of what is read: a chart is known by its
// first bytes, whatever it is called.
const ChartFileExt = ".as"

// ErrChartExists means a chart file of that name is already in the library.
// It is a separate cause because it is not a fault: the name is taken, and the
// save prompt stays open for another.
var ErrChartExists = errors.New("store: a chart of that name is already saved")

// Position is how far Sextile may write into one charts folder.
type Position string

const (
	PositionProtected Position = "protected"
	PositionAsk       Position = "ask"
	PositionAdd       Position = "add"
	PositionAllow     Position = "allow"
)

func (p Position) valid() bool {
	switch p {
	case PositionProtected, PositionAsk, PositionAdd, PositionAllow:
		return true
	}
	return false
}

var (
	// ErrOriginalsProtected: the folder is at protected (stored, by a fault in
	// the position file, or by SEXTILE_CHARTS_READONLY=1). Nothing was written.
	ErrOriginalsProtected = errors.New("This charts folder is protected. Nothing was written.")
	// ErrJournal: the journal line could not be appended before a write.
	ErrJournal = errors.New("The journal could not be written. Nothing was written.")
	// ErrJournalUncorrected: a write failed after its journal line, and the
	// line saying so could not be appended.
	ErrJournalUncorrected = errors.New("store: the journal lists a write that did not happen")
	// ErrBackupExists: the dated copy's name is taken.
	ErrBackupExists = errors.New("store: the dated copy's name is taken")
	// ErrNewFolderPosition: Sextile created the charts folder and could not
	// record it as its own, so it removed it again.
	ErrNewFolderPosition = errors.New("The position of the new folder could not be recorded. Nothing was written.")
	// ErrFolderAppeared: the charts folder was created by another program
	// while Sextile was creating it.
	ErrFolderAppeared = errors.New("The charts folder appeared while Sextile was creating it. Nothing was written.")
	// ErrFolderBusy: another Sextile window holds this charts folder's writing
	// lock. Reading stays free.
	ErrFolderBusy = errors.New("Another Sextile window writes this charts folder. Nothing was written.")
	// errStagingSwapped: the door's staging file was replaced before its rename.
	errStagingSwapped = errors.New("store: the staging file was replaced by another entry")
)

// readonlyEnv set to 1 makes every folder protected for this run, whatever is
// stored. The launcher sets it for a read-only Docker mount.
const readonlyEnv = "SEXTILE_CHARTS_READONLY"

// positionFileName is the position file's base name, as the Log names it.
const positionFileName = "launcher-originals.conf"

// PositionFile is $XDG_CONFIG_HOME/sextile/launcher-originals.conf. One line
// per charts folder, `<folder identity>=<position>`, split at the last `=`.
// Lines starting with `#` and blank lines are kept and ignored.
func PositionFile() string {
	return filepath.Join(configHome(), "sextile", positionFileName)
}

// PositionFileName is the position file's base name.
func PositionFileName() string { return positionFileName }

// PositionRead is what ReadPosition found for one charts folder.
type PositionRead struct {
	Stored      Position // the word on the folder's line; "" when it has none
	HasLine     bool
	Effective   Position // what the door acts on
	Fault       string   // why the folder is protected by a faulty file; "" when none
	ReadonlyEnv bool     // SEXTILE_CHARTS_READONLY=1
}

// ReadPosition reads the position file for chartsDir, every call. A missing
// file or line is ask. A file that cannot be read, is not a regular file,
// holds more than one line or an unknown word for the folder, or a folder
// whose identity cannot be written on one line, is protected with a Fault.
func ReadPosition(chartsDir string) PositionRead {
	r := PositionRead{ReadonlyEnv: os.Getenv(readonlyEnv) == "1"}
	r.Stored, r.HasLine, r.Fault = storedPosition(chartsDir)
	switch {
	case r.Fault != "":
		r.Effective = PositionProtected
	case !r.HasLine:
		r.Effective = PositionAsk
	default:
		r.Effective = r.Stored
	}
	if r.ReadonlyEnv {
		r.Effective = PositionProtected
	}
	return r
}

// The faults ReadPosition can name.
const (
	faultUnreadable = "cannot be read"
	faultNotAFile   = "is not a file"
	faultNoIdentity = "cannot name this folder"
)

func storedPosition(chartsDir string) (Position, bool, string) {
	id, ok := positionIdentity(chartsDir)
	if !ok {
		return "", false, faultNoIdentity
	}
	b, fault := readPositionFile()
	if fault != "" {
		return "", false, fault
	}
	words := lineWords(b, id)
	switch {
	case len(words) == 0:
		return "", false, ""
	case len(words) > 1:
		return "", false, fmt.Sprintf("has %d lines for it", len(words))
	}
	p := Position(words[0])
	if !p.valid() {
		return "", false, fmt.Sprintf("says %q for it", words[0])
	}
	return p, true, ""
}

// positionIdentity is the folder's identity as a line of the file holds it.
func positionIdentity(chartsDir string) (string, bool) {
	id, err := folderIdentity(chartsDir)
	if err != nil || id == "" || strings.ContainsAny(id, "\r\n") {
		return "", false
	}
	return id, true
}

// readPositionFile is the position file's bytes; nil when it does not exist.
// The file is opened without following a link and read through that handle,
// so what is checked is what is read.
func readPositionFile() ([]byte, string) {
	f, err := os.OpenFile(PositionFile(), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	switch {
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
		return nil, ""
	case errors.Is(err, syscall.ELOOP):
		return nil, faultNotAFile
	case err != nil:
		return nil, faultUnreadable
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, faultUnreadable
	}
	if !fi.Mode().IsRegular() {
		return nil, faultNotAFile
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, faultUnreadable
	}
	return b, ""
}

// positionLine is one line of the position file: its bytes with their line
// end, and the identity and word when it is a position line.
type positionLine struct {
	text     string
	id, word string
	isLine   bool
}

func splitPositionFile(b []byte) []positionLine {
	var out []positionLine
	for _, t := range strings.SplitAfter(string(b), "\n") {
		if t == "" {
			continue
		}
		l := positionLine{text: t}
		body := strings.TrimRight(t, "\r\n")
		if s := strings.TrimSpace(body); s != "" && !strings.HasPrefix(s, "#") {
			if i := strings.LastIndex(body, "="); i >= 0 {
				l.id, l.word, l.isLine = body[:i], strings.TrimSpace(body[i+1:]), true
			}
		}
		out = append(out, l)
	}
	return out
}

func lineWords(b []byte, id string) []string {
	var words []string
	for _, l := range splitPositionFile(b) {
		if l.isLine && l.id == id {
			words = append(words, l.word)
		}
	}
	return words
}

// beforePositionRename runs between the position file's temp write and its
// rename: a test changes the file there.
var beforePositionRename = func() {}

// SetPosition writes chartsDir's line as p and keeps every other byte of the
// file. The write is a synced temp file renamed over the file; a file that is
// a symlink is refused. If the file changed while the new one was written, it
// starts again from the new bytes, three times at most.
func SetPosition(chartsDir string, p Position) error {
	return setPosition(chartsDir, p, false)
}

// SetPositionReplacing is SetPosition for the launcher's word: every earlier
// line for the folder, a faulty one included, gives way to the one written.
func SetPositionReplacing(chartsDir string, p Position) error {
	return setPosition(chartsDir, p, true)
}

func setPosition(chartsDir string, p Position, replaceAll bool) error {
	if !p.valid() {
		return fmt.Errorf("store: SetPosition: %q is not a position", p)
	}
	id, ok := positionIdentity(chartsDir)
	if !ok {
		return fmt.Errorf("store: SetPosition: %s %s", positionFileName, faultNoIdentity)
	}
	if err := ClaimFolder(chartsDir); err != nil {
		return err
	}
	if _, err := privateDir(configHome(), false, "sextile"); err != nil {
		return fmt.Errorf("store: SetPosition: %w", err)
	}
	path := PositionFile()
	// Each write of the file runs under a short flock beside it; a filesystem
	// that cannot lock falls back to the compare below alone.
	lock, unsupported, err := takePositionLock(path + ".lock")
	if err != nil {
		return fmt.Errorf("store: SetPosition: %s is being written: %w", positionFileName, err)
	}
	if !unsupported {
		defer lock.Close()
	}
	for try := 0; try < 3; try++ {
		read, err := positionFileBytes(path)
		if err != nil {
			return err
		}
		next, err := withLine(read, id, string(p), replaceAll)
		if err != nil {
			return err
		}
		tmp, err := writePositionTemp(filepath.Dir(path), next)
		if err != nil {
			return err
		}
		beforePositionRename()
		now, err := positionFileBytes(path)
		if err != nil {
			os.Remove(tmp)
			return err
		}
		if !bytes.Equal(now, read) {
			os.Remove(tmp)
			continue
		}
		if err := os.Rename(tmp, path); err != nil {
			os.Remove(tmp)
			return fmt.Errorf("store: SetPosition: %w", err)
		}
		syncDir(filepath.Dir(path))
		return nil
	}
	return fmt.Errorf("store: SetPosition: %s kept changing while it was written", positionFileName)
}

// takePositionLock takes the position file's lock, waiting up to a second for
// another window's write, which takes microseconds.
func takePositionLock(path string) (*os.File, bool, error) {
	var held *LockHeldError
	for i := 0; ; i++ {
		f, unsupported, err := takeLock(path)
		if !errors.As(err, &held) || i == 50 {
			return f, unsupported, err
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// positionFileBytes reads the position file for a write: nil when absent, an
// error when it is a link or not a regular file.
func positionFileBytes(path string) ([]byte, error) {
	b, err := readRegular(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: SetPosition: %s: %w", positionFileName, err)
	}
	return b, nil
}

// withLine is b with id's one line set to word, or a line appended.
func withLine(b []byte, id, word string, replaceAll bool) ([]byte, error) {
	lines := splitPositionFile(b)
	if n := len(lineWords(b, id)); n > 1 && !replaceAll {
		return nil, fmt.Errorf("store: SetPosition: %s has %d lines for this folder", positionFileName, n)
	}
	var out strings.Builder
	done := false
	for _, l := range lines {
		if l.isLine && l.id == id {
			if done {
				continue // a later line for the folder gives way to the one written
			}
			end := l.text[len(strings.TrimRight(l.text, "\r\n")):]
			out.WriteString(id + "=" + word + end)
			done = true
			continue
		}
		out.WriteString(l.text)
	}
	if !done {
		s := out.String()
		line := id + "=" + word + "\n"
		if i := strings.LastIndex(s, "\n"); s != "" && !strings.HasSuffix(s, "\n") {
			// The last line has no line end: the new line goes before it, so
			// every other line keeps its bytes.
			return []byte(s[:i+1] + line + s[i+1:]), nil
		}
		out.WriteString(line)
	}
	return []byte(out.String()), nil
}

func writePositionTemp(dir string, b []byte) (string, error) {
	f, err := os.CreateTemp(dir, "."+positionFileName+".*.tmp")
	if err != nil {
		return "", fmt.Errorf("store: SetPosition: %w", err)
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Chmod(0o600)
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("store: SetPosition: %w", err)
	}
	return f.Name(), nil
}

// Journal acts.
const (
	actAdd    = "add"
	actChange = "change"
	actFailed = "failed"
)

// JournalPath is $XDG_DATA_HOME/sextile/backup/<FolderID>.journal.
func JournalPath(chartsDir string) string {
	return filepath.Join(BackupBase(), FolderID(chartsDir)+".journal")
}

// appendJournal appends one synced line: the UTC time, the act, the quoted
// base name, the SHA-256 before and after, and the backup's base name.
var appendJournal = func(chartsDir, act, file, before, after, backup string) error {
	if !validFolderID(FolderID(chartsDir)) {
		return errNoFolderID
	}
	if _, err := privateDir(dataHome(), false, "sextile", chartBackupSub); err != nil {
		return err
	}
	f, err := os.OpenFile(JournalPath(chartsDir), os.O_WRONLY|os.O_APPEND|os.O_CREATE|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("the journal is not a regular file")
	}
	if fi.Mode().Perm() != 0o600 {
		if err := f.Chmod(0o600); err != nil {
			return err
		}
	}
	line := strings.Join([]string{time.Now().UTC().Format(time.RFC3339), act, strconv.Quote(file), before, after, backup}, "\t") + "\n"
	if _, err := f.WriteString(line); err != nil {
		return err
	}
	return f.Sync()
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// BackupName is the dated copy's base name: the file's stem (one final `.as`
// removed) and the UTC stamp YYYYMMDDTHHMMSSZ, ending in `.as`.
func BackupName(file string, stamp time.Time) string {
	stem := strings.TrimSuffix(filepath.Base(file), ChartFileExt)
	return stem + "." + stamp.UTC().Format("20060102T150405Z") + ChartFileExt
}

// LeftoverTemps lists, without opening them, the entries of chartsDir whose
// name ends in `.sextile-tmp`.
func LeftoverTemps(chartsDir string) ([]string, error) {
	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), chartTempSuffix) {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// folderLocks are the charts folders this process writes: the first window to
// write a folder holds its lock until it exits.
var folderLocks = struct {
	sync.Mutex
	held        map[string]*os.File
	unsupported map[string]bool
	noted       map[string]bool
}{held: map[string]*os.File{}, unsupported: map[string]bool{}, noted: map[string]bool{}}

// ClaimFolder takes this process's writing lock on chartsDir, or answers
// ErrFolderBusy when another process holds it. The lock is a flock on
// BackupDir(chartsDir)+".writer.lock", beside the journal, held until the
// process exits. Where the filesystem cannot lock, nothing is held and the
// writes go on (FolderLockNote says so once).
func ClaimFolder(chartsDir string) error {
	id := FolderID(chartsDir)
	if !validFolderID(id) {
		return errNoFolderID
	}
	folderLocks.Lock()
	defer folderLocks.Unlock()
	if folderLocks.held[id] != nil || folderLocks.unsupported[id] {
		return nil
	}
	if _, err := privateDir(dataHome(), false, "sextile", chartBackupSub); err != nil {
		return fmt.Errorf("The charts folder's lock could not be taken (%v). Nothing was written.", err)
	}
	f, unsupported, err := takeLock(BackupDir(chartsDir) + ".writer.lock")
	var held *LockHeldError
	switch {
	case unsupported:
		folderLocks.unsupported[id] = true
		return nil
	case errors.As(err, &held):
		return ErrFolderBusy
	case err != nil:
		return fmt.Errorf("The charts folder's lock could not be taken (%v). Nothing was written.", err)
	}
	folderLocks.held[id] = f
	return nil
}

// FolderLockNote is, once per charts folder, the large log's line when that
// folder's filesystem cannot lock; "" otherwise.
func FolderLockNote(chartsDir string) string {
	id := FolderID(chartsDir)
	folderLocks.Lock()
	defer folderLocks.Unlock()
	if !folderLocks.unsupported[id] || folderLocks.noted[id] {
		return ""
	}
	folderLocks.noted[id] = true
	return "charts folder: this disk cannot lock a file. Do not save from two windows at once"
}

// Door is the one type whose methods reach a charts folder.
type Door struct{ dir string }

// OriginalsDoor is the door to chartsDir. The directory is resolved once by the
// caller and handed in, so a save checks, shows and writes one place.
func OriginalsDoor(chartsDir string) Door { return Door{dir: chartsDir} }

func (d Door) protected() bool { return ReadPosition(d.dir).Effective == PositionProtected }

// journalFailed appends the failed line for a write that did not happen and
// returns err, marked ErrJournalUncorrected when that line could not be added.
func (d Door) journalFailed(err error, file, before, after, backup string) error {
	if jerr := appendJournal(d.dir, actFailed, file, before, after, backup); jerr != nil {
		return fmt.Errorf("%w (%s): %w", ErrJournalUncorrected, file, err)
	}
	return err
}

// Place puts record into the charts folder as <slug>.as and returns the path.
// It never replaces a file (ErrChartExists). A folder that does not exist is
// created and recorded as add before anything is placed in it.
func (d Door) Place(slug string, record []byte) (string, error) {
	if slug == "" {
		return "", fmt.Errorf("store: Place: empty slug")
	}
	if d.dir == "" {
		return "", fmt.Errorf("store: Place: no charts directory given")
	}
	if err := ClaimFolder(d.dir); err != nil {
		return "", err
	}
	if d.protected() {
		return "", ErrOriginalsProtected
	}
	if err := d.makeOwnFolder(); err != nil {
		return "", err
	}
	file := slug + ChartFileExt
	after := sha(record)
	if err := appendJournal(d.dir, actAdd, file, "-", after, "-"); err != nil {
		return "", fmt.Errorf("%w: %v", ErrJournal, err)
	}
	path, err := d.place(slug, record)
	if err != nil {
		return "", d.journalFailed(err, file, "-", after, "-")
	}
	return path, nil
}

// makeOwnFolder creates the charts folder when it does not exist, then writes
// its line as add. If the line cannot be written it removes the folders it
// created, while they are empty, and refuses.
func (d Door) makeOwnFolder() error {
	var missing []string
	for p := filepath.Clean(d.dir); ; p = filepath.Dir(p) {
		if _, err := statPath(p); !errors.Is(err, fs.ErrNotExist) {
			break
		}
		missing = append(missing, p)
		if filepath.Dir(p) == p {
			break
		}
	}
	if len(missing) == 0 {
		return nil
	}
	var made []string
	undo := func() {
		for i := len(made) - 1; i >= 0; i-- {
			os.Remove(made[i]) // a folder that is not empty stays
		}
	}
	for i := len(missing) - 1; i >= 0; i-- {
		err := os.Mkdir(missing[i], 0o700)
		switch {
		case err == nil:
			made = append(made, missing[i])
			syncDir(filepath.Dir(missing[i]))
		case errors.Is(err, fs.ErrExist) && i == 0:
			// Another program made the charts folder meanwhile: it is not
			// Sextile's own, and nothing is placed in it on that claim.
			undo()
			return ErrFolderAppeared
		case errors.Is(err, fs.ErrExist):
		default:
			undo()
			return fmt.Errorf("store: Place: %w", err)
		}
		beforeMkdir()
	}
	if err := setOwnPosition(d.dir); err != nil {
		undo()
		return fmt.Errorf("%w (%v)", ErrNewFolderPosition, err)
	}
	return nil
}

// beforeMkdir runs between the folders the door creates: a test makes the
// charts folder appear there.
var beforeMkdir = func() {}

// setOwnPosition records a folder Sextile created as add. A seam for the
// test of the failing line.
var setOwnPosition = func(dir string) error { return SetPosition(dir, PositionAdd) }

// place writes record into a staging file and gives it the chart's name by a
// move that refuses an existing name.
func (d Door) place(slug string, record []byte) (string, error) {
	path := filepath.Join(d.dir, slug+ChartFileExt)
	st, err := newStaging(d.dir, slug, record)
	if err != nil {
		return "", fmt.Errorf("store: Place: %w", err)
	}
	if err := st.check(); err != nil {
		return "", err
	}
	err = renameNoReplace(st.path, path)
	if errors.Is(err, errNoAtomicMove) {
		return d.placeReserved(st, path)
	}
	if err != nil {
		st.remove()
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("%w: %s", ErrChartExists, path)
		}
		return "", fmt.Errorf("store: Place: %w", err)
	}
	syncDir(d.dir)
	return path, nil
}

// placeReserved is the placement on a filesystem with neither
// RENAME_NOREPLACE nor hard links (FAT, exFAT, some network mounts). The name
// is reserved by an exclusive create of an empty file, the staging file is
// checked, the reserved name is checked to still be that empty file, and the
// staging file is renamed over it. The name holds no file, an empty file, or
// the whole record. On a failure both files the door made are removed.
func (d Door) placeReserved(st staging, path string) (string, error) {
	r, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		st.remove()
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("%w: %s", ErrChartExists, path)
		}
		return "", fmt.Errorf("store: Place: %w", err)
	}
	reserved, err := r.Stat()
	r.Close()
	removeReserved := func() {
		if now, lerr := os.Lstat(path); lerr == nil && os.SameFile(now, reserved) && now.Size() == 0 {
			os.Remove(path)
		}
	}
	if err != nil {
		removeReserved()
		st.remove()
		return "", fmt.Errorf("store: Place: %w", err)
	}
	beforeReservedRename()
	if now, lerr := os.Lstat(path); lerr != nil || !os.SameFile(now, reserved) || now.Size() != 0 || links(now) != 1 {
		st.remove()
		return "", fmt.Errorf("store: Place: %s changed while it was reserved", filepath.Base(path))
	}
	if err := st.check(); err != nil {
		removeReserved()
		return "", err
	}
	if err := os.Rename(st.path, path); err != nil {
		removeReserved()
		st.remove()
		return "", fmt.Errorf("store: Place: %w", err)
	}
	syncDir(d.dir)
	return path, nil
}

// beforeReservedRename runs between the reservation and the rename onto it: a
// test swaps the reserved name there.
var beforeReservedRename = func() {}

// Replace swaps the chart file <file> for record. original is the bytes the
// caller built record from: a file that no longer holds them is not replaced
// (ErrChartChanged). It first keeps the file as it is under
// BackupDir/BackupName(file, stamp), never over an existing file, then appends
// the journal line, then renames a staging file over the chart. It returns
// the dated copy's path as the operator is told it.
func (d Door) Replace(file string, original, record []byte, stamp time.Time) (string, error) {
	base := filepath.Base(file)
	path := filepath.Join(d.dir, base)
	if err := ClaimFolder(d.dir); err != nil {
		return "", err
	}
	if d.protected() {
		return "", ErrOriginalsProtected
	}
	if current, err := readRegular(path); err != nil {
		return "", fmt.Errorf("store: Replace: %w", err)
	} else if !bytes.Equal(current, original) {
		return "", fmt.Errorf("%w: %s", ErrChartChanged, base)
	}
	name := BackupName(base, stamp)
	if err := d.keepDatedCopy(name, original); err != nil {
		return "", err
	}
	before, after := sha(original), sha(record)
	if err := appendJournal(d.dir, actChange, base, before, after, name); err != nil {
		return "", fmt.Errorf("%w: %v", ErrJournal, err)
	}
	if err := d.replace(base, path, original, record); err != nil {
		return "", d.journalFailed(err, base, before, after, name)
	}
	return ChartBackupDisplay(d.dir, name), nil
}

func (d Door) replace(base, path string, original, record []byte) error {
	st, err := newStaging(d.dir, strings.TrimSuffix(base, ChartFileExt), record)
	if err != nil {
		return fmt.Errorf("store: Replace: %w", err)
	}
	if err := beforeReplaceRename(); err != nil {
		st.remove()
		return fmt.Errorf("store: Replace: %w", err)
	}
	// Checked again as late as it can be: an edit saved meanwhile must not be
	// renamed over.
	if now, err := readRegular(path); err != nil || !bytes.Equal(now, original) {
		st.remove()
		return fmt.Errorf("%w: %s", ErrChartChanged, base)
	}
	if err := st.check(); err != nil {
		return err
	}
	if err := os.Rename(st.path, path); err != nil {
		st.remove()
		return fmt.Errorf("store: Replace: %w", err)
	}
	syncDir(d.dir)
	return nil
}

// keepDatedCopy writes original to the backup folder as name, exclusively: an
// existing name refuses the change.
func (d Door) keepDatedCopy(name string, original []byte) error {
	folder, err := backupFolder(d.dir)
	if err != nil {
		return fmt.Errorf("store: Replace: cannot keep a copy: %w", err)
	}
	bak := filepath.Join(folder, name)
	refuse := fmt.Errorf("%w: A backup named %s already exists. Nothing was written.", ErrBackupExists, name)
	if _, err := os.Lstat(bak); err == nil {
		return refuse
	}
	staged, err := os.CreateTemp(folder, "."+name+".*"+chartTempSuffix)
	if err != nil {
		return fmt.Errorf("store: Replace: cannot keep a copy: %w", err)
	}
	n, err := writeBackup(staged, original)
	if err == nil && n != len(original) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = staged.Sync()
	}
	if cerr := staged.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Link(staged.Name(), bak)
		if errors.Is(err, os.ErrExist) {
			err = refuse
		}
	}
	os.Remove(staged.Name())
	syncDir(folder)
	if errors.Is(err, ErrBackupExists) {
		return err
	}
	if err != nil {
		return fmt.Errorf("store: Replace: cannot keep a copy: %w", err)
	}
	return nil
}

// staging is the door's own file in the charts folder: created exclusively
// without following a link, written and given its mode through its handle.
type staging struct {
	path string
	made os.FileInfo
}

func newStaging(dir, stem string, record []byte) (staging, error) {
	var f *os.File
	var err error
	for i := 0; i < 16; i++ {
		var r [6]byte
		rand.Read(r[:])
		name := filepath.Join(dir, "."+stem+"."+hex.EncodeToString(r[:])+chartTempSuffix)
		f, err = os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o600)
		if !errors.Is(err, fs.ErrExist) {
			break
		}
	}
	if err != nil {
		return staging{}, err
	}
	st := staging{path: f.Name()}
	n, err := writeChartRecord(f, record)
	if err == nil && n != len(record) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = f.Chmod(0o600)
	}
	if err == nil {
		err = syncTemp(f)
	}
	if err == nil {
		st.made, err = f.Stat()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(st.path)
		return staging{}, err
	}
	afterStaging(st.path)
	return st, nil
}

// afterStaging runs once the staging file is written: a test swaps it there.
var afterStaging = func(path string) {}

// check is the last look before the rename: the path must still be the same
// regular file, with one link, that the door made. Otherwise nothing is
// published and the foreign entry is left as it is.
func (st staging) check() error {
	now, err := os.Lstat(st.path)
	if err != nil || !now.Mode().IsRegular() || !os.SameFile(now, st.made) || links(now) != 1 {
		return errStagingSwapped
	}
	return nil
}

// remove removes the staging file if it is still the door's own.
func (st staging) remove() {
	if st.check() == nil {
		os.Remove(st.path)
	}
}

func links(fi os.FileInfo) uint64 {
	if s, ok := fi.Sys().(*syscall.Stat_t); ok {
		return uint64(s.Nlink)
	}
	return 1
}

// Fault-injection seams for the door's tests. Production never reassigns them.
var (
	// writeChartRecord writes the record into the new file.
	writeChartRecord = func(w io.Writer, b []byte) (int, error) { return w.Write(b) }
	// syncTemp flushes a new file before it is given its final name.
	syncTemp = func(f *os.File) error { return f.Sync() }
	// linkFile is the hard link linkNoReplace makes.
	linkFile = os.Link
)

// errNoAtomicMove means the filesystem has no atomic move that refuses an
// existing name: neither RENAME_NOREPLACE nor hard links.
var errNoAtomicMove = errors.New("no atomic no-replace move on this filesystem")

// linkNoReplace gives from the name to by a hard link, which refuses an
// existing to (os.ErrExist), and then removes from. A filesystem that cannot
// link answers errNoAtomicMove.
func linkNoReplace(from, to string) error {
	if err := linkFile(from, to); err != nil {
		if linkUnsupported(err) {
			return fmt.Errorf("%w: %v", errNoAtomicMove, err)
		}
		return err
	}
	os.Remove(from)
	return nil
}

// linkUnsupported reports whether a failed link means the filesystem does not
// do hard links, rather than that the link itself failed. EMLINK is not in the
// set: a link-capable filesystem answers it.
func linkUnsupported(err error) bool {
	for _, e := range []syscall.Errno{
		syscall.EPERM, syscall.EOPNOTSUPP, syscall.ENOTSUP, syscall.ENOSYS,
		syscall.EXDEV, syscall.EINVAL,
	} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
