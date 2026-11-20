package ui

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// uiForbiddenImports are the packages internal/ui's shipped code may not
// import: Astrolog's runner and parsers, and the raw payload they produce. The
// ui reaches both only through lookup. Test files may import them, to build
// fixtures.
var uiForbiddenImports = []string{
	"github.com/cruiserxyz/sextile/internal/astrolog",
	"github.com/cruiserxyz/sextile/internal/payload",
}

func TestUIDoesNotImportAstrolog(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		checked++
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			for _, banned := range uiForbiddenImports {
				if path == banned || strings.HasPrefix(path, banned+"/") {
					t.Errorf("%s imports %s; internal/ui reaches it only through lookup", name, path)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no source file of internal/ui was checked")
	}
}
