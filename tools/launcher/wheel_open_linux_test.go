//go:build linux

package launcher

// W1 part 2: the launcher says where the wheel will be and opens the browser
// once, the first time the wheel answers. The rig's PATH holds only the fake
// docker and a few tools, so an opener exists only when a test puts a stub on
// it; the container is "up" while the fake docker waits on a hold file.

import (
	"bytes"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// installOpener puts a stub named name on the rig's PATH. Each call appends its
// arguments, one line, to the returned log.
func installOpener(t *testing.T, r *rig, name string) string {
	t.Helper()
	log := filepath.Join(r.root, name+".calls")
	mustWrite(t, filepath.Join(r.fakebin, name), "#!/usr/bin/env bash\necho \"$@\" >> \""+log+"\"\n", 0o755)
	return log
}

func opened(t *testing.T, log string) []string {
	t.Helper()
	b, err := os.ReadFile(log)
	if err != nil {
		return nil
	}
	return strings.Fields(strings.ReplaceAll(string(b), "\n", " "))
}

// running is a launcher started in the background, its output in pipes the
// test reads only once it has exited: Wait does not return while anything the
// launcher started still holds those pipes.
type running struct {
	cmd    *exec.Cmd
	so, se bytes.Buffer
	hold   string
	done   chan struct{} // closed when the launcher has exited
	err    error
}

func (r *rig) startHeld(args ...string) *running {
	r.t.Helper()
	h := &running{hold: filepath.Join(r.root, "hold"), done: make(chan struct{})}
	r.setenv("FAKE_DOCKER_RUN_HOLD", h.hold)
	h.cmd = exec.Command(r.script(), args...)
	h.cmd.Dir = r.work
	h.cmd.Env = r.environ()
	h.cmd.Stdout, h.cmd.Stderr = &h.so, &h.se
	if err := h.cmd.Start(); err != nil {
		r.t.Fatal(err)
	}
	go func() { h.err = h.cmd.Wait(); close(h.done) }()
	r.t.Cleanup(func() {
		_ = os.WriteFile(h.hold, nil, 0o644)
		select {
		case <-h.done:
		case <-time.After(5 * time.Second):
			_ = h.cmd.Process.Kill()
		}
	})
	return h
}

// release lets the container exit and waits for the launcher to finish.
func (h *running) release(t *testing.T) time.Duration {
	t.Helper()
	start := time.Now()
	if err := os.WriteFile(h.hold, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.done:
		if h.err != nil {
			t.Fatalf("the launcher failed: %v\n%s", h.err, h.se.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the launcher did not finish after the container exited: something it started still holds its pipes")
	}
	return time.Since(start)
}

func (r *rig) waitForRun() {
	r.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if r.find("run") != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	r.t.Fatal("the launcher never reached docker run")
}

// serveWheel answers HTTP on the port, as Sextile does once the wheel is up.
func serveWheel(t *testing.T, port int) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Skipf("cannot take port %d: %v", port, err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("1")) })}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { srv.Close() })
}

func waitForCalls(t *testing.T, log string, n int) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if len(opened(t, log)) >= n {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the opener was called %d times, want %d", len(opened(t, log)), n)
}

// A3: the browser opens exactly once, and only when the wheel answers.
func TestLauncherOpensBrowserOnce(t *testing.T) {
	for _, name := range []string{"wslview", "xdg-open"} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			log := installOpener(t, r, name)
			r.setenv("SEXTILE_OPEN_POLL", "0.1")
			port := freePort(t)
			h := r.startHeld("--charts-dir", r.mkdir("c"), "--port", strconv.Itoa(port))
			r.waitForRun()

			// Docker is up and publishes the port, but nothing answers yet.
			time.Sleep(700 * time.Millisecond)
			if got := opened(t, log); len(got) != 0 {
				t.Fatalf("the browser opened before the wheel answered: %q", got)
			}

			serveWheel(t, port)
			waitForCalls(t, log, 1)
			// It is polled for a while after; it must not open again.
			time.Sleep(1200 * time.Millisecond)
			want := "http://localhost:" + strconv.Itoa(port) + "/"
			if got := opened(t, log); len(got) != 1 || got[0] != want {
				t.Fatalf("opener calls = %q, want exactly [%s]", got, want)
			}
			h.release(t)
		})
	}

	t.Run("wslview wins over xdg-open", func(t *testing.T) {
		r := newRig(t)
		wsl := installOpener(t, r, "wslview")
		xdg := installOpener(t, r, "xdg-open")
		r.setenv("SEXTILE_OPEN_POLL", "0.1")
		port := freePort(t)
		h := r.startHeld("--charts-dir", r.mkdir("c"), "--port", strconv.Itoa(port))
		r.waitForRun()
		serveWheel(t, port)
		waitForCalls(t, wsl, 1)
		time.Sleep(300 * time.Millisecond)
		if got := opened(t, xdg); len(got) != 0 {
			t.Errorf("xdg-open was called as well: %q", got)
		}
		h.release(t)
	})
}

// A Docker that has published the port but has nothing behind it accepts and
// drops the connection. That is not the wheel answering.
func TestLauncherIgnoresABareConnect(t *testing.T) {
	r := newRig(t)
	log := installOpener(t, r, "xdg-open")
	r.setenv("SEXTILE_OPEN_POLL", "0.1")
	port := freePort(t)
	h := r.startHeld("--charts-dir", r.mkdir("c"), "--port", strconv.Itoa(port))
	r.waitForRun()

	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Skipf("cannot take port %d: %v", port, err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	time.Sleep(1500 * time.Millisecond)
	if got := opened(t, log); len(got) != 0 {
		t.Fatalf("a bare connect opened the browser: %q", got)
	}
	h.release(t)
}

// --no-open: nothing waits for the wheel, however available an opener is.
func TestLauncherNoOpenStartsNoWaiter(t *testing.T) {
	r := newRig(t)
	log := installOpener(t, r, "xdg-open")
	r.setenv("SEXTILE_OPEN_POLL", "0.1")
	port := freePort(t)
	h := r.startHeld("--charts-dir", r.mkdir("c"), "--port", strconv.Itoa(port), "--no-open")
	r.waitForRun()
	serveWheel(t, port)
	time.Sleep(1500 * time.Millisecond)
	if got := opened(t, log); len(got) != 0 {
		t.Fatalf("--no-open and the browser opened: %q", got)
	}
	if run := r.find("run"); indexOf(run, "--no-open") >= 0 {
		t.Errorf("--no-open reached the app: %q", run)
	}
	h.release(t)
}

// The waiter lives as long as the container: when Docker exits it ends, and a
// wheel that answers afterwards is not opened. It also leaves the launcher's
// output pipes alone, or the caller reading them would wait on it.
func TestLauncherWaiterEndsWithTheContainer(t *testing.T) {
	t.Run("a later answer opens nothing", func(t *testing.T) {
		r := newRig(t)
		log := installOpener(t, r, "xdg-open")
		r.setenv("SEXTILE_OPEN_POLL", "0.1")
		port := freePort(t)
		res := r.run("--charts-dir", r.mkdir("c"), "--port", strconv.Itoa(port))
		r.mustSucceed(res)
		time.Sleep(400 * time.Millisecond)
		serveWheel(t, port)
		time.Sleep(1200 * time.Millisecond)
		if got := opened(t, log); len(got) != 0 {
			t.Fatalf("the waiter outlived the container and opened %q", got)
		}
	})

	t.Run("it does not hold the output pipes", func(t *testing.T) {
		r := newRig(t)
		installOpener(t, r, "xdg-open")
		r.setenv("SEXTILE_OPEN_POLL", "30")
		start := time.Now()
		res := r.run("--charts-dir", r.mkdir("c"), "--port", strconv.Itoa(freePort(t)))
		r.mustSucceed(res)
		if took := time.Since(start); took > 10*time.Second {
			t.Fatalf("the launcher's output stayed open for %s after docker exited", took)
		}
	})
}

// The launcher says where the wheel will be, on a terminal before Docker starts
// and in doctor; with no opener it says to open it by hand.
func TestLauncherSaysWhereTheWheelIs(t *testing.T) {
	port := strconv.Itoa(freePort(t))
	url := "http://localhost:" + port

	t.Run("launch on a terminal", func(t *testing.T) {
		r := newRig(t)
		installOpener(t, r, "xdg-open")
		r.setenv("SEXTILE_OPEN_POLL", "30")
		s := r.startPTY(allTTY, "--charts-dir", r.mkdir("c"), "--port", port)
		out, code := s.finish()
		if code != 0 {
			t.Fatalf("exit %d:\n%s", code, out)
		}
		want := "Wheel: " + url + "; your browser opens on the first Ctrl+X"
		i, j := strings.Index(out, want), strings.Index(out, "starting Sextile")
		if i < 0 || j < 0 || i < j {
			t.Fatalf("want %q after the start line:\n%s", want, out)
		}
	})

	t.Run("no opener", func(t *testing.T) {
		r := newRig(t)
		s := r.startPTY(allTTY, "--charts-dir", r.mkdir("c"), "--port", port)
		out, _ := s.finish()
		if !strings.Contains(out, "Wheel: "+url+"; open it yourself after the first Ctrl+X") || strings.Contains(out, "your browser opens") {
			t.Fatalf("with no opener the line must not promise one:\n%s", out)
		}
	})

	t.Run("--no-open", func(t *testing.T) {
		r := newRig(t)
		installOpener(t, r, "xdg-open")
		s := r.startPTY(allTTY, "--charts-dir", r.mkdir("c"), "--port", port, "--no-open")
		out, _ := s.finish()
		if !strings.Contains(out, "Wheel: "+url+"; open it yourself after the first Ctrl+X") || strings.Contains(out, "your browser opens") {
			t.Fatalf("with --no-open the line must not promise the browser:\n%s", out)
		}
	})

	t.Run("doctor", func(t *testing.T) {
		r := newRig(t)
		installOpener(t, r, "wslview")
		res := r.run("doctor", "--charts-dir", r.mkdir("c"), "--port", port)
		if !strings.Contains(res.stdout, "wheel: "+url+"; your browser opens on the first Ctrl+X") {
			t.Fatalf("doctor does not say where the wheel will be:\n%s", res.stdout)
		}
		r = newRig(t)
		res = r.run("doctor", "--charts-dir", r.mkdir("c"), "--port", port)
		if !strings.Contains(res.stdout, "wheel: "+url+";") || !strings.Contains(res.stdout, "no wslview or xdg-open") {
			t.Fatalf("doctor with no opener:\n%s", res.stdout)
		}
		r.mustExit2(r.run("doctor", "--no-open"))
	})
}
