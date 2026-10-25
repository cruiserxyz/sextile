package ui

// the specification C8 - the zodiac finder (⌃z, or `, z`), its own popup
// beside the house finder (operator, 2026-10-08). Chart values are constructed
// synthetic test inputs.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

var keyCtrlZ = tea.KeyMsg{Type: tea.KeyCtrlZ}

// openZodiacFinderOn presses ⌃z on the main screen.
func openZodiacFinderOn(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = tpress(t, m, keyCtrlZ)
	if m.startup != startupZodiacOpen {
		t.Fatalf("Ctrl+Z did not open the zodiac finder: startup = %q", m.startup)
	}
	return m
}

func zodiacRowNames(m Model) []string {
	var out []string
	for _, z := range m.zodiacFinder.matches {
		out = append(out, z.Name())
	}
	return out
}

func zodiacTopEdge(t *testing.T, m Model) string {
	t.Helper()
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(l, "Zodiac ·") {
			return l
		}
	}
	t.Fatalf("no zodiac finder title on screen:\n%s", ansi.Strip(m.View()))
	return ""
}

// C8. Positive control: changing the ⌃z binding fails this test by name.
func TestZodiacFinderKeys(t *testing.T) {
	t.Run("ctrl+z opens it for the focused chart, esc closes", func(t *testing.T) {
		m := openZodiacFinderOn(t, finderModel(t, 2, 1))
		if m.zodiacFinder.slotIdx != 1 {
			t.Errorf("the finder is for slot %d, want the focused 1", m.zodiacFinder.slotIdx)
		}
		m, _ = tpress(t, m, keyEsc)
		if m.startup != startupReady {
			t.Errorf("esc left startup = %q", m.startup)
		}
	})
	t.Run("the leader sequence , z opens it", func(t *testing.T) {
		m := finderModel(t, 2, 0)
		m, _ = press(t, m, ",", "z")
		if m.startup != startupZodiacOpen {
			t.Errorf(", z left startup = %q", m.startup)
		}
	})
	t.Run("eleven rows: Tropical, then the ten in C2's order, footer n of 11", func(t *testing.T) {
		m := openZodiacFinderOn(t, finderModel(t, 1, 0))
		want := []string{"Tropical", "Fagan-Bradley", "Lahiri", "Deluce", "Raman", "Usha-Shasi", "Krishnamurti", "Djwhal Khul", "Yukteshwar", "J.N. Bhasin", "Galactic Center"}
		if got := zodiacRowNames(m); strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("rows = %v, want %v", got, want)
		}
		view := ansi.Strip(m.View())
		if !regexp.MustCompile(`\d+ of 11\b`).MatchString(view) {
			t.Errorf("the footer does not count 11 rows:\n%s", view)
		}
		for _, row := range []string{"Lahiri           Fagan-Bradley + 0.883208°", "Fagan-Bradley    Astrolog's default", "Deluce           Fagan-Bradley − 3.075453°"} {
			if !strings.Contains(view, row) {
				t.Errorf("the finder lacks the F1 row %q:\n%s", row, view)
			}
		}
	})
	t.Run("Help lists both, the key bar neither", func(t *testing.T) {
		m := finderModel(t, 2, 0)
		var key, seq bool
		for _, l := range m.helpLines() {
			if strings.Contains(l.text, "Choose zodiac") {
				key = key || strings.Contains(l.text, "⌃z")
				seq = seq || strings.Contains(l.text, ",z")
			}
		}
		if !key || !seq {
			t.Errorf("Help lists ctrl+z = %v and the leader sequence = %v", key, seq)
		}
		for _, e := range m.keyBarEntries() {
			if e.Bar && (e.ID == BindingZodiac || e.Binding.Help().Desc == "Choose zodiac") {
				t.Errorf("the key bar lists the zodiac finder: %+v", e)
			}
		}
	})
}

// C8: the cursor opens on the focused chart's own zodiac, selected or not.
func TestZodiacFinderOpensOnOwnZodiac(t *testing.T) {
	m := selectedSet(t, 3, true, 2)
	m.cfg = Config{Accessible: true, Color: ColorNever}
	sessionSandbox(t)
	stubFrameLookups(t)
	m.slots.SetSlotSelected(2, false)
	m.frames.records = append(m.frames.records, chosenZ(m, 2, recipe.Lahiri))
	if got := m.ownZodiac(m.focusedSlot()); got != recipe.Lahiri {
		t.Fatalf("setup: the unselected chart should show its own Lahiri, got %v", got)
	}
	m = openZodiacFinderOn(t, m)
	if got := m.zodiacFinder.matches[m.zodiacFinder.cursor]; got != recipe.Lahiri {
		t.Errorf("cursor opened on %s, want the chart's own Lahiri", got.Name())
	}
	if got := visibleCursorRow(t, m); got != "Lahiri" {
		t.Errorf("the screen marks %q, want Lahiri", got)
	}
	if !strings.Contains(strings.Join(strings.Fields(ansi.Strip(m.View())), " "), "Lahiri Fagan-Bradley + 0.883208° La ●") {
		t.Errorf("the current zodiac carries no ●:\n%s", ansi.Strip(m.View()))
	}

	g := selectedSet(t, 2, false, 1)
	g.cfg = Config{Accessible: true, Color: ColorNever}
	g.frames.records = []store.FrameRecord{chosenZ(g, 0, recipe.Deluce), chosenZ(g, 1, recipe.Raman)}
	g = openZodiacFinderOn(t, g)
	if got := g.zodiacFinder.matches[g.zodiacFinder.cursor]; got != recipe.Raman {
		t.Errorf("a selected second chart: cursor opened on %s, want its own Raman", got.Name())
	}
}

// C8: moving and typing change nothing; Enter applies; Esc changes nothing.
func TestZodiacFinderAppliesOnlyOnEnter(t *testing.T) {
	m := finderModel(t, 2, 0)
	m = openZodiacFinderOn(t, m)
	for i := 0; i < 4; i++ {
		m, _ = tpress(t, m, keyDown)
	}
	m = typeInFinder(t, m, "ra")
	if len(m.frames.records) != 0 || sessionExists() {
		t.Fatalf("moving and typing wrote something: %+v", m.frames.records)
	}
	m, _ = tpress(t, m, keyEsc)
	if m.startup != startupReady || len(m.frames.records) != 0 || sessionExists() {
		t.Errorf("esc changed something: startup %q records %+v", m.startup, m.frames.records)
	}
	if got := m.ownZodiac(m.focusedSlot()); got != recipe.Tropical {
		t.Errorf("own zodiac = %v after esc", got)
	}

	m = openZodiacFinderOn(t, m)
	m = typeInFinder(t, m, "raman")
	var cmd tea.Cmd
	m, cmd = tpress(t, m, keyEnter)
	m = pump(t, m, cmd)
	if m.startup != startupReady {
		t.Errorf("enter left startup = %q", m.startup)
	}
	if got := m.ownZodiac(m.focusedSlot()); got != recipe.Raman {
		t.Errorf("own zodiac = %v, want Raman", got)
	}
	if got := sessionFrames(t); len(got) != 1 || got[0].Zodiac != "sidereal Raman" {
		t.Errorf("session frames = %+v, want one Raman record", got)
	}
}

// Enter on the current row is a choice too (it records, as the house finder's does).
func TestZodiacFinderEnterOnCurrentRowRecordsAChoice(t *testing.T) {
	m := openZodiacFinderOn(t, finderModel(t, 1, 0))
	var cmd tea.Cmd
	m, cmd = tpress(t, m, keyEnter)
	_ = pump(t, m, cmd)
	got := sessionFrames(t)
	if len(got) != 1 || got[0].Zodiac != "tropical" || !got[0].HasZodiac {
		t.Errorf("session frames = %+v, want one recorded tropical", got)
	}
}

// Enter applies through chooseZodiac and nothing else.
func TestZodiacFinderAppliesThroughTheChooser(t *testing.T) {
	ref := finderModel(t, 2, 1)
	ref, cmd := ref.chooseZodiac(1, recipe.Lahiri)
	ref = pump(t, ref, cmd)
	want := sessionFrames(t)
	if len(want) != 1 {
		t.Fatalf("setup: the shipped door wrote %+v", want)
	}
	if err := os.Remove(store.SessionPath()); err != nil {
		t.Fatal(err)
	}

	m := openZodiacFinderOn(t, finderModel(t, 2, 1))
	m, _ = tpress(t, m, keyDown)
	m, _ = tpress(t, m, keyDown)
	m, _ = tpress(t, m, keyUp)
	if sessionExists() {
		t.Fatal("cursor moves wrote the session")
	}
	m = typeInFinder(t, m, "lahiri")
	if sessionExists() {
		t.Fatal("typing wrote the session")
	}
	m, cmd = tpress(t, m, keyEnter)
	m = pump(t, m, cmd)
	got := sessionFrames(t)
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("session after Enter = %+v, want what the chooser writes: %+v", got, want)
	}
	if len(m.frames.records) != len(ref.frames.records) {
		t.Errorf("records = %+v, chooser's = %+v", m.frames.records, ref.frames.records)
	}
}

// C8: the title names the chart the choice changes: the one that decides.
func TestZodiacFinderTitleNamesItsChart(t *testing.T) {
	t.Run("an unselected chart", func(t *testing.T) {
		m := openZodiacFinderOn(t, finderModel(t, 2, 1))
		if e := zodiacTopEdge(t, m); !strings.Contains(e, "Zodiac · Synthetic 2") || strings.Contains(e, "comparison") {
			t.Errorf("title = %q", e)
		}
	})
	t.Run("a selected chart that is not the first names itself", func(t *testing.T) {
		m := selectedSet(t, 2, false, 1)
		m.cfg = Config{Accessible: true, Color: ColorNever}
		sessionSandbox(t)
		stubFrameLookups(t)
		m = openZodiacFinderOn(t, m)
		if e := zodiacTopEdge(t, m); !strings.Contains(e, "Zodiac · Synthetic 2") || strings.Contains(e, "comparison") {
			t.Errorf("title = %q", e)
		}
	})
	t.Run("the first selected chart decides for itself", func(t *testing.T) {
		m := selectedSet(t, 2, false, 0)
		m.cfg = Config{Accessible: true, Color: ColorNever}
		sessionSandbox(t)
		stubFrameLookups(t)
		m = openZodiacFinderOn(t, m)
		if e := zodiacTopEdge(t, m); !strings.Contains(e, "Zodiac · Synthetic 1") || strings.Contains(e, "comparison") {
			t.Errorf("title = %q", e)
		}
	})
	t.Run("a long name never hides the chart it changes at 80x24", func(t *testing.T) {
		long := "A Considerably Longer Constructed Subject Name Than Any Box Holds"
		m := selectedSet(t, 2, false, 1)
		m.cfg = Config{Accessible: true, Color: ColorNever}
		sessionSandbox(t)
		stubFrameLookups(t)
		m.slots.All()[1].Chart.Name = long
		m.slots.All()[0].Chart.Name = long + " Two"
		m.width, m.height = 80, 24
		m = openZodiacFinderOn(t, m)
		e := zodiacTopEdge(t, m)
		if w := ansi.StringWidth(e); w > 80 {
			t.Errorf("title edge is %d cells at 80x24: %q", w, e)
		}
		if !strings.Contains(e, "Zodiac · A Considerably") {
			t.Errorf("the chart the choice changes is hidden: %q", e)
		}
	})
}

// C8: another owner's keyboard is not stolen.
func TestZodiacFinderDoesNotStealOtherOwners(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		m := finderModel(t, 1, 0)
		m, _ = press(t, m, "?")
		if !m.helpOpen {
			t.Fatal("setup: help did not open")
		}
		m, _ = tpress(t, m, keyCtrlZ)
		if m.startup == startupZodiacOpen || !m.helpOpen {
			t.Errorf("ctrl+z over help: startup %q, help open %v", m.startup, m.helpOpen)
		}
	})
	t.Run("console", func(t *testing.T) {
		m := finderModel(t, 1, 0)
		m, _ = press(t, m, ",", "l")
		if !m.console.open {
			t.Fatal("setup: console did not open")
		}
		m, _ = tpress(t, m, keyCtrlZ)
		if m.startup == startupZodiacOpen {
			t.Error("ctrl+z over the console opened the finder")
		}
	})
	t.Run("another popup", func(t *testing.T) {
		m := finderModel(t, 1, 0)
		m.startup = startupAtlasOpen
		m, _ = tpress(t, m, keyCtrlZ)
		if m.startup != startupAtlasOpen {
			t.Errorf("ctrl+z over the atlas left startup = %q", m.startup)
		}
	})
	t.Run("the house finder", func(t *testing.T) {
		m := openFinder(t, finderModel(t, 1, 0))
		m, _ = tpress(t, m, keyCtrlZ)
		if m.startup != startupHouseOpen {
			t.Errorf("ctrl+z over the house finder left startup = %q", m.startup)
		}
	})
	t.Run("and the house finder's ctrl+h is not the zodiac finder's", func(t *testing.T) {
		m := openZodiacFinderOn(t, finderModel(t, 1, 0))
		m = typeInFinder(t, m, "ra")
		m, _ = tpress(t, m, keyCtrlHMain)
		if m.startup != startupZodiacOpen || m.zodiacFinder.filterText != "r" {
			t.Errorf("ctrl+h in the zodiac finder: startup %q filter %q, want a backspace", m.startup, m.zodiacFinder.filterText)
		}
	})
	t.Run("a pending leader", func(t *testing.T) {
		m := finderModel(t, 1, 0)
		m, _ = press(t, m, ",")
		m, _ = tpress(t, m, keyCtrlZ)
		if m.startup == startupZodiacOpen {
			t.Error("ctrl+z inside a leader sequence opened the finder")
		}
	})
	t.Run("z alone does nothing, and the leader z steps no time", func(t *testing.T) {
		m := finderModel(t, 1, 1)
		before := m.slots.Floating().Time
		m, cmd := press(t, m, "z")
		_ = pump(t, m, cmd)
		if m.startup == startupZodiacOpen || !m.slots.Floating().Time.Equal(before) {
			t.Errorf("bare z did something: startup %q", m.startup)
		}
		m, _ = press(t, finderModel(t, 1, 1), ",", "z")
		if m.startup != startupZodiacOpen || !m.slots.Floating().Time.Equal(before) {
			t.Errorf(", z: startup %q, time moved: %v", m.startup, !m.slots.Floating().Time.Equal(before))
		}
	})
}

// Bubble Tea delivers ⌃z as an ordinary key, and nothing the app returns
// suspends the program.
func TestCtrlZDoesNotSuspend(t *testing.T) {
	msg := readOneInputMsg(t, "\x1a")
	k, ok := msg.(tea.KeyMsg)
	if !ok || k.Type != tea.KeyCtrlZ {
		t.Fatalf("Bubble Tea delivered %T %v for ^Z, want tea.KeyCtrlZ", msg, msg)
	}
	var suspends func(cmd tea.Cmd) bool
	suspends = func(cmd tea.Cmd) bool {
		if cmd == nil {
			return false
		}
		switch out := cmd().(type) {
		case tea.SuspendMsg:
			return true
		case tea.BatchMsg:
			for _, c := range out {
				if suspends(c) {
					return true
				}
			}
		}
		return false
	}
	for name, m := range map[string]Model{
		"main screen":     finderModel(t, 1, 0),
		"the zodiac open": openZodiacFinderOn(t, finderModel(t, 1, 0)),
		"the house open":  openFinder(t, finderModel(t, 1, 0)),
		"help":            func() Model { m, _ := press(t, finderModel(t, 1, 0), "?"); return m }(),
	} {
		next, cmd := m.Update(msg)
		if suspends(cmd) {
			t.Errorf("%s: ^Z returned tea.Suspend", name)
		}
		if _, ok := next.(Model); !ok {
			t.Errorf("%s: Update returned %T", name, next)
		}
	}
}
