package rules

import (
	"strings"
	"testing"
	"testing/fstest"
)

const goodTerms = `- id: under-the-beams
  display: under the beams
  citation: Lilly, *Christian Astrology*, London 1647, p.113
  grade: primary
- id: not-under-the-beams
  display: not under the beams
  citation: Lilly, *Christian Astrology*, London 1647, p.113
  grade: primary
- id: condition
  display: (condition)
  grade: plain
- id: yes
  display: yes
  grade: plain
`

const goodRule = `rule: solar.beams
kind: within
measure: elongation.lon.abs
verdict: under-the-beams
otherwise: not-under-the-beams
rows:
  - id: solar.beams/a
    label: A
    threshold: 17°
    inclusive: no
    citation:
      author: William Lilly
      work: Christian Astrology
      edition: London 1647
      locator: p.113
    grade: primary
  - id: solar.beams/b
    label: B
    threshold: 12°
    inclusive: yes
    bodies: [venus]
    condition:
      fact: sign.same-as-sun
      equals: yes
    citation:
      author: William Lilly
      work: Christian Astrology
      edition: London 1647
      locator: p.300
    grade: primary
`

func loadFiles(terms, rule string) (Set, error) {
	return load(fstest.MapFS{"terms.yaml": {Data: []byte(terms)}, "solar-phase.yaml": {Data: []byte(rule)}})
}

// The shipped files load: every term complete, every row complete.
func TestShippedRulesLoad(t *testing.T) {
	set, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	r, ok := set.Rules["solar.beams"]
	if !ok || len(r.Rows) != 2 || r.Rows[0].ID != "solar.beams/17" || r.Rows[0].Inclusive || r.Rows[0].Threshold != 17 ||
		r.Rows[1].ID != "solar.beams/12" || !r.Rows[1].Inclusive || r.Rows[1].Threshold != 12 {
		t.Errorf("solar.beams = %+v", r)
	}
}

// Every term has a display, a grade, and a citation unless it is plain
// (fixture: a term without a grade fails).
func TestTermsAreComplete(t *testing.T) {
	if _, err := loadFiles(goodTerms, goodRule); err != nil {
		t.Fatalf("the good fixture: %v", err)
	}
	for name, terms := range map[string]string{
		"no grade":            strings.Replace(goodTerms, "  grade: primary\n- id: not-under", "- id: not-under", 1),
		"no citation":         strings.Replace(goodTerms, "  citation: Lilly, *Christian Astrology*, London 1647, p.113\n  grade: primary\n- id: not-under", "  grade: primary\n- id: not-under", 1),
		"plain with citation": strings.Replace(goodTerms, "  display: yes\n", "  display: yes\n  citation: somebody\n", 1),
		"unknown grade":       strings.Replace(goodTerms, "grade: plain\n- id: yes", "grade: folk\n- id: yes", 1),
		"no display":          strings.Replace(goodTerms, "  display: (condition)\n", "", 1),
		"unknown key":         strings.Replace(goodTerms, "  display: yes\n", "  display: yes\n  colour: red\n", 1),
		"said twice":          goodTerms + "- id: yes\n  display: yes\n  grade: plain\n",
		"no condition term":   strings.Replace(goodTerms, "- id: condition\n  display: (condition)\n  grade: plain\n", "", 1),
	} {
		if _, err := loadFiles(terms, goodRule); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
}

// A row missing any required field fails the load.
func TestRowsAreComplete(t *testing.T) {
	for _, field := range []string{"    label: A\n", "    threshold: 17°\n", "    inclusive: no\n", "    grade: primary\n", "      locator: p.113\n", "      author: William Lilly\n"} {
		if _, err := loadFiles(goodTerms, strings.Replace(goodRule, field, "", 1)); err == nil {
			t.Errorf("a row without %q loaded", strings.TrimSpace(field))
		}
	}
	for name, rule := range map[string]string{
		"no measure":             strings.Replace(goodRule, "measure: elongation.lon.abs\n", "", 1),
		"kind not built":         strings.Replace(goodRule, "kind: within", "kind: table", 1),
		"verdict not a term":     strings.Replace(goodRule, "verdict: under-the-beams", "verdict: combust", 1),
		"id not the rule's":      strings.Replace(goodRule, "id: solar.beams/a", "id: other/a", 1),
		"threshold not a number": strings.Replace(goodRule, "threshold: 17°", "threshold: 17", 1),
		"inclusive not yes/no":   strings.Replace(goodRule, "inclusive: no", "inclusive: maybe", 1),
		"condition term missing": strings.Replace(goodRule, "equals: yes", "equals: same", 1),
		"a flow map":             strings.Replace(goodRule, "    citation:\n      author: William Lilly\n      work: Christian Astrology\n      edition: London 1647\n      locator: p.113\n", "    citation: {author: x}\n", 1),
		"a plain row":            strings.Replace(goodRule, "    grade: primary\n  - id: solar.beams/b", "    grade: plain\n  - id: solar.beams/b", 1),
		"a body that is a map":   strings.Replace(goodRule, "    bodies: [venus]\n", "    bodies:\n      - id: venus\n", 1),
	} {
		if _, err := loadFiles(goodTerms, rule); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
}

type facts struct {
	value    float64
	category string
}

func (f facts) Value(key string) (float64, bool) {
	return f.value, key == "elongation.lon.abs"
}
func (f facts) Category(key string) (string, bool) { return f.category, key == "sign.same-as-sun" }

// The verdict is taken on the parsed value at exactly the threshold:
// exclusive rows do not hold there, inclusive ones do.
func TestWithinBoundaries(t *testing.T) {
	set, err := loadFiles(goodTerms, goodRule)
	if err != nil {
		t.Fatal(err)
	}
	r := set.Rules["solar.beams"]
	for _, tc := range []struct {
		measure float64
		want    string
	}{
		{16.99972, "under-the-beams not-under-the-beams"}, // 16°59′59″: under A (17, exclusive); outside B (12)
		{17, "not-under-the-beams not-under-the-beams"},   // exactly 17: A exclusive, so not under
		{12, "under-the-beams under-the-beams"},           // exactly 12: B inclusive, so under
		{12.00028, "under-the-beams not-under-the-beams"}, // 12°00′01″
	} {
		vs, err := Evaluate(r, "venus", facts{value: tc.measure, category: "yes"})
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, v := range vs {
			got = append(got, v.Term)
		}
		if strings.Join(got, " ") != tc.want {
			t.Errorf("measure %v: %v, want %s", tc.measure, got, tc.want)
		}
	}
	// A row limited to other bodies is not rendered for this one; a failed
	// condition keeps the row with its otherwise term and the note.
	vs, _ := Evaluate(r, "mars", facts{value: 1, category: "yes"})
	if len(vs) != 1 || vs[0].Row.ID != "solar.beams/a" {
		t.Errorf("mars: %+v", vs)
	}
	vs, _ = Evaluate(r, "venus", facts{value: 1, category: "no"})
	if len(vs) != 2 || vs[1].Term != "not-under-the-beams" || !vs[1].Condition {
		t.Errorf("failed condition: %+v", vs)
	}
	if _, err := Evaluate(r, "venus", facts{}); err != nil {
		t.Errorf("a present measure of 0 is a value: %v", err)
	}
}

// R4 of the specification: a row's id and label name the
// criterion, never the author; the citation is the attribution.
func TestRowIdsAreNeutral(t *testing.T) {
	if _, err := loadFiles(goodTerms, goodRule); err != nil {
		t.Fatalf("the good fixture: %v", err)
	}
	for name, rule := range map[string]string{
		"id names the author":    strings.Replace(goodRule, "id: solar.beams/a\n", "id: solar.beams/lilly-113\n", 1),
		"label names the author": strings.Replace(goodRule, "label: A\n", "label: Lilly\n", 1),
	} {
		_, err := loadFiles(goodTerms, rule)
		if err == nil || !strings.Contains(err.Error(), "names the author") {
			t.Errorf("%s: %v", name, err)
		}
	}
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range s.Rules {
		for _, row := range r.Rows {
			if sur := authorSurname(row.Citation.Author); strings.Contains(strings.ToLower(row.ID+row.Label), sur) {
				t.Errorf("shipped row %s (%s) names its author", row.ID, row.Label)
			}
		}
	}
}

// Every shipped solar row, judged exactly at its threshold and one second of
// arc either side: an inclusive row holds at the threshold, an exclusive one
// does not. Lilly's combustion needs the same sign; cazimi renders nothing
// when no row holds, and every row when any does.
func TestSolarRowsAgainstFixtures(t *testing.T) {
	set, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	sec := 1.0 / 3600
	for _, id := range []string{"solar.beams", "solar.combust", "solar.cazimi"} {
		r, ok := set.Rules[id]
		if !ok {
			t.Fatalf("no rule %s", id)
		}
		for _, row := range r.Rows {
			for _, tc := range []struct {
				measure float64
				holds   bool
			}{
				{row.Threshold - sec, true},
				{row.Threshold, row.Inclusive},
				{row.Threshold + sec, false},
			} {
				one := r
				one.Rows = []Row{row}
				one.OmitWhenNoneHold = false
				vs, err := Evaluate(one, "venus", facts{value: tc.measure, category: "yes"})
				if err != nil || len(vs) != 1 {
					t.Fatalf("%s at %v: %+v, %v", row.ID, tc.measure, vs, err)
				}
				if got := vs[0].Term == r.Verdict; got != tc.holds {
					t.Errorf("%s at %.6f° (threshold %s, inclusive %v): holds %v, want %v", row.ID, tc.measure, row.ThresholdText(), row.Inclusive, got, tc.holds)
				}
			}
		}
	}
	combust := set.Rules["solar.combust"]
	vs, _ := Evaluate(combust, "venus", facts{value: 5, category: "no"})
	if len(vs) != 1 || vs[0].Term != "not-combust" || !vs[0].Condition {
		t.Errorf("combust in another sign: %+v", vs)
	}
	cazimi := set.Rules["solar.cazimi"]
	if vs, _ := Evaluate(cazimi, "venus", facts{value: 1, category: "yes"}); len(vs) != 0 {
		t.Errorf("cazimi at 1°: %+v, want nothing", vs)
	}
	// 16′30″: Lilly's 17′ holds, his 16′ and al-Bīrūnī's do not — all three show.
	vs, _ = Evaluate(cazimi, "venus", facts{value: 16.5 / 60, category: "yes"})
	if len(vs) != 3 || vs[0].Term != "cazimi" || vs[1].Term != "not-cazimi" || vs[2].Term != "not-cazimi" {
		t.Errorf("cazimi at 16′30″: %+v", vs)
	}
	// 16′ exactly: Lilly's two hold (within), al-Bīrūnī's does not (less than).
	vs, _ = Evaluate(cazimi, "venus", facts{value: 16.0 / 60, category: "yes"})
	if len(vs) != 3 || vs[1].Term != "cazimi" || vs[2].Term != "not-cazimi" {
		t.Errorf("cazimi at 16′: %+v", vs)
	}
	for text, want := range map[string]float64{"17°": 17, "8°30′": 8.5, "16′": 16.0 / 60} {
		if got, ok := parseThreshold(text); !ok || got != want {
			t.Errorf("parseThreshold(%q) = %v, %v", text, got, ok)
		}
	}
	for _, bad := range []string{"17", "8.5°", "8°60′", "0′", "°", "17°30", "60′", "90′"} {
		if _, ok := parseThreshold(bad); ok {
			t.Errorf("parseThreshold(%q) accepted", bad)
		}
	}
}
