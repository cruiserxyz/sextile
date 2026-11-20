package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ui/dialog"
)

// keys-one-title: every key has one title, written once. Help shows it, the
// bar shortens it by one rule (specs/keys-one-title.md).

// titleTable is one table of the keyboard and the owner whose bar it feeds.
type titleTable struct {
	name    string
	owner   owner
	entries []keys.BindingEntry
}

// titleTables are every table function of the keyboard, each as its owner
// would hand it over.
func titleTables(m Model) []titleTable {
	var out []titleTable
	for _, o := range []owner{
		ownerMain, ownerCity, ownerOpenChart, ownerChartTitle, ownerSaveChart, ownerBodies,
		ownerHouses, ownerZodiac, ownerAspects, ownerKeybindings, ownerConsole, ownerDetails,
		ownerSessions, ownerSessionName, ownerKeybindings, ownerLog, ownerExperiments,
	} {
		out = append(out, titleTable{"owner " + string(rune('A'+int(o))), o, m.ownerBindings(o)})
	}
	return append(out,
		titleTable{"title confirm", ownerChartTitle, titleConfirmBindings()},
		titleTable{"timeline search", ownerDetails, timelineSearchBindings()},
		titleTable{"report search", ownerDetails, reportSearchBindings()},
		titleTable{"leader only", ownerMain, leaderOnlyBindings()},
	)
}

func wordsSubsequence(sub, of []string) bool {
	i := 0
	for _, w := range of {
		if i < len(sub) && sub[i] == w {
			i++
		}
	}
	return i == len(sub)
}

// 1: every bar word is its Help title with words removed, in order, never
// another word, and only articles or the owner's implied words are removed.
func TestBarWordIsTheTitleWithWordsRemoved(t *testing.T) {
	m := helpFixture(t, 144, 40)
	bars := 0
	for _, tb := range titleTables(m) {
		for _, e := range m.withWords(tb.owner, tb.entries) {
			if !e.Bar {
				continue
			}
			bars++
			// The title is read under the owner's own facts: Details' Esc names the
			// panel it goes back to.
			title := e.TitleFor(m.keysFacts(tb.owner))
			if title == "" || e.Word == "" {
				t.Errorf("%s: bar entry %s has title %q and bar word %q", tb.name, e.BarKey, title, e.Word)
				continue
			}
			if e.BarTitle != "" {
				title = e.BarTitle
			}
			tw, ww := strings.Fields(title), strings.Fields(e.Word)
			if !wordsSubsequence(ww, tw) {
				t.Errorf("%s: bar word %q is not %q with words removed", tb.name, e.Word, title)
				continue
			}
			drop := map[string]bool{"a": true, "an": true, "the": true}
			for _, w := range keys.ImpliedWords(tb.owner) {
				drop[w] = true
			}
			kept := map[int]bool{}
			i := 0
			for j, w := range tw {
				if i < len(ww) && ww[i] == w {
					kept[j] = true
					i++
				}
			}
			for j, w := range tw {
				if !kept[j] && !drop[w] {
					t.Errorf("%s: %q lost the word %q, which is neither an article nor implied by its owner", tb.name, title, w)
				}
			}
		}
	}
	if bars < 40 {
		t.Fatalf("only %d bar entries read: the tables were not walked", bars)
	}
}

// 2: the one shortening rule.
func TestBarWordRule(t *testing.T) {
	main, popup, details := keys.ImpliedWords(ownerMain), keys.ImpliedWords(ownerCity), keys.ImpliedWords(ownerDetails)
	for _, c := range []struct {
		title   string
		implied []string
		want    string
	}{
		{"Move chart up", main, "Move up"},
		{"Remove chart", main, "Remove"},
		{"Step a day", main, "Step day"},
		{"Move chart", main, "Move"},
		{"Move row", popup, "Move"},
		{"Page list", popup, "Page"},
		{"Scroll Details", main, "Scroll Details"},
		{"Scroll Details", details, "Scroll"},
		{"Switch tab", details, "Switch tab"},
		{"Focus panel", main, "Focus panel"},
	} {
		if got := keys.BarWord(c.title, c.implied); got != c.want {
			t.Errorf("keys.BarWord(%q, %v) = %q, want %q", c.title, c.implied, got, c.want)
		}
	}
	m := helpFixture(t, 144, 40)
	want := map[keys.BindingID]string{
		keys.BindingSlotMoveUp: "Move up", keys.BindingSlotMoveDown: "Move down", keys.BindingSlotDelete: "Remove",
		keys.BindingTimeStepDay:  "Step day",
		keys.BindingFocusSession: "Focus panel", keys.BindingScrollUp: "Scroll Details",
	}
	seen := 0
	for _, e := range m.withWords(ownerMain, m.ownerBindings(ownerMain)) {
		if w, ok := want[e.ID]; ok {
			seen++
			if e.Word != w {
				t.Errorf("main bar word of binding %d is %q, want %q", e.ID, e.Word, w)
			}
		}
	}
	if seen != len(want) {
		t.Errorf("read %d of %d main entries", seen, len(want))
	}
	for _, e := range m.withWords(ownerCity, m.ownerBindings(ownerCity)) {
		if e.ID == keys.BindingListUp && e.Bar {
			t.Errorf("a popup's list-up is a bar entry (%q): common movement is not on the footer", e.Word)
		}
	}
}

// 3: Help shows the title as written; Binding.Help().Desc is the title.
func TestHelpDescIsTheTitle(t *testing.T) {
	m := helpFixture(t, 144, 40)
	n := 0
	for _, tb := range titleTables(m) {
		for _, e := range tb.entries {
			n++
			if e.Binding.Help().Desc != e.Title {
				t.Errorf("%s: binding %d (%s) has Help().Desc %q but Title %q", tb.name, e.ID, e.BarKey, e.Binding.Help().Desc, e.Title)
			}
			if (e.Bar || e.Help) && e.Title == "" {
				t.Errorf("%s: binding %d (%s) is listed but has no title", tb.name, e.ID, e.BarKey)
			}
		}
	}
	if n < 150 {
		t.Fatalf("only %d entries read", n)
	}
}

func titleKeyMsg(k string) tea.KeyMsg {
	named := map[string]tea.KeyType{
		"up": tea.KeyUp, "down": tea.KeyDown, "tab": tea.KeyTab, "shift+tab": tea.KeyShiftTab,
		"right": tea.KeyRight, "left": tea.KeyLeft, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
		"ctrl+pgup": tea.KeyCtrlPgUp, "ctrl+pgdown": tea.KeyCtrlPgDown,
	}
	if t, ok := named[k]; ok {
		return tea.KeyMsg{Type: t}
	}
	return escRunes(k)
}

// mainHelpRows are Help's binding rows above the first popup section.
func mainHelpRows(m Model) []string {
	var out []string
	for _, l := range keybindingLines(m) {
		if !l.Binding && l.Text == dialog.TitleCity {
			break
		}
		if l.Binding {
			out = append(out, l.Text)
		}
	}
	return out
}

// 5: Enter reads Details in the bar and in Help, at every size and on every
// panel, whatever tab Details would show.
func TestEnterTitleFollowsTheTab(t *testing.T) {
	m := helpFixture(t, 144, 40)
	for _, c := range []struct {
		panel PanelID
		want  string
	}{{PanelStatus, "Details"}, {PanelChart, "Details"}, {PanelBodies, "Details"}, {PanelAspects, "Details"}} {
		m.focused = c.panel
		bar := barActions(m)
		if len(bar) < 1 || bar[0] != c.want+": <enter>" {
			t.Errorf("focus %v: bar = %v, want first entry %q", c.panel, bar, c.want+": <enter>")
		}
		found := false
		for _, r := range mainHelpRows(m) {
			if strings.HasSuffix(r, "  <enter>") {
				found = true
				if !strings.HasPrefix(r, c.want+" ") {
					t.Errorf("focus %v: Help row %q, want it titled %q", c.panel, r, c.want)
				}
			}
		}
		if !found {
			t.Errorf("focus %v: Help has no enter row", c.panel)
		}
		after, _ := m.attachDetails()
		if !after.details.attached {
			t.Errorf("focus %v: Enter did not attach Details", c.panel)
		}
	}
	m.focused = PanelChart
	if line := barLine(m); !strings.Contains(line, "Details: <enter> | Moon phase: p/P | Select: <space>") {
		t.Errorf("Chart-focused bar = %q", line)
	}
}

// 9: the reference page says what `?` lists, in its Keys section.
const questionMarkSentence = "`?` lists the keys of the place where you are, and every command."

func keysSectionSaysQuestionMark(page string) bool {
	text := strings.Join(strings.Fields(page), " ")
	i := strings.Index(text, "## Keys ")
	j := strings.Index(text, "## Charts ")
	return i >= 0 && j > i && strings.Contains(text[i:j], questionMarkSentence)
}

func TestReadmeKeyTitles(t *testing.T) {
	b, err := os.ReadFile("../../docs/REFERENCE.md")
	if err != nil {
		t.Fatal(err)
	}
	if !keysSectionSaysQuestionMark(string(b)) {
		t.Errorf("the reference page's Keys section does not say %s", questionMarkSentence)
	}
	if keysSectionSaysQuestionMark(strings.Replace(string(b), questionMarkSentence, "", 1)) {
		t.Error("control: the missing sentence was not named")
	}
}
