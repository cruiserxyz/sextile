package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// modal_box.go holds the centred-modal chrome that the atlas picker, the chart
// picker and (from an earlier change) the aspect chooser and body selector all draw.
//
// It is an EXTRACTION, not a new design: the math below was character-identical
// in renderAtlasModal and renderChartModal, differing only in the border
// colour. chart_modal.go said so itself — "the box-layout math is duplicated
// rather than shared so the tested atlas render is untouched; a future simplify
// pass can converge them" — and an earlier step froze the atlas render, which is
// what made converging safe. The goldens in atlas_lift_golden_test.go and
// chart_box_golden_test.go are byte-identical across this move; that is the
// acceptance, not a review of the diff.
//
// What deliberately does NOT live here is the LIST. The two pickers filter
// different things in different ways — an empty filter browses the whole chart
// library but only hints at the atlas, and the chart list reserves rows for
// unreadable files — so each caller passes its own list renderer.

// modalBox is one centred modal's chrome: a titled frame, a search line, a
// rule, a caller's list, and a footer, inside a padded border.
type modalBox struct {
	// Title names the popup in its frame's top edge, drawn by the function
	// that draws a panel's title (the specification C1, C2).
	// Required: a popup with no name cannot say which popup it is.
	Title string

	// NoInput drops the search line and the rule under it, and gives both
	// rows to the list: help has nothing to type into.
	NoInput bool

	// Position is the list's `N of M`, drawn into the bottom edge. Nil for a
	// popup with no list (the forms).
	Position *listPosition

	// Scroll reports the list's scroll geometry for the list height the box
	// resolved, and the list row its track starts on (below a pinned block).
	// Nil: the popup has no list that scrolls.
	Scroll func(listHeight int) (v viewport, firstRow int)

	// FilterText is echoed into the search line. The block cursor is the
	// box's, not the caller's.
	FilterText string

	// Prompt leads the search line. Empty means "Search: "; a popup whose
	// list holds one kind of thing names it.
	Prompt string

	// Fit is what the content needs. The box is as wide as the widest line
	// it draws and as tall as its rows, and no wider or taller, up to what
	// the terminal gives (operator, 2026-10-27: "make the popup windows
	// adjust their size to what they actually need. everywhere. as a rule").
	// Required: a popup that does not say what it needs would be given a
	// size by a constant, which is the thing this rule removes.
	Fit modalFit

	// Inputs, when set, replaces the search line with one line per entry,
	// each rendered at the resolved content width: a form's fields
	// (form.go). The list area gives up one row for each line past the
	// first, so the most rows the terminal gives the box stay the same.
	// Nil means the pickers' "Search: " line, echoing FilterText.
	Inputs []modalInput

	// Zone reserves the message zone: a blank row and the row for a message,
	// at the foot of the box, from the moment it opens. Every popup that can
	// refuse or note something has it, and it is the same in all of them, so a
	// message never sits against the content (operator, 2026-10-27).
	Zone bool

	// Message is what the zone says now. A message that needs more than its
	// row wraps and takes rows from the foot of the content above, which shows
	// fewer rows meanwhile; the box does not change size, and the content has
	// its rows back when the message clears.
	Message string

	// KeepRows is the rows of the content a message may not take: what stays
	// on screen whatever is said (a pinned block, and the list's cursor row).
	// Zero means one row.
	KeepRows int
}

// modalZoneRows is the rows the message zone keeps: the blank row above the
// message and the message's own row.
const modalZoneRows = 2

// modalFit is a popup's own account of its size.
type modalFit struct {
	// Width is the cells of the widest line the list area draws, over every
	// entry the popup can show and not only the filtered ones: a box that
	// changed width while the operator typed would jump under the cursor.
	// The search line and the title are the box's to account for.
	Width int

	// Rows is the rows the list area draws now: the filtered entries, a
	// pinned block and the notes. It is the CURRENT count, so the box is as
	// tall as what it shows; it never exceeds what the terminal gives.
	Rows int
}

// modalInput is one input line of a form: the line, and the cells it needs.
type modalInput struct {
	// Width is the label, the room to type and the cursor.
	Width  int
	Render func(width int) string
}

const (
	// modalSearchRoom is how many cells the search line leaves for typing
	// after its prompt: a filter is a word or two, and the line is a floor
	// under the box, not content (chosen 2026-10-27).
	modalSearchRoom = 24

	// modalFieldRoom is how many cells a form's field leaves for its value.
	// A chart's name or a session's name is a few words; a longer one scrolls
	// in its field (fieldLine shows the tail). Wider than the search room
	// because the line under a form echoes what was typed as a file name
	// (chosen 2026-10-27).
	modalFieldRoom = 40

	// modalProseWidth is where a sentence in a popup wraps. A box is as wide
	// as its widest line, so prose needs a width to be measured against, or
	// one long sentence would make the box as wide as the terminal.
	modalProseWidth = 60

	// modalSideMargin is the columns kept free on each side of a box.
	modalSideMargin = 2

	// modalChromeWidth is what the border and the padding add to the content.
	modalChromeWidth = 2 + 2*modalHPadding

	modalHPadding = 2
)

// modalFitProbe is the fit guard's seam (TestEveryPopupFitsItsContent): it
// takes cells off the content width, to show that the box needed the ones it
// was given -- narrower by one, something is cut. Zero in the program, always.
var modalFitProbe struct {
	narrowBy int
	// uncapped lifts the terminal's height, so a list that scrolls shows every
	// row and its widest line can be seen.
	uncapped bool
}

// modalContentCap is the widest content a box can have in this terminal.
func modalContentCap(termW int) int {
	return max(termW-2*modalSideMargin-modalChromeWidth, 10)
}

// modalTitleBudget is the cells a popup's title may take before it is cut.
func modalTitleBudget(termW int) int {
	return min(modalContentCap(termW), modalProseWidth)
}

// modalWidthFloor is the content width the frame itself needs: the title in
// the top edge (with the focus mark colourless frames carry) and the widest
// counter in the bottom one. Neither is content, but a box narrower than them
// would cut a name or a count.
func modalWidthFloor(b modalBox, cfg Config) int {
	mark := 0
	if !cfg.useColor() {
		mark = 1
	}
	floor := min(ansi.StringWidth(b.Title), modalProseWidth) + mark + 4 - modalChromeWidth
	if b.Position != nil {
		widest := listPosition{Cursor: max(b.Position.Total, 1), Total: max(b.Position.Total, 1)}
		floor = max(floor, ansi.StringWidth(widest.text())+4-modalChromeWidth)
	}
	return floor
}

// searchLineWidth is the floor the search line puts under the box.
func searchLineWidth(prompt string) int {
	return ansi.StringWidth(prompt) + modalSearchRoom + 1
}

// modalContentWidth is the box's content width in this terminal: as wide as its
// widest line -- the content, or the floor the search line, a form's fields
// and the frame put under it -- up to what the terminal gives.
func modalContentWidth(b modalBox, cfg Config, termW int) int {
	prompt := b.Prompt
	if prompt == "" {
		prompt = "Search: "
	}
	need := max(b.Fit.Width, modalWidthFloor(b, cfg))
	switch {
	case b.NoInput:
	case len(b.Inputs) > 0:
		for _, in := range b.Inputs {
			need = max(need, in.Width)
		}
	default:
		need = max(need, searchLineWidth(prompt))
	}
	return min(need, modalContentCap(termW))
}

// renderModalBox draws the box around a caller's list. list is called once with
// the resolved content width and list height.
func renderModalBox(b modalBox, cfg Config, termW, termH int, list func(width, height int) string) string {
	if b.Title == "" {
		panic("renderModalBox: a popup must have a title")
	}
	const (
		hPadding = modalHPadding
		vPadding = 1
	)
	if b.Fit.Width < 1 {
		panic("renderModalBox: a popup must say how wide its content is (modalBox.Fit)")
	}

	prompt := b.Prompt
	if prompt == "" {
		prompt = "Search: "
	}

	contentWidth := modalContentWidth(b, cfg, termW)
	if modalFitProbe.narrowBy > 0 {
		contentWidth = max(contentWidth-modalFitProbe.narrowBy, 1)
	}

	listHeight := modalListHeight(termH)
	if modalFitProbe.uncapped {
		listHeight = 1 << 10
	}

	var inputLines []string
	switch {
	case b.NoInput:
		listHeight += 2
	case len(b.Inputs) > 0:
		for _, in := range b.Inputs {
			inputLines = append(inputLines, lipgloss.NewStyle().Width(contentWidth).Render(in.Render(contentWidth)))
		}
		listHeight = max(listHeight-(len(b.Inputs)-1), 1)
	default:
		// A filter longer than the line keeps its tail, where the cursor is.
		room := max(contentWidth-ansi.StringWidth(prompt)-1, 0)
		inputLines = []string{lipgloss.NewStyle().Width(contentWidth).Render(prompt + cutLeft(b.FilterText, room, cfg) + "█")}
	}
	zone := 0
	if b.Zone {
		zone = modalZoneRows
	}
	// The content has what the terminal gives it, up to its rows; the zone is
	// the box's own rows below it, on top of that: a popup that can say
	// something has the room for it.
	wanted := min(listHeight, max(b.Fit.Rows, 1))
	total := wanted + zone
	if !modalFitProbe.uncapped {
		// Never taller than the terminal: the top edge, the padding, the input
		// lines, their rule and the bottom edge are outside the list.
		chrome := 1 + 2*vPadding + len(inputLines) + 1
		if !b.NoInput {
			chrome++
		}
		total = min(total, max(termH-chrome, 1))
	}
	// On a terminal too short for both, the content keeps its rows and the zone
	// gives way (a quiet zone is empty rows, so nothing is lost until something
	// is said).
	reserved := min(max(total-wanted, 0), zone)
	contentHeight := total - reserved
	var zoneLines []string
	if b.Zone {
		blank := strings.Repeat(" ", contentWidth)
		if b.Message == "" {
			for i := 0; i < reserved; i++ {
				zoneLines = append(zoneLines, blank)
			}
		} else {
			lines := wrapAtSpace(strings.Join(strings.Fields(b.Message), " "), contentWidth)
			// A blank row above the message, then the message; what it needs
			// beyond the zone's rows it takes from the foot of the content,
			// which keeps the rows it may not lose.
			takeable := max(contentHeight-max(b.KeepRows, 1), 0)
			rows := max(min(1+len(lines), reserved+takeable), 1)
			if rows < 1+len(lines) {
				keep := max(rows-1, 1)
				lines[keep-1] = cutRight(strings.Join(lines[keep-1:], " "), contentWidth, cfg)
				lines = lines[:keep]
			}
			if rows >= 2 {
				zoneLines = append(zoneLines, blank)
			}
			for _, l := range lines {
				l = fitLine(l, contentWidth, cfg)
				if cfg.useColor() {
					l = lipgloss.NewStyle().Foreground(errorColour).Render(l)
				}
				zoneLines = append(zoneLines, l)
			}
			contentHeight = total - len(zoneLines)
		}
	}
	sepCh := "─"
	if cfg.useASCII() {
		sepCh = "-"
	}
	separator := strings.Repeat(sepCh, contentWidth)
	listBlock := list(contentWidth, contentHeight)
	if len(zoneLines) > 0 {
		listBlock += "\n" + strings.Join(zoneLines, "\n")
	}

	parts := inputLines
	if !b.NoInput {
		parts = append(parts, separator)
	}
	parts = append(parts, listBlock)
	body := lipgloss.JoinVertical(lipgloss.Left, parts...)

	border := lipgloss.RoundedBorder()
	leftCorner, rightCorner, dash := "╭", "╮", "─"
	if cfg.useASCII() {
		border = asciiBorder
		leftCorner, rightCorner, dash = "+", "+", "-"
	}
	// A popup always owns the keyboard, so its frame is the focus frame.
	frame := panelBorderColor(true, cfg)
	style := lipgloss.NewStyle().
		Border(border, false, true, true, true).
		Padding(vPadding, hPadding)
	if cfg.useColor() {
		style = style.BorderForeground(frame)
	}
	rest := style.Render(body)
	width := lipgloss.Width(rest)
	top := renderTopBorder(width, tabStrip{tabs: []Tab{{Title: b.Title}}}, frame, focusColourFor(cfg), true, cfg, leftCorner, rightCorner, dash)
	box := top + "\n" + rest

	if b.Scroll != nil {
		v, firstRow := b.Scroll(contentHeight)
		// The track starts on the list's first row: below the top edge, the
		// padding, the search line and its rule.
		above := 1 + vPadding + len(inputLines)
		if !b.NoInput {
			above++
		}
		box = withScrollIndicatorAt(box, v, cfg, true, above+firstRow)
	}
	if b.Position != nil {
		box = withPositionCounter(box, *b.Position, cfg, true)
	}
	return paintGlyphRoom(box)
}

// modalListHeight is how many rows the box gives a caller's list at this
// terminal height.
//
// Navigation needs this number as much as drawing does: scrolling happens when
// the cursor MOVES, not when the list is painted, so a key handler has to know
// how tall the window is. Deriving it in one place is what stops the two from
// drifting apart and putting the cursor outside the window that gets drawn.
func modalListHeight(termH int) int {
	const (
		minModalHeight = 14
		maxModalHeight = 30
		vPadding       = 1
	)
	modalHeight := clamp(termH-10, minModalHeight, maxModalHeight)
	contentHeight := modalHeight - 2 - (vPadding * 2)
	if contentHeight < 4 {
		contentHeight = 4
	}
	// search + separator = 2 fixed lines; the rest is list. The row a hint
	// line used to take is the list's now.
	listHeight := contentHeight - 2
	if listHeight < 1 {
		listHeight = 1
	}
	return listHeight
}

// clamp bounds v into [lo, hi]. It lived in atlas_modal.go and moved here with
// the box; the chart picker was already calling it across files.
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// The popups' names, in their frames' top edges
// (the specification V1). Sentence case, like every title.
const (
	popupTitleCity       = "City"
	popupTitleOpenChart  = "Open chart"
	popupTitleChartTitle = "Chart title"
	popupTitleSaveChart  = "Save chart"
	popupTitleBodies     = "Bodies"
	popupTitleAspects    = "Aspects"
	popupTitleKeys       = "Help"
)
