package templates

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/cruiserxyz/sextile/internal/rules"
)

// Line is one rendered line of a tab, in the three columns every line has
// (the specification R1): a label from column 0, a value
// from the common value column, and an aside — the criterion of a verdict or
// a qualifier — dimmed at the right. A Blank line separates two groups.
type Line struct {
	Label, Value, Aside string
	Blank               bool
}

// Rendered is a template's output: its lines, and the rule rows whose
// verdicts they show (for the Log, P10).
type Rendered struct {
	Lines    []Line
	RuleRows []string
	Slots    []SlotTrace
}

// SlotTrace is what one expression slot chose (T14): the template line and
// the slot's place on it (1-based, counting every slot of the line), and the
// term id or fact key it evaluated to. Never a fact's value, never an
// argument: the Log shows which branch was taken and nothing more.
type SlotTrace struct {
	Line, Slot int
	Chose      string
}

// Render fills the template for one subject (a body id the template lists)
// from each section's facts. A line naming an absent fact is omitted (and so
// is a rule line whose rule omits itself); a fact the section does not have
// at all is an error, because only "not applicable" may omit a line (P5):
// a failed or incomplete computation never reaches here. Blank template lines
// separate groups: the output carries exactly one blank between groups and
// none at either end (R7).
func Render(t Template, subject string, sections map[string]Section, set rules.Set) (Rendered, error) {
	var out Rendered
	for _, sec := range t.sections {
		facts, ok := sections[sec.name]
		if !ok {
			return out, fmt.Errorf("templates: %s: no facts for section @%s", t.ID, sec.name)
		}
		for _, l := range sec.lines {
			if l.blank {
				if n := len(out.Lines); n > 0 && !out.Lines[n-1].Blank {
					out.Lines = append(out.Lines, Line{Blank: true})
				}
				continue
			}
			lines, rows, slots, err := renderLine(t, l, subject, facts, set)
			if err != nil {
				return out, err
			}
			out.Lines = append(out.Lines, lines...)
			out.RuleRows = append(out.RuleRows, rows...)
			out.Slots = append(out.Slots, slots...)
		}
	}
	for n := len(out.Lines); n > 0 && out.Lines[n-1].Blank; n = len(out.Lines) {
		out.Lines = out.Lines[:n-1]
	}
	return out, nil
}

func renderLine(t Template, l line, subject string, facts Section, set rules.Set) ([]Line, []string, []SlotTrace, error) {
	fail := func(format string, args ...any) error {
		return fmt.Errorf("templates: %s:%d: %s", t.File, l.n, fmt.Sprintf(format, args...))
	}
	// Every fact the line names must be there, absent or not: a missing one
	// is a defect upstream, never "not applicable". Then any absent fact
	// omits the whole line.
	got := map[string]Fact{}
	absent := false
	for _, p := range l.parts {
		if p.expr != nil {
			for _, k := range p.expr.refs {
				if _, ok := facts.Fact(k); !ok {
					return nil, nil, nil, fail("no fact %s for this section", k)
				}
			}
		}
		if p.fact == "" {
			continue
		}
		f, ok := facts.Fact(p.fact)
		if !ok {
			return nil, nil, nil, fail("no fact %s for this section", p.fact)
		}
		absent = absent || f.Absent
		got[p.fact] = f
	}
	if absent {
		return nil, nil, nil, nil
	}
	// An expression slot is evaluated left to right; reaching an absent fact
	// omits the line (T8), and what it chose is traced for the Log (T14).
	var traces []SlotTrace
	resolved := l.parts
	for i, p := range l.parts {
		if p.expr == nil {
			continue
		}
		if &resolved[0] == &l.parts[0] {
			resolved = append([]part(nil), l.parts...)
		}
		v, ok := p.expr.eval(facts)
		if !ok {
			// The line is omitted, but the slots already evaluated on it ran:
			// the Log still says which branch they took.
			return nil, nil, traces, nil
		}
		shown, chose := "", v.term
		if v.key != "" {
			chose = v.key
			f, _ := facts.Fact(v.key)
			shown = f.Text // a value fact: a category fact never reaches here
		} else {
			shown = set.Terms[v.term].Display
		}
		resolved[i] = part{resolved: true, shown: shown}
		traces = append(traces, SlotTrace{Line: l.n, Slot: p.expr.n, Chose: chose})
	}
	l.parts = resolved
	if len(l.parts) == 1 && l.parts[0].rule != "" {
		lines, rows, err := renderRule(set.Rules[l.parts[0].rule], subject, facts, set, fail)
		return lines, rows, nil, err
	}
	parts := l.parts
	var ln Line
	// The leading term is the label (R2) when a fact follows it.
	if lbl := labelTerm(parts, set); lbl != "" {
		ln.Label = lbl
		parts = parts[1:]
	}
	// Trailing parenthesised terms are the aside (R9).
	parts, ln.Aside = splitAside(parts, set)
	var b strings.Builder
	var items []string
	for _, p := range parts {
		switch {
		case p.literal != "":
			b.WriteString(p.literal)
		case p.resolved:
			b.WriteString(p.shown)
		case p.term != "":
			b.WriteString(set.Terms[p.term].Display)
		case p.fact != "":
			f := got[p.fact]
			switch f.Kind {
			case KindCategory:
				term, ok := set.Terms[f.Term]
				if !ok {
					return nil, nil, nil, fail("fact %s names term %q, which rules/terms.yaml does not hold", p.fact, f.Term)
				}
				b.WriteString(term.Display)
			case KindInstant:
				// An instant may carry its kind as a term (a station's direction,
				// C4); the line shows the instant alone. The kind's words would
				// cut the line at the Details panel's width (R10), and the motion
				// line above the stations already implies which station was last.
				b.WriteString(f.Text)
			case KindList:
				// At most the count the registry declares, whatever the
				// producer handed over (P6).
				items = f.Items
				if len(items) > p.max {
					items = items[:p.max]
				}
			default:
				b.WriteString(f.Text)
			}
		}
	}
	ln.Value = strings.TrimSpace(b.String())
	// A list's items take the value column, one per line, the first on the
	// label's own line when the line has no other value (R1, R6).
	lines := []Line{ln}
	if len(items) > 0 && ln.Value == "" {
		lines[0].Value = items[0]
		items = items[1:]
	}
	for _, it := range items {
		lines = append(lines, Line{Value: it})
	}
	return lines, nil, traces, nil
}

// labelTerm is the display text of a line's leading term when a fact
// follows it on the line (R2); "" when the line has no label.
func labelTerm(parts []part, set rules.Set) string {
	if len(parts) < 2 || parts[0].term == "" {
		return ""
	}
	for _, p := range parts[1:] {
		if p.fact != "" || p.expr != nil || p.resolved {
			return set.Terms[parts[0].term].Display
		}
	}
	return ""
}

// splitAside takes the trailing `(… {term:x} …)` off a line's parts and
// returns the terms' display text as the aside (R9). Anything else leaves
// the parts alone.
func splitAside(parts []part, set rules.Set) ([]part, string) {
	n := len(parts)
	if n < 3 || parts[n-1].literal == "" || strings.TrimSpace(parts[n-1].literal) != ")" {
		return parts, ""
	}
	var terms []string
	i := n - 2
	for ; i >= 0 && parts[i].term != ""; i-- {
		terms = append([]string{set.Terms[parts[i].term].Display}, terms...)
	}
	if i < 0 || len(terms) == 0 || parts[i].literal == "" || !strings.HasSuffix(strings.TrimSpace(parts[i].literal), "(") {
		return parts, ""
	}
	open := strings.TrimSuffix(strings.TrimSpace(parts[i].literal), "(")
	kept := append([]part(nil), parts[:i]...)
	if open != "" {
		kept = append(kept, part{literal: open})
	}
	return kept, strings.Join(terms, " ")
}

// renderRule asks the rule's question once — the verdict term is the label —
// and answers it once per row that applies to the subject, in the rules
// file's order: `yes` when the row's verdict holds, `no` otherwise, the
// condition note when the row's condition failed, and the criterion
// (`within <threshold>`) as the aside (R3). No author, work or page reaches
// the screen (R4): the row's id goes to the Log. The template cannot change
// this form.
func renderRule(r rules.Rule, subject string, facts Section, set rules.Set, fail func(string, ...any) error) ([]Line, []string, error) {
	f, ok := facts.Fact(r.Measure)
	if !ok {
		return nil, nil, fail("no fact %s for rule %s", r.Measure, r.ID)
	}
	if f.Absent {
		return nil, nil, nil
	}
	verdicts, err := rules.Evaluate(r, subject, sectionFacts{facts})
	if err != nil {
		return nil, nil, fail("%v", err)
	}
	var lines []Line
	var rows []string
	for i, v := range verdicts {
		ln := Line{Value: set.Terms["no"].Display}
		if v.Term == r.Verdict {
			ln.Value = set.Terms["yes"].Display
		}
		if v.Condition {
			ln.Value += " " + set.Terms["condition"].Display
		}
		ln.Aside = set.Terms["within"].Display + " " + v.Row.ThresholdText()
		if v.Row.Grade == "snippet" || v.Row.Grade == "unverified" {
			ln.Aside += " " + v.Row.Grade
		}
		if i == 0 {
			ln.Label = set.Terms[r.Verdict].Display
		}
		lines = append(lines, ln)
		rows = append(rows, v.Row.ID)
	}
	return lines, rows, nil
}

// LabelWidth is the label column: every value starts after it and one gap.
const LabelWidth = 17

const labelGap = 2

// Laid is one line laid out for a panel of a given width: Main is the label
// and the value in their columns, Aside the dim text that ends at the right
// edge (empty when the line has none or it did not fit).
type Laid struct {
	Main, Aside string
}

// Layout places lines at width (R1, R10): the label from column 0, the value
// from column LabelWidth+2, the aside right-aligned with at least one space
// before it. A line too long for the width drops its aside first, then cuts
// its value at a word boundary with `…` — never the label, never inside a
// number. A blank line lays out empty.
func Layout(lines []Line, width int) []Laid {
	out := make([]Laid, 0, len(lines))
	for _, l := range lines {
		if l.Blank {
			out = append(out, Laid{})
			continue
		}
		main := l.Label
		if l.Value != "" {
			main = pad(l.Label, LabelWidth) + strings.Repeat(" ", labelGap) + l.Value
		}
		aside := l.Aside
		if aside != "" && utf8.RuneCountInString(main)+1+utf8.RuneCountInString(aside) > width {
			aside = ""
		}
		if utf8.RuneCountInString(main) > width {
			main = cutValue(l.Label, l.Value, width)
		}
		if aside != "" {
			main = pad(main, width-utf8.RuneCountInString(aside))
		}
		out = append(out, Laid{Main: main, Aside: aside})
	}
	return out
}

func pad(s string, w int) string {
	if n := utf8.RuneCountInString(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

// cutValue shortens a value so label, gap, value and `…` fit in width, at
// the last word boundary that fits; a value with no boundary that fits is
// `…` alone, so no number is ever cut in two.
func cutValue(label, value string, width int) string {
	head := pad(label, LabelWidth) + strings.Repeat(" ", labelGap)
	room := width - utf8.RuneCountInString(head) - 1
	if room < 0 {
		room = 0
	}
	cut := ""
	for i, r := range value {
		if r != ' ' {
			continue
		}
		if utf8.RuneCountInString(value[:i]) <= room {
			cut = value[:i]
		} else {
			break
		}
	}
	return head + cut + "…"
}

// sectionFacts adapts a section to what a rule reads.
type sectionFacts struct{ s Section }

func (f sectionFacts) Value(key string) (float64, bool) {
	v, ok := f.s.Fact(key)
	if !ok || v.Absent || v.Kind != KindValue {
		return 0, false
	}
	return v.Num, true
}

func (f sectionFacts) Category(key string) (string, bool) {
	v, ok := f.s.Fact(key)
	if !ok || v.Absent || v.Kind != KindCategory {
		return "", false
	}
	return v.Term, true
}

// ForBody is the shipped template that lists the body, if any.
func ForBody(ts []Template, body string) (Template, bool) {
	for _, t := range ts {
		for _, b := range t.Bodies {
			if b == body {
				return t, true
			}
		}
	}
	return Template{}, false
}
