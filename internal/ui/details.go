package ui

import (
	"context"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
)

type detailsViewState struct {
	title       string
	body        string
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
	// attached is whether row navigation is inside Details (D12). Enter
	// attaches and Esc detaches; it is a flag and never a focus change,
	// because Details is not in the focus ring (C15, C40). While it is set,
	// Details owns the keyboard (D18, updateDetailsKey).
	//
	// Details' SUBJECT — the (panel, slot) it reports on — is not stored:
	// it is (m.focused, m.cursorIdx), see detailsSubject. m.focused is never
	// Details, and the keys that would move either are Details' own while
	// attached, so the subject is frozen for the whole attachment by
	// construction rather than by a copy that could drift.
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
// focused slot. It follows the ring (tab, ←/→, 1/2/3) and the slot cursor,
// and attaching or detaching changes neither (C31). The console's priorFocus
// (console.go) is the same shape for the log console.
type detailsSubject struct {
	panel PanelID
	slot  int
}

func (m Model) detailsSubject() detailsSubject {
	return detailsSubject{panel: m.focused, slot: m.cursorIdx}
}

// detailsReportLines splits a report into the lines the panel scrolls. A
// report's final newline TERMINATES its last line rather than starting an
// empty one — Astrolog ends every report with one — so it is never a row the
// operator can scroll onto. An empty report has no lines at all.
func detailsReportLines(body string) []string {
	body = strings.TrimSuffix(body, "\n")
	if body == "" {
		return nil
	}
	return strings.Split(body, "\n")
}

// detailsLineCount is the report's line count as the panel renders it.
func detailsLineCount(body string) int {
	return len(detailsReportLines(body))
}

// detailsInnerHeight is how many report lines the Details panel body can show,
// or 0 when the panel is not in the layout.
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
	m.details.scrollOffset = clampOffset(m.details.scrollOffset, total, height)
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
		return len(templateBodyLines(m.tpl.lines, r.w-2, m.cfg)), r.h - 2
	}
	return len(m.detailsView().lines), m.detailsReportHeight()
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
	m.details.scrollOffset = clampOffset(m.details.scrollOffset+delta, total, height)
	return m
}

// adoptDetailsReport replaces the visible frame with a completed report and
// decides what happens to the scroll offset: the same title is the same
// context refreshed under the operator's eyes, so their position is kept (and
// re-clamped to the new text); a different title is a new report, read from
// the top.
func (m Model) adoptDetailsReport(title, body string) Model {
	same := title == m.details.title
	prev := m.details.scrollOffset
	m.details.title, m.details.body, m.details.scrollOffset = title, body, 0
	if !same {
		// A new subject: the query was about the old one (the specification 7).
		m.details.query, m.details.searching = "", false
		return m
	}
	m.details.scrollOffset = clampOffset(prev, len(m.detailsView().lines), m.detailsReportHeight())
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
}

var (
	computeRawReport       = lookup.ComputeRawReport
	computeCrossBodyReport = lookup.CrossBodyRawReport
	// computeSynastryBodyReport is the cross report's anchored × anchored form.
	computeSynastryBodyReport = lookup.SynastryCrossBodyRawReport
)

// detailsVisible is the work gate, not merely a render check: no Details
// request may start unless layoutGeometry actually includes the panel.
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
	if !m.detailsVisible() || m.startup != startupReady {
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
	switch m.focused {
	case PanelChart:
		req.title = detailsPrefix + tabTitleChart
		req.report.Kind = lookup.RawReportChart
	case PanelBodies:
		body, ok := bodyAtCursor(m.chart.positions, m.chart.cusps, m.bodyCursorIdx)
		if !ok {
			return detailsRequest{}, false
		}
		req.report.Kind = lookup.RawReportBody
		req.report.Body = body
		req.title = detailsPrefix + bodyName(body)
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
			req.title += " " + m.pairTabTitle(sc.numeral(pair.left), sc.numeral(pair.right))
		}
	default:
		return detailsRequest{}, false
	}
	return req, true
}

// aspectsDetailsFromFrame is the Aspects panel's Details: the committed
// frame's own report for whichever content the panel is showing — the focused
// slot's -aa list on Aspects, the pair's -rt -aa or -r0 -aa list while
// comparing — exactly the text the waterfall was parsed from, so both panels
// always describe the same Astrolog run. Both read the SAME derived tab id as
// the strip and the renderer, so the Details title always names what
// the Aspects panel is actually showing. ok is false when the frame carries no
// report yet (nothing committed, or an error frame), so the caller keeps the
// last complete Details frame in place.
func (m Model) aspectsDetailsFromFrame() (title, body string, ok bool) {
	if active := m.activeTabID(PanelAspects); isAspectsPairTab(active) {
		title := ""
		for _, tab := range m.panelTabs(PanelAspects) {
			if tab.ID == active {
				title = tab.Title
			}
		}
		return detailsPrefix + title, m.chart.crossAspectsReport, m.chart.crossAspectsReport != ""
	}
	return detailsPrefix + tabTitleAspects, m.chart.aspectsReport, m.chart.aspectsReport != ""
}

// applyAspectsDetails fills Details from the frame with no Astrolog work
// : it cancels and fences any in-flight report — a Chart or body
// report left over from the previous focus must not paint over this — clears
// the pending state, and replaces title and body atomically, or leaves the
// last complete frame untouched when the frame has no report to give.
func (m Model) applyAspectsDetails() Model {
	if !m.detailsVisible() {
		return m
	}
	m = m.suspendSurface(surfaceDetails)
	m.details.pending = false
	m.details.waitForText = false
	m.details.timerArmed = false
	if title, body, ok := m.aspectsDetailsFromFrame(); ok {
		m = m.adoptDetailsReport(title, body)
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

// deferDetails handles contexts that change without a chart-text recompute
// (Bodies cursor and panel focus). Every key cancels/fences old work and records
// a new desired context, while at most one trailing timer remains armed; no
// Astrolog process starts until navigation settles. The last successful title
// and body remain untouched throughout the burst, so Details has no visible
// intermediate state to repaint.
func (m Model) deferDetails() (Model, tea.Cmd) {
	if !m.detailsVisible() {
		return m, nil
	}
	if m.reportTabHidden() {
		m.details.stale = true
		m.detailsHold.seq++ // the timeline or template tab now awaits its content
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
	return m, armRefresh(surfaceDetails, m.refresh[surfaceDetails].timerID)
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
		m.detailsHold.seq++ // the timeline or template tab now awaits its content
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

// The hold keeps every Details tab faint while a navigation key is held. A
// terminal reports no key release, so a held key is a run of the same key:
// one press, the terminal's delay before the first repeat (up to 500 ms on
// Windows and GNOME), then repeats every 30-50 ms. A tap is a run of one and
// never greys: the previous content stays as drawn until the new content
// commits and paints. The second key of a run, within holdStartWindow of the
// first, starts the hold; each further key extends it. The hold ends when no
// key of the run has arrived for releaseQuiet, three of the run's measured
// repeat gaps, so it ends about 0.1 s after the key is let go. Any other key
// ends the run and is judged on its own.
const (
	holdStartWindow = 650 * time.Millisecond
	// holdQuietUnmeasured is releaseQuiet before the run has a repeat gap.
	holdQuietUnmeasured = 150 * time.Millisecond
	holdQuietMin        = 60 * time.Millisecond
	holdQuietMax        = 200 * time.Millisecond
	// holdGaps is how many of the run's latest repeat gaps are measured.
	holdGaps = 5
)

type detailsHoldMsg struct{ id uint64 }

// detailsHoldState is the hold. seq counts the changes that leave a Details
// tab awaiting content (markDetailsPending, and a hidden report tab's
// timeline or template); Update compares it across a key. on keeps every
// Details tab faint; id names the newest quiet timer, an older one is stale.
// key, keys and last are the current run: its key, how many keys it has had
// and when the newest arrived; gaps are its latest repeat gaps. tapped says
// the wait on screen was started by a tap, so it is not drawn faint; it
// clears once nothing waits.
type detailsHoldState struct {
	seq    uint64
	on     bool
	id     uint64
	key    string
	keys   int
	last   time.Time
	gaps   []time.Duration
	tapped bool
}

// followKeyRun judges one key for the hold. changed says the key changed
// what Details describes; only such a key joins or starts a run.
func (m Model) followKeyRun(k tea.KeyMsg, changed bool) (Model, tea.Cmd) {
	h := m.detailsHold
	now := m.clock.Now()
	sameRun := h.keys > 0 && k.String() == h.key && now.Sub(h.last) <= holdStartWindow
	if !sameRun && (k.String() != h.key || changed) {
		h.on, h.key, h.keys, h.gaps = false, "", 0, nil
	}
	if !changed {
		m.detailsHold = h
		return m, nil
	}
	if sameRun {
		// The gap before the first repeat is the terminal's delay, not
		// its repeat rate.
		if h.keys >= 2 {
			// A fresh slice: an earlier Model must keep its own gaps.
			h.gaps = append(slices.Clone(h.gaps), now.Sub(h.last))
			if len(h.gaps) > holdGaps {
				h.gaps = h.gaps[1:]
			}
		}
		h.keys++
	} else {
		h.key, h.keys = k.String(), 1
	}
	h.last = now
	h.tapped = h.keys < 2
	if h.tapped {
		m.detailsHold = h
		return m, nil
	}
	h.on = true
	h.id++
	id := h.id
	m.detailsHold = h
	return m, longTimer(h.releaseQuiet(), func(time.Time) tea.Msg { return detailsHoldMsg{id: id} })
}

// releaseQuiet is how long the run may go without a key before the hold
// ends: three times the median of its measured repeat gaps, clamped.
func (h detailsHoldState) releaseQuiet() time.Duration {
	if len(h.gaps) == 0 {
		return holdQuietUnmeasured
	}
	gaps := slices.Clone(h.gaps)
	slices.Sort(gaps)
	switch q := 3 * gaps[len(gaps)/2]; {
	case q < holdQuietMin:
		return holdQuietMin
	case q > holdQuietMax:
		return holdQuietMax
	default:
		return q
	}
}

// releaseDetailsHold ends the hold when its newest timer fires.
func (m Model) releaseDetailsHold(msg detailsHoldMsg) Model {
	if msg.id == m.detailsHold.id {
		m.detailsHold.on = false
	}
	return m
}

func (m Model) markDetailsPending(waitForText bool) Model {
	m.detailsHold.seq++
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
	m.details = detailsViewState{tabs: m.details.tabs, preEnter: m.details.preEnter, preEnterSet: m.details.preEnterSet}
	m = m.suspendTimeline()
	// A template tab's lines that never completed forget their key, so the
	// run starts again when Details returns.
	if !m.tpl.complete {
		m.tpl = templateState{cycle: m.tpl.cycle, cycleFor: m.tpl.cycleFor}
	} else if m.tpl.done != m.tpl.key {
		// A run cancelled with the lines still the previous moment's: the key
		// is forgotten (the lines stay) so the run starts again on return.
		m.tpl.keyed = false
	}
	m = m.suspendSurface(surfaceTemplate)
	return m.suspendSurface(surfaceDetails)
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
	lens := m.comparisonLens()
	if !req.cross {
		lens = m.showLens(m.focusedSlot())
	}
	return func() tea.Msg {
		// Exactly one spawn either way: the comparison body report is a
		// different Astrolog run, not an extra one. Its command rides on the
		// payload — on the error path too, whenever the spawn
		// happened; an empty command means no process started and nothing
		// is carried.
		raw, command, err := "", "", error(nil)
		if req.cross && req.synastry {
			raw, command, err = computeSynastryBodyReport(ctx, lens, req.natalChart, req.when, req.place, req.otherLabel, req.report.Body)
		} else if req.cross {
			raw, command, err = computeCrossBodyReport(ctx, lens, req.natalChart, req.when, req.place, req.report.Body)
		} else {
			raw, command, err = computeRawReport(ctx, req.when, req.place, lens, req.report)
		}
		var commands []string
		if command != "" {
			commands = []string{command}
		}
		return refreshResultMsg{
			surface: surfaceDetails,
			runID:   runID,
			wantID:  wantID,
			payload: detailsPayload{title: req.title, body: raw, commands: commands},
			err:     err,
		}
	}
}
