package templates

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/cruiserxyz/sextile/internal/rules"
)

// The slot grammar (the specification T3). A slot that is not a
// legacy slot ({term:x}, {rule:x}, {fact.key}) is an expression, parsed and
// typed when the template loads. It can SELECT between cited terms or
// registered facts; it cannot judge: there is no number, no comparison but
// equality, and the only string literal is a term id handed to a helper.
//
//	cond   := or [ "?" cond ":" cond ]          right-associative
//	or     := and { "||" and }
//	and    := cmp { "&&" cmp }
//	cmp    := unary [ ("==" | "!=") unary ]     a category fact against a term
//	unary  := "!" unary | primary
//	primary:= "(" cond ")" | helper "(" args ")" | fact-key
//
// A fact key is one atomic token (factKeyRE: hyphens and dots are part of
// it). Helpers resolve through the HelperRegistry, so a new helper needs no
// change here.

const (
	// MaxSlotBytes is the most text between a slot's braces.
	MaxSlotBytes = 10000
	// MaxNesting is the deepest nesting of parentheses, calls and ? : .
	MaxNesting = 32
)

type vtype int

const (
	tBool  vtype = iota + 1
	tTerm        // a term id
	tCat         // a category fact key
	tVal         // a value/instant/span fact key
	tShown       // a conditional whose branches differ: a term or a fact
)

func (t vtype) String() string {
	return map[vtype]string{tBool: "true/false", tTerm: "a term", tCat: "a category fact", tVal: "a value fact", tShown: "a term or a fact"}[t]
}

type node struct {
	op   string // fact keyref str call not and or eq ne cond
	s    string // fact key, string, helper name
	h    Helper
	kids []*node
	typ  vtype
}

// exprSlot is a parsed expression slot.
type exprSlot struct {
	root *node
	refs []string // every fact key the slot names, once
	n    int      // 1-based ordinal among the line's expression slots
}

type loadCtx struct {
	reg      Registry
	set      rules.Set
	allow    map[rune]bool
	helpers  HelperRegistry
	bodies   []string
	format   int
	reserved map[string]bool
}

// reservedTerms are the terms a conditional may not select: a rule's verdict
// and otherwise, and the yes and no that answer them (T7).
func reservedTerms(set rules.Set) map[string]bool {
	r := map[string]bool{"yes": true, "no": true}
	for _, rule := range set.Rules {
		r[rule.Verdict], r[rule.Otherwise] = true, true
	}
	return r
}

type exprError struct{ msg string }

type tok struct {
	kind byte // w word, s string, o operator, e end
	text string
}

type parser struct {
	toks []tok
	pos  int
	nest int
	ctx  *loadCtx
	refs []string
}

func (p *parser) fail(format string, args ...any) {
	panic(exprError{fmt.Sprintf(format, args...)})
}

// parseExpr parses and types one expression slot's text.
func parseExpr(text string, ctx *loadCtx, ordinal int) (e *exprSlot, err error) {
	defer func() {
		if r := recover(); r != nil {
			ee, ok := r.(exprError)
			if !ok {
				panic(r)
			}
			e, err = nil, fmt.Errorf("%s", ee.msg)
		}
	}()
	if len(text) > MaxSlotBytes {
		return nil, fmt.Errorf("a slot is %d bytes; the most is %d", len(text), MaxSlotBytes)
	}
	p := &parser{ctx: ctx}
	p.toks = p.lex(text)
	if len(p.toks) == 1 {
		p.fail("empty slot")
	}
	root := p.cond()
	if t := p.peek(); t.kind != 'e' {
		p.fail("unexpected %s after the expression", t.show())
	}
	switch root.typ {
	case tTerm, tVal, tShown:
	case tCat:
		p.fail("a category fact may only be compared, not shown by an expression; the slot's result must be a term or a value fact key")
	default:
		p.fail("the slot's result is %s; it must be a term or a value fact key", root.typ)
	}
	return &exprSlot{root: root, refs: p.refs, n: ordinal}, nil
}

func (t tok) show() string {
	switch t.kind {
	case 'e':
		return "the end of the slot"
	case 's':
		return fmt.Sprintf("string literal %q", t.text)
	}
	return "`" + t.text + "`"
}

func isWordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-'
}

func (p *parser) lex(s string) []tok {
	var out []tok
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			i++
		case c == '"':
			j := strings.IndexByte(s[i+1:], '"')
			if j < 0 {
				p.fail("unterminated string literal")
			}
			out = append(out, tok{'s', s[i+1 : i+1+j]})
			i += j + 2
		case c == '-' || c == '+' || c == '*' || c == '/' || c == '%':
			p.fail("arithmetic operator `%c` is not allowed: a slot selects, it does not calculate", c)
		case isWordByte(c):
			j := i
			for j < len(s) && isWordByte(s[j]) {
				j++
			}
			out = append(out, tok{'w', s[i:j]})
			i = j
		default:
			two := ""
			if i+1 < len(s) {
				two = s[i : i+2]
			}
			switch {
			case two == "==" || two == "!=" || two == "&&" || two == "||":
				out = append(out, tok{'o', two})
				i += 2
			case two == "??":
				p.fail("`??` is not allowed: use has(...) in the test")
			case two == "<=" || two == ">=":
				p.fail("comparison `%s` is not allowed: a threshold is a rules/ row", two)
			case c == '<' || c == '>':
				p.fail("comparison `%c` is not allowed: a threshold is a rules/ row", c)
			case c == '=':
				p.fail("`=` is not allowed (equality is `==`)")
			case c == '(' || c == ')' || c == '?' || c == ':' || c == ',' || c == '!':
				out = append(out, tok{'o', string(c)})
				i++
			case c == '[' || c == ']':
				p.fail("index `%c` is not allowed", c)
			case c == '\'':
				p.fail("string literal in single quotes is not allowed")
			default:
				r, _ := utf8.DecodeRuneInString(s[i:])
				p.fail("character %q is not allowed in a slot", r)
			}
		}
	}
	return append(out, tok{kind: 'e'})
}

func (p *parser) peek() tok { return p.toks[p.pos] }

func (p *parser) next() tok {
	t := p.toks[p.pos]
	if t.kind != 'e' {
		p.pos++
	}
	return t
}

func (p *parser) isOp(s string) bool { t := p.peek(); return t.kind == 'o' && t.text == s }

func (p *parser) expect(s string) {
	if !p.isOp(s) {
		p.fail("expected `%s`, found %s", s, p.peek().show())
	}
	p.pos++
}

func (p *parser) enter() {
	p.nest++
	if p.nest > MaxNesting {
		p.fail("nests deeper than %d levels", MaxNesting)
	}
}

func (p *parser) leave() { p.nest-- }

func (p *parser) cond() *node {
	test := p.or()
	if !p.isOp("?") {
		return test
	}
	p.pos++
	if test.typ != tBool {
		p.fail("the test of ? : is %s; it must be true/false", test.typ)
	}
	p.enter()
	a := p.cond()
	p.expect(":")
	b := p.cond()
	p.leave()
	for _, br := range []*node{a, b} {
		switch br.typ {
		case tTerm, tVal, tShown:
		case tCat:
			p.fail("a branch of ? : is a category fact; a category fact may only be compared, a branch must be a term or a value fact key")
		default:
			p.fail("a branch of ? : is %s; it must be a term or a value fact key", br.typ)
		}
		for _, id := range resultTerms(br) {
			if p.ctx.reserved[id] {
				p.fail("term %q is reserved (a rule uses it as a verdict, or it answers one): a conditional may not select it", id)
			}
		}
	}
	t := tShown
	if a.typ == b.typ && a.typ == tTerm {
		t = tTerm
	}
	return &node{op: "cond", kids: []*node{test, a, b}, typ: t}
}

// resultTerms are the term ids a node can evaluate to as its result.
func resultTerms(n *node) []string {
	switch n.op {
	case "cond":
		return append(resultTerms(n.kids[1]), resultTerms(n.kids[2])...)
	case "call":
		if n.h.Result != ResTerm {
			return nil
		}
		var out []string
		for i, k := range n.kids {
			if n.h.Args[i] == ArgTermID {
				out = append(out, k.s)
			}
		}
		return out
	}
	return nil
}

func (p *parser) or() *node {
	l := p.and()
	for p.isOp("||") {
		p.pos++
		r := p.and()
		p.needBool("||", l, r)
		l = &node{op: "or", kids: []*node{l, r}, typ: tBool}
	}
	return l
}

func (p *parser) and() *node {
	l := p.cmp()
	for p.isOp("&&") {
		p.pos++
		r := p.cmp()
		p.needBool("&&", l, r)
		l = &node{op: "and", kids: []*node{l, r}, typ: tBool}
	}
	return l
}

func (p *parser) needBool(op string, ns ...*node) {
	for _, n := range ns {
		if n.typ != tBool {
			p.fail("`%s` takes true/false operands, found %s", op, n.typ)
		}
	}
}

func (p *parser) cmp() *node {
	l := p.unary()
	if !p.isOp("==") && !p.isOp("!=") {
		return l
	}
	op := p.next().text
	r := p.unary()
	cat, term := l, r
	if cat.typ == tTerm {
		cat, term = r, l
	}
	if cat.op != "fact" || cat.typ != tCat || term.typ != tTerm || term.op != "call" || term.s != "term" {
		p.fail("`%s` compares a category fact with term(...); found %s and %s", op, l.typ, r.typ)
	}
	n := &node{op: "eq", kids: []*node{cat, term}, typ: tBool}
	if op == "!=" {
		n.op = "ne"
	}
	return n
}

func (p *parser) unary() *node {
	if p.isOp("!") {
		p.pos++
		p.enter()
		k := p.unary()
		p.leave()
		p.needBool("!", k)
		return &node{op: "not", kids: []*node{k}, typ: tBool}
	}
	return p.primary()
}

func (p *parser) primary() *node {
	t := p.next()
	switch {
	case t.kind == 'o' && t.text == "(":
		p.enter()
		n := p.cond()
		p.leave()
		p.expect(")")
		return n
	case t.kind == 's':
		p.fail("string literal %q outside term(...): every word arrives through a term", t.text)
	case t.kind == 'w':
		return p.word(t.text)
	}
	p.fail("unexpected %s", t.show())
	return nil
}

// word classifies an identifier: a body name, a bool or number literal, a
// call, a fact key, or something unknown.
func (p *parser) word(w string) *node {
	for _, b := range p.ctx.bodies {
		if strings.EqualFold(w, b) {
			p.fail("body name %q is not allowed: a template is for a class of bodies", w)
		}
	}
	switch {
	case w == "true" || w == "false":
		p.fail("bool literal %q is not allowed", w)
	case w[0] >= '0' && w[0] <= '9':
		p.fail("number literal %q is not allowed: a threshold is a rules/ row", w)
	case p.isOp("("):
		return p.call(w)
	case factKeyRE.MatchString(w):
		return p.fact(w, "")
	case strings.Contains(w, "."):
		p.fail("member access %q is not allowed: only a whole fact key may be named", w)
	}
	p.fail("unknown identifier %q (a term is written term(%q))", w, w)
	return nil
}

// fact resolves a fact key; helper names the call it is the argument of.
func (p *parser) fact(key, helper string) *node {
	what := key
	if helper != "" {
		what = helper + "(" + key + ")"
	}
	k, ok := p.ctx.reg.FactKind(key)
	if !ok {
		if hyphenSplitsAtKey(key, p.ctx.reg) {
			p.fail("arithmetic operator `-` is not allowed: a slot selects, it does not calculate (%q)", key)
		}
		for prefix := key; strings.Contains(prefix, "."); {
			prefix = prefix[:strings.LastIndex(prefix, ".")]
			if _, ok := p.ctx.reg.FactKind(prefix); ok {
				p.fail("%s: member access on fact key %q is not allowed", what, prefix)
			}
		}
		p.fail("%s: no such fact in the registry", what)
	}
	if k == KindList {
		p.fail("%s: a list fact stands alone on its line as a legacy slot, not in an expression", what)
	}
	seen := false
	for _, r := range p.refs {
		seen = seen || r == key
	}
	if !seen {
		p.refs = append(p.refs, key)
	}
	typ := tVal
	if k == KindCategory {
		typ = tCat
	}
	return &node{op: "fact", s: key, typ: typ}
}

func (p *parser) call(name string) *node {
	if name == "rule" {
		p.fail("rule(...) is not a helper: {rule:x} stands alone on its line")
	}
	h, ok := p.ctx.helpers.Lookup(name)
	if !ok {
		p.fail("no such function %q", name)
	}
	p.pos++ // (
	p.enter()
	n := &node{op: "call", s: name, h: h}
	for i, kind := range h.Args {
		if i > 0 {
			if !p.isOp(",") {
				p.fail("%s takes %d arguments", name, len(h.Args))
			}
			p.pos++
		} else if p.isOp(")") {
			p.fail("%s takes %d arguments", name, len(h.Args))
		}
		switch kind {
		case ArgTermID:
			t := p.next()
			if t.kind != 's' {
				p.fail("%s takes %s, found %s", name, kind, t.show())
			}
			if _, ok := p.ctx.set.Terms[t.text]; !ok {
				p.fail("%s(%q): no such term in rules/terms.yaml", name, t.text)
			}
			n.kids = append(n.kids, &node{op: "str", s: t.text})
		case ArgFactKey:
			t := p.next()
			if t.kind != 'w' || p.isOp("(") {
				p.fail("%s takes %s, found %s", name, kind, t.show())
			}
			f := p.fact(t.text, name)
			n.kids = append(n.kids, &node{op: "keyref", s: f.s})
		case ArgBool:
			a := p.cond()
			if a.typ != tBool {
				p.fail("%s takes %s, found %s", name, kind, a.typ)
			}
			n.kids = append(n.kids, a)
		}
	}
	if !p.isOp(")") {
		p.fail("%s takes %d arguments, found %s after them", name, len(h.Args), p.peek().show())
	}
	p.pos++
	p.leave()
	n.typ = tBool
	if h.Result == ResTerm {
		n.typ = tTerm
	}
	return n
}

// evaluation

type val struct {
	b    bool
	term string // a term id
	key  string // a fact key
}

// eval evaluates the slot; ok is false when evaluation reached an absent
// fact, which omits the line (T8). Evaluation is left to right and && / ||
// short-circuit, so nothing is evaluated on an absent value that a guard
// already ruled out.
func (e *exprSlot) eval(facts Section) (v val, ok bool) {
	return evalNode(e.root, facts)
}

func evalNode(n *node, facts Section) (val, bool) {
	switch n.op {
	case "str":
		return val{term: n.s}, true
	case "keyref":
		return val{key: n.s}, true
	case "fact":
		f, _ := facts.Fact(n.s)
		if f.Absent {
			return val{}, false
		}
		return val{key: n.s}, true
	case "not":
		v, ok := evalNode(n.kids[0], facts)
		return val{b: !v.b}, ok
	case "and", "or":
		l, ok := evalNode(n.kids[0], facts)
		if !ok || l.b == (n.op == "or") {
			return val{b: l.b}, ok
		}
		return evalNode(n.kids[1], facts)
	case "eq", "ne":
		f, _ := facts.Fact(n.kids[0].s)
		if f.Absent {
			return val{}, false
		}
		want, ok := evalNode(n.kids[1], facts)
		if !ok {
			return val{}, false
		}
		return val{b: (f.Term == want.term) == (n.op == "eq")}, true
	case "cond":
		t, ok := evalNode(n.kids[0], facts)
		if !ok {
			return val{}, false
		}
		if t.b {
			return evalNode(n.kids[1], facts)
		}
		return evalNode(n.kids[2], facts)
	case "call":
		args := make([]Operand, len(n.kids))
		for i, k := range n.kids {
			v, ok := evalNode(k, facts)
			if !ok {
				return val{}, false
			}
			args[i] = Operand{Bool: v.b, Str: v.term}
			if k.op == "keyref" {
				f, _ := facts.Fact(k.s)
				args[i] = Operand{Str: k.s, Bool: !f.Absent}
			}
		}
		r := n.h.Eval(args)
		if n.h.Result == ResTerm {
			return val{term: r.Str}, true
		}
		return val{b: r.Bool}, true
	}
	return val{}, false
}

// hyphenSplitsAtKey: an unregistered token that is a registered fact key, a
// hyphen and something more reads as subtraction, not as one atomic key.
// Registered hyphenated keys (sign.same-as-sun) stay atomic.
func hyphenSplitsAtKey(tok string, reg Registry) bool {
	if _, ok := reg.FactKind(tok); ok {
		return false
	}
	for i := 0; i < len(tok); i++ {
		if tok[i] != '-' {
			continue
		}
		if _, ok := reg.FactKind(tok[:i]); ok {
			return true
		}
	}
	return false
}
