// All chart values in this file are constructed synthetic test inputs.
package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// submitSave opens the prompt, types name and presses Enter, returning the
// model and the command Enter produced (nil when the prompt refused).
func submitSave(t *testing.T, m Model, name string) (Model, tea.Cmd) {
	t.Helper()
	m = typeInto(openSave(t, m), name)
	tm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return tm.(Model), cmd
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

// C1, C5: a name over 54 bytes is refused before any Astrolog call and nothing
// is written; exactly 54 saves; a 28-rune accented name (56 bytes) is refused.
func TestSaveRefusesALongName(t *testing.T) {
	for _, c := range []struct {
		name    string
		typed   string
		refused bool
	}{
		{"54 bytes saves", strings.Repeat("a", 54), false},
		{"55 bytes is refused", strings.Repeat("a", 55), true},
		{"28 accented letters (56 bytes) is refused", strings.Repeat("é", 28), true},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := saveBase(t)
			m, cmd := submitSave(t, m, c.typed)
			if c.refused {
				if cmd != nil {
					t.Fatal("a refused name still dispatched a command")
				}
				if m.startup != startupSaveOpen || m.save.typedName() != c.typed || m.save.saving {
					t.Errorf("the prompt did not stay open with the typed name (startup %v, name %q)", m.startup, m.save.typedName())
				}
				if got := dirEntries(t, chartsDirOf(t)); len(got) != 0 {
					t.Errorf("a refused name wrote %v", got)
				}
				return
			}
			if cmd == nil {
				t.Fatalf("a 54-byte name was refused: %q", m.save.refusal)
			}
			done := cmd().(SaveChartDoneMsg)
			if done.Err != nil {
				t.Fatalf("a 54-byte name failed to save: %v", done.Err)
			}
		})
	}
}

// C3: the refusal names the field, its byte count and the bound, says accented
// letters count as 2, and is drawn at 80x24 with the typed text still in the
// field.
func TestLongNameRefusalSaysWhy(t *testing.T) {
	m := saveBase(t)
	typed := strings.Repeat("é", 28)
	m, _ = submitSave(t, m, typed)
	want := "Name too long: 56 of 54 bytes (é counts 2)"
	if m.save.refusal != want {
		t.Fatalf("refusal = %q, want %q", m.save.refusal, want)
	}
	// The prompt's own words, with the box's edges taken off each line: a
	// sentence wrapped inside the box still reads as one.
	var inner []string
	for _, l := range strings.Split(renderSaveModal(m.save, m.cfg, 80, 24), "\n") {
		inner = append(inner, strings.Trim(l, "|│+╭╮╰╯─- "))
	}
	flat := strings.Join(strings.Fields(strings.Join(inner, " ")), " ")
	for _, part := range []string{"Name too long: 56 of 54 bytes (é counts 2)", strings.Repeat("é", 20)} {
		if !strings.Contains(flat, part) {
			t.Errorf("the prompt at 80x24 does not show %q:\n%s", part, flat)
		}
	}
	// It is one row of the message zone, and the box does not grow for it.
	rested := renderSaveModal(saveModalState{form: m.save.form, dest: m.save.dest}, m.cfg, 80, 24)
	if a, b := strings.Count(rested, "\n"), strings.Count(renderSaveModal(m.save, m.cfg, 80, 24), "\n"); a != b {
		t.Errorf("the refusal changed the prompt's height: %d lines, then %d", a+1, b+1)
	}
	// the title form says the same for its fields
	titleLibrary(t, nil, map[string]string{"NoPlace.as": noPlaceChart})
	tm := openedPicker(t)
	tm, _ = pickRow(t, tm, "NoPlace.as")
	tm = typeText(t, tm, "Fine")
	tm.title.form.focus = titlePlaceField
	tm = typeText(t, tm, strings.Repeat("p", 70))
	tm, cmd := sendKey(t, tm, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("a 70-byte place dispatched a write")
	}
	if !strings.HasPrefix(tm.title.refusal, "Place too long: 70 of 54 bytes") {
		t.Errorf("title refusal = %q", tm.title.refusal)
	}
	if tm.title.form.focus != titlePlaceField || !strings.Contains(titleFrame(tm), strings.Repeat("p", 20)) {
		t.Error("the typed place is not still in its field")
	}
}

// C1: a slot place over the bound is never typed here, so it is cut and logged.
func TestSaveCutsALongSlotPlace(t *testing.T) {
	m := saveBase(t)
	// A double quote sits in the part that survives the cut: Astrolog writes it
	// as a single quote, and the Log must name the label as written.
	m.slots.All()[1].Location.Name = strings.Repeat("P", 30) + `"` + strings.Repeat("Q", 50)
	m = runSave(t, m, "Cut Place")
	if m.startup == startupSaveOpen {
		t.Fatalf("the save did not complete: %q", m.save.refusal)
	}
	var cutLine string
	for _, l := range m.logLines {
		if strings.HasPrefix(l.text, "save: Place shortened to 54 bytes for Astrolog: ") {
			cutLine = l.text
		}
	}
	if cutLine == "" {
		t.Fatalf("no 'Place shortened' line in the Log: %+v", m.logLines)
	}
	path := filepath.Join(chartsDirOf(t), "cut-place.as")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	logged := strings.TrimPrefix(cutLine, "save: Place shortened to 54 bytes for Astrolog: ")
	if len(logged) > 54 || !strings.HasSuffix(logged, "…") || strings.Contains(logged, `"`) {
		t.Errorf("the Log names %q, want the cut label as written (a single quote, at most 54 bytes)", logged)
	}
	if !strings.Contains(string(b), `"`+logged+`"`) {
		t.Errorf("the Log's place %q is not the place label in the saved file:\n%s", logged, b)
	}
}

// C2: the title form refuses an over-long title or place and writes nothing.
func TestRenameRefusesALongName(t *testing.T) {
	for _, c := range []struct {
		name         string
		title, place string
		want         string
	}{
		{"title", strings.Repeat("t", 55), "Bremen", "Title too long: 55 of 54 bytes"},
		{"place", "Fine", strings.Repeat("é", 28), "Place too long: 56 of 54 bytes"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := titleLibrary(t, nil, map[string]string{"NoPlace.as": noPlaceChart})
			path := filepath.Join(dir, "NoPlace.as")
			before := fileText(t, path)
			m := openedPicker(t)
			m, _ = pickRow(t, m, "NoPlace.as")
			m = typeText(t, m, c.title)
			m.title.form.focus = titlePlaceField
			m = typeText(t, m, c.place)
			m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
			if cmd != nil || m.title.saving || m.startup != startupTitleOpen {
				t.Fatalf("the over-long %s was submitted (startup %v)", c.name, m.startup)
			}
			if !strings.HasPrefix(m.title.refusal, c.want) {
				t.Errorf("refusal = %q, want prefix %q", m.title.refusal, c.want)
			}
			if fileText(t, path) != before {
				t.Error("the file changed")
			}
			noBackup(t, path)
		})
	}
}
