package ui

import (
	"os"
	"sync"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The aspect list the fixtures were captured from: `astrolog -HA` of Astrolog
// 8.00, 24 aspects (internal/astrolog/testdata/ha-default.txt).
var (
	fixtureAspectsOnce sync.Once
	fixtureAspectsList aspect.AspectRegistry
	fixtureAspectsErr  error
)

func fixtureAspects() (aspect.AspectRegistry, error) {
	fixtureAspectsOnce.Do(func() {
		b, err := os.ReadFile("../astrolog/testdata/ha-default.txt")
		if err != nil {
			fixtureAspectsErr = err
			return
		}
		rows, err := astrolog.ParseAspectRegistry(string(b))
		fixtureAspectsList, fixtureAspectsErr = aspect.NewAspectRegistry(rows), err
	})
	return fixtureAspectsList, fixtureAspectsErr
}

// testAspects is the fixture's aspect registry; it panics when the fixture
// cannot be read, which no test can go on without.
func testAspects() aspect.AspectRegistry {
	reg, err := fixtureAspects()
	if err != nil {
		panic(err)
	}
	return reg
}

// kindOf is the fixture registry's aspect for three letters.
func kindOf(abbrev string) aspect.AspectKind {
	row, ok := testAspects().ByAbbrev(abbrev)
	if !ok {
		panic("no aspect " + abbrev + " in the fixture registry")
	}
	return row.Kind
}
