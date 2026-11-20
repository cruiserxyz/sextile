package lookup

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

const amsterdamWinter = 3600

func readVoidFixture(t *testing.T) VoidResult {
	t.Helper()
	// The fixture was printed with the 12-hour, month-first clock.
	prev := astrolog.CurrentClock()
	astrolog.SetClock(astrolog.Clock{Hours: astrolog.Hours12, Dates: astrolog.MonthFirst})
	t.Cleanup(func() { astrolog.SetClock(prev) })
	b, err := os.ReadFile("testdata/voids-2026.txt")
	if err != nil {
		t.Fatal(err)
	}
	res, err := astrolog.ReadVoids(string(b), amsterdamWinter, 2026, fixtureRegistry(t), testAspects())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestVoidSearchReadsVoidsAndPhases(t *testing.T) {
	res := readVoidFixture(t)
	plus1 := time.FixedZone("", amsterdamWinter)
	at := func(y int, m time.Month, d, h, min int) time.Time { return time.Date(y, m, d, h, min, 0, 0, plus1) }
	want := []VoidPeriod{
		{Start: at(2026, 10, 31, 22, 59), End: at(2026, 11, 1, 5, 18)},
		{Start: at(2026, 11, 3, 1, 10), End: at(2026, 11, 3, 9, 27)},
	}
	for _, w := range want {
		found := false
		for _, v := range res.Voids {
			found = found || (v.Start.Equal(w.Start) && v.End.Equal(w.End))
		}
		if !found {
			t.Errorf("no void %v to %v among %d voids", w.Start, w.End, len(res.Voids))
		}
	}
	newMoon := false
	for _, p := range res.Phases {
		newMoon = newMoon || (p.Label == "New Moon" && p.Instant.Equal(at(2026, 11, 9, 8, 2)))
	}
	if !newMoon {
		t.Errorf("no New Moon at 2026-11-09 08:02 among %d phases", len(res.Phases))
	}
	if !res.VoidAt(at(2026, 11, 3, 5, 0)) || res.VoidAt(at(2026, 11, 3, 10, 0)) {
		t.Error("VoidAt: want true at 05:00 on 3 Nov and false at 10:00")
	}
	if res.VoidAt(at(2026, 11, 3, 9, 27)) {
		t.Error("VoidAt: the end is exclusive")
	}
}

func TestVoidWithoutEndIsDropped(t *testing.T) {
	prev := astrolog.CurrentClock()
	astrolog.SetClock(astrolog.Clock{Hours: astrolog.Hours12, Dates: astrolog.MonthFirst})
	defer astrolog.SetClock(prev)
	const out = "Sat 10/31/2026 10:59pm    Moon (Can) Squ [Lib] Venus (v/c 6:18)\n"
	res, err := astrolog.ReadVoids(out, amsterdamWinter, 2026, fixtureRegistry(t), testAspects())
	if err != nil {
		t.Fatalf("err = %v, want none", err)
	}
	if len(res.Voids) != 0 {
		t.Errorf("voids = %v, want the unended void dropped", res.Voids)
	}
	if len(res.Rows) != 1 {
		t.Errorf("rows = %d, want the row kept", len(res.Rows))
	}
}

// lookup is stateless: the ui's memory holds void results (spec L6).
func TestLookupHoldsNoVoidState(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range pkgs {
		for name, f := range pkg.Files {
			for _, decl := range f.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.VAR {
					continue
				}
				for _, spec := range gd.Specs {
					vs := spec.(*ast.ValueSpec)
					text := ""
					if vs.Type != nil {
						text = sourceText(fset, name, vs.Type)
					}
					for _, v := range vs.Values {
						text += " " + sourceText(fset, name, v)
					}
					if strings.Contains(text, "Void") {
						t.Errorf("%s: package-level var %v mentions Void: %q", name, vs.Names, text)
					}
				}
			}
		}
	}
}

func sourceText(fset *token.FileSet, file string, n ast.Node) string {
	b, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	return string(b[fset.Position(n.Pos()).Offset:fset.Position(n.End()).Offset])
}

var (
	lookupRegistryOnce sync.Once
	lookupRegistry     aspect.Registry
)

// lookupTestRegistry is the object list of the Astrolog the fixtures were
// captured from, for the tests that build a request without a *testing.T.
func lookupTestRegistry() aspect.Registry {
	lookupRegistryOnce.Do(func() {
		b, err := os.ReadFile("../astrolog/testdata/ho-all.txt")
		if err == nil {
			lookupRegistry, err = astrolog.ParseObjectRegistry(string(b))
		}
		if err != nil {
			panic(err)
		}
	})
	return lookupRegistry
}
