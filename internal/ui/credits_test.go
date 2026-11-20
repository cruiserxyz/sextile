package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile"
)

// All values in this file are constructed synthetic test inputs.

// The Status panel's Details has two tabs, Credits first, and remembers which
// one it was on.

// fixtureCopyright stands for LICENSE's copyright line in every frame a test draws.
const fixtureCopyright = "Copyright (c) 2026 Example Holder"

func statusModel(t *testing.T, w, h int) Model {
	t.Helper()
	m := sessionsModel(t, 1)
	m.cfg = Config{Color: ColorNever, Version: "0.1.4", Copyright: fixtureCopyright}
	m.width, m.height = w, h
	return m
}

func TestStatusTitleAndFoldedLineSayStatus(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {132, 40}} {
		m := statusModel(t, size[0], size[1])
		m, _ = press(t, m, "1")
		if view := strings.Join(screenLines(m), "\n"); !strings.Contains(view, "[1] Status") || strings.Contains(view, "[1] Session") {
			t.Errorf("%dx%d: the panel is not titled Status:\n%s", size[0], size[1], view)
		}
		m, _ = press(t, m, "enter")
		lines := screenLines(m)
		if bar := lines[len(lines)-1]; !strings.Contains(bar, "Back to Status: <esc>") {
			t.Errorf("%dx%d: the bar is %q, want Back to Status: <esc>", size[0], size[1], bar)
		}
		m, _ = press(t, m, "esc", "2")
		if view := strings.Join(screenLines(m), "\n"); !strings.Contains(view, "[1] Status") {
			t.Errorf("%dx%d: the folded or open Status line is missing", size[0], size[1])
		}
	}
}

func TestStatusDetailsOpensOnCreditsFirst(t *testing.T) {
	m := statusModel(t, 132, 40)
	m, _ = press(t, m, "1", "enter")
	if got := m.activeTabID(PanelDetails); got != TabDetailsReport {
		t.Fatalf("the first Status Details tab is %q, want the Credits tab", got)
	}
	tabs := m.panelTabs(PanelDetails)
	if len(tabs) != 2 || tabs[0].Title != "Credits" || tabs[1].Title != "Session" {
		t.Fatalf("the Status Details tabs are %v, want Credits then Session", tabs)
	}
	view := strings.Join(screenLines(m), "\n")
	for _, want := range []string{
		"[0] [Credits] - Session", "Sextile 0.1.4",
		"The calculations are Astrolog 8.00's, by Walter D. Pullen,",
		"with the Swiss Ephemeris by Astrodienst AG.",
		"The screen follows lazygit, by Jesse Duffield.",
		"Built with Bubble Tea, by Charm.",
		"MIT",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("Credits lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Sessions in ~/charts") {
		t.Errorf("the first tab lists the sessions:\n%s", view)
	}
	m, _ = press(t, m, "]")
	if got := m.activeTabID(PanelDetails); got != TabStatusSession {
		t.Fatalf("] gives tab %q, want the Session tab", got)
	}
	view = strings.Join(screenLines(m), "\n")
	if !strings.Contains(view, "Sessions in ~/charts") || !strings.Contains(view, "Charts folder: ~/charts") || strings.Contains(view, "Walter D. Pullen") {
		t.Errorf("the Session tab is wrong:\n%s", view)
	}
}

func TestStatusDetailsRemembersItsTab(t *testing.T) {
	m := statusModel(t, 132, 40)
	m, _ = press(t, m, "1", "enter", "]")
	m, _ = press(t, m, "2")
	m, _ = press(t, m, "1")
	if got := m.activeTabID(PanelDetails); got != TabStatusSession {
		t.Errorf("after another panel and back the tab is %q, want Session", got)
	}
	m, _ = press(t, m, "esc", "enter")
	if got := m.activeTabID(PanelDetails); got != TabStatusSession {
		t.Errorf("after closing and opening Details the tab is %q, want Session", got)
	}
	m = m.forgetTabMemory()
	if got := m.activeTabID(PanelDetails); got != TabDetailsReport {
		t.Errorf("after a new session the tab is %q, want Credits", got)
	}
}

func TestNewSessionForgetsTheStatusTab(t *testing.T) {
	m := statusModel(t, 132, 40)
	m, _ = press(t, m, "1", "enter", "]")
	m, cmd := pressNewSession(t, m)
	m = pump(t, m, cmd)
	m.focused = PanelStatus
	if got := m.activeTabID(PanelDetails); got != TabDetailsReport {
		t.Errorf("after a new session the Status tab is %q, want Credits", got)
	}
}

func TestWordmarkFitsItsRoom(t *testing.T) {
	if msg := checkWordmark(wordmarkText); msg != "" {
		t.Error(msg)
	}
	nine := strings.Repeat("xxxx\n", 9)
	if checkWordmark(nine) == "" {
		t.Error("nine rows pass the size check")
	}
	if checkWordmark(strings.Repeat("x", 61)) == "" {
		t.Error("a 61-column row passes the size check")
	}
	if checkWordmark("   \n\n") != "" || wordmarkRows("   \n\n") != nil {
		t.Error("an empty wordmark is not empty")
	}
}

func TestEmptyWordmarkShowsThePlainName(t *testing.T) {
	if strings.TrimSpace(wordmarkText) != "" {
		t.Skip("the wordmark file holds lettering")
	}
	m := statusModel(t, 132, 40)
	if first := strings.Split(m.creditsText(), "\n")[0]; first != "Sextile" {
		t.Errorf("the first Credits line is %q, want the plain name", first)
	}
}

func TestFixtureCopyrightIsShown(t *testing.T) {
	m := statusModel(t, 132, 40)
	if !strings.Contains("\n"+m.creditsText()+"\n", "\nCopyright 2026 Example Holder\n") {
		t.Errorf("Credits does not show the copyright line:\n%s", m.creditsText())
	}
	m.cfg.Copyright = ""
	if strings.Contains(m.creditsText(), "Copyright") {
		t.Error("a copyright line shows with none given")
	}
}

// The one test that reads the real LICENSE: the line the program starts with,
// and the line Credits shows, are the file's own.
func TestCopyrightLineIsLICENSEs(t *testing.T) {
	b, err := os.ReadFile("../../LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	var line string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "Copyright (c) ") {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatal("LICENSE has no copyright line")
	}
	if got := sextile.Copyright(); got != line {
		t.Errorf("the embedded LICENSE gives %q, the file says %q", got, line)
	}
	m := statusModel(t, 132, 40)
	m.cfg.Copyright = sextile.Copyright()
	want := strings.Replace(line, "(c) ", "", 1)
	if !strings.Contains("\n"+m.creditsText()+"\n", "\n"+want+"\n") {
		t.Errorf("Credits does not show LICENSE's line:\n%s", m.creditsText())
	}
}

func TestCreditLinksNameTheBuild(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    []string
	}{
		{"0.1.4", []string{
			"Keybindings: https://github.com/cruiserxyz/sextile/blob/v0.1.4/docs/REFERENCE.md#keys",
			"First steps: https://github.com/cruiserxyz/sextile/blob/v0.1.4/README.md#your-first-chart",
			"Reference: https://github.com/cruiserxyz/sextile/blob/v0.1.4/docs/REFERENCE.md",
			"Tell us: https://github.com/cruiserxyz/sextile/issues",
			"Release notes: https://github.com/cruiserxyz/sextile/releases/tag/v0.1.4",
		}},
		{"devel", []string{
			"Keybindings: https://github.com/cruiserxyz/sextile/blob/main/docs/REFERENCE.md#keys",
			"First steps: https://github.com/cruiserxyz/sextile/blob/main/README.md#your-first-chart",
			"Reference: https://github.com/cruiserxyz/sextile/blob/main/docs/REFERENCE.md",
			"Tell us: https://github.com/cruiserxyz/sextile/issues",
			"Release notes: https://github.com/cruiserxyz/sextile/releases",
		}},
	} {
		got := creditLinks(tc.version, "")
		if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
			t.Errorf("version %q: links\n%s\nwant\n%s", tc.version, strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
		}
		for _, l := range got {
			if strings.Count(l, "https://") != 1 {
				t.Errorf("%q does not hold exactly one address", l)
			}
		}
	}
	if got := creditLinks("", ""); !strings.Contains(got[0], "/blob/main/") {
		t.Errorf("an empty version gives %q, want main", got[0])
	}
}

func TestSupportLineIsAbsentWhileNoAddressIsSet(t *testing.T) {
	if supportURL != "" {
		t.Skip("an address is configured")
	}
	if strings.Contains(strings.Join(creditLinks("0.1.4", supportURL), "\n"), "Support") {
		t.Error("a Support line shows with no address")
	}
	if got := creditLinks("0.1.4", "https://example.invalid/x"); !strings.HasSuffix(got[len(got)-1], "Support Sextile: https://example.invalid/x") {
		t.Errorf("a configured address gives %v", got)
	}
}

func TestRepoAddressMatchesTheREADMEIssueLink(t *testing.T) {
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "]("+repoURL+"/issues)") {
		t.Errorf("README's issue link is not %s/issues", repoURL)
	}
}

func TestCreditsLinesAreOnePlainLineEach(t *testing.T) {
	m := statusModel(t, 132, 40)
	for _, l := range strings.Split(m.creditsText(), "\n") {
		if ansi.StringWidth(l) > 100 {
			t.Errorf("Credits line %q is %d cells wide", l, ansi.StringWidth(l))
		}
	}
}

// The wordmark is drawn once: four lines of ASCII art, no second copy.
func TestWordmarkIsDrawnOnce(t *testing.T) {
	if n := strings.Count(strings.TrimRight(wordmarkText, "\n"), "\n") + 1; n != 4 {
		t.Errorf("the wordmark has %d lines, want 4:\n%s", n, wordmarkText)
	}
}
