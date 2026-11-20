package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/tour"
)

// isolate points everything the tool sets at the test's own folders, so what
// the tool sets is put back when the test ends.
func isolate(t *testing.T) (charts string) {
	t.Helper()
	root := t.TempDir()
	charts = filepath.Join(root, "own-charts")
	if err := os.MkdirAll(charts, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SEXTILE_CHARTS_DIR", charts)
	for _, k := range []string{"SEXTILE_DEV_CHARTS_DIR", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(k, "")
	}
	return charts
}

// stub is a program that answers like Sextile does to this tool: it writes its
// arguments to out, takes its terminal, shows its screen, then writes every
// byte it is sent to out.
func stub(t *testing.T) (bin, out string) {
	t.Helper()
	return slowStub(t, 0)
}

// slowStub is stub that waits before it takes its terminal, as a program does
// that starts slowly on a loaded machine.
func slowStub(t *testing.T, wait time.Duration) (bin, out string) {
	t.Helper()
	dir := t.TempDir()
	out = filepath.Join(dir, "received")
	bin = filepath.Join(dir, "sextile")
	script := fmt.Sprintf("#!/bin/sh\nprintf 'args:%%s\\n' \"$*\" > %q\nsleep %.1f\nstty raw -echo\nprintf '\\033[?1049h'\nexec cat >> %q\n", out, wait.Seconds(), out)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, out
}

func testPty(t *testing.T) *pty {
	t.Helper()
	p, err := openPty(40, 132)
	if err != nil {
		t.Skipf("no pty: %v", err)
	}
	t.Cleanup(func() { _ = p.master.Close() })
	return p
}

func tempFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestHelpDescribesTheFormat(t *testing.T) {
	out := tempFile(t)
	if code := run([]string{"--help"}, os.Stdin, out, out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	b, _ := os.ReadFile(out.Name())
	for _, want := range []string{"tour record", "tour play", "Ctrl+]", "KEY DURATION", "@session NAME", "<down>"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("--help lacks %q:\n%s", want, b)
		}
	}
}

func TestRecordRefusesAFileInsideAChartsFolder(t *testing.T) {
	charts := isolate(t)
	if err := os.MkdirAll(filepath.Join(charts, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{filepath.Join(charts, "tour.keys"), filepath.Join(charts, "sub", "tour.keys")} {
		if err := refuseCharts(file, testCharts(t)); err == nil || !strings.Contains(err.Error(), "inside a charts folder") {
			t.Errorf("a file at %s was not refused: %v", file, err)
		}
		errOut := tempFile(t)
		if code := run([]string{"record", file}, os.Stdin, tempFile(t), errOut); code != 1 {
			t.Errorf("record %s exited %d, want 1", file, code)
		}
		if _, err := os.Stat(file); err == nil {
			t.Errorf("%s was written", file)
		}
	}
	if err := refuseCharts(filepath.Join(filepath.Dir(charts), "tour.keys"), testCharts(t)); err != nil {
		t.Errorf("a file beside the charts folder was refused: %v", err)
	}
}

// The refusal holds for a charts folder named by a relative path, for any
// folder that holds a chart file, and for a name that is not a recording's;
// the tool's own folder is never made inside a charts folder, and the program
// it runs keeps its cache in that folder.
func TestRecordRefusesEveryWayIntoAChartsFolder(t *testing.T) {
	charts := isolate(t)
	t.Chdir(filepath.Dir(charts))
	t.Setenv("SEXTILE_CHARTS_DIR", filepath.Base(charts))
	if err := refuseCharts(filepath.Join(charts, "tour.keys"), testCharts(t)); err == nil {
		t.Error("a recording inside a charts folder named by a relative path was not refused")
	}
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "Client.as"), []byte("@AI800  ; Astrolog chart info.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := refuseCharts(filepath.Join(other, "tour.keys"), testCharts(t)); err == nil {
		t.Error("a recording in a folder that holds a chart file was not refused")
	}
	if err := refuseCharts(filepath.Join(t.TempDir(), "preferences.yaml"), testCharts(t)); err == nil {
		t.Error("a name that does not end in .keys was not refused")
	}
	if err := refuseCharts(filepath.Join(t.TempDir(), "mine.keys"), testCharts(t)); err != nil {
		t.Errorf("a recording in an empty folder was refused: %v", err)
	}
	t.Setenv("TMPDIR", other)
	if _, err := newStage("/bin/true", ""); err == nil {
		t.Error("the tool's own folder was made inside a folder that holds a chart file")
	}
	t.Setenv("TMPDIR", t.TempDir())
	st, err := newStage("/bin/true", "")
	if err != nil {
		t.Fatal(err)
	}
	defer st.close()
	if got := os.Getenv("XDG_CACHE_HOME"); !strings.HasPrefix(got, st.root) {
		t.Errorf("the program's cache is %q, not inside the tool's folder %s", got, st.root)
	}
}

// testCharts is the user's charts folder as the tool reads it at its start.
func testCharts(t *testing.T) string {
	t.Helper()
	c, err := userCharts()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRecordWritesEveryKeyWithItsPauseAndSaysSo(t *testing.T) {
	isolate(t)
	bin, received := stub(t)
	p := testPty(t)
	file := filepath.Join(t.TempDir(), "mine.keys")
	errOut := tempFile(t)
	done := make(chan error, 1)
	go func() { done <- record(file, bin, p.slave, tempFile(t), errOut) }()

	// Type only once record reads the terminal key by key, as a person would
	// after the screen appears; a key typed before is held and changed.
	if err := awaitRaw(p.master, 15*time.Second); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond)
	for _, k := range []struct {
		bytes string
		wait  time.Duration
	}{{"\x1b[B", 400 * time.Millisecond}, {"0", 600 * time.Millisecond}, {"u", 300 * time.Millisecond}, {"\r", 500 * time.Millisecond}, {"\x1d", 0}} {
		if _, err := p.master.Write([]byte(k.bytes)); err != nil {
			t.Fatal(err)
		}
		time.Sleep(k.wait)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("record did not end at Ctrl+]")
	}

	say, _ := os.ReadFile(errOut.Name())
	if !strings.HasPrefix(string(say), "tour: every key you type") || !strings.Contains(string(say), "written to "+file) {
		t.Errorf("the first thing record says is %q", say)
	}
	// Only the owner reads a recording: it holds every key typed.
	if fi, err := os.Stat(file); err != nil {
		t.Fatal(err)
	} else if fi.Mode().Perm() != 0o600 {
		t.Errorf("the recording's mode is %v, want -rw-------", fi.Mode().Perm())
	}
	text, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	script, err := tour.Parse(string(text))
	if err != nil {
		t.Fatalf("the recording does not parse: %v\n%s", err, text)
	}
	var keys []string
	for _, s := range script.Steps {
		keys = append(keys, tour.SpellKey(s.Key))
	}
	if got := strings.Join(keys, " "); got != "<down> 0 u <enter>" {
		t.Errorf("recorded keys %q, want %q", got, "<down> 0 u <enter>")
	}
	for i, want := range []time.Duration{400, 600, 300, 500} {
		if got := script.Steps[i].Rest; got < want*time.Millisecond-150*time.Millisecond || got > want*time.Millisecond+400*time.Millisecond {
			t.Errorf("step %d rests %s, want about %s", i+1, got, want*time.Millisecond)
		}
	}
	if script.Start < time.Second || script.Start > 2500*time.Millisecond {
		t.Errorf("the opening picture rests %s, want about 1.2s", script.Start)
	}
	got, _ := os.ReadFile(received)
	if !strings.HasPrefix(string(got), "args:\n") || !strings.Contains(string(got), "\x1b[B") {
		t.Errorf("the program was sent %q, want the keys as typed", got)
	}
	if err := record(file, bin, p.slave, tempFile(t), tempFile(t)); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Errorf("a second recording over the same file: %v", err)
	}
}

func TestPlayPressesTheKeysAtTheirPaceInTheNamedSession(t *testing.T) {
	isolate(t)
	bin, received := stub(t)
	file := filepath.Join(t.TempDir(), "p.keys")
	if err := os.WriteFile(file, []byte("@session Sextile Tour\n@start 600ms\n<down> 400ms\n0 300ms\n/ 200ms\n<enter> 100ms\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	began := time.Now()
	if err := play(file, bin, tempFile(t)); err != nil {
		t.Fatal(err)
	}
	// At least the file's 1.6s: the keys keep their pace. play also waits for
	// the program's screen first and gives it 3s to leave at the end, and a
	// loaded machine stretches both, so the ceiling only says play does not hang.
	if took := time.Since(began); took < 1600*time.Millisecond || took > readyWithin {
		t.Errorf("play took %s, want the file's 1.6s plus the program's start and end", took)
	}
	got, _ := os.ReadFile(received)
	if want := "args:--session Sextile Tour\n\x1b[B0/\r"; string(got) != want {
		t.Errorf("the program was started and sent %q, want %q", got, want)
	}
}

// A program that takes its terminal late still gets every key as it was
// written: before that, the terminal holds the keys, sends them on together and
// turns Enter into Ctrl+J. The pause of @start counts from the moment it is ready.
func TestPlayWaitsForTheProgramToTakeItsTerminal(t *testing.T) {
	isolate(t)
	bin, received := slowStub(t, 1500*time.Millisecond)
	file := filepath.Join(t.TempDir(), "late.keys")
	if err := os.WriteFile(file, []byte("@start 100ms\n<down> 100ms\n<enter> 300ms\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	began := time.Now()
	if err := play(file, bin, tempFile(t)); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(began); took < 2*time.Second {
		t.Errorf("play took %s, want the program's 1.5s start and then the file's 0.5s", took)
	}
	got, _ := os.ReadFile(received)
	if want := "args:\n\x1b[B\r"; string(got) != want {
		t.Errorf("the program was sent %q, want %q", got, want)
	}
}

// askingStub starts as Sextile does: it asks the terminal for its colours in
// raw mode and reads whatever comes in as the answer, goes back, and only then
// takes the terminal and shows its screen.
func askingStub(t *testing.T) (bin, out string) {
	t.Helper()
	dir := t.TempDir()
	out = filepath.Join(dir, "received")
	bin = filepath.Join(dir, "sextile")
	// The question waits for an answer as a terminal read does: up to 0.8s,
	// ending at the first bytes that come in.
	script := fmt.Sprintf("#!/bin/sh\nprintf 'args:%%s\\n' \"$*\" > %q\nstty raw -echo min 0 time 8\ndd bs=256 count=1 of=/dev/null 2>/dev/null\nstty -raw echo min 1 time 0\nsleep 0.5\nstty raw -echo\nprintf '\\033[?1049h'\nexec cat >> %q\n", out, out)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, out
}

// The terminal's raw mode alone is no sign that the program is ready: Sextile
// takes it once to ask for the terminal's colours, and a key sent then is read
// as the answer. The keys wait for the program's screen.
func TestPlayWaitsForTheScreenNotTheColourQuestion(t *testing.T) {
	isolate(t)
	bin, received := askingStub(t)
	file := filepath.Join(t.TempDir(), "asks.keys")
	if err := os.WriteFile(file, []byte("@start 100ms\n<down> 100ms\n<enter> 300ms\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := play(file, bin, tempFile(t)); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(received)
	if want := "args:\n\x1b[B\r"; string(got) != want {
		t.Errorf("the program was sent %q, want %q", got, want)
	}
}

func TestPlayRefusesAKeyItCannotSend(t *testing.T) {
	isolate(t)
	bin, _ := stub(t)
	file := filepath.Join(t.TempDir(), "f.keys")
	if err := os.WriteFile(file, []byte("<f1> 1s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := play(file, bin, tempFile(t)); err == nil || !strings.Contains(err.Error(), "cannot be sent") {
		t.Errorf("play of an unsendable key: %v", err)
	}
}

// The tour's own file runs on the real program: the chart opens in the session
// the file names, and the keys reach it.
func TestPlayRunsTheToursOwnFileOnTheRealProgram(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog is not on PATH")
	}
	// A machine whose zone names no city, as GitHub's runners are: the tour
	// still starts as it does everywhere.
	t.Setenv("TZ", "Etc/UTC")
	repo, _ := filepath.Abs("../..")
	bin := filepath.Join(t.TempDir(), "sextile")
	build := exec.Command("go", "build", "-o", bin, "./cmd/sextile")
	build.Dir = repo
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	isolate(t)
	b, err := os.ReadFile(filepath.Join(repo, "internal", "ui", "testdata", "tours", "sextile-tour.keys"))
	if err != nil {
		t.Fatal(err)
	}
	script, err := tour.Parse(string(b))
	if err != nil {
		t.Fatal(err)
	}
	script.Start = 4 * time.Second
	for i := range script.Steps {
		script.Steps[i].Rest = 350 * time.Millisecond
	}
	script.Steps[len(script.Steps)-1].Rest = 2 * time.Second
	file := filepath.Join(t.TempDir(), "fast.keys")
	if err := os.WriteFile(file, []byte(script.Text()), 0o644); err != nil {
		t.Fatal(err)
	}
	out := tempFile(t)
	if err := play(file, bin, out); err != nil {
		t.Fatal(err)
	}
	screen, _ := os.ReadFile(out.Name())
	for _, want := range []string{"Sextile Tour", "Sextile 0.1.0", "/ura"} {
		if !bytes.Contains(screen, []byte(want)) {
			t.Errorf("the played program never drew %q", want)
		}
	}
}

func TestSizeIsTheLanesWhenOutputIsNoTerminal(t *testing.T) {
	if r, c := size(tempFile(t)); r != 40 || c != 132 {
		t.Errorf("size = %d by %d, want 40 by 132", r, c)
	}
}

// Nothing the released program or its image is made of mentions the recorder.
func TestReleasedBinaryAndImageHoldNoRecorder(t *testing.T) {
	root, _ := filepath.Abs("../..")
	banned := []string{"tools/tour", "internal/tour", "sextile-tour", "tour record", "tour play"}
	var scanned int
	err := filepath.WalkDir(filepath.Join(root, "cmd"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		scanned++
		for _, w := range banned {
			if strings.Contains(string(b), w) {
				t.Errorf("%s mentions %q", path, w)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 5 {
		t.Fatalf("scanned only %d files of cmd/", scanned)
	}
	docker, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range banned {
		if strings.Contains(string(docker), w) {
			t.Errorf("the Dockerfile mentions %q", w)
		}
	}
	if !strings.Contains(string(docker), "cmd/sextile") {
		t.Error("the Dockerfile does not build cmd/sextile, so this scan reads the wrong file")
	}
	list := exec.Command("go", "list", "-deps", "./cmd/sextile")
	list.Dir = root
	deps, err := list.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	if !strings.Contains(string(deps), "github.com/cruiserxyz/sextile/internal/ui\n") {
		t.Fatal("go list does not show the program's own packages, so this scan proves nothing")
	}
	for _, p := range []string{"internal/tour", "tools/tour"} {
		if strings.Contains(string(deps), "sextile/"+p+"\n") {
			t.Errorf("the released program is built with %s", p)
		}
	}
}
