// All chart values reachable from this file are constructed synthetic test inputs.
package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// The screen half of the charts folder's door (specs/originals-door.md): the
// refusal, the two questions, `, w` pressed twice and the launch's lines. Each
// test sets the folder's position through the position file, as a user or the
// launcher would, and reads what was written back from the disk.

// originalsWorld points the position file, the backups and the journal at
// temp folders and returns a charts folder that exists.
func originalsWorld(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("SEXTILE_CHARTS_READONLY", "")
	dir := filepath.Join(root, "charts")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SEXTILE_CHARTS_DIR", dir)
	t.Setenv("SEXTILE_DEV_CHARTS_DIR", "")
	return dir
}

// positioned puts dir at p, as a hand edit of the position file does, so the
// test's own process holds no lock on the folder; "" leaves it with no line.
func positioned(t *testing.T, dir string, p store.Position) {
	t.Helper()
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(store.PositionFile()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.PositionFile(), []byte(folderIdentityFor(t, dir)+"="+string(p)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// saveAt is a ready model with the save prompt's folder at p.
func saveAt(t *testing.T, p store.Position) (Model, string) {
	t.Helper()
	dir := originalsWorld(t)
	positioned(t, dir, p)
	m := arrowBase(t)
	m.cursorIdx = 1
	return m, dir
}

// pressEnter presses Enter.
func pressEnter(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	return sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
}

// stored is the word the position file holds for dir, "" for none.
func stored(dir string) store.Position { return store.ReadPosition(dir).Stored }

// traced reports whether the large log holds a trace line with sub.
func traced(m Model, sub string) bool {
	for _, l := range m.logLines {
		if l.lane == laneTrace && strings.Contains(l.text, sub) {
			return true
		}
	}
	return false
}

// noted counts the Log's note lines that are exactly text.
func noted(m Model, text string) int {
	n := 0
	for _, l := range m.logLines {
		if l.lane == laneNote && l.text == text {
			n++
		}
	}
	return n
}

// savedFiles lists the chart files in dir.
func savedFiles(t *testing.T, dir string) []string {
	t.Helper()
	hits, err := filepath.Glob(filepath.Join(dir, "*"+store.ChartFileExt))
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

// finishSave runs a save command and folds its message in.
func finishSave(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("the answer started no save")
	}
	done, ok := cmd().(SaveChartDoneMsg)
	if !ok {
		t.Fatalf("the save command returned %T", cmd())
	}
	u, _ := m.Update(done)
	return u.(Model)
}

// O6: a protected folder is unchanged by a save and by a rename, and no
// Astrolog process starts.
func TestProtectedFolderIsUnchanged(t *testing.T) {
	t.Run("save", func(t *testing.T) {
		m, dir := saveAt(t, store.PositionProtected)
		spawns := chartFileCapturingAstrolog(t)
		before := diskSnapshot(t, dir)
		m = typeInto(openSave(t, m), "Anna Return")
		m, cmd := pressEnter(t, m)
		if cmd != nil || m.save.Saving || m.save.Confirming {
			t.Fatalf("cmd %v, saving %v, confirming %v; want a refusal", cmd != nil, m.save.Saving, m.save.Confirming)
		}
		if m.startup != startupSaveOpen || m.save.typedName() != "Anna Return" {
			t.Errorf("startup %s, name %q; want the prompt open with the typed name", m.startup, m.save.typedName())
		}
		if !strings.Contains(m.save.Refusal, "This charts folder is protected.\nNothing was written.") || !strings.Contains(m.save.Refusal, "To allow saving here, press ? and run\nProtect the charts folder.") {
			t.Errorf("refusal %q", m.save.Refusal)
		}
		if after := diskSnapshot(t, dir); after != before {
			t.Errorf("the folder changed:\n--- before\n%s\n--- after\n%s", before, after)
		}
		if got := spawns(); len(got) != 0 {
			t.Errorf("%d Astrolog processes started", len(got))
		}
	})
	t.Run("rename", func(t *testing.T) {
		dir := renameLibrary(t)
		sessionSandbox(t)
		t.Setenv("SEXTILE_CHARTS_DIR", dir)
		positioned(t, dir, store.PositionProtected)
		m := openedPicker(t)
		m = cursorTo(t, m, "Titled.as")
		m, _ = ctrlR(t, m)
		m = eraseField(t, m)
		m = typeText(t, m, "Renamed Titled")
		spawns := chartFileCapturingAstrolog(t)
		before := diskSnapshot(t, dir)
		m, cmd := pressEnter(t, m)
		if cmd != nil || m.title.Saving || m.title.Confirming {
			t.Fatalf("cmd %v, saving %v, confirming %v; want a refusal", cmd != nil, m.title.Saving, m.title.Confirming)
		}
		if !strings.Contains(m.title.Refusal, "To allow changes here, press ? and run\nProtect the charts folder.") || m.title.Fields.Value(0) != "Renamed Titled" {
			t.Errorf("refusal %q, title %q", m.title.Refusal, m.title.Fields.Value(0))
		}
		if after := diskSnapshot(t, dir); after != before {
			t.Errorf("the folder changed:\n--- before\n%s\n--- after\n%s", before, after)
		}
		if got := spawns(); len(got) != 0 {
			t.Errorf("%d Astrolog processes started", len(got))
		}
	})
}

// O6b: in the refusal a comma is text, as in every field.
func TestCommaStillTypesInTheRefusal(t *testing.T) {
	refused := func(t *testing.T) Model {
		m, _ := saveAt(t, store.PositionProtected)
		m = typeInto(openSave(t, m), "Anna")
		m, _ = pressEnter(t, m)
		if m.save.Refusal == "" {
			t.Fatal("premise: no refusal")
		}
		return m
	}
	for _, tc := range []struct {
		name string
		keys []string
		want string
	}{
		{"comma then letter", []string{",", "x"}, "Anna,x"},
		{"comma, c, then letter", []string{",", "c", "x"}, "Anna,cx"},
		{"comma, c, comma, then letter", []string{",", "c", ",", "x"}, "Anna,c,x"},
		{"comma, w", []string{",", "w"}, "Anna,w"},
		{"a letter", []string{"x"}, "Annax"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := refused(t)
			m, _ = press(t, m, tc.keys...)
			if got := m.save.typedName(); got != tc.want {
				t.Errorf("name %q, want %q", got, tc.want)
			}
			if m.save.Refusal != "" {
				t.Errorf("refusal %q, want it cleared by the first typed character", m.save.Refusal)
			}
		})
	}
}

// O7: at ask, a confirmed name shows the question before anything starts.
func TestFirstWriteAsks(t *testing.T) {
	for _, p := range []store.Position{"", store.PositionAsk} {
		t.Run("stored "+string(p), func(t *testing.T) {
			m, dir := saveAt(t, p)
			spawns := chartFileCapturingAstrolog(t)
			m = typeInto(openSave(t, m), "Anna Return 2026")
			m, cmd := pressEnter(t, m)
			if cmd != nil || m.save.Saving || !m.save.Confirming {
				t.Fatalf("cmd %v, saving %v, confirming %v; want the question and no save", cmd != nil, m.save.Saving, m.save.Confirming)
			}
			frame := flat(dialog.RenderChoice(firstWriteChoice(m.save), m.cfg, 132, 40, popupChrome(m.cfg)))
			for _, want := range []string{"Sextile asks before it writes a new chart in", dir, "Create anna-return-2026.as there?"} {
				if !strings.Contains(unspaced(frame), unspaced(want)) {
					t.Errorf("the question lacks %q:\n%s", want, frame)
				}
			}
			if got, want := keys.BarLine(m.keysContext(), 100), "Choose: <left>/<right> | Take: <enter> | Yes: y | Always: a | No: n/<esc>"; got != want {
				t.Errorf("bar %q, want %q", got, want)
			}
			if files := savedFiles(t, dir); len(files) != 0 {
				t.Errorf("files written before the answer: %v", files)
			}
			if got := spawns(); len(got) != 0 {
				t.Errorf("%d Astrolog processes started before the answer", len(got))
			}
		})
	}
}

// O8: y saves once, a saves and remembers add, n, Esc and Enter on the lit No write nothing.
func TestFirstWriteAnswers(t *testing.T) {
	asked := func(t *testing.T) (Model, string) {
		m, dir := saveAt(t, store.PositionAsk)
		m = typeInto(openSave(t, m), "Anna Return")
		m, _ = pressEnter(t, m)
		if !m.save.Confirming {
			t.Fatal("premise: no question")
		}
		return m, dir
	}
	t.Run("y", func(t *testing.T) {
		m, dir := asked(t)
		m, cmd := sendKey(t, m, keyMsg("y"))
		finishSave(t, m, cmd)
		if files := savedFiles(t, dir); len(files) != 1 || filepath.Base(files[0]) != "anna-return.as" {
			t.Errorf("files %v, want anna-return.as", files)
		}
		if got := store.ReadPosition(dir); got.Effective != store.PositionAsk || got.Stored != store.PositionAsk {
			t.Errorf("position after y: %+v, want it still ask", got)
		}
	})
	t.Run("a", func(t *testing.T) {
		m, dir := asked(t)
		m, cmd := sendKey(t, m, keyMsg("a"))
		finishSave(t, m, cmd)
		if files := savedFiles(t, dir); len(files) != 1 {
			t.Errorf("files %v, want one chart", files)
		}
		if stored(dir) != store.PositionAdd {
			t.Errorf("stored %q after a, want add", stored(dir))
		}
	})
	for _, k := range []string{"n", "esc", "enter"} {
		t.Run("other "+k, func(t *testing.T) {
			m, dir := asked(t)
			spawns := chartFileCapturingAstrolog(t)
			m, cmd := sendKey(t, m, keyMsg(k))
			if cmd != nil || m.save.Saving || m.save.Confirming {
				t.Fatalf("cmd %v, saving %v, confirming %v; want the name back", cmd != nil, m.save.Saving, m.save.Confirming)
			}
			if m.startup != startupSaveOpen || m.save.typedName() != "Anna Return" {
				t.Errorf("startup %s, name %q; want the prompt open with the typed name", m.startup, m.save.typedName())
			}
			if files := savedFiles(t, dir); len(files) != 0 || stored(dir) != store.PositionAsk {
				t.Errorf("files %v, stored %q; want nothing written", files, stored(dir))
			}
			if got := spawns(); len(got) != 0 {
				t.Errorf("%d Astrolog processes started", len(got))
			}
		})
	}
}

// O9: add and allow save with no question, and so does a folder that does not
// exist yet.
func TestOwnFolderSavesWithoutQuestion(t *testing.T) {
	for _, p := range []store.Position{store.PositionAdd, store.PositionAllow} {
		t.Run(string(p), func(t *testing.T) {
			m, dir := saveAt(t, p)
			m = typeInto(openSave(t, m), "Anna Return")
			m, cmd := pressEnter(t, m)
			if m.save.Confirming || !m.save.Saving {
				t.Fatalf("confirming %v, saving %v; want the save to start", m.save.Confirming, m.save.Saving)
			}
			finishSave(t, m, cmd)
			if files := savedFiles(t, dir); len(files) != 1 {
				t.Errorf("files %v, want one chart", files)
			}
		})
	}
	t.Run("a folder that does not exist", func(t *testing.T) {
		m, dir := saveAt(t, "")
		fresh := filepath.Join(dir, "new")
		t.Setenv("SEXTILE_CHARTS_DIR", fresh)
		m = typeInto(openSave(t, m), "Anna Return")
		m, cmd := pressEnter(t, m)
		if m.save.Confirming || !m.save.Saving {
			t.Fatalf("confirming %v, saving %v; want the save to start", m.save.Confirming, m.save.Saving)
		}
		finishSave(t, m, cmd)
		if files := savedFiles(t, fresh); len(files) != 1 {
			t.Errorf("files %v, want one chart", files)
		}
		if stored(fresh) != store.PositionAdd {
			t.Errorf("the new folder is at %q, want add", stored(fresh))
		}
	})
}

// renameForm is a Ctrl+R rename of Titled.as typed to a new title, not yet
// submitted, in a folder at p.
func renameForm(t *testing.T, p store.Position) (Model, string) {
	t.Helper()
	dir := renameLibrary(t)
	sessionSandbox(t)
	t.Setenv("SEXTILE_CHARTS_DIR", dir)
	positioned(t, dir, p)
	m := openedPicker(t)
	m = cursorTo(t, m, "Titled.as")
	m, _ = ctrlR(t, m)
	m = eraseField(t, m)
	return typeText(t, m, "Renamed Titled"), dir
}

// O11: at ask and add a rename shows the change question first.
func TestRenameAsks(t *testing.T) {
	for _, p := range []store.Position{"", store.PositionAsk, store.PositionAdd} {
		t.Run("stored "+string(p), func(t *testing.T) {
			m, dir := renameForm(t, p)
			spawns := chartFileCapturingAstrolog(t)
			before := diskSnapshot(t, dir)
			m, cmd := pressEnter(t, m)
			if cmd != nil || m.title.Saving || !m.title.Confirming {
				t.Fatalf("cmd %v, saving %v, confirming %v; want the question", cmd != nil, m.title.Saving, m.title.Confirming)
			}
			if m.title.stamp.IsZero() || m.title.stamp.Location() != time.UTC {
				t.Errorf("stamp %v, want a UTC time fixed when the question opened", m.title.stamp)
			}
			frame := flat(titleFrame(m))
			for _, want := range []string{"Change Titled.as?", "title " + "Constructed Titled", "Renamed Titled", "(unchanged)", store.BackupName("Titled.as", m.title.stamp)} {
				if !strings.Contains(unspaced(frame), unspaced(want)) {
					t.Errorf("the question lacks %q:\n%s", want, titleFrame(m))
				}
			}
			if after := diskSnapshot(t, dir); after != before {
				t.Errorf("the folder changed before the answer")
			}
			if got := spawns(); len(got) != 0 {
				t.Errorf("%d Astrolog processes started before the answer", len(got))
			}
		})
	}
}

// O11a: y changes the file and keeps the copy the question named; n, Esc and
// Enter on the lit No write nothing and returns to the form with its text.
func TestRenameAnswers(t *testing.T) {
	t.Run("y", func(t *testing.T) {
		m, dir := renameForm(t, store.PositionAsk)
		m, _ = pressEnter(t, m)
		stamp := m.title.stamp
		m, cmd := yes(t, m)
		if cmd == nil || !m.title.Saving {
			t.Fatal("y started no change")
		}
		m, _ = finishRename(t, m, cmd)
		path := filepath.Join(dir, "Titled.as")
		if got := fileText(t, path); !strings.Contains(got, "Renamed Titled") {
			t.Errorf("the file was not changed:\n%s", got)
		}
		if got, want := filepath.Base(backupFile(path)), store.BackupName("Titled.as", stamp); got != want {
			t.Errorf("the dated copy is %s, the question named %s", got, want)
		}
	})
	for _, k := range []string{"n", "enter", "esc"} {
		t.Run("other "+k, func(t *testing.T) {
			m, dir := renameForm(t, store.PositionAsk)
			m, _ = pressEnter(t, m)
			spawns := chartFileCapturingAstrolog(t)
			before := diskSnapshot(t, dir)
			m, cmd := sendKey(t, m, keyMsg(k))
			if cmd != nil || m.title.Saving || m.title.Confirming || m.startup != startupTitleOpen {
				t.Fatalf("cmd %v, saving %v, confirming %v, startup %s; want the form", cmd != nil, m.title.Saving, m.title.Confirming, m.startup)
			}
			if got := m.title.Fields.Value(0); got != "Renamed Titled" {
				t.Errorf("title %q, want the typed text kept", got)
			}
			if after := diskSnapshot(t, dir); after != before {
				t.Error("the folder changed")
			}
			if got := spawns(); len(got) != 0 {
				t.Errorf("%d Astrolog processes started", len(got))
			}
		})
	}
}

// O25: at allow a rename asks nothing and still keeps a dated copy and
// writes a journal line.
func TestAllowStillKeepsCopyAndJournal(t *testing.T) {
	m, dir := renameForm(t, store.PositionAllow)
	m, cmd := pressEnter(t, m)
	if m.title.Confirming || cmd == nil || !m.title.Saving {
		t.Fatalf("confirming %v, saving %v; want the change to start at once", m.title.Confirming, m.title.Saving)
	}
	finishRename(t, m, cmd)
	path := filepath.Join(dir, "Titled.as")
	if got := fileText(t, path); !strings.Contains(got, "Renamed Titled") {
		t.Errorf("the file was not changed:\n%s", got)
	}
	if copies := datedCopies(path); len(copies) != 1 {
		t.Errorf("dated copies %v, want one", copies)
	}
	journal := fileText(t, store.JournalPath(dir))
	if !strings.Contains(journal, "\tchange\t\"Titled.as\"\t") {
		t.Errorf("the journal lacks the change line:\n%s", journal)
	}
}

// O16a: a write that failed after its journal line, with the line saying so
// not appended, is named in the Log.
func TestUncorrectedJournalIsReported(t *testing.T) {
	cause := fmt.Errorf("%w (anna-return.as): %w", store.ErrJournalUncorrected, errors.New("disk full"))
	const want = "journal: its last line names a write to anna-return.as that did not happen"
	t.Run("save", func(t *testing.T) {
		m, _ := saveAt(t, store.PositionAdd)
		m = typeInto(openSave(t, m), "Anna Return")
		m.save.Saving = true
		u, _ := m.Update(SaveChartDoneMsg{Gen: m.saveGen, Name: "Anna Return", Err: cause})
		if got := noted(u.(Model), want); got != 1 {
			t.Errorf("the Log holds the line %d times: %v", got, logTexts(u.(Model)))
		}
	})
	t.Run("rename", func(t *testing.T) {
		m, _ := renameForm(t, store.PositionAllow)
		m, _ = pressEnter(t, m)
		chart := m.title.chart
		cause := fmt.Errorf("%w (Titled.as): %w", store.ErrJournalUncorrected, errors.New("disk full"))
		u, _ := m.Update(TitleWrittenMsg{Gen: m.titleGen, Chart: chart, Err: cause})
		const wantRename = "journal: its last line names a write to Titled.as that did not happen"
		if got := noted(u.(Model), wantRename); got != 1 {
			t.Errorf("the Log holds the line %d times: %v", got, logTexts(u.(Model)))
		}
	})
}

// keyWorld is a main screen whose charts folder is at p.
func keyWorld(t *testing.T, p store.Position) (Model, string) {
	t.Helper()
	dir := originalsWorld(t)
	positioned(t, dir, p)
	return leaderModel(t), dir
}

const protectedLine = "charts folder: protected, Sextile writes nothing here"

// O18: the folder row on a protected folder makes it ask.
func TestKeyOpensAProtectedFolder(t *testing.T) {
	const asks = "charts folder: Sextile asks before each write"
	t.Run("stored protected", func(t *testing.T) {
		m, dir := keyWorld(t, store.PositionProtected)
		m = pressFolder(t, m)
		if stored(dir) != store.PositionAsk || noted(m, asks) != 1 {
			t.Errorf("stored %q, line count %d; want ask and %q once: %v", stored(dir), noted(m, asks), asks, logTexts(m))
		}
	})
	t.Run("the read-only variable is set", func(t *testing.T) {
		m, dir := keyWorld(t, store.PositionProtected)
		t.Setenv("SEXTILE_CHARTS_READONLY", "1")
		m = pressFolder(t, m)
		const line = "charts folder: asks first from the next start, writes nothing now"
		if stored(dir) != store.PositionAsk || noted(m, line) != 1 {
			t.Errorf("stored %q, line count %d; want ask and %q once: %v", stored(dir), noted(m, line), line, logTexts(m))
		}
		if got := store.ReadPosition(dir).Effective; got != store.PositionProtected {
			t.Errorf("this run is at %q, want it still protected", got)
		}
	})
}

// O19: the folder row on ask, add or no line protects the folder, in one press.
func TestKeyProtectsAFolder(t *testing.T) {
	for _, p := range []store.Position{"", store.PositionAsk, store.PositionAdd} {
		t.Run("stored "+string(p), func(t *testing.T) {
			m, dir := keyWorld(t, p)
			m = pressFolder(t, m)
			if stored(dir) != store.PositionProtected || noted(m, protectedLine) != 1 {
				t.Errorf("stored %q, line count %d; want protected and %q once: %v", stored(dir), noted(m, protectedLine), protectedLine, logTexts(m))
			}
			if _, err := store.OriginalsDoor(dir).Place("anna", []byte("x")); !errors.Is(err, store.ErrOriginalsProtected) {
				t.Errorf("the door answered %v, want it to refuse", err)
			}
		})
	}
}

// The folder row protects in one press and the same press opens it again; `, w` and
// Ctrl+W (keys_one_map_test.go) do nothing.
func TestOneCWPressProtectsAndTheNextOpens(t *testing.T) {
	m, dir := keyWorld(t, store.PositionAsk)
	m = pressFolder(t, m)
	if stored(dir) != store.PositionProtected || noted(m, protectedLine) != 1 {
		t.Fatalf("one press of the folder row: stored %q, line count %d, want protected and %q once", stored(dir), noted(m, protectedLine), protectedLine)
	}
	m = pressFolder(t, m)
	if stored(dir) != store.PositionAsk {
		t.Errorf("the second press of the folder row: stored %q, want ask", stored(dir))
	}
	if m.leader.pending {
		t.Error("the leader is still pending")
	}
}

func TestCommaWAloneDoesNothing(t *testing.T) {
	m, dir := keyWorld(t, store.PositionAsk)
	before := len(m.logLines)
	m, _ = press(t, m, ",", "w")
	if stored(dir) != store.PositionAsk || m.leader.pending || len(m.logLines) != before {
		t.Errorf("`, w`: stored %q, pending %v, log lines %d (was %d); want nothing", stored(dir), m.leader.pending, len(m.logLines), before)
	}
}

// O20: at allow the file is unchanged, the Log names the file, the large log
// holds its path.
func TestKeyLeavesAllowAlone(t *testing.T) {
	m, dir := keyWorld(t, store.PositionAllow)
	before := fileText(t, store.PositionFile())
	m = pressFolder(t, m)
	const line = "charts folder: no question before a write, set in launcher-originals.conf"
	if got := fileText(t, store.PositionFile()); got != before || stored(dir) != store.PositionAllow {
		t.Errorf("the position file changed:\n%s", got)
	}
	if noted(m, line) != 1 {
		t.Errorf("the Log lacks %q: %v", line, logTexts(m))
	}
	if !traced(m, store.PositionFile()) {
		t.Errorf("the large log lacks the full path %s", store.PositionFile())
	}
}

// O20a: a faulty file is unchanged, and the Log names the fault.
func TestKeyLeavesAFaultyFileAlone(t *testing.T) {
	m, dir := keyWorld(t, "")
	id := folderIdentityFor(t, dir)
	faulty := id + "=ask\n" + id + "=add\n"
	if err := os.MkdirAll(filepath.Dir(store.PositionFile()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.PositionFile(), []byte(faulty), 0o600); err != nil {
		t.Fatal(err)
	}
	m = pressFolder(t, m)
	const line = "charts folder: protected, launcher-originals.conf has 2 lines for it"
	if got := fileText(t, store.PositionFile()); got != faulty {
		t.Errorf("the position file changed:\n%s", got)
	}
	if noted(m, line) != 1 {
		t.Errorf("the Log lacks %q: %v", line, logTexts(m))
	}
	if !traced(m, store.PositionFile()) {
		t.Errorf("the large log lacks the full path %s", store.PositionFile())
	}
}

// folderIdentityFor is the identity the position file keys dir by: the line
// SetPosition writes for it, read back.
func folderIdentityFor(t *testing.T, dir string) string {
	t.Helper()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	return filepath.Clean(abs)
}

// O22: the command is in the registry, it has no comma form, and Keybindings
// shows it.
func TestOriginalsKeyIsListed(t *testing.T) {
	var entry *keys.BindingEntry
	for _, e := range mainBindings(keys.DefaultKeys()) {
		if e.ID == keys.BindingOriginals {
			e := e
			entry = &e
		}
	}
	if entry == nil || !entry.Palette || entry.Title != "Protect the charts folder, or not" {
		t.Fatalf("registry entry: %+v", entry)
	}
	for _, b := range defaultLeaderMap() {
		if b.ID == keys.BindingOriginals {
			t.Errorf("the leader map still has %v for the folder row", b.Seq)
		}
	}
	for _, e := range mainBindings(keys.DefaultKeys()) {
		if e.ID == keys.BindingTimeStepWeek && !e.Binding.Enabled() {
			t.Error("bare w no longer steps a week")
		}
	}
	m := leaderModel(t)
	var row string
	for _, l := range keybindingLines(m) {
		if l.Binding && strings.HasPrefix(l.Text, "Protect the charts folder, or not ") {
			row = l.Text
		}
	}
	if row == "" {
		t.Error("Keybindings does not list the folder row")
	}
	m = openCommandsPopup(t, paletteModel(t))
	found := false
	for _, title := range commandTitles(m) {
		found = found || title == "Protect the charts folder, or not"
	}
	if !found {
		t.Errorf("the popup lacks the row: %v", commandTitles(m))
	}
}

// O23: leftover staging files are named once at the start and left alone.
func TestLeftoverTempFilesAreNamed(t *testing.T) {
	start := func(t *testing.T, names ...string) (Model, string) {
		dir := originalsWorld(t)
		for _, n := range names {
			if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		before := diskSnapshot(t, dir)
		m, _ := applyStartupPerimeter(newTestModel(t, Config{}, RealClock{}), perimeter())
		if after := diskSnapshot(t, dir); after != before {
			t.Errorf("the start changed the folder:\n--- before\n%s\n--- after\n%s", before, after)
		}
		return m, dir
	}
	t.Run("one", func(t *testing.T) {
		m, _ := start(t, ".GoodFella.4183920.sextile-tmp")
		const line = "charts folder: a crash left .GoodFella.4183920.sextile-tmp"
		if noted(m, line) != 1 {
			t.Errorf("the Log lacks %q: %v", line, logTexts(m))
		}
	})
	t.Run("several", func(t *testing.T) {
		m, _ := start(t, ".a.1.sextile-tmp", ".b.2.sextile-tmp", ".c.3.sextile-tmp")
		const line = "charts folder: a crash left 3 .sextile-tmp files"
		if noted(m, line) != 1 {
			t.Errorf("the Log lacks %q: %v", line, logTexts(m))
		}
	})
	t.Run("one with a name too long for a Log line", func(t *testing.T) {
		long := "." + strings.Repeat("Long", 15) + ".4183920.sextile-tmp"
		m, _ := start(t, long)
		const line = "charts folder: a crash left 1 file ending in .sextile-tmp"
		if noted(m, line) != 1 {
			t.Errorf("the Log lacks %q: %v", line, logTexts(m))
		}
	})
	t.Run("none", func(t *testing.T) {
		m, _ := start(t, "anna.as")
		for _, l := range logTexts(m) {
			if strings.HasPrefix(l, "charts folder") {
				t.Errorf("the Log says %q for a folder with no leftover", l)
			}
		}
	})
}

// O2a: the reason a position file is distrusted goes to the large log, once,
// and the Log panel says nothing.
func TestFailClosedReasonIsTraced(t *testing.T) {
	dir := originalsWorld(t)
	id := folderIdentityFor(t, dir)
	if err := os.MkdirAll(filepath.Dir(store.PositionFile()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.PositionFile(), []byte(id+"=ask\n"+id+"=add\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, _ := applyStartupPerimeter(newTestModel(t, Config{}, RealClock{}), perimeter())
	want := "charts folder position: launcher-originals.conf has 2 lines for it (" + store.PositionFile() + ")"
	n := 0
	for _, l := range m.logLines {
		if l.lane == laneTrace && l.text == want {
			n++
		}
		if l.lane != laneTrace && strings.Contains(l.text, "launcher-originals.conf") {
			t.Errorf("the Log panel says %q", l.text)
		}
	}
	if n != 1 {
		t.Errorf("the large log holds %q %d times: %v", want, n, m.logLines)
	}
}

// O26: where the screen has no Log panel the key bar shows the line, and the
// large log holds it at every size.
func TestOriginalsLinesShowWithoutALogPanel(t *testing.T) {
	press2 := func(t *testing.T, w, h int) Model {
		m, _ := keyWorld(t, store.PositionAsk)
		m.width, m.height = w, h
		return func() Model { m = pressFolder(t, m); return m }()
	}
	t.Run("no Log panel", func(t *testing.T) {
		m := press2(t, 80, 24)
		if m.logOnScreen() {
			t.Fatal("premise: 80x24 shows a Log panel")
		}
		if got := m.footerIndicator(); got != protectedLine {
			t.Errorf("footer %q, want %q", got, protectedLine)
		}
		if noted(m, protectedLine) != 1 {
			t.Errorf("the large log lacks the line: %v", logTexts(m))
		}
	})
	t.Run("a Log panel", func(t *testing.T) {
		m := press2(t, 132, 40)
		if !m.logOnScreen() {
			t.Fatal("premise: 132x40 shows no Log panel")
		}
		if got := m.footerIndicator(); got == protectedLine {
			t.Errorf("the footer repeats the Log's line %q", got)
		}
		if noted(m, protectedLine) != 1 {
			t.Errorf("the Log lacks the line: %v", logTexts(m))
		}
	})
	t.Run("the notice fits the bar", func(t *testing.T) {
		m := press2(t, 80, 24)
		if w := ansi.StringWidth(keys.RenderBar(m.keysContext(), 80)); w > 80 {
			t.Errorf("the bar is %d cells wide on an 80-cell terminal", w)
		}
	})
}

// An answer reads the position again: a folder protected since the question
// opened gets the refusal and no Astrolog process, and `a` never writes add
// over a word set since.
func TestAnswerReadsThePositionAgain(t *testing.T) {
	asked := func(t *testing.T) (Model, string) {
		m, dir := saveAt(t, store.PositionAsk)
		m = typeInto(openSave(t, m), "Anna Return")
		m, _ = pressEnter(t, m)
		if !m.save.Confirming {
			t.Fatal("premise: no question")
		}
		return m, dir
	}
	for _, k := range []string{"y", "a"} {
		t.Run("protected meanwhile, "+k, func(t *testing.T) {
			m, dir := asked(t)
			positioned(t, dir, store.PositionProtected)
			spawns := chartFileCapturingAstrolog(t)
			before := diskSnapshot(t, dir)
			m, cmd := sendKey(t, m, keyMsg(k))
			if cmd != nil || m.save.Saving || m.save.Refusal == "" {
				t.Errorf("cmd %v, saving %v, refusal %q; want the refusal", cmd != nil, m.save.Saving, m.save.Refusal)
			}
			if stored(dir) != store.PositionProtected {
				t.Errorf("stored %q, want protected kept", stored(dir))
			}
			if got := spawns(); len(got) != 0 {
				t.Errorf("%d Astrolog processes started", len(got))
			}
			if diskSnapshot(t, dir) != before {
				t.Error("the folder changed")
			}
		})
	}
	t.Run("allow meanwhile, a", func(t *testing.T) {
		m, dir := asked(t)
		positioned(t, dir, store.PositionAllow)
		m, cmd := sendKey(t, m, keyMsg("a"))
		finishSave(t, m, cmd)
		if stored(dir) != store.PositionAllow {
			t.Errorf("stored %q, want the hand-set allow kept", stored(dir))
		}
	})
	t.Run("rename, protected meanwhile", func(t *testing.T) {
		m, dir := renameForm(t, store.PositionAsk)
		m, _ = pressEnter(t, m)
		positioned(t, dir, store.PositionProtected)
		spawns := chartFileCapturingAstrolog(t)
		before := diskSnapshot(t, dir)
		m, cmd := yes(t, m)
		if cmd != nil || m.title.Saving || m.title.Refusal == "" {
			t.Errorf("cmd %v, saving %v, refusal %q; want the refusal", cmd != nil, m.title.Saving, m.title.Refusal)
		}
		if got := spawns(); len(got) != 0 {
			t.Errorf("%d Astrolog processes started", len(got))
		}
		if diskSnapshot(t, dir) != before {
			t.Error("the folder changed")
		}
	})
}

// holdFolder takes dir's writing lock through a file of its own, as another
// Sextile window would, and returns the release.
func holdFolder(t *testing.T, dir string) func() {
	t.Helper()
	path := store.BackupDir(dir) + ".writer.lock"
	os.MkdirAll(filepath.Dir(path), 0o700)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	release := func() { f.Close() }
	t.Cleanup(release)
	return release
}

const busyText = "Another Sextile window writes this charts folder. Nothing was written."

// O29: while another window holds the folder, a save, a rename and `, w` write
// nothing and start no Astrolog process; the refusal shows where V1's does,
// the typed text is kept; after the holder exits, the act goes on to its
// question.
func TestSecondWindowWritesNothing(t *testing.T) {
	t.Run("save", func(t *testing.T) {
		m, dir := saveAt(t, store.PositionAsk)
		release := holdFolder(t, dir)
		spawns := chartFileCapturingAstrolog(t)
		before := diskSnapshot(t, dir)
		m = typeInto(openSave(t, m), "Anna Return")
		m, _ = pressEnter(t, m)
		if m.save.Refusal != busyText || m.save.Confirming || m.save.typedName() != "Anna Return" {
			t.Errorf("refusal %q, confirming %v, name %q", m.save.Refusal, m.save.Confirming, m.save.typedName())
		}
		if got := spawns(); len(got) != 0 {
			t.Errorf("%d Astrolog processes started", len(got))
		}
		if diskSnapshot(t, dir) != before {
			t.Error("the folder changed")
		}
		release()
		m, _ = pressEnter(t, m)
		if !m.save.Confirming {
			t.Errorf("after the holder left, no question (refusal %q)", m.save.Refusal)
		}
	})
	t.Run("rename", func(t *testing.T) {
		m, dir := renameForm(t, store.PositionAsk)
		holdFolder(t, dir)
		spawns := chartFileCapturingAstrolog(t)
		before := diskSnapshot(t, dir)
		m, _ = pressEnter(t, m)
		if m.title.Refusal != busyText || m.title.Confirming || m.title.Fields.Value(0) != "Renamed Titled" {
			t.Errorf("refusal %q, confirming %v, title %q", m.title.Refusal, m.title.Confirming, m.title.Fields.Value(0))
		}
		if got := spawns(); len(got) != 0 {
			t.Errorf("%d Astrolog processes started", len(got))
		}
		if diskSnapshot(t, dir) != before {
			t.Error("the folder changed")
		}
	})
	t.Run("the folder row", func(t *testing.T) {
		m, dir := keyWorld(t, store.PositionAsk)
		holdFolder(t, dir)
		m = pressFolder(t, m)
		if stored(dir) != store.PositionAsk {
			t.Errorf("stored %q, want ask kept", stored(dir))
		}
		if noted(m, "charts folder: another Sextile window writes this charts folder, nothing was written") != 1 {
			t.Errorf("the Log lacks the refusal: %v", logTexts(m))
		}
	})
}

// pressFolder runs the charts folder row of Keybindings: `?`, the row found by
// its words, Enter. The row is the one way to it now that `, c w` is gone.
func pressFolder(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = tpress(t, m, runes("?"))
	if !m.keybindingsOpen {
		t.Fatal("? did not open Keybindings")
	}
	m = kbType(t, m, "protect the charts folder")
	if len(m.keybindings.Matches) != 1 {
		t.Fatalf("%d rows match the folder row, want one", len(m.keybindings.Matches))
	}
	m, _ = tpress(t, m, keyEnter)
	return m
}
