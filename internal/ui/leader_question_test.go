package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// `, p` is retired: nothing in the command line is p, and pressing it does
// nothing. Ctrl+P and ? open Keybindings as before.
func TestCommaPDoesNothing(t *testing.T) {
	for _, c := range keys.Commands() {
		if c.Letter == "p" {
			t.Fatalf("the command line still has a p row: %+v", c)
		}
	}
	m := leaderModel(t)
	before := overSnapshotMain(m)
	m, _ = tpress(t, m, runes(","))
	m, _ = tpress(t, m, runes("p"))
	if after := overSnapshotMain(m); after != before || m.keybindingsOpen {
		t.Errorf(", p changed something:\n%s\n%s", before, after)
	}
	if m, _ = tpress(t, leaderModel(t), keyCtrlP); !m.keybindingsOpen {
		t.Error("Ctrl+P no longer opens Keybindings")
	}
}

func overSnapshotMain(m Model) string {
	return strings.Join([]string{string(m.startup), string(rune('0' + int(m.currentOwner()))), boolStr(m.keybindingsOpen), boolStr(m.leader.pending)}, "|")
}

func boolStr(b bool) string {
	if b {
		return "y"
	}
	return "n"
}

// In the command line `?` closes it and opens Keybindings, over the main screen
// and over a Details search line alike; it is never listed in the menu.
func TestQuestionMarkInTheCommandLineOpensKeybindings(t *testing.T) {
	m := leaderModel(t)
	m, _ = tpress(t, m, runes(","))
	menu := strings.Split(strings.TrimSpace(ansi.Strip(keys.RenderBar(m.keysContext(), 140))), "  ")
	for _, e := range menu {
		if strings.HasPrefix(e, "? ") || e == "?" || strings.HasPrefix(e, "p ") {
			t.Errorf("the comma menu lists %q", e)
		}
	}
	m, _ = tpress(t, m, runes("?"))
	if m.leader.pending || !m.keybindingsOpen {
		t.Errorf("? in the command line: pending %v, Keybindings open %v", m.leader.pending, m.keybindingsOpen)
	}

	for _, c := range overCases() {
		if !offersCopy(c.name) {
			continue
		}
		s := c.open(t)
		s, _ = tpress(t, s, runes(","))
		if !s.leader.pending {
			t.Fatalf("%s: no command line", c.name)
		}
		s, _ = tpress(t, s, runes("?"))
		if s.leader.pending || !s.keybindingsOpen {
			t.Errorf("%s: ? in the command line: pending %v, Keybindings open %v", c.name, s.leader.pending, s.keybindingsOpen)
		}
	}
}
