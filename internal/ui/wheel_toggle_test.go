package ui

import (
	"net"
	"net/http"
	"strings"
	"testing"
)

// A second Ctrl+X closes the wheel; a third opens it again (the specification
// W1 part 3, A2).

// liveWheel is a stand-in for the preview package's server that really
// listens, so "the port refuses" is observed on a socket and not inferred from
// a flag: showWheel starts it on first use and answers its address, and
// closeWheelServer stops it. The real server's close is pinned in
// internal/preview.
type liveWheel struct {
	t        *testing.T
	l        net.Listener
	lastAddr string
	closed   int
}

func (w *liveWheel) url() string { return "http://" + w.l.Addr().String() + "/" }

func installLiveWheel(t *testing.T) *liveWheel {
	t.Helper()
	w := &liveWheel{t: t}
	prevShow, prevClose := showWheel, closeWheelServer
	showWheel = func([]byte) (string, error) {
		if w.l == nil {
			// Like the real server, a reopened wheel takes its old address.
			addr := "127.0.0.1:0"
			if w.lastAddr != "" {
				addr = w.lastAddr
			}
			l, err := net.Listen("tcp", addr)
			if err != nil {
				return "", err
			}
			w.l = l
			go func() { _ = http.Serve(l, http.NotFoundHandler()) }()
		}
		return w.url(), nil
	}
	closeWheelServer = func() error {
		w.closed++
		if w.l != nil {
			err := w.l.Close()
			addr := w.l.Addr().String()
			w.l = nil
			w.lastAddr = addr
			return err
		}
		return nil
	}
	t.Cleanup(func() {
		showWheel, closeWheelServer = prevShow, prevClose
		if w.l != nil {
			w.l.Close()
		}
	})
	return w
}

// refused reports whether nothing answers at addr any more.
func refused(addr string) bool {
	c, err := net.Dial("tcp", addr)
	if err == nil {
		c.Close()
		return false
	}
	return true
}

func lanesText(m Model, lane logLane) []string {
	var out []string
	for _, l := range m.logLines {
		if l.lane == lane {
			out = append(out, l.text)
		}
	}
	return out
}

// reopen is the third press: Ctrl+X, the render it asks for, its publish.
func reopen(t *testing.T, m Model) Model {
	t.Helper()
	m, cmd := m.dispatchRenderWheel()
	if !m.previewActive || cmd == nil {
		t.Fatalf("Ctrl+X on a closed wheel did not start a render (active=%v)", m.previewActive)
	}
	st := imageState(m)
	m, cmd = m.onRefreshResult(refreshResultMsg{
		surface: surfaceImage, runID: st.runningRunID, wantID: st.wantID,
		payload: imagePayload{bytes: []byte("<svg/>")},
	})
	if cmd == nil {
		t.Fatal("the render did not start a publish")
	}
	m, _ = m.onWheelPublished(cmd().(wheelPublishedMsg))
	return m
}

func TestCtrlXTogglesPreviewClosed(t *testing.T) {
	w := installLiveWheel(t)
	m := publishOnce(t, modelWithSlot(t))
	addr := w.l.Addr().String()
	if refused(addr) {
		t.Fatal("the wheel is not being served before the second Ctrl+X")
	}

	m, cmd := m.dispatchRenderWheel()

	if cmd != nil {
		t.Error("closing the wheel started more work")
	}
	if m.previewActive {
		t.Error("previewActive is still true after the wheel was closed")
	}
	if !refused(addr) {
		t.Error("the wheel still answers after the second Ctrl+X")
	}
	if got := lanesText(m, laneWheelClosed); len(got) != 1 || got[0] != "Wheel closed; Ctrl+X opens it again" {
		t.Errorf("closed lines = %q, want exactly the one", got)
	}
	if n := len(lanesText(m, laneError)); n != 0 || m.errorUnseen {
		t.Errorf("closing is not an error: %d error lines, errorUnseen=%v", n, m.errorUnseen)
	}
}

func TestCtrlXTogglesPreviewClosedThenReopens(t *testing.T) {
	w := installLiveWheel(t)
	m := publishOnce(t, modelWithSlot(t))
	m, _ = m.dispatchRenderWheel() // closes
	if w.closed != 1 {
		t.Fatalf("the server was closed %d times, want 1", w.closed)
	}

	m = reopen(t, m)

	if !m.previewActive || refused(w.l.Addr().String()) {
		t.Error("the wheel is not being served after the third Ctrl+X")
	}
	open := lanesText(m, laneWheelOpen)
	if len(open) != 2 || open[0] != open[1] {
		t.Errorf("notice lines = %q, want the address named again on reopening", open)
	}
	// The order of the story: open, closed, open.
	var order []logLane
	for _, l := range m.logLines {
		if l.lane == laneWheelOpen || l.lane == laneWheelClosed {
			order = append(order, l.lane)
		}
	}
	if len(order) != 3 || order[0] != laneWheelOpen || order[1] != laneWheelClosed || order[2] != laneWheelOpen {
		t.Errorf("wheel lines run %v, want open, closed, open", order)
	}
}

// A press that lands before the first publish has answered is a refresh, as
// it always was: there is no server to close yet.
func TestCtrlXBeforeTheFirstPublishDoesNotClose(t *testing.T) {
	w := installLiveWheel(t)
	m := modelWithSlot(t)
	m, _ = m.dispatchRenderWheel()
	m, _ = m.dispatchRenderWheel()
	if w.closed != 0 || !m.previewActive {
		t.Errorf("closed %d times, active=%v: the second press before any publish must not close", w.closed, m.previewActive)
	}
	if got := lanesText(m, laneWheelClosed); len(got) != 0 {
		t.Errorf("closed lines = %q", got)
	}
}

// A publish that lands after the close says nothing: the line would claim a
// wheel that is gone.
func TestWheelNoticeNotAfterClose(t *testing.T) {
	stubShowWheelURL(t, wheelURLStub)
	m := modelWithSlot(t)
	m.previewActive = false
	m, _ = m.onWheelPublished(wheelPublishedMsg{path: wheelURLStub})
	if got := wheelOpenLines(m); len(got) != 0 {
		t.Errorf("a publish after the close wrote %q", got)
	}
	if strings.Contains(m.wheelURL, "http") {
		t.Errorf("wheelURL = %q after a publish into a closed wheel", m.wheelURL)
	}
}
