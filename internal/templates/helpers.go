package templates

import (
	"fmt"
	"sort"
)

// HelperKind says what a helper does to a template's words, which decides
// what it must be accompanied by (the specification T9).
type HelperKind string

const (
	// HelperSelect chooses between words that already exist: a cited term or a
	// registered fact. Nothing new reaches the screen.
	HelperSelect HelperKind = "select"
	// HelperFormat changes how a registered fact is written.
	HelperFormat HelperKind = "format"
	// HelperDerivation computes a number no registered fact holds. It needs a
	// `// derivation:` marker in this package and a row in the divergence
	// ledger (the design notes) before it can be
	// registered; v1 has none.
	HelperDerivation HelperKind = "derivation"
)

// ArgKind is what one argument of a helper is.
type ArgKind int

const (
	// ArgTermID is a string literal naming a term in rules/terms.yaml, the
	// only place a string literal may stand.
	ArgTermID ArgKind = iota + 1
	// ArgFactKey is a bare, registered, non-list fact key.
	ArgFactKey
	// ArgBool is an expression whose type is bool.
	ArgBool
)

func (a ArgKind) String() string {
	switch a {
	case ArgTermID:
		return "a term id in quotes"
	case ArgFactKey:
		return "a fact key"
	case ArgBool:
		return "a true/false expression"
	}
	return "?"
}

// ResultKind is what a helper's call evaluates to.
type ResultKind int

const (
	ResBool ResultKind = iota + 1
	ResTerm
)

// Operand is one evaluated argument or result. For ArgTermID, Str is the
// term id; for ArgFactKey, Str is the key and Bool says the fact is present;
// for ArgBool, Bool is the value. A ResTerm result carries the term id in Str.
type Operand struct {
	Bool bool
	Str  string
}

// Helper is one function a slot may call. The parser resolves a call through
// the registry and types it from Args and Result; only the helper's own Eval
// knows what it does.
type Helper struct {
	Name   string
	Args   []ArgKind
	Result ResultKind
	Kind   HelperKind
	Eval   func(args []Operand) Operand
}

// HelperRegistry is the helpers a slot may call.
type HelperRegistry struct{ byName map[string]Helper }

// DefaultHelpers is a fresh registry holding the v1 helpers, `term` and
// `has`. A test registers on its own copy and never on the shipped one.
func DefaultHelpers() HelperRegistry {
	r := HelperRegistry{byName: map[string]Helper{}}
	for _, h := range []Helper{
		{Name: "term", Args: []ArgKind{ArgTermID}, Result: ResTerm, Kind: HelperSelect,
			Eval: func(a []Operand) Operand { return Operand{Str: a[0].Str} }},
		{Name: "has", Args: []ArgKind{ArgFactKey}, Result: ResBool, Kind: HelperSelect,
			Eval: func(a []Operand) Operand { return Operand{Bool: a[0].Bool} }},
	} {
		if err := r.Register(h); err != nil {
			panic(err)
		}
	}
	return r
}

// Register adds a helper, refusing a name already taken or a malformed one.
func (r HelperRegistry) Register(h Helper) error {
	if !idRE.MatchString(h.Name) {
		return fmt.Errorf("helper %q: not a lower-case name", h.Name)
	}
	if _, dup := r.byName[h.Name]; dup {
		return fmt.Errorf("helper %q registered twice", h.Name)
	}
	switch h.Kind {
	case HelperSelect, HelperFormat, HelperDerivation:
	default:
		return fmt.Errorf("helper %q: kind %q is not select, format or derivation", h.Name, h.Kind)
	}
	if h.Result != ResBool && h.Result != ResTerm {
		return fmt.Errorf("helper %q: no result kind", h.Name)
	}
	if h.Eval == nil {
		return fmt.Errorf("helper %q: no Eval", h.Name)
	}
	r.byName[h.Name] = h
	return nil
}

// Lookup is the helper called name.
func (r HelperRegistry) Lookup(name string) (Helper, bool) {
	h, ok := r.byName[name]
	return h, ok
}

// All is every helper, by name.
func (r HelperRegistry) All() []Helper {
	out := make([]Helper, 0, len(r.byName))
	for _, h := range r.byName {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Clone is an independent copy, so a test can register on it.
func (r HelperRegistry) Clone() HelperRegistry {
	c := HelperRegistry{byName: make(map[string]Helper, len(r.byName))}
	for k, v := range r.byName {
		c.byName[k] = v
	}
	return c
}
