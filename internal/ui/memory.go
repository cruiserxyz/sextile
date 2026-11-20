package ui

import (
	"slices"

	tea "github.com/charmbracelet/bubbletea"
)

// Memory (specs/details-follows-its-tab.md C12, C13).
//
// Each cached surface (the text frame, the Details report, the Timeline, a
// template tab) keeps its last committed results in memory, keyed by the FULL
// key the result was computed for, at most memoryBound per surface, the least
// recently used dropped first. It lives in the Model and nowhere else: nothing
// here opens a file, and a session never carries it across a restart.
//
// A wanted key that has an entry is adopted in the frame of the key, through
// the same code a fresh accepted result lands by, so the effects are the same;
// what differs is that no process starts, and that the surface's run and commit
// floor are fenced first, so no result of an earlier run can commit afterwards.
// A hit writes to the large log the lines that keep the drawn result traceable,
// in the order a fresh commit writes them: the stored argv lines, then the trace
// lines that describe the result itself (the rule applied after Astrolog: a
// cross report's kept rows, the stars an influence chart leaves out, a
// template's verdicts and slot branches). A prefetched result is never
// committed fresh, so drawing it is when they are written. They are written
// whoever asked: a want the minute tick registered is quiet for the run it
// started, and a hit is the operator's own move back to a result. Nothing goes
// to the Log panel: a revisit is not a new failure or a new request.
//
// Only a result that drew without complaint is remembered. A failure, or a
// result that carried one beside its rows, is not a thing to show again; asking
// again is how it gets a second chance.

// memoryBound is how many results each surface remembers.
const memoryBound = 64

type lruItem[K comparable, V any] struct {
	key K
	val V
}

// lru is a bounded, least-recently-used map. It is a value: every operation
// returns the new state and leaves the receiver as it was, so a Model copied
// before an Update never sees what the Update remembered.
type lru[K comparable, V any] struct {
	items []lruItem[K, V] // oldest first
}

func (c lru[K, V]) len() int { return len(c.items) }

func (c lru[K, V]) index(k K) int {
	for i := range c.items {
		if c.items[i].key == k {
			return i
		}
	}
	return -1
}

// peek is the value for k without counting as a use.
func (c lru[K, V]) peek(k K) (V, bool) {
	if i := c.index(k); i >= 0 {
		return c.items[i].val, true
	}
	var zero V
	return zero, false
}

// use is the value for k, with k now the most recently used.
func (c lru[K, V]) use(k K) (V, lru[K, V], bool) {
	i := c.index(k)
	if i < 0 {
		var zero V
		return zero, c, false
	}
	it := c.items[i]
	items := make([]lruItem[K, V], 0, len(c.items))
	items = append(items, c.items[:i]...)
	items = append(items, c.items[i+1:]...)
	items = append(items, it)
	return it.val, lru[K, V]{items: items}, true
}

// put stores v under k as the most recently used, dropping the least recently
// used entry when there are more than memoryBound.
func (c lru[K, V]) put(k K, v V) lru[K, V] {
	items := make([]lruItem[K, V], 0, min(len(c.items)+1, memoryBound))
	for _, it := range c.items {
		if it.key != k {
			items = append(items, it)
		}
	}
	items = append(items, lruItem[K, V]{key: k, val: v})
	if len(items) > memoryBound {
		items = slices.Clone(items[len(items)-memoryBound:])
	}
	return lru[K, V]{items: items}
}

// memory is the four surfaces' results.
type memory struct {
	text     lru[textKey, FirstFrameReadyMsg]
	report   lru[detailsKey, detailsPayload]
	timeline lru[timelineKey, timelinePayload]
	template lru[templateKey, templatePayload]
	// voids are the void searches that landed with a timeline search, by what
	// they were asked (spec L6); lookup holds none.
	voids voidMemory
	// hits counts the results drawn from memory, per surface, for the
	// coverage of the every-frame test.
	hits [numSurfaces]int
	// prefetched are the keys the idle prefetch put in memory that no fresh
	// result has replaced and no draw has used yet (newest last, bounded); drawn
	// counts the ones a draw used, for the every-frame test's coverage.
	prefetched []any
	drawn      int
}

// forgetMemory drops every remembered result: a workspace identity changed
// (suspendDetails, which every session reset and every chart or city pick goes
// through), so no result computed for the old one may be drawn again.
func (m Model) forgetMemory() Model {
	m.mem = memory{hits: m.mem.hits, drawn: m.mem.drawn}
	// What the prefetch was computing belonged to the workspace that is gone,
	// and the keys it found failing are not known to fail in the next one.
	m = m.stopPrefetch()
	m.pre.failed = nil
	return m
}

// markPrefetched notes that key entered memory by the idle prefetch.
func (mem memory) markPrefetched(key any) memory {
	list := append(slices.Clone(mem.prefetched), key)
	if len(list) > 4*memoryBound {
		list = slices.Clone(list[len(list)-4*memoryBound:])
	}
	mem.prefetched = list
	return mem
}

// unmarkPrefetched drops key from the prefetched ones and says whether it was
// there: a fresh result replaced it, or a draw used it.
func (mem memory) unmarkPrefetched(key any) (memory, bool) {
	i := slices.Index(mem.prefetched, key)
	if i < 0 {
		return mem, false
	}
	list := slices.Clone(mem.prefetched)
	mem.prefetched = slices.Delete(list, i, i+1)
	return mem, true
}

// drawnFromMemory counts a draw of key from memory as a prefetched one when the
// entry was.
func (m Model) drawnFromMemory(key any) Model {
	var was bool
	if m.mem, was = m.mem.unmarkPrefetched(key); was {
		m.mem.drawn++
	}
	return m
}

// fenceForMemory is what a result adopted from memory does to its surface
// before it lands: any run for another key is cancelled, and the run and commit
// floor fences move to the surface's present want, so a result of an earlier
// run (a stale success is the text surface's burst frame) can never commit
// afterwards. The want itself does not move.
func (m Model) fenceForMemory(s surface) Model {
	r := &m.refresh[s]
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	r.runningRunID++
	r.runningWantID = r.wantID
	r.commitFloor = r.wantID
	r.lastCommitWantID = r.wantID
	return m
}

// rememberText keeps a committed text frame under the key it was computed for.
func (m Model) rememberText(msg FirstFrameReadyMsg) Model {
	if !msg.keyed || msg.Err != nil || msg.StationErr != nil {
		return m
	}
	for _, c := range msg.Columns {
		if c.Err != nil {
			return m
		}
	}
	m.mem.text = m.mem.text.put(msg.key, msg)
	m.mem, _ = m.mem.unmarkPrefetched(msg.key)
	return m
}

// rememberReport keeps a landed Details report under its key.
func (m Model) rememberReport(p detailsPayload) Model {
	m.mem.report = m.mem.report.put(p.key, p)
	m.mem, _ = m.mem.unmarkPrefetched(p.key)
	return m
}

// rememberTimeline keeps a landed Timeline list under its key, unless one of
// its searches failed beside the rows of the others.
func (m Model) rememberTimeline(p timelinePayload) Model {
	if len(p.failures) > 0 {
		return m
	}
	m.mem.timeline = m.mem.timeline.put(p.key, p)
	return m
}

// rememberTemplate keeps a landed template result under its key.
func (m Model) rememberTemplate(p templatePayload) Model {
	m.mem.template = m.mem.template.put(p.key, p)
	m.mem, _ = m.mem.unmarkPrefetched(p.key)
	return m
}

// adoptTextFromMemory draws the text frame the Model asks for from memory, in
// this frame, when memory has it. It starts no process; the commands it
// returns are what the commit owes Details, the Timeline and the wheel.
func (m Model) adoptTextFromMemory() (Model, tea.Cmd, bool) {
	key, ok := m.textKeyFor()
	if !ok {
		return m, nil, false
	}
	msg, used, ok := m.mem.text.use(key)
	if !ok {
		return m, nil, false
	}
	m.mem.text = used
	m.mem.hits[surfaceText]++
	m = m.drawnFromMemory(key)
	m = m.fenceForMemory(surfaceText)
	// The frame lands as a fresh one asked for now: this want, this run, and the
	// floating moment the screen asks for (a chart cast alone does not read it,
	// so the stored frame's moment may be another).
	msg.RunID, msg.WantID = m.refresh[surfaceText].runningRunID, m.refresh[surfaceText].wantID
	msg.TransitTime = transitTimeFromWorkspace(m)
	m, cmd := applyTextCommit(m, msg)
	return m, cmd, true
}

// reportInMemory says the report the Details tab asks for is remembered.
func (m Model) reportInMemory() bool {
	key, ok := m.detailsKeyFor()
	if !ok {
		return false
	}
	_, found := m.mem.report.peek(key)
	return found
}

// adoptReportFromMemory draws the Details report the Model asks for from
// memory, in this frame, when memory has it.
func (m Model) adoptReportFromMemory() (Model, bool) {
	key, ok := m.detailsKeyFor()
	if !ok {
		return m, false
	}
	p, used, ok := m.mem.report.use(key)
	if !ok {
		return m, false
	}
	m.mem.report = used
	m.mem.hits[surfaceDetails]++
	m = m.drawnFromMemory(key)
	m = m.fenceForMemory(surfaceDetails)
	m.details.timerArmed = false
	m.keyRun.seq++ // the key changed what Details describes, as a scheduled run would
	m = m.logCommands(p.commands)
	m = m.replayReportTrace(p)
	return m.landReport(p), true
}

// adoptTimelineFromMemory draws the Timeline list for key from memory.
func (m Model) adoptTimelineFromMemory(key timelineKey) (Model, bool) {
	p, used, ok := m.mem.timeline.use(key)
	if !ok {
		return m, false
	}
	m.mem.timeline = used
	m.mem.hits[key.list.surface()]++
	m = m.fenceForMemory(key.list.surface())
	m = m.logCommands(p.commands)
	return m.landTimeline(p), true
}

// adoptTemplateFromMemory draws the template tab for key from memory.
func (m Model) adoptTemplateFromMemory(key templateKey) (Model, bool) {
	p, used, ok := m.mem.template.use(key)
	if !ok {
		return m, false
	}
	m.mem.template = used
	m.mem.hits[surfaceTemplate]++
	m = m.drawnFromMemory(key)
	m = m.fenceForMemory(surfaceTemplate)
	wantID := m.refresh[surfaceTemplate].wantID
	m = m.logCommands(p.commands)
	if p.cycle != nil && cycleKeyOf(p.key) == cycleKeyOf(m.tpl.key) {
		m.tpl.cycle, m.tpl.cycleFor = p.cycle, cycleKeyOf(p.key)
	}
	m.tpl.done, m.tpl.hasDone = p.key, true
	// The template's verdict and slot-branch traces follow the argv lines, as a
	// fresh result's do.
	return m.landTemplate(p, wantID, true), true
}
