// All chart values reachable from this file are constructed synthetic test inputs.
package ui

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// V1/V2 of the specification — the save prompt's frames and what
// the Log says afterwards, frozen at the wording the operator approved on
// 2026-09-20 (§ approved: the folder is elided so the filename is never cut).
//
// dest is a FIXED string rather than store.ChartsDir(), so these goldens are
// the same on every machine. The resolver's own behaviour is C12's, in
// internal/store.
const tasteSaveDest = "/home/u/.local/share/sextile/charts"

func saveTasteStates() []struct {
	name  string
	state saveModalState
} {
	return []struct {
		name  string
		state saveModalState
	}{
		{"empty", saveModalState{form: newSaveForm(), dest: tasteSaveDest}},
		{"typed", saveModalState{form: typedSaveForm("Midsummer Transit"), dest: tasteSaveDest}},
		{"taken", saveModalState{
			form:    typedSaveForm("Autumn Morning"),
			dest:    tasteSaveDest,
			refusal: `"autumn-morning" is taken — try another name`,
		}},
		// A save in flight renders byte-identically to "typed", so it has no
		// frame of its own.
	}
}

// typedSaveForm is the save prompt's form after the operator typed name.
func typedSaveForm(name string) form {
	f := newSaveForm()
	f.fields[0].value = name
	return f
}

// TestSavePromptTasteFrames is V1: the prompt at both supported widths, in
// each state the operator can put it in, with colour and without.
//
// The "-color" frames were captured on the code BEFORE the save prompt drew
// through the form (the specification C2.1), so they pin today's look
// across that refactor rather than whatever the refactor produced.
func TestSavePromptTasteFrames(t *testing.T) {
	forceColor(t)
	modes := []struct {
		suffix string
		cfg    Config
	}{
		{"", Config{Accessible: true, Color: ColorNever}},
		{"-color", Config{Color: ColorAlways}},
	}
	for _, mode := range modes {
		for _, size := range []struct{ w, h int }{{80, 24}, {132, 40}} {
			for _, s := range saveTasteStates() {
				name := fmt.Sprintf("save-prompt-%s-%dx%d%s", s.name, size.w, size.h, mode.suffix)
				t.Run(name, func(t *testing.T) {
					got := renderSaveModal(s.state, mode.cfg, size.w, size.h)
					checkGolden(t, name, got)
				})
			}
		}
	}
}

// TestSaveLogTasteLines is V2: the sentences a save leaves in the Log. They
// are frozen as text rather than as a rendered panel because the panel's width
// is the Log's business and these lines are this slice's.
func TestSaveLogTasteLines(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(t *testing.T) Model
	}{
		{"save-log-success", func(t *testing.T) Model {
			return runSave(t, saveBase(t), "Midsummer Transit")
		}},
		// A refusal adds NO line: it is shown in the prompt, which stays
		// open, so the Log still holds only the first save's confirmation.
		// The golden's content is that absence.
		{"save-log-refusal-adds-no-line", func(t *testing.T) Model {
			m := runSave(t, saveBase(t), "Autumn Morning")
			return runSave(t, m, "Autumn Morning")
		}},
		{"save-log-unplaced", func(t *testing.T) Model {
			m := saveBase(t)
			floating := m.slots.All()[1]
			floating.Location.Name = "Nowhere"
			floating.Location.Lat, floating.Location.Lon = -30.0, -140.0
			return runSave(t, m, "Midsummer Transit")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.run(t)
			var lines []string
			for _, l := range m.logLines {
				text := l.text
				// The success line carries an absolute path through a
				// t.TempDir(); only its tail is stable across runs.
				if i := strings.Index(text, tasteSaveDest); i >= 0 {
					text = text[:i] + "<charts>" + text[i+len(tasteSaveDest):]
				}
				if i := strings.Index(text, os.TempDir()); i >= 0 {
					if j := strings.LastIndex(text, "/"); j > i {
						text = text[:i] + "<charts>" + text[j:]
					}
				}
				lines = append(lines, fmt.Sprintf("%d %s", l.lane, text))
			}
			checkGolden(t, tc.name, strings.Join(lines, "\n")+"\n")
		})
	}
}
