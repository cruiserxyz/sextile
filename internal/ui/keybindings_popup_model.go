package ui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
)

// keybindings_popup_model.go is the Model's half of the Keybindings popup (`?`,
// `, p` and Ctrl+P; the popup itself is keybindings_popup.go): every key, in
// three groups, searchable. It is the one place keys are listed besides the
// key bar. Its rows are the entries of the key tables (keys.KeybindingRows);
// Enter on a row runs it as its key would, and a row that cannot run where the
// popup was opened is drawn struck through and does nothing.
//
// The popup is a flag, keybindingsOpen, over whatever had the keyboard
// (kbOrigin): the main screen, attached Details, or a chooser that stays as it
// was underneath and has the keyboard back on Esc. It is never a startup value:
// blocked asks m.startup == startupReady, and a startup value would grey every
// row.

// keybindingsBindings is the popup's table. It types into its filter, so the
// list moves with the arrows and the paging keys only.
func keybindingsBindings() []keys.BindingEntry {
	return keys.TitledLocal(append([]keys.BindingEntry{
		listUp().ShownWhen(func(c keys.Context) bool { return c.Popup.Rows > 1 }),
		listDown(),
		confirm("Run").ShownWhen(func(c keys.Context) bool { return c.Popup.Rows > 0 }),
		{ID: keys.BindingSearch, Binding: key.NewBinding(), Title: "Filter", BarKey: "type", Bar: true, BarWhen: func(c keys.Context) bool { return !c.Popup.Filtering }},
		{
			ID: keys.BindingClose, Binding: key.NewBinding(key.WithKeys("esc", "?")), Title: "Close", BarKey: "esc/?", Bar: true, Pinned: true,
			TitleFn:  func(c keys.Context) string { return map[bool]string{true: "Clear", false: "Close"}[c.Popup.Filtering] },
			BarKeyFn: func(c keys.Context) string { return map[bool]string{true: "esc", false: "esc/?"}[c.Popup.Filtering] },
		},
	}, pageEntries()...))
}

// kbListHeight is how many lines of the list the popup shows: the box's own
// height, less the rule and the filter line while a filter is typed.
func (m Model) kbListHeight(st keys.KbState) int {
	h := min(min(dialog.ListHeight(m.height)+2, max(st.Rows, 1)), max(m.height-4, 1))
	if st.FilterText != "" {
		h = max(h-2, 1)
	}
	return h
}

// keybindingRows are the rows of the popup for origin, the owner that has the
// keyboard now.
func (m Model) keybindingRows(origin owner) []keys.Row {
	main := m.keysContextFor(ownerMain)
	originCtx := main
	if origin != ownerMain {
		originCtx = m.keysContextFor(origin)
	}
	return keys.KeybindingRows(keys.KbInput{
		Origin: origin, OriginTable: originCtx.Table, OriginCtx: originCtx,
		Main: main.Table, LeaderOnly: leaderOnlyBindings(), MainCtx: main,
		Recent: m.recentActions,
	})
}

// openKeybindings opens the popup over whatever has the keyboard now: at the
// keys of that place, or, with global set, scrolled to the commands with the
// cursor on the first.
func openKeybindings(m Model, global bool) (tea.Model, tea.Cmd) {
	origin := m.currentOwner()
	var notes []string
	if origin == ownerZodiac {
		notes = keys.ZodiacNote
	}
	st := keys.NewKbState(m.keybindingRows(origin), origin, notes)
	st.Width = keybindingsWidth(st, m.kbOff, m.cfg)
	if global {
		st.ShowGroup(keys.GroupGlobal, m.kbListHeight(st))
	}
	m.keybindings = st
	m.keybindingsOpen = true
	return m, nil
}

// kbOff is whether a row cannot run now. A command's answer is read when it
// is asked, so a row never goes stale while the popup is open.
func (m Model) kbOff(r keys.Row) bool {
	return r.Off || (r.Command && m.blocked(r.ID) != "")
}

// closeKeybindings closes the popup; whatever it was opened over has the
// keyboard again, as it was.
func (m Model) closeKeybindings() Model {
	m.keybindingsOpen = false
	m.keybindings = keys.KbState{}
	return m
}

// updateKeybindingsKey handles one keystroke while the popup has the keyboard.
func updateKeybindingsKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	st := m.keybindings
	height := m.kbListHeight(st)
	switch keys.MatchBinding(msg, keybindingsBindings()) {
	case keys.BindingClose:
		switch {
		case msg.String() == "?" && st.FilterText != "":
			// A filter is being typed: the question mark is text.
		case st.FilterText != "":
			st.FilterText = ""
			st.Refilter()
			m.keybindings = st
			return m, nil
		default:
			return m.closeKeybindings(), nil
		}
	case keys.BindingConfirm:
		return m.runKeybindingsRow()
	case keys.BindingListUp:
		st.MoveBy(-1, height)
		m.keybindings = st
		return m, nil
	case keys.BindingListDown:
		st.MoveBy(1, height)
		m.keybindings = st
		return m, nil
	case keys.BindingPageUp:
		st.MoveBy(-max(height-1, 1), height)
		m.keybindings = st
		return m, nil
	case keys.BindingPageDown:
		st.MoveBy(max(height-1, 1), height)
		m.keybindings = st
		return m, nil
	case keys.BindingPageHome:
		st.MoveBy(-len(st.Matches), height)
		m.keybindings = st
		return m, nil
	case keys.BindingPageEnd:
		st.MoveBy(len(st.Matches), height)
		m.keybindings = st
		return m, nil
	}
	if st.TypeKey(msg) {
		st.Refilter()
		st.Reveal(m.kbListHeight(st))
		m.keybindings = st
	}
	return m, nil
}

// runKeybindingsRow is Enter on the row under the cursor. A row that cannot
// run now does nothing. Any other closes the popup and runs as its key would:
// a command by its id (and, from Global, becomes the most recent), a key by
// pressing it where the popup was opened, so a row that asks before it writes
// still asks and a chooser it was opened over has it.
func (m Model) runKeybindingsRow() (tea.Model, tea.Cmd) {
	st := m.keybindings
	if st.Cursor < 0 || st.Cursor >= len(st.Matches) {
		return m, nil
	}
	row := st.Matches[st.Cursor]
	if m.kbOff(row) {
		return m, nil
	}
	m = m.closeKeybindings()
	if row.Command {
		if row.Group == keys.GroupGlobal && m.isPaletteCommand(row.ID) && row.ID != keys.BindingHelp {
			m.recentActions = promoteAction(m.recentActions, row.ID)
		}
		tm, cmd, _ := m.runAction(row.ID)
		return tm, cmd
	}
	if len(row.Keys) == 0 {
		return m, nil
	}
	return m.Update(pressedKey(row.Keys[0]))
}

// pressedKey is the key press Bubble Tea would deliver for the key name a
// binding carries (`ctrl+o`, `shift+tab`, `y`).
func pressedKey(name string) tea.KeyMsg {
	named := map[string]tea.KeyType{
		"tab": tea.KeyTab, "shift+tab": tea.KeyShiftTab, "enter": tea.KeyEnter, "esc": tea.KeyEsc,
		"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
		"shift+up": tea.KeyShiftUp, "shift+down": tea.KeyShiftDown,
		"pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "home": tea.KeyHome, "end": tea.KeyEnd,
		"delete": tea.KeyDelete, "backspace": tea.KeyBackspace,
		"ctrl+pgup": tea.KeyCtrlPgUp, "ctrl+pgdown": tea.KeyCtrlPgDown,
	}
	switch {
	case name == " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	case named[name] != 0:
		return tea.KeyMsg{Type: named[name]}
	case len(name) == 6 && strings.HasPrefix(name, "ctrl+") && name[5] >= 'a' && name[5] <= 'z':
		return tea.KeyMsg{Type: tea.KeyCtrlA + tea.KeyType(name[5]-'a')}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
}

// isPaletteCommand is whether id is one of the commands the popup keeps
// recent.
func (m Model) isPaletteCommand(id keys.BindingID) bool {
	return slices.ContainsFunc(m.paletteEntries(), func(e keys.BindingEntry) bool { return e.ID == id })
}

// maxRecentActions is how many recent commands the list shows first and the
// session keeps.
const maxRecentActions = 5

// promoteAction is recent with id first, listed once, cut to maxRecentActions.
func promoteAction(recent []keys.BindingID, id keys.BindingID) []keys.BindingID {
	out := make([]keys.BindingID, 0, maxRecentActions)
	out = append(out, id)
	for _, r := range recent {
		if r != id && len(out) < maxRecentActions {
			out = append(out, r)
		}
	}
	return out
}

// paletteEntries are the commands: the entries whose table row carries Palette.
func (m Model) paletteEntries() []keys.BindingEntry {
	var out []keys.BindingEntry
	for _, e := range slices.Concat(mainBindings(m.keys), leaderOnlyBindings()) {
		if e.Palette {
			out = append(out, e)
		}
	}
	return out
}

// recentActionTitles are the recent commands for the session file, by each
// command's own title (never a title that changes with the screen); nil when
// there are none.
func (m Model) recentActionTitles() []string {
	var out []string
	for _, id := range m.recentActions {
		for _, e := range m.paletteEntries() {
			if e.ID == id {
				out = append(out, e.Title)
				break
			}
		}
	}
	return out
}

// recentActionsFromTitles reads the session's titles back: a title no command
// has any more is dropped, a repeat counts once, and the list is cut to
// maxRecentActions.
func (m Model) recentActionsFromTitles(titles []string) []keys.BindingID {
	var out []keys.BindingID
	for _, t := range titles {
		for _, e := range m.paletteEntries() {
			if e.Title == t && !slices.Contains(out, e.ID) && len(out) < maxRecentActions {
				out = append(out, e.ID)
				break
			}
		}
	}
	return out
}
