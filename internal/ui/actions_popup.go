package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
)

// actions_popup.go draws the Actions palette in the shared popup box: the
// search line and its rule, one row per command (title, the reason it cannot
// run now in a middle column, the key at the right edge), and the count on the
// bottom border. The Model's half is actions_popup_model.go.

// actionsNoMatch is the note shown when the search matches nothing, in the
// words of the finders' (`(no matching house systems)`).
const actionsNoMatch = "(no matching actions)"

// actionRowSpec is one command as a list row: its title, a blocked command's
// reason in a column that begins one space past the longest title, and the key
// pinned to the right. A blocked row is faint whole; with colours off its
// reason alone marks it. The reason is part of the left cell and not its
// LeftTail: the tail closes its own faint span, which would end the row's
// faint before the key.
func actionRowSpec(r actionRow, reason string, titleW int, cursor bool) menu.RowSpec {
	spec := menu.RowSpec{Left: r.Title, Right: []string{r.Key}, Cursor: cursor, Pinned: true}
	if reason != "" {
		spec.Left += strings.Repeat(" ", titleW-ansi.StringWidth(r.Title)+1) + reason
		spec.Dim = true
	}
	return spec
}

// countText is the bottom border's text: a refused Enter's `<Title>: <reason>`
// until the next key, else the number of commands with nothing typed, else
// `N of M` for a search.
func (st actionsState) countText() string {
	switch {
	case st.Notice != "":
		return st.Notice
	case strings.TrimSpace(st.FilterText) == "":
		return fmt.Sprintf("%d actions", len(st.Matches))
	}
	return fmt.Sprintf("%d of %d", len(st.Matches), st.Total)
}

// actionsCounter is the border text for the box: the widest it reads in the
// ordinary case (the box is never narrower than that) and the call that writes
// the current text into the bottom edge. An edge too short for the whole text
// keeps itself as it was, as for every counter.
func actionsCounter(st actionsState, cfg Config) *dialog.Counter {
	return &dialog.Counter{
		Widest: func() string { return fmt.Sprintf("%d actions", max(st.Total, 1)) },
		Draw: func(box string) string {
			return withPositionCounter(box, listPosition{Text: st.countText()}, cfg, true)
		},
	}
}

// actionsWidth is the content width the palette needs: the widest row of all
// the commands, each with the reason it has now. It is read once, when the
// palette opens, and kept in the state, so the box is the size it opened at
// while the operator types.
func actionsWidth(st actionsState, why func(keys.BindingID) string, cfg Config) int {
	width := 1
	for _, r := range st.Matches {
		width = max(width, menu.RowWidth(actionRowSpec(r, why(r.ID), st.TitleW, false), cfg))
	}
	return width
}

// renderActions draws the palette. why is blocked: the reason a command cannot
// run now, "" when it can, read at draw time so a row is never stale.
func renderActions(st actionsState, why func(keys.BindingID) string, cfg Config, termW, termH int) string {
	return dialog.Render(dialog.Box{
		Title:    dialog.TitleActions,
		Fit:      dialog.Fit{Width: max(st.Width, 1), Rows: st.Total},
		Position: actionsCounter(st, cfg),
		Scroll: func(height int) (pane.Viewport, int) {
			rows, off := st.WindowAt(st.Cursor, height)
			return pane.Viewport{Total: len(st.Matches), Visible: len(rows), Offset: off}, 0
		},
		FilterText: st.FilterText,
	}, cfg, termW, termH, popupChrome(cfg), func(width, height int) string {
		return menu.Block(actionRowLines(st, why, cfg, width, height), width, height)
	})
}

// actionRowLines are the visible rows, or the note when nothing matches.
func actionRowLines(st actionsState, why func(keys.BindingID) string, cfg Config, width, height int) []string {
	if height < 1 {
		return nil
	}
	if len(st.Matches) == 0 {
		return []string{menu.Note(actionsNoMatch, width, cfg)}
	}
	rows, off := st.WindowAt(st.Cursor, height)
	out := make([]string, 0, len(rows))
	for i, r := range rows {
		out = append(out, menu.Row(actionRowSpec(r, why(r.ID), st.TitleW, off+i == st.Cursor), cfg, width))
	}
	return out
}
