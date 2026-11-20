package ui

import (
	"strings"
	"testing"
)

// (d) The focused Log's footer offers no Console entry; Keybindings keeps it.
func TestFocusedLogFooterHasNoConsole(t *testing.T) {
	m := reachOwner(t, keyOwnerCases()[9], Config{Color: ColorNever}, 144, 40)
	if m.currentOwner() != ownerLog {
		t.Fatalf("owner %d, want the Log", m.currentOwner())
	}
	if row := lastRow(m.View()); strings.Contains(row, "Console") {
		t.Errorf("the Log's footer names the console: %q", row)
	}
	rowAt(t, openedKeybindings(kbModel(t)), "Log console")
}
