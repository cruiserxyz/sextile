package aspect_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestAspectPackageHoldsNoAspectList: the registry of the Astrolog in use is
// the only list of aspects. The package declares no constant of type
// AspectKind, no variable that is an AspectKind literal, and no list or map of
// aspects, and no .go file of the module names the old abbreviation table.
func TestAspectPackageHoldsNoAspectList(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	isKind := func(e ast.Expr) bool {
		id, ok := e.(*ast.Ident)
		return ok && (id.Name == "AspectKind" || id.Name == "RegistryAspect")
	}
	listOf := func(e ast.Expr) bool {
		switch x := e.(type) {
		case *ast.ArrayType:
			return isKind(x.Elt)
		case *ast.MapType:
			return isKind(x.Key) || isKind(x.Value)
		}
		return false
	}
	checked := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.GenDecl:
				if x.Tok != token.CONST {
					return true
				}
				for _, sp := range x.Specs {
					if vs, ok := sp.(*ast.ValueSpec); ok && vs.Type != nil && isKind(vs.Type) {
						t.Errorf("%s: a constant of type %v names an aspect", fset.Position(vs.Pos()), vs.Names)
					}
				}
			case *ast.CompositeLit:
				if x.Type != nil && len(x.Elts) > 0 && (listOf(x.Type) || isKind(x.Type)) {
					t.Errorf("%s: a literal of an aspect or of a list of aspects", fset.Position(x.Pos()))
				}
			}
			return true
		})
	}
	if checked < 5 {
		t.Fatalf("read %d source files of the package; the check looked at nothing", checked)
	}
}

// TestNoFileNamesTheOldAbbreviationTable: the identifier aspectAbbrev, the
// table the registry replaced, occurs in no .go file of the module.
func TestNoFileNamesTheOldAbbreviationTable(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		up := filepath.Dir(root)
		if up == root {
			t.Fatal("no go.mod above the package")
		}
		root = up
	}
	fset := token.NewFileSet()
	seen := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); path != root && (n == "testdata" || n == "tmp" || strings.HasPrefix(n, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}
		seen++
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == "aspectAbbrev" {
				t.Errorf("%s: names aspectAbbrev", fset.Position(id.Pos()))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen < 100 {
		t.Fatalf("read %d .go files of the module; the walk looked at too little", seen)
	}
}

// aspectLetters are the three letters of every aspect of the fixture registry.
func aspectLetters(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, r := range fixtureAspects(t).Rows() {
		out[r.Kind.Abbrev] = true
	}
	if len(out) != 24 {
		t.Fatalf("the fixture registry has %d letters, want 24", len(out))
	}
	return out
}

// TestNoFileHoldsAnAspectsLetters: no non-test .go file of the module holds a
// string literal equal to one of Astrolog's aspect letters, except the one
// glyph table of the panel, which is keyed by them. A renamed table of the
// eleven (or of all twenty-four) is the same list under another name.
func TestNoFileHoldsAnAspectsLetters(t *testing.T) {
	root := moduleRoot(t)
	letters := aspectLetters(t)
	glyphTable := filepath.Join(root, "internal", "ui", "panel", "chart_view.go")
	fset := token.NewFileSet()
	seen := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); path != root && (n == "testdata" || n == "tmp" || strings.HasPrefix(n, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || path == glyphTable {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}
		seen++
		ast.Inspect(f, func(n ast.Node) bool {
			if bl, ok := n.(*ast.BasicLit); ok && bl.Kind == token.STRING {
				if v, err := strconv.Unquote(bl.Value); err == nil && letters[v] {
					t.Errorf("%s: the string %s is an aspect's letters", fset.Position(bl.Pos()), bl.Value)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen < 100 {
		t.Fatalf("read %d .go files of the module; the walk looked at too little", seen)
	}
}

// moduleRoot is the folder holding go.mod above the package.
func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		up := filepath.Dir(root)
		if up == root {
			t.Fatal("no go.mod above the package")
		}
		root = up
	}
}

// aspectNames are the names an aspect list could be declared with: the old
// eleven kinds and the names of Astrolog's own twenty-four, lower case.
func aspectNames(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, n := range []string{"Conjunction", "Opposition", "Square", "Trine", "Sextile", "Quincunx",
		"Semisextile", "Semisquare", "Sesquisquare", "Quintile", "Biquintile"} {
		out[strings.ToLower(n)] = true
	}
	for _, r := range fixtureAspects(t).Rows() {
		out[strings.ToLower(r.Kind.Name)] = true
	}
	return out
}

// TestNoDeclarationBlockListsAspectNames: no const or var block of a non-test
// file of the module holds three or more identifiers that are an aspect's name
// (case aside). An enumeration of the old kinds, typed or not, under whatever
// type, is such a block; an unrelated enumeration is not.
func TestNoDeclarationBlockListsAspectNames(t *testing.T) {
	root := moduleRoot(t)
	names := aspectNames(t)
	fset := token.NewFileSet()
	seen := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); path != root && (n == "testdata" || n == "tmp" || strings.HasPrefix(n, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}
		seen++
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || (gd.Tok != token.CONST && gd.Tok != token.VAR) {
				continue
			}
			var hit []string
			for _, sp := range gd.Specs {
				if vs, ok := sp.(*ast.ValueSpec); ok {
					for _, id := range vs.Names {
						if names[strings.ToLower(id.Name)] {
							hit = append(hit, id.Name)
						}
					}
				}
			}
			if len(hit) >= 3 {
				t.Errorf("%s: a block declares %v, a list of aspects", fset.Position(gd.Pos()), hit)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen < 100 {
		t.Fatalf("read %d .go files of the module; the walk looked at too little", seen)
	}
}
