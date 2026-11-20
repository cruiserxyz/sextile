package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/store"
)

// All chart values in this file are constructed synthetic test inputs.

// specs/session-panel.md: a one-line panel above the Chart panel, the numbers
// that follow from it, and the Details of the Session panel.

// screenLines is the frame as plain text, one string per row.
func screenLines(m Model) []string { return strings.Split(ansi.Strip(m.View()), "\n") }

// sessionPanelRow is the row of the Session panel's line: the first row under
// its title, or the title row itself while the panel is folded.
func sessionPanelRow(t *testing.T, m Model) string {
	t.Helper()
	r, ok := m.geometry()[PanelStatus]
	if !ok {
		t.Fatal("the layout has no Session panel")
	}
	lines := screenLines(m)
	if r.h == 1 {
		return lines[r.y]
	}
	return lines[r.y+1]
}

// 1: the Session panel stands above the Chart panel and holds one line, with
// the Session panel in focus (at rest it is open with the Chart panel).
func TestSessionPanelStandsAboveTheChartPanel(t *testing.T) {
	for _, size := range [][2]int{{144, 40}, {80, 40}, {132, 30}} {
		m := sessionsModel(t, 1)
		m.cfg = Config{Color: ColorNever}
		m.width, m.height = size[0], size[1]
		m.focused = PanelStatus
		geo := m.geometry()
		if geo[PanelStatus].y != 0 || geo[PanelChart].y != geo[PanelStatus].y+geo[PanelStatus].h || geo[PanelStatus].h != 3 {
			t.Errorf("%dx%d: Session %+v, Chart %+v, want Session on row 0, three rows tall, Chart right below", size[0], size[1], geo[PanelStatus], geo[PanelChart])
		}
		lines := screenLines(m)
		if !strings.Contains(lines[0], "[1] Status") || !strings.Contains(lines[3], "[2] Chart") {
			t.Errorf("%dx%d: titles %q and %q, want [1] Status over [2] Chart", size[0], size[1], lines[0], lines[3])
		}
		if want := "unnamed → ~/charts"; !strings.Contains(lines[1], want) {
			t.Errorf("%dx%d: the Session line is %q, want it to hold %q", size[0], size[1], lines[1], want)
		}
	}
}

// 2: the panels are numbered 1 to 4 and 0, and the keys follow the numbers.
func TestPanelKeysFollowThePanelNumbers(t *testing.T) {
	for _, tc := range []struct {
		key   string
		panel PanelID
		title string
	}{{"1", PanelStatus, "[1] Status"}, {"2", PanelChart, "[2] Chart"}, {"3", PanelBodies, "[3] Bodies"}, {"4", PanelAspects, "[4] Aspects"}} {
		m := withFrame(sessionsModel(t, 1))
		m.width, m.height = 80, 40
		m.focused = PanelAspects
		if tc.panel == PanelAspects {
			m.focused = PanelChart
		}
		m, _ = press(t, m, tc.key)
		if m.focused != tc.panel {
			t.Errorf("key %s: focus %q, want %q", tc.key, m.focused, tc.panel)
		}
		if !strings.Contains(strings.Join(screenLines(m), "\n"), tc.title) {
			t.Errorf("key %s: the screen has no %s", tc.key, tc.title)
		}
	}
	// 0 is Details' number: it never moves the focus.
	m := withFrame(sessionsModel(t, 1))
	m.width, m.height = 160, 40
	m.focused = PanelBodies
	m, _ = press(t, m, "0")
	if m.focused != PanelBodies || !strings.Contains(strings.Join(screenLines(m), "\n"), "[0] ") {
		t.Errorf("0: focus %q, want Bodies kept and a [0] panel on screen", m.focused)
	}
}

// 2: Tab and Shift+Tab, and the arrows, walk Session, Chart, Bodies, Aspects.
func TestTabWalksSessionChartBodiesAspects(t *testing.T) {
	m := withFrame(sessionsModel(t, 1))
	m.focused = PanelStatus
	var forward []PanelID
	for i := 0; i < 5; i++ {
		m, _ = press(t, m, "tab")
		forward = append(forward, m.focused)
	}
	want := []PanelID{PanelChart, PanelBodies, PanelAspects, PanelStatus, PanelChart}
	if strings.Join(panelNames(forward), " ") != strings.Join(panelNames(want), " ") {
		t.Errorf("Tab from Session walks %v, want %v", forward, want)
	}
	m.focused = PanelStatus
	m = sendKeys(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focused != PanelAspects {
		t.Errorf("Shift+Tab from Session lands on %q, want Aspects", m.focused)
	}
	m = sendKeys(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.focused != PanelStatus {
		t.Errorf("Right from Aspects lands on %q, want Session", m.focused)
	}
	m = sendKeys(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.focused != PanelAspects {
		t.Errorf("Left from Session lands on %q, want Aspects", m.focused)
	}
}

func panelNames(ps []PanelID) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = string(p)
	}
	return out
}

// 5: the line is true at every moment: it follows save as, a rename of the
// running session, a new session and another session opened.
func TestSessionLineFollowsEverySessionChange(t *testing.T) {
	m := armedModel(t)
	m.cfg = Config{Accessible: true, Color: ColorNever}
	line := func(want string) {
		t.Helper()
		if got := sessionPanelRow(t, m); !strings.Contains(got, want+" ") && !strings.Contains(got, want+"│") {
			t.Fatalf("the Session line is %q, want it to read %q", strings.TrimSpace(got), want)
		}
	}
	line("unnamed -> ~/charts")

	m = saveAs(t, m, "Mine")
	line("Mine -> ~/charts")

	m = openSessions(t, m)
	for i, r := range m.sessions.Rows {
		if r.File == m.named.file {
			m.sessions.Cursor = i + 2
		}
	}
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	m = clearField(t, m, 10)
	m = typeText(t, m, "Renamed")
	m, _ = press(t, m, "enter", "esc")
	line("Renamed -> ~/charts")

	m, cmd := pressNewSession(t, m)
	m = pump(t, m, cmd)
	line("unnamed -> ~/charts")

	other := otherSession(t, "Other work", "Client Zed")
	m = openSessions(t, m)
	for i, r := range m.sessions.Rows {
		if r.File == other {
			m.sessions.Cursor = i + 2
		}
	}
	m, cmd = press(t, m, "enter")
	if cmd == nil {
		t.Fatal("Enter on a saved session started nothing")
	}
	tm, _ := m.Update(cmd())
	m = tm.(Model)
	line("Other work -> ~/charts")
}

// 5: the trash leaves the line as it was and takes the trashed session off the
// Details list.
func TestSessionLineAndListAfterTheTrash(t *testing.T) {
	m := sessionsModel(t, 1)
	m.cfg = Config{Accessible: true, Color: ColorNever}
	otherSession(t, "Old", "Client Zed")
	m = openSessions(t, m)
	m.sessions.Cursor = 2
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
	m, _ = press(t, m, "y")
	m, _ = press(t, m, "esc")
	if got := sessionPanelRow(t, m); !strings.Contains(got, "unnamed -> ~/charts") {
		t.Errorf("after the trash the line is %q", strings.TrimSpace(got))
	}
	m.focused = PanelStatus
	m = m.refreshSessionInfo()
	if got := m.sessionDetails(); strings.Contains(got, "Old") {
		t.Errorf("the trashed session is still listed:\n%s", got)
	}
}

// 3: with the Status panel in focus, the Session tab of Details shows the saved
// sessions of this folder with the running one marked and the folder's position.
func TestSessionDetailsText(t *testing.T) {
	m := sessionsModel(t, 1)
	m.cfg = Config{Color: ColorNever, Version: "0.1.4", Copyright: fixtureCopyright}
	otherSession(t, "Anna, returns", "Client Zed")
	otherSession(t, "Q and A", "Client Zed")
	m.width, m.height = 160, 40
	m, _ = press(t, m, "1")
	got := m.sessionDetails()
	for _, want := range []string{
		"Sessions in ~/charts",
		"● unnamed (this window)",
		"Anna, returns",
		"Q and A",
		"Charts folder: ~/charts",
		"Sextile asks before each write.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the Session details lack %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Walter D. Pullen") {
		t.Errorf("the Session tab carries the credits:\n%s", got)
	}
	// On screen, in the Details panel, at 160x40 where Details stands beside:
	// Credits first, the Session tab after `]`.
	m, _ = press(t, m, "]")
	if view := strings.Join(screenLines(m), "\n"); !strings.Contains(view, "Sessions in ~/charts") || !strings.Contains(view, "[0] Credits - [Session]") {
		t.Errorf("the screen does not show the Session tab under a [0] Credits - [Session] title:\n%s", view)
	}
	// A running named session is the marked row, and `unnamed` is no longer listed.
	m = saveAs(t, m, "Mine")
	m = m.refreshSessionInfo()
	got = m.sessionDetails()
	if !strings.Contains(got, "● Mine") || strings.Contains(got, "unnamed (this window)") {
		t.Errorf("the running session is not the marked one:\n%s", got)
	}
}

// 4: Enter opens the Session panel's Details, as on every panel, and `, O`
// stays the sessions list.
func TestEnterOnTheSessionPanelOpensItsDetails(t *testing.T) {
	m := sessionsModel(t, 1)
	m.width, m.height = 80, 24
	m, _ = press(t, m, "1", "enter")
	if m.startup != startupReady || !m.details.attached || m.focused != PanelStatus {
		t.Fatalf("Enter on Session: startup %q, attached %v, focus %q", m.startup, m.details.attached, m.focused)
	}
	if d, s := m.geometry()[PanelDetails], m.geometry()[PanelStatus]; d.y != s.y+s.h {
		t.Errorf("Details %+v is not right below the Session panel %+v", d, s)
	}
	m, _ = press(t, m, "]")
	if view := strings.Join(screenLines(m), "\n"); !strings.Contains(view, "Sessions in ~/charts") {
		t.Errorf("Details shows no sessions:\n%s", view)
	}
	m, _ = press(t, m, "esc")
	m = openSessions(t, m)
}

// 8: the panel writes no file. A walk through every focus key, Enter, Esc, `0`
// and a resize leaves the state folders and the charts folder byte for byte as
// they were.
func TestSessionPanelWritesNoFile(t *testing.T) {
	m := sessionsModel(t, 1)
	otherSession(t, "Anna", "Client Zed")
	m.width, m.height = 80, 24
	before := folderDigest(t, os.Getenv("HOME"))
	for _, k := range []string{"1", "enter", "esc", "2", "3", "enter", "4", "esc", "tab", "tab", "enter", "0", "esc", "1", "0", "esc"} {
		m, _ = press(t, m, k)
	}
	m = sendKeys(t, m, tea.WindowSizeMsg{Width: 160, Height: 40}, tea.WindowSizeMsg{Width: 66, Height: 15})
	m, _ = press(t, m, "1", "enter", "esc")
	if after := folderDigest(t, os.Getenv("HOME")); after != before {
		t.Errorf("a file changed while the Session panel was used:\n--- before ---\n%s--- after ---\n%s", before, after)
	}
	// The control: a write in the charts folder shows in the digest.
	if err := os.MkdirAll(store.ChartsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.ChartsDir(), "new.as"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if folderDigest(t, os.Getenv("HOME")) == before {
		t.Error("the digest did not see a new file")
	}
}

// folderDigest is every file under root with its size, mode, time and hash,
// one per line, sorted.
func folderDigest(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		sum := ""
		if info.Mode().IsRegular() {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			h := sha256.Sum256(b)
			sum = hex.EncodeToString(h[:6])
		}
		rel, _ := filepath.Rel(root, path)
		lines = append(lines, strings.Join([]string{rel, info.Mode().String(), info.ModTime().UTC().Format("15:04:05.000000000"), sum}, "|"))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}
