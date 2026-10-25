package templates

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/cruiserxyz/sextile/internal/rules"
)

// testRegistry is the fact keys this slice's template names, with kinds.
type testRegistry map[string]Kind

func (r testRegistry) FactKind(key string) (Kind, bool) { k, ok := r[key]; return k, ok }

func (r testRegistry) FactMax(key string) int {
	if r[key] == KindList {
		return 5
	}
	return 0
}

var reg = testRegistry{
	"elongation.lon":                     KindValue,
	"elongation.lon.abs":                 KindValue,
	"elongation.side":                    KindCategory,
	"sign.same-as-sun":                   KindCategory,
	"synodic.conjunction.inferior.last5": KindList,
	"synodic.phase":                      KindCategory,
	"synodic.phase.alt":                  KindCategory,
	"elongation.trend":                   KindCategory,
	"motion.direction":                   KindCategory,
	"station.last":                       KindInstant,
	"station.next":                       KindInstant,
	"synodic.conjunction.inferior.last":  KindInstant,
	"synodic.conjunction.inferior.next":  KindInstant,
	"synodic.conjunction.superior.last":  KindInstant,
	"synodic.conjunction.superior.next":  KindInstant,
	"brightness.magnitude":               KindValue,
	"brightness.lit":                     KindValue,
	"synodic.conjunction.inferior.sixth": KindValue,
	"synodic.conjunction.inferior.drift": KindValue,
}

type factSection map[string]Fact

func (s factSection) Fact(key string) (Fact, bool) { f, ok := s[key]; return f, ok }

func set(t *testing.T) rules.Set {
	t.Helper()
	s, err := rules.Load()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

const header = "---\nformat: 1\norigin: shipped\nid: x\ntab: Details\nsubject: {kind: body, class: inferior-planet, bodies: [venus]}\n---\n"

func loadOne(t *testing.T, src string) error {
	t.Helper()
	allow, err := fsAllow()
	if err != nil {
		t.Fatal(err)
	}
	_, err = loadAll(fstest.MapFS{"x.md": {Data: []byte(src)}, "allowlist.txt": {Data: allow}}, reg, set(t))
	return err
}

func fsAllow() ([]byte, error) { return files.ReadFile("allowlist.txt") }

// Every shipped template loads against the registry and the rules, so a
// broken template cannot pass ./check.
func TestEveryShippedTemplateLoads(t *testing.T) {
	ts, err := Shipped(reg, set(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) == 0 {
		t.Fatal("no shipped template")
	}
	tpl, ok := ForBody(ts, "venus")
	if !ok || tpl.ID != "body-inferior-planet" || tpl.Tab != "Details" {
		t.Errorf("ForBody(venus) = %+v, %v", tpl, ok)
	}
	if _, ok := ForBody(ts, "mars"); ok {
		t.Error("a template for mars")
	}
	// A template with an invalid slot is a load error naming its file and
	// line: the loader that checks the shipped set can see a bad one.
	allow, _ := fsAllow()
	bad := fstest.MapFS{"bad.md": {Data: []byte(header2 + "## @now\n{term:elongation} {elongation.side == term(\"shiny\") ? term(\"east\") : term(\"west\")}\n")}, "allowlist.txt": {Data: allow}}
	if _, err := loadAll(bad, reg, set(t)); err == nil || !strings.Contains(err.Error(), "bad.md:9:") {
		t.Errorf("a template with an invalid slot loaded, or lost its file:line: %v", err)
	}
}

// A missing or mistyped front-matter field, an unknown slot, rule, term or
// key is a load error naming the file and line; unknown never means omit.
func TestUnknownSlotIsALoadError(t *testing.T) {
	if err := loadOne(t, header+"## @now\n{term:elongation} {elongation.lon}\n"); err != nil {
		t.Fatalf("the good fixture: %v", err)
	}
	for name, tc := range map[string]struct{ src, want string }{
		"unknown fact":      {header + "## @now\n{elongation.latitude}\n", "x.md:9:"},
		"unknown term":      {header + "## @now\n{term:shiny}\n", "x.md:9:"},
		"unknown rule":      {header + "## @now\n{rule:solar.glow}\n", "x.md:9:"},
		"not a slot":        {header + "## @now\n{Venus}\n", "x.md:9:"},
		"no format":         {strings.Replace(header, "format: 1\n", "", 1) + "## @now\n{term:elongation}\n", "lacks \"format\""},
		"format 3":          {strings.Replace(header, "format: 1", "format: 3", 1) + "## @now\n{term:elongation}\n", "x.md:2:"},
		"origin user":       {strings.Replace(header, "origin: shipped", "origin: user", 1) + "## @now\n{term:elongation}\n", "x.md:3:"},
		"no subject":        {strings.Replace(header, "subject: {kind: body, class: inferior-planet, bodies: [venus]}\n", "", 1) + "## @now\n{term:elongation}\n", "lacks \"subject\""},
		"mistyped subject":  {strings.Replace(header, "{kind: body,", "{kind: chart,", 1) + "## @now\n{term:elongation}\n", "x.md:6:"},
		"unknown key":       {strings.Replace(header, "tab: Details\n", "tab: Details\ncolour: red\n", 1) + "## @now\n{term:elongation}\n", "x.md:6:"},
		"no section":        {header + "{term:elongation}\n", "x.md:8:"},
		"other heading":     {header + "## @then\n{term:elongation}\n", "x.md:8:"},
		"rule not alone":    {header + "## @now\n{term:elongation} {rule:solar.beams}\n", "x.md:9:"},
		"subject key twice": {strings.Replace(header, "{kind: body,", "{kind: body, kind: body,", 1) + "## @now\n{term:elongation}\n", "x.md:6:"},
	} {
		err := loadOne(t, tc.src)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: %v, want an error containing %q", name, err, tc.want)
		}
	}
}

// Outside slots and section markers a line holds only spaces and the
// allowlisted characters: a word or a digit as a literal fails the load.
func TestLiteralsAreAllowlisted(t *testing.T) {
	if err := loadOne(t, header+"## @now\n{term:elongation} · {elongation.lon} – ({term:longitude}); —, :\n"); err != nil {
		t.Fatalf("allowlisted literals: %v", err)
	}
	for _, lit := range []string{"combust", "17°", "Lilly", "x", "7", "°", "'", "%"} {
		err := loadOne(t, header+"## @now\n{term:elongation} "+lit+"\n")
		if err == nil || !strings.Contains(err.Error(), "allowlist") {
			t.Errorf("literal %q: %v", lit, err)
		}
	}
}

// hasValue reports whether any rendered line carries the value.
func hasValue(lines []Line, value string) bool {
	for _, l := range lines {
		if l.Value == value {
			return true
		}
	}
	return false
}

// A line naming an absent fact is omitted; a fact the section lacks
// entirely is an error, never an omission.
func TestAbsentOmitsTheLine(t *testing.T) {
	ts, err := Shipped(reg, set(t))
	if err != nil {
		t.Fatal(err)
	}
	tpl, _ := ForBody(ts, "venus")
	now := nowFacts(14.87, []string{"Oct 24, 2026   0°45′ Sco"})
	full, err := Render(tpl, "venus", map[string]Section{"now": now}, set(t))
	if err != nil {
		t.Fatal(err)
	}
	if !hasValue(full.Lines, "approaching the Sun") {
		t.Fatalf("the full render lacks the trend line: %+v", full.Lines)
	}
	// An absent trend omits its own line and nothing else: at a conjunction
	// the trend is absent, the phase is not. An absent elongation omits its
	// line and every rule line that measures it.
	now["elongation.trend"] = Fact{Kind: KindCategory, Absent: true}
	r, err := Render(tpl, "venus", map[string]Section{"now": now}, set(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Lines) != len(full.Lines)-1 || r.Lines[0] != full.Lines[0] || r.Lines[1] != full.Lines[1] || hasValue(r.Lines, "approaching the Sun") {
		t.Errorf("absent trend: %+v", r.Lines)
	}
	now["elongation.trend"] = Fact{Kind: KindCategory, Term: "approaching"}
	now["elongation.lon"] = Fact{Kind: KindValue, Absent: true}
	now["elongation.lon.abs"] = Fact{Kind: KindValue, Absent: true}
	r, err = Render(tpl, "venus", map[string]Section{"now": now}, set(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.RuleRows) != 0 {
		t.Errorf("absent elongation: rows %v", r.RuleRows)
	}
	for _, l := range r.Lines {
		if l.Label == "elongation" || l.Label == "under the beams" || l.Label == "combust" {
			t.Errorf("absent elongation still rendered %+v", l)
		}
	}
	if r.Lines[0].Label != "phase" || len(r.Lines) != len(full.Lines)-4 {
		t.Errorf("absent elongation: %+v", r.Lines)
	}
	delete(now, "elongation.side")
	if _, err := Render(tpl, "venus", map[string]Section{"now": now}, set(t)); err == nil {
		t.Error("a fact the section lacks rendered")
	}
}

// A list fact renders one item per line under its first line.
func TestListFactRepeats(t *testing.T) {
	ts, _ := Shipped(reg, set(t))
	tpl, _ := ForBody(ts, "venus")
	items := []string{"Oct 24, 2026   0°45′ Sco", "Mar 23, 2025   2°50′ Ari", "Aug 13, 2023  20°28′ Leo", "Jan  9, 2022  18°43′ Cap", "Jun  3, 2020  13°36′ Gem"}
	r, err := Render(tpl, "venus", map[string]Section{"now": nowFacts(14.87, items)}, set(t))
	if err != nil {
		t.Fatal(err)
	}
	// The first item sits on the label's own line, the rest in the value
	// column under it (R1); the eight-year line follows the list.
	i := len(r.Lines) - 1 - len(items)
	if i < 0 || r.Lines[i].Label != "inner conjunction" || r.Lines[len(r.Lines)-1].Label != "8 years" {
		t.Fatalf("lines %+v", r.Lines)
	}
	for k, it := range items {
		if l := r.Lines[i+k]; l.Value != it || (k > 0 && l.Label != "") {
			t.Errorf("item %d: %+v", k, l)
		}
	}
	// A producer that hands over more than the registry's count shows only
	// that many.
	six := append(append([]string(nil), items...), "Oct 26, 2018   3°06′ Sco")
	r, err = Render(tpl, "venus", map[string]Section{"now": nowFacts(14.87, six)}, set(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(r.Lines) - 1 - i; got != 5 {
		t.Errorf("six items rendered %d list lines, want 5", got)
	}
}

func nowFacts(elong float64, items []string) factSection {
	side := "east"
	if elong < 0 {
		side = "west"
	}
	abs := elong
	if abs < 0 {
		abs = -abs
	}
	return factSection{
		"elongation.lon":                     {Kind: KindValue, Text: "14°52′", Num: elong},
		"elongation.lon.abs":                 {Kind: KindValue, Text: "14°52′", Num: abs},
		"elongation.side":                    {Kind: KindCategory, Term: side},
		"synodic.conjunction.inferior.last5": {Kind: KindList, Items: items},
		"synodic.phase":                      {Kind: KindCategory, Term: "evening-star"},
		"synodic.phase.alt":                  {Kind: KindCategory, Term: "occidental"},
		"elongation.trend":                   {Kind: KindCategory, Term: "approaching"},
		"sign.same-as-sun":                   {Kind: KindCategory, Term: "no"},
		"motion.direction":                   {Kind: KindCategory, Term: "retrograde"},
		"station.last":                       {Kind: KindInstant, Term: "station-retrograde", Text: "Oct 3, 2026 7:16 8°29′ Sco"},
		"station.next":                       {Kind: KindInstant, Term: "station-direct", Text: "Nov 14, 2026 0:27 22°51′ Lib"},
		"synodic.conjunction.inferior.last":  {Kind: KindInstant, Text: "Mar 23, 2025 1:08 2°39′ Ari"},
		"synodic.conjunction.inferior.next":  {Kind: KindInstant, Text: "Oct 24, 2026 3:44 0°45′ Sco"},
		"synodic.conjunction.superior.last":  {Kind: KindInstant, Text: "Jan 6, 2026 16:36 16°22′ Cap"},
		"synodic.conjunction.superior.next":  {Kind: KindInstant, Text: "Aug 12, 2027 0:21 19°06′ Leo"},
		"brightness.magnitude":               {Kind: KindValue, Text: "−4.3"},
		"brightness.lit":                     {Kind: KindValue, Text: "4%"},
		"synodic.conjunction.inferior.sixth": {Kind: KindValue, Text: "Mar 25, 2017   4°57′ Ari"},
		"synodic.conjunction.inferior.drift": {Kind: KindValue, Text: "−2°18′"},
	}
}

// {rule:x} renders every row that applies, in file order, each in the
// renderer's fixed form (R3), and names the rows for the Log; the whole
// section reads in the contract's three columns.
func TestRuleRendersEveryAttestedRow(t *testing.T) {
	ts, _ := Shipped(reg, set(t))
	tpl, _ := ForBody(ts, "venus")
	r, err := Render(tpl, "venus", map[string]Section{"now": nowFacts(14.87, nil)}, set(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []Line{
		{Label: "elongation", Value: "14°52′ east", Aside: "by longitude"},
		{Label: "phase", Value: "evening star · occidental"},
		{Value: "approaching the Sun"},
		{Label: "under the beams", Value: "yes", Aside: "within 17°"},
		{Value: "no", Aside: "within 12°"},
		{Label: "combust", Value: "no (condition)", Aside: "within 8°30′"},
		{Blank: true},
		{Label: "motion", Value: "retrograde"},
		{Label: "station", Value: "last Oct 3, 2026 7:16 8°29′ Sco", Aside: "ecliptic"},
		{Label: "station", Value: "next Nov 14, 2026 0:27 22°51′ Lib", Aside: "ecliptic"},
		{Label: "inner conjunction", Value: "last Mar 23, 2025 1:08 2°39′ Ari"},
		{Label: "inner conjunction", Value: "next Oct 24, 2026 3:44 0°45′ Sco"},
		{Label: "outer conjunction", Value: "last Jan 6, 2026 16:36 16°22′ Cap"},
		{Label: "outer conjunction", Value: "next Aug 12, 2027 0:21 19°06′ Leo"},
		{Blank: true},
		{Label: "magnitude", Value: "−4.3 · 4% lit"},
		{Blank: true},
		{Label: "inner conjunction"},
		{Label: "8 years", Value: "Mar 25, 2017   4°57′ Ari · −2°18′"},
	}
	if fmt.Sprint(r.Lines) != fmt.Sprint(want) {
		t.Errorf("got\n%+v\nwant\n%+v", r.Lines, want)
	}
	if strings.Join(r.RuleRows, " ") != "solar.beams/17 solar.beams/12 solar.combust/8.5" {
		t.Errorf("rule rows %v", r.RuleRows)
	}
}
