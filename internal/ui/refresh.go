package ui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/preview"
)

// surface identifies an independently-debounced render target. Each surface runs
// its own latest-wins single-flight loop, so image, chart text, and passive raw
// Details work never block or supersede one another.
type surface int

const (
	surfaceImage    surface = iota // the external wheel SVG (async latest-wins)
	surfaceText                    // bodies + aspects (leading + trailing) — wired in S4
	surfaceDetails                 // passive raw Astrolog report (strict latest-only)
	surfaceTimeline                // Details timeline searches (strict latest-only, keyed by search inputs)
	surfaceTemplate                // a body template tab's facts (strict latest-only, keyed by its inputs)
	numSurfaces
)

// Per-surface debounce policy (policy B; tune by feel). Text gets a leading edge
// so the first frame of a held-key burst is instant. Image also renders leading
// when preview-following a committed chart frame so a held key visibly animates
// the graphic; the expensive work still runs off Update and remains single-flight.
const (
	textDebounce  = 50 * time.Millisecond
	imageDebounce = 150 * time.Millisecond
	// Details keeps one checkpoint timer during held navigation. A single key
	// settles in 150ms; a key arriving inside that window causes one further
	// checkpoint, so a burst settles within 150-300ms without per-key timers.
	detailsDebounce = 150 * time.Millisecond
)

func debounceFor(s surface) time.Duration {
	switch s {
	case surfaceImage:
		return imageDebounce
	case surfaceDetails:
		return detailsDebounce
	default:
		return textDebounce
	}
}

// refreshState is the per-surface coordinator state: a latest-wins single-flight
// with cancellation. wantID is the latest DESIRED state; runningRunID identifies
// the one actually-launched run. Keeping the two DISTINCT is what stops a stale
// duplicate result from looking current and causes correct supersede behaviour
// (the reviewer A). cancel is the in-flight run's CancelFunc — a runtime handle, nil
// when idle — and is the only mutable business-free field carried in the Model.
type refreshState struct {
	wantID           uint64             // bumps on every relevant input
	timerID          uint64             // latest armed debounce token; stale ticks self-ignore
	runningRunID     uint64             // unique per actually-started run (NOT the want)
	runningWantID    uint64             // the wantID this run was launched for
	lastCommitWantID uint64             // wantID of the last committed result
	cancel           context.CancelFunc // cancels the in-flight run; nil when idle

	// commitFloor fences off results from a superseded WORKSPACE IDENTITY
	// (atlas city re-pick, natal anchor pick, cold-start (re)birth): results
	// whose wantID is below the floor must never paint, even if already
	// posted to the message queue. bumpIdentity raises it to the new wantID.
	// In current flows the runID gate already drops those results (startRun
	// unconditionally advances runningRunID), so the floor is the EXPLICIT,
	// non-emergent statement of identity safety — the design-check
	// endorsed keeping both.
	commitFloor uint64
}

// refreshTickMsg fires when a surface's debounce window elapses. Only the tick
// whose timerID still matches the surface's latest-armed token acts; earlier
// ticks (a newer input re-armed in the meantime) self-ignore.
type refreshTickMsg struct {
	surface surface
	timerID uint64
}

// refreshResultMsg carries a completed surface run. runID/wantID let the
// coordinator drop superseded runs (runID) and stale-but-completed frames
// (wantID) without committing them.
type refreshResultMsg struct {
	surface surface
	runID   uint64
	wantID  uint64
	payload refreshPayload
	err     error
}

// refreshPayload is the surface-specific result a committed run delivers.
type refreshPayload interface{ isRefreshPayload() }

// imagePayload is surfaceImage's result: the rendered wheel SVG bytes. The
// bytes are handed to showWheel only at the commit point (latest run only), so
// two renders can never race the stable browser URL (the reviewer F).
type imagePayload struct {
	bytes []byte
	// rings name the charts of a multi-ring render, in ring order, so a staging
	// failure can say which slot it was (C16). Empty for a single chart.
	rings []string
	// commands are the argv lines of the spawns that started, written to the
	// Log's argv lane at the commit gate on success and failure alike. Only the multi-ring render reports them.
	commands []string
}

func (imagePayload) isRefreshPayload() {}

// textPayload is surfaceText's result: the chart compute (positions, natal
// layer, aspects, the moment it was computed for). It wraps the exported
// FirstFrameReadyMsg so cross-package drivers (the invariants first-frame
// golden) can hand-construct results; the Update loop converts the exported
// message into the coordinator's internal result shape.
type textPayload struct{ msg FirstFrameReadyMsg }

func (textPayload) isRefreshPayload() {}

// detailsPayload is surfaceDetails' raw, uncolored Astrolog stdout plus the
// context title captured at dispatch, and the command that produced the
// report — one entry, or none when no process started. It
// becomes an argv line at commitRefresh(surfaceDetails).
type detailsPayload struct {
	title    string
	body     string
	commands []string
}

func (detailsPayload) isRefreshPayload() {}

// showWheel publishes a rendered wheel SVG to the browser preview. Indirected
// through a package var (same idiom as astrolog.binary) so tests can substitute
// a stub — the real preview.Show starts an HTTP server and may launch the
// Windows browser, side effects a unit test must not trigger.
var showWheel = preview.Show

// closeWheelServer stops the wheel server (a second Ctrl+X). Indirected like
// showWheel.
var closeWheelServer = preview.Close

// requestViewerOpen tells the preview the operator asked to see the wheel
// (Ctrl+X), so the next publish may open a viewer. Indirected like showWheel.
var requestViewerOpen = preview.RequestOpen

// armRefresh schedules a trailing debounce tick for surface s carrying timerID.
func armRefresh(s surface, timerID uint64) tea.Cmd {
	return tea.Tick(debounceFor(s), func(time.Time) tea.Msg {
		return refreshTickMsg{surface: s, timerID: timerID}
	})
}

// noteWant registers that surface s's desired output changed (the caller has
// already mutated the target state): it bumps wantID and arms the trailing
// debounce. When leading is true and the surface is idle, it also starts a run
// immediately so the first frame of a burst is instant. Ctrl+X uses
// requestImageNow instead to preserve repeat-key coalescing.
func (m Model) noteWant(s surface, leading bool) (Model, tea.Cmd) {
	m.refresh[s].wantID++
	m.refresh[s].timerID = m.refresh[s].wantID
	cmds := []tea.Cmd{armRefresh(s, m.refresh[s].timerID)}
	if leading && m.refresh[s].cancel == nil && m.refresh[s].runningWantID != m.refresh[s].wantID {
		var c tea.Cmd
		m, c = m.startRun(s)
		if c != nil {
			cmds = append(cmds, c)
		}
	}
	return m, tea.Batch(cmds...)
}

// startRun launches surface s's compute for the current wantID: assigns a fresh
// runID, creates a cancelable context, stores the CancelFunc, and returns the
// surface Cmd. If the surface has nothing to run right now (e.g. no focused
// slot), it cancels the unused context to avoid a leak and returns no Cmd.
//
// runningRunID advances UNCONDITIONALLY — even on a nil-Cmd start. That
// increment is load-bearing for identity safety: any result already posted by
// an earlier run fails the runID gate the moment a newer start happens, so a
// superseded workspace's frame can never paint (commitFloor states the same
// contract explicitly).
func (m Model) startRun(s surface) (Model, tea.Cmd) {
	m.refresh[s].runningRunID++
	m.refresh[s].runningWantID = m.refresh[s].wantID
	ctx, cancel := context.WithCancel(context.Background())
	cmd := m.buildSurfaceCmd(s, ctx, m.refresh[s].runningRunID, m.refresh[s].runningWantID)
	if cmd == nil {
		cancel()
		return m, nil
	}
	m.refresh[s].cancel = cancel
	if s == surfaceImage {
		m.wheelRunKey, _ = m.wheelStillKey()
	}
	return m, cmd
}

// wheelStillKey is the stillKey of the wheel the model would ask for now.
func (m Model) wheelStillKey() (string, bool) {
	p, ok := m.wheelPlan()
	if !ok {
		return "", false
	}
	return p.stillKey(m.wheelLens(p)), true
}

// bumpIdentity registers a WORKSPACE-IDENTITY change for surface s (atlas city
// re-pick, natal anchor pick, cold-start birth): unlike noteWant, it hard-
// cancels any in-flight run NOW (its result is for a workspace the operator
// has left — don't even finish computing it), fences off already-queued
// results via commitFloor, invalidates pending debounce ticks (timerID), and
// starts the new identity's run immediately (no debounce — an identity change
// is a single deliberate act, not a burst). A tick is still armed as a
// defensive retry in case the immediate start had nothing to run.
func (m Model) bumpIdentity(s surface) (Model, tea.Cmd) {
	if c := m.refresh[s].cancel; c != nil {
		c()
		m.refresh[s].cancel = nil
	}
	m.refresh[s].wantID++
	m.refresh[s].commitFloor = m.refresh[s].wantID
	m.refresh[s].timerID = m.refresh[s].wantID
	cmds := []tea.Cmd{armRefresh(s, m.refresh[s].timerID)}
	var c tea.Cmd
	m, c = m.startRun(s)
	if c != nil {
		cmds = append(cmds, c)
	}
	return m, tea.Batch(cmds...)
}

// suspendSurface cancels and fences a surface without arming new work. Details
// uses it when its panel disappears at narrow widths, guaranteeing a resize
// cannot leave a hidden Astrolog process or allow its queued result to paint.
func (m Model) suspendSurface(s surface) Model {
	if c := m.refresh[s].cancel; c != nil {
		c()
	}
	m.refresh[s].cancel = nil
	m.refresh[s].wantID++
	m.refresh[s].timerID = m.refresh[s].wantID
	m.refresh[s].runningRunID++
	m.refresh[s].runningWantID = m.refresh[s].wantID
	m.refresh[s].commitFloor = m.refresh[s].wantID
	m.refresh[s].lastCommitWantID = m.refresh[s].wantID
	if s == surfaceDetails {
		m.details.timerArmed = false
	}
	return m
}

// buildSurfaceCmd constructs the cancelable Cmd that computes surface s and
// returns a refreshResultMsg tagged with runID/wantID.
func (m Model) buildSurfaceCmd(s surface, ctx context.Context, runID, wantID uint64) tea.Cmd {
	switch s {
	case surfaceImage:
		// Gate the render-START on previewActive (not just the request
		// triggers): this makes it impossible for a queued debounce tick or a
		// stale-want follow-up to revive rendering after a render error cleared
		// previewActive (the review). previewActive is set true by
		// Ctrl+X (dispatchRenderWheel) before its immediate request, and stays
		// true across auto-refreshes until a render error turns it off.
		if !m.previewActive {
			return nil
		}
		plan, ok := m.wheelPlan()
		if !ok {
			return nil
		}
		if m.unresolvedFor(m.focusedSlot()) != nil {
			return nil
		}
		lens := m.wheelLens(plan)
		switch plan.mode {
		case wheelPair:
			// The active tab's bi-wheel: the pair's left chart, the base,
			// passed first, which Astrolog draws on the outer ring with its
			// houses; the cursor's chart inside (the floating slot at the
			// operator's target, or an anchored slot at its own moment).
			return renderOverlayWheelCmd(ctx, runID, wantID, plan.inner, plan.rings[1].When, plan.rings[1].Place, lens, plan.labels)
		case wheelRings:
			return renderMultiWheelCmd(ctx, runID, wantID, plan.rings, plan.labels, lens)
		}
		return renderWheelCmd(ctx, runID, wantID, plan.when, plan.place, lens, plan.highlight)
	case surfaceText:
		// FREEZE-SAFETY (the design review): the text surface must
		// ALWAYS be runnable — a nil Cmd here with no live tick would strand
		// an unmet want forever. buildTextCmd never returns nil: with no
		// workspace it falls back to a wall-clock transit compute, exactly as
		// the pre-coordinator cold-start path did.
		return buildTextCmd(m, ctx, runID, wantID)
	case surfaceDetails:
		return buildDetailsCmd(m, ctx, runID, wantID)
	case surfaceTimeline:
		return buildTimelineCmd(m, ctx, runID, wantID)
	case surfaceTemplate:
		return buildTemplateCmd(m, ctx, runID, wantID)
	}
	return nil
}

func (m Model) floatingImageTime() time.Time {
	if !m.targetFloatingTime.IsZero() {
		return m.targetFloatingTime
	}
	if m.slots != nil {
		return m.slots.Floating().Time
	}
	return m.clock.Now()
}

// ensureRunningLatest is the trailing-edge core shared by the debounce tick and
// the immediate Ctrl+X path: guarantee a run is in flight for the current want.
// If a caller reaches this helper with a run already in flight for an OLDER
// want, cancel it and replace. onRefreshTick deliberately avoids that path for
// text and image live-burst policy; identity changes and explicit immediate
// paths may still need the hard replacement behavior. If a run is in flight for
// the current want, do nothing; if idle and there is something new since the
// last commit, start.
func (m Model) ensureRunningLatest(s surface) (Model, tea.Cmd) {
	if m.refresh[s].cancel != nil {
		if m.refresh[s].runningWantID == m.refresh[s].wantID {
			return m, nil
		}
		m.refresh[s].cancel()
		m.refresh[s].cancel = nil
		return m.startRun(s)
	}
	if m.refresh[s].wantID == m.refresh[s].lastCommitWantID {
		return m, nil
	}
	return m.startRun(s)
}

// onRefreshTick handles a debounce tick: ignore it if a newer input re-armed the
// timer, else ensure a run is in flight for the latest want.
//
// Surface policies diverge when a run for an OLDER want is in flight (the reviewer S4
// design-check Q5):
//   - text: POLITE — let the older run finish (it is the burst's leading frame
//     and will commit; canceling it when compute latency ≈ the 50ms debounce
//     would routinely erase the "first frame instant" half of the UX contract)
//     and re-arm the tick to check back. Identity bumps still hard-cancel.
//   - image: LIVE SERIAL — let the active render finish, because a completed
//     intermediate SVG is still a useful held-key animation frame. Re-arm the
//     tick as a backup; a stale successful image result publishes when it is a
//     frame of a time step (onRefreshResult), and either way chains a follow-up
//     render for the newest want.
func (m Model) onRefreshTick(msg refreshTickMsg) (Model, tea.Cmd) {
	s := msg.surface
	if s == surfaceDetails {
		if !m.details.timerArmed || msg.timerID != m.refresh[s].timerID {
			return m, nil
		}
		// A timer armed for the report starts nothing once the report tab is
		// hidden; the catch-up asks when it is chosen.
		if m.reportTabHidden() {
			m.details.timerArmed = false
			m.details.stale = true
			return m, nil
		}
		// Only one Details timer exists during a held-key burst. If input
		// changed since it was armed, move the checkpoint to the latest want and
		// check again after one full idle window; do not start work yet.
		if m.refresh[s].timerID != m.refresh[s].wantID {
			m.refresh[s].timerID = m.refresh[s].wantID
			return m, armRefresh(s, m.refresh[s].timerID)
		}
		m.details.timerArmed = false
		return m.ensureRunningLatest(s)
	}
	if msg.timerID != m.refresh[s].timerID {
		return m, nil
	}
	if (s == surfaceText || s == surfaceImage) && m.refresh[s].cancel != nil && m.refresh[s].runningWantID != m.refresh[s].wantID {
		return m, armRefresh(s, m.refresh[s].timerID)
	}
	return m.ensureRunningLatest(s)
}

// onRefreshResult handles a completed run. It drops results from a superseded
// run (runID mismatch) and silently drops a canceled run (expected noise, not a
// failure). Past those shared gates, commit policy is per-surface:
//
//   - image: a completed-but-stale SUCCESS is committed only when the latest
//     want differs from its plan in the floating time alone: during held-key
//     stepping, that is the visible in-between frame. A stale picture of any
//     other plan (the cursor has moved on) is dropped. A follow-up run starts
//     immediately for the latest want either way. Stale image ERRORS stay silent and only
//     start the follow-up; a mid-burst render failure should not close a working
//     preview when a newer target already exists.
//   - text: COMMIT even when stale (serial single-flight runs carry strictly
//     increasing wantIDs, so any surviving result is newer than the last
//     commit) — this is the burst's leading frame, the "first frame instant"
//     half of the UX contract. No follow-up is chained: when a same-runID
//     result arrives with a newer want outstanding, the newest armed debounce
//     tick is necessarily still pending (had it been processed it would have
//     cancel+restarted and bumped runningRunID, failing our runID gate), and
//     that tick starts the trailing run (the design review, verified).
//     Results below commitFloor belong to a superseded workspace identity and
//     drop entirely, error included.
func (m Model) onRefreshResult(msg refreshResultMsg) (Model, tea.Cmd) {
	s := msg.surface
	if msg.runID != m.refresh[s].runningRunID {
		return m, nil
	}
	// This run is terminal: cancel its context to release resources (the reviewer —
	// the result path must call cancel too, not only the supersede path; on an
	// already-finished context cancel is a no-op) and clear the in-flight handle.
	if c := m.refresh[s].cancel; c != nil {
		c()
	}
	m.refresh[s].cancel = nil
	if errors.Is(msg.err, context.Canceled) {
		return m, nil
	}
	if msg.wantID < m.refresh[s].commitFloor {
		return m, nil
	}
	// Details is strict latest-only. Its requests currently hard-cancel and bump
	// runID, so this is a defensive second gate against any future polite/debounced
	// request path committing a report for an abandoned focus context.
	if (s == surfaceDetails || s == surfaceTimeline || s == surfaceTemplate) && msg.wantID != m.refresh[s].wantID {
		return m, nil
	}
	staleImage := s == surfaceImage && msg.wantID != m.refresh[s].wantID
	if staleImage && msg.err != nil {
		return m.startRun(s)
	}
	// A stale picture publishes only as a frame of a held time step: when the
	// newest want differs from the run's plan in the floating time alone. A
	// picture of another chart, tab or lens is the cursor's scan leaving
	// pictures behind; it is dropped, the held picture stays, and the render
	// for the newest want still follows (S4b B4, B5).
	if staleImage {
		if now, ok := m.wheelStillKey(); !ok || now != m.wheelRunKey {
			return m.startRun(s)
		}
	}
	// A STALE text ERROR is dropped whole (the review): committing it
	// would blank/downgrade a good chart mid-burst for nothing — it isn't even
	// logged — and the still-pending debounce tick recomputes the settled
	// frame regardless (the same pending-tick guarantee the no-chain rule
	// rests on; lastCommitWantID stays behind, so the tick's idle check
	// fires). Stale SUCCESSES still commit: that's the leading frame.
	if s == surfaceText && msg.err != nil && msg.wantID != m.refresh[s].wantID {
		return m, nil
	}
	m.refresh[s].lastCommitWantID = msg.wantID
	m, commitCmd := m.commitRefresh(s, msg)
	if staleImage && m.previewActive {
		var followCmd tea.Cmd
		m, followCmd = m.startRun(s)
		return m, tea.Batch(commitCmd, followCmd)
	}
	return m, commitCmd
}

// commitRefresh applies a latest, non-superseded result's surface-specific side
// effects. For the image surface this is the ONLY place that queues bytes for
// preview.Show. A render error clears previewActive so a failed render stops
// the auto-refresh.
func (m Model) commitRefresh(s surface, msg refreshResultMsg) (Model, tea.Cmd) {
	switch s {
	case surfaceImage:
		// Route on surface (always set), not payload type: an error result may
		// carry a nil payload, and the error handling must not depend on it.
		if p, ok := msg.payload.(imagePayload); ok {
			m = m.logCommandsFor(surfaceImage, msg.wantID, p.commands)
		}
		if msg.err != nil {
			m.previewActive = false
			m.wheelPublishPending = nil
			// Error lane: the render the operator asked for failed.
			m = m.noteEphemeris(msg.err)
			reason := m.failureReason(msg.err, wheelRenderFallback)
			if ring, staged := lookup.StagedRing(msg.err); staged {
				// A ring's chart file could not be written: nothing was drawn,
				// the picture on screen is still the last one, and the line
				// names the slot (C16).
				if p, _ := msg.payload.(imagePayload); ring >= 1 && ring <= len(p.rings) {
					reason = "The chart file for " + p.rings[ring-1] + " could not be written."
				}
			}
			return m.logError("Couldn't draw the wheel. %s", reason).logWarnings(msg.err), nil
		}
		// Symmetric to the buildSurfaceCmd render-start gate: drop a SUCCESSFUL
		// render that was already in flight when the preview was disabled (a
		// render or publish error cleared previewActive). Without this, an
		// in-flight render committing after a publish failure would queue a new
		// publish and revive the broken preview (the reviewer an earlier change high).
		if !m.previewActive {
			return m, nil
		}
		// Hand the bytes to the serialized off-thread publisher rather than
		// calling preview.Show synchronously in Update (the reviewer an earlier change): keeps the
		// HTTP publish + browser launch off the render loop while
		// preserving order (one publish at a time) and latest-wins.
		img, _ := msg.payload.(imagePayload)
		m.wheelPublishPending, m.wheelPublishKey = img.bytes, m.wheelRunKey
		return m.kickWheelPublish()
	case surfaceText:
		p, _ := msg.payload.(textPayload)
		return applyTextCommit(m, p.msg)
	case surfaceDetails:
		p, _ := msg.payload.(detailsPayload)
		// The report's command becomes an argv line HERE, at the commit gate,
		// on the success and the accepted-error branch alike — a
		// result the coordinator dropped never reaches this switch, so a
		// spawn that never painted is never logged (C3).
		m = m.logCommandsFor(surfaceDetails, msg.wantID, p.commands)
		if msg.err != nil {
			// A failed replacement is not a frame. Preserve the last complete
			// report exactly as rendered and surface the failure only in Log —
			// on the error lane, beside the command that failed.
			m.details.pending = false
			m.details.waitForText = false
			m.details.timerArmed = false
			m = m.noteEphemeris(msg.err).logError("Couldn't read the full report. %s", m.failureReason(msg.err, m.consolePointer(len(p.commands) > 0))).logWarnings(msg.err)
			m.errorKeptView = true
			return m, nil
		}
		// Title and report land together. Navigation leaves the last successful
		// frame in place until this atomic replacement, so there is no visible
		// pending, blank, partial, or error frame. The scroll offset follows
		// the context, not the struct: same title keeps the reader's place.
		m = m.adoptDetailsReport(p.title, p.body)
		m.details.pending = false
		m.details.waitForText = false
		m.details.timerArmed = false
		return m, nil
	case surfaceTimeline:
		return m.commitTimeline(msg)
	case surfaceTemplate:
		return m.commitTemplate(msg)
	}
	return m, nil
}

// wheelPublishedMsg reports the outcome of an off-thread wheel publish.
type wheelPublishedMsg struct {
	path string
	err  error
	// openSeq is the Model's wheelOpenSeq when the publish started.
	openSeq uint64
}

// kickWheelPublish starts a publish if bytes are pending and none is in flight.
// Serializes publication (one preview.Show at a time) and keeps latest-wins
// (wheelPublishPending always holds the newest committed bytes; intermediate
// frames are coalesced away).
func (m Model) kickWheelPublish() (Model, tea.Cmd) {
	if m.wheelPublishing || m.wheelPublishPending == nil {
		return m, nil
	}
	bytes := m.wheelPublishPending
	m.wheelPublishPending = nil
	// A picture that waited behind another publish while the screen moved to
	// another plan is dropped, never shown (S4b B4). Plans that differ in the
	// floating time alone keep it: those are a held step's intermediate frames.
	if now, ok := m.wheelStillKey(); !ok || now != m.wheelPublishKey {
		return m, nil
	}
	m.wheelPublishing = true
	return m, publishWheelCmd(bytes, m.wheelOpenSeq)
}

// publishWheelCmd publishes the bytes to the browser preview off the Update
// loop (preview.Show via the stubbable showWheel).
func publishWheelCmd(img []byte, openSeq uint64) tea.Cmd {
	return func() tea.Msg {
		path, err := showWheel(img)
		return wheelPublishedMsg{path: path, err: err, openSeq: openSeq}
	}
}

// onWheelPublished records a finished publish and chains the next pending one.
// A publish error clears previewActive so a broken browser handoff stops
// auto-refresh.
func (m Model) onWheelPublished(msg wheelPublishedMsg) (Model, tea.Cmd) {
	m.wheelPublishing = false
	if errors.Is(msg.err, preview.ErrViewerClosed) && msg.openSeq < m.wheelOpenSeq {
		// Closed before a newer Ctrl+X: that press asked to reopen, and its
		// own publish (queued or still rendering) is the one that answers.
		return m.kickWheelPublish()
	}
	if errors.Is(msg.err, preview.ErrViewerClosed) {
		// The operator closed the viewer: that is a choice, not a failure.
		// Following stops quietly; Ctrl+X opens it again.
		m.previewActive = false
		m.wheelPublishPending = nil
		return m, nil
	}
	if msg.err != nil {
		m.previewActive = false
		m.wheelPublishPending = nil
		// Show can return a live server URL together with a browser-launch
		// error. Only then is the address worth a note: it is the one way in
		// left. (A wheel that opened names its address too, once, on its own
		// lane: logWheelOpen.)
		if msg.path != "" {
			m = m.logNote("wheel", "open it by hand at %s", msg.path)
		}
		// Error lane: the publish failed. The address above is a
		// note; the two are told apart by call site, never by text.
		return m.noteEphemeris(msg.err).logError("Couldn't show the wheel. %s", m.failureReason(msg.err, wheelPublishFallback)), nil
	}
	// The first publish of a session that answers on an address says where it
	// is. Later ones, a refresh while the operator moves time, say nothing; a
	// file path (the desktop viewers) is no address.
	// A publish that landed after the operator closed the wheel says nothing.
	if m.previewActive && m.wheelURL == "" && strings.HasPrefix(msg.path, "http://") {
		m.wheelURL = msg.path
		m = m.logWheelOpen(msg.path)
	}
	return m.kickWheelPublish()
}

// requestImage registers a new desired wheel image and starts rendering
// immediately when the image surface is idle. If an image is already rendering,
// its stale completion starts a follow-up for the latest want. No-op before a
// slot exists.
func (m Model) requestImage() (Model, tea.Cmd) {
	if m.focusedSlot() == nil {
		return m, nil
	}
	m = m.rememberWheel()
	return m.noteWant(surfaceImage, true)
}

// requestImageNow starts a wheel render immediately, bypassing the debounce —
// the explicit Ctrl+X affordance. It still flows through the coordinator's
// single-flight + cancellation, so a held Ctrl+X cannot stack concurrent
// renders. No-op before a slot exists.
func (m Model) requestImageNow() (Model, tea.Cmd) {
	if m.focusedSlot() == nil {
		return m, nil
	}
	m = m.rememberWheel()
	// Coalesce behind an in-flight render rather than cancel-and-restart it:
	// Ctrl+X key-repeat would otherwise keep superseding the active render and
	// starve it to completion-never (the reviewer an earlier change). The running render already
	// targets the focused slot; let it finish. If the focused state changed, the
	// cursor-move trigger already armed a trailing want, so the follow-up still
	// renders the latest.
	if m.refresh[surfaceImage].cancel != nil {
		return m, nil
	}
	m.refresh[surfaceImage].wantID++
	m.refresh[surfaceImage].timerID = m.refresh[surfaceImage].wantID
	return m.ensureRunningLatest(surfaceImage)
}
