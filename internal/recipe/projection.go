package recipe

import (
	"fmt"

	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// Projection is the output intent: what to take from a ChartSpec. It is a
// sealed sum type (the unexported isProjection marker closes the variant set to
// this package), so the compiler's mapping from Projection to astrolog mode is
// exhaustively known — adding a variant forces the compiler to handle it.
//
// A ChartSpec computed once can be paired with several Projections; only the
// projection-owned tail (mode, render, sink) differs between them.
type Projection interface {
	isProjection()
	// Label returns a short human label for diagnostics and plan labelling.
	Label() string
	// Validate reports whether the projection is well-formed. The compiler
	// calls it before lowering, so a degenerate projection (e.g. a zero-value
	// WheelImage) is rejected rather than silently compiled.
	Validate() error
}

// Positions takes the body-position list (astrolog's `-v0`).
type Positions struct{}

func (Positions) isProjection()   {}
func (Positions) Label() string   { return "positions" }
func (Positions) Validate() error { return nil }

// Aspects takes the influence-ordered aspect list (astrolog's `-a`).
type Aspects struct{}

func (Aspects) isProjection()   {}
func (Aspects) Label() string   { return "aspects" }
func (Aspects) Validate() error { return nil }

// RawReportKind names one of Astrolog's unparsed text reports. The zero value
// is invalid so an uninitialized RawReport cannot silently select a mode.
type RawReportKind uint8

const (
	RawReportChart RawReportKind = iota + 1
	RawReportBody
	RawReportAspects
)

// RawReport asks Astrolog for stdout exactly as it formats it. Construction is
// closed behind the functions below so only the three supported report shapes
// are representable; Body is meaningful only for RawReportBody.
type RawReport struct {
	kind RawReportKind
	body aspect.Body
}

func (RawReport) isProjection() {}

// ChartRawReport returns Astrolog's uncolored influence chart for the whole
// chart: per-object positional power, aspect power, total, rank and
// percent, followed by the sign/element/mode and house/quadrant tables. It
// replaced the `-v` position listing, which only repeated what the Bodies
// panel already shows.
func ChartRawReport() RawReport { return RawReport{kind: RawReportChart} }

// BodyRawReport returns Astrolog's uncolored aspect report restricted to body
// b (`-a -RO <body>`). Validate rejects an unknown body at compile time.
func BodyRawReport(b aspect.Body) RawReport {
	return RawReport{kind: RawReportBody, body: b}
}

// AspectsRawReport returns Astrolog's uncolored aspect listing (`-a`).
func AspectsRawReport() RawReport { return RawReport{kind: RawReportAspects} }

func (r RawReport) Kind() RawReportKind { return r.kind }
func (r RawReport) Body() aspect.Body   { return r.body }

func (r RawReport) Label() string {
	switch r.kind {
	case RawReportChart:
		return "raw-chart"
	case RawReportBody:
		return "raw-body-" + r.body.String()
	case RawReportAspects:
		return "raw-aspects"
	default:
		return "raw-report-invalid"
	}
}

func (r RawReport) Validate() error {
	switch r.kind {
	case RawReportChart, RawReportAspects:
		if r.body != aspect.BodyUnknown {
			return fmt.Errorf("recipe: RawReport: body is only valid for a body report")
		}
		return nil
	case RawReportBody:
		if !knownBody(r.body) {
			return fmt.Errorf("recipe: RawReport: %v is not a known body", r.body)
		}
		return nil
	default:
		return fmt.Errorf("recipe: RawReport: unknown kind %d", int(r.kind))
	}
}

// WheelImage renders the chart as a graphical wheel image. It carries only the
// render styling and format — NOT an output path: where the
// image is delivered is an execution/preview concern, not part of "what to
// compute".
type WheelImage struct {
	Render RenderSpec
}

func (WheelImage) isProjection()     {}
func (WheelImage) Label() string     { return "wheel-image" }
func (w WheelImage) Validate() error { return w.Render.validate() }

// RenderSpec is the typed styling for a wheel image — a curated subset of
// astrolog's `-X*` switches, not the whole family. Zero value is not intended
// for use; build one via DefaultWheelRender and adjust.
type RenderSpec struct {
	Format        ImageFormat // native Astrolog output format; zero is SVG
	Width         int         // -Xw (horizontal pixels)
	SymbolScale   int         // -Xs (object symbol size, percent)
	TextScale     int         // -XS (text size, percent)
	Enhanced      bool        // -Xi (slightly modified / enhanced graphic form)
	AspectGlyphs  bool        // -XA (draw aspect glyphs over aspect lines)
	ReverseColors bool        // -Xr (white background)
	NoBorder      bool        // -Xu (suppress the outer border)
	NoInfoText    bool        // -Xt (suppress the info text band)
	ThickLines    bool        // -Xx (thicker lines) + -Xx0 (antialiased)
	DegreeWheel   bool        // -XL (degree wheel overlay)

	// Highlight is the selected/focused-body highlight. The
	// zero value means no highlight: the wheel renders exactly as it did
	// before the highlight existed.
	Highlight HighlightSpec
}

// ImageFormat selects the native image format Astrolog writes.
type ImageFormat uint8

const (
	ImageSVG ImageFormat = iota
	ImagePNG
)

// HighlightSpec names the wheel's highlight members: the selected bodies plus the focused
// cursor body. The compiler lowers an active highlight into the three-layer
// composite plan — a plain base wheel, a show-list foreground of exactly
// these members, and a furniture layer — and internal/compose dims the base
// uniformly as vector groups. Every member renders at Astrolog's own
// colors; the operator dropped a special cursor tint (2026-08-23), so the
// spec carries membership and nothing else.
type HighlightSpec struct {
	Selected BodySet     // bodies rendered at full color over the dimmed base
	Focused  aspect.Body // cursor body; BodyUnknown = no focused body
}

// Active reports whether any highlight is requested at all. An inactive spec
// must compile to a wheel identical to one with no highlight fields.
func (h HighlightSpec) Active() bool {
	return len(h.Selected) > 0 || h.Focused != aspect.BodyUnknown
}

// NewHighlight builds the shipped highlight for a selected set and a focused
// body. Pass BodyUnknown for no focused body. Membership is deliberately not
// validated: bodies the chart does not draw (or that Astrolog cannot number)
// are silently omitted by the compiler, per the failure contract.
func NewHighlight(selected []aspect.Body, focused aspect.Body) HighlightSpec {
	return HighlightSpec{
		Selected: NewBodySet(selected...),
		Focused:  focused,
	}
}

// validate rejects a degenerate render. A wheel needs a positive width — the
// zero value (which would compile to a bare image selector and let astrolog pick a
// default size) is a caller mistake, not a valid request. Any explicitly set
// scale must also be positive. Build a render via DefaultWheelRender.
func (r RenderSpec) validate() error {
	if r.Format != ImageSVG && r.Format != ImagePNG {
		return fmt.Errorf("recipe: RenderSpec: unknown image format %d", r.Format)
	}
	if r.Width <= 0 {
		return fmt.Errorf("recipe: RenderSpec: Width must be > 0 (got %d). Use DefaultWheelRender", r.Width)
	}
	if r.SymbolScale < 0 || r.TextScale < 0 {
		return fmt.Errorf("recipe: RenderSpec: scales must be >= 0 (SymbolScale=%d TextScale=%d)", r.SymbolScale, r.TextScale)
	}
	return nil
}

// DefaultWheelRender returns the wheel styling Sextile ships, matching the
// legacy lazyastro graphic: 1200px, 400% symbols/text, aspect glyphs, reversed
// (light) colours, no border, no info band, thick antialiased lines, degree
// wheel.
func DefaultWheelRender() RenderSpec {
	return RenderSpec{
		Width:         1200,
		SymbolScale:   400,
		TextScale:     400,
		Enhanced:      true,
		AspectGlyphs:  true,
		ReverseColors: true,
		NoBorder:      true,
		NoInfoText:    true,
		ThickLines:    true,
		DegreeWheel:   true,
	}
}
