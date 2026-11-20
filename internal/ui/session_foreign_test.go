package ui

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
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

// A run in one charts folder over the general session another charts folder
// saved never writes that file (specs/session-other-folder-kept.md). Every
// test here starts the way a launch does: two real temp folders A and B with
// their real folder identities, A's session on disk, and the real perimeter
// read in B.

// foreignPast is the modification time A's file is given, so a rewrite with
// the same bytes still shows in the file's mtime.
var foreignPast = time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)

// foreignOpt shapes the setup: the chart files B's library holds, a session
// that names no folder at all or the folder the launch runs in, a file this
// build refuses (it also holds a frames entry the reader drops, so the launch
// moves it aside), one it cannot move aside (session.yaml is a symlink), and a
// change to the perimeter read.
type foreignOpt struct {
	library  map[string]string
	noFolder bool
	here     bool
	refuse   bool
	symlink  bool
	named    string // a --session launch of this named session, saved in B
	tweak    func(*ingest.StartupPerimeterReadMsg)
}

// droppedFrame is a frames entry this build drops (an unknown house system),
// which makes the reader refuse the whole file.
const droppedFrame = "frames:\n  - file: x.as\n    when: \"1995-03-20T10:00:00Z\"\n    houses: Nonsense\n    from: chosen\n"

// foreignRun is a launch in charts folder B over the general session saved in
// charts folder A, and what A's file was when the launch began.
type foreignRun struct {
	m     Model
	a, b  string
	path  string
	text  string
	ino   uint64
	mtime time.Time
}

func foreignLaunch(t *testing.T, opt foreignOpt) *foreignRun {
	t.Helper()
	sessionSandbox(t)
	stubHouseLookups(t)
	root := t.TempDir()
	r := &foreignRun{a: filepath.Join(root, "A"), b: filepath.Join(root, "B")}
	for _, dir := range []string{r.a, r.b} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for name, text := range opt.library {
		if err := os.WriteFile(filepath.Join(r.b, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("SEXTILE_CHARTS_DIR", r.a)
	rec := snapshotChart()
	rec.Source = store.ChartSource{Kind: store.ChartKindFile, File: "Client.as"}
	saved := store.Session{
		FloatingSelected: true,
		Slots:            []store.SessionSlot{{Chart: rec, Usable: true, Selected: true, Bodies: []string{"Moon"}}},
	}
	wantFolder := store.FolderID(r.a)
	if opt.here {
		wantFolder = store.FolderID(r.b)
	}
	if !opt.noFolder {
		saved.Folder = wantFolder
	}
	if err := store.SaveSession(saved); err != nil {
		t.Fatal(err)
	}
	r.path = store.SessionPath()
	if opt.refuse {
		f, err := os.OpenFile(r.path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(droppedFrame); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(r.path, foreignPast, foreignPast); err != nil {
		t.Fatal(err)
	}
	if opt.symlink {
		target := filepath.Join(root, "elsewhere.yaml")
		if err := os.Rename(r.path, target); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, r.path); err != nil {
			t.Fatal(err)
		}
	}
	r.text = fileText(t, r.path)
	r.ino, r.mtime = fileIdentity(t, r.path)
	if strings.Contains(r.text, "folder:") == opt.noFolder {
		t.Fatalf("premise: A's file does not hold what the case needs (no folder %v):\n%s", opt.noFolder, r.text)
	}

	t.Setenv("SEXTILE_CHARTS_DIR", r.b)
	if opt.named != "" {
		// A --session launch: main opens the named session, saved in B, and
		// the launch's read hands over THAT file. The general session A saved
		// is never read by this launch.
		first, err := store.SaveNamedSessionAs(opt.named, store.Session{Folder: store.FolderID(r.b), FloatingSelected: true})
		if err != nil {
			t.Fatal(err)
		}
		first.Release()
		_, name, lock, err := store.OpenNamedSessionByName(opt.named, store.FolderID(r.b))
		if err != nil {
			t.Fatal(err)
		}
		var tm tea.Model = newTestModel(t, Config{Accessible: true, NamedSession: lock, NamedSessionName: name}, FrozenClock(sessionFrozen))
		p, ok := tm.(Model).perimeterCmd()().(ingest.StartupPerimeterReadMsg)
		if !ok || !p.HasSession || p.SessionErr != nil || p.Session.Folder != store.FolderID(r.b) || p.FolderID != store.FolderID(r.b) {
			t.Fatalf("premise: the real --session read did not hand over the named file of B (%+v)", p)
		}
		p.LocationCandidate = location.Candidate{Source: "stored-location", Place: berlinRaw}
		tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
		tm, _ = tm.Update(p)
		r.m = tm.(Model)
		r.m.startup = startupReady
		if !r.m.named.running() || r.m.slots == nil {
			t.Fatal("premise: the launch does not run the named session")
		}
		if r.m.sessionForeign {
			t.Fatal("premise: a --session launch marked the run before it met the general session")
		}
		r.untouched(t, "after the --session launch")
		return r
	}
	p, ok := readPerimeterCmd(false)().(ingest.StartupPerimeterReadMsg)
	if !ok || !p.HasSession || p.SessionErr != nil || p.FolderID != store.FolderID(r.b) || p.FolderID == store.FolderID(r.a) {
		t.Fatalf("premise: the real read in B did not hand over A's session (%+v)", p)
	}
	if !opt.noFolder && p.Session.Folder != wantFolder {
		t.Fatalf("premise: the session read names folder %q, want %q", p.Session.Folder, wantFolder)
	}
	switch k := p.SessionKeep; {
	case opt.symlink:
		if k.Kept != "" || k.Problem == "" {
			t.Fatalf("premise: the read did not leave the refused file in place (%+v)", k)
		}
	case opt.refuse:
		if k.Kept == "" || k.Problem != "" {
			t.Fatalf("premise: the read did not move the refused file aside (%+v)", k)
		}
	case k != (store.SessionKeep{}):
		t.Fatalf("premise: the read refused the file (%+v)", k)
	}
	p.LocationCandidate = location.Candidate{Source: "stored-location", Place: berlinRaw}
	if opt.tweak != nil {
		opt.tweak(&p)
	}
	r.m = startedModel(t, p)
	r.m.startup = startupReady
	if r.m.slots == nil || !r.m.ownsSession() {
		t.Fatal("premise: the launch has no workspace of its own")
	}
	return r
}

// open anchors a chart file of B and marks the Sun on it, so B's workspace
// differs from A's session in every way a write would show.
func (r *foreignRun) open(t *testing.T) {
	t.Helper()
	chart := store.NamedChart{
		ID: "client-one", Name: "Client One", Year: 2001, Place: fixtureOneP, PlaceLabel: "Test Bay",
		Source: store.ChartSource{Kind: store.ChartKindFile, File: "Client.as"},
	}
	picked, cmd := applyChartPicked(r.m, ChartPickedMsg{Chart: chart})
	if cmd == nil {
		t.Fatal("a pick must dispatch a resolve")
	}
	r.m, cmd = resolved(t, picked, fixtureOneWhen)
	r.m = pump(t, r.m, cmd)
	if r.m.slots.AnchoredCount() != 1 {
		t.Fatalf("premise: %d charts open, want 1", r.m.slots.AnchoredCount())
	}
	if _, ok := r.m.slots.ToggleObjectSelection(0, objSun); !ok {
		t.Fatal("could not mark the Sun")
	}
}

// untouched fails the test unless A's file is what A left: the same bytes, the
// same inode and the same modification time.
func (r *foreignRun) untouched(t *testing.T, when string) {
	t.Helper()
	r.sameAs(t, r.path, when)
}

// sameAs is untouched for another name of A's file: the copy a refused file
// was moved to, or the file a symlink points at.
func (r *foreignRun) sameAs(t *testing.T, path, when string) {
	t.Helper()
	if got := fileText(t, path); got != r.text {
		t.Errorf("%s: A's session changed:\n%s", when, got)
	}
	if ino, mt := fileIdentity(t, path); ino != r.ino || !mt.Equal(r.mtime) {
		t.Errorf("%s: A's session was rewritten (inode %d to %d, mtime %v to %v)", when, r.ino, ino, r.mtime, mt)
	}
}

// foreignLine is the Log line a house or zodiac choice leaves in a run that
// cannot save it.
func foreignLine(label, what string) string {
	return "session: " + label + "'s " + what + " kept until quit"
}

// T1
func TestForeignSessionSurvivesQuit(t *testing.T) {
	r := foreignLaunch(t, foreignOpt{})
	r.open(t)
	if !r.m.sessionForeign {
		t.Error("the launch did not mark the session as another folder's")
	}
	quitWindow(t, r.m)
	r.untouched(t, "after the quit")

	// Premise: without the mark the same quit writes B's workspace over it.
	unmarked := r.m
	unmarked.sessionForeign = false
	quitWindow(t, unmarked)
	if got := fileText(t, r.path); got == r.text || !strings.Contains(got, "Client One") {
		t.Errorf("premise: the same quit without the mark did not write B's session:\n%s", got)
	}
}

// T2
func TestForeignSessionSurvivesFrame(t *testing.T) {
	r := foreignLaunch(t, foreignOpt{})
	r.open(t)
	m, cmd := r.m.chooseHouseSystem(0, recipe.Regiomontanus)
	m = pump(t, m, cmd)
	r.untouched(t, "after a house choice")
	if n := logHas(m, foreignLine("Client One", "houses")); n != 1 {
		t.Errorf("the house line appears %d times, want once: %+v", n, m.logLines)
	}
	if n := logHas(m, "tried again at quit"); n != 0 {
		t.Errorf("a retry at quit is promised %d times, and none will come", n)
	}
	if rec, ok := m.frames.find("Client.as", false); !ok || rec.Houses != recipe.Regiomontanus {
		t.Errorf("the choice is not kept for this run: %+v (found %v)", rec, ok)
	}

	m, cmd = m.chooseZodiac(0, recipe.FaganBradley)
	m = pump(t, m, cmd)
	r.untouched(t, "after a zodiac choice")
	if n := logHas(m, foreignLine("Client One", "zodiac")); n != 1 {
		t.Errorf("the zodiac line appears %d times, want once: %+v", n, m.logLines)
	}

	// Premise: without the mark the same choice writes.
	unmarked := r.m
	unmarked.sessionForeign = false
	unmarked, cmd = unmarked.chooseHouseSystem(0, recipe.Porphyry)
	pump(t, unmarked, cmd)
	if got := fileText(t, r.path); !strings.Contains(got, "houses: "+recipe.Porphyry.StoredName()) {
		t.Errorf("premise: the same choice without the mark did not reach the file:\n%s", got)
	}
}

// T3
func TestForeignRunSaysSo(t *testing.T) {
	for _, tc := range []struct{ name, leader, spelled string }{
		{"default leader", "", ",O"},
		{"configured leader", `\`, `\O`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := foreignLaunch(t, foreignOpt{tweak: func(p *ingest.StartupPerimeterReadMsg) {
				if tc.leader != "" {
					p.Keys = store.KeysConfig{Leader: tc.leader, LeaderPresent: true}
				}
			}})
			want := []string{
				"session: the last session belongs to another charts folder, it was not opened",
				"session: these charts are not kept unless the session gets a name",
			}
			if got := sessionLines(r.m); !reflect.DeepEqual(got, want) {
				t.Errorf("session lines = %q, want exactly %q", got, want)
			}
			// The key it names is the one that opens the Sessions list.
			m, _ := press(t, r.m, strings.Split(tc.spelled, "")...)
			if m.startup != startupSessionsOpen {
				t.Errorf("%s did not open the Sessions list (startup %q)", tc.spelled, m.startup)
			}
		})
	}
}

// T4
func TestForeignRunNamedKeepsSaving(t *testing.T) {
	r := foreignLaunch(t, foreignOpt{})
	r.open(t)
	m := saveAs(t, r.m, "Client work")
	if !m.named.running() {
		t.Fatalf("the save did not make the window run a named session (refusal %q)", m.sessions.Refusal)
	}
	named := store.NamedSessionPath("client-work.yaml")
	if got := fileText(t, named); strings.Contains(got, "Mars") || strings.Contains(got, "houses:") {
		t.Fatalf("premise: the named file already holds the later change:\n%s", got)
	}
	r.untouched(t, "after Save as")

	// A change, then an immediate write: the named file holds it.
	if _, ok := m.slots.ToggleObjectSelection(0, objMars); !ok {
		t.Fatal("could not mark Mars")
	}
	m, cmd := m.chooseHouseSystem(0, recipe.Regiomontanus)
	m = pump(t, m, cmd)
	if got := fileText(t, named); !strings.Contains(got, "Mars") || !strings.Contains(got, "houses: "+recipe.Regiomontanus.StoredName()) {
		t.Errorf("the immediate write did not reach the named session:\n%s", got)
	}
	r.untouched(t, "after the immediate write")

	// Another change, then the quit.
	if _, ok := m.slots.ToggleObjectSelection(0, objMoon); !ok {
		t.Fatal("could not mark the Moon")
	}
	quitWindow(t, m)
	if got := fileText(t, named); !strings.Contains(got, "Moon") {
		t.Errorf("the quit did not write the named session:\n%s", got)
	}
	r.untouched(t, "after the quit")
	if n := logHas(m, "kept only until you quit"); n != 0 {
		t.Errorf("a named run says its choices are kept only until you quit (%d lines)", n)
	}
}

// T5
func TestForeignRunNamedThenNew(t *testing.T) {
	// back leaves the named session with `<leader> n` and chooses a house
	// system for the floating chart: an immediate write, then the quit.
	back := func(t *testing.T, r *foreignRun, m Model) {
		t.Helper()
		t.Cleanup(store.ReleaseInstanceLock)
		m = resetKey(t, m)
		if m.named.running() {
			t.Fatal("the New row did not return to the unnamed session")
		}
		m, cmd := m.chooseHouseSystem(len(m.slots.All())-1, recipe.Regiomontanus)
		m = pump(t, m, cmd)
		r.untouched(t, "after a house choice back in the unnamed session")
		if n := logHas(m, foreignLine("the floating chart", "houses")); n != 1 {
			t.Errorf("the house line appears %d times, want once: %+v", n, m.logLines)
		}
		quitWindow(t, m)
		r.untouched(t, "after the quit")
	}
	t.Run("created", func(t *testing.T) {
		r := foreignLaunch(t, foreignOpt{})
		r.open(t)
		m := saveAs(t, r.m, "Client work")
		if !m.named.running() {
			t.Fatalf("the save did not make the window run a named session (refusal %q)", m.sessions.Refusal)
		}
		r.untouched(t, "after Save as")
		back(t, r, m)
	})
	t.Run("opened", func(t *testing.T) {
		r := foreignLaunch(t, foreignOpt{})
		r.open(t)
		file := otherSession(t, "Other work", "Client Zed")
		m := openSessions(t, r.m)
		m, _ = press(t, m, "down", "down")
		m, cmd := press(t, m, "enter")
		if cmd == nil {
			t.Fatal("Enter on a saved session started nothing")
		}
		r.untouched(t, "after Open")
		tm, next := m.Update(cmd())
		m = pump(t, tm.(Model), next)
		if m.named.file != file {
			t.Fatalf("the window runs %q, want %q", m.named.file, file)
		}
		back(t, r, m)
	})
}

// T6
func TestForeignRunFileReplaced(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(t *testing.T, r *foreignRun)
	}{
		{"deleted", func(t *testing.T, r *foreignRun) {
			if err := os.Remove(r.path); err != nil {
				t.Fatal(err)
			}
		}},
		{"replaced", func(t *testing.T, r *foreignRun) {
			later := store.Session{Folder: store.FolderID(r.a), FloatingSelected: false}
			if err := store.SaveSession(later); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(r.path, foreignPast.Add(time.Hour), foreignPast.Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := foreignLaunch(t, foreignOpt{})
			r.open(t)
			tc.change(t, r)
			// What the path holds now is what it must hold at the end.
			held, existed := "", false
			var ino uint64
			var mtime time.Time
			if _, err := os.Stat(r.path); err == nil {
				held, existed = fileText(t, r.path), true
				ino, mtime = fileIdentity(t, r.path)
			}
			same := func(when string) {
				t.Helper()
				_, err := os.Stat(r.path)
				if !existed {
					if !errors.Is(err, os.ErrNotExist) {
						t.Errorf("%s: a file appeared where A's was deleted (%v)", when, err)
					}
					return
				}
				if err != nil {
					t.Fatalf("%s: %v", when, err)
				}
				i, mt := fileIdentity(t, r.path)
				if got := fileText(t, r.path); got != held || i != ino || !mt.Equal(mtime) {
					t.Errorf("%s: the replaced file was written over:\n%s", when, got)
				}
			}
			m, cmd := r.m.chooseHouseSystem(0, recipe.Regiomontanus)
			m = pump(t, m, cmd)
			same("after an immediate write")
			quitWindow(t, m)
			same("after the quit")
		})
	}
}

// T7
func TestNoFolderSessionAsToday(t *testing.T) {
	r := foreignLaunch(t, foreignOpt{noFolder: true})
	if r.m.sessionForeign {
		t.Error("a session that names no folder was marked as another folder's")
	}
	if n := r.m.slots.AnchoredCount(); n != 0 {
		t.Errorf("a session naming no folder restored %d chart(s)", n)
	}
	want := "session: the last session belongs to another charts folder, it was not opened"
	if got := sessionLines(r.m); !reflect.DeepEqual(got, []string{want}) {
		t.Errorf("session lines = %q, want exactly today's %q", got, want)
	}
	r.open(t)
	quitWindow(t, r.m)
	got := fileText(t, r.path)
	if got == r.text || !strings.Contains(got, "folder: "+store.FolderID(r.b)) || !strings.Contains(got, "Client One") {
		t.Errorf("the quit did not replace the old session with B's:\n%s", got)
	}
}

// T8
func TestForeignRenameProceeds(t *testing.T) {
	setup := func(t *testing.T) (*foreignRun, Model, string) {
		t.Helper()
		r := foreignLaunch(t, foreignOpt{library: map[string]string{"K.as": kochChart("Client K", true)}})
		m := openedPickerOn(t, r.m)
		for i, c := range m.picker.Matches {
			if c.Source.File == "K.as" {
				m.picker.Cursor = i
			}
		}
		m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
		if m.startup != startupTitleOpen || !m.title.rename {
			t.Fatalf("Ctrl+R did not open the rename form (startup %v)", m.startup)
		}
		return r, typeText(t, clearField(t, m, 20), "Client Kay"), filepath.Join(r.b, "K.as")
	}
	t.Run("rename", func(t *testing.T) {
		r, m, path := setup(t)
		m, cmd := submit(t, m)
		m, _ = finishRename(t, m, cmd)
		if m.title.Refusal != "" {
			t.Errorf("the rename was refused: %q", m.title.Refusal)
		}
		if got := fileText(t, path); !strings.Contains(got, `"Client Kay"`) {
			t.Errorf("the chart file was not renamed:\n%s", got)
		}
		if got := fileText(t, backupFile(path)); !strings.Contains(got, `"Client K"`) {
			t.Errorf("the backup does not hold the chart as it was:\n%s", got)
		}
		fileKeepsItsSystem(t, r.b, "K.as")
		if n := logHas(m, "Couldn't"); n != 0 {
			t.Errorf("the rename said it could not: %+v", m.logLines)
		}
		r.untouched(t, "after the rename")
		quitWindow(t, m)
		r.untouched(t, "after the quit")
	})
	t.Run("failed rewrite", func(t *testing.T) {
		r, m, path := setup(t)
		// The file changes after the picker listed it: the rename is refused.
		if err := os.WriteFile(path, []byte(kochChart("Client K", true)+"; changed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		m, cmd := submit(t, m)
		m, _ = finishRename(t, m, cmd)
		if m.title.Refusal == "" {
			t.Fatal("premise: the rename was not refused")
		}
		if _, ok := m.frames.find("K.as", false); ok {
			t.Error("the failed rename left a record in the model")
		}
		r.untouched(t, "after the failed rename")
	})
}

// Save defaults freezes the frames of the charts open at the keypress into the
// session. A run that never saves the general session keeps nothing of its
// charts, which its start already said, so Save defaults adds no line about it.
func TestForeignSaveDefaultsKeepsNothing(t *testing.T) {
	r := foreignLaunch(t, foreignOpt{})
	r.open(t)
	open := r.m.openFramesNow()
	if len(open) == 0 {
		t.Fatal("premise: the open chart has no frame to freeze")
	}
	msg := DefaultCitySavedMsg{Place: berlinP, FrameSaved: true, Frame: "Whole Sign, tropical", Open: open}
	m, _ := applyDefaultCitySaved(r.m, msg)
	r.untouched(t, "after Save defaults")
	for _, l := range m.logLines {
		if l.lane == laneError || strings.Contains(l.text, "open charts") {
			t.Errorf("Save defaults in a run of another folder's session wrote %q", l.text)
		}
	}
	saidSaved := false
	for _, l := range m.logLines {
		saidSaved = saidSaved || strings.HasPrefix(l.text, "defaults: ")
	}
	if !saidSaved {
		t.Errorf("the saved default is not in the Log: %+v", m.logLines)
	}

	// Premise: without the mark the same save writes the frames.
	unmarked := r.m
	unmarked.sessionForeign = false
	applyDefaultCitySaved(unmarked, msg)
	if got := fileText(t, r.path); !strings.Contains(got, "frames:") {
		t.Errorf("premise: the same save without the mark did not write the frames:\n%s", got)
	}
}

// The mark belongs to the read of the general session that found another
// charts folder's session, and to nothing else. Each case goes through the
// real read of real files, then attempts the writes a run makes (an immediate
// write and the quit) and compares the files that must survive.
func TestForeignMarkOnlyWhereSpecified(t *testing.T) {
	const otherFolder = "session: the last session belongs to another charts folder, it was not opened"
	t.Run("a refused file moved aside", func(t *testing.T) {
		r := foreignLaunch(t, foreignOpt{refuse: true})
		asides, err := filepath.Glob(r.path + ".refused-*")
		if err != nil || len(asides) != 1 {
			t.Fatalf("premise: the read kept %v (%v), want one copy", asides, err)
		}
		aside := asides[0]
		if _, err := os.Lstat(r.path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("premise: the refused file is still at the session path (%v)", err)
		}
		if r.m.sessionForeign {
			t.Error("a refused file the read moved aside marked the run")
		}
		for _, want := range []string{otherFolder, "session: unreadable, kept " + filepath.Base(aside)} {
			if n := logHas(r.m, want); n != 1 {
				t.Errorf("%q appears %d times, want once: %+v", want, n, r.m.logLines)
			}
		}
		if n := logHas(r.m, "saved only by name"); n != 0 {
			t.Error("the run says it is not saved, and it is")
		}
		r.open(t)
		m, cmd := r.m.chooseHouseSystem(0, recipe.Regiomontanus)
		m = pump(t, m, cmd)
		r.sameAs(t, aside, "after a house choice")
		quitWindow(t, m)
		r.sameAs(t, aside, "after the quit")
		// Nothing of A's is left at the session path, so B writes its own as before.
		if got := fileText(t, r.path); !strings.Contains(got, "folder: "+store.FolderID(r.b)) || !strings.Contains(got, "Client One") {
			t.Errorf("B did not write its own session:\n%s", got)
		}
	})
	t.Run("a refused file left in place", func(t *testing.T) {
		r := foreignLaunch(t, foreignOpt{refuse: true, symlink: true})
		if !r.m.sessionKept {
			t.Fatal("premise: the launch does not keep the refused file")
		}
		if r.m.sessionForeign {
			t.Error("a refused file that stayed in place marked the run")
		}
		r.open(t)
		m, cmd := r.m.chooseHouseSystem(0, recipe.Regiomontanus)
		m = pump(t, m, cmd)
		r.untouched(t, "after a house choice")
		// The kept file's failure is the quit's to report, so no quitWindow.
		m.Update(escRunes("q"))
		r.untouched(t, "after the quit")
		if fi, err := os.Lstat(r.path); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("the session path is no longer the symlink (%v)", err)
		}
	})
	t.Run("a named session", func(t *testing.T) {
		sessionSandbox(t)
		stubHouseLookups(t)
		root := t.TempDir()
		a, b := filepath.Join(root, "A"), filepath.Join(root, "B")
		for _, dir := range []string{a, b} {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		t.Setenv("SEXTILE_CHARTS_DIR", b)
		// The session --session opens is saved in B, as main requires of it.
		first, err := store.SaveNamedSessionAs("Client work", store.Session{Folder: store.FolderID(b), FloatingSelected: true})
		if err != nil {
			t.Fatal(err)
		}
		first.Release()
		_, name, lock, err := store.OpenNamedSessionByName("Client work", store.FolderID(b))
		if err != nil {
			t.Fatal(err)
		}
		// After main opened it and before the launch reads it, its bytes are
		// replaced by ones that name A.
		path := store.NamedSessionPath(lock.File())
		text := fileText(t, path)
		swapped := strings.Replace(text, "folder: "+store.FolderID(b), "folder: "+store.FolderID(a), 1)
		if swapped == text {
			t.Fatalf("premise: the named file does not name B:\n%s", text)
		}
		if err := os.WriteFile(path, []byte(swapped), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, foreignPast, foreignPast); err != nil {
			t.Fatal(err)
		}
		r := &foreignRun{a: a, b: b, path: path, text: swapped}
		r.ino, r.mtime = fileIdentity(t, path)

		var tm tea.Model = newTestModel(t, Config{Accessible: true, NamedSession: lock, NamedSessionName: name}, FrozenClock(sessionFrozen))
		p, ok := tm.(Model).perimeterCmd()().(ingest.StartupPerimeterReadMsg)
		if !ok || !p.HasSession || p.SessionErr != nil || p.Session.Folder != store.FolderID(a) || p.FolderID != store.FolderID(b) {
			t.Fatalf("premise: the real --session read did not hand over the named file naming A (%+v)", p)
		}
		p.LocationCandidate = location.Candidate{Source: "stored-location", Place: berlinRaw}
		tm, _ = tm.Update(AtlasLoadedMsg{Index: location.NewIndex([]location.PlaceInfo{berlinP, fixtureOneP})})
		tm, _ = tm.Update(p)
		r.m = tm.(Model)
		r.m.startup = startupReady
		if !r.m.named.running() || r.m.slots == nil {
			t.Fatal("premise: the launch does not run the named session")
		}

		if r.m.sessionForeign {
			t.Error("a named session naming another folder marked the general session")
		}
		if got := sessionLines(r.m); !reflect.DeepEqual(got, []string{otherFolder}) {
			t.Errorf("session lines = %q, want today's %q", got, otherFolder)
		}
		r.open(t)
		m, cmd := r.m.chooseHouseSystem(0, recipe.Regiomontanus)
		m = pump(t, m, cmd)
		r.untouched(t, "after a house choice")
		// the New row returns to the general session. This run never read it, so it is
		// not marked, and the run writes it as before.
		t.Cleanup(store.ReleaseInstanceLock)
		m = resetKey(t, m)
		if m.named.running() {
			t.Fatal("the New row did not return to the unnamed session")
		}
		m, cmd = m.chooseHouseSystem(len(m.slots.All())-1, recipe.Regiomontanus)
		m = pump(t, m, cmd)
		if g, ok, err := store.LoadSession(); err != nil || !ok || g.Folder != store.FolderID(b) {
			t.Errorf("the unnamed session was not written for B: %+v ok %v err %v", g, ok, err)
		}
		if n := logHas(m, "kept only until you quit"); n != 0 {
			t.Errorf("a run that saves says its choices are kept only until you quit (%d lines)", n)
		}
		quitWindow(t, m)
		r.untouched(t, "after the quit")
	})
	t.Run("this folder", func(t *testing.T) {
		r := foreignLaunch(t, foreignOpt{here: true})
		if r.m.sessionForeign {
			t.Error("a session saved in this folder marked the run")
		}
		if got := sessionLines(r.m); len(got) != 0 {
			t.Errorf("a session of this folder said %q", got)
		}
		quitWindow(t, r.m)
		if ino, mt := fileIdentity(t, r.path); ino == r.ino && mt.Equal(r.mtime) {
			t.Error("the quit did not write this folder's session")
		}
	})
}

// T9. A --session launch never reads the general session, so the launch's
// mark (R1) cannot judge it. When the New row leaves the named session, the run
// returns to a general session it has not met: it reads that file once, as a
// launch does (R6), and one that names another charts folder is never written,
// by the immediate write of a house choice or by the quit.
func TestForeignAfterNamedLaunchThenNew(t *testing.T) {
	t.Run("another folder's", func(t *testing.T) {
		r := foreignLaunch(t, foreignOpt{named: "Client work"})
		t.Cleanup(store.ReleaseInstanceLock)
		m := resetKey(t, r.m)
		if m.named.running() {
			t.Fatal("the New row did not return to the unnamed session")
		}
		if !m.sessionForeign {
			t.Error("the New row after a --session launch did not mark the run over A's general session")
		}
		if n := logHas(m, "not kept unless the session gets a name"); n != 1 {
			t.Errorf("the saved-only-by-name line appears %d times, want once: %+v", n, m.logLines)
		}
		m, cmd := m.chooseHouseSystem(len(m.slots.All())-1, recipe.Regiomontanus)
		m = pump(t, m, cmd)
		r.untouched(t, "after a house choice back in the unnamed session")
		if n := logHas(m, foreignLine("the floating chart", "houses")); n != 1 {
			t.Errorf("the house line appears %d times, want once: %+v", n, m.logLines)
		}
		quitWindow(t, m)
		r.untouched(t, "after the quit")
	})
	t.Run("this folder's", func(t *testing.T) {
		r := foreignLaunch(t, foreignOpt{named: "Client work", here: true})
		t.Cleanup(store.ReleaseInstanceLock)
		m := resetKey(t, r.m)
		if m.sessionForeign {
			t.Error("the New row over this folder's own general session marked the run")
		}
		if n := logHas(m, "not kept unless the session gets a name"); n != 0 {
			t.Errorf("a run that saves says it is saved only by name (%d lines)", n)
		}
		quitWindow(t, m)
		if ino, mt := fileIdentity(t, r.path); ino == r.ino && mt.Equal(r.mtime) {
			t.Error("the quit did not write this folder's general session")
		}
	})
}

// T9, the other window (review 2026-11-07, finding 1): a window that does not
// own the general session keeps nothing, so it reads nothing at the New row
// either: the read can move a refused file aside, and that is the owner's to
// do. A's refused file stays in place, with no kept copy beside it.
func TestForeignAfterNamedLaunchOtherWindowReadsNothing(t *testing.T) {
	r := foreignLaunch(t, foreignOpt{named: "Client work", refuse: true})
	if _, err := store.AcquireInstanceLock(); err != nil { // another window takes it
		t.Fatal(err)
	}
	t.Cleanup(store.ReleaseInstanceLock)
	m := resetKey(t, r.m)
	if m.named.running() || !m.cfg.SecondWindow {
		t.Fatalf("the New row did not return to the unnamed session as a second window (named %v, second %v)", m.named.running(), m.cfg.SecondWindow)
	}
	if m.sessionForeign {
		t.Error("a window that owns nothing marked the run")
	}
	r.untouched(t, "after the New row in a window that owns nothing")
	entries, err := os.ReadDir(filepath.Dir(r.path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "session.yaml.refused-") {
			t.Errorf("a window that owns nothing moved the refused file aside: %s", e.Name())
		}
	}
	quitWindow(t, m)
	r.untouched(t, "after the quit")
}
