package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/recipe"
)

// keyCtrlH is what a terminal that sends Backspace as ^H (0x08) delivers:
// bubbletea decodes the byte as KeyCtrlH, a different type from the 0x7f
// KeyBackspace (the specification).
var keyCtrlH = tea.KeyMsg{Type: tea.KeyCtrlH}

func typed(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// Acceptance 1 and 3: in every picker ^H deletes the last character of the
// search text as Backspace does, and on an empty search does nothing and does
// not close the picker. The atlas and chart pickers are driven through the
// whole Update path, so their routing is under test too.
func TestCtrlHDeletesInEveryPicker(t *testing.T) {
	t.Run("atlas", func(t *testing.T) {
		m := detailsTestModel(t)
		tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
		tm, _ = tm.Update(typed("ber"))
		tm, _ = tm.Update(keyCtrlH)
		got := tm.(Model)
		if got.startup != startupAtlasOpen {
			t.Fatalf("^H left the atlas: startup = %q", got.startup)
		}
		if got.atlas.filterText != "be" {
			t.Errorf("filter = %q after ^H, want %q", got.atlas.filterText, "be")
		}
		for range 3 {
			tm, _ = tm.Update(keyCtrlH)
		}
		got = tm.(Model)
		if got.startup != startupAtlasOpen || got.atlas.filterText != "" {
			t.Errorf("^H on an empty atlas search: startup %q, filter %q; want the picker open and empty", got.startup, got.atlas.filterText)
		}
	})
	t.Run("chart", func(t *testing.T) {
		m := detailsTestModel(t)
		tm, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
		open := tm.(Model).startup
		tm, _ = tm.Update(typed("fel"))
		tm, _ = tm.Update(keyCtrlH)
		got := tm.(Model)
		if got.startup != open {
			t.Fatalf("^H left the chart picker: startup %q, want %q", got.startup, open)
		}
		if got.picker.filterText != "fe" {
			t.Errorf("filter = %q after ^H, want %q", got.picker.filterText, "fe")
		}
		for range 3 {
			tm, _ = tm.Update(keyCtrlH)
		}
		got = tm.(Model)
		if got.startup != open || got.picker.filterText != "" {
			t.Errorf("^H on an empty chart search: startup %q, filter %q; want the picker open and empty", got.startup, got.picker.filterText)
		}
	})
	t.Run("aspect", func(t *testing.T) {
		st := openTest(recipe.AspectPolicy{})
		st, _ = updateAspectModalKey(st, typed("squ"), testListHeight)
		st, out := updateAspectModalKey(st, keyCtrlH, testListHeight)
		if out.Confirmed || out.Cancelled {
			t.Fatal("^H ended the aspect picker")
		}
		if st.filterText != "sq" {
			t.Errorf("filter = %q after ^H, want %q", st.filterText, "sq")
		}
		for range 3 {
			st, out = updateAspectModalKey(st, keyCtrlH, testListHeight)
			if out.Confirmed || out.Cancelled {
				t.Fatal("^H on an empty search ended the aspect picker")
			}
		}
		if st.filterText != "" {
			t.Errorf("filter = %q, want empty", st.filterText)
		}
	})
	t.Run("body", func(t *testing.T) {
		st := openBodyTest(recipe.ObjectPolicy{}, nil)
		st, _ = updateBodyModalKey(st, typed("mar"), testListHeight)
		st, out := updateBodyModalKey(st, keyCtrlH, testListHeight)
		if out.Confirmed || out.Cancelled {
			t.Fatal("^H ended the body picker")
		}
		if st.filterText != "ma" {
			t.Errorf("filter = %q after ^H, want %q", st.filterText, "ma")
		}
		for range 3 {
			st, out = updateBodyModalKey(st, keyCtrlH, testListHeight)
			if out.Confirmed || out.Cancelled {
				t.Fatal("^H on an empty search ended the body picker")
			}
		}
		if st.filterText != "" {
			t.Errorf("filter = %q, want empty", st.filterText)
		}
	})
}

// Acceptance 2: every text field built on form.go deletes on ^H as on
// Backspace, and ^H on an empty field leaves it empty.
func TestCtrlHDeletesInFormFields(t *testing.T) {
	f := newForm(formField{label: "Title"})
	f, _ = f.updateKey(typed("Jane"))
	f, handled := f.updateKey(keyCtrlH)
	if !handled {
		t.Fatal("the form did not handle ^H")
	}
	if got := f.fields[0].value; got != "Jan" {
		t.Errorf("value = %q after ^H, want %q", got, "Jan")
	}
	for range 4 {
		f, _ = f.updateKey(keyCtrlH)
	}
	if got := f.fields[0].value; got != "" {
		t.Errorf("value = %q, want empty", got)
	}
}

// Acceptance 4: on the main screen ^H is not consumed by this slice — no
// command runs and nothing is typed anywhere. the specification C13 later
// gave the main screen's ^H its one meaning, the house finder: the model now
// changes by opening it, and by nothing else (Esc puts it back as it was).
func TestCtrlHOutsideAPickerIsNotConsumed(t *testing.T) {
	m := detailsTestModel(t)
	tm, cmd := m.Update(keyCtrlH)
	if cmd != nil {
		t.Error("^H on the main screen returned a command")
	}
	opened := tm.(Model)
	if opened.startup != startupHouseOpen {
		t.Fatalf("^H on the main screen left startup = %q, want the house finder", opened.startup)
	}
	closed, _ := opened.Update(keyEsc)
	back := closed.(Model)
	back.houseFinder = m.houseFinder
	if !modelsLookEqual(m, back) {
		t.Error("^H then Esc on the main screen changed the model")
	}
}
