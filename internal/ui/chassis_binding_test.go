package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// requirement C8 / C8a (the specification). These are the structural half of
// the chassis lift; the behavioural half is the goldens in
// atlas_lift_golden_test.go and chart_box_golden_test.go being byte-identical
// across it.

func parseUIFile(t *testing.T, name string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return fset, f
}

// listStateFields are the pieces of a filtered picker that the chassis owns.
// A caller declaring any of them directly has its own list again, which is the
// duplication this lift exists to end.
var listStateFields = []string{"filterText", "cursor", "matches", "truncated"}

// C8a — the atlas picker binds the shared chassis rather than carrying its own
// list, cursor and search buffer.
func TestAtlasModalBindsChassis(t *testing.T) {
	_, f := parseUIFile(t, "popup/atlas.go")

	var state *ast.StructType
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != "Atlas" {
			return true
		}
		if st, ok := ts.Type.(*ast.StructType); ok {
			state = st
		}
		return false
	})
	if state == nil {
		t.Fatal("Atlas not found in popup/atlas.go")
	}

	embedsChassis := false
	declared := map[string]bool{}
	for _, fld := range state.Fields.List {
		if len(fld.Names) == 0 { // embedded
			if isMenuFuzzyList(fld.Type) {
				embedsChassis = true
			}
			continue
		}
		for _, n := range fld.Names {
			declared[n.Name] = true
		}
	}

	if !embedsChassis {
		t.Error("popup.Atlas does not embed the menu.FuzzyList chassis")
	}
	for _, name := range listStateFields {
		if declared[name] {
			t.Errorf("popup.Atlas declares %q itself; it must come from the chassis", name)
		}
	}
}

// C8 — the chassis names no domain concept, so a second caller can bind it.
// The word is spelled in pieces so this test does not fail itself.
func TestChassisIsAspectAgnostic(t *testing.T) {
	_, f := parseUIFile(t, "menu/fuzzy_checklist.go")
	forbidden := "as" + "pect"

	ast.Inspect(f, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if strings.Contains(strings.ToLower(id.Name), forbidden) {
			t.Errorf("chassis identifier %q names the domain it must not know about", id.Name)
		}
		return true
	})

	for _, imp := range f.Imports {
		if strings.Contains(strings.ToLower(imp.Path.Value), forbidden) {
			t.Errorf("chassis imports %s", imp.Path.Value)
		}
	}
}

// C8 — the chassis exposes a pinned block above the rows and one below, and
// renders with both empty (how a plain picker uses it) and both populated (how
// the checklist modals will).
func TestChassisPinnedBlocks(t *testing.T) {
	const width, height = 40, 6

	empty := menu.RenderPinnedBody(menu.PinnedBody{Rows: []string{"one", "two"}}, width, height)
	if lines := strings.Count(empty, "\n") + 1; lines != height {
		t.Errorf("empty-blocks body is %d lines, want %d", lines, height)
	}
	if !strings.Contains(empty, "one") || !strings.Contains(empty, "two") {
		t.Error("empty-blocks body dropped its rows")
	}

	full := menu.RenderPinnedBody(menu.PinnedBody{
		Above: []string{"GROUPS"},
		Rows:  []string{"one", "two"},
		Below: []string{"ENABLED: a b c"},
	}, width, height)
	if lines := strings.Count(full, "\n") + 1; lines != height {
		t.Errorf("populated body is %d lines, want %d", lines, height)
	}
	for _, want := range []string{"GROUPS", "one", "two", "ENABLED: a b c"} {
		if !strings.Contains(full, want) {
			t.Errorf("populated body dropped %q", want)
		}
	}

	// The pinned tail survives a list long enough to fill the box — the
	// property that makes a tag bar trustworthy.
	crowded := menu.RenderPinnedBody(menu.PinnedBody{
		Above: []string{"GROUPS"},
		Rows:  []string{"1", "2", "3", "4", "5", "6", "7", "8"},
		Below: []string{"TAGBAR"},
	}, width, height)
	if !strings.Contains(crowded, "TAGBAR") {
		t.Error("a full list squeezed out the pinned tail")
	}
	if lines := strings.Count(crowded, "\n") + 1; lines != height {
		t.Errorf("crowded body is %d lines, want %d", lines, height)
	}
}

func exprString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.IndexExpr:
		return exprString(v.X)
	case *ast.SelectorExpr:
		return exprString(v.X) + "." + v.Sel.Name
	case *ast.StarExpr:
		return exprString(v.X)
	}
	return ""
}

// C6's structural clause (the specification) — the body selector binds the
// same chassis and declares no list, cursor or search editor of its own.
//
// It rides ALONGSIDE the behavioural TestBodySelectorReusesChassis, and neither
// is sufficient alone: construction satisfies this one while a parallel list
// sits beside it, and the behavioural one could pass against a modal that
// embedded the chassis and then shadowed its fields. Together with C8a above,
// all three callers are held to the same clause, so none of them can grow a
// third implementation.
func TestBodyModalBindsChassis(t *testing.T) {
	_, f := parseUIFile(t, "popup/body.go")

	var state *ast.StructType
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != "Body" {
			return true
		}
		if st, ok := ts.Type.(*ast.StructType); ok {
			state = st
		}
		return false
	})
	if state == nil {
		t.Fatal("Body not found in popup/body.go")
	}

	embedsChassis := false
	declared := map[string]bool{}
	for _, fld := range state.Fields.List {
		if len(fld.Names) == 0 { // embedded
			if isMenuFuzzyList(fld.Type) {
				embedsChassis = true
			}
			continue
		}
		for _, n := range fld.Names {
			declared[n.Name] = true
		}
	}

	if !embedsChassis {
		t.Error("popup.Body does not embed the menu.FuzzyList chassis")
	}
	for _, name := range listStateFields {
		if declared[name] {
			t.Errorf("popup.Body declares %q itself; it must come from the chassis", name)
		}
	}
}

// isMenuFuzzyList reports whether an embedded field's type is menu.FuzzyList,
// with or without type arguments or a pointer.
func isMenuFuzzyList(e ast.Expr) bool {
	switch t := e.(type) {
	case *ast.StarExpr:
		return isMenuFuzzyList(t.X)
	case *ast.IndexExpr:
		return isMenuFuzzyList(t.X)
	case *ast.IndexListExpr:
		return isMenuFuzzyList(t.X)
	case *ast.SelectorExpr:
		x, ok := t.X.(*ast.Ident)
		return ok && x.Name == "menu" && t.Sel.Name == "FuzzyList"
	}
	return false
}
