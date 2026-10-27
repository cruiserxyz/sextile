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

	// Alt are the other blocks of text this popup shows in place of Info in
	// another step (the title form's second step): the box is sized for the
	// fullest of them and Info, once, so it does not change between steps.
	Alt [][]string

	// Refusal, when non-empty, is drawn in the box's message zone, which the
	// form keeps from the moment it opens. The form stays open with it; a
	// refusal is the caller's next instruction to the operator, not a dialog
	// of its own, and it never makes the box bigger: one longer than its row
	// wraps and takes rows from the foot of the info above it.
	Refusal string
}

// modalRefusalRoom is the cells the message zone has: a sentence of
// the kind the forms say ("That name has no letters or digits to save under.")
// fits, and the form is that wide from the moment it opens (chosen 2026-10-27).
const modalRefusalRoom = modalProseWidth

// renderForm draws f inside modalBox's chrome. The box is decided when the form
// opens and holds for the whole opening (operator, 2026-10-27): as wide as its
// widest line -- the fields (each asks for its label and modalFieldRoom cells to
// type in), the info wrapped at modalProseWidth when the caller says to wrap it,
// and the message zone -- and as tall as its info (the fullest block, if the
// form has steps) and the zone, a blank row and the row where a refusal appears.
// It does not change size with the values typed, the placeholders, which field is
// focused or a refusal appearing.
//
// An info line that is NOT wrapped echoes what was typed (the save prompt's
// file name): it is cut to the box and never sizes it, or the box would widen
// with every letter.
func renderForm(f form, v formView, cfg Config, termW, termH int) string {
	inputs := make([]modalInput, len(f.fields))
	for i := range f.fields {
		fld, focused := f.fields[i], i == f.focus
		inputs[i] = modalInput{
			Width:  ansi.StringWidth(fld.label) + modalFieldRoom + 1,
			Render: func(width int) string { return fieldLine(fld, focused, cfg, width) },
		}
	}

	// The lines are laid out before the box is, at a width that does not
	// depend on it: the box takes its width from them.
	wrapAt := min(modalProseWidth, modalContentCap(termW))
	fit := modalFit{Width: modalRefusalRoom}
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
	for _, alt := range v.Alt {
		fit.Rows = max(fit.Rows, len(layout(alt)))
	}

	return renderModalBox(modalBox{
		Title:   v.Title,
		Inputs:  inputs,
		Fit:     fit,
		Zone:    true,
		Message: v.Refusal,
	}, cfg, termW, termH, func(width, height int) string {
		out := make([]string, 0, height)
		for _, l := range lines {
			out = append(out, cutRight(l, width, cfg))
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
