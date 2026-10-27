package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/store"
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

type BindingID int

const (
	BindingQuit BindingID = iota
	BindingNextView
	BindingPrevView
	BindingFocusChart
	BindingFocusBodies
	BindingFocusAspects
	BindingDetailsTab
	BindingHelp
	BindingOpenAtlas
	BindingOpenCharts
	BindingSaveChart
	BindingSelect
	BindingSlotCycleUp
	BindingSlotCycleDown
	// The workspace list's own keys (the specification): ⌃k / ⌃j move the
	// focused chart up and down, del takes it out of the workspace.
	BindingSlotMoveUp
	BindingSlotMoveDown
	BindingSlotDelete
	BindingTimeStepYear
	BindingTimeStepMonth
	BindingTimeStepWeek
	BindingTimeStepDay
	BindingTimeStepHour
	BindingTimeStepMinute
	BindingRenderWheel
	BindingJumpNow
	BindingSearch
	BindingToggleBack
	BindingScrollUp
	BindingScrollDown
	BindingLogConsole
	BindingAspects
	BindingBodies
	BindingHouses
	BindingZodiac
	// BindingSaveCity is `<leader> c d`: keep the current city as the default.
	BindingSaveCity
	BindingAttach
	BindingTabPrev
	BindingTabNext
	// BindingForceQuit is Ctrl+C. Update matches it once, before any owner
	// sees the key, so it quits from everywhere (C10).
	BindingForceQuit
	// BindingCancelLeader is Esc while a leader is pending. A pending leader
	// is a state of the main screen, so its Esc is a main-table entry.
	BindingCancelLeader

	// The popups', the console's and attached Details' own actions. One id
	// names one action; each owner's table gives it that owner's words.
	BindingListUp
	BindingListDown
	BindingConfirm
	BindingClose
	BindingToggle
	BindingNextField
	BindingPrevField
	BindingCopy
	// BindingRename is Ctrl+R in the chart picker: retitle the highlighted
	// chart (the specification C4).
	BindingRename

	// The paging keys (the page-cursor specification): ⇞ ⇟ and Home End move the
	// cursor of the owner's list by the one rule in cursor_page.go. g and G are
	// Home and End where the letter is not typed as text. They are appended so
	// the ids above keep their numbers.
	BindingPageUp
	BindingPageDown
	BindingPageHome
	BindingPageEnd

	// BindingHouseDisplay is `<leader> a`: the House display popup.
	BindingHouseDisplay

	// BindingSessions is `<leader> o`: the Sessions list. BindingSessionDelete
	// is Ctrl+D there: move the highlighted session to the trash.
	BindingSessions
	BindingSessionDelete
)

// BindingNone is what matchBinding returns for a key no entry matches.
const BindingNone BindingID = -1

// BindingEntry is one row of an owner's table.
type BindingEntry struct {
	ID BindingID
	// Binding holds the keys the entry matches. Its Help() is the Keys
	// popup's spelling of the key (`tab/→`) and the description it lists.
	Binding key.Binding
	// Action is the bar's word for it (`Next panel`); BarKey is the key as
	// the bar spells it (`→`). No Log line names a key: the bar is the one
	// place a key is explained (the specification Part B).
	Action string
	BarKey string
	// Bar puts the entry in the key bar, in table order. Pinned keeps it
	// there when the bar is too narrow for every entry (C4).
	Bar    bool
	Pinned bool
	// Help lists the entry in the Keys popup.
	Help bool
	// When, if set, is the condition under which the entry is available.
	// An unavailable entry neither matches nor appears in the bar.
	When func(Model) bool
	// HelpWhen, if set, is the condition under which the Keys popup lists the
	// entry. It is separate from When, which is whether the key acts: the popup
	// is the full reference for the screen and lists a key while it cannot act
	// (Select is there while nothing can be selected), and only the list keys
	// wait for a chart to be open.
	HelpWhen func(Model) bool
}

type keyMap struct {
	Quit           key.Binding
	ForceQuit      key.Binding
	CancelLeader   key.Binding
	NextView       key.Binding
	PrevView       key.Binding
	FocusChart     key.Binding
	FocusBodies    key.Binding
	FocusAspects   key.Binding
	DetailsTab     key.Binding
	Help           key.Binding
	OpenAtlas      key.Binding
	OpenCharts     key.Binding
	SaveChart      key.Binding
	Select         key.Binding
	SlotCycleUp    key.Binding
	SlotCycleDown  key.Binding
	SlotMoveUp     key.Binding
	SlotMoveDown   key.Binding
	SlotDelete     key.Binding
	TimeStepYear   key.Binding
	TimeStepMonth  key.Binding
	TimeStepWeek   key.Binding
	TimeStepDay    key.Binding
	TimeStepHour   key.Binding
	TimeStepMinute key.Binding
	RenderWheel    key.Binding
	JumpNow        key.Binding
	ToggleBack     key.Binding
	ScrollUp       key.Binding
	ScrollDown     key.Binding
	PageUp         key.Binding
	PageDown       key.Binding
	CursorHome     key.Binding
	CursorEnd      key.Binding
	LogConsole     key.Binding
	Aspects        key.Binding
	Bodies         key.Binding
	Houses         key.Binding
	Zodiac         key.Binding
	SaveCity       key.Binding
	HouseDisplay   key.Binding
	Attach         key.Binding
	TabPrev        key.Binding
	TabNext        key.Binding
}

// mainBindings is the main screen's table. Its order is the bar's order and
// the Keys popup's: the bar's entries first in the order they read, Help and
// Quit last and pinned, so a narrow bar keeps them and drops from the middle.
func (k keyMap) mainBindings() []BindingEntry {
	return []BindingEntry{
		{ID: BindingNextView, Binding: k.NextView, Action: "Next panel", BarKey: "→", Bar: true, Help: true},
		// Enter opens Details: the bar names the tab it will show (Timeline or
		// Report, set by keyBarEntries) and only while Details is on screen.
		{ID: BindingAttach, Binding: k.Attach, Action: "Timeline", BarKey: "enter", Bar: true, Help: true, When: Model.canAttachDetails},
		// Select, Step day and Move are shown only where they act: a key the
		// bar does not show does nothing and says nothing
		// (the specification Part B, § Bar conditions).
		{ID: BindingSelect, Binding: k.Select, Action: "Select", BarKey: "space", Bar: true, Help: true, When: Model.mainSelects},
		{ID: BindingTimeStepDay, Binding: k.TimeStepDay, Action: "Step day", BarKey: "d/D", Bar: true, Help: true, When: Model.hasWorkspace},
		{ID: BindingSlotCycleUp, Binding: k.SlotCycleUp, Action: "Move", BarKey: "↑↓", Bar: true, Help: true, When: Model.mainMoves},
		// The list keys act on an anchored chart under the Chart panel's cursor,
		// and ⌃k / ⌃j only where it has a neighbour to swap with, so each is on
		// the bar exactly while it does something (D29). The Keys popup, the full
		// reference, lists them whenever a chart is open.
		{ID: BindingSlotMoveUp, Binding: k.SlotMoveUp, Action: "Chart up", BarKey: "⌃k", Bar: true, Help: true, When: Model.canMoveSlotUp, HelpWhen: Model.holdsAChart},
		{ID: BindingSlotMoveDown, Binding: k.SlotMoveDown, Action: "Chart down", BarKey: "⌃j", Bar: true, Help: true, When: Model.canMoveSlotDown, HelpWhen: Model.holdsAChart},
		{ID: BindingSlotDelete, Binding: k.SlotDelete, Action: "Remove", BarKey: "del", Bar: true, Help: true, When: Model.canRemoveSlot, HelpWhen: Model.holdsAChart},
		{ID: BindingFocusChart, Binding: k.FocusChart, Action: "Focus", BarKey: "1/2/3", Bar: true, Help: true},
		// Ctrl+PgUp/PgDn scroll the Details report from anywhere, so the bar names
		// them only while Details is on screen with more than it can show. Both
		// carry the condition: a key that cannot act does not match.
		{ID: BindingScrollUp, Binding: k.ScrollUp, Action: "Scroll details", BarKey: "⌃⇞⌃⇟", Bar: true, Help: true, When: Model.detailsScrolls},
		{ID: BindingSlotCycleDown, Binding: k.SlotCycleDown, BarKey: "↓", Help: true},
		{ID: BindingPrevView, Binding: k.PrevView, BarKey: "←", Help: true},
		{ID: BindingFocusBodies, Binding: k.FocusBodies, BarKey: "2", Help: true},
		{ID: BindingFocusAspects, Binding: k.FocusAspects, BarKey: "3", Help: true},
		{ID: BindingDetailsTab, Binding: k.DetailsTab, BarKey: "0", Help: true},
		{ID: BindingTabPrev, Binding: k.TabPrev, BarKey: "[", Help: true},
		{ID: BindingTabNext, Binding: k.TabNext, BarKey: "]", Help: true},
		{ID: BindingScrollDown, Binding: k.ScrollDown, BarKey: "⌃⇟", Help: true, When: Model.detailsScrolls},
		// The paging keys move the focused list's cursor (Chart, Bodies). Where the
		// focused panel owns no cursor, ⇞ ⇟ scroll Details as they always did, so
		// they carry no condition; Home and End have nothing to fall back to.
		{ID: BindingPageUp, Binding: k.PageUp, BarKey: "⇞", Help: true},
		{ID: BindingPageDown, Binding: k.PageDown, BarKey: "⇟", Help: true},
		{ID: BindingPageHome, Binding: k.CursorHome, BarKey: "home/g", Help: true, When: Model.mainPages},
		{ID: BindingPageEnd, Binding: k.CursorEnd, BarKey: "end/G", Help: true, When: Model.mainPages},
		{ID: BindingTimeStepYear, Binding: k.TimeStepYear, BarKey: "y/Y", Help: true},
		{ID: BindingTimeStepMonth, Binding: k.TimeStepMonth, BarKey: "o/O", Help: true},
		{ID: BindingTimeStepWeek, Binding: k.TimeStepWeek, BarKey: "w/W", Help: true},
		{ID: BindingTimeStepHour, Binding: k.TimeStepHour, BarKey: "h/H", Help: true},
		{ID: BindingTimeStepMinute, Binding: k.TimeStepMinute, BarKey: "m/M", Help: true},
		{ID: BindingJumpNow, Binding: k.JumpNow, BarKey: "n", Help: true},
		{ID: BindingToggleBack, Binding: k.ToggleBack, BarKey: "N", Help: true},
		{ID: BindingOpenAtlas, Binding: k.OpenAtlas, BarKey: "⌃l", Help: true},
		{ID: BindingOpenCharts, Binding: k.OpenCharts, BarKey: "⌃o", Help: true},
		{ID: BindingSaveChart, Binding: k.SaveChart, BarKey: "⌃s", Help: true},
		{ID: BindingBodies, Binding: k.Bodies, BarKey: "⌃b", Help: true},
		{ID: BindingHouses, Binding: k.Houses, BarKey: "⌃h", Help: true},
		{ID: BindingZodiac, Binding: k.Zodiac, BarKey: "⌃z", Help: true},
		{ID: BindingAspects, Binding: k.Aspects, BarKey: "⌃a", Help: true},
		{ID: BindingRenderWheel, Binding: k.RenderWheel, BarKey: "⌃x", Help: true},
		// Reached through the leader, never by its bare key, so it stays out
		// of the flat help list: the leader section prints it with the active
		// leader in front.
		{ID: BindingLogConsole, Binding: k.LogConsole, BarKey: "l", When: Model.leaderPending},
		{ID: BindingSaveCity, Binding: k.SaveCity, BarKey: "c", When: Model.leaderPending},
		{ID: BindingHouseDisplay, Binding: k.HouseDisplay, BarKey: "a", When: Model.leaderPending},
		// Esc while a leader is pending. Not in the bar and not in help: the
		// pending indicator is what tells the operator a leader is armed.
		{ID: BindingCancelLeader, Binding: k.CancelLeader, BarKey: "esc", When: Model.leaderPending},
		{ID: BindingForceQuit, Binding: k.ForceQuit, BarKey: "⌃c", Help: true},
		{ID: BindingHelp, Binding: k.Help, Action: "Help", BarKey: "?", Bar: true, Pinned: true, Help: true},
		{ID: BindingQuit, Binding: k.Quit, Action: "Quit", BarKey: "q", Bar: true, Pinned: true, Help: true},
	}
}

// leaderOnlyBindings are actions the leader alone reaches whose last key is a
// main-screen key: `<leader> o` opens the Sessions list, and bare o steps the
// month. They claim no key of the main table -- the leader resolver dispatches
// the sequence -- and the Keys popup's leader section reads their description
// here.
func (k keyMap) leaderOnlyBindings() []BindingEntry {
	return []BindingEntry{{ID: BindingSessions, Binding: key.NewBinding(key.WithHelp("o", "Sessions")), BarKey: "o"}}
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
		return bodyRowCount(rows.positions, rows.cusps) >= 1
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
		return m.slots.CanSelect(m.cursorIdx)
	case PanelBodies:
		_, ok := bodyAtCursor(m.chart.positions, m.chart.cusps, m.bodyCursorIdx)
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
		return bodyRowCount(m.chart.positions, m.chart.cusps) > 1
	}
	return false
}
func (m Model) leaderPending() bool { return m.leader.pending }

// The popups' shared bindings. A search line types every printable key, so a
// popup list moves with the arrows only; j and k are letters there.
func listUp() BindingEntry {
	return BindingEntry{ID: BindingListUp, Binding: key.NewBinding(key.WithKeys("up")), Action: "Move", BarKey: "↑↓", Bar: true}
}

func listDown() BindingEntry {
	return BindingEntry{ID: BindingListDown, Binding: key.NewBinding(key.WithKeys("down")), BarKey: "↓"}
}

func confirm(action string) BindingEntry {
	return BindingEntry{ID: BindingConfirm, Binding: key.NewBinding(key.WithKeys("enter")), Action: action, BarKey: "enter", Bar: true}
}

func closeOn(action string) BindingEntry {
	return BindingEntry{ID: BindingClose, Binding: key.NewBinding(key.WithKeys("esc")), Action: action, BarKey: "esc", Bar: true, Pinned: true}
}

// closeOnEscOrQ is closeOn for an owner with nothing to type into, where q
// leaves as Esc does.
func closeOnEscOrQ(action string) BindingEntry {
	return BindingEntry{ID: BindingClose, Binding: key.NewBinding(key.WithKeys("esc", "q")), Action: action, BarKey: "esc/q", Bar: true, Pinned: true}
}

func toggle() BindingEntry {
	return BindingEntry{ID: BindingToggle, Binding: key.NewBinding(key.WithKeys(" ")), Action: "Toggle", BarKey: "space", Bar: true}
}

// shownWhen is e with a condition for showing it in the bar. The bar shows
// only what acts (the specification Part B); the handler still owns
// what the key does, so a hidden entry pressed anyway does nothing.
func (e BindingEntry) shownWhen(cond func(Model) bool) BindingEntry {
	e.When = cond
	return e
}

// pageEntries are the paging keys of an owner with a cursor over a list: ⇞ ⇟
// Home End. Nothing here types, so g and G are not among them in a popup with a
// filter box, where they are filter text. Their keys stay off the bar (it is
// full); the README is their reference.
func pageEntries() []BindingEntry {
	return []BindingEntry{
		{ID: BindingPageUp, Binding: key.NewBinding(key.WithKeys("pgup")), BarKey: "⇞"},
		{ID: BindingPageDown, Binding: key.NewBinding(key.WithKeys("pgdown")), BarKey: "⇟"},
		{ID: BindingPageHome, Binding: key.NewBinding(key.WithKeys("home")), BarKey: "home"},
		{ID: BindingPageEnd, Binding: key.NewBinding(key.WithKeys("end")), BarKey: "end"},
	}
}

// popupRows is how many rows the open popup's list holds right now.
func (m Model) popupRows() int {
	switch m.currentOwner() {
	case ownerCity:
		if m.atlas.loadErr != nil || m.atlas.index == nil {
			return 0
		}
		return len(m.atlas.matches)
	case ownerOpenChart:
		return len(m.picker.matches)
	case ownerSessions:
		return len(m.sessions.rows) + 1
	case ownerBodies:
		return len(m.bodyModal.matches)
	case ownerAspects:
		return len(m.aspectModal.matches)
	case ownerHouses:
		return len(m.houseFinder.matches)
	case ownerZodiac:
		return len(m.zodiacFinder.matches)
	}
	return 0
}

func (m Model) popupHasRows() bool { return m.popupRows() > 0 }
func (m Model) popupMoves() bool   { return m.popupRows() > 1 }

// pickerRenames is where Ctrl+R acts: on a chart-file row under the cursor.
func (m Model) pickerRenames() bool {
	if m.picker.loadErr != nil || m.picker.cursor < 0 || m.picker.cursor >= len(m.picker.matches) {
		return false
	}
	return m.picker.matches[m.picker.cursor].Source.Kind == store.ChartKindFile
}

// saveHasName is where Enter saves: a name typed and no save in flight.
func (m Model) saveHasName() bool {
	return !m.save.saving && strings.TrimSpace(m.save.typedName()) != ""
}

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

// cityBindings is the City popup's table.
func cityBindings() []BindingEntry {
	return append([]BindingEntry{listUp().shownWhen(Model.popupMoves), listDown(), confirm("Choose").shownWhen(Model.popupHasRows), closeOn("Cancel")}, pageEntries()...)
}

// openChartBindings is the Open chart popup's table. Ctrl+R renames the
// highlighted chart; it is this table's alone, so it does nothing anywhere
// else (the specification C4.5).
func openChartBindings() []BindingEntry {
	return append([]BindingEntry{
		listUp().shownWhen(Model.popupMoves), listDown(), confirm("Open").shownWhen(Model.popupHasRows),
		{ID: BindingRename, Binding: key.NewBinding(key.WithKeys("ctrl+r")), Action: "Rename", BarKey: "⌃r", Bar: true, When: Model.pickerRenames},
		closeOn("Cancel"),
	}, pageEntries()...)
}

// sessionsBindings is the Sessions list's table: the chart list's moves. Enter
// saves on the first row and opens a session on the others; Ctrl+R renames and
// Ctrl+D trashes the highlighted session. Nothing in it types.
func sessionsBindings() []BindingEntry {
	return append([]BindingEntry{
		listUp().shownWhen(Model.popupMoves), listDown(),
		confirm("Save as").shownWhen(Model.sessionsOnSaveRow),
		confirm("Open").shownWhen(Model.sessionsCanOpen),
		{ID: BindingRename, Binding: key.NewBinding(key.WithKeys("ctrl+r")), Action: "Rename", BarKey: "⌃r", Bar: true, When: Model.sessionsOnFreeRow},
		{ID: BindingSessionDelete, Binding: key.NewBinding(key.WithKeys("ctrl+d")), Action: "Delete", BarKey: "⌃d", Bar: true, When: Model.sessionsCanTrash},
		closeOn("Close"),
	}, pageEntries()...)
}

// sessionNameBindings is the Sessions name field's table: Enter saves (or
// renames), Esc goes back to the list.
func sessionNameBindings() []BindingEntry {
	return []BindingEntry{confirm("Save").shownWhen(Model.sessionNameTyped), closeOn("Back")}
}

// chartTitleBindings is the Chart title popup's table. Esc goes back to the
// chart list; Enter with an empty title opens the chart without one.
func chartTitleBindings() []BindingEntry {
	return []BindingEntry{
		{ID: BindingNextField, Binding: key.NewBinding(key.WithKeys("tab")), Action: "Next field", BarKey: "tab", Bar: true},
		{ID: BindingPrevField, Binding: key.NewBinding(key.WithKeys("shift+tab")), BarKey: "⇧ tab"},
		confirm("Save title"),
		closeOn("Back"),
	}
}

// titleConfirmBindings is the Chart title popup's second step: Enter writes,
// Esc goes back to the form with nothing written.
func titleConfirmBindings() []BindingEntry {
	return []BindingEntry{confirm("Save title"), closeOn("Cancel")}
}

// saveChartBindings is the Save chart popup's table.
func saveChartBindings() []BindingEntry {
	return []BindingEntry{confirm("Save").shownWhen(Model.saveHasName), closeOn("Cancel")}
}

// bodiesBindings is the Bodies popup's table.
func bodiesBindings() []BindingEntry {
	return append(append([]BindingEntry{listUp().shownWhen(Model.popupMoves), listDown(), toggle().shownWhen(Model.popupHasRows)}, chooserCloseBindings()...), pageEntries()...)
}

// chooserCloseBindings is the closing half of the Aspects and Bodies tables.
// Under chooserCloseKeeps (the 2026-10-22 trial) Enter and Esc are one entry,
// Close, since both leave the ticks in force; with the switch off it is the
// old Apply and Cancel pair. q stays a letter: both choosers take filter text.
func chooserCloseBindings() []BindingEntry {
	if chooserCloseKeeps {
		return []BindingEntry{{ID: BindingClose, Binding: key.NewBinding(key.WithKeys("enter", "esc")), Action: "Close", BarKey: "enter/esc", Bar: true, Pinned: true}}
	}
	return []BindingEntry{confirm("Apply"), closeOn("Cancel")}
}

// housesBindings is the house finder's table. Ctrl+H is not in it: inside the
// search line it is a backspace.
func housesBindings() []BindingEntry {
	return append([]BindingEntry{listUp().shownWhen(Model.popupMoves), listDown(), confirm("Choose").shownWhen(Model.popupHasRows), closeOn("Cancel")}, pageEntries()...)
}

// zodiacBindings is the zodiac finder's table: the house finder's, since it is
// the same kind of popup. Ctrl+H is not in it either: inside the search line it
// is a backspace.
func zodiacBindings() []BindingEntry { return housesBindings() }

// displayBindings is the House display popup's table. Nothing in it types, so
// j and k move as they do on the main screen, g and G are Home and End, and q
// leaves as Esc does (C11). Space chooses the focused form and leaves the popup
// open; it is on the bar only while the focus is on a form that is not yet the
// chosen one. Enter and Esc close it keeping the choice.
func displayBindings() []BindingEntry {
	return []BindingEntry{
		{ID: BindingListUp, Binding: key.NewBinding(key.WithKeys("up", "k")), Action: "Move", BarKey: "↑↓", Bar: true},
		{ID: BindingListDown, Binding: key.NewBinding(key.WithKeys("down", "j")), BarKey: "↓"},
		{ID: BindingToggle, Binding: key.NewBinding(key.WithKeys(" ")), Action: "Choose", BarKey: "space", Bar: true, When: Model.displayCanChoose},
		{ID: BindingClose, Binding: key.NewBinding(key.WithKeys("enter", "esc", "q")), Action: "Close", BarKey: "enter/esc/q", Bar: true, Pinned: true},
		{ID: BindingPageUp, Binding: key.NewBinding(key.WithKeys("pgup")), BarKey: "⇞"},
		{ID: BindingPageDown, Binding: key.NewBinding(key.WithKeys("pgdown")), BarKey: "⇟"},
		{ID: BindingPageHome, Binding: key.NewBinding(key.WithKeys("home", "g")), BarKey: "home/g"},
		{ID: BindingPageEnd, Binding: key.NewBinding(key.WithKeys("end", "G")), BarKey: "end/G"},
	}
}

// aspectsBindings is the Aspects popup's table.
func aspectsBindings() []BindingEntry {
	return append(append([]BindingEntry{listUp().shownWhen(Model.popupMoves), listDown(), toggle().shownWhen(Model.popupHasRows)}, chooserCloseBindings()...), pageEntries()...)
}

// helpBindings is the Keys popup's table. It has nothing to type into, so j
// and k move as they do on the main screen, q closes it as Esc does, and ?
// closes it as it opened it.
func helpBindings() []BindingEntry {
	return []BindingEntry{
		{ID: BindingListUp, Binding: key.NewBinding(key.WithKeys("up", "k")), Action: "Move", BarKey: "↑↓", Bar: true},
		{ID: BindingListDown, Binding: key.NewBinding(key.WithKeys("down", "j")), BarKey: "↓"},
		{ID: BindingScrollUp, Binding: key.NewBinding(key.WithKeys("pgup")), Action: "Page", BarKey: "⇞⇟", Bar: true},
		{ID: BindingScrollDown, Binding: key.NewBinding(key.WithKeys("pgdown")), BarKey: "⇟"},
		// The text has no filter box, so g and G are Home and End here.
		{ID: BindingPageHome, Binding: key.NewBinding(key.WithKeys("home", "g")), BarKey: "home/g"},
		{ID: BindingPageEnd, Binding: key.NewBinding(key.WithKeys("end", "G")), BarKey: "end/G"},
		{ID: BindingHelp, Binding: key.NewBinding(key.WithKeys("?")), BarKey: "?"},
		closeOnEscOrQ("Close"),
	}
}

// consoleBindings is the Log console's table.
func consoleBindings() []BindingEntry {
	return []BindingEntry{
		{ID: BindingListUp, Binding: key.NewBinding(key.WithKeys("up", "k")), Action: "Move", BarKey: "↑↓", Bar: true, When: Model.consoleMoves},
		{ID: BindingListDown, Binding: key.NewBinding(key.WithKeys("down", "j")), BarKey: "↓"},
		{ID: BindingCopy, Binding: key.NewBinding(key.WithKeys("enter")), Action: "Copy line", BarKey: "enter", Bar: true, When: Model.consoleHasLines},
		// ⇞ ⇟ Home End (and g G, the console has nothing to type) page the cursor.
		{ID: BindingPageUp, Binding: key.NewBinding(key.WithKeys("pgup")), BarKey: "⇞"},
		{ID: BindingPageDown, Binding: key.NewBinding(key.WithKeys("pgdown")), BarKey: "⇟"},
		{ID: BindingPageHome, Binding: key.NewBinding(key.WithKeys("home", "g")), BarKey: "home/g"},
		{ID: BindingPageEnd, Binding: key.NewBinding(key.WithKeys("end", "G")), BarKey: "end/G"},
		closeOnEscOrQ("Close"),
	}
}

// detailsBindings is attached Details' table: the keys Details answers itself,
// ahead of the main screen's. ↑↓ move the row cursor on the
// timeline tab and scroll the report a line on the report tab (timeline C26).
// The timeline's own keys — the jump, the row marks, the filter — exist on the
// timeline tab only; the step keys and now/back work on both tabs (D37).
func (k keyMap) detailsBindings() []BindingEntry {
	return []BindingEntry{
		{ID: BindingListUp, Binding: key.NewBinding(key.WithKeys("up", "k")), Action: "Move", BarKey: "↑↓", Bar: true, When: Model.detailsMoves},
		{ID: BindingListDown, Binding: key.NewBinding(key.WithKeys("down", "j")), BarKey: "↓"},
		{ID: BindingConfirm, Binding: key.NewBinding(key.WithKeys("enter")), Action: "Jump", BarKey: "enter", Bar: true, When: Model.detailsOnTimeline},
		// `,y` is the leader's (updateLeaderKey), so this entry matches no key
		// of its own; it is here for the bar, on both tabs.
		{ID: BindingCopy, Binding: key.NewBinding(), Action: "Copy", BarKey: ", y", Bar: true},
		{ID: BindingToggle, Binding: k.Select, Action: "Mark", BarKey: "space", Bar: true, When: Model.detailsOnTimelineRows},
		{ID: BindingSearch, Binding: timelineSearchKey, Action: "Filter", BarKey: "/", Bar: true, When: Model.detailsFilterable},
		{ID: BindingTabNext, Binding: k.TabNext, Action: "Tab", BarKey: "[ ]", Bar: true, When: Model.detailsHasSeveralTabs},
		{ID: BindingTabPrev, Binding: k.TabPrev, BarKey: "["},
		{ID: BindingDetailsTab, Binding: k.DetailsTab, BarKey: "0"},
		{ID: BindingFocusChart, Binding: k.FocusChart, Action: "Focus", BarKey: "1/2/3", Bar: true},
		{ID: BindingFocusBodies, Binding: k.FocusBodies, BarKey: "2"},
		{ID: BindingFocusAspects, Binding: k.FocusAspects, BarKey: "3"},
		{ID: BindingScrollUp, Binding: k.ScrollUp, Action: "Scroll", BarKey: "⌃⇞⌃⇟", Bar: true, When: Model.detailsScrolls},
		{ID: BindingScrollDown, Binding: k.ScrollDown, BarKey: "⌃⇟", When: Model.detailsScrolls},
		// ⇞ ⇟ Home End move the timeline's row cursor (a tab with no row cursor
		// scrolls two lines on ⇞ ⇟, as it always did). Their keys are not in the
		// bar: it is full, and the README is their reference.
		{ID: BindingPageUp, Binding: k.PageUp, BarKey: "⇞"},
		{ID: BindingPageDown, Binding: k.PageDown, BarKey: "⇟"},
		{ID: BindingPageHome, Binding: k.CursorHome, BarKey: "home/g", When: Model.detailsOnTimelineRows},
		{ID: BindingPageEnd, Binding: k.CursorEnd, BarKey: "end/G", When: Model.detailsOnTimelineRows},
		{ID: BindingTimeStepYear, Binding: k.TimeStepYear, Action: "Step", BarKey: "y o w d h m", Bar: true},
		{ID: BindingTimeStepMonth, Binding: k.TimeStepMonth, BarKey: "o/O"},
		{ID: BindingTimeStepWeek, Binding: k.TimeStepWeek, BarKey: "w/W"},
		{ID: BindingTimeStepDay, Binding: k.TimeStepDay, BarKey: "d/D"},
		{ID: BindingTimeStepHour, Binding: k.TimeStepHour, BarKey: "h/H"},
		{ID: BindingTimeStepMinute, Binding: k.TimeStepMinute, BarKey: "m/M"},
		{ID: BindingJumpNow, Binding: k.JumpNow, BarKey: "n"},
		{ID: BindingToggleBack, Binding: k.ToggleBack, BarKey: "N"},
		// q and Left leave as Esc does; the shared closeOn stays Esc-only, so a
		// popup or form that types q is unaffected (time-keys-everywhere rule 4).
		{ID: BindingClose, Binding: key.NewBinding(key.WithKeys("esc", "q", "left")), Action: "Close", BarKey: "esc/q", Bar: true, Pinned: true},
		// Every other main key — the popups, ⌃x, help, quit — is not listed:
		// it falls through to the main screen's dispatch unless detailsRefused
		// names it (passDetailsKeyToMain).
	}
}

// timelineSearchBindings is the bar while the search line is open: the line
// types, the arrows move, Enter applies and Esc goes back.
func timelineSearchBindings() []BindingEntry {
	return []BindingEntry{
		{ID: BindingListUp, Binding: key.NewBinding(key.WithKeys("up")), Action: "Move", BarKey: "↑↓", Bar: true},
		{ID: BindingListDown, Binding: key.NewBinding(key.WithKeys("down")), BarKey: "↓"},
		{ID: BindingConfirm, Binding: key.NewBinding(key.WithKeys("enter")), Action: "Apply", BarKey: "enter", Bar: true},
		{ID: BindingClose, Binding: key.NewBinding(key.WithKeys("esc")), Action: "Back", BarKey: "esc", Bar: true},
		// ⇞ ⇟ page the cursor, as the arrows move it; Home, End, g and G are text
		// or nothing in the line. Ctrl+⇞ ⇟ pass through to the Details scroll.
		{ID: BindingPageUp, Binding: key.NewBinding(key.WithKeys("pgup")), BarKey: "⇞"},
		{ID: BindingPageDown, Binding: key.NewBinding(key.WithKeys("pgdown")), BarKey: "⇟"},
		{ID: BindingScrollUp, Binding: key.NewBinding(key.WithKeys("ctrl+pgup")), BarKey: "⌃⇞"},
		{ID: BindingScrollDown, Binding: key.NewBinding(key.WithKeys("ctrl+pgdown")), BarKey: "⌃⇟"},
	}
}

func (m Model) detailsOnTimelineRows() bool {
	return m.detailsOnTimeline() && len(m.timelineShown()) > 0
}

// detailsHasSeveralTabs is whether `[ ]` have anything to switch between: only
// the Chart panel's Details has a second tab (the specification).
func (m Model) detailsHasSeveralTabs() bool { return len(m.panelTabs(PanelDetails)) > 1 }

func (m Model) detailsOnTimeline() bool { return m.activeTabID(PanelDetails) == TabDetailsTimeline }

// owner is whatever has the keyboard.
type owner int

const (
	ownerMain owner = iota
	ownerCity
	ownerOpenChart
	ownerChartTitle
	ownerSaveChart
	ownerBodies
	ownerHouses
	ownerZodiac
	ownerAspects
	ownerHelp
	ownerConsole
	ownerDetails
	ownerDisplay
	ownerSessions
	ownerSessionName
)

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
	case m.console.open:
		return ownerConsole
	case m.details.attached:
		return ownerDetails
	}
	return ownerMain
}

// ownerBindings is owner o's table.
func (m Model) ownerBindings(o owner) []BindingEntry {
	switch o {
	case ownerCity:
		return cityBindings()
	case ownerOpenChart:
		return openChartBindings()
	case ownerChartTitle:
		if m.title.confirming {
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
	case ownerSessions:
		return sessionsBindings()
	case ownerSessionName:
		return sessionNameBindings()
	case ownerHelp:
		return helpBindings()
	case ownerConsole:
		return consoleBindings()
	case ownerDetails:
		if m.timeline.searching && m.timelineSearchLine() {
			return timelineSearchBindings()
		}
		if m.detailsReportSearchOpen() {
			return reportSearchBindings()
		}
		return m.keys.detailsBindings()
	}
	return m.keys.mainBindings()
}

// available drops the entries whose condition does not hold right now.
func (m Model) available(entries []BindingEntry) []BindingEntry {
	out := make([]BindingEntry, 0, len(entries))
	for _, e := range entries {
		if e.When != nil && !e.When(m) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// matchBinding is the one place a key press is matched: the id of the first
// entry whose keys it is, or BindingNone.
func matchBinding(msg tea.KeyMsg, entries []BindingEntry) BindingID {
	for _, e := range entries {
		if key.Matches(msg, e.Binding) {
			return e.ID
		}
	}
	return BindingNone
}

// strandingKeys are the keys a leader may never take. `q` is the only quit
// from the main screen; esc, enter and tab are the ways out of a popup or the
// console. A leader on any of them would leave the operator with no exit.
func strandingKeys() []string { return []string{"q", "esc", "enter", "tab"} }

func defaultKeys() keyMap {
	return keyMap{
		// q quits from the main screen only. Every other owner types it or
		// ignores it (C11); Ctrl+C is the quit that works everywhere.
		Quit: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", "Quit"),
		),
		ForceQuit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("⌃c", "Quit from anywhere"),
		),
		CancelLeader: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "Cancel the leader"),
		),
		// Panel navigation. `l`/`h` were provisional lazygit-style nav
		// removed 2026-06-03 when an earlier change reclaimed `h`/`H` for the
		// canonical time-step hour key (the design notes:119).
		// The bar names the arrow; the Keys popup names both keys.
		NextView: key.NewBinding(
			key.WithKeys("tab", "right"),
			key.WithHelp("tab/→", "Next panel"),
		),
		PrevView: key.NewBinding(
			key.WithKeys("shift+tab", "left"),
			key.WithHelp("⇧ tab/←", "Previous panel"),
		),
		// The ring digits. `0` is Details' digit: Details is not in the focus
		// ring, so `0` cycles its tabs from anywhere instead of focusing it
		// (D14).
		FocusChart: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "Focus Chart"),
		),
		FocusBodies: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "Focus Bodies"),
		),
		FocusAspects: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "Focus Aspects"),
		),
		DetailsTab: key.NewBinding(
			key.WithKeys("0"),
			key.WithHelp("0", "Next Details tab"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "Help"),
		),
		OpenAtlas: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("⌃l", "Choose city"),
		),
		// Ctrl+O opens the saved-chart library picker. Selecting a
		// chart sets it as the natal anchor and recomputes the overlay.
		OpenCharts: key.NewBinding(
			key.WithKeys("ctrl+o"),
			key.WithHelp("⌃o", "Open chart"),
		),
		// The leader sequence reaches the same action for a terminal that
		// eats ctrl+s for flow control.
		SaveChart: key.NewBinding(
			key.WithKeys("ctrl+s"),
			key.WithHelp("⌃s", "Save chart"),
		),
		Select: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "Select"),
		),
		SlotCycleUp: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "Previous row"),
		),
		SlotCycleDown: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "Next row"),
		),
		// The workspace list (the specification M4, M5). bubbletea reports LF as
		// ctrl+j and CR as enter, so ⌃j does not collide with Enter.
		SlotMoveUp: key.NewBinding(
			key.WithKeys("ctrl+k"),
			key.WithHelp("⌃k", "Move chart up"),
		),
		SlotMoveDown: key.NewBinding(
			key.WithKeys("ctrl+j"),
			key.WithHelp("⌃j", "Move chart down"),
		),
		SlotDelete: key.NewBinding(
			key.WithKeys("delete"),
			key.WithHelp("del", "Remove chart"),
		),
		// Time-step keys per the design notes:113-122. Lowercase
		// = forward, uppercase = backward. `o`/`O` takes month precisely so
		// that `m`/`M` stay free for the minute step, which pitch.md:122 has
		// promised since before v1 and which rectification work needs: the
		// finest step the other keys do not offer.
		TimeStepYear: key.NewBinding(
			key.WithKeys("y", "Y"),
			key.WithHelp("y/Y", "Step a year"),
		),
		TimeStepMonth: key.NewBinding(
			key.WithKeys("o", "O"),
			key.WithHelp("o/O", "Step a month"),
		),
		TimeStepWeek: key.NewBinding(
			key.WithKeys("w", "W"),
			key.WithHelp("w/W", "Step a week"),
		),
		TimeStepDay: key.NewBinding(
			key.WithKeys("d", "D"),
			key.WithHelp("d/D", "Step a day"),
		),
		TimeStepHour: key.NewBinding(
			key.WithKeys("h", "H"),
			key.WithHelp("h/H", "Step an hour"),
		),
		TimeStepMinute: key.NewBinding(
			key.WithKeys("m", "M"),
			key.WithHelp("m/M", "Step a minute"),
		),
		// Ctrl+X renders the focused slot's chart as a wheel image and serves
		// it (or hands it to the viewer); a second press closes or refreshes it.
		RenderWheel: key.NewBinding(
			key.WithKeys("ctrl+x"),
			key.WithHelp("⌃x", "Wheel image"),
		),
		// `n` jumps the floating slot to now and makes it follow the clock;
		// `N` toggles back one level. Both act from any cursor position.
		JumpNow: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "Now"),
		),
		ToggleBack: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "Back to the previous time"),
		),
		// Ctrl+PgUp/PgDn scroll the Details report two lines at a time — lazygit's
		// `scrollHeight: 2`. They act on Details from wherever focus is:
		// Details is not focusable, so no other panel competes for them. A
		// modifier pair because the plain keys page the focused list.
		ScrollUp: key.NewBinding(
			key.WithKeys("ctrl+pgup"),
			key.WithHelp("⌃⇞", "Scroll Details up"),
		),
		ScrollDown: key.NewBinding(
			key.WithKeys("ctrl+pgdown"),
			key.WithHelp("⌃⇟", "Scroll Details down"),
		),
		// ⇞ ⇟ Home End (and g G) move the cursor of the focused list: first to
		// the edge of its window, then by a page of the window minus one row.
		// Where the focused panel has no cursor, ⇞ ⇟ still scroll Details.
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("⇞", "Page the list up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("⇟", "Page the list down"),
		),
		// g and G are Home and End where the letter is not typed as text. A
		// configured leader is consumed before this table (model.go), so a
		// leader of g keeps the leader and loses the jump; Home and End work.
		CursorHome: key.NewBinding(
			key.WithKeys("home", "g"),
			key.WithHelp("home/g", "First row"),
		),
		CursorEnd: key.NewBinding(
			key.WithKeys("end", "G"),
			key.WithHelp("end/G", "Last row"),
		),
		// The leader sequence `l` turns the Log into a console. The key
		// here is the sequence's last key, not a main-mode binding: the
		// leader resolver dispatches it, key.Matches never sees it.
		LogConsole: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "Log console"),
		),
		// `<leader> c d` writes the current city as the default city. The key
		// here is the sequence's first key: its last, d, is the day step in main
		// mode and must not read as a claim on it. Typed after the leader, c
		// arms the second key, which is what the coverage tests exercise.
		SaveCity: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "Save defaults"),
		),
		// `<leader> a` opens the House display popup. Like the other leader
		// sequences it is reached through the leader, never by its bare key, so
		// it stays out of the flat help list (the leader section prints it).
		HouseDisplay: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "House display"),
		),
		// ⌃a opens the aspect chooser; the body selector takes ⌃b. Plain
		// letters were the first choice and were wrong: every letter is
		// also a time-step or a panel key somewhere, and the modifier is
		// what keeps a chooser from competing with the keys an operator
		// uses continuously while reading a chart.
		Aspects: key.NewBinding(
			key.WithKeys("ctrl+a"),
			key.WithHelp("⌃a", "Choose aspects"),
		),
		Bodies: key.NewBinding(
			key.WithKeys("ctrl+b"),
			key.WithHelp("⌃b", "Choose bodies"),
		),
		// ⌃h opens the house finder (the specification). Inside a search box
		// it is a backspace (the specification); that is the popup
		// owners' business, because this table is the main screen's alone. The
		// leader sequence `, h` reaches the same action, and is the one that
		// works on a terminal that sends ⌃h for Backspace.
		Houses: key.NewBinding(
			key.WithKeys("ctrl+h"),
			key.WithHelp("⌃h", "Choose houses"),
		),
		// ⌃z opens the zodiac finder (the specification C8), the house
		// finder's sibling; the leader sequence `, z` reaches the same action.
		// Bubble Tea delivers ⌃z as an ordinary key and suspends only on a
		// tea.Suspend this program never returns (TestCtrlZDoesNotSuspend).
		Zodiac: key.NewBinding(
			key.WithKeys("ctrl+z"),
			key.WithHelp("⌃z", "Choose zodiac"),
		),
		// Enter attaches Details: row navigation moves inside it and it owns
		// the keyboard until Esc (the specification D12, D18).
		Attach: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "Into Details"),
		),
		// `[`/`]` cycle the focused view's tabs: Details while attached, the
		// focused ring panel otherwise, which today has one tab (C34, C36).
		TabPrev: key.NewBinding(
			key.WithKeys("["),
			key.WithHelp("[", "Previous tab"),
		),
		TabNext: key.NewBinding(
			key.WithKeys("]"),
			key.WithHelp("]", "Next tab"),
		),
	}
}
