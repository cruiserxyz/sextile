package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"strings"

	"github.com/charmbracelet/bubbles/key"

	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
)

// keys.go is the one place a key is named. Every owner of the keyboard — the
// main screen, the seven popups, the Log console and attached Details — has
// ONE table function here, and that table is both what its key handler matches
// against and what the key bar draws (the specification C1). A
// key that is not in its owner's table does nothing there, and the bar cannot
// advertise a key its owner does not handle.
//
// Typing into a search line or a form field (printable characters, Space where
// it types, Backspace) is text entry and lives with the field, not here.

// mainBindings is the main screen's table. Its order is the bar's order: the
// bar's entries first in the order they read, then Commands, Keybindings and
// Quit last and pinned, so a narrow bar keeps them and drops from the middle.
// Each entry declares its group in the Keybindings popup once, here.
//
// Each key that runs an action of its own is an entry of its own, with its own
// title; the bar names a pair under one word (BarTitle). Keys that run the same
// action are one entry.
func mainBindings(k keys.KeyMap) []keys.BindingEntry {
	return keys.Titled([]keys.BindingEntry{
		{ID: keys.BindingNextView, Group: keys.GroupNavigation, Binding: k.NextView, Title: "Next panel", BarKey: "right", Help: true},
		{ID: keys.BindingPrevView, Group: keys.GroupNavigation, Binding: k.PrevView, Title: "Previous panel", BarKey: "left", Help: true},
		// Enter opens Details: the title names the tab it will show (Timeline or
		// Report, by attachTab) and the bar offers it only while Details is on screen.
		{ID: keys.BindingAttach, Group: keys.GroupLocal, Binding: k.Attach, Title: "Timeline", TitleFn: func(c keys.Context) string { return c.Attach.Title }, BarKey: "enter", Bar: true, Help: true, When: func(c keys.Context) bool { return c.Attach.CanAttach }, Palette: true},
		// The Moon phase key comes after Enter, unpinned: it shows on a bar of usual
		// width and gives way to the pinned Commands, Keybindings and Quit when
		// the room is short.
		{ID: keys.BindingMoonPhase, Group: keys.GroupNavigation, Binding: k.MoonPhase, Title: "Next Moon phase", BarTitle: "Moon phase", BarKey: "p/P", Kept: true, Help: true, Bar: true},
		{ID: keys.BindingMoonPhasePrev, Group: keys.GroupNavigation, Binding: k.MoonPhasePrev, Title: "Previous Moon phase", BarKey: "P", Help: true},
		// Select, Step day and Move are shown only where they act: a key the
		// bar does not show does nothing and says nothing
		// (the specification Part B, § Bar conditions).
		{ID: keys.BindingSelect, Group: keys.GroupLocal, Binding: k.Select, Title: "Select", BarKey: "space", Bar: true, Help: true, When: func(c keys.Context) bool { return c.Main.Selects }, Palette: true},
		{ID: keys.BindingTimeStepDay, Group: keys.GroupNavigation, Binding: k.TimeStepDay, Title: "Step a day forward", BarTitle: "Step a day", BarKey: "d/D", Bar: true, Help: true, When: func(c keys.Context) bool { return c.Main.HasWorkspace }},
		{ID: keys.BindingTimeStepDayBack, Group: keys.GroupNavigation, Binding: k.TimeStepDayBack, Title: "Step a day back", BarKey: "D", Help: true, When: func(c keys.Context) bool { return c.Main.HasWorkspace }},
		{ID: keys.BindingSlotCycleUp, Group: keys.GroupNavigation, Binding: k.SlotCycleUp, Title: "Go to the chart above", BarKey: "up", Help: true, When: func(c keys.Context) bool { return c.Main.Moves }},
		{ID: keys.BindingSlotCycleDown, Group: keys.GroupNavigation, Binding: k.SlotCycleDown, Title: "Go to the chart below", BarKey: "down", Help: true},
		// The list keys act on an anchored chart under the Chart panel's cursor,
		// and K / J only where it has a neighbour to swap with, so each is on
		// the bar exactly while it does something (D29). The Keybindings popup
		// lists them while the Chart panel has the focus and a chart is open.
		{ID: keys.BindingSlotMoveUp, Group: keys.GroupLocal, Binding: k.SlotMoveUp, Title: "Move chart up", BarKey: "K", Bar: true, Help: true, When: func(c keys.Context) bool { return c.Main.CanMoveUp }, HelpWhen: func(c keys.Context) bool { return c.Main.HoldsAChart && c.Main.ChartFocused }, Palette: true},
		{ID: keys.BindingSlotMoveDown, Group: keys.GroupLocal, Binding: k.SlotMoveDown, Title: "Move chart down", BarKey: "J", Bar: true, Help: true, When: func(c keys.Context) bool { return c.Main.CanMoveDown }, HelpWhen: func(c keys.Context) bool { return c.Main.HoldsAChart && c.Main.ChartFocused }, Palette: true},
		{ID: keys.BindingSlotDelete, Group: keys.GroupLocal, Binding: k.SlotDelete, Title: "Remove chart", BarKey: "del", Bar: true, Help: true, When: func(c keys.Context) bool { return c.Main.CanRemove }, HelpWhen: func(c keys.Context) bool { return c.Main.HoldsAChart && c.Main.ChartFocused }, Palette: true},
		{ID: keys.BindingFocusSession, Group: keys.GroupNavigation, Binding: k.FocusSession, Title: "Focus Status", BarTitle: "Focus panel", BarKey: "1-4", Bar: true, Help: true},
		{ID: keys.BindingFocusChart, Group: keys.GroupNavigation, Binding: k.FocusChart, Title: "Focus Chart", BarKey: "2", Help: true},
		{ID: keys.BindingFocusBodies, Group: keys.GroupNavigation, Binding: k.FocusBodies, Title: "Focus Bodies", BarKey: "3", Help: true},
		{ID: keys.BindingFocusAspects, Group: keys.GroupNavigation, Binding: k.FocusAspects, Title: "Focus Aspects", BarKey: "4", Help: true},
		// Ctrl+PgUp/PgDn scroll the Details report from anywhere, so the bar names
		// them only while Details is on screen with more than it can show. Both
		// carry the condition: a key that cannot act does not match.
		{ID: keys.BindingScrollUp, Group: keys.GroupNavigation, Binding: k.ScrollUp, Title: "Scroll Details up", BarTitle: "Scroll Details", BarKey: "ctrl+pgup/pgdown", Help: true, When: func(c keys.Context) bool { return c.Main.DetailsScrolls }},
		{ID: keys.BindingScrollDown, Group: keys.GroupNavigation, Binding: k.ScrollDown, Title: "Scroll Details down", BarKey: "ctrl+pgdown", Help: true, When: func(c keys.Context) bool { return c.Main.DetailsScrolls }},
		{ID: keys.BindingDetailsTab, Group: keys.GroupNavigation, Binding: k.DetailsTab, Title: "Next Details tab", BarKey: "0", Help: true},
		{ID: keys.BindingTabPrev, Group: keys.GroupLocal, Binding: k.TabPrev, Title: "Previous tab", BarKey: "[", Help: true},
		{ID: keys.BindingTabNext, Group: keys.GroupLocal, Binding: k.TabNext, Title: "Next tab", BarKey: "]", Help: true},
		// The paging keys move the focused list's cursor (Chart, Bodies). Where the
		// focused panel owns no cursor, ⇞ ⇟ scroll Details as they always did, so
		// they carry no condition; Home and End have nothing to fall back to.
		{ID: keys.BindingPageUp, Group: keys.GroupNavigation, Binding: k.PageUp, Title: "Page list up", BarKey: "pgup", Help: true},
		{ID: keys.BindingPageDown, Group: keys.GroupNavigation, Binding: k.PageDown, Title: "Page list down", BarKey: "pgdown", Help: true},
		{ID: keys.BindingPageHome, Group: keys.GroupNavigation, Binding: k.CursorHome, Title: "First row", BarKey: "home/g", Help: true, When: func(c keys.Context) bool { return c.Main.Pages }},
		{ID: keys.BindingPageEnd, Group: keys.GroupNavigation, Binding: k.CursorEnd, Title: "Last row", BarKey: "end/G", Help: true, When: func(c keys.Context) bool { return c.Main.Pages }},
		{ID: keys.BindingTimeStepYear, Group: keys.GroupNavigation, Binding: k.TimeStepYear, Title: "Step a year forward", BarKey: "y/Y", Help: true},
		{ID: keys.BindingTimeStepYearBack, Group: keys.GroupNavigation, Binding: k.TimeStepYearBack, Title: "Step a year back", BarKey: "Y", Help: true},
		{ID: keys.BindingTimeStepMonth, Group: keys.GroupNavigation, Binding: k.TimeStepMonth, Title: "Step a month forward", BarKey: "o/O", Help: true},
		{ID: keys.BindingTimeStepMonthBack, Group: keys.GroupNavigation, Binding: k.TimeStepMonthBack, Title: "Step a month back", BarKey: "O", Help: true},
		{ID: keys.BindingTimeStepWeek, Group: keys.GroupNavigation, Binding: k.TimeStepWeek, Title: "Step a week forward", BarKey: "w/W", Help: true},
		{ID: keys.BindingTimeStepWeekBack, Group: keys.GroupNavigation, Binding: k.TimeStepWeekBack, Title: "Step a week back", BarKey: "W", Help: true},
		{ID: keys.BindingTimeStepHour, Group: keys.GroupNavigation, Binding: k.TimeStepHour, Title: "Step an hour forward", BarKey: "h/H", Help: true},
		{ID: keys.BindingTimeStepHourBack, Group: keys.GroupNavigation, Binding: k.TimeStepHourBack, Title: "Step an hour back", BarKey: "H", Help: true},
		{ID: keys.BindingTimeStepMinute, Group: keys.GroupNavigation, Binding: k.TimeStepMinute, Title: "Step a minute forward", BarKey: "m/M", Help: true},
		{ID: keys.BindingTimeStepMinuteBack, Group: keys.GroupNavigation, Binding: k.TimeStepMinuteBack, Title: "Step a minute back", BarKey: "M", Help: true},
		{ID: keys.BindingTimeStepSecond, Group: keys.GroupNavigation, Binding: k.TimeStepSecond, Title: "Step a second forward", BarKey: "s/S", Help: true},
		{ID: keys.BindingTimeStepSecondBack, Group: keys.GroupNavigation, Binding: k.TimeStepSecondBack, Title: "Step a second back", BarKey: "S", Help: true},
		{ID: keys.BindingJumpNow, Group: keys.GroupNavigation, Binding: k.JumpNow, Title: "Now", BarKey: "n", Help: true, Palette: true},
		{ID: keys.BindingOpenAtlas, Group: keys.GroupGlobal, Binding: k.OpenAtlas, Title: "Choose city", BarKey: "ctrl+l", Help: true, Palette: true, Opens: true},
		// ⌃y copies what `, y` copies.
		{ID: keys.BindingCopy, Group: keys.GroupGlobal, Binding: k.Copy, Title: "Copy", BarKey: "ctrl+y", Help: true, Palette: true},
		{ID: keys.BindingOriginals, Group: keys.GroupGlobal, Binding: key.NewBinding(), Title: "Protect the charts folder, or not", Help: true, Palette: true},
		{ID: keys.BindingOpenCharts, Group: keys.GroupGlobal, Binding: k.OpenCharts, Title: "Open chart", BarKey: "ctrl+o", Help: true, Palette: true, Opens: true},
		{ID: keys.BindingSaveChart, Group: keys.GroupGlobal, Binding: k.SaveChart, Title: "Save chart", BarKey: "ctrl+s", Help: true, Palette: true, Opens: true},
		{ID: keys.BindingBodies, Group: keys.GroupGlobal, Binding: k.Bodies, Title: "Choose bodies", BarKey: "ctrl+b", Help: true, Palette: true, Opens: true},
		{ID: keys.BindingHouses, Group: keys.GroupGlobal, Binding: k.Houses, Title: "Choose houses", Help: true, Palette: true, Opens: true},
		{ID: keys.BindingZodiac, Group: keys.GroupGlobal, Binding: k.Zodiac, Title: "Choose zodiac", BarKey: "ctrl+z", Help: true, Palette: true, Opens: true},
		{ID: keys.BindingNodes, Group: keys.GroupGlobal, Binding: k.Nodes, Title: "Nodes and Lilith", BarKey: "ctrl+t", Help: true, Palette: true, Opens: true},
		{ID: keys.BindingAspects, Group: keys.GroupGlobal, Binding: k.Aspects, Title: "Choose aspects", BarKey: "ctrl+a", Help: true, Palette: true, Opens: true},
		{ID: keys.BindingRenderWheel, Group: keys.GroupGlobal, Binding: k.RenderWheel, Title: "Wheel image", BarKey: "ctrl+x", Help: true, Palette: true},
		// The leader alone reaches these: the key column prints the sequence
		// from the leader map, and none holds a key of the main table.
		{ID: keys.BindingLogConsole, Group: keys.GroupGlobal, Binding: key.NewBinding(), Title: "Log", Help: true, Palette: true},
		{ID: keys.BindingSaveCity, Group: keys.GroupGlobal, Binding: key.NewBinding(), Title: "Keep as defaults", Help: true, Palette: true},
		// Esc while a leader is pending. Not in the bar and not in the Keybindings popup: the
		// pending indicator is what tells the operator a leader is armed.
		{ID: keys.BindingCancelLeader, Group: keys.GroupGlobal, Binding: k.CancelLeader, Title: "Leave the command list", BarKey: "esc", When: func(c keys.Context) bool { return c.Leader.Pending }},
		{ID: keys.BindingForceQuit, Group: keys.GroupGlobal, Binding: k.ForceQuit, Title: "Quit from anywhere", BarKey: "ctrl+c", Help: true},
		// The comma is the key bar's word for the commands: it shows the leader
		// as configured, and not at all with no leader. ⌃p opens the same popup.
		{ID: keys.BindingActions, Group: keys.GroupGlobal, Binding: k.Actions, Title: "Commands", BarKey: ",", BarKeyFn: func(c keys.Context) string { return c.Leader.Key }, BarWhen: func(c keys.Context) bool { return c.Leader.Key != "" }, Bar: true, Pinned: true, Help: true, Opens: true},
		{ID: keys.BindingHelp, Group: keys.GroupGlobal, Binding: k.Help, Title: "Keybindings", BarKey: "?", Bar: true, Pinned: true, Help: true, Palette: true, Opens: true},
		{ID: keys.BindingQuit, Group: keys.GroupGlobal, Binding: k.Quit, Title: "Quit", BarKey: "q", Bar: true, Pinned: true, Help: true, Palette: true},
	})
}

// leaderOnlyBindings are actions the leader alone reaches whose last key is a
// main-screen key: `<leader> O` opens the Sessions list, and bare o steps the
// month. They claim no key of the main table -- the leader resolver dispatches
// the sequence -- and the Keybindings popup reads their titles here.
func leaderOnlyBindings() []keys.BindingEntry {
	return keys.Titled([]keys.BindingEntry{
		{ID: keys.BindingSessions, Group: keys.GroupGlobal, Binding: key.NewBinding(), Title: "Sessions", BarKey: "o", Help: true, Palette: true, Opens: true},
		// The Log console is opened by `l` inside the Log's focus.
		{ID: keys.BindingConsole, Group: keys.GroupGlobal, Binding: key.NewBinding(), Title: "Log console", BarKey: "l", Palette: true, Opens: true},
	})
}

// detailsScrolls is where the page keys act: Details is on screen and its
// active tab holds more than it can show.
func (m Model) detailsScrolls() bool {
	if !m.detailsVisible() {
		return false
	}
	if isListTab(m.activeTabID(PanelDetails)) {
		return len(m.timelineShown()) > m.timelineHeight()
	}
	total, height := m.detailsScrollSpan()
	return total > height
}

// mainPages is where the paging keys act on the main screen: the focused panel
// owns a cursor with at least one row to put it on (Chart or Bodies). It is not
// mainMoves: a one-row list owns the key and the press is a no-op there, instead
// of falling through to Details. A cursorless panel (Aspects) or an empty
// Chart or Bodies lets ⇞ ⇟ scroll Details exactly as before.
func (m Model) mainPages() bool {
	switch m.focused {
	case PanelChart:
		return m.slots != nil && len(m.slots.All()) >= 1
	case PanelBodies:
		rows := m.shownChart()
		return panel.BodyRowCount(rows.positions, rows.cusps) >= 1
	}
	return false
}

// hasWorkspace is whether there is a workspace to step: before the first
// location resolves a step key has no floating slot to move, so the bar does
// not offer it (the only condition left on the step keys, D37).
func (m Model) hasWorkspace() bool { return m.slots != nil }

// mainSelects is where Space acts: on the Chart's slots (while fewer than six
// are selected, or the slot is one of them), or on the Bodies row under the
// cursor, once there is a workspace.
func (m Model) mainSelects() bool {
	if m.slots == nil {
		return false
	}
	switch m.focused {
	case PanelChart:
		// A seventh selection is refused, so Space is not offered there (M7);
		// on a selected slot it still deselects.
		// With only the floating slot open there is nothing to compare, so
		// nothing is offered or selected.
		return m.slots.AnchoredCount() > 0 && m.slots.CanSelect(m.cursorIdx)
	case PanelBodies:
		_, ok := panel.ObjectAtCursor(m.chart.positions, m.chart.cusps, m.bodyCursorIdx)
		return ok
	}
	return false
}

// holdsAChart is when the Keybindings popup lists the chart list's keys: at
// least one chart is open to remove or move.
func (m Model) holdsAChart() bool {
	return m.slots != nil && m.slots.AnchoredCount() > 0
}

// mainMoves is where ↑↓ act: a Chart or Bodies list with somewhere to go.
func (m Model) mainMoves() bool {
	switch m.focused {
	case PanelChart:
		return m.slots != nil && len(m.slots.All()) > 1
	case PanelBodies:
		return panel.BodyRowCount(m.chart.positions, m.chart.cusps) > 1
	}
	return false
}

// chordKey is how a bar spells a chord of the leader: the configured leader key,
// then the letter (`, y`, or `x y` for a user whose leader is x).
func chordKey(letter string) func(keys.Context) string {
	return func(c keys.Context) string { return c.Leader.Key + " " + letter }
}

// leaderSet is whether a leader key is configured: a chord is shown only then.
func leaderSet(c keys.Context) bool { return c.Leader.Key != "" }

// The popups' shared bindings. A search line types every printable key, so a
// popup list moves with the arrows only; j and k are letters there.
func listUp() keys.BindingEntry {
	return keys.BindingEntry{ID: keys.BindingListUp, Binding: key.NewBinding(key.WithKeys("up")), Title: "Move row up", BarKey: "up"}
}

func listDown() keys.BindingEntry {
	return keys.BindingEntry{ID: keys.BindingListDown, Binding: key.NewBinding(key.WithKeys("down")), Title: "Move row down", BarKey: "down"}
}

func confirm(title string) keys.BindingEntry {
	return keys.BindingEntry{ID: keys.BindingConfirm, Binding: key.NewBinding(key.WithKeys("enter")), Title: title, BarKey: "enter", Bar: true}
}

func closeOn(title string) keys.BindingEntry {
	return keys.BindingEntry{ID: keys.BindingClose, Binding: key.NewBinding(key.WithKeys("esc")), Title: title, BarKey: "esc", Bar: true, Pinned: true}
}

// closeOnEscOrQ is closeOn for an owner with nothing to type into, where q
// leaves as Esc does.
func closeOnEscOrQ(title string) keys.BindingEntry {
	return keys.BindingEntry{ID: keys.BindingClose, Binding: key.NewBinding(key.WithKeys("esc", "q")), Title: title, BarKey: "esc/q", Bar: true, Pinned: true}
}

func toggle() keys.BindingEntry {
	return keys.BindingEntry{ID: keys.BindingToggle, Binding: key.NewBinding(key.WithKeys(" ")), Title: "Tick", BarKey: "space", Bar: true}
}

// pageEntries are the paging keys of an owner with a cursor over a list: ⇞ ⇟
// Home End. Nothing here types, so g and G are not among them in a popup with a
// filter box, where they are filter text. Their keys stay off the bar (it is
// full); docs/REFERENCE.md is their reference.
func pageEntries() []keys.BindingEntry {
	return []keys.BindingEntry{
		{ID: keys.BindingPageUp, Title: "Page list up", Binding: key.NewBinding(key.WithKeys("pgup")), BarKey: "pgup"},
		{ID: keys.BindingPageDown, Title: "Page list down", Binding: key.NewBinding(key.WithKeys("pgdown")), BarKey: "pgdown"},
		{ID: keys.BindingPageHome, Title: "First row", Binding: key.NewBinding(key.WithKeys("home")), BarKey: "home"},
		{ID: keys.BindingPageEnd, Title: "Last row", Binding: key.NewBinding(key.WithKeys("end")), BarKey: "end"},
	}
}

// popupRows is how many rows the open popup's list holds right now.
func (m Model) popupRows() int {
	switch m.currentOwner() {
	case ownerCity:
		if m.atlas.LoadErr != nil || m.atlas.Index == nil {
			return 0
		}
		return len(m.atlas.Matches)
	case ownerOpenChart:
		return len(m.picker.Matches)
	case ownerSessions:
		return len(m.sessions.Rows) + 2
	case ownerBodies:
		return len(m.bodyModal.Matches) + m.bodyModal.LeadRows()
	case ownerAspects:
		return len(m.aspectModal.Matches)
	case ownerHouses:
		return len(m.houseFinder.Matches) + m.houseFinder.Shown.Count()
	case ownerZodiac:
		return len(m.zodiacFinder.Matches)
	case ownerNodes:
		return len(m.nodesFinder.Matches)
	case ownerKeybindings:
		return len(m.keybindings.Matches)
	}
	return 0
}

func (m Model) popupHasRows() bool { return m.popupRows() > 0 }
func (m Model) popupMoves() bool   { return m.popupRows() > 1 }

// pickerRenames is where Ctrl+R acts: on a chart-file row under the cursor.
func (m Model) pickerRenames() bool {
	if m.picker.loadErr != nil || m.picker.Cursor < 0 || m.picker.Cursor >= len(m.picker.Matches) {
		return false
	}
	return m.picker.Matches[m.picker.Cursor].Source.Kind == store.ChartKindFile
}

// saveHasName is where Enter saves: a name typed and no save in flight.
func (m Model) saveHasName() bool {
	return !m.save.Saving && strings.TrimSpace(m.save.typedName()) != ""
}

// logMoves and logHasLines are where the focused Log's keys act.
func (m Model) logMoves() bool    { return len(m.logPanelLines()) > 1 }
func (m Model) logHasLines() bool { return len(m.logPanelLines()) > 0 }

// consoleMoves and consoleHasLines are where the console's keys act.
func (m Model) consoleMoves() bool    { return len(m.console.snapshot) > 1 }
func (m Model) consoleHasLines() bool { return len(m.console.snapshot) > 0 }

// timelineSearchKey is `/`, the timeline's search line (the specification).
// It is not a keys.KeyMap field: only attached Details answers it.
var timelineSearchKey = key.NewBinding(key.WithKeys("/"))

// detailsMoves is where ↑↓ act in attached Details: a timeline with more
// than one row, or a report with more than fits.
func (m Model) detailsMoves() bool {
	if isListTab(m.activeTabID(PanelDetails)) {
		return len(m.timelineShown()) > 1
	}
	return m.detailsScrolls()
}

// detailsBindings is attached Details' table: the keys Details answers itself,
// ahead of the main screen's. ↑↓ move the row cursor on the
// timeline tab and scroll the report a line on the report tab (timeline C26).
// The timeline's own keys — the jump, the row marks, the filter — exist on the
// timeline tab only; the step keys and now/back work on both tabs (D37).
func detailsBindings(k keys.KeyMap) []keys.BindingEntry {
	return keys.Titled([]keys.BindingEntry{
		{ID: keys.BindingListUp, Group: keys.GroupLocal, Binding: key.NewBinding(key.WithKeys("up", "k")), Title: "Move row up", BarKey: "up", When: func(c keys.Context) bool { return c.Details.Moves }},
		{ID: keys.BindingListDown, Group: keys.GroupLocal, Binding: key.NewBinding(key.WithKeys("down", "j")), Title: "Move row down", BarKey: "down"},
		{ID: keys.BindingConfirm, Group: keys.GroupLocal, Binding: key.NewBinding(key.WithKeys("enter")), Title: "Jump", BarKey: "enter", Bar: true, When: func(c keys.Context) bool { return c.Details.OnTimeline }},
		{ID: keys.BindingMoonPhase, Group: keys.GroupNavigation, Binding: k.MoonPhase, Title: "Next Moon phase", BarTitle: "Moon phase", BarKey: "p/P", Kept: true, Bar: true},
		{ID: keys.BindingMoonPhasePrev, Group: keys.GroupNavigation, Binding: k.MoonPhasePrev, Title: "Previous Moon phase", BarKey: "P"},
		// `, y` is the leader's (updateLeaderKey) and ⌃y falls through to the main
		// table, so this entry matches no key of its own; it is here for the bar,
		// on both tabs.
		{ID: keys.BindingCopy, Group: keys.GroupGlobal, Binding: key.NewBinding(), Title: "Copy", BarKeyFn: chordKey("y"), BarKey: ", y", BarWhen: leaderSet, Bar: true},
		{ID: keys.BindingToggle, Group: keys.GroupLocal, Binding: k.Select, Title: "Mark", BarKey: "space", Bar: true, When: func(c keys.Context) bool { return c.Details.OnTimelineRows }},
		{ID: keys.BindingSearch, Group: keys.GroupLocal, Binding: timelineSearchKey, Title: "Filter", BarKey: "/", Bar: true, When: func(c keys.Context) bool { return c.Details.Filterable }},
		{ID: keys.BindingTabNext, Group: keys.GroupLocal, Binding: k.TabNext, Title: "Next tab", BarTitle: "Switch tab", BarKey: "[ ]", Bar: true, When: func(c keys.Context) bool { return c.Details.HasSeveralTabs }},
		{ID: keys.BindingTabPrev, Group: keys.GroupLocal, Binding: k.TabPrev, Title: "Previous tab", BarKey: "["},
		{ID: keys.BindingDetailsTab, Group: keys.GroupNavigation, Binding: k.DetailsTab, BarKey: "0"},
		{ID: keys.BindingFocusSession, Group: keys.GroupNavigation, Binding: k.FocusSession, Title: "Focus Status", BarTitle: "Focus", BarKey: "1-4", Bar: true},
		{ID: keys.BindingFocusChart, Group: keys.GroupNavigation, Binding: k.FocusChart, Title: "Focus Chart", BarKey: "2"},
		{ID: keys.BindingFocusBodies, Group: keys.GroupNavigation, Binding: k.FocusBodies, Title: "Focus Bodies", BarKey: "3"},
		{ID: keys.BindingFocusAspects, Group: keys.GroupNavigation, Binding: k.FocusAspects, Title: "Focus Aspects", BarKey: "4"},
		{ID: keys.BindingScrollUp, Group: keys.GroupNavigation, Binding: k.ScrollUp, Title: "Scroll Details up", BarTitle: "Scroll Details", BarKey: "ctrl+pgup/pgdown", When: func(c keys.Context) bool { return c.Details.Scrolls }},
		{ID: keys.BindingScrollDown, Group: keys.GroupNavigation, Binding: k.ScrollDown, Title: "Scroll Details down", BarKey: "ctrl+pgdown", When: func(c keys.Context) bool { return c.Details.Scrolls }},
		// ⇞ ⇟ Home End move the timeline's row cursor (a tab with no row cursor
		// scrolls two lines on ⇞ ⇟, as it always did). Their keys are not in the
		// bar: it is full, and docs/REFERENCE.md is their reference.
		{ID: keys.BindingPageUp, Group: keys.GroupNavigation, Title: "Page list up", Binding: k.PageUp, BarKey: "pgup"},
		{ID: keys.BindingPageDown, Group: keys.GroupNavigation, Title: "Page list down", Binding: k.PageDown, BarKey: "pgdown"},
		{ID: keys.BindingPageHome, Group: keys.GroupNavigation, Title: "First row", Binding: k.CursorHome, BarKey: "home/g", When: func(c keys.Context) bool { return c.Details.OnTimelineRows }},
		{ID: keys.BindingPageEnd, Group: keys.GroupNavigation, Title: "Last row", Binding: k.CursorEnd, BarKey: "end/G", When: func(c keys.Context) bool { return c.Details.OnTimelineRows }},
		{ID: keys.BindingTimeStepYear, Group: keys.GroupNavigation, Binding: k.TimeStepYear, Title: "Step a year forward", BarTitle: "Step", BarKey: "y o w d h m s", Bar: true},
		{ID: keys.BindingTimeStepYearBack, Group: keys.GroupNavigation, Binding: k.TimeStepYearBack, Title: "Step a year back", BarKey: "Y"},
		{ID: keys.BindingTimeStepMonth, Group: keys.GroupNavigation, Binding: k.TimeStepMonth, Title: "Step a month forward", BarKey: "o"},
		{ID: keys.BindingTimeStepMonthBack, Group: keys.GroupNavigation, Binding: k.TimeStepMonthBack, Title: "Step a month back", BarKey: "O"},
		{ID: keys.BindingTimeStepWeek, Group: keys.GroupNavigation, Binding: k.TimeStepWeek, Title: "Step a week forward", BarKey: "w"},
		{ID: keys.BindingTimeStepWeekBack, Group: keys.GroupNavigation, Binding: k.TimeStepWeekBack, Title: "Step a week back", BarKey: "W"},
		{ID: keys.BindingTimeStepDay, Group: keys.GroupNavigation, Binding: k.TimeStepDay, Title: "Step a day forward", BarKey: "d"},
		{ID: keys.BindingTimeStepDayBack, Group: keys.GroupNavigation, Binding: k.TimeStepDayBack, Title: "Step a day back", BarKey: "D"},
		{ID: keys.BindingTimeStepHour, Group: keys.GroupNavigation, Binding: k.TimeStepHour, Title: "Step an hour forward", BarKey: "h"},
		{ID: keys.BindingTimeStepHourBack, Group: keys.GroupNavigation, Binding: k.TimeStepHourBack, Title: "Step an hour back", BarKey: "H"},
		{ID: keys.BindingTimeStepMinute, Group: keys.GroupNavigation, Binding: k.TimeStepMinute, Title: "Step a minute forward", BarKey: "m"},
		{ID: keys.BindingTimeStepMinuteBack, Group: keys.GroupNavigation, Binding: k.TimeStepMinuteBack, Title: "Step a minute back", BarKey: "M"},
		{ID: keys.BindingTimeStepSecond, Group: keys.GroupNavigation, Binding: k.TimeStepSecond, Title: "Step a second forward", BarKey: "s"},
		{ID: keys.BindingTimeStepSecondBack, Group: keys.GroupNavigation, Binding: k.TimeStepSecondBack, Title: "Step a second back", BarKey: "S"},
		{ID: keys.BindingJumpNow, Group: keys.GroupNavigation, Binding: k.JumpNow, BarKey: "n"},
		// q and Left leave as Esc does; the shared closeOn stays Esc-only, so a
		// popup or form that types q is unaffected (time-keys-everywhere rule 4).
		// The number keys, Tab and Shift+Tab move the focus and Details follows it.
		{ID: keys.BindingClose, Group: keys.GroupLocal, Binding: key.NewBinding(key.WithKeys("esc", "q", "left")), HelpKey: keys.Spell("esc", "left") + "/q", Title: "Back", TitleFn: func(c keys.Context) string { return "Back to " + c.Details.Back }, BarKey: "esc", Bar: true, Pinned: true},
		// Right does nothing in Details: Left leaves it, and the panels the arrows
		// would switch are not what the operator is reading.
		{ID: keys.BindingNextView, Group: keys.GroupNavigation, Binding: key.NewBinding(key.WithKeys("right")), BarKey: "right"},
		// Every other main key — the popups, ⌃x, Keybindings, quit — is not listed:
		// it falls through to the main screen's dispatch unless detailsRefused
		// names it (passDetailsKeyToMain).
	})
}

func (m Model) detailsOnTimelineRows() bool {
	return m.detailsOnTimeline() && len(m.timelineShown()) > 0
}

// detailsHasSeveralTabs is whether `[ ]` have anything to switch between: only
// the Chart panel's Details has a second tab (the specification).
func (m Model) detailsHasSeveralTabs() bool { return len(m.panelTabs(PanelDetails)) > 1 }

func (m Model) detailsOnTimeline() bool { return isListTab(m.activeTabID(PanelDetails)) }

// currentOwner names whatever has the keyboard. The key dispatch and the key
// bar both ask it, so the bar always shows the keys of the owner that will
// receive the next key.
func (m Model) currentOwner() owner {
	if m.keybindingsOpen {
		return ownerKeybindings
	}
	switch m.startup {
	case startupAtlasOpen:
		return ownerCity
	case startupChartOpen:
		return ownerOpenChart
	case startupBodyOpen:
		return ownerBodies
	case startupAspectOpen:
		return ownerAspects
	case startupHouseOpen:
		return ownerHouses
	case startupZodiacOpen:
		return ownerZodiac
	case startupNodesOpen:
		return ownerNodes
	case startupExperimentsOpen:
		return ownerExperiments
	case startupSessionsOpen:
		return ownerSessions
	case startupSessionNameOpen:
		return ownerSessionName
	case startupSaveOpen:
		return ownerSaveChart
	case startupTitleOpen:
		return ownerChartTitle
	}
	switch {
	case m.console.open:
		return ownerConsole
	case m.logFocus.active:
		return ownerLog
	case m.details.attached:
		return ownerDetails
	}
	return ownerMain
}

// ownerBindings is owner o's table.
func (m Model) ownerBindings(o owner) []keys.BindingEntry {
	switch o {
	case ownerCity:
		return cityBindings()
	case ownerOpenChart:
		return openChartBindings()
	case ownerChartTitle:
		if m.variant(o) == keys.VariantTitleConfirm {
			return titleConfirmBindings()
		}
		return chartTitleBindings()
	case ownerSaveChart:
		if m.variant(o) == keys.VariantSaveAsk {
			return saveAskBindings()
		}
		return saveChartBindings()
	case ownerBodies:
		return bodiesBindings()
	case ownerAspects:
		return aspectsBindings()
	case ownerHouses:
		return housesBindings()
	case ownerZodiac:
		return zodiacBindings()
	case ownerNodes:
		return nodesBindings()
	case ownerExperiments:
		return experimentsBindings()
	case ownerSessions:
		if m.variant(o) == keys.VariantNewSessionAsk {
			return m.sessionAskBindings()
		}
		return sessionsBindings()
	case ownerSessionName:
		return sessionNameBindings()
	case ownerKeybindings:
		return keybindingsBindings()
	case ownerConsole:
		return consoleBindings()
	case ownerLog:
		return logBindings()
	case ownerDetails:
		switch m.variant(o) {
		case keys.VariantTimelineSearch:
			return timelineSearchBindings()
		case keys.VariantReportSearch:
			return reportSearchBindings()
		}
		return detailsBindings(m.keys)
	}
	return mainBindings(m.keys)
}

// strandingKeys are the keys a leader may never take. `q` is the only quit
// from the main screen; esc, enter and tab are the ways out of a popup or the
// console. A leader on any of them would leave the operator with no exit.
func strandingKeys() []string { return []string{"q", "esc", "enter", "tab"} }

// keyRegistry is every key in use, read from the tables the owners already
// declare: one table per owner and variant, and the command map.
func keyRegistry(k keys.KeyMap) keys.Registry {
	return keys.Registry{
		Owners: map[keys.Owner][][]keys.BindingEntry{
			keys.OwnerMain:        {mainBindings(k), leaderOnlyBindings()},
			keys.OwnerCity:        {cityBindings()},
			keys.OwnerOpenChart:   {openChartBindings()},
			keys.OwnerChartTitle:  {chartTitleBindings(), titleConfirmBindings()},
			keys.OwnerSaveChart:   {saveChartBindings(), saveAskBindings()},
			keys.OwnerBodies:      {bodiesBindings()},
			keys.OwnerAspects:     {aspectsBindings()},
			keys.OwnerHouses:      {housesBindings()},
			keys.OwnerZodiac:      {zodiacBindings()},
			keys.OwnerNodes:       {nodesBindings()},
			keys.OwnerExperiments: {experimentsBindings()},
			keys.OwnerSessions:    {sessionsBindings(), newSessionAskBindings(), deleteAskBindings()},
			keys.OwnerSessionName: {sessionNameBindings()},
			keys.OwnerKeybindings: {keybindingsBindings()},
			keys.OwnerConsole:     {consoleBindings()},
			keys.OwnerLog:         {logBindings()},
			keys.OwnerDetails:     {detailsBindings(k), timelineSearchBindings(), reportSearchBindings()},
		},
		Commands: keys.Commands(),
	}
}
