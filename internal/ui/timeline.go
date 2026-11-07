package ui

import (
	"context"
	"fmt"
	"github.com/charmbracelet/bubbles/key"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
)

// The Details timeline tab (the specification): every dated
// event Astrolog's -t, -d and -tr searches emit for the floating slot's
// month, in the floating slot's clock, one row per event and each row
// Astrolog's own line. Enter on a row moves the floating slot to that instant
// through the ordinary time-step path, and the list stays where it was.

// timelineKey is every search input (C10): the floating slot's month and
// year in its own zone, that zone, and the anchored charts the selected set
// merges — which ones, in which order, each with its own natal moment and
// place. Two equal keys mean the
// same searches; nothing else may make the timeline recompute (C25).
type timelineKey struct {
	year  int
	month time.Month
	zone  string
	// natals is the comparing timeline's ordered set of anchored natals —
	// each one's numeral, moment, place and chart identity — in one string so
	// the key stays comparable. Empty while single (the sky alone). Moving the
	// cursor within the selected set changes none of it, so it changes nothing
	// here (C9); a chart of the same moment and place but another identity, or
	// another numeral, is another input (review round 2).
	natals string
	// objects, aspects and nodes are the COMMITTED body policy, aspect policy
	// and node mode the searches carry (C13). They are strings so the key
	// stays comparable: a policy holds maps and slices.
	objects, aspects string
	trueNode         bool
	// skyMarks is the floating slot's sorted marks, which narrow the sky
	// search; each natal's own are in natals. A mark change re-asks, a tick-only
	// change is objects (F14).
	skyMarks string
	// houses is the system the comparing searches cast the natals in: ①'s own,
	// whatever the cursor or the frame. Zero (and unused) while single, where
	// the sky alone is searched and no chart is cast (the specification C4).
	houses recipe.HouseSystem
	// zodiac is the zodiac every search is cast in: ①'s while comparing (the
	// zodiac stays the first chart's, S4b B2/B3), the cursor chart's otherwise.
	// Unlike the houses it matters to the sky alone as well, since the
	// ingresses and aspects of -d read the zodiac they are searched in (the
	// specification C10).
	zodiac recipe.Zodiac
}

// chooserKey is a lens's search-relevant part in comparable form. Order-free
// inputs (an ObjectSet, the order ticks were collected in) are normalised, so the
// same policy always has the same key. It does not resolve a delta policy and
// an explicit one to a common set (that needs Astrolog's defaults), so a modal
// that re-confirms the same visible bodies as an explicit selection re-runs the
// searches once.
func chooserKey(l lookup.Lens) (objects, aspects string, trueNode bool) {
	o := l.Objects
	objects = fmt.Sprintf("%d|%v|%v|%v", int(o.Baseline), o.Add.Members(), o.Remove.Members(), o.Selected())
	aspects = fmt.Sprintf("%t|%v", l.Aspects.Explicit, l.Aspects.Selected())
	return objects, aspects, o.TrueNodeMode
}

// timelineObjects is the object policy of one slot's searches: the committed
// lens's objects with the slot's own marks as Only, the same narrowing
// showLens gives the Aspects list (read from the committed lens, never from a
// chooser preview). A slot with no marks carries no Only.
func (m Model) timelineObjects(lens lookup.Lens, s *store.Slot) recipe.ObjectPolicy {
	o := lens.Objects
	o.Only = nil
	if m.slots == nil || s == nil {
		return o
	}
	if sel := m.slots.SelectedObjects(m.slotIndexOf(s)); len(sel) > 0 {
		nums := make([]int, len(sel))
		for i, ob := range sel {
			nums[i] = ob.Number
		}
		o.Only = recipe.NewObjectSet(nums...)
	}
	return o
}

// committedLens is the lens the timeline follows: the one the operator last
// confirmed. While the aspect or body chooser is open its live preview rewrites
// m.lens on every toggle, and a preview must spawn no timeline search (C15), so
// the lens captured when the chooser opened stands until Enter commits.
func (m Model) committedLens() lookup.Lens {
	if m.startup == startupAspectOpen || m.startup == startupBodyOpen {
		return m.chooserEntry.lens
	}
	return m.lens
}

// timelineState is the list Details shows on the timeline tab and the inputs
// it belongs to. rows are only ever rows computed for key: a change of key
// starts a fresh list, and the previous inputs' rows move to kept, which is
// drawn dimmed and never acted on (C44, amended by
// the specification rule 4). No result is
// cached: a key that comes back is searched again.
type timelineState struct {
	key      timelineKey
	keyed    bool
	rows     []lookup.TimelineRow
	complete bool
	// kept is the last complete list, for the PREVIOUS inputs, shown dimmed
	// while this key's search is in flight (the specification
	// rule 4). It is display only: no action reads it, and rows stays the
	// one list that belongs to key. A failed search drops it (rule 8).
	kept []lookup.TimelineRow
	// failed says this key's search ended in failure, so nothing is awaited.
	failed bool
	// content is the key of the list the tab can draw: rows once complete,
	// kept before. hasContent says there is one. The view rule reads it
	// (timelinePaneState): the list is drawn bright for its own key, faint for
	// another version of the same subject, and not at all for another subject.
	content    timelineKey
	hasContent bool
	// marks are the rows Space marked for `,y`. They belong to this key's
	// list, so a new month's load (a fresh timelineState) clears them, and
	// they are keyed by row identity, never by index.
	marks map[timelineMark]bool
	// query is the `/` filter and searching says its line is open for typing.
	// Both belong to this key's list: a new month's load is a fresh
	// timelineState, which is the one automatic way the query empties
	// (the specification rule 3a).
	query     string
	searching bool
}

// timelineMark names one row: its instant, Astrolog's own line and the
// numeral of its slot, because two selected slots can print the identical line
// at one instant (C10) and a mark on one must not mark the other.
type timelineMark struct {
	instant time.Time
	line    string
	numeral int
	// early says Enter landed this row on its minute's FIRST second (an S/R
	// station, C14 of the specification); the zero value is the last.
	early bool
}

// landed is the instant Enter put the moment on for this row.
func (k timelineMark) landed() time.Time {
	if k.early {
		return k.instant
	}
	return landedAt(k.instant)
}

func markOf(r lookup.TimelineRow) timelineMark {
	return timelineMark{r.Instant.UTC(), r.Line, r.Numeral, landsEarly(r)}
}

// toggleTimelineMark is Space on the timeline tab: mark or unmark the cursor
// row. The map is copied, so a Model copy never shares marks with this one.
func (m Model) toggleTimelineMark() Model {
	rows := m.timelineShown()
	c := m.details.cursor
	if m.activeTabID(PanelDetails) != TabDetailsTimeline || c < 0 || c >= len(rows) {
		return m
	}
	mk := markOf(rows[c])
	next := make(map[timelineMark]bool, len(m.timeline.marks)+1)
	for k := range m.timeline.marks {
		next[k] = true
	}
	if next[mk] {
		delete(next, mk)
	} else {
		next[mk] = true
	}
	m.timeline.marks = next
	return m
}

// timelinePayload is surfaceTimeline's result, stamped with the key it was
// computed for so a late result for inputs the operator has since left is
// discarded at commit (§ Failure modes, the stale-result guard).
type timelinePayload struct {
	key      timelineKey
	rows     []lookup.TimelineRow
	commands []string
	// failures are the searches that failed beside the rows of the others, and
	// labels name each selected slot by its numeral and chart name as it stood
	// when the search was asked for (C17).
	failures []lookup.TimelineFailure
	labels   map[int]string
}

func (timelinePayload) isRefreshPayload() {}

// timelineRows is the list as it may be SHOWN or acted on: the rows, but
// only while they belong to the search inputs as they stand right now. The
// inputs change the moment a new client is anchored or a city is picked —
// before any compute commits and before syncTimeline runs — so gating on the
// live key rather than on a clear somewhere downstream is what keeps the
// previous client's rows off the screen on every path (C44).
func (m Model) timelineRows() []lookup.TimelineRow {
	if !m.timeline.keyed {
		return nil
	}
	if _, key, ok := m.timelineInputs(); !ok || key != m.timeline.key {
		return nil
	}
	return m.timeline.rows
}

// timelineStale is the last complete list when it is NOT for the current
// inputs: what the timeline tab keeps on screen, dimmed, while the new list is
// searched. Nil when there is none to keep (a first load, rule 9).
func (m Model) timelineStale() []lookup.TimelineRow {
	if m.timeline.complete {
		return m.timeline.rows
	}
	return m.timeline.kept
}

// timelineStaleFor is timelineStale for a search about to be asked for key: the
// previous list survives only for the same compared charts (a new month or lens
// for the same charts). Another subject's rows are never kept — the previous
// client's transits stay off screen, as C44 requires.
func (m Model) timelineStaleFor(key timelineKey) []lookup.TimelineRow {
	if !m.timeline.keyed || m.timeline.key.natals != key.natals {
		return nil
	}
	return m.timelineStale()
}

// timelineWaiting is detailsTabWaiting's timeline case: a search is owed or in
// flight for the current inputs. A search that failed awaits nothing.
func (m Model) timelineWaiting() bool {
	_, key, ok := m.timelineInputs()
	if !ok {
		return false
	}
	if !m.timeline.keyed || m.timeline.key != key {
		return true
	}
	return !m.timeline.complete && !m.timeline.failed
}

// timelineDisplayRows is what the timeline body DRAWS: the current list, or,
// while the tab waits, the previous one kept for the dimmed look.
func (m Model) timelineDisplayRows() []lookup.TimelineRow {
	if m.timelineWaiting() {
		if _, key, ok := m.timelineInputs(); ok && m.timeline.keyed && m.timeline.key == key {
			return m.timeline.kept
		}
		if _, key, ok := m.timelineInputs(); ok {
			return m.timelineStaleFor(key)
		}
		return nil
	}
	return m.timelineShown()
}

// timelineShown is the rows as the panel SHOWS them and every action reads
// them: the list, cut to the rows the menu.Filter strategy accepts for the `/` query
// against the row as displayed, still in time order (the specification
// rules 2 and 6). With no query it is the list itself.
func (m Model) timelineShown() []lookup.TimelineRow {
	rows := m.timelineRows()
	if m.timeline.query == "" {
		return rows
	}
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = panel.DisplayRow(r, m.cfg)
	}
	kept, _, _ := menu.Filter{Query: m.timeline.query}.Apply(lines, 0)
	out := make([]lookup.TimelineRow, 0, len(kept))
	for _, i := range kept {
		out = append(out, rows[i])
	}
	return out
}

// timelineSearchLine says whether the panel's last row is the search line:
// open for typing (only while Details has the keyboard) or showing a kept query.
func (m Model) timelineSearchLine() bool {
	return m.activeTabID(PanelDetails) == TabDetailsTimeline &&
		((m.timeline.searching && m.details.attached) || m.timeline.query != "")
}

// timelineHeight is how many rows the timeline can show: the panel's inner
// height, less the search line when it is there.
func (m Model) timelineHeight() int {
	h := m.detailsInnerHeight()
	if h > 0 && m.timelineSearchLine() {
		h--
	}
	return h
}

// openTimelineSearch is `/`: the line opens, prefilled with the kept query.
func (m Model) openTimelineSearch() Model {
	m.timeline.searching = true
	return m.rememberSearch(m.timeline.query).clampTimelineCursor()
}

// closeTimelineSearch is Enter in the line: it closes and the query stays.
func (m Model) closeTimelineSearch() Model {
	m.timeline.searching = false
	return m.clampTimelineCursor()
}

// cancelTimelineSearch is Esc in the line: it closes, and the query and the
// row cursor it opened with come back.
func (m Model) cancelTimelineSearch() Model {
	m.timeline.searching = false
	m.timeline.query = m.searchBefore.query
	return m.restoreTimelineCursor().clampTimelineCursor()
}

// timelineSearchBindings is the bar while the search line is open: the line
// types, the arrows move, Enter applies and Esc goes back.
func timelineSearchBindings() []keys.BindingEntry {
	return keys.Titled([]keys.BindingEntry{
		{ID: keys.BindingListUp, Binding: key.NewBinding(key.WithKeys("up")), Title: "Move row", HelpKey: "↑/↓", BarKey: "↑↓", Bar: true},
		{ID: keys.BindingListDown, Folded: true, Binding: key.NewBinding(key.WithKeys("down")), BarKey: "↓"},
		{ID: keys.BindingConfirm, Binding: key.NewBinding(key.WithKeys("enter")), Title: "Jump", BarKey: "enter", Bar: true},
		{ID: keys.BindingClose, Binding: key.NewBinding(key.WithKeys("esc")), Title: "Back", BarKey: "esc", Bar: true},
		// ⇞ ⇟ page the cursor, as the arrows move it; Home, End, g and G are text
		// or nothing in the line. Ctrl+⇞ ⇟ pass through to the Details scroll.
		{ID: keys.BindingPageUp, Title: "Page list", HelpKey: "⇞/⇟", Binding: key.NewBinding(key.WithKeys("pgup")), BarKey: "⇞"},
		{ID: keys.BindingPageDown, Folded: true, Binding: key.NewBinding(key.WithKeys("pgdown")), BarKey: "⇟"},
		{ID: keys.BindingScrollUp, Binding: key.NewBinding(key.WithKeys("ctrl+pgup")), BarKey: "⌃⇞"},
		{ID: keys.BindingScrollDown, Binding: key.NewBinding(key.WithKeys("ctrl+pgdown")), BarKey: "⌃⇟"},
	})
}

// updateTimelineSearchKey is the line's own key state: letters, Space and
// Backspace edit the query (the fuzzy popups' typing helper), the arrows move
// the cursor, Enter jumps to the row under it and Esc goes back. Nothing else
// is read, so j, k, y, d and the comma are text here.
func (m Model) updateTimelineSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keys.MatchBinding(msg, timelineSearchBindings()) {
	case keys.BindingScrollUp:
		return m.scrollDetailsTab(-detailsScrollStep), nil
	case keys.BindingScrollDown:
		return m.scrollDetailsTab(detailsScrollStep), nil
	case keys.BindingPageUp:
		return m.pageTimeline(menu.PageUp), nil
	case keys.BindingPageDown:
		return m.pageTimeline(menu.PageDown), nil
	}
	switch msg.Type {
	case tea.KeyEnter:
		// The arrows move the row cursor in the open line, so Enter is the
		// row's Enter too: the line closes with its query and the row under
		// the cursor is jumped to, as with the line closed (operator,
		// 2026-11-06: the first Enter only closed the line, the second jumped).
		m = m.closeTimelineSearch()
		if m.detailsRowKeysBlocked() {
			return m, nil
		}
		return m.jumpToTimelineRow()
	case tea.KeyEsc:
		return m.cancelTimelineSearch(), nil
	case tea.KeyUp:
		return m.moveTimelineCursor(-1), nil
	case tea.KeyDown:
		return m.moveTimelineCursor(+1), nil
	}
	l := menu.FuzzyList[struct{}]{FilterText: m.timeline.query}
	if l.TypeKey(msg) {
		m.timeline.query = l.FilterText
		m = m.placeTimelineCursor().clampTimelineCursor()
	}
	return m, nil
}

// timelineInputs derives the request and its key from committed state: the
// floating slot's committed Time (the one the Chart panel shows) and place,
// and the anchor record. ok is false before a workspace with a zone exists.
func (m Model) timelineInputs() (lookup.TimelineRequest, timelineKey, bool) {
	if m.slots == nil {
		return lookup.TimelineRequest{}, timelineKey{}, false
	}
	floating := m.slots.Floating()
	loc, err := time.LoadLocation(floating.Location.TZ)
	if err != nil || floating.Location.TZ == "" || floating.Time.IsZero() {
		return lookup.TimelineRequest{}, timelineKey{}, false
	}
	lens := m.committedLens()
	sc := m.deriveScene()
	// The list is asked in the lens of the chart the cursor is on, the
	// floating chart's own when it is there (operator, 2026-10-24: "the chart
	// you are on" decides). A move to a chart in another lens changes the key
	// and asks again; a move between charts in one lens asks nothing.
	if m.ownHouses(sc.cursor).unresolved {
		// The cursor's chart file could not be read: no guessed system, no
		// search, as the main chart refuses to cast (F1).
		return lookup.TimelineRequest{}, timelineKey{}, false
	}
	view := m.viewLens()
	lens.Houses, lens.Zodiac = view.Houses, view.Zodiac
	// The sky search takes the floating slot's marks, each natal its own
	// (F14): the committed lens's objects with the slot's selected objects as
	// Only, the way showLens narrows the Aspects list.
	all := m.slots.All()
	lens.Objects = m.timelineObjects(lens, all[len(all)-1])
	req := lookup.TimelineRequest{When: floating.Time, Place: floating.Location, Lens: lens}
	// The month is Astrolog's, in its own calendar: a Julian 29 February 1500
	// is February whatever Go's calendar calls the day (the specification N8).
	year, month := lookup.TimelineMonth(floating.Time, loc)
	key := timelineKey{year: year, month: month, zone: floating.Location.TZ}
	key.objects, key.aspects, key.trueNode = chooserKey(lens)
	key.skyMarks = fmt.Sprint(lens.Objects.Only.Members())
	key.zodiac = lens.Zodiac
	// Transit and return rows (-t, -tr) are about the selected charts: they
	// appear only while comparing, one pair of searches per selected anchored
	// slot, each row tagged with its slot's numeral. With the cursor on an
	// unselected chart, or fewer than two slots selected, the timeline is the
	// sky alone (-d). Operator hot-fix 2026-09-30, then P4 / C10 / C13 of
	// the specification.
	if sc.mode == sceneComparing {
		var key0 strings.Builder
		for _, slot := range sc.set {
			if slot.Floating {
				continue
			}
			n := sc.numeral(slot)
			chart := anchorOf(slot)
			objects := m.timelineObjects(lens, slot)
			req.Natals = append(req.Natals, lookup.TimelineNatal{Numeral: n, Chart: chart, Objects: objects})
			fmt.Fprintf(&key0, "%d|%d|%v|%v|%s|%s\x00%s|%v\x1e", n, chart.When.UnixNano(), chart.Place.Lon, chart.Place.Lat, chart.Place.TZ, slot.Chart.ID, slot.Chart.Name, objects.Only.Members())
		}
		key.natals = key0.String()
		key.houses = lens.Houses
	}
	return req, key, true
}

// syncTimeline runs the searches when, and only when, their inputs differ
// from the ones the timeline was last asked for: the first time Details
// declares the tab for a set of inputs (C43), and on every change of any
// input (C10) — never for a day, hour or minute move inside the same month
// (C25). It is called after every text commit and when Details comes into
// view; like every Details request, it starts nothing while Details is not
// in the layout.
func (m Model) syncTimeline() (Model, tea.Cmd) {
	// Hidden tabs do no work (the specification rule 3):
	// the search is owed only while the timeline tab is the active one.
	if !m.detailsVisible() || m.focused != PanelChart || m.activeTabID(PanelDetails) != TabDetailsTimeline {
		return m, nil
	}
	_, key, ok := m.timelineInputs()
	if !ok || (m.timeline.keyed && m.timeline.key == key) {
		return m, nil
	}
	prev := m.timeline
	m.timeline = timelineState{key: key, keyed: true, kept: m.timelineStaleFor(key)}
	if m.timeline.kept != nil {
		m.timeline.content, m.timeline.hasContent = prev.content, prev.hasContent
	}
	m.details.cursor, m.details.rowOffset = 0, 0
	// A list already searched for these inputs is drawn now, from memory.
	if nm, ok := m.adoptTimelineFromMemory(key); ok {
		return nm, nil
	}
	return m.bumpIdentity(surfaceTimeline)
}

// timelineKeyFor is the key of the search the Model would ask for now; ok is
// false when there is no request. The same key syncTimeline and
// buildTimelineCmd compare, so the row's inputs and the commit gate cannot
// disagree.
func (m Model) timelineKeyFor() (timelineKey, bool) {
	_, key, ok := m.timelineInputs()
	return key, ok
}

// newTimelineRow is the Timeline surface's refresh policy (specs/
// refresh-descriptor-timeline.md): the coordinator asks it through Model.rowFor.
// Its spawn is the one search seam: a test counts and scripts the searches on
// its own Model without starting Astrolog.
// It has no rest timer: bumpIdentity starts the run at once and arms one
// defensive retry tick of textDebounce. The key is not in accept:
// commitTimeline logs the argv lines first and then drops a result for a
// left key, so such a search still writes its argv line.
func newTimelineRow() surfaceRow {
	return surfaceRow{
		inputs: func(m Model) (refreshKey, bool) {
			k, ok := m.timelineKeyFor()
			return k, ok
		},
		trigger:   triggerIdentity,
		supersede: supersedeHard,
		debounce:  textDebounce,
		build:     buildTimelineCmd,
		accept:    acceptExactWant,
		commit:    (Model).commitTimeline,
		suspend:   forgetIncompleteTimeline,
		spawn:     rowSpawns{timeline: lookup.Timeline},
	}
}

// forgetIncompleteTimeline is the row's suspend: a list that never completed
// forgets its key, so the searches run again when Details returns; a complete
// one stays, for the same inputs.
func forgetIncompleteTimeline(m Model) Model {
	if !m.timeline.complete {
		m.timeline = timelineState{}
	}
	return m
}

// suspendTimeline cancels an in-flight search when Details leaves the
// layout; suspendSurface fences the surface and asks the row to forget.
func (m Model) suspendTimeline() Model {
	return m.suspendSurface(surfaceTimeline)
}

func buildTimelineCmd(m Model, ctx context.Context, runID, wantID uint64) tea.Cmd {
	req, key, ok := m.timelineInputs()
	if !ok || !m.timeline.keyed || key != m.timeline.key {
		return nil
	}
	compute := m.rows[surfaceTimeline].spawn.timeline
	labels := map[int]string{}
	sc := m.deriveScene()
	for i, slot := range sc.set {
		n := i + 1
		labels[n] = slotLabel(n, slot, m.cfg)
	}
	return func() tea.Msg {
		res, err := compute(ctx, req)
		return refreshResultMsg{
			surface: surfaceTimeline,
			runID:   runID,
			wantID:  wantID,
			payload: timelinePayload{key: key, rows: res.Rows, commands: res.Commands, failures: res.Failures, labels: labels},
			err:     err,
		}
	}
}

// commitTimeline adopts a completed search. Its commands become argv lines
// here, at the commit gate, on success and failure alike.
func (m Model) commitTimeline(msg refreshResultMsg) (Model, tea.Cmd) {
	p, _ := msg.payload.(timelinePayload)
	m = m.logCommandsFor(surfaceTimeline, msg.wantID, p.commands, failedLast(msg.err, p.commands)...)
	// A result for inputs the operator has left paints nothing — a late
	// search for the previous client must never replace the current list.
	if p.key != m.timeline.key {
		return m, nil
	}
	if msg.err != nil {
		// A failed search is not a list. The rows already on screen are
		// either the complete list for these same inputs (kept, C14) or
		// nothing (a change of inputs already cleared them, C44). Error
		// lane: the search the operator's navigation asked for failed (C41).
		// The dimmed list was for other inputs: it goes, and nothing is
		// awaited any more (the specification rule 8).
		m.timeline.kept, m.timeline.failed = nil, true
		if !m.timeline.complete {
			m.timeline.hasContent = false // the dimmed list goes; a complete list for these inputs stays
		}
		if len(p.failures) > 0 {
			// Every search of a comparing timeline failed: each is named (C17).
			return m.logTimelineFailures(p), nil
		}
		m = m.noteEphemeris(msg.err)
		return m.logTimelineError(m.timelineReason(msg.err, p.commands)).logWarnings(msg.err), nil
	}
	// Searches that failed beside others that did not: the rows of the others
	// are shown, and each failure is one Log line naming its search (C17, C17a).
	m = m.logTimelineFailures(p)
	return m.landTimeline(p).rememberTimeline(p), nil
}

// landTimeline puts a searched list on screen under its key. A fresh result and
// one adopted from memory land by it alike.
func (m Model) landTimeline(p timelinePayload) Model {
	m.timeline.rows, m.timeline.complete, m.timeline.kept = p.rows, true, nil
	m.timeline.content, m.timeline.hasContent = p.key, true
	return m.placeTimelineCursor().clampTimelineCursor()
}

// timelineReason is why a search failed, in the Log's words: the refusal's own
// sentence when the parser or the default-set read refused, else the usual
// reason pointing at the console when a command was recorded.
func (m Model) timelineReason(err error, commands []string) string {
	if line, ok := lookup.TimelineRefusedLine(err); ok {
		return timelineRefusal(err, line)
	}
	return m.failureReason(err, m.consolePointer(len(commands) > 0))
}

// timelineRefusal is the sentence for a search Sextile refused to go on with
// rather than one whose spawn went wrong.
func timelineRefusal(err error, line string) string {
	if lookup.DefaultSetsRefused(err) {
		// The sets that let the searches honour the body selector could not
		// be read, so none ran: the timeline is refused, not shown unfiltered
		// (C12a).
		return line + " The timeline is not shown rather than shown with bodies you hid."
	}
	return fmt.Sprintf("Astrolog printed a line it could not place: %q", line)
}

// logTimelineError is the one writer of the timeline's error line.
func (m Model) logTimelineError(reason string) Model {
	return m.logError("Couldn't read the timeline. %s", reason)
}

// logTimelineFailures writes one error line per failed search, in spawn order:
// the sky search by name, a transit or return search by its slot (numeral and
// chart name) and Astrolog's own switch for it.
func (m Model) logTimelineFailures(p timelinePayload) Model {
	for _, f := range p.failures {
		m = m.noteEphemeris(f.Err)
		reason := m.timelineReason(f.Err, p.commands)
		if f.Numeral == 0 {
			m = m.logTimelineError(fmt.Sprintf("The sky search (%s) failed. %s", f.Search, reason)).logWarnings(f.Err)
			continue
		}
		label := p.labels[f.Numeral]
		if label == "" {
			label = panel.NumeralForColour(f.Numeral, m.cfg)
		}
		m = m.logTimelineError(fmt.Sprintf("The %s search for %s failed. %s", f.Search, label, reason)).logWarnings(f.Err)
	}
	return m
}

// clampTimelineCursor keeps the row cursor on a row and inside the rendered
// window (C29), with the window itself inside the list. While the timeline is
// shown and Details does not have the keyboard it instead follows the nearest
// event (the specification rule 3): the cursor sits on it and
// the window centres it.
func (m Model) clampTimelineCursor() Model {
	rows := m.timelineShown()
	total := len(rows)
	visible := m.timelineHeight()
	if total == 0 {
		m.details.cursor, m.details.rowOffset = 0, 0
		return m
	}
	if !m.details.attached && m.activeTabID(PanelDetails) == TabDetailsTimeline {
		if n := m.nearestRow(rows); n >= 0 {
			m.details.cursor = n
			if visible > 0 {
				m.details.rowOffset = centreOffset(n, total, visible)
			}
			m.details.cursorAt, m.details.cursorSet = markOf(rows[n]), true
			return m
		}
	}
	m = m.recordTimelineCursor()
	if visible <= 0 {
		return m
	}
	m.details.rowOffset = menu.ScrollToShow(m.details.rowOffset, m.details.cursor, total, visible)
	return m
}

// recordTimelineCursor is the identity half of clampTimelineCursor: the cursor
// is held on a row and the row under it is recorded (cursorAt), so a reload can
// put the cursor back on the same event. It leaves the window alone, which is
// why a page op calls it and not the clamp: paging sets the offset exactly, and
// the clamp's scroll margin would move it.
func (m Model) recordTimelineCursor() Model {
	rows := m.timelineShown()
	if len(rows) == 0 {
		return m
	}
	m.details.cursor = min(max(m.details.cursor, 0), len(rows)-1)
	m.details.cursorAt, m.details.cursorSet = markOf(rows[m.details.cursor]), true
	return m
}

// pageTimeline applies a paging key to the attached timeline's cursor: the
// shared rule (menu.PageCursor) over the rows shown and the rows the panel can show
// (the search line takes one when it is there). The offset it returns is the
// one stored, exactly.
func (m Model) pageTimeline(op menu.PageOp) Model {
	total, visible := len(m.timelineShown()), m.timelineHeight()
	if total == 0 || visible <= 0 {
		return m
	}
	m.details.cursor, m.details.rowOffset = menu.PageCursor(m.details.cursor, m.details.rowOffset, total, visible, op)
	return m.recordTimelineCursor()
}

// floatingTime is the floating slot's committed Time, or the zero time.
func (m Model) floatingTime() time.Time {
	if m.slots == nil {
		return time.Time{}
	}
	return m.slots.Floating().Time
}

// nearestRow is the bold row of rows: the row Enter just jumped to while the
// floating moment still equals its instant (rule 9), else the first row at or
// after the FLOATING slot's moment, whichever chart the cursor is on
// (timeline-tab-rules T1 rule 4). -1 when there is none.
func (m Model) nearestRow(rows []lookup.TimelineRow) int {
	// The cursor is no longer necessarily on the floating slot: Enter on a chart
	// row leaves it on that chart (the specification C2), and
	// every cursor move clears pressedSet, so the floating check is the moment's.
	if p := m.details.pressedAt; m.details.pressedSet && m.floatingTime().Equal(p.landed()) {
		for i, r := range rows {
			if markOf(r) == p {
				return i
			}
		}
	}
	return nearestTimelineRow(rows, m.floatingTime())
}

// nearestTimelineRow is the index of the nearest event: the first row whose
// instant is at or after t. It is -1 when every row is earlier (the month is
// past) or there are no rows. It is the one definition of "nearest": whatever
// list the timeline shows is handed to it.
func nearestTimelineRow(rows []lookup.TimelineRow, t time.Time) int {
	// A moment Enter put on a row's last second (landedAt) is that row's, in
	// any zone: compared exactly, never by truncating to a UTC minute, which a
	// row listed in local mean time (e.g. +00:19:32) does not sit on.
	for i, r := range rows {
		if landedFor(r).Equal(t) {
			return i
		}
	}
	for i, r := range rows {
		if !r.Instant.Before(t) {
			return i
		}
	}
	return -1
}

// centreOffset is the window offset that puts line in the middle row, as far
// as the list allows.
func centreOffset(line, total, visible int) int {
	return pane.ClampOffset(line-visible/2, total, visible)
}

// placeTimelineCursor puts the cursor where a freshly loaded list wants it
// (rules 4 and 5): while Details has the keyboard, on the row the cursor was
// on if that event is still listed (same instant and line), else on the
// nearest event, centred. Without the keyboard the clamp that follows
// centres the nearest event itself.
func (m Model) placeTimelineCursor() Model {
	rows := m.timelineShown()
	if !m.details.attached || len(rows) == 0 {
		return m
	}
	visible := m.timelineHeight()
	if m.details.cursorSet {
		for i, r := range rows {
			if markOf(r) == m.details.cursorAt {
				m.details.cursor = i
				if visible > 0 {
					m.details.rowOffset = menu.ScrollToShow(m.details.rowOffset, i, len(rows), visible)
				}
				return m
			}
		}
	}
	if n := m.nearestRow(rows); n >= 0 {
		m.details.cursor = n
		if visible > 0 {
			m.details.rowOffset = centreOffset(n, len(rows), visible)
		}
	}
	return m
}

// moveTimelineCursor moves the row cursor by step, stopping at the first and
// last rows (C30). It changes nothing else: no slot, no time, no compute.
func (m Model) moveTimelineCursor(step int) Model {
	m.details.cursor += step
	return m.clampTimelineCursor()
}

// leadBodiesTo puts the Bodies cursor on the body row leads to (lookup.LeadObject:
// the moving object, the transiting object, the faster object of a sky aspect). A
// row that is not resolved, or a body Bodies does not list, moves nothing. A led
// row outside the window it was in lands in the middle of the panel, not at its
// edge (operator, 2026-11-06); a row in sight leaves the window where it is.
func (m Model) leadBodiesTo(row lookup.TimelineRow) Model {
	if m.slots == nil {
		return m
	}
	o, ok := lookup.LeadObject(row)
	if !ok {
		return m
	}
	planets, under, height := m.bodiesRows()
	first, last := panel.BodyDrawnRows(planets, under, height, m.bodyTop)
	m = m.walkBodyCursorTo(o)
	if r := m.bodyCursorIdx; r < first || r > last {
		m.bodyTop = panel.BodyWindowStart(planets, under, r, panel.BodyDisplayRow(planets, under, height, r)-height/2, height)
	}
	return m
}

// scrollTimeline moves the timeline window by delta rows and pulls the
// cursor into it, so the row Enter would act on is always a row the
// operator can see (C29).
func (m Model) scrollTimeline(delta int) Model {
	total, visible := len(m.timelineShown()), m.timelineHeight()
	if total == 0 || visible <= 0 {
		return m
	}
	m.details.rowOffset = pane.ClampOffset(m.details.rowOffset+delta, total, visible)
	last := min(m.details.rowOffset+visible, total) - 1
	m.details.cursor = min(max(m.details.cursor, m.details.rowOffset), last)
	return m
}

// scrollDetailsTab scrolls whichever Details tab is active — the report by
// lines, the timeline by rows. The page keys and the mouse wheel both come
// here, attached or not (C19).
func (m Model) scrollDetailsTab(delta int) Model {
	if m.activeTabID(PanelDetails) == TabDetailsTimeline {
		return m.scrollTimeline(delta)
	}
	return m.scrollDetails(delta)
}

// attachTab is the Details tab Enter will show: from the Chart the default tab
// becomes the timeline and any other choice stays; from Bodies and Aspects the
// tab already chosen stays.
func (m Model) attachTab() TabID {
	active := m.activeTabID(PanelDetails)
	if m.focused == PanelChart {
		if tabs := m.panelTabs(PanelDetails); len(tabs) > 1 && active == tabs[0].ID {
			return TabDetailsTimeline
		}
	}
	return active
}

// attachTitle is Enter's title: the tab attachDetails shows.
func (m Model) attachTitle() string {
	if m.attachTab() == TabDetailsTimeline {
		return "Timeline"
	}
	return "Report"
}

// canAttachDetails is whether Enter has a Details to open from here.
func (m Model) canAttachDetails() bool {
	return isFocusable(m.focused) && m.detailsVisible()
}

// attachDetails is Enter from a ring panel (C15): Details takes row
// navigation, focus does not move, and nothing computes.
func (m Model) attachDetails() (Model, tea.Cmd) {
	if m.blocked(keys.BindingAttach) != "" {
		return m, nil
	}
	m.details.attached = true
	// Enter from the Chart: the default tab becomes the timeline, any other
	// choice stays; either way the tab Enter found is what leaving restores.
	m.details.preEnterSet = false
	if m.focused == PanelChart {
		m.details.preEnter, m.details.preEnterSet = m.tabChoice(PanelDetails), true
		if tab := m.attachTab(); tab != m.activeTabID(PanelDetails) {
			m = m.withDetailsTab(PanelChart, tab)
		}
	}
	if m.activeTabID(PanelDetails) == TabDetailsTimeline {
		m.details.cursorSet = false
		m = m.placeTimelineCursor()
	}
	return m.clampTimelineCursor(), nil
}

// restorePreEnterTab puts the Chart's Details tab back to the one Enter found,
// whatever was chosen while attached (acceptance 4 and 5).
func (m Model) restorePreEnterTab() Model {
	if m.details.preEnterSet {
		m = m.withDetailsTab(PanelChart, m.details.preEnter)
		m.details.preEnterSet = false
	}
	return m
}

// detachDetails is Esc while attached (C16). The focused panel never moved,
// so it is the subject's panel already.
func (m Model) detachDetails() (Model, tea.Cmd) {
	m.details.attached = false
	m = m.restorePreEnterTab()
	m.timeline.searching, m.details.searching = false, false
	m = m.clampTimelineCursor()
	// Rule 6 of the specification.
	return m.catchUpDetails()
}

// detachAndFocus is rule 7: 1, 2 or 3 from attached Details leaves it and
// focuses that panel, as the same digit does from the ring. A different panel
// is a new subject, which the focus change itself requests for; the same panel
// gets the catch-up.
func (m Model) detachAndFocus(p PanelID) (Model, tea.Cmd) {
	m.details.attached = false
	m = m.restorePreEnterTab()
	m.timeline.searching, m.details.searching = false, false
	m = m.clampTimelineCursor()
	if p == m.focused {
		return m.catchUpDetails()
	}
	m.details.stale = false
	return m.setFocusedPanel(p)
}

// updateDetailsKey owns the keyboard while Details is attached (D18), the
// way updateConsoleKey does for the console: Details resolves each key from
// its own table, and everything else — `q` included — is swallowed, so neither
// the ring nor the slot can move behind an attached panel, and in particular
// a key Details does not name never reaches the ring or the slot (C48).
func (m Model) updateDetailsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.timeline.searching && m.timelineSearchLine() {
		return m.updateTimelineSearchKey(msg)
	}
	if m.details.searching && m.detailsReportSearchLine() {
		return m.updateDetailsReportSearchKey(msg)
	}
	id := keys.MatchBinding(msg, m.available(ownerDetails, detailsBindings(m.keys)))
	// A waiting tab with a row cursor ignores its row keys
	// (the specification rule 7); everything else acts.
	if m.detailsRowKeysBlocked() {
		switch id {
		case keys.BindingToggle, keys.BindingSearch, keys.BindingListUp, keys.BindingListDown, keys.BindingConfirm, keys.BindingScrollDown, keys.BindingScrollUp, keys.BindingPageDown, keys.BindingPageUp, keys.BindingPageHome, keys.BindingPageEnd:
			return m, nil
		}
	}
	// `/` on a waiting report does nothing (the specification 8).
	if id == keys.BindingSearch && m.detailsReportFilterBlocked() {
		return m, nil
	}
	switch id {
	case keys.BindingToggle:
		return m.toggleTimelineMark(), nil
	case keys.BindingSearch:
		if !m.detailsOnTimeline() {
			return m.openDetailsReportSearch(), nil
		}
		return m.openTimelineSearch(), nil
	case keys.BindingClose:
		return m.detachDetails()
	case keys.BindingListUp:
		return m.stepDetailsRow(-1), nil
	case keys.BindingListDown:
		return m.stepDetailsRow(+1), nil
	case keys.BindingConfirm:
		return m.jumpToTimelineRow()
	case keys.BindingScrollDown:
		return m.scrollDetailsTab(detailsScrollStep), nil
	case keys.BindingScrollUp:
		return m.scrollDetailsTab(-detailsScrollStep), nil
	// ⇞ ⇟ Home End move the timeline's row cursor by the one paging rule; a tab
	// with no row cursor (the report) scrolls two lines on ⇞ ⇟ as it always did.
	case keys.BindingPageDown:
		if m.detailsOnTimeline() {
			return m.pageTimeline(menu.PageDown), nil
		}
		return m.scrollDetailsTab(detailsScrollStep), nil
	case keys.BindingPageUp:
		if m.detailsOnTimeline() {
			return m.pageTimeline(menu.PageUp), nil
		}
		return m.scrollDetailsTab(-detailsScrollStep), nil
	case keys.BindingPageHome:
		return m.pageTimeline(menu.PageHome), nil
	case keys.BindingPageEnd:
		return m.pageTimeline(menu.PageEnd), nil
	// The timeline's own navigation: the list is cut to the floating slot's
	// month, and these are the keys that move it. `o`/`O` step the month
	// (C57, operator 2026-09-30, reversing pass-7 P2); `n` jumps to now and
	// `N` toggles back (C58, operator 2026-10-01). Each takes its ordinary
	// path, so the commit that re-keys the searches (C10, C25) is untouched.
	// The table offers them on both tabs and from any cursor position (D37,
	// operator 2026-10-13, reversing C59 and the floating-only rule).
	case keys.BindingTimeStepYear:
		return m.advanceTime(unitYear, stepDirectionFromRune(msg))
	case keys.BindingTimeStepMonth:
		return m.advanceTime(unitMonth, stepDirectionFromRune(msg))
	case keys.BindingTimeStepWeek:
		return m.advanceTime(unitWeek, stepDirectionFromRune(msg))
	case keys.BindingTimeStepDay:
		return m.advanceTime(unitDay, stepDirectionFromRune(msg))
	case keys.BindingTimeStepHour:
		return m.advanceTime(unitHour, stepDirectionFromRune(msg))
	case keys.BindingTimeStepMinute:
		return m.advanceTime(unitMinute, stepDirectionFromRune(msg))
	case keys.BindingJumpNow:
		return m.jumpFloatingToNow()
	case keys.BindingToggleBack:
		return m.toggleFloatingBack()
	case keys.BindingFocusChart, keys.BindingFocusBodies, keys.BindingFocusAspects:
		return m.detachAndFocus(panelDigits[id])
	case keys.BindingTabNext, keys.BindingDetailsTab:
		return m.cycleTab(PanelDetails, +1)
	case keys.BindingTabPrev:
		return m.cycleTab(PanelDetails, -1)
	}
	return m.passDetailsKeyToMain(msg)
}

// detailsRefused are the main screen's keys that do nothing while Details has
// focus: each would move, select, reorder or remove something in a main panel
// underneath it (operator, 2026-10-24). Every other main key works from Details
// exactly as on the main screen.
var detailsRefused = map[keys.BindingID]string{
	keys.BindingNextView:      "moves the focus to another main panel",
	keys.BindingPrevView:      "moves the focus to another main panel",
	keys.BindingSelect:        "selects a chart or a body in the focused panel",
	keys.BindingSlotCycleUp:   "moves the Chart or Bodies cursor",
	keys.BindingSlotCycleDown: "moves the Chart or Bodies cursor",
	keys.BindingPageHome:      "moves the Chart or Bodies cursor",
	keys.BindingPageEnd:       "moves the Chart or Bodies cursor",
	keys.BindingSlotMoveUp:    "reorders the charts",
	keys.BindingSlotMoveDown:  "reorders the charts",
	keys.BindingSlotDelete:    "removes a chart",
	keys.BindingAttach:        "Details is already attached. Attaching again resets what leaving restores",
	keys.BindingActions:       "opens the Actions list, whose commands act on the main screen underneath Details",
}

// passDetailsKeyToMain is a key Details does not answer itself: the main
// screen's own dispatch, unless the key is refused. A popup or the console
// opened from here owns the keys ahead of attached Details (currentOwner), and
// Details stays attached underneath, so closing it lands back here.
func (m Model) passDetailsKeyToMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	id := keys.MatchBinding(msg, m.available(ownerMain, mainBindings(m.keys)))
	if _, refused := detailsRefused[id]; refused {
		if id == keys.BindingActions {
			// The one refusal that says so (operator, 2026-11-07): the others stay
			// silent as the 2026-10-24 list has them.
			return m.showNotice(noticeActionsInDetails)
		}
		return m, nil
	}
	return m.dispatchMainBinding(id, msg)
}

// stepDetailsRow is ↑/↓/j/k while attached: the row cursor on the timeline
// tab (C17), one report line on the report tab (C26). Neither moves a slot.
func (m Model) stepDetailsRow(step int) Model {
	if m.activeTabID(PanelDetails) == TabDetailsTimeline {
		return m.moveTimelineCursor(step)
	}
	return m.scrollDetails(step)
}

// landedAt is where Enter puts the moment for a row listed at instant: the last
// second of the listed minute. Astrolog lists an event at its minute and the
// event happens inside it, so the screen shows the state just after it
// (the specification). Every "is the moment still the
// entered row's" check compares against it, never against the bare instant.
func landedAt(instant time.Time) time.Time { return instant.Add(59 * time.Second) }

// landsEarly says Enter puts the moment on the FIRST second of the row's
// minute: an S/R station, where the body is still direct (C14 of
// the specification; the node rows are relabelled by lookup first,
// so the word is true for them too). Every other row lands on the last.
func landsEarly(r lookup.TimelineRow) bool {
	return r.Kind == lookup.TimelineKindStation && r.Station == "S/R"
}

// landedFor is where Enter puts the moment for row: landedAt, except an S/R
// station, which lands on its listed instant. Every "is the moment still the
// entered row's" check goes through it (or the pressed mark's landed).
func landedFor(r lookup.TimelineRow) time.Time {
	if landsEarly(r) {
		return r.Instant
	}
	return landedAt(r.Instant)
}

// jumpToTimelineRow is Enter on a timeline row (C20): the row's instant
// becomes the floating slot's target, through exactly the path a time-step
// key takes, and the slot's Time follows when that compute commits. The
// Chart-panel cursor goes to the row's chart (a sky row: the floating slot), so
// the screen shows the row where it was found (the specification
// C2, C3). Details stays attached, on the timeline, with the row cursor where it
// was. On the report tab, or with no row under the cursor, nothing moves.
func (m Model) jumpToTimelineRow() (Model, tea.Cmd) {
	if m.activeTabID(PanelDetails) != TabDetailsTimeline {
		return m, nil
	}
	c := m.details.cursor
	if c < 0 || c >= len(m.timelineShown()) || m.slots == nil {
		return m, nil
	}
	row := m.timelineShown()[c]
	instant := landedFor(row)
	before := m.comparisonSig()
	origin := m.cursorIdx
	// The cursor target, resolved before any selection changes: the floating
	// slot, whose bodies a timeline row is about, with a chart row's chart (in
	// the selected set while its rows are listed) as the base it is read
	// against; a moment is never a base, so "now in k" lives on now's row
	// (operator 2026-10-24). With the floating slot unselected there is no such
	// tab, and the cursor walks to the row's chart as it always did.
	all := m.slots.All()
	floating := len(all) - 1
	target := floating
	floatSelected := m.deriveScene().numeral(all[floating]) != 0
	var rowChart *store.Slot
	if set := m.deriveScene().set; row.Numeral > 0 && row.Numeral <= len(set) {
		rowChart = set[row.Numeral-1]
		for i, s := range all {
			if s == rowChart && !floatSelected {
				target = i
			}
		}
	}
	// Enter never changes the selection (operator 2026-10-23): an unselected
	// floating slot stays unselected and only its moment moves.
	// The row the cursor stood on when Enter was pressed: the walks below pass
	// rows on their way, and only the destination is an arrival (S4a A8).
	pressed := m
	// The cursor walks to the row's chart (a sky row: the floating slot) by the
	// Chart panel's own Up/Down, so the screen shows the row's own line, selected
	// and compared against now (operator 2026-10-13: "if (1) it hit, it selects
	// it + selecting the tab that compares it against now/float ... if a
	// non-transit row is selected, select the now/floating slot"). Every step
	// is applied for its state only, with Details attached and the focus left
	// where it is (E1, operator 2026-10-24); the one recompute below is Enter's,
	// and its frame is the one that lands, the main panels holding the old
	// scene until it does (the specification).
	m = m.walkCursorTo(target)
	m = m.withTabMemoryOf(pressed)
	m = m.noteArrival()
	moved := m.cursorIdx != origin
	// Every row that names a body moves now's Bodies cursor to it by Bodies'
	// own Up/Down action, so `2` lands on it (the specification and
	// specs/timeline-row-body.md).
	m = m.leadBodiesTo(row)
	// Each press clamps the row cursor against the list its transient state
	// shows (a cursor on the unselected floating slot lists the sky rows
	// alone); the row is Enter's own and stays where it was.
	m.details.cursor = c
	m.targetFloatingTime = instant
	m.floatingDetached = true
	// The pressed row is set after the presses, which each end it (rule 8).
	m.details.pressedAt, m.details.pressedSet = markOf(row), true
	m.details.pressedChart = rowChart
	// Enter stores what it shows (S4a A19): with the floating slot selected, a
	// chart row's tab is "now in that chart" (the base is the chart) and a sky
	// row shows now alone (no base). With the floating slot unselected (six
	// charts already selected) there is no pair to store and the base stays.
	if floatSelected {
		m = m.withBase(rowChart)
	}
	var cmd tea.Cmd
	if moved || m.comparisonSig() != before {
		m, cmd = m.afterFocusChange()
	} else {
		m, cmd = m.setFloatingTarget(instant)
	}
	return m.clampTimelineCursor(), cmd
}

// copiedRows is what `,y` copies on the timeline tab: the marked rows in time
// order, or every row shown when none is marked, each as the panel displays it
// (aligned, unclipped). The list is already in time order.
func (m Model) copiedRows() []string {
	var out []string
	shown := m.timelineShown()
	pad := panel.AnyJulian(shown)
	for _, r := range shown {
		if len(m.timeline.marks) == 0 || m.timeline.marks[markOf(r)] {
			out = append(out, panel.DisplayRowPad(r, m.cfg, pad))
		}
	}
	return out
}

// yankDetails is `,y`: the timeline tab's marked (else all) rows, or the
// report tab's whole report, through the same clipboard seam and confirmation
// the console's copy uses, and one Log note saying what went.
func (m Model) yankDetails() (Model, tea.Cmd) {
	var text, scope, what string
	if m.activeTabID(PanelDetails) == TabDetailsTimeline {
		rows := m.copiedRows()
		noun := "rows"
		if len(rows) == 1 {
			noun = "row"
		}
		text, scope, what = strings.Join(rows, "\n"), "timeline", fmt.Sprintf("copied %d %s", len(rows), noun)
	} else {
		text, scope, what = m.detailsBodyShown(), "details", "copied the report"
	}
	if text == "" {
		return m.showNotice(noticeNothingCopy)
	}
	if err := yankLine(text); err != nil {
		return m.logError("Couldn't copy the Details panel to the clipboard.").showNotice(noticeCopyFailed)
	}
	m = m.logNote(scope, "%s", what)
	m.yank.gen++
	m.yank.active = true
	m.yank.notice = ""
	m.yank.chars = utf8.RuneCountInString(text)
	return m, armYankFlash(m.yank.gen)
}
