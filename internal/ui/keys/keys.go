// Package keys holds the keyboard's vocabulary and what is drawn from it: the
// binding ids, one owner's table of entries, the matcher, the leader's prefix
// tree, the key bar and the Keys popup. It knows nothing about charts, slots
// or the model: everything an entry's condition or a title needs arrives in a
// Context the ui package resolves for the frame (see context.go).
package keys

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// Typing into a search line or a form field (printable characters, Space where
// it types, Backspace) is text entry and lives with the field, not here.

// BindingID names one action of the keyboard. One id names one action; each
// owner's table gives it that owner's words.
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

	// BindingNewSession is `<leader> n`: a new, unnamed session. It is appended
	// so the ids above keep their numbers.
	BindingNewSession

	// BindingActions is Ctrl+P: the Actions palette, the list of every command
	// with its key. It opens the list and is not one of its rows.
	BindingActions

	// BindingDefaults is Ctrl+D in the Bodies chooser: tick the lens's default
	// bodies again. It is appended so the ids above keep their numbers.
	BindingDefaults

	// BindingConsole is the Log console, opened directly: `l` while the Log has
	// the focus, and the Actions row Log console. BindingLogConsole is `<leader> l`,
	// which now gives the Log the focus. Appended so the ids above keep their
	// numbers.
	BindingConsole
)

// BindingNone is what matchBinding returns for a key no entry matches.
const BindingNone BindingID = -1

// BindingEntry is one row of an owner's table.
type BindingEntry struct {
	ID BindingID
	// Binding holds the keys the entry matches. Its Help() is the Keys
	// popup's spelling of the key (`tab/→`) and the description it lists.
	Binding key.Binding
	// Title is the key's one name, written once: Help shows it as is
	// (`Move chart up`) and the bar shortens it by barWord. TitleFn, if set,
	// is the title for the model's state (Enter: Timeline or Report) and Title
	// is its resting value. The one method that reads them is title.
	Title   string
	TitleFn func(Context) string
	// BarKey is the key as the bar spells it (`→`). HelpKey, if set, is the
	// spelling Help gives the key; without it Help uses the binding's own
	// spelling (the main table), then BarKey. No Log line names a key: the bar
	// is the one place a key is explained (the specification Part B).
	BarKey  string
	HelpKey string
	// Folded marks the second (third) key of a pair: it matches its key and
	// has no bar entry and no Help row, since the first entry carries the
	// pair's title and both keys.
	Folded bool
	// Word is the bar's word for the entry, set by withWords from the title
	// and the owner; it is what the key bar draws.
	Word string
	// Bar puts the entry in the key bar, in table order. Pinned keeps it
	// there when the bar is too narrow for every entry (C4).
	Bar    bool
	Pinned bool
	// Help lists the entry in the Keys popup.
	Help bool
	// When, if set, is the condition under which the entry is available.
	// An unavailable entry neither matches nor appears in the bar.
	When func(Context) bool
	// HelpWhen, if set, is the condition under which the Keys popup lists the
	// entry. It is separate from When, which is whether the key acts: the popup
	// is the full reference for the screen and lists a key while it cannot act
	// (Select is there while nothing can be selected), and only the list keys
	// wait for a chart to be open.
	HelpWhen func(Context) bool
	// Palette marks the entry as a row of the Actions palette: the commands
	// (open a chooser, save, Now, Back, the wheel, sessions, Help, quit). Motion,
	// paging, tab switching and the time steps are not. The palette lists the
	// marked rows of the main table and of the leader-only entries.
	Palette bool
	// ShownSeq, if set, is the leader sequence the key column prints for the
	// entry although the leader map does not hold it: `<leader> l l` is the Log
	// console, and `<leader> l` is the Log.
	ShownSeq []string
}

// KeyMap is the main screen's keys, one binding per action.
type KeyMap struct {
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
	Actions        key.Binding
}

// MatchBinding is the one place a key press is matched: the id of the first
// entry whose keys it is, or BindingNone.
func MatchBinding(msg tea.KeyMsg, entries []BindingEntry) BindingID {
	for _, e := range entries {
		if key.Matches(msg, e.Binding) {
			return e.ID
		}
	}
	return BindingNone
}

// DefaultKeys is the key map every launch starts from.
func DefaultKeys() KeyMap {
	return KeyMap{
		// q quits from the main screen only. Every other owner types it or
		// ignores it (C11); Ctrl+C is the quit that works everywhere.
		Quit: key.NewBinding(
			key.WithKeys("q"),
			key.WithHelp("q", ""),
		),
		ForceQuit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("⌃c", ""),
		),
		CancelLeader: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", ""),
		),
		// Panel navigation. `l`/`h` were provisional lazygit-style nav
		// removed 2026-06-03 when an earlier change reclaimed `h`/`H` for the
		// canonical time-step hour key (the design notes:119).
		// The bar names the arrow; the Keys popup names both keys.
		NextView: key.NewBinding(
			key.WithKeys("tab", "right"),
			key.WithHelp("tab/→ ⇧ tab/←", ""),
		),
		PrevView: key.NewBinding(
			key.WithKeys("shift+tab", "left"),
			key.WithHelp("⇧ tab/←", ""),
		),
		// The ring digits. `0` is Details' digit: Details is not in the focus
		// ring, so `0` cycles its tabs from anywhere instead of focusing it
		// (D14).
		FocusChart: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1/2/3", ""),
		),
		FocusBodies: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", ""),
		),
		FocusAspects: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", ""),
		),
		DetailsTab: key.NewBinding(
			key.WithKeys("0"),
			key.WithHelp("0", ""),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", ""),
		),
		OpenAtlas: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("⌃l", ""),
		),
		// Ctrl+O opens the saved-chart library picker. Selecting a
		// chart sets it as the natal anchor and recomputes the overlay.
		OpenCharts: key.NewBinding(
			key.WithKeys("ctrl+o"),
			key.WithHelp("⌃o", ""),
		),
		// The leader sequence reaches the same action for a terminal that
		// eats ctrl+s for flow control.
		SaveChart: key.NewBinding(
			key.WithKeys("ctrl+s"),
			key.WithHelp("⌃s", ""),
		),
		Select: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", ""),
		),
		SlotCycleUp: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/↓ (j/k)", ""),
		),
		SlotCycleDown: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", ""),
		),
		// The workspace list (the specification M4, M5). bubbletea reports LF as
		// ctrl+j and CR as enter, so ⌃j does not collide with Enter.
		SlotMoveUp: key.NewBinding(
			key.WithKeys("ctrl+k"),
			key.WithHelp("⌃k", ""),
		),
		SlotMoveDown: key.NewBinding(
			key.WithKeys("ctrl+j"),
			key.WithHelp("⌃j", ""),
		),
		SlotDelete: key.NewBinding(
			key.WithKeys("delete"),
			key.WithHelp("del", ""),
		),
		// Time-step keys per the design notes:113-122. Lowercase
		// = forward, uppercase = backward. `o`/`O` takes month precisely so
		// that `m`/`M` stay free for the minute step, which pitch.md:122 has
		// promised since before v1 and which rectification work needs: the
		// finest step the other keys do not offer.
		TimeStepYear: key.NewBinding(
			key.WithKeys("y", "Y"),
			key.WithHelp("y/Y", ""),
		),
		TimeStepMonth: key.NewBinding(
			key.WithKeys("o", "O"),
			key.WithHelp("o/O", ""),
		),
		TimeStepWeek: key.NewBinding(
			key.WithKeys("w", "W"),
			key.WithHelp("w/W", ""),
		),
		TimeStepDay: key.NewBinding(
			key.WithKeys("d", "D"),
			key.WithHelp("d/D", ""),
		),
		TimeStepHour: key.NewBinding(
			key.WithKeys("h", "H"),
			key.WithHelp("h/H", ""),
		),
		TimeStepMinute: key.NewBinding(
			key.WithKeys("m", "M"),
			key.WithHelp("m/M", ""),
		),
		// Ctrl+X renders the focused slot's chart as a wheel image and serves
		// it (or hands it to the viewer); a second press closes or refreshes it.
		RenderWheel: key.NewBinding(
			key.WithKeys("ctrl+x"),
			key.WithHelp("⌃x", ""),
		),
		// `n` jumps the floating slot to now and makes it follow the clock;
		// `N` toggles back one level. Both act from any cursor position.
		JumpNow: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", ""),
		),
		ToggleBack: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", ""),
		),
		// Ctrl+PgUp/PgDn scroll the Details report two lines at a time — lazygit's
		// `scrollHeight: 2`. They act on Details from wherever focus is:
		// Details is not focusable, so no other panel competes for them. A
		// modifier pair because the plain keys page the focused list.
		ScrollUp: key.NewBinding(
			key.WithKeys("ctrl+pgup"),
			key.WithHelp("⌃⇞/⌃⇟", ""),
		),
		ScrollDown: key.NewBinding(
			key.WithKeys("ctrl+pgdown"),
			key.WithHelp("⌃⇟", ""),
		),
		// ⇞ ⇟ Home End (and g G) move the cursor of the focused list: first to
		// the edge of its window, then by a page of the window minus one row.
		// Where the focused panel has no cursor, ⇞ ⇟ still scroll Details.
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("⇞/⇟", ""),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("⇟", ""),
		),
		// g and G are Home and End where the letter is not typed as text. A
		// configured leader is consumed before this table (model.go), so a
		// leader of g keeps the leader and loses the jump; Home and End work.
		CursorHome: key.NewBinding(
			key.WithKeys("home", "g"),
			key.WithHelp("home/g", ""),
		),
		CursorEnd: key.NewBinding(
			key.WithKeys("end", "G"),
			key.WithHelp("end/G", ""),
		),
		// The leader sequence `l` turns the Log into a console. The key
		// here is the sequence's last key, not a main-mode binding: the
		// leader resolver dispatches it, key.Matches never sees it.
		LogConsole: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", ""),
		),
		// `<leader> c d` writes the current city as the default city. The key
		// here is the sequence's first key: its last, d, is the day step in main
		// mode and must not read as a claim on it. Typed after the leader, c
		// arms the second key, which is what the coverage tests exercise.
		SaveCity: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", ""),
		),
		// `<leader> a` opens the House display popup. Like the other leader
		// sequences it is reached through the leader, never by its bare key, so
		// it stays out of the flat help list (the leader section prints it).
		HouseDisplay: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", ""),
		),
		// ⌃a opens the aspect chooser; the body selector takes ⌃b. Plain
		// letters were the first choice and were wrong: every letter is
		// also a time-step or a panel key somewhere, and the modifier is
		// what keeps a chooser from competing with the keys an operator
		// uses continuously while reading a chart.
		Aspects: key.NewBinding(
			key.WithKeys("ctrl+a"),
			key.WithHelp("⌃a", ""),
		),
		Bodies: key.NewBinding(
			key.WithKeys("ctrl+b"),
			key.WithHelp("⌃b", ""),
		),
		// ⌃h opens the house finder (the specification). Inside a search box
		// it is a backspace (the specification); that is the popup
		// owners' business, because this table is the main screen's alone. The
		// leader sequence `, h` reaches the same action, and is the one that
		// works on a terminal that sends ⌃h for Backspace.
		Houses: key.NewBinding(
			key.WithKeys("ctrl+h"),
			key.WithHelp("⌃h", ""),
		),
		// ⌃z opens the zodiac finder (the specification C8), the house
		// finder's sibling; the leader sequence `, z` reaches the same action.
		// Bubble Tea delivers ⌃z as an ordinary key and suspends only on a
		// tea.Suspend this program never returns (TestCtrlZDoesNotSuspend).
		Zodiac: key.NewBinding(
			key.WithKeys("ctrl+z"),
			key.WithHelp("⌃z", ""),
		),
		// Enter attaches Details: row navigation moves inside it and it owns
		// the keyboard until Esc (the specification D12, D18).
		Attach: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", ""),
		),
		// `[`/`]` cycle the focused view's tabs: Details while attached, the
		// focused ring panel otherwise, which today has one tab (C34, C36).
		TabPrev: key.NewBinding(
			key.WithKeys("["),
			key.WithHelp("[/]", ""),
		),
		TabNext: key.NewBinding(
			key.WithKeys("]"),
			key.WithHelp("]", ""),
		),
		// ⌃p opens the Actions palette: every command with its key, searchable.
		// It is listed in Help and not on the key bar (operator, 2026-11-04:
		// "Only in Help").
		Actions: key.NewBinding(
			key.WithKeys("ctrl+p"),
			key.WithHelp("⌃p", ""),
		),
	}
}

// Available drops the entries whose condition does not hold in this context.
func Available(c Context, entries []BindingEntry) []BindingEntry {
	out := make([]BindingEntry, 0, len(entries))
	for _, e := range entries {
		if e.When != nil && !e.When(c) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// ShownWhen is e with a condition for showing it in the bar. The bar shows
// only what acts (the specification Part B); the handler still owns
// what the key does, so a hidden entry pressed anyway does nothing.
func (e BindingEntry) ShownWhen(cond func(Context) bool) BindingEntry {
	e.When = cond
	return e
}

// Titled gives every entry's binding its Help text: the key's spelling (the
// entry's HelpKey, else the one the binding already carries, else BarKey) and
// the title as the description. Nothing writes a Help description by hand, so
// Help and the bar cannot name a key differently.
func Titled(entries []BindingEntry) []BindingEntry {
	for i := range entries {
		e := &entries[i]
		spelling := e.HelpKey
		if spelling == "" {
			spelling = e.Binding.Help().Key
		}
		if spelling == "" {
			spelling = e.BarKey
		}
		e.Binding.SetHelp(spelling, e.Title)
	}
	return entries
}

// TitleFor is the entry's title for this context: TitleFn when it has one (Enter
// names the Details tab it will show), else Title. The bar, Help and the
// leader's help all read it here.
func (e BindingEntry) TitleFor(c Context) string {
	if e.TitleFn != nil {
		return e.TitleFn(c)
	}
	return e.Title
}

// Owner is whatever has the keyboard.
type Owner int

const (
	OwnerMain Owner = iota
	OwnerCity
	OwnerOpenChart
	OwnerChartTitle
	OwnerSaveChart
	OwnerBodies
	OwnerHouses
	OwnerZodiac
	OwnerAspects
	OwnerHelp
	OwnerConsole
	OwnerDetails
	OwnerDisplay
	OwnerSessions
	OwnerSessionName
	OwnerActions
	// OwnerLog is the Log panel with the keyboard: a mode beside the console,
	// never the focused panel. Appended so the owners above keep their numbers.
	OwnerLog
	// OwnerExperiments is the hidden Experiments popup (`,l p`). Appended so the
	// owners above keep their numbers.
	OwnerExperiments
)

// Variant is which of an owner's tables is in force: the chart-title popup has
// one while it asks to confirm, attached Details has one for each search line.
type Variant int

const (
	VariantDefault Variant = iota
	VariantTitleConfirm
	VariantTimelineSearch
	VariantReportSearch
)
