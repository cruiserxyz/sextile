// Package hooks holds the tests of scripts/pre-push and scripts/install-hooks.
package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func sh(t *testing.T, dir string, env []string, stdin string, name string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return string(out), code
}

// repo makes a git repository with one commit and a stub ./check that
// records its arguments and exits with $STUB_EXIT.
func repo(t *testing.T) (dir, sha, hook string) {
	t.Helper()
	dir = t.TempDir()
	sh(t, dir, nil, "", "git", "init", "-q", "-b", "main")
	sh(t, dir, nil, "", "git", "config", "user.name", "Tester")
	sh(t, dir, nil, "", "git", "config", "user.email", "t@example.invalid")
	sh(t, dir, nil, "", "git", "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sh(t, dir, nil, "", "git", "add", "-A")
	sh(t, dir, nil, "", "git", "commit", "-q", "-m", "area: first\n\nwhy")
	out, _ := sh(t, dir, nil, "", "git", "rev-parse", "HEAD")
	sha = strings.TrimSpace(out)
	stub := "#!/bin/sh\necho \"stub $*\" >> \"$STUB_LOG\"\nexit ${STUB_EXIT:-0}\n"
	if err := os.WriteFile(filepath.Join(dir, "stub-check"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	hook = filepath.Join(wd, "..", "..", "scripts", "pre-push")
	return dir, sha, hook
}

func TestPrePushChecksEveryPushedRef(t *testing.T) {
	dir, sha, hook := repo(t)
	log := filepath.Join(dir, "log")
	env := []string{"SEXTILE_CHECK=" + filepath.Join(dir, "stub-check"), "STUB_LOG=" + log}
	zero := strings.Repeat("0", 40)
	stdin := "refs/heads/main " + sha + " refs/heads/main " + zero + "\n" +
		"refs/tags/v0.1.0 " + sha + " refs/tags/v0.1.0 " + zero + "\n" +
		"(delete) " + zero + " refs/heads/old " + sha + "\n"
	out, code := sh(t, dir, env, stdin, hook, "origin", "git@example.invalid:x.git")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	b, _ := os.ReadFile(log)
	calls := strings.Count(string(b), "stub --release "+sha)
	if calls != 2 {
		t.Fatalf("want ./check --release called for the branch and the tag (2), got %d\n%s\n%s", calls, b, out)
	}
	if !strings.Contains(out, "area: first") {
		t.Fatalf("the hook must print the messages it publishes:\n%s", out)
	}
	if !strings.Contains(out, "deletion") {
		t.Fatalf("a deletion is reported, not checked:\n%s", out)
	}
}

func TestPrePushRefusesOnAFailedCheck(t *testing.T) {
	dir, sha, hook := repo(t)
	env := []string{"SEXTILE_CHECK=" + filepath.Join(dir, "stub-check"), "STUB_LOG=" + filepath.Join(dir, "log"), "STUB_EXIT=1"}
	stdin := "refs/heads/main " + sha + " refs/heads/main " + strings.Repeat("0", 40) + "\n"
	out, code := sh(t, dir, env, stdin, hook, "origin", "x")
	if code == 0 || !strings.Contains(out, "the push is refused") {
		t.Fatalf("want a refusal, got exit %d\n%s", code, out)
	}
}

func TestInstallHooksPutsTheHookInPlace(t *testing.T) {
	dir, _, hook := repo(t)
	// install-hooks copies scripts/pre-push relative to the repo it runs in,
	// so give the temp repo a copy of both scripts.
	src, err := os.ReadFile(hook)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := os.ReadFile(filepath.Join(filepath.Dir(hook), "install-hooks"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "scripts", "pre-push"), src, 0o755)
	os.WriteFile(filepath.Join(dir, "scripts", "install-hooks"), inst, 0o755)
	out, code := sh(t, dir, nil, "", filepath.Join(dir, "scripts", "install-hooks"))
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".git", "hooks", "pre-push"))
	if err != nil || string(got) != string(src) {
		t.Fatalf("the installed hook differs or is missing: %v", err)
	}
}
