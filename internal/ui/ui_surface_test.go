package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/store"
)

// frozenInstantForGolden is the canonical "now" for slot-rendering goldens;
// matches the invariants-package frozen instant for consistency.
func frozenInstantForGolden() time.Time {
	return time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
}

// slotsForGolden constructs a 2-slot workspace (floating in `floating`,
// anchored at `anchorWhen` in `anchorPlace`) for golden snapshots.
func slotsForGolden(floating location.PlaceInfo, anchorWhen time.Time, anchorPlace location.PlaceInfo) (*store.Slots, error) {
	s, err := store.NewSlots(frozenInstantForGolden(), floating)
	if err != nil {
		return nil, err
	}
	if err := addAnchor(s, anchorWhen, anchorPlace); err != nil {
		return nil, err
	}
	return s, nil
}

func TestUISurfaceGolden(t *testing.T) {
	cases := []struct {
		name       string
		width      int
		height     int
		helpOpen   bool
		accessible bool
	}{
		{"single-column-80x24", 80, 24, false, false},
		{"dual-column-132x40", 132, 40, false, false},
		{"single-column-80x24-help", 80, 24, true, false},
		{"dual-column-132x40-help", 132, 40, true, false},
		{"single-column-80x24-accessible", 80, 24, false, true},
		{"dual-column-132x40-accessible", 132, 40, false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var m tea.Model = NewModel(Config{Accessible: tc.accessible})
			m, _ = m.Update(tea.WindowSizeMsg{Width: tc.width, Height: tc.height})
			if tc.helpOpen {
				m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
			}
			got := m.View()

			checkGolden(t, tc.name, got)
		})
	}
}

func focused(t *testing.T, m tea.Model) PanelID {
	t.Helper()
	mm, ok := m.(Model)
	if !ok {
		t.Fatalf("not a Model: %T", m)
	}
	return mm.focused
}

func logLines(t *testing.T, m tea.Model) []logLine {
	t.Helper()
	mm, ok := m.(Model)
	if !ok {
		t.Fatalf("not a Model: %T", m)
	}
	return mm.logLines
}

// TestFocusCycleSkipsPassivePanels verifies Tab / Shift+Tab never land on
// details or log — they are passive.
func TestFocusCycleSkipsPassivePanels(t *testing.T) {
	var m tea.Model = NewModel(Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 40})

	if got := focused(t, m); got != PanelChart {
		t.Fatalf("initial focus: %q, want chart", got)
	}

	for _, want := range []PanelID{PanelBodies, PanelAspects, PanelChart} {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
		if got := focused(t, m); got != want {
			t.Errorf("next cycle: got %q, want %q", got, want)
		}
	}

	for _, want := range []PanelID{PanelAspects, PanelBodies, PanelChart} {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
		if got := focused(t, m); got != want {
			t.Errorf("prev cycle: got %q, want %q", got, want)
		}
	}
}

// TestQuitOnlyInMainState verifies `q` quits from main UI but is a no-op
// while a modal is open it closes help as Esc does and never quits.
func TestQuitOnlyInMainState(t *testing.T) {
	var m tea.Model = NewModel(Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("`q` in main state: expected tea.Quit command, got nil")
	}
	if msg := cmd(); msg != (tea.QuitMsg{}) {
		t.Fatalf("`q` in main state: expected tea.QuitMsg, got %T", msg)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !m.(Model).helpOpen {
		t.Fatal("help modal should be open after `?`")
	}

	after, cmd2 := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd2 != nil {
		t.Fatalf("`q` in modal state: expected no command, got %T", cmd2)
	}
	if after.(Model).helpOpen {
		t.Fatal("`q` in the help modal must close it as Esc does")
	}
}

// TestPanelNavKeysNotShadowedByTimeStepBindings verifies the post-an earlier change
// reclamation of `h` and the deletion of the provisional `l`/`h` vim
// nav: `h` now drives the canonical hour time-step (pitch.md:119);
// `l` is unbound. Panel cycling lives on tab/shift+tab only. This
// guards against accidental re-introduction of the collision.
func TestPanelNavKeysNotShadowedByTimeStepBindings(t *testing.T) {
	var m tea.Model = NewModel(Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	initial := focused(t, m)

	// `l` should be a no-op for panel cycling now (the provisional vim
	// binding was removed). Focus must not change from the cold-start
	// chart panel.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if got := focused(t, m); got != initial {
		t.Errorf("`l` should no longer cycle panels; focus = %q, want unchanged %q", got, initial)
	}

	// `tab` is the canonical next-panel key and must still work.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := focused(t, m); got == initial {
		t.Errorf("`tab` should cycle panels; focus stayed %q", got)
	}
}

// TestNumericDirectJumps verifies `1`/`2`/`3` focus chart/bodies/aspects.
func TestNumericDirectJumps(t *testing.T) {
	var m tea.Model = NewModel(Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	for _, c := range []struct {
		k    string
		want PanelID
	}{
		{"2", PanelBodies},
		{"3", PanelAspects},
		{"1", PanelChart},
	} {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(c.k)})
		if got := focused(t, m); got != c.want {
			t.Errorf("key %s: focused = %q, want %q", c.k, got, c.want)
		}
	}
}

// TestMouseClickFocusesFocusablePanel verifies left-click inside a
// focusable panel's rectangle moves focus; click elsewhere is a no-op.
func TestMouseClickFocusesFocusablePanel(t *testing.T) {
	var m tea.Model = NewModel(Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})

	click := tea.MouseMsg{X: 10, Y: 13, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	m, _ = m.Update(click)
	if got := focused(t, m); got != PanelBodies {
		t.Errorf("click in bodies: focused = %q, want bodies", got)
	}

	click2 := tea.MouseMsg{X: 100, Y: 5, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	m, _ = m.Update(click2)
	if got := focused(t, m); got != PanelBodies {
		t.Errorf("click on details (passive): focused = %q, want bodies", got)
	}

	click3 := tea.MouseMsg{X: 10, Y: 39, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	m, _ = m.Update(click3)
	if got := focused(t, m); got != PanelBodies {
		t.Errorf("click on the key bar: focused = %q, want bodies", got)
	}
}

// TestResizeAcrossBreakpoint verifies focus survives crossing 132.
func TestResizeAcrossBreakpoint(t *testing.T) {
	var m tea.Model = NewModel(Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if got := focused(t, m); got != PanelChart {
		t.Errorf("initial focus: got %q, want chart", got)
	}
	if m.View() == "" {
		t.Error("empty view at 100x30")
	}

	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	if got := focused(t, m); got != PanelChart {
		t.Errorf("focus lost across resize: got %q, want chart", got)
	}
	if m.View() == "" {
		t.Error("empty view at 160x40")
	}

	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if got := focused(t, m); got != PanelChart {
		t.Errorf("focus lost on shrink: got %q, want chart", got)
	}
}

// TestResizeWithHelpOpen verifies modal survives resize.
func TestResizeWithHelpOpen(t *testing.T) {
	var m tea.Model = NewModel(Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !m.(Model).helpOpen {
		t.Fatal("help should be open after `?`")
	}

	m, _ = m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	if !m.(Model).helpOpen {
		t.Error("modal should stay open across resize")
	}
	if m.View() == "" {
		t.Error("empty view with help open after resize")
	}
}

// TestTerminalTooSmall verifies the friendly fallback names required size.
func TestTerminalTooSmall(t *testing.T) {
	var m tea.Model = NewModel(Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	out := m.View()
	if out == "" {
		t.Fatal("empty view at 40x10")
	}
	if !strings.Contains(out, "too small") {
		t.Errorf("expected 'too small' hint, got:\n%s", out)
	}
	if !strings.Contains(out, "66") || !strings.Contains(out, "15") {
		t.Errorf("expected required dims (66 / 15) in hint, got:\n%s", out)
	}
}

// TestModalPreservesStoredFocus verifies the stored focus is preserved while
// a modal is open and still correct after the modal closes. The UI's
// visual suppression while modal-open is captured by the help-open goldens.
func TestModalPreservesStoredFocus(t *testing.T) {
	var m tea.Model = NewModel(Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 132, Height: 40})

	// Move focus off the default.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := focused(t, m); got != PanelBodies {
		t.Fatalf("pre-modal focus: got %q, want bodies", got)
	}

	// Open modal — stored focus must not change.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if got := focused(t, m); got != PanelBodies {
		t.Errorf("during modal: focus should stay bodies, got %q", got)
	}

	// Close modal — stored focus is still bodies (now visually active again).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := focused(t, m); got != PanelBodies {
		t.Errorf("after modal close: focus should still be bodies, got %q", got)
	}
}

// TestRoutineInteractionsDoNotPolluteLog keeps the Log panel for actionable
// failures, warnings, and hints rather than resize/focus/modal key chatter.
func TestRoutineInteractionsDoNotPolluteLog(t *testing.T) {
	var m tea.Model = NewModel(Config{})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if logs := logLines(t, m); len(logs) != 0 {
		t.Errorf("routine interactions polluted Log: %v", logs)
	}
}

// assertSelfContainedFrame pins the one property that lets a frame be read as
// a frame instead of as a byte soup: every escape it carries is an SGR
// sequence, and every line closes the styles it opens.
//
// Both halves have a present customer and neither is about publication. A
// stray cursor-move, scroll or clear sequence would make the frame unreadable
// with `cat` and would mean View() had started composing what bubbletea's
// renderer owns; an OSC would mean a test had reached the runner's terminal,
// which console.go's yankLine seam exists to prevent and which was until now
// only a hand-maintained comment. A line that leaves SGR open bleeds its
// colour into the next line and into the reader's shell, which is what makes
// a colour diff unreadable.
func assertSelfContainedFrame(t *testing.T, label, content string) {
	t.Helper()
	sgr := regexp.MustCompile(`^\x1b\[[0-9;]*m`)
	for i, line := range strings.Split(content, "\n") {
		depth := 0
		for at := 0; at < len(line); at++ {
			if line[at] != 0x1b {
				continue
			}
			seq := sgr.FindString(line[at:])
			if seq == "" {
				t.Errorf("%s line %d: non-SGR escape at byte %d: %q",
					label, i+1, at, line[at:min(at+8, len(line))])
				break
			}
			if params := seq[2 : len(seq)-1]; params == "" || params == "0" {
				depth = 0
			} else {
				depth++
			}
			at += len(seq) - 1
		}
		if depth != 0 {
			t.Errorf("%s line %d: ends with %d unreset SGR span(s); a frame line must close what it opens",
				label, i+1, depth)
		}
	}
}

// TestGoldenCorpusIsSelfContainedFrames applies that property to the whole
// corpus, so the corpus never acquires a rule about how goldens present
// themselves.
func TestGoldenCorpusIsSelfContainedFrames(t *testing.T) {
	corpora := []string{"testdata"}
	for _, dir := range corpora {
		matches, err := filepath.Glob(filepath.Join(dir, "*.golden"))
		if err != nil {
			t.Fatalf("glob %s: %v", dir, err)
		}
		if len(matches) == 0 {
			t.Fatalf("no goldens found under %s", dir)
		}
		for _, path := range matches {
			t.Run(filepath.Base(path), func(t *testing.T) {
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("read %s: %v", path, err)
				}
				assertSelfContainedFrame(t, path, string(raw))
			})
		}
	}
}
