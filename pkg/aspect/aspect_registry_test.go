package aspect_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The aspect registry is the one source of an aspect's identity: it finds a row
// by Astrolog's index and by its three letters, keeps Astrolog's order, and is
// not ready until it has been read.
func TestAspectRegistryFindsRowsByIndexAndLetters(t *testing.T) {
	reg := fixtureAspects(t)
	if !reg.Ready() || len(reg.Rows()) != 24 {
		t.Fatalf("fixture registry ready %v with %d rows, want 24", reg.Ready(), len(reg.Rows()))
	}
	for i, row := range reg.Rows() {
		if row.Kind.Index != i+1 {
			t.Fatalf("row %d has index %d: the registry must keep Astrolog's order", i, row.Kind.Index)
		}
		if byIndex, ok := reg.ByIndex(row.Kind.Index); !ok || byIndex != row {
			t.Errorf("ByIndex(%d) = %+v, %v", row.Kind.Index, byIndex, ok)
		}
		if byLetters, ok := reg.ByAbbrev(row.Kind.Abbrev); !ok || byLetters != row {
			t.Errorf("ByAbbrev(%q) = %+v, %v", row.Kind.Abbrev, byLetters, ok)
		}
		if !row.Kind.Known() || row.Kind.String() != row.Kind.Name {
			t.Errorf("kind %+v: Known %v, String %q", row.Kind, row.Kind.Known(), row.Kind.String())
		}
	}
	sep, ok := reg.ByAbbrev("Sep")
	if !ok || sep.Kind.Index != 13 || sep.Angle != 51.43 {
		t.Errorf("Sep = %+v, %v; want index 13 at 51.43", sep, ok)
	}
}

func TestAspectRegistryRefusesWhatItDoesNotHold(t *testing.T) {
	reg := fixtureAspects(t)
	if _, ok := reg.ByAbbrev("Xyz"); ok {
		t.Error("ByAbbrev found letters the registry does not hold")
	}
	if _, ok := reg.ByIndex(99); ok {
		t.Error("ByIndex found an index the registry does not hold")
	}
	var zero aspect.AspectRegistry
	if zero.Ready() {
		t.Error("the zero registry is ready")
	}
	if _, ok := zero.ByAbbrev("Con"); ok {
		t.Error("the zero registry holds an aspect")
	}
	var none aspect.AspectKind
	if none.Known() {
		t.Error("the zero kind is an aspect")
	}
	// Letters two rows share name no aspect: never a guess.
	dup := aspect.NewAspectRegistry([]aspect.RegistryAspect{
		{Kind: aspect.AspectKind{Index: 1, Name: "A", Abbrev: "Aaa"}},
		{Kind: aspect.AspectKind{Index: 2, Name: "B", Abbrev: "Aaa"}},
	})
	if _, ok := dup.ByAbbrev("Aaa"); ok {
		t.Error("ByAbbrev resolved letters that two rows share")
	}
}

// The registry fixture is kept in two places, as a package may read only its
// own testdata. They must be one file: one byte of difference and the two
// suites would test two different registries.
func TestTheTwoAspectRegistryFixturesAreOneFile(t *testing.T) {
	here, err := os.ReadFile("testdata/ha-default.txt")
	if err != nil {
		t.Fatal(err)
	}
	canon, err := os.ReadFile("../../internal/astrolog/testdata/ha-default.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(here) == 0 || !bytes.Equal(here, canon) {
		t.Errorf("pkg/aspect/testdata/ha-default.txt (%d bytes) differs from internal/astrolog/testdata/ha-default.txt (%d bytes)", len(here), len(canon))
	}
}
