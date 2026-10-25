// All chart values in this file are constructed synthetic test inputs.
package store_test

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
)

// A Sextile killed in the middle of writing a chart leaves the chart's name
// free: nothing under it, nothing the library lists, and the next save under
// the same name succeeds. The save runs in a child process (this test binary,
// re-run as TestPlaceChartFileKilledMidWriteHelper) that is SIGKILLed while
// half the record is written.
//
// It lives outside package store because the listing and the full save are
// internal/lookup's, which imports store.
func TestSaveKilledMidWriteLeavesTheNameFree(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestPlaceChartFileKilledMidWriteHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "SEXTILE_TEST_KILL_SAVE_DIR="+dir)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	half := make(chan bool, 1)
	go func() {
		s := bufio.NewScanner(stdout)
		for s.Scan() {
			if s.Text() == "half written" {
				half <- true
				return
			}
		}
		half <- false
	}()
	select {
	case ok := <-half:
		if !ok {
			cmd.Wait()
			t.Fatal("the save process ended without writing half the record")
		}
	case <-time.After(60 * time.Second):
		cmd.Process.Kill()
		cmd.Wait()
		t.Fatal("the save process never reached the middle of its write")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()

	if _, err := os.Lstat(filepath.Join(dir, "client.as")); !os.IsNotExist(err) {
		t.Fatalf("the killed save left something under the chart's name (lstat: %v)", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Error("the killed save left no temp file: the kill did not land mid-write")
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") || !strings.HasSuffix(e.Name(), ".sextile-tmp") {
			t.Errorf("the killed save left %s, which is not a hidden temp file", e.Name())
		}
	}
	lib, err := lookup.LoadChartLibraryIn(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range lib.Charts {
		t.Errorf("the library lists %q after the killed save", c.Name)
	}
	for _, d := range lib.Drops {
		t.Errorf("the library reports %s after the killed save: %s", d.File, d.Reason)
	}

	saved, err := lookup.SaveFloatingChart(context.Background(), dir, "Client",
		time.Date(1987, 3, 14, 16, 30, 0, 0, time.FixedZone("CEST", 2*3600)),
		location.PlaceInfo{Name: "Amsterdam", Country: "NL", Lat: 52.3667, Lon: 4.9, TZ: "Europe/Amsterdam"})
	if err != nil {
		t.Fatalf("a fresh save under the same name: %v", err)
	}
	if saved.File != "client.as" {
		t.Errorf("the fresh save wrote %s, want client.as", saved.File)
	}
}
