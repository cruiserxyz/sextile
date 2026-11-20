package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
)

// questionAt opens the first-write question (ask) or the change question.
func questionAt(t *testing.T, which string) Model {
	t.Helper()
	if which == "first write" {
		m, _ := saveAt(t, store.PositionAsk)
		m = typeInto(openSave(t, m), "Anna Return")
		m, _ = pressEnter(t, m)
		if !m.save.Confirming {
			t.Fatal("premise: no first-write question")
		}
		return m
	}
	m, _ := renameForm(t, store.PositionAsk)
	m, _ = pressEnter(t, m)
	if !m.title.Confirming {
		t.Fatal("premise: no change question")
	}
	return m
}

// litOf is the lit button of whichever question is open.
func litOf(m Model) string {
	if m.save.Confirming {
		return firstWriteButtons[m.save.Lit].Label
	}
	return changeButtons[m.title.Lit].Label
}

// boxless is a frame with its vertical edges taken off, so a sentence wrapped
// across rows reads as itself.
func boxless(frame string) string {
	return flat(strings.ReplaceAll(frame, "│", "|"))
}

func asking(m Model) bool { return m.save.Confirming || m.title.Confirming }

// TestChoicePopupOpensOnNo: both questions open with No lit.
func TestChoicePopupOpensOnNo(t *testing.T) {
	for _, which := range []string{"first write", "change"} {
		t.Run(which, func(t *testing.T) {
			m := questionAt(t, which)
			if got := litOf(m); got != buttonNo {
				t.Errorf("%s opens with %q lit, want No", which, got)
			}
		})
	}
}

// TestChoicePopupKeys: the arrows move the lit button and stop at the ends,
// Enter takes the lit one, y, a, n and Esc act, and every other key does
// nothing at all.
func TestChoicePopupKeys(t *testing.T) {
	for _, which := range []string{"first write", "change"} {
		buttons := firstWriteButtons
		if which == "change" {
			buttons = changeButtons
		}
		t.Run(which+" arrows", func(t *testing.T) {
			m := questionAt(t, which)
			for i := 0; i < len(buttons)+1; i++ {
				m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyLeft})
			}
			if got := litOf(m); got != buttons[0].Label {
				t.Errorf("after left presses %q is lit, want %q (no wrapping)", got, buttons[0].Label)
			}
			for i := 0; i < len(buttons)+1; i++ {
				m, _ = sendKey(t, m, keyMsg("l"))
			}
			if got := litOf(m); got != buttonNo {
				t.Errorf("after right presses %q is lit, want No (no wrapping)", got)
			}
			m, _ = sendKey(t, m, keyMsg("h"))
			if got := litOf(m); got != buttons[len(buttons)-2].Label {
				t.Errorf("after h %q is lit, want %q", got, buttons[len(buttons)-2].Label)
			}
			if !asking(m) {
				t.Error("moving the lit button closed the question")
			}
		})
		t.Run(which+" enter takes the lit button", func(t *testing.T) {
			m := questionAt(t, which)
			m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyLeft})
			m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
			if cmd == nil || asking(m) || !(m.save.Saving || m.title.Saving) {
				t.Errorf("Enter on %s: cmd %v, asking %v; want it to start", buttons[len(buttons)-2].Label, cmd != nil, asking(m))
			}
		})
		t.Run(which+" y", func(t *testing.T) {
			m := questionAt(t, which)
			m, cmd := sendKey(t, m, keyMsg("y"))
			if cmd == nil || asking(m) {
				t.Errorf("y: cmd %v, asking %v; want it to start", cmd != nil, asking(m))
			}
		})
		for _, k := range []string{"n", "esc"} {
			t.Run(which+" "+k, func(t *testing.T) {
				m := questionAt(t, which)
				m, cmd := sendKey(t, m, keyMsg(k))
				if cmd != nil || asking(m) || m.save.Saving || m.title.Saving {
					t.Errorf("%s: cmd %v, asking %v; want the form back and nothing started", k, cmd != nil, asking(m))
				}
			})
		}
		for _, k := range []string{"x", "q", "3", " ", "tab", "Y", "A", "up", "down"} {
			t.Run(which+" inert "+k, func(t *testing.T) {
				m := questionAt(t, which)
				before := m
				got, cmd := sendKey(t, m, keyMsg(k))
				if cmd != nil || !asking(got) || litOf(got) != litOf(before) || got.save.Saving || got.title.Saving ||
					got.save.typedName() != before.save.typedName() || got.title.Fields.Value(0) != before.title.Fields.Value(0) {
					t.Errorf("%q did something: cmd %v, asking %v, lit %s", k, cmd != nil, asking(got), litOf(got))
				}
			})
		}
	}
	t.Run("a is Always", func(t *testing.T) {
		m := questionAt(t, "first write")
		dir := m.save.dest
		m, cmd := sendKey(t, m, keyMsg("a"))
		finishSave(t, m, cmd)
		if stored(dir) != store.PositionAdd {
			t.Errorf("stored %q after a, want add", stored(dir))
		}
	})
	t.Run("a does nothing in the change question", func(t *testing.T) {
		m := questionAt(t, "change")
		got, cmd := sendKey(t, m, keyMsg("a"))
		if cmd != nil || !got.title.Confirming {
			t.Errorf("a: cmd %v, confirming %v; want nothing", cmd != nil, got.title.Confirming)
		}
	})
}

// TestChoicePopupShowsItsButtons: the lit button is bracketed and the form
// stays on screen behind the popup.
func TestChoicePopupShowsItsButtons(t *testing.T) {
	m := questionAt(t, "first write")
	m.width, m.height = 80, 24
	frame := ansi.Strip(m.View())
	for _, want := range []string{"Yes (y)", "Always (a)", "[ No (n) ]", "Create anna-return.as there?"} {
		if !strings.Contains(unspaced(boxless(frame)), unspaced(want)) {
			t.Errorf("the screen lacks %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "[ Yes (y) ]") {
		t.Errorf("Yes is lit while No should be:\n%s", frame)
	}
}

// TestRefusalsAreNeverCut: every refusal that ends in "Nothing was written."
// shows whole at both sizes, with that sentence on a line of its own.
func TestRefusalsAreNeverCut(t *testing.T) {
	long := "A backup named Equinox-2026-09.20261113T091800Z.as already exists in /data/op/Documents/Sextile/.sextile/backup. Nothing was written."
	texts := []string{
		store.ErrFolderBusy.Error(),
		store.ErrJournal.Error(),
		store.ErrNewFolderPosition.Error(),
		store.ErrFolderAppeared.Error(),
		"The charts folder's lock could not be taken (permission denied). Nothing was written.",
		long,
	}
	for _, size := range [][2]int{{80, 24}, {132, 40}} {
		for _, text := range texts {
			for _, which := range []string{"save", "title"} {
				m := questionAt(t, "first write")
				if which == "title" {
					m = questionAt(t, "change")
					m.title.Confirming = false
					m.title.Refusal = text
				} else {
					m.save.Confirming = false
					m.save.Refusal = text
				}
				frame := ansi.Strip(renderSaveModal(m.save, m.cfg, size[0], size[1]))
				if which == "title" {
					frame = ansi.Strip(renderTitleForm(m.title, m.cfg, size[0], size[1]))
				}
				rows := strings.Split(frame, "\n")
				sentence := strings.TrimSuffix(text, " Nothing was written.")
				if strings.Contains(frame, "…") {
					t.Errorf("%s %dx%d: a cut mark for %q:\n%s", which, size[0], size[1], text, frame)
				}
				if !strings.Contains(unspaced(boxless(frame)), unspaced(sentence)) {
					t.Errorf("%s %dx%d: %q is cut:\n%s", which, size[0], size[1], sentence, frame)
				}
				alone := false
				for _, r := range rows {
					if strings.TrimSpace(strings.Trim(r, "│|+ ")) == "Nothing was written." {
						alone = true
					}
				}
				if !alone {
					t.Errorf("%s %dx%d: \"Nothing was written.\" is not on a line of its own for %q:\n%s", which, size[0], size[1], text, frame)
				}
			}
		}
	}
}

// TestRefusalLayoutSplitsOnce: the split lives in dialog.MessageLines, and a
// message that does not end in the sentence is laid out as it was.
func TestRefusalLayoutSplitsOnce(t *testing.T) {
	got := dialog.MessageLines("Another Sextile window writes this charts folder. Nothing was written.", 80)
	if len(got) != 2 || got[1] != "Nothing was written." {
		t.Errorf("lines %q, want the sentence on its own line", got)
	}
	got = dialog.MessageLines("This charts folder is protected. Nothing was written.\nTo allow saving here, press ? and run Protect the charts folder.", 80)
	if len(got) != 2 {
		t.Errorf("lines %q, want the protected refusal's two lines kept", got)
	}
}

func TestNoticeKeepsItsFirstSentence(t *testing.T) {
	whole := "Another Sextile window writes this charts folder. Nothing was written."
	if got := noticeFitting(whole, 132); got != whole {
		t.Errorf("at 132: %q", got)
	}
	if got := noticeFitting(whole, 60); got != "Another Sextile window writes this charts folder." {
		t.Errorf("at 60: %q, want the first sentence", got)
	}
}
