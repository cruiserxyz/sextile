package astrolog

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/testenv/astrodir"
)

// C1: every process buildCmd starts begins with -YQ 0, then the hook-clear band
// (the specification C1 amends this row), then the call's own argv,
// which itself is left as it was.
func TestEveryCallTurnsPagingOff(t *testing.T) {
	ctx := context.Background()
	ownArgv := []string{"-qm", "-R0", "1"}
	body := BodyCall{argv: ownArgv, band: newBand([]string{"-R0", "1"})}
	for _, c := range []struct {
		name string
		call spawnCall
		own  []string
	}{
		{"body call", body, ownArgv},
		{"exempt call", newExempt("test: no chart", "-HO"), []string{"-HO"}},
		{"bounded exempt call", newExempt("test: no chart", "-Hc").bounded(), []string{"-Hc"}},
	} {
		cmd, argv, err := buildCmd(ctx, c.call)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		want := append(append([]string{"-YQ", "0"}, HookClearBand()...), c.own...)
		if !reflect.DeepEqual(cmd.Args[1:], want) {
			t.Errorf("%s: executed argv = %v, want %v", c.name, cmd.Args[1:], want)
		}
		if !reflect.DeepEqual(argv, want) {
			t.Errorf("%s: returned argv = %v, want %v", c.name, argv, want)
		}
	}
	if !reflect.DeepEqual(body.Argv(), []string{"-qm", "-R0", "1"}) || !reflect.DeepEqual(ownArgv, []string{"-qm", "-R0", "1"}) {
		t.Errorf("the call's own argv changed: %v", body.Argv())
	}
}

// C2: the recorded command is what ran, -YQ 0 included.
func TestRecordedCommandIncludesNoPaging(t *testing.T) {
	old := binary
	binary = "echo"
	t.Cleanup(func() { binary = old })
	out, command, err := spawn(context.Background(), newExempt("test: no chart", "hello"))
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := strings.Join(append(append([]string{"-YQ", "0"}, HookClearBand()...), "hello"), " ")
	if got := strings.TrimSpace(out); got != wantArgs {
		t.Fatalf("echo received %q, want %q", got, wantArgs)
	}
	if !strings.Contains(command, " -YQ 0 ") || !strings.HasSuffix(command, " hello") || strings.Index(command, "-YQ 0") > strings.Index(command, "hello") {
		t.Errorf("recorded command %q does not carry -YQ 0 before the call's argv", command)
	}
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	ents, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// C3: a data directory whose astrolog.as pages at 7 lines (the distinctive length the test writes) must not hang a long
// answer or the version read.
func TestPagingAstrologAsDoesNotHang(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary integration test")
	}
	src := astrodir.Need(t)
	dir := t.TempDir()
	copyDir(t, src, dir)
	as := filepath.Join(dir, "astrolog.as")
	b, err := os.ReadFile(as)
	if err != nil {
		t.Fatal(err)
	}
	// Rewrite an existing -YQ line to 7, or append one: the test must not
	// become vacuous on an install whose astrolog.as has none.
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	changed := false
	for i, l := range lines {
		if strings.HasPrefix(l, "-YQ ") {
			lines[i] = "-YQ 7"
			changed = true
		}
	}
	if !changed {
		lines = append(lines, "-YQ 7")
	}
	if err := os.WriteFile(as, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(as)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, l := range strings.Split(string(written), "\n") {
		if strings.HasPrefix(l, "-YQ ") {
			if l != "-YQ 7" {
				t.Fatalf("astrolog.as carries %q, want -YQ 7", l)
			}
			n++
		}
	}
	if n != 1 {
		t.Fatalf("astrolog.as has %d -YQ lines, want exactly one -YQ 7", n)
	}
	t.Setenv("ASTROLOG", dir)
	// Register restoration first, then unset: t.Setenv puts back whatever was
	// there (set or unset) when the test ends.
	t.Setenv("ASTR800", os.Getenv("ASTR800"))
	t.Setenv("ASTR", os.Getenv("ASTR"))
	os.Unsetenv("ASTR800")
	os.Unsetenv("ASTR")
	t.Chdir(t.TempDir())

	// Precondition: the paging file is the one Astrolog really loads. Run the
	// binary WITHOUT -YQ 0 on a long answer and require the pager prompt; a
	// file beside the binary or in the cwd that shadows ASTROLOG would make
	// every assertion below pass for no reason.
	pctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	raw := exec.CommandContext(pctx, "astrolog", "-qa", "6", "15", "1990", "14:30", "5:00", "122W", "37N", "-a")
	raw.Stdin = nil // /dev/null
	rawOut, _ := raw.CombinedOutput()
	// 7 is distinctive: the host's own page length (24) or any other file
	// pages somewhere else. The prompt follows exactly 7 complete lines.
	prompt := strings.Index(string(rawOut), "Press return")
	if prompt < 0 || strings.Count(string(rawOut)[:prompt], "\n") != 7 {
		t.Fatal("the paging astrolog.as is not the one Astrolog loads (a file beside the binary or in the cwd shadows ASTROLOG)")
	}

	out, err := Run(context.Background(), testCall([]string{"-qa", "6", "15", "1990", "14:30", "5:00", "122W", "37N"}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(out.Raw, "Press return") {
		t.Errorf("Run output carries a pager prompt")
	}
	if !strings.Contains(out.Raw, "Sun :") {
		t.Errorf("Run output has no `Sun :` position row")
	}
	if len(out.Aspects) == 0 {
		t.Errorf("Run returned no aspects")
	}
	v, _, err := Version()
	if err != nil || v != "8.00" {
		t.Errorf("Version = %q, %v; want 8.00", v, err)
	}
}
