// All chart values in this file are constructed synthetic test inputs.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

// stateSandbox gives one test its own XDG bases and a charts folder, and
// returns the charts folder.
func stateSandbox(t *testing.T) (charts string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("SEXTILE_LAUNCH", "")
	t.Setenv("SEXTILE_CHARTS_HOST_PATH", "")
	t.Setenv("SEXTILE_BACKUP_HOST_PATH", "")
	charts = filepath.Join(root, "charts")
	if err := os.MkdirAll(charts, 0o755); err != nil {
		t.Fatal(err)
	}
	return charts
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// FolderID is the full sha256 of the physical path: 64 lowercase hex, one ID
// for a folder reached through a link, and the launcher's host path only
// inside the launcher's contract.
func TestFolderIDIsTheFullDigestOfThePhysicalPath(t *testing.T) {
	charts := stateSandbox(t)
	t.Cleanup(OverrideContainerProbe(func() bool { return true }))
	id := FolderID(charts)
	if !validFolderID(id) {
		t.Fatalf("FolderID = %q, want 64 lowercase hex", id)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(charts, alias); err != nil {
		t.Fatal(err)
	}
	if got := FolderID(alias); got != id {
		t.Errorf("the same folder through a link has ID %s, want %s", got, id)
	}
	if got := FolderID(t.TempDir()); got == id {
		t.Error("two folders share one ID")
	}

	host := "/data/operator/Clients"
	t.Setenv("SEXTILE_CHARTS_HOST_PATH", host)
	if got := FolderID(charts); got != id {
		t.Errorf("SEXTILE_CHARTS_HOST_PATH steered the ID without SEXTILE_LAUNCH")
	}
	t.Setenv("SEXTILE_LAUNCH", "docker sextile:test")
	if got, want := FolderID("/charts"), folderHash(host); got != want {
		t.Errorf("inside the launcher's contract FolderID = %s, want the host path's %s", got, want)
	}
	t.Setenv("SEXTILE_CHARTS_HOST_PATH", "relative/path")
	if got := FolderID(charts); got != id {
		t.Errorf("a relative SEXTILE_CHARTS_HOST_PATH was honoured")
	}
}

// Two folders holding a chart of the same name keep two backups, in two
// folders, outside both charts folders.
func TestTwoFoldersKeepTwoBackups(t *testing.T) {
	stateSandbox(t)
	a, b := t.TempDir(), t.TempDir()
	for _, dir := range []string{a, b} {
		seedChart(t, dir, "Client.as", retitleBefore+"; "+dir+"\n")
		if _, err := ReplaceChartWithBackup(dir, "Client.as", tempHolding(t, dir, retitleFirst), []byte(retitleBefore+"; "+dir+"\n")); err != nil {
			t.Fatalf("retitle in %s: %v", dir, err)
		}
	}
	if BackupDir(a) == BackupDir(b) {
		t.Fatalf("two folders share the backup folder %s", BackupDir(a))
	}
	for _, dir := range []string{a, b} {
		if got := readString(t, filepath.Join(BackupDir(dir), "Client.as")); got != retitleBefore+"; "+dir+"\n" {
			t.Errorf("%s's backup holds %q, want its own original", dir, got)
		}
		onlyEntries(t, dir, "Client.as")
	}
}

// A backup folder is named by the full 64-hex FolderID of the charts folder's
// physical path, and holds nothing but backups: no file anywhere under the
// data base names the charts folder in plain text.
func TestBackupFolderIsTheFullDigestAndHoldsOnlyBackups(t *testing.T) {
	stateSandbox(t)
	dir := t.TempDir()
	seedChart(t, dir, "Client.as", retitleBefore)
	if _, err := ReplaceChartWithBackup(dir, "Client.as", tempHolding(t, dir, retitleFirst), []byte(retitleBefore)); err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(physical))
	if got, want := filepath.Base(BackupDir(dir)), hex.EncodeToString(sum[:]); got != want {
		t.Errorf("backup folder = %s, want the full digest %s", got, want)
	}
	entries, err := os.ReadDir(BackupDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "Client.as" {
		t.Errorf("the backup folder holds %q, want only Client.as", names)
	}
	filepath.Walk(os.Getenv("XDG_DATA_HOME"), func(p string, fi os.FileInfo, err error) error {
		if err == nil && fi.Mode().IsRegular() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), physical) {
				t.Errorf("%s names the charts folder in plain text", p)
			}
		}
		return nil
	})
}

// seedLegacy writes 0.1.1's state into charts/.sextile and returns the three
// source paths and their bytes.
func seedLegacy(t *testing.T, charts, tag string) map[string]string {
	t.Helper()
	files := map[string]string{
		filepath.Join(charts, ".sextile", "data", "sextile", "session", "session.yaml"): "version: 3\nfloating:\n  selected: false\n# " + tag + "\n",
		filepath.Join(charts, ".sextile", "config", "sextile", "preferences.yaml"):      "version: 1\n# " + tag + "\n",
		filepath.Join(charts, ".sextile", "backup", "Client.as"):                        retitleBefore + "; " + tag + "\n",
		filepath.Join(charts, ".sextile", "backup", "Other.as"):                         retitleBefore + "; other " + tag + "\n",
		filepath.Join(charts, ".sextile", "data", "sextile", "session", "sextile.lock"): "4242\n",
	}
	for p, body := range files {
		writeFile(t, p, body)
	}
	return files
}

// The one-time copy: the session (stamped with this folder), the preferences
// and each backup land in their new places, private; the legacy folder is
// byte-identical afterwards; a second run copies nothing and says nothing;
// the lock is never copied and the charts folder gains nothing.
func TestMigrationCopiesLegacyStateOnce(t *testing.T) {
	charts := stateSandbox(t)
	seedLegacy(t, charts, "A")
	before := treeOf(t, charts)

	r := MigrateLegacyState(charts)
	if got := strings.Join(r.Copied, ","); got != "session,preferences,2 backups" || len(r.Already)+len(r.Refused) != 0 {
		t.Fatalf("result = %+v", r)
	}
	if !strings.Contains(r.Line(), "left as they are") {
		t.Errorf("the Log line does not say the old files were left: %q", r.Line())
	}
	sess := readString(t, SessionPath())
	if want := "version: 3\nfolder: " + FolderID(charts) + "\nfloating:\n"; !strings.HasPrefix(sess, want) {
		t.Errorf("migrated session = %q, want it to start %q", sess, want)
	}
	if s, ok, err := LoadSession(); err != nil || !ok || s.Folder != FolderID(charts) {
		t.Errorf("LoadSession = folder %q ok %v err %v", s.Folder, ok, err)
	}
	if got := readString(t, PreferencesPath()); got != "version: 1\n# A\n" {
		t.Errorf("preferences = %q", got)
	}
	if got := readString(t, filepath.Join(BackupDir(charts), "Client.as")); got != retitleBefore+"; A\n" {
		t.Errorf("backup = %q", got)
	}
	for _, p := range []string{SessionPath(), PreferencesPath(), filepath.Join(BackupDir(charts), "Client.as")} {
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("%s: mode %v err %v, want 0600", p, fi.Mode().Perm(), err)
		}
	}
	for _, d := range []string{SessionDir(), BackupDir(charts)} {
		if fi, err := os.Stat(d); err != nil || fi.Mode().Perm() != 0o700 {
			t.Errorf("%s: mode %v err %v, want 0700", d, fi.Mode().Perm(), err)
		}
	}
	if _, err := os.Lstat(filepath.Join(SessionDir(), "sextile.lock")); !os.IsNotExist(err) {
		t.Errorf("the legacy lock was copied (stat err %v)", err)
	}
	if got := treeOf(t, charts); got != before {
		t.Errorf("the charts folder changed:\nbefore %s\nafter  %s", before, got)
	}

	again := MigrateLegacyState(charts)
	if len(again.Copied)+len(again.Refused) != 0 || again.Line() != "" {
		t.Errorf("a second run did something: %+v", again)
	}
	if got := treeOf(t, charts); got != before {
		t.Errorf("the second run changed the charts folder")
	}
}

// A destination that already holds a file is never replaced: the copy is
// reported as already there.
func TestMigrationNeverReplacesAnExistingTarget(t *testing.T) {
	charts := stateSandbox(t)
	seedLegacy(t, charts, "A")
	writeFile(t, SessionPath(), "version: 3\n# the current session\n")
	writeFile(t, PreferencesPath(), "version: 1\n# current preferences\n")

	r := MigrateLegacyState(charts)
	if got := strings.Join(r.Already, ","); got != "session,preferences" {
		t.Errorf("already = %q, want session,preferences (result %+v)", got, r)
	}
	if got := readString(t, SessionPath()); got != "version: 3\n# the current session\n" {
		t.Errorf("the existing session was replaced: %q", got)
	}
	if got := readString(t, PreferencesPath()); got != "version: 1\n# current preferences\n" {
		t.Errorf("the existing preferences were replaced: %q", got)
	}
	if !strings.Contains(r.Line(), "already there: session, preferences") {
		t.Errorf("the Log line does not say what was already there: %q", r.Line())
	}
}

// The hand-run session of a build before 0.1.2 ($XDG_DATA_HOME/sextile/session)
// is copied too, and belongs to the folder it is opened in.
func TestMigrationCopiesTheHandRunSession(t *testing.T) {
	charts := stateSandbox(t)
	writeFile(t, legacySessionPath(), "version: 3\nfloating:\n  selected: true\n")
	r := MigrateLegacyState(charts)
	if got := strings.Join(r.Copied, ","); got != "session (hand-run)" {
		t.Fatalf("result = %+v", r)
	}
	if s, ok, err := LoadSession(); err != nil || !ok || s.Folder != FolderID(charts) || !s.FloatingSelected {
		t.Errorf("LoadSession = %+v ok %v err %v", s, ok, err)
	}
	if got := readString(t, legacySessionPath()); got != "version: 3\nfloating:\n  selected: true\n" {
		t.Errorf("the hand-run source changed: %q", got)
	}
	// It belongs to no one folder: a launch on another folder does not hear
	// of it again.
	if other := MigrateLegacyState(t.TempDir()); len(other.Copied) != 0 || other.Line() != "" {
		t.Errorf("another folder's launch copied or reported the hand-run session again: %+v", other)
	}
}

// Two launches migrating two legacy folders at once, each paused AFTER it
// found the session's place empty: the exclusive publish, not the check,
// decides. Exactly one session wins whole, the other launch reports it as
// already there, and nothing is replaced. Run with a real lock and with the
// lock unsupported, where nothing serialises the two.
func TestConcurrentMigrationsPublishExactlyOneSession(t *testing.T) {
	for _, mode := range []string{"lock unsupported", "no lock taken"} {
		t.Run(mode, func(t *testing.T) {
			stateSandbox(t)
			a, b := t.TempDir(), t.TempDir()
			srcA, srcB := seedLegacy(t, a, "A"), seedLegacy(t, b, "B")
			_ = srcB
			if mode == "lock unsupported" {
				prev := flock
				flock = func(int, int) error { return syscall.ENOTSUP }
				t.Cleanup(func() { flock = prev })
				for range 2 {
					l, err := AcquireInstanceLock()
					if err != nil || !l.Unsupported {
						t.Fatalf("premise: lock = %+v, %v; want unsupported", l, err)
					}
				}
			}

			var arrived sync.WaitGroup
			arrived.Add(2)
			release := make(chan struct{})
			prev := afterAbsenceCheck
			afterAbsenceCheck = func(target string) {
				if target == SessionPath() {
					arrived.Done()
					<-release
				}
			}
			t.Cleanup(func() { afterAbsenceCheck = prev })

			results := make([]MigrationResult, 2)
			var done sync.WaitGroup
			for i, dir := range []string{a, b} {
				done.Add(1)
				go func() {
					defer done.Done()
					results[i] = MigrateLegacyState(dir)
				}()
			}
			arrived.Wait() // both found the session's place empty
			close(release)
			done.Wait()

			won := 0
			for _, r := range results {
				if strings.Contains(strings.Join(r.Copied, ","), "session") {
					won++
				} else if !strings.Contains(strings.Join(r.Already, ","), "session") {
					t.Errorf("a loser did not report the session as already there: %+v", r)
				}
			}
			if won != 1 {
				t.Fatalf("%d sessions were published, want exactly 1 (%+v)", won, results)
			}
			got := readString(t, SessionPath())
			legacyA := srcA[filepath.Join(a, ".sextile", "data", "sextile", "session", "session.yaml")]
			wantA := string(stampFolder([]byte(legacyA), FolderID(a)))
			wantB := string(stampFolder([]byte(strings.Replace(legacyA, "# A", "# B", 1)), FolderID(b)))
			winner := 0
			if !strings.Contains(strings.Join(results[0].Copied, ","), "session") {
				winner = 1
			}
			if want := []string{wantA, wantB}[winner]; got != want {
				t.Errorf("the session holds %q, want the winner's whole copy %q", got, want)
			}
			if tmps, _ := filepath.Glob(filepath.Join(SessionDir(), ".*")); len(tmps) != 0 {
				t.Errorf("temp files left in the session folder: %q", tmps)
			}
		})
	}
}

// A symlink at any destination folder, or at the destination file, refuses
// that copy plainly; nothing outside is created, read into or replaced.
func TestMigrationRefusesLinkedDestinations(t *testing.T) {
	for _, linked := range []string{"state/sextile", "state/sextile/session", "session.yaml", "config/sextile", "data/sextile/backup", "data/sextile/backup/<id>"} {
		t.Run(linked, func(t *testing.T) {
			charts := stateSandbox(t)
			seedLegacy(t, charts, "A")
			outside := t.TempDir()
			writeFile(t, filepath.Join(outside, "keep"), "somebody else's\n")
			var link, label string
			switch linked {
			case "state/sextile":
				link, label = filepath.Join(os.Getenv("XDG_STATE_HOME"), "sextile"), "session"
			case "state/sextile/session":
				link, label = SessionDir(), "session"
			case "session.yaml":
				link, label = SessionPath(), "session"
				// a link to a file outside, which must stay as it is
				if err := os.MkdirAll(SessionDir(), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(outside, "keep"), link); err != nil {
					t.Fatal(err)
				}
			case "config/sextile":
				link, label = filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "sextile"), "preferences"
			case "data/sextile/backup":
				link, label = filepath.Join(os.Getenv("XDG_DATA_HOME"), "sextile", "backup"), "2 backups"
			case "data/sextile/backup/<id>":
				link, label = BackupDir(charts), "2 backups"
			}
			if linked != "session.yaml" {
				if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, link); err != nil {
					t.Fatal(err)
				}
			}
			before := treeOf(t, outside)

			r := MigrateLegacyState(charts)
			if !strings.Contains(strings.Join(r.Refused, ","), label) {
				t.Errorf("refused = %q, want %q among them (result %+v)", r.Refused, label, r)
			}
			if strings.Contains(strings.Join(r.Copied, ","), label) {
				t.Errorf("%s was copied through a link: %+v", label, r)
			}
			if got := treeOf(t, outside); got != before {
				t.Errorf("the folder behind the link changed:\nbefore %s\nafter  %s", before, got)
			}
			if !strings.Contains(r.Line(), "not copied, a link") {
				t.Errorf("the Log line does not say what was refused: %q", r.Line())
			}
		})
	}
}

// A legacy source reached through a link, or one that is not a regular file,
// is not read: refused, and nothing is copied from it.
func TestMigrationRefusesLinkedSources(t *testing.T) {
	charts := stateSandbox(t)
	elsewhere := t.TempDir()
	seedLegacy(t, elsewhere, "X")
	if err := os.Symlink(filepath.Join(elsewhere, ".sextile"), filepath.Join(charts, ".sextile")); err != nil {
		t.Fatal(err)
	}
	r := MigrateLegacyState(charts)
	if len(r.Copied) != 0 {
		t.Errorf("copied through a linked .sextile: %+v", r)
	}
	if len(r.Refused) == 0 {
		t.Errorf("a linked .sextile was not reported: %+v", r)
	}
	if _, err := os.Lstat(SessionPath()); !os.IsNotExist(err) {
		t.Errorf("a session was written from a linked source (stat err %v)", err)
	}
}

// The session records the folder it was saved in, and reads it back; a
// session without the line has no folder.
func TestSessionRecordsItsFolder(t *testing.T) {
	sessionSandbox(t)
	id := strings.Repeat("0f", 32)
	if err := SaveSession(Session{Folder: id, Slots: oneSlot(fixtureChart(), false)}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(readString(t, SessionPath()), "version: 3\nfolder: "+id+"\n") {
		t.Errorf("the folder line is not written after the version:\n%s", readString(t, SessionPath()))
	}
	s, ok, err := LoadSession()
	if err != nil || !ok || s.Folder != id || len(s.Slots) != 1 || !s.Slots[0].Usable {
		t.Errorf("LoadSession = %+v ok %v err %v", s, ok, err)
	}
	writeFile(t, SessionPath(), "version: 3\nfloating:\n  selected: false\n")
	if s, _, _ := LoadSession(); s.Folder != "" {
		t.Errorf("a session with no folder line read as folder %q", s.Folder)
	}
}

// The backup path the operator is told is absolute; inside the launcher's
// Docker contract it is the host's path to the same file.
func TestBackupDisplayPath(t *testing.T) {
	charts := stateSandbox(t)
	t.Cleanup(OverrideContainerProbe(func() bool { return true }))
	if got, want := ChartBackupDisplay(charts, "Client.as"), filepath.Join(BackupDir(charts), "Client.as"); got != want || !filepath.IsAbs(got) {
		t.Errorf("display = %q, want %q", got, want)
	}
	t.Setenv("SEXTILE_BACKUP_HOST_PATH", "/data/op/.local/share/sextile/backup")
	if got := ChartBackupDisplay(charts, "Client.as"); got != filepath.Join(BackupDir(charts), "Client.as") {
		t.Errorf("the host path was shown outside the launcher's contract: %q", got)
	}
	t.Setenv("SEXTILE_LAUNCH", "docker sextile:test")
	t.Setenv("SEXTILE_CHARTS_HOST_PATH", "/data/op/Clients")
	want := filepath.Join("/data/op/.local/share/sextile/backup", folderHash("/data/op/Clients"), "Client.as")
	if got := ChartBackupDisplay("/charts", "Client.as"); got != want {
		t.Errorf("Docker display = %q, want %q", got, want)
	}
}

// The launcher's host path names the charts folder only inside Docker, where
// the folder is /charts. A native or hand-run launch with a forged
// SEXTILE_LAUNCH and SEXTILE_CHARTS_HOST_PATH keeps its own physical identity,
// so folder B can never pass for folder A.
func TestForgedLauncherEnvDoesNotLendAnotherFoldersIdentity(t *testing.T) {
	stateSandbox(t)
	a, b := t.TempDir(), t.TempDir()
	own := FolderID(b)
	t.Cleanup(OverrideContainerProbe(func() bool { return false }))
	for _, launch := range []string{"no-docker x", "1", "docker sextile:test"} {
		t.Setenv("SEXTILE_LAUNCH", launch)
		t.Setenv("SEXTILE_CHARTS_HOST_PATH", a)
		t.Setenv("SEXTILE_BACKUP_HOST_PATH", "/elsewhere/backup")
		if got := FolderID(b); got != own {
			t.Errorf("SEXTILE_LAUNCH=%q: FolderID(B) = %s, want B's own %s", launch, got, own)
		}
		if got := ChartBackupDisplay(b, "Client.as"); got != filepath.Join(BackupDir(b), "Client.as") {
			t.Errorf("SEXTILE_LAUNCH=%q: display = %q, want the real path", launch, got)
		}
	}
}

// Migration keeps no record of its own: each launch looks again, says nothing
// when everything is already in place, and copies a legacy backup that was
// added to .sextile after an earlier launch.
func TestMigrationPicksUpALaterLegacyBackup(t *testing.T) {
	charts := stateSandbox(t)
	seedLegacy(t, charts, "A")
	if r := MigrateLegacyState(charts); len(r.Copied) == 0 {
		t.Fatalf("premise: the first launch copied nothing: %+v", r)
	}
	if line := MigrateLegacyState(charts).Line(); line != "" {
		t.Errorf("a launch with everything in place said %q, want nothing", line)
	}
	writeFile(t, filepath.Join(charts, ".sextile", "backup", "Later.as"), retitleBefore+"; later\n")
	r := MigrateLegacyState(charts)
	if got := strings.Join(r.Copied, ","); got != "1 backup" {
		t.Errorf("copied = %q, want the later backup (result %+v)", got, r)
	}
	if got := readString(t, filepath.Join(BackupDir(charts), "Later.as")); got != retitleBefore+"; later\n" {
		t.Errorf("the later backup = %q", got)
	}
	if !strings.Contains(r.Line(), "copied 1 backup") || !strings.Contains(r.Line(), "already there") {
		t.Errorf("the line for a launch that copied something = %q, want it to name what was already there too", r.Line())
	}
	if _, err := os.Lstat(filepath.Join(os.Getenv("XDG_STATE_HOME"), "sextile", "migrated")); !os.IsNotExist(err) {
		t.Errorf("migration left a record of its own (stat err %v)", err)
	}
}

// /charts is only the launcher's mount inside its container. A native run
// with --charts-dir /charts and a hand-set "docker ..." launch and host path
// keeps the native identity of /charts; inside a container the host path is
// used, as the launcher's Docker run needs.
func TestForgedDockerEnvAtChartsOutsideAContainer(t *testing.T) {
	stateSandbox(t)
	t.Setenv("SEXTILE_LAUNCH", "docker sextile:test")
	t.Setenv("SEXTILE_CHARTS_HOST_PATH", "/client/A")
	t.Setenv("SEXTILE_BACKUP_HOST_PATH", "/client/backup")
	native := folderHash("/charts")
	if _, err := os.Stat("/charts"); err == nil {
		t.Skip("this host has a real /charts; its physical path would differ")
	}

	t.Cleanup(OverrideContainerProbe(func() bool { return false }))
	if got := FolderID("/charts"); got != native {
		t.Errorf("outside a container FolderID(/charts) = %s, want the native %s (not /client/A's %s)", got, native, folderHash("/client/A"))
	}
	if got := ChartBackupDisplay("/charts", "Client.as"); got != filepath.Join(BackupDir("/charts"), "Client.as") {
		t.Errorf("outside a container the display used the host path: %q", got)
	}

	OverrideContainerProbe(func() bool { return true })
	if got, want := FolderID("/charts"), folderHash("/client/A"); got != want {
		t.Errorf("inside the container FolderID(/charts) = %s, want the host path's %s", got, want)
	}
	if got, want := ChartBackupDisplay("/charts", "Client.as"), filepath.Join("/client/backup", folderHash("/client/A"), "Client.as"); got != want {
		t.Errorf("inside the container display = %q, want %q", got, want)
	}
}
