package ui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// keybindings_popup_test.go holds the Keybindings popup: one list of keys
// opened by `?`, `, p` and Ctrl+P, in three sections, filtered by what is
// typed. The seam under it (runAction, blocked) is actions_test.go.

var keyCtrlP = tea.KeyMsg{Type: tea.KeyCtrlP}

func screen(m Model) string { return ansi.Strip(m.View()) }

// kbModel is a ready Chart-focused workspace: three opened charts above Now,
// the cursor on the middle one, in the plain style.
func kbModel(t *testing.T) Model {
	t.Helper()
	m := paletteModel(t)
	m.cfg = Config{Accessible: true, Color: ColorNever}
	m.focused = PanelChart
	return m
}

// openCommandsPopup is Ctrl+P on a ready model, and checks it opened.
func openCommandsPopup(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = tpress(t, m, keyCtrlP)
	if !m.keybindingsOpen || m.currentOwner() != ownerKeybindings {
		t.Fatalf("Ctrl+P did not open Keybindings: open=%v owner=%v", m.keybindingsOpen, m.currentOwner())
	}
	return m
}

// commandTitles are the descriptions of the Global rows now shown, in order.
func commandTitles(m Model) []string {
	var out []string
	for _, r := range m.keybindings.Matches {
		if r.Group == keys.GroupGlobal {
			out = append(out, r.Desc)
		}
	}
	return out
}

// runViaCommands opens Keybindings at the commands, types query and presses
// Enter on the first row that matches.
func runViaCommands(t *testing.T, m Model, query string) Model {
	t.Helper()
	m = openCommandsPopup(t, m)
	m = kbType(t, m, query)
	m, _ = tpress(t, m, keyEnter)
	return m
}

// rowAt is the row with this description, or a failure.
func rowAt(t *testing.T, m Model, desc string) keys.Row {
	t.Helper()
	for _, r := range m.keybindings.All {
		if r.Desc == desc {
			return r
		}
	}
	t.Fatalf("no row %q in %v", desc, rowDescs(m.keybindings.All))
	return keys.Row{}
}

func rowDescs(rows []keys.Row) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Desc)
	}
	return out
}

// 1: `?`, `, ?` and Ctrl+P open the same popup, titled Keybindings. The Help
// modal is gone from the screen and from the key bar.
func TestThreeKeysOpenOneKeybindingsPopup(t *testing.T) {
	for _, tc := range []struct {
		name string
		keys []string
	}{{"?", []string{"?"}}, {", ?", []string{",", "?"}}, {"Ctrl+P", nil}} {
		t.Run(tc.name, func(t *testing.T) {
			m := kbModel(t)
			if tc.keys == nil {
				m, _ = tpress(t, m, keyCtrlP)
			} else {
				m, _ = press(t, m, tc.keys...)
			}
			if !m.keybindingsOpen || m.currentOwner() != ownerKeybindings {
				t.Fatalf("open=%v owner=%v", m.keybindingsOpen, m.currentOwner())
			}
			view := screen(m)
			if !strings.Contains(view, "Keybindings") || strings.Contains(view, "─Help") {
				t.Errorf("the popup is not titled Keybindings:\n%s", view)
			}
			if bar := strings.TrimSpace(lastRow(m.View())); strings.Contains(bar, "Help") {
				t.Errorf("the bar still says Help: %q", bar)
			}
		})
	}
}

// 8: Esc closes it, and `?` pressed again closes it.
func TestEscAndQuestionMarkClose(t *testing.T) {
	for _, closer := range []tea.KeyMsg{keyEsc, runes("?")} {
		m := openedKeybindings(kbModel(t))
		m, _ = tpress(t, m, closer)
		if m.keybindingsOpen || m.currentOwner() != ownerMain {
			t.Errorf("%v left it open (owner %v)", closer, m.currentOwner())
		}
	}
}

// 2, 25, 26: three groups with their headers, an empty line between two
// groups and none at the top or the bottom, and no line about the comma.
func TestKeybindingsHasThreeHeadersWithAnEmptyLineBetweenGroups(t *testing.T) {
	m := openedKeybindings(kbModel(t))
	if got, want := kbHeaders(m), []string{"─── Local", "─── Global", "─── Navigation"}; !slices.Equal(got, want) {
		t.Fatalf("headers %v, want %v", got, want)
	}
	lines := m.keybindings.Lines()
	if lines[0].Kind != keys.LineHeader {
		t.Errorf("the popup opens with %+v, want the first header", lines[0])
	}
	for i, l := range lines {
		if l.Kind == keys.LineHeader && i > 0 && lines[i-1].Kind != keys.LineBlank {
			t.Errorf("header %q is not under an empty line", l.Text)
		}
	}
	if k := lines[len(lines)-1].Kind; k == keys.LineBlank {
		t.Error("the list ends in an empty line")
	}
	if n := strings.Count(screen(m), "does the same as Ctrl"); n != 0 {
		t.Errorf("the comma sentence shows %d times, want none", n)
	}
}

// Local is the keys of the place the popup was opened from.
func TestLocalDiffersByPlace(t *testing.T) {
	local := func(m Model) []string {
		var out []string
		for _, r := range m.keybindings.All {
			if r.Group == keys.GroupLocal {
				out = append(out, r.Desc)
			}
		}
		return out
	}
	t.Run("the Chart panel", func(t *testing.T) {
		m := openedKeybindings(kbModel(t))
		for _, want := range []string{"Select", "Move chart up", "Move chart down", "Remove chart"} {
			if !slices.Contains(local(m), want) {
				t.Errorf("Local on the Chart panel lacks %q: %v", want, local(m))
			}
		}
	})
	t.Run("Bodies", func(t *testing.T) {
		b := kbModel(t)
		b.focused = PanelBodies
		m := openedKeybindings(b)
		if !slices.Contains(local(m), "Select") || slices.Contains(local(m), "Move chart up") {
			t.Errorf("Local on Bodies: %v, want Select and no chart list keys", local(m))
		}
	})
	t.Run("the Timeline", func(t *testing.T) {
		stubTextCompute(t)
		stubRawReports(t)
		m := openedKeybindings(onTimeline(t, Config{Accessible: true}))
		if m.keybindings.Origin != ownerDetails {
			t.Fatalf("origin %v, want attached Details", m.keybindings.Origin)
		}
		if r := rowAt(t, m, "Filter"); r.Key != "/" || r.Group != keys.GroupLocal {
			t.Errorf("the Timeline's search key reads %+v, want Local /", r)
		}
		if slices.Contains(local(m), "Move chart up") {
			t.Errorf("Local in Details holds a Chart panel key: %v", local(m))
		}
	})
	t.Run("the houses chooser", func(t *testing.T) {
		m := openFinder(t, finderModel(t, 2, 0))
		m, _ = tpress(t, m, runes("?"))
		if !m.keybindingsOpen || m.keybindings.Origin != ownerHouses {
			t.Fatalf("`?` in the chooser: open=%v origin %v", m.keybindingsOpen, m.keybindings.Origin)
		}
		for _, want := range []string{"Choose", "Cancel"} {
			if !slices.Contains(local(m), want) {
				t.Errorf("Local in the houses chooser lacks %q: %v", want, local(m))
			}
		}
		if !strings.Contains(screen(m), "Keybindings · Houses") {
			t.Errorf("the title does not name the chooser:\n%s", screen(m))
		}
	})
}

// "Nothing is left out": every key of the main tables that has a title is a
// row, once, and a registry entry cannot go missing unseen.
func TestNothingIsLeftOut(t *testing.T) {
	m := kbModel(t)
	m = openedKeybindings(m)
	count := map[keys.BindingID]int{}
	for _, r := range m.keybindings.All {
		count[r.ID]++
	}
	c := m.keysContextFor(ownerMain)
	for _, e := range slices.Concat(mainBindings(m.keys), leaderOnlyBindings()) {
		if e.TitleFor(c) == "" || !(e.Help || e.Palette) {
			continue
		}
		if count[e.ID] != 1 {
			t.Errorf("%q (id %d, %s) appears %d times, want once", e.TitleFor(c), e.ID, e.Group, count[e.ID])
		}
	}
	if !m.keybindings.Matches[0].Command && len(m.keybindings.All) < 30 {
		t.Errorf("only %d rows", len(m.keybindings.All))
	}
}

// 3: the group is one field of each registry entry, and Validate reads it.
func TestValidateReportsAnEntryWithoutAGroup(t *testing.T) {
	m := kbModel(t)
	r := keyRegistry(m.keys)
	if clashes := keys.Validate(r); len(clashes) != 0 {
		t.Fatalf("the tables as they are clash: %v", clashes)
	}
	table := r.Owners[keys.OwnerMain][0]
	name := table[3].Title
	table[3].Group = keys.GroupNone
	var found bool
	for _, c := range keys.Validate(r) {
		found = found || (c.Key == name && strings.Contains(c.What, "no group"))
	}
	if !found {
		t.Errorf("Validate did not name %q as an entry with no group: %v", name, keys.Validate(r))
	}
}

// The registry's main and details tables put every titled entry in a section.
func TestEveryEntryDeclaresItsGroup(t *testing.T) {
	m := kbModel(t)
	for o, tables := range keyRegistry(m.keys).Owners {
		for _, table := range tables {
			for _, e := range table {
				if e.Group == keys.GroupNone {
					t.Errorf("%s: %q declares no group", keys.OwnerName(o), e.Title)
				}
			}
		}
	}
}

// 4: the key sits right-aligned in its column, then the description.
func TestKeyColumnIsRightAligned(t *testing.T) {
	m := openedKeybindings(kbModel(t))
	view := screen(m)
	ends := map[string]int{}
	for _, l := range strings.Split(view, "\n") {
		for _, row := range []struct{ key, desc string }{{"<ctrl+o>", "Open chart"}, {"<ctrl+s>", "Save chart"}, {", O", "Sessions"}, {"?", "Keybindings"}} {
			if i := strings.Index(l, " "+row.desc); i >= 0 && strings.HasSuffix(strings.TrimRight(l[:i], " "), row.key) {
				ends[row.desc] = len([]rune(l[:i]))
			}
		}
	}
	if len(ends) < 3 {
		t.Fatalf("only %v of the sample rows are on screen:\n%s", ends, view)
	}
	var first int
	for desc, end := range ends {
		if first == 0 {
			first = end
		}
		if end != first {
			t.Errorf("%q: its key ends at column %d, the others at %d", desc, end, first)
		}
	}
}

// The key takes the blue-green colour; with colour off it is plain.
func TestKeyIsBlueGreenOnlyWithColour(t *testing.T) {
	forceColor(t)
	m := kbModel(t)
	m.cfg = Config{Color: ColorAlways}
	m = openedKeybindings(m)
	if !strings.Contains(m.View(), "\x1b[36m") {
		t.Error("no key is drawn in the blue-green colour")
	}
	m.cfg = Config{Accessible: true, Color: ColorNever}
	if strings.Contains(m.View(), "\x1b[36m") {
		t.Error("a key is coloured with colour off")
	}
}

// 5: opened with `?` the list starts at Local, the cursor on its first row;
// opened with Ctrl+P it is scrolled to Global, the cursor on Global's first row.
func TestOpenedAtLocalOrAtGlobal(t *testing.T) {
	m := openedKeybindings(kbModel(t))
	if r := m.keybindings.Matches[m.keybindings.Cursor]; r.Group != keys.GroupLocal || m.keybindings.Cursor != 0 {
		t.Errorf("`?` puts the cursor on %+v (row %d), want Local's first row", r, m.keybindings.Cursor)
	}
	if !strings.Contains(screen(m), "─── Local") {
		t.Errorf("`?` does not show Local:\n%s", screen(m))
	}
	g := openCommandsPopup(t, kbModel(t))
	if r := g.keybindings.Matches[g.keybindings.Cursor]; r.Group != keys.GroupGlobal {
		t.Errorf("Ctrl+P puts the cursor on %+v, want Global's first row", r)
	}
	if v := screen(g); !strings.Contains(v, "─── Global") || strings.Contains(v, "─── Local") {
		t.Errorf("Ctrl+P is not scrolled to Global:\n%s", v)
	}
}

// 6: Enter runs the row under the cursor and closes the popup, as its key
// would. A row that asks before it writes still asks.
func TestEnterRunsTheRowAndCloses(t *testing.T) {
	t.Run("a command", func(t *testing.T) {
		m := runViaCommands(t, kbModel(t), "choose houses")
		if m.keybindingsOpen || m.startup != startupHouseOpen || m.currentOwner() != ownerHouses {
			t.Fatalf("Enter on Choose houses: open=%v startup %q owner %v", m.keybindingsOpen, m.startup, m.currentOwner())
		}
		if len(m.recentActions) != 1 || m.recentActions[0] != keys.BindingHouses {
			t.Errorf("recent = %v, want [Houses]", m.recentActions)
		}
		pressed, _ := openHouses(t, kbModel(t))
		if pressed.startup != m.startup {
			t.Errorf("`, u` leaves startup %q, the popup %q", pressed.startup, m.startup)
		}
	})
	t.Run("a key of the place", func(t *testing.T) {
		m := kbModel(t)
		before := m.slots.All()[1].Time
		order := func(m Model) []string {
			var out []string
			for _, s := range m.slots.All() {
				out = append(out, s.Time.String())
			}
			return out
		}
		was := order(m)
		m = openedKeybindings(m)
		m = kbType(t, m, "chart up")
		m, _ = tpress(t, m, keyEnter)
		if m.keybindingsOpen {
			t.Error("Enter left the popup open")
		}
		if got := order(m); slices.Equal(got, was) {
			t.Errorf("Move chart up did not move a chart: %v (was %v, %v)", got, was, before)
		}
	})
}

// 7, 27: a row that cannot run where you are is drawn struck through, and
// Enter on it does nothing.
func TestStruckRowDoesNothing(t *testing.T) {
	m := openedKeybindings(kbModel(t))
	row := rowAt(t, m, "Save chart")
	if m.blocked(row.ID) == "" {
		t.Fatal("fixture: Save chart runs on an opened chart")
	}
	if !m.kbOff(row) {
		t.Error("a command that cannot run is not marked off")
	}
	m = kbType(t, m, "save chart")
	before := m.startup
	m, _ = tpress(t, m, keyEnter)
	if !m.keybindingsOpen || m.startup != before || len(m.recentActions) != 0 {
		t.Errorf("Enter on an off row: open=%v startup %q recent %v, want the popup open and nothing run", m.keybindingsOpen, m.startup, m.recentActions)
	}
	if v := screen(m); strings.Contains(v, "only Now can be saved") {
		t.Errorf("the popup explains the off row, which this piece does not do:\n%s", v)
	}
}

// 27: with colour on the description of a row that cannot run carries the
// strike-through escape, the key keeps its colour, and the row is not dim.
func TestOffRowIsStruckNotDim(t *testing.T) {
	forceColor(t)
	m := kbModel(t)
	m.cfg = Config{Color: ColorAlways}
	m = openedKeybindings(m)
	m = kbType(t, m, "chart")
	var row keys.Row
	for i, r := range m.keybindings.Matches {
		if r.Desc == "Save chart" {
			row = r
			m.keybindings.Cursor = (i + 1) % len(m.keybindings.Matches)
		}
	}
	if !m.kbOff(row) {
		t.Fatal("fixture: Save chart is off here")
	}
	view := m.View()
	if !strings.Contains(view, "\x1b[9mSave chart…\x1b[29m") {
		t.Errorf("the description is not struck through:\n%q", view)
	}
	if strings.Contains(view, "\x1b[2m\x1b[9m") || strings.Contains(view, "\x1b[9m\x1b[2m") {
		t.Error("a struck row is also drawn dim")
	}
}

// With colour off a struck line cannot be seen, so the row says so itself.
func TestOffRowSaysSoWithoutColour(t *testing.T) {
	m := openedKeybindings(kbModel(t))
	m = kbType(t, m, "save chart")
	if !strings.Contains(screen(m), "Save chart…"+kbOffMark) {
		t.Errorf("an off row with colour off carries no mark:\n%s", screen(m))
	}
}

// 9, 31: the commands last chosen lead Global, in their order, with no line
// under them.
func TestRecentCommandsLeadGlobalWithNoLineUnderThem(t *testing.T) {
	m := kbModel(t)
	m = runViaCommands(t, m, "choose zodiac")
	m, _ = tpress(t, m, keyEsc)
	m = openCommandsPopup(t, m)
	var global []keys.Row
	for _, r := range m.keybindings.Matches {
		if r.Group == keys.GroupGlobal {
			global = append(global, r)
		}
	}
	if len(global) < 2 || global[0].Desc != "Choose zodiac" || !global[0].Recent || global[1].Recent {
		t.Fatalf("Global starts %v, want Choose zodiac alone as recent", rowDescs(global[:min(3, len(global))]))
	}
	if m.keybindings.Matches[m.keybindings.Cursor].ID != global[0].ID {
		t.Errorf("`, p` does not put the cursor on the last chosen command")
	}
	lines := m.keybindings.Lines()
	for i, l := range lines {
		if l.Kind == keys.LineRow && m.keybindings.Matches[l.Row].Desc == "Choose zodiac" && lines[i+1].Kind != keys.LineRow {
			t.Errorf("the recent command is followed by %+v, want the next row", lines[i+1])
		}
	}
	for _, l := range strings.Split(screen(m), "\n") {
		if strings.Contains(l, "┈") {
			t.Errorf("a dotted line in the popup: %q", l)
		}
	}
}

// 10: typing filters. Plain text searches the descriptions, a leading `@`
// searches the keys. The headers stay above what matches, and a group with no
// match is not drawn.
func TestFilterByDescriptionAndByKey(t *testing.T) {
	m := openedKeybindings(kbModel(t))
	zod := kbType(t, m, "zod")
	if got := kbTitles(zod); !slices.Equal(got, []string{"Choose zodiac"}) {
		t.Errorf("`zod` keeps %v", got)
	}
	if got := kbHeaders(zod); !slices.Equal(got, []string{"─── Global"}) {
		t.Errorf("`zod` shows headers %v, want only Global", got)
	}
	k := kbType(t, m, "@K")
	if got := kbTitles(k); !slices.Equal(got, []string{"Move chart up"}) {
		t.Errorf("`@K` keeps %v, want Move chart up", got)
	}
	if got := kbHeaders(k); !slices.Equal(got, []string{"─── Local"}) {
		t.Errorf("`@K` shows headers %v, want only Local", got)
	}
	for _, q := range []string{"@<ctrl+z>", "@ctrl+z"} {
		if got := kbTitles(kbType(t, m, q)); !slices.Equal(got, []string{"Choose zodiac"}) {
			t.Errorf("`%s` keeps %v, want Choose zodiac", q, got)
		}
	}
	if got := kbTitles(kbType(t, m, "@enter")); len(got) != 1 || got[0] != m.attachTitle() {
		t.Errorf("`@enter` keeps %v, want %s", got, m.attachTitle())
	}
}

// The filter line is inside the popup, under a rule, and only while a filter is
// typed. Esc clears the filter first, then closes.
func TestFilterLineShowsOnlyWhileTypedAndEscClearsFirst(t *testing.T) {
	m := openedKeybindings(kbModel(t))
	if strings.Contains(screen(m), "Filter (") {
		t.Errorf("the filter line shows with nothing typed:\n%s", screen(m))
	}
	if bar := strings.TrimSpace(lastRow(m.View())); bar != "Run: <enter> | Filter: type | Close: <esc>/?" {
		t.Errorf("bar = %q", bar)
	}
	m = kbType(t, m, "zod")
	view := screen(m)
	if !strings.Contains(view, "Find (start with @ to find by key): zod█") {
		t.Errorf("the filter line is missing:\n%s", view)
	}
	// The rule over it joins both side borders, as lazygit draws it.
	rows := strings.Split(view, "\n")
	for i, r := range rows {
		if strings.Contains(r, "Find (start with @") {
			above := strings.TrimSpace(rows[i-1])
			joined := strings.Contains(above, "├──") && strings.Contains(above, "──┤") || strings.Contains(above, "+--") && strings.Contains(above, "--+")
			if !joined {
				t.Errorf("the rule over the filter line is %q, want it joined to the borders", above)
			}
			break
		}
	}
	if bar := strings.TrimSpace(lastRow(m.View())); bar != "Run: <enter> | Clear: <esc>" {
		t.Errorf("bar while typed = %q", bar)
	}
	m, _ = tpress(t, m, keyEsc)
	if !m.keybindingsOpen || m.keybindings.FilterText != "" || len(m.keybindings.Matches) != len(m.keybindings.All) {
		t.Fatalf("Esc with a filter typed: open=%v filter %q rows %d of %d", m.keybindingsOpen, m.keybindings.FilterText, len(m.keybindings.Matches), len(m.keybindings.All))
	}
	m, _ = tpress(t, m, keyEsc)
	if m.keybindingsOpen {
		t.Error("the second Esc did not close")
	}
}

// `?` is text while a filter is typed.
func TestQuestionMarkIsTextWhileFiltering(t *testing.T) {
	m := kbType(t, openedKeybindings(kbModel(t)), "a")
	m, _ = tpress(t, m, runes("?"))
	if !m.keybindingsOpen || m.keybindings.FilterText != "a?" {
		t.Errorf("open=%v filter %q, want the question mark typed", m.keybindingsOpen, m.keybindings.FilterText)
	}
}

// 11: inside a chooser, `?` opens Keybindings over it. Local is the chooser's
// keys, the commands are struck through, and Esc returns to the chooser as it was.
func TestInsideAChooserTheCommandsAreOffAndEscReturns(t *testing.T) {
	m := openFinder(t, finderModel(t, 2, 0))
	chooser := m.houseFinder
	m, _ = tpress(t, m, runes("?"))
	if !m.keybindingsOpen || m.keybindings.Origin != ownerHouses {
		t.Fatalf("open=%v origin %v", m.keybindingsOpen, m.keybindings.Origin)
	}
	for _, r := range m.keybindings.All {
		faint := m.kbOff(r)
		switch {
		case r.Group == keys.GroupGlobal && r.ID != keys.BindingForceQuit && !faint:
			t.Errorf("the command %q is not off inside a chooser", r.Desc)
		case r.Group == keys.GroupLocal && faint && r.ID != keys.BindingListUp:
			t.Errorf("the chooser's own key %q is off", r.Desc)
		}
	}
	faintZodiac := rowAt(t, m, "Choose zodiac")
	if !m.kbOff(faintZodiac) {
		t.Fatal("Choose zodiac is not off")
	}
	m = kbType(t, m, "choose zodiac")
	m, _ = tpress(t, m, keyEnter)
	if !m.keybindingsOpen || m.startup != startupHouseOpen {
		t.Errorf("Enter on an off command: open=%v startup %q", m.keybindingsOpen, m.startup)
	}
	m, _ = tpress(t, m, keyEsc)
	m, _ = tpress(t, m, keyEsc)
	if m.keybindingsOpen || m.startup != startupHouseOpen || m.currentOwner() != ownerHouses {
		t.Fatalf("Esc did not return to the chooser: open=%v startup %q owner %v", m.keybindingsOpen, m.startup, m.currentOwner())
	}
	if m.houseFinder.FilterText != chooser.FilterText || m.houseFinder.Cursor != chooser.Cursor {
		t.Errorf("the chooser changed under the popup: %+v then %+v", chooser.FilterText, m.houseFinder.FilterText)
	}
}

// `?` is reserved in a list popup, the City list included, and stays text in a
// true form field.
func TestQuestionMarkInPopups(t *testing.T) {
	for _, tc := range []struct {
		name    string
		startup startupState
		opens   bool
	}{
		{"City", startupAtlasOpen, true},
		{"Open chart", startupChartOpen, true},
		{"Sessions", startupSessionsOpen, true},
		{"chart title", startupTitleOpen, false},
		{"save chart", startupSaveOpen, false},
		{"session name", startupSessionNameOpen, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := kbModel(t)
			m.startup = tc.startup
			got, _ := m.dispatchKey(runes("?"))
			if gotOpen := got.(Model).keybindingsOpen; gotOpen != tc.opens {
				t.Errorf("`?` in %s: Keybindings open=%v, want %v", tc.name, gotOpen, tc.opens)
			}
		})
	}
}

// 12: the resting key bar pins `Commands: ,`, `Keybindings: ?` and `Quit: q`
// at its right end, at 80 and 132 columns.
func TestBarPinsCommandsKeybindingsAndQuit(t *testing.T) {
	for _, w := range []int{80, 132} {
		m := kbModel(t)
		m.width, m.height = w, 40
		bar := strings.TrimSpace(lastRow(m.View()))
		if !strings.HasSuffix(bar, "Commands: , | Keybindings: ? | Quit: q") {
			t.Errorf("%d columns: the bar ends %q", w, bar)
		}
		if strings.Contains(bar, "Help") {
			t.Errorf("%d columns: the bar says Help: %q", w, bar)
		}
	}
}

// Every key name a table carries presses back as itself, so Enter on a row
// presses the key the row names.
func TestPressedKeyRoundTrips(t *testing.T) {
	m := kbModel(t)
	for o, tables := range keyRegistry(m.keys).Owners {
		for _, table := range tables {
			for _, e := range table {
				for _, k := range e.Binding.Keys() {
					if got := pressedKey(k).String(); got != k {
						t.Errorf("%s: key %q presses as %q", keys.OwnerName(o), k, got)
					}
				}
			}
		}
	}
}

// With box-drawing borders the rule over the filter line is solid and joined
// to both sides, as lazygit draws it: `├───┤`, not the panels' dotted rule.
func TestFilterRuleIsSolidAsLazygits(t *testing.T) {
	m := paletteModel(t)
	m.cfg = Config{Color: ColorNever}
	m.focused = PanelChart
	m = kbType(t, openedKeybindings(m), "zod")
	rows := strings.Split(screen(m), "\n")
	for i, r := range rows {
		if strings.Contains(r, "Find (start with @") {
			if above := rows[i-1]; !strings.Contains(above, "├───") || !strings.Contains(above, "───┤") || strings.Contains(above[strings.Index(above, "├"):], "┈") {
				t.Errorf("the rule over the filter line is %q", above)
			}
			return
		}
	}
	t.Fatalf("no filter line:\n%s", screen(m))
}
