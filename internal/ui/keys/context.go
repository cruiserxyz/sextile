package keys

import (
	"time"

	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// Context is everything one frame's keys need to know about the program: which
// owner has the keyboard, that owner's table, the answers to the conditions
// its entries ask, and what the bar and the Keys popup draw with. The ui
// package resolves it from the Model (keysContext) and resolves only the
// active owner's answers; every other group is its zero value, so an entry's
// condition is a read of one field and never reaches into the program.
type Context struct {
	// Owner and Variant say whose table is in force; Table is that table.
	Owner   Owner
	Variant Variant
	Table   []BindingEntry

	// Availability, one group per owner. Only the owner's own group is filled
	// (Main is also filled, in part, for Help, which lists the main table).
	Main     MainFacts
	Details  DetailsFacts
	Popup    PopupFacts
	Picker   PickerFacts
	Sessions SessionsFacts
	Save     SaveFacts
	Console  ConsoleFacts
	Log      LogFacts
	Display  DisplayFacts

	// Attach is Enter on the main screen: whether it opens Details and the
	// title it carries (the tab it would show).
	Attach AttachFacts

	// Leader is the leader's configuration and pending state.
	Leader LeaderFacts

	// Help is what the Keys popup lists, filled for its owner only.
	Help HelpFacts

	// Presentation: the terminal's style, and the footer's right-hand slot as
	// ui finished it (a pending leader, a copy receipt, a notice, the error
	// mark), with whether it is one of the notices, which the bar must not
	// cut as it cuts a leader.
	Style        term.Style
	Footer       string
	FooterNotice bool
}

// MainFacts are the main table's conditions.
type MainFacts struct {
	Selects        bool // Space acts: a chart slot or a Bodies row can be selected
	HasWorkspace   bool // there is a workspace to step
	Moves          bool // the focused list has somewhere to go
	Pages          bool // the focused panel owns a cursor with a row
	CanMoveUp      bool
	CanMoveDown    bool
	CanRemove      bool
	DetailsScrolls bool // Details is on screen with more than it can show
	HoldsAChart    bool // at least one chart is open (the Keys popup lists the list keys)
}

// DetailsFacts are attached Details' conditions.
type DetailsFacts struct {
	Moves          bool
	Scrolls        bool
	OnTimeline     bool
	OnTimelineRows bool
	Filterable     bool
	HasSeveralTabs bool
}

// PopupFacts are the open popup list's: how many rows it holds right now.
type PopupFacts struct{ Rows int }

// PickerFacts are the chart picker's.
type PickerFacts struct{ Renames bool }

// SessionsFacts are the Sessions list's and its name form's.
type SessionsFacts struct {
	OnSaveRow bool
	CanOpen   bool
	OnFreeRow bool
	CanTrash  bool
	NameTyped bool
}

// SaveFacts are the Save popup's.
type SaveFacts struct{ HasName bool }

// ConsoleFacts are the console's: how many lines its snapshot holds.
type ConsoleFacts struct{ Lines int }

// LogFacts are the focused Log's: how many lines the panel holds.
type LogFacts struct{ Lines int }

// DisplayFacts are the House display popup's.
type DisplayFacts struct{ CanChoose bool }

// AttachFacts are Enter's on the main screen.
type AttachFacts struct {
	CanAttach bool
	Title     string
}

// LeaderFacts are the leader's key, its pending sequence, the sequences it
// knows (the Keys popup lists them), the prefix tree and the timeout.
type LeaderFacts struct {
	Pending  bool
	Seq      []string
	Key      string
	Bindings []LeaderBinding
	Tree     *LeaderNode
	Timeout  time.Duration
}

// HelpFacts are the Keys popup's: the main table, the leader-only entries, the
// table of each popup it lists, and the scroll position.
type HelpFacts struct {
	Main       []BindingEntry
	LeaderOnly []BindingEntry
	Popups     map[Owner][]BindingEntry
	State      HelpState
}
