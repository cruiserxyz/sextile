//go:build linux

package launcher

// The prompt rows need a terminal. A pty is opened through /dev/ptmx with
// golang.org/x/sys/unix; the script runs with the slave as its controlling
// terminal and the test plays the person at the master.

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type session struct {
	t      *testing.T
	r      *rig
	cmd    *exec.Cmd
	master *os.File
	mu     sync.Mutex
	buf    bytes.Buffer
	done   chan struct{}
	seen   int // how much of the output earlier expect calls consumed
	so, se bytes.Buffer
}

// stdio says which of the script's standard streams are the terminal.
type stdio struct{ in, out, err bool }

var allTTY = stdio{true, true, true}

func openPty(t *testing.T) (master, slave *os.File) {
	t.Helper()
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("no /dev/ptmx: %v", err)
	}
	if err := unix.IoctlSetPointerInt(int(m.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(int(m.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	s, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = unix.IoctlSetWinsize(int(s.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: 24, Col: 80})
	return m, s
}

// startPTY starts the launcher with the chosen streams on a terminal.
func (r *rig) startPTY(io stdio, args ...string) *session {
	r.t.Helper()
	m, s := openPty(r.t)
	cmd := exec.Command(r.script(), args...)
	cmd.Dir = r.work
	cmd.Env = r.environ()
	ss := &session{t: r.t, r: r, cmd: cmd, master: m, done: make(chan struct{})}
	if io.in {
		cmd.Stdin = s
	}
	if io.out {
		cmd.Stdout = s
	} else {
		cmd.Stdout = &ss.so
	}
	if io.err {
		cmd.Stderr = s
	} else {
		cmd.Stderr = &ss.se
	}
	if io.in {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	}
	if err := cmd.Start(); err != nil {
		r.t.Fatal(err)
	}
	s.Close() // the child holds the only slave, so the master sees EIO at its exit
	go func() {
		b := make([]byte, 4096)
		for {
			n, err := m.Read(b)
			if n > 0 {
				ss.mu.Lock()
				ss.buf.Write(b[:n])
				ss.mu.Unlock()
			}
			if err != nil {
				close(ss.done)
				return
			}
		}
	}()
	r.t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		m.Close()
	})
	return ss
}

func (s *session) output() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// expect waits for text to appear after what earlier calls consumed.
func (s *session) expect(text string) {
	s.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		out := s.output()
		if i := strings.Index(out[s.seen:], text); i >= 0 {
			s.seen += i + len(text)
			return
		}
		select {
		case <-s.done:
			out = s.output()
			if i := strings.Index(out[s.seen:], text); i >= 0 {
				s.seen += i + len(text)
				return
			}
			s.t.Fatalf("the launcher ended before showing %q:\n%s", text, out)
		case <-time.After(10 * time.Millisecond):
		}
	}
	s.t.Fatalf("timed out waiting for %q:\n%s", text, s.output())
}

func (s *session) send(text string) {
	s.t.Helper()
	if _, err := s.master.WriteString(text); err != nil {
		s.t.Fatalf("typing %q: %v", text, err)
	}
}

// finish waits for the launcher and returns what was shown and the exit code.
func (s *session) finish() (string, int) {
	s.t.Helper()
	waited := make(chan error, 1)
	go func() { waited <- s.cmd.Wait() }()
	var err error
	select {
	case err = <-waited:
	case <-time.After(20 * time.Second):
		_ = s.cmd.Process.Kill()
		s.t.Fatalf("the launcher did not finish:\n%s", s.output())
	}
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
	}
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		s.t.Fatal(err)
	}
	noMismatch(s.t, s.output())
	return s.output(), code
}

// answer runs the launcher on a terminal, answering each prompt in turn.
// A reply is typed after the text that asks for it.
func (r *rig) answer(args []string, replies ...[2]string) (string, int) {
	r.t.Helper()
	// stdin and stderr are the terminal, as in a real run of the prompt;
	// stdout is a pipe of its own and must stay empty.
	s := r.startPTY(stdio{in: true, err: true}, args...)
	for _, rp := range replies {
		s.expect(rp[0])
		s.send(rp[1])
	}
	out, code := s.finish()
	if so := strings.TrimSpace(strings.ReplaceAll(s.so.String(), "#1 [internal] load build definition", "")); so != "" {
		s.t.Fatalf("the launcher wrote to stdout during the prompt flow: %q", so)
	}
	return out, code
}

const choose = "Choose [1]: "

func octal(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

// ------------------------------------------------------------------- C11b

func TestTtyFlag(t *testing.T) {
	has := func(r *rig) bool { return indexOf(r.find("run"), "-t") >= 0 }

	r := newRig(t)
	s := r.startPTY(allTTY, "--charts-dir", r.mkdir("c"))
	if _, code := s.finish(); code != 0 {
		t.Fatal("run failed")
	}
	if !has(r) {
		t.Fatalf("stdin and stdout are terminals, so -t: %q", r.find("run"))
	}

	// Only stdin.
	r = newRig(t)
	s = r.startPTY(stdio{in: true, err: true}, "--charts-dir", r.mkdir("c"))
	if _, code := s.finish(); code != 0 {
		t.Fatal("run failed")
	}
	if has(r) {
		t.Fatalf("stdout is not a terminal, so no -t: %q", r.find("run"))
	}

	// Only stdout.
	r = newRig(t)
	s = r.startPTY(stdio{out: true, err: true}, "--charts-dir", r.mkdir("c"))
	if _, code := s.finish(); code != 0 {
		t.Fatal("run failed")
	}
	if has(r) {
		t.Fatalf("stdin is not a terminal, so no -t: %q", r.find("run"))
	}

	// Neither.
	r = newRig(t)
	r.mustSucceed(r.run("--charts-dir", r.mkdir("c")))
	if has(r) {
		t.Fatalf("no terminal, no -t: %q", r.find("run"))
	}
}

// ------------------------------------------------------------------- C16

func prepareHome(t *testing.T, r *rig, docs bool) {
	t.Helper()
	if docs {
		mustMkdir(t, filepath.Join(r.home, "Documents"))
	}
}

func TestFirstRunPrompt(t *testing.T) {
	t.Run("Enter is 1", func(t *testing.T) {
		r := newRig(t)
		prepareHome(t, r, true)
		out, code := r.answer(nil, [2]string{choose, "\n"})
		if code != 0 {
			t.Fatalf("exit %d\n%s", code, out)
		}
		want := filepath.Join(r.home, ".charts")
		if got := mountSource(t, r.find("run")); got != want {
			t.Fatalf("mounted %q want %q", got, want)
		}
		contains(t, out, "Which folder?")
		contains(t, out, "1) ~/.charts")
		contains(t, out, "2) ~/Documents/charts")
		contains(t, out, "3) another folder")
		contains(t, out, "charts folder: "+want+" (created); remembered in "+r.confPath())
	})

	t.Run("a Linux home offers bin/sextile's folder as 2", func(t *testing.T) {
		r := newRig(t)
		prepareHome(t, r, true)
		mustMkdir(t, filepath.Join(r.home, ".local", "share"))
		out, code := r.answer(nil, [2]string{choose, "2\n"})
		if code != 0 {
			t.Fatalf("exit %d\n%s", code, out)
		}
		contains(t, out, "1) ~/.charts")
		contains(t, out, "2) ~/.local/share/sextile/charts")
		contains(t, out, "3) ~/Documents/charts")
		contains(t, out, "4) another folder")
		if got, want := mountSource(t, r.find("run")), filepath.Join(r.home, ".local", "share", "sextile", "charts"); got != want {
			t.Fatalf("mounted %q want %q", got, want)
		}
	})

	t.Run("an absolute XDG_DATA_HOME is the data folder", func(t *testing.T) {
		r := newRig(t)
		data := r.mkdir("data")
		r.setenv("XDG_DATA_HOME", data)
		out, code := r.answer(nil, [2]string{choose, "2\n"})
		if code != 0 {
			t.Fatalf("exit %d\n%s", code, out)
		}
		contains(t, out, "3) another folder")
		if got, want := mountSource(t, r.find("run")), filepath.Join(data, "sextile", "charts"); got != want {
			t.Fatalf("mounted %q want %q", got, want)
		}
	})

	t.Run("a relative XDG_DATA_HOME and no ~/.local/share: not offered", func(t *testing.T) {
		r := newRig(t)
		r.setenv("XDG_DATA_HOME", "rel/data")
		out, code := r.answer(nil, [2]string{choose, "\n"})
		if code != 0 {
			t.Fatalf("exit %d\n%s", code, out)
		}
		if strings.Contains(out, "sextile/charts") || !strings.Contains(out, "2) another folder") {
			t.Fatalf("the data folder is offered only when it exists:\n%s", out)
		}
	})

	t.Run("2 is Documents", func(t *testing.T) {
		r := newRig(t)
		prepareHome(t, r, true)
		_, code := r.answer(nil, [2]string{choose, "2\n"})
		if code != 0 {
			t.Fatal("failed")
		}
		if got, want := mountSource(t, r.find("run")), filepath.Join(r.home, "Documents", "charts"); got != want {
			t.Fatalf("mounted %q want %q", got, want)
		}
	})

	t.Run("no Documents, no option 2", func(t *testing.T) {
		r := newRig(t)
		out, code := r.answer(nil, [2]string{choose, "\n"})
		if code != 0 {
			t.Fatalf("exit %d\n%s", code, out)
		}
		if strings.Contains(out, "Documents") || !strings.Contains(out, "2) another folder") {
			t.Fatalf("option 2 is Documents only when ~/Documents exists:\n%s", out)
		}
	})

	t.Run("an existing folder says how many charts", func(t *testing.T) {
		r := newRig(t)
		prepareHome(t, r, true)
		for _, n := range []string{"a.as", "b.as", "c.as"} {
			mustWrite(t, filepath.Join(r.home, ".charts", n), "@AI780\n", 0o644)
		}
		mustWrite(t, filepath.Join(r.home, ".charts", "notes.txt"), "x", 0o644)
		out, code := r.answer(nil, [2]string{choose, "\n"})
		if code != 0 {
			t.Fatal("failed")
		}
		contains(t, out, "holds 3 charts")
		contains(t, out, "(exists)")
	})

	t.Run("an empty existing folder", func(t *testing.T) {
		r := newRig(t)
		mustMkdir(t, filepath.Join(r.home, ".charts"))
		out, _ := r.answer(nil, [2]string{choose, "\n"})
		contains(t, out, "exists, no charts yet")
	})

	t.Run("3 and a path: ~ expanded", func(t *testing.T) {
		r := newRig(t)
		prepareHome(t, r, true)
		_, code := r.answer(nil, [2]string{choose, "3\n"}, [2]string{"Folder: ", "~/my/charts\n"})
		if code != 0 {
			t.Fatal("failed")
		}
		if got, want := mountSource(t, r.find("run")), filepath.Join(r.home, "my", "charts"); got != want {
			t.Fatalf("mounted %q want %q", got, want)
		}
	})

	t.Run("3 and a relative path: from the launch directory", func(t *testing.T) {
		r := newRig(t)
		prepareHome(t, r, true)
		_, code := r.answer(nil, [2]string{choose, "3\n"}, [2]string{"Folder: ", "here/charts\n"})
		if code != 0 {
			t.Fatal("failed")
		}
		if got, want := mountSource(t, r.find("run")), filepath.Join(r.work, "here", "charts"); got != want {
			t.Fatalf("mounted %q want %q", got, want)
		}
	})

	t.Run("3 and the path on the same line", func(t *testing.T) {
		r := newRig(t)
		prepareHome(t, r, true)
		_, code := r.answer(nil, [2]string{choose, "3 ~/inline\n"})
		if code != 0 {
			t.Fatal("failed")
		}
		if got, want := mountSource(t, r.find("run")), filepath.Join(r.home, "inline"); got != want {
			t.Fatalf("mounted %q want %q", got, want)
		}
	})
}

// D6. The first-run prompt counts the charts a folder holds by their first
// three bytes, the @AI header, and by nothing in their names: three files
// named .as or otherwise count; a settings file, a file named .as with no
// header, a text file, a dotfile, a folder and a note that merely mentions the
// header do not.
func TestLauncherCountsChartsByHeader(t *testing.T) {
	r := newRig(t)
	prepareHome(t, r, true)
	dir := filepath.Join(r.home, ".charts")
	for _, n := range []string{"a.as", "b.as", "c.legacy"} {
		mustWrite(t, filepath.Join(dir, n), "@AI780  ; Astrolog chart info.\n", 0o644)
	}
	mustWrite(t, filepath.Join(dir, "notes.txt"), "birth data for later\n", 0o644)
	mustWrite(t, filepath.Join(dir, "mentions.txt"), "see the @AI780 header\n", 0o644)
	mustWrite(t, filepath.Join(dir, "settings.as"), "@AD780  ; Astrolog defaults.\n", 0o644)
	mustWrite(t, filepath.Join(dir, "plain.as"), "no header here\n", 0o644)
	mustWrite(t, filepath.Join(dir, ".hidden.as"), "@AI780\n", 0o644)
	mustMkdir(t, filepath.Join(dir, "sub.as"))
	out, code := r.answer(nil, [2]string{choose, "\n"})
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	contains(t, out, "holds 3 charts")
}

// Launched from inside the clone, as the README's "cd sextile" leaves a person,
// a relative answer would put client charts in the checkout.
func TestPromptRefusesFolderInsideClone(t *testing.T) {
	r := newRig(t)
	r.setenv("FAKE_DOCKER_IMAGE", "absent")
	prepareHome(t, r, true)
	r.work = r.clone // the launch directory
	before := snapshot(t, r.root)
	out, code := r.answer(nil, [2]string{choose, "3\n"}, [2]string{"Folder: ", "charts\n"})
	if code != 2 {
		t.Fatalf("want exit 2, got %d\n%s", code, out)
	}
	contains(t, out, filepath.Join(r.clone, "charts")+" is inside the Sextile checkout ("+r.clone+"); keep client charts outside it")
	if _, err := os.Stat(filepath.Join(r.clone, "charts")); err == nil {
		t.Fatal("the folder inside the clone was created")
	}
	if _, err := os.Stat(filepath.Dir(r.confPath())); err == nil {
		t.Fatal("a config was written")
	}
	r.noBuildOrRun()
	for _, x := range diffSnapshots(before, snapshot(t, r.root)) {
		if !strings.Contains(x, "docker.log") {
			t.Fatalf("something was created or changed: %s", x)
		}
	}
}

// ------------------------------------------------------------------ C16c

func TestPromptCreatesFolder(t *testing.T) {
	r := newRig(t)
	prepareHome(t, r, true)
	// An existing parent keeps its mode; each folder the launcher makes is 0700.
	parent := filepath.Join(r.root, "parent")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "a", "b", "c")
	_, code := r.answer(nil, [2]string{choose, "3\n"}, [2]string{"Folder: ", target + "\n"})
	if code != 0 {
		t.Fatal("failed")
	}
	if got := octal(t, parent); got != 0o755 {
		t.Fatalf("an existing folder's mode changed to %v", got)
	}
	for _, d := range []string{"a", "a/b", "a/b/c"} {
		if got := octal(t, filepath.Join(parent, d)); got != 0o700 {
			t.Fatalf("created folder %s has mode %v, want 0700", d, got)
		}
	}

	// An existing chosen folder is left as it is.
	r2 := newRig(t)
	existing := filepath.Join(r2.home, ".charts")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, code := r2.answer(nil, [2]string{choose, "\n"}); code != 0 {
		t.Fatal("failed")
	}
	if got := octal(t, existing); got != 0o755 {
		t.Fatalf("existing folder mode changed to %v", got)
	}
}

// ------------------------------------------------------- C16d, C23

func TestPromptWritesConfig(t *testing.T) {
	r := newRig(t)
	r.setenv("FAKE_DOCKER_IMAGE", "absent") // so a build happens, and the order shows
	r.setenv("FAKE_DOCKER_PROBE", r.confPath())
	prepareHome(t, r, true)
	// A config folder that already exists with a loose mode ends private.
	loose := filepath.Dir(r.confPath())
	if err := os.Mkdir(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, r.root)
	chosen := filepath.Join(r.root, "picked", "deep")
	out, code := r.answer(nil, [2]string{choose, "3\n"}, [2]string{"Folder: ", chosen + "\n"})
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	b, err := os.ReadFile(r.confPath())
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, l := range strings.Split(string(b), "\n") {
		if l != "" && !strings.HasPrefix(l, "#") {
			kept = append(kept, l)
		}
	}
	if len(kept) != 1 || kept[0] != "charts-dir="+chosen {
		t.Fatalf("config content %q", b)
	}
	if got := octal(t, r.confPath()); got != 0o600 {
		t.Fatalf("config mode %v, want 0600", got)
	}
	if got := octal(t, filepath.Dir(r.confPath())); got != 0o700 {
		t.Fatalf("config dir mode %v, want 0700", got)
	}
	probe, _ := os.ReadFile(r.log + ".probe")
	if string(probe) != "build:yes\nrun:yes\n" {
		t.Fatalf("the config must exist before the build and the run: %q", probe)
	}

	// C23: the launcher wrote the config and the folder chosen at the prompt
	// (the folder and its new parents, as folders) and nothing else, anywhere
	// under the temporary root: no file in the clone, HOME or the charts folder.
	after := snapshot(t, r.root)
	confDir := filepath.Dir(r.confPath())
	if bad := unexpectedWrites(before, after, func(entry, path string) bool {
		if path == confDir || path == r.confPath() {
			return true
		}
		if strings.HasPrefix(path, confDir+string(filepath.Separator)+".launcher.conf.") {
			return false // a leftover temporary file
		}
		isDir := strings.HasPrefix(after[path], "d")
		return isDir && strings.HasPrefix(entry, "added") && (path == chosen || strings.HasPrefix(chosen, path+string(filepath.Separator)))
	}); len(bad) != 0 {
		t.Fatalf("the launcher wrote more than the config and the chosen folder: %v", bad)
	}

	// The next start needs no terminal: it reads the remembered folder.
	r.log = filepath.Join(r.root, "second.log")
	r.mustSucceed(r.run())
	if got := mountSource(t, r.find("run")); got != chosen {
		t.Fatalf("second start mounted %q want %q", got, chosen)
	}
}

// A failed config write leaves no temporary file behind, and says so.
func TestConfigWriteFailureLeavesNoTemp(t *testing.T) {
	for _, failing := range []string{"mv", "chmod"} {
		r := newRig(t)
		prepareHome(t, r, true)
		r.setenv("FAKE_TOOL_FAIL", failing)
		out, code := r.answer(nil, [2]string{choose, "\n"})
		if code != 2 {
			t.Fatalf("%s failing: want exit 2, got %d\n%s", failing, code, out)
		}
		contains(t, out, r.confPath())
		r.noBuildOrRun()
		entries, err := os.ReadDir(filepath.Dir(r.confPath()))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			t.Fatalf("%s failing: %s was left in the config folder", failing, e.Name())
		}
	}
}

// ------------------------------------------------------------------ C16e

func TestPromptPathRejected(t *testing.T) {
	cases := map[string]func(r *rig) string{
		"comma":               func(r *rig) string { return filepath.Join(r.root, "new", "a,b") },
		"existing non-folder": func(r *rig) string { p := filepath.Join(r.root, "afile"); mustWrite(t, p, "x", 0o644); return p },
		"parent is a file": func(r *rig) string {
			p := filepath.Join(r.root, "afile2")
			mustWrite(t, p, "x", 0o644)
			return filepath.Join(p, "sub")
		},
		"comma under new tree": func(r *rig) string { return filepath.Join(r.root, "tree", "x", "a,b") },
		"~user":                func(r *rig) string { return "~someone/charts" },
	}
	for name, mk := range cases {
		r := newRig(t)
		r.setenv("FAKE_DOCKER_IMAGE", "absent")
		prepareHome(t, r, true)
		path := mk(r)
		before := snapshot(t, r.root)
		out, code := r.answer(nil, [2]string{choose, "3\n"}, [2]string{"Folder: ", path + "\n"})
		if code != 2 {
			t.Fatalf("%s: want exit 2, got %d\n%s", name, code, out)
		}
		r.noBuildOrRun()
		d := diffSnapshots(before, snapshot(t, r.root))
		// The only difference allowed is the docker call log itself.
		for _, x := range d {
			if !strings.Contains(x, "docker.log") {
				t.Fatalf("%s: something was created or changed: %v", name, d)
			}
		}
		if _, err := os.Stat(filepath.Dir(r.confPath())); err == nil {
			t.Fatalf("%s: a config folder appeared", name)
		}
	}
}

// ------------------------------------------------------------------ C16b

func TestPromptBadAnswer(t *testing.T) {
	r := newRig(t)
	r.setenv("FAKE_DOCKER_IMAGE", "absent")
	prepareHome(t, r, true)
	before := snapshot(t, r.root)
	out, code := r.answer(nil, [2]string{choose, "x\n"}, [2]string{choose, "4\n"}, [2]string{choose, "zzz\n"})
	if code != 2 {
		t.Fatalf("three bad answers must end in exit 2, got %d\n%s", code, out)
	}
	r.noBuildOrRun()
	for _, x := range diffSnapshots(before, snapshot(t, r.root)) {
		if !strings.Contains(x, "docker.log") {
			t.Fatalf("a refused prompt wrote: %s", x)
		}
	}
	if strings.Count(out, choose) != 3 {
		t.Fatalf("asked at most three times:\n%s", out)
	}

	// Two bad answers then a good one still works.
	r2 := newRig(t)
	prepareHome(t, r2, true)
	_, code = r2.answer(nil, [2]string{choose, "x\n"}, [2]string{choose, "7\n"}, [2]string{choose, "1\n"})
	if code != 0 {
		t.Fatalf("a good third answer is accepted, got exit %d", code)
	}

	// Without ~/Documents the range is 1-2: 3 is bad.
	r3 := newRig(t)
	out, code = r3.answer(nil, [2]string{choose, "3\n"}, [2]string{choose, "3\n"}, [2]string{choose, "3\n"})
	if code != 2 {
		t.Fatalf("with two options, 3 is not an answer; exit %d\n%s", code, out)
	}

	// End of input is a refusal, not a loop.
	r4 := newRig(t)
	s := r4.startPTY(allTTY)
	s.expect(choose)
	s.send("\x04") // Ctrl+D on an empty line
	if _, code := s.finish(); code != 2 {
		t.Fatalf("EOF at the prompt: exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(r4.home, ".charts")); err == nil {
		t.Fatal("EOF must create nothing")
	}
}

// ------------------------------------------------------------------ C18b

func TestConfigLocationRefused(t *testing.T) {
	type tc struct {
		name string
		set  func(r *rig) (wantInMessage string)
	}
	cases := []tc{
		{"relative XDG_CONFIG_HOME", func(r *rig) string {
			r.setenv("XDG_CONFIG_HOME", "relative/cfg")
			return "XDG_CONFIG_HOME"
		}},
		{"inside the clone", func(r *rig) string {
			r.setenv("XDG_CONFIG_HOME", filepath.Join(r.clone, "cfg"))
			return "inside"
		}},
		{"a link that leads into the clone", func(r *rig) string {
			mustMkdir(t, filepath.Join(r.clone, "cfg"))
			link := filepath.Join(r.root, "link")
			if err := os.Symlink(filepath.Join(r.clone, "cfg"), link); err != nil {
				t.Fatal(err)
			}
			r.setenv("XDG_CONFIG_HOME", link)
			return "inside"
		}},
		{"config folder is a symlink", func(r *rig) string {
			elsewhere := r.mkdir("elsewhere")
			if err := os.Symlink(elsewhere, filepath.Join(r.xdg, "sextile")); err != nil {
				t.Fatal(err)
			}
			return filepath.Join(r.xdg, "sextile")
		}},
		{"config file is a symlink", func(r *rig) string {
			mustMkdir(t, filepath.Join(r.xdg, "sextile"))
			target := filepath.Join(r.root, "real.conf")
			mustWrite(t, target, "charts-dir="+r.mkdir("ok")+"\n", 0o600)
			if err := os.Symlink(target, r.confPath()); err != nil {
				t.Fatal(err)
			}
			return r.confPath()
		}},
	}
	for _, c := range cases {
		for _, tty := range []bool{false, true} {
			r := newRig(t)
			r.setenv("FAKE_DOCKER_IMAGE", "absent")
			want := c.set(r)
			before := snapshot(t, r.root)
			var out string
			var code int
			if tty {
				s := r.startPTY(allTTY)
				out, code = s.finish()
				// The person is never asked: no prompt was shown.
				if strings.Contains(out, "Which folder?") {
					t.Fatalf("%s: prompted despite the bad location:\n%s", c.name, out)
				}
			} else {
				res := r.run()
				out, code = res.stderr, res.code
			}
			if code != 2 {
				t.Fatalf("%s (tty=%v): want exit 2, got %d\n%s", c.name, tty, code, out)
			}
			if !strings.Contains(out, want) {
				t.Fatalf("%s (tty=%v): the message must name %q:\n%s", c.name, tty, want, out)
			}
			r.noBuildOrRun()
			for _, x := range diffSnapshots(before, snapshot(t, r.root)) {
				if !strings.Contains(x, "docker.log") {
					t.Fatalf("%s (tty=%v): something was written: %s", c.name, tty, x)
				}
			}
		}
	}
}

// ------------------------------------------------------------------ C22b

func TestDoctorExit(t *testing.T) {
	type tc struct {
		name string
		make func(r *rig) []string
		want int
	}
	freeP := func() string { return strconv.Itoa(freePort(t)) }
	cases := []tc{
		{"clean", func(r *rig) []string {
			r.writeConf("charts-dir=" + r.mkdir("kept") + "\n")
			return []string{"--port", freeP()}
		}, 0},
		{"no docker", func(r *rig) []string { r.removeDocker(); return []string{"--charts-dir", r.mkdir("c")} }, 1},
		{"permission denied", func(r *rig) []string {
			r.setenv("FAKE_DOCKER_DENIED", "1")
			return []string{"--charts-dir", r.mkdir("c")}
		}, 1},
		{"daemon down", func(r *rig) []string {
			r.setenv("FAKE_DOCKER_FAIL", "version")
			return []string{"--charts-dir", r.mkdir("c")}
		}, 1},
		{"too old", func(r *rig) []string {
			r.setenv("FAKE_DOCKER_SERVER", "27.0.0")
			return []string{"--charts-dir", r.mkdir("c")}
		}, 1},
		{"remote endpoint", func(r *rig) []string {
			r.setenv("FAKE_DOCKER_ENDPOINT", "tcp://h:2375")
			return []string{"--charts-dir", r.mkdir("c")}
		}, 1},
		{"port busy", func(r *rig) []string {
			return []string{"--charts-dir", r.mkdir("c"), "--port", strconv.Itoa(busyPort(t))}
		}, 1},
		{"invalid config", func(r *rig) []string { r.writeConf("nonsense\n"); return nil }, 1},
		{"missing remembered folder", func(r *rig) []string {
			r.writeConf("charts-dir=" + filepath.Join(r.root, "gone") + "\n")
			return nil
		}, 1},
		// Notes do not fail it.
		{"image not built", func(r *rig) []string {
			r.setenv("FAKE_DOCKER_IMAGE", "absent")
			return []string{"--charts-dir", r.mkdir("c"), "--port", freeP()}
		}, 0},
		{"no time zone", func(r *rig) []string { return []string{"--charts-dir", r.mkdir("c"), "--port", freeP()} }, 0},
	}
	for _, c := range cases {
		r := newRig(t)
		args := c.make(r)
		res := r.run(append([]string{"doctor"}, args...)...)
		if res.code != c.want {
			t.Fatalf("%s: doctor exit %d, want %d\n%s%s", c.name, res.code, c.want, res.stdout, res.stderr)
		}
		if c.want == 1 && !regexp.MustCompile(`(?m)^FAIL `).MatchString(res.stdout) {
			t.Fatalf("%s: exit 1 needs a FAIL line:\n%s", c.name, res.stdout)
		}
		if c.want == 0 && strings.Contains(res.stdout, "FAIL") {
			t.Fatalf("%s: exit 0 with a FAIL line:\n%s", c.name, res.stdout)
		}
	}

	// No folder known: a FAIL without a terminal, a note at one.
	r := newRig(t)
	res := r.run("doctor", "--port", freeP())
	if res.code != 1 || !regexp.MustCompile(`(?m)^FAIL +charts folder:`).MatchString(res.stdout) {
		t.Fatalf("no folder and no terminal: exit %d\n%s", res.code, res.stdout)
	}
	r = newRig(t)
	s := r.startPTY(allTTY, "doctor", "--port", freeP())
	out, code := s.finish()
	if code != 0 || !regexp.MustCompile(`(?m)^note +charts folder:`).MatchString(out) {
		t.Fatalf("no folder at a terminal is a note: exit %d\n%s", code, out)
	}
}

// ---------------------------------------------------------- the goldens

// frame shows the screen as a person saw it.
func (r *rig) frame(title, out string, code int) string {
	return fmt.Sprintf("=== %s ===\n%s\n[exit %d]\n\n", title, strings.TrimRight(r.norm(out), "\n"), code)
}

func TestPromptGolden(t *testing.T) {
	var sb strings.Builder

	r := newRig(t)
	prepareHome(t, r, true)
	out, code := r.answer(nil, [2]string{choose, "\n"})
	sb.WriteString(r.frame("first-run-empty (Enter)", out, code))

	r = newRig(t)
	prepareHome(t, r, true)
	for _, n := range []string{"Alice.as", "Bob.as", "Carol.as"} {
		mustWrite(t, filepath.Join(r.home, ".charts", n), "@AI780\n", 0o644)
	}
	out, code = r.answer(nil, [2]string{choose, "\n"})
	sb.WriteString(r.frame("first-run-existing (Enter, ~/.charts holds 3 charts)", out, code))

	r = newRig(t)
	prepareHome(t, r, true)
	mustMkdir(t, filepath.Join(r.home, ".local", "share"))
	out, code = r.answer(nil, [2]string{choose, "2\n"})
	sb.WriteString(r.frame("first-run-linux-home (2, ~/.local/share exists)", out, code))

	r = newRig(t)
	prepareHome(t, r, true)
	out, code = r.answer(nil, [2]string{choose, "3\n"}, [2]string{"Folder: ", "~/work/clients\n"})
	sb.WriteString(r.frame("first-run-custom (3, then a path)", out, code))

	golden(t, "prompt", sb.String())
}

func TestDoctorGolden(t *testing.T) {
	var sb strings.Builder
	norm := func(r *rig, s string, p int) string {
		s = r.norm(s)
		s = strings.ReplaceAll(s, strconv.Itoa(p+1), "5308")
		return strings.ReplaceAll(s, strconv.Itoa(p), "5307")
	}
	setup := func() (*rig, int) {
		r := newRig(t)
		mustWrite(t, filepath.Join(r.sys, "etc/timezone"), "Europe/Amsterdam\n", 0o644)
		r.setImages("img aaaa00000001 1 localhost/sextile:v0.1.0-rc.1", "img bbbb00000002 1 localhost/sextile:v0.0.9")
		r.writeConf("charts-dir=" + r.mkdir("charts") + "\n")
		return r, freePort(t)
	}

	r, p := setup()
	res := r.run("doctor", "--port", strconv.Itoa(p))
	sb.WriteString(r.frame("doctor-clean", norm(r, res.stdout+res.stderr, p), res.code))

	r, _ = setup()
	p = busyPort(t)
	res = r.run("doctor", "--port", strconv.Itoa(p))
	sb.WriteString(r.frame("doctor-port-busy", norm(r, res.stdout+res.stderr, p), res.code))

	r, p = setup()
	r.removeDocker()
	res = r.run("doctor", "--port", strconv.Itoa(p))
	sb.WriteString(r.frame("doctor-no-docker", norm(r, res.stdout+res.stderr, p), res.code))

	golden(t, "doctor", sb.String())
}

func TestMessagesGolden(t *testing.T) {
	var sb strings.Builder
	add := func(title string, r *rig, p int, res result) {
		out := r.norm(res.stderr + res.stdout)
		if p > 0 {
			out = strings.ReplaceAll(out, strconv.Itoa(p+1), "5308")
			out = strings.ReplaceAll(out, strconv.Itoa(p), "5307")
		}
		sb.WriteString(r.frame(title, out, res.code))
	}

	r := newRig(t, plainClone())
	r.setenv("FAKE_DOCKER_IMAGE", "absent")
	add("build note: no git history", r, 0, r.run("--charts-dir", r.mkdir("c")))

	r = newRig(t)
	mustWrite(t, filepath.Join(r.clone, "marker.txt"), "edited\n", 0o644)
	add("build note: uncommitted changes", r, 0, r.run("--charts-dir", r.mkdir("c")))

	r = newRig(t)
	p := busyPort(t)
	add("busy port", r, p, r.run("--charts-dir", r.mkdir("c"), "--port", strconv.Itoa(p)))

	r = newRig(t)
	add("no terminal, no folder chosen", r, 0, r.run())

	r = newRig(t)
	r.setenv("FAKE_DOCKER_ENDPOINT", "tcp://build-host:2375")
	add("remote endpoint", r, 0, r.run("--charts-dir", r.mkdir("c")))

	r = newRig(t)
	r.setenv("XDG_CONFIG_HOME", "relative/cfg")
	add("config: relative XDG_CONFIG_HOME", r, 0, r.run())

	r = newRig(t)
	r.setenv("XDG_CONFIG_HOME", filepath.Join(r.clone, "cfg"))
	add("config: inside the clone", r, 0, r.run())

	r = newRig(t)
	if err := os.Symlink(r.mkdir("elsewhere"), filepath.Join(r.xdg, "sextile")); err != nil {
		t.Fatal(err)
	}
	add("config: folder is a symlink", r, 0, r.run())

	r = newRig(t)
	r.writeConf("charts-dir=a\nmore\n")
	add("config: not valid", r, 0, r.run())

	r = newRig(t)
	r.writeConf("charts-dir=" + filepath.Join(r.root, "usb", "charts") + "\n")
	add("config: remembered folder is gone", r, 0, r.run())

	r = newRig(t)
	r.setenv("FAKE_DOCKER_DENIED", "1")
	add("docker: permission denied", r, 0, r.run())

	r = newRig(t)
	add("forwarded --charts-dir", r, 0, r.run("--", "--charts-dir=/elsewhere"))

	r = newRig(t)
	r.withAstrolog(false)
	add("--no-docker: go absent", r, 0, r.run("--no-docker", "--charts-dir", r.mkdir("c")))

	r = newRig(t)
	add("--no-docker: go and astrolog absent", r, 0, r.run("--no-docker", "--charts-dir", r.mkdir("c")))

	r = newRig(t)
	r.withGo()
	r.withAstrolog(false)
	r.setenv("FAKE_GO_FAIL", "1")
	add("--no-docker: the build fails", r, 0, r.run("--no-docker", "--charts-dir", r.mkdir("c")))

	r = newRig(t, plainClone())
	r.withGo()
	r.withAstrolog(false)
	res := r.run("--no-docker", "--charts-dir", r.mkdir("c"), "--no-open")
	add("--no-docker: no git history", r, 0, result{stderr: res.stderr, code: res.code})

	golden(t, "messages", sb.String())
}

// --------------------------------------------- try feedback, 2026-10-23

// stalledPort returns a loopback port whose connects hang: a listener with a
// zero backlog whose queue is already full drops further SYNs, as a port can
// on some WSL networking modes. It skips where a connect does not stall.
func stalledPort(t *testing.T) int {
	t.Helper()
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Skipf("socket: %v", err)
	}
	t.Cleanup(func() { unix.Close(fd) })
	if err := unix.Bind(fd, &unix.SockaddrInet4{Addr: [4]byte{127, 0, 0, 1}}); err != nil {
		t.Skipf("bind: %v", err)
	}
	if err := unix.Listen(fd, 0); err != nil {
		t.Skipf("listen: %v", err)
	}
	sa, err := unix.Getsockname(fd)
	if err != nil {
		t.Fatal(err)
	}
	port := sa.(*unix.SockaddrInet4).Port
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	for i := 0; i < 4; i++ {
		if c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
			t.Cleanup(func() { c.Close() })
		}
	}
	if c, err := net.DialTimeout("tcp", addr, 3*time.Second); err == nil {
		c.Close()
		t.Skip("a connect to a full listen queue does not stall here")
	}
	return port
}

// The operator's first run sat about two minutes with nothing on screen
// before the charts prompt. An unanswered port probe waits out the kernel's
// SYN retries (about 127 s); the probe is bounded to two seconds.
func TestPortProbeIsBounded(t *testing.T) {
	p := stalledPort(t)
	r := newRig(t)
	start := time.Now()
	res := r.run("--charts-dir", r.mkdir("c"), "--port", strconv.Itoa(p))
	if took := time.Since(start); took > 10*time.Second {
		t.Fatalf("the port probe took %s against a port that does not answer", took)
	}
	r.mustSucceed(res)
}

// No wait is silent on a terminal: each step says what it is doing, before
// the prompt and before Docker starts. Into a pipe the launcher stays quiet.
func TestProgressOnTerminal(t *testing.T) {
	r := newRig(t)
	prepareHome(t, r, true)
	out, code := r.answer(nil, [2]string{choose, "\n"})
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	order := []string{"checking Docker", "checking that port", "Sextile keeps your charts", "starting Sextile (charts: "}
	at := -1
	for _, want := range order {
		i := strings.Index(out, want)
		if i < 0 || i < at {
			t.Fatalf("want %q after the earlier steps %q:\n%s", want, order, out)
		}
		at = i
	}

	r = newRig(t)
	res := r.run("--charts-dir", r.mkdir("c"))
	r.mustSucceed(res)
	if strings.Contains(res.stderr, "checking") || strings.Contains(res.stderr, "starting Sextile") {
		t.Fatalf("progress lines are for a terminal, not a pipe: %q", res.stderr)
	}
}
