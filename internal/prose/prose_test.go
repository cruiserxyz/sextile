package prose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// repoRoot is two levels up from internal/prose.
const repoRoot = "../.."

// codeLiterals are string literals that contain a semicolon because they are
// code, a wire format or a diagnostic label that never reaches a person's
// screen. Each is matched exactly; a new one needs a reason here.
var codeLiterals = map[string]string{
	";":                        "a separator character",
	"|&;<>()$`\\*?[]{}!#~='\"": "shell metacharacters",
	"\\x1b\\[[0-9;]*m":         "SGR regexp",
	"%d|%d|%v|%v|%s|%s\x00%s;": "cache key",
	"@AI780  ; Astrolog chart info.\n-qj %s\n-zl %s %s\n%s\n-c %s\n%s":   "Astrolog file syntax",
	"@AI780  ; Astrolog chart info.\n-qj %s\n-zl %s %s\n%s%s\n-c %s\n%s": "Astrolog file syntax",
	"text/html; charset=utf-8":     "HTTP header",
	"text/plain; charset=utf-8":    "HTTP header",
	"image/svg+xml; charset=utf-8": "HTTP header",
	"default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src 'self'": "HTTP header",
}

// codePrefixes are literals that are code by their opening.
var codePrefixes = []string{
	"<!doctype html>",  // the served wheel page: CSS
	"chart-file read:", // argv exemption label, a diagnostic key
}

func isCode(s string) bool {
	if _, ok := codeLiterals[s]; ok {
		return true
	}
	for _, p := range codePrefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// TestNoSemicolonInGoMessages: no string literal in the non-test Go code that
// a user can read (Log lines, notices, refusals, popup texts, flag help,
// errors) holds a semicolon. Literals that are code are listed in codeLiterals.
func TestNoSemicolonInGoMessages(t *testing.T) {
	var bad []string
	for _, d := range []string{"internal", "cmd", "tools", "pkg"} {
		root := filepath.Join(repoRoot, d)
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			fs := token.NewFileSet()
			f, perr := parser.ParseFile(fs, p, nil, 0)
			if perr != nil {
				t.Errorf("parse %s: %v", p, perr)
				return nil
			}
			ast.Inspect(f, func(n ast.Node) bool {
				b, ok := n.(*ast.BasicLit)
				if !ok || b.Kind != token.STRING {
					return true
				}
				s, uerr := strconv.Unquote(b.Value)
				if uerr != nil || !strings.Contains(s, ";") || isCode(s) {
					return true
				}
				bad = append(bad, p+":"+strconv.Itoa(fs.Position(b.Pos()).Line)+": "+strconv.Quote(s))
				return true
			})
			return nil
		})
	}
	for _, b := range bad {
		t.Errorf("semicolon in a user-visible string: %s", b)
	}
}

var quoted = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|'[^']*'`)

// shellMessages returns the quoted strings (and heredoc lines) of a shell
// script that are not comments.
type msg struct{ loc, text string }

func shellMessages(t *testing.T, path string) []msg {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []msg
	inHere := ""
	for i, line := range strings.Split(string(raw), "\n") {
		loc := path + ":" + strconv.Itoa(i+1)
		if inHere != "" {
			if strings.TrimSpace(line) == inHere {
				inHere = ""
				continue
			}
			out = append(out, msg{loc, line})
			continue
		}
		if m := regexp.MustCompile(`<<-?\s*'?([A-Z_]+)'?\s*$`).FindStringSubmatch(line); m != nil {
			inHere = m[1]
		}
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		for _, q := range quoted.FindAllString(line, -1) {
			out = append(out, msg{loc, q})
		}
	}
	return out
}

// shellCode are quoted shell fragments that are code, not messages.
var shellCode = []string{}

// TestNoSemicolonInLauncherMessages: the bash launcher, the installer and the
// launcher's recorded output hold no semicolon in anything they print.
func TestNoSemicolonInLauncherMessages(t *testing.T) {
	var all []msg
	for _, f := range []string{"sextile", "scripts/install"} {
		all = append(all, shellMessages(t, filepath.Join(repoRoot, f))...)
	}
	goldens, _ := filepath.Glob(filepath.Join(repoRoot, "tools/launcher/testdata/*.golden"))
	for _, g := range goldens {
		raw, err := os.ReadFile(g)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(raw), "\n") {
			all = append(all, msg{g + ":" + strconv.Itoa(i+1), line})
		}
	}
	for _, m := range all {
		if !strings.Contains(m.text, ";") {
			continue
		}
		skip := false
		for _, c := range shellCode {
			if strings.Contains(m.text, c) {
				skip = true
			}
		}
		if !skip {
			t.Errorf("semicolon in a launcher message: %s: %s", m.loc, m.text)
		}
	}
}

var codeSpan = regexp.MustCompile("`[^`]*`")

// TestNoSemicolonInReferenceProse: docs/REFERENCE.md prose outside fenced
// blocks and code spans holds no semicolon.
func TestNoSemicolonInReferenceProse(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot, "docs/REFERENCE.md"))
	if err != nil {
		t.Fatal(err)
	}
	fenced := false
	for i, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		if strings.Contains(codeSpan.ReplaceAllString(line, ""), ";") {
			t.Errorf("docs/REFERENCE.md:%d: semicolon in prose: %s", i+1, line)
		}
	}
}
