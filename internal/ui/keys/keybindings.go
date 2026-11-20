package keys

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
)

// keybindings.go is the Keybindings popup's model: the rows it lists for the
// place it was opened from, the filter over them, and the lines they make
// (headers, empty lines, the separator under the recent commands). It draws
// nothing; the ui package draws it in the shared popup box.

// Row is one key of the popup: what the cursor can stand on.
type Row struct {
	ID    BindingID
	Group Group
	// Key is the key as the popup spells it (`<ctrl+o>`, `, O`, `y/Y`).
	Key string
	// Keys are the key names Bubble Tea gives the entry's binding. The first
	// is what Enter presses for a row that is not a command.
	Keys []string
	// Desc is the entry's title for the place the popup was opened from.
	Desc string
	// Off is a row that cannot run where the popup was opened from. A
	// command's answer can change while the popup is open, so the ui package
	// asks again for those (Command).
	Off bool
	// Command is a row that runs by its id, as its key would.
	Command bool
	// Recent is a command last chosen: it leads Global.
	Recent bool
	// Opens is a command that hands the keyboard to a chooser, a list or a
	// prompt. The popup writes `…` after its description.
	Opens bool
}

// KbInput is what the popup's rows are made from.
type KbInput struct {
	// Origin is the owner that had the keyboard when the popup opened, with
	// its table and the answers to its conditions. For the main screen the
	// table is the main table.
	Origin      Owner
	OriginTable []BindingEntry
	OriginCtx   Context
	// Main and LeaderOnly are the main screen's tables, and MainCtx the
	// answers to their conditions.
	Main       []BindingEntry
	LeaderOnly []BindingEntry
	MainCtx    Context
	// Recent are the commands last chosen, most recent first.
	Recent []BindingID
}

// globalOrder and navigationOrder are the order of the rows of those two
// sections: the rest of the table follows, in its own order. Navigation reads
// the time keys first, then the panels, the rows and paging.
var globalOrder = []BindingID{
	BindingOpenCharts, BindingSaveChart, BindingOpenAtlas, BindingHouses, BindingZodiac,
	BindingNodes, BindingAspects, BindingBodies, BindingRenderWheel, BindingCopy, BindingSessions,
	BindingLogConsole, BindingSaveCity, BindingOriginals, BindingConsole, BindingActions,
	BindingHelp, BindingQuit, BindingForceQuit,
}

var navigationOrder = []BindingID{
	BindingTimeStepYear, BindingTimeStepYearBack, BindingTimeStepMonth, BindingTimeStepMonthBack,
	BindingTimeStepWeek, BindingTimeStepWeekBack, BindingTimeStepDay, BindingTimeStepDayBack,
	BindingTimeStepHour, BindingTimeStepHourBack, BindingTimeStepMinute, BindingTimeStepMinuteBack,
	BindingTimeStepSecond, BindingTimeStepSecondBack, BindingMoonPhase, BindingMoonPhasePrev, BindingJumpNow,
	BindingNextView, BindingPrevView, BindingFocusSession, BindingFocusChart, BindingFocusBodies,
	BindingFocusAspects, BindingDetailsTab, BindingSlotCycleUp, BindingSlotCycleDown, BindingListUp,
	BindingListDown, BindingPageUp, BindingPageDown, BindingPageHome, BindingPageEnd,
	BindingScrollUp, BindingScrollDown,
}

// rank is the place of id in order, or past its end.
func rank(order []BindingID, id BindingID) int {
	if i := slices.Index(order, id); i >= 0 {
		return i
	}
	return len(order)
}

// isCommand is an entry that runs by its id: the commands, and the two
// doors, the popup itself.
func isCommand(e BindingEntry) bool {
	return e.Palette || e.ID == BindingHelp || e.ID == BindingActions
}

// rowKey is the key the popup prints for an entry: a command's Ctrl key or its
// comma form, else the key the binding spells.
func rowKey(c Context, e BindingEntry) string {
	if e.ID == BindingConsole {
		return strings.TrimSpace(c.Leader.Key + " L l")
	}
	if k := ActionKey(c, e); k != "" {
		return k
	}
	if k := e.Binding.Help().Key; k != "" {
		return k
	}
	return e.BarKey
}

// KeybindingRows are the rows of the popup for the place it was opened from,
// in the order they are drawn: Local, Global, Navigation.
//
// Local is the origin's own table: a panel's keys, the Timeline's, a
// chooser's. Global is every command and the keys that work everywhere, read
// from the main screen's tables. Navigation is the keys that move: the main
// screen's, or the Details' own while Details has the keyboard. Where the
// popup was opened over a chooser the commands and the main screen's movement
// cannot run, and those rows are faint.
func KeybindingRows(in KbInput) []Row {
	main := in.Origin == OwnerMain
	away := !main && in.Origin != OwnerDetails

	row := func(c Context, e BindingEntry, g Group) Row {
		return Row{
			ID: e.ID, Group: g, Key: rowKey(c, e), Keys: e.Binding.Keys(),
			Desc: e.TitleFor(c), Command: isCommand(e), Opens: e.Opens,
		}
	}
	listed := func(c Context, e BindingEntry, g Group, fromMain bool) bool {
		if e.Group != g || e.TitleFor(c) == "" {
			return false
		}
		return !fromMain || e.Help || e.Palette
	}
	seen := map[BindingID]bool{}
	var local, global, nav []Row

	for _, e := range in.OriginTable {
		if !listed(in.OriginCtx, e, GroupLocal, main) || seen[e.ID] {
			continue
		}
		if e.HelpWhen != nil && !e.HelpWhen(in.OriginCtx) {
			continue
		}
		r := row(in.OriginCtx, e, GroupLocal)
		r.Off = e.When != nil && !e.When(in.OriginCtx)
		seen[e.ID] = true
		local = append(local, r)
	}

	for _, e := range slices.Concat(in.Main, in.LeaderOnly) {
		if !listed(in.MainCtx, e, GroupGlobal, true) || seen[e.ID] {
			continue
		}
		r := row(in.MainCtx, e, GroupGlobal)
		r.Off = away && e.ID != BindingForceQuit
		seen[e.ID] = true
		global = append(global, r)
	}
	slices.SortStableFunc(global, func(a, b Row) int { return rank(globalOrder, a.ID) - rank(globalOrder, b.ID) })
	var recent []Row
	for _, id := range in.Recent {
		for i, r := range global {
			if r.ID == id && r.Command && !r.Recent {
				r.Recent = true
				recent = append(recent, r)
				global = slices.Delete(global, i, i+1)
				break
			}
		}
	}
	global = append(recent, global...)

	if in.Origin == OwnerDetails {
		for _, e := range in.OriginTable {
			if !listed(in.OriginCtx, e, GroupNavigation, false) || seen[e.ID] {
				continue
			}
			seen[e.ID] = true
			r := row(in.OriginCtx, e, GroupNavigation)
			r.Off = e.When != nil && !e.When(in.OriginCtx)
			nav = append(nav, r)
		}
	} else {
		for _, e := range in.Main {
			if !listed(in.MainCtx, e, GroupNavigation, true) || seen[e.ID] {
				continue
			}
			if e.HelpWhen != nil && !e.HelpWhen(in.MainCtx) {
				continue
			}
			seen[e.ID] = true
			r := row(in.MainCtx, e, GroupNavigation)
			r.Off = away || (e.When != nil && !e.When(in.MainCtx))
			nav = append(nav, r)
		}
	}
	slices.SortStableFunc(nav, func(a, b Row) int { return rank(navigationOrder, a.ID) - rank(navigationOrder, b.ID) })

	return slices.Concat(local, global, nav)
}

// OwnerTitle is the name of the popup a chooser's Keybindings is opened over,
// for the title (`Keybindings · Houses`); "" for the main screen and Details.
func OwnerTitle(o Owner) string {
	switch o {
	case OwnerCity:
		return dialog.TitleCity
	case OwnerOpenChart:
		return dialog.TitleOpenChart
	case OwnerBodies:
		return dialog.TitleBodies
	case OwnerAspects:
		return dialog.TitleAspects
	case OwnerHouses:
		return dialog.TitleHouses
	case OwnerZodiac:
		return dialog.TitleZodiac
	case OwnerNodes:
		return dialog.TitleNodes
	case OwnerSessions:
		return dialog.TitleSessions
	case OwnerExperiments:
		return dialog.TitleExperiments
	case OwnerLog:
		return dialog.TitleLog
	case OwnerConsole:
		return dialog.TitleLogConsole
	}
	return ""
}

// ZodiacNote is what the Keybindings popup says about the ayanamsas the zodiac
// chooser offers: each is a fixed offset from Fagan-Bradley. It closes the list
// when the popup is opened over that chooser, and each line is short enough for
// a popup at 80 columns.
var ZodiacNote = []string{
	"Zodiac: every sidereal zodiac stands a fixed distance",
	"from Fagan-Bradley. Each chart is turned by the",
	"precession of its own date. So two charts of different",
	"dates stand a little differently to each other than in",
	"the tropical zodiac.",
}

// FilterPrompt leads the filter line at the popup's foot.
const FilterPrompt = "Find (start with @ to find by key): "

// KbState is the popup while it is open. The list machinery under it is the
// chassis's (typing, the cursor); its rows are Matches, the rows that pass the
// filter, and the lines they make are Lines.
type KbState struct {
	menu.FuzzyList[Row]
	// All is every row of the place the popup was opened from, whatever is typed.
	All []Row
	// Top is the first visible line.
	Top int
	// Origin is the owner the popup was opened over.
	Origin Owner
	// Title is the frame's title.
	Title string
	// Notes close the list.
	Notes []string
	// KeyW is the key column's width.
	KeyW int
	// Width and Rows are the box's size, decided once when the popup opens
	// (the box does not change size while the popup is open).
	Width, Rows int
}

// MinKeyW is the narrowest the key column is drawn.
const MinKeyW = 7

// NewKbState is the popup for these rows, nothing typed, the cursor on the
// first row.
func NewKbState(rows []Row, origin Owner, notes []string) KbState {
	st := KbState{All: rows, Origin: origin, Notes: notes, KeyW: MinKeyW}
	st.Title = "Keybindings"
	if t := OwnerTitle(origin); t != "" {
		st.Title += " · " + t
	}
	for _, r := range rows {
		st.KeyW = max(st.KeyW, ansi.StringWidth(r.Key))
	}
	st.Matches = rows
	st.Rows = len(st.Lines())
	return st
}

// Refilter sets the rows that pass the typed text and puts the cursor on the
// first of them.
func (st *KbState) Refilter() {
	st.Matches = FilterRows(st.All, st.FilterText)
	st.Cursor = 0
	st.Top = 0
}

// FilterRows are the rows a filter keeps. Plain text searches the
// descriptions, ignoring case. A leading `@` searches the keys instead: how a
// row spells its key (`<ctrl+z>`), or the names Bubble Tea gives it (`ctrl+z`). A key
// search typed with a capital is exact about case, so `@K` finds K and not k.
func FilterRows(rows []Row, text string) []Row {
	text = strings.TrimSpace(text)
	if text == "" {
		return rows
	}
	byKey := strings.HasPrefix(text, "@")
	q := strings.TrimSpace(strings.TrimPrefix(text, "@"))
	exact := byKey && q != strings.ToLower(q)
	if !exact {
		q = strings.ToLower(q)
	}
	fold := func(s string) string {
		if exact {
			return s
		}
		return strings.ToLower(s)
	}
	var out []Row
	for _, r := range rows {
		hay := []string{r.Desc}
		if byKey {
			hay = append([]string{r.Key}, r.Keys...)
		}
		for _, h := range hay {
			if strings.Contains(fold(h), q) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// LineKind says what a line of the popup is.
type LineKind int

const (
	// LineNote is plain text: the closing note.
	LineNote LineKind = iota
	// LineHeader is a group's header.
	LineHeader
	// LineBlank is the empty line that ends a group.
	LineBlank
	// LineRow is a row; Row indexes Matches.
	LineRow
)

// Line is one line of the popup's list.
type Line struct {
	Kind LineKind
	Text string
	Row  int
}

// Lines are the lines of the rows that pass the filter: each group that has a
// row under its header, an empty line between two groups, and the closing
// notes after an empty line. The list never starts or ends on an empty line. A
// group with no match is not drawn.
func (st KbState) Lines() []Line {
	var out []Line
	typed := strings.TrimSpace(st.FilterText) != ""
	for _, g := range []Group{GroupLocal, GroupGlobal, GroupNavigation} {
		var idx []int
		for i, r := range st.Matches {
			if r.Group == g {
				idx = append(idx, i)
			}
		}
		if len(idx) == 0 {
			continue
		}
		if len(out) > 0 {
			out = append(out, Line{Kind: LineBlank})
		}
		out = append(out, Line{Kind: LineHeader, Text: "─── " + g.String()})
		for _, i := range idx {
			out = append(out, Line{Kind: LineRow, Row: i})
		}
	}
	if !typed && len(st.Notes) > 0 && len(out) > 0 {
		out = append(out, Line{Kind: LineBlank})
	}
	if !typed {
		for _, n := range st.Notes {
			out = append(out, Line{Kind: LineNote, Text: n})
		}
	}
	return out
}

// lineOf is the line the row at index sits on, and the line of its group's
// header.
func lineOf(lines []Line, row int) (at, header int) {
	for i, l := range lines {
		switch {
		case l.Kind == LineHeader:
			header = i
		case l.Kind == LineRow && l.Row == row:
			return i, header
		}
	}
	return 0, 0
}

// Reveal moves the window the least that keeps the cursor's row on screen,
// with its group's header when the cursor is on the group's first row.
func (st *KbState) Reveal(height int) {
	lines := st.Lines()
	if len(st.Matches) == 0 {
		st.Top = 0
		return
	}
	at, header := lineOf(lines, st.Cursor)
	st.Top = menu.ScrollToShow(st.Top, at, len(lines), height)
	if at-header == 1 {
		st.Top = menu.ScrollToShow(st.Top, header, len(lines), height)
	}
	switch st.Cursor {
	case 0:
		st.Top = 0
	case len(st.Matches) - 1:
		// The last row: whatever closes the list (a note) shows with it.
		st.Top = max(len(lines)-height, st.Top)
	}
}

// ShowGroup puts the cursor on the group's first row and its header at the top
// of the window: how the popup opens at the commands.
func (st *KbState) ShowGroup(g Group, height int) {
	for i, r := range st.Matches {
		if r.Group != g {
			continue
		}
		st.Cursor = i
		_, header := lineOf(st.Lines(), i)
		st.Top = max(min(header, len(st.Lines())-height), 0)
		return
	}
}

// MoveBy puts the cursor n rows on, stopping at the ends, and keeps it shown.
func (st *KbState) MoveBy(n, height int) {
	if len(st.Matches) == 0 {
		return
	}
	st.Cursor = min(max(st.Cursor+n, 0), len(st.Matches)-1)
	st.Reveal(height)
}
