package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lastSessionSandbox gives the test its own XDG bases and two charts folders.
func lastSessionSandbox(t *testing.T) (a, b string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	a, b = filepath.Join(root, "A"), filepath.Join(root, "B")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return a, b
}

func lastOf(t *testing.T, dir string) string {
	t.Helper()
	f, err := LastSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// The record is kept per charts folder, in a private file in the state
// folder, and holds only the session's file name; nothing is written in the
// charts folder.
func TestLastSessionIsAPrivateRecordPerChartsFolder(t *testing.T) {
	a, b := lastSessionSandbox(t)
	if got := lastOf(t, a); got != "" {
		t.Fatalf("a fresh folder has a record: %q", got)
	}
	if err := RecordLastSession(a, "client-work.yaml"); err != nil {
		t.Fatal(err)
	}
	if got := lastOf(t, a); got != "client-work.yaml" {
		t.Errorf("record = %q", got)
	}
	if got := lastOf(t, b); got != "" {
		t.Errorf("folder B reads A's record: %q", got)
	}
	path := filepath.Join(os.Getenv("XDG_STATE_HOME"), "sextile", "last-session", FolderID(a))
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the record is not at %s: %v", path, err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("record mode %v, want 0600", fi.Mode().Perm())
	}
	if di, _ := os.Stat(filepath.Dir(path)); di.Mode().Perm() != 0o700 {
		t.Errorf("record folder mode %v, want 0700", di.Mode().Perm())
	}
	if b, _ := os.ReadFile(path); string(b) != "client-work.yaml\n" {
		t.Errorf("record holds %q, want only the file name", b)
	}
	if entries, _ := os.ReadDir(a); len(entries) != 0 {
		t.Errorf("the charts folder was written to: %v", entries)
	}
	// The last window wins.
	if err := RecordLastSession(a, "other.yaml"); err != nil {
		t.Fatal(err)
	}
	if got := lastOf(t, a); got != "other.yaml" {
		t.Errorf("record after a second switch = %q", got)
	}
	if err := ForgetLastSession(a); err != nil {
		t.Fatal(err)
	}
	if got := lastOf(t, a); got != "" {
		t.Errorf("record after forget = %q", got)
	}
	if err := ForgetLastSession(a); err != nil {
		t.Errorf("forgetting no record: %v", err)
	}
}

// A record that is not a plain session file name is no record, and none is
// written.
func TestLastSessionRefusesAPath(t *testing.T) {
	a, _ := lastSessionSandbox(t)
	for _, bad := range []string{"../x.yaml", "/etc/x.yaml", ".hidden.yaml", "x.txt", ""} {
		if err := RecordLastSession(a, bad); err != nil {
			t.Fatal(err)
		}
		if got := lastOf(t, a); got != "" {
			t.Errorf("RecordLastSession(%q) recorded %q", bad, got)
		}
	}
	dir, err := privateDir(stateHome(), true, "sextile", lastSessionDirName)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, FolderID(a)), []byte("../../escape.yaml\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := lastOf(t, a); got != "" {
		t.Errorf("a record holding a path is read as %q", got)
	}
}

// A rename moves the record with the session, a trash removes it, and either
// leaves a record naming another session alone.
func TestMoveLastSessionKeepsTheRecordTrue(t *testing.T) {
	a, _ := lastSessionSandbox(t)
	if err := RecordLastSession(a, "old.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := MoveLastSession(a, "unrelated.yaml", ""); err != nil || lastOf(t, a) != "old.yaml" {
		t.Errorf("trashing another session changed the record: %q, %v", lastOf(t, a), err)
	}
	if err := MoveLastSession(a, "old.yaml", "new.yaml"); err != nil || lastOf(t, a) != "new.yaml" {
		t.Errorf("after a rename the record is %q, %v", lastOf(t, a), err)
	}
	if err := MoveLastSession(a, "new.yaml", ""); err != nil || lastOf(t, a) != "" {
		t.Errorf("after a trash the record is %q, %v", lastOf(t, a), err)
	}
	if err := MoveLastSession(a, "", "x.yaml"); err != nil || lastOf(t, a) != "" {
		t.Errorf("no record became %q, %v", lastOf(t, a), err)
	}
}

// ReopenLastSession opens the recorded session; a missing one clears the
// record, and a refused one leaves the record and the file as they are.
func TestReopenLastSession(t *testing.T) {
	a, _ := lastSessionSandbox(t)
	if file, _, lock, err := ReopenLastSession(a); file != "" || lock != nil || err != nil {
		t.Fatalf("no record opened %q (%v)", file, err)
	}
	l, err := SaveNamedSessionAs("Client Work", Session{Folder: FolderID(a)})
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	if err := RecordLastSession(a, "client-work.yaml"); err != nil {
		t.Fatal(err)
	}
	file, name, lock, err := ReopenLastSession(a)
	if err != nil || file != "client-work.yaml" || name != "Client Work" || lock == nil {
		t.Fatalf("reopen = %q %q %v %v", file, name, lock, err)
	}
	// Open in this window now: a second reopen is refused, record kept.
	if _, name, l2, err := ReopenLastSession(a); err != ErrSessionOpenElsewhere || l2 != nil || name != "Client Work" {
		t.Errorf("a held session reopened: %q %v %v", name, l2, err)
	}
	lock.Release()
	if lastOf(t, a) != "client-work.yaml" {
		t.Error("a refused reopen cleared the record")
	}

	if err := RecordLastSession(a, "gone.yaml"); err != nil {
		t.Fatal(err)
	}
	if _, name, _, err := ReopenLastSession(a); err != ErrSessionNotFound || name != "gone" {
		t.Errorf("missing: %q %v", name, err)
	}
	if got := lastOf(t, a); got != "" {
		t.Errorf("a missing session's record stayed: %q", got)
	}

	junk := "version: 4\nname: Odd\nfolder: " + FolderID(a) + "\nsomething: unknown\n"
	if err := os.WriteFile(NamedSessionPath("odd.yaml"), []byte(junk), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RecordLastSession(a, "odd.yaml"); err != nil {
		t.Fatal(err)
	}
	if _, name, l3, err := ReopenLastSession(a); err == nil || l3 != nil || name != "Odd" {
		t.Errorf("an unusable session reopened: %q %v", name, err)
	}
	if b, _ := os.ReadFile(NamedSessionPath("odd.yaml")); string(b) != junk {
		t.Error("the refused session's file changed")
	}
	if lastOf(t, a) != "odd.yaml" {
		t.Error("an unusable session's record was cleared")
	}
	if strings.Contains(lastOf(t, a), "/") {
		t.Error("record holds a path")
	}
}

// A window acting on a record it read earlier never undoes a newer window's
// record: clearing a missing session, a rename and a trash each change the
// record only while it still names the session they act on.
func TestRecordChangesActOnTheRecordAsItIsNow(t *testing.T) {
	a, _ := lastSessionSandbox(t)
	// Window A read old.yaml; window B has since switched to new.yaml.
	if err := RecordLastSession(a, "new.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := forgetLastSession(a, "old.yaml"); err != nil || lastOf(t, a) != "new.yaml" {
		t.Errorf("clearing the missing old.yaml took the newer record: %q, %v", lastOf(t, a), err)
	}
	if err := MoveLastSession(a, "old.yaml", "renamed.yaml"); err != nil || lastOf(t, a) != "new.yaml" {
		t.Errorf("renaming old.yaml took the newer record: %q, %v", lastOf(t, a), err)
	}
	if err := MoveLastSession(a, "old.yaml", ""); err != nil || lastOf(t, a) != "new.yaml" {
		t.Errorf("trashing old.yaml took the newer record: %q, %v", lastOf(t, a), err)
	}
	// Window B reopens and finds new.yaml gone: its own record goes.
	if _, _, _, err := ReopenLastSession(a); err != ErrSessionNotFound || lastOf(t, a) != "" {
		t.Errorf("a missing new.yaml kept its record: %q, %v", lastOf(t, a), err)
	}
}
