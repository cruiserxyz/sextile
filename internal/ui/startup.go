package ui

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/lookup"
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
	// startupExperimentsOpen is the hidden Experiments popup (experiments_popup_model.go).
	startupExperimentsOpen startupState = "experiments-open"
	// startupSessionNameOpen is the Sessions list's name field: Save as, or a
	// rename (sessions_popup_model.go).
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
// spawning astrolog — the same idiom as objectRegistryRead below.
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
