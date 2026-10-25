package env

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseEnv(t *testing.T) {
	in := strings.Join([]string{
		"# a full-line comment",
		"",
		"  SEXTILE_DEV_CHARTS_DIR=./test_charts/  ",
		`QUOTED="has spaces"`,
		"SINGLE='single quoted'",
		"export EXPORTED=value",
		"WITH_HASH=a#b",                // '#' mid-value is kept (no inline-comment stripping)
		"DEMO=Test Subject 1985 0:10W", // unquoted value with spaces
		"=novalue",                     // empty key → skipped
		"NOEQUALS",                     // no '=' → skipped
	}, "\n")

	got, err := parseEnv(strings.NewReader(in))
	if err != nil {
		t.Fatalf("parseEnv: unexpected error: %v", err)
	}
	want := map[string]string{
		"SEXTILE_DEV_CHARTS_DIR": "./test_charts/",
		"QUOTED":                 "has spaces",
		"SINGLE":                 "single quoted",
		"EXPORTED":               "value",
		"WITH_HASH":              "a#b",
		"DEMO":                   "Test Subject 1985 0:10W",
	}
	if len(got) != len(want) {
		t.Fatalf("parseEnv key count = %d, want %d (got %v)", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("parseEnv[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestParseEnv_SurfacesScannerError(t *testing.T) {
	// A line longer than bufio.Scanner's token limit must surface as an error
	// rather than silently yielding a truncated environment.
	huge := "BIG=" + strings.Repeat("x", bufio.MaxScanTokenSize+1)
	if _, err := parseEnv(strings.NewReader(huge)); err == nil {
		t.Error("parseEnv on an oversized line returned nil error (silent truncation)")
	}
}

func TestApplyEnv_RealEnvWins(t *testing.T) {
	t.Setenv("SEXTILE_ENVTEST_PRESET", "from-real-env")

	applyEnv(map[string]string{
		"SEXTILE_ENVTEST_PRESET": "from-dotenv", // must NOT override the real one
		"SEXTILE_ENVTEST_FRESH":  "from-dotenv", // unset → applied
	})
	t.Cleanup(func() { os.Unsetenv("SEXTILE_ENVTEST_FRESH") })

	if got := os.Getenv("SEXTILE_ENVTEST_PRESET"); got != "from-real-env" {
		t.Errorf("preset key overridden: got %q, want from-real-env", got)
	}
	if got := os.Getenv("SEXTILE_ENVTEST_FRESH"); got != "from-dotenv" {
		t.Errorf("fresh key not applied: got %q, want from-dotenv", got)
	}
}

func TestResolveDirIn(t *testing.T) {
	root := filepath.FromSlash("/repo")
	cases := []struct {
		name, root, in, want string
	}{
		{"relative joins root", root, "./test_charts/", filepath.Join(root, "test_charts")},
		{"bare relative joins root", root, "charts", filepath.Join(root, "charts")},
		{"absolute passes through", root, filepath.FromSlash("/abs/charts"), filepath.FromSlash("/abs/charts")},
		{"empty passes through", root, "", ""},
		{"unknown root returns input", "", "./test_charts", "./test_charts"},
	}
	for _, c := range cases {
		if got := resolveDirIn(c.root, c.in); got != c.want {
			t.Errorf("%s: resolveDirIn(%q, %q) = %q, want %q", c.name, c.root, c.in, got, c.want)
		}
	}
}

// The root search must find this repository's root — the ancestor holding
// go.mod — when the suite runs from the package directory. It asks the search
// itself: this test binary is a release build (no dev tag), where RepoRoot() is
// deliberately "" (the specification C1, C4).
func TestRepoRoot_FindsModuleRoot(t *testing.T) {
	root := findRepoRoot()
	if root == "" {
		t.Fatal("findRepoRoot() is empty; expected the module root")
	}
	if fi, err := os.Stat(filepath.Join(root, "go.mod")); err != nil || fi.IsDir() {
		t.Errorf("findRepoRoot() = %q does not contain go.mod", root)
	}
	if devBuild == false && RepoRoot() != "" {
		t.Errorf("a release build reported RepoRoot() = %q, want empty", RepoRoot())
	}
}
