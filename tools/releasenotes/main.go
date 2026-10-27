// Command releasenotes prints the CHANGELOG.md section of a version, and
// refuses a tag that has none: a release without its notes does not exist.
//
//	releasenotes vX.Y.Z[-pre]  [CHANGELOG.md]
//
// A pre-release tag (v0.1.0-rc.1) reads its base version's section ([0.1.0]).
// The section must be non-empty and open with a sentence: its first line is
// not a heading or a list item. A final tag (no -pre) also refuses a heading
// still dated "unreleased"; a pre-release tag reads it. Exit 0 and the
// section on stdout; exit 1 with the reason otherwise.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

var tagRE = regexp.MustCompile(`^v(\d+\.\d+\.\d+)(-[0-9A-Za-z.]+)?$`)

// notSentenceRE matches a first line that is a heading, a list item, a quote,
// a table row or an HTML comment; a sentence that starts with a number passes.
var notSentenceRE = regexp.MustCompile(`^(#|[-*+<>|]|\d+[.)](\s|$))`)

// datedRE is what a final release's heading carries after "## [X.Y.Z]".
var datedRE = regexp.MustCompile(`^ - \d{4}-\d{2}-\d{2}$`)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprintln(stderr, "usage: releasenotes vX.Y.Z[-pre] [CHANGELOG.md]")
		return 2
	}
	m := tagRE.FindStringSubmatch(args[0])
	if m == nil {
		fmt.Fprintf(stderr, "releasenotes: %q is not a vX.Y.Z tag\n", args[0])
		return 1
	}
	version := m[1]
	path := "CHANGELOG.md"
	if len(args) == 2 {
		path = args[1]
	}
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "releasenotes: %v\n", err)
		return 1
	}
	defer f.Close()
	section, err := extract(f, version, m[2] == "")
	if err != nil {
		fmt.Fprintf(stderr, "releasenotes: %v\n", err)
		return 1
	}
	fmt.Fprint(stdout, section)
	return 0
}

// extract returns the text under "## [version]" up to the next "## " heading.
// With final set, a heading without a " - YYYY-MM-DD" date is refused.
func extract(r io.Reader, version string, final bool) (string, error) {
	heading := "## [" + version + "]"
	sc := bufio.NewScanner(r)
	var lines []string
	in := false
	var found string
	for sc.Scan() {
		l := sc.Text()
		if strings.HasPrefix(l, "## ") {
			if in {
				break
			}
			if l == heading || strings.HasPrefix(l, heading+" ") {
				in = true
				found = l
			}
			continue
		}
		if in {
			lines = append(lines, l)
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	if !in {
		return "", fmt.Errorf("CHANGELOG.md has no section %s", heading)
	}
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if text == "" {
		return "", fmt.Errorf("the section %s is empty", heading)
	}
	if final && !datedRE.MatchString(strings.TrimPrefix(found, heading)) {
		return "", fmt.Errorf("the section %s has no release date (%q). Write \"%s - YYYY-MM-DD\" before tagging a final release", heading, found, heading)
	}
	first := strings.SplitN(text, "\n", 2)[0]
	if notSentenceRE.MatchString(first) {
		return "", fmt.Errorf("the section %s must open with a sentence, it opens with %q", heading, first)
	}
	return text + "\n", nil
}
