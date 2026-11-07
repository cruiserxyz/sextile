package popup

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ui/dialog"
)

// While a write is in flight nothing is typed and no field moves: the text on
// screen must stay the text being written.
func TestFormTakesNothingWhileSaving(t *testing.T) {
	f := Form{Fields: dialog.NewForm(dialog.Field{Label: "A: "}, dialog.Field{Label: "B: "})}
	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}
	f.Apply(Type, key)
	if got := f.Fields.Value(0); got != "x" {
		t.Fatalf("typing at rest gave %q, want %q", got, "x")
	}
	f.Apply(NextField, key)
	if f.Fields.Focus != 1 {
		t.Fatalf("NextField at rest left the cursor on %d, want 1", f.Fields.Focus)
	}
	f.Saving = true
	f.Apply(Type, key)
	f.Apply(PrevField, key)
	if got := f.Fields.Value(1); got != "" || f.Fields.Focus != 1 {
		t.Errorf("while saving: field 1 %q focus %d, want it untouched", got, f.Fields.Focus)
	}
}
