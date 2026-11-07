package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// The loading label (specs/details-loading-overlay.md): after the active
// Details tab has waited a second for its content, `loading` stands in the
// bottom edge where the position counter stands, its dots stepping every half
// second. A shorter wait shows nothing.
const (
	loadingAfter = time.Second
	loadingStep  = 500 * time.Millisecond
)

// loadingState is when the active tab started waiting. Nothing persists.
type loadingState struct {
	on    bool
	tab   TabID
	since time.Time
	gen   uint64
}

// loadingTickMsg repaints a waiting tab. It carries the wait's generation, so
// a tick of a wait that has ended changes nothing.
type loadingTickMsg struct{ gen uint64 }

// loadingTimer starts the label's repaint timer. It is a variable so that the
// test binary can switch it off, as it does the minute and idle timers
// (main_test.go): the label is derived from the clock at paint, so a test of
// it moves the fake clock and delivers the tick by hand, and no other test
// waits on a repaint that only animates.
var loadingTimer = longTimer

func armLoadingTick(gen uint64, d time.Duration) tea.Cmd {
	return loadingTimer(d, func(time.Time) tea.Msg { return loadingTickMsg{gen: gen} })
}

// syncLoading compares the active Details tab's waiting flag with the stored
// state after every Update, so each path that starts or ends a wait is covered.
// A wait that starts arms the first timer; one that ends arms nothing.
func (m Model) syncLoading() (Model, tea.Cmd) {
	id := m.activeTabID(PanelDetails)
	waiting := m.detailsTabWaiting(id)
	switch {
	case waiting && (!m.loading.on || m.loading.tab != id):
		m.loading = loadingState{on: true, tab: id, since: m.clock.Now(), gen: m.loading.gen + 1}
		return m, armLoadingTick(m.loading.gen, loadingAfter)
	case !waiting && m.loading.on:
		m.loading = loadingState{gen: m.loading.gen + 1}
	}
	return m, nil
}

// onLoadingTick repaints and, while the same wait goes on, arms the next step.
func (m Model) onLoadingTick(msg loadingTickMsg) (tea.Model, tea.Cmd) {
	if !m.loading.on || msg.gen != m.loading.gen {
		return m, nil
	}
	return m, armLoadingTick(m.loading.gen, loadingStep)
}

// loadingPhase is the dot count to draw now plus one, or 0 for no overlay. It
// derives from the elapsed time, so a late tick cannot put the dots out of step.
func (m Model) loadingPhase(id TabID) int {
	// The wait was timed for one tab; another tab's wait starts its own second.
	if !m.loading.on || m.loading.tab != id {
		return 0
	}
	el := m.clock.Now().Sub(m.loading.since)
	if el < loadingAfter {
		return 0
	}
	return int((el-loadingAfter)/loadingStep)%4 + 1
}

// loadingLabel is `loading` and the phase's dots (phase 1 has none). It is
// not padded: at the edge's left the word stays put and each dot takes the
// place of one border dash, so the border never shows a gap.
func loadingLabel(phase int) string {
	return "loading" + strings.Repeat(".", phase-1)
}

// withLoadingLabel draws the box's bottom edge for a wait: no counter, and
// from phase 1 the label at the edge's left, one dash in from the corner
// (operator, 2026-11-06: "no overlay, instead replace the bottom n of n string
// ... as foreground text color, no bg color", then "move the loading animation
// float to the left bottom"). The label is a neon blue (term.LoadingColour) as
// its text colour, no background, bold while Details has the keyboard. Phase 0 draws the
// plain edge; an edge too narrow for the label keeps the plain edge.
func withLoadingLabel(box string, phase int, cfg Config, keyboard bool) string {
	if box == "" {
		return box
	}
	lines := strings.Split(box, "\n")
	label := lipgloss.NewStyle()
	if cfg.UseColor() {
		label = label.Foreground(term.LoadingColour).Bold(keyboard)
	}
	text := ""
	if phase > 0 {
		text = loadingLabel(phase)
	}
	if edge, ok := bottomEdgeLeft(lipgloss.Width(lines[len(lines)-1]), text, label, cfg, keyboard); ok {
		lines[len(lines)-1] = edge
	}
	return strings.Join(lines, "\n")
}

// bottomEdgeLeft is a bottom edge holding text at its left, one dash in from
// the corner, the rest dashes: the edge is always whole, whatever the text's
// width. False when the edge cannot hold the text whole.
func bottomEdgeLeft(width int, text string, style lipgloss.Style, cfg Config, keyboard bool) (string, bool) {
	tw := ansi.StringWidth(text)
	if width < tw+4 {
		return "", false
	}
	left, right, dash := "╰", "╯", "─"
	if cfg.UseASCII() {
		left, right, dash = "+", "+", "-"
	}
	border := lipgloss.NewStyle()
	if cfg.UseColor() {
		border = border.Foreground(pane.BorderColour(keyboard, cfg))
	}
	if tw == 0 {
		return border.Render(left + strings.Repeat(dash, width-2) + right), true
	}
	return border.Render(left+dash) + style.Render(text) + border.Render(strings.Repeat(dash, width-3-tw)+right), true
}
