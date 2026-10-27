// All chart values in this file are constructed synthetic test inputs.
package store

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
)

// namedFixture is a session with one chart, saved in charts.
func namedFixture(charts string) Session {
	return Session{
		Folder: FolderID(charts),
		Slots: []SessionSlot{{
			Chart:    SessionChart{ID: "client-ann", Name: "Client Ann", When: time.Date(1990, 5, 6, 7, 8, 0, 0, time.UTC), Place: location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"}, Source: ChartSource{Kind: ChartKindFile, File: "client-ann.as"}},
			Usable:   true,
			Selected: true,
		}},
		FloatingSelected: true,
	}
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// Save as writes sessions/<file>.yaml, 0600 in a 0700 folder: the general
// session's own text with `name:` straight after the version line.
func TestSaveSessionAsWritesANamedFile(t *testing.T) {
	charts := stateSandbox(t)
	s := namedFixture(charts)
	lock, err := SaveNamedSessionAs("Client Ann: second look", s)
	if err != nil {
		t.Fatalf("SaveNamedSessionAs: %v", err)
	}
	defer lock.Release()
	if lock.File() != "client-ann-second-look.yaml" {
		t.Errorf("file = %q, want the chart-file naming rule's client-ann-second-look.yaml", lock.File())
	}
	path := filepath.Join(os.Getenv("XDG_DATA_HOME"), "sextile", "sessions", "client-ann-second-look.yaml")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("no named file: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("file mode %v, want 0600", fi.Mode().Perm())
	}
	if di, _ := os.Stat(filepath.Dir(path)); di == nil || di.Mode().Perm() != 0o700 {
		t.Errorf("sessions folder mode %v, want 0700", di)
	}
	b, _ := os.ReadFile(path)
	text := string(b)
	lines := strings.SplitN(text, "\n", 3)
	if lines[0] != "version: 3" || lines[1] != `name: "Client Ann: second look"` {
		t.Errorf("the file opens %q, %q; want the version then name:", lines[0], lines[1])
	}
	if !strings.Contains(text, "\nfolder: "+FolderID(charts)+"\n") {
		t.Errorf("no folder: line for this charts folder:\n%s", text)
	}
	if rest := strings.Replace(text, lines[1]+"\n", "", 1); rest != formatSession(s) {
		t.Errorf("beyond name:, the file is not the general session's format:\n--- got ---\n%s--- want ---\n%s", rest, formatSession(s))
	}
	got, name, err := LoadNamedSession(lock.File())
	if err != nil || name != "Client Ann: second look" || len(got.Slots) != 1 || got.Slots[0].Chart.Name != "Client Ann" {
		t.Errorf("read back: %+v name %q err %v", got, name, err)
	}
}

// A name whose file exists is refused, and nothing at all is written.
func TestSaveSessionAsRefusesATakenName(t *testing.T) {
	charts := stateSandbox(t)
	dir := SessionsDir()
	writeFile(t, filepath.Join(dir, "client-ann.yaml"), "version: 3\nname: Client Ann\n# theirs\n")
	before := dirNames(t, dir)
	_, err := SaveNamedSessionAs("client  ANN", namedFixture(charts))
	if !errors.Is(err, ErrSessionNameTaken) {
		t.Fatalf("err = %v, want ErrSessionNameTaken", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "client-ann.yaml")); string(b) != "version: 3\nname: Client Ann\n# theirs\n" {
		t.Errorf("the existing file changed: %q", b)
	}
	if after := dirNames(t, dir); !slices.Equal(after, before) {
		t.Errorf("the sessions folder changed: %v, was %v", after, before)
	}
	if _, err := SaveNamedSessionAs("!!!", namedFixture(charts)); !errors.Is(err, ErrSessionNameEmpty) {
		t.Errorf("a name with no letters: err = %v", err)
	}
}

// The list holds this charts folder's sessions only, newest first.
func TestSessionsListShowsOnlyThisFolder(t *testing.T) {
	charts := stateSandbox(t)
	other := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, folder string }{{"Old A", charts}, {"New A", charts}, {"From B", other}} {
		l, err := SaveNamedSessionAs(c.name, namedFixture(c.folder))
		if err != nil {
			t.Fatal(err)
		}
		l.Release()
	}
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	for file, at := range map[string]time.Time{"old-a.yaml": base, "new-a.yaml": base.Add(time.Hour), "from-b.yaml": base.Add(2 * time.Hour)} {
		if err := os.Chtimes(NamedSessionPath(file), at, at); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ListNamedSessions(FolderID(charts), "")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range got {
		names = append(names, s.Name)
	}
	if !slices.Equal(names, []string{"New A", "Old A"}) {
		t.Errorf("listed %v, want [New A Old A]", names)
	}
	if len(got) == 2 && !got[0].Modified.Equal(base.Add(time.Hour)) {
		t.Errorf("Modified = %v, want the file's mtime", got[0].Modified)
	}
}

// A rename publishes the new name exclusively and unlinks the old: a taken
// name is refused with both files untouched, and the lock of the session this
// window runs follows it.
func TestRenameSessionMovesFileExclusively(t *testing.T) {
	charts := stateSandbox(t)
	id := FolderID(charts)
	running, err := SaveNamedSessionAs("Ann", namedFixture(charts))
	if err != nil {
		t.Fatal(err)
	}
	l, err := SaveNamedSessionAs("Bob", namedFixture(charts))
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	annBefore, _ := os.ReadFile(NamedSessionPath("ann.yaml"))
	bobBefore, _ := os.ReadFile(NamedSessionPath("bob.yaml"))

	if _, _, err := RenameNamedSession("ann.yaml", "BOB", running); !errors.Is(err, ErrSessionNameTaken) {
		t.Fatalf("rename onto a taken name: err = %v", err)
	}
	if a, _ := os.ReadFile(NamedSessionPath("ann.yaml")); string(a) != string(annBefore) {
		t.Error("the refused rename changed ann.yaml")
	}
	if b, _ := os.ReadFile(NamedSessionPath("bob.yaml")); string(b) != string(bobBefore) {
		t.Error("the refused rename changed bob.yaml")
	}

	file, moved, err := RenameNamedSession("ann.yaml", "Ann Second", running)
	if err != nil || file != "ann-second.yaml" {
		t.Fatalf("rename: file %q err %v", file, err)
	}
	defer moved.Release()
	if _, err := os.Lstat(NamedSessionPath("ann.yaml")); !os.IsNotExist(err) {
		t.Errorf("the old file is still there (%v)", err)
	}
	b, _ := os.ReadFile(NamedSessionPath("ann-second.yaml"))
	if want := strings.Replace(string(annBefore), "name: Ann\n", "name: Ann Second\n", 1); string(b) != want {
		t.Errorf("renamed file:\n%s\nwant:\n%s", b, want)
	}
	// The lock moved: the new name is held, the old is free.
	if moved.File() != "ann-second.yaml" {
		t.Errorf("the running lock is for %q", moved.File())
	}
	if _, _, _, err := OpenNamedSession("ann-second.yaml", id); !errors.Is(err, ErrSessionOpenElsewhere) {
		t.Errorf("the renamed session is not held by this window: %v", err)
	}
	old, err := lockNamed(SessionsDir(), "ann.yaml")
	if err != nil {
		t.Errorf("the old name's lock is still held: %v", err)
	}
	old.Release()
}

// Trash moves the file to sessions/.trash/<stem>-<UTC stamp>.yaml, byte for
// byte; the session this window runs is refused.
func TestDeleteSessionMovesToTrash(t *testing.T) {
	charts := stateSandbox(t)
	running, err := SaveNamedSessionAs("Running", namedFixture(charts))
	if err != nil {
		t.Fatal(err)
	}
	defer running.Release()
	l, err := SaveNamedSessionAs("Old Work", namedFixture(charts))
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	before, _ := os.ReadFile(NamedSessionPath("old-work.yaml"))

	if _, err := TrashNamedSession("running.yaml", running, time.Now()); !errors.Is(err, ErrSessionRunning) {
		t.Errorf("trashing the running session: err = %v", err)
	}
	if _, err := os.Stat(NamedSessionPath("running.yaml")); err != nil {
		t.Errorf("the running session was moved: %v", err)
	}

	at := time.Date(2026, 10, 26, 23, 4, 5, 0, time.FixedZone("CET", 3600))
	path, err := TrashNamedSession("old-work.yaml", running, at)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(SessionsDir(), ".trash", "old-work-20261026T220405Z.yaml")
	if path != want {
		t.Errorf("trash path %q, want %q", path, want)
	}
	if b, err := os.ReadFile(want); err != nil || string(b) != string(before) {
		t.Errorf("the trashed file is not byte-identical (%v)", err)
	}
	if _, err := os.Lstat(NamedSessionPath("old-work.yaml")); !os.IsNotExist(err) {
		t.Errorf("the session is still in the list's folder (%v)", err)
	}
	if di, _ := os.Stat(filepath.Dir(want)); di == nil || di.Mode().Perm() != 0o700 {
		t.Errorf("trash folder mode %v, want 0700", di)
	}
}

// A link at the sessions folder, or at the sextile folder above it, is
// refused: nothing is written through it and the list reports it.
func TestSessionsDirRefusesSymlinks(t *testing.T) {
	for _, at := range []string{"sessions", "sextile"} {
		t.Run(at, func(t *testing.T) {
			charts := stateSandbox(t)
			elsewhere := t.TempDir()
			data := os.Getenv("XDG_DATA_HOME")
			link := filepath.Join(data, "sextile", "sessions")
			if at == "sextile" {
				link = filepath.Join(data, "sextile")
			}
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(elsewhere, link); err != nil {
				t.Fatal(err)
			}
			if _, err := SaveNamedSessionAs("Ann", namedFixture(charts)); err == nil {
				t.Error("save as wrote through a link")
			}
			if _, err := ListNamedSessions(FolderID(charts), ""); err == nil {
				t.Error("the list read through a link")
			}
			if names := dirNames(t, elsewhere); len(names) != 0 {
				t.Errorf("files were made through the link: %v", names)
			}
		})
	}
}

// yamlFiles is the .yaml files of the sessions folder and their bytes.
func yamlFiles(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, n := range dirNames(t, SessionsDir()) {
		if strings.HasSuffix(n, ".yaml") {
			b, _ := os.ReadFile(filepath.Join(SessionsDir(), n))
			out[n] = string(b)
		}
	}
	return out
}

// Where the filesystem cannot lock, a named session could have two writers:
// opening, save as, rename and trash are refused, and no session file is
// written.
func TestNamedSessionLockUnsupportedIsRefused(t *testing.T) {
	charts := stateSandbox(t)
	l, err := SaveNamedSessionAs("Ann", namedFixture(charts))
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	before := yamlFiles(t)
	locksBefore := lockFiles(t)
	prev := flock
	flock = func(int, int) error { return syscall.ENOTSUP }
	defer func() { flock = prev }()

	if _, _, lock, err := OpenNamedSession("ann.yaml", FolderID(charts)); !errors.Is(err, ErrSessionLockUnsupported) || lock != nil {
		t.Errorf("open: lock %v err %v, want ErrSessionLockUnsupported", lock, err)
	}
	if lock, err := SaveNamedSessionAs("Bob", namedFixture(charts)); !errors.Is(err, ErrSessionLockUnsupported) || lock != nil {
		t.Errorf("save as: lock %v err %v", lock, err)
	}
	if _, _, err := RenameNamedSession("ann.yaml", "Cat", nil); !errors.Is(err, ErrSessionLockUnsupported) {
		t.Errorf("rename: err %v", err)
	}
	if _, err := TrashNamedSession("ann.yaml", nil, time.Now()); !errors.Is(err, ErrSessionLockUnsupported) {
		t.Errorf("trash: err %v", err)
	}
	if after := yamlFiles(t); !maps.Equal(after, before) {
		t.Errorf("session files changed: %v, was %v", after, before)
	}
	if _, err := ListNamedSessions(FolderID(charts), ""); err != nil {
		t.Errorf("list: %v", err)
	}
	if after := lockFiles(t); !slices.Equal(after, locksBefore) {
		t.Errorf("lock files left behind: %v, was %v", after, locksBefore)
	}
	if strings.Contains(ErrSessionLockUnsupported.Error(), "\n") {
		t.Error("the refusal is not one sentence")
	}
}

// A named session holding a block or a floating key this build does not know
// is refused untouched, since a save would regenerate the file without it; a
// session of known keys only still opens.
func TestNamedSessionWithUnknownContentIsRefused(t *testing.T) {
	charts := stateSandbox(t)
	l, err := SaveNamedSessionAs("Known", namedFixture(charts))
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	known, _ := os.ReadFile(NamedSessionPath("known.yaml"))
	cases := map[string]string{
		"block.yaml":    string(known) + "client_assignment:\n  client: someone\n",
		"floating.yaml": strings.Replace(string(known), "floating:\n", "floating:\n  pinned: true\n", 1),
		"comment.yaml":  string(known) + "# kept by hand\n",
	}
	for file, text := range cases {
		writeFile(t, NamedSessionPath(file), text)
		if _, _, lock, err := OpenNamedSession(file, FolderID(charts)); !errors.Is(err, ErrSessionUnknownContent) || lock != nil {
			t.Errorf("%s: err %v, want ErrSessionUnknownContent", file, err)
		}
		if _, _, err := LoadNamedSession(file); !errors.Is(err, ErrSessionUnknownContent) {
			t.Errorf("%s: load err %v", file, err)
		}
		if b, _ := os.ReadFile(NamedSessionPath(file)); string(b) != text {
			t.Errorf("%s changed", file)
		}
	}
	_, _, lock, err := OpenNamedSession("known.yaml", FolderID(charts))
	if err != nil {
		t.Fatalf("a known-only session was refused: %v", err)
	}
	lock.Release()
}

// Rename first reads the session as open does: a malformed one, or one with
// unknown content, is refused with its bytes and name unchanged.
func TestRenameRefusesAnUnreadableSession(t *testing.T) {
	charts := stateSandbox(t)
	id := FolderID(charts)
	for file, text := range map[string]string{
		"broken.yaml":  "version: 3\nname: Broken\nfolder: " + id + "\ncharts:\n  - id: x\n    name: Half\nfloating:\n  selected: false\n",
		"unknown.yaml": "version: 3\nname: Unknown\nfolder: " + id + "\nfloating:\n  selected: false\nclient_assignment:\n  client: x\n",
	} {
		writeFile(t, NamedSessionPath(file), text)
		if _, _, err := RenameNamedSession(file, "Renamed "+file[:3], nil); err == nil {
			t.Errorf("%s was renamed", file)
		}
		if b, _ := os.ReadFile(NamedSessionPath(file)); string(b) != text {
			t.Errorf("%s changed:\n%s", file, b)
		}
	}
	for _, n := range dirNames(t, SessionsDir()) {
		if strings.HasPrefix(n, "renamed") && strings.HasSuffix(n, ".yaml") {
			t.Errorf("a renamed file was made: %s", n)
		}
	}
}

// lockFiles is the .lock files of the sessions folder.
func lockFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, n := range dirNames(t, SessionsDir()) {
		if strings.HasSuffix(n, ".lock") {
			out = append(out, n)
		}
	}
	return out
}

// Any line the writer would not produce -- an indented comment or key outside
// floating: included -- makes the session refused untouched on open, on
// --session and on rename.
func TestNamedSessionStrictLoaderRefusesIndentedContent(t *testing.T) {
	charts := stateSandbox(t)
	id := FolderID(charts)
	l, err := SaveNamedSessionAs("Known", namedFixture(charts))
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	known, _ := os.ReadFile(NamedSessionPath("known.yaml"))
	for i, extra := range []string{"  # client note\n", "  future_key: value\n"} {
		file := fmt.Sprintf("odd-%d.yaml", i)
		text := strings.Replace(strings.Replace(string(known), "name: Known\n", fmt.Sprintf("name: Odd %d\n", i), 1), "version: 3\n", "version: 3\n"+extra, 1)
		writeFile(t, NamedSessionPath(file), text)
		if _, _, lock, err := OpenNamedSession(file, id); err == nil {
			lock.Release()
			t.Errorf("%q: opened", extra)
		}
		if _, _, lock, err := OpenNamedSessionByName(fmt.Sprintf("odd %d", i), id); err == nil {
			lock.Release()
			t.Errorf("%q: opened by --session", extra)
		}
		if _, _, err := RenameNamedSession(file, fmt.Sprintf("Other %d", i), nil); err == nil {
			t.Errorf("%q: renamed", extra)
		}
		if b, _ := os.ReadFile(NamedSessionPath(file)); string(b) != text {
			t.Errorf("%q: the file changed", extra)
		}
	}
	if _, _, lock, err := OpenNamedSession("known.yaml", id); err != nil {
		t.Errorf("the writer's own file was refused: %v", err)
	} else {
		lock.Release()
	}
}

// A running session whose file was changed outside Sextile is not written
// over; the window's own writes keep working.
func TestSaveNamedSessionRefusesAnExternalEdit(t *testing.T) {
	charts := stateSandbox(t)
	lock, err := SaveNamedSessionAs("Ann", namedFixture(charts))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	s := namedFixture(charts)
	for i := 0; i < 2; i++ {
		s.FloatingSelected = i == 0
		if err := SaveNamedSession(lock, "Ann", s); err != nil {
			t.Fatalf("own save %d: %v", i, err)
		}
	}
	edited := fileString(t, NamedSessionPath("ann.yaml")) + "client_assignment:\n  client: someone\n"
	writeFile(t, NamedSessionPath("ann.yaml"), edited)
	if err := SaveNamedSession(lock, "Ann", s); !errors.Is(err, ErrSessionChangedOutside) {
		t.Errorf("save over an edited file: err %v, want ErrSessionChangedOutside", err)
	}
	if got := fileString(t, NamedSessionPath("ann.yaml")); got != edited {
		t.Errorf("the edited file was written over:\n%s", got)
	}
}

// A released lock writes nothing.
func TestSaveNamedSessionRefusesAReleasedLock(t *testing.T) {
	charts := stateSandbox(t)
	lock, err := SaveNamedSessionAs("Ann", namedFixture(charts))
	if err != nil {
		t.Fatal(err)
	}
	before := fileString(t, NamedSessionPath("ann.yaml"))
	lock.Release()
	s := namedFixture(charts)
	s.FloatingSelected = false
	if err := SaveNamedSession(lock, "Ann", s); err == nil {
		t.Error("a released lock saved")
	}
	if got := fileString(t, NamedSessionPath("ann.yaml")); got != before {
		t.Errorf("the file changed:\n%s", got)
	}
}

func fileString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
