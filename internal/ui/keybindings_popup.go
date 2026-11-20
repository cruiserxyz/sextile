package ui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// keybindings_popup.go draws the Keybindings popup in the shared popup box: the
// groups each under its header with an empty line between two groups, a row per
// key (a cell of margin, the key right-aligned in its column, in the blue-green
// colour, then its description), "(Type to filter)" on the top edge, the count on the bottom
// border, and, while a filter is typed, a rule and the filter line at the foot.
// The table starts one cell inside the frame. The Model's half is
// keybindings_popup_model.go.

// kbOffMark ends a row that cannot run, for a terminal drawn without colour,
// where a line through the words cannot be seen.
const kbOffMark = "  (n/a)"

// kbTypeToFilter is the words at the right end of the top edge.
const kbTypeToFilter = "(Type to filter)"

// kbCursorKey is a key on the cursor row's band: the bright form of the key
// colour.
const kbCursorKey = lipgloss.Color("14")

// kbRowLeft is a row's text: its key right-aligned in the key column, a
// space, its description (ending in `…` for a command that opens a chooser, a
// list or a prompt). The key takes the blue-green colour when colours are on,
// the bright form on the cursor row. A row that cannot run has its description
// struck through, and without colour it ends in (n/a).
func kbRowLeft(r keys.Row, struck, cursor bool, keyW int, cfg Config) string {
	key := strings.Repeat(" ", max(keyW-ansi.StringWidth(r.Key), 0)) + r.Key
	desc := r.Desc
	if r.Opens {
		desc += "…"
	}
	if cfg.UseColor() {
		pad := key[:len(key)-len(r.Key)]
		colour := term.KeyColumnColour
		if cursor {
			colour = kbCursorKey
		}
		key = pad + lipgloss.NewStyle().Foreground(colour).Render(r.Key)
		if struck {
			desc = term.SGRStrike + desc + term.SGRNoStrike
		}
	}
	text := " " + key + " " + desc
	if struck && !cfg.UseColor() {
		text += kbOffMark
	}
	return text
}

// keybindingsWidth is the content width the popup needs: its widest line over
// every row, the rule, the notes and the filter line. It is read once, when
// the popup opens, and kept in the state, so the box is the size it opened at
// while the operator types.
func keybindingsWidth(st keys.KbState, off func(keys.Row) bool, cfg Config) int {
	width := max(ansi.StringWidth(keys.FilterPrompt)+dialog.SearchRoom+2, 1)
	for _, n := range st.Notes {
		width = max(width, ansi.StringWidth(n)+2)
	}
	for _, r := range st.All {
		width = max(width, menu.RowWidth(menu.RowSpec{Left: kbRowLeft(r, off(r), false, st.KeyW, cfg)}, cfg)+1)
	}
	return width
}

// kbCounter is the border text: the cursor's row of the rows that pass.
func kbCounter(st keys.KbState, cfg Config) *dialog.Counter {
	return &dialog.Counter{
		Widest: func() string { return listPosition{Cursor: len(st.All), Total: len(st.All)}.text() },
		Draw: func(box string) string {
			return withPositionCounter(box, listPosition{Cursor: st.Cursor, Total: len(st.Matches)}, cfg, true)
		},
	}
}

// renderKeybindings draws the popup. off is Model.kbOff, read at draw time
// so a row is never stale.
func renderKeybindings(st keys.KbState, off func(keys.Row) bool, cfg Config, termW, termH int) string {
	lines := st.Lines()
	box := dialog.Render(dialog.Box{
		Title:    st.Title,
		NoInput:  true,
		Tight:    true,
		TopRight: kbTypeToFilter,
		Fit:      dialog.Fit{Width: max(st.Width, 1), Rows: max(st.Rows, 1)},
		Position: kbCounter(st, cfg),
		Scroll: func(height int) (pane.Viewport, int) {
			h := kbWindow(st, height)
			return pane.Viewport{Total: len(lines), Visible: h, Offset: min(st.Top, max(len(lines)-h, 0))}, 0
		},
	}, cfg, termW, termH, popupChrome(cfg), func(width, height int) string {
		return kbBody(st, lines, off, cfg, width, height)
	})
	return withFilterRule(box, st, cfg)
}

// withFilterRule joins the rule over the filter line to both side borders, as
// lazygit draws it, while a filter is typed.
func withFilterRule(box string, st keys.KbState, cfg Config) string {
	if st.FilterText == "" {
		return box
	}
	rows := strings.Split(box, "\n")
	for i := 2; i < len(rows); i++ {
		if strings.Contains(ansi.Strip(rows[i]), keys.FilterPrompt) {
			// lazygit's rule here is solid, where a panel's is dotted.
			joined := strings.Split(pane.WithPanelRule(box, i-2, cfg, true), "\n")
			joined[i-1] = strings.ReplaceAll(joined[i-1], pane.RuleGlyph, "─")
			return strings.Join(joined, "\n")
		}
	}
	return box
}

// kbWindow is the lines of the list that show in a box whose list area is
// height lines: the filter and its rule take two while a filter is typed.
func kbWindow(st keys.KbState, height int) int {
	if st.FilterText != "" {
		return max(height-2, 1)
	}
	return height
}

// kbHeader is a group's header line: its words in the column where the
// descriptions start, bold, in the green of the frame.
func kbHeader(text string, keyW int, cfg Config, width int) string {
	lead := strings.Repeat(" ", keyW+2)
	if !cfg.UseColor() {
		return menu.Heading(lead+text, width, cfg)
	}
	words := lipgloss.NewStyle().Foreground(cfg.FocusColour()).Bold(true).Render(text)
	return term.FitLine(lead+words, width, cfg)
}

// kbFoot is the rule and the filter line at the popup's foot while a filter is
// typed.
func kbFoot(st keys.KbState, cfg Config, width int) []string {
	rule := "─"
	if cfg.UseASCII() {
		rule = "-"
	}
	room := max(width-ansi.StringWidth(keys.FilterPrompt)-2, 0)
	filter := " " + keys.FilterPrompt + term.CutLeft(st.FilterText, room, cfg) + "█"
	return []string{strings.Repeat(rule, width), term.FitLine(filter, width, cfg)}
}

// kbBody is the list area: the window of lines, then the rule and the filter
// line at the foot while a filter is typed.
func kbBody(st keys.KbState, lines []keys.Line, off func(keys.Row) bool, cfg Config, width, height int) string {
	window := kbWindow(st, height)
	top := min(st.Top, max(len(lines)-window, 0))
	// The window never starts or ends on an empty line: a group's gap that
	// falls on its edge is skipped, and the line after it takes its place.
	var shown []keys.Line
	end := min(top+window, len(lines))
	switch {
	case top > 0 && top < len(lines) && lines[top].Kind == keys.LineBlank && end == len(lines):
		// The list ends inside the window, so skipping the gap would leave an empty
		// row at the foot: the gap is left out and the line before it fills the first
		// row.
		shown = append(slices.Clone(lines[top-1:top]), lines[top+1:end]...)
	case top < len(lines) && lines[top].Kind == keys.LineBlank:
		top++
		end = min(top+window, len(lines))
		shown = slices.Clone(lines[top:end])
	default:
		shown = slices.Clone(lines[top:end])
	}
	if end < len(lines) && len(shown) > 0 && shown[len(shown)-1].Kind == keys.LineBlank {
		shown[len(shown)-1] = lines[end]
	}
	out := make([]string, 0, height)
	for _, l := range shown {
		switch l.Kind {
		case keys.LineHeader:
			out = append(out, kbHeader(l.Text, st.KeyW, cfg, width))
		case keys.LineRow:
			r := st.Matches[l.Row]
			f := off(r)
			out = append(out, menu.Row(menu.RowSpec{Left: kbRowLeft(r, f, l.Row == st.Cursor, st.KeyW, cfg), Cursor: l.Row == st.Cursor}, cfg, width))
		case keys.LineBlank:
			out = append(out, menu.Note("", width, cfg))
		default:
			out = append(out, menu.Note(" "+l.Text, width, cfg))
		}
	}
	if st.FilterText == "" {
		return menu.Block(out, width, height)
	}
	for len(out) < window {
		out = append(out, menu.Note("", width, cfg))
	}
	for _, foot := range kbFoot(st, cfg, width) {
		out = append(out, menu.Note(foot, width, cfg))
	}
	return menu.Block(out, width, height)
}
