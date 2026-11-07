package panel

import (
	"testing"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Titan and Titania print one four-character label; their Astrolog numbers
// tell them apart.
func TestSamePositionTellsTitanFromTitania(t *testing.T) {
	titan := aspect.Position{Object: aspect.Object{Number: 58, Name: "Titan"}}
	titania := aspect.Position{Object: aspect.Object{Number: 66, Name: "Titania"}}
	if samePosition(titan, titania) {
		t.Error("Titan and Titania are the same position")
	}
	if !samePosition(titan, aspect.Position{Object: aspect.Object{Number: 58, Name: "Titan"}, Longitude: 5}) {
		t.Error("Titan in two charts is not the same position")
	}
	sun := aspect.Position{Object: aspect.Object{Number: 1, Name: "Sun"}}
	if samePosition(sun, titan) || !samePosition(sun, sun) {
		t.Error("the Sun is not its own identity")
	}
}
