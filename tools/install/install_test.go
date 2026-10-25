// Package install holds the tests of scripts/install, the one-line installer.
//
// Every test runs the script with PATH holding only fakes of git, docker and
// uname (shell scripts that record their arguments and anything left on their
// standard input), HOME in a temporary directory, and the working directory
// somewhere else. Nothing here reaches the network or a real Docker. One test
// runs the real git against a local repository, so the argv recorder is not the
// only witness of what the script does.
package install

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	tag     = "v0.1.0"
	repoURL = "https://github.com/cruiserxyz/sextile.git"
)

func repoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func scriptPath(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "scripts", "install"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func look(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("%s is needed to run these tests: %v", name, err)
	}
	return p
}

// fakeGit records every call (each argument on its own line, then --END--) to
// $FAKE_LOG, appends whatever is on its stdin to $FAKE_STDIN, and answers the
// few verbs the installer uses from files under the directory's .git:
// fake-origin and fake-tag.
const fakeGit = `#!/bin/sh
{ for a in "$@"; do printf '%s\n' "$a"; done; printf '%s\n' '--END--'; } >> "$FAKE_LOG"
@CAT@ >> "$FAKE_STDIN"
dir=
while [ $# -gt 0 ]; do
  case $1 in
    -c) shift 2 ;;
    -C) dir=$2; shift 2 ;;
    *) break ;;
  esac
done
verb=$1
shift
case $verb in
  clone)
    branch= prev= last=
    while [ $# -gt 0 ]; do
      case $1 in
        --branch) branch=$2; shift 2 ;;
        *) prev=$last last=$1; shift ;;
      esac
    done
    [ "${FAKE_CLONE_EXIT:-0}" -eq 0 ] || exit "$FAKE_CLONE_EXIT"
    @MKDIR@ -p "$last/.git"
    printf '%s\n' "$branch" > "$last/.git/fake-tag"
    printf '%s\n' "$prev" > "$last/.git/fake-origin"
    ;;
  config)
    [ -f "$dir/.git/fake-origin" ] || exit 1
    @CAT@ "$dir/.git/fake-origin"
    ;;
  describe)
    if [ -n "${FAKE_DESCRIBE+x}" ]; then printf '%s\n' "$FAKE_DESCRIBE"; exit 0; fi
    [ -f "$dir/.git/fake-tag" ] || exit 128
    @CAT@ "$dir/.git/fake-tag"
    ;;
  fetch)
    exit "${FAKE_FETCH_EXIT:-0}"
    ;;
  checkout)
    for last in "$@"; do :; done
    printf '%s\n' "${last#refs/tags/}" > "$dir/.git/fake-tag"
    ;;
  *)
    echo "FAKE-GIT-UNEXPECTED $verb" >&2
    exit 99
    ;;
esac
`

const fakeDocker = `#!/bin/sh
{ for a in "$@"; do printf '%s\n' "$a"; done; printf '%s\n' '--END--'; } >> "$FAKE_LOG"
@CAT@ >> "$FAKE_STDIN"
case $1 in
  info) exit "${FAKE_DOCKER_INFO_EXIT:-0}" ;;
  *) echo "FAKE-DOCKER-UNEXPECTED $1" >&2; exit 99 ;;
esac
`

const fakeUname = `#!/bin/sh
printf '%s\n' "${FAKE_UNAME:-Linux}"
`

type rig struct {
	t     *testing.T
	home  string
	bin   string
	work  string
	log   string
	stdin string
	env   []string
}

// newRig makes the fakes; withGit and withDocker say which of the two exist.
func newRig(t *testing.T, withGit, withDocker bool) *rig {
	t.Helper()
	root := t.TempDir()
	r := &rig{
		t:     t,
		home:  filepath.Join(root, "home"),
		bin:   filepath.Join(root, "bin"),
		work:  filepath.Join(root, "cwd"),
		log:   filepath.Join(root, "calls.log"),
		stdin: filepath.Join(root, "stdin.log"),
	}
	for _, d := range []string{r.home, r.bin, r.work} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sub := strings.NewReplacer("@CAT@", look(t, "cat"), "@MKDIR@", look(t, "mkdir"))
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(r.bin, name), []byte(sub.Replace(body)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("uname", fakeUname)
	if withGit {
		write("git", fakeGit)
	}
	if withDocker {
		write("docker", fakeDocker)
	}
	for _, f := range []string{r.log, r.stdin} {
		if err := os.WriteFile(f, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

type result struct {
	stdout, stderr string
	code           int
}

// lastLine is the last non-empty line of stdout.
func (res result) lastLine() string {
	lines := strings.Split(strings.TrimRight(res.stdout, "\n"), "\n")
	return lines[len(lines)-1]
}

// run runs the script under shell with PATH = the fakes only. stdin is what
// the shell is given; with args == nil the script is the file argument.
func (r *rig) run(shell string, stdin []byte, extraEnv []string, args ...string) result {
	r.t.Helper()
	return r.runPath(r.bin, shell, stdin, extraEnv, args...)
}

func (r *rig) runPath(path, shell string, stdin []byte, extraEnv []string, args ...string) result {
	r.t.Helper()
	cmd := exec.Command(shell, args...)
	cmd.Dir = r.work
	cmd.Env = append([]string{
		"PATH=" + path,
		"HOME=" + r.home,
		"FAKE_LOG=" + r.log,
		"FAKE_STDIN=" + r.stdin,
	}, extraEnv...)
	cmd.Stdin = bytes.NewReader(stdin)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	res := result{stdout: so.String(), stderr: se.String()}
	if ee, ok := err.(*exec.ExitError); ok {
		res.code = ee.ExitCode()
	} else if err != nil {
		r.t.Fatalf("%s: %v", shell, err)
	}
	return res
}

// runScript runs scripts/install as a file argument of the system's sh.
func (r *rig) runScript(extraEnv ...string) result {
	r.t.Helper()
	return r.run(look(r.t, "sh"), nil, extraEnv, scriptPath(r.t))
}

// calls is every recorded call of the fake git and docker, in order.
func (r *rig) calls() [][]string {
	r.t.Helper()
	b, err := os.ReadFile(r.log)
	if err != nil {
		r.t.Fatal(err)
	}
	var all [][]string
	var cur []string
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		switch {
		case l == "--END--":
			all = append(all, cur)
			cur = nil
		case l == "" && cur == nil:
		default:
			cur = append(cur, l)
		}
	}
	return all
}

// callsOf is the recorded calls whose first word after any -c/-C pairs is verb.
func (r *rig) callsOf(verb string) [][]string {
	var out [][]string
	for _, c := range r.calls() {
		a := c
		for len(a) >= 2 && (a[0] == "-c" || a[0] == "-C") {
			a = a[2:]
		}
		if len(a) > 0 && a[0] == verb {
			out = append(out, c)
		}
	}
	return out
}

func (r *rig) leaked() string {
	b, err := os.ReadFile(r.stdin)
	if err != nil {
		r.t.Fatal(err)
	}
	return string(b)
}

func (r *rig) exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func (r *rig) mkClone(dir, origin, atTag string) {
	r.t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		r.t.Fatal(err)
	}
	for f, v := range map[string]string{"fake-origin": origin, "fake-tag": atTag} {
		if err := os.WriteFile(filepath.Join(dir, ".git", f), []byte(v+"\n"), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
}

func eq(t *testing.T, what string, got, want any) {
	t.Helper()
	if fmt.Sprintf("%q", got) != fmt.Sprintf("%q", want) {
		t.Fatalf("%s:\n got %q\nwant %q", what, got, want)
	}
}

// ---------------------------------------------------------------- I1-1

func TestInstallRefusesWhenGitOrDockerIsMissing(t *testing.T) {
	cases := []struct {
		name       string
		git, dock  bool
		env        []string
		want       string
		gitCallsOK bool
	}{
		{"no git", false, true, nil, "git was not found", true},
		{"no docker", true, false, nil, "https://docs.docker.com/get-docker/", false},
		{"daemon down", true, true, []string{"FAKE_DOCKER_INFO_EXIT=1"}, "daemon did not answer", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, c.git, c.dock)
			res := r.runScript(c.env...)
			if res.code != 2 {
				t.Fatalf("exit %d, want 2\n%s%s", res.code, res.stdout, res.stderr)
			}
			if !strings.Contains(res.stderr, c.want) {
				t.Fatalf("stderr lacks %q:\n%s", c.want, res.stderr)
			}
			if n := strings.Count(strings.TrimRight(res.stderr, "\n"), "\n"); n != 0 || res.stdout != "" {
				t.Fatalf("want one sentence on stderr and nothing on stdout:\nstdout %q\nstderr %q", res.stdout, res.stderr)
			}
			if r.exists(filepath.Join(r.home, "sextile")) {
				t.Fatal("a folder was written before the prerequisites passed")
			}
			if g := r.callsOf("clone"); len(g) != 0 {
				t.Fatalf("git cloned: %q", g)
			}
		})
	}
}

func TestInstallRefusesAnotherSystem(t *testing.T) {
	r := newRig(t, true, true)
	res := r.runScript("FAKE_UNAME=MINGW64_NT-10.0")
	if res.code != 2 {
		t.Fatalf("exit %d, want 2\n%s%s", res.code, res.stdout, res.stderr)
	}
	const sentence = "Docker is the install, on Linux or macOS, and on Windows from a WSL terminal"
	if !strings.Contains(res.stderr, sentence) || !strings.Contains(res.stderr, "MINGW64_NT-10.0") {
		t.Fatalf("stderr: %q", res.stderr)
	}
	if !strings.Contains(repoFile(t, "docs/REFERENCE.md"), sentence) {
		t.Fatalf("docs/REFERENCE.md no longer says %q, which the installer quotes", sentence)
	}
	if len(r.calls()) != 0 {
		t.Fatalf("calls before the platform check: %q", r.calls())
	}
	for _, sys := range []string{"Linux", "Darwin"} {
		r := newRig(t, true, true)
		if res := r.runScript("FAKE_UNAME=" + sys); res.code != 0 {
			t.Fatalf("%s: exit %d\n%s%s", sys, res.code, res.stdout, res.stderr)
		}
	}
}

// ---------------------------------------------------------------- I1-2

func TestInstallClonesTheTagIntoHome(t *testing.T) {
	r := newRig(t, true, true)
	res := r.runScript()
	if res.code != 0 {
		t.Fatalf("exit %d\n%s%s", res.code, res.stdout, res.stderr)
	}
	dir := filepath.Join(r.home, "sextile")
	clones := r.callsOf("clone")
	if len(clones) != 1 {
		t.Fatalf("want one clone, got %q", r.calls())
	}
	eq(t, "the clone's argv", clones[0],
		[]string{"-c", "advice.detachedHead=false", "clone", "--branch", tag, "--depth", "1", "--", repoURL, dir})
	desc := r.callsOf("describe")
	if len(desc) != 1 {
		t.Fatalf("want git describe once, got %q", r.calls())
	}
	eq(t, "the verification", desc[0], []string{"-C", dir, "describe", "--tags", "--exact-match"})
	eq(t, "the last line", res.lastLine(), "cd ~/sextile && ./sextile")
	if !strings.Contains(res.stdout, "./sextile doctor") {
		t.Fatalf("stdout does not name ./sextile doctor:\n%s", res.stdout)
	}
	if !r.exists(filepath.Join(dir, ".git")) {
		t.Fatal("no clone on disk")
	}
	for _, c := range r.calls() {
		for _, a := range c {
			if a == "run" || a == "build" || strings.Contains(a, "sextile doctor") {
				t.Fatalf("the installer started something: %q", c)
			}
		}
	}
	if len(r.callsOf("info")) != 1 {
		t.Fatalf("want docker info once, got %q", r.calls())
	}
}

func TestInstallFailsWhenTheCloneIsNotAtTheTag(t *testing.T) {
	r := newRig(t, true, true)
	res := r.runScript("FAKE_DESCRIBE=v0.0.9")
	if res.code != 1 {
		t.Fatalf("exit %d, want 1\n%s%s", res.code, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stderr, "is not at "+tag) {
		t.Fatalf("stderr: %q", res.stderr)
	}
	if strings.Contains(res.stdout, "cd ~/sextile && ./sextile") {
		t.Fatalf("it told the reader to start a wrong tree:\n%s", res.stdout)
	}
	r = newRig(t, true, true)
	if res := r.runScript("FAKE_CLONE_EXIT=128"); res.code != 1 || strings.Contains(res.stdout, "./sextile") {
		t.Fatalf("a failed clone: exit %d\n%s%s", res.code, res.stdout, res.stderr)
	}
}

// ---------------------------------------------------------------- I1-3

func TestInstallUpdatesAnExistingClone(t *testing.T) {
	for _, origin := range []string{repoURL, strings.TrimSuffix(repoURL, ".git")} {
		t.Run(origin, func(t *testing.T) {
			r := newRig(t, true, true)
			dir := filepath.Join(r.home, "sextile")
			r.mkClone(dir, origin, "main")
			mine := filepath.Join(dir, "my-notes.txt")
			if err := os.WriteFile(mine, []byte("keep\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			res := r.runScript()
			if res.code != 0 {
				t.Fatalf("exit %d\n%s%s", res.code, res.stdout, res.stderr)
			}
			if len(r.callsOf("clone")) != 0 {
				t.Fatalf("it cloned over an existing clone: %q", r.calls())
			}
			eq(t, "fetch", r.callsOf("fetch"),
				[][]string{{"-C", dir, "fetch", "--quiet", "origin", "refs/tags/" + tag + ":refs/tags/" + tag}})
			eq(t, "checkout", r.callsOf("checkout"),
				[][]string{{"-C", dir, "checkout", "--quiet", "--detach", "refs/tags/" + tag}})
			if b, err := os.ReadFile(mine); err != nil || string(b) != "keep\n" {
				t.Fatalf("a file of the user's changed: %q %v", b, err)
			}
			eq(t, "the last line", res.lastLine(), "cd ~/sextile && ./sextile")
		})
	}
}

func TestInstallRefusesAFolderThatIsNotItsClone(t *testing.T) {
	setups := map[string]func(r *rig, dir string){
		"plain folder": func(r *rig, dir string) {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"another repository": func(r *rig, dir string) {
			r.mkClone(dir, "https://example.invalid/other.git", "main")
		},
		"a file": func(r *rig, dir string) {
			if err := os.WriteFile(dir, []byte("not a folder\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, setup := range setups {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, true, true)
			dir := filepath.Join(r.home, "sextile")
			setup(r, dir)
			if st, err := os.Stat(dir); err == nil && st.IsDir() {
				if err := os.WriteFile(filepath.Join(dir, "precious.txt"), []byte("mine\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			res := r.runScript()
			if res.code != 2 {
				t.Fatalf("exit %d, want 2\n%s%s", res.code, res.stdout, res.stderr)
			}
			if !strings.Contains(res.stderr, "left untouched") || res.stdout != "" {
				t.Fatalf("stdout %q stderr %q", res.stdout, res.stderr)
			}
			for _, verb := range []string{"clone", "fetch", "checkout"} {
				if c := r.callsOf(verb); len(c) != 0 {
					t.Fatalf("git %s ran in a folder that is not ours: %q", verb, c)
				}
			}
			if b, err := os.ReadFile(filepath.Join(dir, "precious.txt")); name != "a file" && (err != nil || string(b) != "mine\n") {
				t.Fatalf("the folder was touched: %q %v", b, err)
			}
		})
	}
}

// ---------------------------------------------------------------- I1-4

func TestInstallHonoursSextileDirWithSpacesAndQuotes(t *testing.T) {
	for _, base := range []string{"my charts dir", "it's mine"} {
		t.Run(base, func(t *testing.T) {
			r := newRig(t, true, true)
			dir := filepath.Join(r.work, base, "sextile here")
			res := r.runScript("SEXTILE_DIR=" + dir)
			if res.code != 0 {
				t.Fatalf("exit %d\n%s%s", res.code, res.stdout, res.stderr)
			}
			clones := r.callsOf("clone")
			if len(clones) != 1 || clones[0][len(clones[0])-1] != dir {
				t.Fatalf("the clone's target is not the one path %q: %q", dir, clones)
			}
			if r.exists(filepath.Join(r.home, "sextile")) {
				t.Fatal("it also wrote ~/sextile")
			}
			last := res.lastLine()
			const tail = " && ./sextile"
			if !strings.HasPrefix(last, "cd ") || !strings.HasSuffix(last, tail) {
				t.Fatalf("last line %q", last)
			}
			word := strings.TrimSuffix(strings.TrimPrefix(last, "cd "), tail)
			out, err := exec.Command(look(t, "sh"), "-c", "printf %s "+word).Output()
			if err != nil || string(out) != dir {
				t.Fatalf("a shell reads %q back as %q (%v), want %q", word, out, err, dir)
			}
		})
	}
}

// ---------------------------------------------------------------- I1-5

var datedHeading = regexp.MustCompile(`(?m)^## \[(\d+\.\d+\.\d+)\] - \d{4}-\d{2}-\d{2}\s*$`)

// newestDated is the highest version among the dated headings, wherever they
// sit in the file.
func newestDated(changelog string) []string {
	var best []string
	var bestV [3]int
	for _, m := range datedHeading.FindAllStringSubmatch(changelog, -1) {
		var v [3]int
		fmt.Sscanf(m[1], "%d.%d.%d", &v[0], &v[1], &v[2])
		if best == nil || v[0] > bestV[0] || v[0] == bestV[0] && (v[1] > bestV[1] || v[1] == bestV[1] && v[2] > bestV[2]) {
			best, bestV = m, v
		}
	}
	return best
}

func TestInstallTagIsTheNewestDatedChangelogSection(t *testing.T) {
	changelog := repoFile(t, "CHANGELOG.md")
	m := newestDated(changelog)
	if m == nil {
		t.Fatal("CHANGELOG.md has no dated section")
	}
	if g := newestDated(changelog + "\n## [0.2.0] - 2026-11-01\n"); g == nil || g[1] != "0.2.0" {
		t.Fatalf("a dated 0.2.0 heading at the end of the file is not the newest: %v", g)
	}
	script := repoFile(t, "scripts/install")
	got := regexp.MustCompile(`(?m)^TAG=(\S+)$`).FindStringSubmatch(script)
	if got == nil || got[1] != "v"+m[1] {
		t.Fatalf("scripts/install says TAG=%v; CHANGELOG.md's newest dated section is %s, so a release bumps the installer", got, "v"+m[1])
	}
	if got[1] != tag {
		t.Fatalf("these tests expect %s, scripts/install and CHANGELOG.md say %s: update the tests with the release", tag, got[1])
	}
	// The URLs a reader runs carry the same tag.
	want := "https://raw.githubusercontent.com/cruiserxyz/sextile/" + got[1] + "/scripts/install"
	if !strings.Contains(script, want) {
		t.Fatalf("scripts/install's head comment does not show %s", want)
	}
	if n := strings.Count(repoFile(t, "README.md"), want); n != 1 {
		t.Fatalf("README.md shows %s %d times, want 1 (the one-liner)", want, n)
	}
}

// ---------------------------------------------------------------- I1-6

func TestInstallIsPOSIXShAndRunsUnderEveryShell(t *testing.T) {
	st, err := os.Stat(scriptPath(t))
	if err != nil || st.Mode().Perm()&0o111 == 0 {
		t.Fatalf("scripts/install must be executable: %v %v", st, err)
	}
	if first := strings.SplitN(repoFile(t, "scripts/install"), "\n", 2)[0]; first != "#!/bin/sh" {
		t.Fatalf("first line %q, want #!/bin/sh", first)
	}
	if sc, err := exec.LookPath("shellcheck"); err == nil {
		if out, err := exec.Command(sc, "-s", "sh", scriptPath(t)).CombinedOutput(); err != nil {
			t.Fatalf("shellcheck:\n%s", out)
		}
	}
	for _, name := range []string{"dash", "bash", "sh"} {
		t.Run(name, func(t *testing.T) {
			shell, err := exec.LookPath(name)
			if err != nil {
				if name == "dash" {
					t.Skip("dash is not installed here; CI has it")
				}
				t.Fatal(err)
			}
			if out, err := exec.Command(shell, "-n", scriptPath(t)).CombinedOutput(); err != nil {
				t.Fatalf("%s -n: %v\n%s", name, err, out)
			}
			r := newRig(t, true, true)
			res := r.run(shell, nil, nil, scriptPath(t))
			if res.code != 0 || res.lastLine() != "cd ~/sextile && ./sextile" {
				t.Fatalf("%s: exit %d\n%s%s", name, res.code, res.stdout, res.stderr)
			}
		})
	}
}

func TestInstallUsesNoBashisms(t *testing.T) {
	banned := map[string]*regexp.Regexp{
		"[[ ]]":               regexp.MustCompile(`\[\[`),
		"local":               regexp.MustCompile(`(^|[;&|]\s*)local\s`),
		"function keyword":    regexp.MustCompile(`^\s*function\s`),
		"eval":                regexp.MustCompile(`\beval\b`),
		"array":               regexp.MustCompile(`\w+=\(`),
		"here-string":         regexp.MustCompile(`<<<`),
		"process substit.":    regexp.MustCompile(`<\(|>\(`),
		"${x//y}":             regexp.MustCompile(`\$\{[^}]*//`),
		"echo -e":             regexp.MustCompile(`\becho\s+-[en]`),
		"sudo or package mgr": regexp.MustCompile(`\b(sudo|apt|apt-get|brew|yum|dnf|pacman)\b`),
	}
	for n, l := range strings.Split(repoFile(t, "scripts/install"), "\n") {
		code := strings.TrimSpace(l)
		if strings.HasPrefix(code, "#") {
			continue
		}
		for name, re := range banned {
			if re.MatchString(code) {
				t.Errorf("scripts/install:%d uses %s: %s", n+1, name, code)
			}
		}
	}
}

// ---------------------------------------------------------------- I1-7

func TestInstallNeverReadsStdin(t *testing.T) {
	t.Run("dev null", func(t *testing.T) {
		r := newRig(t, true, true)
		f, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		cmd := exec.Command(look(t, "sh"), scriptPath(t))
		cmd.Dir = r.work
		cmd.Env = []string{"PATH=" + r.bin, "HOME=" + r.home, "FAKE_LOG=" + r.log, "FAKE_STDIN=" + r.stdin}
		cmd.Stdin = f
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
	})
	t.Run("a command waits on stdin", func(t *testing.T) {
		r := newRig(t, true, true)
		marker := filepath.Join(r.work, "RAN")
		res := r.run(look(t, "sh"), []byte("touch "+marker+"\n"), nil, scriptPath(t))
		if res.code != 0 {
			t.Fatalf("exit %d\n%s%s", res.code, res.stdout, res.stderr)
		}
		if r.exists(marker) {
			t.Fatal("the command on stdin ran")
		}
		if got := r.leaked(); got != "" {
			t.Fatalf("git or docker was handed the installer's stdin: %q", got)
		}
	})
	t.Run("the script is stdin", func(t *testing.T) {
		r := newRig(t, true, true)
		res := r.run(look(t, "sh"), []byte(repoFile(t, "scripts/install")), nil)
		if res.code != 0 || res.lastLine() != "cd ~/sextile && ./sextile" {
			t.Fatalf("exit %d\n%s%s", res.code, res.stdout, res.stderr)
		}
		if got := r.leaked(); got != "" {
			t.Fatalf("git or docker was handed the installer's stdin: %q", got)
		}
	})
}

// ---------------------------------------------------------------- the download cut short

func TestInstallRunsNothingWhenTheDownloadIsCut(t *testing.T) {
	script := repoFile(t, "scripts/install")
	lines := strings.SplitAfter(script, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if last := lines[len(lines)-1]; last != "main \"$@\"\n" {
		t.Fatalf("the last line is %q; it must be exactly main \"$@\", the one call", last)
	}
	cut := 0
	for n := 1; n < len(lines); n++ {
		r := newRig(t, true, true)
		prefix := strings.Join(lines[:n], "")
		res := r.run(look(t, "sh"), []byte(prefix), nil)
		if c := r.calls(); len(c) != 0 {
			t.Fatalf("a download cut after line %d ran %q (exit %d)\n%s", n, c, res.code, res.stderr)
		}
		if r.exists(filepath.Join(r.home, "sextile")) || res.stdout != "" {
			t.Fatalf("a download cut after line %d wrote or printed: %q", n, res.stdout)
		}
		cut++
	}
	if cut < 20 {
		t.Fatalf("only %d cuts tried", cut)
	}
	r := newRig(t, true, true)
	if res := r.run(look(t, "sh"), []byte(script), nil); res.code != 0 || len(r.callsOf("clone")) != 1 {
		t.Fatalf("the whole script did not install: exit %d\n%s", res.code, res.stderr)
	}
}

// ---------------------------------------------------------------- I1-8

func TestInstallNamesOnlyTheHostsItNeeds(t *testing.T) {
	allowed := map[string]bool{
		"github.com":                true,
		"raw.githubusercontent.com": true,
		"docs.docker.com":           true,
	}
	hosts := 0
	for _, u := range regexp.MustCompile(`https?://[^\s"'$)]+`).FindAllString(repoFile(t, "scripts/install"), -1) {
		p, err := url.Parse(u)
		if err != nil || !allowed[p.Host] {
			t.Errorf("scripts/install names %s", u)
		}
		if p.Host == "github.com" && !strings.HasPrefix(p.Path, "/cruiserxyz/sextile") {
			t.Errorf("github.com URL outside this repository: %s", u)
		}
		hosts++
	}
	if hosts == 0 {
		t.Fatal("no URL found; the pattern is wrong")
	}
}

// ---------------------------------------------------------------- real git

// TestInstallWithRealGit runs the script against the real git, with the
// GitHub URL rewritten to a local repository, so the argv recorder is not the
// only witness: the clone is shallow, at the tag, with the origin the README
// names, and an existing clone moves to the tag without losing anything.
func TestInstallWithRealGit(t *testing.T) {
	git := look(t, "git")
	newReal := func(t *testing.T) (*rig, string, func(dir string, args ...string) string) {
		r := newRig(t, false, true)
		src := filepath.Join(filepath.Dir(r.bin), "source")
		env := append(os.Environ(),
			"HOME="+r.home, "XDG_CONFIG_HOME="+filepath.Join(r.home, ".config"), "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@example.invalid",
			"GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@example.invalid")
		g := func(dir string, args ...string) string {
			t.Helper()
			cmd := exec.Command(git, append([]string{"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
			cmd.Dir = dir
			cmd.Env = env
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
			return strings.TrimSpace(string(out))
		}
		if err := os.MkdirAll(src, 0o755); err != nil {
			t.Fatal(err)
		}
		g(src, "init", "-q", "-b", "main")
		for _, f := range []string{"one.txt", "two.txt"} {
			if err := os.WriteFile(filepath.Join(src, f), []byte(f+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			g(src, "add", "-A")
			g(src, "commit", "-q", "-m", "add "+f)
			if f == "one.txt" {
				g(src, "tag", tag)
			}
		}
		cfg := fmt.Sprintf("[url %q]\n\tinsteadOf = %s\n", "file://"+src, repoURL)
		if err := os.WriteFile(filepath.Join(r.home, ".gitconfig"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
		return r, src, g
	}
	path := func(r *rig) string { return r.bin + ":" + filepath.Dir(git) + ":/usr/bin:/bin" }
	realGitEnv := []string{"XDG_CONFIG_HOME=", "GIT_CONFIG_NOSYSTEM=1"}

	t.Run("fresh clone", func(t *testing.T) {
		r, _, g := newReal(t)
		res := r.runPath(path(r), look(t, "sh"), nil, realGitEnv, scriptPath(t))
		if res.code != 0 {
			t.Fatalf("exit %d\n%s%s", res.code, res.stdout, res.stderr)
		}
		dir := filepath.Join(r.home, "sextile")
		eq(t, "describe", g(dir, "describe", "--tags", "--exact-match"), tag)
		eq(t, "shallow", g(dir, "rev-parse", "--is-shallow-repository"), "true")
		eq(t, "commits", g(dir, "rev-list", "--count", "HEAD"), "1")
		eq(t, "origin", g(dir, "config", "--get", "remote.origin.url"), repoURL)
		if r.exists(filepath.Join(dir, "two.txt")) {
			t.Fatal("the clone has a file from after the tag")
		}
		eq(t, "the last line", res.lastLine(), "cd ~/sextile && ./sextile")
	})

	t.Run("existing clone moves to the tag", func(t *testing.T) {
		r, _, g := newReal(t)
		dir := filepath.Join(r.home, "sextile")
		g(r.home, "clone", "-q", repoURL, dir)
		if !r.exists(filepath.Join(dir, "two.txt")) {
			t.Fatal("setup: the clone should start ahead of the tag")
		}
		mine := filepath.Join(dir, "mine.txt")
		if err := os.WriteFile(mine, []byte("keep\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		res := r.runPath(path(r), look(t, "sh"), nil, realGitEnv, scriptPath(t))
		if res.code != 0 {
			t.Fatalf("exit %d\n%s%s", res.code, res.stdout, res.stderr)
		}
		eq(t, "describe", g(dir, "describe", "--tags", "--exact-match"), tag)
		eq(t, "full history kept", g(dir, "rev-list", "--count", "main"), "2")
		if b, err := os.ReadFile(mine); err != nil || string(b) != "keep\n" {
			t.Fatalf("an untracked file of the user's changed: %q %v", b, err)
		}
	})

	t.Run("local changes in the way are kept", func(t *testing.T) {
		r, _, g := newReal(t)
		dir := filepath.Join(r.home, "sextile")
		g(r.home, "clone", "-q", repoURL, dir)
		// two.txt is tracked on main and absent at the tag, so a careless
		// checkout would delete the user's edit of it.
		two := filepath.Join(dir, "two.txt")
		if err := os.WriteFile(two, []byte("my uncommitted edit\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		res := r.runPath(path(r), look(t, "sh"), nil, realGitEnv, scriptPath(t))
		if b, _ := os.ReadFile(two); string(b) != "my uncommitted edit\n" {
			t.Fatalf("an edit of the user's was lost: %q (exit %d)\n%s", b, res.code, res.stderr)
		}
		if res.code == 0 && strings.Contains(res.stdout, "./sextile") {
			t.Fatalf("it reported success with the edit in the way:\n%s", res.stdout)
		}
	})
}
