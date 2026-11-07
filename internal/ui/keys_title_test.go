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
		ownerHouses, ownerZodiac, ownerAspects, ownerHelp, ownerConsole, ownerDetails,
		ownerDisplay, ownerSessions, ownerSessionName, ownerActions, ownerLog, ownerExperiments,
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
			title := m.titleOf(e)
			if title == "" || e.Word == "" {
				t.Errorf("%s: bar entry %s has title %q and bar word %q", tb.name, e.BarKey, title, e.Word)
				continue
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
		{"Move row", main, "Move row"},
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
		keys.BindingTimeStepDay: "Step day", keys.BindingSlotCycleUp: "Move row", keys.BindingNextView: "Switch panel",
		keys.BindingFocusChart: "Focus panel", keys.BindingScrollUp: "Scroll Details",
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
		if e.ID == keys.BindingListUp && e.Word != "Move" {
			t.Errorf("a popup's list-up bar word is %q, want Move", e.Word)
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
	for _, l := range m.helpLines() {
		if !l.Binding && l.Text == dialog.TitleCity {
			break
		}
		if l.Binding {
			out = append(out, l.Text)
		}
	}
	return out
}

// 4: a pair is one Help row and one bar entry, and both keys act.
func TestPairIsOneRowAndBothKeysAct(t *testing.T) {
	m := helpFixture(t, 144, 40)
	pairs := []struct {
		title string
		ids   []keys.BindingID
		key   string
	}{
		{"Move row", []keys.BindingID{keys.BindingSlotCycleUp, keys.BindingSlotCycleDown}, "↑/↓ (j/k)"},
		{"Switch panel", []keys.BindingID{keys.BindingNextView, keys.BindingPrevView}, "tab/→ ⇧ tab/←"},
		{"Page list", []keys.BindingID{keys.BindingPageUp, keys.BindingPageDown}, "⇞/⇟"},
		{"Switch tab", []keys.BindingID{keys.BindingTabPrev, keys.BindingTabNext}, "[/]"},
		{"Scroll Details", []keys.BindingID{keys.BindingScrollUp, keys.BindingScrollDown}, "⌃⇞/⌃⇟"},
		{"Focus panel", []keys.BindingID{keys.BindingFocusChart, keys.BindingFocusBodies, keys.BindingFocusAspects}, "1/2/3"},
	}
	table := mainBindings(m.keys)
	rows := mainHelpRows(m)
	for _, p := range pairs {
		count := 0
		for _, r := range rows {
			if strings.HasPrefix(r, p.title+" ") {
				count++
				if !strings.HasSuffix(r, "  "+p.key) {
					t.Errorf("the %q row is %q, want its keys %q", p.title, r, p.key)
				}
			}
		}
		if count != 1 {
			t.Errorf("Help has %d rows titled %q, want one", count, p.title)
		}
		for i, id := range p.ids {
			var e keys.BindingEntry
			for _, c := range table {
				if c.ID == id {
					e = c
				}
			}
			if (i == 0) == e.Folded || (i > 0 && (e.Bar || e.Help)) {
				t.Errorf("%q: binding %d folded=%v bar=%v help=%v, want only the first entry to show", p.title, id, e.Folded, e.Bar, e.Help)
			}
			if len(e.Binding.Keys()) == 0 {
				t.Fatalf("%q: binding %d has no keys", p.title, id)
			}
			for _, k := range e.Binding.Keys() {
				if got := keys.MatchBinding(titleKeyMsg(k), table); got != id {
					t.Errorf("%q: key %q matches binding %d, want %d", p.title, k, got, id)
				}
			}
		}
	}
	bar := 0
	for _, e := range m.withWords(ownerMain, table) {
		if e.Bar && e.Word == "Move row" {
			bar++
			if e.BarKey != "↑↓" {
				t.Errorf("the bar names %q, want ↑↓", e.BarKey)
			}
		}
	}
	if bar != 1 {
		t.Errorf("the bar has %d Move row entries, want one", bar)
	}
	// Both keys act: down then up moves the Chart cursor there and back.
	m.focused = PanelChart
	m.cursorIdx = 0
	down, _ := escSend(t, m, escKey(tea.KeyDown))
	up, _ := escSend(t, down, escKey(tea.KeyUp))
	if down.cursorIdx == 0 || up.cursorIdx != 0 {
		t.Errorf("down moved the cursor to %d and up back to %d, want a move and a return to 0", down.cursorIdx, up.cursorIdx)
	}
}

// 5: Enter reads Timeline or Report in the bar and in Help, by the tab it shows.
func TestEnterTitleFollowsTheTab(t *testing.T) {
	m := helpFixture(t, 144, 40)
	for _, c := range []struct {
		panel PanelID
		want  string
	}{{PanelChart, "Timeline"}, {PanelBodies, "Report"}, {PanelAspects, "Report"}} {
		m.focused = c.panel
		bar := barActions(m)
		if len(bar) < 2 || bar[1] != c.want+": enter" {
			t.Errorf("focus %v: bar = %v, want second entry %q", c.panel, bar, c.want+": enter")
		}
		found := false
		for _, r := range mainHelpRows(m) {
			if strings.HasSuffix(r, "  enter") {
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
		shown := "Report"
		if after.activeTabID(PanelDetails) == TabDetailsTimeline {
			shown = "Timeline"
		}
		if shown != c.want {
			t.Errorf("focus %v: title %q but Enter shows the %s tab", c.panel, c.want, shown)
		}
	}
	m.focused = PanelChart
	if line := barLine(m); !strings.Contains(line, "Switch panel: ←→ | Timeline: enter | Select: space") {
		t.Errorf("Chart-focused bar = %q", line)
	}
}

// popupSectionOrder is the rule's result: popups by the opening key's place in
// the main table (the leader-only Sessions last).
func popupSectionOrder() []struct {
	title string
	owner owner
} {
	return []struct {
		title string
		owner owner
	}{
		{dialog.TitleCity, ownerCity}, {dialog.TitleOpenChart, ownerOpenChart}, {dialog.TitleSaveChart, ownerSaveChart},
		{dialog.TitleBodies, ownerBodies}, {dialog.TitleHouses, ownerHouses}, {dialog.TitleZodiac, ownerZodiac},
		{dialog.TitleAspects, ownerAspects}, {dialog.TitleLog, ownerLog}, {dialog.TitleHouseDisplay, ownerDisplay},
		{dialog.TitleSessions, ownerSessions}, {dialog.TitleLogConsole, ownerConsole},
	}
}

// 6: Help lists a section for each popup, in order, titled with its own title.
func TestHelpHasOneSectionPerPopup(t *testing.T) {
	m := helpFixture(t, 144, 40)
	want := popupSectionOrder()
	var got []string
	lines := m.helpLines()
	for i, l := range lines {
		if l.Binding || l.Text == "" {
			continue
		}
		for _, w := range want {
			if l.Text == w.title {
				got = append(got, l.Text)
				if i+1 >= len(lines) || !lines[i+1].Binding {
					t.Errorf("section %q has no rows under it", w.title)
				}
			}
		}
	}
	var wantTitles []string
	for _, w := range want {
		wantTitles = append(wantTitles, w.title)
	}
	if strings.Join(got, "|") != strings.Join(wantTitles, "|") {
		t.Errorf("Help's popup sections are %v, want %v", got, wantTitles)
	}
}

// 7: a section lists its popup's table, the bar's keys and the ones the bar omits.
func TestPopupSectionMatchesItsTable(t *testing.T) {
	m := helpFixture(t, 144, 40)
	blocks := m.helpPopupBlocks()
	if len(blocks) != len(popupSectionOrder()) {
		t.Fatalf("%d popup blocks, want %d", len(blocks), len(popupSectionOrder()))
	}
	for i, w := range popupSectionOrder() {
		b := blocks[i]
		if b.Title != w.title {
			t.Errorf("block %d is %q, want %q", i, b.Title, w.title)
			continue
		}
		var want []string
		omitted := 0
		for _, e := range m.ownerBindings(w.owner) {
			if e.Title == "" || e.Folded {
				continue
			}
			want = append(want, e.Title+" "+e.Binding.Help().Key)
			if !e.Bar {
				omitted++
			}
		}
		var got []string
		for _, r := range b.Rows {
			got = append(got, r.Title+" "+r.Key)
		}
		if strings.Join(got, "|") != strings.Join(want, "|") || len(want) == 0 {
			t.Errorf("%s section is %v, want its table %v", w.title, got, want)
		}
		if omitted == 0 && w.owner != ownerSaveChart { // Save chart has only its two bar keys
			t.Errorf("%s: no entry the bar omits is listed", w.title)
		}
	}
}

// 9: the README says where each popup's keys are.
func TestReadmeKeyTitles(t *testing.T) {
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(string(b)), " ")
	if !strings.Contains(text, "`?` lists every key, including each popup's.") {
		t.Errorf("the README does not say `?` lists every key, including each popup's")
	}
	i := strings.Index(text, "### Change the view, not the file")
	j := strings.Index(text, "### Keep a moment")
	if i < 0 || j < i || !strings.Contains(text[i:j], "The Help (`?`) lists the keys of every popup too.") {
		t.Errorf("'Change the view' does not carry the sentence about Help listing every popup's keys")
	}
}
