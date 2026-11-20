// Package keys holds the keyboard's vocabulary and what is drawn from it: the
// binding ids, one owner's table of entries, the matcher, the leader's prefix
// tree, the key bar and the Keybindings popup. It knows nothing about charts, slots
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
	BindingFocusSession
	BindingFocusChart
	BindingFocusBodies
	BindingFocusAspects
	BindingDetailsTab
	// BindingKeybindings is `?`: the Keybindings popup, opened at the keys of
	// the place you are in.
	BindingHelp
	BindingOpenAtlas
	BindingOpenCharts
	BindingSaveChart
	BindingSelect
	BindingSlotCycleUp
	BindingSlotCycleDown
	// The workspace list's own keys: K / J (and Shift+↑ / Shift+↓) move the
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
	BindingTimeStepSecond
	// BindingMoonPhase is p: the floating chart jumps to the next Moon phase.
	BindingMoonPhase
	BindingRenderWheel
	BindingJumpNow
	BindingSearch
	BindingScrollUp
	BindingScrollDown
	BindingLogConsole
	BindingAspects
	BindingBodies
	BindingHouses
	BindingZodiac
	// BindingNodes is `<leader> t` and Ctrl+T: the Nodes and Lilith chooser.
	BindingNodes
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

	// BindingSessions is `<leader> O`: the Sessions list. BindingSessionDelete
	// is Ctrl+D there: move the highlighted session to the trash.
	BindingSessions
	BindingSessionDelete

	// BindingNewSession is a new, unnamed session: the Sessions list's second row
	// runs it, and it has no key. It is appended so the ids above keep their
	// numbers.
	BindingNewSession

	// BindingActions is Ctrl+P: the Keybindings popup, opened
	// at the commands.
	BindingActions

	// BindingConsole is the Log console, opened directly: `l` while the Log has
	// the focus, and the Actions row Log console. BindingLogConsole is `<leader> L`,
	// which now gives the Log the focus. Appended so the ids above keep their
	// numbers.
	BindingConsole

	// BindingOriginals is the folder row of Keybindings: protect the charts folder,
	// or open it again. Appended so the ids above keep their numbers.
	BindingOriginals

	// The choice popup's own keys (dialog.Choice): left and right move the lit
	// button, y answers Yes. Enter, a and n/esc are BindingConfirm, BindingToggle
	// and BindingClose there. Appended so the ids above keep their numbers.
	BindingChoosePrev
	BindingChooseNext
	BindingYes

	// BindingNo is n in the new-session question: No. Appended so the ids above
	// keep their numbers.
	BindingNo

	// The back key of each time step and of the Moon phase jump: the upper case
	// letter is an action of its own, with its own row. Appended so the ids above
	// keep their numbers.
	BindingTimeStepYearBack
	BindingTimeStepMonthBack
	BindingTimeStepWeekBack
	BindingTimeStepDayBack
	BindingTimeStepHourBack
	BindingTimeStepMinuteBack
	BindingTimeStepSecondBack
	BindingMoonPhasePrev
)

// BindingNone is what matchBinding returns for a key no entry matches.
const BindingNone BindingID = -1

// Group is the section of the Keybindings popup an entry belongs to. It is
// declared once, on the entry; the popup and Validate both read it.
type Group int

const (
	// GroupNone is an entry that declared no group: Validate reports it.
	GroupNone Group = iota
	// GroupLocal is a key of the place the popup was opened from: a panel, a
	// tab, or the popup you were in.
	GroupLocal
	// GroupGlobal is a command, with its Ctrl key or its comma form, and the
	// keys that work everywhere (the Keybindings popup, quit).
	GroupGlobal
	// GroupNavigation is a key that moves: through time, between panels,
	// along the rows of a list.
	GroupNavigation
)

// String is the section's name as the popup's header spells it.
func (g Group) String() string {
	switch g {
	case GroupLocal:
		return "Local"
	case GroupGlobal:
		return "Global"
	case GroupNavigation:
		return "Navigation"
	}
	return "none"
}

// BindingEntry is one row of an owner's table.
type BindingEntry struct {
	ID BindingID
	// Group is the entry's section in the Keybindings popup.
	Group Group
	// Binding holds the keys the entry matches. Its Help() is the Keybindings
	// popup's spelling of the key (`<left>/<right>`) and the description it lists.
	Binding key.Binding
	// Title is the key's one name, written once: the popup shows it as is
	// (`Move chart up`) and the bar shortens it by barWord. TitleFn, if set,
	// is the title for the model's state (Enter: Timeline or Report) and Title
	// is its resting value. The one method that reads them is title.
	Title   string
	TitleFn func(Context) string
	// BarKey is the key as the bar spells it (`right`). HelpKey, if set, is the
	// spelling the popup gives the key; without it the popup uses the binding's own
	// spelling (the main table), then BarKey. No Log line names a key: the bar
	// is the one place a key is explained (the specification Part B).
	BarKey  string
	HelpKey string
	// BarTitle, if set, is the title the bar shortens into the entry's word,
	// where the bar names two keys of two actions under one word (`Move: up/down`)
	// and the popup lists each key under its own title.
	BarTitle string
	// Word is the bar's word for the entry, set by withWords from the title
	// and the owner; it is what the key bar draws.
	Word string
	// Bar puts the entry in the key bar, in table order. Pinned keeps it
	// there when the bar is too narrow for every entry (C4).
	Bar    bool
	Pinned bool
	// Kept is a bar entry that gives way only after every other unpinned entry
	// has: it stays beside the pinned ones as long as it fits at all.
	Kept bool
	// BarWhen, if set, is the condition under which a Bar entry is drawn; the
	// entry matches its key either way. BarKeyFn, if set, is the key as the bar
	// spells it for the model's state, with BarKey as its resting value.
	BarWhen  func(Context) bool
	BarKeyFn func(Context) string
	// Help lists the entry in the Keybindings popup.
	Help bool
	// When, if set, is the condition under which the entry is available.
	// An unavailable entry neither matches nor appears in the bar.
	When func(Context) bool
	// HelpWhen, if set, is the condition under which the Keybindings popup
	// lists the entry. It is separate from When, which is whether the key acts:
	// the popup lists a key while it cannot act (drawn faint), and only the
	// chart list's keys wait for the Chart panel and an open chart.
	HelpWhen func(Context) bool
	// Palette marks the entry as a command: it runs by its id, from the popup
	// and from its key alike, and the popup keeps the commands last chosen on
	// top of Global. Motion, paging, tab switching and the time steps are not
	// commands.
	Palette bool
	// Chord marks an entry whose action is also a chord of the leader that the
	// owner runs: an owner that takes only some of the chords (a popup's filter, a
	// search line, the console) offers and lists exactly the entries marked here.
	Chord bool
	// Opens marks a command that hands the keyboard to a chooser, a list or a
	// prompt. The Keybindings popup ends its description in `…`. One test runs
	// every command and holds the field to what happened.
	Opens bool
}

// KeyMap is the main screen's keys, one binding per action.
type KeyMap struct {
	Quit           key.Binding
	ForceQuit      key.Binding
	CancelLeader   key.Binding
	NextView       key.Binding
	PrevView       key.Binding
	FocusSession   key.Binding
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
	TimeStepSecond key.Binding
	MoonPhase      key.Binding
	// The back keys: the upper case letter of each step is its own action.
	TimeStepYearBack   key.Binding
	TimeStepMonthBack  key.Binding
	TimeStepWeekBack   key.Binding
	TimeStepDayBack    key.Binding
	TimeStepHourBack   key.Binding
	TimeStepMinuteBack key.Binding
	TimeStepSecondBack key.Binding
	MoonPhasePrev      key.Binding
	RenderWheel        key.Binding
	JumpNow            key.Binding
	ScrollUp           key.Binding
	ScrollDown         key.Binding
	PageUp             key.Binding
	PageDown           key.Binding
	CursorHome         key.Binding
	CursorEnd          key.Binding
	Aspects            key.Binding
	Bodies             key.Binding
	Houses             key.Binding
	Zodiac             key.Binding
	Nodes              key.Binding
	Attach             key.Binding
	TabPrev            key.Binding
	TabNext            key.Binding
	Actions            key.Binding
	Copy               key.Binding
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
			key.WithHelp(Spell("ctrl+c"), ""),
		),
		CancelLeader: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp(Spell("esc"), ""),
		),
		// Panel navigation. `l`/`h` were provisional lazygit-style nav
		// removed 2026-06-03 when an earlier change reclaimed `h`/`H` for the
		// canonical time-step hour key (the design notes:119).
		// The bar names the arrow keys in words; the Keybindings popup spells them.
		NextView: key.NewBinding(
			key.WithKeys("tab", "right"),
			key.WithHelp(Spell("right"), ""),
		),
		PrevView: key.NewBinding(
			key.WithKeys("shift+tab", "left"),
			key.WithHelp(Spell("left"), ""),
		),
		// The ring digits. `0` is Details' digit: Details is not in the focus
		// ring, so `0` cycles its tabs from anywhere instead of focusing it
		// (D14).
		FocusSession: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", ""),
		),
		FocusChart: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", ""),
		),
		FocusBodies: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", ""),
		),
		FocusAspects: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", ""),
		),
		DetailsTab: key.NewBinding(
			key.WithKeys("0"),
			key.WithHelp("0", ""),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", ""),
		),
		OpenAtlas: ctrlBinding(BindingOpenAtlas),
		// Ctrl+O opens the saved-chart library picker. Selecting a
		// chart sets it as the natal anchor and recomputes the overlay.
		OpenCharts: ctrlBinding(BindingOpenCharts),
		// The leader sequence reaches the same action for a terminal that
		// eats ctrl+s for flow control.
		SaveChart: ctrlBinding(BindingSaveChart),
		Select: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp(Spell(" "), ""),
		),
		SlotCycleUp: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp(Spell("up"), ""),
		),
		SlotCycleDown: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp(Spell("down"), ""),
		),
		// The workspace list (the specification M4, M5). bubbletea reports LF as
		// K / J and Shift+↑ / Shift+↓ move the chart under the cursor; bare k and
		// j stay row movement.
		SlotMoveUp: key.NewBinding(
			key.WithKeys("K", "shift+up"),
			key.WithHelp("K", ""),
		),
		SlotMoveDown: key.NewBinding(
			key.WithKeys("J", "shift+down"),
			key.WithHelp("J", ""),
		),
		SlotDelete: key.NewBinding(
			key.WithKeys("delete"),
			key.WithHelp(Spell("delete"), ""),
		),
		// Time-step keys per the design notes:113-122. Lowercase
		// = forward, uppercase = backward. `o`/`O` takes month precisely so
		// that `m`/`M` stay free for the minute step, which pitch.md:122 has
		// promised since before v1 and which rectification work needs: the
		// finest step the other keys do not offer.
		TimeStepYear: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", ""),
		),
		TimeStepYearBack: key.NewBinding(
			key.WithKeys("Y"),
			key.WithHelp("Y", ""),
		),
		TimeStepMonth: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", ""),
		),
		TimeStepMonthBack: key.NewBinding(
			key.WithKeys("O"),
			key.WithHelp("O", ""),
		),
		TimeStepWeek: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", ""),
		),
		TimeStepWeekBack: key.NewBinding(
			key.WithKeys("W"),
			key.WithHelp("W", ""),
		),
		TimeStepDay: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", ""),
		),
		TimeStepDayBack: key.NewBinding(
			key.WithKeys("D"),
			key.WithHelp("D", ""),
		),
		TimeStepHour: key.NewBinding(
			key.WithKeys("h"),
			key.WithHelp("h", ""),
		),
		TimeStepHourBack: key.NewBinding(
			key.WithKeys("H"),
			key.WithHelp("H", ""),
		),
		TimeStepMinute: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", ""),
		),
		TimeStepMinuteBack: key.NewBinding(
			key.WithKeys("M"),
			key.WithHelp("M", ""),
		),
		TimeStepSecond: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", ""),
		),
		TimeStepSecondBack: key.NewBinding(
			key.WithKeys("S"),
			key.WithHelp("S", ""),
		),
		// p / P jump the floating chart to the next / previous Moon phase.
		MoonPhase: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", ""),
		),
		MoonPhasePrev: key.NewBinding(
			key.WithKeys("P"),
			key.WithHelp("P", ""),
		),
		// Ctrl+X renders the focused slot's chart as a wheel image and serves
		// it (or hands it to the viewer); a second press closes or refreshes it.
		RenderWheel: ctrlBinding(BindingRenderWheel),
		// `n` jumps the floating slot to now and makes it follow the clock, from
		// any cursor position.
		JumpNow: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", ""),
		),
		// Ctrl+PgUp/PgDn scroll the Details report two lines at a time — lazygit's
		// `scrollHeight: 2`. They act on Details from wherever focus is:
		// Details is not focusable, so no other panel competes for them. A
		// modifier pair because the plain keys page the focused list.
		ScrollUp: key.NewBinding(
			key.WithKeys("ctrl+pgup"),
			key.WithHelp(Spell("ctrl+pgup"), ""),
		),
		ScrollDown: key.NewBinding(
			key.WithKeys("ctrl+pgdown"),
			key.WithHelp(Spell("ctrl+pgdown"), ""),
		),
		// ⇞ ⇟ Home End (and g G) move the cursor of the focused list: first to
		// the edge of its window, then by a page of the window minus one row.
		// Where the focused panel has no cursor, ⇞ ⇟ still scroll Details.
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp(Spell("pgup"), ""),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp(Spell("pgdown"), ""),
		),
		// g and G are Home and End where the letter is not typed as text. A
		// configured leader is consumed before this table (model.go), so a
		// leader of g keeps the leader and loses the jump; Home and End work.
		CursorHome: key.NewBinding(
			key.WithKeys("home", "g"),
			key.WithHelp(Spell("home")+"/g", ""),
		),
		CursorEnd: key.NewBinding(
			key.WithKeys("end", "G"),
			key.WithHelp(Spell("end")+"/G", ""),
		),
		// ⌃a opens the aspect chooser; the body selector takes ⌃b. Plain
		// letters were the first choice and were wrong: every letter is
		// also a time-step or a panel key somewhere, and the modifier is
		// what keeps a chooser from competing with the keys an operator
		// uses continuously while reading a chart.
		Aspects: ctrlBinding(BindingAspects),
		Bodies:  ctrlBinding(BindingBodies),
		// Houses is ⌃u and Zodiac is ⌃z. Bubble Tea delivers ⌃z as an ordinary key
		// and suspends only on a tea.Suspend this program never returns
		// (TestCtrlZDoesNotSuspend).
		Houses: ctrlBinding(BindingHouses),
		Zodiac: ctrlBinding(BindingZodiac),
		Nodes:  ctrlBinding(BindingNodes),
		// Enter attaches Details: row navigation moves inside it and it owns
		// the keyboard until Esc (the specification D12, D18).
		Attach: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp(Spell("enter"), ""),
		),
		// `[`/`]` cycle the focused view's tabs: Details while attached, the
		// focused ring panel otherwise, which today has one tab (C34, C36).
		TabPrev: key.NewBinding(
			key.WithKeys("["),
			key.WithHelp("[", ""),
		),
		TabNext: key.NewBinding(
			key.WithKeys("]"),
			key.WithHelp("]", ""),
		),
		// ⌃p opens the Keybindings popup at the commands.
		Actions: key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp(Spell("ctrl+p"), "")),
		// ⌃y copies what `, y` copies.
		Copy: ctrlBinding(BindingCopy),
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

// InGroup gives the entries that declared no group the group g: a table whose
// entries all belong to one section (a popup's own keys are all Local) says so
// once.
func InGroup(g Group, entries []BindingEntry) []BindingEntry {
	for i := range entries {
		if entries[i].Group == GroupNone {
			entries[i].Group = g
		}
	}
	return entries
}

// TitledLocal is Titled for a popup's table: every entry is Local to it.
func TitledLocal(entries []BindingEntry) []BindingEntry {
	return Titled(InGroup(GroupLocal, entries))
}

// Titled gives every entry's binding its help text: the key's spelling (the
// entry's HelpKey, else the one the binding already carries, else the binding's keys spelled, else BarKey) and
// the title as the description. Nothing writes a description by hand, so the
// popup and the bar cannot name a key differently.
func Titled(entries []BindingEntry) []BindingEntry {
	for i := range entries {
		e := &entries[i]
		spelling := e.HelpKey
		if spelling == "" {
			spelling = e.Binding.Help().Key
		}
		if spelling == "" {
			spelling = Spell(e.Binding.Keys()...)
		}
		if spelling == "" {
			spelling = e.BarKey
		}
		e.Binding.SetHelp(spelling, e.Title)
	}
	return entries
}

// TitleFor is the entry's title for this context: TitleFn when it has one (Enter
// names the Details tab it will show), else Title. The bar and the
// Keybindings popup both read it here.
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
	OwnerConsole
	OwnerDetails
	OwnerSessions
	OwnerSessionName
	// OwnerKeybindings is the Keybindings popup, over whatever it was opened
	// from.
	OwnerKeybindings
	// OwnerLog is the Log panel with the keyboard: a mode beside the console,
	// never the focused panel. Appended so the owners above keep their numbers.
	OwnerLog
	// OwnerExperiments is the hidden Experiments popup (`,L p`). Appended so the
	// owners above keep their numbers.
	OwnerExperiments
	// OwnerNodes is the Nodes and Lilith chooser. Appended so the owners above
	// keep their numbers.
	OwnerNodes
)

// Variant is which of an owner's tables is in force: the chart-title popup has
// one while it asks to confirm, attached Details has one for each search line.
type Variant int

const (
	VariantDefault Variant = iota
	VariantTitleConfirm
	VariantTimelineSearch
	VariantReportSearch
	// VariantSaveAsk is the save prompt while it asks its first-write question.
	VariantSaveAsk
	// VariantNewSessionAsk is the Sessions list while it asks a question: to save the
	// unnamed session before a new one starts, or to trash a session.
	VariantNewSessionAsk
)
