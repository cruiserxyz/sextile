package ui

import (
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// leaveModel is a window running the saved session try4 with three charts,
// after which Goethe is opened and selected and the floating chart is moved to
// a moment no file may hold: none of that is saved yet.
func leaveModel(t *testing.T) Model {
	t.Helper()
	freshModels(t)
	m := sessionsModel(t, 3)
	t.Cleanup(store.ReleaseInstanceLock)
	m = saveAs(t, m, "try4")
	if !m.named.running() {
		t.Fatalf("the save did not make the window run try4 (refusal %q)", m.sessions.Refusal)
	}
	goethe := store.SessionChart{ID: "goethe", Name: "Goethe", Source: store.ChartSource{Kind: store.ChartKindFile, File: "goethe.as"}}
	place := location.PlaceInfo{Name: "Frankfurt", Country: "DE", Lat: 50.1, Lon: 8.7, TZ: "Europe/Berlin"}
	if err := m.slots.AddAnchored(goethe, time.Date(1749, 8, 28, 11, 30, 0, 0, time.UTC), place); err != nil {
		t.Fatal(err)
	}
	m.slots.SetSlotSelected(m.slots.AnchoredCount()-1, true)
	m.slots.SetFloatingTime(time.Date(1999, 12, 31, 23, 58, 0, 0, time.UTC))
	return m
}

// (a), (c) A new session saves the named one first: Goethe and his selection
// are in the file, the floating chart's moment is not.
func TestNewSessionSavesTheNamedSessionFirst(t *testing.T) {
	m := leaveModel(t)
	names := anchoredNames(m)
	var marks []bool
	for i := 0; i < m.slots.AnchoredCount(); i++ {
		marks = append(marks, m.slots.SlotSelected(i))
	}
	m = resetKey(t, m)
	text := fileText(t, store.NamedSessionPath("try4.yaml"))
	if !strings.Contains(text, "Goethe") {
		t.Errorf("try4 does not hold Goethe:\n%s", text)
	}
	if strings.Contains(text, "1999-12-31") {
		t.Errorf("try4 holds the floating chart's moment:\n%s", text)
	}
	m = openSessions(t, m)
	m, _ = press(t, m, "down", "down")
	m, cmd := press(t, m, "enter")
	if cmd == nil {
		t.Fatalf("Enter on try4 started nothing: rows %+v cursor %d log %v", sessionsRows(m), m.sessions.Cursor, logTexts(m))
	}
	tm, _ := m.Update(cmd())
	m = tm.(Model)
	if got := anchoredNames(m); !slices.Equal(got, names) {
		t.Errorf("reopened charts = %v, want %v", got, names)
	}
	var got []bool
	for i := 0; i < m.slots.AnchoredCount(); i++ {
		got = append(got, m.slots.SlotSelected(i))
	}
	if !slices.Equal(got, marks) {
		t.Errorf("reopened selection = %v, want %v", got, marks)
	}
}

// leaveExit is a way out of a named session into another: a new session or
// another named one.
type leaveExit struct {
	name string
	run  func(t *testing.T, m Model) Model
}

var leaveExits = []leaveExit{
	{"new session", func(t *testing.T, m Model) Model { return resetKey(t, m) }},
	{"open another", func(t *testing.T, m Model) Model {
		m = openSessions(t, m)
		for i, r := range m.sessions.Rows {
			if r.File == "other.yaml" {
				m.sessions.Cursor = i + 2
			}
		}
		m, cmd := press(t, m, "enter")
		return pump(t, m, cmd)
	}},
}

// leaveFailure is a way the save before leaving can fail.
type leaveFailure struct {
	name, line string
	arm        func(t *testing.T)
}

var leaveFailures = []leaveFailure{
	{"unwritable folder", "the session this window ran was not saved", func(t *testing.T) {
		dir := store.SessionsDir()
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	}},
	{"file changed outside", "changed outside Sextile and was left as it is", func(t *testing.T) {
		p := store.NamedSessionPath("try4.yaml")
		if err := os.WriteFile(p, []byte(fileText(t, p)+"# edited elsewhere\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}},
}

// (b) A save that fails, or is refused because the file changed outside
// Sextile, leaves the window in the session: the lock still held, every chart
// and selection as they were, nothing new started or opened.
func TestLeavingAFailedSaveStaysInTheNamedSession(t *testing.T) {
	for _, exit := range leaveExits {
		for _, fail := range leaveFailures {
			t.Run(exit.name+"/"+fail.name, func(t *testing.T) {
				m := leaveModel(t)
				otherSession(t, "Other", "Client Zed")
				m = m.reloadSessions("")
				before := m.sessionSnapshot()
				fail.arm(t)
				m = exit.run(t, m)
				if m.named.file != "try4.yaml" || m.named.lock == nil || !m.named.running() {
					t.Fatalf("the window left try4: %+v", m.named)
				}
				_ = os.Chmod(store.SessionsDir(), 0o700)
				_, _, other, err := store.OpenNamedSession("try4.yaml", store.FolderID(store.ChartsDir()))
				if err == nil {
					other.Release()
				}
				if !errors.Is(err, store.ErrSessionOpenElsewhere) {
					t.Errorf("the lock on try4 is not held: another handle got %v", err)
				}
				if got := m.sessionSnapshot(); !reflect.DeepEqual(got, before) {
					t.Errorf("the session changed:\n%+v\nwant\n%+v", got, before)
				}
				if logHas(m, fail.line) != 1 {
					t.Errorf("no single %q line: %v", fail.line, logTexts(m))
				}
				for _, l := range m.logLines {
					if strings.Contains(l.text, fail.line) && l.lane != laneError {
						t.Errorf("the line is not on the error lane: %+v", l)
					}
					if strings.Contains(l.text, store.SessionsDir()) && strings.Contains(l.text, fail.line) {
						t.Errorf("the line names a path: %q", l.text)
					}
				}
				if logHas(m, "new session") != 0 {
					t.Errorf("a new session started: %v", logTexts(m))
				}
			})
		}
	}
}

// keptModel is leaveModel with the session marked as not written when left, as
// a refused read or a file changed in another program leaves it.
func keptModel(t *testing.T) Model {
	t.Helper()
	m := leaveModel(t)
	m.named.kept = true
	if m.savedWhenLeft() {
		t.Fatal("premise: the session counts as saved when left")
	}
	return m
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func nameIt(t *testing.T, m Model, name string) (Model, tea.Cmd) {
	t.Helper()
	m = clearField(t, m, 80)
	m = typeText(t, m, name)
	return press(t, m, "enter")
}

// (a) Quit from a session not written when left asks first: Yes saves it under a
// name and quits, No quits without a write, Esc stays.
func TestQuitFromAKeptSessionAsksFirst(t *testing.T) {
	m := keptModel(t)
	path := store.NamedSessionPath("try4.yaml")
	before := fileText(t, path)
	m, cmd := press(t, m, "q")
	if isQuit(cmd) || !m.sessions.Asking {
		t.Fatalf("q quit without asking (asking %v)", m.sessions.Asking)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"Quit", "Save this session first?"} {
		if !strings.Contains(v, want) {
			t.Errorf("the question lacks %q:\n%s", want, v)
		}
	}
	if esc, cmd := press(t, m, "esc"); isQuit(cmd) || esc.sessions.Asking || esc.startup == startupSessionsOpen || !esc.named.running() {
		t.Errorf("Esc did not stay: asking %v, startup %q", esc.sessions.Asking, esc.startup)
	}
	if _, cmd := press(t, m, "n"); !isQuit(cmd) || fileText(t, path) != before {
		t.Errorf("No: quit %v, file changed %v", isQuit(cmd), fileText(t, path) != before)
	}
	y, _ := press(t, m, "y")
	if y.startup != startupSessionNameOpen {
		t.Fatalf("Yes did not open the name field (startup %q)", y.startup)
	}
	if _, cmd := nameIt(t, y, "Rescued"); !isQuit(cmd) || !strings.Contains(fileText(t, store.NamedSessionPath("rescued.yaml")), "Goethe") {
		t.Errorf("Yes then a name: quit %v, rescued.yaml lacks Goethe", isQuit(cmd))
	}
}

// (b) Opening another session from one not written when left asks the same.
func TestOpeningAnotherFromAKeptSessionAsksFirst(t *testing.T) {
	start := func() Model {
		m := keptModel(t)
		otherSession(t, "Other", "Client Zed")
		m = openSessions(t, m)
		for i, r := range m.sessions.Rows {
			if r.File == "other.yaml" {
				m.sessions.Cursor = i + 2
			}
		}
		return m
	}
	m := start()
	path := store.NamedSessionPath("try4.yaml")
	before := fileText(t, path)
	m, cmd := press(t, m, "enter")
	if cmd != nil || !m.sessions.Asking || m.named.file != "try4.yaml" {
		t.Fatalf("Enter opened without asking (asking %v, runs %q)", m.sessions.Asking, m.named.file)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Open session") {
		t.Errorf("the question is not titled Open session:\n%s", v)
	}
	if esc, _ := press(t, m, "esc"); esc.sessions.Asking || esc.startup != startupSessionsOpen || esc.named.file != "try4.yaml" {
		t.Errorf("Esc did not return to the list: asking %v, startup %q, runs %q", esc.sessions.Asking, esc.startup, esc.named.file)
	}
	no, cmd := press(t, m, "n")
	no = pump(t, no, cmd)
	if no.named.file != "other.yaml" || fileText(t, path) != before {
		t.Errorf("No: runs %q, try4 changed %v", no.named.file, fileText(t, path) != before)
	}
	y, _ := press(t, m, "y")
	y, cmd = nameIt(t, y, "Rescued")
	y = pump(t, y, cmd)
	if y.named.file != "other.yaml" || !strings.Contains(fileText(t, store.NamedSessionPath("rescued.yaml")), "Goethe") {
		t.Errorf("Yes then a name: runs %q, rescued.yaml lacks Goethe", y.named.file)
	}
}

// (c) A session still being opened is not left: New and opening another do
// nothing.
func TestNewAndOpenDoNothingWhileASessionIsPending(t *testing.T) {
	m := leaveModel(t)
	otherSession(t, "Other", "Client Zed")
	m.named.pending = true
	before := m.sessionSnapshot()
	n := onNewRow(t, m)
	n, cmd := press(t, n, "enter")
	if cmd != nil || n.sessions.Asking || n.startup != startupSessionsOpen || !n.named.running() || !reflect.DeepEqual(n.sessionSnapshot(), before) {
		t.Errorf("New did something: asking %v, startup %q, runs %v", n.sessions.Asking, n.startup, n.named.running())
	}
	o := openSessions(t, m)
	for i, r := range o.sessions.Rows {
		if r.File == "other.yaml" {
			o.sessions.Cursor = i + 2
		}
	}
	o, cmd = press(t, o, "enter")
	if cmd != nil || o.sessions.Asking || o.named.file != "try4.yaml" || !reflect.DeepEqual(o.sessionSnapshot(), before) {
		t.Errorf("Open did something: asking %v, runs %q", o.sessions.Asking, o.named.file)
	}
}

// New from a session not written when left asks, as the other exits do.
func TestNewFromAKeptSessionAsks(t *testing.T) {
	m := onNewRow(t, keptModel(t))
	m, _ = press(t, m, "enter")
	if !m.sessions.Asking || m.slots.AnchoredCount() == 0 {
		t.Errorf("asking %v, %d charts: the session was left without asking", m.sessions.Asking, m.slots.AnchoredCount())
	}
}

// (d) The question names what is true for each kind of session.
func TestTheQuestionNamesWhatIsTrue(t *testing.T) {
	if b := newSessionChoice(askingModel(t)).Body; len(b) != 2 || b[1] != "Its 2 charts are open under no name. No forgets which were open. Their files stay." {
		t.Errorf("the unnamed wording changed: %q", b)
	}
	m := onNewRow(t, keptModel(t))
	m, _ = press(t, m, "enter")
	if b := newSessionChoice(m).Body; len(b) != 2 || b[1] != "Sextile left try4's file as it was, so its 4 charts are in no file." {
		t.Errorf("the kept wording is wrong: %q", b)
	}
	one := keptModel(t)
	one.slots = listWorkspace(t, 1)
	one = onNewRow(t, one)
	one, _ = press(t, one, "enter")
	if b := newSessionChoice(one).Body; len(b) != 2 || b[1] != "Sextile left try4's file as it was, so its 1 chart is in no file." {
		t.Errorf("the one-chart wording is wrong: %q", b)
	}
}

// F3, K4 The question's Save as writes the session once, and the file holds
// the current charts before the leave: a window leaving a session saved a
// moment ago does not write it again, so a full disk after the first write
// stops nothing.
func TestSaveFirstThenNewWritesOnce(t *testing.T) {
	m := askingModel(t)
	names := anchoredNames(m)
	m, _ = press(t, m, "y")
	m, _ = nameIt(t, m, "Mine")
	if m.named.running() || m.startup == startupSessionNameOpen {
		t.Fatalf("the window did not go on to a new session (startup %q)", m.startup)
	}
	text := fileText(t, store.NamedSessionPath("mine.yaml"))
	for _, n := range names {
		if !strings.Contains(text, n) {
			t.Errorf("mine.yaml lacks %q before the leave:\n%s", n, text)
		}
	}
	// The direct leave with saved set, over a file that is current.
	k := leaveModel(t)
	if err := k.saveRunningSession(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fileText(t, store.NamedSessionPath("try4.yaml")), "Goethe") {
		t.Fatal("premise: try4.yaml lacks Goethe")
	}
	dir := store.SessionsDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	tm, _ := startNewSessionAfter(k, true)
	if got := tm.(Model); got.named.running() || logHas(got, "not saved") != 0 {
		t.Errorf("leaving a session saved a moment ago wrote again: named %v, log %v", got.named.running(), logTexts(got))
	}
	tm, _ = startNewSessionAfter(k, false)
	if !tm.(Model).named.running() {
		t.Error("premise: an unsaved leave did not try to write")
	}
}
