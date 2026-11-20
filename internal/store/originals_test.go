// All chart values in this file are constructed synthetic test inputs.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Verifiers for specs/originals-door.md's store rows. Every test runs in
// testenv's sandbox: HOME and the XDG bases are temp folders.

const doorRecord = "@AI800  ; Astrolog chart info.\n-qb Mar 20 1995 12:00pm DT 1E   8:48E 53:04N\n-zi \"Constructed\" \"Bremen, Germany\"\n"

// snapshot is the folder snapshot: every entry's name, type, size and SHA-256,
// hidden entries included, sorted.
func snapshot(t *testing.T, dir string) string {
	t.Helper()
	var rows []string
	filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		row := fmt.Sprintf("%s %s %d", rel, fi.Mode().Type(), fi.Size())
		if fi.Mode().IsRegular() {
			b, _ := os.ReadFile(p)
			s := sha256.Sum256(b)
			row += " " + hex.EncodeToString(s[:])
		}
		rows = append(rows, row)
		return nil
	})
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}

// writePositionFile writes the position file's bytes as they are.
func writePositionFile(t *testing.T, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(PositionFile()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(PositionFile(), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func identity(t *testing.T, dir string) string {
	t.Helper()
	id, ok := positionIdentity(dir)
	if !ok {
		t.Fatalf("no identity for %s", dir)
	}
	return id
}

func journalLines(t *testing.T, dir string) []string {
	t.Helper()
	b, err := os.ReadFile(JournalPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// O1
func TestOriginalsPositionFromFile(t *testing.T) {
	dir := t.TempDir()
	if r := ReadPosition(dir); r.Effective != PositionAsk || r.HasLine || r.Fault != "" {
		t.Errorf("no file: %+v, want ask with no line", r)
	}
	for _, p := range []Position{PositionProtected, PositionAsk, PositionAdd, PositionAllow} {
		writePositionFile(t, "# my folders\n\n/elsewhere=protected\n"+identity(t, dir)+"="+string(p)+"\n")
		if r := ReadPosition(dir); r.Effective != p || r.Stored != p || !r.HasLine {
			t.Errorf("%s: %+v", p, r)
		}
	}
	writePositionFile(t, "/elsewhere=allow\n")
	if r := ReadPosition(dir); r.Effective != PositionAsk || r.HasLine {
		t.Errorf("another folder's line: %+v, want ask", r)
	}
	t.Run("split at the last =", func(t *testing.T) {
		odd := filepath.Join(t.TempDir(), "a=b")
		if err := os.Mkdir(odd, 0o700); err != nil {
			t.Fatal(err)
		}
		writePositionFile(t, identity(t, odd)+"=add\n")
		if r := ReadPosition(odd); r.Effective != PositionAdd {
			t.Errorf("%+v, want add", r)
		}
	})
}

// O2
func TestOriginalsPositionFailsClosed(t *testing.T) {
	dir := t.TempDir()
	id := identity(t, dir)
	for _, tc := range []struct{ name, body, fault string }{
		{"two lines", id + "=ask\n" + id + "=add\n", "has 2 lines for it"},
		{"unknown word", id + "=open\n", `says "open" for it`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writePositionFile(t, tc.body)
			if r := ReadPosition(dir); r.Effective != PositionProtected || r.Fault != tc.fault {
				t.Errorf("%+v, want protected with %q", r, tc.fault)
			}
		})
	}
	t.Run("not a file", func(t *testing.T) {
		writePositionFile(t, "")
		os.Remove(PositionFile())
		if err := os.Mkdir(PositionFile(), 0o700); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Remove(PositionFile()) })
		if r := ReadPosition(dir); r.Effective != PositionProtected || r.Fault != faultNotAFile {
			t.Errorf("%+v", r)
		}
	})
	t.Run("a link", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "elsewhere.conf")
		os.WriteFile(target, []byte(id+"=allow\n"), 0o600)
		os.Remove(PositionFile())
		if err := os.Symlink(target, PositionFile()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Remove(PositionFile()) })
		if r := ReadPosition(dir); r.Effective != PositionProtected || r.Fault != faultNotAFile {
			t.Errorf("%+v", r)
		}
	})
	t.Run("cannot be read", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root reads anything")
		}
		writePositionFile(t, id+"=allow\n")
		os.Chmod(PositionFile(), 0)
		t.Cleanup(func() { os.Chmod(PositionFile(), 0o600) })
		if r := ReadPosition(dir); r.Effective != PositionProtected || r.Fault != faultUnreadable {
			t.Errorf("%+v", r)
		}
	})
	t.Run("an identity with a newline", func(t *testing.T) {
		odd := filepath.Join(t.TempDir(), "two\nlines")
		if err := os.Mkdir(odd, 0o700); err != nil {
			t.Fatal(err)
		}
		if r := ReadPosition(odd); r.Effective != PositionProtected || r.Fault != faultNoIdentity {
			t.Errorf("%+v", r)
		}
	})
}

// O3
func TestOriginalsReadonlyEnvWins(t *testing.T) {
	dir := t.TempDir()
	writePositionFile(t, identity(t, dir)+"=allow\n")
	t.Setenv("SEXTILE_CHARTS_READONLY", "1")
	r := ReadPosition(dir)
	if r.Effective != PositionProtected || r.Stored != PositionAllow || !r.ReadonlyEnv {
		t.Errorf("%+v, want protected over a stored allow", r)
	}
	before := snapshot(t, dir)
	if _, err := OriginalsDoor(dir).Place("anna", []byte(doorRecord)); !errors.Is(err, ErrOriginalsProtected) {
		t.Errorf("Place: %v", err)
	}
	if snapshot(t, dir) != before {
		t.Error("the folder changed")
	}
	seedChart(t, dir, "Anna.as", doorRecord)
	before = snapshot(t, dir)
	if _, err := OriginalsDoor(dir).Replace("Anna.as", []byte(doorRecord), []byte("x"), stamp); !errors.Is(err, ErrOriginalsProtected) {
		t.Errorf("Replace: %v", err)
	}
	if snapshot(t, dir) != before {
		t.Error("the folder changed")
	}
	if err := SetPosition(dir, PositionAsk); err != nil {
		t.Fatal(err)
	}
	if r := ReadPosition(dir); r.Stored != PositionAsk || r.Effective != PositionProtected {
		t.Errorf("after SetPosition: %+v", r)
	}
}

// O4
func TestDoorRefusesWhenProtected(t *testing.T) {
	dir := t.TempDir()
	seedChart(t, dir, "Anna.as", doorRecord)
	writePositionFile(t, identity(t, dir)+"=protected\n")
	before := snapshot(t, dir)
	if _, err := OriginalsDoor(dir).Place("ben", []byte(doorRecord)); !errors.Is(err, ErrOriginalsProtected) {
		t.Errorf("Place: %v", err)
	}
	if _, err := OriginalsDoor(dir).Replace("Anna.as", []byte(doorRecord), []byte(doorRecord+"; x\n"), stamp); !errors.Is(err, ErrOriginalsProtected) {
		t.Errorf("Replace: %v", err)
	}
	if snapshot(t, dir) != before {
		t.Error("the charts folder changed")
	}
	if l := journalLines(t, dir); len(l) != 0 {
		t.Errorf("the journal holds %q", l)
	}
	if _, err := os.Stat(BackupDir(dir)); !os.IsNotExist(err) {
		t.Error("a backup folder was made")
	}
}

// O10
func TestCreatedFolderIsOwn(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "charts")
	if _, err := OriginalsDoor(dir).Place("anna", []byte(doorRecord)); err != nil {
		t.Fatal(err)
	}
	if r := ReadPosition(dir); r.Stored != PositionAdd {
		t.Errorf("%+v, want add", r)
	}
	t.Run("the folder appears meanwhile", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "new", "charts")
		prev := beforeMkdir
		t.Cleanup(func() { beforeMkdir = prev })
		beforeMkdir = func() {
			if _, err := os.Stat(filepath.Dir(dir)); err == nil {
				os.Mkdir(dir, 0o700)
				os.WriteFile(filepath.Join(dir, "Theirs.as"), []byte(doorRecord), 0o600)
			}
		}
		_, err := OriginalsDoor(dir).Place("anna", []byte(doorRecord))
		if !errors.Is(err, ErrFolderAppeared) {
			t.Fatalf("err = %v, want ErrFolderAppeared", err)
		}
		if r := ReadPosition(dir); r.HasLine {
			t.Errorf("a folder another program made was recorded: %+v", r)
		}
		onlyEntries(t, dir, "Theirs.as")
	})
	t.Run("the line cannot be written", func(t *testing.T) {
		parent := t.TempDir()
		dir := filepath.Join(parent, "charts")
		prev := setOwnPosition
		t.Cleanup(func() { setOwnPosition = prev })
		setOwnPosition = func(string) error { return errors.New("injected") }
		_, err := OriginalsDoor(dir).Place("anna", []byte(doorRecord))
		if !errors.Is(err, ErrNewFolderPosition) || !strings.HasPrefix(err.Error(), "The position of the new folder could not be recorded. Nothing was written.") {
			t.Fatalf("err = %v", err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Error("the created folder was left")
		}
	})
}

// O12
func TestEveryChangeKeepsADatedCopy(t *testing.T) {
	if got := BackupName("Anna.as", stamp); got != "Anna.20261113T091800Z.as" {
		t.Errorf("BackupName = %q", got)
	}
	if got := BackupName("notes.as.as", stamp); got != "notes.as.20261113T091800Z.as" {
		t.Errorf("one final .as: %q", got)
	}
	if got := BackupName("plain", stamp); got != "plain.20261113T091800Z.as" {
		t.Errorf("no .as: %q", got)
	}
	dir := t.TempDir()
	path := seedChart(t, dir, "Anna.as", doorRecord)
	second := doorRecord + "; second\n"
	if _, err := OriginalsDoor(dir).Replace("Anna.as", []byte(doorRecord), []byte(second), stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := OriginalsDoor(dir).Replace("Anna.as", []byte(second), []byte(doorRecord+"; third\n"), stamp.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{BackupName("Anna.as", stamp): doorRecord, BackupName("Anna.as", stamp.Add(time.Minute)): second} {
		if got := readString(t, filepath.Join(BackupDir(dir), name)); got != want {
			t.Errorf("%s holds %q, want %q", name, got, want)
		}
	}
	t.Run("a taken name refuses", func(t *testing.T) {
		now := readString(t, path)
		_, err := OriginalsDoor(dir).Replace("Anna.as", []byte(now), []byte("x"), stamp)
		want := "A backup named " + BackupName("Anna.as", stamp) + " already exists. Nothing was written."
		if !errors.Is(err, ErrBackupExists) || !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %v", err)
		}
		if readString(t, path) != now {
			t.Error("the chart changed")
		}
		if got := readString(t, filepath.Join(BackupDir(dir), BackupName("Anna.as", stamp))); got != doorRecord {
			t.Errorf("the existing copy was written over: %q", got)
		}
	})
}

// O13
func TestOldBackupsUntouched(t *testing.T) {
	dir := t.TempDir()
	seedChart(t, dir, "Anna.as", doorRecord)
	folder, err := backupFolder(dir)
	if err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(folder, "Anna.as")
	if err := os.WriteFile(old, []byte("the first original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OriginalsDoor(dir).Replace("Anna.as", []byte(doorRecord), []byte(doorRecord+"; x\n"), stamp); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, old); got != "the first original\n" {
		t.Errorf("the old backup changed: %q", got)
	}
}

// O14
func TestJournalComesFirst(t *testing.T) {
	dir := t.TempDir()
	seedChart(t, dir, "Anna.as", doorRecord)
	prev := appendJournal
	t.Cleanup(func() { appendJournal = prev })
	appendJournal = func(string, string, string, string, string, string) error { return errors.New("disk full") }
	before := snapshot(t, dir)
	if _, err := OriginalsDoor(dir).Place("ben", []byte(doorRecord)); !errors.Is(err, ErrJournal) || !strings.HasPrefix(err.Error(), "The journal could not be written. Nothing was written.") {
		t.Errorf("Place: %v", err)
	}
	if _, err := OriginalsDoor(dir).Replace("Anna.as", []byte(doorRecord), []byte("x"), stamp); !errors.Is(err, ErrJournal) {
		t.Errorf("Replace: %v", err)
	}
	if snapshot(t, dir) != before {
		t.Error("the charts folder changed without a journal line")
	}
}

// O15
func TestJournalLineFormat(t *testing.T) {
	dir := t.TempDir()
	seedChart(t, dir, "Anna \"A\".as", doorRecord)
	if _, err := OriginalsDoor(dir).Place("ben", []byte(doorRecord)); err != nil {
		t.Fatal(err)
	}
	if _, err := OriginalsDoor(dir).Replace("Anna \"A\".as", []byte(doorRecord), []byte("new"), stamp); err != nil {
		t.Fatal(err)
	}
	lines := journalLines(t, dir)
	if len(lines) != 2 {
		t.Fatalf("journal = %q", lines)
	}
	want := [][]string{
		{"add", strconv.Quote("ben.as"), "-", sha([]byte(doorRecord)), "-"},
		{"change", strconv.Quote("Anna \"A\".as"), sha([]byte(doorRecord)), sha([]byte("new")), BackupName("Anna \"A\".as", stamp)},
	}
	for i, l := range lines {
		f := strings.Split(l, "\t")
		if len(f) != 6 {
			t.Fatalf("line %d has %d fields: %q", i, len(f), l)
		}
		if _, err := time.Parse(time.RFC3339, f[0]); err != nil || !strings.HasSuffix(f[0], "Z") {
			t.Errorf("line %d time %q", i, f[0])
		}
		if strings.Join(f[1:], "|") != strings.Join(want[i], "|") {
			t.Errorf("line %d = %q, want %q", i, f[1:], want[i])
		}
	}
}

// O16
func TestJournalRecordsAFailedWrite(t *testing.T) {
	dir := t.TempDir()
	seedChart(t, dir, "Anna.as", doorRecord)
	prev := beforeReplaceRename
	t.Cleanup(func() { beforeReplaceRename = prev })
	beforeReplaceRename = func() error { return errors.New("injected") }
	before := snapshot(t, dir)
	if _, err := OriginalsDoor(dir).Replace("Anna.as", []byte(doorRecord), []byte("new"), stamp); err == nil {
		t.Fatal("no error")
	}
	if snapshot(t, dir) != before {
		t.Error("the charts folder changed")
	}
	lines := journalLines(t, dir)
	if len(lines) != 2 || strings.Split(lines[1], "\t")[1] != "failed" ||
		strings.Join(strings.Split(lines[1], "\t")[2:6], "|") != strings.Join(strings.Split(lines[0], "\t")[2:6], "|") {
		t.Errorf("journal = %q, want a change line and a failed line for the same file, hashes and copy", lines)
	}
	t.Run("the failed line cannot be written", func(t *testing.T) {
		prevJ := appendJournal
		t.Cleanup(func() { appendJournal = prevJ })
		appendJournal = func(dir, act, file, b, a, k string) error {
			if act == actFailed {
				return errors.New("disk full")
			}
			return prevJ(dir, act, file, b, a, k)
		}
		_, err := OriginalsDoor(dir).Replace("Anna.as", []byte(doorRecord), []byte("new"), stamp.Add(time.Second))
		if !errors.Is(err, ErrJournalUncorrected) {
			t.Errorf("err = %v, want ErrJournalUncorrected", err)
		}
	})
}

// O17
func TestJournalFileIsPrivate(t *testing.T) {
	dir := t.TempDir()
	if _, err := OriginalsDoor(dir).Place("anna", []byte(doorRecord)); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(JournalPath(dir))
	if err != nil || fi.Mode().Perm() != 0o600 || JournalPath(dir) != filepath.Join(os.Getenv("XDG_DATA_HOME"), "sextile", "backup", FolderID(dir)+".journal") {
		t.Errorf("journal %s: %v %v", JournalPath(dir), fi, err)
	}
	t.Run("an existing journal open to others is narrowed", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(BackupBase(), 0o700)
		for i, mode := range []os.FileMode{0o644, 0o700} {
			if err := os.WriteFile(JournalPath(dir), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			os.Chmod(JournalPath(dir), mode)
			if _, err := OriginalsDoor(dir).Place(fmt.Sprintf("anna%d", i), []byte(doorRecord)); err != nil {
				t.Fatal(err)
			}
			if fi, err := os.Stat(JournalPath(dir)); err != nil || fi.Mode().Perm() != 0o600 {
				t.Errorf("from %v: journal mode %v (%v), want 0600", mode, fi.Mode().Perm(), err)
			}
		}
	})
	t.Run("not a regular file", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(BackupBase(), 0o700)
		if err := os.Mkdir(JournalPath(dir), 0o700); err != nil {
			t.Fatal(err)
		}
		before := snapshot(t, dir)
		if _, err := OriginalsDoor(dir).Place("anna", []byte(doorRecord)); !errors.Is(err, ErrJournal) {
			t.Errorf("err = %v, want ErrJournal", err)
		}
		if snapshot(t, dir) != before {
			t.Error("the charts folder changed")
		}
	})
}

// O21
func TestPositionFileKeepsOtherLines(t *testing.T) {
	dir := t.TempDir()
	id := identity(t, dir)
	body := "# hand-written\r\n\n/one=allow\n" + id + "=ask\n/two=protected"
	writePositionFile(t, body)
	if err := SetPosition(dir, PositionProtected); err != nil {
		t.Fatal(err)
	}
	if got, want := readString(t, PositionFile()), strings.Replace(body, id+"=ask", id+"=protected", 1); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	other := t.TempDir()
	if err := SetPosition(other, PositionAdd); err != nil {
		t.Fatal(err)
	}
	// The last line has no line end, so the new line goes before it and every
	// other line keeps its bytes.
	if got, want := readString(t, PositionFile()), strings.Replace(strings.Replace(body, id+"=ask", id+"=protected", 1), "/two=protected", identity(t, other)+"=add\n/two=protected", 1); got != want {
		t.Errorf("appended: got %q, want %q", got, want)
	}
	if fi, _ := os.Stat(PositionFile()); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	t.Run("a link is refused", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "real.conf")
		os.WriteFile(target, []byte("/x=ask\n"), 0o600)
		os.Remove(PositionFile())
		if err := os.Symlink(target, PositionFile()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Remove(PositionFile()) })
		if err := SetPosition(dir, PositionAsk); err == nil {
			t.Error("a linked position file was written")
		}
		if got := readString(t, target); got != "/x=ask\n" {
			t.Errorf("the link's target changed: %q", got)
		}
	})
}

// O21a
func TestPositionFileSeesAConcurrentWrite(t *testing.T) {
	dir := t.TempDir()
	writePositionFile(t, "/one=allow\n")
	prev := beforePositionRename
	t.Cleanup(func() { beforePositionRename = prev })
	n := 0
	beforePositionRename = func() {
		n++
		if n == 1 {
			os.WriteFile(PositionFile(), []byte("/one=allow\n/launcher=ask\n"), 0o600)
		}
	}
	if err := SetPosition(dir, PositionAdd); err != nil {
		t.Fatal(err)
	}
	if got, want := readString(t, PositionFile()), "/one=allow\n/launcher=ask\n"+identity(t, dir)+"=add\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	t.Run("three changes refuse", func(t *testing.T) {
		writePositionFile(t, "/one=allow\n")
		m := 0
		beforePositionRename = func() {
			m++
			os.WriteFile(PositionFile(), []byte(fmt.Sprintf("/one=allow\n/n=%d\n", m)), 0o600)
		}
		if err := SetPosition(dir, PositionAsk); err == nil {
			t.Error("a file that kept changing was replaced")
		}
		if got := readString(t, PositionFile()); got != "/one=allow\n/n=3\n" {
			t.Errorf("file = %q, want the last concurrent write", got)
		}
	})
}

// O27
func TestSwappedStagingFileIsNeverPublished(t *testing.T) {
	dir := t.TempDir()
	seedChart(t, dir, "Anna.as", doorRecord)
	bob := seedChart(t, dir, "Bob.as", "bob's own chart\n")
	prev := afterStaging
	t.Cleanup(func() { afterStaging = prev })
	var swapped string
	afterStaging = func(path string) {
		swapped = path
		os.Remove(path)
		if err := os.Symlink(bob, path); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := OriginalsDoor(dir).Replace("Anna.as", []byte(doorRecord), []byte("new"), stamp); err == nil {
		t.Fatal("a swapped staging file was published")
	}
	if fi, err := os.Lstat(filepath.Join(dir, "Anna.as")); err != nil || !fi.Mode().IsRegular() || readString(t, filepath.Join(dir, "Anna.as")) != doorRecord {
		t.Error("Anna.as is not the original regular file")
	}
	if readString(t, bob) != "bob's own chart\n" {
		t.Error("Bob.as changed")
	}
	if fi, err := os.Lstat(swapped); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the foreign entry was not left as it is")
	}
	if _, err := OriginalsDoor(dir).Place("carl", []byte(doorRecord)); err == nil {
		t.Error("Place published a swapped staging file")
	}
	if _, err := os.Lstat(filepath.Join(dir, "carl.as")); !os.IsNotExist(err) {
		t.Error("carl.as was placed")
	}
}

// Failure modes: a read-only folder.
func TestReadOnlyFilesystemIsReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores mode bits")
	}
	dir := t.TempDir()
	seedChart(t, dir, "Anna.as", doorRecord)
	os.Chmod(dir, 0o555)
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	before := snapshot(t, dir)
	if _, err := OriginalsDoor(dir).Place("ben", []byte(doorRecord)); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("err = %v, want the system's reason", err)
	}
	if snapshot(t, dir) != before {
		t.Error("the folder changed")
	}
	lines := journalLines(t, dir)
	if len(lines) != 2 || strings.Split(lines[1], "\t")[1] != "failed" {
		t.Errorf("journal = %q, want add then failed", lines)
	}
}

// Failure modes: the position is read at each act.
func TestPositionIsReadAtEachAct(t *testing.T) {
	dir := t.TempDir()
	d := OriginalsDoor(dir)
	if _, err := d.Place("anna", []byte(doorRecord)); err != nil {
		t.Fatal(err)
	}
	writePositionFile(t, identity(t, dir)+"=protected\n")
	if _, err := d.Place("ben", []byte(doorRecord)); !errors.Is(err, ErrOriginalsProtected) {
		t.Errorf("err = %v, want the newer protected", err)
	}
}

// holdElsewhere takes path's flock through a file of its own, as another
// Sextile window would, and returns the release.
func holdElsewhere(t *testing.T, path string) func() {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o700)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	release := func() { f.Close() }
	t.Cleanup(release)
	return release
}

// O28
func TestFirstWriterHoldsTheFolder(t *testing.T) {
	dir := t.TempDir()
	if _, err := OriginalsDoor(dir).Place("anna", []byte(doorRecord)); err != nil {
		t.Fatal(err)
	}
	lock := BackupDir(dir) + ".writer.lock"
	f, err := os.OpenFile(lock, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("no lock file beside the journal: %v", err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		t.Error("the folder's lock is not held after the first write")
	}
	if strings.HasPrefix(lock, dir) {
		t.Errorf("the lock %s is inside the charts folder", lock)
	}
	if _, err := OriginalsDoor(dir).Place("ben", []byte(doorRecord)); err != nil {
		t.Errorf("the holder's second write: %v", err)
	}
	t.Run("a change of the line claims too", func(t *testing.T) {
		dir := t.TempDir()
		if err := SetPosition(dir, PositionProtected); err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(BackupDir(dir)+".writer.lock", os.O_RDWR, 0)
		if err != nil {
			t.Fatalf("no lock after a change of the line: %v", err)
		}
		defer f.Close()
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			t.Error("the folder's lock is not held after a change of its line")
		}
	})
}

// O29, the store half: another holder refuses every act and every change of
// the folder's line, before the journal and before anything is written.
func TestAnotherHolderRefusesEveryWrite(t *testing.T) {
	dir := t.TempDir()
	seedChart(t, dir, "Anna.as", doorRecord)
	holdElsewhere(t, BackupDir(dir)+".writer.lock")
	before := snapshot(t, dir)
	if _, err := OriginalsDoor(dir).Place("ben", []byte(doorRecord)); !errors.Is(err, ErrFolderBusy) {
		t.Errorf("Place: %v", err)
	}
	if _, err := OriginalsDoor(dir).Replace("Anna.as", []byte(doorRecord), []byte("x"), stamp); !errors.Is(err, ErrFolderBusy) {
		t.Errorf("Replace: %v", err)
	}
	if err := SetPosition(dir, PositionProtected); !errors.Is(err, ErrFolderBusy) {
		t.Errorf("SetPosition: %v", err)
	}
	if ErrFolderBusy.Error() != "Another Sextile window writes this charts folder. Nothing was written." {
		t.Errorf("text %q", ErrFolderBusy)
	}
	if snapshot(t, dir) != before || len(journalLines(t, dir)) != 0 || ReadPosition(dir).HasLine {
		t.Error("something was written")
	}
	t.Run("a protected folder answers the holder first", func(t *testing.T) {
		dir := t.TempDir()
		seedChart(t, dir, "Anna.as", doorRecord)
		writePositionFile(t, identity(t, dir)+"=protected\n")
		holdElsewhere(t, BackupDir(dir)+".writer.lock")
		if _, err := OriginalsDoor(dir).Replace("Anna.as", []byte(doorRecord), []byte("x"), stamp); !errors.Is(err, ErrFolderBusy) {
			t.Errorf("Replace: %v, want ErrFolderBusy before the position is read", err)
		}
		if _, err := OriginalsDoor(dir).Place("ben", []byte(doorRecord)); !errors.Is(err, ErrFolderBusy) {
			t.Errorf("Place: %v, want ErrFolderBusy before the position is read", err)
		}
	})
}

// O30
func TestFolderLockUnsupported(t *testing.T) {
	prev := instanceLockFault
	t.Cleanup(func() { instanceLockFault = prev })
	instanceLockFault = "ENOTSUP"
	dir := t.TempDir()
	if _, err := OriginalsDoor(dir).Place("anna", []byte(doorRecord)); err != nil {
		t.Fatalf("a folder that cannot lock refused the write: %v", err)
	}
	if FolderLockNote(dir) == "" {
		t.Error("no note for the large log")
	}
	if FolderLockNote(dir) != "" {
		t.Error("the note came twice")
	}
}

// O31
func TestPositionWritesAreSerialised(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Dir(PositionFile()), 0o700)
	release := holdElsewhere(t, PositionFile()+".lock")
	done := make(chan error, 1)
	go func() { done <- SetPosition(dir, PositionAdd) }()
	time.Sleep(150 * time.Millisecond)
	if ReadPosition(dir).HasLine {
		t.Fatal("the file was written while another write held its lock")
	}
	// The other writer's change lands while it holds the lock; the waiting
	// write must read the file after that, and keep the change.
	if err := os.WriteFile(PositionFile(), []byte("/other=protected\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if ReadPosition(dir).Stored != PositionAdd {
		t.Error("the write did not happen after the lock was released")
	}
	if got := readString(t, PositionFile()); !strings.HasPrefix(got, "/other=protected\n") {
		t.Errorf("the other writer's line was lost: %q", got)
	}
}
