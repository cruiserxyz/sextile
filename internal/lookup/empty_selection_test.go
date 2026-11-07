package lookup

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
)

// A selection with no object left casts no chart: Astrolog prints no body rows
// and the cast is refused, as an empty Explicit selection always was. It is why
// the body chooser does not let an object without its ephemeris file be ticked
// (an all-absent selection would be this chart), and why the argv narrowing may
// leave an object list empty without a second rule.
func TestASelectionWithNoObjectLeftIsRefusedNotShown(t *testing.T) {
	if _, err := exec.LookPath("astrolog"); err != nil {
		t.Skip("astrolog not on PATH; skipping live-binary test")
	}
	l := Hellenistic
	l.Objects = recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{}}
	place := location.PlaceInfo{Name: "Berlin", Lat: 52.52, Lon: 13.405, TZ: "Europe/Berlin"}
	res, err := FirstCallTransits(context.Background(), nil, time.Date(1985, 6, 15, 12, 0, 0, 0, time.UTC), place, l)
	if err == nil || len(res.Positions) != 0 {
		t.Fatalf("an empty selection cast %d positions, err %v; want a refusal", len(res.Positions), err)
	}
}
