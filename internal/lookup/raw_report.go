package lookup

import (
	"context"
	"fmt"
	"time"

	"github.com/cruiserxyz/sextile/internal/astrolog"
	"github.com/cruiserxyz/sextile/internal/location"
	"github.com/cruiserxyz/sextile/internal/recipe"
	"github.com/cruiserxyz/sextile/internal/store"
	"github.com/cruiserxyz/sextile/pkg/aspect"
)

// RawReportKind is the report context lookup exposes to the UI. lookup maps it
// to recipe's sealed projection while retaining ownership of ChartSpec
// construction; the UI never assembles Astrolog switches or recipe sources.
type RawReportKind uint8

const (
	RawReportChart RawReportKind = iota + 1
	RawReportBody
	// RawReportAspects is the chart's -aa list. Since an earlier change the Details panel
	// takes that list from the frame it was parsed from (Result.AspectsReport)
	// instead of spawning this; the kind stays for callers that want the
	// report on its own (a CLI, a debug dump), never for the Aspects panel.
	RawReportAspects
)

// RawReportRequest selects a single-chart raw report. Body is required only
// for RawReportBody and rejected for the other kinds.
type RawReportRequest struct {
	Kind RawReportKind
	Body aspect.Body
	// Label is the chart's name and place as its report header prints them
	// (the specification C1); the zero value is the unnamed chart.
	Label store.ReportLabel
}

func (r RawReportRequest) projection() (recipe.RawReport, error) {
	var p recipe.RawReport
	switch r.Kind {
	case RawReportChart:
		if r.Body != aspect.BodyUnknown {
			return p, fmt.Errorf("body is only valid for a body report")
		}
		p = recipe.ChartRawReport()
	case RawReportBody:
		p = recipe.BodyRawReport(r.Body)
	case RawReportAspects:
		if r.Body != aspect.BodyUnknown {
			return p, fmt.Errorf("body is only valid for a body report")
		}
		p = recipe.AspectsRawReport()
	default:
		return p, fmt.Errorf("unknown report kind %d", int(r.Kind))
	}
	if err := p.Validate(); err != nil {
		return recipe.RawReport{}, err
	}
	return p, nil
}

// ComputeRawReport returns an unparsed, uncolored Astrolog report for one
// moment at one place under lens, together with the command that produced
// it — carried from astrolog unchanged, and present beside the
// error whenever the spawn happened.
func ComputeRawReport(ctx context.Context, when time.Time, place location.PlaceInfo, lens Lens, request RawReportRequest) (report string, command string, err error) {
	if !place.HasTimezone() {
		return "", "", fmt.Errorf("lookup: ComputeRawReport: place %q has no IANA timezone", place.Display())
	}
	projection, err := request.projection()
	if err != nil {
		return "", "", fmt.Errorf("lookup: ComputeRawReport: %w", err)
	}
	spec, err := lens.localSpecAt(astrolog.SlotJD(when), place, request.Label)
	if err != nil {
		return "", "", fmt.Errorf("lookup: ComputeRawReport: %w", err)
	}
	return astrolog.ComputeRawReport(ctx, spec, projection)
}

// CrossBodyRawReport returns Astrolog's uncolored transit→natal cross-aspect
// list restricted to one body: the same `-rt … -aa` relationship run
// the Aspects panel reads, plus `-RO <n>`, so the report keeps real power,
// real orb and Astrolog's own applying/separating verdict.
//
// The restriction is by OBJECT, not by role: every row where that body is
// either end survives — the natal Sun's transits and the transiting Sun's
// contacts alike. That is what an astrologer asks for when they put the
// cursor on the Sun during a transit reading, and it is Astrolog's own
// semantics for -RO, not a Sextile rule layered on top.
func CrossBodyRawReport(
	ctx context.Context,
	lens Lens,
	natalChart store.AnchorChart,
	transitWhen time.Time,
	transitPlace location.PlaceInfo,
	body aspect.Body,
) (report string, command string, err error) {
	if !transitPlace.HasTimezone() {
		return "", "", fmt.Errorf("lookup: CrossBodyRawReport: place %q has no IANA timezone", transitPlace.Display())
	}
	if !natalChart.Place.HasTimezone() {
		return "", "", fmt.Errorf("lookup: CrossBodyRawReport: natal place %q has no IANA timezone", natalChart.Place.Display())
	}
	projection := recipe.BodyRawReport(body)
	if err := projection.Validate(); err != nil {
		return "", "", fmt.Errorf("lookup: CrossBodyRawReport: %w", err)
	}
	spec, err := lens.localComparisonSpecAt(
		astrolog.SlotJD(natalChart.When), natalChart.Place, natalChart.Label,
		astrolog.SlotJD(transitWhen), transitPlace, store.ReportLabel{},
	)
	if err != nil {
		return "", "", fmt.Errorf("lookup: CrossBodyRawReport: %w", err)
	}
	return astrolog.ComputeRawReport(ctx, spec, projection)
}

// SynastryCrossBodyRawReport is CrossBodyRawReport for two ANCHORED charts: the
// same `-RO <n>` restriction over a `-r0 -aa` relationship run (Relation
// Synastry), chart 1 (natalChart, the baseline) first, both staged charts in
// their own local clock.
func SynastryCrossBodyRawReport(
	ctx context.Context,
	lens Lens,
	natalChart store.AnchorChart,
	otherWhen time.Time,
	otherPlace location.PlaceInfo,
	otherLabel store.ReportLabel,
	body aspect.Body,
) (report string, command string, err error) {
	if !otherPlace.HasTimezone() {
		return "", "", fmt.Errorf("lookup: SynastryCrossBodyRawReport: place %q has no IANA timezone", otherPlace.Display())
	}
	if !natalChart.Place.HasTimezone() {
		return "", "", fmt.Errorf("lookup: SynastryCrossBodyRawReport: chart 1 place %q has no IANA timezone", natalChart.Place.Display())
	}
	projection := recipe.BodyRawReport(body)
	if err := projection.Validate(); err != nil {
		return "", "", fmt.Errorf("lookup: SynastryCrossBodyRawReport: %w", err)
	}
	spec, err := lens.localSynastrySpecAt(
		astrolog.SlotJD(natalChart.When), natalChart.Place, natalChart.Label,
		astrolog.SlotJD(otherWhen), otherPlace, otherLabel,
	)
	if err != nil {
		return "", "", fmt.Errorf("lookup: SynastryCrossBodyRawReport: %w", err)
	}
	return astrolog.ComputeRawReport(ctx, spec, projection)
}
