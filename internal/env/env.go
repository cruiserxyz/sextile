// Package env loads Sextile's .env and resolves repo-root-relative paths, in
// development builds only (-tags dev; scripts/run.sh passes it). A release
// build does nothing here: Load is a no-op, RepoRoot is "" and ResolveDir
// returns a relative path unchanged (the specification C1, C4).
//
// A .env at the repository root — the nearest ancestor of the working directory
// that holds go.mod — seeds process-environment defaults, in a -tags dev build
// only (the binary scripts/run.sh builds); the shipped release binary and the
// untagged test suite never read it. Real exported environment variables always win:
// Load only sets a key the process does not already define. Relative directory
// values (e.g. SEXTILE_DEV_CHARTS_DIR=./test_charts) resolve against the repo
// root via ResolveDir, so `go test` (CWD = the package directory) and the binary
// agree on the target regardless of the working directory.
//
// The package is stdlib-only by design — go.mod stays dependency-clean — and its
// readers of what Load computes (RepoRoot, ResolveDir) call Load themselves, so
// the chart library still honours .env wherever it is reached. Merely importing
// env reads nothing, and main calls Load explicitly, before it takes the
// instance lock, so the lock and the session share one XDG_DATA_HOME. A release
// build reads nothing then; in a dev build an unreadable .env may report
// before a lock-refused screen.
package env

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	once     sync.Once
	repoRoot string
)

// Dev reports whether this is a development build (-tags dev). Only a
// development build reads .env, resolves paths against the repo root, or honours
// SEXTILE_DEV_CHARTS_DIR.
func Dev() bool { return devBuild }

// Load resolves the repo root and applies the root's .env to the process
// environment, exactly once. Idempotent and safe from both main and tests.
func Load() { once.Do(load) }

func load() {
	if !devBuild {
		return // a released binary never reads a .env it happens to find
	}
	repoRoot = findRepoRoot()
	if repoRoot == "" {
		return
	}
	path := filepath.Join(repoRoot, ".env")
	f, err := os.Open(path)
	if err != nil {
		// A missing .env is the normal case and silent. A present-but-
		// unreadable one (permissions, a directory, …) is a real
		// misconfiguration: surface it, but never fatally — the process
		// continues on the real environment plus defaults.
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "sextile: env: cannot read %s: %v\n", path, err)
		}
		return
	}
	defer f.Close()
	kv, err := parseEnv(f)
	if err != nil {
		// Surface a truncated/oversized .env (e.g. a line past bufio's limit)
		// rather than silently applying a partial environment. Whatever parsed
		// cleanly before the error is still applied.
		fmt.Fprintf(os.Stderr, "sextile: env: reading %s: %v\n", path, err)
	}
	applyEnv(kv)
}

// RepoRoot returns the directory .env was resolved against (the go.mod root),
// or "" when none was found (e.g. CWD outside the repo). Triggers Load.
func RepoRoot() string {
	Load()
	return repoRoot
}

// ResolveDir resolves a possibly repo-root-relative directory to an absolute
// path. Empty and already-absolute paths pass through unchanged; a relative
// path joins onto RepoRoot(). When the root is unknown the path is returned as
// given (best effort rather than a hard failure).
func ResolveDir(path string) string {
	return resolveDirIn(RepoRoot(), path)
}

func resolveDirIn(root, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	if root != "" {
		return filepath.Join(root, path)
	}
	return path
}

// findRepoRoot walks up from the working directory to the first ancestor that
// holds a go.mod file, returning "" if none exists.
func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if fi, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !fi.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// parseEnv reads KEY=VALUE lines, skipping blank lines, #-comment lines, and an
// optional leading `export `. Surrounding single or double quotes on the value
// are stripped; otherwise the value is taken literally (no escape processing,
// no inline-comment stripping — values may contain '#'). Later duplicate keys
// win within the file. A scanner error (e.g. a line longer than bufio's token
// limit) is returned alongside whatever parsed before it, so a truncated or
// oversized .env is never silently treated as complete.
//
// Every key is applied (load-all, standard dotenv semantics) rather than
// scoped to a SEXTILE_* prefix: the .env legitimately seeds non-prefixed vars
// the app honours (e.g. NO_COLOR, XDG_DATA_HOME), and tests override whatever
// they depend on. applyEnv never overrides an already-set key, so this only
// supplies defaults.
func parseEnv(r io.Reader) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq <= 0 { // no '=', or empty key
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := unquote(strings.TrimSpace(line[eq+1:]))
		out[key] = val
	}
	return out, sc.Err()
}

// unquote strips one layer of matched surrounding single or double quotes.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// applyEnv sets each key the process does not already define — real environment
// variables win over .env.
func applyEnv(kv map[string]string) {
	for k, v := range kv {
		if _, ok := os.LookupEnv(k); !ok {
			os.Setenv(k, v)
		}
	}
}
