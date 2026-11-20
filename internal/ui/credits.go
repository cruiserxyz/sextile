package ui

import (
	_ "embed"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// credits.go is the Status panel's Credits tab, close to lazygit's Status: the
// name in lettering, a copyright line, the credits, and one line per link.

// wordmarkText is the lettering, shown exactly as it is. The file is supplied
// by hand; while it holds only whitespace the tab shows the plain name.
//
//go:embed wordmark.txt
var wordmarkText string

// wordmarkMaxRows and wordmarkMaxCols are the room the lettering has where
// Details opens below a panel.
const (
	wordmarkMaxRows = 8
	wordmarkMaxCols = 60
)

// repoURL is the one address of Sextile's repository the tab's links start
// from.
const repoURL = "https://github.com/cruiserxyz/sextile"

// supportURL is the address of the Support Sextile line. While it is empty the
// line is absent.
const supportURL = ""

// wordmarkRows are the lettering's rows with the blank rows around it cut off,
// or nil while the file is empty.
func wordmarkRows(text string) []string {
	rows := strings.Split(strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), " \t\r\n"), "\n")
	for len(rows) > 0 && strings.TrimSpace(rows[0]) == "" {
		rows = rows[1:]
	}
	if len(rows) == 0 {
		return nil
	}
	return rows
}

// checkWordmark says whether the lettering fits its room: at most 8 rows and
// 60 columns.
func checkWordmark(text string) string {
	rows := wordmarkRows(text)
	if len(rows) > wordmarkMaxRows {
		return "the wordmark has more than 8 rows"
	}
	for _, r := range rows {
		if ansi.StringWidth(r) > wordmarkMaxCols {
			return "the wordmark has a row wider than 60 columns"
		}
	}
	return ""
}

// copyrightLine is the Credits tab's line, `Copyright <year> <name>`, from the
// LICENSE's copyright line (`Copyright (c) <year> <name>`).
func copyrightLine(license string) string {
	return strings.TrimSpace(strings.Replace(license, "Copyright (c) ", "Copyright ", 1))
}

// exactRelease is a release version: digits, dot, digits, dot, digits, and
// nothing after.
var exactRelease = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)$`)

// releaseRef is the git ref the pages of this build live at: the release tag
// when the version is exactly a release, else main. A build between releases
// ("v0.1.3-15-g1b28d0f") and one without git ("local") have no tag of their own.
func releaseRef(version string) (ref string, release bool) {
	if m := exactRelease.FindStringSubmatch(strings.TrimSpace(version)); m != nil {
		return "v" + m[1], true
	}
	return "main", false
}

// creditLinks are the tab's link lines, each a label and one address.
func creditLinks(version, support string) []string {
	ref, release := releaseRef(version)
	blob := repoURL + "/blob/" + ref
	notes := repoURL + "/releases"
	if release {
		notes = repoURL + "/releases/tag/" + ref
	}
	links := []string{
		"Keybindings: " + blob + "/docs/REFERENCE.md#keys",
		"First steps: " + blob + "/README.md#your-first-chart",
		"Reference: " + blob + "/docs/REFERENCE.md",
		"Tell us: " + repoURL + "/issues",
		"Release notes: " + notes,
	}
	if support != "" {
		links = append(links, "Support Sextile: "+support)
	}
	return links
}

// creditsText is the Credits tab: the lettering (the plain name while the file
// is empty), the copyright line, the credits and the links.
func (m Model) creditsText() string {
	var b []string
	if rows := wordmarkRows(wordmarkText); rows != nil {
		b = append(b, rows...)
	} else {
		b = append(b, "Sextile")
	}
	b = append(b, "")
	if c := copyrightLine(m.cfg.Copyright); c != "" {
		b = append(b, c, "")
	}
	title := "Sextile"
	if m.cfg.Version != "" {
		title += " " + m.cfg.Version
	}
	b = append(b, title,
		"  The calculations are Astrolog 8.00's, by Walter D. Pullen,",
		"  with the Swiss Ephemeris by Astrodienst AG.",
		"  The screen follows lazygit, by Jesse Duffield.",
		"  Built with Bubble Tea, by Charm.",
		"  Sextile is MIT licensed. Astrolog keeps its own licence.",
		"")
	b = append(b, creditLinks(m.cfg.Version, supportURL)...)
	return strings.Join(b, "\n")
}
