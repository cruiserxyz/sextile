package preview

import (
	"errors"
	"net/http"
	"os"
	"sync"
	"testing"
)

// A client's chart stays in memory. Publishing and refreshing must not create
// a wheel image anywhere under TMPDIR.
func TestChartImageIsNotWrittenAtRest(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	resetPreview(t)
	if _, err := Show([]byte(`<svg id="client-chart"/>`)); err != nil {
		t.Fatalf("Show: %v", err)
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("preview wrote client data under TMPDIR: %v", entries)
	}
}

func TestCleanupStopsServingAndShowCannotRestart(t *testing.T) {
	resetPreview(t)
	url, err := Show([]byte(`<svg id="client-chart"/>`))
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if err := Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	client := &http.Client{}
	if response, err := client.Get(url + "chart.svg"); err == nil {
		response.Body.Close()
		t.Fatalf("preview still answers after Cleanup: %s", response.Status)
	}
	got, err := Show([]byte(`<svg id="late-publish"/>`))
	if !errors.Is(err, ErrSessionEnded) {
		t.Fatalf("late Show err = %v, want ErrSessionEnded", err)
	}
	if got != "" {
		t.Fatalf("late Show returned %q; no server may restart after quit", got)
	}
	previewMu.RLock()
	defer previewMu.RUnlock()
	if previewImage != nil || previewServer != nil {
		t.Fatal("Cleanup retained chart bytes or a server")
	}
}

// Show runs in a tea.Cmd while quit runs on the Update loop. Whichever wins,
// Cleanup closes the session permanently and drops every chart byte.
func TestShowAndCleanupDoNotRace(t *testing.T) {
	resetPreview(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = Show([]byte(`<svg id="concurrent"/>`))
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = Cleanup()
	}()
	wg.Wait()

	if _, err := Show([]byte(`<svg id="after"/>`)); !errors.Is(err, ErrSessionEnded) {
		t.Fatalf("after Cleanup, Show err = %v, want ErrSessionEnded", err)
	}
	previewMu.RLock()
	defer previewMu.RUnlock()
	if previewImage != nil || previewServer != nil {
		t.Fatal("concurrent cleanup retained chart bytes or a server")
	}
}
