package popup

import (
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// house_display.go is the houses chooser's second part, Shown as: under the
// systems, a dotted rule, the heading and one row per form the Bodies house
// column can be drawn in, the form in force marked ●. The cursor reaches it from
// the last system with Down and leaves it with Up from the first form; Enter on
// a form puts it in force and closes.
//
// The choice is the session's, for every slot. The part writes nothing: ui puts
// the form in force when Enter falls on it. The forms and their words are
// passed in; ui owns them (the preferences file reads the same table).

// ShownAsHeading is the part's heading.
const ShownAsHeading = "Houses shown as"

// HouseFormRow is one form as the part lists it.
type HouseFormRow struct {
	Form  panel.HouseForm
	Title string
}

// ShownAs is the part's state. The list's selection is the form in force when
// the chooser opened; its cursor is the row Enter would apply.
type ShownAs struct {
	// On says the chooser has this part at all.
	On    bool
	Forms []HouseFormRow
	List  menu.RadioList
	// Focus says the cursor is in this part and not in the systems.
	Focus bool
}

// NewShownAs is the part with the mark on the form in force.
func NewShownAs(forms []HouseFormRow, chosen panel.HouseForm) ShownAs {
	idx := 0
	for i, info := range forms {
		if info.Form == chosen {
			idx = i
			break
		}
	}
	return ShownAs{On: true, Forms: forms, List: menu.NewRadioList(len(forms), idx)}
}

// Rows is the lines the part takes under the systems: the rule, the heading
// and a row per form. None when the chooser has no part.
func (s ShownAs) Rows() int {
	if !s.On {
		return 0
	}
	return 2 + len(s.Forms)
}

// Count is how many rows the cursor can rest on in the part.
func (s ShownAs) Count() int {
	if !s.On {
		return 0
	}
	return len(s.Forms)
}

// Form is the form under the cursor.
func (s ShownAs) Form() panel.HouseForm {
	if s.List.Cursor < 0 || s.List.Cursor >= len(s.Forms) {
		return panel.HouseAngularity
	}
	return s.Forms[s.List.Cursor].Form
}

// Enter puts the cursor on the first form, where it arrives from the systems.
func (s *ShownAs) Enter() { s.List.Cursor = 0 }

// Apply runs one moving action in this part and says whether the
// cursor left it upward, back into the systems. Typed keys do nothing here.
func (s *ShownAs) Apply(a Action) (leftUp bool) {
	switch a {
	case Up:
		if s.List.Cursor == 0 {
			return true
		}
		s.List.MoveUp()
	case Down:
		s.List.MoveDown()
	case PageUp, PageHome:
		if s.List.Cursor == 0 {
			return true
		}
		s.List.Cursor = 0
	case PageDown, PageEnd:
		s.List.Cursor = s.List.N - 1
	}
	return false
}

// Width is the widest line the part draws, over every form the choice can be
// on, so the box does not change width while a form is chosen.
func (s ShownAs) Width(cfg term.Style) int {
	w := menu.RowWidth(menu.RowSpec{Left: ShownAsHeading}, cfg)
	for _, info := range s.Forms {
		w = max(w, menu.RowWidth(formRow(info, true, false), cfg))
	}
	return w
}

// Lines draws the part: the dotted rule, the heading, the forms. The cursor
// shows only while it is in this part.
func (s ShownAs) Lines(cfg term.Style, width int) []string {
	lines := []string{menu.Rule(cfg, width), menu.Row(menu.RowSpec{Left: ShownAsHeading}, cfg, width)}
	for i, info := range s.Forms {
		lines = append(lines, menu.Row(formRow(info, s.List.IsSelected(i), s.Focus && i == s.List.Cursor), cfg, width))
	}
	return lines
}

// formRow is one form's row: its title, and the ● the systems use on the form
// in force.
func formRow(info HouseFormRow, inForce, cursor bool) menu.RowSpec {
	mark := "  "
	if inForce {
		mark = "● "
	}
	return menu.RowSpec{Left: info.Title, Right: []string{mark}, Cursor: cursor}
}
