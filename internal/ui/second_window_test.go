package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// A launch that finds the instance lock held by another window runs as a
// second window: it restores the session that window keeps and never writes
// it (cmd/sextile/main.go, Config.SecondWindow).

func sessionDirNames(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(store.SessionDir())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// A second window that changes the selection and a house choice, then quits,
// leaves the session file byte for byte as the first window wrote it.
func TestSecondWindowQuitLeavesTheSessionFile(t *testing.T) {
	sessionSandbox(t)
	stubHouseLookups(t)
	holder := listModel(t, 2, 0)
	if err := store.SaveSession(holder.sessionSnapshot()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.SessionPath())
	if err != nil {
		t.Fatal(err)
	}
	names := sessionDirNames(t)

	m := listModel(t, 2, 0)
	m.cfg.SecondWindow = true
	m, cmd := m.chooseHouseSystem(1, recipe.Regiomontanus)
	m = pump(t, m, cmd)
	m = send(t, m, keySpace)
	if m.slots.SlotSelected(0) == holder.slots.SlotSelected(0) {
		t.Fatal("premise: Space did not change the selection")
	}
	if logHas(m, "Couldn't save") != 0 {
		t.Errorf("a second window reported a session write: %v", m.logLines)
	}
	tm, cmd := m.Update(escRunes("q"))
	if cmd == nil {
		t.Fatal("q did not quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q did not quit")
	}
	if e := tm.(Model).SessionWriteError(); e != nil {
		t.Errorf("a session write was attempted: %v", e)
	}
	after, err := os.ReadFile(store.SessionPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("the second window rewrote the session:\n--- before ---\n%s--- after ---\n%s", before, after)
	}
	if got := sessionDirNames(t); !slices.Equal(got, names) {
		t.Errorf("the session folder changed: %v, was %v", got, names)
	}
}

// A second window reads a refused session file and leaves it where it is:
// moving it aside is the holder's job, and the file is the holder's.
func TestSecondWindowLeavesARefusedSessionInPlace(t *testing.T) {
	sessionSandbox(t)
	const refused = "version: 99\n"
	seedSession(t, refused)
	p := readPerimeterCmd(true)().(ingest.StartupPerimeterReadMsg)
	if p.SessionErr == nil {
		t.Fatal("premise: the session was not refused")
	}
	if got := fileText(t, store.SessionPath()); got != refused {
		t.Errorf("the refused file changed: %q", got)
	}
	if got := sessionDirNames(t); !slices.Equal(got, []string{"session.yaml"}) {
		t.Errorf("the session folder holds %v, want only session.yaml", got)
	}
	// The first window's read does move it aside, so the assertion above is
	// not vacuous.
	readPerimeterCmd(false)()
	if _, err := os.Stat(store.SessionPath()); !os.IsNotExist(err) {
		t.Errorf("control: the first window's read left the refused file (%v)", err)
	}
}

// The second window says so once, on the Log and in the key bar.
func TestSecondWindowSaysSoOnce(t *testing.T) {
	sessionSandbox(t)
	m := NewModel(Config{SecondWindow: true})
	m.width, m.height = 200, 40
	m, _ = applyStartupPerimeter(m, ingest.StartupPerimeterReadMsg{})
	line := "second window: " + secondWindowNote
	if n := logHas(m, line); n != 1 {
		t.Errorf("the Log holds the line %d times, want 1: %v", n, m.logLines)
	}
	if got := m.footerIndicator(); got != noticeSecondWindow {
		t.Errorf("key bar slot = %q, want the notice", got)
	}
	if !strings.Contains(csText(renderKeyBar(200, m.keyBarEntries(), m.cfg, m.footerIndicator())), noticeSecondWindow) {
		t.Error("the key bar does not show the notice at 200 columns")
	}
	first := NewModel(Config{})
	first, _ = applyStartupPerimeter(first, ingest.StartupPerimeterReadMsg{})
	if logHas(first, line) != 0 || first.footerIndicator() != "" {
		t.Error("a first window says it is a second one")
	}
}

// A rename in a second window of a chart whose file names a house system is
// refused, even when this window holds a record for it: the rewrite drops the
// file's house line, and only the first window can keep the system.
func TestSecondWindowRefusesARenameThatNeedsTheSession(t *testing.T) {
	sessionSandbox(t)
	dir := titleLibrary(t, nil, map[string]string{"K.as": kochChart("Client K", true)})
	before := fileText(t, filepath.Join(dir, "K.as"))
	m := openedPicker(t)
	m.cfg.SecondWindow = true
	m.frames.records = []store.FrameRecord{{File: "K.as", When: frameWhenUI, Houses: recipe.Koch, HasHouses: true, From: store.FrameFromChosen}}
	for i, c := range m.picker.matches {
		if c.Source.File == "K.as" {
			m.picker.cursor = i
		}
	}
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	m = typeText(t, m, "Client Kay")
	m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = finishRename(t, m, cmd)
	if got := fileText(t, filepath.Join(dir, "K.as")); got != before {
		t.Errorf("the chart file was rewritten by a window that cannot keep its houses:\n%s", got)
	}
	noBackup(t, filepath.Join(dir, "K.as"))
	if !strings.Contains(m.title.refusal, "first Sextile window") {
		t.Errorf("refusal = %q, want it to name the first window", m.title.refusal)
	}
	if _, err := os.Stat(store.SessionPath()); !os.IsNotExist(err) {
		t.Errorf("a session was written (%v)", err)
	}
}

// A chart whose file names no house system needs nothing kept: a second
// window renames it.
func TestSecondWindowRenamesAChartWithNoHouseLine(t *testing.T) {
	sessionSandbox(t)
	dir := titleLibrary(t, nil, map[string]string{"N.as": kochChart("Client N", false)})
	m := openedPicker(t)
	m.cfg.SecondWindow = true
	for i, c := range m.picker.matches {
		if c.Source.File == "N.as" {
			m.picker.cursor = i
		}
	}
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	m = typeText(t, m, "Client Nan")
	m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	_, _ = finishRename(t, m, cmd)
	if got := fileText(t, filepath.Join(dir, "N.as")); !strings.Contains(got, "Client Nan") {
		t.Errorf("the rename did not happen:\n%s", got)
	}
	if _, err := os.Stat(store.SessionPath()); !os.IsNotExist(err) {
		t.Errorf("a session was written (%v)", err)
	}
}
