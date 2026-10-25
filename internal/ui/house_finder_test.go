package ui

// the specification. Chart values are constructed synthetic test inputs.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

var (
	keyCtrlHMain = tea.KeyMsg{Type: tea.KeyCtrlH}
	keyRuneH     = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")}
)

// finderModel is a ready workspace of n anchored charts with the cursor on
// list index cursor, in colours-off ASCII so the cursor row shows as "> ".
func finderModel(t *testing.T, n, cursor int) Model {
	t.Helper()
	sessionSandbox(t)
	stubHouseLookups(t)
	m := listModel(t, n, cursor)
	m.cfg = Config{Accessible: true, Color: ColorNever}
	return m
}

func openFinder(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = tpress(t, m, keyCtrlHMain)
	if m.startup != startupHouseOpen {
		t.Fatalf("Ctrl+H did not open the finder: startup = %q", m.startup)
	}
	return m
}

func typeInFinder(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func finderNames(m Model) []string {
	var out []string
	for _, h := range m.houseFinder.matches {
		out = append(out, lookup.HouseName(h))
	}
	return out
}

var cursorRowRE = regexp.MustCompile(`[|│]\s+> (.+?)(\s{2,}|\s*[|│]|$)`)

// visibleCursorRow is the name on the row the screen marks as the cursor's.
func visibleCursorRow(t *testing.T, m Model) string {
	t.Helper()
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
		if g := cursorRowRE.FindStringSubmatch(l); g != nil && !strings.Contains(l, "Search:") {
			return strings.TrimSpace(g[1])
		}
	}
	t.Fatalf("no cursor row on screen:\n%s", ansi.Strip(m.View()))
	return ""
}

func sessionFrames(t *testing.T) []store.FrameRecord {
	t.Helper()
	s, ok, err := store.LoadSession()
	if err != nil || !ok {
		return nil
	}
	return s.Frames
}

func sessionExists() bool {
	_, err := os.Stat(store.SessionPath())
	return err == nil
}

func systemNamed(t *testing.T, name string) recipe.HouseSystem {
	t.Helper()
	for h := recipe.HouseSystem(0); h < recipe.HouseSystemCount; h++ {
		if lookup.HouseName(h) == name {
			return h
		}
	}
	t.Fatalf("no house system named %q", name)
	return 0
}

// C13. Positive control: removing the Ctrl+H binding fails this test.
func TestHouseFinderKeys(t *testing.T) {
	t.Run("ctrl+h opens it for the focused chart, esc closes", func(t *testing.T) {
		m := openFinder(t, finderModel(t, 2, 1))
		m, _ = tpress(t, m, keyEsc)
		if m.startup != startupReady {
			t.Errorf("esc left startup = %q", m.startup)
		}
	})
	t.Run("the leader sequence , h opens it", func(t *testing.T) {
		m := finderModel(t, 2, 0)
		m, _ = press(t, m, ",", "h")
		if m.startup != startupHouseOpen {
			t.Errorf(", h left startup = %q", m.startup)
		}
	})
	t.Run("Help lists both, the key bar neither", func(t *testing.T) {
		m := finderModel(t, 2, 0)
		var key, seq bool
		for _, l := range m.helpLines() {
			if strings.Contains(l.text, "Choose houses") {
				key = key || strings.Contains(l.text, "⌃h")
				seq = seq || strings.Contains(l.text, ",h")
			}
		}
		if !key || !seq {
			t.Errorf("Help lists ctrl+h = %v and the leader sequence = %v", key, seq)
		}
		for _, e := range m.keyBarEntries() {
			if e.Bar && (e.ID == BindingHouses || e.Binding.Help().Desc == "Choose houses") {
				t.Errorf("the key bar lists the finder: %+v", e)
			}
		}
	})
}

// Pass 7: h is the hour step; after the leader it opens the finder instead.
func TestLeaderHOpensFinderWithoutSteppingTime(t *testing.T) {
	m := finderModel(t, 1, 1) // the floating slot, where h steps the hour
	before := m.slots.Floating().Time
	target := m.targetFloatingTime
	m, _ = press(t, m, ",", "h")
	if m.startup != startupHouseOpen {
		t.Fatalf(", h left startup = %q", m.startup)
	}
	if got := m.slots.Floating().Time; !got.Equal(before) || !m.targetFloatingTime.Equal(target) {
		t.Errorf("the hour stepped: %v -> %v", before, got)
	}
	// Control: bare h on the same screen does step the hour.
	c := finderModel(t, 1, 1)
	c, cmd := press(t, c, "h")
	_ = pump(t, c, cmd)
	if c.startup == startupHouseOpen {
		t.Errorf("bare h opened the finder")
	}
}

// C14.
func TestHouseFinderAppliesOnlyOnEnter(t *testing.T) {
	setup := func(t *testing.T) Model {
		m := selectedSet(t, 2, false, 1)
		m.cfg = Config{Accessible: true, Color: ColorNever}
		sessionSandbox(t)
		stubHouseLookups(t)
		m.frames = m.frames.with(chosen(m, 0, recipe.Placidus))
		m.frames = m.frames.with(chosen(m, 1, recipe.Equal))
		return m
	}
	t.Run("opens on the focused chart's own system, whatever frames the comparison", func(t *testing.T) {
		// Now visits Ben: Ben's Equal frames the comparison; now's own is
		// Placidus. The finder edits now's own, so the cursor opens on Placidus.
		m := selectedSet(t, 2, true, 2)
		m.cfg = Config{Accessible: true, Color: ColorNever}
		sessionSandbox(t)
		stubHouseLookups(t)
		m.frames = m.frames.with(chosen(m, 1, recipe.Equal))
		m.frames = m.frames.with(store.FrameRecord{Floating: true, Houses: recipe.Placidus, HasHouses: true, From: store.FrameFromChosen})
		m = choosePair(m, scenePair{left: slotAt(m, 1), right: slotAt(m, 2)})
		if got := m.comparisonLens().Houses; got != recipe.Equal {
			t.Fatalf("setup: the comparison should be framed by Ben's Equal, got %v", got)
		}
		m = openFinder(t, m)
		if got := m.houseFinder.matches[m.houseFinder.cursor]; got != recipe.Placidus {
			t.Errorf("cursor opened on %s, want now's own Placidus", lookup.HouseName(got))
		}
	})
	t.Run("moving and esc change nothing", func(t *testing.T) {
		m := setup(t)
		recs := append([]store.FrameRecord(nil), m.frames.records...)
		m = openFinder(t, m)
		for i := 0; i < 4; i++ {
			m, _ = tpress(t, m, keyDown)
		}
		m, _ = tpress(t, m, keyEsc)
		if len(m.frames.records) != len(recs) {
			t.Fatalf("records changed: %+v", m.frames.records)
		}
		for i := range recs {
			if m.frames.records[i] != recs[i] {
				t.Errorf("record %d changed: %+v", i, m.frames.records[i])
			}
		}
		if sessionExists() {
			t.Error("a cursor move or esc wrote the session")
		}
	})
	t.Run("enter applies the row and closes", func(t *testing.T) {
		m := setup(t)
		m = openFinder(t, m)
		m, _ = tpress(t, m, keyDown)
		want := m.houseFinder.matches[m.houseFinder.cursor]
		if sessionExists() {
			t.Fatal("a cursor move wrote the session")
		}
		var cmd tea.Cmd
		m, cmd = tpress(t, m, keyEnter)
		m = pump(t, m, cmd)
		if m.startup != startupReady {
			t.Errorf("enter left startup = %q", m.startup)
		}
		if got := m.ownHouses(m.focusedSlot()).hs; got != want {
			t.Errorf("own system = %s, want %s", lookup.HouseName(got), lookup.HouseName(want))
		}
	})
}

// C15.
func TestHouseFinderPinnedOnlyWhenEmpty(t *testing.T) {
	pinned := []string{"Placidus", "Koch", "Whole Sign", "Equal", "Porphyry", "Regiomontanus", "Campanus"}
	m := openFinder(t, finderModel(t, 1, 0))
	names := finderNames(m)
	if len(names) != 22 {
		t.Fatalf("%d rows with an empty search, want 22 each once: %v", len(names), names)
	}
	for i, p := range pinned {
		if names[i] != p {
			t.Errorf("row %d = %q, want pinned %q", i, names[i], p)
		}
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Errorf("%q is listed twice", n)
		}
		seen[n] = true
	}
	rest := names[len(pinned):]
	for i := 1; i < len(rest); i++ {
		if strings.ToLower(rest[i-1]) > strings.ToLower(rest[i]) {
			t.Errorf("below the pinned block %q comes before %q: not alphabetical", rest[i-1], rest[i])
		}
	}
	if !strings.Contains(ansi.Strip(m.View()), "-----") {
		t.Error("no rule between the pinned block and the rest")
	}

	m = typeInFinder(t, m, "pl")
	got := finderNames(m)
	want := []string{"Placidus", "Pullen (S.Ratio)", "Pullen (S.Delta)"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("pl = %v, want %v", got, want)
	}
	if strings.Count(ansi.Strip(m.View()), "Placidus") != 1 {
		t.Errorf("Placidus not listed exactly once:\n%s", ansi.Strip(m.View()))
	}
	m = typeInFinder(t, m, "x")
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	m = typeInFinder(t, m, "e")
	seen = map[string]bool{}
	for _, n := range finderNames(m) {
		if seen[n] {
			t.Errorf("%q listed twice under a search", n)
		}
		seen[n] = true
	}
	if strings.Contains(ansi.Strip(m.View()), "-----------") && !strings.Contains(ansi.Strip(m.View()), "Search: e█") {
		t.Error("search echo missing")
	}
}

func topEdge(t *testing.T, m Model) string {
	t.Helper()
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(l, "Houses ·") {
			return l
		}
	}
	t.Fatalf("no finder title on screen:\n%s", ansi.Strip(m.View()))
	return ""
}

// C16.
func TestHouseFinderTitleNamesItsChart(t *testing.T) {
	t.Run("an unselected chart", func(t *testing.T) {
		m := openFinder(t, finderModel(t, 2, 1))
		if e := topEdge(t, m); !strings.Contains(e, "Houses · Synthetic 2") || strings.Contains(e, "comparison") {
			t.Errorf("title = %q", e)
		}
	})
	t.Run("a selected chart on its own tab frames itself and names no one else", func(t *testing.T) {
		m := selectedSet(t, 2, false, 1)
		m.cfg = Config{Accessible: true, Color: ColorNever}
		sessionSandbox(t)
		stubHouseLookups(t)
		m = openFinder(t, m)
		if e := topEdge(t, m); !strings.Contains(e, "Houses · Synthetic 2") || strings.Contains(e, "comparison") {
			t.Errorf("title = %q", e)
		}
	})
	t.Run("a chart visiting another names itself, the one Enter edits", func(t *testing.T) {
		m := selectedSet(t, 2, true, 2)
		m.cfg = Config{Accessible: true, Color: ColorNever}
		sessionSandbox(t)
		stubHouseLookups(t)
		m = choosePair(m, scenePair{left: slotAt(m, 1), right: slotAt(m, 2)})
		m = openFinder(t, m)
		if e := topEdge(t, m); !strings.Contains(e, "Houses · Now") || strings.Contains(e, "comparison") {
			t.Errorf("title = %q", e)
		}
	})
	t.Run("the first selected chart decides for itself", func(t *testing.T) {
		m := selectedSet(t, 2, false, 0)
		m.cfg = Config{Accessible: true, Color: ColorNever}
		sessionSandbox(t)
		stubHouseLookups(t)
		m = openFinder(t, m)
		if e := topEdge(t, m); !strings.Contains(e, "Houses · Synthetic 1") || strings.Contains(e, "comparison") {
			t.Errorf("title = %q", e)
		}
	})
}

// Build note: Ctrl+H is a backspace inside a search box, in the finder itself
// as everywhere else (the specification).
func TestCtrlHInsideASearchBoxStillDeletes(t *testing.T) {
	m := openFinder(t, finderModel(t, 1, 0))
	m = typeInFinder(t, m, "pl")
	m, _ = tpress(t, m, keyCtrlHMain)
	if m.startup != startupHouseOpen || m.houseFinder.filterText != "p" {
		t.Errorf("in the finder ctrl+h left startup %q, filter %q; want it open with %q", m.startup, m.houseFinder.filterText, "p")
	}
	a := finderModel(t, 1, 0)
	a.atlas.filterText = "ab"
	a.atlas.prior = a.startup
	a.startup = startupAtlasOpen
	a, _ = tpress(t, a, keyCtrlHMain)
	if a.startup != startupAtlasOpen || a.atlas.filterText != "a" {
		t.Errorf("in the atlas ctrl+h left startup %q, filter %q", a.startup, a.atlas.filterText)
	}
}

// Build note: Enter applies through chooseHouseSystem and nothing else.
func TestFinderAppliesThroughTheChooser(t *testing.T) {
	// The reference: the shipped door, called directly.
	ref := finderModel(t, 2, 1)
	ref, cmd := ref.chooseHouseSystem(1, recipe.Koch)
	ref = pump(t, ref, cmd)
	want := sessionFrames(t)
	if len(want) != 1 {
		t.Fatalf("setup: the shipped door wrote %+v", want)
	}
	os.Remove(store.SessionPath())

	m := openFinder(t, finderModel(t, 2, 1))
	m, _ = tpress(t, m, keyDown)
	m, _ = tpress(t, m, keyDown)
	m, _ = tpress(t, m, keyUp)
	if sessionExists() {
		t.Fatal("cursor moves wrote the session")
	}
	m = typeInFinder(t, m, "koch")
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

// Pass 6: no matches, and a shrinking list.
func TestFinderEmptyAndShrinkingResults(t *testing.T) {
	t.Run("no matches: enter changes nothing, esc closes", func(t *testing.T) {
		m := openFinder(t, finderModel(t, 1, 0))
		before := m.ownHouses(m.focusedSlot())
		m = typeInFinder(t, m, "zzz")
		if len(m.houseFinder.matches) != 0 {
			t.Fatalf("zzz matched %v", finderNames(m))
		}
		var cmd tea.Cmd
		m, cmd = tpress(t, m, keyEnter)
		m = pump(t, m, cmd)
		if sessionExists() || m.ownHouses(m.focusedSlot()) != before {
			t.Error("enter on an empty result changed something")
		}
		if m.startup != startupHouseOpen {
			t.Errorf("enter on an empty result left startup = %q", m.startup)
		}
		m, _ = tpress(t, m, keyEsc)
		if m.startup != startupReady {
			t.Errorf("esc left startup = %q", m.startup)
		}
	})
	t.Run("filtered to zero and back to one: enter applies the visible row", func(t *testing.T) {
		m := openFinder(t, finderModel(t, 1, 0))
		for i := 0; i < 5; i++ {
			m, _ = tpress(t, m, keyDown)
		}
		m = typeInFinder(t, m, "placz")
		if len(m.houseFinder.matches) != 0 {
			t.Fatalf("placz matched %v", finderNames(m))
		}
		m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
		if got := visibleCursorRow(t, m); got != "Placidus" {
			t.Fatalf("visible row = %q", got)
		}
		var cmd tea.Cmd
		m, cmd = tpress(t, m, keyEnter)
		m = pump(t, m, cmd)
		if got := m.ownHouses(m.focusedSlot()).hs; got != recipe.Placidus {
			t.Errorf("applied %s, want the visible Placidus", lookup.HouseName(got))
		}
	})
}

// Pass 6: another owner's keyboard is not stolen.
func TestHouseFinderDoesNotStealOtherOwners(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		m := finderModel(t, 1, 0)
		m, _ = press(t, m, "?")
		if !m.helpOpen {
			t.Fatal("setup: help did not open")
		}
		m, _ = tpress(t, m, keyCtrlHMain)
		if m.startup == startupHouseOpen || !m.helpOpen {
			t.Errorf("ctrl+h over help: startup %q, help open %v", m.startup, m.helpOpen)
		}
	})
	t.Run("console", func(t *testing.T) {
		m := finderModel(t, 1, 0)
		m, _ = press(t, m, ",", "l")
		if !m.console.open {
			t.Fatal("setup: console did not open")
		}
		m, _ = tpress(t, m, keyCtrlHMain)
		if m.startup == startupHouseOpen {
			t.Error("ctrl+h over the console opened the finder")
		}
	})
	t.Run("another popup", func(t *testing.T) {
		m := finderModel(t, 1, 0)
		m.startup = startupAtlasOpen
		m, _ = tpress(t, m, keyCtrlHMain)
		if m.startup != startupAtlasOpen {
			t.Errorf("ctrl+h over the atlas left startup = %q", m.startup)
		}
	})
	t.Run("a pending leader", func(t *testing.T) {
		m := finderModel(t, 1, 0)
		m, _ = press(t, m, ",")
		m, _ = tpress(t, m, keyCtrlHMain)
		if m.startup == startupHouseOpen {
			t.Error("ctrl+h inside a leader sequence opened the finder")
		}
	})
}

// Pass 6: the title never loses which chart Enter changes.
func TestHouseFinderLongTitleAt80x24(t *testing.T) {
	long := "A Considerably Longer Constructed Subject Name Than Any Box Holds"
	for _, withBase := range []bool{false, true} {
		var m Model
		if withBase {
			m = selectedSet(t, 2, false, 1)
			m.cfg = Config{Accessible: true, Color: ColorNever}
			sessionSandbox(t)
			stubHouseLookups(t)
			m.slots.All()[1].Chart.Name = long
			m.slots.All()[0].Chart.Name = long + " Two"
		} else {
			m = finderModel(t, 2, 1)
			m.slots.All()[1].Chart.Name = long
		}
		m.width, m.height = 80, 24
		m = openFinder(t, m)
		e := topEdge(t, m)
		if w := ansi.StringWidth(e); w > 80 {
			t.Errorf("withBase=%v: title edge is %d cells at 80x24: %q", withBase, w, e)
		}
		if !strings.Contains(e, "Houses · A Considerably") {
			t.Errorf("withBase=%v: the chart Enter changes is hidden: %q", withBase, e)
		}
		if !strings.Contains(e, "…") && !strings.Contains(e, "~") && !strings.Contains(e, "...") {
			t.Errorf("withBase=%v: a cut title carries no cut mark: %q", withBase, e)
		}
	}
}

// Pass 7: Enter on the current row is a choice.
func TestEnterOnCurrentRowRecordsAChoice(t *testing.T) {
	m := finderModel(t, 1, 0)
	slot := m.slots.All()[0]
	m.frames = m.frames.withFile("synthetic-1.as", fileFrame{when: slot.Time, houses: recipe.Equal, has: true})
	m = openFinder(t, m)
	if got := m.houseFinder.matches[m.houseFinder.cursor]; got != recipe.Equal {
		t.Fatalf("cursor opened on %s, want the file's Equal", lookup.HouseName(got))
	}
	var cmd tea.Cmd
	m, cmd = tpress(t, m, keyEnter)
	_ = pump(t, m, cmd)
	got := sessionFrames(t)
	if len(got) != 1 || got[0].Houses != recipe.Equal || got[0].From != store.FrameFromChosen || !got[0].HasHouses {
		t.Errorf("session frames = %+v, want one chosen Equal", got)
	}
}

// Pass 7: Enter acts on the row the screen marks, after a filter.
func TestFilteredFinderAppliesTheVisibleCursorRow(t *testing.T) {
	m := openFinder(t, finderModel(t, 1, 0))
	m = typeInFinder(t, m, "e")
	for i := 0; i < 3; i++ {
		m, _ = tpress(t, m, keyDown)
	}
	m = typeInFinder(t, m, "q")
	shown := visibleCursorRow(t, m)
	var cmd tea.Cmd
	m, cmd = tpress(t, m, keyEnter)
	m = pump(t, m, cmd)
	if got := lookup.HouseName(m.ownHouses(m.focusedSlot()).hs); got != shown {
		t.Errorf("applied %q, the screen marked %q", got, shown)
	}
}

// Pass 7: the same, after the window has scrolled.
func TestScrolledFinderAppliesTheVisibleRow(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {132, 40}} {
		m := finderModel(t, 1, 0)
		m.width, m.height = size[0], size[1]
		m = openFinder(t, m)
		for i := 0; i < 17; i++ {
			m, _ = tpress(t, m, keyDown)
		}
		shown := visibleCursorRow(t, m)
		// The cursor opened on Whole Sign (row 2), so 17 steps land on row 19.
		if c := m.houseFinder.cursor; c != 19 || shown != finderNames(m)[c] {
			t.Errorf("%v: the screen marks %q, the cursor is on row %d, %q", size, shown, c, finderNames(m)[c])
		}
		var cmd tea.Cmd
		m, cmd = tpress(t, m, keyEnter)
		m = pump(t, m, cmd)
		if got := lookup.HouseName(m.ownHouses(m.focusedSlot()).hs); got != shown {
			t.Errorf("%v: applied %q, the screen marked %q", size, got, shown)
		}
	}
}
