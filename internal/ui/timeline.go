package ui

import (
	"context"
	"fmt"
	"github.com/charmbracelet/bubbles/key"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"slices"
	"strings"
	"sync"
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
	// list is the list the key asks for: the Timeline's and the Moon list's keys
	// hold the same inputs and differ only here (M6a).
	list  timelineList
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
	nodes            recipe.NodeForm
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
func chooserKey(l lookup.Lens) (objects, aspects string, nodes recipe.NodeForm) {
	o := l.Objects
	objects = fmt.Sprintf("%d|%v|%v|%v", int(o.Baseline), o.Add.Members(), o.Remove.Members(), o.Selected())
	aspects = fmt.Sprintf("%t|%v", l.Aspects.Explicit, l.Aspects.Selected())
	return objects, aspects, o.Nodes
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
	if !isListTab(m.activeTabID(PanelDetails)) || c < 0 || c >= len(rows) {
		return m
	}
	l := m.list(m.shownList())
	mk := markOf(rows[c])
	next := make(map[timelineMark]bool, len(l.marks)+1)
	for k := range l.marks {
		next[k] = true
	}
	if next[mk] {
		delete(next, mk)
	} else {
		next[mk] = true
	}
	l.marks = next
	return m
}

// timelinePayload is a list's result, stamped with the key it was
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
	// voids are the void searches this search spawned, with the keys they
	// were asked under; commitTimeline remembers them on success only.
	voids []voidLanding
	// leftOut is what the searches left out for want of ephemeris data; the
	// console says so, no screen does.
	leftOut lookup.LeftOut
}

// voidMemory is the void searches that landed, by what they were asked. It is
// declared here, beside the timeline that asks, so memory.go keeps naming no
// package but slices and bubbletea.
type voidMemory = lru[lookup.VoidKey, lookup.VoidResult]

// rememberVoids keeps the void searches a landed timeline search ran.
func (m Model) rememberVoids(p timelinePayload) Model {
	for _, v := range p.voids {
		m.mem.voids = m.mem.voids.put(v.key, v.res)
	}
	return m
}

// voidLanding is one void search's answer under its key.
type voidLanding struct {
	key lookup.VoidKey
	res lookup.VoidResult
}

func (timelinePayload) isRefreshPayload() {}

// timelineRows is the list as it may be SHOWN or acted on: the rows, but
// only while they belong to the search inputs as they stand right now. The
// inputs change the moment a new client is anchored or a city is picked —
// before any compute commits and before syncTimeline runs — so gating on the
// live key rather than on a clear somewhere downstream is what keeps the
// previous client's rows off the screen on every path (C44).
func (m Model) timelineRows() []lookup.TimelineRow { return m.timelineRowsFor(m.shownList()) }

// timelineRowsFor is timelineRows for list k.
func (m Model) timelineRowsFor(k timelineList) []lookup.TimelineRow {
	l := m.list(k)
	if !l.keyed {
		return nil
	}
	if _, key, ok := m.timelineInputsFor(k); !ok || key != l.key {
		return nil
	}
	return l.rows
}

// timelineStale is the last complete list when it is NOT for the current
// inputs: what the timeline tab keeps on screen, dimmed, while the new list is
// searched. Nil when there is none to keep (a first load, rule 9).
func (m Model) timelineStale(k timelineList) []lookup.TimelineRow {
	l := m.list(k)
	if l.complete {
		return l.rows
	}
	return l.kept
}

// timelineStaleFor is timelineStale for a search about to be asked for key: the
// previous list survives only for the same compared charts (a new month or lens
// for the same charts). Another subject's rows are never kept — the previous
// client's transits stay off screen, as C44 requires.
func (m Model) timelineStaleFor(key timelineKey) []lookup.TimelineRow {
	l := m.list(key.list)
	if !l.keyed || l.key.natals != key.natals {
		return nil
	}
	return m.timelineStale(key.list)
}

// timelineWaiting is detailsTabWaiting's timeline case: a search is owed or in
// flight for the current inputs. A search that failed awaits nothing.
func (m Model) timelineWaiting() bool { return m.timelineWaitingFor(m.shownList()) }

// timelineWaitingFor is timelineWaiting for list k.
func (m Model) timelineWaitingFor(k timelineList) bool {
	l := m.list(k)
	_, key, ok := m.timelineInputsFor(k)
	if !ok {
		return false
	}
	if !l.keyed || l.key != key {
		return true
	}
	return !l.complete && !l.failed
}

// timelineDisplayRows is what the timeline body DRAWS: the current list, or,
// while the tab waits, the previous one kept for the dimmed look.
func (m Model) timelineDisplayRows() []lookup.TimelineRow {
	k := m.shownList()
	l := m.list(k)
	if m.timelineWaitingFor(k) {
		if _, key, ok := m.timelineInputsFor(k); ok && l.keyed && l.key == key {
			return l.kept
		}
		if _, key, ok := m.timelineInputsFor(k); ok {
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
	query := m.list(m.shownList()).query
	if query == "" {
		return rows
	}
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = panel.DisplayRow(r, m.cfg)
	}
	kept, _, _ := menu.Filter{Query: query}.Apply(lines, 0)
	out := make([]lookup.TimelineRow, 0, len(kept))
	for _, i := range kept {
		out = append(out, rows[i])
	}
	return out
}

// timelineSearchLine says whether the panel's last row is the search line:
// open for typing (only while Details has the keyboard) or showing a kept query.
func (m Model) timelineSearchLine() bool {
	l := m.list(m.shownList())
	return isListTab(m.activeTabID(PanelDetails)) &&
		((l.searching && m.details.attached) || l.query != "")
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
	l := m.list(m.shownList())
	l.searching = true
	return m.rememberSearch(l.query).clampTimelineCursor()
}

// closeTimelineSearch is Enter in the line: it closes and the query stays. The
// row cursor comes back on the row it was on before `/` when that row is still
// shown, else on the next event after the floating chart's moment, else on the
// last row shown.
func (m Model) closeTimelineSearch() Model {
	m.list(m.shownList()).searching = false
	rows := m.timelineShown()
	if len(rows) == 0 {
		return m.clampTimelineCursor()
	}
	at := -1
	if b := m.searchBefore; b.cursorSet {
		for i, r := range rows {
			if markOf(r) == b.cursorAt {
				at = i
				break
			}
		}
	}
	if at < 0 {
		if at = m.nearestRow(rows); at < 0 {
			at = len(rows) - 1
		}
	}
	m.details.followNow = false
	m.details.cursor = at
	if visible := m.timelineHeight(); visible > 0 {
		m.details.rowOffset = menu.ScrollToShow(m.details.rowOffset, at, len(rows), visible)
	}
	return m.recordTimelineCursor()
}

// cancelTimelineSearch is Esc in the line: it closes, and the query and the
// row cursor it opened with come back.
func (m Model) cancelTimelineSearch() Model {
	l := m.list(m.shownList())
	l.searching = false
	l.query = m.searchBefore.query
	return m.restoreTimelineCursor().clampTimelineCursor()
}

// timelineSearchBindings is the bar while the search line is open: the line
// types, Enter closes it with its query and Esc goes back. It takes no arrow
// and no paging key.
func timelineSearchBindings() []keys.BindingEntry {
	return keys.TitledLocal([]keys.BindingEntry{
		{ID: keys.BindingConfirm, Binding: key.NewBinding(key.WithKeys("enter")), Title: "Apply", BarKey: "enter", Bar: true},
		{ID: keys.BindingClose, Binding: key.NewBinding(key.WithKeys("esc")), Title: "Back", BarKey: "esc", Bar: true},
		// Home, End, g and G are text or nothing in the line.
		{ID: keys.BindingCopy, Binding: key.NewBinding(), Title: "Copy", BarKeyFn: chordKey("y"), BarKey: ", y", Chord: true, HelpWhen: func(keys.Context) bool { return false }},
	})
}

// updateTimelineSearchKey is the line's own key state (filterLineKey): letters,
// Space and Backspace edit the query, Enter closes the line with its query,
// Backspace on an empty query closes it too, and Esc goes back. The arrows and
// the paging keys do nothing while it is open, and the list draws no cursor
// row. Nothing else is read, so j, k, y, d and the comma are text here.
func (m Model) updateTimelineSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	shown := m.list(m.shownList())
	query, act := filterLineKey(msg, shown.query)
	switch act {
	case filterClose:
		return m.closeTimelineSearch(), nil
	case filterCancel:
		return m.cancelTimelineSearch(), nil
	case filterEdited:
		shown.query = query
		m = m.placeTimelineCursor().clampTimelineCursor()
	}
	return m, nil
}

// timelineInputs derives the request and its key from committed state: the
// floating slot's committed Time (the one the Chart panel shows) and place,
// and the anchor record. ok is false before a workspace with a zone exists.
func (m Model) timelineInputs() (lookup.TimelineRequest, timelineKey, bool) {
	return m.timelineInputsFor(m.shownList())
}

// timelineInputsFor is timelineInputs for list k. The two lists ask the same
// inputs; the Moon list also limits the search to the Moon and its key names it.
func (m Model) timelineInputsFor(k timelineList) (lookup.TimelineRequest, timelineKey, bool) {
	if m.slots == nil {
		return lookup.TimelineRequest{}, timelineKey{}, false
	}
	floating := m.slots.Floating()
	loc, err := lookup.PlaceZone(floating.Time, floating.Location)
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
	lens.Houses, lens.Zodiac, lens.Objects.Nodes = view.Houses, view.Zodiac, view.Objects.Nodes
	// The sky search takes the floating slot's marks, each natal its own
	// (F14): the committed lens's objects with the slot's selected objects as
	// Only, the way showLens narrows the Aspects list.
	all := m.slots.All()
	lens.Objects = m.timelineObjects(lens, all[len(all)-1])
	req := lookup.TimelineRequest{When: floating.Time, Place: floating.Location, Lens: lens, Moon: k.moon()}
	// The month is Astrolog's, in its own calendar: a Julian 29 February 1500
	// is February whatever Go's calendar calls the day (the specification N8).
	year, month := lookup.TimelineMonth(floating.Time, loc)
	key := timelineKey{list: k, year: year, month: month, zone: floating.Location.TZ}
	key.objects, key.aspects, key.nodes = chooserKey(lens)
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
	return m.syncTimelineFor(m.shownList())
}

// syncTimelineFor is syncTimeline for list k: it searches only while k's tab is
// the active one (M7).
func (m Model) syncTimelineFor(k timelineList) (Model, tea.Cmd) {
	// Hidden tabs do no work (the specification rule 3):
	// the search is owed only while the list's tab is the active one.
	if !m.detailsVisible() || m.focused != PanelChart || m.activeTabID(PanelDetails) != k.tab() {
		return m, nil
	}
	_, key, ok := m.timelineInputsFor(k)
	l := m.list(k)
	if !ok || (l.keyed && l.key == key) {
		return m, nil
	}
	prev := *l
	*l = timelineState{key: key, keyed: true, kept: m.timelineStaleFor(key)}
	if l.kept != nil {
		l.content, l.hasContent = prev.content, prev.hasContent
	}
	m.details.cursor, m.details.rowOffset = 0, 0
	// A list already searched for these inputs is drawn now, from memory.
	if nm, ok := m.adoptTimelineFromMemory(key); ok {
		return nm, nil
	}
	return m.bumpIdentity(k.surface())
}

// timelineKeyFor is the key of the search the Model would ask for now; ok is
// false when there is no request. The same key syncTimeline and
// buildTimelineCmd compare, so the row's inputs and the commit gate cannot
// disagree.
func (m Model) timelineKeyFor() (timelineKey, bool) { return m.timelineKeyForList(m.shownList()) }

// timelineKeyForList is timelineKeyFor for list k.
func (m Model) timelineKeyForList(k timelineList) (timelineKey, bool) {
	_, key, ok := m.timelineInputsFor(k)
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
func newTimelineRow(k timelineList) surfaceRow {
	return surfaceRow{
		inputs: func(m Model) (refreshKey, bool) {
			key, ok := m.timelineKeyForList(k)
			return key, ok
		},
		trigger:   triggerIdentity,
		supersede: supersedeHard,
		debounce:  textDebounce,
		build: func(m Model, ctx context.Context, runID, wantID uint64) tea.Cmd {
			return buildTimelineCmdFor(m, k, ctx, runID, wantID)
		},
		accept:  acceptExactWant,
		commit:  func(m Model, msg refreshResultMsg) (Model, tea.Cmd) { return m.commitTimelineFor(k, msg) },
		suspend: func(m Model) Model { return forgetIncompleteTimeline(m, k) },
		spawn:   rowSpawns{timeline: lookup.Timeline, voids: lookup.RunVoidSearch, voidsAhead: lookup.RunVoidSearch},
	}
}

// forgetIncompleteTimeline is the row's suspend: a list that never completed
// forgets its key, so the searches run again when Details returns; a complete
// one stays, for the same inputs.
func forgetIncompleteTimeline(m Model, k timelineList) Model {
	if l := m.list(k); !l.complete {
		*l = timelineState{}
	}
	return m
}

// suspendTimeline cancels an in-flight search when Details leaves the
// layout; suspendSurface fences the surface and asks the row to forget.
func (m Model) suspendTimeline() Model {
	for _, k := range allLists {
		m = m.suspendSurface(k.surface())
	}
	return m
}

func buildTimelineCmd(m Model, ctx context.Context, runID, wantID uint64) tea.Cmd {
	return buildTimelineCmdFor(m, m.shownList(), ctx, runID, wantID)
}

// buildTimelineCmdFor is buildTimelineCmd for list k.
func buildTimelineCmdFor(m Model, k timelineList, ctx context.Context, runID, wantID uint64) tea.Cmd {
	req, key, ok := m.timelineInputsFor(k)
	if l := m.list(k); !ok || !l.keyed || key != l.key {
		return nil
	}
	compute := m.rows[k.surface()].spawn.timeline
	labels := map[int]string{}
	sc := m.deriveScene()
	for i, slot := range sc.set {
		n := i + 1
		labels[n] = slotLabel(n, slot, m.cfg)
	}
	// The remembered void searches are copied here, on the Update goroutine; the
	// cmd reads the copy and appends what it spawns under its own lock.
	remembered := slices.Clone(m.mem.voids.items)
	voidSpawn := m.rows[k.surface()].spawn.voids
	var mu sync.Mutex
	var spawned []voidLanding
	req.Voids = func(ctx context.Context, vr lookup.VoidRequest) (lookup.VoidResult, bool, error) {
		vk := lookup.VoidKeyFor(vr)
		for _, it := range remembered {
			if it.key == vk {
				return it.val, false, nil
			}
		}
		res, err := voidSpawn(ctx, vr)
		if err != nil {
			return res, true, err
		}
		mu.Lock()
		spawned = append(spawned, voidLanding{key: vk, res: res})
		mu.Unlock()
		return res, true, nil
	}
	return func() tea.Msg {
		res, err := compute(ctx, req)
		mu.Lock()
		landed := slices.Clone(spawned)
		mu.Unlock()
		return refreshResultMsg{
			surface: k.surface(),
			runID:   runID,
			wantID:  wantID,
			payload: timelinePayload{key: key, rows: res.Rows, commands: res.Commands, failures: res.Failures, labels: labels, voids: landed, leftOut: res.LeftOut},
			err:     err,
		}
	}
}

// commitTimeline adopts a completed search. Its commands become argv lines
// here, at the commit gate, on success and failure alike.
func (m Model) commitTimeline(msg refreshResultMsg) (Model, tea.Cmd) {
	return m.commitTimelineFor(m.shownList(), msg)
}

// commitTimelineFor is commitTimeline for list k.
func (m Model) commitTimelineFor(k timelineList, msg refreshResultMsg) (Model, tea.Cmd) {
	p, _ := msg.payload.(timelinePayload)
	m = m.logCommandsFor(k.surface(), msg.wantID, p.commands, failedLast(msg.err, p.commands)...)
	m = m.logLeftOut("timeline", p.leftOut)
	l := m.list(k)
	// A result for inputs the operator has left paints nothing — a late
	// search for the previous client must never replace the current list.
	if p.key != l.key {
		return m, nil
	}
	if msg.err != nil {
		// A failed search is not a list. The rows already on screen are
		// either the complete list for these same inputs (kept, C14) or
		// nothing (a change of inputs already cleared them, C44). Error
		// lane: the search the operator's navigation asked for failed (C41).
		// The dimmed list was for other inputs: it goes, and nothing is
		// awaited any more (the specification rule 8).
		l.kept, l.failed = nil, true
		if !l.complete {
			l.hasContent = false // the dimmed list goes; a complete list for these inputs stays
		}
		if len(p.failures) > 0 {
			// Every search of a comparing timeline failed: each is named (C17).
			return m.logTimelineFailures(k, p), nil
		}
		if text, ok := lookup.AspectsRefusal(msg.err); ok {
			// The aspect list's own refusal, said as the Aspects panel says it.
			return m.logError("aspects", "%s", aspectRefusalWords(text)), nil
		}
		m = m.noteEphemeris(msg.err)
		return m.logTimelineError(k, m.timelineReason(msg.err, p.commands)).logWarnings(msg.err), nil
	}
	// Searches that failed beside others that did not: the rows of the others
	// are shown, and each failure is one Log line naming its search (C17, C17a).
	m = m.logTimelineFailures(k, p)
	return m.landTimeline(p).rememberTimeline(p).rememberVoids(p), nil
}

// landTimeline puts a searched list on screen under its key. A fresh result and
// one adopted from memory land by it alike.
func (m Model) landTimeline(p timelinePayload) Model {
	l := m.list(p.key.list)
	l.rows, l.complete, l.kept = p.rows, true, nil
	l.content, l.hasContent = p.key, true
	return m.placeTimelineCursor().clampTimelineCursor()
}

// aspectRefusalWords is the aspect list's refusal as the Log says it: what did
// not happen, then why, with no closing full stop. The Aspects panel keeps the
// refusal's own sentences.
func aspectRefusalWords(text string) string {
	const lead = "Can't read the aspect list. "
	if rest, ok := strings.CutPrefix(text, lead); ok {
		return "the aspect list was not read, " + strings.TrimSuffix(rest, ".")
	}
	return text
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
		return line + ", the timeline is not shown rather than shown with bodies you hid"
	}
	return fmt.Sprintf("a line of Astrolog's list could not be read, %q", line)
}

// logTimelineError is the one writer of a list's error line.
func (m Model) logTimelineError(k timelineList, reason string) Model {
	return m.logError(k.scope(), "%s not read, %s", k.noun(), reason)
}

// logTimelineFailures writes one error line per failed search, in spawn order:
// the sky search by name, a transit or return search by its slot (numeral and
// chart name) and Astrolog's own switch for it.
func (m Model) logTimelineFailures(k timelineList, p timelinePayload) Model {
	for _, f := range p.failures {
		m = m.noteEphemeris(f.Err)
		reason := m.timelineReason(f.Err, p.commands)
		if f.Numeral == 0 {
			m = m.logError(k.scope(), "the events of the sky (%s) not listed, %s", f.Search, reason).logWarnings(f.Err)
			continue
		}
		label := p.labels[f.Numeral]
		if label == "" {
			label = panel.NumeralForColour(f.Numeral, m.cfg)
		}
		m = m.logError(k.scope(), "the %s events for %s not listed, %s", f.Search, label, reason).logWarnings(f.Err)
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
	if (!m.details.attached || m.details.followNow) && isListTab(m.activeTabID(PanelDetails)) {
		if n := m.nearestRow(rows); n >= 0 {
			m.details.cursor = n
			// Outside Details the window always centres the nearest row. Inside,
			// after a move of Now, it scrolls only when that row is out of sight.
			inSight := n >= m.details.rowOffset && n < m.details.rowOffset+visible
			if visible > 0 && (!m.details.attached || !inSight) {
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
	m.details.followNow = false
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
	m.details.followNow = false
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
	m.details.followNow = false
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
	if isListTab(m.activeTabID(PanelDetails)) {
		return m.scrollTimeline(delta)
	}
	return m.scrollDetails(delta)
}

// attachTab is the Details tab Enter will show: from the Chart the default tab
// becomes the Timeline and any other choice stays; from Bodies and Aspects the
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

// attachTitle is Enter's title on the bar: Details, at every size.
func (m Model) attachTitle() string { return "Details" }

// canAttachDetails is whether Enter has a Details to open from here: any panel
// of the left column, at any size the layout draws.
func (m Model) canAttachDetails() bool {
	return isFocusable(m.focused) && m.layout() != nil
}

// attachDetails is Enter from a ring panel (C15): Details opens, beside the
// panels or right below this one, and takes row navigation. Focus does not
// move, and nothing computes here: the reconciler asks for what Details owes.
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
	return m.placeDetailsCursor(), nil
}

// placeDetailsCursor puts the timeline's row cursor where attaching puts it,
// for the subject Details now belongs to.
func (m Model) placeDetailsCursor() Model {
	if isListTab(m.activeTabID(PanelDetails)) {
		m.details.cursorSet = false
		m = m.placeTimelineCursor()
	}
	return m.clampTimelineCursor()
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
// so it is the subject's panel already. Where Details stood right below the
// panel it goes with the Esc, and what was computed for it goes too.
func (m Model) detachDetails() (Model, tea.Cmd) {
	m.details.attached = false
	m = m.restorePreEnterTab()
	for _, k := range allLists {
		m.list(k).searching = false
	}
	m.details.searching = false
	m = m.clampTimelineCursor()
	if !m.detailsVisible() {
		return m.suspendDetails(), nil
	}
	// Rule 6 of the specification.
	return m.catchUpDetails()
}

// updateDetailsKey owns the keyboard while Details is attached (D18), the
// way updateConsoleKey does for the console: Details resolves each key from
// its own table. A key it does not name goes on to the main screen unless
// detailsRefused names it, so no key that would move, select, reorder or remove
// something in a panel under Details acts (C48). The number keys, Tab and
// Shift+Tab are the exception: they move the focus, and Details goes with it.
// Left leaves Details and Right does nothing.
func (m Model) updateDetailsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.list(m.shownList()).searching && m.timelineSearchLine() {
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
	if tm, cmd, ok := m.pressTimeStep(id); ok {
		return tm, cmd
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
	case keys.BindingNextView:
		return m, nil
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
	// The lists are cut to the floating slot's month, and the step keys move
	// it: `o`/`O` step a month, `p`/`P` jump to a Moon phase and `n` jumps to
	// now. Each takes its ordinary path, so the commit that
	// re-keys the searches is untouched. The table offers them on both tabs and
	// from any cursor position.
	case keys.BindingJumpNow:
		return m.jumpFloatingToNow()
	case keys.BindingFocusSession, keys.BindingFocusChart, keys.BindingFocusBodies, keys.BindingFocusAspects:
		return m.setFocusedPanel(panelDigits[id])
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
	keys.BindingSelect:        "selects a chart or a body in the focused panel",
	keys.BindingSlotCycleUp:   "moves the Chart or Bodies cursor",
	keys.BindingSlotCycleDown: "moves the Chart or Bodies cursor",
	keys.BindingPageHome:      "moves the Chart or Bodies cursor",
	keys.BindingPageEnd:       "moves the Chart or Bodies cursor",
	keys.BindingSlotMoveUp:    "reorders the charts",
	keys.BindingSlotMoveDown:  "reorders the charts",
	keys.BindingSlotDelete:    "removes a chart",
	keys.BindingAttach:        "Details is open already",
}

// passDetailsKeyToMain is a key Details does not answer itself: the main
// screen's own dispatch, unless the key is refused. A popup or the console
// opened from here owns the keys ahead of attached Details (currentOwner), and
// Details stays attached underneath, so closing it lands back here.
func (m Model) passDetailsKeyToMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	id := keys.MatchBinding(msg, m.available(ownerMain, mainBindings(m.keys)))
	if _, refused := detailsRefused[id]; refused {
		return m, nil
	}
	return m.dispatchMainBinding(id, msg)
}

// stepDetailsRow is ↑/↓/j/k while attached: the row cursor on the timeline
// tab (C17), one report line on the report tab (C26). Neither moves a slot.
func (m Model) stepDetailsRow(step int) Model {
	if isListTab(m.activeTabID(PanelDetails)) {
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
// the specification). Every other row lands on the last.
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
// C2, C3). Then Details closes, as Esc closes it: the jump is what Enter was
// for, on every list tab that travels in time; the enter-stays experiment
// keeps it open. On the report tab, or with no row under the cursor, nothing
// moves and Details stays.
func (m Model) jumpToTimelineRow() (Model, tea.Cmd) {
	if !isListTab(m.activeTabID(PanelDetails)) {
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
	m = m.aimNow(instant)
	// The pressed row is set after the presses, which each end it (rule 8).
	m.details.pressedAt, m.details.pressedSet = markOf(row), true
	m.details.followNow = false
	m.details.pressedChart = rowChart
	// Enter stores what it shows (S4a A19): with the floating slot selected, a
	// chart row's tab is "now in that chart" (the base is the chart) and a sky
	// row shows now alone (no base). With the floating slot unselected (six
	// charts already selected) there is no pair to store and the base stays.
	if floatSelected {
		m = m.withBase(rowChart)
	}
	// The jump moves the moment: every filter ends, and the cursor stays on the row.
	m.details.cursorAt, m.details.cursorSet = markOf(row), true
	var cmd tea.Cmd
	if moved || m.comparisonSig() != before {
		m, cmd = m.afterFocusChange()
		m = m.clearDetailsFilters()
	} else {
		m, cmd = m.setFloatingTarget(instant)
	}
	m = m.clampTimelineCursor()
	if !m.details.attached || m.experiment(expEnterStays) {
		return m, cmd
	}
	m, closed := m.detachDetails()
	return m, tea.Batch(cmd, closed)
}

// copiedRows is what `,y` copies on the timeline tab: the marked rows in time
// order, or every row shown when none is marked, each as the panel displays it
// (aligned, unclipped). The list is already in time order.
func (m Model) copiedRows() []string {
	var out []string
	shown := m.timelineShown()
	pad := panel.AnyJulian(shown)
	marks := m.list(m.shownList()).marks
	for _, r := range shown {
		if len(marks) == 0 || marks[markOf(r)] {
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
	if isListTab(m.activeTabID(PanelDetails)) {
		rows := m.copiedRows()
		noun := "rows"
		if len(rows) == 1 {
			noun = "row"
		}
		text, scope, what = strings.Join(rows, "\n"), m.shownList().scope(), fmt.Sprintf("%d %s copied", len(rows), noun)
	} else {
		text, scope, what = m.detailsBodyShown(), "details", "the report copied"
	}
	if text == "" {
		return m.showNotice(noticeNothingCopy)
	}
	if err := yankLine(text); err != nil {
		return m.logError(scope, "the copy did not reach the clipboard").showNotice(noticeCopyFailed)
	}
	m = m.logNote(scope, "%s", what)
	m.yank.gen++
	m.yank.active = true
	m.yank.notice = ""
	m.yank.chars = utf8.RuneCountInString(text)
	return m, armYankFlash(m.yank.gen)
}
