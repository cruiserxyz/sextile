package ui

import (
	"context"
	"errors"
	"fmt"
	"github.com/charmbracelet/bubbles/key"
	"github.com/cruiserxyz/sextile/internal/ui/dialog"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
)

// body_modal_model.go is the body selector's Model glue: the probe timers, the
// key and registry handlers and the chooser entry. The popup itself is in
// popup/body.go.

// probeTarget is what a probe would ask about: the focused slot's moment and
// place and its effective frame, resolved the way the chart compute resolves
// them (buildTextCmd). ok is false when there is nothing to ask — no
// workspace, a zone that is not known, or a house system that could not be
// read — and then no call is made and nothing is marked.
func (m Model) probeTarget() (in popup.ProbeInputs, when time.Time, place location.PlaceInfo, ok bool) {
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
	in = popup.ProbeInputs{
		When: when.UnixNano(), Lat: place.Lat, Lon: place.Lon, TZ: place.TZ,
		Houses: lens.Houses, Zodiac: lens.Zodiac, TrueNode: lens.Objects.TrueNodeMode,
	}
	return in, when, place, true
}

// armProbeRest is called when the object under the cursor may have changed. It
// ends the probe in flight, makes every earlier rest timer stale and, for a
// body that can be probed, arms one for the new row. The returned Cmd is only
// a timer: nothing is spawned until the cursor has rested.
func (m Model) armProbeRest() (Model, tea.Cmd) {
	m.bodyModal.StopProbe()
	return m.armRest()
}

// restartProbeRest is called for a key that left the same body under the
// cursor: the cursor has not rested for 150 ms if a key arrived, so a rest
// still pending starts over. A body with a current verdict, or whose probe is
// already running, is left alone — the running probe is not cancelled.
func (m Model) restartProbeRest() (Model, tea.Cmd) {
	cur, ok := m.bodyModal.ObjectUnderCursor()
	if !ok {
		return m, nil
	}
	if m.bodyModal.InflightOn && m.bodyModal.Inflight == cur.Number {
		return m, nil
	}
	in, _, _, resolved := m.probeTarget()
	if !resolved {
		return m, nil
	}
	if e, seen := m.bodyModal.Probes[cur.Number]; seen && e.Inputs == in {
		return m, nil
	}
	return m.armRest()
}

// armRest makes every earlier rest timer stale and, for a body that can be
// probed, arms one for the row under the cursor.
func (m Model) armRest() (Model, tea.Cmd) {
	m.bodyModal.RestID++
	in, _, _, ok := m.probeTarget()
	if ok {
		m.bodyModal.Inputs = in
	}
	cur, curOK := m.bodyModal.ObjectUnderCursor()
	if !ok || !curOK || (cur.Number >= recipe.HouseWheelObjectFirst && cur.Number <= recipe.HouseWheelObjectLast) {
		return m, nil
	}
	seq, restID, obj := m.bodyModal.Seq, m.bodyModal.RestID, cur.Number
	return m, tea.Tick(probeRest, func(time.Time) tea.Msg {
		return bodyProbeRestMsg{seq: seq, restID: restID, obj: obj}
	})
}

// applyBodyProbeRest starts the probe for the body the cursor has rested on.
func applyBodyProbeRest(m Model, msg bodyProbeRestMsg) (Model, tea.Cmd) {
	b := m.bodyModal
	if m.startup != startupBodyOpen || msg.seq != b.Seq || msg.restID != b.RestID {
		return m, nil
	}
	if cur, ok := b.ObjectUnderCursor(); !ok || cur.Number != msg.obj {
		return m, nil
	}
	in, when, place, ok := m.probeTarget()
	if !ok {
		return m, nil
	}
	m.bodyModal.Inputs = in
	if e, seen := m.bodyModal.Probes[msg.obj]; seen && e.Inputs == in {
		return m, nil
	}
	if _, absent := m.bodyModal.Absent[msg.obj]; absent {
		return m, nil // the startup probe already named its file
	}
	m.bodyModal.StopProbe()
	ctx, cancel := context.WithCancel(context.Background())
	m.bodyModal.Cancel = cancel
	m.bodyModal.Inflight, m.bodyModal.InflightOn = msg.obj, true
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
	if m.startup != startupBodyOpen || msg.seq != m.bodyModal.Seq {
		return m, nil
	}
	if m.bodyModal.InflightOn && m.bodyModal.Inflight == msg.obj {
		m.bodyModal.InflightOn = false
	}
	if msg.err != nil {
		// A probe that failed because it was cancelled is silent, and so is one
		// that finished before the cursor moved on but was delivered after:
		// the rest it started under is no longer the current one.
		if msg.cancelled || errors.Is(msg.err, context.Canceled) || msg.restID != m.bodyModal.RestID {
			return m, nil
		}
		// The body's name and nothing else: the error carries Astrolog's
		// stderr and the command, which hold the client's chart details.
		return m.logTrace("bodies", "couldn't check whether %s can be drawn", m.bodyName(msg.obj)), nil
	}
	m.bodyModal.SetProbe(msg.obj, msg.inputs, msg.probe)
	return m, nil
}

// bodyName is the registry's name for an object number.
func (m Model) bodyName(obj int) string {
	for _, o := range m.bodyModal.Registry {
		if o.Number == obj {
			return o.Name
		}
	}
	return fmt.Sprintf("object %d", obj)
}

// applyBodyModalKey is the Model-level half of the selector: it runs one
// keystroke through the modal and, when the operator finishes, puts the
// resulting policy in force and recomputes.
//
// Nothing here starts a process except the confirm branch and the debounced
// preview — opening, filtering and moving all return a nil Cmd (C7).
func applyBodyModalKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// SNAPSHOT, not a struct copy. popup.Body holds `selected` as a map,
	// so a copied struct shares that map with the original and any comparison
	// afterwards reports "unchanged" no matter what the keystroke did.
	beforeTicks := ticksOn(m.bodyModal.Selected)
	beforeObj, beforeOK := m.bodyModal.ObjectUnderCursor()
	beforeFilter := m.bodyModal.FilterText
	next, out := updateBodyModalKey(m.bodyModal, msg, popup.ListHeight(m.height))
	m.bodyModal = next

	switch {
	case out.Cancelled:
		m.bodyModal.StopProbe()
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
		m.bodyModal.StopProbe()
		m.startup = startupReady
		if out.Dismissed && sameObjectSelection(m.lens.Objects, m.chooserEntry.lens.Objects) {
			// Esc with nothing ticked changes no lens and starts no
			// work (C9).
			return m, nil
		}
		m = m.confirmedObjects(out.Policy)
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
	if cur, ok := next.ObjectUnderCursor(); ok != beforeOK || (ok && cur.Number != beforeObj.Number) {
		m, probeCmd = m.armProbeRest()
	} else if keys.MatchBinding(msg, bodiesBindings()) != keys.BindingNone || next.FilterText != beforeFilter {
		// The same body is still under the cursor: a key the popup acts on
		// restarts a rest that is still pending. A key it ignores (Ctrl+R, say)
		// is not a key press as far as the popup is concerned.
		m, probeCmd = m.restartProbeRest()
	}
	if !sameTickSets(beforeTicks, ticksOn(next.Selected)) {
		m = m.chosenObjects(next.Policy())
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
	// Not before the first frame (a modal opened then is overwritten by its
	// commit), not after a failed read, and not while the drawn set cannot be
	// read (the Log says why): blocked answers all three.
	switch m.blocked(keys.BindingBodies) {
	case "":
	case whyDrawnUnread:
		_, err := m.lens.VisibleObjectNumbers()
		return m.logNote("bodies", "the body list cannot open: %v", err), nil
	default:
		return m, nil
	}
	if m.objectRegistry == nil {
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
	m.bodyModal = m.openedBodyModal()
	m.startup = startupBodyOpen
	m.chooserEntry = m.captureChooserEntry()
	return m.beginBodyProbing()
}

// openedBodyModal is the chooser as it opens. The ticks are what the chart
// draws (blocked has answered that they can be read), the rows with no
// ephemeris file are the startup probe's answer, and Ctrl+D ticks what the lens
// draws when nothing is chosen (defaultObjects, as this host draws it).
func (m Model) openedBodyModal() popup.Body {
	drawn, _ := m.lens.VisibleObjectNumbers()
	b := popup.Open(m.objectRegistry, m.lens.Objects, drawn)
	b.Defaults = m.defaultDrawn()
	b.SetAbsent(m.ephemeris.Absent, m.ephemeris.Said)
	return b
}

// beginBodyProbing numbers the opening that just began and arms the rest timer
// for the body under the cursor.
func (m Model) beginBodyProbing() (Model, tea.Cmd) {
	m.bodyModalSeq++
	m.bodyModal.Seq = m.bodyModalSeq
	return m.armProbeRest()
}

// applyObjectRegistryLoaded records the startup read. A failure is kept rather
// than discarded so the refusal can name it.
func applyObjectRegistryLoaded(m Model, msg ObjectRegistryLoadedMsg) (Model, tea.Cmd) {
	pending := m.bodyOpenPending
	m.bodyOpenPending = false
	awaiting := m.awaitingRegistry
	m.awaitingRegistry = false

	if msg.Err != nil {
		m.objectRegErr = msg.Err
		m.cantStart.objectList = oneLine(msg.Err.Error())
		return m, nil
	}
	m.objectRegistry = msg.Registry
	m.lens.Registry = msg.Registry
	if awaiting {
		return maybeResolveStartup(m)
	}
	// A saved drawn set that waited for the list resolves now.
	var resolveCmd tea.Cmd
	if m.drawnRaw != nil {
		m = m.restoreDrawn(m.drawnRaw)
		if m.slots != nil {
			m, resolveCmd = m.bumpIdentity(surfaceText)
		}
	}

	// Someone pressed ⌃b before the read landed. Honour it now — but only if
	// the app is still in a state that can hold a modal, since opening
	// mid-startup would be overwritten by the frame commit.
	if pending {
		switch m.blocked(keys.BindingBodies) {
		case "":
			m.bodyModal = m.openedBodyModal()
			m.startup = startupBodyOpen
			m.chooserEntry = m.captureChooserEntry()
			var probe tea.Cmd
			m, probe = m.beginBodyProbing()
			return m, tea.Batch(resolveCmd, probe)
		case whyDrawnUnread:
			_, err := m.lens.VisibleObjectNumbers()
			return m.logNote("bodies", "the body list cannot open: %v", err), resolveCmd
		}
	}
	return m, resolveCmd
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
// m.lens and the modal is closed. It fences the text surface exactly as before,
// makes Details wait for that text and ask for its report again (the report is
// cast in the same lens, as recastHouses does for a house change), and lets the
// timeline pick up the committed lens, running its searches only when the
// confirmed policy changes what they carry.
func (m Model) confirmChooser() (Model, tea.Cmd) {
	m = m.waitDetailsForText()
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
	inputs    popup.ProbeInputs
	probe     lookup.BodyProbe
	err       error
	cancelled bool
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
		return sameObjectSet(a.Add, b.Add) && sameObjectSet(a.Remove, b.Remove)
	}
	return true
}

func sameObjectSet(x, y recipe.ObjectSet) bool {
	return slices.Equal(x.Members(), y.Members())
}

// chooserCloseBindings is the closing half of the Aspects and Bodies tables.
// Under chooserCloseKeeps (the 2026-10-22 trial) Enter and Esc are one entry,
// Close, since both leave the ticks in force; with the switch off it is the
// old Apply and Cancel pair. q stays a letter: both choosers take filter text.
func chooserCloseBindings() []keys.BindingEntry {
	if chooserCloseKeeps {
		return []keys.BindingEntry{{ID: keys.BindingClose, Binding: key.NewBinding(key.WithKeys("enter", "esc")), Title: "Close", BarKey: "enter/esc", Bar: true, Pinned: true}}
	}
	return []keys.BindingEntry{confirm("Apply"), closeOn("Cancel")}
}

// bodiesBindings is the Bodies popup's table.
func bodiesBindings() []keys.BindingEntry {
	return keys.Titled(append(append([]keys.BindingEntry{
		listUp().ShownWhen(func(c keys.Context) bool { return c.Popup.Rows > 1 }), listDown(), toggle().ShownWhen(func(c keys.Context) bool { return c.Popup.Rows > 0 }),
		// Ctrl+D, not d: the search line takes every typed letter.
		{ID: keys.BindingDefaults, Binding: key.NewBinding(key.WithKeys("ctrl+d")), Title: "Defaults", BarKey: "⌃d", Bar: true},
	}, chooserCloseBindings()...), pageEntries()...))
}

// updateBodyModalKey resolves one keystroke through the bodies bindings table
// to the popup's Action and applies it. The popup never reads the table.
func updateBodyModalKey(state popup.Body, msg tea.KeyMsg, listHeight int) (popup.Body, popup.Outcome) {
	a := popup.Type
	switch keys.MatchBinding(msg, bodiesBindings()) {
	case keys.BindingConfirm:
		a = popup.Confirm
	case keys.BindingClose:
		switch {
		case !chooserCloseKeeps:
			a = popup.Discard
		case msg.Type == tea.KeyEsc:
			a = popup.Dismiss
		default:
			a = popup.Keep
		}
	case keys.BindingListUp:
		a = popup.Up
	case keys.BindingListDown:
		a = popup.Down
	case keys.BindingPageUp:
		a = popup.PageUp
	case keys.BindingPageDown:
		a = popup.PageDown
	case keys.BindingPageHome:
		a = popup.PageHome
	case keys.BindingPageEnd:
		a = popup.PageEnd
	case keys.BindingToggle:
		a = popup.Toggle
	case keys.BindingDefaults:
		a = popup.Defaults
	}
	out := state.Apply(a, msg, listHeight)
	return state, out
}

// renderBodyModal draws the selector with the chrome and the position counter
// the popups share.
func renderBodyModal(state popup.Body, cfg Config, termW, termH int) string {
	return state.Render(cfg, termW, termH, popupChrome(cfg), func(cursor, total int) *dialog.Counter {
		pos := listPosition{Cursor: cursor, Total: total}
		return popupCounter(&pos, cfg)
	})
}
