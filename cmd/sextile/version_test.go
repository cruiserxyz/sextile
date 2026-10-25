package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// The version rows run the REAL binary, built by the test: a unit test of the
// string builders could pass while the real binary still starts the screen or
// reads state first (the specification C7-C11).

var built = struct {
	sync.Mutex
	dir   string
	bins  map[string]string
	goenv []string
}{bins: map[string]string{}}

// captureGoEnv records go's cache roots as they are before testenv.Run points
// HOME elsewhere. A failure leaves the defaults; builds are then merely slow.
func captureGoEnv() {
	for _, k := range []string{"GOCACHE", "GOMODCACHE"} {
		if out, err := exec.Command("go", "env", k).Output(); err == nil {
			built.goenv = append(built.goenv, k+"="+strings.TrimSpace(string(out)))
		}
	}
}

func cleanBuilt() {
	built.Lock()
	defer built.Unlock()
	if built.dir != "" {
		os.RemoveAll(built.dir)
	}
}

// binary builds ./cmd/sextile once per distinct ldflags value (and one with no
// -ldflags at all, key "none") and returns its path.
func binary(t *testing.T, ldflags string) string {
	t.Helper()
	built.Lock()
	defer built.Unlock()
	if bin, ok := built.bins[ldflags]; ok {
		return bin
	}
	if built.dir == "" {
		d, err := os.MkdirTemp("", "sextile-bin-")
		if err != nil {
			t.Fatal(err)
		}
		built.dir = d
	}
	bin := filepath.Join(built.dir, fmt.Sprintf("sextile-%d", len(built.bins)))
	args := []string{"build", "-buildvcs=false", "-o", bin}
	if ldflags != "none" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, ".")
	cmd := exec.Command("go", args...)
	cmd.Env = append(append(os.Environ(), built.goenv...), "GOFLAGS=-p=4", "GOMAXPROCS=4", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	built.bins[ldflags] = bin
	return bin
}

func TestVersionString(t *testing.T) {
	info := func(ver string, kv ...string) *debug.BuildInfo {
		bi := &debug.BuildInfo{Main: debug.Module{Version: ver}}
		for i := 0; i+1 < len(kv); i += 2 {
			bi.Settings = append(bi.Settings, debug.BuildSetting{Key: kv[i], Value: kv[i+1]})
		}
		return bi
	}
	table := []struct {
		name     string
		ldflag   string
		info     *debug.BuildInfo
		haveInfo bool
		want     string
	}{
		{"-X value wins over everything", "v1.2.3", info("v0.0.1", "vcs.revision", "abcdef0123"), true, "v1.2.3"},
		{"module version when not (devel)", "", info("v0.4.0"), true, "v0.4.0"},
		{"devel with revision", "", info("(devel)", "vcs.revision", "0123456789abcdef"), true, "devel (0123456)"},
		{"devel with dirty revision", "", info("(devel)", "vcs.revision", "0123456789abcdef", "vcs.modified", "true"), true, "devel (0123456+dirty)"},
		{"vcs.modified=false is clean", "", info("(devel)", "vcs.revision", "0123456789", "vcs.modified", "false"), true, "devel (0123456)"},
		{"short revision kept whole", "", info("(devel)", "vcs.revision", "abc"), true, "devel (abc)"},
		{"(devel) without vcs", "", info("(devel)"), true, "devel"},
		{"empty module version without vcs", "", info(""), true, "devel"},
		{"no build info", "", nil, false, "devel"},
	}
	for _, c := range table {
		if got := versionString(c.ldflag, c.info, c.haveInfo); got != c.want {
			t.Errorf("%s: versionString = %q, want %q", c.name, got, c.want)
		}
	}

	// The same rows on the real binary: line 1 for each ldflags case.
	for _, c := range []struct{ name, ldflags, want string }{
		{"-X main.version=v9.9.9", "-X main.version=v9.9.9", "sextile v9.9.9"},
		{"-X main.version= (empty)", "-X main.version=", "sextile devel"},
		{"no -X, no vcs", "none", "sextile devel"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := runVersionBinary(t, binary(t, c.ldflags), versionRun{})
			if first, _, _ := strings.Cut(r.stdout, "\n"); first != c.want {
				t.Errorf("line 1 = %q, want %q (stdout %q)", first, c.want, r.stdout)
			}
		})
	}
}

// versionRun configures one run of the built binary.
type versionRun struct {
	path       string // PATH value for the child
	flag       string // the version flag; "" means --version
	dotEnvFIFO bool   // run from a child dir of a go.mod tree whose .env is a FIFO with no writer
	heldLock   bool   // another "instance" holds the session lock for the whole run
}

type versionResult struct {
	stdout, stderr string
	code           int
	elapsed        time.Duration
}

// runVersionBinary runs `sextile --version` with a sandboxed environment whose
// HOME and XDG roots are canary directories, and fails if anything under them
// changed.
func runVersionBinary(t *testing.T, bin string, vr versionRun) versionResult {
	t.Helper()
	root := t.TempDir()
	dirs := map[string]string{}
	for _, k := range []string{"HOME", "XDG_DATA_HOME", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "CHARTS"} {
		d := filepath.Join(root, k)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "canary"), []byte(k), 0o644); err != nil {
			t.Fatal(err)
		}
		dirs[k] = d
	}
	// FIFOs with no writer are tripwires for a READ: opening one blocks, so a
	// --version that touches preferences, the session or a chart file hangs.
	for _, f := range []string{
		filepath.Join(dirs["XDG_CONFIG_HOME"], "sextile", "preferences.yaml"),
		filepath.Join(dirs["XDG_DATA_HOME"], "sextile", "session", "session.yaml"),
		filepath.Join(dirs["CHARTS"], "client.as"),
	} {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mkfifo(f, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if vr.heldLock {
		holdSessionLock(t, dirs["XDG_DATA_HOME"])
	}
	before := fingerprint(t, root)
	cwd := root
	flag := vr.flag
	if flag == "" {
		flag = "--version"
	}
	args := []string{"--charts-dir", dirs["CHARTS"], flag}
	if vr.dotEnvFIFO {
		// A released binary must never open the .env it finds above its working
		// directory: opening a FIFO with no writer would block it.
		child := filepath.Join(root, "tree", "a", "b")
		if err := os.MkdirAll(child, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "tree", "go.mod"), []byte("module probe\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mkfifo(filepath.Join(root, "tree", ".env"), 0o644); err != nil {
			t.Fatal(err)
		}
		before = fingerprint(t, root)
		cwd = child
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = cwd
	cmd.Env = []string{
		"HOME=" + dirs["HOME"], "XDG_DATA_HOME=" + dirs["XDG_DATA_HOME"], "XDG_CONFIG_HOME=" + dirs["XDG_CONFIG_HOME"],
		"XDG_STATE_HOME=" + dirs["XDG_STATE_HOME"], "XDG_CACHE_HOME=" + dirs["XDG_CACHE_HOME"],
		"PATH=" + vr.path, "TERM=xterm-256color",
	}
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	start := time.Now()
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("--version did not finish within 15 s: it is blocked, most likely reading a preferences, session or chart file")
	}
	res := versionResult{stdout: o.String(), stderr: e.String(), elapsed: time.Since(start)}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			res.code = ee.ExitCode()
		} else {
			t.Fatalf("running %s: %v", bin, err)
		}
	}
	if after := fingerprint(t, root); after != before {
		t.Errorf("--version created, modified or removed something under HOME, XDG_* or --charts-dir:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	return res
}

// holdSessionLock takes the instance lock on dataHome's session folder from a
// descriptor this test owns, writing a pid as the real holder does, and keeps
// it until the test ends. It does not import the store: the lock is a file
// contract (the specification), and a test that called the store's own
// function could not notice the store changing it.
func holdSessionLock(t *testing.T, dataHome string) {
	t.Helper()
	dir := filepath.Join(dataHome, "sextile", "session")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "sextile.lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("4242\n"); err != nil {
		t.Fatal(err)
	}
}

// fingerprint lists every path under root with its mode and content hash.
func fingerprint(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, _ := d.Info()
		sum := ""
		if info != nil && info.Mode().IsRegular() {
			b, _ := os.ReadFile(p)
			sum = fmt.Sprintf("%x", sha256.Sum256(b))
		}
		mt, mode := "", ""
		if info != nil {
			mt = strconv.FormatInt(info.ModTime().UnixNano(), 10)
			mode = info.Mode().String() // type and permission bits: a chmod is a change
		}
		lines = append(lines, fmt.Sprintf("%s %v %s %s %s", p, d.Type(), mode, sum, mt))
		return nil
	})
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// fakeAstrolog writes an `astrolog` into a new dir and returns the dir. body is
// the sh script after the shebang.
func fakeAstrolog(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "astrolog"), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// ignoreTermChild starts a background child that ignores SIGTERM, so only
// SIGKILL can end it. The fake then records `$!`, that child's pid.
const ignoreTermChild = "sh -c 'trap \"\" TERM; while :; do sleep 1; done' &\n"

const fakeBanner = `echo '|                        ** Astrolog version 7.99 **                       |'
`

func TestVersionFlagPrintsAndExits(t *testing.T) {
	dir := fakeAstrolog(t, fakeBanner)
	r := runVersionBinary(t, binary(t, "-X main.version=v9.9.9"), versionRun{path: dir + ":/usr/bin:/bin"})
	if r.code != 0 {
		t.Errorf("exit %d, want 0 (stderr %q)", r.code, r.stderr)
	}
	if r.stderr != "" {
		t.Errorf("stderr = %q, want empty", r.stderr)
	}
	if strings.ContainsRune(r.stdout, 0x1b) {
		t.Errorf("stdout carries a terminal escape sequence: %q", r.stdout)
	}
	lines := strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n")
	if len(lines) != 2 || !strings.HasSuffix(r.stdout, "\n") {
		t.Fatalf("stdout = %q, want exactly two newline-terminated lines", r.stdout)
	}
	if lines[0] != "sextile v9.9.9" {
		t.Errorf("line 1 = %q", lines[0])
	}
	// The canary check inside runVersionBinary is the no-side-effects proof; a
	// run that never reached the program (a refused flag) would not print two
	// lines, so this cannot pass on a binary that ignores --version.
}

// runVersion reads the toolchain's build record through readBuildInfo, so a
// stub can carry a dirty VCS stamp the -buildvcs=false test binaries never have.
func TestVersionUsesBuildInfo(t *testing.T) {
	oldRead, oldVer := readBuildInfo, version
	t.Cleanup(func() { readBuildInfo, version = oldRead, oldVer })
	version = ""
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{Version: "(devel)"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abcdef1234567890"},
				{Key: "vcs.modified", Value: "true"},
			},
		}, true
	}
	var out bytes.Buffer
	runVersion(&out)
	if first, _, _ := strings.Cut(out.String(), "\n"); first != "sextile devel (abcdef1+dirty)" {
		t.Errorf("line 1 = %q, want %q", first, "sextile devel (abcdef1+dirty)")
	}
}

// C1 on the released app: the untagged binary, started below a go.mod whose .env
// is a FIFO with no writer, still answers --version.
func TestVersionIgnoresDotEnvOnTheReleaseBinary(t *testing.T) {
	dir := fakeAstrolog(t, fakeBanner)
	r := runVersionBinary(t, binary(t, "-X main.version=v9.9.9"), versionRun{path: dir + ":/usr/bin:/bin", dotEnvFIFO: true})
	lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
	if r.code != 0 || len(lines) != 2 || lines[0] != "sextile v9.9.9" {
		t.Errorf("exit %d, stdout %q, stderr %q; want the normal two lines", r.code, r.stdout, r.stderr)
	}
}

// The flag package accepts one dash as well as two; the single-dash form must
// answer exactly as --version does.
func TestVersionSingleDashFlag(t *testing.T) {
	dir := fakeAstrolog(t, fakeBanner)
	bin := binary(t, "-X main.version=v9.9.9")
	double := runVersionBinary(t, bin, versionRun{path: dir + ":/usr/bin:/bin"})
	single := runVersionBinary(t, bin, versionRun{path: dir + ":/usr/bin:/bin", flag: "-version"})
	if single.code != 0 || single.stderr != "" || single.stdout != double.stdout {
		t.Errorf("-version: exit %d, stdout %q, stderr %q; want exit 0 and --version's answer %q", single.code, single.stdout, single.stderr, double.stdout)
	}
	if lines := strings.Split(strings.TrimSpace(single.stdout), "\n"); len(lines) != 2 {
		t.Errorf("-version printed %q, want two lines", single.stdout)
	}
}

func TestVersionAstrologLines(t *testing.T) {
	rec := filepath.Join(t.TempDir(), "argv")
	dir := fakeAstrolog(t, `echo "$*" >> '`+rec+`'
`+fakeBanner)
	r := runVersionBinary(t, binary(t, "-X main.version=v9.9.9"), versionRun{path: dir + ":/usr/bin:/bin"})
	lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout = %q", r.stdout)
	}
	if want := "astrolog 7.99 (" + filepath.Join(dir, "astrolog") + ")"; lines[1] != want {
		t.Errorf("line 2 = %q, want %q", lines[1], want)
	}
	got, err := os.ReadFile(rec)
	if err != nil {
		t.Fatalf("the fake astrolog was never run: %v", err)
	}
	// One call: -YQ 0, then the hook-clear band (the specification
	// C1; its exact order is pinned by internal/astrolog TestEveryCallClearsHooks,
	// which cmd/sextile may not import), then -Hc.
	call := strings.TrimSuffix(string(got), "\n")
	if strings.Contains(call, "\n") || !strings.HasPrefix(call, "-YQ 0 -~") || !strings.HasSuffix(call, " -Hc") || strings.Count(call, "-~") != 54 {
		t.Errorf("astrolog argv log = %q, want exactly one call: -YQ 0, the 54 hook clears, -Hc (C1)", got)
	}
}

func TestVersionAstrologFailures(t *testing.T) {
	bin := binary(t, "-X main.version=v9.9.9")
	line2 := func(t *testing.T, r versionResult) string {
		t.Helper()
		if r.code != 0 {
			t.Errorf("exit %d, want 0", r.code)
		}
		lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
		if len(lines) != 2 {
			t.Fatalf("stdout = %q, want two lines", r.stdout)
		}
		return lines[1]
	}

	t.Run("absent", func(t *testing.T) {
		r := runVersionBinary(t, bin, versionRun{path: t.TempDir()})
		if got := line2(t, r); got != "astrolog: not found" {
			t.Errorf("line 2 = %q", got)
		}
	})

	t.Run("silent", func(t *testing.T) {
		dir := fakeAstrolog(t, "echo hello\n")
		r := runVersionBinary(t, bin, versionRun{path: dir + ":/usr/bin:/bin"})
		if got, want := line2(t, r), "astrolog: "+filepath.Join(dir, "astrolog")+" did not report a version"; got != want {
			t.Errorf("line 2 = %q, want %q", got, want)
		}
	})

	t.Run("sleeping", func(t *testing.T) {
		pidFile := filepath.Join(t.TempDir(), "pid")
		// exec: the recorded pid is the sleeping process itself, so killing it
		// leaves no child holding the output pipe open.
		dir := fakeAstrolog(t, "echo $$ > '"+pidFile+"'\nexec sleep 30\n")
		r := runVersionBinary(t, bin, versionRun{path: dir + ":/usr/bin:/bin"})
		if got, want := line2(t, r), "astrolog: "+filepath.Join(dir, "astrolog")+" did not report a version"; got != want {
			t.Errorf("line 2 = %q, want %q", got, want)
		}
		if r.elapsed < 4*time.Second || r.elapsed > 7*time.Second {
			t.Errorf("took %v, want about 5 s (limit 5 s, whole command within 7 s)", r.elapsed)
		}
		raw, err := os.ReadFile(pidFile)
		if err != nil {
			t.Fatalf("the sleeping fake never started: %v", err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for processAlive(pid) {
			if time.Now().After(deadline) {
				syscall.Kill(pid, syscall.SIGKILL)
				t.Fatalf("the fake astrolog (pid %d) was still running after --version returned", pid)
			}
			time.Sleep(50 * time.Millisecond)
		}
	})

	t.Run("banner printed, then killed", func(t *testing.T) {
		dir := fakeAstrolog(t, fakeBanner+"exec sleep 30\n")
		r := runVersionBinary(t, bin, versionRun{path: dir + ":/usr/bin:/bin"})
		if got, want := line2(t, r), "astrolog: "+filepath.Join(dir, "astrolog")+" did not report a version"; got != want {
			t.Errorf("line 2 = %q, want %q (a killed run is not an answer, whatever it printed)", got, want)
		}
		if r.elapsed > 7*time.Second {
			t.Errorf("took %v, want within 7 s", r.elapsed)
		}
	})

	t.Run("wrapper exits first, child outlives it", func(t *testing.T) {
		pidFile := filepath.Join(t.TempDir(), "child")
		dir := fakeAstrolog(t, ignoreTermChild+"echo $! > '"+pidFile+"'\n"+fakeBanner)
		t.Cleanup(func() {
			if raw, err := os.ReadFile(pidFile); err == nil {
				if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil {
					syscall.Kill(pid, syscall.SIGKILL)
				}
			}
		})
		r := runVersionBinary(t, bin, versionRun{path: dir + ":/usr/bin:/bin"})
		line2(t, r)
		if r.elapsed > 7*time.Second {
			t.Errorf("took %v, want within 7 s", r.elapsed)
		}
		// Defined: a bounded call leaves no process behind, on timeout or exit.
		raw, err := os.ReadFile(pidFile)
		if err != nil {
			t.Fatalf("the fake never recorded its child: %v", err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for processAlive(pid) {
			if time.Now().After(deadline) {
				t.Fatalf("the wrapper's child (pid %d) outlived --version after the wrapper exited", pid)
			}
			time.Sleep(50 * time.Millisecond)
		}
	})

	t.Run("wrapper exits cleanly, detached child survives", func(t *testing.T) {
		pidFile := filepath.Join(t.TempDir(), "child")
		// The child's output goes elsewhere, so the pipe closes with the wrapper
		// and cmd.Wait() succeeds: only the post-Wait group kill can end it.
		dir := fakeAstrolog(t, "sh -c 'trap \"\" TERM; while :; do sleep 1; done' >/dev/null 2>&1 </dev/null &\necho $! > '"+pidFile+"'\n"+fakeBanner+"exit 0\n")
		t.Cleanup(func() {
			if raw, err := os.ReadFile(pidFile); err == nil {
				if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil {
					syscall.Kill(pid, syscall.SIGKILL)
				}
			}
		})
		r := runVersionBinary(t, bin, versionRun{path: dir + ":/usr/bin:/bin"})
		if got, want := line2(t, r), "astrolog 7.99 ("+filepath.Join(dir, "astrolog")+")"; got != want {
			t.Errorf("line 2 = %q, want %q", got, want)
		}
		if r.elapsed > 7*time.Second {
			t.Errorf("took %v, want within 7 s", r.elapsed)
		}
		raw, err := os.ReadFile(pidFile)
		if err != nil {
			t.Fatalf("the fake never recorded its child: %v", err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for processAlive(pid) {
			if time.Now().After(deadline) {
				t.Fatalf("the detached child (pid %d) outlived a successful --version", pid)
			}
			time.Sleep(50 * time.Millisecond)
		}
	})

	t.Run("child holds the output pipe", func(t *testing.T) {
		pidFile := filepath.Join(t.TempDir(), "child")
		dir := fakeAstrolog(t, ignoreTermChild+"echo $! > '"+pidFile+"'\nwait\n")
		t.Cleanup(func() {
			if raw, err := os.ReadFile(pidFile); err == nil {
				if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil {
					syscall.Kill(pid, syscall.SIGKILL)
				}
			}
		})
		r := runVersionBinary(t, bin, versionRun{path: dir + ":/usr/bin:/bin"})
		if got, want := line2(t, r), "astrolog: "+filepath.Join(dir, "astrolog")+" did not report a version"; got != want {
			t.Errorf("line 2 = %q, want %q", got, want)
		}
		if r.elapsed > 7*time.Second {
			t.Errorf("took %v, want within 7 s: a child holding stdout kept spawn waiting", r.elapsed)
		}
		// The deadline kills the whole process group: the background child must
		// be gone by now, before the safety-net cleanup below runs.
		raw, err := os.ReadFile(pidFile)
		if err != nil {
			t.Fatalf("the fake never recorded its child: %v", err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for processAlive(pid) {
			if time.Now().After(deadline) {
				t.Fatalf("the fake's background child (pid %d) outlived --version", pid)
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
}

// processAlive is false for a gone process (ESRCH) and, where /proc exists, for
// a zombie awaiting its reaper. Without /proc a process that signals is alive.
func processAlive(pid int) bool {
	if err := syscall.Kill(pid, 0); err != nil {
		return err != syscall.ESRCH // EPERM: it exists
	}
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		if _, serr := os.Stat("/proc/self"); serr != nil {
			return true // no /proc to ask
		}
		return syscall.Kill(pid, 0) == nil // it vanished between the two looks
	}
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	return i < 0 || i+2 >= len(s) || s[i+2] != 'Z'
}

func TestUsageListsVersion(t *testing.T) {
	cmd := exec.Command(binary(t, "none"), "--help")
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin"}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("--help: %v\n%s", err, out.String())
	}
	text := out.String()
	entry := regexp.MustCompile(`(?m)^\s+-{1,2}version\b`)
	if n := len(entry.FindAllString(text, -1)); n != 1 {
		t.Errorf("--help lists --version %d times, want once:\n%s", n, text)
	}
	if !strings.Contains(text, "checks only Astrolog's version answer") {
		t.Errorf("--help does not say what --version checks:\n%s", text)
	}
}

// C5: --version and --help answer while another instance holds the lock, and
// they neither refuse nor touch anything (runVersionBinary's fingerprint).
func TestVersionAndHelpIgnoreTheLock(t *testing.T) {
	dir := fakeAstrolog(t, fakeBanner)
	bin := binary(t, "-X main.version=v9.9.9")

	// The control: without the lock held, the same run is the plain answer, and
	// with it held a launch WITHOUT these flags would be refused (that half is
	// internal/harness TestSecondInstanceRefusesAndWritesNothing).
	r := runVersionBinary(t, bin, versionRun{path: dir + ":/usr/bin:/bin", heldLock: true})
	lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
	if r.code != 0 || len(lines) != 2 || lines[0] != "sextile v9.9.9" {
		t.Errorf("--version with the lock held: exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	if strings.Contains(r.stdout+r.stderr, "already running") {
		t.Errorf("--version was refused by the lock: %q %q", r.stdout, r.stderr)
	}

	for _, flag := range []string{"--help", "-h", "-help"} {
		h := runVersionBinary(t, bin, versionRun{path: dir + ":/usr/bin:/bin", heldLock: true, flag: flag})
		if h.code != 0 {
			t.Errorf("%s with the lock held: exit %d, stderr %q", flag, h.code, h.stderr)
		}
		if !strings.Contains(h.stdout+h.stderr, "Usage:") || strings.Contains(h.stdout+h.stderr, "already running") {
			t.Errorf("%s with the lock held did not print usage: stdout %q stderr %q", flag, h.stdout, h.stderr)
		}
	}
}
