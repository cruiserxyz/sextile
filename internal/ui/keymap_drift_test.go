// Help/keymap parity invariant.
//
// The UI's `keyMap` struct (internal/ui/keys.go) declares every user-facing
// key.Binding. The ordered binding table in `keyMap.mainBindings()` is the single
// registry used by help rendering. If a binding is added to keyMap but not
// represented in Entries(), matching and help can silently drift. If an entry
// is duplicated or points at a non-existent field, the registry is no longer a
// faithful mirror of keyMap.
//
// This test is the silent-drift guard. It parses the UI source with
// go/parser + go/ast, extracts:
//   - from keys.go: the field names of type keyMap whose type is `key.Binding`
//   - from Entries(): the selector names referenced as `Binding: k.<Field>`
//
// and asserts exact set parity, plus exactly-once coverage in the table.
//
// Since the specification every owner of the keyboard has a
// table of its own; the rule is unchanged for the main screen's, and the test
// also requires every owner's table function to exist in keys.go, so an owner
// cannot quietly lose the table its key bar is drawn from.
package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"testing"
)

const uiKeysPath = "keys.go"

// ownerTables are the ten owners' table functions, in the spec's order: the
// main screen, the seven popups, the Log console and attached Details.
var ownerTables = []string{
	"mainBindings", "cityBindings", "openChartBindings", "chartTitleBindings",
	"saveChartBindings", "bodiesBindings", "aspectsBindings", "helpBindings",
	"consoleBindings", "detailsBindings",
}

// TestUIKeymapHelpParity fails when internal/ui/keys.go's keyMap fields and
// keyMap.mainBindings() diverge. Either direction counts: missing entries,
// dangling selectors, and duplicate table references all fail.
func TestUIKeymapHelpParity(t *testing.T) {
	keyMapFields := extractKeyMapFields(t, uiKeysPath)
	entryBindings := extractEntryBindings(t, uiKeysPath)

	declared := map[string]bool{}
	ast.Inspect(parseFile(t, uiKeysPath), func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			declared[fn.Name.Name] = true
		}
		return true
	})
	for _, table := range ownerTables {
		if !declared[table] {
			t.Errorf("owner table %s() is not declared in %s", table, uiKeysPath)
		}
	}

	only := func(a map[string]bool, b map[string]int) []string {
		var out []string
		for k := range a {
			if b[k] == 0 {
				out = append(out, k)
			}
		}
		sort.Strings(out)
		return out
	}

	onlyEntries := func(a map[string]int, b map[string]bool) []string {
		var out []string
		for k := range a {
			if !b[k] {
				out = append(out, k)
			}
		}
		sort.Strings(out)
		return out
	}

	var duplicateEntries []string
	for k, n := range entryBindings {
		if n > 1 {
			duplicateEntries = append(duplicateEntries, k)
		}
	}
	sort.Strings(duplicateEntries)

	missingInEntries := only(keyMapFields, entryBindings)
	missingInKeyMap := onlyEntries(entryBindings, keyMapFields)

	if len(missingInEntries) == 0 && len(missingInKeyMap) == 0 && len(duplicateEntries) == 0 {
		return
	}

	if len(missingInEntries) > 0 {
		t.Errorf("keyMap fields missing from keyMap.mainBindings(): %v\n  → add matching BindingEntry rows in %s.", missingInEntries, uiKeysPath)
	}
	if len(missingInKeyMap) > 0 {
		t.Errorf("keyMap.mainBindings() references selectors not on keyMap: %v\n  → add matching key.Binding fields to keyMap in %s, or drop the stale BindingEntry rows.", missingInKeyMap, uiKeysPath)
	}
	if len(duplicateEntries) > 0 {
		t.Errorf("keyMap.mainBindings() references keyMap fields more than once: %v\n  → each key.Binding field must appear in exactly one BindingEntry row in %s.", duplicateEntries, uiKeysPath)
	}
}

// extractKeyMapFields parses uiKeysPath and returns the set of field names
// of `type keyMap struct` whose field type is `key.Binding`. Non-binding
// fields (if any future refactor adds them) are ignored.
func extractKeyMapFields(t *testing.T, path string) map[string]bool {
	t.Helper()
	file := parseFile(t, path)
	out := make(map[string]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != "keyMap" {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return false
		}
		for _, f := range st.Fields.List {
			if !isKeyBinding(f.Type) {
				continue
			}
			for _, name := range f.Names {
				if ast.IsExported(name.Name) {
					out[name.Name] = true
				}
			}
		}
		return false
	})
	if len(out) == 0 {
		t.Fatalf("no exported key.Binding fields found on type keyMap in %s", path)
	}
	return out
}

// extractEntryBindings parses keyMap.mainBindings() and returns a count per
// selector name referenced as `Binding: k.<Field>`.
func extractEntryBindings(t *testing.T, path string) map[string]int {
	t.Helper()
	file := parseFile(t, path)
	out := make(map[string]int)
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "mainBindings" || fn.Recv == nil || len(fn.Recv.List) != 1 {
			return true
		}
		recvName, ok := receiverTypeName(fn.Recv.List[0].Type)
		if !ok || recvName != "keyMap" {
			return true
		}
		ast.Inspect(fn.Body, func(nn ast.Node) bool {
			ret, ok := nn.(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				return true
			}
			lit, ok := ret.Results[0].(*ast.CompositeLit)
			if !ok {
				return true
			}
			for _, elt := range lit.Elts {
				entry, ok := elt.(*ast.CompositeLit)
				if !ok {
					continue
				}
				field := extractBindingSelector(entry)
				if field == "" {
					continue
				}
				out[field]++
			}
			return false
		})
		return false
	})
	if len(out) == 0 {
		t.Fatalf("no `Binding: k.<Field>` selectors found in keyMap.mainBindings() in %s", path)
	}
	return out
}

func parseFile(t *testing.T, path string) *ast.File {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Clean(path), nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return file
}

// isKeyBinding reports whether an AST expression is the type `key.Binding`.
func isKeyBinding(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "key" && sel.Sel.Name == "Binding"
}

func receiverTypeName(expr ast.Expr) (string, bool) {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name, true
	case *ast.StarExpr:
		ident, ok := v.X.(*ast.Ident)
		if !ok {
			return "", false
		}
		return ident.Name, true
	default:
		return "", false
	}
}

func extractBindingSelector(entry *ast.CompositeLit) string {
	for _, elt := range entry.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyIdent, ok := kv.Key.(*ast.Ident)
		if !ok || keyIdent.Name != "Binding" {
			continue
		}
		sel, ok := kv.Value.(*ast.SelectorExpr)
		if !ok {
			return ""
		}
		xid, ok := sel.X.(*ast.Ident)
		if !ok || xid.Name != "k" {
			return ""
		}
		return sel.Sel.Name
	}
	return ""
}
