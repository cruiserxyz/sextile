package ui

import (
	"fmt"
	"github.com/cruiserxyz/sextile/internal/ui/keys"
	"github.com/cruiserxyz/sextile/internal/ui/menu"
	"github.com/cruiserxyz/sextile/internal/ui/popup"
	"github.com/cruiserxyz/sextile/internal/ui/term"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

type Model struct {
	width, height int
	panels        []Panel
	focused       PanelID
	helpOpen      bool
	// help is the help popup's cursor and window, reset each time it opens.
	help keys.HelpState
	// actionsOpen is the Actions palette (⌃p, actions_popup_model.go). It is a
	// flag beside helpOpen and never a startup value: blocked reads
	// startup == startupReady, and a startup value would grey every row.
	// recentActions are the commands run through it in this run, most recent
	// first; nothing is written anywhere.
	actionsOpen   bool
	actions       actionsState
	recentActions []keys.BindingID
	startup       startupState
	// limitedViewShown is whether the Limited view notice has been offered this
	// session; it is offered once.
	limitedViewShown bool
	keys             keys.KeyMap
	cfg              Config
	// logLines is the one ordered log buffer. Every line is appended exactly
	// once, on one lane; the 6-row Log and the console each derive
	// their own view from it — see logPanelLines and consoleLines.
	logLines []logLine
	clock    Clock
	// probers is the {Full, PositionsOnly} bundle the compute Cmds
	// dispatch through. Default nil — lookup falls back to the bundled
	// astrolog wrappers. Since an earlier change (phase from Astrolog's -aa) no lookup
	// path spawns positions-only; the slot stays as the exec-budget
	// measurement seam.
	probers *lookup.Probers
	atlas   atlasModalState
	picker  chartPickerState // saved-chart library picker (Ctrl+O); an earlier change
	save    saveModalState   // save-the-floating-slot prompt (Ctrl+S / <leader>s)
	title   titleFormState   // the title form an untitled chart's pick opens
	// saveGen is bumped on every save dispatch and on Esc, so a save the
	// operator backed out of cannot land afterwards. The same shape as
	// pickGen; see applySaveDone.
	saveGen int
	// titleGen tags each retitle write, so a result that lands after its
	// form has gone is dropped rather than anchoring anything.
	titleGen int
	// abandonedSaves and abandonedTitles are the generations of writes Esc
	// walked away from while Astrolog was still writing them. Such a write
	// finishes; its result changes nothing on screen but is still logged,
	// once (the specification A8). Any other stale result is dropped.
	abandonedSaves  []int
	abandonedTitles []int
	// sessionWriteErr is the session write that failed at quit (A7).
	sessionWriteErr *SessionWriteError

	// aspectModal is the aspect chooser (`a`); an earlier change. aspectRegistry is read
	// once at startup and kept, so the chooser's list cannot shift under the
	// operator mid-session. A nil registry means the read failed and the
	// chooser refuses to open (C10).
	aspectModal    popup.Aspects
	aspectRegistry []aspect.RegistryAspect
	aspectRegErr   error
	// aspectOpenPending records that the chooser was asked for before the
	// registry had landed, so the read can open it on arrival.
	aspectOpenPending bool

	// bodyModal is the body selector (⌃b). objectRegistry is read once at
	// startup, before any restriction this program emits — see C5a and
	// loadObjectRegistryCmd. A nil registry means the read failed and the
	// selector refuses to open (C10).
	bodyModal      popup.Body
	objectRegistry []aspect.RegistryObject
	objectRegErr   error
	// awaitingRegistry is a start held for the object list: the saved session
	// names a drawn set, which only the list can turn into objects.
	awaitingRegistry bool
	// drawnKept are the saved drawn names this Astrolog's registry does not
	// list, verbatim and in the order read; drawnRaw is the whole saved list
	// while the registry is not in. Both are written back with the session.
	drawnKept, drawnRaw []string
	// fallbackSaid is the line given when a drawn set left nothing drawable or
	// nothing was ticked, so a key or a confirm does not repeat it.
	fallbackSaid string
	// ephemeris is what the startup probe learned about the data folder: which
	// objects have no ephemeris file. ephemerisSaid is the account the Log and
	// the footer last gave of it, so it is said once and again only when it changes.
	ephemeris lookup.EphemerisAvailability
	// drainSaid is the Log line already written for a selection that draws
	// nothing because its bodies have no ephemeris file (empty: none), so the
	// same account is not repeated by every refresh.
	drainSaid     string
	ephemerisSaid string
	// starsLeftSaid is the set of fixed stars the Log last said the influence
	// chart leaves out (empty: none), so it is said once per set.
	starsLeftSaid string
	// bodyOpenPending records that the selector was asked for before the
	// registry had landed, so the read can open it on arrival.
	bodyOpenPending bool
	// bodyModalSeq numbers the openings of the body popup, so a probe result
	// that outlives its opening is recognised and dropped.
	bodyModalSeq uint64
	// houseFinder is the house finder (⌃h, the specification).
	houseFinder houseFinderState
	// zodiacFinder is the zodiac finder (⌃z, the specification C8).
	zodiacFinder zodiacFinderState
	// houseDisplay is how the Bodies house column is drawn (house_display.go):
	// the whole session's choice, all slots, set live from the `, a` popup and
	// read from preferences.yaml at start. Never part of the session record.
	houseDisplay panel.HouseForm
	// houseDisplayPopup is the House display popup (`, a`): its list. The
	// popup writes no file; `, c d` keeps the form.
	houseDisplayPopup houseDisplayState
	// experimentsOn holds the Experiments table's ids that are on (experiments.go);
	// experimentsPopup is the hidden popup's list. Neither is ever written to a
	// file from here but through the popup's own command.
	experimentsOn    map[string]bool
	experimentsPopup popup.Experiments
	// chooserEntry is the screen as the aspect or body chooser found it,
	// put back on Esc.
	chooserEntry chooserEntry
	pickerGen    int // bumped on each picker open; drops stale lazy-load responses
	pickGen      int // bumped on each pick and on Esc; drops superseded chart resolves
	chart        chartViewState
	// shown is the previous scene's frame while a scene change waits for its
	// own (shown.go).
	shown      shownFrame
	chartEpoch uint64 // last epoch a text commit gave the chart
	details    detailsViewState
	// textAsked is the key the newest text want was registered for
	// (noteAsked); textAskedOK says there is one.
	textAsked   textKey
	textAskedOK bool
	// keyRun is the run of equal navigation keys the newest key belongs to.
	keyRun detailsRunState
	// keyFirst says the key being handled starts a run of equal keys (it is not
	// the repeat of one inside holdStartWindow): the reconciler starts what it
	// newly owes at once. Set by updateSynced for the length of one reconcile.
	keyFirst bool
	// mem is what each cached surface remembers (memory.go). RAM only.
	mem memory
	// pre is the idle prefetch loop (prefetch.go): not a surface, it touches no
	// surface's want or run.
	pre prefetch
	// timeline is the Details timeline tab's list and the search inputs it
	// belongs to (timeline.go). Ephemeral: never written to store.
	timeline timelineState
	// tpl is a body template tab's lines and the facts' inputs
	// (template_tab.go). Ephemeral: never written to store.
	tpl   templateState
	place location.PlaceInfo // populated once atlas-Enter resolves
	// placeGuessed says place came from the host time zone and no city has
	// been chosen or saved since; the floating Chart row then says so.
	placeGuessed bool
	lens         lookup.Lens // active calculation lens; cp2 = Hellenistic

	// slots is the workspace (canonical 2026-05-04). Constructed once
	// location resolution completes; carries the floating slot (now+place)
	// and an optional anchored slot when a natal chart is loaded. nil
	// before resolution.
	slots *store.Slots

	// cursorIdx is the focused slot's position in slots.All(). UI focus
	// state — store.Slots stays passive (no cursor inside). Cursor starts
	// on the floating slot; cycled via ↑/↓ in main mode.
	cursorIdx int

	// chartTop is the first slot row the Chart panel draws when the list
	// windows. Presentation state like cursorIdx: never stored, never saved.
	chartTop int
	// bodiesColStart is the first column the comparing Bodies window drew
	// last; it holds until the focus passes an edge. Presentation state, never
	// stored.
	bodiesColStart int

	// bodyCursorIdx is the Bodies panel's row cursor for the FOCUSED slot.
	// Like cursorIdx it is ephemeral presentation state; persistent body
	// selection lives per slot in store.Slots. The row each slot was left on
	// is kept in bodyRows (focusSlot), so returning to a slot puts the cursor
	// back where it was (M8).
	bodyCursorIdx int

	// bodyTop is the display row the Bodies list is drawn from (the rule counts
	// as a row), remembered like chartTop so a window can hold still while the
	// cursor moves inside it, and so a page op can leave the cursor at the top
	// of a scrolled window. Presentation state: bodyWindowStart is the one rule
	// for it, and it is re-settled wherever bodyCursorIdx is clamped.
	bodyTop int

	// bodyRows is the Bodies row each anchored or floating slot was last left
	// on, keyed by slot so a reorder or a removal carries it correctly. UI
	// state, never store. Replaced whole on write — a Model is copied by value
	// and a shared map would let an older copy see a newer one's cursor.
	bodyRows map[*store.Slot]int

	// base is the Aspects tab's memory (operator, 2026-10-22: "as soon as the
	// tab changes it will stay at that tab"; 2026-10-24: the tab picks the
	// base): the chart the cursor's chart is read against, nil for the own tab.
	// Never the floating slot. `[` / `]`, Enter on a timeline row, a deselect
	// or a removal write it, and one arrival: landing on the base swaps it for
	// the chart you came from. A slot pointer, so it follows a chart through
	// ⌃k/⌃j.
	base *store.Slot
	// at is the row the cursor last arrived on, so the next arrival knows the
	// chart it came from (noteArrival).
	at *store.Slot

	// searchBefore is what the open `/` line found when it opened; Esc puts it back.
	searchBefore searchBefore

	// sessionKept is set when startup refused session.yaml and could not move it
	// aside (or, for a hand-built read with no keep report, for a line too long
	// to read): the file is the astrologer's, so no session is written.
	sessionKept bool

	// sessionForeign is set when the launch runs the general session and the
	// file it read names another charts folder (applyStartupPerimeter): the
	// file is that folder's, so this run never writes the general session. It
	// lasts for the whole launch -- no transition clears it, not saving or
	// opening a named session, not `<leader> n`, not a city change -- and the
	// path is never read again, so a file deleted or replaced meanwhile is
	// still not written. A named session keeps saving as ever.
	sessionForeign bool

	// named is the named session this window runs; zero while it runs the
	// general one (sessions_popup_model.go). sessions is the Sessions list.
	named    namedRun
	sessions sessionsPopupState
	// sessionGen numbers the named-session opens; a load that is not the
	// latest one is dropped.
	sessionGen int

	// pendingRechecks maps the slot a startup re-check was ISSUED against to the
	// slot that holds its chart now, across the city changes that rebuild every
	// slot as a new object. It holds only re-checks still pending — an entry goes
	// when its re-check lands or its slot is removed — and every value is one hop,
	// so it is bounded by the re-checks in flight, not by the rebuilds. Replaced
	// whole on write, like bodyRows.
	pendingRechecks map[*store.Slot]*store.Slot

	// targetFloatingTime is the operator-controlled time-step destination
	// (mutated on every press of y/Y o/O w/W d/D h/H). slots.floating.Time
	// is the LAST COMMITTED time — what bodies/aspects were actually
	// computed for and what the chart panel header displays. Separating
	// the two means the header never races ahead of the chart: each
	// recompute commits its result's TransitTime to slots.floating.Time
	// so header + bodies + aspects advance together. Held-key feels like
	// a sweep at compute rate, not a header dragging behind no bodies.
	// Set to the slot's floating Time
	// on construction; advanceTime mutates it; the text commit
	// (applyTextCommit) never touches it.
	targetFloatingTime time.Time

	// pendingPerimeter holds the cold-start perimeter read until the atlas
	// also loads. Resolution runs once both signals are in. cp3.
	pendingPerimeter *ingest.StartupPerimeterReadMsg

	// previousFloatingTime backs the one-level n/N toggle: `n` jumps the
	// floating target to now (stashing the prior target here); `N` swaps the
	// target with this stash. It tracks the TARGET (operator intent), never the
	// lagging committed slot.Time, so it stays correct mid-held-key-burst.
	previousFloatingTime time.Time

	// floatingDetached is false while the floating chart follows the clock
	// (attached: its title reads `~ Now`) and true once the operator has moved
	// it (`~`). Its zero value is attached, which is how a session starts, and
	// how one comes back from a restore. A time-step key, `N` and Enter on a
	// timeline row detach it; `n` attaches it again; nothing else changes it.
	floatingDetached bool

	// nowTickGen numbers the minute timers armed for an attached floating
	// chart; a tick of any other number is a retired timer (live_tick.go).
	nowTickGen uint64
	// tickWant is the text surface's want the latest minute tick registered,
	// and quiet is, per surface, the want a tick's work registered there. A
	// result for either writes no argv line to the Log (live_tick.go).
	tickWant uint64
	quiet    [numSurfaces]uint64

	// previewActive is true once Ctrl+X has opened the external wheel-image
	// browser preview. While active, a change to the focused slot's chart (cursor
	// move, or a recompute commit when the floating slot is focused)
	// re-renders the image so the open browser refreshes in place.
	previewActive bool

	// wheelURL is the address the Log has named for this session's wheel
	// server, "" until the first successful publish of a server that answers on
	// one. It is what keeps the notice to one line however often the wheel
	// refreshes; closing the wheel clears it, so the next open says where it is
	// again.
	wheelURL string

	// refresh holds the per-surface refresh coordinators (image, text, Details): a
	// latest-wins single-flight with cancellation (see internal/ui/refresh.go).
	// The image surface (S3) replaced the old wheelRendering/wheelPending
	// single-flight; the text surface (S4) replaced the retired
	// computeGen/dispatchSeq/lastCommittedSeq ordering counters.
	refresh [numSurfaces]refreshState

	// rows is the refresh table: one row per surface (row_spawns.go
	// defaultRows), set once in the constructor. The rows carry the spawns a
	// test replaces, so a stub reaches the Model that holds it and no other.
	rows [numSurfaces]surfaceRow

	// wheelPublishing / wheelPublishPending serialize the wheel-image PUBLISH
	// step (preview.Show — HTTP publish + browser launch) OFF the Update
	// thread. The coordinator already guarantees only the LATEST render commits;
	// commit hands the bytes here. At most one publish runs at a time (so
	// concurrent publications cannot race), and the newest committed
	// bytes always win — bytes arriving mid-publish queue in wheelPublishPending
	// and publish when the current one finishes. (the reviewer an earlier change: keep publication
	// off the synchronous Update path.)
	wheelPublishing     bool
	wheelPublishPending []byte
	// wheelPublishKey is the stillKey of the plan the pending bytes were
	// rendered for. A queued picture is published only while the screen still
	// asks for that plan, time apart (S4b B4): one that waited behind a publish
	// while the cursor moved on is a picture of a chart the screen has left.
	wheelPublishKey string
	// wheelOpenSeq counts Ctrl+X presses. A publish carries the count it was
	// started under, so a "viewer closed" answer older than the newest Ctrl+X
	// cannot cancel the reopen that press asked for.
	wheelOpenSeq uint64

	// wheelKey is the inputs of the wheel render last asked for (wheelPlan.key),
	// wheelKeyed whether one was. refreshWheelIfActive asks for nothing when the
	// wheel it would ask for is that one (C9).
	wheelKey   string
	wheelKeyed bool
	// wheelRunKey is the stillKey of the plan the wheel run in flight was built
	// for, so a result that arrives after the want moved on can tell whether
	// it is a frame of the same animation or a picture of a chart left behind.
	wheelRunKey string

	// leaderKey is the key that arms a pending sequence; leaderTimeout is
	// how long that sequence waits for its next key. Both are configuration
	// (preferences.yaml `keys:`), resolved once at startup by
	// applyKeyConfig — the leader is deliberately NOT a key.Binding, since
	// the resolver dispatches it and key.Matches never sees it.
	leaderKey     string
	leaderTimeout time.Duration

	// leaderMap is the compiled-in prefix tree; leaderBindings is the same
	// map in the order the help modal lists it. No user-writable map ships
	// yet, so both are built once and never mutated.
	leaderMap      *keys.LeaderNode
	leaderBindings []keys.LeaderBinding

	// rowStripeNoted is set once the retired display.row_stripe has been
	// reported, so applying the display block again writes no second note.
	rowStripeNoted bool

	// wheelFormatRefused is the wheel_format value the Log has already refused,
	// so the same word is not refused again at every Ctrl+X. A valid value
	// clears it.
	wheelFormatRefused string

	// leader is the pending prefix: armed or not, what has been typed, and
	// the generation a timeout tick must match to act.
	leader leaderState

	// console is the Log-as-console mode (`,l`): a mode that owns the
	// keyboard, not a focused panel. Ephemeral — never written to the
	// preferences or session files, zeroed on close.
	console consoleState
	// logFocus is the Log panel holding the keyboard (log_focus.go). A mode
	// beside the console: m.focused is never the Log.
	logFocus logFocusState

	// yank is the copy confirmation shown in the key bar's right slot.
	yank yankState
	// resetArmed is the yank generation of the "`, n` again" notice that armed
	// the second press of `, n`; 0 when nothing is armed (reset_session.go).
	resetArmed uint64
	// loading is when the active Details tab started waiting (loading_overlay.go).
	loading loadingState
	// errorUnseen records an error line appended since the console was last
	// opened. The key bar shows the mark while it is set and no Log panel is
	// on screen (the specification).
	errorUnseen bool
	// errorKeptView is set when the unseen error came from a refresh that
	// left the previous view on screen, so the mark says so.
	errorKeptView bool

	// lastUnavailable is the set of uncomputable objects the Log has already
	// named, so reportUnavailable speaks on a CHANGE rather than on every
	// frame. Ephemeral: never persisted, and a restart re-reports.
	lastUnavailable []string

	// frames is each chart's house system and where it came from
	// (house_frames.go).
	frames frameState

	// cantStart is what the whole-screen "Sextile can't start" box reports
	// (cant_start.go). Ephemeral: a fix needs a restart, so nothing clears it.
	cantStart cantStartState
}

func NewModel(cfg Config) Model {
	return NewModelWithClock(cfg, RealClock{})
}

// NewModelWithClock is the test-friendly constructor: invariants tests
// pass FrozenClock(t) to make first-frame goldens deterministic.
func NewModelWithClock(cfg Config, clock Clock) Model {
	m := Model{
		panels:  defaultPanels(),
		focused: PanelChart,
		keys:    keys.DefaultKeys(),
		cfg:     cfg,
		rows:    defaultRows(),
		startup: startupBootstrapping,
		clock:   clock,
		lens:    lookup.Hellenistic,
		// The compiled-in defaults. preferences.yaml can replace the leader
		// and the timeout at startup; the map is not user-writable yet.
		leaderKey:      defaultLeaderKey,
		leaderTimeout:  defaultLeaderTimeout,
		leaderMap:      mustDefaultLeaderMap(),
		leaderBindings: defaultLeaderMap(),
	}
	if text, ok := launchNote(cfg.Launch); ok {
		// First, so the large log always says what ran.
		m = m.logTrace("launcher", "%s", text)
	}
	if cfg.Migration != "" {
		m = m.logTrace("state", "%s", cfg.Migration)
	}
	// A launch with --session runs that named session, whose lock main took;
	// one it could not open says why, once.
	if cfg.NamedSession != nil {
		m.named = namedRun{file: cfg.NamedSession.File(), name: cfg.NamedSessionName, lock: cfg.NamedSession}
	}
	if cfg.SessionNote != "" {
		m = m.logNote("session", "%s", cfg.SessionNote)
	}
	if cfg.InstanceLockUnsupported {
		// Said once, that a second copy cannot be told apart from this one
		// (the specification C6), in the large log only: it is about Sextile's
		// working, not the operator's charts.
		m = m.logTrace("lock", "a second copy cannot be detected on this folder")
	}
	return m
}

// launchNote is the Log's first line for the launcher's SEXTILE_LAUNCH value:
// `docker <image>` or `no-docker <describe>`. The value is only shown, never
// interpreted, and only in the shape the launcher writes: the name after the
// word is ASCII letters, digits and . _ : / @ + -, so no control, format or
// bidirectional character reaches the Log. Any other value says nothing.
func launchNote(v string) (string, bool) {
	if rest, ok := strings.CutPrefix(v, "docker "); ok && launchNameRE.MatchString(rest) {
		return "in Docker, image " + rest, true
	}
	if rest, ok := strings.CutPrefix(v, "no-docker "); ok && launchNameRE.MatchString(rest) {
		return "without Docker, build " + rest, true
	}
	return "", false
}

// launchNameRE is the image or build name launchNote accepts.
var launchNameRE = regexp.MustCompile(`^[A-Za-z0-9._:/@+-]+$`)

func (m Model) Init() tea.Cmd {
	// A launch the instance lock refused reads nothing and starts nothing.
	if m.cantStart.locked() {
		return nil
	}
	// The ephemeris probe runs before anything casts a chart: the first argv
	// must already leave out an object whose file is missing, and the host's
	// default object set it reads is the only answer to what a chart draws.
	// It is bounded (ephemerisProbeDeadline), and a failed or late one is a
	// can't-start (cantStart.ephemerisProbe), never a start on a guess. Nothing
	// else needs to wait for it: the reads below start only after it has
	// answered, and its message is handled before theirs.
	// Sequence, not Batch: the probe is a PATH lookup that starts no process,
	// and running it first is what makes "Astrolog is not installed" the FIRST
	// line rather than a line that may or may not beat the compute failure it
	// explains. Everything after it still starts together.
	return tea.Sequence(probeAstrologCmd(), loadEphemerisCmd(), tea.Batch(m.perimeterCmd(), loadAtlasCmd(), loadAspectRegistryCmd(), loadObjectRegistryCmd()))
}

// Update is the program's one entry. The work a minute tick starts is quiet in
// the Log: whatever surface wants it registers are marked once the message has
// been handled (hush).
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !m.isTickWork(msg) {
		return m.updateSynced(msg)
	}
	before := m.surfaceWants()
	next, cmd := m.updateSynced(msg)
	if nm, ok := next.(Model); ok {
		next = nm.hush(before)
	}
	return next, cmd
}

func (m Model) updateSynced(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	nm, ok := next.(Model)
	if !ok {
		return next, cmd
	}
	// The reconciler: every pane that shows a computed result is owed the
	// result for the key it wants now, whatever entry point changed it
	// (reconcile.go). Before the run of equal keys is judged, because a request
	// it starts counts as the key having changed what Details describes.
	// A key that starts a run of equal keys has the reconciler start what it
	// newly owes at once; a repeat inside the run keeps the debounce.
	k, isKey := msg.(tea.KeyMsg)
	nm.keyFirst = isKey && !isBurst(k) && !nm.keyJoinsRun(k)
	nm, rcmd := nm.reconcile()
	nm.keyFirst = false
	// Every key is judged for the run of equal keys (followKeyRun). A burst is
	// replayed through Update one key at a time, which counts there.
	if isKey && !isBurst(k) {
		nm = nm.followKeyRun(k, nm.keyRun.seq != m.keyRun.seq)
	}
	// The idle prefetch comes last: whatever the message owed has been started,
	// and a run of the operator's own stops a call that is out.
	nm, pcmd := nm.prefetchAfter(msg, m.startup == startupReady)
	// The loading overlay's timer goes last, after the work this message
	// started: the program runs them all at once, but a test's pump waits on a
	// batch in order and must not stall on the timer before the work.
	nm, lcmd := nm.syncLoading()
	switch {
	case rcmd == nil && pcmd == nil && lcmd == nil:
		return nm, cmd
	}
	return nm, tea.Batch(cmd, rcmd, pcmd, lcmd)
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	msg = deleteFromKeypadNumericMode(msg)
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		wasDetailsVisible := m.detailsVisible()
		m.width, m.height = msg.Width, msg.Height
		m = m.settleBodiesColStart()
		nowDetailsVisible := m.detailsVisible()
		m = m.clampDetailsOffset()
		// A console that was too small to have an inner height could not
		// scroll its window onto a moving cursor. Widening is where that
		// gets put right.
		m = m.clampConsole()
		m = m.settleLogFocus()
		m = m.settleBodyTop()
		if wasDetailsVisible && !nowDetailsVisible {
			m = m.suspendDetails()
			return m, nil
		}
		if !wasDetailsVisible && nowDetailsVisible {
			var detailsCmd, timelineCmd tea.Cmd
			m, detailsCmd = m.requestDetails()
			m, timelineCmd = m.syncTimeline()
			return m, tea.Batch(detailsCmd, timelineCmd)
		}
		if nowDetailsVisible {
			m = m.clampTimelineCursor()
		}
		return m, nil
	case experimentWrittenMsg:
		return applyExperimentWritten(m, msg), nil
	case ingest.StartupPerimeterReadMsg:
		var cmd tea.Cmd
		m, cmd = applyStartupPerimeter(m, msg)
		return m, cmd
	case AstrologProbedMsg:
		var cmd tea.Cmd
		m, cmd = applyAstrologProbe(m, msg)
		return m, cmd
	case EphemerisProbedMsg:
		return applyEphemerisProbed(m, msg)
	case ObjectRegistryLoadedMsg:
		var cmd tea.Cmd
		m, cmd = applyObjectRegistryLoaded(m, msg)
		return m, cmd
	case bodyProbeRestMsg:
		return applyBodyProbeRest(m, msg)
	case bodyProbeResultMsg:
		return applyBodyProbeResult(m, msg)
	case AspectRegistryLoadedMsg:
		var cmd tea.Cmd
		m, cmd = applyAspectRegistryLoaded(m, msg)
		return m, cmd
	case AtlasLoadedMsg:
		var cmd tea.Cmd
		m, cmd = applyAtlasLoaded(m, msg)
		return m, cmd
	case DefaultCitySavedMsg:
		return applyDefaultCitySaved(m, msg)
	case AtlasPickedMsg:
		var cmd tea.Cmd
		m, cmd = applyAtlasPicked(m, msg)
		return m, cmd
	case ChartsLoadedMsg:
		var cmd tea.Cmd
		m, cmd = applyChartsLoaded(m, msg)
		return m, cmd
	case ChartPickedMsg:
		var cmd tea.Cmd
		m, cmd = applyChartPicked(m, msg)
		return m, cmd
	case ChartResolvedMsg:
		var cmd tea.Cmd
		m, cmd = applyChartResolved(m, msg)
		return m, cmd
	case SaveChartDoneMsg:
		var cmd tea.Cmd
		m, cmd = applySaveDone(m, msg)
		return m, cmd
	case NamedSessionLoadedMsg:
		var cmd tea.Cmd
		m, cmd = applyNamedSessionLoaded(m, msg)
		return m, cmd
	case RenameReadMsg:
		return applyRenameRead(m, msg)
	case TitleWrittenMsg:
		var cmd tea.Cmd
		m, cmd = applyTitleWritten(m, msg)
		return m, cmd
	case SessionChartRecheckedMsg:
		var cmd tea.Cmd
		m, cmd = applySessionChartRecheck(m, msg)
		return m, cmd
	case FirstFrameReadyMsg:
		// The exported text-surface result: convert into the coordinator's
		// internal shape so all ordering policy (superseded-run drop, cancel
		// drop, identity floor, leading-frame commit) lives in one place.
		return m.onRefreshResult(refreshResultMsg{
			surface: surfaceText,
			runID:   msg.RunID,
			wantID:  msg.WantID,
			payload: textPayload{msg: msg},
			err:     msg.Err,
		})
	case nowTickMsg:
		return m.onNowTick(msg)
	case leaderTimeoutMsg:
		return m.onLeaderTimeout(msg)
	case yankFlashMsg:
		return m.onYankFlash(msg)
	case loadingTickMsg:
		return m.onLoadingTick(msg)
	case prefetchIdleMsg:
		return m.onPrefetchIdle(msg)
	case prefetchDoneMsg:
		return m.onPrefetchDone(msg)
	case refreshTickMsg:
		return m.onRefreshTick(msg)
	case refreshResultMsg:
		return m.onRefreshResult(msg)
	case wheelPublishedMsg:
		return m.onWheelPublished(msg)
	case tea.KeyMsg:
		// While the can't-start box is up nothing else owns the keyboard.
		if m.cantStart.any() {
			return m.cantStartKey(msg)
		}
		// Ctrl+C quits from every owner, a pending leader included, before
		// any of them sees the key (C10).
		if key.Matches(msg, m.keys.ForceQuit) {
			return m.quit()
		}
		// Several keys read at once are pressed one at a time, each through
		// whichever owner has the keyboard by then (C14).
		if isBurst(msg) {
			return m.replayBurst(msg)
		}
		return m.dispatchKey(msg)
	case tea.MouseMsg:
		return m.updateMouse(msg)
	}
	return m, nil
}

// dispatchKey hands one key to whatever has the keyboard.
func (m Model) dispatchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Any key but the two that make `, n` disarms the reset warning.
	if m.resetArmed != 0 {
		starts := !m.leader.pending && m.leaderKey != "" && msg.String() == m.leaderKey
		ends := m.leader.pending && msg.String() == "n"
		if !starts && !ends {
			m.resetArmed = 0
		}
	}
	// A layout that lost the Log panel (a third chart widened the left column)
	// leaves nothing to hold the keyboard: the focus ends before the key runs.
	if m.logFocus.active && !m.logOnScreen() {
		m.logFocus = logFocusState{}
	}
	switch m.currentOwner() {
	case ownerCity:
		return updateAtlasModalKey(m, msg)
	case ownerOpenChart:
		return updateChartModalKey(m, msg)
	case ownerBodies:
		return applyBodyModalKey(m, msg)
	case ownerAspects:
		return applyAspectModalKey(m, msg)
	case ownerHouses:
		return updateHouseFinderKey(m, msg)
	case ownerZodiac:
		return updateZodiacFinderKey(m, msg)
	case ownerDisplay:
		return updateHouseDisplayKey(m, msg)
	case ownerExperiments:
		return updateExperimentsKey(m, msg)
	case ownerSessions:
		return updateSessionsKey(m, msg)
	case ownerSessionName:
		return updateSessionNameKey(m, msg)
	case ownerSaveChart:
		return updateSaveModalKey(m, msg)
	case ownerChartTitle:
		return updateTitleFormKey(m, msg)
	case ownerHelp:
		return m.updateHelpKey(msg)
	case ownerActions:
		return updateActionsKey(m, msg)
	case ownerConsole:
		return m.updateConsoleKey(msg)
	case ownerLog:
		return m.updateLogKey(msg)
	case ownerDetails:
		// The leader works inside Details too, with the main screen's
		// sequences.
		if m.leader.pending {
			return m.updateLeaderKey(msg)
		}
		// An open search line takes every key, the comma included.
		if m.leaderKey != "" && msg.String() == m.leaderKey && !(m.timeline.searching && m.timelineSearchLine()) && !m.detailsReportSearchOpen() {
			return m.armLeader()
		}
		return m.updateDetailsKey(msg)
	}
	// A pending leader takes the next key ahead of the main table — that is
	// what lets a configured leader shadow a binding.
	if m.leader.pending {
		return m.updateLeaderKey(msg)
	}
	if m.leaderKey != "" && msg.String() == m.leaderKey {
		return m.armLeader()
	}
	return m.updateMainKey(msg)
}

func (m Model) updateMainKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.dispatchMainBinding(keys.MatchBinding(msg, m.available(ownerMain, mainBindings(m.keys))), msg)
}

// dispatchMainBinding is what a main binding does once a key has matched it. A
// real key reaches it through updateMainKey. The cases here are the ones that
// need the key message (the time steps read the letter's case for their
// direction) or are motion: focus, tabs, rows, paging and scrolling. Every
// other id is a command and runs through runAction.
func (m Model) dispatchMainBinding(id keys.BindingID, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if p, ok := panelDigits[id]; ok {
		return m.pressPanelDigit(p)
	}
	switch id {
	case keys.BindingNextView:
		return m.setFocusedPanel(m.nextFocus())
	case keys.BindingPrevView:
		return m.setFocusedPanel(m.prevFocus())
	case keys.BindingTabNext:
		return m.cycleTab(m.focusedView(), +1)
	case keys.BindingTabPrev:
		return m.cycleTab(m.focusedView(), -1)
	case keys.BindingDetailsTab:
		// `0` is Details' digit, and Details is not in the ring: it cycles
		// Details' tabs from anywhere and never focuses it (D14, C33, C40).
		return m.cycleTab(PanelDetails, +1)
	case keys.BindingSlotCycleUp:
		return m.cycleFocusedRow(-1)
	case keys.BindingSlotCycleDown:
		return m.cycleFocusedRow(+1)
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
	case keys.BindingScrollDown:
		return m.scrollDetailsTab(detailsScrollStep), nil
	case keys.BindingScrollUp:
		return m.scrollDetailsTab(-detailsScrollStep), nil
	case keys.BindingPageDown:
		if m.mainPages() {
			return m.pageFocusedList(menu.PageDown)
		}
		return m.scrollDetailsTab(detailsScrollStep), nil
	case keys.BindingPageUp:
		if m.mainPages() {
			return m.pageFocusedList(menu.PageUp)
		}
		return m.scrollDetailsTab(-detailsScrollStep), nil
	case keys.BindingPageHome:
		return m.pageFocusedList(menu.PageHome)
	case keys.BindingPageEnd:
		return m.pageFocusedList(menu.PageEnd)
	case keys.BindingActions:
		// The door to the commands, not one of them (actions_popup_model.go).
		return openActions(m)
	}
	// Everything else is a command, which runs by its id (actions.go).
	if tm, cmd, ok := m.runAction(id); ok {
		return tm, cmd
	}
	return m, nil
}

// pageFocusedList routes a paging key to the list the focused panel owns. Each
// owner calls menu.PageCursor (cursor_page.go) with its own total and the number of
// cursor rows its render shows, and stores both results.
func (m Model) pageFocusedList(op menu.PageOp) (Model, tea.Cmd) {
	switch m.focused {
	case PanelChart:
		return m.pageChart(op)
	case PanelBodies:
		return m.pageBodies(op)
	}
	return m, nil
}

// pageChart pages the Chart panel's cursor over the workspace's slots, with the
// Chart window (chartTop) as the offset. The focused slot follows as it does for
// an arrow step, so Bodies and Details follow it; chartWindow finds nothing to
// correct afterwards, because the page op already holds the cursor in the window.
func (m Model) pageChart(op menu.PageOp) (Model, tea.Cmd) {
	if m.slots == nil {
		return m, nil
	}
	total := len(m.slots.All())
	if total < 1 {
		return m, nil
	}
	idx, top := menu.PageCursor(m.cursorIdx, m.chartTop, total, m.geometry()[PanelChart].h-2, op)
	m.chartTop = top
	if idx == m.cursorIdx {
		return m, nil
	}
	m = m.focusSlot(idx)
	return m.afterFocusChange()
}

// timeStepUnit names the calendar/duration granularity a step key advances.
// Per the design notes:113-122: y/Y year, o/O month, w/W week,
// d/D day, h/H hour, m/M minute.
type timeStepUnit int

const (
	unitYear timeStepUnit = iota
	unitMonth
	unitWeek
	unitDay
	unitHour
	unitMinute
)

// stepDirectionFromRune returns +1 (forward) or -1 (backward) based on
// whether the pressed key's rune is uppercase. Per pitch.md convention:
// lowercase = forward, uppercase = backward. Defaults to forward when
// the message carries no rune (defensive — shouldn't happen for the
// time-step bindings, which only accept rune keys).
func stepDirectionFromRune(msg tea.KeyMsg) int {
	if len(msg.Runes) > 0 && unicode.IsUpper(msg.Runes[0]) {
		return -1
	}
	return +1
}

// advanceTime mutates the time-step TARGET by the given step and registers
// the change with the text surface's refresh coordinator: the first press of
// a burst computes immediately (leading edge), further presses while that
// run is in flight coalesce onto the trailing debounce tick, which computes
// the settled final target — "first + latest, fast". No-op if no workspace
// exists yet (cold-start, pre-resolution).
//
// Everywhere by design (D37, operator 2026-10-13): a step key acts on the
// FLOATING slot whichever slot the cursor is on, and the cursor, the selection
// and the active tab stay where they are. The anchored slots' Time is the
// immutable natal moment and never moves; only a panel that names the key for
// itself (the console, a popup, the `/` line) wins, and those never reach here.
func (m Model) advanceTime(unit timeStepUnit, direction int) (Model, tea.Cmd) {
	if m.slots == nil {
		return m, nil
	}
	// The round-3 fix: mutate the TARGET (m.targetFloatingTime), NOT
	// slots.floating.Time. The slot's Time is the "what's currently
	// displayed" truth, advanced only when a matching compute commits
	// (applyTextCommit). The target is the operator's intent, advanced on
	// every keypress and read by buildTextCmd (via transitTimeFromWorkspace)
	// when a run actually starts. Separation guarantees the header (which
	// renders slots.floating.Time) stays synchronized with bodies and
	// aspects across the burst.
	years, months, days, dur := stepDelta(unit, direction)
	target := advanceTimeForPlace(m.targetFloatingTime, m.slots.Floating().Location, years, months, days, dur)
	// Astrolog cannot name a day before Julian Day 0: a step that would cross it
	// leaves the time where it is and says why in one plain line
	// (the specification C10).
	if lookup.BeforeJulianDayZero(target) {
		return m.logNote("time", "stopped at Julian Day 0, the earliest moment Astrolog can cast"), nil
	}
	m.details.pressedSet = false
	m.floatingDetached = true
	return m.setFloatingTarget(target)
}

// floatingTitle is the title the floating slot's Chart row carries: `~ Now`
// while the chart follows the clock, `~` once it has been moved. Empty before a
// workspace exists.
func (m Model) floatingTitle() string {
	if m.slots == nil {
		return ""
	}
	if m.floatingDetached {
		return "~"
	}
	return "~ Now"
}

// setFloatingTarget is the time-step path every floating-slot move takes: the
// step keys and Enter on a timeline row (C21) alike. It sets the TARGET, fences
// Details until the matching frame commits, and registers the want with the
// text surface; the committed slot Time, Bodies, Aspects and — through the
// commit's refreshWheelIfActive — the wheel follow from that one compute.
func (m Model) setFloatingTarget(target time.Time) (Model, tea.Cmd) {
	m.targetFloatingTime = target
	m = m.waitDetailsForText()
	m, textCmd := m.noteWant(surfaceText, true)
	if !m.previewActive {
		return m, textCmd
	}
	m, imageCmd := m.requestImage()
	return m, tea.Batch(textCmd, imageCmd)
}

// advanceTimeForPlace is the DST-aware calendar+duration arithmetic
// store.Slots.AdvanceFloating uses, extracted so advanceTime can apply
// it to m.targetFloatingTime without going through the store mutator.
// Calendar arithmetic (years/months/days) runs in the floating slot's
// local timezone so "+1 month at the same local clock time" survives
// DST boundaries. dur is added as a fixed wall-clock duration.
func advanceTimeForPlace(t time.Time, place location.PlaceInfo, years, months, days int, dur time.Duration) time.Time {
	if years == 0 && months == 0 && days == 0 && dur == 0 {
		return t
	}
	if years != 0 || months != 0 || days != 0 {
		if loc, err := time.LoadLocation(place.TZ); err == nil {
			t = t.In(loc).AddDate(years, months, days)
		} else {
			t = t.AddDate(years, months, days)
		}
	}
	if dur != 0 {
		t = t.Add(dur)
	}
	return t
}

// stepDelta translates a (unit, direction) pair into AddDate + Duration
// arguments suitable for store.Slots.AdvanceFloating. Centralizes the
// magnitude-per-unit decision so the unit table is one place.
func stepDelta(unit timeStepUnit, direction int) (years, months, days int, dur time.Duration) {
	switch unit {
	case unitYear:
		return direction, 0, 0, 0
	case unitMonth:
		return 0, direction, 0, 0
	case unitWeek:
		return 0, 0, 7 * direction, 0
	case unitDay:
		return 0, 0, direction, 0
	case unitHour:
		return 0, 0, 0, time.Duration(direction) * time.Hour
	case unitMinute:
		return 0, 0, 0, time.Duration(direction) * time.Minute
	}
	return 0, 0, 0, 0
}

// cycleSlotCursor moves the focus cursor across slots.All() by `step`
// (+1 = next, -1 = previous), stopping at the ends (C12). No-op before slots
// are constructed (cold-start, pre-resolution). Logs only real
// transitions — a single-slot workspace stays silent.
// cycleSlot moves the focused-slot cursor and reloads the panels for the newly
// focused slot: a text recompute (Bodies + single-mode aspects always track the
// focused slot under S5b) plus, if the wheel preview is open, an image refresh.
//
// The text recompute goes through bumpIdentity, NOT noteWant: a focus change
// alters WHICH slot's chart the surface computes, so a still-in-flight
// old-focus result must never paint. noteWant keeps the running run's runID
// when a run is already in flight, so its stale result would commit as a
// "leading frame" — acceptable for a time change (a slightly-earlier moment),
// WRONG for a focus change (the other slot's bodies). bumpIdentity cancels the
// in-flight run, advances the runID, and raises commitFloor so the stale frame
// is fenced (a review finding). Image staleness is handled by the image surface's
// own drop-stale-frames gate, so refreshWheelIfActive suffices there.
// stateOnly applies one panel action from another owner — Details' row
// actions — for its STATE only, with the focus left where it is (operator,
// 2026-10-24: "UI actions in regards to panels MUST be user-induced ONLY";
// it retires D36's press with the panel focused for the press). The action is
// the one the panel's own key runs; the run it would have started is released
// and the coordinator's bookkeeping — every surface's refresh state and the
// wheel last asked for — is put back as it was, so an action that takes
// several steps costs no intermediate compute and its last step pays the one
// recompute, as a single keypress would. A caller that applied anything
// therefore ends with afterFocusChange or afterComparisonChange; a step left
// standing alone would show the new state against the old frame. Dropping the
// step's command without this restore is the trap: the surface would read as
// in flight with no run to deliver, and a later noteWant would wait forever.
func (m Model) stateOnly(act func(Model) (Model, tea.Cmd)) Model {
	refresh, wheelKey, wheelKeyed := m.refresh, m.wheelKey, m.wheelKeyed
	m, _ = act(m)
	// A run the step started has a context nobody will drive: release it.
	for s := range m.refresh {
		if m.refresh[s].runningRunID != refresh[s].runningRunID && m.refresh[s].cancel != nil {
			m.refresh[s].cancel()
		}
	}
	m.refresh, m.wheelKey, m.wheelKeyed = refresh, wheelKey, wheelKeyed
	return m
}

// walkCursorTo steps the Chart cursor by the Chart panel's own Up/Down action
// (stateOnly) until it is on list index idx; the steps clamp at the ends, so a
// step that moves nothing stops the walk. The focus never moves; the caller's
// last step pays the one recompute.
func (m Model) walkCursorTo(idx int) Model {
	for m.slots != nil && m.cursorIdx != idx {
		step := +1
		if idx < m.cursorIdx {
			step = -1
		}
		before := m.cursorIdx
		m = m.stateOnly(func(m Model) (Model, tea.Cmd) { return m.cycleSlot(step) })
		if m.cursorIdx == before {
			break
		}
	}
	return m
}

// walkBodyCursorTo steps the Bodies cursor by the Bodies panel's own Up/Down
// action (stateOnly) until it is on o's row, the twin of walkCursorTo. An
// object the rows do not list (hidden by the body selector) moves nothing; a
// step that moves nothing stops the walk. The row is then remembered for the focused slot, so
// entering Bodies later lands on it.
func (m Model) walkBodyCursorTo(o aspect.Object) Model {
	rows := m.shownChart()
	target := -1
	for i := 0; i < panel.BodyRowCount(rows.positions, rows.cusps); i++ {
		if ro, ok := panel.ObjectAtCursor(rows.positions, rows.cusps, i); ok && ro.Number == o.Number {
			target = i
			break
		}
	}
	if m.slots == nil || target < 0 {
		return m
	}
	for m.bodyCursorIdx != target {
		step := +1
		if target < m.bodyCursorIdx {
			step = -1
		}
		before := m.bodyCursorIdx
		m = m.stateOnly(func(m Model) (Model, tea.Cmd) { return m.cycleBodyRow(step) })
		if m.bodyCursorIdx == before {
			break
		}
	}
	return m.rememberBodyRow()
}

func (m Model) cycleSlot(step int) (Model, tea.Cmd) {
	old := m.cursorIdx
	m = m.cycleSlotCursor(step)
	if m.cursorIdx == old {
		return m, nil // single-slot workspace or nil slots — nothing moved
	}
	return m.afterFocusChange()
}

// afterFocusChange is what a new focused slot costs: the previous slot's
// Bodies payload is dropped (it would sit under the new store-selection target
// while its recompute is in flight), Details waits for the text, the text
// surface recomputes through bumpIdentity, and an open wheel follows.
func (m Model) afterFocusChange() (Model, tea.Cmd) {
	// Bodies and Aspects keep painting the scene they last swapped to until the
	// new frame commits; the blanking below is the live state only.
	m = m.holdShown()
	// An arrival keeps the base, unless it lands on it (noteArrival).
	m = m.noteArrival().pruneTabMemory()
	m.chart.positions = nil
	m.chart.stations = nil // a newly focused chart never shows the previous chart's marks
	m.chart.cusps = aspect.Cusps{}
	m.chart.bodyStyles = nil
	m = m.waitDetailsForText()
	var textCmd, imgCmd tea.Cmd
	m, textCmd = m.bumpIdentity(surfaceText)
	m, imgCmd = m.refreshWheelIfActive()
	return m, tea.Batch(textCmd, imgCmd)
}

func (m Model) cycleSlotCursor(step int) Model {
	if m.slots == nil {
		return m
	}
	all := m.slots.All()
	n := len(all)
	if n <= 1 {
		return m
	}
	return m.focusSlot(min(max(m.cursorIdx+step, 0), n-1))
}

// focusSlot moves the cursor to list index i. The Bodies row of the slot it
// leaves is remembered and the row of the slot it lands on is put back; a slot
// never focused starts at the top (C12).
func (m Model) focusSlot(i int) Model {
	if m.slots == nil {
		m.cursorIdx = i
		return m
	}
	m = m.rememberBodyRow()
	m.cursorIdx = i
	m.bodyCursorIdx = m.bodyRows[m.focusedSlot()]
	// The timeline's bold row belongs to the chart under the cursor: it moves
	// in the frame of the key (the specification rule 6).
	m.details.pressedSet = false
	return m.clampTimelineCursor().settleChartTop().settleBodiesColStart().resetBodyTop()
}

// settleChartTop records the Chart window for the cursor's current place.
func (m Model) settleChartTop() Model {
	if m.slots == nil {
		return m
	}
	m.chartTop, _ = chartWindow(len(m.slots.All()), m.cursorIdx, m.geometry()[PanelChart].h-2, m.chartTop)
	return m
}

// rememberBodyRow records the Bodies row the focused slot is on.
func (m Model) rememberBodyRow() Model {
	slot := m.focusedSlot()
	if slot == nil || m.bodyRows[slot] == m.bodyCursorIdx {
		return m
	}
	rows := make(map[*store.Slot]int, len(m.bodyRows)+1)
	for k, v := range m.bodyRows {
		rows[k] = v
	}
	rows[slot] = m.bodyCursorIdx
	m.bodyRows = rows
	return m
}

// keepBodyRow records row as the Bodies row slot was left on.
func (m Model) keepBodyRow(slot *store.Slot, row int) Model {
	rows := make(map[*store.Slot]int, len(m.bodyRows)+1)
	for k, v := range m.bodyRows {
		rows[k] = v
	}
	rows[slot] = row
	m.bodyRows = rows
	return m
}

// trackRecheck notes that a re-check was issued against slot.
func (m Model) trackRecheck(slot *store.Slot) Model {
	r := make(map[*store.Slot]*store.Slot, len(m.pendingRechecks)+1)
	for k, v := range m.pendingRechecks {
		r[k] = v
	}
	r[slot] = slot
	m.pendingRechecks = r
	return m
}

// rehomeRechecks points every pending re-check whose chart sat in old at fresh.
func (m Model) rehomeRechecks(old, fresh *store.Slot) Model {
	if len(m.pendingRechecks) == 0 {
		return m
	}
	r := make(map[*store.Slot]*store.Slot, len(m.pendingRechecks))
	for k, v := range m.pendingRechecks {
		if v == old {
			v = fresh
		}
		r[k] = v
	}
	m.pendingRechecks = r
	return m
}

// dropRechecks forgets the pending re-check issued against issued, and any whose
// chart is in held (a removed slot).
func (m Model) dropRechecks(issued, held *store.Slot) Model {
	if len(m.pendingRechecks) == 0 {
		return m
	}
	r := make(map[*store.Slot]*store.Slot, len(m.pendingRechecks))
	for k, v := range m.pendingRechecks {
		if (issued != nil && k == issued) || (held != nil && v == held) {
			continue
		}
		r[k] = v
	}
	m.pendingRechecks = r
	return m
}

// resolveSlot is the slot that holds, now, the chart a re-check was issued
// against: one hop through pendingRechecks, or the slot itself when it is not
// tracked.
func (m Model) resolveSlot(s *store.Slot) *store.Slot {
	if cur, ok := m.pendingRechecks[s]; ok {
		return cur
	}
	return s
}

// forgetBodyRow drops a removed slot's remembered row.
func (m Model) forgetBodyRow(slot *store.Slot) Model {
	if _, ok := m.bodyRows[slot]; !ok {
		return m
	}
	rows := make(map[*store.Slot]int, len(m.bodyRows))
	for k, v := range m.bodyRows {
		if k != slot {
			rows[k] = v
		}
	}
	m.bodyRows = rows
	return m
}

// anchoredFocused reports whether the cursor rests on an anchored slot — one
// that can be removed or moved. The floating slot is pinned last (M6).
func (m Model) anchoredFocused() bool {
	return m.slots != nil && m.cursorIdx >= 0 && m.cursorIdx < m.slots.AnchoredCount()
}

// canRemoveSlot is where del acts: the Chart panel owns the keyboard and its
// cursor is on an anchored slot. With Bodies or Aspects focused the key does
// nothing, so a stray press there cannot take a client's chart out.
func (m Model) canRemoveSlot() bool {
	return m.focused == PanelChart && m.anchoredFocused()
}

// canMoveSlot reports whether the focused anchored slot has a neighbour in
// that direction: -1 up, +1 down. The first anchored row cannot go up and the
// last cannot go down; nothing moves below the floating slot.
func (m Model) canMoveSlot(delta int) bool {
	if !m.canRemoveSlot() {
		return false
	}
	j := m.cursorIdx + delta
	return j >= 0 && j < m.slots.AnchoredCount()
}

func (m Model) canMoveSlotUp() bool   { return m.canMoveSlot(-1) }
func (m Model) canMoveSlotDown() bool { return m.canMoveSlot(+1) }

// moveFocusedSlot is ⌃k / ⌃j: the focused anchored slot swaps with its
// neighbour and the cursor follows it. The slot itself is unchanged, so its
// Bodies row, selection and marks travel with it; what can change is which
// slot is the baseline, and the text recomputes only then (comparisonSig:
// whether the cursor compares, against which baseline, and which pair); the
// wheel and the timeline always follow the new order.
func (m Model) moveFocusedSlot(delta int) (Model, tea.Cmd) {
	id := keys.BindingSlotMoveDown
	if delta < 0 {
		id = keys.BindingSlotMoveUp
	}
	if m.blocked(id) != "" {
		return m, nil
	}
	before := m.comparisonSig()
	if !m.slots.MoveAnchored(m.cursorIdx, delta) {
		return m, nil
	}
	m.cursorIdx += delta
	// The text recomputes only when the baseline or the pair changed. The
	// wheel and the timeline follow the new order regardless: a move inside
	// the selected set renumbers the timeline and reorders the wheel's rings.
	// Both are keyed on their inputs in order, so an unchanged set costs
	// nothing.
	// Outside a comparison there is no order to follow: the cursor's own chart
	// is all either surface draws, and it did not change.
	after := m.comparisonSig()
	if after == before && !after.comparing {
		return m, nil
	}
	var textCmd, timelineCmd, imageCmd tea.Cmd
	if after != before {
		m = m.holdShown()
		m, textCmd = m.bumpIdentity(surfaceText)
	}
	if m.timeline.keyed {
		// A timeline never asked for has nothing stale to renumber; its first
		// search is the next text commit's business (C43).
		m, timelineCmd = m.syncTimeline()
	}
	m, imageCmd = m.refreshWheelIfActive()
	// An unchanged order asks for nothing: no command at all, not an empty batch.
	var cmds []tea.Cmd
	for _, c := range []tea.Cmd{textCmd, timelineCmd, imageCmd} {
		if c != nil {
			cmds = append(cmds, c)
		}
	}
	if len(cmds) == 0 {
		return m, nil
	}
	return m, tea.Batch(cmds...)
}

// removeFocusedSlot is del: the focused anchored slot leaves the workspace and
// the cursor stays on the index it held, which the slot below has now taken —
// the floating slot when it was the last anchored one. No chart file is
// touched; ⌃o brings the chart back.
func (m Model) removeFocusedSlot() (Model, tea.Cmd) {
	if m.blocked(keys.BindingSlotDelete) != "" {
		return m, nil
	}
	gone := m.focusedSlot()
	m = m.forgetBodyRow(gone)
	m = m.dropRechecks(nil, gone)
	m.slots.RemoveAnchored(m.cursorIdx)
	m.bodyCursorIdx = m.bodyRows[m.focusedSlot()]
	m = m.resetBodyTop()
	// What the cursor now rests on was not what Bodies and the wheel showed.
	m.chart = chartViewState{}
	m = m.suspendDetails()
	return m.afterFocusChange()
}

// afterComparisonChange recomputes after the comparison's operands moved: the
// baseline changed, or the pair came on or off.
func (m Model) afterComparisonChange() (Model, tea.Cmd) {
	m = m.holdShown()
	var textCmd, imageCmd tea.Cmd
	m, textCmd = m.bumpIdentity(surfaceText)
	m, imageCmd = m.refreshWheelIfActive()
	return m, tea.Batch(textCmd, imageCmd)
}

// cycleFocusedRow routes ↑/↓ to the list owned by the focused panel. Chart
// changes the focused slot (and therefore recomputes its payload); Bodies moves
// only its ephemeral row cursor. Other panels do not claim row navigation yet.
func (m Model) cycleFocusedRow(step int) (Model, tea.Cmd) {
	switch m.focused {
	case PanelChart:
		return m.cycleSlot(step)
	case PanelBodies:
		return m.cycleBodyRow(step)
	default:
		return m, nil
	}
}

// cycleBodyRow is the Bodies panel's Up/Down: the row cursor moves, Details
// follows it, and so does an open wheel.
func (m Model) cycleBodyRow(step int) (Model, tea.Cmd) {
	old := m.bodyCursorIdx
	m = m.cycleBodyCursor(step)
	if m.bodyCursorIdx == old {
		return m, nil
	}
	// Details follows the cursor through the reconciler (reconcile.go).
	// The strongest highlight tier follows the cursor.
	return m.refreshWheelIfActive()
}

func (m Model) cycleBodyCursor(step int) Model {
	// The rows the operator sees: while a scene change holds the previous
	// frame, that frame's rows (the live ones are blank until the commit).
	rows := m.shownChart()
	n := panel.BodyRowCount(rows.positions, rows.cusps)
	if n <= 1 {
		m.bodyCursorIdx = 0
		return m.settleBodyTop()
	}
	return m.moveBodyCursorTo(min(max(m.bodyCursorIdx+step, 0), n-1))
}

// moveBodyCursorTo puts the Bodies cursor on row and settles the window for it.
func (m Model) moveBodyCursorTo(row int) Model {
	m.bodyCursorIdx = row
	if !m.holding() {
		// The row the committed frame is drawn with follows the cursor, so a
		// hold keeps the row the operator left it on.
		m.shown.bodyCursor = m.bodyCursorIdx
	}
	return m.settleBodyTop()
}

// bodiesRows is the Bodies list's shape and room: the rows above the rule, the
// rows under it, and how many display rows the panel body has. The page keys
// and the window arithmetic read it, so both see the list the render draws.
func (m Model) bodiesRows() (planets, under, height int) {
	rows := m.shownChart()
	planets, under = panel.BodyRowSplit(rows.positions, rows.cusps)
	return planets, under, m.geometry()[PanelBodies].h - 2
}

// settleBodyTop holds the stored window where the render would draw it for the
// cursor's row, or at the top when there are no rows. A window that already
// holds the cursor is not moved.
func (m Model) settleBodyTop() Model {
	planets, under, height := m.bodiesRows()
	m.bodyTop = panel.BodyWindowStart(planets, under, m.bodyCursorIdx, m.bodyTop, height)
	return m
}

// resetBodyTop is settleBodyTop for a list that changed under the cursor (another
// slot, another chart): the window is derived from the cursor alone, as it is
// the first time a list is drawn.
func (m Model) resetBodyTop() Model {
	m.bodyTop = 0
	return m.settleBodyTop()
}

// pageBodies pages the Bodies cursor over the focused slot's rows by the paging
// rule (cursor_page.go), counted on the window the panel DRAWS: the cursor rows
// it shows with the cusp rule inside it or not, not the smallest count it could
// show. The page re-derives the window the render will draw (bodyWindowStart) and
// puts the cursor on its first or last cursor row, so a page up lands on the top
// drawn row and a page down on the bottom one at every panel height. Bodies
// follows as it does for an arrow step: Details defers and an open wheel repaints.
func (m Model) pageBodies(op menu.PageOp) (Model, tea.Cmd) {
	planets, under, height := m.bodiesRows()
	n := planets + under
	if n < 1 {
		return m, nil
	}
	height = max(height, 1)
	row := min(max(m.bodyCursorIdx, 0), n-1)
	// The window as it is drawn now, and the cursor rows it shows: the page is
	// counted on those, with the rule inside the window or not.
	start := panel.BodyWindowStart(planets, under, row, m.bodyTop, height)
	first, last := panel.BodyDrawnRows(planets, under, height, start)
	display := n
	if panel.BodyRuled(planets, under, height) {
		display++
	}
	var top int
	switch op {
	case menu.PageDown:
		if row < last {
			row, top = last, start
		} else {
			// The row at the edge stays as the one row of context: it leads the
			// next window, which is then re-derived as the render will draw it.
			top = panel.BodyWindowStart(planets, under, last, panel.BodyDisplayRow(planets, under, height, last), height)
			_, row = panel.BodyDrawnRows(planets, under, height, top)
		}
	case menu.PageUp:
		if row > first {
			row, top = first, start
		} else {
			top = panel.BodyWindowStart(planets, under, first, panel.BodyDisplayRow(planets, under, height, first)-height+1, height)
			row, _ = panel.BodyDrawnRows(planets, under, height, top)
		}
	case menu.PageHome:
		top = panel.BodyWindowStart(planets, under, 0, 0, height)
		row = 0
	case menu.PageEnd:
		top = panel.BodyWindowStart(planets, under, n-1, display, height)
		row = n - 1
	}
	old := m.bodyCursorIdx
	m.bodyTop = top
	m = m.moveBodyCursorTo(row)
	if m.bodyCursorIdx == old {
		return m, nil
	}
	// Details follows the cursor through the reconciler (reconcile.go).
	return m.refreshWheelIfActive()
}

// toggleFocusedSelection applies Space according to panel context. Selection
// mutates store only; crossing the two-selected-slot comparison boundary also
// refreshes text and an active wheel because the projection changes.
func (m Model) toggleFocusedSelection() (Model, tea.Cmd) {
	if m.blocked(keys.BindingSelect) != "" {
		return m, nil
	}
	switch m.focused {
	case PanelChart:
		// What the cursor compares changes when it joins or leaves the
		// selected set, when selecting or deselecting moves the baseline, and
		// when the set it sits in gains or loses the pair its tab names.
		before := m.comparisonSig()
		if _, ok := m.slots.ToggleSlotSelection(m.cursorIdx); !ok {
			return m, nil
		}
		// Rule 8 of the specification: a selection change
		// ends the entered row.
		m.details.pressedSet = false
		m = m.pruneTabMemory().noteArrival()
		if m.comparisonSig() == before {
			return m, nil
		}
		return m.afterComparisonChange()
	case PanelBodies:
		// blocked has answered that a row is under the cursor.
		obj, _ := panel.ObjectAtCursor(m.chart.positions, m.chart.cusps, m.bodyCursorIdx)
		m.slots.ToggleObjectSelection(m.cursorIdx, obj)
		// Selected bodies show only themselves on the chart's own surfaces
		// (showLens), so a changed selection owes all three a refresh: the
		// Aspects list (the text surface), the Details report (held until that
		// frame commits, as for any frame the report must match) and the wheel
		// (its key carries the selection, and the highlight tiers follow it).
		// The Bodies table stays complete, so its rows are not what changes.
		m = m.waitDetailsForText()
		var textCmd, imageCmd tea.Cmd
		m, textCmd = m.noteWant(surfaceText, false)
		m, imageCmd = m.refreshWheelIfActive()
		return m, tea.Batch(textCmd, imageCmd)
	}
	return m, nil
}

// anchorOf is a slot's moment and place as the compute and the wheel take
// them. A nil slot is the zero handle, which reads as "no natal chart".
func anchorOf(s *store.Slot) store.AnchorChart {
	if s == nil {
		return store.AnchorChart{}
	}
	return store.AnchorChart{When: s.Time, Place: s.Location, Label: s.Chart.Label}
}

// baselineChart is the comparison's left side: the first selected anchored
// slot in list order, or the zero handle.
func (m Model) baselineChart() store.AnchorChart {
	if m.slots == nil {
		return store.AnchorChart{}
	}
	return anchorOf(m.deriveScene().baseline)
}

// focusedChart is the chart of the anchored slot under the cursor, or the zero
// handle while the cursor is on the floating slot. Single-chart computes read
// it (C22).
func (m Model) focusedChart() store.AnchorChart {
	if !m.anchoredFocused() {
		return store.AnchorChart{}
	}
	return anchorOf(m.focusedSlot())
}

// wheelHighlight derives the wheel image's highlight state from the
// focused slot alone: its marked objects, plus the Bodies cursor object as the
// strongest tier — but only while the Bodies panel is focused, so an idle
// cursor does not keep one object lit. ObjectAtCursor's ok gate makes an
// out-of-range cursor (or a plain frame cusp) mean "no focused object" rather
// than a stale highlight; a row with no glyph (Fortune, a fixed star) lights like
// any other.
func (m Model) wheelHighlight() lookup.WheelHighlight {
	var hl lookup.WheelHighlight
	if m.slots == nil {
		return hl
	}
	for _, o := range m.slots.SelectedObjects(m.cursorIdx) {
		hl.Selected = append(hl.Selected, o.Number)
	}
	if m.focused == PanelBodies {
		if o, ok := panel.ObjectAtCursor(m.chart.positions, m.chart.cusps, m.bodyCursorIdx); ok {
			hl.Focused = o
		}
	}
	return hl
}

// focusedSlot returns the slot at the current cursor position. Returns
// nil if slots aren't constructed yet, or if cursorIdx is out of bounds
// (shouldn't happen under normal flow, but defends against partial-init
// callers). UI helper — store.Slots stays cursor-free.
func (m Model) focusedSlot() *store.Slot {
	if m.slots == nil {
		return nil
	}
	all := m.slots.All()
	if m.cursorIdx < 0 || m.cursorIdx >= len(all) {
		return nil
	}
	return all[m.cursorIdx]
}

// dispatchRenderWheel handles Ctrl+X: opens (or refreshes) the browser wheel
// preview for the focused slot, or closes it when it is already being served.
// Sets previewActive so subsequent focused-chart changes auto-refresh. No-op
// before a slot exists (cold-start).
func (m Model) dispatchRenderWheel() (Model, tea.Cmd) {
	if m.blocked(keys.BindingRenderWheel) != "" {
		return m, nil
	}
	// A second Ctrl+X on a wheel that is being served closes it. "Being
	// served" is the Log having named an address: the xdg backends have none,
	// and a first press still waiting for its publish is a refresh, not a
	// toggle.
	if m.previewActive && m.wheelURL != "" {
		return m.closeWheel()
	}
	// The hosted format is read again at every opening, so a hand edit of
	// wheel_format takes effect on this press and needs no restart.
	m = m.applyWheelFormat(readWheelFormat())
	m.previewActive = true
	// Ctrl+X is the one gesture that may open a viewer, including after the
	// operator closed the last one; a follow refresh never asks.
	m.wheelOpenSeq++
	m.rows[surfaceImage].spawn.wheel.requestOpen()
	return m.requestImageNow()
}

// closeWheel stops the wheel server and the following of the focused chart. A
// render still in flight is dropped at its commit (previewActive is the gate),
// and a publish still in flight finds the viewer closed.
func (m Model) closeWheel() (Model, tea.Cmd) {
	// Close drops the server and the chart bytes before it stops the listener,
	// so an error here (the listener failing to close) leaves nothing running
	// to report and nothing the operator could do; the line below is true.
	_ = m.rows[surfaceImage].spawn.wheel.closeServer()
	m.previewActive = false
	m.wheelURL = ""
	m.wheelPublishPending = nil
	return m.logWheelClosed(), nil
}

// refreshWheelIfActive issues a (debounced, coalesced) wheel render when the
// preview is open, else a no-op. Used by the auto-refresh triggers (slot cursor
// move, and a recompute commit on the floating slot). The image surface of the
// refresh coordinator (internal/ui/refresh.go) handles latest-wins +
// cancellation, so held-key bursts settle to one trailing render.
func (m Model) refreshWheelIfActive() (Model, tea.Cmd) {
	if !m.previewActive {
		return m, nil
	}
	// The aspect chooser and the body selector preview the TEXT surfaces and
	// nothing else — the aspect chooser's C9 and the body selector's C7 both
	// exclude the WHEEL by name, because it is an external browser preview and
	// a burst of toggles would spend one render each on it.
	//
	// The guard has to be HERE rather than at the toggle, because a preview
	// reaches the wheel indirectly: the toggle schedules a text recompute, and
	// the ordinary text-commit path auto-refreshes the wheel. Without this the
	// exclusion both rows state was not implemented at all — Ctrl+X to open the
	// browser, then Ctrl+B and a single Space, and the preview spent a wheel
	// render. Found by the independent cross-family reviewer, who traced the
	// shared commit path rather than the toggle.
	//
	// Closing the chooser recomputes, and the wheel catches up on that commit.
	if m.startup == startupAspectOpen || m.startup == startupBodyOpen {
		return m, nil
	}
	// C9: a move inside the selected set, or a commit that changed nothing the
	// wheel draws, asks for the picture already asked for.
	if m.wheelUnchanged() {
		return m, nil
	}
	return m.requestImage()
}

// focusedIsFloating reports whether the cursor sits on the floating slot (always
// the last entry in slots.All()). A recompute only changes the floating slot,
// so the preview refreshes on commit only when that slot is the one on screen.
func (m Model) focusedIsFloating() bool {
	if m.slots == nil {
		return false
	}
	all := m.slots.All()
	return len(all) > 0 && m.cursorIdx == len(all)-1
}

// jumpFloatingToNow handles `n`: jump the floating-slot target to wall-clock
// now, stashing the prior target so `N` can toggle back. It acts from any
// cursor position (D37).
func (m Model) jumpFloatingToNow() (Model, tea.Cmd) {
	if m.blocked(keys.BindingJumpNow) != "" {
		return m, nil
	}
	m.details.pressedSet = false
	m.previousFloatingTime = m.targetFloatingTime
	m.targetFloatingTime = m.clock.Now()
	m.floatingDetached = false
	m = m.waitDetailsForText()
	m, wantCmd := m.noteWant(surfaceText, true)
	m, tickCmd := m.armNowTick()
	return m, tea.Batch(wantCmd, tickCmd)
}

// toggleFloatingBack handles `N`: swap the floating-slot target with the
// stashed previous target, a one-level back-and-forth toggle. A silent
// no-op when there is no stash yet; from any cursor position like `n` (D37).
func (m Model) toggleFloatingBack() (Model, tea.Cmd) {
	if m.blocked(keys.BindingToggleBack) != "" {
		return m, nil
	}
	m.details.pressedSet = false
	m.floatingDetached = true
	m.targetFloatingTime, m.previousFloatingTime = m.previousFloatingTime, m.targetFloatingTime
	m = m.waitDetailsForText()
	return m.noteWant(surfaceText, true)
}

// helpBindings is the Keys popup's table. It has nothing to type into, so j
// and k move as they do on the main screen, q closes it as Esc does, and ?
// closes it as it opened it.
func helpBindings() []keys.BindingEntry {
	return keys.Titled([]keys.BindingEntry{
		{ID: keys.BindingListUp, Binding: key.NewBinding(key.WithKeys("up", "k")), Title: "Move row", HelpKey: "↑/↓", BarKey: "↑↓", Bar: true},
		{ID: keys.BindingListDown, Folded: true, Binding: key.NewBinding(key.WithKeys("down", "j")), BarKey: "↓"},
		{ID: keys.BindingScrollUp, Binding: key.NewBinding(key.WithKeys("pgup")), Title: "Page list", HelpKey: "⇞/⇟", BarKey: "⇞⇟", Bar: true},
		{ID: keys.BindingScrollDown, Folded: true, Binding: key.NewBinding(key.WithKeys("pgdown")), BarKey: "⇟"},
		// The text has no filter box, so g and G are Home and End here.
		{ID: keys.BindingPageHome, Title: "First row", Binding: key.NewBinding(key.WithKeys("home", "g")), BarKey: "home/g"},
		{ID: keys.BindingPageEnd, Title: "Last row", Binding: key.NewBinding(key.WithKeys("end", "G")), BarKey: "end/G"},
		{ID: keys.BindingHelp, Binding: key.NewBinding(key.WithKeys("?")), BarKey: "?"},
		closeOnEscOrQ("Close"),
	})
}

// updateHelpKey handles a key while help is open: ↑/↓ walk the bindings
// (the specification C5b), and ? or Esc close it. The list
// stops at its ends.
func (m Model) updateHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	step := 0
	switch keys.MatchBinding(msg, helpBindings()) {
	case keys.BindingClose, keys.BindingHelp:
		m.helpOpen = false
		return m, nil
	case keys.BindingListUp:
		step = -1
	case keys.BindingListDown:
		step = 1
	case keys.BindingScrollUp:
		step = -keys.HelpListHeight(m.height)
	case keys.BindingScrollDown:
		step = keys.HelpListHeight(m.height)
	case keys.BindingPageHome:
		m.help.Offset = 0
		return m, nil
	case keys.BindingPageEnd:
		m.help.Offset = keys.HelpScroll(0, len(m.helpLines()), len(m.helpLines()), keys.HelpListHeight(m.height))
		return m, nil
	default:
		return m, nil
	}
	m.help.Offset = keys.HelpScroll(m.help.Offset, step, len(m.helpLines()), keys.HelpListHeight(m.height))
	return m, nil
}

func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.modalOpen() {
		return m, nil
	}
	// The console covers the panel the wheel would otherwise scroll, and
	// Details is not rendered underneath it. Pointing at a panel nobody can
	// see must not move it. The focused Log owns the keyboard the same way: a
	// click behind it would move focus the operator has handed over.
	if m.console.open || m.logFocus.active {
		return m, nil
	}
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	panel := hitTestPanel(m.width, m.height, chartContentLines(m.slots), m.shownCount(), msg.X, msg.Y)
	switch msg.Button {
	case tea.MouseButtonWheelDown:
		// The wheel scrolls the panel it is pointing at, and Details is the
		// only panel that scrolls. It never moves focus: pointing is not
		// choosing.
		return m.wheelScroll(panel, detailsScrollStep), nil
	case tea.MouseButtonWheelUp:
		return m.wheelScroll(panel, -detailsScrollStep), nil
	case tea.MouseButtonLeft:
		// Attached Details owns the keyboard, and a click behind it would
		// move focus the operator has handed over
		// (the specification C13). The wheel still scrolls it.
		if m.details.attached {
			return m, nil
		}
		if isFocusable(panel) {
			return m.setFocusedPanel(panel)
		}
	}
	return m, nil
}

// wheelScroll applies a wheel notch to the panel under the pointer. Anything
// but Details is inert.
func (m Model) wheelScroll(panel PanelID, delta int) Model {
	if panel != PanelDetails {
		return m
	}
	return m.scrollDetailsTab(delta)
}

// setFocusedPanel changes the active view and refreshes the passive Details
// report for that context. Re-selecting the current panel is a no-op, both
// here and at the Jump digit that routes to it.
// panelDigits is the one table behind the panel digits. A digit focuses its
// panel; pressed again on the panel it already focuses, it steps that panel's
// tabs, as `0` always does for Details (operator, 2026-10-04: continuous "3"
// in Aspects cycles the tabs). Dispatch reads only this table and
// TestPanelDigitAgainStepsItsTabs walks it, so a panel that gains tabs gains
// the gesture with no per-panel code.
var panelDigits = map[keys.BindingID]PanelID{
	keys.BindingFocusChart:   PanelChart,
	keys.BindingFocusBodies:  PanelBodies,
	keys.BindingFocusAspects: PanelAspects,
}

// pressPanelDigit is every panel digit: focus the panel, or, when it already
// has the keyboard, step its tabs (cycleTab leaves a one-tab panel as it is).
// While Details is attached the digit only focuses, as before.
func (m Model) pressPanelDigit(p PanelID) (Model, tea.Cmd) {
	if p == m.focused && !m.details.attached {
		return m.cycleTab(p, +1)
	}
	return m.setFocusedPanel(p)
}

func (m Model) setFocusedPanel(panel PanelID) (Model, tea.Cmd) {
	if !isFocusable(panel) || panel == m.focused {
		return m, nil
	}
	crossesBodies := panel == PanelBodies || m.focused == PanelBodies
	m.focused = panel
	// Details and the Chart's Timeline follow the focus through the reconciler
	// (reconcile.go): the new subject's report, and the Timeline's first search.
	if !crossesBodies {
		return m, nil
	}
	// Entering Bodies lights the cursor body's strongest tier; leaving drops
	// it — either way the wheel must repaint.
	return m.refreshWheelIfActive()
}

// logLane is which of the three lanes a log line was written on. The lane
// is assigned by the writer verb — logNote, logError, logCommands — and never
// inferred from the text. It decides which view renders the line
// and whether it can be evicted:
//
//	lane    | 6-row Log | console | capped
//	argv    | no        | yes     | yes, at MaxLogLines
//	error   | yes       | yes     | no
//	note    | yes       | no      | no
//	warning | no        | yes     | no
//	wheel   | yes       | yes     | no     (two lanes: open, closed)
//	now     | no        | yes     | no     (at most one line, replaced in place)
//	trace   | no        | yes     | yes, with argv at MaxLogLines
type logLane uint8

const (
	// laneNote is everything that is not one of the other two: startup
	// resolution, configuration refusals, unresolved fallbacks, hints —
	// including lines that happen to carry a Go error value.
	laneNote logLane = iota
	// laneError is a failure of an operation the operator just asked for,
	// which failed to produce the requested complete result. A closed set of
	// call sites writes it, pinned by TestErrorLaneMembership.
	laneError
	// laneArgv is one executed Astrolog command, recorded by an earlier change and
	// appended here at the commit gate that painted its result.
	laneArgv
	// laneWarning is a warning line of a refused Astrolog call beyond the
	// first, which the error line's one-sentence reason already carries. It
	// renders only in the console, where there is room to read every line
	// (the specification C12). Written only by logWarnings.
	laneWarning
	// laneWheelOpen is the one line that says where the wheel is served,
	// written by logWheelOpen when the first publish of a session lands. It is
	// not an error and not a note: it has its own colour (WheelOpenColour) and
	// its own words, so it takes neither the error form nor the `scope:` form.
	laneWheelOpen
	// laneWheelClosed is the one line that says the wheel was closed by a
	// second Ctrl+X, written by logWheelClosed. Red like an error, but the
	// operator asked for it: it is not in the error lane, marks no unseen
	// error and takes no "Couldn't" form.
	laneWheelClosed
	// laneNow is the one line that keeps the provenance of the minute tick's
	// calls, which write no argv line: the last command of the latest tick,
	// replaced in place by setNowLine. Its text is the full command, which is
	// what Enter copies; the console shows it behind `now: ` (live_tick.go).
	laneNow
	// laneTrace is what Sextile says about its own working, for the large log
	// only. The Log panel speaks about the operator's charts and requests: a
	// request that did not happen, a screen that shows other than what was
	// asked, data that changed. A workaround that works, Sextile's housekeeping
	// (how it was launched, the one-time copy of old state files, where a chart's
	// house system came from, a template's branch trace), and an internal check
	// that failed when nobody asked for anything, are written here. A line is
	// not in the Log panel, sets no unseen-error mark and shows no notice; it
	// is in the console in arrival order, drawn as stored, and the
	// MaxLogLines cap drops it with the argv lines. Written only by logTrace.
	laneTrace
)

// logLine is one entry of the Model's log buffer: its lane and its text.
type logLine struct {
	lane logLane
	text string
	// failed marks an argv or now line whose Astrolog call ended in an error:
	// the large log draws a red `!` before it. It is drawn, never stored in
	// text, so Enter and `,y` copy the plain command.
	failed bool
}

// logNote appends a note: `scope: what happened`, the form every note
// takes (the specification A3). scope is a literal of lower-case
// letters and dots, kept by convention (the invariant was retired 2026-10-03). The
// line is built here, so substituted text cannot break the form: a line break
// becomes a space and a closing full stop is dropped.
//
// The Log speaks only when something happened the operator would otherwise
// have to check for (§ Log scope): a file written, or a thing that did not
// come back as it was left. Routine reading writes nothing, and neither does
// a key that had nothing to act on.
func (m Model) logNote(scope, format string, args ...any) Model {
	return m.appendLog(laneNote, scope+": "+closeLine(oneLine(fmt.Sprintf(format, args...)), ""))
}

// logTrace appends a console-only line, `scope: what happened`, formed as
// logNote forms it. See laneTrace for what belongs here.
func (m Model) logTrace(scope, format string, args ...any) Model {
	return m.appendLog(laneTrace, scope+": "+closeLine(oneLine(fmt.Sprintf(format, args...)), ""))
}

// logWheelOpen appends the line that names where the wheel is served. url is
// the server's address as preview reports it; its closing slash is dropped
// from the line, which is read by a person and typed into a browser.
func (m Model) logWheelOpen(url string) Model {
	return m.appendLog(laneWheelOpen, closeLine(oneLine(fmt.Sprintf("Wheel at %s (Ctrl+X again closes it)", strings.TrimSuffix(url, "/"))), ""))
}

// logWheelClosed appends the line that says the wheel server was stopped.
func (m Model) logWheelClosed() Model {
	return m.appendLog(laneWheelClosed, "Wheel closed. Ctrl+X opens it again")
}

// logError appends an error line: a failure of something the operator asked
// for, as one sentence in the error-style voice (A4). format starts
// "Couldn't "; the line ends with exactly one full stop whatever the
// substituted cause ended with. Errors render in the 6-row Log and the
// console, in the error colour, and are never evicted. The call sites are
// pinned by TestErrorLaneMembership.
func (m Model) logError(format string, args ...any) Model {
	return m.appendLog(laneError, closeLine(oneLine(fmt.Sprintf(format, args...)), "."))
}

// logErrorLine appends text to the error lane exactly as written: the caller
// owns the wording, so no "Couldn't" form and no full stop are added.
func (m Model) logErrorLine(text string) Model {
	return m.appendLog(laneError, closeLine(oneLine(text), ""))
}

// abandon adds gen to a set of abandoned write generations. It copies, so a
// Model value held elsewhere keeps its own set.
func abandon(set []int, gen int) []int {
	return append(append([]int(nil), set...), gen)
}

// reclaim reports whether gen is in set, and returns the set without it.
func reclaim(set []int, gen int) ([]int, bool) {
	for i, g := range set {
		if g == gen {
			return append(append([]int(nil), set[:i]...), set[i+1:]...), true
		}
	}
	return set, false
}

// closeLine ends text with stop: any closing full stops are dropped first,
// so a substituted cause cannot double one or leave one on a note. A cut mark
// (`...` or `…`, from logIdent) is not a full stop: it stays, and closes the
// line by itself.
func closeLine(text, stop string) string {
	text = strings.TrimRight(text, " ")
	if strings.HasSuffix(text, "...") || strings.HasSuffix(text, "…") {
		return text
	}
	return strings.TrimRight(text, ".") + stop
}

// oneLine folds every line break in s into one space, so a substituted
// multi-line cause stays one Log line.
func oneLine(s string) string {
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
}

// logWarnings appends, for the console only, the warning lines of a refused
// Astrolog call after the first: the error line already carries the first as
// its reason (failureReason), and the Log has no room for the rest. A line
// reaches the buffer with its terminal control bytes removed, so neither the
// console's render nor a copy can carry one (C12). An err that is not such a
// refusal appends nothing.
func (m Model) logWarnings(err error) Model {
	lines := warningTexts(err)
	if len(lines) > 1 {
		for _, l := range lines[1:] {
			m = m.appendLog(laneWarning, l)
		}
	}
	return m
}

// logCommands appends one argv line per carried command, in carried order.
// Argv lines render only in the console and are the only lines the
// MaxLogLines cap evicts. Call it at a commit gate and nowhere else: a
// command belongs in the buffer only once the frame it produced has painted
// .
//
// failed lists the indexes of the commands whose call ended in an error; each
// of those lines is marked (logLine.failed).
func (m Model) logCommands(commands []string, failed ...int) Model {
	for i, c := range commands {
		m = m.appendLogLine(logLine{lane: laneArgv, text: c, failed: slices.Contains(failed, i)})
	}
	return m
}

// failedLast is the failed list for a result that carries commands in spawn
// order and ended in err: the last command is the call that failed, because
// the failing call ends the run. No error, or no command (Astrolog could not
// start), marks nothing.
func failedLast(err error, commands []string) []int {
	if err == nil || len(commands) == 0 {
		return nil
	}
	return []int{len(commands) - 1}
}

// appendLog is the one place a line enters the buffer. An open console is
// untouched by it, with one exception: the console renders the snapshot it
// took when it opened, so nothing here has to keep a cursor aligned, but a now
// line replaces the snapshot's own in place (nowInSnapshot), so the tick shows.
func (m Model) appendLog(lane logLane, text string) Model {
	return m.appendLogLine(logLine{lane: lane, text: text})
}

// appendLogLine is appendLog for a line that carries more than its text.
func (m Model) appendLogLine(line logLine) Model {
	lane := line.lane
	if lane == laneNow {
		m = m.nowInSnapshot(line)
		// The now line is one line, and it lives at the bottom: a later write
		// takes the old one out and appends the new. The buffer is copied, not
		// edited, because earlier Model values share it.
		lines := make([]logLine, 0, len(m.logLines)+1)
		for _, l := range m.logLines {
			if l.lane != laneNow {
				lines = append(lines, l)
			}
		}
		m.logLines = lines
	}
	m.logLines = append(m.logLines, line)
	if lane == laneError {
		m.errorUnseen = true
		m.errorKeptView = false
	}
	if lane == laneArgv || lane == laneTrace {
		m.logLines = trimConsoleOnly(m.logLines)
	}
	return m
}

// trimConsoleOnly drops the oldest argv and trace lines until at most
// MaxLogLines of the two remain, leaving every error and note line in place. It returns a
// fresh slice when it drops anything, so no caller's copy of the buffer is
// edited underneath it.
func trimConsoleOnly(lines []logLine) []logLine {
	excess := -MaxLogLines
	for _, l := range lines {
		if l.lane == laneArgv || l.lane == laneTrace {
			excess++
		}
	}
	if excess <= 0 {
		return lines
	}
	out := make([]logLine, 0, len(lines)-excess)
	for _, l := range lines {
		if (l.lane == laneArgv || l.lane == laneTrace) && excess > 0 {
			excess--
			continue
		}
		out = append(out, l)
	}
	return out
}

// logPanelLines is the 6-row Log's view of the buffer: error and note lines
// in arrival order, never an argv line. Derived live on every
// render.
func (m Model) logPanelLines() []string {
	out := make([]string, 0, len(m.logLines))
	for _, l := range m.logLines {
		if l.lane != laneArgv && l.lane != laneWarning && l.lane != laneNow && l.lane != laneTrace {
			out = append(out, l.text)
		}
	}
	return out
}

// logPanelRows is logPanelLines as the 6-row Log draws them: an error line
// in the error colour, or behind `! ` with colours off (A1, A2).
func (m Model) logPanelRows() []string {
	out := make([]string, 0, len(m.logLines))
	for _, l := range m.logLines {
		switch l.lane {
		case laneArgv, laneWarning, laneNow, laneTrace:
		default:
			out = append(out, renderLaneLine(l, 0, m.cfg))
		}
	}
	return out
}

// consoleLines is the console's view of the buffer: EVERY lane, in arrival
// order. openConsole takes it once, as the snapshot the console then renders
// until it closes. Always a fresh slice, so a later append cannot reach into an
// open console.
//
// It excluded `note` until an earlier change, on the reasoning recorded at
// `console_taste_test.go`: the console showed argv+error and the 6-row Log
// showed error+note, so every lane had a home. That is true at 132x40 and false
// at 80x24, where there is no Log panel — so a note had no surface at all, and
// an astrologer at the smaller supported width could not read a single line the
// app wrote. Not truncated: absent. Driving the real binary at 80x24 found zero
// occurrences of `session:`, `active city:` or `location:` anywhere in a whole
// capture.
//
// The console is therefore the COMPLETE record and the 6-row Log stays the
// glance surface it already was (error+note, no argv, requirement C7). A note joins
// argv under the console's existing contract for a line wider than the box:
// read what fits, and `Enter` copies the whole stored line.
func (m Model) consoleLines() []logLine {
	out := make([]logLine, 0, len(m.logLines))
	var now []logLine
	for _, l := range m.logLines {
		if l.lane == laneNow {
			now = append(now, l)
			continue
		}
		out = append(out, l)
	}
	// The now line is always the console's last, however many lines arrived
	// after the tick that wrote it.
	return append(out, now...)
}

// nowInSnapshot puts a tick's command into an open console: the snapshot's now
// line, last by construction, takes the new text, and the cursor and window stay
// where they were. A console opened before any tick gains the line at its end.
func (m Model) nowInSnapshot(line logLine) Model {
	if !m.console.open {
		return m
	}
	snap := slices.Clone(m.console.snapshot)
	if n := len(snap); n > 0 && snap[n-1].lane == laneNow {
		snap[n-1] = line
	} else {
		snap = append(snap, line)
	}
	m.console.snapshot = snap
	return m
}

func (m Model) modalOpen() bool {
	return m.helpOpen || m.actionsOpen || m.startupShowsModal()
}

// startupShowsModal reports whether `startup` is currently naming an open modal
// rather than a startup phase. The field does both jobs, so anything that
// transitions startup has to ask which job it is doing.
func (m Model) startupShowsModal() bool {
	switch m.startup {
	case startupAtlasOpen, startupChartOpen, startupAspectOpen, startupBodyOpen, startupHouseOpen, startupZodiacOpen, startupDisplayOpen, startupExperimentsOpen, startupSessionsOpen, startupSessionNameOpen, startupSaveOpen, startupTitleOpen:
		return true
	}
	return false
}

// View is the frame, finished: every glyph a terminal may draw wide has a space
// of its own style after it (glyph_room.go).
func (m Model) View() string { return term.PaintGlyphRoom(m.viewFrame()) }

func (m Model) viewFrame() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.cantStart.any() {
		return m.cantStartView()
	}
	visualFocus := m.focused
	if m.modalOpen() {
		visualFocus = PanelNone
	}
	composed := compose(m.width, m.height, m.panels, visualFocus, m.modalOpen(), m.keysContext(), m.cfg, m.logPanelRows(), m.paintedChart(), m.detailsFrame(), m.slots, m.cursorIdx, m.bodyCursorIdx, m.chartTop, m.clock.Now(), m.place, m.lens, m.rowTag, m.floatingTitle(), m.tabStrips(), m.console, m.bodiesView(), m.logFocusView())
	if m.helpOpen {
		modal := keys.RenderHelp(m.keysContextFor(ownerHelp), popupChrome(m.cfg), m.width, m.height)
		return term.Centered(composed, modal, m.width, m.height)
	}
	if m.currentOwner() == ownerActions {
		return term.Centered(composed, renderActions(m.actions, m.blocked, m.cfg, m.width, m.height), m.width, m.height)
	}
	if m.startup == startupAtlasOpen {
		modal := renderAtlasModal(m.atlas, m.cfg, m.width, m.height)
		return term.Centered(composed, modal, m.width, m.height)
	}
	if m.startup == startupChartOpen {
		picker := m.picker
		picker.now = m.clock.Now()
		modal := renderChartModal(picker, m.cfg, m.width, m.height)
		return term.Centered(composed, modal, m.width, m.height)
	}
	if m.startup == startupSaveOpen {
		modal := renderSaveModal(m.save, m.cfg, m.width, m.height)
		return term.Centered(composed, modal, m.width, m.height)
	}
	if m.startup == startupTitleOpen {
		modal := renderTitleForm(m.title, m.cfg, m.width, m.height)
		return term.Centered(composed, modal, m.width, m.height)
	}
	if m.startup == startupBodyOpen {
		modal := renderBodyModal(m.bodyModal, m.cfg, m.width, m.height)
		return term.Centered(composed, modal, m.width, m.height)
	}
	if m.startup == startupHouseOpen {
		modal := renderHouseFinder(m.houseFinder, m.cfg, m.width, m.height)
		return term.Centered(composed, modal, m.width, m.height)
	}
	if m.startup == startupZodiacOpen {
		modal := renderZodiacFinder(m.zodiacFinder, m.cfg, m.width, m.height)
		return term.Centered(composed, modal, m.width, m.height)
	}
	if m.startup == startupDisplayOpen {
		modal := renderHouseDisplay(m.houseDisplayPopup, m.cfg, m.width, m.height)
		return term.Centered(composed, modal, m.width, m.height)
	}
	if m.startup == startupExperimentsOpen {
		modal := renderExperiments(m.experimentsPopup, m.cfg, m.width, m.height)
		return term.Centered(composed, modal, m.width, m.height)
	}
	if m.startup == startupAspectOpen {
		modal := renderAspectModal(m.aspectModal, m.cfg, m.width, m.height)
		return term.Centered(composed, modal, m.width, m.height)
	}
	if m.startup == startupSessionsOpen {
		return term.Centered(composed, renderSessions(m, m.width, m.height), m.width, m.height)
	}
	if m.startup == startupSessionNameOpen {
		return term.Centered(composed, renderSessionName(m, m.width, m.height), m.width, m.height)
	}
	return composed
}

// focusablePanels lists the panels that can receive focus via Tab / Shift+Tab
// / number keys / mouse click.
func focusablePanels() []PanelID {
	return []PanelID{PanelChart, PanelBodies, PanelAspects}
}

func isFocusable(name PanelID) bool {
	if name == PanelNone {
		return false
	}
	for _, p := range focusablePanels() {
		if p == name {
			return true
		}
	}
	return false
}

func (m Model) nextFocus() PanelID {
	p := focusablePanels()
	for i, n := range p {
		if n == m.focused {
			return p[(i+1)%len(p)]
		}
	}
	return p[0]
}

func (m Model) prevFocus() PanelID {
	p := focusablePanels()
	for i, n := range p {
		if n == m.focused {
			return p[(i-1+len(p))%len(p)]
		}
	}
	return p[len(p)-1]
}

// deleteFromKeypadNumericMode turns the Delete key as st sends it into the key
// Bubble Tea knows. st (and terminals built on its key table) sends Delete as
// `ESC [P` unless the program switches the keypad to application mode, which
// Bubble Tea never does; Bubble Tea has no entry for that sequence and passes
// it on as an unknown CSI message, so `del` on the Chart panel did nothing.
// Only that one message changes; the unknown-CSI type is unexported, so it is
// recognised by its type name and its exact text.
func deleteFromKeypadNumericMode(msg tea.Msg) tea.Msg {
	if s, ok := msg.(fmt.Stringer); ok &&
		fmt.Sprintf("%T", msg) == "tea.unknownCSISequenceMsg" &&
		s.String() == "?CSI[80]?" {
		return tea.KeyMsg{Type: tea.KeyDelete}
	}
	return msg
}
