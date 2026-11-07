package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/internal/ui/term"
)

// resolveAspectsBag lifts the committed frame into the bag the renderers
// consume. It is the one seam between chartViewState and the tab renderers.
func resolveAspectsBag(state chartViewState) panel.AspectsBag {
	return panel.AspectsBag{
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
var aspectsRenderers = map[TabID]func(panel.AspectsBag, int, int, term.Style) string{
	TabAspects:      panel.RenderAspectsTabRows,
	TabAspectsCross: panel.RenderAspectsTabCross,
}
