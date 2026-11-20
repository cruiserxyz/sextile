package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/store"
)

// The comma is a Ctrl key without Ctrl: over a list that binds Ctrl+R the menu
// names `r rename`, and the leader then r runs what the key runs.
func TestCommaRRenamesTheChartUnderTheCursor(t *testing.T) {
	var c overCase
	for _, k := range overCases() {
		if k.name == "chart list with a file" {
			c = k
		}
	}
	m := c.open(t)
	m, _ = tpress(t, m, runes(","))
	got := strings.Split(barOf(m), "  ")
	slices.Sort(got)
	if want := []string{"<esc>", "r rename"}; !slices.Equal(got, want) {
		t.Fatalf("the menu over the chart list reads %q, want %q", got, want)
	}
	m, _ = tpress(t, m, runes("r"))
	if m.currentOwner() != ownerChartTitle || m.title.Fields.Value(0) != "Kepler" {
		t.Errorf(", r: owner %v, title field %q, want the rename form of Kepler", m.currentOwner(), m.title.Fields.Value(0))
	}
}

// Over Sessions the two Ctrl keys have comma twins that do what the keys do.
func TestCommaTwinsOfTheSessionsKeysDoWhatTheKeysDo(t *testing.T) {
	var c overCase
	for _, k := range overCases() {
		if k.name == "sessions" {
			c = k
		}
	}
	state := func(m Model) string {
		return fmt.Sprintf("%s|%v|%d", overSnapshot(m, c), m.sessions.Asking, len(m.sessions.Rows))
	}
	for _, tc := range []struct{ letter, ctrl, word string }{{"r", "ctrl+r", "rename"}, {"d", "ctrl+d", "delete"}} {
		t.Run(tc.word, func(t *testing.T) {
			// Each path gets a world of its own: the first one trashes the file.
			viaKey := c.open(t)
			before := state(viaKey)
			viaKey, _ = tpress(t, viaKey, pressedKey(tc.ctrl))
			m := c.open(t)
			menu := strings.Split(barOf(func() Model { x, _ := tpress(t, m, runes(",")); return x }()), "  ")
			if !slices.Contains(menu, tc.letter+" "+tc.word) {
				t.Fatalf("the menu %q lacks %q", menu, tc.letter+" "+tc.word)
			}
			viaComma, _ := tpress(t, m, runes(","))
			viaComma, _ = tpress(t, viaComma, runes(tc.letter))
			if want, got := state(viaKey), state(viaComma); got != want {
				t.Errorf(", %s left %q, %s left %q", tc.letter, got, tc.ctrl, want)
			}
			if state(viaComma) == before {
				t.Errorf(", %s did nothing", tc.letter)
			}
		})
	}
}

// `, c w` has left the comma: it does nothing, the `, c` line does not list it,
// and the folder row of Keybindings still runs.
func TestCommaCWDoesNothingAndTheFolderRowRuns(t *testing.T) {
	m, dir := keyWorld(t, store.PositionAsk)
	before := mapShape(m)
	m, _ = press(t, m, ",", "c")
	if line := strings.TrimSpace(barOf(m)); line != "d save defaults  <esc>" {
		t.Errorf("the `, c` line reads %q", line)
	}
	m, _ = press(t, m, "w")
	if mapShape(m) != before || stored(dir) != store.PositionAsk {
		t.Errorf("`, c w` changed something: shape %q (was %q), position %q", mapShape(m), before, stored(dir))
	}
	m = pressFolder(t, m)
	if stored(dir) != store.PositionProtected {
		t.Errorf("the folder row left the position at %q, want protected", stored(dir))
	}
}
