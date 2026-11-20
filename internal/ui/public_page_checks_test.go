package ui

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// The public pages are README.md, every docs/*.md and the pending section of
// CHANGELOG.md (readPublicPages). The checks below read the real pages. Each
// has a control: a copy of the text with one line broken, which the check must
// name.

// ---------------------------------------------------------------------------
// Reading a page

// backtickSpan is one `code span` of a line.
type backtickSpan struct {
	Text       string
	Start, End int // byte offsets of the span, backticks included
}

var spanRE = regexp.MustCompile("`([^`]+)`")

// pageLine is one line of a page outside a fenced code block.
type pageLine struct {
	No      int // line number in the file
	Text    string
	Heading string // text of the nearest heading above
	Spans   []backtickSpan
}

// proseLines are the lines of a page outside fenced code blocks, with their
// spans and the heading they sit under.
func proseLines(p publicPage) []pageLine {
	var out []pageLine
	fenced := false
	heading := ""
	for i, line := range strings.Split(p.Text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		if strings.HasPrefix(line, "#") {
			heading = strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
		l := pageLine{No: p.First + i, Text: line, Heading: heading}
		for _, m := range spanRE.FindAllStringSubmatchIndex(line, -1) {
			l.Spans = append(l.Spans, backtickSpan{Text: line[m[2]:m[3]], Start: m[0], End: m[1]})
		}
		out = append(out, l)
	}
	return out
}

// ---------------------------------------------------------------------------
// K1: every key a page names is a key of the program

// docKeyRegistry is what the program holds: the keys of every owner's table as
// a page spells them (`Ctrl+L`, `PgUp`, `↑`), the command letters, the Ctrl
// key each command answers to, and the Keybindings popup's own spelling
// (`<ctrl+l>`).
type docKeyRegistry struct {
	keys      map[string]bool // a page's spelling
	popup     map[string]bool // the Keybindings spelling, without the brackets
	commands  []keys.Command
	ctrlOf    map[string]string // command letter (one-key rows) -> "Ctrl+L"
	hasThen   map[string]bool   // a letter that opens a two-key row
	leaderKey string
}

// docKeyName is how a page writes the key name Bubble Tea gives.
func docKeyName(k string) string {
	switch k {
	case " ":
		return "Space"
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	case "pgdown":
		return "PgDn"
	case "pgup":
		return "PgUp"
	}
	if strings.HasPrefix(k, "ctrl+") {
		return "Ctrl+" + docKeyName(strings.TrimPrefix(k, "ctrl+"))
	}
	if strings.HasPrefix(k, "shift+") {
		return "Shift+" + docKeyName(strings.TrimPrefix(k, "shift+"))
	}
	if len(k) > 1 && k[0] >= 'a' && k[0] <= 'z' {
		return strings.ToUpper(k[:1]) + k[1:]
	}
	return k
}

// ctrlDocName is "Ctrl+" and the key, with a letter in capitals as a page
// writes it.
func ctrlDocName(k string) string {
	rest := strings.TrimPrefix(k, "ctrl+")
	if len(rest) == 1 {
		return "Ctrl+" + strings.ToUpper(rest)
	}
	return docKeyName(k)
}

func buildDocKeyRegistry(t *testing.T) docKeyRegistry {
	t.Helper()
	m := kbModel(t)
	r := docKeyRegistry{
		keys: map[string]bool{}, popup: map[string]bool{}, commands: keys.Commands(),
		ctrlOf: map[string]string{}, hasThen: map[string]bool{}, leaderKey: m.leaderKey,
	}
	add := func(k string) {
		r.popup[k] = true
		switch {
		case strings.HasPrefix(k, "ctrl+"):
			r.keys[ctrlDocName(k)] = true
		default:
			r.keys[docKeyName(k)] = true
		}
	}
	for o := owner(0); o <= ownerExperiments; o++ {
		for _, e := range m.keysContextFor(o).Table {
			for _, k := range e.Binding.Keys() {
				add(k)
			}
		}
	}
	for _, c := range r.commands {
		r.keys[c.Letter] = true
		if c.Then != "" {
			r.hasThen[c.Letter] = true
		}
		if k, ok := keys.CommandCtrl(c); ok {
			add(k)
			r.ctrlOf[c.Letter] = ctrlDocName(k)
		}
	}
	add(r.leaderKey)
	return r
}

// commandWordAlias is the page's word for a command whose registry word is
// not the one a page uses.
var commandWordAlias = map[string]string{"bodies": "planets", "save defaults": "defaults"}

// bareKeyNames are the named keys a page may write in backticks.
var bareKeyNames = map[string]bool{
	"Space": true, "Enter": true, "Esc": true, "Tab": true, "Delete": true, "Home": true, "End": true,
	"PgUp": true, "PgDn": true, "Shift+Tab": true, "↑": true, "↓": true, "←": true, "→": true,
}

// codeSections are the headings of the sections whose one-letter spans are
// codes of the screen (`A`, `s`, `c` for a house), not keys.
var codeSections = map[string]bool{"House display": true, "Frame codes": true}

// modifierNames are written alone in backticks to name a key of the keyboard,
// not a key of Sextile ("hold `Shift`").
var modifierNames = map[string]bool{"Shift": true, "Ctrl": true, "Cmd": true}

var (
	commaSpanRE = regexp.MustCompile(`^,\s?([A-Za-z])(?:\s([A-Za-z]))?$`)
	ctrlSpanRE  = regexp.MustCompile(`^Ctrl\+(.+)$`)
	popupSpanRE = regexp.MustCompile(`^<([a-z0-9+/]+)>$`)
	keyRuneRE   = regexp.MustCompile(`^[A-Za-z0-9?/\[\]]$`)
	gapRE       = regexp.MustCompile(`^\s*(?:(?:and )?then\s+)?$`)
)

// keyProblems lists every key, comma command and Ctrl key the page names that
// the registry does not hold, and every command a page gives another key than
// the registry does. Keys are found only where a page writes them: in
// backticks.
func keyProblems(p publicPage, r docKeyRegistry) []string {
	var out []string
	bad := func(l pageLine, format string, a ...any) {
		out = append(out, fmt.Sprintf("%s:%d: %s: %s", p.Name, l.No, fmt.Sprintf(format, a...), strings.TrimSpace(l.Text)))
	}
	commandOf := func(letter, then string) (keys.Command, bool) {
		for _, c := range r.commands {
			if c.Letter == letter && c.Then == then {
				return c, true
			}
		}
		return keys.Command{}, false
	}
	for _, l := range proseLines(p) {
		isRow := strings.HasPrefix(strings.TrimSpace(l.Text), "|")
		inCommands := l.Heading == "Commands"
		consumed := map[int]bool{}
		type seq struct {
			cmd keys.Command
			ok  bool
		}
		var seqs []seq
		// gone is a removal note: the key or command right before "is gone" names
		// one the program no longer has, so it is not read as a key claim.
		gone := func(last int) bool {
			return strings.HasPrefix(strings.TrimLeft(l.Text[l.Spans[last].End:], " "), "is gone")
		}
		for i, s := range l.Spans {
			var letter, then string
			last := i
			switch {
			case s.Text == r.leaderKey:
				// The comma, then the span after it names the command.
				n := i + 1
				for n < len(l.Spans) {
					prev := l.Spans[n-1]
					gap := l.Text[prev.End:l.Spans[n].Start]
					if !gapRE.MatchString(gap) || len(l.Spans[n].Text) != 1 || !isLetter(l.Spans[n].Text) {
						break
					}
					consumed[n] = true
					last = n
					if letter == "" {
						letter = l.Spans[n].Text
						if !r.hasThen[letter] {
							break
						}
					} else {
						then = l.Spans[n].Text
						break
					}
					n++
				}
				if letter == "" {
					continue
				}
			case commaSpanRE.MatchString(s.Text):
				m := commaSpanRE.FindStringSubmatch(s.Text)
				letter, then = m[1], m[2]
				consumed[i] = true
			default:
				continue
			}
			switch c, ok := commandOf(letter, then); {
			case gone(last):
			case ok:
				seqs = append(seqs, seq{c, true})
			case then == "" && r.hasThen[letter]:
				// `,` `c` alone: the first key of a two-key row.
			default:
				bad(l, "`,` `%s`%s is not a command of the key table", letter, map[bool]string{true: " `" + then + "`"}[then != ""])
			}
		}
		// A line about the Mac's own keyboard, and a section of one-letter codes,
		// name keys that are not Sextile's.
		notSextileKeys := codeSections[l.Heading]
		for _, s := range l.Spans {
			if s.Text == "Cmd" {
				notSextileKeys = true
			}
		}
		var ctrls []string
		for i, s := range l.Spans {
			if consumed[i] {
				continue
			}
			t := s.Text
			if m := popupSpanRE.FindStringSubmatch(t); m != nil {
				if !r.popup[m[1]] {
					bad(l, "`%s` is not a key the Keybindings list spells", t)
				}
				continue
			}
			if m := ctrlSpanRE.FindStringSubmatch(t); m != nil {
				switch {
				case m[1] == "^":
					// Docker's own detach key, not a key of Sextile.
				case r.keys[t]:
					ctrls = append(ctrls, t)
				case gone(i):
				default:
					bad(l, "`%s` is not a key of the key table", t)
				}
				continue
			}
			if modifierNames[t] || t == r.leaderKey {
				continue
			}
			if !notSextileKeys && (keyRuneRE.MatchString(t) || bareKeyNames[t]) {
				if !r.keys[t] && !gone(i) {
					bad(l, "`%s` is not a key of the key table", t)
				}
			}
		}
		// A command with its two keys on one table row: the Ctrl key must be
		// the letter's own, and "none" must be true.
		if isRow && len(seqs) == 1 {
			c := seqs[0].cmd
			want, has := r.ctrlOf[c.Letter]
			if c.Then != "" {
				has = false
			}
			switch {
			case has && len(ctrls) > 0 && !slices.Contains(ctrls, want):
				bad(l, "the command `,` `%s` answers to %s, not %s", c.Letter, want, ctrls[0])
			case !has && len(ctrls) > 0:
				bad(l, "the command `,` `%s` has no Ctrl key, and the row gives %s", c.Letter, ctrls[0])
			case has && len(ctrls) == 0 && strings.Contains(strings.ToLower(l.Text), "| none"):
				bad(l, "the command `,` `%s` answers to %s, and the row says none", c.Letter, want)
			}
		}
		// In a Commands section a row says what each command does: the words
		// beside a command's key name that command.
		if isRow && inCommands {
			cells := strings.Split(strings.Trim(strings.TrimSpace(l.Text), "|"), "|")
			for ci, cell := range cells {
				cell = strings.TrimSpace(cell)
				var letter, then string
				var words []string
				if m := spanRE.FindAllStringSubmatch(cell, -1); len(m) == 1 && m[0][0] == cell && len(m[0][1]) == 1 && isLetter(m[0][1]) {
					// A table of command letters: the cell after the letter says what it does.
					letter = m[0][1]
					if ci+1 < len(cells) {
						words = []string{cells[ci+1]}
					}
				} else if len(seqs) == 1 && strings.HasPrefix(cell, "`,`") {
					// A table of commands: the words stand before the key, or after it.
					letter, then = seqs[0].cmd.Letter, seqs[0].cmd.Then
					for _, c := range cells[:ci] {
						if !spanOnly(strings.TrimSpace(c)) {
							words = append(words, c)
						}
					}
					if len(words) == 0 && ci+1 < len(cells) {
						words = []string{cells[ci+1]}
					}
				} else {
					continue
				}
				c, ok := commandOf(letter, then)
				if !ok {
					if !r.hasThen[letter] {
						bad(l, "`%s` is not a command letter", letter)
					}
					continue
				}
				// The words are the row's other cells, or, in a two-column
				// table of command letters, the next cell.
				said := strings.ToLower(strings.Join(words, " "))
				w := strings.TrimSuffix(c.Word, "s")
				alias := commandWordAlias[c.Word]
				if !strings.Contains(said, w) && (alias == "" || !strings.Contains(said, alias)) {
					bad(l, "`%s` is the command %q, and the row says %q", letter, c.Word, strings.TrimSpace(said))
				}
			}
		}
	}
	return out
}

// spanOnly is a cell that holds nothing but key spans.
func spanOnly(cell string) bool {
	return strings.TrimSpace(spanRE.ReplaceAllString(cell, "")) == "" && strings.Contains(cell, "`")
}

func isLetter(s string) bool {
	return len(s) == 1 && (s[0] >= 'a' && s[0] <= 'z' || s[0] >= 'A' && s[0] <= 'Z')
}

// K1: every key, comma command and Ctrl key a public page names is one the
// program holds, and the panel numbers the pages name are the screen's.
func TestPublicPagesNameOnlyRealKeys(t *testing.T) {
	reg := buildDocKeyRegistry(t)
	pages := publicPages(t)
	for _, p := range pages {
		for _, msg := range keyProblems(p, reg) {
			t.Error(msg)
		}
		for _, msg := range publicPageProblems(t, p.Text) {
			t.Errorf("%s: %s", p.Name, msg)
		}
	}

	// The control: lines of a real page, each broken in a copy, are named with
	// their file and line.
	var ref publicPage
	for _, p := range pages {
		if p.Name == "docs/REFERENCE.md" {
			ref = p
		}
	}
	if ref.Text == "" {
		t.Fatal("docs/REFERENCE.md is not a public page")
	}
	for _, c := range []struct{ from, to string }{
		{"| Open a chart | `,` `o` | `Ctrl+O` |", "| Open a chart | `,` `o` | `Ctrl+Q` |"},
		{"| Sessions | `,` `O` | none |", "| Sessions | `,` `O` | `Ctrl+O` |"},
		{"| Choose the zodiac | `,` `z` | `Ctrl+Z` |", "| Choose the zodiac | `,` `j` | `Ctrl+Z` |"},
		{"| Keep all defaults | `,` `c` `d` | none |", "| Keep all defaults | `,` `c` `q` | none |"},
		{"| Save Now as a chart | `,` `s` | `Ctrl+S` |", "| Save Now as a chart | `,` `s` | none |"},
		{"| Draw the wheel | `,` `x` | `Ctrl+X` |", "| Draw the wheel | `,` `y` | `Ctrl+Y` |"},
		{"`Ctrl+L` opens the city chooser.", "`Ctrl+K` opens the city chooser."},
		{"| `PgUp` `PgDn` |", "| `PgUp` `Fn9` `Q` |"},
	} {
		lineNo := lineOf(ref.Text, c.from)
		if lineNo == 0 {
			t.Errorf("control: the REFERENCE has no line %q", c.from)
			continue
		}
		broken := ref
		broken.Text = strings.Replace(ref.Text, c.from, c.to, 1)
		got := keyProblems(broken, reg)
		want := fmt.Sprintf("docs/REFERENCE.md:%d:", lineNo)
		if !slices.ContainsFunc(got, func(s string) bool { return strings.HasPrefix(s, want) }) {
			t.Errorf("control: breaking %q to %q was not named at %s: %q", c.from, c.to, want, got)
		}
	}
}

// lineOf is the 1-based line of text that holds s, or 0.
func lineOf(text, s string) int {
	i := strings.Index(text, s)
	if i < 0 {
		return 0
	}
	return strings.Count(text[:i], "\n") + 1
}

// ---------------------------------------------------------------------------
// K2: the list of pages is read from the folder

func TestEveryPublicPageIsChecked(t *testing.T) {
	pages := publicPages(t)
	var got []string
	for _, p := range pages {
		got = append(got, p.Name)
	}
	// The set equals the folder's own listing, not a list written here.
	want := []string{"README.md"}
	files, err := filepath.Glob("../../docs/*.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		want = append(want, "docs/"+filepath.Base(f))
	}
	// The changelog is checked while it has a pending section. A release dates
	// it, and dated sections are published history that no check rewrites.
	if cl, err := os.ReadFile("../../CHANGELOG.md"); err == nil && strings.Contains(string(cl), "] - unreleased") {
		want = append(want, "CHANGELOG.md")
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("the checks read %v, the folder holds %v", got, want)
	}
	for _, must := range []string{"README.md", "docs/REFERENCE.md", "docs/CHEATSHEET.md", "docs/ASTROLOG.md"} {
		if !slices.Contains(got, must) {
			t.Errorf("%s is not among the checked pages %v", must, got)
		}
	}

	// The control: a new page in a folder is read, checked and named by the
	// key check from its first commit, and a dated CHANGELOG section is not.
	root := t.TempDir()
	write := func(rel, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("README.md", "# R\n")
	write("docs/NEW-PAGE.md", "# New\n\nPress `,` `Q`.\n")
	write("docs/picture.svg", "<svg/>")
	write("CHANGELOG.md", "# C\n\n## [0.2.0] - unreleased\n\nfine `,` `s`\n\n## [0.1.0] - 2026-01-01\n\nold `,` `Q`\n")
	fresh, err := readPublicPages(root)
	if err != nil {
		t.Fatal(err)
	}
	reg := buildDocKeyRegistry(t)
	var problems []string
	for _, p := range fresh {
		problems = append(problems, keyProblems(p, reg)...)
	}
	if len(problems) != 1 || !strings.HasPrefix(problems[0], "docs/NEW-PAGE.md:3:") {
		t.Errorf("a new page's wrong key, and no other, should be named: %q (pages %d)", problems, len(fresh))
	}
}

// ---------------------------------------------------------------------------
// K3: every relative link and anchor resolves

var linkRE = regexp.MustCompile(`!?\[[^\]]*\]\(([^)\s]+)\)`)

// headingAnchor is the anchor GitHub gives a heading.
func headingAnchor(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	var b strings.Builder
	for _, r := range h {
		switch {
		case r == ' ' || r == '-':
			b.WriteByte('-')
		case r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r > 127 && isWordRune(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isWordRune(r rune) bool {
	return r != '·' && r != '→' && r != '←' && r != '↑' && r != '↓' && r != '…' && r != '–' && r != '—'
}

// anchorsOf is every anchor of a markdown file, with GitHub's -1, -2 for a
// repeated heading.
func anchorsOf(text string) map[string]bool {
	out := map[string]bool{}
	seen := map[string]int{}
	fenced := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced || !strings.HasPrefix(line, "#") {
			continue
		}
		a := headingAnchor(strings.TrimLeft(line, "#"))
		if n := seen[a]; n > 0 {
			out[fmt.Sprintf("%s-%d", a, n)] = true
		} else {
			out[a] = true
		}
		seen[a]++
	}
	return out
}

// linkProblems lists every relative link of the page whose file is missing
// from root, and every anchor its file does not hold.
func linkProblems(root string, p publicPage) []string {
	var out []string
	dir := path.Dir(p.Name)
	anchors := map[string]map[string]bool{}
	for _, l := range proseLines(p) {
		for _, m := range linkRE.FindAllStringSubmatch(l.Text, -1) {
			target := m[1]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			file, anchor, _ := strings.Cut(target, "#")
			full := p.Name
			if file != "" {
				full = path.Clean(path.Join(dir, file))
			}
			fsPath := filepath.Join(root, filepath.FromSlash(full))
			info, err := os.Stat(fsPath)
			if err != nil {
				out = append(out, fmt.Sprintf("%s:%d: the link %q points at %s, which is not there", p.Name, l.No, target, full))
				continue
			}
			if anchor == "" || info.IsDir() {
				continue
			}
			if anchors[full] == nil {
				text := p.Text
				if full != p.Name || p.Name == "CHANGELOG.md" {
					b, err := os.ReadFile(fsPath)
					if err != nil {
						out = append(out, fmt.Sprintf("%s:%d: %v", p.Name, l.No, err))
						continue
					}
					text = string(b)
				}
				anchors[full] = anchorsOf(text)
			}
			if !anchors[full][anchor] {
				out = append(out, fmt.Sprintf("%s:%d: the link %q points at an anchor %s does not hold", p.Name, l.No, target, full))
			}
		}
	}
	return out
}

func TestPublicLinksResolve(t *testing.T) {
	root := filepath.Join("..", "..")
	pages := publicPages(t)
	for _, p := range pages {
		for _, msg := range linkProblems(root, p) {
			t.Error(msg)
		}
	}
	// The control: a link to a missing file and one to a missing anchor are
	// each named at their line.
	for _, p := range pages {
		if p.Name != "README.md" {
			continue
		}
		for _, c := range []struct{ from, to string }{
			{"(docs/CHEATSHEET.md)", "(docs/CHEATSHEET-GONE.md)"},
			{"(docs/REFERENCE.md#sessions)", "(docs/REFERENCE.md#no-such-section)"},
		} {
			lineNo := lineOf(p.Text, c.from)
			if lineNo == 0 {
				t.Errorf("control: the README has no link %q", c.from)
				continue
			}
			broken := p
			broken.Text = strings.Replace(p.Text, c.from, c.to, 1)
			got := linkProblems(root, broken)
			want := fmt.Sprintf("README.md:%d:", lineNo)
			if !slices.ContainsFunc(got, func(s string) bool { return strings.HasPrefix(s, want) }) {
				t.Errorf("control: breaking %q was not named at %s: %q", c.from, want, got)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// V2: the voice

var (
	voiceWordRE = regexp.MustCompile(`(?i)\b(simply|just|easy|powerful|seamless)\b|\bat your fingertips\b`)
	interpretRE = regexp.MustCompile(`(?i)\binterpret\w*`)
	negationRE  = regexp.MustCompile(`(?i)\b(no|not|never|nor|without|nothing|neither)\b|n't\b`)
	sentenceRE  = regexp.MustCompile(`[^.!?]+[.!?]?`)
)

// plansPage is the one page that may name an interpretation: the roadmap
// lists an interpretation layer as a plan (D55), while every other page says
// what Sextile does today, and it interprets nothing yet.
const plansPage = "docs/ROADMAP.md"

// voiceProblems lists a forbidden word of the voice, and a sentence that
// promises an interpretation: one with "interpret" in it and no negation,
// on any page but the roadmap.
func voiceProblems(p publicPage) []string {
	var out []string
	for _, l := range proseLines(p) {
		if m := voiceWordRE.FindString(l.Text); m != "" {
			out = append(out, fmt.Sprintf("%s:%d: the word %q is not in the voice: %s", p.Name, l.No, m, strings.TrimSpace(l.Text)))
		}
		if p.Name == plansPage {
			continue
		}
		for _, s := range sentenceRE.FindAllString(l.Text, -1) {
			if interpretRE.MatchString(s) && !negationRE.MatchString(s) {
				out = append(out, fmt.Sprintf("%s:%d: promises an interpretation: %s", p.Name, l.No, strings.TrimSpace(s)))
			}
		}
	}
	return out
}

func TestPublicPagesKeepTheVoice(t *testing.T) {
	pages := publicPages(t)
	for _, p := range pages {
		for _, msg := range voiceProblems(p) {
			t.Error(msg)
		}
	}
	for _, c := range []struct{ line string }{
		{"It is simply a chart."},
		{"Try it just once."},
		{"An easy start."},
		{"A powerful list."},
		{"A seamless step."},
		{"Everything at your fingertips."},
		{"Sextile will interpret your chart for you."},
		{"Sextile gives an interpretation of every aspect."},
	} {
		p := publicPage{Name: "x.md", Text: "# X\n\n" + c.line + "\n", First: 1}
		if got := voiceProblems(p); len(got) == 0 || !strings.HasPrefix(got[0], "x.md:3:") {
			t.Errorf("control: %q was not named at line 3: %q", c.line, got)
		}
	}
	for _, ok := range []string{"It does not interpret a chart.", "Sextile adds no interpretation.", "Never interpreted."} {
		if got := voiceProblems(publicPage{Name: "x.md", Text: ok, First: 1}); len(got) != 0 {
			t.Errorf("control: %q is refused: %q", ok, got)
		}
	}
	plan := "# X\n\n- Interpretation as a study aid. Short texts beside the numbers.\n"
	if got := voiceProblems(publicPage{Name: plansPage, Text: plan, First: 1}); len(got) != 0 {
		t.Errorf("control: the roadmap may plan an interpretation layer, got %q", got)
	}
	if got := voiceProblems(publicPage{Name: "README.md", Text: plan, First: 1}); len(got) == 0 {
		t.Errorf("control: the README promised an interpretation and nothing was named")
	}
}

// ---------------------------------------------------------------------------
// F1, F3, F9: the shape of the README and the reference

func pageNamed(t *testing.T, name string) publicPage {
	t.Helper()
	for _, p := range publicPages(t) {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("%s is not a public page", name)
	return publicPage{}
}

const readmeFirstLine = "A terminal astrology suite."

var plainWordsBeforePicture = regexp.MustCompile("(?i)`|docker|\\bastrolog\\b|^\\s*\\$ ")

// readmeOpeningProblems: the README opens with its first line right after the
// title, and before its first picture names no command, key, Docker or Astrolog.
func readmeOpeningProblems(text string) []string {
	var out []string
	lines := strings.Split(text, "\n")
	var body []string
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) != "" {
			body = append(body, l)
		}
	}
	if len(body) < 1 || body[0] != readmeFirstLine {
		out = append(out, "the README does not open with its first line right after the title")
	}
	for i, l := range lines {
		if strings.HasPrefix(l, "![") {
			break
		}
		if i == 0 {
			continue
		}
		if m := plainWordsBeforePicture.FindString(l); m != "" {
			out = append(out, fmt.Sprintf("README.md:%d: names %q before the first picture: %s", i+1, strings.TrimSpace(m), l))
		}
	}
	if !strings.Contains(text, "\n![") {
		out = append(out, "the README has no picture")
	}
	return out
}

func TestReadmeOpensInPlainWords(t *testing.T) {
	text := pageNamed(t, "README.md").Text
	for _, msg := range readmeOpeningProblems(text) {
		t.Error(msg)
	}
	// Controls: a key, Docker or Astrolog before the picture, and a changed
	// first line, each named for its own reason.
	for _, c := range []struct{ from, to, named string }{
		{"\n![", "\nPress `p` to begin.\n\n![", "before the first picture"},
		{"\n![", "\nIt runs in Docker.\n\n![", "before the first picture"},
		{"\n![", "\nAstrolog draws it.\n\n![", "before the first picture"},
		{readmeFirstLine, "An astrology suite.", "first line"},
	} {
		if !strings.Contains(text, c.from) {
			t.Fatalf("control: the README has no %q", c.from)
		}
		named := false
		for _, msg := range readmeOpeningProblems(strings.Replace(text, c.from, c.to, 1)) {
			named = named || strings.Contains(msg, c.named)
		}
		if !named {
			t.Errorf("control: replacing %q with %q was not named as %q", c.from, c.to, c.named)
		}
	}
}

// sectionOf is the text of the "## heading" section: its lines up to the next
// "## ", with the number of the line the heading is on.
func sectionOf(text, heading string) (string, int) {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if l != "## "+heading {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(lines[j], "## ") {
				end = j
				break
			}
		}
		return strings.Join(lines[i+1:end], "\n"), i + 1
	}
	return "", 0
}

// readmeNeedsProblems: the box "Before you start" stands before the first
// install line and says what a reader needs to know first.
func readmeNeedsProblems(text string) []string {
	var out []string
	box, at := sectionOf(text, "Before you start")
	if at == 0 {
		return []string{"the README has no section \"Before you start\""}
	}
	install := strings.Index(text, "curl -fsSL")
	if i := strings.Index(text, "## Before you start"); install >= 0 && i > install {
		out = append(out, "the box \"Before you start\" comes after the first install line")
	}
	low := strings.ToLower(box)
	for _, want := range []struct{ re, says string }{
		{`terminal window`, "that Sextile runs in a terminal window"},
		{`docker`, "that it needs Docker"},
		{`tested on linux`, "which systems are tested"},
		{`windows`, "which systems are tested (Windows)"},
		{`should work`, "that a Mac is expected to work"},
		{`mac is not tested|not tested[^.]*mac`, "that a Mac is not tested"},
		{`chart files? (are|stay)|never writes over a chart|never writes over`, "what Sextile does to chart files"},
	} {
		if !regexp.MustCompile(want.re).MatchString(low) {
			out = append(out, fmt.Sprintf("README.md:%d: the box does not say %s", at, want.says))
		}
	}
	return out
}

func TestReadmeSaysWhatItNeedsFirst(t *testing.T) {
	text := pageNamed(t, "README.md").Text
	for _, msg := range readmeNeedsProblems(text) {
		t.Error(msg)
	}
	for _, c := range []struct {
		name string
		edit func(string) string
	}{
		{"Docker dropped from the box", func(s string) string {
			box, _ := sectionOf(s, "Before you start")
			return strings.Replace(s, box, strings.ReplaceAll(box, "Docker", "A helper"), 1)
		}},
		{"the Mac wording dropped", func(s string) string {
			return strings.Replace(s, "It should work there with Docker.", "It runs there.", 1)
		}},
		{"the box after the install line", func(s string) string {
			return strings.Replace(s, "## Before you start", "```bash\ncurl -fsSL x\n```\n\n## Before you start", 1)
		}},
	} {
		got := readmeNeedsProblems(c.edit(text))
		if len(got) == 0 {
			t.Errorf("control: %s was not named", c.name)
		}
	}
}

// referenceOpeningProblems: the reference opens with an index of tasks, each a
// link, then the keys, then settings and files, then the limits.
func referenceOpeningProblems(text string) []string {
	var out []string
	lines := strings.Split(text, "\n")
	var h2 []string
	var h2At []int
	fenced := false
	for i, l := range lines {
		if strings.HasPrefix(l, "```") {
			fenced = !fenced
		}
		if !fenced && strings.HasPrefix(l, "## ") {
			h2 = append(h2, strings.TrimPrefix(l, "## "))
			h2At = append(h2At, i+1)
		}
	}
	if len(h2) == 0 || h2[0] != "I want to ..." {
		return []string{"docs/REFERENCE.md: the first section is not \"I want to ...\""}
	}
	index, _ := sectionOf(text, "I want to ...")
	entries := 0
	linkItem := regexp.MustCompile(`^- \[[^\]]+\]\(#[a-z0-9-]+\)$`)
	for i, l := range strings.Split(index, "\n") {
		if strings.HasPrefix(l, "- ") {
			entries++
			if !linkItem.MatchString(l) {
				out = append(out, fmt.Sprintf("docs/REFERENCE.md:%d: an entry of the index is not a link: %s", h2At[0]+1+i, l))
			}
		}
	}
	if entries < 5 {
		out = append(out, fmt.Sprintf("docs/REFERENCE.md: the index has %d entries", entries))
	}
	order := func(name string) int { return slices.Index(h2, name) }
	for _, n := range []string{"Keys", "Settings", "Where files are", "Limits"} {
		if order(n) < 0 {
			out = append(out, fmt.Sprintf("docs/REFERENCE.md: no section %q", n))
		}
	}
	if len(out) == 0 && !(order("Keys") < order("Settings") && order("Settings") < order("Limits") && order("Where files are") < order("Limits")) {
		out = append(out, fmt.Sprintf("docs/REFERENCE.md: the sections are not in the order keys, settings and files, limits: %q", h2))
	}
	if order("Limits") >= 0 && order("Limits") < 2 {
		out = append(out, "docs/REFERENCE.md: the page opens with the limits")
	}
	return out
}

func TestReferenceOpensWithTasks(t *testing.T) {
	text := pageNamed(t, "docs/REFERENCE.md").Text
	for _, msg := range referenceOpeningProblems(text) {
		t.Error(msg)
	}
	for _, c := range []struct{ name, from, to string }{
		{"a plain entry in the index", "- [open a chart](#open-a-chart)", "- open a chart"},
		{"limits first", "## I want to ...", "## Limits\n\n## I want to ..."},
		{"the index dropped", "## I want to ...", "## Index"},
	} {
		if !strings.Contains(text, c.from) {
			t.Fatalf("control: the REFERENCE has no %q", c.from)
		}
		if len(referenceOpeningProblems(strings.Replace(text, c.from, c.to, 1))) == 0 {
			t.Errorf("control: %s was not named", c.name)
		}
	}
}

// A removal note names a key the program no longer has: "`N` is gone" and
// "`,` `n` is gone" are true sentences, not key claims. The same key in any
// other sentence is still named.
func TestPublicPagesMayNameARemovedKeyAsGone(t *testing.T) {
	reg := buildDocKeyRegistry(t)
	gone := publicPage{Name: "CHANGELOG.md", First: 1, Text: "- `N` is gone.\n- A new session is a row. `,` `n` is gone.\n- `Ctrl+H` is gone.\n"}
	if got := keyProblems(gone, reg); len(got) != 0 {
		t.Errorf("removal notes were read as key claims: %q", got)
	}
	claim := publicPage{Name: "CHANGELOG.md", First: 1, Text: "- `N` goes back.\n- `,` `n` starts a new session.\n- `Ctrl+H` opens the houses.\n"}
	got := keyProblems(claim, reg)
	for _, want := range []string{"CHANGELOG.md:1:", "CHANGELOG.md:2:", "CHANGELOG.md:3:"} {
		if !slices.ContainsFunc(got, func(s string) bool { return strings.HasPrefix(s, want) }) {
			t.Errorf("control: the claim at %s was not named: %q", want, got)
		}
	}
}
