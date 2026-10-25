package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	// rows to the list, so the box keeps its outer size: help has nothing to
	// type into.
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

	// FitRows, when set, is the most rows the list needs at the resolved
	// content width. The list takes that many rows when the terminal gives
	// more, so a popup whose content has one largest size is not taller than
	// it. Nil: the list takes every row the terminal gives.
	FitRows func(width int) int

	// Inputs, when set, replaces the search line with one line per entry,
	// each rendered at the resolved content width: a form's fields
	// (form.go). The list area gives up one row for each line past the
	// first, so the box's outer size does not change with the field count.
	// Nil means the pickers' "Search: " line, echoing FilterText.
	Inputs []func(width int) string
}

// renderModalBox draws the box around a caller's list. list is called once with
// the resolved content width and list height.
func renderModalBox(b modalBox, cfg Config, termW, termH int, list func(width, height int) string) string {
	if b.Title == "" {
		panic("renderModalBox: a popup must have a title")
	}
	const (
		minModalWidth = 60
		maxModalWidth = 90
		hPadding      = 2
		vPadding      = 1
	)

	modalWidth := clamp(termW/2, minModalWidth, maxModalWidth)

	contentWidth := modalWidth - 2 - (hPadding * 2)

	listHeight := modalListHeight(termH)

	prompt := b.Prompt
	if prompt == "" {
		prompt = "Search: "
	}
	inputLines := []string{lipgloss.NewStyle().Width(contentWidth).Render(prompt + b.FilterText + "█")}
	switch {
	case b.NoInput:
		inputLines = nil
		listHeight += 2
	case len(b.Inputs) > 0:
		inputLines = inputLines[:0]
		for _, in := range b.Inputs {
			inputLines = append(inputLines, lipgloss.NewStyle().Width(contentWidth).Render(in(contentWidth)))
		}
		listHeight = max(listHeight-(len(b.Inputs)-1), 1)
	}
	if b.FitRows != nil {
		listHeight = min(listHeight, max(b.FitRows(contentWidth), 1))
	}
	sepCh := "─"
	if cfg.useASCII() {
		sepCh = "-"
	}
	separator := strings.Repeat(sepCh, contentWidth)
	listBlock := list(contentWidth, listHeight)

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
		v, firstRow := b.Scroll(listHeight)
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
	return box
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
	// line used to take is the list's now, so the box keeps its size.
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
