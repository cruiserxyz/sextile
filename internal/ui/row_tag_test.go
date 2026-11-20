package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
)

// A chart row's tag names the house system and zodiac Astrolog cast for the
// chart's file, with the operator's short marks where the approved codes do
// not reach (operator, 2026-11-15): `#` and Astrolog's number for a system
// outside the 22, `3` before the code for 3D houses, and the signed offset for
// an unnamed sidereal zodiac. A `*` follows the system when Astrolog cast
// another than the file names, and the Log says so once.
func TestFileChartRowTagIsWhatAstrologCast(t *testing.T) {
	for file, want := range map[string]string{
		"Kepler.as":         "WhTr",
		"House12.as":        "PRTr",
		"House3D.as":        "3PlTr",
		"House23.as":        "#23Tr",
		"HouseSidereal.as":  "Ko+0.88", // =s 0.88, not Lahiri's 0.883208
		"Sidereal.as":       "Wh+0.88",
		"Sidereal25.as":     "Wh+2.5",
		"ArcticPlacidus.as": "Po*Tr", // asks for Placidus, cast in Porphyry at 69N
		"HouseAbbrev.as":    "PlTr",
	} {
		t.Run(file, func(t *testing.T) {
			m := openMoment(t, file, 200, 60)
			var got string
			for _, s := range m.slots.All() {
				if !s.Floating {
					got = m.rowTag(s)
				}
			}
			if got != want {
				t.Errorf("tag %q, want %q", got, want)
			}
			if n := strings.Count(ansi.Strip(m.View()), "asked for Placidus, Astrolog casts Porphyry here"); file == "ArcticPlacidus.as" && n != 1 {
				t.Errorf("the Log says what was asked and what was cast %d times, want once:\n%s", n, ansi.Strip(m.View()))
			}
		})
	}
}

// A chosen system stays the operator's choice, and the row still names what
// Astrolog cast: Placidus chosen for a chart at 69N casts Porphyry, so the row
// shows `Po*` and the Log says once what was asked and what was cast.
func TestChosenPolarHousesShowWhatWasCast(t *testing.T) {
	m := openMoment(t, "Arctic-Tromso.as", 200, 60)
	idx := -1
	for i, s := range m.slots.All() {
		if !s.Floating {
			idx = i
		}
	}
	m, cmd := m.chooseHouseSystem(idx, recipe.Placidus)
	m = pumpPatient(t, m, cmd)
	if got := m.rowTag(m.slots.All()[idx]); got != "Po*Tr" {
		t.Errorf("tag %q, want Po*Tr", got)
	}
	if !strings.Contains(ansi.Strip(m.View()), "asked for Placidus, Astrolog casts Porphyry here") {
		t.Errorf("the Log does not say what was asked and what was cast:\n%s", ansi.Strip(m.View()))
	}
}

// Every row shows its clock to the second, with the clock's offset.
func TestRowShowsTheClockToTheSecond(t *testing.T) {
	for file, want := range map[string]string{"Seconds.as": "16:30:45 +2 DT", "Kepler.as": "14:37:00 LMT"} {
		m := openMoment(t, file, 200, 60)
		for _, s := range m.slots.All() {
			if !s.Floating {
				if c := slotCells(s, nil); !strings.Contains(c.clock+" "+c.zone, want) {
					t.Errorf("%s: the row reads %q, want %q in it", file, c.clock+" "+c.zone, want)
				}
			}
		}
	}
}

// A file's named zodiac is the chart's starting setting: the chooser opens on
// it and the tag names it.
func TestFileZodiacIsTheChartsStartingZodiac(t *testing.T) {
	m := openMoment(t, "Lahiri.as", 200, 60)
	for _, s := range m.slots.All() {
		if s.Floating {
			continue
		}
		if z := m.ownZodiac(s); z != recipe.Lahiri {
			t.Errorf("the chart's zodiac is %s, want the file's Lahiri", z.Name())
		}
		if got := m.rowTag(s); got != "WhLa" {
			t.Errorf("tag %q, want WhLa", got)
		}
	}
}

// A chart whose file sets an offset no named ayanamsa has opens the zodiac
// chooser with that offset as its first row, marked current (operator,
// 2026-11-15: "Extra first row"); a named one gets no extra row.
func TestZodiacChooserShowsTheFilesOwnOffset(t *testing.T) {
	for file, want := range map[string]bool{"Sidereal25.as": true, "Lahiri.as": false} {
		m := openMoment(t, file, 132, 40)
		for i, s := range m.slots.All() {
			if s.Floating {
				continue
			}
			m.cursorIdx = i
			st := m.zodiacFinderFor(s)
			rows := st.Kind.List("")
			first := st.Kind.Row(rows[0], rows[0] == st.Current, false, nil)
			has := strings.Contains(first.Left, "Sidereal +2.5, from its file") && st.Current == popup.FileZodiac
			if has != want {
				t.Errorf("%s: first row %q (current %v), want the file's offset row: %v", file, first.Left, st.Current, want)
			}
		}
	}
}
