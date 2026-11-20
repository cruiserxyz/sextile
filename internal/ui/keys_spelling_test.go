package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// Keys are spelled as lazygit spells them: angle brackets in Keybindings, plain
// words in the footer, and no glyph there at all. A Ctrl key is on a bar only
// when it is a popup's own ctrl+r or ctrl+d.

const keyGlyphs = "⌃⇧↑↓←→⇞⇟⏎⌫"

var spellingOwners = []owner{
	ownerMain, ownerCity, ownerOpenChart, ownerChartTitle, ownerSaveChart, ownerBodies,
	ownerHouses, ownerZodiac, ownerAspects, ownerKeybindings, ownerConsole, ownerDetails,
	ownerSessions, ownerSessionName, ownerLog, ownerExperiments,
}

// ctrlKeyInBar finds every Ctrl key a bar line or a bar key names.
var ctrlKeyInBar = regexp.MustCompile(`ctrl\+[a-z0-9]+`)

// popupBarCtrl are the only Ctrl keys a bar may show, and only a popup's own bar:
// the main screen's Ctrl keys repeat comma commands and stay off it.
var popupBarCtrl = map[string]bool{"ctrl+r": true, "ctrl+d": true}

func TestNoKeyGlyphInKeybindingsOrTheFooter(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {132, 40}} {
		m := helpFixture(t, size[0], size[1])
		for _, o := range spellingOwners {
			popup := o != ownerMain && o != ownerDetails && o != ownerLog && o != ownerConsole
			okCtrl := func(text string) string {
				for _, k := range ctrlKeyInBar.FindAllString(text, -1) {
					if !popup || !popupBarCtrl[k] {
						return k
					}
				}
				return ""
			}
			for _, r := range m.keybindingRows(o) {
				if strings.ContainsAny(r.Key+r.Desc, keyGlyphs) {
					t.Errorf("%dx%d owner %d: Keybindings row %q spells %q with a glyph", size[0], size[1], o, r.Desc, r.Key)
				}
			}
			c := m.keysContextFor(o)
			for _, line := range []string{
				ansi.Strip(keys.RenderBar(c, size[0])), keys.BarLine(c, size[0]),
			} {
				if strings.ContainsAny(line, keyGlyphs) {
					t.Errorf("%dx%d owner %d: the footer reads %q with a glyph", size[0], size[1], o, line)
				}
				if k := okCtrl(line); k != "" {
					t.Errorf("%dx%d owner %d: the footer reads %q and shows %s", size[0], size[1], o, line, k)
				}
			}
			// Every table entry that may be drawn, whatever its condition says now.
			for _, e := range keys.WithWords(c, o, c.Table) {
				if !e.Bar {
					continue
				}
				if strings.ContainsAny(e.BarKey, keyGlyphs) {
					t.Errorf("owner %d: bar entry %q has the key %q", o, e.Title, e.BarKey)
				}
				if k := okCtrl(e.BarKey); k != "" {
					t.Errorf("owner %d: bar entry %q has the Ctrl key %s", o, e.Title, k)
				}
			}
		}
	}
}

func TestKeybindingsSpellsKeysAsLazygitDoes(t *testing.T) {
	m := openedKeybindings(kbModel(t))
	for desc, want := range map[string]string{
		"Choose city":           "<ctrl+l>",
		m.attachTitle():         "<enter>",
		"Go to the chart above": "<up>",
		"Go to the chart below": "<down>",
		"Scroll Details up":     "<ctrl+pgup>",
		"Scroll Details down":   "<ctrl+pgdown>",
		"Page list up":          "<pgup>",
		"Page list down":        "<pgdown>",
		"Move chart up":         "K",
		"Next panel":            "<right>",
		"Previous panel":        "<left>",
	} {
		if got := rowAt(t, m, desc).Key; got != want {
			t.Errorf("row %q is spelled %q, want %q", desc, got, want)
		}
	}
	if !strings.Contains(screen(m), "Choose city…") || !strings.Contains(screen(m), "<ctrl+l>") {
		t.Errorf("the popup does not show `<ctrl+l> Choose city…`:\n%s", screen(m))
	}
	if got := keys.Spell(" "); got != "<space>" {
		t.Errorf("Spell(space) = %q", got)
	}
}

func TestResting80ColumnBarKeepsMoonPhaseWithColour(t *testing.T) {
	m := helpFixture(t, 80, 24)
	m.cfg = Config{Color: ColorAlways}
	line := ansi.Strip(keys.RenderBar(m.keysContextFor(ownerMain), 80))
	if !strings.Contains(line, "Moon phase") {
		t.Errorf("the 80-column bar lost Moon phase: %q", line)
	}
	for _, w := range []string{"Commands", "Keybindings", "Quit"} {
		if !strings.Contains(line, w) {
			t.Errorf("the 80-column bar lost %s: %q", w, line)
		}
	}
}

// In Details Left leaves it as Esc does and Right does nothing, beside the
// panels or right under one.
func TestDetailsLeftLeavesAndRightDoesNothing(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {132, 40}} {
		open := func() Model {
			m := everySizeModel(t, 1, size[0], size[1])
			m, _ = press(t, m, "enter")
			if !m.details.attached {
				t.Fatalf("%dx%d: Enter did not attach Details", size[0], size[1])
			}
			return m
		}
		m := open()
		focus, tab := m.focused, m.activeTabID(PanelDetails)
		r, _ := pressOnly(t, m, keyRight)
		if r.focused != focus || !r.details.attached || r.activeTabID(PanelDetails) != tab {
			t.Errorf("%dx%d: Right changed focus %q->%q, attached %v, tab %v->%v", size[0], size[1], focus, r.focused, r.details.attached, tab, r.activeTabID(PanelDetails))
		}
		l, _ := pressOnly(t, open(), keyLeft)
		if l.details.attached || l.focused != focus {
			t.Errorf("%dx%d: Left left attached %v, focus %q, want detached on %q", size[0], size[1], l.details.attached, l.focused, focus)
		}
		d, _ := pressOnly(t, open(), runes("2"))
		if d.focused != PanelChart || !d.details.attached {
			t.Errorf("%dx%d: 2 left focus %q attached %v", size[0], size[1], d.focused, d.details.attached)
		}
	}
}

// The sessions popup's bar shows its own Rename and Delete keys, each only
// while it acts.
func TestSessionsBarShowsItsOwnCtrlKeys(t *testing.T) {
	m := helpFixture(t, 132, 40)
	c := m.keysContextFor(ownerSessions)
	c.Sessions.OnFreeRow, c.Sessions.CanTrash = true, true
	line := keys.BarLine(c, 132)
	for _, want := range []string{"Rename: <c-r>", "Delete: <c-d>"} {
		if !strings.Contains(line, want) {
			t.Errorf("the sessions bar reads %q, want %q", line, want)
		}
	}
	c.Sessions.OnFreeRow, c.Sessions.CanTrash = false, false
	if line := keys.BarLine(c, 132); strings.Contains(line, "ctrl") {
		t.Errorf("the sessions bar shows a Ctrl key that does not act: %q", line)
	}
}
