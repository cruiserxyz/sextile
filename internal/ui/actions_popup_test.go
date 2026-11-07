package ui

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// actions_popup_test.go holds the Actions palette (palette P2): Ctrl+P opens a
// searchable list of the 24 commands, each with its key, the blocked ones faint
// with their reason. The seam under it (runAction, blocked) is actions_test.go.

var keyCtrlP = tea.KeyMsg{Type: tea.KeyCtrlP}

// alphabetical is the 24 titles with nothing typed and nothing run yet, as m
// names them: Enter reads Timeline or Report by the Details tab it would show.
func alphabetical(m Model) []string {
	out := []string{
		"Back to the previous time", "Choose aspects", "Choose bodies", "Choose city", "Choose houses", "Choose zodiac",
		"Copy", "Help", "House display", "Log", "Log console", "Move chart down", "Move chart up", "New session", "Now", "Open chart",
		"Quit", "Remove chart", "Save chart", "Save defaults", "Select", "Sessions", m.attachTitle(), "Wheel image",
	}
	slices.SortStableFunc(out, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return out
}

// actionTitles are the palette's rows now, in order.
func actionTitles(m Model) []string {
	out := make([]string, 0, len(m.actions.Matches))
	for _, r := range m.actions.Matches {
		out = append(out, r.Title)
	}
	return out
}

// typeInActions sends text one key at a time through the whole Update path.
func typeInActions(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m, _ = tpress(t, m, runes(string(r)))
	}
	return m
}

// openActionsPalette is Ctrl+P on a ready model, and checks it opened.
func openActionsPalette(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = tpress(t, m, keyCtrlP)
	if !m.actionsOpen || m.currentOwner() != ownerActions {
		t.Fatalf("Ctrl+P did not open the Actions palette: open=%v owner=%v", m.actionsOpen, m.currentOwner())
	}
	return m
}

// runViaPalette opens the palette, types query and presses Enter on the first
// row.
func runViaPalette(t *testing.T, m Model, query string) Model {
	t.Helper()
	m = openActionsPalette(t, m)
	m = typeInActions(t, m, query)
	m, _ = tpress(t, m, keyEnter)
	return m
}

func screen(m Model) string { return ansi.Strip(m.View()) }

// Ctrl+P on the main screen opens "Actions": the search line, the 24 rows, the
// count, and its own key bar. A popup, Help or attached Details that owns the
// keys does not open it.
func TestCtrlPOpensActions(t *testing.T) {
	t.Run("opens on the main screen", func(t *testing.T) {
		m := paletteModel(t)
		if m.currentOwner() != ownerMain {
			t.Fatalf("fixture: owner %v, want the main screen", m.currentOwner())
		}
		before := m.startup
		m = openActionsPalette(t, m)
		if m.startup != before {
			t.Errorf("opening the palette moved the startup phase from %q to %q: blocked reads it, and every row would grey", before, m.startup)
		}
		if got, want := actionTitles(m), alphabetical(m); !slices.Equal(got, want) {
			t.Errorf("rows\n%v\nwant\n%v", got, want)
		}
		view := screen(m)
		for _, want := range []string{"Actions", "Search:", "24 actions", "Back to the previous time", "no earlier time to go back to"} {
			if !strings.Contains(view, want) {
				t.Errorf("the screen lacks %q:\n%s", want, view)
			}
		}
		if bar := strings.TrimSpace(lastRow(m.View())); bar != "Move: ↑↓ | Run: enter | Close: esc" {
			t.Errorf("key bar = %q, want its own: Move: ↑↓ | Run: enter | Close: esc", bar)
		}
		if m.actions.Cursor != 0 {
			t.Errorf("cursor starts on row %d, want the first", m.actions.Cursor)
		}
	})
	t.Run("not under Help", func(t *testing.T) {
		m := paletteModel(t)
		m.helpOpen = true
		got, _ := tpress(t, m, keyCtrlP)
		if got.actionsOpen || !got.helpOpen {
			t.Errorf("Ctrl+P in Help: palette open=%v, Help open=%v", got.actionsOpen, got.helpOpen)
		}
	})
	t.Run("not under a popup", func(t *testing.T) {
		m := paletteModel(t)
		m.startup = startupAtlasOpen
		got, _ := tpress(t, m, keyCtrlP)
		if got.actionsOpen || got.startup != startupAtlasOpen {
			t.Errorf("Ctrl+P in the City popup: palette open=%v, startup %q", got.actionsOpen, got.startup)
		}
	})
	t.Run("not under the Log console", func(t *testing.T) {
		m := paletteModel(t)
		m.console.open = true
		got, _ := tpress(t, m, keyCtrlP)
		if got.actionsOpen || !got.console.open {
			t.Errorf("Ctrl+P in the console: palette open=%v, console open=%v", got.actionsOpen, got.console.open)
		}
	})
	t.Run("not from attached Details", func(t *testing.T) {
		stubTextCompute(t)
		stubRawReports(t)
		m := onTimeline(t, Config{Accessible: true})
		got, _ := tpress(t, m, keyCtrlP)
		if got.actionsOpen || !got.details.attached {
			t.Errorf("Ctrl+P in attached Details: palette open=%v, Details attached=%v", got.actionsOpen, got.details.attached)
		}
	})
}

// With nothing typed the commands run in this run lead, most recent first, and
// the rest follow alphabetically by title.
func TestActionsRowsAreAlphabeticalRecentFirst(t *testing.T) {
	m := paletteModel(t)
	// Nothing run yet.
	if got := actionTitles(openActionsPalette(t, m)); !slices.Equal(got, alphabetical(m)) {
		t.Fatalf("a fresh run lists\n%v\nwant\n%v", got, alphabetical(m))
	}
	m = runViaPalette(t, m, "now")
	if m.actionsOpen {
		t.Fatal("Enter on Now left the palette open")
	}
	m = runViaPalette(t, m, "help")
	if !m.helpOpen {
		t.Fatal("Enter on Help did not open Help")
	}
	m, _ = tpress(t, m, keyEsc)
	m = openActionsPalette(t, m)
	rest := slices.DeleteFunc(alphabetical(m), func(s string) bool { return s == "Help" || s == "Now" })
	want := append([]string{"Help", "Now"}, rest...)
	if got := actionTitles(m); !slices.Equal(got, want) {
		t.Errorf("after Now then Help the rows are\n%v\nwant\n%v", got, want)
	}
	// Running Now again moves it to the front, and it is listed once.
	m, _ = tpress(t, m, keyEsc)
	m = runViaPalette(t, m, "now")
	want = append([]string{"Now", "Help"}, rest...)
	if got := actionTitles(openActionsPalette(t, m)); !slices.Equal(got, want) {
		t.Errorf("after Now again the rows are\n%v\nwant\n%v", got, want)
	}
}

// Typing filters by title with the finders' tiers (location.MatchTier through
// menu.RankByTier): starts with, contains, then the letters in order. Within a
// tier the command run most recently leads, then alphabetical.
func TestActionsFilterRanksLikeTheFinders(t *testing.T) {
	m := openActionsPalette(t, paletteModel(t))
	m = typeInActions(t, m, "ch")
	want := []string{
		"Choose aspects", "Choose bodies", "Choose city", "Choose houses", "Choose zodiac", // starts with
		"Move chart down", "Move chart up", "Open chart", "Remove chart", "Save chart", // contains
		"Back to the previous time", // c, then h, in order
	}
	if got := actionTitles(m); !slices.Equal(got, want) {
		t.Fatalf("`ch` ranks\n%v\nwant\n%v", got, want)
	}
	if view := screen(m); !strings.Contains(view, "11 of 24") || strings.Contains(view, "24 actions") {
		t.Errorf("the count with `ch` typed is not `11 of 24`:\n%s", view)
	}
	if m.actions.Cursor != 0 {
		t.Errorf("typing left the cursor on row %d, want the first", m.actions.Cursor)
	}

	// Within a tier the command run most recently leads.
	m, _ = tpress(t, m, keyEsc)
	m = runViaPalette(t, m, "open chart")
	if m.startup != startupChartOpen {
		t.Fatalf("Open chart did not open its picker: startup %q", m.startup)
	}
	m, _ = tpress(t, m, keyEsc)
	m = typeInActions(t, openActionsPalette(t, m), "ch")
	want = []string{
		"Choose aspects", "Choose bodies", "Choose city", "Choose houses", "Choose zodiac",
		"Open chart", "Move chart down", "Move chart up", "Remove chart", "Save chart",
		"Back to the previous time",
	}
	if got := actionTitles(m); !slices.Equal(got, want) {
		t.Errorf("`ch` after Open chart ran ranks\n%v\nwant\n%v", got, want)
	}

	// Nothing matches: the finders' note, and the count says none of 24.
	m = typeInActions(t, m, "zzz")
	if len(m.actions.Matches) != 0 {
		t.Fatalf("`chzzz` still matches %v", actionTitles(m))
	}
	if view := screen(m); !strings.Contains(view, "(no matching actions)") || !strings.Contains(view, "0 of 24") {
		t.Errorf("an empty result lacks the no-match line or `0 of 24`:\n%s", view)
	}
}

// The key at the right is Help's own spelling: the same text, for every row.
// With a configured leader a leader row is the leader plus its sequence, as
// Help's leader section prints it, and Save chart shows its main key only.
var helpRow = regexp.MustCompile(`^(.*?) {2,}(\S.*)$`)

func TestActionsKeyColumnIsHelpsSpelling(t *testing.T) {
	m := paletteModel(t)
	m.leaderKey = ";"
	m = openActionsPalette(t, m)
	help := helpBindingTexts(m.helpLines())
	// helpKey is the key Help prints for title: the first binding row of that
	// title (the main groups come before the leader section).
	helpKey := func(title string) (string, bool) {
		for _, l := range help {
			// A row is its title, a run of padding, then the key.
			if parts := helpRow.FindStringSubmatch(l); parts != nil && parts[1] == title {
				return parts[2], true
			}
		}
		return "", false
	}
	// Help leaves its own two out, and the Log console has no key row of its
	// own: it is the Log's `l`, which the Log section lists.
	unlisted := map[string]string{"Help": "?", "Quit": "q", "Log console": ";l l"}
	checked := 0
	for _, r := range m.actions.Matches {
		want, listed := helpKey(r.Title)
		if w, ok := unlisted[r.Title]; ok {
			want, listed = w, true
		}
		if !listed {
			t.Errorf("%q is a palette row and Help lists no key for it", r.Title)
			continue
		}
		if r.Key != want {
			t.Errorf("%q: the palette prints %q, Help prints %q", r.Title, r.Key, want)
		}
		checked++
	}
	if checked != 24 {
		t.Errorf("compared %d of the 24 rows with Help", checked)
	}
	keyOf := func(title string) string {
		for _, r := range m.actions.Matches {
			if r.Title == title {
				return r.Key
			}
		}
		return "(no row)"
	}
	// The leader rows carry the configured leader; the compound one its
	// sequence. Help's leader section writes the leader and the sequence with
	// no space between them, and so does the palette.
	for title, want := range map[string]string{
		"Log": ";l", "Log console": ";l l", "Copy": ";y", "Save defaults": ";c d", "House display": ";a", "Sessions": ";o", "New session": ";n (twice)",
		"Save chart": "⌃s", "Choose houses": "⌃h", "Select": "space", m.attachTitle(): "enter", "Remove chart": "del",
	} {
		if got := keyOf(title); got != want {
			t.Errorf("%q key = %q, want %q", title, got, want)
		}
	}
	if view := screen(m); !strings.Contains(view, "Log console") || !strings.Contains(view, ";l l") {
		t.Errorf("the screen does not show the configured leader on the Log console row:\n%s", view)
	}
}

// Enter on an available row closes the palette, records the command, and runs
// it as the key would: Choose houses opens the house finder, and Esc there
// returns to the main screen, not to the palette.
func TestEnterRunsTheRowAndCloses(t *testing.T) {
	m := escBase(t)
	m.focused = PanelChart
	want := snapshotForEsc(m)
	m = openActionsPalette(t, m)
	m = typeInActions(t, m, "choose houses")
	if got := actionTitles(m); len(got) == 0 || got[0] != "Choose houses" {
		t.Fatalf("`choose houses` leads with %v", got)
	}
	m, _ = tpress(t, m, keyEnter)
	if m.actionsOpen {
		t.Error("Enter left the palette open")
	}
	if m.startup != startupHouseOpen || m.currentOwner() != ownerHouses {
		t.Fatalf("Enter on Choose houses: startup %q, owner %v, want the house finder", m.startup, m.currentOwner())
	}
	if len(m.recentActions) != 1 || m.recentActions[0] != keys.BindingHouses {
		t.Errorf("recent = %v, want [Houses]", m.recentActions)
	}
	if !strings.Contains(screen(m), "Houses") {
		t.Errorf("the house finder is not on screen:\n%s", screen(m))
	}
	// The same state the key leaves.
	pressed, _ := tpress(t, escBase(t), tea.KeyMsg{Type: tea.KeyCtrlH})
	if pressed.startup != m.startup {
		t.Errorf("the key leaves startup %q, the palette %q", pressed.startup, m.startup)
	}
	m, _ = tpress(t, m, keyEsc)
	if m.actionsOpen || m.currentOwner() != ownerMain {
		t.Errorf("Esc in the finder: palette open=%v, owner %v, want the main screen", m.actionsOpen, m.currentOwner())
	}
	requireSameScreen(t, "after Esc in the finder", want, snapshotForEsc(m))
}

// Enter on a blocked row runs nothing: the palette stays open and the bottom
// border says why, in place of the count, until the next key.
func TestEnterOnABlockedRowSaysWhy(t *testing.T) {
	m := paletteModel(t)
	m.cursorIdx = 2 // the last opened chart: nothing below it to swap with
	if why := m.blocked(keys.BindingSlotMoveDown); why != whyNoneBelow {
		t.Fatalf("fixture: Move chart down is blocked by %q, want %q", why, whyNoneBelow)
	}
	before := snapOf(m)
	m = openActionsPalette(t, m)
	m = typeInActions(t, m, "move chart down")
	if got := actionTitles(m); len(got) == 0 || got[0] != "Move chart down" {
		t.Fatalf("`move chart down` leads with %v", got)
	}
	m, cmd := tpress(t, m, keyEnter)
	if cmd != nil {
		t.Error("Enter on a blocked row returned a command")
	}
	if !m.actionsOpen {
		t.Fatal("Enter on a blocked row closed the palette")
	}
	if len(m.recentActions) != 0 {
		t.Errorf("a command that did not run was recorded: %v", m.recentActions)
	}
	if !before.equal(snapOf(m)) {
		t.Error("Enter on a blocked row changed the slots, the cursor, the Log or a notice")
	}
	view := screen(m)
	if !strings.Contains(view, "Move chart down: no chart below this one") {
		t.Errorf("the border does not read `Move chart down: no chart below this one`:\n%s", view)
	}
	if strings.Contains(view, "1 of 24") {
		t.Errorf("the count still shows beside the reason:\n%s", view)
	}
	// The next key brings the count back.
	m, _ = tpress(t, m, keyUp)
	if view := screen(m); strings.Contains(view, "Move chart down: no chart below this one") || !strings.Contains(view, "1 of 24") {
		t.Errorf("the reason outlived the next key:\n%s", view)
	}
}

// A blocked row is faint with colours on and says its reason in the text, so it
// reads with colours off too; an available row has no reason.
func TestBlockedActionsAreFaintAndSayWhy(t *testing.T) {
	m := openActionsPalette(t, paletteModel(t))
	row := func(view, title string) string {
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(ansi.Strip(l), title) {
				return l
			}
		}
		t.Fatalf("no row %q in:\n%s", title, view)
		return ""
	}
	forceColor(t)
	m.cfg = Config{Color: ColorAlways}
	colour := m.View()
	if l := row(colour, "Back to the previous time"); !strings.Contains(l, "\x1b[2m") {
		t.Errorf("a blocked row is not faint: %q", l)
	}
	if l := row(colour, "Choose aspects"); strings.Contains(l, "\x1b[2m") {
		t.Errorf("an available row is faint: %q", l)
	}
	m.cfg = Config{Accessible: true}
	plain := m.View()
	if l := ansi.Strip(row(plain, "Back to the previous time")); !strings.Contains(l, "no earlier time to go back to") {
		t.Errorf("with colours off a blocked row does not say why: %q", l)
	}
	if l := ansi.Strip(row(plain, "Choose aspects")); strings.Contains(l, "no ") || strings.Contains(l, "needs") {
		t.Errorf("an available row carries a reason: %q", l)
	}
}

// Esc closes the palette and leaves the screen exactly as before, whatever was
// typed or moved, and the next opening starts clean.
func TestEscClosesActions(t *testing.T) {
	m := escBase(t)
	want := snapshotForEsc(m)
	opened := openActionsPalette(t, m)
	opened = typeInActions(t, opened, "ch")
	opened, _ = tpress(t, opened, keyDown)
	opened, _ = tpress(t, opened, keyDown)
	closed, cmd := tpress(t, opened, keyEsc)
	if cmd != nil {
		t.Error("Esc returned a command")
	}
	if closed.actionsOpen || closed.currentOwner() != ownerMain {
		t.Errorf("Esc: palette open=%v, owner %v", closed.actionsOpen, closed.currentOwner())
	}
	requireSameScreen(t, "after Esc", want, snapshotForEsc(closed))
	if closed.View() != m.View() {
		t.Error("the screen after Esc is not the screen before Ctrl+P")
	}
	again := openActionsPalette(t, closed)
	if again.actions.FilterText != "" || again.actions.Cursor != 0 || len(again.actions.Matches) != 24 {
		t.Errorf("the next opening starts with filter %q, cursor %d, %d rows", again.actions.FilterText, again.actions.Cursor, len(again.actions.Matches))
	}
	if len(closed.recentActions) != 0 {
		t.Errorf("Esc recorded %v as run", closed.recentActions)
	}
}

// ↑↓ and the paging keys move the cursor, stop at the ends, and Enter acts on
// the row under it.
func TestActionsCursorMovesAndEnterUsesIt(t *testing.T) {
	m := openActionsPalette(t, paletteModel(t))
	m, _ = tpress(t, m, keyUp)
	if m.actions.Cursor != 0 {
		t.Errorf("Up on the first row moved the cursor to %d", m.actions.Cursor)
	}
	m, _ = tpress(t, m, keyDown)
	m, _ = tpress(t, m, keyDown)
	if m.actions.Cursor != 2 {
		t.Fatalf("two Downs put the cursor on %d, want 2", m.actions.Cursor)
	}
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyEnd})
	if m.actions.Cursor != 23 {
		t.Errorf("End put the cursor on %d, want the last row", m.actions.Cursor)
	}
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeyHome})
	if m.actions.Cursor != 0 {
		t.Errorf("Home put the cursor on %d, want the first row", m.actions.Cursor)
	}
	// Row 2 is "Choose bodies": Enter opens the body selector.
	m, _ = tpress(t, m, keyDown)
	m, _ = tpress(t, m, keyDown)
	m, _ = tpress(t, m, keyEnter)
	if m.startup != startupBodyOpen {
		t.Errorf("Enter on the third row (%q) left startup %q, want the body selector", "Choose bodies", m.startup)
	}
}

// Ctrl+P is in Help and not on the key bar: the main bar is the bar it was at
// both widths, and Help lists Actions with its key in its first group.
func TestCtrlPIsNotOnTheBar(t *testing.T) {
	m := paletteModel(t)
	table := mainBindings(keys.DefaultKeys())
	var without []keys.BindingEntry
	var actions *keys.BindingEntry
	for i := range table {
		if table[i].ID == keys.BindingActions {
			actions = &table[i]
			continue
		}
		without = append(without, table[i])
	}
	if actions == nil {
		t.Fatal("Ctrl+P is not in the main table")
	}
	if actions.Bar || actions.Palette || !actions.Help || actions.Title != "Actions" {
		t.Errorf("the Ctrl+P entry is %+v, want title Actions, in Help, not on the bar and not one of its own rows", *actions)
	}
	if k := actions.Binding.Keys(); len(k) != 1 || k[0] != "ctrl+p" {
		t.Errorf("Ctrl+P matches %v", k)
	}
	for _, w := range []int{80, 132} {
		with := m.barText(ownerMain, table, w, Config{Accessible: true}, "")
		base := m.barText(ownerMain, without, w, Config{Accessible: true}, "")
		if with != base {
			t.Errorf("the %d-column main bar changed:\n%q\nwas\n%q", w, with, base)
		}
		if strings.Contains(with, "⌃p") {
			t.Errorf("the %d-column main bar names ⌃p: %q", w, with)
		}
	}
	// Help lists it, in the first group, with its key.
	var found bool
	for _, s := range keys.HelpSections(table) {
		for _, e := range s.Entries {
			if e.ID == keys.BindingActions {
				found = s.Title == "Start here"
			}
		}
	}
	if !found {
		t.Error("Help does not list Actions in its Start here group")
	}
	listed := false
	for _, l := range helpBindingTexts(m.helpLines()) {
		if strings.HasPrefix(l, "Actions") && strings.HasSuffix(l, "⌃p") {
			listed = true
		}
	}
	if !listed {
		t.Errorf("Help's rows do not include `Actions ... ⌃p`:\n%s", strings.Join(helpBindingTexts(m.helpLines()), "\n"))
	}
}

// The palette is drawn at 80x24 in colour and accessible, with nothing typed
// and with `ch`, as component goldens like the finders'.
func TestActionsGolden(t *testing.T) {
	forceColor(t)
	modes := []struct {
		name string
		cfg  Config
	}{{"color", Config{Color: ColorAlways}}, {"accessible", Config{Accessible: true}}}
	for _, st := range []struct{ name, filter string }{{"empty", ""}, {"ch", "ch"}} {
		for _, mode := range modes {
			name := fmt.Sprintf("actions-%s-80x24-%s", st.name, mode.name)
			t.Run(name, func(t *testing.T) {
				// One opened chart over Now, the cursor on it: nothing above or below
				// to swap with, no earlier time to go back to, and no room for Details
				// at 80x24, so four commands are blocked, each with its own reason.
				m := escBase(t)
				m.cfg = mode.cfg
				m.width, m.height = 80, 24
				m.focused = PanelChart
				m.cursorIdx = 0
				next, _ := openActions(m)
				m = next.(Model)
				state := m.actions
				state.FilterText = st.filter
				state = m.refilterActions(state)
				checkGolden(t, name, renderActions(state, m.blocked, mode.cfg, m.width, m.height))
			})
		}
	}
}
