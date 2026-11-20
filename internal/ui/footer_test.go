package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// footerOwners is every screen with a footer row, reached at 132x40 without
// colour: the resting bar, every popup of keyOwnerCases and Sessions.
func footerOwners(t *testing.T, cfg Config) map[string]Model {
	t.Helper()
	out := map[string]Model{}
	for _, c := range keyOwnerCases() {
		w := 132
		if c.owner == ownerDetails || c.owner == ownerLog {
			w = 144 // Details and the focused Log exist only in the dual column
		}
		out[c.name] = reachOwner(t, c, cfg, w, 40)
	}
	s := openSessions(t, sessionsModel(t, 2))
	s.cfg, s.width, s.height = cfg, 132, 40
	out["sessions"] = s
	return out
}

func footerPairs(m Model) []string { return barPairs(lastRow(m.View())) }

// (a) Common movement is not on the footer, anywhere, and stays in Keybindings.
func TestFooterCarriesNoCommonMovement(t *testing.T) {
	for name, m := range footerOwners(t, Config{Color: ColorNever}) {
		for _, p := range footerPairs(m) {
			word, _, _ := strings.Cut(p, ":")
			switch strings.TrimSpace(word) {
			case "Switch panel", "Move", "Move row":
				t.Errorf("%s: the footer names %q", name, p)
			}
		}
	}
	kb := openedKeybindings(kbModel(t))
	for _, desc := range []string{"Next panel", "Go to the chart above"} {
		rowAt(t, kb, desc)
	}
}

var footerKeyToken = regexp.MustCompile(`^(<[a-z0-9-]+>|[^<>\s/]+)$`)

// (b) Footer keys are in lazygit's notation; Keybindings keeps its own.
func TestFooterKeysAreInLazygitNotation(t *testing.T) {
	bare := map[string]bool{"enter": true, "esc": true, "space": true, "tab": true, "up": true, "down": true, "left": true, "right": true, "del": true}
	rows := map[string][]string{}
	for name, m := range footerOwners(t, Config{Color: ColorNever}) {
		rows[name] = footerPairs(m)
	}
	cm := consoleTasteModel(t, Config{Color: ColorNever}, 132, 40, 12)
	cm, _ = press(t, cm, ",")
	for _, w := range strings.Fields(lastRow(cm.View())) {
		if w == "<esc>" {
			rows["comma menu"] = []string{"x: " + w}
		}
	}
	if rows["comma menu"] == nil {
		t.Errorf("the comma menu does not end with <esc>: %q", lastRow(cm.View()))
	}
	for name, pairs := range rows {
		for _, p := range pairs {
			if p == "…" || p == "..." {
				continue
			}
			_, key, ok := strings.Cut(p, ": ")
			if !ok {
				continue
			}
			for _, tok := range strings.FieldsFunc(key, func(r rune) bool { return r == ' ' || r == '/' }) {
				if bare[tok] || strings.HasPrefix(tok, "ctrl+") || !footerKeyToken.MatchString(tok) && !strings.HasPrefix(tok, "<") {
					t.Errorf("%s: the footer pair %q writes %q, not lazygit's notation", name, p, tok)
				}
			}
		}
	}
	if got := rowAt(t, openedKeybindings(kbModel(t)), "Choose houses").Key; !strings.Contains(got, "ctrl+u") || strings.Contains(got, "<c-") {
		t.Errorf("Keybindings prints %q for Choose houses, want its ctrl+u notation", got)
	}
}

// (c) The footer row is the terminal's blue in both colour looks, the comma
// menu's letters bold and its words not, the notice cyan, no escape without colour.
func TestFooterIsTheTerminalBlue(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	for _, look := range []struct {
		name string
		prof termenv.Profile
	}{{"256 colours", termenv.ANSI256}, {"16 colours", termenv.ANSI}} {
		lipgloss.SetColorProfile(look.prof)
		for name, m := range footerOwners(t, Config{Color: ColorAlways}) {
			lines := strings.Split(m.View(), "\n")
			text := 0
			for _, c := range parseDesignScreen(lines[len(lines)-1])[0] {
				if c.r == ' ' {
					continue
				}
				text++
				if c.fg != "34" {
					t.Errorf("%s, %s: footer cell %q is %q, want 34", look.name, name, c.r, c.fg)
				}
			}
			if text == 0 {
				t.Errorf("%s, %s: empty footer", look.name, name)
			}
		}
		cm := consoleTasteModel(t, Config{Color: ColorAlways}, 132, 40, 12)
		cm, _ = press(t, cm, ",")
		lines := strings.Split(cm.View(), "\n")
		row := parseDesignScreen(lines[len(lines)-1])[0]
		var bold, plainWord int
		for i, c := range row {
			if c.r == ' ' {
				continue
			}
			if c.fg != "34" {
				t.Errorf("%s: comma menu cell %q is %q, want 34", look.name, c.r, c.fg)
			}
			if i == 1 && !c.bold {
				t.Errorf("%s: the first key letter of the comma menu is not bold", look.name)
			}
			if c.bold {
				bold++
			} else {
				plainWord++
			}
		}
		if bold == 0 || plainWord == 0 {
			t.Errorf("%s: comma menu has %d bold and %d plain cells, want both", look.name, bold, plainWord)
		}
		notice := keys.RenderBar(keys.Context{Footer: "Copied", Style: Config{Color: ColorAlways}}, 132)
		if !strings.Contains(notice, "\x1b[36mCopied") {
			t.Errorf("%s: the notice is not SGR 36: %q", look.name, notice)
		}
	}
	lipgloss.SetColorProfile(termenv.Ascii)
	for name, m := range footerOwners(t, Config{Color: ColorNever}) {
		lines := strings.Split(m.View(), "\n")
		if strings.Contains(lines[len(lines)-1], "\x1b[") {
			t.Errorf("%s: an escape in the footer with colours off", name)
		}
	}
}

// (d) Every popup's footer ends with its way out.
func TestEveryPopupFooterEndsWithItsWayOut(t *testing.T) {
	for name, m := range footerOwners(t, Config{Color: ColorNever}) {
		switch name {
		case "main", "log", "details":
			continue
		}
		pairs := footerPairs(m)
		last := pairs[len(pairs)-1]
		word, key, _ := strings.Cut(last, ": ")
		if word != "Cancel" && word != "Close" && word != "Back" || !strings.Contains(key, "<esc>") {
			t.Errorf("%s: the footer %q does not end with its way out", name, pairs)
		}
	}
}

// (e) The chart chooser at 132x40.
func TestChartChooserFooterExample(t *testing.T) {
	m := footerOwners(t, Config{Color: ColorNever})["open chart"]
	if got, want := lastRow(m.View()), " Tick: <space> | Open: <enter> | Rename: <c-r> | Cancel: <esc>"; got != want {
		t.Errorf("the chart chooser footer is %q, want %q", got, want)
	}
}
