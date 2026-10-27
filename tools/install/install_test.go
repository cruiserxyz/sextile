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

const repoURL = "https://github.com/cruiserxyz/sextile.git"

// tag is the release scripts/install clones, read from its TAG= line, so a
// release bumps the script and these tests follow; I1-5 ties that line to
// CHANGELOG.md's newest dated section.
var tag = func() string {
	b, err := os.ReadFile(filepath.Join("..", "..", "scripts", "install"))
	if err != nil {
		panic(err)
	}
	m := regexp.MustCompile(`(?m)^TAG=(v[0-9.]+)$`).FindSubmatch(b)
	if m == nil {
		panic("scripts/install has no TAG= line")
	}
	return string(m[1])
}()

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
	// The script writes a file and a folder, so it needs the tools that do;
	// they are the real ones, linked in, because PATH holds only this folder.
	for _, n := range []string{"mkdir", "chmod", "mv"} {
		if err := os.Symlink(look(t, n), filepath.Join(r.bin, n)); err != nil {
			t.Fatal(err)
		}
	}
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

// appDir is where the script clones when SEXTILE_DIR and XDG_DATA_HOME are unset.
func (r *rig) appDir() string {
	return filepath.Join(r.home, ".local", "share", "sextile", "app")
}

// wrapperPath is the command the script writes.
func (r *rig) wrapperPath() string {
	return filepath.Join(r.home, ".local", "bin", "sextile")
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

func TestInstallNeedsGitButOnlyWarnsAboutDocker(t *testing.T) {
	t.Run("no git refuses", func(t *testing.T) {
		r := newRig(t, false, true)
		res := r.runScript()
		if res.code != 2 {
			t.Fatalf("exit %d, want 2\n%s%s", res.code, res.stdout, res.stderr)
		}
		if !strings.Contains(res.stderr, "git was not found") {
			t.Fatalf("stderr lacks the git sentence:\n%s", res.stderr)
		}
		if n := strings.Count(strings.TrimRight(res.stderr, "\n"), "\n"); n != 0 || res.stdout != "" {
			t.Fatalf("want one sentence on stderr and nothing on stdout:\nstdout %q\nstderr %q", res.stdout, res.stderr)
		}
		if r.exists(filepath.Join(r.home, ".local")) || r.exists(filepath.Join(r.home, "sextile")) {
			t.Fatal("a folder was written before git was found")
		}
		if g := r.callsOf("clone"); len(g) != 0 {
			t.Fatalf("git cloned: %q", g)
		}
	})
	for _, c := range []struct {
		name string
		dock bool
		env  []string
		want string
	}{
		{"no docker", false, nil, "Docker was not found"},
		{"daemon down", true, []string{"FAKE_DOCKER_INFO_EXIT=1"}, "daemon did not answer"},
	} {
		t.Run(c.name+" warns and installs", func(t *testing.T) {
			r := newRig(t, true, c.dock)
			res := r.runScript(c.env...)
			if res.code != 0 {
				t.Fatalf("exit %d, want 0\n%s%s", res.code, res.stdout, res.stderr)
			}
			for _, w := range []string{c.want, "https://docs.docker.com/get-docker/", "sextile --no-docker", "go and astrolog", "sextile doctor"} {
				if !strings.Contains(res.stderr, w) {
					t.Fatalf("the warning lacks %q:\n%s", w, res.stderr)
				}
			}
			if len(r.callsOf("clone")) != 1 || !r.exists(r.wrapperPath()) {
				t.Fatalf("the install did not go on: %q", r.calls())
			}
			eq(t, "the last line", res.lastLine(), "sextile")
		})
	}
	t.Run("a working docker says nothing", func(t *testing.T) {
		r := newRig(t, true, true)
		if res := r.runScript(); res.code != 0 || res.stderr != "" {
			t.Fatalf("exit %d, stderr %q", res.code, res.stderr)
		}
	})
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
	dir := r.appDir()
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
	eq(t, "the last line", res.lastLine(), "sextile")
	if !strings.Contains(res.stdout, "sextile doctor") || strings.Contains(res.stdout, "./sextile") || strings.Contains(res.stdout, "cd ") {
		t.Fatalf("stdout must name sextile doctor and sextile, with no cd and no ./:\n%s", res.stdout)
	}
	if r.exists(filepath.Join(r.home, "sextile")) {
		t.Fatal("it also wrote ~/sextile")
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
	if strings.Contains(res.stdout, "To start Sextile") || r.exists(r.wrapperPath()) {
		t.Fatalf("it told the reader to start, or wrote the command for, a wrong tree:\n%s", res.stdout)
	}
	r = newRig(t, true, true)
	if res := r.runScript("FAKE_CLONE_EXIT=128"); res.code != 1 || strings.Contains(res.stdout, "To start Sextile") || r.exists(r.wrapperPath()) {
		t.Fatalf("a failed clone: exit %d\n%s%s", res.code, res.stdout, res.stderr)
	}
}

// ---------------------------------------------------------------- I1-3

func TestInstallUpdatesAnExistingClone(t *testing.T) {
	for _, origin := range []string{repoURL, strings.TrimSuffix(repoURL, ".git")} {
		t.Run(origin, func(t *testing.T) {
			r := newRig(t, true, true)
			dir := r.appDir()
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
			eq(t, "the last line", res.lastLine(), "sextile")
			if !r.exists(r.wrapperPath()) {
				t.Fatal("the command was not written for the updated clone")
			}
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
			dir := r.appDir()
			if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
				t.Fatal(err)
			}
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
			if r.exists(r.wrapperPath()) {
				t.Fatal("a command was written for a folder that is not ours")
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
			if r.exists(filepath.Join(r.home, "sextile")) || r.exists(r.appDir()) {
				t.Fatal("it also wrote the default folders")
			}
			eq(t, "the last line", res.lastLine(), "sextile")
			// The command the script wrote must reach the launcher in that
			// folder, whatever the folder's name holds: stand a launcher in
			// the fake clone and run the wrapper.
			launcher := filepath.Join(dir, "sextile")
			if err := os.WriteFile(launcher, []byte("#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(r.wrapperPath(), "doctor", "--charts-dir", "a b'c")
			cmd.Env = []string{"PATH=" + r.bin, "HOME=" + r.home}
			out, err := cmd.CombinedOutput()
			if err != nil || string(out) != "doctor\n--charts-dir\na b'c\n" {
				t.Fatalf("the wrapper ran %q (%v), want the launcher in %q with the arguments kept", out, err, dir)
			}
		})
	}
}

// ---------------------------------------------------------------- the command

const wrapperMarker = "# Sextile command, written by its install script"

func wrapperText(dir string) string {
	q := "'" + strings.ReplaceAll(dir+"/sextile", "'", `'\''`) + "'"
	return "#!/bin/sh\n" + wrapperMarker + "\nexec " + q + " \"$@\"\n"
}

// rcFiles writes a startup file of each shell the script might name, so a test
// can prove none of them changed.
func rcFiles(t *testing.T, home string) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, n := range []string{".bashrc", ".zshrc", ".profile", ".bash_profile", ".zprofile"} {
		body := "# mine: " + n + "\nalias ll='ls -l'\n"
		if err := os.WriteFile(filepath.Join(home, n), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		files[n] = body
	}
	return files
}

func sameFiles(t *testing.T, home string, want map[string]string) {
	t.Helper()
	for n, body := range want {
		if b, err := os.ReadFile(filepath.Join(home, n)); err != nil || string(b) != body {
			t.Fatalf("%s changed: %q %v", n, b, err)
		}
	}
	ents, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if _, ok := want[e.Name()]; !ok && strings.HasPrefix(e.Name(), ".") && strings.Contains(e.Name(), "rc") {
			t.Fatalf("a startup file appeared: %s", e.Name())
		}
	}
}

func TestInstallWritesWrapperAndPrintsPathLine(t *testing.T) {
	t.Run("clone under XDG_DATA_HOME, wrapper exact, PATH line, rc files untouched", func(t *testing.T) {
		r := newRig(t, true, true)
		rc := rcFiles(t, r.home)
		data := filepath.Join(r.work, "data home")
		res := r.runScript("XDG_DATA_HOME="+data, "SHELL=/bin/zsh")
		if res.code != 0 {
			t.Fatalf("exit %d\n%s%s", res.code, res.stdout, res.stderr)
		}
		dir := filepath.Join(data, "sextile", "app")
		clones := r.callsOf("clone")
		if len(clones) != 1 || clones[0][len(clones[0])-1] != dir {
			t.Fatalf("the clone is not in %q: %q", dir, clones)
		}
		b, err := os.ReadFile(r.wrapperPath())
		if err != nil {
			t.Fatal(err)
		}
		eq(t, "the wrapper", string(b), wrapperText(dir))
		if st, _ := os.Stat(r.wrapperPath()); st.Mode().Perm() != 0o755 {
			t.Fatalf("the wrapper's mode is %v, want 0755", st.Mode().Perm())
		}
		for _, w := range []string{`export PATH="$HOME/.local/bin:$PATH"`, "~/.zshrc", "~/.local/bin/sextile"} {
			if !strings.Contains(res.stdout, w) {
				t.Fatalf("stdout lacks %q:\n%s", w, res.stdout)
			}
		}
		eq(t, "the last line", res.lastLine(), "sextile")
		sameFiles(t, r.home, rc)
	})

	t.Run("the file named follows the shell", func(t *testing.T) {
		for shell, file := range map[string]string{"/bin/bash": "~/.bashrc", "/usr/bin/zsh": "~/.zshrc", "/bin/dash": "~/.profile", "": "~/.profile"} {
			r := newRig(t, true, true)
			rc := rcFiles(t, r.home)
			env := []string{"SHELL=" + shell}
			res := r.runScript(env...)
			if res.code != 0 || !strings.Contains(res.stdout, "put this line in "+file) {
				t.Fatalf("SHELL=%q: exit %d, want the file %s named\n%s%s", shell, res.code, file, res.stdout, res.stderr)
			}
			sameFiles(t, r.home, rc)
		}
	})

	t.Run("no PATH line when ~/.local/bin is on PATH", func(t *testing.T) {
		r := newRig(t, true, true)
		res := r.runPath(r.bin+":"+filepath.Join(r.home, ".local", "bin"), look(t, "sh"), nil, nil, scriptPath(t))
		if res.code != 0 || strings.Contains(res.stdout, "export PATH") || strings.Contains(res.stdout, "right now") {
			t.Fatalf("exit %d, it explained a PATH that is fine:\n%s%s", res.code, res.stdout, res.stderr)
		}
		eq(t, "the last line", res.lastLine(), "sextile")
	})

	t.Run("a relative XDG_DATA_HOME is not a place", func(t *testing.T) {
		r := newRig(t, true, true)
		if res := r.runScript("XDG_DATA_HOME=rel/data"); res.code != 0 {
			t.Fatalf("exit %d\n%s%s", res.code, res.stdout, res.stderr)
		}
		clones := r.callsOf("clone")
		if len(clones) != 1 || clones[0][len(clones[0])-1] != r.appDir() {
			t.Fatalf("the clone is not in %q: %q", r.appDir(), clones)
		}
	})

	t.Run("one of its own wrappers is replaced, and the command follows SEXTILE_DIR", func(t *testing.T) {
		r := newRig(t, true, true)
		if res := r.runScript(); res.code != 0 {
			t.Fatalf("first: exit %d\n%s%s", res.code, res.stdout, res.stderr)
		}
		other := filepath.Join(r.work, "elsewhere")
		if res := r.runScript("SEXTILE_DIR=" + other); res.code != 0 {
			t.Fatalf("second: exit %d\n%s%s", res.code, res.stdout, res.stderr)
		}
		b, err := os.ReadFile(r.wrapperPath())
		if err != nil || string(b) != wrapperText(other) {
			t.Fatalf("the wrapper is %q (%v), want it to name %s", b, err, other)
		}
	})
}

func TestInstallRefusesAForeignCommand(t *testing.T) {
	setups := map[string]func(t *testing.T, r *rig, path string){
		"someone else's script": func(t *testing.T, r *rig, path string) {
			if err := os.WriteFile(path, []byte("#!/bin/sh\necho mine\n"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"the marker on the wrong line": func(t *testing.T, r *rig, path string) {
			if err := os.WriteFile(path, []byte(wrapperMarker+"\n#!/bin/sh\nexec true\n"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"a near marker": func(t *testing.T, r *rig, path string) {
			if err := os.WriteFile(path, []byte("#!/bin/sh\n"+wrapperMarker+" \nexec true\n"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"an empty file": func(t *testing.T, r *rig, path string) {
			if err := os.WriteFile(path, nil, 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"a symlink to the launcher": func(t *testing.T, r *rig, path string) {
			target := filepath.Join(r.work, "launcher")
			if err := os.WriteFile(target, []byte("#!/bin/bash\n# ./sextile: the launcher\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
		},
		"a dangling symlink": func(t *testing.T, r *rig, path string) {
			if err := os.Symlink(filepath.Join(r.work, "nothing"), path); err != nil {
				t.Fatal(err)
			}
		},
		"a folder": func(t *testing.T, r *rig, path string) {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, setup := range setups {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, true, true)
			if err := os.MkdirAll(filepath.Dir(r.wrapperPath()), 0o755); err != nil {
				t.Fatal(err)
			}
			setup(t, r, r.wrapperPath())
			before, _ := os.Lstat(r.wrapperPath())
			var content []byte
			if before.Mode().IsRegular() {
				content, _ = os.ReadFile(r.wrapperPath())
			}
			res := r.runScript()
			if res.code == 0 {
				t.Fatalf("it installed over %s\n%s%s", name, res.stdout, res.stderr)
			}
			if res.code != 2 || res.stdout != "" || !strings.Contains(res.stderr, r.wrapperPath()) {
				t.Fatalf("want exit 2, nothing on stdout, and the file named: exit %d\nstdout %q\nstderr %q", res.code, res.stdout, res.stderr)
			}
			if n := len(r.callsOf("clone")) + len(r.callsOf("fetch")) + len(r.callsOf("checkout")); n != 0 {
				t.Fatalf("git changed something before the refusal: %q", r.calls())
			}
			if r.exists(r.appDir()) {
				t.Fatal("the clone was made although the command could not be written")
			}
			after, err := os.Lstat(r.wrapperPath())
			if err != nil || after.Mode() != before.Mode() || after.Size() != before.Size() {
				t.Fatalf("%s changed: %v %v", r.wrapperPath(), after, err)
			}
			if before.Mode().IsRegular() {
				if b, _ := os.ReadFile(r.wrapperPath()); string(b) != string(content) {
					t.Fatalf("the file's content changed: %q", b)
				}
			}
		})
	}
}

func TestInstallNotesTheOldClone(t *testing.T) {
	old := func(r *rig) string { return filepath.Join(r.home, "sextile") }
	t.Run("an old clone is named and left alone", func(t *testing.T) {
		r := newRig(t, true, true)
		r.mkClone(old(r), repoURL, "v0.1.0")
		mine := filepath.Join(old(r), "my-notes.txt")
		if err := os.WriteFile(mine, []byte("keep\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		res := r.runScript()
		if res.code != 0 {
			t.Fatalf("exit %d\n%s%s", res.code, res.stdout, res.stderr)
		}
		var line string
		for _, l := range strings.Split(res.stdout, "\n") {
			if strings.Contains(l, "~/sextile") {
				line = l
			}
		}
		for _, w := range []string{"no longer used", "left as it is", "remove"} {
			if !strings.Contains(line, w) {
				t.Fatalf("the line about ~/sextile lacks %q: %q\n%s", w, line, res.stdout)
			}
		}
		for _, verb := range []string{"fetch", "checkout"} {
			for _, c := range r.callsOf(verb) {
				if strings.Contains(strings.Join(c, " "), old(r)) {
					t.Fatalf("git %s touched the old clone: %q", verb, c)
				}
			}
		}
		if b, err := os.ReadFile(mine); err != nil || string(b) != "keep\n" {
			t.Fatalf("a file in the old clone changed: %q %v", b, err)
		}
		if b, _ := os.ReadFile(filepath.Join(old(r), ".git", "fake-tag")); string(b) != "v0.1.0\n" {
			t.Fatalf("the old clone moved to %q", b)
		}
		eq(t, "the last line", res.lastLine(), "sextile")
	})
	t.Run("no note when SEXTILE_DIR is set", func(t *testing.T) {
		r := newRig(t, true, true)
		r.mkClone(old(r), repoURL, "v0.1.0")
		res := r.runScript("SEXTILE_DIR=" + filepath.Join(r.work, "here"))
		if res.code != 0 || strings.Contains(res.stdout, "~/sextile") {
			t.Fatalf("exit %d\n%s%s", res.code, res.stdout, res.stderr)
		}
	})
	t.Run("no note for a folder that is not a Sextile clone", func(t *testing.T) {
		r := newRig(t, true, true)
		r.mkClone(old(r), "https://example.invalid/other.git", "main")
		res := r.runScript()
		if res.code != 0 || strings.Contains(res.stdout, "~/sextile") {
			t.Fatalf("exit %d\n%s%s", res.code, res.stdout, res.stderr)
		}
	})
	t.Run("no note without an old folder", func(t *testing.T) {
		r := newRig(t, true, true)
		res := r.runScript()
		if res.code != 0 || strings.Contains(res.stdout, "~/sextile") {
			t.Fatalf("exit %d\n%s%s", res.code, res.stdout, res.stderr)
		}
	})
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
			if res.code != 0 || res.lastLine() != "sextile" {
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
		if res.code != 0 || res.lastLine() != "sextile" {
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
		if r.exists(filepath.Join(r.home, "sextile")) || r.exists(filepath.Join(r.home, ".local")) || res.stdout != "" {
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
		dir := r.appDir()
		eq(t, "describe", g(dir, "describe", "--tags", "--exact-match"), tag)
		eq(t, "shallow", g(dir, "rev-parse", "--is-shallow-repository"), "true")
		eq(t, "commits", g(dir, "rev-list", "--count", "HEAD"), "1")
		eq(t, "origin", g(dir, "config", "--get", "remote.origin.url"), repoURL)
		if r.exists(filepath.Join(dir, "two.txt")) {
			t.Fatal("the clone has a file from after the tag")
		}
		eq(t, "the last line", res.lastLine(), "sextile")
	})

	t.Run("existing clone moves to the tag", func(t *testing.T) {
		r, _, g := newReal(t)
		dir := r.appDir()
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
		dir := r.appDir()
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
		if res.code == 0 && strings.Contains(res.stdout, "To start Sextile") {
			t.Fatalf("it reported success with the edit in the way:\n%s", res.stdout)
		}
	})
}
