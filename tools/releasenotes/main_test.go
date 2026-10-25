package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const changelog = `# Changelog

## [Unreleased]

## [0.2.0] - unreleased

## [0.1.0] - 2026-10-30

Sextile 0.1.0 opens your chart files and compares them with today's sky.

- Details
`

func runWith(t *testing.T, tag, content string) (int, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "CHANGELOG.md")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := run([]string{tag, p}, &out, &errb)
	return code, out.String() + errb.String()
}

func TestSectionIsPrinted(t *testing.T) {
	for _, tag := range []string{"v0.1.0", "v0.1.0-rc.1"} {
		code, out := runWith(t, tag, changelog)
		if code != 0 || !strings.HasPrefix(out, "Sextile 0.1.0 opens your chart files") || !strings.Contains(out, "- Details") {
			t.Fatalf("%s: exit %d\n%s", tag, code, out)
		}
	}
}

func TestRefusals(t *testing.T) {
	cases := []struct{ name, tag, content, want string }{
		{"no section", "v0.3.0", changelog, "no section ## [0.3.0]"},
		{"empty section", "v0.2.0", changelog, "is empty"},
		{"opens with a list", "v0.1.0", strings.Replace(changelog, "Sextile 0.1.0 opens", "- Sextile 0.1.0 opens", 1), "must open with a sentence"},
		{"opens with a heading", "v0.1.0", strings.Replace(changelog, "Sextile 0.1.0 opens", "### Added\n\nSextile 0.1.0 opens", 1), "must open with a sentence"},
		{"opens with a comment", "v0.1.0", strings.Replace(changelog, "Sextile 0.1.0 opens", "<!-- draft -->\nSextile 0.1.0 opens", 1), "must open with a sentence"},
		{"opens with a numbered item", "v0.1.0", strings.Replace(changelog, "Sextile 0.1.0 opens", "1) Sextile 0.1.0 opens", 1), "must open with a sentence"},
		{"final tag, undated", "v0.1.0", strings.Replace(changelog, "## [0.1.0] - 2026-10-30", "## [0.1.0] - unreleased", 1), "no release date"},
		{"final tag, no date", "v0.1.0", strings.Replace(changelog, "## [0.1.0] - 2026-10-30", "## [0.1.0]", 1), "no release date"},
		{"final tag, not a date", "v0.1.0", strings.Replace(changelog, "## [0.1.0] - 2026-10-30", "## [0.1.0] - soon", 1), "no release date"},
		{"not a tag", "0.1.0", changelog, "is not a vX.Y.Z tag"},
		{"missing file", "v0.1.0", "", "no section"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, out := runWith(t, c.tag, c.content)
			if code != 1 || !strings.Contains(out, c.want) {
				t.Fatalf("want exit 1 with %q, got %d\n%s", c.want, code, out)
			}
		})
	}
}

func TestOpeningMayStartWithANumber(t *testing.T) {
	numbered := strings.Replace(changelog, "Sextile 0.1.0 opens", "3.14 is not a list. Sextile 0.1.0 opens", 1)
	if code, out := runWith(t, "v0.1.0", numbered); code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

func TestPreReleaseReadsUndated(t *testing.T) {
	undated := strings.Replace(changelog, "## [0.1.0] - 2026-10-30", "## [0.1.0] - unreleased", 1)
	code, out := runWith(t, "v0.1.0-rc.1", undated)
	if code != 0 || !strings.HasPrefix(out, "Sextile 0.1.0 opens") {
		t.Fatalf("rc, undated: exit %d\n%s", code, out)
	}
}
