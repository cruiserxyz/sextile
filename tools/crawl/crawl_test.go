package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The rules here are synthetic on purpose: the words the real rules deny
// must not appear in this repository, this file included.
const synthRules = `# synthetic rules for the crawl's own tests
deny \bzorblax\b
deny-ci quuxmark
strip ok@example\.invalid
deny [a-z]+@example\.invalid
trailer Made-With
author Tester <t@example.invalid>
exec scripts/
`

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// fixture writes the rules file (plus any extra lines), the pin inside the
// tree, and one clean file. It returns the tree and the env lookup.
func fixture(t *testing.T, extraRules string) (dir string, getenv func(string) string) {
	t.Helper()
	root := t.TempDir()
	dir = filepath.Join(root, "tree")
	rulesPath := filepath.Join(root, "rules.txt")
	rules := []byte(synthRules + extraRules)
	if err := os.WriteFile(rulesPath, rules, 0o644); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, pinPath), sha(rules)+"\n")
	mustWrite(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")
	getenv = func(k string) string {
		if k == envRules {
			return rulesPath
		}
		return ""
	}
	return dir, getenv
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func crawl(t *testing.T, getenv func(string) string, args ...string) (code int, out string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code = run(args, &stdout, &stderr, getenv)
	return code, stdout.String() + stderr.String()
}

func TestCleanTreePasses(t *testing.T) {
	dir, env := fixture(t, "")
	code, out := crawl(t, env, "tree", dir)
	if code != 0 {
		t.Fatalf("clean tree: exit %d\n%s", code, out)
	}
}

func TestTreeFindings(t *testing.T) {
	cases := []struct {
		name  string
		plant func(t *testing.T, dir string)
		want  string
	}{
		{"content", func(t *testing.T, dir string) {
			mustWrite(t, filepath.Join(dir, "a.go"), "// made by zorblax\n")
		}, "a.go:1: content matches deny"},
		{"content case-insensitive", func(t *testing.T, dir string) {
			mustWrite(t, filepath.Join(dir, "b.txt"), "QUUXMARK was here\n")
		}, "b.txt:1: content matches deny"},
		{"path", func(t *testing.T, dir string) {
			mustWrite(t, filepath.Join(dir, "notes", "zorblax.md"), "fine\n")
		}, "notes/zorblax.md: path matches deny"},
		{"directory name", func(t *testing.T, dir string) {
			mustWrite(t, filepath.Join(dir, "zorblax", "x.md"), "fine\n")
		}, "zorblax/: path matches deny"},
		{"symlink target", func(t *testing.T, dir string) {
			if err := os.Symlink("/elsewhere/zorblax/file", filepath.Join(dir, "link")); err != nil {
				t.Fatal(err)
			}
		}, "link: symlink target"},
		{"executable outside scripts/", func(t *testing.T, dir string) {
			p := filepath.Join(dir, "run.sh")
			mustWrite(t, p, "#!/bin/sh\n")
			if err := os.Chmod(p, 0o755); err != nil {
				t.Fatal(err)
			}
		}, "run.sh: executable outside the allowed prefixes"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir, env := fixture(t, "")
			c.plant(t, dir)
			code, out := crawl(t, env, "tree", dir)
			if code != 1 || !strings.Contains(out, c.want) {
				t.Fatalf("want exit 1 with %q, got exit %d\n%s", c.want, code, out)
			}
		})
	}
}

func TestStripKeepsTheOneAllowedForm(t *testing.T) {
	dir, env := fixture(t, "")
	mustWrite(t, filepath.Join(dir, "c.txt"), "write to ok@example.invalid\n")
	if code, out := crawl(t, env, "tree", dir); code != 0 {
		t.Fatalf("the allowed form must pass: exit %d\n%s", code, out)
	}
	mustWrite(t, filepath.Join(dir, "c.txt"), "write to other@example.invalid\n")
	if code, out := crawl(t, env, "tree", dir); code != 1 || !strings.Contains(out, "c.txt:1: content matches deny") {
		t.Fatalf("any other form must fail: exit %d\n%s", code, out)
	}
}

func TestExecutableUnderScriptsIsAllowed(t *testing.T) {
	dir, env := fixture(t, "")
	p := filepath.Join(dir, "scripts", "ok")
	mustWrite(t, p, "#!/bin/sh\n")
	if err := os.Chmod(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if code, out := crawl(t, env, "tree", dir); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

func TestPinnedFile(t *testing.T) {
	good := "two\nlines\n"
	dir, env := fixture(t, "allow PINNED.txt "+sha([]byte(good))+"\n")
	// The pin in the tree must match the rules file WITH the extra line.
	mustWrite(t, filepath.Join(dir, "PINNED.txt"), good)
	if code, out := crawl(t, env, "tree", dir); code != 0 {
		t.Fatalf("pinned file with the allowed content: exit %d\n%s", code, out)
	}
	// A pinned file is not scanned: a deny word inside it is covered by its hash...
	mustWrite(t, filepath.Join(dir, "PINNED.txt"), good+"zorblax\n")
	code, out := crawl(t, env, "tree", dir)
	if code != 1 || !strings.Contains(out, "PINNED.txt: pinned file has hash") {
		t.Fatalf("changed pinned file: want exit 1 with the hash finding, got %d\n%s", code, out)
	}
}

func TestRefusesToRunWithoutRulesOrPin(t *testing.T) {
	dir, env := fixture(t, "")
	if code, out := crawl(t, env, "tree", dir); code != 0 {
		t.Fatalf("precondition: %d\n%s", code, out)
	}
	t.Run("no env", func(t *testing.T) {
		code, out := crawl(t, func(string) string { return "" }, "tree", dir)
		if code != 2 || !strings.Contains(out, "is not set") {
			t.Fatalf("want exit 2, got %d\n%s", code, out)
		}
	})
	t.Run("rules file missing", func(t *testing.T) {
		code, _ := crawl(t, func(string) string { return filepath.Join(dir, "nope.txt") }, "tree", dir)
		if code != 2 {
			t.Fatalf("want exit 2, got %d", code)
		}
	})
	t.Run("pin mismatch", func(t *testing.T) {
		mustWrite(t, filepath.Join(dir, pinPath), strings.Repeat("0", 64)+"\n")
		code, out := crawl(t, env, "tree", dir)
		if code != 2 || !strings.Contains(out, "does not match the pin") {
			t.Fatalf("want exit 2 with the pin message, got %d\n%s", code, out)
		}
	})
	t.Run("pin missing", func(t *testing.T) {
		if err := os.Remove(filepath.Join(dir, pinPath)); err != nil {
			t.Fatal(err)
		}
		if code, _ := crawl(t, env, "tree", dir); code != 2 {
			t.Fatalf("want exit 2, got %d", code)
		}
	})
	t.Run("rules without a deny line", func(t *testing.T) {
		root := t.TempDir()
		p := filepath.Join(root, "r.txt")
		mustWrite(t, p, "author Tester <t@example.invalid>\n")
		code, out := crawl(t, func(string) string { return p }, "tree", dir)
		if code != 2 || !strings.Contains(out, "no deny rule") {
			t.Fatalf("want exit 2, got %d\n%s", code, out)
		}
	})
}

// --- history ------------------------------------------------------------------

type repo struct {
	t   *testing.T
	dir string
}

func (r repo) git(env []string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", r.dir}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func asTester(name, email string) []string {
	return []string{
		"GIT_AUTHOR_NAME=" + name, "GIT_AUTHOR_EMAIL=" + email,
		"GIT_COMMITTER_NAME=" + name, "GIT_COMMITTER_EMAIL=" + email,
	}
}

// historyFixture makes a git repository in the crawl tree with one clean
// commit, authored by the required identity.
func historyFixture(t *testing.T) (repo, func(string) string) {
	t.Helper()
	dir, env := fixture(t, "")
	r := repo{t, dir}
	r.git(nil, "init", "-q", "-b", "main")
	r.git(nil, "config", "commit.gpgsign", "false")
	r.commitWith("init: first tree\n\nWhy this exists.\n", asTester("Tester", "t@example.invalid"))
	return r, env
}

// commitWith commits with the exact message text from a file.
func (r repo) commitWith(message string, env []string) {
	r.t.Helper()
	p := filepath.Join(r.t.TempDir(), "msg")
	if err := os.WriteFile(p, []byte(message), 0o644); err != nil {
		r.t.Fatal(err)
	}
	r.git(nil, "add", "-A")
	r.git(env, "commit", "-q", "--allow-empty", "-F", p, "--cleanup=verbatim")
}

func TestHistoryCleanPasses(t *testing.T) {
	r, env := historyFixture(t)
	mustWrite(t, filepath.Join(r.dir, "b.go"), "package main\n")
	r.commitWith("area: add b\n\nBecause the test needs two commits.\n", asTester("Tester", "t@example.invalid"))
	code, out := crawl(t, env, "history", "HEAD", r.dir)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

func TestHistoryFindings(t *testing.T) {
	ok := asTester("Tester", "t@example.invalid")
	cases := []struct {
		name string
		do   func(t *testing.T, r repo)
		want string
	}{
		{"subject too long", func(t *testing.T, r repo) {
			r.commitWith("area: "+strings.Repeat("x", 55)+"\n\nwhy\n", ok)
		}, "subject is 61 characters"},
		{"subject without area", func(t *testing.T, r repo) {
			r.commitWith("Add things\n\nwhy\n", ok)
		}, "is not `area: imperative subject`"},
		{"subject ends with a period", func(t *testing.T, r repo) {
			r.commitWith("area: add things.\n\nwhy\n", ok)
		}, "ends with a period"},
		{"no body", func(t *testing.T, r repo) {
			r.commitWith("area: add things\n", ok)
		}, "no blank line and body"},
		{"empty body", func(t *testing.T, r repo) {
			r.commitWith("area: add things\n\n   \n", ok)
		}, "the body is empty"},
		{"trailer", func(t *testing.T, r repo) {
			r.commitWith("area: add things\n\nwhy\n\nMade-With: a tool\n", ok)
		}, "is a \"made-with\" trailer"},
		{"wrong author", func(t *testing.T, r repo) {
			r.commitWith("area: add things\n\nwhy\n", asTester("Someone", "s@example.invalid"))
		}, "author is \"Someone <s@example.invalid>\""},
		{"deny word in message", func(t *testing.T, r repo) {
			r.commitWith("area: add things\n\nzorblax told me to.\n", ok)
		}, "message line 3 matches deny"},
		{"mark in an earlier tree, gone from the head", func(t *testing.T, r repo) {
			mustWrite(t, filepath.Join(r.dir, "leak.txt"), "zorblax\n")
			r.commitWith("area: add a file\n\nwhy\n", ok)
			if err := os.Remove(filepath.Join(r.dir, "leak.txt")); err != nil {
				t.Fatal(err)
			}
			r.commitWith("area: remove the file\n\nwhy\n", ok)
		}, "leak.txt:1: content matches deny"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, env := historyFixture(t)
			c.do(t, r)
			code, out := crawl(t, env, "history", "HEAD", r.dir)
			if code != 1 || !strings.Contains(out, c.want) {
				t.Fatalf("want exit 1 with %q, got exit %d\n%s", c.want, code, out)
			}
			if c.name == "mark in an earlier tree, gone from the head" {
				// The working tree itself is clean: only the history check sees it.
				if code, out := crawl(t, env, "tree", r.dir); code != 0 {
					t.Fatalf("the head tree should be clean: %d\n%s", code, out)
				}
			}
		})
	}
}

func TestHistoryRefusesWithoutPinInTheTree(t *testing.T) {
	r, env := historyFixture(t)
	r.git(nil, "rm", "-q", pinPath)
	r.commitWith("area: drop the pin\n\nwhy\n", asTester("Tester", "t@example.invalid"))
	code, out := crawl(t, env, "history", "HEAD", r.dir)
	if code != 2 || !strings.Contains(out, "cannot read the rules pin") {
		t.Fatalf("want exit 2, got %d\n%s", code, out)
	}
}

// An exec rule without a trailing slash names one path, not a prefix.
func TestExecRuleWithoutSlashMatchesTheExactPath(t *testing.T) {
	plant := func(dir, rel string) {
		p := filepath.Join(dir, rel)
		mustWrite(t, p, "#!/bin/sh\n")
		if err := os.Chmod(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	dir, env := fixture(t, "exec sextile\n")
	plant(dir, "sextile")
	plant(dir, "scripts/foo")
	if code, out := crawl(t, env, "tree", dir); code != 0 {
		t.Fatalf("sextile and scripts/foo are allowed: exit %d\n%s", code, out)
	}
	plant(dir, "sextile-x")
	code, out := crawl(t, env, "tree", dir)
	if code != 1 || !strings.Contains(out, "sextile-x: executable outside the allowed prefixes") {
		t.Fatalf("sextile-x must be refused: exit %d\n%s", code, out)
	}
	if strings.Contains(out, "\nsextile: ") || strings.HasPrefix(out, "sextile: ") {
		t.Fatalf("sextile itself stays allowed:\n%s", out)
	}
}

// --- messages and the full DAG -------------------------------------------------

func (r repo) head() string {
	r.t.Helper()
	return strings.TrimSpace(r.git(nil, "rev-parse", "HEAD"))
}

// messagesFixture is a history fixture plus the base the range starts from.
func messagesFixture(t *testing.T) (r repo, env func(string) string, base string) {
	t.Helper()
	r, env = historyFixture(t)
	return r, env, r.head()
}

func TestMessagesRejectsTrailer(t *testing.T) {
	r, env, base := messagesFixture(t)
	r.commitWith("area: add things\n\nwhy\nMade-With: a tool\n", asTester("Tester", "t@example.invalid"))
	code, out := crawl(t, env, "messages", "HEAD", base, r.dir)
	if code != 1 || !strings.Contains(out, "is a \"made-with\" trailer") {
		t.Fatalf("want exit 1 with the trailer finding, got %d\n%s", code, out)
	}
}

func TestMessagesRejectsWrongAuthorAndCommitter(t *testing.T) {
	r, env, base := messagesFixture(t)
	good := asTester("Tester", "t@example.invalid")
	// author wrong, committer right
	r.commitWith("area: add one\n\nwhy\n", append(append([]string{}, good...),
		"GIT_AUTHOR_NAME=Someone", "GIT_AUTHOR_EMAIL=s@example.invalid"))
	code, out := crawl(t, env, "messages", "HEAD", base, r.dir)
	if code != 1 || !strings.Contains(out, "author is \"Someone <s@example.invalid>\"") || strings.Contains(out, "committer is") {
		t.Fatalf("want only the author finding, got %d\n%s", code, out)
	}
	// committer wrong, author right
	r2, env2, base2 := messagesFixture(t)
	r2.commitWith("area: add one\n\nwhy\n", append(append([]string{}, good...),
		"GIT_COMMITTER_NAME=Someone", "GIT_COMMITTER_EMAIL=s@example.invalid"))
	code, out = crawl(t, env2, "messages", "HEAD", base2, r2.dir)
	if code != 1 || !strings.Contains(out, "committer is \"Someone <s@example.invalid>\"") || strings.Contains(out, "author is") {
		t.Fatalf("want only the committer finding, got %d\n%s", code, out)
	}
}

// sideMerge leaves HEAD at a no-ff merge whose side parent carries a bad
// message and whose first parent and merge message are clean.
func sideMerge(t *testing.T, r repo) {
	t.Helper()
	ok := asTester("Tester", "t@example.invalid")
	r.git(nil, "checkout", "-q", "-b", "side")
	r.commitWith("Side work without an area\n\nwhy\n", ok)
	r.git(nil, "checkout", "-q", "main")
	r.commitWith("area: main moves on\n\nwhy\n", ok)
	p := filepath.Join(t.TempDir(), "msg")
	if err := os.WriteFile(p, []byte("area: merge the side\n\nwhy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.git(ok, "merge", "-q", "--no-ff", "-F", p, "--cleanup=verbatim", "side")
}

func TestMessagesIncludesMergedSideParent(t *testing.T) {
	r, env, base := messagesFixture(t)
	sideMerge(t, r)
	code, out := crawl(t, env, "messages", "HEAD", base, r.dir)
	if code != 1 || !strings.Contains(out, "Side work without an area") {
		t.Fatalf("the merged side parent's message must be found, got %d\n%s", code, out)
	}
}

func TestMessagesRefusesMissingOrNonAncestorBase(t *testing.T) {
	ok := asTester("Tester", "t@example.invalid")
	r, env, base := messagesFixture(t)
	r.commitWith("area: add one\n\nwhy\n", ok)
	tip := r.head()
	// a sibling of tip: same parent, not an ancestor of it
	r.git(nil, "checkout", "-q", "-b", "other", base)
	r.commitWith("area: add two\n\nwhy\n", ok)
	sibling := r.head()
	cases := []struct {
		name, rev, base, want string
	}{
		{"missing base", tip, "no-such-ref", "no-such-ref is not a commit"},
		{"missing rev", "no-such-ref", base, "no-such-ref is not a commit"},
		{"non-ancestor base", tip, sibling, "is not an ancestor of"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, out := crawl(t, env, "messages", c.rev, c.base, r.dir)
			if code == 0 || !strings.Contains(out, c.want) {
				t.Fatalf("want a refusal containing %q, got exit %d\n%s", c.want, code, out)
			}
		})
	}
	t.Run("empty range passes", func(t *testing.T) {
		if code, out := crawl(t, env, "messages", tip, tip, r.dir); code != 0 {
			t.Fatalf("exit %d\n%s", code, out)
		}
	})
	t.Run("pin comes from REV", func(t *testing.T) {
		r.git(nil, "checkout", "-q", "-b", "nopin", tip)
		r.git(nil, "rm", "-q", pinPath)
		r.commitWith("area: drop the pin\n\nwhy\n", ok)
		code, out := crawl(t, env, "messages", "HEAD", tip, r.dir)
		if code != 2 || !strings.Contains(out, "cannot read the rules pin") {
			t.Fatalf("want exit 2, got %d\n%s", code, out)
		}
	})
}
