package ui

import (
	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// aspectsBag is the typed data bag every Aspects tab renderer reads. The
// field names are the bag's public contract: a later text/template renderer
// reads the same fields, so nothing here may depend on Model or the frame
// type — resolveAspectsBag is the only function allowed to see the frame.
type aspectsBag struct {
	Rows        []aspect.Aspect       // the focused slot's own aspects
	RowStyles   astrologstyle.Aspects // Astrolog's per-aspect SGR for Rows
	Cross       []lookup.OverlayAspect
	CrossStyles astrologstyle.Aspects // keyed primary/natal first, as -r0 emitted it
	Synastry    bool                  // Cross is a pair of two anchored charts: no phase is shown
	Settled     bool                  // the frame committed with no error (A6)
	// Mark is the entered row's cell on the tab being drawn; zero marks
	// nothing (renderAspectsBody clears it for any other tab).
	Mark enteredMark
}

// resolveAspectsBag lifts the committed frame into the bag the renderers
// consume. It is the one seam between chartViewState and the tab renderers.
func resolveAspectsBag(state chartViewState) aspectsBag {
	return aspectsBag{
		Rows:        state.aspects,
		RowStyles:   state.aspectStyles,
		Cross:       state.crossAspects,
		CrossStyles: state.crossAspectStyles,
		Synastry:    state.crossSynastry,
		Settled:     state.settled,
		Mark:        state.mark,
	}
}

// aspectsRenderers binds each Aspects tab to the renderer that turns the
// resolved bag into the panel body. Adding a tab is one row here plus its
// entry in panelTabs.
var aspectsRenderers = map[TabID]func(aspectsBag, int, int, Config) string{
	TabAspects:      renderAspectsTabRows,
	TabAspectsCross: renderAspectsTabCross,
}

// renderAspectsTabRows renders the focused slot's own aspects through the
// single-chart power-sorted waterfall.
func renderAspectsTabRows(bag aspectsBag, innerWidth, innerHeight int, cfg Config) string {
	if len(bag.Rows) == 0 {
		return emptyLine(bag.Settled, ownEmptyLine)
	}
	return renderAspectWaterfallMarked(bag.Rows, bag.RowStyles, innerWidth, innerHeight, cfg, bag.Mark)
}

// ownEmptyLine and crossEmptyLine are the whole body of a tab whose settled
// frame has no aspects: an explicit statement rather than a blank panel
// (the specification A6).
const (
	ownEmptyLine   = "(no aspects in orb)"
	crossEmptyLine = "(no cross-aspects in orb)"
)

// emptyLine is line for a settled frame, and nothing while the frame is
// still to come or failed: a blank panel is waiting, a line is an answer.
func emptyLine(settled bool, line string) string {
	if !settled {
		return ""
	}
	return line
}

// renderAspectsTabCross renders the active pair's cross-aspects through the
// cross waterfall over the full inner height. The tab strip already names the
// pair (`② in ①`: the left glyph of every cell is ①'s, the base, the right
// one ②'s, the chart under the cursor), so no in-body header row is spent on it. A pair of
// two anchored charts shows no applying/separating phase.
func renderAspectsTabCross(bag aspectsBag, innerWidth, innerHeight int, cfg Config) string {
	if len(bag.Cross) == 0 {
		return emptyLine(bag.Settled, crossEmptyLine)
	}
	return renderCrossAspectWaterfallMarked(bag.Cross, bag.CrossStyles, innerWidth, innerHeight, cfg, !bag.Synastry, bag.Mark)
}
