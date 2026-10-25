package ui

import (
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// The console is the Log with the keyboard. `,l` gives it the whole right
// column (or the whole main area when the terminal is narrow), a cursor that
// moves line by line, and an Enter that copies the line under it. Esc puts
// everything back.
//
// It is a mode, not a panel: focusablePanels, m.focused and the tab cycle are
// untouched while it is open, so Tab still means what it always meant the
// moment it closes.

// yankFlashDuration is how long the key bar confirms a copy.
const yankFlashDuration = 1500 * time.Millisecond

// yankLine hands text to the system clipboard. It is one package-level
// variable so a test can replace it: no test may emit an OSC 52 escape into
// the test runner's terminal, and no caller anywhere builds that sequence by
// hand.
var yankLine = func(text string) error { return copyToClipboard(os.Stdout, text) }

// copyToClipboard emits text as an OSC 52 clipboard sequence on w. Sextile
// builds no escape sequence of its own here; termenv owns the encoding, and
// the writer is a parameter so the emission is checkable without writing to
// the terminal a test is running in.
func copyToClipboard(w io.Writer, text string) error {
	termenv.NewOutput(w).Copy(text)
	return nil
}

// consoleState is the console's whole existence: whether it is open, the
// snapshot it renders, which snapshot line the cursor is on, which line the
// rendered window starts at, and the panel that had focus when it opened.
// Ephemeral Model state — never written to the preferences or session files,
// and zeroed on close.
type consoleState struct {
	open bool
	// snapshot is the argv+error lines as they stood when the console opened
	//. Everything the console does — cursor, window, indicator,
	// Enter — addresses THIS list, never the Model's live buffer: a line
	// arriving or evicted while the console is open changes nothing on
	// screen, and the next `,l` takes a fresh one. The operator chose the
	// snapshot over a live list ("snapshot and open. simplify."), which is
	// what let absorbTrim and its state machine go.
	snapshot   []logLine
	cursor     int
	offset     int
	priorFocus PanelID
}

// yankState is the copy confirmation: whether one is showing and which
// generation armed it, so a flash timer can only ever clear its own flash.
type yankState struct {
	active bool
	gen    uint64
	// chars is the length of the line just copied, for the confirmation
	// treatment that reports it.
	chars int
	// notice, when set, is a message the key bar shows instead of the copy
	// confirmation: an action that did nothing, or a copy that failed. It
	// shares the generation rule, so a tick clears only its own.
	notice string
}

// yankFlashMsg clears a copy confirmation. Generation-tagged like the leader
// timeout and the refresh coordinator's ticks.
type yankFlashMsg struct{ gen uint64 }

func armYankFlash(gen uint64) tea.Cmd {
	return longTimer(yankFlashDuration, func(time.Time) tea.Msg { return yankFlashMsg{gen: gen} })
}

// noticeDuration is how long the key bar keeps a notice.
const noticeDuration = 3 * time.Second

// The key bar's notices: a key that did nothing, or a copy that did not land.
const (
	noticeOnlyNow       = "Only Now can be saved as a new chart; focus Now first."
	noticeNothingCopy   = "Nothing to copy"
	noticeCopyFailed    = "Copy failed: the clipboard is unavailable."
	noticeDefaultsSaved = "Defaults saved"
	// noticeSecondWindow is shown once, when a second window starts.
	noticeSecondWindow = "Another Sextile window keeps the session; this window's changes are not kept between runs."
)

// secondWindowNote is the same on the Log's note lane, after its scope.
const secondWindowNote = "another Sextile window keeps the session; this window's changes are not kept between runs"

// limitedViewPrefix opens the notice a terminal too small for Details shows.
const limitedViewPrefix = "Limited view: "

// showNotice puts text in the key bar's right slot for noticeDuration. It
// writes no Log line.
func (m Model) showNotice(text string) (Model, tea.Cmd) {
	m.yank.gen++
	m.yank.active = true
	m.yank.notice = text
	m.yank.chars = 0
	gen := m.yank.gen
	return m, longTimer(noticeDuration, func(time.Time) tea.Msg { return yankFlashMsg{gen: gen} })
}

// isNotice reports whether a footer indicator is one of the notices.
func isNotice(indicator string) bool {
	s := ansi.Strip(indicator)
	return s == noticeOnlyNow || s == noticeNothingCopy || s == noticeCopyFailed || s == noticeDefaultsSaved || s == noticeSecondWindow || strings.HasPrefix(s, limitedViewPrefix)
}

// consoleRect is the console's rectangle: the whole right column when the
// layout is dual, the whole main area when it is not. Reports false below the
// minimum terminal size, where tooSmallView renders instead — the console
// state survives that, so widening returns to it.
func consoleRect(width, height, chartLines, shown int) (panelRect, bool) {
	if layoutGeometry(width, height, chartLines, shown) == nil {
		return panelRect{}, false
	}
	mainHeight := height - KeyBarHeight
	if left := leftColumnWidth(shown); width >= dualColumnMinWidth(shown) {
		return panelRect{x: left, y: 0, w: width - left, h: mainHeight}, true
	}
	return panelRect{x: 0, y: 0, w: width, h: mainHeight}, true
}

func (m Model) consoleRect() (panelRect, bool) {
	return consoleRect(m.width, m.height, chartContentLines(m.slots), m.shownCount())
}

// consoleInnerHeight is how many buffer lines the console can show, or 0 when
// there is no console-sized rectangle to show them in.
func (m Model) consoleInnerHeight() int {
	r, ok := m.consoleRect()
	if !ok {
		return 0
	}
	if r.h-2 < 0 {
		return 0
	}
	return r.h - 2
}

// consoleWindow returns the half-open range of buffer indexes the console
// renders from a stored offset. Kept apart from the renderer so the frozen-view
// contract is checkable as arithmetic instead of by reading rendered text.
func consoleWindow(total, offset, innerHeight int) (start, end int) {
	if total <= 0 || innerHeight <= 0 {
		return 0, 0
	}
	start = clampOffset(offset, total, innerHeight)
	end = start + innerHeight
	if end > total {
		end = total
	}
	return start, end
}

// consoleViewport is the scroll arithmetic the console hands the shared
// indicator: the WHOLE snapshot measured against the window actually
// rendered. Total is the snapshot length rather than the window's, so the
// thumb reports how much of the list lies off screen.
func consoleViewport(total int, st consoleState, innerHeight int) viewport {
	start, _ := consoleWindow(total, st.offset, innerHeight)
	return viewport{Total: total, Visible: innerHeight, Offset: start}
}

// scrollToShow nudges the window the least it can to keep line inside it with
// listScrollMargin rows of context above and below (specs/
// focus-and-nearest-event.md rule 6): a cursor moving inside the margins does
// not scroll the window, one that would come within the margin of an edge
// scrolls it. A window too short to hold both margins plus a row centres on
// the line instead, because it would otherwise scroll on every step. The
// popups' fuzzy lists, the Log console, help and the timeline all use this.
func scrollToShow(offset, line, total, visible int) int {
	if visible <= 0 {
		return offset
	}
	if total <= visible {
		return 0
	}
	if visible < 2*listScrollMargin+1 {
		return clampOffset(line-visible/2, total, visible)
	}
	if line-offset < listScrollMargin {
		offset = line - listScrollMargin
	}
	if line-offset > visible-1-listScrollMargin {
		offset = line - (visible - 1 - listScrollMargin)
	}
	return clampOffset(offset, total, visible)
}

// openConsole opens the console over a snapshot of the argv+error lines,
// with the cursor on the newest of them and the panel that had focus
// remembered. It starts nothing: no process, no refresh surface, no timer.
func (m Model) openConsole() (tea.Model, tea.Cmd) {
	m.errorUnseen = false
	m.errorKeptView = false
	m.console = consoleState{open: true, priorFocus: m.focused, snapshot: m.consoleLines()}
	if n := len(m.console.snapshot); n > 0 {
		at := n - 1
		// The now line is last; the cursor lands on it only while the floating
		// chart follows the clock and the Chart cursor is on that chart.
		// Otherwise it opens on the newest line that is not the now line.
		if m.console.snapshot[at].lane == laneNow && !(m.slots != nil && !m.floatingDetached && m.focusedIsFloating()) && n > 1 {
			at = n - 2
		}
		m.console.cursor = at
		m.console.offset = scrollToShow(0, at, n, m.consoleInnerHeight())
	}
	return m, nil
}

// closeConsole puts focus back where it was and zeroes the console's state.
func (m Model) closeConsole() (tea.Model, tea.Cmd) {
	m.focused = m.console.priorFocus
	m.console = consoleState{}
	return m, nil
}

// moveConsoleCursor steps the cursor towards the oldest (negative) or newest
// (positive) snapshot line, clamped at both ends, scrolling the window only
// when the cursor would otherwise leave it.
func (m Model) moveConsoleCursor(delta int) (tea.Model, tea.Cmd) {
	total := len(m.console.snapshot)
	if total == 0 {
		return m, nil
	}
	cursor := m.console.cursor + delta
	if cursor < 0 {
		cursor = 0
	}
	if cursor > total-1 {
		cursor = total - 1
	}
	m.console.cursor = cursor
	m.console.offset = scrollToShow(m.console.offset, cursor, total, m.consoleInnerHeight())
	return m, nil
}

// clampConsole holds the cursor inside the snapshot and the window around the
// cursor. It runs on resize: a console that had no inner height could not
// scroll its window onto a moving cursor, and widening is where that gets
// put right. It is a no-op in the ordinary case — a cursor already inside its
// window does not move it.
func (m Model) clampConsole() Model {
	if !m.console.open {
		return m
	}
	total := len(m.console.snapshot)
	if total == 0 {
		m.console.cursor, m.console.offset = 0, 0
		return m
	}
	if m.console.cursor > total-1 {
		m.console.cursor = total - 1
	}
	if m.console.cursor < 0 {
		m.console.cursor = 0
	}
	m.console.offset = scrollToShow(m.console.offset, m.console.cursor, total, m.consoleInnerHeight())
	return m
}

// updateConsoleKey owns the keyboard while the console is open: Esc closes,
// the cursor keys move, Enter copies, and everything else — `q` included — is
// swallowed rather than falling through to a chart the operator cannot see the
// effect of (the specification C11). Ctrl+C still quits; Update
// takes it before the console sees it.
func (m Model) updateConsoleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch matchBinding(msg, consoleBindings()) {
	case BindingClose:
		return m.closeConsole()
	case BindingListUp:
		return m.moveConsoleCursor(-1)
	case BindingListDown:
		return m.moveConsoleCursor(+1)
	case BindingCopy:
		return m.yankConsoleLine()
	case BindingPageUp:
		return m.pageConsole(pageUp)
	case BindingPageDown:
		return m.pageConsole(pageDown)
	case BindingPageHome:
		return m.pageConsole(pageHome)
	case BindingPageEnd:
		return m.pageConsole(pageEnd)
	}
	return m, nil
}

// pageConsole applies a paging key to the console's cursor by the one rule
// (pageCursor) over the snapshot and the rows the panel shows. The panel can
// have no rows after a resize; the rule reads that as one and keeps the cursor
// inside the snapshot. The window is stored exactly: the console draws from it.
func (m Model) pageConsole(op pageOp) (tea.Model, tea.Cmd) {
	total := len(m.console.snapshot)
	if total == 0 {
		return m, nil
	}
	m.console.cursor, m.console.offset = pageCursor(m.console.cursor, m.console.offset, total, m.consoleInnerHeight(), op)
	return m, nil
}

// yankConsoleLine copies the stored snapshot line under the cursor — in
// full, never the rendered row, so no border, padding, truncation or styling
// reaches the clipboard, whatever lane the line is in. An
// empty snapshot has no line to copy and does not pretend otherwise.
func (m Model) yankConsoleLine() (tea.Model, tea.Cmd) {
	total := len(m.console.snapshot)
	if total == 0 || m.console.cursor < 0 || m.console.cursor >= total {
		return m.showNotice(noticeNothingCopy)
	}
	text := m.console.snapshot[m.console.cursor].text
	if err := yankLine(text); err != nil {
		// One of the five error-lane sites: the copy the
		// operator asked for did not happen.
		return m.logError("Couldn't copy that line to the clipboard.").showNotice(noticeCopyFailed)
	}
	m.yank.gen++
	m.yank.active = true
	m.yank.notice = ""
	m.yank.chars = utf8.RuneCountInString(text)
	return m, armYankFlash(m.yank.gen)
}

// onYankFlash retires a copy confirmation. A tick carrying any generation but
// the current one belongs to a flash that is already gone.
func (m Model) onYankFlash(msg yankFlashMsg) (tea.Model, tea.Cmd) {
	if !m.yank.active || msg.gen != m.yank.gen {
		return m, nil
	}
	m.yank.active = false
	return m, nil
}

// consoleEmptyLine is the console's body when it opened on nothing.
const consoleEmptyLine = "(no messages yet)"

// renderConsole draws the Log as a console over the lines it is handed —
// the console's snapshot, at both call sites in compose: the window of
// snapshot lines around the cursor, the cursor row in whichever active-row
// treatment the run is configured for, and the shared scroll indicator
// measuring the WHOLE snapshot.
func renderConsole(lines []logLine, st consoleState, strip tabStrip, width, height int, focused bool, cfg Config) string {
	innerW, innerH := width-2, height-2
	if innerW <= 0 || innerH <= 0 {
		return ""
	}
	total := len(lines)
	scroll := consoleViewport(total, st, innerH)
	start, end := consoleWindow(total, st.offset, innerH)
	rows := make([]string, 0, innerH)
	for i := start; i < end; i++ {
		rows = append(rows, consoleRow(lines[i], i == st.cursor, innerW, cfg))
	}
	if total == 0 {
		// An empty console says so rather than standing blank (A6).
		rows = append(rows, consoleEmptyLine)
	}
	box := renderTabbedBox(strip, width, height, focused, cfg, strings.Join(rows, "\n"))
	box = withScrollIndicator(box, scroll, cfg, focused)
	return withPositionCounter(box, listPosition{Cursor: st.cursor, Total: total}, cfg, focused)
}

// consoleRow renders one snapshot line, reusing the shared active-row
// treatment so the console's cursor looks like every other panel's cursor
// and follows the same operator setting. Log lines are prose, not table
// rows, so no selection column is reserved. An error line is drawn as one
// (renderErrorLine); the stored text, and so what Enter copies, is never
// edited here.
func consoleRow(line logLine, cursorRow bool, innerW int, cfg Config) string {
	prefix := rowPrefix(true, cursorRow, false, false, cfg)
	text := renderLaneLine(line, innerW-ansi.StringWidth(prefix), cfg)
	return renderRowState(prefix+text, innerW, cursorRow, cfg)
}
