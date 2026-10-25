package env

import (
	"bytes"
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// These tests run the real build the way a user would: a probe binary
// (internal/envprobe) is compiled once per variant, with and without
// `-tags dev`, and executed in a temp tree. An in-process test could not tell a
// release binary that never reads .env from one that reads it and forgets.

var probeBuilds = struct {
	sync.Mutex
	dir   string
	built map[string]string
	err   map[string]error
}{built: map[string]string{}, err: map[string]error{}}

// probeBinary builds envprobe once per tag variant ("" or "dev") into a temp
// dir that lives for the whole test binary.
func probeBinary(t *testing.T, tags string) string {
	t.Helper()
	probeBuilds.Lock()
	defer probeBuilds.Unlock()
	if bin, ok := probeBuilds.built[tags]; ok {
		return bin
	}
	if err, ok := probeBuilds.err[tags]; ok {
		t.Fatalf("probe build failed earlier: %v", err)
	}
	if probeBuilds.dir == "" {
		d, err := os.MkdirTemp("", "envprobe-")
		if err != nil {
			t.Fatal(err)
		}
		probeBuilds.dir = d
	}
	name := "probe"
	args := []string{"build", "-buildvcs=false", "-o"}
	if tags != "" {
		name += "-" + tags
	}
	bin := filepath.Join(probeBuilds.dir, name)
	args = append(args, bin)
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	args = append(args, "./internal/envprobe")
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "GOFLAGS=-p=4", "GOMAXPROCS=4", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		probeBuilds.err[tags] = err
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	probeBuilds.built[tags] = bin
	return bin
}

func TestMain(m *testing.M) {
	code := m.Run()
	probeBuilds.Lock()
	if probeBuilds.dir != "" {
		os.RemoveAll(probeBuilds.dir)
	}
	probeBuilds.Unlock()
	os.Exit(code)
}

// tree is a temp repo: root holds go.mod, and run starts in root/a/b.
type tree struct {
	root, child, home string
}

func newTree(t *testing.T, dotenv string) tree {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module probe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if dotenv != "" {
		if err := os.WriteFile(filepath.Join(root, ".env"), []byte(dotenv), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return tree{root: root, child: child, home: home}
}

// run executes the probe in the child directory with a minimal environment
// (nothing from the test's own), plus extra KEY=value pairs.
func (tr tree) run(t *testing.T, bin string, extra []string, args ...string) (stdout, stderr string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = tr.child
	cmd.Env = append([]string{"HOME=" + tr.home, "PATH=" + os.Getenv("PATH")}, extra...)
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %v: %v\nstderr: %s", bin, args, err, e.String())
	}
	return o.String(), e.String()
}

const fourKeys = "PLAIN_KEY=plain\n" +
	"QUOTED_KEY=\"two words\"\n" +
	"DUP_KEY=first\nDUP_KEY=last\n" +
	"export EXPORTED_KEY=exp\n"

var fourKeyNames = []string{"PLAIN_KEY", "QUOTED_KEY", "DUP_KEY", "EXPORTED_KEY"}

// C1: a release build never reads a .env it finds above its working directory.
func TestReleaseBuildIgnoresDotEnv(t *testing.T) {
	bin := probeBinary(t, "")
	tr := newTree(t, fourKeys)
	out, errOut := tr.run(t, bin, nil, append([]string{"get"}, fourKeyNames...)...)
	for _, k := range fourKeyNames {
		if !strings.Contains(out, k+"=") {
			t.Errorf("probe output lacks the line for %s (the probe did not ask?):\n%s", k, out)
		}
	}
	for _, v := range []string{"plain", "two words", "first", "last", "exp"} {
		if strings.Contains(out, v) {
			t.Errorf("a release build loaded %q from .env:\n%s", v, out)
		}
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want empty", errOut)
	}

	// An unreadable .env: a directory opens but cannot be read, and a mode-0
	// file cannot be opened. A build that so much as tries says so on stderr
	// (the dev build does, below), so empty stderr proves it never looked.
	for name, mk := range map[string]func(string) error{
		"directory": func(p string) error { return os.Mkdir(p, 0o755) },
		"mode 000":  func(p string) error { return os.WriteFile(p, []byte(fourKeys), 0o000) },
	} {
		t.Run("unreadable "+name, func(t *testing.T) {
			if name == "mode 000" && os.Geteuid() == 0 {
				t.Skip("root reads a mode-000 file")
			}
			tr := newTree(t, "")
			if err := mk(filepath.Join(tr.root, ".env")); err != nil {
				t.Fatal(err)
			}
			_, errOut := tr.run(t, bin, nil, "get", "PLAIN_KEY")
			if errOut != "" {
				t.Errorf("stderr = %q, want empty: the release build touched .env", errOut)
			}
		})
	}
}

// C2: a dev build parses .env as the loader always has, and the real
// environment wins over the file.
func TestDevBuildLoadsDotEnv(t *testing.T) {
	bin := probeBinary(t, "dev")
	tr := newTree(t, fourKeys)
	out, errOut := tr.run(t, bin, nil, append([]string{"get"}, fourKeyNames...)...)
	for _, want := range []string{"PLAIN_KEY=plain\n", "QUOTED_KEY=two words\n", "DUP_KEY=last\n", "EXPORTED_KEY=exp\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("dev build output lacks %q:\n%s", want, out)
		}
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want empty", errOut)
	}

	out, _ = tr.run(t, bin, []string{"PLAIN_KEY=from-real-env"}, "get", "PLAIN_KEY")
	if out != "PLAIN_KEY=from-real-env\n" {
		t.Errorf("an exported variable must win over .env, got %q", out)
	}

	// Control for C1's unreadable-.env rows: the dev build does report it.
	bad := newTree(t, "")
	if err := os.Mkdir(filepath.Join(bad.root, ".env"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, errOut := bad.run(t, bin, nil, "get", "PLAIN_KEY"); errOut == "" {
		t.Error("a dev build said nothing about an unreadable .env, so C1's empty-stderr check proves nothing")
	}
}

// Merely importing env reads no .env (the specification pass 6: a refused
// launch reads nothing). Only an explicit Load, or an accessor that needs the
// root, does.
func TestImportingEnvDoesNotLoadDotEnv(t *testing.T) {
	bin := probeBinary(t, "dev")
	tr := newTree(t, fourKeys)
	out, _ := tr.run(t, bin, nil, "noload", "PLAIN_KEY")
	if out != "PLAIN_KEY=\n" {
		t.Errorf("a dev binary that only imported env already applied .env: %q", out)
	}
	if out, _ := tr.run(t, bin, nil, "get", "PLAIN_KEY"); out != "PLAIN_KEY=plain\n" {
		t.Errorf("control: an explicit Load must still apply .env, got %q", out)
	}
}

// C3: the tag changes one constant and nothing else.
func TestDevTagSelectsOneConstant(t *testing.T) {
	files := func(tags string) []string {
		args := []string{"list"}
		if tags != "" {
			args = append(args, "-tags", tags)
		}
		args = append(args, "-f", "{{range .GoFiles}}{{.}}\n{{end}}", ".")
		cmd := exec.Command("go", args...)
		cmd.Env = append(os.Environ(), "GOWORK=off")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("go %s: %v", strings.Join(args, " "), err)
		}
		fs := strings.Fields(string(out))
		sort.Strings(fs)
		return fs
	}
	rel, dev := files(""), files("dev")
	only := func(a, b []string) (out []string) {
		in := map[string]bool{}
		for _, f := range b {
			in[f] = true
		}
		for _, f := range a {
			if !in[f] {
				out = append(out, f)
			}
		}
		return out
	}
	if got := only(rel, dev); len(got) != 1 || got[0] != "dev_off.go" {
		t.Errorf("files only in the release build = %v, want [dev_off.go]", got)
	}
	if got := only(dev, rel); len(got) != 1 || got[0] != "dev_on.go" {
		t.Errorf("files only in the dev build = %v, want [dev_on.go]", got)
	}
}

// C4: a release build keeps a relative path relative, even under a go.mod.
func TestReleaseResolveDirKeepsRelative(t *testing.T) {
	tr := newTree(t, "")
	out, _ := tr.run(t, probeBinary(t, ""), nil, "resolve", "rel/x")
	if got := strings.TrimSpace(out); got != "rel/x" {
		t.Errorf("release resolve = %q, want rel/x", got)
	}
	out, _ = tr.run(t, probeBinary(t, "dev"), nil, "resolve", "rel/x")
	if got, want := strings.TrimSpace(out), filepath.Join(tr.root, "rel", "x"); got != want {
		t.Errorf("dev resolve = %q, want %q", got, want)
	}
}

// C5: tier 3 of store.ChartsDir, the dev chart folder, steers a dev build only.
func TestReleaseIgnoresDevChartsDir(t *testing.T) {
	tr := newTree(t, "")
	devDir := filepath.Join(tr.home, "fixtures")
	if err := os.Mkdir(devDir, 0o755); err != nil {
		t.Fatal(err)
	}
	set := []string{"SEXTILE_DEV_CHARTS_DIR=" + devDir}
	xdgDefault := filepath.Join(tr.home, ".local", "share", "sextile", "charts")

	out, _ := tr.run(t, probeBinary(t, ""), set, "charts")
	if got := strings.TrimSpace(out); got != xdgDefault {
		t.Errorf("release ChartsDir = %q, want the XDG default %q", got, xdgDefault)
	}
	// Pins the seam's default the other way: a real dev build honours tier 3
	// (internal/store's devTier defaults to env.Dev).
	out, _ = tr.run(t, probeBinary(t, "dev"), set, "charts")
	if got := strings.TrimSpace(out); got != devDir {
		t.Errorf("dev ChartsDir = %q, want %q (tier 3 must still work in a dev build)", got, devDir)
	}
}

// runTimed is run with a deadline; it reports whether the deadline killed the
// probe instead of failing on the kill.
func (tr tree) runTimed(t *testing.T, bin string, d time.Duration, args ...string) (timedOut bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = tr.child
	cmd.Env = []string{"HOME=" + tr.home, "PATH=" + os.Getenv("PATH")}
	err := cmd.Run()
	if ctx.Err() != nil {
		return true
	}
	if err != nil {
		t.Fatalf("%s %v: %v", bin, args, err)
	}
	return false
}

// C1, the read that discards values: a .env that is a FIFO with no writer blocks
// anything that opens it. A release probe must finish; the dev probe must block,
// which proves the trap springs on a binary that does read.
func TestReleaseBuildNeverOpensDotEnv(t *testing.T) {
	tr := newTree(t, "")
	if err := syscall.Mkfifo(filepath.Join(tr.root, ".env"), 0o644); err != nil {
		t.Fatal(err)
	}
	if tr.runTimed(t, probeBinary(t, ""), 5*time.Second, "get", "PLAIN_KEY") {
		t.Error("the release probe blocked on a .env FIFO: it opens .env")
	}
	if !tr.runTimed(t, probeBinary(t, "dev"), 2*time.Second, "get", "PLAIN_KEY") {
		t.Error("the dev probe did not block on a .env FIFO, so the trap proves nothing")
	}
}

// C3, beyond file names: each constant file holds the constraint, the package
// clause and ONE const named devBuild, nothing else.
func TestDevTagFilesHoldOnlyTheConstant(t *testing.T) {
	for file, c := range map[string]struct{ constraint, value string }{
		"dev_on.go":  {"//go:build dev", "true"},
		"dev_off.go": {"//go:build !dev", "false"},
	} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if first, _, _ := strings.Cut(string(src), "\n"); first != c.constraint {
			t.Errorf("%s: first line = %q, want %q", file, first, c.constraint)
		}
		f, err := parser.ParseFile(token.NewFileSet(), file, src, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		if f.Name.Name != "env" || len(f.Imports) != 0 || len(f.Decls) != 1 {
			t.Errorf("%s: package %q, %d imports, %d decls; want package env, no imports, one decl", file, f.Name.Name, len(f.Imports), len(f.Decls))
			continue
		}
		gd, ok := f.Decls[0].(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST || len(gd.Specs) != 1 {
			t.Errorf("%s: the one declaration is not a single const", file)
			continue
		}
		vs, ok := gd.Specs[0].(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || vs.Names[0].Name != "devBuild" || len(vs.Values) != 1 {
			t.Errorf("%s: the const is not exactly devBuild = <value>", file)
			continue
		}
		if id, ok := vs.Values[0].(*ast.Ident); !ok || id.Name != c.value {
			t.Errorf("%s: devBuild = %v, want %s", file, vs.Values[0], c.value)
		}
	}
}

// C4/C5 together: a relative dev chart folder, from a child directory under a
// go.mod root, resolves against that root in a dev build and is ignored by a
// release build.
func TestDevRelativeChartsDirResolvesAgainstRoot(t *testing.T) {
	tr := newTree(t, "")
	set := []string{"SEXTILE_DEV_CHARTS_DIR=./rel"}
	out, _ := tr.run(t, probeBinary(t, "dev"), set, "charts")
	if got, want := strings.TrimSpace(out), filepath.Join(tr.root, "rel"); got != want {
		t.Errorf("dev ChartsDir = %q, want %q", got, want)
	}
	out, _ = tr.run(t, probeBinary(t, ""), set, "charts")
	if got, want := strings.TrimSpace(out), filepath.Join(tr.home, ".local", "share", "sextile", "charts"); got != want {
		t.Errorf("release ChartsDir = %q, want the XDG default %q", got, want)
	}
}
