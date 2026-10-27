package hooks

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fakeAstrolog answers the ASTROLOG row's probes the way a complete Astrolog
// 8.00 does, and breaks one thing when FAKE_ASTROLOG_BREAK names it:
// semo_12 (the Moon in 1500 comes back from the fallback, with no message,
// as a real install without that file does), sepl_12 (the same for Pluto and
// the Moon; a missing semo_18 or sepl_18 moves the 1500 Moon too),
// seas_12, seas_18 and Eris (Astrolog's "not found" line, and the body at
// 0Ari00), semo_18 and sepl_18 (the same silent fallback for the 1990 chart),
// damaged (the Swiss Ephemeris "damaged" line), exit1 (a failing exit status
// with no message), hang (the version probe never answers).
const fakeAstrolog = `#!/bin/sh
b=${FAKE_ASTROLOG_BREAK-}
case " $* " in
*" -Hc "*)
  [ "$b" = hang ] && exec sleep 30
  echo "|                        ** Astrolog version 8.00 **                       |"
  exit 0 ;;
*" 1500 "*)
  moon="23Aqu15'00\""; pluto="22Sco53'50\""; chiron="21Gem40'31\""
  case $b in
  semo_12 | sepl_12) moon="23Aqu15'03\"" ;;
  semo_18 | sepl_18) moon="23Aqu14'59\"" ;;
  esac
  [ "$b" = sepl_12 ] && pluto="22Sco53'35\""
  if [ "$b" = seas_12 ]; then
    echo "SwissEph file 'seas_12.se1' not found in PATH '.;:'"; chiron="0Ari00'00\""
  fi
  [ "$b" = damaged ] && echo "Ephemeris file seas_12.se1 is damaged (0h)."
  echo "Astrolog 8.00: Mon Jun 15, 1500"
  echo "Moon: $moon   - 5:04'33\" (-)"
  echo "Plut: $pluto R +13:33'13\" (R)"
  echo "Chir: $chiron   - 5:13'45\" (d)"
  [ "$b" = exit1 ] && exit 1
  exit 0 ;;
*" 1990 "*" -R0 1 2 3 "*)
  sun="+0.9550731 +0.0000263"; moon="+13.475105 +0.9099220"; chiron="16Can05'31\""
  [ "$b" = sepl_18 ] && sun="+0.9550718 +0.0000263"
  [ "$b" = sepl_18 -o "$b" = semo_18 ] && moon="+13.475086 +0.9098571"
  if [ "$b" = seas_18 ]; then
    echo "SwissEph file 'seas_18.se1' not found in PATH '.;:'"; chiron="0Ari00'00\""
  fi
  echo "Sun : 24Gem25'40\" + 0:00'00\" (-) $sun"
  echo "Moon: 19Pis33'04\" + 3:25'19\" (-) $moon"
  echo "Chir: $chiron - 6:15'56\" (-)"
  exit 0 ;;
*" -R0 "*)
  case " $* " in
  *" 43 "*) n=Hygi; f=se00010s ;;   *" 45 "*) n=Eris; f=s136199s ;;
  *" 46 "*) n=Haum; f=s136108s ;;   *" 47 "*) n=Make; f=s136472s ;;
  *" 48 "*) n=Gong; f=s225088s ;;   *" 49 "*) n=Quao; f=se50000s ;;
  *" 50 "*) n=Sedn; f=se90377s ;;   *" 51 "*) n=Orcu; f=se90482s ;;
  esac
  [ "$b" = "$n" ] && echo "SwissEph file '$f.se1' not found in PATH '.;:'"
  echo "$n: 17Ari31'47\" -17:17'14\" (d)"
  exit 0 ;;
esac
echo "Sun : 24Gem19   - 0:00' (-)"
`

// fakeShellcheck lints as much as the ASTROLOG-style probe needs: the
// unquoted $1 of the known-bad snippet is SC2086, anything else is clean.
const fakeShellcheck = `#!/bin/sh
if grep -q 'echo \$1' ; then echo "SC2086 (info): Double quote to prevent globbing"; exit 1; fi
`

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
	bin string
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
	write(filepath.Join(bin, "astrolog"), fakeAstrolog, 0o755)
	write(filepath.Join(bin, "shellcheck"), fakeShellcheck, 0o755)

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

	c := checkTree{t: t, dir: dir, log: log, bin: bin}
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

// TestReleaseTrapRemovesOnlyInTheCheckItself: ./check --release removes its
// extracted tree on EXIT, and every child bash forks with that trap still
// installed until it resets it. probe kills its watchdog subshell the moment
// a fast probe returns; a SIGTERM that lands before the child's reset runs the
// trap there, and the tree vanished under the rows still running in it (a
// release check failing on getcwd after ASTROLOG, about one run in fifteen
// with the stub toolchain). This runs the trap ./check installs the way such
// a child runs it, in a subshell, then in the check's own shell.
func TestReleaseTrapRemovesOnlyInTheCheckItself(t *testing.T) {
	wd, _ := os.Getwd()
	b, err := os.ReadFile(filepath.Join(wd, "..", "..", "check"))
	if err != nil {
		t.Fatal(err)
	}
	traps := regexp.MustCompile(`(?m)^\s*trap '([^']*(?:'\\''[^']*)*)' EXIT$`).FindAllStringSubmatch(string(b), -1)
	if len(traps) != 1 || !strings.Contains(traps[0][1], `"$WORK"`) {
		t.Fatalf("want exactly one EXIT trap in ./check, the one that removes WORK; found %q", traps)
	}
	work := filepath.Join(t.TempDir(), "sextile-release.test")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `set -Eeuo pipefail
WORK=$1
( eval "$CHECK_TRAP" ) || true
[ -d "$WORK" ] || { echo "a child running the trap removed WORK"; exit 3; }
eval "$CHECK_TRAP"
[ ! -e "$WORK" ] || { echo "the check's own shell kept WORK"; exit 4; }
`
	out, code := sh(t, wd, []string{"CHECK_TRAP=" + traps[0][1]}, "", "bash", "-c", script, "trapprobe", work)
	if code != 0 {
		t.Fatalf("EXIT trap %q: exit %d\n%s", traps[0][1], code, out)
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

// withBreak is the tree with the fake astrolog broken in one way.
func (c checkTree) withBreak(what string) checkTree {
	c.env = append(append([]string{}, c.env...), "FAKE_ASTROLOG_BREAK="+what)
	return c
}

// TestCheckProvesTheAstrologData: the ASTROLOG row is not satisfied by an
// astrolog that merely exists. A complete one passes; one missing a single
// data file, damaged, failing or hanging fails the row and the refusal names
// what is wrong, per file. Without sepl_12 or semo_12 the real Astrolog says
// nothing and falls back, so those two are told apart by arc-second values.
func TestCheckProvesTheAstrologData(t *testing.T) {
	t.Run("complete", func(t *testing.T) {
		out, code := newCheckTree(t).run()
		if code != 0 || !strings.Contains(out, "check: ASTROLOG ok") {
			t.Fatalf("a complete astrolog must pass, got %d\n%s", code, out)
		}
	})
	for _, tc := range []struct{ brk, want string }{
		{"semo_12", "semo_12.se1 (the Moon in 1500)"},
		{"sepl_12", "sepl_12.se1 (the planets in 1500)"},
		{"seas_12", "seas_12.se1 (Chiron in 1500)"},
		{"semo_18", "semo_18.se1 (the Moon in 1990)"},
		{"sepl_18", "sepl_18.se1 (the planets in 1990)"},
		{"seas_18", "seas_18.se1 (Chiron in 1990)"},
		{"damaged", "seas_12.se1 (Chiron in 1500)"},
		{"Eris", "s136199s.se1 (Eris)"},
	} {
		t.Run("missing "+tc.brk, func(t *testing.T) {
			out, code := newCheckTree(t).withBreak(tc.brk).run()
			if code != 1 || !strings.Contains(out, "check: ASTROLOG FAIL") ||
				!strings.Contains(out, "lacks its ephemeris files ("+tc.want+"); scripts/build-astrolog installs a complete one") {
				t.Fatalf("an astrolog broken at %s must fail the ASTROLOG row naming %q, got %d\n%s", tc.brk, tc.want, code, out)
			}
		})
	}
	t.Run("exit status", func(t *testing.T) {
		out, code := newCheckTree(t).withBreak("exit1").run()
		if code != 1 || !strings.Contains(out, "check: ASTROLOG FAIL") || !strings.Contains(out, "failed its self-checks (the 1500 chart: exit 1)") {
			t.Fatalf("a failing exit status with no message must fail the row, got %d\n%s", code, out)
		}
	})
	t.Run("hang", func(t *testing.T) {
		c := newCheckTree(t).withBreak("hang")
		c.env = append(c.env, "CHECK_TEST_PROBE_SECONDS=1")
		out, code := c.run()
		if code != 1 || !strings.Contains(out, "check: ASTROLOG FAIL") || !strings.Contains(out, "failed its self-checks (the version: timed out after 1s)") {
			t.Fatalf("a hanging astrolog must be refused by the watchdog, got %d\n%s", code, out)
		}
	})
}

// pathWithout is a directory of links to every command on PATH except name,
// for a test of what ./check does when name is not installed.
func pathWithout(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		entries, _ := os.ReadDir(d)
		for _, e := range entries {
			if e.Name() == name {
				continue
			}
			_ = os.Symlink(filepath.Join(d, e.Name()), filepath.Join(dir, e.Name())) // the first on PATH wins
		}
	}
	return dir
}

// TestReleaseCheckRefusesWithoutShellcheck: the installer's shell check skips
// when shellcheck is absent, so ./check --release must not run without it;
// plain ./check keeps its behaviour.
func TestReleaseCheckRefusesWithoutShellcheck(t *testing.T) {
	c := newCheckTree(t)
	if err := os.Remove(filepath.Join(c.bin, "shellcheck")); err != nil {
		t.Fatal(err)
	}
	c.env[0] = "PATH=" + c.bin + ":" + pathWithout(t, "shellcheck")
	out, code := c.run("--release", "HEAD")
	if code != 1 || !strings.Contains(out, "check: shellcheck is not on PATH, so the installer's shell check would skip") ||
		!strings.Contains(out, "check: FAIL SHELLCHECK") || c.rows() != "" {
		t.Fatalf("want an immediate failure without shellcheck, got %d\n%s", code, out)
	}
	if out, code := c.run(); code != 0 {
		t.Fatalf("plain check must not need shellcheck, got %d\n%s", code, out)
	}
	// with shellcheck present the release check goes on
	c = newCheckTree(t)
	if out, code := c.run("--release", "HEAD"); code != 0 || !strings.Contains(out, "check: SHELLCHECK ok") {
		t.Fatalf("a release check with shellcheck must pass, got %d\n%s", code, out)
	}
}

// TestReleaseCheckRefusesAShellcheckThatDoesNotLint: a shim named shellcheck
// that exits 0 would let the installer's shell check pass without checking.
func TestReleaseCheckRefusesAShellcheckThatDoesNotLint(t *testing.T) {
	c := newCheckTree(t)
	if err := os.WriteFile(filepath.Join(c.bin, "shellcheck"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, code := c.run("--release", "HEAD")
	if code != 1 || !strings.Contains(out, "check: shellcheck on PATH does not lint (") || !strings.Contains(out, "check: FAIL SHELLCHECK") || c.rows() != "" {
		t.Fatalf("a shellcheck shim that exits 0 must be refused, got %d\n%s", code, out)
	}
}
