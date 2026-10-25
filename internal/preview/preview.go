// Package preview displays a chart image through one of three configured
// preview backends. It is a side-effecting helper that knows
// nothing about astrology: callers hand it rendered SVG bytes, the ui triggers
// it via a tea.Cmd, and astrolog remains the image producer.
//
// The xdg backends write a private stable file and launch xdg-open directly or
// inside $TERMINAL. The http backend starts the WSL browser server, keeps the
// current image in memory, and opens it through cmd.exe.
package preview

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"

	_ "github.com/cruiserxyz/sextile/internal/env"
)

// preferredAddress is a variable only so a test can point it at a free port;
// production never reassigns it.
var preferredAddress = "127.0.0.1:5307"

const (
	modeXD         = "xdg"
	modeTerminalXD = "terminal-xdg"
	modeHTTP       = "http"
	// modeServe is the http preview for a machine with no browser to launch
	// (the Docker image): it serves the wheel and never starts a browser.
	modeServe      = "serve"
	previewModeEnv = "SEXTILE_PREVIEW_MODE"
	// previewAddrEnv, when set, is the exact address the preview listens on.
	previewAddrEnv = "SEXTILE_PREVIEW_ADDR"
	chartImageName = "sextile-chart.png"
)

var (
	previewMu    sync.RWMutex
	previewImage []byte
	// previewPNG says previewImage is a PNG, which the server then hosts as
	// /chart.png; any other image is the SVG it hosts as /chart.svg. It follows
	// the bytes, not a setting read here: the bytes are what the renderer was
	// asked for (wheel_format), and what is served can never disagree with
	// them.
	previewPNG     bool
	previewVersion uint64
	previewServer  *http.Server
	previewURL     string
	previewEnd     bool
	browserOpened  bool
	legacyDir      string
	legacyPath     string
	legacyOpened   bool
	legacyProcess  *exec.Cmd
	viewerClosed   bool
	// legacyDone is closed the moment the owned viewer's Wait returns, so Show
	// sees an exit at once rather than when the Wait goroutine gets the lock.
	legacyDone chan struct{}
	// viewerGroups holds the process group of the live terminal viewer and of
	// every closed one that left a member behind, so quit stops them all. A
	// group with no member left is dropped when its terminal exits: its id may
	// then be reused, and quit must never signal someone else's processes.
	viewerGroups []int
)

// pngSignature opens every PNG file.
var pngSignature = []byte("\x89PNG\r\n\x1a\n")

// ErrSessionEnded is returned by Show after Cleanup. Once Sextile has quit, a
// late wheel publication must not bring the server or the client chart back.
var ErrSessionEnded = errors.New("preview: the session has ended; no image is served")

// ErrViewerClosed is returned by Show once the operator has closed the
// terminal viewer and has not asked (RequestOpen) to open it again: there is nothing left
// to refresh, and the caller stops following until RequestOpen.
var ErrViewerClosed = errors.New("preview: the wheel viewer was closed")

// listenPreview and launchBrowser are variables so tests can exercise Show
// without claiming a fixed machine port or opening a real browser.
var listenPreview = listenForPreview

// listenForPreview listens on exactly SEXTILE_PREVIEW_ADDR when it is set, and
// fails if that cannot be bound; unset, it keeps the loopback default and its
// fallback to a free loopback port.
func listenForPreview() (net.Listener, error) {
	if addr := os.Getenv(previewAddrEnv); addr != "" {
		return net.Listen("tcp", addr)
	}
	return listenLocal(preferredAddress)
}

var launchBrowser = func(url string) error {
	cmd, err := windowsBrowserCommand(url)
	if err != nil {
		return err
	}
	return cmd.Run()
}

var launchXDGBrowser = func(path string) error {
	return xdgBrowserCommand(path).Start()
}

var launchTerminalXDGBrowser = func(path string) error {
	cmd, err := terminalXDGBrowserCommand(path)
	if err != nil {
		return err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	legacyProcess = cmd
	legacyDone = done
	viewerGroups = append(viewerGroups, cmd.Process.Pid)
	go func() {
		_ = cmd.Wait()
		close(done)
		viewerExited(cmd)
	}()
	return nil
}

// viewerExited records that the terminal viewer this package launched has
// exited: the operator closed it (operator, 2026-10-05: "when the wheel
// terminal instance is closed, it cannot be reopened"). The process is the
// viewer's handle; its exit is the fact, not a guess. The next Show launches a
// new one when asked (RequestOpen), and a follow refresh gets ErrViewerClosed. It runs on the Wait goroutine and
// changes nothing for a process this package no longer owns (replaced, or
// cleaned up at quit).
func viewerExited(cmd *exec.Cmd) {
	previewMu.Lock()
	defer previewMu.Unlock()
	dropEmptyGroupLocked(cmd.Process.Pid)
	if legacyProcess != cmd {
		return
	}
	noteViewerExitLocked()
}

// dropEmptyGroupLocked forgets group when no process is left in it. While a
// member lives the id cannot be reused, so keeping it is safe.
func dropEmptyGroupLocked(group int) {
	if err := syscall.Kill(-group, 0); !errors.Is(err, syscall.ESRCH) {
		return
	}
	kept := viewerGroups[:0]
	for _, g := range viewerGroups {
		if g != group {
			kept = append(kept, g)
		}
	}
	viewerGroups = kept
}

// noteViewerExitLocked applies a recorded exit of the owned viewer, if its
// Wait has returned. RequestOpen and Show call it first, so a Ctrl+X that
// lands between the exit and the Wait goroutine taking the lock still opens a
// new viewer.
func noteViewerExitLocked() {
	if legacyProcess == nil || legacyDone == nil {
		return
	}
	select {
	case <-legacyDone:
	default:
		return
	}
	legacyProcess = nil
	legacyDone = nil
	legacyOpened = false
	viewerClosed = true
}

func xdgBrowserCommand(path string) *exec.Cmd {
	return exec.Command("xdg-open", path)
}

func terminalXDGBrowserCommand(path string) (*exec.Cmd, error) {
	term := os.Getenv("TERMINAL")
	if term == "" {
		return nil, fmt.Errorf("TERMINAL is not set (needed to host xdg-open)")
	}
	return exec.Command(term, "-e", "xdg-open", path), nil
}

func selectedMode() (string, error) {
	mode := os.Getenv(previewModeEnv)
	if mode == "" {
		return modeTerminalXD, nil
	}
	switch mode {
	case modeXD, modeTerminalXD, modeHTTP, modeServe:
		return mode, nil
	default:
		return "", fmt.Errorf("preview: %s=%q; want xdg, terminal-xdg, http, or serve", previewModeEnv, mode)
	}
}

// RequestOpen is the operator asking to see the wheel (Ctrl+X). A viewer they
// closed is un-closed at once, so the next Show opens a new one; a live viewer
// is left as it is. A refresh that follows the wheel never asks, so a closed
// viewer stays closed until the operator opens it.
func RequestOpen() {
	previewMu.Lock()
	defer previewMu.Unlock()
	requestOpenLocked()
}

func requestOpenLocked() {
	noteViewerExitLocked()
	viewerClosed = false
}

// Show publishes img at the session's stable browser URL. The first call
// starts the server and opens the Windows default browser; later calls update
// the in-memory image and return the same URL. The caller keeps ownership of
// img, so the bytes are copied before Show returns.
func Show(img []byte) (string, error) {
	previewMu.Lock()
	defer previewMu.Unlock()
	return showLocked(img)
}

func showLocked(img []byte) (string, error) {
	if previewEnd {
		return "", ErrSessionEnded
	}
	noteViewerExitLocked()
	if viewerClosed {
		return "", ErrViewerClosed
	}
	mode, err := selectedMode()
	if err != nil {
		return "", err
	}
	if mode != modeHTTP && mode != modeServe {
		return showXDGLocked(img, mode)
	}
	return showHTTPLocked(img, mode == modeServe)
}

func showHTTPLocked(img []byte, serveOnly bool) (string, error) {
	previewImage = append(previewImage[:0], img...)
	previewPNG = bytes.HasPrefix(img, pngSignature)
	previewVersion++

	if previewServer == nil {
		listener, err := listenPreview()
		if err != nil {
			return "", fmt.Errorf("preview: listen: %w", err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		previewURL = "http://localhost:" + strconv.Itoa(port) + "/"
		previewServer = &http.Server{Handler: http.HandlerFunc(servePreview)}
		server := previewServer
		go func() {
			_ = server.Serve(listener)
		}()
	}

	if serveOnly {
		return previewURL, nil
	}
	if !browserOpened {
		if err := launchBrowser(previewURL); err != nil {
			return previewURL, fmt.Errorf("preview: open Windows browser: %w", err)
		}
		browserOpened = true
	}
	return previewURL, nil
}

func showXDGLocked(img []byte, mode string) (string, error) {
	if legacyPath == "" {
		dir, err := os.MkdirTemp("", "sextile-preview-")
		if err != nil {
			return "", fmt.Errorf("preview: image directory: %w", err)
		}
		legacyDir = dir
		legacyPath = filepath.Join(dir, chartImageName)
	}
	if err := writePreservingInode(legacyPath, img); err != nil {
		return legacyPath, fmt.Errorf("preview: write image: %w", err)
	}
	if !legacyOpened {
		var err error
		if mode == modeXD {
			err = launchXDGBrowser(legacyPath)
		} else {
			err = launchTerminalXDGBrowser(legacyPath)
		}
		if err != nil {
			return legacyPath, fmt.Errorf("preview: launch %s: %w", mode, err)
		}
		legacyOpened = true
	}
	return legacyPath, nil
}

// Close stops the HTTP server and drops the chart bytes, because the operator
// pressed Ctrl+X on a wheel that is already open. Unlike Cleanup the session
// goes on: a Show that was already on its way finds the viewer closed
// (ErrViewerClosed) instead of starting the server again, and the next
// RequestOpen lets Show start it on the same address. With no server running
// (the xdg backends, or nothing shown yet) it does nothing.
func Close() error {
	previewMu.Lock()
	server := previewServer
	if server == nil {
		previewMu.Unlock()
		return nil
	}
	previewServer = nil
	previewImage = nil
	previewPNG = false
	previewURL = ""
	viewerClosed = true
	previewMu.Unlock()

	if err := server.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("preview: close server: %w", err)
	}
	return nil
}

// Cleanup stops the HTTP server and drops the chart bytes. Safe to call when
// the preview was never opened or has already been cleaned up.
func Cleanup() error {
	previewMu.Lock()
	previewEnd = true
	server := previewServer
	previewServer = nil
	previewImage = nil
	previewPNG = false
	previewURL = ""
	dir := legacyDir
	legacyDir = ""
	legacyPath = ""
	legacyOpened = false
	groups := viewerGroups
	viewerGroups = nil
	legacyProcess = nil
	legacyDone = nil
	viewerClosed = false
	previewMu.Unlock()

	if server != nil {
		if err := server.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("preview: close server: %w", err)
		}
	}
	if dir != "" {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("preview: remove image directory: %w", err)
		}
	}
	var stopErr error
	for _, group := range groups {
		if err := syscall.Kill(-group, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) && stopErr == nil {
			stopErr = fmt.Errorf("preview: stop terminal viewer: %w", err)
		}
	}
	return stopErr
}

func writePreservingInode(dst string, data []byte) error {
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteAt(data, 0); err != nil {
		return err
	}
	return f.Truncate(int64(len(data)))
}

func listenLocal(address string) (net.Listener, error) {
	listener, err := net.Listen("tcp", address)
	if err == nil {
		return listener, nil
	}
	return net.Listen("tcp", "127.0.0.1:0")
}

func windowsBrowserCommand(url string) (*exec.Cmd, error) {
	cmdExe, err := exec.LookPath("cmd.exe")
	if err != nil {
		return nil, fmt.Errorf("cmd.exe not found in WSL PATH: %w", err)
	}
	// start treats its first quoted argument as a window title, hence the empty
	// string before the URL.
	return exec.Command(cmdExe, "/d", "/c", "start", "", url), nil
}

// allowedHost reports whether a request's Host is exactly localhost,
// 127.0.0.1 or [::1], each optionally followed by :<digits>. Anything else is
// a foreign name (a DNS-rebinding page) or malformed, and an empty Host too.
func allowedHost(host string) bool {
	return allowedHostRe.MatchString(host)
}

var allowedHostRe = regexp.MustCompile(`^(?i:localhost|127\.0\.0\.1|\[::1\])(:[0-9]+)?$`)

func servePreview(w http.ResponseWriter, r *http.Request) {
	if !allowedHost(r.Host) {
		w.WriteHeader(http.StatusMisdirectedRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	switch r.URL.Path {
	case "/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src 'self'")
		previewMu.RLock()
		ext := chartExtension(previewPNG)
		previewMu.RUnlock()
		_, _ = w.Write([]byte(strings.ReplaceAll(previewPage, "{{ext}}", ext)))
	case "/version":
		previewMu.RLock()
		version := previewVersion
		previewMu.RUnlock()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprint(w, version)
	case "/chart.svg", "/chart.png":
		previewMu.RLock()
		img := append([]byte(nil), previewImage...)
		png := previewPNG
		previewMu.RUnlock()
		// Only the format being hosted answers; the other path is not there.
		if len(img) == 0 || r.URL.Path != "/chart."+chartExtension(png) {
			http.NotFound(w, r)
			return
		}
		if png {
			w.Header().Set("Content-Type", "image/png")
		} else {
			w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(img)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(img)
		}
	default:
		http.NotFound(w, r)
	}
}

// chartExtension is the file extension the hosted chart goes by.
func chartExtension(png bool) string {
	if png {
		return "png"
	}
	return "svg"
}

const previewPage = `<!doctype html>
<html lang="en">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Sextile wheel</title>
<style>
html,body{width:100%;height:100%;margin:0;overflow:hidden;background:#111;color:#ddd}
body{font:14px system-ui,sans-serif}
img{display:block;width:100vw;height:100vh;object-fit:contain}
</style>
<img id="chart" alt="Sextile chart">
<script>
const chart=document.querySelector('#chart');
let shown='';
// A format change while this tab is open (wheel_format) leaves it asking for the
// other file: load the page again, which asks for the right one.
chart.onerror=()=>setTimeout(()=>location.reload(),1000);
async function refresh(){
  try{
    const response=await fetch('/version',{cache:'no-store'});
    if(response.ok){
      const next=(await response.text()).trim();
      if(next!==shown){
        chart.src='/chart.{{ext}}?v='+encodeURIComponent(next);
        shown=next;
      }
    }
  }catch(_){}
  setTimeout(refresh,250);
}
refresh();
</script>
</html>
`
