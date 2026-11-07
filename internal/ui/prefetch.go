package ui

import (
	"context"
	"errors"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
)

// Prefetch (specs/details-follows-its-tab.md C15-C17).
//
// While nothing is happening, the results one key away are computed ahead, so
// the move there paints from memory (memory.go). It is a loop of its own and
// not a refresh surface: it never touches a surface's want, run or commit
// floor, it draws nothing, and the only thing it leaves behind is an entry in
// memory. The operator's own request always wins:
//
//   - an idle period starts 300 ms after a key (prefetchRest) and takes ONE
//     snapshot of the keys memory lacks, in the order of prefetchSnapshot;
//   - it computes them one Astrolog call at a time, through the same compute
//     the surfaces use, on a Model one key away, so the argv is the argv a fresh
//     run would have;
//   - the next key, or a surface starting a run, cancels the call in flight
//     and ends the period;
//   - it speaks nowhere: a result writes no Log line and no argv line (those
//     are written when the result is drawn, memory.go), and a failure is not
//     said at all; the key is only left alone from then on, until it is the
//     wanted key and the normal run fails or not in the open (C6);
//   - the period ends when every key of its snapshot has been attempted once.

// prefetchRest is how long no key must have arrived before an idle period starts.
const prefetchRest = 300 * time.Millisecond

// prefetchPolls bounds how often an idle tick waits for a busy surface before
// the period is given up: 50 ticks of prefetchRest, fifteen seconds.
const prefetchPolls = 50

// idleTimer starts the timer that ends an idle rest. Like minuteTimer it is one
// package-level variable, so the test binary can put in its place a timer that
// does not sleep, or none; production always runs longTimer.
var idleTimer = longTimer

// prefetchIdleMsg ends a rest: when no key has arrived since seq was issued,
// the idle period starts. polls counts the times it waited for a busy surface.
type prefetchIdleMsg struct {
	seq   uint64
	polls int
}

// prefetchDoneMsg carries one prefetch call's result.
type prefetchDoneMsg struct {
	seq     uint64
	cand    prefetchCand
	surface surface
	result  tea.Msg
}

// prefetchCand is one result to compute ahead: the surface that would, the full
// key it would have, and the Model one key away it is asked of.
type prefetchCand struct {
	surface surface
	key     any
	at      func(Model) Model
}

// prefetch is the loop's state. All of it is the Model's own value: the slices
// are never written in place, so a Model copied before an Update does not see
// what the Update did.
type prefetch struct {
	// seq names the idle period; every input and every stop moves it on, and a
	// message of an older one does nothing.
	seq uint64
	// cancel stops the call in flight; nil when there is none.
	cancel context.CancelFunc
	// queue is the rest of this period's snapshot.
	queue []prefetchCand
	// failed are the keys whose call failed: left alone until a workspace
	// identity changes (a failure is not retried by the prefetch, C17), so a
	// quiet screen starts no call in a second idle period either.
	failed []any
	// calls counts the calls started, for the tests and the load account.
	calls int
}

// stopPrefetch cancels the call in flight and ends the idle period.
func (m Model) stopPrefetch() Model {
	if m.pre.cancel != nil {
		m.pre.cancel()
	}
	m.pre.cancel, m.pre.queue = nil, nil
	m.pre.seq++
	return m
}

// prefetchRefused says nothing may be computed ahead now: no workspace, a
// launch the instance lock refused or Astrolog unusable (cantStart covers
// both, and Astrolog not installed), a modal owning the keyboard, or Sextile
// not past its start.
func (m Model) prefetchRefused() bool {
	return m.slots == nil || m.cantStart.any() || m.modalOpen() || m.startup != startupReady
}

// refreshBusy says some refresh surface has a run in flight, or one owed that a
// timer or a frame on its way is about to start: the operator's own work, which
// a prefetch never runs beside.
func (m Model) refreshBusy() bool {
	for s := range m.refresh {
		if m.refresh[s].cancel != nil {
			return true
		}
	}
	return m.textOutstanding() || m.details.timerArmed
}

// armIdle starts a rest for the idle period seq.
func (m Model) armIdle(polls int) tea.Cmd {
	seq := m.pre.seq
	return idleTimer(prefetchRest, func(time.Time) tea.Msg { return prefetchIdleMsg{seq: seq, polls: polls} })
}

// prefetchAfter is the loop's part of every Update, run after the reconciler
// has started what the message owed. A key (or a click) ends the period and
// waits for the next; the Model reaching ready starts the first rest; a resize or the minute tick ends it and waits for the
// next key too (a quiet screen computes nothing ahead on a clock's account);
// and a period whose call is in flight when a surface starts a run, or a
// modal opens, is stopped there.
func (m Model) prefetchAfter(msg tea.Msg, wasReady bool) (Model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg:
		m = m.stopPrefetch()
		return m, m.armIdle(0)
	case tea.WindowSizeMsg, nowTickMsg:
		return m.stopPrefetch(), nil
	}
	if m.pre.cancel != nil && (m.refreshBusy() || m.prefetchRefused()) {
		m = m.stopPrefetch()
	}
	// Reaching ready (the first frame committed, or a modal closed by a message
	// that is no key) is a rest too: the first key of a run may be the one that
	// asks for what the first idle period would have computed.
	if !wasReady && m.startup == startupReady && m.slots != nil {
		return m, m.armIdle(0)
	}
	return m, nil
}

// onPrefetchIdle starts the idle period its rest ended, unless a key came, a
// call is out, or something forbids it. A busy surface is waited for.
func (m Model) onPrefetchIdle(msg prefetchIdleMsg) (Model, tea.Cmd) {
	if msg.seq != m.pre.seq || m.pre.cancel != nil || len(m.pre.queue) > 0 || m.prefetchRefused() {
		return m, nil
	}
	if m.refreshBusy() {
		if msg.polls >= prefetchPolls {
			return m, nil
		}
		return m, m.armIdle(msg.polls + 1)
	}
	m.pre.queue = m.prefetchSnapshot()
	return m.prefetchNext()
}

// prefetchNext starts the next call of the snapshot that still lacks its
// result; the period ends when none is left.
func (m Model) prefetchNext() (Model, tea.Cmd) {
	for len(m.pre.queue) > 0 {
		c := m.pre.queue[0]
		m.pre.queue = m.pre.queue[1:]
		if m.prefetchHas(c) || slices.Contains(m.pre.failed, c.key) {
			continue // a draw or a fresh run gave it since the snapshot
		}
		ctx, cancel := context.WithCancel(context.Background())
		cmd := m.prefetchCall(ctx, c)
		if cmd == nil {
			cancel() // nothing to ask: attempted, and left alone
			m.pre.failed = append(slices.Clone(m.pre.failed), c.key)
			continue
		}
		m.pre.cancel = cancel
		m.pre.calls++
		return m, cmd
	}
	return m, nil
}

// prefetchCall is the compute of c, on the Model one key away, as the surface
// itself runs it.
func (m Model) prefetchCall(ctx context.Context, c prefetchCand) tea.Cmd {
	hyp := c.at(m)
	var inner tea.Cmd
	switch c.surface {
	case surfaceText:
		inner = buildTextCmd(hyp, ctx, 0, 0)
	case surfaceDetails:
		inner = buildDetailsCmd(hyp, ctx, 0, 0)
	case surfaceTemplate:
		inner = templateCmd(hyp, ctx, 0, 0)
	}
	if inner == nil {
		return nil
	}
	seq := m.pre.seq
	return func() tea.Msg {
		return prefetchDoneMsg{seq: seq, cand: c, surface: c.surface, result: inner()}
	}
}

// prefetchHas says memory already holds the result c would compute.
func (m Model) prefetchHas(c prefetchCand) bool {
	switch k := c.key.(type) {
	case textKey:
		_, ok := m.mem.text.peek(k)
		return ok
	case detailsKey:
		_, ok := m.mem.report.peek(k)
		return ok
	case templateKey:
		_, ok := m.mem.template.peek(k)
		return ok
	}
	return false
}

// onPrefetchDone files one call's result in memory, silently, and goes on to the
// next call. A result of an older period (a key, a surface or an identity
// change ended it) is dropped, and a call that failed leaves its key alone.
func (m Model) onPrefetchDone(msg prefetchDoneMsg) (Model, tea.Cmd) {
	if msg.seq != m.pre.seq {
		return m, nil
	}
	if m.pre.cancel != nil {
		m.pre.cancel()
	}
	m.pre.cancel = nil
	m = m.fileForLater(msg)
	if !m.prefetchHas(msg.cand) {
		m.pre.failed = append(slices.Clone(m.pre.failed), msg.cand.key)
	}
	if m.refreshBusy() || m.prefetchRefused() {
		m.pre.queue = nil
		return m, nil
	}
	return m.prefetchNext()
}

// fileForLater puts a successful result in the surface's memory, with the argv
// lines and trace notes it carries (the draw replays them), and marks it as
// prefetched. A failed result, or one the surface does not remember, files
// nothing.
func (m Model) fileForLater(msg prefetchDoneMsg) Model {
	switch r := msg.result.(type) {
	case FirstFrameReadyMsg:
		if errors.Is(r.Err, context.Canceled) {
			return m
		}
		m = m.rememberText(r)
	case refreshResultMsg:
		if r.err != nil {
			return m
		}
		switch p := r.payload.(type) {
		case detailsPayload:
			m = m.rememberReport(p)
		case templatePayload:
			m = m.rememberTemplate(p)
		}
	}
	if m.prefetchHas(msg.cand) {
		m.mem = m.mem.markPrefetched(msg.cand.key)
	}
	return m
}

// ---- the snapshot ----------------------------------------------------------

// prefetchSnapshot is the keys one key away that memory lacks, in this order and
// without a key twice:
//
//  1. for every reportable Bodies row of the focused chart, its report and,
//     where the row offers a template tab, the template's result;
//  2. for every chart in the Chart list, the text frame with only the Chart
//     cursor moved there, and that chart's Chart report;
//  3. when the focused chart is the floating one, its text frame and the active
//     Details tab's result (the report or the template, never the Timeline) a
//     day on and a day back;
//  4. the current Bodies row's report and template result, when Bodies is not
//     the focused panel. (The current Aspects tab's frame is the text frame the
//     screen shows, which memory holds from its commit: nothing to add.)
//
// The Timeline is never computed ahead.
func (m Model) prefetchSnapshot() []prefetchCand {
	var out []prefetchCand
	seen := map[any]bool{}
	add := func(c prefetchCand, ok bool) {
		if !ok || seen[c.key] || m.prefetchHas(c) || slices.Contains(m.pre.failed, c.key) {
			return
		}
		seen[c.key] = true
		out = append(out, c)
	}
	details := m.detailsVisible() && m.detailsLive()
	report := func(at func(Model) Model) (prefetchCand, bool) {
		if !details {
			return prefetchCand{}, false
		}
		hyp := at(m)
		k, ok := hyp.detailsKeyFor()
		return prefetchCand{surface: surfaceDetails, key: k, at: at}, ok && hyp.unresolvedFor(hyp.focusedSlot()) == nil
	}
	template := func(at func(Model) Model) (prefetchCand, bool) {
		if !details {
			return prefetchCand{}, false
		}
		hyp := at(m)
		k, ok := hyp.templateKeyFor()
		return prefetchCand{surface: surfaceTemplate, key: k, at: at}, ok && hyp.unresolvedFor(hyp.focusedSlot()) == nil
	}
	text := func(at func(Model) Model) (prefetchCand, bool) {
		k, ok := at(m).textKeyFor()
		return prefetchCand{surface: surfaceText, key: k, at: at}, ok
	}
	onBodies := func(row int) func(Model) Model {
		return func(m Model) Model { m.focused, m.bodyCursorIdx = PanelBodies, row; return m }
	}

	// (1) every reportable row of the shown chart's Bodies.
	ch := m.shownChart()
	for row := range panel.BodyRowCount(ch.positions, ch.cusps) {
		if _, ok := panel.ObjectAtCursor(ch.positions, ch.cusps, row); !ok {
			continue
		}
		add(report(onBodies(row)))
		add(template(onBodies(row)))
	}

	// (2) every chart of the list, the cursor moved there and nothing else.
	for i := range m.slots.All() {
		onChart := func(m Model) Model { m.cursorIdx = i; return m }
		add(text(onChart))
		add(report(func(m Model) Model { m = onChart(m); m.focused = PanelChart; return m }))
	}

	// (3) the floating chart a day on and a day back.
	if m.focusedIsFloating() {
		for _, dir := range []int{+1, -1} {
			years, months, days, dur := stepDelta(unitDay, dir)
			target := advanceTimeForPlace(transitTimeFromWorkspace(m), m.slots.Floating().Location, years, months, days, dur)
			if lookup.BeforeJulianDayZero(target) {
				continue
			}
			moved := func(m Model) Model {
				m.targetFloatingTime = target
				m.slots = m.slots.FloatingAt(target)
				return m
			}
			add(text(moved))
			switch id := m.activeTabID(PanelDetails); {
			case id == TabDetailsReport:
				add(report(moved))
			case isTemplateTab(id):
				add(template(moved))
			}
		}
	}

	// (4) the current Bodies row, when another panel is focused.
	if m.focused != PanelBodies {
		current := onBodies(m.bodyCursorIdx)
		add(report(current))
		add(template(current))
	}
	return out
}
