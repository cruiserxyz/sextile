package aspect_test

import (
	"os"
	"testing"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// fixtureAspects is `astrolog -HA` of Astrolog 8.00, 24 aspects
// (testdata/ha-default.txt, a copy of internal/astrolog/testdata/ha-default.txt).
func fixtureAspects(t *testing.T) aspect.AspectRegistry {
	t.Helper()
	b, err := os.ReadFile("testdata/ha-default.txt")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := astrolog.ParseAspectRegistry(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return aspect.NewAspectRegistry(rows)
}

// kindOf is the fixture registry's aspect for three letters.
func kindOf(abbrev string) aspect.AspectKind {
	b, err := os.ReadFile("testdata/ha-default.txt")
	if err != nil {
		panic(err)
	}
	rows, err := astrolog.ParseAspectRegistry(string(b))
	if err != nil {
		panic(err)
	}
	row, ok := aspect.NewAspectRegistry(rows).ByAbbrev(abbrev)
	if !ok {
		panic("no aspect " + abbrev + " in the fixture registry")
	}
	return row.Kind
}
