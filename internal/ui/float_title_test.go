package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

// The floating row's title gives way to its place (operator 2026-10-24): in the
// one-chart layout `~ Now` shortens to `~Now`, then `Now`, when the full title
// would push the place off the row and a short one would not, and stays whole
// when it costs nothing. It never becomes `~`, which means the chart was moved.
func TestFloatTitleGivesWayToThePlace(t *testing.T) {
	defaultClock(t)
	place := testPlaceBerlin()
	slots, err := store.NewSlots(testFrozenTime(), place)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Accessible: true}
	s := slots.All()[0]
	used := ansi.StringWidth(slotMoment(s.Time, s.Location)) + 1 + ansi.StringWidth(slotZone(s.Time, s.Location)) +
		1 + ansi.StringWidth(place.Display()) + 1 + len("WhTr")
	prefixW := ansi.StringWidth(chartRowPrefix(false, true, 0, false, cfg))
	sawShort, sawWhole := false, false
	for width := prefixW + used; width <= prefixW+used+12; width++ {
		row := ansi.Strip(chartSlotsBodyLayout(slots, 0, false, testFrozenTime(), place, lookup.Hellenistic, width, cfg, nil, "~ Now", true))
		if !strings.Contains(row, place.Display()) && width >= prefixW+len("Now")+nameGutter+used {
			t.Errorf("width %d: the floating row lost its place %q:\n%q", width, place.Display(), row)
		}
		switch {
		case strings.Contains(row, "~ Now"):
			sawWhole = true
		case strings.Contains(row, "~Now"), strings.Contains(row, "Now  "):
			sawShort = true
		default:
			t.Errorf("width %d: the attached floating row lost its Now: %q", width, row)
		}
	}
	if !sawShort || !sawWhole {
		t.Errorf("the sweep did not see both the short (%v) and the whole (%v) title", sawShort, sawWhole)
	}

	t.Run("an anchored name that already takes the room keeps the whole title", func(t *testing.T) {
		m := rowsFrameModel(t, 132, 40, "Constructedverylongname Withanevenlongersurname")
		if !strings.Contains(ansi.Strip(m.View()), "~ Now") {
			t.Errorf("the title shortened though shortening saves nothing:\n%s", ansi.Strip(m.View()))
		}
	})
}

// The frame: 132x40, one chart (the floating slot alone), colours off; the
// floating row shows its place.
func TestFloatTitleFrame(t *testing.T) {
	defaultClock(t)
	m := readyModelWithSlots(t, berlinP)
	m.cfg = Config{Color: ColorNever}
	m.width, m.height = 132, 40
	v := m.View()
	assertFrameGeometry(t, v, 132, 40)
	if !strings.Contains(ansi.Strip(v), berlinP.Display()) {
		t.Errorf("the one-chart floating row does not show its place %q:\n%s", berlinP.Display(), ansi.Strip(v))
	}
	checkGolden(t, "float-title-one-chart-132x40-never", v)
}
