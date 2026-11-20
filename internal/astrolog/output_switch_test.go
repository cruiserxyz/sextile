// All chart values in this file are constructed synthetic test inputs.
package astrolog

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// O4a: no Astrolog call is given a path inside the charts folder after an
// output switch. The rename and the save are run through a shim that records
// the working folder and the arguments; every output path, resolved against
// the working folder, lies outside the folder that holds the chart. The scan
// below keeps the list of output sites closed.
func TestNoOutputSwitchPointsIntoTheChartsFolder(t *testing.T) {
	real, err := exec.LookPath("astrolog")
	if err != nil {
		t.Skip("astrolog is not on PATH")
	}
	shimDir, log := t.TempDir(), filepath.Join(t.TempDir(), "calls")
	script := "#!/bin/sh\npwd >> " + strconv.Quote(log) + "\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> " + strconv.Quote(log) + "; done\nprintf -- '--end--\\n' >> " + strconv.Quote(log) + "\nexec " + strconv.Quote(real) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(shimDir, "astrolog"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	charts := t.TempDir()
	src := filepath.Join(charts, "Anna.as")
	if err := os.WriteFile(src, []byte("@AI780  ; Astrolog chart info.\n-qb Sep 23 2026 2:06 DT 1E   8:48E 53:04N\n-zi \"Anna\" \"Bremen\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RetitleRecord(context.Background(), src, "Anna Two", "Bremen"); err != nil {
		t.Fatal(err)
	}
	if _, err := ChartRecord(context.Background(), ChartToSave{Name: "Ben", Place: "Bremen", When: time.Date(1990, 1, 2, 3, 4, 0, 0, time.UTC), Lon: 8.8, Lat: 53.07}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	outputs := 0
	for _, call := range strings.Split(strings.TrimSuffix(string(b), "--end--\n"), "--end--\n") {
		lines := strings.Split(strings.TrimSuffix(call, "\n"), "\n")
		cwd, args := lines[0], lines[1:]
		for i, a := range args {
			if (a == "=o" || a == "-o" || a == "-Xo") && i+1 < len(args) {
				outputs++
				out := args[i+1]
				if !filepath.IsAbs(out) {
					out = filepath.Join(cwd, out)
				}
				if rel, err := filepath.Rel(charts, out); err == nil && !strings.HasPrefix(rel, "..") {
					t.Errorf("%s %s writes into the charts folder", a, args[i+1])
				}
			}
		}
	}
	if outputs != 2 {
		t.Errorf("saw %d output switches, want the rename's and the save's", outputs)
	}

	// The output sites in the package's own source: each one is named here
	// with what it writes into, so a new one is placed on purpose.
	known := map[string]int{"retitle.go": 1, "savechart.go": 1, "image.go": 1}
	got := map[string]int{}
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, sw := range []string{`"=o"`, `"-o"`, `"-Xo"`} {
			got[f] += strings.Count(string(src), sw)
		}
		if got[f] == 0 {
			delete(got, f)
		}
	}
	for f, n := range got {
		if known[f] != n {
			t.Errorf("%s holds %d output switches, the list says %d: place each one outside the charts folder and list it", f, n, known[f])
		}
	}
}
