package ui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/ingest"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// startupState is the UI's cold-start state machine. cp1 shipped two
// states; cp2 completed the chain. cp3 (this slice) keeps the same shape
// but routes the bootstrapping → {atlas-open | computing} transition
// through location.Resolve, deferred until both perimeter + atlas msgs
// have arrived.
//
//	bootstrapping → atlas-open → computing → ready
//	bootstrapping →           ready                (when resolution succeeds)
//
// Each state has its own render branch in Model.View. Transitions are
// driven by typed messages (StartupPerimeterReadMsg, AtlasLoadedMsg,
// AtlasPickedMsg, FirstFrameReadyMsg). Tests inject the stream directly.
type startupState string

const (
	startupBootstrapping startupState = "bootstrapping"
	startupAtlasOpen     startupState = "atlas-open"
	startupChartOpen     startupState = "chart-open"
	startupSaveOpen      startupState = "save-open"
	startupTitleOpen     startupState = "title-open"
	startupAspectOpen    startupState = "aspect-open"
	startupBodyOpen      startupState = "body-open"
	startupHouseOpen     startupState = "house-open"
	startupZodiacOpen    startupState = "zodiac-open"
	startupDisplayOpen   startupState = "display-open"
	startupSessionsOpen  startupState = "sessions-open"
	// startupSessionNameOpen is the Sessions list's name field: Save as, or a
	// rename (sessions_popup.go).
	startupSessionNameOpen startupState = "session-name-open"
	startupComputing       startupState = "computing"
	startupReady           startupState = "ready"
)

// AtlasLoadedMsg is the result of the cold-start atlas read. Index is nil
// when Err is non-nil. The modal renders a fallback line when Err is set
// (no atlas, no picker).
type AtlasLoadedMsg struct {
	Index []location.Indexed
	Err   error
}

// AspectRegistryLoadedMsg carries the aspect set read out of the user's own
// Astrolog binary. Registry is nil when Err is non-nil, and the
// chooser then refuses to open rather than offering a set it invented.
type AspectRegistryLoadedMsg struct {
	Registry []aspect.RegistryAspect
	Err      error
}

// aspectRegistryRead is indirected so a test can supply a registry without
// spawning astrolog — the same idiom as firstCallNatalPositions below.
var aspectRegistryRead = lookup.AspectRegistry

// ObjectRegistryLoadedMsg carries the object set read out of the user's own
// Astrolog binary. Registry is nil when Err is non-nil, and the selector then
// refuses to open rather than offering a set it invented (C10).
type ObjectRegistryLoadedMsg struct {
	Registry []aspect.RegistryObject
	Err      error
}

// objectRegistryRead is indirected for the same reason aspectRegistryRead is.
var objectRegistryRead = lookup.ObjectRegistry

// loadObjectRegistryCmd reads the object registry once, at startup — and
// crucially BEFORE the compiler emits any `-R0` band. `-HO` reports only
// unrestricted objects, so a read taken later returns the current selection and
// nothing else, and the operator could never re-add a body (C5a).
func loadObjectRegistryCmd() tea.Cmd {
	return func() tea.Msg {
		reg, err := objectRegistryRead(context.Background())
		if err != nil {
			return ObjectRegistryLoadedMsg{Err: err}
		}
		return ObjectRegistryLoadedMsg{Registry: reg}
	}
}

// loadAspectRegistryCmd reads the registry once, at startup. It is read once
// and kept: the chooser's list must not change under the operator mid-session.
func loadAspectRegistryCmd() tea.Cmd {
	return func() tea.Msg {
		reg, err := aspectRegistryRead(context.Background())
		if err != nil {
			return AspectRegistryLoadedMsg{Err: err}
		}
		return AspectRegistryLoadedMsg{Registry: reg}
	}
}

// AstrologProbedMsg is the result of the startup probe: whether Astrolog can
// be found at all. Err is lookup.ErrAstrologNotInstalled when it cannot.
//
// It is asked once, at Init, because the alternative is learning it from the
// first compute and then from every keystroke after — the same fact, spelled
// out by a failure each time, in a six-row Log.
type AstrologProbedMsg struct {
	Err error
}

// probeAstrologCmd asks PATH whether Astrolog is installed, off the Update
// loop. It starts no process (see lookup.ProbeAstrolog).
func probeAstrologCmd() tea.Cmd {
	return func() tea.Msg {
		return AstrologProbedMsg{Err: lookup.ProbeAstrolog()}
	}
}

// applyAstrologProbe says once, and in the operator's words, that the program
// every chart depends on is missing. A successful probe is silent: a tool that
// announces its dependencies are present is talking about itself.
func applyAstrologProbe(m Model, msg AstrologProbedMsg) (Model, tea.Cmd) {
	if msg.Err == nil {
		return m, nil
	}
	m.cantStart.binary = m.cantStart.binary || errors.Is(msg.Err, lookup.ErrAstrologNotInstalled)
	return m.logError("Couldn't find the Astrolog program. Install it and restart."), nil
}

// AtlasPickedMsg is emitted by the atlas modal on Enter against a real
// match. It carries the picked PlaceInfo all downstream handlers need.
type AtlasPickedMsg struct {
	Place location.PlaceInfo
}

// ChartsLoadedMsg carries the saved-chart library read triggered when the
// chart picker opens (Ctrl+O). Charts is nil when Err is set. Drops names every
// file in the library directory that did not become a chart, so the picker can
// say what it could not read instead of showing a quietly short list. AtlasErr
// is the one failure that drops every chart at once, kept apart because it also
// goes on the error lane. Gen is the picker generation the read was dispatched
// under; applyChartsLoaded drops any message whose Gen != m.pickerGen so a late
// response from a superseded open cannot clobber a newer one.
type ChartsLoadedMsg struct {
	Charts   []store.NamedChart
	Drops    []store.ChartDrop
	AtlasErr error
	Err      error
	Gen      int
	// Seen is the listing's fingerprint of each chart file (lookup.Library),
	// handed to a retitle so a file changed since listing is refused.
	Seen map[string]string
}

// ChartPickedMsg is emitted by the chart picker on Enter against a real match.
// It carries the chosen library row, which becomes the natal anchor once its
// birth moment has been resolved.
type ChartPickedMsg struct {
	Chart store.NamedChart
}

// ChartResolvedMsg carries the authoritative birth moment of a picked chart,
// read from the file its row named. A listing row holds no moment, so this is
// what turns a pick into an anchor.
//
// Gen is the pick generation the resolve was dispatched under. applyChartResolved
// drops any message whose Gen != m.pickGen, so a resolve superseded by a later
// pick — or cancelled by Esc — changes no state and writes no error line.
type ChartResolvedMsg struct {
	Chart store.NamedChart
	When  time.Time
	Err   error
	Gen   int
	// Houses is the system the chart file names, as Astrolog read it in the
	// same read that gave When; HasHouses is false when it names none
	// (the specification C2).
	Houses    recipe.HouseSystem
	HasHouses bool
	// Label is the name and place the chart's reports print, and Place the
	// casting place the chart list would show, both from the same read as When
	// (the specification C0). HasPlace is false when the read
	// could not place the chart; the listing row's place then stands.
	Label    store.ReportLabel
	Place    location.PlaceInfo
	HasPlace bool
	// Unplaced is set by a successful read that could not place the chart (its
	// coordinates are beyond the atlas): the listing row's name and place are
	// then NOT the file's, and the pick is refused rather than mixing them with
	// the read's moment.
	Unplaced bool
}

// resolveChartCmd reads the picked chart's authoritative moment off the Update
// loop: one astrolog spawn. The command it ran is deliberately not carried: a
// library resolution is not part of the chart the operator is looking at, and
// the console's Enter copies a command straight to a system clipboard, so the
// argv lane stays reserved for spawns that produced the frame.
func resolveChartCmd(c store.NamedChart, gen int) tea.Cmd {
	return resolveChartCmdIn("", c, gen)
}

// resolveChartCmdIn is resolveChartCmd reading from the directory dir names
// (empty: the resolver's answer now).
func resolveChartCmdIn(dir string, c store.NamedChart, gen int) tea.Cmd {
	return func() tea.Msg {
		p, err := lookup.ResolvePickedChartIn(context.Background(), dir, c)
		return ChartResolvedMsg{Chart: c, When: p.When, Err: err, Gen: gen, Houses: p.Houses, HasHouses: p.HasHouses, Label: p.Label, Place: p.Place, HasPlace: p.HasPlace, Unplaced: err == nil && !p.HasPlace}
	}
}

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
	Stations   map[aspect.Body]lookup.Station
	StationErr error
	// Unavailable names the selected objects this Astrolog cannot compute.
	// Their rows are dropped before they reach Positions, so this is the only
	// thing that can tell the operator why a body they ticked is not drawn.
	Unavailable []string
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
	// Columns is one Bodies column per chart of the shown set, in numeral
	// order, on a comparing frame only (an alone frame carries none).
	// ColumnsFor names the shown set they were computed for.
	Columns    []bodyColumn
	ColumnsFor columnsFor
	// fresh is the position lookups this compute actually ran, for the
	// result cache; the commit merges them.
	fresh resultCache
	// Casts is the house system Astrolog reported for every chart this frame
	// cast, with the one asked for (the specification C7).
	Casts []castFact
}

// loadAtlasCmd reads astrolog's atlas at the resolved path and emits
// AtlasLoadedMsg. Always asynchronous (filesystem read in a goroutine).
func loadAtlasCmd() tea.Cmd {
	return func() tea.Msg {
		path, err := location.AtlasPath()
		if err != nil {
			return AtlasLoadedMsg{Err: err}
		}
		entries, err := location.LoadAtlas(path)
		if err != nil {
			return AtlasLoadedMsg{Err: err}
		}
		return AtlasLoadedMsg{Index: location.NewIndex(entries)}
	}
}

// loadChartsCmd reads the chart library (lookup.LoadChartLibrary) off the Update
// loop and emits ChartsLoadedMsg. Unlike the atlas (loaded once at Init), the
// library is loaded lazily each time the picker opens, so a chart saved out of
// band shows up on the next open.
func loadChartsCmd(gen int) tea.Cmd {
	return func() tea.Msg {
		lib, err := lookup.LoadChartLibrary()
		return ChartsLoadedMsg{
			Charts:   lib.Charts,
			Drops:    lib.Drops,
			AtlasErr: lib.AtlasErr,
			Err:      err,
			Gen:      gen,
			Seen:     lib.Seen,
		}
	}
}

// DefaultCitySavedMsg is the result of keeping the defaults (`<leader> c d`):
// the current city, the house display and the floating chart's frame. Place is
// the city that was written, so a late result names what was saved and not what
// the screen shows by then; House is the form that was written, kept only when
// HouseSaved says the write landed. The house display is written after the
// city, and only once the city was, and the frame after the house display, so
// Err is the city's, HouseErr the house display's and FrameErr the frame's.
// Frame is the "<house name>, <zodiac name>" that was written, kept only when
// FrameSaved says the write landed.
type DefaultCitySavedMsg struct {
	Place      location.PlaceInfo
	Err        error
	House      houseForm
	HouseSaved bool
	HouseErr   error
	Frame      string
	FrameSaved bool
	FrameErr   error
	// Open is the frame every chart open at the keypress was shown in, for
	// the ones that had no record (openFramesNow). A save that lands keeps them.
	Open []store.FrameRecord
}

// saveDefaultCityCmd writes place, then the house display form, then the frame
// (houses and zodiac, in the words the store writes), to preferences.yaml
// through the store. It is the only door to the stored city, the stored house
// display and the stored frame: a start, a pick in the atlas, a choice in the
// popup and a quit write nothing there.
func saveDefaultCityCmd(place location.PlaceInfo, form houseForm, houses recipe.HouseSystem, zodiac recipe.Zodiac, open []store.FrameRecord) tea.Cmd {
	return func() tea.Msg {
		msg := DefaultCitySavedMsg{Place: place, Err: store.SetLocation(place), Open: open}
		if msg.Err != nil {
			return msg
		}
		msg.House = form
		msg.HouseErr = store.SetHouseDisplay(form.info().value)
		msg.HouseSaved = msg.HouseErr == nil
		if msg.HouseErr != nil {
			return msg
		}
		msg.Frame = lookup.HouseName(houses) + ", " + zodiac.Name()
		msg.FrameErr = store.SetFrame(houses.StoredName(), zodiac.RecordName())
		msg.FrameSaved = msg.FrameErr == nil
		return msg
	}
}

// saveDefaultCity is `<leader> c d`: it keeps the current city, the house
// display and the floating chart's house system and zodiac as the defaults.
// With no city chosen yet there is nothing to keep, and nothing is written. A
// second window writes nothing either: the session that has to keep the open
// charts' frames (applyDefaultCitySaved) is the first window's.
func (m Model) saveDefaultCity() (tea.Model, tea.Cmd) {
	if !m.ownsSession() {
		return m.logErrorLine("Save defaults is for the first window, which keeps the session. This window keeps nothing"), nil
	}
	if !m.place.HasTimezone() {
		return m.logErrorLine("default city not saved: no city chosen yet"), nil
	}
	var floating *store.Slot
	if m.slots != nil {
		f := m.slots.Floating()
		floating = &f
	}
	return m, saveDefaultCityCmd(m.place, m.houseDisplay, m.ownHouses(floating).hs, m.ownZodiac(floating), m.openFramesNow())
}

// applyDefaultCitySaved logs the outcome of a save, and when everything was
// kept also shows a footer notice so it is seen when the Log is not on screen.
// A failure is one error line naming what was not kept; the city is kept, and
// said so, even when the house display or the frame was not.
func applyDefaultCitySaved(m Model, msg DefaultCitySavedMsg) (Model, tea.Cmd) {
	failed := ""
	switch {
	case msg.Err != nil:
		cause, _, _ := strings.Cut(msg.Err.Error(), "\n")
		failed = "default city not saved: " + cause
	case msg.HouseErr != nil:
		cause, _, _ := strings.Cut(msg.HouseErr.Error(), "\n")
		failed = "default house display not saved: " + cause
	case msg.FrameErr != nil:
		cause, _, _ := strings.Cut(msg.FrameErr.Error(), "\n")
		failed = "default frame not saved: " + cause
	}
	if msg.Err == nil {
		m.placeGuessed = false
		if msg.HouseSaved {
			m = m.logNote("default house display saved", "%s", msg.House.info().title)
		}
		if msg.FrameSaved {
			m = m.logNote("default frame saved", "%s", msg.Frame)
		}
		m = m.logNote("default city saved", "%s", msg.Place.Display())
	}
	if msg.FrameSaved && failed == "" {
		// The default is for the floating chart on a fresh start and for charts
		// opened later: every chart open at the keypress keeps the frame it showed. The
		// default stays saved when this write fails; a quit retries it.
		var added bool
		if m, added = m.keepFrames(msg.Open); added {
			if err := m.writeSessionNow(); err != nil {
				var outside bool
				if m, outside = m.sessionChangedOutside(err); !outside {
					cause, _, _ := strings.Cut(err.Error(), "\n")
					failed = "open charts' frames not kept: " + cause
				}
			}
		}
	}
	if failed != "" {
		return m.logErrorLine(failed), nil
	}
	return m.showNotice(noticeDefaultsSaved)
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
func computeChartCmd(ctx context.Context, runID, wantID uint64, probers *lookup.Probers, transitTime time.Time, place location.PlaceInfo, lens lookup.Lens) tea.Cmd {
	return func() tea.Msg {
		result, err := firstCallTransits(ctx, probers, transitTime, place, lens)
		var stations stationFrame
		if err == nil {
			stations = probeStations(ctx, probers, result.Positions, transitTime, place, lens)
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
func renderWheelCmd(ctx context.Context, runID, wantID uint64, when time.Time, place location.PlaceInfo, lens lookup.Lens, highlight lookup.WheelHighlight) tea.Cmd {
	return func() tea.Msg {
		img, err := renderWheelImage(ctx, when, place, lens, highlight)
		return refreshResultMsg{
			surface: surfaceImage,
			runID:   runID,
			wantID:  wantID,
			payload: imagePayload{bytes: img},
			err:     err,
		}
	}
}

var renderWheelImage = lookup.RenderWheelImage

// renderOverlayWheelCmd is the overlay-mode image analogue: it renders the
// BI-WHEEL (inner natal `chart` / outer transit at `transitWhen`+`transitPlace`)
// via astrolog's -r0 -XV path, tagged with the image surface's runID/wantID.
// Same coordinator contract as renderWheelCmd — preview.Show is called once by
// the coordinator for the latest committed result only, never here.
func renderOverlayWheelCmd(ctx context.Context, runID, wantID uint64, chart store.AnchorChart, transitWhen time.Time, transitPlace location.PlaceInfo, lens lookup.Lens, labels []string) tea.Cmd {
	return func() tea.Msg {
		img, err := renderOverlayWheelImage(ctx, chart, transitWhen, transitPlace, lens)
		return refreshResultMsg{
			surface: surfaceImage,
			runID:   runID,
			wantID:  wantID,
			payload: imagePayload{bytes: img, rings: labels},
			err:     err,
		}
	}
}

var renderOverlayWheelImage = lookup.RenderOverlayWheelImage

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
		return computeOverlayCmd(ctx, runID, wantID, m.probers, frame, focus, transitTime, m.place, !m.focusedIsFloating(), lens, opts...)
	case computeModeNatalSingle:
		return computeNatalSingleCmd(ctx, runID, wantID, m.probers, m.focusedChart(), lens)
	default:
		return computeChartCmd(ctx, runID, wantID, m.probers, transitTime, m.place, lens)
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
	return func() tea.Msg {
		msg := inner()
		frame, ok := msg.(FirstFrameReadyMsg)
		if !ok || frame.Err != nil {
			return msg
		}
		var r lookup.Result
		var err error
		if chart.IsZero() {
			r, err = firstCallTransits(ctx, probers, transitTime, place, lens)
		} else {
			r, err = firstCallNatalPositions(ctx, probers, chart, lens)
		}
		frame.Commands = concatCommands(frame.Commands, r.Commands)
		if err != nil {
			frame.Err = err
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
func computeOverlayCmd(ctx context.Context, runID, wantID uint64, probers *lookup.Probers, chart, focus store.AnchorChart, transitTime time.Time, place location.PlaceInfo, focusedNatal bool, lens lookup.Lens, opts ...overlayOption) tea.Cmd {
	plan := overlayPlan{houseFrame: !focusedNatal}
	for _, opt := range opts {
		opt(&plan)
	}
	return func() tea.Msg {
		f := &resultFetcher{cache: plan.cache}
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
			return FirstFrameReadyMsg{Overlay: true, Err: ownErr, RunID: runID, WantID: wantID, TransitTime: transitTime, Commands: concatCommands(natal.Commands, own.Commands), Unavailable: unionNames(natal.Unavailable, own.Unavailable)}
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
	stations := probeStations(ctx, probers, bodies, probeAt, probePlace, lens)
	if transitErr != nil {
		return FirstFrameReadyMsg{Overlay: true, Stations: stations.marks, StationErr: stations.err, Positions: bodies, Cusps: bodyCusps, BodyStyles: styles, Aspects: ownAspects, AspectStyles: ownStyles, AspectsReport: ownReport, Err: transitErr, RunID: runID, WantID: wantID, TransitTime: transitTime, Commands: concatCommands(natal.Commands, ownCommands, transit.Commands, stations.commands), Unavailable: unionNames(natal.Unavailable, transit.Unavailable)}
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
		cross, crossErr = overlayCrossAspects(ctx, frameLens, left, transitTime, place)
	} else {
		// Two anchored charts: Astrolog's plain relationship list (-r0),
		// baseline as chart 1. Its applying/separating verdict is a
		// synthetic one, so the frame says it is a synastry pair and the
		// renderer never shows it.
		cross, crossErr = synastryCrossAspects(ctx, frameLens, left, plan.synastry.When, plan.synastry.Place, plan.synastry.Label)
	}
	if crossErr != nil {
		return FirstFrameReadyMsg{Overlay: true, Stations: stations.marks, StationErr: stations.err, Positions: bodies, Cusps: bodyCusps, BodyStyles: styles, Aspects: ownAspects, AspectStyles: ownStyles, AspectsReport: ownReport, Err: crossErr, RunID: runID, WantID: wantID, TransitTime: transitTime, Commands: concatCommands(natal.Commands, ownCommands, transit.Commands, cross.Commands, stations.commands), Unavailable: unionNames(natal.Unavailable, transit.Unavailable)}
	}
	// One column per shown chart, through the same cache: a chart this run
	// already looked up is that chart's column, and a chart whose lookup
	// fails leaves only its own column blank (C8).
	var columns []bodyColumn
	var columnCommands []string
	if len(plan.shown) > 0 {
		columns, columnCommands = f.columns(ctx, probers, plan.shown, place, lens)
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
				return FirstFrameReadyMsg{Overlay: true, Stations: stations.marks, StationErr: stations.err, Positions: bodies, Cusps: bodyCusps, BodyStyles: styles, Aspects: ownAspects, AspectStyles: ownStyles, AspectsReport: ownReport, Err: framedErr, RunID: runID, WantID: wantID, TransitTime: transitTime, Commands: concatCommands(natal.Commands, ownCommands, transit.Commands, cross.Commands, columnCommands, framed.Commands, stations.commands), Unavailable: unionNames(natal.Unavailable, transit.Unavailable)}
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
		RunID:              runID,
		WantID:             wantID,
		TransitTime:        transitTime,
		Columns:            columns,
		ColumnsFor:         plan.shown,
	}
}

// The chart computes' lookup calls are indirected through package vars (the
// renderWheelImage / computeRawReport idiom) so a unit test can pin the frame
// shape each compute assembles without spawning astrolog. All three
// frame-producing paths (computeChartCmd, computeNatalSingleCmd,
// computeOverlayCmd) go through these — lookup.Probers cannot be the seam,
// because its func type returns payload.RawOutput, which ui may not import
// .
var (
	firstCallNatalPositions = lookup.FirstCallNatalPositions
	firstCallTransits       = lookup.FirstCallTransits
	overlayCrossAspects     = lookup.OverlayCrossAspects
	synastryCrossAspects    = lookup.SynastryCrossAspects
	stationMarks            = lookup.StationMarks
)

// stationFrame is what a station probe hands the compute that ran it.
type stationFrame struct {
	marks    map[aspect.Body]lookup.Station
	commands []string
	err      error
}

// probeStations asks lookup for the station marks of the chart whose positions
// the frame carries (the specification C4). It runs INSIDE the
// compute, so marks and positions travel in one message. A failed probe is
// reported in err and costs the marks only; the positions still land.
func probeStations(ctx context.Context, probers *lookup.Probers, positions []aspect.Position, at time.Time, place location.PlaceInfo, lens lookup.Lens) stationFrame {
	marks, commands, err := stationMarks(ctx, probers, positions, at, place, lens)
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
func computeNatalSingleCmd(ctx context.Context, runID, wantID uint64, probers *lookup.Probers, chart store.AnchorChart, lens lookup.Lens) tea.Cmd {
	return func() tea.Msg {
		natal, err := firstCallNatalPositions(ctx, probers, chart, lens)
		var stations stationFrame
		if err == nil {
			stations = probeStations(ctx, probers, natal.Positions, chart.When, chart.Place, lens)
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

// applyStartupPerimeter stores the perimeter read and tries to resolve.
// If the atlas has not yet loaded, the perimeter sits pending until
// AtlasLoadedMsg arrives; resolution runs once both signals are in.
func applyStartupPerimeter(m Model, msg ingest.StartupPerimeterReadMsg) (Model, tea.Cmd) {
	if msg.HasSession && msg.SessionErr == nil && !sessionBelongsHere(msg) {
		// One session is shared by every charts folder, but it is only ever
		// opened in the folder it was saved in: a same-named chart in another
		// folder may be another client's copy. Nothing of it is restored.
		msg.HasSession, msg.Session = false, store.Session{}
		m = m.logNote("session", "the saved session belongs to another charts folder and was not opened")
	}
	m.pendingPerimeter = &msg
	// The keyboard configuration needs neither the atlas nor a resolved
	// location, so it applies here rather than waiting on resolution. It
	// starts nothing and is silent unless something was refused.
	m = m.applyKeyConfig(msg.Keys)
	m = m.applyDisplayConfig(msg.Display)
	m = m.applyWheelFormat(msg.WheelFormat)
	m = m.applyHouseDisplay(msg.HouseDisplay)
	m = m.applyFrameConfig(msg.Frame)
	m = m.applyClockConfig(msg.Clock)
	// A refused session file was moved aside (or not) by the read itself; say so
	// now, so a quit from the city modal still reports a failed keep (C4).
	m = m.reportSessionKeep(msg.SessionKeep)
	if !m.cfg.SecondWindow || m.named.running() {
		return maybeResolveStartup(m)
	}
	// A second window says once, on the Log and in the key bar, that what is
	// done here is not kept: the session it restored is the other window's.
	m = m.logNote("second window", "%s", secondWindowNote)
	m, notice := m.showNotice(noticeSecondWindow)
	m, cmd := maybeResolveStartup(m)
	return m, tea.Batch(notice, cmd)
}

// applyAtlasLoaded folds the atlas index into the model and tries to
// resolve. If the perimeter has not yet been read, the atlas index is
// stored and resolution waits for StartupPerimeterReadMsg.
func applyAtlasLoaded(m Model, msg AtlasLoadedMsg) (Model, tea.Cmd) {
	m.atlas.index = msg.Index
	m.atlas.loadErr = msg.Err
	m.cantStart.atlas = m.cantStart.atlas || msg.Err != nil
	m.atlas.refilter()
	return maybeResolveStartup(m)
}

// maybeResolveStartup runs the candidate-chain resolution once both the
// perimeter read and the atlas index are in. No-op while either is
// missing. Per the reviewer 2026-04-29 design review.
//
// Resolution policy:
//   - Location chain: stored-location → IANA preseed. On full miss, open
//     the atlas modal so the user picks explicitly.
//   - Anchor: the last quit's session only. Without one, transit-only.
//   - Resolver warnings land in the log panel.
//   - No hardcoded "Berlin" technical fallback — refusing is better than
//     silently casting the wrong chart.
func maybeResolveStartup(m Model) (Model, tea.Cmd) {
	if m.pendingPerimeter == nil {
		return m, nil
	}
	// Wait for atlas: either the index loaded, or load errored. Nil index
	// with nil err means we haven't heard back yet.
	if m.atlas.index == nil && m.atlas.loadErr == nil {
		return m, nil
	}
	pending := *m.pendingPerimeter
	m.pendingPerimeter = nil

	locCands := []location.Candidate{}
	if pending.HasLocationCandidate() {
		locCands = append(locCands, pending.LocationCandidate)
	}
	if pending.IANAPreseed != "" {
		// Carry the ZONE, not just the city name the zone was cut from. City
		// names repeat across the world — the atlas holds an Amsterdam in NL,
		// one in New York state and a New Amsterdam in Guyana — and the
		// resolver now refuses an ambiguous name rather than taking the first
		// atlas row. The zone is what tells it which one was meant, and it is
		// the same host fact that produced the name, so the two cannot drift.
		locCands = append(locCands, location.Candidate{
			Source: "iana-preseed",
			Place:  location.RawPlace{Name: pending.IANAPreseed, TZ: location.HostIANA()},
		})
	}

	// The resolver's per-candidate notes stay in locRes.Warnings and out of
	// the Log (operator, 2026-10-25): a guessed city is said once, by the
	// city line below, and an unresolved one opens the picker.
	locRes, locErr := location.Resolve(locCands, m.atlas.index)
	if locErr != nil {
		m.startup = startupAtlasOpen
		m.atlas.preseed = pending.IANAPreseed
		m.atlas.filterText = pending.IANAPreseed
		m.atlas.refilter()
		// Re-stash the pending perimeter so applyAtlasPicked can restore
		// the session's chart once the user disambiguates the city. Without
		// this the session's anchor silently disappears on the
		// unresolved-location startup path.
		stashed := pending
		m.pendingPerimeter = &stashed
		return m, nil
	}
	m.place = locRes.Place
	m.placeGuessed = locRes.Source == "iana-preseed"
	if m.placeGuessed {
		m = m.logNote("city", "%s, from the time zone — , c d saves it", locRes.Place.Display())
	}

	// Construct the workspace: one floating slot at (now, resolved place).
	// Refusal of an untimed place is impossible here — location.Resolve
	// already gates on HasTimezone — but we surface the error defensively.
	slots, err := store.NewSlots(m.clock.Now(), locRes.Place)
	if err != nil {
		m = m.logNote("slots", "%v — proceeding without workspace", err)
	} else {
		m.slots = slots
		m.cursorIdx = 0 // floating is index 0 when there's no anchor
		m.targetFloatingTime = slots.Floating().Time
	}

	// The last quit's workspace comes back. It is the only thing that anchors
	// at startup: without a readable session the workspace is floating-only
	// (C9).
	m, restored := m.restoreSession(pending)
	// A chart-file anchor owes its re-read once the first frame is out.
	recheck := m.sessionRecheckCmd(pending, restored)
	// Last, so the remembered marks land on the restored workspace.
	// The restored cursor and selections are an arrival: the tab starts on
	// the chart alone (S4a A27).
	m = m.applySessionSelections(pending).forgetTabMemory()

	m.startup = startupComputing
	// Cold start births the first workspace identity — route through
	// bumpIdentity (immediate compute, identity floor, no debounce).
	var recompute tea.Cmd
	m, recompute = m.bumpIdentity(surfaceText)
	// Batched, not sequenced: the re-read must not hold up the first frame,
	// which is the whole reason it runs after it. The floating chart starts
	// attached to the clock, so its first minute tick is armed with the
	// workspace.
	m, tick := m.armNowTick()
	return m, tea.Batch(recompute, recheck, tick)
}

// applyAtlasPicked transitions out of atlas-open and queues the compute. A pick
// lasts for the session and writes nothing; `<leader> c d` is what keeps a city.
//
// The live chart is cleared on transition, but Bodies and Aspects keep
// painting the previous committed frame until the picked place's frame
// commits (the specification), the way a cursor move does. Anchor
// state and store-owned selection survive the clear so buildTextCmd keeps its
// comparison route when a selected natal pair is present.
func applyAtlasPicked(m Model, msg AtlasPickedMsg) (Model, tea.Cmd) {
	var recheck tea.Cmd // set only on the cold start that had to ask for a city
	m.place = msg.Place
	m.placeGuessed = false
	m = m.clearChartKeepingHold()
	m = m.suspendDetails()
	m = m.rememberBodyRow()
	m.bodyCursorIdx = 0
	m = m.resetBodyTop()
	// Refresh the slot workspace for the newly picked place. Both flows
	// land here: cold-start-via-atlas (no prior slots) and post-startup
	// re-pick (slots exist; every anchored slot is preserved — the natal
	// charts do not move when the user changes their current city).
	m = m.refreshSlotsForPlace(msg.Place)

	// Cold-start path: if the unresolved-location branch in
	// maybeResolveStartup stashed pendingPerimeter, the session is still
	// waiting to be restored. Restore it now, so a cold start that had to ask
	// for the city does not silently lose the session's chart.
	if m.pendingPerimeter != nil {
		pending := *m.pendingPerimeter
		m.pendingPerimeter = nil
		// A cold start that had to ask for the city reaches restore HERE
		// instead, once there is a workspace to restore into (C13).
		var restored bool
		m, restored = m.restoreSession(pending)
		recheck = m.sessionRecheckCmd(pending, restored)
		// The restored cursor and selections are an arrival: the tab starts on
		// the chart alone (S4a A27).
		m = m.applySessionSelections(pending).forgetTabMemory()
	}

	m.startup = startupComputing
	// bumpIdentity hard-cancels any in-flight compute for the PRIOR city
	// (held-key sweep, initial startup dispatch) and fences off its already-
	// queued results via the commitFloor, so the previous city's frame can
	// never paint over the re-picked one. buildTextCmd reads the rebuilt
	// slots to choose overlay vs transit-only.
	var recompute tea.Cmd
	m, recompute = m.bumpIdentity(surfaceText)
	// A chart attached to the clock keeps its minute tick through a city
	// change; the timer armed for the old workspace retires by number.
	m, tick := m.armNowTick()
	return m, tea.Batch(recompute, recheck, tick)
}

// applyChartsLoaded stores the loaded chart library into the open picker (or
// records a load error) and refilters against whatever the user has typed so
// far. The picker renders a loading line until this arrives.
//
// A missing atlas is the one library failure that is not about any single file:
// it takes every chart down at once, and the fix is outside Sextile. It goes on
// the error lane so it survives in the Log and the console, beside the named
// rows the picker shows for the files themselves.
func applyChartsLoaded(m Model, msg ChartsLoadedMsg) (Model, tea.Cmd) {
	if msg.Gen != m.pickerGen {
		// Stale response from a superseded open — dropping it keeps a late
		// failure from latching over a newer successful load (the reviewer an earlier change MED).
		return m, nil
	}
	m.picker.loaded = true
	if msg.Err != nil {
		m.picker.loadErr = msg.Err
		m.picker.charts = nil
		m.picker.drops = nil
	} else {
		m.picker.loadErr = nil // a successful load clears any prior error
		m.picker.charts = msg.Charts
		m.picker.drops = msg.Drops
		m.picker.seen = msg.Seen
		// A listing that succeeded and listed a chart is evidence the folder is
		// readable: only then may a session write drop a deleted chart's record
		// (the specification C10).
		if len(msg.Charts) > 0 && msg.AtlasErr == nil {
			m.frames.pruneOK = true
		}
	}
	if msg.AtlasErr != nil {
		m = m.logError("Couldn't find atlas.as. Put it beside Astrolog and restart.")
	}
	m.picker.refilter()
	if m.picker.fit.Width == 0 {
		m.picker.fit = chartFit(m.picker, m.cfg)
	}
	if m.picker.land != "" {
		// A rename reloads the library and puts the cursor back on the chart
		// it renamed (the specification C4.3).
		for i, c := range m.picker.matches {
			if c.Source.File == m.picker.land {
				m.picker.cursor = i
				m.picker.scrollTo(i, chartBlockHeight(m.picker, modalListHeight(m.height)))
			}
		}
		m.picker.land = ""
	}
	return m, nil
}

// applyChartPicked starts the pick. A listing row carries no birth moment, so
// nothing is anchored here: the pick dispatches a resolve against the file the
// row named, bumps the pick generation, and leaves the picker open showing that
// it is reading. The anchor is set in applyChartResolved when — and only when —
// that resolve lands.
//
// Bumping the generation is what makes a second pick safe: the first resolve
// still returns, and is dropped whole.
func applyChartPicked(m Model, msg ChartPickedMsg) (Model, tea.Cmd) {
	if m.slots == nil {
		// No resolved workspace to anchor onto — restore the view we opened
		// from rather than resolving a chart we could not use. OpenCharts
		// gates on this too; this is defense in depth.
		m.startup = m.picker.prior
		return m, nil
	}
	if needsTitle(msg.Chart) {
		// the specification C3.4: an untitled chart asks for its title
		// first. Skipping the form comes back to resolvePick.
		return openTitleForm(m, msg.Chart), nil
	}
	return resolvePick(m, msg.Chart)
}

// resolvePick dispatches the resolve for a picked row: the pick itself, once
// any title form has been answered or skipped.
func resolvePick(m Model, c store.NamedChart) (Model, tea.Cmd) {
	return resolvePickIn(m, "", c)
}

// resolvePickIn is resolvePick reading the chart from the directory dir names
// (empty: the resolver's answer now). A pick that follows a write passes the
// directory the write went to.
func resolvePickIn(m Model, dir string, c store.NamedChart) (Model, tea.Cmd) {
	if m.slots == nil {
		m.startup = m.picker.prior
		return m, nil
	}
	m.pickGen++
	m.picker.resolving = true
	m.picker.resolveErr = nil
	m.picker.refusal = ""
	m.picker.pending = c
	return m, resolveChartCmdIn(dir, c, m.pickGen)
}

// applyChartResolved anchors the picked chart now that its authoritative moment
// is known: it sets the overlay handle and the anchored slot, selects the
// anchored+floating pair, and dispatches the comparison recompute. Picking a
// chart changes the workspace identity (a new anchored slot), so it clears the
// stale chart payload and goes through bumpIdentity — which cancels the prior
// anchor's in-flight compute and floors its queued results so a rapid re-pick
// never paints stale. The transit place and the floating-slot time are
// untouched; only the anchor changes.
//
// A failed resolve changes nothing at all. The anchor the operator was looking
// at stays exactly as it was, the picker stays open, and the reason goes on the
// error lane, where it survives in the Log and the console. The popup itself
// says nothing about it (the specification C5). A
// resolve that SUCCEEDED but produced a record that cannot anchor is refused
// the same way, and for the same reason: an explicit pick that quietly does
// nothing is indistinguishable from a broken key.
func applyChartResolved(m Model, msg ChartResolvedMsg) (Model, tea.Cmd) {
	if msg.Gen != m.pickGen {
		// Superseded by a later pick, or cancelled with Esc. Drop it whole:
		// no anchor, no state, no error line.
		return m, nil
	}
	m.picker.resolving = false
	if msg.Err != nil {
		m.picker.resolveErr = msg.Err
		return m.noteEphemeris(msg.Err).logError("Couldn't open that chart. %s", m.failureReason(msg.Err, pickFallback)), nil
	}
	if m.slots == nil {
		m.startup = m.picker.prior
		return m, nil
	}
	c := msg.Chart
	// The casting place is the one the read found for the file as it is NOW: a
	// file replaced since the listing gives its own place with its own moment.
	if msg.HasPlace {
		c.Place = msg.Place
	}
	if msg.Unplaced {
		return m.refuseAnchor("no known place")
	}
	// What the file says about its own houses is known before anything casts.
	// For a chart that is already open, the system it is shown in is compared
	// before and after, so a changed house line recasts it.
	var openBefore recipe.HouseSystem
	openIdx := m.slots.FindAnchored(store.SessionChart{Source: c.Source})
	if openIdx >= 0 {
		openBefore = m.ownHouses(m.slots.All()[openIdx]).hs
	}
	if f := filepath.Base(c.Source.File); c.Source.Kind == store.ChartKindFile && c.Source.File != "" {
		m.frames = m.frames.withFile(f, fileFrame{when: msg.When, houses: msg.Houses, has: msg.HasHouses})
	}
	// The record the session file will name this chart by, built BEFORE
	// anything moves so it can be checked before anything moves. The slot keeps
	// the moment and the place; only this remembers which library row they are.
	rec := store.SessionChart{ID: c.ID, Name: c.Name, When: msg.When, Place: c.Place, Source: c.Source, Label: msg.Label}
	if msg.HasPlace {
		// The title the read found, with the moment it found it beside: a file
		// replaced since the listing is named by what it holds now, and one
		// that became untitled is shown under its file stem and sends no name.
		if msg.Label.Name != "" {
			rec.Name = msg.Label.Name
		} else {
			base := filepath.Base(c.Source.File)
			rec.Name = strings.TrimSuffix(base, filepath.Ext(base))
		}
	}
	if reason := rec.UnusableReason(); reason != "" {
		// The library lists on a weaker bar than anchoring needs, by design: a
		// listing row carries no moment at all, so a file with no
		// `place:`, a `lat:` that parsed to NaN, or a zone this host cannot
		// load is pickable and still not castable. ingest answers that by
		// falling back to the session snapshot, which carries the same chart;
		// here there is no fallback and the operator chose this row explicitly,
		// so the pick refuses and names what is missing. Anchoring it anyway
		// leaves a second slot on the header that the comparison never
		// computes, which is the whole of an earlier change.
		return m.refuseAnchor(reason)
	}
	m.picker.resolveErr = nil
	m.picker.refusal = ""
	if i := m.slots.FindAnchored(rec); i >= 0 {
		// A chart already in the list: the cursor goes to its row and nothing
		// is added (M3). One file is one row. The pick just read the file, so if
		// its name, id, moment or place is not the slot's the file wins, in place: the slot keeps its
		// position, selection, body marks and Bodies row (C5).
		open := m.slots.All()[i]
		if open.Time.Equal(msg.When) && open.Location == c.Place && open.Chart.Name == rec.Name && open.Chart.ID == rec.ID && open.Chart.Label == rec.Label {
			// Nothing about the chart changed: the pick only moves the cursor.
			// The picker closes back to where it opened from; the settled frame
			// stays, and a recompute happens only if the cursor lands on a
			// different slot, exactly as ↑/↓ would.
			// The Charts panel takes the keyboard either way: the operator
			// just chose this chart, so the next ↑/↓ or Space is about it.
			m.startup = m.picker.prior
			recast := m.ownHouses(open).hs != openBefore
			if i == m.cursorIdx {
				m, cmd := m.setFocusedPanel(PanelChart)
				return recastAfter(m, cmd, recast)
			}
			m = m.focusSlot(i)
			m.focused = PanelChart
			m, cmd := m.afterFocusChange()
			return recastAfter(m, cmd, recast)
		}
		m = m.clearChartKeepingHold()
		m = m.suspendDetails()
		m = m.rememberBodyRow()
		if err := m.slots.ReplaceAnchored(i, rec, msg.When, c.Place); err != nil {
			m = m.logNote("slots", "anchor refused (%v)", err)
		}
		m = m.focusSlot(i)
	} else {
		m = m.clearChartKeepingHold()
		m = m.suspendDetails()
		// The row of the slot being left is remembered BEFORE the list changes:
		// the insert shifts what the cursor index points at.
		m = m.rememberBodyRow()
		if err := m.slots.AddAnchored(rec, msg.When, c.Place); err != nil {
			m = m.logNote("slots", "anchor refused (%v)", err)
		} else {
			// The new slot sits directly above the floating slot and takes
			// focus, leaving every other slot as it was (M2).
			m = m.focusSlot(m.slots.AnchoredCount() - 1)
			m.bodyCursorIdx = 0
			m = m.resetBodyTop()
		}
	}
	// The cursor arrived on a chart (an add, a replace): the Charts panel
	// takes the keyboard on it. Details was suspended above and the wheel repaints below, so
	// the focus change needs neither from setFocusedPanel.
	m.focused = PanelChart
	m = m.pruneTabMemory().noteArrival()
	m.startup = startupComputing
	var recompute, wheel tea.Cmd
	m, recompute = m.bumpIdentity(surfaceText)
	// The focused slot may have changed (an add, or a jump to an open chart):
	// an open wheel follows it the way it follows ↑/↓ (afterFocusChange).
	m, wheel = m.refreshWheelIfActive()
	return m, tea.Batch(recompute, wheel)
}

// refuseAnchor is the pick's one refusal: the picker names what the chart
// lacks, and the error lane says so.
func (m Model) refuseAnchor(reason string) (Model, tea.Cmd) {
	m.picker.refusal = reason
	return m.logError("Couldn't anchor that chart. It has %s.", reason), nil
}

// refreshSlotsForPlace rebuilds m.slots for the newly picked place,
// preserving any operator-controlled state from the prior workspace:
// the natal anchor (anchor doesn't move when the user changes city)
// AND the floating slot's Time (if the user time-stepped to a target
// moment, re-picking the city should keep them at that target rather
// than snap back to wall-clock now). Without preservation, an earlier change
// review round-3 noted that re-pick silently discards step navigation.
//
// Falls back to m.clock.Now() for the floating time when no prior
// workspace exists (true cold-start atlas-pick). Logs and leaves
// m.slots untouched if place lacks an IANA timezone — atlas picks
// always have one, so that branch is defensive.
func (m Model) refreshSlotsForPlace(place location.PlaceInfo) Model {
	previousSlots := m.slots
	var prevAnchored []*store.Slot
	// a review finding: preserve the OPERATOR'S TARGET
	// (m.targetFloatingTime) across re-pick, not just the committed
	// slot.Floating().Time. If the user time-stepped to T+5 and the
	// matching computes haven't returned yet, slot.Time is still T+0
	// — using slot.Time would silently snap back to the unstepped
	// moment when they change city. Use the target when set; fall
	// back to slot.Time (e.g. after a long settle) and finally to
	// clock.Now() for cold-start.
	floatingTime := m.clock.Now()
	if m.slots != nil {
		prevAnchored = m.slots.All()[:m.slots.AnchoredCount()]
		floatingTime = m.slots.Floating().Time
	}
	if !m.targetFloatingTime.IsZero() {
		floatingTime = m.targetFloatingTime
	}
	slots, err := store.NewSlots(floatingTime, place)
	if err != nil {
		m = m.logNote("slots", "%v — workspace not refreshed", err)
		return m
	}
	// The caller (applyAtlasPicked) has already remembered the focused slot's
	// row; remembering again here would read the reset cursor and overwrite it.
	m.slots = slots
	m.cursorIdx = 0
	m.targetFloatingTime = slots.Floating().Time
	// The whole list comes across, in order, each slot with its own chart. A
	// slot whose place cannot be re-added (never the case for one that was
	// added) is dropped and said so; the rest still carry over.
	oldRows := m.bodyRows
	m.bodyRows = nil
	// The tab's memory names slots, and these are new objects: each old slot
	// maps to its rebuilt counterpart (anchored by its place in the list,
	// floating to floating) so the base survives the new city.
	counterpart := map[*store.Slot]*store.Slot{}
	for _, prev := range prevAnchored {
		if err := m.slots.AddAnchored(prev.Chart, prev.Time, prev.Location); err != nil {
			m = m.logNote("slots", "anchor refused after refresh (%v)", err)
			continue
		}
		m.cursorIdx = m.slots.AnchoredCount() // floating sits behind the anchored slots
		fresh := m.slots.All()[m.slots.AnchoredCount()-1]
		counterpart[prev] = fresh
		m = m.rehomeRechecks(prev, fresh)
		if row, ok := oldRows[prev]; ok {
			m = m.keepBodyRow(fresh, row)
		}
	}
	m.slots.CopySelectionsFrom(previousSlots)
	// The cursor is on the floating slot, as it has always been after a city
	// re-pick, and now keeps the row it was left on like every other slot.
	if previousSlots != nil {
		old := previousSlots.All()
		counterpart[old[len(old)-1]] = m.slots.All()[len(m.slots.All())-1]
		if row, ok := oldRows[old[len(old)-1]]; ok {
			m = m.keepBodyRow(m.slots.All()[len(m.slots.All())-1], row)
		}
	}
	m.bodyCursorIdx = m.bodyRows[m.focusedSlot()]
	m = m.resetBodyTop()
	// A city change is not a tab choice: the base and the row arrived on go
	// over to the rebuilt slots, and only what has no counterpart is dropped.
	return m.remapTabMemory(counterpart)
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
	m, cmd := commitTextFrame(m, msg, m.isTickFrame(msg.WantID))
	// The first ready frame with no Details to show says what size would.
	if !m.limitedViewShown && m.startup == startupReady && m.geometry() != nil && !m.detailsVisible() {
		var noticeCmd tea.Cmd
		m.limitedViewShown = true
		m, noticeCmd = m.showNotice(limitedViewNotice(m.shownCount()))
		return m, tea.Batch(cmd, noticeCmd)
	}
	return m, cmd
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
	if quiet {
		m = m.setNowLine(msg.Commands)
	} else {
		m = m.logCommands(msg.Commands)
	}
	if msg.Err != nil {
		m = m.noteEphemeris(msg.Err).logError("Couldn't cast the chart. %s", m.failureReason(msg.Err, m.consolePointer(len(msg.Commands) > 0))).logWarnings(msg.Err)
	}
	m = m.reportUnavailable(msg.Unavailable)
	m.chart.positions = msg.Positions
	m.chart.stations = msg.Stations
	// One note per failed probe run: a burst coalesces into one commit, so the
	// note cannot repeat per key (the specification C5).
	if msg.StationErr != nil {
		m = m.logNote("bodies", "station marks unavailable (%v)", msg.StationErr)
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
	m.chart.settled = msg.Err == nil
	m = m.commitColumns(msg)
	m = m.commitCasts(msg.Casts)
	m = m.noteHouseSources()
	if rows := bodyRowCount(m.chart.positions, m.chart.cusps); rows == 0 {
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
	m = m.stampShown()
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
	if msg.Err == nil && textSettled && (detailsNeedsText || m.focused == PanelAspects) {
		m, detailsCmd = m.requestDetails()
	} else if msg.Err != nil && textSettled && m.details.waitForText && m.reportTabHidden() {
		// The report tab is hidden: the pending mark stays as it was and the
		// catch-up asks again when the report tab is chosen.
		m.details.stale = true
	} else if msg.Err != nil && textSettled && m.details.waitForText {
		// The new chart frame failed, so no matching Details report can run.
		// Fence pending work but retain the last successful Details frame.
		m.details.pending = false
		m.details.waitForText = false
		m = m.suspendSurface(surfaceDetails)
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
	if msg.Err == nil && (m.comparing() || m.focusedIsFloating()) {
		var imageCmd tea.Cmd
		m, imageCmd = m.refreshWheelIfActive()
		return m, tea.Batch(detailsCmd, imageCmd, timelineCmd)
	}
	return m, tea.Batch(detailsCmd, timelineCmd)
}

// reportUnavailable says, once, which selected objects this Astrolog cannot
// compute. Their rows were dropped at the parse rather than drawn at 0° Aries
// with no motion, which is what Astrolog returns for them — so without this
// line a body the operator ticked would simply never appear, and the selector
// would look broken in exactly the way a silently-dropped row always does.
//
// ONCE means once per change, not once per frame. The text surface recomputes
// on every time-step and every cursor move, and the same three names arriving
// sixty times during a held-key sweep would push the whole Log out of its six
// rows. The set is remembered and compared, so selecting Eris logs one line,
// stepping time logs none, and deselecting it logs the recovery.
func (m Model) reportUnavailable(names []string) Model {
	if sameStringSet(m.lastUnavailable, names) {
		return m
	}
	m.lastUnavailable = append([]string(nil), names...)
	if len(names) == 0 {
		// Recovery says nothing: the bodies are drawn again, and that is
		// what the operator sees (the specification).
		return m
	}
	return m.logNote("bodies", "your Astrolog has no ephemeris for %s — not drawn", strings.Join(names, ", "))
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
