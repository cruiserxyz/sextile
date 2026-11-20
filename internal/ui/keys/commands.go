package keys

import "github.com/charmbracelet/bubbles/key"

// Command is one letter of the map: `,` then Letter runs ID, and so does
// the Ctrl key CtrlKey derives from it.
type Command struct {
	Letter string // "o", "O", ...
	ID     BindingID
	Then   string // the second key of a two-key row ("" for every other row)
	Word   string // the pending menu's word
	// PrefixWord, on the first row of a group of two-key rows, is the word the
	// first line names the group's letter with (`c defaults…`).
	PrefixWord string
}

// commands is the map, in the menu's fixed order: the most used first, so a
// narrow terminal cuts the rare ones, and the two-key rows last.
var commands = []Command{
	{Letter: "o", ID: BindingOpenCharts, Word: "open"},
	{Letter: "s", ID: BindingSaveChart, Word: "save"},
	{Letter: "l", ID: BindingOpenAtlas, Word: "city"},
	{Letter: "u", ID: BindingHouses, Word: "houses"},
	{Letter: "z", ID: BindingZodiac, Word: "zodiac"},
	{Letter: "a", ID: BindingAspects, Word: "aspects"},
	{Letter: "b", ID: BindingBodies, Word: "bodies"},
	{Letter: "x", ID: BindingRenderWheel, Word: "wheel"},
	{Letter: "y", ID: BindingCopy, Word: "copy"},
	{Letter: "O", ID: BindingSessions, Word: "sessions"},
	{Letter: "L", ID: BindingLogConsole, Word: "log"},
	// t is last of the one-key rows, so a bar too narrow for every entry cuts it first.
	{Letter: "t", ID: BindingNodes, Word: "nodes"},
	{Letter: "c", Then: "d", ID: BindingSaveCity, Word: "save defaults", PrefixWord: "defaults…"},
}

// Commands is the map, in the menu's fixed order. The slice is a copy.
func Commands() []Command {
	return append([]Command(nil), commands...)
}

// CtrlKey is the Ctrl key a lower-case letter derives, as Bubble Tea names it
// ("ctrl+o"), or "" for anything else: a capital has the comma form only
// because Bubble Tea never reports Shift with a Ctrl letter. A letter whose
// Ctrl key the terminal already uses is refused by Validate, not hidden here.
func CtrlKey(letter string) string {
	if len(letter) != 1 || letter[0] < 'a' || letter[0] > 'z' {
		return ""
	}
	return "ctrl+" + letter
}

// CommandCtrl is the Ctrl key a command answers to: its derived key, for a
// one-key lower-case row whose key is not Reserved. The second result is false
// for every other row (a capital, a two-key row, a letter the terminal owns).
func CommandCtrl(c Command) (string, bool) {
	if c.Then != "" {
		return "", false
	}
	k := CtrlKey(c.Letter)
	if k == "" || Reserved[k] {
		return "", false
	}
	return k, true
}

// ctrlBinding is the main screen's binding for the command id: the Ctrl key
// its row answers to, or a binding that matches nothing when it has none.
func ctrlBinding(id BindingID) key.Binding {
	for _, c := range commands {
		if c.ID != id {
			continue
		}
		k, ok := CommandCtrl(c)
		if !ok {
			return key.NewBinding()
		}
		return key.NewBinding(key.WithKeys(k), key.WithHelp(Spell(k), ""))
	}
	return key.NewBinding()
}
