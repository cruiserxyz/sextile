package keys

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// help.go is the Keys popup: an ordinary popup list, drawn by the same frame
// as every other popup, scrolling inside it, with a cursor that walks the
// bindings (the specification C5–C5b). It used to be its own
// box sized to its content, which ran off the top and bottom of the screen even
// at 132x40.
//
// What it lists is piece 4's to change; this file only gives it a frame,
// a cursor and a position.

// HelpLine is one line of the help list. Only binding lines are positions the
// cursor can take; a heading and the blank line before it are notes. Header
// marks the section titles (the popup draws them bold); the closing zodiac
// note is not one.
type HelpLine struct {
	Text    string
	Binding bool
	Header  bool
}

// HelpZodiacNote is what the Keys popup says about the ayanamsas the finder
// offers (the specification C9). It closes the list, and each line is short enough for any popup at 80 columns
// (the Help box is as wide as its widest line, this note's, 54 cells): it is
// never clipped.
var HelpZodiacNote = []string{
	"Zodiac: Astrolog's ayanamsas are fixed offsets from",
	"Fagan-Bradley. Each chart is precessed to its own",
	"date, so a sidereal comparison's pair differs from the",
	"tropical pair by the precession between the two dates.",
}

// helpGroup is one titled block of the Help reference.
type helpGroup struct {
	title string
	ids   []BindingID
}

// helpGroups is the Help reference in reading order: what a newcomer needs
// first, then one block per task. Help itself and the main screen's q are not
// listed: inside Help, Esc and q close it, and the key bar says so.
var helpGroups = []helpGroup{
	{"Start here", []BindingID{
		BindingNextView, BindingSlotCycleUp, BindingAttach,
		BindingOpenCharts, BindingOpenAtlas, BindingActions, BindingForceQuit,
	}},
	{"Move", []BindingID{
		BindingFocusChart,
		BindingSelect, BindingSlotMoveUp, BindingSlotMoveDown, BindingSlotDelete,
		BindingPageUp, BindingPageHome, BindingPageEnd,
	}},
	{"Time (lowercase forward, uppercase back)", []BindingID{
		BindingTimeStepYear, BindingTimeStepMonth, BindingTimeStepWeek, BindingTimeStepDay,
		BindingTimeStepHour, BindingTimeStepMinute, BindingJumpNow, BindingToggleBack,
	}},
	{"Views", []BindingID{
		BindingDetailsTab, BindingTabPrev, BindingScrollUp,
		BindingBodies, BindingAspects, BindingHouses, BindingZodiac,
	}},
	{"Output", []BindingID{BindingSaveChart, BindingRenderWheel}},
}

// HelpUnlisted are main-table entries Help leaves out on purpose.
var HelpUnlisted = map[BindingID]bool{BindingHelp: true, BindingQuit: true}

// HelpSection is a titled run of the main table's entries.
type HelpSection struct {
	Title   string
	Entries []BindingEntry
}

// HelpSections puts the main table's help entries into the Help groups; an
// entry in no group and not left out on purpose lands in a final "More" group,
// so a new binding is never silently missing from Help.
func HelpSections(entries []BindingEntry) []HelpSection {
	byID := map[BindingID]BindingEntry{}
	for _, e := range entries {
		byID[e.ID] = e
	}
	var out []HelpSection
	seen := map[BindingID]bool{}
	for _, g := range helpGroups {
		s := HelpSection{Title: g.title}
		for _, id := range g.ids {
			if e, ok := byID[id]; ok {
				s.Entries = append(s.Entries, e)
				seen[id] = true
			}
		}
		out = append(out, s)
	}
	more := HelpSection{Title: "More"}
	for _, e := range entries {
		if !seen[e.ID] && !HelpUnlisted[e.ID] {
			more.Entries = append(more.Entries, e)
		}
	}
	return append(out, more)
}

// HelpPair is one Help row: a title and the keys' spelling. HelpBlock is a
// titled run of them.
type HelpPair struct{ Title, Key string }

type HelpBlock struct {
	Title string
	Rows  []HelpPair
}

// popupHelp names a popup by the key that opens it: the opener's id, the
// owner whose table the popup uses and the popup's own title.
var popupHelp = map[BindingID]struct {
	owner Owner
	title string
}{
	BindingOpenAtlas:    {OwnerCity, dialog.TitleCity},
	BindingOpenCharts:   {OwnerOpenChart, dialog.TitleOpenChart},
	BindingSaveChart:    {OwnerSaveChart, dialog.TitleSaveChart},
	BindingBodies:       {OwnerBodies, dialog.TitleBodies},
	BindingHouses:       {OwnerHouses, dialog.TitleHouses},
	BindingZodiac:       {OwnerZodiac, dialog.TitleZodiac},
	BindingAspects:      {OwnerAspects, dialog.TitleAspects},
	BindingLogConsole:   {OwnerLog, dialog.TitleLog},
	BindingConsole:      {OwnerConsole, dialog.TitleLogConsole},
	BindingHouseDisplay: {OwnerDisplay, dialog.TitleHouseDisplay},
	BindingSessions:     {OwnerSessions, dialog.TitleSessions},
}

// HelpLines is the Keys popup's list for this context: the main table's Help
// entries whose HelpWhen holds now, then each popup's table, then the leader's
// (the specification C15). The action column is padded so the keys line up.
func HelpLines(c Context) []HelpLine {
	leader, leaderBindings := c.Leader.Key, c.Leader.Bindings
	var blocks []HelpBlock
	var main, lead []HelpPair
	for _, s := range HelpSections(c.Help.Main) {
		b := HelpBlock{Title: s.Title}
		for _, entry := range s.Entries {
			if entry.Help && (entry.HelpWhen == nil || entry.HelpWhen(c)) {
				p := HelpPair{entry.TitleFor(c), ActionKey(c, entry)}
				b.Rows = append(b.Rows, p)
				main = append(main, p)
			}
		}
		if len(b.Rows) > 0 {
			blocks = append(blocks, b)
		}
	}
	// One section per popup, after the main groups: the popup's own table,
	// whatever its keys' conditions are right now (Help is the full reference).
	popups := HelpPopupBlocks(c)
	var popupRows []HelpPair
	for _, b := range popups {
		popupRows = append(popupRows, b.Rows...)
	}
	if leader != "" {
		for _, b := range leaderBindings {
			lead = append(lead, HelpPair{BindingDesc(c, b.ID), b.Spelling(leader)})
		}
	}
	actionW := 0
	for _, p := range append(append(append([]HelpPair(nil), main...), popupRows...), lead...) {
		actionW = max(actionW, ansi.StringWidth(p.Title))
	}
	row := func(p HelpPair) HelpLine {
		return HelpLine{Text: p.Title + strings.Repeat(" ", actionW-ansi.StringWidth(p.Title)) + "  " + p.Key, Binding: true}
	}
	lines := make([]HelpLine, 0, len(main)+len(popupRows)+len(lead)+len(HelpZodiacNote)+24)
	for i, b := range append(blocks, popups...) {
		if i > 0 {
			lines = append(lines, HelpLine{})
		}
		lines = append(lines, HelpLine{Text: b.Title, Header: true})
		for _, p := range b.Rows {
			lines = append(lines, row(p))
		}
	}
	if len(lead) > 0 {
		lines = append(lines, HelpLine{}, HelpLine{Text: "leader: " + leader, Header: true})
		for _, p := range lead {
			lines = append(lines, row(p))
		}
	}
	lines = append(lines, HelpLine{})
	for _, n := range HelpZodiacNote {
		lines = append(lines, HelpLine{Text: n})
	}
	return lines
}

// HelpPopupBlocks is one block per popup, in the order its opening key comes
// in the main table (the leader-only ones after it): the popup's table, every
// entry that has a title and is not folded into a pair.
func HelpPopupBlocks(c Context) []HelpBlock {
	var out []HelpBlock
	for _, e := range slices.Concat(c.Help.Main, c.Help.LeaderOnly) {
		p, ok := popupHelp[e.ID]
		if !ok {
			continue
		}
		b := HelpBlock{Title: p.title}
		for _, t := range c.Help.Popups[p.owner] {
			if t.Title != "" && !t.Folded {
				b.Rows = append(b.Rows, HelpPair{t.TitleFor(c), t.Binding.Help().Key})
			}
		}
		out = append(out, b)
	}
	return out
}

// HelpPopupOwners are the owners whose tables the Keys popup lists, so the
// context's builder fills exactly those.
func HelpPopupOwners() []Owner {
	var out []Owner
	for _, p := range popupHelp {
		out = append(out, p.owner)
	}
	slices.Sort(out)
	return out
}

// HelpState is the first visible line of the Help reference. Help is read,
// not chosen from, so it scrolls and has no cursor.
type HelpState struct {
	Offset int
}

// HelpScroll moves the window by step lines, kept inside the list.
func HelpScroll(offset, step, total, height int) int {
	return min(max(offset+step, 0), max(total-height, 0))
}

// HelpListHeight is the help popup's list height at this terminal height: the
// box's list height plus the search line and rule help does not draw.
func HelpListHeight(termH int) int {
	return dialog.ListHeight(termH) + 2
}

// RenderHelp draws the Keys popup for the context, in the chrome the other
// popups use.
func RenderHelp(c Context, chrome dialog.Chrome, termW, termH int) string {
	lines, cfg := HelpLines(c), c.Style
	window := func(height int) int {
		return HelpScroll(c.Help.State.Offset, 0, len(lines), height)
	}
	return dialog.Render(dialog.Box{
		Title:   dialog.TitleKeys,
		NoInput: true,
		Fit:     helpFit(lines, cfg),
		Scroll: func(height int) (pane.Viewport, int) {
			return pane.Viewport{Total: len(lines), Visible: height, Offset: window(height)}, 0
		},
	}, cfg, termW, termH, chrome, func(width, height int) string {
		return menu.Block(helpListRows(lines, window(height), cfg, width, height), width, height)
	})
}

// helpFit is the Help list's size: its widest line, binding or note, and every
// line it has (it scrolls when the terminal gives fewer rows).
func helpFit(lines []HelpLine, cfg term.Style) dialog.Fit {
	fit := dialog.Fit{Rows: len(lines)}
	for _, l := range lines {
		if l.Binding {
			fit.Width = max(fit.Width, menu.RowWidth(menu.RowSpec{Left: l.Text}, cfg))
			continue
		}
		fit.Width = max(fit.Width, ansi.StringWidth(l.Text))
	}
	fit.Width = max(fit.Width, 1)
	return fit
}

// helpListRows draws the window of the help list: binding lines through the
// shared row renderer, so the cursor row carries the band every popup list
// does, and headings as notes.
func helpListRows(lines []HelpLine, offset int, cfg term.Style, width, height int) []string {
	end := min(offset+height, len(lines))
	rows := make([]string, 0, height)
	for i := offset; i < end; i++ {
		if !lines[i].Binding {
			if lines[i].Header {
				rows = append(rows, menu.Heading(lines[i].Text, width, cfg))
				continue
			}
			rows = append(rows, menu.Note(lines[i].Text, width, cfg))
			continue
		}
		rows = append(rows, menu.Row(menu.RowSpec{Left: lines[i].Text}, cfg, width))
	}
	return rows
}

// LeaderSpelling is how Help prints a leader sequence: the configured leader,
// then the sequence as typed (`,l`, `,c d`).
func LeaderSpelling(leader string, seq []string) string { return leader + ShowSeq(seq) }

// ActionKey is the key Help prints for a command, which the Actions palette
// prints beside the same command: the main screen's key when the entry has one
// (Save chart shows ⌃s and not its leader sequence), else the configured leader
// with the sequence that reaches it, else nothing. c is a context that carries
// the leader (every owner's does).
func ActionKey(c Context, e BindingEntry) string {
	if e.Help {
		return e.Binding.Help().Key
	}
	if c.Leader.Key == "" {
		return ""
	}
	for _, b := range c.Leader.Bindings {
		if b.ID == e.ID {
			return b.Spelling(c.Leader.Key)
		}
	}
	if len(e.ShownSeq) > 0 {
		return LeaderSpelling(c.Leader.Key, e.ShownSeq)
	}
	return ""
}

// BindingDesc is the help description registered for an action, so the leader
// section and the flat list can never describe the same binding differently.
func BindingDesc(c Context, id BindingID) string {
	for _, e := range slices.Concat(c.Help.Main, c.Help.LeaderOnly) {
		if e.ID == id {
			return e.Title
		}
	}
	return ""
}
