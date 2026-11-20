package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
)

// readonlyFolder is a charts folder in a sandbox, at the stored position p ("" for
// none), in a run the launcher started read-only.
func readonlyFolder(t *testing.T, p store.Position) string {
	t.Helper()
	sessionSandbox(t)
	dir := filepath.Join(t.TempDir(), "charts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if p != "" {
		if err := store.SetPosition(dir, p); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("SEXTILE_CHARTS_READONLY", "1")
	return dir
}

// In a run started read-only, the folder row cannot open the folder for this run, so the
// refusal never offers it as the way to save now. A folder stored as protected
// is told the two steps that open it; any other read-only run is told that a new
// start is needed. Outside a read-only run the refusal is as before.
func TestReadonlyRunRefusalSaysWhatANewStartNeeds(t *testing.T) {
	m := newTestModel(t, originalsTasteCfg, FrozenClock(time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)))

	stored := readonlyFolder(t, store.PositionProtected)
	got := m.protectedRefusal(refusalSaving, stored)
	if want := "This charts folder is protected.\nNothing was written.\nThis run only reads it. To save here, press ?\nand run Protect the charts folder, then start Sextile again."; got != want {
		t.Errorf("stored protected, read-only run:\n%s\nwant\n%s", got, want)
	}

	flag := readonlyFolder(t, "")
	got = m.protectedRefusal(refusalChanges, flag)
	if want := "This charts folder is read-only for this run.\nNothing was written.\nA new start is needed to save here."; got != want {
		t.Errorf("read-only run, folder not stored as protected:\n%s\nwant\n%s", got, want)
	}

	t.Setenv("SEXTILE_CHARTS_READONLY", "")
	if got := m.protectedRefusal(refusalSaving, stored); !strings.HasSuffix(got, "To allow saving here, press ? and run\nProtect the charts folder.") {
		t.Errorf("an ordinary protected folder: %q", got)
	}
}

// Every refusal fits the save popup and the rename form whole, at 80x24 and
// 132x40: no line of it is cut.
func TestReadonlyRunRefusalsFitTheirPopups(t *testing.T) {
	m := newTestModel(t, originalsTasteCfg, FrozenClock(time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)))
	for _, p := range []store.Position{store.PositionProtected, ""} {
		dir := readonlyFolder(t, p)
		for _, s := range [][2]int{{80, 24}, {132, 40}} {
			save := tasteSaveState(dir, "anna return")
			save.Refusal = m.protectedRefusal(refusalSaving, dir)
			frame := unspaced(flat(renderSaveModal(save, originalsTasteCfg, s[0], s[1])))
			for _, line := range strings.Split(save.Refusal, "\n") {
				if !strings.Contains(frame, unspaced(line)) {
					t.Errorf("%dx%d, position %q: the refusal line %q is cut:\n%s", s[0], s[1], p, line, frame)
				}
			}
		}
	}
}

// The question before a new chart at `ask` is asked before every new chart until
// Always, so its words are true on every save: it never claims a first write.
func TestAskQuestionIsTrueOnEverySave(t *testing.T) {
	state := tasteSaveState("/data/op/Documents/Sextile", "anna return")
	for _, s := range [][2]int{{80, 24}, {132, 40}} {
		frame := flat(dialog.RenderChoice(firstWriteChoice(state), originalsTasteCfg, s[0], s[1], popupChrome(originalsTasteCfg)))
		if strings.Contains(frame, "not written") || !strings.Contains(unspaced(frame), unspaced("Sextile asks before it writes a new chart in")) {
			t.Errorf("%dx%d: the question reads\n%s", s[0], s[1], frame)
		}
	}
}
