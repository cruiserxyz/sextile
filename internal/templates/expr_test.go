package templates

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// The slot grammar (the specification): a slot that is not a
// legacy slot is an expression, parsed and typed at load.

const header2 = "---\nformat: 2\norigin: shipped\nid: x\ntab: Details\nsubject: {kind: body, class: inferior-planet, bodies: [venus]}\n---\n"

const condSide = `elongation.side == term("east") ? term("evening-star") : term("morning-star")`

// reason is a load error without its "templates: x.md:9: " prefix and without
// the echo of the slot's own text, so a want-string cannot match the fixture.
func reason(err error) string {
	m := err.Error()
	if i := strings.Index(m, "x.md:9: "); i >= 0 {
		m = m[i+len("x.md:9: "):]
	}
	if strings.HasPrefix(m, "{") {
		if i := strings.Index(m, "}: "); i >= 0 {
			m = m[i+3:]
		}
	}
	return m
}

// load2 loads a format 2 template whose only line is the given slot text.
func load2(t *testing.T, slots ...string) error {
	t.Helper()
	return loadOne(t, header2+"## @now\n"+strings.Join(slots, "\n")+"\n")
}

func mustAllow() []byte { b, _ := fsAllow(); return b }

func render2(t *testing.T, sec Section, lines ...string) Rendered {
	t.Helper()
	allow, _ := fsAllow()
	ts, err := loadAll(fsMap(header2+"## @now\n"+strings.Join(lines, "\n")+"\n", allow), reg, set(t))
	if err != nil {
		t.Fatal(err)
	}
	r, err := Render(ts[0], "venus", map[string]Section{"now": sec}, set(t))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// T1 — legacy slots keep their rendered lines and their load errors, frozen.
func TestOldSlotsAreUnchanged(t *testing.T) {
	items := []string{"2026-10-24 0°45′ Sco", "2025-03-23 2°50′ Ari"}
	ts, err := Shipped(reg, set(t))
	if err != nil {
		t.Fatal(err)
	}
	tpl, _ := ForBody(ts, "venus")
	r, err := Render(tpl, "venus", map[string]Section{"now": nowFacts(14.87, items)}, set(t))
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprint([]Line{
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
		{Label: "inner conjunction", Value: "2026-10-24 0°45′ Sco"},
		{Value: "2025-03-23 2°50′ Ari"},
		{Label: "8 years", Value: "Mar 25, 2017   4°57′ Ari · −2°18′"},
	})
	if got := fmt.Sprint(r.Lines); got != want {
		t.Errorf("shipped:\ngot  %s\nwant %s", got, want)
	}
	allow, _ := fsAllow()
	fx, err := loadAll(fsMap(header+"## @now\n{term:east}\n{rule:solar.beams}\n{elongation.lon}\n{synodic.conjunction.inferior.last5}\n", allow), reg, set(t))
	if err != nil {
		t.Fatal(err)
	}
	r, err = Render(fx[0], "venus", map[string]Section{"now": nowFacts(14.87, items)}, set(t))
	if err != nil {
		t.Fatal(err)
	}
	want = "[{ east  false} {under the beams yes within 17° false} { no within 12° false} { 14°52′  false} { 2026-10-24 0°45′ Sco  false} { 2025-03-23 2°50′ Ari  false}]"
	if got := fmt.Sprint(r.Lines); got != want {
		t.Errorf("fixture:\ngot  %s\nwant %s", got, want)
	}
	for src, want := range map[string]string{
		"{elongation.latitude}": "templates: x.md:9: {elongation.latitude}: no such fact in the registry",
		"{term:shiny}":          "templates: x.md:9: {term:shiny}: no such term in rules/terms.yaml",
		"{rule:solar.glow}":     "templates: x.md:9: {rule:solar.glow}: no such rule in rules/",
	} {
		if err := loadOne(t, header+"## @now\n"+src+"\n"); err == nil || err.Error() != want {
			t.Errorf("%s: %v, want %q", src, err, want)
		}
	}
}

// T2 — an unknown name is a load error, never a render one.
func TestUnknownIdentifierIsALoadError(t *testing.T) {
	for name, tc := range map[string]struct{ slot, want string }{
		"fact":     {`has(elongation.latitude) ? term("east") : term("west")`, "no such fact"},
		"fact cmp": {`elongation.latitude == term("east") ? term("east") : term("west")`, "no such fact"},
		"term":     {`term("shiny")`, "no such term"},
		"function": {`frob("east")`, "no such function"},
		"word":     {`east`, "unknown identifier"},
	} {
		err := load2(t, "{"+tc.slot+"}")
		if err == nil || !strings.Contains(err.Error(), "x.md:9:") || !strings.Contains(reason(err), tc.want) {
			t.Errorf("%s: %v, want x.md:9 and %q after the slot", name, err, tc.want)
		}
	}
}

// T3 — the grammar allowlist: one accepting fixture per construct, one
// refusing fixture per refused construct.
func TestGrammarAllowlist(t *testing.T) {
	const tt = `? term("evening-star") : term("morning-star")`
	for name, slot := range map[string]string{
		"fact in parens":   `(elongation.lon)`,
		"hyphen dot key":   `(elongation.lon.abs)`,
		"term":             `term("east")`,
		"has":              `has(elongation.lon) ` + tt,
		"==":               `elongation.side == term("east") ` + tt,
		"!=":               `elongation.side != term("east") ` + tt,
		"term on the left": `term("east") == elongation.side ` + tt,
		"&&":               `has(elongation.lon) && has(elongation.lon.abs) ` + tt,
		"||":               `has(elongation.lon) || has(elongation.lon.abs) ` + tt,
		"!":                `!has(elongation.lon) ` + tt,
		"!a && b":          `!has(elongation.lon) && has(elongation.lon.abs) ` + tt,
		"a && b || c":      `has(elongation.lon) && has(elongation.lon.abs) || has(elongation.side) ` + tt,
		"a || b ? x : y":   `has(elongation.lon) || has(elongation.lon.abs) ` + tt,
		"parens":           `(has(elongation.lon) || has(elongation.lon.abs)) && has(elongation.side) ` + tt,
		"nested ? :":       `has(elongation.lon) ? (has(elongation.side) ? term("east") : term("west")) : term("occidental")`,
		"chained else":     `has(elongation.lon) ? term("east") : has(elongation.side) ? term("west") : term("occidental")`,
		"whitespace":       "  has( elongation.lon )\t?\tterm( \"east\" )  :  term(\"west\")  ",
		"cond":             condSide,
		"hyphenated key":   `sign.same-as-sun == term("east") ? term("evening-star") : term("morning-star")`,
		"hyphenated has":   `has(sign.same-as-sun) ? elongation.lon : term("west")`,
		"fact branches":    `has(elongation.lon) ? elongation.lon : elongation.lon.abs`,
	} {
		if err := load2(t, "{"+slot+"}"); err != nil {
			t.Errorf("accept %s: %v", name, err)
		}
	}
	for name, tc := range map[string]struct{ slot, construct string }{
		"<":             {`elongation.lon < elongation.lon.abs ? term("east") : term("west")`, "comparison `<`"},
		"<=":            {`elongation.lon <= elongation.lon.abs ? term("east") : term("west")`, "comparison `<=`"},
		">":             {`elongation.lon > elongation.lon.abs ? term("east") : term("west")`, "comparison `>`"},
		">=":            {`elongation.lon >= elongation.lon.abs ? term("east") : term("west")`, "comparison `>=`"},
		"number":        {`elongation.lon == 17 ? term("east") : term("west")`, "number literal"},
		"true":          {`true ? term("east") : term("west")`, "bool literal"},
		"false":         {`false ? term("east") : term("west")`, "bool literal"},
		"bare string":   {`"east"`, "string literal"},
		"string in has": {`has("elongation.lon") ? term("east") : term("west")`, "string literal"},
		"??":            {`elongation.lon ?? elongation.lon.abs`, "`??` is not allowed"},
		"member access": {`elongation.side.name`, "member access"},
		"body name":     {`venus`, "body name"},
		"body call":     {`venus("x")`, "body name"},
		"plus":          {`elongation.lon + elongation.lon.abs`, "arithmetic operator `+`"},
		"minus":         {`elongation.lon - elongation.lon.abs`, "arithmetic operator `-`"},
		"times":         {`elongation.lon * 2`, "arithmetic operator `*`"},
		"rule call":     {`rule("solar.beams")`, "rule(...) is not a helper"},
		"loop":          {`for`, `unknown identifier "for"`},
		"block":         {`{term("east")}`, "stray or unterminated brace `{`"},
		"index":         {`elongation.lon[0]`, "index `[`"},
		"assign":        {`elongation.lon = 1`, "`=` is not allowed"},
		"empty":         {``, "empty"},
		"hyphen minus":  {`elongation.lon-elongation.lon.abs`, "arithmetic operator `-`"},
		"hyphen number": {`elongation.lon-1`, "arithmetic operator `-`"},
		"trailing":      {`term("east") term("west")`, "after the expression"},
	} {
		src := "{" + tc.slot + "}"
		err := load2(t, src)
		if err == nil || !strings.Contains(err.Error(), "x.md:9:") || !strings.Contains(reason(err), tc.construct) {
			t.Errorf("refuse %s: %v, want x.md:9 and %q", name, err, tc.construct)
		}
	}
}

// T6 — letters are allowed only inside a slot.
func TestLettersOnlyInsideSlots(t *testing.T) {
	if err := load2(t, "{term:elongation} {"+condSide+"} · ({term:by-longitude})"); err != nil {
		t.Fatalf("letters inside a slot: %v", err)
	}
	for _, lit := range []string{"combust", "17°", "Lilly", "x", "7", "°", "'", "%"} {
		err := load2(t, "{term:elongation} {"+condSide+"} "+lit)
		if err == nil || !strings.Contains(err.Error(), "allowlist") {
			t.Errorf("literal %q outside a slot: %v", lit, err)
		}
	}
	if err := loadOne(t, header2+"## @nowx\n{term:elongation}\n"); err == nil {
		t.Error("a section marker with a letter after it loaded")
	}
}

// T7 — a slot's result is a term or a fact; a bool is an error; a term a rule
// uses as verdict or otherwise, and yes and no, cannot be selected.
func TestSlotResultTypes(t *testing.T) {
	for name, slot := range map[string]string{
		"bool has":        `has(elongation.lon)`,
		"bool eq":         `elongation.side == term("east")`,
		"bool not":        `!has(elongation.lon)`,
		"bool branch":     `has(elongation.lon) ? has(elongation.lon) : term("east")`,
		"list fact":       `(synodic.conjunction.inferior.last5)`,
		"category result": `(elongation.side)`,
		"category then":   `has(elongation.lon) ? elongation.side : term("east")`,
		"category else":   `has(elongation.lon) ? term("east") : elongation.side`,
		"category nested": `has(elongation.lon) ? (has(elongation.side) ? term("east") : elongation.side) : term("west")`,
		"list branch":     `has(elongation.lon) ? synodic.conjunction.inferior.last5 : term("east")`,
	} {
		err := load2(t, "{"+slot+"}")
		if err == nil || !strings.Contains(err.Error(), "x.md:9:") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := load2(t, `{has(elongation.lon) ? term("east") : elongation.lon}`); err != nil {
		t.Errorf("a term and a fact as branches: %v", err)
	}
}

func TestReservedTermsAreNotSelectable(t *testing.T) {
	for _, id := range []string{"yes", "no", "under-the-beams", "not-under-the-beams"} {
		for name, slot := range map[string]string{
			"then":   `has(elongation.lon) ? term("` + id + `") : term("east")`,
			"else":   `has(elongation.lon) ? term("east") : term("` + id + `")`,
			"nested": `has(elongation.lon) ? (has(elongation.side) ? term("east") : term("` + id + `")) : term("west")`,
		} {
			err := load2(t, "{"+slot+"}")
			if err == nil || !strings.Contains(err.Error(), "x.md:9:") || !strings.Contains(reason(err), fmt.Sprintf("term %q is reserved", id)) {
				t.Errorf("%s %s: %v", id, name, err)
			}
		}
	}
	// only the literal term(...) call may stand opposite a category fact (T3)
	ext := pickHelpers(t)
	allow, _ := fsAllow()
	for _, slot := range []string{`elongation.side == pick(has(elongation.lon), "east", "west") ? term("east") : term("west")`, `pick(has(elongation.lon), "east", "west") != elongation.side ? term("east") : term("west")`} {
		_, err := loadAllWith(fsMap(header2+"## @now\n{"+slot+"}\n", allow), reg, set(t), ext)
		if err == nil || !strings.Contains(err.Error(), "x.md:9:") || !strings.Contains(err.Error(), "term(...)") {
			t.Errorf("a helper opposite a category fact: %v", err)
		}
	}
	// A reserved term in a TEST is only compared, never shown.
	if err := load2(t, `{elongation.side == term("yes") ? term("east") : term("west")}`); err != nil {
		t.Errorf("a reserved term in a test: %v", err)
	}
}

// T8 — absence is decided lazily, left to right; an absent value reached by
// evaluation omits the line.
func TestAbsentInExpressionOmitsTheLine(t *testing.T) {
	lonAbsent := func() factSection {
		s := nowFacts(14.87, nil)
		s["elongation.lon"] = Fact{Kind: KindValue, Absent: true}
		return s
	}
	sideAbsent := func() factSection {
		s := nowFacts(14.87, nil)
		s["elongation.side"] = Fact{Kind: KindCategory, Absent: true}
		return s
	}
	absAbsent := func() factSection {
		s := nowFacts(14.87, nil)
		s["elongation.lon.abs"] = Fact{Kind: KindValue, Absent: true}
		return s
	}
	n := func(r Rendered) int { return len(r.Lines) }
	// unguarded absent: omitted, and the other line stays
	r := render2(t, sideAbsent(), `{term:elongation} {`+condSide+`}`, `{term:inner-conjunction} {elongation.lon}`)
	if n(r) != 1 || r.Lines[0].Label != "inner conjunction" || len(r.Slots) != 0 {
		t.Errorf("unguarded absent: %+v", r)
	}
	// guarded, the else branch is a term
	g := `{has(elongation.lon) ? term("occidental") : term("oriental")}`
	if r := render2(t, lonAbsent(), g); n(r) != 1 || r.Lines[0].Value != "oriental" {
		t.Errorf("guarded absent: %+v", r.Lines)
	}
	if r := render2(t, nowFacts(14.87, nil), g); n(r) != 1 || r.Lines[0].Value != "occidental" {
		t.Errorf("guarded present: %+v", r.Lines)
	}
	// guarded, but the chosen branch is the absent key
	if r := render2(t, lonAbsent(), `{!has(elongation.lon) ? elongation.lon : term("oriental")}`); n(r) != 0 {
		t.Errorf("chosen absent branch: %+v", r.Lines)
	}
	// an absent key only in the unchosen branch renders
	if r := render2(t, absAbsent(), `{has(elongation.lon) ? elongation.lon : elongation.lon.abs}`); n(r) != 1 || r.Lines[0].Value != "14°52′" {
		t.Errorf("unchosen absent branch: %+v", r.Lines)
	}
	s := sideAbsent()
	if r := render2(t, s, `{has(elongation.side) && elongation.side == term("east") ? term("evening-star") : term("morning-star")}`); n(r) != 1 || r.Lines[0].Value != "morning star" {
		t.Errorf("short-circuit &&: %+v", r.Lines)
	}
	if r := render2(t, s, `{!has(elongation.side) || elongation.side == term("east") ? term("evening-star") : term("morning-star")}`); n(r) != 1 || r.Lines[0].Value != "evening star" {
		t.Errorf("short-circuit ||: %+v", r.Lines)
	}
	// the comparison is reached before the guard: an absent side omits the line
	if r := render2(t, sideAbsent(), `{elongation.side == term("east") && has(elongation.side) ? term("evening-star") : term("morning-star")}`); n(r) != 0 {
		t.Errorf("comparison before guard: %+v", r.Lines)
	}
	// a key the section lacks entirely is an error, as for a legacy slot
	allow, _ := fsAllow()
	ts, _ := loadAll(fsMap(header2+"## @now\n{"+condSide+"}\n", allow), reg, set(t))
	s2 := nowFacts(14.87, nil)
	delete(s2, "elongation.side")
	if _, err := Render(ts[0], "venus", map[string]Section{"now": s2}, set(t)); err == nil {
		t.Error("a fact the section lacks rendered")
	}
}

// pickHelpers is the default registry plus the test-only pick(bool, term id,
// term id) and neg(bool).
func pickHelpers(t *testing.T) HelperRegistry {
	t.Helper()
	ext := DefaultHelpers().Clone()
	for _, h := range []Helper{
		{Name: "pick", Args: []ArgKind{ArgBool, ArgTermID, ArgTermID}, Result: ResTerm, Kind: HelperSelect,
			Eval: func(a []Operand) Operand {
				if a[0].Bool {
					return Operand{Str: a[1].Str}
				}
				return Operand{Str: a[2].Str}
			}},
		{Name: "neg", Args: []ArgKind{ArgBool}, Result: ResBool, Kind: HelperSelect,
			Eval: func(a []Operand) Operand { return Operand{Bool: !a[0].Bool} }},
	} {
		if err := ext.Register(h); err != nil {
			t.Fatal(err)
		}
	}
	return ext
}

// T3 — each precedence and associativity boundary changes the selected term.
func TestGrammarPrecedenceRenders(t *testing.T) {
	// a = has(elongation.lon), b = has(elongation.lon.abs), c = has(elongation.side)
	facts := func(a, b, c bool) factSection {
		s := nowFacts(14.87, nil)
		s["elongation.lon"] = Fact{Kind: KindValue, Absent: !a, Text: "x"}
		s["elongation.lon.abs"] = Fact{Kind: KindValue, Absent: !b, Text: "x"}
		s["elongation.side"] = Fact{Kind: KindCategory, Absent: !c, Term: "east"}
		return s
	}
	const a, b, c = "has(elongation.lon)", "has(elongation.lon.abs)", "has(elongation.side)"
	const x, y, z = `term("east")`, `term("west")`, `term("occidental")`
	for _, tc := range []struct {
		name, slot string
		f          factSection
		want       string
	}{
		// ! binds tighter than &&: (!a) && b, not !(a && b)
		{"!a && b", "!" + a + " && " + b + " ? " + x + " : " + y, facts(false, false, true), "west"},
		{"!(a && b)", "!(" + a + " && " + b + ") ? " + x + " : " + y, facts(false, false, true), "east"},
		{"!a && b true", "!" + a + " && " + b + " ? " + x + " : " + y, facts(false, true, true), "east"},
		// && binds tighter than ||
		{"a && b || c", a + " && " + b + " || " + c + " ? " + x + " : " + y, facts(false, false, true), "east"},
		{"a && (b || c)", a + " && (" + b + " || " + c + ") ? " + x + " : " + y, facts(false, false, true), "west"},
		{"a || b && c", a + " || " + b + " && " + c + " ? " + x + " : " + y, facts(true, false, false), "east"},
		{"(a || b) && c", "(" + a + " || " + b + ") && " + c + " ? " + x + " : " + y, facts(true, false, false), "west"},
		// || binds tighter than ? :
		{"a || b ? x : y", a + " || " + b + " ? " + x + " : " + y, facts(false, true, false), "east"},
		{"a || b ? x : y false", a + " || " + b + " ? " + x + " : " + y, facts(false, false, false), "west"},
		// ? : is right-associative
		{"a ? x : b ? y : z, a", a + " ? " + x + " : " + b + " ? " + y + " : " + z, facts(true, false, false), "east"},
		{"a ? x : b ? y : z, b", a + " ? " + x + " : " + b + " ? " + y + " : " + z, facts(false, true, false), "west"},
		{"a ? x : b ? y : z, neither", a + " ? " + x + " : " + b + " ? " + y + " : " + z, facts(false, false, false), "occidental"},
		{"a ? (b ? y : z) : x", a + " ? (" + b + " ? " + y + " : " + z + ") : " + x, facts(true, false, false), "occidental"},
		// == binds tighter than &&
		{"== and &&", c + " && elongation.side == term(\"west\") ? " + x + " : " + y, facts(true, true, true), "west"},
		{"!=", "elongation.side != term(\"west\") ? " + x + " : " + y, facts(true, true, true), "east"},
	} {
		r := render2(t, tc.f, "{"+tc.slot+"}")
		if len(r.Lines) != 1 || r.Lines[0].Value != tc.want {
			t.Errorf("%s: %+v, want %q", tc.name, r.Lines, tc.want)
		}
	}
}

// A category key with a hyphen is one atomic token and compares.
func TestHyphenatedCategoryKey(t *testing.T) {
	r2 := testRegistry{}
	for k, v := range reg {
		r2[k] = v
	}
	r2["sign.same-as-sun"] = KindCategory
	allow, _ := fsAllow()
	ts, err := loadAll(fsMap(header2+"## @now\n{sign.same-as-sun == term(\"east\") ? term(\"evening-star\") : term(\"morning-star\")}\n{has(sign.same-as-sun) ? elongation.lon : term(\"west\")}\n", allow), r2, set(t))
	if err != nil {
		t.Fatal(err)
	}
	sec := nowFacts(14.87, nil)
	sec["sign.same-as-sun"] = Fact{Kind: KindCategory, Term: "east"}
	r, err := Render(ts[0], "venus", map[string]Section{"now": sec}, set(t))
	if err != nil || len(r.Lines) != 2 || r.Lines[0].Value != "evening star" || r.Lines[1].Value != "14°52′" {
		t.Errorf("%+v %v", r.Lines, err)
	}
}

// T9 — a helper is a registry entry; the parser resolves calls through it.
func TestHelperRegistryIsExtensible(t *testing.T) {
	hs := DefaultHelpers()
	var names []string
	for _, h := range hs.All() {
		names = append(names, h.Name+":"+string(h.Kind))
	}
	if got := strings.Join(names, " "); got != "has:select term:select" {
		t.Fatalf("v1 registry %q", got)
	}
	slot := `{pick(has(elongation.lon), "evening-star", "morning-star")}`
	allow, _ := fsAllow()
	src := header2 + "## @now\n" + slot + "\n"
	if _, err := loadAll(fsMap(src, allow), reg, set(t)); err == nil || !strings.Contains(err.Error(), "pick") {
		t.Fatalf("pick before it is registered: %v", err)
	}
	ext := hs.Clone()
	if err := ext.Register(Helper{Name: "pick", Args: []ArgKind{ArgBool, ArgTermID, ArgTermID}, Result: ResTerm, Kind: HelperSelect,
		Eval: func(a []Operand) Operand {
			if a[0].Bool {
				return Operand{Str: a[1].Str}
			}
			return Operand{Str: a[2].Str}
		}}); err != nil {
		t.Fatal(err)
	}
	ts, err := loadAllWith(fsMap(src, allow), reg, set(t), ext)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Render(ts[0], "venus", map[string]Section{"now": nowFacts(14.87, nil)}, set(t))
	if err != nil || len(r.Lines) != 1 || r.Lines[0].Value != "evening star" {
		t.Errorf("pick: %+v %v", r.Lines, err)
	}
	// its argument kinds are checked at load, from the registry
	if _, err := loadAllWith(fsMap(header2+"## @now\n{pick(has(elongation.lon), \"zzz\", \"east\")}\n", allow), reg, set(t), ext); err == nil || !strings.Contains(err.Error(), "zzz") {
		t.Errorf("pick with an unknown term: %v", err)
	}
	if len(hs.All()) != 2 {
		t.Error("registering on the clone changed the shipped registry")
	}
}

// derivationFindings names every derivation helper of reg that lacks a
// `// derivation: <name>` marker in the Go source or a row naming it in the
// quickref's ledger.
func derivationFindings(reg HelperRegistry, goSrc, quickref string) []string {
	var out []string
	for _, h := range reg.All() {
		if h.Kind != HelperDerivation {
			continue
		}
		if !strings.Contains(goSrc, "// derivation: "+h.Name) {
			out = append(out, h.Name+": no `// derivation: "+h.Name+"` marker")
		}
		row := false
		for _, l := range strings.Split(quickref, "\n") {
			if strings.HasPrefix(l, "|") && strings.Contains(l, "`"+h.Name+"`") {
				row = true
			}
		}
		if !row {
			out = append(out, h.Name+": no ledger row naming it")
		}
	}
	return out
}

// T10 (called from TestOutputIsNeutral) — no rule label or citation reaches a line through an expression.
func expressionOutputIsNeutral(t *testing.T) {
	s := set(t)
	r := render2(t, nowFacts(14.87, nil), `{term:elongation} {`+condSide+`}`, `{has(elongation.lon) ? elongation.lon : elongation.lon.abs}`)
	for _, rule := range s.Rules {
		for _, row := range rule.Rows {
			for _, word := range []string{row.Label, row.Citation.Author, row.Citation.Work, row.Citation.Edition, row.Citation.Locator} {
				for _, l := range r.Lines {
					if strings.Contains(strings.ToLower(l.Label+" "+l.Value+" "+l.Aside), strings.ToLower(word)) {
						t.Errorf("%q reaches the screen in %+v", word, l)
					}
				}
			}
		}
	}
}

// T13 — hostile templates are refused, quickly, without a panic.
func TestHostileTemplatesNeverPanic(t *testing.T) {
	deep := func(n int) string { return strings.Repeat("(", n) + `elongation.lon` + strings.Repeat(")", n) }
	hostile := map[string]struct{ src, want string }{
		"70000 bytes":       {"{" + strings.Repeat("a", 70000) + "}", "The most is 10000"},
		"70000 line":        {strings.Repeat(" ", 70000) + "{" + strings.Repeat("a", 70000) + "}", "The most is 10000"},
		"10001 bytes":       {"{" + strings.Repeat("a", 10001) + "}", "The most is 10000"},
		"33 deep":           {"{" + deep(33) + "}", "32"},
		"33 not":            {"{" + strings.Repeat("!", 33) + `has(elongation.lon) ? term("east") : term("west")}`, "32"},
		"33 else chain":     {"{" + strings.Repeat(`has(elongation.lon) ? term("east") : `, 33) + `term("west")}`, "32"},
		"unterminated":      {`{has(elongation.lon) ? term("east") : term("west")`, "brace"},
		"unterminated str":  {`{term("east}`, "unterminated string literal"},
		"lone close":        {`}`, "brace"},
		"nested braces":     {`{{{{term("east")}}}}`, "brace"},
		"less than":         {`{elongation.lon < elongation.lon.abs}`, "comparison `<`"},
		"number":            {`{17}`, "number literal"},
		"bool":              {`{true}`, "bool literal"},
		"string":            {`{"hello"}`, "string literal"},
		"coalesce":          {`{elongation.lon ?? elongation.lon.abs}`, "`??` is not allowed"},
		"member":            {`{elongation.side.x}`, "member access on fact key"},
		"body":              {`{venus}`, "body name"},
		"arithmetic":        {`{elongation.lon + elongation.lon.abs}`, "arithmetic operator"},
		"rule call":         {`{rule("solar.beams")}`, "rule(...) is not a helper"},
		"many operators":    {"{" + strings.Repeat("&& || ! ", 2000) + "}", "The most is 10000"},
		"nul and non-utf8":  {"{\x00\xff\xfe}", "is not allowed in a slot"},
		"quotes everywhere": {`{"""""""}`, "string literal"},
	}
	if len(hostile) < 10 {
		t.Fatal("fewer than ten hostile fixtures")
	}
	nestCalls := func(n int) string {
		return "{" + strings.Repeat("neg(", n-1) + "has(elongation.lon)" + strings.Repeat(")", n-1) + ` ? term("east") : term("west")}`
	}
	ext := pickHelpers(t)
	for n, wantErr := range map[int]bool{32: false, 33: true} {
		_, err := loadAllWith(fsMap(header2+"## @now\n"+nestCalls(n)+"\n", mustAllow()), reg, set(t), ext)
		if (err != nil) != wantErr || (err != nil && (!strings.Contains(err.Error(), "32") || !strings.Contains(err.Error(), "x.md:9:"))) {
			t.Errorf("%d nested calls: %v", n, err)
		}
	}
	if err := load2(t, "{"+strings.Repeat("!", 31)+`has(elongation.lon) ? term("east") : term("west")}`); err != nil {
		t.Errorf("31 ! and the has( call are 32 levels, which is allowed: %v", err)
	}
	if err := load2(t, "{"+strings.Repeat("!", 32)+`has(elongation.lon) ? term("east") : term("west")}`); err == nil {
		t.Error("32 ! and the has( call are 33 levels, which is refused")
	}
	// 31 conditionals chained through else and the last term( call: 32 levels
	if err := load2(t, "{"+strings.Repeat(`has(elongation.lon) ? term("east") : `, 31)+`term("west")}`); err != nil {
		t.Errorf("31 nested ? : and a call are 32 levels, which is allowed: %v", err)
	}
	if err := load2(t, "{"+strings.Repeat(`has(elongation.lon) ? term("east") : `, 32)+`term("west")}`); err == nil {
		t.Error("32 nested ? : and a call are 33 levels, which is refused")
	}
	if err := load2(t, "{"+deep(32)+"}"); err != nil {
		t.Errorf("32 deep is allowed: %v", err)
	}
	if err := load2(t, "{"+strings.Repeat(" ", MaxSlotBytes-len(`term("east")`))+`term("east")}`); err != nil {
		t.Errorf("10000 bytes are allowed: %v", err)
	}
	allow, _ := fsAllow()
	s := set(t)
	for name, tc := range hostile {
		type res struct {
			err   error
			panic any
			took  time.Duration
		}
		done := make(chan res, 1)
		go func() {
			var r res
			start := time.Now()
			defer func() { r.panic, r.took = recover(), time.Since(start); done <- r }()
			_, r.err = loadAll(fsMap(header2+"## @now\n"+tc.src+"\n", allow), reg, s)
		}()
		select {
		case r := <-done:
			switch {
			case r.panic != nil:
				t.Errorf("%s: panic %v", name, r.panic)
			case r.err == nil:
				t.Errorf("%s: loaded", name)
			case !strings.Contains(reason(r.err), tc.want) || !strings.Contains(r.err.Error(), "x.md:9:"):
				t.Errorf("%s: %v, want x.md:9 and %q", name, r.err, tc.want)
			case r.took >= time.Second:
				t.Errorf("%s took %v", name, r.took)
			}
		case <-time.After(time.Second):
			t.Errorf("%s: no answer within a second", name)
		}
	}
}

// T15 — expression slots need format: 2.
func TestFormatTwoGatesExpressions(t *testing.T) {
	one := strings.Replace(header2, "format: 2", "format: 1", 1)
	err := loadOne(t, one+"## @now\n{"+condSide+"}\n")
	if err == nil || !strings.Contains(err.Error(), "x.md:9:") || !strings.Contains(err.Error(), "format: 2") {
		t.Errorf("an expression in format 1: %v", err)
	}
	allow := mustAllow()
	roots := map[string]string{
		"term call":      `{term("east")}`,
		"has cond":       `{has(elongation.lon) ? term("east") : term("west")}`,
		"comparison":     `{elongation.side == term("east") ? term("east") : term("west")}`,
		"paren fact":     `{(elongation.lon)}`,
		"helper call":    `{pick(has(elongation.lon), "east", "west")}`,
		"fact in a cond": `{has(elongation.lon) ? elongation.lon : elongation.lon.abs}`,
	}
	for name, slot := range roots {
		_, err := loadAllWith(fsMap(one+"## @now\n"+slot+"\n", allow), reg, set(t), pickHelpers(t))
		if err == nil || !strings.Contains(err.Error(), "x.md:9:") || !strings.Contains(err.Error(), "format: 2") {
			t.Errorf("%s in format 1: %v", name, err)
		}
		if _, err := loadAllWith(fsMap(header2+"## @now\n"+slot+"\n", allow), reg, set(t), pickHelpers(t)); err != nil {
			t.Errorf("%s in format 2: %v", name, err)
		}
	}
	if err := loadOne(t, one+"## @now\n{term:elongation} {elongation.lon}\n{rule:solar.beams}\n"); err != nil {
		t.Errorf("legacy slots in format 1: %v", err)
	}
	if err := loadOne(t, header2+"## @now\n{term:elongation} {elongation.lon}\n{"+condSide+"}\n"); err != nil {
		t.Errorf("legacy and expression slots in format 2: %v", err)
	}
	ts, _ := Shipped(reg, set(t))
	if b, _ := files.ReadFile("body-inferior-planet.md"); !strings.HasPrefix(string(b), "---\nformat: 2\n") || len(ts) == 0 {
		t.Error("the shipped template does not say format: 2")
	}
}

// T16 — a conditional picks a cited term and is laid out in the same columns.
func TestConditionalPicksTheTerm(t *testing.T) {
	for _, tc := range []struct {
		elong float64
		want  string
	}{{14.87, "evening star"}, {-14.87, "morning star"}} {
		r := render2(t, nowFacts(tc.elong, nil), `{term:elongation} {`+condSide+`}`, `{term:elongation} {elongation.lon}`)
		if len(r.Lines) != 2 || r.Lines[0].Label != "elongation" || r.Lines[0].Value != tc.want {
			t.Fatalf("at %v: %+v", tc.elong, r.Lines)
		}
		laid := Layout(r.Lines, 54)
		legacy := laid[1].Main
		prefix := legacy[:strings.Index(legacy, "1")]
		if got := laid[0].Main; got != prefix+tc.want {
			t.Errorf("at %v: columns differ:\n%q\n%q", tc.elong, got, prefix+tc.want)
		}
		if len(r.Slots) != 1 {
			t.Errorf("slot trace %+v", r.Slots)
		}
	}
	r := render2(t, nowFacts(14.87, nil), `{`+condSide+`}`)
	if got := Layout(r.Lines, 54)[0].Main; got != strings.Repeat(" ", LabelWidth+labelGap)+"evening star" {
		t.Errorf("an unlabelled selection: %q", got)
	}
	if want := (SlotTrace{Line: 9, Slot: 1, Chose: "evening-star"}); len(r.Slots) != 1 || r.Slots[0] != want {
		t.Errorf("trace %+v, want %+v", r.Slots, want)
	}
	// the ordinal counts expression slots only
	r = render2(t, nowFacts(14.87, nil), `{term:elongation} {elongation.lon} {`+condSide+`} {has(elongation.lon) ? elongation.lon : elongation.lon.abs}`)
	if len(r.Slots) != 2 || r.Slots[0].Slot != 1 || r.Slots[1].Slot != 2 || r.Slots[1].Chose != "elongation.lon" {
		t.Errorf("ordinals %+v", r.Slots)
	}
}

// T17 — a literal handed to term or has is validated at load.
func TestHelperArgumentsAreCheckedAtLoad(t *testing.T) {
	for name, tc := range map[string]struct{ slot, helper, value string }{
		"term":   {`term("shiny")`, "term", "shiny"},
		"has":    {`has(elongation.latitude) ? term("east") : term("west")`, "has", "elongation.latitude"},
		"cond":   {`has(elongation.lon) ? term("east") : term("glow")`, "term", "glow"},
		"inside": {`has(elongation.lon) && has(elongation.nope) ? term("east") : term("west")`, "has", "elongation.nope"},
	} {
		err := load2(t, "{"+tc.slot+"}")
		if err == nil || !strings.Contains(err.Error(), "x.md:9:") || !strings.Contains(reason(err), tc.helper+"(") || !strings.Contains(reason(err), tc.value) || !strings.Contains(reason(err), "no such") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Failure modes — a helper handed a value it cannot take is refused at load.
func TestHelperTypesAreCheckedAtLoad(t *testing.T) {
	for name, slot := range map[string]string{
		"== two terms":     `term("east") == term("west") ? term("east") : term("west")`,
		"== value fact":    `elongation.lon == term("east") ? term("east") : term("west")`,
		"== two facts":     `elongation.side == elongation.side ? term("east") : term("west")`,
		"test not bool":    `term("east") ? term("east") : term("west")`,
		"&& on terms":      `term("east") && has(elongation.lon) ? term("east") : term("west")`,
		"! on a term":      `!term("east") ? term("east") : term("west")`,
		"has of a term":    `has(term("east")) ? term("east") : term("west")`,
		"has of a list":    `has(synodic.conjunction.inferior.last5) ? term("east") : term("west")`,
		"term no args":     `term()`,
		"term two args":    `term("east", "west")`,
		"term of a fact":   `term(elongation.side)`,
		"has no args":      `has()`,
		"has two args":     `has(elongation.lon, elongation.lon.abs) ? term("east") : term("west")`,
		"missing else":     `has(elongation.lon) ? term("east")`,
		"missing then":     `has(elongation.lon) ? : term("east")`,
		"unclosed call":    `has(elongation.lon ? term("east") : term("west")`,
		"== on a bool":     `has(elongation.lon) == term("east") ? term("east") : term("west")`,
		"chained ==":       `elongation.side == term("east") == term("west") ? term("east") : term("west")`,
		"cond result bool": `has(elongation.lon) ? (elongation.side == term("east")) : term("west")`,
	} {
		err := load2(t, "{"+slot+"}")
		if err == nil || !strings.Contains(err.Error(), "x.md:9:") {
			t.Errorf("%s: %v", name, err)
		}
	}
}
