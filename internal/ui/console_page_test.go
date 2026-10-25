package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// A6: the Log console pages its cursor by the one rule: a 60-line snapshot, the
// cursor at the newest line as the console opens.
func TestConsolePageKeys(t *testing.T) {
	// A terminal short enough for a 12-row console (the smallest there is) over 60 lines.
	m := consoleModel(t, 60)
	m.height = 15
	m, _ = press(t, m, ",", "l")
	if !m.console.open {
		t.Fatal("the console did not open")
	}
	v := m.consoleInnerHeight()
	total := len(m.console.snapshot)
	if v != 12 || total < 3*v {
		t.Fatalf("fixture: %d lines in a %d-row console", total, v)
	}
	if m.console.cursor != total-1 {
		t.Fatalf("fixture: opened with the cursor on %d, want the newest line %d", m.console.cursor, total-1)
	}
	at := func(step string, wantCursor, wantOffset int) {
		t.Helper()
		if m.console.cursor != wantCursor || m.console.offset != wantOffset {
			t.Fatalf("%s: cursor %d offset %d, want %d / %d (%d lines, %d rows)", step, m.console.cursor, m.console.offset, wantCursor, wantOffset, total, v)
		}
		if start, _ := consoleWindow(total, m.console.offset, v); start != m.console.offset {
			t.Errorf("%s: the console draws from %d, %d stored", step, start, m.console.offset)
		}
	}
	// Opened on the newest line with the window the margin mover left: PgUp goes
	// to the window's first row and the window stays.
	opened := m.console.offset
	m, _ = press(t, m, "pgup")
	at("PgUp from the newest line", opened, opened)
	m, _ = press(t, m, "pgup")
	at("PgUp again pages the window up", opened-(v-1), opened-(v-1))
	m, _ = press(t, m, "pgdown")
	at("PgDn goes to the window's last row", opened-(v-1)+v-1, opened-(v-1))
	m, _ = press(t, m, "pgdown")
	at("PgDn again pages the window down", opened+v-1, opened)
	m, _ = press(t, m, "home")
	at("Home", 0, 0)
	m, _ = press(t, m, "end")
	at("End", total-1, total-v)
	m, _ = press(t, m, "g")
	at("g", 0, 0)
	m, _ = press(t, m, "G")
	at("G", total-1, total-v)
	// The console still owns the keyboard: nothing here reached a panel.
	if !m.console.open {
		t.Error("a paging key closed the console")
	}

	// Enter copies the line under the cursor wherever paging left it.
	if m.console.snapshot[m.console.cursor].text != "command 59" {
		t.Errorf("the cursor is on %q, want the newest line", m.console.snapshot[m.console.cursor].text)
	}
}

// A6: a console that lost its inner height to a resize keeps the cursor in
// range and never panics.
func TestConsolePageKeysWithNoRows(t *testing.T) {
	m := openedConsole(t, 60)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 6})
	m = next.(Model)
	if h := m.consoleInnerHeight(); h != 0 {
		t.Fatalf("fixture: the console has %d rows after the resize, want none", h)
	}
	total := len(m.console.snapshot)
	keys := []string{"pgup", "pgdown", "end", "home", "G", "g", "pgdown", "pgdown", "pgup"}
	// Held long enough that a page which moves by the (zero) height would walk off the list.
	for i := 0; i < 2*total; i++ {
		keys = append(keys, "pgup")
	}
	keys = append(keys, "end")
	for i := 0; i < 2*total; i++ {
		keys = append(keys, "pgdown")
	}
	for i, k := range keys {
		m, _ = press(t, m, k)
		if m.console.cursor < 0 || m.console.cursor >= total || m.console.offset < 0 || m.console.offset >= total {
			t.Fatalf("press %d (%s): cursor %d offset %d outside %d lines", i, k, m.console.cursor, m.console.offset, total)
		}
	}
}
