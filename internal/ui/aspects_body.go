package ui

import (
	"github.com/cruiserxyz/sextile/internal/ui/panel"
)

// renderAspectsBody returns the contents (no border) of the [3] Aspects panel
// for the active tab: the focused slot's own power-sorted waterfall on
// TabAspects, the active pair's cross waterfall on a pair tab (TabAspectsCross
// for the first pair, aspects-pair-N for the rest; see aspects_tabs.go).
func renderAspectsBody(state chartViewState, active TabID, innerWidth, innerHeight int, cfg Config) string {
	render, ok := aspectsRenderers[active]
	if !ok {
		render = aspectsRenderers[TabAspects]
		if isAspectsPairTab(active) {
			render = panel.RenderAspectsTabCross
		}
	}
	bag := resolveAspectsBag(state)
	if TabID(bag.Mark.Tab) != active {
		bag.Mark = panel.EnteredMark{} // the marks stay on the row's own tab (rule 5)
	}
	return render(bag, innerWidth, innerHeight, cfg)
}
