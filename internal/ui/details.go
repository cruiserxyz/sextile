package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/pane"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

type detailsViewState struct {
	title string
	body  string
	// key is the key the report tab's title and body were computed for, stored
	// WITH them in one assignment (adoptDetailsReport); keyed says there is
	// one. The screen draws the body only for the subject the caption names
	// (the view rule, subject.go).
	key   contentKey
	keyed bool
	// runKey is the key of the report run in flight (set when it starts); the
	// reconciler asks for no second run for a key that has one.
	runKey detailsKey
	// failed is the wanted report key whose accepted run most recently failed.
	// It stands while the wanted key is that key; a wanted key that changes
	// leaves it behind, and a success for any key clears it.
	failed struct {
		set bool
		key detailsKey
	}
	pending     bool
	waitForText bool
	timerArmed  bool
	// scrollOffset is the first report line the panel shows. Ephemeral
	// presentation state: never written to store, cleared with the struct by
	// suspendDetails, and re-clamped at render time.
	scrollOffset int
	// query is the report tab's own `/` query (the specification):
	// kept across refreshes of the same title, cleared when the title changes.
	// searching is its line open. Neither is the timeline's or the Aspects
	// panel's.
	query     string
	searching bool

	// Everything below is presentation state too, never written to store.
	//
	// tabs is the stored CHOICE of Details' tab per focused panel (only the
	// Chart panel offers a second tab, the timeline); the list itself is
	// derived (panelTabs). A panel with no entry is on its first tab. Replaced
	// whole on write, like bodyRows (the specification).
	tabs map[PanelID]TabID
	// preEnter is the Chart's Details tab as it was when Enter attached Details
	// from the Chart panel; every way out of attached Details puts it back.
	// preEnterSet tells "report" (the empty choice) from "nothing remembered".
	preEnter    TabID
	preEnterSet bool
	// attached is whether Details is open and row navigation is inside it
	// (D12). Enter attaches and Esc detaches; it is a flag and never a focus
	// change, because Details is not in the focus ring (C15, C40). Open, it
	// stands beside the panels or right below the panel in focus (layoutFor).
	// While it is set, Details owns the keyboard (D18, updateDetailsKey).
	//
	// Details' SUBJECT — the (panel, slot) it reports on — is not stored:
	// it is (m.focused, m.cursorIdx), see detailsSubject. m.focused is never
	// Details. A focus key moves the focus and Details with it, so the subject
	// is always the panel Details stands beside or below; the slot cursor is
	// Details' own while attached.
	attached bool
	// cursor is the timeline row under the row cursor; rowOffset is the
	// first timeline row the panel shows. Both are re-clamped against the
	// rows and the geometry on every change.
	cursor, rowOffset int
	// cursorAt names the event under the cursor, so a list reloaded for a new
	// month can put the cursor back on it (identity, never index).
	cursorAt  timelineMark
	cursorSet bool
	// pressedAt is the row Enter last jumped to (the specification
	// rule 9): it stays the bold row while the floating moment equals its
	// instant, and any step or cursor move clears it.
	pressedAt  timelineMark
	pressedSet bool
	// followNow says a navigation key moved Now (a step key, p/P, n) since the
	// operator last placed the row cursor: an attached list tab then puts the
	// cursor on the nearest row, scrolling only when that row is out of sight.
	// Enter on a row, and any cursor, page or scroll key, end it.
	followNow bool
	// pressedChart is the slot whose numeral the pressed row carried, resolved
	// at the press: numerals shift when the set changes, a slot does not. Nil
	// for a sky row.
	pressedChart *store.Slot
	// stale is whether the subject or the floating moment changed while the
	// report tab was hidden and no report was requested for it
	// (the specification rule 2): the report is caught up
	// when its tab is chosen.
	stale bool
}

// detailsSubject is what Details reports on: the focused ring panel and the
// focused slot. It follows the ring (tab, ←/→, 1-4) and the slot cursor,
// attached or not, and attaching or detaching changes neither (C31). The console's priorFocus
// (console.go) is the same shape for the log console.
type detailsSubject struct {
	panel PanelID
	slot  int
}

func (m Model) detailsSubject() detailsSubject {
	return detailsSubject{panel: m.focused, slot: m.cursorIdx}
}

// detailsInnerHeight is how many report lines the Details panel body can show,
// or 0 when Details is not on screen.
func (m Model) detailsInnerHeight() int {
	r, ok := m.geometry()[PanelDetails]
	if !ok {
		return 0
	}
	return r.h - 2
}

// clampDetailsOffset holds the STORED offset inside what the current geometry
// can scroll. renderDetailsPanel clamps what it DRAWS (C18) without mutating
// the Model, so a resize that makes the report fit would otherwise leave a
// stale offset behind: the next scroll key would jump to 0 instead of
// stepping, and until then the stored and drawn positions would disagree.
// Called on every resize, so a stale offset is unreachable.
func (m Model) clampDetailsOffset() Model {
	visible := m.detailsInnerHeight()
	if visible <= 0 {
		return m
	}
	total, height := m.detailsScrollSpan()
	m.details.scrollOffset = pane.ClampOffset(m.details.scrollOffset, total, height)
	return m
}

// detailsScrollSpan is how many lines the active Details tab scrolls over and
// how many of them it shows: the template tab's own laid-out lines at the
// panel's full height, the report (under its query line) otherwise. Scrolling,
// the scroll condition and the resize clamp all ask it, so none of them measures
// one tab against the other's text.
func (m Model) detailsScrollSpan() (total, height int) {
	if isTemplateTab(m.activeTabID(PanelDetails)) {
		r, ok := m.geometry()[PanelDetails]
		if !ok {
			return 0, 0
		}
		return len(panel.TemplateBodyLines(m.tpl.lines, r.w-2, m.cfg)), r.h - 2
	}
	return len(m.detailsView().Lines), m.detailsReportHeight()
}

// scrollDetails moves the report by delta lines, clamped to what it can show.
// Presentation only: no Astrolog process starts, no refresh surface is
// touched, and the visible title and body are left exactly as they are
// .
func (m Model) scrollDetails(delta int) Model {
	visible := m.detailsInnerHeight()
	if visible <= 0 {
		return m
	}
	total, height := m.detailsScrollSpan()
	m.details.scrollOffset = pane.ClampOffset(m.details.scrollOffset+delta, total, height)
	return m
}

// adoptDetailsReport replaces the visible frame with a completed report and its
// key, in one assignment, and decides what happens to the scroll offset: the
// same subject is the same context refreshed under the operator's eyes, so
// their position is kept (and re-clamped to the new text); a different subject
// is a new report, read from the top.
func (m Model) adoptDetailsReport(title, body string, key contentKey) Model {
	same := m.details.keyed && m.details.key.subject() == key.subject()
	prev := m.details.scrollOffset
	m.details.title, m.details.body, m.details.scrollOffset = title, body, 0
	m.details.key, m.details.keyed = key, true
	m.details.failed.set = false
	if !same {
		// A new subject: the query was about the old one (the specification 7).
		m.details.query, m.details.searching = "", false
		return m
	}
	v := m.detailsViewOf(body)
	height := m.detailsInnerHeight()
	if height > 0 && v.Line {
		height--
	}
	m.details.scrollOffset = pane.ClampOffset(prev, len(v.Lines), height)
	return m
}

type detailsRequest struct {
	title  string
	report lookup.RawReportRequest
	when   time.Time
	place  location.PlaceInfo
	// cross marks a body report that must be computed over the comparison
	// PAIR instead of one slot. natalChart is the anchored chart;
	// when/place then carry the committed transit moment, so the report
	// describes exactly the relationship the Aspects panel is showing.
	cross      bool
	natalChart store.AnchorChart
	// synastry marks the cross report as the pair of two ANCHORED charts
	// (Astrolog -r0): when/place then carry the other chart's moment and place.
	synastry bool
	// otherLabel is that other chart's name and place for its report header.
	otherLabel store.ReportLabel
	// other is that other chart as an anchor, record included.
	other store.AnchorChart
	// cursorNumeral is the cursor chart's numeral as the pair tab spells it
	// (② or 2); the Log line that says the report was filtered names it.
	cursorNumeral string
}

// detailsVisible is the work gate, not merely a render check: no Details
// request may start unless the layout actually places the panel.
func (m Model) detailsVisible() bool {
	_, ok := m.geometry()[PanelDetails]
	return ok
}

// detailsRequestFor snapshots the report source currently visible to the
// operator for the contexts that need their own Astrolog report (Chart, a
// body). It uses committed slot times, so raw Details never races ahead of
// the Chart/Bodies frame it explains. The Aspects panel has no request: its
// Details is the committed frame's own report (aspectsDetailsFromFrame).
func (m Model) detailsRequestFor() (detailsRequest, bool) {
	if !m.detailsVisible() {
		return detailsRequest{}, false
	}
	slot := m.focusedSlot()
	if slot == nil {
		return detailsRequest{}, false
	}
	req := detailsRequest{when: slot.Time, place: slot.Location}
	// The report names the chart it is about: the focused slot's label, empty
	// for the floating slot (the specification C1, C6).
	req.report.Label = slot.Chart.Label
	req.report.Record = slot.Chart.Record
	switch m.focused {
	case PanelChart:
		req.title = detailsPrefix + tabTitleChart
		req.report.Kind = lookup.RawReportChart
	case PanelBodies:
		title, object, ok := m.detailsBodyTitle()
		if !ok {
			return detailsRequest{}, false
		}
		req.report.Kind = lookup.RawReportBody
		req.report.Object = object
		req.title = title
		// While comparing, a body's report follows the Aspects panel's active
		// pair (C4): its cross rows against the pair's other chart, in the
		// relation the pair has — transit for the floating slot,
		// synastry for another anchored one. Single, it is the cursor's own
		// chart alone. The other side is the slot's COMMITTED Time/Location,
		// the same committed state every other Details request reads, and the
		// same place the frame computed at.
		if sc := m.deriveScene(); sc.pair != nil {
			pair := sc.pair
			req.cross = true
			req.natalChart = anchorOf(pair.left)
			req.when, req.place = pair.right.Time, pair.right.Location
			req.synastry = !pair.right.Floating
			req.otherLabel = pair.right.Chart.Label
			req.other = anchorOf(pair.right)
			req.cursorNumeral = panel.NumeralForColour(sc.numeral(pair.right), m.cfg)
		}
	default:
		return detailsRequest{}, false
	}
	return req, true
}

// detailsBodyTitle is the title of the Bodies row under the cursor, in the
// frame Bodies paints (shownChart), with the active pair appended while
// comparing. It is the ONE computation behind both the next body report's
// title (detailsRequestFor) and the caption shown before that report commits
// (detailsCaption).
func (m Model) detailsBodyTitle() (title string, o aspect.Object, ok bool) {
	ch := m.shownChart()
	o, ok = panel.ObjectAtCursor(ch.positions, ch.cusps, m.bodyCursorIdx)
	if !ok {
		return "", aspect.Object{}, false
	}
	title = detailsPrefix + panel.BodyName(o)
	if word := nodeFormTitle(o, m.detailsLens(detailsRequest{cross: m.deriveScene().pair != nil}).Objects.Nodes); word != "" {
		title = detailsPrefix + word
	}
	if sc := m.deriveScene(); sc.pair != nil {
		title += " " + m.pairTabTitle(sc.numeral(sc.pair.left), sc.numeral(sc.pair.right))
	}
	return title, o, true
}

// aspectsDetailsFromFrame is the Aspects panel's Details: the committed
// frame's own report for whichever content the panel is showing — the focused
// slot's -aa list on Aspects, the pair's -rt -aa or -r0 -aa list while
// comparing — exactly the text the waterfall was parsed from, so both panels
// always describe the same Astrolog run. Both read the SAME derived tab id as
// the strip and the renderer, so the Details title always names what
// the Aspects panel is actually showing. ok is false when the frame carries no
// report yet (nothing committed, or an error frame), so the caller keeps the
// last complete Details frame in place. key is the frame the copy was taken
// from: the stamp of the committed frame, never the key the screen asks for now.
func (m Model) aspectsDetailsFromFrame() (title, body string, key contentKey, ok bool) {
	pk, stamped := m.shownPaneKey(PanelAspects)
	if !stamped {
		pk, stamped = m.wantedPaneKey(PanelAspects)
	}
	key = contentKey{kind: reportFrame, frame: pk}
	if !stamped {
		return "", "", key, false
	}
	if active := m.activeTabID(PanelAspects); isAspectsPairTab(active) {
		title := ""
		for _, tab := range m.panelTabs(PanelAspects) {
			if tab.ID == active {
				title = tab.Title
			}
		}
		return detailsPrefix + title, m.chart.crossAspectsReport, key, m.chart.crossAspectsReport != ""
	}
	return detailsPrefix + tabTitleAspects, m.chart.aspectsReport, key, m.chart.aspectsReport != ""
}

// applyAspectsDetails fills Details from the frame with no Astrolog work
// : it cancels and fences any in-flight report — a Chart or body
// report left over from the previous focus must not paint over this — clears
// the pending state, and replaces title, body and key atomically, or leaves the
// last complete frame untouched when the frame has no report to give (the view
// rule hides it when it is another subject's).
func (m Model) applyAspectsDetails() Model {
	if !m.detailsVisible() {
		return m
	}
	m = m.suspendSurface(surfaceDetails)
	m.details.pending = false
	m.details.waitForText = false
	m.details.timerArmed = false
	if title, body, key, ok := m.aspectsDetailsFromFrame(); ok {
		m = m.adoptDetailsReport(title, body, key)
	}
	return m
}

// requestDetails starts one report immediately for a context that is already
// settled (initial/current text commit or a wide resize). It deliberately does
// not clear or retitle the panel: the completed title+body replaces the last
// successful frame atomically in commitRefresh.
func (m Model) requestDetails() (Model, tea.Cmd) {
	if !m.detailsVisible() {
		return m, nil
	}
	// Rule 1 of the specification: only the report tab
	// asks for a report. The change is remembered for the catch-up.
	if m.reportTabHidden() {
		m.details.stale = true
		return m, nil
	}
	m.details.stale = false // a request for the current subject settles the remembered change
	if m.focused == PanelAspects {
		return m.applyAspectsDetails(), nil
	}
	_, ok := m.detailsRequestFor()
	if !ok {
		m.details.pending = false
		m.details.waitForText = false
		return m.suspendSurface(surfaceDetails), nil
	}
	return m.startDetailsNow()
}

// startDetailsNow is the settled-context analogue of bumpIdentity without its
// defensive trailing retry timer. detailsRequestFor already proved this surface
// runnable, so one immediate run is sufficient and avoids an otherwise useless
// post-commit timer message.
func (m Model) startDetailsNow() (Model, tea.Cmd) {
	// A report already computed for this key is drawn now, from memory.
	if nm, ok := m.adoptReportFromMemory(); ok {
		return nm, nil
	}
	if c := m.refresh[surfaceDetails].cancel; c != nil {
		c()
	}
	m.refresh[surfaceDetails].cancel = nil
	m.details.pending = true
	m.details.timerArmed = false
	m.refresh[surfaceDetails].wantID++
	m.refresh[surfaceDetails].commitFloor = m.refresh[surfaceDetails].wantID
	m.refresh[surfaceDetails].timerID = m.refresh[surfaceDetails].wantID
	return m.startRun(surfaceDetails)
}

// deferDetails is deferDetailsWith for a repeat key: the debounce rests.
func (m Model) deferDetails() (Model, tea.Cmd) { return m.deferDetailsWith(false) }

// deferDetailsWith handles contexts that change without a chart-text recompute
// (Bodies cursor and panel focus). Every key cancels/fences old work and records
// a new desired context, while at most one trailing timer remains armed; no
// Astrolog process starts until navigation settles, except for atOnce, the first
// key of a run of equal keys, which starts the report now. The last successful
// title and body remain untouched throughout the burst, so Details has no
// visible intermediate state to repaint.
func (m Model) deferDetailsWith(atOnce bool) (Model, tea.Cmd) {
	if !m.detailsVisible() {
		return m, nil
	}
	if m.reportTabHidden() {
		m.details.stale = true
		m.keyRun.seq++ // the timeline or template tab now awaits its content
		return m, nil
	}
	m.details.stale = false // a request for the current subject settles the remembered change
	// A focus change can arrive while a slot/time/comparison text frame is
	// still settling. Do not let its raw report lead the visible
	// Bodies/Aspects frame; the final text commit will start Details.
	if m.refresh[surfaceText].wantID != m.refresh[surfaceText].lastCommitWantID {
		m = m.markDetailsPending(true)
		return m.suspendSurface(surfaceDetails), nil
	}
	// Aspects reads the committed frame directly: no timer to arm.
	if m.focused == PanelAspects {
		return m.applyAspectsDetails(), nil
	}
	if _, ok := m.detailsRequestFor(); !ok {
		m = m.markDetailsPending(true)
		return m.suspendSurface(surfaceDetails), nil
	}
	m = m.markDetailsPending(false)
	// The first key of a run of equal keys starts the report at once; only a
	// repeat inside the run rests for the debounce (specs/details-follows-its-tab.md C14).
	if atOnce {
		return m.startDetailsNow()
	}
	if c := m.refresh[surfaceDetails].cancel; c != nil {
		c()
	}
	m.refresh[surfaceDetails].cancel = nil
	m.refresh[surfaceDetails].wantID++
	m.refresh[surfaceDetails].runningRunID++
	m.refresh[surfaceDetails].runningWantID = m.refresh[surfaceDetails].wantID
	m.refresh[surfaceDetails].commitFloor = m.refresh[surfaceDetails].wantID
	if m.details.timerArmed {
		return m, nil
	}
	m.refresh[surfaceDetails].timerID = m.refresh[surfaceDetails].wantID
	m.details.timerArmed = true
	return m, m.armRefresh(surfaceDetails, m.refresh[surfaceDetails].timerID)
}

// waitDetailsForText handles time, slot, and comparison navigation whose raw
// report must match a newly computed chart frame. It cancels/fences Details but
// arms no report timer; the latest (not intermediate) text commit starts the
// single final report.
func (m Model) waitDetailsForText() Model {
	if !m.detailsVisible() {
		return m
	}
	// No pending mark and no fence while the report tab is hidden; the
	// catch-up asks for the report when the report tab is chosen.
	if m.reportTabHidden() {
		m.details.stale = true
		m.keyRun.seq++ // the timeline or template tab now awaits its content
		return m
	}
	m.details.stale = false
	m = m.markDetailsPending(true)
	return m.suspendSurface(surfaceDetails)
}

// reportTabHidden is the one guard of the report's requests (rule 2 of
// the specification): the report is asked for only while
// the report tab is the active Details tab, attached or not.
func (m Model) reportTabHidden() bool {
	return m.activeTabID(PanelDetails) != TabDetailsReport
}

// catchUpDetails is the report's catch-up: the report for the current subject
// is requested now when something changed while the report tab was hidden, and
// not otherwise. It runs when the report tab is chosen or becomes the active
// one again; a text frame still computing makes it wait for that frame like
// any other change. While the report tab is still hidden it asks for nothing
// and keeps the change remembered.
func (m Model) catchUpDetails() (Model, tea.Cmd) {
	if !m.details.stale {
		return m, nil
	}
	m.details.stale = false
	if m.refresh[surfaceText].wantID != m.refresh[surfaceText].lastCommitWantID {
		return m.waitDetailsForText(), nil
	}
	return m.requestDetails()
}

// A held navigation key is a run of the same key: a terminal reports no key
// release, so one press, the terminal's delay before the first repeat (up to
// 500 ms on Windows and GNOME), then repeats every 30-50 ms. The run is
// tracked and nothing else: since D1 a held key no longer turns a pane faint
// (specs/details-follows-its-tab.md C8) — what a pane draws follows its keys
// alone (the view rule, subject.go). The second key of a run, within
// holdStartWindow of the first, joins it; any other key ends it.
const holdStartWindow = 650 * time.Millisecond

// detailsRunState is the current run of equal keys. seq counts the changes
// that leave a Details tab awaiting content (markDetailsPending, and a hidden
// report tab's timeline or template); Update compares it across a key. key and
// keys are the run's key and how many keys it has had; last is when the newest
// arrived.
type detailsRunState struct {
	seq  uint64
	key  string
	keys int
	last time.Time
}

// keyJoinsRun says k is a repeat of the run's key inside holdStartWindow of the
// run's newest key: a key that is not has no run to join and starts one.
func (m Model) keyJoinsRun(k tea.KeyMsg) bool {
	h := m.keyRun
	return h.keys > 0 && k.String() == h.key && m.clock.Now().Sub(h.last) <= holdStartWindow
}

// followKeyRun judges one key for the run. changed says the key changed what
// Details describes; only such a key joins or starts a run.
func (m Model) followKeyRun(k tea.KeyMsg, changed bool) Model {
	h := m.keyRun
	now := m.clock.Now()
	sameRun := m.keyJoinsRun(k)
	if !sameRun && (k.String() != h.key || changed) {
		h.key, h.keys = "", 0
	}
	if changed {
		if sameRun {
			h.keys++
		} else {
			h.key, h.keys = k.String(), 1
		}
		h.last = now
	}
	m.keyRun = h
	return m
}

func (m Model) markDetailsPending(waitForText bool) Model {
	m.keyRun.seq++
	m.details.pending = true
	m.details.waitForText = waitForText
	return m
}

// suspendDetails runs when Details stops being shown. The report frame goes
// with it, and so does the attachment: a keyboard owned by a panel nobody can
// see has no return destination (C52). The focused panel never moved when
// Details attached, so detaching here leaves focus on the subject panel. The
// tab choice survives, so the panel comes back on the tab it left on.
func (m Model) suspendDetails() Model {
	m = m.restorePreEnterTab()
	// A workspace identity is leaving (a session reset, a chart or city pick, a
	// chart closed) or Details is: nothing computed for it is drawn again.
	m = m.forgetMemory()
	m.details = detailsViewState{tabs: m.details.tabs, preEnter: m.details.preEnter, preEnterSet: m.details.preEnterSet}
	m = m.suspendTimeline()
	m = m.suspendSurface(surfaceTemplate)
	return m.suspendSurface(surfaceDetails)
}

// detailsLens is the lens a Details report is cast in: the chart's own report
// shows only the selected bodies, in its own lens; a pair report (cross or
// synastry) is a comparison, ignores the selection and is cast in the frame's
// lens. The spawn and the key read it alike.
func (m Model) detailsLens(req detailsRequest) lookup.Lens {
	if req.cross {
		return m.comparisonLens()
	}
	return m.showLens(m.focusedSlot())
}

// detailsKey is the question a Details report answers: every input that
// changes the argv, as plain comparable values. The search text, the scroll
// place and the tab choice are presentation and are not in it. The run, want
// and floor numbers stay their own fields beside it (refreshResultMsg).
type detailsKey struct {
	panel PanelID
	kind  lookup.RawReportKind
	// The cursor slot and its label.
	slot     int
	floating bool
	// chart, natalChart and otherChart are the identities of the cursor chart
	// and of the pair's two sides (slotIdentity), which the subject keeps.
	chart, natalChart, otherChart string
	slotWhen                      int64
	slotPlace                     location.PlaceInfo
	label                         store.ReportLabel
	object                        int
	// The pair: the other side's moment, place and label, and the title that
	// names the pair tab.
	cross, synastry bool
	natalWhen       int64
	natalPlace      location.PlaceInfo
	natalLabel      store.ReportLabel
	otherWhen       int64
	otherPlace      location.PlaceInfo
	otherLabel      store.ReportLabel
	title           string
	// The lens the spawn uses; only is Objects.Only, sorted and joined.
	houses  recipe.HouseSystem
	zodiac  recipe.Zodiac
	objects string
	aspects string
	nodes   recipe.NodeForm
	only    string
}

// detailsKeyFor is the key of the report the Model would ask for now; ok is
// false when there is no request. Built from detailsRequestFor and
// detailsLens, the values buildDetailsCmd spawns from, so the key a run
// carries and the key the commit gate compares cannot disagree.
func (m Model) detailsKeyFor() (detailsKey, bool) {
	req, ok := m.detailsRequestFor()
	if !ok {
		return detailsKey{}, false
	}
	slot := m.focusedSlot()
	lens := m.detailsLens(req)
	objects, aspects, nodes := chooserKey(lens)
	var only []string
	for _, n := range lens.Objects.Only.Members() {
		only = append(only, strconv.Itoa(n))
	}
	var natalID, otherID string
	if req.cross {
		if pair := m.deriveScene().pair; pair != nil {
			natalID, otherID = slotIdentity(pair.left), slotIdentity(pair.right)
		}
	}
	return detailsKey{
		panel: m.focused, kind: req.report.Kind,
		chart: slotIdentity(slot), natalChart: natalID, otherChart: otherID,
		slot: m.cursorIdx, floating: slot.Floating, slotWhen: slot.Time.UnixNano(), slotPlace: slot.Location,
		label: req.report.Label, object: req.report.Object.Number,
		cross: req.cross, synastry: req.synastry, natalWhen: req.natalChart.When.UnixNano(), natalPlace: req.natalChart.Place, natalLabel: req.natalChart.Label,
		otherWhen: req.when.UnixNano(), otherPlace: req.place, otherLabel: req.otherLabel,
		title:  req.title,
		houses: lens.Houses, zodiac: lens.Zodiac, objects: objects, aspects: aspects, nodes: nodes,
		only: strings.Join(only, ","),
	}, true
}

func buildDetailsCmd(m Model, ctx context.Context, runID, wantID uint64) tea.Cmd {
	req, ok := m.detailsRequestFor()
	if !ok {
		return nil
	}
	if m.unresolvedFor(m.focusedSlot()) != nil {
		return nil
	}
	// The chart's own report shows only the selected bodies, in its own lens;
	// a pair report (cross or synastry) is a comparison, ignores the selection
	// and is cast in the frame's lens.
	lens := m.detailsLens(req)
	key, _ := m.detailsKeyFor()
	reports := m.rows[surfaceDetails].spawn.reports
	return func() tea.Msg {
		// Exactly one spawn either way: the comparison body report is a
		// different Astrolog run, not an extra one. Its command rides on the
		// payload — on the error path too, whenever the spawn
		// happened; an empty command means no process started and nothing
		// is carried.
		raw, command, err := "", "", error(nil)
		if req.cross && req.synastry {
			raw, command, err = reports.synastry(ctx, lens, req.natalChart, req.other, req.report.Object)
		} else if req.cross {
			raw, command, err = reports.cross(ctx, lens, req.natalChart, req.when, req.place, req.report.Object)
		} else {
			raw, command, err = reports.raw(ctx, req.when, req.place, lens, req.report)
		}
		// A cross body report lists the body at either end; the Aspects tab
		// shows it only as the cursor chart's, so keep those rows. This
		// arranges Astrolog's text and computes nothing; the Log says so.
		note := ""
		if req.cross && err == nil {
			var kept, total int
			if raw, kept, total = lookup.KeepCursorSide(raw, req.report.Object); kept != total {
				note = fmt.Sprintf("kept the rows where %s is %s's: %d of %d rows, totals dropped", panel.BodyName(req.report.Object), req.cursorNumeral, kept, total)
			}
		}
		var commands []string
		title := req.title
		if command != "" {
			commands = []string{command}
			title = nodeTitleFromCall(title, req.report.Object, lookup.CommandNodeForm(command))
		}
		influence := !req.cross && req.report.Kind == lookup.RawReportChart
		var starsLeft []int
		if influence {
			starsLeft = lens.InfluenceLeavesOut()
		}
		return refreshResultMsg{
			surface: surfaceDetails,
			runID:   runID,
			wantID:  wantID,
			payload: detailsPayload{title: title, body: raw, commands: commands, note: note, key: key, influence: influence, starsLeft: starsLeft},
			err:     err,
		}
	}
}

// detailsScrollStep is how far one PgUp/PgDn press or one wheel notch moves
// the Details report: two lines, mirroring lazygit's `scrollHeight`.
const detailsScrollStep = 2

// newDetailsRow is the Details surface's refresh policy (specs/
// refresh-descriptor-details.md): the coordinator asks it through Model.rowFor.
// applyAspectsDetails, a synchronous adoption of the Aspects tab's text with no
// run, is the second commit path and stays outside the row until the Text
// surface has one.
func newDetailsRow() surfaceRow {
	return surfaceRow{
		inputs: func(m Model) (refreshKey, bool) {
			k, ok := m.detailsKeyFor()
			return k, ok
		},
		trigger:   triggerEvent,
		supersede: supersedeHard,
		debounce:  detailsDebounce,
		rest: restTimer{
			armed:  func(m Model) bool { return m.details.timerArmed },
			hidden: (Model).reportTabHidden,
			park: func(m Model) Model {
				m.details.timerArmed = false
				m.details.stale = true
				return m
			},
			disarm: func(m Model) Model {
				m.details.timerArmed = false
				return m
			},
		},
		build:   buildDetailsCmd,
		accept:  acceptDetails,
		commit:  (Model).commitDetails,
		suspend: func(m Model) Model { m.details.timerArmed = false; return m },
		started: func(m Model) Model {
			m.details.runKey, _ = m.detailsKeyFor()
			return m
		},
		spawn: rowSpawns{reports: detailsSpawns{
			raw:      lookup.ComputeRawReport,
			cross:    lookup.CrossBodyRawReport,
			synastry: lookup.SynastryCrossBodyRawReport,
		}},
	}
}

// acceptDetails is Details' result gate: strict latest-only on the want, and
// the report lands only for the question the screen asks now, beside the ids
// (never instead of them). Dropped silently: no Log line, no note, and
// details.pending is left as it is: the entry that changed the key owns the
// next run.
func acceptDetails(m Model, msg refreshResultMsg) bool {
	if !acceptExactWant(m, msg) {
		return false
	}
	p, _ := msg.payload.(detailsPayload)
	now, ok := m.detailsKeyFor()
	return ok && p.key == now
}

// commitDetails lands an accepted Details result.
func (m Model) commitDetails(msg refreshResultMsg) (Model, tea.Cmd) {
	p, _ := msg.payload.(detailsPayload)
	// The report's command becomes an argv line HERE, at the commit gate,
	// on the success and the accepted-error branch alike — a
	// result the coordinator dropped never reaches this, so a
	// spawn that never painted is never logged (C3).
	m = m.logCommandsFor(surfaceDetails, msg.wantID, p.commands, failedLast(msg.err, p.commands)...)
	if p.note != "" && !m.isQuiet(surfaceDetails, msg.wantID) {
		m = m.logTrace("details", "%s", p.note)
	}
	if p.influence && !m.isQuiet(surfaceDetails, msg.wantID) {
		// Said once per set of left-out stars (Astrolog NEXT.md BUG 7), and
		// again only when the set changes.
		said := fmt.Sprint(p.starsLeft)
		if len(p.starsLeft) == 0 {
			said = ""
		}
		if said != m.starsLeftSaid {
			m.starsLeftSaid = said
			if said != "" {
				m = m.logTrace("details", "%s", starsLeftLine)
			}
		}
	}
	if msg.err != nil {
		// A failed replacement is not a frame (C6).
		m.details.pending = false
		m.details.waitForText = false
		m.details.timerArmed = false
		// The pane reads one faint line for the key that failed and keeps no
		// older content under it: the old report is gone from the pane, and
		// from every action on it, with the line.
		m.details.title, m.details.body, m.details.keyed = "", "", false
		m.details.key = contentKey{}
		m.details.failed.set, m.details.failed.key = true, p.key
		m.details.query, m.details.searching, m.details.scrollOffset = "", false, 0
		m = m.noteEphemeris(msg.err).logError("details", "report not read, %s", m.failureReason(msg.err, m.consolePointer(len(p.commands) > 0))).logWarnings(msg.err)
		m.errorKeptView = true
		return m, nil
	}
	// Title and report land together. Navigation leaves the last successful
	// frame in place until this atomic replacement, so there is no visible
	// pending, blank, partial, or error frame. The scroll offset follows
	// the context, not the struct: same title keeps the reader's place.
	m = m.landReport(p).rememberReport(p)
	return m, nil
}

// replayReportTrace writes the trace lines a fresh report commit writes about
// the result itself, for a report drawn from memory: the note of the rule
// applied after Astrolog and the fixed stars an influence chart leaves out.
func (m Model) replayReportTrace(p detailsPayload) Model {
	if p.note != "" {
		m = m.logTrace("details", "%s", p.note)
	}
	if p.influence && len(p.starsLeft) > 0 {
		m = m.logTrace("details", "%s", starsLeftLine)
	}
	return m
}

// starsLeftLine is the trace line for the fixed stars an influence chart leaves
// out (Astrolog NEXT.md BUG 7).
const starsLeftLine = "no fixed stars in the influence chart, Astrolog 8.00 miscounts them"

// landReport puts a computed report on screen under its key. A fresh result and
// one adopted from memory land by it alike.
func (m Model) landReport(p detailsPayload) Model {
	m = m.adoptDetailsReport(p.title, p.body, contentKey{kind: reportRaw, raw: p.key})
	m.details.pending = false
	m.details.waitForText = false
	m.details.timerArmed = false
	return m
}

// nodeTitleFromCall is a Details title with the form words of a Node or Lilith
// report taken from the call that cast it (cast), whatever form the request
// asked for: the title names what Astrolog drew.
func nodeTitleFromCall(title string, o aspect.Object, cast recipe.NodeForm) string {
	to := nodeFormTitle(o, cast)
	if to == "" {
		return title
	}
	for _, f := range []recipe.NodeForm{recipe.NodesMean, recipe.NodesTrue, recipe.NodesMeanNatural, recipe.NodesTrueNatural} {
		if from := nodeFormTitle(o, f); from != to && strings.Contains(title, from) {
			return strings.Replace(title, from, to, 1)
		}
	}
	return title
}

// nodeFormTitle is the Details title of the North Node, the South Node and
// Lilith, which names the form of the report's call in words: `North Node
// (true)`, `Lilith (natural)`. "" for every other object.
func nodeFormTitle(o aspect.Object, f recipe.NodeForm) string {
	node, lilith := f.Words()
	switch o.Name {
	case "North Node", "South Node":
		return o.Name + " (" + node + ")"
	case "Lilith":
		return o.Name + " (" + lilith + ")"
	}
	return ""
}
