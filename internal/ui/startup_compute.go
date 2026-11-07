package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// FirstFrameReadyMsg is the result of a chart compute (cold-start or any later
// refresh). Carries the focused slot's positions + aspects (single mode) or the
// cross-aspects (overlay mode), or an Err that defers/blanks the render.
type FirstFrameReadyMsg struct {
	// Positions is the FOCUSED slot's bodies in every mode (the dual
	// Natal/Transit layering was retired when Bodies pinned to the focused
	// slot): transit positions when the floating slot is focused, natal when
	// the anchored slot is. Aspects carries the single-chart intra-aspects.
	Positions      []aspect.Position
	NatalPositions []aspect.Position
	// Stations are the focused chart's station marks (lookup.StationMarks),
	// computed inside the same compute as Positions so Bodies commits both
	// at once and a held frame keeps its own. StationErr is the probe's
	// failure: the positions landed without marks and the commit logs one note.
	Stations   map[int]lookup.Station
	StationErr error
	// Unavailable names the selected objects this Astrolog cannot compute.
	// Their rows are dropped before they reach Positions, so this is the only
	// thing that can tell the operator why a body they ticked is not drawn.
	Unavailable []string
	// UnavailableUnion is set when Unavailable was joined from several chart
	// calls (a comparison): no single date belongs to it.
	UnavailableUnion bool
	// NatalCusps is the ANCHORED chart's house frame. Moving bodies are
	// placed into it, so a transiting body's house names one of the
	// client's houses rather than a house of a chart cast for wherever the
	// operator happens to be. Zero outside overlay mode.
	NatalCusps aspect.Cusps
	// FramedPositions are the moving bodies as the frame's zodiac places
	// them, when the moving chart's own zodiac is another; their houses in
	// NatalCusps are read from here. Nil when Positions already are.
	FramedPositions []aspect.Position
	// Cusps is the FOCUSED chart's own house frame, from the lookup that
	// produced Positions (no extra spawn); Bodies lists its twelve cusps.
	Cusps        aspect.Cusps
	BodyStyles   astrologstyle.Bodies
	Aspects      []aspect.Aspect
	AspectStyles astrologstyle.Aspects
	// AspectsReport is the focused slot's aspect list exactly as Astrolog
	// printed it (the -aa stdout Aspects were parsed from); Details shows it
	// while the Aspects tab is focused, with no spawn of its own.
	AspectsReport string
	// CrossAspects is the transit→natal overlay aspect list (astrolog -r0,
	// role-preserving), populated only on the overlay branch (Overlay=true).
	// Aspects is populated on that branch too (the focused slot's own list)
	// since an earlier change, so the Aspects panel's first tab stays live.
	CrossAspects []lookup.OverlayAspect
	// CrossAspectStyles keeps Astrolog's primary/secondary order, which is
	// also the cell order the overlay renderer prints (natal left, transit
	// right — an earlier change).
	CrossAspectStyles astrologstyle.Aspects
	// CrossAspectsReport is the -rt -aa relationship list as Astrolog printed
	// it; Details shows it on the Transit→Natal tab.
	CrossAspectsReport string
	// CrossSynastry marks CrossAspects as a pair of two anchored charts
	// (Astrolog -r0): the renderer shows no applying/separating phase for it.
	CrossSynastry bool
	// Overlay marks an overlay-mode frame. Explicit (not derived from
	// CrossAspects) so a valid overlay with zero cross-aspects is still an
	// overlay frame (the reviewer an earlier change). Since an earlier change the Aspects panel renders by
	// its active tab; the first overlay commit after a non-overlay frame
	// activates the Transit→Natal tab.
	Overlay bool
	Err     error
	// RunID / WantID are the refresh coordinator's ordering tokens for the
	// text surface (see internal/ui/refresh.go), assigned by startRun when
	// the compute is launched. A result whose RunID is not the running run
	// was superseded (canceled or replaced) and is ignored; a result whose
	// WantID is below the surface's commitFloor belongs to a superseded
	// workspace identity (atlas re-pick, anchor change) and never paints.
	// Exported so cross-package drivers (the invariants first-frame golden)
	// can hand-construct a result matching the scripted dispatch — after a
	// single cold-start dispatch both tokens are 1.
	RunID  uint64
	WantID uint64
	// TransitTime is the floating-slot moment this compute was
	// dispatched against. The text commit applies it to
	// slots.floating.Time so the chart panel header (which renders
	// slot.Time) advances ONLY when matching bodies/aspects land —
	// header + bodies + aspects stay synchronized through held-key
	// bursts (a review finding).
	TransitTime time.Time
	// Commands is every Astrolog command behind this frame, one
	// ready-to-paste line per spawn, in the order the spawns completed —
	// for the overlay compute that is natal, transit, then cross. Carried from lookup unchanged; applyTextCommit turns it
	// into argv lines for the console, and the Err branches carry
	// the spawns that ran before the failure.
	Commands []string
	// FailedAt lists the indexes into Commands of the calls that ended in an
	// error, which the large log marks. Nil means the frame did not say: an
	// error frame then marks its last command (the failing call ends a lookup,
	// so that is right wherever Commands holds one lookup's spawns). A frame
	// that knows better sets it, empty when the failing lookup spawned nothing.
	FailedAt []int
	// Columns is one Bodies column per chart of the shown set, in numeral
	// order, on a comparing frame only (an alone frame carries none).
	// ColumnsFor names the shown set they were computed for.
	Columns    []panel.BodyColumn
	ColumnsFor columnsFor
	// key is the key of the frame this compute was asked for (textKeyFor at the
	// run's start); keyed says a run set it. The commit stamps it on the frame
	// (C1), so the screen compares what was computed with what is wanted now.
	key   textKey
	keyed bool
	// fresh is the position lookups this compute actually ran, for the
	// result cache; the commit merges them.
	fresh resultCache
	// Casts is the house system Astrolog reported for every chart this frame
	// cast, with the one asked for (the specification C7).
	Casts []castFact
}

// computeChartCmd runs lookup.FirstCallTransits at `transitTime` for `place`
// under the coordinator's cancelable ctx and emits FirstFrameReadyMsg tagged
// with the run's tokens. Failure is reported via the Err field — the state
// machine then chooses between rendering empty panels or surfacing the error.
// A superseded run is SIGKILLed via ctx (astrolog.invoke normalizes that to
// context.Canceled, which the coordinator drops silently).
//
// transitTime is the operator-controlled moment to compute transits for
// (from the floating slot's Time), NOT the wall-clock now — time-step
// keys advance it; the chart panel header advertises the same value, so
// header and bodies/aspects stay in step. `probers` is the computer bundle;
// pass nil to default to the bundled astrolog binary.
func computeChartCmd(ctx context.Context, runID, wantID uint64, cast castSpawns, probers *lookup.Probers, transitTime time.Time, place location.PlaceInfo, lens lookup.Lens) tea.Cmd {
	return func() tea.Msg {
		result, err := cast.transit(ctx, probers, transitTime, place, lens)
		var stations stationFrame
		if err == nil {
			stations = probeStations(ctx, cast, probers, result.Positions, transitTime, place, lens)
		}
		return FirstFrameReadyMsg{
			Stations:      stations.marks,
			StationErr:    stations.err,
			Positions:     result.Positions,
			Cusps:         result.Cusps,
			BodyStyles:    result.BodyStyles,
			Aspects:       result.Aspects,
			AspectStyles:  result.AspectStyles,
			AspectsReport: result.AspectsReport,
			Commands:      concatCommands(result.Commands, stations.commands),
			Unavailable:   result.Unavailable,
			Err:           err,
			RunID:         runID,
			WantID:        wantID,
			TransitTime:   transitTime,
			Casts:         castsOf(floatingKey(transitTime, place), lens, result, err),
		}
	}
}

// renderWheelCmd renders the wheel image for `when` at `place` under `lens`
// (astrolog -XV…) under a cancelable ctx, returning the BYTES tagged with the
// image surface's runID/wantID. preview.Show is intentionally NOT called here:
// the coordinator calls it once, synchronously, for the latest committed result
// only (refresh.go commitRefresh), so two renders can never race the stable
// browser preview. The expensive astrolog spawn runs off the Update loop, and a
// superseded run is SIGKILLed via ctx (astrolog.invoke normalizes that to
// context.Canceled, which the coordinator drops silently).
func renderWheelCmd(ctx context.Context, runID, wantID uint64, render func(context.Context, time.Time, location.PlaceInfo, lookup.Lens, lookup.WheelHighlight) ([]byte, error), when time.Time, place location.PlaceInfo, lens lookup.Lens, highlight lookup.WheelHighlight) tea.Cmd {
	return func() tea.Msg {
		img, err := render(ctx, when, place, lens, highlight)
		return refreshResultMsg{
			surface: surfaceImage,
			runID:   runID,
			wantID:  wantID,
			payload: imagePayload{bytes: img},
			err:     err,
		}
	}
}

// renderOverlayWheelCmd is the overlay-mode image analogue: it renders the
// BI-WHEEL (inner natal `chart` / outer transit at `transitWhen`+`transitPlace`)
// via astrolog's -r0 -XV path, tagged with the image surface's runID/wantID.
// Same coordinator contract as renderWheelCmd — preview.Show is called once by
// the coordinator for the latest committed result only, never here.
func renderOverlayWheelCmd(ctx context.Context, runID, wantID uint64, render func(context.Context, store.AnchorChart, time.Time, location.PlaceInfo, lookup.Lens) ([]byte, error), chart store.AnchorChart, transitWhen time.Time, transitPlace location.PlaceInfo, lens lookup.Lens, labels []string) tea.Cmd {
	return func() tea.Msg {
		img, err := render(ctx, chart, transitWhen, transitPlace, lens)
		return refreshResultMsg{
			surface: surfaceImage,
			runID:   runID,
			wantID:  wantID,
			payload: imagePayload{bytes: img, rings: labels},
			err:     err,
		}
	}
}

// computeMode is the routing decision buildTextCmd makes. Split
// out so tests can pin the contract against an enum rather than asserting
// only that the returned tea.Cmd is non-nil (both branches return non-nil,
// so a future accidental collapse to transit-only would not be caught by a
// nil-check alone).
type computeMode int

const (
	computeModeTransit     computeMode = iota // focused floating slot's single transit chart
	computeModeOverlay                        // transit-over-natal: focused-slot bodies + cross-aspects
	computeModeNatalSingle                    // focused anchored slot's single natal chart
)

// selectComputeMode returns the routing decision for the current workspace.
// A comparison is derived from the cursor and the selected set (comparing):
// the cursor on a member of a set of two or more. Otherwise the focused slot
// drives a single chart, selected or not: anchored → natal, floating → transit.
func selectComputeMode(m Model) computeMode {
	if m.comparing() {
		return computeModeOverlay
	}
	if !m.focusedChart().IsZero() {
		return computeModeNatalSingle
	}
	return computeModeTransit
}

// buildTextCmd derives the chart-compute Cmd for the text surface from the
// current workspace state via selectComputeMode — the refresh coordinator's
// buildSurfaceCmd(surfaceText) calls it at run-start, snapshotting the FULL
// request (time, place, anchor) at that moment. Centralizes the routing so
// every recompute trigger (startup, atlas pick, time-step) consults one place
// — the "given workspace, derive compute" contract the reviewer's strategy consult
// identified as the load-bearing prerequisite.
//
// NEVER returns nil (freeze-safety, the design review): with no
// workspace, transitTimeFromWorkspace falls back to the wall clock and the
// transit-only branch still computes — exactly the pre-coordinator cold-start
// behavior. A nil here with no live debounce tick would strand an unmet want.
//
// Reads the workspace's slots (the baseline for a comparison, the focused
// slot for a single chart), m.clock, m.place. Does NOT mutate the model.
func buildTextCmd(m Model, ctx context.Context, runID, wantID uint64) tea.Cmd {
	key, keyed := m.textKeyFor()
	return stampTextKey(buildTextCompute(m, ctx, runID, wantID), key, keyed)
}

// stampTextKey puts the key a text run was asked for on the frame it returns.
func stampTextKey(cmd tea.Cmd, key textKey, keyed bool) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		if f, ok := msg.(FirstFrameReadyMsg); ok {
			f.key, f.keyed = key, keyed
			return f
		}
		return msg
	}
}

func buildTextCompute(m Model, ctx context.Context, runID, wantID uint64) tea.Cmd {
	transitTime := transitTimeFromWorkspace(m)
	// The house system and zodiac are the cursor chart's own; a comparison's
	// calls take the frame's (overlayInputs). Everything else about the lens
	// is m.lens.
	lens := m.viewLens()
	if bad := m.unresolvedFor(m.focusedSlot()); bad != nil {
		// No guessed system: the cast fails the way any failed compute does.
		return func() tea.Msg {
			return FirstFrameReadyMsg{Err: bad, RunID: runID, WantID: wantID, TransitTime: transitTime}
		}
	}
	return m.withSelectedAspects(ctx, transitTime, m.textCompute(ctx, runID, wantID, transitTime, lens))
}

// textCompute is the frame compute for the mode the model is in, under the
// screen's lens.
func (m Model) textCompute(ctx context.Context, runID, wantID uint64, transitTime time.Time, lens lookup.Lens) tea.Cmd {
	cast := m.rows[surfaceText].spawn.cast
	switch selectComputeMode(m) {
	case computeModeOverlay:
		// Bodies follows the focused slot even in overlay: natal positions when
		// an anchored slot is focused, transit positions when the floating slot
		// is. The cross-aspects are the ACTIVE PAIR's left chart × its right one,
		// the base × the cursor's chart: the floating slot on the right (-rt,
		// the base natal) or an anchored cursor (synastry, -r0, the base first).
		// The house frame is the scene's frame: the base, else the cursor's own.
		// On a floating cursor its own chart needs no focus compute; on an
		// anchored cursor with a base it does, for Bodies.
		sc := m.deriveScene()
		frame, focus, opts := m.overlayInputs(sc, transitTime)
		return computeOverlayCmd(ctx, runID, wantID, cast, m.probers, frame, focus, transitTime, m.place, !m.focusedIsFloating(), lens, opts...)
	case computeModeNatalSingle:
		return computeNatalSingleCmd(ctx, runID, wantID, cast, m.probers, m.focusedChart(), lens)
	default:
		return computeChartCmd(ctx, runID, wantID, cast, m.probers, transitTime, m.place, lens)
	}
}

// withSelectedAspects is the Aspects half of "selected bodies show only
// themselves": the focused chart's OWN aspect list is the frame's Aspects,
// AspectStyles and AspectsReport asked under showLens, while the frame's
// Positions, stations, columns and cross aspects stay what the full lens gave
// (the Bodies table keeps every body so the selection can grow; a comparison
// ignores the selection). One Astrolog call computes positions and aspects
// together, so the restricted list needs a call of its own, and only while a
// body is selected; nothing selected returns inner untouched, and so the same
// argv as before.
//
// The restricted call goes through the same lookup as the frame, so its argv
// is on the argv lane beside the frame's and the encoder is the compiler's.
// When it fails the frame fails with it: the full list would be a claim about
// a selection it does not follow.
func (m Model) withSelectedAspects(ctx context.Context, transitTime time.Time, inner tea.Cmd) tea.Cmd {
	lens := m.showLens(m.focusedSlot())
	if len(lens.Objects.Only) == 0 {
		return inner
	}
	chart, probers, place := m.focusedChart(), m.probers, m.place
	cast := m.rows[surfaceText].spawn.cast
	return func() tea.Msg {
		msg := inner()
		frame, ok := msg.(FirstFrameReadyMsg)
		if !ok || frame.Err != nil {
			return msg
		}
		var r lookup.Result
		var err error
		if chart.IsZero() {
			r, err = cast.transit(ctx, probers, transitTime, place, lens)
		} else {
			r, err = cast.natal(ctx, probers, chart, lens)
		}
		before := frame.Commands
		frame.Commands = concatCommands(frame.Commands, r.Commands)
		if err != nil {
			frame.Err = err
			frame.FailedAt = failedAfter(r.Commands, before)
			return frame
		}
		frame.Aspects, frame.AspectStyles, frame.AspectsReport = r.Aspects, r.AspectStyles, r.AspectsReport
		return frame
	}
}

// overlayInputs is what the overlay compute is asked for, read from a scene:
// frame is the natal lookup whose cusps ride along (the scene's frame: the
// base, else the cursor's chart), focus the cursor's own chart when it is not the frame
// (zero for the floating slot), and opts the house-frame flag, the lenses, the
// active pair's cross options and the Bodies columns. The scene is an argument
// so a test can hand it one whose frame is another slot.
func (m Model) overlayInputs(sc scene, transitTime time.Time) (frame, focus store.AnchorChart, opts []overlayOption) {
	frame, frameSlot := anchorOf(sc.frame), sc.frame
	if sc.frame.Floating {
		// The floating slot on its own tab frames itself, so the "natal" the
		// compute needs is not the sky but ①, as it always was: it is unused
		// (no house overlay, no cross call) and cached for the other tabs, so
		// it is cast in ①'s own lens, the one its column asks for.
		frame, frameSlot = anchorOf(sc.baseline), sc.baseline
	}
	if sc.cursor != sc.frame {
		focus = m.focusedChart()
	}
	// The frame chart and the comparison are cast in the frame's lens; the
	// sky in the floating chart's own (its Bodies and its column).
	sky := m.slots.Floating()
	opts = []overlayOption{withHouseFrame(sc.cursor != sc.frame), withLenses(m.lensFor(frameSlot), m.lensFor(&sky))}
	if pair := sc.pair; pair == nil {
		// The `Aspects` tab shows the cursor's own chart: no cross call.
		opts = append(opts, withoutCross())
	} else {
		// Rule 5: the cross call's LEFT chart is the pair's left; the
		// house frame stays on the scene's frame.
		opts = append(opts, withCrossLeft(anchorOf(pair.left)))
		if !pair.right.Floating {
			opts = append(opts, withSynastryPair(anchorOf(pair.right)))
		}
	}
	opts = append(opts, withColumns(m.shownColumns(sc, transitTime), m.chart.results))
	return frame, focus, opts
}

// transitTimeFromWorkspace is the operator-controlled "now" the chart
// compute should target. Sources from the floating slot's Time when the
// workspace exists (carrying any time-step advances per an earlier change); falls
// back to the wall clock for the cold-start path before slots are
// constructed.
//
// The user-visible contract the review pinned: chart panel
// header (chartSlotsBody, slot.Time) MUST equal the lookup compute
// moment. This helper is the only place we resolve "the time the chart
// is for"; buildTextCmd and any future caller must consult it.
func transitTimeFromWorkspace(m Model) time.Time {
	if m.slots != nil {
		// Read the TARGET (operator's intent), not slots.Floating().Time
		// (last-committed). During a held-key burst targetFloatingTime
		// races ahead while committed time advances on each result
		// arrival — header + bodies stay in step on the committed time
		// (a review finding).
		if !m.targetFloatingTime.IsZero() {
			return m.targetFloatingTime
		}
		return m.slots.Floating().Time
	}
	return m.clock.Now()
}

// computeOverlayCmd computes an overlay-mode frame: the focused slot's bodies
// for the Bodies panel plus the transit→natal cross-aspects for the Aspects
// panel. It runs lookup.FirstCallNatalPositions (the anchored chart) and
// lookup.FirstCallTransits (now at `place`) — both are needed to derive the
// cross-aspects regardless of which slot is focused — and selects Bodies from
// whichever the cursor sits on (focusedNatal). Runs under the coordinator's
// cancelable ctx; a superseded overlay compute dies at whichever lookup it is in.
//
// The single FirstFrameReadyMsg keeps the model's apply path identical to the
// single-chart branches (one ready message, not two partial ones).
//
// chart is the frame (the slot whose cusps the screen reads: the base). focus, when
// non-zero, is an anchored slot under the cursor that is NOT the frame; its
// own chart is computed for Bodies and its own aspects, whichever pair the
// cross-aspects describe. It is zero with one anchored slot, so the calls an
// overlay makes there are the ones it always made.
//
// By default the pair is chart × the floating slot (withCrossLeft names
// another left chart) and the frame's house cusps ride along when the
// floating slot is focused. Two options widen it for a set of more than one
// anchored slot: withSynastryPair names an anchored slot as the pair's other
// side, and withHouseFrame says whether the frame rides along.
func computeOverlayCmd(ctx context.Context, runID, wantID uint64, cast castSpawns, probers *lookup.Probers, chart, focus store.AnchorChart, transitTime time.Time, place location.PlaceInfo, focusedNatal bool, lens lookup.Lens, opts ...overlayOption) tea.Cmd {
	plan := overlayPlan{houseFrame: !focusedNatal}
	for _, opt := range opts {
		opt(&plan)
	}
	return func() tea.Msg {
		f := &resultFetcher{cache: plan.cache, cast: cast}
		msg := overlayFrame(f, ctx, runID, wantID, probers, chart, focus, transitTime, place, focusedNatal, lens, plan)
		msg.fresh = f.fresh
		msg.Casts = f.casts
		return msg
	}
}

// overlayFrame is the overlay compute's body; its lookups go through f.
func overlayFrame(f *resultFetcher, ctx context.Context, runID, wantID uint64, probers *lookup.Probers, chart, focus store.AnchorChart, transitTime time.Time, place location.PlaceInfo, focusedNatal bool, lens lookup.Lens, plan overlayPlan) FirstFrameReadyMsg {
	// Every return below carries the commands of the steps that already
	// ran, in completion order: natal, transit, cross. A
	// lookup error still carries its own spawns (C14), so the list is
	// concatenated the same way on every branch.
	//
	// Each chart is cast in its own lens: the frame chart in the frame's, the
	// cursor's Bodies in the cursor's, the sky in the floating chart's. The
	// comparison (the cross call, the house overlay) takes the frame's.
	frameLens, skyLens := lens, lens
	if plan.frameLens != nil {
		frameLens, skyLens = *plan.frameLens, *plan.skyLens
	}
	natal, natalErr := f.natal(ctx, probers, chart, frameLens)
	if natalErr != nil {
		return FirstFrameReadyMsg{Overlay: true, Err: natalErr, RunID: runID, WantID: wantID, TransitTime: transitTime, Commands: concatCommands(natal.Commands), Unavailable: natal.Unavailable}
	}
	// Bodies follow the focused slot, so a focused anchored slot other than
	// the frame needs its own natal compute for them.
	own, ownCommands := natal, []string(nil)
	if !focus.IsZero() {
		var ownErr error
		own, ownErr = f.natal(ctx, probers, focus, lens)
		ownCommands = own.Commands
		if ownErr != nil {
			return FirstFrameReadyMsg{Overlay: true, Err: ownErr, RunID: runID, WantID: wantID, TransitTime: transitTime, Commands: concatCommands(natal.Commands, own.Commands), UnavailableUnion: true, Unavailable: unionNames(natal.Unavailable, own.Unavailable)}
		}
	}
	// An anchored pair never reads the transit chart: only a floating
	// cursor (its Bodies) or a floating pair (its -rt) needs it.
	var transit lookup.Result
	var transitErr error
	if (plan.synastry.IsZero() && !plan.noCross) || !focusedNatal {
		transit, transitErr = f.transit(ctx, probers, transitTime, place, skyLens)
	}
	// Bodies = the FOCUSED slot; the cross-aspects below still use BOTH.
	// Computed before the transit-error return so a transit failure while the
	// NATAL slot is focused still paints the natal bodies we already have
	// (only the floating-focused case legitimately blanks on a transit error).
	bodies := transit.Positions
	bodyCusps := transit.Cusps
	styles := transit.BodyStyles
	// The focused slot's OWN aspects ride along too: the Aspects
	// panel's first tab renders them while the pair is selected, and they
	// are the fallback frame when the pair is deselected before the next
	// single-chart compute lands. Both lookup.Results are already fetched;
	// no extra spawn.
	ownAspects := transit.Aspects
	ownStyles := transit.AspectStyles
	ownReport := transit.AspectsReport
	if focusedNatal {
		bodies = own.Positions
		bodyCusps = own.Cusps
		styles = own.BodyStyles
		ownAspects = own.Aspects
		ownStyles = own.AspectStyles
		ownReport = own.AspectsReport
	}
	// The station probe is for the FOCUSED chart's Bodies only: the chart whose
	// positions `bodies` holds, at its own moment and place.
	probeAt, probePlace := transitTime, place
	if focusedNatal {
		probeChart := chart
		if !focus.IsZero() {
			probeChart = focus
		}
		probeAt, probePlace = probeChart.When, probeChart.Place
	}
	stations := probeStations(ctx, f.cast, probers, bodies, probeAt, probePlace, lens)
	if transitErr != nil {
		return FirstFrameReadyMsg{Overlay: true, Stations: stations.marks, StationErr: stations.err, Positions: bodies, Cusps: bodyCusps, BodyStyles: styles, Aspects: ownAspects, AspectStyles: ownStyles, AspectsReport: ownReport, Err: transitErr, RunID: runID, WantID: wantID, TransitTime: transitTime, Commands: concatCommands(natal.Commands, ownCommands, transit.Commands, stations.commands), FailedAt: failedAfter(transit.Commands, natal.Commands, ownCommands), UnavailableUnion: true, Unavailable: unionNames(natal.Unavailable, transit.Unavailable)}
	}
	// Cross-aspects come from astrolog's -rt -aa relationship run (real
	// power, role-preserving, Astrolog's own applying/separating with the
	// natal chart held still) — see lookup.OverlayCrossAspects. One spawn.
	var cross lookup.OverlayAspects
	var crossErr error
	left := chart
	if !plan.crossLeft.IsZero() {
		left = plan.crossLeft
	}
	if plan.noCross {
		// The `Aspects` tab: the frame's own aspects only, no cross call.
	} else if plan.synastry.IsZero() {
		cross, crossErr = f.cast.overlay(ctx, frameLens, left, transitTime, place)
	} else {
		// Two anchored charts: Astrolog's plain relationship list (-r0),
		// baseline as chart 1. Its applying/separating verdict is a
		// synthetic one, so the frame says it is a synastry pair and the
		// renderer never shows it.
		cross, crossErr = f.cast.synastry(ctx, frameLens, left, plan.synastry.When, plan.synastry.Place, plan.synastry.Label)
	}
	if crossErr != nil {
		return FirstFrameReadyMsg{Overlay: true, Stations: stations.marks, StationErr: stations.err, Positions: bodies, Cusps: bodyCusps, BodyStyles: styles, Aspects: ownAspects, AspectStyles: ownStyles, AspectsReport: ownReport, Err: crossErr, RunID: runID, WantID: wantID, TransitTime: transitTime, Commands: concatCommands(natal.Commands, ownCommands, transit.Commands, cross.Commands, stations.commands), FailedAt: failedAfter(cross.Commands, natal.Commands, ownCommands, transit.Commands), UnavailableUnion: true, Unavailable: unionNames(natal.Unavailable, transit.Unavailable)}
	}
	// One column per shown chart, through the same cache: a chart this run
	// already looked up is that chart's column, and a chart whose lookup
	// fails leaves only its own column blank (C8).
	var columns []panel.BodyColumn
	var columnCommands []string
	var columnFailures []int
	if len(plan.shown) > 0 {
		columns, columnCommands, columnFailures = f.columns(ctx, probers, plan.shown, place, lens)
	}
	// The frame's cusps ride along when the cursor is not the frame, which is
	// whenever a base is chosen (operator 2026-10-24): the cursor's body is
	// placed in the base's houses (bold) beside the base's own house of that
	// body (faint). On the own tab the cursor is the frame, so a body's own
	// house already is its frame house and a dual frame would say the same
	// thing twice.
	//
	// The placing is a comparison, so it is done in the frame's zodiac: when
	// the cursor chart's own zodiac is another, its bodies are cast once more
	// in the frame's and placed from there.
	var overlayPositions, framedPositions []aspect.Position
	var overlayCusps aspect.Cusps
	var framedCommands []string
	if plan.houseFrame {
		overlayPositions, overlayCusps = natal.Positions, natal.Cusps
		ownLens := skyLens
		if focusedNatal {
			ownLens = lens
		}
		if ownLens.Zodiac != frameLens.Zodiac && (!focusedNatal || !focus.IsZero()) {
			placing := ownLens
			placing.Zodiac = frameLens.Zodiac
			var framed lookup.Result
			var framedErr error
			if focusedNatal {
				framed, framedErr = f.natal(ctx, probers, focus, placing)
			} else {
				framed, framedErr = f.transit(ctx, probers, transitTime, place, placing)
			}
			framedCommands = framed.Commands
			if framedErr != nil {
				return FirstFrameReadyMsg{Overlay: true, Stations: stations.marks, StationErr: stations.err, Positions: bodies, Cusps: bodyCusps, BodyStyles: styles, Aspects: ownAspects, AspectStyles: ownStyles, AspectsReport: ownReport, Err: framedErr, RunID: runID, WantID: wantID, TransitTime: transitTime, Commands: concatCommands(natal.Commands, ownCommands, transit.Commands, cross.Commands, columnCommands, framed.Commands, stations.commands), FailedAt: failedAfter(framed.Commands, natal.Commands, ownCommands, transit.Commands, cross.Commands, columnCommands), UnavailableUnion: true, Unavailable: unionNames(natal.Unavailable, transit.Unavailable)}
			}
			framedPositions = framed.Positions
		}
	}
	return FirstFrameReadyMsg{
		Overlay:            true,
		Stations:           stations.marks,
		StationErr:         stations.err,
		Positions:          bodies,
		Cusps:              bodyCusps,
		NatalPositions:     overlayPositions,
		NatalCusps:         overlayCusps,
		FramedPositions:    framedPositions,
		BodyStyles:         styles,
		Aspects:            ownAspects,
		AspectStyles:       ownStyles,
		AspectsReport:      ownReport,
		CrossAspects:       cross.Aspects,
		CrossAspectStyles:  cross.AspectStyles,
		CrossAspectsReport: cross.Report,
		CrossSynastry:      !plan.synastry.IsZero(),
		Commands:           concatCommands(natal.Commands, ownCommands, transit.Commands, cross.Commands, columnCommands, framedCommands, stations.commands),
		FailedAt:           columnFailed(len(concatCommands(natal.Commands, ownCommands, transit.Commands, cross.Commands)), columnFailures),
		RunID:              runID,
		WantID:             wantID,
		TransitTime:        transitTime,
		Columns:            columns,
		ColumnsFor:         plan.shown,
	}
}

// stationFrame is what a station probe hands the compute that ran it.
type stationFrame struct {
	marks    map[int]lookup.Station
	commands []string
	err      error
}

// probeStations asks lookup for the station marks of the chart whose positions
// the frame carries (the specification C4). It runs INSIDE the
// compute, so marks and positions travel in one message. A failed probe is
// reported in err and costs the marks only; the positions still land.
func probeStations(ctx context.Context, cast castSpawns, probers *lookup.Probers, positions []aspect.Position, at time.Time, place location.PlaceInfo, lens lookup.Lens) stationFrame {
	marks, commands, err := cast.stations(ctx, probers, positions, at, place, lens)
	return stationFrame{marks: marks, commands: commands, err: err}
}

// overlayPlan is what an overlay compute does beyond today's baseline ×
// floating pair; overlayOption builds it.
type overlayPlan struct {
	// synastry, when non-zero, is the anchored chart on the pair's right side,
	// paired with crossLeft (or the baseline when crossLeft is zero).
	synastry store.AnchorChart
	// crossLeft, when non-zero, is the cross call's left chart; zero is the
	// baseline.
	crossLeft store.AnchorChart
	// houseFrame says the baseline's positions and cusps ride along.
	houseFrame bool
	// noCross skips the cross-aspects call: the Aspects tab shows the
	// cursor's own chart and no pair is on screen.
	noCross bool
	// shown, when set, asks for one Bodies column per chart of the shown
	// set; cache holds the lookups of the last committed frame.
	shown columnsFor
	cache resultCache
	// frameLens is the frame chart's own lens, the one every comparison call
	// takes (the cross call, the house overlay); skyLens the floating chart's
	// own. Nil is the cursor's lens.
	frameLens, skyLens *lookup.Lens
}

type overlayOption func(*overlayPlan)

// withSynastryPair pairs the baseline with another ANCHORED chart (-r0).
func withSynastryPair(other store.AnchorChart) overlayOption {
	return func(p *overlayPlan) { p.synastry = other }
}

// withCrossLeft names the chart on the LEFT of the cross call (the natal of
// -rt, chart 1 of -r0) when it is not the baseline; the house frame stays the
// baseline's.
func withCrossLeft(left store.AnchorChart) overlayOption {
	return func(p *overlayPlan) { p.crossLeft = left }
}

// withoutCross makes the compute skip the cross-aspects call.
func withoutCross() overlayOption {
	return func(p *overlayPlan) { p.noCross = true }
}

// withColumns asks the compute for one Bodies column per shown chart, reusing
// the cached lookups of charts that did not move.
func withColumns(shown columnsFor, cache resultCache) overlayOption {
	return func(p *overlayPlan) { p.shown, p.cache = shown, cache }
}

// withHouseFrame sets whether the frame's house cusps ride along (C5, S4a).
func withHouseFrame(on bool) overlayOption {
	return func(p *overlayPlan) { p.houseFrame = on }
}

// withLenses names the lens the frame chart and the comparison are cast in
// and the lens the sky is cast in, when they are not the cursor's own.
func withLenses(frame, sky lookup.Lens) overlayOption {
	return func(p *overlayPlan) { p.frameLens, p.skyLens = &frame, &sky }
}

// failedAfter is the FailedAt of a frame whose failing lookup spawned `failing`
// after the earlier lists: the index of the lookup's last command in the joined
// list, or an empty (not nil) list when it spawned nothing, so no earlier
// success is marked in its place.
func failedAfter(failing []string, earlier ...[]string) []int {
	if len(failing) == 0 {
		return []int{}
	}
	return []int{len(concatCommands(earlier...)) + len(failing) - 1}
}

// columnFailed is the FailedAt of a frame whose columns' lookups failed:
// failures are indexes into the columns' own commands, offset by the commands
// that came before them. Nil when none failed.
func columnFailed(offset int, failures []int) []int {
	var out []int
	for _, i := range failures {
		out = append(out, offset+i)
	}
	return out
}

// concatCommands joins the command lists of several lookup results into one
// fresh list, in the order given (which is the order the spawns completed).
// It only carries: nothing here composes, quotes, or edits a command line
// . A fresh slice keeps the message from aliasing a result's
// backing array.
func concatCommands(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}

// computeNatalSingleCmd computes the anchored slot's single natal chart
// (positions + intra-natal aspects) for single-mode display when the cursor
// sits on the natal slot. No TransitTime is set: the natal moment is fixed, so
// applyTextCommit must NOT advance the floating slot's committed time for a
// natal frame (the floating header stays where the operator left it).
func computeNatalSingleCmd(ctx context.Context, runID, wantID uint64, cast castSpawns, probers *lookup.Probers, chart store.AnchorChart, lens lookup.Lens) tea.Cmd {
	return func() tea.Msg {
		natal, err := cast.natal(ctx, probers, chart, lens)
		var stations stationFrame
		if err == nil {
			stations = probeStations(ctx, cast, probers, natal.Positions, chart.When, chart.Place, lens)
		}
		return FirstFrameReadyMsg{
			Stations:      stations.marks,
			StationErr:    stations.err,
			Positions:     natal.Positions,
			Cusps:         natal.Cusps,
			BodyStyles:    natal.BodyStyles,
			Aspects:       natal.Aspects,
			AspectStyles:  natal.AspectStyles,
			AspectsReport: natal.AspectsReport,
			Commands:      concatCommands(natal.Commands, stations.commands),
			Unavailable:   natal.Unavailable,
			Err:           err,
			RunID:         runID,
			WantID:        wantID,
			Casts:         castsOf(anchoredKey(chart), lens, natal, err),
		}
	}
}
