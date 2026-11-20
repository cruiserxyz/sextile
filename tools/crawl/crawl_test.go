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

// A pinned file may have had several contents over its history: each `allow`
// line for the same path adds one allowed hash, so a release that crawls every
// commit's tree accepts the file as it was in each of them. Any other content is
// still refused, and the finding names every allowed hash.
func TestPinnedFileWithSeveralAllowedContents(t *testing.T) {
	// Both allowed contents carry a denied word: a pinned file is compared by
	// hash and never scanned, which is what lets a name stand in LICENSE alone.
	older, newer := "holder one zorblax\n", "holder two zorblax\n"
	dir, env := fixture(t, "allow PINNED.txt "+sha([]byte(older))+"\nallow PINNED.txt "+sha([]byte(newer))+"\n")
	for _, content := range []string{older, newer} {
		mustWrite(t, filepath.Join(dir, "PINNED.txt"), content)
		if code, out := crawl(t, env, "tree", dir); code != 0 {
			t.Fatalf("pinned file with allowed content %q: exit %d\n%s", content, code, out)
		}
	}
	mustWrite(t, filepath.Join(dir, "PINNED.txt"), "holder three\n")
	code, out := crawl(t, env, "tree", dir)
	if code != 1 || !strings.Contains(out, sha([]byte(older))[:12]) || !strings.Contains(out, sha([]byte(newer))[:12]) {
		t.Fatalf("a third content: want exit 1 naming both allowed hashes, got %d\n%s", code, out)
	}
}

// A release crawls the whole tree of every commit in its history: a pinned
// file that changed between two releases passes in both commits when each of
// its contents has its `allow` line, and a version no line allows is refused in
// whichever commit holds it, the newest one allowed again included.
func TestHistoryAcceptsEveryAllowedVersionOfAPinnedFile(t *testing.T) {
	older, newer := "holder one zorblax\n", "holder two zorblax\n"
	dir, env := fixture(t, "allow PINNED.txt "+sha([]byte(older))+"\nallow PINNED.txt "+sha([]byte(newer))+"\n")
	r := repo{t, dir}
	r.git(nil, "init", "-q", "-b", "main")
	r.git(nil, "config", "commit.gpgsign", "false")
	mustWrite(t, filepath.Join(dir, "PINNED.txt"), older)
	r.commitWith("init: first tree\n\nWhy this exists.\n", asTester("Tester", "t@example.invalid"))
	mustWrite(t, filepath.Join(dir, "PINNED.txt"), newer)
	r.commitWith("area: the second holder\n\nBecause the file changed.\n", asTester("Tester", "t@example.invalid"))
	if code, out := crawl(t, env, "history", "HEAD", dir); code != 0 {
		t.Fatalf("two allowed versions in history: exit %d\n%s", code, out)
	}
	mustWrite(t, filepath.Join(dir, "PINNED.txt"), "holder three\n")
	r.commitWith("area: a third holder\n\nBecause the test needs one.\n", asTester("Tester", "t@example.invalid"))
	bad := strings.TrimSpace(r.git(nil, "rev-parse", "--short=12", "HEAD"))
	mustWrite(t, filepath.Join(dir, "PINNED.txt"), newer)
	r.commitWith("area: the second holder again\n\nBecause the test needs it.\n", asTester("Tester", "t@example.invalid"))
	code, out := crawl(t, env, "history", "HEAD", dir)
	if code != 1 || !strings.Contains(out, "commit "+bad+": PINNED.txt: pinned file has hash") {
		t.Fatalf("a refused version in the middle of history: want exit 1 naming commit %s, got %d\n%s", bad, code, out)
	}
}

// The history scan reuses what it found for content it has seen, keyed by the
// file's mode too: the same content once executable outside the allowed
// prefixes is refused in that commit although the newest commit has it plain.
func TestHistoryRemembersTheModeOfWhatItScanned(t *testing.T) {
	r, env := historyFixture(t)
	mustWrite(t, filepath.Join(r.dir, "notes.txt"), "plain words\n")
	if err := os.Chmod(filepath.Join(r.dir, "notes.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	r.commitWith("area: notes, executable\n\nBecause the test needs it.\n", asTester("Tester", "t@example.invalid"))
	bad := strings.TrimSpace(r.git(nil, "rev-parse", "--short=12", "HEAD"))
	if err := os.Chmod(filepath.Join(r.dir, "notes.txt"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.commitWith("area: notes, plain\n\nBecause the test needs it.\n", asTester("Tester", "t@example.invalid"))
	code, out := crawl(t, env, "history", "HEAD", r.dir)
	if code != 1 || !strings.Contains(out, "commit "+bad+": notes.txt: executable outside the allowed prefixes") {
		t.Fatalf("the executable notes in an older commit: want exit 1 naming commit %s, got %d\n%s", bad, code, out)
	}
}

// A pinned path may be absent, in the tree and in any commit of history: one
// pinned file is retired and another is new, and an absent file leaks nothing.
// A link or a directory in its place is refused, because nothing would be
// compared with its allowed hashes.
func TestPinnedFileMayBeAbsentButNeverALinkOrDirectory(t *testing.T) {
	good := "two\nlines\n"
	extra := "allow PINNED.txt " + sha([]byte(good)) + "\n"
	dir, env := fixture(t, extra)
	if code, out := crawl(t, env, "tree", dir); code != 0 {
		t.Fatalf("absent pinned file: want exit 0, got %d\n%s", code, out)
	}
	mustWrite(t, filepath.Join(dir, "OTHER.txt"), good)
	if err := os.Symlink("OTHER.txt", filepath.Join(dir, "PINNED.txt")); err != nil {
		t.Fatal(err)
	}
	if code, out := crawl(t, env, "tree", dir); code != 1 || !strings.Contains(out, "PINNED.txt: pinned file is a symlink") {
		t.Fatalf("symlink in the pinned file's place: want exit 1, got %d\n%s", code, out)
	}
	dir2, env2 := fixture(t, extra)
	mustWrite(t, filepath.Join(dir2, "PINNED.txt", "inside.txt"), good)
	if code, out := crawl(t, env2, "tree", dir2); code != 1 || !strings.Contains(out, "PINNED.txt: pinned file is a directory") {
		t.Fatalf("directory in the pinned file's place: want exit 1, got %d\n%s", code, out)
	}

	// History: the file arrives in the second commit and leaves in the third;
	// every commit is clean. A directory in its place in a fourth is refused.
	dir3, env3 := fixture(t, extra)
	r := repo{t, dir3}
	r.git(nil, "init", "-q", "-b", "main")
	r.git(nil, "config", "commit.gpgsign", "false")
	ok := asTester("Tester", "t@example.invalid")
	r.commitWith("init: first tree\n\nWhy this exists.\n", ok)
	mustWrite(t, filepath.Join(dir3, "PINNED.txt"), good)
	r.commitWith("area: the pinned file arrives\n\nBecause the test needs it.\n", ok)
	r.git(nil, "rm", "-q", "PINNED.txt")
	r.commitWith("area: the pinned file leaves\n\nBecause the test needs it.\n", ok)
	if code, out := crawl(t, env3, "history", "HEAD", dir3); code != 0 {
		t.Fatalf("history with the pinned file absent before and after: want exit 0, got %d\n%s", code, out)
	}
	mustWrite(t, filepath.Join(dir3, "PINNED.txt", "inside.txt"), good)
	r.commitWith("area: a directory in its place\n\nBecause the test needs it.\n", ok)
	bad := strings.TrimSpace(r.git(nil, "rev-parse", "--short=12", "HEAD"))
	if code, out := crawl(t, env3, "history", "HEAD", dir3); code != 1 || !strings.Contains(out, "commit "+bad+": PINNED.txt: pinned file is a directory") {
		t.Fatalf("history with a directory in the pinned file's place: want exit 1 naming %s, got %d\n%s", bad, code, out)
	}

	// An EMPTY directory in its place: git keeps no file under it, so only the tree entry shows it.
	dir4, env4 := fixture(t, extra)
	r4 := repo{t, dir4}
	r4.git(nil, "init", "-q", "-b", "main")
	r4.git(nil, "config", "commit.gpgsign", "false")
	r4.commitWith("init: first tree\n\nWhy this exists.\n", ok)
	withEmptyTree(t, r4, "PINNED.txt", "area: an empty directory in its place\n\nBecause the test needs it.\n", ok)
	if code, out := crawl(t, env4, "history", "HEAD", dir4); code != 1 || !strings.Contains(out, "PINNED.txt: pinned file is a directory") {
		t.Fatalf("history with an empty directory in the pinned file's place: want exit 1, got %d\n%s", code, out)
	}
}

// withEmptyTree commits HEAD's tree plus an empty directory at name (git
// stores it as a tree entry with no file under it), and moves main to it.
func withEmptyTree(t *testing.T, r repo, name, message string, env []string) {
	t.Helper()
	empty := strings.TrimSpace(r.git(nil, "hash-object", "-t", "tree", "-w", "--stdin"))
	listing := r.git(nil, "ls-tree", "HEAD") + "040000 tree " + empty + "\t" + name + "\n"
	cmd := exec.Command("git", "-C", r.dir, "mktree")
	cmd.Stdin = strings.NewReader(listing)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("mktree: %v", err)
	}
	p := filepath.Join(t.TempDir(), "msg")
	if err := os.WriteFile(p, []byte(message), 0o644); err != nil {
		t.Fatal(err)
	}
	c := strings.TrimSpace(r.git(env, "commit-tree", strings.TrimSpace(string(out)), "-p", "HEAD", "-F", p))
	r.git(nil, "update-ref", "refs/heads/main", c)
}

// A denied word as the name of an empty directory is found in history too.
func TestHistoryFindsADeniedEmptyDirectory(t *testing.T) {
	r, env := historyFixture(t)
	withEmptyTree(t, r, "zorblax", "area: an empty directory\n\nBecause the test needs it.\n", asTester("Tester", "t@example.invalid"))
	if code, out := crawl(t, env, "history", "HEAD", r.dir); code != 1 || !strings.Contains(out, "zorblax/: path matches deny") {
		t.Fatalf("a denied empty directory: want exit 1, got %d\n%s", code, out)
	}
}

// --- the changelog author -------------------------------------------------------

const scribeRule = "changelog-author Scribe <s@example.invalid>\n"

// changelogFixture is a history with a CHANGELOG.md, under rules that name the
// changelog author. It returns the commit the next commit builds on.
func changelogFixture(t *testing.T, extra string) (repo, func(string) string, string) {
	t.Helper()
	dir, env := fixture(t, extra)
	r := repo{t, dir}
	r.git(nil, "init", "-q", "-b", "main")
	r.git(nil, "config", "commit.gpgsign", "false")
	mustWrite(t, filepath.Join(dir, "CHANGELOG.md"), "# Changes\n")
	r.commitWith("init: first tree\n\nWhy this exists.\n", asTester("Tester", "t@example.invalid"))
	return r, env, r.head()
}

// asScribe authors as the changelog author and commits as the author.
func asScribe() []string {
	return []string{
		"GIT_AUTHOR_NAME=Scribe", "GIT_AUTHOR_EMAIL=s@example.invalid",
		"GIT_COMMITTER_NAME=Tester", "GIT_COMMITTER_EMAIL=t@example.invalid",
	}
}

func TestChangelogAuthorMayChangeOnlyTheChangelog(t *testing.T) {
	r, env, base := changelogFixture(t, scribeRule)
	mustWrite(t, filepath.Join(r.dir, "CHANGELOG.md"), "# Changes\n\n## 1.0\n")
	r.commitWith("changelog: 1.0\n\nThe section of the release.\n", asScribe())
	if code, out := crawl(t, env, "history", "HEAD", r.dir); code != 0 {
		t.Fatalf("history: the changelog author changing only CHANGELOG.md: want exit 0, got %d\n%s", code, out)
	}
	if code, out := crawl(t, env, "messages", "HEAD", base, r.dir); code != 0 {
		t.Fatalf("messages: the changelog author changing only CHANGELOG.md: want exit 0, got %d\n%s", code, out)
	}
}

func TestChangelogAuthorRefusedOtherwise(t *testing.T) {
	const want = "author is \"Scribe <s@example.invalid>\""
	cases := []struct {
		name  string
		rules string
		do    func(t *testing.T, r repo)
	}{
		{"rules name no changelog author", "", func(t *testing.T, r repo) {
			mustWrite(t, filepath.Join(r.dir, "CHANGELOG.md"), "# Changes\n\n## 1.0\n")
			r.commitWith("changelog: 1.0\n\nwhy\n", asScribe())
		}},
		{"another file changes too", scribeRule, func(t *testing.T, r repo) {
			mustWrite(t, filepath.Join(r.dir, "CHANGELOG.md"), "# Changes\n\n## 1.0\n")
			mustWrite(t, filepath.Join(r.dir, "b.go"), "package main\n")
			r.commitWith("changelog: 1.0\n\nwhy\n", asScribe())
		}},
		{"only another file changes", scribeRule, func(t *testing.T, r repo) {
			mustWrite(t, filepath.Join(r.dir, "b.go"), "package main\n")
			r.commitWith("area: add b\n\nwhy\n", asScribe())
		}},
		{"nothing changes", scribeRule, func(t *testing.T, r repo) {
			r.commitWith("changelog: nothing\n\nwhy\n", asScribe())
		}},
		{"the changelog is deleted", scribeRule, func(t *testing.T, r repo) {
			r.git(nil, "rm", "-q", "CHANGELOG.md")
			r.commitWith("changelog: gone\n\nwhy\n", asScribe())
		}},
		{"the changelog's mode changes", scribeRule, func(t *testing.T, r repo) {
			if err := os.Chmod(filepath.Join(r.dir, "CHANGELOG.md"), 0o755); err != nil {
				t.Fatal(err)
			}
			r.commitWith("changelog: mode\n\nwhy\n", asScribe())
		}},
		{"an empty directory comes too", scribeRule, func(t *testing.T, r repo) {
			mustWrite(t, filepath.Join(r.dir, "CHANGELOG.md"), "# Changes\n\n## 1.0\n")
			r.git(nil, "add", "CHANGELOG.md")
			tree := strings.TrimSpace(r.git(nil, "write-tree")) // HEAD's tree with the new changelog
			empty := strings.TrimSpace(r.git(nil, "hash-object", "-t", "tree", "-w", "--stdin"))
			listing := r.git(nil, "ls-tree", tree) + "040000 tree " + empty + "\tquiet\n"
			cmd := exec.Command("git", "-C", r.dir, "mktree")
			cmd.Stdin = strings.NewReader(listing)
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("mktree: %v", err)
			}
			c := strings.TrimSpace(r.git(asScribe(), "commit-tree", strings.TrimSpace(string(out)), "-p", "HEAD", "-m", "changelog: 1.0", "-m", "why"))
			r.git(nil, "update-ref", "refs/heads/main", c)
		}},
		{"the changelog becomes a link", scribeRule, func(t *testing.T, r repo) {
			if err := os.Remove(filepath.Join(r.dir, "CHANGELOG.md")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("main.go", filepath.Join(r.dir, "CHANGELOG.md")); err != nil {
				t.Fatal(err)
			}
			r.commitWith("changelog: link\n\nwhy\n", asScribe())
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, env, base := changelogFixture(t, c.rules)
			c.do(t, r)
			for _, mode := range [][]string{{"history", "HEAD", r.dir}, {"messages", "HEAD", base, r.dir}} {
				code, out := crawl(t, env, mode...)
				if code != 1 || !strings.Contains(out, want) {
					t.Fatalf("%s: want exit 1 with %q, got %d\n%s", mode[0], want, code, out)
				}
			}
		})
	}
}

// The changelog author's commit is still committed by the author.
func TestChangelogAuthorStillNeedsTheCommitter(t *testing.T) {
	r, env, _ := changelogFixture(t, scribeRule)
	mustWrite(t, filepath.Join(r.dir, "CHANGELOG.md"), "# Changes\n\n## 1.0\n")
	r.commitWith("changelog: 1.0\n\nwhy\n", asTester("Scribe", "s@example.invalid"))
	code, out := crawl(t, env, "history", "HEAD", r.dir)
	if code != 1 || !strings.Contains(out, "committer is \"Scribe <s@example.invalid>\"") || strings.Contains(out, "author is") {
		t.Fatalf("want only the committer finding, got %d\n%s", code, out)
	}
}

// A root commit and a merge have no one parent to compare with.
func TestChangelogAuthorNeedsExactlyOneParent(t *testing.T) {
	const want = "author is \"Scribe <s@example.invalid>\""
	dir, env := fixture(t, scribeRule)
	r := repo{t, dir}
	r.git(nil, "init", "-q", "-b", "main")
	r.git(nil, "config", "commit.gpgsign", "false")
	mustWrite(t, filepath.Join(dir, "CHANGELOG.md"), "# Changes\n")
	r.commitWith("changelog: first\n\nwhy\n", asScribe())
	if code, out := crawl(t, env, "history", "HEAD", dir); code != 1 || !strings.Contains(out, want) {
		t.Fatalf("root commit by the changelog author: want exit 1, got %d\n%s", code, out)
	}

	// The merge's tree differs from its first parent only in CHANGELOG.md, so
	// only the one-parent rule refuses it.
	r2, env2, base := changelogFixture(t, scribeRule)
	ok := asTester("Tester", "t@example.invalid")
	r2.git(nil, "checkout", "-q", "-b", "side")
	mustWrite(t, filepath.Join(r2.dir, "CHANGELOG.md"), "# Changes\n\n## 1.0\n")
	r2.commitWith("area: the section on a side line\n\nwhy\n", ok)
	r2.git(nil, "checkout", "-q", "main")
	p := filepath.Join(t.TempDir(), "msg")
	if err := os.WriteFile(p, []byte("changelog: merge\n\nwhy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r2.git(asScribe(), "merge", "-q", "--no-ff", "-F", p, "--cleanup=verbatim", "side")
	if code, out := crawl(t, env2, "messages", "HEAD", base, r2.dir); code != 1 || !strings.Contains(out, want) {
		t.Fatalf("merge by the changelog author: want exit 1, got %d\n%s", code, out)
	}
}

func TestChangelogAuthorLineMustBeOneIdentity(t *testing.T) {
	for name, extra := range map[string]string{
		"repeated":  scribeRule + scribeRule,
		"malformed": "changelog-author Scribe\n",
	} {
		dir, env := fixture(t, extra)
		if code, out := crawl(t, env, "tree", dir); code != 2 {
			t.Errorf("%s changelog-author: want exit 2, got %d\n%s", name, code, out)
		}
	}
}

// A malformed or repeated hash on an allow line stops the crawl: it cannot run
// on rules that would refuse the file they mean to allow.
func TestAllowLineMustBeOneNewHash(t *testing.T) {
	h := sha([]byte("x"))
	for name, extra := range map[string]string{
		"repeated":   "allow PINNED.txt " + h + "\nallow PINNED.txt " + h + "\n",
		"not hex":    "allow PINNED.txt " + strings.Repeat("z", 64) + "\n",
		"upper case": "allow PINNED.txt " + strings.ToUpper(h) + "\n",
	} {
		dir, env := fixture(t, extra)
		if code, out := crawl(t, env, "tree", dir); code != 2 {
			t.Errorf("%s hash: want exit 2, got %d\n%s", name, code, out)
		}
	}
}
