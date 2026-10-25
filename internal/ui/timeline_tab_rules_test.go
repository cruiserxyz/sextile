package ui

import (
	"slices"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// The Timeline tab is about the floating chart's month and the selected group
// that contains it (timeline-tab-rules T1): the tab exists for the floating
// slot, and for any member of a selected group that has the floating slot in
// it; everywhere else it is absent, not empty.

// ruleCase is one cursor over one selection of the three-slot list
// ① = index 0, ② = index 1, now = index 2.
type ruleCase struct {
	name     string
	selected []int // list indices of the selected slots
	cursor   int
	tab      bool  // the Timeline tab is declared
	natals   []int // the numerals the searches carry (the full list); nil is the sky alone
}

func ruleModel(t *testing.T, c ruleCase) Model {
	t.Helper()
	m := timelineModel(t, Config{Accessible: true})
	if err := addAnchor(m.slots, inMonth, testPlaceAmsterdam()); err != nil {
		t.Fatalf("addAnchor: %v", err)
	}
	for i := range m.slots.All() {
		m.slots.SetSlotSelected(i, slices.Contains(c.selected, i))
	}
	m.cursorIdx = c.cursor
	m.width = max(m.width, dualColumnMinWidth(m.shownCount()))
	return m.noteArrival()
}

// T1: the six rows of the rule's table, the nothing-selected case, and the
// floating slot selected alone. Where the tab exists, the searches it asks for
// say which list it is: Natals set is the full list, empty is the sky alone.
func TestTimelineTabFollowsTheGroup(t *testing.T) {
	for _, c := range []ruleCase{
		// The table, row by row.
		{name: "floating, in the group with a chart", selected: []int{0, 2}, cursor: 2, tab: true, natals: []int{1}},
		{name: "a chart in the group with the floating slot", selected: []int{0, 2}, cursor: 0, tab: true, natals: []int{1}},
		{name: "a chart outside the group with the floating slot", selected: []int{0, 2}, cursor: 1, tab: false},
		{name: "floating, not selected", selected: []int{0}, cursor: 2, tab: true, natals: nil},
		{name: "a chart in a group without the floating slot", selected: []int{0, 1}, cursor: 0, tab: false},
		{name: "a chart outside a group without the floating slot", selected: []int{0}, cursor: 1, tab: false},
		// Nothing selected: the floating slot shows the reduced list, a chart none.
		{name: "nothing selected, on floating", selected: nil, cursor: 2, tab: true, natals: nil},
		{name: "nothing selected, on a chart", selected: nil, cursor: 0, tab: false},
		// The floating slot selected alone: in the group, with no chart to add.
		{name: "floating selected alone", selected: []int{2}, cursor: 2, tab: true, natals: nil},
		// Every chart selected: the full list carries both, whichever one the cursor is on.
		{name: "all three, on the second chart", selected: []int{0, 1, 2}, cursor: 1, tab: true, natals: []int{1, 2}},
		{name: "all three, on floating", selected: []int{0, 1, 2}, cursor: 2, tab: true, natals: []int{1, 2}},
	} {
		t.Run(c.name, func(t *testing.T) {
			calls := stubTimeline(t, nil)
			m := ruleModel(t, c)
			has := slices.Contains(tabIDs(m.panelTabs(PanelDetails)), TabDetailsTimeline)
			if has != c.tab {
				t.Fatalf("tabs %v, want the Timeline tab present=%v", tabIDs(m.panelTabs(PanelDetails)), c.tab)
			}
			// Choosing the tab is what owes its search; an absent tab owes none.
			m, cmd := syncTimelineVisited(m)
			m = pump(t, m, cmd)
			if !c.tab {
				if len(*calls) != 0 {
					t.Fatalf("no tab, but %d timeline searches ran", len(*calls))
				}
				return
			}
			if len(*calls) == 0 {
				t.Fatal("the tab is declared but no search ran")
			}
			var got []int
			for _, n := range (*calls)[len(*calls)-1].req.Natals {
				got = append(got, n.Numeral)
			}
			if !slices.Equal(got, c.natals) {
				t.Errorf("the searches carry natals %v, want %v", got, c.natals)
			}
		})
	}
}

// The tab's presence is read from the cursor and the selection every time, so
// it follows a cursor move and a Space with no other signal.
func TestTimelineTabIsReevaluatedOnCursorAndSelection(t *testing.T) {
	m := ruleModel(t, ruleCase{selected: []int{0, 2}, cursor: 2})
	has := func() bool { return slices.Contains(tabIDs(m.panelTabs(PanelDetails)), TabDetailsTimeline) }
	if !has() {
		t.Fatal("fixture: no tab on the floating slot")
	}
	m, _ = tpress(t, m, keyUp) // ②, outside the group
	m, _ = tpress(t, m, keyUp) // ①, in the group with now
	if m.cursorIdx != 0 || !has() {
		t.Fatalf("on ①, in the group with now: cursor %d tab=%v, want the tab", m.cursorIdx, has())
	}
	m, _ = tpress(t, m, keyDown) // ②
	if m.cursorIdx != 1 || has() {
		t.Fatalf("on ②, outside the group: cursor %d tab=%v, want none", m.cursorIdx, has())
	}
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeySpace}) // ② joins the group
	if !m.slots.SlotSelected(1) || !has() {
		t.Fatalf("after Space on ②: selected=%v tab=%v, want the tab", m.slots.SlotSelected(1), has())
	}
	m, _ = tpress(t, m, keyDown) // now
	m, _ = tpress(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m, _ = tpress(t, m, keyUp) // ②, now out of the group
	if m.slots.SlotSelected(2) || m.cursorIdx != 1 || has() {
		t.Fatalf("now deselected, on ②: now selected=%v cursor %d tab=%v, want none", m.slots.SlotSelected(2), m.cursorIdx, has())
	}
}

// T3: the tab keys skip an absent Timeline tab, and a Timeline tab remembered
// from the floating chart attaches to the report on a chart that has none.
func TestAbsentTimelineTabIsSkippedAndFallsBack(t *testing.T) {
	stubTimeline(t, nil)
	m := ruleModel(t, ruleCase{selected: []int{0}, cursor: 1}) // ②: not in the group, no tab
	if got := tabIDs(m.panelTabs(PanelDetails)); !slices.Equal(got, []TabID{TabDetailsReport}) {
		t.Fatalf("fixture: tabs %v, want the report alone", got)
	}
	// `]`, `[` and `0` from the report with the tab absent stay on the report.
	for _, k := range []tea.KeyMsg{runes("]"), runes("["), runes("0")} {
		var cmd tea.Cmd
		m, cmd = tpress(t, m, k)
		m = pump(t, m, cmd)
		if got := m.activeTabID(PanelDetails); got != TabDetailsReport {
			t.Fatalf("after %q: tab %s, want the report", k.String(), got)
		}
	}
	// A Timeline chosen on the floating slot is remembered across the move to
	// a chart without one, and Enter there attaches to the report.
	m = m.withDetailsTab(PanelChart, TabDetailsTimeline)
	if m.tabChoice(PanelDetails) != TabDetailsTimeline {
		t.Fatal("fixture: the Timeline choice was not remembered")
	}
	if got := m.activeTabID(PanelDetails); got != TabDetailsReport {
		t.Fatalf("a remembered Timeline on a chart without one shows %s, want the report", got)
	}
	if got := m.attachLabel(); got != "Report" {
		t.Errorf("the Enter label is %q on a chart without a Timeline, want Report", got)
	}
	m, cmd := tpress(t, m, keyEnter)
	m = pump(t, m, cmd)
	if !m.details.attached || m.activeTabID(PanelDetails) != TabDetailsReport {
		t.Fatalf("Enter attached=%v on tab %s, want attached on the report", m.details.attached, m.activeTabID(PanelDetails))
	}
	// Leaving restores what Enter found, and moving to the floating slot makes
	// the remembered Timeline the active tab again.
	m, _ = tpress(t, m, keyEsc)
	m, c2 := tpress(t, m, keyDown)
	m = pump(t, m, c2)
	if !m.focusedIsFloating() || m.activeTabID(PanelDetails) != TabDetailsTimeline {
		t.Errorf("back on the floating slot: floating=%v tab=%s, want the remembered Timeline", m.focusedIsFloating(), m.activeTabID(PanelDetails))
	}
}
