// Package launcher holds the tests of ./sextile, the Docker launcher.
//
// Every test runs a COPY of the script from a temporary "clone" with HOME,
// XDG_CONFIG_HOME and PATH in the temporary directory. PATH holds a fake
// `docker` that records its arguments and answers from environment variables;
// no test touches the real home, a real docker or the network.
package launcher

import (
	"bytes"
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite tools/launcher/testdata/*.golden from the real script")

// fakeDocker records one record per call to $FAKE_DOCKER_LOG (each argument
// followed by NUL, the record ended by 0x1e) and answers from the environment:
//
//	FAKE_DOCKER_SERVER   server version            (default 29.7.2)
//	FAKE_DOCKER_IMAGE    labelled|unlabelled|absent (default absent)
//	FAKE_DOCKER_COMMIT   the commit label a labelled image carries
//	FAKE_DOCKER_EXPECT_IMAGE if set, `image inspect` of any other image fails loudly (exit 99, FAKE-DOCKER-MISMATCH)
//	FAKE_DOCKER_FAIL     a verb ("build", "run", "version", "image inspect")
//	FAKE_DOCKER_DENIED   1: `version` says permission denied
//	FAKE_DOCKER_ENDPOINT the context's endpoint     (default unix:///var/run/docker.sock)
//	FAKE_DOCKER_RUN_EXIT run's exit status          (default 0)
//	FAKE_DOCKER_RUN_HOLD a path: `run` does not return until it exists
//	FAKE_DOCKER_PS_FAIL_ID an image id whose `ps -a --filter ancestor=<id>` fails
//
// Images are STATE, not canned output: the file $FAKE_DOCKER_LOG.state, which
// `image rm` rewrites, holds one line per fact:
//
//	img <id> <1|0> <repo:tag>...   an image and all its tags; 1 when the IMAGE carries the launcher label
//	ctr <id>                       a container (running or stopped) made from image <id>
//
// `image ls` lists tags (only those of labelled images with exactly
// --filter label=io.github.cruiserxyz.sextile.launcher=1; any other filter
// exits 2; the ID is in the output only when the --format asks for it). `image rm <repo:tag>`
// untags, and deletes the image with its last tag, which fails while a
// container uses it. `image rm <id>` fails, as real Docker does, when the id
// has more than one tag or a container uses it. `ps -a --filter ancestor=<id>`
// reports the containers. Set it up with rig.setImages.
//
//	FAKE_DOCKER_PROBE    a path; build and run note into $FAKE_DOCKER_LOG.probe whether it exists
const fakeDocker = `#!/usr/bin/env bash
{ for a in "$@"; do printf '%s\0' "$a"; done; printf '\036'; } >> "$FAKE_DOCKER_LOG"
inspected() { local x=; [ -f "$FAKE_DOCKER_LOG.inspected" ] && x=$(<"$FAKE_DOCKER_LOG.inspected"); printf '%s' "$x"; }
S=$FAKE_DOCKER_LOG.state
LINES=()
load() {
  LINES=()
  [ -f "$S" ] || return 0
  while IFS= read -r l; do [ -n "$l" ] && LINES+=("$l"); done < "$S"
}
save() {
  : > "$S"
  for ((j = 0; j < ${#LINES[@]}; j++)); do printf '%s\n' "${LINES[$j]}" >> "$S"; done
}
drop_line() {
  local keep=() j
  for ((j = 0; j < ${#LINES[@]}; j++)); do [ "$j" -eq "$1" ] || keep+=("${LINES[$j]}"); done
  LINES=("${keep[@]}")
}
ntags() { local n=0 j kind id lab tags; for ((j = 0; j < ${#LINES[@]}; j++)); do read -r kind id lab tags <<< "${LINES[$j]}"; if [ "$kind" = img ] && [ "$id" = "$1" ]; then set -- $tags "$1"; n=$(($# - 1)); break; fi; done; echo "$n"; }
nctrs() { local n=0 j kind id rest; for ((j = 0; j < ${#LINES[@]}; j++)); do read -r kind id rest <<< "${LINES[$j]}"; [ "$kind" = ctr ] && [ "$id" = "$1" ] && n=$((n + 1)); done; echo "$n"; }
verb=$1
key=$1
case $1 in image | context) key="$1 $2" ;; esac
if [ -n "${FAKE_DOCKER_FAIL-}" ] && { [ "$FAKE_DOCKER_FAIL" = "$verb" ] || [ "$FAKE_DOCKER_FAIL" = "$key" ]; }; then
  case $verb in
    version)
      echo "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?" >&2 ;;
    build)
      echo "BUILD-ERROR-LINE: step 3 failed" >&2 ;;
    *) echo "fake docker: $key failed" >&2 ;;
  esac
  exit 1
fi
if [ -n "${FAKE_DOCKER_PROBE-}" ] && { [ "$verb" = build ] || [ "$verb" = run ]; }; then
  if [ -e "$FAKE_DOCKER_PROBE" ]; then p=yes; else p=no; fi
  printf '%s:%s\n' "$verb" "$p" >> "$FAKE_DOCKER_LOG.probe"
fi
case $verb in
  version)
    if [ "${FAKE_DOCKER_DENIED-}" = 1 ]; then
      echo "permission denied while trying to connect to the Docker daemon socket at unix:///var/run/docker.sock" >&2
      exit 1
    fi
    if [ "${2-}" = --format ]; then
      echo "${FAKE_DOCKER_SERVER:-29.7.2}"
    else
      printf 'Client: Docker Engine\nServer: %s\n' "${FAKE_DOCKER_ENGINE:-Docker Engine}"
    fi ;;
  context)
    echo "${FAKE_DOCKER_ENDPOINT-unix:///var/run/docker.sock}" ;;
  image)
    case $2 in
      inspect)
        target=${*: -1}
        if [ -n "${FAKE_DOCKER_EXPECT_IMAGE-}" ] && [ "$target" != "$FAKE_DOCKER_EXPECT_IMAGE" ]; then
          echo "FAKE-DOCKER-MISMATCH: inspected $target, expected $FAKE_DOCKER_EXPECT_IMAGE" >&2
          exit 99
        fi
        printf '%s' "$target" > "$FAKE_DOCKER_LOG.inspected"
        case ${FAKE_DOCKER_IMAGE:-absent} in
          labelled) echo "1 ${FAKE_DOCKER_COMMIT-}" ;;
          unlabelled) echo " " ;;
          *) echo "Error: No such image" >&2; exit 1 ;;
        esac ;;
      ls)
        load
        fmt=; labelled=; prev=
        for a in "$@"; do
          case $prev in
            --format) fmt=$a ;;
            --filter)
              if [ "$a" != "label=io.github.cruiserxyz.sextile.launcher=1" ]; then
                echo "fake docker: unexpected filter $a" >&2
                exit 2
              fi
              labelled=1 ;;
          esac
          prev=$a
        done
        repo=${*: -1}
        for ((i = 0; i < ${#LINES[@]}; i++)); do
          read -r kind id lab tags <<< "${LINES[$i]}"
          [ "$kind" = img ] || continue
          [ -z "$labelled" ] || [ "$lab" = 1 ] || continue
          for ref in $tags; do
            case $ref in "$repo":*) ;; *) continue ;; esac
            case $fmt in *'{{.ID}}'*) echo "$id $ref" ;; *) echo "$ref" ;; esac
          done
        done ;;
      rm)
        load
        target=${*: -1}
        idx=-1; tid=
        for ((i = 0; i < ${#LINES[@]}; i++)); do
          read -r kind id lab tags <<< "${LINES[$i]}"
          if [ "$kind" = img ]; then
            for ref in $tags; do
              if [ "$ref" = "$target" ]; then idx=$i; tid=$id; fi
            done
          fi
        done
        if [ "$idx" -ge 0 ]; then
          if [ "$(ntags "$tid")" -le 1 ] && [ "$(nctrs "$tid")" -gt 0 ]; then
            echo "Error response from daemon: conflict: unable to delete $target (cannot be forced) - container is using its referenced image" >&2
            exit 1
          fi
          read -r kind id lab tags <<< "${LINES[$idx]}"
          left=
          for ref in $tags; do [ "$ref" = "$target" ] || left="$left $ref"; done
          if [ -z "$left" ]; then drop_line "$idx"; else LINES[$idx]="img $id $lab$left"; fi
          save
          echo "Untagged: $target"
        else
          if [ "$(ntags "$target")" -eq 0 ]; then
            echo "Error response from daemon: No such image: $target" >&2
            exit 1
          fi
          if [ "$(ntags "$target")" -gt 1 ]; then
            echo "Error response from daemon: conflict: unable to delete $target (must be forced) - image is referenced in multiple repositories" >&2
            exit 1
          fi
          if [ "$(nctrs "$target")" -gt 0 ]; then
            echo "Error response from daemon: conflict: unable to delete $target (cannot be forced) - container is using its referenced image" >&2
            exit 1
          fi
          for ((i = 0; i < ${#LINES[@]}; i++)); do
            read -r kind id lab tags <<< "${LINES[$i]}"
            if [ "$kind" = img ] && [ "$id" = "$target" ]; then drop_line "$i"; break; fi
          done
          save
          echo "Deleted: $target"
        fi ;;
    esac ;;
  ps)
    load
    for a in "$@"; do
      case $a in
        ancestor=*)
          if [ -n "${FAKE_DOCKER_PS_FAIL_ID-}" ] && [ "${a#ancestor=}" = "$FAKE_DOCKER_PS_FAIL_ID" ]; then
            echo "fake docker: ps failed for $FAKE_DOCKER_PS_FAIL_ID" >&2
            exit 1
          fi
          for ((i = 0; i < $(nctrs "${a#ancestor=}"); i++)); do echo "c0ffee$i"; done ;;
      esac
    done ;;
  build)
    # The image it names must be the one the launcher inspected.
    prev=
    for a in "$@"; do
      if [ "$prev" = -t ] && [ "$a" != "$(inspected)" ]; then
        echo "FAKE-DOCKER-MISMATCH: built $a, inspected $(inspected)" >&2
        exit 99
      fi
      prev=$a
    done
    case " $* " in *" -q "*) echo "sha256:0123456789ab" ;; *) echo "#1 [internal] load build definition" ;; esac ;;
  run)
    for a in "$@"; do
      case $a in
        localhost/sextile:*)
          if [ "$a" != "$(inspected)" ]; then
            echo "FAKE-DOCKER-MISMATCH: ran $a, inspected $(inspected)" >&2
            exit 99
          fi
          break ;;
      esac
    done
    # The container stays "up" until $FAKE_DOCKER_RUN_HOLD exists.
    if [ -n "${FAKE_DOCKER_RUN_HOLD-}" ]; then
      while [ ! -e "$FAKE_DOCKER_RUN_HOLD" ]; do sleep 0.05; done
    fi
    exit "${FAKE_DOCKER_RUN_EXIT:-0}" ;;
  *)
    echo "fake docker: unexpected verb $verb" >&2
    exit 99 ;;
esac
`

// sudoShim fails the test run if the launcher ever tries sudo.
const sudoShim = "#!/usr/bin/env bash\necho sudo >> \"$FAKE_DOCKER_LOG.sudo\"\nexit 1\n"

// tools the script may use besides docker.
var toolNames = []string{"bash", "id", "mkdir", "mv", "mktemp", "chmod", "readlink", "grep", "git", "rm", "rmdir", "sleep", "head"}

// wrapped tools can be made to fail: FAKE_TOOL_FAIL=mv, or =chmod for `chmod 600`.
var wrappedTools = map[string]string{
	"mv":    "[ \"${FAKE_TOOL_FAIL-}\" = mv ] && exit 1\n",
	"chmod": "[ \"${FAKE_TOOL_FAIL-}\" = chmod ] && [ \"$1\" = 600 ] && exit 1\n",
}

type rig struct {
	t       *testing.T
	root    string
	clone   string // physical path of the clone holding the script copy
	home    string
	xdg     string
	work    string // the launch directory
	sys     string // SEXTILE_TEST_SYSROOT
	fakebin string
	toolbin string
	log     string
	env     map[string]string
	unset   map[string]bool
	docker  bool
	git     bool
}

type option func(*rig, *rigPlan)

type rigPlan struct{ gitClone bool }

func noDocker() option { return func(r *rig, _ *rigPlan) { r.docker = false } }
func noGit() option    { return func(r *rig, _ *rigPlan) { r.git = false } }
func plainClone() option {
	return func(_ *rig, p *rigPlan) { p.gitClone = false }
}

func scriptSource(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "sextile"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func repoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// newRig makes the temporary world: a clone holding a copy of the script
// (a git checkout at tag v0.1.0-rc.1 unless plainClone), a home, a config
// home, a fake docker and the few real tools the script uses.
func newRig(t *testing.T, opts ...option) *rig {
	t.Helper()
	// Not t.TempDir: its name carries the test's name, which may hold a comma
	// (a path Docker cannot mount) or a digit run that looks like a port.
	tmp, err := os.MkdirTemp("", "sxl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })
	root, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{
		t: t, root: root,
		clone:   filepath.Join(root, "clone"),
		home:    filepath.Join(root, "home"),
		xdg:     filepath.Join(root, "xdg"),
		work:    filepath.Join(root, "work"),
		sys:     filepath.Join(root, "sys"),
		fakebin: filepath.Join(root, "fakebin"),
		toolbin: filepath.Join(root, "toolbin"),
		log:     filepath.Join(root, "docker.log"),
		env:     map[string]string{"FAKE_DOCKER_IMAGE": "labelled"},
		unset:   map[string]bool{},
		docker:  true,
		git:     true,
	}
	plan := &rigPlan{gitClone: true}
	for _, o := range opts {
		o(r, plan)
	}
	for _, d := range []string{r.clone, r.home, r.xdg, r.work, r.sys, r.fakebin, r.toolbin} {
		mustMkdir(t, d)
	}
	mustWrite(t, filepath.Join(r.clone, "sextile"), string(scriptSource(t)), 0o755)
	mustWrite(t, filepath.Join(r.clone, "marker.txt"), "tracked\n", 0o644)
	if r.docker {
		mustWrite(t, filepath.Join(r.fakebin, "docker"), fakeDocker, 0o755)
	}
	mustWrite(t, filepath.Join(r.fakebin, "sudo"), sudoShim, 0o755)
	for _, n := range toolNames {
		if n == "git" && !r.git {
			continue
		}
		p, err := exec.LookPath(n)
		if err != nil {
			t.Fatalf("the tests need %s on PATH: %v", n, err)
		}
		if guard, ok := wrappedTools[n]; ok {
			mustWrite(t, filepath.Join(r.toolbin, n), "#!/usr/bin/env bash\n"+guard+"exec "+p+" \"$@\"\n", 0o755)
			continue
		}
		if err := os.Symlink(p, filepath.Join(r.toolbin, n)); err != nil {
			t.Fatal(err)
		}
	}
	if plan.gitClone {
		r.git2("init", "-q", "-b", "main")
		r.git2("add", "-A")
		r.git2("commit", "-q", "-m", "first")
		r.git2("tag", "v0.1.0-rc.1")
		r.syncCommit()
	}
	return r
}

// syncCommit tells the fake docker which commit a labelled image was built from.
func (r *rig) syncCommit() { r.setenv("FAKE_DOCKER_COMMIT", r.git2("rev-parse", "HEAD")) }

// git2 runs the real git in the clone, without the ambient GIT_* variables.
func (r *rig) git2(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.clone
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GIT_") {
			env = append(env, e)
		}
	}
	env = append(env, "HOME="+r.home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@example.invalid", "GIT_COMMITTER_DATE=2020-01-01T00:00:00Z",
		"GIT_AUTHOR_DATE=2020-01-01T00:00:00Z")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// removeDocker takes the fake docker off PATH after the rig was made.
func (r *rig) removeDocker() {
	r.t.Helper()
	r.docker = false
	if err := os.Rename(filepath.Join(r.fakebin, "docker"), filepath.Join(r.root, "docker.off")); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) setenv(k, v string) { r.env[k] = v; delete(r.unset, k) }
func (r *rig) unsetenv(k string)  { delete(r.env, k); r.unset[k] = true }

func (r *rig) path() string {
	return r.fakebin + ":" + r.toolbin
}

func (r *rig) environ() []string {
	env := []string{
		"PATH=" + r.path(),
		"HOME=" + r.home,
		"FAKE_DOCKER_LOG=" + r.log,
		"SEXTILE_TEST_SYSROOT=" + r.sys,
	}
	if r.xdg != "" && !r.unset["XDG_CONFIG_HOME"] {
		if _, set := r.env["XDG_CONFIG_HOME"]; !set {
			env = append(env, "XDG_CONFIG_HOME="+r.xdg)
		}
	}
	keys := make([]string, 0, len(r.env))
	for k := range r.env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, k+"="+r.env[k])
	}
	return env
}

func (r *rig) script() string { return filepath.Join(r.clone, "sextile") }

type result struct {
	stdout, stderr string
	code           int
}

func (r *rig) run(args ...string) result { return r.runIn("", args...) }

func (r *rig) runIn(stdin string, args ...string) result {
	r.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.script(), args...)
	cmd.Dir = r.work
	cmd.Env = r.environ()
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		r.t.Fatalf("running the launcher: %v", err)
	}
	noMismatch(r.t, so.String()+se.String())
	return result{so.String(), se.String(), code}
}

// calls returns the recorded docker calls.
func (r *rig) calls() [][]string {
	b, err := os.ReadFile(r.log)
	if err != nil {
		return nil
	}
	var out [][]string
	for _, rec := range strings.Split(string(b), "\x1e") {
		if rec == "" {
			continue
		}
		parts := strings.Split(rec, "\x00")
		out = append(out, parts[:len(parts)-1])
	}
	return out
}

func verbOf(c []string) string {
	if len(c) > 1 && (c[0] == "image" || c[0] == "context") {
		return c[0] + " " + c[1]
	}
	if len(c) == 0 {
		return ""
	}
	return c[0]
}

func (r *rig) verbs() []string {
	var v []string
	for _, c := range r.calls() {
		v = append(v, verbOf(c))
	}
	return v
}

// find returns the first recorded call of a verb, or nil.
func (r *rig) find(verb string) []string {
	for _, c := range r.calls() {
		if verbOf(c) == verb {
			return c
		}
	}
	return nil
}

func (r *rig) count(verb string) int {
	n := 0
	for _, c := range r.calls() {
		if verbOf(c) == verb {
			n++
		}
	}
	return n
}

func indexOf(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}

// imageIndex is where the image sits in a run call.
func imageIndex(call []string) int {
	for i, a := range call {
		if strings.HasPrefix(a, "localhost/sextile:") {
			return i
		}
	}
	return -1
}

func mountSource(t *testing.T, call []string) string {
	t.Helper()
	i := indexOf(call, "--mount")
	if i < 0 || i+1 >= len(call) {
		t.Fatalf("no --mount in %q", call)
	}
	m := call[i+1]
	const pre, post = "type=bind,source=", ",target=/charts"
	if !strings.HasPrefix(m, pre) || !strings.HasSuffix(m, post) {
		t.Fatalf("unexpected mount %q", m)
	}
	return strings.TrimSuffix(strings.TrimPrefix(m, pre), post)
}

func (r *rig) confPath() string { return filepath.Join(r.xdg, "sextile", "launcher.conf") }

func (r *rig) writeConf(content string) {
	r.t.Helper()
	mustWrite(r.t, r.confPath(), content, 0o600)
}

func (r *rig) mkdir(rel string) string {
	r.t.Helper()
	return mustMkdir(r.t, filepath.Join(r.root, rel))
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no loopback: %v", err)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}

// busyPort returns a port something accepts connections on.
func busyPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no loopback: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return l.Addr().(*net.TCPAddr).Port
}

// snapshot records every entry below the directories: mode, size, mtime and
// content digest of files, targets of symlinks.
func snapshot(t *testing.T, dirs ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, d := range dirs {
		err := filepath.WalkDir(d, func(p string, de fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := os.Lstat(p)
			if err != nil {
				return err
			}
			desc := fmt.Sprintf("%v", info.Mode())
			switch {
			case info.Mode()&os.ModeSymlink != 0:
				tgt, _ := os.Readlink(p)
				desc += " -> " + tgt
			case info.Mode().IsRegular():
				b, _ := os.ReadFile(p)
				desc += fmt.Sprintf(" %d %d %x", info.Size(), info.ModTime().UnixNano(), sha256.Sum256(b))
			}
			out[p] = desc
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// diffSnapshots lists the paths that were added, removed or changed.
func diffSnapshots(before, after map[string]string) []string {
	var d []string
	for p, a := range after {
		b, ok := before[p]
		switch {
		case !ok:
			d = append(d, "added "+p)
		case a != b:
			d = append(d, "changed "+p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			d = append(d, "removed "+p)
		}
	}
	sort.Strings(d)
	return d
}

// noMismatch fails when the fake docker saw the launcher inspect one image
// and build or run another.
func noMismatch(t *testing.T, out string) {
	t.Helper()
	if strings.Contains(out, "FAKE-DOCKER-MISMATCH") {
		t.Fatalf("the launcher inspected one image and used another:\n%s", out)
	}
}

func (r *rig) mustSucceed(res result) {
	r.t.Helper()
	if res.code != 0 {
		r.t.Fatalf("exit %d\nstdout: %s\nstderr: %s", res.code, res.stdout, res.stderr)
	}
}

func (r *rig) mustExit2(res result) {
	r.t.Helper()
	if res.code != 2 {
		r.t.Fatalf("want exit 2, got %d\nstdout: %s\nstderr: %s", res.code, res.stdout, res.stderr)
	}
}

func (r *rig) noBuildOrRun() {
	r.t.Helper()
	for _, v := range r.verbs() {
		if v == "build" || v == "run" {
			r.t.Fatalf("docker %s was called: %q", v, r.verbs())
		}
	}
}

func contains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("want %q in:\n%s", want, got)
	}
}

// norm replaces the temporary paths so output can be compared and frozen.
func (r *rig) norm(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	for _, p := range [][2]string{
		{r.clone, "$CLONE"}, {r.home, "$HOME"}, {r.xdg, "$XDG"}, {r.work, "$PWD"},
		{r.fakebin, "$FAKEBIN"}, {r.root, "$TMP"},
	} {
		s = strings.ReplaceAll(s, p[0], p[1])
	}
	return s
}

// golden compares got with testdata/<name>.golden, or rewrites it under -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `go test ./tools/launcher -update` to write it)", path, err)
	}
	if string(want) != got {
		t.Fatalf("golden mismatch for %s\n--- want\n%s\n--- got\n%s", path, want, got)
	}
}

// ---------------------------------------------------------------- C1, C1b

// assertGitignoreWorks asks real git, with the repository's own .gitignore in
// a scratch repository, whether the launcher is ignored and bin/ is.
func assertGitignoreWorks(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Log("git is not on PATH; only the literal .gitignore checks ran")
		return
	}
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".gitignore"), repoFile(t, ".gitignore"), 0o644)
	mustWrite(t, filepath.Join(dir, "sextile"), "#!/usr/bin/env bash\n", 0o755)
	mustWrite(t, filepath.Join(dir, "bin", "x"), "binary\n", 0o755)
	check := func(path string) error {
		cmd := exec.Command("git", "check-ignore", "-q", path)
		cmd.Dir = dir
		var env []string
		for _, e := range os.Environ() {
			if !strings.HasPrefix(e, "GIT_") {
				env = append(env, e)
			}
		}
		cmd.Env = append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		return cmd.Run()
	}
	init := exec.Command("git", "init", "-q")
	init.Dir = dir
	if out, err := init.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := check("sextile"); err == nil {
		t.Fatal("git ignores the launcher: .gitignore must not match sextile")
	}
	if err := check("bin/x"); err != nil {
		t.Fatalf("git does not ignore bin/x: %v", err)
	}
}

func TestScriptIsTrackedAndExecutable(t *testing.T) {
	path := filepath.Join("..", "..", "sextile")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("sextile is not executable: %v", info.Mode())
	}
	src := string(scriptSource(t))
	if !strings.HasPrefix(src, "#!/usr/bin/env bash\n") {
		t.Fatalf("sextile must start with a bash shebang, got %q", strings.SplitN(src, "\n", 2)[0])
	}
	// Tracked: no ignore rule may match it, and a version-control system must
	// say the file is tracked. Never a silent pass.
	for _, l := range strings.Split(repoFile(t, ".gitignore"), "\n") {
		switch strings.TrimSpace(l) {
		case "/sextile", "sextile", "*", "/*":
			t.Fatalf(".gitignore ignores the launcher with %q", l)
		}
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	exists := func(name string) bool { _, err := os.Stat(filepath.Join(root, name)); return err == nil }
	switch {
	case exists(".git"):
		if err := trackedInIndex(root); err != nil {
			t.Fatal(err)
		}
	case exists(".jj"):
		if _, err := exec.LookPath("jj"); err != nil {
			t.Fatal("no VCS could confirm tracking: .jj is present but jj is not on PATH")
		}
		// Paths are relative to the working directory, so run it at the root.
		cmd := exec.Command("jj", "-R", root, "file", "list", "sextile")
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("jj file list sextile: %v\n%s", err, out)
		}
		if strings.TrimSpace(string(out)) != "sextile" {
			t.Fatalf("jj does not list sextile as tracked:\n%s", out)
		}
	default:
		// A tree extracted by ./check --release has neither; the check says
		// which repository and commit it came from.
		if err := trackedFromEnvironment(root); err != nil {
			t.Fatal(err)
		}
	}
}

// trackedFromEnvironment is the case for a tree with neither .git nor .jj: it
// needs both release variables, and fails without them.
func trackedFromEnvironment(root string) error {
	repo, rev := os.Getenv("SEXTILE_RELEASE_REPO"), os.Getenv("SEXTILE_RELEASE_REV")
	if repo == "" || rev == "" {
		return fmt.Errorf("no VCS could confirm tracking: neither .git nor .jj is present, and SEXTILE_RELEASE_REPO and SEXTILE_RELEASE_REV are not both set")
	}
	return trackedInReleaseCommit(root, repo, rev)
}

// gitIn runs git in dir with no inherited GIT_* variables (a stale GIT_DIR or
// GIT_INDEX_FILE must not answer for another repository) and no user config.
func gitIn(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GIT_") {
			env = append(env, e)
		}
	}
	cmd.Env = append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %v\n%s", strings.Join(args, " "), err, ee.Stderr)
		}
		return "", fmt.Errorf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// trackedInIndex proves, in a git work tree at root, that sextile is in the
// index as an executable blob and that the working file is byte for byte that
// blob (an unstaged edit or a local chmod does not count as tracked).
func trackedInIndex(root string) error {
	out, err := gitIn(root, "ls-files", "-s", "--", "sextile")
	if err != nil {
		return fmt.Errorf("git does not answer for sextile: %w", err)
	}
	meta, name, ok := strings.Cut(strings.TrimSpace(out), "\t")
	fields := strings.Fields(meta)
	if !ok || name != "sextile" || len(fields) != 3 {
		return fmt.Errorf("git does not track sextile (ls-files -s: %q)", out)
	}
	if fields[0] != "100755" {
		return fmt.Errorf("git tracks sextile as mode %s, want 100755", fields[0])
	}
	got, err := gitIn(root, "hash-object", filepath.Join(root, "sextile"))
	if err != nil {
		return fmt.Errorf("hashing sextile: %w", err)
	}
	if strings.TrimSpace(got) != fields[1] {
		return fmt.Errorf("the working sextile is blob %s, but git tracks blob %s", strings.TrimSpace(got), fields[1])
	}
	return nil
}

// trackedInReleaseCommit proves, for a tree extracted from commit rev of the
// repository at repo, that sextile is tracked there as an executable blob and
// that the extracted file is byte for byte that blob. rev must be a full
// commit id (a branch can move), and the same commit must also hold this test
// file byte for byte. That rejects a repository unrelated to the tree under
// test; it does not prove every file came from rev — ./check --release, which
// extracts the tree from rev and sets both variables itself, is what binds them.
func trackedInReleaseCommit(root, repo, rev string) error {
	if len(rev) != 40 || strings.Trim(rev, "0123456789abcdef") != "" {
		return fmt.Errorf("release commit %q is not a full commit id", rev)
	}
	out, err := gitIn(repo, "ls-tree", rev, "--", "sextile")
	if err != nil {
		return fmt.Errorf("release commit %s does not confirm tracking: %w", rev, err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 || lines[0] == "" {
		return fmt.Errorf("release commit %s: want exactly one ls-tree entry for sextile, got %q", rev, out)
	}
	meta, name, ok := strings.Cut(lines[0], "\t")
	fields := strings.Fields(meta)
	if !ok || name != "sextile" || len(fields) != 3 {
		return fmt.Errorf("release commit %s: unexpected ls-tree entry %q", rev, lines[0])
	}
	if fields[0] != "100755" || fields[1] != "blob" {
		return fmt.Errorf("release commit %s tracks sextile as mode %s type %s, want 100755 blob", rev, fields[0], fields[1])
	}
	got, err := gitIn(root, "hash-object", filepath.Join(root, "sextile"))
	if err != nil {
		return fmt.Errorf("hashing the extracted sextile: %w", err)
	}
	if strings.TrimSpace(got) != fields[2] {
		return fmt.Errorf("the extracted sextile is blob %s, but release commit %s tracks blob %s", strings.TrimSpace(got), rev, fields[2])
	}
	const self = "tools/launcher/launcher_test.go"
	want, err := gitIn(repo, "rev-parse", rev+":"+self)
	if err != nil {
		return fmt.Errorf("release commit %s does not hold %s, so it is not this tree's commit: %w", rev, self, err)
	}
	have, err := gitIn(root, "hash-object", filepath.Join(root, self))
	if err != nil {
		return fmt.Errorf("hashing the extracted %s: %w", self, err)
	}
	if strings.TrimSpace(have) != strings.TrimSpace(want) {
		return fmt.Errorf("release commit %s holds another %s than this tree: it is not this tree's commit", rev, self)
	}
	return nil
}

// TestTrackingInIndex drives the ordinary checkout case: tracked means in the
// index, executable, and the working file equal to the indexed blob.
func TestTrackingInIndex(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	repoWith := func(t *testing.T, mode os.FileMode, track bool) string {
		t.Helper()
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "sextile"), "#!/usr/bin/env bash\necho hi\n", mode)
		for _, args := range [][]string{{"init", "-q"}, {"config", "core.fileMode", "true"}} {
			if _, err := gitIn(dir, args...); err != nil {
				t.Fatal(err)
			}
		}
		if track {
			if _, err := gitIn(dir, "add", "sextile"); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	t.Run("tracked passes", func(t *testing.T) {
		if err := trackedInIndex(repoWith(t, 0o755, true)); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("unstaged edit fails", func(t *testing.T) {
		dir := repoWith(t, 0o755, true)
		mustWrite(t, filepath.Join(dir, "sextile"), "#!/usr/bin/env bash\necho edited\n", 0o755)
		if err := trackedInIndex(dir); err == nil || !strings.Contains(err.Error(), "tracks blob") {
			t.Fatalf("want a blob mismatch, got %v", err)
		}
	})
	t.Run("indexed 100644 fails", func(t *testing.T) {
		dir := repoWith(t, 0o644, true)
		if err := os.Chmod(filepath.Join(dir, "sextile"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := trackedInIndex(dir); err == nil || !strings.Contains(err.Error(), "100644") {
			t.Fatalf("want a mode failure, got %v", err)
		}
	})
	t.Run("untracked fails even with a stale GIT_DIR", func(t *testing.T) {
		other := repoWith(t, 0o755, true)
		dir := repoWith(t, 0o755, false)
		t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
		if err := trackedInIndex(dir); err == nil {
			t.Fatal("an untracked sextile passed because GIT_DIR named another repository")
		}
	})
}

// TestTrackingFromReleaseCommit drives the release-tree case against a
// throwaway repository: it passes for an exact extract and fails for each way
// the proof can be unmet.
func TestTrackingFromReleaseCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	setup := func(t *testing.T, mode os.FileMode) (root, repo, rev string) {
		t.Helper()
		repo = t.TempDir()
		mustWrite(t, filepath.Join(repo, "sextile"), "#!/usr/bin/env bash\necho hi\n", mode)
		mustWrite(t, filepath.Join(repo, "tools", "launcher", "launcher_test.go"), "package launcher\n", 0o644)
		run := func(args ...string) string {
			cmd := exec.Command("git", args...)
			cmd.Dir = repo
			cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
				"GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@example.invalid",
				"GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@example.invalid")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
			return strings.TrimSpace(string(out))
		}
		run("init", "-q", "-b", "main")
		run("config", "core.fileMode", "true")
		run("add", "-A")
		run("commit", "-q", "-m", "area: first")
		rev = run("rev-parse", "HEAD")
		root = t.TempDir()
		archive := exec.Command("sh", "-c", "git archive --format=tar "+rev+" | tar -x -C "+root)
		archive.Dir = repo
		if out, err := archive.CombinedOutput(); err != nil {
			t.Fatalf("git archive: %v\n%s", err, out)
		}
		return root, repo, rev
	}
	t.Run("exact extract passes", func(t *testing.T) {
		root, repo, rev := setup(t, 0o755)
		if err := trackedInReleaseCommit(root, repo, rev); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("edited extract fails", func(t *testing.T) {
		root, repo, rev := setup(t, 0o755)
		mustWrite(t, filepath.Join(root, "sextile"), "#!/usr/bin/env bash\necho changed\n", 0o755)
		if err := trackedInReleaseCommit(root, repo, rev); err == nil || !strings.Contains(err.Error(), "tracks blob") {
			t.Fatalf("want a blob mismatch, got %v", err)
		}
	})
	t.Run("mode 100644 fails", func(t *testing.T) {
		root, repo, rev := setup(t, 0o644)
		if err := trackedInReleaseCommit(root, repo, rev); err == nil || !strings.Contains(err.Error(), "100644") {
			t.Fatalf("want a mode failure, got %v", err)
		}
	})
	t.Run("branch name fails", func(t *testing.T) {
		root, repo, _ := setup(t, 0o755)
		if err := trackedInReleaseCommit(root, repo, "main"); err == nil || !strings.Contains(err.Error(), "full commit id") {
			t.Fatalf("want a refusal for a branch name, got %v", err)
		}
	})
	t.Run("unrelated repository fails", func(t *testing.T) {
		root, _, _ := setup(t, 0o755)
		// Another repository with the same launcher but another tree.
		_, other, _ := setup(t, 0o755)
		mustWrite(t, filepath.Join(other, "tools", "launcher", "launcher_test.go"), "package launcher // other\n", 0o644)
		run := exec.Command("sh", "-c", "git add -A && git -c user.name=T -c user.email=t@example.invalid commit -q -m 'area: other'")
		run.Dir = other
		if out, err := run.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		rp := exec.Command("git", "rev-parse", "HEAD")
		rp.Dir = other
		b, err := rp.Output()
		if err != nil {
			t.Fatal(err)
		}
		if err := trackedInReleaseCommit(root, other, strings.TrimSpace(string(b))); err == nil || !strings.Contains(err.Error(), "not this tree's commit") {
			t.Fatalf("want a refusal for an unrelated repository, got %v", err)
		}
	})
	t.Run("unknown commit fails", func(t *testing.T) {
		root, repo, _ := setup(t, 0o755)
		if err := trackedInReleaseCommit(root, repo, strings.Repeat("0", 40)); err == nil {
			t.Fatal("want a failure for a commit the repository does not have")
		}
	})
	t.Run("missing env fails", func(t *testing.T) {
		root, repo, rev := setup(t, 0o755)
		for name, env := range map[string][2]string{
			"neither": {"", ""},
			"repo":    {repo, ""},
			"rev":     {"", rev},
		} {
			t.Setenv("SEXTILE_RELEASE_REPO", env[0])
			t.Setenv("SEXTILE_RELEASE_REV", env[1])
			if err := trackedFromEnvironment(root); err == nil {
				t.Fatalf("%s: tracking passed without both environment variables", name)
			}
		}
		t.Setenv("SEXTILE_RELEASE_REPO", repo)
		t.Setenv("SEXTILE_RELEASE_REV", rev)
		if err := trackedFromEnvironment(root); err != nil {
			t.Fatalf("both variables set: %v", err)
		}
	})
}

func TestGitignoreBinNotScript(t *testing.T) {
	var hasBin bool
	for _, l := range strings.Split(repoFile(t, ".gitignore"), "\n") {
		l = strings.TrimSpace(l)
		if l == "/bin/" {
			hasBin = true
		}
		if l == "/sextile" || l == "sextile" {
			t.Fatalf(".gitignore must not ignore the launcher: %q", l)
		}
	}
	if !hasBin {
		t.Fatal(".gitignore does not ignore /bin/")
	}
	assertGitignoreWorks(t)
}

// ------------------------------------------------------------- C2 to C5b

func TestNoDocker(t *testing.T) {
	r := newRig(t, noDocker())
	res := r.run()
	r.mustExit2(res)
	contains(t, res.stderr, "Docker")
	contains(t, res.stderr, "https://docs.docker.com/get-docker/")
	if len(r.calls()) != 0 {
		t.Fatalf("nothing may run: %q", r.verbs())
	}
}

func TestDaemonDenied(t *testing.T) {
	r := newRig(t)
	r.setenv("FAKE_DOCKER_DENIED", "1")
	res := r.run()
	r.mustExit2(res)
	contains(t, res.stderr, "cannot reach the Docker daemon")
	contains(t, res.stderr, "docker group")
	contains(t, res.stderr, "Docker Desktop")
	if _, err := os.Stat(r.log + ".sudo"); err == nil {
		t.Fatal("the launcher called sudo")
	}
	if strings.Contains(res.stderr, "sudo") {
		t.Fatalf("the message must not point at sudo:\n%s", res.stderr)
	}
	r.noBuildOrRun()
}

func TestDaemonDown(t *testing.T) {
	r := newRig(t)
	r.setenv("FAKE_DOCKER_FAIL", "version")
	res := r.run()
	r.mustExit2(res)
	contains(t, res.stderr, "Docker is not running")
	r.noBuildOrRun()
}

func TestDockerTooOld(t *testing.T) {
	r := newRig(t)
	r.setenv("FAKE_DOCKER_SERVER", "27.5.1")
	res := r.run()
	r.mustExit2(res)
	contains(t, res.stderr, "27.5.1")
	contains(t, res.stderr, "28")
	r.noBuildOrRun()

	// The boundary: 28 and anything above it passes.
	for _, v := range []string{"28.0.0", "100.1.0"} {
		r2 := newRig(t)
		r2.setenv("FAKE_DOCKER_SERVER", v)
		r2.mustSucceed(r2.run("--charts-dir", r2.mkdir("c")))
		if r2.count("run") != 1 {
			t.Fatalf("Docker %s must run: %q", v, r2.verbs())
		}
	}
	// 9.x is below 28 (a string compare would pass it).
	r3 := newRig(t)
	r3.setenv("FAKE_DOCKER_SERVER", "9.9.9")
	r3.mustExit2(r3.run("--charts-dir", r3.mkdir("c")))
}

func TestRemoteContextRefused(t *testing.T) {
	r := newRig(t)
	r.setenv("FAKE_DOCKER_ENDPOINT", "tcp://build-host:2375")
	r.setenv("FAKE_DOCKER_IMAGE", "absent")
	res := r.run("--charts-dir", r.mkdir("c"))
	r.mustExit2(res)
	contains(t, res.stderr, "local Docker")
	r.noBuildOrRun()

	// DOCKER_HOST wins over the context.
	r2 := newRig(t)
	r2.setenv("FAKE_DOCKER_IMAGE", "absent")
	r2.setenv("DOCKER_HOST", "ssh://user@elsewhere")
	res = r2.run("--charts-dir", r2.mkdir("c"))
	r2.mustExit2(res)
	contains(t, res.stderr, "local Docker")
	r2.noBuildOrRun()

	// ... and a local DOCKER_HOST passes over a remote context.
	r3 := newRig(t)
	r3.setenv("FAKE_DOCKER_ENDPOINT", "tcp://build-host:2375")
	r3.setenv("DOCKER_HOST", "unix:///run/user/1000/docker.sock")
	r3.mustSucceed(r3.run("--charts-dir", r3.mkdir("c")))
}

// ------------------------------------------------------------ C6 to C10

func TestTagFromGit(t *testing.T) {
	r := newRig(t)
	r.mustSucceed(r.run("--charts-dir", r.mkdir("c")))
	run := r.find("run")
	if run == nil || run[imageIndex(run)] != "localhost/sextile:v0.1.0-rc.1" {
		t.Fatalf("the clean tag names the image: %q", run)
	}

	// A commit after the tag: <tag>-<n>-g<hash>.
	mustWrite(t, filepath.Join(r.clone, "more.txt"), "x\n", 0o644)
	r.git2("add", "-A")
	r.git2("commit", "-q", "-m", "second")
	r.syncCommit()
	want := "localhost/sextile:" + r.git2("describe", "--tags", "--always", "--dirty")
	if !regexp.MustCompile(`^localhost/sextile:v0\.1\.0-rc\.1-1-g[0-9a-f]+$`).MatchString(want) {
		t.Fatalf("unexpected describe %q", want)
	}
	r2 := r
	r2.log = filepath.Join(r.root, "docker2.log")
	r2.mustSucceed(r2.run("--charts-dir", r.mkdir("c")))
	run = r2.find("run")
	if run == nil || run[imageIndex(run)] != want {
		t.Fatalf("image %q, want %q", run, want)
	}

	// A slash in the tag becomes a dash.
	r2.git2("tag", "rel/1.0")
	r2.git2("tag", "-d", "v0.1.0-rc.1")
	r2.log = filepath.Join(r.root, "docker3.log")
	r2.mustSucceed(r2.run("--charts-dir", r.mkdir("c")))
	run = r2.find("run")
	if run == nil || run[imageIndex(run)] != "localhost/sextile:rel-1.0" {
		t.Fatalf("a / in the tag must become -: %q", run)
	}
}

func TestGitDescribeFails(t *testing.T) {
	r := newRig(t, plainClone())
	r.git2("init", "-q", "-b", "main") // a .git with no commit: describe fails
	r.setenv("FAKE_DOCKER_IMAGE", "absent")
	res := r.run("--charts-dir", r.mkdir("c"))
	r.mustExit2(res)
	contains(t, res.stderr, "git")
	r.noBuildOrRun()
}

func TestBuildArgsAndLabel(t *testing.T) {
	r := newRig(t)
	r.setenv("FAKE_DOCKER_IMAGE", "absent")
	r.mustSucceed(r.run("--charts-dir", r.mkdir("c")))
	b := r.find("build")
	if b == nil {
		t.Fatalf("no build: %q", r.verbs())
	}
	for _, pair := range [][2]string{
		{"--build-arg", "SEXTILE_VERSION=v0.1.0-rc.1"},
		{"--label", "io.github.cruiserxyz.sextile.launcher=1"},
		{"-t", "localhost/sextile:v0.1.0-rc.1"},
		{"--label", "io.github.cruiserxyz.sextile.commit=" + r.git2("rev-parse", "HEAD")},
	} {
		found := false
		for i := 0; i+1 < len(b); i++ {
			if b[i] == pair[0] && b[i+1] == pair[1] {
				found = true
			}
		}
		if !found {
			t.Fatalf("want %s %s in %q", pair[0], pair[1], b)
		}
	}
	if b[len(b)-1] != r.clone {
		t.Fatalf("the build context is the clone %q, got %q", r.clone, b[len(b)-1])
	}
	if indexOf(b, "-q") >= 0 {
		t.Fatalf("a clean-tag build streams its output, no -q: %q", b)
	}
}

func TestNoBuildWhenPresent(t *testing.T) {
	r := newRig(t)
	r.setenv("FAKE_DOCKER_EXPECT_IMAGE", "localhost/sextile:v0.1.0-rc.1")
	r.mustSucceed(r.run("--charts-dir", r.mkdir("c")))
	if r.count("build") != 0 || r.count("run") != 1 {
		t.Fatalf("a labelled clean-tag image is reused: %q", r.verbs())
	}
}

// C7c: the tag says nothing about the content of local and dirty images.
func TestLocalAndDirtyAlwaysBuild(t *testing.T) {
	cases := map[string]func() *rig{
		"local": func() *rig { return newRig(t, plainClone()) },
		"dirty": func() *rig {
			r := newRig(t)
			mustWrite(t, filepath.Join(r.clone, "marker.txt"), "edited\n", 0o644)
			return r
		},
	}
	for name, mk := range cases {
		r := mk()
		r.setenv("FAKE_DOCKER_IMAGE", "labelled")
		r.mustSucceed(r.run("--charts-dir", r.mkdir("c")))
		b := r.find("build")
		if b == nil {
			t.Fatalf("%s: an existing labelled image must still be rebuilt: %q", name, r.verbs())
		}
		if indexOf(b, "-q") < 0 {
			t.Fatalf("%s: the rebuild is quiet (docker build -q): %q", name, b)
		}
		// A quiet build shows its output only when it fails.
		r2 := mk()
		r2.setenv("FAKE_DOCKER_IMAGE", "labelled")
		res := r2.run("--charts-dir", r2.mkdir("c"))
		if strings.Contains(res.stdout+res.stderr, "sha256:") {
			t.Fatalf("%s: a good quiet build prints nothing of Docker's:\n%s%s", name, res.stdout, res.stderr)
		}
		r3 := mk()
		r3.setenv("FAKE_DOCKER_FAIL", "build")
		res = r3.run("--charts-dir", r3.mkdir("c"))
		if res.code == 0 || !strings.Contains(res.stderr, "BUILD-ERROR-LINE") {
			t.Fatalf("%s: a failed quiet build shows its output (exit %d):\n%s", name, res.code, res.stderr)
		}
	}
}

func TestUnlabelledImageRebuilt(t *testing.T) {
	r := newRig(t)
	r.setenv("FAKE_DOCKER_EXPECT_IMAGE", "localhost/sextile:v0.1.0-rc.1")
	r.setenv("FAKE_DOCKER_IMAGE", "unlabelled")
	r.mustSucceed(r.run("--charts-dir", r.mkdir("c")))
	v := r.verbs()
	bi, ri := -1, -1
	for i, x := range v {
		if x == "build" && bi < 0 {
			bi = i
		}
		if x == "run" && ri < 0 {
			ri = i
		}
	}
	if bi < 0 || ri < 0 || bi > ri {
		t.Fatalf("an unlabelled image is replaced by a build before the run: %q", v)
	}
	b := r.find("build")
	if i := indexOf(b, "-t"); i < 0 || b[i+1] != "localhost/sextile:v0.1.0-rc.1" {
		t.Fatalf("the build must replace the name: %q", b)
	}
}

// An image that carries the launcher's label but was built from another commit
// is not this checkout's image.
func TestImageFromOtherCommitRebuilt(t *testing.T) {
	r := newRig(t)
	head := r.git2("rev-parse", "HEAD")
	r.setenv("FAKE_DOCKER_IMAGE", "labelled")
	r.setenv("FAKE_DOCKER_COMMIT", strings.Repeat("0", 40))
	r.setenv("FAKE_DOCKER_EXPECT_IMAGE", "localhost/sextile:v0.1.0-rc.1")
	r.mustSucceed(r.run("--charts-dir", r.mkdir("c")))
	b := r.find("build")
	if b == nil {
		t.Fatalf("an image built from another commit must be rebuilt: %q", r.verbs())
	}
	if i := indexOf(b, "io.github.cruiserxyz.sextile.commit="+head); i < 0 {
		t.Fatalf("the rebuild labels this commit %s: %q", head, b)
	}
	if r.count("run") != 1 {
		t.Fatalf("then it runs: %q", r.verbs())
	}

	// An image with no commit label at all is stale too.
	r2 := newRig(t)
	r2.setenv("FAKE_DOCKER_COMMIT", "")
	r2.mustSucceed(r2.run("--charts-dir", r2.mkdir("c")))
	if r2.find("build") == nil {
		t.Fatalf("an image without the commit label must be rebuilt: %q", r2.verbs())
	}

	// A local build has no commit to label.
	r3 := newRig(t, plainClone())
	r3.mustSucceed(r3.run("--charts-dir", r3.mkdir("c")))
	for _, a := range r3.find("build") {
		if strings.Contains(a, "sextile.commit") {
			t.Fatalf("no commit label for :local: %q", a)
		}
	}
}

func TestLocalTag(t *testing.T) {
	check := func(name string, r *rig) {
		t.Helper()
		r.setenv("FAKE_DOCKER_IMAGE", "absent")
		r.mustSucceed(r.run("--charts-dir", r.mkdir("c")))
		b := r.find("build")
		if b == nil {
			t.Fatalf("%s: no build", name)
		}
		if i := indexOf(b, "--build-arg"); i < 0 || b[i+1] != "SEXTILE_VERSION=local" {
			t.Fatalf("%s: build arg: %q", name, b)
		}
		run := r.find("run")
		if run == nil || run[imageIndex(run)] != "localhost/sextile:local" {
			t.Fatalf("%s: image %q", name, run)
		}
	}
	check("no .git", newRig(t, plainClone()))
	check("git missing from PATH", newRig(t, noGit()))
}

func TestLocalBuildNote(t *testing.T) {
	r := newRig(t, plainClone())
	r.setenv("FAKE_DOCKER_IMAGE", "absent")
	res := r.run("--charts-dir", r.mkdir("c"))
	r.mustSucceed(res)
	want := "rebuilding localhost/sextile:local (no git history, so every start checks for changes)\n"
	if res.stderr != want {
		t.Fatalf("local note:\n got %q\nwant %q", res.stderr, want)
	}

	r2 := newRig(t)
	mustWrite(t, filepath.Join(r2.clone, "marker.txt"), "edited\n", 0o644)
	res = r2.run("--charts-dir", r2.mkdir("c"))
	r2.mustSucceed(res)
	if n := strings.Count(res.stderr, "\n"); n != 1 || !strings.Contains(res.stderr, "localhost/sextile:v0.1.0-rc.1-dirty") ||
		!strings.Contains(res.stderr, "uncommitted changes") {
		t.Fatalf("dirty note (one line naming the tag): %q", res.stderr)
	}

	// A clean tag says nothing.
	r3 := newRig(t)
	r3.setenv("FAKE_DOCKER_IMAGE", "absent")
	res = r3.run("--charts-dir", r3.mkdir("c"))
	r3.mustSucceed(res)
	if strings.Contains(res.stderr, "rebuilding") {
		t.Fatalf("a clean tag prints no rebuild note: %q", res.stderr)
	}
}

func TestRebuild(t *testing.T) {
	r := newRig(t)
	r.mustSucceed(r.run("--charts-dir", r.mkdir("c"), "--rebuild"))
	if r.count("build") != 1 {
		t.Fatalf("--rebuild must build even though the labelled image exists: %q", r.verbs())
	}
	run := r.find("run")
	if run == nil || indexOf(run, "--rebuild") >= 0 {
		t.Fatalf("--rebuild is the launcher's, not the app's: %q", run)
	}
}

func TestBuildFailureStops(t *testing.T) {
	for _, name := range []string{"clean", "local"} {
		var r *rig
		if name == "clean" {
			r = newRig(t)
		} else {
			r = newRig(t, plainClone())
		}
		r.setenv("FAKE_DOCKER_IMAGE", "absent")
		r.setenv("FAKE_DOCKER_FAIL", "build")
		res := r.run("--charts-dir", r.mkdir("c"))
		if res.code == 0 {
			t.Fatalf("%s: a failed build must not exit 0", name)
		}
		if r.count("run") != 0 {
			t.Fatalf("%s: docker run must not be called after a failed build: %q", name, r.verbs())
		}
	}
}

// ----------------------------------------------------------- C11 to C11c

func TestRunArgv(t *testing.T) {
	uid, gid := strconv.Itoa(os.Getuid()), strconv.Itoa(os.Getgid())

	t.Run("everything set", func(t *testing.T) {
		r := newRig(t)
		charts := r.mkdir("charts")
		mustWrite(t, filepath.Join(r.sys, "usr/share/zoneinfo/Europe/Amsterdam"), "", 0o644)
		r.setenv("TZ", "Europe/Amsterdam")
		r.setenv("TERM", "xterm-256color")
		r.setenv("COLORTERM", "truecolor")
		r.setenv("NO_COLOR", "1")
		r.setenv("SEXTILE_COLOR", "always")
		p := strconv.Itoa(freePort(t))
		r.mustSucceed(r.run("--charts-dir", charts, "--port", p, "--accessible"))
		want := []string{
			"run", "--rm", "--pull", "never", "-i",
			"--user", uid + ":" + gid,
			"--mount", "type=bind,source=" + charts + ",target=/charts",
			"--tmpfs", "/tmp",
			"-p", "127.0.0.1:" + p + ":" + p,
			"-e", "SEXTILE_PREVIEW_ADDR=0.0.0.0:" + p,
			"-e", "SEXTILE_LAUNCH=docker localhost/sextile:v0.1.0-rc.1",
			"-e", "TZ=Europe/Amsterdam",
			"-e", "TERM", "-e", "COLORTERM", "-e", "NO_COLOR", "-e", "SEXTILE_COLOR",
			"localhost/sextile:v0.1.0-rc.1", "--accessible",
		}
		if got := r.find("run"); fmt.Sprintf("%q", got) != fmt.Sprintf("%q", want) {
			t.Fatalf("run argv:\n got %q\nwant %q", got, want)
		}
	})

	t.Run("nothing set", func(t *testing.T) {
		r := newRig(t)
		charts := r.mkdir("charts")
		r.mustSucceed(r.run("--charts-dir", charts))
		want := []string{
			"run", "--rm", "--pull", "never", "-i",
			"--user", uid + ":" + gid,
			"--mount", "type=bind,source=" + charts + ",target=/charts",
			"--tmpfs", "/tmp",
			"-p", "127.0.0.1:5307:5307",
			"-e", "SEXTILE_PREVIEW_ADDR=0.0.0.0:5307",
			"-e", "SEXTILE_LAUNCH=docker localhost/sextile:v0.1.0-rc.1",
			"localhost/sextile:v0.1.0-rc.1",
		}
		if got := r.find("run"); fmt.Sprintf("%q", got) != fmt.Sprintf("%q", want) {
			t.Fatalf("run argv:\n got %q\nwant %q", got, want)
		}
	})

	t.Run("set but empty counts as set", func(t *testing.T) {
		r := newRig(t)
		r.setenv("NO_COLOR", "")
		r.mustSucceed(r.run("--charts-dir", r.mkdir("charts")))
		if run := r.find("run"); indexOf(run, "NO_COLOR") < 0 || indexOf(run, "TERM") >= 0 {
			t.Fatalf("-e NO_COLOR exactly when it is set: %q", run)
		}
	})

	t.Run("the port is the same inside and out", func(t *testing.T) {
		r := newRig(t)
		r.mustSucceed(r.run("--charts-dir", r.mkdir("charts"), "--port=65535"))
		run := r.find("run")
		if indexOf(run, "127.0.0.1:65535:65535") < 0 || indexOf(run, "SEXTILE_PREVIEW_ADDR=0.0.0.0:65535") < 0 {
			t.Fatalf("%q", run)
		}
	})
}

func TestExitStatusIsDockers(t *testing.T) {
	for _, want := range []int{0, 3, 137} {
		r := newRig(t)
		r.setenv("FAKE_DOCKER_RUN_EXIT", strconv.Itoa(want))
		res := r.run("--charts-dir", r.mkdir("c"))
		if res.code != want {
			t.Fatalf("docker run exited %d, the launcher %d", want, res.code)
		}
	}
}

// ----------------------------------------------------------- C12 to C14

func TestPortInvalid(t *testing.T) {
	for _, v := range []string{"abc", "80", "1023", "65536", "0", "99999999", "5307x", "-1", "08080", "1e3", " 5307"} {
		r := newRig(t)
		res := r.run("--charts-dir", r.mkdir("c"), "--port", v)
		r.mustExit2(res)
		if !strings.Contains(res.stderr, v) {
			t.Fatalf("--port %q: the message must name the value:\n%s", v, res.stderr)
		}
		r.noBuildOrRun()
	}
	// The bounds themselves are valid.
	for _, v := range []string{"1024", "65535"} {
		r := newRig(t)
		r.mustSucceed(r.run("--charts-dir", r.mkdir("c"), "--port", v))
	}
}

func TestPortBusy(t *testing.T) {
	r := newRig(t)
	r.setenv("FAKE_DOCKER_IMAGE", "absent")
	p := busyPort(t)
	res := r.run("--charts-dir", r.mkdir("c"), "--port", strconv.Itoa(p))
	r.mustExit2(res)
	want := fmt.Sprintf("port %d is in use; run ./sextile --port %d\n", p, p+1)
	if res.stderr != want {
		t.Fatalf("got %q want %q", res.stderr, want)
	}
	r.noBuildOrRun()

	// 65535 suggests 65534, never 65536.
	l, err := net.Listen("tcp", "127.0.0.1:65535")
	if err != nil {
		t.Log("65535 unavailable here; the edge is checked only when it can be bound")
		return
	}
	defer l.Close()
	r2 := newRig(t)
	res = r2.run("--charts-dir", r2.mkdir("c"), "--port", "65535")
	r2.mustExit2(res)
	if !strings.Contains(res.stderr, "--port 65534") || strings.Contains(res.stderr, "65536") {
		t.Fatalf("65535 busy must suggest 65534: %q", res.stderr)
	}
}

// A second window: with no --port and the default port taken, the launcher
// takes the next free port, says so on one line, and publishes that port.
// The explicit --port refusal is TestPortBusy.
func TestDefaultPortBusyPicksTheNext(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:5307")
	if err == nil {
		defer l.Close()
	} else if c, derr := net.Dial("tcp", "127.0.0.1:5307"); derr != nil {
		t.Skipf("5307 can be neither bound nor reached here: %v", err)
	} else {
		c.Close() // something else already holds it, which is what is wanted
	}
	r := newRig(t)
	res := r.run("--charts-dir", r.mkdir("c"))
	r.mustSucceed(res)
	var p int
	if _, err := fmt.Sscanf(res.stderr, "port 5307 is in use; this window uses port %d\n", &p); err != nil {
		t.Fatalf("stderr = %q, want one line naming the port this window uses", res.stderr)
	}
	if want := fmt.Sprintf("port 5307 is in use; this window uses port %d\n", p); res.stderr != want || p <= 5307 || p > 5407 {
		t.Fatalf("stderr = %q, want exactly %q with a port above 5307", res.stderr, want)
	}
	run := r.find("run")
	ps := strconv.Itoa(p)
	if indexOf(run, "127.0.0.1:"+ps+":"+ps) < 0 || indexOf(run, "SEXTILE_PREVIEW_ADDR=0.0.0.0:"+ps) < 0 {
		t.Fatalf("the run does not publish port %d: %q", p, run)
	}

	// An explicit --port 5307 is still refused.
	r2 := newRig(t)
	res = r2.run("--charts-dir", r2.mkdir("c"), "--port", "5307")
	r2.mustExit2(res)
	if res.stderr != "port 5307 is in use; run ./sextile --port 5308\n" {
		t.Fatalf("explicit busy --port: stderr = %q", res.stderr)
	}
	r2.noBuildOrRun()
}

func TestTimezoneOrder(t *testing.T) {
	zone := func(r *rig) (string, bool) {
		res := r.run("--charts-dir", r.mkdir("c"))
		r.mustSucceed(res)
		if res.stdout != "" || res.stderr != "" {
			t.Fatalf("the time zone prints nothing: %q %q", res.stdout, res.stderr)
		}
		run := r.find("run")
		for i, a := range run {
			if a == "-e" && strings.HasPrefix(run[i+1], "TZ=") {
				return strings.TrimPrefix(run[i+1], "TZ="), true
			}
		}
		return "", false
	}
	setup := func(tzEnv string) *rig {
		r := newRig(t)
		for _, z := range []string{"Asia/Tokyo", "Europe/Berlin", "America/New_York"} {
			mustWrite(t, filepath.Join(r.sys, "usr/share/zoneinfo", z), "", 0o644)
		}
		mustWrite(t, filepath.Join(r.sys, "etc/timezone"), "Europe/Berlin\nignored\n", 0o644)
		if err := os.Symlink("/usr/share/zoneinfo/America/New_York", filepath.Join(r.sys, "etc/localtime")); err != nil {
			t.Fatal(err)
		}
		if tzEnv != "" {
			r.setenv("TZ", tzEnv)
		}
		return r
	}

	if z, ok := zone(setup("Asia/Tokyo")); !ok || z != "Asia/Tokyo" {
		t.Fatalf("$TZ naming a zoneinfo file comes first, got %q %v", z, ok)
	}
	if z, ok := zone(setup("Nowhere/Land")); !ok || z != "Europe/Berlin" {
		t.Fatalf("an unknown $TZ falls to /etc/timezone's first line, got %q %v", z, ok)
	}
	if z, ok := zone(setup("../../etc/timezone")); !ok || z != "Europe/Berlin" {
		t.Fatalf("a $TZ that escapes zoneinfo is not a name, got %q %v", z, ok)
	}
	if z, ok := zone(setup("")); !ok || z != "Europe/Berlin" {
		t.Fatalf("no $TZ: /etc/timezone, got %q %v", z, ok)
	}

	r := setup("")
	mustWrite(t, filepath.Join(r.sys, "etc/timezone"), "\n", 0o644) // empty first line
	if z, ok := zone(r); !ok || z != "America/New_York" {
		t.Fatalf("an empty /etc/timezone falls to the /etc/localtime link, got %q %v", z, ok)
	}

	r = newRig(t)
	mustMkdir(t, filepath.Join(r.sys, "etc"))
	if err := os.Symlink("/var/db/timezone/zoneinfo/Pacific/Auckland", filepath.Join(r.sys, "etc/localtime")); err != nil {
		t.Fatal(err)
	}
	if z, ok := zone(r); !ok || z != "Pacific/Auckland" {
		t.Fatalf("the part after zoneinfo/ of the link, got %q %v", z, ok)
	}

	r = newRig(t)
	if z, ok := zone(r); ok {
		t.Fatalf("when nothing yields a name, no -e TZ: got %q", z)
	}
}

// ------------------------------------------------------- C15 to C20

func TestChartsPrecedence(t *testing.T) {
	// The flag wins and the config is neither read nor validated.
	r := newRig(t)
	r.writeConf("this is not a valid config\nat all\n")
	rel := mustMkdir(t, filepath.Join(r.work, "rel", "charts"))
	r.mustSucceed(r.run("--charts-dir", "rel/charts"))
	if got := mountSource(t, r.find("run")); got != rel {
		t.Fatalf("a relative --charts-dir resolves from the launch directory: %q want %q", got, rel)
	}

	// The remembered folder, when there is no flag.
	r2 := newRig(t)
	kept := r2.mkdir("kept")
	r2.writeConf("# note\n\ncharts-dir=" + kept + "\n")
	r2.mustSucceed(r2.run())
	if got := mountSource(t, r2.find("run")); got != kept {
		t.Fatalf("remembered folder: %q want %q", got, kept)
	}

	// The flag outranks a valid remembered folder.
	other := r2.mkdir("other")
	r2.log = filepath.Join(r2.root, "second.log")
	r2.mustSucceed(r2.run("--charts-dir="+other, "--rebuild"))
	if got := mountSource(t, r2.find("run")); got != other {
		t.Fatalf("flag over config: %q want %q", got, other)
	}

	// A flag naming a missing folder is refused, and not created.
	r3 := newRig(t)
	missing := filepath.Join(r3.root, "typo")
	res := r3.run("--charts-dir", missing)
	r3.mustExit2(res)
	if _, err := os.Stat(missing); err == nil {
		t.Fatal("--charts-dir must never create the folder")
	}
	r3.noBuildOrRun()
}

func TestForwardedChartsDirRefused(t *testing.T) {
	cases := [][]string{
		{"--", "--charts-dir"},
		{"--", "--charts-dir", "/x"},
		{"--", "--charts-dir=/x"},
		{"--", "--version", "--charts-dir=/x"},
		{"-charts-dir=/x"},
		{"-charts-dir", "/x"},
		{"--", "-charts-dir=/x"},
	}
	for _, c := range cases {
		r := newRig(t)
		args := append([]string{"--charts-dir", r.mkdir("c")}, c...)
		res := r.run(args...)
		r.mustExit2(res)
		contains(t, res.stderr, "--charts-dir")
		r.noBuildOrRun()
	}
}

func TestChartsDirInsideCloneRefused(t *testing.T) {
	const verbatim = " is inside the Sextile checkout ("
	inside := func(r *rig) map[string]string {
		sub := mustMkdir(t, filepath.Join(r.clone, "charts"))
		link := filepath.Join(r.root, "linked")
		if err := os.Symlink(sub, link); err != nil {
			t.Fatal(err)
		}
		return map[string]string{"below the clone": sub, "the clone itself": r.clone, "a link into the clone": link}
	}
	r0 := newRig(t)
	for name := range inside(r0) {
		r := newRig(t)
		dir := inside(r)[name]
		res := r.run("--charts-dir", dir)
		r.mustExit2(res)
		contains(t, res.stderr, dir+verbatim+r.clone+"); keep client charts outside it")
		r.noBuildOrRun()
	}

	// A remembered folder inside the clone is refused too.
	r := newRig(t)
	sub := mustMkdir(t, filepath.Join(r.clone, "kept"))
	r.writeConf("charts-dir=" + sub + "\n")
	res := r.run()
	r.mustExit2(res)
	contains(t, res.stderr, sub+verbatim+r.clone+"); keep client charts outside it")
	r.noBuildOrRun()

	// Outside the clone still works.
	r2 := newRig(t)
	r2.mustSucceed(r2.run("--charts-dir", r2.mkdir("outside")))
}

func TestChartsDirNotRemembered(t *testing.T) {
	r := newRig(t)
	r.mustSucceed(r.run("--charts-dir", r.mkdir("c")))
	if _, err := os.Stat(filepath.Join(r.xdg, "sextile")); err == nil {
		t.Fatal("a --charts-dir run must not create the config")
	}
	r.writeConf("charts-dir=" + r.mkdir("kept") + "\n")
	before, _ := os.ReadFile(r.confPath())
	r.mustSucceed(r.run("--charts-dir", r.mkdir("c2")))
	after, _ := os.ReadFile(r.confPath())
	if !bytes.Equal(before, after) {
		t.Fatalf("the config changed: %q -> %q", before, after)
	}
}

func TestNoTerminalNoChoice(t *testing.T) {
	r := newRig(t)
	r.setenv("FAKE_DOCKER_IMAGE", "absent")
	before := snapshot(t, r.home, r.xdg, r.clone, r.work)
	res := r.runIn("1\n") // an answer on a pipe is not a terminal
	r.mustExit2(res)
	contains(t, res.stderr, "--charts-dir")
	r.noBuildOrRun()
	if d := diffSnapshots(before, snapshot(t, r.home, r.xdg, r.clone, r.work)); len(d) != 0 {
		t.Fatalf("nothing may be written: %v", d)
	}
}

func TestConfigInvalidOrFolderMissing(t *testing.T) {
	good := func(r *rig) string { return "charts-dir=" + r.mkdir("ok") + "\n" }
	bad := map[string]func(r *rig) string{
		"two lines":      func(r *rig) string { return good(r) + good(r) },
		"unknown key":    func(r *rig) string { return "folder=" + r.mkdir("ok") + "\n" },
		"relative path":  func(r *rig) string { return "charts-dir=charts\n" },
		"empty file":     func(r *rig) string { return "" },
		"only comments":  func(r *rig) string { return "# nothing\n\n" },
		"empty value":    func(r *rig) string { return "charts-dir=\n" },
		"spaced key":     func(r *rig) string { return " charts-dir=" + r.mkdir("ok") + "\n" },
		"comma in path":  func(r *rig) string { return "charts-dir=/tmp/a,b\n" },
		"extra garbage":  func(r *rig) string { return good(r) + "stray line\n" },
		"missing folder": func(r *rig) string { return "charts-dir=" + filepath.Join(r.root, "gone") + "\n" },
	}
	for name, content := range bad {
		r := newRig(t)
		r.setenv("FAKE_DOCKER_IMAGE", "absent")
		c := content(r)
		r.writeConf(c)
		before := snapshot(t, r.home, r.xdg, r.clone, r.work)
		res := r.run()
		if res.code != 2 {
			t.Fatalf("%s: want exit 2, got %d\n%s", name, res.code, res.stderr)
		}
		if !strings.Contains(res.stderr, r.confPath()) {
			t.Fatalf("%s: the message must name the config file:\n%s", name, res.stderr)
		}
		if name == "missing folder" && !strings.Contains(res.stderr, filepath.Join(r.root, "gone")) {
			t.Fatalf("%s: the message must name the folder:\n%s", name, res.stderr)
		}
		r.noBuildOrRun()
		if d := diffSnapshots(before, snapshot(t, r.home, r.xdg, r.clone, r.work)); len(d) != 0 {
			t.Fatalf("%s: nothing may be created: %v", name, d)
		}
		if _, err := os.Stat(filepath.Join(r.root, "gone")); err == nil {
			t.Fatalf("%s: a missing remembered folder is not re-created", name)
		}
	}

	// A valid file with comments and blank lines is accepted.
	r := newRig(t)
	dir := r.mkdir("fine")
	r.writeConf("# remembered\n\n   \ncharts-dir=" + dir + "\n# end\n")
	r.mustSucceed(r.run())
}

func TestUnmountablePath(t *testing.T) {
	for _, name := range []string{"a,b", "a\nb"} {
		r := newRig(t)
		dir := r.mkdir(name)
		res := r.run("--charts-dir", dir)
		r.mustExit2(res)
		contains(t, res.stderr, "mount")
		r.noBuildOrRun()
	}
	// A remembered folder with a comma is refused too.
	r := newRig(t)
	r.writeConf("charts-dir=" + r.mkdir("x,y") + "\n")
	res := r.run()
	r.mustExit2(res)
	contains(t, res.stderr, "mount")
	r.noBuildOrRun()
	// A relative path that only resolves to a comma is caught after resolving.
	r = newRig(t)
	r.mkdir("work/p,q")
	r.mustExit2(r.run("--charts-dir", "p,q"))
}

// ------------------------------------------------------- C21 to C21c

func TestArgForwarding(t *testing.T) {
	r := newRig(t)
	charts := r.mkdir("c")
	p := strconv.Itoa(freePort(t))
	// Launcher flags are recognised anywhere before "--"; the rest is
	// forwarded in order, verbatim.
	r.mustSucceed(r.run("a b", "--port="+p, "*", "$HOME", "--charts-dir", charts, "", "--rebuild", "-x", "l1\nl2", "doctor",
		"--", "--port", "9", "--rebuild", "--help", "-h", "doctor", "--version"))
	run := r.find("run")
	want := []string{"a b", "*", "$HOME", "", "-x", "l1\nl2", "doctor", "--port", "9", "--rebuild", "--help", "-h", "doctor", "--version"}
	got := run[imageIndex(run)+1:]
	if fmt.Sprintf("%q", got) != fmt.Sprintf("%q", want) {
		t.Fatalf("forwarded:\n got %q\nwant %q", got, want)
	}
	if indexOf(run, "127.0.0.1:"+p+":"+p) < 0 {
		t.Fatalf("--port= was not taken: %q", run)
	}
	if r.count("build") != 1 {
		t.Fatalf("--rebuild was not taken: %q", r.verbs())
	}

	// "doctor" anywhere but first is an argument.
	r2 := newRig(t)
	r2.mustSucceed(r2.run("--charts-dir", r2.mkdir("c"), "doctor"))
	if run := r2.find("run"); run == nil || run[len(run)-1] != "doctor" {
		t.Fatalf("doctor only counts as the first argument: %q", run)
	}

	// --help is the launcher's.
	r3 := newRig(t)
	res := r3.run("--help")
	r3.mustSucceed(res)
	contains(t, res.stdout, "usage:")
	if len(r3.calls()) != 0 {
		t.Fatalf("--help runs nothing: %q", r3.verbs())
	}
}

func TestFlagErrors(t *testing.T) {
	cases := [][]string{
		{"--port"},
		{"--port="},
		{"--charts-dir"},
		{"--charts-dir="},
		{"--port", "5400", "--port", "5401"},
		{"--port=5400", "--port", "5401"},
		{"--rebuild", "--rebuild"},
		{"--no-docker", "--no-docker"},
		{"--charts-dir", "/tmp", "--charts-dir=/tmp"},
	}
	for _, c := range cases {
		r := newRig(t)
		r.mustExit2(r.run(c...))
		if len(r.calls()) != 0 {
			t.Fatalf("%q: nothing may reach docker: %q", c, r.verbs())
		}
	}
}

func TestDoctorArgs(t *testing.T) {
	for _, c := range [][]string{
		{"doctor", "--rebuild"},
		{"doctor", "--no-docker"},
		{"doctor", "foo"},
		{"doctor", "--"},
		{"doctor", "--help"},
		{"doctor", "-h"},
		{"doctor", "--port", "5400", "extra"},
		{"doctor", "--", "--port", "5400"},
	} {
		r := newRig(t)
		r.mustExit2(r.run(c...))
		if len(r.calls()) != 0 {
			t.Fatalf("%q: a refused doctor calls nothing: %q", c, r.verbs())
		}
	}

	// --port and --charts-dir, both spellings, are what doctor diagnoses.
	for _, form := range []string{"spaced", "equals"} {
		r := newRig(t)
		p := strconv.Itoa(freePort(t))
		d := r.mkdir("diag")
		args := []string{"doctor", "--port", p, "--charts-dir", d}
		if form == "equals" {
			args = []string{"doctor", "--port=" + p, "--charts-dir=" + d}
		}
		res := r.run(args...)
		r.mustSucceed(res)
		if !strings.Contains(res.stdout, p+" is free") || !strings.Contains(res.stdout, d) {
			t.Fatalf("%s: doctor must diagnose the given port and folder:\n%s", form, res.stdout)
		}
	}
	r := newRig(t)
	p := busyPort(t)
	res := r.run("doctor", "--port", strconv.Itoa(p), "--charts-dir", r.mkdir("c"))
	if res.code != 1 || !strings.Contains(res.stdout, strconv.Itoa(p)+" in use") {
		t.Fatalf("doctor with a busy --port: exit %d\n%s", res.code, res.stdout)
	}
}

// ------------------------------------------------------------- C22 to C22c

var doctorNames = []string{"docker", "daemon", "endpoint", "image", "port", "wheel", "time zone", "charts folder", "config", "other images"}

var doctorNativeNames = []string{"go", "astrolog", "astrolog files"}

func TestDoctorLines(t *testing.T) {
	for _, engine := range []string{"Docker Engine", "Podman Engine"} {
		r := newRig(t)
		r.setenv("FAKE_DOCKER_ENGINE", engine)
		r.setImages(
			"img aaaa00000001 1 localhost/sextile:v0.1.0-rc.1",
			"img bbbb00000002 1 localhost/sextile:v0.0.9",
			"img cccc00000003 1 localhost/sextile:local",
		)
		r.writeConf("charts-dir=" + r.mkdir("kept") + "\n")
		res := r.run("doctor", "--port", strconv.Itoa(freePort(t)))
		r.mustSucceed(res)
		lines := strings.Split(strings.TrimSuffix(res.stdout, "\n"), "\n")
		// The rows for --no-docker follow, under their own heading.
		if len(lines) != len(doctorNames)+1+len(doctorNativeNames) || lines[len(doctorNames)] != "for --no-docker (optional):" {
			t.Fatalf("%s: want %d lines and the --no-docker block, got %d:\n%s", engine, len(doctorNames), len(lines), res.stdout)
		}
		for i, n := range doctorNames {
			re := regexp.MustCompile(`^(ok|note|FAIL) +` + regexp.QuoteMeta(n) + `: .`)
			if !re.MatchString(lines[i]) {
				t.Fatalf("%s: line %d should be the %q check: %q", engine, i+1, n, lines[i])
			}
		}
		if !strings.Contains(lines[9], "2 other") {
			t.Fatalf("two images besides this checkout's: %q", lines[9])
		}
		if !strings.Contains(lines[3], "localhost/sextile:v0.1.0-rc.1") {
			t.Fatalf("the image line names this checkout's image: %q", lines[3])
		}
		podman := strings.Contains(lines[1], "Podman")
		if podman != (engine == "Podman Engine") || (podman && !strings.HasPrefix(lines[1], "note")) {
			t.Fatalf("%s: the Podman note belongs on the daemon line, as a note: %q", engine, lines[1])
		}
	}
}

func TestDoctorReadOnly(t *testing.T) {
	allowed := map[string]bool{"version": true, "context inspect": true, "image inspect": true, "image ls": true, "ps": true}
	scenarios := map[string]func(*rig) []string{
		"no folder known":  func(r *rig) []string { return []string{"doctor"} },
		"flag, not exists": func(r *rig) []string { return []string{"doctor", "--charts-dir", filepath.Join(r.root, "nope")} },
		"valid config": func(r *rig) []string {
			r.writeConf("charts-dir=" + r.mkdir("kept") + "\n")
			return []string{"doctor"}
		},
		"missing remembered folder": func(r *rig) []string {
			r.writeConf("charts-dir=" + filepath.Join(r.root, "gone") + "\n")
			return []string{"doctor"}
		},
		"local image": func(r *rig) []string { return []string{"doctor", "--charts-dir", r.mkdir("c")} },
	}
	for name, mk := range scenarios {
		r := newRig(t, plainClone())
		if name == "valid config" {
			r.setenv("FAKE_DOCKER_IMAGE", "absent")
		}
		args := mk(r)
		before := snapshot(t, r.home, r.xdg, r.clone, r.work, r.sys)
		r.run(args...)
		for _, v := range r.verbs() {
			if !allowed[v] {
				t.Fatalf("%s: doctor called docker %s", name, v)
			}
		}
		if d := diffSnapshots(before, snapshot(t, r.home, r.xdg, r.clone, r.work, r.sys)); len(d) != 0 {
			t.Fatalf("%s: doctor created or changed files: %v", name, d)
		}
	}
	// Podman's shim is a note, from `docker version`, which is allowed.
	r := newRig(t)
	r.setenv("FAKE_DOCKER_ENGINE", "Podman Engine")
	res := r.run("doctor", "--charts-dir", r.mkdir("c"))
	r.mustSucceed(res)
	if !regexp.MustCompile(`(?m)^note +daemon: .*Podman`).MatchString(res.stdout) {
		t.Fatalf("a note names Podman:\n%s", res.stdout)
	}
}

// ------------------------------------------------------------------- C23

// fakeDockerFile is the fake docker's own log and receipts, not the launcher's.
func fakeDockerFile(entry string) bool { return strings.Contains(entry, "docker.log") }

// unexpectedWrites diffs the WHOLE temporary root (every file by size, mtime
// and digest), ignoring only the fake docker's files and what the row allows.
func unexpectedWrites(before, after map[string]string, allowed func(entry, path string) bool) []string {
	var bad []string
	for _, d := range diffSnapshots(before, after) {
		path := d[strings.Index(d, " ")+1:]
		if fakeDockerFile(path) || (allowed != nil && allowed(d, path)) {
			continue
		}
		bad = append(bad, d)
	}
	return bad
}

func TestWritesOnlyConfig(t *testing.T) {
	// A plain launch with a flag writes nothing at all, anywhere under the root.
	r := newRig(t)
	r.setenv("FAKE_DOCKER_IMAGE", "absent")
	charts := r.mkdir("c")
	before := snapshot(t, r.root)
	r.mustSucceed(r.run("--charts-dir", charts, "--rebuild"))
	if d := unexpectedWrites(before, snapshot(t, r.root), nil); len(d) != 0 {
		t.Fatalf("a launch wrote: %v", d)
	}

	// With a remembered folder, still nothing; also for a local build.
	r2 := newRig(t, plainClone())
	r2.writeConf("charts-dir=" + r2.mkdir("kept") + "\n")
	before = snapshot(t, r2.root)
	r2.mustSucceed(r2.run())
	if d := unexpectedWrites(before, snapshot(t, r2.root), nil); len(d) != 0 {
		t.Fatalf("a launch from the config wrote: %v", d)
	}

	// The first-run prompt (pty_linux_test.go) writes the config and the
	// chosen folder and nothing else: TestPromptWritesConfig.
}

// ------------------------------------------------------------------- C24

func TestBash32Constructs(t *testing.T) {
	banned := map[string]*regexp.Regexp{
		"mapfile":                regexp.MustCompile(`\bmapfile\b`),
		"readarray":              regexp.MustCompile(`\breadarray\b`),
		"declare -A":             regexp.MustCompile(`\b(declare|typeset|local)\s+-[a-zA-Z]*A`),
		"declare -n / local -n":  regexp.MustCompile(`\b(declare|typeset|local)\s+-[a-zA-Z]*n\b`),
		"lowercase expansion":    regexp.MustCompile(`\$\{[A-Za-z_0-9@*]+,,?[^}]*\}`),
		"uppercase expansion":    regexp.MustCompile(`\$\{[A-Za-z_0-9@*]+\^\^?[^}]*\}`),
		"|&":                     regexp.MustCompile(`\|&`),
		"&>>":                    regexp.MustCompile(`&>>`),
		";& and ;;& in case":     regexp.MustCompile(`;;?&`),
		"[[ -v":                  regexp.MustCompile(`\[\[\s+-v\b`),
		"coproc":                 regexp.MustCompile(`\bcoproc\b`),
		"${var@op}":              regexp.MustCompile(`\$\{[A-Za-z_0-9]+@[QEPAa]\}`),
		"globstar":               regexp.MustCompile(`\*\*/`),
		"negative array index":   regexp.MustCompile(`\[-[0-9]+\]`),
		"readlink -f / realpath": regexp.MustCompile(`readlink\s+-f|\brealpath\b`),
		"printf %(...)T":         regexp.MustCompile(`%\([^)]*\)T`),
		"read -N / -i":           regexp.MustCompile(`\bread\s+(-[a-zA-Z]*[Ni]\b)`),
		"${!arr[@]} keys":        regexp.MustCompile(`\$\{![A-Za-z_]+\[[@*]\]\}`),
	}
	inHeredoc := ""
	for n, l := range strings.Split(string(scriptSource(t)), "\n") {
		if inHeredoc != "" {
			if l == inHeredoc {
				inHeredoc = ""
			}
			continue
		}
		if m := regexp.MustCompile(`<<-?'?([A-Z]+)'?\s*$`).FindStringSubmatch(l); m != nil {
			inHeredoc = m[1]
		}
		code := strings.TrimSpace(l)
		if strings.HasPrefix(code, "#") {
			continue
		}
		for name, re := range banned {
			if re.MatchString(code) {
				t.Errorf("sextile:%d uses %s (not in bash 3.2): %s", n+1, name, code)
			}
		}
	}
	if out, err := exec.Command("bash", "-n", filepath.Join("..", "..", "sextile")).CombinedOutput(); err != nil {
		t.Fatalf("bash -n: %v\n%s", err, out)
	}
}

// ------------------------------------- README, reference page and CHANGELOG

// section returns the lines under the heading that matches, up to the next
// heading of the same or a shallower level.
func section(t *testing.T, doc string, heading string) []string {
	t.Helper()
	lines := strings.Split(doc, "\n")
	level := 0
	var out []string
	in := false
	inFence := false
	for _, l := range lines {
		if strings.HasPrefix(l, "```") {
			inFence = !inFence
		}
		isHeading := !inFence && strings.HasPrefix(l, "#")
		if isHeading {
			n := len(l) - len(strings.TrimLeft(l, "#"))
			if in && n <= level {
				break
			}
			if !in && strings.TrimSpace(strings.TrimLeft(l, "# ")) == heading {
				in = true
				level = n
				continue
			}
		}
		if in {
			out = append(out, l)
		}
	}
	if !in {
		t.Fatalf("no heading %q", heading)
	}
	return out
}

// reference is the page the README links for the install routes it does not
// lead with: by hand, and without Docker.
var reference = filepath.Join("docs", "REFERENCE.md")

// codeBlocks returns the lines of every fenced block, in order.
func codeBlocks(lines []string) [][]string {
	var all [][]string
	var cur []string
	in := false
	for _, l := range lines {
		if strings.HasPrefix(l, "```") {
			if in {
				all = append(all, cur)
				cur = nil
			}
			in = !in
			continue
		}
		if in {
			cur = append(cur, l)
		}
	}
	return all
}

// The README's Install and run leads with the one-line installer, then the
// launcher it ends at, doctor first.
func TestReadmeLeadsWithLauncher(t *testing.T) {
	const url = "https://raw.githubusercontent.com/cruiserxyz/sextile/v0.1.0/scripts/install"
	got := codeBlocks(section(t, repoFile(t, "README.md"), "Install and run"))
	want := [][]string{
		{"curl -fsSL " + url + " | sh"},
		{"cd ~/sextile", "./sextile doctor", "./sextile"},
	}
	if len(got) < len(want) || fmt.Sprintf("%q", got[:len(want)]) != fmt.Sprintf("%q", want) {
		t.Fatalf("Install's first code blocks:\n got %q\nwant %q", got, want)
	}
}

// headingAnchors is the set of GitHub anchors of a Markdown file's headings:
// lower case, spaces to hyphens, punctuation dropped.
func headingAnchors(doc string) map[string]bool {
	out := map[string]bool{}
	inFence := false
	for _, l := range strings.Split(doc, "\n") {
		if strings.HasPrefix(l, "```") {
			inFence = !inFence
		}
		if inFence || !strings.HasPrefix(l, "#") {
			continue
		}
		h := strings.ToLower(strings.TrimSpace(strings.TrimLeft(l, "#")))
		var b strings.Builder
		for _, r := range h {
			switch {
			case r == ' ' || r == '-':
				b.WriteRune('-')
			case r == '_' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127:
				b.WriteRune(r)
			}
		}
		out[b.String()] = true
	}
	return out
}

// Every relative link in the README reaches a file in the tree, and every
// anchor it names is a heading of that file.
func TestReadmeLinksResolve(t *testing.T) {
	readme := repoFile(t, "README.md")
	link := regexp.MustCompile(`\]\(([^)\s]+)\)`)
	n := 0
	for _, m := range link.FindAllStringSubmatch(readme, -1) {
		target := m[1]
		if strings.Contains(target, "://") {
			continue
		}
		n++
		file, anchor, _ := strings.Cut(target, "#")
		doc := readme
		if file != "" {
			if _, err := os.Stat(filepath.Join("..", "..", file)); err != nil {
				t.Errorf("README links %q, which is not in the tree", target)
				continue
			}
			doc = repoFile(t, file)
		}
		if anchor != "" && !headingAnchors(doc)[anchor] {
			t.Errorf("README links %q, but %s has no heading with that anchor", target, map[bool]string{true: "the README", false: file}[file == ""])
		}
	}
	if n < 8 {
		t.Errorf("found only %d relative links in the README; the pattern has stopped matching", n)
	}
}

func TestReferenceByHand(t *testing.T) {
	sec := strings.Join(section(t, repoFile(t, reference), "By hand"), "\n")
	for _, l := range []string{
		`docker build -t sextile --build-arg SEXTILE_VERSION="$(git describe --tags --always)" .`,
		`mkdir -p ~/.charts`,
		`docker run -it --rm --user "$(id -u):$(id -g)" -v ~/.charts:/charts --tmpfs /tmp -p 127.0.0.1:5307:5307 sextile`,
	} {
		contains(t, sec, "\n"+l+"\n")
	}
}

func TestReferenceNativeBinary(t *testing.T) {
	sec := strings.Join(section(t, repoFile(t, reference), "Without Docker"), "\n")
	for _, l := range []string{"mkdir -p bin", "go build -o bin/sextile ./cmd/sextile", "bin/sextile --charts-dir ~/.charts"} {
		contains(t, sec, "\n"+l)
	}
	// The binary is built and run by hand; the one launcher line is the
	// launcher's own way of doing the same.
	for _, l := range strings.Split(sec, "\n") {
		if strings.Contains(l, "./sextile") && !strings.Contains(l, "`./sextile --no-docker`") {
			t.Fatalf("the native section runs the launcher other than as ./sextile --no-docker: %q", l)
		}
	}
	contains(t, sec, "`./sextile --no-docker`")
}

func TestReferenceLocalBinPath(t *testing.T) {
	sec := strings.Join(section(t, repoFile(t, reference), "Without Docker"), "\n")
	contains(t, sec, "~/.local/bin")
	contains(t, sec, "PATH")
	contains(t, sec, "scripts/build-astrolog")
}

func TestNoDiagnoseScriptCited(t *testing.T) {
	for _, f := range []string{"README.md", reference, filepath.Join("internal", "ui", "failures.go")} {
		if strings.Contains(repoFile(t, f), "diagnose-astrolog") {
			t.Fatalf("%s still cites scripts/diagnose-astrolog.sh, which does not exist", f)
		}
	}
}

func TestChangelogLauncherLine(t *testing.T) {
	// releasenotes finds the [0.1.0] section whatever its date, so dating it
	// for the release does not break this.
	cmd := exec.Command("go", "run", "./tools/releasenotes", "v0.1.0-rc.1")
	cmd.Dir = filepath.Join("..", "..")
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	if err := cmd.Run(); err != nil {
		t.Fatalf("releasenotes: %v\n%s", err, se.String())
	}
	out := so.String()
	contains(t, out, "./sextile")
}

// setImages writes the fake docker's image state (see fakeDocker): "img <id>
// <1|0> <repo:tag>..." and "ctr <id>" lines.
func (r *rig) setImages(lines ...string) {
	r.t.Helper()
	mustWrite(r.t, r.log+".state", strings.Join(lines, "\n")+"\n", 0o644)
}

// tagsLeft is every tag the fake docker still holds, sorted.
func (r *rig) tagsLeft() []string {
	r.t.Helper()
	b, err := os.ReadFile(r.log + ".state")
	if err != nil {
		r.t.Fatal(err)
	}
	var tags []string
	for _, l := range strings.Split(string(b), "\n") {
		if f := strings.Fields(l); len(f) >= 4 && f[0] == "img" {
			tags = append(tags, f[3:]...)
		}
	}
	sort.Strings(tags)
	return tags
}

// preparePrune sets a current image plus other images as Docker would hold
// them: the current image under a second (alias) tag, two unused older
// labelled images, an older labelled image a container uses, and an unlabelled
// localhost/sextile image. extra lines are appended to the state.
func preparePrune(r *rig, extra ...string) {
	r.setenv("FAKE_DOCKER_IMAGE", "labelled")
	r.syncCommit()
	r.setImages(append([]string{
		"img aaaa00000001 1 localhost/sextile:v0.1.0-rc.1 localhost/sextile:alias",
		"img bbbb00000002 1 localhost/sextile:v0.0.8",
		"img cccc00000003 1 localhost/sextile:v0.0.9",
		"img dddd00000004 1 localhost/sextile:v0.0.7",
		"ctr dddd00000004",
		"img eeee00000005 0 localhost/sextile:local",
	}, extra...)...)
}

func removedRefs(r *rig) []string {
	var refs []string
	for _, c := range r.calls() {
		if verbOf(c) == "image rm" {
			refs = append(refs, strings.Join(c[2:], " "))
		}
	}
	return refs
}

func TestLauncherPrunesOlderLabelledImages(t *testing.T) {
	r := newRig(t)
	preparePrune(r)
	res := r.run("--charts-dir", r.mkdir("c"))
	r.mustSucceed(res)
	got := removedRefs(r)
	if len(got) != 2 || got[0] != "localhost/sextile:v0.0.8" || got[1] != "localhost/sextile:v0.0.9" {
		t.Fatalf("only the two unused older labelled tags go (not the current id, its alias, the unlabelled one, or the one a container uses): %q", got)
	}
	if n := strings.Count(res.stderr, "removed 2 older sextile images\n"); n != 1 {
		t.Fatalf("stderr must say `removed 2 older sextile images` once:\n%s", res.stderr)
	}
	for _, c := range r.calls() {
		if verbOf(c) == "image rm" && indexOf(c, "-f") >= 0 {
			t.Fatalf("image rm must never be forced: %q", c)
		}
	}
	v := r.verbs()
	if v[len(v)-1] != "run" {
		t.Fatalf("pruning comes before the final docker run: %q", v)
	}
	// Nothing to remove: nothing is said.
	r2 := newRig(t)
	preparePrune(r2)
	r2.setImages("img aaaa00000001 1 localhost/sextile:v0.1.0-rc.1")
	res = r2.run("--charts-dir", r2.mkdir("c"))
	r2.mustSucceed(res)
	if strings.Contains(res.stderr, "older sextile") || len(removedRefs(r2)) != 0 {
		t.Fatalf("with nothing to prune the launcher says and removes nothing:\n%s", res.stderr)
	}
}

// Real Docker refuses `image rm <id>` while the image has several tags, so the
// launcher removes tags: an old image with two tags must end with none.
func TestLauncherPrunesTagsOfMultiTaggedOldImage(t *testing.T) {
	r := newRig(t)
	preparePrune(r,
		"img ffff00000006 1 localhost/sextile:v0.0.5 localhost/sextile:v0.0.5-rc",
		"img gggg00000007 1 localhost/sextile:v0.0.4 localhost/sextile:v0.0.4-rc",
		"ctr gggg00000007",
	)
	res := r.run("--charts-dir", r.mkdir("c"))
	r.mustSucceed(res)
	want := []string{
		"localhost/sextile:alias",
		"localhost/sextile:local",
		"localhost/sextile:v0.0.4",
		"localhost/sextile:v0.0.4-rc",
		"localhost/sextile:v0.0.7",
		"localhost/sextile:v0.1.0-rc.1",
	}
	if got := r.tagsLeft(); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("left: the current image and its alias, the unlabelled one, and the two in use; none of the unused old ones' tags:\n got %q\nwant %q\n%s", got, want, res.stderr)
	}
	if n := strings.Count(res.stderr, "removed 4 older sextile images\n"); n != 1 {
		t.Fatalf("four tags were removed (v0.0.8, v0.0.9, and the two of the multi-tag image):\n%s", res.stderr)
	}
	if strings.Contains(res.stderr, "could not") {
		t.Fatalf("nothing failed:\n%s", res.stderr)
	}
}

// Doctor counts exactly the tags the next launch removes: not unlabelled
// images, not the current image's alias, not images a container uses.
func TestDoctorCountsWhatPruningRemoves(t *testing.T) {
	cases := []struct {
		name  string
		extra []string
		want  string
	}{
		{"one", nil, "note  other images: 1 other images; removed at the next launch"},
		{"two", []string{"img ffff00000006 1 localhost/sextile:v0.0.5"}, "note  other images: 2 other images; removed at the next launch"},
		{"multi-tag", []string{"img ffff00000006 1 localhost/sextile:v0.0.5 localhost/sextile:v0.0.5-rc"}, "note  other images: 3 other images; removed at the next launch"},
	}
	for _, c := range cases {
		r := newRig(t)
		// One unused old tag (v0.0.8) besides the extras, so the first case is N=1;
		// the alias, the unlabelled and the in-use ones must not be counted.
		lines := []string{
			"img aaaa00000001 1 localhost/sextile:v0.1.0-rc.1 localhost/sextile:alias",
			"img bbbb00000002 1 localhost/sextile:v0.0.8",
			"img dddd00000004 1 localhost/sextile:v0.0.7",
			"ctr dddd00000004",
			"img eeee00000005 0 localhost/sextile:local",
		}
		r.setImages(append(lines, c.extra...)...)
		r.writeConf("charts-dir=" + r.mkdir("kept") + "\n")
		res := r.run("doctor", "--port", strconv.Itoa(freePort(t)))
		r.mustSucceed(res)
		var got string
		for _, l := range strings.Split(res.stdout, "\n") {
			if strings.Contains(l, "other images:") {
				got = l
			}
		}
		if got != c.want {
			t.Fatalf("%s: doctor line\n got %q\nwant %q", c.name, got, c.want)
		}
		// The same state, launched: exactly that many tags go.
		n := strings.Fields(strings.SplitN(got, ": ", 2)[1])[0]
		r.setenv("FAKE_DOCKER_IMAGE", "labelled")
		r.syncCommit()
		res = r.run("--charts-dir", r.mkdir("c"))
		r.mustSucceed(res)
		if !strings.Contains(res.stderr, "removed "+n+" older sextile images\n") {
			t.Fatalf("%s: doctor said %s, the launch removed otherwise:\n%s", c.name, n, res.stderr)
		}
	}
	// None: its own wording.
	r := newRig(t)
	r.setImages("img aaaa00000001 1 localhost/sextile:v0.1.0-rc.1", "img eeee00000005 0 localhost/sextile:local")
	r.writeConf("charts-dir=" + r.mkdir("kept") + "\n")
	res := r.run("doctor", "--port", strconv.Itoa(freePort(t)))
	r.mustSucceed(res)
	contains(t, res.stdout, "note  other images: no other images to remove\n")
}

// A `docker ps` failure leaves a tag unjudged. Doctor must say so, never print
// a definite count that silently leaves it out.
func TestDoctorSaysWhenItCouldNotCheck(t *testing.T) {
	doctorLine := func(r *rig) string {
		r.writeConf("charts-dir=" + r.mkdir("kept") + "\n")
		res := r.run("doctor", "--port", strconv.Itoa(freePort(t)))
		r.mustSucceed(res)
		for _, l := range strings.Split(res.stdout, "\n") {
			if strings.Contains(l, "other images") {
				return l
			}
		}
		t.Fatalf("no other-images line:\n%s", res.stdout)
		return ""
	}
	// Every ps fails: three labelled candidates (bbbb, cccc, dddd), none judged.
	r := newRig(t)
	preparePrune(r)
	r.setenv("FAKE_DOCKER_FAIL", "ps")
	want := "note  other images: 3 other images could not be checked (docker ps failed)"
	if got := doctorLine(r); got != want {
		t.Fatalf("ps failing:\n got %q\nwant %q", got, want)
	}
	// One image's ps fails: the count is of the rest, with the failure beside it.
	r = newRig(t)
	preparePrune(r)
	r.setenv("FAKE_DOCKER_PS_FAIL_ID", "bbbb00000002")
	want = "note  other images: 1 other images; removed at the next launch; 1 could not be checked (docker ps failed)"
	if got := doctorLine(r); got != want {
		t.Fatalf("ps failing for one image:\n got %q\nwant %q", got, want)
	}
}

func TestLauncherPruneFailureDoesNotStopLaunch(t *testing.T) {
	for _, verb := range []string{"image rm", "image ls", "ps"} {
		r := newRig(t)
		preparePrune(r)
		r.setenv("FAKE_DOCKER_FAIL", verb)
		res := r.run("--charts-dir", r.mkdir("c"))
		r.mustSucceed(res)
		if r.count("run") != 1 {
			t.Fatalf("%s failing: the launch must still reach docker run: %q", verb, r.verbs())
		}
		if n := strings.Count(res.stderr, "could not "); n != 1 {
			t.Fatalf("%s failing: one stderr line says so, got %d:\n%s", verb, n, res.stderr)
		}
		if strings.Contains(res.stderr, "removed ") {
			t.Fatalf("%s failing: nothing was removed:\n%s", verb, res.stderr)
		}
	}
}
