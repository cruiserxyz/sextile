// Package templates turns a shipped body template, the facts Go obtained from
// Astrolog and the cited rules into the lines of a Details tab
// (the specification). It is Presentation and nothing else:
// a template arranges named facts, terms and rule verdicts; it holds no words
// of its own (every letter and digit arrives through a slot), and it cannot
// reorder or drop a rule's rows. A template may SELECT between cited terms by
// a category fact; it never judges: no threshold, no number, no uncited word
// (the specification). A line whose fact is absent is omitted.
package templates

import (
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/cruiserxyz/sextile/internal/rules"
)

//go:embed *.md allowlist.txt
var files embed.FS

// Kind is what a fact is, which decides how a slot shows it.
type Kind int

const (
	KindValue Kind = iota + 1
	KindInstant
	KindSpan
	KindList
	KindCategory
)

// Registry is what a template may name: the kind of every fact key Go can
// supply, and for a list the most items it shows (P6). A slot naming a key
// it does not know fails the load, and so does a list with no maximum.
type Registry interface {
	FactKind(key string) (Kind, bool)
	FactMax(key string) int
}

// Fact is one fact for one section. Text is a value's or instant's display
// form, Term a category's term id, Items a list's entries (newest first),
// Num the value a rule judges, as parsed from Astrolog's output.
type Fact struct {
	Kind   Kind
	Absent bool // not applicable here: the line naming it is omitted
	Text   string
	Term   string
	Items  []string
	Num    float64
}

// Section is the facts of one moment a template binds (@now, @base).
type Section interface {
	Fact(key string) (Fact, bool)
}

// Template is one loaded template.
type Template struct {
	ID, Tab, File string
	Bodies        []string
	sections      []section
}

type section struct {
	name  string // "now" or "base"
	lines []line
}

type line struct {
	n     int // 1-based line in the file
	parts []part
	blank bool // a blank template line: a group separator (R7)
}

// part is literal text or one slot.
type part struct {
	literal string
	term    string // {term:x}
	rule    string // {rule:x}
	fact    string // {fact.key}
	max     int    // a list fact's most items

	expr     *exprSlot // an expression slot, evaluated at render
	resolved bool      // an expression slot after evaluation: shown is its text
	shown    string
}

var (
	slotRE    = regexp.MustCompile(`\{([^{}]*)\}`)
	factKeyRE = regexp.MustCompile(`^[a-z][a-z0-9-]*(\.[a-z0-9][a-z0-9-]*)+$`)
	idRE      = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// Shipped loads every embedded template against the registry and the rules.
func Shipped(reg Registry, set rules.Set) ([]Template, error) {
	return loadAll(files, reg, set)
}

// LoadFS loads the templates of any file system (with its allowlist.txt):
// a test's fixture set, where the shipped one is not the one under test.
func LoadFS(fsys fs.FS, reg Registry, set rules.Set) ([]Template, error) {
	return loadAll(fsys, reg, set)
}

func loadAll(fsys fs.FS, reg Registry, set rules.Set) ([]Template, error) {
	return loadAllWith(fsys, reg, set, DefaultHelpers())
}

func loadAllWith(fsys fs.FS, reg Registry, set rules.Set, helpers HelperRegistry) ([]Template, error) {
	allow, err := allowlist(fsys)
	if err != nil {
		return nil, err
	}
	names, err := fs.Glob(fsys, "*.md")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	var out []Template
	ids := map[string]bool{}
	for _, name := range names {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		t, err := parse(name, string(b), reg, set, allow, helpers)
		if err != nil {
			return nil, err
		}
		if ids[t.ID] {
			return nil, fmt.Errorf("templates: %s: id %s is used twice", name, t.ID)
		}
		ids[t.ID] = true
		out = append(out, t)
	}
	return out, nil
}

// allowlist reads allowlist.txt: one character per line, comments aside.
func allowlist(fsys fs.FS) (map[rune]bool, error) {
	b, err := fs.ReadFile(fsys, "allowlist.txt")
	if err != nil {
		return nil, fmt.Errorf("templates: allowlist.txt: %w", err)
	}
	allow := map[rune]bool{' ': true}
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		r := []rune(l)
		if len(r) != 1 {
			return nil, fmt.Errorf("templates: allowlist.txt: %q is not one character", l)
		}
		allow[r[0]] = true
	}
	return allow, nil
}

func parse(name, src string, reg Registry, set rules.Set, allow map[rune]bool, helpers HelperRegistry) (Template, error) {
	t := Template{File: name}
	format := 0
	fail := func(n int, format string, args ...any) error {
		return fmt.Errorf("templates: %s:%d: %s", name, n, fmt.Sprintf(format, args...))
	}
	// The source is in memory, so it is split, not scanned: a scanner's token
	// limit would silently drop an oversized line (T13).
	lines := strings.Split(src, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	if len(lines) == 0 || lines[0] != "---" {
		return t, fail(1, "a template starts with front matter (---)")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return t, fail(1, "front matter is not closed (---)")
	}
	seen := map[string]int{}
	for i := 1; i < end; i++ {
		key, val, ok := strings.Cut(lines[i], ":")
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		if !ok || key == "" {
			return t, fail(i+1, "%q is not `key: value`", lines[i])
		}
		if _, dup := seen[key]; dup {
			return t, fail(i+1, "%q said twice", key)
		}
		seen[key] = i + 1
		switch key {
		case "format":
			switch val {
			case "1":
				format = 1
			case "2":
				format = 2
			default:
				return t, fail(i+1, "format %q: only 1 and 2 are known", val)
			}
		case "origin":
			if val != "shipped" {
				return t, fail(i+1, "origin %q: only shipped is built", val)
			}
		case "id":
			if !idRE.MatchString(val) {
				return t, fail(i+1, "id %q is not lower-case words joined by -", val)
			}
			t.ID = val
		case "tab":
			if val == "" {
				return t, fail(i+1, "tab is empty")
			}
			t.Tab = val
		case "subject":
			bodies, err := parseSubject(val)
			if err != nil {
				return t, fail(i+1, "subject: %v", err)
			}
			t.Bodies = bodies
		default:
			return t, fail(i+1, "unknown front-matter key %q", key)
		}
	}
	for _, k := range []string{"format", "origin", "id", "tab", "subject"} {
		if _, ok := seen[k]; !ok {
			return t, fail(1, "front matter lacks %q", k)
		}
	}
	ctx := &loadCtx{reg: reg, set: set, allow: allow, helpers: helpers, bodies: t.Bodies, format: format, reserved: reservedTerms(set)}
	var cur *section
	for i := end + 1; i < len(lines); i++ {
		n, text := i+1, lines[i]
		switch strings.TrimRight(text, " ") {
		case "":
			// A blank line inside a section separates groups; one before
			// the section's first line is nothing.
			if cur != nil && len(cur.lines) > 0 {
				cur.lines = append(cur.lines, line{n: n, blank: true})
			}
			continue
		case "## @now", "## @base":
			t.sections = append(t.sections, section{name: strings.TrimPrefix(strings.TrimRight(text, " "), "## @")})
			cur = &t.sections[len(t.sections)-1]
			continue
		}
		if strings.HasPrefix(text, "#") {
			return t, fail(n, "%q: the only headings are ## @now and ## @base", text)
		}
		if cur == nil {
			return t, fail(n, "a line before the first section marker (## @now or ## @base)")
		}
		l, err := parseLine(text, ctx)
		if err != nil {
			return t, fail(n, "%v", err)
		}
		l.n = n
		cur.lines = append(cur.lines, l)
	}
	if len(t.sections) == 0 {
		return t, fail(end+1, "no section (## @now or ## @base)")
	}
	return t, nil
}

// parseSubject reads `{kind: body, class: <class>, bodies: [a, b]}`.
func parseSubject(val string) ([]string, error) {
	inner, ok := strings.CutPrefix(val, "{")
	if ok {
		inner, ok = strings.CutSuffix(inner, "}")
	}
	if !ok {
		return nil, fmt.Errorf("%q is not {kind: body, class: ..., bodies: [...]}", val)
	}
	_, list, ok := strings.Cut(inner, "bodies:")
	if !ok {
		return nil, fmt.Errorf("lacks bodies")
	}
	head, _, _ := strings.Cut(inner, "bodies:")
	fields := map[string]string{}
	for _, f := range strings.Split(head, ",") {
		if strings.TrimSpace(f) == "" {
			continue
		}
		k, v, ok := strings.Cut(f, ":")
		if !ok {
			return nil, fmt.Errorf("%q is not key: value", f)
		}
		k = strings.TrimSpace(k)
		if _, dup := fields[k]; dup {
			return nil, fmt.Errorf("%q said twice", k)
		}
		fields[k] = strings.TrimSpace(v)
	}
	if fields["kind"] != "body" || fields["class"] == "" || len(fields) != 2 {
		return nil, fmt.Errorf("needs exactly kind: body and a class before bodies, got %v", fields)
	}
	list = strings.TrimSpace(list)
	inner, ok = strings.CutPrefix(list, "[")
	if ok {
		inner, ok = strings.CutSuffix(inner, "]")
	}
	if !ok {
		return nil, fmt.Errorf("bodies %q is not [a, b]", list)
	}
	var bodies []string
	for _, b := range strings.Split(inner, ",") {
		b = strings.TrimSpace(b)
		if !idRE.MatchString(b) {
			return nil, fmt.Errorf("body %q is not a lower-case id", b)
		}
		bodies = append(bodies, b)
	}
	return bodies, nil
}

func parseLine(text string, ctx *loadCtx) (line, error) {
	reg, set, allow := ctx.reg, ctx.set, ctx.allow
	var l line
	nExpr := 0
	rest := text
	for {
		loc := slotRE.FindStringIndex(rest)
		lit := rest
		if loc != nil {
			lit = rest[:loc[0]]
		}
		if i := strings.IndexAny(lit, "{}"); i >= 0 {
			return l, fmt.Errorf("a stray or unterminated brace `%c`", lit[i])
		}
		for _, r := range lit {
			if !allow[r] {
				return l, fmt.Errorf("%q is not allowed outside a slot (templates/allowlist.txt): every word arrives through {term:}, {rule:} or a fact", string(r))
			}
		}
		if lit != "" {
			l.parts = append(l.parts, part{literal: lit})
		}
		if loc == nil {
			break
		}
		slot := rest[loc[0]+1 : loc[1]-1]
		p, err := parseSlot(slot, ctx, nExpr+1)
		if err != nil {
			return l, err
		}
		if p.expr != nil {
			nExpr++
			if ctx.format < 2 {
				return l, fmt.Errorf("{%s}: an expression slot needs `format: 2` in the front matter", slot)
			}
		}
		l.parts = append(l.parts, p)
		rest = rest[loc[1]:]
	}
	if strings.ContainsAny(text, "{}") && len(l.parts) == 0 {
		return l, fmt.Errorf("a stray brace")
	}
	for _, p := range l.parts {
		if p.literal != "" && strings.ContainsAny(p.literal, "{}") {
			return l, fmt.Errorf("a stray brace")
		}
	}
	nRules, lists := 0, 0
	for _, p := range l.parts {
		if p.rule != "" {
			nRules++
		}
		if p.fact != "" {
			if k, _ := reg.FactKind(p.fact); k == KindList {
				lists++
			}
		}
	}
	// A rule's verdicts and a list's items take whole lines of their own,
	// in an order the template cannot change.
	if nRules > 0 && len(l.parts) != 1 {
		return l, fmt.Errorf("a {rule:} slot stands alone on its line")
	}
	if lists > 1 {
		return l, fmt.Errorf("one list fact per line")
	}
	// The label column is LabelWidth wide, and a label that does not fit
	// fails here rather than on screen (R11).
	label := labelTerm(l.parts, set)
	if nRules == 1 {
		label = set.Terms[set.Rules[l.parts[0].rule].Verdict].Display
	}
	if n := utf8.RuneCountInString(label); n > LabelWidth {
		return l, fmt.Errorf("label %q is %d characters. The label column holds %d", label, n, LabelWidth)
	}
	return l, nil
}

func parseSlot(slot string, ctx *loadCtx, ordinal int) (part, error) {
	reg, set := ctx.reg, ctx.set
	switch {
	case strings.HasPrefix(slot, "term:"):
		id := strings.TrimPrefix(slot, "term:")
		if _, ok := set.Terms[id]; !ok {
			return part{}, fmt.Errorf("{term:%s}: no such term in rules/terms.yaml", id)
		}
		return part{term: id}, nil
	case strings.HasPrefix(slot, "rule:"):
		id := strings.TrimPrefix(slot, "rule:")
		if _, ok := set.Rules[id]; !ok {
			return part{}, fmt.Errorf("{rule:%s}: no such rule in rules/", id)
		}
		return part{rule: id}, nil
	case factKeyRE.MatchString(slot) && !namesMemberOfKey(slot, reg) && !hyphenSplitsAtKey(slot, reg):
		k, ok := reg.FactKind(slot)
		if !ok {
			return part{}, fmt.Errorf("{%s}: no such fact in the registry", slot)
		}
		p := part{fact: slot}
		if k == KindList {
			if p.max = reg.FactMax(slot); p.max < 1 {
				return part{}, fmt.Errorf("{%s}: a list fact with no maximum in the registry", slot)
			}
		}
		return p, nil
	}
	e, err := parseExpr(slot, ctx, ordinal)
	if err != nil {
		return part{}, fmt.Errorf("{%s}: %v", elide(slot), err)
	}
	return part{expr: e}, nil
}

// elide shortens a slot for an error message.
func elide(s string) string {
	if r := []rune(s); len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return s
}

// namesMemberOfKey: an unregistered key that extends a registered one is a
// member access, which only the expression parser names as such.
func namesMemberOfKey(key string, reg Registry) bool {
	if _, ok := reg.FactKind(key); ok {
		return false
	}
	for prefix := key; strings.Contains(prefix, "."); {
		prefix = prefix[:strings.LastIndex(prefix, ".")]
		if _, ok := reg.FactKind(prefix); ok {
			return true
		}
	}
	return false
}
