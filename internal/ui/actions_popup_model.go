package ui

import (
	"slices"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
)

// actions_popup_model.go is the Model's half of the Actions palette (⌃p; the
// popup itself is actions_popup.go): every command with its key, searchable.
// Its rows are the entries whose table row carries Palette; each is the title
// and key Help prints, and a command that cannot run now is drawn faint with
// the reason blocked gives (actions.go). Enter on an available row closes the
// palette and runs the command by its id, as its key would; Enter on a blocked
// row runs nothing.
//
// The list is menu.FuzzyList, the chassis popup.Finder is built on, and not
// popup.Finder itself: that is the house and zodiac finder, whose choices are
// fixed when it opens, which names a chart in its title and marks a current
// choice. The palette's rows read the model live (a reason changes while the
// palette is open), its title is fixed, and its count line also carries the
// reason of a refused Enter. Typing, the cursor, paging and the window are the
// chassis's own code, so they behave as in every other popup.
//
// The palette's open state is the flag actionsOpen beside helpOpen, never a
// startup value: blocked asks m.startup == startupReady, and a startup value
// would grey every row.

// actionRow is one command as the palette lists it: its id, the title for
// this screen (Enter reads Timeline or Report) and the key Help spells for it.
type actionRow struct {
	ID    keys.BindingID
	Title string
	Key   string
}

// actionsState is the palette while it is open. The list machinery is the
// chassis's.
type actionsState struct {
	menu.FuzzyList[actionRow]
	// Total is how many commands the palette lists, whatever is typed.
	Total int
	// TitleW is the widest title over all of them: the reason column begins
	// one cell past it, whatever is typed.
	TitleW int
	// Width is the box's content width, decided once when the palette opens
	// from the widest row it showed then (operator, 2026-10-27: a popup's size
	// never changes while it is open).
	Width int
	// Notice is `<Title>: <reason>` after Enter on a blocked row. It stands in
	// the count's place until the next key.
	Notice string
}

// actionsBindings is the palette's table. It types into its search line, so
// the list moves with the arrows and the paging keys only.
func actionsBindings() []keys.BindingEntry {
	return keys.Titled(append([]keys.BindingEntry{
		listUp().ShownWhen(func(c keys.Context) bool { return c.Popup.Rows > 1 }),
		listDown(),
		confirm("Run").ShownWhen(func(c keys.Context) bool { return c.Popup.Rows > 0 }),
		closeOn("Close"),
	}, pageEntries()...))
}

// openActions is ⌃p: the palette over the main screen, nothing typed, the
// cursor on the first row.
func openActions(m Model) (tea.Model, tea.Cmd) {
	st := m.refilterActions(actionsState{})
	st.Width = actionsWidth(st, m.blocked, m.cfg)
	st.ScrollTo(st.Cursor, dialog.ListHeight(m.height))
	m.actions = st
	m.actionsOpen = true
	return m, nil
}

// actionRows is every command, in the order an empty search shows: the ones
// run through the palette in this run first (most recent first), then the rest
// alphabetically by title.
func (m Model) actionRows() []actionRow {
	c := m.keysContextFor(ownerMain)
	var rows []actionRow
	for _, e := range slices.Concat(mainBindings(m.keys), leaderOnlyBindings()) {
		if e.Palette {
			rows = append(rows, actionRow{ID: e.ID, Title: e.TitleFor(c), Key: keys.ActionKey(c, e)})
		}
	}
	recency := func(id keys.BindingID) int {
		if i := slices.Index(m.recentActions, id); i >= 0 {
			return i
		}
		return len(m.recentActions)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if ri, rj := recency(rows[i].ID), recency(rows[j].ID); ri != rj {
			return ri < rj
		}
		return strings.ToLower(rows[i].Title) < strings.ToLower(rows[j].Title)
	})
	return rows
}

// refilterActions recomputes the list for st's search text: every command when
// nothing is typed, else the ones whose title the text matches, ranked by
// location.MatchTier as the house and zodiac finders rank. Within a tier the
// order is actionRows': recent first, then alphabetical.
func (m Model) refilterActions(st actionsState) actionsState {
	all := m.actionRows()
	st.Total = len(all)
	st.TitleW = 0
	for _, r := range all {
		st.TitleW = max(st.TitleW, ansi.StringWidth(r.Title))
	}
	q := strings.ToLower(strings.TrimSpace(st.FilterText))
	if q == "" {
		st.SetMatches(all, false)
		return st
	}
	var hits []actionRow
	var tiers []int
	for _, r := range all {
		if t := location.MatchTier(strings.ToLower(r.Title), q); t > 0 {
			hits = append(hits, r)
			tiers = append(tiers, t)
		}
	}
	st.SetMatches(menu.RankByTier(hits, tiers), false)
	return st
}

// updateActionsKey handles one keystroke while the palette has the keyboard.
func updateActionsKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	st := m.actions
	st.Notice = "" // a refusal's reason lasts until the next key
	height := dialog.ListHeight(m.height)
	switch keys.MatchBinding(msg, actionsBindings()) {
	case keys.BindingClose:
		m.actionsOpen = false
		m.actions = actionsState{}
		return m, nil
	case keys.BindingConfirm:
		return m.runActionRow(st)
	case keys.BindingListUp:
		st.MoveUp()
		st.ScrollTo(st.Cursor, height)
	case keys.BindingListDown:
		st.MoveDown()
		st.ScrollTo(st.Cursor, height)
	case keys.BindingPageUp:
		st.Page(menu.PageUp, height)
	case keys.BindingPageDown:
		st.Page(menu.PageDown, height)
	case keys.BindingPageHome:
		st.Page(menu.PageHome, height)
	case keys.BindingPageEnd:
		st.Page(menu.PageEnd, height)
	default:
		if st.TypeKey(msg) {
			st = m.refilterActions(st)
			st.Cursor = 0
			st.ScrollTo(st.Cursor, height)
		}
	}
	m.actions = st
	return m, nil
}

// runActionRow is Enter on the row under the cursor. A command that cannot run
// now runs nothing and says why in the count's place; any other closes the
// palette, becomes the most recent, and runs by its id exactly as its key
// would, so a chooser it opens returns to the main screen on Esc.
func (m Model) runActionRow(st actionsState) (tea.Model, tea.Cmd) {
	if st.Cursor < 0 || st.Cursor >= len(st.Matches) {
		m.actions = st
		return m, nil
	}
	row := st.Matches[st.Cursor]
	if why := m.blocked(row.ID); why != "" {
		st.Notice = row.Title + ": " + why
		m.actions = st
		return m, nil
	}
	m.actionsOpen = false
	m.actions = actionsState{}
	m.recentActions = promoteAction(m.recentActions, row.ID)
	tm, cmd, _ := m.runAction(row.ID)
	return tm, cmd
}

// promoteAction is recent with id first and listed once.
func promoteAction(recent []keys.BindingID, id keys.BindingID) []keys.BindingID {
	out := make([]keys.BindingID, 0, len(recent)+1)
	out = append(out, id)
	for _, r := range recent {
		if r != id {
			out = append(out, r)
		}
	}
	return out
}
