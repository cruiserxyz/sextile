package dialog

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// choice.go is the small popup that asks the operator one question and takes
// its answer from a row of buttons: drawn centred over the popup the question
// came from, which stays visible behind it.

// TitleChartsFolder is the choice popup's name for the questions about the
// charts folder.
const TitleChartsFolder = "Charts folder"

// Button is one answer: its label and the key that takes it. Every button
// shows its key, as `Yes (y)`, so a choice popup reads the same everywhere.
type Button struct {
	Label, Key string
}

// TitleNewSession is the choice popup's name for the question asked before a
// new session starts.
const TitleNewSession = "New session"

// TitleOpenSession and TitleQuit name the same question when it stands in
// front of opening another session and of quitting.
const (
	TitleOpenSession = "Open session"
	TitleQuit        = "Quit"
)

// TitleDeleteSession is the choice popup's name for the question asked before
// a session goes to the trash.
const TitleDeleteSession = "Delete session"

// Choice is one question with its buttons. Lit is the button Enter takes.
// Title names the popup; empty is the charts-folder questions' name.
type Choice struct {
	Title   string
	Body    []string
	Buttons []Button
	Lit     int
}

// Moved is c with the lit button moved by delta, stopping at the ends.
func (c Choice) Moved(delta int) Choice {
	c.Lit = min(max(c.Lit+delta, 0), len(c.Buttons)-1)
	return c
}

// buttonGap is the cells between two buttons.
const buttonGap = 2

// buttonText is one button with its key: the lit one in brackets, an unlit
// one in blanks of the same width.
func buttonText(b Button, lit bool) string {
	label := b.Label
	if b.Key != "" {
		label += " (" + b.Key + ")"
	}
	if lit {
		return "[ " + label + " ]"
	}
	return "  " + label + "  "
}

// buttonRow is the buttons in one row, the lit one in reverse video where
// colour is in force and in brackets always.
func (c Choice) buttonRow(cfg term.Style) (row string, cells int) {
	var parts []string
	for i, b := range c.Buttons {
		t := buttonText(b, i == c.Lit)
		cells += ansi.StringWidth(t)
		if i == c.Lit && cfg.UseColor() {
			t = term.SGRReverse + t + term.SGRReset
		}
		parts = append(parts, t)
	}
	cells += buttonGap * (len(c.Buttons) - 1)
	return strings.Join(parts, strings.Repeat(" ", buttonGap)), cells
}

// RenderChoice draws the question and its buttons in the popup's chrome. The
// body wraps at ProseWidth and is never cut; a path wider than that breaks
// after a slash.
func RenderChoice(c Choice, cfg term.Style, termW, termH int, chrome Chrome) string {
	wrapAt := min(ProseWidth, modalContentCap(termW))
	var lines []string
	fit := Fit{}
	for _, l := range c.Body {
		for _, w := range wrapAtSpace(l, wrapAt) {
			lines = append(lines, w)
			fit.Width = max(fit.Width, ansi.StringWidth(w))
		}
	}
	row, cells := c.buttonRow(cfg)
	fit.Width = max(fit.Width, cells)
	fit.Rows = len(lines) + 2
	title := c.Title
	if title == "" {
		title = TitleChartsFolder
	}
	return Render(Box{
		Title:   title,
		NoInput: true,
		Fit:     fit,
		Taller:  fit.Rows,
	}, cfg, termW, termH, chrome, func(width, height int) string {
		// On a terminal too short for the whole question the buttons stay and
		// the body gives way.
		body := lines[:min(len(lines), max(height-2, 0))]
		out := append(slices.Clone(body), "", strings.Repeat(" ", max((width-cells)/2, 0))+row)
		return strings.Join(out, "\n")
	})
}

// nothingWritten is the sentence every refusal of the door ends with.
const nothingWritten = "Nothing was written."

// MessageLines is a message as the box lays it out at this width: one
// paragraph for each line break, each wrapped at a space or after a slash. A
// message that ends in "Nothing was written." gives that sentence a line of
// its own, so no width can cut it from the sentence before.
func MessageLines(msg string, width int) []string {
	if before, ok := strings.CutSuffix(msg, " "+nothingWritten); ok {
		msg = before + "\n" + nothingWritten
	}
	var lines []string
	for _, para := range strings.Split(msg, "\n") {
		lines = append(lines, wrapAtSpace(strings.Join(strings.Fields(para), " "), width)...)
	}
	return lines
}
