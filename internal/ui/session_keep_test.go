package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

// the specification: a session file this build refuses is kept at
// launch, not overwritten at quit. All chart values are synthetic.

const keepSecret = "PRIVATE-CLIENT-STRING-7f3a"

// refusedSession is a version-3 file this build refuses, carrying a string that
// must never reach the Log or standard error.
const refusedSession = "version: 3\ncharts:\n  - id: x\n    name: " + keepSecret + "\n"

func keptCopies(t *testing.T) []string {
	t.Helper()
	m, err := filepath.Glob(store.SessionPath() + ".refused-*")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func lineTexts(m Model) []string {
	var out []string
	for _, l := range m.logLines {
		out = append(out, l.text)
	}
	return out
}

func quitModel(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := press(t, m, "q")
	if cmd == nil {
		t.Fatal("q did not quit")
	}
	return next
}

// C3: one note-lane line, naming the kept file's base name only.
func TestRefusedSessionKeptIsLogged(t *testing.T) {
	sessionSandbox(t)
	seedPreferences(t)
	seedSession(t, refusedSession)
	m := startedModel(t, realPerimeter(t))

	copies := keptCopies(t)
	if len(copies) != 1 {
		t.Fatalf("kept copies = %v, want one", copies)
	}
	want := "session: refused, kept " + filepath.Base(copies[0])
	n := 0
	for _, l := range m.logLines {
		if strings.Contains(l.text, "kept") {
			n++
			if l.lane != laneNote || l.text != want {
				t.Errorf("line = %v %q, want the note %q", l.lane, l.text, want)
			}
			// The Log is 64 cells wide and clips without an ellipsis: a longer
			// line would cut the name this line exists to give.
			if len(l.text) > 64 {
				t.Errorf("line is %d cells, the Log clips at 64: %q", len(l.text), l.text)
			}
		}
	}
	if n != 1 {
		t.Errorf("%d kept lines, want exactly one: %q", n, lineTexts(m))
	}
	for _, l := range lineTexts(m) {
		if strings.Contains(l, keepSecret) || strings.Contains(l, string(filepath.Separator)) {
			t.Errorf("a Log line carries content or a path: %q", l)
		}
	}
}

// C1: a symlinked session.yaml is left alone and the error lane says why.
func TestRefusedSessionSymlinkReportsWhy(t *testing.T) {
	sessionSandbox(t)
	seedPreferences(t)
	target := filepath.Join(t.TempDir(), "elsewhere.yaml")
	if err := os.WriteFile(target, []byte(refusedSession), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.SessionDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, store.SessionPath()); err != nil {
		t.Fatal(err)
	}
	m := startedModel(t, realPerimeter(t))
	found := false
	for _, l := range m.logLines {
		if l.lane == laneError && l.text == "Couldn't keep the refused session file: it is a symlink." {
			found = true
		}
	}
	if !found {
		t.Errorf("no error-lane line saying why; log = %q", lineTexts(m))
	}
}

// pass 4: link and target byte-identical after a launch and a quit, and the
// failure reaches standard error's source, SessionWriteError (C4).
func TestRefusedSessionSymlinkIsLeftAlone(t *testing.T) {
	sessionSandbox(t)
	seedPreferences(t)
	target := filepath.Join(t.TempDir(), "elsewhere.yaml")
	if err := os.WriteFile(target, []byte(refusedSession), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.SessionDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, store.SessionPath()); err != nil {
		t.Fatal(err)
	}
	m := quitModel(t, startedModel(t, realPerimeter(t)))

	if l, err := os.Readlink(store.SessionPath()); err != nil || l != target {
		t.Errorf("the link changed: %q %v", l, err)
	}
	if b, _ := os.ReadFile(target); string(b) != refusedSession {
		t.Errorf("the target changed: %q", b)
	}
	if got := keptCopies(t); len(got) != 0 {
		t.Errorf("copies appeared: %v", got)
	}
	err := m.SessionWriteError()
	if err == nil || !strings.Contains(err.Error(), "it is a symlink") || strings.Contains(err.Error(), keepSecret) {
		t.Errorf("SessionWriteError = %v, want the symlink reason and no content", err)
	}
}

// pass 5: session.yaml a directory -> nothing renamed, nothing written.
func TestRefusedSessionDirectoryIsLeftAlone(t *testing.T) {
	sessionSandbox(t)
	seedPreferences(t)
	if err := os.MkdirAll(filepath.Join(store.SessionPath(), "inside"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(store.SessionPath(), "inside", "f")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := quitModel(t, startedModel(t, realPerimeter(t)))

	if fi, err := os.Lstat(store.SessionPath()); err != nil || !fi.IsDir() {
		t.Errorf("session.yaml is no longer the directory: %v", err)
	}
	if b, _ := os.ReadFile(marker); string(b) != "keep" {
		t.Errorf("the directory's content changed: %q", b)
	}
	if got := keptCopies(t); len(got) != 0 {
		t.Errorf("copies appeared: %v", got)
	}
	if err := m.SessionWriteError(); err == nil || !strings.Contains(err.Error(), "it is a directory") {
		t.Errorf("SessionWriteError = %v, want the directory reason", err)
	}
}

// pass 5: a file kept for one dropped entry still restores its valid charts.
func TestPartlyRefusedSessionStillRestoresValidCharts(t *testing.T) {
	sessionSandbox(t)
	seedPreferences(t)
	if err := store.SaveSession(store.Session{Slots: []store.SessionSlot{listEntry(1, true), listEntry(2, false)}}); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(store.SessionPath())
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(string(text), "    lat: 42\n", "    lat: nope\n", 1)
	if broken == string(text) {
		t.Fatal("fixture: the second chart's latitude was not found")
	}
	seedSession(t, broken)
	m := startedModel(t, realPerimeter(t))

	if m.slots.AnchoredCount() != 1 {
		t.Errorf("%d charts restored, want the one valid chart", m.slots.AnchoredCount())
	}
	copies := keptCopies(t)
	if len(copies) != 1 {
		t.Fatalf("kept copies = %v, want one", copies)
	}
	if b, _ := os.ReadFile(copies[0]); string(b) != broken {
		t.Error("the kept copy is not the file that was refused")
	}
	// The quit writes the valid chart anew; the dropped one lives on in the copy.
	quitModel(t, m)
	if b, _ := os.ReadFile(copies[0]); string(b) != broken {
		t.Error("the quit changed the kept copy")
	}
}

// pass 6: an empty file and an overlong line, through launch and quit.
func TestUnreadableSessionBytesSurviveLaunch(t *testing.T) {
	long := listEntry(2, false)
	long.Chart.Name = strings.Repeat("N", (1<<20)+1024)
	for name, text := range map[string]string{
		"empty":     "",
		"overlong":  "version: 2\ncharts:\n  - id: x\n    name: " + long.Chart.Name + "\n",
		"no number": "version:\n",
	} {
		t.Run(name, func(t *testing.T) {
			sessionSandbox(t)
			seedPreferences(t)
			seedSession(t, text)
			quitModel(t, startedModel(t, realPerimeter(t)))
			copies := keptCopies(t)
			if len(copies) != 1 {
				t.Fatalf("kept copies = %v, want one", copies)
			}
			if b, _ := os.ReadFile(copies[0]); string(b) != text {
				t.Errorf("kept copy differs from the original (%d vs %d bytes)", len(b), len(text))
			}
			if _, ok, err := store.LoadSession(); !ok || err != nil {
				t.Errorf("the quit did not write a readable new session: ok %v err %v", ok, err)
			}
		})
	}
}

// pass 4: the copy survives, and a later write failure still reaches standard
// error's source.
func TestBackupThenSessionWriteFailure(t *testing.T) {
	sessionSandbox(t)
	seedPreferences(t)
	seedSession(t, refusedSession)
	m := startedModel(t, realPerimeter(t))
	copies := keptCopies(t)
	if len(copies) != 1 {
		t.Fatalf("kept copies = %v, want one", copies)
	}
	// Whatever stands at session.yaml now, the quit's rename cannot replace it.
	if err := os.MkdirAll(filepath.Join(store.SessionPath(), "block"), 0o755); err != nil {
		t.Fatal(err)
	}
	m = quitModel(t, m)
	if b, _ := os.ReadFile(copies[0]); string(b) != refusedSession {
		t.Error("the refused copy did not survive the failed write")
	}
	if err := m.SessionWriteError(); err == nil || !strings.Contains(err.Error(), "couldn't save the session") {
		t.Errorf("SessionWriteError = %v, want the failed write", err)
	}
}

// pass 6: a private string in the rejected file appears neither in the Log nor
// in what standard error would print, kept or not.
func TestRefusedSessionNeverPrintsContents(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		sessionSandbox(t)
		seedPreferences(t)
		if symlink {
			target := filepath.Join(t.TempDir(), "elsewhere.yaml")
			if err := os.WriteFile(target, []byte(refusedSession), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(store.SessionDir(), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, store.SessionPath()); err != nil {
				t.Fatal(err)
			}
		} else {
			seedSession(t, refusedSession)
		}
		m := quitModel(t, startedModel(t, realPerimeter(t)))
		for _, l := range lineTexts(m) {
			if strings.Contains(l, keepSecret) {
				t.Errorf("symlink=%v: the Log printed the file's content: %q", symlink, l)
			}
		}
		if err := m.SessionWriteError(); err != nil && strings.Contains(err.Error(), keepSecret) {
			t.Errorf("symlink=%v: standard error would print the content: %v", symlink, err)
		}
	}
}

// the specification C4: a failed keep is reported the moment the
// perimeter arrives, not when a workspace is finally built. A cold start that
// must ask for the city has no workspace yet, and quitting from that modal must
// still print the failure.
func TestFailedKeepReportedBeforeTheCityIsChosen(t *testing.T) {
	p := ingest.StartupPerimeterReadMsg{
		LocationCandidate: location.Candidate{
			Source: "stored-location",
			Place:  location.RawPlace{Name: "Atlantis", Country: "??"},
		},
		SessionKeep: store.SessionKeep{Problem: "it is a symlink"},
	}
	var tm tea.Model = NewModelWithClock(Config{Accessible: true}, FrozenClock(sessionFrozen))
	tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
	tm, _ = tm.Update(p)
	m := tm.(Model)
	if m.startup != startupAtlasOpen {
		t.Fatalf("startup = %q, want the atlas modal open", m.startup)
	}
	found := false
	for _, l := range m.logLines {
		if l.lane == laneError && l.text == "Couldn't keep the refused session file: it is a symlink." {
			found = true
		}
	}
	if !found {
		t.Errorf("no error-lane line before the city was chosen: %q", lineTexts(m))
	}
	if err := m.SessionWriteError(); err == nil || !strings.Contains(err.Error(), "it is a symlink") {
		t.Errorf("SessionWriteError = %v, want the failed keep before any workspace exists", err)
	}
	if !m.sessionKept {
		t.Error("sessionKept is clear: a later write could replace the file")
	}
}

// F4: a kept copy is named even when the move off the original then failed.
func TestFailedMoveStillNamesTheKeptCopy(t *testing.T) {
	m := startedModel(t, withKeep(perimeter(), store.SessionKeep{
		Kept: "session.yaml.refused-20261008T120000Z", Problem: "it could not be moved"}))
	var note, errLine bool
	for _, l := range m.logLines {
		note = note || (l.lane == laneNote && l.text == "session: refused, kept session.yaml.refused-20261008T120000Z")
		errLine = errLine || (l.lane == laneError && l.text == "Couldn't keep the refused session file: it could not be moved.")
	}
	if !note || !errLine {
		t.Errorf("note %v, error %v; log = %q", note, errLine, lineTexts(m))
	}
	if !m.sessionKept {
		t.Error("sessionKept is clear after a failed move")
	}
}

func withKeep(p ingest.StartupPerimeterReadMsg, k store.SessionKeep) ingest.StartupPerimeterReadMsg {
	p.SessionKeep = k
	return p
}

// A successful keep's clear survives the later restore, whatever the error.
func TestSuccessfulKeepOfOverlongLineClearsSessionKept(t *testing.T) {
	p := withKeep(perimeter(), store.SessionKeep{Kept: "session.yaml.refused-20261008T120000Z"})
	p.SessionErr = store.ErrSessionLineTooLong
	m := startedModel(t, p)
	if m.sessionKept {
		t.Error("sessionKept is set after the file was kept: the launch would write nothing")
	}
}
