package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
)

// No chooser keeps a default: `, c d` is the one way. The city, houses and
// zodiac choosers draw no box and no radio marks, Tab and Space mark nothing,
// and Enter chooses the row under the cursor without touching preferences.yaml.
// The Bodies chooser keeps its first row, which restores the default bodies.

func seedPrefs(t *testing.T, text string) {
	t.Helper()
	path := store.PreferencesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func currentPrefs(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(store.PreferencesPath())
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(b)
}

// viewLine is the line of the screen that holds text, -1 when none does.
func viewLine(view, text string) int {
	for i, l := range strings.Split(ansi.Strip(view), "\n") {
		if strings.Contains(l, text) {
			return i
		}
	}
	return -1
}

// keepFixture is a workspace of two open charts, a city and a preferences file
// holding a frame, a house display and a clock block.
func keepFixture(t *testing.T) Model {
	t.Helper()
	sessionSandbox(t)
	seedPrefs(t, "frame:\n  houses: Koch\n  zodiac: sidereal Lahiri\nhouse_display: marked\nclock:\n  mode: held\n")
	m := framesFixture(t, 2, 0)
	m.cfg = Config{Accessible: true, Color: ColorNever}
	m.width, m.height = 132, 40
	m.place = amsterdamNL
	return m
}

// openCity opens the city chooser over a small atlas.
func openCity(t *testing.T, m Model) Model {
	t.Helper()
	m.atlas.Index = location.NewIndex([]location.PlaceInfo{amsterdamNL, testPlaceBerlin()})
	m = sendKeys(t, m, ctrlMsg("l"))
	if m.startup != startupAtlasOpen {
		t.Fatalf("Ctrl+L left startup %q", m.startup)
	}
	return m
}

func openZodiac(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = press(t, m, ",", "z")
	if m.startup != startupZodiacOpen {
		t.Fatalf("`, z` left startup %q", m.startup)
	}
	return m
}

func TestBodiesRowRestoresTheDefaultsAndCtrlDDoesNot(t *testing.T) {
	m := openedChooser(t, recipe.ObjectPolicy{Baseline: recipe.Explicit, Selection: []int{1, 43}, Nodes: recipe.NodesTrue})
	before := tickedNums(m.bodyModal)
	m = keyIn(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
	if got := tickedNums(m.bodyModal); len(got) != len(before) {
		t.Errorf("Ctrl+D changed the ticks %v to %v", before, got)
	}
	m = restoreRow(t, m)
	if got := tickedNums(m.bodyModal); len(got) == len(before) {
		t.Errorf("the restore row left the ticks %v", got)
	}
	if m.bodyModal.Cursor != menu.LeadCursor {
		t.Errorf("the cursor moved off the restore row to %d", m.bodyModal.Cursor)
	}
}

func TestChoosersDrawNoKeepBoxAndNoRadioMarksAndTabDoesNothing(t *testing.T) {
	openers := map[string]func(*testing.T, Model) Model{"city": openCity, "houses": openFinder, "zodiac": openZodiac}
	for name, open := range openers {
		t.Run(name, func(t *testing.T) {
			m := open(t, keepFixture(t))
			view := ansi.Strip(m.View())
			for _, banned := range []string{"Keep as default", "[ ]", "[x]", "( )", "(•)", "(*)", "+ Keep"} {
				if strings.Contains(view, banned) {
					t.Errorf("the %s chooser draws %q:\n%s", name, banned, view)
				}
			}
			if name == "city" {
				m = sendKeys(t, m, keyMsg("b"), keyMsg("e"), keyMsg("r"))
			}
			bar := lastRow(m.View())
			if !strings.Contains(bar, "Choose: <enter>") || strings.Contains(bar, "Mark") || strings.Contains(bar, "tab") {
				t.Errorf("the bar is %q, want Choose: <enter> and no Mark or tab", bar)
			}
			before := ansi.Strip(m.View())
			startup := m.startup
			m, cmd := press(t, m, "tab")
			if cmd != nil || m.startup != startup || ansi.Strip(m.View()) != before {
				t.Errorf("Tab changed the %s chooser (cmd %v, startup %q)", name, cmd != nil, m.startup)
			}
		})
	}
}

// Enter on Koch chooses Koch for the chart and writes nothing to preferences.
func TestHousesEnterChoosesTheRowUnderTheCursor(t *testing.T) {
	m := keepFixture(t)
	before := currentPrefs(t)
	if m.ownHouses(m.focusedSlot()).hs == recipe.Koch {
		t.Fatal("fixture: the chart already has Koch")
	}
	m = openFinder(t, m)
	m = sendKeys(t, m, keyMsg("k"), keyMsg("o"), keyMsg("c"), keyMsg("h"))
	if got := m.houseFinder.Matches[m.houseFinder.Cursor]; got != recipe.Koch {
		t.Fatalf("the cursor is on %v, want Koch", got)
	}
	m, cmd := press(t, m, "enter")
	m = deliver(m, cmd)
	if m.startup != startupReady || m.ownHouses(m.focusedSlot()).hs != recipe.Koch {
		t.Errorf("Enter on Koch: startup %q, system %v", m.startup, m.ownHouses(m.focusedSlot()).hs)
	}
	if currentPrefs(t) != before {
		t.Errorf("Enter wrote the preferences:\n%s", currentPrefs(t))
	}
}

// Space does nothing in the houses and zodiac choosers: it marks no row and
// types no space, so the cursor stays where it is.
func TestSpaceDoesNothingInHousesAndZodiac(t *testing.T) {
	for name, open := range map[string]func(*testing.T, Model) Model{"houses": openFinder, "zodiac": openZodiac} {
		m := open(t, keepFixture(t))
		m, _ = press(t, m, "down", "down")
		before := ansi.Strip(m.View())
		m, cmd := press(t, m, " ")
		if cmd != nil || ansi.Strip(m.View()) != before {
			t.Errorf("%s: Space changed the chooser (cmd %v):\n%s", name, cmd != nil, ansi.Strip(m.View()))
		}
	}
}

func TestZodiacEnterChoosesTheRowUnderTheCursor(t *testing.T) {
	m := keepFixture(t)
	before := currentPrefs(t)
	own := m.ownZodiac(m.focusedSlot())
	m = openZodiac(t, m)
	m, _ = press(t, m, "down", "down")
	target := m.zodiacFinder.Matches[m.zodiacFinder.Cursor]
	if target == own {
		t.Fatal("fixture: the cursor is on the chart's own zodiac")
	}
	m, cmd := press(t, m, "enter")
	m = deliver(m, cmd)
	if got := m.ownZodiac(m.focusedSlot()); got != target || m.startup != startupReady {
		t.Errorf("Enter: the chart has %v (startup %q), want %v", got, m.startup, target)
	}
	if currentPrefs(t) != before {
		t.Errorf("Enter wrote the preferences:\n%s", currentPrefs(t))
	}
}

func TestCitySpaceTypesAndEnterAppliesTheCursorCity(t *testing.T) {
	m := openCity(t, keepFixture(t))
	m = sendKeys(t, m, keyMsg("b"), keyMsg("e"), keyMsg("r"), keyMsg(" "))
	if m.atlas.FilterText != "ber " {
		t.Fatalf("Space did not type: filter %q", m.atlas.FilterText)
	}
	m = sendKeys(t, m, keyMsg("backspace"))
	if len(m.atlas.Matches) == 0 {
		t.Fatalf("no match for %q", m.atlas.FilterText)
	}
	want := m.atlas.Matches[m.atlas.Cursor]
	before := currentPrefs(t)
	m, _ = press(t, m, "enter")
	if m.place.Name != want.Name || m.startup == startupAtlasOpen {
		t.Errorf("Enter: city %q startup %q, want %q applied and the chooser closed", m.place.Name, m.startup, want.Name)
	}
	if currentPrefs(t) != before {
		t.Errorf("Enter wrote the preferences:\n%s", currentPrefs(t))
	}
}

// Nothing a chooser does writes preferences.yaml: Enter on a row other than the
// current one in the city, the houses (a system and a form) and the zodiac.
func TestNoChooserWritesPreferences(t *testing.T) {
	m := keepFixture(t)
	prefs := currentPrefs(t)
	check := func(what string, m Model, cmd tea.Cmd) {
		t.Helper()
		_ = deliver(m, cmd)
		if got := currentPrefs(t); got != prefs {
			t.Errorf("%s wrote the preferences:\n%s", what, got)
		}
	}

	c := openCity(t, m)
	c = sendKeys(t, c, keyMsg("b"), keyMsg("e"), keyMsg("r"))
	c, cmd := press(t, c, "enter")
	check("Enter in the city chooser", c, cmd)

	h := openFinder(t, m)
	h, _ = press(t, h, "down", "down")
	h, cmd = press(t, h, "enter")
	check("Enter on a system", h, cmd)

	f := openFinder(t, m)
	f, _ = press(t, f, "end", "down", "down")
	f, cmd = press(t, f, "enter")
	check("Enter on a form", f, cmd)

	z := openZodiac(t, m)
	z, _ = press(t, z, "down", "down")
	z, cmd = press(t, z, "enter")
	check("Enter in the zodiac chooser", z, cmd)
}
