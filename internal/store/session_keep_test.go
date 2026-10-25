package store

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// All chart values here are constructed synthetic test inputs.

var keptName = regexp.MustCompile(`^session\.yaml\.refused-[0-9]{8}T[0-9]{6}Z(-[0-9]+)?$`)

func writeSessionText(t *testing.T, dir, text string, mode os.FileMode) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "session.yaml")
	if err := os.WriteFile(path, []byte(text), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func refusedFiles(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, "session.yaml.refused-*"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func freezeKeepClock(t *testing.T, at time.Time) {
	t.Helper()
	old := keepClock
	keepClock = func() time.Time { return at }
	t.Cleanup(func() { keepClock = old })
}

func setSeam[T any](t *testing.T, p *T, v T) {
	t.Helper()
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

func setKeepHook(t *testing.T, f func()) {
	t.Helper()
	old := keepHook
	keepHook = f
	t.Cleanup(func() { keepHook = old })
}

// refusedCases are the files C1 names: a read error, and a file the reader
// accepted but dropped an entry from.
func refusedCases() map[string]string {
	good := chartsText(SessionSlot{Chart: strictChart(1)}, SessionSlot{Chart: strictChart(2)})
	rejected := strings.Replace(good, "    lat: 12\n", "    lat: not-a-number\n", 1)
	long := strictChart(3)
	long.Name = strings.Repeat("N", sessionMaxLine+1024)
	return map[string]string{
		"a later version":                 "version: 4\ncharts:\n  - id: x\n",
		"no version line":                 "charts:\n  - id: x\n",
		"an empty file":                   "",
		"an overlong line":                chartsText(SessionSlot{Chart: long}),
		"one rejected chart entry":        rejected,
		"a version 1 chart it cannot use": "version: 1\nchart:\n  id: x\n  name: \n  when: \"2001-02-03T04:05:00Z\"\n  place: P\n  lat: 1\n  lon: 2\n  tz: UTC\nselection:\n  comparison: false\n",
	}
}

// C1: a refused file is renamed at the launch's own read, mode unchanged.
func TestRefusedSessionIsKept(t *testing.T) {
	for name, text := range refusedCases() {
		t.Run(name, func(t *testing.T) {
			dir := sessionSandbox(t)
			path := writeSessionText(t, dir, text, 0o640)
			s, _, keep, err := LoadSessionKeepingRefused()
			if name == "one rejected chart entry" || name == "a version 1 chart it cannot use" {
				if err != nil {
					t.Fatalf("err = %v, want the file read with a dropped entry", err)
				}
				if name == "one rejected chart entry" && usable(s) != 1 {
					t.Errorf("usable slots = %d, want the valid chart still returned", usable(s))
				}
			} else if err == nil {
				t.Fatal("the refused file came back with no error")
			}
			if keep.Problem != "" || !keptName.MatchString(keep.Kept) {
				t.Fatalf("keep = %+v, want a kept name", keep)
			}
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				t.Errorf("session.yaml still exists after the keep: %v", err)
			}
			copy := filepath.Join(dir, keep.Kept)
			got, err := os.ReadFile(copy)
			if err != nil || string(got) != text {
				t.Errorf("kept copy differs from the refused bytes (err %v, %d vs %d bytes)", err, len(got), len(text))
			}
			if fi, err := os.Stat(copy); err != nil || fi.Mode().Perm() != 0o640 {
				t.Errorf("kept copy mode = %v (%v), want 0640 unchanged", fi.Mode().Perm(), err)
			}
		})
	}
}

func usable(s Session) int {
	n := 0
	for _, sl := range s.Slots {
		if sl.Usable {
			n++
		}
	}
	return n
}

// C2: an accepted file, or none, makes no copy.
func TestAcceptedSessionMakesNoCopy(t *testing.T) {
	v1 := "version: 1\nchart:\n  id: x\n  name: X\n  when: \"2001-02-03T04:05:00Z\"\n  place: P\n  lat: 1\n  lon: 2\n  tz: UTC\nselection:\n  comparison: false\n"
	cases := map[string]*string{
		"no file":                nil,
		"version 2":              ptr(chartsText(SessionSlot{Chart: strictChart(1)})),
		"version 1":              &v1,
		"unknown trailing block": ptr(chartsText(SessionSlot{Chart: strictChart(1)}) + "extras:\n  - nonsense\n"),
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			dir := sessionSandbox(t)
			if text != nil {
				writeSessionText(t, dir, *text, 0o600)
			}
			_, _, keep, err := LoadSessionKeepingRefused()
			if err != nil || keep != (SessionKeep{}) {
				t.Errorf("err = %v, keep = %+v; want a plain accepted read", err, keep)
			}
			if got := refusedFiles(t, dir); len(got) != 0 {
				t.Errorf("copies appeared for an accepted file: %v", got)
			}
			if text != nil {
				if b, err := os.ReadFile(filepath.Join(dir, "session.yaml")); err != nil || string(b) != *text {
					t.Errorf("the accepted file was touched: %v", err)
				}
			}
			// The quit: writing the session leaves no copy, and replaces the file.
			if err := SaveSession(Session{FloatingSelected: true}); err != nil {
				t.Fatal(err)
			}
			if got := refusedFiles(t, dir); len(got) != 0 {
				t.Errorf("copies appeared after the quit: %v", got)
			}
			if b, err := os.ReadFile(filepath.Join(dir, "session.yaml")); err != nil || string(b) != formatSession(Session{FloatingSelected: true}) {
				t.Errorf("session.yaml after the quit = %q (%v), want the saved text", b, err)
			}
		})
	}
}

func ptr(s string) *string { return &s }

// C5: refused launches in a row keep distinct copies, even within one second.
func TestRefusedSessionCopiesDoNotCollide(t *testing.T) {
	dir := sessionSandbox(t)
	freezeKeepClock(t, time.Date(2026, 10, 8, 12, 30, 45, 0, time.UTC))
	seen := map[string]string{}
	for i, text := range []string{"version: 4\na\n", "version: 5\nb\n", "version: 6\nc\n"} {
		writeSessionText(t, dir, text, 0o600)
		_, _, keep, _ := LoadSessionKeepingRefused()
		if keep.Kept == "" {
			t.Fatalf("launch %d kept nothing: %+v", i, keep)
		}
		if _, dup := seen[keep.Kept]; dup {
			t.Fatalf("launch %d reused the name %s", i, keep.Kept)
		}
		seen[keep.Kept] = text
	}
	if _, ok := seen["session.yaml.refused-20261008T123045Z"]; !ok {
		t.Errorf("first name is not the UTC stamp: %v", seen)
	}
	for name, text := range seen {
		if b, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(b) != text {
			t.Errorf("%s no longer holds its refused bytes (%v)", name, err)
		}
	}
}

// pass 5: an existing .refused-* name (file, symlink or directory) is skipped.
func TestRefusedBackupDestinationIsNeverReplaced(t *testing.T) {
	dir := sessionSandbox(t)
	freezeKeepClock(t, time.Date(2026, 10, 8, 12, 30, 45, 0, time.UTC))
	writeSessionText(t, dir, "version: 4\nnew\n", 0o600)
	base := filepath.Join(dir, "session.yaml.refused-20261008T123045Z")
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base, []byte("old file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, base+"-2"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "dangling"), base+"-3"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(base+"-4", 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, keep, _ := LoadSessionKeepingRefused()
	if keep.Kept != "session.yaml.refused-20261008T123045Z-5" {
		t.Fatalf("keep = %+v, want the first free suffix", keep)
	}
	if b, _ := os.ReadFile(base); string(b) != "old file" {
		t.Errorf("the existing file was replaced: %q", b)
	}
	if b, _ := os.ReadFile(outside); string(b) != "outside" {
		t.Errorf("the symlink target was written through: %q", b)
	}
	if _, err := os.Lstat(filepath.Join(dir, "dangling")); !os.IsNotExist(err) {
		t.Errorf("the dangling symlink target was created: %v", err)
	}
	if fi, err := os.Lstat(base + "-4"); err != nil || !fi.IsDir() {
		t.Errorf("the directory was replaced: %v", err)
	}
}

// pass 3: bytes changed between the refused read and the rename.
func TestRefusedSessionChangedBeforeRenameIsLeftAlone(t *testing.T) {
	dir := sessionSandbox(t)
	path := writeSessionText(t, dir, "version: 4\nbefore\n", 0o600)
	setKeepHook(t, func() {
		if err := os.WriteFile(path, []byte("version: 2\nfloating:\n  selected: true\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	_, _, keep, _ := LoadSessionKeepingRefused()
	if keep.Kept != "" || keep.Problem == "" {
		t.Fatalf("keep = %+v, want a reported conflict and no copy", keep)
	}
	if b, _ := os.ReadFile(path); string(b) != "version: 2\nfloating:\n  selected: true\n" {
		t.Errorf("the changed file was touched: %q", b)
	}
	if got := refusedFiles(t, dir); len(got) != 0 {
		t.Errorf("copies appeared: %v", got)
	}
}

// pass 7: swapped to a symlink after the refused read.
func TestRefusedSessionSwappedToSymlinkIsLeftAlone(t *testing.T) {
	dir := sessionSandbox(t)
	path := writeSessionText(t, dir, "version: 4\nbefore\n", 0o600)
	target := filepath.Join(t.TempDir(), "target.yaml")
	if err := os.WriteFile(target, []byte("version: 4\nbefore\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	setKeepHook(t, func() {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	})
	_, _, keep, _ := LoadSessionKeepingRefused()
	if keep.Kept != "" || keep.Problem == "" {
		t.Fatalf("keep = %+v, want a reported conflict and no copy", keep)
	}
	if l, err := os.Readlink(path); err != nil || l != target {
		t.Errorf("the link changed: %q %v", l, err)
	}
	if b, _ := os.ReadFile(target); string(b) != "version: 4\nbefore\n" {
		t.Errorf("the target changed: %q", b)
	}
	if got := refusedFiles(t, dir); len(got) != 0 {
		t.Errorf("copies appeared: %v", got)
	}
}

// pass 6: the kept copy keeps its mode and the new session is written 0600.
func TestBackupLeavesNewSessionPrivate(t *testing.T) {
	dir := sessionSandbox(t)
	writeSessionText(t, dir, "version: 4\nx\n", 0o644)
	_, _, keep, _ := LoadSessionKeepingRefused()
	if keep.Kept == "" {
		t.Fatalf("keep = %+v", keep)
	}
	if err := SaveSession(Session{FloatingSelected: true}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Join(dir, keep.Kept)); err != nil || fi.Mode().Perm() != 0o644 {
		t.Errorf("kept copy mode = %v (%v), want 0644", fi.Mode().Perm(), err)
	}
	if fi, err := os.Stat(filepath.Join(dir, "session.yaml")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("new session mode = %v (%v), want 0600", fi.Mode().Perm(), err)
	}
}

// F1: a different file swapped into session.yaml while the move is under way is
// never deleted; the judged bytes are in the kept copy.
func TestRefusedSessionSwappedDuringMoveIsNeverDeleted(t *testing.T) {
	dir := sessionSandbox(t)
	path := writeSessionText(t, dir, "version: 4\njudged\n", 0o600)
	const other = "version: 2\nfloating:\n  selected: true\n"
	setSeam(t, &keepAfterLink, func() {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(other), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	_, _, keep, _ := LoadSessionKeepingRefused()
	if keep.Problem == "" || keep.Kept == "" {
		t.Fatalf("keep = %+v, want the kept copy named and the conflict reported", keep)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != other {
		t.Errorf("the swapped-in file was lost: %q %v", b, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, keep.Kept)); string(b) != "version: 4\njudged\n" {
		t.Errorf("kept copy = %q, want the judged bytes", b)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "session.yaml.keep-*")); len(left) != 0 {
		t.Errorf("placeholders left behind: %v", left)
	}
}

// F2: a path that cannot be checked suppresses writes.
func TestUncheckableSessionSuppressesWrites(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: mode bits do not deny the stat")
	}
	dir := sessionSandbox(t)
	writeSessionText(t, dir, "version: 4\nx\n", 0o600)
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	_, _, keep, _ := LoadSessionKeepingRefused()
	if keep.Problem != "it could not be checked" || keep.Kept != "" {
		t.Errorf("keep = %+v, want the could-not-be-checked problem", keep)
	}
}

// F3: a symlink is left alone whether it dangles or its target is acceptable.
func symlinkLeftAlone(t *testing.T, targetText *string) {
	dir := sessionSandbox(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target.yaml")
	if targetText != nil {
		if err := os.WriteFile(target, []byte(*targetText), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(dir, "session.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	_, _, keep, _ := LoadSessionKeepingRefused()
	if keep.Problem != "it is a symlink" || keep.Kept != "" {
		t.Errorf("keep = %+v, want the symlink problem", keep)
	}
	if l, err := os.Readlink(link); err != nil || l != target {
		t.Errorf("the link changed: %q %v", l, err)
	}
	if targetText != nil {
		if b, _ := os.ReadFile(target); string(b) != *targetText {
			t.Errorf("the target changed: %q", b)
		}
	}
	if got := refusedFiles(t, dir); len(got) != 0 {
		t.Errorf("copies appeared: %v", got)
	}
}

func TestDanglingSessionSymlinkIsLeftAlone(t *testing.T) { symlinkLeftAlone(t, nil) }

func TestAcceptedSessionSymlinkIsLeftAlone(t *testing.T) {
	symlinkLeftAlone(t, ptr(chartsText(SessionSlot{Chart: strictChart(1)})))
}

// F4: once the kept name exists it is reported, even if the move then fails.
func TestKeptCopyIsNamedEvenWhenTheMoveFails(t *testing.T) {
	dir := sessionSandbox(t)
	path := writeSessionText(t, dir, "version: 4\nx\n", 0o600)
	setSeam(t, &keepRename, func(string, string) error { return os.ErrPermission })
	_, _, keep, _ := LoadSessionKeepingRefused()
	if keep.Kept == "" || keep.Problem == "" {
		t.Fatalf("keep = %+v, want both the kept name and the problem", keep)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, keep.Kept)); string(b) != "version: 4\nx\n" {
		t.Errorf("kept copy = %q", b)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "version: 4\nx\n" {
		t.Errorf("the original was disturbed: %q %v", b, err)
	}
}
