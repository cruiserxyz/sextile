package hooks

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Synthetic rules, as in the crawl's own tests: the words the real rules deny
// must not appear in this repository.
const checkRules = `# synthetic rules for the check script's tests
deny \bzorblax\b
trailer Made-With
author Tester <t@example.invalid>
exec check
`

// checkTree is a throwaway git repository holding ./check and the crawl tool,
// the rules pin, a base commit on main, and a stub toolchain on PATH: `go`
// runs for real only for `go run` (the crawl), everything else is logged and
// succeeds, so no row of the repo's own suite ever runs.
type checkTree struct {
	t   *testing.T
	dir string
	env []string
	log string
}

func (c checkTree) git(args ...string) string {
	c.t.Helper()
	out, code := sh(c.t, c.dir, c.env, "", "git", args...)
	if code != 0 {
		c.t.Fatalf("git %v: exit %d\n%s", args, code, out)
	}
	return out
}

func (c checkTree) commit(message string) {
	c.t.Helper()
	p := filepath.Join(c.t.TempDir(), "msg")
	if err := os.WriteFile(p, []byte(message), 0o644); err != nil {
		c.t.Fatal(err)
	}
	c.git("add", "-A")
	c.git("commit", "-q", "--allow-empty", "-F", p, "--cleanup=verbatim")
}

func newCheckTree(t *testing.T) checkTree {
	t.Helper()
	wd, _ := os.Getwd()
	root := filepath.Join(wd, "..", "..")
	dir := t.TempDir()
	bin := t.TempDir()
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "rows.log")
	write := func(path, content string, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(bin, "go"), "#!/bin/sh\nif [ \"$1\" = run ]; then exec \""+realGo+"\" \"$@\"; fi\nif [ \"$1\" = test ]; then printf 'repo=%s\\nrev=%s\\n' \"${SEXTILE_RELEASE_REPO-<unset>}\" \"${SEXTILE_RELEASE_REV-<unset>}\" > \"$ROWS_LOG.env\"; fi\necho \"go $*\" >> \"$ROWS_LOG\"\n", 0o755)
	write(filepath.Join(bin, "gofmt"), "#!/bin/sh\necho \"gofmt $*\" >> \"$ROWS_LOG\"\n", 0o755)
	write(filepath.Join(bin, "astrolog"), "#!/bin/sh\nexit 0\n", 0o755)

	rules := []byte(checkRules)
	rulesPath := filepath.Join(t.TempDir(), "rules.txt")
	write(rulesPath, checkRules, 0o644)
	sum := sha256.Sum256(rules)
	write(filepath.Join(dir, "tools/crawl/rules.sha256"), hex.EncodeToString(sum[:])+"\n", 0o644)
	for _, f := range []string{"check", "tools/crawl/main.go"} {
		b, err := os.ReadFile(filepath.Join(root, f))
		if err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if f == "check" {
			mode = 0o755
		}
		write(filepath.Join(dir, f), string(b), mode)
	}
	write(filepath.Join(dir, "go.mod"), "module example.invalid/checktree\n\ngo 1.21\n", 0o644)

	c := checkTree{t: t, dir: dir, log: log}
	c.env = []string{
		"PATH=" + bin + ":" + os.Getenv("PATH"),
		"ROWS_LOG=" + log,
		"SEXTILE_CRAWL_RULES=" + rulesPath,
		"GIT_AUTHOR_NAME=Tester", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=Tester", "GIT_COMMITTER_EMAIL=t@example.invalid",
		"GOFLAGS=", "GOMAXPROCS=4",
	}
	c.git("init", "-q", "-b", "main")
	c.git("config", "commit.gpgsign", "false")
	c.commit("area: first tree\n\nWhy this exists.\n")
	return c
}

// run executes ./check in the tree and returns exit code and all output.
func (c checkTree) run(args ...string) (string, int) {
	c.t.Helper()
	return sh(c.t, c.dir, c.env, "", filepath.Join(c.dir, "check"), args...)
}

// rows is what the stub toolchain recorded.
func (c checkTree) rows() string {
	b, _ := os.ReadFile(c.log)
	return string(b)
}

// badChild puts HEAD on a branch whose commit has a subject without an area.
func (c checkTree) badChild() {
	c.t.Helper()
	c.git("checkout", "-q", "-b", "work")
	c.commit("Add things without an area\n\nWhy.\n")
}

func TestPlainCheckStopsBeforeExpensiveRowsWhenMessagesFail(t *testing.T) {
	c := newCheckTree(t)
	c.badChild()
	out, code := c.run()
	if code != 1 || !strings.Contains(out, "check: MESSAGES FAIL") || !strings.Contains(out, "is not `area: imperative subject`") {
		t.Fatalf("want exit 1 with MESSAGES FAIL, got %d\n%s", code, out)
	}
	for _, row := range []string{"ASTROLOG", "GOFMT", "VET", "TEST", "CRAWL"} {
		if strings.Contains(out, "check: "+row) {
			t.Fatalf("row %s ran after MESSAGES failed:\n%s", row, out)
		}
	}
	if rows := c.rows(); rows != "" {
		t.Fatalf("no gofmt/vet/test may have run, the stubs recorded:\n%s", rows)
	}
	// the same tree with a clean commit passes MESSAGES and goes on to the rows
	c2 := newCheckTree(t)
	c2.git("checkout", "-q", "-b", "work")
	c2.commit("area: add things\n\nOne paragraph.\n")
	out, code = c2.run()
	if code != 0 || !strings.Contains(out, "check: MESSAGES ok") || !strings.Contains(out, "check: TEST ok") {
		t.Fatalf("a clean range must pass and run the rows, got %d\n%s", code, out)
	}
	if strings.Index(out, "check: MESSAGES ok") > strings.Index(out, "check: ASTROLOG ok") {
		t.Fatalf("MESSAGES must come before every other row:\n%s", out)
	}
}

func TestPlainCheckFailsWhenItCannotTellTheRange(t *testing.T) {
	// not a git work tree
	c := newCheckTree(t)
	c.env = append(c.env, "GIT_DIR=/nonexistent-git-dir")
	out, code := c.run()
	if code != 1 || !strings.Contains(out, "MESSAGES") || c.rows() != "" {
		t.Fatalf("want an immediate failure outside a git work tree, got %d\n%s", code, out)
	}
	// a repository with neither origin/main nor main
	c = newCheckTree(t)
	c.git("branch", "-m", "trunk")
	out, code = c.run()
	if code != 1 || !strings.Contains(out, "no base") || c.rows() != "" {
		t.Fatalf("want an immediate failure without a base ref, got %d\n%s", code, out)
	}
	// main is not an ancestor of HEAD
	c = newCheckTree(t)
	c.git("checkout", "-q", "--orphan", "island")
	c.commit("area: island\n\nWhy.\n")
	out, code = c.run()
	if code != 1 || !strings.Contains(out, "is not an ancestor of HEAD, so the range is undefined") || c.rows() != "" {
		t.Fatalf("want an immediate failure for a base that is not an ancestor, got %d\n%s", code, out)
	}
	// origin/main wins over main when it exists: the bad commit is below it
	c = newCheckTree(t)
	c.badChild()
	c.git("update-ref", "refs/remotes/origin/main", "HEAD")
	c.commit("area: more things\n\nWhy.\n")
	out, code = c.run()
	if code != 0 || !strings.Contains(out, "check: MESSAGES ok") {
		t.Fatalf("the range starts at origin/main, so the older bad commit is outside it: %d\n%s", code, out)
	}
}

// TestReleaseRowsKnowTheirCommit: the rows of a release check run in a tree
// with no .git, so ./check --release tells them the repository and the full
// commit the tree came from; plain ./check tells them nothing.
func TestReleaseRowsKnowTheirCommit(t *testing.T) {
	c := newCheckTree(t)
	envFile := c.log + ".env"
	head := strings.TrimSpace(c.git("rev-parse", "HEAD"))
	repo, err := filepath.EvalSymlinks(c.dir)
	if err != nil {
		t.Fatal(err)
	}
	out, code := c.run("--release", "HEAD")
	if code != 0 {
		t.Fatalf("release check failed: %d\n%s", code, out)
	}
	b, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("the TEST row did not run in release mode: %v\n%s", err, out)
	}
	if want := "repo=" + repo + "\nrev=" + head + "\n"; string(b) != want {
		t.Fatalf("release rows saw\n%swant\n%s", b, want)
	}
	// a short name still reaches the rows as the full commit id
	if err := os.Remove(envFile); err != nil {
		t.Fatal(err)
	}
	if _, code := c.run("--release", "main"); code != 0 {
		t.Fatal("release main failed")
	}
	if b, _ := os.ReadFile(envFile); string(b) != "repo="+repo+"\nrev="+head+"\n" {
		t.Fatalf("a branch name must reach the rows as the full commit id, saw\n%s", b)
	}
	// plain mode exports neither
	if err := os.Remove(envFile); err != nil {
		t.Fatal(err)
	}
	// even when the caller's environment carries stale values
	stale := c
	stale.env = append(append([]string{}, c.env...), "SEXTILE_RELEASE_REPO=/stale/repo", "SEXTILE_RELEASE_REV="+strings.Repeat("a", 40))
	out, code = stale.run()
	if code != 0 {
		t.Fatalf("plain check failed: %d\n%s", code, out)
	}
	b, err = os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("the TEST row did not run in plain mode: %v\n%s", err, out)
	}
	if want := "repo=<unset>\nrev=<unset>\n"; string(b) != want {
		t.Fatalf("plain rows saw\n%swant\n%s", b, want)
	}
}
