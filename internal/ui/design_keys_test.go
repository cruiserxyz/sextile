package ui

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// the specification: the key bar, and the keys that mean one
// thing everywhere. Each owner of the keyboard is reached the way the operator
// reaches it, from escBase (esc_restore_test.go), and read back from View().

// keysPlaces is a small atlas, so the city picker has a list to move in.
func keysPlaces() AtlasLoadedMsg {
	return AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{
		{Name: "Aachen", Country: "DE", Lat: 50.77, Lon: 6.08, TZ: "Europe/Berlin"},
		{Name: "Amsterdam", Country: "NL", Lat: 52.37, Lon: 4.90, TZ: "Europe/Amsterdam"},
		{Name: "Athens", Country: "GR", Lat: 37.98, Lon: 23.73, TZ: "Europe/Athens"},
	})}
}

// keysUntitled is a library row whose pick asks for a title.
func keysUntitled() store.NamedChart {
	return store.NamedChart{
		ID: "keys-untitled", Name: "Keys untitled", Untitled: true, Year: 1990,
		Place:  location.PlaceInfo{Name: "Berlin", Country: "DE", Lat: 52.5244, Lon: 13.4105, TZ: "Europe/Berlin"},
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "keys-untitled.as"},
	}
}

// keyOwnerCase is one owner of the keyboard and how to reach it.
type keyOwnerCase struct {
	name  string
	owner owner
	title string // a popup's frame title; empty for the others
	open  func(t *testing.T, m Model) Model
}

func keyOwnerCases() []keyOwnerCase {
	return []keyOwnerCase{
		{"main", ownerMain, "", func(t *testing.T, m Model) Model { return m }},
		{"city", ownerCity, popupTitleCity, func(t *testing.T, m Model) Model {
			m, _ = escSend(t, m, keysPlaces(), escKey(tea.KeyCtrlL))
			return m
		}},
		{"open chart", ownerOpenChart, popupTitleOpenChart, func(t *testing.T, m Model) Model {
			m, _ = escSend(t, m, escKey(tea.KeyCtrlO))
			m, _ = escSend(t, m, ChartsLoadedMsg{Charts: listRowsCharts(), Gen: m.pickerGen})
			return m
		}},
		{"chart title", ownerChartTitle, popupTitleChartTitle, func(t *testing.T, m Model) Model {
			m, _ = escSend(t, m, escKey(tea.KeyCtrlO))
			m, _ = escSend(t, m, ChartsLoadedMsg{Charts: listRowsCharts(), Gen: m.pickerGen}, ChartPickedMsg{Chart: keysUntitled()})
			return m
		}},
		{"save chart", ownerSaveChart, popupTitleSaveChart, func(t *testing.T, m Model) Model {
			m.focused = PanelChart
			m.cursorIdx = len(m.slots.All()) - 1
			m, _ = escSend(t, m, escKey(tea.KeyCtrlS))
			return m
		}},
		{"bodies", ownerBodies, popupTitleBodies, func(t *testing.T, m Model) Model {
			m, _ = escSend(t, m, escKey(tea.KeyCtrlB))
			return m
		}},
		{"aspects", ownerAspects, popupTitleAspects, func(t *testing.T, m Model) Model {
			m, _ = escSend(t, m, escKey(tea.KeyCtrlA))
			return m
		}},
		{"help", ownerHelp, popupTitleKeys, func(t *testing.T, m Model) Model {
			m, _ = escSend(t, m, escRunes("?"))
			return m
		}},
		{"console", ownerConsole, "", func(t *testing.T, m Model) Model {
			m = m.logCommands([]string{"astrolog -one", "astrolog -two", "astrolog -three"})
			m, _ = escSend(t, m, escRunes(","), escRunes("l"))
			return m
		}},
		{"details", ownerDetails, "", func(t *testing.T, m Model) Model {
			m, _ = escSend(t, m, escKey(tea.KeyEnter))
			return m
		}},
		{"house display", ownerDisplay, popupTitleDisplay, func(t *testing.T, m Model) Model {
			m, _ = escSend(t, m, escRunes(","), escRunes("a"))
			return m
		}},
	}
}

// reachOwner opens c from escBase at w×h under cfg and checks it got there.
func reachOwner(t *testing.T, c keyOwnerCase, cfg Config, w, h int) Model {
	t.Helper()
	m := escBase(t)
	m.cfg = cfg
	m.width, m.height = w, h
	m = c.open(t, m)
	if got := m.currentOwner(); got != c.owner {
		t.Fatalf("%s: the keyboard is with owner %d, want %d", c.name, got, c.owner)
	}
	return m
}

// lastRow is the frame's bottom row with the escapes stripped and the padding
// trimmed.
func lastRow(view string) string {
	lines := strings.Split(view, "\n")
	return strings.TrimRight(ansi.Strip(lines[len(lines)-1]), " ")
}

// barPairs is a bar's text split into its pairs.
func barPairs(row string) []string {
	return strings.Split(strings.TrimPrefix(row, " "), keyBarSep)
}

// checkBarShape is C3 and C4 read off the bar: every pair is one of the
// owner's bar entries, in table order, every pinned entry is there, and if
// anything is missing it is a run of unpinned entries from the end, with one
// `…` where they were.
func checkBarShape(t *testing.T, where string, row string, entries []BindingEntry, width int) {
	t.Helper()
	var bar []BindingEntry
	for _, e := range entries {
		if e.Bar {
			bar = append(bar, e)
		}
	}
	pairs := barPairs(row)
	i, more := 0, 0
	for _, p := range pairs {
		if p == "…" || p == "..." {
			more++
			// Everything unpinned from here to the next pinned entry is gone.
			for i < len(bar) && !bar[i].Pinned {
				i++
			}
			continue
		}
		for i < len(bar) && bar[i].Action+": "+bar[i].BarKey != p {
			if bar[i].Pinned || more == 0 {
				t.Fatalf("%s: the bar %q skips %q", where, row, bar[i].Action+": "+bar[i].BarKey)
			}
			i++
		}
		if i == len(bar) {
			t.Fatalf("%s: the bar %q carries %q, which is not a bar entry in table order", where, row, p)
		}
		i++
	}
	for ; i < len(bar); i++ {
		if bar[i].Pinned || more == 0 {
			t.Fatalf("%s: the bar %q is missing %q", where, row, bar[i].Action+": "+bar[i].BarKey)
		}
	}
	if more > 1 {
		t.Fatalf("%s: the bar %q has %d `…`, want at most one", where, row, more)
	}
	// Something left out must have had no room: the first pair dropped,
	// put back beside the `…`, would not fit.
	if more == 1 {
		shown := map[string]bool{}
		for _, p := range pairs {
			shown[p] = true
		}
		var dropped []string
		for _, e := range bar {
			if p := e.Action + ": " + e.BarKey; !shown[p] {
				dropped = append(dropped, p)
			}
		}
		// Putting the first one back: in place of the `…` if it was the only
		// one left out, beside it otherwise.
		back := ansi.StringWidth(row) + len(keyBarSep) + ansi.StringWidth(dropped[0])
		if len(dropped) == 1 {
			back = ansi.StringWidth(row) - ansi.StringWidth("…") + ansi.StringWidth(dropped[0])
		}
		if back <= width {
			t.Fatalf("%s: the bar %q left out %q, which fits in %d columns", where, row, dropped[0], width)
		}
	}
}

// fullBarModel is the main screen where every bar entry acts: the floating
// slot of a pair focused in the Chart panel. Elsewhere Select, Step day and
// Move are not shown (the specification).
func fullBarModel(t *testing.T) Model {
	t.Helper()
	m := escBase(t)
	m.focused = PanelChart
	m.cursorIdx = len(m.slots.All()) - 1
	return m
}

// C3, C4: the bottom row is the owner's table.
func TestKeyBarIsTheOwnersBindings(t *testing.T) {
	for _, c := range keyOwnerCases() {
		for _, size := range [][2]int{{80, 24}, {144, 40}} {
			if c.owner == ownerDetails && size[0] < 144 {
				continue // Details exists only in the dual column
			}
			t.Run(fmt.Sprintf("%s/%dx%d", c.name, size[0], size[1]), func(t *testing.T) {
				m := reachOwner(t, c, Config{Accessible: true}, size[0], size[1])
				entries := m.keyBarEntries()
				row := lastRow(m.View())
				checkBarShape(t, c.name, row, entries, size[0])
				// A popup with nothing to act on yet (no city typed, no name)
				// shows only its way out (the specification B3).
				if !strings.HasPrefix(row, " ") || len(barPairs(row)) < 1 || !strings.Contains(row, "esc") && c.owner != ownerMain {
					t.Fatalf("the bar is %q", row)
				}
			})
		}
	}

	// Every cell of the bar is the muted colour while colours are on.
	t.Run("muted", func(t *testing.T) {
		forceColor(t)
		for _, c := range keyOwnerCases() {
			m := reachOwner(t, c, Config{Color: ColorAlways}, 144, 40)
			lines := strings.Split(m.View(), "\n")
			cells := parseDesignScreen(lines[len(lines)-1])[0]
			text := 0
			for _, cell := range cells {
				if cell.r == ' ' {
					continue
				}
				text++
				if cell.fg != "38;5;244" {
					t.Fatalf("%s: bar cell %q is %q, not the muted colour", c.name, cell.r, cell.fg)
				}
			}
			if text == 0 {
				t.Fatalf("%s: the bar is empty", c.name)
			}
		}
	})

	// C4: narrow bars keep the pinned entries and show where pairs went.
	t.Run("narrow", func(t *testing.T) {
		main := fullBarModel(t).available(defaultKeys().mainBindings())
		row := strings.TrimRight(ansi.Strip(renderKeyBar(60, main, Config{}, "")), " ")
		if !strings.HasSuffix(row, " | … | Help: ? | Quit: q") {
			t.Errorf("the 60-column main bar is %q, want it to end `… | Help: ? | Quit: q`", row)
		}
		checkBarShape(t, "main at 60", row, main, 60)
		for _, c := range keyOwnerCases()[1:] {
			entries := escBase(t).ownerBindings(c.owner)
			row := strings.TrimRight(ansi.Strip(renderKeyBar(24, entries, Config{}, "")), " ")
			if !strings.Contains(row, "esc") {
				t.Errorf("%s at 24 columns dropped its Esc entry: %q", c.name, row)
			}
		}
	})

	// Failure mode: a leader rebound onto a main-table key shadows it, and the
	// bar still shows the main table.
	t.Run("leader on ?", func(t *testing.T) {
		m := escBase(t)
		m = m.applyKeyConfig(store.KeysConfig{Leader: "?", LeaderPresent: true})
		row := lastRow(m.View())
		checkBarShape(t, "main with leader ?", row, m.keyBarEntries(), m.width)
		if !strings.Contains(row, "Help: ?") {
			t.Errorf("the bar %q stopped showing the main table's Help entry", row)
		}
	})
}

// popupFrame finds the popup whose title starts with title and returns its
// whole title and the rows inside its frame, escapes stripped, without the
// side borders.
func popupFrame(t *testing.T, view, title string) (string, []string) {
	t.Helper()
	lines := strings.Split(ansi.Strip(view), "\n")
	top, col := -1, -1
	for i, l := range lines {
		r := []rune(l)
		for j := 0; j+2 < len(r); j++ {
			if string(r[j:j+2]) == "╭─" && strings.HasPrefix(string(r[j+2:]), focusMarkUnicode+title) {
				top, col = i, j
				break
			}
		}
		if top >= 0 {
			break
		}
	}
	if top < 0 {
		t.Fatalf("no popup titled %q on screen:\n%s", title, ansi.Strip(view))
	}
	r := []rune(lines[top])
	whole, _, _ := strings.Cut(string(r[col+2:]), "─")
	whole = strings.TrimPrefix(whole, focusMarkUnicode)
	right := col + 1
	for right < len(r) && r[right] != '╮' {
		right++
	}
	var rows []string
	for _, l := range lines[top+1:] {
		r := []rune(l)
		if r[col] == '╰' {
			return whole, rows
		}
		rows = append(rows, string(r[col+1:right]))
	}
	t.Fatalf("popup %q has no bottom edge", title)
	return "", nil
}

func popupInterior(t *testing.T, view, title string) []string {
	t.Helper()
	_, rows := popupFrame(t, view, title)
	return rows
}

// hintShape matches a key hint: `[…: …]`.
var hintShape = regexp.MustCompile(`\[[^\]]*:[^\]]*\]`)

// C5: no popup draws a hint or says what it is doing, inside its frame or in
// its title (operator, 2026-10-03): it shows only what it is for, and a
// failure that matters is in the Log.
func TestPopupsCarryNoHints(t *testing.T) {
	cfg := Config{Color: ColorNever}
	var pairs []string
	for _, c := range keyOwnerCases() {
		for _, e := range escBase(t).ownerBindings(c.owner) {
			if e.Action != "" {
				pairs = append(pairs, strings.ToLower(e.Action+": "+e.BarKey), strings.ToLower(e.BarKey+": "+e.Action))
			}
		}
	}
	// A state is reached through reach; work, if set, then puts the popup
	// mid-work. A working popup must look exactly like the same popup idle:
	// the same title and the same rows, so no status can hide anywhere in it.
	type state struct {
		name  string
		reach func(t *testing.T, m Model) Model
		work  func(m Model) Model
	}
	typed := func(s string) func(t *testing.T, m Model) Model {
		return func(t *testing.T, m Model) Model { m, _ = escSend(t, m, escRunes(s)); return m }
	}
	byOwner := map[owner][]state{
		ownerCity: {{"rest", nil, nil}, {"typed", typed("a"), nil}},
		ownerOpenChart: {
			{"rest", nil, nil},
			{"typed", typed("o"), nil},
			{"reading", nil, func(m Model) Model {
				m.picker.resolving, m.picker.pending = true, listRowsCharts()[0]
				return m
			}},
			{"failed", nil, func(m Model) Model {
				m.picker.resolveErr, m.picker.pending = os.ErrNotExist, listRowsCharts()[0]
				return m
			}},
			{"refused", nil, func(m Model) Model {
				m.picker.refusal, m.picker.pending = "no place", listRowsCharts()[0]
				return m
			}},
		},
		ownerChartTitle: {
			{"rest", nil, nil},
			{"typed", typed("Anna"), nil},
			{"saving", typed("Anna"), func(m Model) Model { m.title.saving = true; return m }},
		},
		ownerSaveChart: {
			{"rest", nil, nil},
			{"typed", typed("x"), nil},
			{"saving", typed("x"), func(m Model) Model { m.save.saving = true; return m }},
		},
		ownerBodies: {
			{"rest", nil, nil},
			{"typed", typed("s"), nil},
			{"toggled", func(t *testing.T, m Model) Model { m, _ = escSend(t, m, escKey(tea.KeySpace)); return m }, nil},
		},
		ownerAspects: {
			{"rest", nil, nil},
			{"typed", typed("s"), nil},
			{"toggled", func(t *testing.T, m Model) Model { m, _ = escSend(t, m, escKey(tea.KeySpace)); return m }, nil},
		},
		ownerHelp: {
			{"rest", nil, nil},
			{"moved", func(t *testing.T, m Model) Model { m, _ = escSend(t, m, escKey(tea.KeyDown)); return m }, nil},
		},
	}
	popups := 0
	for _, c := range keyOwnerCases() {
		states, ok := byOwner[c.owner]
		if !ok {
			continue
		}
		popups++
		for _, st := range states {
			t.Run(c.name+"/"+st.name, func(t *testing.T) {
				m := reachOwner(t, c, cfg, 144, 40)
				if st.reach != nil {
					m = st.reach(t, m)
				}
				title, rows := popupFrame(t, m.View(), c.title)
				if st.work != nil {
					wt, wrows := popupFrame(t, st.work(m).View(), c.title)
					if wt != title || strings.Join(wrows, "\n") != strings.Join(rows, "\n") {
						t.Errorf("working, the popup is not the popup idle:\n%s\n%s\nidle:\n%s\n%s", wt, strings.Join(wrows, "\n"), title, strings.Join(rows, "\n"))
					}
				}
				if title != c.title {
					t.Errorf("the title is %q, want %q alone", title, c.title)
				}
				// The padding rows are empty, and no row inside carries a
				// hint, a key pair or a status.
				if strings.TrimSpace(rows[0]) != "" || strings.TrimSpace(rows[len(rows)-1]) != "" {
					t.Errorf("the frame's padding rows are not empty: %q, %q", rows[0], rows[len(rows)-1])
				}
				// The two sentences the 2026-10-21 answers allow: the empty
				// city chooser's typing line, and the Bodies count and legend.
				// Each appears in its own popup and nowhere else.
				joined := strings.Join(rows, "\n")
				if got, want := strings.Contains(joined, atlasTypeHint), c.owner == ownerCity && st.name == "rest"; got != want {
					t.Errorf("the city typing line present = %v, want %v:\n%s", got, want, joined)
				}
				if got, want := strings.Contains(joined, " selected · ● all ◐ some ○ none"), c.owner == ownerBodies; got != want {
					t.Errorf("the Bodies legend present = %v, want %v:\n%s", got, want, joined)
				}
				for _, r := range rows {
					if hintShape.MatchString(r) {
						t.Errorf("the popup draws a hint: %q", strings.TrimSpace(r))
					}
					for _, p := range pairs {
						if strings.Contains(strings.ToLower(r), p) {
							t.Errorf("the popup draws the key pair %q: %q", p, strings.TrimSpace(r))
						}
					}
					for _, s := range []string{"Saving", "Reading ", "Could not read", "Can't anchor"} {
						if strings.Contains(r, s) {
							t.Errorf("the popup says what it is doing inside its frame: %q", strings.TrimSpace(r))
						}
					}
				}
			})
		}
	}
	if popups != 7 {
		t.Fatalf("checked %d popups, want 7", popups)
	}
}

// keyTokenRE matches a key spelled in a message (C6's token set).
var keyTokenRE = regexp.MustCompile(`(^|[^\pL])(esc|Esc|pgup|pgdown)([^\pL]|$)|ctrl\+|Ctrl\+|⌃|⇧|↑|↓`)

// C6: no message names a key. The four Log hints that did were retired with
// the refused-key notes (the specification Part B); what is left is the
// title form's required-field error, which must not name one either.
func TestKeyMessagesUseTheTable(t *testing.T) {
	t.Run("empty place error", func(t *testing.T) {
		m := reachOwner(t, keyOwnerCases()[3], Config{Color: ColorNever}, 144, 40)
		m, _ = escSend(t, m, escRunes("Anna"), escKey(tea.KeyTab))
		m.title.chart.PlaceLabel = ""
		m.title.form.fields[1].value = ""
		m, _ = escSend(t, m, escKey(tea.KeyEnter))
		view := ansi.Strip(m.View())
		if !strings.Contains(view, emptyPlaceRefusal) {
			t.Fatalf("the error is not on screen:\n%s", view)
		}
		if keyTokenRE.MatchString(emptyPlaceRefusal) {
			t.Errorf("%q names a key", emptyPlaceRefusal)
		}
	})
}

// C9: Esc in the title form goes back to the list; empty Enter opens untitled.
func TestTitleFormEscGoesBack(t *testing.T) {
	dir := titleLibrary(t, []string{"Equinox-2026-09.as"}, nil)
	path := dir + "/Equinox-2026-09.as"
	before := fileText(t, path)

	t.Run("esc", func(t *testing.T) {
		m := openedPicker(t)
		m, _ = escSend(t, m, escRunes("E"))
		filter, cursor, gen := m.picker.filterText, m.picker.cursor, m.pickGen
		m, _ = pickRow(t, m, "Equinox-2026-09.as")
		if m.startup != startupTitleOpen {
			t.Fatal("the pick did not ask for a title")
		}
		m.picker.cursor = cursor // pickRow moved it onto the row; the list is the list
		m, cmd := escSend(t, m, escKey(tea.KeyEsc))
		if cmd != nil {
			t.Error("Esc dispatched a command; it must open nothing")
		}
		if m.startup != startupChartOpen || m.picker.filterText != filter || m.picker.cursor != cursor {
			t.Errorf("Esc left startup %s, filter %q, cursor %d; want the list as it was (%q, %d)", m.startup, m.picker.filterText, m.picker.cursor, filter, cursor)
		}
		if m.picker.resolving || m.pickGen < gen {
			t.Error("Esc resolved the pick")
		}
	})
	t.Run("empty enter", func(t *testing.T) {
		m := openedPicker(t)
		m, _ = pickRow(t, m, "Equinox-2026-09.as")
		m, cmd := escSend(t, m, escKey(tea.KeyEnter))
		if cmd == nil || !m.picker.resolving || m.startup != startupChartOpen {
			t.Fatal("Enter with an empty title did not open the chart untitled")
		}
	})
	t.Run("esc while writing", func(t *testing.T) {
		m := openedPicker(t)
		m, _ = pickRow(t, m, "Equinox-2026-09.as")
		m = typeText(t, m, "Constructed Title")
		m, _ = escSend(t, m, escKey(tea.KeyEnter)) // the second step
		m, write := escSend(t, m, escKey(tea.KeyEnter))
		if write == nil || !m.title.saving {
			t.Fatal("Enter with a title did not start the write")
		}
		m, cmd := escSend(t, m, escKey(tea.KeyEsc))
		if cmd != nil || m.startup != startupChartOpen {
			t.Fatalf("Esc during the write: startup %s, command %v", m.startup, cmd != nil)
		}
		snap := snapshotForEsc(m)
		picker := fmt.Sprintf("%q %d", m.picker.filterText, m.picker.cursor)
		m, _ = escSend(t, m, write())
		requireSameScreen(t, "after the late write", snap, snapshotForEsc(m))
		if got := fmt.Sprintf("%q %d", m.picker.filterText, m.picker.cursor); got != picker || m.picker.resolving {
			t.Errorf("the late write moved the list: %s, resolving %v", got, m.picker.resolving)
		}
		if fileText(t, path) == before {
			t.Error("the write handed to Astrolog did not finish")
		}
	})
}

// C10: Ctrl+C quits from every owner and a pending leader, through the one
// quit path that writes the session.
func TestCtrlCQuitsFromEverywhere(t *testing.T) {
	cases := keyOwnerCases()
	cases = append(cases, keyOwnerCase{"pending leader", ownerMain, "", func(t *testing.T, m Model) Model {
		m, _ = escSend(t, m, escRunes(","))
		if !m.leader.pending {
			t.Fatal("the leader did not arm")
		}
		return m
	}})
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			session := sessionSandbox(t)
			m := reachOwner(t, c, Config{Accessible: true}, 144, 40)
			_, cmd := escSend(t, m, escKey(tea.KeyCtrlC))
			if !quits(cmd) {
				t.Fatal("Ctrl+C did not quit")
			}
			if _, err := os.Stat(session); err != nil {
				t.Errorf("the quit wrote no session: %v", err)
			}
		})
	}
	t.Run("cold-start city picker", func(t *testing.T) {
		sessionSandbox(t)
		path := seedSession(t, "chart:\n  name: Kept\n")
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		m := NewModelWithClock(Config{Accessible: true}, FrozenClock(testFrozenTime()))
		m, _ = escSend(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
		m.startup = startupAtlasOpen
		_, cmd := escSend(t, m, escKey(tea.KeyCtrlC))
		if !quits(cmd) {
			t.Fatal("Ctrl+C did not quit the cold-start picker")
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != string(before) {
			t.Errorf("the session file changed: %q, %v", after, err)
		}
	})
}

// C11: q quits from the main screen and acts as Esc where nothing is typed;
// where a field takes letters it is a letter.
func TestQBacksOutWhereNothingIsTyped(t *testing.T) {
	typed := map[owner]func(m Model) string{
		ownerCity:       func(m Model) string { return m.atlas.filterText },
		ownerOpenChart:  func(m Model) string { return m.picker.filterText },
		ownerChartTitle: func(m Model) string { return m.title.form.value(m.title.form.focus) },
		ownerSaveChart:  func(m Model) string { return m.save.form.value(0) },
		ownerBodies:     func(m Model) string { return m.bodyModal.filterText },
		ownerAspects:    func(m Model) string { return m.aspectModal.filterText },
	}
	for _, c := range keyOwnerCases() {
		t.Run(c.name, func(t *testing.T) {
			m := reachOwner(t, c, Config{Accessible: true}, 144, 40)
			after, cmd := escSend(t, m, escRunes("q"))
			if c.owner == ownerMain {
				if !quits(cmd) {
					t.Fatal("q did not quit the main screen")
				}
				return
			}
			if quits(cmd) {
				t.Fatal("q quit from here")
			}
			switch c.owner {
			case ownerDetails:
				// q leaves Details (time-keys-everywhere rule 4), a second q
				// quits from the main screen (rule 6).
				if after.details.attached {
					t.Fatal("q did not detach Details")
				}
				if _, cmd := escSend(t, after, escRunes("q")); !quits(cmd) {
					t.Fatal("the second q did not quit the main screen")
				}
				return
			case ownerHelp, ownerConsole, ownerDisplay:
				if got := after.currentOwner(); got != ownerMain {
					t.Fatalf("q left the keyboard with owner %d, want the main screen", got)
				}
				viaEsc, _ := escSend(t, m, escKey(tea.KeyEsc))
				if after.View() != viaEsc.View() {
					t.Error("the screen after q is not the screen after Esc")
				}
				return
			}
			field, ok := typed[c.owner]
			if !ok {
				t.Fatalf("owner %d has no row: say whether q is text or closes", c.owner)
			}
			if got := field(after); !strings.HasSuffix(got, "q") {
				t.Errorf("q did not type: the line holds %q", got)
			}
		})
	}
}

// The finders and the Details search lines take letters, so q is a letter
// there too; the too-small screen is the main table's.
func TestQIsALetterInFindersAndSearchLines(t *testing.T) {
	t.Run("house finder", func(t *testing.T) {
		m, _ := escSend(t, escBase(t), escKey(tea.KeyCtrlH))
		if m.currentOwner() != ownerHouses {
			t.Fatalf("owner %d, want the house finder", m.currentOwner())
		}
		m, cmd := escSend(t, m, escRunes("q"))
		if quits(cmd) || m.currentOwner() != ownerHouses || !strings.HasSuffix(m.houseFinder.filterText, "q") {
			t.Errorf("q: owner %d, line %q, quit %v", m.currentOwner(), m.houseFinder.filterText, quits(cmd))
		}
	})
	t.Run("zodiac finder", func(t *testing.T) {
		m, _ := escSend(t, escBase(t), escKey(tea.KeyCtrlZ))
		if m.currentOwner() != ownerZodiac {
			t.Fatalf("owner %d, want the zodiac finder", m.currentOwner())
		}
		m, cmd := escSend(t, m, escRunes("q"))
		if quits(cmd) || m.currentOwner() != ownerZodiac || !strings.HasSuffix(m.zodiacFinder.filterText, "q") {
			t.Errorf("q: owner %d, line %q, quit %v", m.currentOwner(), m.zodiacFinder.filterText, quits(cmd))
		}
	})
	t.Run("timeline line", func(t *testing.T) {
		m := onTimeline(t, Config{Accessible: true})
		m, _ = tpress(t, m, runes("/"))
		m, cmd := tpress(t, m, runes("q"))
		if quits(cmd) || !m.details.attached || !m.timeline.searching || !strings.HasSuffix(m.timeline.query, "q") {
			t.Errorf("q: attached %v, searching %v, query %q, quit %v", m.details.attached, m.timeline.searching, m.timeline.query, quits(cmd))
		}
	})
	t.Run("report line", func(t *testing.T) {
		m := attachedReport(t, false, "[0] Aspects", ownFilterBody(80))
		m, _ = tpress(t, m, runes("/"))
		if !m.detailsReportSearchOpen() {
			t.Fatal("fixture: `/` did not open the report's line")
		}
		m, cmd := tpress(t, m, runes("q"))
		if quits(cmd) || !m.details.attached || !m.details.searching || !strings.HasSuffix(m.details.query, "q") {
			t.Errorf("q: attached %v, searching %v, query %q, quit %v", m.details.attached, m.details.searching, m.details.query, quits(cmd))
		}
	})
	t.Run("too small", func(t *testing.T) {
		m := escBase(t)
		m.width, m.height = 20, 5
		if m.currentOwner() != ownerMain {
			t.Fatalf("owner %d, want the main table", m.currentOwner())
		}
		if _, cmd := escSend(t, m, escRunes("q")); !quits(cmd) {
			t.Error("q did not quit from the too-small screen")
		}
	})
}

// The Close entry of the three non-typing owners carries esc and q, and the
// bar says so.
func TestCloseEntryOfTheNonTypingOwnersIsEscOrQ(t *testing.T) {
	for name, entries := range map[string][]BindingEntry{
		"help":    helpBindings(),
		"console": consoleBindings(),
		"details": defaultKeys().detailsBindings(),
	} {
		var closer *BindingEntry
		for i := range entries {
			if entries[i].ID == BindingClose {
				closer = &entries[i]
			}
		}
		if closer == nil {
			t.Fatalf("%s has no Close entry", name)
		}
		if closer.BarKey != "esc/q" {
			t.Errorf("%s: Close shows %q, want esc/q", name, closer.BarKey)
		}
		keys := closer.Binding.Keys()
		hasEsc, hasQ := false, false
		for _, k := range keys {
			hasEsc = hasEsc || k == "esc"
			hasQ = hasQ || k == "q"
		}
		if !hasEsc || !hasQ {
			t.Errorf("%s: Close keys are %v, want esc and q", name, keys)
		}
	}
}

// counterRE matches `n of m` as a whole counter.
func counterAt(view string, n, total int) bool {
	return regexp.MustCompile(fmt.Sprintf(`(^|[^0-9])%d of %d([^0-9]|$)`, n, total)).MatchString(ansi.Strip(view))
}

// C12: lists stop at their ends.
func TestListsStopAtTheEnds(t *testing.T) {
	type list struct {
		name  string
		reach func(t *testing.T) Model
		total func(m Model) int
		up    tea.KeyMsg
		down  tea.KeyMsg
	}
	byName := map[string]keyOwnerCase{}
	for _, c := range keyOwnerCases() {
		byName[c.name] = c
	}
	popup := func(name string, typed string) func(t *testing.T) Model {
		return func(t *testing.T) Model {
			m := reachOwner(t, byName[name], Config{Accessible: true}, 144, 40)
			if typed != "" {
				m, _ = escSend(t, m, escRunes(typed))
			}
			return m
		}
	}
	up, down := escKey(tea.KeyUp), escKey(tea.KeyDown)
	lists := []list{
		{"chart slots", func(t *testing.T) Model { m := escBase(t); m.focused = PanelChart; return m },
			func(m Model) int { return len(m.slots.All()) }, up, down},
		{"bodies", func(t *testing.T) Model { return escBase(t) },
			func(m Model) int { return len(m.chart.positions) }, up, down},
		{"console", popup("console", ""), func(m Model) int { return len(m.console.snapshot) }, up, down},
		{"timeline", func(t *testing.T) Model {
			stubTextCompute(t)
			stubRawReports(t)
			return onTimeline(t, Config{Accessible: true})
		}, func(m Model) int { return len(m.timelineRows()) }, up, down},
		{"city popup", popup("city", "a"), func(m Model) int { return len(m.atlas.matches) }, up, down},
		{"open chart popup", popup("open chart", ""), func(m Model) int { return len(m.picker.matches) }, up, down},
		{"bodies popup", popup("bodies", ""), func(m Model) int { return m.bodyModal.totalRows() }, up, down},
		{"aspects popup", popup("aspects", ""), func(m Model) int { return m.aspectModal.totalRows() }, up, down},
	}
	for _, l := range lists {
		t.Run(l.name, func(t *testing.T) {
			m := l.reach(t)
			n := l.total(m)
			if n < 2 {
				t.Fatalf("the list has %d rows; it needs two to have two ends", n)
			}
			for i := 0; i <= n; i++ {
				m, _ = escSend(t, m, l.up)
			}
			if !counterAt(m.View(), 1, n) {
				t.Fatalf("at the top the counter is not `1 of %d`", n)
			}
			m, _ = escSend(t, m, l.up)
			if !counterAt(m.View(), 1, n) {
				t.Errorf("↑ at the first row moved: the counter is not `1 of %d`", n)
			}
			for i := 0; i <= n; i++ {
				m, _ = escSend(t, m, l.down)
			}
			if !counterAt(m.View(), n, n) {
				t.Fatalf("at the bottom the counter is not `%d of %d`", n, n)
			}
			m, _ = escSend(t, m, l.down)
			if !counterAt(m.View(), n, n) {
				t.Errorf("↓ at the last row moved: the counter is not `%d of %d`", n, n)
			}
		})
	}
}

// C13: a click does not move focus while something other than the main screen
// has the keyboard.
func TestMouseIgnoredWhileOwned(t *testing.T) {
	for _, c := range keyOwnerCases()[1:] {
		t.Run(c.name, func(t *testing.T) {
			m := reachOwner(t, c, Config{Color: ColorNever}, 144, 40)
			had := m.focused
			titles := map[PanelID]string{PanelChart: "[1] Chart", PanelBodies: "[2] Bodies"}
			other := PanelChart
			if had == PanelChart {
				other = PanelBodies
			}
			r := m.geometry()[other]
			m, _ = escSend(t, m, tea.MouseMsg{X: r.x + 2, Y: r.y + 1, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
			if m.focused != had {
				t.Fatalf("a click moved focus from %v to %v", had, m.focused)
			}
			// The chart title popup goes back to the list it came from; a
			// second Esc closes that.
			for i := 0; i < 2 && m.currentOwner() != ownerMain; i++ {
				m, _ = escSend(t, m, escKey(tea.KeyEsc))
			}
			if m.currentOwner() != ownerMain {
				t.Fatal("Esc did not close the owner")
			}
			view := ansi.Strip(m.View())
			if !strings.Contains(view, "╭─"+focusMarkUnicode+titles[had]) || strings.Contains(view, "╭─"+focusMarkUnicode+titles[other]) {
				t.Errorf("after Esc the focus frame is not on %s:\n%s", titles[had], view)
			}
		})
	}
	// The control: on the main screen the same click does move focus.
	m := escBase(t)
	r := m.geometry()[PanelChart]
	m, _ = escSend(t, m, tea.MouseMsg{X: r.x + 2, Y: r.y + 1, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if m.focused != PanelChart {
		t.Fatalf("self-check: the click on the main screen did not reach Chart (%v)", m.focused)
	}
}

// C14: keys read at once act as if pressed one by one, through whichever owner
// has the keyboard as each arrives.
func TestBurstReplayedEverywhere(t *testing.T) {
	byName := map[string]keyOwnerCase{}
	for _, c := range keyOwnerCases() {
		byName[c.name] = c
	}
	cases := []struct {
		name  string
		reach func(t *testing.T) Model
		keys  string
	}{
		{"main", func(t *testing.T) Model { m := escBase(t); m.bodyCursorIdx = 0; return m }, "jjjj"},
		{"console", func(t *testing.T) Model { return reachOwner(t, byName["console"], Config{Accessible: true}, 144, 40) }, "kk"},
		{"save popup", func(t *testing.T) Model {
			return reachOwner(t, byName["save chart"], Config{Accessible: true}, 144, 40)
		}, "aqb"},
		{"help across owners", func(t *testing.T) Model { return escBase(t) }, "?jj"},
		// A quit ends the burst and keeps what the keys before it returned.
		{"main up to a quit", func(t *testing.T) Model { m := escBase(t); m.width = 144; m.bodyCursorIdx = 0; return m }, "jq"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start := c.reach(t)
			if c.name == "main up to a quit" {
				// The key before the quit must return a command, or dropping
				// it could not change the count.
				if _, cmd := escSend(t, start, escRunes(c.keys[:1])); cmdCount(cmd) == 0 {
					t.Fatalf("%q alone returns no command; the case proves nothing", c.keys[:1])
				}
			}
			burst, burstCmd := escSend(t, start, escRunes(c.keys))
			slow := start
			var slowCmds []tea.Cmd
			for _, r := range c.keys {
				var cmd tea.Cmd
				slow, cmd = escSend(t, slow, escRunes(string(r)))
				if cmd != nil {
					slowCmds = append(slowCmds, cmd)
				}
			}
			var slowCmd tea.Cmd
			if len(slowCmds) > 0 {
				slowCmd = tea.Batch(slowCmds...)
			}
			if burst.View() != slow.View() {
				t.Errorf("the burst and the separate presses render differently:\n--- burst\n%s\n--- one by one\n%s", ansi.Strip(burst.View()), ansi.Strip(slow.View()))
			}
			want := 0
			for _, c := range slowCmds {
				want += cmdCount(c)
			}
			if got := cmdCount(burstCmd); got != want {
				t.Errorf("the burst returned %d commands, the presses %d", got, want)
			}
			if quits(burstCmd) != quits(slowCmd) {
				t.Errorf("the burst quit %v, the presses %v", quits(burstCmd), quits(slowCmd))
			}
			if burst.View() == start.View() {
				t.Error("the burst changed nothing")
			}
		})
	}
	// A paste is text, never keys: a pasted `q` does not quit and a pasted
	// leader sequence opens nothing (bubbletea spells a paste in brackets).
	for _, paste := range []string{"q", ",l", "jj"} {
		m := escBase(t)
		after, cmd := escSend(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(paste), Paste: true})
		if cmd != nil || after.View() != m.View() {
			t.Errorf("a pasted %q acted on the main screen", paste)
		}
	}

	// The save popup took every key, `q` included.
	m := reachOwner(t, byName["save chart"], Config{Accessible: true}, 144, 40)
	m, _ = escSend(t, m, escRunes("aqb"))
	if got := m.save.form.value(0); got != "aqb" {
		t.Errorf("the save popup holds %q after the burst, want aqb", got)
	}
}

// C15: the Help popup lists the main table then the leader's, in order. It
// has no cursor, so the whole list is read by scrolling a line at a time.
func TestHelpModalMatchesBindingTable(t *testing.T) {
	m := escBase(t)
	m.cfg = Config{Color: ColorNever}
	keys := m.keys
	var want []string
	for _, s := range helpSections(keys.mainBindings()) {
		for _, e := range s.entries {
			if helpUnlisted[e.ID] {
				continue
			}
			// The list keys are listed once the workspace is a list (HelpWhen); this
			// one-chart workspace is not.
			if e.Help && (e.HelpWhen == nil || e.HelpWhen(m)) {
				h := e.Binding.Help()
				want = append(want, h.Desc+" "+h.Key)
			}
		}
	}
	for _, b := range m.leaderBindings {
		want = append(want, bindingDesc(keys, b.ID)+" "+m.leaderKey+showSeq(b.Seq))
	}
	var got []string
	for _, l := range m.helpLines() {
		if l.binding {
			got = append(got, strings.Join(strings.Fields(l.text), " "))
		}
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the Help popup lists\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, row := range got {
		if strings.Contains(row, "reserved") {
			t.Errorf("a reserved entry is listed: %q", row)
		}
	}
}

// C2's behaviour half: every entry of every owner's table, pressed where it is
// available, does something — changes the model or returns a command. ↑ at a
// list's first row and ↓ at its last are C12's, so each entry is tried from
// several states and passes if any of them shows its effect.
func TestEveryOwnerEntryActs(t *testing.T) {
	old := yankLine
	yankLine = func(string) error { return nil }
	t.Cleanup(func() { yankLine = old })
	stubTextCompute(t)
	stubRawReports(t)

	changed := func(before, after Model, cmd tea.Cmd) bool {
		return cmd != nil || before.View() != after.View() || fmt.Sprintf("%+v", before) != fmt.Sprintf("%+v", after)
	}
	press := func(m Model, k string) Model { m, _ = escSend(t, m, keyMsgFor(t, k)); return m }

	for _, c := range keyOwnerCases() {
		t.Run(c.name, func(t *testing.T) {
			var states []Model
			switch c.owner {
			case ownerMain:
				states = hotkeyStates(t)
				for _, s := range hotkeyStates(t)[:1] {
					s.cursorIdx = 0
					states = append(states, s, press(s, ","))
				}
			case ownerDetails:
				// N goes back only once n has jumped; with nothing to go back
				// to it is silent (the specification Part B).
				timeline := onTimeline(t, Config{Accessible: true})
				states = []Model{reachOwner(t, c, Config{Accessible: true}, 144, 40), timeline, press(timeline, "n")}
			case ownerChartTitle:
				base := reachOwner(t, c, Config{Accessible: true}, 144, 40)
				states = []Model{base, press(base, "tab")}
			case ownerSaveChart:
				base := reachOwner(t, c, Config{Accessible: true}, 144, 40)
				states = []Model{base, press(base, "x")}
			case ownerCity:
				base := reachOwner(t, c, Config{Accessible: true}, 144, 40)
				states = []Model{press(base, "a")}
			default:
				states = []Model{reachOwner(t, c, Config{Accessible: true}, 144, 40)}
			}
			// Each state again with its cursor or window moved off either
			// end, since a list's ends are C12's to keep still.
			for _, s := range append([]Model(nil), states...) {
				// ⌃⇟ is the key that scrolls Details now; ⇟ pages the list's cursor.
				states = append(states, press(press(s, "down"), "down"), press(press(s, "up"), "up"), press(s, "pgdown"), press(s, "ctrl+pgdown"))
			}
			for _, e := range escBase(t).ownerBindings(c.owner) {
				// An entry with no key of its own is a leader sequence shown
				// in the bar (Details' `,y` copy): it is driven the way the
				// operator types it, leader first.
				keys := e.Binding.Keys()
				k := ""
				if len(keys) > 0 {
					k = keys[0]
				} else if e.ID == BindingCopy {
					k = "y"
				} else {
					t.Fatalf("%s: entry %d has no key and is no known leader sequence", c.name, e.ID)
				}
				acted := false
				for _, s := range states {
					if e.When != nil && !e.When(s) {
						continue
					}
					if len(keys) == 0 {
						s = press(s, ",")
					}
					tm, cmd := s.Update(keyMsgFor(t, k))
					if changed(s, tm.(Model), cmd) {
						acted = true
						break
					}
				}
				if !acted {
					t.Errorf("%s: entry %d (%q) did nothing in any state where it is available", c.name, e.ID, k)
				}
			}
		})
	}
}

// The Help popup's first page: the "Start here" group with its rows at both
// sizes, no Limited view line, and neither Help itself nor the main Quit.
func TestHelpFirstPageAt80x24(t *testing.T) {
	page := func(w, h int) string {
		m, _ := escSend(t, helpFixture(t, w, h), escRunes("?"))
		var got []string
		for _, r := range popupInterior(t, m.View(), popupTitleKeys) {
			if s := strings.TrimSpace(r); s != "" {
				got = append(got, strings.Join(strings.Fields(s), " "))
			}
		}
		return strings.Join(got, "\n")
	}
	rows := []string{"Next panel tab/→", "Previous row ↑/k", "Next row ↓/j", "Into Details enter", "Open chart ⌃o", "Choose city ⌃l"}
	for _, size := range [][2]int{{80, 24}, {144, 40}} {
		p := page(size[0], size[1])
		for _, want := range append([]string{"Start here"}, rows...) {
			if !strings.Contains(p, want) {
				t.Errorf("%dx%d first page lacks %q:\n%s", size[0], size[1], want, p)
			}
		}
		for _, bad := range []string{"Limited view", "Keys ?", "Quit q", "Esc"} {
			if strings.Contains(p, bad) {
				t.Errorf("%dx%d first page carries %q:\n%s", size[0], size[1], bad, p)
			}
		}
	}
	if w := page(144, 40); !strings.Contains(w, "Quit from anywhere ⌃c") {
		t.Errorf("144x40 first page lacks the Quit from anywhere row:\n%s", w)
	}
}

// Help has no cursor: PgDn moves the first visible line by one list height,
// clamped at the end, and PgUp moves back.
func TestHelpPagesWithPgDn(t *testing.T) {
	m, _ := escSend(t, helpFixture(t, 80, 24), escRunes("?"))
	h := helpListHeight(24)
	total := len(m.helpLines())
	if total <= h {
		t.Fatalf("the list (%d lines) fits one page of %d; nothing to page", total, h)
	}
	m, _ = escSend(t, m, escKey(tea.KeyPgDown))
	if m.help.offset != h {
		t.Fatalf("PgDn: offset %d, want %d", m.help.offset, h)
	}
	for i := 0; i < total; i++ {
		m, _ = escSend(t, m, escKey(tea.KeyPgDown))
	}
	if want := total - h; m.help.offset != want {
		t.Fatalf("PgDn past the end: offset %d, want %d", m.help.offset, want)
	}
	m, _ = escSend(t, m, escKey(tea.KeyPgUp))
	if want := total - 2*h; m.help.offset != max(want, 0) {
		t.Fatalf("PgUp: offset %d, want %d", m.help.offset, max(want, 0))
	}
	for i := 0; i < total; i++ {
		m, _ = escSend(t, m, escKey(tea.KeyPgUp))
	}
	if m.help.offset != 0 {
		t.Fatalf("PgUp past the top: offset %d, want 0", m.help.offset)
	}
}

// ↓ and ↑ scroll the Help window one line.
func TestHelpArrowsScrollOneLine(t *testing.T) {
	m, _ := escSend(t, helpFixture(t, 80, 24), escRunes("?"))
	m, _ = escSend(t, m, escKey(tea.KeyDown))
	if m.help.offset != 1 {
		t.Fatalf("↓: offset %d, want 1", m.help.offset)
	}
	m, _ = escSend(t, m, escKey(tea.KeyUp))
	m, _ = escSend(t, m, escKey(tea.KeyUp))
	if m.help.offset != 0 {
		t.Fatalf("↑ at the top: offset %d, want 0", m.help.offset)
	}
}

// attachBar is the main bar's entries of m, as (action, key) pairs.
func barActions(m Model) []string {
	var out []string
	for _, e := range m.keyBarEntries() {
		if e.Bar {
			out = append(out, e.Action+": "+e.BarKey)
		}
	}
	return out
}

// Enter label: Enter's label names the tab it opens: Timeline from the Chart,
// Report from Bodies or Aspects, as the second bar entry.
func TestEnterLabelFollowsTheTabAttachShows(t *testing.T) {
	m := helpFixture(t, 144, 40)
	for _, c := range []struct {
		panel PanelID
		want  string
	}{{PanelChart, "Timeline: enter"}, {PanelBodies, "Report: enter"}, {PanelAspects, "Report: enter"}} {
		m.focused = c.panel
		bar := barActions(m)
		if len(bar) < 2 || bar[1] != c.want {
			t.Errorf("focus %v: bar = %v, want second entry %q", c.panel, bar, c.want)
		}
		// The label is what Enter then shows.
		after, _ := m.attachDetails()
		shown := "Report"
		if after.activeTabID(PanelDetails) == TabDetailsTimeline {
			shown = "Timeline"
		}
		if !strings.HasPrefix(c.want, shown+":") {
			t.Errorf("focus %v: label %q but Enter shows the %s tab", c.panel, c.want, shown)
		}
	}
	m.focused = PanelChart
	if line := barLine(m); !strings.Contains(line, "Next panel: → | Timeline: enter | Select: space") {
		t.Errorf("Chart-focused bar = %q", line)
	}
}

// Enter label, absent: no Timeline or Report entry without Details to open or once attached;
// attached Details keeps Jump.
func TestEnterLabelAbsentWithoutDetails(t *testing.T) {
	hidden := helpFixture(t, 80, 24)
	for _, a := range barActions(hidden) {
		if strings.HasSuffix(a, ": enter") {
			t.Errorf("80x24 bar names Enter: %q", a)
		}
	}
	m := helpFixture(t, 144, 40)
	m.focused = PanelChart
	m, _ = m.attachDetails()
	bar := barActions(m)
	jump := false
	for _, a := range bar {
		if a == "Timeline: enter" || a == "Report: enter" {
			t.Errorf("attached bar still names %q", a)
		}
		if a == "Jump: enter" {
			jump = true
		}
	}
	if !jump {
		t.Errorf("attached bar = %v, want Jump: enter", bar)
	}
}

// Popup confirm labels: the list popups confirm with Choose; the multi-selection popups keep
// Apply and Open chart keeps Open.
func TestPopupConfirmLabels(t *testing.T) {
	oldChooserRule(t)
	want := map[owner]string{
		ownerCity: "Choose", ownerHouses: "Choose", ownerZodiac: "Choose",
		ownerBodies: "Apply", ownerAspects: "Apply", ownerOpenChart: "Open",
	}
	for o, w := range want {
		got := ""
		for _, e := range escBase(t).ownerBindings(o) {
			if e.ID == BindingConfirm {
				got = e.Action
			}
		}
		if got != w {
			t.Errorf("owner %d confirms with %q, want %q", o, got, w)
		}
	}
}

// Too small: below the minimum size the screen is still the too-small view.
func TestBelowMinimumIsStillTooSmall(t *testing.T) {
	for _, sz := range [][2]int{{65, 24}, {80, 14}} {
		m := helpFixture(t, sz[0], sz[1])
		if got, want := m.View(), tooSmallView(sz[0], sz[1], chartContentLines(m.slots), m.cfg); got != want {
			t.Errorf("%dx%d is not tooSmallView:\n%s", sz[0], sz[1], ansi.Strip(got))
		}
	}
}
