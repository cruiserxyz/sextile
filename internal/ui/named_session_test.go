package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
)

// All chart values in this file are constructed synthetic test inputs.

// Named sessions (`, o`): a window runs exactly one session, the general one
// or a named one, and keeps updating only that one.

// sessionsModel is a ready workspace of n anchored charts in a sandbox.
func sessionsModel(t *testing.T, n int) Model {
	t.Helper()
	sessionSandbox(t)
	return listModel(t, n, 0)
}

// openSessions is `, o`.
func openSessions(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = press(t, m, ",", "o")
	if m.startup != startupSessionsOpen {
		t.Fatalf("`, o` did not open the Sessions list (startup %q)", m.startup)
	}
	return m
}

// clearField deletes what the focused field holds, as Backspace would.
func clearField(t *testing.T, m Model, n int) Model {
	t.Helper()
	for i := 0; i < n; i++ {
		m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	return m
}

// saveAs is `, o`, Enter on the first row, the name typed over the prefill,
// Enter.
func saveAs(t *testing.T, m Model, name string) Model {
	t.Helper()
	m = openSessions(t, m)
	m, _ = press(t, m, "enter")
	m = clearField(t, m, 80)
	m = typeText(t, m, name)
	m, _ = press(t, m, "enter")
	return m
}

// otherSession saves a named session holding one chart, in this charts
// folder, as another window would have, and lets go of its lock.
func otherSession(t *testing.T, name, chart string) string {
	t.Helper()
	s := store.Session{
		Folder: store.FolderID(store.ChartsDir()),
		Slots: []store.SessionSlot{{
			Chart: store.SessionChart{ID: "zed", Name: chart, When: time.Date(1977, 7, 7, 7, 7, 0, 0, time.UTC),
				Place: location.PlaceInfo{Name: "Testford", Country: "TS", Lat: 41, Lon: -71, TZ: "America/New_York"}},
			Usable: true, Selected: true,
		}},
	}
	l, err := store.SaveNamedSessionAs(name, s)
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	return l.File()
}

func anchoredNames(m Model) []string {
	var out []string
	for i := 0; i < m.slots.AnchoredCount(); i++ {
		out = append(out, m.slots.All()[i].Chart.Name)
	}
	return out
}

// The name is prefilled with the first SELECTED chart's title in the Chart
// panel's order; else the first chart's; else nothing.
func TestPrefillIsFirstSelectedElseFirstChart(t *testing.T) {
	m := sessionsModel(t, 3)
	m.slots.SetSlotSelected(0, false)
	m.slots.SetSlotSelected(2, true)
	m.slots.SetSlotSelected(1, true)
	prefill := func(m Model) string {
		t.Helper()
		m = openSessions(t, m)
		if m.sessions.cursor != 0 {
			t.Fatalf("the list opens on row %d, want the save row", m.sessions.cursor)
		}
		m, _ = press(t, m, "enter")
		if m.startup != startupSessionNameOpen {
			t.Fatalf("Enter on the first row did not open the name field (startup %q)", m.startup)
		}
		return m.sessions.form.value(0)
	}
	if got := prefill(m); got != "Synthetic 2" {
		t.Errorf("prefill = %q, want the first selected chart in panel order, Synthetic 2", got)
	}
	m.slots.SetSlotSelected(1, false)
	m.slots.SetSlotSelected(2, false)
	if got := prefill(m); got != "Synthetic 1" {
		t.Errorf("prefill with nothing selected = %q, want the first chart, Synthetic 1", got)
	}
	empty := sessionsModel(t, 0)
	if got := prefill(empty); got != "" {
		t.Errorf("prefill with no chart = %q, want empty", got)
	}
}

// After Save as, every place that saved the general session saves the named
// one instead -- a house choice at once, the quit -- and the general session
// stays as it was at that moment.
func TestWindowKeepsUpdatingItsNamedSession(t *testing.T) {
	m := sessionsModel(t, 2)
	stubHouseLookups(t)
	m = saveAs(t, m, "Client work")
	if m.startup != startupReady {
		t.Fatalf("the save did not close the popup (startup %q, refusal %q)", m.startup, m.sessions.refusal)
	}
	named := store.NamedSessionPath("client-work.yaml")
	if _, err := os.Stat(named); err != nil {
		t.Fatalf("no named file: %v", err)
	}
	general := fileText(t, store.SessionPath())

	m, cmd := m.chooseHouseSystem(1, recipe.Regiomontanus)
	m = pump(t, m, cmd)
	if got := fileText(t, named); !strings.Contains(got, "houses: "+recipe.Regiomontanus.StoredName()) {
		t.Errorf("the house choice did not reach the named session:\n%s", got)
	}
	m = send(t, m, keySpace) // the cursor's chart: deselect it
	tm, _ := m.Update(escRunes("q"))
	if e := tm.(Model).SessionWriteError(); e != nil {
		t.Fatalf("quit: %v", e)
	}
	got := fileText(t, named)
	if !strings.Contains(got, "name: Client work\n") || strings.Count(got, "selected: true") != 1 {
		t.Errorf("the quit did not write the named session as the window holds it:\n%s", got)
	}
	if after := fileText(t, store.SessionPath()); after != general {
		t.Errorf("the general session changed after Save as:\n--- at save as ---\n%s--- after ---\n%s", general, after)
	}
}

// Enter on a saved session saves this window's session first, then the
// chosen one's charts, selection and frames replace the open ones, and the
// window runs it from then on.
func TestOpenNamedSessionReplacesCharts(t *testing.T) {
	m := sessionsModel(t, 2)
	file := otherSession(t, "Other work", "Client Zed")
	m = openSessions(t, m)
	if len(m.sessions.rows) != 1 || m.sessions.rows[0].File != file {
		t.Fatalf("listed %+v, want %s", m.sessions.rows, file)
	}
	m, _ = press(t, m, "down")
	m, cmd := press(t, m, "enter")
	if cmd == nil {
		t.Fatal("Enter on a saved session started nothing")
	}
	// This window's general session was saved before the switch.
	g, ok, err := store.LoadSession()
	if err != nil || !ok || len(g.Slots) != 2 {
		t.Fatalf("the general session was not saved first: ok %v err %v %+v", ok, err, g.Slots)
	}
	tm, _ := m.Update(cmd())
	m = tm.(Model)
	if got := anchoredNames(m); !slices.Equal(got, []string{"Client Zed"}) {
		t.Errorf("open charts = %v, want the session's [Client Zed]", got)
	}
	if !m.slots.SlotSelected(0) {
		t.Error("the session's selection did not come back")
	}
	if m.named.file != file {
		t.Errorf("the window runs %q, want %q", m.named.file, file)
	}
	m.cursorIdx = 0
	m = send(t, m, keySpace)
	tm, _ = m.Update(escRunes("q"))
	if e := tm.(Model).SessionWriteError(); e != nil {
		t.Fatalf("quit: %v", e)
	}
	if got := fileText(t, store.NamedSessionPath(file)); !strings.Contains(got, "Client Zed") || strings.Contains(got, "selected: true") {
		t.Errorf("the quit did not write the opened session:\n%s", got)
	}
}

// A session another window runs is marked so, and cannot be opened, renamed
// or deleted from here.
func TestNamedSessionOpenElsewhereIsRefused(t *testing.T) {
	m := sessionsModel(t, 1)
	file := otherSession(t, "Busy", "Client Zed")
	_, _, other, err := store.OpenNamedSession(file, store.FolderID(store.ChartsDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Release()
	before := fileText(t, store.NamedSessionPath(file))

	m = openSessions(t, m)
	if len(m.sessions.rows) != 1 || !m.sessions.rows[0].OpenElsewhere {
		t.Fatalf("rows = %+v, want Busy marked open elsewhere", m.sessions.rows)
	}
	m.width, m.height = 80, 24
	if view := csText(m.View()); !strings.Contains(view, "in another window") {
		t.Errorf("the list does not say the session is open elsewhere:\n%s", view)
	}
	m, _ = press(t, m, "down")
	m, cmd := press(t, m, "enter")
	if cmd != nil || m.named.file != "" || !slices.Equal(anchoredNames(m), []string{"Synthetic 1"}) {
		t.Errorf("Enter opened a session another window runs (named %q, charts %v)", m.named.file, anchoredNames(m))
	}
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	if m.startup != startupSessionsOpen {
		t.Errorf("Ctrl+R opened a rename of a session another window runs (startup %q)", m.startup)
	}
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
	if got := fileText(t, store.NamedSessionPath(file)); got != before {
		t.Error("the session changed")
	}
	if names, _ := os.ReadDir(filepath.Join(store.SessionsDir(), ".trash")); len(names) != 0 {
		t.Errorf("Ctrl+D trashed a session another window runs: %v", names)
	}
	if logHas(m, "session: Busy is open in another window") < 3 {
		t.Errorf("the three refusals are not each said: %v", m.logLines)
	}
}

// Ctrl+R renames from the list (a taken name refused in the field) and Ctrl+D
// moves a session to the trash with one Log line naming its full path; the
// session this window runs is not trashed.
func TestSessionsListRenamesAndTrashes(t *testing.T) {
	m := sessionsModel(t, 1)
	otherSession(t, "Old", "Client Zed")
	otherSession(t, "Taken", "Client Zed")
	m = saveAs(t, m, "Mine")

	m = openSessions(t, m)
	row := func(file string) {
		t.Helper()
		for i, r := range m.sessions.rows {
			if r.File == file {
				m.sessions.cursor = i + 1
				return
			}
		}
		t.Fatalf("%s is not listed: %+v", file, m.sessions.rows)
	}
	row("old.yaml")
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	if m.startup != startupSessionNameOpen || m.sessions.form.value(0) != "Old" {
		t.Fatalf("Ctrl+R: startup %q, field %q", m.startup, m.sessions.form.value(0))
	}
	m = clearField(t, m, 10)
	m = typeText(t, m, "taken")
	m, _ = press(t, m, "enter")
	if m.startup != startupSessionNameOpen || m.sessions.refusal == "" {
		t.Errorf("a taken name was not refused in the field (startup %q)", m.startup)
	}
	m = clearField(t, m, 10)
	m = typeText(t, m, "Older")
	m, _ = press(t, m, "enter")
	if _, err := os.Stat(store.NamedSessionPath("older.yaml")); err != nil {
		t.Fatalf("not renamed: %v", err)
	}
	if m.startup != startupSessionsOpen {
		t.Errorf("after the rename the list is not back (startup %q)", m.startup)
	}

	row("mine.yaml")
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
	if _, err := os.Stat(store.NamedSessionPath("mine.yaml")); err != nil {
		t.Errorf("the running session was trashed: %v", err)
	}
	if logHas(m, "Switch to another session first") != 1 {
		t.Errorf("the refusal was not said: %v", m.logLines)
	}

	row("older.yaml")
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
	trash, _ := filepath.Glob(filepath.Join(store.SessionsDir(), ".trash", "older-*.yaml"))
	if len(trash) != 1 {
		t.Fatalf("trash holds %v", trash)
	}
	if logHas(m, trash[0]) != 1 {
		t.Errorf("no Log line names %s: %v", trash[0], m.logLines)
	}
	for _, r := range m.sessions.rows {
		if r.File == "older.yaml" {
			t.Error("the trashed session is still listed")
		}
	}
}

// A second window (the general session's lock is held) starts empty and keeps
// nothing -- until Save as, after which its session is persistent.
func TestSecondWindowStartsEmptyThenSaveAsPersists(t *testing.T) {
	sessionSandbox(t)
	holder := listModel(t, 2, 0)
	if err := store.SaveSession(holder.sessionSnapshot()); err != nil {
		t.Fatal(err)
	}
	general := fileText(t, store.SessionPath())

	p := readPerimeterCmd(true)().(ingest.StartupPerimeterReadMsg)
	if p.HasSession || p.SessionErr != nil {
		t.Fatalf("a second window read the general session: has %v err %v", p.HasSession, p.SessionErr)
	}
	p.LocationCandidate = location.Candidate{Source: "stored-location", Place: berlinRaw}
	var tm tea.Model = NewModelWithClock(Config{Accessible: true, SecondWindow: true}, FrozenClock(sessionFrozen))
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(p)
	m := tm.(Model)
	if n := m.slots.AnchoredCount(); n != 0 {
		t.Fatalf("a second window started with %d chart(s)", n)
	}
	if !strings.Contains(m.footerIndicator(), "named session") {
		t.Errorf("the notice does not say how to keep this window: %q", m.footerIndicator())
	}
	m.startup = startupReady
	m.focused = PanelChart
	if err := m.slots.AddAnchored(store.SessionChart{ID: "x", Name: "Client X", When: fixtureOneWhen, Place: fixtureOneP}, fixtureOneWhen, fixtureOneP); err != nil {
		t.Fatal(err)
	}

	quitNow := func(m Model) {
		t.Helper()
		tm, _ := m.Update(escRunes("q"))
		if e := tm.(Model).SessionWriteError(); e != nil {
			t.Fatalf("quit: %v", e)
		}
	}
	quitNow(m)
	if got := fileText(t, store.SessionPath()); got != general {
		t.Error("a second window wrote the general session")
	}
	if entries, _ := os.ReadDir(store.SessionsDir()); len(entries) != 0 {
		t.Errorf("a second window wrote a named session unasked: %v", entries)
	}

	m = saveAs(t, m, "Second desk")
	if m.startup != startupReady {
		t.Fatalf("save as did not close (startup %q, refusal %q)", m.startup, m.sessions.refusal)
	}
	m.cursorIdx = 0
	m = send(t, m, keySpace)
	quitNow(m)
	got := fileText(t, store.NamedSessionPath("second-desk.yaml"))
	if !strings.Contains(got, "Client X") || !strings.Contains(got, "selected: true") {
		t.Errorf("the second window's named session was not kept:\n%s", got)
	}
	if fileText(t, store.SessionPath()) != general {
		t.Error("the second window wrote the general session after Save as")
	}
}

// The Keys popup lists `, o` with its description.
func TestHelpListsSessions(t *testing.T) {
	m := sessionsModel(t, 1)
	for _, l := range m.helpLines() {
		if strings.Contains(l.text, "Sessions") && strings.Contains(l.text, ",o") {
			return
		}
	}
	var texts []string
	for _, l := range m.helpLines() {
		texts = append(texts, l.text)
	}
	t.Errorf("the Keys popup does not list `,o` Sessions:\n%s", strings.Join(texts, "\n"))
}

// A launch main started with --session reads that named session, not the
// general one, and keeps updating it.
func TestSessionFlagStartsTheNamedSession(t *testing.T) {
	sessionSandbox(t)
	holder := listModel(t, 2, 0)
	if err := store.SaveSession(holder.sessionSnapshot()); err != nil {
		t.Fatal(err)
	}
	file := otherSession(t, "Flag work", "Client Zed")
	_, name, lock, err := store.OpenNamedSession(file, store.FolderID(store.ChartsDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	m := NewModelWithClock(Config{Accessible: true, NamedSession: lock, NamedSessionName: name}, FrozenClock(sessionFrozen))
	p := m.perimeterCmd()().(ingest.StartupPerimeterReadMsg)
	if !p.HasSession || len(p.Session.Slots) != 1 || p.Session.Slots[0].Chart.Name != "Client Zed" {
		t.Fatalf("the launch read %+v, want the named session", p.Session)
	}
	if m.named.file != file || !m.ownsSession() {
		t.Errorf("the window runs %q (owns %v)", m.named.file, m.ownsSession())
	}
}

// A --session launch whose named file became unreadable between main's
// admission and the startup read keeps the file exactly as it is: the quit
// writes nothing over it, as a refused general session is kept.
func TestSessionFlagRereadRefusedLeavesTheFile(t *testing.T) {
	sessionSandbox(t)
	file := otherSession(t, "Flag work", "Client Zed")
	_, name, lock, err := store.OpenNamedSession(file, store.FolderID(store.ChartsDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	// Replaced after admission, before the read: a later Sextile's file.
	const later = "version: 99\nname: Flag work\n"
	if err := os.WriteFile(store.NamedSessionPath(file), []byte(later), 0o600); err != nil {
		t.Fatal(err)
	}
	var tm tea.Model = NewModelWithClock(Config{Accessible: true, NamedSession: lock, NamedSessionName: name}, FrozenClock(sessionFrozen))
	p := tm.(Model).perimeterCmd()().(ingest.StartupPerimeterReadMsg)
	if p.SessionErr == nil {
		t.Fatal("premise: the startup read did not refuse the file")
	}
	p.LocationCandidate = location.Candidate{Source: "stored-location", Place: berlinRaw}
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(p)
	m := tm.(Model)
	if err := m.slots.AddAnchored(store.SessionChart{ID: "x", Name: "Client X", When: fixtureOneWhen, Place: fixtureOneP}, fixtureOneWhen, fixtureOneP); err != nil {
		t.Fatal(err)
	}
	tm, _ = m.Update(escRunes("q"))
	if got := fileText(t, store.NamedSessionPath(file)); got != later {
		t.Errorf("the quit wrote over the refused named session:\n%s", got)
	}
	if _, err := os.Stat(store.SessionPath()); !os.IsNotExist(err) {
		t.Errorf("the quit wrote the general session instead (%v)", err)
	}
}

// A running named session whose file is changed outside Sextile is left as it
// is: neither an autosave nor the quit writes over it, one Log line says so,
// and Save as still works.
func TestNamedSessionChangedOutsideIsLeftAlone(t *testing.T) {
	m := sessionsModel(t, 1)
	stubHouseLookups(t)
	l, err := store.SaveNamedSessionAs("Other work", store.Session{
		Folder: store.FolderID(store.ChartsDir()),
		Slots: []store.SessionSlot{{
			Chart: store.SessionChart{ID: "zed", Name: "Client Zed", When: fixtureOneWhen, Place: fixtureOneP,
				Source: store.ChartSource{Kind: store.ChartKindFile, File: "zed.as"}},
			Usable: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	file := l.File()
	m = openSessions(t, m)
	m, _ = press(t, m, "down")
	m, cmd := press(t, m, "enter")
	tm, _ := m.Update(cmd())
	m = tm.(Model)
	if m.named.file != file {
		t.Fatalf("premise: the window runs %q", m.named.file)
	}
	edited := fileText(t, store.NamedSessionPath(file)) + "client_assignment:\n  client: someone\n"
	if err := os.WriteFile(store.NamedSessionPath(file), []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	m, cmd = m.chooseHouseSystem(0, recipe.Regiomontanus)
	m = pump(t, m, cmd)
	tm, _ = m.Update(escRunes("q"))
	if e := tm.(Model).SessionWriteError(); e != nil {
		t.Errorf("quit reported %v", e)
	}
	if got := fileText(t, store.NamedSessionPath(file)); got != edited {
		t.Errorf("the edited session was written over:\n%s", got)
	}
	if logHas(m, "changed outside Sextile") != 1 {
		t.Errorf("no single Log line says the file changed: %v", m.logLines)
	}
	m = saveAs(t, m, "Rescued")
	if _, err := os.Stat(store.NamedSessionPath("rescued.yaml")); err != nil {
		t.Errorf("Save as no longer works: %v", err)
	}
}
