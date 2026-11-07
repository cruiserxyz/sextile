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

// mainBindings is the main screen's table. Its order is the bar's order and
// the Keys popup's: the bar's entries first in the order they read, Help and
// Quit last and pinned, so a narrow bar keeps them and drops from the middle.
//
// A pair of keys with one meaning is one entry that carries the title and both
// keys (its spelling is the binding's own Help key in defaultKeys), followed by
// Folded entries for the other key(s): they match their key and show nowhere.
func mainBindings(k keys.KeyMap) []keys.BindingEntry {
	return keys.Titled([]keys.BindingEntry{
		{ID: keys.BindingNextView, Binding: k.NextView, Title: "Switch panel", BarKey: "←→", Bar: true, Help: true},
		// Enter opens Details: the title names the tab it will show (Timeline or
		// Report, by attachTab) and the bar offers it only while Details is on screen.
		{ID: keys.BindingAttach, Binding: k.Attach, Title: "Timeline", TitleFn: func(c keys.Context) string { return c.Attach.Title }, BarKey: "enter", Bar: true, Help: true, When: func(c keys.Context) bool { return c.Attach.CanAttach }, Palette: true},
		// Select, Step day and Move are shown only where they act: a key the
		// bar does not show does nothing and says nothing
		// (the specification Part B, § Bar conditions).
		{ID: keys.BindingSelect, Binding: k.Select, Title: "Select", BarKey: "space", Bar: true, Help: true, When: func(c keys.Context) bool { return c.Main.Selects }, Palette: true},
		{ID: keys.BindingTimeStepDay, Binding: k.TimeStepDay, Title: "Step a day", BarKey: "d/D", Bar: true, Help: true, When: func(c keys.Context) bool { return c.Main.HasWorkspace }},
		{ID: keys.BindingSlotCycleUp, Binding: k.SlotCycleUp, Title: "Move row", BarKey: "↑↓", Bar: true, Help: true, When: func(c keys.Context) bool { return c.Main.Moves }},
		// The list keys act on an anchored chart under the Chart panel's cursor,
		// and ⌃k / ⌃j only where it has a neighbour to swap with, so each is on
		// the bar exactly while it does something (D29). The Keys popup, the full
		// reference, lists them whenever a chart is open.
		{ID: keys.BindingSlotMoveUp, Binding: k.SlotMoveUp, Title: "Move chart up", BarKey: "⌃k", Bar: true, Help: true, When: func(c keys.Context) bool { return c.Main.CanMoveUp }, HelpWhen: func(c keys.Context) bool { return c.Main.HoldsAChart }, Palette: true},
		{ID: keys.BindingSlotMoveDown, Binding: k.SlotMoveDown, Title: "Move chart down", BarKey: "⌃j", Bar: true, Help: true, When: func(c keys.Context) bool { return c.Main.CanMoveDown }, HelpWhen: func(c keys.Context) bool { return c.Main.HoldsAChart }, Palette: true},
		{ID: keys.BindingSlotDelete, Binding: k.SlotDelete, Title: "Remove chart", BarKey: "del", Bar: true, Help: true, When: func(c keys.Context) bool { return c.Main.CanRemove }, HelpWhen: func(c keys.Context) bool { return c.Main.HoldsAChart }, Palette: true},
		{ID: keys.BindingFocusChart, Binding: k.FocusChart, Title: "Focus panel", BarKey: "1/2/3", Bar: true, Help: true},
		// Ctrl+PgUp/PgDn scroll the Details report from anywhere, so the bar names
		// them only while Details is on screen with more than it can show. Both
		// carry the condition: a key that cannot act does not match.
		{ID: keys.BindingScrollUp, Binding: k.ScrollUp, Title: "Scroll Details", BarKey: "⌃⇞⌃⇟", Bar: true, Help: true, When: func(c keys.Context) bool { return c.Main.DetailsScrolls }},
		{ID: keys.BindingSlotCycleDown, Binding: k.SlotCycleDown, Folded: true, BarKey: "↓"},
		{ID: keys.BindingPrevView, Binding: k.PrevView, Folded: true, BarKey: "←"},
		{ID: keys.BindingFocusBodies, Binding: k.FocusBodies, Folded: true, BarKey: "2"},
		{ID: keys.BindingFocusAspects, Binding: k.FocusAspects, Folded: true, BarKey: "3"},
		{ID: keys.BindingDetailsTab, Binding: k.DetailsTab, Title: "Next Details tab", BarKey: "0", Help: true},
		{ID: keys.BindingTabPrev, Binding: k.TabPrev, Title: "Switch tab", BarKey: "[", Help: true},
		{ID: keys.BindingTabNext, Binding: k.TabNext, Folded: true, BarKey: "]"},
		{ID: keys.BindingScrollDown, Binding: k.ScrollDown, Folded: true, BarKey: "⌃⇟", When: func(c keys.Context) bool { return c.Main.DetailsScrolls }},
		// The paging keys move the focused list's cursor (Chart, Bodies). Where the
		// focused panel owns no cursor, ⇞ ⇟ scroll Details as they always did, so
		// they carry no condition; Home and End have nothing to fall back to.
		{ID: keys.BindingPageUp, Binding: k.PageUp, Title: "Page list", BarKey: "⇞", Help: true},
		{ID: keys.BindingPageDown, Binding: k.PageDown, Folded: true, BarKey: "⇟"},
		{ID: keys.BindingPageHome, Binding: k.CursorHome, Title: "First row", BarKey: "home/g", Help: true, When: func(c keys.Context) bool { return c.Main.Pages }},
		{ID: keys.BindingPageEnd, Binding: k.CursorEnd, Title: "Last row", BarKey: "end/G", Help: true, When: func(c keys.Context) bool { return c.Main.Pages }},
		{ID: keys.BindingTimeStepYear, Binding: k.TimeStepYear, Title: "Step a year", BarKey: "y/Y", Help: true},
		{ID: keys.BindingTimeStepMonth, Binding: k.TimeStepMonth, Title: "Step a month", BarKey: "o/O", Help: true},
		{ID: keys.BindingTimeStepWeek, Binding: k.TimeStepWeek, Title: "Step a week", BarKey: "w/W", Help: true},
		{ID: keys.BindingTimeStepHour, Binding: k.TimeStepHour, Title: "Step an hour", BarKey: "h/H", Help: true},
		{ID: keys.BindingTimeStepMinute, Binding: k.TimeStepMinute, Title: "Step a minute", BarKey: "m/M", Help: true},
		{ID: keys.BindingJumpNow, Binding: k.JumpNow, Title: "Now", BarKey: "n", Help: true, Palette: true},
		{ID: keys.BindingToggleBack, Binding: k.ToggleBack, Title: "Back to the previous time", BarKey: "N", Help: true, Palette: true},
		{ID: keys.BindingOpenAtlas, Binding: k.OpenAtlas, Title: "Choose city", BarKey: "⌃l", Help: true, Palette: true},
		// ⌃p opens the Actions palette. Listed in Help and not on the bar, and not
		// one of the palette's own rows: it is the door to them.
		{ID: keys.BindingActions, Binding: k.Actions, Title: "Actions", BarKey: "⌃p", Help: true},
		{ID: keys.BindingOpenCharts, Binding: k.OpenCharts, Title: "Open chart", BarKey: "⌃o", Help: true, Palette: true},
		{ID: keys.BindingSaveChart, Binding: k.SaveChart, Title: "Save chart", BarKey: "⌃s", Help: true, Palette: true},
		{ID: keys.BindingBodies, Binding: k.Bodies, Title: "Choose bodies", BarKey: "⌃b", Help: true, Palette: true},
		{ID: keys.BindingHouses, Binding: k.Houses, Title: "Choose houses", BarKey: "⌃h", Help: true, Palette: true},
		{ID: keys.BindingZodiac, Binding: k.Zodiac, Title: "Choose zodiac", BarKey: "⌃z", Help: true, Palette: true},
		{ID: keys.BindingAspects, Binding: k.Aspects, Title: "Choose aspects", BarKey: "⌃a", Help: true, Palette: true},
		{ID: keys.BindingRenderWheel, Binding: k.RenderWheel, Title: "Wheel image", BarKey: "⌃x", Help: true, Palette: true},
		// Reached through the leader, never by its bare key, so it stays out
		// of the flat help list: the leader section prints it with the active
		// leader in front.
		{ID: keys.BindingLogConsole, Binding: k.LogConsole, Title: "Log", BarKey: "l", When: func(c keys.Context) bool { return c.Leader.Pending }, Palette: true},
		{ID: keys.BindingSaveCity, Binding: k.SaveCity, Title: "Save defaults", BarKey: "c", When: func(c keys.Context) bool { return c.Leader.Pending }, Palette: true},
		{ID: keys.BindingHouseDisplay, Binding: k.HouseDisplay, Title: "House display", BarKey: "a", When: func(c keys.Context) bool { return c.Leader.Pending }, Palette: true},
		// Esc while a leader is pending. Not in the bar and not in help: the
		// pending indicator is what tells the operator a leader is armed.
		{ID: keys.BindingCancelLeader, Binding: k.CancelLeader, Title: "Cancel the leader", BarKey: "esc", When: func(c keys.Context) bool { return c.Leader.Pending }},
		{ID: keys.BindingForceQuit, Binding: k.ForceQuit, Title: "Quit from anywhere", BarKey: "⌃c", Help: true},
		{ID: keys.BindingHelp, Binding: k.Help, Title: "Help", BarKey: "?", Bar: true, Pinned: true, Help: true, Palette: true},
		{ID: keys.BindingQuit, Binding: k.Quit, Title: "Quit", BarKey: "q", Bar: true, Pinned: true, Help: true, Palette: true},
	})
}

// leaderOnlyBindings are actions the leader alone reaches whose last key is a
// main-screen key: `<leader> o` opens the Sessions list, and bare o steps the
// month. They claim no key of the main table -- the leader resolver dispatches
// the sequence -- and the Keys popup's leader section reads their description
// here.
func leaderOnlyBindings() []keys.BindingEntry {
	return keys.Titled([]keys.BindingEntry{
		{ID: keys.BindingSessions, Binding: key.NewBinding(), Title: "Sessions", BarKey: "o", Palette: true},
		{ID: keys.BindingNewSession, Binding: key.NewBinding(), Title: "New session", BarKey: "n", Palette: true},
		// `<leader> y` copies the whole content of what has the keyboard. The
		// Log console is `<leader> l l`, shown so although the leader map holds
		// only `<leader> l` (the Log): the console is opened by `l` inside the
		// Log's focus.
		{ID: keys.BindingCopy, Binding: key.NewBinding(), Title: "Copy", BarKey: "y", Palette: true},
		{ID: keys.BindingConsole, Binding: key.NewBinding(), Title: "Log console", BarKey: "l", Palette: true, ShownSeq: []string{"l", "l"}},
	})
}

// detailsScrolls is where the page keys act: Details is on screen and its
// active tab holds more than it can show.
func (m Model) detailsScrolls() bool {
	if !m.detailsVisible() {
		return false
	}
	if m.activeTabID(PanelDetails) == TabDetailsTimeline {
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

// holdsAChart is when the Keys popup lists the list keys: at least one chart is
// open to remove or move. With none, the popup reads as it always has.
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

// The popups' shared bindings. A search line types every printable key, so a
// popup list moves with the arrows only; j and k are letters there.
func listUp() keys.BindingEntry {
	return keys.BindingEntry{ID: keys.BindingListUp, Binding: key.NewBinding(key.WithKeys("up")), Title: "Move row", HelpKey: "↑/↓", BarKey: "↑↓", Bar: true}
}

func listDown() keys.BindingEntry {
	return keys.BindingEntry{ID: keys.BindingListDown, Folded: true, Binding: key.NewBinding(key.WithKeys("down")), BarKey: "↓"}
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
	return keys.BindingEntry{ID: keys.BindingToggle, Binding: key.NewBinding(key.WithKeys(" ")), Title: "Toggle", BarKey: "space", Bar: true}
}

// pageEntries are the paging keys of an owner with a cursor over a list: ⇞ ⇟
// Home End. Nothing here types, so g and G are not among them in a popup with a
// filter box, where they are filter text. Their keys stay off the bar (it is
// full); the README is their reference.
func pageEntries() []keys.BindingEntry {
	return []keys.BindingEntry{
		{ID: keys.BindingPageUp, Title: "Page list", HelpKey: "⇞/⇟", Binding: key.NewBinding(key.WithKeys("pgup")), BarKey: "⇞"},
		{ID: keys.BindingPageDown, Folded: true, Binding: key.NewBinding(key.WithKeys("pgdown")), BarKey: "⇟"},
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
		return len(m.sessions.Rows) + 1
	case ownerBodies:
		return len(m.bodyModal.Matches)
	case ownerAspects:
		return len(m.aspectModal.Matches)
	case ownerHouses:
		return len(m.houseFinder.Matches)
	case ownerZodiac:
		return len(m.zodiacFinder.Matches)
	case ownerActions:
		return len(m.actions.Matches)
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
// It is not a keyMap field: the Keys popup lists main-screen keys only, and
// no attached-Details key appears there.
var timelineSearchKey = key.NewBinding(key.WithKeys("/"))

// detailsMoves is where ↑↓ act in attached Details: a timeline with more
// than one row, or a report with more than fits.
func (m Model) detailsMoves() bool {
	if m.activeTabID(PanelDetails) == TabDetailsTimeline {
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
		{ID: keys.BindingListUp, Binding: key.NewBinding(key.WithKeys("up", "k")), Title: "Move row", HelpKey: "↑/↓", BarKey: "↑↓", Bar: true, When: func(c keys.Context) bool { return c.Details.Moves }},
		{ID: keys.BindingListDown, Folded: true, Binding: key.NewBinding(key.WithKeys("down", "j")), BarKey: "↓"},
		{ID: keys.BindingConfirm, Binding: key.NewBinding(key.WithKeys("enter")), Title: "Jump", BarKey: "enter", Bar: true, When: func(c keys.Context) bool { return c.Details.OnTimeline }},
		// `,y` is the leader's (updateLeaderKey), so this entry matches no key
		// of its own; it is here for the bar, on both tabs.
		{ID: keys.BindingCopy, Binding: key.NewBinding(), Title: "Copy", BarKey: ", y", Bar: true},
		{ID: keys.BindingToggle, Binding: k.Select, Title: "Mark", BarKey: "space", Bar: true, When: func(c keys.Context) bool { return c.Details.OnTimelineRows }},
		{ID: keys.BindingSearch, Binding: timelineSearchKey, Title: "Filter", BarKey: "/", Bar: true, When: func(c keys.Context) bool { return c.Details.Filterable }},
		{ID: keys.BindingTabNext, Binding: k.TabNext, Title: "Switch tab", HelpKey: "[/]", BarKey: "[ ]", Bar: true, When: func(c keys.Context) bool { return c.Details.HasSeveralTabs }},
		{ID: keys.BindingTabPrev, Binding: k.TabPrev, Folded: true, BarKey: "["},
		{ID: keys.BindingDetailsTab, Binding: k.DetailsTab, BarKey: "0"},
		{ID: keys.BindingFocusChart, Binding: k.FocusChart, Title: "Focus", HelpKey: "1/2/3", BarKey: "1/2/3", Bar: true},
		{ID: keys.BindingFocusBodies, Binding: k.FocusBodies, Folded: true, BarKey: "2"},
		{ID: keys.BindingFocusAspects, Binding: k.FocusAspects, Folded: true, BarKey: "3"},
		{ID: keys.BindingScrollUp, Binding: k.ScrollUp, Title: "Scroll Details", HelpKey: "⌃⇞/⌃⇟", BarKey: "⌃⇞⌃⇟", Bar: true, When: func(c keys.Context) bool { return c.Details.Scrolls }},
		{ID: keys.BindingScrollDown, Binding: k.ScrollDown, Folded: true, BarKey: "⌃⇟", When: func(c keys.Context) bool { return c.Details.Scrolls }},
		// ⇞ ⇟ Home End move the timeline's row cursor (a tab with no row cursor
		// scrolls two lines on ⇞ ⇟, as it always did). Their keys are not in the
		// bar: it is full, and the README is their reference.
		{ID: keys.BindingPageUp, Title: "Page list", HelpKey: "⇞/⇟", Binding: k.PageUp, BarKey: "⇞"},
		{ID: keys.BindingPageDown, Folded: true, Binding: k.PageDown, BarKey: "⇟"},
		{ID: keys.BindingPageHome, Title: "First row", Binding: k.CursorHome, BarKey: "home/g", When: func(c keys.Context) bool { return c.Details.OnTimelineRows }},
		{ID: keys.BindingPageEnd, Title: "Last row", Binding: k.CursorEnd, BarKey: "end/G", When: func(c keys.Context) bool { return c.Details.OnTimelineRows }},
		{ID: keys.BindingTimeStepYear, Binding: k.TimeStepYear, Title: "Step", BarKey: "y o w d h m", Bar: true},
		{ID: keys.BindingTimeStepMonth, Binding: k.TimeStepMonth, BarKey: "o/O"},
		{ID: keys.BindingTimeStepWeek, Binding: k.TimeStepWeek, BarKey: "w/W"},
		{ID: keys.BindingTimeStepDay, Binding: k.TimeStepDay, BarKey: "d/D"},
		{ID: keys.BindingTimeStepHour, Binding: k.TimeStepHour, BarKey: "h/H"},
		{ID: keys.BindingTimeStepMinute, Binding: k.TimeStepMinute, BarKey: "m/M"},
		{ID: keys.BindingJumpNow, Binding: k.JumpNow, BarKey: "n"},
		{ID: keys.BindingToggleBack, Binding: k.ToggleBack, BarKey: "N"},
		// q and Left leave as Esc does; the shared closeOn stays Esc-only, so a
		// popup or form that types q is unaffected (time-keys-everywhere rule 4).
		{ID: keys.BindingClose, Binding: key.NewBinding(key.WithKeys("esc", "q", "left")), Title: "Close", BarKey: "esc/q", Bar: true, Pinned: true},
		// Every other main key — the popups, ⌃x, help, quit — is not listed:
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

func (m Model) detailsOnTimeline() bool { return m.activeTabID(PanelDetails) == TabDetailsTimeline }

// currentOwner names whatever has the keyboard. The key dispatch and the key
// bar both ask it, so the bar always shows the keys of the owner that will
// receive the next key.
func (m Model) currentOwner() owner {
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
	case startupDisplayOpen:
		return ownerDisplay
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
	case m.helpOpen:
		return ownerHelp
	case m.actionsOpen:
		return ownerActions
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
		return saveChartBindings()
	case ownerBodies:
		return bodiesBindings()
	case ownerAspects:
		return aspectsBindings()
	case ownerHouses:
		return housesBindings()
	case ownerZodiac:
		return zodiacBindings()
	case ownerDisplay:
		return displayBindings()
	case ownerExperiments:
		return experimentsBindings()
	case ownerSessions:
		return sessionsBindings()
	case ownerSessionName:
		return sessionNameBindings()
	case ownerHelp:
		return helpBindings()
	case ownerActions:
		return actionsBindings()
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
