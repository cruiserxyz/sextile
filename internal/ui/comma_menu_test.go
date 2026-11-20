package ui

import (
	"github.com/muesli/termenv"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// menuWant is the menu's line as the table spells it, key first: the one-key
// rows in the table's order, the group of two-key rows by its letter and word,
// then esc.
func menuWant() []string {
	var out []string
	for _, c := range keys.Commands() {
		switch {
		case c.Then == "":
			out = append(out, c.Letter+" "+c.Word)
		case c.PrefixWord != "":
			out = append(out, c.Letter+" "+c.PrefixWord)
		}
	}
	return append(out, "<esc>")
}

func lastRows(view string, n int) []string { return lastRowsRaw(ansi.Strip(view), n) }

// lastRowsRaw is lastRows with the escape codes left in.
func lastRowsRaw(view string, n int) []string {
	rows := strings.Split(view, "\n")
	return rows[len(rows)-n:]
}

// While a comma waits the key bar is one line, key first with no prefix. At 80
// columns it is never wider than the terminal, starts with `o open` and keeps
// `esc`; at 132 every entry is on it in the table's order.
func TestCommaMenuIsOneLineAndFitsEightyColumns(t *testing.T) {
	m := consoleTasteModel(t, Config{Color: ColorNever}, 80, 24, 12)
	m, _ = press(t, m, ",")
	rows := lastRows(m.View(), 2)
	if strings.Contains(rows[0], "o open") || strings.Contains(rows[0], " then ") {
		t.Errorf("the menu is more than one line: %q", rows)
	}
	line := strings.TrimRight(rows[1], " ")
	if ansi.StringWidth(rows[1]) > 80 {
		t.Errorf("the menu is %d cells at 80 columns", ansi.StringWidth(rows[1]))
	}
	if !strings.HasPrefix(line, " o open  s save") || strings.Contains(line, "p all") || strings.Contains(line, "?") || !strings.HasSuffix(line, "<esc>") {
		t.Errorf("the menu at 80 columns = %q, want it to start `o open`, name no `p` or `?` and end `<esc>`", line)
	}
	if strings.Contains(line, " then ") || strings.Contains(line, ", ") || strings.Contains(line, "c w") || strings.Contains(line, "d save") {
		t.Errorf("the menu carries a prefix or a second-level row: %q", line)
	}

	wide := consoleTasteModel(t, Config{Color: ColorNever}, 132, 40, 12)
	wide, _ = press(t, wide, ",")
	text := strings.TrimRight(lastRows(wide.View(), 1)[0], " ")
	if slices.Contains(strings.Split(strings.TrimSpace(text), "  "), "…") {
		t.Errorf("the menu at 132 columns is cut: %q", text)
	}
	at := 0
	for _, e := range menuWant() {
		i := strings.Index(text[at:], e)
		if i < 0 {
			t.Fatalf("%q is missing or out of order in %q", e, text)
		}
		at += i + len(e)
	}
	if want := " " + strings.Join(menuWant(), "  "); text != want {
		t.Errorf("the menu at 132 columns = %q, want %q", text, want)
	}
}

// After the first key of a two-key row the bar is one line with that level's
// entries, and the resting bar is one line with no menu. `, c x` cancels.
func TestCommaMenuAfterCAndAtRest(t *testing.T) {
	m := consoleTasteModel(t, Config{Color: ColorNever}, 80, 24, 12)
	rest := lastRows(m.View(), 2)
	if strings.Contains(rest[0], "esc") && strings.Contains(rest[0], "o open") || !strings.Contains(rest[1], "Quit: q") {
		t.Errorf("the resting bar is not the one-line bar: %q", rest)
	}
	c, _ := press(t, m, ",", "c")
	rows := lastRows(c.View(), 2)
	if got := strings.TrimRight(rows[1], " "); got != " d save defaults  <esc>" {
		t.Errorf("the line after `, c` = %q", got)
	}
	x, _ := press(t, m, ",", "c", "x")
	if x.leader.pending || x.startup != m.startup || mapShape(x) != mapShape(m) {
		t.Errorf("`, c x` left the leader pending %v, shape %q (was %q)", x.leader.pending, mapShape(x), mapShape(m))
	}
}

// With colour on the line is drawn in the terminal blue; with colour off it
// carries no colour, only the bold of its key letters.
func TestCommaMenuIsDrawnInTheFooterBlue(t *testing.T) {
	forceColor(t)
	sgr, _, _ := strings.Cut(lipgloss.NewStyle().Foreground(term.FooterColour).Render("x"), "x")
	m := consoleTasteModel(t, Config{}, 132, 40, 12)
	m, _ = press(t, m, ",")
	if l := lastRowsRaw(m.View(), 1)[0]; !strings.Contains(l, strings.TrimSuffix(sgr, "m")) {
		t.Errorf("the menu lacks the footer blue %q: %q", sgr, l)
	}
	plain := consoleTasteModel(t, Config{Color: ColorNever}, 132, 40, 12)
	plain, _ = press(t, plain, ",")
	l := lastRowsRaw(plain.View(), 1)[0]
	if rest := strings.NewReplacer("\x1b[1m", "", "\x1b[0m", "").Replace(l); strings.Contains(rest, "\x1b") {
		t.Errorf("the menu has a colour escape with colours off: %q", l)
	}
}

// In both looks each key letter of the menu is bold and its word is not: with
// colours off, and with 256 colours.
func TestCommaMenuLettersAreBoldAndWordsAreNot(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	lipgloss.SetColorProfile(termenv.ANSI256)
	for _, look := range []struct {
		name string
		cfg  Config
		key  lipgloss.Style
		word lipgloss.Style
	}{
		{"no colour", Config{Color: ColorNever}, lipgloss.NewStyle().Bold(true), lipgloss.NewStyle()},
		{"256 colours", Config{Color: ColorAlways}, lipgloss.NewStyle().Bold(true).Foreground(term.FooterColour), lipgloss.NewStyle().Foreground(term.FooterColour)},
	} {
		t.Run(look.name, func(t *testing.T) {
			m := consoleTasteModel(t, look.cfg, 132, 40, 12)
			m, _ = press(t, m, ",")
			line := lastRowsRaw(m.View(), 1)[0]
			for _, e := range [][2]string{{"o", "open"}, {"s", "save"}, {"<esc>", ""}} {
				want := look.key.Render(e[0])
				if e[1] != "" {
					want += " " + look.word.Render(e[1])
				}
				if !strings.Contains(line, want) {
					t.Errorf("%s: the menu lacks %q: %q", look.name, want, line)
				}
			}
			if !strings.Contains(look.key.Render("o"), "\x1b[1") {
				t.Fatalf("%s: the key style is not bold: %q", look.name, look.key.Render("o"))
			}
			if strings.Contains(look.word.Render("open"), "\x1b[1") {
				t.Fatalf("%s: the word style is bold", look.name)
			}
			// No word of the menu is bold: every bold run is a key.
			for _, w := range []string{"open", "save", "city", "defaults…"} {
				at := strings.Index(line, w)
				if at < 0 {
					continue
				}
				if before := line[:at]; strings.LastIndex(before, "\x1b[1") > strings.LastIndex(before, "\x1b[0m") {
					t.Errorf("%s: the word %q stands in a bold run", look.name, w)
				}
			}
		})
	}
}

// The screen keeps its height while the menu shows, and the model's own
// geometry does not change.
func TestCommaMenuKeepsTheScreenHeight(t *testing.T) {
	for _, size := range []struct{ w, h int }{{80, 24}, {132, 40}} {
		m := consoleTasteModel(t, Config{Color: ColorNever}, size.w, size.h, 12)
		if got := len(strings.Split(m.View(), "\n")); got != size.h {
			t.Errorf("%dx%d at rest: %d rows", size.w, size.h, got)
		}
		rest := m.geometry()[PanelBodies].h
		for _, keysTyped := range [][]string{{","}, {",", "c"}} {
			p, _ := press(t, m, keysTyped...)
			if got := len(strings.Split(p.View(), "\n")); got != size.h {
				t.Errorf("%dx%d after %v: %d rows", size.w, size.h, keysTyped, got)
			}
			if got := p.geometry()[PanelBodies].h; got != rest {
				t.Errorf("%dx%d after %v: Bodies is %d rows, %d at rest", size.w, size.h, keysTyped, got, rest)
			}
		}
	}
}

// Keybindings opens on the Local header with no line about the comma, names
// each command by its Ctrl key and has no leader section.
func TestKeybindingsOpensOnLocalAndNamesTheCtrlKey(t *testing.T) {
	m := escBase(t)
	lines := keybindingLines(m)
	first := openedKeybindings(m).keybindings.Lines()
	if first[0].Kind != keys.LineHeader || first[0].Text != "─── Local" {
		t.Errorf("Keybindings opens with %q (kind %v), want the Local header", first[0].Text, first[0].Kind)
	}
	for _, l := range lines {
		if strings.Contains(l.Text, "no Ctrl form") {
			t.Errorf("Keybindings still says %q", l.Text)
		}
	}
	texts := helpBindingTexts(lines)
	keyOf := func(title string) string {
		for _, l := range texts {
			if parts := helpRow.FindStringSubmatch(l); parts != nil && parts[1] == title {
				return parts[2]
			}
		}
		return "(no row)"
	}
	for title, want := range map[string]string{"Open chart": "<ctrl+o>", "Choose houses": "<ctrl+u>", "Keep as defaults": ", c d", "Sessions": ", O", "Log": ", L"} {
		if got := keyOf(title); got != want {
			t.Errorf("Help prints %q for %q, want %q", got, title, want)
		}
	}
	for _, l := range lines {
		if strings.HasPrefix(l.Text, "leader:") {
			t.Errorf("Help still has a leader section: %q", l.Text)
		}
	}
}

// The popup prints for every command the key the table spells for it.
func TestKeybindingsPrintsTheCommandsKey(t *testing.T) {
	m := openedKeybindings(paletteModel(t))
	hc := m.keysContextFor(ownerMain)
	letters := map[keys.BindingID]string{}
	for _, e := range slices.Concat(hc.Table, leaderOnlyBindings()) {
		letters[e.ID] = keys.ActionKey(hc, e)
	}
	for desc, want := range map[string]string{"Choose houses": "<ctrl+u>", "Sessions": ", O"} {
		if got := rowAt(t, m, desc).Key; got != want {
			t.Errorf("the popup prints %q for %q, want %q", got, desc, want)
		}
	}
	checked := 0
	for _, r := range m.keybindings.Matches {
		if want, ok := letters[r.ID]; ok && r.Group == keys.GroupGlobal && want != "" {
			checked++
			if r.Key != want {
				t.Errorf("%q prints %q, want %q", r.Desc, r.Key, want)
			}
		}
	}
	if checked < 10 {
		t.Errorf("only %d commands of the table are in the popup", checked)
	}
	if n := strings.Count(screen(m), "then the letter does the same as Ctrl."); n != 0 {
		t.Errorf("the line about the letter shows %d times, want none", n)
	}
}
