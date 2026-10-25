package templates

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"unicode/utf8"

	"github.com/cruiserxyz/sextile/internal/rules"
)

// The reading contract (the specification): every line in
// three columns the template cannot move, the leading term as the label, a
// rule as one question with an answer per row, qualifiers in the aside,
// groups one blank apart, overflow that never cuts a label or a number, and
// no author on screen.

// R1 — the same slots in another order lay out in the same columns.
func TestLinesLayOutInThreeColumns(t *testing.T) {
	allow, _ := fsAllow()
	a := header + "## @now\n{term:elongation} {elongation.lon} {elongation.side} ({term:longitude})\n{rule:solar.beams}\n"
	b := header + "## @now\n{rule:solar.beams}\n{term:elongation} {elongation.side} {elongation.lon} ({term:longitude})\n"
	for name, src := range map[string]string{"a": a, "b": b} {
		ts, err := loadAll(fsMap(src, allow), reg, set(t))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		r, err := Render(ts[0], "venus", map[string]Section{"now": nowFacts(14.87, nil)}, set(t))
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range Layout(r.Lines, 54) {
			if l.Main == "" {
				continue
			}
			if utf8.RuneCountInString(l.Main)+utf8.RuneCountInString(l.Aside) != 54 && l.Aside != "" {
				t.Errorf("%s: aside not at the right edge: %q|%q", name, l.Main, l.Aside)
			}
			if rs := []rune(l.Main); len(rs) > LabelWidth+labelGap && rs[LabelWidth] != ' ' {
				t.Errorf("%s: the value column is not at %d: %q", name, LabelWidth+labelGap, l.Main)
			}
		}
	}
}

// R2 — a leading term followed by a fact is the label; a line with no
// leading term starts at the value column.
func TestLeadingTermIsTheLabel(t *testing.T) {
	allow, _ := fsAllow()
	ts, err := loadAll(fsMap(header+"## @now\n{term:elongation} {elongation.lon}\n{elongation.lon} {term:elongation}\n", allow), reg, set(t))
	if err != nil {
		t.Fatal(err)
	}
	r, err := Render(ts[0], "venus", map[string]Section{"now": nowFacts(14.87, nil)}, set(t))
	if err != nil {
		t.Fatal(err)
	}
	if r.Lines[0].Label != "elongation" || r.Lines[0].Value != "14°52′" {
		t.Errorf("leading term: %+v", r.Lines[0])
	}
	if r.Lines[1].Label != "" || r.Lines[1].Value != "14°52′ elongation" {
		t.Errorf("no leading term: %+v", r.Lines[1])
	}
	laid := Layout(r.Lines, 54)
	if !strings.HasPrefix(laid[1].Main, strings.Repeat(" ", LabelWidth+labelGap)+"14°52′") {
		t.Errorf("an unlabelled line does not start at the value column: %q", laid[1].Main)
	}
}

// R3 — the question once as the label, one answer per row: yes for the row
// whose verdict holds, no otherwise, the criterion as the aside.
func TestRuleRendersAsQuestionAndAnswers(t *testing.T) {
	ts, _ := Shipped(reg, set(t))
	tpl, _ := ForBody(ts, "venus")
	for _, tc := range []struct {
		elong        float64
		first, later string
	}{{14.87, "yes", "no"}, {11, "yes", "yes"}, {20, "no", "no"}} {
		r, err := Render(tpl, "venus", map[string]Section{"now": nowFacts(tc.elong, nil)}, set(t))
		if err != nil {
			t.Fatal(err)
		}
		i := 0
		for i < len(r.Lines) && r.Lines[i].Label != "under the beams" {
			i++
		}
		if i+1 >= len(r.Lines) {
			t.Fatalf("at %v°: no rule lines in %+v", tc.elong, r.Lines)
		}
		got := []Line{r.Lines[i], r.Lines[i+1]}
		want := []Line{{Label: "under the beams", Value: tc.first, Aside: "within 17°"}, {Value: tc.later, Aside: "within 12°"}}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("at %v°: got %+v, want %+v", tc.elong, got, want)
		}
	}
}

// R4 — no rendered line carries a row's label or any citation field.
func TestOutputIsNeutral(t *testing.T) {
	expressionOutputIsNeutral(t) // an expression slot is no way round it (T10)
	s := set(t)
	ts, _ := Shipped(reg, s)
	for _, tpl := range ts {
		r, err := Render(tpl, "venus", map[string]Section{"now": nowFacts(14.87, []string{"Mar 23, 2025   2°39′ Ari"})}, s)
		if err != nil {
			t.Fatal(err)
		}
		for _, rule := range s.Rules {
			for _, row := range rule.Rows {
				for _, word := range []string{row.Label, row.Citation.Author, row.Citation.Work, row.Citation.Edition, row.Citation.Locator} {
					for _, l := range r.Lines {
						if text := l.Label + " " + l.Value + " " + l.Aside; strings.Contains(strings.ToLower(text), strings.ToLower(word)) {
							t.Errorf("%s: %q reaches the screen in %+v", tpl.ID, word, l)
						}
					}
				}
			}
		}
	}
}

// R7 — blank template lines separate groups: exactly one blank between
// groups, none inside, none at either end, two in a row collapse.
func TestGroupsSeparatedByOneBlankLine(t *testing.T) {
	allow, _ := fsAllow()
	src := header + "## @now\n\n{term:elongation} {elongation.lon}\n\n\n{rule:solar.beams}\n{term:inner-conjunction} {synodic.conjunction.inferior.last5}\n\n"
	ts, err := loadAll(fsMap(src, allow), reg, set(t))
	if err != nil {
		t.Fatal(err)
	}
	r, err := Render(ts[0], "venus", map[string]Section{"now": nowFacts(14.87, []string{"a", "b"})}, set(t))
	if err != nil {
		t.Fatal(err)
	}
	var shape []string
	for _, l := range r.Lines {
		if l.Blank {
			shape = append(shape, "_")
		} else {
			shape = append(shape, "x")
		}
	}
	if got := strings.Join(shape, ""); got != "x_xxxx" {
		t.Errorf("groups %s, want x_xxxx", got)
	}
}

// R9 — trailing parenthesised terms go to the aside, not the value.
func TestQualifiersSitInTheThirdColumn(t *testing.T) {
	ts, _ := Shipped(reg, set(t))
	tpl, _ := ForBody(ts, "venus")
	r, err := Render(tpl, "venus", map[string]Section{"now": nowFacts(14.87, nil)}, set(t))
	if err != nil {
		t.Fatal(err)
	}
	if l := r.Lines[0]; l.Value != "14°52′ east" || l.Aside != "by longitude" {
		t.Errorf("qualifier: %+v", l)
	}
}

// R10 — too narrow: the aside goes first, then the value is cut at a word
// boundary with …; the label is never cut and no number is cut in two.
func TestOverflowDropsTheThirdColumnFirst(t *testing.T) {
	lines := []Line{{Label: "elongation", Value: "14°52′ east", Aside: "longitude"}}
	at := func(w int) Laid { return Layout(lines, w)[0] }
	if l := at(54); l.Aside != "longitude" || utf8.RuneCountInString(l.Main)+utf8.RuneCountInString(l.Aside) != 54 {
		t.Errorf("at 54: %+v", l)
	}
	if l := at(40); l.Aside != "longitude" {
		t.Errorf("at 40 everything still fits exactly: %+v", l)
	}
	if l := at(38); l.Aside != "" || l.Main != "elongation         14°52′ east" {
		t.Errorf("at 38 the aside should go first: %+v", l)
	}
	if l := at(28); l.Aside != "" || l.Main != "elongation         14°52′…" {
		t.Errorf("at 28 the value is cut at the word boundary: %+v", l)
	}
	if l := at(22); l.Aside != "" || l.Main != "elongation         …" {
		t.Errorf("at 22 no number is cut in two: %+v", l)
	}
}

// R11 — a term too long for the label column fails the load, naming the line.
func TestLabelsFitTheColumn(t *testing.T) {
	long := strings.Repeat("ab", 9) // 18
	terms := "- id: long\n  display: " + long + "\n  grade: plain\n- id: elongation\n  display: elongation\n  grade: plain\n- id: within\n  display: within\n  grade: plain\n- id: yes\n  display: yes\n  grade: plain\n- id: no\n  display: no\n  grade: plain\n- id: condition\n  display: (condition)\n  grade: plain\n"
	s := setFrom(t, terms)
	allow, _ := fsAllow()
	if _, err := loadAll(fsMap(header+"## @now\n{term:elongation} {elongation.lon}\n", allow), reg, s); err != nil {
		t.Fatalf("a 10-character label: %v", err)
	}
	_, err := loadAll(fsMap(header+"## @now\n{term:long} {elongation.lon}\n", allow), reg, s)
	if err == nil || !strings.Contains(err.Error(), "x.md:9:") || !strings.Contains(err.Error(), "label column") {
		t.Errorf("an 18-character label loaded: %v", err)
	}
}

func fsMap(src string, allow []byte) fstest.MapFS {
	return fstest.MapFS{"x.md": {Data: []byte(src)}, "allowlist.txt": {Data: allow}}
}

// setFrom is a rules set with the given terms file and a two-row beams rule.
func setFrom(t *testing.T, terms string) rules.Set {
	t.Helper()
	const rule = `rule: solar.beams
kind: within
measure: elongation.lon.abs
verdict: under-the-beams
otherwise: not-under-the-beams
rows:
  - id: solar.beams/17
    label: general
    threshold: 17°
    inclusive: no
    citation:
      author: A. Author
      work: A Work
      edition: London 1647
      locator: p.113
    grade: primary
`
	s, err := rules.LoadFS(fstest.MapFS{"terms.yaml": {Data: []byte(terms + "- id: under-the-beams\n  display: under the beams\n  citation: A Work p.113\n  grade: primary\n- id: not-under-the-beams\n  display: not under the beams\n  citation: A Work p.113\n  grade: primary\n")}, "solar-phase.yaml": {Data: []byte(rule)}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
