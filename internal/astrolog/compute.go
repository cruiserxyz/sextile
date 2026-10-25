package astrolog

import (
	"context"
	"fmt"
	"math"

	"github.com/cruiserxyz/sextile/internal/astrologstyle"
	"github.com/cruiserxyz/sextile/internal/payload"
	"github.com/cruiserxyz/sextile/internal/recipe"
)

// ComputeChart compiles a chart's positions + aspects from a validated semantic
// spec and runs astrolog. It is the entry for the natal-UNSET + location-known
// first frame: lookup builds the recipe.ChartSpec (who/when/where + reference
// frame + body policy) and this lowers it through Compile, which owns switch
// ordering and the toggle/last-wins semantics. Run appends `-a` for the second
// (aspects) invocation, so the positions argv and the aspects argv share one
// chart prefix.
//
// Returns Run's payload.RawOutput unchanged: Positions, Aspects, and the merged
// Raw stdout for diagnostics.
func ComputeChart(ctx context.Context, spec recipe.ChartSpec) (payload.RawOutput, error) {
	plan, err := Compile(spec, recipe.Positions{})
	if err != nil {
		return payload.RawOutput{}, fmt.Errorf("astrolog: ComputeChart: %w", err)
	}
	return Run(ctx, plan.Invocations[0].Call())
}

// ComputeChartPositionsOnly is ComputeChart minus the second `-aa` invocation:
// positions only, Aspects empty. It backs ComputeCrossAspects' ephemeris
// probe and the lookup Probers seam (the t+ε applying probes that motivated
// it, the, were retired by an earlier change).
//
// argv comes from the same Compile path as ComputeChart (so the switch ordering
// and toggle semantics are identical); only the Run vs RunPositionsOnly choice
// differs.
func ComputeChartPositionsOnly(ctx context.Context, spec recipe.ChartSpec) (payload.RawOutput, error) {
	plan, err := Compile(spec, recipe.Positions{})
	if err != nil {
		return payload.RawOutput{}, fmt.Errorf("astrolog: ComputeChartPositionsOnly: %w", err)
	}
	return RunPositionsOnly(ctx, plan.Invocations[0].Call())
}

// rawReportDegreeSwitch is `-k0`: the degree character without colour.
const rawReportDegreeSwitch = "-k0"

// ComputeRawReport runs one typed RawReport projection and returns Astrolog's
// stdout, together with the command it executed. It deliberately
// bypasses Run/RunPositionsOnly: those paths add `-k` to extract body colors
// and parse typed chart payloads, while Details needs Astrolog's own plain
// text: `-k0` (the degree character, no colour) and reportText's one byte
// translation are all that touch it.
//
// The command is returned on the failure path too whenever the spawn
// happened; it is empty when the error came before any process started.
func ComputeRawReport(ctx context.Context, spec recipe.ChartSpec, projection recipe.RawReport) (report string, command string, err error) {
	plan, err := Compile(spec, projection)
	if err != nil {
		return "", "", fmt.Errorf("astrolog: ComputeRawReport: %w", err)
	}
	inv := plan.Invocations[0]
	if inv.Artifact != TextStdout || inv.Parser != ParseRawReport {
		return "", "", fmt.Errorf("astrolog: ComputeRawReport: projection compiled to artifact=%d parser=%d", inv.Artifact, inv.Parser)
	}
	call, _, cleanup, err := stageCall(inv)
	defer cleanup()
	if err != nil {
		return "", "", fmt.Errorf("astrolog: ComputeRawReport: %w", err)
	}
	if err := validateArgv(call.argv); err != nil {
		return "", "", fmt.Errorf("astrolog: ComputeRawReport: %w", err)
	}
	// The same degree character the chart call prints (`-k` without colour):
	// every orb reads a0°04', never 0:04'. The one byte translation is
	// reportText's (the specification C8).
	call, err = call.with(rawReportDegreeSwitch)
	if err != nil {
		return "", "", fmt.Errorf("astrolog: ComputeRawReport: %w", err)
	}
	out, command, err := spawn(ctx, call)
	if err != nil {
		return "", command, fmt.Errorf("astrolog: ComputeRawReport: %w (output: %.400s)", err, out)
	}
	return reportText(out), command, nil
}

// ComputeCrossAspects runs astrolog's -r0 relationship aspect list for a
// Comparison spec and returns the cross-aspects between the two charts — Primary
// from the first chart, Secondary from the second (roles preserved). Unlike
// ComputeChart it is a SINGLE invocation: the -r0 -a output IS the cross-aspect
// list, so there is no separate positions run (and thus no 0Ari sanity scan —
// there is no positions block to scan). It stages the two charts as temp chart
// files, runs under ctx (a superseded run SIGKILLs cleanly), and parses with the
// role-preserving cross-aspect parser. The spec's Source must be a Comparison.
func ComputeCrossAspects(ctx context.Context, spec recipe.ChartSpec) (payload.CrossAspectOutput, error) {
	plan, err := Compile(spec, recipe.Aspects{})
	if err != nil {
		return payload.CrossAspectOutput{}, fmt.Errorf("astrolog: ComputeCrossAspects: %w", err)
	}
	inv := plan.Invocations[0]
	if inv.Parser != ParseCrossAspects {
		return payload.CrossAspectOutput{}, fmt.Errorf("astrolog: ComputeCrossAspects: spec source is not a Comparison; use ComputeChart for a single chart")
	}
	// Commands accumulate in spawn order — the ephemeris probe first, then
	// the relationship run — and every return after a spawn started carries
	// what ran so far. A relationship command names staged chart
	// paths that cleanup removes below: it is exactly what ran, but not
	// replayable (documented non-goal, an earlier change).
	var commands []string
	// Ephemeris guard (parity with Run/RunPositionsOnly): the -r0 -a output is an
	// aspect list with no positions block to sanity-scan, and a disabled Swiss
	// Ephemeris collapses every body to 0°Aries — turning the cross-aspects into
	// a flood of bogus 0-orb conjunctions whose <Ari> tags the parser silently
	// drops to empty. Probe the PRIMARY chart's positions first and reuse
	// checkPositionSanity, so an ephemeris failure surfaces as
	// ErrEphemerisDisabled rather than silent/garbage cross-aspects.
	if comp, ok := spec.Source.(recipe.Comparison); ok {
		// A pair moved to local-clock chart files keeps this probe: the
		// primary's UT moment is what the probe casts (the specification N27).
		primarySrc := comp.Primary
		if l, isLocal := primarySrc.(recipe.LocalMomentAtPlace); isLocal {
			primarySrc = recipe.MomentAtPlace{JD: l.JD, Lon: l.Lon, Lat: l.Lat}
		}
		if primary, ok := primarySrc.(recipe.MomentAtPlace); ok {
			// The probe uses a GUARANTEED multi-body policy (AstrologDefault),
			// NOT the caller's spec.Objects: checkPositionSanity needs >=2 bodies
			// rendering 0Ari00 to detect a collapse, so a restrictive policy
			// (e.g. one that removes all but one body) must not blind the guard.
			// The probe only verifies ephemeris liveness; the body SET is
			// irrelevant to that, while the actual -r0 run honors spec.Objects.
			// The probe never carries -zZ, whatever the source names.
			primary.TZ = ""
			// Nor a name: the probe's command is shown in the console, and a
			// pair's names travel in its staged files alone.
			primary.Name, primary.Place = "", ""
			probe := recipe.ChartSpec{
				Source:  primary,
				Houses:  spec.Houses,
				Zodiac:  spec.Zodiac,
				Objects: recipe.ObjectPolicy{Baseline: recipe.AstrologDefault},
			}
			probed, err := ComputeChartPositionsOnly(ctx, probe)
			commands = append(commands, probed.Commands...)
			if err != nil {
				return payload.CrossAspectOutput{Commands: commands}, fmt.Errorf("astrolog: ComputeCrossAspects: ephemeris probe: %w", err)
			}
		}
	}
	staged, _, cleanup, err := stageCall(inv)
	defer cleanup()
	if err != nil {
		return payload.CrossAspectOutput{Commands: commands}, fmt.Errorf("astrolog: ComputeCrossAspects: %w", err)
	}
	call, err := staged.derive(colorAspectsArgv)
	if err != nil {
		return payload.CrossAspectOutput{Commands: commands}, fmt.Errorf("astrolog: ComputeCrossAspects: %w", err)
	}
	if err := validateArgv(call.argv); err != nil {
		return payload.CrossAspectOutput{Commands: commands}, fmt.Errorf("astrolog: ComputeCrossAspects: %w", err)
	}
	out, command, err := spawn(ctx, call)
	commands = recordCommand(commands, command)
	if err != nil {
		return payload.CrossAspectOutput{Commands: commands}, fmt.Errorf("astrolog: ComputeCrossAspects: %w", err)
	}
	cross := parseCrossAspects(out)
	if err := checkCrossAspectSanity(out); err != nil {
		return payload.CrossAspectOutput{Commands: commands}, fmt.Errorf("astrolog: ComputeCrossAspects: %w (output: %.400s)", err, out)
	}
	rows, err := astrologstyle.ParseCrossAspectRows(out)
	if err != nil {
		return payload.CrossAspectOutput{Commands: commands}, fmt.Errorf("astrolog: ComputeCrossAspects: %w", err)
	}
	return payload.CrossAspectOutput{Aspects: cross, AspectStyles: mapCrossAspectStyles(rows), Report: reportText(out), Commands: commands}, nil
}

// formatLon emits astrolog's "<deg>:<arcmin><E|W>" form. Atlas convention
// rounds to whole arcminutes — sub-arcminute precision (≈1.8 km at the
// equator) is below astrolog's input resolution anyway.
func formatLon(lon float64) string {
	cardinal := "E"
	if lon < 0 {
		cardinal = "W"
		lon = -lon
	}
	deg, min := splitDegMin(lon)
	return fmt.Sprintf("%d:%02d%s", deg, min, cardinal)
}

// formatLat emits astrolog's "<deg>:<arcmin><N|S>" form.
func formatLat(lat float64) string {
	cardinal := "N"
	if lat < 0 {
		cardinal = "S"
		lat = -lat
	}
	deg, min := splitDegMin(lat)
	return fmt.Sprintf("%d:%02d%s", deg, min, cardinal)
}

// splitDegMin splits a non-negative decimal-degree value into integer
// degree + arcminute, with minute carry on rounding (59.6 → deg=60, min=0).
func splitDegMin(d float64) (int, int) {
	deg := int(math.Floor(d))
	min := int(math.Round((d - float64(deg)) * 60))
	if min == 60 {
		deg++
		min = 0
	}
	return deg, min
}
