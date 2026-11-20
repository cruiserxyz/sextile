package astrolog

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The aspect list the fixtures were captured from: `astrolog -HA` of Astrolog
// 8.00, 24 aspects (testdata/ha-default.txt).
var (
	testAspectsOnce sync.Once
	testAspectsList aspect.AspectRegistry
	testAspectsErr  error
)

func loadTestAspects() (aspect.AspectRegistry, error) {
	testAspectsOnce.Do(func() {
		b, err := os.ReadFile(filepath.Join(testPackageDir, "testdata", "ha-default.txt"))
		if err != nil {
			testAspectsErr = err
			return
		}
		rows, err := ParseAspectRegistry(string(b))
		testAspectsList, testAspectsErr = aspect.NewAspectRegistry(rows), err
	})
	return testAspectsList, testAspectsErr
}

// testAspects is the fixture's aspect registry.
func testAspects(t *testing.T) aspect.AspectRegistry {
	t.Helper()
	reg, err := loadTestAspects()
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

// kindOf is the fixture registry's aspect for three letters.
func kindOf(abbrev string) aspect.AspectKind {
	reg, err := loadTestAspects()
	if err != nil {
		panic(err)
	}
	row, ok := reg.ByAbbrev(abbrev)
	if !ok {
		panic("no aspect " + abbrev + " in the fixture registry")
	}
	return row.Kind
}

// mustAspects is parseAspects for a list that must be read.
func mustAspects(t *testing.T, out string, reg aspect.Registry) []aspect.Aspect {
	t.Helper()
	got, err := parseAspects(out, reg, testAspects(t))
	if err != nil {
		t.Fatalf("parseAspects: %v", err)
	}
	return got
}

// mustCross is parseCrossAspects for a list that must be read.
func mustCross(t *testing.T, out string, reg aspect.Registry) []aspect.CrossAspect {
	t.Helper()
	got, err := parseCrossAspects(out, reg, testAspects(t))
	if err != nil {
		t.Fatalf("parseCrossAspects: %v", err)
	}
	return got
}
