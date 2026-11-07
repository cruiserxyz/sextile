package ui

import (
	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/lookup"
	"github.com/cruiserxyz/sextile/internal/ui/panel"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// chartViewState carries the parsed astrolog output the bodies + aspects
// panels render. Empty when the startup state is anything but ready.
//
// The panels always show the FOCUSED slot's single chart (positions +
// intra-aspects); overlay mode (S5b) re-skins them: the `overlay` marker adds
// the transit→natal cross waterfall. positions carries the focused slot's
// bodies in BOTH modes; Bodies never adds an inline overlay heading.
type chartViewState struct {
	positions []aspect.Position
	// stations are the focused chart's station marks (D38), committed with
	// positions in one frame; a frame held on screen keeps its own.
	stations map[int]lookup.Station
	// forced is the entered station row's word for its body, for THIS paint
	// only (enteredStation); it outranks stations and is drawn deepest red.
	forced map[int]lookup.Station
	// cusps is the FOCUSED chart's own house frame, parsed from the lookup
	// that produced positions; Bodies lists its twelve cusps under the rule.
	// The zero value means "no frame": only the angles are listed.
	cusps        aspect.Cusps
	aspects      []aspect.Aspect
	bodyStyles   astrologstyle.Bodies
	aspectStyles astrologstyle.Aspects
	// crossAspects is the transit→natal overlay aspect list (astrolog -r0).
	// Populated only on the overlay branch; rendered by the Aspects panel's
	// Transit→Natal tab. aspects is populated on that branch too (the
	// focused slot's own aspects), so the Aspects tab stays live.
	crossAspects      []lookup.OverlayAspect
	crossAspectStyles astrologstyle.Aspects
	// aspectsReport / crossAspectsReport are the two lists exactly as Astrolog
	// printed them (the -aa and -rt -aa stdout the rows above were parsed
	// from). Details shows the one behind the active Aspects tab.
	aspectsReport      string
	crossAspectsReport string
	// crossSynastry marks crossAspects as the pair of two anchored charts
	// (Astrolog -r0): its applying/separating verdict is a synthetic one and
	// is never displayed.
	crossSynastry bool
	// overlay marks an overlay-mode frame. Explicit (not derived from a
	// populated slice) so an overlay with zero cross-aspects is still an
	// overlay frame. Since an earlier change it is the frame marker only — the first
	// overlay commit after a non-overlay frame activates the cross tab — and
	// no longer a render switch: the Aspects body follows the active tab.
	overlay bool
	// settled is true when the frame on screen is a compute that committed
	// with no error. Only a settled frame's empty list says so; before the
	// first frame and after a failed one the Aspects tabs stay blank
	// (the specification A6).
	settled bool
	// natalPositions and natalCusps describe the ANCHORED chart, populated
	// on the overlay branch. Together they let a moving body's row show
	// where that body sat natally and which of the client's houses it is
	// transiting now. natalCusps is the frame; natalPositions supplies the
	// natal house per body, matched by identity.
	natalPositions []aspect.Position
	natalCusps     aspect.Cusps
	// framedPositions are the moving bodies in the frame's zodiac, when
	// theirs is another (FirstFrameReadyMsg.FramedPositions).
	framedPositions []aspect.Position
	// columns are the comparing frame's Bodies columns, one per chart of the
	// shown set it was computed for (columnsFor); results is the cache of
	// position lookups that lets the next frame recompute only what moved.
	columns    []panel.BodyColumn
	columnsFor columnsFor
	results    resultCache
	// epoch is non-zero once a text commit has filled the chart; a hard reset
	// zeroes it with everything else (shown.go: holding).
	epoch uint64
	// mark is the entered row's marks for THIS paint only: Model.paintedChart
	// derives it from the scene each time it draws, and a committed frame never
	// carries one.
	mark panel.EnteredMark
	// lit is the object the enter-colour experiment lights on the Bodies panel
	// (the entered aspect row's led object), for THIS paint only, like mark; the
	// colour is mark.Colour.
	lit aspect.Object
	// houseForm is the house display the operator chose, for THIS paint only,
	// like mark: Model.paintedChart fills it from the Model, so a committed
	// frame never carries one and a hard reset cannot lose the choice.
	houseForm panel.HouseForm
}
