package ui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
)

// The houses chooser's Shown as part: under the systems, a dotted rule, the
// heading and the four forms, the form in force marked ●. The cursor reaches it
// with Down past the last system and leaves it with Up from the first form;
// Enter on a form puts it in force and closes, leaving the chart's system; Esc
// applies nothing. Space and typing do nothing there.

// shownMarks lists, one per form in form order, the mark each form's row shows
// on screen: "●" for the form in force, " " for the others.
func shownMarks(t *testing.T, m Model) []string {
	t.Helper()
	var out []string
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
		for _, info := range houseForms {
			if strings.Contains(l, info.title) {
				if strings.Contains(l, "●") {
					out = append(out, "●")
				} else {
					out = append(out, " ")
				}
			}
		}
	}
	if len(out) != len(houseForms) {
		t.Fatalf("found %d form rows on screen, want %d:\n%s", len(out), len(houseForms), ansi.Strip(m.View()))
	}
	return out
}

func inForce(t *testing.T, m Model) panel.HouseForm {
	t.Helper()
	return m.houseDisplay
}

func TestShownAsPartIsReachedWithDownPastTheLastSystem(t *testing.T) {
	m := openFinder(t, finderModel(t, 2, 0))
	view := ansi.Strip(m.View())
	for _, want := range []string{"Houses shown as", houseForms[0].title, houseForms[3].title} {
		if !strings.Contains(view, want) {
			t.Errorf("the houses chooser does not show %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "( )") || strings.Contains(view, "(•)") || strings.Contains(view, "(*)") {
		t.Errorf("the houses chooser draws radio marks:\n%s", view)
	}
	if marks := shownMarks(t, m); marks[0] != "●" || marks[1] != " " {
		t.Errorf("marks on open = %q, want only the form in force marked", marks)
	}
	if m.houseFinder.Shown.Focus {
		t.Fatal("the chooser opened with the cursor in the Shown as part")
	}
	m, _ = press(t, m, "end", "down")
	if !m.houseFinder.Shown.Focus || m.currentOwner() != ownerHouses {
		t.Fatalf("Down past the last system left focus=%v owner=%v, want the Shown as part with the chooser open", m.houseFinder.Shown.Focus, m.currentOwner())
	}
	m, _ = press(t, m, "up")
	if m.houseFinder.Shown.Focus || m.houseFinder.Cursor != len(m.houseFinder.Matches)-1 {
		t.Errorf("Up from the first form: focus %v, cursor %d; want the last system", m.houseFinder.Shown.Focus, m.houseFinder.Cursor)
	}
}

func TestShownAsEnterOnAFormAppliesItAndLeavesTheSystem(t *testing.T) {
	m := keepFixture(t)
	before := currentPrefs(t)
	own := m.ownHouses(m.focusedSlot()).hs
	m = openFinder(t, m)
	m, _ = press(t, m, "end", "down", "down", "down") // forms 0 -> 1 -> 2
	if got := m.houseFinder.Shown.List.Cursor; got != 2 {
		t.Fatalf("cursor on form %d, want 2", got)
	}
	was := inForce(t, m)
	if was == houseForms[2].form {
		t.Fatal("fixture: form 2 is already in force")
	}
	m, _ = press(t, m, "enter")
	if got := inForce(t, m); got != houseForms[2].form || m.startup != startupReady {
		t.Errorf("Enter left form %v, startup %q; want form %v in force and the chooser closed", got, m.startup, houseForms[2].form)
	}
	if got := m.ownHouses(m.focusedSlot()).hs; got != own {
		t.Errorf("Enter on a form changed the chart's system to %v, want %v", got, own)
	}
	if currentPrefs(t) != before {
		t.Errorf("Enter on a form wrote the preferences:\n%s", currentPrefs(t))
	}
}

func TestShownAsSpaceDoesNothing(t *testing.T) {
	m := openFinder(t, finderModel(t, 2, 0))
	was := inForce(t, m)
	m, _ = press(t, m, "end", "down", "down")
	marks := shownMarks(t, m)
	m, cmd := press(t, m, " ")
	if cmd != nil {
		t.Error("Space scheduled a command")
	}
	if got := inForce(t, m); got != was || m.startup != startupHouseOpen || m.houseFinder.FilterText != "" {
		t.Errorf("Space: form %v, startup %q, filter %q; want nothing changed", got, m.startup, m.houseFinder.FilterText)
	}
	if got := shownMarks(t, m); !reflect.DeepEqual(got, marks) {
		t.Errorf("marks after Space = %q, want %q", got, marks)
	}
}

func TestShownAsEscKeepsWhatWasInForce(t *testing.T) {
	m := openFinder(t, finderModel(t, 2, 0))
	was := inForce(t, m)
	m, _ = press(t, m, "end", "down", "down", "esc")
	if got := inForce(t, m); got != was || m.startup != startupReady {
		t.Errorf("Esc left form %v, startup %q; want %v and the chooser closed", got, m.startup, was)
	}
}

func TestShownAsTypingDoesNothingAndTheSearchStaysInTheSystems(t *testing.T) {
	m := openFinder(t, finderModel(t, 2, 0))
	m, _ = press(t, m, "end", "down", "k", "o", "q")
	if m.houseFinder.FilterText != "" || m.startup != startupHouseOpen {
		t.Errorf("typing in the Shown as part left filter %q, startup %q", m.houseFinder.FilterText, m.startup)
	}
	m, _ = press(t, m, "up", "k", "o")
	if m.houseFinder.FilterText != "ko" {
		t.Errorf("typing in the systems left filter %q, want ko", m.houseFinder.FilterText)
	}
}

func TestShownAsPagingKeysReachTheFirstAndLastForm(t *testing.T) {
	m := openFinder(t, finderModel(t, 2, 0))
	m, _ = press(t, m, "end", "down", "end")
	if got := m.houseFinder.Shown.List.Cursor; got != len(houseForms)-1 {
		t.Errorf("End put the cursor on form %d, want the last", got)
	}
	m, _ = press(t, m, "home")
	if got := m.houseFinder.Shown.List.Cursor; got != 0 {
		t.Errorf("Home put the cursor on form %d, want the first form", got)
	}
}

// Tab keeps nothing in a chooser: no table binds it.
func TestNoChooserTableBindsTab(t *testing.T) {
	for name, table := range map[string][]keys.BindingEntry{"houses": housesBindings(), "zodiac": zodiacBindings(), "city": cityBindings()} {
		if got := keys.MatchBinding(tea.KeyMsg{Type: tea.KeyTab}, table); got != keys.BindingNone {
			t.Errorf("Tab matches %v in the %s table, want nothing", got, name)
		}
		if got := keys.MatchBinding(tea.KeyMsg{Type: tea.KeySpace}, table); got != keys.BindingNone && name != "city" {
			t.Errorf("Space matches %v in the %s table, want nothing", got, name)
		}
	}
}
