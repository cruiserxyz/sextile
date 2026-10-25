package ui

// the specification V2: the Charts row tag - the 22 codes and the
// substitution mark - frozen as the specification F4 shows it, with
// every zodiac code Tr (the zodiac chooser is the next slice).

import (
	"fmt"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// f4Model is the four-slot list of F4: ① Constructed One (Placidus) and now
// are the group, Tromsø Client is in it too, set to Placidus of its own, and
// sits at 69.6 N where Astrolog cast its Placidus as Porphyry, and Maria is
// not selected and is set to Koch. Now keeps its own Whole Sign.
func f4Model(t *testing.T, width, height int) Model {
	t.Helper()
	amsterdam := location.PlaceInfo{Name: "Amsterdam", Country: "NL", Lat: 52.37, Lon: 4.9, TZ: "Europe/Amsterdam"}
	bremen := location.PlaceInfo{Name: "Bremen", Country: "DE", Lat: 53.07, Lon: 8.8, TZ: "Europe/Berlin"}
	tromso := location.PlaceInfo{Name: "Tromsø", Country: "NO", Lat: 69.65, Lon: 18.96, TZ: "Europe/Oslo"}
	graz := location.PlaceInfo{Name: "Graz", Country: "AT", Lat: 47.07, Lon: 15.44, TZ: "Europe/Vienna"}
	m := NewModelWithClock(Config{Color: ColorNever}, FrozenClock(time.Date(2026, 10, 8, 11, 5, 0, 0, time.UTC)))
	m.startup = startupReady
	m.place = amsterdam
	slots, err := store.NewSlots(time.Date(2026, 10, 8, 11, 5, 0, 0, time.UTC), amsterdam)
	if err != nil {
		t.Fatal(err)
	}
	add := func(id, name string, when time.Time, place location.PlaceInfo) {
		chart := store.SessionChart{ID: id, Name: name, Source: store.ChartSource{Kind: store.ChartKindFile, File: id + ".as"}}
		if err := slots.AddAnchored(chart, when, place); err != nil {
			t.Fatal(err)
		}
	}
	add("constructed-one", "Constructed One", time.Date(1995, 3, 20, 10, 0, 0, 0, time.UTC), bremen)
	add("tromso-client", "Tromsø Client", time.Date(1990, 6, 1, 12, 0, 0, 0, time.UTC), tromso)
	add("maria", "Maria", time.Date(1984, 7, 7, 7, 15, 0, 0, time.UTC), graz)
	slots.SetSlotSelected(0, true)
	slots.SetSlotSelected(1, true)
	slots.SetSlotSelected(2, false)
	slots.SetSlotSelected(3, true)
	m.slots = slots
	m.targetFloatingTime = slots.Floating().Time
	m.cursorIdx = 0
	m.focused = PanelChart
	m.width, m.height = width, height
	all := slots.All()
	m.frames.records = []store.FrameRecord{
		{File: "constructed-one.as", When: all[0].Time, Houses: recipe.Placidus, HasHouses: true, From: store.FrameFromChosen},
		{File: "tromso-client.as", When: all[1].Time, Houses: recipe.Placidus, HasHouses: true, From: store.FrameFromChosen},
		{File: "maria.as", When: all[2].Time, Houses: recipe.Koch, HasHouses: true, From: store.FrameFromChosen},
	}
	m = m.commitCasts([]castFact{{key: chartKey{when: all[1].Time, place: all[1].Location}, asked: recipe.Placidus, used: "Porphyry"}})
	return m.noteArrival()
}

func TestHouseTagFrames(t *testing.T) {
	for _, size := range [][2]int{{132, 40}, {80, 24}} {
		name := fmt.Sprintf("house_tag_%dx%d", size[0], size[1])
		t.Run(name, func(t *testing.T) {
			m := f4Model(t, size[0], size[1])
			checkGolden(t, name, ansi.Strip(m.View()))
		})
	}
}
