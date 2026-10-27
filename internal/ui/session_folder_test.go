package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// All chart values in this file are constructed synthetic test inputs.

// One session is shared by every charts folder, but it is opened only in the
// folder it was saved in. Folder A and folder B both hold a Client.as with the
// same name and the same details; a session saved in A must restore nothing
// when Sextile starts in B, and say so on one Log line. Started in A, the same
// session restores, so the test can see a restore when there is one.
func TestSessionFromAnotherFolderIsNotRestored(t *testing.T) {
	sessionSandbox(t)
	root := t.TempDir()
	a, b := filepath.Join(root, "A"), filepath.Join(root, "B")
	chart, err := os.ReadFile(filepath.Join("..", "astrolog", "testdata", "charts", "Equinox-2026-09.as"))
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{a, b} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "Client.as"), chart, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("SEXTILE_CHARTS_DIR", a)
	rec := snapshotChart()
	rec.Source = store.ChartSource{Kind: store.ChartKindFile, File: "Client.as"}
	if err := store.SaveSession(store.Session{
		Folder: store.FolderID(a),
		Slots:  []store.SessionSlot{{Chart: rec, Usable: true, Selected: true}},
	}); err != nil {
		t.Fatal(err)
	}

	launch := func(dir string) Model {
		t.Helper()
		t.Setenv("SEXTILE_CHARTS_DIR", dir)
		p, ok := readPerimeterCmd(false)().(ingest.StartupPerimeterReadMsg)
		if !ok || !p.HasSession {
			t.Fatalf("premise: the saved session was not read (%+v)", p)
		}
		p.LocationCandidate = location.Candidate{Source: "stored-location", Place: berlinRaw}
		return startedModel(t, p)
	}

	inB := launch(b)
	if n := inB.slots.AnchoredCount(); n != 0 {
		t.Errorf("starting in B restored %d chart(s) from A's session", n)
	}
	want := "session: the saved session belongs to another charts folder and was not opened"
	if got := strings.Join(sessionLines(inB), " | "); got != want {
		t.Errorf("session lines in B = %q, want exactly %q", got, want)
	}
	if len(inB.frames.records) != 0 {
		t.Errorf("A's house records were loaded in B: %+v", inB.frames.records)
	}

	inA := launch(a)
	if n := inA.slots.AnchoredCount(); n != 1 {
		t.Errorf("starting in A restored %d chart(s), want its 1", n)
	}
	if got := sessionLines(inA); len(got) != 0 {
		t.Errorf("starting in A said %q", got)
	}
}

// A session that names no folder belongs nowhere: an old session the one-time
// copy did not stamp is not restored either.
func TestSessionWithoutAFolderIsNotRestored(t *testing.T) {
	p := withSession(perimeter(), store.Session{
		Slots: []store.SessionSlot{{Chart: snapshotChart(), Usable: true}},
	})
	p.Session.Folder = ""
	m := startedModel(t, p)
	if n := m.slots.AnchoredCount(); n != 0 {
		t.Errorf("a session naming no folder restored %d chart(s)", n)
	}
	if got := strings.Join(sessionLines(m), " | "); !strings.Contains(got, "belongs to another charts folder") {
		t.Errorf("session lines = %q", got)
	}
}

// The quit writes the folder the session was saved in.
func TestQuitRecordsTheChartsFolder(t *testing.T) {
	sessionSandbox(t)
	m := anchorFixture(t)
	press(t, m, "q")
	s, ok, err := store.LoadSession()
	if err != nil || !ok {
		t.Fatalf("LoadSession: ok %v err %v", ok, err)
	}
	if want := store.FolderID(store.ChartsDir()); s.Folder != want || want == "" {
		t.Errorf("the session names folder %q, want %q", s.Folder, want)
	}
}

// The one-time copy's line, handed over by main, is one Log line.
func TestMigrationLineIsLogged(t *testing.T) {
	m := NewModelWithClock(Config{Migration: "copied session from the old places; the old files were left as they are"}, FrozenClock(sessionFrozen))
	n := 0
	for _, l := range m.logLines {
		if strings.HasPrefix(l.text, "state: copied session") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the migration line appears %d times, want 1: %+v", n, m.logLines)
	}
}

// A forged launcher environment naming folder A, on a native launch in folder
// B, does not make A's session restore in B.
func TestForgedLauncherEnvDoesNotRestoreAnotherFoldersSession(t *testing.T) {
	sessionSandbox(t)
	root := t.TempDir()
	a, b := filepath.Join(root, "A"), filepath.Join(root, "B")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("SEXTILE_CHARTS_DIR", a)
	if err := store.SaveSession(store.Session{
		Folder: store.FolderID(a),
		Slots:  []store.SessionSlot{{Chart: snapshotChart(), Usable: true}},
	}); err != nil {
		t.Fatal(err)
	}
	// A native run, outside any container, even with --charts-dir /charts and
	// a launch value that claims Docker.
	t.Cleanup(store.OverrideContainerProbe(func() bool { return false }))
	for _, tc := range []struct{ launch, dir string }{{"no-docker x", b}, {"1", b}, {"docker sextile:test", "/charts"}} {
		launch := tc.launch
		t.Setenv("SEXTILE_LAUNCH", launch)
		t.Setenv("SEXTILE_CHARTS_HOST_PATH", a)
		t.Setenv("SEXTILE_CHARTS_DIR", tc.dir)
		p, ok := readPerimeterCmd(false)().(ingest.StartupPerimeterReadMsg)
		if !ok || !p.HasSession {
			t.Fatalf("premise: the saved session was not read")
		}
		p.LocationCandidate = location.Candidate{Source: "stored-location", Place: berlinRaw}
		if n := startedModel(t, p).slots.AnchoredCount(); n != 0 {
			t.Errorf("SEXTILE_LAUNCH=%q: A's session restored %d chart(s) in B", launch, n)
		}
	}
}
