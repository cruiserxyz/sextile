// Command tour records and plays a key file in a terminal: the Sextile Tour's
// keys, one step to a line, with the pause after each.
//
//	go run ./tools/tour record FILE    run Sextile and write every key you press to FILE
//	go run ./tools/tour play FILE      run Sextile and press the keys of FILE at their pace
//
// Both run the program on the tour's own chart in a session of its own, in a
// folder that is made for the run, so nothing of yours is read or written. The
// capture lane (internal/ui, tour_capture_test.go) plays the same file into
// docs/tour.svg. This tool is not part of the released program or the image.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/tour"
)

const usage = `usage: tour record [-bin PATH] FILE
       tour play   [-bin PATH] FILE

record runs Sextile and writes every key you press, with its pause, to FILE.
Everything you type is written there, text typed into a search or a name field
included. FILE must not exist and must not be inside a charts folder. Ctrl+]
ends the recording.

play runs Sextile and presses the keys of FILE at their pace.

-bin PATH runs the Sextile at PATH. Without it the tool builds ./cmd/sextile,
so run it from the repository root.

`

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin, stdout, stderr *os.File) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(stdout, usage+tour.Format+"\n")
		return 0
	}
	verb, rest := args[0], args[1:]
	var bin string
	var file string
	for i := 0; i < len(rest); i++ {
		switch {
		case rest[i] == "-bin" || rest[i] == "--bin":
			if i+1 >= len(rest) {
				fmt.Fprint(stderr, "tour: -bin needs a path\n")
				return 2
			}
			bin = rest[i+1]
			i++
		case file == "" && !strings.HasPrefix(rest[i], "-"):
			file = rest[i]
		default:
			fmt.Fprintf(stderr, "tour: unexpected %q\n\n%s", rest[i], usage)
			return 2
		}
	}
	if (verb != "record" && verb != "play") || file == "" {
		fmt.Fprint(stderr, usage)
		return 2
	}
	var err error
	if verb == "record" {
		err = record(file, bin, stdin, stdout, stderr)
	} else {
		err = play(file, bin, stdout)
	}
	if err != nil {
		fmt.Fprintf(stderr, "tour: %v\n", err)
		return 1
	}
	return 0
}

// userCharts is the charts folder Sextile reads for this user, as an absolute
// path, read before the tool points the program at its own folder.
func userCharts() (string, error) { return filepath.Abs(store.ChartsDir()) }

// refuseCharts is an error when file is not a recording's name (it must end in
// .keys) or would land in a charts folder: charts, the one Sextile reads for
// this user, or a folder that holds a chart file. A path the check cannot place
// is refused too.
func refuseCharts(file, charts string) error {
	if filepath.Ext(file) != ".keys" {
		return fmt.Errorf("%s is not a recording's name. Name it so that it ends in .keys", file)
	}
	return refuseChartsFolder(filepath.Dir(file), file, charts)
}

// refuseChartsFolder is an error when dir is inside charts, or dir itself holds
// a chart file (its first bytes are @AI), so a recording or the tool's own
// folder never lands among a person's charts. Sextile reads a charts folder's
// own files, not its subfolders. what names the path in the error.
func refuseChartsFolder(dir, what, charts string) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	if real, err := filepath.EvalSymlinks(charts); err == nil {
		charts = real
	}
	rel, err := filepath.Rel(charts, dir)
	if err != nil {
		return fmt.Errorf("cannot tell whether %s is inside the charts folder %s, so nothing is written there", what, charts)
	}
	inside := rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	if inside || holdsChart(dir) {
		return fmt.Errorf("%s is inside a charts folder. A recording holds what you typed, so it does not belong there", what)
	}
	return nil
}

// holdsChart says whether dir holds a file whose first bytes are a chart's.
func holdsChart(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		f, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		head := make([]byte, 3)
		n, _ := f.Read(head)
		f.Close()
		if n == 3 && string(head) == "@AI" {
			return true
		}
	}
	return false
}

// tourZone is the time zone of the tour's chart and pictures: Amsterdam's.
const tourZone = "Europe/Amsterdam"

// stage is what a run needs: the folder made for it, and the command to run.
type stage struct {
	root string
	cmd  *exec.Cmd
}

func (s *stage) close() { _ = os.RemoveAll(s.root) }

// newStage makes the run's folders, the tour chart and the session that holds
// it, and builds the command that runs Sextile there.
func newStage(bin, session string) (*stage, error) {
	mine, err := userCharts()
	if err != nil {
		return nil, err
	}
	if err := refuseChartsFolder(os.TempDir(), "the temporary folder "+os.TempDir(), mine); err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp("", "sextile-tour-")
	if err != nil {
		return nil, err
	}
	s := &stage{root: root}
	fail := func(err error) (*stage, error) { s.close(); return nil, err }

	if bin == "" {
		if _, err := os.Stat(filepath.Join("cmd", "sextile")); err != nil {
			return fail(errors.New("no -bin given and ./cmd/sextile is not here: run the tool from the repository root"))
		}
		bin = filepath.Join(root, "sextile")
		build := exec.Command("go", "build", "-o", bin, "./cmd/sextile")
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			return fail(fmt.Errorf("building ./cmd/sextile: %w", err))
		}
	}

	charts := filepath.Join(root, "charts")
	if err := os.MkdirAll(charts, 0o700); err != nil {
		return fail(err)
	}
	if err := os.WriteFile(filepath.Join(charts, tour.ChartFile), tour.ChartText(), 0o600); err != nil {
		return fail(err)
	}
	env := map[string]string{
		"SEXTILE_CHARTS_DIR":     charts,
		"SEXTILE_DEV_CHARTS_DIR": "",
		"XDG_DATA_HOME":          filepath.Join(root, "data"),
		"XDG_STATE_HOME":         filepath.Join(root, "state"),
		"XDG_CONFIG_HOME":        filepath.Join(root, "config"),
		"XDG_CACHE_HOME":         filepath.Join(root, "cache"),
	}
	for k, v := range env {
		if err := os.Setenv(k, v); err != nil {
			return fail(err)
		}
	}
	snap := tour.Session(charts)
	args := []string(nil)
	if session != "" {
		lock, err := store.SaveNamedSessionAs(session, snap)
		if err != nil {
			return fail(fmt.Errorf("starting the session %q: %w", session, err))
		}
		lock.Release()
		args = []string{"--session", session}
	} else if err := store.SaveSession(snap); err != nil {
		return fail(fmt.Errorf("starting the session: %w", err))
	}
	s.cmd = exec.Command(bin, args...)
	// The program runs in the tour's own time zone, as its pictures do. A
	// machine whose zone names no city (Etc/UTC, as on GitHub's runners) would
	// start Sextile asking for its city, and the keys would go there.
	s.cmd.Env = append(os.Environ(), "TZ="+tourZone)
	if os.Getenv("TERM") == "" {
		s.cmd.Env = append(s.cmd.Env, "TERM=xterm-256color")
	}
	return s, nil
}

// pty is a terminal for the program to run in.
type pty struct{ master, slave *os.File }

func openPty(rows, cols uint16) (*pty, error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}
	if err := unix.IoctlSetPointerInt(int(m.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		return nil, err
	}
	n, err := unix.IoctlGetInt(int(m.Fd()), unix.TIOCGPTN)
	if err != nil {
		return nil, err
	}
	s, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}
	if err := unix.IoctlSetWinsize(int(s.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: rows, Col: cols}); err != nil {
		return nil, err
	}
	return &pty{m, s}, nil
}

// size is the terminal's size, or the lane's 132 by 40 when stdout is no terminal.
func size(out *os.File) (rows, cols uint16) {
	if ws, err := unix.IoctlGetWinsize(int(out.Fd()), unix.TIOCGWINSZ); err == nil && ws.Row > 0 && ws.Col > 0 {
		return ws.Row, ws.Col
	}
	return 40, 132
}

// fullScreen is what the program writes when its screen takes the terminal
// over. Before it, Sextile asks the terminal for its colours in raw mode of its
// own, and reads what comes in as the answer.
var fullScreen = []byte("\x1b[?1049h")

// screen tells when the program's screen first shows, or that it ended first.
type screen struct {
	shown chan struct{} // closed when fullScreen first passes through
	gone  chan struct{} // closed when the program's output ends
}

// await waits until the program's screen shows.
func (s screen) await(within time.Duration) error {
	select {
	case <-s.shown:
		return nil
	case <-s.gone:
		select {
		case <-s.shown:
			return nil
		default:
		}
		return errors.New("the program ended before it showed its screen")
	case <-time.After(within):
		return fmt.Errorf("the program did not show its screen within %s", within)
	}
}

// watch copies to out and closes seen the first time mark passes through,
// also when mark is split across two writes.
type watch struct {
	out  io.Writer
	mark []byte
	seen chan struct{}
	tail []byte
	done bool
}

func (w *watch) Write(b []byte) (int, error) {
	if !w.done {
		buf := append(append([]byte(nil), w.tail...), b...)
		if bytes.Contains(buf, w.mark) {
			close(w.seen)
			w.done, w.tail = true, nil
		} else if keep := len(w.mark) - 1; len(buf) > keep {
			w.tail = buf[len(buf)-keep:]
		} else {
			w.tail = buf
		}
	}
	return w.out.Write(b)
}

// start runs the staged program on a terminal of out's size, and copies what it
// draws to out. The returned wait ends the run: it hangs the terminal up, gives
// the program a moment to leave, and restores out's screen.
func (s *stage) start(out *os.File) (*pty, screen, func(), error) {
	rows, cols := size(out)
	p, err := openPty(rows, cols)
	if err != nil {
		return nil, screen{}, nil, fmt.Errorf("opening a terminal: %w", err)
	}
	s.cmd.Stdin, s.cmd.Stdout, s.cmd.Stderr = p.slave, p.slave, p.slave
	s.cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := s.cmd.Start(); err != nil {
		return nil, screen{}, nil, err
	}
	_ = p.slave.Close()
	scr := screen{shown: make(chan struct{}), gone: make(chan struct{})}
	copied := scr.gone
	go func() {
		_, _ = io.Copy(&watch{out: out, mark: fullScreen, seen: scr.shown}, p.master)
		close(copied)
	}()
	exited := make(chan struct{})
	go func() {
		_ = s.cmd.Wait()
		close(exited)
	}()
	end := func() {
		_ = p.master.Close()
		select {
		case <-exited:
		case <-time.After(3 * time.Second):
			_ = s.cmd.Process.Kill()
			<-exited
		}
		<-copied
		fmt.Fprint(out, "\x1b[?1049l\x1b[?25h\x1b[0m\r\n")
	}
	return p, scr, end, nil
}

// readyWithin is how long play waits for the program to take its terminal.
const readyWithin = 30 * time.Second

// awaitRaw waits until the program has put its terminal in raw mode, the sign
// that it reads keys one by one. A key sent before that is held by the
// terminal, sent on together with the next ones, and Enter arrives as Ctrl+J.
// The master side reads the terminal's mode as the program set it.
func awaitRaw(master *os.File, within time.Duration) error {
	deadline := time.Now().Add(within)
	for {
		t, err := unix.IoctlGetTermios(int(master.Fd()), unix.TCGETS)
		if err != nil {
			return fmt.Errorf("reading the terminal's mode: %w", err)
		}
		if t.Lflag&unix.ICANON == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the program did not take its terminal within %s", within)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func play(file, bin string, out *os.File) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	script, err := tour.Parse(string(b))
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	var press [][]byte
	for _, st := range script.Steps {
		kb, err := tour.KeyBytes(st.Key)
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		press = append(press, kb)
	}
	st, err := newStage(bin, script.Session)
	if err != nil {
		return err
	}
	defer st.close()
	p, scr, end, err := st.start(out)
	if err != nil {
		return err
	}
	defer end()
	if err := scr.await(readyWithin); err != nil {
		return err
	}
	if err := awaitRaw(p.master, readyWithin); err != nil {
		return err
	}
	time.Sleep(script.Start)
	for i, step := range script.Steps {
		if _, err := p.master.Write(press[i]); err != nil {
			return fmt.Errorf("the program has stopped before step %d: %w", i+1, err)
		}
		time.Sleep(step.Rest)
	}
	return nil
}
