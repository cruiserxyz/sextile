package ui

import (
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// Two chart files may hold the same moment and place and differ in their own
// settings (one asks for Whole (MC) houses, the other for 3D Placidus). Each
// chart's row names the system its own cast used, never the other's.
func TestTwoChartsAtOneMomentKeepTheirOwnCast(t *testing.T) {
	m := timelineModel(t, Config{Accessible: true})
	when := time.Date(1990, 6, 1, 14, 30, 0, 0, time.UTC)
	place := location.PlaceInfo{Name: "Amsterdam", Lat: 52.37, Lon: 4.88, TZ: "Europe/Amsterdam"}
	whole := store.SessionChart{ID: "whole", Name: "Whole (MC) test", Record: "-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n-c 23\n"}
	threeD := store.SessionChart{ID: "threed", Name: "3D Placidus", Record: "-qb Jun 1 1990 16:30 DT 1E 4:53E 52:22N\n-c 0\n-c3 1\n"}
	for _, c := range []store.SessionChart{whole, threeD} {
		if err := m.slots.AddAnchored(c, when, place); err != nil {
			t.Fatal(err)
		}
	}
	var slots []*store.Slot
	for _, s := range m.slots.All() {
		if s.Chart.ID == whole.ID || s.Chart.ID == threeD.ID {
			slots = append(slots, s)
		}
	}
	if len(slots) != 2 {
		t.Fatalf("%d of the added charts found, want 2", len(slots))
	}
	m = m.commitCasts([]castFact{
		{key: anchoredKey(anchorOf(slots[0])), asked: recipe.WholeSign, used: "Whole (MC)"},
		{key: anchoredKey(anchorOf(slots[1])), asked: recipe.WholeSign, used: "3D Placidus"},
	})
	for i, want := range []string{"Whole (MC)", "3D Placidus"} {
		if got, _ := m.castHouses(slots[i], recipe.WholeSign); got != want {
			t.Errorf("%s: cast %q, want its own %q", slots[i].Chart.Name, got, want)
		}
	}
}
