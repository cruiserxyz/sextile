package astrolog

import (
	"slices"
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// The per-object cast is asked about every object the all-objects cast was
// asked for and did not return at a real position: a no-data row (0 Aries) and
// an object with no row at all alike.
func TestProbeCandidatesAreEveryObjectWithoutARealRow(t *testing.T) {
	registry := []aspect.RegistryObject{
		{Object: aspect.Object{Number: 0, Name: "Earth"}},
		{Object: aspect.Object{Number: 1, Name: "Sun"}},
		{Object: aspect.Object{Number: 2, Name: "Moon"}},
		{Object: aspect.Object{Number: 12, Name: "Ceres"}},
		{Object: aspect.Object{Number: 84, Name: "Sirius"}},
	}
	// Sun has a real row, Ceres a no-data row (0Ari, no motion), Moon and
	// Sirius no row at all.
	out := "Body  Locat. Ret. Lati. Rul.      House  Rul. Veloc.\n\n" +
		"Sun : 24Gem19   - 0:00' (-) [ 9th house] [-] +0.969\n" +
		"Cere:  0Ari00   + 0:00' (-) [ 1st house] [-] +0.000\n"
	got := probeCandidates(registry, out)
	if want := []int{2, 12, 84}; !slices.Equal(got, want) {
		t.Errorf("candidates = %v, want %v", got, want)
	}
}

// A label several registry names share claims none of them: two real
// rows under it still leave both objects to be probed one by one.
func TestProbeCandidatesSharedLabelClaimsNothing(t *testing.T) {
	registry := []aspect.RegistryObject{
		{Object: aspect.Object{Number: 1, Name: "Sun"}},
		{Object: aspect.Object{Number: 58, Name: "Titan"}},
		{Object: aspect.Object{Number: 66, Name: "Titania"}},
	}
	out := "Body  Locat. Ret. Lati. Rul.      House  Rul. Veloc.\n\n" +
		"Sun : 24Gem19   - 0:00' (-) [ 9th house] [-] +0.969\n" +
		"Tita:  5Leo10   + 0:10' (-) [ 1st house] [-] +0.100\n" +
		"Tita: 12Vir40   - 0:20' (-) [ 3rd house] [-] +0.050\n"
	if got, want := probeCandidates(registry, out), []int{58, 66}; !slices.Equal(got, want) {
		t.Errorf("candidates = %v, want %v", got, want)
	}
}
