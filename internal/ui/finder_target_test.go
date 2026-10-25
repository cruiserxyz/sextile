package ui

// A finder's choice changes the focused chart's own system and no other
// chart's, selected or not (operator, 2026-10-24: "only the focused chart
// should have it"). Chart values are constructed synthetic test inputs.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// chooseInFinder opens the finder with open, types q, presses Enter and lets the
// recast finish.
func chooseInFinder(t *testing.T, m Model, ctrl tea.KeyMsg, q string) Model {
	t.Helper()
	m, _ = tpress(t, m, ctrl)
	m = typeInFinder(t, m, q)
	var cmd tea.Cmd
	m, cmd = tpress(t, m, keyEnter)
	return pump(t, m, cmd)
}

func TestHouseChoiceChangesTheHousesOnScreen(t *testing.T) {
	setup := func(t *testing.T, cursor int) Model {
		sessionSandbox(t)
		stubHouseLookups(t)
		m := framesFixture(t, 2, cursor) // Anna ①, Ben ②, now ③: all selected
		m.cfg = Config{Accessible: true, Color: ColorNever}
		m = withOwnHouses(m, recipe.WholeSign)
		return settleHouses(t, m)
	}
	shown := func(t *testing.T, m Model, tags ...string) {
		t.Helper()
		screen := ansi.Strip(m.View())
		for _, s := range tags {
			if !strings.Contains(screen, s) {
				t.Errorf("the screen should carry %s:\n%s", s, screen)
			}
		}
	}
	t.Run("on the second chart", func(t *testing.T) {
		m := setup(t, 1)
		m = chooseInFinder(t, m, keyCtrlHMain, "placidus")
		if got := m.ownHouses(slotAt(m, 1)).hs; got != recipe.Placidus {
			t.Errorf("② is shown in %v, want Placidus", got)
		}
		if got := m.lensFor(slotAt(m, 1)).Houses; got != recipe.Placidus {
			t.Errorf("② is cast in %v, want Placidus", got)
		}
	})
	t.Run("on now's own tab now alone changes", func(t *testing.T) {
		m := setup(t, 2)
		m = chooseInFinder(t, m, keyCtrlHMain, "placidus")
		if got := m.lensFor(slotAt(m, 2)).Houses; got != recipe.Placidus {
			t.Errorf("now is cast in %v, want Placidus", got)
		}
		if got := m.rowTag(slotAt(m, 2)); got != "PlTr" {
			t.Errorf("now's row tag = %q, want PlTr", got)
		}
		if got := m.ownHouses(slotAt(m, 0)).hs; got != recipe.WholeSign {
			t.Errorf("①'s own system = %v, want its Whole Sign untouched", got)
		}
		if got := m.rowTag(slotAt(m, 0)); got != "WhTr" {
			t.Errorf("①'s row tag = %q, want WhTr", got)
		}
		shown(t, m, "PlTr", "WhTr")
	})
	t.Run("on now visiting a chart now alone changes", func(t *testing.T) {
		m := setup(t, 2)
		m = choosePair(m, scenePair{left: slotAt(m, 1), right: slotAt(m, 2)})
		m = chooseInFinder(t, m, keyCtrlHMain, "placidus")
		if got := m.ownHouses(slotAt(m, 2)).hs; got != recipe.Placidus {
			t.Errorf("now is shown in %v, want Placidus", got)
		}
		if got := m.ownHouses(slotAt(m, 1)).hs; got == recipe.Placidus {
			t.Errorf("② (the frame) was changed to %v", got)
		}
		if got := m.rowTag(slotAt(m, 2)); got != "PlTr" {
			t.Errorf("now's row tag = %q, want PlTr", got)
		}
	})
}

func TestZodiacChoiceChangesTheZodiacOnScreen(t *testing.T) {
	setup := func(t *testing.T, cursor int) Model {
		sessionSandbox(t)
		stubFrameLookups(t)
		m := framesFixture(t, 2, cursor)
		m.cfg = Config{Accessible: true, Color: ColorNever}
		return settleHouses(t, m)
	}
	t.Run("on the second chart", func(t *testing.T) {
		m := setup(t, 1)
		m = chooseInFinder(t, m, keyCtrlZ, "lahiri")
		if got := m.ownZodiac(slotAt(m, 1)); got != recipe.Lahiri {
			t.Errorf("② is shown in %v, want Lahiri", got)
		}
		if got := m.lensFor(slotAt(m, 1)).Zodiac; got != recipe.Lahiri {
			t.Errorf("② is cast in %v, want Lahiri", got)
		}
		if got := m.rowTag(slotAt(m, 1)); !strings.HasSuffix(got, "La") {
			t.Errorf("②'s row tag = %q, want it to end in La", got)
		}
		if got := m.ownZodiac(slotAt(m, 0)); got == recipe.Lahiri {
			t.Errorf("① was changed to %v", got)
		}
	})
	t.Run("on now", func(t *testing.T) {
		m := setup(t, 2)
		m = chooseInFinder(t, m, keyCtrlZ, "lahiri")
		if got := m.ownZodiac(slotAt(m, 2)); got != recipe.Lahiri {
			t.Errorf("now is shown in %v, want Lahiri", got)
		}
	})
}

func TestUnselectedChartFinderEditsItself(t *testing.T) {
	t.Run("houses", func(t *testing.T) {
		m := finderModel(t, 2, 1)
		m = chooseInFinder(t, m, keyCtrlHMain, "placidus")
		if got := m.ownHouses(slotAt(m, 1)).hs; got != recipe.Placidus {
			t.Errorf("② own system = %v, want Placidus", got)
		}
		if got := m.ownHouses(slotAt(m, 1)).hs; got != recipe.Placidus {
			t.Errorf("② is shown in %v, want Placidus", got)
		}
		if got := m.ownHouses(slotAt(m, 0)).hs; got == recipe.Placidus {
			t.Errorf("① was changed to %v", got)
		}
	})
	t.Run("zodiac", func(t *testing.T) {
		stubFrameLookups(t)
		m := finderModel(t, 2, 1)
		m = chooseInFinder(t, m, keyCtrlZ, "lahiri")
		if got := m.ownZodiac(slotAt(m, 1)); got != recipe.Lahiri {
			t.Errorf("② own zodiac = %v, want Lahiri", got)
		}
		if got := m.ownZodiac(slotAt(m, 0)); got == recipe.Lahiri {
			t.Errorf("① was changed to %v", got)
		}
	})
}
