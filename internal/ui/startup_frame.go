package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
)

// newTextRow is the Text surface's refresh policy (specs/
// refresh-descriptor-text.md): the coordinator asks it through Model.rowFor. It
// has no inputs (no Text key yet: that is its own slice), no rest timer and no
// suspend: identity changes fence it through bumpIdentity. The trigger is the
// entry points only, noteWant (leading) and bumpIdentity. Supersede is polite
// for a burst. The accept is the stale-error drop, and a stale success still
// commits as the burst's leading frame. FREEZE-SAFETY: the build never
// returns nil, or an unmet want would be stranded. Its spawns are the cast
// lookups, held on the row so a test replaces them on its own Model.
func newTextRow() surfaceRow {
	return surfaceRow{
		trigger:   triggerEventLeading,
		supersede: supersedePolite,
		debounce:  textDebounce,
		build:     buildTextCmd,
		accept:    acceptTextNotStaleError,
		commit: func(m Model, msg refreshResultMsg) (Model, tea.Cmd) {
			p, _ := msg.payload.(textPayload)
			m, cmd := applyTextCommit(m, p.msg)
			return m.rememberText(p.msg), cmd
		},
		// started: a selection whose bodies all lack their ephemeris file draws
		// the default set, and the Log says so once per run that asks for it.
		started: Model.sayDrainedSelection,
		// remembered: a frame computed before for the key this want is for
		// is drawn at once (memory.go).
		remembered: (Model).adoptTextFromMemory,
		// asked: the key of the frame this want is for (reconcileText).
		asked: func(m Model) Model {
			m.textAsked, m.textAskedOK = m.textKeyFor()
			return m
		},
		spawn: rowSpawns{cast: realCastSpawns()},
	}
}

// newImageRow is the Image surface's refresh policy (specs/
// refresh-descriptor-image.md). No inputs (the wheel's own key is
// wheelPlan.stillKey, the accept's question); the trigger is the entry points
// only (noteWant leading, and Ctrl+X); supersede is live serial: a render in
// flight is never cancelled, and a result the accept refuses is followed by a
// run for the newest want. No rest timer, no suspend. started stamps the run
// key. Its spawns are the renders and the preview server's three calls.
func newImageRow() surfaceRow {
	return surfaceRow{
		trigger:   triggerEventLeading,
		supersede: supersedeLiveSerial,
		debounce:  imageDebounce,
		build:     buildImageCmd,
		accept:    acceptImageFrame,
		commit:    commitImage,
		started:   stampWheelRunKey,
		spawn:     rowSpawns{wheel: realWheelSpawns()},
	}
}

// acceptTextNotStaleError drops a STALE text ERROR whole: committing it would
// blank a good chart mid-burst for nothing (it is not even logged), and the
// still-pending debounce tick recomputes the settled frame (lastCommitWantID
// stays behind, so the tick's idle check fires). Stale SUCCESSES still commit:
// that is the leading frame.
func acceptTextNotStaleError(m Model, msg refreshResultMsg) bool {
	return msg.err == nil || msg.wantID == m.refresh[msg.surface].wantID
}

// applyTextCommit stores the chart payload and moves to ready. It is the
// text surface's COMMIT body, invoked by the refresh coordinator only after
// the ordering gates passed (superseded-run drop, canceled-run drop,
// identity floor, stale-error drop — see onRefreshResult in refresh.go; the
// old computeGen/dispatchSeq checks lived here before S4 retired them). A
// commit is either a SUCCESS (current or the burst's leading frame — stale
// successes paint for instant feedback while the pending tick computes the
// settled final frame) or a CURRENT-want error; stale errors never reach
// here (they would blank a good chart mid-burst — the review).
// A current-want Err is non-fatal: the ready view renders empty panels
// rather than blocking on the modal indefinitely.
func applyTextCommit(m Model, msg FirstFrameReadyMsg) (Model, tea.Cmd) {
	return commitTextFrame(m, msg, m.isTickFrame(msg.WantID))
}

// commitTextFrame is the commit itself.
func commitTextFrame(m Model, msg FirstFrameReadyMsg, quiet bool) (Model, tea.Cmd) {
	// The frame's commands become argv lines HERE, at the commit gate, in
	// carried order — never where the compute returns, so a result
	// the coordinator dropped leaves no line (C3). An error frame logs its
	// commands too: the command that failed is the one the operator most
	// needs (C4), and the failure itself goes on the error lane (C11).
	// A frame the minute tick asked for logs nothing there: the last of its
	// commands replaces the one `now:` line instead (live_tick.go).
	failed := msg.FailedAt
	if failed == nil {
		failed = failedLast(msg.Err, msg.Commands)
	}
	if quiet {
		m = m.setNowLine(msg.Commands, failed...)
	} else {
		m = m.logCommands(msg.Commands, failed...)
	}
	if msg.Err != nil {
		return m.commitRefusedFrame(msg)
	}
	m.refused = textRefused{}
	var date string
	if s := m.focusedSlot(); s != nil && !msg.UnavailableUnion {
		if s.Floating && !msg.TransitTime.IsZero() {
			// The moment this frame was cast for: Now may have moved on
			// while it was computed.
			cast := *s
			cast.Time = msg.TransitTime
			s = &cast
		}
		date = slotDate(s)
	}
	m = m.reportUnavailable(msg.Unavailable, date)
	m.chart.positions = msg.Positions
	m.chart.nodes = msg.Nodes
	m.chart.stations = msg.Stations
	// One note per failed probe run: a burst coalesces into one commit, so the
	// note cannot repeat per key (the specification C5).
	if msg.StationErr != nil {
		m = m.logTrace("bodies", "station marks unavailable (%v)", msg.StationErr)
	}
	m.chart.natalPositions = msg.NatalPositions
	m.chart.natalCusps = msg.NatalCusps
	m.chart.framedPositions = msg.FramedPositions
	m.chart.cusps = msg.Cusps
	m.chart.bodyStyles = msg.BodyStyles
	m.chart.aspects = msg.Aspects
	m.chart.aspectStyles = msg.AspectStyles
	m.chart.aspectsReport = msg.AspectsReport
	m.chart.crossAspects = msg.CrossAspects
	m.chart.crossAspectStyles = msg.CrossAspectStyles
	m.chart.crossAspectsReport = msg.CrossAspectsReport
	m.chart.crossSynastry = msg.CrossSynastry
	m.chart.overlay = msg.Overlay
	m.chart.settled = true
	m = m.commitColumns(msg)
	m = m.commitCasts(msg.Casts)
	m = m.noteHouseSources()
	if rows := panel.BodyRowCount(m.chart.positions, m.chart.cusps); rows == 0 {
		m.bodyCursorIdx = 0
	} else if m.bodyCursorIdx >= rows {
		m.bodyCursorIdx = rows - 1
	}
	// `startup` names BOTH the startup phase and which modal is open, so a
	// frame commit must not promote out of a modal state. The aspect chooser
	// previews while it is open (C9): without this guard the preview's own
	// result closes the modal the instant it lands, which is the exact
	// opposite of what a preview is for.
	//
	// The pickers are unaffected: applyAtlasPicked and applyChartResolved both
	// move to startupComputing BEFORE their compute, so a commit never lands
	// while one of them is on screen.
	if !m.startupShowsModal() {
		m.startup = startupReady
	}
	// Commit the compute's target time to slots.floating.Time so the
	// chart panel header advances IN LOCKSTEP with bodies/aspects.
	// During a held-key burst this means slot.Time advances one
	// committed step at a time even though m.targetFloatingTime has
	// already raced to the latest target.
	if m.slots != nil && !msg.TransitTime.IsZero() {
		m.slots.SetFloatingTime(msg.TransitTime)
	}
	// Bodies and Aspects swap to this frame together: it carries all of their
	// results, so the stamp is the whole swap.
	m.chartEpoch++
	m.chart.epoch = m.chartEpoch
	// The key the run was asked for rides with its result; a frame built by
	// hand carries none and takes the key asked for now.
	key, keyed := msg.key, msg.keyed
	if !keyed {
		key, keyed = m.textKeyFor()
	}
	m = m.stampShown(key, keyed)
	// The list under the cursor changed (new positions, new cusps): the window
	// is put back where the cursor's row is in sight, wherever the rows went.
	// After the stamp, which releases the hold: shownChart is this frame now.
	m = m.settleBodyTop()
	// Details is derived only from a settled committed frame, not from the
	// operator's possibly newer target or an intermediate held-key frame. Direct
	// focus/body navigation owns its own trailing timer and must not be preempted
	// by an unrelated text commit.
	var detailsCmd tea.Cmd
	textSettled := msg.WantID == m.refresh[surfaceText].wantID
	detailsNeedsText := m.details.waitForText || (m.details.title == "" && m.details.body == "")
	// While Aspects is focused Details is the frame's own report, so
	// every settled commit refreshes it — no timer, no spawn.
	if textSettled && (detailsNeedsText || m.focused == PanelAspects) {
		m, detailsCmd = m.requestDetails()
	}

	// Auto-refresh the external wheel preview. In overlay mode the bi-wheel
	// tracks the transit (floating) moment, which advances on every overlay
	// recompute, so refresh regardless of which slot is focused. In single mode
	// only the floating slot's wheel changes on recompute, so refresh just when
	// it is focused (a static natal slot needs no re-render). Trailing-debounced
	// on the image surface, so a burst's commits coalesce into one render each.
	// The timeline follows the committed frame too: a commit that changed
	// any of its search inputs — the floating month, zone or anchor — runs
	// its searches, and any other commit leaves them alone (C10, C25).
	var timelineCmd tea.Cmd
	m, timelineCmd = m.syncTimeline()
	m = m.clampTimelineCursor()
	if m.comparing() || m.focusedIsFloating() {
		var imageCmd tea.Cmd
		m, imageCmd = m.refreshWheelIfActive()
		return m, tea.Batch(detailsCmd, imageCmd, timelineCmd)
	}
	return m, tea.Batch(detailsCmd, timelineCmd)
}

// commitRefusedFrame is the commit of a text frame Astrolog refused to cast.
// Nothing the panels show is replaced: Bodies, Aspects and Details keep what
// they showed, and the floating chart keeps the moment that content was cast
// at. The refused key is remembered, so the panes say the cast failed (the
// view rule, subject.go) and nothing asks for that frame again until the key
// asked for changes. The Log says which chart failed and why.
func (m Model) commitRefusedFrame(msg FirstFrameReadyMsg) (Model, tea.Cmd) {
	m = m.noteEphemeris(msg.Err).logError("astrolog", "%s not cast, %s", chartCaption(m.focusedSlot()), m.failureReason(msg.Err, m.consolePointer(len(msg.Commands) > 0))).logWarnings(msg.Err)
	// An error commits only for the newest want, so the key asked for now is
	// the key that was refused.
	key, keyed := m.textKeyFor()
	m.refused = textRefused{set: keyed, key: key}
	if !m.startupShowsModal() {
		m.startup = startupReady
	}
	if msg.WantID == m.refresh[surfaceText].wantID {
		if m.details.waitForText && m.reportTabHidden() {
			// The report tab is hidden: the pending mark stays as it was and
			// the catch-up asks again when the report tab is chosen.
			m.details.stale = true
		} else {
			// No report can be cast over a frame that was refused.
			m.details.pending = false
			m.details.waitForText = false
			m = m.suspendSurface(surfaceDetails)
		}
	}
	var timelineCmd tea.Cmd
	m, timelineCmd = m.syncTimeline()
	m = m.clampTimelineCursor()
	return m, timelineCmd
}

// reportUnavailable notes, once, in the log console which selected objects this
// Astrolog cannot compute. Their rows were dropped at the parse rather than
// drawn at 0° Aries with no motion, which is what Astrolog returns for them.
// The note is the console's, never the Log panel's: how Sextile works around
// the missing data is not something the operator asked about.
//
// ONCE means once per change, not once per frame. The text surface recomputes
// on every time-step and every cursor move, and the same three names arriving
// sixty times during a held-key sweep would fill the console. The set is
// remembered and compared, so selecting Eris notes one line, stepping time
// notes none, and deselecting it notes nothing.
func (m Model) reportUnavailable(names []string, date string) Model {
	if sameStringSet(m.lastUnavailable, names) {
		return m
	}
	m.lastUnavailable = append([]string(nil), names...)
	if len(names) == 0 {
		// Recovery says nothing: the bodies are drawn again, and that is
		// what the operator sees (the specification).
		return m
	}
	// The same sentence shape as the startup account (ephemerisAccount); this is
	// the date backstop, for a chart outside a file's range.
	what := "no ephemeris data"
	if date != "" {
		what += " for " + date
	}
	// The bound absentLines has: at most three names, when the sentence fits
	// the Log's width; otherwise the count.
	if list, few := nameList(names); few {
		if line := list + " can't be drawn: " + what; fits(line, m.logLineLimit()) {
			return m.logTrace("bodies", "%s", line)
		}
	}
	if len(names) == 1 {
		return m.logTrace("bodies", "1 body can't be drawn: %s", what)
	}
	return m.logTrace("bodies", "%d bodies can't be drawn: %s", len(names), what)
}

// sameStringSet compares two name lists as sets, so a reordering by Astrolog
// does not read as a change and re-log.
func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
		if seen[s] < 0 {
			return false
		}
	}
	return true
}

// unionNames merges two unavailable-object lists, preserving first-encounter
// order and dropping duplicates. Overlay mode computes two charts under one
// lens, so an object with no ephemeris is missing from BOTH — reporting it
// twice would be noise, and reporting neither is how it went silent.
func unionNames(a, b []string) []string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, n := range list {
			if seen[n] {
				continue
			}
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// slotDate is the date a slot's row shows, in Astrolog's calendar (Julian
// before 15 October 1582): a chart file's own clock, or the slot's moment at
// its place. "" when the slot shows no moment.
func slotDate(s *store.Slot) string {
	if c := s.Chart.Clock; !s.Floating && !c.IsZero() {
		return strings.TrimSpace(civilDate(c.Year, c.Month, c.Day))
	}
	if s.Time.IsZero() {
		return ""
	}
	t := zonedIn(s.Time, s.Location)
	st := lookup.StampMoment(t, t.Location())
	return strings.TrimSpace(civilDate(st.Year, st.Month, st.Day))
}
