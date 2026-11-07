// the specification C1: every pickable row in the four list modals is
// drawn by the one shared row renderer, and nothing between that renderer and
// the modal box pads, truncates or restyles it.
//
// The second half is the one a behavioural test cannot hold. The renderer
// promises a line exactly `width` cells wide with its stripe running to the
// last cell; a caller that trims the line afterwards, or pads it, or wraps it
// in a style, silently breaks the stripe or the right edge on some row at some
// width, and every unit test of the renderer still passes. So this follows the
// code, not the output.
package ui

import (
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// listRowRenderer is the shared renderer's name. The walk stops at it: what it
// does inside is C2–C6's business.
const listRowRenderer = "menu.Row"

// listModalRoots maps each list modal's entry point to the row builders the
// spec names for it. Each builder must call the renderer directly.
var listModalRoots = map[string][]string{
	"popup.Atlas.Render":   {"ResultList"},
	"renderChartModal":     {"chartRows"},
	"popup.Aspects.Render": {"AspectFamilyRows", "AspectListRows"},
	"popup.Render":         {"ListRows"},
	"keys.RenderHelp":      {"helpListRows"},
}

// listNonRowHelpers draw lines that are NOT pickable rows — a placeholder, the
// unreadable-file rows, the body tag bar — and so may pad and style their own
// text. The walk does not descend into them.
//
// The boundary, stated so nobody reads more into a pass: this is a list of
// names, not a data-flow proof. A pickable row handed to one of these would
// not be caught. Each one's signature takes text it builds itself or a plain
// message, never a rendered row, and adding a name here is the deliberate act
// that would let one through.
var listNonRowHelpers = map[string]bool{
	"menu.Note":      true,
	"menu.Heading":   true,
	"menu.Message":   true,
	"chartDropBlock": true,
	"chartDropLines": true,
	"fitDropLines":   true,
	"TagBar":         true,
	// The aspect chooser's decision whether its rows keep the orb: a width
	// measured over the registry, a number and never a line.
	"aspectFullWidth": true,
	// The dashed rule under a pinned block (the specification 6):
	// built from a glyph and a width, never from a rendered row.
	"menu.Rule": true,
}

// forbiddenRowCalls are the ways a line gets padded, truncated or restyled.
// Bare identifiers are package-local helpers; "pkg.Name" are qualified calls;
// ".Name" is any method of that name (a lipgloss.Style's Render).
var forbiddenRowCalls = []string{
	"padOrTrim",
	"truncateRow",
	"ansi.Truncate",
	"ansi.TruncateLeft",
	"ansi.StringWidth",
	"lipgloss.*",
	"strings.Trim*",
	"strings.Repeat",
	".Render",
}

func TestListModalsShareOneRowRenderer(t *testing.T) {
	dir := "."
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	funcs := map[string][]*ast.FuncDecl{}
	imports := map[string]bool{}
	// The bodies popup's rows are drawn in internal/ui/popup. Its functions join
	// the walk, except the Render method: a method named like lipgloss's would
	// make every `.Render` call look local, so it is kept apart as a root.
	roots := map[string][]*ast.FuncDecl{}
	// The Keys popup is drawn in internal/ui/keys: its functions join the walk,
	// and RenderHelp, the box's caller, is a root.
	for _, sub := range []string{"popup", "keys"} {
		subEntries, err := os.ReadDir(sub)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range subEntries {
			if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, filepath.Join(sub, e.Name()), nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range f.Imports {
				p := strings.Trim(imp.Path.Value, `"`)
				imports[p[strings.LastIndex(p, "/")+1:]] = true
			}
			for _, d := range f.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok {
					if sub == "keys" && fd.Name.Name == "RenderHelp" {
						roots["keys.RenderHelp"] = append(roots["keys.RenderHelp"], fd)
						continue
					}
					if fd.Name.Name == "Render" && fd.Recv != nil {
						key := "popup.Render"
						t := fd.Recv.List[0].Type
						if ix, ok := t.(*ast.IndexExpr); ok { // a generic receiver: Finder[T]
							t = ix.X
						}
						if id, ok := t.(*ast.Ident); ok && id.Name != "Body" {
							key = "popup." + id.Name + ".Render"
						}
						roots[key] = append(roots[key], fd)
						continue
					}
					funcs[fd.Name.Name] = append(funcs[fd.Name.Name], fd)
				}
			}
		}
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			local := p[strings.LastIndex(p, "/")+1:]
			if imp.Name != nil {
				local = imp.Name.Name
			}
			imports[local] = true
		}
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok {
				funcs[fd.Name.Name] = append(funcs[fd.Name.Name], fd)
			}
		}
	}

	rendererCalled := false
	for _, ds := range funcs {
		for _, fd := range ds {
			if fd.Body == nil {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
						if x, ok := sel.X.(*ast.Ident); ok && x.Name+"."+sel.Sel.Name == listRowRenderer {
							rendererCalled = true
						}
					}
				}
				return true
			})
		}
	}
	if !rendererCalled {
		t.Fatalf("internal/ui calls no %s: the list modals share no row renderer", listRowRenderer)
	}

	// callName names a call the way forbiddenRowCalls spells it, and reports
	// whether it is a package-local function or method the walk can follow.
	callName := func(call *ast.CallExpr) (name string, local bool) {
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			return fn.Name, len(funcs[fn.Name]) > 0
		case *ast.SelectorExpr:
			if x, ok := fn.X.(*ast.Ident); ok && imports[x.Name] {
				return x.Name + "." + fn.Sel.Name, false
			}
			return "." + fn.Sel.Name, len(funcs[fn.Sel.Name]) > 0
		}
		return "", false
	}
	forbidden := func(name string) bool {
		for _, f := range forbiddenRowCalls {
			if f == name {
				return true
			}
			if strings.HasSuffix(f, "*") && strings.HasPrefix(name, strings.TrimSuffix(f, "*")) {
				return true
			}
		}
		return false
	}
	bare := func(name string) string { return strings.TrimPrefix(name, ".") }

	for rootName, builders := range listModalRoots {
		decls := funcs[rootName]
		if r, isRoot := roots[rootName]; isRoot {
			decls = r
		}
		if len(decls) != 1 {
			t.Errorf("%s: want one declaration, found %d", rootName, len(decls))
			continue
		}

		// The list closure handed to dialog.Render is where the row path
		// starts; everything the box draws as list content comes out of it.
		var listFn *ast.FuncLit
		ast.Inspect(decls[0].Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Render" && len(call.Args) > 0 {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "dialog" {
					listFn, _ = call.Args[len(call.Args)-1].(*ast.FuncLit)
				}
			}
			return true
		})
		if listFn == nil {
			t.Errorf("%s: no list closure passed to renderModalBox", rootName)
			continue
		}

		reached := map[string]bool{}
		var violations []string
		var walk func(where string, body ast.Node)
		walk = func(where string, body ast.Node) {
			ast.Inspect(body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name, local := callName(call)
				if forbidden(name) {
					violations = append(violations, where+" calls "+name+" ("+fset.Position(call.Pos()).String()+")")
				}
				target := bare(name)
				if !local || target == listRowRenderer || listNonRowHelpers[target] || reached[target] {
					reached[target] = reached[target] || target == listRowRenderer
					return true
				}
				reached[target] = true
				for _, fd := range funcs[target] {
					if fd.Body != nil {
						walk(target, fd.Body)
					}
				}
				return true
			})
		}
		walk(rootName+"'s list closure", listFn.Body)

		// Every function on the path — the builders and the parents that
		// stack their output — may only pass lines along, never make or
		// alter one: what it adds must come straight from the renderer, a
		// non-row helper, or another function on the path.
		var onPath []string
		for name := range reached {
			if len(funcs[name]) > 0 && name != listRowRenderer && !listNonRowHelpers[name] {
				onPath = append(onPath, name)
			}
		}
		sort.Strings(onPath)
		pathOK := func(name string) bool {
			return name == listRowRenderer || listNonRowHelpers[name] || reached[name] && len(funcs[name]) > 0
		}
		for _, bad := range unrenderedLines(listFn.Body, callName, pathOK) {
			t.Errorf("%s's list closure adds a line that did not come from %s, a non-row helper or the path: %s", rootName, listRowRenderer, bad)
		}
		for _, name := range onPath {
			for _, fd := range funcs[name] {
				// A function that returns a menu.RowSpec builds CELLS for
				// the renderer to lay out; its []string are not lines.
				if fd.Body == nil || returnsRowSpec(fd) {
					continue
				}
				for _, bad := range unrenderedLines(fd.Body, callName, pathOK) {
					t.Errorf("%s: %s adds a line that did not come from %s, a non-row helper or the path: %s",
						rootName, name, listRowRenderer, bad)
				}
			}
		}

		sort.Strings(violations)
		for _, v := range violations {
			t.Errorf("%s: on the row path, %s — a row must leave %s exactly as the box draws it", rootName, v, listRowRenderer)
		}
		if !reached[listRowRenderer] {
			t.Errorf("%s: its rows never reach %s", rootName, listRowRenderer)
		}
		for _, b := range builders {
			if !reached[b] {
				t.Errorf("%s: the row builder %s is not on its list path", rootName, b)
				continue
			}
			calls := false
			for _, fd := range funcs[b] {
				ast.Inspect(fd.Body, func(n ast.Node) bool {
					if call, ok := n.(*ast.CallExpr); ok {
						if name, _ := callName(call); bare(name) == listRowRenderer {
							calls = true
						}
					}
					return true
				})
			}
			if !calls {
				t.Errorf("%s builds rows without calling %s", b, listRowRenderer)
			}

		}
	}
}

// unrenderedLines finds every line a function on the row path adds — each
// argument appended to a slice, each element of a returned slice literal, each
// value stored into a slice element — that is not, directly, a call to an
// allowed function or a slice passed along whole (`rows...`). Containing a
// menu.Row call somewhere is not enough: a builder could draw its rows through
// the renderer on one branch and append a raw row on another, and a parent
// could add a cell to a line it was handed.
func unrenderedLines(body ast.Node, callName func(*ast.CallExpr) (string, bool), allowed func(string) bool) []string {
	ok := func(e ast.Expr) bool {
		call, isCall := e.(*ast.CallExpr)
		if !isCall {
			return false
		}
		name, _ := callName(call)
		return allowed(strings.TrimPrefix(name, "."))
	}
	// Lines are []string. Without type-checking the package, a slice counts
	// as lines when this function declares it as one: a []string parameter,
	// `var x []string`, `make([]string, …)` or a `[]string{…}` literal. Every
	// line slice in the modals is declared one of those ways.
	isStrings := func(e ast.Expr) bool {
		at, ok := e.(*ast.ArrayType)
		if !ok || at.Len != nil {
			return false
		}
		id, ok := at.Elt.(*ast.Ident)
		return ok && id.Name == "string"
	}
	lines := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Field:
			if isStrings(x.Type) {
				for _, nm := range x.Names {
					lines[nm.Name] = true
				}
			}
		case *ast.ValueSpec:
			if x.Type != nil && isStrings(x.Type) {
				for _, nm := range x.Names {
					lines[nm.Name] = true
				}
			}
		case *ast.AssignStmt:
			for k, r := range x.Rhs {
				id, isIdent := x.Lhs[min(k, len(x.Lhs)-1)].(*ast.Ident)
				if !isIdent {
					continue
				}
				if lit, isLit := r.(*ast.CompositeLit); isLit && isStrings(lit.Type) {
					lines[id.Name] = true
				}
				if call, isCall := r.(*ast.CallExpr); isCall && len(call.Args) > 0 {
					if fn, isFn := call.Fun.(*ast.Ident); isFn && fn.Name == "make" && isStrings(call.Args[0]) {
						lines[id.Name] = true
					}
				}
			}
		}
		return true
	})
	isLines := func(e ast.Expr) bool {
		if lit, ok := e.(*ast.CompositeLit); ok {
			return isStrings(lit.Type)
		}
		if ix, ok := e.(*ast.IndexExpr); ok {
			e = ix.X
		}
		if sl, ok := e.(*ast.SliceExpr); ok {
			e = sl.X
		}
		id, ok := e.(*ast.Ident)
		return ok && lines[id.Name]
	}

	var bad []string
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if id, isIdent := x.Fun.(*ast.Ident); isIdent && id.Name == "append" && len(x.Args) > 1 && isLines(x.Args[0]) {
				for k, a := range x.Args[1:] {
					spread := x.Ellipsis.IsValid() && k == len(x.Args)-2
					if spread {
						// A slice passed along whole: a path function's
						// result, or a field of a value handed in
						// (menu.PinnedBody's b.Rows...). A local slice is not
						// passed along — it was made here, and a row in it
						// bypassed the renderer (round-3 review).
						if _, isField := a.(*ast.SelectorExpr); isField || ok(a) {
							continue
						}
					}
					if !ok(a) {
						bad = append(bad, "append of "+rowExprString(a))
					}
				}
			}
		case *ast.AssignStmt:
			for k, l := range x.Lhs {
				if ix, isIndex := l.(*ast.IndexExpr); isIndex && isLines(ix.X) && k < len(x.Rhs) && !ok(x.Rhs[k]) {
					bad = append(bad, "store of "+rowExprString(x.Rhs[k]))
				}
			}
		case *ast.ReturnStmt:
			for _, r := range x.Results {
				if lit, isLit := r.(*ast.CompositeLit); isLit && isStrings(lit.Type) {
					for _, el := range lit.Elts {
						if !ok(el) {
							bad = append(bad, "return of "+rowExprString(el))
						}
					}
				}
			}
		}
		return true
	})
	return bad
}

func rowExprString(e ast.Expr) string {
	var b strings.Builder
	if err := format.Node(&b, token.NewFileSet(), e); err != nil {
		return "?"
	}
	return b.String()
}

func returnsRowSpec(fd *ast.FuncDecl) bool {
	if fd.Type.Results == nil {
		return false
	}
	for _, r := range fd.Type.Results.List {
		if sel, ok := r.Type.(*ast.SelectorExpr); ok && sel.Sel.Name == "RowSpec" {
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == "menu" {
				return true
			}
		}
	}
	return false
}
