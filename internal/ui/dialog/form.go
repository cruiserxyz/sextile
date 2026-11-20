package dialog

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// form.go is the chassis every prompt that asks the operator to TYPE something
// draws through: labelled fields, one focused field carrying the cursor, an
// info block beneath the rule, a refusal, and a footer, inside Box's
// chrome (the specification, item 2).
//
// It owns the typing and the drawing and nothing else. What Enter means, what
// Esc restores, and when the form is busy belong to the caller's own state
// machine: the save prompt keeps all of that in save_modal.go and hands this
// file only its values. A form never acts on its own.

// Field is one labelled input line.
type Field struct {
	Label string // drawn before the value, e.g. "Save as: "
	Value string // exactly what was typed; a placeholder never enters it

	// Placeholder is shown, dim, while value is empty, and disappears at the
	// first typed character. It is a hint about what to type -- today's name,
	// say -- and Enter submits value, so an untouched field submits "".
	Placeholder string
}

// Form is a set of fields and which one has the cursor.
type Form struct {
	Fields []Field
	Focus  int
}

// NewForm builds a form with the cursor on its first field.
func NewForm(fields ...Field) Form {
	return Form{Fields: fields}
}

// Value returns field i's typed Value, or "" for a field that does not exist.
func (f Form) Value(i int) string {
	if i < 0 || i >= len(f.Fields) {
		return ""
	}
	return f.Fields[i].Value
}

// Values returns every field's typed value, in field order. It is what a
// caller submits.
func (f Form) Values() []string {
	out := make([]string, len(f.Fields))
	for i, fld := range f.Fields {
		out[i] = fld.Value
	}
	return out
}

// NextField and PrevField move the cursor between fields, wrapping. Tab and
// ⇧tab reach them through the caller's table; they are not typing.
func (f Form) NextField() Form {
	if len(f.Fields) > 0 {
		f.Focus = (f.Focus + 1) % len(f.Fields)
	}
	return f
}

func (f Form) PrevField() Form {
	if len(f.Fields) > 0 {
		f.Focus = (f.Focus - 1 + len(f.Fields)) % len(f.Fields)
	}
	return f
}

// UpdateKey is the form's text entry: one typed character, Space or
// Backspace, reporting whether the key was one it handles. Every other key —
// Enter, Esc, Tab — is the caller's table's: the caller decides what they
// mean, and a form that swallowed them could not be submitted or left.
func (f Form) UpdateKey(msg tea.KeyMsg) (Form, bool) {
	if len(f.Fields) == 0 {
		return f, false
	}
	// A form is a value; its fields must not be shared with the copy it was
	// made from, or typing into one would type into both.
	f.Fields = append([]Field(nil), f.Fields...)
	switch msg.Type {
	case tea.KeyBackspace, tea.KeyCtrlH: // ^H is Backspace on some terminals (the specification)
		fld := &f.Fields[f.Focus]
		if r := []rune(fld.Value); len(r) > 0 {
			fld.Value = string(r[:len(r)-1])
		}
		return f, true
	case tea.KeySpace:
		f.Fields[f.Focus].Value += " "
		return f, true
	case tea.KeyRunes:
		// Control characters become spaces rather than entering the value. A
		// bracketed paste arrives as ONE KeyRunes carrying every byte between
		// the paste markers, newlines included, so "Jane Doe\n b. 1987" would
		// otherwise split one input line across two terminal rows and break
		// the fixed-height box -- and, if confirmed, hand a caller a label its
		// writer has to refuse. astrolog.ChartRecord refuses such a label
		// outright; this keeps a form from ever building one.
		f.Fields[f.Focus].Value += sanitiseTyped(msg.Runes)
		return f, true
	}
	return f, false
}

// sanitiseTyped maps control characters to spaces and passes everything else
// through. Spaces rather than deletion: a pasted two-line value is one label
// with a line break in it, and joining is what the operator meant; dropping
// the break would run two words together.
func sanitiseTyped(runes []rune) string {
	var b strings.Builder
	for _, r := range runes {
		if unicode.IsControl(r) {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// FormView is what a caller draws around its form's fields.
type FormView struct {
	// Title names the form's popup in its frame (Box.Title).
	Title string

	// Fixed are lines after the fields that the form shows and does not let
	// the operator change: a label, a value and a dim note.
	Fixed []FixedLine

	// Info is the block beneath the rule, one entry per line, each truncated
	// to the box: the save prompt's destination, for instance.
	Info []string

	// WrapInfo breaks an Info line too wide for the box at its last space
	// that fits, instead of cutting it, so a sentence ending in a file name
	// keeps the name whole. Off for the save prompt, whose destination line
	// is elided by design (save_modal.go destLine).
	WrapInfo bool

	// Refusal, when non-empty, is drawn in the box's message zone, which the
	// form keeps from the moment it opens. The form stays open with it; a
	// refusal is the caller's next instruction to the operator, not a dialog
	// of its own. One longer than its row wraps and is never cut: the box takes
	// the rows it needs.
	Refusal string
}

// FixedLine is a line a form shows and does not let the operator change.
type FixedLine struct {
	Label, Value, Note string
}

// RefusalRoom is the cells a refusal has: a sentence of the kind the forms say
// ("That name has no letters or digits to save under.") fits, and the form is
// that wide from the moment it opens (chosen 2026-10-27).
const RefusalRoom = ProseWidth

// RenderForm draws f inside Box's chrome. The box is as wide as its widest
// line from the moment it opens (operator, 2026-10-27) -- the fields (each asks
// for its label and FieldRoom cells to type in), the info wrapped at ProseWidth
// when the caller says to wrap it, and the room for a refusal -- and as tall as
// its fields and its info. It does not change size with the values typed, the
// placeholders or which field is focused. A refusal takes a blank row and its
// own rows below the info, so the box grows by them and is back to its height
// when the refusal goes (operator, 2026-11-14: no popup keeps an empty row).
//
// An info line that is NOT wrapped echoes what was typed (the save prompt's
// file name): it is cut to the box and never sizes it, or the box would widen
// with every letter.
func RenderForm(f Form, v FormView, cfg term.Style, termW, termH int, chrome Chrome) string {
	inputs := make([]input, len(f.Fields))
	for i := range f.Fields {
		fld, focused := f.Fields[i], i == f.Focus
		inputs[i] = input{
			Width:  ansi.StringWidth(fld.Label) + FieldRoom + 1,
			Render: func(width int) string { return fieldLine(fld, focused, cfg, chrome, width) },
		}
	}

	for _, fx := range v.Fixed {
		fx := fx
		inputs = append(inputs, input{
			Width: ansi.StringWidth(fx.Label + fx.Value + "  " + fx.Note),
			Render: func(width int) string {
				line := term.CutRight(fx.Label+fx.Value, width, cfg)
				if note := width - ansi.StringWidth(line) - 2; fx.Note != "" && note > 0 {
					line += "  " + chrome.Dim(term.CutRight(fx.Note, note, cfg))
				}
				return line
			},
		})
	}

	// The lines are laid out before the box is, at a width that does not
	// depend on it: the box takes its width from them.
	wrapAt := min(ProseWidth, modalContentCap(termW))
	fit := Fit{Width: RefusalRoom}
	layout := func(info []string) []string {
		var lines []string
		for _, l := range info {
			if !v.WrapInfo {
				lines = append(lines, l)
				continue
			}
			for _, w := range wrapAtSpace(l, wrapAt) {
				lines = append(lines, w)
				fit.Width = max(fit.Width, ansi.StringWidth(w))
			}
		}
		return lines
	}
	lines := layout(v.Info)
	fit.Rows = len(lines)

	box := Box{
		Title:   v.Title,
		Inputs:  inputs,
		Fit:     fit,
		Zone:    true,
		Message: v.Refusal,
	}
	return Render(box, cfg, termW, termH, chrome, func(width, height int) string {
		out := make([]string, 0, height)
		for _, l := range lines {
			out = append(out, term.CutRight(l, width, cfg))
		}
		for len(out) < height {
			out = append(out, "")
		}
		return strings.Join(out[:height], "\n")
	})
}

// wrapAtSpace splits s into lines no wider than width, breaking at spaces. A
// single word wider than width -- a backup folder's absolute path -- is broken
// after its last slash that fits, or at the width when it has none, so the
// whole path stays on screen instead of being cut.
func wrapAtSpace(s string, width int) []string {
	var out []string
	for width > 0 && ansi.StringWidth(s) > width {
		cut, slash, hard := -1, -1, 0
		for i, r := range s {
			if ansi.StringWidth(s[:i]) > width {
				break
			}
			hard = i
			if r == ' ' {
				cut = i
			}
			if r == '/' && i > 0 && ansi.StringWidth(s[:i+1]) <= width {
				slash = i + 1
			}
		}
		switch {
		case cut > 0:
			out = append(out, s[:cut])
			s = s[cut+1:]
		case slash > 0:
			out = append(out, s[:slash])
			s = s[slash:]
		case hard > 0:
			out = append(out, s[:hard])
			s = s[hard:]
		default:
			return append(out, s)
		}
	}
	return append(out, s)
}

// fieldLine is one field's input line: label, value, and the block cursor when
// the field is focused. While the value is empty and colour is in force, the
// placeholder takes its place, faint, with the cursor on its first character.
// Without colour no placeholder is drawn at all.
//
// A value too long for the box shows its TAIL behind an ellipsis. The tail is
// where the cursor and the operator's attention are; wrapping instead would
// add a line and resize the box under them.
func fieldLine(fld Field, focused bool, cfg term.Style, chrome Chrome, width int) string {
	cursor := ""
	if focused {
		cursor = "█"
	}
	room := width - ansi.StringWidth(fld.Label) - ansi.StringWidth(cursor)
	if fld.Value == "" && fld.Placeholder != "" && cfg.UseColor() {
		// Cut BEFORE styling: a cut through a styled string can drop its
		// reset, and the faint would bleed into the padding after it.
		hint := []rune(term.CutRight(fld.Placeholder, max(width-ansi.StringWidth(fld.Label), 0), cfg))
		if !focused || len(hint) == 0 {
			return fld.Label + chrome.Dim(string(hint))
		}
		// The cursor sits ON the hint's first character, in reverse video,
		// rather than pushing the hint one cell right (operator, 2026-10-02):
		// the field reads as the hint with the cursor where typing begins.
		return fld.Label + term.SGRReverse + string(hint[0]) + term.SGRReset + chrome.Dim(string(hint[1:]))
	}
	// Without colour no attribute could tell a hint from typed text, so none
	// is drawn (operator, 2026-10-02); the caller's info line says what the
	// field is for.
	// A long value keeps what was typed last, so the cursor end stays in view.
	return fld.Label + term.CutLeft(fld.Value, room, cfg) + cursor
}
