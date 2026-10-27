// All chart values reachable from this file are constructed synthetic test inputs.
package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// The form chassis, the specification item 2 (C2.2–C2.8). C2.1, the save
// prompt rendering byte-identically through it, is TestSavePromptTasteFrames.

// twoFieldForm is the shape item 3's title form will take: a title field with
// today's name as its hint, and a place field.
func twoFieldForm() form {
	return newForm(
		formField{label: "Title: ", placeholder: "Fixture Untitled"},
		formField{label: "Place: ", placeholder: "Constructed Town"},
	)
}

// typeIntoForm types keys into f the way a caller's handler does: Tab and
// ⇧tab are the caller's table entries (nextField, prevField), everything else
// is the form's text entry.
func typeIntoForm(f form, keys ...tea.KeyMsg) form {
	for _, k := range keys {
		switch k.Type {
		case tea.KeyTab:
			f = f.nextField()
		case tea.KeyShiftTab:
			f = f.prevField()
		default:
			f, _ = f.updateKey(k)
		}
	}
	return f
}

func formRunes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

var (
	keyFormTab      = tea.KeyMsg{Type: tea.KeyTab}
	keyFormShiftTab = tea.KeyMsg{Type: tea.KeyShiftTab}
	keyFormBack     = tea.KeyMsg{Type: tea.KeyBackspace}
)

func formFrame(f form, cfg Config) string {
	return renderForm(f, formView{
		Title: "Test form",
		Info:  []string{"Fixture.as"},
	}, cfg, 80, 24)
}

// lineWith returns the frame's one line containing label, failing if there is
// not exactly one.
func lineWith(t *testing.T, frame, label string) string {
	t.Helper()
	var found []string
	for _, l := range strings.Split(frame, "\n") {
		if strings.Contains(l, label) {
			found = append(found, l)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one line holding %q, got %d in\n%s", label, len(found), frame)
	}
	return found[0]
}

// C2.2: several fields, each with its label and value, and exactly one of them
// carrying the cursor -- the focused one.
func TestFormFields(t *testing.T) {
	f := typeIntoForm(twoFieldForm(), formRunes("Constructed One"), keyFormTab, formRunes("Bremen"))
	frame := formFrame(f, Config{Accessible: true, Color: ColorNever})

	title, place := lineWith(t, frame, "Title: "), lineWith(t, frame, "Place: ")
	if !strings.Contains(title, "Title: Constructed One") {
		t.Errorf("title line = %q, want its label and typed value", title)
	}
	if !strings.Contains(place, "Place: Bremen█") {
		t.Errorf("place line = %q, want the focused field's value followed by the cursor", place)
	}
	if strings.Contains(title, "█") {
		t.Errorf("title line = %q carries a cursor, but focus is on the place field", title)
	}
	if n := strings.Count(frame, "█"); n != 1 {
		t.Errorf("the frame carries %d cursors, want exactly 1:\n%s", n, frame)
	}
	if got := f.values(); len(got) != 2 || got[0] != "Constructed One" || got[1] != "Bremen" {
		t.Errorf("values() = %q, want each field's own value in order", got)
	}
}

// C2.3 and C2.4: with colour the placeholder is faint, with the focused
// field's cursor drawn in reverse video ON its first character; without colour
// no placeholder is drawn at all. It goes at the first typed character and
// comes back when the value is erased.
func TestFormPlaceholder(t *testing.T) {
	forceColor(t)
	color := Config{Color: ColorAlways}
	f := twoFieldForm()
	const atRest = "Title: " + sgrReverse + "F" + sgrReset + sgrFaint + "ixture Untitled"
	if line := lineWith(t, formFrame(f, color), "Title: "); !strings.Contains(line, atRest) {
		t.Fatalf("empty focused field = %q, want %q", line, atRest)
	}
	if strings.Contains(formFrame(f, color), "█") {
		t.Error("the colour frame draws a block cursor beside the hint")
	}
	line := lineWith(t, formFrame(f, color), "Place: ")
	if !strings.Contains(line, sgrFaint+"Constructed Town") || strings.Contains(line, sgrReverse) {
		t.Errorf("unfocused empty field = %q, want its whole hint, faint, and no cursor", line)
	}

	f = typeIntoForm(f, formRunes("C"))
	if line := lineWith(t, formFrame(f, color), "Title: "); strings.Contains(line, "ixture Untitled") || !strings.Contains(line, "Title: C█") {
		t.Errorf("after one typed character the line is %q, want the typed text and no hint", line)
	}
	f = typeIntoForm(f, keyFormBack)
	if line := lineWith(t, formFrame(f, color), "Title: "); !strings.Contains(line, atRest) {
		t.Errorf("after erasing back to empty the hint did not return: %q", line)
	}

	plain := formFrame(twoFieldForm(), Config{Accessible: true, Color: ColorNever})
	if strings.Contains(plain, "Fixture Untitled") || strings.Contains(plain, "Constructed Town") {
		t.Errorf("the colourless frame draws a placeholder:\n%s", plain)
	}
	// The top edge carries the popup's bold title, as a panel's does with
	// colours off; nothing below it is styled.
	_, body, _ := strings.Cut(plain, "\n")
	if !strings.Contains(lineWith(t, plain, "Title: "), "Title: █") || strings.Contains(body, "\x1b[") {
		t.Errorf("the colourless frame should show a bare cursor and no SGR below its title:\n%q", plain)
	}
}

// C2.5: a placeholder never becomes a value.
func TestFormSubmitsTypedValuesOnly(t *testing.T) {
	f := twoFieldForm()
	if got := f.values(); got[0] != "" || got[1] != "" {
		t.Fatalf("an untouched form submits %q, want two empty values", got)
	}
	f = typeIntoForm(f, keyFormTab, formRunes("Bremen"))
	if got := f.values(); got[0] != "" || got[1] != "Bremen" {
		t.Errorf("values() = %q, want the untouched title empty and the place as typed", got)
	}
	f = typeIntoForm(f, keyFormBack, keyFormBack, keyFormBack, keyFormBack, keyFormBack, keyFormBack, keyFormBack)
	if got := f.value(1); got != "" {
		t.Errorf("a value erased past empty is %q, want \"\" (backspace on empty is a no-op)", got)
	}
}

// C2.6: Tab and Shift+Tab cycle, wrapping; with one field they change nothing.
func TestFormFocusCycle(t *testing.T) {
	f := newForm(formField{label: "A: "}, formField{label: "B: "}, formField{label: "C: "})
	for i, want := range []int{1, 2, 0, 1} {
		f = typeIntoForm(f, keyFormTab)
		if f.focus != want {
			t.Fatalf("after %d Tab presses focus = %d, want %d", i+1, f.focus, want)
		}
	}
	for i, want := range []int{0, 2, 1} {
		f = typeIntoForm(f, keyFormShiftTab)
		if f.focus != want {
			t.Fatalf("after %d Shift+Tab presses focus = %d, want %d", i+1, f.focus, want)
		}
	}

	one := typeIntoForm(newSaveForm(), formRunes("Constructed"))
	for _, got := range []form{one.nextField(), one.prevField()} {
		if got.focus != 0 || got.value(0) != "Constructed" {
			t.Errorf("a field move on a one-field form: focus %d, value %q; want 0, unchanged", got.focus, got.value(0))
		}
	}
	// Enter, Esc and Tab are the caller's table's, never the form's.
	for _, k := range []tea.KeyMsg{{Type: tea.KeyEnter}, {Type: tea.KeyEsc}, keyFormTab, keyFormShiftTab} {
		if _, handled := one.updateKey(k); handled {
			t.Errorf("the form handled %v; it must leave it to the caller", k)
		}
	}
}

// C2.7: the box keeps its size across values, placeholders, the focused field,
// a value too long for its line and a refusal: the form keeps one empty line
// under its info from the moment it opens, a refusal appears in that line, and
// one too long for it is cut (operator, 2026-10-27: a popup's size is decided
// when it opens and never changes while it is open).
func TestFormBoxSizeStable(t *testing.T) {
	forceColor(t)
	long := strings.Repeat("Constructed Very Long Title ", 6)
	const refusal = "Constructed.as changed since the list was read — reopen the picker"
	states := []struct {
		name    string
		f       form
		refusal string
	}{
		{"placeholders", twoFieldForm(), ""},
		{"typed", typeIntoForm(twoFieldForm(), formRunes("Constructed One")), ""},
		{"second-focused", typeIntoForm(twoFieldForm(), keyFormTab, formRunes("Bremen")), ""},
		{"too-long", typeIntoForm(twoFieldForm(), formRunes(long)), ""},
		{"refused", typeIntoForm(twoFieldForm(), formRunes("Constructed One")), refusal},
		{"refused, longer than its line", typeIntoForm(twoFieldForm(), formRunes("Constructed One")), refusal + " and then a good deal more than one line holds"},
	}
	for _, cfg := range []Config{{Color: ColorAlways}, {Accessible: true, Color: ColorNever}} {
		var wantLines, wantWidth int
		for i, s := range states {
			frame := renderForm(s.f, formView{Title: "Test form", Info: []string{"Fixture.as"}, Refusal: s.refusal}, cfg, 80, 24)
			lines := strings.Split(frame, "\n")
			width := 0
			for _, l := range lines {
				width = max(width, ansi.StringWidth(l))
			}
			if i == 0 {
				wantLines, wantWidth = len(lines), width
				continue
			}
			if len(lines) != wantLines || width != wantWidth {
				t.Errorf("%+v %s: %d lines × %d wide, want %d × %d as at rest:\n%s", cfg, s.name, len(lines), width, wantLines, wantWidth, frame)
			}
			if s.refusal != "" && !strings.Contains(ansi.Strip(frame), ansi.Strip(refusal[:30])) {
				t.Errorf("%+v %s: the refusal is not in the form:\n%s", cfg, s.name, frame)
			}
		}
	}

	// The too-long value shows its tail, so the cursor stays on screen.
	frame := formFrame(typeIntoForm(twoFieldForm(), formRunes(long+"END")), Config{Accessible: true, Color: ColorNever})
	if line := lineWith(t, frame, "Title: "); !strings.Contains(line, "END█") || !strings.Contains(line, "Title: ...") {
		t.Errorf("long value line = %q, want the label, an ellipsis, and the tail ending at the cursor", line)
	}

	// A two-field form is one line taller than the one-field save prompt: the
	// second field is a line of its own, and nothing else is added.
	one := strings.Split(renderSaveModal(saveModalState{form: newSaveForm(), dest: tasteSaveDest}, Config{Accessible: true, Color: ColorNever}, 80, 24), "\n")
	two := strings.Split(formFrame(twoFieldForm(), Config{Accessible: true, Color: ColorNever}), "\n")
	if len(two) != len(one)+1 {
		t.Errorf("one-field box is %d lines, two-field box %d; want one more (the field)", len(one), len(two))
	}
}

// C2.8 (second field): a paste into a field that is not the first is
// sanitised the same way the save prompt's one field is.
func TestFormSanitisesPaste(t *testing.T) {
	f := typeIntoForm(twoFieldForm(), keyFormTab, formRunes("Bremen,\nGermany\t\r1"))
	if got := f.value(1); got != "Bremen, Germany  1" {
		t.Errorf("pasted place = %q, want each control character turned into a space", got)
	}
	if got := f.value(0); got != "" {
		t.Errorf("the paste leaked into the unfocused field: %q", got)
	}
	frame := formFrame(f, Config{Accessible: true, Color: ColorNever})
	if !strings.Contains(lineWith(t, frame, "Place: "), "Bremen, Germany  1█") {
		t.Errorf("the pasted value did not stay on one line:\n%s", frame)
	}
}
