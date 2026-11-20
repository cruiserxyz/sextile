package keys

import "strings"

// keyspell.go is how a key is spelled for a person: the leader's sequences,
// the commands' Ctrl and comma forms, and the one sentence that says what the
// leader does.

// Spell is how the Keybindings popup spells keys, given by Bubble Tea's own
// names, as lazygit does: a key with a name or a modifier stands in angle
// brackets (`<enter>`, `<ctrl+l>`, `<up>`), a printable key stays bare (`K`,
// `/`), and a pair is joined by a slash (`<up>/<down>`). A space is `<space>`.
func Spell(names ...string) string {
	out := make([]string, len(names))
	for i, n := range names {
		switch {
		case n == " ":
			out[i] = "<space>"
		case len([]rune(n)) <= 1:
			out[i] = n
		default:
			out[i] = "<" + n + ">"
		}
	}
	return strings.Join(out, "/")
}

// LeaderSpelling is how a notice prints a leader sequence: the configured leader,
// then the sequence as typed (`,l`, `,c d`).
func LeaderSpelling(leader string, seq []string) string { return leader + ShowSeq(seq) }

// ActionKey is the key the Keybindings popup prints for a command: its Ctrl
// key (`<ctrl+o>`), else its comma form with the configured leader (`, O`,
// `, c d`) when it has no Ctrl form, else the main screen's key when the entry
// has one, else nothing.
func ActionKey(c Context, e BindingEntry) string {
	for _, cmd := range commands {
		if cmd.ID != e.ID {
			continue
		}
		if _, ok := CommandCtrl(cmd); ok {
			k, _ := CommandCtrl(cmd)
			return Spell(k)
		}
		seq := cmd.Letter
		if cmd.Then != "" {
			seq += " " + cmd.Then
		}
		return strings.TrimSpace(c.Leader.Key + " " + seq)
	}
	if e.Help {
		return e.Binding.Help().Key
	}
	return ""
}

// BindingDesc is the description the context's table registers for an action.
func BindingDesc(c Context, id BindingID) string {
	for _, e := range c.Table {
		if e.ID == id {
			return e.Title
		}
	}
	return ""
}

// footerNamed are the key names a footer row writes in angle brackets.
var footerNamed = map[string]string{
	"enter": "<enter>", "esc": "<esc>", "space": "<space>", "tab": "<tab>", "up": "<up>", "down": "<down>",
	"left": "<left>", "right": "<right>", "pgup": "<pgup>", "pgdown": "<pgdown>", "home": "<home>", "end": "<end>",
	"del": "<delete>", "delete": "<delete>", "backspace": "<backspace>",
}

// FooterKey is how a footer row writes a key, as lazygit does: a named key in
// angle brackets (`<enter>`, `<esc>`), Ctrl as `<c-r>`, a letter bare. A pair
// or a run (`esc/q`, `y o w d h m s`) is written part by part. Keybindings keeps
// its own notation (Spell).
func FooterKey(k string) string {
	words := strings.Fields(k)
	for i, w := range words {
		parts := strings.Split(w, "/")
		for j, p := range parts {
			switch {
			case footerNamed[p] != "":
				parts[j] = footerNamed[p]
			case strings.HasPrefix(p, "ctrl+"):
				parts[j] = "<c-" + strings.TrimPrefix(p, "ctrl+") + ">"
			case strings.HasPrefix(p, "shift+"):
				parts[j] = "<s-" + strings.TrimPrefix(p, "shift+") + ">"
			}
		}
		words[i] = strings.Join(parts, "/")
	}
	return strings.Join(words, " ")
}
