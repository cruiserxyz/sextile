package astrolog

import (
	"slices"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// A star the all-objects cast printed no row for is a candidate like any other
// object without a real row.
func TestProbeCandidatesIncludeAStarWithNoRow(t *testing.T) {
	registry := []aspect.RegistryObject{
		{Object: aspect.Object{Number: 1, Name: "Sun"}},
		{Object: aspect.Object{Number: 84, Name: "Sirius"}, Star: true},
	}
	out := "Body  Locat. Ret. Lati. Rul.      House  Rul. Veloc.\n\n" +
		"Sun : 24Gem19   - 0:00' (-) [ 9th house] [-] +0.969\n"
	if got, want := probeCandidates(registry, out), []int{84}; !slices.Equal(got, want) {
		t.Errorf("candidates = %v, want %v", got, want)
	}
}
