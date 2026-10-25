package ui

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// form.go is the chassis every prompt that asks the operator to TYPE something
// draws through: labelled fields, one focused field carrying the cursor, an
// info block beneath the rule, a refusal, and a footer, inside modalBox's
// chrome (the specification, item 2).
//
// It owns the typing and the drawing and nothing else. What Enter means, what
// Esc restores, and when the form is busy belong to the caller's own state
// machine: the save prompt keeps all of that in save_modal.go and hands this
// file only its values. A form never acts on its own.

// formField is one labelled input line.
type formField struct {
	label string // drawn before the value, e.g. "Save as: "
	value string // exactly what was typed; a placeholder never enters it

	// placeholder is shown, dim, while value is empty, and disappears at the
	// first typed character. It is a hint about what to type -- today's name,
	// say -- and Enter submits value, so an untouched field submits "".
	placeholder string
}

// form is a set of fields and which one has the cursor.
type form struct {
	fields []formField
	focus  int
}

// newForm builds a form with the cursor on its first field.
func newForm(fields ...formField) form {
	return form{fields: fields}
}

// value returns field i's typed value, or "" for a field that does not exist.
func (f form) value(i int) string {
	if i < 0 || i >= len(f.fields) {
		return ""
	}
	return f.fields[i].value
}

// values returns every field's typed value, in field order. It is what a
// caller submits.
func (f form) values() []string {
	out := make([]string, len(f.fields))
	for i, fld := range f.fields {
		out[i] = fld.value
	}
	return out
}

// nextField and prevField move the cursor between fields, wrapping. Tab and
// ⇧tab reach them through the caller's table; they are not typing.
func (f form) nextField() form {
	if len(f.fields) > 0 {
		f.focus = (f.focus + 1) % len(f.fields)
	}
	return f
}

func (f form) prevField() form {
	if len(f.fields) > 0 {
		f.focus = (f.focus - 1 + len(f.fields)) % len(f.fields)
	}
	return f
}

// updateKey is the form's text entry: one typed character, Space or
// Backspace, reporting whether the key was one it handles. Every other key —
// Enter, Esc, Tab — is the caller's table's: the caller decides what they
// mean, and a form that swallowed them could not be submitted or left.
func (f form) updateKey(msg tea.KeyMsg) (form, bool) {
	if len(f.fields) == 0 {
		return f, false
	}
	// A form is a value; its fields must not be shared with the copy it was
	// made from, or typing into one would type into both.
	f.fields = append([]formField(nil), f.fields...)
	switch msg.Type {
	case tea.KeyBackspace, tea.KeyCtrlH: // ^H is Backspace on some terminals (the specification)
		fld := &f.fields[f.focus]
		if r := []rune(fld.value); len(r) > 0 {
			fld.value = string(r[:len(r)-1])
		}
		return f, true
	case tea.KeySpace:
		f.fields[f.focus].value += " "
		return f, true
	case tea.KeyRunes:
		// Control characters become spaces rather than entering the value. A
		// bracketed paste arrives as ONE KeyRunes carrying every byte between
		// the paste markers, newlines included, so "Jane Doe\n b. 1987" would
		// otherwise split one input line across two terminal rows and break
		// the fixed-height box -- and, if confirmed, hand a caller a label its
		// writer has to refuse. astrolog.ChartRecord refuses such a label
		// outright; this keeps a form from ever building one.
		f.fields[f.focus].value += sanitiseTyped(msg.Runes)
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

// formView is what a caller draws around its form's fields.
type formView struct {
	// Title names the form's popup in its frame (modalBox.Title).
	Title string

	// Info is the block beneath the rule, one entry per line, each truncated
	// to the box: the save prompt's destination, for instance.
	Info []string

	// WrapInfo breaks an Info line too wide for the box at its last space
	// that fits, instead of cutting it, so a sentence ending in a file name
	// keeps the name whole. Off for the save prompt, whose destination line
	// is elided by design (save_modal.go destLine).
	WrapInfo bool

	// Refusal, when non-empty, is drawn after a blank line beneath Info. The
	// form stays open with it; a refusal is the caller's next instruction to
	// the operator, not a dialog of its own.
	Refusal string
}

// renderForm draws f inside modalBox's chrome. The box's outer size depends
// only on the terminal, the resting footer and the number of fields -- never on
// values, placeholders, the refusal or which field is focused.
func renderForm(f form, v formView, cfg Config, termW, termH int) string {
	inputs := make([]func(width int) string, len(f.fields))
	for i := range f.fields {
		fld, focused := f.fields[i], i == f.focus
		inputs[i] = func(width int) string { return fieldLine(fld, focused, cfg, width) }
	}
	return renderModalBox(modalBox{
		Title:  v.Title,
		Inputs: inputs,
	}, cfg, termW, termH, func(width, height int) string {
		lines := make([]string, 0, height)
		for _, l := range v.Info {
			if v.WrapInfo {
				for _, w := range wrapAtSpace(l, width) {
					lines = append(lines, cutRight(w, width, cfg))
				}
				continue
			}
			lines = append(lines, cutRight(l, width, cfg))
		}
		if v.Refusal != "" {
			lines = append(lines, "")
			for _, w := range wrapAtSpace(v.Refusal, width) {
				lines = append(lines, cutRight(w, width, cfg))
			}
		}
		for len(lines) < height {
			lines = append(lines, "")
		}
		return strings.Join(lines[:height], "\n")
	})
}

// wrapAtSpace splits s into lines no wider than width, breaking at spaces.
// A single word wider than width stays one line, for the caller to cut.
func wrapAtSpace(s string, width int) []string {
	var out []string
	for ansi.StringWidth(s) > width {
		cut := -1
		for i, r := range s {
			if r == ' ' && ansi.StringWidth(s[:i]) <= width {
				cut = i
			}
		}
		if cut <= 0 {
			break
		}
		out = append(out, s[:cut])
		s = s[cut+1:]
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
func fieldLine(fld formField, focused bool, cfg Config, width int) string {
	cursor := ""
	if focused {
		cursor = "█"
	}
	room := width - ansi.StringWidth(fld.label) - ansi.StringWidth(cursor)
	if fld.value == "" && fld.placeholder != "" && cfg.useColor() {
		// Cut BEFORE styling: a cut through a styled string can drop its
		// reset, and the faint would bleed into the padding after it.
		hint := []rune(cutRight(fld.placeholder, max(width-ansi.StringWidth(fld.label), 0), cfg))
		if !focused || len(hint) == 0 {
			return fld.label + renderDim(string(hint), true)
		}
		// The cursor sits ON the hint's first character, in reverse video,
		// rather than pushing the hint one cell right (operator, 2026-10-02):
		// the field reads as the hint with the cursor where typing begins.
		return fld.label + sgrReverse + string(hint[0]) + sgrReset + renderDim(string(hint[1:]), true)
	}
	// Without colour no attribute could tell a hint from typed text, so none
	// is drawn (operator, 2026-10-02); the caller's info line says what the
	// field is for.
	// A long value keeps what was typed last, so the cursor end stays in view.
	return fld.label + cutLeft(fld.value, room, cfg) + cursor
}
