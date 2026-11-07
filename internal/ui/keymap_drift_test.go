// Help/keymap parity invariant.
//
// The UI's `KeyMap` struct (internal/ui/keys/keys.go) declares every user-facing
// key.Binding. The ordered binding table in `mainBindings(keys.KeyMap)` is the single
// registry used by help rendering. If a binding is added to keyMap but not
// represented in Entries(), matching and help can silently drift. If an entry
// is duplicated or points at a non-existent field, the registry is no longer a
// faithful mirror of keyMap.
//
// This test is the silent-drift guard. It parses the UI source with
// go/parser + go/ast, extracts:
//   - from keys/keys.go: the field names of type KeyMap whose type is `key.Binding`
//   - from Entries(): the selector names referenced as `Binding: k.<Field>`
//
// and asserts exact set parity, plus exactly-once coverage in the table.
//
// Since the specification every owner of the keyboard has a
// table of its own; the rule is unchanged for the main screen's, and the test
// also requires every owner's table function to exist in its own file (ownerTables), so an owner
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

// keyMapPath declares the KeyMap type; uiKeysPath holds the main table that reads it.
const (
	keyMapPath = "keys/keys.go"
	uiKeysPath = "keys.go"
)

// ownerTables maps each owner's table function to the file that declares it:
// keys.go keeps the main screen's and attached Details' tables, every other
// owner's sits above its key handler (split item 6.1).
var ownerTables = map[string]string{
	"mainBindings": "keys.go", "detailsBindings": "keys.go",
	"cityBindings": "atlas_modal_model.go", "openChartBindings": "chart_modal.go",
	"chartTitleBindings": "title_form.go", "titleConfirmBindings": "title_form.go",
	"saveChartBindings": "save_modal.go", "bodiesBindings": "body_modal_model.go",
	"aspectsBindings": "aspect_modal_model.go", "housesBindings": "house_finder_model.go",
	"zodiacBindings": "zodiac_finder_model.go", "displayBindings": "house_display_popup_model.go",
	"sessionsBindings": "sessions_popup_model.go", "sessionNameBindings": "sessions_popup_model.go",
	"helpBindings": "model.go", "consoleBindings": "console.go", "actionsBindings": "actions_popup_model.go",
	"timelineSearchBindings": "timeline.go", "reportSearchBindings": "report_search_bindings.go",
}

// ownerTableFiles is every file that holds an owner's table, once each.
func ownerTableFiles() []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range ownerTables {
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}

// missingTables names the tables of tables that their file does not declare.
func missingTables(t *testing.T, tables map[string]string) []string {
	t.Helper()
	declared := map[string]map[string]bool{}
	var out []string
	for table, file := range tables {
		if declared[file] == nil {
			declared[file] = map[string]bool{}
			ast.Inspect(parseFile(t, file), func(n ast.Node) bool {
				if fn, ok := n.(*ast.FuncDecl); ok {
					declared[file][fn.Name.Name] = true
				}
				return true
			})
		}
		if !declared[file][table] {
			out = append(out, table)
		}
	}
	sort.Strings(out)
	return out
}

// TestOwnerTablesSitInTheirFiles is the wrong-file control: pointing one
// table at a file that does not declare it must be reported by that table's
// name, so the map above cannot pass while it names the wrong file.
func TestOwnerTablesSitInTheirFiles(t *testing.T) {
	wrong := map[string]string{}
	for k, v := range ownerTables {
		wrong[k] = v
	}
	wrong["cityBindings"] = "keys.go"
	if got := missingTables(t, wrong); len(got) != 1 || got[0] != "cityBindings" {
		t.Errorf("a table pointed at the wrong file was reported as %v, want [cityBindings]", got)
	}
}

// TestUIKeymapHelpParity fails when internal/ui/keys's KeyMap fields and
// mainBindings(keys.KeyMap) diverge. Either direction counts: missing entries,
// dangling selectors, and duplicate table references all fail.
func TestUIKeymapHelpParity(t *testing.T) {
	keyMapFields := extractKeyMapFields(t, keyMapPath)
	entryBindings := extractEntryBindings(t, uiKeysPath)

	for _, table := range missingTables(t, ownerTables) {
		t.Errorf("owner table %s() is not declared in %s", table, ownerTables[table])
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
		t.Errorf("KeyMap fields missing from mainBindings(keys.KeyMap): %v\n  → add matching BindingEntry rows in %s.", missingInEntries, uiKeysPath)
	}
	if len(missingInKeyMap) > 0 {
		t.Errorf("mainBindings(keys.KeyMap) references selectors not on KeyMap: %v\n  → add matching key.Binding fields to KeyMap in %s, or drop the stale BindingEntry rows.", missingInKeyMap, keyMapPath)
	}
	if len(duplicateEntries) > 0 {
		t.Errorf("mainBindings(keys.KeyMap) references KeyMap fields more than once: %v\n  → each key.Binding field must appear in exactly one BindingEntry row in %s.", duplicateEntries, uiKeysPath)
	}
}

// extractKeyMapFields parses the file and returns the set of field names
// of `type KeyMap struct` whose field type is `key.Binding`. Non-binding
// fields (if any future refactor adds them) are ignored.
func extractKeyMapFields(t *testing.T, path string) map[string]bool {
	t.Helper()
	file := parseFile(t, path)
	out := make(map[string]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != "KeyMap" {
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
		t.Fatalf("no exported key.Binding fields found on type KeyMap in %s", path)
	}
	return out
}

// extractEntryBindings parses mainBindings(keys.KeyMap) and returns a count per
// selector name referenced as `Binding: k.<Field>`.
func extractEntryBindings(t *testing.T, path string) map[string]int {
	t.Helper()
	file := parseFile(t, path)
	out := make(map[string]int)
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "mainBindings" || fn.Recv != nil || len(fn.Type.Params.List) != 1 {
			return true
		}
		if sel, isSel := fn.Type.Params.List[0].Type.(*ast.SelectorExpr); !isSel || sel.Sel.Name != "KeyMap" {
			return true
		}
		ast.Inspect(fn.Body, func(nn ast.Node) bool {
			ret, ok := nn.(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				return true
			}
			res := ret.Results[0]
			// The table is returned through keys.Titled(...): read its argument.
			if call, isCall := res.(*ast.CallExpr); isCall && len(call.Args) == 1 {
				res = call.Args[0]
			}
			lit, ok := res.(*ast.CompositeLit)
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
		t.Fatalf("no `Binding: k.<Field>` selectors found in mainBindings(keys.KeyMap) in %s", path)
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
