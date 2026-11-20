package keys

import "testing"

// A lower-case letter derives its Ctrl key by one rule: ctrl+letter. A capital
// derives none; the letters the terminal owns are refused by Validate.
func TestCtrlKeyDerivesFromTheLetter(t *testing.T) {
	for _, l := range []string{"o", "s", "l", "z", "a", "b", "x", "y", "w", "p", "u", "h", "c", "m"} {
		if got, want := CtrlKey(l), "ctrl+"+l; got != want {
			t.Errorf("CtrlKey(%q) = %q, want %q", l, got, want)
		}
	}
	for _, l := range []string{"O", "L", "A", "Z", "", "oo", "1"} {
		if got := CtrlKey(l); got != "" {
			t.Errorf("CtrlKey(%q) = %q, want none", l, got)
		}
	}
}

// CommandCtrl is the Ctrl key only of a one-key lower-case row the terminal
// does not own.
func TestCommandCtrlRefusesWhatTheTerminalOwns(t *testing.T) {
	if k, ok := CommandCtrl(Command{Letter: "o"}); !ok || k != "ctrl+o" {
		t.Errorf("o: %q %v", k, ok)
	}
	for _, c := range []Command{{Letter: "h"}, {Letter: "c"}, {Letter: "O"}, {Letter: "c", Then: "d"}} {
		if k, ok := CommandCtrl(c); ok {
			t.Errorf("%+v gave %q", c, k)
		}
	}
}

// The table holds twelve one-key rows in the menu's order, most used first, then
// the two-key row `c d`.
func TestCommandsTableShape(t *testing.T) {
	cs := Commands()
	if len(cs) != 13 {
		t.Fatalf("%d commands, want 13", len(cs))
	}
	var order string
	for _, c := range cs[:12] {
		order += c.Letter
		if c.Then != "" {
			t.Errorf("%q has a second key %q", c.Letter, c.Then)
		}
	}
	if order != "osluzabxyOLt" {
		t.Errorf("order %q", order)
	}
	if r := cs[12]; r.Letter != "c" || r.Then != "d" || r.ID != BindingSaveCity || r.Word != "save defaults" || r.PrefixWord != "defaults…" {
		t.Errorf("row 13 %+v, want c d saving the defaults, the group named defaults…", r)
	}
}
