package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/store"
)

// A dev .env can set XDG_STATE_HOME, and the lock file lives in SessionDir(),
// so the lock is taken after the environment is loaded: it lands in the folder
// every later SessionDir() read uses, not in the one the process started with.
// The stand-in for .env sets XDG_STATE_HOME the way a real one does; this
// proves the order of the two calls, not that a dev build's .env parser
// applies the variable (internal/env's tests do that).
func TestLockIsTakenInTheFolderTheEnvironmentChose(t *testing.T) {
	before := t.TempDir()
	after := t.TempDir()
	t.Setenv("XDG_STATE_HOME", before)
	prev := loadEnv
	loadEnv = func() { os.Setenv("XDG_STATE_HOME", after) }
	t.Cleanup(func() { loadEnv = prev })

	if _, _, refused, err := prepareLaunch(&options{}); refused != nil || err != nil {
		t.Fatalf("prepareLaunch: refused %v, err %v", refused, err)
	}

	want := filepath.Join(store.SessionDir(), "sextile.lock")
	if filepath.Dir(want) != filepath.Join(after, "sextile", "session") {
		t.Fatalf("premise: SessionDir() = %s, want it under %s", store.SessionDir(), after)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("no lock file in the session folder the environment chose: %v", err)
	}
	if _, err := os.Stat(filepath.Join(before, "sextile", "session", "sextile.lock")); err == nil {
		t.Errorf("a lock file was also made in the folder the process started with")
	}
}

// seedLegacyFolder writes 0.1.1's session and one backup into dir/.sextile,
// tagged so a copy can be traced to its folder.
func seedLegacyFolder(t *testing.T, dir, tag string) {
	t.Helper()
	for p, body := range map[string]string{
		filepath.Join(dir, ".sextile", "data", "sextile", "session", "session.yaml"): "version: 3\nfloating:\n  selected: false\n# " + tag + "\n",
		filepath.Join(dir, ".sextile", "backup", "Client.as"):                        "backup " + tag + "\n",
	} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// --charts-dir B, with the environment naming A: the one-time copy runs on B,
// the folder the flag chose, after the lock is held, and A is left alone. The
// window that finds the lock held copies nothing.
func TestLaunchMigratesTheFolderTheFlagChose(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	a, b := filepath.Join(root, "A"), filepath.Join(root, "B")
	seedLegacyFolder(t, a, "A")
	seedLegacyFolder(t, b, "B")
	t.Setenv("SEXTILE_CHARTS_DIR", a)

	cfg, _, refused, err := prepareLaunch(&options{chartsDir: b})
	if refused != nil || err != nil {
		t.Fatalf("prepareLaunch: refused %v, err %v", refused, err)
	}
	sess, rerr := os.ReadFile(store.SessionPath())
	if rerr != nil {
		t.Fatalf("no session was copied: %v", rerr)
	}
	if !strings.Contains(string(sess), "# B") || !strings.Contains(string(sess), "folder: "+store.FolderID(b)) {
		t.Errorf("the session copied is not B's, stamped with B:\n%s", sess)
	}
	if _, err := os.Stat(filepath.Join(store.BackupDir(b), "Client.as")); err != nil {
		t.Errorf("B's backup was not copied: %v", err)
	}
	if _, err := os.Stat(store.BackupDir(a)); !os.IsNotExist(err) {
		t.Errorf("A's backups were copied too (stat err %v)", err)
	}
	if !strings.Contains(cfg.Migration, "copied session, 1 backup") {
		t.Errorf("the Log line handed to the UI = %q", cfg.Migration)
	}
}

func TestSecondWindowDoesNotMigrate(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	b := filepath.Join(root, "B")
	seedLegacyFolder(t, b, "B")
	holdSessionLock(t, filepath.Join(root, "state"))

	cfg, _, refused, err := prepareLaunch(&options{chartsDir: b})
	if refused != nil || err != nil || !cfg.SecondWindow {
		t.Fatalf("premise: a second window; got cfg %+v refused %v err %v", cfg, refused, err)
	}
	if cfg.Migration != "" {
		t.Errorf("a second window migrated: %q", cfg.Migration)
	}
	if _, err := os.Stat(store.SessionPath()); !os.IsNotExist(err) {
		t.Errorf("a second window copied the session (stat err %v)", err)
	}
}

// The real binary, run with --charts-dir B while SEXTILE_CHARTS_DIR names A:
// the copy comes from B only. The binary has no terminal (its own session, no
// controlling tty), so the screen cannot start; everything before the screen
// has run by then, and the copy is what this checks.
func TestBinaryMigratesFromTheChartsDirFlagOnly(t *testing.T) {
	runBinaryMigration(t, "none")
}

// The same launch where the filesystem cannot lock: nothing serialises two
// windows, and the copy must still happen on the real launch path. The build
// makes flock answer ENOTSUP (store's instanceLockFault seam).
func TestBinaryMigratesWhenLockingIsUnsupported(t *testing.T) {
	runBinaryMigration(t, "-X github.com/cruiserxyz/sextile/internal/store.instanceLockFault=ENOTSUP")
}

func runBinaryMigration(t *testing.T, ldflags string) {
	t.Helper()
	bin := binary(t, ldflags)
	root := t.TempDir()
	state, data := filepath.Join(root, "state"), filepath.Join(root, "data")
	a, b := filepath.Join(root, "A"), filepath.Join(root, "B")
	seedLegacyFolder(t, a, "A")
	seedLegacyFolder(t, b, "B")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--charts-dir", b)
	cmd.Env = []string{
		"HOME=" + filepath.Join(root, "home"), "XDG_STATE_HOME=" + state, "XDG_DATA_HOME=" + data,
		"XDG_CONFIG_HOME=" + filepath.Join(root, "config"), "XDG_CACHE_HOME=" + filepath.Join(root, "cache"),
		"SEXTILE_CHARTS_DIR=" + a, "PATH=/usr/bin:/bin", "TERM=dumb",
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	cmd.Run() // fails without a terminal; the copy happened before
	if ctx.Err() != nil {
		t.Fatalf("the binary did not exit without a terminal:\n%s", out.String())
	}

	sess, err := os.ReadFile(filepath.Join(state, "sextile", "session", "session.yaml"))
	if err != nil {
		t.Fatalf("the binary copied no session: %v\n%s", err, out.String())
	}
	if !strings.Contains(string(sess), "# B") || strings.Contains(string(sess), "# A") {
		t.Errorf("the session the binary copied is not B's:\n%s", sess)
	}
	entries, _ := os.ReadDir(filepath.Join(data, "sextile", "backup"))
	if len(entries) != 1 {
		t.Fatalf("backup folders = %d, want B's alone", len(entries))
	}
	if got, _ := os.ReadFile(filepath.Join(data, "sextile", "backup", entries[0].Name(), "Client.as")); string(got) != "backup B\n" {
		t.Errorf("the backup copied is %q, want B's", got)
	}
}
