package ui

import (
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
)

// The Log takes the focus with `,L`. Like the console it is a mode beside the
// ring, never m.focused: Details takes its subject from m.focused, the wheel
// and every refresh read it, and none of them may notice the Log being read.
// Entering and leaving start no work and defer none.
//
// While it holds the keyboard the Log lists every line of the panel (notes and
// errors, no argv), moves a line cursor over them, copies the line (Enter) or
// all of them (`,y`), and opens the console (`l`). Every key it does not own
// gives the focus back and then runs from the screen it came from.

// logFocusState is the Log panel with the keyboard: whether it has it, which
// line of the panel's lines the cursor is on, and which line the visible rows
// start at. Ephemeral Model state, never written to a file.
type logFocusState struct {
	active bool
	cursor int
	offset int
}

// logFocusView is what compose draws the focused Log from.
type logFocusView struct {
	active bool
	cursor int
	offset int
	lines  []logLine
}

func (m Model) logFocusView() logFocusView {
	if !m.logFocus.active {
		return logFocusView{}
	}
	return logFocusView{active: true, cursor: m.logFocus.cursor, offset: m.logFocus.offset, lines: m.logPanelEntries()}
}

// logPanelEntries are the Log panel's lines as stored: the lanes the panel
// shows, in arrival order.
func (m Model) logPanelEntries() []logLine {
	out := make([]logLine, 0, len(m.logLines))
	for _, l := range m.logLines {
		if !l.cleared && l.lane != laneArgv && l.lane != laneWarning && l.lane != laneNow && l.lane != laneTrace {
			out = append(out, l)
		}
	}
	return out
}

// logInnerHeight is how many lines the side Log shows, 0 with no Log panel.
func (m Model) logInnerHeight() int {
	r, ok := m.geometry()[PanelLog]
	if !ok {
		return 0
	}
	return max(r.h-2, 0)
}

// focusLog is `,L`: with the Log on screen it takes the focus, cursor on the
// newest line, and the error mark is answered as opening the console answers
// it. Pressed while the Log is focused, or with no Log panel on screen, it
// opens the console.
func (m Model) focusLog() (tea.Model, tea.Cmd) {
	if m.logFocus.active || !m.logOnScreen() {
		return m.openConsole()
	}
	m.errorUnseen = false
	m.errorKeptView = false
	n := len(m.logPanelLines())
	m.logFocus = logFocusState{active: true, cursor: max(n-1, 0)}
	m.logFocus.offset = max(n-m.logInnerHeight(), 0)
	return m, nil
}

// settleLogFocus holds the cursor inside the lines and the window around it,
// and gives the focus back when a resize left no Log panel on screen.
func (m Model) settleLogFocus() Model {
	if !m.logFocus.active {
		return m
	}
	if !m.logOnScreen() {
		m.logFocus = logFocusState{}
		return m
	}
	n := len(m.logPanelLines())
	m.logFocus.cursor = min(max(m.logFocus.cursor, 0), max(n-1, 0))
	m.logFocus.offset = menu.ScrollToShow(m.logFocus.offset, m.logFocus.cursor, n, m.logInnerHeight())
	return m
}

// logBindings is the focused Log's table. The leader's `y` is the leader's
// (updateLeaderKey), so its entry matches no key of its own.
func logBindings() []keys.BindingEntry {
	return keys.TitledLocal([]keys.BindingEntry{
		{ID: keys.BindingListUp, Binding: key.NewBinding(key.WithKeys("up", "k")), Title: "Move row up", BarKey: "up", When: func(c keys.Context) bool { return c.Log.Lines > 1 }},
		{ID: keys.BindingListDown, Binding: key.NewBinding(key.WithKeys("down", "j")), Title: "Move row down", BarKey: "down"},
		{ID: keys.BindingConfirm, Binding: key.NewBinding(key.WithKeys("enter")), Title: "Copy line", BarKey: "enter", Bar: true, When: func(c keys.Context) bool { return c.Log.Lines > 0 }},
		{ID: keys.BindingCopy, Binding: key.NewBinding(), Title: "Copy all", BarKeyFn: chordKey("y"), BarKey: ", y", BarWhen: leaderSet, Bar: true, When: func(c keys.Context) bool { return c.Log.Lines > 0 }},
		{ID: keys.BindingConsole, Binding: key.NewBinding(key.WithKeys("l")), Title: "Console", BarKey: "l"},
		{ID: keys.BindingPageUp, Title: "Page list up", Binding: key.NewBinding(key.WithKeys("pgup")), BarKey: "pgup"},
		{ID: keys.BindingPageDown, Binding: key.NewBinding(key.WithKeys("pgdown")), Title: "Page list down", BarKey: "pgdown"},
		{ID: keys.BindingPageHome, Title: "First row", Binding: key.NewBinding(key.WithKeys("home", "g")), BarKey: "home/g"},
		{ID: keys.BindingPageEnd, Title: "Last row", Binding: key.NewBinding(key.WithKeys("end", "G")), BarKey: "end/G"},
		{ID: keys.BindingClose, Binding: key.NewBinding(key.WithKeys("esc", "q")), Title: "Back", BarKey: "esc", Bar: true, Pinned: true},
	})
}

// updateLogKey owns the keyboard while the Log is focused. Its own keys are in
// logBindings; the leader works (`,y` copies, `,L l` opens the console); every
// other key gives the focus back and then runs from the screen it came from,
// so no key is dead and none acts on a panel that does not look focused.
func (m Model) updateLogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.leader.pending {
		return m.updateLeaderKey(msg)
	}
	if m.leaderKey != "" && msg.String() == m.leaderKey {
		return m.armLeader()
	}
	// ⌃y is `, y`: the Log keeps the keyboard while it copies.
	if key.Matches(msg, m.keys.Copy) {
		return m.copyAll()
	}
	switch keys.MatchBinding(msg, m.available(ownerLog, logBindings())) {
	case keys.BindingClose:
		m.logFocus = logFocusState{}
		return m, nil
	case keys.BindingListUp:
		return m.moveLogCursor(-1), nil
	case keys.BindingListDown:
		return m.moveLogCursor(+1), nil
	case keys.BindingPageUp:
		return m.pageLog(menu.PageUp), nil
	case keys.BindingPageDown:
		return m.pageLog(menu.PageDown), nil
	case keys.BindingPageHome:
		return m.pageLog(menu.PageHome), nil
	case keys.BindingPageEnd:
		return m.pageLog(menu.PageEnd), nil
	case keys.BindingConfirm:
		return m.yankLogLine()
	case keys.BindingConsole:
		return m.openConsole()
	}
	// The hidden door of the Experiments popup: in no table, bar or Keybindings popup.
	if isPlainKey(msg, "p") {
		return m.openExperiments()
	}
	m.logFocus = logFocusState{}
	return m.dispatchKey(msg)
}

func (m Model) moveLogCursor(delta int) Model {
	n := len(m.logPanelLines())
	if n == 0 {
		return m
	}
	m.logFocus.cursor = min(max(m.logFocus.cursor+delta, 0), n-1)
	m.logFocus.offset = menu.ScrollToShow(m.logFocus.offset, m.logFocus.cursor, n, m.logInnerHeight())
	return m
}

func (m Model) pageLog(op menu.PageOp) Model {
	n := len(m.logPanelLines())
	if n == 0 {
		return m
	}
	m.logFocus.cursor, m.logFocus.offset = menu.PageCursor(m.logFocus.cursor, m.logFocus.offset, n, m.logInnerHeight(), op)
	return m
}

// yankLogLine copies the stored text of the highlighted line, in full, through
// the console's seam and with its receipt; it writes no Log line.
func (m Model) yankLogLine() (tea.Model, tea.Cmd) {
	lines := m.logPanelLines()
	if m.logFocus.cursor < 0 || m.logFocus.cursor >= len(lines) {
		return m.showNotice(noticeNothingCopy)
	}
	text := lines[m.logFocus.cursor]
	if err := yankLine(text); err != nil {
		return m.logError("log", "the line did not reach the clipboard").showNotice(noticeCopyFailed)
	}
	m.yank.gen++
	m.yank.active = true
	m.yank.notice = ""
	m.yank.chars = utf8.RuneCountInString(text)
	return m, armYankFlash(m.yank.gen)
}

// whyNothingToCopy is the reason the Copy action gives when it has nothing.
const whyNothingToCopy = "nothing to copy"

// copyPlan is what `,y` copies from the owner that has the keyboard: the lines,
// the Log note's scope and noun, and which panel it was. Attached Details
// copies as it always did (yankDetails) and has no plan here.
type copyPlan struct {
	lines   []string
	scope   string
	noun    string
	console bool
}

func (m Model) copyPlan() copyPlan {
	switch m.currentOwner() {
	case ownerLog:
		return copyPlan{lines: m.logPanelLines(), scope: "log", noun: "line"}
	case ownerConsole:
		lines := make([]string, len(m.console.snapshot))
		for i, l := range m.console.snapshot {
			lines[i] = l.text
		}
		return copyPlan{lines: lines, scope: "log", noun: "line", console: true}
	}
	switch m.focused {
	case PanelChart:
		return copyPlan{lines: m.copyRows(PanelChart), scope: "chart", noun: "row"}
	case PanelBodies:
		return copyPlan{lines: m.copyRows(PanelBodies), scope: "bodies", noun: "row"}
	case PanelAspects:
		return copyPlan{lines: m.copyRows(PanelAspects), scope: "aspects", noun: "aspect"}
	}
	return copyPlan{}
}

// copyBlocked is why Copy cannot run now, "" when it can.
func (m Model) copyBlocked() string {
	if m.currentOwner() == ownerDetails {
		if isListTab(m.activeTabID(PanelDetails)) {
			if len(m.copiedRows()) == 0 {
				return whyNothingToCopy
			}
		} else if m.detailsBodyShown() == "" {
			return whyNothingToCopy
		}
		return ""
	}
	if len(m.copyPlan().lines) == 0 {
		return whyNothingToCopy
	}
	return ""
}

// copyAll is `,y`: the whole content of whatever has the keyboard, through the
// same clipboard seam and receipt as every other copy, and one Log note.
func (m Model) copyAll() (tea.Model, tea.Cmd) {
	if m.currentOwner() == ownerDetails {
		return m.yankDetails()
	}
	p := m.copyPlan()
	if len(p.lines) == 0 {
		return m.showNotice(noticeNothingCopy)
	}
	text := strings.Join(p.lines, "\n")
	if err := yankLine(text); err != nil {
		return m.logCopyFailed(p).showNotice(noticeCopyFailed)
	}
	noun := p.noun
	if len(p.lines) != 1 {
		noun += "s"
	}
	m = m.logNote(p.scope, "%d %s copied", len(p.lines), noun)
	m.yank.gen++
	m.yank.active = true
	m.yank.notice = ""
	m.yank.chars = utf8.RuneCountInString(text)
	return m, armYankFlash(m.yank.gen)
}

// logCopyFailed is the error line of a copy that did not reach the clipboard;
// it names the panel, one literal for each.
func (m Model) logCopyFailed(p copyPlan) Model {
	switch {
	case p.console:
		return m.logError("log", "the console lines did not reach the clipboard")
	case p.scope == "log":
		return m.logError("log", "the Log lines did not reach the clipboard")
	case p.scope == "chart":
		return m.logError("chart", "the rows did not reach the clipboard")
	case p.scope == "bodies":
		return m.logError("bodies", "the rows did not reach the clipboard")
	}
	return m.logError("aspects", "the aspects did not reach the clipboard")
}

// copyWidth is the width a copied row is laid out in: wide enough that no
// panel cuts a row, since the clipboard has no panel edge.
const copyWidth = 400

// copyRows is the whole content of a main panel as `,y` copies it: the rows the
// panel draws, each in its own text without colour, not cut to the panel's
// width, trailing spaces trimmed, one per line. Empty when the panel has
// nothing to show.
func (m Model) copyRows(p PanelID) []string {
	cfg := m.cfg
	cfg.Color = ColorNever
	switch p {
	case PanelChart:
		render := func(w int) string {
			return chartSlotsBodyLayout(m.slots, m.cursorIdx, false, m.clock.Now(), m.place, m.lens, w, cfg, m.rowTag, m.floatingTitle(), m.placeWitness())
		}
		// The row pins its tag to the right edge, so the width decides the
		// padding. The narrowest width that still shows every word the wide
		// layout shows is the row as it is, with nothing cut and no gap.
		whole := render(copyWidth)
		w := sort.Search(copyWidth, func(i int) bool { return sameWords(render(i+1), whole) }) + 1
		rows := plainRows(render(w))
		// With any chart selected (Space), only those rows go, in slot order.
		if set := m.deriveScene().set; len(set) > 0 && len(rows) == len(m.slots.All()) {
			var chosen []string
			for i, s := range m.slots.All() {
				if slices.Contains(set, s) {
					chosen = append(chosen, rows[i])
				}
			}
			return chosen
		}
		return rows
	case PanelBodies:
		v, state := m.bodiesView(), m.paintedChart()
		slot, cursor := m.cursorIdx, m.bodyCursorIdx
		if v.held {
			slot, cursor = v.slot, v.cursor
		}
		// One row of room beyond the rows, so the rule between the planets and
		// the angles is drawn (as a blank row) and no row is windowed out.
		rows := panel.BodyRowCount(state.positions, state.cusps)
		bag := resolveBodiesBag(v, state, m.slots, slot, cursor, copyWidth, rows+1, false, cfg)
		bag.Top = 0
		body, _, _ := panel.RenderBodies(bag)
		return plainRows(body)
	case PanelAspects:
		state := m.paintedChart()
		active := m.tabStrips()[PanelAspects].activeID()
		n := max(len(state.aspects), len(state.crossAspects))
		// One column wide, as many rows as aspects: the cells come out one per
		// line in the panel's own order, none left off for want of room.
		body := renderAspectsBody(state, active, panel.AspectColWidth+3, n+1, cfg)
		var out []string
		for _, row := range plainRows(body) {
			if row != panel.OwnEmptyLine && row != panel.CrossEmptyLine {
				out = append(out, row)
			}
		}
		return out
	}
	return nil
}

// sameWords says two renderings hold the same words in the same order, however
// the spaces between them fall.
func sameWords(a, b string) bool {
	return strings.Join(strings.Fields(ansi.Strip(a)), " ") == strings.Join(strings.Fields(ansi.Strip(b)), " ")
}

// plainRows splits a rendered body into rows with every escape sequence and
// trailing space gone and the blank rows (padding, the Bodies rule) dropped.
func plainRows(body string) []string {
	var out []string
	for _, row := range strings.Split(body, "\n") {
		if row = strings.TrimRight(ansi.Strip(row), " "); row != "" {
			out = append(out, row)
		}
	}
	return out
}
