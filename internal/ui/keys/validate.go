package keys

import (
	"fmt"
	"sort"
	"strings"
)

// Reserved are the keys no command, popup or table may take for itself: Ctrl+C
// quits, a terminal sends Ctrl+H, Ctrl+I, Ctrl+J and Ctrl+M for Backspace, Tab,
// Line feed and Enter, and Esc, Enter and Tab are the ways out of a popup.
var Reserved = map[string]bool{
	"ctrl+c": true,
	"ctrl+h": true,
	"ctrl+i": true,
	"ctrl+j": true,
	"ctrl+m": true,
	"esc":    true,
	"enter":  true,
	"tab":    true,
}

// Registry is every key in use, read from tables that already exist: each
// owner's tables (one per variant, the main screen's included) and the
// command map.
type Registry struct {
	Owners   map[Owner][][]BindingEntry
	Commands []Command
}

// Clash is one key that two things claim, or one that may not be claimed.
type Clash struct {
	Owner string
	Key   string
	What  string
}

func (c Clash) String() string {
	return fmt.Sprintf("%s: %s: %s", c.Owner, c.Key, c.What)
}

// popupCtrl are the only Ctrl letters a popup may bind: rename in the chart
// picker and delete in the Sessions list.
var popupCtrl = map[string]bool{"ctrl+r": true, "ctrl+d": true}

// OwnerName is the owner's name as a Clash prints it.
func OwnerName(o Owner) string {
	names := map[Owner]string{
		OwnerMain: "main", OwnerCity: "city", OwnerOpenChart: "open chart",
		OwnerChartTitle: "chart title", OwnerSaveChart: "save chart",
		OwnerBodies: "bodies", OwnerHouses: "houses", OwnerZodiac: "zodiac",
		OwnerAspects: "aspects", OwnerConsole: "console",
		OwnerDetails: "details", OwnerSessions: "sessions",
		OwnerSessionName: "session name", OwnerKeybindings: "keybindings",
		OwnerLog: "log", OwnerExperiments: "experiments", OwnerNodes: "nodes",
	}
	if n, ok := names[o]; ok {
		return n
	}
	return fmt.Sprintf("owner %d", int(o))
}

// entryLabel names an entry in a Clash: its title, else its keys, else its id.
func entryLabel(e BindingEntry) string {
	switch {
	case e.Title != "":
		return e.Title
	case len(e.Binding.Keys()) > 0:
		return strings.Join(e.Binding.Keys(), "/")
	}
	return fmt.Sprintf("id %d", e.ID)
}

// CtrlLetter is the letter of a Ctrl letter key (`ctrl+r` is `r`), and whether k
// is one a comma can stand for: the keys a terminal reserves are not.
func CtrlLetter(k string) (string, bool) {
	if !ctrlLetter(k) || Reserved[k] {
		return "", false
	}
	return k[5:], true
}

// ctrlLetter is whether k is a Ctrl letter (ctrl+a to ctrl+z).
func ctrlLetter(k string) bool {
	return len(k) == 6 && strings.HasPrefix(k, "ctrl+") && k[5] >= 'a' && k[5] <= 'z'
}

// Validate returns every clash among the keys in use, in a fixed order:
// an entry with no group, a key two entries of one owner both match, a command whose Ctrl key the
// terminal owns or the main screen gave to another action, a lower-case
// command letter with no working Ctrl form, a popup holding a Ctrl letter,
// and two commands typed with the same sequence. Keys are compared as Bubble
// Tea names them.
func Validate(r Registry) []Clash {
	var out []Clash
	owners := make([]Owner, 0, len(r.Owners))
	for o := range r.Owners {
		owners = append(owners, o)
	}
	sort.Slice(owners, func(i, j int) bool { return owners[i] < owners[j] })

	for _, o := range owners {
		name := OwnerName(o)
		for _, table := range r.Owners[o] {
			seen := map[string]BindingEntry{}
			for _, e := range table {
				if e.Group == GroupNone {
					out = append(out, Clash{name, entryLabel(e), "the entry declares no group (Local, Global or Navigation)"})
				}
				for _, k := range e.Binding.Keys() {
					if prev, ok := seen[k]; ok && prev.ID != e.ID {
						out = append(out, Clash{name, k, fmt.Sprintf("two entries match this key (ids %d and %d)", prev.ID, e.ID)})
					} else if !ok {
						seen[k] = e
					}
					if o != OwnerMain && o != OwnerDetails && o != OwnerLog && o != OwnerConsole && ctrlLetter(k) && !popupCtrl[k] {
						out = append(out, Clash{name, k, "a popup binds a Ctrl letter other than ctrl+r and ctrl+d"})
					}
				}
			}
		}
	}

	mainKeys := map[string]BindingID{}
	for _, table := range r.Owners[OwnerMain] {
		for _, e := range table {
			for _, k := range e.Binding.Keys() {
				mainKeys[k] = e.ID
			}
		}
	}
	seqs := map[string]Command{}
	for _, c := range r.Commands {
		seq := c.Letter + c.Then
		if prev, ok := seqs[seq]; ok {
			out = append(out, Clash{"commands", seq, fmt.Sprintf("two rows share this sequence (ids %d and %d)", prev.ID, c.ID)})
		} else {
			seqs[seq] = c
		}
		if c.Then != "" || c.Letter == "" || c.Letter[0] < 'a' || c.Letter[0] > 'z' {
			continue
		}
		k := CtrlKey(c.Letter)
		if k == "" || Reserved[k] {
			out = append(out, Clash{"commands", k, fmt.Sprintf("letter %q has no working Ctrl form", c.Letter)})
			continue
		}
		if id, ok := mainKeys[k]; ok && id != c.ID {
			out = append(out, Clash{"commands", k, fmt.Sprintf("the main screen binds this key to id %d, not %d", id, c.ID)})
		}
	}
	return out
}
