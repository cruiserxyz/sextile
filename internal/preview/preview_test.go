package preview

import (
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

func resetPreview(t *testing.T) *[]string {
	t.Helper()
	_ = Cleanup()
	t.Setenv(previewModeEnv, modeHTTP)

	previewMu.Lock()
	previewImage = nil
	previewVersion = 0
	previewServer = nil
	previewURL = ""
	previewEnd = false
	browserOpened = false
	legacyDir = ""
	legacyPath = ""
	legacyOpened = false
	legacyProcess = nil
	legacyDone = nil
	viewerGroups = nil
	previewMu.Unlock()

	origListen, origLaunch := listenPreview, launchBrowser
	listenPreview = func() (net.Listener, error) {
		return net.Listen("tcp", "127.0.0.1:0")
	}
	opened := []string{}
	launchBrowser = func(url string) error {
		opened = append(opened, url)
		return nil
	}
	t.Cleanup(func() {
		_ = Cleanup()
		listenPreview, launchBrowser = origListen, origLaunch
	})
	return &opened
}

func get(t *testing.T, url string) *http.Response {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return response
}

func body(t *testing.T, response *http.Response) []byte {
	t.Helper()
	defer response.Body.Close()
	b, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return b
}

func TestShowServesAndRefreshesOneBrowserPage(t *testing.T) {
	opened := resetPreview(t)
	firstBytes := []byte(`<svg id="first"/>`)
	url, err := Show(firstBytes)
	if err != nil {
		t.Fatalf("first Show: %v", err)
	}
	if !strings.HasPrefix(url, "http://localhost:") {
		t.Fatalf("URL = %q, want Windows-host localhost", url)
	}
	if !reflect.DeepEqual(*opened, []string{url}) {
		t.Fatalf("browser launches = %v, want [%s]", *opened, url)
	}

	page := get(t, url)
	if page.StatusCode != http.StatusOK || !strings.Contains(string(body(t, page)), "fetch('/version'") {
		t.Fatal("root page does not carry the automatic refresh loop")
	}
	chart := get(t, url+"chart.svg")
	if got := chart.Header.Get("Content-Type"); got != "image/svg+xml; charset=utf-8" {
		t.Fatalf("chart Content-Type = %q", got)
	}
	if got := string(body(t, chart)); got != `<svg id="first"/>` {
		t.Fatalf("first chart = %q", got)
	}
	if got := string(body(t, get(t, url+"version"))); got != "1" {
		t.Fatalf("first version = %q, want 1", got)
	}

	// Show owns a copy: Astrolog/UI may reuse its result buffer after publish.
	firstBytes[0] = 'X'
	if got := string(body(t, get(t, url+"chart.svg"))); got != `<svg id="first"/>` {
		t.Fatalf("caller mutation changed served chart to %q", got)
	}

	secondURL, err := Show([]byte(`<svg id="second"/>`))
	if err != nil {
		t.Fatalf("second Show: %v", err)
	}
	if secondURL != url {
		t.Fatalf("URL moved %q -> %q", url, secondURL)
	}
	if len(*opened) != 1 {
		t.Fatalf("refresh reopened the browser %d times", len(*opened))
	}
	if got := string(body(t, get(t, url+"chart.svg"))); got != `<svg id="second"/>` {
		t.Fatalf("refreshed chart = %q", got)
	}
	if got := string(body(t, get(t, url+"version"))); got != "2" {
		t.Fatalf("second version = %q, want 2", got)
	}
}

func TestListenLocalFallsBackWhenPreferredPortIsBusy(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()

	listener, err := listenLocal(busy.Addr().String())
	if err != nil {
		t.Fatalf("listenLocal: %v", err)
	}
	defer listener.Close()
	if listener.Addr().String() == busy.Addr().String() {
		t.Fatalf("fallback reused occupied address %s", busy.Addr())
	}
	if host, _, _ := net.SplitHostPort(listener.Addr().String()); host != "127.0.0.1" {
		t.Fatalf("fallback host = %q, want loopback", host)
	}
}

func TestWindowsBrowserCommandUsesCmdStart(t *testing.T) {
	dir := t.TempDir()
	cmdPath := filepath.Join(dir, "cmd.exe")
	if err := os.WriteFile(cmdPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	cmd, err := windowsBrowserCommand("http://localhost:5307/")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{cmdPath, "/d", "/c", "start", "", "http://localhost:5307/"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("browser argv = %#v, want %#v", cmd.Args, want)
	}
}

func TestSelectedModeDefaultsToTerminalXDG(t *testing.T) {
	t.Setenv(previewModeEnv, "")
	got, err := selectedMode()
	if err != nil || got != modeTerminalXD {
		t.Fatalf("selectedMode() = (%q, %v), want (%q, nil)", got, err, modeTerminalXD)
	}
}

func TestSelectedModeAcceptsAllBackends(t *testing.T) {
	for _, want := range []string{modeXD, modeTerminalXD, modeHTTP} {
		t.Run(want, func(t *testing.T) {
			t.Setenv(previewModeEnv, want)
			got, err := selectedMode()
			if err != nil || got != want {
				t.Fatalf("selectedMode() = (%q, %v), want (%q, nil)", got, err, want)
			}
		})
	}
}

func TestSelectedModeRejectsUnknownBackend(t *testing.T) {
	t.Setenv(previewModeEnv, "wat")
	if _, err := selectedMode(); err == nil {
		t.Fatal("selectedMode() accepted an unknown backend")
	}
}

func TestXDGBrowserCommands(t *testing.T) {
	path := "/tmp/sextile-preview/chart.svg"
	cmd := xdgBrowserCommand(path)
	if got, want := cmd.Args, []string{"xdg-open", path}; !reflect.DeepEqual(got, want) {
		t.Fatalf("direct xdg argv = %#v, want %#v", got, want)
	}

	t.Setenv("TERMINAL", "st")
	cmd, err := terminalXDGBrowserCommand(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cmd.Args, []string{"st", "-e", "xdg-open", path}; !reflect.DeepEqual(got, want) {
		t.Fatalf("terminal xdg argv = %#v, want %#v", got, want)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
	if !cmd.SysProcAttr.Setpgid || cmd.SysProcAttr.Pdeathsig != syscall.SIGTERM {
		t.Fatal("terminal xdg command is not configured for owned process-group cleanup")
	}
}

func TestShowFileBackedModesWriteAndReusePrivateImage(t *testing.T) {
	originalDirect, originalTerminal := launchXDGBrowser, launchTerminalXDGBrowser
	t.Cleanup(func() {
		launchXDGBrowser, launchTerminalXDGBrowser = originalDirect, originalTerminal
		_ = Cleanup()
	})

	for _, mode := range []string{modeXD, modeTerminalXD} {
		t.Run(mode, func(t *testing.T) {
			_ = Cleanup()
			previewMu.Lock()
			previewEnd = false
			previewMu.Unlock()
			t.Setenv(previewModeEnv, mode)

			opened := 0
			open := func(path string) error {
				opened++
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("opened path %q: %v", path, err)
				}
				return nil
			}
			launchXDGBrowser = open
			launchTerminalXDGBrowser = open

			path, err := Show([]byte("first"))
			if err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != "first" {
				t.Fatalf("first image = (%q, %v), want first", got, err)
			}
			if _, err := Show([]byte("second")); err != nil {
				t.Fatal(err)
			}
			if opened != 1 {
				t.Fatalf("open calls = %d, want 1", opened)
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != "second" {
				t.Fatalf("refreshed image = (%q, %v), want second", got, err)
			}
			if err := Cleanup(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("image after Cleanup: %v, want not exist", err)
			}
			previewMu.Lock()
			previewEnd = false
			previewMu.Unlock()
		})
	}
}

func TestCleanupStopsOwnedTerminalProcessGroup(t *testing.T) {
	_ = Cleanup()
	previewMu.Lock()
	previewEnd = false
	previewMu.Unlock()
	t.Setenv(previewModeEnv, modeTerminalXD)

	terminal := filepath.Join(t.TempDir(), "fake-terminal")
	if err := os.WriteFile(terminal, []byte("#!/bin/sh\nsleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TERMINAL", terminal)

	if _, err := Show([]byte("owned")); err != nil {
		t.Fatal(err)
	}
	previewMu.Lock()
	process := legacyProcess
	previewMu.Unlock()
	if process == nil || process.Process == nil {
		t.Fatal("terminal process was not retained")
	}
	pid := process.Process.Pid
	if err := Cleanup(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(-pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("owned terminal process group %d survived Cleanup", pid)
}

func TestBrowserLaunchFailureReturnsTheLiveURLAndCanRetry(t *testing.T) {
	resetPreview(t)
	attempts := 0
	launchBrowser = func(string) error {
		attempts++
		if attempts == 1 {
			return os.ErrNotExist
		}
		return nil
	}
	url, err := Show([]byte(`<svg id="first"/>`))
	if err == nil || url == "" {
		t.Fatalf("first Show = (%q, %v), want a live URL with launch error", url, err)
	}
	if got := string(body(t, get(t, url+"chart.svg"))); got != `<svg id="first"/>` {
		t.Fatalf("server after launch failure serves %q", got)
	}
	retryURL, err := Show([]byte(`<svg id="second"/>`))
	if err != nil || retryURL != url || attempts != 2 {
		t.Fatalf("retry = (%q, %v), attempts %d; want (%q, nil), 2", retryURL, err, attempts, url)
	}
}

func TestHandlerRefusesNonReadMethodsAndUnknownPaths(t *testing.T) {
	resetPreview(t)
	url, err := Show([]byte(`<svg/>`))
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodPost, url+"chart.svg", strings.NewReader("replacement"))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", response.StatusCode)
	}
	response.Body.Close()
	response = get(t, url+"anything")
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("unknown path status = %d, want 404", response.StatusCode)
	}
	response.Body.Close()
}

func TestPreviewPageRefreshContractIsPresent(t *testing.T) {
	if !strings.Contains(previewPage, "fetch('/version'") ||
		!strings.Contains(previewPage, "chart.src='/chart.{{ext}}?v='") {
		t.Fatal("preview page no longer polls the version and replaces the chart URL")
	}
	for _, rule := range []string{"overflow:hidden", "width:100vw", "height:100vh", "object-fit:contain"} {
		if !strings.Contains(previewPage, rule) {
			t.Errorf("preview page lacks responsive viewport rule %q", rule)
		}
	}
	if preferredAddress != "127.0.0.1:5307" {
		t.Fatalf("preferred address = %q", preferredAddress)
	}
}

// fakeTerminalViewer installs a $TERMINAL that records each launch in a file
// and exits after life seconds, standing in for a viewer the operator closes.
func fakeTerminalViewer(t *testing.T, life string) (launches func() int) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "launches")
	terminal := filepath.Join(dir, "fake-terminal")
	script := "#!/bin/sh\necho launched >> '" + log + "'\nsleep " + life + "\n"
	if err := os.WriteFile(terminal, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TERMINAL", terminal)
	return func() int {
		b, _ := os.ReadFile(log)
		return strings.Count(string(b), "launched")
	}
}

// waitForViewerExit waits until the Wait goroutine has recorded the exit.
func waitForViewerExit(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		previewMu.Lock()
		closed := viewerClosed
		previewMu.Unlock()
		if closed {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the viewer's exit was never recorded")
}

func waitForLaunches(t *testing.T, launches func() int, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if launches() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("launches = %d, want %d", launches(), want)
}

func freshTerminalPreview(t *testing.T) {
	t.Helper()
	_ = Cleanup()
	previewMu.Lock()
	previewEnd = false
	previewMu.Unlock()
	t.Setenv(previewModeEnv, modeTerminalXD)
	t.Cleanup(func() { _ = Cleanup() })
}

// TestClosedTerminalViewerOpensAgain is the operator's bug of 2026-10-05:
// "when the wheel terminal instance is closed, it cannot be reopened". Once
// the viewer process has exited, an open request (Ctrl+X) launches a new one
// on the same image file.
func TestClosedTerminalViewerOpensAgain(t *testing.T) {
	freshTerminalPreview(t)
	launches := fakeTerminalViewer(t, "0.2")

	RequestOpen()
	path, err := Show([]byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	waitForLaunches(t, launches, 1)
	waitForViewerExit(t)

	RequestOpen()
	again, err := Show([]byte("second"))
	if err != nil {
		t.Fatal(err)
	}
	waitForLaunches(t, launches, 2)
	if again != path {
		t.Fatalf("reopened on %q, want the same image file %q", again, path)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "second" {
		t.Fatalf("reopened image = (%q, %v), want second", got, err)
	}
}

// TestFollowNeverReopensAClosedViewer: a publish nobody asked to open (the
// follow while the operator steps time) rewrites the file of an open viewer
// without a second launch, and after the viewer is closed opens nothing and
// says so.
func TestFollowNeverReopensAClosedViewer(t *testing.T) {
	freshTerminalPreview(t)
	launches := fakeTerminalViewer(t, "0.5")

	RequestOpen()
	path, err := Show([]byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	waitForLaunches(t, launches, 1)
	if _, err := Show([]byte("second")); err != nil {
		t.Fatalf("follow of an open viewer: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "second" {
		t.Fatalf("followed image = %q, want second", got)
	}
	waitForViewerExit(t)

	if _, err := Show([]byte("third")); !errors.Is(err, ErrViewerClosed) {
		t.Fatalf("follow after close: err = %v, want ErrViewerClosed", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "second" {
		t.Fatalf("image after a follow of a closed viewer = %q, want it untouched (second)", got)
	}
	if n := launches(); n != 1 {
		t.Fatalf("launches = %d, want 1: a follow must never open a viewer", n)
	}
}

// TestCtrlXInTheExitWindowOpensAgain pins the window between the viewer's
// exit and the Wait goroutine taking the lock (the reviewer, hotfix round 1): the
// test holds the lock across the exit, so the goroutine cannot record it, and
// an open request in that window must still launch a new viewer.
func TestCtrlXInTheExitWindowOpensAgain(t *testing.T) {
	freshTerminalPreview(t)
	launches := fakeTerminalViewer(t, "0.2")

	RequestOpen()
	if _, err := Show([]byte("first")); err != nil {
		t.Fatal(err)
	}
	waitForLaunches(t, launches, 1)

	previewMu.Lock()
	done := legacyDone
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		previewMu.Unlock()
		t.Fatal("the viewer never exited")
	}
	requestOpenLocked()
	_, err := showLocked([]byte("second"))
	previewMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	waitForLaunches(t, launches, 2)
}

// TestCleanupStopsAClosedViewersGroup: a terminal that exits may leave a
// child in its process group (the reviewer, hotfix round 1). Quit stops it although
// the terminal itself is long gone.
func TestCleanupStopsAClosedViewersGroup(t *testing.T) {
	freshTerminalPreview(t)
	dir := t.TempDir()
	childFile := filepath.Join(dir, "child")
	terminal := filepath.Join(dir, "fake-terminal")
	script := "#!/bin/sh\nsleep 60 &\necho $! > '" + childFile + "'\n"
	if err := os.WriteFile(terminal, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TERMINAL", terminal)

	RequestOpen()
	if _, err := Show([]byte("owned")); err != nil {
		t.Fatal(err)
	}
	waitForViewerExit(t)
	raw, err := os.ReadFile(childFile)
	if err != nil {
		t.Fatal(err)
	}
	child := strings.TrimSpace(string(raw))
	if !processRunning(child) {
		t.Fatalf("setup: the viewer's child %s is not running", child)
	}
	if err := Cleanup(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !processRunning(child) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the closed viewer's child %s survived Cleanup", child)
}

// processRunning reports whether pid is alive and not a zombie.
func processRunning(pid string) bool {
	stat, err := os.ReadFile("/proc/" + pid + "/stat")
	if err != nil {
		return false
	}
	fields := strings.Fields(string(stat[strings.LastIndexByte(string(stat), ')')+1:]))
	return len(fields) > 0 && fields[0] != "Z"
}

// TestCtrlXCannotBeUsedUpByAnOlderPublish (the reviewer, hotfix round 2): after a
// close, a Ctrl+X un-closes the viewer for every publish that follows it, so
// an older follow publish landing first opens the viewer and the press's own
// publish still succeeds; nothing is consumed.
func TestCtrlXCannotBeUsedUpByAnOlderPublish(t *testing.T) {
	freshTerminalPreview(t)
	launches := fakeTerminalViewer(t, "0.3")

	RequestOpen()
	if _, err := Show([]byte("first")); err != nil {
		t.Fatal(err)
	}
	waitForLaunches(t, launches, 1)
	waitForViewerExit(t)

	RequestOpen()
	if _, err := Show([]byte("older follow")); err != nil {
		t.Fatalf("older publish after Ctrl+X: %v", err)
	}
	if _, err := Show([]byte("the press's own")); err != nil {
		t.Fatalf("the Ctrl+X publish after an older one: %v, want it to land", err)
	}
	waitForLaunches(t, launches, 2)
}

// TestEmptyViewerGroupIsForgotten (the reviewer, hotfix round 2): a closed viewer
// that left nothing in its process group is dropped, so quit can never signal
// a reused group id.
func TestEmptyViewerGroupIsForgotten(t *testing.T) {
	freshTerminalPreview(t)
	launches := fakeTerminalViewer(t, "0")

	RequestOpen()
	if _, err := Show([]byte("first")); err != nil {
		t.Fatal(err)
	}
	waitForLaunches(t, launches, 1)
	waitForViewerExit(t)

	previewMu.Lock()
	groups := append([]int(nil), viewerGroups...)
	previewMu.Unlock()
	if len(groups) != 0 {
		t.Fatalf("viewerGroups = %v after an empty group's terminal exited, want none", groups)
	}
}

// A closed wheel answers nobody. The session lives on: a publish that was
// already on its way does not start the server again, and the next
// RequestOpen brings it back at the same address.
func TestCloseStopsServingAndReopensAtTheSameAddress(t *testing.T) {
	resetPreview(t)
	realListen()
	want := freeAddr(t)
	t.Setenv(previewAddrEnv, want)

	url, err := Show([]byte(`<svg id="client-chart"/>`))
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if err := Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if response, err := http.Get(url + "chart.svg"); err == nil {
		response.Body.Close()
		t.Fatalf("the wheel still answers after Close: %s", response.Status)
	}
	previewMu.RLock()
	kept := previewImage != nil || previewServer != nil
	previewMu.RUnlock()
	if kept {
		t.Fatal("Close kept chart bytes or a server")
	}

	// A publish already on its way: closed, not restarted.
	if got, err := Show([]byte(`<svg id="late"/>`)); !errors.Is(err, ErrViewerClosed) || got != "" {
		t.Fatalf("Show after Close = (%q, %v), want (\"\", ErrViewerClosed)", got, err)
	}
	if response, err := http.Get(url + "chart.svg"); err == nil {
		response.Body.Close()
		t.Fatal("a late Show started the server again")
	}

	RequestOpen()
	again, err := Show([]byte(`<svg id="reopened"/>`))
	if err != nil {
		t.Fatalf("Show after RequestOpen: %v", err)
	}
	if again != url {
		t.Fatalf("reopened at %q, want the same address %q", again, url)
	}
	if got := string(body(t, get(t, again+"chart.svg"))); got != `<svg id="reopened"/>` {
		t.Fatalf("reopened image = %q", got)
	}
}

func TestCloseWithNothingRunningIsHarmless(t *testing.T) {
	resetPreview(t)
	if err := Close(); err != nil {
		t.Fatalf("Close before any Show: %v", err)
	}
	// Nothing was open, so nothing is closed: the first Show still serves.
	if _, err := Show([]byte(`<svg/>`)); err != nil {
		t.Fatalf("Show after a no-op Close: %v", err)
	}
}

// wheel_format: the server hosts the format the renderer was asked for, under
// its own name and Content-Type, and the page asks for that name. Absent or
// svg is the SVG the server has always hosted.
func TestWheelFormatPreference(t *testing.T) {
	png := append(append([]byte(nil), pngSignature...), []byte("IHDR-and-the-rest")...)
	svg := []byte(`<svg id="wheel"/>`)

	for _, tc := range []struct {
		name        string
		img         []byte
		path, other string
		contentType string
		pageAsksFor string
		pageMustNot string
	}{
		{"png", png, "/chart.png", "/chart.svg", "image/png", "/chart.png?v=", "/chart.svg"},
		{"svg", svg, "/chart.svg", "/chart.png", "image/svg+xml; charset=utf-8", "/chart.svg?v=", "/chart.png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetPreview(t)
			url, err := Show(tc.img)
			if err != nil {
				t.Fatalf("Show: %v", err)
			}
			base := strings.TrimSuffix(url, "/")

			resp := get(t, base+tc.path)
			if got := resp.Header.Get("Content-Type"); got != tc.contentType {
				t.Errorf("%s Content-Type = %q, want %q", tc.path, got, tc.contentType)
			}
			if got := body(t, resp); string(got) != string(tc.img) {
				t.Errorf("%s served %d bytes that are not the published image", tc.path, len(got))
			}
			other, err := http.Get(base + tc.other)
			if err != nil {
				t.Fatal(err)
			}
			other.Body.Close()
			if other.StatusCode != http.StatusNotFound {
				t.Errorf("%s answers %d, want 404: only the hosted format is there", tc.other, other.StatusCode)
			}
			page := string(body(t, get(t, url)))
			if !strings.Contains(page, tc.pageAsksFor) || strings.Contains(page, tc.pageMustNot) {
				t.Errorf("the page does not ask for %q alone:\n%s", tc.pageAsksFor, page)
			}
		})
	}

	// A later publish in the other format moves the hosting with it.
	t.Run("follows the bytes", func(t *testing.T) {
		resetPreview(t)
		url, _ := Show(svg)
		if _, err := Show(png); err != nil {
			t.Fatal(err)
		}
		base := strings.TrimSuffix(url, "/")
		if got := get(t, base+"/chart.png").Header.Get("Content-Type"); got != "image/png" {
			t.Errorf("after a PNG publish /chart.png is %q", got)
		}
	})
}
