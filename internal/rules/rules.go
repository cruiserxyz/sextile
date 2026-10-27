// Package rules holds the cited rules and terms a body template shows
// (the specification): every word a template prints is a
// term with its citation and grade, and every verdict a rule row with its
// attribution, threshold and source. The package evaluates a rule on facts
// Go obtained from Astrolog; it computes no astrology of its own.
//
// It is the deliberate exception the design notes §12 records: cited
// rules live here rather than in internal/baked behind internal/astrolog,
// because routing them through astrolog would attribute them to Astrolog
// (operator and council, 2026-10-14).
package rules

import (
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed *.yaml
var files embed.FS

// Grades a term or row may carry. plain (ordinary English) needs no
// citation; every other grade needs one.
var grades = map[string]bool{
	"primary": true, "translation": true, "secondary": true, "snippet": true,
	"unverified": true, "convention": true, "plain": true,
}

// Term is one word a template may show.
type Term struct {
	ID, Display, Citation, Grade string
}

// Citation is where a rule row's figure comes from.
type Citation struct {
	Author, Work, Edition, Locator string
}

// Condition limits a row to a category fact equal to a term id.
type Condition struct {
	Fact, Equals string
}

// Row is one attribution of a rule.
type Row struct {
	ID, Label  string
	Threshold  float64
	Inclusive  bool
	Citation   Citation
	Grade      string
	Vault      string
	Bodies     []string // empty: every body
	Condition  *Condition
	thresholdS string
}

// ThresholdText is the threshold as the file wrote it: degrees, arc-minutes
// or both, as the source states it (17°, 8°30′, 16′).
func (r Row) ThresholdText() string { return r.thresholdS }

// thresholdRE is a threshold as a source states it: whole degrees with
// optional arc-minutes, or arc-minutes alone.
var thresholdRE = regexp.MustCompile(`^(?:(\d+)°(?:(\d{1,2})′)?|(\d+)′)$`)

// parseThreshold is the threshold in degrees.
func parseThreshold(s string) (float64, bool) {
	m := thresholdRE.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	var deg, min int
	if m[3] != "" {
		min, _ = strconv.Atoi(m[3])
	} else {
		deg, _ = strconv.Atoi(m[1])
		if m[2] != "" {
			min, _ = strconv.Atoi(m[2])
		}
	}
	if min > 59 {
		return 0, false
	}
	t := float64(deg) + float64(min)/60
	return t, t > 0
}

// Rule is a named rule and its rows, in file order.
type Rule struct {
	ID, Kind, Measure, Verdict, Otherwise string
	OmitWhenNoneHold                      bool
	Rows                                  []Row
}

// Set is every shipped term and rule.
type Set struct {
	Terms map[string]Term
	Rules map[string]Rule
}

// Load reads the embedded files.
func Load() (Set, error) { return load(files) }

// LoadFS reads a terms.yaml and rule files from any file system: a test's
// fixture, where the shipped set is not the one under test.
func LoadFS(fsys fs.FS) (Set, error) { return load(fsys) }

func load(fsys fs.FS) (Set, error) {
	set := Set{Terms: map[string]Term{}, Rules: map[string]Rule{}}
	names, err := fs.Glob(fsys, "*.yaml")
	if err != nil {
		return set, err
	}
	sort.Strings(names)
	for _, name := range names {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return set, err
		}
		root, err := parseYAML(string(b))
		if err != nil {
			return set, fmt.Errorf("rules: %s: %w", name, err)
		}
		if name == "terms.yaml" {
			err = readTerms(root, set.Terms)
		} else {
			err = readRule(root, set.Rules)
		}
		if err != nil {
			return set, fmt.Errorf("rules: %s: %w", name, err)
		}
	}
	for _, r := range set.Rules {
		for _, id := range []string{r.Verdict, r.Otherwise} {
			if _, ok := set.Terms[id]; !ok {
				return set, fmt.Errorf("rules: rule %s names term %q, which terms.yaml does not hold", r.ID, id)
			}
		}
		for _, row := range r.Rows {
			if row.Condition != nil {
				if _, ok := set.Terms[row.Condition.Equals]; !ok {
					return set, fmt.Errorf("rules: row %s's condition names term %q, which terms.yaml does not hold", row.ID, row.Condition.Equals)
				}
			}
		}
	}
	if _, ok := set.Terms["condition"]; !ok {
		return set, fmt.Errorf("rules: terms.yaml lacks the term condition, which a failed row condition shows")
	}
	return set, nil
}

// fields reads a map node's keys: required ones must be present scalars,
// optional ones may be absent, and any other key is an error.
type fieldReader struct {
	n    *node
	seen map[string]bool
	err  error
}

func readFields(n *node, what string) *fieldReader {
	r := &fieldReader{n: n, seen: map[string]bool{}}
	if !n.isMap() {
		r.err = fmt.Errorf("line %d: %s must be a map", n.line, what)
	}
	return r
}

func (r *fieldReader) scalar(key string, required bool) string {
	if r.err != nil {
		return ""
	}
	r.seen[key] = true
	v, ok := r.n.fields[key]
	if !ok {
		if required {
			r.err = fmt.Errorf("line %d: missing %q", r.n.line, key)
		}
		return ""
	}
	if v.scalar == nil || strings.TrimSpace(*v.scalar) == "" {
		r.err = fmt.Errorf("line %d: %q must be a non-empty scalar", v.line, key)
		return ""
	}
	return *v.scalar
}

func (r *fieldReader) node(key string, required bool) *node {
	if r.err != nil {
		return nil
	}
	r.seen[key] = true
	v, ok := r.n.fields[key]
	if !ok && required {
		r.err = fmt.Errorf("line %d: missing %q", r.n.line, key)
	}
	return v
}

func (r *fieldReader) yesNo(key string) bool {
	switch v := r.scalar(key, true); v {
	case "yes":
		return true
	case "no", "":
		return false
	default:
		if r.err == nil {
			r.err = fmt.Errorf("line %d: %q is yes or no, not %q", r.n.fields[key].line, key, v)
		}
	}
	return false
}

// done refuses a key nobody read.
func (r *fieldReader) done() error {
	if r.err != nil {
		return r.err
	}
	for _, k := range r.n.keys {
		if !r.seen[k] {
			return fmt.Errorf("line %d: unknown key %q", r.n.fields[k].line, k)
		}
	}
	return nil
}

func readTerms(root *node, into map[string]Term) error {
	if root.list == nil {
		return fmt.Errorf("terms.yaml is a list of terms")
	}
	for _, item := range root.list {
		f := readFields(item, "a term")
		t := Term{ID: f.scalar("id", true), Display: f.scalar("display", true), Citation: f.scalar("citation", false), Grade: f.scalar("grade", true)}
		if err := f.done(); err != nil {
			return err
		}
		if err := checkGrade(t.Grade, t.Citation, item.line); err != nil {
			return fmt.Errorf("term %s: %w", t.ID, err)
		}
		if _, dup := into[t.ID]; dup {
			return fmt.Errorf("line %d: term %s said twice", item.line, t.ID)
		}
		into[t.ID] = t
	}
	return nil
}

func checkGrade(grade, citation string, line int) error {
	switch {
	case !grades[grade]:
		return fmt.Errorf("line %d: grade %q is not one of primary, translation, secondary, snippet, unverified, convention, plain", line, grade)
	case grade != "plain" && citation == "":
		return fmt.Errorf("line %d: a %s grade needs a citation", line, grade)
	case grade == "plain" && citation != "":
		return fmt.Errorf("line %d: a plain term carries no citation", line)
	}
	return nil
}

func readRule(root *node, into map[string]Rule) error {
	f := readFields(root, "a rule file")
	r := Rule{
		ID: f.scalar("rule", true), Kind: f.scalar("kind", true), Measure: f.scalar("measure", true),
		Verdict: f.scalar("verdict", true), Otherwise: f.scalar("otherwise", true),
	}
	if v := f.scalar("omit_when_none_hold", false); v != "" {
		r.OmitWhenNoneHold = v == "yes"
		if v != "yes" && v != "no" {
			return fmt.Errorf("omit_when_none_hold is yes or no, not %q", v)
		}
	}
	rows := f.node("rows", true)
	if err := f.done(); err != nil {
		return err
	}
	if r.Kind != "within" {
		return fmt.Errorf("rule %s: kind %q is not built (only within, interval, table and composite are reserved)", r.ID, r.Kind)
	}
	if rows.list == nil || len(rows.list) == 0 {
		return fmt.Errorf("rule %s: rows must be a non-empty list", r.ID)
	}
	ids := map[string]bool{}
	for _, item := range rows.list {
		row, err := readRow(item, r.ID)
		if err != nil {
			return fmt.Errorf("rule %s: %w", r.ID, err)
		}
		if ids[row.ID] {
			return fmt.Errorf("rule %s: row %s said twice", r.ID, row.ID)
		}
		ids[row.ID] = true
		r.Rows = append(r.Rows, row)
	}
	if _, dup := into[r.ID]; dup {
		return fmt.Errorf("rule %s said twice", r.ID)
	}
	into[r.ID] = r
	return nil
}

func readRow(item *node, rule string) (Row, error) {
	f := readFields(item, "a row")
	row := Row{ID: f.scalar("id", true), Label: f.scalar("label", true), Grade: f.scalar("grade", true), Vault: f.scalar("vault", false)}
	row.thresholdS = f.scalar("threshold", true)
	row.Inclusive = f.yesNo("inclusive")
	cit := f.node("citation", true)
	bodies := f.node("bodies", false)
	cond := f.node("condition", false)
	if err := f.done(); err != nil {
		return row, err
	}
	if !strings.HasPrefix(row.ID, rule+"/") {
		return row, fmt.Errorf("line %d: row id %q is not %s/<attribution>", item.line, row.ID, rule)
	}
	t, ok := parseThreshold(row.thresholdS)
	if !ok {
		return row, fmt.Errorf("line %d: threshold %q is not degrees and arc-minutes as a source states them (17°, 8°30′, 16′)", item.line, row.thresholdS)
	}
	row.Threshold = t
	cf := readFields(cit, "citation")
	row.Citation = Citation{Author: cf.scalar("author", true), Work: cf.scalar("work", true), Edition: cf.scalar("edition", true), Locator: cf.scalar("locator", true)}
	if err := cf.done(); err != nil {
		return row, fmt.Errorf("row %s: %w", row.ID, err)
	}
	if err := checkGrade(row.Grade, row.Citation.Locator, item.line); err != nil {
		return row, fmt.Errorf("row %s: %w", row.ID, err)
	}
	// The screen is neutral (the specification R4): a row's id
	// and label name the criterion, never the author. The citation is the
	// attribution, and it stays in this file and the Log's row ids.
	if surname := authorSurname(row.Citation.Author); surname != "" {
		for _, s := range []string{row.ID, row.Label} {
			if strings.Contains(strings.ToLower(s), surname) {
				return row, fmt.Errorf("row %s: %q names the author (%s). Ids and labels name the criterion, the citation names the author", row.ID, s, row.Citation.Author)
			}
		}
	}
	if row.Grade == "plain" {
		return row, fmt.Errorf("row %s: a rule row cites a source. Plain is for terms", row.ID)
	}
	if bodies != nil {
		if bodies.list == nil || len(bodies.list) == 0 {
			return row, fmt.Errorf("row %s: bodies is a non-empty list", row.ID)
		}
		for _, b := range bodies.list {
			if b.scalar == nil {
				return row, fmt.Errorf("row %s: a body is a plain id, not a map", row.ID)
			}
			row.Bodies = append(row.Bodies, *b.scalar)
		}
	}
	if cond != nil {
		cf := readFields(cond, "condition")
		row.Condition = &Condition{Fact: cf.scalar("fact", true), Equals: cf.scalar("equals", true)}
		if err := cf.done(); err != nil {
			return row, fmt.Errorf("row %s: %w", row.ID, err)
		}
	}
	return row, nil
}

// authorSurname is the last word of a citation's author, lower-cased: the
// part a row id or label must not carry.
func authorSurname(author string) string {
	fields := strings.Fields(author)
	if len(fields) == 0 {
		return ""
	}
	return strings.ToLower(fields[len(fields)-1])
}
