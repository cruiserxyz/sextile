package ui

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/store"
)

// the specification C1-C3, C6 and the build notes of passes 3, 5, 6.
// The refusal is the can't-start screen with a lock reason shown alone. A lock
// held by another window is not a refusal: that launch is a second window
// (second_window_test.go).

const (
	lockRecordReason = "Couldn't record this window in /x/sextile.lock: input/output error."
	lockRecordRemedy = "Fix that, then start Sextile again."
	lockUnsupportedN = "a second copy cannot be detected on this folder"
)

// lockRecordErr is a refusal that still happens: the lock was taken but the
// holder pid could not be written.
var lockRecordErr = &store.LockError{Kind: store.LockRecord, Path: "/x/sextile.lock", Err: syscall.EIO}

func lockRefusedSized(t *testing.T, cfg Config, err error, w, h int) Model {
	t.Helper()
	var tm tea.Model = NewLockRefusedModel(cfg, err)
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return tm.(Model)
}

func TestLockRefusedScreenFitsAt80x24(t *testing.T) {
	for _, cfg := range []Config{{}, {Accessible: true, Color: ColorNever}} {
		m := lockRefusedSized(t, cfg, lockRecordErr, 80, 24)
		view := m.View()
		lines := strings.Split(view, "\n")
		if len(lines) > 24 {
			t.Errorf("accessible=%v: %d lines, want <= 24:\n%s", cfg.Accessible, len(lines), view)
		}
		for _, l := range lines {
			if w := ansi.StringWidth(l); w > 80 {
				t.Errorf("accessible=%v: a line of %d cells: %q", cfg.Accessible, w, l)
			}
		}
		got := csText(view)
		for _, want := range []string{lockRecordReason, lockRecordRemedy, "q quits"} {
			if !strings.Contains(got, want) {
				t.Errorf("accessible=%v: 80x24 lacks %q:\n%s", cfg.Accessible, want, view)
			}
		}
	}
}

func TestLockRefusedScreenIsNotTheAstrologScreen(t *testing.T) {
	got := csText(lockRefusedSized(t, Config{}, lockRecordErr, 132, 40).View())
	if !strings.Contains(got, "Sextile can't start") {
		t.Errorf("not the can't-start screen:\n%s", got)
	}
	for _, banned := range []string{"Docker", "install Astrolog", "README", "To fix it", "Then start Sextile again", "astrolog binary", "ephemeris", "atlas.as"} {
		if strings.Contains(got, banned) {
			t.Errorf("the lock refusal shows the Astrolog remedy %q:\n%s", banned, got)
		}
	}
}

func TestLockRefusalReasons(t *testing.T) {
	path := "/data/sextile/session/sextile.lock"
	for _, c := range []struct {
		name string
		err  error
		want string
	}{
		{"unsafe path", &store.LockError{Kind: store.LockUnsafe, Path: path}, "Couldn't use " + path + ": it is not a plain file of its own."},
		{"cannot open", &store.LockError{Kind: store.LockOpen, Path: path, Err: syscall.EACCES}, "Couldn't open " + path + ": permission denied."},
		{"other flock error", &store.LockError{Kind: store.LockFlock, Path: path, Err: syscall.EIO}, "Couldn't lock " + path + ": input/output error."},
		{"pid not written", &store.LockError{Kind: store.LockRecord, Path: path, Err: syscall.EIO}, "Couldn't record this window in " + path + ": input/output error."},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := csText(lockRefusedSized(t, Config{}, c.err, 132, 40).View())
			if !strings.Contains(got, "Sextile can't start") || !strings.Contains(got, c.want) {
				t.Errorf("view lacks %q:\n%s", c.want, got)
			}
			if !strings.Contains(got, "then start Sextile again.") || !strings.Contains(got, "q quits") {
				t.Errorf("view lacks a remedy and the quit key:\n%s", got)
			}
			if strings.Contains(got, "Docker") || strings.Contains(got, "install Astrolog") {
				t.Errorf("shows the Astrolog remedies:\n%s", got)
			}
		})
	}
}

// The refusal starts nothing: no Astrolog probe, no perimeter read, no atlas
// or registry load (pass 6, "the losing launch reads nothing").
func TestLockRefusedModelStartsNothing(t *testing.T) {
	if cmd := NewLockRefusedModel(Config{}, lockRecordErr).Init(); cmd != nil {
		t.Errorf("Init returned a command: it would start work for a launch that was refused")
	}
	// ...and the normal model still does, so the assertion above is not vacuous.
	if NewModel(Config{}).Init() == nil {
		t.Fatal("the normal model's Init returns no command; the control is meaningless")
	}
}

// Quitting the refusal writes no session, whichever reason it carries. The
// model is given a workspace on purpose: today's m.quit() writes one whenever
// m.slots is set, and only an explicit refusal guard stops that.
func TestLockRefusalQuitWritesNoSession(t *testing.T) {
	for name, err := range map[string]error{
		"record": lockRecordErr,
		"unsafe": &store.LockError{Kind: store.LockUnsafe, Path: "/x/sextile.lock"},
		"open":   &store.LockError{Kind: store.LockOpen, Path: "/x/sextile.lock", Err: syscall.EACCES},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", t.TempDir())
			m := lockRefusedSized(t, Config{}, err, 80, 24)
			m.slots = readyModelWithSlots(t, berlinP).slots
			for _, k := range []tea.KeyMsg{escRunes("q"), escKey(tea.KeyCtrlC)} {
				tm, cmd := m.Update(k)
				if cmd == nil {
					t.Fatalf("%v did not quit", k)
				}
				if _, ok := cmd().(tea.QuitMsg); !ok {
					t.Fatalf("%v did not quit", k)
				}
				if e := tm.(Model).SessionWriteError(); e != nil {
					t.Errorf("session write attempted: %v", e)
				}
			}
			if _, statErr := os.Stat(store.SessionPath()); !os.IsNotExist(statErr) {
				t.Errorf("quitting the refusal screen wrote %s (stat: %v)", store.SessionPath(), statErr)
			}
			if entries, _ := os.ReadDir(store.SessionDir()); len(entries) != 0 {
				t.Errorf("quitting the refusal screen created %d entries in the session folder", len(entries))
			}
		})
	}
}

func TestLockRefusedOnlyQuits(t *testing.T) {
	m := lockRefusedSized(t, Config{}, lockRecordErr, 80, 24)
	frame := m.View()
	for name, k := range map[string]tea.KeyMsg{
		"letter": escRunes("a"), "ctrl+s": escKey(tea.KeyCtrlS), "enter": escKey(tea.KeyEnter),
		"burst": escRunes("abcdefghij"), "leader": escRunes(m.leaderKey),
	} {
		tm, cmd := m.Update(k)
		if cmd != nil {
			t.Errorf("%s returned a command", name)
		}
		if tm.(Model).View() != frame {
			t.Errorf("%s changed the frame", name)
		}
	}
}

// C6: where flock is not supported the app runs, and the Log says so once, on
// the note lane.
func TestUnsupportedLockLogsOnce(t *testing.T) {
	notes := func(m Model) (out []string) {
		for _, l := range m.logLines {
			if l.lane == laneNote {
				out = append(out, l.text)
			}
		}
		return
	}
	m := NewModel(Config{InstanceLockUnsupported: true})
	got := notes(m)
	if len(got) != 1 || !strings.Contains(got[0], lockUnsupportedN) {
		t.Fatalf("note lane = %q, want exactly one line holding %q", got, lockUnsupportedN)
	}
	if strings.HasSuffix(got[0], ".") {
		t.Errorf("a note ends without a full stop (A3): %q", got[0])
	}
	// Later startup messages do not write it again.
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 132, Height: 40})
	if n := notes(tm.(Model)); len(n) != 1 {
		t.Errorf("after a resize the note lane holds %d lines, want 1: %q", len(n), n)
	}
	if n := notes(NewModel(Config{})); len(n) != 0 {
		t.Errorf("a normal launch wrote notes: %q", n)
	}
	if !strings.Contains(csText(tm.(Model).View()), lockUnsupportedN) {
		t.Errorf("the 6-row Log does not show the note at 132x40:\n%s", tm.(Model).View())
	}
}

func TestLockRefusedModelKnowsAnUnclassifiedError(t *testing.T) {
	got := csText(lockRefusedSized(t, Config{}, errors.New("boom"), 132, 40).View())
	if !strings.Contains(got, "Sextile can't start") || !strings.Contains(got, "boom") {
		t.Errorf("an error that is not a LockError must still refuse and say why:\n%s", got)
	}
}
