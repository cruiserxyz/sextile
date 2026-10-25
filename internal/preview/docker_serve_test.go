package preview

import (
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// realListen points listenPreview at the production listener, which
// resetPreview replaces with a loopback-port-0 stub.
func realListen() { listenPreview = listenForPreview }

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

func tcpAddr(t *testing.T, l net.Listener) *net.TCPAddr {
	t.Helper()
	a, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address %v is not TCP", l.Addr())
	}
	return a
}

func TestPreviewAddrIsExact(t *testing.T) {
	resetPreview(t)
	realListen()
	want := freeAddr(t)
	t.Setenv(previewAddrEnv, want)
	wantTCP, err := net.ResolveTCPAddr("tcp", want)
	if err != nil {
		t.Fatal(err)
	}
	// The listener itself, through the production function.
	l, err := listenForPreview()
	if err != nil {
		t.Fatalf("listenForPreview: %v", err)
	}
	got := tcpAddr(t, l)
	if !got.IP.Equal(wantTCP.IP) || got.Port != wantTCP.Port {
		t.Errorf("bound %v, want exactly %v", got, wantTCP)
	}
	l.Close()
	// And through Show, which must serve at that port.
	url, err := Show([]byte(`<svg/>`))
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if url != "http://localhost:"+strconv.Itoa(wantTCP.Port)+"/" {
		t.Fatalf("url = %q, want port %d", url, wantTCP.Port)
	}
}

func TestPreviewDefaultStaysLoopback(t *testing.T) {
	resetPreview(t)
	realListen()
	t.Setenv(previewAddrEnv, "")
	if preferredAddress != "127.0.0.1:5307" {
		t.Fatalf("preferredAddress = %q, want 127.0.0.1:5307", preferredAddress)
	}
	setPreferred := func(t *testing.T, addr string) {
		old := preferredAddress
		preferredAddress = addr
		t.Cleanup(func() { preferredAddress = old })
	}
	t.Run("preferred port taken falls back to a loopback port", func(t *testing.T) {
		occupier, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer occupier.Close()
		setPreferred(t, occupier.Addr().String())
		taken := tcpAddr(t, occupier)
		l, err := listenForPreview()
		if err != nil {
			t.Fatalf("listenForPreview: %v", err)
		}
		defer l.Close()
		a := tcpAddr(t, l)
		if !a.IP.Equal(net.IPv4(127, 0, 0, 1)) || a.Port == taken.Port {
			t.Fatalf("fallback bound %v, want 127.0.0.1 and a port other than %d", a, taken.Port)
		}
	})
	t.Run("preferred port free binds it exactly", func(t *testing.T) {
		free := freeAddr(t)
		setPreferred(t, free)
		want, _ := net.ResolveTCPAddr("tcp", free)
		l, err := listenForPreview()
		if err != nil {
			t.Fatalf("listenForPreview: %v", err)
		}
		defer l.Close()
		a := tcpAddr(t, l)
		if !a.IP.Equal(want.IP) || a.Port != want.Port {
			t.Fatalf("bound %v, want exactly %v", a, want)
		}
	})
}

func TestPreviewAddrBusyFailsLoud(t *testing.T) {
	resetPreview(t)
	realListen()
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	t.Setenv(previewAddrEnv, busy.Addr().String())
	if _, err := Show([]byte(`<svg/>`)); err == nil {
		t.Fatal("Show succeeded although the set address is occupied")
	}
	previewMu.RLock()
	defer previewMu.RUnlock()
	if previewServer != nil {
		t.Fatal("a server was started after the listen failure")
	}
}

func TestServeModeNeverLaunches(t *testing.T) {
	opened := resetPreview(t)
	t.Setenv(previewModeEnv, "serve")
	url, err := Show([]byte(`<svg id="one"/>`))
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if len(*opened) != 0 {
		t.Fatalf("serve mode launched a browser: %v", *opened)
	}
	if got := string(body(t, get(t, url+"chart.svg"))); got != `<svg id="one"/>` {
		t.Fatalf("first image = %q", got)
	}
	url2, err := Show([]byte(`<svg id="two"/>`))
	if err != nil || url2 != url {
		t.Fatalf("second Show = (%q, %v), want (%q, nil)", url2, err, url)
	}
	if got := string(body(t, get(t, url+"chart.svg"))); got != `<svg id="two"/>` {
		t.Fatalf("second image = %q, want the new one at the same URL", got)
	}
	if len(*opened) != 0 {
		t.Fatalf("serve mode launched a browser: %v", *opened)
	}
}

func TestPreviewRefusesForeignHost(t *testing.T) {
	resetPreview(t)
	url, err := Show([]byte(`<svg/>`))
	if err != nil {
		t.Fatal(err)
	}
	addr := strings.TrimSuffix(strings.TrimPrefix(url, "http://localhost:"), "/")
	_, port, _ := net.SplitHostPort("x:" + addr)
	for _, path := range []string{"/", "/chart.svg", "/version"} {
		for _, host := range []string{"evil.example", "203.0.113.5:5307", "[localhost", "localhost]", "[::1", "localhost:abc", "::1", "localhost:5307x"} {
			req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+port+path, nil)
			req.Host = host
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s Host %q: %v", path, host, err)
			}
			b := body(t, resp)
			if resp.StatusCode != http.StatusMisdirectedRequest || len(b) != 0 {
				t.Errorf("%s Host %q: status %d body %q, want 421 and no body", path, host, resp.StatusCode, b)
			}
		}
		for _, method := range []string{http.MethodHead, http.MethodPost} {
			req, _ := http.NewRequest(method, "http://127.0.0.1:"+port+path, nil)
			req.Host = "evil.example"
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s %s foreign Host: %v", method, path, err)
			}
			b := body(t, resp)
			if resp.StatusCode != http.StatusMisdirectedRequest || len(b) != 0 {
				t.Errorf("%s %s foreign Host: status %d body %q, want 421 and no body", method, path, resp.StatusCode, b)
			}
		}
		headReq, _ := http.NewRequest(http.MethodHead, "http://127.0.0.1:"+port+path, nil)
		headReq.Host = "localhost"
		if resp, err := http.DefaultClient.Do(headReq); err != nil {
			t.Fatalf("HEAD %s localhost: %v", path, err)
		} else {
			resp.Body.Close()
			if resp.StatusCode == http.StatusMisdirectedRequest {
				t.Errorf("HEAD %s with Host localhost got 421", path)
			}
		}
		// An empty Host cannot be sent by the Go client, so write it raw
		// (HTTP/1.0 carries no Host).
		conn, err := net.Dial("tcp", "127.0.0.1:"+port)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = conn.Write([]byte("GET " + path + " HTTP/1.0\r\n\r\n"))
		raw, _ := io.ReadAll(conn)
		conn.Close()
		if !strings.HasPrefix(string(raw), "HTTP/1.0 421 ") || strings.Contains(string(raw), "\r\n\r\n<") || !strings.HasSuffix(string(raw), "\r\n\r\n") {
			t.Errorf("%s with no Host: got %q, want 421 and no body", path, raw)
		}
		for _, host := range []string{"localhost", "localhost:5307", "127.0.0.1", "127.0.0.1:1", "[::1]", "[::1]:5307"} {
			req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+port+path, nil)
			req.Host = host
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s Host %q: %v", path, host, err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("%s Host %q: status %d, want 200", path, host, resp.StatusCode)
			}
		}
	}
}
