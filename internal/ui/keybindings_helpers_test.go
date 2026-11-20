package ui

import (
	"regexp"
	"testing"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// Helpers the Keybindings tests share.

// kbLine is one row of the popup, as its description then its key.
type kbLine struct {
	Text    string
	Binding bool
}

// openedKeybindings is m with the Keybindings popup open at the keys of the
// place m is in.
func openedKeybindings(m Model) Model {
	tm, _ := openKeybindings(m, false)
	return tm.(Model)
}

// keybindingLines are the rows the popup lists for m, as a person reads one:
// `Description  key`.
func keybindingLines(m Model) []kbLine { return keybindingLinesOf(m, m.currentOwner()) }

// keybindingLinesOf is keybindingLines for the popup opened over owner o.
func keybindingLinesOf(m Model, o owner) []kbLine {
	var out []kbLine
	for _, r := range m.keybindingRows(o) {
		out = append(out, kbLine{Text: r.Desc + "  " + r.Key, Binding: true})
	}
	return out
}

// kbTitles are the descriptions of the rows the popup shows now, in order.
func kbTitles(m Model) []string {
	out := make([]string, 0, len(m.keybindings.Matches))
	for _, r := range m.keybindings.Matches {
		out = append(out, r.Desc)
	}
	return out
}

// kbHeaders are the group headers the popup draws now.
func kbHeaders(m Model) []string {
	var out []string
	for _, l := range m.keybindings.Lines() {
		if l.Kind == keys.LineHeader {
			out = append(out, l.Text)
		}
	}
	return out
}

// kbType types text into the popup, one key at a time through Update.
func kbType(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m, _ = tpress(t, m, runes(string(r)))
	}
	return m
}

// helpRow splits a line of keybindingLines into its description and its key.
var helpRow = regexp.MustCompile(`^(.*?) {2,}(\S.*)$`)
