package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
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
		if got.atlas.FilterText != "be" {
			t.Errorf("filter = %q after ^H, want %q", got.atlas.FilterText, "be")
		}
		for range 3 {
			tm, _ = tm.Update(keyCtrlH)
		}
		got = tm.(Model)
		if got.startup != startupAtlasOpen || got.atlas.FilterText != "" {
			t.Errorf("^H on an empty atlas search: startup %q, filter %q; want the picker open and empty", got.startup, got.atlas.FilterText)
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
		if got.picker.FilterText != "fe" {
			t.Errorf("filter = %q after ^H, want %q", got.picker.FilterText, "fe")
		}
		for range 3 {
			tm, _ = tm.Update(keyCtrlH)
		}
		got = tm.(Model)
		if got.startup != open || got.picker.FilterText != "" {
			t.Errorf("^H on an empty chart search: startup %q, filter %q; want the picker open and empty", got.startup, got.picker.FilterText)
		}
	})
	t.Run("aspect", func(t *testing.T) {
		st := openTest(recipe.AspectPolicy{})
		st, _ = updateAspectModalKey(st, typed("squ"), testListHeight)
		st, out := updateAspectModalKey(st, keyCtrlH, testListHeight)
		if out.Confirmed || out.Cancelled {
			t.Fatal("^H ended the aspect picker")
		}
		if st.FilterText != "sq" {
			t.Errorf("filter = %q after ^H, want %q", st.FilterText, "sq")
		}
		for range 3 {
			st, out = updateAspectModalKey(st, keyCtrlH, testListHeight)
			if out.Confirmed || out.Cancelled {
				t.Fatal("^H on an empty search ended the aspect picker")
			}
		}
		if st.FilterText != "" {
			t.Errorf("filter = %q, want empty", st.FilterText)
		}
	})
	t.Run("body", func(t *testing.T) {
		st := openBodyTest(recipe.ObjectPolicy{}, nil)
		st, _ = updateBodyModalKey(st, typed("mar"), testListHeight)
		st, out := updateBodyModalKey(st, keyCtrlH, testListHeight)
		if out.Confirmed || out.Cancelled {
			t.Fatal("^H ended the body picker")
		}
		if st.FilterText != "ma" {
			t.Errorf("filter = %q after ^H, want %q", st.FilterText, "ma")
		}
		for range 3 {
			st, out = updateBodyModalKey(st, keyCtrlH, testListHeight)
			if out.Confirmed || out.Cancelled {
				t.Fatal("^H on an empty search ended the body picker")
			}
		}
		if st.FilterText != "" {
			t.Errorf("filter = %q, want empty", st.FilterText)
		}
	})
}

// Acceptance 2: every text field built on form.go deletes on ^H as on
// Backspace, and ^H on an empty field leaves it empty.
func TestCtrlHDeletesInFormFields(t *testing.T) {
	f := dialog.NewForm(dialog.Field{Label: "Title"})
	f, _ = f.UpdateKey(typed("Jane"))
	f, handled := f.UpdateKey(keyCtrlH)
	if !handled {
		t.Fatal("the form did not handle ^H")
	}
	if got := f.Fields[0].Value; got != "Jan" {
		t.Errorf("value = %q after ^H, want %q", got, "Jan")
	}
	for range 4 {
		f, _ = f.UpdateKey(keyCtrlH)
	}
	if got := f.Fields[0].Value; got != "" {
		t.Errorf("value = %q, want empty", got)
	}
}

// Acceptance 4: on the main screen ^H runs no command and types nothing
// anywhere: houses has its comma form only, and a terminal's Backspace sent as
// ^H must not open a chooser.
func TestCtrlHOutsideAPickerIsNotConsumed(t *testing.T) {
	m := detailsTestModel(t)
	tm, cmd := m.Update(keyCtrlH)
	if cmd != nil {
		t.Error("^H on the main screen returned a command")
	}
	if got := tm.(Model); got.startup != m.startup || !modelsLookEqual(m, got) {
		t.Errorf("^H on the main screen changed the model (startup %q -> %q)", m.startup, got.startup)
	}
}
