// Command crawl refuses what must never enter this repository.
//
// It reads its rules from the file named by SEXTILE_CRAWL_RULES, which is
// kept outside the tree: the tree must not contain the words it forbids. The
// rules file's SHA-256 is pinned in tools/crawl/rules.sha256, so a rule
// cannot be weakened without a visible change here.
//
//	crawl tree [DIR]          every path, symlink target, file mode and line
//	                          of the working tree at DIR (default .)
//	crawl history REV [DIR]   every commit on REV's first-parent ancestry in
//	                          the git repository at DIR: message format,
//	                          author and committer, and the whole tree of
//	                          each commit
//	crawl messages REV BASE [DIR]
//	                          only the message, author and committer of every
//	                          commit in BASE..REV (full DAG); BASE must be an
//	                          ancestor of REV
//
// Exit 0 when nothing was found, 1 with findings on stdout, 2 when the crawl
// could not run (no rules file, a pin mismatch, bad arguments): a crawl that
// cannot run never passes.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	usage      = "usage: crawl tree [DIR] | crawl history REV [DIR] | crawl messages REV BASE [DIR]"
	envRules   = "SEXTILE_CRAWL_RULES"
	pinPath    = "tools/crawl/rules.sha256"
	maxSubject = 60
)

var subjectRE = regexp.MustCompile(`^[a-z][a-z0-9./-]*: \S.*$`)

type denyRule struct {
	text string
	re   *regexp.Regexp
}

type rules struct {
	hash     string
	strip    []*regexp.Regexp // removed from a line before the deny rules see it
	deny     []denyRule
	trailers []string // lower-case keys
	author   string   // "Name <email>"
	pinned   map[string]string
	execOK   []string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

func run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	rulesPath := getenv(envRules)
	if rulesPath == "" {
		fmt.Fprintf(stderr, "crawl: %s is not set; the crawl cannot run without its rules\n", envRules)
		return 2
	}
	r, err := loadRules(rulesPath)
	if err != nil {
		fmt.Fprintf(stderr, "crawl: %v\n", err)
		return 2
	}
	switch args[0] {
	case "tree":
		dir := "."
		if len(args) > 1 {
			dir = args[1]
		}
		if len(args) > 2 {
			fmt.Fprintln(stderr, "usage: crawl tree [DIR]")
			return 2
		}
		pin, err := os.ReadFile(filepath.Join(dir, pinPath))
		if err != nil {
			fmt.Fprintf(stderr, "crawl: cannot read the rules pin %s: %v\n", pinPath, err)
			return 2
		}
		if err := r.checkPin(pin); err != nil {
			fmt.Fprintf(stderr, "crawl: %v\n", err)
			return 2
		}
		findings, err := crawlTree(dir, r)
		if err != nil {
			fmt.Fprintf(stderr, "crawl: %v\n", err)
			return 2
		}
		return report(findings, stdout, "tree "+dir)
	case "history":
		if len(args) < 2 || len(args) > 3 {
			fmt.Fprintln(stderr, "usage: crawl history REV [DIR]")
			return 2
		}
		rev, dir := args[1], "."
		if len(args) == 3 {
			dir = args[2]
		}
		findings, err := crawlHistory(dir, rev, r)
		if err != nil {
			fmt.Fprintf(stderr, "crawl: %v\n", err)
			return 2
		}
		return report(findings, stdout, "history "+rev)
	case "messages":
		if len(args) < 3 || len(args) > 4 {
			fmt.Fprintln(stderr, "usage: crawl messages REV BASE [DIR]")
			return 2
		}
		rev, base, dir := args[1], args[2], "."
		if len(args) == 4 {
			dir = args[3]
		}
		findings, err := crawlMessages(dir, rev, base, r)
		if err != nil {
			fmt.Fprintf(stderr, "crawl: %v\n", err)
			return 2
		}
		return report(findings, stdout, "messages "+base+".."+rev)
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
}

func report(findings []string, stdout io.Writer, what string) int {
	for _, f := range findings {
		fmt.Fprintln(stdout, f)
	}
	if len(findings) > 0 {
		fmt.Fprintf(stdout, "crawl: %d finding(s) in %s\n", len(findings), what)
		return 1
	}
	fmt.Fprintf(stdout, "crawl: clean (%s)\n", what)
	return 0
}

// --- rules ------------------------------------------------------------------

func loadRules(path string) (*rules, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read the rules file %s: %v", path, err)
	}
	sum := sha256.Sum256(b)
	r := &rules{hash: hex.EncodeToString(sum[:]), pinned: map[string]string{}}
	sc := bufio.NewScanner(bytes.NewReader(b))
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kind, rest, _ := strings.Cut(line, " ")
		rest = strings.TrimSpace(rest)
		switch kind {
		case "deny", "deny-ci":
			expr := rest
			if kind == "deny-ci" {
				expr = "(?i)" + expr
			}
			re, err := regexp.Compile(expr)
			if err != nil {
				return nil, fmt.Errorf("rules line %d: bad pattern: %v", n, err)
			}
			r.deny = append(r.deny, denyRule{text: rest, re: re})
		case "strip":
			re, err := regexp.Compile(rest)
			if err != nil {
				return nil, fmt.Errorf("rules line %d: bad strip pattern: %v", n, err)
			}
			r.strip = append(r.strip, re)
		case "trailer":
			if rest == "" {
				return nil, fmt.Errorf("rules line %d: trailer needs a key", n)
			}
			r.trailers = append(r.trailers, strings.ToLower(rest))
		case "author":
			if r.author != "" {
				return nil, fmt.Errorf("rules line %d: a second author line", n)
			}
			if !strings.HasSuffix(rest, ">") || !strings.Contains(rest, " <") {
				return nil, fmt.Errorf("rules line %d: author must be `Name <email>`", n)
			}
			r.author = rest
		case "allow":
			p, h, ok := strings.Cut(rest, " ")
			h = strings.TrimSpace(h)
			if !ok || len(h) != 64 {
				return nil, fmt.Errorf("rules line %d: allow needs `path sha256`", n)
			}
			r.pinned[filepath.ToSlash(p)] = h
		case "exec":
			if rest == "" {
				return nil, fmt.Errorf("rules line %d: exec needs a path prefix", n)
			}
			r.execOK = append(r.execOK, rest) // "dir/" is a prefix, anything else one exact path
		default:
			return nil, fmt.Errorf("rules line %d: unknown directive %q", n, kind)
		}
	}
	if len(r.deny) == 0 {
		return nil, errors.New("the rules file has no deny rule; refusing to run with nothing to deny")
	}
	if r.author == "" {
		return nil, errors.New("the rules file names no author")
	}
	return r, nil
}

func (r *rules) checkPin(pin []byte) error {
	want := strings.TrimSpace(string(pin))
	if i := strings.IndexAny(want, " \t"); i >= 0 {
		want = want[:i]
	}
	if want != r.hash {
		return fmt.Errorf("the rules file's hash %s does not match the pin in %s (%s); a changed rules file needs a visible change here", r.hash[:12], pinPath, want)
	}
	return nil
}

// execAllowed: a rule ending in "/" is a directory prefix; any other rule
// names exactly one path ("exec check" allows check and not check-x).
func (r *rules) execAllowed(path string) bool {
	for _, p := range r.execOK {
		if path == p || (strings.HasSuffix(p, "/") && strings.HasPrefix(path, p)) {
			return true
		}
	}
	return false
}

// denyHits returns the rules a string matches, after the strip patterns
// (the one allowed form of something otherwise denied) are removed from it.
func (r *rules) denyHits(s string) []string {
	for _, re := range r.strip {
		s = re.ReplaceAllString(s, "")
	}
	var hits []string
	for _, d := range r.deny {
		if d.re.MatchString(s) {
			hits = append(hits, d.text)
		}
	}
	return hits
}

// scanContent reports every line of content that a deny rule matches.
func (r *rules) scanContent(where string, content []byte) []string {
	var out []string
	line := 0
	for len(content) > 0 {
		line++
		var l []byte
		if i := bytes.IndexByte(content, '\n'); i >= 0 {
			l, content = content[:i], content[i+1:]
		} else {
			l, content = content, nil
		}
		for _, hit := range r.denyHits(string(l)) {
			out = append(out, fmt.Sprintf("%s:%d: content matches deny %s", where, line, hit))
		}
	}
	return out
}

// checkEntry applies every rule to one tree entry: its path, its mode, its
// symlink target or its content.
func (r *rules) checkEntry(path string, isLink, isExec bool, content []byte) []string {
	var out []string
	for _, hit := range r.denyHits(path) {
		out = append(out, fmt.Sprintf("%s: path matches deny %s", path, hit))
	}
	if isLink {
		target := string(content)
		for _, hit := range r.denyHits(target) {
			out = append(out, fmt.Sprintf("%s: symlink target %q matches deny %s", path, target, hit))
		}
		return out
	}
	if isExec && !r.execAllowed(path) {
		out = append(out, fmt.Sprintf("%s: executable outside the allowed prefixes", path))
	}
	if want, ok := r.pinned[path]; ok {
		sum := sha256.Sum256(content)
		if got := hex.EncodeToString(sum[:]); got != want {
			out = append(out, fmt.Sprintf("%s: pinned file has hash %s, the rules allow only %s", path, got[:12], want[:12]))
		}
		return out
	}
	return append(out, r.scanContent(path, content)...)
}

// --- tree ---------------------------------------------------------------------

func crawlTree(dir string, r *rules) ([]string, error) {
	var findings []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if rel == ".git" || rel == ".jj" {
				return filepath.SkipDir
			}
			for _, hit := range r.denyHits(rel) {
				findings = append(findings, fmt.Sprintf("%s/: path matches deny %s", rel, hit))
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			findings = append(findings, r.checkEntry(rel, true, false, []byte(target))...)
			return nil
		}
		if !info.Mode().IsRegular() {
			findings = append(findings, fmt.Sprintf("%s: not a regular file", rel))
			return nil
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		findings = append(findings, r.checkEntry(rel, false, info.Mode()&0o111 != 0, content)...)
		return nil
	})
	return findings, err
}

// --- history ------------------------------------------------------------------

type commit struct {
	sha       string
	tree      string
	author    string
	committer string
	message   string
}

func git(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func readCommit(dir, sha string) (*commit, error) {
	raw, err := git(dir, "cat-file", "commit", sha)
	if err != nil {
		return nil, err
	}
	c := &commit{sha: sha}
	head, msg, _ := strings.Cut(string(raw), "\n\n")
	c.message = msg
	for _, line := range strings.Split(head, "\n") {
		kind, rest, _ := strings.Cut(line, " ")
		switch kind {
		case "tree":
			c.tree = rest
		case "author":
			c.author = identity(rest)
		case "committer":
			c.committer = identity(rest)
		}
	}
	return c, nil
}

// identity strips the timestamp from "Name <email> 1700000000 +0100".
func identity(s string) string {
	if i := strings.LastIndex(s, ">"); i >= 0 {
		return s[:i+1]
	}
	return s
}

// pinAt checks the rules against the pin in REV's own tree, so the rules
// applied are the ones that tree declares.
func pinAt(dir, rev string, r *rules) error {
	pin, err := git(dir, "show", rev+":"+pinPath)
	if err != nil {
		return fmt.Errorf("cannot read the rules pin %s at %s: %v", pinPath, rev, err)
	}
	return r.checkPin(pin)
}

// resolveCommit turns a revision into a full commit id, or says it is missing.
func resolveCommit(dir, rev string) (string, error) {
	out, err := git(dir, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%s is not a commit in %s", rev, dir)
	}
	return strings.TrimSpace(string(out)), nil
}

// crawlMessages checks the message, author and committer of every commit in
// BASE..REV over the full DAG. BASE must be an ancestor of REV, so a base that
// has drifted off the line cannot silently shrink the range.
func crawlMessages(dir, rev, base string, r *rules) ([]string, error) {
	revSHA, err := resolveCommit(dir, rev)
	if err != nil {
		return nil, err
	}
	baseSHA, err := resolveCommit(dir, base)
	if err != nil {
		return nil, err
	}
	if _, err := git(dir, "merge-base", "--is-ancestor", baseSHA, revSHA); err != nil {
		return nil, fmt.Errorf("%s is not an ancestor of %s; the range is not defined", base, rev)
	}
	if err := pinAt(dir, revSHA, r); err != nil {
		return nil, err
	}
	list, err := git(dir, "rev-list", revSHA, "^"+baseSHA)
	if err != nil {
		return nil, err
	}
	var findings []string
	for _, sha := range strings.Fields(string(list)) {
		c, err := readCommit(dir, sha)
		if err != nil {
			return nil, err
		}
		findings = append(findings, checkMessage(sha[:12], c, r)...)
	}
	return findings, nil
}

func crawlHistory(dir, rev string, r *rules) ([]string, error) {
	list, err := git(dir, "rev-list", "--first-parent", rev)
	if err != nil {
		return nil, err
	}
	shas := strings.Fields(string(list))
	if len(shas) == 0 {
		return nil, fmt.Errorf("%s names no commit", rev)
	}
	if err := pinAt(dir, rev, r); err != nil {
		return nil, err
	}
	var findings []string
	seen := map[string][]string{} // blob sha + path -> findings, so an unchanged file is scanned once
	for _, sha := range shas {
		c, err := readCommit(dir, sha)
		if err != nil {
			return nil, err
		}
		short := sha[:12]
		findings = append(findings, checkMessage(short, c, r)...)
		entries, err := git(dir, "ls-tree", "-r", "-z", c.tree)
		if err != nil {
			return nil, err
		}
		for _, e := range bytes.Split(entries, []byte{0}) {
			if len(e) == 0 {
				continue
			}
			// "<mode> <type> <sha>\t<path>"
			meta, path, ok := bytes.Cut(e, []byte{'\t'})
			if !ok {
				continue
			}
			f := strings.Fields(string(meta))
			if len(f) != 3 {
				continue
			}
			mode, typ, blob := f[0], f[1], f[2]
			p := string(path)
			key := blob + " " + p
			if typ != "blob" {
				findings = append(findings, fmt.Sprintf("commit %s: %s: not a blob (%s)", short, p, typ))
				continue
			}
			if cached, ok := seen[key]; ok {
				for _, s := range cached {
					findings = append(findings, "commit "+short+": "+s)
				}
				continue
			}
			content, err := git(dir, "cat-file", "blob", blob)
			if err != nil {
				return nil, err
			}
			out := r.checkEntry(p, mode == "120000", mode == "100755", content)
			seen[key] = out
			for _, s := range out {
				findings = append(findings, "commit "+short+": "+s)
			}
		}
	}
	return findings, nil
}

func checkMessage(short string, c *commit, r *rules) []string {
	var out []string
	if c.author != r.author {
		out = append(out, fmt.Sprintf("commit %s: author is %q, must be %q", short, c.author, r.author))
	}
	if c.committer != r.author {
		out = append(out, fmt.Sprintf("commit %s: committer is %q, must be %q", short, c.committer, r.author))
	}
	lines := strings.Split(strings.TrimRight(c.message, "\n"), "\n")
	subject := lines[0]
	if !subjectRE.MatchString(subject) {
		out = append(out, fmt.Sprintf("commit %s: subject %q is not `area: imperative subject`", short, subject))
	}
	if n := utf8.RuneCountInString(subject); n > maxSubject {
		out = append(out, fmt.Sprintf("commit %s: subject is %d characters, at most %d", short, n, maxSubject))
	}
	if strings.HasSuffix(subject, ".") {
		out = append(out, fmt.Sprintf("commit %s: subject ends with a period", short))
	}
	if len(lines) < 3 || strings.TrimSpace(lines[1]) != "" {
		out = append(out, fmt.Sprintf("commit %s: no blank line and body after the subject", short))
	} else {
		body := lines[2:]
		nonEmpty := false
		for i, l := range body {
			if strings.TrimSpace(l) != "" {
				nonEmpty = true
			}
			lower := strings.ToLower(l)
			for _, key := range r.trailers {
				if strings.HasPrefix(lower, key+":") {
					out = append(out, fmt.Sprintf("commit %s: body line %d is a %s trailer", short, i+3, strconv.Quote(key)))
				}
			}
		}
		if !nonEmpty {
			out = append(out, fmt.Sprintf("commit %s: the body is empty", short))
		}
	}
	for i, l := range lines {
		for _, hit := range r.denyHits(l) {
			out = append(out, fmt.Sprintf("commit %s: message line %d matches deny %s", short, i+1, hit))
		}
	}
	return out
}
