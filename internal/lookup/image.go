package lookup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	_ "github.com/cruiserxyz/sextile/internal/env" // init() seeds SEXTILE_WHEEL_HIGHLIGHT from .env
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// WheelHighlight is the ui-facing highlight state for a wheel render:
// the focused slot's selected bodies plus the Bodies-panel cursor body.
// BodyUnknown means no focused body. The zero value renders the plain wheel.
// lookup lowers it into the recipe highlight with the shipped tier colors —
// the ui speaks bodies, never Astrolog color indices.
//
// The ui still computes and passes this state while the composite is parked
// ; lookup simply declines to lower it unless the opt-in is set, so
// the wiring stays warm for pickup.
type WheelHighlight struct {
	Selected []aspect.Body
	Focused  aspect.Body
}

// RenderWheelImage renders the chart for `when` at `place` under `lens` as a
// wheel-image SVG, returning the raw bytes. It is the image analogue of
// FirstCallTransits: the ui hands the focused slot's moment + place (plus its
// highlight state), lookup derives the astrolog primitives (the same way the
// position/aspect path does), and astrolog compiles the WheelImage projection
// and runs it.
func RenderWheelImage(ctx context.Context, when time.Time, place location.PlaceInfo, lens Lens, highlight WheelHighlight) ([]byte, error) {
	if !place.HasTimezone() {
		return nil, fmt.Errorf("lookup: RenderWheelImage: place %q has no IANA timezone", place.Display())
	}
	jd := astrolog.SlotJD(when)
	spec, err := lens.specAt(jd, place)
	if err != nil {
		return nil, fmt.Errorf("lookup: RenderWheelImage: %w", err)
	}
	render := recipe.DefaultWheelRender()
	render.Format = previewImageFormat()
	if wheelHighlightEnabled() {
		render.Highlight = recipe.NewHighlight(highlight.Selected, highlight.Focused)
	}
	return astrolog.RenderWheelImage(ctx, spec, render)
}

// wheelHighlightEnvVar opts the parked highlight composite back in. The
// The composite ships DISABLED: with this variable
// unset the wheel is one plain Astrolog render — byte-identical to the image
// that shipped before the highlight existed — no matter what the operator has
// selected. Set it to 1/true/on/yes, in the environment or the repo .env, to
// restore the dimmed base plus selected-body aspect web.
//
// The switch lives at this single seam, where ui highlight state becomes a
// recipe highlight, precisely so the parked path stays whole: recipe, the
// compiler, and internal/compose still build and test the three-layer plan
// unchanged, and pickup is one variable rather than an excavation.
const wheelHighlightEnvVar = "SEXTILE_WHEEL_HIGHLIGHT"

// wheelHighlightEnabled reports whether the parked composite is opted in.
// The environment is read per call — a wheel render already costs at least
// one astrolog process, so the lookup is free — which keeps t.Setenv honest
// and lets the operator flip the switch without a rebuild.
func wheelHighlightEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(wheelHighlightEnvVar))) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

// RenderOverlayWheelImage renders the transit-over-natal BI-WHEEL: the anchored
// natal chart is passed first and Astrolog draws it on the outer ring with its
// houses, the transit moment (`transitWhen` at `transitPlace`) inside, under
// `lens`. It is the overlay analogue of RenderWheelImage,
// building a recipe.Comparison spec (the same two-chart input the cross-aspect
// path uses) and letting astrolog compile the WheelImage projection over it
// into the -r0 native-SVG bi-wheel.
func RenderOverlayWheelImage(ctx context.Context, natalChart store.AnchorChart, transitWhen time.Time, transitPlace location.PlaceInfo, lens Lens) ([]byte, error) {
	if natalChart.IsZero() {
		return nil, fmt.Errorf("lookup: RenderOverlayWheelImage: natal chart is zero")
	}
	if !natalChart.Place.HasTimezone() {
		return nil, fmt.Errorf("lookup: RenderOverlayWheelImage: natal place %q has no IANA timezone", natalChart.Place.Display())
	}
	if !transitPlace.HasTimezone() {
		return nil, fmt.Errorf("lookup: RenderOverlayWheelImage: transit place %q has no IANA timezone", transitPlace.Display())
	}
	spec, err := lens.comparisonSpecAt(
		astrolog.SlotJD(natalChart.When), natalChart.Place,
		astrolog.SlotJD(transitWhen), transitPlace,
	)
	if err != nil {
		return nil, fmt.Errorf("lookup: RenderOverlayWheelImage: %w", err)
	}
	// A comparison bi-wheel renders with no highlight switches at all; the compiler suppresses them for comparison sources anyway.
	render := recipe.DefaultWheelRender()
	render.Format = previewImageFormat()
	return astrolog.RenderWheelImage(ctx, spec, render)
}

// WheelRing is one chart of a multi-wheel: a moment at a place. A chart is
// exactly that, so an anchored slot and the floating slot are both rings.
type WheelRing struct {
	When  time.Time
	Place location.PlaceInfo
}

// RenderMultiWheelImage renders the selected set as concentric rings, in
// numeral order: rings[0] (chart 1) innermost and carrying the houses
// (astrolog -r3..-r6, `-i <f1> -i2 <f2> … -rN`). It carries the lens's object
// and aspect band exactly as RenderOverlayWheelImage does.
//
// A set of two is not a multi-wheel: it compiles to today's bi-wheel (-r0,
// chart 1 the baseline), the same argv RenderOverlayWheelImage produces, so
// a caller holding a selected set can always call this one function. Fewer
// than two or more than six rings is an error.
func RenderMultiWheelImage(ctx context.Context, rings []WheelRing, lens Lens) ([]byte, error) {
	img, _, err := RenderMultiWheelImageCommands(ctx, rings, lens)
	return img, err
}

// RenderMultiWheelImageCommands is RenderMultiWheelImage that also returns the
// command of every spawn that started, in order, for the console's argv lane
// (gate 12) — on failure too.
func RenderMultiWheelImageCommands(ctx context.Context, rings []WheelRing, lens Lens) ([]byte, []string, error) {
	if n := len(rings); n < 2 || n > recipe.MaxRings {
		return nil, nil, fmt.Errorf("lookup: RenderMultiWheelImage: %d rings, want 2 to %d", n, recipe.MaxRings)
	}
	jds := make([]float64, len(rings))
	places := make([]location.PlaceInfo, len(rings))
	for i, r := range rings {
		if !r.Place.HasTimezone() {
			return nil, nil, fmt.Errorf("lookup: RenderMultiWheelImage: ring %d place %q has no IANA timezone", i+1, r.Place.Display())
		}
		jds[i], places[i] = astrolog.SlotJD(r.When), r.Place
	}
	var spec recipe.ChartSpec
	var err error
	if len(rings) == 2 {
		spec, err = lens.comparisonSpecAt(jds[0], places[0], jds[1], places[1])
	} else {
		spec, err = lens.multiWheelSpecAt(jds, places)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("lookup: RenderMultiWheelImage: %w", err)
	}
	render := recipe.DefaultWheelRender()
	render.Format = previewImageFormat()
	return astrolog.RenderWheelImageCommands(ctx, spec, render)
}

// hostedWheelFormat is the image format the HTTP page is hosted in: the
// operator's wheel_format preference, SVG until the ui says otherwise. Desktop
// viewers ignore it.
var hostedWheelFormat atomic.Int32

// SetWheelFormat sets the format the next wheel render produces for the HTTP
// page (recipe.ImageSVG or recipe.ImagePNG). The ui calls it from the
// operator's wheel_format preference each time the wheel is opened.
func SetWheelFormat(f recipe.ImageFormat) { hostedWheelFormat.Store(int32(f)) }

// previewImageFormat keeps the Astrolog output aligned with the selected
// display backend. Desktop viewers receive native PNG so xdg-open resolves
// the normal raster image association; the HTTP page (http, and serve for the
// image) receives the format the operator chose, SVG by default.
func previewImageFormat() recipe.ImageFormat {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SEXTILE_PREVIEW_MODE"))) {
	case "http", "serve":
		return recipe.ImageFormat(hostedWheelFormat.Load())
	default:
		return recipe.ImagePNG
	}
}

// StagedRing reports which ring's chart file could not be staged when err is
// that failure (astrolog.StagingError): the 1-based position in the render's
// own ring order. The ui maps it to the selected slot that sat there. Any error
// in the chain with a `StagedRing() int` method answers it.
func StagedRing(err error) (int, bool) {
	var staged interface{ StagedRing() int }
	if errors.As(err, &staged) {
		return staged.StagedRing(), true
	}
	return 0, false
}
