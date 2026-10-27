// Package testenv isolates a test binary from the operator's own files.
//
// Sextile resolves every file it owns — the chart library, preferences, the
// session — from HOME and the XDG base directories. A test that reaches one of
// those paths without having redirected them operates on the operator's real
// data. Before an earlier change that was not a hazard but a fact: `go test ./...`
// overwrote a real session.yaml, because pressing "q" in an internal/ui test
// runs the whole quit path including store.SaveSession, and only the tests
// that remembered a per-test sandbox were safe.
//
// Run makes isolation the DEFAULT for a whole test binary instead of something
// each test must remember. Per-test sandboxes still work and still win —
// t.Setenv overrides what Run set, so nothing existing changes meaning. What
// changes is the cost of forgetting one: a stray file in a temp directory
// rather than the operator's data.
//
// It is imported only from _test.go files, and pinned by
// internal/invariants.TestStoreReachingTestPackagesIsolateByDefault.
package testenv

import (
	"fmt"
	"os"
	"path/filepath"
)

// Run executes a package's tests with HOME and the XDG base directories
// pointed at a fresh temp directory, removed when they finish. Use it as the
// whole body of TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(testenv.Run(m.Run)) }
//
// The directories are not created: Sextile's own readers must still handle an
// absent chart library and an absent preferences file, which is the state a
// first launch is in, and creating them here would hide that.
func Run(run func() int) int {
	dir, err := os.MkdirTemp("", "sextile-testenv-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "testenv: cannot isolate this test binary: %v\n", err)
		return 1
	}
	defer os.RemoveAll(dir)

	// Every variable Sextile's path resolution consults, plus the XDG bases it
	// does not read today: an unset one falls back to $HOME, which is already
	// redirected, but naming them keeps a future path from resolving out of
	// the sandbox without this file being touched.
	//
	// The two SEXTILE_*_CHARTS_DIR tiers are not optional. ChartsDir
	// gives SEXTILE_CHARTS_DIR the TOP precedence and consults
	// SEXTILE_DEV_CHARTS_DIR before the XDG default, so redirecting HOME alone
	// isolates nothing for an operator who exports either — and ./test_charts,
	// the dev library this repo's own .env names, is a real chart directory on
	// the operator's disk. The live tier is pointed inside the sandbox (that is
	// where an earlier change says isolation belongs) and the dev tier is cleared, which
	// an earlier change explicitly allows.
	//
	// SEXTILE_ATLAS_PATH is deliberately NOT redirected: it is read-only
	// reference data the chart-file translation genuinely needs, and no copy of it
	// exists inside a sandbox. Tests that want it missing set it themselves.
	for k, v := range map[string]string{
		"HOME":                   dir,
		"XDG_DATA_HOME":          filepath.Join(dir, ".local", "share"),
		"XDG_CONFIG_HOME":        filepath.Join(dir, ".config"),
		"XDG_STATE_HOME":         filepath.Join(dir, ".local", "state"),
		"XDG_CACHE_HOME":         filepath.Join(dir, ".cache"),
		"SEXTILE_CHARTS_DIR":     filepath.Join(dir, ".local", "share", "sextile", "charts"),
		"SEXTILE_DEV_CHARTS_DIR": "",
		// The launcher's contract: with SEXTILE_LAUNCH set, the host path
		// decides which folder's backups and session a run uses. A test
		// started from a launched shell must not key on the operator's own
		// charts folder.
		"SEXTILE_LAUNCH":             "",
		"SEXTILE_CHARTS_HOST_PATH":   "",
		"SEXTILE_BACKUP_HOST_PATH":   "",
		"SEXTILE_SESSIONS_HOST_PATH": "",
	} {
		if err := os.Setenv(k, v); err != nil {
			fmt.Fprintf(os.Stderr, "testenv: cannot set %s: %v\n", k, err)
			return 1
		}
	}
	return run()
}
