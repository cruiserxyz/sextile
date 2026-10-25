package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// help.go is the Keys popup: an ordinary popup list, drawn by the same frame
// as every other popup, scrolling inside it, with a cursor that walks the
// bindings (the specification C5–C5b). It used to be its own
// box sized to its content, which ran off the top and bottom of the screen even
// at 132x40.
//
// What it lists is piece 4's to change; this file only gives it a frame,
// a cursor and a position.

// helpLine is one line of the help list. Only binding lines are positions the
// cursor can take; a heading and the blank line before it are notes.
type helpLine struct {
	text    string
	binding bool
}

// helpZodiacNote is what the Keys popup says about the ayanamsas the finder
// offers (the specification C9). It closes the list, and each line is short enough for the narrowest popup (56
// cells of text at 80 columns): it is never clipped and never needs a scroll.
var helpZodiacNote = []string{
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
		BindingNextView, BindingSlotCycleUp, BindingSlotCycleDown, BindingAttach,
		BindingOpenCharts, BindingOpenAtlas, BindingForceQuit,
	}},
	{"Move", []BindingID{
		BindingPrevView, BindingFocusChart, BindingFocusBodies, BindingFocusAspects,
		BindingSelect, BindingSlotMoveUp, BindingSlotMoveDown, BindingSlotDelete,
		BindingPageUp, BindingPageDown, BindingPageHome, BindingPageEnd,
	}},
	{"Time (lowercase forward, uppercase back)", []BindingID{
		BindingTimeStepYear, BindingTimeStepMonth, BindingTimeStepWeek, BindingTimeStepDay,
		BindingTimeStepHour, BindingTimeStepMinute, BindingJumpNow, BindingToggleBack,
	}},
	{"Views", []BindingID{
		BindingDetailsTab, BindingTabPrev, BindingTabNext, BindingScrollUp, BindingScrollDown,
		BindingBodies, BindingAspects, BindingHouses, BindingZodiac,
	}},
	{"Output", []BindingID{BindingSaveChart, BindingRenderWheel}},
}

// helpUnlisted are main-table entries Help leaves out on purpose.
var helpUnlisted = map[BindingID]bool{BindingHelp: true, BindingQuit: true}

// limitedViewNotice says what size shows Details and the Log: the layout's own
// dual-column threshold for the charts shown, at the height the design treats
// as comfortable.
func limitedViewNotice(shown int) string {
	return fmt.Sprintf("Limited view: %dx30 shows Details and the Log", dualColumnMinWidth(shown))
}

// helpSections puts the main table's help entries into the Help groups; an
// entry in no group and not left out on purpose lands in a final "More" group,
// so a new binding is never silently missing from Help.
func helpSections(entries []BindingEntry) []struct {
	title   string
	entries []BindingEntry
} {
	byID := map[BindingID]BindingEntry{}
	for _, e := range entries {
		byID[e.ID] = e
	}
	type section = struct {
		title   string
		entries []BindingEntry
	}
	var out []section
	seen := map[BindingID]bool{}
	for _, g := range helpGroups {
		s := section{title: g.title}
		for _, id := range g.ids {
			if e, ok := byID[id]; ok {
				s.entries = append(s.entries, e)
				seen[id] = true
			}
		}
		out = append(out, s)
	}
	more := section{title: "More"}
	for _, e := range entries {
		if !seen[e.ID] && !helpUnlisted[e.ID] {
			more.entries = append(more.entries, e)
		}
	}
	return append(out, more)
}

// helpListing is the help list's lines: the main screen's table, then the
// leader's heading and the leader's table, each in table order, one
// `Action  key` row per entry with help (the specification
// C15). The action column is padded so the keys line up.
// helpLines is the Keys popup's list for this model: the main table's Help
// entries whose HelpWhen holds now, then the leader's.
func (m Model) helpLines() []helpLine {
	keys, leader, leaderBindings := m.keys, m.leaderKey, m.leaderBindings
	type pair struct{ action, key string }
	type block struct {
		title string
		rows  []pair
	}
	var blocks []block
	var main, lead []pair
	for _, s := range helpSections(keys.mainBindings()) {
		b := block{title: s.title}
		for _, entry := range s.entries {
			if entry.Help && (entry.HelpWhen == nil || entry.HelpWhen(m)) {
				h := entry.Binding.Help()
				b.rows = append(b.rows, pair{h.Desc, h.Key})
				main = append(main, pair{h.Desc, h.Key})
			}
		}
		if len(b.rows) > 0 {
			blocks = append(blocks, b)
		}
	}
	if leader != "" {
		for _, b := range leaderBindings {
			lead = append(lead, pair{bindingDesc(keys, b.ID), leader + showSeq(b.Seq)})
		}
	}
	actionW := 0
	for _, p := range append(append([]pair(nil), main...), lead...) {
		actionW = max(actionW, ansi.StringWidth(p.action))
	}
	row := func(p pair) helpLine {
		return helpLine{text: p.action + strings.Repeat(" ", actionW-ansi.StringWidth(p.action)) + "  " + p.key, binding: true}
	}
	lines := make([]helpLine, 0, len(main)+len(lead)+len(helpZodiacNote)+6)
	for i, b := range blocks {
		if i > 0 {
			lines = append(lines, helpLine{})
		}
		lines = append(lines, helpLine{text: b.title})
		for _, p := range b.rows {
			lines = append(lines, row(p))
		}
	}
	if len(lead) > 0 {
		lines = append(lines, helpLine{}, helpLine{text: "leader: " + leader})
		for _, p := range lead {
			lines = append(lines, row(p))
		}
	}
	lines = append(lines, helpLine{})
	for _, n := range helpZodiacNote {
		lines = append(lines, helpLine{text: n})
	}
	return lines
}

// helpState is the first visible line of the Help reference. Help is read,
// not chosen from, so it scrolls and has no cursor.
type helpState struct {
	offset int
}

// helpScroll moves the window by step lines, kept inside the list.
func helpScroll(offset, step, total, height int) int {
	return min(max(offset+step, 0), max(total-height, 0))
}

// helpListHeight is the help popup's list height at this terminal height: the
// box's list height plus the search line and rule help does not draw.
func helpListHeight(termH int) int {
	return modalListHeight(termH) + 2
}

// renderHelpModal draws the Keys popup.
func renderHelpModal(lines []helpLine, cfg Config, st helpState, termW, termH int) string {
	window := func(height int) int {
		return helpScroll(st.offset, 0, len(lines), height)
	}
	return renderModalBox(modalBox{
		Title:   popupTitleKeys,
		NoInput: true,
		Scroll: func(height int) (viewport, int) {
			return viewport{Total: len(lines), Visible: height, Offset: window(height)}, 0
		},
	}, cfg, termW, termH, func(width, height int) string {
		return listBlock(helpListRows(lines, window(height), cfg, width, height), width, height)
	})
}

// helpListRows draws the window of the help list: binding lines through the
// shared row renderer, so the cursor row carries the band every popup list
// does, and headings as notes.
func helpListRows(lines []helpLine, offset int, cfg Config, width, height int) []string {
	end := min(offset+height, len(lines))
	rows := make([]string, 0, height)
	for i := offset; i < end; i++ {
		if !lines[i].binding {
			rows = append(rows, listNote(lines[i].text, width, cfg))
			continue
		}
		rows = append(rows, listRow(listRowSpec{Left: lines[i].text}, cfg, width))
	}
	return rows
}

// bindingDesc is the help description registered for an action, so the leader
// section and the flat list can never describe the same binding differently.
func bindingDesc(keys keyMap, id BindingID) string {
	for _, e := range keys.mainBindings() {
		if e.ID == id {
			return e.Binding.Help().Desc
		}
	}
	return ""
}
