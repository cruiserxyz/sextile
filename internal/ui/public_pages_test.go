package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// The public pages name the panel numbers and the keys that focus them. The
// registry is the model's own: each panel's strip prefix (`[3] `) and the key
// that focuses it (panelDigits with the key map). A page that names a number
// the registry does not hold for that panel fails here.

// panelRegistry maps a panel's title to the number the screen draws on it and
// to the key that focuses it.
func panelRegistry(t *testing.T) (number map[string]string, digit map[string]string, order []string) {
	t.Helper()
	m := helpFixture(t, 132, 40)
	number, digit = map[string]string{}, map[string]string{}
	for _, p := range focusablePanels() {
		tabs := m.panelTabs(p)
		if len(tabs) == 0 {
			t.Fatalf("panel %s has no title", p)
		}
		title := tabs[0].Title
		if p == PanelBodies {
			title = tabTitleBodies
		}
		prefix := strings.TrimSpace(m.tabStrips()[p].prefix)
		number[title] = strings.Trim(prefix, "[]")
		order = append(order, title)
	}
	number["Details"] = strings.Trim(strings.TrimSpace(detailsPrefix), "[]")
	for id, p := range panelDigits {
		var k []string
		switch id {
		case keys.BindingFocusSession:
			k = m.keys.FocusSession.Keys()
		case keys.BindingFocusChart:
			k = m.keys.FocusChart.Keys()
		case keys.BindingFocusBodies:
			k = m.keys.FocusBodies.Keys()
		case keys.BindingFocusAspects:
			k = m.keys.FocusAspects.Keys()
		}
		title := m.panelTabs(p)[0].Title
		if p == PanelBodies {
			title = tabTitleBodies
		}
		if len(k) != 1 {
			t.Fatalf("%s: focus keys %v", p, k)
		}
		digit[title] = k[0]
	}
	digit["Details"] = m.keys.DetailsTab.Keys()[0]
	return number, digit, order
}

var (
	pageBracketed = regexp.MustCompile(`\[(\d)\]\s*(Status|Session|Chart|Bodies|Aspects|Details)\b`)
	pageFocuses   = regexp.MustCompile("`(\\d)` (?:focuses|opens|goes to|moves to) (?:the )?(Status|Session|Chart|Bodies|Aspects|Details)\\b")
	pageDigitRun  = regexp.MustCompile("(?:`\\d` )+`\\d`")
	pageDigits    = regexp.MustCompile("`(\\d)`")
	pageNames     = regexp.MustCompile(`\b(Status|Session|Chart|Bodies|Aspects)\b`)
)

// publicPageProblems lists every place the text names a panel number or a
// focus key the registry does not hold, and every mention of the retired
// Limited view.
func publicPageProblems(t *testing.T, text string) []string {
	t.Helper()
	number, digit, _ := panelRegistry(t)
	var out []string
	for _, m := range pageBracketed.FindAllStringSubmatch(text, -1) {
		// Details is titled by what it shows (`[0] Aspects`, `[0] Sun`), so a
		// panel name after its number is its content, never a wrong number.
		if m[1] == number["Details"] {
			continue
		}
		if number[m[2]] != m[1] {
			out = append(out, fmt.Sprintf("%q: %s is [%s]", m[0], m[2], number[m[2]]))
		}
	}
	for _, m := range pageFocuses.FindAllStringSubmatch(text, -1) {
		if digit[m[2]] != m[1] {
			out = append(out, fmt.Sprintf("%q: %s is focused by %s", m[0], m[2], digit[m[2]]))
		}
	}
	// A row such as "`Tab` `1` `2` `3` `4` | Move between the Session, Chart, Bodies and Aspects panels":
	// the digits and the panel names that follow them pair up in order.
	for _, line := range strings.Split(text, "\n") {
		loc := pageDigitRun.FindStringIndex(line)
		if loc == nil {
			continue
		}
		digits := pageDigits.FindAllStringSubmatch(line[loc[0]:loc[1]], -1)
		names := pageNames.FindAllString(line[loc[1]:], -1)
		if len(names) < len(digits) {
			continue
		}
		for i, d := range digits {
			if digit[names[i]] != d[1] {
				out = append(out, fmt.Sprintf("%q: the %s key is %s, not %s", strings.TrimSpace(line), names[i], digit[names[i]], d[1]))
			}
		}
	}
	if strings.Contains(text, "Limited view") {
		out = append(out, "names the Limited view notice, which is gone")
	}
	return out
}

// publicPage is one page a reader sees. First is the number of its first line
// in the file it comes from, so a line of the CHANGELOG's pending section is
// reported at its place in CHANGELOG.md.
type publicPage struct {
	Name  string
	Text  string
	First int
}

// readPublicPages reads the pages a reader sees, under root: README.md, every
// .md directly in docs/ (read from the folder, so a new page is checked from
// its first commit) and the pending section of CHANGELOG.md, from its
// "## [X.Y.Z] - unreleased" heading to the next "## [". A dated section is
// published history and is never read.
func readPublicPages(root string) ([]publicPage, error) {
	paths, err := filepath.Glob(filepath.Join(root, "docs", "*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	paths = append([]string{filepath.Join(root, "README.md")}, paths...)
	var pages []publicPage
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		name, _ := filepath.Rel(root, p)
		pages = append(pages, publicPage{Name: filepath.ToSlash(name), Text: string(b), First: 1})
	}
	b, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(b), "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "## [") && strings.Contains(l, "] - unreleased") {
			start = i
			break
		}
	}
	if start >= 0 {
		end := len(lines)
		for i := start + 1; i < len(lines); i++ {
			if strings.HasPrefix(lines[i], "## [") {
				end = i
				break
			}
		}
		pages = append(pages, publicPage{Name: "CHANGELOG.md", Text: strings.Join(lines[start:end], "\n"), First: start + 1})
	}
	return pages, nil
}

// publicPages reads the pages from the repository root.
func publicPages(t *testing.T) []publicPage {
	t.Helper()
	pages, err := readPublicPages(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return pages
}

// The control: a copy of a page with a wrong number put into it is named, so
// the test above can fail, and the registry's own numbers pass.
func TestPublicPagesCheckNamesAWrongPanelNumber(t *testing.T) {
	number, digit, order := panelRegistry(t)
	if number["Status"] != "1" || number["Chart"] != "2" || number["Bodies"] != "3" || number["Aspects"] != "4" || number["Details"] != "0" {
		t.Fatalf("the registry's numbers are %v", number)
	}
	good := "The `[2] Chart` panel. [2] Chart and [3] Bodies. `3` focuses Bodies. [0] Aspects is Details on the Aspects panel.\n| `Tab` `1` `2` `3` `4` | Move between the Status, Chart, Bodies and Aspects panels |\n"
	if got := publicPageProblems(t, good); len(got) != 0 {
		t.Fatalf("the registry's own numbers are refused: %v (digits %v, order %v)", got, digit, order)
	}
	for _, bad := range []string{
		good + "[3] Chart opens first.\n",
		good + "[4] Details shows the report.\n",
		good + "`2` focuses Bodies.\n",
		good + "| `Tab` `1` `2` `3` | Move between the Chart, Bodies and Aspects panels |\n",
		good + "Limited view: 132x30 shows Details and the Log\n",
	} {
		if len(publicPageProblems(t, bad)) == 0 {
			t.Errorf("a wrong page passed:\n%s", bad)
		}
	}
}
