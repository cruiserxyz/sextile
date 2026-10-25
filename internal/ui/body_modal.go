package ui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// body_modal.go is the body selector (⌃b): which objects the chart draws.
//
// It is the aspect chooser's sibling and deliberately the SMALLER of the two,
// because it binds the chassis that slice extracted rather than growing its
// own. Every piece of list machinery here comes from fuzzyList — the search
// buffer, the cursor, the window and its scroll margin — and the box chrome
// from renderModalBox. This file declares no list, no cursor and no search
// editor of its own; that is C6, and it is the whole argument for the
// extraction having been worth doing.
//
// Two shape differences from the aspect chooser:
//
//   - GROUP ROWS above the list (the specification), the aspect
//     families' shape: five tri-state toggles pinned over a dashed rule, and
//     one combined cursor that walks them first, so the list index is the
//     cursor minus groupRows().
//   - A TAG BAR in the pinned block BELOW, this being that block's first user.
//     134 rows do not fit on any screen, so the operator cannot see their
//     selection by scanning the list; the tag bar is where the chosen set
//     stays visible while they filter.

// bodyModalState is the selector's state. The list machinery is embedded, so
// filterText, cursor, matches and truncated all come from the chassis.
type bodyModalState struct {
	fuzzyList[aspect.RegistryObject]

	// registry is every object the user's binary carries, read once at
	// startup from `astrolog -R1 -HO`. Nil until it has been read; the modal
	// refuses to open.
	registry []aspect.RegistryObject

	// selected holds Astrolog object NUMBERS the operator has ticked. Not
	// aspect.Body values: the registry is 0-based and runs to 133, and 117 of
	// its rows have no enum name at all.
	selected map[int]bool

	// entry is the policy in force when the modal opened. Only the switched-off
	// rule (chooserCloseKeeps = false) restores it, on Esc; by default every
	// closing key keeps the ticks.
	entry recipe.ObjectPolicy

	// The ephemeris probe (the specification). All of it
	// lives and dies with this opening of the popup, so closing forgets every
	// verdict (C6) and a result that outlives its opening is recognised by
	// seq and dropped (N6).
	//
	// seq numbers the opening. probes holds one verdict per probed body, each
	// tagged with the inputs it was asked under. inputs are the inputs now in
	// force. restID is the latest armed rest timer; an older one is stale.
	// cancel ends the probe in flight.
	seq    uint64
	probes map[int]bodyProbeEntry
	inputs probeInputs
	restID uint64
	cancel context.CancelFunc
	// inflight is the body whose probe is running now, if any.
	inflight   int
	inflightOn bool
}

// probeInputs is everything a probe's answer depends on besides the body: the
// focused slot's moment and place, its effective frame and the node mode.
// Comparable, so "the same inputs" is ==.
type probeInputs struct {
	when     int64
	lat, lon float64
	tz       string
	houses   recipe.HouseSystem
	zodiac   recipe.Zodiac
	trueNode bool
}

// bodyProbeEntry is one verdict: Drawable, or not with the files Astrolog named.
type bodyProbeEntry struct {
	inputs   probeInputs
	drawable bool
	files    []string
}

// probeRest is how long the cursor must stay on a body before it is probed:
// the details debounce (refresh.go), so holding a key through the list runs
// nothing until the cursor stops.
const probeRest = detailsDebounce

// probeBody asks Astrolog about one body (a seam for tests).
var probeBody = lookup.ProbeBody

// bodyProbeRestMsg fires when the rest timer armed for body obj elapses.
type bodyProbeRestMsg struct {
	seq, restID uint64
	obj         int
}

// bodyProbeResultMsg carries one probe's verdict back to the opening that asked.
type bodyProbeResultMsg struct {
	seq       uint64
	restID    uint64 // the rest the probe started under
	obj       int
	inputs    probeInputs
	probe     lookup.BodyProbe
	err       error
	cancelled bool
}

// objectUnderCursor is the registry object on the cursor row, if any. The
// cursor walks the group rows first, so on one of them there is none.
func (b bodyModalState) objectUnderCursor() (aspect.RegistryObject, bool) {
	i := b.cursor - groupRows()
	if i < 0 || i >= len(b.matches) {
		return aspect.RegistryObject{}, false
	}
	return b.matches[i], true
}

// setProbe records a verdict for a body under the inputs it was asked with.
func (b *bodyModalState) setProbe(obj int, in probeInputs, p lookup.BodyProbe) {
	if b.probes == nil {
		b.probes = map[int]bodyProbeEntry{}
	}
	b.probes[obj] = bodyProbeEntry{inputs: in, drawable: p.Drawable, files: p.MissingFiles}
}

// recordProbe stores a verdict under the inputs now in force.
func (b *bodyModalState) recordProbe(obj int, drawable bool, files []string) {
	b.setProbe(obj, b.inputs, lookup.BodyProbe{Drawable: drawable, MissingFiles: files})
}

// undrawable is the verdict for obj when it is current and negative.
func (b bodyModalState) undrawable(obj int) (bodyProbeEntry, bool) {
	e, ok := b.probes[obj]
	if !ok || e.drawable || e.inputs != b.inputs {
		return bodyProbeEntry{}, false
	}
	return e, true
}

// stopProbe ends the probe in flight; its result, if one still arrives, is
// silent.
func (b *bodyModalState) stopProbe() {
	b.inflightOn = false
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
}

// probeTarget is what a probe would ask about: the focused slot's moment and
// place and its effective frame, resolved the way the chart compute resolves
// them (buildTextCmd). ok is false when there is nothing to ask — no
// workspace, a zone that is not known, or a house system that could not be
// read — and then no call is made and nothing is marked.
func (m Model) probeTarget() (in probeInputs, when time.Time, place location.PlaceInfo, ok bool) {
	slot := m.focusedSlot()
	if slot == nil || m.unresolvedFor(slot) != nil {
		return in, when, place, false
	}
	if chart := m.focusedChart(); !chart.IsZero() {
		when, place = chart.When, chart.Place
	} else {
		when, place = transitTimeFromWorkspace(m), m.place
	}
	// A zone that is empty or does not load is unresolved, as it is for the
	// chart call: no call, nothing marked, nothing said.
	if !place.HasTimezone() {
		return in, when, place, false
	}
	if _, err := time.LoadLocation(place.TZ); err != nil {
		return in, when, place, false
	}
	lens := m.viewLens()
	in = probeInputs{
		when: when.UnixNano(), lat: place.Lat, lon: place.Lon, tz: place.TZ,
		houses: lens.Houses, zodiac: lens.Zodiac, trueNode: lens.Objects.TrueNodeMode,
	}
	return in, when, place, true
}

// armProbeRest is called when the object under the cursor may have changed. It
// ends the probe in flight, makes every earlier rest timer stale and, for a
// body that can be probed, arms one for the new row. The returned Cmd is only
// a timer: nothing is spawned until the cursor has rested.
func (m Model) armProbeRest() (Model, tea.Cmd) {
	m.bodyModal.stopProbe()
	return m.armRest()
}

// restartProbeRest is called for a key that left the same body under the
// cursor: the cursor has not rested for 150 ms if a key arrived, so a rest
// still pending starts over. A body with a current verdict, or whose probe is
// already running, is left alone — the running probe is not cancelled.
func (m Model) restartProbeRest() (Model, tea.Cmd) {
	cur, ok := m.bodyModal.objectUnderCursor()
	if !ok {
		return m, nil
	}
	if m.bodyModal.inflightOn && m.bodyModal.inflight == cur.Number {
		return m, nil
	}
	in, _, _, resolved := m.probeTarget()
	if !resolved {
		return m, nil
	}
	if e, seen := m.bodyModal.probes[cur.Number]; seen && e.inputs == in {
		return m, nil
	}
	return m.armRest()
}

// armRest makes every earlier rest timer stale and, for a body that can be
// probed, arms one for the row under the cursor.
func (m Model) armRest() (Model, tea.Cmd) {
	m.bodyModal.restID++
	in, _, _, ok := m.probeTarget()
	if ok {
		m.bodyModal.inputs = in
	}
	cur, curOK := m.bodyModal.objectUnderCursor()
	if !ok || !curOK || (cur.Number >= recipe.HouseWheelObjectFirst && cur.Number <= recipe.HouseWheelObjectLast) {
		return m, nil
	}
	seq, restID, obj := m.bodyModal.seq, m.bodyModal.restID, cur.Number
	return m, tea.Tick(probeRest, func(time.Time) tea.Msg {
		return bodyProbeRestMsg{seq: seq, restID: restID, obj: obj}
	})
}

// applyBodyProbeRest starts the probe for the body the cursor has rested on.
func applyBodyProbeRest(m Model, msg bodyProbeRestMsg) (Model, tea.Cmd) {
	b := m.bodyModal
	if m.startup != startupBodyOpen || msg.seq != b.seq || msg.restID != b.restID {
		return m, nil
	}
	if cur, ok := b.objectUnderCursor(); !ok || cur.Number != msg.obj {
		return m, nil
	}
	in, when, place, ok := m.probeTarget()
	if !ok {
		return m, nil
	}
	m.bodyModal.inputs = in
	if e, seen := m.bodyModal.probes[msg.obj]; seen && e.inputs == in {
		return m, nil
	}
	m.bodyModal.stopProbe()
	ctx, cancel := context.WithCancel(context.Background())
	m.bodyModal.cancel = cancel
	m.bodyModal.inflight, m.bodyModal.inflightOn = msg.obj, true
	lens, probers := m.viewLens(), m.probers
	seq, restID, obj := msg.seq, msg.restID, msg.obj
	return m, func() tea.Msg {
		p, err := probeBody(ctx, probers, when, place, lens, obj)
		return bodyProbeResultMsg{
			seq: seq, restID: restID, obj: obj, inputs: in, probe: p, err: err,
			cancelled: ctx.Err() != nil,
		}
	}
}

// applyBodyProbeResult records a verdict, or says once that the probe failed.
// A result for a popup that has closed, or been reopened, is dropped without a
// word, as is a probe that was cancelled because the cursor moved on.
func applyBodyProbeResult(m Model, msg bodyProbeResultMsg) (Model, tea.Cmd) {
	if m.startup != startupBodyOpen || msg.seq != m.bodyModal.seq {
		return m, nil
	}
	if m.bodyModal.inflightOn && m.bodyModal.inflight == msg.obj {
		m.bodyModal.inflightOn = false
	}
	if msg.err != nil {
		// A probe that failed because it was cancelled is silent, and so is one
		// that finished before the cursor moved on but was delivered after:
		// the rest it started under is no longer the current one.
		if msg.cancelled || errors.Is(msg.err, context.Canceled) || msg.restID != m.bodyModal.restID {
			return m, nil
		}
		// The body's name and nothing else: the error carries Astrolog's
		// stderr and the command, which hold the client's chart details.
		return m.logNote("bodies", "couldn't check whether %s can be drawn", m.bodyName(msg.obj)), nil
	}
	m.bodyModal.setProbe(msg.obj, msg.inputs, msg.probe)
	return m, nil
}

// bodyName is the registry's name for an object number.
func (m Model) bodyName(obj int) string {
	for _, o := range m.bodyModal.registry {
		if o.Number == obj {
			return o.Name
		}
	}
	return fmt.Sprintf("object %d", obj)
}

// tagBarRows is how many pinned rows sit BELOW the list. One line, wrapped
// never — the bar elides rather than growing, so the list height is a constant
// and the window cannot resize under a moving cursor.
const tagBarRows = 1

// refilter recomputes the visible objects, best match first by
// location.MatchTier over the registry's NAME, ties in registry order. The name
// is the only text a row carries: the registry has no abbreviation field.
func (b *bodyModalState) refilter() {
	if b.registry == nil {
		b.clear()
		return
	}
	q := strings.ToLower(strings.TrimSpace(b.filterText))
	if q == "" {
		b.setMatches(append([]aspect.RegistryObject(nil), b.registry...), false)
		return
	}
	var hits []aspect.RegistryObject
	var tiers []int
	for _, o := range b.registry {
		if t := location.MatchTier(strings.ToLower(o.Name), q); t > 0 {
			hits = append(hits, o)
			tiers = append(tiers, t)
		}
	}
	b.setMatches(rankByTier(hits, tiers), false)
}

// openBodyModal seeds the selector from the policy in force.
//
// The seed when there is NO override is the lens's own drawn set, not every
// object the registry carries. The aspect chooser seeds all counting aspects
// there, and the specification § Open leaves the question open with an
// instruction not to diverge — but the two cases are not alike in size, and
// following it literally here would be a different behaviour, not the same one.
// Aspects: 24 rows, of which the host has most on. Objects: 134 rows, of which
// the shipped lens draws eighteen. Seeding everything would mean the operator
// opens the selector, unticks one fixed star, confirms, and gets a 133-object
// wheel they never asked for — the first keystroke silently widening the chart
// by two orders of magnitude rather than by a little.
//
// Seeding from what is on screen keeps the analogy where it actually holds: the
// aspect chooser's defect is that it seeds from the REGISTRY instead of from
// what the chart is doing, and this seeds from what the chart is doing. When
// the aspect side is fixed it will move toward this, not away from it.
func openBodyModal(registry []aspect.RegistryObject, policy recipe.ObjectPolicy, lensDefault []int) bodyModalState {
	st := bodyModalState{registry: registry, selected: map[int]bool{}, entry: policy, probes: map[int]bodyProbeEntry{}}
	if policy.Baseline == recipe.Explicit {
		for _, n := range policy.Selection {
			st.selected[n] = true
		}
	} else {
		for _, n := range lensDefault {
			st.selected[n] = true
		}
	}
	st.refilter()
	return st
}

// policy turns the ticked set into an ObjectPolicy. It is always Explicit —
// the operator confirmed a set, even an empty one.
//
// TrueNodeMode is carried over from the entry policy rather than dropped.
// It selects the true-node and Black-Moon-Lilith VARIANTS of objects 16 and 18,
// which is orthogonal to which objects are drawn: losing it here would silently
// swap the mean node for the true one the first time the operator opened this
// modal, in a slice that is not about nodes at all.
func (b bodyModalState) policy() recipe.ObjectPolicy {
	sel := make([]int, 0, len(b.selected))
	for n, on := range b.selected {
		if on {
			sel = append(sel, n)
		}
	}
	sort.Ints(sel)
	return recipe.ObjectPolicy{
		Baseline:     recipe.Explicit,
		Selection:    sel,
		TrueNodeMode: b.entry.TrueNodeMode,
	}
}

// toggleAt flips the row under the cursor. Out-of-range is a no-op rather than
// a panic: a filter can empty the list while the cursor still holds a value.
//
// The cursor is combined, as in the aspect chooser: the first groupRows()
// positions are the group rows, and the list index is the cursor minus them.
func (b *bodyModalState) toggleAt(cursor int) {
	if cursor >= 0 && cursor < groupRows() {
		b.toggleGroup(cursor)
		return
	}
	i := cursor - groupRows()
	if i < 0 || i >= len(b.matches) {
		return
	}
	n := b.matches[i].Number
	b.selected[n] = !b.selected[n]
}

// groupRows is how many pinned group rows sit above the list.
func groupRows() int { return len(recipe.ObjectGroups) }

// bodyPinnedRows is the lines pinned above the list: the group rows and the
// rule under them. Only the group rows are cursor rows.
func bodyPinnedRows() int { return groupRows() + 1 }

// totalRows is the group block plus the filtered list.
func (b bodyModalState) totalRows() int { return groupRows() + len(b.matches) }

// groupMembers is the object numbers of group g that the filter leaves
// visible. Like the aspect families, a group follows the filter: its count,
// its state and its toggle all act on what the operator can see.
func (b bodyModalState) groupMembers(g int) []int {
	var out []int
	for _, o := range b.matches {
		if recipe.ObjectGroupOf(o.Number) == g {
			out = append(out, o.Number)
		}
	}
	return out
}

// groupState is how a group row reads, derived every render.
func (b bodyModalState) groupState(g int) familyState {
	members := b.groupMembers(g)
	on := 0
	for _, n := range members {
		if b.selected[n] {
			on++
		}
	}
	switch {
	case on == 0:
		return familyNone
	case on == len(members):
		return familyAll
	default:
		return familyPartial
	}
}

// toggleGroup ticks every visible member unless all are ticked, then unticks
// them all (exactly aspectModalState.toggleFamily).
func (b *bodyModalState) toggleGroup(g int) {
	members := b.groupMembers(g)
	if len(members) == 0 {
		return
	}
	want := b.groupState(g) != familyAll
	for _, n := range members {
		b.selected[n] = want
	}
}

// bodyListHeight is the rows the list gets: the box's list area minus the tag
// bar and the pinned group block with its rule. Navigation needs the same number
// drawing does, or the cursor lands outside the window that was painted.
func bodyListHeight(termH int) int {
	h := modalListHeight(termH) - tagBarRows - bodyPinnedRows()
	if h < 1 {
		h = 1
	}
	return h
}

// renderBodyModal draws the selector: a flat filtered list with the tag bar
// pinned beneath it.
func renderBodyModal(state bodyModalState, cfg Config, termW, termH int) string {
	pos := listPosition{Cursor: state.cursor, Total: state.totalRows()}
	// The probe's status line sits directly below the pinned bar, and takes a
	// list row only while the cursor is on a body that cannot be drawn. The
	// window is laid out for the height that remains, so the cursor row and
	// the bar are never the rows that give way (N5).
	status, hasStatus := bodyProbeStatus(state)
	// The note is given a row only if a list row is left beside it: at 80x24
	// the group block, its rule and the bar leave the list one row, and the
	// cursor row outranks the note (the specification 7).
	statusRows := func(height int) int {
		if hasStatus && height-tagBarRows-bodyPinnedRows() >= 2 {
			return 1
		}
		return 0
	}
	return renderModalBox(modalBox{
		Title:    popupTitleBodies,
		Position: &pos,
		Scroll: func(height int) (viewport, int) {
			// The tag bar below is pinned and is not part of the track.
			h := height - tagBarRows - statusRows(height) - bodyPinnedRows()
			_, offset := state.windowAt(state.cursor-groupRows(), h)
			return viewport{Total: len(state.matches), Visible: h, Offset: offset}, bodyPinnedRows()
		},
		FilterText: state.filterText,
		Prompt:     "Search bodies: ",
	}, cfg, termW, termH, func(width, height int) string {
		extra := statusRows(height)
		below := []string{listNote(bodyTagBar(state, width, cfg), width, cfg)}
		if extra > 0 {
			below = append(below, bodyProbeNote(status, width, cfg))
		}
		return renderPinnedBody(pinnedBody{
			Above: append(bodyGroupRows(state, cfg, width), listRule(cfg, width)),
			Rows:  bodyListRows(state, cfg, width, height-tagBarRows-extra-bodyPinnedRows()),
			Below: below,
		}, width, height)
	})
}

// bodyListRows renders the filtered list.
func bodyListRows(state bodyModalState, cfg Config, width, height int) []string {
	if height < 1 {
		return nil
	}
	if state.registry == nil {
		return []string{listNote("(no object registry)", width, cfg)}
	}
	if len(state.matches) == 0 {
		return []string{listNote("(no matching bodies)", width, cfg)}
	}
	// The list's own index is the combined cursor minus the group rows; a
	// negative one means the cursor is up in the group block.
	listCursor := state.cursor - groupRows()
	visible, offset := state.windowAt(listCursor, height)
	rows := make([]string, 0, len(visible))
	for i, o := range visible {
		rows = append(rows, listRow(bodyRow(state, o, listCursor >= 0 && offset+i == listCursor), cfg, width))
	}
	return rows
}

// The three marks a group row draws, and the legend in the tag bar explains.
const (
	groupMarkAll  = "●"
	groupMarkSome = "◐"
	groupMarkNone = "○"
)

// bodyGroupRows renders the pinned group block: state glyph, name and the
// count of members the filter leaves visible, in the aspect families' form.
func bodyGroupRows(state bodyModalState, cfg Config, width int) []string {
	rows := make([]string, 0, groupRows())
	for i, g := range recipe.ObjectGroups {
		mark := groupMarkNone
		switch state.groupState(i) {
		case familyAll:
			mark = groupMarkAll
		case familyPartial:
			mark = groupMarkSome
		}
		rows = append(rows, listRow(listRowSpec{
			Left:   fmt.Sprintf("[%s] %-15s (%d)", mark, g.Name, len(state.groupMembers(i))),
			Cursor: state.cursor == i,
		}, cfg, width))
	}
	return rows
}

// bodyProbeNote draws the missing-file line: not a pickable row, so it styles
// its own text, in the key bar's error colour when colours are on so it
// catches the eye (operator, 2026-10-11: "a reddish color").
func bodyProbeNote(status string, width int, cfg Config) string {
	line := listNote(status, width, cfg)
	if cfg.useColor() {
		line = lipgloss.NewStyle().Foreground(errorColour).Render(line)
	}
	return line
}

// bodyProbeStatus is the line under the Enabled bar while the cursor is on a
// body Astrolog cannot draw: which body, and which file it is missing.
func bodyProbeStatus(state bodyModalState) (string, bool) {
	cur, ok := state.objectUnderCursor()
	if !ok {
		return "", false
	}
	e, marked := state.undrawable(cur.Number)
	if !marked {
		return "", false
	}
	return fmt.Sprintf("%s can't be drawn: no ephemeris file %s", cur.Name, strings.Join(e.files, ", ")), true
}

// bodyRow is one object: tick box and the object's full name, on the left, with
// no right cell (the specification C9b).
//
// The form is legacy's, settled in the specification § Taste lane V4 and
// not re-litigated here. Bodies have no angle and no orb, so the aspect
// chooser's four-field row does not transfer and nothing replaces it. The
// registry number is deliberately NOT shown: the short form lives in the tag
// bar and nowhere else.
func bodyRow(state bodyModalState, o aspect.RegistryObject, onCursor bool) listRowSpec {
	mark := " "
	if state.selected[o.Number] {
		mark = "✓"
	}
	name := o.Name
	_, marked := state.undrawable(o.Number)
	if marked {
		// Astrolog could not draw it: `!` before the name, dim with colours on.
		name = "! " + name
	}
	return listRowSpec{
		Left:   fmt.Sprintf("[%s] %s", mark, name),
		Cursor: onCursor,
		Dim:    marked,
	}
}

// bodyTagBar is the pinned block below the list: how many bodies are ticked,
// what the group marks mean, and the chosen names in registry order.
//
// It exists because the list cannot show the selection. The registry does not
// fit on any terminal, and the moment the operator types a filter most of what
// they ticked is off screen. The count is of the whole selection, whatever the
// filter shows.
//
// The names follow the legend after one space and are cut with the cut mark;
// when the legend leaves no room for them they are omitted. It never wraps, so
// the list height stays constant and the window cannot resize under a moving
// cursor.
func bodyTagBar(state bodyModalState, width int, cfg Config) string {
	var names []string
	for _, o := range state.registry {
		if state.selected[o.Number] {
			names = append(names, o.Name)
		}
	}
	dot := "·"
	if cfg.useASCII() {
		dot = "-"
	}
	legend := fmt.Sprintf("%d of %d selected %s %s all %s some %s none",
		len(names), len(state.registry), dot, groupMarkAll, groupMarkSome, groupMarkNone)
	room := width - ansi.StringWidth(legend) - 1
	if len(names) == 0 || room <= ansi.StringWidth(cutMark(cfg)) {
		return cutRight(legend, width, cfg)
	}
	return legend + " " + cutRight(strings.Join(names, " "), room, cfg)
}

// updateBodyModalKey handles one keystroke. The caller in model.go has already
// gated on the modal being open.
//
// Nothing here starts an Astrolog process: every branch returns state only, and
// only confirming hands a policy back to the caller (C7).
func updateBodyModalKey(state bodyModalState, msg tea.KeyMsg, listHeight int) (bodyModalState, bodyModalOutcome) {
	switch matchBinding(msg, bodiesBindings()) {
	case BindingConfirm:
		return state, bodyModalOutcome{Confirmed: true, Policy: state.policy()}
	case BindingClose:
		if chooserCloseKeeps {
			return state, bodyModalOutcome{Confirmed: true, Dismissed: msg.Type == tea.KeyEsc, Policy: state.policy()}
		}
		return state, bodyModalOutcome{Cancelled: true, Policy: state.entry}
	case BindingListUp:
		if state.cursor > 0 {
			state.cursor--
		}
		state.scrollTo(state.cursor-groupRows(), listHeight)
	case BindingListDown:
		if state.cursor+1 < state.totalRows() {
			state.cursor++
		}
		state.scrollTo(state.cursor-groupRows(), listHeight)
	case BindingPageUp:
		state.pageBodies(pageUp, listHeight)
	case BindingPageDown:
		state.pageBodies(pageDown, listHeight)
	case BindingPageHome:
		state.pageBodies(pageHome, listHeight)
	case BindingPageEnd:
		state.pageBodies(pageEnd, listHeight)
	case BindingToggle:
		state.toggleAt(state.cursor)
	default:
		if state.typeKey(msg) {
			cur := state.cursor // refilter resets it
			state.refilter()
			state.cursor = cursorAfterTyping(cur, groupRows(), len(state.matches))
			state.scrollTo(state.cursor-groupRows(), listHeight)
		}
	}
	return state, bodyModalOutcome{}
}

// pageBodies applies a paging key by the one rule. The probe's status line takes
// a list row only while the cursor is on a body that cannot be drawn, so the
// number of rows a page can rely on is one short whenever there is room for the
// note (the smallest count the render shows). The stored window is then held
// where the render draws it for the cursor's new row, with or without the note.
func (b *bodyModalState) pageBodies(op pageOp, listHeight int) {
	smallest := listHeight
	if listHeight >= 2 {
		smallest--
	}
	b.pageBelow(groupRows(), op, smallest)
	h := listHeight
	if _, has := bodyProbeStatus(*b); has && listHeight >= 2 {
		h--
	}
	_, b.offset = b.windowAt(b.cursor-groupRows(), h)
}

// bodyModalOutcome is what one keystroke decided. Both Confirmed and Cancelled
// carry the policy the caller should put in force, so the caller never has to
// know which of the two restores and which applies.
type bodyModalOutcome struct {
	Confirmed bool
	// Dismissed is a Confirmed that came from Esc rather than Enter: the
	// ticks stay in force, and with none made there is nothing to apply.
	Dismissed bool
	Cancelled bool
	Policy    recipe.ObjectPolicy
}

// sameObjectSelection compares two policies by what they would compute: the
// profile and the normalised selection. Comparing the structs directly would
// call two policies different over selection ORDER, which the compiler sorts
// away — and would compare the Add/Remove maps, which are nil under Explicit.
func sameObjectSelection(a, b recipe.ObjectPolicy) bool {
	if a.Baseline != b.Baseline || a.TrueNodeMode != b.TrueNodeMode {
		return false
	}
	x, y := a.Selected(), b.Selected()
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	// Under a delta baseline Selected() is nil for both, so the deltas decide.
	if a.Baseline != recipe.Explicit {
		return sameBodySet(a.Add, b.Add) && sameBodySet(a.Remove, b.Remove)
	}
	return true
}

func sameBodySet(x, y recipe.BodySet) bool {
	if len(x) != len(y) {
		return false
	}
	for k := range x {
		if !y.Has(k) {
			return false
		}
	}
	return true
}

// applyBodyModalKey is the Model-level half of the selector: it runs one
// keystroke through the modal and, when the operator finishes, puts the
// resulting policy in force and recomputes.
//
// Nothing here starts a process except the confirm branch and the debounced
// preview — opening, filtering and moving all return a nil Cmd (C7).
func applyBodyModalKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// SNAPSHOT, not a struct copy. bodyModalState holds `selected` as a map,
	// so a copied struct shares that map with the original and any comparison
	// afterwards reports "unchanged" no matter what the keystroke did.
	beforeTicks := ticksOn(m.bodyModal.selected)
	beforeObj, beforeOK := m.bodyModal.objectUnderCursor()
	beforeFilter := m.bodyModal.filterText
	next, out := updateBodyModalKey(m.bodyModal, msg, bodyListHeight(m.height))
	m.bodyModal = next

	switch {
	case out.Cancelled:
		m.bodyModal.stopProbe()
		m.startup = startupReady
		restored := !sameObjectSelection(m.lens.Objects, out.Policy)
		m.lens.Objects = out.Policy
		if !restored {
			return m, nil
		}
		// Reached only with chooserCloseKeeps = false. A preview is on screen,
		// or on its way: the frame from when the selector opened comes back at
		// once and the preview is fenced (C7a).
		return m.restoreChooserEntry()
	case out.Confirmed:
		m.bodyModal.stopProbe()
		m.startup = startupReady
		if out.Dismissed && sameObjectSelection(m.lens.Objects, m.chooserEntry.lens.Objects) {
			// Esc with nothing ticked changes no lens and starts no
			// work (C9).
			return m, nil
		}
		m.lens.Objects = out.Policy
		// bumpIdentity (not noteWant): a new object set changes WHAT the
		// surface computes, so an in-flight old-set result must never paint.
		//
		// The timeline follows the committed lens, so confirming is where its
		// searches re-run (once: syncTimeline is idempotent per key).
		return m.confirmChooser()
	}

	// Live preview, on the same contract as the aspect chooser: a toggle
	// repaints the panels BEHIND the modal, so the operator judges the set
	// they are building against the chart rather than against their memory of
	// it. noteWant with leading=false coalesces a run of toggles into ONE
	// compute, so holding Space costs one Astrolog process and not one per
	// keystroke.
	//
	// The trigger is a changed SELECTION between before and after this key,
	// never a difference from the policy in force: opening with no override
	// seeds the lens's set, which is already a different POLICY from the
	// delta one in force, so comparing against that would preview on every
	// keystroke including plain typing.
	//
	// The probe is the other thing a key can start, and only a timer: when the
	// body under the cursor changed — by a cursor key, or by a filter that put
	// another object under the same row — the rest timer is armed, and the
	// call itself waits for it (the specification C1).
	var probeCmd tea.Cmd
	if cur, ok := next.objectUnderCursor(); ok != beforeOK || (ok && cur.Number != beforeObj.Number) {
		m, probeCmd = m.armProbeRest()
	} else if matchBinding(msg, bodiesBindings()) != BindingNone || next.filterText != beforeFilter {
		// The same body is still under the cursor: a key the popup acts on
		// restarts a rest that is still pending. A key it ignores (Ctrl+R, say)
		// is not a key press as far as the popup is concerned.
		m, probeCmd = m.restartProbeRest()
	}
	if !sameTickSets(beforeTicks, ticksOn(next.selected)) {
		m.lens.Objects = next.policy()
		tm, previewCmd := m.noteWant(surfaceText, false)
		return tm, tea.Batch(previewCmd, probeCmd)
	}
	return m, probeCmd
}

// openBodySelector is the ⌃b handler. It refuses to open when the registry
// could not be read, and the frame keeps computing with whatever was already
// in force (C10). The failed read was logged once, when it landed; the key
// itself says nothing (the specification Part B). Offering a list Sextile
// invented would be worse than not opening: the operator would pick from a set
// their binary does not have.
func openBodySelector(m Model) (tea.Model, tea.Cmd) {
	// `startup` does double duty in this model: it names the startup phase AND
	// which modal is open, so a modal opened before the first frame commits is
	// silently overwritten the moment it does. Until then the key is inert.
	if m.startup != startupReady {
		return m, nil
	}
	if m.objectRegistry == nil {
		if m.objectRegErr != nil {
			return m, nil
		}
		// A read that has not LANDED yet is not a refusal. Take the request
		// and open when it arrives, rather than making the key depend on how
		// long the operator waited.
		//
		// It starts no SECOND read. Init fires one unconditionally, so a nil
		// registry with no error means exactly one thing — the startup read
		// is in flight — and a duplicate spends another Astrolog process for
		// the same answer. Worse, the two race: if the duplicate failed
		// first it would clear this pending flag, and the original's success
		// would arrive with nothing to open. C5a asks for one read; this is
		// what makes it one.
		m.bodyOpenPending = true
		return m, nil
	}
	m.bodyModal = openBodyModal(m.objectRegistry, m.lens.Objects, m.lens.VisibleObjectNumbers())
	m.startup = startupBodyOpen
	m.chooserEntry = m.captureChooserEntry()
	return m.beginBodyProbing()
}

// beginBodyProbing numbers the opening that just began and arms the rest timer
// for the body under the cursor.
func (m Model) beginBodyProbing() (Model, tea.Cmd) {
	m.bodyModalSeq++
	m.bodyModal.seq = m.bodyModalSeq
	return m.armProbeRest()
}

// applyObjectRegistryLoaded records the startup read. A failure is kept rather
// than discarded so the refusal can name it.
func applyObjectRegistryLoaded(m Model, msg ObjectRegistryLoadedMsg) (Model, tea.Cmd) {
	pending := m.bodyOpenPending
	m.bodyOpenPending = false

	if msg.Err != nil {
		m.objectRegErr = msg.Err
		return m.logNote("bodies", "registry unreadable (%v)", msg.Err), nil
	}
	m.objectRegistry = msg.Registry

	// Someone pressed ⌃b before the read landed. Honour it now — but only if
	// the app is still in a state that can hold a modal, since opening
	// mid-startup would be overwritten by the frame commit.
	if pending && m.startup == startupReady {
		m.bodyModal = openBodyModal(m.objectRegistry, m.lens.Objects, m.lens.VisibleObjectNumbers())
		m.startup = startupBodyOpen
		m.chooserEntry = m.captureChooserEntry()
		return m.beginBodyProbing()
	}
	return m, nil
}

// chooserEntry is what the screen showed when a chooser opened: the frame
// its panels drew and the Bodies cursor a preview may have clamped. Under
// chooserCloseKeeps = false, Esc puts both back (the specification C7a);
// by default nothing is put back and this is only read for the lens.
type chooserEntry struct {
	chart      chartViewState
	bodyCursor int
	// lens is the committed lens the chooser opened over. The chooser's live
	// preview rewrites m.lens; the Details timeline follows this one until the
	// operator confirms (astrolog-calls-honour-choosers C15).
	lens lookup.Lens
}

func (m Model) captureChooserEntry() chooserEntry {
	return chooserEntry{chart: m.chart, bodyCursor: m.bodyCursorIdx, lens: m.lens}
}

// confirmChooser is Enter (and, by default, Esc) on either chooser: the new policy is already in
// m.lens and the modal is closed. It fences the text surface exactly as before
// and lets the timeline pick up the committed lens, running its searches only
// when the confirmed policy changes what they carry.
func (m Model) confirmChooser() (Model, tea.Cmd) {
	m, textCmd := m.bumpIdentity(surfaceText)
	m, timelineCmd := m.syncTimeline()
	return m, tea.Batch(textCmd, timelineCmd)
}

// restoreChooserEntry (the switched-off Esc) puts the entry frame back at once and recomputes the
// entry policy behind it. bumpIdentity fences the text surface, so a preview
// already computing, or already queued, can no longer paint; the recompute
// draws the same frame again and brings Details, which a preview's commit may
// have refreshed, back in line with it.
func (m Model) restoreChooserEntry() (Model, tea.Cmd) {
	m.chart = m.chooserEntry.chart
	m.bodyCursorIdx = m.chooserEntry.bodyCursor
	return m.bumpIdentity(surfaceText)
}
